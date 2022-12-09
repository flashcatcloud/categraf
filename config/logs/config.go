//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"time"
)

// ContainerCollectAll is the name of the docker integration that collect logs from all containers
const ContainerCollectAll = "container_collect_all"

// SnmpTraps is the name of the integration that collects logs from SNMP traps received by the Agent
const SnmpTraps = "snmp_traps"

// DefaultIntakeProtocol indicates that no special protocol is in use for the endpoint intake track type.
const DefaultIntakeProtocol IntakeProtocol = ""

// DefaultIntakeOrigin indicates that no special DD_SOURCE header is in use for the endpoint intake track type.
const DefaultIntakeOrigin IntakeOrigin = "agent"

// ServerlessIntakeOrigin is the lambda extension origin
const ServerlessIntakeOrigin IntakeOrigin = "lambda-extension"

// logs-intake endpoints depending on the site and environment.

// HTTPConnectivity is the status of the HTTP connectivity
type HTTPConnectivity bool

var (
	// HTTPConnectivitySuccess is the status for successful HTTP connectivity
	HTTPConnectivitySuccess HTTPConnectivity = true
	// HTTPConnectivityFailure is the status for failed HTTP connectivity
	HTTPConnectivityFailure HTTPConnectivity = false
)

// ContainerCollectAllSource returns a source to collect all logs from all containers.
func ContainerCollectAllSource(containerCollectAll bool) *LogSource {
	if containerCollectAll {
		// source to collect all logs from all containers
		return NewLogSource(ContainerCollectAll, &LogsConfig{
			Type:    DockerType,
			Service: "docker",
			Source:  "docker",
		})
	}
	return nil
}

// ExpectedTagsDuration returns a duration of the time expected tags will be submitted for.
func ExpectedTagsDuration() time.Duration {
	return time.Duration(1) * time.Second
}

// IsExpectedTagsSet returns boolean showing if expected tags feature is enabled.
func IsExpectedTagsSet() bool {
	return ExpectedTagsDuration() > 0
}

// TaggerWarmupDuration is used to configure the tag providers
func TaggerWarmupDuration() time.Duration {
	// TODO support custom param
	return time.Duration(0 * time.Second)
}

// AggregationTimeout is used when performing aggregation operations
func AggregationTimeout() time.Duration {
	return time.Duration(1000 * time.Millisecond)
}
