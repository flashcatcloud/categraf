package mysql

var STATUS_VARS = map[string]struct{}{
	"prepared_stmt_count":        struct{}{}, // command metrics
	"slow_queries":               struct{}{},
	"questions":                  struct{}{},
	"queries":                    struct{}{},
	"com_select":                 struct{}{},
	"com_insert":                 struct{}{},
	"com_update":                 struct{}{},
	"com_delete":                 struct{}{},
	"com_replace":                struct{}{},
	"com_load":                   struct{}{},
	"com_insert_select":          struct{}{},
	"com_update_multi":           struct{}{},
	"com_delete_multi":           struct{}{},
	"com_replace_select":         struct{}{},
	"connections":                struct{}{}, // connection metrics
	"max_used_connections":       struct{}{},
	"aborted_clients":            struct{}{},
	"aborted_connects":           struct{}{},
	"open_files":                 struct{}{}, // table cache metrics
	"open_tables":                struct{}{},
	"bytes_sent":                 struct{}{}, // network metrics
	"bytes_received":             struct{}{},
	"qcache_hits":                struct{}{}, // query cache metrics
	"qcache_inserts":             struct{}{},
	"qcache_lowmem_prunes":       struct{}{},
	"table_locks_waited":         struct{}{}, // table lock metrics
	"table_locks_waited_rate":    struct{}{},
	"created_tmp_tables":         struct{}{}, // temporary table metrics
	"created_tmp_disk_tables":    struct{}{},
	"created_tmp_files":          struct{}{},
	"threads_connected":          struct{}{}, // thread metrics
	"threads_running":            struct{}{},
	"key_buffer_bytes_unflushed": struct{}{}, // myisam metrics
	"key_buffer_bytes_used":      struct{}{},
	"key_read_requests":          struct{}{},
	"key_reads":                  struct{}{},
	"key_write_requests":         struct{}{},
	"key_writes":                 struct{}{},
}

var VARIABLES_VARS = map[string]struct{}{
	"key_buffer_size":         struct{}{},
	"key_cache_utilization":   struct{}{},
	"max_connections":         struct{}{},
	"max_prepared_stmt_count": struct{}{},
	"query_cache_size":        struct{}{},
	"table_open_cache":        struct{}{},
	"thread_cache_size":       struct{}{},
}

var INNODB_VARS = map[string]struct{}{
	"innodb_data_reads":                    struct{}{},
	"innodb_data_writes":                   struct{}{},
	"innodb_os_log_fsyncs":                 struct{}{},
	"innodb_mutex_spin_waits":              struct{}{},
	"innodb_mutex_spin_rounds":             struct{}{},
	"innodb_mutex_os_waits":                struct{}{},
	"innodb_row_lock_waits":                struct{}{},
	"innodb_row_lock_time":                 struct{}{},
	"innodb_row_lock_current_waits":        struct{}{},
	"innodb_current_row_locks":             struct{}{},
	"innodb_buffer_pool_bytes_dirty":       struct{}{},
	"innodb_buffer_pool_bytes_free":        struct{}{},
	"innodb_buffer_pool_bytes_used":        struct{}{},
	"innodb_buffer_pool_bytes_total":       struct{}{},
	"innodb_buffer_pool_read_requests":     struct{}{},
	"innodb_buffer_pool_reads":             struct{}{},
	"innodb_buffer_pool_pages_utilization": struct{}{},
}

var BINLOG_VARS = map[string]struct{}{
	"binlog_space_usage_bytes": struct{}{},
}

