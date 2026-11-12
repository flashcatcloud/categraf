package haproxy

import (
	"net/http"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	for _, proxy := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "proxy-from-env"}[proxy], func(t *testing.T) {
			nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client { return newHTTPClient(true, proxy, 2*time.Second) })
		})
	}
}
