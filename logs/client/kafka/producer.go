package kafka

import (
	"fmt"
	"log"
	"time"

	"github.com/IBM/sarama"

	"flashcat.cloud/categraf/logs/util"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
)

const (
	AsyncProducer = "async"
	SyncProducer  = "sync"

	Worker   = "categraf_logs_agent"
	Mname    = "send_kafka_messages"
	AllTopic = "categraf_all_topics"
)

type (
	Producer interface {
		Send(*sarama.ProducerMessage) error
		Close() error
		Metrics()
	}

	AsyncProducerWrapper struct {
		asyncProducer sarama.AsyncProducer
		stop          chan struct{}
		slist         *types.SampleList
	}

	SyncProducerWrapper struct {
		syncProducer sarama.SyncProducer
		stop         chan struct{}
		slist        *types.SampleList
	}
)

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
			slist:         types.NewSampleList(),
		}
		go apw.errorWorker()
		go apw.successWorker()
		return apw, nil
	case SyncProducer:
		p, err := sarama.NewSyncProducer(brokers, config)
		return &SyncProducerWrapper{
			syncProducer: p,
			stop:         stop,
			slist:        types.NewSampleList(),
		}, err
	default:
		return nil, fmt.Errorf("unknown producer type: %s", typ)
	}
}

func (p *AsyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	go p.slist.PushFront(types.NewSample("", Mname, 1, map[string]string{
		"source": Worker,
		"status": "sent",
		"topic":  AllTopic,
	}).SetTime(time.Now()))
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
			if p.slist == nil || p.slist.Len() == 0 {
				continue
			}
			samples := p.slist.PopBackAll()
			if util.Debug() {
				for _, s := range samples {
					log.Println("DEBUG====", s.Metric, s.Value, s.Labels, s.Timestamp)
				}
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
			log.Println("E! kafka producer error", err)
			p.slist.PushFront(types.NewSample("", Mname, 1, map[string]string{
				"source": Worker,
				"status": "failed",
				"topic":  AllTopic,
			}).SetTime(time.Now()))
		case <-p.stop:
			return
		}
	}
}

func (p *AsyncProducerWrapper) successWorker() {
	for {
		select {
		case <-p.asyncProducer.Successes():
			p.slist.PushFront(types.NewSample("", Mname, 1, map[string]string{
				"source": Worker,
				"status": "success",
				"topic":  AllTopic,
			}).SetTime(time.Now()))
		case <-p.stop:
			return
		}
	}
}

func (p *SyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	_, _, err := p.syncProducer.SendMessage(msg)
	now := time.Now()
	go p.slist.PushFront(types.NewSample("", Mname, 1, map[string]string{
		"source": Worker,
		"status": "sent",
		"topic":  AllTopic,
	}).SetTime(now))
	go p.slist.PushFront(types.NewSample("", Mname, 1, map[string]string{
		"source": Worker,
		"status": "sent",
		"topic":  msg.Topic,
	}).SetTime(now))
	if err == nil {
		go p.slist.PushFront(types.NewSample("", Mname, 1, map[string]string{
			"source": Worker,
			"status": "success",
			"topic":  AllTopic,
		}).SetTime(now))
	} else {
		go p.slist.PushFront(types.NewSample("", Mname, 1, map[string]string{
			"source": Worker,
			"status": "failed",
			"topic":  AllTopic,
		}).SetTime(now))
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
			if p.slist == nil || p.slist.Len() == 0 {
				continue
			}
			samples := p.slist.PopBackAll()
			writer.WriteSamples(samples)
		}
	}
}
