package consul

import (
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"

	"github.com/hashicorp/consul/api"
)

const inputName = "consul"

type Instance struct {
	Address           string `toml:"address"`
	Scheme            string `toml:"scheme"`
	Token             string `toml:"token"`
	Username          string `toml:"username"`
	Password          string `toml:"password"`
	Datacenter        string `toml:"datacenter"`
	AllowStale        *bool  `toml:"allow_stale"`
	RequireConsistent *bool  `toml:"require_consistent"`
	KVPrefix          string `toml:"kv_prefix"`
	KVFilter          string `toml:"kv_filter"`
	tls.ClientConfig

	// client used to connect to Consul agnet
	client *api.Client

	config.InstanceConfig
}

func (ins *Instance) Init() error {
	conf := api.DefaultConfig()

	if ins.Address == "" {
		return types.ErrInstancesEmpty
	}
	conf.Address = ins.Address

	if ins.Scheme != "" {
		conf.Scheme = ins.Scheme
	}

	if ins.Datacenter != "" {
		conf.Datacenter = ins.Datacenter
	}

	if ins.AllowStale == nil {
		flag := true
		ins.AllowStale = &flag
	}

	if ins.RequireConsistent == nil {
		flag := false
		ins.RequireConsistent = &flag
	}

	if ins.KVFilter == "" {
		ins.KVFilter = ".*"
	}

	if ins.Token != "" {
		conf.Token = ins.Token
	}

	if ins.Username != "" {
		conf.HttpAuth = &api.HttpBasicAuth{
			Username: ins.Username,
			Password: ins.Password,
		}
	}

	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return err
	}

	conf.Transport = &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	client, err := api.NewClient(conf)
	if err != nil {
		return err
	}
	ins.client = client

	return nil
}

type Consul struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Consul{}
	})
}

func (c *Consul) Clone() inputs.Input {
	return &Consul{}
}

func (c *Consul) Name() string {
	return inputName
}

func (c *Consul) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(c.Instances))
	for i := 0; i < len(c.Instances); i++ {
		ret[i] = c.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	tag := ins.DefaultTags()
	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample(inputName, "scrape_use_seconds", use, tag))
	}(begun)

	fns := []func(*types.SampleList) error{
		ins.collectHealthCheckMetric,
		ins.collectAgentMetric,
		ins.collectPeersMetric,
		ins.collectLeaderMetric,
		ins.collectNodesMetric,
		ins.collectMembersMetric,
		ins.collectMembersWanMetric,
		ins.collectServicesMetric,
		ins.collectKeyValues,
	}

	up := 1
	for _, fn := range fns {
		if err := fn(slist); err != nil {
			up = 0
			log.Println("E! failed to gather http target:", ins.Address, "error:", err)
		}
	}
	slist.PushFront(types.NewSample(inputName, "up", up, tag))
}

func (ins *Instance) DefaultTags() map[string]string {
	return map[string]string{"address": ins.Address}
}

func (ins *Instance) collectHealthCheckMetric(slist *types.SampleList) error {
	checks, _, err := ins.client.Health().State("any", nil)
	if err != nil {
		return err
	}

	for _, check := range checks {
		tags := make(map[string]string)
		tags["check_id"] = check.CheckID
		tags["check_name"] = check.Name
		tags["node"] = check.Node
		copyTags(tags, ins.DefaultTags())

		var passing, warning, critical, maintenance float64
		switch check.Status {
		case api.HealthPassing:
			passing = 1
			tags["status"] = api.HealthPassing
		case api.HealthWarning:
			warning = 1
			tags["status"] = api.HealthWarning
		case api.HealthCritical:
			critical = 1
			tags["status"] = api.HealthCritical
		case api.HealthMaint:
			maintenance = 1
			tags["status"] = api.HealthMaint
		}

		if check.ServiceID == "" {
			slist.PushFront(types.NewSample(inputName, "health_node_status", passing, tags))
			slist.PushFront(types.NewSample(inputName, "health_node_status", warning, tags))
			slist.PushFront(types.NewSample(inputName, "health_node_status", critical, tags))
			slist.PushFront(types.NewSample(inputName, "health_node_status", maintenance, tags))
		} else {
			tags["service_id"] = check.ServiceID
			tags["service_name"] = check.ServiceName
			slist.PushFront(types.NewSample(inputName, "health_service_status", passing, tags))
			slist.PushFront(types.NewSample(inputName, "health_service_status", warning, tags))
			slist.PushFront(types.NewSample(inputName, "health_service_status", critical, tags))
			slist.PushFront(types.NewSample(inputName, "health_service_status", maintenance, tags))
			slist.PushFront(types.NewSample(inputName, "service_checks", 1, tags))
		}

		delete(tags, "status")

		set := make(map[string]struct{})
		for _, t := range check.ServiceTags {
			if _, ok := set[t]; ok {
				continue
			}
			slist.PushFront(types.NewSample(inputName, "service_tag", 1, tags, map[string]string{"tag": t}))
			set[t] = struct{}{}
		}
	}
	return nil
}

