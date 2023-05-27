package clickhouse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/pkg/stringx"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"

	"github.com/tidwall/gjson"
)

const inputName = "clickhouse"

var defaultTimeout = 5 * time.Second

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
	Username       string          `toml:"username"`
	Password       string          `toml:"password"`
	Servers        []string        `toml:"servers"`
	AutoDiscovery  bool            `toml:"auto_discovery"`
	ClusterInclude []string        `toml:"cluster_include"`
	ClusterExclude []string        `toml:"cluster_exclude"`
	Timeout        config.Duration `toml:"timeout"`
	Metrics        []MetricConfig  `toml:"metrics"`
	HTTPClient     *http.Client
	tls.ClientConfig
}

type connect struct {
	Cluster  string `json:"cluster"`
	ShardNum int    `json:"shard_num"`
	Hostname string `json:"host_name"`
	url      *url.URL
}

func (ins *Instance) Init() error {
	if len(ins.Servers) == 0 {
		return types.ErrInstancesEmpty
	}

	timeout := defaultTimeout
	if time.Duration(ins.Timeout) != 0 {
		timeout = time.Duration(ins.Timeout)
	}
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return err
	}

	ins.HTTPClient = &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: 1,
		},
	}
	return nil
}

type ClickHouse struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &ClickHouse{}
	})
}

func (ck *ClickHouse) Clone() inputs.Input {
	return &ClickHouse{}
}

func (ck *ClickHouse) Name() string {
	return inputName
}

func (ck *ClickHouse) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(ck.Instances))
	for i := 0; i < len(ck.Instances); i++ {
		ret[i] = ck.Instances[i]
	}
	return ret
}

func (ck *ClickHouse) Drop() {
	for i := 0; i < len(ck.Instances); i++ {
		if ck.Instances[i].HTTPClient != nil {
			ck.Instances[i].HTTPClient.CloseIdleConnections()
		}
	}
}

// Gather collect data from ClickHouse server
func (ins *Instance) Gather(slist *types.SampleList) {
	var (
		connects []connect
		exists   = func(host string) bool {
			for _, c := range connects {
				if c.Hostname == host {
					return true
				}
			}
			return false
		}
	)

	for _, server := range ins.Servers {
		u, err := url.Parse(server)
		if err != nil {
			log.Println("E! failed to parse server url, error: ", err)
			return
		}
		switch {
		case ins.AutoDiscovery:
			var conns []connect
			if err := ins.execQuery(u, "SELECT cluster, shard_num, host_name FROM system.clusters "+ins.clusterIncludeExcludeFilter(), &conns); err != nil {
				log.Println("E! failed to exec clickhouse query:", "SELECT cluster, shard_num, host_name FROM system.clusters "+ins.clusterIncludeExcludeFilter())
				continue
			}
			for _, c := range conns {
				if !exists(c.Hostname) {
					c.url = &url.URL{
						Scheme: u.Scheme,
						Host:   net.JoinHostPort(c.Hostname, u.Port()),
					}
					connects = append(connects, c)
				}
			}
		default:
			connects = append(connects, connect{
				Hostname: u.Hostname(),
				url:      u,
			})
		}
	}

	for i := range connects {
		metricsFuncs := []func(slist *types.SampleList, conn *connect) error{
			ins.tables,
			ins.zookeeper,
			ins.replicationQueue,
			ins.detachedParts,
			ins.dictionaries,
			ins.mutations,
			ins.disks,
			ins.processes,
			ins.textLog,
		}

		for _, metricFunc := range metricsFuncs {
			if err := metricFunc(slist, &connects[i]); err != nil {
				log.Println("E! failed to exec  metrics Funcs error:", err)
			}
		}

		for metric := range commonMetrics {
			if err := ins.commonMetrics(slist, &connects[i], metric); err != nil {
				log.Println("E! failed to exec query commonMetrics error:", err)
			}
		}
		log.Println("E!metrics=", len(ins.Metrics))
		waitMetrics := new(sync.WaitGroup)

		for i := 0; i < len(ins.Metrics); i++ {
			m := ins.Metrics[i]
			waitMetrics.Add(1)
			//tags := map[string]string{"address": ins.Address}
			//go ins.scrapeMetric(waitMetrics, slist, m, tags)
			go ins.execCustomQuery(&connects[i], waitMetrics, slist, m)
		}
		waitMetrics.Wait()

	}
	return
}

