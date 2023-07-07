//go:build !linux
// +build !linux

package netstat

func FilterEntries(entries []Entry, srcIP string, srcPort uint32, dstIP string, dstPort uint32) map[string]struct {
	Txq int
	Rxq int
} {
	result := make(map[string]struct {
		Txq int
		Rxq int
	})
	return result
}
