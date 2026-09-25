// Package worldgen produces one deterministic terrain map from a single
// seed shared by all players.
//
// The generator is pure integer math: splitmix64-hashed value noise in
// fixed point, integer classification thresholds and a fixed-order spawn
// selection, so every platform and every build agrees on every byte.
//
// Balance guarantees:
//   - plains dominate; rock only falls inside mountain-mask regions, so
//     mountains cluster into localized chains instead of speckle;
//   - every tile carries one biome from data/biomes.json; biomes come
//     from smooth temperature/moisture fields plus a distance-to-water
//     flood, so regions are contiguous with gradual borders, and every
//     map has at least three biomes;
//   - forests clump with clearings and grow the biome's dominant tree;
//   - every spawn sits on a flat (vertex spread <= spawnFlatMax) tile
//     inside the largest walkable land component, with a grass disk, a
//     starter ring of birch forest and rock corners at identical
//     offsets, and minimum pairwise separation. One land component plus
//     water-only obstacles means every spawn reaches every other.
package worldgen

import (
	"errors"
	"sort"

	"rts/sim/biome"
	"rts/sim/fixed"
	"rts/sim/hash"
	"rts/sim/resource"
	"rts/sim/terrain"
	"rts/sim/tree"
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
	rockLevel   int32 = 42

	// forestClear is the clearing-noise floor: forest tiles need a solid
	// forest signal, holes appear where the noise dips (B5 clumps with
	// clearings). prairieClear keeps the prairie nearly treeless.
	forestClear  int32 = 20
	prairieClear int32 = 52

	// mountainMask gates all rock inside localized mountain regions
	// (T2); noise above the threshold marks the chain belt.
	mountainMask int32 = 40

	// Biome classification thresholds on 0..63 fields.
	taigaTemp      int32 = 18
	savannaTemp    int32 = 46
	coastTemp      int32 = 26
	coastWaterR    int   = 4
	temperateMoist       = 32

	spawnMargin  = 14
	spawnMinDist = 48
	spawnLandMin = 25 // walkable tiles in the 5x5 spawn window (of 25)
	spawnWindow  = 5
	// spawnFlatMax is the largest vertex-height spread (max-min) allowed
	// in the 7x7 corner area around a spawn (T3).
	spawnFlatMax int32 = 8

	goldScatter   = 90  // one gold node per N grass tiles (hash bucket)
	oilScatter    = 120 // one oil node per N sand tiles (hidden until Modern)
	palmScatter   = 6   // one palm per N coast sand tiles
	baobabScatter = 34  // one baobab per N savanna grass tiles

	berrySalt         = 0x1f4a7c93e2b5d617
	goldSalt          = 0x52aa1c3d7e99f011
	oilSalt           = 0x0c7e1d5b3a8f6249
	palmSalt          = 0x73b8e1c0a9d4f265
	baobabSalt        = 0x2e5c9a3f7b1d8e04
	minBiomes         = 3
	spawnTreeSp uint8 = tree.Birch // starter ring species for every player

	// WildZone keeps aggressive wildlife (boar, wolf, black wolf, bear)
	// this many tiles from every spawn: safe starter grounds, deadly
	// wilderness.
	WildZone = 60

	// RelicClear keeps aggressive dens this many tiles from every relic:
	// a leash-bound wolf (leash 14) or bear (leash 20) can never wander
	// into the relic cluster, and contesting the center stays a journey,
	// not a siege of a den.
	RelicClear = 40

	hashLabel = "rts.map/v6"
)

// ErrNoSpawns means the map does not contain enough separated land patches
// for the requested number of players.
var ErrNoSpawns = errors.New("worldgen: not enough land for requested players")

// Node is a harvestable resource node placed by the generator. Tree is
// the species id for wood and palm nodes (see sim/tree), 0 otherwise.
type Node struct {
	X, Y   int32
	Kind   resource.Kind
	Tree   uint8
	Amount int32
}

