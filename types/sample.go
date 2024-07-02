package types

import (
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/pkg/conv"
)

type (
	pair struct {
		key string
		val string
	}

	Sample struct {
		Metric    string            `json:"metric"`
		Timestamp time.Time         `json:"timestamp"`
		Value     interface{}       `json:"value"`
		Labels    map[string]string `json:"labels"`
	}
)

var (
	labelReplacer  = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")
	metricReplacer = strings.NewReplacer("-", "_", ".", "_", " ", "_", "'", "_", "\"", "_")
	zeroTime       = time.Unix(0, 0)
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
	switch precision {
	case "s":
		timestamp = timestamp / 1000 * 1000
	case "m":
		ts := timestamp / 1000 * 1000 // ms
		timestamp = ts - ts%60000
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

	// sort labels
	pairs := make([]pair, 0, len(item.Labels))
	for k, v := range item.Labels {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].val < pairs[j].val
		}
		return pairs[i].key < pairs[j].key
	})

	// add other labels
	for _, p := range pairs {
		pt.Labels = append(pt.Labels, prompb.Label{
			Name:  labelReplacer.Replace(p.key),
			Value: p.val,
		})
	}

	return &pt
}

func (s *Sample) SetTime(t time.Time) *Sample {
	if t.IsZero() || zeroTime.Equal(t) {
		return s
	}
	s.Timestamp = t
	return s
}
