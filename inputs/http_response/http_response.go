package http_response

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/httpx"
	"flashcat.cloud/categraf/pkg/netx"
	"flashcat.cloud/categraf/types"
)

const (
	inputName = "http_response"

	Success          uint64 = 0
	ConnectionFailed uint64 = 1
	Timeout          uint64 = 2
	DNSError         uint64 = 3
	AddressError     uint64 = 4
	BodyMismatch     uint64 = 5
	CodeMismatch     uint64 = 6
)

type requestTiming struct {
	reqStart     time.Time
	dnsStart     time.Time
	dnsEnd       time.Time
	connStart    time.Time
	connEnd      time.Time
	tlsHandStart time.Time
	tlsHandEnd   time.Time
	gotFirstByte time.Time
	reqEnd       time.Time
	remoteAddr   string
	connReused   bool
}

func (t *requestTiming) Trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			t.dnsStart = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			t.dnsEnd = time.Now()
		},
		ConnectStart: func(string, string) {
			t.connStart = time.Now()
		},
		ConnectDone: func(_, addr string, _ error) {
			t.connEnd = time.Now()
			t.remoteAddr = addr
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.connReused = info.Reused
		},
		TLSHandshakeStart: func() {
			t.tlsHandStart = time.Now()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.tlsHandEnd = time.Now()
		},
		GotFirstResponseByte: func() {
			t.gotFirstByte = time.Now()
		},
	}
}

func (t *requestTiming) PopulateFields(fields map[string]interface{}, end time.Time) {
	dnsRequest := elapsedMilliseconds(t.dnsStart, t.dnsEnd)
	tcpConnect := elapsedMilliseconds(t.connStart, t.connEnd)
	tlsHandshake := elapsedMilliseconds(t.tlsHandStart, t.tlsHandEnd)
	firstByte := elapsedMilliseconds(t.reqStart, t.gotFirstByte)

	fields["dns_request"] = dnsRequest
	fields["tcp_connect"] = tcpConnect
	fields["tls_handshake"] = tlsHandshake
	fields["first_byte"] = firstByte
	fields["total_cost"] = elapsedMilliseconds(t.reqStart, end)

	fields["dns_time"] = dnsRequest
	fields["connect_time"] = tcpConnect
	fields["tls_time"] = tlsHandshake
	fields["first_response_time"] = firstByte
	fields["end_response_time"] = elapsedMilliseconds(t.gotFirstByte, end)
}

func elapsedMilliseconds(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return -1
	}
	return end.Sub(start).Milliseconds()
}

type Instance struct {
	config.InstanceConfig

	Targets                         []string        `toml:"targets"`
	Interface                       string          `toml:"interface"`
	ResponseTimeout                 config.Duration `toml:"response_timeout"`
	Headers                         []string        `toml:"headers"`
	Body                            string          `toml:"body"`
	ExpectResponseSubstring         string          `toml:"expect_response_substring"`
	ExpectResponseRegularExpression string          `toml:"expect_response_regular_expression"`
	ExpectResponseStatusCode        *int            `toml:"expect_response_status_code"`
	ExpectResponseStatusCodes       string          `toml:"expect_response_status_codes"`
	config.HTTPProxy

	client httpClient
	config.HTTPCommonConfig

	// Mappings Set the mapping of extra tags in batches
	Mappings map[string]map[string]string `toml:"mappings"`

	regularExpression *regexp.Regexp `toml:"-"`
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (ins *Instance) Init() error {
	if len(ins.Targets) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.ResponseTimeout < config.Duration(time.Second) {
		ins.ResponseTimeout = config.Duration(time.Second * 3)
	}
	ins.InitHTTPClientConfig()
	ins.Timeout = ins.ResponseTimeout

	client, err := ins.createHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %v", err)
	}

	ins.client = client

	for _, target := range ins.Targets {
		addr, err := url.Parse(target)
		if err != nil {
			return fmt.Errorf("failed to parse http url: %s, error: %v", target, err)
		}

		if addr.Scheme != "http" && addr.Scheme != "https" {
			return fmt.Errorf("only http and https are supported, target: %s", target)
		}
	}
	if ins.HTTPCommonConfig.Headers == nil {
		ins.HTTPCommonConfig.Headers = make(map[string]string)
	}
	// compatible with old config
	for i := 0; i < len(ins.Headers); i += 2 {
		ins.HTTPCommonConfig.Headers[ins.Headers[i]] = ins.Headers[i+1]
	}
	if len(ins.ExpectResponseRegularExpression) > 0 {
		ins.regularExpression = regexp.MustCompile(ins.ExpectResponseRegularExpression)
	}

	return nil
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{}

	if ins.Interface != "" {
		dialer.LocalAddr, err = netx.LocalAddressByInterfaceName(ins.Interface)
		if err != nil {
			return nil, err
		}
	}

	proxy, err := ins.Proxy()
	if err != nil {
		return nil, err
	}

	client := httpx.CreateHTTPClient(httpx.TlsConfig(tlsCfg),
		httpx.NetDialer(dialer), httpx.Proxy(proxy),

		httpx.DisableKeepAlives(*ins.DisableKeepAlives),
		httpx.Timeout(time.Duration(ins.Timeout)),
		httpx.FollowRedirects(*ins.FollowRedirects))
	return client, err
}

