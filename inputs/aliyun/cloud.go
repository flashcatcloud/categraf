package aliyun

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	cms20190101 "github.com/alibabacloud-go/cms-20190101/v8/client"
	"github.com/alibabacloud-go/tea/tea"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/aliyun/internal/manager"
	internalTypes "flashcat.cloud/categraf/inputs/aliyun/internal/types"
	"flashcat.cloud/categraf/pkg/cache"
	"flashcat.cloud/categraf/pkg/limiter"
	"flashcat.cloud/categraf/pkg/stringx"
	"flashcat.cloud/categraf/types"
)

const (
	inputName = "aliyun"
	timefmt   = "2006-01-02 15:04:05"
)

type (
	Aliyun struct {
		config.PluginConfig

		Instances []*Instance `toml:"instances"`
	}

	Instance struct {
		config.InstanceConfig

		// credentials.Config
		Credential

		client *manager.Manager `toml:"-"`

		windowStart time.Time `toml:"-"`
		windowEnd   time.Time `toml:"-"`

		Delay  config.Duration `toml:"delay"`
		Period config.Duration `toml:"period"`

		Namespaces []string       `json:"namespaces"`
		Filters    []MetricFilter `toml:"metric_filters"`

		// 最大请求次数 限流用
		RateLimit int `toml:"ratelimit"`

		CacheTTL       config.Duration `toml:"cache_ttl"`
		BatchSize      int             `toml:"batch_size"`
		RecentlyActive string          `toml:"recently_active"`

		// 请求超时设置
		Timeout config.Duration `toml:"timeout"`

		// 企业云监控配置项
		// batchSize int `toml:"batchSize"`

		metricCache *metricCache      `toml:"-"`
		metaCache   *cache.BasicCache `toml:"-"`

		EcsAgentHostTag string `toml:"ecs_host_tag"`
	}

	Credential struct {
		AccessKeyID     *string `toml:"access_key_id"`
		AccessKeySecret *string `toml:"access_key_secret"`
		Region          *string `toml:"region"`
		Endpoint        *string `toml:"endpoint"`
	}

	MetricFilter struct {
		MetricNames []string `toml:"metric_names"`
		Dimensions  string   `toml:"dimensions"`
		Namespace   string   `toml:"namespace"`
	}

	filteredMetric struct {
		metrics []internalTypes.Metric
	}
	// metricCache caches metrics, their filters, and generated queries.
	metricCache struct {
		ttl     time.Duration
		built   time.Time
		metrics []filteredMetric
	}
)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Aliyun{}
	})
}

func (a *Aliyun) Clone() inputs.Input {
	return &Aliyun{}
}

func (a *Aliyun) Name() string {
	return inputName
}

var _ inputs.SampleGatherer = new(Instance)
var _ inputs.Input = new(Aliyun)
var _ inputs.InstancesGetter = new(Aliyun)

func (ins *Instance) Init() error {
	if ins == nil ||
		ins.AccessKeySecret == nil ||
		ins.AccessKeyID == nil ||
		ins.Region == nil ||
		ins.Endpoint == nil {
		return types.ErrInstancesEmpty
	}
	if ins.BatchSize == 0 {
		ins.BatchSize = 500
	}
	if ins.RateLimit == 0 {
		ins.RateLimit = 25
	}
	if ins.CacheTTL == 0 {
		ins.CacheTTL = config.Duration(time.Hour)
	}
	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(time.Second * 5)
	}
	if len(ins.Namespaces) == 0 {
		ins.Namespaces = append(ins.Namespaces, "")
	}
	if len(ins.EcsAgentHostTag) == 0 {
		ins.EcsAgentHostTag = "agent_hostname"
	}
	ins.metaCache = cache.NewBasicCache()

	err := ins.initialize()
	if err != nil {
		return err
	}

	return nil
}

func (s *Aliyun) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}

