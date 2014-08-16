package tasks

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// LaunchRequest is a subset of exec.Cmd plus the addition of Uid/Gid.
// This structure is gob'd and base64'd and sent to the child process
// in the environment variable _RUNSIT_LAUNCH_INFO.  The child then
// drops root and execs itself to be the requested process.
type LaunchRequest struct {
	Uid      int   // or 0 to not change
	Gid      int   // or 0 to not change
	Gids     []int // supplemental
	Path     string
	Env      []string
	Argv     []string // must include Path as argv[0]
	Dir      string
	NumFiles int // new nfile fd rlimit, or 0 to not change
}

func (lr *LaunchRequest) start(extraFiles []*os.File) (cmd *exec.Cmd, outPipe, errPipe io.ReadCloser, err error) {
	var buf bytes.Buffer
	b64enc := base64.NewEncoder(base64.StdEncoding, &buf)
	err = gob.NewEncoder(b64enc).Encode(lr)
	b64enc.Close()
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			for _, p := range []io.ReadCloser{outPipe, errPipe} {
				if p != nil {
					p.Close()
				}
			}
		}
	}()

	cmd = exec.Command(os.Args[0])
	cmd.Env = append(cmd.Env, "_RUNSIT_LAUNCH_INFO="+buf.String())
	cmd.ExtraFiles = extraFiles
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	outPipe, err = cmd.StdoutPipe()
	if err != nil {
		return
	}
	errPipe, err = cmd.StderrPipe()
	if err != nil {
		return
	}

	err = cmd.Start()
	if err != nil {
		return
	}

	return cmd, outPipe, errPipe, nil
}
