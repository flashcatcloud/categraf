package jolokia_agent

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/jolokia"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "jolokia_agent"

type JolokiaAgent struct {
	config.Interval
	counter   uint64
	waitgrp   sync.WaitGroup
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &JolokiaAgent{}
	})
}

func (r *JolokiaAgent) Prefix() string {
	return ""
}

func (r *JolokiaAgent) Init() error {
	if len(r.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(r.Instances); i++ {
		if err := r.Instances[i].Init(); err != nil {
			if !errors.Is(err, types.ErrInstancesEmpty) {
				return err
			}
		}
	}

	return nil
}

func (r *JolokiaAgent) Drop() {}

func (r *JolokiaAgent) Gather(slist *list.SafeList) {
	atomic.AddUint64(&r.counter, 1)

	for i := range r.Instances {
		ins := r.Instances[i]

		if len(ins.URLs) == 0 {
			continue
		}

		r.waitgrp.Add(1)
		go func(slist *list.SafeList, ins *Instance) {
			defer r.waitgrp.Done()

			if ins.IntervalTimes > 0 {
				counter := atomic.LoadUint64(&r.counter)
				if counter%uint64(ins.IntervalTimes) != 0 {
					return
				}
			}

			ins.gatherOnce(slist)
		}(slist, ins)
	}

	r.waitgrp.Wait()
}

type Instance struct {
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

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
		return nil
	}

	if ins.DefaultFieldSeparator == "" {
		ins.DefaultFieldSeparator = "_"
	}

	return nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) {
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
