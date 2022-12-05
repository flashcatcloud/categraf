package house

import (
	"encoding/json"
	"flashcat.cloud/categraf/types"
	"strings"
)

var labelReplacer = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")

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