func (ins *Instance) initialize() error {
	if len(*ins.AccessKeyID) == 0 {
		return fmt.Errorf("%s", "access_key_id is required")
	}
	if len(*ins.AccessKeySecret) == 0 {
		return fmt.Errorf("%s", "E! access_key_secret is required")
	}
	if len(*ins.Region) == 0 {
		return fmt.Errorf("%s", "region is required")
	}
	if len(*ins.Endpoint) == 0 {
		return fmt.Errorf("%s", "endpoint is required")
	}

	if ins.client == nil {
		cms := manager.NewCmsClient(*ins.AccessKeyID, *ins.AccessKeySecret, *ins.Region, *ins.Endpoint)
		m, err := manager.New(cms)
		if err != nil {
			return fmt.Errorf("connect to aliyun error, %s", err)
		}
		ins.client = m
	}

	if ins.metaCache.Size() == 0 {
		hosts, err := ins.client.GetEcsHosts()
		if err != nil {
			log.Println(err)
			return err
		}
		for _, host := range hosts {
			k := ins.client.EcsKey(*host.InstanceId)
			ins.metaCache.Add(k, host)
		}
	}
	return nil
}

func (f *metricCache) isValid() bool {
	return f != nil && f.metrics != nil && time.Since(f.built) < f.ttl
}

// getFilteredMetrics returns metrics specified in the config file or metrics listed from Cloudwatch.
func (ins *Instance) getFilteredMetrics() ([]filteredMetric, error) {
	if ins.metricCache != nil && ins.metricCache.isValid() {
		return ins.metricCache.metrics, nil
	}
	fMetrics := []filteredMetric{}

	allMetrics, err := ins.fetchNamespaceMetrics(ins.Namespaces)
	if err != nil {
		return nil, err
	}
	metrics := make([]internalTypes.Metric, 0, len(allMetrics))
	if len(ins.Filters) != 0 {
		for _, metric := range allMetrics {
			for _, f := range ins.Filters {
				if len(f.MetricNames) != 0 {
					for _, name := range f.MetricNames {
						if len(name) == 0 {
							name = metric.MetricName
						}
						if isSelected(metric, name, f.Dimensions, f.Namespace) {
							metrics = append(metrics, internalTypes.Metric{
								MetricName: name,
								Namespace:  metric.Namespace,
								Dimensions: metric.Dimensions,
								LabelStr:   metric.LabelStr,
							})
						}
					}
				} else {
					if isSelected(metric, "", f.Dimensions, f.Namespace) {
						metrics = append(metrics, internalTypes.Metric{
							MetricName: metric.MetricName,
							Namespace:  metric.Namespace,
							Dimensions: metric.Dimensions,
							LabelStr:   metric.LabelStr,
						})
					}
				}
			}
		}
	} else {
		metrics = allMetrics
	}
	fMetrics = append(fMetrics, filteredMetric{
		metrics: metrics,
	})

	if config.Config.DebugMode {
		for _, m := range metrics {
			log.Println("D!", m.Namespace, m.MetricName, m.Dimensions)
		}
	}

	ins.metricCache = &metricCache{
		metrics: fMetrics,
		built:   time.Now(),
		ttl:     time.Duration(ins.CacheTTL),
	}

	return fMetrics, nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	ins.updateWindow(time.Now())

	lmtr := limiter.NewRateLimiter(ins.RateLimit, time.Second)
	defer lmtr.Stop()
	wg := sync.WaitGroup{}

	if ins.metricCache.isValid() {
		for _, filtered := range ins.metricCache.metrics {
			for j := range filtered.metrics {
				<-lmtr.C
				wg.Add(1)
				go ins.sendMetrics(filtered.metrics[j], &wg, slist)
			}
		}
	} else {
		filteredMetrics, err := ins.getFilteredMetrics()
		if err != nil {
			log.Println("E!", err)
			return
		}
		for _, filtered := range filteredMetrics {
			for j := range filtered.metrics {
				<-lmtr.C
				wg.Add(1)
				go ins.sendMetrics(filtered.metrics[j], &wg, slist)
			}
		}
	}
	wg.Wait()
}

