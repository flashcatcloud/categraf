package traffic_server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "traffic_server"

type TrafficServer struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &TrafficServer{}
	})
}

func (r *TrafficServer) Clone() inputs.Input {
	return &TrafficServer{}
}

func (r *TrafficServer) Name() string {
	return inputName
}

func (r *TrafficServer) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	Targets []string        `toml:"targets"`
	Method  string          `toml:"method"`
	Timeout config.Duration `toml:"timeout"`
	client  httpClient
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (ins *Instance) Init() error {
	if len(ins.Targets) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.Timeout < config.Duration(time.Second) {
		ins.Timeout = config.Duration(time.Second * 5)
	}

	if ins.Method == "" {
		ins.Method = "GET"
	}

	ins.client = &http.Client{
		Timeout: time.Duration(ins.Timeout),
	}

	for _, target := range ins.Targets {
		addr, err := url.Parse(target)
		if err != nil {
			return fmt.Errorf("failed to parse target url: %s, error: %v", target, err)
		}

		if addr.Scheme != "http" {
			return fmt.Errorf("only http are supported, target: %s", target)
		}
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	wg := new(sync.WaitGroup)
	for _, target := range ins.Targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			ins.gather(slist, target)
		}(target)
	}
	wg.Wait()
}

type Data struct {
	Global map[string]string `json:"global"`
}

func (ins *Instance) gather(slist *types.SampleList, target string) {
	if ins.DebugMod {
		log.Println("D! traffic_server... target:", target)
	}

	labels := map[string]string{"target": target}

	data := &Data{}

	err := ins.gatherJSONData(target, data)
	if err != nil {
		log.Println("E! failed to gather json data:", err)
		return
	}
	var fields = make(map[string]interface{})
	for key, value := range data.Global {
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		fields[strings.ReplaceAll(strings.ToLower(key), ".", "_")] = v
	}
	slist.PushSamples(inputName, fields, labels)

}

// gatherJSONData query the data source and parse the response JSON
func (ins *Instance) gatherJSONData(address string, value interface{}) error {
	request, err := http.NewRequest(ins.Method, address, nil)
	if err != nil {
		return err
	}

	response, err := ins.client.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// ignore the err here; LimitReader returns io.EOF and we're not interested in read errors.
		body, _ := io.ReadAll(io.LimitReader(response.Body, 200))
		return fmt.Errorf("%s returned HTTP status %s: %q", address, response.Status, body)
	}

	err = json.NewDecoder(response.Body).Decode(value)
	if err != nil {
		return err
	}

	return nil
}
