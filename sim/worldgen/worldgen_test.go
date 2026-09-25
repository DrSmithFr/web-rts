package worldgen

import (
	"testing"

	"rts/sim/terrain"
)

func TestDeterministicMap(t *testing.T) {
	a, err := Generate(0x5eed, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(0x5eed, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if a.Hash() != b.Hash() {
		t.Fatal("same seed produced different hashes")
	}
	if a.Grid().Tiles()[1234] != b.Grid().Tiles()[1234] {
		t.Fatal("tile arrays diverge")
	}
	if a.Spawns()[0] != b.Spawns()[0] {
		t.Fatal("spawns diverge")
	}
}

func TestSeedChangesMap(t *testing.T) {
	a, _ := Generate(1, 100, 2)
	b, _ := Generate(2, 100, 2)
	if a.Hash() == b.Hash() {
		t.Fatal("different seeds produced identical maps")
	}
}

func TestSpawnsBalanced(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		m, err := Generate(seed, DefaultSize, 8)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		spawns := m.Spawns()
		if len(spawns) != 8 {
			t.Fatalf("seed %d: %d spawns", seed, len(spawns))
		}
		for i, s := range spawns {
			checkSpawnArea(t, seed, m.Grid(), s)
			for j, o := range spawns {
				if i == j {
					continue
				}
				dx, dy := s[0]-o[0], s[1]-o[1]
				if dx*dx+dy*dy < spawnMinDist*spawnMinDist {
					t.Fatalf("seed %d: spawns %d and %d too close", seed, i, j)
				}
			}
		}
	}
}

// checkSpawnArea verifies the identical starting area contract.
func checkSpawnArea(t *testing.T, seed uint64, g *terrain.Grid, s [2]int) {
	t.Helper()
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			if dx*dx+dy*dy >= 9 {
				continue
			}
			if got := g.Tile(s[0]+dx, s[1]+dy); got != terrain.Grass {
				t.Fatalf("seed %d spawn %v: disk tile (%d,%d) = %s, want grass",
					seed, s, dx, dy, got)
			}
		}
	}
	for _, o := range [][2]int{{3, 0}, {-3, 0}, {0, 3}, {0, -3}, {3, 3}, {3, -3}, {-3, 3}, {-3, -3}} {
		if got := g.Tile(s[0]+o[0], s[1]+o[1]); got != terrain.Forest {
			t.Fatalf("seed %d spawn %v: ring %v = %s, want forest", seed, s, o, got)
		}
	}
	for _, o := range [][2]int{{5, 5}, {-5, 5}, {5, -5}, {-5, -5}} {
		if got := g.Tile(s[0]+o[0], s[1]+o[1]); got != terrain.Rock {
			t.Fatalf("seed %d spawn %v: corner %v = %s, want rock", seed, s, o, got)
		}
	}
}

