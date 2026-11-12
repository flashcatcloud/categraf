package network

import "encoding/binary"

const (
	marker_type_break     = 1
	marker_type_reset     = 2
	marker_type_interrupt = 3
)

type MarkerPacket struct {
	Packet
	markerType uint8
}

func (pck *MarkerPacket) bytes() []byte {
	if pck.sessionCtx.handshakeComplete && pck.sessionCtx.Version >= 315 {
		return []byte{0, 0x0, 0, 0xB, 0xC, 0, 0, 0, 1, 0, pck.markerType}
	} else {
		return []byte{0, 0xB, 0, 0, 0xC, 0, 0, 0, 1, 0, pck.markerType}
	}
}

func newMarkerPacket(markerType uint8, sessionCtx *SessionContext) *MarkerPacket {
	return &MarkerPacket{
		Packet: Packet{
			sessionCtx: sessionCtx,
			dataOffset: 0,
			length:     0xB,
			packetType: MARKER,
			flag:       0x20,
		},
		markerType: markerType,
	}
}
func newMarkerPacketFromData(packetData []byte, sessionCtx *SessionContext) *MarkerPacket {
	if len(packetData) != 0xB {
		return nil
	}
	pck := MarkerPacket{
		Packet: Packet{
			sessionCtx: sessionCtx,
			dataOffset: 0,
			packetType: PacketType(packetData[4]),
			flag:       packetData[5],
		},
		markerType: packetData[10],
	}
	if sessionCtx.handshakeComplete && sessionCtx.Version >= 315 {
		pck.length = binary.BigEndian.Uint32(packetData)
	} else {
		pck.length = uint32(binary.BigEndian.Uint16(packetData))
	}
	if pck.packetType != MARKER {
		return nil
	}
	return &pck
}
