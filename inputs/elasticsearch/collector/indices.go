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
	"flashcat.cloud/categraf/pkg/filter"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/inputs/elasticsearch/pkg/clusterinfo"
)

type labels struct {
	keys   func(...string) []string
	values func(*clusterinfo.Response, ...string) []string
}

type indexMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(indexStats IndexStatsIndexResponse) float64
	Labels labels
}

type shardMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(data IndexStatsIndexShardsDetailResponse) float64
	Labels labels
}

type aliasMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func() float64
	Labels labels
}

// Indices information struct
type Indices struct {
	client               *http.Client
	url                  *url.URL
	shards               bool
	aliases              bool
	indicesIncluded      []string
	numMostRecentIndices int
	indexMatchers        map[string]filter.Filter
	clusterInfoCh        chan *clusterinfo.Response
	lastClusterInfo      *clusterinfo.Response

	up                prometheus.Gauge
	totalScrapes      prometheus.Counter
	jsonParseFailures prometheus.Counter

	indexMetrics []*indexMetric
	shardMetrics []*shardMetric
	aliasMetrics []*aliasMetric
}

// NewIndices defines Indices Prometheus metrics
func NewIndices(client *http.Client, url *url.URL, shards bool, includeAliases bool, indicesIncluded []string, numMostRecentIndices int, indexMatchers map[string]filter.Filter) *Indices {

	indexLabels := labels{
		keys: func(...string) []string {
			return []string{"index", "cluster"}
		},
		values: func(lastClusterinfo *clusterinfo.Response, s ...string) []string {
			if lastClusterinfo != nil {
				return append(s, lastClusterinfo.ClusterName)
			}
			// this shouldn't happen, as the clusterinfo Retriever has a blocking
			// Run method. It blocks until the first clusterinfo call has succeeded
			return append(s, "unknown_cluster")
		},
	}

	shardLabels := labels{
		keys: func(...string) []string {
			return []string{"index", "shard", "node", "primary", "cluster"}
		},
		values: func(lastClusterinfo *clusterinfo.Response, s ...string) []string {
			if lastClusterinfo != nil {
				return append(s, lastClusterinfo.ClusterName)
			}
			// this shouldn't happen, as the clusterinfo Retriever has a blocking
			// Run method. It blocks until the first clusterinfo call has succeeded
			return append(s, "unknown_cluster")
		},
	}

	aliasLabels := labels{
		keys: func(...string) []string {
			return []string{"index", "alias", "cluster"}
		},
		values: func(lastClusterinfo *clusterinfo.Response, s ...string) []string {
			if lastClusterinfo != nil {
				return append(s, lastClusterinfo.ClusterName)
			}
			// this shouldn't happen, as the clusterinfo Retriever has a blocking
			// Run method. It blocks until the first clusterinfo call has succeeded
			return append(s, "unknown_cluster")
		},
	}

	indices := &Indices{
		client:               client,
		url:                  url,
		shards:               shards,
		aliases:              includeAliases,
		indicesIncluded:      indicesIncluded,
		numMostRecentIndices: numMostRecentIndices,
		indexMatchers:        indexMatchers,
		clusterInfoCh:        make(chan *clusterinfo.Response),
		lastClusterInfo: &clusterinfo.Response{
			ClusterName: "unknown_cluster",
		},

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "index_stats", "up"),
			Help: "Was the last scrape of the Elasticsearch index endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "index_stats", "total_scrapes"),
			Help: "Current total Elasticsearch index scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, "index_stats", "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),

		indexMetrics: []*indexMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "docs_count"),
					"Total count of documents",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Docs.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "docs_deleted"),
					"Total count of deleted documents",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Docs.Deleted)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "store_size_in_bytes"),
					"Current total size of stored index data in bytes with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Store.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "throttle_time_seconds"),
					"Total time the index has been throttled in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Store.ThrottleTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_count"),
					"Current number of segments with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_memory_in_bytes"),
					"Current size of segments with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.MemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_terms_memory_in_bytes"),
					"Current number of terms with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.TermsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_stored_fields_memory_in_bytes"),
					"Current size of fields with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.StoredFieldsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_term_vectors_memory_in_bytes"),
					"Current size of term vectors with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.TermVectorsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_norms_memory_in_bytes"),
					"Current size of norms with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.NormsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_points_memory_in_bytes"),
					"Current size of points with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.PointsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_doc_values_memory_in_bytes"),
					"Current size of doc values with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.DocValuesMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_index_writer_memory_in_bytes"),
					"Current size of index writer with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.IndexWriterMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_version_map_memory_in_bytes"),
					"Current size of version map with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.VersionMapMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_fixed_bit_set_memory_in_bytes"),
					"Current size of fixed bit with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.FixedBitSetMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "segments_max_unsafe_auto_id_timestamp"),
					"Current max unsafe auto id timestamp with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Segments.MaxUnsafeAutoIDTimestamp)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "translog_earliest_last_modified_age"),
					"Current earliest last modified age with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Translog.EarliestLastModifiedAge)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "translog_operations"),
					"Current number of operations in the transaction log with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Translog.Operations)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "translog_size_in_bytes"),
					"Current size of transaction log with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Translog.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "translog_uncommitted_operations"),
					"Current number of uncommitted operations in the transaction log with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Translog.UncommittedOperations)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "translog_uncommitted_size_in_bytes"),
					"Current size of uncommitted transaction log with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Translog.UncommittedSizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "completion_size_in_bytes"),
					"Current size of completion with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Completion.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_query_time_seconds"),
					"Total search query time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.QueryTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_query_current"),
					"The number of currently active queries",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.QueryCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_open_contexts"),
					"Total number of open search contexts",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.OpenContexts)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_query_total"),
					"Total number of queries",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.QueryTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_fetch_time_seconds"),
					"Total search fetch time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.FetchTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_fetch"),
					"Total search fetch count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.FetchTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_fetch_current"),
					"Current search fetch count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.FetchCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_scroll_time_seconds"),
					"Total search scroll time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.ScrollTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_scroll_current"),
					"Current search scroll count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.ScrollCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_scroll"),
					"Total search scroll count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.ScrollTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_suggest_time_seconds"),
					"Total search suggest time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.SuggestTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_suggest_total"),
					"Total search suggest count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.SuggestTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "search_suggest_current"),
					"Current search suggest count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Search.SuggestCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "indexing_index_time_seconds"),
					"Total indexing index time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.IndexTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "index_current"),
					"The number of documents currently being indexed to an index",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.IndexCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "index_failed"),
					"Total indexing index failed count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.IndexFailed)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "delete_current"),
					"The number of delete operations currently being processed",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.DeleteCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "indexing_index"),
					"Total indexing index count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.IndexTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "indexing_delete_time_seconds"),
					"Total indexing delete time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.DeleteTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "indexing_delete"),
					"Total indexing delete count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.DeleteTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "indexing_noop_update"),
					"Total indexing no-op update count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.NoopUpdateTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "indexing_throttle_time_seconds"),
					"Total indexing throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Indexing.ThrottleTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "get_time_seconds"),
					"Total get time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.TimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "get_exists_total"),
					"Total exists count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.ExistsTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "get_exists_time_seconds"),
					"Total exists time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.ExistsTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "get_total"),
					"Total get count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "get_missing_total"),
					"Total missing count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.MissingTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "get_missing_time_seconds"),
					"Total missing time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.MissingTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "get_current"),
					"Current get count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Get.Current)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_time_seconds"),
					"Total merge time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_total"),
					"Total merge count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_total_docs"),
					"Total merge docs count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalDocs)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_total_size_in_bytes"),
					"Total merge size in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalSizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_current"),
					"Current merge count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.Current)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_current_docs"),
					"Current merge docs count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.CurrentDocs)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_current_size_in_bytes"),
					"Current merge size in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.CurrentSizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_total_throttle_time_seconds"),
					"Total merge I/O throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalThrottledTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_total_stopped_time_seconds"),
					"Total large merge stopped time in seconds, allowing smaller merges to complete",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalStoppedTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "merges_total_auto_throttle_bytes"),
					"Total bytes that were auto-throttled during merging",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Merges.TotalAutoThrottleInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "refresh_external_total_time_seconds"),
					"Total external refresh time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Refresh.ExternalTotalTime) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "refresh_external_total"),
					"Total external refresh count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Refresh.ExternalTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "refresh_total_time_seconds"),
					"Total refresh time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Refresh.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "refresh_total"),
					"Total refresh count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Refresh.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "refresh_listeners"),
					"Total number of refresh listeners",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Refresh.Listeners)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "recovery_current_as_source"),
					"Current number of recovery as source",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Recovery.CurrentAsSource)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "recovery_current_as_target"),
					"Current number of recovery as target",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Recovery.CurrentAsTarget)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "recovery_throttle_time_seconds"),
					"Total recovery throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Recovery.ThrottleTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "flush_time_seconds_total"),
					"Total flush time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Flush.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "flush_total"),
					"Total flush count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Flush.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "flush_periodic"),
					"Total periodic flush count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Flush.Periodic)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "warmer_time_seconds_total"),
					"Total warmer time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Warmer.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "warmer_total"),
					"Total warmer count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Warmer.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "query_cache_memory_in_bytes"),
					"Total query cache memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "query_cache_size"),
					"Total query cache size",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.CacheSize)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "query_cache_total_count"),
					"Total query cache count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.TotalCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "query_cache_hit_count"),
					"Total query cache hits count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.HitCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "query_cache_miss_count"),
					"Total query cache misses count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.MissCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "query_cache_cache_count"),
					"Total query cache caches count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.CacheCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "query_cache_evictions"),
					"Total query cache evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.QueryCache.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "request_cache_memory_in_bytes"),
					"Total request cache memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "request_cache_hit_count"),
					"Total request cache hits count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.HitCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "request_cache_miss_count"),
					"Total request cache misses count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.MissCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "request_cache_evictions"),
					"Total request cache evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.RequestCache.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "fielddata_memory_in_bytes"),
					"Total fielddata memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Fielddata.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "fielddata_evictions"),
					"Total fielddata evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Fielddata.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "seq_no_global_checkpoint"),
					"Global checkpoint",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Seq.GlobalCheckpoint)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "seq_no_local_checkpoint"),
					"Local checkpoint",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Seq.LocalCheckpoint)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_total", "seq_no_max_seq_no"),
					"Max sequence number",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Total.Seq.MaxSeqNo)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "docs_count"),
					"Total count of documents",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Docs.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "docs_deleted"),
					"Total count of deleted documents",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Docs.Deleted)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "store_size_in_bytes"),
					"Current total size of stored index data in bytes with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Store.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "throttle_time_seconds"),
					"Total time the index has been throttled in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Store.ThrottleTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_count"),
					"Current number of segments with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.Count)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_memory_in_bytes"),
					"Current size of segments with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.MemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_terms_memory_in_bytes"),
					"Current number of terms with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.TermsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_stored_fields_memory_in_bytes"),
					"Current size of fields with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.StoredFieldsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_term_vectors_memory_in_bytes"),
					"Current size of term vectors with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.TermVectorsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_norms_memory_in_bytes"),
					"Current size of norms with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.NormsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_points_memory_in_bytes"),
					"Current size of points with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.PointsMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_doc_values_memory_in_bytes"),
					"Current size of doc values with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.DocValuesMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_index_writer_memory_in_bytes"),
					"Current size of index writer with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.IndexWriterMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_version_map_memory_in_bytes"),
					"Current size of version map with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.VersionMapMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_fixed_bit_set_memory_in_bytes"),
					"Current size of fixed bit with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.FixedBitSetMemoryInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "segments_max_unsafe_auto_id_timestamp"),
					"Current max unsafe auto id timestamp with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Segments.MaxUnsafeAutoIDTimestamp)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "translog_earliest_last_modified_age"),
					"Current earliest last modified age with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Translog.EarliestLastModifiedAge)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "translog_operations"),
					"Current number of operations in the transaction log with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Translog.Operations)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "translog_size_in_bytes"),
					"Current size of transaction log with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Translog.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "translog_uncommitted_operations"),
					"Current number of uncommitted operations in the transaction log with all shards on all nodes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Translog.UncommittedOperations)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "translog_uncommitted_size_in_bytes"),
					"Current size of uncommitted transaction log with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Translog.UncommittedSizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "completion_size_in_bytes"),
					"Current size of completion with all shards on all nodes in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Completion.SizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_query_time_seconds"),
					"Total search query time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.QueryTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_query_current"),
					"The number of currently active queries",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.QueryCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_open_contexts"),
					"Total number of open search contexts",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.OpenContexts)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_query_total"),
					"Total number of queries",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.QueryTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_fetch_time_seconds"),
					"Total search fetch time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.FetchTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_fetch"),
					"Total search fetch count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.FetchTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_fetch_current"),
					"Current search fetch count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.FetchCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_scroll_time_seconds"),
					"Total search scroll time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.ScrollTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_scroll_current"),
					"Current search scroll count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.ScrollCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_scroll"),
					"Total search scroll count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.ScrollTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_suggest_time_seconds"),
					"Total search suggest time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.SuggestTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_suggest_total"),
					"Total search suggest count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.SuggestTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "search_suggest_current"),
					"Current search suggest count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Search.SuggestCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "indexing_index_time_seconds"),
					"Total indexing index time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.IndexTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "index_current"),
					"The number of documents currently being indexed to an index",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.IndexCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "index_failed"),
					"Total indexing index failed count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.IndexFailed)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "delete_current"),
					"The number of delete operations currently being processed",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.DeleteCurrent)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "indexing_index"),
					"Total indexing index count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.IndexTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "indexing_delete_time_seconds"),
					"Total indexing delete time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.DeleteTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "indexing_delete"),
					"Total indexing delete count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.DeleteTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "indexing_noop_update"),
					"Total indexing no-op update count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.NoopUpdateTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "indexing_throttle_time_seconds"),
					"Total indexing throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Indexing.ThrottleTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "get_time_seconds"),
					"Total get time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Get.TimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "get_exists_total"),
					"Total exists count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Get.ExistsTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "get_exists_time_seconds"),
					"Total exists time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Get.ExistsTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "get_total"),
					"Total get count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Get.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "get_missing_total"),
					"Total missing count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Get.MissingTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "get_missing_time_seconds"),
					"Total missing time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Get.MissingTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "get_current"),
					"Current get count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Get.Current)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_time_seconds"),
					"Total merge time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_total"),
					"Total merge count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_total_docs"),
					"Total merge docs count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.TotalDocs)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_total_size_in_bytes"),
					"Total merge size in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.TotalSizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_current"),
					"Current merge count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.Current)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_current_docs"),
					"Current merge docs count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.CurrentDocs)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_current_size_in_bytes"),
					"Current merge size in bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.CurrentSizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_total_throttle_time_seconds"),
					"Total merge I/O throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.TotalThrottledTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_total_stopped_time_seconds"),
					"Total large merge stopped time in seconds, allowing smaller merges to complete",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.TotalStoppedTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "merges_total_auto_throttle_bytes"),
					"Total bytes that were auto-throttled during merging",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Merges.TotalAutoThrottleInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "refresh_external_total_time_seconds"),
					"Total external refresh time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Refresh.ExternalTotalTime) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "refresh_external_total"),
					"Total external refresh count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Refresh.ExternalTotal)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "refresh_total_time_seconds"),
					"Total refresh time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Refresh.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "refresh_total"),
					"Total refresh count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Refresh.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "refresh_listeners"),
					"Total number of refresh listeners",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Refresh.Listeners)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "recovery_current_as_source"),
					"Current number of recovery as source",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Recovery.CurrentAsSource)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "recovery_current_as_target"),
					"Current number of recovery as target",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Recovery.CurrentAsTarget)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "recovery_throttle_time_seconds"),
					"Total recovery throttle time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Recovery.ThrottleTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "flush_time_seconds_total"),
					"Total flush time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Flush.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "flush_total"),
					"Total flush count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Flush.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "flush_periodic"),
					"Total periodic flush count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Flush.Periodic)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "warmer_time_seconds_total"),
					"Total warmer time in seconds",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Warmer.TotalTimeInMillis) / 1000
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "warmer_total"),
					"Total warmer count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Warmer.Total)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "query_cache_memory_in_bytes"),
					"Total query cache memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.QueryCache.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "query_cache_size"),
					"Total query cache size",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.QueryCache.CacheSize)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "query_cache_total_count"),
					"Total query cache count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.QueryCache.TotalCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "query_cache_hit_count"),
					"Total query cache hits count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.QueryCache.HitCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "query_cache_miss_count"),
					"Total query cache misses count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.QueryCache.MissCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "query_cache_cache_count"),
					"Total query cache caches count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.QueryCache.CacheCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "query_cache_evictions"),
					"Total query cache evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.QueryCache.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "request_cache_memory_in_bytes"),
					"Total request cache memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.RequestCache.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "request_cache_hit_count"),
					"Total request cache hits count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.RequestCache.HitCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "request_cache_miss_count"),
					"Total request cache misses count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.RequestCache.MissCount)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "request_cache_evictions"),
					"Total request cache evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.RequestCache.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "fielddata_memory_in_bytes"),
					"Total fielddata memory bytes",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Fielddata.MemorySizeInBytes)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "fielddata_evictions"),
					"Total fielddata evictions count",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Fielddata.Evictions)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "seq_no_global_checkpoint"),
					"Global checkpoint",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Seq.GlobalCheckpoint)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "seq_no_local_checkpoint"),
					"Local checkpoint",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Seq.LocalCheckpoint)
				},
				Labels: indexLabels,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats_primaries", "seq_no_max_seq_no"),
					"Max sequence number",
					indexLabels.keys(), nil,
				),
				Value: func(indexStats IndexStatsIndexResponse) float64 {
					return float64(indexStats.Primaries.Seq.MaxSeqNo)
				},
				Labels: indexLabels,
			},
		},
		shardMetrics: []*shardMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats", "shards_docs"),
					"Count of documents on this shard",
					shardLabels.keys(), nil,
				),
				Value: func(data IndexStatsIndexShardsDetailResponse) float64 {
					return float64(data.Docs.Count)
				},
				Labels: shardLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats", "shards_docs_deleted"),
					"Count of deleted documents on this shard",
					shardLabels.keys(), nil,
				),
				Value: func(data IndexStatsIndexShardsDetailResponse) float64 {
					return float64(data.Docs.Deleted)
				},
				Labels: shardLabels,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats", "shards_store_size_in_bytes"),
					"Store size of this shard",
					shardLabels.keys(), nil,
				),
				Value: func(data IndexStatsIndexShardsDetailResponse) float64 {
					return float64(data.Store.SizeInBytes)
				},
				Labels: shardLabels,
			},
		},

		aliasMetrics: []*aliasMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "indices_stats", "aliases"),
					"Record aliases associated with an index",
					aliasLabels.keys(), nil,
				),
				Value: func() float64 {
					return float64(1)
				},
				Labels: aliasLabels,
			},
		},
	}

	// start go routine to fetch clusterinfo updates and save them to lastClusterinfo
	go func() {
		timer := time.NewTimer(2 * time.Minute)
		for {
			select {
			case ci := <-indices.clusterInfoCh:
				if ci != nil {
					log.Println("received cluster info update, cluster: ", ci.ClusterName)
					indices.lastClusterInfo = ci
				}
			case <-timer.C:
				close(indices.clusterInfoCh)
				return
			}
		}
	}()
	return indices
}

