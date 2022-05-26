package switch_legacy

import (
	"sync"
	"sync/atomic"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "switch_legacy"

type Switch struct {
	config.Interval
	counter       uint64
	waitgrp       sync.WaitGroup
	Instances     []*Instance       `toml:"instances"`
	SwitchIdLabel string            `toml:"switch_id_label"`
	Mappings      map[string]string `toml:"mappings"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Switch{}
	})
}

func (s *Switch) Prefix() string {
	return inputName
}

func (s *Switch) Init() error {
	if len(s.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(s.Instances); i++ {
		if err := s.Instances[i].Init(); err != nil {
			return err
		} else {
			s.Instances[i].parent = s
		}
	}

	return nil
}

func (s *Switch) Drop() {}

func (s *Switch) Gather(slist *list.SafeList) {
	atomic.AddUint64(&s.counter, 1)

	for i := range s.Instances {
		ins := s.Instances[i]

		s.waitgrp.Add(1)
		go func(slist *list.SafeList, ins *Instance) {
			defer s.waitgrp.Done()

			if ins.IntervalTimes > 0 {
				counter := atomic.LoadUint64(&s.counter)
				if counter%uint64(ins.IntervalTimes) != 0 {
					return
				}
			}

			ins.gatherOnce(slist)
		}(slist, ins)
	}

	s.waitgrp.Wait()
}

type Instance struct {
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	IPs          []string `toml:"ips"`
	Community    string   `toml:"community"`
	IndexTag     bool     `toml:"index_tag"`
	IgnoreIfaces []string `toml:"ignore_ifaces"`

	ConcurrencyForAddress int `toml:"concurrency_for_address"`
	ConcurrencyForRequest int `toml:"concurrency_for_request"`

	PingEnable       int64 `toml:"ping_enable"`
	PingModeFastping bool  `toml:"ping_mode_fastping"`
	PingTimeoutMs    int64 `toml:"ping_timeout_ms"`
	PingRetries      int   `toml:"ping_retries"`

	SnmpModeGosnmp bool  `toml:"snmp_mode_gosnmp"`
	SnmpTimeoutMs  int64 `toml:"snmp_timeout_ms"`
	SnmpRetries    int   `toml:"snmp_retries"`

	GatherPingMetrics   bool `toml:"gather_ping_metrics"`
	GatherFlowMetrics   bool `toml:"gather_flow_metrics"`
	GatherCpuMetrics    bool `toml:"gather_cpu_metrics"`
	GatherMemMetrics    bool `toml:"gather_mem_metrics"`
	GatherOperStatus    bool `toml:"gather_oper_status"`
	GatherPkt           bool `toml:"gather_pkt"`
	GatherBroadcastPkt  bool `toml:"gather_broadcast_pkt"`
	GatherMulticastPkt  bool `toml:"gather_multicast_pkt"`
	GatherDiscards      bool `toml:"gather_discards"`
	GatherErrors        bool `toml:"gather_errors"`
	GatherUnknownProtos bool `toml:"gather_unknown_protos"`
	GatherOutQlen       bool `toml:"gather_out_qlen"`

	SpeedLimit            float64 `toml:"speed_limit"`
	PktLimit              float64 `toml:"pkt_limit"`
	BroadcastPktLimit     float64 `toml:"broadcast_pkt_limit"`
	MulticastPktLimit     float64 `toml:"multicast_pkt_limit"`
	DiscardsPktLimit      float64 `toml:"discards_pkt_limit"`
	ErrorsPktLimit        float64 `toml:"errors_pkt_limit"`
	UnknownProtosPktLimit float64 `toml:"unknown_protos_pkt_limit"`
	OutQlenPktLimit       float64 `toml:"out_qlen_pkt_limit"`

	Customs []Custom `toml:"customs"`

	parent *Switch
}

type Custom struct {
	Metric string            `toml:"metric"`
	Tags   map[string]string `toml:"tags"`
	OID    string            `toml:"oid"`
}

func (ins *Instance) Init() error {
	return nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) error {
	return nil
}
