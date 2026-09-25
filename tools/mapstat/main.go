// Command mapstat prints terrain, biome and species statistics for
// tuning worldgen thresholds, and with -accept runs the map-side
// acceptance criteria (C1..C5) over the given number of seeds.
//
// Usage:
//
//	mapstat [seeds]           statistics over seeds 1..N
//	mapstat -accept [seeds]   pass/fail acceptance gate (exit 1 on failure)
package main

import (
	"flag"
	"fmt"
	"os"

	"rts/sim/biome"
	"rts/sim/terrain"
	"rts/sim/tree"
	"rts/sim/worldgen"
)

func main() {
	accept := flag.Bool("accept", false, "run the C1..C5 acceptance gate")
	size := flag.Int("size", worldgen.DefaultSize, "map size per side")
	players := flag.Int("players", worldgen.DefaultPlayers, "player count")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mapstat [-accept] [-size N] [-players N] [seeds]")
		flag.PrintDefaults()
	}
	flag.Parse()
	seeds := 100
	if flag.NArg() > 0 {
		fmt.Sscan(flag.Arg(0), &seeds)
	}
	if *accept {
		runAccept(seeds, *size, *players)
		return
	}
	runStats(seeds, *size, *players)
}

func runStats(seeds, size, players int) {
	var counts [terrain.TypeCount]int
	var bcounts [biome.Count]int
	var species [tree.Count]int
	tilesTotal := 0
	failures := 0
	spawnGaps := 1 << 30
	minSpawn := 1 << 30
	for s := uint64(1); s <= uint64(seeds); s++ {
		m, err := worldgen.Generate(s, size, players)
		if err != nil {
			failures++
			continue
		}
		for _, t := range m.Grid().Tiles() {
			counts[t]++
		}
		for _, b := range m.Biomes() {
			bcounts[b]++
		}
		for _, n := range m.Nodes() {
			species[n.Tree]++
		}
		tilesTotal += size * size
		d := 1 << 30
		for i, a := range m.Spawns() {
			for j, b := range m.Spawns() {
				if i != j {
					dx, dy := a[0]-b[0], a[1]-b[1]
					if dx*dx+dy*dy < d {
						d = dx*dx + dy*dy
					}
				}
			}
		}
		if d < spawnGaps {
			spawnGaps = d
		}
		if m.Spawns()[0][0] < minSpawn {
			minSpawn = m.Spawns()[0][0]
		}
	}
	fmt.Printf("seeds=%d failures=%d minSpawnDist2=%d\n", seeds, failures, spawnGaps)
	for t, c := range counts {
		fmt.Printf("%-8s %5.1f%%\n", terrain.Type(t), 100*float64(c)/float64(tilesTotal))
	}
	for b := 0; b < biome.Count; b++ {
		fmt.Printf("biome%-4d %5.1f%%\n", b, 100*float64(bcounts[b])/float64(tilesTotal))
	}
	fmt.Printf("trees: spruce=%d birch=%d palm=%d baobab=%d\n",
		species[tree.Spruce], species[tree.Birch], species[tree.Palm], species[tree.Baobab])
}

// runAccept gates the map-side criteria over `seeds` seeds:
// C1 plains majority, C2 flat connected spawns, C3 stable seed (two
// generations agree), C4 at least 3 biomes, C5 all four species.
// (C6 chop behaviours and C7 farm multipliers are sim-level and live in
// the Go test suite.)
func runAccept(seeds, size, players int) {
	bad := 0
	for s := uint64(1); s <= uint64(seeds); s++ {
		m, err := worldgen.Generate(s, size, players)
		if err != nil {
			fmt.Printf("seed %d FAIL generate: %v\n", s, err)
			bad++
			continue
		}
		m2, err := worldgen.Generate(s, size, players)
		if err != nil || m.Hash() != m2.Hash() {
			fmt.Printf("seed %d FAIL determinism (C3)\n", s)
			bad++
		}
		plains, nb := 0, map[uint8]bool{}
		for i, t := range m.Grid().Tiles() {
			if t == terrain.Grass || t == terrain.Forest || t == terrain.Sand {
				plains++
			}
			nb[m.Biomes()[i]] = true
		}
		if plains*2 <= size*size {
			fmt.Printf("seed %d FAIL plains %.1f%% <= 50%% (C1)\n", s, 100*float64(plains)/float64(size*size))
			bad++
		}
		if len(nb) < 3 {
			fmt.Printf("seed %d FAIL only %d biomes (C4)\n", s, len(nb))
			bad++
		}
		var sp [tree.Count]int
		for _, n := range m.Nodes() {
			if int(n.Tree) < tree.Count {
				sp[n.Tree]++
			}
		}
		for id := 1; id < tree.Count; id++ {
			if sp[id] == 0 {
				fmt.Printf("seed %d FAIL species %d absent (C5)\n", s, id)
				bad++
			}
		}
		if !spawnsFlat(m) {
			fmt.Printf("seed %d FAIL spawn not flat (C2)\n", s)
			bad++
		}
		if !spawnsConnected(m) {
			fmt.Printf("seed %d FAIL spawns not connected (C2)\n", s)
			bad++
		}
	}
	if bad > 0 {
		fmt.Printf("accept: FAIL (%d problems over %d seeds)\n", bad, seeds)
		os.Exit(1)
	}
	fmt.Printf("accept: ok over %d seeds\n", seeds)
}

// spawnsFlat checks the vertex spread on every spawn building disk.
func spawnsFlat(m *worldgen.Map) bool {
	g := m.Grid()
	for _, s := range m.Spawns() {
		var hi, lo int32 = 0, 1 << 30
		for dy := -3; dy <= 4; dy++ {
			for dx := -3; dx <= 4; dx++ {
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
		if hi != lo {
			return false
		}
	}
	return true
}

// spawnsConnected floods land from the first spawn (only water blocks).
func spawnsConnected(m *worldgen.Map) bool {
	size := m.Grid().Size()
	spawns := m.Spawns()
	seen := make([]bool, size*size)
	queue := []int{spawns[0][1]*size + spawns[0][0]}
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
			return false
		}
	}
	return true
}
