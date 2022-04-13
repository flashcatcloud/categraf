package redis

import (
	"log"
	"time"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

var (
	DefaultInterval = time.Second * 30
	DefaultTimeout  = time.Second * 20
)

type Target struct {
	IntervalMS int64
	TimeoutMS  int64
	Address    string
	Password   string
	quit       chan struct{}
	Labels     map[string]string
}

type Redis struct {
	ConfigBytes []byte

	IntervalMS int64
	TimeoutMS  int64
	Targets    []*Target
	Labels     map[string]string
}

// overwrite func
func (r *Redis) Description() string {
	return "Read metrics from one or many redis servers"
}

func (r *Redis) GetPointer() interface{} {
	return r
}

// overwrite func
func (r *Redis) TidyConfig() error {
	log.Printf("-------%#v", r)

	if len(r.Targets) == 0 {
		log.Println("I! [redis] Targets is empty")
	}

	return nil
}

// overwrite func
func (r *Redis) StopGoroutines() {
	for i := 0; i < len(r.Targets); i++ {
		close(r.Targets[i].quit)
	}
}

// overwrite func
func (r *Redis) StartGoroutines(queue chan *types.Sample) {
	for i := 0; i < len(r.Targets); i++ {
		r.Targets[i].quit = make(chan struct{})
		go r.Targets[i].LoopGather(r, queue)
	}
}

func (t *Target) getInterval(r *Redis) time.Duration {
	if t.IntervalMS != 0 {
		return time.Duration(t.IntervalMS) * time.Millisecond
	}

	if r.IntervalMS != 0 {
		return time.Duration(r.IntervalMS) * time.Millisecond
	}

	return DefaultInterval
}

func (t *Target) LoopGather(r *Redis, queue chan *types.Sample) {
	for {
		select {
		case <-t.quit:
			return
		default:
			time.Sleep(t.getInterval(r))
			t.Gather(r, queue)
		}
	}
}

func (t *Target) Gather(r *Redis, queue chan *types.Sample) {
	queue <- &types.Sample{
		Metric:    "------test_metric-----",
		Timestamp: time.Now().Unix(),
		Value:     time.Now().Unix(),
		Labels:    map[string]string{"region": "bejing"},
	}
}

func init() {
	inputs.Add("redis", func() inputs.Input {
		return &Redis{}
	})
}
