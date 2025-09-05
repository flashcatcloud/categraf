package diskio

import (
	"fmt"
	"log"
	"time"

	"github.com/shirou/gopsutil/v3/disk"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const inputName = "diskio"

type DiskIO struct {
	ps system.PS

	config.PluginConfig
	Devices      []string `toml:"devices"`
	deviceFilter filter.Filter
	lastSeen     time.Time
	lastStat     map[string]disk.IOCountersStat
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &DiskIO{
			ps: system.NewSystemPS(),
		}
	})
}

func (d *DiskIO) Clone() inputs.Input {
	return &DiskIO{
		ps: system.NewSystemPS(),
	}
}

func (c *DiskIO) Name() string {
	return inputName
}

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

func (d *DiskIO) Gather(slist *types.SampleList) {
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

	now := time.Now()
	for k, io := range diskio {
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
		if lastValue, exists := d.lastStat[k]; exists {
			deltaRWCount := float64(io.ReadCount + io.WriteCount - lastValue.ReadCount - lastValue.WriteCount)
			deltaRWTime := float64(io.ReadTime + io.WriteTime - lastValue.ReadTime - lastValue.WriteTime)
			deltaIOTime := float64(io.IoTime - lastValue.IoTime)
			if deltaRWCount > 0 {
				fields["io_await"] = deltaRWTime / deltaRWCount
				fields["io_svctm"] = deltaIOTime / deltaRWCount
			}
			itv := float64(now.Sub(d.lastSeen).Milliseconds())
			if itv > 0 {
				fields["io_util"] = 100 * deltaIOTime / itv
			}
		}

		slist.PushSamples("diskio", fields, map[string]string{"name": io.Name})
	}
	d.lastSeen = now
	d.lastStat = diskio
}
