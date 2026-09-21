package fog

import "testing"

func TestDecayCycle(t *testing.T) {
	g := New(5)
	g.ApplyCircle(2, 2, 1)
	if g.State(2, 2) != Visible || g.State(3, 2) != Visible {
		t.Fatal("circle not applied")
	}
	if g.State(0, 0) != Unexplored {
		t.Fatal("far tile must stay unexplored")
	}
	g.Decay()
	if g.State(2, 2) != Explored || g.State(0, 0) != Unexplored {
		t.Fatal("decay must turn visible into explored only")
	}
	g.ApplyCircle(0, 0, 0)
	if g.State(0, 0) != Visible {
		t.Fatal("re-sighting must not pass explored")
	}
	g.Decay()
	if g.State(2, 2) != Explored {
		t.Fatal("explored must not decay further")
	}
}

func TestPackedRoundtrip(t *testing.T) {
	g := New(3)
	g.ApplyCircle(1, 1, 1)
	g.Decay()
	packed := g.Packed(nil)
	if len(packed) != PackedLen(3) {
		t.Fatalf("packed len %d, want %d", len(packed), PackedLen(3))
	}
	for i := 0; i < 9; i++ {
		want := g.states[i]
		got := (packed[i/4] >> (2 * (i % 4))) & 3
		if got != want {
			t.Fatalf("tile %d: packed %d want %d", i, got, want)
		}
	}
}

func TestOutOfBoundsSafe(t *testing.T) {
	g := New(4)
	g.ApplyCircle(0, 0, 10) // must clamp, not panic
	if g.State(3, 2) != Visible || g.State(-1, 0) != Unexplored {
		t.Fatal("circle/edge handling broken")
	}
}

func TestCounts(t *testing.T) {
	g := New(10)
	g.ApplyCircle(5, 5, 2)
	_, _, vis := g.Counts()
	if vis == 0 || vis >= 100 {
		t.Fatalf("visible %d out of 100", vis)
	}
}
