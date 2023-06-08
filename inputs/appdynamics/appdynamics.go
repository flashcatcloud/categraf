package appdynamics

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
)

const (
	inputName = "appdynamics"
)

type (
	AppDynamics struct {
		config.PluginConfig

		Instances []*Instance `toml:"instances"`
	}
)

var _ inputs.Input = new(AppDynamics)
var _ inputs.InstancesGetter = new(AppDynamics)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &AppDynamics{}
	})
}

func (ad *AppDynamics) Clone() inputs.Input {
	return &AppDynamics{}
}

func (ad *AppDynamics) Name() string {
	return inputName
}

func (ad *AppDynamics) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(ad.Instances))
	for i := 0; i < len(ad.Instances); i++ {
		ret[i] = ad.Instances[i]
	}
	return ret
}

func (ad *AppDynamics) Drop() {
	for i := 0; i < len(ad.Instances); i++ {
		ad.Instances[i].Drop()
	}
}
