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
	"strconv"
	"strings"

	"github.com/imdario/mergo"
	"github.com/prometheus/client_golang/prometheus"
)

type clusterSettingsMetric struct {
	Type  prometheus.ValueType
	Desc  *prometheus.Desc
	Value func(clusterSettings ClusterSettingsResponse) float64
}

// ClusterSettings information struct
type ClusterSettings struct {
	client *http.Client
	url    *url.URL

	metrics []*clusterSettingsMetric

	up                              prometheus.Gauge
	shardAllocationEnabled          prometheus.Gauge
	maxShardsPerNode                prometheus.Gauge
	totalScrapes, jsonParseFailures prometheus.Counter
}

var (
	shardAllocationMap = map[string]int{
		"all":           0,
		"primaries":     1,
		"new_primaries": 2,
		"none":          3,
	}
	// Threshold enabled
	thresholdMap = map[string]int{
		"false": 0,
		"true":  1,
	}
)

// NewClusterSettings defines Cluster Settings Prometheus metrics
func NewClusterSettings(client *http.Client, url *url.URL) *ClusterSettings {
	return &ClusterSettings{
		client: client,
		url:    url,

		metrics: []*clusterSettingsMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "threshold_enabled"),
					"Is disk allocation decider enabled.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					return float64(thresholdMap[clusterSettings.Cluster.Routing.Allocation.Disk.ThresholdEnabled])
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "allocation_threshold_enabled"),
					"Current mode of cluster wide shard routing allocation settings.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					return float64(shardAllocationMap[clusterSettings.Cluster.Routing.Allocation.Enabled])
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "allocation_watermark_flood_stage_ratio"),
					"Current mode of cluster wide shard routing allocation settings.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					floodStage, err := getValueAsRatio(clusterSettings.Cluster.Routing.Allocation.Disk.Watermark.FloodStage)
					if err != nil {
						return 0
					}
					return floodStage
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "allocation_watermark_high_ratio"),
					"Current mode of cluster wide shard routing allocation settings.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					high, err := getValueAsRatio(clusterSettings.Cluster.Routing.Allocation.Disk.Watermark.High)
					if err != nil {
						return 0
					}
					return high
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "allocation_watermark_low_ratio"),
					"Current mode of cluster wide shard routing allocation settings.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					low, err := getValueAsRatio(clusterSettings.Cluster.Routing.Allocation.Disk.Watermark.Low)
					if err != nil {
						return 0
					}
					return low
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "allocation_watermark_flood_stage_bytes"),
					"Current mode of cluster wide shard routing allocation settings.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					floodStage, err := getValueInBytes(clusterSettings.Cluster.Routing.Allocation.Disk.Watermark.FloodStage)
					if err != nil {
						return 0
					}
					return floodStage
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "allocation_watermark_high_bytes"),
					"Current mode of cluster wide shard routing allocation settings.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					high, err := getValueInBytes(clusterSettings.Cluster.Routing.Allocation.Disk.Watermark.High)
					if err != nil {
						return 0
					}
					return high
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clustersettings_stats", "allocation_watermark_low_bytes"),
					"Current mode of cluster wide shard routing allocation settings.",
					nil, nil,
				),
				Value: func(clusterSettings ClusterSettingsResponse) float64 {
					low, err := getValueInBytes(clusterSettings.Cluster.Routing.Allocation.Disk.Watermark.Low)
					if err != nil {
						return 0
					}
					return low
				},
			},
		},

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "up"),
			Help: "Was the last scrape of the Elasticsearch cluster settings endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "total_scrapes"),
			Help: "Current total Elasticsearch cluster settings scrapes.",
		}),
		shardAllocationEnabled: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "shard_allocation_enabled"),
			Help: "Current mode of cluster wide shard routing allocation settings.",
		}),
		maxShardsPerNode: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "max_shards_per_node"),
			Help: "Current maximum number of shards per node setting.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "clustersettings_stats", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),
	}
}

