package tasks

type TaskFile interface {
	// Name returns the task's base name, without any directory
	// prefix or .json suffix.
	Name() string

	// ConfigFileName returns the filename of the JSON file to read.
	// This returns the empty string when a file has been deleted.
	// TODO: make this more abstract, a ReadSeekCloser instead?
	ConfigFileName() string
}
