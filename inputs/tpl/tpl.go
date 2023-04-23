package tpl

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
)

const inputName = "plugin_tpl"

type PluginTpl struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &PluginTpl{}
	})
}

func (pt *PluginTpl) Clone() inputs.Input {
	return &PluginTpl{}
}

func (pt *PluginTpl) Name() string {
	return inputName
}

func (pt *PluginTpl) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
}
