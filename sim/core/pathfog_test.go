package core

import (
	"testing"

	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/terrain"
	"rts/sim/worldgen"
)

// channelMap builds a walkable island split by a horizontal water strip
// with a single gap at column gap, so any north-south route must funnel
// through it.
func channelMap(size, gap int) *worldgen.Map {
	grid := terrain.NewGridSquare(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if y == size/2 && x != gap {
				grid.SetTile(x, y, terrain.Deep)
			}
		}
	}
	spawns := [][2]int{{3, 3}, {size - 4, 3}}
	return worldgen.Custom(grid, spawns, nil)
}

func playerUnitIDs(w *World, owner uint8) []entity.ID {
	var ids []entity.ID
	w.Units().Each(func(id entity.ID, u *Unit) {
		if u.Owner == owner {
			ids = append(ids, id)
		}
	})
	return ids
}

func ids32(ids []entity.ID) []uint32 {
	out := make([]uint32, 0, len(ids))
	for _, id := range ids {
		out = append(out, uint32(id))
	}
	return out
}

func TestGroupMoveThroughGap(t *testing.T) {
	run := func() (uint64, int, int) {
		w := NewWorldWithMap(0x42, channelMap(30, 14))
		ids := playerUnitIDs(w, 0)
		w.Step([]proto.Order{{Player: 0, Kind: orders.MoveGroup,
			Payload: orders.MoveGroupPayload(ids32(ids), 24*16+8, 26*16+8)}})
		for i := 0; i < 20*120; i++ {
			w.Step(nil)
		}
		moved, onSlot := 0, 0
		for _, id := range ids {
			u := w.Units().Get(id)
			if u.State == UnitIdle && int32(u.X) > 15<<16 {
				moved++
			}
			if u.X == u.TX && u.Y == u.TY {
				onSlot++
			}
		}
		return w.Hash(), moved, onSlot
	}
	h1, moved1, slots1 := run()
	h2, moved2, slots2 := run()
	if h1 != h2 || moved1 != moved2 || slots1 != slots2 {
		t.Fatalf("nondeterministic group move: %x/%d/%d vs %x/%d/%d", h1, moved1, slots1, h2, moved2, slots2)
	}
	if moved1 != 3 || slots1 != 3 {
		t.Fatalf("moved %d/3 across the gap, on-slot %d/3", moved1, slots1)
	}
	// Formation slots must be distinct tiles.
	w := NewWorldWithMap(0x42, channelMap(30, 14))
	ids := playerUnitIDs(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.MoveGroup,
		Payload: orders.MoveGroupPayload(ids32(ids), 24*16+8, 26*16+8)}})
	seen := map[[2]int32]bool{}
	for _, id := range ids {
		u := w.Units().Get(id)
		key := [2]int32{tileOf(u.TX), tileOf(u.TY)}
		if seen[key] {
			t.Fatalf("two units share formation slot %v", key)
		}
		seen[key] = true
	}
}

func TestUnreachableMoveWaits(t *testing.T) {
	grid := terrain.NewGridSquare(30)
	for y := 10; y < 15; y++ {
		for x := 0; x < 30; x++ {
			grid.SetTile(x, y, terrain.Deep)
		}
	}
	w := NewWorldWithMap(0x43, worldgen.Custom(grid, [][2]int{{3, 3}, {25, 25}}, nil))
	ids := playerUnitIDs(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.MoveGroup,
		Payload: orders.MoveGroupPayload(ids32(ids), 3*16+8, 27*16+8)}})
	for i := 0; i < 20*60; i++ {
		w.Step(nil)
	}
	for _, id := range playerUnitIDs(w, 0) {
		u := w.Units().Get(id)
		if u.HP <= 0 {
			t.Fatal("unit died waiting")
		}
		if !w.tileWalkable(tileOf(u.X), tileOf(u.Y)) {
			t.Fatalf("unit on unwalkable tile %d,%d", tileOf(u.X), tileOf(u.Y))
		}
	}
}

func TestBuildingBlocksRoute(t *testing.T) {
	w := NewWorldWithMap(0x42, channelMap(30, 14))
	ids := playerUnitIDs(w, 0)
	// Wall the single gap with a row of campfires (1 tile each).
	for x := 13; x <= 15; x++ {
		w.Step([]proto.Order{{Player: 0, Kind: orders.Build,
			Payload: orders.BuildPayload(uint32(ids[0]), 2, uint16(x), 16)}})
	}
	for i := 0; i < 40; i++ {
		w.Step(nil)
	}
	// Order the group south across the now-sealed gap: units must wait
	// alive on walkable ground, never enter a building.
	w.Step([]proto.Order{{Player: 0, Kind: orders.MoveGroup,
		Payload: orders.MoveGroupPayload(ids32(ids), 14*16+8, 20*16+8)}})
	for i := 0; i < 20*60; i++ {
		w.Step(nil)
	}
	for _, id := range playerUnitIDs(w, 0) {
		u := w.Units().Get(id)
		tx, ty := tileOf(u.X), tileOf(u.Y)
		if !w.tileWalkable(tx, ty) {
			t.Fatalf("unit at %d,%d stands inside a building or water", tx, ty)
		}
	}
}

func TestFogLifecycle(t *testing.T) {
	w := NewWorldWithMap(0x42, channelMap(30, 14))
	f := w.Fog(0)
	if f.State(3, 3) != 2 {
		t.Fatalf("spawn tile fog %d, want visible", f.State(3, 3))
	}
	if f.State(28, 28) != 0 {
		t.Fatalf("far tile fog %d, want unexplored", f.State(28, 28))
	}
	ids := playerUnitIDs(w, 0)
	if f.State(0, 8) != 2 {
		t.Fatalf("tile (0,8) fog %d, want visible from start", f.State(0, 8))
	}
	for _, id := range ids {
		u := w.Units().Get(id)
		u.X, u.Y = tileCenter(27), tileCenter(27)
	}
	w.Step(nil)
	if f.State(0, 8) != 1 {
		t.Fatalf("tile (0,8) fog %d after moving away, want explored", f.State(0, 8))
	}
	if f.State(27, 27) != 2 {
		t.Fatal("new position must be visible")
	}
	wb := NewWorldWithMap(0x42, channelMap(30, 14))
	if w.Hash() == wb.Hash() {
		t.Fatal("diverged vision must change the hash")
	}
}
