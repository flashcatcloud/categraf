package procstat

import (
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

// Pattern matches on the process name
func (pg *NativeFinder) Pattern(pattern string, user string) ([]PID, error) {
	var pids []PID

	if user != "" {
		procs, err := process.Processes()
		if err != nil {
			return pids, err
		}
		for _, p := range procs {
			username, err := p.Username()
			if err != nil {
				//skip, this can happen if we don't have permissions or
				//the pid no longer exists
				continue
			}
			if username == user {
				name, err := p.Name()
				if err != nil {
					//skip, this can be caused by the pid no longer existing
					//or you having no permissions to access it
					continue
				}
				if strings.Contains(name, pattern) {
					pids = append(pids, PID(p.Pid))
				}
			}
		}
		return pids, err
	} else {
		procs, err := pg.FastProcessList()
		if err != nil {
			return pids, err
		}
		for _, p := range procs {
			name, err := p.Name()
			if err != nil {
				//skip, this can be caused by the pid no longer existing
				//or you having no permissions to access it
				continue
			}
			if strings.Contains(name, pattern) {
				pids = append(pids, PID(p.Pid))
			}
		}
		return pids, err
	}

}
