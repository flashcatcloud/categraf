//go:build linux
// +build linux

// Copyright 2015 The Prometheus Authors
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

package systemd

import (
	"context"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

// Init returns a new Collector exposing systemd statistics.
func (s *Systemd) Init() error {
	if !s.Enable {
		return types.ErrInstancesEmpty
	}
	if s.UnitExclude == "" {
		s.UnitExclude = ".+\\.(automount|device|mount|scope|slice)"
	}
	if s.UnitInclude == "" {
		s.UnitInclude = ".+"
	}
	s.unitIncludePattern = regexp.MustCompile(fmt.Sprintf("^(?:%s)$", s.UnitInclude))
	s.unitExcludePattern = regexp.MustCompile(fmt.Sprintf("^(?:%s)$", s.UnitExclude))

	conn, err := s.newSystemdDbusConn()
	if err != nil {
		return fmt.Errorf("couldn't get dbus connection: %w", err)
	}
	s.conn = conn

	return nil
}

func (s *Systemd) Drop() {
	if s.conn != nil {
		s.conn.Close()
	}
}

// Gather  gathers metrics from systemd.  Dbus collection is done in parallel
// to reduce wait time for responses.
func (s *Systemd) Gather(slist *types.SampleList) {
	begin := time.Now()

	systemdVersion, systemdVersionFull := s.getSystemdVersion()
	if systemdVersion < minSystemdVersionSystemState {
		log.Println("msg", "Detected systemd version is lower than minimum, some systemd state and timer metrics will not be available", "current", systemdVersion, "minimum", minSystemdVersionSystemState)
	}
	slist.PushSample(inputName, "version", systemdVersion, map[string]string{"version": systemdVersionFull})

	allUnits, err := s.getAllUnits()
	if err != nil {
		log.Println("E! couldn't get units: %w", err)
		return
	}

	begin = time.Now()
	summary := summarizeUnits(allUnits)
	s.collectSummaryMetrics(slist, summary)
	if config.Config.DebugMode {
		log.Println("D!", "collectSummaryMetrics took", "duration_seconds", time.Since(begin).Seconds())
	}

	begin = time.Now()
	units := filterUnits(allUnits, s.unitIncludePattern, s.unitExcludePattern)
	if config.Config.DebugMode {
		log.Println("D!", "filterUnits took", "duration_seconds", time.Since(begin).Seconds())
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		begin = time.Now()
		s.collectUnitStatusMetrics(slist, units)
	}()

	if s.EnableStartTimeMetrics {
		wg.Add(1)
		go func() {
			defer wg.Done()
			begin = time.Now()
			s.collectUnitStartTimeMetrics(slist, units)
		}()
	}

	if s.EnableTaskMetrics {
		wg.Add(1)
		go func() {
			defer wg.Done()
			begin = time.Now()
			s.collectUnitTasksMetrics(slist, units)
		}()
	}

	if systemdVersion >= minSystemdVersionSystemState {
		wg.Add(1)
		go func() {
			defer wg.Done()
			begin = time.Now()
			s.collectTimers(slist, units)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		begin = time.Now()
		s.collectSockets(slist, units)
	}()

	if systemdVersion >= minSystemdVersionSystemState {
		begin = time.Now()
		err = s.collectSystemState(slist)
	}
	if err != nil {
		log.Println("E! collect systemd state:", err)
	}
}

func (s *Systemd) collectUnitStatusMetrics(slist *types.SampleList, units []unit) {
	for _, unit := range units {
		serviceType := ""
		if strings.HasSuffix(unit.Name, ".service") {
			serviceTypeProperty, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Service", "Type")
			if err != nil {
				log.Println("E!", "couldn't get unit type", "unit", unit.Name, "err", err)
			} else {
				serviceType = serviceTypeProperty.Value.Value().(string)
			}
		} else if strings.HasSuffix(unit.Name, ".mount") {
			serviceTypeProperty, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Mount", "Type")
			if err != nil {
				log.Println("E!", "couldn't get unit type", "unit", unit.Name, "err", err)
			} else {
				serviceType = serviceTypeProperty.Value.Value().(string)
			}
		}
		for _, stateName := range unitStatesName {
			isActive := 0.0
			if stateName == unit.ActiveState {
				isActive = 1.0
			}
			slist.PushSample(inputName, "unit_state", isActive, map[string]string{"name": unit.Name,
				"state": stateName, "type": serviceType})
		}
		if s.EnableRestartMetrics && strings.HasSuffix(unit.Name, ".service") {
			// NRestarts wasn't added until systemd 235.
			restartsCount, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Service", "NRestarts")
			if err != nil {
				log.Println("E!", "couldn't get unit NRestarts", "unit", unit.Name, "err", err)
			} else {
				slist.PushSample(inputName, "service_restart_total", restartsCount.Value.Value().(uint32),
					map[string]string{"name": unit.Name})
			}
		}
	}
}

func (s *Systemd) collectSockets(slist *types.SampleList, units []unit) {
	tag := make(map[string]string)
	for _, unit := range units {
		if !strings.HasSuffix(unit.Name, ".socket") {
			continue
		}

		acceptedConnectionCount, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Socket", "NAccepted")
		if err != nil {
			log.Println("W!", "couldn't get unit NAccepted", "unit", unit.Name, "err", err)
			continue
		}
		tag["name"] = unit.Name
		slist.PushSample(inputName, "socket_accepted_connections_total",
			acceptedConnectionCount.Value.Value().(uint32), tag)

		currentConnectionCount, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Socket", "NConnections")
		if err != nil {
			log.Println("W!", "couldn't get unit NConnections", "unit", unit.Name, "err", err)
			continue
		}
		slist.PushSample(inputName, "socket_current_connections",
			currentConnectionCount.Value.Value().(uint32), tag)

		// NRefused wasn't added until systemd 239.
		refusedConnectionCount, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Socket", "NRefused")
		if err != nil {
			log.Printf("couldn't get unit '%s' NRefused: %s", unit.Name, err)
		} else {
			slist.PushSample(inputName, "socket_refused_connections_total",
				refusedConnectionCount.Value.Value().(uint32), tag)
		}
	}
}

func (s *Systemd) collectUnitStartTimeMetrics(slist *types.SampleList, units []unit) {
	var startTimeUsec uint64

	tag := map[string]string{}
	for _, unit := range units {
		if unit.ActiveState != "active" {
			startTimeUsec = 0
		} else {
			timestampValue, err := s.conn.GetUnitPropertyContext(context.TODO(), unit.Name, "ActiveEnterTimestamp")
			if err != nil {
				log.Println("W!", "couldn't get unit StartTimeUsec", "unit", unit.Name, "err", err)
				continue
			}
			startTimeUsec = timestampValue.Value.Value().(uint64)
		}
		tag["name"] = unit.Name

		slist.PushSample(inputName, "unit_start_time_seconds", float64(startTimeUsec)/1e6, tag)
	}
}

func (s *Systemd) collectUnitTasksMetrics(slist *types.SampleList, units []unit) {
	var (
		val uint64
	)
	tag := make(map[string]string)
	for _, unit := range units {
		tag["name"] = unit.Name
		if strings.HasSuffix(unit.Name, ".service") {
			tasksCurrentCount, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Service", "TasksCurrent")
			if err != nil {
				log.Println("E!", "couldn't get unit TasksCurrent", "unit", unit.Name, "err", err)
			} else {
				val = tasksCurrentCount.Value.Value().(uint64)
				// Don't set if tasksCurrent if dbus reports MaxUint64.
				if val != math.MaxUint64 {
					slist.PushSample(inputName, "unit_tasks_current", float64(val), tag)
				}
			}
			tasksMaxCount, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Service", "TasksMax")
			if err != nil {
				log.Println("E!", "couldn't get unit TasksMax", "unit", unit.Name, "err", err)
			} else {
				val = tasksMaxCount.Value.Value().(uint64)
				// Don't set if tasksMax if dbus reports MaxUint64.
				if val != math.MaxUint64 {
					slist.PushSample(inputName, "unit_tasks_max", float64(val), tag)
				}
			}
		}
	}
}

func (s *Systemd) collectTimers(slist *types.SampleList, units []unit) {
	tag := make(map[string]string)
	for _, unit := range units {
		if !strings.HasSuffix(unit.Name, ".timer") {
			continue
		}
		tag["name"] = unit.Name

		lastTriggerValue, err := s.conn.GetUnitTypePropertyContext(context.TODO(), unit.Name, "Timer", "LastTriggerUSec")
		if err != nil {
			log.Println("W!", "couldn't get unit LastTriggerUSec", "unit", unit.Name, "err", err)
			continue
		}

		slist.PushSample(inputName, "timer_last_trigger_seconds",
			float64(lastTriggerValue.Value.Value().(uint64))/1e6, tag)
	}
}

func (s *Systemd) collectSummaryMetrics(slist *types.SampleList, summary map[string]float64) {
	for stateName, count := range summary {
		slist.PushSample(inputName, "units", count, map[string]string{"state": stateName})
	}
}

func (s *Systemd) collectSystemState(slist *types.SampleList) error {
	systemState, err := s.conn.GetManagerProperty("SystemState")
	if err != nil {
		return fmt.Errorf("couldn't get system state: %w", err)
	}
	isSystemRunning := 0.0
	if systemState == `"running"` {
		isSystemRunning = 1.0
	}
	slist.PushSample(inputName, "system_running", isSystemRunning)
	return nil
}

func (s *Systemd) newSystemdDbusConn() (*dbus.Conn, error) {
	if s.SystemdPrivate {
		return dbus.NewSystemdConnectionContext(context.TODO())
	}
	return dbus.NewWithContext(context.TODO())
}

func (s *Systemd) getAllUnits() ([]unit, error) {
	allUnits, err := s.conn.ListUnitsContext(context.TODO())
	if err != nil {
		return nil, err
	}

	result := make([]unit, 0, len(allUnits))
	for _, status := range allUnits {
		unit := unit{
			UnitStatus: status,
		}
		result = append(result, unit)
	}

	return result, nil
}

func summarizeUnits(units []unit) map[string]float64 {
	summarized := make(map[string]float64)

	for _, unitStateName := range unitStatesName {
		summarized[unitStateName] = 0.0
	}

	for _, unit := range units {
		summarized[unit.ActiveState] += 1.0
	}

	return summarized
}

func filterUnits(units []unit, includePattern, excludePattern *regexp.Regexp) []unit {
	filtered := make([]unit, 0, len(units))
	for _, unit := range units {
		if includePattern.MatchString(unit.Name) && !excludePattern.MatchString(unit.Name) && unit.LoadState == "loaded" {
			filtered = append(filtered, unit)
		}
	}

	return filtered
}

func (s *Systemd) getSystemdVersion() (float64, string) {
	version, err := s.conn.GetManagerProperty("Version")
	if err != nil {
		return 0, ""
	}
	version = strings.TrimPrefix(strings.TrimSuffix(version, `"`), `"`)
	parsedVersion := systemdVersionRE.FindString(version)
	v, err := strconv.ParseFloat(parsedVersion, 64)
	if err != nil {
		return 0, ""
	}
	return v, version
}
