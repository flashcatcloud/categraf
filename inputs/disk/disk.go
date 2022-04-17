package disk

import (
	"log"
	"strings"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

const inputName = "disk"

type DiskStats struct {
	ps system.PS

	PrintConfigs    bool     `toml:"print_configs"`
	IntervalSeconds int64    `toml:"interval_seconds"`
	MountPoints     []string `toml:"mount_points"`
	IgnoreFS        []string `toml:"ignore_fs"`
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(inputName, func() inputs.Input {
		return &DiskStats{
			ps: ps,
		}
	})
}

func (s *DiskStats) GetInputName() string {
	return inputName
}

func (s *DiskStats) GetIntervalSeconds() int64 {
	return s.IntervalSeconds
}

// overwrite func
func (s *DiskStats) Init() error {
	return nil
}

func (s *DiskStats) Gather() []*types.Sample {
	var samples []*types.Sample

	disks, partitions, err := s.ps.DiskUsage(s.MountPoints, s.IgnoreFS)
	if err != nil {
		log.Println("E! failed to get disk usage:", err)
		return samples
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

	return samples
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
