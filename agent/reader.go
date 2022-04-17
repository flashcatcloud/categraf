package agent

import (
	"fmt"
	"log"
	"sort"
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

const agentHostnameLabelKey = "agent_hostname"

type Reader struct {
	Instance inputs.Input
	QuitChan chan struct{}
	Queue    chan *types.Sample
}

var InputReaders = map[string]*Reader{}

func (r *Reader) Start() {
	// start consumer goroutines
	go r.read()

	// start collector instance
	go r.startInstance()
}

func (r *Reader) startInstance() {
	interval := config.GetInterval()
	if r.Instance.GetInterval() > 0 {
		interval = time.Duration(r.Instance.GetInterval())
	}
	for {
		select {
		case <-r.QuitChan:
			close(r.QuitChan)
			return
		default:
			time.Sleep(interval)
			r.gatherOnce()
		}
	}
}

func (r *Reader) gatherOnce() {
	defer func() {
		if r := recover(); r != nil {
			if strings.Contains(fmt.Sprint(r), "closed channel") {
				return
			} else {
				log.Println("E! gather metrics panic:", r)
			}
		}
	}()

	samples := r.Instance.Gather()

	if len(samples) == 0 {
		return
	}

	now := time.Now()
	for i := 0; i < len(samples); i++ {
		if samples[i].Timestamp.IsZero() {
			samples[i].Timestamp = now
		}

		samples[i].Metric = r.Instance.GetInputName() + "_" + strings.ReplaceAll(samples[i].Metric, "-", "_")
		r.Queue <- samples[i]
	}
}

func (r *Reader) read() {
	batch := config.Config.WriterOpt.Batch
	if batch <= 0 {
		batch = 2000
	}

	series := make([]*prompb.TimeSeries, 0, batch)

	var count int

	for {
		select {
		case item := <-r.Queue:
			if item == nil {
				// queue closed
				return
			}
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
		log.Println(">> count:", len(series))

		for i := 0; i < len(series); i++ {
			var sb strings.Builder

			sb.WriteString(">> ")

			sort.SliceStable(series[i].Labels, func(x, y int) bool {
				return strings.Compare(series[i].Labels[x].Name, series[i].Labels[y].Name) == -1
			})

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

			log.Println(sb.String())
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
	wg.Wait()
}

func convert(item *types.Sample) *prompb.TimeSeries {
	if item.Labels == nil {
		item.Labels = make(map[string]string)
	}

	// add label: agent_hostname
	if _, has := item.Labels[agentHostnameLabelKey]; !has {
		if !config.Config.Global.OmitHostname {
			item.Labels[agentHostnameLabelKey] = config.Config.Global.Hostname
		}
	}

	// add global labels
	for k, v := range config.Config.Global.Labels {
		if _, has := item.Labels[k]; !has {
			item.Labels[k] = v
		}
	}

	pt := &prompb.TimeSeries{}

	timestamp := item.Timestamp.UnixMilli()
	if config.Config.Global.Precision == "s" {
		timestamp = item.Timestamp.Unix()
	}

	pt.Samples = append(pt.Samples, prompb.Sample{
		Timestamp: timestamp,
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
