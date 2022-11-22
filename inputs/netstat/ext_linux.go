//go:build linux
// +build linux

package netstat

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

// Copyright 2022 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

func (p Proc) Netstat() (*ProcNetstat, error) {
	filename := p.path("net/netstat")
	data, err := file.ReadBytes(filename)
	if err != nil {
		return nil, err
	}
	procNetstat, err := parseNetstat(bytes.NewReader(data), filename)
	procNetstat.PID = p.PID
	return &procNetstat, err
}

// parseNetstat parses the metrics from proc/<pid>/net/netstat file
// and returns a ProcNetstat structure.
func parseNetstat(r io.Reader, fileName string) (ProcNetstat, error) {
	var (
		scanner     = bufio.NewScanner(r)
		procNetstat = ProcNetstat{}
	)

	for scanner.Scan() {
		nameParts := strings.Split(scanner.Text(), " ")
		scanner.Scan()
		valueParts := strings.Split(scanner.Text(), " ")
		// Remove trailing :.
		protocol := strings.TrimSuffix(nameParts[0], ":")
		if len(nameParts) != len(valueParts) {
			return procNetstat, fmt.Errorf("mismatch field count mismatch in %s: %s",
				fileName, protocol)
		}
		for i := 1; i < len(nameParts); i++ {
			value, err := strconv.ParseFloat(valueParts[i], 64)
			if err != nil {
				return procNetstat, err
			}
			key := nameParts[i]

			switch protocol {
			case "TcpExt":
				switch key {
				case "SyncookiesSent":
					procNetstat.TcpExt.SyncookiesSent = &value
				case "SyncookiesRecv":
					procNetstat.TcpExt.SyncookiesRecv = &value
				case "SyncookiesFailed":
					procNetstat.TcpExt.SyncookiesFailed = &value
				case "EmbryonicRsts":
					procNetstat.TcpExt.EmbryonicRsts = &value
				case "PruneCalled":
					procNetstat.TcpExt.PruneCalled = &value
				case "RcvPruned":
					procNetstat.TcpExt.RcvPruned = &value
				case "OfoPruned":
					procNetstat.TcpExt.OfoPruned = &value
				case "OutOfWindowIcmps":
					procNetstat.TcpExt.OutOfWindowIcmps = &value
				case "LockDroppedIcmps":
					procNetstat.TcpExt.LockDroppedIcmps = &value
				case "ArpFilter":
					procNetstat.TcpExt.ArpFilter = &value
				case "TW":
					procNetstat.TcpExt.TW = &value
				case "TWRecycled":
					procNetstat.TcpExt.TWRecycled = &value
				case "TWKilled":
					procNetstat.TcpExt.TWKilled = &value
				case "PAWSActive":
					procNetstat.TcpExt.PAWSActive = &value
				case "PAWSEstab":
					procNetstat.TcpExt.PAWSEstab = &value
				case "DelayedACKs":
					procNetstat.TcpExt.DelayedACKs = &value
				case "DelayedACKLocked":
					procNetstat.TcpExt.DelayedACKLocked = &value
				case "DelayedACKLost":
					procNetstat.TcpExt.DelayedACKLost = &value
				case "ListenOverflows":
					procNetstat.TcpExt.ListenOverflows = &value
				case "ListenDrops":
					procNetstat.TcpExt.ListenDrops = &value
				case "TCPHPHits":
					procNetstat.TcpExt.TCPHPHits = &value
				case "TCPPureAcks":
					procNetstat.TcpExt.TCPPureAcks = &value
				case "TCPHPAcks":
					procNetstat.TcpExt.TCPHPAcks = &value
				case "TCPRenoRecovery":
					procNetstat.TcpExt.TCPRenoRecovery = &value
				case "TCPSackRecovery":
					procNetstat.TcpExt.TCPSackRecovery = &value
				case "TCPSACKReneging":
					procNetstat.TcpExt.TCPSACKReneging = &value
				case "TCPSACKReorder":
					procNetstat.TcpExt.TCPSACKReorder = &value
				case "TCPRenoReorder":
					procNetstat.TcpExt.TCPRenoReorder = &value
				case "TCPTSReorder":
					procNetstat.TcpExt.TCPTSReorder = &value
				case "TCPFullUndo":
					procNetstat.TcpExt.TCPFullUndo = &value
				case "TCPPartialUndo":
					procNetstat.TcpExt.TCPPartialUndo = &value
				case "TCPDSACKUndo":
					procNetstat.TcpExt.TCPDSACKUndo = &value
				case "TCPLossUndo":
					procNetstat.TcpExt.TCPLossUndo = &value
				case "TCPLostRetransmit":
					procNetstat.TcpExt.TCPLostRetransmit = &value
				case "TCPRenoFailures":
					procNetstat.TcpExt.TCPRenoFailures = &value
				case "TCPSackFailures":
					procNetstat.TcpExt.TCPSackFailures = &value
				case "TCPLossFailures":
					procNetstat.TcpExt.TCPLossFailures = &value
				case "TCPFastRetrans":
					procNetstat.TcpExt.TCPFastRetrans = &value
				case "TCPSlowStartRetrans":
					procNetstat.TcpExt.TCPSlowStartRetrans = &value
				case "TCPTimeouts":
					procNetstat.TcpExt.TCPTimeouts = &value
				case "TCPLossProbes":
					procNetstat.TcpExt.TCPLossProbes = &value
				case "TCPLossProbeRecovery":
					procNetstat.TcpExt.TCPLossProbeRecovery = &value
				case "TCPRenoRecoveryFail":
					procNetstat.TcpExt.TCPRenoRecoveryFail = &value
				case "TCPSackRecoveryFail":
					procNetstat.TcpExt.TCPSackRecoveryFail = &value
				case "TCPRcvCollapsed":
					procNetstat.TcpExt.TCPRcvCollapsed = &value
				case "TCPDSACKOldSent":
					procNetstat.TcpExt.TCPDSACKOldSent = &value
				case "TCPDSACKOfoSent":
					procNetstat.TcpExt.TCPDSACKOfoSent = &value
				case "TCPDSACKRecv":
					procNetstat.TcpExt.TCPDSACKRecv = &value
				case "TCPDSACKOfoRecv":
					procNetstat.TcpExt.TCPDSACKOfoRecv = &value
				case "TCPAbortOnData":
					procNetstat.TcpExt.TCPAbortOnData = &value
				case "TCPAbortOnClose":
					procNetstat.TcpExt.TCPAbortOnClose = &value
				case "TCPDeferAcceptDrop":
					procNetstat.TcpExt.TCPDeferAcceptDrop = &value
				case "IPReversePathFilter":
					procNetstat.TcpExt.IPReversePathFilter = &value
				case "TCPTimeWaitOverflow":
					procNetstat.TcpExt.TCPTimeWaitOverflow = &value
				case "TCPReqQFullDoCookies":
					procNetstat.TcpExt.TCPReqQFullDoCookies = &value
				case "TCPReqQFullDrop":
					procNetstat.TcpExt.TCPReqQFullDrop = &value
				case "TCPRetransFail":
					procNetstat.TcpExt.TCPRetransFail = &value
				case "TCPRcvCoalesce":
					procNetstat.TcpExt.TCPRcvCoalesce = &value
				case "TCPOFOQueue":
					procNetstat.TcpExt.TCPOFOQueue = &value
				case "TCPOFODrop":
					procNetstat.TcpExt.TCPOFODrop = &value
				case "TCPOFOMerge":
					procNetstat.TcpExt.TCPOFOMerge = &value
				case "TCPChallengeACK":
					procNetstat.TcpExt.TCPChallengeACK = &value
				case "TCPSYNChallenge":
					procNetstat.TcpExt.TCPSYNChallenge = &value
				case "TCPFastOpenActive":
					procNetstat.TcpExt.TCPFastOpenActive = &value
				case "TCPFastOpenActiveFail":
					procNetstat.TcpExt.TCPFastOpenActiveFail = &value
				case "TCPFastOpenPassive":
					procNetstat.TcpExt.TCPFastOpenPassive = &value
				case "TCPFastOpenPassiveFail":
					procNetstat.TcpExt.TCPFastOpenPassiveFail = &value
				case "TCPFastOpenListenOverflow":
					procNetstat.TcpExt.TCPFastOpenListenOverflow = &value
				case "TCPFastOpenCookieReqd":
					procNetstat.TcpExt.TCPFastOpenCookieReqd = &value
				case "TCPFastOpenBlackhole":
					procNetstat.TcpExt.TCPFastOpenBlackhole = &value
				case "TCPSpuriousRtxHostQueues":
					procNetstat.TcpExt.TCPSpuriousRtxHostQueues = &value
				case "BusyPollRxPackets":
					procNetstat.TcpExt.BusyPollRxPackets = &value
				case "TCPAutoCorking":
					procNetstat.TcpExt.TCPAutoCorking = &value
				case "TCPFromZeroWindowAdv":
					procNetstat.TcpExt.TCPFromZeroWindowAdv = &value
				case "TCPToZeroWindowAdv":
					procNetstat.TcpExt.TCPToZeroWindowAdv = &value
				case "TCPWantZeroWindowAdv":
					procNetstat.TcpExt.TCPWantZeroWindowAdv = &value
				case "TCPSynRetrans":
					procNetstat.TcpExt.TCPSynRetrans = &value
				case "TCPOrigDataSent":
					procNetstat.TcpExt.TCPOrigDataSent = &value
				case "TCPHystartTrainDetect":
					procNetstat.TcpExt.TCPHystartTrainDetect = &value
				case "TCPHystartTrainCwnd":
					procNetstat.TcpExt.TCPHystartTrainCwnd = &value
				case "TCPHystartDelayDetect":
					procNetstat.TcpExt.TCPHystartDelayDetect = &value
				case "TCPHystartDelayCwnd":
					procNetstat.TcpExt.TCPHystartDelayCwnd = &value
				case "TCPACKSkippedSynRecv":
					procNetstat.TcpExt.TCPACKSkippedSynRecv = &value
				case "TCPACKSkippedPAWS":
					procNetstat.TcpExt.TCPACKSkippedPAWS = &value
				case "TCPACKSkippedSeq":
					procNetstat.TcpExt.TCPACKSkippedSeq = &value
				case "TCPACKSkippedFinWait2":
					procNetstat.TcpExt.TCPACKSkippedFinWait2 = &value
				case "TCPACKSkippedTimeWait":
					procNetstat.TcpExt.TCPACKSkippedTimeWait = &value
				case "TCPACKSkippedChallenge":
					procNetstat.TcpExt.TCPACKSkippedChallenge = &value
				case "TCPWinProbe":
					procNetstat.TcpExt.TCPWinProbe = &value
				case "TCPKeepAlive":
					procNetstat.TcpExt.TCPKeepAlive = &value
				case "TCPMTUPFail":
					procNetstat.TcpExt.TCPMTUPFail = &value
				case "TCPMTUPSuccess":
					procNetstat.TcpExt.TCPMTUPSuccess = &value
				case "TCPWqueueTooBig":
					procNetstat.TcpExt.TCPWqueueTooBig = &value
				}
			case "IpExt":
				switch key {
				case "InNoRoutes":
					procNetstat.IpExt.InNoRoutes = &value
				case "InTruncatedPkts":
					procNetstat.IpExt.InTruncatedPkts = &value
				case "InMcastPkts":
					procNetstat.IpExt.InMcastPkts = &value
				case "OutMcastPkts":
					procNetstat.IpExt.OutMcastPkts = &value
				case "InBcastPkts":
					procNetstat.IpExt.InBcastPkts = &value
				case "OutBcastPkts":
					procNetstat.IpExt.OutBcastPkts = &value
				case "InOctets":
					procNetstat.IpExt.InOctets = &value
				case "OutOctets":
					procNetstat.IpExt.OutOctets = &value
				case "InMcastOctets":
					procNetstat.IpExt.InMcastOctets = &value
				case "OutMcastOctets":
					procNetstat.IpExt.OutMcastOctets = &value
				case "InBcastOctets":
					procNetstat.IpExt.InBcastOctets = &value
				case "OutBcastOctets":
					procNetstat.IpExt.OutBcastOctets = &value
				case "InCsumErrors":
					procNetstat.IpExt.InCsumErrors = &value
				case "InNoECTPkts":
					procNetstat.IpExt.InNoECTPkts = &value
				case "InECT1Pkts":
					procNetstat.IpExt.InECT1Pkts = &value
				case "InECT0Pkts":
					procNetstat.IpExt.InECT0Pkts = &value
				case "InCEPkts":
					procNetstat.IpExt.InCEPkts = &value
				case "ReasmOverlaps":
					procNetstat.IpExt.ReasmOverlaps = &value
				}
			}
		}
	}
	return procNetstat, scanner.Err()
}
