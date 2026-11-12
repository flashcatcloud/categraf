package redfish

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := &Instance{Addresses: []Address{{URL: endpoint}}, Sets: []Set{{URN: "/redfish/v1"}}}
		if err := ins.Init(); err != nil {
			t.Fatal(err)
		}
		return ins.Addresses[0].client
	})
}
