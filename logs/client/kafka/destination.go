//go:build !no_logs

package kafka

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	json "github.com/mailru/easyjson"

	coreconfig "flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/client"
	"flashcat.cloud/categraf/pkg/backoff"
)

// ContentType options,
const (
	TextContentType = "text/plain"
	JSONContentType = "application/json"
)

// HTTP errors.
var (
	errClient = errors.New("client error")
	errServer = errors.New("server error")
)

// emptyPayload is an empty payload used to check HTTP connectivity without sending logs.
var emptyPayload []byte

// Destination sends a payload over HTTP.
type Destination struct {
	topic   string
	brokers []string

	apiKey              string
	contentType         string
	contentEncoding     ContentEncoding
	client              sarama.SyncProducer
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
	return newDestination(endpoint, contentType, destinationsContext, time.Second*10, maxConcurrentBackgroundSends)
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

	if coreconfig.Config.Logs.Config == nil {
		coreconfig.Config.Logs.Config = sarama.NewConfig()
		coreconfig.Config.Logs.Producer.Partitioner = sarama.NewRandomPartitioner
		coreconfig.Config.Logs.Producer.Return.Successes = true
	}

	brokers := strings.Split(endpoint.Addr, ",")
	c, err := sarama.NewSyncProducer(brokers, coreconfig.Config.Logs.Config)
	if err != nil {
		panic(err)
	}
	return &Destination{
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

// Send sends a payload over HTTP,
// the error returned can be retryable and it is the responsibility of the callee to retry.
func (d *Destination) Send(payload []byte) error {
	if d.blockedUntil.After(time.Now()) {
		// log.Printf("%s: sleeping until %v before retrying\n", d.url, d.blockedUntil)
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
	err = NewBuilder().WithMessage(d.apiKey, encodedPayload).WithTopic(topic).Send(d.client)
	if err != nil {
		log.Printf("W! send message to kafka error %s, topic:%s", err, topic)
		if ctx.Err() == context.Canceled {
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
		payloadChan := make(chan []byte, logsconfig.ChanSize)
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
	if endpoint.UseCompression {
		return NewGzipContentEncoding(endpoint.CompressionLevel)
	}
	return IdentityContentType
}

func (d *Destination) waitForBackoff() {
	ctx, cancel := context.WithDeadline(d.destinationsContext.Context(), d.blockedUntil)
	defer cancel()
	<-ctx.Done()
}
