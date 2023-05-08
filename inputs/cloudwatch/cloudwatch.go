// forked from telegraf https://github.com/influxdata/telegraf/blob/master/plugins/inputs/cloudwatch/sample.conf
package cloudwatch

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwClient "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	internalaws "flashcat.cloud/categraf/pkg/aws"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/limiter"
	internalProxy "flashcat.cloud/categraf/pkg/proxy"
	"flashcat.cloud/categraf/pkg/stringx"
	internalTypes "flashcat.cloud/categraf/types"
	internalMetric "flashcat.cloud/categraf/types/metric"
)

//go:embed sample.conf
var sampleConfig string

const inputName = "cloudwatch"

const (
	StatisticAverage     = "Average"
	StatisticMaximum     = "Maximum"
	StatisticMinimum     = "Minimum"
	StatisticSum         = "Sum"
	StatisticSampleCount = "SampleCount"
)

type (
	CloudWatch struct {
		config.PluginConfig
		Instances []*Instance `toml:"instances"`
	}

	// CloudWatch contains the configuration and cache for the cloudwatch plugin.
	Instance struct {
		config.InstanceConfig

		StatisticExclude []string        `toml:"statistic_exclude"`
		StatisticInclude []string        `toml:"statistic_include"`
		Timeout          config.Duration `toml:"timeout"`

		internalProxy.HTTPProxy

		Period         config.Duration `toml:"period"`
		Delay          config.Duration `toml:"delay"`
		Namespace      string          `toml:"namespace" deprecated:"1.25.0;use 'namespaces' instead"`
		Namespaces     []string        `toml:"namespaces"`
		Metrics        []*Metric       `toml:"metrics"`
		CacheTTL       config.Duration `toml:"cache_ttl"`
		RateLimit      int             `toml:"ratelimit"`
		RecentlyActive string          `toml:"recently_active"`
		BatchSize      int             `toml:"batch_size"`

		client          cloudwatchClient
		statFilter      filter.Filter
		metricCache     *metricCache
		queryDimensions map[string]*map[string]string
		windowStart     time.Time
		windowEnd       time.Time

		internalaws.CredentialConfig
	}
)

// Metric defines a simplified Cloudwatch metric.
type Metric struct {
	StatisticExclude *[]string    `toml:"statistic_exclude"`
	StatisticInclude *[]string    `toml:"statistic_include"`
	MetricNames      []string     `toml:"names"`
	Dimensions       []*Dimension `toml:"dimensions"`
}

// Dimension defines a simplified Cloudwatch dimension (provides metric filtering).
type Dimension struct {
	Name         string `toml:"name"`
	Value        string `toml:"value"`
	valueMatcher filter.Filter
}

// metricCache caches metrics, their filters, and generated queries.
type metricCache struct {
	ttl     time.Duration
	built   time.Time
	metrics []filteredMetric
	queries map[string][]types.MetricDataQuery
}

type cloudwatchClient interface {
	ListMetrics(context.Context, *cwClient.ListMetricsInput, ...func(*cwClient.Options)) (*cwClient.ListMetricsOutput, error)
	GetMetricData(context.Context, *cwClient.GetMetricDataInput, ...func(*cwClient.Options)) (*cwClient.GetMetricDataOutput, error)
}

func (*CloudWatch) SampleConfig() string {
	return sampleConfig
}

func (ins *Instance) Init() error {
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
	if len(ins.Namespace) != 0 {
		ins.Namespaces = append(ins.Namespaces, ins.Namespace)
	}

	if len(ins.Namespaces) == 0 {
		return internalTypes.ErrInstancesEmpty
	}
	err := ins.initializeCloudWatch()
	if err != nil {
		return err
	}

	// Set config level filter (won't change throughout life of plugin).
	ins.statFilter, err = filter.NewIncludeExcludeFilter(ins.StatisticInclude, ins.StatisticExclude)
	if err != nil {
		return err
	}

	return nil
}

func (cw *CloudWatch) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(cw.Instances))
	for i := 0; i < len(cw.Instances); i++ {
		ret[i] = cw.Instances[i]
	}
	return ret
}

