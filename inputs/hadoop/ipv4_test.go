package hadoop

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		component := new(Component)
		if err := component.Initialize(CommonConfig{}); err != nil {
			t.Fatal(err)
		}
		return component.httpClient
	})
}
