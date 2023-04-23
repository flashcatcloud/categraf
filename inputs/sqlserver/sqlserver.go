package sqlserver

import (
	"database/sql"
	"errors"
	"time"

	"fmt"
	"log"
	"strings"
	"sync"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"

	mssql "github.com/denisenkom/go-mssqldb"
)

const inputName = "sqlserver"

// DO NOT REMOVE THE NEXT TWO LINES! This is required to embed the sampleConfig data.

var sampleConfig string

type SQLServer struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

// SQLServer struct
type Instance struct {
	config.InstanceConfig
	Servers      []string `toml:"servers"`
	AuthMethod   string   `toml:"auth_method"`
	QueryVersion int      `toml:"query_version" deprecated:"1.16.0;use 'database_type' instead"`
	DatabaseType string   `toml:"database_type"`
	IncludeQuery []string `toml:"include_query"`
	ExcludeQuery []string `toml:"exclude_query"`
	HealthMetric bool     `toml:"health_metric"`

	pools   []*sql.DB
	queries MapQuery
}

// Query struct
type Query struct {
	ScriptName     string
	Script         string
	ResultByRow    bool
	OrderedColumns []string
}

// MapQuery type
type MapQuery map[string]Query

// HealthMetric struct tracking the number of attempted vs successful connections for each connection string
type HealthMetric struct {
	AttemptedQueries  int
	SuccessfulQueries int
}

const (
	typeSQLServer = "SQLServer"
)

const (
	healthMetricName              = "sqlserver_health"
	healthMetricInstanceTag       = "sql_instance"
	healthMetricDatabaseTag       = "database_name"
	healthMetricAttemptedQueries  = "attempted_queries"
	healthMetricSuccessfulQueries = "successful_queries"
	healthMetricDatabaseType      = "database_type"
)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &SQLServer{}
	})
}

func (pt *SQLServer) Clone() inputs.Input {
	return &SQLServer{}
}

func (pt *SQLServer) Name() string {
	return inputName
}

func (pt *SQLServer) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type scanner interface {
	Scan(dest ...interface{}) error
}

