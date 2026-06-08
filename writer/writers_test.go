package writer

import (
	"testing"

	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func TestReloadWritersReplacesWriterMapAndPreservesQueue(t *testing.T) {
	oldConfig := config.Config
	oldWriters := writers
	defer func() {
		config.Config = oldConfig
		writers = oldWriters
	}()

	queue := types.NewSafeListLimited[*prompb.TimeSeries](10)
	writers = &Writers{
		writerMap: map[string]Writer{
			"http://old.example/prometheus/v1/write": {},
		},
		queue: queue,
	}
	config.Config = &config.ConfigType{
		WriterOpt: config.WriterOpt{Batch: 1000, ChanSize: 10},
		Writers: []config.WriterOption{
			{
				Url:                 "http://new.example/prometheus/v1/write",
				Timeout:             5000,
				DialTimeout:         2500,
				MaxIdleConnsPerHost: 100,
			},
		},
	}

	if err := ReloadWriters(); err != nil {
		t.Fatalf("ReloadWriters error = %v", err)
	}
	if writers.queue != queue {
		t.Fatal("ReloadWriters replaced the queue")
	}
	if _, ok := writers.writerMap["http://new.example/prometheus/v1/write"]; !ok {
		t.Fatal("new writer was not installed")
	}
	if _, ok := writers.writerMap["http://old.example/prometheus/v1/write"]; ok {
		t.Fatal("old writer was not removed")
	}
}
