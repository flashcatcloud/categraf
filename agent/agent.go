package agent

import (
	"log"

	"flashcat.cloud/categraf/traces"

	// auto registry
	_ "flashcat.cloud/categraf/inputs/conntrack"
	_ "flashcat.cloud/categraf/inputs/cpu"
	_ "flashcat.cloud/categraf/inputs/disk"
	_ "flashcat.cloud/categraf/inputs/diskio"
	_ "flashcat.cloud/categraf/inputs/dns_query"
	_ "flashcat.cloud/categraf/inputs/docker"
	_ "flashcat.cloud/categraf/inputs/elasticsearch"
	_ "flashcat.cloud/categraf/inputs/exec"
	_ "flashcat.cloud/categraf/inputs/http_response"
	_ "flashcat.cloud/categraf/inputs/ipvs"
	_ "flashcat.cloud/categraf/inputs/jenkins"
	_ "flashcat.cloud/categraf/inputs/jolokia_agent"
	_ "flashcat.cloud/categraf/inputs/jolokia_proxy"
	_ "flashcat.cloud/categraf/inputs/kafka"
	_ "flashcat.cloud/categraf/inputs/kernel"
	_ "flashcat.cloud/categraf/inputs/kernel_vmstat"
	_ "flashcat.cloud/categraf/inputs/kubernetes"
	_ "flashcat.cloud/categraf/inputs/linux_sysctl_fs"
	_ "flashcat.cloud/categraf/inputs/logstash"
	_ "flashcat.cloud/categraf/inputs/mem"
	_ "flashcat.cloud/categraf/inputs/mongodb"
	_ "flashcat.cloud/categraf/inputs/mysql"
	_ "flashcat.cloud/categraf/inputs/net"
	_ "flashcat.cloud/categraf/inputs/net_response"
	_ "flashcat.cloud/categraf/inputs/netstat"
	_ "flashcat.cloud/categraf/inputs/nfsclient"
	_ "flashcat.cloud/categraf/inputs/nginx"
	_ "flashcat.cloud/categraf/inputs/nginx_upstream_check"
	_ "flashcat.cloud/categraf/inputs/ntp"
	_ "flashcat.cloud/categraf/inputs/nvidia_smi"
	_ "flashcat.cloud/categraf/inputs/oracle"
	_ "flashcat.cloud/categraf/inputs/phpfpm"
	_ "flashcat.cloud/categraf/inputs/ping"
	_ "flashcat.cloud/categraf/inputs/processes"
	_ "flashcat.cloud/categraf/inputs/procstat"
	_ "flashcat.cloud/categraf/inputs/prometheus"
	_ "flashcat.cloud/categraf/inputs/rabbitmq"
	_ "flashcat.cloud/categraf/inputs/redis"
	_ "flashcat.cloud/categraf/inputs/redis_sentinel"
	_ "flashcat.cloud/categraf/inputs/switch_legacy"
	_ "flashcat.cloud/categraf/inputs/system"
	_ "flashcat.cloud/categraf/inputs/tomcat"
	_ "flashcat.cloud/categraf/inputs/zookeeper"
)

type Agent struct {
	InputFilters   map[string]struct{}
	InputReaders   map[string]*InputReader
	TraceCollector *traces.Collector
}

func NewAgent(filters map[string]struct{}) *Agent {
	return &Agent{
		InputFilters: filters,
		InputReaders: make(map[string]*InputReader),
	}
}

func (a *Agent) Start() {
	log.Println("I! agent starting")
	a.startLogAgent()
	err := a.startMetricsAgent()
	if err != nil {
		log.Println(err)
	}
	err = a.startTracesAgent()
	if err != nil {
		log.Println(err)
	}
	a.startPrometheusScrape()
	log.Println("I! agent started")
}

func (a *Agent) Stop() {
	log.Println("I! agent stopping")
	a.stopLogAgent()
	a.stopMetricsAgent()
	err := a.stopTracesAgent()
	if err != nil {
		log.Println(err)
	}
	a.stopPrometheusScrape()
	log.Println("I! agent stopped")
}

func (a *Agent) Reload() {
	log.Println("I! agent reloading")
	a.Stop()
	a.Start()
	log.Println("I! agent reloaded")
}
