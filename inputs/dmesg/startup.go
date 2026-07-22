//go:build linux
// +build linux

package dmesg

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const startupLookbackAll = "-1"

type kmsgSeeker interface {
	Seek(offset int64, whence int) (int64, error)
}

func setupStartupPosition(f kmsgSeeker, value string, nowUsec func() (int64, error)) (int64, error) {
	lookback, readAll, err := parseStartupLookback(value)
	if err != nil {
		return 0, err
	}

	if readAll {
		_, err := f.Seek(0, io.SeekStart)
		if err != nil {
			return 0, fmt.Errorf("seek /dev/kmsg to beginning: %w", err)
		}
		return 0, nil
	}

	if lookback == 0 {
		_, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, fmt.Errorf("seek /dev/kmsg to end: %w", err)
		}
		return 0, nil
	}

	now, err := nowUsec()
	if err != nil {
		return 0, fmt.Errorf("get monotonic time: %w", err)
	}

	minTsUsec := now - lookback.Microseconds()
	if minTsUsec < 0 {
		minTsUsec = 0
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("seek /dev/kmsg to beginning: %w", err)
	}

	return minTsUsec, nil
}

func parseStartupLookback(value string) (time.Duration, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, false, nil
	}
	if value == startupLookbackAll {
		return 0, true, nil
	}

	lookback, err := time.ParseDuration(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid startup_lookback %q: use a duration such as \"1h\" or \"0s\", or %q to read all retained messages", value, startupLookbackAll)
	}
	if lookback < 0 {
		return 0, false, fmt.Errorf("invalid startup_lookback %q: negative durations are not supported; use %q to read all retained messages", value, startupLookbackAll)
	}

	return lookback, false, nil
}
