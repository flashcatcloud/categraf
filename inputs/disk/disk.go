package disk

import (
	"fmt"
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

const InputName = "disk"

type DiskStats struct {
	quit chan struct{}
	ps   system.PS

	PrintConfigs    bool     `toml:"print_configs"`
	IntervalSeconds int64    `toml:"interval_seconds"`
	MountPoints     []string `toml:"mount_points"`
	IgnoreFS        []string `toml:"ignore_fs"`
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(InputName, func() inputs.Input {
		return &DiskStats{
			quit: make(chan struct{}),
			ps:   ps,
		}
	})
}

func (s *DiskStats) getInterval() time.Duration {
	if s.IntervalSeconds != 0 {
		return time.Duration(s.IntervalSeconds) * time.Second
	}
	return config.GetInterval()
}

// overwrite func
func (s *DiskStats) Init() error {
	return nil
}

// overwrite func
func (s *DiskStats) StopGoroutines() {
	s.quit <- struct{}{}
}

// overwrite func
func (s *DiskStats) StartGoroutines(queue chan *types.Sample) {
	go s.LoopGather(queue)
}

func (s *DiskStats) LoopGather(queue chan *types.Sample) {
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
func (s *DiskStats) Gather(queue chan *types.Sample) {
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

	disks, partitions, err := s.ps.DiskUsage(s.MountPoints, s.IgnoreFS)
	if err != nil {
		log.Println("E! failed to get disk usage:", err)
		return
	}

	for i, du := range disks {
		if du.Total == 0 {
			// Skip dummy filesystem (procfs, cgroupfs, ...)
			continue
		}
		mountOpts := MountOptions(partitions[i].Opts)
		tags := map[string]string{
			"path":   du.Path,
			"device": strings.Replace(partitions[i].Device, "/dev/", "", -1),
			"fstype": du.Fstype,
			"mode":   mountOpts.Mode(),
		}
		var usedPercent float64
		if du.Used+du.Free > 0 {
			usedPercent = float64(du.Used) /
				(float64(du.Used) + float64(du.Free)) * 100
		}

		fields := map[string]interface{}{
			"total":        du.Total,
			"free":         du.Free,
			"used":         du.Used,
			"used_percent": usedPercent,
			"inodes_total": du.InodesTotal,
			"inodes_free":  du.InodesFree,
			"inodes_used":  du.InodesUsed,
		}

		samples = append(samples, inputs.NewSamples(fields, tags)...)
	}
}

type MountOptions []string

func (opts MountOptions) Mode() string {
	if opts.exists("rw") {
		return "rw"
	} else if opts.exists("ro") {
		return "ro"
	} else {
		return "unknown"
	}
}

func (opts MountOptions) exists(opt string) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}
