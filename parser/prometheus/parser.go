package prometheus

import (
	"bytes"
	"log"
	"math"
	"mime"
	"net/http"

	dto "github.com/prometheus/client_model/go"

	"flashcat.cloud/categraf/pkg/filter"
	util "flashcat.cloud/categraf/pkg/metrics"
	"flashcat.cloud/categraf/types"
)

const (
	MetricHeader = "# HELP "
)

type Parser struct {
	NamePrefix            string
	DefaultTags           map[string]string
	Header                http.Header
	IgnoreMetricsFilter   filter.Filter
	IgnoreLabelKeysFilter filter.Filter
	DuplicationAllowed    bool
}

func NewParser(namePrefix string, defaultTags map[string]string, header http.Header, duplicationAllowed bool, ignoreMetricsFilter, ignoreLabelKeysFilter filter.Filter) *Parser {
	return &Parser{
		NamePrefix:            namePrefix,
		DefaultTags:           defaultTags,
		Header:                header,
		IgnoreMetricsFilter:   ignoreMetricsFilter,
		IgnoreLabelKeysFilter: ignoreLabelKeysFilter,
		DuplicationAllowed:    duplicationAllowed,
	}
}

func EmptyParser() *Parser {
	return &Parser{}
}

func (p *Parser) parse(buf []byte, slist *types.SampleList) error {
	metricFamilies, err := util.Parse(buf, p.Header)
	if err != nil {
		return err
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
				util.HandleSummary(p.NamePrefix, m, tags, metricName, nil, slist)
			} else if mf.GetType() == dto.MetricType_HISTOGRAM {
				util.HandleHistogram(p.NamePrefix, m, tags, metricName, nil, slist)
			} else {
				util.HandleGaugeCounter(p.NamePrefix, m, tags, metricName, nil, slist)
			}
		}
	}

	return nil
}

func (p *Parser) Parse(buf []byte, slist *types.SampleList) error {
	mediatype, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
	if mediatype == "application/vnd.google.protobuf" || !p.DuplicationAllowed {
		return p.parse(buf, slist)
	}

	var (
		metricHeaderBytes = []byte(MetricHeader)
		typeHeaderBytes   = []byte("# TYPE ")
	)

	metrics := bytes.Split(buf, metricHeaderBytes)
	for i := range metrics {
		if i != 0 {
			metrics[i] = append(append([]byte(nil), metricHeaderBytes...), metrics[i]...)
		}

		typeMetrics := bytes.Split(metrics[i], typeHeaderBytes)
		for j := range typeMetrics {
			if j != 0 {
				typeMetrics[j] = append(append([]byte(nil), typeHeaderBytes...), typeMetrics[j]...)
			}

			if len(bytes.TrimSpace(typeMetrics[j])) == 0 {
				continue
			}

			err := p.parse(typeMetrics[j], slist)
			if err != nil {
				log.Println("E! parse metrics failed, error:", err, "metrics:", string(typeMetrics[j]))
			}
		}
	}

	return nil
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
