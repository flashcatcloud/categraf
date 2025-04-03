package node_exporter

import (
	"fmt"
	"log"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/node_exporter/collector"
	"flashcat.cloud/categraf/types"
)

const (
	inputName = "node_exporter"
)

type (
	Exporter struct {
		Typ string `toml:"type"`
		config.PluginConfig

		Collectors []string `toml:"collectors"`

		exporterMetricsRegistry *prometheus.Registry
		nc                      *collector.NodeCollector
	}
)

var _ inputs.Input = new(Exporter)
var _ inputs.SampleGatherer = new(Exporter)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Exporter{}
	})
}

func (e *Exporter) Init() error {
	if len(e.Collectors) == 0 {
		return types.ErrInstancesEmpty
	}

	r := prometheus.NewRegistry()
	e.exporterMetricsRegistry = r
	nc, err := collector.NewNodeCollector(e.DebugMod, e.Collectors...)
	if err != nil {
		return fmt.Errorf("couldn't create collector: %s", err)
	}
	e.nc = nc

	if err := e.exporterMetricsRegistry.Register(e.nc); err != nil {
		return fmt.Errorf("couldn't register node collector: %s", err)
	}
	return nil
}

func (e *Exporter) Clone() inputs.Input {
	return &Exporter{}
}

func (e *Exporter) Name() string {
	return inputName
}

func (e *Exporter) Type() string {
	return e.Typ
}

func (e *Exporter) Drop() {
}

func (e *Exporter) Gather(slist *types.SampleList) {
	labels := e.GetLabels()
	err := inputs.Collect(e.nc, slist, labels)
	if err != nil {
		log.Println("E! node exporter collects error:", err)
	}
}
