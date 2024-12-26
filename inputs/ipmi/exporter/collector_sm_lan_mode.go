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
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/inputs/ipmi/exporter/freeipmi"
)

const (
	SMLANModeCollectorName CollectorName = "sm-lan-mode"
)

var (
	lanModeDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "config", "lan_mode"),
		"Returns configured LAN mode (0=dedicated, 1=shared, 2=failover).",
		nil,
		nil,
	)
)

type SMLANModeCollector struct {
	debugMod bool
}

func (c SMLANModeCollector) Name() CollectorName {
	return SMLANModeCollectorName
}

func (c SMLANModeCollector) Cmd() string {
	return "ipmi-raw"
}

func (c SMLANModeCollector) Args() []string {
	return []string{"0x0", "0x30", "0x70", "0x0c", "0"}
}

func (c SMLANModeCollector) Collect(result freeipmi.Result, ch chan<- prometheus.Metric, target ipmiTarget) (int, error) {
	octets, err := freeipmi.GetRawOctets(result)
	if err != nil {
		log.Println("E!", "Failed to collect LAN mode data", "target", targetName(target.host), "error", err)
		return 0, err
	}
	if len(octets) != 3 {
		log.Println("E!", "Unexpected number of octets", "target", targetName(target.host), "octets", octets)
		return 0, fmt.Errorf("unexpected number of octets in raw response: %d", len(octets))
	}

	switch octets[2] {
	case "00", "01", "02":
		value, _ := strconv.Atoi(octets[2])
		ch <- prometheus.MustNewConstMetric(lanModeDesc, prometheus.GaugeValue, float64(value))
	default:
		log.Println("E!", "Unexpected lan mode status (ipmi-raw)", "target", targetName(target.host), "sgatus", octets[2])
		return 0, fmt.Errorf("unexpected lan mode status: %s", octets[2])
	}

	return 1, nil
}
