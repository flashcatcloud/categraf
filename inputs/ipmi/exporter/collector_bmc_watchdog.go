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
	BMCWatchdogCollectorName CollectorName = "bmc-watchdog"
)

var (
	bmcWatchdogTimerDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "timer_state"),
		"Watchdog timer running (1: running, 0: stopped)",
		[]string{},
		nil,
	)
	watchdogTimerUses       = []string{"BIOS FRB2", "BIOS POST", "OS LOAD", "SMS/OS", "OEM"}
	bmcWatchdogTimerUseDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "timer_use_state"),
		"Watchdog timer use (1: active, 0: inactive)",
		[]string{"name"},
		nil,
	)
	bmcWatchdogLoggingDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "logging_state"),
		"Watchdog log flag (1: Enabled, 0: Disabled / note: reverse of freeipmi)",
		[]string{},
		nil,
	)
	watchdogTimeoutActions       = []string{"None", "Hard Reset", "Power Down", "Power Cycle"}
	bmcWatchdogTimeoutActionDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "timeout_action_state"),
		"Watchdog timeout action (1: active, 0: inactive)",
		[]string{"action"},
		nil,
	)
	watchdogPretimeoutInterrupts       = []string{"None", "SMI", "NMI / Diagnostic Interrupt", "Messaging Interrupt"}
	bmcWatchdogPretimeoutInterruptDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "pretimeout_interrupt_state"),
		"Watchdog pre-timeout interrupt (1: active, 0: inactive)",
		[]string{"interrupt"},
		nil,
	)
	bmcWatchdogPretimeoutIntervalDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "pretimeout_interval_seconds"),
		"Watchdog pre-timeout interval in seconds",
		[]string{},
		nil,
	)
	bmcWatchdogInitialCountdownDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "initial_countdown_seconds"),
		"Watchdog initial countdown in seconds",
		[]string{},
		nil,
	)
	bmcWatchdogCurrentCountdownDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "bmc_watchdog", "current_countdown_seconds"),
		"Watchdog initial countdown in seconds",
		[]string{},
		nil,
	)
)

type BMCWatchdogCollector struct {
	debugMod bool
}

func (c BMCWatchdogCollector) Name() CollectorName {
	return BMCWatchdogCollectorName
}

func (c BMCWatchdogCollector) Cmd() string {
	return "bmc-watchdog"
}

func (c BMCWatchdogCollector) Args() []string {
	return []string{"--get"}
}

func (c BMCWatchdogCollector) Collect(result freeipmi.Result, ch chan<- prometheus.Metric, target ipmiTarget) (int, error) {
	timerState, err := freeipmi.GetBMCWatchdogTimerState(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog timer", "target", targetName(target.host), "error", err)
		return 0, err
	}
	currentTimerUse, err := freeipmi.GetBMCWatchdogTimerUse(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog timer use", "target", targetName(target.host), "error", err)
		return 0, err
	}
	loggingState, err := freeipmi.GetBMCWatchdogLoggingState(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog logging", "target", targetName(target.host), "error", err)
		return 0, err
	}
	currentTimeoutAction, err := freeipmi.GetBMCWatchdogTimeoutAction(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog timeout action", "target", targetName(target.host), "error", err)
		return 0, err
	}
	currentPretimeoutInterrupt, err := freeipmi.GetBMCWatchdogPretimeoutInterrupt(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog pretimeout interrupt", "target", targetName(target.host), "error", err)
		return 0, err
	}
	pretimeoutInterval, err := freeipmi.GetBMCWatchdogPretimeoutInterval(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog pretimeout interval", "target", targetName(target.host), "error", err)
		return 0, err
	}
	initialCountdown, err := freeipmi.GetBMCWatchdogInitialCountdown(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog initial countdown", "target", targetName(target.host), "error", err)
		return 0, err
	}
	currentCountdown, err := freeipmi.GetBMCWatchdogCurrentCountdown(result)
	if err != nil {
		log.Println("E!", "Failed to collect BMC watchdog current countdown", "target", targetName(target.host), "error", err)
		return 0, err
	}

	ch <- prometheus.MustNewConstMetric(bmcWatchdogTimerDesc, prometheus.GaugeValue, timerState)
	for _, timerUse := range watchdogTimerUses {
		if currentTimerUse == timerUse {
			ch <- prometheus.MustNewConstMetric(bmcWatchdogTimerUseDesc, prometheus.GaugeValue, 1, timerUse)
		} else {
			ch <- prometheus.MustNewConstMetric(bmcWatchdogTimerUseDesc, prometheus.GaugeValue, 0, timerUse)
		}
	}
	ch <- prometheus.MustNewConstMetric(bmcWatchdogLoggingDesc, prometheus.GaugeValue, loggingState)
	for _, timeoutAction := range watchdogTimeoutActions {
		if currentTimeoutAction == timeoutAction {
			ch <- prometheus.MustNewConstMetric(bmcWatchdogTimeoutActionDesc, prometheus.GaugeValue, 1, timeoutAction)
		} else {
			ch <- prometheus.MustNewConstMetric(bmcWatchdogTimeoutActionDesc, prometheus.GaugeValue, 0, timeoutAction)
		}
	}
	for _, pretimeoutInterrupt := range watchdogPretimeoutInterrupts {
		if currentPretimeoutInterrupt == pretimeoutInterrupt {
			ch <- prometheus.MustNewConstMetric(bmcWatchdogPretimeoutInterruptDesc, prometheus.GaugeValue, 1, pretimeoutInterrupt)
		} else {
			ch <- prometheus.MustNewConstMetric(bmcWatchdogPretimeoutInterruptDesc, prometheus.GaugeValue, 0, pretimeoutInterrupt)
		}
	}
	ch <- prometheus.MustNewConstMetric(bmcWatchdogPretimeoutIntervalDesc, prometheus.GaugeValue, pretimeoutInterval)
	ch <- prometheus.MustNewConstMetric(bmcWatchdogInitialCountdownDesc, prometheus.GaugeValue, initialCountdown)
	ch <- prometheus.MustNewConstMetric(bmcWatchdogCurrentCountdownDesc, prometheus.GaugeValue, currentCountdown)
	return 1, nil
}
