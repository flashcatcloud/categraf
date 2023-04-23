package jolokia_agent

import (
	"fmt"
	"log"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/jolokia"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "jolokia_agent"

type JolokiaAgent struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &JolokiaAgent{}
	})
}

func (r *JolokiaAgent) Clone() inputs.Input {
	return &JolokiaAgent{}
}

func (r *JolokiaAgent) Name() string {
	return inputName
}

func (r *JolokiaAgent) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	URLs            []string               `toml:"urls"`
	Username        string                 `toml:"username"`
	Password        string                 `toml:"password"`
	ResponseTimeout config.Duration        `toml:"response_timeout"`
	Metrics         []jolokia.MetricConfig `toml:"metric"`

	DefaultTagPrefix      string `toml:"default_tag_prefix"`
	DefaultFieldPrefix    string `toml:"default_field_prefix"`
	DefaultFieldSeparator string `toml:"default_field_separator"`

	tls.ClientConfig
	clients  []*jolokia.Client
	gatherer *jolokia.Gatherer
}

func (ins *Instance) Init() error {
	if len(ins.URLs) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.DefaultFieldSeparator == "" {
		ins.DefaultFieldSeparator = "_"
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if ins.gatherer == nil {
		ins.gatherer = jolokia.NewGatherer(ins.createMetrics())
	}

	if ins.clients == nil {
		ins.clients = make([]*jolokia.Client, 0, len(ins.URLs))
		for _, url := range ins.URLs {
			client, err := ins.createClient(url)
			if err != nil {
				log.Println("E! failed to create client:", err)
				continue
			}
			ins.clients = append(ins.clients, client)
		}
	}

	var wg sync.WaitGroup

	for _, client := range ins.clients {
		wg.Add(1)
		go func(client *jolokia.Client) {
			defer wg.Done()

			err := ins.gatherer.Gather(client, slist)
			if err != nil {
				log.Println("E!", fmt.Errorf("unable to gather metrics for %s: %v", client.URL, err))
			}
		}(client)
	}

	wg.Wait()
}

func (ins *Instance) createMetrics() []jolokia.Metric {
	var metrics []jolokia.Metric

	for _, metricConfig := range ins.Metrics {
		metrics = append(metrics, jolokia.NewMetric(metricConfig,
			ins.DefaultFieldPrefix, ins.DefaultFieldSeparator, ins.DefaultTagPrefix))
	}

	return metrics
}

func (ins *Instance) createClient(url string) (*jolokia.Client, error) {
	return jolokia.NewClient(url, &jolokia.ClientConfig{
		Username:        ins.Username,
		Password:        ins.Password,
		ResponseTimeout: time.Duration(ins.ResponseTimeout),
		ClientConfig:    ins.ClientConfig,
	})
}
