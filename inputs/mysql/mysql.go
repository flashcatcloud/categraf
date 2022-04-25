package mysql

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "mysql"

type Instance struct {
	Address       string            `toml:"address"`
	Username      string            `toml:"username"`
	Password      string            `toml:"password"`
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	tls.ClientConfig
}

func (ins *Instance) Init() error {
	if ins.Address == "" {
		return errors.New("address is blank")
	}
	return nil
}

type MySQL struct {
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &MySQL{}
	})
}

func (m *MySQL) GetInputName() string {
	return inputName
}

func (m *MySQL) GetInterval() config.Duration {
	return m.Interval
}

func (m *MySQL) Init() error {
	if len(m.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(m.Instances); i++ {
		if err := m.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (m *MySQL) Drop() {}

func (m *MySQL) Gather() (samples []*types.Sample) {
	atomic.AddUint64(&m.Counter, 1)

	slist := list.NewSafeList()

	for i := range m.Instances {
		ins := m.Instances[i]
		m.wg.Add(1)
		go m.gatherOnce(slist, ins)
	}
	m.wg.Wait()

	interfaceList := slist.PopBackAll()
	for i := 0; i < len(interfaceList); i++ {
		samples = append(samples, interfaceList[i].(*types.Sample))
	}

	return
}

func (m *MySQL) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer m.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&m.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	tags := map[string]string{"address": ins.Address}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("scrape_use_seconds", use, tags))
	}(begun)

	//
}
