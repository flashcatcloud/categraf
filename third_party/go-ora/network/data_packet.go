package network

import (
	"bytes"
	"encoding/binary"
	"errors"
)

type DataPacket struct {
	Packet

	dataFlag uint16
	buffer   []byte
}

func (pck *DataPacket) bytes() []byte {
	output := pck.Packet.bytes()
	binary.BigEndian.PutUint16(output[8:], pck.dataFlag)
	ret := bytes.NewBuffer(output)
	if len(pck.buffer) > 0 {
		ret.Write(pck.buffer)
	}
	return ret.Bytes()
}

func newDataPacket(initialData []byte, sessionCtx *SessionContext) (*DataPacket, error) {
	//var outputData []byte = initialData
	var err error
	if sessionCtx.AdvancedService.HashAlgo != nil {
		hashData := sessionCtx.AdvancedService.HashAlgo.Compute(initialData)
		initialData = append(initialData, hashData...)
	}
	if sessionCtx.AdvancedService.CryptAlgo != nil {
		//outputData = make([]byte, len(outputData))
		//copy(outputData, outputData)
		tracer := sessionCtx.ConnOption.Tracer
		tracer.LogPacket("Write packet (Decrypted): ", initialData)
		initialData, err = sessionCtx.AdvancedService.CryptAlgo.Encrypt(initialData)
		if err != nil {
			return nil, err
		}
	}
	if sessionCtx.AdvancedService.HashAlgo != nil || sessionCtx.AdvancedService.CryptAlgo != nil {
		foldingKey := uint8(0)
		initialData = append(initialData, foldingKey)
	}

	return &DataPacket{
		Packet: Packet{
			sessionCtx: sessionCtx,
			dataOffset: 0xA,
			length:     uint32(len(initialData)) + 0xA,
			packetType: DATA,
			flag:       0,
		},
		dataFlag: 0,
		buffer:   initialData,
	}, nil
}

func newDataPacketFromData(packetData []byte, sessionCtx *SessionContext) (*DataPacket, error) {
	if len(packetData) < 0xA || PacketType(packetData[4]) != DATA {
		return nil, errors.New("Not data packet")
	}
	pck := &DataPacket{
		Packet: Packet{
			sessionCtx: sessionCtx,
			dataOffset: 0xA,
			//length:     binary.BigEndian.Uint16(packetData),
			packetType: PacketType(packetData[4]),
			flag:       packetData[5],
		},
		dataFlag: binary.BigEndian.Uint16(packetData[8:]),
		buffer:   packetData[10:],
	}
	if sessionCtx.handshakeComplete && sessionCtx.Version >= 315 {
		pck.length = binary.BigEndian.Uint32(packetData)
	} else {
		pck.length = uint32(binary.BigEndian.Uint16(packetData))
	}
	var err error
	if sessionCtx.AdvancedService.CryptAlgo != nil || sessionCtx.AdvancedService.HashAlgo != nil {
		pck.buffer = pck.buffer[:len(pck.buffer)-1]
	}
	if sessionCtx.AdvancedService.CryptAlgo != nil {
		pck.buffer, err = sessionCtx.AdvancedService.CryptAlgo.Decrypt(pck.buffer)
		if err != nil {
			return nil, err
		}
		tracer := sessionCtx.ConnOption.Tracer
		tracer.LogPacket("Read packet (Decrypted): ", pck.buffer)
	}
	if sessionCtx.AdvancedService.HashAlgo != nil {
		pck.buffer, err = sessionCtx.AdvancedService.HashAlgo.Validate(pck.buffer)
		if err != nil {
			return nil, err
		}
	}
	return pck, nil
}

//func (pck *DataPacket) Data() []byte {
//	return pck.buffer
//}