var OPTIONAL_STATUS_VARS = map[string]struct{}{
	"binlog_cache_disk_use":      struct{}{},
	"binlog_cache_use":           struct{}{},
	"handler_commit":             struct{}{},
	"handler_delete":             struct{}{},
	"handler_prepare":            struct{}{},
	"handler_read_first":         struct{}{},
	"handler_read_key":           struct{}{},
	"handler_read_next":          struct{}{},
	"handler_read_prev":          struct{}{},
	"handler_read_rnd":           struct{}{},
	"handler_read_rnd_next":      struct{}{},
	"handler_rollback":           struct{}{},
	"handler_update":             struct{}{},
	"handler_write":              struct{}{},
	"opened_tables":              struct{}{},
	"qcache_total_blocks":        struct{}{},
	"qcache_free_blocks":         struct{}{},
	"qcache_free_memory":         struct{}{},
	"qcache_not_cached":          struct{}{},
	"qcache_queries_in_cache":    struct{}{},
	"select_full_join":           struct{}{},
	"select_full_range_join":     struct{}{},
	"select_range":               struct{}{},
	"select_range_check":         struct{}{},
	"select_scan":                struct{}{},
	"sort_merge_passes":          struct{}{},
	"sort_range":                 struct{}{},
	"sort_rows":                  struct{}{},
	"sort_scan":                  struct{}{},
	"table_locks_immediate":      struct{}{},
	"table_locks_immediate_rate": struct{}{},
	"threads_cached":             struct{}{},
	"threads_created":            struct{}{},
	"table_open_cache_hits":      struct{}{}, // Status Vars added in Mysql 5.6.6
	"table_open_cache_misses":    struct{}{},
}

var OPTIONAL_INNODB_VARS = map[string]struct{}{
	"innodb_active_transactions":            struct{}{},
	"innodb_buffer_pool_bytes_data":         struct{}{},
	"innodb_buffer_pool_pages_data":         struct{}{},
	"innodb_buffer_pool_pages_dirty":        struct{}{},
	"innodb_buffer_pool_pages_flushed":      struct{}{},
	"innodb_buffer_pool_pages_free":         struct{}{},
	"innodb_buffer_pool_pages_total":        struct{}{},
	"innodb_buffer_pool_read_ahead":         struct{}{},
	"innodb_buffer_pool_read_ahead_evicted": struct{}{},
	"innodb_buffer_pool_read_ahead_rnd":     struct{}{},
	"innodb_buffer_pool_wait_free":          struct{}{},
	"innodb_buffer_pool_write_requests":     struct{}{},
	"innodb_checkpoint_age":                 struct{}{},
	"innodb_current_transactions":           struct{}{},
	"innodb_data_fsyncs":                    struct{}{},
	"innodb_data_pending_fsyncs":            struct{}{},
	"innodb_data_pending_reads":             struct{}{},
	"innodb_data_pending_writes":            struct{}{},
	"innodb_data_read":                      struct{}{},
	"innodb_data_written":                   struct{}{},
	"innodb_dblwr_pages_written":            struct{}{},
	"innodb_dblwr_writes":                   struct{}{},
	"innodb_hash_index_cells_total":         struct{}{},
	"innodb_hash_index_cells_used":          struct{}{},
	"innodb_history_list_length":            struct{}{},
	"innodb_ibuf_free_list":                 struct{}{},
	"innodb_ibuf_merged":                    struct{}{},
	"innodb_ibuf_merged_delete_marks":       struct{}{},
	"innodb_ibuf_merged_deletes":            struct{}{},
	"innodb_ibuf_merged_inserts":            struct{}{},
	"innodb_ibuf_merges":                    struct{}{},
	"innodb_ibuf_segment_size":              struct{}{},
	"innodb_ibuf_size":                      struct{}{},
	"innodb_lock_structs":                   struct{}{},
	"innodb_locked_tables":                  struct{}{},
	"innodb_locked_transactions":            struct{}{},
	"innodb_log_waits":                      struct{}{},
	"innodb_log_write_requests":             struct{}{},
	"innodb_log_writes":                     struct{}{},
	"innodb_lsn_current":                    struct{}{},
	"innodb_lsn_flushed":                    struct{}{},
	"innodb_lsn_last_checkpoint":            struct{}{},
	"innodb_mem_adaptive_hash":              struct{}{},
	"innodb_mem_additional_pool":            struct{}{},
	"innodb_mem_dictionary":                 struct{}{},
	"innodb_mem_file_system":                struct{}{},
	"innodb_mem_lock_system":                struct{}{},
	"innodb_mem_page_hash":                  struct{}{},
	"innodb_mem_recovery_system":            struct{}{},
	"innodb_mem_thread_hash":                struct{}{},
	"innodb_mem_total":                      struct{}{},
	"innodb_os_file_fsyncs":                 struct{}{},
	"innodb_os_file_reads":                  struct{}{},
	"innodb_os_file_writes":                 struct{}{},
	"innodb_os_log_pending_fsyncs":          struct{}{},
	"innodb_os_log_pending_writes":          struct{}{},
	"innodb_os_log_written":                 struct{}{},
	"innodb_pages_created":                  struct{}{},
	"innodb_pages_read":                     struct{}{},
	"innodb_pages_written":                  struct{}{},
	"innodb_pending_aio_log_ios":            struct{}{},
	"innodb_pending_aio_sync_ios":           struct{}{},
	"innodb_pending_buffer_pool_flushes":    struct{}{},
	"innodb_pending_checkpoint_writes":      struct{}{},
	"innodb_pending_ibuf_aio_reads":         struct{}{},
	"innodb_pending_log_flushes":            struct{}{},
	"innodb_pending_log_writes":             struct{}{},
	"innodb_pending_normal_aio_reads":       struct{}{},
	"innodb_pending_normal_aio_writes":      struct{}{},
	"innodb_queries_inside":                 struct{}{},
	"innodb_queries_queued":                 struct{}{},
	"innodb_read_views":                     struct{}{},
	"innodb_rows_deleted":                   struct{}{},
	"innodb_rows_inserted":                  struct{}{},
	"innodb_rows_read":                      struct{}{},
	"innodb_rows_updated":                   struct{}{},
	"innodb_s_lock_os_waits":                struct{}{},
	"innodb_s_lock_spin_rounds":             struct{}{},
	"innodb_s_lock_spin_waits":              struct{}{},
	"innodb_semaphore_wait_time":            struct{}{},
	"innodb_semaphore_waits":                struct{}{},
	"innodb_tables_in_use":                  struct{}{},
	"innodb_x_lock_os_waits":                struct{}{},
	"innodb_x_lock_spin_rounds":             struct{}{},
	"innodb_x_lock_spin_waits":              struct{}{},
}

