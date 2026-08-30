//go:build !windows
// +build !windows

package cmdx

import (
	"errors"
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

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		// IMPORTANT: cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} is necessary before cmd.Start()
		killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if cmd.WaitDelay <= 0 {
			go func() {
				<-done // allow goroutine to exit after inherited pipes close
			}()
			return killErr, true
		}
		if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
			go func() {
				<-done // allow goroutine to exit
			}()
			return killErr, true
		}
		waitErr := <-done
		if killErr == nil && processWasSignaled(cmd) {
			return killErr, true
		}
		// The main process had already exited normally; only inherited pipes
		// reached the outer timeout. Preserve its real result.
		return waitErr, false
	case err = <-done:
		if errors.Is(err, exec.ErrWaitDelay) {
			// The main process has already been reaped. Stop descendants that
			// kept the command's output pipes open.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return err, false
	}
}

func processWasSignaled(cmd *exec.Cmd) bool {
	if cmd.ProcessState == nil {
		return false
	}
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	return ok && status.Signaled()
}

func CmdStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}
