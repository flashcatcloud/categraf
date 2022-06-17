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

	// gather
	slist := list.NewSafeList()
	r.Instance.Gather(slist)

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

		if len(r.Instance.Prefix()) > 0 {
			s.Metric = r.Instance.Prefix() + "_" + metricReplacer.Replace(s.Metric)
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
		r.Queue <- s

		// write to clickhouse queue
		house.MetricsHouse.Push(s)
	}
}

func (r *Reader) read() {
	batch := config.Config.WriterOpt.Batch
	if batch <= 0 {
		batch = 2000
	}

	series := make([]*types.Sample, 0, batch)

	var count int

	for {
		select {
		case item, open := <-r.Queue:
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
