package netstat

import (
	"path/filepath"
	"strconv"
)

// ProcNetstat models the content of /proc/<pid>/net/netstat.
type ProcNetstat struct {
	// The process ID.
	PID    int
	TcpExt map[string]interface{}
	IpExt  map[string]interface{}
}

// FS represents a pseudo-filesystem, normally /proc or /sys, which provides an
// interface to kernel data structures.
type FS string

// Path appends the given path elements to the filesystem path, adding separators
// as necessary.
func (fs FS) Path(p ...string) string {
	return filepath.Join(append([]string{string(fs)}, p...)...)
}

// Proc provides information about a running process.
type Proc struct {
	// The process ID.
	PID int

	fs FS
}

func (p Proc) path(pa ...string) string {
	if p.PID == 0 {
		return p.fs.Path(pa...)
	}
	return p.fs.Path(append([]string{strconv.Itoa(p.PID)}, pa...)...)
}
