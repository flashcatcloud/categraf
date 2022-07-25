package mysql

import (
	"database/sql"
	"log"
	"regexp"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
)

func (ins *Instance) gatherGlobalVariables(slist *types.SampleList, db *sql.DB, globalTags map[string]string, cache map[string]float64) {
	rows, err := db.Query(SQL_GLOBAL_VARIABLES)
	if err != nil {
		log.Println("E! failed to query global variables:", err)
		return
	}

	defer rows.Close()

	var (
		tags      = tagx.Copy(globalTags)
		textItems = map[string]string{
			"innodb_version":         "",
			"version":                "",
			"version_comment":        "",
			"wsrep_cluster_name":     "",
			"wsrep_provider_options": "",
			"tx_isolation":           "",
			"transaction_isolation":  "",
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

			slist.PushFront(types.NewSample(inputName, "global_variables_"+key, floatVal, tags))
			continue
		}
	}

	slist.PushFront(types.NewSample(inputName, "version_info", 1, tags, map[string]string{
		"version":         textItems["version"],
		"innodb_version":  textItems["innodb_version"],
		"version_comment": textItems["version_comment"],
	}))

	// mysql_galera_variables_info metric.
	// PXC/Galera variables information.
	if textItems["wsrep_cluster_name"] != "" {
		slist.PushFront(types.NewSample(inputName, "galera_variables_info", 1, tags, map[string]string{
			"wsrep_cluster_name": textItems["wsrep_cluster_name"],
		}))
	}

	// mysql_galera_gcache_size_bytes metric.
	if textItems["wsrep_provider_options"] != "" {
		slist.PushFront(types.NewSample(inputName, "galera_gcache_size_bytes", parseWsrepProviderOptions(textItems["wsrep_provider_options"]), tags))
	}

	if textItems["transaction_isolation"] != "" || textItems["tx_isolation"] != "" {
		level := textItems["transaction_isolation"]
		if level == "" {
			level = textItems["tx_isolation"]
		}

		slist.PushFront(types.NewSample(inputName, "transaction_isolation", 1, tags, map[string]string{"level": level}))
	}
}

// parseWsrepProviderOptions parse wsrep_provider_options to get gcache.size in bytes.
func parseWsrepProviderOptions(opts string) float64 {
	var val float64
	r, _ := regexp.Compile(`gcache.size = (\d+)([MG]?);`)
	data := r.FindStringSubmatch(opts)
	if data == nil {
		return 0
	}

	val, _ = strconv.ParseFloat(data[1], 64)
	switch data[2] {
	case "M":
		val = val * 1024 * 1024
	case "G":
		val = val * 1024 * 1024 * 1024
	}

	return val
}
