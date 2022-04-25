//go:build linux
// +build linux

package kernelvmstat

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "kernelvmstat"

type KernelVmstat struct {
	Interval  config.Duration `toml:"interval"`
	WhiteList map[string]int  `toml:"white_list"`

	statFile string
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &KernelVmstat{
			statFile: "/proc/vmstat",
		}
	})
}

func (s *KernelVmstat) GetInputName() string {
	return inputName
}

func (s *KernelVmstat) GetInterval() config.Duration {
	return s.Interval
}

func (s *KernelVmstat) Init() error {
	return nil
}

func (s *KernelVmstat) Drop() {}

func (s *KernelVmstat) Gather(slist *list.SafeList) {
	data, err := s.getProcVmstat()
	if err != nil {
		log.Println("E! failed to gather vmstat:", err)
		return
	}

	fields := make(map[string]interface{})

	dataFields := bytes.Fields(data)
	for i, field := range dataFields {
		// dataFields is an array of {"stat1_name", "stat1_value", "stat2_name",
		// "stat2_value", ...}
		// We only want the even number index as that contain the stat name.
		if i%2 == 0 {
			// Convert the stat value into an integer.
			m, err := strconv.ParseInt(string(dataFields[i+1]), 10, 64)
			if err != nil {
				if config.Config.DebugMode {
					log.Println("D! failed to parse vmstat field:", string(dataFields[i]))
				}
				continue
			}

			key := string(field)
			if need, has := s.WhiteList[key]; has && need == 1 {
				fields[key] = m
			}
		}
	}

	inputs.PushSamples(slist, fields)
}

func (s *KernelVmstat) getProcVmstat() ([]byte, error) {
	if _, err := os.Stat(s.statFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s does not exist", s.statFile)
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.statFile)
	if err != nil {
		return nil, err
	}

	return data, nil
}