func (ins *Instance) collectAgentMetric(slist *types.SampleList) error {
	agentInfo, err := ins.client.Agent().Metrics()
	if err != nil {
		return err
	}

	tags := make(map[string]string)
	copyTags(tags, ins.DefaultTags())
	for _, counter := range agentInfo.Counters {
		name := strings.NewReplacer(".", "_", "-", "_").Replace(counter.Name)
		slist.PushFront(types.NewSample(name, "count", counter.Count, tags))
		slist.PushFront(types.NewSample(name, "sum", counter.Sum, tags))
		slist.PushFront(types.NewSample(name, "max", counter.Max, tags))
		slist.PushFront(types.NewSample(name, "mean", counter.Mean, tags))
		slist.PushFront(types.NewSample(name, "min", counter.Min, tags))
		slist.PushFront(types.NewSample(name, "stddev", counter.Stddev, tags))
	}

	for _, gauge := range agentInfo.Gauges {
		name := strings.NewReplacer(".", "_", "-", "_").Replace(gauge.Name)
		slist.PushFront(types.NewSample("", name, gauge.Value, tags))
	}

	for _, point := range agentInfo.Points {
		name := strings.NewReplacer(".", "_", "-", "_").Replace(point.Name)
		slist.PushFront(types.NewSample("", name, point.Points, tags))
	}

	for _, sample := range agentInfo.Samples {
		name := strings.NewReplacer(".", "_", "-", "_").Replace(sample.Name)
		slist.PushFront(types.NewSample(name, "count", sample.Count, tags))
		slist.PushFront(types.NewSample(name, "sum", sample.Sum, tags))
		slist.PushFront(types.NewSample(name, "max", sample.Max, tags))
		slist.PushFront(types.NewSample(name, "mean", sample.Mean, tags))
		slist.PushFront(types.NewSample(name, "min", sample.Min, tags))
		slist.PushFront(types.NewSample(name, "stddev", sample.Stddev, tags))
	}

	return nil
}

func (ins *Instance) collectPeersMetric(slist *types.SampleList) error {
	peers, err := ins.client.Status().Peers()
	if err != nil {
		return err
	}

	slist.PushFront(types.NewSample(inputName, "raft_peers", len(peers), ins.DefaultTags()))
	return nil
}

func (ins *Instance) collectLeaderMetric(slist *types.SampleList) error {
	leader, err := ins.client.Status().Leader()
	if err != nil {
		return err
	}
	if len(leader) == 0 {
		slist.PushFront(types.NewSample(inputName, "raft_leader", 0, ins.DefaultTags()))
	} else {
		slist.PushFront(types.NewSample(inputName, "raft_leader", 1, ins.DefaultTags()))
	}
	return nil
}

func (ins *Instance) collectNodesMetric(slist *types.SampleList) error {
	nodes, _, err := ins.client.Catalog().Nodes(&api.QueryOptions{AllowStale: *ins.AllowStale, RequireConsistent: *ins.RequireConsistent})
	if err != nil {
		return err
	}
	slist.PushFront(types.NewSample(inputName, "serf_lan_members", len(nodes), ins.DefaultTags()))
	return nil
}

func (ins *Instance) collectMembersMetric(slist *types.SampleList) error {
	members, err := ins.client.Agent().Members(false)
	if err != nil {
		return err
	}
	for _, entry := range members {
		tags := ins.DefaultTags()
		tags["member"] = entry.Name
		slist.PushFront(types.NewSample(inputName, "serf_lan_member_status", entry.Status, tags))
	}
	return nil
}

func (ins *Instance) collectMembersWanMetric(slist *types.SampleList) error {
	members, err := ins.client.Agent().Members(true)
	if err != nil {
		return err
	}
	for _, entry := range members {
		tags := ins.DefaultTags()
		tags["member"] = entry.Name
		tags["dc"] = entry.Tags["dc"]
		slist.PushFront(types.NewSample(inputName, "serf_wan_member_status", entry.Status, tags))
	}
	return nil
}

func (ins *Instance) collectServicesMetric(slist *types.SampleList) error {
	serviceNames, _, err := ins.client.Catalog().Services(nil)
	if err != nil {
		return err
	}
	slist.PushFront(types.NewSample(inputName, "catalog_services", len(serviceNames), ins.DefaultTags()))
	return nil
}

func (ins *Instance) collectKeyValues(slist *types.SampleList) error {
	if ins.KVPrefix == "" {
		return nil
	}

	kv := ins.client.KV()
	pairs, _, err := kv.List(ins.KVPrefix, nil)
	if err != nil {
		return err
	}

	for _, pair := range pairs {
		tags := ins.DefaultTags()
		tags["key"] = pair.Key
		if regexp.MustCompile(ins.KVFilter).MatchString(pair.Key) {
			if val, err := strconv.ParseFloat(string(pair.Value), 64); err == nil {
				slist.PushFront(types.NewSample(inputName, "catalog_kv", val, tags))
			}
		}
	}
	return nil
}

func copyTags(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}