// Start initialize a list of connection pools
func (s *Instance) Init() error {
	if len(s.Servers) == 0 {
		return types.ErrInstancesEmpty
	}

	if s.AuthMethod == "" {
		s.AuthMethod = "connection_string"
	}

	if err := s.initQueries(); err != nil {
		log.Println("E! initQueries err:", err)
		return err
	}

	for _, serv := range s.Servers {
		var pool *sql.DB

		switch strings.ToLower(s.AuthMethod) {
		case "connection_string":
			// Use the DSN (connection string) directly. In this case,
			// empty username/password causes use of Windows
			// integrated authentication.
			var err error
			pool, err = sql.Open("mssql", serv)
			if err != nil {
				log.Println("E! open mssql error:", err)
				continue
			}
		default:
			return errors.New(fmt.Sprintf("unknown auth method: %v", s.AuthMethod))
		}

		s.pools = append(s.pools, pool)
	}

	return nil
}
func (s *Instance) initQueries() error {
	s.queries = make(MapQuery)
	queries := s.queries
	log.Println("Config: database_type: ", s.DatabaseType, " query_version: ", s.QueryVersion)

	// To prevent query definition conflicts
	// Constant definitions for type "SQLServer" start with sqlServer
	if s.DatabaseType == typeSQLServer { // These are still V2 queries and have not been refactored yet.
		queries["SQLServerPerformanceCounters"] = Query{ScriptName: "SQLServerPerformanceCounters", Script: sqlServerPerformanceCounters, ResultByRow: false}
		queries["SQLServerWaitStatsCategorized"] = Query{ScriptName: "SQLServerWaitStatsCategorized", Script: sqlServerWaitStatsCategorized, ResultByRow: false}
		queries["SQLServerDatabaseIO"] = Query{ScriptName: "SQLServerDatabaseIO", Script: sqlServerDatabaseIO, ResultByRow: false}
		queries["SQLServerProperties"] = Query{ScriptName: "SQLServerProperties", Script: sqlServerProperties, ResultByRow: false}
		queries["SQLServerMemoryClerks"] = Query{ScriptName: "SQLServerMemoryClerks", Script: sqlServerMemoryClerks, ResultByRow: false}
		queries["SQLServerSchedulers"] = Query{ScriptName: "SQLServerSchedulers", Script: sqlServerSchedulers, ResultByRow: false}
		queries["SQLServerRequests"] = Query{ScriptName: "SQLServerRequests", Script: sqlServerRequests, ResultByRow: false}
		queries["SQLServerVolumeSpace"] = Query{ScriptName: "SQLServerVolumeSpace", Script: sqlServerVolumeSpace, ResultByRow: false}
		queries["SQLServerCpu"] = Query{ScriptName: "SQLServerCpu", Script: sqlServerRingBufferCPU, ResultByRow: false}
		queries["SQLServerAvailabilityReplicaStates"] = Query{ScriptName: "SQLServerAvailabilityReplicaStates", Script: sqlServerAvailabilityReplicaStates, ResultByRow: false}
		queries["SQLServerDatabaseReplicaStates"] = Query{ScriptName: "SQLServerDatabaseReplicaStates", Script: sqlServerDatabaseReplicaStates, ResultByRow: false}
		queries["SQLServerRecentBackups"] = Query{ScriptName: "SQLServerRecentBackups", Script: sqlServerRecentBackups, ResultByRow: false}
	} else {
		// Decide if we want to run version 1 or version 2 queries
		if s.QueryVersion == 2 {
			queries["PerformanceCounters"] = Query{ScriptName: "PerformanceCounters", Script: sqlPerformanceCountersV2, ResultByRow: true}
			queries["WaitStatsCategorized"] = Query{ScriptName: "WaitStatsCategorized", Script: sqlWaitStatsCategorizedV2, ResultByRow: false}
			queries["DatabaseIO"] = Query{ScriptName: "DatabaseIO", Script: sqlDatabaseIOV2, ResultByRow: false}
			queries["ServerProperties"] = Query{ScriptName: "ServerProperties", Script: sqlServerPropertiesV2, ResultByRow: false}
			queries["MemoryClerk"] = Query{ScriptName: "MemoryClerk", Script: sqlMemoryClerkV2, ResultByRow: false}
			queries["Schedulers"] = Query{ScriptName: "Schedulers", Script: sqlServerSchedulersV2, ResultByRow: false}
			queries["SqlRequests"] = Query{ScriptName: "SqlRequests", Script: sqlServerRequestsV2, ResultByRow: false}
			queries["VolumeSpace"] = Query{ScriptName: "VolumeSpace", Script: sqlServerVolumeSpaceV2, ResultByRow: false}
			queries["Cpu"] = Query{ScriptName: "Cpu", Script: sqlServerCPUV2, ResultByRow: false}
		} else {
			queries["PerformanceCounters"] = Query{ScriptName: "PerformanceCounters", Script: sqlPerformanceCounters, ResultByRow: true}
			queries["WaitStatsCategorized"] = Query{ScriptName: "WaitStatsCategorized", Script: sqlWaitStatsCategorized, ResultByRow: false}
			queries["CPUHistory"] = Query{ScriptName: "CPUHistory", Script: sqlCPUHistory, ResultByRow: false}
			queries["DatabaseIO"] = Query{ScriptName: "DatabaseIO", Script: sqlDatabaseIO, ResultByRow: false}
			queries["DatabaseSize"] = Query{ScriptName: "DatabaseSize", Script: sqlDatabaseSize, ResultByRow: false}
			queries["DatabaseStats"] = Query{ScriptName: "DatabaseStats", Script: sqlDatabaseStats, ResultByRow: false}
			queries["DatabaseProperties"] = Query{ScriptName: "DatabaseProperties", Script: sqlDatabaseProperties, ResultByRow: false}
			queries["MemoryClerk"] = Query{ScriptName: "MemoryClerk", Script: sqlMemoryClerk, ResultByRow: false}
			queries["VolumeSpace"] = Query{ScriptName: "VolumeSpace", Script: sqlVolumeSpace, ResultByRow: false}
			queries["PerformanceMetrics"] = Query{ScriptName: "PerformanceMetrics", Script: sqlPerformanceMetrics, ResultByRow: false}
		}
	}

	filterQueries, err := filter.NewIncludeExcludeFilter(s.IncludeQuery, s.ExcludeQuery)
	if err != nil {
		return err
	}

	for query := range queries {
		if !filterQueries.Match(query) {
			delete(queries, query)
		}
	}

	var querylist []string
	for query := range queries {
		querylist = append(querylist, query)
	}
	log.Println("Config: Effective Queries: ", querylist)

	return nil
}

