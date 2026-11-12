//go:build !no_prometheus

package prometheus

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
)

// An explicit region bypasses the region-discovery config loader. Credential
// hooks must reach the independent loader that constructs the EC2 SDK client.
func TestEC2ContainerCredentialsPolicy(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "container-auth")
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			emptyConfig := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(emptyConfig, nil, 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AWS_CONFIG_FILE", emptyConfig)
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", emptyConfig)
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var tokens, apiCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/credentials" {
					tokens.Add(1)
					if r.Header.Get("Authorization") != "container-auth" {
						t.Error("lost container authorization")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"AccessKeyId":"container-key","SecretAccessKey":"secret","Token":"session","Expiration":%q}`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
					return
				}
				apiCalls.Add(1)
				if !strings.Contains(r.Header.Get("Authorization"), "Credential=container-key/") {
					t.Error("lost credential signing")
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				action := r.Form.Get("Action")
				w.Header().Set("Content-Type", "text/xml")
				switch action {
				case "DescribeAvailabilityZones":
					fmt.Fprint(w, `<DescribeAvailabilityZonesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><availabilityZoneInfo/></DescribeAvailabilityZonesResponse>`)
				case "DescribeInstances":
					fmt.Fprint(w, `<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><reservationSet/></DescribeInstancesResponse>`)
				default:
					t.Errorf("unexpected action %q", action)
					http.Error(w, "unexpected action", 500)
				}
			}))
			defer server.Close()
			endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
			t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", endpoint+"/credentials")
			policy, closePolicy := newNetworkPolicy()
			defer closePolicy()
			d := discoveryFromYAML(t, fmt.Sprintf("scrape_configs:\n- job_name: ec2\n  ec2_sd_configs:\n  - region: us-east-1\n    endpoint: %q\n    refresh_interval: 100ms\n", endpoint), policy)
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			for i := 0; i < 2; i++ {
				nextDiscoveryTargets(t, d)
				server.CloseClientConnections()
			}
			a, aaaa := counts()
			if tokens.Load() < 2 || apiCalls.Load() < 3 || a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("tokens=%d api=%d A=%d AAAA=%d", tokens.Load(), apiCalls.Load(), a, aaaa)
			}
		})
	}
}
