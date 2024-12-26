package appdynamics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"text/template"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/stringx"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

type (
	Instance struct {
		config.InstanceConfig

		config.HTTPProxy
		URLBase   string              `toml:"url_base"`
		URLVars   []map[string]string `toml:"url_vars"`
		URLVarKey []string            `toml:"url_var_label_keys"`

		URLs            []string          `toml:"-"`
		Headers         map[string]string `toml:"headers"`
		Method          string            `toml:"method"`
		FollowRedirects bool              `toml:"follow_redirects"`
		Username        string            `toml:"username"`
		Password        string            `toml:"password"`

		Period    config.Duration `toml:"period"`
		Delay     config.Duration `toml:"delay"`
		Timeout   config.Duration `toml:"timeout"`
		Precision string          `toml:"precision"`

		Filters []string `toml:"filters"`

		RequestInflight      int `toml:"request_inflight"`
		ForceRequestInflight int `toml:"force_request_inflight"`

		tls.ClientConfig
		client *http.Client `toml:"-"`

		config.UrlLabel
	}
	Metric struct {
		ID   int    `json:"metricId"`
		Name string `json:"metricName"`
		Path string `json:"metricPath"`

		Values []MetricValue `json:"metricValues"`
	}
	MetricValue struct {
		Timestamp int64 `json:"startTimeInMillis"`

		Current float64 `json:"current"`
		Min     float64 `json:"min"`
		Max     float64 `json:"max"`
		Count   float64 `json:"count"`
		Sum     float64 `json:"sum"`
		Value   float64 `json:"value"`

		StandardDiv float64 `json:"standardDeviation"`
	}
)

var _ inputs.SampleGatherer = new(Instance)
var replacer = strings.NewReplacer(":", "_", "{", "_", "}", "_", "[", "_", "]", "_", "|", "_", "(", "_", ")",
	"_", " ", "_", "%", "_", "/", "_per_")

func (ins *Instance) Drop() {
}

func (ins *Instance) prepare() error {
	tmpl, err := template.New("appdynamics").Parse(ins.URLBase)
	if err != nil {
		e := fmt.Errorf("failed to parse url template, error: %s", err)
		log.Println(e)
		return e
	}

	var buf bytes.Buffer
	for _, vars := range ins.URLVars {
		buf.Reset()
		err = tmpl.Execute(&buf, vars)
		if err != nil {
			e := fmt.Errorf("failed to prepare url template, error: %s", err)
			log.Println(e)
			return e
		}
		target := buf.String()
		addr, err := url.Parse(target)
		if err != nil {
			e := fmt.Errorf("failed to parse http(s) url: %s, error: %v", target, err)
			log.Println(e)
			return e
		}

		if addr.Scheme != "http" && addr.Scheme != "https" {
			e := fmt.Errorf("only http and https are supported, url: %s", target)
			log.Println(e)
			return e
		}

		addr.RawQuery = url.PathEscape(addr.RawQuery)
		ins.URLs = append(ins.URLs, addr.String())
	}
	return nil
}

func (ins *Instance) Init() error {
	if ins == nil ||
		len(ins.URLBase) == 0 {
		return types.ErrInstancesEmpty
	}
	err := ins.prepare()
	if err != nil {
		return err
	}

	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(5 * time.Second)
	}
	if ins.Delay == 0 {
		ins.Delay = config.Duration(1 * time.Minute)
	}
	if ins.Period == 0 {
		ins.Period = config.Duration(1 * time.Minute)
	}
	if ins.RequestInflight == 0 {
		ins.RequestInflight = 10
	}
	if ins.RequestInflight > 100 {
		ins.RequestInflight = 60
	}
	if ins.ForceRequestInflight > 0 {
		ins.RequestInflight = ins.ForceRequestInflight
	}
	c, err := ins.createHTTPClient()
	if err != nil {
		return err
	}
	ins.client = c

	if err := ins.PrepareUrlTemplate(); err != nil {
		return err
	}

	return nil
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{}

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
		Timeout:   time.Duration(ins.Timeout),
	}

	if !ins.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client, nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.URLs) == 0 {
		return
	}

	wg := new(sync.WaitGroup)
	for idx, target := range ins.URLs {
		wg.Add(1)
		go func(idx int, target string) {
			defer wg.Done()
			labels := map[string]string{}
			for k, v := range ins.URLVars[idx] {
				if len(ins.URLVarKey) == 0 {
					labels[k] = v
				}
				for _, key := range ins.URLVarKey {
					if k == key {
						labels[k] = v
					}
				}
			}
			ins.gather(slist, target, labels)
		}(idx, target)
	}
	wg.Wait()
}

