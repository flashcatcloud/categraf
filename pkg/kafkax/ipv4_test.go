package kafkax

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	"github.com/IBM/sarama"
)

func TestPrepareIPv4ConfigDefaultAndProxy(t *testing.T) {
	cfg := sarama.NewConfig()
	cfg.Net.ResolveCanonicalBootstrapServers = true
	addresses := []string{"unresolved.invalid:9092"}
	t.Setenv("PREFER_IPV4_STACK", "false")
	got, brokers, err := PrepareIPv4Config(cfg, addresses)
	if err != nil || got != cfg || brokers[0] != addresses[0] || !got.Net.ResolveCanonicalBootstrapServers {
		t.Fatalf("default changed: %v %v", brokers, err)
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	cfg.Net.Proxy.Enable = true
	cfg.Net.Proxy.Dialer = &net.Dialer{Timeout: time.Second}
	_, _, err = PrepareIPv4Config(cfg, addresses)
	if err == nil || !strings.Contains(err.Error(), "custom proxy") {
		t.Fatalf("expected actionable proxy error, got %v", err)
	}
	if !cfg.Net.ResolveCanonicalBootstrapServers || !cfg.Net.Proxy.Enable {
		t.Fatal("modified proxy config")
	}
}

func TestCanonicalIPv4BrokersFailures(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "ipv6-only.invalid.", nil, net.IPv6loopback)
	if _, err := canonicalIPv4Brokers(context.Background(), []string{"ipv6-only.invalid.:9092"}); err == nil {
		t.Fatal("accepted IPv6-only bootstrap")
	}
	a, aaaa := counts()
	if a == 0 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
	if _, err := canonicalIPv4Brokers(context.Background(), []string{"invalid-portless-host"}); err == nil {
		t.Fatal("accepted malformed bootstrap")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := canonicalIPv4Brokers(ctx, []string{"ipv6-only.invalid.:9092"}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestPrepareIPv4CanonicalConfigPreserved(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveCanonicalDNS(t, "bootstrap.invalid.", "canonical.invalid.", net.IPv4(192, 0, 2, 1), net.IPv6loopback, net.IPv4(127, 0, 0, 1))
	cfg := sarama.NewConfig()
	cfg.Net.ResolveCanonicalBootstrapServers = true
	cfg.Net.TLS.Enable = true
	cfg.Net.TLS.Config = &tls.Config{ServerName: "configured-name"}
	cfg.Net.SASL.Enable, cfg.Net.SASL.User = true, "configured-user"
	cfg.Net.DialTimeout, cfg.Net.KeepAlive = time.Second, 15*time.Second
	cfg.Net.LocalAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
	prepared, brokers, err := PrepareIPv4Config(cfg, []string{"bootstrap.invalid.:9092", "bootstrap.invalid.:9092", "bootstrap.invalid.:9093"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(brokers, ",") != "canonical.invalid:9092,canonical.invalid:9093" {
		t.Fatalf("canonical brokers=%v", brokers)
	}
	if prepared == cfg || prepared.Net.ResolveCanonicalBootstrapServers || !cfg.Net.ResolveCanonicalBootstrapServers || cfg.Net.Proxy.Enable {
		t.Fatal("changed original or failed to disable duplicate lookup")
	}
	if !prepared.Net.TLS.Enable || prepared.Net.TLS.Config != cfg.Net.TLS.Config || !prepared.Net.SASL.Enable || prepared.Net.SASL.User != cfg.Net.SASL.User {
		t.Fatal("TLS/SASL settings changed")
	}
	if prepared.Net.DialTimeout != cfg.Net.DialTimeout || prepared.Net.KeepAlive != cfg.Net.KeepAlive || prepared.Net.LocalAddr != cfg.Net.LocalAddr {
		t.Fatal("TCP settings changed")
	}
	_, aaaa, ptr := counts()
	if aaaa != 0 || ptr != 3 {
		t.Fatalf("AAAA=%d PTR=%d", aaaa, ptr)
	}
}

func TestCanonicalIPv4BrokersMissingPTR(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "missing-ptr.invalid.", net.IPv4(192, 0, 2, 1), net.IPv6loopback)
	_, err := canonicalIPv4Brokers(context.Background(), []string{"missing-ptr.invalid.:9092"})
	if err == nil || !strings.Contains(err.Error(), "canonical name") {
		t.Fatalf("missing PTR should fail: %v", err)
	}
	a, aaaa := counts()
	if a != 1 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
}

func TestPrepareNilConfigPreservesDefault(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "false")
	cfg, _, err := PrepareIPv4Config(nil, []string{"localhost:9092"})
	if cfg != nil || err != nil {
		t.Fatalf("cfg=%v err=%v", cfg, err)
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	cfg, _, err = PrepareIPv4Config(nil, []string{"localhost:9092"})
	if err != nil || cfg == nil || !cfg.Net.Proxy.Enable {
		t.Fatalf("cfg=%v err=%v", cfg, err)
	}
}
