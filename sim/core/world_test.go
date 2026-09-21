package core

import (
	"testing"

	"rts/sim/proto"
)

func hashSeq(w *World, steps uint32, mk func(uint32) []proto.Order) []uint64 {
	var out []uint64
	for t := uint32(0); t < steps; t++ {
		w.Step(mk(t))
		out = append(out, w.Hash())
	}
	return out
}

func TestDeterministicSequence(t *testing.T) {
	noopAt := func(t uint32) []proto.Order {
		if t%13 == 0 {
			return []proto.Order{{Tick: t, Player: 0, Kind: proto.KindNoop}}
		}
		return nil
	}
	a := hashSeq(NewWorld(0x5eed), 300, noopAt)
	b := hashSeq(NewWorld(0x5eed), 300, noopAt)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("tick %d diverged: %016x vs %016x", i, a[i], b[i])
		}
	}
}

func TestSeedChangesHash(t *testing.T) {
	a := NewWorld(1)
	b := NewWorld(2)
	for i := 0; i < 10; i++ {
		a.Step(nil)
		b.Step(nil)
	}
	if a.Hash() == b.Hash() {
		t.Fatal("different seeds produced equal hashes")
	}
}

func TestHashAdvancesEveryTick(t *testing.T) {
	w := NewWorld(7)
	prev := w.Hash()
	for i := 0; i < 50; i++ {
		w.Step(nil)
		h := w.Hash()
		if h == prev {
			t.Fatalf("hash unchanged at tick %d", w.Tick())
		}
		prev = h
	}
}

func TestOrderAccounting(t *testing.T) {
	w := NewWorld(3)
	w.Step([]proto.Order{
		{Tick: 0, Player: 0, Kind: proto.KindNoop},
		{Tick: 0, Player: 1, Kind: proto.Kind(200)},
	})
	applied, rejected := w.Stats()
	if applied != 1 || rejected != 1 {
		t.Fatalf("stats = (%d, %d), want (1, 1)", applied, rejected)
	}
	if w.Tick() != 1 {
		t.Fatalf("tick = %d, want 1", w.Tick())
	}
}

func TestAppliedOrdersVisibleInHash(t *testing.T) {
	a, b := NewWorld(4), NewWorld(4)
	a.Step([]proto.Order{{Tick: 0, Player: 0, Kind: proto.KindNoop}})
	b.Step(nil)
	if a.Hash() == b.Hash() {
		t.Fatal("applied order must change the hash")
	}
}

func TestHashHexFormat(t *testing.T) {
	w := NewWorld(9)
	h := w.HashHex()
	if len(h) != 16 {
		t.Fatalf("HashHex length = %d, want 16", len(h))
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("HashHex not lowercase hex: %q", h)
		}
	}
}

func TestMapIsDeterministicAndHashed(t *testing.T) {
	a, b := NewWorld(0x5eed), NewWorld(0x5eed)
	if a.Map().Hash() != b.Map().Hash() {
		t.Fatal("map hash differs for the same seed")
	}
	if len(a.Map().Spawns()) == 0 {
		t.Fatal("missing spawns")
	}
	// The map must contribute to the world hash: same counters, same tick,
	// different map (seed) implies different world hash.
	c := NewWorld(0x5eee)
	if a.Hash() == c.Hash() {
		t.Fatal("map seed invisible in world hash")
	}
}
