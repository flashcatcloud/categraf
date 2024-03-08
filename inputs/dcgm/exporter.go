//go:build dcgm

package dcgm

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/dcgm/dcgmexporter"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/types"
)

const (
	inputName = "dcgm"
	FlexKey   = "f" // Monitor all GPUs if MIG is disabled or all GPU instances if MIG is enabled
	MajorKey  = "g" // Monitor top-level entities: GPUs or NvSwitches or CPUs
	MinorKey  = "i" // Monitor sub-level entities: GPU instances/NvLinks/CPUCores - GPUI cannot be specified if MIG is disabled)
)

type (
	Exporter struct {
		config.PluginConfig
		Instances []*Instance `toml:"instances"`
	}

	Instance struct {
		config.InstanceConfig

		CollectorsFile      string `toml:"collectors"`
		Kubernetes          bool   `toml:"kubernetes"`
		KubernetesGPUIDType string `toml:"kubernetes-gpu-id-type"`
		UseOldNamespace     bool   `toml:"use-old-namespace"`
		CPUDevices          string `toml:"cpu-devices"`
		cpuDevices          string `toml:"-"`
		GPUDevices          string `toml:"devices"`
		gpuDevices          string `toml:"-"`
		SwitchDevices       string `toml:"switch-devices"`
		switchDevices       string `toml:"-"`
		ConfigMapData       string `toml:"configmap-data"`
		RemoteHostEngine    string `toml:"remote-hostengine-info"`
		FakeGPU             bool   `toml:"fake-gpus"`
		ReplaceBlanks       bool   `toml:"replace-blanks-in-model-name"`

		metricsChan chan string
		registry    *dcgmexporter.Registry
		plCleanup   func()
		pipeline    *dcgmexporter.MetricsPipeline
		dcgmCleanup func()
		stop        chan interface{}
	}
)

var _ inputs.Input = new(Exporter)
var _ inputs.SampleGatherer = new(Instance)
var _ inputs.InstancesGetter = new(Exporter)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Exporter{}
	})
}

func (e *Exporter) Clone() inputs.Input {
	return &Exporter{}
}

func (e *Exporter) Name() string {
	return inputName
}

func (e *Exporter) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(e.Instances))
	for i := 0; i < len(e.Instances); i++ {
		ret[i] = e.Instances[i]
	}
	return ret
}

func (e *Exporter) Drop() {
	for i := 0; i < len(e.Instances); i++ {
		e.Instances[i].Drop()
	}
}

