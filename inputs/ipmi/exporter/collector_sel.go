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
	"log"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/inputs/ipmi/exporter/freeipmi"
)

const (
	SELCollectorName CollectorName = "sel"
)

var (
	selEntriesCountDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "sel", "logs_count"),
		"Current number of log entries in the SEL.",
		[]string{},
		nil,
	)

	selFreeSpaceDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "sel", "free_space_bytes"),
		"Current free space remaining for new SEL entries.",
		[]string{},
		nil,
	)
)

type SELCollector struct {
	debugMod bool
}

func (c SELCollector) Name() CollectorName {
	return SELCollectorName
}

func (c SELCollector) Cmd() string {
	return "ipmi-sel"
}

func (c SELCollector) Args() []string {
	return []string{"--info"}
}

func (c SELCollector) Collect(result freeipmi.Result, ch chan<- prometheus.Metric, target ipmiTarget) (int, error) {
	entriesCount, err := freeipmi.GetSELInfoEntriesCount(result)
	if err != nil {
		log.Println("E!", "Failed to collect SEL data", "target", targetName(target.host), "error", err)
		return 0, err
	}
	freeSpace, err := freeipmi.GetSELInfoFreeSpace(result)
	if err != nil {
		log.Println("E!", "Failed to collect SEL data", "target", targetName(target.host), "error", err)
		return 0, err
	}
	ch <- prometheus.MustNewConstMetric(
		selEntriesCountDesc,
		prometheus.GaugeValue,
		entriesCount,
	)
	ch <- prometheus.MustNewConstMetric(
		selFreeSpaceDesc,
		prometheus.GaugeValue,
		freeSpace,
	)
	return 1, nil
}
