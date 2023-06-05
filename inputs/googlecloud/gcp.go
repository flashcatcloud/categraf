package googlecloud

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
)

const (
	inputName = "googlecloud"
)

type (
	GoogleCloud struct {
		config.PluginConfig
		Instances []*Instance `toml:"instances"`
	}
)

var _ inputs.Input = new(GoogleCloud)
var _ inputs.InstancesGetter = new(GoogleCloud)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &GoogleCloud{}
	})
}

func (g *GoogleCloud) Clone() inputs.Input {
	return &GoogleCloud{}
}

func (c *GoogleCloud) Name() string {
	return inputName
}

func (c *GoogleCloud) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(c.Instances))
	for i := 0; i < len(c.Instances); i++ {
		ret[i] = c.Instances[i]
	}
	return ret
}

func (c *GoogleCloud) Drop() {
	for i := 0; i < len(c.Instances); i++ {
		c.Instances[i].Drop()
	}
}
