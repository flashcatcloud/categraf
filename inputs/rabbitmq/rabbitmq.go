package rabbitmq

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "rabbitmq"

type RabbitMQ struct {
	config.Interval
	counter   uint64
	waitgrp   sync.WaitGroup
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &RabbitMQ{}
	})
}

func (r *RabbitMQ) Prefix() string {
	return inputName
}

func (r *RabbitMQ) Init() error {
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

func (r *RabbitMQ) Drop() {}

func (r *RabbitMQ) Gather(slist *list.SafeList) {
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
	URL      string `toml:"url"`
	Username string `toml:"username"`
	Password string `toml:"password"`

	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	HeaderTimeout config.Duration `toml:"header_timeout"`
	ClientTimeout config.Duration `toml:"client_timeout"`

	Nodes     []string `toml:"nodes"`
	Exchanges []string `toml:"exchanges"`

	MetricInclude             []string `toml:"metric_include"`
	MetricExclude             []string `toml:"metric_exclude"`
	QueueInclude              []string `toml:"queue_name_include"`
	QueueExclude              []string `toml:"queue_name_exclude"`
	FederationUpstreamInclude []string `toml:"federation_upstream_include"`
	FederationUpstreamExclude []string `toml:"federation_upstream_exclude"`

	tls.ClientConfig
	client *http.Client

	metricFilter   filter.Filter
	queueFilter    filter.Filter
	upstreamFilter filter.Filter

	excludeEveryQueue bool
}

func (ins *Instance) Init() error {
	if ins.URL == "" {
		return errors.New("url is blank")
	}

	var err error

	if err := ins.createQueueFilter(); err != nil {
		return err
	}

	if ins.upstreamFilter, err = filter.NewIncludeExcludeFilter(ins.FederationUpstreamInclude, ins.FederationUpstreamExclude); err != nil {
		return err
	}

	if ins.metricFilter, err = filter.NewIncludeExcludeFilter(ins.MetricInclude, ins.MetricExclude); err != nil {
		return err
	}

	ins.client, err = ins.createHTTPClient()
	return err
}

func (ins *Instance) createQueueFilter() error {
	queueFilter, err := filter.NewIncludeExcludeFilter(ins.QueueInclude, ins.QueueExclude)
	if err != nil {
		return err
	}
	ins.queueFilter = queueFilter

	for _, q := range ins.QueueExclude {
		if q == "*" {
			ins.excludeEveryQueue = true
		}
	}

	return nil
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	if ins.HeaderTimeout <= 0 {
		ins.HeaderTimeout = config.Duration(time.Second * 3)
	}

	if ins.ClientTimeout <= 0 {
		ins.ClientTimeout = config.Duration(time.Second * 4)
	}

	trans := &http.Transport{
		ResponseHeaderTimeout: time.Duration(ins.HeaderTimeout),
	}

	if ins.UseTLS {
		tlsConfig, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return nil, err
		}
		trans.TLSClientConfig = tlsConfig
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.ClientTimeout),
	}

	return client, nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) {
	tags := map[string]string{"url": ins.URL}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("scrape_use_seconds", use, tags))
	}(begun)

	// TODO up

}
