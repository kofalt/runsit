package tasks

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/bradfitz/runsit/jsonconfig"
	. "github.com/bradfitz/runsit/logger"
)

// A Task is a named daemon. A single instance of Task exists for the
// life of the runsit daemon, despite how many times the task has
// failed and restarted. (the exception is if the config file for the
// task is deleted, and then the *Task is removed from the global tasks
// map and a new one could appear later with the same name)
type Task struct {
	// Immutable:
	Name     string
	tf       TaskFile
	controlc chan interface{}

	// State owned by loop's goroutine:
	config    jsonconfig.Obj // last valid config
	configErr error          // configuration error
	errTime   time.Time      // of last configErr
	running   *TaskInstance
	failures  []*TaskInstance // last few failures, oldest first.
}

func NewTask(name string) *Task {
	t := &Task{
		Name:     name,
		controlc: make(chan interface{}),
	}
	go t.loop()
	return t
}

func (t *Task) Printf(format string, args ...interface{}) {
	Logger.Printf(fmt.Sprintf("Task %q: %s", t.Name, format), args...)
}

func (t *Task) loop() {
	t.Printf("Starting")
	defer t.Printf("Loop exiting")
	for cm := range t.controlc {
		switch m := cm.(type) {
		case statusRequestMessage:
			m.resCh <- t.status()
		case updateMessage:
			t.update(m.tf)
		case stopMessage:
			err := t.stop()
			m.resc <- err
		case instanceGoneMessage:
			t.onTaskFinished(m)
		case restartIfStoppedMessage:
			t.restartIfStopped()
		}
	}
}

func (t *Task) Update(tf TaskFile) {
	t.controlc <- updateMessage{tf}
}

// run in Task.loop
func (t *Task) onTaskFinished(m instanceGoneMessage) {
	m.in.Printf("Task exited; err=%v", m.in.waitErr)
	if m.in == t.running {
		t.running = nil
	}
	const keepFailures = 5
	if len(t.failures) == keepFailures {
		copy(t.failures, t.failures[1:])
		t.failures = t.failures[:keepFailures-1]
	}
	t.failures = append(t.failures, m.in)

	aliveTime := m.in.endTime.Sub(m.in.StartTime)
	restartIn := 0 * time.Second
	if min := 5 * time.Second; aliveTime < min {
		restartIn = min - aliveTime
	}

	if m.in.waitErr == nil {
		// TODO: vary restartIn based on whether this instance
		// and the previous few completed successfully or not?
	}

	time.AfterFunc(restartIn, func() {
		t.controlc <- restartIfStoppedMessage{}
	})
}

// run in Task.loop
func (t *Task) restartIfStopped() {
	if t.running != nil || t.config == nil {
		return
	}
	t.Printf("Restarting")
	t.updateFromConfig(t.config)
}

// run in Task.loop
func (t *Task) update(tf TaskFile) {
	t.config = nil
	t.stop()

	fileName := tf.ConfigFileName()
	if fileName == "" {
		t.Printf("config file deleted; stopping")
		DeleteTask(t.Name)
		return
	}

	jc, err := jsonconfig.ReadFile(fileName)
	if err != nil {
		t.configError("Bad config file: %v", err)
		return
	}
	t.updateFromConfig(jc)
}

// run in Task.loop
func (t *Task) configError(format string, args ...interface{}) error {
	t.configErr = fmt.Errorf(format, args...)
	t.errTime = time.Now()
	t.Printf("%v", t.configErr)
	return t.configErr
}

// run in Task.loop
func (t *Task) startError(format string, args ...interface{}) error {
	// TODO: make start error and config error different?
	return t.configError(format, args...)
}

func (t *Task) Stop() error {
	errc := make(chan error, 1)
	t.controlc <- stopMessage{errc}
	return <-errc
}

// runs in Task.loop
func (t *Task) stop() error {
	in := t.running
	if in == nil {
		return nil
	}

	// TODO: more graceful kill types
	in.Printf("sending SIGKILL")

	// Was: in.cmd.Process.Kill(); but we want to kill
	// the entire process group.
	processGroup := 0 - in.Pid()
	rv := syscall.Kill(processGroup, 9)
	in.Printf("Kill result: %v", rv)
	t.running = nil
	return nil
}

