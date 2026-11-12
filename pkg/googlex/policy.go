// Package googlex keeps business gRPC and credential HTTP policies together.
package googlex

import (
	"flashcat.cloud/categraf/pkg/grpcx"
	"flashcat.cloud/categraf/pkg/netx"
	"fmt"
	"google.golang.org/api/option"
	"net"
	"net/http"
	"sync"
)

// Policy owns only cloned credential transports. SDK defaults, redirects,
// certificates and metadata-specific timeouts are retained on each clone.
type Policy struct {
	mu          sync.Mutex
	clients     map[*http.Client]*http.Client
	dialOptions []option.ClientOption
}

func FromEnv() *Policy {
	if !netx.PreferIPv4() {
		return nil
	}
	p := &Policy{clients: make(map[*http.Client]*http.Client)}
	for _, dial := range grpcx.DialOptionsFromEnv() {
		p.dialOptions = append(p.dialOptions, option.WithGRPCDialOption(dial))
	}
	return p
}
func (p *Policy) Client(source *http.Client) *http.Client {
	if p == nil {
		return source
	}
	if source == nil {
		source = http.DefaultClient
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if client := p.clients[source]; client != nil {
		return client
	}
	copy := *source
	transport := source.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if original, ok := transport.(*http.Transport); ok {
		private := original.Clone()
		// Capture IPv4 at policy creation, including lazy token refreshes.
		netx.HTTPTransportConfigurerFromIPv4()(private)
		copy.Transport = private
	} else {
		copy.Transport = unsupportedTransport{}
	}
	p.clients[source] = &copy
	return &copy
}
func (p *Policy) Options() []option.ClientOption {
	if p == nil {
		return nil
	}
	opts := []option.ClientOption{option.WithAuthHTTPPolicy(p.Client, netx.LookupHostIPv4(net.DefaultResolver))}
	opts = append(opts, p.dialOptions...)
	return opts
}
func (p *Policy) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, client := range p.clients {
		client.CloseIdleConnections()
	}
}

type unsupportedTransport struct{}

func (unsupportedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("IPv4 credential policy requires an adapted custom HTTP transport")
}
