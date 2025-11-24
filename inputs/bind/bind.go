package bind

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const (
	inputName = "bind"
)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Bind{}
	})
}

type (
	Bind struct {
		config.PluginConfig
		Instances []*Instance `toml:"instances"`
	}

	Instance struct {
		config.InstanceConfig

		Urls                 []string        `toml:"urls"`
		GatherMemoryContexts bool            `toml:"gather_memory_contexts"`
		GatherViews          bool            `toml:"gather_views"`
		Timeout              config.Duration `toml:"timeout"`

		client http.Client
	}
)

func (b *Bind) Name() string {
	return inputName
}

func (b *Bind) Clone() inputs.Input {
	return &Bind{}
}

func (b *Bind) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(b.Instances))
	for i := 0; i < len(b.Instances); i++ {
		ret[i] = b.Instances[i]
	}
	return ret
}

var _ inputs.SampleGatherer = new(Instance)
var _ inputs.Input = new(Bind)
var _ inputs.InstancesGetter = new(Bind)

func (b *Instance) Init() error {
	if len(b.Urls) == 0 {
		return types.ErrInstancesEmpty
	}

	b.client = http.Client{
		Timeout: time.Duration(b.Timeout),
	}

	return nil
}

func (b *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup

	for _, u := range b.Urls {
		addr, err := url.Parse(u)
		if err != nil {
			log.Printf("unable to parse address %q: %s", u, err)
			continue
		}

		wg.Add(1)
		go func(addr *url.URL) {
			defer wg.Done()
			err = b.gatherURL(addr, slist)
			if err != nil {
				log.Printf("E! gather url:%s error:%s", addr, err)
			}
		}(addr)
	}

	wg.Wait()
}

func (b *Instance) gatherURL(addr *url.URL, slist *types.SampleList) error {
	switch addr.Path {
	case "":
		// BIND 9.6 - 9.8
		return b.readStatsXMLv2(addr, slist)
	case "/json/v1":
		// BIND 9.10+
		return b.readStatsJSON(addr, slist)
	case "/xml/v2":
		// BIND 9.9
		return b.readStatsXMLv2(addr, slist)
	case "/xml/v3":
		// BIND 9.9+
		return b.readStatsXMLv3(addr, slist)
	default:
		return fmt.Errorf("provided URL %s is ambiguous, please check plugin documentation for supported URL formats",
			addr)
	}
}