// Gather takes in an accumulator and adds the metrics that the Input
// gathers. This is called every "interval".
func (ins *Instance) Gather(slist *internalTypes.SampleList) {
	filteredMetrics, err := getFilteredMetrics(ins)
	if err != nil {
		log.Println("E! filter metrics error,", err)
		return
	}

	ins.updateWindow(time.Now())

	// Get all of the possible queries so we can send groups of 100.
	queries := ins.getDataQueries(filteredMetrics)
	if len(queries) == 0 {
		log.Println("E! data queries length is 0")
		return
	}

	// Limit concurrency or we can easily exhaust user connection limit.
	// See cloudwatch API request limits:
	// http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/cloudwatch_limits.html
	lmtr := limiter.NewRateLimiter(ins.RateLimit, time.Second)
	defer lmtr.Stop()
	wg := sync.WaitGroup{}
	rLock := sync.Mutex{}

	results := map[string][]types.MetricDataResult{}

	for namespace, namespacedQueries := range queries {
		var batches [][]types.MetricDataQuery

		for ins.BatchSize < len(namespacedQueries) {
			namespacedQueries, batches = namespacedQueries[ins.BatchSize:], append(batches, namespacedQueries[0:ins.BatchSize:ins.BatchSize])
		}
		batches = append(batches, namespacedQueries)

		for i := range batches {
			wg.Add(1)
			<-lmtr.C
			go func(n string, inm []types.MetricDataQuery) {
				defer wg.Done()
				result, err := ins.gatherMetrics(ins.getDataInputs(inm))
				if err != nil {
					log.Println("E!", err)
					return
				}

				rLock.Lock()
				results[n] = append(results[n], result...)
				rLock.Unlock()
			}(namespace, batches[i])
		}
	}

	wg.Wait()

	err = ins.aggregateMetrics(slist, results)
	if err != nil {
		log.Println("E! aggregate metrics error,", err)
	}
}

func (ins *Instance) initializeCloudWatch() error {
	proxy, err := ins.HTTPProxy.Proxy()
	if err != nil {
		return err
	}

	cfg, err := ins.CredentialConfig.Credentials()
	if err != nil {
		return err
	}
	ins.client = cwClient.NewFromConfig(cfg, func(options *cwClient.Options) {
		// Disable logging
		options.ClientLogMode = 0

		options.HTTPClient = &http.Client{
			// use values from DefaultTransport
			Transport: &http.Transport{
				Proxy: proxy,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: true,
				}).DialContext,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			Timeout: time.Duration(ins.Timeout),
		}
	})

	// Initialize regex matchers for each Dimension value.
	for _, m := range ins.Metrics {
		for _, dimension := range m.Dimensions {
			matcher, err := filter.NewIncludeExcludeFilter([]string{dimension.Value}, nil)
			if err != nil {
				return err
			}

			dimension.valueMatcher = matcher
		}
	}
	return nil
}

type filteredMetric struct {
	metrics    []types.Metric
	statFilter filter.Filter
}

// getFilteredMetrics returns metrics specified in the config file or metrics listed from Cloudwatch.
func getFilteredMetrics(c *Instance) ([]filteredMetric, error) {
	if c.metricCache != nil && c.metricCache.isValid() {
		return c.metricCache.metrics, nil
	}

	fMetrics := []filteredMetric{}

	// check for provided metric filter
	if c.Metrics != nil {
		for _, m := range c.Metrics {
			metrics := []types.Metric{}
			if !hasWildcard(m.Dimensions) {
				dimensions := make([]types.Dimension, 0, len(m.Dimensions))
				for _, d := range m.Dimensions {
					dimensions = append(dimensions, types.Dimension{
						Name:  aws.String(d.Name),
						Value: aws.String(d.Value),
					})
				}
				for _, name := range m.MetricNames {
					for _, namespace := range c.Namespaces {
						metrics = append(metrics, types.Metric{
							Namespace:  aws.String(namespace),
							MetricName: aws.String(name),
							Dimensions: dimensions,
						})
					}
				}
			} else {
				allMetrics := c.fetchNamespaceMetrics()
				for _, name := range m.MetricNames {
					for _, metric := range allMetrics {
						if isSelected(name, metric, m.Dimensions) {
							for _, namespace := range c.Namespaces {
								metrics = append(metrics, types.Metric{
									Namespace:  aws.String(namespace),
									MetricName: aws.String(name),
									Dimensions: metric.Dimensions,
								})
							}
						}
					}
				}
			}

			if m.StatisticExclude == nil {
				m.StatisticExclude = &c.StatisticExclude
			}
			if m.StatisticInclude == nil {
				m.StatisticInclude = &c.StatisticInclude
			}
			statFilter, err := filter.NewIncludeExcludeFilter(*m.StatisticInclude, *m.StatisticExclude)
			if err != nil {
				return nil, err
			}

			fMetrics = append(fMetrics, filteredMetric{
				metrics:    metrics,
				statFilter: statFilter,
			})
		}
	} else {
		metrics := c.fetchNamespaceMetrics()
		fMetrics = []filteredMetric{
			{
				metrics:    metrics,
				statFilter: c.statFilter,
			},
		}
	}

	c.metricCache = &metricCache{
		metrics: fMetrics,
		built:   time.Now(),
		ttl:     time.Duration(c.CacheTTL),
	}

	return fMetrics, nil
}

