redis_exporter metric alias:

```
metricMapGauges: map[string]string{
    // # Server
    "uptime_in_seconds": "uptime_in_seconds",
    "process_id":        "process_id",
    "io_threads_active": "io_threads_active",

    // # Clients
    "connected_clients":        "connected_clients",
    "blocked_clients":          "blocked_clients",
    "tracking_clients":         "tracking_clients",
    "clients_in_timeout_table": "clients_in_timeout_table",

    // redis 2,3,4.x
    "client_longest_output_list": "client_longest_output_list",
    "client_biggest_input_buf":   "client_biggest_input_buf",

    // the above two metrics were renamed in redis 5.x
    "client_recent_max_output_buffer": "client_recent_max_output_buffer_bytes",
    "client_recent_max_input_buffer":  "client_recent_max_input_buffer_bytes",

    // # Memory
    "allocator_active":     "allocator_active_bytes",
    "allocator_allocated":  "allocator_allocated_bytes",
    "allocator_resident":   "allocator_resident_bytes",
    "allocator_frag_ratio": "allocator_frag_ratio",
    "allocator_frag_bytes": "allocator_frag_bytes",
    "allocator_rss_ratio":  "allocator_rss_ratio",
    "allocator_rss_bytes":  "allocator_rss_bytes",

    "used_memory":              "memory_used_bytes",
    "used_memory_rss":          "memory_used_rss_bytes",
    "used_memory_peak":         "memory_used_peak_bytes",
    "used_memory_lua":          "memory_used_lua_bytes",
    "used_memory_overhead":     "memory_used_overhead_bytes",
    "used_memory_startup":      "memory_used_startup_bytes",
    "used_memory_dataset":      "memory_used_dataset_bytes",
    "used_memory_scripts":      "memory_used_scripts_bytes",
    "number_of_cached_scripts": "number_of_cached_scripts",
    "maxmemory":                "memory_max_bytes",

    "maxmemory_reservation":         "memory_max_reservation_bytes",
    "maxmemory_desired_reservation": "memory_max_reservation_desired_bytes",

    "maxfragmentationmemory_reservation":         "memory_max_fragmentation_reservation_bytes",
    "maxfragmentationmemory_desired_reservation": "memory_max_fragmentation_reservation_desired_bytes",

    "mem_fragmentation_ratio": "mem_fragmentation_ratio",
    "mem_fragmentation_bytes": "mem_fragmentation_bytes",
    "mem_clients_slaves":      "mem_clients_slaves",
    "mem_clients_normal":      "mem_clients_normal",

    "expired_stale_perc": "expired_stale_percentage",

    // https://github.com/antirez/redis/blob/17bf0b25c1171486e3a1b089f3181fff2bc0d4f0/src/evict.c#L349-L352
    // ... the sum of AOF and slaves buffer ....
    "mem_not_counted_for_evict": "mem_not_counted_for_eviction_bytes",

    "lazyfree_pending_objects": "lazyfree_pending_objects",
    "active_defrag_running":    "active_defrag_running",

    "migrate_cached_sockets": "migrate_cached_sockets_total",

    "active_defrag_hits":       "defrag_hits",
    "active_defrag_misses":     "defrag_misses",
    "active_defrag_key_hits":   "defrag_key_hits",
    "active_defrag_key_misses": "defrag_key_misses",

    // https://github.com/antirez/redis/blob/0af467d18f9d12b137af3b709c0af579c29d8414/src/expire.c#L297-L299
    "expired_time_cap_reached_count": "expired_time_cap_reached_total",

    // # Persistence
    "loading":                      "loading_dump_file",
    "rdb_changes_since_last_save":  "rdb_changes_since_last_save",
    "rdb_bgsave_in_progress":       "rdb_bgsave_in_progress",
    "rdb_last_save_time":           "rdb_last_save_timestamp_seconds",
    "rdb_last_bgsave_status":       "rdb_last_bgsave_status",
    "rdb_last_bgsave_time_sec":     "rdb_last_bgsave_duration_sec",
    "rdb_current_bgsave_time_sec":  "rdb_current_bgsave_duration_sec",
    "rdb_last_cow_size":            "rdb_last_cow_size_bytes",
    "aof_enabled":                  "aof_enabled",
    "aof_rewrite_in_progress":      "aof_rewrite_in_progress",
    "aof_rewrite_scheduled":        "aof_rewrite_scheduled",
    "aof_last_rewrite_time_sec":    "aof_last_rewrite_duration_sec",
    "aof_current_rewrite_time_sec": "aof_current_rewrite_duration_sec",
    "aof_last_cow_size":            "aof_last_cow_size_bytes",
    "aof_current_size":             "aof_current_size_bytes",
    "aof_base_size":                "aof_base_size_bytes",
    "aof_pending_rewrite":          "aof_pending_rewrite",
    "aof_buffer_length":            "aof_buffer_length",
    "aof_rewrite_buffer_length":    "aof_rewrite_buffer_length",
    "aof_pending_bio_fsync":        "aof_pending_bio_fsync",
    "aof_delayed_fsync":            "aof_delayed_fsync",
    "aof_last_bgrewrite_status":    "aof_last_bgrewrite_status",
    "aof_last_write_status":        "aof_last_write_status",
    "module_fork_in_progress":      "module_fork_in_progress",
    "module_fork_last_cow_size":    "module_fork_last_cow_size",

    // # Stats
    "pubsub_channels":         "pubsub_channels",
    "pubsub_patterns":         "pubsub_patterns",
    "latest_fork_usec":        "latest_fork_usec",
    "tracking_total_keys":     "tracking_total_keys",
    "tracking_total_items":    "tracking_total_items",
    "tracking_total_prefixes": "tracking_total_prefixes",

    // # Replication
    "connected_slaves":               "connected_slaves",
    "repl_backlog_size":              "replication_backlog_bytes",
    "repl_backlog_active":            "repl_backlog_is_active",
    "repl_backlog_first_byte_offset": "repl_backlog_first_byte_offset",
    "repl_backlog_histlen":           "repl_backlog_history_bytes",
    "master_repl_offset":             "master_repl_offset",
    "second_repl_offset":             "second_repl_offset",
    "slave_expires_tracked_keys":     "slave_expires_tracked_keys",
    "slave_priority":                 "slave_priority",
    "sync_full":                      "replica_resyncs_full",
    "sync_partial_ok":                "replica_partial_resync_accepted",
    "sync_partial_err":               "replica_partial_resync_denied",

    // # Cluster
    "cluster_stats_messages_sent":     "cluster_messages_sent_total",
    "cluster_stats_messages_received": "cluster_messages_received_total",

    // # Tile38
    // based on https://tile38.com/commands/server/
    "tile38_aof_size":             "tile38_aof_size_bytes",
    "tile38_avg_point_size":       "tile38_avg_item_size_bytes",
    "tile38_sys_cpus":             "tile38_cpus_total",
    "tile38_heap_released_bytes":  "tile38_heap_released_bytes",
    "tile38_heap_alloc_bytes":     "tile38_heap_size_bytes",
    "tile38_http_transport":       "tile38_http_transport",
    "tile38_in_memory_size":       "tile38_in_memory_size_bytes",
    "tile38_max_heap_size":        "tile38_max_heap_size_bytes",
    "tile38_alloc_bytes":          "tile38_mem_alloc_bytes",
    "tile38_num_collections":      "tile38_num_collections_total",
    "tile38_num_hooks":            "tile38_num_hooks_total",
    "tile38_num_objects":          "tile38_num_objects_total",
    "tile38_num_points":           "tile38_num_points_total",
    "tile38_pointer_size":         "tile38_pointer_size_bytes",
    "tile38_read_only":            "tile38_read_only",
    "tile38_go_threads":           "tile38_threads_total",
    "tile38_go_goroutines":        "tile38_go_goroutines_total",
    "tile38_last_gc_time_seconds": "tile38_last_gc_time_seconds",
    "tile38_next_gc_bytes":        "tile38_next_gc_bytes",

    // addtl. KeyDB metrics
    "server_threads":        "server_threads_total",
    "long_lock_waits":       "long_lock_waits_total",
    "current_client_thread": "current_client_thread",
},

metricMapCounters: map[string]string{
    "total_connections_received": "connections_received_total",
    "total_commands_processed":   "commands_processed_total",

    "rejected_connections":   "rejected_connections_total",
    "total_net_input_bytes":  "net_input_bytes_total",
    "total_net_output_bytes": "net_output_bytes_total",

    "expired_keys":    "expired_keys_total",
    "evicted_keys":    "evicted_keys_total",
    "keyspace_hits":   "keyspace_hits_total",
    "keyspace_misses": "keyspace_misses_total",

    "used_cpu_sys":              "cpu_sys_seconds_total",
    "used_cpu_user":             "cpu_user_seconds_total",
    "used_cpu_sys_children":     "cpu_sys_children_seconds_total",
    "used_cpu_user_children":    "cpu_user_children_seconds_total",
    "used_cpu_sys_main_thread":  "cpu_sys_main_thread_seconds_total",
    "used_cpu_user_main_thread": "cpu_user_main_thread_seconds_total",

    "unexpected_error_replies":     "unexpected_error_replies",
    "total_error_replies":          "total_error_replies",
    "total_reads_processed":        "total_reads_processed",
    "total_writes_processed":       "total_writes_processed",
    "io_threaded_reads_processed":  "io_threaded_reads_processed",
    "io_threaded_writes_processed": "io_threaded_writes_processed",
    "dump_payload_sanitizations":   "dump_payload_sanitizations",
}
```
