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
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"

	"flashcat.cloud/categraf/inputs/ipmi/exporter/freeipmi"
)

const (
	BMCCollectorName CollectorName = "bmc"
)

var (
	bmcInfoDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc", "info"),
		"Constant metric with value '1' providing details about the BMC.",
		[]string{"firmware_revision", "manufacturer_id", "system_firmware_version"},
		nil,
	)
)

type BMCCollector struct {
	debugMod bool
}

func (c BMCCollector) Name() CollectorName {
	return BMCCollectorName
}

func (c BMCCollector) Cmd() string {
	return "bmc-info"
}

func (c BMCCollector) Args() []string {
	return []string{}
}

func (c BMCCollector) Collect(result freeipmi.Result, ch chan<- prometheus.Metric, target ipmiTarget) (int, error) {
	firmwareRevision, err := freeipmi.GetBMCInfoFirmwareRevision(result)
	if err != nil {
		klog.ErrorS(err, "failed to collect BMC data", "target", targetName(target.host))
		return 0, err
	}
	manufacturerID, err := freeipmi.GetBMCInfoManufacturerID(result)
	if err != nil {
		klog.ErrorS(err, "failed to collect BMC data", "target", targetName(target.host))
		return 0, err
	}
	systemFirmwareVersion, err := freeipmi.GetBMCInfoSystemFirmwareVersion(result)
	if err != nil {
		// This one is not always available.
		klog.ErrorS(err, "failed to parse bmc-info data", "target", targetName(target.host))
		systemFirmwareVersion = "N/A"
	}
	ch <- prometheus.MustNewConstMetric(
		bmcInfoDesc,
		prometheus.GaugeValue,
		1,
		firmwareRevision, manufacturerID, systemFirmwareVersion,
	)
	return 1, nil
}
