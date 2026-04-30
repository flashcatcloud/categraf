package collector

import (
	"strings"
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
	defer isolateCollectorGlobals()()

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

func TestNewNodeCollectorReturnsErrorWhenCollectorFactoryPanics(t *testing.T) {
	defer isolateCollectorGlobals()()

	const collectorName = "test_panics"
	registerCollector(collectorName, defaultDisabled, func() (Collector, error) {
		panic("factory boom")
	})

	_, err := NewNodeCollector(false, "--collector."+collectorName)
	if err == nil {
		t.Fatal("expected error from panicking collector factory")
	}

	if !strings.Contains(err.Error(), collectorName) {
		t.Fatalf("expected error to include collector name %q, got %q", collectorName, err)
	}
	if !strings.Contains(err.Error(), "factory boom") {
		t.Fatalf("expected error to include panic value, got %q", err)
	}
}

func isolateCollectorGlobals() func() {
	oldFactories := factories
	oldCollectorState := collectorState
	oldForcedCollectors := forcedCollectors

	factories = make(map[string]func() (Collector, error))
	collectorState = make(map[string]*bool)
	forcedCollectors = map[string]bool{}

	return func() {
		factories = oldFactories
		collectorState = oldCollectorState
		forcedCollectors = oldForcedCollectors
	}
}
