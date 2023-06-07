package consul

import (
	"log"
	"net/http"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"

	"github.com/hashicorp/consul/api"
)

const inputName = "consul"

type Instance struct {
	Address    string `toml:"address"`
	Scheme     string `toml:"scheme"`
	Token      string `toml:"token"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
	Datacenter string `toml:"datacenter"`
	tls.ClientConfig

	// client used to connect to Consul agnet
	client *api.Client

	config.InstanceConfig
}

func (ins *Instance) Init() error {
	conf := api.DefaultConfig()

	if ins.Address != "" {
		conf.Address = ins.Address
	}

	if ins.Scheme != "" {
		conf.Scheme = ins.Scheme
	}

	if ins.Datacenter != "" {
		conf.Datacenter = ins.Datacenter
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
	tag := map[string]string{"address": ins.Address}
	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample(inputName, "scrape_use_seconds", use, tag))
	}(begun)

	checks, _, err := ins.client.Health().State("any", nil)
	if err != nil {
		slist.PushFront(types.NewSample(inputName, "up", 0, tag))
		log.Println("E! failed to gather http target:", ins.Address, "error:", err)
		return
	}
	slist.PushFront(types.NewSample(inputName, "up", 1, tag))

	for _, check := range checks {
		tags := make(map[string]string)
		for k, v := range tag {
			tags[k] = v
		}
		tags["check_id"] = check.CheckID
		tags["check_name"] = check.Name
		tags["node"] = check.Node

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
		}

		delete(tags, "status")

		set := make(map[string]struct{})
		for _, t := range check.ServiceTags {
			if _, ok := set[t]; ok {
				continue
			}
			slist.PushFront(types.NewSample(inputName, "health_checks_service_tag", 1, tags, map[string]string{"tag": t}))
			set[t] = struct{}{}
		}
	}
}