// Describe add Snapshots metrics descriptions
func (cs *ClusterSettings) Describe(ch chan<- *prometheus.Desc) {
	ch <- cs.up.Desc()
	ch <- cs.totalScrapes.Desc()
	ch <- cs.shardAllocationEnabled.Desc()
	ch <- cs.maxShardsPerNode.Desc()
	ch <- cs.jsonParseFailures.Desc()
	for _, metric := range cs.metrics {
		ch <- metric.Desc
	}
}

func (cs *ClusterSettings) getAndParseURL(u *url.URL, data interface{}) error {
	res, err := cs.client.Get(u.String())
	if err != nil {
		return fmt.Errorf("failed to get from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		cs.jsonParseFailures.Inc()
		return err
	}

	if err := json.Unmarshal(bts, data); err != nil {
		cs.jsonParseFailures.Inc()
		return err
	}

	return nil
}

func (cs *ClusterSettings) fetchAndDecodeClusterSettingsStats() (ClusterSettingsResponse, error) {

	u := *cs.url
	u.Path = path.Join(u.Path, "/_cluster/settings")
	q := u.Query()
	q.Set("include_defaults", "true")
	u.RawQuery = q.Encode()
	u.RawPath = q.Encode()
	var csfr ClusterSettingsFullResponse
	var csr ClusterSettingsResponse
	err := cs.getAndParseURL(&u, &csfr)
	if err != nil {
		return csr, err
	}
	err = mergo.Merge(&csr, csfr.Defaults, mergo.WithOverride)
	if err != nil {
		return csr, err
	}
	err = mergo.Merge(&csr, csfr.Persistent, mergo.WithOverride)
	if err != nil {
		return csr, err
	}
	err = mergo.Merge(&csr, csfr.Transient, mergo.WithOverride)

	return csr, err
}

// Collect gets cluster settings  metric values
func (cs *ClusterSettings) Collect(ch chan<- prometheus.Metric) {

	cs.totalScrapes.Inc()
	defer func() {
		ch <- cs.up
		ch <- cs.totalScrapes
		ch <- cs.jsonParseFailures
		ch <- cs.shardAllocationEnabled
		ch <- cs.maxShardsPerNode
	}()

	csr, err := cs.fetchAndDecodeClusterSettingsStats()
	if err != nil {
		cs.shardAllocationEnabled.Set(0)
		cs.up.Set(0)
		log.Println("failed to fetch and decode cluster settings stats, err: ", err)
		return
	}
	cs.up.Set(1)

	shardAllocationMap := map[string]int{
		"all":           0,
		"primaries":     1,
		"new_primaries": 2,
		"none":          3,
	}

	cs.shardAllocationEnabled.Set(float64(shardAllocationMap[csr.Cluster.Routing.Allocation.Enabled]))

	if maxShardsPerNodeString, ok := csr.Cluster.MaxShardsPerNode.(string); ok {
		maxShardsPerNode, err := strconv.ParseInt(maxShardsPerNodeString, 10, 64)
		if err == nil {
			cs.maxShardsPerNode.Set(float64(maxShardsPerNode))
		}
	}

	for _, metric := range cs.metrics {
		ch <- prometheus.MustNewConstMetric(
			metric.Desc,
			metric.Type,
			metric.Value(csr),
		)
	}
}

func getValueInBytes(value string) (float64, error) {
	type UnitValue struct {
		unit string
		val  float64
	}

	unitValues := []UnitValue{
		{"pb", 1024 * 1024 * 1024 * 1024 * 1024},
		{"tb", 1024 * 1024 * 1024 * 1024},
		{"gb", 1024 * 1024 * 1024},
		{"mb", 1024 * 1024},
		{"kb", 1024},
		{"b", 1},
	}

	for _, uv := range unitValues {
		if strings.HasSuffix(value, uv.unit) {
			numberStr := strings.TrimSuffix(value, uv.unit)

			number, err := strconv.ParseFloat(numberStr, 64)
			if err != nil {
				return 0, err
			}
			return number * uv.val, nil
		}
	}

	return 0, fmt.Errorf("failed to convert unit %s to bytes", value)
}

func getValueAsRatio(value string) (float64, error) {
	if strings.HasSuffix(value, "%") {
		percentValue, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(value, "%")))
		if err != nil {
			return 0, err
		}

		return float64(percentValue) / 100, nil
	}

	ratio, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}

	return ratio, nil
}
