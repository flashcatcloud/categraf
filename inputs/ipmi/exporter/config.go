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
	"strings"
	"sync"

	"flashcat.cloud/categraf/inputs/ipmi/exporter/freeipmi"
	"github.com/prometheus/client_golang/prometheus"
)

// CollectorName is used for unmarshaling the list of collectors in the yaml config file
type CollectorName string

// ConfiguredCollector wraps an existing Collector implementation,
// potentially altering its default settings.
type ConfiguredCollector struct {
	collector    Collector
	command      string
	default_args []string
	custom_args  []string
}

func (c ConfiguredCollector) Name() CollectorName {
	return c.collector.Name()
}

func (c ConfiguredCollector) Cmd() string {
	if c.command != "" {
		return c.command
	}
	return c.collector.Cmd()
}

func (c ConfiguredCollector) Args() []string {
	args := []string{}
	if c.custom_args != nil {
		// custom args come first, this way it is quite easy to
		// override a Collector to use e.g. sudo
		args = append(args, c.custom_args...)
	}
	if c.default_args != nil {
		args = append(args, c.default_args...)
	} else {
		args = append(args, c.collector.Args()...)
	}
	return args
}

func (c ConfiguredCollector) Collect(output freeipmi.Result, ch chan<- prometheus.Metric, target ipmiTarget) (int, error) {
	return c.collector.Collect(output, ch, target)
}

func (c CollectorName) GetInstance(debugMod bool) (Collector, error) {
	// This is where a new Collector would have to be "registered"
	switch c {
	case IPMICollectorName:
		return IPMICollector{debugMod: debugMod}, nil
	case BMCCollectorName:
		return BMCCollector{debugMod: debugMod}, nil
	case BMCWatchdogCollectorName:
		return BMCWatchdogCollector{debugMod: debugMod}, nil
	case SELCollectorName:
		return SELCollector{debugMod: debugMod}, nil
	case DCMICollectorName:
		return DCMICollector{debugMod: debugMod}, nil
	case ChassisCollectorName:
		return ChassisCollector{debugMod: debugMod}, nil
	case SMLANModeCollectorName:
		return SMLANModeCollector{debugMod: debugMod}, nil
	}
	return nil, fmt.Errorf("invalid Collector: %s", string(c))
}

func (c CollectorName) IsValid(debugMod bool) error {
	_, err := c.GetInstance(debugMod)
	return err
}

// Config is the Go representation of the yaml config file.
type Config struct {
	Modules map[string]IPMIConfig `yaml:"modules"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// SafeConfig wraps Config for concurrency-safe operations.
type SafeConfig struct {
	sync.RWMutex
	C *Config
}

// IPMIConfig is the Go representation of a module configuration in the yaml
// config file.
type IPMIConfig struct {
	User             string                     `toml:"user"`
	Password         string                     `toml:"pass"`
	Privilege        string                     `toml:"privilege"`
	Driver           string                     `toml:"driver"`
	Timeout          uint32                     `toml:"timeout"`
	Collectors       []CollectorName            `toml:"collectors"`
	ExcludeSensorIDs []int64                    `toml:"exclude_sensor_ids"`
	WorkaroundFlags  []string                   `toml:"workaround_flags"`
	CollectorCmd     map[CollectorName]string   `toml:"collector_cmd"`
	CollectorArgs    map[CollectorName][]string `toml:"default_args"`
	CustomArgs       map[CollectorName][]string `toml:"custom_args"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `toml:",inline"`
}

var defaultConfig = IPMIConfig{
	Collectors: []CollectorName{IPMICollectorName, DCMICollectorName, BMCCollectorName, ChassisCollectorName},
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

func (c IPMIConfig) GetCollectors(debugMod bool) []Collector {
	result := []Collector{}
	for _, co := range c.Collectors {
		// At this point validity has already been checked
		i, _ := co.GetInstance(debugMod)
		cc := ConfiguredCollector{
			collector:    i,
			command:      c.CollectorCmd[i.Name()],
			default_args: c.CollectorArgs[i.Name()],
			custom_args:  c.CustomArgs[i.Name()],
		}
		result = append(result, cc)
	}
	return result
}

func (c IPMIConfig) GetFreeipmiConfig() string {
	var b strings.Builder
	if c.Driver != "" {
		fmt.Fprintf(&b, "driver-type %s\n", c.Driver)
	}
	if c.Privilege != "" {
		fmt.Fprintf(&b, "privilege-level %s\n", c.Privilege)
	}
	if c.User != "" {
		fmt.Fprintf(&b, "username %s\n", c.User)
	}
	if c.Password != "" {
		fmt.Fprintf(&b, "password %s\n", freeipmi.EscapePassword(c.Password))
	}
	if c.Timeout != 0 {
		fmt.Fprintf(&b, "session-timeout %d\n", c.Timeout)
	}
	if len(c.WorkaroundFlags) > 0 {
		fmt.Fprintf(&b, "workaround-flags")
		for _, flag := range c.WorkaroundFlags {
			fmt.Fprintf(&b, " %s", flag)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}
