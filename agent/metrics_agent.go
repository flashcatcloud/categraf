package agent

import (
	"errors"
	"log"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"

	// auto registry
	_ "flashcat.cloud/categraf/inputs/arp_packet"
	_ "flashcat.cloud/categraf/inputs/conntrack"
	_ "flashcat.cloud/categraf/inputs/cpu"
	_ "flashcat.cloud/categraf/inputs/disk"
	_ "flashcat.cloud/categraf/inputs/diskio"
	_ "flashcat.cloud/categraf/inputs/dns_query"
	_ "flashcat.cloud/categraf/inputs/docker"
	_ "flashcat.cloud/categraf/inputs/elasticsearch"
	_ "flashcat.cloud/categraf/inputs/exec"
	_ "flashcat.cloud/categraf/inputs/greenplum"
	_ "flashcat.cloud/categraf/inputs/haproxy"
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
	_ "flashcat.cloud/categraf/inputs/mtail"
	_ "flashcat.cloud/categraf/inputs/mysql"
	_ "flashcat.cloud/categraf/inputs/net"
	_ "flashcat.cloud/categraf/inputs/net_response"
	_ "flashcat.cloud/categraf/inputs/netstat"
	_ "flashcat.cloud/categraf/inputs/netstat_filter"
	_ "flashcat.cloud/categraf/inputs/nfsclient"
	_ "flashcat.cloud/categraf/inputs/nginx"
	_ "flashcat.cloud/categraf/inputs/nginx_upstream_check"
	_ "flashcat.cloud/categraf/inputs/ntp"
	_ "flashcat.cloud/categraf/inputs/nvidia_smi"
	_ "flashcat.cloud/categraf/inputs/oracle"
	_ "flashcat.cloud/categraf/inputs/phpfpm"
	_ "flashcat.cloud/categraf/inputs/ping"
	_ "flashcat.cloud/categraf/inputs/postgresql"
	_ "flashcat.cloud/categraf/inputs/processes"
	_ "flashcat.cloud/categraf/inputs/procstat"
	_ "flashcat.cloud/categraf/inputs/prometheus"
	_ "flashcat.cloud/categraf/inputs/rabbitmq"
	_ "flashcat.cloud/categraf/inputs/redis"
	_ "flashcat.cloud/categraf/inputs/redis_sentinel"
	_ "flashcat.cloud/categraf/inputs/rocketmq_offset"
	_ "flashcat.cloud/categraf/inputs/snmp"
	_ "flashcat.cloud/categraf/inputs/sqlserver"
	_ "flashcat.cloud/categraf/inputs/switch_legacy"
	_ "flashcat.cloud/categraf/inputs/system"
	_ "flashcat.cloud/categraf/inputs/tomcat"
	_ "flashcat.cloud/categraf/inputs/vsphere"
	_ "flashcat.cloud/categraf/inputs/zookeeper"
)

type MetricsAgent struct {
	InputFilters  map[string]struct{}
	InputReaders  map[string]*InputReader
	InputProvider inputs.Provider
}

func NewMetricsAgent() AgentModule {
	c := config.Config
	agent := &MetricsAgent{
		InputFilters: parseFilter(c.InputFilters),
		InputReaders: make(map[string]*InputReader),
	}

	provider, err := inputs.NewProvider(c, agent)
	if err != nil {
		log.Println("E! init metrics agent error: ", err)
		return nil
	}
	agent.InputProvider = provider
	return agent
}

func (ma *MetricsAgent) FilterPass(inputKey string) bool {
	if len(ma.InputFilters) > 0 {
		// do filter
		if _, has := ma.InputFilters[inputKey]; !has {
			return false
		}
	}
	return true
}

func (ma *MetricsAgent) Start() error {
	if _, err := ma.InputProvider.LoadConfig(); err != nil {
		log.Println("E! input provider load config get err: ", err)
	}
	ma.InputProvider.StartReloader()

	names, err := ma.InputProvider.GetInputs()
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Println("I! no inputs")
		return nil
	}

	for _, name := range names {
		_, inputKey := inputs.ParseInputName(name)
		if !ma.FilterPass(inputKey) {
			continue
		}

		configs, err := ma.InputProvider.GetInputConfig(name)
		if err != nil {
			log.Println("E! failed to get configuration of plugin:", name, "error:", err)
			continue
		}

		ma.RegisterInput(name, configs)
	}
	return nil
}

func (ma *MetricsAgent) Stop() error {
	ma.InputProvider.StopReloader()
	for name := range ma.InputReaders {
		ma.InputReaders[name].Stop()
	}
	return nil
}

func (ma *MetricsAgent) RegisterInput(name string, configs []cfg.ConfigWithFormat) {
	_, inputKey := inputs.ParseInputName(name)
	if !ma.FilterPass(inputKey) {
		return
	}

	creator, has := inputs.InputCreators[inputKey]
	if !has {
		log.Println("E! input:", name, "not supported")
		return
	}

	// construct input instance
	input := creator()

	err := cfg.LoadConfigs(configs, input)
	if err != nil {
		log.Println("E! failed to load configuration of plugin:", name, "error:", err)
		return
	}

	if err = input.InitInternalConfig(); err != nil {
		log.Println("E! failed to init input:", name, "error:", err)
		return
	}

	if err = inputs.MayInit(input); err != nil {
		if !errors.Is(err, types.ErrInstancesEmpty) {
			log.Println("E! failed to init input:", name, "error:", err)
		}
		return
	}

	instances := inputs.MayGetInstances(input)
	if instances != nil {
		empty := true
		for i := 0; i < len(instances); i++ {
			if err := instances[i].InitInternalConfig(); err != nil {
				log.Println("E! failed to init input:", name, "error:", err)
				continue
			}

			if err := inputs.MayInit(instances[i]); err != nil {
				if !errors.Is(err, types.ErrInstancesEmpty) {
					log.Println("E! failed to init input:", name, "error:", err)
				}
				continue
			}
			empty = false
		}

		if empty {
			return
		}
	}

	reader := newInputReader(name, input)
	go reader.startInput()
	ma.InputReaders[name] = reader
	log.Println("I! input:", name, "started")
}

func (ma *MetricsAgent) DeregisterInput(name string) {
	if reader, has := ma.InputReaders[name]; has {
		reader.Stop()
		delete(ma.InputReaders, name)
		log.Println("I! input:", name, "stopped")
	} else {
		log.Printf("W! dereigster input name [%s] not found", name)
	}
}

func (ma *MetricsAgent) ReregisterInput(name string, configs []cfg.ConfigWithFormat) {
	ma.DeregisterInput(name)
	ma.RegisterInput(name, configs)
}

func parseFilter(filterStr string) map[string]struct{} {
	filters := strings.Split(filterStr, ":")
	filtermap := make(map[string]struct{})
	for i := 0; i < len(filters); i++ {
		if strings.TrimSpace(filters[i]) == "" {
			continue
		}
		filtermap[filters[i]] = struct{}{}
	}
	return filtermap
}
