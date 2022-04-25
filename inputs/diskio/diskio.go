package diskio

import (
	"fmt"
	"log"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/pkg/filter"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "diskio"

type DiskIO struct {
	ps system.PS

	Interval     config.Duration `toml:"interval"`
	Devices      []string        `toml:"devices"`
	deviceFilter filter.Filter
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(inputName, func() inputs.Input {
		return &DiskIO{
			ps: ps,
		}
	})
}

func (d *DiskIO) GetInputName() string {
	return inputName
}

func (d *DiskIO) GetInterval() config.Duration {
	return d.Interval
}

func (d *DiskIO) Drop() {}

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

func (d *DiskIO) Gather(slist *list.SafeList) {
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

		inputs.PushSamples(slist, fields, map[string]string{"name": io.Name})
	}
}