// Animal is a wild unit spawn point; Def is the data/units.json unit id
// (deer, boar). Animals are created as entities by the world, not the map.
type Animal struct {
	X, Y int32
	Def  uint16
}

// Animal unit def ids mirrored from data/units.json.
const (
	DefDeer      uint16 = 5
	DefBoar      uint16 = 6
	DefRabbit    uint16 = 9
	DefSheep     uint16 = 10
	DefBear      uint16 = 11
	DefWolf      uint16 = 12
	DefBlackWolf uint16 = 13
)

// RelicCount is the number of victory relics placed near the map center.
const RelicCount = 5

// Map is a generated world map.
type Map struct {
	grid    *terrain.Grid
	biomes  []uint8
	spawns  [][2]int
	nodes   []Node
	animals []Animal
	relics  [RelicCount][2]int32
	digest  uint64
}

// Grid exposes the terrain grid.
func (m *Map) Grid() *terrain.Grid { return m.grid }

// Biomes returns the row-major biome byte per tile (sim/biome ids).
func (m *Map) Biomes() []uint8 { return m.biomes }

// BiomeAt returns the biome id of a tile.
func (m *Map) BiomeAt(x, y int) uint8 {
	size := m.grid.Size()
	return m.biomes[y*size+x]
}

// Spawns returns the player start positions in order.
func (m *Map) Spawns() [][2]int { return m.spawns }

// Nodes returns the harvestable nodes in deterministic (row-major, then
// per-player berry rings) order.
func (m *Map) Nodes() []Node { return m.nodes }

// Animals returns the wild animal spawn points in deterministic order.
func (m *Map) Animals() []Animal { return m.animals }

// Relics returns the relic tile positions in deterministic order.
func (m *Map) Relics() [RelicCount][2]int32 { return m.relics }

// Hash returns the canonical map digest.
func (m *Map) Hash() uint64 { return m.digest }

// Custom wraps a hand-made grid, spawns and nodes into a Map (tests and
// scenario tools). All tiles are temperate. The digest is computed like a
// generated map's.
func Custom(grid *terrain.Grid, spawns [][2]int, nodes []Node) *Map {
	return CustomWithAnimals(grid, spawns, nodes, nil)
}

// CustomWithAnimals is Custom with explicit wild animal spawn points.
func CustomWithAnimals(grid *terrain.Grid, spawns [][2]int, nodes []Node, animals []Animal) *Map {
	return CustomWithBiomes(grid, spawns, nodes, animals, nil)
}

// CustomWithBiomes is CustomWithAnimals with an explicit row-major biome
// layer; a nil layer means all-temperate.
func CustomWithBiomes(grid *terrain.Grid, spawns [][2]int, nodes []Node, animals []Animal, biomes []uint8) *Map {
	m := &Map{grid: grid, spawns: spawns, nodes: nodes, animals: animals}
	m.biomes = biomes
	if m.biomes == nil {
		m.biomes = make([]uint8, grid.Size()*grid.Size())
	}
	m.relics = placeRelics(grid)
	m.digest = hashMap(m)
	return m
}

// CustomWithRelics is CustomWithAnimals with explicit relic tiles.
func CustomWithRelics(grid *terrain.Grid, spawns [][2]int, nodes []Node, animals []Animal, relics [][2]int32) *Map {
	m := CustomWithAnimals(grid, spawns, nodes, animals)
	for i, r := range relics {
		if i >= RelicCount {
			break
		}
		m.relics[i] = r
	}
	m.digest = hashMap(m)
	return m
}

