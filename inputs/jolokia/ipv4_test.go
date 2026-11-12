package jolokia

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		client, err := NewClient(endpoint, &ClientConfig{})
		if err != nil {
			t.Fatal(err)
		}
		return client.client
	})
}
