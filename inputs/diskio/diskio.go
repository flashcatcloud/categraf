package diskio

import (
	"fmt"
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const InputName = "diskio"

type DiskIO struct {
	quit chan struct{}
	ps   system.PS

	PrintConfigs    bool     `toml:"print_configs"`
	IntervalSeconds int64    `toml:"interval_seconds"`
	Devices         []string `toml:"devices"`
	deviceFilter    filter.Filter
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(InputName, func() inputs.Input {
		return &DiskIO{
			quit: make(chan struct{}),
			ps:   ps,
		}
	})
}

func (d *DiskIO) getInterval() time.Duration {
	if d.IntervalSeconds != 0 {
		return time.Duration(d.IntervalSeconds) * time.Second
	}
	return config.GetInterval()
}

// overwrite func
func (d *DiskIO) Init() error {
	for _, device := range d.Devices {
		if filter.HasMeta(device) {
			deviceFilter, err := filter.Compile(d.Devices)
			if err != nil {
				return fmt.Errorf("error compiling device pattern: %s", err.Error())
			}
			d.deviceFilter = deviceFilter
		}
	}
	return nil
}

// overwrite func
func (d *DiskIO) StopGoroutines() {
	d.quit <- struct{}{}
}

// overwrite func
func (d *DiskIO) StartGoroutines(queue chan *types.Sample) {
	go d.LoopGather(queue)
}

func (d *DiskIO) LoopGather(queue chan *types.Sample) {
	interval := d.getInterval()
	for {
		select {
		case <-d.quit:
			close(d.quit)
			return
		default:
			time.Sleep(interval)
			d.Gather(queue)
		}
	}
}

// overwrite func
func (d *DiskIO) Gather(queue chan *types.Sample) {
	var samples []*types.Sample

	defer func() {
		// if r := recover(); r != nil {
		// 	if strings.Contains(fmt.Sprint(r), "closed channel") {
		// 		return
		// 	} else {
		// 		log.Println("E! gather metrics panic:", r)
		// 	}
		// }

		now := time.Now()
		for i := 0; i < len(samples); i++ {
			samples[i].Timestamp = now
			samples[i].Metric = InputName + "_" + samples[i].Metric
			queue <- samples[i]
		}
	}()

	// ----------------------------------------------

	devices := []string{}
	if d.deviceFilter == nil {
		// no glob chars
		devices = d.Devices
	}

	diskio, err := d.ps.DiskIO(devices)
	if err != nil {
		log.Println("E! failed to get disk io:", err)
		return
	}

	for _, io := range diskio {
		if d.deviceFilter != nil && !d.deviceFilter.Match(io.Name) {
			continue
		}

		fields := map[string]interface{}{
			"reads":            io.ReadCount,
			"writes":           io.WriteCount,
			"read_bytes":       io.ReadBytes,
			"write_bytes":      io.WriteBytes,
			"read_time":        io.ReadTime,
			"write_time":       io.WriteTime,
			"io_time":          io.IoTime,
			"weighted_io_time": io.WeightedIO,
			"iops_in_progress": io.IopsInProgress,
			"merged_reads":     io.MergedReadCount,
			"merged_writes":    io.MergedWriteCount,
		}

		samples = append(samples, inputs.NewSamples(fields, map[string]string{"name": io.Name})...)
	}
}