func (ins *Instance) clusterIncludeExcludeFilter() string {
	if len(ins.ClusterInclude) == 0 && len(ins.ClusterExclude) == 0 {
		return ""
	}
	var (
		escape = func(in string) string {
			return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(in) + "'"
		}
		makeFilter = func(expr string, args []string) string {
			in := make([]string, 0, len(args))
			for _, v := range args {
				in = append(in, escape(v))
			}
			return fmt.Sprintf("cluster %s (%s)", expr, strings.Join(in, ", "))
		}
		includeFilter, excludeFilter string
	)

	if len(ins.ClusterInclude) != 0 {
		includeFilter = makeFilter("IN", ins.ClusterInclude)
	}
	if len(ins.ClusterExclude) != 0 {
		excludeFilter = makeFilter("NOT IN", ins.ClusterExclude)
	}
	if includeFilter != "" && excludeFilter != "" {
		return "WHERE " + includeFilter + " OR " + excludeFilter
	}
	if includeFilter == "" && excludeFilter != "" {
		return "WHERE " + excludeFilter
	}
	return "WHERE " + includeFilter
}

func (ins *Instance) commonMetrics(slist *types.SampleList, conn *connect, metric string) error {
	var intResult []struct {
		Metric string   `json:"metric"`
		Value  chUInt64 `json:"value"`
	}

	var floatResult []struct {
		Metric string  `json:"metric"`
		Value  float64 `json:"value"`
	}

	tags := ins.makeDefaultTags(conn)

	if commonMetricsIsFloat[metric] {
		if err := ins.execQuery(conn.url, commonMetrics[metric], &floatResult); err != nil {
			return err
		}
		for _, r := range floatResult {
			slist.PushFront(types.NewSample("clickhouse_"+metric, stringx.SnakeCase(r.Metric), r.Value, tags))
		}
	} else {
		if err := ins.execQuery(conn.url, commonMetrics[metric], &intResult); err != nil {
			return err
		}
		for _, r := range intResult {
			slist.PushFront(types.NewSample("clickhouse_"+metric, stringx.SnakeCase(r.Metric), r.Value, tags))
		}
	}
	return nil
}

func (ins *Instance) zookeeper(slist *types.SampleList, conn *connect) error {
	var zkExists []struct {
		ZkExists chUInt64 `json:"zk_exists"`
	}

	if err := ins.execQuery(conn.url, systemZookeeperExistsSQL, &zkExists); err != nil {
		return err
	}
	tags := ins.makeDefaultTags(conn)

	if len(zkExists) > 0 && zkExists[0].ZkExists > 0 {
		var zkRootNodes []struct {
			ZkRootNodes chUInt64 `json:"zk_root_nodes"`
		}
		if err := ins.execQuery(conn.url, systemZookeeperRootNodesSQL, &zkRootNodes); err != nil {
			return err
		}

		slist.PushFront(types.NewSample("clickhouse_zookeeper", "root_nodes", uint64(zkRootNodes[0].ZkRootNodes), tags))
	}
	return nil
}

func (ins *Instance) replicationQueue(slist *types.SampleList, conn *connect) error {
	var replicationQueueExists []struct {
		ReplicationQueueExists chUInt64 `json:"replication_queue_exists"`
	}

	if err := ins.execQuery(conn.url, systemReplicationExistsSQL, &replicationQueueExists); err != nil {
		return err
	}

	tags := ins.makeDefaultTags(conn)

	if len(replicationQueueExists) > 0 && replicationQueueExists[0].ReplicationQueueExists > 0 {
		var replicationTooManyTries []struct {
			NumTriesReplicas     chUInt64 `json:"replication_num_tries_replicas"`
			TooManyTriesReplicas chUInt64 `json:"replication_too_many_tries_replicas"`
		}
		if err := ins.execQuery(conn.url, systemReplicationNumTriesSQL, &replicationTooManyTries); err != nil {
			return err
		}

		slist.PushFront(types.NewSample("clickhouse_replication_queue", "too_many_tries_replicas", uint64(replicationTooManyTries[0].TooManyTriesReplicas), tags))
		slist.PushFront(types.NewSample("clickhouse_replication_queue", "num_tries_replicas", uint64(replicationTooManyTries[0].NumTriesReplicas), tags))
	}
	return nil
}

