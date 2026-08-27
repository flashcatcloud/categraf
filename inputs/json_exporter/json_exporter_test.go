package json_exporter

import (
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func TestGatherExtractsValueAndObjectMetrics(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"counter": 1234,
			"timestamp_ms": 1657568506000,
			"location": "mars",
			"values": [
				{"id":"id-A","count":1,"some_boolean":true,"state":"ACTIVE"},
				{"id":"id-B","count":2,"some_boolean":true,"state":"INACTIVE"},
				{"id":"id-C","count":3,"some_boolean":false,"state":"ACTIVE"}
			]
		}`)
	}))
	defer server.Close()

	ins := &Instance{
		Targets: []string{server.URL},
		Metrics: []Metric{
			{
				Name: "global_value",
				Path: "{.counter}",
				Labels: map[string]string{
					"environment": "beta",
					"location":    "planet-{.location}",
				},
			},
			{
				Name: "value",
				Type: ObjectScrape,
				Path: `{.values[?(@.state == "ACTIVE")]}`,
				Labels: map[string]string{
					"environment": "beta",
					"id":          "{.id}",
				},
				Values: map[string]string{
					"active":  "1",
					"boolean": "{.some_boolean}",
					"count":   "{.count}",
				},
			},
			{
				Name:            "timestamped_value",
				Path:            "{.counter}",
				EpochTimestamp:  "{.timestamp_ms}",
				AllowMissingKey: false,
			},
			{
				Name:            "missing_value",
				Path:            "{.missing}",
				AllowMissingKey: true,
			},
		},
	}
	ins.LabelKey = "target"

	if err := ins.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	slist := types.NewSampleList()
	ins.Gather(slist)
	samples := indexSamples(slist.PopBackAll())

	requireSample(t, samples, "json_exporter_up", 1, map[string]string{"target": server.URL})
	requireSample(t, samples, "json_exporter_global_value", 1234, map[string]string{
		"environment": "beta",
		"location":    "planet-mars",
		"target":      server.URL,
	})
	requireSample(t, samples, "json_exporter_value_active", 1, map[string]string{"id": "id-A"})
	requireSample(t, samples, "json_exporter_value_count", 3, map[string]string{"id": "id-C"})
	requireSample(t, samples, "json_exporter_value_boolean", 0, map[string]string{"id": "id-C"})

	timestamped := requireSample(t, samples, "json_exporter_timestamped_value", 1234, nil)
	wantTimestamp := time.UnixMilli(1657568506000)
	if !timestamped.Timestamp.Equal(wantTimestamp) {
		t.Fatalf("timestamp = %v, want %v", timestamped.Timestamp, wantTimestamp)
	}
	if got := samples["json_exporter_missing_value"]; len(got) != 0 {
		t.Fatalf("missing-key metric should be omitted, got %#v", got)
	}
}

func TestGatherUsesHTTPConfiguration(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("X-Test"); got != "json-exporter" {
			t.Errorf("X-Test = %q, want json-exporter", got)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "password" {
			t.Errorf("BasicAuth = %q/%q/%v", username, password, ok)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll(body) error = %v", err)
		}
		if got := string(body); got != `{"query":"health"}` {
			t.Errorf("body = %q", got)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"healthy":true}`)
	}))
	defer server.Close()

	ins := &Instance{
		Targets:          []string{server.URL},
		ValidStatusCodes: []int{http.StatusCreated},
		Metrics: []Metric{{
			Name: "healthy",
			Path: "{.healthy}",
		}},
		HTTPCommonConfig: config.HTTPCommonConfig{
			Method:   http.MethodPost,
			Username: "user",
			Password: "password",
			Body:     `{"query":"health"}`,
			Headers:  map[string]string{"X-Test": "json-exporter"},
		},
	}

	if err := ins.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	slist := types.NewSampleList()
	ins.Gather(slist)
	samples := indexSamples(slist.PopBackAll())
	requireSample(t, samples, "json_exporter_up", 1, nil)
	requireSample(t, samples, "json_exporter_healthy", 1, nil)
}

func TestGatherMarksTargetDownOnHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ins := &Instance{
		Targets: []string{server.URL},
		Metrics: []Metric{{Name: "value", Path: "{.value}"}},
	}
	if err := ins.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	slist := types.NewSampleList()
	ins.Gather(slist)
	samples := indexSamples(slist.PopBackAll())
	requireSample(t, samples, "json_exporter_up", 0, nil)
	if got := samples["json_exporter_value"]; len(got) != 0 {
		t.Fatalf("value metric should not be emitted for failed request, got %#v", got)
	}
}

func TestInitRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ins  *Instance
	}{
		{name: "targets are required", ins: &Instance{Metrics: []Metric{{Name: "value", Path: "{.value}"}}}},
		{name: "metrics are required", ins: &Instance{Targets: []string{"http://example.com"}}},
		{name: "metric name is required", ins: &Instance{Targets: []string{"http://example.com"}, Metrics: []Metric{{Path: "{.value}"}}}},
		{name: "metric path is required", ins: &Instance{Targets: []string{"http://example.com"}, Metrics: []Metric{{Name: "value"}}}},
		{name: "metric type is validated", ins: &Instance{Targets: []string{"http://example.com"}, Metrics: []Metric{{Name: "value", Path: "{.value}", Type: "histogram"}}}},
		{name: "object values are required", ins: &Instance{Targets: []string{"http://example.com"}, Metrics: []Metric{{Name: "value", Path: "{.values[*]}", Type: ObjectScrape}}}},
		{name: "jsonpath is validated", ins: &Instance{Targets: []string{"http://example.com"}, Metrics: []Metric{{Name: "value", Path: "{.value"}}}},
		{name: "target scheme is validated", ins: &Instance{Targets: []string{"file:///tmp/data.json"}, Metrics: []Metric{{Name: "value", Path: "{.value}"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ins.Init(); err == nil {
				t.Fatal("Init() error = nil, want non-nil")
			}
		})
	}

	ins := &Instance{Metrics: []Metric{{Name: "value", Path: "{.value}"}}}
	if err := ins.Init(); !errors.Is(err, types.ErrInstancesEmpty) {
		t.Fatalf("Init() error = %v, want ErrInstancesEmpty", err)
	}
}

func TestSanitizeValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  float64
		ok    bool
	}{
		{input: "1234", want: 1234, ok: true},
		{input: "1234.5", want: 1234.5, ok: true},
		{input: "true", want: 1, ok: true},
		{input: "FALSE", want: 0, ok: true},
		{input: "<nil>", want: math.NaN(), ok: true},
		{input: "text", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := sanitizeValue(tt.input)
			if (err == nil) != tt.ok {
				t.Fatalf("sanitizeValue(%q) error = %v, ok = %v", tt.input, err, tt.ok)
			}
			if tt.ok && !(math.IsNaN(tt.want) && math.IsNaN(got)) && got != tt.want {
				t.Fatalf("sanitizeValue(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func indexSamples(samples []*types.Sample) map[string][]*types.Sample {
	indexed := make(map[string][]*types.Sample)
	for _, sample := range samples {
		indexed[sample.Metric] = append(indexed[sample.Metric], sample)
	}
	return indexed
}

func requireSample(t *testing.T, samples map[string][]*types.Sample, name string, value float64, labels map[string]string) *types.Sample {
	t.Helper()

	for _, sample := range samples[name] {
		gotValue, ok := sample.Value.(float64)
		if !ok || gotValue != value {
			continue
		}
		matches := true
		for key, want := range labels {
			if sample.Labels[key] != want {
				matches = false
				break
			}
		}
		if matches {
			return sample
		}
	}

	t.Fatalf("sample %q value=%v labels=%v not found in %#v", name, value, labels, samples[name])
	return nil
}
