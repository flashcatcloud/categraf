package ping

import (
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	ping "github.com/prometheus-community/pro-bing"
)

const (
	inputName                = "ping"
	defaultPingDataBytesSize = 56
)

type Instance struct {
	config.InstanceConfig

	Targets      []string `toml:"targets"`
	Count        int      `toml:"count"`         // ping -c <COUNT>
	PingInterval float64  `toml:"ping_interval"` // ping -i <INTERVAL>
	Timeout      float64  `toml:"timeout"`       // ping -W <TIMEOUT>
	Interface    string   `toml:"interface"`     // ping -I/-S <INTERFACE/SRC_ADDR>
	IPv6         bool     `toml:"ipv6"`          // Whether to resolve addresses using ipv6 or not.
	Size         *int     `toml:"size"`          // Packet size
	Conc         int      `toml:"concurrency"`   // max concurrency coroutine

	calcInterval  time.Duration
	calcTimeout   time.Duration
	sourceAddress string
}

func (ins *Instance) Init() error {
	if len(ins.Targets) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.Count < 1 {
		ins.Count = 1
	}

	if ins.Conc == 0 {
		ins.Conc = 10
	}

	if ins.PingInterval < 0.2 {
		ins.calcInterval = time.Duration(0.2 * float64(time.Second))
	} else {
		ins.calcInterval = time.Duration(ins.PingInterval * float64(time.Second))
	}

	if ins.Timeout == 0 {
		ins.calcTimeout = time.Duration(3) * time.Second
	} else {
		ins.calcTimeout = time.Duration(ins.Timeout * float64(time.Second))
	}

	if ins.Interface != "" {
		if addr := net.ParseIP(ins.Interface); addr != nil {
			ins.sourceAddress = ins.Interface
		} else {
			i, err := net.InterfaceByName(ins.Interface)
			if err != nil {
				return fmt.Errorf("failed to get interface: %v", err)
			}

			addrs, err := i.Addrs()
			if err != nil {
				return fmt.Errorf("failed to get the address of interface: %v", err)
			}

			ins.sourceAddress = addrs[0].(*net.IPNet).IP.String()
		}
	}

	return nil
}

type Ping struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Ping{}
	})
}

func (p *Ping) Clone() inputs.Input {
	return &Ping{}
}

func (p *Ping) Name() string {
	return inputName
}

func (p *Ping) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(p.Instances))
	for i := 0; i < len(p.Instances); i++ {
		ret[i] = p.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.Targets) == 0 {
		return
	}

	wg := new(sync.WaitGroup)
	ch := make(chan struct{}, ins.Conc)
	for _, target := range ins.Targets {
		ch <- struct{}{}
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			ins.gather(slist, target)
			<-ch
		}(target)
	}
	wg.Wait()
}

func (ins *Instance) gather(slist *types.SampleList, target string) {
	if config.Config.DebugMode {
		log.Println("D! ping...", target)
	}

	labels := map[string]string{"target": target}

	fields := map[string]interface{}{}

	defer func() {
		for field, value := range fields {
			slist.PushFront(types.NewSample(inputName, field, value, labels))
		}
	}()

	stats, err := ins.ping(target)
	if err != nil {
		log.Println("E! failed to ping:", target, "error:", err)
		if strings.Contains(err.Error(), "unknown") {
			fields["result_code"] = 1
		} else {
			fields["result_code"] = 2
		}
		return
	}

	fields["result_code"] = 0

	if stats.PacketsSent == 0 {
		if config.Config.DebugMode {
			log.Println("D! no packets sent, target:", target)
		}
		fields["result_code"] = 2
		return
	}

	if stats.PacketsRecv == 0 {
		if config.Config.DebugMode {
			log.Println("D! no packets received, target:", target)
		}
		fields["result_code"] = 1
		fields["minimum_response_ms"] = float64(-1)
		fields["average_response_ms"] = float64(-1)
		fields["maximum_response_ms"] = float64(-1)
		fields["percent_packet_loss"] = float64(100)
		return
	}

	// Set TTL only on supported platform. See golang.org/x/net/ipv4/payload_cmsg.go
	switch runtime.GOOS {
	case "aix", "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd", "solaris":
		fields["ttl"] = stats.ttl
	}

	//nolint:unconvert // Conversion may be needed for float64 https://github.com/mdempsky/unconvert/issues/40
	fields["percent_packet_loss"] = float64(stats.PacketLoss)
	fields["minimum_response_ms"] = float64(stats.MinRtt) / float64(time.Millisecond)
	fields["average_response_ms"] = float64(stats.AvgRtt) / float64(time.Millisecond)
	fields["maximum_response_ms"] = float64(stats.MaxRtt) / float64(time.Millisecond)
	fields["standard_deviation_ms"] = float64(stats.StdDevRtt) / float64(time.Millisecond)
}

type pingStats struct {
	ping.Statistics
	ttl int
}

func (ins *Instance) ping(destination string) (*pingStats, error) {
	ps := &pingStats{}

	pinger, err := ping.NewPinger(destination)
	if err != nil {
		return nil, fmt.Errorf("failed to create new pinger: %w", err)
	}

	pinger.SetPrivileged(true)

	if ins.IPv6 {
		pinger.SetNetwork("ip6")
	}

	pinger.Size = defaultPingDataBytesSize
	if ins.Size != nil {
		pinger.Size = *ins.Size
	}

	pinger.Source = ins.sourceAddress
	pinger.Interval = ins.calcInterval
	pinger.Timeout = ins.calcTimeout

	// Get Time to live (TTL) of first response, matching original implementation
	once := &sync.Once{}
	pinger.OnRecv = func(pkt *ping.Packet) {
		once.Do(func() {
			ps.ttl = pkt.TTL
		})
	}

	pinger.Count = ins.Count
	err = pinger.Run()
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			if runtime.GOOS == "linux" {
				return nil, fmt.Errorf("permission changes required, enable CAP_NET_RAW capabilities (refer to the ping plugin's README.md for more info)")
			}

			return nil, fmt.Errorf("permission changes required, refer to the ping plugin's README.md for more info")
		}
		return nil, fmt.Errorf("%w", err)
	}

	ps.Statistics = *pinger.Statistics()

	return ps, nil
}
