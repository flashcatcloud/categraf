//go:build !linux
// +build !linux

package linux_sysctl_fs

import (
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "linux_sysctl_fs"

type SysctlFS struct {
	PrintConfigs    bool  `toml:"print_configs"`
	IntervalSeconds int64 `toml:"interval_seconds"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &SysctlFS{}
	})
}

func (s *SysctlFS) GetInputName() string {
	return inputName
}

func (s *SysctlFS) GetIntervalSeconds() int64 {
	return s.IntervalSeconds
}

func (s *SysctlFS) Init() error {
	return nil
}

func (s *SysctlFS) Gather() (samples []*types.Sample) {
	return
}
