package vsphere

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	gtypes "github.com/vmware/govmomi/vim25/types"
)

func TestChunkifyQueryLimits(t *testing.T) {
	for _, tc := range []struct {
		name             string
		resource         string
		realTime         bool
		maxMetrics       int
		maxObjects       int
		objects          int
		metrics          int
		metricLimit      int
		wantQueries      int
		splitTimeBuckets bool
	}{
		{name: "historical defaults", maxMetrics: 256, maxObjects: 256, objects: 30, metrics: 10, metricLimit: 256},
		{name: "historical reduced metrics", maxMetrics: 4, maxObjects: 256, objects: 100, metrics: 3, metricLimit: 4},
		{name: "historical both limits reduced", maxMetrics: 4, maxObjects: 4, objects: 10, metrics: 3, metricLimit: 4},
		{name: "historical large object", maxMetrics: 4, maxObjects: 256, objects: 1, metrics: 11, metricLimit: 4},
		{name: "historical exact metric limit", maxMetrics: 4, maxObjects: 256, objects: 2, metrics: 2, metricLimit: 4, wantQueries: 1},
		{name: "historical ignores realtime object limit", maxMetrics: 8, maxObjects: 1, objects: 3, metrics: 2, metricLimit: 8, wantQueries: 1},
		{name: "historical single metric", maxMetrics: 1, maxObjects: 1, objects: 3, metrics: 5, metricLimit: 1},
		{name: "historical multiple time buckets", maxMetrics: 5, maxObjects: 256, objects: 5, metrics: 8, metricLimit: 5, splitTimeBuckets: true},
		{name: "historical absolute metric limit", maxMetrics: maxRealtimeMetrics + 10, maxObjects: 256, objects: 1, metrics: maxRealtimeMetrics + 1, metricLimit: maxRealtimeMetrics},
		{name: "cluster aggregate limit", resource: "cluster", maxMetrics: 256, maxObjects: 256, objects: 5, metrics: 6, metricLimit: 10},
		{name: "cluster smaller configured limit", resource: "cluster", maxMetrics: 4, maxObjects: 256, objects: 5, metrics: 3, metricLimit: 4},
		{name: "realtime object limit", realTime: true, maxMetrics: 1, maxObjects: 2, objects: 5, metrics: 3, metricLimit: maxRealtimeMetrics, wantQueries: 3},
		{name: "realtime exact object limit", realTime: true, maxMetrics: 1, maxObjects: 2, objects: 2, metrics: 3, metricLimit: maxRealtimeMetrics, wantQueries: 1},
		{name: "realtime multiple time buckets", realTime: true, maxMetrics: 1, maxObjects: 2, objects: 5, metrics: 3, metricLimit: maxRealtimeMetrics, wantQueries: 5, splitTimeBuckets: true},
		{name: "realtime large object", realTime: true, maxMetrics: 256, maxObjects: 2, objects: 1, metrics: maxRealtimeMetrics + 1, metricLimit: maxRealtimeMetrics},
		{name: "realtime aggregate metric limit", realTime: true, maxMetrics: 256, maxObjects: 256, objects: 2, metrics: maxRealtimeMetrics/2 + 1, metricLimit: maxRealtimeMetrics},
		{name: "no objects", maxMetrics: 4, maxObjects: 4, metrics: 3, metricLimit: 4},
		{name: "no metrics", maxMetrics: 4, maxObjects: 4, objects: 3, metricLimit: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Endpoint{
				Parent: &Instance{
					MaxQueryMetrics:    tc.maxMetrics,
					MaxQueryObjects:    tc.maxObjects,
					CollectConcurrency: 4,
					MetricLookback:     3,
				},
				URL:              &url.URL{Host: "vcenter.test"},
				hwMarks:          NewTSCache(time.Hour, false),
				metricNameLookup: make(map[int32]string),
			}
			resource := tc.resource
			if resource == "" {
				resource = "resourcepool"
			}
			res := &resourceKind{name: resource, realTime: tc.realTime, sampling: 300, objects: make(objectMap)}
			if tc.realTime {
				res.sampling = 20
			}
			now := time.Date(2026, time.November, 11, 0, 0, 0, 0, time.UTC)
			latest := now.Add(-time.Duration(res.sampling) * time.Second)
			defaultStart := latest.Add(-time.Duration(res.sampling) * time.Second * 2)
			cachedStart := defaultStart.Add(-20 * time.Second)
			for i := 1; i <= tc.metrics; i++ {
				id := int32(i)
				e.metricNameLookup[id] = fmt.Sprintf("metric-%d", i)
				res.metrics = append(res.metrics, gtypes.PerfMetricId{CounterId: id})
			}
			for i := 0; i < tc.objects; i++ {
				id := fmt.Sprintf("object-%d", i)
				res.objects[id] = &objectRef{ref: gtypes.ManagedObjectReference{Type: "ResourcePool", Value: id}}
				if tc.splitTimeBuckets {
					for j := 2; j <= tc.metrics; j += 2 {
						e.hwMarks.Put(id, e.metricNameLookup[int32(j)], cachedStart)
					}
				}
			}
			// Unknown counters must still be skipped when batching queries.
			res.metrics = append(res.metrics, gtypes.PerfMetricId{CounterId: -1})

			var mux sync.Mutex
			var chunks []queryChunk
			e.chunkify(context.Background(), res, now, latest, func(chunk queryChunk) {
				mux.Lock()
				defer mux.Unlock()
				chunks = append(chunks, chunk)
			})
			if tc.wantQueries > 0 && len(chunks) != tc.wantQueries {
				t.Errorf("submitted %d queries, want %d", len(chunks), tc.wantQueries)
			}

			seen := make(map[string]map[int32]int)
			for _, chunk := range chunks {
				if len(chunk) == 0 {
					t.Fatal("submitted an empty query")
				}
				if tc.realTime && len(chunk) > tc.maxObjects {
					t.Errorf("query has %d specs, object limit is %d", len(chunk), tc.maxObjects)
				}
				metrics := 0
				for _, spec := range chunk {
					if len(spec.MetricId) == 0 {
						t.Fatal("submitted a spec without metrics")
					}
					if _, ok := res.objects[spec.Entity.Value]; !ok {
						t.Fatalf("unexpected entity: %v", spec.Entity)
					}
					if spec.IntervalId != res.sampling || spec.MaxSample != maxSampleConst || spec.Format != "normal" || spec.EndTime == nil || !spec.EndTime.Equal(now) {
						t.Fatalf("query metadata was not preserved: %+v", spec)
					}
					if seen[spec.Entity.Value] == nil {
						seen[spec.Entity.Value] = make(map[int32]int)
					}
					for _, metric := range spec.MetricId {
						if metric.CounterId < 1 || int(metric.CounterId) > tc.metrics {
							t.Fatalf("unexpected counter: %d", metric.CounterId)
						}
						start := defaultStart
						if tc.splitTimeBuckets && metric.CounterId%2 == 0 {
							start = cachedStart
						}
						if spec.StartTime == nil || !spec.StartTime.Equal(start) {
							t.Fatalf("wrong start time for %s counter %d: %v, want %v", spec.Entity.Value, metric.CounterId, spec.StartTime, start)
						}
						seen[spec.Entity.Value][metric.CounterId]++
					}
					metrics += len(spec.MetricId)
				}
				if metrics > tc.metricLimit {
					t.Errorf("query has %d metrics, limit is %d", metrics, tc.metricLimit)
				}
			}
			for id := range res.objects {
				for i := 1; i <= tc.metrics; i++ {
					if count := seen[id][int32(i)]; count != 1 {
						t.Fatalf("%s counter %d was queried %d times, want once", id, i, count)
					}
				}
			}
		})
	}
}
