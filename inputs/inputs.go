package inputs

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/types"
)

type Input interface {
	Init() error
	GetInputName() string
	GetInterval() config.Duration
	Gather() []*types.Sample
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
		floatValue, err := conv.ToFloat64(value)
		if err != nil {
			continue
		}
		samples = append(samples, NewSample(metric, floatValue, labels...))
	}

	return samples
}
