//go:build !windows

package utils

import "syscall"

var defaultSignals = map[string]syscall.Signal{
	"DATA":  syscall.SIGUSR1,
	"STATS": syscall.SIGUSR2,
}
