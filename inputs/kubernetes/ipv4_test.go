package kubernetes

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := new(Instance)
		if err := ins.initHTTPTransport(); err != nil {
			t.Fatal(err)
		}
		return &http.Client{Transport: ins.RoundTripper}
	})
}
