package clickhouse

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := &Instance{Servers: []string{endpoint}}
		if err := ins.Init(); err != nil {
			t.Fatal(err)
		}
		return ins.HTTPClient
	})
}
