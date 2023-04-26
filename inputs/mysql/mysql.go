package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/go-sql-driver/mysql"
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
	config.InstanceConfig

	Address        string `toml:"address"`
	Username       string `toml:"username"`
	Password       string `toml:"password"`
	Parameters     string `toml:"parameters"`
	TimeoutSeconds int64  `toml:"timeout_seconds"`

	Queries       []QueryConfig `toml:"queries"`
	GlobalQueries []QueryConfig `toml:"-"`

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
		return types.ErrInstancesEmpty
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
	net := "tcp"
	if strings.HasSuffix(ins.Address, ".sock") {
		net = "unix"
	}
	ins.dsn = fmt.Sprintf("%s:%s@%s(%s)/?%s", ins.Username, ins.Password, net, ins.Address, ins.Parameters)
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
	config.PluginConfig
	Instances []*Instance   `toml:"instances"`
	Queries   []QueryConfig `toml:"queries"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &MySQL{}
	})
}

func (m *MySQL) Clone() inputs.Input {
	return &MySQL{}
}

func (m *MySQL) Name() string {
	return inputName
}

func (m *MySQL) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(m.Instances))
	for i := 0; i < len(m.Instances); i++ {
		m.Instances[i].GlobalQueries = m.Queries
		ret[i] = m.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	tags := map[string]string{"address": ins.Address}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushSample(inputName, "scrape_use_seconds", use, tags)
	}(begun)

	db, err := sql.Open("mysql", ins.dsn)
	if err != nil {
		slist.PushSample(inputName, "up", 0, tags)
		log.Println("E! failed to open mysql:", err)
		return
	}

	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Minute)

	if err = db.Ping(); err != nil {
		slist.PushSample(inputName, "up", 0, tags)
		log.Println("E! failed to ping mysql:", err)
		return
	}

	slist.PushSample(inputName, "up", 1, tags)

	cache := make(map[string]float64)

	ins.gatherGlobalStatus(slist, db, tags, cache)
	ins.gatherGlobalVariables(slist, db, tags, cache)
	ins.gatherEngineInnodbStatus(slist, db, tags, cache)
	ins.gatherEngineInnodbStatusCompute(slist, db, tags, cache)
	ins.gatherBinlog(slist, db, tags)
	ins.gatherProcesslistByState(slist, db, tags)
	ins.gatherProcesslistByUser(slist, db, tags)
	ins.gatherSchemaSize(slist, db, tags)
	ins.gatherTableSize(slist, db, tags, false)
	ins.gatherTableSize(slist, db, tags, true)
	ins.gatherSlaveStatus(slist, db, tags)
	ins.gatherCustomQueries(slist, db, tags)
}
