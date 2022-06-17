package agent

import (
	"errors"
	"log"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"

	// auto registry
	_ "flashcat.cloud/categraf/inputs/cpu"
	_ "flashcat.cloud/categraf/inputs/disk"
	_ "flashcat.cloud/categraf/inputs/diskio"
	_ "flashcat.cloud/categraf/inputs/docker"
	_ "flashcat.cloud/categraf/inputs/exec"
	_ "flashcat.cloud/categraf/inputs/http_response"
	_ "flashcat.cloud/categraf/inputs/kernel"
	_ "flashcat.cloud/categraf/inputs/kernel_vmstat"
	_ "flashcat.cloud/categraf/inputs/kubernetes"
	_ "flashcat.cloud/categraf/inputs/linux_sysctl_fs"
	_ "flashcat.cloud/categraf/inputs/mem"
	_ "flashcat.cloud/categraf/inputs/mysql"
	_ "flashcat.cloud/categraf/inputs/net"
	_ "flashcat.cloud/categraf/inputs/net_response"
	_ "flashcat.cloud/categraf/inputs/netstat"
	_ "flashcat.cloud/categraf/inputs/nginx_upstream_check"
	_ "flashcat.cloud/categraf/inputs/ntp"
	_ "flashcat.cloud/categraf/inputs/nvidia_smi"
	_ "flashcat.cloud/categraf/inputs/oracle"
	_ "flashcat.cloud/categraf/inputs/ping"
	_ "flashcat.cloud/categraf/inputs/processes"
	_ "flashcat.cloud/categraf/inputs/procstat"
	_ "flashcat.cloud/categraf/inputs/prometheus"
	_ "flashcat.cloud/categraf/inputs/rabbitmq"
	_ "flashcat.cloud/categraf/inputs/redis"
	_ "flashcat.cloud/categraf/inputs/switch_legacy"
	_ "flashcat.cloud/categraf/inputs/system"
	_ "flashcat.cloud/categraf/inputs/tomcat"
)

type Agent struct {
	InputFilters map[string]struct{}
	cds          config.Discovery
}

func NewAgent(filters map[string]struct{}, cds config.Discovery) *Agent {
	return &Agent{
		InputFilters: filters,
		cds:          cds,
	}
}

func (a *Agent) Start() {
	log.Println("I! agent starting")

	a.startLogAgent()
	a.startInputs()
}

func (a *Agent) Stop() {
	log.Println("I! agent stopping")

	stopLogAgent()
	for name := range InputReaders {
		InputReaders[name].QuitChan <- struct{}{}
		close(InputReaders[name].Queue)
		InputReaders[name].Instance.Drop()
	}

	log.Println("I! agent stopped")
}

func (a *Agent) Reload() {
	log.Println("I! agent reloading")

	a.Stop()
	a.Start()
}

func (a *Agent) startInputs() error {
	names, err := a.cds.Inputs()
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Println("I! no inputs")
		return nil
	}

	for _, name := range names {
		if len(a.InputFilters) > 0 {
			// do filter
			if _, has := a.InputFilters[name]; !has {
				continue
			}
		}

		creator, has := inputs.InputCreators[name]
		if !has {
			log.Println("E! input:", name, "not supported")
			continue
		}

		// construct input instance
		instance := creator()
		// load configurations from configuration discovery service
		if err = a.cds.Load(instance); err != nil {
			log.Println("E! failed to load input configuration:", name, "error:", err)
			continue
		}

		if err = instance.Init(); err != nil {
			if !errors.Is(err, types.ErrInstancesEmpty) {
				log.Println("E! failed to init input:", name, "error:", err)
			}
			continue
		}

		reader := &Reader{
			Instance: instance,
			QuitChan: make(chan struct{}, 1),
			Queue:    make(chan *types.Sample, config.Config.WriterOpt.ChanSize),
		}

		log.Println("I! input:", name, "started")
		reader.Start()

		InputReaders[name] = reader
	}

	return nil
}
