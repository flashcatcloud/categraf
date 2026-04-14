package mysql

import (
	"database/sql"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

func (ins *Instance) gatherProcesslistByUser(slist *types.SampleList, db *sql.DB, globalTags map[string]string) {
	if !ins.GatherProcessListProcessByUser {
		return
	}

	rows, err := db.Query(SQL_INFO_SCHEMA_PROCESSLIST_BY_USER)
	if err != nil {
		klog.ErrorS(err, "failed to query mysql processlist by user", "address", ins.Address)
		return
	}

	defer rows.Close()

	labels := tagx.Copy(globalTags)

	for rows.Next() {
		var user string
		var connections int64

		err = rows.Scan(&user, &connections)
		if err != nil {
			klog.ErrorS(err, "failed to scan mysql processlist by user rows", "address", ins.Address)
			return
		}

		slist.PushFront(types.NewSample(inputName, "processlist_processes_by_user", connections, labels, map[string]string{"user": user}))
	}
}
