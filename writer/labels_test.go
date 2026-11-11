package writer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/config"
)

func TestWriteTimeSeriesNormalizesBeforeDispatch(t *testing.T) {
	captured := make(chan []prompb.Label, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compressed, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, err := snappy.Decode(nil, compressed)
		var request prompb.WriteRequest
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := proto.Unmarshal(data, &request); err != nil || len(request.Timeseries) != 1 {
			t.Errorf("unexpected write request: %v, %v", request, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		captured <- request.Timeseries[0].Labels
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	w, err := newWriter(config.WriterOption{Url: server.URL, DialTimeout: 1000, Timeout: 1000})
	if err != nil {
		t.Fatal(err)
	}
	previousWriters, previousConfig := writers, config.Config
	t.Cleanup(func() { writers, config.Config = previousWriters, previousConfig })
	writers = &Writers{writerMap: map[string]Writer{"test": w}}
	config.Config = &config.ConfigType{}
	series := []prompb.TimeSeries{{Labels: []prompb.Label{
		{Name: "z", Value: "last"}, {Name: "__name__", Value: "test"}, {Name: "a", Value: "first"},
	}}}
	want := []prompb.Label{{Name: "__name__", Value: "test"}, {Name: "a", Value: "first"}, {Name: "z", Value: "last"}}
	WriteTimeSeries(series)
	select {
	case got := <-captured:
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(series[0].Labels, want) {
			t.Fatalf("labels sent = %v, input = %v, want %v", got, series[0].Labels, want)
		}
	default:
		t.Fatal("no remote write request received")
	}
	// Reusing sorted input and empty series must preserve the labels.
	normalizeTimeSeriesLabels(series)
	normalizeTimeSeriesLabels([]prompb.TimeSeries{{}})
	normalizeTimeSeriesLabels(nil)
	if !reflect.DeepEqual(series[0].Labels, want) {
		t.Fatalf("sorted labels changed: %v", series[0].Labels)
	}
}
