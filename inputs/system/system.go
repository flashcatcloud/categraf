package system

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
)

const InputName = "system"

type SystemStats struct {
	PrintConfigs    bool
	IntervalSeconds int64

	quit chan struct{}

	CollectUserNumber bool
}

func init() {
	inputs.Add(InputName, func() inputs.Input {
		return &SystemStats{}
	})
}

func (s *SystemStats) getInterval() time.Duration {
	if s.IntervalSeconds != 0 {
		return time.Duration(s.IntervalSeconds) * time.Second
	}
	return config.GetInterval()
}

// overwrite func
func (s *SystemStats) TidyConfig() error {
	return nil
}

// overwrite func
func (s *SystemStats) StopGoroutines() {
	s.quit <- struct{}{}
}

// overwrite func
func (s *SystemStats) StartGoroutines(queue chan *types.Sample) {
	go s.LoopGather(queue)
}

func (s *SystemStats) LoopGather(queue chan *types.Sample) {
	interval := s.getInterval()
	for {
		select {
		case <-s.quit:
			close(s.quit)
			return
		default:
			time.Sleep(interval)
			s.Gather(queue)
		}
	}
}

// overwrite func
func (s *SystemStats) Gather(queue chan *types.Sample) {
	var samples []*types.Sample

	defer func() {
		if r := recover(); r != nil {
			if strings.Contains(fmt.Sprint(r), "closed channel") {
				return
			} else {
				log.Println("E! gather metrics panic:", r)
			}
		}

		now := time.Now()
		for i := 0; i < len(samples); i++ {
			samples[i].Timestamp = now
			samples[i].Metric = InputName + "_" + samples[i].Metric
			queue <- samples[i]
		}
	}()

	// ----------------------------------------------

	loadavg, err := load.Avg()
	if err != nil && !strings.Contains(err.Error(), "not implemented") {
		log.Println("E! failed to gather system load:", err)
		return
	}

	numCPUs, err := cpu.Counts(true)
	if err != nil {
		log.Println("E! failed to gather cpu number:", err)
		return
	}

	fields := map[string]interface{}{
		"load1":        loadavg.Load1,
		"load5":        loadavg.Load5,
		"load15":       loadavg.Load15,
		"n_cpus":       numCPUs,
		"load_norm_1":  loadavg.Load1 / float64(numCPUs),
		"load_norm_5":  loadavg.Load5 / float64(numCPUs),
		"load_norm_15": loadavg.Load15 / float64(numCPUs),
	}

	uptime, err := host.Uptime()
	if err != nil {
		log.Println("E! failed to get host uptime:", err)
		return
	}

	fields["uptime"] = uptime

	if s.CollectUserNumber {
		users, err := host.Users()
		if err == nil {
			fields["n_users"] = len(users)
		} else if os.IsNotExist(err) {
			log.Println("W! reading os users:", err)
		} else if os.IsPermission(err) {
			log.Println("W! reading os users:", err)
		}
	}

	samples = inputs.NewSamples(fields)
}