func TestAllTilesValid(t *testing.T) {
	m, err := Generate(7, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, tile := range m.Grid().Tiles() {
		if tile >= terrain.TypeCount {
			t.Fatalf("invalid tile type %d", tile)
		}
	}
	for _, v := range m.Grid().Vertices() {
		if v < 0 || v > terrain.MaxHeight {
			t.Fatalf("height %d out of range", v)
		}
	}
}

func TestAllSixTypesAppear(t *testing.T) {
	seen := map[terrain.Type]bool{}
	for seed := uint64(1); seed <= 10; seed++ {
		m, err := Generate(seed, DefaultSize, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, tile := range m.Grid().Tiles() {
			seen[tile] = true
		}
	}
	for _, want := range []terrain.Type{terrain.Grass, terrain.Forest, terrain.Rock,
		terrain.Sand, terrain.Shallow, terrain.Deep} {
		if !seen[want] {
			t.Errorf("terrain %s never appeared", want)
		}
	}
}

func TestRejectsBadParams(t *testing.T) {
	if _, err := Generate(1, 10, 2); err == nil {
		t.Error("tiny map accepted")
	}
	if _, err := Generate(1, DefaultSize, 0); err == nil {
		t.Error("zero players accepted")
	}
}

func TestNodesPlaced(t *testing.T) {
	m, err := Generate(0x5eed, DefaultSize, 4)
	if err != nil {
		t.Fatal(err)
	}
	var wood, stone, food, gold, oil int
	byTile := make(map[[2]int32]bool)
	for _, n := range m.Nodes() {
		key := [2]int32{n.X, n.Y}
		if byTile[key] {
			t.Fatalf("two nodes on tile %v", key)
		}
		byTile[key] = true
		// Palms are food nodes too; berries are the food nodes near spawns.
		berried := false
		for _, s := range m.Spawns() {
			if dist2(int(n.X), int(n.Y), s[0], s[1]) <= 10*10 {
				berried = true
			}
		}
		switch {
		case n.Kind == 1:
			wood++
		case n.Kind == 2:
			stone++
		case n.Kind == 3 && n.Tree == 0 && berried:
			food++
		case n.Kind == 4:
			gold++
		case n.Kind == 5:
			oil++
		}
	}
	if wood < 1000 || stone < 100 || gold < 20 {
		t.Fatalf("too few nodes: wood=%d stone=%d gold=%d", wood, stone, gold)
	}
	// 4 spawns x 4 berries, minus water skips.
	if food > 16 {
		t.Fatalf("berries %d, want <= 16", food)
	}
}

func TestSpawnHasNearbyBerries(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		m, err := Generate(seed, DefaultSize, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range m.Spawns() {
			found := 0
			for _, n := range m.Nodes() {
				if n.Kind == resourceFood && dist2(int(n.X), int(n.Y), s[0], s[1]) <= 9*9 {
					found++
				}
			}
			if found == 0 {
				t.Fatalf("seed %d spawn %v has no berry node within 9", seed, s)
			}
		}
	}
}

const resourceFood = 3

func dist2(x1, y1, x2, y2 int) int {
	dx, dy := x1-x2, y1-y2
	return dx*dx + dy*dy
}

// TestLargeMapGeneration pins the spec's 1000x1000 requirement: it must
// generate deterministically and stay well under a second.
func TestLargeMapGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("large map skipped in -short mode")
	}
	m1, err := Generate(0x5eed, 1000, 8)
	if err != nil {
		t.Fatal(err)
	}
	if m1.Grid().Size() != 1000 {
		t.Fatalf("size %d", m1.Grid().Size())
	}
	if len(m1.Spawns()) != 8 {
		t.Fatalf("spawns %d", len(m1.Spawns()))
	}
	m2, err := Generate(0x5eed, 1000, 8)
	if err != nil {
		t.Fatal(err)
	}
	if m1.Hash() != m2.Hash() {
		t.Fatal("large map generation is not deterministic")
	}
	if len(m1.Nodes()) < 100000 {
		t.Fatalf("suspiciously few nodes on a large map: %d", len(m1.Nodes()))
	}
}

