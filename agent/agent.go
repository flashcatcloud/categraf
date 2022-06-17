package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"github.com/toolkits/pkg/file"

	// auto registry
	_ "flashcat.cloud/categraf/inputs/cpu"
	_ "flashcat.cloud/categraf/inputs/disk"
	_ "flashcat.cloud/categraf/inputs/diskio"
	_ "flashcat.cloud/categraf/inputs/docker"
	_ "flashcat.cloud/categraf/inputs/exec"
	_ "flashcat.cloud/categraf/inputs/http_response"
	_ "flashcat.cloud/categraf/inputs/kernel"
	_ "flashcat.cloud/categraf/inputs/kernel_vmstat"
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

const inputFilePrefix = "input."

type Agent struct {
	InputFilters         map[string]struct{}
	ClickHouseMetricChan chan *types.Sample
}

func NewAgent(filters map[string]struct{}) *Agent {
	return &Agent{
		InputFilters:         filters,
		ClickHouseMetricChan: make(chan *types.Sample, 100000),
	}
}

func (a *Agent) Start() {
	log.Println("I! agent starting")
	// 指标数据写clickhouse
	go a.startWriteMetricToClickHouse()
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
	names, err := a.getInputsByDirs()
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
		// set configurations for input instance
		cfg.LoadConfigs(path.Join(config.Config.ConfigDir, inputFilePrefix+name), instance)

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
		reader.Start(a.ClickHouseMetricChan)

		InputReaders[name] = reader
	}

	return nil
}

// input dir should has prefix input.
func (a *Agent) getInputsByDirs() ([]string, error) {
	dirs, err := file.DirsUnder(config.Config.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get dirs under %s : %v", config.Config.ConfigDir, err)
	}

	count := len(dirs)
	if count == 0 {
		return dirs, nil
	}

	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if strings.HasPrefix(dirs[i], inputFilePrefix) {
			names = append(names, dirs[i][len(inputFilePrefix):])
		}
	}

	return names, nil
}

// start write metric to clickhouse
func (a *Agent) startWriteMetricToClickHouse() {

	batch := config.Config.WriterOpt.Batch
	if batch <= 0 {
		batch = 10000
	}

	batchs := make([]*types.ClickHouseSample, 0, batch)

	var count int

	for {
		select {
		case item, open := <-a.ClickHouseMetricChan:
			if !open {
				// queue closed
				return
			}
			if item == nil {
				continue
			}

			batchs = append(batchs, convertToClickhouse(item))
			count++
			if count >= batch {
				start := time.Now()
				writeClickHouse(batchs)
				log.Println("This batchs insert coset: ", time.Since(start))
				count = 0
				batchs = make([]*types.ClickHouseSample, 0, batch)
			}
		default:
			if len(batchs) > 0 {
				start := time.Now()
				writeClickHouse(batchs)
				log.Println("This batchs insert coset: ", time.Since(start))
				count = 0
				batchs = make([]*types.ClickHouseSample, 0, batch)
			}
			time.Sleep(time.Second * 60)
		}
	}
}

func convertToClickhouse(item *types.Sample) *types.ClickHouseSample {
	if item.Labels == nil {
		item.Labels = make(map[string]string)
	}

	// add label: agent_hostname
	if _, has := item.Labels[agentHostnameLabelKey]; !has {
		if !config.Config.Global.OmitHostname {
			item.Labels[agentHostnameLabelKey] = config.Config.GetHostname()
		}
	}

	// add global labels
	for k, v := range config.Config.Global.Labels {
		if _, has := item.Labels[k]; !has {
			item.Labels[k] = v
		}
	}

	cs := &types.ClickHouseSample{}

	cs.Timestamp = item.Timestamp
	cs.Value = item.Value
	cs.Metric = item.Metric

	tag := ""

	// add other labels
	for k, v := range item.Labels {
		k = strings.Replace(k, "/", "_", -1)
		k = strings.Replace(k, ".", "_", -1)
		k = strings.Replace(k, "-", "_", -1)
		k = strings.Replace(k, " ", "_", -1)
		tag = fmt.Sprint(tag, "|", k, "=", v)
	}

	cs.Tags = strings.Trim(tag, "|")

	return cs
}

func writeClickHouse(batchs []*types.ClickHouseSample) error {
	ctx := context.Background()
	conn := <-writer.ClickHouseWriterChan
	defer func() {
		writer.ClickHouseWriterChan <- conn
	}()
	err := (*conn).Exec(ctx, `
		CREATE TABLE IF NOT EXISTS metric_log (
			  event_time DateTime
			, event_date Date
			, metric LowCardinality(String)
			, tags String
			, value Float64
		) ENGINE = MergeTree
		PARTITION BY toYYYYMM(event_date)
		ORDER BY (event_time, metric, tags)
		TTL event_date + toIntervalDay(365)
		SETTINGS index_granularity = 8192
	`)
	if err != nil {
		return err
	}

	batch, err := (*conn).PrepareBatch(ctx, "INSERT INTO metric_log")
	if err != nil {
		return err
	}

	for _, e := range batchs {
		err := batch.Append(
			e.Timestamp, //会自动转换时间格式
			e.Timestamp, //会自动转换时间格式
			e.Metric,
			e.Tags,
			e.Value,
		)
		if err != nil {
			return err
		}
	}
	return batch.Send()
}
