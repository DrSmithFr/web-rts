// Package biome defines the map biome layer and its balance table: the
// dominant tree species and the agricultural production multiplier of
// each biome, loaded from the embedded JSON data file.
package biome

import (
	"encoding/json"
	"fmt"

	assets "rts"
	"rts/sim/fixed"
)

// Biome ids fixed by data/biomes.json; every map tile carries exactly
// one of these bytes.
const (
	Temperate uint8 = 0
	Taiga     uint8 = 1
	Savanna   uint8 = 2
	Coast     uint8 = 3
	Prairie   uint8 = 4

	// Count is the number of biomes; ids run 0..Count-1.
	Count = 5
)

// Def is one biome: FarmMul is the agriculture multiplier in milli
// (1000 = 1.0x) and Tree the dominant species id (see sim/tree).
type Def struct {
	ID      uint8  `json:"id"`
	Key     string `json:"key"`
	FarmMul int32  `json:"farmMul"`
	Tree    uint8  `json:"tree"`
}

// FarmMul16 returns the agriculture multiplier in 16.16 fixed point.
func (d *Def) FarmMul16() fixed.F16 {
	return fixed.F16((int64(d.FarmMul) << 16) / 1000)
}

// Table is a validated biome table indexed by id.
type Table struct {
	defs [Count]*Def
}

// Get returns the def for a biome id, or nil when out of range.
func (t *Table) Get(id uint8) *Def {
	if id >= Count {
		return nil
	}
	return t.defs[id]
}

// Defs returns all definitions in table order.
func (t *Table) Defs() []*Def {
	var out []*Def
	for i := 0; i < Count; i++ {
		out = append(out, t.defs[i])
	}
	return out
}

// Parse validates and loads a table from raw JSON.
func Parse(raw []byte) (*Table, error) {
	var doc struct {
		Biomes []Def `json:"biomes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("biome: %w", err)
	}
	if len(doc.Biomes) != Count {
		return nil, fmt.Errorf("biome: want %d biomes, got %d", Count, len(doc.Biomes))
	}
	t := &Table{}
	for _, d := range doc.Biomes {
		if d.ID >= Count {
			return nil, fmt.Errorf("biome: id %d out of range", d.ID)
		}
		if t.defs[d.ID] != nil {
			return nil, fmt.Errorf("biome: duplicate id %d", d.ID)
		}
		switch {
		case d.Key == "":
			return nil, fmt.Errorf("biome %d: empty key", d.ID)
		case d.FarmMul < 1:
			return nil, fmt.Errorf("biome %s: farmMul must be >= 1", d.Key)
		case d.Tree > 4: // sim/tree species range
			return nil, fmt.Errorf("biome %s: tree id out of range", d.Key)
		}
		def := d
		t.defs[d.ID] = &def
	}
	for i := 0; i < Count; i++ {
		if t.defs[i] == nil {
			return nil, fmt.Errorf("biome: missing id %d", i)
		}
	}
	return t, nil
}

// MustLoad parses the embedded balance file; a failure is a build-time bug.
func MustLoad() *Table {
	t, err := Parse(assets.BiomesJSON)
	if err != nil {
		panic(err)
	}
	return t
}
