package apache

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/apache/exporter"
	"flashcat.cloud/categraf/types"
)

const inputName = "apache"

type Apache struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type Instance struct {
	config.InstanceConfig
	LogLevel string `toml:"log_level"`
	exporter.Config

	e *exporter.Exporter

	logger log.Logger
}

var _ inputs.Input = new(Apache)
var _ inputs.SampleGatherer = new(Instance)
var _ inputs.InstancesGetter = new(Apache)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Apache{}
	})
}

func (a *Apache) Clone() inputs.Input {
	return &Apache{}
}

func (a *Apache) Name() string {
	return inputName
}

func (a *Apache) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(a.Instances))
	for i := 0; i < len(a.Instances); i++ {
		ret[i] = a.Instances[i]
	}
	return ret
}

func (a *Apache) Drop() {

	for _, i := range a.Instances {
		if i == nil {
			continue
		}

		if i.e != nil {
			i.e.Close()
		}
	}
}

// 根据字符串配置获取对应的日志级别过滤器
func getLevelFilter(logLevel string) level.Option {
	switch strings.ToLower(logLevel) {
	case "debug":
		return level.AllowDebug()
	case "info":
		return level.AllowInfo()
	case "warn":
		return level.AllowWarn()
	case "error":
		return level.AllowError()
	default:
		return level.AllowInfo() // 默认Info级别
	}
}

func (ins *Instance) Init() error {
	if len(ins.ScrapeURI) == 0 {
		return types.ErrInstancesEmpty
	}

	if len(ins.LogLevel) == 0 {
		ins.LogLevel = "info"
	}
	logger := log.NewLogfmtLogger(os.Stdout)
	logger = log.With(logger,
		"ts", log.DefaultTimestampUTC,
		"caller", log.DefaultCaller,
	)

	// 使用配置的日志级别
	logger = level.NewFilter(logger, getLevelFilter(ins.LogLevel))
	ins.logger = logger

	e, err := exporter.New(logger, &ins.Config)
	if err != nil {
		return fmt.Errorf("could not instantiate apache exporter: %v", err)
	}

	ins.e = e
	return nil

}

func (ins *Instance) Gather(slist *types.SampleList) {

	//  collect
	err := inputs.Collect(ins.e, slist)
	if err != nil {
		ins.logger.Log("E! failed to collect metrics:", err)
	}
}