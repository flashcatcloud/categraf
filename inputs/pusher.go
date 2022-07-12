package inputs

import (
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

func PushSamples(slist *list.SafeList, fields map[string]interface{}, labels ...map[string]string) {
	for metric, value := range fields {
		slist.PushFront(types.NewSample(metric, value, labels...))
	}
}

func PushMeasurements(slist *list.SafeList, measurement string, fields map[string]interface{}, labels ...map[string]string) {
	for metric, value := range fields {
		slist.PushFront(types.NewSample(measurement+"_"+metric, value, labels...))
	}
}
