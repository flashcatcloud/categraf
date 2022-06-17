package house

import (
	"encoding/json"
	"strings"
	"time"

	"flashcat.cloud/categraf/types"
)

var labelReplacer = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")

type Sample struct {
	Timestamp time.Time `json:"timestamp"`
	Metric    string    `json:"metric"`
	Tags      string    `json:"tags"`
	Value     float64   `json:"value"`
}

func convertTags(s *types.Sample) string {
	tags := make(map[string]string)
	for k, v := range s.Labels {
		tags[labelReplacer.Replace(k)] = v
	}

	bs, err := json.Marshal(tags)
	if err != nil {
		return ""
	}

	return string(bs)
}
