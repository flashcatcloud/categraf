package categraf

import (
	"log"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/metrics"
	"flashcat.cloud/categraf/types"
)

const (
	inputName     = `self_metrics`
	defaultPrefix = "categraf"
)

type Categraf struct {
	config.PluginConfig
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Categraf{}
	})
}

func (pt *Categraf) Clone() inputs.Input {
	return &Categraf{}
}

func (pt *Categraf) Name() string {
	return inputName
}

func (ins *Categraf) Gather(slist *types.SampleList) {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		log.Println(err)
		return
	}
	vTag := map[string]string{
		"version": config.Version,
	}
	slist.PushSample(defaultPrefix, "info", 1, vTag)
	for _, mf := range mfs {
		metricName := mf.GetName()
		for _, m := range mf.Metric {
			tags := metrics.MakeLabels(m, ins.GetLabels())

			if mf.GetType() == dto.MetricType_SUMMARY {
				metrics.HandleSummary(defaultPrefix, m, tags, metricName, nil, slist)
			} else if mf.GetType() == dto.MetricType_HISTOGRAM {
				metrics.HandleHistogram(defaultPrefix, m, tags, metricName, nil, slist)
			} else {
				metrics.HandleGaugeCounter(defaultPrefix, m, tags, metricName, nil, slist)
			}
		}
	}
}
