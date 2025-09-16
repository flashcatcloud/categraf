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

	if dataSignal := syscall.SIGUSR1; GetDefaultSignal("DATA") != dataSignal {
		t.Fail()
	}

	if statsSignal := syscall.SIGUSR2; GetDefaultSignal("STATS") != statsSignal {
		t.Fail()
	}
}
