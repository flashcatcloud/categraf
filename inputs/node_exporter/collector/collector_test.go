package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

type parameterizedTestCollector struct {
	value string
}

func (c *parameterizedTestCollector) Update(ch chan<- prometheus.Metric) error {
	return nil
}

func TestNewNodeCollectorRebuildsCollectorsWhenFilterParamsChange(t *testing.T) {
	oldFactories := factories
	oldCollectorState := collectorState
	oldForcedCollectors := forcedCollectors
	defer func() {
		factories = oldFactories
		collectorState = oldCollectorState
		forcedCollectors = oldForcedCollectors
	}()

	factories = make(map[string]func() (Collector, error))
	collectorState = make(map[string]*bool)
	forcedCollectors = map[string]bool{}

	const collectorName = "test_parameterized"
	currentValue := "first"
	registerCollector(collectorName, defaultDisabled, func() (Collector, error) {
		return &parameterizedTestCollector{value: currentValue}, nil
	})

	if _, err := NewNodeCollector(false,
		"--collector."+collectorName,
		"--collector."+collectorName+".value=first",
	); err != nil {
		t.Fatalf("create first node collector: %v", err)
	}

	currentValue = "second"
	nc, err := NewNodeCollector(false,
		"--collector."+collectorName,
		"--collector."+collectorName+".value=second",
	)
	if err != nil {
		t.Fatalf("create second node collector: %v", err)
	}

	got := nc.Collectors[collectorName].(*parameterizedTestCollector).value
	if got != "second" {
		t.Fatalf("expected rebuilt collector with value %q, got %q", "second", got)
	}
}
