package mysql

import (
	"database/sql"
	"log"
	"regexp"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/inputs"
	"github.com/toolkits/pkg/container/list"
)

func (m *MySQL) gatherEngineInnodbStatus(slist *list.SafeList, ins *Instance, db *sql.DB, globalTags map[string]string, cache map[string]float64) {
	rows, err := db.Query(SQL_ENGINE_INNODB_STATUS)
	if err != nil {
		log.Println("E! failed to query engine innodb status:", err)
		return
	}

	defer rows.Close()

	var typeCol, nameCol, statusCol string
	// First row should contain the necessary info. If many rows returned then it's unknown case.
	if rows.Next() {
		if err := rows.Scan(&typeCol, &nameCol, &statusCol); err != nil {
			log.Println("E! failed to scan result, sql:", SQL_ENGINE_INNODB_STATUS, "error:", err)
			return
		}
	}

	// 0 queries inside InnoDB, 0 queries in queue
	// 0 read views open inside InnoDB
	rQueries, _ := regexp.Compile(`(\d+) queries inside InnoDB, (\d+) queries in queue`)
	rViews, _ := regexp.Compile(`(\d+) read views open inside InnoDB`)

	for _, line := range strings.Split(statusCol, "\n") {
		if data := rQueries.FindStringSubmatch(line); data != nil {
			value, err := strconv.ParseFloat(data[1], 64)
			if err != nil {
				continue
			}
			slist.PushFront(inputs.NewSample("engine_innodb_queries_inside_innodb", value))

			value, err = strconv.ParseFloat(data[2], 64)
			if err != nil {
				continue
			}
			slist.PushFront(inputs.NewSample("engine_innodb_queries_in_queue", value))
		} else if data := rViews.FindStringSubmatch(line); data != nil {
			value, err := strconv.ParseFloat(data[1], 64)
			if err != nil {
				continue
			}
			slist.PushFront(inputs.NewSample("engine_innodb_read_views_open_inside_innodb", value))
		}
	}
}
