package netstat

import (
	"log"
	"syscall"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

const inputName = "netstat_filter"

type NetStatFilter struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}
type Instance struct {
	ps system.PS
	config.InstanceConfig

	Laddr_IP   string `toml:"laddr_ip"`
	Laddr_Port uint32 `toml:"laddr_port"`
	Raddr_IP   string `toml:"raddr_ip"`
	Raddr_Port uint32 `toml:"raddr_port"`
}

func init() {

	inputs.Add(inputName, func() inputs.Input {
		return &NetStats{
			ps: ps,
		}
	})
}
func (l *NetStatFilter) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(l.Instances))
	for i := 0; i < len(l.Instances); i++ {
		ret[i] = l.Instances[i]
	}
	return ret
}
func (ins *Instance) Init() error {
	var zero uint32 = 0
	if len(s.Laddr_IP) != 0 ||
		len(s.Raddr_IP) != 0 ||
		s.Laddr_Port != zero ||
		s.Raddr_Port != zero {
		ins.ps = system.NewSystemPS()
		return nil
	}
	log.Println("E! not setup filter")
	return types.ErrInstancesEmpty
}
func (s *Instance) Gather(slist *types.SampleList) {
	netconns, err := s.ps.NetConnections()
	if err != nil {
		log.Println("E! failed to get net connections:", err)
		return
	}

	counts := make(map[string]int)

	// TODO: add family to tags or else
	tags := map[string]string{}
	var zero uint32 = 0
	if len(s.Laddr_IP) != 0 {
		tags["Laddr_IP"] = s.Laddr_IP
	}
	if len(s.Laddr_Port) != 0 {
		tags["Laddr_Port"] = s.Laddr_Port
	}
	if len(s.Raddr_IP) != 0 {
		tags["Raddr_IP"] = s.Raddr_IP
	}
	if len(s.Raddr_Port) != 0 {
		tags["Raddr_Port"] = s.Raddr_Port
	}

	for _, netcon := range netconns {
		if netcon.Type == syscall.SOCK_DGRAM {
			continue // UDP has no status
		}
		c, ok := counts[netcon.Status]
		if !ok {
			counts[netcon.Status] = 0
		}
		if s.Laddr_IP == netcon.Laddr.IP ||
			s.Laddr_Port == netcon.Laddr.Port ||
			s.Raddr_IP == netcon.Raddr.IP ||
			s.Raddr_Port == netcon.Raddr.Port {
			counts[netcon.Status] = c + 1
		}

	}

	fields := map[string]interface{}{
		"tcp_established": counts["ESTABLISHED"],
		"tcp_syn_sent":    counts["SYN_SENT"],
		"tcp_syn_recv":    counts["SYN_RECV"],
		"tcp_fin_wait1":   counts["FIN_WAIT1"],
		"tcp_fin_wait2":   counts["FIN_WAIT2"],
		"tcp_time_wait":   counts["TIME_WAIT"],
		"tcp_close":       counts["CLOSE"],
		"tcp_close_wait":  counts["CLOSE_WAIT"],
		"tcp_last_ack":    counts["LAST_ACK"],
		"tcp_listen":      counts["LISTEN"],
		"tcp_closing":     counts["CLOSING"],
		"tcp_none":        counts["NONE"],
	}

	slist.PushSamples(inputName, fields, tags)
}