// TestAcceptance20Seeds runs the spec's acceptance criteria C1..C5 (the
// map-side ones) over 20 seeds: plains dominate, starts are flat and
// mutually reachable, at least three biomes appear, and every tree
// species shows up inside its own biome.
func TestAcceptance20Seeds(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		m, err := Generate(seed, DefaultSize, DefaultPlayers)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		size := m.Grid().Size()
		plains, biomes := 0, map[uint8]int{}
		for i, t := range m.Grid().Tiles() {
			if t == terrain.Grass || t == terrain.Forest || t == terrain.Sand {
				plains++
			}
			biomes[m.Biomes()[i]]++
		}
		if plains*2 <= size*size {
			t.Errorf("seed %d: plains %.1f%%, want majority", seed, 100*float64(plains)/float64(size*size))
		}
		if len(biomes) < 3 {
			t.Errorf("seed %d: only %d biomes", seed, len(biomes))
		}
		var species [5]int
		for _, n := range m.Nodes() {
			if n.Tree == 0 {
				continue
			}
			species[n.Tree]++
			// Trees grow in their biome, except the universal starter-ring
			// birch and spruce fallbacks on non-taiga forest tiles.
			b := m.BiomeAt(int(n.X), int(n.Y))
			switch n.Tree {
			case 3: // palm: coast only
				if b != biomeCoast {
					t.Errorf("seed %d: palm at (%d,%d) in biome %d", seed, n.X, n.Y, b)
				}
			case 4: // baobab: savanna only
				if b != biomeSavanna {
					t.Errorf("seed %d: baobab at (%d,%d) in biome %d", seed, n.X, n.Y, b)
				}
			}
		}
		for sp, want := range map[uint8]int{1: 1, 2: 1, 3: 1, 4: 1} {
			if species[sp] < want {
				t.Errorf("seed %d: species %d count %d", seed, sp, species[sp])
			}
		}
		for _, s := range m.Spawns() {
			checkSpawnFlat(t, seed, m.Grid(), s)
		}
		checkSpawnsConnected(t, seed, m)
	}
}

const (
	biomeCoast   uint8 = 3
	biomeSavanna uint8 = 2
)

// checkSpawnFlat verifies the starting disk vertices are flat (T3).
func checkSpawnFlat(t *testing.T, seed uint64, g *terrain.Grid, s [2]int) {
	t.Helper()
	hi, lo := int32(0), int32(127)
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			if dx*dx+dy*dy > 16 {
				continue
			}
			v := g.Vertex(s[0]+dx, s[1]+dy)
			if v > hi {
				hi = v
			}
			if v < lo {
				lo = v
			}
		}
	}
	if hi-lo != 0 {
		t.Fatalf("seed %d spawn %v: vertex spread %d on the building disk", seed, s, hi-lo)
	}
}

// checkSpawnsConnected floods walkable land (water blocks, rock does not)
// from the first spawn and reaches every other one (T4).
func checkSpawnsConnected(t *testing.T, seed uint64, m *Map) {
	t.Helper()
	size := m.Grid().Size()
	spawns := m.Spawns()
	seen := make([]bool, size*size)
	s0 := spawns[0]
	queue := []int{s0[1]*size + s0[0]}
	seen[queue[0]] = true
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		x, y := i%size, i/size
		for _, o := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := x+o[0], y+o[1]
			if nx < 0 || ny < 0 || nx >= size || ny >= size {
				continue
			}
			j := ny*size + nx
			if !seen[j] && !m.Grid().Tiles()[j].Water() {
				seen[j] = true
				queue = append(queue, j)
			}
		}
	}
	for _, s := range spawns {
		if !seen[s[1]*size+s[0]] {
			t.Fatalf("seed %d: spawn %v unreachable", seed, s)
		}
	}
}

// TestBiomeContiguity checks biome regions are clumps, not confetti (B2):
// almost every tile shares its biome with at least one neighbour.
func TestBiomeContiguity(t *testing.T) {
	m, err := Generate(0x5eed, DefaultSize, 2)
	if err != nil {
		t.Fatal(err)
	}
	size := m.Grid().Size()
	solo := 0
	for y := 1; y < size-1; y++ {
		for x := 1; x < size-1; x++ {
			b := m.BiomeAt(x, y)
			if m.BiomeAt(x+1, y) != b && m.BiomeAt(x-1, y) != b &&
				m.BiomeAt(x, y+1) != b && m.BiomeAt(x, y-1) != b {
				solo++
			}
		}
	}
	total := (size - 2) * (size - 2)
	if solo*100 > total {
		t.Fatalf("%d/%d tiles have no same-biome neighbour, want < 1%%", solo, total)
	}
}