type HTTPResponse struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`

	Mappings map[string]map[string]string `toml:"mappings"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &HTTPResponse{}
	})
}

func (h *HTTPResponse) Clone() inputs.Input {
	return &HTTPResponse{}
}

func (h *HTTPResponse) Name() string {
	return inputName
}

func (h *HTTPResponse) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(h.Instances))
	for i := 0; i < len(h.Instances); i++ {
		if len(h.Instances[i].Mappings) == 0 {
			h.Instances[i].Mappings = h.Mappings
		} else {
			m := make(map[string]map[string]string)
			for k, v := range h.Mappings {
				m[k] = v
			}
			for k, v := range h.Instances[i].Mappings {
				m[k] = v
			}
			h.Instances[i].Mappings = m
		}
		ret[i] = h.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.Targets) == 0 {
		return
	}

	wg := new(sync.WaitGroup)
	for _, target := range ins.Targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			ins.gather(slist, target)
		}(target)
	}
	wg.Wait()
}

func (ins *Instance) gather(slist *types.SampleList, target string) {
	if ins.DebugMod {
		log.Println("D! http_response... target:", target)
	}

	labels := map[string]string{"target": target}
	fields := map[string]interface{}{}
	// Add extra tags in batches
	if m, ok := ins.Mappings[target]; ok {
		for k, v := range m {
			labels[k] = v
		}
	}

	defer func() {
		certTag, lok := labels["cert_name"]
		if lok {
			delete(labels, "cert_name")
		}
		if certField, ok := fields["cert_expire_timestamp"]; ok {
			delete(fields, "cert_expire_timestamp")
			certLabel := map[string]string{}
			if lok {
				certLabel["cert_name"] = certTag
			}
			slist.PushSample(inputName, "cert_expire_timestamp", certField, labels, certLabel)
		}
		slist.PushSamples(inputName, fields, labels)
	}()

	var returnTags map[string]string
	var err error

	returnTags, fields, err = ins.httpGather(target)
	if err != nil {
		log.Println("E! failed to gather http target:", target, "error:", err)
	}

	for k, v := range returnTags {
		labels[k] = v
	}
}

