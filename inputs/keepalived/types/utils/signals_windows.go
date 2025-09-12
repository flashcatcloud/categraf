//go:build windows

package utils

import "syscall"

var defaultSignals = map[string]syscall.Signal{
	"DATA":  0,
	"STATS": 0,
}
