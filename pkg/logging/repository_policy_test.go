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
		"inputs/cloudwatch/cloudwatch.go",
		"inputs/kernel/kernel.go",
		"inputs/ldap/ldap.go",
		"inputs/conntrack/conntrack.go",
		"inputs/diskio/diskio.go",
		"inputs/ethtool/command_linux.go",
		"inputs/mem/mem.go",
		"inputs/net/net.go",
		"inputs/kernel_vmstat/kernel_vmstat.go",
		"inputs/nats/nats.go",
		"inputs/nfsclient/nfsclient.go",
		"inputs/nsq/nsq.go",
		"inputs/system/ps.go",
		"inputs/ethtool/ethtool_notlinux.go",
		"inputs/ethtool/ethtool_linux.go",
		"inputs/filecount/filecount.go",
		"inputs/gnmi/gnmi.go",
		"inputs/gnmi/handler.go",
		"inputs/googlecloud/instances.go",
		"inputs/greenplum/greenplum.go",
		"inputs/jenkins/jenkins.go",
		"inputs/kafka/kafka.go",
		"inputs/provider_manager.go",
		"inputs/redis/redis.go",
		"inputs/redis_sentinel/redis_sentinel.go",
		"inputs/snmp/table.go",
		"inputs/jolokia_agent/jolokia_agent.go",
		"inputs/mongodb/mongodb.go",
		"inputs/mongodb/mongodb_server.go",
		"inputs/mysql/engine_innodb.go",
		"inputs/mysql/global_status.go",
		"inputs/mysql/global_variables.go",
		"inputs/mysql/binlog.go",
		"inputs/mysql/custom_queries.go",
		"inputs/mysql/mysql.go",
		"inputs/mysql/processlist.go",
		"inputs/mysql/processlist_by_user.go",
		"inputs/mysql/schema_size.go",
		"inputs/mysql/slave_status.go",
		"inputs/mysql/table_size.go",
		"inputs/clickhouse/clickhouse.go",
		"inputs/bind/bind.go",
		"inputs/chrony/chrony.go",
		"inputs/consul/consul.go",
		"inputs/dns_query/dns_query.go",
		"inputs/dmesg/dmesg.go",
		"inputs/docker/docker.go",
		"inputs/aliyun/cloud.go",
		"inputs/aliyun/internal/manager/cms.go",
		"inputs/amd_rocm_smi/amd_rocm_smi.go",
		"inputs/appdynamics/instances.go",
		"inputs/arp_packet/arp_packet.go",
		"inputs/cadvisor/instances.go",
		"inputs/dcgm/exporter.go",
		"inputs/emc_unity/emc_unity.go",
		"inputs/exec/exec.go",
		"inputs/hadoop/hadoop.go",
		"inputs/huatuo/huatuo.go",
		"inputs/iptables/iptables.go",
		"inputs/ipmi/instances.go",
		"inputs/ipmi/exporter/collector_bmc.go",
		"inputs/ipmi/exporter/collector_bmc_watchdog.go",
		"inputs/ipmi/exporter/collector_chassis.go",
		"inputs/ipmi/exporter/collector_dcmi.go",
		"inputs/ipmi/exporter/collector_ipmi.go",
		"inputs/ipmi/exporter/collector_notwindows.go",
		"inputs/ipmi/exporter/collector_sel.go",
		"inputs/ipmi/exporter/collector_sm_lan_mode.go",
		"inputs/ipmi/exporter/freeipmi/freeipmi.go",
		"inputs/kubernetes/kubernetes.go",
		"inputs/linux_sysctl_fs/linux_sysctl_fs_linux.go",
		"inputs/net_response/net_response.go",
		"inputs/nginx/nginx.go",
		"inputs/netstat_filter/netstat_filter.go",
		"inputs/ipvs/ipvs_linux_amd64.go",
		"inputs/ethtool/namespace_linux.go",
		"inputs/jolokia/gatherer.go",
		"inputs/jolokia_proxy/jolokia_proxy.go",
		"inputs/redfish/redfish.go",
		"inputs/tengine/tengine.go",
		"inputs/tomcat/tomcat.go",
		"inputs/ntp/ntp.go",
		"inputs/nvidia_smi/builder.go",
		"inputs/nvidia_smi/nvidia_smi.go",
		"inputs/node_exporter/exporter.go",
		"inputs/node_exporter/collector/buddyinfo.go",
		"inputs/node_exporter/collector/collector.go",
		"inputs/node_exporter/collector/cpu_linux.go",
		"inputs/node_exporter/collector/diskstats_common.go",
		"inputs/node_exporter/collector/diskstats_linux.go",
		"inputs/node_exporter/collector/netclass_rtnl_linux.go",
		"inputs/node_exporter/collector/ntp.go",
		"inputs/node_exporter/collector/perf_linux.go",
		"inputs/node_exporter/collector/qdisc_linux.go",
		"inputs/node_exporter/collector/runit.go",
		"inputs/node_exporter/collector/supervisord.go",
		"inputs/node_exporter/collector/systemd_linux.go",
		"inputs/node_exporter/collector/textfile.go",
		"inputs/oracle/oracle.go",
		"inputs/phpfpm/phpfpm.go",
		"inputs/ping/ping.go",
		"inputs/ping/ping_notwindows.go",
		"inputs/ping/ping_windows.go",
		"inputs/postgresql/postgresql.go",
		"inputs/prometheus/consul.go",
		"inputs/prometheus/prometheus.go",
		"inputs/processes/processes_notwindows.go",
		"inputs/procstat/win_service_windows.go",
		"inputs/procstat/procstat.go",
		"inputs/nginx_upstream_check/nginx_upstream_check.go",
		"inputs/netstat/netstat.go",
		"inputs/netstat_filter/netstat_tcp.go",
		"inputs/supervisor/supervisor.go",
		"inputs/rocketmq_offset/rocketmq.go",
		"inputs/rabbitmq/rabbitmq.go",
		"inputs/snmp/netsnmp.go",
		"inputs/smart/instances.go",
		"inputs/sqlserver/sqlserver.go",
		"inputs/systemd/systemd_linux.go",
		"inputs/traffic_server/traffic_server.go",
		"inputs/vsphere/finder.go",
		"inputs/vsphere/tscache.go",
		"inputs/whois/whois.go",
		"inputs/x509_cert/x509_cert.go",
		"inputs/zookeeper/zookeeper.go",
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

func TestElasticsearchTreeDoesNotUseStandardLogCalls(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	checkGoTreeForForbiddenStdLog(t, filepath.Join(repoRoot, "inputs/elasticsearch"))
}

func checkGoTreeForForbiddenStdLog(t *testing.T, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if filepath.Base(path) == "README.md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if forbiddenStdLog.Match(content) {
			t.Fatalf("%s still uses forbidden standard log calls", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
