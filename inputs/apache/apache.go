package apache

import (
	"fmt"
	"log"

	"github.com/prometheus/common/promlog"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/apache/exporter"
	"flashcat.cloud/categraf/types"
)

const inputName = "apache"

type Apache struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type Instance struct {
	config.InstanceConfig
	LogLevel string `toml:"log_level"`
	exporter.Config

	e *exporter.Exporter
}

var _ inputs.Input = new(Apache)
var _ inputs.SampleGatherer = new(Instance)
var _ inputs.InstancesGetter = new(Apache)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Apache{}
	})
}

func (a *Apache) Clone() inputs.Input {
	return &Apache{}
}

func (a *Apache) Name() string {
	return inputName
}

func (a *Apache) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(a.Instances))
	for i := 0; i < len(a.Instances); i++ {
		ret[i] = a.Instances[i]
	}
	return ret
}

func (a *Apache) Drop() {

	for _, i := range a.Instances {
		if i == nil {
			continue
		}

		if i.e != nil {
			i.e.Close()
		}
	}
}

func (ins *Instance) Init() error {
	if len(ins.ScrapeURI) == 0 {
		return types.ErrInstancesEmpty
	}

	if len(ins.LogLevel) == 0 {
		ins.LogLevel = "info"
	}
	promlogConfig := &promlog.Config{
		Level: &promlog.AllowedLevel{},
	}
	promlogConfig.Level.Set(ins.LogLevel)
	logger := promlog.New(promlogConfig)
	e, err := exporter.New(logger, &ins.Config)

	if err != nil {
		return fmt.Errorf("could not instantiate mongodb lag exporter: %v", err)
	}

	ins.e = e
	return nil

}

func (ins *Instance) Gather(slist *types.SampleList) {

	//  collect
	err := inputs.Collect(ins.e, slist)
	if err != nil {
		log.Println("E! failed to collect metrics:", err)
	}
}
