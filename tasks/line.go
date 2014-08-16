package tasks

import (
	"time"
)

// Line is a line of output from a TaskInstance.
type Line struct {
	T    time.Time
	Name string // "stdout", "stderr", or "system"
	Data string // line or prefix of line

	isPrefix bool // truncated line? (too long)
	instance *TaskInstance
}
