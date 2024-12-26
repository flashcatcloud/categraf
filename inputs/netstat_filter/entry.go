package netstat

import "net"

// Entry holds the information of a /proc/net/* entry.
// For example, /proc/net/tcp:
// sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
// 0:  0100007F:13AD 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 18083222
type Entry struct {
	Proto   string
	SrcIP   net.IP
	SrcPort uint
	DstIP   net.IP
	DstPort uint
	Txq     uint
	Rxq     uint
	UserId  int
	INode   int
}

// NewEntry creates a new entry with values from /proc/net/
func NewEntry(proto string, srcIP net.IP, srcPort uint, dstIP net.IP, dstPort uint, txq uint, rxq uint, userId int, iNode int) Entry {
	return Entry{
		Proto:   proto,
		SrcIP:   srcIP,
		SrcPort: srcPort,
		DstIP:   dstIP,
		DstPort: dstPort,
		Txq:     txq,
		Rxq:     rxq,
		UserId:  userId,
		INode:   iNode,
	}
}
