package prometheus

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "prometheus"

type Instance struct {
	URLs          []string          `toml:"urls"`
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`
	BearerToken   string            `toml:"bearer_token"`
	Username      string            `toml:"username"`
	Password      string            `toml:"password"`
	IgnoreMetrics []string          `toml:"ignore_metrics"`
	Timeout       config.Duration   `toml:"timeout"`

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
	}

	ins.client = client
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
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Prometheus{}
	})
}

func (p *Prometheus) GetInputName() string {
	return ""
}

func (p *Prometheus) GetInterval() config.Duration {
	return p.Interval
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

	// TODO
}
