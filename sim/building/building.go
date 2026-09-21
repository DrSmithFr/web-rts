// Package building holds the balance table for buildings, loaded from the
// embedded JSON data file. All gameplay numbers live there so balancing
// never requires code changes.
package building

import (
	"encoding/json"
	"errors"
	"fmt"

	assets "rts"
)

// Def is one building definition.
type Def struct {
	ID         uint16 `json:"id"`
	Key        string `json:"key"`
	CostWood   int32  `json:"costWood"`
	CostStone  int32  `json:"costStone"`
	BuildTicks int32  `json:"buildTicks"`
	PopRoom    int32  `json:"popRoom"`
	Size       int32  `json:"size"`
	Dropoff    bool   `json:"dropoff"`
	TrainFood  int32  `json:"trainFood"`
	TrainTicks int32  `json:"trainTicks"`
	MinEra     int32  `json:"minEra"`
}

// TrainsVillager reports whether this building can train villagers.
func (d *Def) TrainsVillager() bool { return d.TrainTicks > 0 && d.TrainFood >= 0 }

// Table is a validated building table with fast id lookup.
type Table struct {
	defs []Def
}

// Get returns the def for an id, or nil.
func (t *Table) Get(id uint16) *Def {
	for i := range t.defs {
		if t.defs[i].ID == id {
			return &t.defs[i]
		}
	}
	return nil
}

// Defs returns all definitions in table order (UI order).
func (t *Table) Defs() []Def { return t.defs }

// Parse validates and loads a table from raw JSON.
func Parse(raw []byte) (*Table, error) {
	var doc struct {
		Buildings []Def `json:"buildings"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("building: %w", err)
	}
	if len(doc.Buildings) == 0 {
		return nil, errors.New("building: empty table")
	}
	seen := make([]bool, 0x100) // data ids stay < 256 for phase 3
	t := &Table{}
	for _, d := range doc.Buildings {
		switch {
		case d.ID == 0:
			return nil, errors.New("building: id must be non-zero")
		case d.ID < 0x100 && seen[d.ID]:
			return nil, fmt.Errorf("building: duplicate id %d", d.ID)
		case d.Key == "":
			return nil, fmt.Errorf("building %d: empty key", d.ID)
		case d.CostWood < 0 || d.CostStone < 0:
			return nil, fmt.Errorf("building %s: negative cost", d.Key)
		case d.BuildTicks < 1:
			return nil, fmt.Errorf("building %s: buildTicks must be >= 1", d.Key)
		case d.Size < 1 || d.Size > 3:
			return nil, fmt.Errorf("building %s: size out of range", d.Key)
		case d.TrainFood < 0 || d.TrainTicks < 0:
			return nil, fmt.Errorf("building %s: negative train cost", d.Key)
		case d.TrainTicks > 0 && d.TrainFood < 0:
			return nil, fmt.Errorf("building %s: training needs a food cost", d.Key)
		case d.MinEra < 0 || d.MinEra > 4:
			return nil, fmt.Errorf("building %s: minEra out of range", d.Key)
		}
		if d.ID < 0x100 {
			seen[d.ID] = true
		}
		t.defs = append(t.defs, d)
	}
	return t, nil
}

// MustLoad parses the embedded balance file; a failure is a build-time bug.
func MustLoad() *Table {
	t, err := Parse(assets.BuildingsJSON)
	if err != nil {
		panic(err)
	}
	return t
}
