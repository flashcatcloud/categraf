package prometheus

import (
	"bufio"
	"bytes"
	"flashcat.cloud/categraf/pkg/conv"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/prom"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/toolkits/pkg/container/list"
)

type Parser struct {
	NamePrefix            string
	DefaultTags           map[string]string
	Header                http.Header
	IgnoreMetricsFilter   filter.Filter
	IgnoreLabelKeysFilter filter.Filter
}

func NewParser(namePrefix string, defaultTags map[string]string, header http.Header, ignoreMetricsFilter, ignoreLabelKeysFilter filter.Filter) *Parser {
	return &Parser{
		NamePrefix:            namePrefix,
		DefaultTags:           defaultTags,
		Header:                header,
		IgnoreMetricsFilter:   ignoreMetricsFilter,
		IgnoreLabelKeysFilter: ignoreLabelKeysFilter,
	}
}

func (p *Parser) Parse(buf []byte, slist *list.SafeList) error {
	var parser expfmt.TextParser

	// parse even if the buffer begins with a newline
	buf = bytes.TrimPrefix(buf, []byte("\n"))

	// Read raw data
	buffer := bytes.NewBuffer(buf)
	reader := bufio.NewReader(buffer)

	// Prepare output
	metricFamilies := make(map[string]*dto.MetricFamily)
	mediatype, params, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
	if err == nil && mediatype == "application/vnd.google.protobuf" &&
		params["encoding"] == "delimited" &&
		params["proto"] == "io.prometheus.client.MetricFamily" {
		for {
			mf := &dto.MetricFamily{}
			if _, ierr := pbutil.ReadDelimited(reader, mf); ierr != nil {
				if ierr == io.EOF {
					break
				}
				return fmt.Errorf("reading metric family protocol buffer failed: %s", ierr)
			}
			metricFamilies[mf.GetName()] = mf
		}
	} else {
		metricFamilies, err = parser.TextToMetricFamilies(reader)
		if err != nil {
			return fmt.Errorf("reading text format failed: %s", err)
		}
	}

	// read metrics
	for metricName, mf := range metricFamilies {
		if p.IgnoreMetricsFilter != nil && p.IgnoreMetricsFilter.Match(metricName) {
			continue
		}
		for _, m := range mf.Metric {
			// reading tags
			tags := p.makeLabels(m)

			if mf.GetType() == dto.MetricType_SUMMARY {
				p.handleSummary(m, tags, metricName, slist)
			} else if mf.GetType() == dto.MetricType_HISTOGRAM {
				p.handleHistogram(m, tags, metricName, slist)
			} else {
				p.handleSample(m, tags, metricName, slist)
			}
		}
	}

	return nil
}

func (p *Parser) handleSummary(m *dto.Metric, tags map[string]string, metricName string, slist *list.SafeList) {
	slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metricName, "count"), float64(m.GetSummary().GetSampleCount()), tags))
	slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metricName, "sum"), m.GetSummary().GetSampleSum(), tags))

	for _, q := range m.GetSummary().Quantile {
		slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metricName), q.GetValue(), tags, map[string]string{"quantile": fmt.Sprint(q.GetQuantile())}))
	}
}

func (p *Parser) handleHistogram(m *dto.Metric, tags map[string]string, metricName string, slist *list.SafeList) {
	slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metricName, "count"), float64(m.GetHistogram().GetSampleCount()), tags))
	slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metricName, "sum"), m.GetHistogram().GetSampleSum(), tags))
	slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metricName, "bucket"), float64(m.GetHistogram().GetSampleCount()), tags, map[string]string{"le": "+Inf"}))

	for _, b := range m.GetHistogram().Bucket {
		le := fmt.Sprint(b.GetUpperBound())
		value := float64(b.GetCumulativeCount())
		slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metricName, "bucket"), value, tags, map[string]string{"le": le}))
	}
}

func (p *Parser) handleSample(m *dto.Metric, tags map[string]string, metricName string, slist *list.SafeList) {
	fields := getNameAndValue(m, metricName)
	for metric, value := range fields {
		floatValue, err := conv.ToFloat64(value)
		if err != nil {
			continue
		}
		slist.PushFront(inputs.NewSample(prom.BuildMetric(p.NamePrefix, metric, ""), floatValue, tags))
	}
}

// Get labels from metric
func (p *Parser) makeLabels(m *dto.Metric) map[string]string {
	result := map[string]string{}

	for _, lp := range m.Label {
		if p.IgnoreLabelKeysFilter != nil && p.IgnoreLabelKeysFilter.Match(lp.GetName()) {
			continue
		}
		result[lp.GetName()] = lp.GetValue()
	}

	for key, value := range p.DefaultTags {
		result[key] = value
	}

	return result
}

// Get name and value from metric
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
