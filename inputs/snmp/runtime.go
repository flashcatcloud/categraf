package snmp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

type collectionStats struct {
	operationsAttempted   int
	requestsSent          int
	rawResponses          int
	responsesObserved     int
	transportFailures     int
	operationResults      map[operationStatsKey]int
	requestsByOp          map[string]int
	rawResponsesByOp      map[string]int
	responsesByOp         map[string]int
	transportFailuresByOp map[string]int
	successfulFields      int
	failedFields          int
	partialTables         int
	skippedRows           int
	cacheEvents           map[string]int
}

type operationStatsKey struct {
	operation string
	result    string
}

type snmpConnectionStats struct {
	mu       sync.Mutex
	sent     int
	recv     int
	finishes int
}

func (s *snmpConnectionStats) recordSent() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.sent++
	s.mu.Unlock()
}

func (s *snmpConnectionStats) recordRecv() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.recv++
	s.mu.Unlock()
}

func (s *snmpConnectionStats) recordFinish() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.finishes++
	s.mu.Unlock()
}

type snmpConnectionStatsSnapshot struct {
	sent     int
	recv     int
	finishes int
}

func (s *snmpConnectionStats) snapshot() snmpConnectionStatsSnapshot {
	if s == nil {
		return snmpConnectionStatsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return snmpConnectionStatsSnapshot{
		sent:     s.sent,
		recv:     s.recv,
		finishes: s.finishes,
	}
}

type agentRuntime struct {
	mu sync.Mutex

	healthy          bool
	consecutiveFails int
	lastSuccess      time.Time
	lastSeen         time.Time
	nextProbeAt      time.Time
	probeInFlight    bool
	closed           bool
	gatherStats      collectionStats

	requestMu sync.Mutex
	requestWG sync.WaitGroup

	connectionMu sync.Mutex
	connection   snmpConnection
	stats        snmpConnectionStats

	probeMu         sync.Mutex
	probeConnection snmpConnection

	dependencyCache *dependencyCache
	metricCounters  map[string]float64
}

func newAgentRuntime(now time.Time) *agentRuntime {
	return &agentRuntime{
		healthy:         true,
		lastSeen:        now,
		dependencyCache: newDependencyCache(0),
	}
}

func (rt *agentRuntime) configureDependencyCache(ttl time.Duration, maxEntries int) {
	if rt == nil {
		return
	}
	if rt.dependencyCache == nil {
		rt.dependencyCache = newDependencyCache(ttl)
	} else {
		rt.dependencyCache.setTTL(ttl)
	}
	rt.dependencyCache.setMaxEntries(maxEntries)
}

func (rt *agentRuntime) resetGatherStats() {
	rt.mu.Lock()
	rt.gatherStats = collectionStats{}
	rt.mu.Unlock()
}

func (rt *agentRuntime) recordOperation(operation string, requestsSent, rawResponses, responsesObserved int, err error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.gatherStats.operationsAttempted++
	rt.gatherStats.requestsSent += requestsSent
	rt.gatherStats.rawResponses += rawResponses
	rt.gatherStats.responsesObserved += responsesObserved
	if err != nil && responsesObserved == 0 && isTransportFailure(err) {
		rt.gatherStats.transportFailures++
	}

	result := classifyOperationResult(err, responsesObserved)
	key := operationStatsKey{operation: operation, result: result}
	if rt.gatherStats.operationResults == nil {
		rt.gatherStats.operationResults = map[operationStatsKey]int{}
	}
	rt.gatherStats.operationResults[key]++
	if requestsSent > 0 {
		if rt.gatherStats.requestsByOp == nil {
			rt.gatherStats.requestsByOp = map[string]int{}
		}
		rt.gatherStats.requestsByOp[operation] += requestsSent
	}
	if rawResponses > 0 {
		if rt.gatherStats.rawResponsesByOp == nil {
			rt.gatherStats.rawResponsesByOp = map[string]int{}
		}
		rt.gatherStats.rawResponsesByOp[operation] += rawResponses
	}
	if responsesObserved > 0 {
		if rt.gatherStats.responsesByOp == nil {
			rt.gatherStats.responsesByOp = map[string]int{}
		}
		rt.gatherStats.responsesByOp[operation] += responsesObserved
	}
	if err != nil && responsesObserved == 0 && isTransportFailure(err) {
		if rt.gatherStats.transportFailuresByOp == nil {
			rt.gatherStats.transportFailuresByOp = map[string]int{}
		}
		rt.gatherStats.transportFailuresByOp[operation]++
	}
}

func classifyOperationResult(err error, responsesObserved int) string {
	if err == nil {
		return "success"
	}
	if isFatalSNMPError(err) {
		return "fatal"
	}
	if isPermanentSocketError(err) {
		return "permanent_socket"
	}
	if isTransportFailure(err) && responsesObserved == 0 {
		return "timeout"
	}
	return "error"
}

func (rt *agentRuntime) recordBuildStats(stats BuildStats) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.gatherStats.successfulFields += stats.SuccessfulFields
	rt.gatherStats.failedFields += stats.FailedFields
	if stats.isPartialResult() {
		rt.gatherStats.partialTables++
	}
	for _, n := range stats.SkippedRows {
		rt.gatherStats.skippedRows += n
	}
	if len(stats.CacheEvents) > 0 && rt.gatherStats.cacheEvents == nil {
		rt.gatherStats.cacheEvents = map[string]int{}
	}
	for _, event := range stats.CacheEvents {
		rt.gatherStats.cacheEvents[event.Type+":"+event.Result]++
	}
}