func (ins *Instance) detachedParts(slist *types.SampleList, conn *connect) error {
	var detachedParts []struct {
		DetachedParts chUInt64 `json:"detached_parts"`
	}
	if err := ins.execQuery(conn.url, systemDetachedPartsSQL, &detachedParts); err != nil {
		return err
	}

	if len(detachedParts) > 0 {
		tags := ins.makeDefaultTags(conn)
		slist.PushFront(types.NewSample("clickhouse_detached_parts", "detached_parts", uint64(detachedParts[0].DetachedParts), tags))
	}
	return nil
}

func (ins *Instance) dictionaries(slist *types.SampleList, conn *connect) error {
	var brokenDictionaries []struct {
		Origin         string   `json:"origin"`
		BytesAllocated chUInt64 `json:"bytes_allocated"`
		Status         string   `json:"status"`
	}
	if err := ins.execQuery(conn.url, systemDictionariesSQL, &brokenDictionaries); err != nil {
		return err
	}

	for _, dict := range brokenDictionaries {
		tags := ins.makeDefaultTags(conn)

		isLoaded := uint64(1)
		if dict.Status != "LOADED" {
			isLoaded = 0
		}

		if dict.Origin != "" {
			tags["dict_origin"] = dict.Origin
			slist.PushFront(types.NewSample("clickhouse_dictionaries", "is_loaded", isLoaded, tags))
			slist.PushFront(types.NewSample("clickhouse_dictionaries", "bytes_allocated", uint64(dict.BytesAllocated), tags))
		}
	}

	return nil
}

func (ins *Instance) mutations(slist *types.SampleList, conn *connect) error {
	var mutationsStatus []struct {
		Failed    chUInt64 `json:"failed"`
		Running   chUInt64 `json:"running"`
		Completed chUInt64 `json:"completed"`
	}
	if err := ins.execQuery(conn.url, systemMutationSQL, &mutationsStatus); err != nil {
		return err
	}

	if len(mutationsStatus) > 0 {
		tags := ins.makeDefaultTags(conn)

		slist.PushFront(types.NewSample("clickhouse_mutations", "failed", uint64(mutationsStatus[0].Failed), tags))
		slist.PushFront(types.NewSample("clickhouse_mutations", "running", uint64(mutationsStatus[0].Running), tags))
		slist.PushFront(types.NewSample("clickhouse_mutations", "completed", uint64(mutationsStatus[0].Completed), tags))
	}

	return nil
}

func (ins *Instance) disks(slist *types.SampleList, conn *connect) error {
	var disksStatus []struct {
		Name            string   `json:"name"`
		Path            string   `json:"path"`
		FreePercent     chUInt64 `json:"free_space_percent"`
		KeepFreePercent chUInt64 `json:"keep_free_space_percent"`
	}

	if err := ins.execQuery(conn.url, systemDisksSQL, &disksStatus); err != nil {
		return err
	}

	for _, disk := range disksStatus {
		tags := ins.makeDefaultTags(conn)
		tags["name"] = disk.Name
		tags["path"] = disk.Path

		slist.PushFront(types.NewSample("clickhouse_disks", "free_space_percent", uint64(disk.FreePercent), tags))
		slist.PushFront(types.NewSample("clickhouse_disks", "keep_free_space_percent", uint64(disk.KeepFreePercent), tags))
	}

	return nil
}

func (ins *Instance) processes(slist *types.SampleList, conn *connect) error {
	var processesStats []struct {
		QueryType      string  `json:"query_type"`
		Percentile50   float64 `json:"p50"`
		Percentile90   float64 `json:"p90"`
		LongestRunning float64 `json:"longest_running"`
	}

	if err := ins.execQuery(conn.url, systemProcessesSQL, &processesStats); err != nil {
		return err
	}

	for _, process := range processesStats {
		tags := ins.makeDefaultTags(conn)
		tags["query_type"] = process.QueryType

		slist.PushFront(types.NewSample("clickhouse_processes", "percentile_50", process.Percentile50, tags))
		slist.PushFront(types.NewSample("clickhouse_processes", "percentile_90", process.Percentile90, tags))
		slist.PushFront(types.NewSample("clickhouse_processes", "longest_running", process.LongestRunning, tags))
	}

	return nil
}

