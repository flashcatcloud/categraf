package mem

import (
	"fmt"
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const InputName = "mem"

type MemStats struct {
	PrintConfigs    bool
	IntervalSeconds int64

	quit chan struct{}
}

func init() {
	inputs.Add(InputName, func() inputs.Input {
		return &MemStats{}
	})
}

func (s *MemStats) getInterval() time.Duration {
	if s.IntervalSeconds != 0 {
		return time.Duration(s.IntervalSeconds) * time.Second
	}
	return config.GetInterval()
}

// overwrite func
func (s *MemStats) TidyConfig() error {
	return nil
}

// overwrite func
func (s *MemStats) StopGoroutines() {
	s.quit <- struct{}{}
}

// overwrite func
func (s *MemStats) StartGoroutines(queue chan *types.Sample) {
	go s.LoopGather(queue)
}

func (s *MemStats) LoopGather(queue chan *types.Sample) {
	interval := s.getInterval()
	for {
		select {
		case <-s.quit:
			close(s.quit)
			return
		default:
			time.Sleep(interval)
			defer func() {
				if r := recover(); r != nil {
					if strings.Contains(fmt.Sprint(r), "closed channel") {
						return
					} else {
						log.Println("E! gather metrics panic:", r)
					}
				}
			}()
			s.Gather(queue)
		}
	}
}

// overwrite func
func (s *MemStats) Gather(queue chan *types.Sample) {
	var samples []*types.Sample

	defer func() {
		now := time.Now()
		for i := 0; i < len(samples); i++ {
			samples[i].Timestamp = now
			samples[i].Metric = InputName + "_" + samples[i].Metric
			queue <- samples[i]
		}
	}()

	// ----------------------------------------------
}
