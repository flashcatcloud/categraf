//go:build !no_logs

package kafka

import (
	"fmt"

	"github.com/Shopify/sarama"
)

type MessageBuilder struct {
	sarama.ProducerMessage
}

func NewBuilder() *MessageBuilder {
	return &MessageBuilder{}
}

func (m *MessageBuilder) WithMessage(key string, value []byte) *MessageBuilder {
	m.Key = sarama.StringEncoder(key)
	m.Value = sarama.ByteEncoder(value)
	return m
}

func (m *MessageBuilder) WithTopic(topic string) *MessageBuilder {
	m.Topic = topic
	return m
}

func (s *MessageBuilder) build() (*sarama.ProducerMessage, error) {
	switch {
	case len(s.Topic) == 0:
		return nil, fmt.Errorf("Message (%s) must not be nil", "topic")
	case s.Key.Length() == 0:
		return nil, fmt.Errorf("Message (%s) must not be nil", "key")
	case s.Value.Length() == 0:
		return nil, fmt.Errorf("Message (%s) must not be nil", "value")
	}
	return &s.ProducerMessage, nil
}

func (m *MessageBuilder) Send(producer sarama.SyncProducer) error {
	if producer == nil {
		return fmt.Errorf("empty producer")
	}

	msg, err := m.build()
	if err != nil {
		return err
	}

	_, _, err = producer.SendMessage(msg)
	return err
}
