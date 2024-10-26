// Based on https://github.com/realDragonium/Ultraviolet/blob/main/mc/packet.go
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	MaxPacketSize = 2097151

	ServerBoundHandshakePacketID  byte = 0x00
	ServerBoundLoginStartPacketID byte = 0x00

	ForgeSeparator  = "\x00"
	RealIPSeparator = "///"
)

var (
	ErrInvalidPacketID = errors.New("invalid packet id")
	ErrPacketTooBig    = errors.New("packet contains too much data")
	ErrExtraData       = errors.New("packet contains extra unread data")
	ErrUnsupported     = errors.New("packet contains unsupported data")
)

// Packet is the raw representation of message that is send between the client and the server
type Packet struct {
	ID   byte
	Data []byte
}

// Scan decodes and copies the Packet data into the fields
func (pk Packet) Scan(fields ...FieldDecoder) (*bytes.Reader, error) {
	r := bytes.NewReader(pk.Data)
	for _, field := range fields {
		if err := field.Decode(r); err != nil {
			return r, err
		}
	}
	// if r.Len() > 0 {
	// 	return ErrExtraData
	// }
	return r, nil
}

// Marshal encodes the packet and all it's fields
func (pk *Packet) Encode() []byte {
	var packedData []byte
	data := []byte{pk.ID}
	data = append(data, pk.Data...)
	packetLength := McVarInt(int32(len(data))).Encode()
	packedData = append(packedData, packetLength...)

	return append(packedData, data...)
}

func ReadPacket(r DecodeReader) (Packet, error) {
	var packetLength McVarInt
	err := packetLength.Decode(r)
	if err != nil {
		return Packet{}, err
	}

	if packetLength < 1 {
		return Packet{}, fmt.Errorf("packet length too short")
	}

	data := make([]byte, packetLength)
	if _, err := io.ReadFull(r, data); err != nil {
		return Packet{}, fmt.Errorf("reading the content of the packet failed: %v", err)
	}

	return Packet{
		ID:   data[0],
		Data: data[1:],
	}, nil
}

///////////////////////////////////////////////////////////////////////////////

type ServerBoundHandshake struct {
	ProtocolVersion  McVarInt
	ServerRawAddress McString
	Address          string
	ServerPort       McUnsignedShort
	NextState        McVarInt
}

const (
	HandshakeStatus = 1
	HandshakeLogin  = 2
)

func (pk ServerBoundHandshake) ToPacket() *Packet {
	var packet = &Packet{}
	packet.ID = ServerBoundHandshakePacketID
	packet.Data = pk.ProtocolVersion.Encode()
	packet.Data = append(packet.Data, pk.ServerRawAddress.Encode()...)
	packet.Data = append(packet.Data, pk.ServerPort.Encode()...)
	packet.Data = append(packet.Data, pk.NextState.Encode()...)
	return packet
}

func DecodeServerBoundHandshake(packet Packet) (ServerBoundHandshake, error) {
	var pk ServerBoundHandshake
	if packet.ID != ServerBoundHandshakePacketID {
		return pk, ErrInvalidPacketID
	}

	_, err := packet.Scan(&pk.ProtocolVersion, &pk.ServerRawAddress, &pk.ServerPort, &pk.NextState)
	if err != nil {
		return pk, err
	}

	// Parse ServerAddress
	addr := string(pk.ServerRawAddress)
	addr = strings.Split(addr, ForgeSeparator)[0]
	addr = strings.Split(addr, RealIPSeparator)[0]
	pk.Address = addr

	return pk, nil
}

///////////////////////////////////////////////////////////////////////////////

type ServerLoginStart759 struct { // 1.19 - 1.19.2
	Nickname   McString
	HasSigData McByte
	HasUUID    McByte
	UUID       McUUID
}

func (pk ServerLoginStart759) ToPacket() *Packet {
	var packet = &Packet{}
	packet.ID = ServerBoundLoginStartPacketID
	packet.Data = pk.Nickname.Encode()
	packet.Data = append(packet.Data, pk.HasSigData.Encode()...)
	packet.Data = append(packet.Data, pk.HasUUID.Encode()...)
	if pk.HasUUID != 0 {
		packet.Data = append(packet.Data, pk.UUID.Encode()...)
	}
	return packet
}

func DecodeServerBoundLoginStart759(packet Packet) (ServerLoginStart759, error) {
	var pk ServerLoginStart759
	if packet.ID != ServerBoundLoginStartPacketID {
		return pk, ErrInvalidPacketID
	}

	reader, err := packet.Scan(&pk.Nickname)
	if err != nil {
		return pk, err
	}

	pk.HasSigData.Decode(reader)
	if pk.HasSigData != 0 {
		return pk, ErrUnsupported
	}

	pk.HasUUID.Decode(reader)
	if pk.HasUUID != 0 {
		pk.UUID.Decode(reader)
	}

	return pk, nil
}

///////////////////////////////////////////////////////////////////////////////

type ServerLoginStart764 struct { // 1.20.2 - last
	Nickname McString
	UUID     McUUID
}

func (pk ServerLoginStart764) ToPacket() *Packet {
	var packet = &Packet{}
	packet.ID = ServerBoundLoginStartPacketID
	packet.Data = pk.Nickname.Encode()
	packet.Data = append(packet.Data, pk.UUID.Encode()...)
	return packet
}

func DecodeServerBoundLoginStart764(packet Packet) (ServerLoginStart764, error) {
	var pk ServerLoginStart764
	if packet.ID != ServerBoundLoginStartPacketID {
		return pk, ErrInvalidPacketID
	}

	_, err := packet.Scan(&pk.Nickname, &pk.UUID)
	if err != nil {
		return pk, err
	}

	return pk, nil
}

///////////////////////////////////////////////////////////////////////////////

type StatusJSON struct {
	Version StatusVersionJSON `json:"version"`
	// Players     PlaydersJSON     `json:"players"`
	Description StatusDescriptionJSON `json:"description"`
	Favicon     string                `json:"favicon"`
}

type StatusVersionJSON struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type StatusDescriptionJSON struct {
	Text string `json:"text"`
}
