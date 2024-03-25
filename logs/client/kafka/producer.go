package kafka

import (
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"
)

const (
	AsyncProducer = "async"
	SyncProducer  = "sync"
)

type (
	Producer interface {
		Send(*sarama.ProducerMessage) error
		Close() error
		Metrics()
	}

	metric struct {
		sync.Mutex
		success int64
		failed  int64
		sent    int64
		counter map[string]*unit
	}
	unit struct {
		success int64
		failed  int64
		sent    int64
	}

	AsyncProducerWrapper struct {
		asyncProducer sarama.AsyncProducer
		stop          chan struct{}
		metric        *metric
	}

	SyncProducerWrapper struct {
		syncProducer sarama.SyncProducer
		stop         chan struct{}
		metric       *metric
	}
)

func (m *metric) inc(topic, typ string) {
	m.Lock()
	defer m.Unlock()
	switch typ {
	case "failed":
		m.failed++
	case "success":
		m.success++
	case "sent":
		m.sent++
	}

	if len(topic) == 0 {
		return
	}

	if m.counter[topic] == nil {
		m.counter[topic] = &unit{
			success: 0,
			failed:  0,
			sent:    0,
		}
	}
	switch typ {
	case "failed":
		m.counter[topic].failed++
	case "success":
		m.counter[topic].success++
	case "sent":
		m.counter[topic].sent++
	}
}

func (m *metric) Samples() []*types.Sample {
	m.Lock()
	defer m.Unlock()
	const (
		worker   = "categraf_logs_agent"
		mName    = "send_kafka_messages"
		allTopic = "categraf_all_topics"
	)
	now := time.Now()
	samples := append([]*types.Sample{},
		types.NewSample("", mName, m.sent, map[string]string{
			"source": worker,
			"type":   "sent",
			"topic":  allTopic,
		}).SetTime(now))
	samples = append(samples, types.NewSample("", mName, m.success, map[string]string{
		"source": worker,
		"type":   "success",
		"topic":  allTopic,
	}).SetTime(now))
	samples = append(samples, types.NewSample("", mName, m.failed, map[string]string{
		"source": worker,
		"type":   "failed",
		"topic":  allTopic,
	}).SetTime(now))
	for k, v := range m.counter {
		samples = append(samples, types.NewSample("", mName, v.sent, map[string]string{
			"source": worker,
			"type":   "sent",
			"topic":  k,
		}).SetTime(now))
		samples = append(samples, types.NewSample("", mName, v.success, map[string]string{
			"source": worker,
			"type":   "success",
			"topic":  k,
		}).SetTime(now))
		samples = append(samples, types.NewSample("", mName, v.failed, map[string]string{
			"source": worker,
			"type":   "failed",
			"topic":  k,
		}).SetTime(now))
	}

	return samples
}

func New(typ string, brokers []string, config *sarama.Config) (Producer, error) {
	stop := make(chan struct{})
	switch typ {
	case AsyncProducer:
		p, err := sarama.NewAsyncProducer(brokers, config)
		if err != nil {
			return nil, err
		}
		apw := &AsyncProducerWrapper{
			asyncProducer: p,
			stop:          stop,
			metric: &metric{
				counter: make(map[string]*unit),
			},
		}
		go apw.errorWorker()
		go apw.successWorker()
		return apw, nil
	case SyncProducer:
		p, err := sarama.NewSyncProducer(brokers, config)
		return &SyncProducerWrapper{
			syncProducer: p,
			stop:         stop,
			metric: &metric{
				counter: make(map[string]*unit),
			},
		}, err
	default:
		return nil, fmt.Errorf("unknown producer type: %s", typ)
	}
}

func (p *AsyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	go p.metric.inc("", "sent")
	p.asyncProducer.Input() <- msg
	return nil
}

func (p AsyncProducerWrapper) Metrics() {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-timer.C:
			samples := p.metric.Samples()
			for _, s := range samples {
				log.Println("DEBUG====", s.Metric, s.Value, s.Labels)
			}
			writer.WriteSamples(samples)
		}
	}
}

func (p *AsyncProducerWrapper) Close() error {
	close(p.stop)
	return p.asyncProducer.Close()
}

func (p *AsyncProducerWrapper) errorWorker() {
	for {
		select {
		case err := <-p.asyncProducer.Errors():
			p.metric.inc("", "failed")
			log.Println("E! kafka producer error", err)
		case <-p.stop:
			return
		}
	}
}

func (p *AsyncProducerWrapper) successWorker() {
	for {
		select {
		case <-p.asyncProducer.Successes():
			p.metric.inc("", "success")
		case <-p.stop:
			return
		}
	}
}

func (p *SyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	_, _, err := p.syncProducer.SendMessage(msg)
	go p.metric.inc(msg.Topic, "sent")
	if err == nil {
		go p.metric.inc(msg.Topic, "success")
	} else {
		go p.metric.inc(msg.Topic, "failed")
	}
	return err
}

func (p *SyncProducerWrapper) Close() error {
	close(p.stop)
	return p.syncProducer.Close()
}

func (p *SyncProducerWrapper) Metrics() {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-timer.C:
			writer.WriteSamples(p.metric.Samples())
		}
	}
}
