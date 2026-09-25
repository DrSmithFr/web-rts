package orders

import (
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestVectors cross-checks the shared vector file: ARGS must encode to HEX,
// and HEX must decode back to ARGS. The TypeScript client runs the same
// encode check against this file.
func TestVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/order_vectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			t.Fatalf("line %d: %q", i, line)
		}
		kind, args, want := parts[0], parts[1], parts[2]
		raw, derr := hex.DecodeString(want)
		if derr != nil {
			t.Fatalf("line %d: %v", i, derr)
		}
		got := encodeArgs(t, kind, args)
		if hex.EncodeToString(got) != want {
			t.Errorf("line %d encode: got %x want %s", i, got, want)
		}
		decodeArgs(t, kind, args, raw)
	}
}

func encodeArgs(t *testing.T, kind, args string) []byte {
	t.Helper()
	switch kind {
	case "move":
		var items []MoveItem
		if args != "" {
			for _, part := range strings.Split(args, ";") {
				id, xy, _ := strings.Cut(part, ":")
				x, y, _ := strings.Cut(xy, ",")
				items = append(items, MoveItem{ID: u32(t, id), X16: u16(t, x), Y16: u16(t, y)})
			}
		}
		return MovePayload(items)
	case "gather":
		a, b, _ := strings.Cut(args, ":")
		return GatherPayload(u32(t, a), u32(t, b))
	case "build":
		f := strings.Split(args, ":")
		return BuildPayload(u32(t, f[0]), u16(t, f[1]), u16(t, f[2]), u16(t, f[3]))
	case "stop":
		var ids []uint32
		for _, s := range strings.Split(args, ";") {
			ids = append(ids, u32(t, s))
		}
		return StopPayload(ids)
	case "train":
		f := strings.Split(args, ":")
		if len(f) == 2 {
			return TrainPayload(u32(t, f[0]), u16(t, f[1]))
		}
		return TrainPayload(u32(t, args))
	case "attack":
		idsPart, rest, _ := strings.Cut(args, ":")
		target, kind, _ := strings.Cut(rest, ":")
		var ids []uint32
		for _, s := range strings.Split(idsPart, ";") {
			ids = append(ids, u32(t, s))
		}
		return AttackPayload(ids, u32(t, target), uint8(u32(t, kind)))
	case "repair":
		idsPart, target, _ := strings.Cut(args, ":")
		var ids []uint32
		for _, s := range strings.Split(idsPart, ";") {
			ids = append(ids, u32(t, s))
		}
		return RepairPayload(ids, u32(t, target))
	case "patrol":
		idsxy, _, _ := strings.Cut(args, ":")
		var ids []uint32
		for _, s := range strings.Split(idsxy, ";") {
			ids = append(ids, u32(t, s))
		}
		xyPart := args[len(idsxy)+1:]
		xs, ys, _ := strings.Cut(xyPart, ",")
		return PatrolPayload(ids, u16(t, xs), u16(t, ys))
	case "era":
		return AgeUpPayload(uint8(u16(t, args)))
	case "research":
		return ResearchPayload(u16(t, args))
	case "move-group":
		idsxy, _, _ := strings.Cut(args, ":")
		var ids []uint32
		for _, s := range strings.Split(idsxy, ";") {
			ids = append(ids, u32(t, s))
		}
		xyPart := args[len(idsxy)+1:]
		xs, ys, _ := strings.Cut(xyPart, ",")
		return MoveGroupPayload(ids, u16(t, xs), u16(t, ys))
	}
	t.Fatalf("unknown kind %q", kind)
	return nil
}

