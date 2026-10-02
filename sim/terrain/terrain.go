// Package terrain defines the tile model: six terrain types plus the
// rectangular grid of tiles with integer heights at vertices.
//
// Heights are whole numbers in [0, MaxHeight] stored per vertex, so a
// w x h grid has (w+1)*(h+1) heights. All storage is slices indexed
// row-major; no maps anywhere.
package terrain

// Type is the terrain kind of a tile.
type Type uint8

const (
	Grass Type = iota
	Forest
	Rock
	Sand
	Shallow
	Deep

	TypeCount = 6

	// MaxHeight is the maximum whole-number vertex height.
	MaxHeight int32 = 64
)

// Walkable reports whether units can stand and move on this terrain.
func (t Type) Walkable() bool {
	return t != Rock && t != Shallow && t != Deep
}

// Water reports whether this terrain is any water kind.
func (t Type) Water() bool {
	return t == Shallow || t == Deep
}

// String returns the debug name used in logs (UI names live in i18n).
func (t Type) String() string {
	switch t {
	case Grass:
		return "grass"
	case Forest:
		return "forest"
	case Rock:
		return "rock"
	case Sand:
		return "sand"
	case Shallow:
		return "shallow"
	case Deep:
		return "deep"
	default:
		return "invalid"
	}
}

// Grid is one rectangular map: w x h tiles plus vertex heights.
type Grid struct {
	w, h  int
	tiles []Type
	verts []int32
}

// NewGrid builds an empty (all-grass, flat) w x h grid.
func NewGrid(w, h int) *Grid {
	return &Grid{
		w:     w,
		h:     h,
		tiles: make([]Type, w*h),
		verts: make([]int32, (w+1)*(h+1)),
	}
}

// NewGridSquare builds an empty n x n grid.
func NewGridSquare(n int) *Grid {
	return NewGrid(n, n)
}

// Width returns the number of tiles per row.
func (g *Grid) Width() int { return g.w }

// Height returns the number of tile rows.
func (g *Grid) Height() int { return g.h }

// Size is the side length of a square grid; it panics when the grid is
// not square. Only code that still assumes square maps may use it.
func (g *Grid) Size() int {
	if g.w != g.h {
		panic("terrain: Size on non-square grid")
	}
	return g.w
}

// InBounds reports whether tile coordinates are inside the grid.
func (g *Grid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.w && y < g.h
}

// Tile returns the terrain at (x, y); it panics when out of bounds, which
// is always a programming error.
func (g *Grid) Tile(x, y int) Type {
	if !g.InBounds(x, y) {
		panic("terrain: tile out of bounds")
	}
	return g.tiles[y*g.w+x]
}

// SetTile assigns the terrain at (x, y).
func (g *Grid) SetTile(x, y int, t Type) {
	if !g.InBounds(x, y) {
		panic("terrain: tile out of bounds")
	}
	g.tiles[y*g.w+x] = t
}

// Vertex returns the height of vertex (x, y), where x and y run
// 0..Width and 0..Height.
func (g *Grid) Vertex(x, y int) int32 {
	stride := g.w + 1
	if x < 0 || y < 0 || x > g.w || y > g.h {
		panic("terrain: vertex out of bounds")
	}
	return g.verts[y*stride+x]
}

// SetVertex assigns a vertex height, clamped to [0, MaxHeight].
func (g *Grid) SetVertex(x, y int, h int32) {
	if h < 0 {
		h = 0
	}
	if h > MaxHeight {
		h = MaxHeight
	}
	stride := g.w + 1
	g.verts[y*stride+x] = h
}

// Tiles returns the row-major tile slice. The caller must not modify it
// outside simulation code.
func (g *Grid) Tiles() []Type { return g.tiles }

// Vertices returns the row-major vertex height slice.
func (g *Grid) Vertices() []int32 { return g.verts }
