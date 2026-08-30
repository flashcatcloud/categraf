//go:build !windows

package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandRunBoundsInheritedOutputPipes(t *testing.T) {
	script := filepath.Join(t.TempDir(), "inherited-pipe.sh")
	content := "#!/bin/sh\nsleep 30 &\necho 'test_metric value=1'\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	timeout := 2 * time.Second
	started := time.Now()
	out, stderr, err := commandRun(script, timeout, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= timeout {
		t.Fatalf("command waited for inherited pipes too long: %s", elapsed)
	}
	if len(stderr) != 0 {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(string(out), "test_metric value=1") {
		t.Fatalf("command output was not preserved: %q", out)
	}
}

func TestCommandRunDoesNotMisclassifyPipeDrainAsRuntimeTimeout(t *testing.T) {
	script := filepath.Join(t.TempDir(), "late-inherited-pipe.sh")
	content := "#!/bin/sh\nsleep 0.1\nsleep 30 &\necho 'late_metric value=1'\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	timeout := 2 * time.Second
	started := time.Now()
	out, _, err := commandRun(script, timeout, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("pipe draining was misclassified as command timeout: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= timeout {
		t.Fatalf("command waited too long for inherited pipe drain: %s", elapsed)
	}
	if !strings.Contains(string(out), "late_metric value=1") {
		t.Fatalf("command output was not preserved: %q", out)
	}
}

func TestEffectiveDrainGraceStaysBelowTimeout(t *testing.T) {
	if got := effectiveDrainGrace(5*time.Second, 0); got != time.Second {
		t.Fatalf("unexpected default drain grace: %s", got)
	}
	if got := effectiveDrainGrace(time.Second, 5*time.Second); got != 500*time.Millisecond {
		t.Fatalf("drain grace was not clamped below timeout: %s", got)
	}
}
