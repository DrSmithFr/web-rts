// Package tree holds the balance table for tree species: chop speed,
// wood and food yields and the sawmill requirement, loaded from the
// embedded JSON data file. All species numbers live there so balancing
// never requires code changes.
package tree

import (
	"encoding/json"
	"fmt"

	assets "rts"
)

// Species ids fixed by data/trees.json; None is the "not a tree" byte
// carried by non-tree resource nodes.
const (
	None   uint8 = 0
	Spruce uint8 = 1
	Birch  uint8 = 2
	Palm   uint8 = 3
	Baobab uint8 = 4

	// Count is the highest id plus one; the table must hold one def per
	// id in 1..Count-1.
	Count = 5
)

// Def is one tree species. GatherMul is the chop speed in milli (1000 =
// one resource unit per tick, 1500 = one and a half). Wood and Food are
// the node's starting amounts. Sawmill marks species that cannot be
// chopped before the player has a completed sawmill.
type Def struct {
	ID        uint8  `json:"id"`
	Key       string `json:"key"`
	GatherMul int32  `json:"gatherMul"`
	Wood      int32  `json:"wood"`
	Food      int32  `json:"food"`
	Sawmill   bool   `json:"sawmill"`
}

// Table is a validated species table indexed by id.
type Table struct {
	defs [Count]*Def
}

// Get returns the def for a species id, or nil for None and unknown ids.
func (t *Table) Get(id uint8) *Def {
	if id == 0 || id >= Count {
		return nil
	}
	return t.defs[id]
}

// Defs returns all definitions in table order.
func (t *Table) Defs() []*Def {
	var out []*Def
	for i := 1; i < Count; i++ {
		out = append(out, t.defs[i])
	}
	return out
}

// Parse validates and loads a table from raw JSON.
func Parse(raw []byte) (*Table, error) {
	var doc struct {
		Trees []Def `json:"trees"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}
	if len(doc.Trees) != Count-1 {
		return nil, fmt.Errorf("tree: want %d species, got %d", Count-1, len(doc.Trees))
	}
	t := &Table{}
	for _, d := range doc.Trees {
		if d.ID == 0 || d.ID >= Count {
			return nil, fmt.Errorf("tree: id %d out of range", d.ID)
		}
		if t.defs[d.ID] != nil {
			return nil, fmt.Errorf("tree: duplicate id %d", d.ID)
		}
		switch {
		case d.Key == "":
			return nil, fmt.Errorf("tree %d: empty key", d.ID)
		case d.GatherMul < 1:
			return nil, fmt.Errorf("tree %s: gatherMul must be >= 1", d.Key)
		case d.Wood < 0 || d.Food < 0:
			return nil, fmt.Errorf("tree %s: negative yield", d.Key)
		case d.Wood == 0 && d.Food == 0:
			return nil, fmt.Errorf("tree %s: yields nothing", d.Key)
		}
		def := d
		t.defs[d.ID] = &def
	}
	for i := 1; i < Count; i++ {
		if t.defs[i] == nil {
			return nil, fmt.Errorf("tree: missing species id %d", i)
		}
	}
	return t, nil
}

// MustLoad parses the embedded balance file; a failure is a build-time bug.
func MustLoad() *Table {
	t, err := Parse(assets.TreesJSON)
	if err != nil {
		panic(err)
	}
	return t
}
