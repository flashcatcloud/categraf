package ntp

import (
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/beevik/ntp"
)

const inputName = "ntp"

type NTPStat struct {
	config.PluginConfig
	NTPServers []string `toml:"ntp_servers"`
	TimeOut    int64    `toml:"timeout"`
	server     string
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NTPStat{
			// default timeout is 5 seconds
			TimeOut: 5,
		}
	})
}

func (n *NTPStat) Clone() inputs.Input {
	return &NTPStat{}
}

func (n *NTPStat) Name() string {
	return inputName
}

func (n *NTPStat) Init() error {
	if len(n.NTPServers) == 0 {
		return types.ErrInstancesEmpty
	}
	return nil
}

func (n *NTPStat) Gather(slist *types.SampleList) {
	for _, server := range n.NTPServers {
		if n.server == "" {
			n.server = server
		}

		resp, err := ntp.QueryWithOptions(n.server, ntp.QueryOptions{
			Timeout: time.Duration(n.TimeOut) * time.Second,
			Version: 4,
		})

		if err != nil {
			log.Println("E! failed to connect ntp server:", n.server, "error:", err)
			n.server = ""
			continue
		}

		// offset in ms
		delta := resp.ClockOffset.Seconds() * 1000
		slist.PushFront(types.NewSample(inputName, "offset_ms", delta).SetTime(resp.Time))
		break
	}
}
