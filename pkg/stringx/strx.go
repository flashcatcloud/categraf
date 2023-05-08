package stringx

import (
	"unicode"
)

func SnakeCase(in string) string {
	runes := []rune(in)
	length := len(runes)

	var out []rune
	for i := 0; i < length; i++ {
		if runes[i] == '.' {
			continue
		}
		if i > 0 && unicode.IsUpper(runes[i]) && ((i+1 < length && unicode.IsLower(runes[i+1])) || unicode.IsLower(runes[i-1])) {
			if runes[i-1] != '_' {
				out = append(out, '_')
			}
		}
		out = append(out, unicode.ToLower(runes[i]))
	}

	return string(out)
}
