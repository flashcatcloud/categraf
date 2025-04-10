package ipmi

import (
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

		desc := metric.Desc()
		if desc.Err() != nil {
			log.Println("E! got invalid metric:", desc.Name(), desc.Err())
			continue
		}

		dtoMetric := &dto.Metric{}
		err := metric.Write(dtoMetric)
		if err != nil {
			log.Println("E! failed to write metric:", desc.String())
			continue
		}

		labels := map[string]string{
			"target": m.Target,
		}
		for _, kv := range desc.ConstLabels() {
			labels[*kv.Name] = *kv.Value
		}

		for _, kv := range dtoMetric.Label {
			labels[*kv.Name] = *kv.Value
		}

		for k, v := range constLabels {
			labels[k] = v
		}

		switch {
		case dtoMetric.Counter != nil:
			slist.PushSample("", desc.Name(), *dtoMetric.Counter.Value, labels)
		case dtoMetric.Gauge != nil:
			slist.PushSample("", desc.Name(), *dtoMetric.Gauge.Value, labels)
		case dtoMetric.Summary != nil:
			util.HandleSummary("", dtoMetric, nil, desc.Name(), nil, slist)
		case dtoMetric.Histogram != nil:
			util.HandleHistogram("", dtoMetric, nil, desc.Name(), nil, slist)
		default:
			slist.PushSample("", desc.Name(), *dtoMetric.Untyped.Value, labels)
		}
	}
}

func (m *Instance) Drop() {

}
