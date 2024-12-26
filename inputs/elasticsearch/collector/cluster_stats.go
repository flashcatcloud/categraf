package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"

	"github.com/prometheus/client_golang/prometheus"
)

type clusterStatsMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(clusterHealth ClusterStatsResponse) float64
	Labels func(cluster ClusterStatsResponse) []string
}

// ClusterStats type defines the collector struct
type ClusterStats struct {
	client *http.Client
	url    *url.URL

	metrics []*clusterStatsMetric
}

var (
	defaultClusterStatsLabels      = []string{"cluster", "node", "status"}
	defaultClusterStatsLabelValues = func(cluster ClusterStatsResponse) []string {
		return []string{
			cluster.ClusterName,
			cluster.NodeName,
			cluster.Status,
		}
	}
)

// NewClusterStats defines Nodes Prometheus metrics
func NewClusterStats(client *http.Client, url *url.URL) *ClusterStats {
	return &ClusterStats{
		client: client,
		url:    url,

		metrics: []*clusterStatsMetric{
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "count"),
					"Completion in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Count)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "completion_size_in_bytes"),
					"Completion in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Completion.Size)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "docs_count"),
					"Count of documents on this cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Docs.Count)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "docs_deleted"),
					"Count of deleted documents on this cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Docs.Deleted)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "fielddata_evictions"),
					"Evictions from field data",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.FieldData.Evictions)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "fielddata_memory_size_in_bytes"),
					"Field data cache memory usage in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.FieldData.MemorySize)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "query_cache_cache_count"),
					"Query cache cache count",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.QueryCache.CacheCount)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "query_cache_cache_size"),
					"Query cache cache size",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.QueryCache.CacheSize)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "query_cache_evictions"),
					"Evictions from query cache",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.QueryCache.Evictions)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "query_cache_hit_count"),
					"Query cache count",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.QueryCache.HitCount)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "query_cache_memory_size_in_bytes"),
					"Query cache memory usage in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.QueryCache.MemorySize)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "query_cache_miss_count"),
					"Query miss count",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.QueryCache.MissCount)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.CounterValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "query_cache_total_count"),
					"Query cache total count",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.QueryCache.TotalCount)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_count"),
					"Count of index segments on this cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.Count)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_doc_values_memory_in_bytes"),
					"Count of doc values memory",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.DocValuesMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_fixed_bit_set_memory_in_bytes"),
					"Count of fixed bit set",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.FixedBitSet)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_index_writer_memory_in_bytes"),
					"Count of memory for index writer on this cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.IndexWriterMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_max_unsafe_auto_id_timestamp"),
					"Count of memory for index writer on this cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.MaxUnsafeAutoIDTimestamp)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_memory_in_bytes"),
					"Current memory size of segments in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.Memory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_norms_memory_in_bytes"),
					"Count of memory used by norms",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.NormsMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_points_memory_in_bytes"),
					"Point values memory usage in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.PointsMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_stored_fields_memory_in_bytes"),
					"Count of stored fields memory",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.StoredFieldsMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_term_vectors_memory_in_bytes"),
					"Term vectors memory usage in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.TermVectorsMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_terms_memory_in_bytes"),
					"Count of terms in memory for this cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.TermsMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "segments_version_map_memory_in_bytes"),
					"Version map memory usage in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Segments.VersionMapMemory)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_total"),
					"Total number of shards in the cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Total
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_replication"),
					"Number of shards replication",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Replication
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_primaries"),
					"Number of primary shards in the cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Primaries
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_primaries_avg"),
					"Average number of primary shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Primaries.Avg
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_primaries_max"),
					"Max number of primary shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Primaries.Max
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_primaries_min"),
					"Min number of primary shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Primaries.Min
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_replication_avg"),
					"Average number of replication shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Replicas.Avg
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_replication_max"),
					"Max number of replication shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Replicas.Max
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_replication_min"),
					"Min number of replication shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Replicas.Min
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_shards_avg"),
					"Average number of shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Shards.Avg
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_shards_max"),
					"Max number of shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Shards.Max
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "shards_index_shards_min"),
					"Min number of shards per index",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Indices.Shards.Index.Shards.Min
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "store_size_in_bytes"),
					"Current size of the store in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Store.Size)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "total_data_set_size_in_bytes"),
					"Total data set size in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Store.TotalDataSetSize)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_indices", "reserved_in_bytes"),
					"Reserved size in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Indices.Store.Reserved)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "count_coordinating_only"),
					"Count of coordinating only nodes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.Count.CoordinatingOnly)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "count_data"),
					"Count of data nodes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.Count.Data)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "count_ingest"),
					"Count of ingest nodes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.Count.Ingest)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "count_master"),
					"Count of master nodes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.Count.Master)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "count_total"),
					"Total count of nodes in the cluster",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.Count.Total)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "fs_available_in_bytes"),
					"Available disk space in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.FS.AvailableInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "fs_free_in_bytes"),
					"Free disk space in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.FS.FreeInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "fs_total_in_bytes"),
					"Total disk space in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.FS.TotalInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "jvm_max_uptime_seconds"),
					"Max uptime in milliseconds",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.JVM.MaxUptimeInMillis) / 1000
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "jvm_mem_heap_max_in_bytes"),
					"Max heap memory in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.JVM.Mem.HeapMaxInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "jvm_mem_heap_used_in_bytes"),
					"Used heap memory in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.JVM.Mem.HeapUsedInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "jvm_threads"),
					"Number of threads",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.JVM.Threads)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "network_types_http_types_security4"),
					"HTTP security4 network types",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.NetWorkTypes.HTTPTypes.Security)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "network_types_transport_types_security4"),
					"Transport security4 network types",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.NetWorkTypes.TransportTypes.Security)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "os_allocated_processors"),
					"Allocated processors",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.OS.AllocatedProcessors)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "os_available_processors"),
					"Available processors",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.OS.AvailableProcessors)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "os_mem_free_in_bytes"),
					"Free memory in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.OS.Mem.FreeInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "os_mem_free_percent"),
					"Free memory in percent",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.OS.Mem.FreePercent)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "os_mem_total_in_bytes"),
					"Total memory in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.OS.Mem.TotalInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "os_mem_used_in_bytes"),
					"Used memory in bytes",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.OS.Mem.UsedInBytes)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "os_mem_used_percent"),
					"Used memory in percent",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.OS.Mem.UsedPercent)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "process_cpu_percent"),
					"Process CPU in percent",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return float64(cluster.Nodes.Process.CPU.Percent)
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "process_open_file_descriptors_avg"),
					"Average number of open file descriptors",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Nodes.Process.OpenFileDescriptors.Avg
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "process_open_file_descriptors_max"),
					"Max number of open file descriptors",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Nodes.Process.OpenFileDescriptors.Max
				},
				Labels: defaultClusterStatsLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "clusterstats_nodes", "process_open_file_descriptors_min"),
					"Min number of open file descriptors",
					defaultClusterStatsLabels, nil,
				),
				Value: func(cluster ClusterStatsResponse) float64 {
					return cluster.Nodes.Process.OpenFileDescriptors.Min
				},
				Labels: defaultClusterStatsLabelValues,
			},
		},
	}
}

// Describe add metrics descriptions
func (c *ClusterStats) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metrics {
		ch <- metric.Desc
	}
}

func (c *ClusterStats) fetchAndDecodeClusterStats() (ClusterStatsResponse, error) {
	var chr ClusterStatsResponse

	u := *c.url
	u.Path = path.Join(u.Path, "/_cluster/stats")
	res, err := c.client.Get(u.String())
	if err != nil {
		return chr, fmt.Errorf("failed to get cluster stats from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return chr, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return chr, err
	}

	if err := json.Unmarshal(bts, &chr); err != nil {
		return chr, err
	}

	return chr, nil
}

// Collect gets clusters metric values
func (c *ClusterStats) Collect(ch chan<- prometheus.Metric) {
	clusterStatsResp, err := c.fetchAndDecodeClusterStats()
	if err != nil {
		log.Println("failed to fetch and decode cluster health, err: ", err)
		return
	}

	for _, metric := range c.metrics {
		ch <- prometheus.MustNewConstMetric(
			metric.Desc,
			metric.Type,
			metric.Value(clusterStatsResp),
			defaultClusterStatsLabelValues(clusterStatsResp)...,
		)
	}
}
