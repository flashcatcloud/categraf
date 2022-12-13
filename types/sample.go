package types

import (
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/pkg/conv"
)

type Sample struct {
	Metric    string            `json:"metric"`
	Timestamp time.Time         `json:"timestamp"`
	Value     interface{}       `json:"value"`
	Labels    map[string]string `json:"labels"`
}

var (
	labelReplacer  = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")
	metricReplacer = strings.NewReplacer("-", "_", ".", "_", " ", "_", "'", "_", "\"", "_")
)

func NewSample(prefix, metric string, value interface{}, labels ...map[string]string) *Sample {
	s := &Sample{
		Metric: metric,
		Value:  value,
		Labels: make(map[string]string),
	}

	if len(prefix) > 0 {
		s.Metric = prefix + "_" + metricReplacer.Replace(s.Metric)
	} else {
		s.Metric = metricReplacer.Replace(s.Metric)
	}

	for i := 0; i < len(labels); i++ {
		for k, v := range labels[i] {
			s.Labels[k] = v
		}
	}

	return s
}

func (item *Sample) ConvertTimeSeries(precision string) *prompb.TimeSeries {
	value, err := conv.ToFloat64(item.Value)
	if err != nil {
		// If the Labels is empty, it means it is abnormal data
		return nil
	}

	pt := prompb.TimeSeries{}
	timestamp := item.Timestamp.UnixMilli()
	if precision == "s" {
		timestamp = item.Timestamp.Unix()
	}

	pt.Samples = append(pt.Samples, prompb.Sample{
		Timestamp: timestamp,
		Value:     value,
	})

	// add label: metric
	pt.Labels = append(pt.Labels, prompb.Label{
		Name:  model.MetricNameLabel,
		Value: item.Metric,
	})

	// add other labels
	for k, v := range item.Labels {
		pt.Labels = append(pt.Labels, prompb.Label{
			Name:  labelReplacer.Replace(k),
			Value: v,
		})
	}

	return &pt
}
