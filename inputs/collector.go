package inputs

import (
	"errors"
	"log"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	util "flashcat.cloud/categraf/pkg/metrics"
	"flashcat.cloud/categraf/types"
)

const capMetricChan = 1000

func Collect(e prometheus.Collector, slist *types.SampleList, constLabels ...map[string]string) error {
	if e == nil {
		return errors.New("exporter must not be nil")
	}

	metricChan := make(chan prometheus.Metric, capMetricChan)
	go func() {
		e.Collect(metricChan)
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

		labels := map[string]string{}
		for _, kv := range desc.ConstLabels() {
			labels[*kv.Name] = *kv.Value
		}

		for _, kv := range dtoMetric.Label {
			labels[*kv.Name] = *kv.Value
		}

		for _, kvs := range constLabels {
			for k, v := range kvs {
				labels[k] = v
			}
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

	return nil
}
