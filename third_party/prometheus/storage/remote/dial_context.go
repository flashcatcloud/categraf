// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"context"
	"github.com/prometheus/prometheus/util/httppolicy"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/common/config"
)

type hostResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

type dialContextWithRoundRobinDNS struct {
	policy      *httppolicy.Policy
	mu          sync.Mutex
	dialContext config.DialContextFunc
	resolver    hostResolver
	rand        *rand.Rand
}

// newDialContextWithRoundRobinDNS creates a new dialContextWithRoundRobinDNS.
// We discourage creating new instances of struct dialContextWithRoundRobinDNS by explicitly setting its members,
// except for testing purposes, and recommend using newDialContextWithRoundRobinDNS.
func newDialContextWithRoundRobinDNS() *dialContextWithRoundRobinDNS {
	return &dialContextWithRoundRobinDNS{
		dialContext: http.DefaultTransport.(*http.Transport).DialContext,
		resolver:    net.DefaultResolver,
		rand:        rand.New(rand.NewSource(time.Now().Unix())),
	}
}

func (dc *dialContextWithRoundRobinDNS) dialContextFn() config.DialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dc.policy != nil && dc.policy.Network != nil {
			network = dc.policy.Network(network)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return dc.dialContext(ctx, network, addr)
		}

		lookup := dc.resolver.LookupHost
		if dc.policy != nil && dc.policy.LookupHost != nil {
			lookup = dc.policy.LookupHost
		}
		addrs, err := lookup(ctx, host)
		if err != nil || len(addrs) == 0 {
			return dc.dialContext(ctx, network, addr)
		}

		dc.mu.Lock()
		index := dc.rand.Intn(len(addrs))
		dc.mu.Unlock()
		randomAddr := net.JoinHostPort(addrs[index], port)
		return dc.dialContext(ctx, network, randomAddr)
	}
}