// fetchNamespaceMetrics retrieves available metrics for a given CloudWatch namespace.
func (ins *Instance) fetchNamespaceMetrics() []types.Metric {
	metrics := []types.Metric{}

	for _, namespace := range ins.Namespaces {
		params := &cwClient.ListMetricsInput{
			Dimensions: []types.DimensionFilter{},
			Namespace:  aws.String(namespace),
		}
		if ins.RecentlyActive == "PT3H" {
			params.RecentlyActive = types.RecentlyActivePt3h
		}

		for {
			resp, err := ins.client.ListMetrics(context.Background(), params)
			if err != nil {
				log.Printf("E! failed to list metrics with namespace %s: %v", namespace, err)
				// skip problem namespace on error and continue to next namespace
				break
			}
			metrics = append(metrics, resp.Metrics...)

			if resp.NextToken == nil {
				break
			}
			params.NextToken = resp.NextToken
		}
	}
	return metrics
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

// getDataQueries gets all of the possible queries so we can maximize the request payload.
func (ins *Instance) getDataQueries(filteredMetrics []filteredMetric) map[string][]types.MetricDataQuery {
	if ins.metricCache != nil && ins.metricCache.queries != nil && ins.metricCache.isValid() {
		return ins.metricCache.queries
	}

	ins.queryDimensions = map[string]*map[string]string{}

	dataQueries := map[string][]types.MetricDataQuery{}
	for i, filtered := range filteredMetrics {
		for j, metric := range filtered.metrics {
			id := strconv.Itoa(j) + "_" + strconv.Itoa(i)
			dimension := ctod(metric.Dimensions)
			if filtered.statFilter.Match("average") {
				ins.queryDimensions["average_"+id] = dimension
				dataQueries[*metric.Namespace] = append(dataQueries[*metric.Namespace], types.MetricDataQuery{
					Id:    aws.String("average_" + id),
					Label: aws.String(snakeCase(*metric.MetricName + "_average")),
					MetricStat: &types.MetricStat{
						Metric: &filtered.metrics[j],
						Period: aws.Int32(int32(time.Duration(ins.Period).Seconds())),
						Stat:   aws.String(StatisticAverage),
					},
				})
			}
			if filtered.statFilter.Match("maximum") {
				ins.queryDimensions["maximum_"+id] = dimension
				dataQueries[*metric.Namespace] = append(dataQueries[*metric.Namespace], types.MetricDataQuery{
					Id:    aws.String("maximum_" + id),
					Label: aws.String(snakeCase(*metric.MetricName + "_maximum")),
					MetricStat: &types.MetricStat{
						Metric: &filtered.metrics[j],
						Period: aws.Int32(int32(time.Duration(ins.Period).Seconds())),
						Stat:   aws.String(StatisticMaximum),
					},
				})
			}
			if filtered.statFilter.Match("minimum") {
				ins.queryDimensions["minimum_"+id] = dimension
				dataQueries[*metric.Namespace] = append(dataQueries[*metric.Namespace], types.MetricDataQuery{
					Id:    aws.String("minimum_" + id),
					Label: aws.String(snakeCase(*metric.MetricName + "_minimum")),
					MetricStat: &types.MetricStat{
						Metric: &filtered.metrics[j],
						Period: aws.Int32(int32(time.Duration(ins.Period).Seconds())),
						Stat:   aws.String(StatisticMinimum),
					},
				})
			}
			if filtered.statFilter.Match("sum") {
				ins.queryDimensions["sum_"+id] = dimension
				dataQueries[*metric.Namespace] = append(dataQueries[*metric.Namespace], types.MetricDataQuery{
					Id:    aws.String("sum_" + id),
					Label: aws.String(snakeCase(*metric.MetricName + "_sum")),
					MetricStat: &types.MetricStat{
						Metric: &filtered.metrics[j],
						Period: aws.Int32(int32(time.Duration(ins.Period).Seconds())),
						Stat:   aws.String(StatisticSum),
					},
				})
			}
			if filtered.statFilter.Match("sample_count") {
				ins.queryDimensions["sample_count_"+id] = dimension
				dataQueries[*metric.Namespace] = append(dataQueries[*metric.Namespace], types.MetricDataQuery{
					Id:    aws.String("sample_count_" + id),
					Label: aws.String(snakeCase(*metric.MetricName + "_sample_count")),
					MetricStat: &types.MetricStat{
						Metric: &filtered.metrics[j],
						Period: aws.Int32(int32(time.Duration(ins.Period).Seconds())),
						Stat:   aws.String(StatisticSampleCount),
					},
				})
			}
		}
	}

	if len(dataQueries) == 0 {
		if config.Config.DebugMode {
			log.Println("D! no metrics found to collect")
		}
		return nil
	}

	if ins.metricCache == nil {
		ins.metricCache = &metricCache{
			queries: dataQueries,
			built:   time.Now(),
			ttl:     time.Duration(ins.CacheTTL),
		}
	} else {
		ins.metricCache.queries = dataQueries
	}

	return dataQueries
}

// gatherMetrics gets metric data from Cloudwatch.
func (ins *Instance) gatherMetrics(
	params *cwClient.GetMetricDataInput,
) ([]types.MetricDataResult, error) {
	results := []types.MetricDataResult{}

	for {
		resp, err := ins.client.GetMetricData(context.Background(), params)
		if err != nil {
			return nil, fmt.Errorf("failed to get metric data: %v", err)
		}

		results = append(results, resp.MetricDataResults...)
		if resp.NextToken == nil {
			break
		}
		params.NextToken = resp.NextToken
	}

	return results, nil
}

func (ins *Instance) aggregateMetrics(
	slist *internalTypes.SampleList,
	metricDataResults map[string][]types.MetricDataResult,
) error {
	var (
		grouper = internalMetric.NewSeriesGrouper()
	)

	for namespace, results := range metricDataResults {
		ns := namespace
		namespace = sanitizeMeasurement(namespace)

		for _, result := range results {
			tags := map[string]string{}

			if dimensions, ok := ins.queryDimensions[*result.Id]; ok {
				tags = *dimensions
			}
			tags["region"] = ins.Region
			tags["namespace"] = ns

			for i := range result.Values {
				grouper.Add(namespace, tags, result.Timestamps[i], *result.Label, result.Values[i])
			}
		}
	}
	for _, metric := range grouper.Metrics() {
		slist.PushSamples(metric.Name(), metric.Fields(), metric.Tags())
	}

	return nil
}

var _ inputs.SampleGatherer = new(Instance)
var _ inputs.Input = new(CloudWatch)
var _ inputs.InstancesGetter = new(CloudWatch)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &CloudWatch{}
	})
}

