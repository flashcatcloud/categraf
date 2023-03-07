package types

import (
	"time"
)

type (
	// Metric cms20190101.DescribeMetricMetaListResponseBodyResourcesResource
	Metric Point
	Point  struct {
		Timestamp  int64    `json:"timestamp"`
		InstanceID string   `json:"instanceId"`
		ClusterID  string   `json:"clusterId"`
		NodeID     string   `json:"nodeId"`
		UserID     string   `json:"userId"`
		Min        *float64 `json:"Minimum,omitempty"`
		Max        *float64 `json:"Maximum,omitempty"`
		Avg        *float64 `json:"Average,omitempty"`
		Val        *float64 `json:"Value,omitempty"`
		Value      *float64 `josn:"-"`

		// filter
		LabelStr   string `json:"-"`
		Dimensions string `json:"-"`
		Namespace  string `json:"-"`
		MetricName string `json:"-"`
	}
)

func (p *Point) GetMetricTime() time.Time {
	sec := p.Timestamp / 1000
	ms := p.Timestamp % 1000 * 1e6
	return time.Unix(sec, ms)
}
