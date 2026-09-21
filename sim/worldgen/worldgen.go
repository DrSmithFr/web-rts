// Package worldgen produces one deterministic terrain map from a single
// seed shared by all players.
//
// The generator is pure integer math: splitmix64-hashed value noise in
// fixed point, integer classification thresholds and a fixed-order spawn
// selection, so every platform and every build agrees on every byte.
//
// Balance guarantee (phase 2 level): every spawn point gets an identical
// starting area — a grass disk, a forest ring of eight tiles and rock
// corners at fixed offsets — plus a minimum pairwise separation, all on
// land selected by a deterministic best-first scan.
package worldgen

import (
	"errors"
	"sort"

	"rts/sim/fixed"
	"rts/sim/hash"
	"rts/sim/resource"
	"rts/sim/terrain"
)

const (
	DefaultSize    = 200
	DefaultPlayers = 2

	amp0, stride0 = 30, 24 // continents
	amp1, stride1 = 16, 8  // hills
	amp2, stride2 = 4, 4   // detail

	deepLevel   int32 = 20
	waterLevel  int32 = 25
	sandLevel   int32 = 28
	forestMoist int32 = 34
	forestMaxH  int32 = 48
	rockLevel   int32 = 44
	slopeRock   int32 = 5

	spawnMargin  = 14
	spawnMinDist = 48
	spawnLandMin = 25 // walkable tiles in the 5x5 spawn window (of 25)
	spawnWindow  = 5

	goldScatter = 90  // one gold node per N grass tiles (hash bucket)
	oilScatter  = 120 // one oil node per N sand tiles (hidden until Modern)

	berrySalt = 0x1f4a7c93e2b5d617
	goldSalt  = 0x52aa1c3d7e99f011
	oilSalt   = 0x0c7e1d5b3a8f6249

	hashLabel = "rts.map/v2"
)

// ErrNoSpawns means the map does not contain enough separated land patches
// for the requested number of players.
var ErrNoSpawns = errors.New("worldgen: not enough land for requested players")

// Node is a harvestable resource node placed by the generator.
type Node struct {
	X, Y   int32
	Kind   resource.Kind
	Amount int32
}

// Map is a generated world map.
type Map struct {
	grid   *terrain.Grid
	spawns [][2]int
	nodes  []Node
	digest uint64
}

// Grid exposes the terrain grid.
func (m *Map) Grid() *terrain.Grid { return m.grid }

// Spawns returns the player start positions in order.
func (m *Map) Spawns() [][2]int { return m.spawns }

// Nodes returns the harvestable nodes in deterministic (row-major, then
// per-player berry rings) order.
func (m *Map) Nodes() []Node { return m.nodes }

// Hash returns the canonical map digest.
func (m *Map) Hash() uint64 { return m.digest }

// Generate builds a map for the given seed, size (tiles per side) and
// player count.
func Generate(seed uint64, size, players int) (*Map, error) {
	if size < spawnMargin*4 || players < 1 {
		return nil, errors.New("worldgen: map too small")
	}
	g := terrain.NewGrid(size)

	for y := int64(0); y <= int64(size); y++ {
		for x := int64(0); x <= int64(size); x++ {
			g.SetVertex(int(x), int(y), heightAt(seed, x, y))
		}
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			g.SetTile(x, y, classify(seed, g, x, y))
		}
	}

	spawns, err := placeSpawns(g, players)
	if err != nil {
		return nil, err
	}
	for _, s := range spawns {
		stampStart(g, s[0], s[1])
	}
	berries := stampBerries(g, spawns)
	nodes := placeNodes(g, seed, berries)

	m := &Map{grid: g, spawns: spawns, nodes: nodes}
	m.digest = hashMap(m)
	return m, nil
}

