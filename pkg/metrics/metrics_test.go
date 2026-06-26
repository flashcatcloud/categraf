package metrics

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseReaderTextFormatUsesValidationScheme(t *testing.T) {
	input := strings.NewReader("# HELP sample_metric sample metric\n# TYPE sample_metric gauge\nsample_metric 1\n")

	families, err := ParseReader(input, http.Header{})
	if err != nil {
		t.Fatalf("ParseReader returned error: %v", err)
	}

	if _, ok := families["sample_metric"]; !ok {
		t.Fatalf("expected sample_metric family, got %v", families)
	}
}

func TestParseReaderTextFormatRejectsInvalidUTF8MetricName(t *testing.T) {
	input := strings.NewReader("sample_metric\xff 1\n")

	if _, err := ParseReader(input, http.Header{}); err == nil {
		t.Fatal("expected invalid UTF-8 metric name error")
	}
}
