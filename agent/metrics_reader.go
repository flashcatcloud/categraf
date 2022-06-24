package agent

import (
	"fmt"
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/house"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"github.com/toolkits/pkg/container/list"
)

const agentHostnameLabelKey = "agent_hostname"

var metricReplacer = strings.NewReplacer("-", "_", ".", "_")

type InputReader struct {
	input    inputs.Input
	quitChan chan struct{}
	queue    chan *types.Sample
}

func NewInputReader(in inputs.Input) *InputReader {
	return &InputReader{
		input:    in,
		quitChan: make(chan struct{}, 1),
		queue:    make(chan *types.Sample, config.Config.WriterOpt.ChanSize),
	}
}

func (r *InputReader) Start() {
	// start consumer goroutines
	go r.read()

	// start collector instance
	go r.startInstance()
}

func (r *InputReader) Stop() {
	r.quitChan <- struct{}{}
	close(r.queue)
	r.input.Drop()
}

func (r *InputReader) startInstance() {
	interval := config.GetInterval()
	if r.input.GetInterval() > 0 {
		interval = time.Duration(r.input.GetInterval())
	}
	for {
		select {
		case <-r.quitChan:
			close(r.quitChan)
			return
		default:
			time.Sleep(interval)
			r.gatherOnce()
		}
	}
}

func (r *InputReader) gatherOnce() {
	defer func() {
		if r := recover(); r != nil {
			if strings.Contains(fmt.Sprint(r), "closed channel") {
				return
			} else {
				log.Println("E! gather metrics panic:", r)
			}
		}
	}()

	// gather
	slist := list.NewSafeList()
	r.input.Gather(slist)

	// handle result
	samples := slist.PopBackAll()

	if len(samples) == 0 {
		return
	}

	now := time.Now()
	for i := 0; i < len(samples); i++ {
		if samples[i] == nil {
			continue
		}

		s := samples[i].(*types.Sample)

		if s.Timestamp.IsZero() {
			s.Timestamp = now
		}

		if len(r.input.Prefix()) > 0 {
			s.Metric = r.input.Prefix() + "_" + metricReplacer.Replace(s.Metric)
		} else {
			s.Metric = metricReplacer.Replace(s.Metric)
		}

		if s.Labels == nil {
			s.Labels = make(map[string]string)
		}

		// add label: agent_hostname
		if _, has := s.Labels[agentHostnameLabelKey]; !has {
			if !config.Config.Global.OmitHostname {
				s.Labels[agentHostnameLabelKey] = config.Config.GetHostname()
			}
		}

		// add global labels
		for k, v := range config.Config.Global.Labels {
			if _, has := s.Labels[k]; !has {
				s.Labels[k] = v
			}
		}

		// write to remote write queue
		r.queue <- s

		// write to clickhouse queue
		house.MetricsHouse.Push(s)
	}
}

func (r *InputReader) read() {
	batch := config.Config.WriterOpt.Batch
	if batch <= 0 {
		batch = 2000
	}

	series := make([]*types.Sample, 0, batch)

	var count int

	for {
		select {
		case item, open := <-r.queue:
			if !open {
				// queue closed
				return
			}

			if item == nil {
				continue
			}

			series = append(series, item)
			count++
			if count >= batch {
				writer.PostSeries(series)
				count = 0
				series = make([]*types.Sample, 0, batch)
			}
		default:
			if len(series) > 0 {
				writer.PostSeries(series)
				count = 0
				series = make([]*types.Sample, 0, batch)
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}
