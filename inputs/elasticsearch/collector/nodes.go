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

package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

func getRoles(node NodeStatsNodeResponse) map[string]bool {
	// default settings (2.x) and map, which roles to consider
	roles := map[string]bool{
		"master":                false,
		"data":                  false,
		"data_hot":              false,
		"data_warm":             false,
		"data_cold":             false,
		"data_frozen":           false,
		"data_content":          false,
		"ml":                    false,
		"remote_cluster_client": false,
		"transform":             false,
		"ingest":                false,
		"client":                true,
	}
	// assumption: a 5.x node has at least one role, otherwise it's a 1.7 or 2.x node
	if len(node.Roles) > 0 {
		for _, role := range node.Roles {
			// set every absent role to false
			if _, ok := roles[role]; !ok {
				roles[role] = false
			} else {
				// if present in the roles field, set to true
				roles[role] = true
			}
		}
	} else {
		for role, setting := range node.Attributes {
			if _, ok := roles[role]; ok {
				if setting == "false" {
					roles[role] = false
				} else {
					roles[role] = true
				}
			}
		}
	}
	if len(node.HTTP) == 0 {
		roles["client"] = false
	}
	return roles
}

func createRoleMetric(role string) *nodeMetric {
	return &nodeMetric{
		Type: prometheus.GaugeValue,
		Desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "nodes", "roles"),
			"Node roles",
			defaultRoleLabels, prometheus.Labels{"role": role},
		),
		Value: func(node NodeStatsNodeResponse) float64 {
			return 1.0
		},
		Labels: func(cluster string, node NodeStatsNodeResponse) []string {
			return []string{
				cluster,
				node.Host,
				node.Name,
			}
		},
	}
}

var (
	defaultNodeLabels               = []string{"cluster", "host", "name", "es_master_node", "es_data_node", "es_ingest_node", "es_client_node"}
	defaultRoleLabels               = []string{"cluster", "host", "name"}
	defaultThreadPoolLabels         = append(defaultNodeLabels, "type")
	defaultBreakerLabels            = append(defaultNodeLabels, "breaker")
	defaultFilesystemDataLabels     = append(defaultNodeLabels, "mount", "path")
	defaultFilesystemIODeviceLabels = append(defaultNodeLabels, "device")
	defaultCacheLabels              = append(defaultNodeLabels, "cache")

	defaultNodeLabelValues = func(cluster string, node NodeStatsNodeResponse) []string {
		roles := getRoles(node)
		return []string{
			cluster,
			node.Host,
			node.Name,
			fmt.Sprintf("%t", roles["master"]),
			fmt.Sprintf("%t", roles["data"]),
			fmt.Sprintf("%t", roles["ingest"]),
			fmt.Sprintf("%t", roles["client"]),
		}
	}
	defaultThreadPoolLabelValues = func(cluster string, node NodeStatsNodeResponse, pool string) []string {
		return append(defaultNodeLabelValues(cluster, node), pool)
	}
	defaultFilesystemDataLabelValues = func(cluster string, node NodeStatsNodeResponse, mount string, path string) []string {
		return append(defaultNodeLabelValues(cluster, node), mount, path)
	}
	defaultFilesystemIODeviceLabelValues = func(cluster string, node NodeStatsNodeResponse, device string) []string {
		return append(defaultNodeLabelValues(cluster, node), device)
	}
	defaultCacheHitLabelValues = func(cluster string, node NodeStatsNodeResponse) []string {
		return append(defaultNodeLabelValues(cluster, node), "hit")
	}
	defaultCacheMissLabelValues = func(cluster string, node NodeStatsNodeResponse) []string {
		return append(defaultNodeLabelValues(cluster, node), "miss")
	}
)

type nodeMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(node NodeStatsNodeResponse) float64
	Labels func(cluster string, node NodeStatsNodeResponse) []string
}

type gcCollectionMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(gcStats NodeStatsJVMGCCollectorResponse) float64
	Labels func(cluster string, node NodeStatsNodeResponse, collector string) []string
}

type breakerMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(breakerStats NodeStatsBreakersResponse) float64
	Labels func(cluster string, node NodeStatsNodeResponse, breaker string) []string
}

type threadPoolMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(threadPoolStats NodeStatsThreadPoolPoolResponse) float64
	Labels func(cluster string, node NodeStatsNodeResponse, breaker string) []string
}

type filesystemDataMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(fsStats NodeStatsFSDataResponse) float64
	Labels func(cluster string, node NodeStatsNodeResponse, mount string, path string) []string
}

type filesystemIODeviceMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(fsStats NodeStatsFSIOStatsDeviceResponse) float64
	Labels func(cluster string, node NodeStatsNodeResponse, device string) []string
}

// Nodes information struct
type Nodes struct {
	client    *http.Client
	url       *url.URL
	all       bool
	node      string
	local     bool
	nodeStats []string

	up                              prometheus.Gauge
	totalScrapes, jsonParseFailures prometheus.Counter

	jvmMetrics                []*nodeMetric
	gcCollectionMetrics       []*gcCollectionMetric
	osMetrics                 []*nodeMetric
	processMetrics            []*nodeMetric
	breakerMetrics            []*breakerMetric
	indicesMetrics            []*nodeMetric
	transportMetrics          []*nodeMetric
	httpMetrics               []*nodeMetric
	threadPoolMetrics         []*threadPoolMetric
	filesystemDataMetrics     []*filesystemDataMetric
	filesystemIODeviceMetrics []*filesystemIODeviceMetric
}

// NewNodes defines Nodes Prometheus metrics
func NewNodes(client *http.Client, url *url.URL, all bool, node string, local bool, nodeStats []string) *Nodes {
	return &Nodes{
		client:    client,
		url:       url,
		all:       all,
		node:      node,
		local:     local,
		nodeStats: nodeStats,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "node_stats", "up"),
			Help: "Was the last scrape of the Elasticsearch nodes endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "node_stats", "total_scrapes"),
			Help: "Current total Elasticsearch node scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "node_stats", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),

		transportMetrics: []*nodeMetric{
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "transport", "rx_packets_total"),
					"Count of packets received",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Transport.RxCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "transport", "rx_size_in_bytes_total"),
					"Total number of bytes received",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Transport.RxSize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "transport", "tx_packets_total"),
					"Count of packets sent",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Transport.TxCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "transport", "tx_size_in_bytes_total"),
					"Total number of bytes sent",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Transport.TxSize)
				},
				Labels: defaultNodeLabelValues,
			},
		},
		jvmMetrics: []*nodeMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm", "threads_count"),
					"Count of threads",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Threads.Count)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm", "threads_peak_count"),
					"Peak count of threads",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Threads.PeakCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm", "timestamp"),
					"JVM timestamp",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Timestamp)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_mem_heap", "used_in_bytes"),
					"JVM memory currently used by area",
					append(defaultNodeLabels, "area"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.HeapUsed)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "heap")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_mem_non_heap", "used_in_bytes"),
					"JVM memory currently used by area",
					append(defaultNodeLabels, "area"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.NonHeapUsed)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "non-heap")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_mem_heap", "max_in_bytes"),
					"JVM memory max",
					append(defaultNodeLabels, "area"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.HeapMax)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "heap")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_mem_heap", "used_percent"),
					"JVM memory used percent",
					append(defaultNodeLabels, "area"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.HeapUsedPercent)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "heap")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_mem_heap", "committed_in_bytes"),
					"JVM memory currently committed by area",
					append(defaultNodeLabels, "area"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.HeapCommitted)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "heap")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_mem_non_heap", "committed_in_bytes"),
					"JVM memory currently committed by area",
					append(defaultNodeLabels, "area"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.NonHeapCommitted)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "non-heap")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_young", "used_in_bytes"),
					"JVM memory currently used by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["young"].Used)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "young")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_young", "max_in_bytes"),
					"JVM memory max by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["young"].Max)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "young")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_young", "peak_used_in_bytes"),
					"JVM memory peak used by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["young"].PeakUsed)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "young")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_young", "peak_max_in_bytes"),
					"JVM memory peak max by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["young"].PeakMax)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "young")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_survivor", "used_in_bytes"),
					"JVM memory currently used by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["survivor"].Used)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "survivor")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_survivor", "max_in_bytes"),
					"JVM memory max by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["survivor"].Max)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "survivor")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_survivor", "peak_used_in_bytes"),
					"JVM memory peak used by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["survivor"].PeakUsed)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "survivor")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_survivor", "peak_max_in_bytes"),
					"JVM memory peak max by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["survivor"].PeakMax)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "survivor")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_old", "used_in_bytes"),
					"JVM memory currently used by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["old"].Used)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "old")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_old", "max_in_bytes"),
					"JVM memory max by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["old"].Max)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "old")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_old", "peak_used_in_bytes"),
					"JVM memory peak used by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["old"].PeakUsed)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "old")
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_memory_pools_old", "peak_max_in_bytes"),
					"JVM memory peak max by pool",
					append(defaultNodeLabels, "pool"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Mem.Pools["old"].PeakMax)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "old")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_buffer_pool_direct", "count"),
					"JVM buffer count",
					append(defaultNodeLabels, "type"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.BufferPools["direct"].Count)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "direct")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_buffer_pool_direct", "total_capacity_in_bytes"),
					"JVM buffer total capacity",
					append(defaultNodeLabels, "type"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.BufferPools["direct"].TotalCapacity)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "direct")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_buffer_pool_direct", "used_in_bytes"),
					"JVM buffer currently used",
					append(defaultNodeLabels, "type"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.BufferPools["direct"].Used)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "direct")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_buffer_pool_mapped", "count"),
					"JVM buffer count",
					append(defaultNodeLabels, "type"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.BufferPools["mapped"].Count)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "mapped")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_buffer_pool_mapped", "total_capacity_in_bytes"),
					"JVM buffer total capacity",
					append(defaultNodeLabels, "type"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.BufferPools["mapped"].TotalCapacity)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "mapped")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_buffer_pool_mapped", "used_in_bytes"),
					"JVM buffer currently used",
					append(defaultNodeLabels, "type"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.BufferPools["mapped"].Used)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "mapped")
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_classes", "current_loaded_count"),
					"JVM classes currently loaded",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Classes.CurrentLoadedCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_classes", "total_loaded_count"),
					"JVM classes total loaded",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Classes.TotalLoadedCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_classes", "total_unloaded_count"),
					"JVM classes total unloaded",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Classes.TotalUnloadedCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm", "uptime_seconds"),
					"JVM process uptime in seconds",
					append(defaultNodeLabels, "type"), nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.JVM.Uptime) / 1000
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return append(defaultNodeLabelValues(cluster, node), "mapped")
				},
			},
		},
		gcCollectionMetrics: []*gcCollectionMetric{
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_gc", "collection_seconds_count"),
					"Count of JVM GC runs",
					append(defaultNodeLabels, "gc"), nil,
				),
				Value: func(gcStats NodeStatsJVMGCCollectorResponse) float64 {
					return float64(gcStats.CollectionCount)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse, collector string) []string {
					return append(defaultNodeLabelValues(cluster, node), collector)
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "jvm_gc", "collection_seconds_sum"),
					"GC run time in seconds",
					append(defaultNodeLabels, "gc"), nil,
				),
				Value: func(gcStats NodeStatsJVMGCCollectorResponse) float64 {
					return float64(gcStats.CollectionTime) / 1000
				},
				Labels: func(cluster string, node NodeStatsNodeResponse, collector string) []string {
					return append(defaultNodeLabelValues(cluster, node), collector)
				},
			},
		},
		osMetrics: []*nodeMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cpu_load_average_1m"),
					"Shortterm load average",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return node.OS.CPU.LoadAvg.Load1
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cpu_load_average_5m"),
					"Midterm load average",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return node.OS.CPU.LoadAvg.Load5
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cpu_load_average_15m"),
					"Longterm load average",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return node.OS.CPU.LoadAvg.Load15
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cpu_percent"),
					"Percent CPU used by OS",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.CPU.Percent)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "mem_free_in_bytes"),
					"Amount of free physical memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Mem.Free)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "mem_used_in_bytes"),
					"Amount of used physical memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Mem.Used)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "mem_actual_free_in_bytes"),
					"Amount of free physical memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Mem.ActualFree)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "mem_actual_used_in_bytes"),
					"Amount of used physical memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Mem.ActualUsed)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "mem_used_percent"),
					"Percent of used physical memory",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return node.OS.Mem.UsedPercent
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "mem_total_in_bytes"),
					"Amount of used physical memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Mem.Total)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "mem_free_percent"),
					"Percent of free physical memory",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return node.OS.Mem.FreePercent
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cgroup_cpu_cfs_period_micros"),
					"CPU CFS period in microseconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.CGroup.CPU.CFSPeriod)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cgroup_cpu_cfs_quota_micros"),
					"CPU CFS quota in microseconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.CGroup.CPU.CFSQuota)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cgroup_cpu_stat_number_of_elapsed_periods"),
					"CPU CFS quota in microseconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.CGroup.CPU.Stat.NumberElapsed)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cgroup_cpu_stat_number_of_times_throttled"),
					"CPU CFS quota in microseconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.CGroup.CPU.Stat.NumberThrottled)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cgroup_cpu_stat_time_throttled_nanos"),
					"CPU CFS quota in microseconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.CGroup.CPU.Stat.TimeThrottled)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "cgroup_cpuacct_usage_nanos"),
					"Cpuacct usage in nanos",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.CGroup.CPUAcct.Usage)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "swap_used_in_bytes"),
					"Amount of used swap memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Swap.Used)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "swap_total_in_bytes"),
					"Amount of total swap memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Swap.Total)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "os", "swap_free_in_bytes"),
					"Amount of free swap memory in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.OS.Swap.Free)
				},
				Labels: defaultNodeLabelValues,
			},
		},
		processMetrics: []*nodeMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "cpu_percent"),
					"Percent CPU used by process",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.CPU.Percent)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "mem_resident_size_in_bytes"),
					"Resident memory in use by process in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.Memory.Resident)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "mem_share_size_in_bytes"),
					"Shared memory in use by process in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.Memory.Share)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "mem_total_virtual_in_bytes"),
					"Total virtual memory used in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.Memory.TotalVirtual)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "timestamp"),
					"Timestamp",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.Timestamp)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "open_files_descriptors"),
					"Open file descriptors",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.OpenFD)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "max_files_descriptors"),
					"Max file descriptors",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.MaxFD)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "process", "cpu_seconds_total"),
					"Process CPU time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Process.CPU.Total) / 1000
				},
				Labels: func(cluster string, node NodeStatsNodeResponse) []string {
					return defaultNodeLabelValues(cluster, node)
				},
			},
		},
		breakerMetrics: []*breakerMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "breakers", "estimated_size_in_bytes"),
					"Estimated size in bytes of breaker",
					defaultBreakerLabels, nil,
				),
				Value: func(breakerStats NodeStatsBreakersResponse) float64 {
					return float64(breakerStats.EstimatedSize)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse, breaker string) []string {
					return append(defaultNodeLabelValues(cluster, node), breaker)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "breakers", "limit_size_in_bytes"),
					"Limit size in bytes for breaker",
					defaultBreakerLabels, nil,
				),
				Value: func(breakerStats NodeStatsBreakersResponse) float64 {
					return float64(breakerStats.LimitSize)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse, breaker string) []string {
					return append(defaultNodeLabelValues(cluster, node), breaker)
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "breakers", "tripped"),
					"tripped for breaker",
					defaultBreakerLabels, nil,
				),
				Value: func(breakerStats NodeStatsBreakersResponse) float64 {
					return float64(breakerStats.Tripped)
				},
				Labels: func(cluster string, node NodeStatsNodeResponse, breaker string) []string {
					return append(defaultNodeLabelValues(cluster, node), breaker)
				},
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "breakers", "overhead"),
					"Overhead of circuit breakers",
					defaultBreakerLabels, nil,
				),
				Value: func(breakerStats NodeStatsBreakersResponse) float64 {
					return breakerStats.Overhead
				},
				Labels: func(cluster string, node NodeStatsNodeResponse, breaker string) []string {
					return append(defaultNodeLabelValues(cluster, node), breaker)
				},
			},
		},
		indicesMetrics: []*nodeMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "fielddata_memory_size_in_bytes"),
					"Field data cache memory usage in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.FieldData.MemorySize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "fielddata_evictions"),
					"Evictions from field data",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.FieldData.Evictions)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "completion_size_in_bytes"),
					"Completion in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Completion.Size)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "filter_cache_memory_size_in_bytes"),
					"Filter cache memory usage in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.FilterCache.MemorySize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "filter_cache_evictions"),
					"Evictions from filter cache",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.FilterCache.Evictions)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "query_cache_memory_size_in_bytes"),
					"Query cache memory usage in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.QueryCache.MemorySize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "query_cache_evictions"),
					"Evictions from query cache",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.QueryCache.Evictions)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "query_cache_total_count"),
					"Query cache total count",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.QueryCache.TotalCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "query_cache_cache_size"),
					"Query cache cache size",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.QueryCache.CacheSize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "query_cache_cache_count"),
					"Query cache cache count",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.QueryCache.CacheCount)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "query_cache_hit_count"),
					"Query cache count",
					defaultCacheLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.QueryCache.HitCount)
				},
				Labels: defaultCacheHitLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "query_cache_miss_count"),
					"Query miss count",
					defaultCacheLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.QueryCache.MissCount)
				},
				Labels: defaultCacheMissLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "request_cache_memory_size_in_bytes"),
					"Request cache memory usage in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.RequestCache.MemorySize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "request_cache_evictions"),
					"Evictions from request cache",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.RequestCache.Evictions)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "request_cache_count"),
					"Request cache count",
					defaultCacheLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.RequestCache.HitCount)
				},
				Labels: defaultCacheHitLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "request_miss_count"),
					"Request miss count",
					defaultCacheLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.RequestCache.MissCount)
				},
				Labels: defaultCacheMissLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "translog_operations"),
					"Total translog operations",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Translog.Operations)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "translog_size_in_bytes"),
					"Total translog size in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Translog.Size)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "get_time_seconds"),
					"Total get time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Get.Time) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "get_total"),
					"Total get",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Get.Total)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "get_missing_time_seconds"),
					"Total time of get missing in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Get.MissingTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "get_missing_total"),
					"Total get missing",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Get.MissingTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "get_exists_time_seconds"),
					"Total time get exists in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Get.ExistsTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "get_exists_total"),
					"Total get exists operations",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Get.ExistsTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_refresh", "time_seconds_total"),
					"Total time spent refreshing in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Refresh.TotalTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_refresh", "total"),
					"Total refreshes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Refresh.Total)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_query_time_seconds"),
					"Total search query time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.QueryTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_query_total"),
					"Total number of queries",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.QueryTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_fetch_time_seconds"),
					"Total search fetch time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.FetchTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_fetch_total"),
					"Total number of fetches",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.FetchTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_suggest_total"),
					"Total number of suggests",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.SuggestTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_suggest_time_seconds"),
					"Total suggest time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.SuggestTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_scroll_total"),
					"Total number of scrolls",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.ScrollTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "search_scroll_time_seconds"),
					"Total scroll time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Search.ScrollTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "docs_count"),
					"Count of documents on this node",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Docs.Count)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "docs_deleted"),
					"Count of deleted documents on this node",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Docs.Deleted)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "store_size_in_bytes"),
					"Current size of stored index data in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Store.Size)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "store_throttle_time_seconds_total"),
					"Throttle time for index store in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Store.ThrottleTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_memory_in_bytes"),
					"Current memory size of segments in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.Memory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_count"),
					"Count of index segments on this node",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.Count)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_terms_memory_in_bytes"),
					"Count of terms in memory for this node",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.TermsMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_index_writer_memory_in_bytes"),
					"Count of memory for index writer on this node",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.IndexWriterMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_max_unsafe_auto_id_timestamp"),
					"Count of memory for index writer on this node",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.MaxUnsafeAutoIDTimestamp)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_norms_memory_in_bytes"),
					"Count of memory used by norms",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.NormsMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_stored_fields_memory_in_bytes"),
					"Count of stored fields memory",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.StoredFieldsMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_doc_values_memory_in_bytes"),
					"Count of doc values memory",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.DocValuesMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_fixed_bit_set_memory_in_bytes"),
					"Count of fixed bit set",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.FixedBitSet)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_term_vectors_memory_in_bytes"),
					"Term vectors memory usage in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.TermVectorsMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_points_memory_in_bytes"),
					"Point values memory usage in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.PointsMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "segments_version_map_memory_in_bytes"),
					"Version map memory usage in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Segments.VersionMapMemory)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "flush_total"),
					"Total flushes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Flush.Total)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "flush_time_seconds"),
					"Cumulative flush time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Flush.Time) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "warmer_total"),
					"Total warmer count",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Warmer.Total)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices", "warmer_time_seconds_total"),
					"Total warmer time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Warmer.TotalTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_indexing", "index_time_seconds_total"),
					"Cumulative index time in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Indexing.IndexTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_indexing", "index_total"),
					"Total index calls",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Indexing.IndexTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_indexing", "delete_time_seconds_total"),
					"Total time indexing delete in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Indexing.DeleteTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_indexing", "delete_total"),
					"Total indexing deletes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Indexing.DeleteTotal)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_indexing", "is_throttled"),
					"Indexing throttling",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					if node.Indices.Indexing.IsThrottled {
						return 1
					}
					return 0
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_indexing", "throttle_time_seconds_total"),
					"Cumulative indexing throttling time",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Indexing.ThrottleTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_merges", "total"),
					"Total merges",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Merges.Total)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_merges", "current"),
					"Current merges",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Merges.Current)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_merges", "current_size_in_bytes"),
					"Size of a current merges in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Merges.CurrentSize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_merges", "docs_total"),
					"Cumulative docs merged",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Merges.TotalDocs)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_merges", "total_size_in_bytes"),
					"Total merge size in bytes",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Merges.TotalSize)
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_merges", "total_time_seconds_total"),
					"Total time spent merging in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Merges.TotalTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_merges", "total_throttled_time_seconds"),
					"Total throttled time of merges in seconds",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					return float64(node.Indices.Merges.TotalThrottledTime) / 1000
				},
				Labels: defaultNodeLabelValues,
			},
		},
		threadPoolMetrics: []*threadPoolMetric{
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "thread_pool", "completed_count"),
					"Thread Pool operations completed",
					defaultThreadPoolLabels, nil,
				),
				Value: func(threadPoolStats NodeStatsThreadPoolPoolResponse) float64 {
					return float64(threadPoolStats.Completed)
				},
				Labels: defaultThreadPoolLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "thread_pool", "rejected_count"),
					"Thread Pool operations rejected",
					defaultThreadPoolLabels, nil,
				),
				Value: func(threadPoolStats NodeStatsThreadPoolPoolResponse) float64 {
					return float64(threadPoolStats.Rejected)
				},
				Labels: defaultThreadPoolLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "thread_pool", "active_count"),
					"Thread Pool threads active",
					defaultThreadPoolLabels, nil,
				),
				Value: func(threadPoolStats NodeStatsThreadPoolPoolResponse) float64 {
					return float64(threadPoolStats.Active)
				},
				Labels: defaultThreadPoolLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "thread_pool", "largest_count"),
					"Thread Pool largest threads count",
					defaultThreadPoolLabels, nil,
				),
				Value: func(threadPoolStats NodeStatsThreadPoolPoolResponse) float64 {
					return float64(threadPoolStats.Largest)
				},
				Labels: defaultThreadPoolLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "thread_pool", "queue_count"),
					"Thread Pool operations queued",
					defaultThreadPoolLabels, nil,
				),
				Value: func(threadPoolStats NodeStatsThreadPoolPoolResponse) float64 {
					return float64(threadPoolStats.Queue)
				},
				Labels: defaultThreadPoolLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "thread_pool", "threads_count"),
					"Thread Pool current threads count",
					defaultThreadPoolLabels, nil,
				),
				Value: func(threadPoolStats NodeStatsThreadPoolPoolResponse) float64 {
					return float64(threadPoolStats.Threads)
				},
				Labels: defaultThreadPoolLabelValues,
			},
		},
		filesystemDataMetrics: []*filesystemDataMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_data", "available_bytes"),
					"Available space on block device in bytes",
					defaultFilesystemDataLabels, nil,
				),
				Value: func(fsStats NodeStatsFSDataResponse) float64 {
					return float64(fsStats.Available)
				},
				Labels: defaultFilesystemDataLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_data", "free_bytes"),
					"Free space on block device in bytes",
					defaultFilesystemDataLabels, nil,
				),
				Value: func(fsStats NodeStatsFSDataResponse) float64 {
					return float64(fsStats.Free)
				},
				Labels: defaultFilesystemDataLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_data", "size_in_bytes"),
					"Size of block device in bytes",
					defaultFilesystemDataLabels, nil,
				),
				Value: func(fsStats NodeStatsFSDataResponse) float64 {
					return float64(fsStats.Total)
				},
				Labels: defaultFilesystemDataLabelValues,
			},
		},
		filesystemIODeviceMetrics: []*filesystemIODeviceMetric{
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_io_stats_device", "operations_count"),
					"Count of disk operations",
					defaultFilesystemIODeviceLabels, nil,
				),
				Value: func(fsIODeviceStats NodeStatsFSIOStatsDeviceResponse) float64 {
					return float64(fsIODeviceStats.Operations)
				},
				Labels: defaultFilesystemIODeviceLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_io_stats_device", "read_operations_count"),
					"Count of disk read operations",
					defaultFilesystemIODeviceLabels, nil,
				),
				Value: func(fsIODeviceStats NodeStatsFSIOStatsDeviceResponse) float64 {
					return float64(fsIODeviceStats.ReadOperations)
				},
				Labels: defaultFilesystemIODeviceLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_io_stats_device", "write_operations_count"),
					"Count of disk write operations",
					defaultFilesystemIODeviceLabels, nil,
				),
				Value: func(fsIODeviceStats NodeStatsFSIOStatsDeviceResponse) float64 {
					return float64(fsIODeviceStats.WriteOperations)
				},
				Labels: defaultFilesystemIODeviceLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_io_stats_device", "read_size_kilobytes_sum"),
					"Total kilobytes read from disk",
					defaultFilesystemIODeviceLabels, nil,
				),
				Value: func(fsIODeviceStats NodeStatsFSIOStatsDeviceResponse) float64 {
					return float64(fsIODeviceStats.ReadSize)
				},
				Labels: defaultFilesystemIODeviceLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "filesystem_io_stats_device", "write_size_kilobytes_sum"),
					"Total kilobytes written to disk",
					defaultFilesystemIODeviceLabels, nil,
				),
				Value: func(fsIODeviceStats NodeStatsFSIOStatsDeviceResponse) float64 {
					return float64(fsIODeviceStats.WriteSize)
				},
				Labels: defaultFilesystemIODeviceLabelValues,
			},
		},
		httpMetrics: []*nodeMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "http", "current_open"),
					"http current open",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					if v, ok := node.HTTP["current_open"]; ok {
						return v.(float64)
					}

					return 0
				},
				Labels: defaultNodeLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "http", "total_opened"),
					"http total opened",
					defaultNodeLabels, nil,
				),
				Value: func(node NodeStatsNodeResponse) float64 {
					if v, ok := node.HTTP["total_opened"]; ok {
						return v.(float64)
					}

					return 0
				},
				Labels: defaultNodeLabelValues,
			},
		},
	}
}

