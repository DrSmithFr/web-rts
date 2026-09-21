package path

import "testing"

// parseMap turns an ASCII map into a walk callback. '#' is wall, any
// other cell walkable; coordinates follow (x, y) with y down.
func parseMap(t *testing.T, art []string) (WalkFn, int32, int32, [2]int32, [2]int32) {
	t.Helper()
	h := len(art)
	w := len(art[0])
	for _, row := range art {
		if len(row) != w {
			t.Fatalf("ragged map row %q", row)
		}
	}
	return func(x, y int32) bool {
		if x < 0 || y < 0 || x >= int32(w) || y >= int32(h) {
			return false
		}
		return art[y][x] != '#'
	}, int32(w), int32(h), [2]int32{-1, -1}, [2]int32{-1, -1}
}

func TestStraightCorridor(t *testing.T) {
	walk, w, h, _, _ := parseMap(t, []string{
		"#########",
		"#.......#",
		"#########",
	})
	f := ComputeField(walk, 0, 0, w, h, 7, 1)
	if f.Distance(1, 1) != 6 {
		t.Fatalf("distance %d, want 6", f.Distance(1, 1))
	}
	x, y := int32(1), int32(1)
	for i := 0; i < 10; i++ {
		x, y, _ = NextStep(&f, walk, nil, x, y)
		if x == 7 {
			return
		}
	}
	t.Fatalf("never reached goal, stopped at %d,%d", x, y)
}

func TestAroundLake(t *testing.T) {
	walk, w, h, _, _ := parseMap(t, []string{
		".........",
		"..#####..",
		"..#####..",
		"..#####..",
		".........",
	})
	f := ComputeField(walk, 0, 0, w, h, 1, 4)
	if d := f.Distance(7, 0); d == Unreachable || d < 8 {
		t.Fatalf("distance %d, want a detour of at least 8", d)
	}
	x, y := int32(7), int32(0)
	for i := 0; i < 40; i++ {
		x, y, _ = NextStep(&f, walk, nil, x, y)
		if x == 1 && y == 4 {
			return
		}
	}
	t.Fatalf("stuck at %d,%d", x, y)
}

func TestDiagonalGapsBlocked(t *testing.T) {
	// Units must not clip through the diagonal gap between the walls.
	walk, w, h, _, _ := parseMap(t, []string{
		"#.#",
		".#.",
		"#.#",
	})
	f := ComputeField(walk, 0, 0, w, h, 0, 0)
	if f.Distance(2, 2) != Unreachable {
		t.Fatal("diagonal gap must block the southeast cell")
	}
}

func TestUnreachableGoal(t *testing.T) {
	walk, w, h, _, _ := parseMap(t, []string{
		"...",
		".#.",
		"...",
	})
	f := ComputeField(walk, 0, 0, w, h, 1, 1)
	if f.Distance(0, 0) != Unreachable {
		t.Fatal("unwalkable goal must leave everything unreachable")
	}
	if _, _, ok := NextStep(&f, walk, nil, 0, 0); ok {
		t.Fatal("NextStep must refuse an unreachable walker")
	}
}

func TestBlockedNeighbourWaits(t *testing.T) {
	walk, w, h, _, _ := parseMap(t, []string{
		"...",
		"...",
		"...",
	})
	f := ComputeField(walk, 0, 0, w, h, 2, 1)
	blocked := func(x, y int32) bool { return x == 1 && (y == 0 || y == 1 || y == 2) }
	x, y, ok := NextStep(&f, walk, blocked, 0, 1)
	if !ok || x != 0 || y != 1 {
		t.Fatalf("expected wait in place, got %d,%d ok=%v", x, y, ok)
	}
}

func TestFieldWindowBounds(t *testing.T) {
	walk, _, _, _, _ := parseMap(t, []string{"....", "...."})
	f := ComputeField(walk, 1, 0, 2, 2, 2, 1)
	if f.Distance(0, 0) != Unreachable {
		t.Fatal("cells outside the window must be unreachable")
	}
	if f.Distance(2, 1) != 0 {
		t.Fatal("goal inside window must be distance 0")
	}
}

func TestSpiralOffsetsValid(t *testing.T) {
	seen := map[[2]int32]bool{}
	for i, o := range SpiralOffsets {
		if seen[o] {
			t.Fatalf("duplicate spiral offset %v at %d", o, i)
		}
		seen[o] = true
		if abs32v(o[0]) > 5 || abs32v(o[1]) > 5 {
			t.Fatalf("offset %v too far from base", o)
		}
	}
	if SpiralOffsets[0] != [2]int32{0, 0} {
		t.Fatal("first slot must be the base")
	}
	if len(SpiralOffsets) != 64 {
		t.Fatalf("len %d, want 64", len(SpiralOffsets))
	}
}

func abs32v(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestWalkAroundEntityDetours(t *testing.T) {
	// The same field, but a blocked wall with a gap forces a detour.
	walk, w, h, _, _ := parseMap(t, []string{
		".....",
		"#.###",
		".....",
		"..###",
		".....",
	})
	_ = h
	f := ComputeField(walk, 0, 0, w, 5, 0, 4)
	// gap at (0,2): route exists down the left edge
	if f.Distance(4, 0) == Unreachable {
		t.Fatal("left-edge gap should keep the map connected")
	}
	x, y := int32(4), int32(0)
	for i := 0; i < 25; i++ {
		var ok bool
		x, y, ok = NextStep(&f, walk, nil, x, y)
		if !ok {
			t.Fatalf("lost field at %d,%d", x, y)
		}
		if x == 0 && y == 4 {
			return
		}
	}
	t.Fatalf("stuck at %d,%d", x, y)
}
