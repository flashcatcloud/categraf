package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	cms20190101 "github.com/alibabacloud-go/cms-20190101/v8/client"
	cms2021101 "github.com/alibabacloud-go/cms-export-20211101/v2/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"flashcat.cloud/categraf/inputs/aliyun/internal/types"
	"flashcat.cloud/categraf/pkg/stringx"
)

const (
	DefaultPageNum  = 1
	DefaultPageSize = 30
	MaxPageSize     = 10000
	MaxPageNum      = 99
)

func (m *Manager) ListMetrics(ctx context.Context, req *cms20190101.DescribeMetricMetaListRequest) ([]*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, error) {
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
		return nil, err
	}
	totalCount, err := strconv.Atoi(*resp.Body.TotalCount)
	if err != nil {
		return nil, err
	}

	pageSize, pageNum := pageCaculator(totalCount)
	req.SetPageSize(pageSize)
	resources := make([]*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, 0, pageSize)
	resources = append(resources, resp.Body.Resources.Resource...)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		req.SetPageNumber(i)
		resp, err := m.cms.DescribeMetricMetaList(req)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resp.Body.Resources.Resource...)
	}
	return resources, nil
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
	err := json.Unmarshal([]byte(datapoints), &points)
	if err != nil {
		return nil, err
	}
	r := types.Point{}
	result := make([]types.Point, 0, len(points))
	for _, point := range points {
		r.UserID = point.UserID
		r.NodeID = point.NodeID
		r.ClusterID = point.ClusterID
		r.InstanceID = point.InstanceID
		r.Namespace = ns
		r.Timestamp = point.Timestamp

		if point.Val != nil {
			r.MetricName = fmt.Sprintf("%s_%s", stringx.SnakeCase(metricName), "value")
			r.Value = tea.Float64(*point.Val)
			result = append(result, r)
		}
		if point.Max != nil {
			r.MetricName = fmt.Sprintf("%s_%s", stringx.SnakeCase(metricName), "maximum")
			r.Value = tea.Float64(*point.Max)
			result = append(result, r)
		}
		if point.Min != nil {
			r.MetricName = fmt.Sprintf("%s_%s", stringx.SnakeCase(metricName), "minimum")
			r.Value = tea.Float64(*point.Min)
			result = append(result, r)
		}

		if point.Avg != nil {
			r.MetricName = fmt.Sprintf("%s_%s", stringx.SnakeCase(metricName), "average")
			r.Value = tea.Float64(*point.Avg)
			result = append(result, r)
		}
	}
	return result, nil
}
func (m *Manager) GetMetric(ctx context.Context, req *cms20190101.DescribeMetricListRequest) ([]types.Point, error) {

	resp, err := m.cms.DescribeMetricList(req)
	result := make([]types.Point, 0, 100)
	if err != nil {
		return nil, err
	}
	points, err := m.dataPointConverter(*req.MetricName, *req.Namespace, *resp.Body.Datapoints)
	if err != nil {
		return nil, err
	}
	result = append(result, points...)
	for resp.Body != nil && resp.Body.NextToken != nil {
		req.NextToken = resp.Body.NextToken
		resp, err = m.cms.DescribeMetricList(req)
		if err != nil {
			log.Println(err)
			continue
		}
		points, err := m.dataPointConverter(*req.MetricName, *req.Namespace, *resp.Body.Datapoints)
		if err != nil {
			return nil, err
		}
		result = append(result, points...)
	}

	return result, nil
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
