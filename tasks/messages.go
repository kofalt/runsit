package tasks

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
