package redis

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

const inputName = "redis"

type Instance struct {
	Address       string            `toml:"address"`
	Username      string            `toml:"username"`
	Password      string            `toml:"password"`
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	tls.ClientConfig
}

type Redis struct {
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Redis{}
	})
}

func (r *Redis) GetInputName() string {
	return inputName
}

func (r *Redis) GetInterval() config.Duration {
	return r.Interval
}

func (r *Redis) Init() error {
	if len(r.Instances) == 0 {
		return errors.New("instances empty")
	}

	return nil
}

func (r *Redis) Drop() {

}

func (r *Redis) Gather() (samples []*types.Sample) {
	atomic.AddUint64(&r.Counter, 1)

	slist := list.NewSafeList()

	for i := range r.Instances {
		ins := r.Instances[i]
		r.wg.Add(1)
		go r.gatherOnce(slist, ins)
	}
	r.wg.Wait()

	interfaceList := slist.PopBackAll()
	for i := 0; i < len(interfaceList); i++ {
		samples = append(samples, interfaceList[i].(*types.Sample))
	}

	return
}

func (r *Redis) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer r.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&r.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	tags := map[string]string{"address": ins.Address}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("scrape_use_seconds", use, tags))
	}(time.Now())

	// get redis connection

}
