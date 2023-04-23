package prometheus

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "prometheus"
const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,*/*;q=0.1`

type Instance struct {
	config.InstanceConfig

	URLs              []string        `toml:"urls"`
	ConsulConfig      ConsulConfig    `toml:"consul"`
	NamePrefix        string          `toml:"name_prefix"`
	BearerTokenString string          `toml:"bearer_token_string"`
	BearerTokeFile    string          `toml:"bearer_token_file"`
	Username          string          `toml:"username"`
	Password          string          `toml:"password"`
	Timeout           config.Duration `toml:"timeout"`
	IgnoreMetrics     []string        `toml:"ignore_metrics"`
	IgnoreLabelKeys   []string        `toml:"ignore_label_keys"`
	Headers           []string        `toml:"headers"`

	config.UrlLabel

	ignoreMetricsFilter   filter.Filter
	ignoreLabelKeysFilter filter.Filter
	tls.ClientConfig
	client *http.Client
}

func (ins *Instance) Empty() bool {
	if len(ins.URLs) > 0 {
		return false
	}

	if ins.ConsulConfig.Enabled && len(ins.ConsulConfig.Queries) > 0 {
		return false
	}

	return true
}

func (ins *Instance) Init() error {
	if ins.Empty() {
		return types.ErrInstancesEmpty
	}

	if ins.ConsulConfig.Enabled && len(ins.ConsulConfig.Queries) > 0 {
		if err := ins.InitConsulClient(); err != nil {
			return err
		}
	}

	for i, u := range ins.URLs {
		ins.URLs[i] = strings.Replace(u, "$hostname", config.Config.GetHostname(), -1)
		ins.URLs[i] = strings.Replace(u, "$ip", config.Config.Global.IP, -1)
		ins.URLs[i] = os.Expand(u, config.GetEnv)
	}

	if ins.Timeout <= 0 {
		ins.Timeout = config.Duration(time.Second * 3)
	}

	client, err := ins.createHTTPClient()
	if err != nil {
		return err
	} else {
		ins.client = client
	}

	if len(ins.IgnoreMetrics) > 0 {
		ins.ignoreMetricsFilter, err = filter.Compile(ins.IgnoreMetrics)
		if err != nil {
			return err
		}
	}

	if len(ins.IgnoreLabelKeys) > 0 {
		ins.ignoreLabelKeysFilter, err = filter.Compile(ins.IgnoreLabelKeys)
		if err != nil {
			return err
		}
	}

	if err := ins.PrepareUrlTemplate(); err != nil {
		return err
	}

	return nil
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	trans := &http.Transport{}

	if ins.UseTLS {
		tlsConfig, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return nil, err
		}
		trans.TLSClientConfig = tlsConfig
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.Timeout),
	}

	return client, nil
}

type Prometheus struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Prometheus{}
	})
}

func (p *Prometheus) Clone() inputs.Input {
	return &Prometheus{}
}

func (p *Prometheus) Name() string {
	return inputName
}

func (p *Prometheus) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(p.Instances))
	for i := 0; i < len(p.Instances); i++ {
		ret[i] = p.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	urlwg := new(sync.WaitGroup)
	defer urlwg.Wait()

	for i := 0; i < len(ins.URLs); i++ {
		u, err := url.Parse(ins.URLs[i])
		if err != nil {
			log.Println("E! failed to parse prometheus scrape url:", ins.URLs[i], "error:", err)
			continue
		}

		urlwg.Add(1)

		go ins.gatherUrl(urlwg, slist, ScrapeUrl{URL: u, Tags: map[string]string{}})
	}

	urls, err := ins.UrlsFromConsul()
	if err != nil {
		log.Println("E! failed to query urls from consul:", err)
		return
	}

	for i := 0; i < len(urls); i++ {
		urlwg.Add(1)
		go ins.gatherUrl(urlwg, slist, urls[i])
	}
}

func (ins *Instance) gatherUrl(urlwg *sync.WaitGroup, slist *types.SampleList, uri ScrapeUrl) {
	defer urlwg.Done()

	u := uri.URL

	if u.Path == "" {
		u.Path = "/metrics"
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		log.Println("E! failed to new request for url:", u.String(), "error:", err)
		return
	}

	ins.setHeaders(req)

	labels := map[string]string{}

	urlKey, urlVal, err := ins.GenerateLabel(u)
	if err != nil {
		log.Println("E! failed to generate url label value:", err)
		return
	}

	labels[urlKey] = urlVal

	for key, val := range uri.Tags {
		labels[key] = val
	}

	res, err := ins.client.Do(req)
	if err != nil {
		slist.PushFront(types.NewSample("", "up", 0, labels))
		log.Println("E! failed to query url:", u.String(), "error:", err)
		return
	}

	if res.StatusCode != http.StatusOK {
		slist.PushFront(types.NewSample("", "up", 0, labels))
		log.Println("E! failed to query url:", u.String(), "status code:", res.StatusCode)
		return
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		slist.PushFront(types.NewSample("", "up", 0, labels))
		log.Println("E! failed to read response body, url:", u.String(), "error:", err)
		return
	}

	slist.PushFront(types.NewSample("", "up", 1, labels))

	parser := prometheus.NewParser(ins.NamePrefix, labels, res.Header, ins.ignoreMetricsFilter, ins.ignoreLabelKeysFilter)
	if err = parser.Parse(body, slist); err != nil {
		log.Println("E! failed to parse response body, url:", u.String(), "error:", err)
	}
}

func (ins *Instance) setHeaders(req *http.Request) {
	if ins.Username != "" && ins.Password != "" {
		req.SetBasicAuth(ins.Username, ins.Password)
	}

	if ins.BearerTokeFile != "" {
		content, err := os.ReadFile(ins.BearerTokeFile)
		if err != nil {
			log.Println("E! failed to read bearer token file:", ins.BearerTokeFile, "error:", err)
			return
		}

		ins.BearerTokenString = strings.TrimSpace(string(content))
	}

	if ins.BearerTokenString != "" {
		req.Header.Set("Authorization", "Bearer "+ins.BearerTokenString)
	}

	req.Header.Set("Accept", acceptHeader)

	for i := 0; i < len(ins.Headers); i += 2 {
		req.Header.Set(ins.Headers[i], ins.Headers[i+1])
	}
}
