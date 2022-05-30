package prom

import (
	"regexp"
	"strings"
)

func ValidName(s string) string {
	nameRe := regexp.MustCompile("([^a-zA-Z0-9_])")
	s = nameRe.ReplaceAllString(s, "_")
	s = strings.ToLower(s)
	return s
}

func BuildMetric(names ...string) string {
	var b strings.Builder
	for i := 0; i < len(names); i++ {
		if names[i] != "" {
			if b.Len() > 0 {
				b.WriteString("_")
			}
			b.WriteString(names[i])
		}
	}

	return b.String()
}
