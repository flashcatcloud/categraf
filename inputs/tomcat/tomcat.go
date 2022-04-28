package tomcat

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

const inputName = "tomcat"

type Instance struct {
	URL           string            `toml:"url"`
	Username      string            `toml:"username"`
	Password      string            `toml:"password"`
	Timeout       config.Duration   `toml:"timeout"`
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	tls.ClientConfig
}

func (ins *Instance) Init() error {
	if ins.URL == "" {
		return errors.New("url is blank")
	}

	return nil
}

type Tomcat struct {
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Tomcat{}
	})
}

func (t *Tomcat) GetInputName() string {
	return inputName
}

func (t *Tomcat) GetInterval() config.Duration {
	return t.Interval
}

func (t *Tomcat) Init() error {
	if len(t.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(t.Instances); i++ {
		if err := t.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (t *Tomcat) Drop() {}

func (t *Tomcat) Gather(slist *list.SafeList) {
	atomic.AddUint64(&t.Counter, 1)
	for i := range t.Instances {
		ins := t.Instances[i]
		t.wg.Add(1)
		go t.gatherOnce(slist, ins)
	}
	t.wg.Wait()
}

func (t *Tomcat) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer t.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&t.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	tags := map[string]string{"url": ins.URL}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("scrape_use_seconds", use, tags))
	}(begun)

	// url cannot connect? up = 0

}
