package procstat

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecJstat_Command(t *testing.T) {
	defer func() {
		execCommand = exec.Command
		execLookPath = exec.LookPath
	}()
	execLookPath = func(file string) (string, error) {
		return file, nil
	}

	tests := []struct {
		name        string
		useSudo     bool
		pathJstat   string
		pid         PID
		expectedCmd string
		// expectedArgs ...
	}{
		{
			name:        "Default",
			useSudo:     false,
			pathJstat:   "jstat",
			pid:         1234,
			expectedCmd: "jstat",
		},
		{
			name:        "Sudo Enabled",
			useSudo:     true,
			pathJstat:   "jstat",
			pid:         1234,
			expectedCmd: "sudo",
		},
		{
			name:        "Custom Path",
			useSudo:     false,
			pathJstat:   "/usr/bin/jstat",
			pid:         1234,
			expectedCmd: "/usr/bin/jstat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedCmd := ""
			capturedArgs := []string{}
			execCommand = func(command string, args ...string) *exec.Cmd {
				capturedCmd = command
				capturedArgs = args
				return exec.Command("echo", "")
			}

			ins := &Instance{
				UseSudo:   tt.useSudo,
				PathJstat: tt.pathJstat,
			}
			if ins.PathJstat == "" {
				ins.PathJstat = "jstat"
			}

			// We need to call ins.execJstat
			// It's private, so we can call it if we are in same package.
			_, _ = ins.execJstat(tt.pid)

			assert.Equal(t, tt.expectedCmd, capturedCmd)
			if tt.useSudo {
				assert.Equal(t, tt.pathJstat, capturedArgs[0])
			}
		})
	}
}
