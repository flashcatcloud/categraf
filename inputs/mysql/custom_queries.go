package mysql

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
)

func (ins *Instance) gatherCustomQueries(slist *types.SampleList, db *sql.DB, globalTags map[string]string) {
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	for i := 0; i < len(ins.Queries); i++ {
		wg.Add(1)
		go ins.gatherOneQuery(slist, db, globalTags, wg, ins.Queries[i])
	}

	for i := 0; i < len(ins.GlobalQueries); i++ {
		wg.Add(1)
		go ins.gatherOneQuery(slist, db, globalTags, wg, ins.GlobalQueries[i])
	}
}

func (ins *Instance) gatherOneQuery(slist *types.SampleList, db *sql.DB, globalTags map[string]string, wg *sync.WaitGroup, query QueryConfig) {
	defer wg.Done()

	timeout := time.Duration(query.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, query.Request)
	if ctx.Err() == context.DeadlineExceeded {
		log.Println("E! query timeout, request:", query.Request)
		return
	}

	if err != nil {
		log.Println("E! failed to query:", err)
		return
	}

	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		log.Println("E! failed to get columns:", err)
		return
	}

	for rows.Next() {
		columns := make([]sql.RawBytes, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		// Scan the result into the column pointers...
		if err := rows.Scan(columnPointers...); err != nil {
			log.Println("E! failed to scan:", err)
			return
		}

		row := make(map[string]string)
		for i, colName := range cols {
			val := columnPointers[i].(*sql.RawBytes)
			row[strings.ToLower(colName)] = string(*val)
		}

		if err = ins.parseRow(row, query, slist, globalTags); err != nil {
			log.Println("E! failed to parse row:", err, "sql:", query.Request)
		}
	}
}

func (ins *Instance) parseRow(row map[string]string, query QueryConfig, slist *types.SampleList, globalTags map[string]string) error {
	labels := tagx.Copy(globalTags)

	for _, label := range query.LabelFields {
		labelValue, has := row[label]
		if has {
			labels[label] = strings.Replace(labelValue, " ", "_", -1)
		}
	}

	for _, column := range query.MetricFields {
		value, err := conv.ToFloat64(row[column])
		if err != nil {
			log.Println("E! failed to convert field:", column, "value:", value, "error:", err)
			return err
		}

		if query.FieldToAppend == "" {
			slist.PushFront(types.NewSample(inputName, query.Mesurement+"_"+column, value, labels))
		} else {
			suffix := cleanName(row[query.FieldToAppend])
			slist.PushFront(types.NewSample(inputName, query.Mesurement+"_"+suffix+"_"+column, value, labels))
		}
	}

	return nil
}

func cleanName(s string) string {
	s = strings.Replace(s, " ", "_", -1) // Remove spaces
	s = strings.Replace(s, "(", "", -1)  // Remove open parenthesis
	s = strings.Replace(s, ")", "", -1)  // Remove close parenthesis
	s = strings.Replace(s, "/", "", -1)  // Remove forward slashes
	s = strings.Replace(s, "*", "", -1)  // Remove asterisks
	s = strings.Replace(s, "%", "percent", -1)
	s = strings.ToLower(s)
	return s
}
