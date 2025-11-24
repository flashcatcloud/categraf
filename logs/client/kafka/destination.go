//go:build !no_logs

package kafka

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	json "github.com/mailru/easyjson"

	coreconfig "flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/client"
	"flashcat.cloud/categraf/logs/util"
	"flashcat.cloud/categraf/pkg/backoff"
)

// ContentType options,
const (
	TextContentType = "text/plain"
	JSONContentType = "application/json"
)

// Kafka errors.
var (
	errClient = errors.New("client error")
	errServer = errors.New("server error")
)

// Destination sends a payload over Kafka.
type Destination struct {
	topic   string
	brokers []string

	apiKey              string
	contentType         string
	contentEncoding     ContentEncoding
	client              Producer
	destinationsContext *client.DestinationsContext
	once                sync.Once
	payloadChan         chan []byte
	climit              chan struct{} // semaphore for limiting concurrent background sends
	backoff             backoff.Policy
	nbErrors            int
	blockedUntil        time.Time
	protocol            logsconfig.IntakeProtocol
	origin              logsconfig.IntakeOrigin
}

// NewDestination returns a new Destination.
// If `maxConcurrentBackgroundSends` > 0, then at most that many background payloads will be sent concurrently, else
// there is no concurrency and the background sending pipeline will block while sending each payload.
// TODO: add support for SOCKS5
func NewDestination(endpoint logsconfig.Endpoint, contentType string, destinationsContext *client.DestinationsContext, maxConcurrentBackgroundSends int) *Destination {
	return newDestination(endpoint, contentType, destinationsContext, time.Duration(coreconfig.ClientTimeout())*time.Second, maxConcurrentBackgroundSends)
}

func newDestination(endpoint logsconfig.Endpoint, contentType string, destinationsContext *client.DestinationsContext, timeout time.Duration, maxConcurrentBackgroundSends int) *Destination {
	if maxConcurrentBackgroundSends < 0 {
		maxConcurrentBackgroundSends = 0
	}

	policy := backoff.NewPolicy(
		endpoint.BackoffFactor,
		endpoint.BackoffBase,
		endpoint.BackoffMax,
		endpoint.RecoveryInterval,
		endpoint.RecoveryReset,
	)
	typ := SyncProducer

	if coreconfig.Config.Logs.Config == nil {
		coreconfig.Config.Logs.Config = sarama.NewConfig()
		// default
		coreconfig.Config.Logs.Producer.Partitioner = sarama.NewRandomPartitioner
		if coreconfig.Config.Logs.PartitionStrategy == "round_robin" {
			coreconfig.Config.Logs.Producer.Partitioner = sarama.NewRoundRobinPartitioner
		}
		if coreconfig.Config.Logs.PartitionStrategy == "random" {
			coreconfig.Config.Logs.Producer.Partitioner = sarama.NewRandomPartitioner
		}
		if coreconfig.Config.Logs.PartitionStrategy == "hash" {
			coreconfig.Config.Logs.Producer.Partitioner = sarama.NewHashPartitioner
		}
		coreconfig.Config.Logs.Producer.Return.Successes = true
		coreconfig.Config.Logs.Producer.Return.Errors = true
		if coreconfig.BatchConcurrence() > 0 {
			size := 256
			if coreconfig.BatchMaxSize() > 256 {
				size = coreconfig.BatchMaxSize()
			}
			coreconfig.Config.Logs.ChannelBufferSize = size
			coreconfig.Config.Logs.Net.MaxOpenRequests = coreconfig.BatchConcurrence()
			coreconfig.Config.Logs.Producer.RequiredAcks = sarama.WaitForAll
		}
	}
	if coreconfig.Config.Logs.UseCompression {
		switch coreconfig.Config.Logs.CompressionCodec {
		case "gzip":
			coreconfig.Config.Logs.Config.Producer.Compression = sarama.CompressionGZIP
		case "snappy":
			coreconfig.Config.Logs.Config.Producer.Compression = sarama.CompressionSnappy
		case "lz4":
			coreconfig.Config.Logs.Config.Producer.Compression = sarama.CompressionLZ4
		case "zstd":
			coreconfig.Config.Logs.Config.Producer.Compression = sarama.CompressionZSTD
		default:
			coreconfig.Config.Logs.Config.Producer.Compression = sarama.CompressionNone
		}
		coreconfig.Config.Logs.Producer.CompressionLevel = coreconfig.Config.Logs.CompressionLevel
	}

	if coreconfig.BatchConcurrence() > 0 {
		typ = AsyncProducer
	}
	if util.Debug() {
		log.Println("D! producer type:", typ, coreconfig.Config.Logs.ChannelBufferSize, coreconfig.Config.Logs.Net.MaxOpenRequests)
	}
	coreconfig.Config.Logs.Config.Producer.Timeout = timeout

	if coreconfig.Config.Logs.SendWithTLS && coreconfig.Config.Logs.SendType == "kafka" {
		coreconfig.Config.Logs.Config.Net.TLS.Enable = true
		coreconfig.Config.Logs.UseTLS = true
		var err error
		coreconfig.Config.Logs.Net.TLS.Config, err = coreconfig.Config.Logs.KafkaConfig.ClientConfig.TLSConfig()
		if err != nil {
			panic(err)
		}
	}
	if coreconfig.Config.Logs.SaslEnable {
		coreconfig.Config.Logs.Config.Net.SASL.Enable = true
		coreconfig.Config.Logs.Config.Net.SASL.User = coreconfig.Config.Logs.SaslUser
		coreconfig.Config.Logs.Config.Net.SASL.Password = coreconfig.Config.Logs.SaslPassword
		coreconfig.Config.Logs.Config.Net.SASL.Mechanism = sarama.SASLMechanism(coreconfig.Config.Logs.SaslMechanism)
		coreconfig.Config.Logs.Config.Net.SASL.Version = coreconfig.Config.Logs.SaslVersion
		coreconfig.Config.Logs.Config.Net.SASL.Handshake = coreconfig.Config.Logs.SaslHandshake
		coreconfig.Config.Logs.Config.Net.SASL.AuthIdentity = coreconfig.Config.Logs.SaslAuthIdentity

		if coreconfig.Config.Logs.Config.Net.SASL.Mechanism == sarama.SASLTypeSCRAMSHA256 {
			coreconfig.Config.Logs.Config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &XDGSCRAMClient{HashGeneratorFcn: SHA256}
			}
		}
		if coreconfig.Config.Logs.Config.Net.SASL.Mechanism == sarama.SASLTypeSCRAMSHA512 {
			coreconfig.Config.Logs.Config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient { return &XDGSCRAMClient{HashGeneratorFcn: SHA512} }
		}
	}

	if len(coreconfig.Config.Logs.KafkaVersion) != 0 {
		for _, v := range sarama.SupportedVersions {
			if v.String() == coreconfig.Config.Logs.KafkaVersion {
				coreconfig.Config.Logs.Config.Version = v
				break
			}
		}
	}
	if util.Debug() {
		log.Printf("D! saram config: %+v", coreconfig.Config.Logs.Config)
	}

	brokers := strings.Split(endpoint.Addr, ",")
	c, err := New(typ, brokers, coreconfig.Config.Logs.Config)
	if err != nil {
		panic(err)
	}

	d := &Destination{
		topic:               endpoint.Topic,
		brokers:             brokers,
		apiKey:              endpoint.APIKey,
		contentType:         contentType,
		contentEncoding:     buildContentEncoding(endpoint),
		client:              c,
		destinationsContext: destinationsContext,
		climit:              make(chan struct{}, maxConcurrentBackgroundSends),
		backoff:             policy,
		protocol:            endpoint.Protocol,
		origin:              endpoint.Origin,
	}
	return d
}

