// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpx

import (
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var (
	// NoProxyIgnoredWarningMap map containing URL's who will ignore the proxy in the future
	NoProxyIgnoredWarningMap = make(map[string]bool)

	// NoProxyUsedInFuture map containing URL's that will use a proxy in the future
	NoProxyUsedInFuture = make(map[string]bool)

	// NoProxyChanged map containing URL's whos proxy behavior will change in the future
	NoProxyChanged = make(map[string]bool)

	// NoProxyMapMutex Lock for all no proxy maps
	NoProxyMapMutex = sync.Mutex{}
)

func logSafeURLString(url *url.URL) string {
	if url == nil {
		return ""
	}
	return url.Scheme + "://" + url.Host
}

func warnOnce(warnMap map[string]bool, key string, format string, params ...interface{}) {
	NoProxyMapMutex.Lock()
	defer NoProxyMapMutex.Unlock()
	if _, ok := warnMap[key]; !ok {
		warnMap[key] = true
		log.Printf(format, params...)
	}
}

// CreateHTTPTransport creates an *http.Transport for use in the agent
func CreateHTTPTransport() *http.Transport {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// tlsConfig.MinVersion = tls.VersionTLS12

	// Most of the following timeouts are a copy of Golang http.DefaultTransport
	// They are mostly used to act as safeguards in case we forget to add a general
	// timeout to our http clients.  Setting DialContext and TLSClientConfig has the
	// desirable side-effect of disabling http/2; if removing those fields then
	// consider the implication of the protocol switch for intakes and other http
	// servers. See ForceAttemptHTTP2 in https://pkg.go.dev/net/http#Transport.
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
			// Enables TCP keepalives to detect broken connections
			KeepAlive: 30 * time.Second,
			// Disable RFC 6555 Fast Fallback ("Happy Eyeballs")
			FallbackDelay: -1 * time.Nanosecond,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 5,
		// This parameter is set to avoid connections sitting idle in the pool indefinitely
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return transport
}
