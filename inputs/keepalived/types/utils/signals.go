package utils

import (
	"fmt"
	"log/slog"
	"syscall"

	"github.com/hashicorp/go-version"
)

var (
	sigNumSupportedVersion = version.Must(version.NewVersion("1.3.8"))
)

const InvalidSignal = syscall.Signal(-1)

// HasSigNumSupport checks if Keepalived supports --signum command.
func HasSigNumSupport(version *version.Version) bool {
	return version == nil || version.GreaterThanOrEqual(sigNumSupportedVersion)
}

// GetDefaultSignal returns default signals for Keepalived.
func GetDefaultSignal(sigString string) (syscall.Signal, error) {
	sig, ok := defaultSignals[sigString]
	if !ok {
		slog.Error("Unsupported signal for your keepalived",
			"signal", sigString,
			"supportedSignals", defaultSignals,
		)
		return InvalidSignal, fmt.Errorf("unsupported signal for keepalived: %s", sigString)
	}

	return sig, nil
}
