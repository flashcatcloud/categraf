package diskio

import (
	"fmt"
	"log"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const InputName = "diskio"

type DiskIO struct {
	ps system.PS

	PrintConfigs    bool     `toml:"print_configs"`
	IntervalSeconds int64    `toml:"interval_seconds"`
	Devices         []string `toml:"devices"`
	deviceFilter    filter.Filter
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(InputName, func() inputs.Input {
		return &DiskIO{
			ps: ps,
		}
	})
}

func (d *DiskIO) GetInputName() string {
	return InputName
}

func (d *DiskIO) GetIntervalSeconds() int64 {
	return d.IntervalSeconds
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

func (d *DiskIO) Gather() []*types.Sample {
	var samples []*types.Sample

	devices := []string{}
	if d.deviceFilter == nil {
		// no glob chars
		devices = d.Devices
	}

	diskio, err := d.ps.DiskIO(devices)
	if err != nil {
		log.Println("E! failed to get disk io:", err)
		return samples
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

	return samples
}
