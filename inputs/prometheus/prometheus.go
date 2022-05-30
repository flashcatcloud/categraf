package prometheus

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/logs"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "prometheus"
const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,*/*;q=0.1`

type Instance struct {
	URLs              []string          `toml:"urls"`
	Labels            map[string]string `toml:"labels"`
	IntervalTimes     int64             `toml:"interval_times"`
	BearerTokenString string            `toml:"bearer_token_string"`
	BearerTokeFile    string            `toml:"bearer_token_file"`
	Username          string            `toml:"username"`
	Password          string            `toml:"password"`
	Timeout           config.Duration   `toml:"timeout"`
	IgnoreMetrics     []string          `toml:"ignore_metrics"`
	IgnoreLabelKeys   []string          `toml:"ignore_label_keys"`
	Headers           []string          `toml:"headers"`

	ignoreMetricsFilter   filter.Filter
	ignoreLabelKeysFilter filter.Filter
	tls.ClientConfig
	client *http.Client
}

func (ins *Instance) Init() error {
	if len(ins.URLs) == 0 {
		return errors.New("urls is empty")
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
	config.Interval
	Instances []*Instance `toml:"instances"`
	logs.Logs

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Prometheus{}
	})
}

func (p *Prometheus) Prefix() string {
	return ""
}

func (p *Prometheus) Init() error {
	if len(p.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(p.Instances); i++ {
		if err := p.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (p *Prometheus) Drop() {}

func (p *Prometheus) Gather(slist *list.SafeList) {
	atomic.AddUint64(&p.Counter, 1)
	for i := range p.Instances {
		ins := p.Instances[i]
		p.wg.Add(1)
		go p.gatherOnce(slist, ins)
	}
	p.wg.Wait()
}

func (p *Prometheus) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer p.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&p.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	urlwg := new(sync.WaitGroup)
	defer urlwg.Wait()

	for i := 0; i < len(ins.URLs); i++ {
		urlwg.Add(1)
		go p.gatherUrl(slist, ins, ins.URLs[i], urlwg)
	}
}

func (p *Prometheus) gatherUrl(slist *list.SafeList, ins *Instance, uri string, urlwg *sync.WaitGroup) {
	defer urlwg.Done()

	u, err := url.Parse(uri)
	if err != nil {
		log.Println("E! failed to parse url:", uri, "error:", err)
		return
	}

	if u.Path == "" {
		u.Path = "/metrics"
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		log.Println("E! failed to new request for url:", u.String(), "error:", err)
		return
	}

	ins.setHeaders(req)

	labels := map[string]string{"url": u.String()}
	for key, val := range ins.Labels {
		labels[key] = val
	}

	res, err := ins.client.Do(req)
	if err != nil {
		slist.PushFront(inputs.NewSample("up", 0, labels))
		log.Println("E! failed to query url:", u.String(), "error:", err)
		return
	}

	if res.StatusCode != http.StatusOK {
		slist.PushFront(inputs.NewSample("up", 0, labels))
		log.Println("E! failed to query url:", u.String(), "status code:", res.StatusCode)
		return
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		slist.PushFront(inputs.NewSample("up", 0, labels))
		log.Println("E! failed to read response body, error:", err)
		return
	}

	slist.PushFront(inputs.NewSample("up", 1, labels))

	parser := prometheus.NewParser(labels, res.Header, ins.ignoreMetricsFilter, ins.ignoreLabelKeysFilter)
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
