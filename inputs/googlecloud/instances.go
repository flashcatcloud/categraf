package googlecloud

import (
	"context"
	"fmt"
	"log"
	"time"

	apiv3 "cloud.google.com/go/monitoring/apiv3"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/monitoring/v3"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

type (
	Instance struct {
		config.InstanceConfig

		GoogleCloudConfig
		v3client *apiv3.MetricClient

		// metric instance_name type
		// resource: type instance_id project_id zone
		Filter string `toml:"filter"`

		Period  config.Duration `toml:"period"`
		Delay   config.Duration `toml:"delay"`
		Timeout config.Duration `toml:"timeout"`

		CacheTTL    config.Duration `toml:"cache_ttl"`
		metricCache *metricCache    `toml:"-"`

		UnmaskProjectID bool `toml:"-"`

		GceHostTag string `toml:"gce_host_tag"`

		RequestInflight      int `toml:"request_inflight"`
		ForceRequestInflight int `toml:"force_request_inflight"`
	}

	filteredMetric monitoring.TimeSeries

	// metricCache caches metrics, their filters, and generated queries.
	metricCache struct {
		ttl   time.Duration
		built time.Time
		// metrics []filteredMetric
		metrics []string
	}

	GoogleCloudConfig struct {
		Version   string `toml:"version"`
		ProjectID string `toml:"project_id"`

		CredentialsFile string `toml:"credentials_file"`
		CredentialsJSON string `toml:"credentials_json"`
	}
)

func (f *metricCache) isValid() bool {
	return f.metrics != nil && time.Since(f.built) < f.ttl
}
func (ins *Instance) Drop() {
	err := ins.v3client.Close()
	if err != nil {
		log.Println("W! close gcp client error:", err)
	}
}

var _ inputs.SampleGatherer = new(Instance)

func (ins *Instance) Init() error {
	if ins == nil ||
		(len(ins.CredentialsFile) == 0 && len(ins.CredentialsJSON) == 0) ||
		len(ins.ProjectID) == 0 {
		return types.ErrInstancesEmpty
	}

	opts := []option.ClientOption{}
	if len(ins.CredentialsFile) > 0 {
		opts = append(opts, option.WithCredentialsFile(ins.CredentialsFile))
	}
	if len(ins.CredentialsJSON) > 0 {
		opts = append(opts, option.WithCredentialsJSON([]byte(ins.CredentialsJSON)))
	}
	ctx := context.Background()
	client, err := apiv3.NewMetricClient(ctx, opts...)
	if err != nil {
		return err
	}
	ins.v3client = client
	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(5 * time.Second)
	}
	if ins.CacheTTL == 0 {
		ins.CacheTTL = config.Duration(1 * time.Hour)
	}
	if ins.Delay == 0 {
		ins.Delay = config.Duration(2 * time.Minute)
	}
	if ins.Period == 0 {
		ins.Period = config.Duration(1 * time.Minute)
	}
	if len(ins.GceHostTag) == 0 {
		ins.GceHostTag = "agent_hostname"
	}
	if ins.RequestInflight == 0 {
		ins.RequestInflight = 30
	}
	if ins.RequestInflight > 100 {
		ins.RequestInflight = 60
	}
	if ins.ForceRequestInflight > 0 {
		ins.RequestInflight = ins.ForceRequestInflight
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if ins == nil ||
		ins.v3client == nil {
		log.Println("E! googlecloud client is nil")
		return
	}

	if len(ins.Filter) != 0 {
		err := ins.readTimeSeriesValue(slist, ins.Filter)
		if err != nil {
			log.Println("E! read time series value error:", err)
		}
	} else {
		if ins.metricCache == nil || !ins.metricCache.isValid() {
			metrics, err := ins.ListMetrics()
			if err != nil {
				log.Println("E! list metrics error:", err)
				return
			}
			ins.metricCache = &metricCache{
				ttl:     time.Duration(ins.CacheTTL),
				built:   time.Now(),
				metrics: metrics,
			}
		}

		token := make(chan struct{}, ins.RequestInflight)
		for _, metric := range ins.metricCache.metrics {
			token <- struct{}{}
			go func(metric string) {
				ins.readTimeSeriesValue(slist, fmt.Sprintf("metric.type=\"%s\"", metric))
				<-token
			}(metric)
		}
	}
}
