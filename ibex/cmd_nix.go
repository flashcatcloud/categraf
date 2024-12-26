//go:build !no_ibex && !windows

package ibex

import (
	"os/exec"
	"syscall"
)

func CmdStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func CmdKill(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func ansiToUtf8(mbcs []byte) (string, error) {
	// fake
	return string(mbcs), nil
}

func utf8ToAnsi(utf8 string) (string, error) {
	// fake
	return utf8, nil
}
