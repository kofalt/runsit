/*
Copyright 2011 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Author: Brad Fitzpatrick <brad@danga.com>

// runsit runs stuff.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bradfitz/runsit/tasks"
)

// Flags.
var (
	httpPort  = flag.Int("http_port", 4762, "HTTP localhost admin port.")
	configDir = flag.String("config_dir", "/etc/runsit", "Directory containing per-task *.json config files.")
)

var (
	logBuf = new(logBuffer)
	logger = log.New(io.MultiWriter(os.Stderr, logBuf), "", log.Lmicroseconds|log.Lshortfile)
)

const systemLogSize = 64 << 10

// logBuffer is a ring buffer.
type logBuffer struct {
	mu   sync.Mutex
	i    int
	full bool
	buf  [systemLogSize]byte
}

func (b *logBuffer) Write(p []byte) (ntot int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(p) > 0 {
		n := copy(b.buf[b.i:], p)
		p = p[n:]
		ntot += n
		b.i += n
		if b.i == len(b.buf) {
			b.i = 0
			b.full = true
		}
	}
	return
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.full {
		return string(b.buf[:b.i])
	}
	s := string(b.buf[b.i:]) + string(b.buf[:b.i])
	if nl := strings.Index(s, "\n"); nl != -1 {
		// Remove first line, since it's probably truncated
		s = s[nl+1:]
	}
	return "...\n" + s
}

// Line is a line of output from a TaskInstance.
type Line struct {
	T    time.Time
	Name string // "stdout", "stderr", or "system"
	Data string // line or prefix of line

	isPrefix bool // truncated line? (too long)
	instance *TaskInstance
}

type updateMessage struct {
	tf TaskFile
}

type stopMessage struct {
	resc chan error
}

type restartIfStoppedMessage struct{}

// instanceGoneMessage is sent when a task instance's process finishes,
// successfully or otherwise. Any error is in instance.waitErr.
type instanceGoneMessage struct {
	in *TaskInstance
}

// statusRequestMessage is sent from the web UI (via the
// RunningInstance accessor) to obtain the task's current status
type statusRequestMessage struct {
	resCh chan<- *TaskStatus
}

func watchConfigDir() {
	for tf := range dirWatcher().Updates() {
		t := GetOrMakeTask(tf.Name(), tf)
		go t.Update(tf)
	}
}

var (
	tasksMu sync.Mutex               // guards tasks
	tasks   = make(map[string]*Task) // name -> Task
)

func GetTask(name string) (t *Task, ok bool) {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	t, ok = tasks[name]
	return
}

func DeleteTask(name string) {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	delete(tasks, name)
}

// GetOrMakeTask returns or create the named task.
func GetOrMakeTask(name string, tf TaskFile) *Task {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	t, ok := tasks[name]
	if !ok {
		t = NewTask(name)
		t.tf = tf
		tasks[name] = t
	}
	return t
}

type byName []*Task

func (s byName) Len() int           { return len(s) }
func (s byName) Less(i, j int) bool { return s[i].Name < s[j].Name }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// GetTasks returns all known tasks.
func GetTasks() []*Task {
	ts := []*Task{}
	tasksMu.Lock()
	defer tasksMu.Unlock()
	for _, t := range tasks {
		ts = append(ts, t)
	}
	sort.Sort(byName(ts))
	return ts
}

func atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}

func handleSignals() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc)

	for s := range sigc {
		switch s {
		case os.Interrupt, os.Signal(syscall.SIGTERM):
			logger.Printf("Got signal %q; stopping all tasks.", s)
			for _, t := range GetTasks() {
				t.Stop()
			}
			logger.Printf("Tasks all stopped after %s; quitting.", s)
			os.Exit(0)
		case os.Signal(syscall.SIGCHLD):
			// Ignore.
		default:
			logger.Printf("unhandled signal: %T %#v", s, s)
		}
	}
}

func main() {
	MaybeBecomeChildProcess()
	flag.Parse()

	listenAddr := "localhost"
	if a := os.Getenv("RUNSIT_LISTEN"); a != "" {
		listenAddr = a
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", listenAddr, *httpPort))
	if err != nil {
		logger.Printf("Error listening on port %d: %v", *httpPort, err)
		os.Exit(1)
		return
	}
	logger.Printf("Listening on port %d", *httpPort)

	go handleSignals()
	go watchConfigDir()
	go runWebServer(ln)
	select {}
}
