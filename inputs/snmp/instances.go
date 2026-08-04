package snmp

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/freedomkk-qfeng/go-fastping"
	"github.com/gosnmp/gosnmp"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

type Instance struct {
	config.InstanceConfig
	// The SNMP agent to query. Format is [SCHEME://]ADDR[:PORT] (e.g.
	// udp://1.2.3.4:161).  If the scheme is not specified then "udp" is used.
	Agents []string `toml:"agents"`

	// The tag used to name the agent host
	AgentHostTag string `toml:"agent_host_tag"`

	ClientConfig

	Tables []Table `toml:"table"`

	DefaultTableErrorPolicy   string          `toml:"default_table_error_policy"`
	DependencyCacheTTL        config.Duration `toml:"dependency_cache_ttl"`
	DependencyCacheMaxEntries int             `toml:"dependency_cache_max_entries"`

	// Name & Fields are the elements of a Table.
	// Categraf chokes if we try to embed a Table. So instead we have to embed the
	// fields of a Table, and construct a Table during runtime.
	Name   string  `toml:"name"`
	Fields []Field `toml:"field"`

	DisableUp     bool `toml:"disable_up"`
	DisableSnmpUp bool `toml:"disable_snmp_up"`
	DisableICMPUp bool `toml:"disable_icmp_up"`

	agentRuntimes []*agentRuntime

	Translator string `toml:"translator"`

	translator Translator

	Mappings map[string]map[string]string `toml:"mappings"`

	healthMonitorStartOnce sync.Once

	// Configuration for health monitoring
	HealthCheckInterval config.Duration `toml:"health_check_interval"`
	HealthCheckTimeout  config.Duration `toml:"health_check_timeout"`
	MaxFailCount        int             `toml:"max_fail_count"`
	RecoveryInterval    config.Duration `toml:"recovery_interval"`

	stop chan struct{}
}

func (ins *Instance) Init() error {

	if len(ins.Agents) == 0 {
		return types.ErrInstancesEmpty
	}

	var err error
	switch ins.Translator {
	case "gosmi":
		ins.translator, err = NewGosmiTranslator(ins.Path)
		if err != nil {
			return err
		}
		ins.translator.SetDebugMode(ins.DebugMod)
	case "", "netsnmp":
		ins.translator = NewNetsnmpTranslator()
		ins.translator.SetDebugMode(ins.DebugMod)
	default:
		return fmt.Errorf("invalid translator value")
	}

	now := time.Now()
	ins.agentRuntimes = make([]*agentRuntime, len(ins.Agents))
	for i := range ins.Agents {
		ins.agentRuntimes[i] = newAgentRuntime(now)
	}

	for i := range ins.Tables {
		if err := ins.Tables[i].Init(ins.translator); err != nil {
			return fmt.Errorf("initializing table %s ins: %s", ins.Tables[i].Name, err)
		}
	}

	for i := range ins.Fields {
		if err := ins.Fields[i].init(ins.translator); err != nil {
			return fmt.Errorf("initializing field %s ins: %w", ins.Fields[i].Name, err)
		}
	}

	if len(ins.AgentHostTag) == 0 {
		ins.AgentHostTag = "agent_host"
	}
	if ins.HealthCheckInterval == 0 {
		ins.HealthCheckInterval = config.Duration(60 * time.Second)
	}
	if ins.HealthCheckTimeout == 0 {
		ins.HealthCheckTimeout = config.Duration(5 * time.Second)
	}
	if ins.MaxFailCount == 0 {
		ins.MaxFailCount = 3
	}
	if ins.RecoveryInterval == 0 {
		ins.RecoveryInterval = config.Duration(5 * time.Minute)
	}
	if ins.DefaultTableErrorPolicy == "" {
		ins.DefaultTableErrorPolicy = ErrorPolicyLegacy
	}
	if ins.DefaultTableErrorPolicy != ErrorPolicyLegacy && ins.DefaultTableErrorPolicy != ErrorPolicyPartial {
		return fmt.Errorf("invalid default_table_error_policy %q", ins.DefaultTableErrorPolicy)
	}
	if ins.DependencyCacheTTL < 0 {
		return fmt.Errorf("dependency_cache_ttl cannot be negative")
	}
	if ins.DependencyCacheMaxEntries < 0 {
		return fmt.Errorf("dependency_cache_max_entries cannot be negative")
	}
	for i := range ins.Tables {
		if ins.Tables[i].ErrorPolicy == "" {
			ins.Tables[i].ErrorPolicy = ins.DefaultTableErrorPolicy
		}
		if ins.Tables[i].ErrorPolicy != ErrorPolicyLegacy && ins.Tables[i].ErrorPolicy != ErrorPolicyPartial {
			return fmt.Errorf("invalid error_policy %q for table %s", ins.Tables[i].ErrorPolicy, ins.Tables[i].Name)
		}
	}
	for _, rt := range ins.agentRuntimes {
		rt.configureDependencyCache(time.Duration(ins.DependencyCacheTTL), ins.DependencyCacheMaxEntries)
	}

	ins.stop = make(chan struct{})

	return nil
}