func (ins *Instance) httpGather(target string) (map[string]string, map[string]interface{}, error) {
	// Prepare fields and tags
	fields := map[string]interface{}{
		"dns_request":         int64(-1),
		"tcp_connect":         int64(-1),
		"tls_handshake":       int64(-1),
		"first_byte":          int64(-1),
		"total_cost":          int64(-1),
		"dns_time":            int64(-1),
		"connect_time":        int64(-1),
		"tls_time":            int64(-1),
		"first_response_time": int64(-1),
		"end_response_time":   int64(-1),
		"response_time_ms":    int64(-1),
		"response_code":       -1,
	}
	tags := map[string]string{"method": ins.Method}

	var body io.Reader
	if ins.Body != "" {
		body = strings.NewReader(ins.Body)
	}

	request, err := http.NewRequest(ins.Method, target, body)
	if err != nil {
		return nil, nil, err
	}
	ins.SetHeaders(request)

	if ins.DebugMod {
		log.Printf("D! >>> %s %s", request.Method, target)
		log.Printf("D! >>> Request Headers: %v", request.Header)
		if ins.Body != "" {
			log.Printf("D! >>> Request Body: %s", ins.Body)
		}
	}

	timing := &requestTiming{}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), timing.Trace()))

	// Start Timer
	start := time.Now()
	timing.reqStart = start
	if ins.DebugMod {
		if cli, ok := ins.client.(*http.Client); ok {
			if transport, ok := cli.Transport.(*http.Transport); ok {
				if transport.MaxIdleConnsPerHost <= 2 {
					log.Printf("D! MaxIdleConnsPerHost is only %d for %s",
						transport.MaxIdleConnsPerHost, target)
				}
			}
		}
	}

	resp, err := ins.client.Do(request)

	// metric: response_time
	duration := time.Since(start)
	fields["response_time"] = duration.Seconds()
	fields["response_time_ms"] = duration.Milliseconds()
	fields["total_cost"] = duration.Milliseconds()
	timing.PopulateFields(fields, time.Now())
	if timing.remoteAddr != "" {
		tags["remote_addr"] = timing.remoteAddr
	}

	// If an error in returned, it means we are dealing with a network error, as
	// HTTP error codes do not generate errors in the net/http library
	if err != nil {
		log.Println("E! network error while polling:", target, "error:", err)

		// metric: result_code
		fields["result_code"] = ConnectionFailed

		var netError net.Error
		if errors.As(err, &netError) && netError.Timeout() {
			fields["result_code"] = Timeout
			return tags, fields, nil
		}

		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			var opErr *net.OpError
			if errors.As(urlErr, &opErr) {
				var dnsErr *net.DNSError
				var parseErr *net.ParseError
				if errors.As(opErr, &dnsErr) {
					fields["result_code"] = DNSError
					return tags, fields, nil
				} else if errors.As(opErr, &parseErr) {
					fields["result_code"] = AddressError
					return tags, fields, nil
				}
			}
		}
		return tags, fields, nil
	} else {
		fields["result_code"] = Success
	}
	timing.reqEnd = time.Now()
	timing.PopulateFields(fields, timing.reqEnd)
	if timing.remoteAddr != "" {
		tags["remote_addr"] = timing.remoteAddr
	}

	if ins.DebugMod {
		if duration > 200*time.Millisecond {
			log.Printf("D! SLOW: %s took %v, connection_reused=%v",
				target, duration, timing.connReused)
		}
	}

	// check tls cert
	if strings.HasPrefix(target, "https://") && resp.TLS != nil {
		fields["cert_expire_timestamp"] = getEarliestCertExpiry(resp.TLS).Unix()
		tags["cert_name"] = getCertName(resp.TLS)
	}

	defer resp.Body.Close()

	// metric: response_code
	fields["response_code"] = resp.StatusCode

	var bs []byte
	if ins.DebugMod || len(ins.ExpectResponseSubstring) > 0 || ins.regularExpression != nil {
		var err error
		bs, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Println("E! failed to read response body:", err)
			return tags, fields, nil
		}
	} else {
		// drain body to allow connection reuse
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Println("E! failed to drain response body:", err)
			return tags, fields, nil
		}
	}

	if ins.DebugMod {
		log.Printf("D! <<< %s", resp.Status)
		log.Printf("D! <<< Response Headers: %v", resp.Header)
		if len(bs) > 0 {
			log.Printf("D! <<< Response Body (%d bytes): %s", len(bs), responseBodySnippet(resp, bs))
		}
	}

	if len(ins.ExpectResponseSubstring) > 0 && !strings.Contains(string(bs), ins.ExpectResponseSubstring) ||
		ins.regularExpression != nil && !ins.regularExpression.Match(bs) {
		log.Println("E! body mismatch, response body:", responseBodySnippet(resp, bs))
		fields["result_code"] = BodyMismatch
	}

	if ins.ExpectResponseStatusCode != nil && *ins.ExpectResponseStatusCode != resp.StatusCode ||
		len(ins.ExpectResponseStatusCodes) > 0 && !strings.Contains(ins.ExpectResponseStatusCodes, fmt.Sprintf("%d", resp.StatusCode)) {
		log.Println("E! status code mismatch, response stats code:", resp.StatusCode)
		fields["result_code"] = CodeMismatch
	}

	return tags, fields, nil
}

func responseBodySnippet(resp *http.Response, bs []byte) string {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.HasPrefix(ct, "text/") &&
		!strings.Contains(ct, "json") &&
		!strings.Contains(ct, "xml") &&
		!strings.Contains(ct, "javascript") {
		return fmt.Sprintf("<binary response body, %d bytes>", len(bs))
	}

	const maxSnippetLen = 1024
	if len(bs) <= maxSnippetLen {
		return string(bs)
	}
	return string(bs[:maxSnippetLen]) + "...(truncated)"
}
