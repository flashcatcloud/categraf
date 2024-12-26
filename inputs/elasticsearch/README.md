# Elasticsearch

#### Elasticsearch 7.x 权限

用户名和密码可以直接通过URI传递，或者通过ES_USERNAME和ES_PASSWORD环境变量传递。指定这两个环境变量将覆盖在URI中传递的认证信息（如果有的话）。

ES 7.x 支持基于角色的访问控制（RBACs）。`elasticsearch` 插件需要以下安全权限：

| 设置                      | 所需权限                                                             | 描述                                                                                    |
|:------------------------|:-----------------------------------------------------------------|:--------------------------------------------------------------------------------------|
| export_cluster_settings | `cluster` `monitor`                                              |                                                                                       |
| exporter defaults       | `cluster` `monitor`                                              | 包括所有集群只读操作，如集群健康与状态、热点线程、节点信息、节点和集群统计信息以及待处理的集群任务。                                    |
| export_indices          | `indices` `monitor` (每个索引或 `*`)                                  | 所有监控所需的操作（恢复、段信息、索引统计和状态）                                                             |
| export_indices_settings | `indices` `monitor` (每个索引或 `*`)                                  |                                                                                       |
| export_indices_mappings | `indices` `view_index_metadata` (每个索引或 `*`)                      |                                                                                       |
| export_shards           | 不确定是 `indices` 或 `cluster` `monitor` 或两者都需要                      |                                                                                       |
| export_snapshots        | `cluster:admin/snapshot/status` 和 `cluster:admin/repository/get` | [ES 论坛帖子](https://discuss.elastic.co/t/permissions-for-backup-user-with-x-pack/88057) |
| export_slm              | `read_slm`                                                       |                                                                                       |
| export_data_stream      | `monitor` 或 `manage` (每个索引或 `*`)                                 |                                                                                       |

### 与旧版`elastisearch`插件的区别

- `elasticsearch_cluster_health_active_shards_percent_as_number`改为`elasticsearch_cluster_health_active_shards_percent`。
- `elasticsearch_cluster_health_status`和`elasticsearch_cluster_health_status_code`合并为`elasticsearch_cluster_health_status`，值为`green=1`、`yellow=2`、`red=3`。
- `elasticsearch_process_cpu_total_in_millis`改为`elasticsearch_process_cpu_seconds_total`，单位为秒。
- `elasticsearch_jvm_uptime_in_millis`改为`elasticsearch_jvm_uptime_seconds`，单位为秒。以此类推，所有`*_in_millis`的指标都改为`*_seconds`。

### Metrics

#### `cluster_health = true` 和 `cluster_health_level =  "cluster"`

| 名称                                                              | 类型         | 描述                       |
|-----------------------------------------------------------------|------------|--------------------------|
| `elasticsearch_cluster_health_active_primary_shards`            | GaugeValue | 集群中主分片的数量。这是跨所有索引的聚合总数。  |
| `elasticsearch_cluster_health_active_shards`                    | GaugeValue | 所有索引中所有分片的聚合总数，包括副本分片。   |
| `elasticsearch_cluster_health_active_shards_percent`            | GaugeValue | 集群中活跃分片的百分比。             |
| `elasticsearch_cluster_health_delayed_unassigned_shards`        | GaugeValue | 为减少重新分配开销而延迟的分片数量。       |
| `elasticsearch_cluster_health_initializing_shards`              | GaugeValue | 正在新创建的分片计数。              |
| `elasticsearch_cluster_health_number_of_data_nodes`             | GaugeValue | 集群中数据节点的数量。              |
| `elasticsearch_cluster_health_number_of_in_flight_fetch`        | GaugeValue | 正在进行的分片信息请求的数量。          |
| `elasticsearch_cluster_health_task_max_waiting_in_queue_millis` | GaugeValue | 任务在队列中等待的最长时间（毫秒）。       |
| `elasticsearch_cluster_health_number_of_nodes`                  | GaugeValue | 集群中节点的数量。                |
| `elasticsearch_cluster_health_number_of_pending_tasks`          | GaugeValue | 尚未执行的集群级别变更的数量。          |
| `elasticsearch_cluster_health_relocating_shards`                | GaugeValue | 当前从一个节点移动到另一个节点的分片数量。    |
| `elasticsearch_cluster_health_unassigned_shards`                | GaugeValue | 存在于集群状态中但在集群本身中找不到的分片数量。 |

#### `cluster_health = true` 和 `cluster_health_level = "indices"`

| 名称                                                           | 类型         | 描述                       |
|--------------------------------------------------------------|------------|--------------------------|
| `elasticsearch_cluster_health_indices_active_primary_shards` | GaugeValue | 集群中主分片的数量。这是跨所有索引的聚合总数。  |
| `elasticsearch_cluster_health_indices_active_shards`         | GaugeValue | 所有索引中所有分片的聚合总数，包括副本分片。   |
| `elasticsearch_cluster_health_indices_initializing_shards`   | GaugeValue | 正在新创建的分片计数。              |
| `elasticsearch_cluster_health_indices_number_of_replicas`    | GaugeValue | 集群中副本的数量。                |
| `elasticsearch_cluster_health_indices_number_of_shards`      | GaugeValue | 集群中分片的数量。                |
| `elasticsearch_cluster_health_indices_relocating_shards`     | GaugeValue | 当前从一个节点移动到另一个节点的分片数量。    |
| `elasticsearch_cluster_health_indices_unassigned_shards`     | GaugeValue | 存在于集群状态中但在集群本身中找不到的分片数量。 |

#### `export_cluster_settings = true`

| 名称                                                                     | 类型         | 描述                  |
|------------------------------------------------------------------------|------------|---------------------|
| `elasticsearch_clustersettings_stats_shard_allocation_enabled`         | GaugeValue | 集群范围的分片路由分配设置的当前模式。 |
| `elasticsearch_clustersettings_stats_max_shards_per_node`              | GaugeValue | 每个节点的最大分片数设置的当前值。   |
| `elasticsearch_clustersettings_allocation_threshold_enabled`           | GaugeValue | 磁盘分配决策器是否启用。        |
| `elasticsearch_clustersettings_allocation_watermark_flood_stage_ratio` | GaugeValue | 作为比例的洪水阶段水位标记。      |
| `elasticsearch_clustersettings_allocation_watermark_high_ratio`        | GaugeValue | 作为比例的磁盘使用的高水位标记。    |
| `elasticsearch_clustersettings_allocation_watermark_low_ratio`         | GaugeValue | 作为比例的磁盘使用的低水位标记。    |
| `elasticsearch_clustersettings_allocation_watermark_flood_stage_bytes` | GaugeValue | 以字节为单位的洪水阶段水位标记。    |
| `elasticsearch_clustersettings_allocation_watermark_high_bytes`        | GaugeValue | 以字节为单位的磁盘使用的高水位标记。  |
| `elasticsearch_clustersettings_allocation_watermark_low_bytes`         | GaugeValue | 以字节为单位的磁盘使用的低水位标记。  |

#### `cluster_stats = true`

| 名称                                                                          | 类型           | 描述               |
|-----------------------------------------------------------------------------|--------------|------------------|
| `elasticsearch_clusterstats_indices_count`                                  | CounterValue | 完成的字节数           |
| `elasticsearch_clusterstats_indices_completion_size_in_bytes`               | CounterValue | 完成的字节数           |
| `elasticsearch_clusterstats_indices_docs_count`                             | GaugeValue   | 此集群上的文档计数        |
| `elasticsearch_clusterstats_indices_docs_deleted`                           | GaugeValue   | 此集群上已删除的文档计数     |
| `elasticsearch_clusterstats_indices_fielddata_evictions`                    | CounterValue | 字段数据的驱逐次数        |
| `elasticsearch_clusterstats_indices_fielddata_memory_size_in_bytes`         | GaugeValue   | 字段数据缓存使用的内存量（字节） |
| `elasticsearch_clusterstats_indices_query_cache_cache_count`                | CounterValue | 查询缓存的缓存计数        |
| `elasticsearch_clusterstats_indices_query_cache_cache_size`                 | GaugeValue   | 查询缓存的缓存大小        |
| `elasticsearch_clusterstats_indices_query_cache_evictions`                  | CounterValue | 查询缓存的驱逐次数        |
| `elasticsearch_clusterstats_indices_query_cache_hit_count`                  | CounterValue | 查询缓存命中计数         |
| `elasticsearch_clusterstats_indices_query_cache_memory_size_in_bytes`       | GaugeValue   | 查询缓存内存使用量（字节）    |
| `elasticsearch_clusterstats_indices_query_cache_miss_count`                 | CounterValue | 查询缓存未命中计数        |
| `elasticsearch_clusterstats_indices_query_cache_total_count`                | CounterValue | 查询缓存总计数          |
| `elasticsearch_clusterstats_indices_segments_count`                         | GaugeValue   | 集群索引段计数          |
| `elasticsearch_clusterstats_indices_segments_doc_values_memory_in_bytes`    | GaugeValue   | 文档值内存使用量（字节）     |
| `elasticsearch_clusterstats_indices_segments_fixed_bit_set_memory_in_bytes` | GaugeValue   | 固定位集内存使用量（字节）    |
| `elasticsearch_clusterstats_indices_segments_index_writer_memory_in_bytes`  | GaugeValue   | 索引编写器内存使用量（字节）   |
| `elasticsearch_clusterstats_indices_segments_max_unsafe_auto_id_timestamp`  | GaugeValue   | 索引编写器内存使用量（字节）   |
| `elasticsearch_clusterstats_indices_segments_memory_in_bytes`               | GaugeValue   | 段当前内存大小（字节）      |
| `elasticsearch_clusterstats_indices_segments_norms_memory_in_bytes`         | GaugeValue   | 标准化内存使用量（字节）     |
| `elasticsearch_clusterstats_indices_segments_points_memory_in_bytes`        | GaugeValue   | 点值内存使用量（字节）      |
| `elasticsearch_clusterstats_indices_segments_stored_fields_memory_in_bytes` | GaugeValue   | 存储字段内存使用量（字节）    |
| `elasticsearch_clusterstats_indices_segments_term_vectors_memory_in_bytes`  | GaugeValue   | 术语向量内存使用量（字节）    |
| `elasticsearch_clusterstats_indices_segments_terms_memory_in_bytes`         | GaugeValue   | 术语内存使用量（字节）      |
| `elasticsearch_clusterstats_indices_segments_version_map_memory_in_bytes`   | GaugeValue   | 版本映射内存使用量（字节）    |
| `elasticsearch_clusterstats_indices_shards_total`                           | GaugeValue   | 集群中的总分片数         |
| `elasticsearch_clusterstats_indices_shards_replication`                     | GaugeValue   | 分片复制数            |
| `elasticsearch_clusterstats_indices_shards_primaries`                       | GaugeValue   | 集群中的主分片数         |
| `elasticsearch_clusterstats_indices_shards_index_primaries_avg`             | GaugeValue   | 每个索引的平均主分片数      |
| `elasticsearch_clusterstats_indices_shards_index_primaries_max`             | GaugeValue   | 每个索引的最大主分片数      |
| `elasticsearch_clusterstats_indices_shards_index_primaries_min`             | GaugeValue   | 每个索引的最小主分片数      |
| `elasticsearch_clusterstats_indices_shards_index_replication_avg`           | GaugeValue   | 每个索引的平均复制分片数     |
| `elasticsearch_clusterstats_indices_shards_index_replication_max`           | GaugeValue   | 每个索引的最大复制分片数     |
| `elasticsearch_clusterstats_indices_shards_index_replication_min`           | GaugeValue   | 每个索引的最小复制分片数     |
| `elasticsearch_clusterstats_indices_shards_index_shards_avg`                | GaugeValue   | 每个索引的平均分片数       |
| `elasticsearch_clusterstats_indices_shards_index_shards_max`                | GaugeValue   | 每个索引的最大分片数       |
| `elasticsearch_clusterstats_indices_shards_index_shards_min`                | GaugeValue   | 每个索引的最小分片数       |
| `elasticsearch_clusterstats_indices_store_size_in_bytes`                    | GaugeValue   | 当前存储大小（字节）       |
| `elasticsearch_clusterstats_indices_total_data_set_size_in_bytes`           | GaugeValue   | 数据集总大小（字节）       |
| `elasticsearch_clusterstats_indices_reserved_in_bytes`                      | GaugeValue   | 保留大小（字节）         |
| `elasticsearch_clusterstats_nodes_count_coordinating_only`                  | GaugeValue   | 仅协调节点计数          |
| `elasticsearch_clusterstats_nodes_count_data`                               | GaugeValue   | 数据节点计数           |
| `elasticsearch_clusterstats_nodes_count_ingest`                             | GaugeValue   | 摄取节点计数           |
| `elasticsearch_clusterstats_nodes_count_master`                             | GaugeValue   | 主节点计数            |
| `elasticsearch_clusterstats_nodes_count_total`                              | GaugeValue   | 集群中节点总计数         |
| `elasticsearch_clusterstats_nodes_fs_available_in_bytes`                    | GaugeValue   | 可用磁盘空间（字节）       |
| `elasticsearch_clusterstats_nodes_fs_free_in_bytes`                         | GaugeValue   | 空闲磁盘空间（字节）       |
| `elasticsearch_clusterstats_nodes_fs_total_in_bytes`                        | GaugeValue   | 磁盘总空间（字节）        |
| `elasticsearch_clusterstats_nodes_jvm_max_uptime_in_millis`                 | GaugeValue   | JVM最大运行时间（毫秒）    |
| `elasticsearch_clusterstats_nodes_jvm_mem_heap_max_in_bytes`                | GaugeValue   | 堆内存最大值（字节）       |
| `elasticsearch_clusterstats_nodes_jvm_mem_heap_used_in_bytes`               | GaugeValue   | 使用的堆内存（字节）       |
| `elasticsearch_clusterstats_nodes_jvm_threads`                              | GaugeValue   | JVM线程数           |
| `elasticsearch_clusterstats_nodes_network_types_http_types_security4`       | GaugeValue   | HTTP安全4网络类型      |
| `elasticsearch_clusterstats_nodes_network_types_transport_types_security4`  | GaugeValue   | 传输安全4网络类型        |
| `elasticsearch_clusterstats_nodes_os_allocated_processors`                  | GaugeValue   | 分配的处理器数          |
| `elasticsearch_clusterstats_nodes_os_available_processors`                  | GaugeValue   | 可用处理器数           |
| `elasticsearch_clusterstats_nodes_os_mem_free_in_bytes`                     | GaugeValue   | 空闲内存（字节）         |
| `elasticsearch_clusterstats_nodes_os_mem_free_percent`                      | GaugeValue   | 空闲内存百分比          |
| `elasticsearch_clusterstats_nodes_os_mem_total_in_bytes`                    | GaugeValue   | 总内存（字节）          |
| `elasticsearch_clusterstats_nodes_os_mem_used_in_bytes`                     | GaugeValue   | 使用的内存（字节）        |
| `elasticsearch_clusterstats_nodes_os_mem_used_percent`                      | GaugeValue   | 使用的内存百分比         |
| `elasticsearch_clusterstats_nodes_process_cpu_percent`                      | GaugeValue   | 进程CPU使用百分比       |
| `elasticsearch_clusterstats_nodes_process_open_file_descriptors_avg`        | GaugeValue   | 打开的文件描述符平均数      |
| `elasticsearch_clusterstats_nodes_process_open_file_descriptors_max`        | GaugeValue   | 打开的文件描述符最大数      |
| `elasticsearch_clusterstats_nodes_process_open_file_descriptors_min`        | GaugeValue   | 打开的文件描述符最小数      |

#### `export_data_stream = true`

| 名称                                                    | 类型           | 描述           |
|-------------------------------------------------------|--------------|--------------|
| `elasticsearch_data_stream_backing_indices_total`     | CounterValue | 后备索引的数量      |
| `elasticsearch_data_stream_store_size_bytes`          | CounterValue | 数据流的存储大小     |
| `elasticsearch_data_stream_stats_up`                  | gauge        | 数据流收集的上行指标   |
| `elasticsearch_data_stream_stats_total_scrapes`       | counter      | 数据流统计的总抓取次数  |
| `elasticsearch_data_stream_stats_json_parse_failures` | counter      | 数据流统计的解析失败次数 |

#### `all_nodes = true`

| 名称                                                             | 类型           | 描述                  |
|----------------------------------------------------------------|--------------|---------------------|
| `elasticsearch_os_cpu_load_average_1m`                         | GaugeValue   | 短期负载平均值             |
| `elasticsearch_os_cpu_load_average_5m`                         | GaugeValue   | 中期负载平均值             |
| `elasticsearch_os_cpu_load_average_15m`                        | GaugeValue   | 长期负载平均值             |
| `elasticsearch_os_cpu_percent`                                 | GaugeValue   | 操作系统使用的CPU百分比       |
| `elasticsearch_os_mem_free_in_bytes`                           | GaugeValue   | 可用物理内存量（字节）         |
| `elasticsearch_os_mem_used_in_bytes`                           | GaugeValue   | 已用物理内存量（字节）         |
| `elasticsearch_os_mem_actual_free_in_bytes`                    | GaugeValue   | 实际可用物理内存量（字节）       |
| `elasticsearch_os_mem_actual_used_in_bytes`                    | GaugeValue   | 实际已用物理内存量（字节）       |
| `elasticsearch_os_mem_used_percent`                            | GaugeValue   | 已用物理内存百分比           |
| `elasticsearch_os_mem_total_in_bytes`                          | GaugeValue   | 物理内存总量（字节）          |
| `elasticsearch_os_mem_free_percent`                            | GaugeValue   | 可用物理内存百分比           |
| `elasticsearch_os_cgroup_cpu_cfs_period_micros`                | GaugeValue   | CPU CFS周期（微秒）       |
| `elasticsearch_os_cgroup_cpu_cfs_quota_micros`                 | GaugeValue   | CPU CFS配额（微秒）       |
| `elasticsearch_os_cgroup_cpu_stat_number_of_elapsed_periods`   | GaugeValue   | CPU CFS配额中的时间段数量    |
| `elasticsearch_os_cgroup_cpu_stat_number_of_times_throttled`   | GaugeValue   | CPU CFS时间段被节流的次数    |
| `elasticsearch_os_cgroup_cpu_stat_time_throttled_nanos`        | GaugeValue   | CPU CFS被节流的时间（纳秒）   |
| `elasticsearch_os_cgroup_cpuacct_usage_nanos`                  | GaugeValue   | cpuacct使用时间（纳秒）     |
| `elasticsearch_os_swap_used_in_bytes`                          | GaugeValue   | 已用交换空间量（字节）         |
| `elasticsearch_os_swap_total_in_bytes`                         | GaugeValue   | 交换空间总量（字节）          |
| `elasticsearch_os_swap_free_in_bytes`                          | GaugeValue   | 可用交换空间量（字节）         |
| `elasticsearch_indices_fielddata_memory_size_in_bytes`         | GaugeValue   | 字段数据缓存内存使用量（字节）     |
| `elasticsearch_indices_fielddata_evictions`                    | CounterValue | 字段数据缓存逐出次数          |
| `elasticsearch_indices_completion_size_in_bytes`               | CounterValue | 自动完成数据大小（字节）        |
| `elasticsearch_indices_filter_cache_memory_size_in_bytes`      | GaugeValue   | 过滤器缓存内存使用量（字节）      |
| `elasticsearch_indices_filter_cache_evictions`                 | CounterValue | 过滤器缓存逐出次数           |
| `elasticsearch_indices_query_cache_memory_size_in_bytes`       | GaugeValue   | 查询缓存内存使用量（字节）       |
| `elasticsearch_indices_query_cache_evictions`                  | CounterValue | 查询缓存逐出次数            |
| `elasticsearch_indices_query_cache_total_count`                | CounterValue | 查询缓存总计数             |
| `elasticsearch_indices_query_cache_cache_size`                 | GaugeValue   | 查询缓存大小              |
| `elasticsearch_indices_query_cache_cache_count`                | CounterValue | 查询缓存缓存计数            |
| `elasticsearch_indices_query_cache_hit_count`                  | CounterValue | 查询缓存命中计数            |
| `elasticsearch_indices_query_cache_miss_count`                 | CounterValue | 查询缓存未命中计数           |
| `elasticsearch_indices_request_cache_memory_size_in_bytes`     | GaugeValue   | 请求缓存内存使用量（字节）       |
| `elasticsearch_indices_request_cache_evictions`                | CounterValue | 请求缓存逐出次数            |
| `elasticsearch_indices_request_cache_hit_count`                | CounterValue | 请求缓存命中计数            |
| `elasticsearch_indices_request_cache_miss_count`               | CounterValue | 请求缓存未命中计数           |
| `elasticsearch_indices_translog_operations`                    | CounterValue | 总事务日志操作数            |
| `elasticsearch_indices_translog_size_in_bytes`                 | GaugeValue   | 事务日志总大小（字节）         |
| `elasticsearch_indices_get_time_seconds`                       | CounterValue | 获取操作总耗时（秒）          |
| `elasticsearch_indices_get_total`                              | CounterValue | 总获取操作数              |
| `elasticsearch_indices_get_missing_time_seconds`               | CounterValue | 缺失文档获取操作总耗时（秒）      |
| `elasticsearch_indices_get_missing_total`                      | CounterValue | 缺失文档获取操作总数          |
| `elasticsearch_indices_get_exists_time_seconds`                | CounterValue | 存在文档获取操作总耗时（秒）      |
| `elasticsearch_indices_get_exists_total`                       | CounterValue | 存在文档获取操作总数          |
| `elasticsearch_indices_refresh_time_seconds_total`             | CounterValue | 刷新操作总耗时（秒）          |
| `elasticsearch_indices_refresh_total`                          | CounterValue | 总刷新操作数              |
| `elasticsearch_indices_search_query_time_seconds`              | CounterValue | 搜索查询总耗时（秒）          |
| `elasticsearch_indices_search_query_total`                     | CounterValue | 总搜索查询数              |
| `elasticsearch_indices_search_fetch_time_seconds`              | CounterValue | 搜索抓取总耗时（秒）          |
| `elasticsearch_indices_search_fetch_total`                     | CounterValue | 总搜索抓取数              |
| `elasticsearch_indices_search_suggest_total`                   | CounterValue | 总搜索建议数              |
| `elasticsearch_indices_search_suggest_time_seconds`            | CounterValue | 搜索建议总耗时（秒）          |
| `elasticsearch_indices_search_scroll_total`                    | CounterValue | 总滚动搜索数              |
| `elasticsearch_indices_search_scroll_time_seconds`             | CounterValue | 滚动搜索总耗时（秒）          |
| `elasticsearch_indices_docs_count`                             | GaugeValue   | 该节点上的文档总数           |
| `elasticsearch_indices_docs_deleted`                           | GaugeValue   | 该节点上被删除的文档数         |
| `elasticsearch_indices_store_size_in_bytes`                    | GaugeValue   | 存储的索引数据总大小（字节）      |
| `elasticsearch_indices_store_throttle_time_seconds_total`      | CounterValue | 索引存储节流时间总计（秒）       |
| `elasticsearch_indices_segments_memory_in_bytes`               | GaugeValue   | 当前段内存大小（字节）         |
| `elasticsearch_indices_segments_count`                         | GaugeValue   | 该节点上的索引段总数          |
| `elasticsearch_indices_segments_terms_memory_in_bytes`         | GaugeValue   | 该节点上术语的内存使用量（字节）    |
| `elasticsearch_indices_segments_index_writer_memory_in_bytes`  | GaugeValue   | 该节点上索引编写器的内存使用量（字节） |
| `elasticsearch_indices_segments_max_unsafe_auto_id_timestamp`  | GaugeValue   | 该节点上最大不安全自动ID时间戳    |
| `elasticsearch_indices_segments_norms_memory_in_bytes`         | GaugeValue   | 该节点上标准化值的内存使用量（字节）  |
| `elasticsearch_indices_segments_stored_fields_memory_in_bytes` | GaugeValue   | 存储字段的内存使用量（字节）      |
| `elasticsearch_indices_segments_doc_values_memory_in_bytes`    | GaugeValue   | 文档值的内存使用量（字节）       |
| `elasticsearch_indices_segments_fixed_bit_set_memory_in_bytes` | GaugeValue   | 固定位集的内存使用量（字节）      |
| `elasticsearch_indices_segments_term_vectors_memory_in_bytes`  | GaugeValue   | 术语向量的内存使用量（字节）      |
| `elasticsearch_indices_segments_points_memory_in_bytes`        | GaugeValue   | 点数据的内存使用量（字节）       |
| `elasticsearch_indices_segments_version_map_memory_in_bytes`   | GaugeValue   | 版本映射的内存使用量（字节）      |
| `elasticsearch_indices_flush_total`                            | CounterValue | 总刷新次数               |
| `elasticsearch_indices_flush_time_seconds`                     | CounterValue | 总刷新时间（秒）            |
| `elasticsearch_indices_warmer_total`                           | CounterValue | 总预热次数               |
| `elasticsearch_indices_warmer_time_seconds_total`              | CounterValue | 总预热时间（秒）            |
| `elasticsearch_indices_indexing_index_time_seconds_total`      | CounterValue | 索引时间总计（秒）           |
| `elasticsearch_indices_indexing_index_total`                   | CounterValue | 总索引次数               |
| `elasticsearch_indices_indexing_delete_time_seconds_total`     | CounterValue | 删除索引的总时间（秒）         |
| `elasticsearch_indices_indexing_delete_total`                  | CounterValue | 总删除次数               |
| `elasticsearch_indices_indexing_is_throttled`                  | GaugeValue   | 是否正在限制索引            |
| `elasticsearch_indices_indexing_throttle_time_seconds_total`   | CounterValue | 索引限流时间总计（秒）         |
| `elasticsearch_indices_merges_total`                           | CounterValue | 总合并次数               |
| `elasticsearch_indices_merges_current`                         | GaugeValue   | 当前合并次数              |
| `elasticsearch_indices_merges_current_size_in_bytes`           | GaugeValue   | 当前合并大小（字节）          |
| `elasticsearch_indices_merges_docs_total`                      | CounterValue | 文档合并总数              |
| `elasticsearch_indices_merges_total_size_in_bytes`             | CounterValue | 合并总大小（字节）           |
| `elasticsearch_indices_merges_total_time_seconds_total`        | CounterValue | 合并总时间（秒）            |
| `elasticsearch_indices_merges_total_throttled_time_seconds`    | CounterValue | 合并被限流的总时间（秒）        |
| `elasticsearch_jvm_threads_count`                              | GaugeValue   | JVM线程数              |
| `elasticsearch_jvm_threads_peak_count`                         | GaugeValue   | JVM线程峰值数            |
| `elasticsearch_jvm_timestamp`                                  | GaugeValue   | JVM时间戳              |
| `elasticsearch_jvm_mem_heap_used_in_bytes`                     | GaugeValue   | JVM堆使用的内存量（字节）      |
| `elasticsearch_jvm_mem_non_heap_used_in_bytes`                 | GaugeValue   | JVM非堆使用的内存量（字节）     |
| `elasticsearch_jvm_mem_heap_max_in_bytes`                      | GaugeValue   | JVM堆最大内存量（字节）       |
| `elasticsearch_jvm_mem_heap_used_percent`                      | GaugeValue   | JVM堆使用的内存百分比        |
| `elasticsearch_jvm_mem_heap_committed_in_bytes`                | GaugeValue   | JVM堆提交的内存量（字节）      |
| `elasticsearch_jvm_mem_non_heap_committed_in_bytes`            | GaugeValue   | JVM非堆提交的内存量（字节）     |
| `elasticsearch_jvm_memory_pools_young_used_in_bytes`           | GaugeValue   | JVM年轻代使用的内存量（字节）    |
| `elasticsearch_jvm_memory_pools_young_max_in_bytes`            | CounterValue | JVM年轻代最大内存量（字节）     |
| `elasticsearch_jvm_memory_pools_young_peak_used_in_bytes`      | CounterValue | JVM年轻代峰值使用的内存量（字节）  |
| `elasticsearch_jvm_memory_pools_young_peak_max_in_bytes`       | CounterValue | JVM年轻代峰值最大内存量（字节）   |
| `elasticsearch_jvm_memory_pools_survivor_used_in_bytes`        | GaugeValue   | JVM幸存区使用的内存量（字节）    |
| `elasticsearch_jvm_memory_pools_survivor_max_in_bytes`         | CounterValue | JVM幸存区最大内存量（字节）     |
| `elasticsearch_jvm_memory_pools_survivor_peak_used_in_bytes`   | CounterValue | JVM幸存区峰值使用的内存量（字节   |

#### `export_indices = true`

| 名称                                                                         | 类型           | 描述                          |
|----------------------------------------------------------------------------|--------------|-----------------------------|
| `elasticsearch_indices_stats_total_docs_count`                             | GaugeValue   | 文档总数                        |
| `elasticsearch_indices_stats_total_docs_deleted`                           | GaugeValue   | 已删除的文档总数                    |
| `elasticsearch_indices_stats_total_store_size_in_bytes`                    | GaugeValue   | 当前所有节点上所有分片存储的索引数据的总大小（字节）  |
| `elasticsearch_indices_stats_total_throttle_time_seconds`                  | GaugeValue   | 索引被节流的总时间（秒）                |
| `elasticsearch_indices_stats_total_segments_count`                         | GaugeValue   | 当前所有节点上所有分片的段数量             |
| `elasticsearch_indices_stats_total_segments_memory_in_bytes`               | GaugeValue   | 当前所有节点上所有分片的段占用内存大小（字节）     |
| `elasticsearch_indices_stats_total_segments_terms_memory_in_bytes`         | GaugeValue   | 当前所有节点上所有分片的词项占用内存大小（字节）    |
| `elasticsearch_indices_stats_total_segments_stored_fields_memory_in_bytes` | GaugeValue   | 当前所有节点上所有分片的存储字段占用内存大小（字节）  |
| `elasticsearch_indices_stats_total_segments_term_vectors_memory_in_bytes`  | GaugeValue   | 当前所有节点上所有分片的词向量占用内存大小（字节）   |
| `elasticsearch_indices_stats_total_segments_norms_memory_in_bytes`         | GaugeValue   | 当前所有节点上所有分片的规范化值占用内存大小（字节）  |
| `elasticsearch_indices_stats_total_segments_points_memory_in_bytes`        | GaugeValue   | 当前所有节点上所有分片的点数据占用内存大小（字节）   |
| `elasticsearch_indices_stats_total_segments_doc_values_memory_in_bytes`    | GaugeValue   | 当前所有节点上所有分片的文档值占用内存大小（字节）   |
| `elasticsearch_indices_stats_total_segments_index_writer_memory_in_bytes`  | GaugeValue   | 当前所有节点上所有分片的索引编写器占用内存大小（字节） |
| `elasticsearch_indices_stats_total_segments_version_map_memory_in_bytes`   | GaugeValue   | 当前所有节点上所有分片的版本映射占用内存大小（字节）  |
| `elasticsearch_indices_stats_total_segments_fixed_bit_set_memory_in_bytes` | GaugeValue   | 当前所有节点上所有分片的固定位集占用内存大小（字节）  |
| `elasticsearch_indices_stats_total_segments_max_unsafe_auto_id_timestamp`  | GaugeValue   | 当前所有节点上所有分片的最大不安全自动ID时间戳    |
| `elasticsearch_indices_stats_total_translog_earliest_last_modified_age`    | GaugeValue   | 当前所有节点上所有分片的事务日志中最早上次修改的年龄  |
| `elasticsearch_indices_stats_total_translog_operations`                    | GaugeValue   | 当前所有节点上所有分片的事务日志操作数量        |
| `elasticsearch_indices_stats_total_translog_size_in_bytes`                 | GaugeValue   | 当前所有节点上所有分片的事务日志大小（字节）      |
| `elasticsearch_indices_stats_total_translog_uncommitted_operations`        | GaugeValue   | 当前所有节点上所有分片的未提交的事务日志操作数量    |
| `elasticsearch_indices_stats_total_translog_uncommitted_size_in_bytes`     | GaugeValue   | 当前所有节点上所有分片的未提交事务日志大小（字节）   |
| `elasticsearch_indices_stats_total_completion_size_in_bytes`               | GaugeValue   | 当前所有节点上所有分片的自动完成数据大小（字节）    |
| `elasticsearch_indices_stats_total_search_query_time_seconds`              | CounterValue | 搜索查询总时间（秒）                  |
| `elasticsearch_indices_stats_total_search_query_current`                   | GaugeValue   | 当前活跃的搜索查询数量                 |
| `elasticsearch_indices_stats_total_search_open_contexts`                   | CounterValue | 打开的搜索上下文总数                  |
| `elasticsearch_indices_stats_total_search_query_total`                     | CounterValue | 搜索查询总数                      |
| `elasticsearch_indices_stats_total_search_fetch_time_seconds`              | CounterValue | 搜索抓取总时间（秒）                  |
| `elasticsearch_indices_stats_total_search_fetch`                           | CounterValue | 搜索抓取总次数                     |
| `elasticsearch_indices_stats_total_search_fetch_current`                   | CounterValue | 当前搜索抓取次数                    |
| `elasticsearch_indices_stats_total_search_scroll_time_seconds`             | CounterValue | 搜索滚动总时间（秒）                  |
| `elasticsearch_indices_stats_total_search_scroll_current`                  | GaugeValue   | 当前搜索滚动次数                    |
| `elasticsearch_indices_stats_total_search_scroll`                          | CounterValue | 搜索滚动总次数                     |
| `elasticsearch_indices_stats_total_search_suggest_time_seconds`            | CounterValue | 搜索建议总时间（秒）                  |
| `elasticsearch_indices_stats_total_search_suggest_total`                   | CounterValue | 搜索建议总次数                     |
| `elasticsearch_indices_stats_total_search_suggest_current`                 | CounterValue | 当前搜索建议次数                    |
| `elasticsearch_indices_stats_total_indexing_index_time_seconds`            | CounterValue | 索引索引总时间（秒）                  |
| `elasticsearch_indices_stats_total_index_current`                          | GaugeValue   | 当前正在索引的文档数                  |
| `elasticsearch_indices_stats_total_index_failed`                           | GaugeValue   | 索引失败的文档数                    |
| `elasticsearch_indices_stats_total_delete_current`                         | GaugeValue   | 当前正在处理的删除操作数                |
| `elasticsearch_indices_stats_total_indexing_index`                         | CounterValue | 索引索引操作总次数                   |
| `elasticsearch_indices_stats_total_indexing_delete_time_seconds`           | CounterValue | 索引删除操作总时间（秒）                |
| `elasticsearch_indices_stats_total_indexing_delete`                        | CounterValue | 索引删除操作总次数                   |
| `elasticsearch_indices_stats_total_indexing_noop_update`                   | CounterValue | 无操作更新总次数                    |
| `elasticsearch_indices_stats_total_indexing_throttle_time_seconds`         | CounterValue | 索引节流总时间（秒）                  |
| `elasticsearch_indices_stats_total_get_time_seconds`                       | CounterValue | 获取操作总时间（秒）                  |
| `elasticsearch_indices_stats_total_get_exists_total`                       | CounterValue | 存在检查操作总次数                   |
| `elasticsearch_indices_stats_total_get_exists_time_seconds`                | CounterValue | 存在检查操作总时间（秒）                |
| `elasticsearch_indices_stats_total_get_total`                              | CounterValue | 获取操作总次数                     |
| `elasticsearch_indices_stats_total_get_missing_total`                      | CounterValue | 缺失检查操作总次数                   |
| `elasticsearch_indices_stats_total_get_missing_time_seconds`               | CounterValue | 缺失检查操作总时间（秒）                |
| `elasticsearch_indices_stats_total_get_current`                            | CounterValue | 当前获取操作次数                    |
| `elasticsearch_indices_stats_total_merges_time_seconds`                    | CounterValue | 合并操作总时间（秒）                  |
| `elasticsearch_indices_stats_total_merges_total`                           | CounterValue | 合并操作总次数                     |
| `elasticsearch_indices_stats_total_merges_total_docs`                      | CounterValue | 合并操作处理的文档总数                 |
| `elasticsearch_indices_stats_total_merges_total_size_in_bytes`             | CounterValue | 合并操作处理的数据总大小（字节）            |
| `elasticsearch_indices_stats_total_merges_current`                         | CounterValue | 当前合并操作数                     |
| `elasticsearch_indices_stats_total_merges_current_docs`                    | CounterValue | 当前合并操作处理的文档数                |
| `elasticsearch_indices_stats_total_merges_current_size_in_bytes`           | CounterValue | 当前合并操作处理的数据大小（字节）           |
| `elasticsearch_indices_stats_total_merges_total_throttle_time_seconds`     | CounterValue | 合并操作I/O节流总时间（秒）             |
| `elasticsearch_indices_stats_total_merges_total_stopped_time_seconds`      | CounterValue | 允许较小合并完成的总大型合并停止时间（秒）       |
| `elasticsearch_indices_stats_total_merges_total_auto_throttle_bytes`       | CounterValue | 合并期间自动节流的总字节数               |
| `elasticsearch_indices_stats_total_refresh_external_total_time_seconds`    | CounterValue | 外部刷新总时间（秒）                  |
| `elasticsearch_indices_stats_total_refresh_external_total`                 | CounterValue | 外部刷新总次数                     |
| `elasticsearch_indices_stats_total_refresh_total_time_seconds`             | CounterValue | 刷新操作总时间（秒）                  |
| `elasticsearch_indices_stats_total_refresh_total`                          | CounterValue | 刷新操作总次数                     |
| `elasticsearch_indices_stats_total_refresh_listeners`                      | CounterValue | 刷新监听器总数                     |
| `elasticsearch_indices_stats_total_recovery_current_as_source`             | CounterValue | 作为源的当前恢复操作数                 |
| `elasticsearch_indices_stats_total_recovery_current_as_target`             | CounterValue | 作为目标的当前恢复操作数                |
| `elasticsearch_indices_stats_total_recovery_throttle_time_seconds`         | CounterValue | 恢复操作节流总时间（秒）                |
| `elasticsearch_indices_stats_total_flush_time_seconds_total`               | CounterValue | 刷新操作总时间（秒）                  |
| `elasticsearch_indices_stats_total_flush_total`                            | CounterValue | 刷新操作总次数                     |
| `elasticsearch_indices_stats_total_flush_periodic`                         | CounterValue | 周期性刷新总次数                    |
| `elasticsearch_indices_stats_total_warmer_time_seconds_total`              | CounterValue | 预热操作总时间（秒）                  |
| `elasticsearch_indices_stats_total_warmer_total`                           | CounterValue | 预热操作总次数                     |
| `elasticsearch_indices_stats_total_query_cache_memory_in_bytes`            | CounterValue | 查询缓存总内存（字节）                 |
| `elasticsearch_indices_stats_total_query_cache_size`                       | GaugeValue   | 查询缓存总大小                     |
| `elasticsearch_indices_stats_total_query_cache_total_count`                | CounterValue | 查询缓存操作总次数                   |
| `elasticsearch_indices_stats_total_query_cache_hit_count`                  | CounterValue | 查询缓存命中总次数                   |
| `elasticsearch_indices_stats_total_query_cache_miss_count`                 | CounterValue | 查询缓存未命中总次数                  |
| `elasticsearch_indices_stats_total_query_cache_cache_count`                | CounterValue | 查询缓存缓存总次数                   |
| `elasticsearch_indices_stats_total_query_cache_evictions`                  | CounterValue | 查询缓存逐出总次数                   |
| `elasticsearch_indices_stats_total_request_cache_memory_in_bytes`          | CounterValue | 请求缓存总内存（字节）                 |
| `elasticsearch_indices_stats_total_request_cache_hit_count`                | CounterValue | 请求缓存命中总次数                   |
| `elasticsearch_indices_stats_total_request_cache_miss_count`               | CounterValue | 请求缓存未命中总次数                  |
| `elasticsearch_indices_stats_total_request_cache_evictions`                | CounterValue | 请求缓存逐出总次数                   |
| `elasticsearch_indices_stats_total_fielddata_memory_in_bytes`              | CounterValue | 字段数据总内存（字节）                 |
| `elasticsearch_indices_stats_total_fielddata_evictions`                    | CounterValue | 字段数据逐出总次数                   |
| `elasticsearch_indices_stats_total_seq_no_global_checkpoint`               | CounterValue | 全局检查点                       |
| `elasticsearch_indices_stats_total_seq_no_local_checkpoint`                | CounterValue | 本地检查点                       |
| `elasticsearch_indices_stats_total_seq_no_max_seq_no`                      | CounterValue | 最大序列号                       |

| 名称                                                                              | 类型           | 描述                          |
|---------------------------------------------------------------------------------|--------------|-----------------------------|
| `elasticsearch_indices_stats_primaries_docs_count`                              | GaugeValue   | 文档总数                        |
| `elasticsearch_indices_stats_primaries_docs_deleted`                            | GaugeValue   | 已删除的文档总数                    |
| `elasticsearch_indices_stats_primaries_store_size_in_bytes`                     | GaugeValue   | 当前所有节点上所有分片存储的索引数据的总大小（字节）  |
| `elasticsearch_indices_stats_primaries_throttle_time_seconds`                   | GaugeValue   | 索引被节流的总时间（秒）                |
| `elasticsearch_indices_stats_primaries_segments_count`                          | GaugeValue   | 当前所有节点上所有分片的段数量             |
| `elasticsearch_indices_stats_primaries_segments_memory_in_bytes`                | GaugeValue   | 当前所有节点上所有分片的段占用内存大小（字节）     |
| `elasticsearch_indices_stats_primaries_segments_terms_memory_in_bytes`          | GaugeValue   | 当前所有节点上所有分片的词项占用内存大小（字节）    |
| `elasticsearch_indices_stats_primaries_segments_stored_fields_memory_in_bytes`  | GaugeValue   | 当前所有节点上所有分片的存储字段占用内存大小（字节）  |
| `elasticsearch_indices_stats_primaries_segments_term_vectors_memory_in_bytes`   | GaugeValue   | 当前所有节点上所有分片的词向量占用内存大小（字节）   |
| `elasticsearch_indices_stats_primaries_segments_norms_memory_in_bytes`          | GaugeValue   | 当前所有节点上所有分片的规范化值占用内存大小（字节）  |
| `elasticsearch_indices_stats_primaries_segments_points_memory_in_bytes`         | GaugeValue   | 当前所有节点上所有分片的点数据占用内存大小（字节）   |
| `elasticsearch_indices_stats_primaries_segments_doc_values_memory_in_bytes`     | GaugeValue   | 当前所有节点上所有分片的文档值占用内存大小（字节）   |
| `elasticsearch_indices_stats_primaries_segments_index_writer_memory_in_bytes`   | GaugeValue   | 当前所有节点上所有分片的索引编写器占用内存大小（字节） |
| `elasticsearch_indices_stats_primaries_segments_version_map_memory_in_bytes`    | GaugeValue   | 当前所有节点上所有分片的版本映射占用内存大小（字节）  |
| `elasticsearch_indices_stats_primaries_segments_fixed_bit_set_memory_in_bytes`  | GaugeValue   | 当前所有节点上所有分片的固定位集占用内存大小（字节）  |
| `elasticsearch_indices_stats_primaries_segments_max_unsafe_auto_id_timestamp`   | GaugeValue   | 当前所有节点上所有分片的最大不安全自动ID时间戳    |
| `elasticsearch_indices_stats_primaries_translog_earliest_last_modified_age`     | GaugeValue   | 当前所有节点上所有分片的事务日志中最早上次修改的年龄  |
| `elasticsearch_indices_stats_primaries_translog_operations`                     | GaugeValue   | 当前所有节点上所有分片的事务日志操作数量        |
| `elasticsearch_indices_stats_primaries_translog_size_in_bytes`                  | GaugeValue   | 当前所有节点上所有分片的事务日志大小（字节）      |
| `elasticsearch_indices_stats_primaries_translog_uncommitted_operations`         | GaugeValue   | 当前所有节点上所有分片的未提交的事务日志操作数量    |
| `elasticsearch_indices_stats_primaries_translog_uncommitted_size_in_bytes`      | GaugeValue   | 当前所有节点上所有分片的未提交事务日志大小（字节）   |
| `elasticsearch_indices_stats_primaries_completion_size_in_bytes`                | GaugeValue   | 当前所有节点上所有分片的自动完成数据大小（字节）    |
| `elasticsearch_indices_stats_primaries_search_query_time_seconds`               | CounterValue | 搜索查询总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_search_query_current`                    | GaugeValue   | 当前活跃的搜索查询数量                 |
| `elasticsearch_indices_stats_primaries_search_open_contexts`                    | CounterValue | 打开的搜索上下文总数                  |
| `elasticsearch_indices_stats_primaries_search_query_total`                      | CounterValue | 搜索查询总数                      |
| `elasticsearch_indices_stats_primaries_search_fetch_time_seconds`               | CounterValue | 搜索抓取总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_search_fetch`                            | CounterValue | 搜索抓取总次数                     |
| `elasticsearch_indices_stats_primaries_search_fetch_current`                    | CounterValue | 当前搜索抓取次数                    |
| `elasticsearch_indices_stats_primaries_search_scroll_time_seconds`              | CounterValue | 搜索滚动总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_search_scroll_current`                   | GaugeValue   | 当前搜索滚动次数                    |
| `elasticsearch_indices_stats_primaries_search_scroll`                           | CounterValue | 搜索滚动总次数                     |
| `elasticsearch_indices_stats_primaries_search_suggest_time_seconds`             | CounterValue | 搜索建议总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_search_suggest_total`                    | CounterValue | 搜索建议总次数                     |
| `elasticsearch_indices_stats_primaries_search_suggest_current`                  | CounterValue | 当前搜索建议次数                    |
| `elasticsearch_indices_stats_primaries_indexing_index_time_seconds`             | CounterValue | 索引索引总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_index_current`                           | GaugeValue   | 当前正在索引的文档数                  |
| `elasticsearch_indices_stats_primaries_index_failed`                            | GaugeValue   | 索引失败的文档数                    |
| `elasticsearch_indices_stats_primaries_delete_current`                          | GaugeValue   | 当前正在处理的删除操作数                |
| `elasticsearch_indices_stats_primaries_indexing_index`                          | CounterValue | 索引索引操作总次数                   |
| `elasticsearch_indices_stats_primaries_indexing_delete_time_seconds`            | CounterValue | 索引删除操作总时间（秒）                |
| `elasticsearch_indices_stats_primaries_indexing_delete`                         | CounterValue | 索引删除操作总次数                   |
| `elasticsearch_indices_stats_primaries_indexing_noop_update`                    | CounterValue | 无操作更新总次数                    |
| `elasticsearch_indices_stats_primaries_indexing_throttle_time_seconds`          | CounterValue | 索引节流总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_get_time_seconds`                        | CounterValue | 获取操作总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_get_exists_total`                        | CounterValue | 存在检查操作总次数                   |
| `elasticsearch_indices_stats_primaries_get_exists_time_seconds`                 | CounterValue | 存在检查操作总时间（秒）                |
| `elasticsearch_indices_stats_primaries_get_total`                               | CounterValue | 获取操作总次数                     |
| `elasticsearch_indices_stats_primaries_get_missing_total`                       | CounterValue | 缺失检查操作总次数                   |
| `elasticsearch_indices_stats_primaries_get_missing_time_seconds`                | CounterValue | 缺失检查操作总时间（秒）                |
| `elasticsearch_indices_stats_primaries_get_current`                             | CounterValue | 当前获取操作次数                    |
| `elasticsearch_indices_stats_primaries_merges_time_seconds`                     | CounterValue | 合并操作总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_merges_total`                            | CounterValue | 合并操作总次数                     |
| `elasticsearch_indices_stats_primaries_merges_primaries_docs`                   | CounterValue | 合并操作处理的文档总数                 |
| `elasticsearch_indices_stats_primaries_merges_primaries_size_in_bytes`          | CounterValue | 合并操作处理的数据总大小（字节）            |
| `elasticsearch_indices_stats_primaries_merges_current`                          | CounterValue | 当前合并操作数                     |
| `elasticsearch_indices_stats_primaries_merges_current_docs`                     | CounterValue | 当前合并操作处理的文档数                |
| `elasticsearch_indices_stats_primaries_merges_current_size_in_bytes`            | CounterValue | 当前合并操作处理的数据大小（字节）           |
| `elasticsearch_indices_stats_primaries_merges_primaries_throttle_time_seconds`  | CounterValue | 合并操作I/O节流总时间（秒）             |
| `elasticsearch_indices_stats_primaries_merges_primaries_stopped_time_seconds`   | CounterValue | 允许较小合并完成的总大型合并停止时间（秒）       |
| `elasticsearch_indices_stats_primaries_merges_primaries_auto_throttle_bytes`    | CounterValue | 合并期间自动节流的总字节数               |
| `elasticsearch_indices_stats_primaries_refresh_external_primaries_time_seconds` | CounterValue | 外部刷新总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_refresh_external_total`                  | CounterValue | 外部刷新总次数                     |
| `elasticsearch_indices_stats_primaries_refresh_primaries_time_seconds`          | CounterValue | 刷新操作总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_refresh_total`                           | CounterValue | 刷新操作总次数                     |
| `elasticsearch_indices_stats_primaries_refresh_listeners`                       | CounterValue | 刷新监听器总数                     |
| `elasticsearch_indices_stats_primaries_recovery_current_as_source`              | CounterValue | 作为源的当前恢复操作数                 |
| `elasticsearch_indices_stats_primaries_recovery_current_as_target`              | CounterValue | 作为目标的当前恢复操作数                |
| `elasticsearch_indices_stats_primaries_recovery_throttle_time_seconds`          | CounterValue | 恢复操作节流总时间（秒）                |
| `elasticsearch_indices_stats_primaries_flush_time_seconds_total`                | CounterValue | 刷新操作总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_flush_total`                             | CounterValue | 刷新操作总次数                     |
| `elasticsearch_indices_stats_primaries_flush_periodic`                          | CounterValue | 周期性刷新总次数                    |
| `elasticsearch_indices_stats_primaries_warmer_time_seconds_total`               | CounterValue | 预热操作总时间（秒）                  |
| `elasticsearch_indices_stats_primaries_warmer_total`                            | CounterValue | 预热操作总次数                     |
| `elasticsearch_indices_stats_primaries_query_cache_memory_in_bytes`             | CounterValue | 查询缓存总内存（字节）                 |
| `elasticsearch_indices_stats_primaries_query_cache_size`                        | GaugeValue   | 查询缓存总大小                     |
| `elasticsearch_indices_stats_primaries_query_cache_primaries_count`             | CounterValue | 查询缓存操作总次数                   |
| `elasticsearch_indices_stats_primaries_query_cache_hit_count`                   | CounterValue | 查询缓存命中总次数                   |
| `elasticsearch_indices_stats_primaries_query_cache_miss_count`                  | CounterValue | 查询缓存未命中总次数                  |
| `elasticsearch_indices_stats_primaries_query_cache_cache_count`                 | CounterValue | 查询缓存缓存总次数                   |
| `elasticsearch_indices_stats_primaries_query_cache_evictions`                   | CounterValue | 查询缓存逐出总次数                   |
| `elasticsearch_indices_stats_primaries_request_cache_memory_in_bytes`           | CounterValue | 请求缓存总内存（字节）                 |
| `elasticsearch_indices_stats_primaries_request_cache_hit_count`                 | CounterValue | 请求缓存命中总次数                   |
| `elasticsearch_indices_stats_primaries_request_cache_miss_count`                | CounterValue | 请求缓存未命中总次数                  |
| `elasticsearch_indices_stats_primaries_request_cache_evictions`                 | CounterValue | 请求缓存逐出总次数                   |
| `elasticsearch_indices_stats_primaries_fielddata_memory_in_bytes`               | CounterValue | 字段数据总内存（字节）                 |
| `elasticsearch_indices_stats_primaries_fielddata_evictions`                     | CounterValue | 字段数据逐出总次数                   |
| `elasticsearch_indices_stats_primaries_seq_no_global_checkpoint`                | CounterValue | 全局检查点                       |
| `elasticsearch_indices_stats_primaries_seq_no_local_checkpoint`                 | CounterValue | 本地检查点                       |
| `elasticsearch_indices_stats_primaries_seq_no_max_seq_no`                       | CounterValue | 最大序列号                       |

#### `export_indices_settings = true`

| 名称                                                        | 类型    | 帮助                                                    |  
|-----------------------------------------------------------|-------|-------------------------------------------------------|
| elasticsearch_indices_settings_creation_timestamp_seconds | gauge | 索引创建时间的时间戳，单位为秒                                       | 
| elasticsearch_indices_settings_stats_read_only_indices    | gauge | 设置为read_only_allow_delete=true的索引数量                   | 
| elasticsearch_indices_settings_total_fields               | gauge | 索引设置中index.mapping.total_fields.limit的值（索引中允许的映射字段总数） | 
| elasticsearch_indices_settings_replicas                   | gauge | 索引设置中index.replicas的值                                 |

#### `export_indices_mappings = true`

| 名称                                                             | 类型      | 帮助                           |
|----------------------------------------------------------------|---------|------------------------------|
| elasticsearch_indices_mappings_stats_fields                    | gauge   | 索引当前映射的字段数                   |
| elasticsearch_indices_mappings_stats_json_parse_failures_total | counter | 解析JSON时的错误数                  |
| elasticsearch_indices_mappings_stats_scrapes_total             | counter | 当前Elasticsearch索引映射抓取的总次数    |
| elasticsearch_indices_mappings_stats_up                        | gauge   | 上一次抓取Elasticsearch索引映射端点是否成功 |

#### `export_slm = true`

| 名称                                                       | 类型      | 帮助                   |
|----------------------------------------------------------|---------|----------------------|
| elasticsearch_slm_stats_up                               | gauge   | SLM收集器的上行指标          |
| elasticsearch_slm_stats_total_scrapes                    | counter | SLM收集器的抓取次数          |
| elasticsearch_slm_stats_json_parse_failures              | counter | SLM收集器的JSON解析失败次数    |
| elasticsearch_slm_stats_retention_runs_total             | counter | 保留运行总次数              |
| elasticsearch_slm_stats_retention_failed_total           | counter | 保留运行失败总次数            |
| elasticsearch_slm_stats_retention_timed_out_total        | counter | 保留运行超时总次数            |
| elasticsearch_slm_stats_retention_deletion_time_seconds  | gauge   | 保留运行删除时间             |
| elasticsearch_slm_stats_total_snapshots_taken_total      | counter | 总共拍摄的快照数             |
| elasticsearch_slm_stats_total_snapshots_failed_total     | counter | 快照失败总次数              |
| elasticsearch_slm_stats_total_snapshots_deleted_total    | counter | 快照删除总次数              |
| elasticsearch_slm_stats_snapshots_taken_total            | counter | 按策略拍摄的快照数            |
| elasticsearch_slm_stats_snapshots_failed_total           | counter | 按策略失败的快照数            |
| elasticsearch_slm_stats_snapshots_deleted_total          | counter | 按策略删除的快照数            |
| elasticsearch_slm_stats_snapshot_deletion_failures_total | counter | 按策略快照删除失败次数          |
| elasticsearch_slm_stats_operation_mode                   | gauge   | SLM操作模式（运行中，停止中，已停止） |