func (ins *Instance) up(slist *types.SampleList, i int, topTags map[string]string, snmpUpOverride *float64) {
	if ins.DisableUp {
		return
	}

	target := ins.Agents[i]
	if !strings.Contains(target, "://") {
		target = "udp://" + target
	}
	var host string
	u, err := url.Parse(target)
	if err == nil {
		host = u.Hostname()
	}

	etags := map[string]string{}
	for k, v := range topTags {
		etags[k] = v
	}
	if m, ok := ins.Mappings[target]; ok {
		for k, v := range m {
			etags[k] = v
		}
	}
	etags[ins.AgentHostTag] = host

	// icmp probe
	if !ins.DisableICMPUp {
		up, rtt, loss := Ping(host, 250)
		slist.PushSample(inputName, "icmp_up", up, etags)
		slist.PushSample(inputName, "icmp_rtt", rtt, etags)
		slist.PushSample(inputName, "icmp_packet_loss", loss, etags)
	}

	// snmp probe
	if ins.DisableSnmpUp {
		return
	}
	if snmpUpOverride != nil {
		slist.PushSample(inputName, "up", *snmpUpOverride, etags)
		return
	}
	oid := ".1.3.6.1.2.1.1.1.0"
	gs, err := ins.getConnection(i)
	if err != nil {
		slist.PushSample(inputName, "up", 0, etags)
		return
	}
	_, err = gs.Get([]string{oid})
	if err != nil {
		if strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "reset") {
			slist.PushSample(inputName, "up", 0, etags)
			return
		}
	}
	slist.PushSample(inputName, "up", 1, etags)
}

// Gather retrieves all the configured fields and tables.
// Any error encountered does not halt the process. The errors are accumulated
// and returned at the end.
func (ins *Instance) Gather(slist *types.SampleList) {
	insLabels := ins.GetLabels()

	var wg sync.WaitGroup
	for i, agent := range ins.Agents {
		wg.Add(1)
		go func(i int, agent string) {
			defer wg.Done()
			// First is the top-level fields. We treat the fields as table prefixes with an empty index.
			t := Table{
				Name:        ins.Name,
				Fields:      ins.Fields,
				ErrorPolicy: ins.DefaultTableErrorPolicy,

				DebugMode: ins.DebugMod,
			}
			for idx, f := range t.Fields {
				t.Fields[idx].Oid = strings.TrimSpace(f.Oid)
			}
			topTags := map[string]string{}
			extraTags := map[string]string{}
			for k, v := range insLabels {
				topTags[k] = v
				extraTags[k] = v
			}
			if m, ok := ins.Mappings[agent]; ok {
				for k, v := range m {
					extraTags[k] = v
				}
			}

			rt := ins.runtime(i)
			rt.resetGatherStats()
			gatherStarted := time.Now()
			defer func() {
				stats := rt.completeGather(ins.MaxFailCount, time.Duration(ins.RecoveryInterval))
				ins.pushCollectionStats(slist, rt, agent, stats, time.Since(gatherStarted))
			}()
			collect, snmpUpOverride := ins.prepareAgentForGather(slist, i, agent, topTags)
			if !collect {
				return
			}

			gs, err := ins.getConnection(i)
			if err != nil {
				log.Printf("agent %s get connection error: %s", agent, err)
				return
			}

			if !ins.DisableUp {
				ins.up(slist, i, topTags, snmpUpOverride)
			}

			if err := ins.gatherTable(slist, gs, rt, t, -1, topTags, extraTags, false); err != nil {
				log.Printf("agent %s ins: %s", agent, err)
				if isFatalSNMPError(err) || isPermanentSocketError(err) {
					return
				}
			}

			// Now is the real tables.
			for tableIndex, t := range ins.Tables {
				if err := ins.gatherTable(slist, gs, rt, t, tableIndex, topTags, extraTags, true); err != nil {
					log.Printf("agent %s ins: gathering table %s error: %s", agent, t.Name, err)
					if isFatalSNMPError(err) || isPermanentSocketError(err) {
						return
					}
				}
			}
		}(i, agent)
	}
	wg.Wait()
}

