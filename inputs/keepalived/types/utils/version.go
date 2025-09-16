package utils

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/hashicorp/go-version"
)

// ParseVersion returns keepalived version from keepalived -v command output.
func ParseVersion(versionOutput string) (*version.Version, error) {
	// version is always at first line
	lines := strings.SplitN(versionOutput, "\n", 2)
	if len(lines) != 2 {
		slog.Error("Failed to parse keepalived version output",
			"output", versionOutput,
			"lines", lines,
		)

		return nil, errors.New("failed to parse keepalived version output")
	}

	versionString := lines[0]

	args := strings.Split(versionString, " ")
	if len(args) < 2 {
		slog.Error("Failed to parse keepalived version string",
			"versionString", versionString,
			"args", args,
		)

		return nil, errors.New("unknown keepalived version format")
	}

	version, err := version.NewVersion(args[1][1:])
	if err != nil {
		slog.Error("Failed to parse keepalived version",
			"versionString", args[1][1:],
			"error", err,
		)

		return nil, errors.New("failed to parse keepalived version")
	}

	return version, nil
}