// Generate builds a map for the given seed, size (tiles per side) and
// player count.
func Generate(seed uint64, size, players int) (*Map, error) {
	if size < spawnMargin*4 || players < 1 {
		return nil, errors.New("worldgen: map too small")
	}
	bdefs := biome.MustLoad()
	tdefs := tree.MustLoad()
	g := terrain.NewGrid(size)

	for y := int64(0); y <= int64(size); y++ {
		for x := int64(0); x <= int64(size); x++ {
			h := heightAt(seed, x, y)
			if b := edgeRise(int(x), int(y), size); b > 0 {
				h += b
			}
			g.SetVertex(int(x), int(y), h)
		}
	}

	// The biome layer precedes terrain so forest density can follow the
	// biome (prairies stay nearly treeless).
	biomes := classifyBiomes(seed, g)
	enforceBiomeMin(biomes, size)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			g.SetTile(x, y, classify(seed, g, x, y, biomes[y*size+x]))
		}
	}

	spawns, err := placeSpawns(g, players)
	if err != nil {
		return nil, err
	}
	ringTrees := make(map[[2]int]bool, len(spawns)*8)
	for _, s := range spawns {
		for _, t := range stampStart(g, s[0], s[1]) {
			ringTrees[t] = true
		}
	}
	berries := stampBerries(g, spawns)
	nodes := placeNodes(g, seed, berries, ringTrees, biomes, bdefs, tdefs)
	relics := placeRelics(g)
	animals := placeAnimals(g, spawns, relics)

	m := &Map{grid: g, biomes: biomes, spawns: spawns, nodes: nodes, animals: animals}
	m.relics = relics
	m.digest = hashMap(m)
	return m, nil
}

// classifyBiomes floods a distance-to-water field from every future water
// tile (land below waterLevel at its corner mean — the same rule classify
// will use) and classifies each tile from temperature (latitude ramp plus
// large-scale noise), moisture and that distance. Smooth input fields keep
// biome borders contiguous and gradual.
func classifyBiomes(seed uint64, g *terrain.Grid) []uint8 {
	size := g.Size()
	near := distanceToWater(g)
	out := make([]uint8, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			out[y*size+x] = classifyBiome(seed, size, x, y, near[y*size+x])
		}
	}
	return out
}

// classifyBiome maps one tile's 0..63 fields to a biome id.
func classifyBiome(seed uint64, size, x, y, nearWater int) uint8 {
	temp := tempAt(seed, size, x, y)
	if nearWater >= 0 && nearWater <= coastWaterR && temp >= coastTemp {
		return biome.Coast
	}
	if temp < taigaTemp {
		return biome.Taiga
	}
	moist := moistureAt(seed, int64(x), int64(y))
	if temp >= savannaTemp && moist <= temperateMoist {
		return biome.Savanna
	}
	if moist > temperateMoist {
		return biome.Temperate
	}
	return biome.Prairie
}

// enforceBiomeMin guarantees at least minBiomes distinct biomes by
// recoloring the map's cold north and warm south bands when a seed
// collapsed them out.
func enforceBiomeMin(biomes []uint8, size int) {
	seen := make([]bool, biome.Count)
	for _, b := range biomes {
		if int(b) < biome.Count {
			seen[b] = true
		}
	}
	n := 0
	for _, s := range seen {
		if s {
			n++
		}
	}
	if n >= minBiomes {
		return
	}
	for i := range biomes {
		y := i / size
		if y < size/6 {
			biomes[i] = biome.Taiga
		} else if y >= 5*size/6 {
			biomes[i] = biome.Savanna
		}
	}
}

// tempAt returns a 0..63 temperature: a latitude ramp (cold at y=0) plus
// low-frequency noise so borders wiggle instead of following rows.
func tempAt(seed uint64, size, x, y int) int32 {
	base := int32(8 + (48*y)/size)
	n := int32(valueNoise(seed^0x51b7f3a9c2d8e601, 6, int64(x), int64(y), 48)) - 32
	t := base + n
	if t < 0 {
		return 0
	}
	if t > 63 {
		return 63
	}
	return t
}