// ClusterLabelUpdates returns a pointer to a channel to receive cluster info updates. It implements the
// (not exported) clusterinfo.consumer interface
func (i *Indices) ClusterLabelUpdates() *chan *clusterinfo.Response {
	return &i.clusterInfoCh
}

// String implements the stringer interface. It is part of the clusterinfo.consumer interface
func (i *Indices) String() string {
	return namespace + "indices"
}

// Describe add Indices metrics descriptions
func (i *Indices) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range i.indexMetrics {
		ch <- metric.Desc
	}
	ch <- i.up.Desc()
	ch <- i.totalScrapes.Desc()
	ch <- i.jsonParseFailures.Desc()
}

func (i *Indices) fetchAndDecodeIndexStats() (indexStatsResponse, error) {
	var isr indexStatsResponse

	u := *i.url
	if len(i.indicesIncluded) == 0 {
		u.Path = path.Join(u.Path, "/_all/_stats")
	} else {
		u.Path = path.Join(u.Path, "/"+strings.Join(i.indicesIncluded, ",")+"/_stats")
	}
	if i.shards {
		u.RawQuery = "ignore_unavailable=true&level=shards"
	} else {
		u.RawQuery = "ignore_unavailable=true"
	}

	bts, err := i.queryURL(&u)
	if err != nil {
		return isr, err
	}

	if err := json.Unmarshal(bts, &isr); err != nil {
		i.jsonParseFailures.Inc()
		return isr, err
	}

	//add config i.numMostRecentIndices process code
	isr.Indices = i.gatherIndividualIndicesStats(isr.Indices)

	if i.aliases {
		isr.Aliases = map[string][]string{}
		asr, err := i.fetchAndDecodeAliases()
		if err != nil {
			log.Println("err: ", err.Error())
			return isr, err
		}

		for indexName, aliases := range asr {
			var aliasList []string
			for aliasName := range aliases.Aliases {
				aliasList = append(aliasList, aliasName)
			}

			//add aliases for filtering indexes
			if len(aliasList) > 0 {
				if _, ok := isr.Indices[indexName]; ok {
					sort.Strings(aliasList)
					isr.Aliases[indexName] = aliasList
				}
			}
		}
	}

	return isr, nil
}

