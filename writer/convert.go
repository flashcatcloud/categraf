package writer

import (
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

var labelReplacer = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")

func convert(item *types.Sample) prompb.TimeSeries {
	value, err := conv.ToFloat64(item.Value)
	if err != nil {
		// If the Labels is empty, it means it is abnormal data
		return prompb.TimeSeries{}
	}

	pt := prompb.TimeSeries{}

	timestamp := item.Timestamp.UnixMilli()
	if config.Config.Global.Precision == "s" {
		timestamp = item.Timestamp.Unix()
	}

	pt.Samples = append(pt.Samples, prompb.Sample{
		Timestamp: timestamp,
		Value:     value,
	})

	// add label: metric
	pt.Labels = append(pt.Labels, prompb.Label{
		Name:  model.MetricNameLabel,
		Value: item.Metric,
	})

	// add other labels
	for k, v := range item.Labels {
		pt.Labels = append(pt.Labels, prompb.Label{
			Name:  labelReplacer.Replace(k),
			Value: v,
		})
	}

	return pt
}
