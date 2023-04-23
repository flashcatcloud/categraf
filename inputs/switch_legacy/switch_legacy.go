package switch_legacy

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/pkg/runtimex"
	"flashcat.cloud/categraf/types"
	"github.com/gaochao1/sw"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/toolkits/pkg/concurrent/semaphore"
	go_snmp "github.com/ulricqin/gosnmp"
)

const inputName = "switch_legacy"

type Switch struct {
	config.PluginConfig
	Instances     []*Instance       `toml:"instances"`
	SwitchIdLabel string            `toml:"switch_id_label"`
	Mappings      map[string]string `toml:"mappings"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Switch{}
	})
}

func (s *Switch) Clone() inputs.Input {
	return &Switch{}
}

func (s *Switch) Name() string {
	return inputName
}

func (s *Switch) MappingIP(ip string) string {
	val, has := s.Mappings[ip]
	if has {
		return val
	}
	return ip
}

func (s *Switch) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}

func (s *Switch) Init() error {
	if len(s.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(s.Instances); i++ {
		if err := s.Instances[i].RealInit(); err != nil {
			return err
		} else {
			s.Instances[i].parent = s
		}
	}

	return nil
}

type Instance struct {
	config.InstanceConfig

	IPs          []string `toml:"ips"`
	Community    string   `toml:"community"`
	IndexTag     bool     `toml:"index_tag"`
	IgnoreIfaces []string `toml:"ignore_ifaces"`

	ConcurrencyForAddress int `toml:"concurrency_for_address"`
	ConcurrencyForRequest int `toml:"concurrency_for_request"`

	PingEnable       bool  `toml:"ping_enable"`
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

	parent    *Switch
	lastifmap *LastifMap
}

type Custom struct {
	Metric string            `toml:"metric"`
	Tags   map[string]string `toml:"tags"`
	OID    string            `toml:"oid"`
}

func (ins *Instance) RealInit() error {
	if len(ins.IPs) == 0 {
		return types.ErrInstancesEmpty
	}

	ips := ins.parseIPs()
	if len(ips) == 0 {
		return errors.New("ips empty")
	}

	ins.lastifmap = NewLastifMap()
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	ips := ins.parseIPs()
	if len(ips) == 0 {
		return
	}

	start := time.Now()
	defer func() {
		log.Println("I! switch gather use:", time.Since(start))
	}()

	log.Println("I! switch total ip count:", len(ips))

	if ins.PingEnable {
		ips = ins.gatherPing(ips, slist)
	}

	if ins.GatherFlowMetrics {
		ins.gatherFlowMetrics(ips, slist)
	}

	if ins.GatherCpuMetrics {
		ins.gatherCpuMetrics(ips, slist)
	}

	if ins.GatherMemMetrics {
		ins.gatherMemMetrics(ips, slist)
	}

	if len(ins.Customs) > 0 {
		ins.gatherCustoms(ips, slist)
	}
}

func (ins *Instance) gatherCustoms(ips []string, slist *types.SampleList) {
	wg := new(sync.WaitGroup)

	for i := 0; i < len(ips); i++ {
		ip := ips[i]
		for j := 0; j < len(ins.Customs); j++ {
			wg.Add(1)
			go ins.custstat(wg, ip, slist, ins.Customs[j])
		}
	}

	wg.Wait()
}

func (ins *Instance) custstat(wg *sync.WaitGroup, ip string, slist *types.SampleList, cust Custom) {
	defer wg.Done()

	defer func() {
		if r := recover(); r != nil {
			log.Println("E! recovered in custstat, ip:", ip, "oid:", cust.OID, "error:", r, "stack:", runtimex.Stack(3))
		}
	}()

	var value float64
	var err error
	var snmpPDUs []go_snmp.SnmpPDU
	for i := 0; i < ins.SnmpRetries; i++ {
		snmpPDUs, err = sw.RunSnmp(ip, ins.Community, cust.OID, "get", int(ins.SnmpTimeoutMs))
		if len(snmpPDUs) > 0 && err == nil {
			value, err = conv.ToFloat64(snmpPDUs[0].Value)
			if err == nil {
				slist.PushFront(types.NewSample(inputName, cust.Metric, value, cust.Tags, map[string]string{ins.parent.SwitchIdLabel: ins.parent.MappingIP(ip)}))
			} else {
				log.Println("E! failed to convert to float64, ip:", ip, "oid:", cust.OID, "value:", snmpPDUs[0].Value)
			}
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (ins *Instance) gatherMemMetrics(ips []string, slist *types.SampleList) {
	result := cmap.New()
	for i := 0; i < len(ips); i++ {
		result.Set(ips[i], -1.0)
	}

	wg := new(sync.WaitGroup)
	se := semaphore.NewSemaphore(ins.ConcurrencyForAddress)
	for i := 0; i < len(ips); i++ {
		ip := ips[i]
		wg.Add(1)
		se.Acquire()
		go ins.memstat(wg, se, ip, result)
	}
	wg.Wait()

	for ip, utilPercentInterface := range result.Items() {
		utilPercent, ok := utilPercentInterface.(float64)
		if !ok {
			continue
		}

		if utilPercent < 0 {
			continue
		}
		slist.PushFront(types.NewSample(inputName, "mem_util", utilPercent, map[string]string{ins.parent.SwitchIdLabel: ins.parent.MappingIP(ip)}))
	}
}

func (ins *Instance) memstat(wg *sync.WaitGroup, sema *semaphore.Semaphore, ip string, result cmap.ConcurrentMap) {
	defer func() {
		sema.Release()
		wg.Done()
	}()

	utilPercent, err := sw.MemUtilization(ip, ins.Community, int(ins.SnmpTimeoutMs), ins.SnmpRetries)
	if err != nil {
		log.Println("E! failed to gather mem, ip:", ip, "error:", err)
		return
	}

	result.Set(ip, float64(utilPercent))
}

func (ins *Instance) gatherCpuMetrics(ips []string, slist *types.SampleList) {
	result := cmap.New()
	for i := 0; i < len(ips); i++ {
		result.Set(ips[i], -1.0)
	}

	wg := new(sync.WaitGroup)
	se := semaphore.NewSemaphore(ins.ConcurrencyForAddress)
	for i := 0; i < len(ips); i++ {
		ip := ips[i]
		wg.Add(1)
		se.Acquire()
		go ins.cpustat(wg, se, ip, result)
	}
	wg.Wait()

	for ip, utilPercentInterface := range result.Items() {
		utilPercent, ok := utilPercentInterface.(float64)
		if !ok {
			continue
		}

		if utilPercent < 0 {
			continue
		}

		slist.PushFront(types.NewSample(inputName, "cpu_util", utilPercent, map[string]string{ins.parent.SwitchIdLabel: ins.parent.MappingIP(ip)}))
	}
}

func (ins *Instance) cpustat(wg *sync.WaitGroup, sema *semaphore.Semaphore, ip string, result cmap.ConcurrentMap) {
	defer func() {
		sema.Release()
		wg.Done()
	}()

	utilPercent, err := sw.CpuUtilization(ip, ins.Community, int(ins.SnmpTimeoutMs), ins.SnmpRetries)
	if err != nil {
		log.Println("E! failed to gather cpu, ip:", ip, "error:", err)
		return
	}

	result.Set(ip, float64(utilPercent))
}

type ChIfStat struct {
	IP          string
	UseTime     time.Duration
	IfStatsList []sw.IfStats
}

func (ins *Instance) gatherFlowMetrics(ips []string, slist *types.SampleList) {
	result := cmap.New()
	for i := 0; i < len(ips); i++ {
		result.Set(ips[i], nil)
	}

	wg := new(sync.WaitGroup)
	se := semaphore.NewSemaphore(ins.ConcurrencyForAddress)
	for i := 0; i < len(ips); i++ {
		ip := ips[i]
		wg.Add(1)
		se.Acquire()
		go ins.ifstat(wg, se, ip, result)
	}
	wg.Wait()

	for ip, chifstatInterface := range result.Items() {
		if chifstatInterface == nil {
			continue
		}

		chifstat, ok := chifstatInterface.(*ChIfStat)
		if !ok {
			continue
		}

		if chifstat == nil {
			continue
		}

		if chifstat.IP == "" {
			continue
		}

		stats := chifstat.IfStatsList
		slist.PushFront(types.NewSample(inputName, "ifstat_use_time_sec", chifstat.UseTime.Seconds(), map[string]string{ins.parent.SwitchIdLabel: ins.parent.MappingIP(ip)}))

		for i := 0; i < len(stats); i++ {
			ifStat := stats[i]

			if ifStat.IfName == "" {
				continue
			}

			tags := map[string]string{
				ins.parent.SwitchIdLabel: ins.parent.MappingIP(ip),
				"ifname":                 ifStat.IfName,
			}

			if ins.IndexTag {
				tags["ifindex"] = fmt.Sprint(ifStat.IfIndex)
			}

			if ins.GatherOperStatus {
				slist.PushFront(types.NewSample(inputName, "if_oper_status", ifStat.IfOperStatus, tags))
			}

			slist.PushFront(types.NewSample(inputName, "if_speed", ifStat.IfSpeed, tags))

			if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
				for _, lastifStat := range lastIfStatList {
					if ifStat.IfIndex == lastifStat.IfIndex {
						interval := ifStat.TS - lastifStat.TS

						speedlimit := ins.SpeedLimit
						if speedlimit == 0 {
							speedlimit = float64(ifStat.IfSpeed)
						}

						IfHCInOctets := 8 * (float64(ifStat.IfHCInOctets) - float64(lastifStat.IfHCInOctets)) / float64(interval)
						IfHCOutOctets := 8 * (float64(ifStat.IfHCOutOctets) - float64(lastifStat.IfHCOutOctets)) / float64(interval)

						if limitCheck(IfHCInOctets, speedlimit) {
							slist.PushFront(types.NewSample(inputName, "if_in", IfHCInOctets, tags))
							if ifStat.IfSpeed > 0 {
								slist.PushFront(types.NewSample(inputName, "if_in_speed_percent", 100*IfHCInOctets/float64(ifStat.IfSpeed), tags))
							}
						} else {
							log.Println("W! if_in out of range, current:", ifStat.IfHCInOctets, "lasttime:", lastifStat.IfHCInOctets, "tags:", tags)
						}

						if limitCheck(IfHCOutOctets, speedlimit) {
							slist.PushFront(types.NewSample(inputName, "if_out", IfHCOutOctets, tags))
							if ifStat.IfSpeed > 0 {
								slist.PushFront(types.NewSample(inputName, "if_out_speed_percent", 100*IfHCOutOctets/float64(ifStat.IfSpeed), tags))
							}
						} else {
							log.Println("W! if_out out of range, current:", ifStat.IfHCOutOctets, "lasttime:", lastifStat.IfHCOutOctets, "tags:", tags)
						}
					}
				}
			}

			if ins.GatherBroadcastPkt {
				if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
					for _, lastifStat := range lastIfStatList {
						if ifStat.IfIndex == lastifStat.IfIndex {
							interval := ifStat.TS - lastifStat.TS

							IfHCInBroadcastPkts := (float64(ifStat.IfHCInBroadcastPkts) - float64(lastifStat.IfHCInBroadcastPkts)) / float64(interval)
							IfHCOutBroadcastPkts := (float64(ifStat.IfHCOutBroadcastPkts) - float64(lastifStat.IfHCOutBroadcastPkts)) / float64(interval)

							if limitCheck(IfHCInBroadcastPkts, ins.BroadcastPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_in_broadcast_pkt", IfHCInBroadcastPkts, tags))
							} else {
								log.Println("W! if_in_broadcast_pkt out of range, current:", ifStat.IfHCInBroadcastPkts, "lasttime:", lastifStat.IfHCInBroadcastPkts, "tags:", tags)
							}

							if limitCheck(IfHCOutBroadcastPkts, ins.BroadcastPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_out_broadcast_pkt", IfHCOutBroadcastPkts, tags))
							} else {
								log.Println("W! if_out_broadcast_pkt out of range, current:", ifStat.IfHCOutBroadcastPkts, "lasttime:", lastifStat.IfHCOutBroadcastPkts, "tags:", tags)
							}
						}
					}
				}
			}

			if ins.GatherMulticastPkt {
				if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
					for _, lastifStat := range lastIfStatList {
						if ifStat.IfIndex == lastifStat.IfIndex {
							interval := ifStat.TS - lastifStat.TS

							IfHCInMulticastPkts := (float64(ifStat.IfHCInMulticastPkts) - float64(lastifStat.IfHCInMulticastPkts)) / float64(interval)
							IfHCOutMulticastPkts := (float64(ifStat.IfHCOutMulticastPkts) - float64(lastifStat.IfHCOutMulticastPkts)) / float64(interval)

							if limitCheck(IfHCInMulticastPkts, ins.MulticastPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_in_multicast_pkt", IfHCInMulticastPkts, tags))
							} else {
								log.Println("W! if_in_multicast_pkt out of range, current:", ifStat.IfHCInMulticastPkts, "lasttime:", lastifStat.IfHCInMulticastPkts, "tags:", tags)
							}

							if limitCheck(IfHCOutMulticastPkts, ins.MulticastPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_out_multicast_pkt", IfHCOutMulticastPkts, tags))
							} else {
								log.Println("W! if_out_multicast_pkt out of range, current:", ifStat.IfHCOutMulticastPkts, "lasttime:", lastifStat.IfHCOutMulticastPkts, "tags:", tags)
							}
						}
					}
				}
			}

			if ins.GatherDiscards {
				if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
					for _, lastifStat := range lastIfStatList {
						if ifStat.IfIndex == lastifStat.IfIndex {
							interval := ifStat.TS - lastifStat.TS

							IfInDiscards := (float64(ifStat.IfInDiscards) - float64(lastifStat.IfInDiscards)) / float64(interval)
							IfOutDiscards := (float64(ifStat.IfOutDiscards) - float64(lastifStat.IfOutDiscards)) / float64(interval)

							if limitCheck(IfInDiscards, ins.DiscardsPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_in_discards", IfInDiscards, tags))
							} else {
								log.Println("W! if_in_discards out of range, current:", ifStat.IfInDiscards, "lasttime:", lastifStat.IfInDiscards, "tags:", tags)
							}

							if limitCheck(IfOutDiscards, ins.DiscardsPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_out_discards", IfOutDiscards, tags))
							} else {
								log.Println("W! if_out_discards out of range, current:", ifStat.IfOutDiscards, "lasttime:", lastifStat.IfOutDiscards, "tags:", tags)
							}
						}
					}
				}
			}

			if ins.GatherErrors {
				if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
					for _, lastifStat := range lastIfStatList {
						if ifStat.IfIndex == lastifStat.IfIndex {
							interval := ifStat.TS - lastifStat.TS

							IfInErrors := (float64(ifStat.IfInErrors) - float64(lastifStat.IfInErrors)) / float64(interval)
							IfOutErrors := (float64(ifStat.IfOutErrors) - float64(lastifStat.IfOutErrors)) / float64(interval)

							if limitCheck(IfInErrors, ins.ErrorsPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_in_errors", IfInErrors, tags))
							} else {
								log.Println("W! if_in_errors out of range, current:", ifStat.IfInErrors, "lasttime:", lastifStat.IfInErrors, "tags:", tags)
							}

							if limitCheck(IfOutErrors, ins.ErrorsPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_out_errors", IfOutErrors, tags))
							} else {
								log.Println("W! if_out_errors out of range, current:", ifStat.IfOutErrors, "lasttime:", lastifStat.IfOutErrors, "tags:", tags)
							}
						}
					}
				}
			}

			if ins.GatherUnknownProtos {
				if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
					for _, lastifStat := range lastIfStatList {
						if ifStat.IfIndex == lastifStat.IfIndex {
							interval := ifStat.TS - lastifStat.TS
							IfInUnknownProtos := (float64(ifStat.IfInUnknownProtos) - float64(lastifStat.IfInUnknownProtos)) / float64(interval)
							if limitCheck(IfInUnknownProtos, ins.UnknownProtosPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_in_unknown_protos", IfInUnknownProtos, tags))
							} else {
								log.Println("W! if_in_unknown_protos out of range, current:", ifStat.IfInUnknownProtos, "lasttime:", lastifStat.IfInUnknownProtos, "tags:", tags)
							}
						}
					}
				}
			}

			if ins.GatherOutQlen {
				if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
					for _, lastifStat := range lastIfStatList {
						if ifStat.IfIndex == lastifStat.IfIndex {
							interval := ifStat.TS - lastifStat.TS
							IfOutQLen := (float64(ifStat.IfOutQLen) - float64(lastifStat.IfOutQLen)) / float64(interval)
							if limitCheck(IfOutQLen, ins.OutQlenPktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_out_qlen", IfOutQLen, tags))
							} else {
								log.Println("W! if_out_qlen out of range, current:", ifStat.IfOutQLen, "lasttime:", lastifStat.IfOutQLen, "tags:", tags)
							}
						}
					}
				}
			}

			if ins.GatherPkt {
				if lastIfStatList := ins.lastifmap.Get(ip); lastIfStatList != nil {
					for _, lastifStat := range lastIfStatList {
						if ifStat.IfIndex == lastifStat.IfIndex {
							interval := ifStat.TS - lastifStat.TS

							IfHCInUcastPkts := (float64(ifStat.IfHCInUcastPkts) - float64(lastifStat.IfHCInUcastPkts)) / float64(interval)
							IfHCOutUcastPkts := (float64(ifStat.IfHCOutUcastPkts) - float64(lastifStat.IfHCOutUcastPkts)) / float64(interval)

							if limitCheck(IfHCInUcastPkts, ins.PktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_in_pkts", IfHCInUcastPkts, tags))
							} else {
								log.Println("W! if_in_pkts out of range, current:", ifStat.IfHCInUcastPkts, "lasttime:", lastifStat.IfHCInUcastPkts, "tags:", tags)
							}

							if limitCheck(IfHCOutUcastPkts, ins.PktLimit) {
								slist.PushFront(types.NewSample(inputName, "if_out_pkts", IfHCOutUcastPkts, tags))
							} else {
								log.Println("W! if_out_pkts out of range, current:", ifStat.IfHCOutUcastPkts, "lasttime:", lastifStat.IfHCOutUcastPkts, "tags:", tags)
							}
						}
					}
				}
			}
		}
		ins.lastifmap.Set(ip, stats)
	}
}

func (ins *Instance) ifstat(wg *sync.WaitGroup, sema *semaphore.Semaphore, ip string, result cmap.ConcurrentMap) {
	defer func() {
		sema.Release()
		wg.Done()
	}()

	var (
		ifList []sw.IfStats
		err    error
		start  = time.Now()
	)

	if ins.SnmpModeGosnmp {
		ifList, err = sw.ListIfStats(ip, ins.Community, int(ins.SnmpTimeoutMs), ins.IgnoreIfaces, ins.SnmpRetries, ins.ConcurrencyForRequest, !ins.GatherPkt, !ins.GatherOperStatus, !ins.GatherBroadcastPkt, !ins.GatherMulticastPkt, !ins.GatherDiscards, !ins.GatherErrors, !ins.GatherUnknownProtos, !ins.GatherOutQlen)
	} else {
		ifList, err = sw.ListIfStatsSnmpWalk(ip, ins.Community, int(ins.SnmpTimeoutMs)*5, ins.IgnoreIfaces, ins.SnmpRetries, !ins.GatherPkt, !ins.GatherOperStatus, !ins.GatherBroadcastPkt, !ins.GatherMulticastPkt, !ins.GatherDiscards, !ins.GatherErrors, !ins.GatherUnknownProtos, !ins.GatherOutQlen)
	}

	if config.Config.DebugMode {
		log.Println("D! switch gather ifstat, ip:", ip, "use:", time.Since(start))
	}

	if err != nil {
		log.Println("E! failed to gather ifstat, ip:", ip, "error:", err)
		return
	}

	if len(ifList) > 0 {
		result.Set(ip, &ChIfStat{
			IP:          ip,
			IfStatsList: ifList,
			UseTime:     time.Since(start),
		})
	}
}

func (ins *Instance) gatherPing(ips []string, slist *types.SampleList) []string {
	pingResult := cmap.New()
	for i := 0; i < len(ips); i++ {
		// init ping result
		pingResult.Set(ips[i], false)
	}

	wg := new(sync.WaitGroup)
	se := semaphore.NewSemaphore(ins.ConcurrencyForAddress)
	for i := 0; i < len(ips); i++ {
		ip := ips[i]
		wg.Add(1)
		se.Acquire()
		go ins.ping(wg, se, ip, pingResult)
	}
	wg.Wait()

	ips = make([]string, 0, len(ips))

	for ip, succ := range pingResult.Items() {
		val := 0
		if succ.(bool) {
			val = 1
			ips = append(ips, ip)
		}

		if ins.GatherPingMetrics {
			slist.PushFront(types.NewSample(inputName, "ping_up", val, map[string]string{ins.parent.SwitchIdLabel: ins.parent.MappingIP(ip)}))
		}
	}

	log.Println("I! switch alive ip count:", len(ips))
	return ips
}

func (ins *Instance) parseIPs() (lst []string) {
	for i := 0; i < len(ins.IPs); i++ {
		item := ins.IPs[i]

		aip := sw.ParseIp(item)
		lst = append(lst, aip...)
	}

	return
}

func (ins *Instance) ping(wg *sync.WaitGroup, sema *semaphore.Semaphore, ip string, result cmap.ConcurrentMap) {
	defer func() {
		sema.Release()
		wg.Done()
	}()

	for i := 0; i < ins.PingRetries; i++ {
		succ := sw.Ping(ip, int(ins.PingTimeoutMs), ins.PingModeFastping)
		if succ {
			result.Set(ip, succ)
			break
		}
	}
}

func limitCheck(value float64, limit float64) bool {
	if value < 0 {
		return false
	}
	if limit > 0 {
		if value > limit {
			return false
		}
	}
	return true
}
