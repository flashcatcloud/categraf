package heartbeat

import (
	"flashcat.cloud/categraf/heartbeat/cpu"
	"flashcat.cloud/categraf/heartbeat/filesystem"
	"flashcat.cloud/categraf/heartbeat/memory"
	"flashcat.cloud/categraf/heartbeat/network"
	"flashcat.cloud/categraf/heartbeat/platform"
)

type (
	SystemInfo struct {
		CPU        interface{} `json:"cpu"`
		Memory     interface{} `json:"memory"`
		Network    interface{} `json:"network"`
		Platform   interface{} `json:"platform"`
		Filesystem interface{} `json:"filesystem"`
	}
)

func collectSystemInfo() (*SystemInfo, error) {
	return collect()
}

func collect() (*SystemInfo, error) {
	si := &SystemInfo{}
	cpuInfo, err := new(cpu.Cpu).Collect()
	if err == nil {
		si.CPU = cpuInfo
	}

	memInfo, err := new(memory.Memory).Collect()
	if err == nil {
		si.Memory = memInfo
	}
	fs, err := new(filesystem.FileSystem).Collect()
	if err == nil {
		si.Filesystem = fs
	}
	net, err := new(network.Network).Collect()
	if err == nil {
		si.Network = net
	}
	pl, err := new(platform.Platform).Collect()
	if err == nil {
		si.Platform = pl
	}

	return si, nil
}
