package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	cms20190101 "github.com/alibabacloud-go/cms-20190101/v8/client"
	cms2021101 "github.com/alibabacloud-go/cms-export-20211101/v2/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"flashcat.cloud/categraf/inputs/aliyun/internal/types"
	"flashcat.cloud/categraf/pkg/limiter"
	"flashcat.cloud/categraf/pkg/stringx"
)

const (
	DefaultPageNum  = 1
	DefaultPageSize = 30
	MaxPageSize     = 10000
	MaxPageNum      = 99
)

func (m *Manager) ListMetrics(ctx context.Context, req *cms20190101.DescribeMetricMetaListRequest) (int, []*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, error) {
	count := 1
	if req.PageNumber == nil {
		req.SetPageNumber(DefaultPageNum)
	}
	if req.PageSize == nil {
		req.SetPageSize(MaxPageSize)
	}
	if req.Namespace == nil {
		req.SetNamespace("")
	}
	if req.Labels == nil {
		req.SetLabels("")
	}
	resp, err := m.cms.DescribeMetricMetaList(req)
	if err != nil {
		return count, nil, err
	}
	totalCount, err := strconv.Atoi(*resp.Body.TotalCount)
	if err != nil {
		return count, nil, err
	}

	pageSize, pageNum := pageCaculator(totalCount)
	req.SetPageSize(pageSize)
	resources := make([]*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, 0, pageSize)
	resources = append(resources, resp.Body.Resources.Resource...)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		count++
		req.SetPageNumber(i)
		resp, err := m.cms.DescribeMetricMetaList(req)
		if err != nil {
			return count, nil, err
		}
		resources = append(resources, resp.Body.Resources.Resource...)
	}
	return count, resources, nil
}

func pageCaculator(totalCount int) (size, num int32) {
	if totalCount < MaxPageSize {
		return MaxPageSize, 0
	}
	items := int32(totalCount - MaxPageSize)
	size = MaxPageSize
	num = int32(items/MaxPageSize) + 1
	if num > 0 {
		if num > MaxPageNum {
			num = MaxPageNum
		}
	}
	return
}

func (m *Manager) dataPointConverter(metricName, ns, datapoints string) ([]types.Point, error) {
	points := make([]types.Point, 0, 100)
	datapoints = strings.Replace(datapoints, "\"Value\":\"\"", "\"Value\":0", -1)
	err := json.Unmarshal([]byte(datapoints), &points)
	if err != nil {
		return nil, err
	}
	result := make([]types.Point, 0, len(points))
	for _, point := range points {
		point.Namespace = ns
		attributes := map[string]*float64{
			"value":   point.Val,
			"maximum": point.Max,
			"minimum": point.Min,
			"average": point.Avg,
			"sum":     point.Sum,
		}
		for attrName, attrValue := range attributes {
			if attrValue != nil {
				point.MetricName = fmt.Sprintf("%s_%s", stringx.SnakeCase(metricName), attrName)
				point.Value = tea.Float64(*attrValue)
				result = append(result, point)
			}
		}
	}
	return result, nil
}

func (m *Manager) requestDebugLog(req *cms20190101.DescribeMetricListRequest, resp *cms20190101.DescribeMetricListResponse, page int, cost time.Duration) {
	if m.debugMode {
		var reqid, token string
		if resp.Body != nil {
			if resp.Body.RequestId != nil {
				reqid = *resp.Body.RequestId
			}
			if resp.Body.NextToken != nil {
				token = *resp.Body.NextToken
			}
		}
		log.Printf("cms.DescribeMetricList request took %s, namespace:%s, metric name:%s, page:%d, request id:%s, next token:%s",
			cost, *req.Namespace, *req.MetricName, page, reqid, token)
	}
}

