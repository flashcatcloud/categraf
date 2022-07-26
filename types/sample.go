package types

import (
	"strings"
	"time"
)

type Sample struct {
	Metric    string            `json:"metric"`
	Timestamp time.Time         `json:"timestamp"`
	Value     interface{}       `json:"value"`
	Labels    map[string]string `json:"labels"`
}

var metricReplacer = strings.NewReplacer("-", "_", ".", "_", " ", "_", "'", "_", "\"", "_")

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
