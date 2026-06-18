//go:build !no_logs

package util

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// 匹配 Logstash 风格的日期占位符 %{+yyyy-MM-dd}
	datePatternRegex = regexp.MustCompile(`%\{\+([^}]+)\}`)

	// Logstash (Joda-Time) 到 Go 时间格式的映射
	// 按长度排序，确保长的模式先被替换
	dateFormatMappings = []struct {
		joda string
		go_  string
	}{
		// 年份
		{"yyyy", "2006"},
		{"yy", "06"},

		// 月份
		{"MM", "01"},
		{"M", "1"},

		// 日期
		{"dd", "02"},
		{"d", "2"},

		// 小时
		{"HH", "15"},
		{"H", "15"},

		// 分钟
		// 12小时制
		{"hh", "03"},

		// 分钟
		{"mm", "04"},
		{"m", "4"},

		// 秒
		{"ss", "05"},
		{"s", "5"},

		// 毫秒

		// ISO8601
		{"ISO8601", "2006-01-02T15:04:05.000Z"},

		// 容错：兼容部分用户习惯的大写年份
		{"YYYY", "2006"},
		{"YY", "06"},

		// 常用的分隔符保持不变
		{"-", "-"},
		{".", "."},
		{"/", "/"},
		{"_", "_"},
		{" ", " "},
		{":", ":"},
	}
)

// ContainsDatePattern returns true if the path contains a logstash date pattern
func ContainsDatePattern(path string) bool {
	return datePatternRegex.MatchString(path)
}

// ExpandDatePattern replaces the logstash date pattern in the path with the actual date
func ExpandDatePattern(path string, now time.Time) string {
	return datePatternRegex.ReplaceAllStringFunc(path, func(match string) string {
		// 提取占位符内容（去掉 %{+ 和 }）
		pattern := match[3 : len(match)-1]

		currentNow := now
		if strings.Contains(pattern, "ISO8601") {
			currentNow = currentNow.UTC()
		}

		// 毫秒级处理：Go 时间格式化会把 1, 2, 3 等数字当成占位符解析
		// 所以我们先将其替换为安全的无冲突占位符，等 Format 完成后再替换为真实毫秒
		pattern = strings.ReplaceAll(pattern, "SSS", "_XXX_")

		// 转换为 Go 时间格式
		goFormat := convertJodaToGoFormat(pattern)

		// 格式化时间
		formatted := currentNow.Format(goFormat)

		millis := fmt.Sprintf("%03d", currentNow.Nanosecond()/1e6)
		return strings.ReplaceAll(formatted, "_XXX_", millis)
	})
}

// convertJodaToGoFormat 将 Joda-Time 格式转换为 Go 时间格式
func convertJodaToGoFormat(jodaFormat string) string {
	result := jodaFormat

	// 按照映射表顺序替换（长的先替换，避免冲突）
	for _, mapping := range dateFormatMappings {
		result = strings.ReplaceAll(result, mapping.joda, mapping.go_)
	}

	return result
}

// parseDatePattern 解析并验证日期模式（用于调试和错误报告）
func parseDatePattern(path string) []string {
	matches := datePatternRegex.FindAllString(path, -1)
	return matches
}
