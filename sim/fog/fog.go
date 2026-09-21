// Package fog implements per-player three-state fog of war.
//
// States per tile: Unexplored (never seen), Explored (seen before, not
// now), Visible (in line of sight right now). Vision is a per-tick union
// of radii around the player's units and completed buildings; a tile that
// loses visibility decays to Explored and never back.
package fog

import "rts/sim/hash"

// State values, ordered so decay is a simple decrement.
const (
	Unexplored uint8 = 0
	Explored   uint8 = 1
	Visible    uint8 = 2
)

// Grid is one player's fog layer over a size x size map.
type Grid struct {
	size   int
	states []uint8
}

// New returns an all-unexplored layer.
func New(size int) *Grid {
	return &Grid{size: size, states: make([]uint8, size*size)}
}

// Size returns the map side length.
func (g *Grid) Size() int { return g.size }

// State returns the fog state of a tile; out-of-bounds reads are
// unexplored.
func (g *Grid) State(x, y int) uint8 {
	if x < 0 || y < 0 || x >= g.size || y >= g.size {
		return Unexplored
	}
	return g.states[y*g.size+x]
}

func (g *Grid) set(x, y int, s uint8) {
	if x < 0 || y < 0 || x >= g.size || y >= g.size {
		return
	}
	i := y*g.size + x
	if g.states[i] < s {
		g.states[i] = s
	}
}

// Decay turns every Visible tile into Explored; call at the start of a
// tick before re-applying vision.
func (g *Grid) Decay() {
	for i, s := range g.states {
		if s == Visible {
			g.states[i] = Explored
		}
	}
}

// ApplyCircle marks every tile within radius (Chebyshev metric on tile
// centers, rounded Euclidean) of (cx, cy) Visible.
func (g *Grid) ApplyCircle(cx, cy, r int) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r+r/2 {
				g.set(cx+dx, cy+dy, Visible)
			}
		}
	}
}

// Counts returns how many tiles sit in each state.
func (g *Grid) Counts() (unexplored, explored, visible int) {
	for _, s := range g.states {
		switch s {
		case Unexplored:
			unexplored++
		case Explored:
			explored++
		case Visible:
			visible++
		}
	}
	return
}

// Packed appends the layer as 2 bits per tile, row-major, into buf.
func (g *Grid) Packed(buf []byte) []byte {
	n := len(g.states)
	for i := 0; i < n; i += 4 {
		var b uint8
		for j := 0; j < 4 && i+j < n; j++ {
			b |= g.states[i+j] << (2 * j)
		}
		buf = append(buf, b)
	}
	return buf
}

// PackedLen is the byte length of Packed output.
func PackedLen(size int) int {
	n := size * size
	return (n + 3) / 4
}

// AppendState folds the layer into a hash digest.
func (g *Grid) AppendState(h *hash.FNV1a64, label string) {
	h.WriteString(label)
	h.WriteU32(uint32(g.size))
	packed := make([]byte, 0, PackedLen(g.size))
	h.Write(g.Packed(packed))
}