// run in Task.loop
func (t *Task) updateFromConfig(jc jsonconfig.Obj) (err error) {
	t.config = nil
	t.stop()

	env := []string{}
	stdEnv := jc.OptionalBool("standardEnv", true)

	userStr := jc.OptionalString("user", "")
	groupStr := jc.OptionalString("group", "")

	// TODO: medium-term hack to run on linux/arm which lacks cgo support,
	// so let users define these, even though user.Lookup will fail.
	userErrUid := jc.OptionalString("userLookupErrUid", "")
	userErrGid := jc.OptionalString("userLookupErrGid", "")
	userErrHome := jc.OptionalString("userLookupErrHome", "")

	// TODO: group? requires http://code.google.com/p/go/issues/detail?id=2617
	var runas *user.User
	if userStr != "" {
		runas, err = user.Lookup(userStr)
		if err != nil {
			if userErrUid != "" {
				runas = &user.User{
					Uid:      userErrUid,
					Gid:      userErrGid,
					Username: userStr,
					HomeDir:  userErrHome,
				}
			} else {
				return t.configError("%v", err)
			}
		}
		if stdEnv {
			env = append(env, fmt.Sprintf("USER=%s", userStr))
			env = append(env, fmt.Sprintf("HOME=%s", runas.HomeDir))
		}
	} else {
		if stdEnv {
			env = append(env, fmt.Sprintf("USER=%s", os.Getenv("USER")))
			env = append(env, fmt.Sprintf("HOME=%s", os.Getenv("HOME")))
		}
	}

	envMap := jc.OptionalObject("env")
	envHas := func(k string) bool {
		_, ok := envMap[k]
		return ok
	}
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	if stdEnv && !envHas("PATH") {
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/bin:/usr/sbin:/sbin:/bin")
	}

	extraFiles := []*os.File{}
	ports := jc.OptionalObject("ports")
	for portName, vi := range ports {
		var ln net.Listener
		var err error
		switch v := vi.(type) {
		case float64:
			ln, err = net.Listen("tcp", ":"+strconv.Itoa(int(v)))
		case string:
			ln, err = net.Listen("tcp", v)
		default:
			return t.configError("port %q value must be a string or integer", portName)
		}
		if err != nil {
			restartIn := 5 * time.Second
			time.AfterFunc(restartIn, func() {
				t.controlc <- updateMessage{t.tf}
			})
			return t.startError("port %q listen error: %v; restarting in %v", portName, err, restartIn)
		}
		lf, err := ln.(*net.TCPListener).File()
		if err != nil {
			return t.startError("error getting file of port %q listener: %v", portName, err)
		}
		Logger.Printf("opened port named %q on %v; fd=%d", portName, vi, lf.Fd())
		ln.Close()
		env = append(env, fmt.Sprintf("RUNSIT_PORTFD_%s=%d", portName, 3+len(extraFiles)))
		extraFiles = append(extraFiles, lf)
		defer lf.Close()
	}

	bin := jc.RequiredString("binary")
	dir := jc.OptionalString("cwd", "")
	args := jc.OptionalList("args")
	groups := jc.OptionalList("groups")
	numFiles := jc.OptionalInt("numFiles", 0)
	if err := jc.Validate(); err != nil {
		return t.configError("configuration error: %v", err)
	}
	t.config = jc

	finalBin := bin
	if !filepath.IsAbs(bin) {
		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			return t.configError("finding absolute path of dir %q: %v", dir, err)
		}
		finalBin = filepath.Clean(filepath.Join(dirAbs, bin))
	}

	_, err = os.Stat(finalBin)
	if err != nil {
		return t.configError("stat of binary %q failed: %v", bin, err)
	}

	argv := []string{filepath.Base(bin)}
	argv = append(argv, args...)

	lr := &LaunchRequest{
		Path:     bin,
		Env:      env,
		Dir:      dir,
		Argv:     argv,
		NumFiles: numFiles,
	}

	if runas != nil {
		lr.Uid = atoi(runas.Uid)
		lr.Gid = atoi(runas.Gid)
	}
	if groupStr != "" {
		gid, err := LookupGroupId(groupStr)
		if err != nil {
			return t.configError("error looking up group %q: %v", groupStr, err)
		}
		lr.Gid = gid // primary group
	}

	// supplemental groups:
	for _, group := range groups {
		gid, err := LookupGroupId(group)
		if err != nil {
			return t.configError("error looking up group %q: %v", group, err)
		}
		lr.Gids = append(lr.Gids, gid)
	}

	cmd, outPipe, errPipe, err := lr.start(extraFiles)
	if err != nil {
		return t.startError("failed to start: %v", err)
	}

	instance := &TaskInstance{
		task:      t,
		config:    jc,
		StartTime: time.Now(),
		Lr:        lr,
		cmd:       cmd,
	}

	t.Printf("started with PID %d", instance.Pid())
	t.running = instance
	go instance.watchPipe(outPipe, "stdout")
	go instance.watchPipe(errPipe, "stderr")
	go instance.awaitDeath()
	return nil
}
