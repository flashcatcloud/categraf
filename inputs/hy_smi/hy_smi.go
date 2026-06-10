package hy_smi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cmdx"
	"flashcat.cloud/categraf/types"
)

const inputName = "hy_smi"

type HySMI struct {
	config.PluginConfig

	HySmiCommand string          `toml:"hy_smi_command"`
	QueryTimeOut config.Duration `toml:"query_timeout"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &HySMI{}
	})
}

func (s *HySMI) Clone() inputs.Input {
	return &HySMI{}
}

func (s *HySMI) Name() string {
	return inputName
}

func (s *HySMI) Init() error {
	if s.HySmiCommand == "" {
		return types.ErrInstancesEmpty
	}
	if s.QueryTimeOut == 0 {
		s.QueryTimeOut = config.Duration(5 * time.Second)
	}
	return nil
}

// cardStats represents the JSON structure returned by hy-smi --json
// Example: {"card0": {"Device ID": "0x6320", "VBIOS version": "...", ...}}
type cardStats map[string]map[string]string

func (s *HySMI) Gather(slist *types.SampleList) {
	if s.HySmiCommand == "" {
		return
	}
	begun := time.Now()

	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample(inputName, "scrape_use_seconds", use))
	}(begun)

	stats, err := s.scrape()
	if err != nil {
		slist.PushFront(types.NewSample(inputName, "scraper_up", 0))
		log.Println("E! failed to scrape hy-smi:", err)
		return
	}

	slist.PushFront(types.NewSample(inputName, "scraper_up", 1))

	for cardName, fields := range stats {
		deviceID := fields["Device ID"]
		vbiosVersion := fields["VBIOS version"]

		// Push gpu_info as a gauge with value 1, carrying info labels
		slist.PushFront(types.NewSample(inputName, "gpu_info", 1, map[string]string{
			"card":          cardName,
			"device_id":     deviceID,
			"vbios_version": vbiosVersion,
		}))

		for fieldKey, rawValue := range fields {
			// Skip string fields that are already used as info labels
			if fieldKey == "Device ID" || fieldKey == "VBIOS version" || fieldKey == "Driver version" {
				continue
			}

			metricName, multiplier := fieldToMetric(fieldKey)
			if metricName == "" {
				if s.DebugMod {
					log.Println("D! hy_smi: unknown field:", fieldKey, "value:", rawValue)
				}
				continue
			}

			val, err := strconv.ParseFloat(strings.TrimSpace(rawValue), 64)
			if err != nil {
				if s.DebugMod {
					log.Println("D! hy_smi: failed to parse field:", fieldKey, "raw value:", rawValue, "error:", err)
				}
				continue
			}

			slist.PushFront(types.NewSample(inputName, metricName, val*multiplier, map[string]string{
				"card": cardName,
			}))
		}
	}
}

func (s *HySMI) scrape() (cardStats, error) {
	cmdAndArgs := strings.Fields(s.HySmiCommand)

	if len(cmdAndArgs) == 0 {
		return nil, fmt.Errorf("hy_smi_command is empty")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(cmdAndArgs[0], cmdAndArgs[1:]...) //nolint:gosec
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err, timeout := cmdx.RunTimeout(cmd, time.Duration(s.QueryTimeOut))
	if timeout {
		return nil, fmt.Errorf("run command: %s timeout", strings.Join(cmdAndArgs, " "))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to run command: %s | error: %v | stdout: %s | stderr: %s",
			strings.Join(cmdAndArgs, " "), err, stdout.String(), stderr.String())
	}

	var stats cardStats
	if err := json.Unmarshal(stdout.Bytes(), &stats); err != nil {
		return nil, fmt.Errorf("failed to parse hy-smi JSON output: %v | stdout: %s", err, stdout.String())
	}

	return stats, nil
}

// fieldToMetric maps hy-smi field names to metric names and value multipliers.
func fieldToMetric(field string) (string, float64) {
	switch field {
	case "Temperature (Sensor edge) (C)":
		return "temperature_sensor_edge_celsius", 1
	case "Temperature (Sensor junction) (C)":
		return "temperature_sensor_junction_celsius", 1
	case "Temperature (Sensor mem) (C)":
		return "temperature_sensor_mem_celsius", 1
	case "Temperature (Sensor core) (C)":
		return "temperature_sensor_core_celsius", 1
	case "HCU use (%)":
		return "hcu_use_ratio", 0.01
	case "HCU memory use (%)":
		return "hcu_memory_use_ratio", 0.01
	case "Voltage (mV)":
		return "voltage_millivolts", 1
	default:
		return "", 0
	}
}