// mix64 is the splitmix64 finalizer, the integer hash under all noise.
func mix64(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// lattice returns a pseudo-random value in [0, 63] for an octave lattice
// coordinate; identical for identical (seed, octave, coord).
func lattice(seed, octave uint64, x, y int64) int64 {
	h := seed ^ 0x9e3779b97f4a7c15
	h = mix64(h ^ uint64(x)*0xff51afd7ed558ccd)
	h = mix64(h ^ uint64(y)*0xc4ceb9fe1a85ec53)
	h = mix64(h ^ octave*0x165667b19e3779f9)
	return int64(h >> 58)
}

// smooth applies the smoothstep curve 3t^2 - 2t^3 in 16.16.
func smooth(t fixed.F16) fixed.F16 {
	return t.Mul(t).Mul(fixed.FromInt(3).Sub(fixed.FromInt(2).Mul(t)))
}

// valueNoise bilinearly samples one octave of hashed lattice values with
// smoothstep weights, returning 0..63.
func valueNoise(seed, octave uint64, x, y, stride int64) int64 {
	x0 := (x / stride) * stride
	y0 := (y / stride) * stride
	x1 := x0 + stride
	y1 := y0 + stride

	tx := smooth(fixed.F16(((x - x0) << 16) / stride))
	ty := smooth(fixed.F16(((y - y0) << 16) / stride))

	v00 := lattice(seed, octave, x0, y0)
	v10 := lattice(seed, octave, x1, y0)
	v01 := lattice(seed, octave, x0, y1)
	v11 := lattice(seed, octave, x1, y1)

	ix0 := v00 + ((v10-v00)*int64(tx))>>16
	ix1 := v01 + ((v11-v01)*int64(tx))>>16
	return ix0 + ((ix1-ix0)*int64(ty))>>16
}

// heightAt sums three noise octaves into a whole-number terrain height.
func heightAt(seed uint64, x, y int64) int32 {
	n0 := valueNoise(seed, 0, x, y, stride0)
	n1 := valueNoise(seed, 1, x, y, stride1)
	n2 := valueNoise(seed, 2, x, y, stride2)
	h := (amp0*n0 + amp1*n1 + amp2*n2) >> 6
	return int32(h + 6)
}

// moistureAt returns 0..63 of humidity for a tile corner.
func moistureAt(seed uint64, x, y int64) int64 {
	return valueNoise(seed^0x2ba166cd9fb7e31, 3, x, y, 32)
}

// classify derives the tile terrain from its corner heights and moisture.
func classify(seed uint64, g *terrain.Grid, x, y int) terrain.Type {
	v00 := g.Vertex(x, y)
	v10 := g.Vertex(x+1, y)
	v01 := g.Vertex(x, y+1)
	v11 := g.Vertex(x+1, y+1)
	mean := int32((int(v00) + int(v10) + int(v01) + int(v11)) / 4)

	hi, lo := v00, v00
	for _, v := range [3]int32{v10, v01, v11} {
		if v > hi {
			hi = v
		}
		if v < lo {
			lo = v
		}
	}
	slope := hi - lo

	switch {
	case mean < deepLevel:
		return terrain.Deep
	case mean < waterLevel:
		return terrain.Shallow
	case mean < sandLevel:
		return terrain.Sand
	case mean >= rockLevel || (slope > slopeRock && mean > sandLevel):
		return terrain.Rock
	case moistureAt(seed, int64(x), int64(y)) > int64(forestMoist) && mean < forestMaxH:
		return terrain.Forest
	default:
		return terrain.Grass
	}
}

type spawnCandidate struct {
	score int
	index int
	x, y  int
}

// placeSpawns picks player starts deterministically: candidates are scored
// by walkable tiles inside the spawn window and sorted best first (ties by
// row-major order). The first start is the best candidate; each following
// start is the candidate maximizing the distance to the already chosen
// ones (ties by sort order), so starts spread out over equal-quality land.
func placeSpawns(g *terrain.Grid, players int) ([][2]int, error) {
	size := g.Size()
	var candidates []spawnCandidate
	for y := spawnMargin; y < size-spawnMargin; y++ {
		for x := spawnMargin; x < size-spawnMargin; x++ {
			score := landScore(g, x, y)
			if score >= spawnLandMin {
				candidates = append(candidates, spawnCandidate{score, y*size + x, x, y})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index < candidates[j].index
	})

	minDist2 := spawnMinDist * spawnMinDist
	var chosen [][2]int
	for len(chosen) < players {
		bestIdx, bestDist := -1, -1
		for i, c := range candidates {
			dupe := false
			for _, s := range chosen {
				if s[0] == c.x && s[1] == c.y {
					dupe = true
					break
				}
			}
			if dupe {
				continue
			}
			d := -1
			for _, s := range chosen {
				dx, dy := c.x-s[0], c.y-s[1]
				d2 := dx*dx + dy*dy
				if d == -1 || d2 < d {
					d = d2
				}
			}
			if d == -1 || d > bestDist {
				bestIdx, bestDist = i, d
			}
		}
		if bestIdx == -1 || (len(chosen) > 0 && bestDist < minDist2) {
			break
		}
		chosen = append(chosen, [2]int{candidates[bestIdx].x, candidates[bestIdx].y})
	}
	if len(chosen) < players {
		return chosen, ErrNoSpawns
	}
	return chosen, nil
}

// landScore counts walkable tiles in the spawnWindow square centered at (x, y).
func landScore(g *terrain.Grid, x, y int) int {
	score := 0
	r := spawnWindow / 2
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if g.Tile(x+dx, y+dy).Walkable() {
				score++
			}
		}
	}
	return score
}