func (rt *agentRuntime) addMetricCounter(metric string, tags map[string]string, delta int) float64 {
	if delta == 0 {
		rt.mu.Lock()
		if rt.metricCounters == nil {
			rt.metricCounters = map[string]float64{}
		}
		value := rt.metricCounters[metricCounterKey(metric, tags)]
		rt.mu.Unlock()
		return value
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.metricCounters == nil {
		rt.metricCounters = map[string]float64{}
	}
	key := metricCounterKey(metric, tags)
	rt.metricCounters[key] += float64(delta)
	return rt.metricCounters[key]
}

func metricCounterKey(metric string, tags map[string]string) string {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(metric)
	for _, k := range keys {
		b.WriteByte('|')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(tags[k])
	}
	return b.String()
}

func isTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "no route to host") ||
		strings.Contains(msg, "use of closed network connection")
}

func isPermanentSocketError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "broken pipe")
}

func (rt *agentRuntime) recordConnectFailure(maxFailCount int, recoveryInterval time.Duration) {
	rt.mu.Lock()
	becameUnhealthy := rt.recordFailureLocked(maxFailCount, recoveryInterval)
	rt.mu.Unlock()
	if becameUnhealthy {
		rt.closeDetachedConnection()
	}
}

func (rt *agentRuntime) completeGather(maxFailCount int, recoveryInterval time.Duration) collectionStats {
	rt.mu.Lock()

	stats := rt.gatherStats
	rt.gatherStats = collectionStats{}

	if stats.operationsAttempted == 0 {
		rt.mu.Unlock()
		return stats
	}
	if stats.responsesObserved > 0 {
		rt.recordSuccessLocked()
		rt.mu.Unlock()
		return stats
	}
	becameUnhealthy := false
	if stats.transportFailures > 0 {
		becameUnhealthy = rt.recordFailureLocked(maxFailCount, recoveryInterval)
	}
	rt.mu.Unlock()
	if becameUnhealthy {
		rt.closeDetachedConnection()
	}
	return stats
}

func (rt *agentRuntime) isHealthy() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.healthy && !rt.closed
}

func (rt *agentRuntime) recordSuccessLocked() {
	rt.healthy = true
	rt.consecutiveFails = 0
	now := time.Now()
	rt.lastSuccess = now
	rt.lastSeen = now
	rt.nextProbeAt = time.Time{}
}

func (rt *agentRuntime) recordFailureLocked(maxFailCount int, recoveryInterval time.Duration) bool {
	wasHealthy := rt.healthy
	rt.consecutiveFails++
	rt.lastSeen = time.Now()
	if rt.consecutiveFails >= maxFailCount {
		rt.healthy = false
		rt.nextProbeAt = time.Now().Add(recoveryInterval)
	}
	return wasHealthy && !rt.healthy
}

func (rt *agentRuntime) closeDetachedConnection() {
	conn := rt.detachConnection()
	if conn != nil {
		_ = conn.Close()
	}
}

func (rt *agentRuntime) beginRequest() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return false
	}
	rt.requestWG.Add(1)
	return true
}

func (rt *agentRuntime) endRequest() {
	rt.requestWG.Done()
}

func (rt *agentRuntime) shouldCollect(now time.Time) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return !rt.closed && rt.healthy
}

func (rt *agentRuntime) beginRecovery(now time.Time) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.closed || rt.healthy || rt.probeInFlight {
		return false
	}
	if !rt.nextProbeAt.IsZero() && now.Before(rt.nextProbeAt) {
		return false
	}
	rt.probeInFlight = true
	return true
}