// Describe add metrics descriptions
func (c *Nodes) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.jvmMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.gcCollectionMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.breakerMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.osMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.processMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.indicesMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.transportMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.threadPoolMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.filesystemDataMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.filesystemIODeviceMetrics {
		ch <- metric.Desc
	}
	for _, metric := range c.httpMetrics {
		ch <- metric.Desc
	}
	ch <- c.up.Desc()
	ch <- c.totalScrapes.Desc()
	ch <- c.jsonParseFailures.Desc()
}

func (c *Nodes) fetchAndDecodeNodeStats() (nodeStatsResponse, error) {
	var nsr nodeStatsResponse

	u := *c.url

	if c.all {
		if c.local {
			u.Path = path.Join(u.Path, "/_nodes/_local/stats")
		} else {
			u.Path = path.Join(u.Path, "/_nodes/stats")
		}
	} else {
		if c.local {
			u.Path = path.Join(u.Path, "/_nodes/_local", c.node, "stats")
		} else {
			u.Path = path.Join(u.Path, "/_nodes", c.node, "stats")
		}
	}
	if len(c.nodeStats) != 0 {
		u.Path = fmt.Sprintf("%s/%s", u.Path, strings.Join(c.nodeStats, ","))
	}

	res, err := c.client.Get(u.String())
	if err != nil {
		return nsr, fmt.Errorf("failed to get cluster health from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return nsr, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		c.jsonParseFailures.Inc()
		return nsr, err
	}

	if err := json.Unmarshal(bts, &nsr); err != nil {
		c.jsonParseFailures.Inc()
		return nsr, err
	}
	return nsr, nil
}

func GetNodeID(client *http.Client, user, password, s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %s: %s", s, err)
	}
	if user != "" && password != "" {
		u.User = url.UserPassword(user, password)
	}
	var nsr nodeStatsResponse
	u.Path = path.Join(u.Path, "/_nodes/_local/name")
	res, err := client.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to get node ID from %s: %s", u.String(), err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(bts, &nsr); err != nil {
		return "", err
	}

	// Only 1 should be returned
	for id := range nsr.Nodes {
		return id, nil
	}
	return "", nil
}

