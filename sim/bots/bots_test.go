package bots

import (
	"sort"
	"testing"

	"rts/sim/core"
	"rts/sim/proto"
	"rts/sim/resource"
	"rts/sim/worldgen"
)

// drive steps world with brain-generated orders, stamping them for the
// next tick exactly like a lockstep peer would.
func drive(w *core.World, brains map[uint8]*Brain, ticks uint32) {
	var pending []proto.Order
	for t := uint32(0); t < ticks; t++ {
		next := pending
		pending = nil
		w.Step(next)
		var ids []uint8
		for p := range brains {
			ids = append(ids, p)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, p := range ids {
			for _, o := range brains[p].Step(w, t) {
				o.Tick = w.Tick()
				pending = append(pending, o)
			}
		}
	}
}

func TestBotReachesAntiquity(t *testing.T) {
	w := core.NewWorldSize(0x5eed, worldgen.DefaultSize, 2)
	brains := map[uint8]*Brain{0: New(0, Hard)}
	drive(w, brains, 9000)
	pl := &w.Players()[0]
	if pl.Era < 1 {
		t.Fatalf("era = %d after 9000 ticks, want >= 1", pl.Era)
	}
	if pl.Res[resource.Wood] < 0 {
		t.Fatal("negative resources")
	}
}

func TestDeterminismOfBrains(t *testing.T) {
	run := func() uint64 {
		w := core.NewWorldSize(0x1234, worldgen.DefaultSize, 3)
		brains := map[uint8]*Brain{
			0: New(0, Hard), 1: New(1, Standard), 2: New(2, Easy),
		}
		drive(w, brains, 4000)
		return w.Hash()
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("brains diverged: %016x vs %016x", a, b)
	}
}

// TestBrainsPlayToVictory runs three brains unsupervised until one
// wins; relic victory is the expected path (units cannot raze
// buildings yet, so elimination is unlikely).
func TestBrainsPlayToVictory(t *testing.T) {
	if testing.Short() {
		t.Skip("long bot game")
	}
	w := core.NewWorldSize(0x5eed, worldgen.DefaultSize, 3)
	brains := map[uint8]*Brain{
		0: New(0, Hard), 1: New(1, Standard), 2: New(2, Standard),
	}
	for round := 0; round < 40; round++ {
		drive(w, brains, 2000)
		if p, kind := w.Winner(); p != core.OwnerNeutral {
			t.Logf("winner player %d kind %d at tick %d", p, kind, w.Tick())
			if kind == core.WinnerNone {
				t.Fatalf("winner set without kind at tick %d", w.Tick())
			}
			return
		}
	}
	t.Fatalf("no winner after %d ticks (era %d/%d/%d)", w.Tick(),
		w.Players()[0].Era, w.Players()[1].Era, w.Players()[2].Era)
}