func (ins *Instance) sendMetrics(metric internalTypes.Metric, wg *sync.WaitGroup, slist *types.SampleList) {
	defer wg.Done()

	ctx := context.Background()
	req := new(cms20190101.DescribeMetricListRequest)
	if len(metric.MetricName) != 0 {
		req.MetricName = tea.String(metric.MetricName)
	}
	if len(metric.Namespace) != 0 {
		req.Namespace = tea.String(metric.Namespace)
	}
	if len(metric.Dimensions) != 0 {
		req.Dimensions = tea.String(metric.Dimensions)
	}
	if !ins.windowEnd.IsZero() {
		req.EndTime = tea.String(ins.windowEnd.Format(timefmt))
	}
	if !ins.windowStart.IsZero() {
		req.StartTime = tea.String(ins.windowStart.Format(timefmt))
	}
	points, err := ins.client.GetMetric(ctx, req)
	if err != nil {
		log.Println("E! get metrics error,", err)
		return
	}
	for _, point := range points {
		if point.Value != nil {
			tags := ins.makeLabels(point)
			mName := fmt.Sprintf("%s_%s", stringx.SnakeCase(point.Namespace), stringx.SnakeCase(point.MetricName))
			slist.PushFront(types.NewSample(inputName, mName, *point.Value, tags, map[string]string{"namespace": metric.Namespace, "metric_name": metric.MetricName}).SetTime(point.GetMetricTime()))
		}
	}

}

func (ins *Instance) makeLabels(point internalTypes.Point, labels ...map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range ins.Labels {
		result[key] = value
	}
	for _, lv := range labels {
		for k, v := range lv {
			result[k] = v
		}
	}
	addLabel := func(instance interface{}) {
		if meta, ok := instance.(*cms20190101.DescribeMonitoringAgentHostsResponseBodyHostsHost); ok {
			result[ins.EcsAgentHostTag] = stringx.SnakeCase(*meta.HostName)
		}
	}
	if instance, ok := ins.metaCache.Get(ins.client.EcsKey(point.InstanceID)); ok {
		addLabel(instance)
	}

	result["user_id"] = point.UserID

	if len(point.InstanceID) != 0 {
		result["instance_id"] = point.InstanceID
	}
	if len(point.ClusterID) != 0 {
		result["cluster_id"] = point.ClusterID
	}
	if len(point.NodeID) != 0 {
		result["node_id"] = point.NodeID
	}
	return result
}

func (ins *Instance) updateWindow(relativeTo time.Time) {
	windowEnd := relativeTo.Add(-time.Duration(ins.Delay))

	if ins.windowEnd.IsZero() {
		// this is the first run, no window info, so just get a single period
		ins.windowStart = windowEnd.Add(-time.Duration(ins.Period))
	} else {
		// subsequent window, start where last window left off
		ins.windowStart = ins.windowEnd
	}

	ins.windowEnd = windowEnd
}

// fetchNamespaceMetrics retrieves available metrics for a given aliyun namespace.
func (ins *Instance) fetchNamespaceMetrics(namespaces []string) ([]internalTypes.Metric, error) {
	// func (ins *Instance) fetchNamespaceMetrics() ([]*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, error) {
	var params *cms20190101.DescribeMetricMetaListRequest
	// namespaces := ins.Namespaces
	if len(namespaces) == 0 {
		namespaces = append(namespaces, "")
	}
	// result := make([]*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, 0, 100)
	result := make([]internalTypes.Metric, 0, 100)
	for i, namespace := range namespaces {
		params = &cms20190101.DescribeMetricMetaListRequest{
			Namespace: tea.String(namespaces[i]),
		}
		resp, err := ins.client.ListMetrics(context.Background(), params)
		if err != nil {
			log.Printf("E! failed to list metrics with namespace %s: %v", namespace, err)
			// skip problem namespace on error and continue to next namespace
			return nil, err
		}
		for _, m := range resp {
			point := internalTypes.Metric{
				LabelStr:   *m.Labels,
				Namespace:  *m.Namespace,
				MetricName: *m.MetricName,
			}
			result = append(result, point)
		}

	}
	return result, nil
}

func isSelected(metric internalTypes.Metric, name, dimensions, namespace string) bool {
	if len(name) != 0 && name != metric.MetricName {
		return false
	}
	if len(dimensions) != 0 && metric.Dimensions != dimensions {
		return false
	}
	if len(namespace) != 0 && metric.Namespace != namespace {
		return false
	}
	return true
}
