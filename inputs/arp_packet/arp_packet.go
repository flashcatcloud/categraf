package arp_packet

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const inputName = "arp_packet"

type ArpPacket struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &ArpPacket{}
	})
}

func (r *ArpPacket) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

var (
	snapshot_len int32         = 1024
	promiscuous  bool          = false
	timeout      time.Duration = 30 * time.Second
	reqARP       uint64        = 0
	resARP       uint64        = 0
	mutex        sync.Mutex
)

type Instance struct {
	config.InstanceConfig
	Ethdevice string `toml:"url"`

	EthHandle *pcap.Handle
	LocalIP   string
}

func (ins *Instance) Init() error {
	if len(ins.Ethdevice) == 0 {
		return types.ErrInstancesEmpty
	}
	ins.LocalIP = config.Config.Global.IP
	go ins.arpStat()
	fmt.Println("I! start arp stat")
	return nil
}
func (ins *Instance) Gather(slist *types.SampleList) {
	tags := map[string]string{"sourceAddr": ins.LocalIP}
	fields := make(map[string]interface{})
	fields["request_num"] = reqARP
	fields["response_num"] = resARP
	slist.PushSamples(inputName, fields, tags)
}

func (ins *Instance) arpStat() {
	// Open device
	var err error
	ins.EthHandle, err = pcap.OpenLive(ins.Ethdevice, snapshot_len, promiscuous, timeout)
	if err != nil {
		log.Fatal(err)
		return
	}

	var filter string = "arp"
	err = ins.EthHandle.SetBPFFilter(filter)

	defer ins.EthHandle.Close()
	// Use the handle as a packet source to process all packets
	packetSource := gopacket.NewPacketSource(ins.EthHandle, ins.EthHandle.LinkType())

	for {
		select {
		case p := <-packetSource.Packets():
			arp := p.Layer(layers.LayerTypeARP).(*layers.ARP)
			if arp.Operation == 2 {
				macs := net.HardwareAddr(arp.SourceHwAddress)
				macd := net.HardwareAddr(arp.DstHwAddress)
				var sip, dip net.IP
				sip = arp.SourceProtAddress
				sourceAddr := sip.String()
				dip = arp.DstProtAddress
				if sourceAddr == ins.LocalIP {
					fmt.Println("send res")
					fmt.Println("ARPResp: SourceProtAddress:", sourceAddr, " mac:", macs)

					fmt.Println("ARPResp: DstProtAddress:", dip.String(), " mac:", macd)
					defer func() {
						mutex.Unlock()
					}()
					mutex.Lock()
					resARP++

				}

			} else if arp.Operation == 1 {
				macs := net.HardwareAddr(arp.SourceHwAddress)
				macd := net.HardwareAddr(arp.DstHwAddress)
				var sip, dip net.IP
				sip = arp.SourceProtAddress
				sourceAddr := sip.String()
				dip = arp.DstProtAddress
				if sourceAddr == ins.LocalIP {
					fmt.Println("send req")
					fmt.Println("ARPResp: SourceProtAddress:", sourceAddr, " mac:", macs)

					fmt.Println("ARPResp: DstProtAddress:", dip.String(), " mac:", macd)
					defer func() {
						mutex.Unlock()
					}()
					mutex.Lock()
					reqARP++
				}
			}
		}
	}
}
