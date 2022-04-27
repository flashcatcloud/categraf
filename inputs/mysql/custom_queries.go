package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/pkg/tagx"
	"github.com/toolkits/pkg/container/list"
)

func (m *MySQL) gatherCustomQueries(slist *list.SafeList, ins *Instance, db *sql.DB, globalTags map[string]string) {
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	for i := 0; i < len(ins.Queries); i++ {
		wg.Add(1)
		go m.gatherOneQuery(slist, ins, db, globalTags, wg, ins.Queries[i])
	}
}

func (m *MySQL) gatherOneQuery(slist *list.SafeList, ins *Instance, db *sql.DB, globalTags map[string]string, wg *sync.WaitGroup, query QueryConfig) {
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
		columns := make([]interface{}, len(cols))
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
			val := columnPointers[i].(*interface{})
			row[strings.ToLower(colName)] = fmt.Sprint(*val)
		}

		if err = m.parseRow(row, query, slist, globalTags); err != nil {
			log.Println("E! failed to parse row:", err, "sql:", query.Request)
		}
	}
}

func (m *MySQL) parseRow(row map[string]string, query QueryConfig, slist *list.SafeList, globalTags map[string]string) error {
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
			slist.PushFront(inputs.NewSample(query.Mesurement+"_"+column, value, labels))
		} else {
			suffix := cleanName(row[query.FieldToAppend])
			slist.PushFront(inputs.NewSample(query.Mesurement+"_"+suffix+"_"+column, value, labels))
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
