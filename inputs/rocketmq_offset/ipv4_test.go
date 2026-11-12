package rocketmq_offset

import (
	"net/http"
	"strings"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := &Instance{RocketMQConsoleIPAndPort: strings.TrimPrefix(endpoint, "http://")}
		if err := ins.Init(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ins.Drop)
		return ins.client
	})
}

func TestDropPreservesSharedUserTransport(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	previous := http.DefaultClient
	source := &sharedTransport{}
	http.DefaultClient = &http.Client{Transport: source}
	t.Cleanup(func() { http.DefaultClient = previous })
	ins := &Instance{RocketMQConsoleIPAndPort: "localhost:8080"}
	if err := ins.Init(); err != nil {
		t.Fatal(err)
	}
	ins.Drop()
	if source.closed {
		t.Fatal("closed the user's shared transport")
	}
}

type sharedTransport struct{ closed bool }

func (s *sharedTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }
func (s *sharedTransport) CloseIdleConnections()                           { s.closed = true }