func (ins *Instance) textLog(slist *types.SampleList, conn *connect) error {
	var textLogExists []struct {
		TextLogExists chUInt64 `json:"text_log_exists"`
	}

	if err := ins.execQuery(conn.url, systemTextLogExistsSQL, &textLogExists); err != nil {
		return err
	}

	if len(textLogExists) > 0 && textLogExists[0].TextLogExists > 0 {
		var textLogLast10MinMessages []struct {
			Level             string   `json:"level"`
			MessagesLast10Min chUInt64 `json:"messages_last_10_min"`
		}
		if err := ins.execQuery(conn.url, systemTextLogSQL, &textLogLast10MinMessages); err != nil {
			return err
		}

		for _, textLogItem := range textLogLast10MinMessages {
			tags := ins.makeDefaultTags(conn)
			tags["level"] = textLogItem.Level
			slist.PushFront(types.NewSample("clickhouse_text_log", "messages_last_10_min", uint64(textLogItem.MessagesLast10Min), tags))
		}
	}
	return nil
}

func (ins *Instance) tables(slist *types.SampleList, conn *connect) error {
	var parts []struct {
		Database string   `json:"database"`
		Table    string   `json:"table"`
		Bytes    chUInt64 `json:"bytes"`
		Parts    chUInt64 `json:"parts"`
		Rows     chUInt64 `json:"rows"`
	}

	if err := ins.execQuery(conn.url, systemPartsSQL, &parts); err != nil {
		return err
	}
	tags := ins.makeDefaultTags(conn)

	for _, part := range parts {
		tags["table"] = part.Table
		tags["database"] = part.Database
		slist.PushFront(types.NewSample("clickhouse_tables", "bytes", uint64(part.Bytes), tags))
		slist.PushFront(types.NewSample("clickhouse_tables", "parts", uint64(part.Parts), tags))
		slist.PushFront(types.NewSample("clickhouse_tables", "rows", uint64(part.Rows), tags))
	}
	return nil
}

func (ins *Instance) makeDefaultTags(conn *connect) map[string]string {
	tags := map[string]string{
		"source": conn.Hostname,
	}
	if len(conn.Cluster) != 0 {
		tags["cluster"] = conn.Cluster
	}
	if conn.ShardNum != 0 {
		tags["shard_num"] = strconv.Itoa(conn.ShardNum)
	}
	return tags
}

type clickhouseError struct {
	StatusCode int
	body       []byte
}

func (e *clickhouseError) Error() string {
	return fmt.Sprintf("received error code %d: %s", e.StatusCode, e.body)
}

func (ins *Instance) execQuery(address *url.URL, query string, i interface{}) error {
	q := address.Query()
	q.Set("query", query+" FORMAT JSON")
	address.RawQuery = q.Encode()
	req, _ := http.NewRequest("GET", address.String(), nil)
	if ins.Username != "" {
		req.Header.Add("X-ClickHouse-User", ins.Username)
	}
	if ins.Password != "" {
		req.Header.Add("X-ClickHouse-Key", ins.Password)
	}
	resp, err := ins.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return &clickhouseError{
			StatusCode: resp.StatusCode,
			body:       body,
		}
	}
	var response struct {
		Data json.RawMessage
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}
	if err := json.Unmarshal(response.Data, i); err != nil {
		return err
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return err
	}
	return nil
}

func (ins *Instance) execCustomQuery(conn *connect, waitMetrics *sync.WaitGroup, slist *types.SampleList, metricConf MetricConfig) error {
	defer waitMetrics.Done()
	address := conn.url
	q := address.Query()
	q.Set("query", metricConf.Request+" FORMAT JSON")
	address.RawQuery = q.Encode()
	req, _ := http.NewRequest("GET", address.String(), nil)
	if ins.Username != "" {
		req.Header.Add("X-ClickHouse-User", ins.Username)
	}
	if ins.Password != "" {
		req.Header.Add("X-ClickHouse-Key", ins.Password)
	}
	resp, err := ins.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return &clickhouseError{
			StatusCode: resp.StatusCode,
			body:       body,
		}
	}
	var response struct {
		Data []json.RawMessage `json:"data"`
	}
	tags := ins.makeDefaultTags(conn)
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}
	log.Println("content:", len(response.Data))
	for _, item := range response.Data {
		localTags := tags
		for _, label := range metricConf.LabelFields {
			localTags[label] = gjson.Get(string(item), label).String()
		}

		for _, column := range metricConf.MetricFields {
			value, err := conv.ToFloat64(gjson.Get(string(item), column).String())
			if err != nil {
				log.Println("E! failed to convert field:", column, "value:", value, "error:", err)
				return err
			}

			if metricConf.FieldToAppend == "" {
				slist.PushSample(inputName, metricConf.Mesurement+"_"+column, value, localTags)
			} else {
				suffix := cleanName(gjson.Get(string(item), metricConf.FieldToAppend).String())
				slist.PushSample(inputName, metricConf.Mesurement+"_"+suffix+"_"+column, value, localTags)
			}
		}
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return err
	}

	//slist.PushFront(types.NewSample("clickhouse_zookeeper", "root_nodes", uint64(zkRootNodes[0].ZkRootNodes), tags))
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

