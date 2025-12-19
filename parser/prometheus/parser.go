package prometheus

import (
	"bytes"
	"io"
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

func (p *Parser) parse(r io.Reader, slist *types.SampleList) error {
	metricFamilies, err := util.ParseReader(r, p.Header)
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
		return p.parse(bytes.NewReader(buf), slist)
	}

	var (
		helpHeader = []byte("# HELP ")
		typeHeader = []byte("# TYPE ")
		infoBytes  = []byte(" info\n")
		gaugeBytes = []byte(" gauge\n")
	)

	offset := 0
	totalLen := len(buf)

	for offset < totalLen {
		// Find next delimiter position relative to current offset
		relIdxHelp := bytes.Index(buf[offset:], helpHeader)
		relIdxType := bytes.Index(buf[offset:], typeHeader)

		var relIdx int

		if relIdxHelp == -1 && relIdxType == -1 {
			// No more delimiters, take the rest
			relIdx = totalLen - offset
		} else if relIdxHelp != -1 && (relIdxType == -1 || relIdxHelp < relIdxType) {
			relIdx = relIdxHelp
		} else {
			relIdx = relIdxType
		}

		// Calculate absolute end of current chunk
		chunkEnd := offset + relIdx

		if chunkEnd > offset {
			chunk := buf[offset:chunkEnd]
			// Trim only leading/trailing whitespace to check for empty content
			if len(bytes.TrimSpace(chunk)) > 0 {
				// Handle "info" type check and replacement for each chunk
				// "info->gauge" replacement feature
				// We do Contains check first to avoid allocation if replacement isn't needed (Zero-Copy path)
				if bytes.Contains(chunk, infoBytes) {
					chunk = bytes.ReplaceAll(chunk, infoBytes, gaugeBytes)
				}

				var reader io.Reader
				// Check if chunk already has a valid header
				if bytes.HasPrefix(chunk, helpHeader) || bytes.HasPrefix(chunk, typeHeader) {
					reader = bytes.NewReader(chunk)
				} else {
					// Fallback: prepend TYPE header
					reader = io.MultiReader(bytes.NewReader(typeHeader), bytes.NewReader(chunk))
				}

				if err := p.parse(reader, slist); err != nil {
					log.Println("E! parse metrics failed, error:", err)
				}
			}
		}

		offset = chunkEnd
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