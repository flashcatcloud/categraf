package mysql

import (
	"database/sql"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
)

// Regexp to match various groups of status vars.
var globalStatusRE = regexp.MustCompile(`^(com|handler|connection_errors|innodb_buffer_pool_pages|innodb_rows|performance_schema)_(.*)$`)

func (ins *Instance) gatherGlobalStatus(slist *types.SampleList, db *sql.DB, globalTags map[string]string, cache map[string]float64) {
	rows, err := db.Query(SQL_GLOBAL_STATUS)
	if err != nil {
		log.Println("E! failed to query global status:", err)
		return
	}

	defer rows.Close()

	var (
		tags      = tagx.Copy(globalTags)
		textItems = map[string]string{
			"wsrep_local_state_uuid":   "",
			"wsrep_cluster_state_uuid": "",
			"wsrep_provider_version":   "",
			"wsrep_evs_repl_latency":   "",
		}
	)

	for rows.Next() {
		var key string
		var val sql.RawBytes

		if err = rows.Scan(&key, &val); err != nil {
			continue
		}

		// key to lower
		key = strings.ToLower(key)

		// collect some string fields
		if _, has := textItems[key]; has {
			textItems[key] = string(val)
			continue
		}

		if floatVal, ok := parseStatus(val); ok {
			cache[key] = floatVal

			// collect float fields
			if _, has := ins.validMetrics[key]; !has {
				continue
			}

			match := globalStatusRE.FindStringSubmatch(key)
			if match == nil {
				slist.PushFront(types.NewSample(inputName, "global_status_"+key, floatVal, tags))
				continue
			}

			switch match[1] {
			case "com":
				// Total number of executed MySQL commands.
				slist.PushFront(types.NewSample(inputName, "global_status_commands_total", floatVal, tags, map[string]string{"command": match[2]}))
			case "handler":
				// Total number of executed MySQL handlers.
				slist.PushFront(types.NewSample(inputName, "global_status_handlers_total", floatVal, tags, map[string]string{"handler": match[2]}))
			case "connection_errors":
				// Total number of MySQL connection errors.
				slist.PushFront(types.NewSample(inputName, "global_status_connection_errors_total", floatVal, tags, map[string]string{"error": match[2]}))
			case "innodb_buffer_pool_pages":
				switch match[2] {
				case "data", "free", "misc", "old", "total", "dirty":
					// Innodb buffer pool pages by state.
					slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_pages_"+match[2], floatVal, tags))
				default:
					// Innodb buffer pool page state changes.
					slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_page_changes_total", floatVal, tags, map[string]string{"operation": match[2]}))
				}
			case "innodb_rows":
				// Total number of MySQL InnoDB row operations.
				slist.PushFront(types.NewSample(inputName, "global_status_innodb_row_ops_total", floatVal, tags, map[string]string{"operation": match[2]}))
			case "performance_schema":
				// Total number of MySQL instrumentations that could not be loaded or created due to memory constraints.
				slist.PushFront(types.NewSample(inputName, "global_status_performance_schema_lost_total", floatVal, tags, map[string]string{"instrumentation": match[2]}))
			}
		}
	}

	// mysql_galera_variables_info metric.
	if textItems["wsrep_local_state_uuid"] != "" {
		slist.PushFront(types.NewSample(inputName, "galera_status_info", 1, tags, map[string]string{
			"wsrep_local_state_uuid":   textItems["wsrep_local_state_uuid"],
			"wsrep_cluster_state_uuid": textItems["wsrep_cluster_state_uuid"],
			"wsrep_provider_version":   textItems["wsrep_provider_version"],
		}))
	}

	// mysql_galera_evs_repl_latency
	if textItems["wsrep_evs_repl_latency"] != "" {
		type evsValue struct {
			name  string
			value float64
			index int
			help  string
		}

		evsMap := []evsValue{
			{name: "min_seconds", value: 0, index: 0, help: "PXC/Galera group communication latency. Min value."},
			{name: "avg_seconds", value: 0, index: 1, help: "PXC/Galera group communication latency. Avg value."},
			{name: "max_seconds", value: 0, index: 2, help: "PXC/Galera group communication latency. Max value."},
			{name: "stdev", value: 0, index: 3, help: "PXC/Galera group communication latency. Standard Deviation."},
			{name: "sample_size", value: 0, index: 4, help: "PXC/Galera group communication latency. Sample Size."},
		}

		evsParsingSuccess := true
		values := strings.Split(textItems["wsrep_evs_repl_latency"], "/")

		if len(evsMap) == len(values) {
			for i, v := range evsMap {
				evsMap[i].value, err = strconv.ParseFloat(values[v.index], 64)
				if err != nil {
					evsParsingSuccess = false
				}
			}

			if evsParsingSuccess {
				for _, v := range evsMap {
					slist.PushFront(types.NewSample(inputName, "galera_evs_repl_latency_"+v.name, v.value, tags))
				}
			}
		}
	}
}

func parseStatus(data sql.RawBytes) (float64, bool) {
	dataString := strings.ToLower(string(data))
	switch dataString {
	case "yes", "on":
		return 1, true
	case "no", "off", "disabled":
		return 0, true
	// SHOW SLAVE STATUS Slave_IO_Running can return "Connecting" which is a non-running state.
	case "connecting":
		return 0, true
	// SHOW GLOBAL STATUS like 'wsrep_cluster_status' can return "Primary" or "non-Primary"/"Disconnected"
	case "primary":
		return 1, true
	case "non-primary", "disconnected":
		return 0, true
	}
	if ts, err := time.Parse("Jan 02 15:04:05 2006 MST", string(data)); err == nil {
		return float64(ts.Unix()), true
	}
	if ts, err := time.Parse("2006-01-02 15:04:05", string(data)); err == nil {
		return float64(ts.Unix()), true
	}
	value, err := strconv.ParseFloat(string(data), 64)
	return value, err == nil
}
