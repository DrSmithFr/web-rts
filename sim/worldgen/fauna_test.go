package worldgen

import (
	"testing"

	"rts/sim/terrain"
)

// TestFaunaPlacement checks the habitat contract over 20 seeds: deer and
// boar sit on forest, sheep on plain, rabbit on plain or forest; every
// aggressive species (boar, wolf, black wolf, bear) stays beyond the
// WildZone from every spawn; bears are rare rock-dwellers.
func TestFaunaPlacement(t *testing.T) {
	deerAcrossSeeds := 0
	for seed := uint64(1); seed <= 20; seed++ {
		m, err := Generate(seed, DefaultSize, DefaultPlayers)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		count := map[uint16]int{}
		for _, a := range m.Animals() {
			count[a.Def]++
			tile := m.Grid().Tile(int(a.X), int(a.Y))
			switch a.Def {
			case DefDeer:
				if tile != terrain.Forest {
					t.Errorf("seed %d: deer at (%d,%d) off forest (%v)", seed, a.X, a.Y, tile)
				}
			case DefBoar, DefWolf, DefBlackWolf:
				if tile != terrain.Forest {
					t.Errorf("seed %d: def %d at (%d,%d) off forest (%v)", seed, a.Def, a.X, a.Y, tile)
				}
				for _, sp := range m.Spawns() {
					if iabs(int(a.X)-sp[0]) < WildZone || iabs(int(a.Y)-sp[1]) < WildZone {
						t.Errorf("seed %d: def %d at (%d,%d) inside wild zone of spawn %v", seed, a.Def, a.X, a.Y, sp)
					}
				}
				for _, rel := range m.Relics() {
					if iabs(int(a.X)-int(rel[0])) < RelicClear || iabs(int(a.Y)-int(rel[1])) < RelicClear {
						t.Errorf("seed %d: def %d den at (%d,%d) inside relic clear", seed, a.Def, a.X, a.Y)
					}
				}
			case DefSheep:
				if tile != terrain.Grass {
					t.Errorf("seed %d: sheep at (%d,%d) off plain (%v)", seed, a.X, a.Y, tile)
				}
			case DefRabbit:
				if tile != terrain.Grass && tile != terrain.Forest {
					t.Errorf("seed %d: rabbit at (%d,%d) bad terrain (%v)", seed, a.X, a.Y, tile)
				}
			case DefBear:
				if tile != terrain.Rock {
					t.Errorf("seed %d: bear at (%d,%d) off rock (%v)", seed, a.X, a.Y, tile)
				}
				for _, sp := range m.Spawns() {
					if iabs(int(a.X)-sp[0]) < WildZone || iabs(int(a.Y)-sp[1]) < WildZone {
						t.Errorf("seed %d: bear at (%d,%d) inside wild zone of spawn %v", seed, a.X, a.Y, sp)
					}
				}
				for _, rel := range m.Relics() {
					if iabs(int(a.X)-int(rel[0])) < RelicClear || iabs(int(a.Y)-int(rel[1])) < RelicClear {
						t.Errorf("seed %d: bear den at (%d,%d) inside relic clear", seed, a.X, a.Y)
					}
				}
			}
		}
		if count[DefBear] > 3 {
			t.Errorf("seed %d: %d bears, want rare", seed, count[DefBear])
		}
		deerAcrossSeeds += count[DefDeer]
		// deer only spawn in forest, so a spawn ring without forest has
		// no deer — legal, but placement must work where forest exists
	}
	if deerAcrossSeeds < 20 {
		t.Errorf("only %d deer across 20 seeds", deerAcrossSeeds)
	}
}
