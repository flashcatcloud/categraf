// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package runtime

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

var defaultHTTPClient *http.Client

func init() { defaultHTTPClient = NewHTTPClientWithNetworkPolicy(nil) }

// NewHTTPClientWithNetworkPolicy creates a private client with the SDK's native
// HTTP, TLS, HTTP/2 and timeout defaults. The policy selects the network before
// the native dialer resolves a host. Nil retains native network selection.
func NewHTTPClientWithNetworkPolicy(networkPolicy func(string) string) *http.Client {
	defaultTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: defaultTransportDialContext(&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion:    tls.VersionTLS12,
			Renegotiation: tls.RenegotiateFreelyAsClient,
		},
	}
	if networkPolicy != nil {
		dial := defaultTransport.DialContext
		defaultTransport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dial(ctx, networkPolicy(network), address)
		}
	}
	// TODO: evaluate removing this once https://github.com/golang/go/issues/59690 has been fixed
	if http2Transport, err := http2.ConfigureTransports(defaultTransport); err == nil {
		// if the connection has been idle for 10 seconds, send a ping frame for a health check
		http2Transport.ReadIdleTimeout = 10 * time.Second
		// if there's no response to the ping within the timeout, the connection will be closed
		http2Transport.PingTimeout = 5 * time.Second
	}
	return &http.Client{
		Transport: defaultTransport,
	}
}