func (m *Manager) GetMetric(ctx context.Context, req *cms20190101.DescribeMetricListRequest, lmtr *limiter.RateLimiter) (int, []types.Point, error) {
	<-lmtr.C
	count := 1
	now := time.Now()
	resp, err := m.cms.DescribeMetricList(req)
	m.requestDebugLog(req, resp, count, time.Since(now))
	result := make([]types.Point, 0, 100)
	if err != nil {
		return count, nil, err
	}
	points, err := m.dataPointConverter(*req.MetricName, *req.Namespace, *resp.Body.Datapoints)
	if err != nil {
		return count, nil, err
	}
	result = append(result, points...)
	for resp.Body != nil && resp.Body.NextToken != nil && strings.TrimSpace(*resp.Body.NextToken) != "" {
		req.NextToken = resp.Body.NextToken
		count++
		<-lmtr.C
		now = time.Now()
		resp, err = m.cms.DescribeMetricList(req)
		m.requestDebugLog(req, resp, count, time.Since(now))
		if err != nil {
			log.Println(err)
			continue
		}
		points, err := m.dataPointConverter(*req.MetricName, *req.Namespace, *resp.Body.Datapoints)
		if err != nil {
			return count, nil, err
		}
		result = append(result, points...)
	}

	return count, result, nil
}

func (m *Manager) GetEcsHosts() ([]*cms20190101.DescribeMonitoringAgentHostsResponseBodyHostsHost, error) {
	// 主机实例描述
	// instanceID hostname ipgroup(外 内）  region os networktype serial number
	req := new(cms20190101.DescribeMonitoringAgentHostsRequest)
	req.SetRegionId(m.cms.region)
	req.SetPageSize(100)
	req.SetPageNumber(DefaultPageNum)
	resp, err := m.cms.DescribeMonitoringAgentHosts(req)
	if err != nil {
		return nil, err
	}
	result := make([]*cms20190101.DescribeMonitoringAgentHostsResponseBodyHostsHost, 0, 100)
	result = append(result, resp.Body.Hosts.Host...)

	totalCount := resp.Body.Total
	pageSize, pageNum := ecsPageCaculator(int(*totalCount))
	req.SetPageSize(pageSize)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		req.SetPageNumber(i)
		resp, err := m.cms.DescribeMonitoringAgentHosts(req)
		if err != nil {
			return nil, err
		}
		result = append(result, resp.Body.Hosts.Host...)
	}
	return result, nil
}

func NewCmsClient(key, secret, region, endpoint string) Option {
	if len(key) == 0 {
		panic("accessKey for cms is required")
	}
	if len(secret) == 0 {
		panic("accessSecret for cms is required")
	}
	if len(region) == 0 {
		panic("region for cms is required")
	}
	if len(endpoint) == 0 {
		panic("endpoint for cms is required")
	}
	return func(m *Manager) error {
		m.cms = &cmsClient{
			apikey:    key,
			apiSecret: secret,
			region:    region,
			endpoint:  endpoint,
		}
		config := &openapi.Config{
			AccessKeyId:     &m.cms.apikey,
			AccessKeySecret: &m.cms.apiSecret,
			RegionId:        &m.cms.region,
			Endpoint:        &m.cms.endpoint,
		}

		cms, err := cms20190101.NewClient(config)

		if err != nil {
			return err
		}
		m.cms.Client = cms
		return nil
	}
}

func NewCmsV2Client(key, secret, region, endpoint string) Option {
	if len(key) == 0 {
		panic("accessKey for cms batch exporter is required")
	}
	if len(secret) == 0 {
		panic("accessSecret for cms batch exporter is required")
	}
	if len(region) == 0 {
		panic("region for cms batch exporter is required")
	}
	if len(endpoint) == 0 {
		panic("endpoint for cms batch exporter is required")
	}
	return func(m *Manager) error {
		m.cmsv2 = &cmsV2Client{
			apikey:    key,
			apiSecret: secret,
			region:    region,
			endpoint:  endpoint,
		}
		config := &openapi.Config{
			AccessKeyId:     &m.cmsv2.apikey,
			AccessKeySecret: &m.cmsv2.apiSecret,
			RegionId:        &m.cmsv2.region,
			Endpoint:        &m.cmsv2.endpoint,
		}

		// 批量导出接口
		cmsv2, err := cms2021101.NewClient(config)
		if err != nil {
			return err
		}
		m.cmsv2.Client = cmsv2

		return nil
	}
}

func ecsPageCaculator(totalCount int) (int32, int32) {
	pageSize := 100
	num := totalCount/pageSize + 1
	return int32(pageSize), int32(num)
}
