// Package era holds the age-up ladder: costs, prerequisite buildings and
// research time for each of the five eras, loaded from embedded JSON.
package era

import (
	"encoding/json"
	"errors"
	"fmt"

	assets "rts"
)

// NumEras is the fixed number of eras (Prehistory .. Modern).
const NumEras = 5

// FirstEra is the era every player starts in.
const FirstEra uint8 = 0

// LastEra is the highest reachable era.
const LastEra uint8 = NumEras - 1

// Def is one era entry: the cost and requirement to advance *into* it.
// Era 0 has zero cost and is never targeted by an age-up order.
type Def struct {
	ID          uint8  `json:"id"`
	Key         string `json:"key"`
	CostFood    int32  `json:"costFood"`
	CostGold    int32  `json:"costGold"`
	ReqBuilding uint16 `json:"reqBuilding"`
	AgeTicks    int32  `json:"ageTicks"`
}

// Table is a validated era table indexed by era id.
type Table struct {
	defs []Def
}

// Get returns the def for an era id, or nil when out of range.
func (t *Table) Get(id uint8) *Def {
	if id >= uint8(len(t.defs)) {
		return nil
	}
	return &t.defs[id]
}

// Defs returns all era definitions in ladder order.
func (t *Table) Defs() []Def { return t.defs }

// Parse validates and loads an era table from raw JSON. Eras must form a
// contiguous ladder starting at id 0.
func Parse(raw []byte) (*Table, error) {
	var doc struct {
		Eras []Def `json:"eras"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("era: %w", err)
	}
	if len(doc.Eras) != NumEras {
		return nil, fmt.Errorf("era: want %d eras, got %d", NumEras, len(doc.Eras))
	}
	t := &Table{}
	for i, d := range doc.Eras {
		switch {
		case int(d.ID) != i:
			return nil, fmt.Errorf("era: id %d out of ladder order", d.ID)
		case d.Key == "":
			return nil, fmt.Errorf("era %d: empty key", d.ID)
		case d.CostFood < 0 || d.CostGold < 0:
			return nil, fmt.Errorf("era %s: negative cost", d.Key)
		case i == 0 && (d.CostFood > 0 || d.CostGold > 0 || d.ReqBuilding > 0):
			return nil, errors.New("era: prehistory must be free")
		case i > 0 && d.AgeTicks < 1:
			return nil, fmt.Errorf("era %s: ageTicks must be >= 1", d.Key)
		case i > 0 && d.ReqBuilding == 0:
			return nil, fmt.Errorf("era %s: needs a prerequisite building", d.Key)
		}
		t.defs = append(t.defs, d)
	}
	return t, nil
}

// MustLoad parses the embedded era ladder; a failure is a build-time bug.
func MustLoad() *Table {
	t, err := Parse(assets.ErasJSON)
	if err != nil {
		panic(err)
	}
	return t
}
