// Command mapstat prints terrain statistics for tuning worldgen thresholds.
package main

import (
	"fmt"
	"os"

	"rts/sim/terrain"
	"rts/sim/worldgen"
)

func main() {
	seeds := 100
	if len(os.Args) > 1 {
		fmt.Sscan(os.Args[1], &seeds)
	}
	var counts [terrain.TypeCount]int
	tilesTotal := 0
	failures := 0
	spawnGaps := 1 << 30
	minSpawn := 1 << 30
	for s := uint64(1); s <= uint64(seeds); s++ {
		m, err := worldgen.Generate(s, worldgen.DefaultSize, worldgen.DefaultPlayers)
		if err != nil {
			failures++
			continue
		}
		for _, t := range m.Grid().Tiles() {
			counts[t]++
			tilesTotal++
		}
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
}
