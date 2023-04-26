package nginx

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "nginx"

type Nginx struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Nginx{}
	})
}

func (ngx *Nginx) Clone() inputs.Input {
	return &Nginx{}
}

func (ngx *Nginx) Name() string {
	return inputName
}

func (ngx *Nginx) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(ngx.Instances))
	for i := 0; i < len(ngx.Instances); i++ {
		ret[i] = ngx.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	Urls []string `toml:"urls"`

	ResponseTimeout config.Duration `toml:"response_timeout"`
	FollowRedirects bool            `toml:"follow_redirects"`
	Username        string          `toml:"username"`
	Password        string          `toml:"password"`
	Headers         []string        `toml:"headers"`

	tls.ClientConfig

	client *http.Client
}

func (ins *Instance) Init() error {
	if len(ins.Urls) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.ResponseTimeout < config.Duration(time.Second) {
		ins.ResponseTimeout = config.Duration(time.Second * 5)
	}

	client, err := ins.createHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %v", err)
	}
	ins.client = client

	for _, u := range ins.Urls {
		addr, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("failed to parse the url: %s, error: %v", u, err)
		}

		if addr.Scheme != "http" && addr.Scheme != "https" {
			return fmt.Errorf("only http and https are supported, url: %s", u)
		}
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup

	if len(ins.Urls) == 0 {
		return
	}

	for _, u := range ins.Urls {
		addr, err := url.Parse(u)
		if err != nil {
			log.Println("E! failed to parse the url:", u, "error:", err)
			continue
		}
		wg.Add(1)
		go func(addr *url.URL) {
			defer wg.Done()
			if err := ins.gather(addr, slist); err != nil {
				log.Println("E!", err)
			}
		}(addr)
	}

	wg.Wait()
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	trans := &http.Transport{
		DisableKeepAlives: true,
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

func (ins *Instance) gather(addr *url.URL, slist *types.SampleList) error {
	if config.Config.DebugMode {
		log.Println("D! nginx... url:", addr)
	}

	var body io.Reader
	request, err := http.NewRequest("GET", addr.String(), body)
	if err != nil {
		return fmt.Errorf("failed to create an HTTP request, url: %s, error: %s", addr.String(), err)
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

	resp, err := ins.client.Do(request)
	if err != nil {
		return fmt.Errorf("failed to request the url: %s, error: %s", addr.String(), err)
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("E! failed to close the body of client:", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the HTTP response status exception, url: %s, status: %s", addr.String(), resp.Status)
	}
	r := bufio.NewReader(resp.Body)

	// Active connections
	_, err = r.ReadString(':')
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	active, err := strconv.ParseUint(strings.TrimSpace(line), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}

	// Server accepts handled requests
	_, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	data := strings.Fields(line)
	accepts, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}

	handled, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	requests, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}

	// Reading/Writing/Waiting
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	data = strings.Fields(line)
	reading, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	writing, err := strconv.ParseUint(data[3], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}
	waiting, err := strconv.ParseUint(data[5], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse the response, error: %s", err)
	}

	host, port, err := net.SplitHostPort(addr.Host)
	if err != nil {
		host = addr.Host
		if addr.Scheme == "http" {
			port = "80"
		} else if addr.Scheme == "https" {
			port = "443"
		} else {
			port = ""
		}
	}
	fields := map[string]interface{}{
		"active":   active,
		"accepts":  accepts,
		"handled":  handled,
		"requests": requests,
		"reading":  reading,
		"writing":  writing,
		"waiting":  waiting,
	}
	tags := map[string]string{
		"server": host,
		"port":   port,
	}

	slist.PushSamples("nginx", fields, tags)

	return nil
}