func (c *CloudWatch) Clone() inputs.Input {
	return &CloudWatch{}
}

func (c *CloudWatch) Name() string {
	return inputName
}

func sanitizeMeasurement(namespace string) string {
	namespace = strings.ReplaceAll(namespace, "/", "_")
	namespace = snakeCase(namespace)
	return inputName + "_" + namespace
}

func snakeCase(s string) string {
	s = stringx.SnakeCase(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "__", "_")
	return s
}

// ctod converts cloudwatch dimensions to regular dimensions.
func ctod(cDimensions []types.Dimension) *map[string]string {
	dimensions := map[string]string{}
	for i := range cDimensions {
		dimensions[snakeCase(*cDimensions[i].Name)] = *cDimensions[i].Value
	}
	return &dimensions
}

func (ins *Instance) getDataInputs(dataQueries []types.MetricDataQuery) *cwClient.GetMetricDataInput {
	return &cwClient.GetMetricDataInput{
		StartTime:         aws.Time(ins.windowStart),
		EndTime:           aws.Time(ins.windowEnd),
		MetricDataQueries: dataQueries,
	}
}

// isValid checks the validity of the metric cache.
func (f *metricCache) isValid() bool {
	return f.metrics != nil && time.Since(f.built) < f.ttl
}

func hasWildcard(dimensions []*Dimension) bool {
	for _, d := range dimensions {
		if d.Value == "" || strings.ContainsAny(d.Value, "*?[") {
			return true
		}
	}
	return false
}

func isSelected(name string, metric types.Metric, dimensions []*Dimension) bool {
	if name != *metric.MetricName {
		return false
	}
	if len(metric.Dimensions) != len(dimensions) {
		return false
	}
	for _, d := range dimensions {
		selected := false
		for _, d2 := range metric.Dimensions {
			if d.Name == *d2.Name {
				if d.Value == "" || d.valueMatcher.Match(*d2.Value) {
					selected = true
				}
			}
		}
		if !selected {
			return false
		}
	}
	return true
}