func (ins *Instance) gather(slist *types.SampleList, link string, labels map[string]string) {
	now := time.Now()
	end := now.Add(-1 * time.Duration(ins.Delay))
	start := end.Add(-1 * time.Duration(ins.Period))
	e := end.Unix()
	e = e - e%60
	s := start.Unix()
	s = s - s%60
	tm := time.Unix(s, 0)
	if ins.Precision == "ms" {
		e = e * 1000
		s = s * 1000
	}

	link = strings.Replace(link, "$START_TIME", fmt.Sprintf("%d", s), -1)
	link = strings.Replace(link, "$END_TIME", fmt.Sprintf("%d", e), -1)
	u, err := url.Parse(link)
	if err != nil {
		log.Println("E! failed to parse url:", link, "error:", err)
		return
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		log.Println("E! failed to new request for url:", u.String(), "error:", err)
		return
	}

	ins.setHeaders(req)

	gTags, err := ins.GenerateLabel(u)
	if err != nil {
		log.Println("E! failed to generate url label value:", err)
		return
	}
	for k, v := range gTags {
		labels[k] = v
	}

	res, err := ins.client.Do(req)
	if err != nil {
		slist.PushFront(types.NewSample("", "up", 0, labels).SetTime(tm))
		log.Println("E! failed to query url:", u.String(), "error:", err)
		return
	}

	if res.StatusCode != http.StatusOK {
		slist.PushFront(types.NewSample("", "up", 0, labels).SetTime(tm))
		log.Println("E! failed to query url:", u.String(), "status code:", res.StatusCode)
		return
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		slist.PushFront(types.NewSample("", "up", 0, labels).SetTime(tm))
		log.Println("E! failed to read response body, url:", u.String(), "error:", err)
		return
	}

	slist.PushFront(types.NewSample("", "up", 1, labels).SetTime(tm))
	metrics := []Metric{}
	err = json.Unmarshal(body, &metrics)
	if err != nil {
		log.Printf("E! failed to unmarshal response body %s, url:%s, error:%s", body, u.String(), err)
	}
	for _, metric := range metrics {
		name := metric.Path
		names := strings.Split(metric.Path, "|")
		if len(names) > 0 {
			name = names[len(names)-1]
		}

		name = stringx.SnakeCase(replacer.Replace(name))

		labels["metric_id"] = fmt.Sprintf("%v", metric.ID)
		for _, val := range metric.Values {
			for _, filter := range ins.Filters {
				if filter == "current" {
					slist.PushFront(types.NewSample(inputName, name+"_current", val.Current, labels).SetTime(tm))
				}
				if filter == "max" {
					slist.PushFront(types.NewSample(inputName, name+"_max", val.Max, labels).SetTime(tm))
				}
				if filter == "min" {
					slist.PushFront(types.NewSample(inputName, name+"_min", val.Min, labels).SetTime(tm))
				}
				if filter == "count" {
					slist.PushFront(types.NewSample(inputName, name+"_count", val.Count, labels).SetTime(tm))
				}
				if filter == "sum" {
					slist.PushFront(types.NewSample(inputName, name+"_sum", val.Sum, labels).SetTime(tm))
				}
				if filter == "value" {
					slist.PushFront(types.NewSample(inputName, name+"_value", val.Value, labels).SetTime(tm))
				}
			}
			if len(ins.Filters) == 0 {
				log.Printf("W! no filter specified, use default filter: current")
				slist.PushFront(types.NewSample(inputName, name+"_current", val.Current, labels).SetTime(tm))
			}
		}
	}
}

func (ins *Instance) setHeaders(req *http.Request) {
	if ins.Username != "" && ins.Password != "" {
		req.SetBasicAuth(ins.Username, ins.Password)
	}

	if len(ins.Headers) == 0 {
		return
	}
	for k, v := range ins.Headers {
		req.Header.Set(k, v)
	}
}
