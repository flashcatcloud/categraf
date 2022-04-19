package ping

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "ping"

type PingInstance struct {
	Targets       []string `toml:"targets"`
	IntervalTimes int64    `toml:"interval_times"`
	Count         int      `toml:"count"`         // ping -c <COUNT>
	PingInterval  float64  `toml:"ping_interval"` // ping -i <INTERVAL>
	Timeout       float64  `toml:"timeout"`       // ping -W <TIMEOUT>
	Deadline      int      `toml:"deadline"`      // ping -w <DEADLINE>
	Interface     string   `toml:"interface"`     // ping -I/-S <INTERFACE/SRC_ADDR>
	IPv6          bool     `toml:"ipv6"`          // Whether to resolve addresses using ipv6 or not.
	Size          int      `toml:"size"`          // Packet size

	calcInterval  time.Duration
	calcTimeout   time.Duration
	sourceAddress string
}

func (ins *PingInstance) Init() error {
	if ins.Count < 1 {
		ins.Count = 1
	}

	if ins.PingInterval < 0.2 {
		ins.calcInterval = time.Duration(0.2 * float64(time.Second))
	} else {
		ins.calcInterval = time.Duration(ins.PingInterval * float64(time.Second))
	}

	if ins.Timeout == 0 {
		ins.calcTimeout = time.Duration(5) * time.Second
	} else {
		ins.calcTimeout = time.Duration(ins.Timeout) * time.Second
	}

	if ins.Deadline < 0 {
		ins.Deadline = 10
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
	Interval  config.Duration `toml:"interval"`
	Instances []*PingInstance `toml:"instances"`
	Counter   uint64
	wg        sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Ping{}
	})
}

func (p *Ping) GetInputName() string {
	return inputName
}

func (p *Ping) GetInterval() config.Duration {
	return p.Interval
}

func (p *Ping) Init() error {
	if len(p.Instances) == 0 {
		return fmt.Errorf("ping instances empty")
	}

	for i := 0; i < len(p.Instances); i++ {
		if err := p.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (p *Ping) Drop() {}

func (p *Ping) Gather() (samples []*types.Sample) {
	atomic.AddUint64(&p.Counter, 1)

	slist := list.NewSafeList()

	for i := range p.Instances {
		ins := p.Instances[i]
		p.wg.Add(1)
		go p.gatherOnce(slist, ins)
	}
	p.wg.Wait()

	interfaceList := slist.PopBackAll()
	for i := 0; i < len(interfaceList); i++ {
		samples = append(samples, interfaceList[i].(*types.Sample))
	}

	return
}

func (p *Ping) gatherOnce(slist *list.SafeList, ins *PingInstance) {
	defer p.wg.Done()

	fmt.Println("ping.....", ins.Targets)
}
