package agent

import (
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

type Consumer struct {
	Instance inputs.Input
	Queue    chan *types.Sample
}

var InputConsumers = map[string]*Consumer{}

func (c *Consumer) Start() {
	// start consumer goroutines
	go consume(c.Queue)

	// start collector goroutines
	c.Instance.StartGoroutines(c.Queue)
}

const batch = 2000

func consume(queue chan *types.Sample) {
	series := make([]*prompb.TimeSeries, 0, batch)

	var count int

	for {
		select {
		case item := <-queue:
			series = append(series, convert(item))
			count++
			if count >= batch {
				postSeries(series)
				count = 0
				series = make([]*prompb.TimeSeries, 0, batch)
			}
		default:
			if len(series) > 0 {
				postSeries(series)
				count = 0
				series = make([]*prompb.TimeSeries, 0, batch)
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func postSeries(series []*prompb.TimeSeries) {
	for _, w := range writer.Writers {
		w.Write(series)
	}
}

func convert(item *types.Sample) *prompb.TimeSeries {
	pt := &prompb.TimeSeries{}

	if item.Timestamp < 0xffffffff {
		// s -> ms
		item.Timestamp = item.Timestamp * 1000
	}

	pt.Samples = append(pt.Samples, prompb.Sample{
		Timestamp: item.Timestamp,
		Value:     item.Value,
	})

	// add label: metric
	pt.Labels = append(pt.Labels, &prompb.Label{
		Name:  model.MetricNameLabel,
		Value: item.Metric,
	})

	// add label: agent_hostname
	if !config.Config.Global.OmitHostname {
		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  "agent_hostname",
			Value: config.Config.Global.Hostname,
		})
	}

	// add other labels
	for k, v := range item.Labels {
		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  k,
			Value: v,
		})
	}

	return pt
}
