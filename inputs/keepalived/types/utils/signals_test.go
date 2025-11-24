package utils

import (
	"syscall"
	"testing"

	"github.com/hashicorp/go-version"
)

func TestHasSigNumSupport(t *testing.T) {
	t.Parallel()

	notSupportingVersion := version.Must(version.NewVersion("1.3.5"))
	if HasSigNumSupport(notSupportingVersion) {
		t.Fail()
	}

	supportingVersion := version.Must(version.NewVersion("1.3.8"))
	if !HasSigNumSupport(supportingVersion) {
		t.Fail()
	}

	if !HasSigNumSupport(nil) {
		t.Fail()
	}
}

func TestGetDefaultSignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sigString string
		expected  syscall.Signal
	}{
		{"DATA signal", "DATA", syscall.SIGUSR1},
		{"STATS signal", "STATS", syscall.SIGUSR2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := GetDefaultSignal(tt.sigString)
			if err != nil {
				t.Fatalf("GetDefaultSignal(%q) returned unexpected error: %v", tt.sigString, err)
			}
			if actual != tt.expected {
				t.Errorf("GetDefaultSignal(%q) = %v, want %v", tt.sigString, actual, tt.expected)
			}
		})
	}
}
