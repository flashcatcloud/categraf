package aws

import (
	"context"
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
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestCredentialRefreshRetainsIPv4AfterSDKClones(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
					t.Error("lost SigV4 signing")
				}
				if err := r.ParseForm(); err != nil || r.Form.Get("Action") != "AssumeRole" {
					t.Errorf("wrong STS action: %v", err)
				}
				w.Header().Set("Content-Type", "text/xml")
				fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>refreshed-key</AccessKeyId><SecretAccessKey>test-secret</SecretAccessKey><SessionToken>test-token</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleResult><ResponseMetadata><RequestId>test</RequestId></ResponseMetadata></AssumeRoleResponse>`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
			}))
			defer server.Close()
			opts := HTTPClientOptionsFromEnv()
			opts = append(opts, awsconfig.WithRegion("us-east-1"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test-key", "test-secret", "")))
			cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
			if err != nil {
				t.Fatal(err)
			}
			if value == "true" {
				b, ok := cfg.HTTPClient.(*awshttp.BuildableClient)
				if !ok {
					t.Fatal("lost SDK BuildableClient")
				}
				cfg.HTTPClient = b.WithDialerOptions(func(d *net.Dialer) { d.Timeout = 250 * time.Millisecond; d.KeepAlive = 30 * time.Second }).WithTimeout(2 * time.Second)
				b = cfg.HTTPClient.(*awshttp.BuildableClient)
				if b.GetTimeout() != 2*time.Second || b.GetDialer().Timeout != 250*time.Millisecond {
					t.Fatal("native metadata timeout settings lost")
				}
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
			service := sts.NewFromConfig(cfg, func(o *sts.Options) { o.BaseEndpoint = awsv2.String(endpoint) })
			cache := awsv2.NewCredentialsCache(stscreds.NewAssumeRoleProvider(service, "arn:aws:iam::123456789012:role/test"))
			for i := 0; i < 2; i++ {
				token, err := cache.Retrieve(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if token.AccessKeyID != "refreshed-key" {
					t.Fatal("wrong credentials")
				}
			}
			a, aaaa := counts()
			if requests.Load() != 2 || a == 0 || value == "true" && aaaa != 0 {
				t.Fatalf("requests=%d A=%d AAAA=%d", requests.Load(), a, aaaa)
			}
		})
	}
}

// Container credentials resolve the endpoint before constructing an independent
// HTTP provider. Exercise both stages and the provider's token-file refresh.
func TestContainerCredentialRefreshPolicy(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, value := range []string{"false", "true"} {
		for _, address := range []string{"loopback", "public", "ipv6-only"} {
			t.Run(value+"/"+address, func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", value)
				ipv4 := net.IPv4(127, 0, 0, 1)
				if address == "public" {
					ipv4 = net.IPv4(192, 0, 2, 1)
				}
				if address == "ipv6-only" {
					ipv4 = nil
				}
				counts := nettest.ObserveDNS(t, "example.com.", ipv4, net.IPv6loopback)
				file := filepath.Join(t.TempDir(), "token")
				t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", file)
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := requests.Add(1)
					if got := r.Header.Get("Authorization"); got != fmt.Sprintf("token-%d", n) {
						t.Errorf("authorization=%q request=%d", got, n)
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"AccessKeyId":"container-key","SecretAccessKey":"secret","Token":"session","Expiration":%q}`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
				}))
				defer server.Close()
				t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", strings.Replace(server.URL, "127.0.0.1", "example.com.", 1))
				opts := HTTPClientOptionsFromEnv()
				opts = append(opts, awsconfig.WithRegion("us-east-1"), awsconfig.WithSharedConfigFiles([]string{}), awsconfig.WithSharedCredentialsFiles([]string{}))
				if value == "true" {
					t.Setenv("PREFER_IPV4_STACK", "false")
				}
				cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
				reject := address == "public" || address == "ipv6-only" && value == "true"
				if reject {
					if err == nil {
						t.Fatal("accepted disallowed/unresolvable container endpoint")
					}
				} else if address == "loopback" {
					if err != nil {
						t.Fatal(err)
					}
					for i := 1; i <= 2; i++ {
						if err := os.WriteFile(file, []byte(fmt.Sprintf("token-%d", i)), 0600); err != nil {
							t.Fatal(err)
						}
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						cred, err := cfg.Credentials.Retrieve(ctx)
						cancel()
						if err != nil {
							t.Fatal(err)
						}
						if cred.AccessKeyID != "container-key" {
							t.Fatal("wrong container credentials")
						}
						server.CloseClientConnections()
					}
					if requests.Load() != 2 {
						t.Fatalf("refreshes=%d", requests.Load())
					}
				} else if err != nil {
					t.Fatal(err)
				}
				a, aaaa := counts()
				if a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
					t.Fatalf("A=%d AAAA=%d", a, aaaa)
				}
			})
		}
	}
}