func (rt *agentRuntime) recoveryDue(now time.Time) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return !rt.closed && !rt.healthy && !rt.probeInFlight && (rt.nextProbeAt.IsZero() || !now.Before(rt.nextProbeAt))
}

func (rt *agentRuntime) finishRecovery(success bool, recoveryInterval time.Duration) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.closed {
		return
	}
	rt.probeInFlight = false
	rt.lastSeen = time.Now()
	if success {
		rt.recordSuccessLocked()
		return
	}
	rt.healthy = false
	rt.nextProbeAt = time.Now().Add(recoveryInterval)
}

func (rt *agentRuntime) close() {
	rt.mu.Lock()
	rt.closed = true
	rt.probeInFlight = false
	rt.mu.Unlock()

	if rt.dependencyCache != nil {
		rt.dependencyCache.disable()
	}

	conn := rt.detachConnection()
	if conn != nil {
		_ = conn.Close()
	}
	rt.closeProbeConnection()
	rt.requestWG.Wait()
}

func (rt *agentRuntime) storeProbeConnection(conn snmpConnection) bool {
	rt.mu.Lock()
	closed := rt.closed
	rt.mu.Unlock()
	if closed {
		if conn != nil {
			_ = conn.Close()
		}
		return false
	}

	rt.probeMu.Lock()
	defer rt.probeMu.Unlock()
	rt.mu.Lock()
	closed = rt.closed
	rt.mu.Unlock()
	if closed {
		if conn != nil {
			_ = conn.Close()
		}
		return false
	}
	rt.probeConnection = conn
	return true
}

func (rt *agentRuntime) clearProbeConnection(conn snmpConnection) {
	rt.probeMu.Lock()
	defer rt.probeMu.Unlock()
	if rt.probeConnection == conn {
		rt.probeConnection = nil
	}
}

func (rt *agentRuntime) closeProbeConnection() {
	rt.probeMu.Lock()
	conn := rt.probeConnection
	rt.probeConnection = nil
	rt.probeMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (rt *agentRuntime) detachConnection() snmpConnection {
	rt.connectionMu.Lock()
	defer rt.connectionMu.Unlock()

	conn := rt.connection
	rt.connection = nil
	return conn
}

func (rt *agentRuntime) cachedConnection() snmpConnection {
	rt.connectionMu.Lock()
	defer rt.connectionMu.Unlock()
	return rt.connection
}

func (rt *agentRuntime) storeConnection(conn snmpConnection) error {
	rt.connectionMu.Lock()
	defer rt.connectionMu.Unlock()

	rt.mu.Lock()
	closed := rt.closed
	rt.mu.Unlock()
	if closed {
		_ = conn.Close()
		return fmt.Errorf("agent runtime is closed")
	}

	rt.connection = conn
	return nil
}

type runtimeConnection struct {
	rt   *agentRuntime
	conn snmpConnection
}

func (c *runtimeConnection) Host() string {
	return c.conn.Host()
}

func (c *runtimeConnection) Walk(oid string, fn gosnmp.WalkFunc) error {
	if !c.rt.beginRequest() {
		return net.ErrClosed
	}
	defer c.rt.endRequest()

	c.rt.requestMu.Lock()
	defer c.rt.requestMu.Unlock()

	before := c.rt.stats.snapshot()
	err := c.conn.Walk(oid, func(pdu gosnmp.SnmpPDU) error {
		return fn(pdu)
	})
	after := c.rt.stats.snapshot()
	c.rt.recordOperation("walk", after.sent-before.sent, after.recv-before.recv, after.finishes-before.finishes, err)
	if isPermanentSocketError(err) {
		c.rt.closeDetachedConnection()
	}
	return err
}

func (c *runtimeConnection) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	if !c.rt.beginRequest() {
		return nil, net.ErrClosed
	}
	defer c.rt.endRequest()

	c.rt.requestMu.Lock()
	defer c.rt.requestMu.Unlock()

	before := c.rt.stats.snapshot()
	pkt, err := c.conn.Get(oids)
	after := c.rt.stats.snapshot()
	responsesObserved := after.finishes - before.finishes
	if pkt != nil && responsesObserved == 0 {
		responsesObserved = 1
	}
	c.rt.recordOperation("get", after.sent-before.sent, after.recv-before.recv, responsesObserved, err)
	if isPermanentSocketError(err) {
		c.rt.closeDetachedConnection()
	}
	return pkt, err
}

func (c *runtimeConnection) Close() error {
	return c.conn.Close()
}