// see https://clickhouse.yandex/docs/en/operations/settings/settings/#session_settings-output_format_json_quote_64bit_integers
type chUInt64 uint64

func (i *chUInt64) UnmarshalJSON(b []byte) error {
	b = bytes.TrimPrefix(b, []byte(`"`))
	b = bytes.TrimSuffix(b, []byte(`"`))
	v, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil {
		return err
	}
	*i = chUInt64(v)
	return nil
}

const (
	systemEventsSQL       = "SELECT event AS metric, toUInt64(value) AS value FROM system.events"
	systemMetricsSQL      = "SELECT          metric, toUInt64(value) AS value FROM system.metrics"
	systemAsyncMetricsSQL = "SELECT          metric, toFloat64(value) AS value FROM system.asynchronous_metrics"
	systemPartsSQL        = `
		SELECT
			database,
			table,
			SUM(bytes) AS bytes,
			COUNT(*)   AS parts,
			SUM(rows)  AS rows
		FROM system.parts
		WHERE active = 1
		GROUP BY
			database, table
		ORDER BY
			database, table
	`
	systemZookeeperExistsSQL    = "SELECT count() AS zk_exists FROM system.tables WHERE database='system' AND name='zookeeper'"
	systemZookeeperRootNodesSQL = "SELECT count() AS zk_root_nodes FROM system.zookeeper WHERE path='/'"

	systemReplicationExistsSQL   = "SELECT count() AS replication_queue_exists FROM system.tables WHERE database='system' AND name='replication_queue'"
	systemReplicationNumTriesSQL = "SELECT countIf(num_tries>1) AS replication_num_tries_replicas, countIf(num_tries>100) " +
		"AS replication_too_many_tries_replicas FROM system.replication_queue SETTINGS empty_result_for_aggregation_by_empty_set=0"

	systemDetachedPartsSQL = "SELECT count() AS detached_parts FROM system.detached_parts SETTINGS empty_result_for_aggregation_by_empty_set=0"

	systemDictionariesSQL = "SELECT origin, status, bytes_allocated FROM system.dictionaries"

	systemMutationSQL = "SELECT countIf(latest_fail_time>toDateTime('0000-00-00 00:00:00') AND is_done=0) " +
		"AS failed, countIf(latest_fail_time=toDateTime('0000-00-00 00:00:00') AND is_done=0) " +
		"AS running, countIf(is_done=1) AS completed FROM system.mutations SETTINGS empty_result_for_aggregation_by_empty_set=0"
	systemDisksSQL = "SELECT name, path, toUInt64(100*free_space / total_space) " +
		"AS free_space_percent, toUInt64( 100 * keep_free_space / total_space) AS keep_free_space_percent FROM system.disks"
	systemProcessesSQL = "SELECT multiIf(positionCaseInsensitive(query,'select')=1,'select',positionCaseInsensitive(query,'insert')=1,'insert','other') " +
		"AS query_type, quantile\n(0.5)(elapsed) AS p50, quantile(0.9)(elapsed) AS p90, max(elapsed) AS longest_running " +
		"FROM system.processes GROUP BY query_type SETTINGS empty_result_for_aggregation_by_empty_set=0"

	systemTextLogExistsSQL = "SELECT count() AS text_log_exists FROM system.tables WHERE database='system' AND name='text_log'"
	systemTextLogSQL       = "SELECT count() AS messages_last_10_min, level FROM system.text_log " +
		"WHERE level <= 'Notice' AND event_time >= now() - INTERVAL 600 SECOND GROUP BY level SETTINGS empty_result_for_aggregation_by_empty_set=0"
)

var commonMetrics = map[string]string{
	"events":               systemEventsSQL,
	"metrics":              systemMetricsSQL,
	"asynchronous_metrics": systemAsyncMetricsSQL,
}

var commonMetricsIsFloat = map[string]bool{
	"events":               false,
	"metrics":              false,
	"asynchronous_metrics": true,
}
