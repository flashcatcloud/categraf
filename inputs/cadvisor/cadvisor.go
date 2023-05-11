package cadvisor

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
)

const (
	inputName = "cadvisor"
)

type Cadvisor struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Cadvisor{}
	})
}

func (c *Cadvisor) Clone() inputs.Input {
	return &Cadvisor{}
}

func (c *Cadvisor) Name() string {
	return inputName
}

func (c *Cadvisor) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(c.Instances))
	for i := 0; i < len(c.Instances); i++ {
		ret[i] = c.Instances[i]
	}
	return ret
}

func (c *Cadvisor) Drop() {
	for i := 0; i < len(c.Instances); i++ {
		c.Instances[i].Drop()
	}
}