// stampStart carves the identical starting area around a spawn: a grass
// disk (radius 3, water included so every start is comparable), a forest
// ring of eight tiles and rock corners. Offsets are identical per player.
func stampStart(g *terrain.Grid, x, y int) {
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			if dx*dx+dy*dy < 9 {
				g.SetTile(x+dx, y+dy, terrain.Grass)
			}
		}
	}
	for _, o := range [][2]int{{3, 0}, {-3, 0}, {0, 3}, {0, -3}, {3, 3}, {3, -3}, {-3, 3}, {-3, -3}} {
		g.SetTile(x+o[0], y+o[1], terrain.Forest)
	}
	for _, o := range [][2]int{{5, 5}, {-5, 5}, {5, -5}, {-5, -5}} {
		g.SetTile(x+o[0], y+o[1], terrain.Rock)
	}
}

// scatter returns a well-mixed 64-bit hash per (seed, salt, tile), used
// for sparse resource placement without materializing tables.
func scatter(seed, salt uint64, x, y int64) uint64 {
	h := mix64(seed ^ salt)
	h = mix64(h ^ uint64(x)*0xff51afd7ed558ccd)
	h = mix64(h ^ uint64(y)*0xc4ceb9fe1a85ec53)
	return h
}

// stampBerries clears four berry spots around each spawn and returns their
// positions. Offsets are tried in a fixed order so every player gets the
// same starting pattern; water spots are skipped for the next candidate.
func stampBerries(g *terrain.Grid, spawns [][2]int) [][2]int {
	offsets := [][2]int{
		{7, 0}, {-7, 0}, {0, 7}, {0, -7},
		{7, 4}, {-7, 4}, {4, 7}, {-4, 7},
		{7, -4}, {-7, -4}, {4, -7}, {-4, -7},
		{9, 0}, {-9, 0}, {0, 9}, {0, -9},
	}
	var out [][2]int
	for _, s := range spawns {
		placed := 0
		for _, o := range offsets {
			if placed == 4 {
				break
			}
			x, y := s[0]+o[0], s[1]+o[1]
			if !g.InBounds(x, y) {
				continue
			}
			if t := g.Tile(x, y); t == terrain.Shallow || t == terrain.Deep {
				continue
			}
			g.SetTile(x, y, terrain.Grass)
			out = append(out, [2]int{x, y})
			placed++
		}
	}
	return out
}

// placeNodes turns forest tiles into wood nodes, rock tiles into stone
// nodes, sprinkles gold on grass and oil on sand by hash bucket, and adds
// berry nodes at the stamped positions.
func placeNodes(g *terrain.Grid, seed uint64, berries [][2]int) []Node {
	var nodes []Node
	for y := 0; y < g.Size(); y++ {
		for x := 0; x < g.Size(); x++ {
			switch g.Tile(x, y) {
			case terrain.Forest:
				nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Wood, Amount: resource.TreeAmount})
			case terrain.Rock:
				nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Stone, Amount: resource.StoneAmount})
			case terrain.Grass:
				if scatter(seed, goldSalt, int64(x), int64(y))%goldScatter == 0 {
					nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Gold, Amount: resource.GoldAmount})
				}
			case terrain.Sand:
				if scatter(seed, oilSalt, int64(x), int64(y))%oilScatter == 0 {
					nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Oil, Amount: resource.OilAmount})
				}
			}
		}
	}
	for _, b := range berries {
		nodes = append(nodes, Node{X: int32(b[0]), Y: int32(b[1]), Kind: resource.Food, Amount: resource.BerryAmount})
	}
	return nodes
}

// hashMap digests the full map state: layout, terrain, heights, spawns and nodes.
func hashMap(m *Map) uint64 {
	h := hash.New()
	h.WriteString(hashLabel)
	size := m.grid.Size()
	h.WriteU32(uint32(size))
	h.WriteU32(uint32(len(m.spawns)))
	for _, s := range m.spawns {
		h.WriteU32(uint32(s[0]))
		h.WriteU32(uint32(s[1]))
	}
	h.WriteU32(uint32(len(m.nodes)))
	for _, n := range m.nodes {
		h.WriteU32(uint32(n.X))
		h.WriteU32(uint32(n.Y))
		h.WriteU32(uint32(n.Kind))
		h.WriteU32(uint32(n.Amount))
	}
	for _, t := range m.grid.Tiles() {
		h.WriteU32(uint32(t))
	}
	for _, v := range m.grid.Vertices() {
		h.WriteU32(uint32(v))
	}
	return h.Sum()
}
