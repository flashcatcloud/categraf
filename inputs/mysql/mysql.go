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

type Instance struct {
	Address        string `toml:"address"`
	Username       string `toml:"username"`
	Password       string `toml:"password"`
	Parameters     string `toml:"parameters"`
	TimeoutSeconds int64  `toml:"timeout_seconds"`

	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	dsn string
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

	return nil
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
	}

	slist.PushFront(inputs.NewSample("up", 1, tags))

	m.gatherGlobalStatus(slist, ins, db, tags)
	m.gatherGlobalVariables(slist, ins, db, tags)
}
