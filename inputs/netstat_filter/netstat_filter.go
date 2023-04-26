package netstat

import (
	"fmt"
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
	config.InstanceConfig

	ps         system.PS
	Laddr_IP   string `toml:"laddr_ip"`
	Laddr_Port uint32 `toml:"laddr_port"`
	Raddr_IP   string `toml:"raddr_ip"`
	Raddr_Port uint32 `toml:"raddr_port"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NetStatFilter{}
	})
}

func (l *NetStatFilter) Clone() inputs.Input {
	return &NetStatFilter{}
}

func (l *NetStatFilter) Name() string {
	return inputName
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
	if len(ins.Laddr_IP) != 0 ||
		len(ins.Raddr_IP) != 0 ||
		ins.Laddr_Port != zero ||
		ins.Raddr_Port != zero {
		ins.ps = system.NewSystemPS()
		return nil
	}
	return types.ErrInstancesEmpty
}
func (ins *Instance) Gather(slist *types.SampleList) {
	netconns, err := ins.ps.NetConnections()
	if err != nil {
		log.Println("E! failed to get net connections:", err)
		return
	}

	counts := make(map[string]int)

	// TODO: add family to tags or else
	tags := map[string]string{}

	if len(ins.Laddr_IP) != 0 {
		tags["laddr_ip"] = ins.Laddr_IP
	}

	if ins.Laddr_Port != 0 {
		tags["laddr_port"] = fmt.Sprint(ins.Laddr_Port)
	}

	if len(ins.Raddr_IP) != 0 {
		tags["raddr_ip"] = ins.Raddr_IP
	}

	if ins.Raddr_Port != 0 {
		tags["raddr_port"] = fmt.Sprint(ins.Raddr_Port)
	}

	for _, netcon := range netconns {
		if netcon.Type == syscall.SOCK_DGRAM {
			continue // UDP has no status
		}

		c, ok := counts[netcon.Status]
		if !ok {
			counts[netcon.Status] = 0
		}

		if (len(ins.Laddr_IP) == 0 || ins.Laddr_IP == netcon.Laddr.IP) &&
			(ins.Laddr_Port == 0 || ins.Laddr_Port == netcon.Laddr.Port) &&
			(len(ins.Raddr_IP) == 0 || ins.Raddr_IP == netcon.Raddr.IP) &&
			(ins.Raddr_Port == 0 || ins.Raddr_Port == netcon.Raddr.Port) {
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