func (ins *Instance) gatherTable(slist *types.SampleList, gs snmpConnection, rt *agentRuntime, t Table, tableIndex int, topTags, extraTags map[string]string, walk bool) error {
	plan := t.dependencyPlanForBuild()
	if walk && t.ErrorPolicy == ErrorPolicyPartial {
		for _, k := range plan.InheritTags {
			if _, ok := topTags[k]; !ok {
				log.Printf("skip table %s because inherited tag %s is unknown", t.Name, k)
				return nil
			}
		}
	}

	buildResult, err := t.BuildResultWithCache(gs, walk, ins.translator, tableBuildOptions{
		dependencyCache: rt.dependencyCache,
		tableID:         t.cacheIDWithIndex(walk, tableIndex),
		tableIndex:      tableIndex,
		topLevel:        !walk,
		now:             time.Now(),
	})
	if buildResult != nil {
		rt.recordBuildStats(buildResult.Stats)
		ins.logBuildSummary(gs.Host(), buildResult.Stats)
		ins.pushBuildStats(slist, rt, gs.Host(), buildResult.Stats)
	}
	if err != nil {
		return err
	}
	resultTable := buildResult.Table
	if !walk {
		for k, v := range buildResult.Dependencies.TopLevelTags {
			topTags[k] = v
		}
	}

	prefix := inputName
	if len(resultTable.Name) != 0 {
		prefix = inputName + "_" + resultTable.Name
	}
	for _, tr := range resultTable.Rows {
		if !walk {
			// top-level table. Add tags to topTags.
			for k, v := range tr.Tags {
				topTags[k] = v
			}
		} else {
			// real table. Inherit any specified tags.
			for _, k := range plan.InheritTags {
				if v, ok := topTags[k]; ok {
					tr.Tags[k] = v
				}
			}
		}
		if _, ok := tr.Tags[ins.AgentHostTag]; !ok {
			tr.Tags[ins.AgentHostTag] = gs.Host()
		}
		for k, v := range extraTags {
			tr.Tags[k] = v
		}
		slist.PushSamples(prefix, tr.Fields, tr.Tags)
	}

	return nil
}

func (ins *Instance) logBuildSummary(agent string, stats BuildStats) {
	if stats.FatalClass != "" {
		log.Printf("agent=%s table=%s result=fatal fatal_class=%s successful_fields=%d failed_fields=%d",
			agent, stats.TableName, stats.FatalClass, stats.SuccessfulFields, stats.FailedFields)
		return
	}
	if !stats.isPartialResult() {
		return
	}
	log.Printf("agent=%s table=%s result=partial successful_fields=%d failed_fields=%d skipped_rows=%d cache_events=%d",
		agent, stats.TableName, stats.SuccessfulFields, stats.FailedFields, totalSkippedRows(stats), len(stats.CacheEvents))
}

