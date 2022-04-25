package falcon

import (
	"encoding/json"
	"strings"
	"time"

	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

// payload = [
//     {
//         "endpoint": "test-endpoint",
//         "metric": "test-metric",
//         "value": 1,
//         "tags": "idc=lg,loc=beijing",
//     },

//     {
//         "endpoint": "test-endpoint",
//         "metric": "test-metric2",
//         "value": 2,
//         "tags": "idc=lg,loc=beijing",
//     },
// ]

type Sample struct {
	Endpoint  string      `json:"endpoint"`
	Metric    string      `json:"metric"`
	Timestamp int64       `json:"timestamp"`
	Value     interface{} `json:"value"`
	Tags      string      `json:"tags"`
}

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(input []byte, slist *list.SafeList) error {
	var samples []Sample

	if input[0] == '[' {
		err := json.Unmarshal(input, &samples)
		if err != nil {
			return err
		}
	} else {
		var s Sample
		err := json.Unmarshal(input, &s)
		if err != nil {
			return err
		}
		samples = append(samples, s)
	}

	now := time.Now()

	for i := 0; i < len(samples); i++ {
		fv, err := conv.ToFloat64(samples[i])
		if err != nil {
			continue
		}

		labels := make(map[string]string)
		tagarr := strings.Split(samples[i].Tags, ",")
		for j := 0; j < len(tagarr); j++ {
			pair := strings.TrimSpace(tagarr[j])
			if pair == "" {
				continue
			}

			kv := strings.Split(pair, "=")
			if len(kv) != 2 {
				continue
			}

			labels[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}

		endpoint := strings.TrimSpace(samples[i].Endpoint)
		if endpoint != "" {
			labels["endpoint"] = endpoint
		}

		item := &types.Sample{
			Metric:    samples[i].Metric,
			Value:     fv,
			Labels:    labels,
			Timestamp: now,
		}

		slist.PushFront(item)
	}

	return nil
}
