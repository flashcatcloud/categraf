package cmdx

import (
	"os/exec"
	"time"
)

func RunTimeout(cmd *exec.Cmd, timeout time.Duration) (error, bool) {
	err := CmdStart(cmd)
	if err != nil {
		return err, false
	}

	return CmdWait(cmd, timeout)
}

// RunTimeoutWithDrain bounds both the command runtime and the time spent
// draining inherited stdout/stderr pipes after the main process exits.
func RunTimeoutWithDrain(cmd *exec.Cmd, timeout, drainGrace time.Duration) (error, bool) {
	cmd.WaitDelay = drainGrace
	return RunTimeout(cmd, timeout)
}
