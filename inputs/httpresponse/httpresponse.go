package httpresponse

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/httpx"
	"flashcat.cloud/categraf/pkg/netx"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const (
	inputName = "httpresponse"

	Success          uint64 = 0
	ConnectionFailed uint64 = 1
	Timeout          uint64 = 2
	DNSError         uint64 = 3
	AddressError     uint64 = 4
	BodyMismatch     uint64 = 5
	CodeMismatch     uint64 = 6
)

type Instance struct {
	Targets                  []string          `toml:"targets"`
	Labels                   map[string]string `toml:"labels"`
	IntervalTimes            int64             `toml:"interval_times"`
	HTTPProxy                string            `toml:"http_proxy"`
	Interface                string            `toml:"interface"`
	Method                   string            `toml:"method"`
	ResponseTimeout          config.Duration   `toml:"response_timeout"`
	FollowRedirects          bool              `toml:"follow_redirects"`
	Username                 string            `toml:"username"`
	Password                 string            `toml:"password"`
	Headers                  []string          `toml:"headers"`
	Body                     string            `toml:"body"`
	ExpectResponseSubstring  string            `toml:"expect_response_substring"`
	ExpectResponseStatusCode *int              `toml:"expect_response_status_code"`

	tls.ClientConfig
	client httpClient
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (ins *Instance) Init() error {
	if ins.ResponseTimeout < config.Duration(time.Second) {
		ins.ResponseTimeout = config.Duration(time.Second * 3)
	}

	if ins.Method == "" {
		ins.Method = "GET"
	}

	if len(ins.Targets) == 0 {
		return errors.New("http_response targets empty")
	}

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

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             httpx.GetProxyFunc(ins.HTTPProxy),
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
			TLSClientConfig:   tlsCfg,
		},
		Timeout: time.Duration(ins.ResponseTimeout),
	}

	if !ins.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client, nil
}

type HTTPResponse struct {
	config.Interval
	Instances []*Instance `toml:"instances"`
	Counter   uint64
	wg        sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &HTTPResponse{}
	})
}

func (h *HTTPResponse) Prefix() string {
	return inputName
}

func (h *HTTPResponse) Init() error {
	if len(h.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(h.Instances); i++ {
		if err := h.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (h *HTTPResponse) Drop() {}

func (h *HTTPResponse) Gather(slist *list.SafeList) {
	atomic.AddUint64(&h.Counter, 1)
	for i := range h.Instances {
		ins := h.Instances[i]
		h.wg.Add(1)
		go h.gatherOnce(slist, ins)
	}
	h.wg.Wait()
}

func (h *HTTPResponse) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer h.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&h.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	if config.Config.DebugMode {
		if len(ins.Targets) == 0 {
			log.Println("D! http_response targets empty")
		}
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

func (ins *Instance) gather(slist *list.SafeList, target string) {
	if config.Config.DebugMode {
		log.Println("D! http_response... target:", target)
	}

	labels := map[string]string{"target": target}
	for k, v := range ins.Labels {
		labels[k] = v
	}

	fields := map[string]interface{}{}

	defer func() {
		for field, value := range fields {
			slist.PushFront(inputs.NewSample(field, value, labels))
		}
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

	for i := 0; i < len(ins.Headers); i += 2 {
		request.Header.Add(ins.Headers[i], ins.Headers[i+1])
		if ins.Headers[i] == "Host" {
			request.Host = ins.Headers[i+1]
		}
	}

	if ins.Username != "" || ins.Password != "" {
		request.SetBasicAuth(ins.Username, ins.Password)
	}

	// Start Timer
	start := time.Now()
	resp, err := ins.client.Do(request)

	// metric: response_time
	fields["response_time"] = time.Since(start).Seconds()

	// If an error in returned, it means we are dealing with a network error, as
	// HTTP error codes do not generate errors in the net/http library
	if err != nil {
		if config.Config.DebugMode {
			log.Println("D! network error while polling:", target, "error:", err)
		}

		// metric: result_code
		fields["result_code"] = ConnectionFailed

		if timeoutError, ok := err.(net.Error); ok && timeoutError.Timeout() {
			fields["result_code"] = Timeout
			return tags, fields, nil
		}

		if urlErr, isURLErr := err.(*url.Error); isURLErr {
			if opErr, isNetErr := (urlErr.Err).(*net.OpError); isNetErr {
				switch (opErr.Err).(type) {
				case *net.DNSError:
					fields["result_code"] = DNSError
					return tags, fields, nil
				case *net.ParseError:
					// Parse error has to do with parsing of IP addresses, so we
					// group it with address errors
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
	}

	defer resp.Body.Close()

	// metric: response_code
	fields["response_code"] = resp.StatusCode

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("E! failed to read response body:", err)
		return tags, fields, nil
	}

	if len(ins.ExpectResponseSubstring) > 0 {
		if !strings.Contains(string(bs), ins.ExpectResponseSubstring) {
			fields["result_code"] = BodyMismatch
		}
	}

	if ins.ExpectResponseStatusCode != nil {
		if *ins.ExpectResponseStatusCode != resp.StatusCode {
			fields["result_code"] = CodeMismatch
		}
	}

	return tags, fields, nil
}
