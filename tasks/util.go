package tasks

import (
	"sync"
	"strconv"
)

var (
	TasksMu sync.Mutex               // guards tasks
	Tasks   = make(map[string]*Task) // name -> Task
)

func GetTask(name string) (t *Task, ok bool) {
	TasksMu.Lock()
	defer TasksMu.Unlock()
	t, ok = Tasks[name]
	return
}

func DeleteTask(name string) {
	TasksMu.Lock()
	defer TasksMu.Unlock()
	delete(Tasks, name)
}

// GetOrMakeTask returns or create the named task.
func GetOrMakeTask(name string, tf TaskFile) *Task {
	TasksMu.Lock()
	defer TasksMu.Unlock()
	t, ok := Tasks[name]
	if !ok {
		t = NewTask(name)
		t.tf = tf
		Tasks[name] = t
	}
	return t
}

func atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
