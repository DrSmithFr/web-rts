package proto

import (
	"bytes"
	"errors"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		o    Order
	}{
		{"noop", Order{Tick: 0, Player: 0, Kind: KindNoop}},
		{"noop max tick", Order{Tick: 0xffffffff, Player: 255, Kind: KindNoop}},
		{"payload", Order{Tick: 12, Player: 3, Kind: KindNoop, Payload: []byte{1, 2, 3, 4, 5}}},
		{"boundary payload", Order{Tick: 1, Player: 7, Kind: KindNoop, Payload: bytes.Repeat([]byte{0xAB}, MaxPayload)}},
	}
	for _, tt := range tests {
		buf := tt.o.EncodeAppend(nil)
		if len(buf) != tt.o.Size() {
			t.Errorf("%s: Size() = %d, encoded %d", tt.name, tt.o.Size(), len(buf))
		}
		got, used, err := Decode(buf)
		if err != nil {
			t.Fatalf("%s: Decode: %v", tt.name, err)
		}
		if used != len(buf) {
			t.Errorf("%s: used = %d, want %d", tt.name, used, len(buf))
		}
		if got.Tick != tt.o.Tick || got.Player != tt.o.Player || got.Kind != tt.o.Kind {
			t.Errorf("%s: header mismatch: %+v", tt.name, got)
		}
		if !bytes.Equal(got.Payload, tt.o.Payload) {
			t.Errorf("%s: payload mismatch", tt.name)
		}
	}
}

func TestDecodeConcatenated(t *testing.T) {
	var buf []byte
	a := Order{Tick: 1, Player: 0, Kind: KindNoop, Payload: []byte{0xFF}}
	b := Order{Tick: 2, Player: 1, Kind: KindNoop}
	buf = a.EncodeAppend(buf)
	buf = b.EncodeAppend(buf)

	got, used, err := Decode(buf)
	if err != nil || got.Tick != 1 || !bytes.Equal(got.Payload, []byte{0xFF}) {
		t.Fatalf("first order: %+v used=%d err=%v", got, used, err)
	}
	got, used2, err := Decode(buf[used:])
	if err != nil || got.Tick != 2 || used+used2 != len(buf) {
		t.Fatalf("second order: %+v used=%d err=%v", got, used2, err)
	}
}

func TestDecodeErrors(t *testing.T) {
	valid := Order{Tick: 5, Player: 1, Kind: KindNoop, Payload: []byte{9, 9}}.EncodeAppend(nil)

	if _, _, err := Decode(nil); !errors.Is(err, ErrShort) {
		t.Errorf("empty: want ErrShort, got %v", err)
	}
	if _, _, err := Decode(valid[:3]); !errors.Is(err, ErrShort) {
		t.Errorf("truncated header: want ErrShort, got %v", err)
	}
	if _, _, err := Decode(valid[:len(valid)-1]); !errors.Is(err, ErrShort) {
		t.Errorf("truncated payload: want ErrShort, got %v", err)
	}
	truncatedVarint := append([]byte{0, 0, 0, 0, 0, 0, 0}, 0x80)
	if _, _, err := Decode(truncatedVarint); !errors.Is(err, ErrShort) {
		t.Errorf("truncated varint: want ErrShort, got %v", err)
	}
	hugeLen := append([]byte{0, 0, 0, 0, 0, 0, 0}, 0xff, 0xff, 0x3f) // 0x7fffff > MaxPayload
	if _, _, err := Decode(hugeLen); !errors.Is(err, ErrTooBig) {
		t.Errorf("huge length: want ErrTooBig, got %v", err)
	}
}
