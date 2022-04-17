//go:build windows
// +build windows

package processes

import (
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "processes"

type Processes struct {
	PrintConfigs    bool  `toml:"print_configs"`
	IntervalSeconds int64 `toml:"interval_seconds"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Processes{}
	})
}

func (p *Processes) GetInputName() string {
	return inputName
}

func (p *Processes) GetIntervalSeconds() int64 {
	return p.IntervalSeconds
}

func (p *Processes) Init() error {
	return nil
}

func (p *Processes) Gather() (samples []*types.Sample) {
	return
}
