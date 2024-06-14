//go:build !windows
// +build !windows

// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"fmt"
	"log"
	"math"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/inputs/ipmi/exporter/freeipmi"
)

const (
	IPMICollectorName CollectorName = "ipmi"
)

var (
	sensorStateDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "sensor", "state"),
		"Indicates the severity of the state reported by an IPMI sensor (0=nominal, 1=warning, 2=critical).",
		[]string{"id", "name", "type"},
		nil,
	)

	sensorValueDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "sensor", "value"),
		"Generic data read from an IPMI sensor of unknown type, relying on labels for context.",
		[]string{"id", "name", "type"},
		nil,
	)

	fanSpeedRPMDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "fan_speed", "rpm"),
		"Fan speed in rotations per minute.",
		[]string{"id", "name"},
		nil,
	)

	fanSpeedRatioDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "fan_speed", "ratio"),
		"Fan speed as a proportion of the maximum speed.",
		[]string{"id", "name"},
		nil,
	)

	fanSpeedStateDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "fan_speed", "state"),
		"Reported state of a fan speed sensor (0=nominal, 1=warning, 2=critical).",
		[]string{"id", "name"},
		nil,
	)

	temperatureDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "temperature", "celsius"),
		"Temperature reading in degree Celsius.",
		[]string{"id", "name"},
		nil,
	)

	temperatureStateDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "temperature", "state"),
		"Reported state of a temperature sensor (0=nominal, 1=warning, 2=critical).",
		[]string{"id", "name"},
		nil,
	)

	voltageDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "voltage", "volts"),
		"Voltage reading in Volts.",
		[]string{"id", "name"},
		nil,
	)

	voltageStateDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "voltage", "state"),
		"Reported state of a voltage sensor (0=nominal, 1=warning, 2=critical).",
		[]string{"id", "name"},
		nil,
	)

	currentDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "current", "amperes"),
		"Current reading in Amperes.",
		[]string{"id", "name"},
		nil,
	)

	currentStateDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "current", "state"),
		"Reported state of a current sensor (0=nominal, 1=warning, 2=critical).",
		[]string{"id", "name"},
		nil,
	)

	powerDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "power", "watts"),
		"Power reading in Watts.",
		[]string{"id", "name"},
		nil,
	)

	powerStateDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "power", "state"),
		"Reported state of a power sensor (0=nominal, 1=warning, 2=critical).",
		[]string{"id", "name"},
		nil,
	)
)

type IPMICollector struct {
	debugMod bool
}

func (c IPMICollector) Name() CollectorName {
	return IPMICollectorName
}

func (c IPMICollector) Cmd() string {
	return "ipmimonitoring"
}

func (c IPMICollector) Args() []string {
	return []string{
		"-Q",
		"--ignore-unrecognized-events",
		"--comma-separated-output",
		"--no-header-output",
		"--sdr-cache-recreate",
		"--output-event-bitmask",
		"--output-sensor-state",
	}
}

func (c IPMICollector) Collect(result freeipmi.Result, ch chan<- prometheus.Metric, target ipmiTarget) (int, error) {
	excludeIds := target.config.ExcludeSensorIDs
	targetHost := targetName(target.host)
	results, err := freeipmi.GetSensorData(result, excludeIds)
	if err != nil {
		log.Println("E!", "Failed to collect sensor data", "target", targetHost, "error", err)
		return 0, err
	}
	for _, data := range results {
		var state float64

		switch data.State {
		case "Nominal":
			state = 0
		case "Warning":
			state = 1
		case "Critical":
			state = 2
		case "N/A":
			state = math.NaN()
		default:
			log.Println("W!", "Unknown sensor state", "target", targetHost, "state", data.State)
			state = math.NaN()
		}

		if c.debugMod {
			log.Println("D!", "Got values", "target", targetHost, "data", fmt.Sprintf("%+v", data))
		}

		switch data.Unit {
		case "RPM":
			collectTypedSensor(ch, fanSpeedRPMDesc, fanSpeedStateDesc, state, data, 1.0)
		case "C":
			collectTypedSensor(ch, temperatureDesc, temperatureStateDesc, state, data, 1.0)
		case "A":
			collectTypedSensor(ch, currentDesc, currentStateDesc, state, data, 1.0)
		case "V":
			collectTypedSensor(ch, voltageDesc, voltageStateDesc, state, data, 1.0)
		case "W":
			collectTypedSensor(ch, powerDesc, powerStateDesc, state, data, 1.0)
		case "%":
			switch data.Type {
			case "Fan":
				collectTypedSensor(ch, fanSpeedRatioDesc, fanSpeedStateDesc, state, data, 0.01)
			default:
				collectGenericSensor(ch, state, data)
			}
		default:
			collectGenericSensor(ch, state, data)
		}
	}
	return 1, nil
}

func (c IPMICollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- sensorStateDesc
	ch <- sensorValueDesc
	ch <- fanSpeedRPMDesc
	ch <- fanSpeedRatioDesc
	ch <- fanSpeedStateDesc
	ch <- temperatureDesc
	ch <- temperatureStateDesc
	ch <- voltageDesc
	ch <- voltageStateDesc
	ch <- currentDesc
	ch <- currentStateDesc
	ch <- powerDesc
	ch <- powerStateDesc
}

func collectTypedSensor(ch chan<- prometheus.Metric, desc, stateDesc *prometheus.Desc, state float64, data freeipmi.SensorData, scale float64) {
	ch <- prometheus.MustNewConstMetric(
		desc,
		prometheus.GaugeValue,
		data.Value*scale,
		strconv.FormatInt(data.ID, 10),
		data.Name,
	)
	ch <- prometheus.MustNewConstMetric(
		stateDesc,
		prometheus.GaugeValue,
		state,
		strconv.FormatInt(data.ID, 10),
		data.Name,
	)
}

func collectGenericSensor(ch chan<- prometheus.Metric, state float64, data freeipmi.SensorData) {
	ch <- prometheus.MustNewConstMetric(
		sensorValueDesc,
		prometheus.GaugeValue,
		data.Value,
		strconv.FormatInt(data.ID, 10),
		data.Name,
		data.Type,
	)
	ch <- prometheus.MustNewConstMetric(
		sensorStateDesc,
		prometheus.GaugeValue,
		state,
		strconv.FormatInt(data.ID, 10),
		data.Name,
		data.Type,
	)
}
