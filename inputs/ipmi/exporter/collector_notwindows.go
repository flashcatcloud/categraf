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
	"path"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/inputs/ipmi/exporter/freeipmi"
)

const (
	namespace   = "ipmi"
	targetLocal = ""
)

type Collector interface {
	Name() CollectorName
	Cmd() string
	Args() []string
	Collect(output freeipmi.Result, ch chan<- prometheus.Metric, target ipmiTarget) (int, error)
}

type metaCollector struct {
	target string
	module string
	config *SafeConfig
}

type ipmiTarget struct {
	host   string
	config IPMIConfig
}

var (
	upDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"'1' if a scrape of the IPMI device was successful, '0' otherwise.",
		[]string{"collector"},
		nil,
	)

	durationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "scrape_duration", "seconds"),
		"Returns how long the scrape took to complete in seconds.",
		nil,
		nil,
	)
)

// Describe implements Prometheus.Collector.
func (c metaCollector) Describe(ch chan<- *prometheus.Desc) {
	// all metrics are described ad-hoc
}

func markCollectorUp(ch chan<- prometheus.Metric, name string, up int) {
	ch <- prometheus.MustNewConstMetric(
		upDesc,
		prometheus.GaugeValue,
		float64(up),
		name,
	)
}

// Collect implements Prometheus.Collector.
func Collect(ch chan<- prometheus.Metric, host, binPath string, config IPMIConfig, debugMod bool) {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()

		if debugMod {
			log.Println("D!", "Scrape duration", "target", targetName(host), "duration", duration)
		}
		ch <- prometheus.MustNewConstMetric(
			durationDesc,
			prometheus.GaugeValue,
			duration,
		)
	}()

	target := ipmiTarget{
		host:   host,
		config: config,
	}

	for _, collector := range config.GetCollectors(debugMod) {
		var up int
		if debugMod {
			log.Println("D!", "Running collector", "target", target.host, "collector", collector.Name())
		}

		fqcmd := path.Join(binPath, collector.Cmd())
		args := collector.Args()
		cfg := config.GetFreeipmiConfig()

		result := freeipmi.Execute(fqcmd, args, cfg, target.host, debugMod)

		up, _ = collector.Collect(result, ch, target)
		markCollectorUp(ch, string(collector.Name()), up)
	}
}

func targetName(target string) string {
	if target == targetLocal {
		return "[local]"
	}
	return target
}
