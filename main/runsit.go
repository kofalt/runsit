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
	"net"
	"os"
	"os/signal"
	"sort"
	"syscall"

	. "github.com/bradfitz/runsit/tasks"
	. "github.com/bradfitz/runsit/logger"
)

// Flags.
var (
	httpPort  = flag.Int("http_port", 4762, "HTTP localhost admin port.")
	configDir = flag.String("config_dir", "/etc/runsit", "Directory containing per-task *.json config files.")
)


func watchConfigDir() {
	for tf := range dirWatcher().Updates() {
		t := GetOrMakeTask(tf.Name(), tf)
		go t.Update(tf)
	}
}

type byName []*Task

func (s byName) Len() int           { return len(s) }
func (s byName) Less(i, j int) bool { return s[i].Name < s[j].Name }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// GetTasks returns all known tasks.
func GetTasks() []*Task {
	ts := []*Task{}
	TasksMu.Lock()
	defer TasksMu.Unlock()
	for _, t := range Tasks {
		ts = append(ts, t)
	}
	sort.Sort(byName(ts))
	return ts
}

func handleSignals() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc)

	for s := range sigc {
		switch s {
		case os.Interrupt, os.Signal(syscall.SIGTERM):
			Logger.Printf("Got signal %q; stopping all tasks.", s)
			for _, t := range GetTasks() {
				t.Stop()
			}
			Logger.Printf("Tasks all stopped after %s; quitting.", s)
			os.Exit(0)
		case os.Signal(syscall.SIGCHLD):
			// Ignore.
		default:
			Logger.Printf("unhandled signal: %T %#v", s, s)
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
		Logger.Printf("Error listening on port %d: %v", *httpPort, err)
		os.Exit(1)
		return
	}
	Logger.Printf("Listening on port %d", *httpPort)

	go handleSignals()
	go watchConfigDir()
	go runWebServer(ln)
	select {}
}
