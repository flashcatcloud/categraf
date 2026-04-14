package mysql

import (
	"database/sql"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

func (ins *Instance) gatherSchemaSize(slist *types.SampleList, db *sql.DB, globalTags map[string]string) {
	if !ins.GatherSchemaSize {
		return
	}

	rows, err := db.Query(SQL_QUERY_SCHEMA_SIZE)
	if err != nil {
		klog.ErrorS(err, "failed to query mysql schema sizes", "address", ins.Address)
		return
	}

	defer rows.Close()

	labels := tagx.Copy(globalTags)

	for rows.Next() {
		var schema string
		var size int64

		err = rows.Scan(&schema, &size)
		if err != nil {
			klog.ErrorS(err, "failed to scan mysql schema size rows", "address", ins.Address)
			return
		}

		slist.PushFront(types.NewSample(inputName, "schema_size_bytes", size, labels, map[string]string{"schema": schema}))
	}
}
