package ipmi

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
)

const (
	inputName = "ipmi"
)

type Ipmi struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Ipmi{}
	})
}

func (i *Ipmi) Clone() inputs.Input {
	return &Ipmi{}
}

func (c *Ipmi) Name() string {
	return inputName
}

func (c *Ipmi) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(c.Instances))
	for i := 0; i < len(c.Instances); i++ {
		ret[i] = c.Instances[i]
	}
	return ret
}

func (c *Ipmi) Drop() {
	for i := 0; i < len(c.Instances); i++ {
		c.Instances[i].Drop()
	}
}
