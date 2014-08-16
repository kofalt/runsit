package tasks

import (
	"fmt"
	"time"
)

// TaskStatus is an one-time snapshot of a task's status, for rendering in
// the web UI.
type TaskStatus struct {
	Running  *TaskInstance   // or nil, if none running
	StartErr error           // if a task is not running, the reason why it failed to start
	ErrTime  time.Time       // time of StartErr
	StartIn  time.Duration   // non-zero if task is rate-limited and will restart in this time
	Failures []*TaskInstance // past few failures
}

func (s *TaskStatus) Summary() string {
	in := s.Running
	if in != nil {
		return "ok"
	}
	if err := s.StartErr; err != nil {
		return fmt.Sprintf("Start error (%v ago): %v", time.Now().Sub(s.ErrTime), err)
	}
	// TODO: flesh these not running states out.
	// e.g. intentionaly stopped, how long we're pausing before
	// next re-start attempt, etc.
	return "not running"
}

// Status returns the task's status.
func (t *Task) Status() *TaskStatus {
	ch := make(chan *TaskStatus, 1)
	t.controlc <- statusRequestMessage{resCh: ch}
	return <-ch
}

// runs in Task.loop
func (t *Task) status() *TaskStatus {
	failures := make([]*TaskInstance, len(t.failures))
	copy(failures, t.failures)
	s := &TaskStatus{
		Running:  t.running,
		Failures: failures,
	}
	if t.running == nil {
		s.StartErr = t.configErr
		s.ErrTime = t.errTime
	}
	return s
}
