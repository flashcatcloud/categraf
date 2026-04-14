package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var forbiddenStdLog = regexp.MustCompile(`\blog\.(Print|Println|Printf|Panic|Panicf|Panicln|Fatal|Fatalf|Fatalln)\b`)
var forbiddenDebugBranch = regexp.MustCompile(`if\s+(config\.Config\.DebugMode|Config\.DebugMode)\s*\{`)

func TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	files := []string{
		"main.go",
		"main_posix.go",
		"main_windows.go",
		"agent/agent.go",
		"agent/ibex_agent.go",
		"agent/logs_agent.go",
		"agent/metrics_agent.go",
		"agent/metrics_reader.go",
		"agent/prometheus_agent.go",
		"agent/install/service_linux.go",
		"agent/update/update_linux.go",
		"agent/update/update_windows.go",
		"api/router_falcon.go",
		"api/router_opentsdb.go",
		"api/server.go",
		"config/config.go",
		"config/hostname.go",
		"config/urllabel.go",
		"ibex/heartbeat.go",
		"ibex/task.go",
		"ibex/tasks.go",
		"ibex/client/cli.go",
		"inputs/http_provider.go",
		"inputs/collector.go",
		"inputs/haproxy/haproxy.go",
		"inputs/haproxy/exporter.go",
		"inputs/http_response/http_response.go",
		"inputs/sockstat/sockstat.go",
		"inputs/self_metrics/metrics.go",
		"inputs/system/system.go",
		"inputs/disk/disk.go",
		"inputs/cpu/cpu.go",
		"inputs/kernel/kernel.go",
		"inputs/ldap/ldap.go",
		"inputs/conntrack/conntrack.go",
		"inputs/diskio/diskio.go",
		"inputs/mem/mem.go",
		"inputs/net/net.go",
		"inputs/kernel_vmstat/kernel_vmstat.go",
		"inputs/nats/nats.go",
		"inputs/nfsclient/nfsclient.go",
		"inputs/nsq/nsq.go",
		"inputs/system/ps.go",
		"inputs/ethtool/ethtool_linux.go",
		"inputs/filecount/filecount.go",
		"inputs/gnmi/gnmi.go",
		"inputs/gnmi/handler.go",
		"inputs/greenplum/greenplum.go",
		"inputs/kafka/kafka.go",
		"inputs/provider_manager.go",
		"inputs/redis/redis.go",
		"inputs/redis_sentinel/redis_sentinel.go",
		"inputs/snmp/table.go",
		"inputs/jolokia_agent/jolokia_agent.go",
		"inputs/mongodb/mongodb.go",
		"inputs/clickhouse/clickhouse.go",
		"inputs/bind/bind.go",
		"inputs/chrony/chrony.go",
		"inputs/consul/consul.go",
		"inputs/dns_query/dns_query.go",
		"inputs/dmesg/dmesg.go",
		"inputs/hadoop/hadoop.go",
		"inputs/kubernetes/kubernetes.go",
		"inputs/net_response/net_response.go",
		"inputs/netstat_filter/netstat_filter.go",
		"inputs/jolokia/gatherer.go",
		"inputs/jolokia_proxy/jolokia_proxy.go",
		"inputs/redfish/redfish.go",
		"inputs/tengine/tengine.go",
		"inputs/tomcat/tomcat.go",
		"inputs/ntp/ntp.go",
		"inputs/prometheus/consul.go",
		"inputs/prometheus/prometheus.go",
		"inputs/nginx_upstream_check/nginx_upstream_check.go",
		"inputs/netstat/netstat.go",
		"inputs/supervisor/supervisor.go",
		"inputs/rocketmq_offset/rocketmq.go",
		"inputs/traffic_server/traffic_server.go",
		"inputs/whois/whois.go",
		"inputs/logstash/logstash.go",
		"parser/influx/parser.go",
		"parser/prometheus/parser.go",
		"pkg/aop/logger.go",
		"pkg/httpx/client.go",
		"pkg/httpx/transport.go",
		"pkg/kubernetes/pod.go",
		"pkg/pprof/profile.go",
		"pkg/snmp/translate.go",
		"writer/writer.go",
		"writer/writers.go",
		"heartbeat/heartbeat.go",
	}

	for _, rel := range files {
		path := filepath.Join(repoRoot, rel)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if forbiddenStdLog.Match(content) {
			t.Fatalf("%s still uses forbidden standard log calls", path)
		}
		if forbiddenDebugBranch.Match(content) {
			t.Fatalf("%s still contains forbidden debug branch", path)
		}
	}
}

func TestLoggingTestsDoNotUseDirectStandardLogCalls(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	path := filepath.Join(repoRoot, "pkg/logging/logging_test.go")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if forbiddenStdLog.Match(content) {
		t.Fatalf("%s still uses forbidden standard log calls", path)
	}
}
