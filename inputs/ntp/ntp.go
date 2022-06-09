package ntp

import (
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/nux"
)

const inputName = "ntp"

type NTPStat struct {
	config.Interval
	NTPServers []string `toml:"ntp_servers"`
	server     string
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NTPStat{}
	})
}

func (n *NTPStat) Prefix() string {
	return inputName
}

func (n *NTPStat) Drop() {}

func (n *NTPStat) Init() error {
	if len(n.NTPServers) == 0 {
		return types.ErrInstancesEmpty
	}
	return nil
}

func (n *NTPStat) Gather(slist *list.SafeList) {
	for _, server := range n.NTPServers {
		if n.server == "" {
			n.server = server
		}

		orgTime := time.Now()
		serverReciveTime, serverTransmitTime, err := nux.NtpTwoTime(n.server)
		if err != nil {
			log.Println("E! failed to connect ntp server:", n.server, "error:", err)
			n.server = ""
			continue
		}

		dstTime := time.Now()

		// https://en.wikipedia.org/wiki/Network_Time_Protocol
		duration := ((serverReciveTime.UnixNano() - orgTime.UnixNano()) + (serverTransmitTime.UnixNano() - dstTime.UnixNano())) / 2

		delta := duration / 1e6 // convert to ms
		slist.PushFront(inputs.NewSample("offset_ms", delta))
		break
	}
}
