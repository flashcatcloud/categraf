//go:build !no_logs

package util

import (
	"testing"
	"time"
)

func TestExpandDatePattern(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-18T15:04:05.123Z")

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "YYYY.MM.dd",
			path:     "/var/log/app-%{+YYYY.MM.dd}.log",
			expected: "/var/log/app-2026.06.18.log",
		},
		{
			name:     "yyyy-MM-dd",
			path:     "/var/log/app-%{+yyyy-MM-dd}.log",
			expected: "/var/log/app-2026-06-18.log",
		},
		{
			name:     "ISO8601",
			path:     "/var/log/app-%{+ISO8601}.log",
			expected: "/var/log/app-2026-06-18T15:04:05.123Z.log",
		},
		{
			name:     "hh-mm-ss-SSS",
			path:     "/var/log/app-%{+hh-mm-ss-SSS}.log",
			expected: "/var/log/app-03-04-05-123.log",
		},
		{
			name:     "No pattern",
			path:     "/var/log/app.log",
			expected: "/var/log/app.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandDatePattern(tt.path, now); got != tt.expected {
				t.Errorf("ExpandDatePattern() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestContainsDatePattern(t *testing.T) {
	if !ContainsDatePattern("/var/log/app-%{+yyyy-MM-dd}.log") {
		t.Error("Expected true, got false")
	}
	if ContainsDatePattern("/var/log/app.log") {
		t.Error("Expected false, got true")
	}
}
