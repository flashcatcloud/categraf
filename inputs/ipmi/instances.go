package ipmi

import (
	"flashcat.cloud/categraf/inputs"
	"log"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs/ipmi/exporter"
	util "flashcat.cloud/categraf/pkg/metrics"
	"flashcat.cloud/categraf/types"
)

// Instance stores the configuration values for the ipmi_sensor input plugin
type Instance struct {
	config.InstanceConfig

	Target string `toml:"target"`
	Path   string `toml:"path"`
	exporter.IPMIConfig
}

func (m *Instance) Init() error {
	if len(m.Target) == 0 {
		return types.ErrInstancesEmpty
	}
	// Set defaults
	if m.Timeout == 0 {
		m.Timeout = 10000
	}

	return nil
}

// Gather is the main execution function for the plugin
func (m *Instance) Gather(slist *types.SampleList) {
	constLabels := m.GetLabels()
	metricChan := make(chan prometheus.Metric, 500)

	go func() {
		exporter.Collect(metricChan, m.Target, m.Path, m.IPMIConfig, m.DebugMod)
		close(metricChan)
	}()
	for metric := range metricChan {
		if metric == nil {
			continue
		}

		desc := metric.Desc().String()
		descName, err := inputs.DescName(desc)
		if err != nil {
			log.Println("E! failed to parse desc name:", desc)
			continue
		}
		icLabels, err := inputs.DescConstLabels(desc)
		if err != nil {
			log.Println("E! failed to read labels:", desc)
			continue
		}

		dtoMetric := &dto.Metric{}
		err = metric.Write(dtoMetric)
		if err != nil {
			log.Println("E! failed to write metric:", desc)
			continue
		}

		labels := map[string]string{
			"target": m.Target,
		}
		for k, v := range icLabels {
			labels[k] = v
		}

		for _, kv := range dtoMetric.Label {
			labels[*kv.Name] = *kv.Value
		}

		for k, v := range constLabels {
			labels[k] = v
		}

		switch {
		case dtoMetric.Counter != nil:
			slist.PushSample("", descName, *dtoMetric.Counter.Value, labels)
		case dtoMetric.Gauge != nil:
			slist.PushSample("", descName, *dtoMetric.Gauge.Value, labels)
		case dtoMetric.Summary != nil:
			util.HandleSummary("", dtoMetric, nil, descName, nil, slist)
		case dtoMetric.Histogram != nil:
			util.HandleHistogram("", dtoMetric, nil, descName, nil, slist)
		default:
			slist.PushSample("", descName, *dtoMetric.Untyped.Value, labels)
		}
	}
}

func (m *Instance) Drop() {

}
