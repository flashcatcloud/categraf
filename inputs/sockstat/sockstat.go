package sockstat

import (
	"errors"
	"log"
	"os"
	"strings"

	"github.com/toolkits/pkg/slice"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "sockstat"

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &SockStat{}
	})
}

func (s *SockStat) Clone() inputs.Input {
	return &SockStat{}
}

func (s *SockStat) Name() string {
	return inputName
}

type SockStat struct {
	config.PluginConfig

	Protocols []string `toml:"protocols"`
}

// A NetSockstat contains the output of /proc/net/sockstat{,6} for IPv4 or IPv6,
// respectively.
type NetSockstat struct {
	// Used is non-nil for IPv4 sockstat results, but nil for IPv6.
	Used      *int
	Protocols []NetSockstatProtocol
}

// A NetSockstatProtocol contains statistics about a given socket protocol.
// Pointer fields indicate that the value may or may not be present on any
// given protocol.
type NetSockstatProtocol struct {
	Protocol string
	InUse    int
	Orphan   *int
	TW       *int
	Alloc    *int
	Mem      *int
	Memory   *int
}

func (ss *SockStat) Gather(slist *types.SampleList) {
	ns, err := ParseNetSockstat()
	if err != nil {
		log.Println("E! failed to get net sockstat: ", err)
		return
	}
	ss.parse(ns, slist)

	ns6, err := ParseNetSockstat6()
	if err != nil {
		if config.Config.DebugMode {
			log.Println("D! failed to get net sockstat6: ", err)
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			log.Println("E! failed to get net sockstat6: ", err)
			return
		}
	}
	ss.parse(ns6, slist)
}

func (ss *SockStat) parse(ns *NetSockstat, slist *types.SampleList) {
	if ns == nil {
		return
	}
	samples := map[string]interface{}{}
	if ns.Used != nil {
		samples["used"] = *ns.Used
	}

	for _, p := range ns.Protocols {
		protocol := strings.ToLower(p.Protocol)
		if len(ss.Protocols) > 0 && !slice.ContainsString(ss.Protocols, protocol) {
			continue
		}
		samples[protocol+"_inuse"] = p.InUse
		if p.Orphan != nil {
			samples[protocol+"_orphan"] = *p.Orphan
		}
		if p.TW != nil {
			samples[protocol+"_tw"] = *p.TW
		}
		if p.Alloc != nil {
			samples[protocol+"_alloc"] = *p.Alloc
		}
		if p.Mem != nil {
			samples[protocol+"_mem"] = *p.Mem
		}
		if p.Memory != nil {
			samples[protocol+"_memory"] = *p.Memory
		}
	}
	slist.PushSamples(inputName, samples)
}
