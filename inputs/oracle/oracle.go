package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	go_ora "github.com/sijms/go-ora/v2"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/types"
)

const inputName = "oracle"

type Instance struct {
	config.InstanceConfig

	Address               string `toml:"address"`
	Username              string `toml:"username"`
	Password              string `toml:"password"`
	IsSysDBA              bool   `toml:"is_sys_dba"`
	IsSysOper             bool   `toml:"is_sys_oper"`
	DisableConnectionPool bool   `toml:"disable_connection_pool"`
	MaxOpenConnections    int    `toml:"max_open_connections"`

	ConnMaxIdleTime config.Duration `toml:"conn_max_idle_time"` // 新增
	ConnMaxLifetime config.Duration `toml:"conn_max_lifetime"`  // 新增

	Metrics       []MetricConfig `toml:"metrics"`
	GlobalMetrics []MetricConfig `toml:"-"`
	client        *sql.DB
}

type MetricConfig struct {
	Mesurement       string          `toml:"mesurement"`
	LabelFields      []string        `toml:"label_fields"`
	MetricFields     []string        `toml:"metric_fields"`
	FieldToAppend    string          `toml:"field_to_append"`
	Timeout          config.Duration `toml:"timeout"`
	Request          string          `toml:"request"`
	IgnoreZeroResult bool            `toml:"ignore_zero_result"`
}

