package mysql

import (
	"database/sql"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

func (ins *Instance) gatherBinlog(slist *types.SampleList, db *sql.DB, globalTags map[string]string) {
	if ins.DisablebinLogs {
		return
	}
	var logBin uint8
	err := db.QueryRow(`SELECT @@log_bin`).Scan(&logBin)
	if err != nil {
		klog.ErrorS(err, "failed to query mysql @@log_bin", "address", ins.Address)
		return
	}

	// If log_bin is OFF, do not run SHOW BINARY LOGS which explicitly produces MySQL error
	if logBin == 0 {
		return
	}

	rows, err := db.Query(`SHOW BINARY LOGS`)
	if err != nil {
		klog.ErrorS(err, "failed to query mysql binary logs", "address", ins.Address)
		return
	}

	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		klog.ErrorS(err, "failed to get mysql binary log columns", "address", ins.Address)
		return
	}

	var (
		size        uint64 = 0
		count       uint64 = 0
		filename    string
		filesize    uint64
		encrypted   string
		algorithm   string // 加密算法（在某些版本中可用）；Reserved for future use: currently unused
		columnCount int    = len(columns)
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
		case 4: // 包含加密算法的版本
			if err := rows.Scan(&filename, &filesize, &encrypted, &algorithm); err != nil {
				return
			}
		default:
			klog.ErrorS(nil, "invalid number of mysql binary log columns", "address", ins.Address, "column_count", columnCount)
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
