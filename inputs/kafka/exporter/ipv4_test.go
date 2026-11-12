package exporter

import (
	"net"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	"github.com/IBM/sarama"
	"github.com/go-kit/log"
)

func TestMetricsKafkaIPv4CreationAndRebuild(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "broker.invalid.", net.IPv4(127, 0, 0, 1), nil)
			broker := sarama.NewMockBroker(t, 0)
			defer broker.Close()
			_, port, _ := net.SplitHostPort(broker.Addr())
			address := net.JoinHostPort("broker.invalid.", port)
			broker.SetHandlerByMap(map[string]sarama.MockResponse{
				"ApiVersionsRequest": sarama.NewMockApiVersionsResponse(t),
				"MetadataRequest":    sarama.NewMockMetadataResponse(t).SetBroker(address, 0).SetLeader("metrics", 0, 0),
			})
			opts := Options{Uri: []string{address}, KafkaVersion: "2.5.0", MetadataRefreshInterval: "1m", DisableCalculateLagRate: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second}
			exporter, err := New(log.NewNopLogger(), opts, ".*", "", ".*", "")
			if err != nil {
				t.Fatal(err)
			}
			defer exporter.Close()
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			rebuilt, err := exporter.newClient()
			if err != nil {
				t.Fatal(err)
			}
			rebuilt.Close()
			a, aaaa := counts()
			if a < 2 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
