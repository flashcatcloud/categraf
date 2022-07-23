package mysql

import (
	"database/sql"
	"log"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

func (ins *Instance) gatherSchemaSize(slist *list.SafeList, db *sql.DB, globalTags map[string]string) {
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

		slist.PushFront(types.NewSample("schema_size_bytes", size, labels, map[string]string{"schema": schema}))
	}
}
