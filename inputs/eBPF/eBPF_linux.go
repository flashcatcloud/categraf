//go:build linux

package eBPF

import (
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"sync"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/cilium/ebpf/link"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go bpf rdma.c -- -I./headers

const inputName = "eBPF"

type eBPF struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &eBPF{}
	})
}

func (pt *eBPF) Clone() inputs.Input {
	return &eBPF{}
}

func (pt *eBPF) Name() string {
	return inputName
}

func (pt *eBPF) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

func (pt *eBPF) Drop() {
	for i := 0; i < len(pt.Instances); i++ {
		pt.Instances[i].Drop()
	}
}

type Instance struct {
	config.InstanceConfig
	Interface string `toml:"interface"`
	objs      bpfObjects
	l         link.Link
}

func (ins *Instance) Init() error {
	iface, err := net.InterfaceByName(ins.Interface)
	if err != nil {
		return fmt.Errorf("lookup network iface %q: %s", ins.Interface, err)
	}

	// Load pre-compiled programs into the kernel.
	ins.objs = bpfObjects{}
	if err := loadBpfObjects(&ins.objs, nil); err != nil {
		return fmt.Errorf("loading objects: %s", err)
	}

	// Attach the program.
	ins.l, err = link.AttachXDP(link.XDPOptions{
		Program:   ins.objs.PacketMonitor,
		Interface: iface.Index,
	})
	if err != nil {
		return fmt.Errorf("could not attach XDP program: %s", err)
	}
	return nil
}

func (ins *Instance) Drop() {
	if ins.l != nil {
		ins.l.Close()
	}
	if !reflect.DeepEqual(ins.objs, bpfObjects{}) {
		ins.objs.Close()
	}
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var (
		key netip.Addr
		val bpfCounters
		wg  sync.WaitGroup
	)
	iter := ins.objs.PacketCnt.Iterate()
	for iter.Next(&key, &val) {
		wg.Add(1)
		go func(k netip.Addr, v bpfCounters) {
			ins.gatherOneSrc(k, v, slist)
			wg.Done()
		}(key, val)
	}
	wg.Wait()
	return
}

func (ins *Instance) gatherOneSrc(key netip.Addr, val bpfCounters, slist *types.SampleList) {
	tags := make(map[string]string)
	fields := make(map[string]interface{})
	tags["source_IP"] = fmt.Sprintf("%s", key)
	tags["interface"] = ins.Interface
	fields["rx_packets"] = val.Pkts
	fields["rx_bytes"] = val.Bytes
	slist.PushSamples(inputName, fields, tags)
}