type Oracle struct {
	config.PluginConfig
	Instances []*Instance    `toml:"instances"`
	Metrics   []MetricConfig `toml:"metrics"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Oracle{}
	})
}

func (o *Oracle) Clone() inputs.Input {
	return &Oracle{}
}

func (o *Oracle) Name() string {
	return inputName
}

func (o *Oracle) Drop() {
	for i := 0; i < len(o.Instances); i++ {
		o.Instances[i].Drop()
	}
}

func (o *Oracle) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(o.Instances))
	for i := 0; i < len(o.Instances); i++ {
		o.Instances[i].GlobalMetrics = o.Metrics
		ret[i] = o.Instances[i]
	}
	return ret
}

func (ins *Instance) Init() error {
	if ins.Address == "" {
		return types.ErrInstancesEmpty
	}

	connString, err := ins.getConnectionString()
	if err != nil {
		return fmt.Errorf("failed to build oracle connect string %s: %v", ins.Address, err)
	}

	conn, err := sql.Open("oracle", connString)
	if err != nil {
		return fmt.Errorf("failed to open oracle database %s: %v", ins.Address, err)
	}
	if conn == nil {
		return fmt.Errorf("failed to open oracle database: %s", ins.Address)
	}
	ins.client = conn

	if ins.MaxOpenConnections == 0 {
		ins.MaxOpenConnections = 2
	}

	// 设置默认值
	if ins.ConnMaxIdleTime == 0 {
		ins.ConnMaxIdleTime = config.Duration(time.Minute * 15) // 默认15分钟
	}
	if ins.ConnMaxLifetime == 0 {
		ins.ConnMaxLifetime = config.Duration(time.Hour * 24) // 默认24小时
	}

	ins.client.SetMaxOpenConns(ins.MaxOpenConnections)
	ins.client.SetMaxIdleConns(ins.MaxOpenConnections)
	// ins.client.SetConnMaxIdleTime(time.Duration(0))
	// ins.client.SetConnMaxLifetime(time.Duration(0))
	ins.client.SetConnMaxIdleTime(time.Duration(ins.ConnMaxIdleTime))
	ins.client.SetConnMaxLifetime(time.Duration(ins.ConnMaxLifetime))

	return nil
}

func (ins *Instance) Drop() error {
	if ins.DebugMod {
		log.Println("D! dropping oracle connection:", ins.Address)
	}

	if len(ins.Address) == 0 || ins.client == nil {
		if ins.DebugMod {
			log.Println("D! oracle address is empty or client is nil, so there is no need to close")
		}
		return nil
	}

	if err := ins.client.Close(); err != nil {
		log.Println("E! failed to close oracle connection:", ins.Address, "error:", err)
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.Address) == 0 {
		if ins.DebugMod {
			log.Println("D! oracle address is empty")
		}
		return
	}
	tags := map[string]string{"address": ins.Address}

	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample(inputName, "scrape_use_seconds", use, tags))
	}(time.Now())

	if err := ins.client.Ping(); err != nil {

		log.Println("I! attempting to rebuild oracle connection:", ins.Address)
		ins.Drop()
		ins.Init()
		if err := ins.client.Ping(); err != nil {
			slist.PushFront(types.NewSample(inputName, "up", 0, tags))
			log.Println("E! failed to ping oracle:", ins.Address, "error:", err)
			return
		}
	} else {
		slist.PushFront(types.NewSample(inputName, "up", 1, tags))
	}

	waitMetrics := new(sync.WaitGroup)

	for i := 0; i < len(ins.Metrics); i++ {
		m := ins.Metrics[i]
		waitMetrics.Add(1)
		go ins.scrapeMetric(waitMetrics, slist, m, tags)
	}

	for i := 0; i < len(ins.GlobalMetrics); i++ {
		m := ins.GlobalMetrics[i]
		waitMetrics.Add(1)
		go ins.scrapeMetric(waitMetrics, slist, m, tags)
	}

	waitMetrics.Wait()
}

func (ins *Instance) scrapeMetric(waitMetrics *sync.WaitGroup, slist *types.SampleList, metricConf MetricConfig, tags map[string]string) {
	defer waitMetrics.Done()

	timeout := time.Duration(metricConf.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := ins.client.QueryContext(ctx, metricConf.Request)

	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("E! %s oracle query timeout (more than %d seconds), request: %s", ins.Address, metricConf.Timeout/(1000*1000*1000),
			strings.ReplaceAll(strings.ReplaceAll(metricConf.Request, "\n", " "), "\r", " "))
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

	if ins.DebugMod {
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
		if err = ins.parseRow(m, metricConf, slist, tags); err != nil {
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

func (ins *Instance) parseRow(row map[string]string, metricConf MetricConfig, slist *types.SampleList, tags map[string]string) error {
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
			slist.PushFront(types.NewSample(inputName, metricConf.Mesurement+"_"+column, value, labels))
		} else {
			suffix := cleanName(row[metricConf.FieldToAppend])
			slist.PushFront(types.NewSample(inputName, metricConf.Mesurement+"_"+suffix+"_"+column, value, labels))
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

func isTNSFormat(connStr string) bool {

	tnsPattern := `^\s*\(\s*DESCRIPTION\s*=.*`
	regex := regexp.MustCompile(tnsPattern)
	return regex.MatchString(connStr)
}

func (ins *Instance) getConnectionString() (string, error) {
	opts := make(map[string]string)
	if ins.IsSysOper {
		opts["dba privilege"] = "sysoper"
	}
	if ins.IsSysDBA {
		opts["dba privilege"] = "sysdba"
	}
	if ins.DisableConnectionPool {
		opts["mode"] = "standalone"
	}

	if isTNSFormat(ins.Address) {
		return go_ora.BuildJDBC(ins.Username, ins.Password, ins.Address, opts), nil
	}

	ip, port, service, err := explode(ins.Address)
	if err != nil {
		log.Println("E! oracle address format error:", err)
		return "", err
	}
	return go_ora.BuildUrl(ip, port, service, ins.Username, ins.Password, opts), nil
}

func explode(target string) (ip string, port int, service string, err error) {
	ErrInvalidTarget := fmt.Errorf("invalid target: %s", target)

	parts := strings.Split(target, "/")
	if len(parts) != 2 {
		return "", 0, "", ErrInvalidTarget
	}

	ipPort := strings.Split(parts[0], ":")
	if len(ipPort) != 2 {
		return "", 0, "", ErrInvalidTarget
	}

	port, err = strconv.Atoi(ipPort[1])
	if err != nil {
		return "", 0, "", ErrInvalidTarget
	}

	ip = strings.TrimSpace(ipPort[0])
	if ip == "" {
		return "", 0, "", ErrInvalidTarget
	}

	service = strings.TrimSpace(parts[1])
	if service == "" {
		return "", 0, "", ErrInvalidTarget
	}

	return
}
