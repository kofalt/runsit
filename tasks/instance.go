package tasks

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/bradfitz/runsit/jsonconfig"
	. "github.com/bradfitz/runsit/logger"
)

// TaskInstance is a particular instance of a running (or now dead) Task.
type TaskInstance struct {
	task      *Task          // set once; not goroutine safe (may only call public methods)
	StartTime time.Time      // set once; immutable
	config    jsonconfig.Obj // set once; immutable
	Lr        *LaunchRequest // set once; immutable (actual command parameters)
	cmd       *exec.Cmd      // set once; immutable (command parameters to helper process)
	output    TaskOutput     // internal locking, safe for concurrent access

	// Set (in awaitDeath) when task finishes running:
	endTime time.Time
	waitErr error // typically nil or *exec.ExitError
}

// ID returns a unique ID string for this task instance.
func (in *TaskInstance) ID() string {
	return fmt.Sprintf("%q/%d-pid%d", in.task.Name, in.StartTime.Unix(), in.Pid())
}

func (in *TaskInstance) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(fmt.Sprintf("Task %s: %s", in.ID(), format), args...)
	in.output.Add(&Line{
		T:        time.Now(),
		Name:     "system",
		Data:     msg,
		instance: in,
	})
	Logger.Print(msg)
}

func (in *TaskInstance) Pid() int {
	if in.cmd == nil || in.cmd.Process == nil {
		return 0
	}
	return in.cmd.Process.Pid
}

func (in *TaskInstance) Output() []*Line {
	return in.output.lineSlice()
}

// run in its own goroutine
func (in *TaskInstance) awaitDeath() {
	in.waitErr = in.cmd.Wait()
	in.endTime = time.Now()
	in.task.controlc <- instanceGoneMessage{in}
}

// run in its own goroutine
func (in *TaskInstance) watchPipe(r io.Reader, name string) {
	br := bufio.NewReader(r)
	for {
		sl, isPrefix, err := br.ReadLine()
		if err == io.EOF {
			// Not worth logging about.
			return
		}
		if err != nil {
			in.Printf("pipe %q closed: %v", name, err)
			return
		}
		in.output.Add(&Line{
			T:        time.Now(),
			Name:     name,
			Data:     string(sl),
			isPrefix: isPrefix,
			instance: in,
		})
	}
	panic("unreachable")
}