// Gather collect data from SQL Server
func (s *Instance) Gather(slist *types.SampleList) {
	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		tags := map[string]string{}
		for i, _ := range s.pools {
			connectionString := s.Servers[i]
			serverName, databaseName := getConnectionIdentifiers(connectionString)
			tags["serverName"] = serverName
			tags["databaseName"] = databaseName
		}
		slist.PushSample(inputName, "scrape_use_seconds", use, tags)
	}(begun)

	var wg sync.WaitGroup
	var mutex sync.Mutex
	var healthMetrics = make(map[string]*HealthMetric)
	tags := map[string]string{}
	for i, pool := range s.pools {
		wg.Add(1)
		query_up := Query{ScriptName: "SQLServerUp", Script: sqlServerUp, ResultByRow: false}
		go func(pool *sql.DB, query Query, serverIndex int) {
			defer wg.Done()
			connectionString := s.Servers[serverIndex]
			serverName, databaseName := getConnectionIdentifiers(connectionString)
			tags["serverName"] = serverName
			tags["databaseName"] = databaseName
			rows, err := pool.Query(query.Script)
			if err != nil {
				slist.PushSample(inputName, "up", 0, tags)
			} else {
				slist.PushSample(inputName, "up", 1, tags)
				defer rows.Close()
			}
		}(pool, query_up, i)

		for _, query := range s.queries {
			wg.Add(1)
			go func(pool *sql.DB, query Query, serverIndex int) {
				defer wg.Done()
				connectionString := s.Servers[serverIndex]
				queryError := s.gatherServer(pool, query, slist, connectionString)

				if queryError != nil {
					log.Println("E! queryError is ", queryError)
				}
				if s.HealthMetric {
					mutex.Lock()
					s.gatherHealth(healthMetrics, connectionString, queryError)
					mutex.Unlock()
				}
			}(pool, query, i)
		}
	}

	wg.Wait()

	if s.HealthMetric {
		s.accHealth(healthMetrics, slist)
	}

}

