package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tagx"
	"github.com/toolkits/pkg/container/list"
)

var slaveStatusQueries = [2]string{"SHOW ALL SLAVES STATUS", "SHOW SLAVE STATUS"}
var slaveStatusQuerySuffixes = [3]string{" NONBLOCKING", " NOLOCK", ""}

func (m *MySQL) gatherSlaveStatus(slist *list.SafeList, ins *Instance, db *sql.DB, globalTags map[string]string) {
	if !ins.GatherSlaveStatus {
		return
	}

	var (
		rows *sql.Rows
		err  error
	)
	// Try the both syntax for MySQL/Percona and MariaDB
	for _, query := range slaveStatusQueries {
		rows, err = db.Query(query)
		if err != nil { // MySQL/Percona
			// Leverage lock-free SHOW SLAVE STATUS by guessing the right suffix
			for _, suffix := range slaveStatusQuerySuffixes {
				rows, err = db.Query(fmt.Sprint(query, suffix))
				if err == nil {
					break
				}
			}
		} else { // MariaDB
			break
		}
	}

	if err != nil {
		log.Println("E! failed to query slave status:", err)
		return
	}

	defer rows.Close()

	var (
		tags      = tagx.Copy(globalTags)
		fields    = make(map[string]interface{})
		textItems = map[string]string{
			"master_host":     "",
			"master_uuid":     "",
			"channel_name":    "",
			"connection_name": "",
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

		// collect float fields
		if _, has := ins.validMetrics[key]; !has {
			continue
		}

		if floatVal, ok := parseStatus(val); ok {
			fields[key] = floatVal
			continue
		}
	}

	if textItems["connection_name"] != "" {
		textItems["channel_name"] = textItems["connection_name"]
	}

	// default channel name is empty
	if textItems["channel_name"] == "" {
		textItems["channel_name"] = "default"
	}

	for k, v := range fields {
		slist.PushFront(inputs.NewSample("slave_status_"+k, v, tags, map[string]string{
			"master_host":  textItems["master_host"],
			"master_uuid":  textItems["master_uuid"],
			"channel_name": textItems["channel_name"],
		}))
	}
}
