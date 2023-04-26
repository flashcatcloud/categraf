//go:build linux
// +build linux

package kernel

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "kernel"

// /proc/stat file line prefixes to gather stats on:
var (
	interrupts      = []byte("intr")
	contextSwitches = []byte("ctxt")
	processesForked = []byte("processes")
	diskPages       = []byte("page")
	bootTime        = []byte("btime")
)

type KernelStats struct {
	config.PluginConfig

	statFile        string
	entropyStatFile string
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &KernelStats{
			statFile:        "/proc/stat",
			entropyStatFile: "/proc/sys/kernel/random/entropy_avail",
		}
	})
}
func (s *KernelStats) Clone() inputs.Input {
	return &KernelStats{
		statFile:        "/proc/stat",
		entropyStatFile: "/proc/sys/kernel/random/entropy_avail",
	}
}

func (s *KernelStats) Name() string {
	return inputName
}

func (s *KernelStats) Gather(slist *types.SampleList) {
	data, err := s.getProcStat()
	if err != nil {
		log.Println("E! failed to read:", s.statFile, "error:", err)
		return
	}

	entropyData, err := os.ReadFile(s.entropyStatFile)
	if err != nil {
		log.Println("E! failed to read:", s.entropyStatFile, "error:", err)
		return
	}

	entropyString := string(entropyData)
	entropyValue, err := strconv.ParseInt(strings.TrimSpace(entropyString), 10, 64)
	if err != nil {
		log.Println("E! failed to parse:", s.entropyStatFile, "error:", err)
		return
	}

	fields := make(map[string]interface{})

	fields["entropy_avail"] = entropyValue

	dataFields := bytes.Fields(data)
	for i, field := range dataFields {
		switch {
		case bytes.Equal(field, interrupts):
			m, err := strconv.ParseInt(string(dataFields[i+1]), 10, 64)
			if err == nil {
				fields["interrupts"] = m
			}

		case bytes.Equal(field, contextSwitches):
			m, err := strconv.ParseInt(string(dataFields[i+1]), 10, 64)
			if err == nil {
				fields["context_switches"] = m
			}

		case bytes.Equal(field, processesForked):
			m, err := strconv.ParseInt(string(dataFields[i+1]), 10, 64)
			if err == nil {
				fields["processes_forked"] = m
			}

		case bytes.Equal(field, bootTime):
			m, err := strconv.ParseInt(string(dataFields[i+1]), 10, 64)
			if err == nil {
				fields["boot_time"] = m
			}

		case bytes.Equal(field, diskPages):
			in, err := strconv.ParseInt(string(dataFields[i+1]), 10, 64)
			if err == nil {
				fields["disk_pages_in"] = in
			}

			out, err := strconv.ParseInt(string(dataFields[i+2]), 10, 64)
			if err == nil {
				fields["disk_pages_out"] = out
			}
		}
	}

	slist.PushSamples(inputName, fields)
}

func (s *KernelStats) getProcStat() ([]byte, error) {
	if _, err := os.Stat(s.statFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("kernel: %s does not exist", s.statFile)
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.statFile)
	if err != nil {
		return nil, err
	}

	return data, nil
}
