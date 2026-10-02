package terrain

import "testing"

func TestTypeWalkable(t *testing.T) {
	tests := []struct {
		typ      Type
		walkable bool
		water    bool
	}{
		{Grass, true, false},
		{Forest, true, false},
		{Rock, false, false},
		{Sand, true, false},
		{Shallow, false, true},
		{Deep, false, true},
	}
	for _, tt := range tests {
		if got := tt.typ.Walkable(); got != tt.walkable {
			t.Errorf("%s: Walkable = %v, want %v", tt.typ, got, tt.walkable)
		}
		if got := tt.typ.Water(); got != tt.water {
			t.Errorf("%s: Water = %v, want %v", tt.typ, got, tt.water)
		}
	}
}

func TestTypeString(t *testing.T) {
	names := []string{"grass", "forest", "rock", "sand", "shallow", "deep"}
	for i, want := range names {
		if got := Type(i).String(); got != want {
			t.Errorf("Type(%d).String() = %q, want %q", i, got, want)
		}
	}
	if got := Type(9).String(); got != "invalid" {
		t.Errorf("invalid type name = %q", got)
	}
}

func TestGridRoundTrip(t *testing.T) {
	g := NewGridSquare(4)
	if g.Size() != 4 || len(g.Tiles()) != 16 || len(g.Vertices()) != 25 {
		t.Fatal("bad initial layout")
	}
	g.SetTile(3, 2, Forest)
	if g.Tile(3, 2) != Forest {
		t.Fatal("tile round trip failed")
	}
	// Row-major layout check: index of (3, 2) must be 2*4+3.
	if g.Tiles()[2*4+3] != Forest {
		t.Fatal("tiles are not row-major")
	}
	g.SetVertex(4, 4, 42)
	if g.Vertex(4, 4) != 42 {
		t.Fatal("vertex round trip failed")
	}
}

func TestVertexClamp(t *testing.T) {
	g := NewGridSquare(2)
	g.SetVertex(0, 0, MaxHeight+100)
	if g.Vertex(0, 0) != MaxHeight {
		t.Errorf("high clamp = %d", g.Vertex(0, 0))
	}
	g.SetVertex(0, 0, -5)
	if g.Vertex(0, 0) != 0 {
		t.Errorf("low clamp = %d", g.Vertex(0, 0))
	}
}

func TestBoundsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	NewGridSquare(2).Tile(-1, 0)
}

func TestGridRect(t *testing.T) {
	g := NewGrid(5, 3)
	if g.Width() != 5 || g.Height() != 3 {
		t.Fatal("bad initial layout")
	}
	if len(g.Tiles()) != 15 || len(g.Vertices()) != 24 {
		t.Fatal("bad storage size")
	}
	g.SetTile(4, 2, Forest)
	if g.Tile(4, 2) != Forest || g.Tiles()[2*5+4] != Forest {
		t.Fatal("tile round trip failed")
	}
	if g.InBounds(5, 0) || g.InBounds(0, 3) {
		t.Fatal("bounds check failed")
	}
	g.SetVertex(5, 3, 7)
	if g.Vertex(5, 3) != 7 {
		t.Fatal("vertex round trip failed")
	}
}

func TestSizePanicsOnRect(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	NewGrid(4, 3).Size()
}
