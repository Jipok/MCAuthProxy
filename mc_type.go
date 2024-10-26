// Based on https://github.com/realDragonium/Ultraviolet/blob/main/mc/type.go
package main

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrMcVarIntSize = errors.New("McVarInt is too big")
)

// A Field is both FieldEncoder and FieldDecoder
type Field interface {
	FieldEncoder
	FieldDecoder
}

// A FieldEncoder can be encode as minecraft protocol used.
type FieldEncoder interface {
	Encode() []byte
}

// A FieldDecoder can Decode from minecraft protocol
type FieldDecoder interface {
	Decode(r DecodeReader) error
}

// DecodeReader is both io.Reader and io.McByteReader
type DecodeReader interface {
	io.ByteReader
	io.Reader
}

type (
	// McByte is signed 8-bit integer, two's complement
	McByte int8
	// McUnsignedShort is unsigned 16-bit integer
	McUnsignedShort uint16
	// McLong is signed 64-bit integer, two's complement
	McLong int64
	// McString is sequence of Unicode scalar values with a max length of 32767
	McString string
	// McChat is encoded as a McString with max length of 262144.
	McChat = McString
	// McVarInt is variable-length data encoding a two's complement signed 32-bit integer
	McVarInt int32
	// UUID is encoded as an unsigned 128-bit integer
	McUUID [16]byte
)

// ReadNMcBytes read N bytes from bytes.Reader
func ReadNBytes(r DecodeReader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return buf, err
	}
	return buf, nil
}

///////////////////////////////////////////////////////////////////////////////

// Encode a McString
func (s McString) Encode() []byte {
	byteMcString := []byte(s)
	var bb []byte
	bb = append(bb, McVarInt(len(byteMcString)).Encode()...) // len
	bb = append(bb, byteMcString...)                         // data
	return bb
}

// Decode a McString
func (s *McString) Decode(r DecodeReader) error {
	var l McVarInt // McString length
	if err := l.Decode(r); err != nil {
		return err
	}

	bb, err := ReadNBytes(r, int(l))
	if err != nil {
		return err
	}

	*s = McString(bb)
	return nil
}

///////////////////////////////////////////////////////////////////////////////

// Encode a McByte
func (b McByte) Encode() []byte {
	return []byte{byte(b)}
}

// Decode a McByte
func (b *McByte) Decode(r DecodeReader) error {
	v, err := r.ReadByte()
	if err != nil {
		return err
	}
	*b = McByte(v)
	return nil
}

///////////////////////////////////////////////////////////////////////////////

// Encode a Unsigned Short
func (us McUnsignedShort) Encode() []byte {
	n := uint16(us)
	return []byte{
		byte(n >> 8),
		byte(n),
	}
}

// Decode a McUnsignedShort
func (us *McUnsignedShort) Decode(r DecodeReader) error {
	bb, err := ReadNBytes(r, 2)
	if err != nil {
		return err
	}

	*us = McUnsignedShort(int16(bb[0])<<8 | int16(bb[1]))
	return nil
}

///////////////////////////////////////////////////////////////////////////////

// Encode a McLong
func (l McLong) Encode() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(l))
	return buf
}

// Decode a McLong
func (l *McLong) Decode(r DecodeReader) error {
	buf, err := ReadNBytes(r, 8)
	if err != nil {
		return err
	}
	*l = McLong(binary.BigEndian.Uint64(buf))
	return nil
}

///////////////////////////////////////////////////////////////////////////////

// Encode a McVarInt
func (v McVarInt) Encode() []byte {
	num := uint32(v)
	var bb []byte
	for {
		b := num & 0x7F
		num >>= 7
		if num != 0 {
			b |= 0x80
		}
		bb = append(bb, byte(b))
		if num == 0 {
			break
		}
	}
	return bb
}

// Decode a McVarInt
func (v *McVarInt) Decode(r DecodeReader) error {
	var n uint32
	for i := 0; ; i++ {
		sec, err := r.ReadByte()
		if err != nil {
			return err
		}

		n |= uint32(sec&0x7F) << uint32(7*i)

		if i >= 5 {
			return ErrMcVarIntSize
		} else if sec&0x80 == 0 {
			break
		}
	}

	*v = McVarInt(n)
	return nil
}

///////////////////////////////////////////////////////////////////////////////

func (u McUUID) Encode() []byte {
	return u[:]
}

// Decode читает 16 байт из reader и устанавливает значение UInt128.
func (u *McUUID) Decode(r DecodeReader) error {
	_, err := io.ReadFull(r, u[:])
	return err
}
