package mysql

import (
	"database/sql"
	"log"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tagx"
	"github.com/toolkits/pkg/container/list"
)

func (m *MySQL) gatherTableSize(slist *list.SafeList, ins *Instance, db *sql.DB, globalTags map[string]string, isSystem bool) {
	query := SQL_QUERY_TABLE_SIZE
	if isSystem {
		query = SQL_QUERY_SYSTEM_TABLE_SIZE
		if !ins.GatherSystemTableSize {
			return
		}
	} else {
		if !ins.GatherTableSize {
			return
		}
	}

	rows, err := db.Query(query)
	if err != nil {
		log.Println("E! failed to get table size:", err)
		return
	}

	defer rows.Close()

	labels := tagx.Copy(globalTags)

	for rows.Next() {
		var schema, table string
		var indexSize, dataSize int64

		err = rows.Scan(&schema, &table, &indexSize, &dataSize)
		if err != nil {
			log.Println("E! failed to scan rows:", err)
			return
		}

		slist.PushFront(inputs.NewSample("table_size_index_bytes", indexSize, labels, map[string]string{"schema": schema, "table": table}))
		slist.PushFront(inputs.NewSample("table_size_data_bytes", dataSize, labels, map[string]string{"schema": schema, "table": table}))
	}
}
