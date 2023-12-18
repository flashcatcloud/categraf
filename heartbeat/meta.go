package heartbeat

import (
	"log"
	"time"

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

var meta *SystemInfo

func collectSystemInfo() (*SystemInfo, error) {
	if meta != nil {
		return meta, nil
	}
	timer := time.NewTimer(0 * time.Second)
	defer timer.Stop()

	go func() {
		for {
			select {
			case <-timer.C:
				info, err := collect()
				if err != nil {
					log.Println("W!", "collectSystemInfo error:", err)
					time.Sleep(1 * time.Second)
					continue
				}
				meta = info
				timer.Reset(1 * time.Minute)
			}
		}
	}()

	info, err := collect()
	meta = info
	return meta, err
}

func collect() (*SystemInfo, error) {

	cpuInfo, err := new(cpu.Cpu).Collect()
	if err != nil {
		return nil, err
	}

	memInfo, err := new(memory.Memory).Collect()
	if err != nil {
		return nil, err
	}
	fs, err := new(filesystem.FileSystem).Collect()
	if err != nil {
		return nil, err
	}
	net, err := new(network.Network).Collect()
	if err != nil {
		return nil, err
	}
	pl, err := new(platform.Platform).Collect()
	if err != nil {
		return nil, err
	}

	return &SystemInfo{
		CPU:        cpuInfo,
		Memory:     memInfo,
		Filesystem: fs,
		Network:    net,
		Platform:   pl,
	}, nil
}
