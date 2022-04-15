package redis

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const InputName = "redis"

type Target struct {
	IntervalSeconds int64
	Labels          map[string]string

	quit chan struct{}

	Address  string
	Password string
}

type Redis struct {
	PrintConfigs    bool
	IntervalSeconds int64
	Labels          map[string]string

	Targets []*Target
}

// overwrite func
func (r *Redis) Init() error {
	if r.PrintConfigs {
		bs, _ := json.MarshalIndent(r, "", "    ")
		fmt.Println(string(bs))
	}
	return nil
}

// overwrite func
func (r *Redis) StopGoroutines() {
	for i := 0; i < len(r.Targets); i++ {
		r.Targets[i].quit <- struct{}{}
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
	if t.IntervalSeconds != 0 {
		return time.Duration(t.IntervalSeconds) * time.Second
	}

	if r.IntervalSeconds != 0 {
		return time.Duration(r.IntervalSeconds) * time.Second
	}

	return config.GetInterval()
}

func (t *Target) LoopGather(r *Redis, queue chan *types.Sample) {
	interval := t.getInterval(r)
	for {
		select {
		case <-t.quit:
			close(t.quit)
			return
		default:
			time.Sleep(interval)
			defer func() {
				if r := recover(); r != nil {
					if strings.Contains(fmt.Sprint(r), "closed channel") {
						return
					} else {
						log.Println("E! gather redis:", t.Address, " panic:", r)
					}
				}
			}()
			t.Gather(r, queue)
		}
	}
}

func (t *Target) Gather(r *Redis, queue chan *types.Sample) {
	// queue <- &types.Sample{
	// 	Metric:    InputName + "_categraf_test_metric",
	// 	Timestamp: time.Now(),
	// 	Value:     float64(time.Now().Unix()),
	// }
}

func init() {
	inputs.Add(InputName, func() inputs.Input {
		return &Redis{}
	})
}
