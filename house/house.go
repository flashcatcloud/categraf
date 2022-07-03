package house

// refactor from [lishijiang](https://github.com/lishijiang)'s version

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type MetricsHouseType struct {
	Opts  config.MetricsHouse
	Chan  chan *types.Sample
	Conns map[string]driver.Conn
}

var MetricsHouse *MetricsHouseType

func InitMetricsHouse() error {
	opts := config.Config.MetricsHouse
	if !opts.Enable {
		return nil
	}

	if opts.Database == "" {
		return errors.New("configuration database is blank")
	}

	if opts.Table == "" {
		return errors.New("configuration table is blank")
	}

	if len(opts.Endpoints) == 0 {
		return errors.New("configuration endpoints is empty")
	}

	if opts.BatchSize <= 0 {
		opts.BatchSize = 10000
	}

	MetricsHouse = &MetricsHouseType{
		Opts:  opts,
		Chan:  make(chan *types.Sample, opts.QueueSize),
		Conns: make(map[string]driver.Conn),
	}

	if err := MetricsHouse.connect(); err != nil {
		return err
	}

	if err := MetricsHouse.createTable(); err != nil {
		return err
	}

	go MetricsHouse.consume()

	return nil
}

func (mh *MetricsHouseType) consume() {
	batch := mh.Opts.BatchSize
	series := make([]*types.Sample, 0, batch)

	var count int

	for {
		select {
		case item, open := <-mh.Chan:
			if !open {
				// queue closed
				return
			}

			if item == nil {
				continue
			}

			series = append(series, item)
			count++
			if count >= batch {
				mh.postSeries(series)
				count = 0
				series = make([]*types.Sample, 0, batch)
			}
		default:
			if len(series) > 0 {
				mh.postSeries(series)
				count = 0
				series = make([]*types.Sample, 0, batch)
			}
			time.Sleep(time.Duration(mh.Opts.IdleDuration))
		}
	}
}

func (mh *MetricsHouseType) postSeries(series []*types.Sample) {
	count := len(mh.Opts.Endpoints)
	for _, i := range rand.Perm(count) {
		ep := mh.Opts.Endpoints[i]
		conn := mh.Conns[ep]
		if err := mh.post(conn, series); err == nil {
			return
		} else {
			log.Println("E! failed to post series to clickhouse:", ep, "error:", err)
		}
	}
}

func (mh *MetricsHouseType) post(conn driver.Conn, series []*types.Sample) error {
	batch, err := conn.PrepareBatch(context.Background(), "INSERT INTO "+mh.Opts.Table)
	if err != nil {
		return err
	}

	for _, e := range series {
		err := batch.Append(
			e.Timestamp, //会自动转换时间格式
			e.Timestamp, //会自动转换时间格式
			e.Metric,
			config.Config.GetHostname(),
			convertTags(e),
			e.Value,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

func (mh *MetricsHouseType) Push(s *types.Sample) {
	if config.Config.MetricsHouse.Enable {
		mh.Chan <- s
	}
}

const SQLCreateTable = `
CREATE TABLE IF NOT EXISTS %s (
	  event_time DateTime
	, event_date Date
	, metric LowCardinality(String)
	, hostname LowCardinality(String)
	, tags String
	, value Float64
) ENGINE = MergeTree
PARTITION BY toYYYYMM(event_date)
ORDER BY (event_time, metric, tags)
TTL event_date + toIntervalDay(365)
SETTINGS index_granularity = 8192
`

func (mh *MetricsHouseType) createTable() error {
	for _, conn := range mh.Conns {
		if err := conn.Exec(context.Background(), fmt.Sprintf(SQLCreateTable, mh.Opts.Table)); err != nil {
			return fmt.Errorf("failed to create table: %v", err)
		}
	}
	return nil
}

func (mh *MetricsHouseType) connect() error {
	for _, endpoint := range mh.Opts.Endpoints {
		conn, err := clickhouse.Open(&clickhouse.Options{
			Addr: []string{endpoint},
			Auth: clickhouse.Auth{
				Database: mh.Opts.Database,
				Username: mh.Opts.Username,
				Password: mh.Opts.Password,
			},
			Debug:           mh.Opts.Debug,
			DialTimeout:     time.Duration(mh.Opts.DialTimeout),
			MaxOpenConns:    mh.Opts.MaxOpenConns,
			MaxIdleConns:    mh.Opts.MaxIdleConns,
			ConnMaxLifetime: time.Duration(mh.Opts.ConnMaxLifetime),
		})

		if err != nil {
			return fmt.Errorf("failed to open clickhouse(%s) connection: %v", endpoint, err)
		}

		if _, has := mh.Conns[endpoint]; has {
			return fmt.Errorf("duplicate endpoint: %s", endpoint)
		}

		mh.Conns[endpoint] = conn
	}

	return nil
}