func GetCatMaster(client *http.Client, user, password, s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %s: %s", s, err)
	}
	if user != "" && password != "" {
		u.User = url.UserPassword(user, password)
	}
	u.Path = path.Join(u.Path, "/_cat/master")
	res, err := client.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to get node ID from %s: %s", u.String(), err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		// NOTE: we are not going to read/discard r.Body under the assumption we'd prefer
		// to let the underlying transport close the connection and re-establish a new one for
		// future calls.
		return "", fmt.Errorf("elasticsearch: Unable to retrieve master node information. API responded with status-code %d, expected %d", res.StatusCode, http.StatusOK)
	}
	response, err := io.ReadAll(res.Body)

	if err != nil {
		return "", err
	}

	masterID := strings.Split(string(response), " ")[0]

	return masterID, nil
}

// Collect gets nodes metric values
func (c *Nodes) Collect(ch chan<- prometheus.Metric) {
	c.totalScrapes.Inc()
	defer func() {
		ch <- c.up
		ch <- c.totalScrapes
		ch <- c.jsonParseFailures
	}()

	nodeStatsResp, err := c.fetchAndDecodeNodeStats()
	if err != nil {
		c.up.Set(0)
		log.Println("failed to fetch and decode node stats, err: ", err)
		return
	}
	c.up.Set(1)

	for _, node := range nodeStatsResp.Nodes {
		// Handle the node labels metric
		roles := getRoles(node)

		for role, roleEnabled := range roles {
			if roleEnabled {
				metric := createRoleMetric(role)
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(node),
					metric.Labels(nodeStatsResp.ClusterName, node)...,
				)
			}
		}

		if isEnable("jvm", c.nodeStats) {
			for _, metric := range c.jvmMetrics {
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(node),
					metric.Labels(nodeStatsResp.ClusterName, node)...,
				)
			}

			// GC Stats
			for collector, gcStats := range node.JVM.GC.Collectors {
				for _, metric := range c.gcCollectionMetrics {
					ch <- prometheus.MustNewConstMetric(
						metric.Desc,
						metric.Type,
						metric.Value(gcStats),
						metric.Labels(nodeStatsResp.ClusterName, node, collector)...,
					)
				}
			}
		}

		if isEnable("breaker", c.nodeStats) {
			// Breaker stats
			for breaker, bstats := range node.Breakers {
				for _, metric := range c.breakerMetrics {
					ch <- prometheus.MustNewConstMetric(
						metric.Desc,
						metric.Type,
						metric.Value(bstats),
						metric.Labels(nodeStatsResp.ClusterName, node, breaker)...,
					)
				}
			}
		}

		if isEnable("process", c.nodeStats) {
			// Process Stats
			for _, metric := range c.processMetrics {
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(node),
					metric.Labels(nodeStatsResp.ClusterName, node)...,
				)
			}
		}

		if isEnable("os", c.nodeStats) {
			// OS Stats
			for _, metric := range c.osMetrics {
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(node),
					metric.Labels(nodeStatsResp.ClusterName, node)...,
				)
			}
		}

		if isEnable("fs", c.nodeStats) {
			// File System Data Stats
			for _, fsDataStats := range node.FS.Data {
				for _, metric := range c.filesystemDataMetrics {
					ch <- prometheus.MustNewConstMetric(
						metric.Desc,
						metric.Type,
						metric.Value(fsDataStats),
						metric.Labels(nodeStatsResp.ClusterName, node, fsDataStats.Mount, fsDataStats.Path)...,
					)
				}
			}

			// File System IO Device Stats
			for _, fsIODeviceStats := range node.FS.IOStats.Devices {
				for _, metric := range c.filesystemIODeviceMetrics {
					ch <- prometheus.MustNewConstMetric(
						metric.Desc,
						metric.Type,
						metric.Value(fsIODeviceStats),
						metric.Labels(nodeStatsResp.ClusterName, node, fsIODeviceStats.DeviceName)...,
					)
				}
			}
		}

		if isEnable("indices", c.nodeStats) {
			// Indices Stats
			for _, metric := range c.indicesMetrics {
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(node),
					metric.Labels(nodeStatsResp.ClusterName, node)...,
				)
			}
		}

		if isEnable("thread_pool", c.nodeStats) {
			// Thread Pool stats
			for pool, pstats := range node.ThreadPool {
				for _, metric := range c.threadPoolMetrics {
					ch <- prometheus.MustNewConstMetric(
						metric.Desc,
						metric.Type,
						metric.Value(pstats),
						metric.Labels(nodeStatsResp.ClusterName, node, pool)...,
					)
				}
			}
		}

		if isEnable("transport", c.nodeStats) {
			// Transport Stats
			for _, metric := range c.transportMetrics {
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(node),
					metric.Labels(nodeStatsResp.ClusterName, node)...,
				)
			}
		}

		if isEnable("http", c.nodeStats) {
			// HTTP Stats
			for _, metric := range c.httpMetrics {
				ch <- prometheus.MustNewConstMetric(
					metric.Desc,
					metric.Type,
					metric.Value(node),
					metric.Labels(nodeStatsResp.ClusterName, node)...,
				)
			}
		}
	}
}

func isEnable(stat string, stats []string) bool {
	for _, s := range stats {
		if s == stat {
			return true
		}
	}
	return false
}
