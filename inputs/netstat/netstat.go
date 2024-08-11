package netstat

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/toolkits/pkg/file"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

const inputName = "netstat"

type NetStats struct {
	ps system.PS
	config.PluginConfig

	DisableSummaryStats    bool `toml:"disable_summary_stats"`
	DisableConnectionStats bool `toml:"disable_connection_stats"`
	TcpExt                 bool `toml:"tcp_ext"`
	IpExt                  bool `toml:"ip_ext"`
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(inputName, func() inputs.Input {
		return &NetStats{
			ps: ps,
		}
	})
}

func (s *NetStats) Clone() inputs.Input {
	return &NetStats{
		ps: system.NewSystemPS(),
	}
}

func (s *NetStats) Name() string {
	return inputName
}

func (s *NetStats) gatherSummary(slist *types.SampleList) {
	if s.DisableSummaryStats {
		return
	}
	if runtime.GOOS != "linux" {
		return
	}
	tags := map[string]string{}
	f := "/proc/net/sockstat"
	prefix, ok := os.LookupEnv("HOST_MOUNT_PREFIX")
	if ok {
		f = path.Join(prefix, f)
	}
	bs, err := os.ReadFile(f)
	if err != nil {
		log.Println("E! failed to read sockstat", f, err)
		return
	}
	reader := bufio.NewReader(bytes.NewBuffer(bs))

	for {
		var lineBytes []byte
		lineBytes, err = file.ReadLine(reader)
		if err == io.EOF {
			return
		}
		line := string(lineBytes)
		s := strings.SplitN(line, ":", 2)
		if len(s) != 2 {
			continue
		}
		metric := strings.ToLower(strings.TrimSpace(s[0]))
		kvs := strings.Split(strings.TrimSpace(s[1]), " ")
		if len(kvs)%2 != 0 {
			continue
		}
		for i := 0; i < len(kvs); i += 2 {
			val, err := strconv.ParseUint(kvs[i+1], 10, 64)
			if err != nil {
				log.Println("W! parse:", f, "line:", line, "field:", kvs[i+1], "failed:", err)
			}
			slist.PushSample(inputName+"_"+metric, strings.ToLower(kvs[i]), val, tags)
		}
	}
}

func (s *NetStats) Gather(slist *types.SampleList) {
	s.gatherExt(slist)

	s.gatherSummary(slist)

	if s.DisableConnectionStats {
		return
	}
	netconns, err := s.ps.NetConnections()
	if err != nil {
		log.Println("E! failed to get net connections:", err)
		return
	}

	counts := make(map[string]int)
	counts["UDP"] = 0

	// TODO: add family to tags or else
	tags := map[string]string{}
	for _, netcon := range netconns {
		if netcon.Type == syscall.SOCK_DGRAM {
			counts["UDP"]++
			continue // UDP has no status
		}
		c, ok := counts[netcon.Status]
		if !ok {
			counts[netcon.Status] = 0
		}
		counts[netcon.Status] = c + 1
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
		"udp_socket":      counts["UDP"],
	}

	slist.PushSamples(inputName, fields, tags)
}

func (s *NetStats) gatherExt(slist *types.SampleList) {
	if !s.TcpExt && !s.IpExt {
		return
	}
	tags := map[string]string{}
	proc := Proc{PID: 0, fs: "/proc"}
	n, err := proc.Netstat()
	if n == nil {
		return
	}
	if err != nil {
		log.Println("E! failed to get ext metrics:", err)
		return
	}

	if s.TcpExt {
		slist.PushSamples(inputName+"_tcpext", n.TcpExt, tags)
	}

	if s.IpExt {
		slist.PushSamples(inputName+"_ipext", n.IpExt, tags)
	}
}
