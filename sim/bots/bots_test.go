package bots_test

import (
	"testing"

	"rts/sim/bots"
	"rts/sim/core"
	"rts/sim/resource"
	"rts/sim/simtest"
	"rts/sim/worldgen"
)

func TestBotReachesAntiquity(t *testing.T) {
	w := core.NewWorldSize(0x5eed, worldgen.DefaultSize, 2)
	brains := map[uint8]*bots.Brain{0: bots.New(0, bots.Hard)}
	simtest.Drive(w, brains, 9000)
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
		brains := map[uint8]*bots.Brain{
			0: bots.New(0, bots.Hard), 1: bots.New(1, bots.Standard), 2: bots.New(2, bots.Easy),
		}
		simtest.Drive(w, brains, 4000)
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
	brains := map[uint8]*bots.Brain{
		0: bots.New(0, bots.Hard), 1: bots.New(1, bots.Standard), 2: bots.New(2, bots.Standard),
	}
	for round := 0; round < 60; round++ {
		simtest.Drive(w, brains, 2000)
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