func (i *Indices) fetchAndDecodeAliases() (aliasesResponse, error) {
	var asr aliasesResponse

	u := *i.url
	u.Path = path.Join(u.Path, "/_alias")

	bts, err := i.queryURL(&u)
	if err != nil {
		return asr, err
	}

	if err := json.Unmarshal(bts, &asr); err != nil {
		i.jsonParseFailures.Inc()
		return asr, err
	}

	return asr, nil
}

func (i *Indices) queryURL(u *url.URL) ([]byte, error) {
	res, err := i.client.Get(u.String())
	if err != nil {
		return []byte{}, fmt.Errorf("failed to get resource from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return []byte{}, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return []byte{}, err
	}

	return bts, nil
}

// gatherSortedIndicesStats gathers stats for all indices in no particular order.
func (i *Indices) gatherIndividualIndicesStats(indices map[string]IndexStatsIndexResponse) map[string]IndexStatsIndexResponse {

	newIndices := make(map[string]IndexStatsIndexResponse)
	// Sort indices into buckets based on their configured prefix, if any matches.
	categorizedIndexNames := i.categorizeIndices(indices)
	for _, matchingIndices := range categorizedIndexNames {
		// Establish the number of each category of indices to use. User can configure to use only the latest 'X' amount.
		indicesCount := len(matchingIndices)
		indicesToTrackCount := indicesCount

		// Sort the indices if configured to do so.
		if i.numMostRecentIndices > 0 {
			if i.numMostRecentIndices < indicesToTrackCount {
				indicesToTrackCount = i.numMostRecentIndices
			}
			sort.Strings(matchingIndices)
		}

		// Gather only the number of indexes that have been configured, in descending order (most recent, if date-stamped).
		for i := indicesCount - 1; i >= indicesCount-indicesToTrackCount; i-- {
			indexName := matchingIndices[i]

			newIndices[indexName] = indices[indexName]
		}
	}

	return newIndices
}

func (i *Indices) categorizeIndices(indices map[string]IndexStatsIndexResponse) map[string][]string {
	categorizedIndexNames := make(map[string][]string, len(indices))

	// If all indices are configured to be gathered, bucket them all together.
	if len(i.indicesIncluded) == 0 || i.indicesIncluded[0] == "_all" {
		for indexName := range indices {
			categorizedIndexNames["_all"] = append(categorizedIndexNames["_all"], indexName)
		}

		return categorizedIndexNames
	}

	// Bucket each returned index with its associated configured index (if any match).
	for indexName := range indices {
		match := indexName
		for name, matcher := range i.indexMatchers {
			// If a configured index matches one of the returned indexes, mark it as a match.
			if matcher.Match(match) {
				match = name
				break
			}
		}

		// Bucket all matching indices together for sorting.
		categorizedIndexNames[match] = append(categorizedIndexNames[match], indexName)
	}

	return categorizedIndexNames
}

// Collect gets Indices metric values
func (i *Indices) Collect(ch chan<- prometheus.Metric) {
	i.totalScrapes.Inc()
	defer func() {
		ch <- i.up
		ch <- i.totalScrapes
		ch <- i.jsonParseFailures
	}()

	// indices
	indexStatsResp, err := i.fetchAndDecodeIndexStats()
	if err != nil {
		i.up.Set(0)
		log.Println("failed to fetch and decode index stats, err", err)
		return
	}
	i.up.Set(1)

	// Alias stats
	if i.aliases {
		for _, metric := range i.aliasMetrics {
			for indexName, aliases := range indexStatsResp.Aliases {
				for _, alias := range aliases {
					labelValues := metric.Labels.values(i.lastClusterInfo, indexName, alias)

					ch <- prometheus.MustNewConstMetric(
						metric.Desc,
						metric.Type,
						metric.Value(),
						labelValues...,
					)
				}
			}
		}
	}

	// Index stats
	for indexName, indexStats := range indexStatsResp.Indices {
		for _, metric := range i.indexMetrics {
			ch <- prometheus.MustNewConstMetric(
				metric.Desc,
				metric.Type,
				metric.Value(indexStats),
				metric.Labels.values(i.lastClusterInfo, indexName)...,
			)

		}
		if i.shards {
			for _, metric := range i.shardMetrics {
				// gaugeVec := prometheus.NewGaugeVec(metric.Opts, metric.Labels)
				for shardNumber, shards := range indexStats.Shards {
					for _, shard := range shards {
						ch <- prometheus.MustNewConstMetric(
							metric.Desc,
							metric.Type,
							metric.Value(shard),
							metric.Labels.values(i.lastClusterInfo, indexName, shardNumber, shard.Routing.Node, strconv.FormatBool(shard.Routing.Primary))...,
						)
					}
				}
			}
		}
	}
}
