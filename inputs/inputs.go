package inputs

import (
	"flashcat.cloud/categraf/types"
)

type Input interface {
	TidyConfig() error
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