// distanceToWater runs one deterministic multi-source BFS (row-major seed
// order) from every tile whose corner-height mean sits below waterLevel
// (the future water tiles), so the biome layer can be built before tile
// classification. Water tiles get 0, land tiles get the walking distance
// to the nearest water, capped at coastWaterR+1. A map with no water
// yields -1 everywhere.
func distanceToWater(g *terrain.Grid) []int {
	size := g.Size()
	near := make([]int, size*size)
	for i := range near {
		near[i] = -1
	}
	isWater := func(x, y int) bool {
		mean := (g.Vertex(x, y) + g.Vertex(x+1, y) + g.Vertex(x, y+1) + g.Vertex(x+1, y+1)) / 4
		return mean < waterLevel
	}
	var queue []int
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if isWater(x, y) {
				near[y*size+x] = 0
				queue = append(queue, y*size+x)
			}
		}
	}
	if len(queue) == 0 {
		return near
	}
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		d := near[i]
		if d >= coastWaterR+1 {
			continue
		}
		x, y := i%size, i/size
		for _, o := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := x+o[0], y+o[1]
			if nx < 0 || ny < 0 || nx >= size || ny >= size {
				continue
			}
			j := ny*size + nx
			if near[j] == -1 {
				near[j] = d + 1
				queue = append(queue, j)
			}
		}
	}
	return near
}

// placeRelics puts the five victory relics in a diamond around the map
// center, each snapped to the nearest walkable open ground.
func placeRelics(g *terrain.Grid) [RelicCount][2]int32 {
	size := g.Size()
	cx, cy := size/2, size/2
	seeds := [RelicCount][2]int32{{0, 0}, {4, 0}, {-4, 0}, {0, 4}, {0, -4}}
	var out [RelicCount][2]int32
	for i, s := range seeds {
		x, y := snapWalkable(g, int(s[0])+cx, int(s[1])+cy)
		out[i] = [2]int32{int32(x), int32(y)}
	}
	return out
}

// snapWalkable returns the nearest tile to (x, y) that units can walk
// on, scanning rings in a fixed order so every seed snaps identically.
func snapWalkable(g *terrain.Grid, x, y int) (int, int) {
	if g.InBounds(x, y) && g.Tile(x, y).Walkable() {
		return x, y
	}
	for r := 1; r < g.Size(); r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx != -r && dx != r && dy != -r && dy != r {
					continue
				}
				nx, ny := x+dx, y+dy
				if g.InBounds(nx, ny) && g.Tile(nx, ny).Walkable() {
					return nx, ny
				}
			}
		}
	}
	return x, y
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

// edgeRise lifts vertex heights toward the map border so land rings the
// whole map, water pools inland and every land region touches the border
// band, which keeps one dominant walkable component (T4) and land (T1).
func edgeRise(x, y, size int) int32 {
	d := x
	if y < d {
		d = y
	}
	if size-x < d {
		d = size - x
	}
	if size-y < d {
		d = size - y
	}
	const band = 14
	if d >= band {
		return 0
	}
	return int32((band - d) * 2)
}

// mountainAt reports whether a tile sits inside the localized mountain
// mask; rock may only fall on mask tiles, which clumps mountains into
// chains and massifs instead of speckle (T2).
func mountainAt(seed uint64, x, y int64) bool {
	return valueNoise(seed^0x6a1c48e2d93b57af, 5, x, y, 36) >= int64(mountainMask)
}

// classify derives the tile terrain from its corner heights, moisture and
// biome. Forest needs both a humid signal and an unbroken forest-noise
// patch (clearings appear where the noise dips); the prairie biome raises
// the noise floor so it stays nearly treeless.
func classify(seed uint64, g *terrain.Grid, x, y int, b uint8) terrain.Type {
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
	case mean >= rockLevel && mountainAt(seed, int64(x), int64(y)):
		return terrain.Rock
	case slope > 10 && mean > sandLevel && mountainAt(seed, int64(x), int64(y)):
		return terrain.Rock
	}

	clear := int32(valueNoise(seed^0x3d9f2c61ae8b5017, 4, int64(x), int64(y), 7))
	floor := forestClear
	if b == biome.Prairie {
		floor = prairieClear
	}
	if moistureAt(seed, int64(x), int64(y)) > int64(forestMoist) &&
		clear > floor && mean < forestMaxH {
		return terrain.Forest
	}
	return terrain.Grass
}

