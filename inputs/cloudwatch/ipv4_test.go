package cloudwatch

import (
	"net/http"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := &Instance{Namespaces: []string{"test"}}
		ins.Region = "us-east-1"
		ins.AccessKey = "test"
		ins.SecretKey = "test"
		if err := ins.Init(); err != nil {
			t.Fatal(err)
		}
		proxy, err := ins.HTTPProxy.Proxy()
		if err != nil {
			t.Fatal(err)
		}
		return ins.newHTTPClient(proxy)
	})
}