func (ins *Instance) pushBuildStats(slist *types.SampleList, rt *agentRuntime, agent string, stats BuildStats) {
	baseTags := map[string]string{
		"agent": agent,
		"table": stats.TableName,
	}
	deltas := map[string]counterDelta{}
	if stats.FatalClass != "" {
		tags := cloneStringMap(baseTags)
		tags["fatal_class"] = stats.FatalClass
		addCounterDelta(deltas, "fatal_table_total", 1, tags)
	}
	if stats.isPartialResult() {
		addCounterDelta(deltas, "partial_table_total", 1, baseTags)
	}
	if stats.SuccessfulFields > 0 {
		addCounterDelta(deltas, "field_success_total", stats.SuccessfulFields, baseTags)
	}
	for _, fieldErr := range stats.FieldErrors {
		tags := cloneStringMap(baseTags)
		tags["operation"] = fieldErr.Operation
		tags["reason"] = fieldErr.Reason
		if fieldErr.Dependency {
			tags["dependency"] = "true"
		} else {
			tags["dependency"] = "false"
		}
		if fieldErr.Fatal {
			tags["fatal"] = "true"
			if stats.FatalClass != "" {
				tags["fatal_class"] = stats.FatalClass
			}
		} else {
			tags["fatal"] = "false"
		}
		addCounterDelta(deltas, "field_error_total", 1, tags)
	}
	for _, event := range stats.CacheEvents {
		tags := cloneStringMap(baseTags)
		tags["type"] = event.Type
		tags["result"] = event.Result
		addCounterDelta(deltas, "dependency_cache_total", 1, tags)
	}
	for reason, count := range stats.SkippedRows {
		if count == 0 {
			continue
		}
		tags := cloneStringMap(baseTags)
		tags["reason"] = reason
		addCounterDelta(deltas, "dependency_skipped_rows_total", count, tags)
	}
	for _, delta := range deltas {
		slist.PushSample(inputName, delta.metric, rt.addMetricCounter(delta.metric, delta.tags, delta.value), delta.tags)
	}
}

type counterDelta struct {
	metric string
	tags   map[string]string
	value  int
}

func addCounterDelta(deltas map[string]counterDelta, metric string, value int, tags map[string]string) {
	if value == 0 {
		return
	}
	storedTags := cloneStringMap(tags)
	key := metricCounterKey(metric, storedTags)
	delta := deltas[key]
	if delta.metric == "" {
		delta.metric = metric
		delta.tags = storedTags
	}
	delta.value += value
	deltas[key] = delta
}

func totalSkippedRows(stats BuildStats) int {
	total := 0
	for _, count := range stats.SkippedRows {
		total += count
	}
	return total
}

func (ins *Instance) pushCollectionStats(slist *types.SampleList, rt *agentRuntime, agent string, stats collectionStats, duration time.Duration) {
	tags := map[string]string{"agent": agent}
	for key, count := range stats.operationResults {
		if count == 0 {
			continue
		}
		metricTags := cloneStringMap(tags)
		metricTags["operation"] = key.operation
		metricTags["result"] = key.result
		slist.PushSample(inputName, "operation_total", rt.addMetricCounter("operation_total", metricTags, count), metricTags)
	}
	for operation, count := range stats.requestsByOp {
		if count == 0 {
			continue
		}
		metricTags := cloneStringMap(tags)
		metricTags["operation"] = operation
		slist.PushSample(inputName, "request_total", rt.addMetricCounter("request_total", metricTags, count), metricTags)
	}
	for operation, count := range stats.rawResponsesByOp {
		if count == 0 {
			continue
		}
		metricTags := cloneStringMap(tags)
		metricTags["operation"] = operation
		slist.PushSample(inputName, "raw_response_total", rt.addMetricCounter("raw_response_total", metricTags, count), metricTags)
	}
	for operation, count := range stats.responsesByOp {
		if count == 0 {
			continue
		}
		metricTags := cloneStringMap(tags)
		metricTags["operation"] = operation
		slist.PushSample(inputName, "response_observed_total", rt.addMetricCounter("response_observed_total", metricTags, count), metricTags)
	}
	for operation, count := range stats.transportFailuresByOp {
		if count == 0 {
			continue
		}
		metricTags := cloneStringMap(tags)
		metricTags["operation"] = operation
		slist.PushSample(inputName, "transport_failure_total", rt.addMetricCounter("transport_failure_total", metricTags, count), metricTags)
	}
	health := 0
	if rt.isHealthy() {
		health = 1
	}
	cacheStats := rt.dependencyCache.stats()
	slist.PushSample(inputName, "health_state", health, tags)
	slist.PushSample(inputName, "gather_duration_seconds", duration.Seconds(), tags)
	slist.PushSample(inputName, "dependency_cache_entries", cacheStats.entries, tags)
	slist.PushSample(inputName, "dependency_cache_eviction_total", cacheStats.evictions, tags)
}

