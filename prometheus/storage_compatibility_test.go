//go:build !no_prometheus

package prometheus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	clientmetrics "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/agent"
	"github.com/prometheus/prometheus/util/logging"
)

func TestPrometheusStorageCompatibility(t *testing.T) {
	for _, mode := range []string{"agent", "tsdb"} {
		t.Run(mode, func(t *testing.T) {
			received := make(chan struct{}, 1)
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				compressed, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				body, err := snappy.Decode(nil, compressed)
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				var request prompb.WriteRequest
				if err := request.Unmarshal(body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				for _, series := range request.Timeseries {
					for _, label := range series.Labels {
						if label.Name == "__name__" && label.Value == "migration_probe" {
							for _, sample := range series.Samples {
								if sample.Value == 7 {
									select {
									case received <- struct{}{}:
									default:
									}
								}
							}
						}
					}
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer receiver.Close()
			exporter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain; version=0.0.4")
				_, _ = io.WriteString(w, "# TYPE migration_probe gauge\nmigration_probe 7\n")
			}))
			defer exporter.Close()
			logger := slog.New(slog.DiscardHandler)
			registry := clientmetrics.NewRegistry()
			dir := t.TempDir()
			local := &readyStorage{stats: tsdb.NewDBStats()}
			readyScraper := &readyScrapeManager{}
			remoteStore := remote.NewStorage(logger, registry, local.StartTime, dir, 2*time.Second, readyScraper, false)
			var db storage.Storage
			var err error
			if mode == "agent" {
				opts := agent.DefaultOptions()
				opts.WALSegmentSize = 10 * 1024 * 1024
				db, err = agent.Open(logger, registry, remoteStore, dir, opts)
			} else {
				opts := tsdb.DefaultOptions()
				opts.WALSegmentSize = 10 * 1024 * 1024
				db, err = tsdb.Open(dir, logger, registry, opts, nil)
			}
			if err != nil {
				_ = remoteStore.Close()
				t.Fatal(err)
			}
			local.Set(db, 0)
			switch opened := db.(type) {
			case *agent.DB:
				opened.SetWriteNotified(remoteStore)
			case *tsdb.DB:
				opened.SetWriteNotified(remoteStore)
			}
			if _, err := local.StartTime(); err != nil {
				t.Fatal(err)
			}
			fanout := storage.NewFanout(logger, local, remoteStore)
			defer func() {
				if err := fanout.Close(); err != nil {
					t.Error(err)
				}
			}()
			manager, err := scrape.NewManager(nil, logger, logging.NewJSONFileLogger, nil, fanout, registry)
			if err != nil {
				t.Fatal(err)
			}
			readyScraper.Set(manager)
			config, err := promconfig.Load(fmt.Sprintf(`global:
  scrape_interval: 100ms
  scrape_timeout: 100ms
scrape_configs:
- job_name: smoke
  static_configs:
  - targets: [%q]
remote_write:
- url: %q
  queue_config:
    min_shards: 1
    max_shards: 1
    capacity: 100
    max_samples_per_send: 10
    batch_send_deadline: 100ms
  metadata_config:
    send: false
`, strings.TrimPrefix(exporter.URL, "http://"), receiver.URL), logger)
			if err != nil {
				t.Fatal(err)
			}
			if err := remoteStore.ApplyConfig(config); err != nil {
				t.Fatal(err)
			}
			if err := manager.ApplyConfig(config); err != nil {
				t.Fatal(err)
			}
			updates := make(chan map[string][]*targetgroup.Group, 1)
			updates <- map[string][]*targetgroup.Group{"smoke": {{Source: "fixture", Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(strings.TrimPrefix(exporter.URL, "http://"))}}}}}
			done := make(chan error, 1)
			go func() { done <- manager.Run(updates) }()
			defer func() {
				manager.Stop()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			select {
			case <-received:
			case <-ctx.Done():
				t.Fatal("sample did not reach remote write through scrape and WAL storage")
			}
		})
	}
}
