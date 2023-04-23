package http_response

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/netx"
	"flashcat.cloud/categraf/pkg/tls"
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

	Targets                  []string        `toml:"targets"`
	Interface                string          `toml:"interface"`
	Method                   string          `toml:"method"`
	ResponseTimeout          config.Duration `toml:"response_timeout"`
	FollowRedirects          bool            `toml:"follow_redirects"`
	Username                 string          `toml:"username"`
	Password                 string          `toml:"password"`
	Headers                  []string        `toml:"headers"`
	Body                     string          `toml:"body"`
	ExpectResponseSubstring  string          `toml:"expect_response_substring"`
	ExpectResponseStatusCode *int            `toml:"expect_response_status_code"`
	config.HTTPProxy

	tls.ClientConfig
	client httpClient
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

	if ins.Method == "" {
		ins.Method = "GET"
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

	proxy, err := ins.Proxy()
	if err != nil {
		return nil, err
	}

	trans := &http.Transport{
		Proxy:             proxy,
		DialContext:       dialer.DialContext,
		DisableKeepAlives: true,
		TLSClientConfig:   tlsCfg,
	}

	if ins.UseTLS {
		trans.TLSClientConfig = tlsCfg
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.ResponseTimeout),
	}

	if !ins.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client, nil
}

type HTTPResponse struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
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
	if config.Config.DebugMode {
		log.Println("D! http_response... target:", target)
	}

	labels := map[string]string{"target": target}
	fields := map[string]interface{}{}

	defer func() {
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
		log.Println("E! network error while polling:", target, "error:", err)

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
			log.Println("E! body mismatch, response body:", string(bs))
			fields["result_code"] = BodyMismatch
		}
	}

	if ins.ExpectResponseStatusCode != nil {
		if *ins.ExpectResponseStatusCode != resp.StatusCode {
			log.Println("E! status code mismatch, response stats code:", resp.StatusCode)
			fields["result_code"] = CodeMismatch
		}
	}

	return tags, fields, nil
}