type spawnCandidate struct {
	score int
	index int
	x, y  int
}

// landComponents labels every land (non-water) tile with its connected
// component id and returns the id of the largest component. Units can
// walk on rock, so only water splits the land.
func landComponents(g *terrain.Grid) (labels []int32, big int32) {
	size := g.Size()
	labels = make([]int32, size*size)
	for i := range labels {
		labels[i] = -1
	}
	big, bigN := int32(-1), 0
	ncomp := int32(0)
	queue := make([]int, 0, size*size)
	for start := 0; start < len(labels); start++ {
		if labels[start] != -1 || g.Tiles()[start].Water() {
			continue
		}
		n := 0
		queue = append(queue[:0], start)
		labels[start] = ncomp
		for head := 0; head < len(queue); head++ {
			i := queue[head]
			n++
			x, y := i%size, i/size
			for _, o := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+o[0], y+o[1]
				if nx < 0 || ny < 0 || nx >= size || ny >= size {
					continue
				}
				j := ny*size + nx
				if labels[j] == -1 && !g.Tiles()[j].Water() {
					labels[j] = ncomp
					queue = append(queue, j)
				}
			}
		}
		if n > bigN {
			bigN, big = n, ncomp
		}
		ncomp++
	}
	return labels, big
}

// flatArea reports the vertex height spread (max-min) over the 7x7 corner
// area around a spawn candidate.
func flatArea(g *terrain.Grid, x, y int) int32 {
	hi, lo := int32(0), int32(127)
	for vy := y - 3; vy <= y+4; vy++ {
		for vx := x - 3; vx <= x+4; vx++ {
			if vx < 0 || vy < 0 || vx > g.Size() || vy > g.Size() {
				continue
			}
			v := g.Vertex(vx, vy)
			if v > hi {
				hi = v
			}
			if v < lo {
				lo = v
			}
		}
	}
	return hi - lo
}

// placeSpawns picks player starts deterministically: candidates are scored
// by walkable tiles inside the spawn window and sorted best first (ties by
// row-major order). Only tiles on flat ground inside the largest land
// component qualify, so every start is comparable and mutually reachable.
// The first start is the best candidate; each following start is the
// candidate maximizing the distance to the already chosen ones (ties by
// sort order), so starts spread out over equal-quality land.
func placeSpawns(g *terrain.Grid, players int) ([][2]int, error) {
	size := g.Size()
	labels, big := landComponents(g)
	var candidates []spawnCandidate
	for y := spawnMargin; y < size-spawnMargin; y++ {
		for x := spawnMargin; x < size-spawnMargin; x++ {
			if flatArea(g, x, y) > spawnFlatMax {
				continue
			}
			if !inComponent(labels, size, big, x, y) {
				continue
			}
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

// inComponent reports whether the whole spawn window sits in component big.
func inComponent(labels []int32, size int, big int32, x, y int) bool {
	r := spawnWindow / 2
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if labels[(y+dy)*size+x+dx] != big {
				return false
			}
		}
	}
	return true
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
// Disk vertices are averaged so the building zone is flat no matter what
// terrain was there (T3). The ring tiles are returned as the starter-tree
// positions: every player always gets fast-chop birch around the base.
func stampStart(g *terrain.Grid, x, y int) [][2]int {
	flattenDisk(g, x, y)
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			if dx*dx+dy*dy < 9 {
				g.SetTile(x+dx, y+dy, terrain.Grass)
			}
		}
	}
	var ring [][2]int
	for _, o := range [][2]int{{3, 0}, {-3, 0}, {0, 3}, {0, -3}, {3, 3}, {3, -3}, {-3, 3}, {-3, -3}} {
		g.SetTile(x+o[0], y+o[1], terrain.Forest)
		ring = append(ring, [2]int{x + o[0], y + o[1]})
	}
	for _, o := range [][2]int{{5, 5}, {-5, 5}, {5, -5}, {-5, -5}} {
		g.SetTile(x+o[0], y+o[1], terrain.Rock)
	}
	return ring
}

