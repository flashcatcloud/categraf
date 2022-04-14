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
	Labels          map[string]string

	quit chan struct{}

	CollectUserNumber bool
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
	for {
		select {
		case <-s.quit:
			close(s.quit)
			return
		default:
			time.Sleep(s.getInterval())
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
func (s *SystemStats) Gather(queue chan *types.Sample) {
	var samples []*types.Sample

	defer func() {
		now := time.Now()
		for i := 0; i < len(samples); i++ {
			samples[i].Timestamp = now
			samples[i].Metric = InputName + "_" + samples[i].Metric
			queue <- samples[i]
		}
	}()

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

	samples = append(samples,
		inputs.NewSample("load1", loadavg.Load1),
		inputs.NewSample("load5", loadavg.Load5),
		inputs.NewSample("load15", loadavg.Load15),
		inputs.NewSample("n_cpus", float64(numCPUs)),
		inputs.NewSample("load_norm_1", loadavg.Load1/float64(numCPUs)),
		inputs.NewSample("load_norm_5", loadavg.Load5/float64(numCPUs)),
		inputs.NewSample("load_norm_15", loadavg.Load15/float64(numCPUs)),
	)

	uptime, err := host.Uptime()
	if err != nil {
		log.Println("E! failed to get host uptime:", err)
		return
	}

	samples = append(samples, inputs.NewSample("uptime", float64(uptime)))

	if s.CollectUserNumber {
		users, err := host.Users()
		if err == nil {
			samples = append(samples, inputs.NewSample("n_users", float64(len(users))))
		} else if os.IsNotExist(err) {
			if config.Config.DebugMode {
				log.Println("D! reading os users:", err)
			}
		} else if os.IsPermission(err) {
			if config.Config.DebugMode {
				log.Println("D! reading os users:", err)
			}
		}
	}
}

func init() {
	inputs.Add(InputName, func() inputs.Input {
		return &SystemStats{}
	})
}
