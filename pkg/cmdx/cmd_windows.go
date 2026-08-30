//go:build windows
// +build windows

package cmdx

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func CmdWait(cmd *exec.Cmd, timeout time.Duration) (error, bool) {
	var err error

	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		err = cmd.Process.Signal(syscall.SIGKILL)
		if cmd.WaitDelay > 0 && errors.Is(err, os.ErrProcessDone) {
			// The main process already exited; Wait is only draining inherited
			// pipes and will be bounded by WaitDelay.
			return <-done, false
		}
		go func() {
			<-done // allow goroutine to exit
		}()
		return err, true
	case err = <-done:
		return err, false
	}
}

func CmdStart(cmd *exec.Cmd) error {
	return cmd.Start()
}
