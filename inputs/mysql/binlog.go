package mysql

import (
	"database/sql"
	"log"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
)

func (ins *Instance) gatherBinlog(slist *types.SampleList, db *sql.DB, globalTags map[string]string) {
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
	slist.PushSample(inputName, "binlog_size_bytes", size, tags)
	slist.PushSample(inputName, "binlog_file_count", count, tags)

	if count == 0 || len(strings.Split(filename, ".")) < 2 {
		return
	}
	value, err := strconv.ParseFloat(strings.Split(filename, ".")[1], 64)
	if err == nil {
		slist.PushSample(inputName, "binlog_file_number", value, tags)
	}
}
