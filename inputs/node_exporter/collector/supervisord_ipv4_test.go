//go:build !nosupervisord

package collector

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestSupervisordHTTPIPv4(t *testing.T) {
	previousURL, previousClient := *supervisordURL, xrpc
	t.Cleanup(func() { *supervisordURL, xrpc = previousURL, previousClient })
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		*supervisordURL = endpoint
		if _, err := NewSupervisordCollector(); err != nil {
			t.Fatal(err)
		}
		return xrpc.HttpClient
	})
}
