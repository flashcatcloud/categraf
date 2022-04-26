package mysql

import (
	"database/sql"
	"log"
	"regexp"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tagx"
	"github.com/toolkits/pkg/container/list"
)

func (m *MySQL) gatherGlobalVariables(slist *list.SafeList, ins *Instance, db *sql.DB, globalTags map[string]string) {
	rows, err := db.Query(SQL_GLOBAL_VARIABLES)
	if err != nil {
		log.Println("E! failed to query global variables:", err)
		return
	}

	defer rows.Close()

	var (
		key       string
		val       sql.RawBytes
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

		// collect float fields
		if _, has := ins.validMetrics[key]; !has {
			continue
		}

		if floatVal, ok := parseStatus(val); ok {
			slist.PushFront(inputs.NewSample("global_variables_"+key, floatVal, tags))
			continue
		}
	}

	slist.PushFront(inputs.NewSample("version_info", 1, tags, map[string]string{
		"version":         textItems["version"],
		"innodb_version":  textItems["innodb_version"],
		"version_comment": textItems["version_comment"],
	}))

	// mysql_galera_variables_info metric.
	// PXC/Galera variables information.
	if textItems["wsrep_cluster_name"] != "" {
		slist.PushFront(inputs.NewSample("galera_variables_info", 1, tags, map[string]string{
			"wsrep_cluster_name": textItems["wsrep_cluster_name"],
		}))
	}

	// mysql_galera_gcache_size_bytes metric.
	if textItems["wsrep_provider_options"] != "" {
		slist.PushFront(inputs.NewSample("galera_gcache_size_bytes", parseWsrepProviderOptions(textItems["wsrep_provider_options"]), tags))
	}

	if textItems["transaction_isolation"] != "" || textItems["tx_isolation"] != "" {
		level := textItems["transaction_isolation"]
		if level == "" {
			level = textItems["tx_isolation"]
		}

		slist.PushFront(inputs.NewSample("transaction_isolation", 1, tags, map[string]string{"level": level}))
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
