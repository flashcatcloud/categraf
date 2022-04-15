package inputs

import (
	"fmt"
	"strconv"

	"flashcat.cloud/categraf/types"
)

type Input interface {
	Init() error
	StartGoroutines(chan *types.Sample)
	StopGoroutines()
}

type Creator func() Input

var InputCreators = map[string]Creator{}

func Add(name string, creator Creator) {
	InputCreators[name] = creator
}

func NewSample(metric string, value float64, labels ...map[string]string) *types.Sample {
	s := &types.Sample{
		Metric: metric,
		Value:  value,
		Labels: make(map[string]string),
	}

	for i := 0; i < len(labels); i++ {
		for k, v := range labels[i] {
			s.Labels[k] = v
		}
	}

	return s
}

func NewSamples(fields map[string]interface{}, labels ...map[string]string) []*types.Sample {
	count := len(fields)
	samples := make([]*types.Sample, 0, count)

	for metric, value := range fields {
		floatValue, err := toFloat64(value)
		if err != nil {
			continue
		}
		samples = append(samples, NewSample(metric, floatValue, labels...))
	}

	return samples
}

func toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, nil
		}

		// try bool
		b, err := strconv.ParseBool(v)
		if err == nil {
			if b {
				return 1, nil
			} else {
				return 0, nil
			}
		}

		if v == "Yes" || v == "yes" || v == "YES" || v == "Y" || v == "ON" || v == "on" || v == "On" {
			return 1, nil
		}

		if v == "No" || v == "no" || v == "NO" || v == "N" || v == "OFF" || v == "off" || v == "Off" {
			return 0, nil
		}

		return 0, fmt.Errorf("unparseable value %v", v)
	case float64:
		return v, nil
	case uint64:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case bool:
		if v {
			return 1, nil
		} else {
			return 0, nil
		}
	case int:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case float32:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("unparseable value %v", v)
	}
}
