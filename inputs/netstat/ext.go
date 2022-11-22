package netstat

import (
	"path/filepath"
	"strconv"
)

// ProcNetstat models the content of /proc/<pid>/net/netstat.
type ProcNetstat struct {
	// The process ID.
	PID int
	TcpExt
	IpExt
}

// FS represents a pseudo-filesystem, normally /proc or /sys, which provides an
// interface to kernel data structures.
type FS string

// Path appends the given path elements to the filesystem path, adding separators
// as necessary.
func (fs FS) Path(p ...string) string {
	return filepath.Join(append([]string{string(fs)}, p...)...)
}

// Proc provides information about a running process.
type Proc struct {
	// The process ID.
	PID int

	fs FS
}

func (p Proc) path(pa ...string) string {
	if p.PID == 0 {
		return p.fs.Path(pa...)
	}
	return p.fs.Path(append([]string{strconv.Itoa(p.PID)}, pa...)...)
}

type TcpExt struct { // nolint:revive
	SyncookiesSent            *float64
	SyncookiesRecv            *float64
	SyncookiesFailed          *float64
	EmbryonicRsts             *float64
	PruneCalled               *float64
	RcvPruned                 *float64
	OfoPruned                 *float64
	OutOfWindowIcmps          *float64
	LockDroppedIcmps          *float64
	ArpFilter                 *float64
	TW                        *float64
	TWRecycled                *float64
	TWKilled                  *float64
	PAWSActive                *float64
	PAWSEstab                 *float64
	DelayedACKs               *float64
	DelayedACKLocked          *float64
	DelayedACKLost            *float64
	ListenOverflows           *float64
	ListenDrops               *float64
	TCPHPHits                 *float64
	TCPPureAcks               *float64
	TCPHPAcks                 *float64
	TCPRenoRecovery           *float64
	TCPSackRecovery           *float64
	TCPSACKReneging           *float64
	TCPSACKReorder            *float64
	TCPRenoReorder            *float64
	TCPTSReorder              *float64
	TCPFullUndo               *float64
	TCPPartialUndo            *float64
	TCPDSACKUndo              *float64
	TCPLossUndo               *float64
	TCPLostRetransmit         *float64
	TCPRenoFailures           *float64
	TCPSackFailures           *float64
	TCPLossFailures           *float64
	TCPFastRetrans            *float64
	TCPSlowStartRetrans       *float64
	TCPTimeouts               *float64
	TCPLossProbes             *float64
	TCPLossProbeRecovery      *float64
	TCPRenoRecoveryFail       *float64
	TCPSackRecoveryFail       *float64
	TCPRcvCollapsed           *float64
	TCPDSACKOldSent           *float64
	TCPDSACKOfoSent           *float64
	TCPDSACKRecv              *float64
	TCPDSACKOfoRecv           *float64
	TCPAbortOnData            *float64
	TCPAbortOnClose           *float64
	TCPDeferAcceptDrop        *float64
	IPReversePathFilter       *float64
	TCPTimeWaitOverflow       *float64
	TCPReqQFullDoCookies      *float64
	TCPReqQFullDrop           *float64
	TCPRetransFail            *float64
	TCPRcvCoalesce            *float64
	TCPOFOQueue               *float64
	TCPOFODrop                *float64
	TCPOFOMerge               *float64
	TCPChallengeACK           *float64
	TCPSYNChallenge           *float64
	TCPFastOpenActive         *float64
	TCPFastOpenActiveFail     *float64
	TCPFastOpenPassive        *float64
	TCPFastOpenPassiveFail    *float64
	TCPFastOpenListenOverflow *float64
	TCPFastOpenCookieReqd     *float64
	TCPFastOpenBlackhole      *float64
	TCPSpuriousRtxHostQueues  *float64
	BusyPollRxPackets         *float64
	TCPAutoCorking            *float64
	TCPFromZeroWindowAdv      *float64
	TCPToZeroWindowAdv        *float64
	TCPWantZeroWindowAdv      *float64
	TCPSynRetrans             *float64
	TCPOrigDataSent           *float64
	TCPHystartTrainDetect     *float64
	TCPHystartTrainCwnd       *float64
	TCPHystartDelayDetect     *float64
	TCPHystartDelayCwnd       *float64
	TCPACKSkippedSynRecv      *float64
	TCPACKSkippedPAWS         *float64
	TCPACKSkippedSeq          *float64
	TCPACKSkippedFinWait2     *float64
	TCPACKSkippedTimeWait     *float64
	TCPACKSkippedChallenge    *float64
	TCPWinProbe               *float64
	TCPKeepAlive              *float64
	TCPMTUPFail               *float64
	TCPMTUPSuccess            *float64
	TCPWqueueTooBig           *float64
}

type IpExt struct { // nolint:revive
	InNoRoutes      *float64
	InTruncatedPkts *float64
	InMcastPkts     *float64
	OutMcastPkts    *float64
	InBcastPkts     *float64
	OutBcastPkts    *float64
	InOctets        *float64
	OutOctets       *float64
	InMcastOctets   *float64
	OutMcastOctets  *float64
	InBcastOctets   *float64
	OutBcastOctets  *float64
	InCsumErrors    *float64
	InNoECTPkts     *float64
	InECT1Pkts      *float64
	InECT0Pkts      *float64
	InCEPkts        *float64
	ReasmOverlaps   *float64
}
