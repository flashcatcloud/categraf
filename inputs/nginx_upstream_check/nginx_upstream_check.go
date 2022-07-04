package nginx_upstream_check

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/netx"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "nginx_upstream_check"

type NginxUpstreamCheck struct {
	config.Interval
	counter   uint64
	waitgrp   sync.WaitGroup
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NginxUpstreamCheck{}
	})
}

func (r *NginxUpstreamCheck) Prefix() string {
	return inputName
}

func (r *NginxUpstreamCheck) Init() error {
	if len(r.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(r.Instances); i++ {
		if err := r.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (r *NginxUpstreamCheck) Drop() {}

func (r *NginxUpstreamCheck) Gather(slist *list.SafeList) {
	atomic.AddUint64(&r.counter, 1)

	for i := range r.Instances {
		ins := r.Instances[i]

		r.waitgrp.Add(1)
		go func(slist *list.SafeList, ins *Instance) {
			defer r.waitgrp.Done()

			if ins.IntervalTimes > 0 {
				counter := atomic.LoadUint64(&r.counter)
				if counter%uint64(ins.IntervalTimes) != 0 {
					return
				}
			}

			ins.gatherOnce(slist)
		}(slist, ins)
	}

	r.waitgrp.Wait()
}

type Instance struct {
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	Targets         []string        `toml:"targets"`
	Interface       string          `toml:"interface"`
	Method          string          `toml:"method"`
	FollowRedirects bool            `toml:"follow_redirects"`
	Username        string          `toml:"username"`
	Password        string          `toml:"password"`
	Headers         []string        `toml:"headers"`
	Timeout         config.Duration `toml:"timeout"`
	config.HTTPProxy

	tls.ClientConfig
	client httpClient
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (ins *Instance) Init() error {
	if ins.Timeout < config.Duration(time.Second) {
		ins.Timeout = config.Duration(time.Second * 5)
	}

	if ins.Method == "" {
		ins.Method = "GET"
	}

	if len(ins.Targets) == 0 {
		return types.ErrInstancesEmpty
	}

	client, err := ins.createHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %v", err)
	}

	ins.client = client

	for _, target := range ins.Targets {
		addr, err := url.Parse(target)
		if err != nil {
			return fmt.Errorf("failed to parse target url: %s, error: %v", target, err)
		}

		if addr.Scheme != "http" && addr.Scheme != "https" {
			return fmt.Errorf("only http and https are supported, target: %s", target)
		}
	}

	if len(ins.Headers)%2 != 0 {
		return fmt.Errorf("headers invalid")
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
	}

	if ins.UseTLS {
		trans.TLSClientConfig = tlsCfg
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.Timeout),
	}

	if !ins.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client, nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) {
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

type NginxUpstreamCheckData struct {
	Servers struct {
		Total      uint64                     `json:"total"`
		Generation uint64                     `json:"generation"`
		Server     []NginxUpstreamCheckServer `json:"server"`
	} `json:"servers"`
}

type NginxUpstreamCheckServer struct {
	Index    uint64 `json:"index"`
	Upstream string `json:"upstream"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Rise     uint64 `json:"rise"`
	Fall     uint64 `json:"fall"`
	Type     string `json:"type"`
	Port     uint16 `json:"port"`
}

func (ins *Instance) gather(slist *list.SafeList, target string) {
	if config.Config.DebugMode {
		log.Println("D! nginx_upstream_check... target:", target)
	}

	labels := map[string]string{"target": target}
	for k, v := range ins.Labels {
		labels[k] = v
	}

	checkData := &NginxUpstreamCheckData{}

	err := ins.gatherJSONData(target, checkData)
	if err != nil {
		log.Println("E! failed to gather json data:", err)
		return
	}

	for _, server := range checkData.Servers.Server {
		tags := map[string]string{
			"upstream": server.Upstream,
			"type":     server.Type,
			"name":     server.Name,
			"port":     strconv.Itoa(int(server.Port)),
		}

		fields := map[string]interface{}{
			"status_code": getStatusCode(server.Status),
			"rise":        server.Rise,
			"fall":        server.Fall,
		}

		types.PushSamples(slist, fields, tags, labels)
	}
}

func getStatusCode(status string) uint8 {
	switch status {
	case "up":
		return 1
	case "down":
		return 2
	default:
		return 0
	}
}

// gatherJSONData query the data source and parse the response JSON
func (ins *Instance) gatherJSONData(address string, value interface{}) error {
	request, err := http.NewRequest(ins.Method, address, nil)
	if err != nil {
		return err
	}

	if ins.Username != "" || ins.Password != "" {
		request.SetBasicAuth(ins.Username, ins.Password)
	}

	for i := 0; i < len(ins.Headers); i += 2 {
		request.Header.Add(ins.Headers[i], ins.Headers[i+1])
		if ins.Headers[i] == "Host" {
			request.Host = ins.Headers[i+1]
		}
	}

	response, err := ins.client.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// ignore the err here; LimitReader returns io.EOF and we're not interested in read errors.
		body, _ := io.ReadAll(io.LimitReader(response.Body, 200))
		return fmt.Errorf("%s returned HTTP status %s: %q", address, response.Status, body)
	}

	err = json.NewDecoder(response.Body).Decode(value)
	if err != nil {
		return err
	}

	return nil
}
