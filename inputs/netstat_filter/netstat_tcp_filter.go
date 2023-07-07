//go:build linux
// +build linux

package netstat

import (
	"fmt"
	"net"
)

func FilterEntries(entries []Entry, srcIP string, srcPort uint32, dstIP string, dstPort uint32) map[string]struct {
	Txq int
	Rxq int
} {
	result := make(map[string]struct {
		Txq int
		Rxq int
	})

	for _, entry := range entries {
		// 判断源IP、源端口、目标IP和目标端口是否与传入参数匹配
		if (len(srcIP) == 0 || entry.SrcIP.Equal(net.ParseIP(srcIP))) &&
			(srcPort == 0 || entry.SrcPort == uint(srcPort)) &&
			(len(dstIP) == 0 || entry.DstIP.Equal(net.ParseIP(dstIP))) &&
			(dstPort == 0 || entry.DstPort == uint(dstPort)) {
			// 构建匹配条件的唯一键
			key := fmt.Sprintf("%s-%d-%s-%d", srcIP, srcPort, dstIP, dstPort)
			// 获取临时变量，对其字段进行修改
			temp := result[key]
			temp.Txq += int(entry.Txq)
			temp.Rxq += int(entry.Rxq)
			// 将修改后的临时变量重新赋值给 result[key]
			result[key] = temp
		}
	}
	return result
}
