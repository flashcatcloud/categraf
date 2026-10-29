package writer

import (
	"testing"

	"github.com/prometheus/client_golang/api"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
)

func newTestWriter(t *testing.T, endpoint, caFile string) func() error {
	t.Helper()
	opt := config.WriterOption{
		Url: endpoint, DialTimeout: 2000, Timeout: 2000, MaxIdleConnsPerHost: 10,
	}
	opt.TLSCA = caFile
	w, err := newWriter(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Client.(api.CloseIdler).CloseIdleConnections)
	return func() error { return w.post([]byte("test payload")) }
}

func TestWriterPreferIPv4Stack(t *testing.T) {
	nettest.CheckHTTPClient(t, newTestWriter)
}

func TestWriterPreferIPv4StackProxy(t *testing.T) {
	nettest.CheckHTTPProxy(t, newTestWriter)
}
