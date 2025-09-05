package kafka

import (
	"fmt"
	"log"

	"github.com/IBM/sarama"

	"flashcat.cloud/categraf/logs/util"
)

const (
	AsyncProducer = "async"
	SyncProducer  = "sync"
)

type (
	Producer interface {
		Send(*sarama.ProducerMessage) error
		Close() error
	}

	AsyncProducerWrapper struct {
		asyncProducer sarama.AsyncProducer
		stop          chan struct{}
		counter       int64
	}

	SyncProducerWrapper struct {
		syncProducer sarama.SyncProducer
		stop         chan struct{}
		counter      int64
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
		}
		go apw.errorWorker()
		go apw.successWorker()
		return apw, nil
	case SyncProducer:
		p, err := sarama.NewSyncProducer(brokers, config)
		return &SyncProducerWrapper{syncProducer: p, stop: stop}, err
	default:
		return nil, fmt.Errorf("unknown producer type: %s", typ)
	}
}

func (p *AsyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	p.asyncProducer.Input() <- msg
	return nil
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
		case <-p.stop:
			return
		}
	}
}

func (p *AsyncProducerWrapper) successWorker() {
	for {
		select {
		case <-p.asyncProducer.Successes():
			p.counter++
			if util.Debug() {
				log.Printf("D! kafka producer message success, total:%d", p.counter)
			}
		case <-p.stop:
			return
		}
	}
}

func (p *SyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	_, _, err := p.syncProducer.SendMessage(msg)
	if err == nil {
		p.counter++
		if util.Debug() {
			log.Printf("D! kafka producer message success, total:%d", p.counter)
		}
	}
	return err
}

func (p *SyncProducerWrapper) Close() error {
	close(p.stop)
	return p.syncProducer.Close()
}
