package mysql

import (
	"database/sql"
	"log"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tagx"
	"github.com/toolkits/pkg/container/list"
)

func (m *MySQL) gatherSchemaSize(slist *list.SafeList, ins *Instance, db *sql.DB, globalTags map[string]string, cache map[string]float64) {
	if !ins.GatherSchemaSize {
		return
	}

	rows, err := db.Query(SQL_QUERY_SCHEMA_SIZE)
	if err != nil {
		log.Println("E! failed to get schema size:", err)
		return
	}

	defer rows.Close()

	labels := tagx.Copy(globalTags)

	for rows.Next() {
		var schema string
		var size int64

		err = rows.Scan(&schema, &size)
		if err != nil {
			log.Println("E! failed to scan rows:", err)
			return
		}

		slist.PushFront(inputs.NewSample("schema_size_bytes", size, labels, map[string]string{"schema": schema}))
	}
}
