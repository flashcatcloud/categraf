//go:build !windows
// +build !windows

package procstat

import (
	"strings"
)

// Pattern matches on the process name
func (pg *NativeFinder) Pattern(pattern string, filters ...Filter) ([]PID, error) {
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
		name, err := p.Exe()
		if err != nil {
			// skip, this can be caused by the pid no longer existing
			// or you having no permissions to access it
			continue
		}
		if strings.Contains(name, pattern) {
			pids = append(pids, PID(p.Pid))
		}
	}
	return pids, err
}
