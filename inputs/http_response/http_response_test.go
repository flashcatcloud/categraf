package http_response

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"flashcat.cloud/categraf/types"
)

func TestInstance_Gather_HTTPTimingMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	ins := &Instance{
		Targets: []string{withLocalhostTarget(t, server.URL)},
	}

	if err := ins.Init(); err != nil {
		t.Fatalf("Instance Init failed: %v", err)
	}

	slist := types.NewSampleList()
	ins.Gather(slist)
	samples := samplesByMetric(slist.PopBackAll())

	assertMetricExists(t, samples, "http_response_dns_request")
	assertMetricExists(t, samples, "http_response_tcp_connect")
	assertMetricExists(t, samples, "http_response_tls_handshake")
	assertMetricExists(t, samples, "http_response_first_byte")
	assertMetricExists(t, samples, "http_response_total_cost")
	assertMetricExists(t, samples, "http_response_dns_time")
	assertMetricExists(t, samples, "http_response_connect_time")
	assertMetricExists(t, samples, "http_response_tls_time")
	assertMetricExists(t, samples, "http_response_first_response_time")
	assertMetricExists(t, samples, "http_response_end_response_time")
	assertMetricExists(t, samples, "http_response_response_time")
	assertMetricExists(t, samples, "http_response_response_time_ms")
	assertMetricExists(t, samples, "http_response_response_code")
	assertMetricExists(t, samples, "http_response_result_code")

	if got := int64Value(t, samples["http_response_dns_request"]); got < 0 {
		t.Fatalf("dns_request = %d, want >= 0", got)
	}
	if got := int64Value(t, samples["http_response_tcp_connect"]); got < 0 {
		t.Fatalf("tcp_connect = %d, want >= 0", got)
	}
	if got := int64Value(t, samples["http_response_tls_handshake"]); got != -1 {
		t.Fatalf("tls_handshake = %d, want -1 for http target", got)
	}
	if got := int64Value(t, samples["http_response_first_byte"]); got < 0 {
		t.Fatalf("first_byte = %d, want >= 0", got)
	}
	if got := int64Value(t, samples["http_response_total_cost"]); got < 0 {
		t.Fatalf("total_cost = %d, want >= 0", got)
	}
	if got := int64Value(t, samples["http_response_response_time_ms"]); got < 0 {
		t.Fatalf("response_time_ms = %d, want >= 0", got)
	}
	if got := intValue(t, samples["http_response_response_code"]); got != http.StatusOK {
		t.Fatalf("response_code = %d, want %d", got, http.StatusOK)
	}
	if got := uint64Value(t, samples["http_response_result_code"]); got != Success {
		t.Fatalf("result_code = %d, want %d", got, Success)
	}
	if method := samples["http_response_total_cost"].Labels["method"]; method != http.MethodGet {
		t.Fatalf("method label = %q, want %q", method, http.MethodGet)
	}
	if remoteAddr := samples["http_response_total_cost"].Labels["remote_addr"]; remoteAddr == "" {
		t.Fatalf("remote_addr label should be exposed")
	}
}

func TestInstance_Gather_HTTPSCertAndTimingMetrics(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	ins := &Instance{
		Targets: []string{withLocalhostTarget(t, server.URL)},
	}
	ins.UseTLS = true
	ins.InsecureSkipVerify = true

	if err := ins.Init(); err != nil {
		t.Fatalf("Instance Init failed: %v", err)
	}

	slist := types.NewSampleList()
	ins.Gather(slist)
	samples := samplesByMetric(slist.PopBackAll())

	assertMetricExists(t, samples, "http_response_cert_expire_timestamp")
	assertMetricExists(t, samples, "http_response_tls_handshake")
	assertMetricExists(t, samples, "http_response_total_cost")

	if got := int64Value(t, samples["http_response_tls_handshake"]); got < 0 {
		t.Fatalf("tls_handshake = %d, want >= 0", got)
	}
	if got := int64Value(t, samples["http_response_total_cost"]); got < 0 {
		t.Fatalf("total_cost = %d, want >= 0", got)
	}
	if got := int64Value(t, samples["http_response_cert_expire_timestamp"]); got <= time.Now().Unix() {
		t.Fatalf("cert_expire_timestamp = %d, want a future timestamp", got)
	}
	if method := samples["http_response_cert_expire_timestamp"].Labels["method"]; method != http.MethodGet {
		t.Fatalf("method label = %q, want %q", method, http.MethodGet)
	}
	if _, ok := samples["http_response_total_cost"].Labels["cert_name"]; ok {
		t.Fatalf("cert_name label should only be exposed on http_response_cert_expire_timestamp")
	}
	if got := uint64Value(t, samples["http_response_result_code"]); got != Success {
		t.Fatalf("result_code = %d, want %d", got, Success)
	}
}

func withLocalhostTarget(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url %q: %v", rawURL, err)
	}

	host := "localhost"
	if port := parsed.Port(); port != "" {
		host += ":" + port
	}
	parsed.Host = host
	return parsed.String()
}

func samplesByMetric(samples []*types.Sample) map[string]*types.Sample {
	ret := make(map[string]*types.Sample, len(samples))
	for _, sample := range samples {
		ret[sample.Metric] = sample
	}
	return ret
}

func assertMetricExists(t *testing.T, samples map[string]*types.Sample, metric string) {
	t.Helper()
	if _, ok := samples[metric]; !ok {
		t.Fatalf("metric %s not found", metric)
	}
}

func int64Value(t *testing.T, sample *types.Sample) int64 {
	t.Helper()

	switch value := sample.Value.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	default:
		t.Fatalf("metric %s has unexpected value type %T", sample.Metric, sample.Value)
		return 0
	}
}

func intValue(t *testing.T, sample *types.Sample) int {
	t.Helper()

	switch value := sample.Value.(type) {
	case int:
		return value
	case int64:
		return int(value)
	default:
		t.Fatalf("metric %s has unexpected value type %T", sample.Metric, sample.Value)
		return 0
	}
}

func uint64Value(t *testing.T, sample *types.Sample) uint64 {
	t.Helper()

	value, ok := sample.Value.(uint64)
	if !ok {
		t.Fatalf("metric %s has unexpected value type %T", sample.Metric, sample.Value)
	}
	return value
}
