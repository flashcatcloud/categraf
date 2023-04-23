package postgresql

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	// Blank import required to register driver
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/types"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
)

const (
	inputName = "postgresql"
)

type Postgresql struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Postgresql{}
	})
}

func (pt *Postgresql) Clone() inputs.Input {
	return &Postgresql{}
}

func (pt *Postgresql) Name() string {
	return inputName
}

func (pt *Postgresql) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}
func (pt *Postgresql) Drop() {
	for i := 0; i < len(pt.Instances); i++ {
		pt.Instances[i].Drop()
	}
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

type Instance struct {
	config.InstanceConfig

	Address            string          `toml:"address"`
	MaxLifetime        config.Duration `toml:"max_lifetime"`
	IsPgBouncer        bool            `toml:"-"`
	OutputAddress      string          `toml:"outputaddress"`
	Databases          []string        `toml:"databases"`
	IgnoredDatabases   []string        `toml:"ignored_databases"`
	PreparedStatements bool            `toml:"prepared_statements"`
	Metrics            []MetricConfig  `toml:"metrics"`

	MaxIdle int
	MaxOpen int
	DB      *sql.DB
}

var ignoredColumns = map[string]bool{"stats_reset": true}

func (p *Instance) IgnoredColumns() map[string]bool {
	return ignoredColumns
}

var socketRegexp = regexp.MustCompile(`/\.s\.PGSQL\.\d+$`)

func (ins *Instance) Init() error {
	if ins.Address == "" {
		return types.ErrInstancesEmpty
	}
	ins.MaxIdle = 1
	ins.MaxOpen = 1
	// ins.MaxLifetime = config.Duration(0)
	if !ins.IsPgBouncer {
		ins.PreparedStatements = true
		ins.IsPgBouncer = false
	} else {
		ins.PreparedStatements = false
	}
	const localhost = "host=localhost sslmode=disable"

	if ins.Address == "localhost" {
		ins.Address = localhost
	}

	connConfig, err := pgx.ParseConfig(ins.Address)
	if err != nil {
		log.Println("E! can't parse address :", err)
		return err
	}

	// Remove the socket name from the path
	connConfig.Host = socketRegexp.ReplaceAllLiteralString(connConfig.Host, "")

	// Specific support to make it work with PgBouncer too
	// See https://github.com/influxdata/telegraf/issues/3253#issuecomment-357505343
	if ins.IsPgBouncer {
		// Remove DriveConfig and revert it by the ParseConfig method
		// See https://github.com/influxdata/telegraf/issues/9134
		connConfig.PreferSimpleProtocol = true
	}

	connectionString := stdlib.RegisterConnConfig(connConfig)
	if ins.DB, err = sql.Open("pgx", connectionString); err != nil {
		log.Println("E! can't open db :", err)
		return err
	}

	ins.DB.SetMaxOpenConns(ins.MaxOpen)
	ins.DB.SetMaxIdleConns(ins.MaxIdle)
	ins.DB.SetConnMaxLifetime(time.Duration(ins.MaxLifetime))
	return nil
}

// closes any necessary channels and connections
func (p *Instance) Drop() {
	// Ignore the returned error as we cannot do anything about it anyway
	//nolint:errcheck,revive
	p.DB.Close()
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var (
		err     error
		query   string
		columns []string
	)

	if len(ins.Databases) == 0 && len(ins.IgnoredDatabases) == 0 {
		query = `SELECT * FROM pg_stat_database`
	} else if len(ins.IgnoredDatabases) != 0 {
		query = fmt.Sprintf(`SELECT * FROM pg_stat_database WHERE datname NOT IN ('%s')`,
			strings.Join(ins.IgnoredDatabases, "','"))
	} else {
		query = fmt.Sprintf(`SELECT * FROM pg_stat_database WHERE datname IN ('%s')`,
			strings.Join(ins.Databases, "','"))
	}

	rows, err := ins.DB.Query(query)
	if err != nil {
		log.Println("E! failed to execute Query :", err)
		return
	}

	defer rows.Close()

	// grab the column information from the result
	if columns, err = rows.Columns(); err != nil {
		log.Println("E! failed to grab column info:", err)
		return
	}

	for rows.Next() {
		err = ins.accRow(rows, slist, columns)
		if err != nil {
			log.Println("E! failed to get row data:", err)
			return
		}
	}

	query = `SELECT * FROM pg_stat_bgwriter`

	bgWriterRow, err := ins.DB.Query(query)
	if err != nil {
		log.Println("E! failed to execute Query:", err)
		return
	}

	defer bgWriterRow.Close()

	// grab the column information from the result
	if columns, err = bgWriterRow.Columns(); err != nil {
		log.Println("E! failed to grab column info:", err)
		return
	}

	for bgWriterRow.Next() {
		err = ins.accRow(bgWriterRow, slist, columns)
		if err != nil {
			log.Println("E! failed to get row data:", err)
			return
		}
	}

	waitMetrics := new(sync.WaitGroup)

	for i := 0; i < len(ins.Metrics); i++ {
		m := ins.Metrics[i]
		waitMetrics.Add(1)
		tags := map[string]string{}
		go ins.scrapeMetric(waitMetrics, slist, m, tags)
	}

	waitMetrics.Wait()
}

func (ins *Instance) scrapeMetric(waitMetrics *sync.WaitGroup, slist *types.SampleList, metricConf MetricConfig, tags map[string]string) {
	defer waitMetrics.Done()

	timeout := time.Duration(metricConf.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := ins.DB.QueryContext(ctx, metricConf.Request)

	if ctx.Err() == context.DeadlineExceeded {
		log.Println("E! postgresql query timeout, request:", metricConf.Request)
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
			slist.PushSample(inputName, metricConf.Mesurement+"_"+column, value, labels)
		} else {
			suffix := cleanName(row[metricConf.FieldToAppend])
			slist.PushSample(inputName, metricConf.Mesurement+"_"+suffix+"_"+column, value, labels)
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

type scanner interface {
	Scan(dest ...interface{}) error
}

func (ins *Instance) accRow(row scanner, slist *types.SampleList, columns []string) error {
	var columnVars []interface{}
	var dbname bytes.Buffer

	// this is where we'll store the column name with its *interface{}
	columnMap := make(map[string]*interface{})

	for _, column := range columns {
		columnMap[column] = new(interface{})
	}

	// populate the array of interface{} with the pointers in the right order
	for i := 0; i < len(columnMap); i++ {
		columnVars = append(columnVars, columnMap[columns[i]])
	}

	// deconstruct array of variables and send to Scan
	err := row.Scan(columnVars...)

	if err != nil {
		return err
	}
	if columnMap["datname"] != nil {
		// extract the database name from the column map
		if dbNameStr, ok := (*columnMap["datname"]).(string); ok {
			if _, err := dbname.WriteString(dbNameStr); err != nil {
				log.Println("E! failed to WriteString:", dbNameStr, err)
				return err
			}
		} else {
			// PG 12 adds tracking of global objects to pg_stat_database
			if _, err := dbname.WriteString("postgres_global"); err != nil {
				log.Println("E! failed to WriteString: postgres_global", err)
				return err
			}
		}
	} else {
		if _, err := dbname.WriteString("postgres"); err != nil {
			log.Println("E! failed to WriteString: postgres", err)
			return err
		}
	}

	var tagAddress string
	tagAddress, err = ins.SanitizedAddress()
	if err != nil {
		log.Println("E! failed to SanitizedAddress", err)
		return err
	}

	tags := map[string]string{"server": tagAddress, "db": dbname.String()}

	fields := make(map[string]interface{})
	for col, val := range columnMap {
		_, ignore := ignoredColumns[col]
		if !ignore {
			fields[col] = *val
		}
	}
	// acc.AddFields("postgresql", fields, tags)
	for key, val := range fields {
		slist.PushSample(inputName, key, val, tags)
	}
	return nil
}

// This will be blank, causing driver.Open to use all of the defaults
func parseURL(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("invalid connection protocol: %s", u.Scheme)
	}

	var kvs []string
	escaper := strings.NewReplacer(` `, `\ `, `'`, `\'`, `\`, `\\`)
	accrue := func(k, v string) {
		if v != "" {
			kvs = append(kvs, k+"="+escaper.Replace(v))
		}
	}

	if u.User != nil {
		v := u.User.Username()
		accrue("user", v)

		v, _ = u.User.Password()
		accrue("password", v)
	}

	if host, port, err := net.SplitHostPort(u.Host); err != nil {
		accrue("host", u.Host)
	} else {
		accrue("host", host)
		accrue("port", port)
	}

	if u.Path != "" {
		accrue("dbname", u.Path[1:])
	}

	q := u.Query()
	for k := range q {
		accrue(k, q.Get(k))
	}

	sort.Strings(kvs) // Makes testing easier (not a performance concern)
	return strings.Join(kvs, " "), nil
}

var kvMatcher, _ = regexp.Compile(`(password|sslcert|sslkey|sslmode|sslrootcert)=\S+ ?`)

// SanitizedAddress utility function to strip sensitive information from the connection string.
func (ins *Instance) SanitizedAddress() (sanitizedAddress string, err error) {
	var (
		canonicalizedAddress string
	)

	if ins.OutputAddress != "" {
		return ins.OutputAddress, nil
	}

	if strings.HasPrefix(ins.Address, "postgres://") || strings.HasPrefix(ins.Address, "postgresql://") {
		if canonicalizedAddress, err = parseURL(ins.Address); err != nil {
			return sanitizedAddress, err
		}
	} else {
		canonicalizedAddress = ins.Address
	}

	sanitizedAddress = kvMatcher.ReplaceAllString(canonicalizedAddress, "")

	return sanitizedAddress, err
}
