package exporter

import (
	"reflect"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestSpecialConversions(t *testing.T) {
	input := &rawMetric{fqName: "mongodb_ss_opLatencies_reads_ops", val: 7, vt: prometheus.CounterValue}
	for i := 0; i < 2; i++ {
		got := metricRenameAndLabel(input, specialConversions)
		if len(got) != 1 {
			t.Fatalf("got %d conversions", len(got))
		}
		if got[0].fqName != "mongodb_ss_opLatencies_ops" || got[0].val != 7 || got[0].vt != prometheus.CounterValue || !reflect.DeepEqual(got[0].ln, []string{"op_type"}) || !reflect.DeepEqual(got[0].lv, []string{"reads"}) {
			t.Fatalf("unexpected conversion: %+v", got[0])
		}
	}
}

func TestCompatibleConversions(t *testing.T) {
	tests := []struct {
		input  *rawMetric
		name   string
		ln, lv []string
	}{
		{&rawMetric{fqName: "mongodb_ss_asserts", val: 3, vt: prometheus.CounterValue, ln: []string{"assert_type"}, lv: []string{"regular"}}, "mongodb_asserts_total", []string{"type"}, []string{"regular"}},
		{&rawMetric{fqName: "mongodb_ss_wt_cache_bytes_currently_in_the_cache", val: 3, vt: prometheus.CounterValue}, "mongodb_mongod_wiredtiger_cache_bytes", []string{"type"}, []string{"total"}},
	}
	for i := 0; i < 2; i++ {
		for _, tt := range tests {
			got := metricRenameAndLabel(tt.input, conversions)
			if len(got) != 1 {
				t.Fatalf("%s: got %d conversions", tt.input.fqName, len(got))
			}
			if got[0].fqName != tt.name || got[0].val != 3 || got[0].vt != prometheus.CounterValue || !reflect.DeepEqual(got[0].ln, tt.ln) || !reflect.DeepEqual(got[0].lv, tt.lv) {
				t.Fatalf("unexpected conversion: %+v", got[0])
			}
		}
	}
}
