package manager

import (
	"context"
	"encoding/json"
	"flashcat.cloud/categraf/config"
	"fmt"
	"log"
	"strconv"
	"unicode"

	cms20190101 "github.com/alibabacloud-go/cms-20190101/v8/client"
	cms2021101 "github.com/alibabacloud-go/cms-export-20211101/v2/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"flashcat.cloud/categraf/inputs/aliyun/internal/types"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ldate)
}

const (
	DefaultPageNum  = 1
	DefaultPageSize = 30
	MaxPageSize     = 10000
	MaxPageNum      = 99
)

type (
	Manager struct {
		domain    string
		region    string
		apikey    string
		apiSecret string

		enterOn    bool
		client     *cms20190101.Client
		enterprise *cms2021101.Client
	}
)

func (m *Manager) Enterprise() *cms2021101.Client {
	return m.enterprise
}

func (m *Manager) Base() *cms20190101.Client {
	return m.client
}

func (m *Manager) ListMetrics(ctx context.Context, nr *cms20190101.DescribeMetricMetaListRequest) ([]*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, error) {
	if nr.PageNumber == nil {
		nr.PageNumber = tea.Int32(DefaultPageNum)
	}
	if nr.PageSize == nil {
		nr.PageSize = tea.Int32(MaxPageSize)
	}
	if nr.Namespace == nil {
		nr.Namespace = tea.String("")
	}
	if nr.Labels == nil {
		nr.Labels = tea.String("")
	}
	nresp, err := m.Base().DescribeMetricMetaList(nr)
	if err != nil {
		return nil, err
	}
	totalCount, err := strconv.Atoi(*nresp.Body.TotalCount)
	if err != nil {
		return nil, err
	}

	pageSize, pageNum := pageCaculator(totalCount)
	nr.SetPageSize(pageSize)
	resources := make([]*cms20190101.DescribeMetricMetaListResponseBodyResourcesResource, 0, pageSize)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		nr.PageNumber = tea.Int32(i)
		resp, err := m.Base().DescribeMetricMetaList(nr)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resp.Body.Resources.Resource...)
	}
	nresp.Body.Resources.Resource = append(nresp.Body.Resources.Resource, resources...)
	return nresp.Body.Resources.Resource, nil
}

func New(key, secret, domain, region *string) (*Manager, error) {
	switch {
	case key == nil:
		return nil, fmt.Errorf("access_Key_id is required")
	case secret == nil:
		return nil, fmt.Errorf("access_key_secret is required")
	case domain == nil:
		return nil, fmt.Errorf("endpoint is required")
	case region == nil:
		return nil, fmt.Errorf("region is required")
	}
	if config.Config.DebugMode {
		log.Println("D! access_key_id:", (*key)[:4]+"**********"+(*key)[len(*key)-4:])
		log.Println("D! access_key_secret:", (*secret)[:4]+"**********"+(*secret)[len(*secret)-4:])
		log.Println("D! endpoint:", *domain)
		log.Println("D! region:", *region)
	}

	config := &openapi.Config{
		AccessKeyId:     key,
		AccessKeySecret: secret,
		RegionId:        region,
		Endpoint:        domain,
	}

	c, err := cms20190101.NewClient(config)

	if err != nil {
		return nil, err
	}

	e, err := cms2021101.NewClient(config)

	return &Manager{
		client:     c,
		enterprise: e,
		region:     *region,
		domain:     *domain,
		apikey:     *key,
		apiSecret:  *secret,
	}, nil
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

func (m *Manager) GetEcsHosts() ([]*cms20190101.DescribeMonitoringAgentHostsResponseBodyHostsHost, error) {
	// 主机实例描述
	// instanceID hostname ipgroup(外 内）  region os networktype serial number
	q := new(cms20190101.DescribeMonitoringAgentHostsRequest)
	resp, err := m.Base().DescribeMonitoringAgentHosts(q)
	if err != nil {
		return nil, err
	}
	return resp.Body.Hosts.Host, nil
}

func (m *Manager) GetMetric(ctx context.Context, req *cms20190101.DescribeMetricListRequest) ([]types.Point, error) {

	resp, err := m.Base().DescribeMetricList(req)
	if err != nil {
		return nil, err
	}
	for resp.Body != nil && resp.Body.NextToken != nil {
		nextToken := resp.Body.NextToken
		req.NextToken = nextToken
		resp, err = m.Base().DescribeMetricList(req)
		if err != nil {
			log.Println(err)
			continue
		}
	}

	points := make([]types.Point, 0, 100)
	err = json.Unmarshal([]byte(*resp.Body.Datapoints), &points)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	metricName := *req.MetricName
	result := make([]types.Point, 0, len(points))
	for _, point := range points {
		if point.Val != nil {
			r := types.Point{
				UserID:     point.UserID,
				InstanceID: point.InstanceID,
				Namespace:  *req.Namespace,
				MetricName: fmt.Sprintf("%s_%s", snakeCase(metricName), "value"),
				Value:      tea.Float64(*point.Val),
			}
			result = append(result, r)
		}
		if point.Max != nil {
			r := types.Point{
				UserID:     point.UserID,
				InstanceID: point.InstanceID,
				Namespace:  *req.Namespace,
				MetricName: fmt.Sprintf("%s_%s", snakeCase(metricName), "maximum"),
				Value:      tea.Float64(*point.Max),
			}
			result = append(result, r)
		}
		if point.Min != nil {
			r := types.Point{
				UserID:     point.UserID,
				InstanceID: point.InstanceID,
				Namespace:  *req.Namespace,
				MetricName: fmt.Sprintf("%s_%s", snakeCase(metricName), "minimum"),
				Value:      tea.Float64(*point.Min),
			}
			result = append(result, r)
		}

		if point.Avg != nil {
			r := types.Point{
				UserID:     point.UserID,
				InstanceID: point.InstanceID,
				Namespace:  *req.Namespace,
				MetricName: fmt.Sprintf("%s_%s", snakeCase(metricName), "average"),
				Value:      tea.Float64(*point.Avg),
			}
			result = append(result, r)
		}
	}

	return result, nil
}

func snakeCase(in string) string {
	runes := []rune(in)
	length := len(runes)

	var out []rune
	for i := 0; i < length; i++ {
		if i > 0 && unicode.IsUpper(runes[i]) && ((i+1 < length && unicode.IsLower(runes[i+1])) || unicode.IsLower(runes[i-1])) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(runes[i]))
	}

	return string(out)
}
