package tpl

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "plugin_tpl"

type PluginTpl struct {
	config.Interval
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &PluginTpl{}
	})
}

func (pt *PluginTpl) Prefix() string              { return inputName }
func (pt *PluginTpl) Init() error                 { return nil }
func (pt *PluginTpl) Drop()                       {}
func (pt *PluginTpl) Gather(slist *list.SafeList) {}

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

func (ins *Instance) Init() error {
	return nil
}

func (ins *Instance) Gather(slist *list.SafeList) {

}
