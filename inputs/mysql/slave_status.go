package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"flashcat.cloud/categraf/types"
)

var slaveStatusQueries = [2]string{"SHOW ALL SLAVES STATUS", "SHOW SLAVE STATUS"}
var slaveStatusQuerySuffixes = [3]string{" NONBLOCKING", " NOLOCK", ""}

func querySlaveStatus(db *sql.DB) (rows *sql.Rows, err error) {
	for _, query := range slaveStatusQueries {
		rows, err = db.Query(query)
		if err == nil {
			return rows, nil
		}

		// Leverage lock-free SHOW SLAVE STATUS by guessing the right suffix
		for _, suffix := range slaveStatusQuerySuffixes {
			rows, err = db.Query(fmt.Sprint(query, suffix))
			if err == nil {
				return rows, nil
			}
		}
	}
	return
}

func (ins *Instance) gatherSlaveStatus(slist *types.SampleList, db *sql.DB, globalTags map[string]string) {
	if !ins.GatherSlaveStatus {
		return
	}

	rows, err := querySlaveStatus(db)
	if err != nil {
		log.Println("E! failed to query slave status:", err)
		return
	}

	if rows == nil {
		log.Println("E! failed to query slave status: rows is nil")
		return
	}

	defer rows.Close()

	slaveCols, err := rows.Columns()
	if err != nil {
		log.Println("E! failed to get columns of slave rows:", err)
		return
	}

	for rows.Next() {
		// As the number of columns varies with mysqld versions,
		// and sql.Scan requires []interface{}, we need to create a
		// slice of pointers to the elements of slaveData.
		scanArgs := make([]interface{}, len(slaveCols))
		for i := range scanArgs {
			scanArgs[i] = &sql.RawBytes{}
		}

		if err := rows.Scan(scanArgs...); err != nil {
			continue
		}

		masterUUID := columnValue(scanArgs, slaveCols, "Master_UUID")
		masterHost := columnValue(scanArgs, slaveCols, "Master_Host")
		channelName := columnValue(scanArgs, slaveCols, "Channel_Name")       // MySQL & Percona
		connectionName := columnValue(scanArgs, slaveCols, "Connection_name") // MariaDB

		if connectionName != "" {
			channelName = connectionName
		}

		if channelName == "" {
			channelName = "default"
		}

		for i, col := range slaveCols {
			key := strings.ToLower(col)
			if _, has := ins.validMetrics[key]; !has {
				continue
			}

			if value, ok := parseStatus(*scanArgs[i].(*sql.RawBytes)); ok {
				slist.PushFront(types.NewSample(inputName, "slave_status_"+key, value, globalTags, map[string]string{
					"master_host":  masterHost,
					"master_uuid":  masterUUID,
					"channel_name": channelName,
				}))
			}
		}
	}
}

func columnIndex(slaveCols []string, colName string) int {
	for idx := range slaveCols {
		if slaveCols[idx] == colName {
			return idx
		}
	}
	return -1
}

func columnValue(scanArgs []interface{}, slaveCols []string, colName string) string {
	var columnIndex = columnIndex(slaveCols, colName)
	if columnIndex == -1 {
		return ""
	}
	return string(*scanArgs[columnIndex].(*sql.RawBytes))
}
