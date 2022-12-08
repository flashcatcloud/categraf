//go:build !no_logs

package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/client"
	"flashcat.cloud/categraf/pkg/backoff"
	httputils "flashcat.cloud/categraf/pkg/httpx"
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
	url                 string
	apiKey              string
	contentType         string
	host                string
	contentEncoding     ContentEncoding
	client              *httputils.ResetClient
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

	return &Destination{
		host:                endpoint.Host,
		url:                 buildURL(endpoint),
		apiKey:              endpoint.APIKey,
		contentType:         contentType,
		contentEncoding:     buildContentEncoding(endpoint),
		client:              httputils.NewResetClient(endpoint.ConnectionResetInterval, httpClientFactory(timeout)),
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

	req, err := http.NewRequest("POST", d.url, bytes.NewReader(encodedPayload))
	if err != nil {
		// the request could not be built,
		// this can happen when the method or the url are valid.
		return err
	}
	req.Header.Set("User-Agent", "categraf")
	req.Header.Set("CATEGRAF-API-KEY", d.apiKey)
	req.Header.Set("Content-Type", d.contentType)
	req.Header.Set("Content-Encoding", d.contentEncoding.name())
	if d.protocol != "" {
		req.Header.Set("CATEGRAF-PROTOCOL", string(d.protocol))
	}
	if d.origin != "" {
		req.Header.Set("CATEGRAF-ORIGIN", string(d.origin))
		// TODO agentversion
		req.Header.Set("CATEGRAF-ORIGIN-VERSION", "0.0.1")
	}
	req = req.WithContext(ctx)

	resp, err := d.client.Do(req)

	if err != nil {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}
		// most likely a network or a connect error, the callee should retry.
		return client.NewRetryableError(err)
	}

	defer resp.Body.Close()
	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// the read failed because the server closed or terminated the connection
		// *after* serving the request.
		return err
	}
	if resp.StatusCode >= 400 {
		log.Printf("W! failed to post http payload. code=%d host=%s response=%s\n", resp.StatusCode, d.host, string(response))
	}
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		// the server could not serve the request, most likely because of an
		// internal error or, (429) because it is overwhelmed
		return client.NewRetryableError(errServer)
	} else if resp.StatusCode >= 400 {
		// the logs-agent is likely to be misconfigured,
		// the URL or the API key may be wrong.
		return errClient
	} else {
		return nil
	}
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

func httpClientFactory(timeout time.Duration) func() *http.Client {
	return func() *http.Client {
		return &http.Client{
			Timeout: timeout,
			// reusing core agent HTTP transport to benefit from proxy settings.
			Transport: httputils.CreateHTTPTransport(),
		}
	}
}

// buildURL buils a url from a logsconfig endpoint.
func buildURL(endpoint logsconfig.Endpoint) string {
	var scheme string
	if endpoint.UseSSL {
		scheme = "https"
	} else {
		scheme = "http"
	}
	var address string
	if endpoint.Port != 0 {
		address = fmt.Sprintf("%v:%v", endpoint.Host, endpoint.Port)
	} else {
		address = endpoint.Host
	}
	url := url.URL{
		Scheme: scheme,
		Host:   address,
	}
	if endpoint.Version == logsconfig.EPIntakeVersion2 && endpoint.TrackType != "" {
		url.Path = fmt.Sprintf("/api/v2/%s", endpoint.TrackType)
	} else {
		url.Path = "/v1/input"
	}
	return url.String()
}

func buildContentEncoding(endpoint logsconfig.Endpoint) ContentEncoding {
	if endpoint.UseCompression {
		return NewGzipContentEncoding(endpoint.CompressionLevel)
	}
	return IdentityContentType
}

// CheckConnectivity check if sending logs through HTTP works
func CheckConnectivity(endpoint logsconfig.Endpoint) logsconfig.HTTPConnectivity {
	log.Println("I! Checking HTTP connectivity...")
	ctx := client.NewDestinationsContext()
	ctx.Start()
	defer ctx.Stop()
	// Lower the timeout to 5s because HTTP connectivity test is done synchronously during the agent bootstrap sequence
	destination := newDestination(endpoint, JSONContentType, ctx, time.Second*5, 0)
	log.Println("I! Sending HTTP connectivity request to", destination.url)
	err := destination.unconditionalSend(emptyPayload)
	if err != nil {
		log.Println("E! HTTP connectivity failure:", err)
	} else {
		log.Println("HTTP connectivity successful")
	}
	return err == nil
}

func (d *Destination) waitForBackoff() {
	ctx, cancel := context.WithDeadline(d.destinationsContext.Context(), d.blockedUntil)
	defer cancel()
	<-ctx.Done()
}
