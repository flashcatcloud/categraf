package ntp

import (
	"errors"
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/nux"
)

const inputName = "ntp"

type NTPStat struct {
	Interval   config.Duration `toml:"interval"`
	NTPServers []string        `toml:"ntp_servers"`
	server     string
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NTPStat{}
	})
}

func (n *NTPStat) GetInputName() string {
	return inputName
}

func (n *NTPStat) GetInterval() config.Duration {
	return n.Interval
}

func (n *NTPStat) Drop() {}

func (n *NTPStat) Init() error {
	if len(n.NTPServers) == 0 {
		return errors.New("ntp servers empty")
	}
	return nil
}

func (n *NTPStat) Gather() (samples []*types.Sample) {
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
		samples = append(samples, inputs.NewSample("offset_ms", delta))
		break
	}

	return
}
