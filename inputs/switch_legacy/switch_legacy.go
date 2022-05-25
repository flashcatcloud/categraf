package switch_legacy

import (
	"fmt"
	"sync"
	"sync/atomic"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "switch_legacy"

type Switch struct {
	config.Interval
	counter       uint64
	waitgrp       sync.WaitGroup
	Instances     []*Instance       `toml:"instances"`
	SwitchIdLabel string            `toml:"switch_id_label"`
	Mappings      map[string]string `toml:"mappings"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Switch{}
	})
}

func (s *Switch) Prefix() string {
	return inputName
}

func (s *Switch) Init() error {
	if len(s.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(s.Instances); i++ {
		if err := s.Instances[i].Init(); err != nil {
			return err
		} else {
			s.Instances[i].parent = s
		}
	}

	for k, v := range s.Mappings {
		fmt.Println(k, v)
	}

	return nil
}

func (s *Switch) Drop() {}

func (s *Switch) Gather(slist *list.SafeList) {
	atomic.AddUint64(&s.counter, 1)

	for i := range s.Instances {
		ins := s.Instances[i]

		s.waitgrp.Add(1)
		go func(slist *list.SafeList, ins *Instance) {
			defer s.waitgrp.Done()

			if ins.IntervalTimes > 0 {
				counter := atomic.LoadUint64(&s.counter)
				if counter%uint64(ins.IntervalTimes) != 0 {
					return
				}
			}

			ins.gatherOnce(slist)
		}(slist, ins)
	}

	s.waitgrp.Wait()
}

type Instance struct {
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`
	IPs           []string          `toml:"ips"`
	Community     string            `toml:"community"`
	UseGosnmp     bool              `toml:"use_gosnmp"`
	IndexTag      bool              `toml:"index_tag"`
	PingTimeoutMs int64             `toml:"ping_timeout_ms"`
	PingRetries   int               `toml:"ping_retries"`

	parent *Switch
}

func (ins *Instance) Init() error {
	return nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) error {
	return nil
}
