package rfc4757

import "encoding/binary"

// UsageToMSMsgType converts Kerberos key usage numbers to Microsoft message type encoded as a little-endian four byte slice.
func UsageToMSMsgType(usage uint32) []byte {
	// Translate usage numbers to the Microsoft T numbers
	switch usage {
	case 3:
		usage = 8
	case 9:
		usage = 8
	case 23:
		usage = 13
	}
	// Now convert to bytes
	tb := make([]byte, 4)
	binary.LittleEndian.PutUint32(tb, usage)
	return tb
}
