//go:build !windows

package cmdx

import (
	"bytes"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

const escapedPipeHelperEnv = "CATEGRAF_CMDX_ESCAPED_PIPE_HELPER"

func TestEscapedPipeHelper(t *testing.T) {
	if os.Getenv(escapedPipeHelperEnv) != "1" {
		return
	}
	child := exec.Command("sh", "-c", "sleep 1")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestRunTimeoutReturnsWhileEscapedDescendantHoldsPipe(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestEscapedPipeHelper$")
	cmd.Env = append(os.Environ(), escapedPipeHelperEnv+"=1")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	started := time.Now()
	_, timedOut := RunTimeout(cmd, 100*time.Millisecond)
	if !timedOut {
		t.Fatal("command with an escaped pipe holder was not reported as timed out")
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Fatalf("RunTimeout waited for escaped descendant pipe: %s", elapsed)
	}
}