func (ins *Instance) Init() error {

	if len(ins.CollectorsFile) == 0 {
		return types.ErrInstancesEmpty
	}

	gOpt, err := parseDeviceOptions(ins.GPUDevices)
	if err != nil {
		return err
	}

	sOpt, err := parseDeviceOptions(ins.SwitchDevices)
	if err != nil {
		return err
	}

	cOpt, err := parseDeviceOptions(ins.CPUDevices)
	if err != nil {
		return err
	}

	cfg := &dcgmexporter.Config{
		CollectorsFile: ins.CollectorsFile,
		// Address:                    i.Address,
		// CollectInterval:            i.Interval * i.GetIntervalTimes(),
		Kubernetes:          ins.Kubernetes,
		KubernetesGPUIdType: dcgmexporter.KubernetesGPUIDType(ins.KubernetesGPUIDType),
		CollectDCP:          true,
		UseOldNamespace:     ins.UseOldNamespace,
		UseRemoteHE:         ins.RemoteHostEngine != "",
		RemoteHEInfo:        ins.RemoteHostEngine,
		GPUDevices:          gOpt,
		SwitchDevices:       sOpt,
		CPUDevices:          cOpt,
		// NoHostname:                 config.Bool(CLINoHostname),
		UseFakeGPUs:      ins.FakeGPU,
		ConfigMapData:    ins.ConfigMapData,
		WebSystemdSocket: false,
		// WebConfigFile:              config.String(CLIWebConfigFile),
		// XIDCountWindowSize:         config.Int(CLIXIDCountWindowSize),
		ReplaceBlanksInModelName: ins.ReplaceBlanks,
		Debug:                    ins.DebugMod,
		// ClockEventsCountWindowSize: config.Int(CLIClockEventsCountWindowSize),
	}

	// The purpose of this function is to capture any panic that may occur
	// during initialization and return an error.
	defer func() {
		if r := recover(); r != nil {
			log.Println(string(debug.Stack()))
			log.Printf("E! encountered a failure; err: %v", r)
		}
	}()

	if ins.DebugMod {
		// enable debug logging
		log.Println("Starting dcgm-exporter")
	}

	if ins.DebugMod {
		log.Printf("%+v", cfg)
	}

	if cfg.UseRemoteHE {
		log.Printf("Attemping to connect to remote hostengine at ", cfg.RemoteHEInfo)
		ins.dcgmCleanup, err = dcgm.Init(dcgm.Standalone, cfg.RemoteHEInfo, "0")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		ins.dcgmCleanup, err = dcgm.Init(dcgm.Embedded)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Println("DCGM successfully initialized!")

	dcgm.FieldsInit()
	defer dcgm.FieldsTerm()

	var groups []dcgm.MetricGroup
	groups, err = dcgm.GetSupportedMetricGroups(0)
	if err != nil {
		cfg.CollectDCP = false
		log.Println("Not collecting DCP metrics: ", err)
	} else {
		log.Println("Collecting DCP Metrics")
		cfg.MetricGroups = groups
	}

	cs, err := dcgmexporter.GetCounterSet(cfg)

	if err != nil {
		log.Fatalln(err)
	}

	// Copy labels from DCGM Counters to ExporterCounters
	for i := range cs.DCGMCounters {
		if cs.DCGMCounters[i].PromType == "label" {
			cs.ExporterCounters = append(cs.ExporterCounters, cs.DCGMCounters[i])
		}
	}

	allCounters := []dcgmexporter.Counter{}

	allCounters = append(allCounters, cs.DCGMCounters...)
	allCounters = append(allCounters,
		dcgmexporter.Counter{
			FieldID: dcgm.DCGM_FI_DEV_CLOCK_THROTTLE_REASONS,
		},
		dcgmexporter.Counter{
			FieldID: dcgm.DCGM_FI_DEV_XID_ERRORS,
		},
	)

	fieldEntityGroupTypeSystemInfo := dcgmexporter.NewEntityGroupTypeSystemInfo(allCounters, cfg)

	for _, egt := range dcgmexporter.FieldEntityGroupTypeToMonitor {
		err := fieldEntityGroupTypeSystemInfo.Load(egt)
		if err != nil {
			log.Printf("Not collecting %s metrics; %s", egt.String(), err)
		}
	}

	hostname := config.Config.GetHostname()

	pipeline, cleanup, err := dcgmexporter.NewMetricsPipeline(cfg,
		cs.DCGMCounters,
		hostname,
		dcgmexporter.NewDCGMCollector,
		fieldEntityGroupTypeSystemInfo,
	)
	ins.pipeline = pipeline
	ins.plCleanup = cleanup
	if err != nil {
		log.Fatal(err)
	}

	ins.registry = dcgmexporter.NewRegistry()

	if dcgmexporter.IsDCGMExpXIDErrorsCountEnabled(cs.ExporterCounters) {
		item, exists := fieldEntityGroupTypeSystemInfo.Get(dcgm.FE_GPU)
		if !exists {
			log.Fatalf("%s collector cannot be initialized", dcgmexporter.DCGMXIDErrorsCount.String())
		}

		xidCollector, err := dcgmexporter.NewXIDCollector(cs.ExporterCounters, hostname, cfg, item)
		if err != nil {
			log.Fatal(err)
		}

		ins.registry.Register(xidCollector)

		log.Printf("%s collector initialized", dcgmexporter.DCGMXIDErrorsCount.String())
	}

	if dcgmexporter.IsDCGMExpClockEventsCountEnabled(cs.ExporterCounters) {
		item, exists := fieldEntityGroupTypeSystemInfo.Get(dcgm.FE_GPU)
		if !exists {
			log.Fatalf("%s collector cannot be initialized", dcgmexporter.DCGMClockEventsCount.String())
		}
		clocksThrottleReasonsCollector, err := dcgmexporter.NewClockEventsCollector(
			cs.ExporterCounters, hostname, cfg, item)
		if err != nil {
			log.Fatal(err)
		}

		ins.registry.Register(clocksThrottleReasonsCollector)

		log.Printf("%s collector initialized", dcgmexporter.DCGMClockEventsCount.String())
	}
	return nil
}

func parseDeviceOptions(devices string) (dcgmexporter.DeviceOptions, error) {
	var dOpt dcgmexporter.DeviceOptions

	letterAndRange := strings.Split(devices, ":")
	count := len(letterAndRange)
	if count > 2 {
		return dOpt, fmt.Errorf("invalid ranged device option '%s'; err: there can only be one specified range",
			devices)
	}

	letter := letterAndRange[0]
	if letter == FlexKey {
		dOpt.Flex = true
		if count > 1 {
			return dOpt, fmt.Errorf("no range can be specified with the flex option 'f'")
		}
	} else if letter == MajorKey || letter == MinorKey {
		var indices []int
		if count == 1 {
			// No range means all present devices of the type
			indices = append(indices, -1)
		} else {
			numbers := strings.Split(letterAndRange[1], ",")
			for _, numberOrRange := range numbers {
				rangeTokens := strings.Split(numberOrRange, "-")
				rangeTokenCount := len(rangeTokens)
				if rangeTokenCount > 2 {
					return dOpt, fmt.Errorf("range can only be '<number>-<number>', but found '%s'", numberOrRange)
				} else if rangeTokenCount == 1 {
					number, err := strconv.Atoi(rangeTokens[0])
					if err != nil {
						return dOpt, err
					}
					indices = append(indices, number)
				} else {
					start, err := strconv.Atoi(rangeTokens[0])
					if err != nil {
						return dOpt, err
					}
					end, err := strconv.Atoi(rangeTokens[1])
					if err != nil {
						return dOpt, err
					}

					// Add the range to the indices
					for i := start; i <= end; i++ {
						indices = append(indices, i)
					}
				}
			}
		}

		if letter == MajorKey {
			dOpt.MajorRange = indices
		} else {
			dOpt.MinorRange = indices
		}
	} else {
		return dOpt, fmt.Errorf("valid options preceding ':<range>' are 'g' or 'i', but found '%s'", letter)
	}

	return dOpt, nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	buf := new(bytes.Buffer)
	labels := ins.GetLabels()
	out, err := ins.pipeline.Run()
	if err != nil {
		log.Println("E! dcgm exporter collects error:", err)
		return
	}
	buf.WriteString(out)
	metrics, err := ins.registry.Gather()
	if err != nil {
		log.Println("E! dcgm exporter collects error:", err)
		return
	}
	err = dcgmexporter.EncodeExpMetrics(buf, metrics)
	if err != nil {
		log.Println("E! dcgm exporter collects error:", err)
		return
	}
	parser := prometheus.NewParser("", labels, http.Header{}, false,
		nil, nil)
	err = parser.Parse(buf.Bytes(), slist)
	if err != nil {
		log.Println("E! dcgm exporter collects error:", err)
		return
	}
}

func (ins *Instance) Drop() {
	ins.registry.Cleanup()
	ins.plCleanup()
	ins.dcgmCleanup()
}
