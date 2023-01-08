package procstat

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type Filter func(p *process.Process) bool

// NativeFinder uses gopsutil to find processes
type NativeFinder struct {
}

func NewNativeFinder() (PIDFinder, error) {
	return &NativeFinder{}, nil
}

// Uid will return all pids for the given user
func (pg *NativeFinder) UID(user string) ([]PID, error) {
	var dst []PID
	procs, err := process.Processes()
	if err != nil {
		return dst, err
	}
	for _, p := range procs {
		username, err := p.Username()
		if err != nil {
			// skip, this can happen if we don't have permissions or
			// the pid no longer exists
			continue
		}
		if username == user {
			dst = append(dst, PID(p.Pid))
		}
	}
	return dst, nil
}

// PidFile returns the pid from the pid file given.
func (pg *NativeFinder) PidFile(path string) ([]PID, error) {
	var pids []PID
	pidString, err := os.ReadFile(path)
	if err != nil {
		return pids, fmt.Errorf("failed to read pidfile '%s'. Error: '%s'",
			path, err)
	}
	pid, err := strconv.ParseInt(strings.TrimSpace(string(pidString)), 10, 32)
	if err != nil {
		return pids, err
	}
	pids = append(pids, PID(pid))
	return pids, nil
}

// FullPattern matches on the command line when the process was executed
func (pg *NativeFinder) FullPattern(pattern string, filters ...Filter) ([]PID, error) {
	var pids []PID

	procs, err := pg.FastProcessList()
	if err != nil {
		return pids, err
	}
PROCS:
	for _, p := range procs {
		for _, filter := range filters {
			if !filter(p) {
				continue PROCS
			}
		}
		cmd, err := p.Cmdline()
		if err != nil {
			// skip, this can be caused by the pid no longer existing
			// or you having no permissions to access it
			continue
		}
		if strings.Contains(cmd, pattern) {
			pids = append(pids, PID(p.Pid))
		}
	}
	return pids, err

}

func (pg *NativeFinder) FastProcessList() ([]*process.Process, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	result := make([]*process.Process, len(pids))
	for i, pid := range pids {
		result[i] = &process.Process{Pid: pid}
	}
	return result, nil
}
