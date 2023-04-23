package haproxy

import (
	"fmt"
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "haproxy"

type HAProxy struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &HAProxy{}
	})
}

func (r *HAProxy) Clone() inputs.Input {
	return &HAProxy{}
}

func (r *HAProxy) Name() string {
	return inputName
}

func (r *HAProxy) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	// URI on which to scrape HAProxy.
	URI string `toml:"uri"`

	// Flag that enables SSL certificate verification for the scrape URI
	SSLVerify bool `toml:"ssl_verify"`

	// Comma-separated list of exported server metrics. See http://cbonte.github.io/haproxy-dconv/configuration-1.5.html#9.1
	ServerMetricFields string `toml:"server_metric_fields"`

	// Comma-separated list of exported server states to exclude. See https://cbonte.github.io/haproxy-dconv/1.8/management.html#9.1, field 17 status
	ServerExcludeStates string `toml:"server_exclude_states"`

	// Timeout for trying to get stats from HAProxy.
	Timeout config.Duration `toml:"timeout"`

	// Flag that enables using HTTP proxy settings from environment variables ($http_proxy, $https_proxy, $no_proxy)
	HTTPProxyFromEnv bool `toml:"proxy_from_env"`

	e *Exporter `toml:"-"`
}

func (ins *Instance) Init() error {
	if len(ins.URI) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.ServerMetricFields == "" {
		ins.ServerMetricFields = serverMetrics.String()
	}

	if ins.ServerExcludeStates == "" {
		ins.ServerExcludeStates = excludedServerStates
	}

	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(time.Duration(5) * time.Second)
	}

	selectedServerMetrics, err := filterServerMetrics(ins.ServerMetricFields)
	if err != nil {
		return fmt.Errorf("failed to filtering server metrics: %s", err)
	}

	e, err := NewExporter(
		ins.URI,
		ins.SSLVerify,
		ins.HTTPProxyFromEnv,
		selectedServerMetrics,
		ins.ServerExcludeStates,
		time.Duration(ins.Timeout),
	)

	if err != nil {
		return fmt.Errorf("could not instantiate haproxy exporter: %s", err)
	}

	ins.e = e
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	defer func(begun time.Time) {
		slist.PushSample(inputName, "scrape_use_seconds", time.Since(begun).Seconds())
	}(time.Now())

	err := inputs.Collect(ins.e, slist)
	if err != nil {
		log.Println("E! failed to collect metrics:", err)
	}
}
