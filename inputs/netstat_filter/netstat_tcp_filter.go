//go:build linux

package netstat

import "net"

func FilterEntries(entries []Entry, srcIP string, srcPort uint32, dstIP string, dstPort uint32) (int, int) {
	var totalTxq, totalRxq int

	for _, entry := range entries {
		// 判断源IP、源端口、目标IP和目标端口是否与传入参数匹配
		if len(srcIP) == 0 || entry.SrcIP.Equal(net.ParseIP(srcIP)) && srcPort == 0 || entry.SrcPort == uint(srcPort) &&
			len(dstIP) == 0 || entry.DstIP.Equal(net.ParseIP(dstIP)) && dstPort == 0 || entry.DstPort == uint(dstPort) {
			totalTxq += int(entry.Txq) //发送结果和
			totalRxq += int(entry.Rxq) //接收结果和

		}
	}

	return totalTxq, totalRxq
}
