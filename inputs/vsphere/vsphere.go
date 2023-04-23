package vsphere

import (
	"context"
	"log"
	"time"

	"github.com/vmware/govmomi/vim25/soap"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "vsphere"

// VSphere is the top level type for the vSphere input plugin. It contains all the configuration
// and a list of connected vSphere endpoints
type VSphere struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &VSphere{}
	})
}

func (vs *VSphere) Clone() inputs.Input {
	return &VSphere{}
}

func (vs *VSphere) Name() string {
	return inputName
}

func (pt *VSphere) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	Vcenter  string `toml:"vcenter"`
	Username string `toml:"username"`
	Password string `toml:"password"`

	DatacenterInstances       bool     `toml:"datacenter_instances"`
	DatacenterMetricInclude   []string `toml:"datacenter_metric_include"`
	DatacenterMetricExclude   []string `toml:"datacenter_metric_exclude"`
	DatacenterInclude         []string `toml:"datacenter_include"`
	DatacenterExclude         []string `toml:"datacenter_exclude"`
	ClusterInstances          bool     `toml:"cluster_instances"`
	ClusterMetricInclude      []string `toml:"cluster_metric_include"`
	ClusterMetricExclude      []string `toml:"cluster_metric_exclude"`
	ClusterInclude            []string `toml:"cluster_include"`
	ClusterExclude            []string `toml:"cluster_exclude"`
	ResourcePoolInstances     bool     `toml:"resoucepool_instances"`
	ResourcePoolMetricInclude []string `toml:"resoucepool_metric_include"`
	ResourcePoolMetricExclude []string `toml:"resoucepool_metric_exclude"`
	ResourcePoolInclude       []string `toml:"resoucepool_include"`
	ResourcePoolExclude       []string `toml:"resoucepool_exclude"`
	HostInstances             bool     `toml:"host_instances"`
	HostMetricInclude         []string `toml:"host_metric_include"`
	HostMetricExclude         []string `toml:"host_metric_exclude"`
	HostInclude               []string `toml:"host_include"`
	HostExclude               []string `toml:"host_exclude"`
	VMInstances               bool     `toml:"vm_instances"`
	VMMetricInclude           []string `toml:"vm_metric_include"`
	VMMetricExclude           []string `toml:"vm_metric_exclude"`
	VMInclude                 []string `toml:"vm_include"`
	VMExclude                 []string `toml:"vm_exclude"`
	DatastoreInstances        bool     `toml:"datastore_instances"`
	DatastoreMetricInclude    []string `toml:"datastore_metric_include"`
	DatastoreMetricExclude    []string `toml:"datastore_metric_exclude"`
	DatastoreInclude          []string `toml:"datastore_include"`
	DatastoreExclude          []string `toml:"datastore_exclude"`

	Separator               string          `toml:"separator"`
	CustomAttributeInclude  []string        `toml:"custom_attribute_include"`
	CustomAttributeExclude  []string        `toml:"custom_attribute_exclude"`
	UseIntSamples           bool            `toml:"use_int_samples"`
	IPAddresses             []string        `toml:"ip_addresses"`
	MetricLookback          int             `toml:"metric_lookback"`
	MaxQueryObjects         int             `toml:"max_query_objects"`
	MaxQueryMetrics         int             `toml:"max_query_metrics"`
	CollectConcurrency      int             `toml:"collect_concurrency"`
	DiscoverConcurrency     int             `toml:"discover_concurrency"`
	ObjectDiscoveryInterval config.Duration `toml:"object_discovery_interval"`
	HistoricalInterval      config.Duration `toml:"historical_interval"`
	Timeout                 config.Duration `toml:"timeout"`
	tls.ClientConfig                        // Mix in the TLS/SSL goodness from core

	endpoints *Endpoint
	cancel    context.CancelFunc
}

func (ins *Instance) Init() error {
	if ins.Vcenter == "" {
		return types.ErrInstancesEmpty
	}
	if ins.DatacenterInclude == nil {
		ins.DatacenterInclude = []string{"/*"}
	}
	if ins.ClusterInclude == nil {
		ins.ClusterInclude = []string{"/*/host/**"}
	}

	if ins.HostInclude == nil {
		ins.HostInclude = []string{"/*/host/**"}
	}

	if ins.ResourcePoolInclude == nil {
		ins.ResourcePoolInclude = []string{"/*/host/**"}
	}

	if ins.VMInclude == nil {
		ins.VMInclude = []string{"/*/vm/**"}
	}

	if ins.DatastoreInclude == nil {
		ins.DatastoreInclude = []string{"/*/datastore/**"}
	}

	if ins.Separator == "" {
		ins.Separator = "_"
	}
	if ins.CustomAttributeInclude == nil {
		ins.CustomAttributeInclude = []string{}
	}
	if ins.CustomAttributeExclude == nil {
		ins.CustomAttributeExclude = []string{"*"}
	}
	if ins.IPAddresses == nil {
		ins.IPAddresses = []string{"ipv4"}
	}

	if ins.MaxQueryObjects <= 0 {
		ins.MaxQueryObjects = 256
	}
	if ins.MaxQueryMetrics <= 0 {
		ins.MaxQueryMetrics = 256
	}

	if ins.CollectConcurrency <= 0 {
		ins.CollectConcurrency = 1
	}
	if ins.DiscoverConcurrency <= 0 {
		ins.DiscoverConcurrency = 1
	}
	if ins.MetricLookback <= 0 {
		ins.MetricLookback = 3
	}
	if ins.ObjectDiscoveryInterval == 0 {
		ins.ObjectDiscoveryInterval = config.Duration(time.Second * 300)
	}
	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(time.Second * 60)
	}
	if ins.HistoricalInterval == 0 {
		ins.HistoricalInterval = config.Duration(time.Second * 300)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ins.cancel = cancel

	// Create endpoints, one for each vCenter we're monitoring
	u, err := soap.ParseURL(ins.Vcenter)
	if err != nil {
		log.Println("E! soap.ParseURL", err)
		return err
	}
	ep, err := NewEndpoint(ctx, ins, u)
	if err != nil {
		log.Println("E! NewEndpoint", err)
		return err
	}
	ins.endpoints = ep
	return nil
}

func (v *Instance) Drop() {
	log.Printf("I! Stopping plugin")
	v.cancel()

	// Wait for all endpoints to finish. No need to wait for
	// Gather() to finish here, since it Stop() will only be called
	// after the last Gather() has finished. We do, however, need to
	// wait for any discovery to complete by trying to grab the
	// "busy" mutex.
	if config.Config.DebugMode {
		log.Printf("D! Waiting for endpoint %q to finish", v.endpoints.URL.Host)
	}
	func() {
		v.endpoints.busy.Lock() // Wait until discovery is finished
		defer v.endpoints.busy.Unlock()
		v.endpoints.Close()
	}()
}

// Gather is the main data collection function called by the Telegraf core. It performs all
// the data collection and writes all metrics into the Accumulator passed as an argument.
func (ins *Instance) Gather(slist *types.SampleList) {
	err := ins.endpoints.Collect(context.Background(), slist)
	if err == context.Canceled {
		// No need to signal errors if we were merely canceled.
		err = nil
	}
	if err != nil {
		// acc.AddError(err)
		log.Printf("E! fail to gather\n", err)
	}

}
