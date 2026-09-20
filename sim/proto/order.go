// Package proto defines the compact binary wire format shared by clients,
// server and replays. Everything crossing the lockstep boundary is encoded
// here: little-endian fixed fields plus a uvarint payload length. No JSON
// exists in the game loop.
package proto

import (
	"encoding/binary"
	"errors"
)

// Kind identifies the order semantics. Values are append-only; never reuse
// one so old replays keep decoding.
type Kind uint16

const (
	KindNoop Kind = 0

	// Reserved for upcoming phases: move=1, attack=2, build=3, patrol=4,
	// stop=5, queue=6, advanceEra=7.
)

const (
	// HeaderSize is tick(4) + player(1) + kind(2).
	HeaderSize = 7
	// MaxPayload bounds a single order payload for network hygiene.
	MaxPayload = 1024
)

// Errors returned by Decode.
var (
	ErrShort  = errors.New("proto: buffer shorter than order")
	ErrTooBig = errors.New("proto: payload too large")
)

// Order is one player command scheduled by the lockstep layer.
type Order struct {
	Tick    uint32
	Player  uint8
	Kind    Kind
	Payload []byte
}

// EncodeAppend appends the wire representation of o to dst.
func (o Order) EncodeAppend(dst []byte) []byte {
	var hdr [HeaderSize]byte
	binary.LittleEndian.PutUint32(hdr[0:], o.Tick)
	hdr[4] = o.Player
	binary.LittleEndian.PutUint16(hdr[5:], uint16(o.Kind))
	dst = append(dst, hdr[:]...)
	dst = binary.AppendUvarint(dst, uint64(len(o.Payload)))
	return append(dst, o.Payload...)
}

// Size returns the encoded length of o.
func (o Order) Size() int {
	return HeaderSize + uvarintLen(uint64(len(o.Payload))) + len(o.Payload)
}

func uvarintLen(v uint64) int {
	n := 1
	for v >= 0x80 {
		v >>= 7
		n++
	}
	return n
}

// Decode parses a single order from src and reports the consumed byte
// count. An empty payload decodes to a nil slice.
func Decode(src []byte) (Order, int, error) {
	var o Order
	if len(src) < HeaderSize {
		return o, 0, ErrShort
	}
	o.Tick = binary.LittleEndian.Uint32(src[0:])
	o.Player = src[4]
	o.Kind = Kind(binary.LittleEndian.Uint16(src[5:]))
	n, used := binary.Uvarint(src[HeaderSize:])
	if used == 0 {
		return o, 0, ErrShort
	}
	if used < 0 || n > MaxPayload {
		return o, 0, ErrTooBig
	}
	total := HeaderSize + used + int(n)
	if len(src) < total {
		return o, 0, ErrShort
	}
	if n > 0 {
		o.Payload = src[HeaderSize+used : total]
	}
	return o, total, nil
}