func (s *Instance) gatherServer(pool *sql.DB, query Query, slist *types.SampleList, connectionString string) error {
	// execute query
	rows, err := pool.Query(query.Script)
	if err != nil {
		serverName, databaseName := getConnectionIdentifiers(connectionString)

		// Error msg based on the format in SSMS. SQLErrorClass() is another term for severity/level: http://msdn.microsoft.com/en-us/library/dd304156.aspx
		if sqlerr, ok := err.(mssql.Error); ok {
			return fmt.Errorf("query %s failed for server: %s and database: %s with Msg %d, Level %d, State %d:, Line %d, Error: %w", query.ScriptName,
				serverName, databaseName, sqlerr.SQLErrorNumber(), sqlerr.SQLErrorClass(), sqlerr.SQLErrorState(), sqlerr.SQLErrorLineNo(), err)
		}

		return fmt.Errorf("query %s failed for server: %s and database: %s with Error: %w", query.ScriptName, serverName, databaseName, err)
	}

	defer rows.Close()

	// grab the column information from the result
	query.OrderedColumns, err = rows.Columns()
	if err != nil {
		return err
	}

	for rows.Next() {
		err = s.accRow(query, slist, rows)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Instance) accRow(query Query, slist *types.SampleList, row scanner) error {
	var columnVars []interface{}
	var fields = make(map[string]interface{})

	// store the column name with its *interface{}
	columnMap := make(map[string]*interface{})
	for _, column := range query.OrderedColumns {
		columnMap[column] = new(interface{})
	}
	// populate the array of interface{} with the pointers in the right order
	for i := 0; i < len(columnMap); i++ {
		columnVars = append(columnVars, columnMap[query.OrderedColumns[i]])
	}
	// deconstruct array of variables and send to Scan
	err := row.Scan(columnVars...)
	if err != nil {
		return err
	}

	// measurement: identified by the header
	// tags: all other fields of type string
	tags := map[string]string{}
	var measurement string
	for header, val := range columnMap {
		if str, ok := (*val).(string); ok {
			if header == "measurement" {
				measurement = str
			} else {
				tags[header] = str
			}
		}
	}

	if s.DatabaseType != "" {
		tags["measurement_db_type"] = s.DatabaseType
	}

	if query.ResultByRow {
		if strings.HasPrefix(measurement, inputName) {
			slist.PushSample("", measurement+"_value", *columnMap["value"], tags)
		} else {
			slist.PushSample("", measurement+"_value", *columnMap["value"], tags)
		}

	} else {
		// values
		for header, val := range columnMap {
			if _, ok := (*val).(string); !ok {
				fields[header] = *val
			}
		}
		// add fields to Accumulator
		for k, v := range fields {
			if strings.HasPrefix(measurement, inputName) {
				slist.PushSample("", measurement+"_"+k, v, tags)
			} else {
				slist.PushSample(inputName, measurement+"_"+k, v, tags)
			}

		}

	}
	return nil
}

// gatherHealth stores info about any query errors in the healthMetrics map
func (s *Instance) gatherHealth(healthMetrics map[string]*HealthMetric, serv string, queryError error) {
	if healthMetrics[serv] == nil {
		healthMetrics[serv] = &HealthMetric{}
	}

	healthMetrics[serv].AttemptedQueries++
	if queryError == nil {
		healthMetrics[serv].SuccessfulQueries++
	}
}

// accHealth accumulates the query health data contained within the healthMetrics map
func (s *Instance) accHealth(healthMetrics map[string]*HealthMetric, slist *types.SampleList) {
	for connectionString, connectionStats := range healthMetrics {
		sqlInstance, databaseName := getConnectionIdentifiers(connectionString)
		tags := map[string]string{healthMetricInstanceTag: sqlInstance, healthMetricDatabaseTag: databaseName}
		fields := map[string]interface{}{
			healthMetricAttemptedQueries:  connectionStats.AttemptedQueries,
			healthMetricSuccessfulQueries: connectionStats.SuccessfulQueries,
			healthMetricDatabaseType:      s.getDatabaseTypeToLog(),
		}

		for k, v := range fields {
			if strings.HasPrefix(healthMetricName, inputName) {
				slist.PushSample("", healthMetricName+"_"+k, v, tags)
			} else {
				slist.PushSample(inputName, healthMetricName+"_"+k, v, tags)
			}

		}

	}
}

// getDatabaseTypeToLog returns the type of database monitored by this plugin instance
func (s *Instance) getDatabaseTypeToLog() string {
	if s.DatabaseType == typeSQLServer {
		return s.DatabaseType
	}

	logname := fmt.Sprintf("QueryVersion-%d", s.QueryVersion)
	return logname
}

// Stop cleanup server connection pools
func (s *Instance) Drop() {
	for _, pool := range s.pools {
		_ = pool.Close()
	}
}
