//go:build !linux

package eBPF

import (
	"log"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "eBPF"

type eBPF struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &eBPF{}
	})
}

func (pt *eBPF) Clone() inputs.Input {
	return &eBPF{}
}

func (pt *eBPF) Name() string {
	return inputName
}

func (pt *eBPF) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	Interface string `toml:"interface"`
}

func (ins *Instance) Init() error {
	log.Println("E! Current platform is not supported")
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	return
}
