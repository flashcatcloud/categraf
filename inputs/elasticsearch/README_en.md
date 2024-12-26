# Elasticsearch Exporter

#### Elasticsearch 7.x security privileges

Username and password can be passed either directly in the URI or through the `ES_USERNAME` and `ES_PASSWORD` environment variables.
Specifying those two environment variables will override authentication passed in the URI (if any).

ES 7.x supports RBACs. The following security privileges are required for the `elasticsearch` plugin.

| Setting                 | Privilege Required                                                 | Description                                                                                                                                 |
|:------------------------|:-------------------------------------------------------------------|:--------------------------------------------------------------------------------------------------------------------------------------------|
| export_cluster_settings | `cluster` `monitor`                                                |                                                                                                                                             |
| exporter defaults       | `cluster` `monitor`                                                | All cluster read-only operations, like cluster health and state, hot threads, node info, node and cluster stats, and pending cluster tasks. |
| export_indices          | `indices` `monitor` (per index or `*`)                             | All actions that are required for monitoring (recovery, segments info, index stats and status)                                              |
| export_indices_settings | `indices` `monitor` (per index or `*`)                             |                                                                                                                                             |
| export_indices_mappings | `indices` `view_index_metadata` (per index or `*`)                 |                                                                                                                                             |
| export_shards           | not sure if `indices` or `cluster` `monitor` or both               |                                                                                                                                             |
| export_snapshots        | `cluster:admin/snapshot/status` and `cluster:admin/repository/get` | [ES Forum Post](https://discuss.elastic.co/t/permissions-for-backup-user-with-x-pack/88057)                                                 |
| export_slm              | `read_slm`                                                         |                                                                                                                                             |
| export_data_stream      | `monitor` or `manage` (per index or `*`)                           |                                                                                                                                             |

### Differences between the old version of `elastisearch` plugin and the new one

- `elasticsearch_cluster_health_active_shards_percent_as_number` has been changed to `elasticsearch_cluster_health_active_shards_percent`.
- `elasticsearch_cluster_health_status` and `elasticsearch_cluster_health_status_code` have been merged into `elasticsearch_cluster_health_status`, with values being `green=1`, `yellow=2`, `red=3`.
- `elasticsearch_process_cpu_total_in_millis` has been changed to `elasticsearch_process_cpu_seconds_total`, with the unit being seconds.
- `elasticsearch_jvm_uptime_in_millis` has been changed to `elasticsearch_jvm_uptime_seconds`, with the unit being seconds. Similarly, all metrics ending with `*_in_millis` have been changed to `*_seconds`.

### Metrics

#### `cluster_health = true` and `cluster_health_level = "cluster"`

| Name                                                            | Type       | Description                                                                                      |
|-----------------------------------------------------------------|------------|--------------------------------------------------------------------------------------------------|
| `elasticsearch_cluster_health_active_primary_shards`            | GaugeValue | The number of primary shards in your cluster. This is an aggregate total across all indices.     |
| `elasticsearch_cluster_health_active_shards`                    | GaugeValue | Aggregate total of all shards across all indices, which includes replica shards.                 |
| `elasticsearch_cluster_health_active_shards_percent`            | GaugeValue | Percentage of active shards in the cluster.                                                      |
| `elasticsearch_cluster_health_delayed_unassigned_shards`        | GaugeValue | Shards delayed to reduce reallocation overhead.                                                  |
| `elasticsearch_cluster_health_initializing_shards`              | GaugeValue | Count of shards that are being freshly created.                                                  |
| `elasticsearch_cluster_health_number_of_data_nodes`             | GaugeValue | Number of data nodes in the cluster.                                                             |
| `elasticsearch_cluster_health_number_of_in_flight_fetch`        | GaugeValue | The number of ongoing shard info requests.                                                       |
| `elasticsearch_cluster_health_task_max_waiting_in_queue_millis` | GaugeValue | Tasks max time waiting in queue.                                                                 |
| `elasticsearch_cluster_health_number_of_nodes`                  | GaugeValue | Number of nodes in the cluster.                                                                  |
| `elasticsearch_cluster_health_number_of_pending_tasks`          | GaugeValue | Cluster level changes which have not yet been executed.                                          |
| `elasticsearch_cluster_health_relocating_shards`                | GaugeValue | The number of shards that are currently moving from one node to another node.                    |
| `elasticsearch_cluster_health_unassigned_shards`                | GaugeValue | The number of shards that exist in the cluster state, but cannot be found in the cluster itself. |

#### `cluster_health = true` and `cluster_health_level = "indices"`

| Name                                                         | Type       | Description                                                                                      |
|--------------------------------------------------------------|------------|--------------------------------------------------------------------------------------------------|
| `elasticsearch_cluster_health_indices_active_primary_shards` | GaugeValue | The number of primary shards in your cluster. This is an aggregate total across all indices.     |
| `elasticsearch_cluster_health_indices_active_shards`         | GaugeValue | Aggregate total of all shards across all indices, which includes replica shards.                 |
| `elasticsearch_cluster_health_indices_initializing_shards`   | GaugeValue | Count of shards that are being freshly created.                                                  |
| `elasticsearch_cluster_health_indices_number_of_replicas`    | GaugeValue | Number of replicas in the cluster.                                                               |
| `elasticsearch_cluster_health_indices_number_of_shards`      | GaugeValue | Number of shards in the cluster.                                                                 |
| `elasticsearch_cluster_health_indices_relocating_shards`     | GaugeValue | The number of shards that are currently moving from one node to another node.                    |
| `elasticsearch_cluster_health_indices_unassigned_shards`     | GaugeValue | The number of shards that exist in the cluster state, but cannot be found in the cluster itself. |

#### `export_cluster_settings = true`

| Name                                                                   | Type       | Description                                                     |
|------------------------------------------------------------------------|------------|-----------------------------------------------------------------|
| `elasticsearch_clustersettings_stats_shard_allocation_enabled`         | GaugeValue | Current mode of cluster wide shard routing allocation settings. |
| `elasticsearch_clustersettings_stats_max_shards_per_node`              | GaugeValue | Current maximum number of shards per node setting.              |
| `elasticsearch_clustersettings_allocation_threshold_enabled`           | GaugeValue | Is disk allocation decider enabled.                             |
| `elasticsearch_clustersettings_allocation_watermark_flood_stage_ratio` | GaugeValue | Flood stage watermark as a ratio.                               |
| `elasticsearch_clustersettings_allocation_watermark_high_ratio`        | GaugeValue | High watermark for disk usage as a ratio.                       |
| `elasticsearch_clustersettings_allocation_watermark_low_ratio`         | GaugeValue | Low watermark for disk usage as a ratio.                        |
| `elasticsearch_clustersettings_allocation_watermark_flood_stage_bytes` | GaugeValue | Flood stage watermark in bytes.                                 |
| `elasticsearch_clustersettings_allocation_watermark_high_bytes`        | GaugeValue | High watermark for disk usage in bytes.                         |
| `elasticsearch_clustersettings_allocation_watermark_low_bytes`         | GaugeValue | Low watermark for disk usage in bytes.                          |

#### `cluster_stats = true`

| Name                                                                                     | Type          | Description                                      |
|------------------------------------------------------------------------------------------|---------------|--------------------------------------------------|
| `elasticsearch_clusterstats_indices_count`                                               | CounterValue  | Completion in bytes                              |
| `elasticsearch_clusterstats_indices_completion_size_in_bytes`                            | CounterValue  | Completion in bytes                              |
| `elasticsearch_clusterstats_indices_docs_count`                                          | GaugeValue    | Count of documents on this cluster               |
| `elasticsearch_clusterstats_indices_docs_deleted`                                        | GaugeValue    | Count of deleted documents on this cluster       |
| `elasticsearch_clusterstats_indices_fielddata_evictions`                                 | CounterValue  | Evictions from field data                        |
| `elasticsearch_clusterstats_indices_fielddata_memory_size_in_bytes`                      | GaugeValue    | Field data cache memory usage in bytes           |
| `elasticsearch_clusterstats_indices_query_cache_cache_count`                             | CounterValue  | Query cache cache count                          |
| `elasticsearch_clusterstats_indices_query_cache_cache_size`                              | GaugeValue    | Query cache cache size                           |
| `elasticsearch_clusterstats_indices_query_cache_evictions`                               | CounterValue  | Evictions from query cache                       |
| `elasticsearch_clusterstats_indices_query_cache_hit_count`                               | CounterValue  | Query cache count                                |
| `elasticsearch_clusterstats_indices_query_cache_memory_size_in_bytes`                    | GaugeValue    | Query cache memory usage in bytes                |
| `elasticsearch_clusterstats_indices_query_cache_miss_count`                              | CounterValue  | Query miss count                                 |
| `elasticsearch_clusterstats_indices_query_cache_total_count`                             | CounterValue  | Query cache total count                          |
| `elasticsearch_clusterstats_indices_segments_count`                                      | GaugeValue    | Count of index segments on this cluster          |
| `elasticsearch_clusterstats_indices_segments_doc_values_memory_in_bytes`                 | GaugeValue    | Count of doc values memory                       |
| `elasticsearch_clusterstats_indices_segments_fixed_bit_set_memory_in_bytes`              | GaugeValue    | Count of fixed bit set                           |
| `elasticsearch_clusterstats_indices_segments_index_writer_memory_in_bytes`               | GaugeValue    | Count of memory for index writer on this cluster |
| `elasticsearch_clusterstats_indices_segments_max_unsafe_auto_id_timestamp`               | GaugeValue    | Count of memory for index writer on this cluster |
| `elasticsearch_clusterstats_indices_segments_memory_in_bytes`                            | GaugeValue    | Current memory size of segments in bytes         |
| `elasticsearch_clusterstats_indices_segments_norms_memory_in_bytes`                      | GaugeValue    | Count of memory used by norms                    |
| `elasticsearch_clusterstats_indices_segments_points_memory_in_bytes`                     | GaugeValue    | Point values memory usage in bytes               |
| `elasticsearch_clusterstats_indices_segments_stored_fields_memory_in_bytes`              | GaugeValue    | Count of stored fields memory                    |
| `elasticsearch_clusterstats_indices_segments_term_vectors_memory_in_bytes`               | GaugeValue    | Term vectors memory usage in bytes               |
| `elasticsearch_clusterstats_indices_segments_terms_memory_in_bytes`                      | GaugeValue    | Count of terms in memory for this cluster        |
| `elasticsearch_clusterstats_indices_segments_version_map_memory_in_bytes`                | GaugeValue    | Version map memory usage in bytes                |
| `elasticsearch_clusterstats_indices_shards_total`                                        | GaugeValue    | Total number of shards in the cluster            |
| `elasticsearch_clusterstats_indices_shards_replication`                                  | GaugeValue    | Number of shards replication                     |
| `elasticsearch_clusterstats_indices_shards_primaries`                                    | GaugeValue    | Number of primary shards in the cluster          |
| `elasticsearch_clusterstats_indices_shards_index_primaries_avg`                          | GaugeValue    | Average number of primary shards per index       |
| `elasticsearch_clusterstats_indices_shards_index_primaries_max`                          | GaugeValue    | Max number of primary shards per index           |
| `elasticsearch_clusterstats_indices_shards_index_primaries_min`                          | GaugeValue    | Min number of primary shards per index           |
| `elasticsearch_clusterstats_indices_shards_index_replication_avg`                        | GaugeValue    | Average number of replication shards per index   |
| `elasticsearch_clusterstats_indices_shards_index_replication_max`                        | GaugeValue    | Max number of replication shards per index       |
| `elasticsearch_clusterstats_indices_shards_index_replication_min`                        | GaugeValue    | Min number of replication shards per index       |
| `elasticsearch_clusterstats_indices_shards_index_shards_avg`                             | GaugeValue    | Average number of shards per index               |
| `elasticsearch_clusterstats_indices_shards_index_shards_max`                             | GaugeValue    | Max number of shards per index                   |
| `elasticsearch_clusterstats_indices_shards_index_shards_min`                             | GaugeValue    | Min number of shards per index                   |
| `elasticsearch_clusterstats_indices_store_size_in_bytes`                                 | GaugeValue    | Current size of the store in bytes               |
| `elasticsearch_clusterstats_indices_total_data_set_size_in_bytes`                        | GaugeValue    | Total data set size in bytes                     |
| `elasticsearch_clusterstats_indices_reserved_in_bytes`                                   | GaugeValue    | Reserved size in bytes                           |
| `elasticsearch_clusterstats_nodes_count_coordinating_only`                               | GaugeValue    | Count of coordinating only nodes                 |
| `elasticsearch_clusterstats_nodes_count_data`                                            | GaugeValue    | Count of data nodes                              |
| `elasticsearch_clusterstats_nodes_count_ingest`                                          | GaugeValue    | Count of ingest nodes                            |
| `elasticsearch_clusterstats_nodes_count_master`                                          | GaugeValue    | Count of master nodes                            |
| `elasticsearch_clusterstats_nodes_count_total`                                           | GaugeValue    | Total count of nodes in the cluster              |
| `elasticsearch_clusterstats_nodes_fs_available_in_bytes`                                 | GaugeValue    | Available disk space in bytes                    |
| `elasticsearch_clusterstats_nodes_fs_free_in_bytes`                                      | GaugeValue    | Free disk space in bytes                         |
| `elasticsearch_clusterstats_nodes_fs_total_in_bytes`                                     | GaugeValue    | Total disk space in bytes                        |
| `elasticsearch_clusterstats_nodes_jvm_max_uptime_in_millis`                              | GaugeValue    | Max uptime in milliseconds                       |
| `elasticsearch_clusterstats_nodes_jvm_mem_heap_max_in_bytes`                             | GaugeValue    | Max heap memory in bytes                         |
| `elasticsearch_clusterstats_nodes_jvm_mem_heap_used_in_bytes`                            | GaugeValue    | Used heap memory in bytes                        |
| `elasticsearch_clusterstats_nodes_jvm_threads`                                           | GaugeValue    | Number of threads                                |
| `elasticsearch_clusterstats_nodes_network_types_http_types_security4`                    | GaugeValue    | HTTP security4 network types                     |
| `elasticsearch_clusterstats_nodes_network_types_transport_types_security4`               | GaugeValue    | Transport security4 network types                |
| `elasticsearch_clusterstats_nodes_os_allocated_processors`                               | GaugeValue    | Allocated processors                             |
| `elasticsearch_clusterstats_nodes_os_available_processors`                               | GaugeValue    | Available processors                             |
| `elasticsearch_clusterstats_nodes_os_mem_free_in_bytes`                                  | GaugeValue    | Free memory in bytes                             |
| `elasticsearch_clusterstats_nodes_os_mem_free_percent`                                   | GaugeValue    | Free memory in percent                           |
| `elasticsearch_clusterstats_nodes_os_mem_total_in_bytes`                                 | GaugeValue    | Total memory in bytes                            |
| `elasticsearch_clusterstats_nodes_os_mem_used_in_bytes`                                  | GaugeValue    | Used memory in bytes                             |
| `elasticsearch_clusterstats_nodes_os_mem_used_percent`                                   | GaugeValue    | Used memory in percent                           |
| `elasticsearch_clusterstats_nodes_process_cpu_percent`                                   | GaugeValue    | Process CPU in percent                           |
| `elasticsearch_clusterstats_nodes_process_open_file_descriptors_avg`                     | GaugeValue    | Average number of open file descriptors          |
| `elasticsearch_clusterstats_nodes_process_open_file_descriptors_max`                     | GaugeValue    | Max number of open file descriptors              |
| `elasticsearch_clusterstats_nodes_process_open_file_descriptors_min`                     | GaugeValue    | Min number of open file descriptors              |

#### `export_data_stream = true`

| Name                                                  | Type         | Description                                      |
|-------------------------------------------------------|--------------|--------------------------------------------------|
| `elasticsearch_data_stream_backing_indices_total`     | CounterValue | Number of backing indices                        |
| `elasticsearch_data_stream_store_size_bytes`          | CounterValue | Store size of data stream                        |
| `elasticsearch_data_stream_stats_up`                  | gauge        | Up metric for Data Stream collection             |
| `elasticsearch_data_stream_stats_total_scrapes`       | counter      | Total scrapes for Data Stream stats              |
| `elasticsearch_data_stream_stats_json_parse_failures` | counter      | Number of parsing failures for Data Stream stats |

#### `export_indices = true`

| Name                                                                       | Type         | Description                                                                                  |
|----------------------------------------------------------------------------|--------------|----------------------------------------------------------------------------------------------|
| `elasticsearch_indices_stats_total_docs_count`                             | GaugeValue   | Total count of documents                                                                     |
| `elasticsearch_indices_stats_total_docs_deleted`                           | GaugeValue   | Total count of deleted documents                                                             |
| `elasticsearch_indices_stats_total_store_size_in_bytes`                    | GaugeValue   | Current total size of stored index data in bytes with all shards on all nodes                |
| `elasticsearch_indices_stats_total_throttle_time_seconds`                  | GaugeValue   | Total time the index has been throttled in seconds                                           |
| `elasticsearch_indices_stats_total_segments_count`                         | GaugeValue   | Current number of segments with all shards on all nodes                                      |
| `elasticsearch_indices_stats_total_segments_memory_in_bytes`               | GaugeValue   | Current size of segments with all shards on all nodes in bytes                               |
| `elasticsearch_indices_stats_total_segments_terms_memory_in_bytes`         | GaugeValue   | Current number of terms with all shards on all nodes in bytes                                |
| `elasticsearch_indices_stats_total_segments_stored_fields_memory_in_bytes` | GaugeValue   | Current size of fields with all shards on all nodes in bytes                                 |
| `elasticsearch_indices_stats_total_segments_term_vectors_memory_in_bytes`  | GaugeValue   | Current size of term vectors with all shards on all nodes in bytes                           |
| `elasticsearch_indices_stats_total_segments_norms_memory_in_bytes`         | GaugeValue   | Current size of norms with all shards on all nodes in bytes                                  |
| `elasticsearch_indices_stats_total_segments_points_memory_in_bytes`        | GaugeValue   | Current size of points with all shards on all nodes in bytes                                 |
| `elasticsearch_indices_stats_total_segments_doc_values_memory_in_bytes`    | GaugeValue   | Current size of doc values with all shards on all nodes in bytes                             |
| `elasticsearch_indices_stats_total_segments_index_writer_memory_in_bytes`  | GaugeValue   | Current size of index writer with all shards on all nodes in bytes                           |
| `elasticsearch_indices_stats_total_segments_version_map_memory_in_bytes`   | GaugeValue   | Current size of version map with all shards on all nodes in bytes                            |
| `elasticsearch_indices_stats_total_segments_fixed_bit_set_memory_in_bytes` | GaugeValue   | Current size of fixed bit with all shards on all nodes in bytes                              |
| `elasticsearch_indices_stats_total_segments_max_unsafe_auto_id_timestamp`  | GaugeValue   | Current max unsafe auto id timestamp with all shards on all nodes                            |
| `elasticsearch_indices_stats_total_translog_earliest_last_modified_age`    | GaugeValue   | Current earliest last modified age with all shards on all nodes                              |
| `elasticsearch_indices_stats_total_translog_operations`                    | GaugeValue   | Current number of operations in the transaction log with all shards on all nodes             |
| `elasticsearch_indices_stats_total_translog_size_in_bytes`                 | GaugeValue   | Current size of transaction log with all shards on all nodes in bytes                        |
| `elasticsearch_indices_stats_total_translog_uncommitted_operations`        | GaugeValue   | Current number of uncommitted operations in the transaction log with all shards on all nodes |
| `elasticsearch_indices_stats_total_translog_uncommitted_size_in_bytes`     | GaugeValue   | Current size of uncommitted transaction log with all shards on all nodes in bytes            |
| `elasticsearch_indices_stats_total_completion_size_in_bytes`               | GaugeValue   | Current size of completion with all shards on all nodes in bytes                             |
| `elasticsearch_indices_stats_total_search_query_time_seconds`              | CounterValue | Total search query time in seconds                                                           |
| `elasticsearch_indices_stats_total_search_query_current`                   | GaugeValue   | The number of currently active queries                                                       |
| `elasticsearch_indices_stats_total_search_open_contexts`                   | CounterValue | Total number of open search contexts                                                         |
| `elasticsearch_indices_stats_total_search_query_total`                     | CounterValue | Total number of queries                                                                      |
| `elasticsearch_indices_stats_total_search_fetch_time_seconds`              | CounterValue | Total search fetch time in seconds                                                           |
| `elasticsearch_indices_stats_total_search_fetch`                           | CounterValue | Total search fetch count                                                                     |
| `elasticsearch_indices_stats_total_search_fetch_current`                   | CounterValue | Current search fetch count                                                                   |
| `elasticsearch_indices_stats_total_search_scroll_time_seconds`             | CounterValue | Total search scroll time in seconds                                                          |
| `elasticsearch_indices_stats_total_search_scroll_current`                  | GaugeValue   | Current search scroll count                                                                  |
| `elasticsearch_indices_stats_total_search_scroll`                          | CounterValue | Total search scroll count                                                                    |
| `elasticsearch_indices_stats_total_search_suggest_time_seconds`            | CounterValue | Total search suggest time in seconds                                                         |
| `elasticsearch_indices_stats_total_search_suggest_total`                   | CounterValue | Total search suggest count                                                                   |
| `elasticsearch_indices_stats_total_search_suggest_current`                 | CounterValue | Current search suggest count                                                                 |
| `elasticsearch_indices_stats_total_indexing_index_time_seconds`            | CounterValue | Total indexing index time in seconds                                                         |
| `elasticsearch_indices_stats_total_index_current`                          | GaugeValue   | The number of documents currently being indexed                                              |
| `elasticsearch_indices_stats_total_index_failed`                           | GaugeValue   | Total indexing index failed count                                                            |
| `elasticsearch_indices_stats_total_delete_current`                         | GaugeValue   | The number of delete operations currently being processed                                    |
| `elasticsearch_indices_stats_total_indexing_index`                         | CounterValue | Total indexing index count                                                                   |
| `elasticsearch_indices_stats_total_indexing_delete_time_seconds`           | CounterValue | Total indexing delete time in seconds                                                        |
| `elasticsearch_indices_stats_total_indexing_delete`                        | CounterValue | Total indexing delete count                                                                  |
| `elasticsearch_indices_stats_total_indexing_noop_update`                   | CounterValue | Total indexing no-op update count                                                            |
| `elasticsearch_indices_stats_total_indexing_throttle_time_seconds`         | CounterValue | Total indexing throttle time in seconds                                                      |
| `elasticsearch_indices_stats_total_get_time_seconds`                       | CounterValue | Total get time in seconds                                                                    |
| `elasticsearch_indices_stats_total_get_exists_total`                       | CounterValue | Total exists count                                                                           |
| `elasticsearch_indices_stats_total_get_exists_time_seconds`                | CounterValue | Total exists time in seconds                                                                 |
| `elasticsearch_indices_stats_total_get_total`                              | CounterValue | Total get count                                                                              |
| `elasticsearch_indices_stats_total_get_missing_total`                      | CounterValue | Total missing count                                                                          |
| `elasticsearch_indices_stats_total_get_missing_time_seconds`               | CounterValue | Total missing time in seconds                                                                |
| `elasticsearch_indices_stats_total_get_current`                            | CounterValue | Current get count                                                                            |
| `elasticsearch_indices_stats_total_merges_time_seconds`                    | CounterValue | Total merge time in seconds                                                                  |
| `elasticsearch_indices_stats_total_merges_total`                           | CounterValue | Total merge count                                                                            |
| `elasticsearch_indices_stats_total_merges_total_docs`                      | CounterValue | Total merge docs count                                                                       |
| `elasticsearch_indices_stats_total_merges_total_size_in_bytes`             | CounterValue | Total merge size in bytes                                                                    |
| `elasticsearch_indices_stats_total_merges_current`                         | CounterValue | Current merge count                                                                          |
| `elasticsearch_indices_stats_total_merges_current_docs`                    | CounterValue | Current merge docs count                                                                     |
| `elasticsearch_indices_stats_total_merges_current_size_in_bytes`           | CounterValue | Current merge size in bytes                                                                  |
| `elasticsearch_indices_stats_total_merges_total_throttle_time_seconds`     | CounterValue | Total merge I/O throttle time in seconds                                                     |
| `elasticsearch_indices_stats_total_merges_total_stopped_time_seconds`      | CounterValue | Total large merge stopped time in seconds, allowing smaller merges to complete               |
| `elasticsearch_indices_stats_total_merges_total_auto_throttle_bytes`       | CounterValue | Total bytes that were auto-throttled during merging                                          |
| `elasticsearch_indices_stats_total_refresh_external_total_time_seconds`    | CounterValue | Total external refresh time in seconds                                                       |
| `elasticsearch_indices_stats_total_refresh_external_total`                 | CounterValue | Total external refresh count                                                                 |
| `elasticsearch_indices_stats_total_refresh_total_time_seconds`             | CounterValue | Total refresh time in seconds                                                                |
| `elasticsearch_indices_stats_total_refresh_total`                          | CounterValue | Total refresh count                                                                          |
| `elasticsearch_indices_stats_total_refresh_listeners`                      | CounterValue | Total number of refresh listeners                                                            |
| `elasticsearch_indices_stats_total_recovery_current_as_source`             | CounterValue | Current number of recovery as source                                                         |
| `elasticsearch_indices_stats_total_recovery_current_as_target`             | CounterValue | Current number of recovery as target                                                         |
| `elasticsearch_indices_stats_total_recovery_throttle_time_seconds`         | CounterValue | Total recovery throttle time in seconds                                                      |
| `elasticsearch_indices_stats_total_flush_time_seconds_total`               | CounterValue | Total flush time in seconds                                                                  |
| `elasticsearch_indices_stats_total_flush_total`                            | CounterValue | Total flush count                                                                            |
| `elasticsearch_indices_stats_total_flush_periodic`                         | CounterValue | Total periodic flush count                                                                   |
| `elasticsearch_indices_stats_total_warmer_time_seconds_total`              | CounterValue | Total warmer time in seconds                                                                 |
| `elasticsearch_indices_stats_total_warmer_total`                           | CounterValue | Total warmer count                                                                           |
| `elasticsearch_indices_stats_total_query_cache_memory_in_bytes`            | CounterValue | Total query cache memory bytes                                                               |
| `elasticsearch_indices_stats_total_query_cache_size`                       | GaugeValue   | Total query cache size                                                                       |
| `elasticsearch_indices_stats_total_query_cache_total_count`                | CounterValue | Total query cache count                                                                      |
| `elasticsearch_indices_stats_total_query_cache_hit_count`                  | CounterValue | Total query cache hits count                                                                 |
| `elasticsearch_indices_stats_total_query_cache_miss_count`                 | CounterValue | Total query cache misses count                                                               |
| `elasticsearch_indices_stats_total_query_cache_cache_count`                | CounterValue | Total query cache caches count                                                               |
| `elasticsearch_indices_stats_total_query_cache_evictions`                  | CounterValue | Total query cache evictions count                                                            |
| `elasticsearch_indices_stats_total_request_cache_memory_in_bytes`          | CounterValue | Total request cache memory bytes                                                             |
| `elasticsearch_indices_stats_total_request_cache_hit_count`                | CounterValue | Total request cache hits count                                                               |
| `elasticsearch_indices_stats_total_request_cache_miss_count`               | CounterValue | Total request cache misses count                                                             |
| `elasticsearch_indices_stats_total_request_cache_evictions`                | CounterValue | Total request cache evictions count                                                          |
| `elasticsearch_indices_stats_total_fielddata_memory_in_bytes`              | CounterValue | Total fielddata memory bytes                                                                 |
| `elasticsearch_indices_stats_total_fielddata_evictions`                    | CounterValue | Total fielddata evictions count                                                              |
| `elasticsearch_indices_stats_total_seq_no_global_checkpoint`               | CounterValue | Global checkpoint                                                                            |
| `elasticsearch_indices_stats_total_seq_no_local_checkpoint`                | CounterValue | Local checkpoint                                                                             |
| `elasticsearch_indices_stats_total_seq_no_max_seq_no`                      | CounterValue | Max sequence number                                                                          |


| Name                                                                            | Type         | Description                                                                                  |
|---------------------------------------------------------------------------------|--------------|----------------------------------------------------------------------------------------------|
| `elasticsearch_indices_stats_primaries_docs_count`                              | GaugeValue   | Total count of documents                                                                     |
| `elasticsearch_indices_stats_primaries_docs_deleted`                            | GaugeValue   | Total count of deleted documents                                                             |
| `elasticsearch_indices_stats_primaries_store_size_in_bytes`                     | GaugeValue   | Current total size of stored index data in bytes with all shards on all nodes                |
| `elasticsearch_indices_stats_primaries_throttle_time_seconds`                   | GaugeValue   | Total time the index has been throttled in seconds                                           |
| `elasticsearch_indices_stats_primaries_segments_count`                          | GaugeValue   | Current number of segments with all shards on all nodes                                      |
| `elasticsearch_indices_stats_primaries_segments_memory_in_bytes`                | GaugeValue   | Current size of segments with all shards on all nodes in bytes                               |
| `elasticsearch_indices_stats_primaries_segments_terms_memory_in_bytes`          | GaugeValue   | Current number of terms with all shards on all nodes in bytes                                |
| `elasticsearch_indices_stats_primaries_segments_stored_fields_memory_in_bytes`  | GaugeValue   | Current size of fields with all shards on all nodes in bytes                                 |
| `elasticsearch_indices_stats_primaries_segments_term_vectors_memory_in_bytes`   | GaugeValue   | Current size of term vectors with all shards on all nodes in bytes                           |
| `elasticsearch_indices_stats_primaries_segments_norms_memory_in_bytes`          | GaugeValue   | Current size of norms with all shards on all nodes in bytes                                  |
| `elasticsearch_indices_stats_primaries_segments_points_memory_in_bytes`         | GaugeValue   | Current size of points with all shards on all nodes in bytes                                 |
| `elasticsearch_indices_stats_primaries_segments_doc_values_memory_in_bytes`     | GaugeValue   | Current size of doc values with all shards on all nodes in bytes                             |
| `elasticsearch_indices_stats_primaries_segments_index_writer_memory_in_bytes`   | GaugeValue   | Current size of index writer with all shards on all nodes in bytes                           |
| `elasticsearch_indices_stats_primaries_segments_version_map_memory_in_bytes`    | GaugeValue   | Current size of version map with all shards on all nodes in bytes                            |
| `elasticsearch_indices_stats_primaries_segments_fixed_bit_set_memory_in_bytes`  | GaugeValue   | Current size of fixed bit with all shards on all nodes in bytes                              |
| `elasticsearch_indices_stats_primaries_segments_max_unsafe_auto_id_timestamp`   | GaugeValue   | Current max unsafe auto id timestamp with all shards on all nodes                            |
| `elasticsearch_indices_stats_primaries_translog_earliest_last_modified_age`     | GaugeValue   | Current earliest last modified age with all shards on all nodes                              |
| `elasticsearch_indices_stats_primaries_translog_operations`                     | GaugeValue   | Current number of operations in the transaction log with all shards on all nodes             |
| `elasticsearch_indices_stats_primaries_translog_size_in_bytes`                  | GaugeValue   | Current size of transaction log with all shards on all nodes in bytes                        |
| `elasticsearch_indices_stats_primaries_translog_uncommitted_operations`         | GaugeValue   | Current number of uncommitted operations in the transaction log with all shards on all nodes |
| `elasticsearch_indices_stats_primaries_translog_uncommitted_size_in_bytes`      | GaugeValue   | Current size of uncommitted transaction log with all shards on all nodes in bytes            |
| `elasticsearch_indices_stats_primaries_completion_size_in_bytes`                | GaugeValue   | Current size of completion with all shards on all nodes in bytes                             |
| `elasticsearch_indices_stats_primaries_search_query_time_seconds`               | CounterValue | Total search query time in seconds                                                           |
| `elasticsearch_indices_stats_primaries_search_query_current`                    | GaugeValue   | The number of currently active queries                                                       |
| `elasticsearch_indices_stats_primaries_search_open_contexts`                    | CounterValue | Total number of open search contexts                                                         |
| `elasticsearch_indices_stats_primaries_search_query_total`                      | CounterValue | Total number of queries                                                                      |
| `elasticsearch_indices_stats_primaries_search_fetch_time_seconds`               | CounterValue | Total search fetch time in seconds                                                           |
| `elasticsearch_indices_stats_primaries_search_fetch`                            | CounterValue | Total search fetch count                                                                     |
| `elasticsearch_indices_stats_primaries_search_fetch_current`                    | CounterValue | Current search fetch count                                                                   |
| `elasticsearch_indices_stats_primaries_search_scroll_time_seconds`              | CounterValue | Total search scroll time in seconds                                                          |
| `elasticsearch_indices_stats_primaries_search_scroll_current`                   | GaugeValue   | Current search scroll count                                                                  |
| `elasticsearch_indices_stats_primaries_search_scroll`                           | CounterValue | Total search scroll count                                                                    |
| `elasticsearch_indices_stats_primaries_search_suggest_time_seconds`             | CounterValue | Total search suggest time in seconds                                                         |
| `elasticsearch_indices_stats_primaries_search_suggest_total`                    | CounterValue | Total search suggest count                                                                   |
| `elasticsearch_indices_stats_primaries_search_suggest_current`                  | CounterValue | Current search suggest count                                                                 |
| `elasticsearch_indices_stats_primaries_indexing_index_time_seconds`             | CounterValue | Total indexing index time in seconds                                                         |
| `elasticsearch_indices_stats_primaries_index_current`                           | GaugeValue   | The number of documents currently being indexed                                              |
| `elasticsearch_indices_stats_primaries_index_failed`                            | GaugeValue   | Total indexing index failed count                                                            |
| `elasticsearch_indices_stats_primaries_delete_current`                          | GaugeValue   | The number of delete operations currently being processed                                    |
| `elasticsearch_indices_stats_primaries_indexing_index`                          | CounterValue | Total indexing index count                                                                   |
| `elasticsearch_indices_stats_primaries_indexing_delete_time_seconds`            | CounterValue | Total indexing delete time in seconds                                                        |
| `elasticsearch_indices_stats_primaries_indexing_delete`                         | CounterValue | Total indexing delete count                                                                  |
| `elasticsearch_indices_stats_primaries_indexing_noop_update`                    | CounterValue | Total indexing no-op update count                                                            |
| `elasticsearch_indices_stats_primaries_indexing_throttle_time_seconds`          | CounterValue | Total indexing throttle time in seconds                                                      |
| `elasticsearch_indices_stats_primaries_get_time_seconds`                        | CounterValue | Total get time in seconds                                                                    |
| `elasticsearch_indices_stats_primaries_get_exists_total`                        | CounterValue | Total exists count                                                                           |
| `elasticsearch_indices_stats_primaries_get_exists_time_seconds`                 | CounterValue | Total exists time in seconds                                                                 |
| `elasticsearch_indices_stats_primaries_get_total`                               | CounterValue | Total get count                                                                              |
| `elasticsearch_indices_stats_primaries_get_missing_total`                       | CounterValue | Total missing count                                                                          |
| `elasticsearch_indices_stats_primaries_get_missing_time_seconds`                | CounterValue | Total missing time in seconds                                                                |
| `elasticsearch_indices_stats_primaries_get_current`                             | CounterValue | Current get count                                                                            |
| `elasticsearch_indices_stats_primaries_merges_time_seconds`                     | CounterValue | Total merge time in seconds                                                                  |
| `elasticsearch_indices_stats_primaries_merges_total`                            | CounterValue | Total merge count                                                                            |
| `elasticsearch_indices_stats_primaries_merges_primaries_docs`                   | CounterValue | Total merge docs count                                                                       |
| `elasticsearch_indices_stats_primaries_merges_primaries_size_in_bytes`          | CounterValue | Total merge size in bytes                                                                    |
| `elasticsearch_indices_stats_primaries_merges_current`                          | CounterValue | Current merge count                                                                          |
| `elasticsearch_indices_stats_primaries_merges_current_docs`                     | CounterValue | Current merge docs count                                                                     |
| `elasticsearch_indices_stats_primaries_merges_current_size_in_bytes`            | CounterValue | Current merge size in bytes                                                                  |
| `elasticsearch_indices_stats_primaries_merges_primaries_throttle_time_seconds`  | CounterValue | Total merge I/O throttle time in seconds                                                     |
| `elasticsearch_indices_stats_primaries_merges_primaries_stopped_time_seconds`   | CounterValue | Total large merge stopped time in seconds, allowing smaller merges to complete               |
| `elasticsearch_indices_stats_primaries_merges_primaries_auto_throttle_bytes`    | CounterValue | Total bytes that were auto-throttled during merging                                          |
| `elasticsearch_indices_stats_primaries_refresh_external_primaries_time_seconds` | CounterValue | Total external refresh time in seconds                                                       |
| `elasticsearch_indices_stats_primaries_refresh_external_total`                  | CounterValue | Total external refresh count                                                                 |
| `elasticsearch_indices_stats_primaries_refresh_primaries_time_seconds`          | CounterValue | Total refresh time in seconds                                                                |
| `elasticsearch_indices_stats_primaries_refresh_total`                           | CounterValue | Total refresh count                                                                          |
| `elasticsearch_indices_stats_primaries_refresh_listeners`                       | CounterValue | Total number of refresh listeners                                                            |
| `elasticsearch_indices_stats_primaries_recovery_current_as_source`              | CounterValue | Current number of recovery as source                                                         |
| `elasticsearch_indices_stats_primaries_recovery_current_as_target`              | CounterValue | Current number of recovery as target                                                         |
| `elasticsearch_indices_stats_primaries_recovery_throttle_time_seconds`          | CounterValue | Total recovery throttle time in seconds                                                      |
| `elasticsearch_indices_stats_primaries_flush_time_seconds_total`                | CounterValue | Total flush time in seconds                                                                  |
| `elasticsearch_indices_stats_primaries_flush_total`                             | CounterValue | Total flush count                                                                            |
| `elasticsearch_indices_stats_primaries_flush_periodic`                          | CounterValue | Total periodic flush count                                                                   |
| `elasticsearch_indices_stats_primaries_warmer_time_seconds_total`               | CounterValue | Total warmer time in seconds                                                                 |
| `elasticsearch_indices_stats_primaries_warmer_total`                            | CounterValue | Total warmer count                                                                           |
| `elasticsearch_indices_stats_primaries_query_cache_memory_in_bytes`             | CounterValue | Total query cache memory bytes                                                               |
| `elasticsearch_indices_stats_primaries_query_cache_size`                        | GaugeValue   | Total query cache size                                                                       |
| `elasticsearch_indices_stats_primaries_query_cache_primaries_count`             | CounterValue | Total query cache count                                                                      |
| `elasticsearch_indices_stats_primaries_query_cache_hit_count`                   | CounterValue | Total query cache hits count                                                                 |
| `elasticsearch_indices_stats_primaries_query_cache_miss_count`                  | CounterValue | Total query cache misses count                                                               |
| `elasticsearch_indices_stats_primaries_query_cache_cache_count`                 | CounterValue | Total query cache caches count                                                               |
| `elasticsearch_indices_stats_primaries_query_cache_evictions`                   | CounterValue | Total query cache evictions count                                                            |
| `elasticsearch_indices_stats_primaries_request_cache_memory_in_bytes`           | CounterValue | Total request cache memory bytes                                                             |
| `elasticsearch_indices_stats_primaries_request_cache_hit_count`                 | CounterValue | Total request cache hits count                                                               |
| `elasticsearch_indices_stats_primaries_request_cache_miss_count`                | CounterValue | Total request cache misses count                                                             |
| `elasticsearch_indices_stats_primaries_request_cache_evictions`                 | CounterValue | Total request cache evictions count                                                          |
| `elasticsearch_indices_stats_primaries_fielddata_memory_in_bytes`               | CounterValue | Total fielddata memory bytes                                                                 |
| `elasticsearch_indices_stats_primaries_fielddata_evictions`                     | CounterValue | Total fielddata evictions count                                                              |
| `elasticsearch_indices_stats_primaries_seq_no_global_checkpoint`                | CounterValue | Global checkpoint                                                                            |
| `elasticsearch_indices_stats_primaries_seq_no_local_checkpoint`                 | CounterValue | Local checkpoint                                                                             |
| `elasticsearch_indices_stats_primaries_seq_no_max_seq_no`                       | CounterValue | Max sequence number                                                                          |

#### `allnodes = true`

| Name                                                           | Type         | Description                                            |
|----------------------------------------------------------------|--------------|--------------------------------------------------------|
| `elasticsearch_os_cpu_load_average_1m`                         | GaugeValue   | Shortterm load average                                 |
| `elasticsearch_os_cpu_load_average_5m`                         | GaugeValue   | Midterm load average                                   |
| `elasticsearch_os_cpu_load_average_15m`                        | GaugeValue   | Longterm load average                                  |
| `elasticsearch_os_cpu_percent`                                 | GaugeValue   | Percent CPU used by OS                                 |
| `elasticsearch_os_mem_free_in_bytes`                           | GaugeValue   | Amount of free physical memory in bytes                |
| `elasticsearch_os_mem_used_in_bytes`                           | GaugeValue   | Amount of used physical memory in bytes                |
| `elasticsearch_os_mem_actual_free_in_bytes`                    | GaugeValue   | Amount of free physical memory in bytes                |
| `elasticsearch_os_mem_actual_used_in_bytes`                    | GaugeValue   | Amount of used physical memory in bytes                |
| `elasticsearch_os_mem_used_percent`                            | GaugeValue   | Percent of used physical memory                        |
| `elasticsearch_os_mem_total_in_bytes`                          | GaugeValue   | Amount of used physical memory in bytes                |
| `elasticsearch_os_mem_free_percent`                            | GaugeValue   | Percent of free physical memory                        |
| `elasticsearch_os_cgroup_cpu_cfs_period_micros`                | GaugeValue   | CPU CFS period in microseconds                         |
| `elasticsearch_os_cgroup_cpu_cfs_quota_micros`                 | GaugeValue   | CPU CFS quota in microseconds                          |
| `elasticsearch_os_cgroup_cpu_stat_number_of_elapsed_periods`   | GaugeValue   | CPU CFS quota in microseconds                          |
| `elasticsearch_os_cgroup_cpu_stat_number_of_times_throttled`   | GaugeValue   | CPU CFS quota in microseconds                          |
| `elasticsearch_os_cgroup_cpu_stat_time_throttled_nanos`        | GaugeValue   | CPU CFS quota in microseconds                          |
| `elasticsearch_os_cgroup_cpuacct_usage_nanos`                  | GaugeValue   | Cpuacct usage in nanos                                 |
| `elasticsearch_os_swap_used_in_bytes`                          | GaugeValue   | Amount of used swap memory in bytes                    |
| `elasticsearch_os_swap_total_in_bytes`                         | GaugeValue   | Amount of total swap memory in bytes                   |
| `elasticsearch_os_swap_free_in_bytes`                          | GaugeValue   | Amount of free swap memory in bytes                    |
| `elasticsearch_indices_fielddata_memory_size_in_bytes`         | GaugeValue   | Field data cache memory usage in bytes                 |
| `elasticsearch_indices_fielddata_evictions`                    | CounterValue | Evictions from field data                              |
| `elasticsearch_indices_completion_size_in_bytes`               | CounterValue | Completion in bytes                                    |
| `elasticsearch_indices_filter_cache_memory_size_in_bytes`      | GaugeValue   | Filter cache memory usage in bytes                     |
| `elasticsearch_indices_filter_cache_evictions`                 | CounterValue | Evictions from filter cache                            |
| `elasticsearch_indices_query_cache_memory_size_in_bytes`       | GaugeValue   | Query cache memory usage in bytes                      |
| `elasticsearch_indices_query_cache_evictions`                  | CounterValue | Evictions from query cache                             |
| `elasticsearch_indices_query_cache_total_count`                | CounterValue | Query cache total count                                |
| `elasticsearch_indices_query_cache_cache_size`                 | GaugeValue   | Query cache cache size                                 |
| `elasticsearch_indices_query_cache_cache_count`                | CounterValue | Query cache cache count                                |
| `elasticsearch_indices_query_cache_hit_count`                  | CounterValue | Query cache hit count                                  |
| `elasticsearch_indices_query_cache_miss_count`                 | CounterValue | Query cache miss count                                 |
| `elasticsearch_indices_request_cache_memory_size_in_bytes`     | GaugeValue   | Request cache memory usage in bytes                    |
| `elasticsearch_indices_request_cache_evictions`                | CounterValue | Evictions from request cache                           |
| `elasticsearch_indices_request_cache_hit_count`                | CounterValue | Request cache hit count                                |
| `elasticsearch_indices_request_cache_miss_count`               | CounterValue | Request cache miss count                               |
| `elasticsearch_indices_translog_operations`                    | CounterValue | Total translog operations                              |
| `elasticsearch_indices_translog_size_in_bytes`                 | GaugeValue   | Total translog size in bytes                           |
| `elasticsearch_indices_get_time_seconds`                       | CounterValue | Total get time in seconds                              |
| `elasticsearch_indices_get_total`                              | CounterValue | Total get count                                        |
| `elasticsearch_indices_get_missing_time_seconds`               | CounterValue | Total time of get missing in seconds                   |
| `elasticsearch_indices_get_missing_total`                      | CounterValue | Total get missing count                                |
| `elasticsearch_indices_get_exists_time_seconds`                | CounterValue | Total time get exists in seconds                       |
| `elasticsearch_indices_get_exists_total`                       | CounterValue | Total get exists operations                            |
| `elasticsearch_indices_refresh_time_seconds_total`             | CounterValue | Total time spent refreshing in seconds                 |
| `elasticsearch_indices_refresh_total`                          | CounterValue | Total refreshes                                        |
| `elasticsearch_indices_search_query_time_seconds`              | CounterValue | Total search query time in seconds                     |
| `elasticsearch_indices_search_query_total`                     | CounterValue | Total number of queries                                |
| `elasticsearch_indices_search_fetch_time_seconds`              | CounterValue | Total search fetch time in seconds                     |
| `elasticsearch_indices_search_fetch_total`                     | CounterValue | Total search fetch count                               |
| `elasticsearch_indices_search_suggest_total`                   | CounterValue | Total search suggest count                             |
| `elasticsearch_indices_search_suggest_time_seconds`            | CounterValue | Total search suggest time in seconds                   |
| `elasticsearch_indices_search_scroll_total`                    | CounterValue | Total search scroll count                              |
| `elasticsearch_indices_search_scroll_time_seconds`             | CounterValue | Total search scroll time in seconds                    |
| `elasticsearch_indices_docs_count`                             | GaugeValue   | Count of documents on this node                        |
| `elasticsearch_indices_docs_deleted`                           | GaugeValue   | Count of deleted documents on this node                |
| `elasticsearch_indices_store_size_in_bytes`                    | GaugeValue   | Current size of stored index data                      |
| `elasticsearch_indices_merges_total_size_in_bytes`             | CounterValue | Total merge size in bytes                              |
| `elasticsearch_indices_merges_total_time_seconds_total`        | CounterValue | Total time spent merging in seconds                    |
| `elasticsearch_indices_merges_total_throttled_time_seconds`    | CounterValue | Total throttled time of merges in seconds              |
| `elasticsearch_jvm_threads_count`                              | GaugeValue   | Count of threads                                       |
| `elasticsearch_jvm_threads_peak_count`                         | GaugeValue   | Peak count of threads                                  |
| `elasticsearch_jvm_timestamp`                                  | GaugeValue   | JVM timestamp                                          |
| `elasticsearch_jvm_mem_heap_used_in_bytes`                     | GaugeValue   | JVM memory currently used by heap in bytes             |
| `elasticsearch_jvm_mem_non_heap_used_in_bytes`                 | GaugeValue   | JVM memory currently used by non-heap in bytes         |
| `elasticsearch_jvm_mem_heap_max_in_bytes`                      | GaugeValue   | Maximum JVM heap memory in bytes                       |
| `elasticsearch_jvm_mem_heap_used_percent`                      | GaugeValue   | JVM heap memory used percent                           |
| `elasticsearch_jvm_mem_heap_committed_in_bytes`                | GaugeValue   | JVM heap memory committed in bytes                     |
| `elasticsearch_jvm_mem_non_heap_committed_in_bytes`            | GaugeValue   | JVM non-heap memory committed in bytes                 |
| `elasticsearch_jvm_memory_pools_young_used_in_bytes`           | GaugeValue   | JVM young generation pool used memory in bytes         |
| `elasticsearch_jvm_memory_pools_young_max_in_bytes`            | CounterValue | Maximum JVM young generation pool memory in bytes      |
| `elasticsearch_jvm_memory_pools_young_peak_used_in_bytes`      | CounterValue | Peak used JVM young generation pool memory in bytes    |
| `elasticsearch_jvm_memory_pools_young_peak_max_in_bytes`       | CounterValue | Peak maximum JVM young generation pool memory in bytes |
| `elasticsearch_jvm_memory_pools_survivor_used_in_bytes`        | GaugeValue   | JVM survivor space pool used memory in bytes           |
| `elasticsearch_jvm_memory_pools_survivor_max_in_bytes`         | CounterValue | Maximum JVM survivor space pool memory in bytes        |
| `elasticsearch_jvm_memory_pools_survivor_peak_used_in_bytes`   | CounterValue | Peak used JVM survivor space pool memory in bytes      |
| `elasticsearch_jvm_memory_pools_survivor_peak_max_in_bytes`    | CounterValue | Peak maximum JVM survivor space pool memory in bytes   |
| `elasticsearch_jvm_memory_pools_old_used_in_bytes`             | GaugeValue   | JVM old generation pool used memory in bytes           |
| `elasticsearch_jvm_memory_pools_old_max_in_bytes`              | CounterValue | Maximum JVM old generation pool memory in bytes        |
| `elasticsearch_jvm_memory_pools_old_peak_used_in_bytes`        | CounterValue | Peak used JVM old generation pool memory in bytes      |
| `elasticsearch_jvm_memory_pools_old_peak_max_in_bytes`         | CounterValue | Peak maximum JVM old generation pool memory in bytes   |
| `elasticsearch_jvm_buffer_pool_direct_count`                   | GaugeValue   | JVM buffer pool direct count                           |
| `elasticsearch_jvm_buffer_pool_direct_total_capacity_in_bytes` | GaugeValue   | JVM buffer pool direct total capacity in bytes         |
| `elasticsearch_jvm_buffer_pool_direct_used_in_bytes`           | GaugeValue   | JVM buffer pool direct used in bytes                   |
| `elasticsearch_jvm_buffer_pool_mapped_count`                   | GaugeValue   | JVM buffer pool mapped count                           |
| `elasticsearch_jvm_buffer_pool_mapped_total_capacity_in_bytes` | GaugeValue   | JVM buffer pool mapped total capacity in bytes         |
| `elasticsearch_jvm_buffer_pool_mapped_used_in_bytes`           | GaugeValue   | JVM buffer pool mapped used in bytes                   |
| `elasticsearch_jvm_classes_current_loaded_count`               | GaugeValue   | JVM classes currently loaded count                     |
| `elasticsearch_jvm_classes_total_loaded_count`                 | GaugeValue   | JVM classes total loaded count                         |
| `elasticsearch_jvm_classes_total_unloaded_count`               | GaugeValue   | JVM classes total unloaded count                       |
| `elasticsearch_jvm_uptime_seconds`                             | GaugeValue   | JVM process uptime in seconds                          |
| `elasticsearch_process_cpu_percent`                            | GaugeValue   | Percent CPU used by process                            |
| `elasticsearch_process_mem_resident_size_in_bytes`             | GaugeValue   | Resident memory in use by process in bytes             |
| `elasticsearch_process_mem_share_size_in_bytes`                | GaugeValue   | Shared memory in use by process in bytes               |
     
#### `export_indices_settings = true`      

| Name                                                                 | Type    | Help                                                                                                |  
|----------------------------------------------------------------------|---------|-----------------------------------------------------------------------------------------------------|
| elasticsearch_indices_settings_creation_timestamp_seconds            | gauge   | Timestamp of the index creation in seconds                                                          | 
| elasticsearch_indices_settings_stats_read_only_indices               | gauge   | Count of indices that have read_only_allow_delete=true                                              | 
| elasticsearch_indices_settings_total_fields                          | gauge   | Index setting value for index.mapping.total_fields.limit (total allowable mapped fields in a index) | 
| elasticsearch_indices_settings_replicas                              | gauge   | Index setting value for index.replicas                                                              | 

#### `export_indices_mappings = true`

| Name                                                                 | Type    | Help                                                                                                |
|----------------------------------------------------------------------|---------|-----------------------------------------------------------------------------------------------------|
| elasticsearch_indices_mappings_stats_fields                          | gauge   | Count of fields currently mapped by index                                                           |
| elasticsearch_indices_mappings_stats_json_parse_failures_total       | counter | Number of errors while parsing JSON                                                                 |
| elasticsearch_indices_mappings_stats_scrapes_total                   | counter | Current total Elasticsearch Indices Mappings scrapes                                                |
| elasticsearch_indices_mappings_stats_up                              | gauge   | Was the last scrape of the Elasticsearch Indices Mappings endpoint successful                       |

#### `export_slm = true`

| Name                                                                 | Type    | Help                                                                                                |
|----------------------------------------------------------------------|---------|-----------------------------------------------------------------------------------------------------|
| elasticsearch_slm_stats_up                                           | gauge   | Up metric for SLM collector                                                                         |
| elasticsearch_slm_stats_total_scrapes                                | counter | Number of scrapes for SLM collector                                                                 |
| elasticsearch_slm_stats_json_parse_failures                          | counter | JSON parse failures for SLM collector                                                               |
| elasticsearch_slm_stats_retention_runs_total                         | counter | Total retention runs                                                                                |
| elasticsearch_slm_stats_retention_failed_total                       | counter | Total failed retention runs                                                                         |
| elasticsearch_slm_stats_retention_timed_out_total                    | counter | Total retention run timeouts                                                                        |
| elasticsearch_slm_stats_retention_deletion_time_seconds              | gauge   | Retention run deletion time                                                                         |
| elasticsearch_slm_stats_total_snapshots_taken_total                  | counter | Total snapshots taken                                                                               |
| elasticsearch_slm_stats_total_snapshots_failed_total                 | counter | Total snapshots failed                                                                              |
| elasticsearch_slm_stats_total_snapshots_deleted_total                | counter | Total snapshots deleted                                                                             |
| elasticsearch_slm_stats_total_snapshots_failed_total                 | counter | Total snapshots failed                                                                              |
| elasticsearch_slm_stats_snapshots_taken_total                        | counter | Snapshots taken by policy                                                                           |
| elasticsearch_slm_stats_snapshots_failed_total                       | counter | Snapshots failed by policy                                                                          |
| elasticsearch_slm_stats_snapshots_deleted_total                      | counter | Snapshots deleted by policy                                                                         |
| elasticsearch_slm_stats_snapshot_deletion_failures_total             | counter | Snapshot deletion failures by policy                                                                |
| elasticsearch_slm_stats_operation_mode                               | gauge   | SLM operation mode (Running, stopping, stopped)                                                     |
