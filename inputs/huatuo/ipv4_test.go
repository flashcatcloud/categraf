package huatuo

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := &Instance{URL: endpoint}
		if err := ins.Init(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ins.Drop)
		return ins.client
	})
}
