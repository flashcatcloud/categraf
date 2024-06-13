//go:build !linux

package ethtool

import (
	"log"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "ethtool"

type Ethtool struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Ethtool{}
	})
}

func (pt *Ethtool) Clone() inputs.Input {
	return &Ethtool{}
}

func (pt *Ethtool) Name() string {
	return inputName
}

func (pt *Ethtool) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	// This is the list of interface names to include
	InterfaceInclude []string `toml:"interface_include"`

	// This is the list of interface names to ignore
	InterfaceExclude []string `toml:"interface_exclude"`

	// Behavior regarding metrics for downed interfaces
	DownInterfaces string `toml:" down_interfaces"`

	// This is the list of namespace names to include
	NamespaceInclude []string `toml:"namespace_include"`

	// This is the list of namespace names to ignore
	NamespaceExclude []string `toml:"namespace_exclude"`

	// Normalization on the key names
	NormalizeKeys []string `toml:"normalize_keys"`
}

func (ins *Instance) Init() error {
	log.Println("E! Current platform is not supported")
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	return
}
