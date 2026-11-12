//go:build !no_prometheus

package prometheus

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	clientmetrics "github.com/prometheus/client_golang/prometheus"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	httpsd "github.com/prometheus/prometheus/discovery/http"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/storage/remote"
)

func TestRemoteWriteRoundRobinAndOAuthPolicy(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	for _, roundRobin := range []bool{false, true} {
		t.Run(fmt.Sprint(roundRobin), func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var tokens, writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					tokens.Add(1)
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"access_token":"fixture","token_type":"Bearer","expires_in":1}`)
					return
				}
				if r.Header.Get("Authorization") != "Bearer fixture" {
					t.Error("lost remote write authentication")
				}
				writes.Add(1)
				w.WriteHeader(204)
			}))
			defer server.Close()
			endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
			policy, closePolicy := newNetworkPolicy()
			defer closePolicy()
			t.Setenv("PREFER_IPV4_STACK", "false")
			httpConfig := commonconfig.DefaultHTTPClientConfig
			httpConfig.OAuth2 = &commonconfig.OAuth2{ClientID: "fixture", ClientSecret: "secret", TokenURL: endpoint + "/token"}
			parsed, _ := url.Parse(endpoint + "/write")
			client, err := remote.NewWriteClient("fixture", &remote.ClientConfig{URL: &commonconfig.URL{URL: parsed}, Timeout: model.Duration(time.Second), HTTPClientConfig: httpConfig, RoundRobinDNS: roundRobin, NetworkPolicy: policy})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if _, err := client.Store(context.Background(), nil, i); err != nil {
					t.Fatal(err)
				}
			}
			a, aaaa := counts()
			if writes.Load() != 2 || tokens.Load() != 2 || a == 0 || aaaa != 0 {
				t.Fatalf("writes=%d refreshes=%d A=%d AAAA=%d", writes.Load(), tokens.Load(), a, aaaa)
			}
		})
	}
}
func TestHTTPDiscoveryPolicy(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"targets":["discovered.invalid:9090"]}]`)
	}))
	defer server.Close()
	policy, closePolicy := newNetworkPolicy()
	defer closePolicy()
	cfg := httpsd.DefaultSDConfig
	cfg.URL = strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
	registry := clientmetrics.NewRegistry()
	metrics := cfg.NewDiscovererMetrics(registry, discovery.NewRefreshMetrics(registry))
	d, err := httpsd.NewDiscovery(&cfg, discovery.DiscovererOptions{Metrics: metrics, HTTPClientOptions: policy.HTTPOptions(), NetworkPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PREFER_IPV4_STACK", "false")
	groups, err := d.Refresh(context.Background())
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups=%v err=%v", groups, err)
	}
	a, aaaa := counts()
	if a == 0 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
}
func TestNotifierPolicy(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case received <- struct{}{}:
		default:
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	policy, closePolicy := newNetworkPolicy()
	defer closePolicy()
	manager := notifier.NewManager(&notifier.Options{NetworkPolicy: policy, HTTPClientOptions: policy.HTTPOptions(), Registerer: clientmetrics.NewRegistry(), QueueCapacity: 10}, model.UTF8Validation, nil)
	cfg := promconfig.DefaultConfig
	am := promconfig.DefaultAlertmanagerConfig
	am.Scheme = "http"
	cfg.AlertingConfig.AlertmanagerConfigs = promconfig.AlertmanagerConfigs{&am}
	if err := manager.ApplyConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	updates := make(chan map[string][]*targetgroup.Group, 1)
	done := make(chan struct{})
	go func() { defer close(done); manager.Run(updates) }()
	defer func() { manager.Stop(); <-done }()
	target := strings.Replace(strings.TrimPrefix(server.URL, "http://"), "127.0.0.1", "example.com.", 1)
	updates <- map[string][]*targetgroup.Group{"config-0": {{Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(target)}}}}}
	deadline := time.Now().Add(2 * time.Second)
	for len(manager.Alertmanagers()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("target discovery timed out")
		}
		time.Sleep(time.Millisecond)
	}
	t.Setenv("PREFER_IPV4_STACK", "false")
	manager.Send(&notifier.Alert{Labels: labels.FromStrings("alertname", "fixture"), StartsAt: time.Now()})
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("notification not received")
	}
	a, aaaa := counts()
	if a == 0 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
}
