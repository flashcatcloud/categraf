package inputs

import (
	"reflect"
	"testing"

	"flashcat.cloud/categraf/types"
	"github.com/prometheus/client_golang/prometheus"
)

func TestCollectorDescParsing(t *testing.T) {
	tests := []struct {
		desc, name         string
		labels             map[string]string
		nameErr, labelsErr bool
	}{
		{`Desc{fqName: "mongo_connections", help: "help", constLabels: {cluster="prod",region="west"}, variableLabels: {}}`, "mongo_connections", map[string]string{"cluster": "prod", "region": "west"}, false, false},
		{`Desc{fqName: "", constLabels: {}}`, "", map[string]string{}, false, false},
		{`Desc{constLabels: {x="y"}}`, "", map[string]string{"x": "y"}, true, false},
		{`Desc{fqName: "metric"}`, "metric", nil, false, true},
		{`Desc{fqName: "metric", constLabels: {invalid}}`, "metric", nil, false, true},
	}
	for _, tt := range tests {
		name, err := DescName(tt.desc)
		if (err != nil) != tt.nameErr || name != tt.name {
			t.Errorf("DescName(%q) = %q, %v", tt.desc, name, err)
		}
		labels, err := DescConstLabels(tt.desc)
		if (err != nil) != tt.labelsErr || !reflect.DeepEqual(labels, tt.labels) {
			t.Errorf("DescConstLabels(%q) = %v, %v", tt.desc, labels, err)
		}
	}
}

func TestCollectMetricLabels(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "mongodb_test_metric", Help: "test", ConstLabels: prometheus.Labels{"cluster": "prod"}}, []string{"db"})
	gauge.WithLabelValues("main").Set(42)
	list := types.NewSampleList()
	if err := Collect(gauge, list, map[string]string{"source": "agent"}); err != nil {
		t.Fatal(err)
	}
	samples := list.PopBackAll()
	if len(samples) != 1 {
		t.Fatalf("got %d samples", len(samples))
	}
	got := samples[0]
	wantLabels := map[string]string{"cluster": "prod", "db": "main", "source": "agent"}
	if got.Metric != "mongodb_test_metric" || got.Value != float64(42) || !reflect.DeepEqual(got.Labels, wantLabels) {
		t.Fatalf("unexpected sample: %+v", got)
	}
}

func BenchmarkCollectorDescParsing(b *testing.B) {
	desc := `Desc{fqName: "mongodb_collection_documents", help: "Collection documents", constLabels: {cluster="production",region="west"}, variableLabels: {database,collection}}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := DescName(desc); err != nil {
			b.Fatal(err)
		}
		if _, err := DescConstLabels(desc); err != nil {
			b.Fatal(err)
		}
	}
}
