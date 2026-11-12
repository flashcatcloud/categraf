package elasticsearch

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := new(Instance)
		client, err := ins.createHTTPClient()
		if err != nil {
			t.Fatal(err)
		}
		return client
	})
}

func TestElasticsearchAuthTransportIPv4(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "AWS_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, secure := range []bool{false, true} {
		for _, auth := range []string{"api-key", "aws"} {
			t.Run(fmt.Sprintf("tls=%t/auth=%s", secure, auth), func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", "true")
				counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					value := r.Header.Get("Authorization")
					if auth == "api-key" && value != "ApiKey test-key" || auth == "aws" && !strings.HasPrefix(value, "AWS4-HMAC-SHA256 ") {
						t.Errorf("Authorization=%q", value)
					}
					w.WriteHeader(204)
				}))
				ins := &Instance{}
				if secure {
					server.TLS, ins.TLSCA = nettest.TLSFixture(t)
					ins.UseTLS = true
					server.StartTLS()
				} else {
					server.Start()
				}
				defer server.Close()
				if auth == "api-key" {
					ins.ApiKey = "test-key"
				} else {
					ins.AwsRegion = "us-east-1"
				}
				client, err := ins.createHTTPClient()
				if err != nil {
					t.Fatal(err)
				}
				defer client.CloseIdleConnections()
				endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
				response, err := client.Post(endpoint, "application/json", strings.NewReader(`{}`))
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				a, aaaa := counts()
				if a == 0 || aaaa != 0 {
					t.Fatalf("A=%d AAAA=%d", a, aaaa)
				}
			})
		}
	}
}
