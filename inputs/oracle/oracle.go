package oracle

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/types"
	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"github.com/jmoiron/sqlx"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "oracle"

type OrclInstance struct {
	Address               string            `toml:"address"`
	Username              string            `toml:"username"`
	Password              string            `toml:"password"`
	IsSysDBA              bool              `toml:"is_sys_dba"`
	IsSysOper             bool              `toml:"is_sys_oper"`
	DisableConnectionPool bool              `toml:"disable_connection_pool"`
	MaxOpenConnections    int               `toml:"max_open_connections"`
	Labels                map[string]string `toml:"labels"`
	IntervalTimes         int64             `toml:"interval_times"`
}

type MetricConfig struct {
	Mesurement       string          `toml:"mesurement"`
	LabelFields      []string        `toml:"label_fields"`
	MetricFields     []string        `toml:"metric_fields"` // column_name -> value type(float64, bool, int64)
	FieldToAppend    string          `toml:"field_to_append"`
	Timeout          config.Duration `toml:"timeout"`
	Request          string          `toml:"request"`
	IgnoreZeroResult bool            `toml:"ignore_zero_result"`
}

type Oracle struct {
	Interval  config.Duration `toml:"interval"`
	Instances []OrclInstance  `toml:"instances"`
	Metrics   []MetricConfig  `toml:"metrics"`

	dbconnpool map[string]*sqlx.DB // key: instance
	Counter    uint64
	wg         sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Oracle{}
	})
}

func (o *Oracle) GetInputName() string {
	return inputName
}

func (o *Oracle) GetInterval() config.Duration {
	return o.Interval
}

func (o *Oracle) Init() error {
	if len(o.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	o.dbconnpool = make(map[string]*sqlx.DB)
	for i := 0; i < len(o.Instances); i++ {
		dbConf := o.Instances[i]
		if dbConf.Address == "" {
			return fmt.Errorf("some oracle address is blank")
		}
		connString := getConnectionString(dbConf)
		db, err := sqlx.Open("godror", connString)
		if err != nil {
			return fmt.Errorf("failed to open oracle connection: %v", err)
		}
		db.SetMaxOpenConns(dbConf.MaxOpenConnections)
		o.dbconnpool[dbConf.Address] = db
	}

	return nil
}

func (o *Oracle) Drop() {
	for address := range o.dbconnpool {
		if config.Config.DebugMode {
			log.Println("D! dropping oracle connection:", address)
		}
		if err := o.dbconnpool[address].Close(); err != nil {
			log.Println("E! failed to close oracle connection:", address, "error:", err)
		}
	}
}

func (o *Oracle) Gather() (samples []*types.Sample) {
	atomic.AddUint64(&o.Counter, 1)

	slist := list.NewSafeList()

	for i := range o.Instances {
		ins := o.Instances[i]
		o.wg.Add(1)
		go o.gatherOnce(slist, ins)
	}
	o.wg.Wait()

	interfaceList := slist.PopBackAll()
	for i := 0; i < len(interfaceList); i++ {
		samples = append(samples, interfaceList[i].(*types.Sample))
	}

	return
}

func (o *Oracle) gatherOnce(slist *list.SafeList, ins OrclInstance) {
	defer o.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&o.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	tags := map[string]string{"address": ins.Address}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	defer func(begun time.Time) {
		use := time.Since(begun).Milliseconds()
		slist.PushFront(inputs.NewSample("scrape_use_ms", use, tags))
	}(time.Now())

	db := o.dbconnpool[ins.Address]

	if err := db.Ping(); err != nil {
		slist.PushFront(inputs.NewSample("up", 0, tags))
		log.Println("E! failed to ping oracle:", ins.Address, "error:", err)
	} else {
		slist.PushFront(inputs.NewSample("up", 1, tags))
	}

	waitMetrics := new(sync.WaitGroup)

	for i := 0; i < len(o.Metrics); i++ {
		m := o.Metrics[i]
		waitMetrics.Add(1)
		go o.scrapeMetric(waitMetrics, slist, db, m, tags)
	}

	waitMetrics.Wait()
}

func (o *Oracle) scrapeMetric(waitMetrics *sync.WaitGroup, slist *list.SafeList, db *sqlx.DB, metricConf MetricConfig, tags map[string]string) {
	defer waitMetrics.Done()

	timeout := time.Duration(metricConf.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, metricConf.Request)

	if ctx.Err() == context.DeadlineExceeded {
		log.Println("E! oracle query timeout, request:", metricConf.Request)
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

	if config.Config.DebugMode {
		log.Println("D! columns:", cols)
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

		// Create our map, and retrieve the value for each column from the pointers slice,
		// storing it in the map with the name of the column as the key.
		m := make(map[string]string)
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			m[strings.ToLower(colName)] = fmt.Sprint(*val)
		}

		count := 0
		if err = o.parseRow(m, metricConf, slist, tags); err != nil {
			log.Println("E! failed to parse row:", err)
			continue
		} else {
			count++
		}

		if !metricConf.IgnoreZeroResult && count == 0 {
			log.Println("E! no metrics found while parsing")
		}
	}
}

func (o *Oracle) parseRow(row map[string]string, metricConf MetricConfig, slist *list.SafeList, tags map[string]string) error {
	labels := make(map[string]string)
	for k, v := range tags {
		labels[k] = v
	}

	for _, label := range metricConf.LabelFields {
		labelValue, has := row[label]
		if has {
			labels[label] = strings.Replace(labelValue, " ", "_", -1)
		}
	}

	for _, column := range metricConf.MetricFields {
		value, err := conv.ToFloat64(row[column])
		if err != nil {
			log.Println("E! failed to convert field:", column, "value:", value, "error:", err)
			return err
		}

		if metricConf.FieldToAppend == "" {
			slist.PushFront(inputs.NewSample(metricConf.Mesurement+"_"+column, value, labels))
		} else {
			suffix := cleanName(row[metricConf.FieldToAppend])
			slist.PushFront(inputs.NewSample(metricConf.Mesurement+"_"+suffix+"_"+column, value, labels))
		}
	}

	return nil
}

// Oracle gives us some ugly names back. This function cleans things up for Prometheus.
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

func getConnectionString(args OrclInstance) string {
	return godror.ConnectionParams{
		StandaloneConnection: args.DisableConnectionPool,
		CommonParams: dsn.CommonParams{
			Username:      args.Username,
			Password:      dsn.NewPassword(args.Password),
			ConnectString: args.Address,
		},
		PoolParams: dsn.PoolParams{
			MinSessions:      0,
			MaxSessions:      args.MaxOpenConnections,
			SessionIncrement: 1,
		},
		ConnParams: dsn.ConnParams{
			IsSysDBA:  args.IsSysDBA,
			IsSysOper: args.IsSysOper,
		},
	}.StringWithPassword()
}
