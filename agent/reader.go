package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

type Reader struct {
	Instance inputs.Input
	Queue    chan *types.Sample
}

var InputReaders = map[string]*Reader{}

func (c *Reader) Start() {
	// start consumer goroutines
	go read(c.Queue)

	// start collector goroutines
	c.Instance.StartGoroutines(c.Queue)
}

func read(queue chan *types.Sample) {
	batch := config.Config.WriterOpt.Batch
	if batch <= 0 {
		batch = 2000
	}

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
	if config.Config.TestMode {
		for i := 0; i < len(series); i++ {
			var sb strings.Builder

			sb.WriteString(">> ")

			for j := range series[i].Labels {
				sb.WriteString(series[i].Labels[j].Name)
				sb.WriteString("=")
				sb.WriteString(series[i].Labels[j].Value)
				sb.WriteString(" ")
			}

			for j := range series[i].Samples {
				sb.WriteString(fmt.Sprint(series[i].Samples[j].Timestamp))
				sb.WriteString(" ")
				sb.WriteString(fmt.Sprint(series[i].Samples[j].Value))
			}

			fmt.Println(sb.String())
		}
		return
	}

	wg := sync.WaitGroup{}
	for key := range writer.Writers {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			writer.Writers[key].Write(series)
		}(key)
	}
}

func convert(item *types.Sample) *prompb.TimeSeries {
	if item.Labels == nil {
		item.Labels = make(map[string]string)
	}

	// add label: agent_hostname
	if _, has := item.Labels["agent_hostname"]; !has {
		if !config.Config.Global.OmitHostname {
			item.Labels["agent_hostname"] = config.Config.Global.Hostname
		}
	}

	// add global labels
	for k, v := range config.Config.Global.Labels {
		if _, has := item.Labels[k]; !has {
			item.Labels[k] = v
		}
	}

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

	// add other labels
	for k, v := range item.Labels {
		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  k,
			Value: v,
		})
	}

	return pt
}
