package metrics

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"flashcat.cloud/categraf/pkg/prom"
	"flashcat.cloud/categraf/types"
)

func MakeLabels(m *dto.Metric, labels map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range labels {
		result[key] = value
	}
	for _, pair := range m.GetLabel() {
		result[pair.GetName()] = pair.GetValue()
	}
	return result
}

func GetMetricTime(ts int64) time.Time {
	// ts millisecond
	var tm time.Time
	if ts <= 0 {
		return tm
	}
	sec := ts / 1000
	ms := ts % 1000 * 1e6
	tm = time.Unix(sec, ms)
	return tm
}

type timeFn func(int64) time.Time

func initTimeFn(fn timeFn) timeFn {
	if fn == nil {
		return GetMetricTime
	}
	return fn
}

func HandleSummary(defaultPrefix string, m *dto.Metric, tags map[string]string, metricName string, tf timeFn, slist *types.SampleList) {
	namePrefix := ""
	if !strings.HasPrefix(metricName, defaultPrefix) {
		namePrefix = defaultPrefix
	}
	fn := initTimeFn(tf)

	slist.PushFront(types.NewSample("", prom.BuildMetric(namePrefix, metricName, "count"), float64(m.GetSummary().GetSampleCount()), tags).SetTime(fn(m.GetTimestampMs())))
	slist.PushFront(types.NewSample("", prom.BuildMetric(namePrefix, metricName, "sum"), m.GetSummary().GetSampleSum(), tags).SetTime(fn(m.GetTimestampMs())))

	for _, q := range m.GetSummary().Quantile {
		slist.PushFront(types.NewSample("", prom.BuildMetric(namePrefix, metricName), q.GetValue(), tags, map[string]string{"quantile": fmt.Sprint(q.GetQuantile())}).SetTime(fn(m.GetTimestampMs())))
	}
}

func HandleHistogram(defaultPrefix string, m *dto.Metric, tags map[string]string, metricName string, tf timeFn, slist *types.SampleList) {
	namePrefix := ""
	if !strings.HasPrefix(metricName, defaultPrefix) {
		namePrefix = defaultPrefix
	}
	fn := initTimeFn(tf)

	slist.PushFront(types.NewSample("", prom.BuildMetric(namePrefix, metricName, "count"), float64(m.GetHistogram().GetSampleCount()), tags).SetTime(fn(m.GetTimestampMs())))
	slist.PushFront(types.NewSample("", prom.BuildMetric(namePrefix, metricName, "sum"), m.GetHistogram().GetSampleSum(), tags).SetTime(fn(m.GetTimestampMs())))
	slist.PushFront(types.NewSample("", prom.BuildMetric(namePrefix, metricName, "bucket"), float64(m.GetHistogram().GetSampleCount()), tags, map[string]string{"le": "+Inf"}).SetTime(fn(m.GetTimestampMs())))

	for _, b := range m.GetHistogram().Bucket {
		le := fmt.Sprint(b.GetUpperBound())
		value := float64(b.GetCumulativeCount())
		slist.PushFront(types.NewSample("", prom.BuildMetric(namePrefix, metricName, "bucket"), value, tags, map[string]string{"le": le}).SetTime(fn(m.GetTimestampMs())))
	}
}

func HandleGaugeCounter(defaultPrefix string, m *dto.Metric, tags map[string]string, metricName string, tf timeFn, slist *types.SampleList) {
	fields := getNameAndValue(m, metricName)
	fn := initTimeFn(tf)
	for metric, value := range fields {
		if !strings.HasPrefix(metric, defaultPrefix) {
			slist.PushFront(types.NewSample("", prom.BuildMetric(defaultPrefix, metric, ""), value, tags).SetTime(fn(m.GetTimestampMs())))
		} else {
			slist.PushFront(types.NewSample("", prom.BuildMetric("", metric, ""), value, tags).SetTime(fn(m.GetTimestampMs())))
		}

	}
}

func getNameAndValue(m *dto.Metric, metricName string) map[string]interface{} {
	fields := make(map[string]interface{})
	if m.Gauge != nil {
		if !math.IsNaN(m.GetGauge().GetValue()) {
			fields[metricName] = m.GetGauge().GetValue()
		}
	} else if m.Counter != nil {
		if !math.IsNaN(m.GetCounter().GetValue()) {
			fields[metricName] = m.GetCounter().GetValue()
		}
	} else if m.Untyped != nil {
		if !math.IsNaN(m.GetUntyped().GetValue()) {
			fields[metricName] = m.GetUntyped().GetValue()
		}
	}
	return fields
}

func Parse(buf []byte, header http.Header) (map[string]*dto.MetricFamily, error) {
	var parser expfmt.TextParser

	// gather even if the buffer begins with a newline
	buf = bytes.TrimPrefix(buf, []byte("\n"))

	// Read raw data
	buffer := bytes.NewBuffer(buf)
	reader := bufio.NewReader(buffer)

	// Prepare output
	metricFamilies := make(map[string]*dto.MetricFamily)
	mediatype, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err == nil && mediatype == "application/vnd.google.protobuf" &&
		params["encoding"] == "delimited" &&
		params["proto"] == "io.prometheus.client.MetricFamily" {
		for {
			mf := &dto.MetricFamily{}
			if _, ierr := pbutil.ReadDelimited(reader, mf); ierr != nil {
				if ierr == io.EOF {
					break
				}
				return nil, fmt.Errorf("reading metric family protocol buffer failed: %s", ierr)
			}
			metricFamilies[mf.GetName()] = mf
		}
	} else {
		metricFamilies, err = parser.TextToMetricFamilies(reader)
		if err != nil {
			return nil, fmt.Errorf("E! reading text format failed: %s", err)
		}
	}
	return metricFamilies, nil
}
