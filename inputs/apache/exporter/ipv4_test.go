package exporter

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
	"github.com/go-kit/log"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		exporter, _ := New(log.NewNopLogger(), &Config{ScrapeURI: endpoint})
		return exporter.client
	})
}
