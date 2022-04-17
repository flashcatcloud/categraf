package exec

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "exec"

type Exec struct {
	PrintConfigs bool            `toml:"print_configs"`
	Interval     config.Duration `toml:"interval"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Exec{}
	})
}

func (e *Exec) GetInputName() string {
	return inputName
}

func (e *Exec) GetInterval() config.Duration {
	return e.Interval
}

func (e *Exec) Init() error {
	return nil
}

func (e *Exec) Gather() (samples []*types.Sample) {
	return
}
