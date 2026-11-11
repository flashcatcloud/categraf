package writer

import (
	"slices"
	"strings"

	"github.com/prometheus/prometheus/prompb"
)

func compareLabelNames(a, b prompb.Label) int {
	return strings.Compare(a.Name, b.Name)
}

func normalizeTimeSeriesLabels(series []prompb.TimeSeries) {
	for i := range series {
		// Series converted from samples are already sorted.
		if !slices.IsSortedFunc(series[i].Labels, compareLabelNames) {
			slices.SortFunc(series[i].Labels, compareLabelNames)
		}
	}
}