var GALERA_VARS = map[string]struct{}{
	"wsrep_cluster_size":           struct{}{},
	"wsrep_local_recv_queue_avg":   struct{}{},
	"wsrep_flow_control_paused":    struct{}{},
	"wsrep_flow_control_paused_ns": struct{}{},
	"wsrep_flow_control_recv":      struct{}{},
	"wsrep_flow_control_sent":      struct{}{},
	"wsrep_cert_deps_distance":     struct{}{},
	"wsrep_local_send_queue_avg":   struct{}{},
	"wsrep_replicated_bytes":       struct{}{},
	"wsrep_received_bytes":         struct{}{},
	"wsrep_received":               struct{}{},
	"wsrep_local_state":            struct{}{},
	"wsrep_local_cert_failures":    struct{}{},
}

var PERFORMANCE_VARS = map[string]struct{}{
	"query_run_time_avg":                 struct{}{},
	"perf_digest_95th_percentile_avg_us": struct{}{},
}

var SCHEMA_VARS = map[string]struct{}{
	"information_schema_size": struct{}{},
}

var TABLE_VARS = map[string]struct{}{
	"information_table_index_size": struct{}{},
	"information_table_data_size":  struct{}{},
}

var REPLICA_VARS = map[string]struct{}{
	"seconds_behind_source": struct{}{},
	"seconds_behind_master": struct{}{},
	"replicas_connected":    struct{}{},
}

var GROUP_REPLICATION_VARS = map[string]struct{}{
	"transactions_count":                struct{}{},
	"transactions_check":                struct{}{},
	"conflict_detected":                 struct{}{},
	"transactions_row_validating":       struct{}{},
	"transactions_remote_applier_queue": struct{}{},
	"transactions_remote_applied":       struct{}{},
	"transactions_local_proposed":       struct{}{},
	"transactions_local_rollback":       struct{}{},
}

var SYNTHETIC_VARS = map[string]struct{}{
	"qcache_utilization":         struct{}{},
	"qcache_instant_utilization": struct{}{},
}
