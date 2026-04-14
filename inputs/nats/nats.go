package nats

import (
	"encoding/json"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	gnatsd "github.com/nats-io/nats-server/v2/server"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"k8s.io/klog/v2"
)

const inputName = "nats"

type Nats struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Nats{}
	})
}

func (n *Nats) Clone() inputs.Input {
	return &Nats{}
}

func (n *Nats) Name() string {
	return inputName
}

func (n *Nats) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(n.Instances))
	for i := 0; i < len(n.Instances); i++ {
		ret[i] = n.Instances[i]
	}
	return ret
}

type Instance struct {
	Server          string          `toml:"server"`
	ResponseTimeout config.Duration `toml:"response_timeout"`

	client *http.Client
	config.HTTPCommonConfig
	config.InstanceConfig
}

func (ins *Instance) Init() error {
	if ins.Server == "" {
		return types.ErrInstancesEmpty
	}
	if ins.ResponseTimeout <= 0 {
		ins.ResponseTimeout = config.Duration(time.Second * 5)
	}

	ins.InitHTTPClientConfig()

	var err error
	ins.client, err = ins.createHTTPClient()
	return err
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if ins.DebugMod {
		klog.V(1).InfoS("nats gather", "server", ins.Server)
	}
	address, err := url.Parse(ins.Server)
	if err != nil {
		klog.ErrorS(err, "error parsing NATS URL", "server", ins.Server)
		return
	}
	address.Path = path.Join(address.Path, "varz")

	resp, err := ins.client.Get(address.String())
	if err != nil {
		klog.ErrorS(err, "error while polling", "url", address.String())
		return
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.ErrorS(err, "error reading body", "url", address.String())
		return
	}

	stats := new(gnatsd.Varz)
	err = json.Unmarshal(bytes, &stats)
	if err != nil {
		klog.ErrorS(err, "error parsing response", "url", address.String())
		return
	}

	fields := map[string]interface{}{
		"in_msgs":           stats.InMsgs,
		"out_msgs":          stats.OutMsgs,
		"in_bytes":          stats.InBytes,
		"out_bytes":         stats.OutBytes,
		"uptime":            stats.Now.Sub(stats.Start).Nanoseconds(),
		"cores":             stats.Cores,
		"cpu":               stats.CPU,
		"mem":               stats.Mem,
		"connections":       stats.Connections,
		"total_connections": stats.TotalConnections,
		"subscriptions":     stats.Subscriptions,
		"slow_consumers":    stats.SlowConsumers,
		"routes":            stats.Routes,
		"remotes":           stats.Remotes,
	}
	tags := map[string]string{
		"server": ins.Server,
	}
	slist.PushSamples(inputName, fields, tags)
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tr := &http.Transport{
		ResponseHeaderTimeout: time.Duration(ins.ResponseTimeout),
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(ins.ResponseTimeout),
	}
	return client, nil
}
