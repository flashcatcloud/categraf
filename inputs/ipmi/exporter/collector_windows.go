//go:build windows
// +build windows

package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
)

type IPMIConfig struct {
	Timeout uint32
}

func Collect(ch chan<- prometheus.Metric, host, binPath string, config IPMIConfig, debugMod bool) {
	return
}
