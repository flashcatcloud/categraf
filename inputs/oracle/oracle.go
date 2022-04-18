package oracle

import (
	"fmt"
	"log"
	"sync"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"github.com/jmoiron/sqlx"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "oracle"

type OrclInstance struct {
	Address               string `toml:"address"`
	Username              string `toml:"username"`
	Password              string `toml:"password"`
	IsSysDBA              bool   `toml:"is_sys_dba"`
	IsSysOper             bool   `toml:"is_sys_oper"`
	DisableConnectionPool bool   `toml:"disable_connection_pool"`
	MaxOpenConnections    int    `toml:"max_open_connections"`
}

type MetricConfig struct {
	Mesurement       string            `toml:"mesurement"`
	LabelFields      []string          `toml:"label_fields"`
	MetricFields     map[string]string `toml:"metric_fields"` // column_name -> value type(float64, bool, int64)
	FieldToAppend    string            `toml:"field_to_append"`
	Timeout          config.Duration   `toml:"timeout"`
	Request          string            `toml:"request"`
	IgnoreZeroResult bool              `toml:"ignore_zero_result"`
}

type Oracle struct {
	PrintConfigs bool            `toml:"print_configs"`
	Interval     config.Duration `toml:"interval"`
	Instances    []OrclInstance  `toml:"instances"`
	Metrics      []MetricConfig  `toml:"metrics"`

	dbconnpool map[string]*sqlx.DB // key: instance
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
		return fmt.Errorf("oracle instances empty")
	}

	o.dbconnpool = make(map[string]*sqlx.DB)
	for i := 0; i < len(o.Instances); i++ {
		dbConf := o.Instances[i]
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
	slist := list.NewSafeList()

	var wg sync.WaitGroup
	for i := range o.Instances {
		wg.Add(1)
		go o.collectOnce(&wg, o.Instances[i], slist)
	}
	wg.Wait()

	interfaceList := slist.PopBackAll()
	for i := 0; i < len(interfaceList); i++ {
		samples = append(samples, interfaceList[i].(*types.Sample))
	}

	return
}

func (o *Oracle) collectOnce(wg *sync.WaitGroup, ins OrclInstance, slist *list.SafeList) {
	log.Println("->", ins.Address)
	log.Printf("%#v\n", ins)
	log.Println("-> metrics count:", len(o.Metrics))

	log.Println(o.Metrics[0].Mesurement)
	log.Println(o.Metrics[0].Request)
	log.Println(o.Metrics[0].FieldToAppend)
	log.Println(o.Metrics[0].IgnoreZeroResult)
	log.Println(o.Metrics[0].LabelFields)
	log.Println(o.Metrics[0].MetricFields)

	defer wg.Done()
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
