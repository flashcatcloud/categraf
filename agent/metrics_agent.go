package agent

import (
	"errors"
	"log"
	"strings"
	"sync"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"

	// auto registry
	_ "flashcat.cloud/categraf/inputs/aliyun"
	_ "flashcat.cloud/categraf/inputs/arp_packet"
	_ "flashcat.cloud/categraf/inputs/cadvisor"
	_ "flashcat.cloud/categraf/inputs/clickhouse"
	_ "flashcat.cloud/categraf/inputs/cloudwatch"
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
	_ "flashcat.cloud/categraf/inputs/ipmi"
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
	_ "flashcat.cloud/categraf/inputs/self_metrics"
	_ "flashcat.cloud/categraf/inputs/smart"
	_ "flashcat.cloud/categraf/inputs/snmp"
	_ "flashcat.cloud/categraf/inputs/sockstat"
	_ "flashcat.cloud/categraf/inputs/sqlserver"
	_ "flashcat.cloud/categraf/inputs/switch_legacy"
	_ "flashcat.cloud/categraf/inputs/system"
	_ "flashcat.cloud/categraf/inputs/systemd"
	_ "flashcat.cloud/categraf/inputs/tomcat"
	_ "flashcat.cloud/categraf/inputs/vsphere"
	_ "flashcat.cloud/categraf/inputs/xskyapi"
	_ "flashcat.cloud/categraf/inputs/zookeeper"
)

type MetricsAgent struct {
	InputFilters  map[string]struct{}
	InputReaders  *Readers
	InputProvider inputs.Provider
}

type Readers struct {
	lock   *sync.RWMutex
	record map[string]map[string]*InputReader
}

func NewReaders() *Readers {
	return &Readers{
		lock:   new(sync.RWMutex),
		record: make(map[string]map[string]*InputReader),
	}
}

func (r *Readers) Add(name string, sum string, reader *InputReader) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if _, ok := r.record[name]; !ok {
		r.record[name] = make(map[string]*InputReader)
	}
	r.record[name][sum] = reader
}

func (r *Readers) Del(name string, sum string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if len(sum) == 0 {
		delete(r.record, name)
		return
	}
	if _, ok := r.record[name]; ok {
		delete(r.record[name], sum)
	}
}

func (r *Readers) GetInput(name string) (map[string]*InputReader, bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	m, has := r.record[name]
	return m, has
}

func (r *Readers) Iter() map[string]map[string]*InputReader {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.record
}

func NewMetricsAgent() AgentModule {
	c := config.Config
	agent := &MetricsAgent{
		InputFilters: parseFilter(c.InputFilters),
		InputReaders: NewReaders(),
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
	for name := range ma.InputReaders.Iter() {
		inputs, _ := ma.InputReaders.GetInput(name)
		for sum, r := range inputs {
			r.Stop()
			ma.InputReaders.Del(name, sum)
		}
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

	newInputs, err := ma.InputProvider.LoadInputConfig(configs, creator())
	if err != nil {
		log.Println("E! failed to load configuration of plugin:", name, "error:", err)
		return
	}

	for sum, nInput := range newInputs {
		ma.inputGo(name, sum, nInput)
	}
}

func (ma *MetricsAgent) inputGo(name string, sum string, input inputs.Input) {
	var err error
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
			instances[i].SetInitialized()
		}

		if empty {
			if config.Config.DebugMode {
				_, inputKey := inputs.ParseInputName(name)
				log.Printf("W! no instances for input:%s", inputKey)
			}
			return
		}
	}

	reader := newInputReader(name, input)
	go reader.startInput()
	ma.InputReaders.Add(name, sum, reader)
	log.Println("I! input:", name, "started")
}

func (ma *MetricsAgent) DeregisterInput(name string, sum string) {
	if inputs, has := ma.InputReaders.GetInput(name); has {
		for isum, input := range inputs {
			if len(sum) == 0 || sum == isum {
				input.Stop()
			}
		}
		ma.InputReaders.Del(name, sum)
		log.Printf("I! input: %s[checksum:%s] stopped", name, sum)
	} else {
		log.Printf("W! dereigster input name [%s] not found", name)
	}
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
