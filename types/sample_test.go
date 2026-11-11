package types

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
)

func TestConvertTimeSeriesPreservesOpenSourceLabels(t *testing.T) {
	labels := map[string]string{"if-name": "edge", "if_name": "core", "Z": "upper"}
	s := NewSample("", "test", 2, labels).SetTime(time.UnixMilli(123456))
	want := []prompb.Label{
		{Name: "Z", Value: "upper"},
		{Name: "__name__", Value: "test"},
		{Name: "if_name", Value: "core"},
		{Name: "if_name", Value: "edge"},
	}
	for _, tc := range []struct {
		precision string
		timestamp int64
	}{{"ms", 123456}, {"s", 123000}, {"m", 120000}} {
		got := s.ConvertTimeSeries(tc.precision)
		if !reflect.DeepEqual(got.Labels, want) {
			t.Fatalf("labels = %v, want %v", got.Labels, want)
		}
		if len(got.Samples) != 1 || got.Samples[0].Timestamp != tc.timestamp || got.Samples[0].Value != 2 {
			t.Fatalf("precision %q: samples = %v", tc.precision, got.Samples)
		}
	}
	if !reflect.DeepEqual(s.Labels, labels) {
		t.Fatalf("raw labels changed: %v", s.Labels)
	}
}

func TestConvertTimeSeriesSortsLabels(t *testing.T) {
	s := NewSample("", "mongodb_test", 1, map[string]string{"z": "1", "a-b": "2", "m": "3"})
	got := s.ConvertTimeSeries("ms")
	var names []string
	for _, l := range got.Labels {
		names = append(names, l.Name)
	}
	if want := []string{"__name__", "a_b", "m", "z"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("labels = %v, want %v", names, want)
	}
	if len(got.Samples) != 1 || got.Samples[0].Value != 1 {
		t.Fatalf("samples = %+v", got.Samples)
	}
}

func BenchmarkConvertTimeSeries(b *testing.B) {
	s := NewSample("", "mongodb_collstats_storageStats_size", 1, map[string]string{
		"agent_hostname": "host", "cl_id": "id", "cl_role": "shardsvr", "collection": "events",
		"database": "flashduty", "rs_nm": "rs0", "rs_state": "1", "instance": "127.0.0.1:27017",
	})
	b.ReportAllocs()
	for b.Loop() {
		s.ConvertTimeSeries("ms")
	}
}
