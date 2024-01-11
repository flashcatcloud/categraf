package mysql

import (
	"database/sql"
	"log"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
)

func (ins *Instance) gatherTableAutoIncrementColumns(slist *types.SampleList, db *sql.DB, globalTags map[string]string) {
	if !ins.GatherAutoIncrementColumns {
		return
	}

	rows, err := db.Query(SQL_QUERY_AUTO_INCREMENT_CLOUMN)
	if err != nil {
		log.Println("E! failed to get table size:", err)
		return
	}

	defer rows.Close()

	labels := tagx.Copy(globalTags)

	for rows.Next() {
		var schema, table, column string
		var autoIncrement, maxInt float64

		err = rows.Scan(&schema, &table, &column, &autoIncrement, &maxInt)
		if err != nil {
			log.Println("E! failed to scan rows:", err)
			return
		}

		slist.PushFront(types.NewSample(inputName, "table_auto_increment", autoIncrement, labels, map[string]string{"schema": schema, "table": table, "column": column}))
		slist.PushFront(types.NewSample(inputName, "table_max_int", maxInt, labels, map[string]string{"schema": schema, "table": table, "column": column}))
	}
}