func (ins *Instance) pushRecoveryProbeStats(slist *types.SampleList, rt *agentRuntime, agent, result string) {
	tags := map[string]string{
		"agent":  agent,
		"result": result,
	}
	slist.PushSample(inputName, "recovery_probe_total", rt.addMetricCounter("recovery_probe_total", tags, 1), tags)
}

// snmpConnection is an interface which wraps a *gosnmp.GoSNMP object.
// We interact through an interface, so we can mock it out in tests.
type snmpConnection interface {
	Host() string

	// BulkWalkAll(string) ([]gosnmp.SnmpPDU, error)

	Walk(string, gosnmp.WalkFunc) error
	Get(oids []string) (*gosnmp.SnmpPacket, error)
	Close() error
}

// getConnection creates a snmpConnection (*gosnmp.GoSNMP) object and caches the
// result using `agentIndex` as the cache key.  This is done to allow multiple
// connections to a single address.  It is an error to use a connection in
// more than one goroutine.
func (ins *Instance) getConnection(idx int) (snmpConnection, error) {
	rt := ins.runtime(idx)
	agent := ins.Agents[idx]
	if !rt.shouldCollect(time.Now()) {
		return nil, fmt.Errorf("agent %s is marked as unhealthy", agent)
	}

	rt.requestMu.Lock()
	defer rt.requestMu.Unlock()

	if gs := rt.cachedConnection(); gs != nil {
		return &runtimeConnection{rt: rt, conn: gs}, nil
	}

	var err error
	var gs GosnmpWrapper
	gs, err = NewWrapper(ins.ClientConfig)
	if err != nil {
		return nil, err
	}
	gs.setStats(&rt.stats)

	err = gs.SetAgent(agent)
	if err != nil {
		return nil, err
	}

	if err := gs.Connect(); err != nil {
		_ = gs.Close()
		rt.recordConnectFailure(ins.MaxFailCount, time.Duration(ins.RecoveryInterval))
		return nil, fmt.Errorf("setting up connection: %w", err)
	}

	if err := rt.storeConnection(gs); err != nil {
		return nil, err
	}

	return &runtimeConnection{rt: rt, conn: gs}, nil
}

func Ping(ip string, timeout int) (up, rttAvg, loss float64) {
	var (
		total = 4
		lost  = 0
	)
	for i := 0; i < total; i++ {
		rtt, err := fastPingRtt(ip, timeout)
		if err != nil {
			lost++
			log.Printf("W! snmp ping %s error:%s", ip, err)
			continue
		}
		if rtt == -1 {
			lost++
			continue
		}
		rttAvg += rtt
	}
	if total == lost {
		rttAvg = -1
		up = 0
		loss = 100
	} else {
		rttAvg = rttAvg / float64(total-lost)
		up = 1
		loss = float64(lost) / float64(total)
	}
	return
}

func fastPingRtt(ip string, timeout int) (float64, error) {
	var rt float64
	rt = -1
	p := fastping.NewPinger()

	host := strings.TrimRight(strings.TrimLeft(ip, "["), "]")
	ra, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return -1, err
	}
	p.AddIPAddr(ra)
	p.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		rt = float64(rtt.Microseconds())
	}
	p.OnIdle = func() {
	}
	p.MaxRTT = time.Millisecond * time.Duration(timeout)
	err = p.Run()
	if err != nil {
		return -1, err
	}

	return rt, err
}

func (ins *Instance) Drop() {
	close(ins.stop)
	for _, rt := range ins.agentRuntimes {
		if rt != nil {
			rt.close()
		}
	}
}
