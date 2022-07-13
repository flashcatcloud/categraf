package types

import (
	"time"

	"flashcat.cloud/categraf/pkg/conv"
)

type Sample struct {
	Metric    string            `json:"metric"`
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
}

func NewSample(metric string, value interface{}, labels ...map[string]string) *Sample {
	floatValue, err := conv.ToFloat64(value)
	if err != nil {
		return nil
	}

	s := &Sample{
		Metric: metric,
		Value:  floatValue,
		Labels: make(map[string]string),
	}

	for i := 0; i < len(labels); i++ {
		for k, v := range labels[i] {
			if v == "-" {
				continue
			}
			s.Labels[k] = v
		}
	}

	return s
}
