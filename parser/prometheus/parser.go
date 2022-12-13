package prometheus

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"

	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/prom"
	"flashcat.cloud/categraf/types"
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

func EmptyParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(buf []byte, slist *types.SampleList) error {
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
				p.HandleSummary(m, tags, metricName, slist)
			} else if mf.GetType() == dto.MetricType_HISTOGRAM {
				p.HandleHistogram(m, tags, metricName, slist)
			} else {
				p.handleGaugeCounter(m, tags, metricName, slist)
			}
		}
	}

	return nil
}

func (p *Parser) HandleSummary(m *dto.Metric, tags map[string]string, metricName string, slist *types.SampleList) {
	namePrefix := ""
	if !strings.HasPrefix(metricName, p.NamePrefix) {
		namePrefix = p.NamePrefix
	}

	samples := make([]*types.Sample, 0, len(m.GetSummary().Quantile)+2)
	samples = append(samples, types.NewSample("", prom.BuildMetric(namePrefix, metricName, "count"), float64(m.GetSummary().GetSampleCount()), tags))
	samples = append(samples, types.NewSample("", prom.BuildMetric(namePrefix, metricName, "sum"), m.GetSummary().GetSampleSum(), tags))

	for _, q := range m.GetSummary().Quantile {
		samples = append(samples, types.NewSample("", prom.BuildMetric(namePrefix, metricName, "quantile"), q.GetValue(), tags, map[string]string{"quantile": fmt.Sprint(q.GetQuantile())}))
	}
	slist.PushFrontN(samples)
}

func (p *Parser) HandleHistogram(m *dto.Metric, tags map[string]string, metricName string, slist *types.SampleList) {
	namePrefix := ""
	if !strings.HasPrefix(metricName, p.NamePrefix) {
		namePrefix = p.NamePrefix
	}

	samples := make([]*types.Sample, 0, len(m.GetHistogram().Bucket)+3)
	samples = append(samples, types.NewSample("", prom.BuildMetric(namePrefix, metricName, "count"), float64(m.GetHistogram().GetSampleCount()), tags))
	samples = append(samples, types.NewSample("", prom.BuildMetric(namePrefix, metricName, "sum"), m.GetHistogram().GetSampleSum(), tags))
	samples = append(samples, types.NewSample("", prom.BuildMetric(namePrefix, metricName, "bucket"), float64(m.GetHistogram().GetSampleCount()), tags, map[string]string{"le": "+Inf"}))

	for _, b := range m.GetHistogram().Bucket {
		le := fmt.Sprint(b.GetUpperBound())
		value := float64(b.GetCumulativeCount())
		samples = append(samples, types.NewSample("", prom.BuildMetric(namePrefix, metricName, "bucket"), value, tags, map[string]string{"le": le}))
	}
	slist.PushFrontN(samples)
}

func (p *Parser) handleGaugeCounter(m *dto.Metric, tags map[string]string, metricName string, slist *types.SampleList) {
	fields := getNameAndValue(m, metricName)
	for metric, value := range fields {
		if !strings.HasPrefix(metric, p.NamePrefix) {
			slist.PushFront(types.NewSample("", prom.BuildMetric(p.NamePrefix, metric, ""), value, tags))
		} else {
			slist.PushFront(types.NewSample("", prom.BuildMetric("", metric, ""), value, tags))
		}
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