func (d *Destination) Close() {
	d.client.Close()
}

func errorToTag(err error) string {
	if err == nil {
		return "none"
	} else if _, ok := err.(*client.RetryableError); ok {
		return "retryable"
	} else {
		return "non-retryable"
	}
}

// Send sends a payload over kafka,
// the error returned can be retryable and it is the responsibility of the callee to retry.
func (d *Destination) Send(payload []byte) error {
	if d.blockedUntil.After(time.Now()) {
		d.waitForBackoff()
	}

	err := d.unconditionalSend(payload)

	if _, ok := err.(*client.RetryableError); ok {
		d.nbErrors = d.backoff.IncError(d.nbErrors)
	} else {
		d.nbErrors = d.backoff.DecError(d.nbErrors)
	}

	d.blockedUntil = time.Now().Add(d.backoff.GetBackoffDuration(d.nbErrors))

	return err
}

func (d *Destination) unconditionalSend(payload []byte) (err error) {
	ctx := d.destinationsContext.Context()

	encodedPayload, err := d.contentEncoding.encode(payload)
	if err != nil {
		return err
	}
	topic := d.topic
	data := &Data{}
	err = json.Unmarshal(payload, data)
	if err != nil {
		log.Println("E! get topic from payload, ", err)
	}
	if data.Topic != "" {
		topic = data.Topic
	}
	msgKey := d.apiKey
	if data.MsgKey != "" {
		msgKey = data.MsgKey
	}
	err = NewBuilder().WithMessage(msgKey, encodedPayload).WithTopic(topic).Send(d.client)
	if err != nil {
		log.Printf("W! send message to kafka error %s, topic:%s", err, topic)
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		// most likely a network or a connect error, the callee should retry.
		return client.NewRetryableError(err)
	}
	return nil
}

// SendAsync sends a payload in background.
func (d *Destination) SendAsync(payload []byte) {
	d.once.Do(func() {
		payloadChan := make(chan []byte, coreconfig.ChanSize())
		d.sendInBackground(payloadChan)
		d.payloadChan = payloadChan
	})
	d.payloadChan <- payload
}

// sendInBackground sends all payloads from payloadChan in background.
func (d *Destination) sendInBackground(payloadChan chan []byte) {
	ctx := d.destinationsContext.Context()
	go func() {
		for {
			select {
			case payload := <-payloadChan:
				// if the channel is non-buffered then there is no concurrency and we block on sending each payload
				if cap(d.climit) == 0 {
					d.unconditionalSend(payload) //nolint:errcheck
					break
				}
				d.climit <- struct{}{}
				go func() {
					d.unconditionalSend(payload) //nolint:errcheck
					<-d.climit
				}()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func buildContentEncoding(endpoint logsconfig.Endpoint) ContentEncoding {
	return IdentityContentType
}

func (d *Destination) waitForBackoff() {
	ctx, cancel := context.WithDeadline(d.destinationsContext.Context(), d.blockedUntil)
	defer cancel()
	<-ctx.Done()
}
