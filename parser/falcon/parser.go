package falcon

import (
	"encoding/json"
	"strings"

	"flashcat.cloud/categraf/types"
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

func (p *Parser) Parse(input []byte, slist *types.SampleList) error {
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

	for i := 0; i < len(samples); i++ {
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

		slist.PushSample("", samples[i].Metric, samples[i].Value, labels)
	}

	return nil
}