func decodeArgs(t *testing.T, kind, args string, raw []byte) {
	t.Helper()
	r := NewReader(raw)
	switch kind {
	case "move":
		n := int(r.Uvarint())
		var items []MoveItem
		for i := 0; i < n; i++ {
			items = append(items, MoveItem{ID: uint32(r.Uvarint()), X16: r.U16(), Y16: r.U16()})
		}
		want := encodeArgs(t, "move", args)
		if got := MovePayload(items); hex.EncodeToString(got) != hex.EncodeToString(want) {
			t.Errorf("move roundtrip: %x vs %x", got, want)
		}
	case "gather":
		a, b, _ := strings.Cut(args, ":")
		if got := uint32(r.Uvarint()); got != u32(t, a) {
			t.Errorf("gather unit: got %d want %s", got, a)
		}
		if got := uint32(r.Uvarint()); got != u32(t, b) {
			t.Errorf("gather node: got %d want %s", got, b)
		}
	case "build":
		f := strings.Split(args, ":")
		if uint32(r.Uvarint()) != u32(t, f[0]) || r.U16() != u16(t, f[1]) ||
			r.U16() != u16(t, f[2]) || r.U16() != u16(t, f[3]) {
			t.Error("build fields mismatch")
		}
	case "stop":
		n := int(r.Uvarint())
		for i := 0; i < n; i++ {
			if uint32(r.Uvarint()) != u32(t, strings.Split(args, ";")[i]) {
				t.Error("stop id mismatch")
			}
		}
	case "train":
		f := strings.Split(args, ":")
		if uint32(r.Uvarint()) != u32(t, f[0]) {
			t.Error("train mismatch")
		}
		if len(f) == 2 {
			if uint32(r.Uvarint()) != u32(t, f[1]) {
				t.Error("train unit def mismatch")
			}
		}
	case "attack":
		idsPart, rest, _ := strings.Cut(args, ":")
		target, kindS, _ := strings.Cut(rest, ":")
		n := int(r.Uvarint())
		if n != len(strings.Split(idsPart, ";")) {
			t.Fatalf("attack n %d", n)
		}
		for _, want := range strings.Split(idsPart, ";") {
			if uint32(r.Uvarint()) != u32(t, want) {
				t.Error("attack id mismatch")
			}
		}
		if uint32(r.Uvarint()) != u32(t, target) {
			t.Error("attack target mismatch")
		}
		if uint8(r.Uvarint()) != uint8(u32(t, kindS)) {
			t.Error("attack target kind mismatch")
		}
	case "repair":
		idsPart, target, _ := strings.Cut(args, ":")
		n := int(r.Uvarint())
		if n != len(strings.Split(idsPart, ";")) {
			t.Fatalf("repair n %d", n)
		}
		for _, want := range strings.Split(idsPart, ";") {
			if uint32(r.Uvarint()) != u32(t, want) {
				t.Error("repair id mismatch")
			}
		}
		if uint32(r.Uvarint()) != u32(t, target) {
			t.Error("repair building mismatch")
		}
		if uint8(r.Uvarint()) != TargetBuilding {
			t.Error("repair target kind mismatch")
		}
	case "patrol":
		idsxy, _, _ := strings.Cut(args, ":")
		ids := strings.Split(idsxy, ";")
		n := int(r.Uvarint())
		if n != len(ids) {
			t.Fatalf("patrol n %d want %d", n, len(ids))
		}
		for _, want := range ids {
			if uint32(r.Uvarint()) != u32(t, want) {
				t.Error("patrol id mismatch")
			}
		}
		xyPart := args[len(idsxy)+1:]
		xs, ys, _ := strings.Cut(xyPart, ",")
		if r.U16() != u16(t, xs) || r.U16() != u16(t, ys) {
			t.Error("patrol coords mismatch")
		}
	case "era":
		if uint32(r.Uvarint()) != u32(t, args) {
			t.Error("era mismatch")
		}
	case "research":
		if uint32(r.Uvarint()) != u32(t, args) {
			t.Error("research mismatch")
		}
	case "move-group":
		idsxy, _, _ := strings.Cut(args, ":")
		ids := strings.Split(idsxy, ";")
		n := int(r.Uvarint())
		if n != len(ids) {
			t.Fatalf("move-group n %d want %d", n, len(ids))
		}
		for _, want := range ids {
			if uint32(r.Uvarint()) != u32(t, want) {
				t.Error("move-group id mismatch")
			}
		}
		xyPart := args[len(idsxy)+1:]
		xs, ys, _ := strings.Cut(xyPart, ",")
		if r.U16() != u16(t, xs) || r.U16() != u16(t, ys) {
			t.Error("move-group coords mismatch")
		}
	}
	if !r.OK() {
		t.Errorf("%s: decoder flagged malformed vector", kind)
	}
}

func TestReaderMalformed(t *testing.T) {
	overflow := make([]byte, 11)
	for i := range overflow {
		overflow[i] = 0x80
	}
	bad := []struct {
		raw  []byte
		read func(*Reader)
	}{
		{[]byte{}, func(r *Reader) { r.Uvarint() }},
		{[]byte{0x80}, func(r *Reader) { r.Uvarint() }},
		{overflow, func(r *Reader) { r.Uvarint() }},
		{[]byte{0x01}, func(r *Reader) { r.U16() }},
	}
	for _, b := range bad {
		r := NewReader(b.raw)
		b.read(r)
		if r.OK() {
			t.Errorf("malformed input %x accepted", b.raw)
		}
	}
}

func u16(t *testing.T, s string) uint16 {
	t.Helper()
	v, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	return uint16(v)
}

func u32(t *testing.T, s string) uint32 {
	t.Helper()
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(v)
}
