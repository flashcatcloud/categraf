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
	Trace                           *bool           `toml:"trace"`
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
	fields := make(map[string]interface{})
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

	// Start Timer
	start := time.Now()
	dns_time := start
	conn_time := start
	tls_time := start
	first_res_time := start

	if ins.Trace != nil && *ins.Trace {
		trace := &httptrace.ClientTrace{
			// request
			DNSDone: func(info httptrace.DNSDoneInfo) {
				dns_time = time.Now()
				fields["dns_time"] = time.Since(start).Milliseconds()
			},
			ConnectDone: func(network, addr string, err error) {
				conn_time = time.Now()
				tags["remote_addr"] = addr
				fields["connect_time"] = time.Since(dns_time).Milliseconds()
			},
			TLSHandshakeDone: func(info tls.ConnectionState, err error) {
				tls_time = time.Now()
				fields["tls_time"] = time.Since(conn_time).Milliseconds()
			},
			GotFirstResponseByte: func() {
				first_res_time = time.Now()
				if tls_time == start {
					fields["first_response_time"] = time.Since(conn_time).Milliseconds()
				} else {
					fields["first_response_time"] = time.Since(tls_time).Milliseconds()
				}
			},
		}
		request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	}
	resp, err := ins.client.Do(request)

	// metric: response_time
	fields["end_response_time"] = time.Since(first_res_time).Milliseconds()
	fields["response_time"] = time.Since(start).Seconds()
	fields["response_time_ms"] = time.Since(start).Milliseconds()

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

	// check tls cert
	if strings.HasPrefix(target, "https://") && resp.TLS != nil {
		fields["cert_expire_timestamp"] = getEarliestCertExpiry(resp.TLS).Unix()
		tags["cert_name"] = getCertName(resp.TLS)
	}

	defer resp.Body.Close()

	// metric: response_code
	fields["response_code"] = resp.StatusCode

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("E! failed to read response body:", err, "target:", target)
		return tags, fields, nil
	}

	if len(ins.ExpectResponseSubstring) > 0 && !strings.Contains(string(bs), ins.ExpectResponseSubstring) {
		log.Println("E! body mismatch, response body:", string(bs))
		fields["result_code"] = BodyMismatch
	}

	if ins.regularExpression != nil && !ins.regularExpression.Match(bs) {
		log.Println("E! body mismatch, response body:", string(bs))
		fields["result_code"] = BodyMismatch
	}

	if ins.ExpectResponseStatusCode != nil && *ins.ExpectResponseStatusCode != resp.StatusCode ||
		len(ins.ExpectResponseStatusCodes) > 0 && !strings.Contains(ins.ExpectResponseStatusCodes, fmt.Sprintf("%d", resp.StatusCode)) {
		log.Println("E! status code mismatch, response stats code:", resp.StatusCode)
		fields["result_code"] = CodeMismatch
	}

	return tags, fields, nil
}
