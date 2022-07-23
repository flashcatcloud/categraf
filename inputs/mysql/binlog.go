package mysql

import (
	"database/sql"
	"log"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

func (ins *Instance) gatherBinlog(slist *list.SafeList, db *sql.DB, globalTags map[string]string) {
	var logBin uint8
	err := db.QueryRow(`SELECT @@log_bin`).Scan(&logBin)
	if err != nil {
		log.Println("E! failed to query SELECT @@log_bin:", err)
		return
	}

	// If log_bin is OFF, do not run SHOW BINARY LOGS which explicitly produces MySQL error
	if logBin == 0 {
		return
	}

	rows, err := db.Query(`SHOW BINARY LOGS`)
	if err != nil {
		log.Println("E! failed to query SHOW BINARY LOGS:", err)
		return
	}

	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Println("E! failed to get columns:", err)
		return
	}

	var (
		size        uint64 = 0
		count       uint64 = 0
		filename    string
		filesize    uint64
		encrypted   string
		columnCount int = len(columns)
	)

	for rows.Next() {
		switch columnCount {
		case 2:
			if err := rows.Scan(&filename, &filesize); err != nil {
				return
			}
		case 3:
			if err := rows.Scan(&filename, &filesize, &encrypted); err != nil {
				return
			}
		default:
			log.Println("E! invalid number of columns:", columnCount)
		}

		size += filesize
		count++
	}

	tags := tagx.Copy(globalTags)
	slist.PushFront(types.NewSample("binlog_size_bytes", size, tags))
	slist.PushFront(types.NewSample("binlog_file_count", count, tags))

	value, err := strconv.ParseFloat(strings.Split(filename, ".")[1], 64)
	if err == nil {
		slist.PushFront(types.NewSample("binlog_file_number", value, tags))
	}
}
