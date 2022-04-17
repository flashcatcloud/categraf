package config

import (
	"strconv"
	"strings"
	"time"
)

// Duration is a time.Duration
type Duration time.Duration

// UnmarshalTOML parses the duration from the TOML config file
func (d *Duration) UnmarshalTOML(b []byte) error {
	// convert to string
	durStr := string(b)

	// Value is a TOML number (e.g. 3, 10, 3.5)
	// First try parsing as integer seconds
	sI, err := strconv.ParseInt(durStr, 10, 64)
	if err == nil {
		dur := time.Second * time.Duration(sI)
		*d = Duration(dur)
		return nil
	}
	// Second try parsing as float seconds
	sF, err := strconv.ParseFloat(durStr, 64)
	if err == nil {
		dur := time.Second * time.Duration(sF)
		*d = Duration(dur)
		return nil
	}

	// Finally, try value is a TOML string (e.g. "3s", 3s) or literal (e.g. '3s')
	durStr = strings.ReplaceAll(durStr, "'", "")
	durStr = strings.ReplaceAll(durStr, "\"", "")
	if durStr == "" {
		durStr = "0s"
	}

	dur, err := time.ParseDuration(durStr)
	if err != nil {
		return err
	}

	*d = Duration(dur)
	return nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	return d.UnmarshalTOML(text)
}
