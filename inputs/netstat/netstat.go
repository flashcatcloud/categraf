package netstat

import (
	"log"
	"syscall"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

const inputName = "netstat"

type NetStats struct {
	ps system.PS
	config.PluginConfig

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

func (s *NetStats) Gather(slist *types.SampleList) {
	s.gatherExt(slist)

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
		tcpFields := map[string]interface{}{
			"tcpext_SyncookiesSent":            *n.TcpExt.SyncookiesSent,
			"tcpext_SyncookiesRecv":            *n.TcpExt.SyncookiesRecv,
			"tcpext_SyncookiesFailed":          *n.TcpExt.SyncookiesFailed,
			"tcpext_EmbryonicRsts":             *n.TcpExt.EmbryonicRsts,
			"tcpext_PruneCalled":               *n.TcpExt.PruneCalled,
			"tcpext_RcvPruned":                 *n.TcpExt.RcvPruned,
			"tcpext_OfoPruned":                 *n.TcpExt.OfoPruned,
			"tcpext_OutOfWindowIcmps":          *n.TcpExt.OutOfWindowIcmps,
			"tcpext_LockDroppedIcmps":          *n.TcpExt.LockDroppedIcmps,
			"tcpext_ArpFilter":                 *n.TcpExt.ArpFilter,
			"tcpext_TW":                        *n.TcpExt.TW,
			"tcpext_TWRecycled":                *n.TcpExt.TWRecycled,
			"tcpext_TWKilled":                  *n.TcpExt.TWKilled,
			"tcpext_PAWSActive":                *n.TcpExt.PAWSActive,
			"tcpext_PAWSEstab":                 *n.TcpExt.PAWSEstab,
			"tcpext_DelayedACKs":               *n.TcpExt.DelayedACKs,
			"tcpext_DelayedACKLocked":          *n.TcpExt.DelayedACKLocked,
			"tcpext_DelayedACKLost":            *n.TcpExt.DelayedACKLost,
			"tcpext_ListenOverflows":           *n.TcpExt.ListenOverflows,
			"tcpext_ListenDrops":               *n.TcpExt.ListenDrops,
			"tcpext_TCPHPHits":                 *n.TcpExt.TCPHPHits,
			"tcpext_TCPPureAcks":               *n.TcpExt.TCPPureAcks,
			"tcpext_TCPHPAcks":                 *n.TcpExt.TCPHPAcks,
			"tcpext_TCPRenoRecovery":           *n.TcpExt.TCPRenoRecovery,
			"tcpext_TCPSackRecovery":           *n.TcpExt.TCPSackRecovery,
			"tcpext_TCPSACKReneging":           *n.TcpExt.TCPSACKReneging,
			"tcpext_TCPSACKReorder":            *n.TcpExt.TCPSACKReorder,
			"tcpext_TCPRenoReorder":            *n.TcpExt.TCPRenoReorder,
			"tcpext_TCPTSReorder":              *n.TcpExt.TCPTSReorder,
			"tcpext_TCPFullUndo":               *n.TcpExt.TCPFullUndo,
			"tcpext_TCPPartialUndo":            *n.TcpExt.TCPPartialUndo,
			"tcpext_TCPDSACKUndo":              *n.TcpExt.TCPDSACKUndo,
			"tcpext_TCPLossUndo":               *n.TcpExt.TCPLossUndo,
			"tcpext_TCPLostRetransmit":         *n.TcpExt.TCPLostRetransmit,
			"tcpext_TCPRenoFailures":           *n.TcpExt.TCPRenoFailures,
			"tcpext_TCPSackFailures":           *n.TcpExt.TCPSackFailures,
			"tcpext_TCPLossFailures":           *n.TcpExt.TCPLossFailures,
			"tcpext_TCPFastRetrans":            *n.TcpExt.TCPFastRetrans,
			"tcpext_TCPSlowStartRetrans":       *n.TcpExt.TCPSlowStartRetrans,
			"tcpext_TCPTimeouts":               *n.TcpExt.TCPTimeouts,
			"tcpext_TCPLossProbes":             *n.TcpExt.TCPLossProbes,
			"tcpext_TCPLossProbeRecovery":      *n.TcpExt.TCPLossProbeRecovery,
			"tcpext_TCPRenoRecoveryFail":       *n.TcpExt.TCPRenoRecoveryFail,
			"tcpext_TCPSackRecoveryFail":       *n.TcpExt.TCPSackRecoveryFail,
			"tcpext_TCPRcvCollapsed":           *n.TcpExt.TCPRcvCollapsed,
			"tcpext_TCPDSACKOldSent":           *n.TcpExt.TCPDSACKOldSent,
			"tcpext_TCPDSACKOfoSent":           *n.TcpExt.TCPDSACKOfoSent,
			"tcpext_TCPDSACKRecv":              *n.TcpExt.TCPDSACKRecv,
			"tcpext_TCPDSACKOfoRecv":           *n.TcpExt.TCPDSACKOfoRecv,
			"tcpext_TCPAbortOnData":            *n.TcpExt.TCPAbortOnData,
			"tcpext_TCPAbortOnClose":           *n.TcpExt.TCPAbortOnClose,
			"tcpext_TCPDeferAcceptDrop":        *n.TcpExt.TCPDeferAcceptDrop,
			"tcpext_IPReversePathFilter":       *n.TcpExt.IPReversePathFilter,
			"tcpext_TCPTimeWaitOverflow":       *n.TcpExt.TCPTimeWaitOverflow,
			"tcpext_TCPReqQFullDoCookies":      *n.TcpExt.TCPReqQFullDoCookies,
			"tcpext_TCPReqQFullDrop":           *n.TcpExt.TCPReqQFullDrop,
			"tcpext_TCPRetransFail":            *n.TcpExt.TCPRetransFail,
			"tcpext_TCPRcvCoalesce":            *n.TcpExt.TCPRcvCoalesce,
			"tcpext_TCPOFOQueue":               *n.TcpExt.TCPOFOQueue,
			"tcpext_TCPOFODrop":                *n.TcpExt.TCPOFODrop,
			"tcpext_TCPOFOMerge":               *n.TcpExt.TCPOFOMerge,
			"tcpext_TCPChallengeACK":           *n.TcpExt.TCPChallengeACK,
			"tcpext_TCPSYNChallenge":           *n.TcpExt.TCPSYNChallenge,
			"tcpext_TCPFastOpenActive":         *n.TcpExt.TCPFastOpenActive,
			"tcpext_TCPFastOpenActiveFail":     *n.TcpExt.TCPFastOpenActiveFail,
			"tcpext_TCPFastOpenPassive":        *n.TcpExt.TCPFastOpenPassive,
			"tcpext_TCPFastOpenPassiveFail":    *n.TcpExt.TCPFastOpenPassiveFail,
			"tcpext_TCPFastOpenListenOverflow": *n.TcpExt.TCPFastOpenListenOverflow,
			"tcpext_TCPFastOpenCookieReqd":     *n.TcpExt.TCPFastOpenCookieReqd,
			"tcpext_TCPFastOpenBlackhole":      *n.TcpExt.TCPFastOpenBlackhole,
			"tcpext_TCPSpuriousRtxHostQueues":  *n.TcpExt.TCPSpuriousRtxHostQueues,
			"tcpext_BusyPollRxPackets":         *n.TcpExt.BusyPollRxPackets,
			"tcpext_TCPAutoCorking":            *n.TcpExt.TCPAutoCorking,
			"tcpext_TCPFromZeroWindowAdv":      *n.TcpExt.TCPFromZeroWindowAdv,
			"tcpext_TCPToZeroWindowAdv":        *n.TcpExt.TCPToZeroWindowAdv,
			"tcpext_TCPWantZeroWindowAdv":      *n.TcpExt.TCPWantZeroWindowAdv,
			"tcpext_TCPSynRetrans":             *n.TcpExt.TCPSynRetrans,
			"tcpext_TCPOrigDataSent":           *n.TcpExt.TCPOrigDataSent,
			"tcpext_TCPHystartTrainDetect":     *n.TcpExt.TCPHystartTrainDetect,
			"tcpext_TCPHystartTrainCwnd":       *n.TcpExt.TCPHystartTrainCwnd,
			"tcpext_TCPHystartDelayDetect":     *n.TcpExt.TCPHystartDelayDetect,
			"tcpext_TCPHystartDelayCwnd":       *n.TcpExt.TCPHystartDelayCwnd,
			"tcpext_TCPACKSkippedSynRecv":      *n.TcpExt.TCPACKSkippedSynRecv,
			"tcpext_TCPACKSkippedPAWS":         *n.TcpExt.TCPACKSkippedPAWS,
			"tcpext_TCPACKSkippedSeq":          *n.TcpExt.TCPACKSkippedSeq,
			"tcpext_TCPACKSkippedFinWait2":     *n.TcpExt.TCPACKSkippedFinWait2,
			"tcpext_TCPACKSkippedTimeWait":     *n.TcpExt.TCPACKSkippedTimeWait,
			"tcpext_TCPACKSkippedChallenge":    *n.TcpExt.TCPACKSkippedChallenge,
			"tcpext_TCPWinProbe":               *n.TcpExt.TCPWinProbe,
			"tcpext_TCPKeepAlive":              *n.TcpExt.TCPKeepAlive,
			"tcpext_TCPMTUPFail":               *n.TcpExt.TCPMTUPFail,
			"tcpext_TCPMTUPSuccess":            *n.TcpExt.TCPMTUPSuccess,
			"tcpext_TCPWqueueTooBig":           *n.TcpExt.TCPWqueueTooBig,
		}
		slist.PushSamples(inputName, tcpFields, tags)
	}

	if s.IpExt {
		ipFields := map[string]interface{}{
			"ipext_InNoRoutes":      *n.IpExt.InNoRoutes,
			"ipext_InTruncatedPkts": *n.IpExt.InTruncatedPkts,
			"ipext_InMcastPkts":     *n.IpExt.InMcastPkts,
			"ipext_OutMcastPkts":    *n.IpExt.OutMcastPkts,
			"ipext_InBcastPkts":     *n.IpExt.InBcastPkts,
			"ipext_OutBcastPkts":    *n.IpExt.OutBcastPkts,
			"ipext_InOctets":        *n.IpExt.InOctets,
			"ipext_OutOctets":       *n.IpExt.OutOctets,
			"ipext_InMcastOctets":   *n.IpExt.InMcastOctets,
			"ipext_OutMcastOctets":  *n.IpExt.OutMcastOctets,
			"ipext_InBcastOctets":   *n.IpExt.InBcastOctets,
			"ipext_OutBcastOctets":  *n.IpExt.OutBcastOctets,
			"ipext_InCsumErrors":    *n.IpExt.InCsumErrors,
			"ipext_InNoECTPkts":     *n.IpExt.InNoECTPkts,
			"ipext_InECT1Pkts":      *n.IpExt.InECT1Pkts,
			"ipext_InECT0Pkts":      *n.IpExt.InECT0Pkts,
			"ipext_InCEPkts":        *n.IpExt.InCEPkts,
			"ipext_ReasmOverlaps":   *n.IpExt.ReasmOverlaps,
		}
		slist.PushSamples(inputName, ipFields, tags)
	}
}
