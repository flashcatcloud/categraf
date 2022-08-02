package types

import (
	"strings"
	"time"

	"flashcat.cloud/categraf/pkg/conv"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
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

func TimeSeriesConvertSample(timeSeries *prompb.TimeSeries) *Sample {
	var t time.Time
	if timeSeries.Samples[0].Timestamp > 0 && len(timeSeries.Samples) == 13 {
		t = time.Unix(0, timeSeries.Samples[0].Timestamp*1e6)
	} else {
		t = time.Now()
	}

	sample := &Sample{
		Timestamp: t,
		Value:     timeSeries.Samples[0].Value,
		Labels:    make(map[string]string),
	}

	var metric string
	for _, label := range timeSeries.Labels {
		if label.Name == model.MetricNameLabel {
			metric = label.Value
			continue
		}

		sample.Labels[label.Name] = label.Value
	}

	sample.Metric = metric
	return sample
}

func (item *Sample) ConvertTimeSeries(precision string) prompb.TimeSeries {
	value, err := conv.ToFloat64(item.Value)
	if err != nil {
		// If the Labels is empty, it means it is abnormal data
		return prompb.TimeSeries{}
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

	return pt
}