// flattenDisk averages the vertex heights inside the spawn disk (radius 4
// tile corners) to their integer mean.
func flattenDisk(g *terrain.Grid, x, y int) {
	var sum, n int32
	var idx [][2]int
	for vy := y - 4; vy <= y+5; vy++ {
		for vx := x - 4; vx <= x+5; vx++ {
			dx, dy := vx-x, vy-y
			if dx*dx+dy*dy > 17 {
				continue
			}
			if vx < 0 || vy < 0 || vx > g.Size() || vy > g.Size() {
				continue
			}
			sum += g.Vertex(vx, vy)
			n++
			idx = append(idx, [2]int{vx, vy})
		}
	}
	if n == 0 {
		return
	}
	mean := sum / n
	for _, v := range idx {
		g.SetVertex(v[0], v[1], mean)
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

// placeNodes turns forest tiles into wood nodes growing the biome's
// dominant species (starter rings force birch), coast sand into palms,
// savanna grass into scattered baobabs, rock tiles into stone nodes, and
// sprinkles gold on grass and oil on sand by hash bucket.
func placeNodes(g *terrain.Grid, seed uint64, berries [][2]int, ringTrees map[[2]int]bool,
	biomes []uint8, bdefs *biome.Table, tdefs *tree.Table) []Node {
	size := g.Size()
	var nodes []Node
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			bi := bdefs.Get(biomes[y*size+x])
			switch g.Tile(x, y) {
			case terrain.Forest:
				sp := bi.Tree
				if ringTrees[[2]int{x, y}] {
					sp = spawnTreeSp
				}
				if sp == tree.None {
					sp = tree.Spruce
				}
				td := tdefs.Get(sp)
				nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Wood, Tree: sp, Amount: td.Wood})
			case terrain.Rock:
				nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Stone, Amount: resource.StoneAmount})
			case terrain.Grass:
				switch {
				case scatter(seed, goldSalt, int64(x), int64(y))%goldScatter == 0:
					nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Gold, Amount: resource.GoldAmount})
				case bi.ID == biome.Savanna &&
					scatter(seed, baobabSalt, int64(x), int64(y))%baobabScatter == 0:
					td := tdefs.Get(tree.Baobab)
					nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Wood, Tree: tree.Baobab, Amount: td.Wood})
				}
			case terrain.Sand:
				switch {
				case bi.ID == biome.Coast &&
					scatter(seed, palmSalt, int64(x), int64(y))%palmScatter == 0:
					td := tdefs.Get(tree.Palm)
					nodes = append(nodes, Node{X: int32(x), Y: int32(y), Kind: resource.Food, Tree: tree.Palm, Amount: td.Food})
				case scatter(seed, oilSalt, int64(x), int64(y))%oilScatter == 0:
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

// placeAnimals scatters species around each spawn by habitat: deer in
// forest, sheep on plain, rabbit on plain or forest — the neighborhood
// larder. Everything with teeth (boar, wolf, black wolf, bear) lives
// beyond WildZone tiles from every spawn: the starting economy stays a
// hunting ground, the wilds beyond stay deadly. Scans are radius-band
// row-major, so placement is deterministic without touching the RNG.
func placeAnimals(g *terrain.Grid, spawns [][2]int, relics [RelicCount][2]int32) []Animal {
	var out []Animal
	taken := map[[2]int]bool{}
	place := func(s [2]int, def uint16, want, minR, maxR, minSpawnDist, minRelicDist int, types ...terrain.Type) {
		done := 0
		for r := minR; r <= maxR && done < want; r++ {
			for dy := -r; dy <= r && done < want; dy++ {
				for dx := -r; dx <= r && done < want; dx++ {
					if iabs(dx) != r && iabs(dy) != r {
						continue
					}
					x, y := s[0]+dx, s[1]+dy
					if !g.InBounds(x, y) || taken[[2]int{x, y}] {
						continue
					}
					if !oneOf(g.Tile(x, y), types) {
						continue
					}
					if minSpawnDist > 0 {
						ok := true
						for _, s2 := range spawns {
							if iabs(x-s2[0]) < minSpawnDist || iabs(y-s2[1]) < minSpawnDist {
								ok = false
								break
							}
						}
						if !ok {
							continue
						}
					}
					if minRelicDist > 0 {
						ok := true
						for _, rel := range relics {
							if iabs(x-int(rel[0])) < minRelicDist || iabs(y-int(rel[1])) < minRelicDist {
								ok = false
								break
							}
						}
						if !ok {
							continue
						}
					}
					taken[[2]int{x, y}] = true
					out = append(out, Animal{X: int32(x), Y: int32(y), Def: def})
					done++
				}
			}
		}
	}
	wood := []terrain.Type{terrain.Forest}
	grass := []terrain.Type{terrain.Grass}
	open := []terrain.Type{terrain.Grass, terrain.Forest}
	const wildZones = WildZone
	for _, s := range spawns {
		place(s, DefDeer, 3, 8, 15, 0, 0, wood...)
		place(s, DefSheep, 2, 6, 13, 0, 0, grass...)
		place(s, DefRabbit, 2, 6, 13, 0, 0, open...)
		place(s, DefBoar, 1, 60, 90, wildZones, RelicClear, wood...)
		place(s, DefWolf, 1, 64, 100, wildZones, RelicClear, wood...)
		place(s, DefBlackWolf, 1, 70, 110, wildZones, RelicClear, wood...)
	}
	return append(out, placeBears(g, spawns, relics)...)
}

// placeBears dens one bear per rock massif: the first rock tile at least
// WildZone tiles (Chebyshev) from every spawn and 12 from any other
// bear, capped at three per map — mountains stay rare and deadly.
func placeBears(g *terrain.Grid, spawns [][2]int, relics [RelicCount][2]int32) []Animal {
	const maxBears = 3
	var out []Animal
	for y := 0; y < g.Size(); y++ {
		for x := 0; x < g.Size(); x++ {
			if len(out) == maxBears {
				return out
			}
			if g.Tile(x, y) != terrain.Rock {
				continue
			}
			ok := true
			for _, s := range spawns {
				if iabs(x-s[0]) < WildZone || iabs(y-s[1]) < WildZone {
					ok = false
					break
				}
			}
			for _, rel := range relics {
				if ok && (iabs(x-int(rel[0])) < RelicClear || iabs(y-int(rel[1])) < RelicClear) {
					ok = false
					break
				}
			}
			for _, b := range out {
				if ok && iabs(x-int(b.X)) < 12 && iabs(y-int(b.Y)) < 12 {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, Animal{X: int32(x), Y: int32(y), Def: DefBear})
			}
		}
	}
	return out
}

func oneOf(t terrain.Type, types []terrain.Type) bool {
	for _, k := range types {
		if k == t {
			return true
		}
	}
	return false
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// hashMap digests the full map state: layout, terrain, biomes, heights,
// spawns and nodes.
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
		h.WriteU32(uint32(n.Tree))
		h.WriteU32(uint32(n.Amount))
	}
	h.WriteU32(uint32(len(m.animals)))
	for _, a := range m.animals {
		h.WriteU32(uint32(a.X))
		h.WriteU32(uint32(a.Y))
		h.WriteU32(uint32(a.Def))
	}
	for _, t := range m.grid.Tiles() {
		h.WriteU32(uint32(t))
	}
	for _, b := range m.biomes {
		h.WriteU32(uint32(b))
	}
	for _, v := range m.grid.Vertices() {
		h.WriteU32(uint32(v))
	}
	return h.Sum()
}
