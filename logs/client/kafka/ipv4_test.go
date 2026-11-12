//go:build !no_logs

package kafka

import (
	"net"
	"testing"
	"time"

	coreconfig "flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/internal/nettest"
	"flashcat.cloud/categraf/logs/client"
	"github.com/IBM/sarama"
)

func TestDestinationIPv4CanonicalDNS(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveCanonicalDNS(t, "bootstrap.invalid.", "canonical.invalid.", net.IPv4(192, 0, 2, 1), net.IPv6loopback, net.IPv4(127, 0, 0, 1))
	broker := sarama.NewMockBroker(t, 0)
	defer broker.Close()
	_, port, _ := net.SplitHostPort(broker.Addr())
	canonical := net.JoinHostPort("canonical.invalid", port)
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"ApiVersionsRequest": sarama.NewMockApiVersionsResponse(t),
		"MetadataRequest":    sarama.NewMockMetadataResponse(t).SetBroker(canonical, 0).SetLeader("logs", 0, 0),
		"ProduceRequest":     sarama.NewMockProduceResponse(t).SetError("logs", 0, sarama.ErrNoError),
	})
	original := coreconfig.Config
	coreconfig.Config = &coreconfig.ConfigType{}
	t.Cleanup(func() { coreconfig.Config = original })
	cfg := sarama.NewConfig()
	cfg.Net.ResolveCanonicalBootstrapServers = true
	cfg.Net.DialTimeout, cfg.Net.ReadTimeout, cfg.Net.WriteTimeout = 2*time.Second, 2*time.Second, 2*time.Second
	cfg.Metadata.Retry.Max = 0
	cfg.Producer.Return.Successes = true
	coreconfig.Config.Logs.Config = cfg
	destinations := client.NewDestinationsContext()
	destinations.Start()
	defer destinations.Stop()
	endpoint := logsconfig.Endpoint{Addr: net.JoinHostPort("bootstrap.invalid.", port), Topic: "logs", APIKey: "test-key"}
	// A rebuilt client must repeat canonicalization without changing shared cfg.
	for i := 0; i < 2; i++ {
		d, err := newDestination(endpoint, JSONContentType, destinations, time.Second, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.brokers) != 1 || d.brokers[0] != canonical {
			t.Errorf("bootstrap addresses: %v", d.brokers)
		}
		if err := d.Send([]byte(`{"test":true}`)); err != nil {
			d.Close()
			t.Fatal(err)
		}
		d.Close()
		if !cfg.Net.ResolveCanonicalBootstrapServers || cfg.Net.Proxy.Enable || cfg.Net.Proxy.Dialer != nil {
			t.Fatal("mutated shared Sarama config")
		}
	}
	a, aaaa, ptr := counts()
	if a < 2 || aaaa != 0 || ptr < 2 {
		t.Fatalf("A=%d AAAA=%d PTR=%d", a, aaaa, ptr)
	}
}
