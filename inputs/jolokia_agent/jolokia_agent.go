package jolokia_agent

import (
	"errors"
	"sync"
	"sync/atomic"

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

	tls.ClientConfig
	clients []*jolokia.Client
}

func (ins *Instance) Init() error {
	if len(ins.URLs) == 0 {
		return nil
	}

	return nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) {

}
