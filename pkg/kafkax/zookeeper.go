package kafkax

import (
	"flashcat.cloud/categraf/pkg/netx"
	"flashcat.cloud/categraf/pkg/zkx"
	"fmt"
	"github.com/krallistic/kazoo-go"
)

// NewKazooFromEnv covers both eager bootstrap resolution and future dials.
func NewKazooFromEnv(servers []string, config *kazoo.Config) (*kazoo.Kazoo, error) {
	if !netx.PreferIPv4() {
		return kazoo.NewKazoo(servers, config)
	}
	if config == nil {
		config = kazoo.NewConfig()
	}
	if config.HostProvider != nil || config.Dialer != nil {
		return nil, fmt.Errorf("IPv4 ZooKeeper policy requires an adapted custom host provider and dialer")
	}
	copy := *config
	copy.HostProvider = zkx.NewHostProvider(copy.Timeout)
	copy.Dialer = zkx.DialerFromEnv()
	return kazoo.NewKazoo(servers, &copy)
}
