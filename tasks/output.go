package tasks

import (
	"container/list"
	"sync"
)

// TaskOutput is the output of a TaskInstance.
// Only the last maxKeepLines lines are kept.
type TaskOutput struct {
	mu    sync.Mutex
	lines list.List // of *Line
}

func (to *TaskOutput) Add(l *Line) {
	to.mu.Lock()
	defer to.mu.Unlock()
	to.lines.PushBack(l)
	const maxKeepLines = 5000
	if to.lines.Len() > maxKeepLines {
		to.lines.Remove(to.lines.Front())
	}
}

func (to *TaskOutput) lineSlice() []*Line {
	to.mu.Lock()
	defer to.mu.Unlock()
	var lines []*Line
	for e := to.lines.Front(); e != nil; e = e.Next() {
		lines = append(lines, e.Value.(*Line))
	}
	return lines
}
