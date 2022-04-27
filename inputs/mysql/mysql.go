package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/go-sql-driver/mysql"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "mysql"

type QueryConfig struct {
	Mesurement    string          `toml:"mesurement"`
	LabelFields   []string        `toml:"label_fields"`
	MetricFields  []string        `toml:"metric_fields"`
	FieldToAppend string          `toml:"field_to_append"`
	Timeout       config.Duration `toml:"timeout"`
	Request       string          `toml:"request"`
}

type Instance struct {
	Address        string `toml:"address"`
	Username       string `toml:"username"`
	Password       string `toml:"password"`
	Parameters     string `toml:"parameters"`
	TimeoutSeconds int64  `toml:"timeout_seconds"`

	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`
	Queries       []QueryConfig     `toml:"queries"`

	ExtraStatusMetrics              bool `toml:"extra_status_metrics"`
	ExtraInnodbMetrics              bool `toml:"extra_innodb_metrics"`
	GatherProcessListProcessByState bool `toml:"gather_processlist_processes_by_state"`
	GatherProcessListProcessByUser  bool `toml:"gather_processlist_processes_by_user"`
	GatherSchemaSize                bool `toml:"gather_schema_size"`
	GatherTableSize                 bool `toml:"gather_table_size"`
	GatherSystemTableSize           bool `toml:"gather_system_table_size"`
	GatherSlaveStatus               bool `toml:"gather_slave_status"`

	validMetrics map[string]struct{}
	dsn          string
	tls.ClientConfig
}

func (ins *Instance) Init() error {
	if ins.Address == "" {
		return errors.New("address is blank")
	}

	if ins.UseTLS {
		tlsConfig, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return fmt.Errorf("failed to register tls config: %v", err)
		}

		err = mysql.RegisterTLSConfig("custom", tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to register tls config: %v", err)
		}
	}

	ins.dsn = fmt.Sprintf("%s:%s@tcp(%s)/?%s", ins.Username, ins.Password, ins.Address, ins.Parameters)

	conf, err := mysql.ParseDSN(ins.dsn)
	if err != nil {
		return err
	}

	if conf.Timeout == 0 {
		if ins.TimeoutSeconds == 0 {
			ins.TimeoutSeconds = 3
		}
		conf.Timeout = time.Second * time.Duration(ins.TimeoutSeconds)
	}

	ins.dsn = conf.FormatDSN()

	ins.InitValidMetrics()

	return nil
}

func (ins *Instance) InitValidMetrics() {
	ins.validMetrics = make(map[string]struct{})

	for key := range STATUS_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range VARIABLES_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range INNODB_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range BINLOG_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range GALERA_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range PERFORMANCE_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range SCHEMA_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range TABLE_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range REPLICA_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range GROUP_REPLICATION_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	for key := range SYNTHETIC_VARS {
		ins.validMetrics[key] = struct{}{}
	}

	if ins.ExtraStatusMetrics {
		for key := range OPTIONAL_STATUS_VARS {
			ins.validMetrics[key] = struct{}{}
		}
	}

	if ins.ExtraInnodbMetrics {
		for key := range OPTIONAL_INNODB_VARS {
			ins.validMetrics[key] = struct{}{}
		}
	}
}

type MySQL struct {
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &MySQL{}
	})
}

func (m *MySQL) GetInputName() string {
	return inputName
}

func (m *MySQL) GetInterval() config.Duration {
	return m.Interval
}

func (m *MySQL) Init() error {
	if len(m.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(m.Instances); i++ {
		if err := m.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (m *MySQL) Drop() {}

func (m *MySQL) Gather(slist *list.SafeList) {
	atomic.AddUint64(&m.Counter, 1)
	for i := range m.Instances {
		ins := m.Instances[i]
		m.wg.Add(1)
		go m.gatherOnce(slist, ins)
	}
	m.wg.Wait()
}

func (m *MySQL) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer m.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&m.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	tags := map[string]string{"address": ins.Address}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("scrape_use_seconds", use, tags))
	}(begun)

	db, err := sql.Open("mysql", ins.dsn)
	if err != nil {
		slist.PushFront(inputs.NewSample("up", 0, tags))
		log.Println("E! failed to open mysql:", err)
		return
	}

	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Minute)

	if err = db.Ping(); err != nil {
		slist.PushFront(inputs.NewSample("up", 0, tags))
		log.Println("E! failed to ping mysql:", err)
		return
	}

	slist.PushFront(inputs.NewSample("up", 1, tags))

	cache := make(map[string]float64)

	m.gatherGlobalStatus(slist, ins, db, tags, cache)
	m.gatherGlobalVariables(slist, ins, db, tags, cache)
	m.gatherEngineInnodbStatus(slist, ins, db, tags, cache)
	m.gatherEngineInnodbStatusCompute(slist, ins, db, tags, cache)
	m.gatherBinlog(slist, ins, db, tags)
	m.gatherProcesslistByState(slist, ins, db, tags)
	m.gatherProcesslistByUser(slist, ins, db, tags)
	m.gatherSchemaSize(slist, ins, db, tags)
	m.gatherTableSize(slist, ins, db, tags, false)
	m.gatherTableSize(slist, ins, db, tags, true)
	m.gatherSlaveStatus(slist, ins, db, tags)
	m.gatherCustomQueries(slist, ins, db, tags)
}
