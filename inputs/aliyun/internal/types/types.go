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

		// alb
		LoadBalancerID   string `json:"loadBalancerId"`
		ListenerProtocol string `json:"listenerProtocol"`
		ListenerPort     string `json:"listenerPort"`

		Device string `json:"device"`

		// acs_cen
		CenID     string `json:"cenId"`
		SrcRegion string `json:"srcRegionId"`
		DstRegion string `json:"dstRegionId"`

		// filter
		LabelStr   string `json:"-"`
		Dimensions string `json:"-"`
		Namespace  string `json:"-"`
		MetricName string `json:"-"`

		// rocketMq
		GroupID string   `json:"groupId,omitempty"`
		Sum     *float64 `json:"Sum,omitempty"`
		Topic   string   `json:"topic,omitempty"`

		// rabbitMq
		ExchangeName string `json:"exchangeName,omitempty"`
		VHostName    string `json:"vhostName,omitempty"`
		RegionID     string `json:"regionId,omitempty"`
		QueueName    string `json:"queueName,omitempty"`
		VHostQueue   string `json:"vhostQueue,omitempty"`
		//hybrid db
		Hostname string `json:"hostname,omitempty"`
	}
)

func (p *Point) GetMetricTime() time.Time {
	sec := p.Timestamp / 1000
	ms := p.Timestamp % 1000 * 1e6
	return time.Unix(sec, ms)
}
