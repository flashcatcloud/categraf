package jolokia_proxy

import (
	"fmt"
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/jolokia"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "jolokia_proxy"

type JolokiaProxy struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &JolokiaProxy{}
	})
}

func (r *JolokiaProxy) Clone() inputs.Input {
	return &JolokiaProxy{}
}

func (r *JolokiaProxy) Name() string {
	return inputName
}

func (r *JolokiaProxy) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type JolokiaProxyTargetConfig struct {
	URL      string `toml:"url"`
	Username string `toml:"username"`
	Password string `toml:"password"`
}

type Instance struct {
	config.InstanceConfig

	URL             string          `toml:"url"`
	Username        string          `toml:"username"`
	Password        string          `toml:"password"`
	ResponseTimeout config.Duration `toml:"response_timeout"`

	DefaultTargetUsername string                     `toml:"default_target_username"`
	DefaultTargetPassword string                     `toml:"default_target_password"`
	Targets               []JolokiaProxyTargetConfig `toml:"target"`
	Metrics               []jolokia.MetricConfig     `toml:"metric"`

	DefaultTagPrefix      string `toml:"default_tag_prefix"`
	DefaultFieldPrefix    string `toml:"default_field_prefix"`
	DefaultFieldSeparator string `toml:"default_field_separator"`

	tls.ClientConfig
	client   *jolokia.Client
	gatherer *jolokia.Gatherer
}

func (ins *Instance) Init() error {
	if len(ins.URL) == 0 {
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

	if ins.client == nil {
		client, err := ins.createClient(ins.URL)
		if err != nil {
			log.Println("E! failed to create client:", err)
			return
		}
		ins.client = client
	}

	err := ins.gatherer.Gather(ins.client, slist)
	if err != nil {
		log.Println("E!", fmt.Errorf("unable to gather metrics for %s: %v", ins.client.URL, err))
	}
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
	proxyConfig := &jolokia.ProxyConfig{
		DefaultTargetUsername: ins.DefaultTargetUsername,
		DefaultTargetPassword: ins.DefaultTargetPassword,
	}

	for _, target := range ins.Targets {
		proxyConfig.Targets = append(proxyConfig.Targets, jolokia.ProxyTargetConfig{
			URL:      target.URL,
			Username: target.Username,
			Password: target.Password,
		})
	}

	return jolokia.NewClient(url, &jolokia.ClientConfig{
		Username:        ins.Username,
		Password:        ins.Password,
		ResponseTimeout: time.Duration(ins.ResponseTimeout),
		ClientConfig:    ins.ClientConfig,
		ProxyConfig:     proxyConfig,
	})
}
