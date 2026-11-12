// Package httppolicy provides optional per-instance network hooks. Nil policies
// retain upstream defaults. Callbacks must be safe for concurrent use.
package httppolicy

import (
	"context"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/go-zookeeper/zk"
	"github.com/prometheus/common/config"
	"google.golang.org/api/option"
	"net"
	"net/http"
	"time"
)

type Policy struct {
	HTTPClientOptions   []config.HTTPClientOption
	ConfigureHTTPClient func(*http.Client) *http.Client
	// OwnHTTPClient registers an SDK-native private client for shutdown cleanup.
	OwnHTTPClient  func(*http.Client)
	LookupHost     func(context.Context, string) ([]string, error)
	Network        func(string) string
	AWSOptions     []func(*awsconfig.LoadOptions) error
	GoogleOptions  []option.ClientOption
	ZKHostProvider func(time.Duration) zk.HostProvider
	ZKDialer       zk.Dialer
}

func (p *Policy) HTTPOptions() []config.HTTPClientOption {
	if p == nil {
		return nil
	}
	return p.HTTPClientOptions
}
func (p *Policy) Client(c *http.Client) *http.Client {
	if p != nil && p.ConfigureHTTPClient != nil {
		return p.ConfigureHTTPClient(c)
	}
	return c
}
func (p *Policy) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	if p != nil && p.Network != nil {
		network = p.Network(network)
	}
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

// AWSConfigOptions returns a private option slice so independent discoverers can
// prepend shared credential policies without changing each other's options.
func (p *Policy) AWSConfigOptions() []func(*awsconfig.LoadOptions) error {
	if p == nil {
		return nil
	}
	return append([]func(*awsconfig.LoadOptions) error(nil), p.AWSOptions...)
}
