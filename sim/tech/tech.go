// Package tech holds the technology tree: costs, prerequisites, research
// buildings and flat stat effects, loaded from embedded JSON. Effects are
// applied as integer modifiers so the hot path stays float-free.
package tech

import (
	"encoding/json"
	"errors"
	"fmt"

	assets "rts"
)

// MaxTechs caps the table so the owned-tech bitset fits one uint64.
const MaxTechs = 64

// Effect fields applied to a whole class at once.
const (
	FieldDmg     uint8 = 1
	FieldArmorP  uint8 = 2
	FieldArmorC  uint8 = 3
	FieldSpeed16 uint8 = 4
	FieldCarry   uint8 = 5
	FieldRange16 uint8 = 6
)

var fieldNames = map[string]uint8{
	"dmg":     FieldDmg,
	"armorP":  FieldArmorP,
	"armorC":  FieldArmorC,
	"speed16": FieldSpeed16,
	"carry":   FieldCarry,
	"range16": FieldRange16,
}

// Effect is one flat modifier: field + target class + value. The JSON
// field name ("dmg", "armorC", ...) maps onto the enum below.
type Effect struct {
	Field uint8 `json:"-"`
	Class uint8 `json:"class"`
	Value int32 `json:"value"`
}

// UnmarshalJSON decodes the human-readable field name.
func (e *Effect) UnmarshalJSON(b []byte) error {
	var raw struct {
		Field string `json:"field"`
		Class uint8  `json:"class"`
		Value int32  `json:"value"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	f, ok := fieldNames[raw.Field]
	if !ok {
		return fmt.Errorf("tech: unknown effect field %q", raw.Field)
	}
	e.Field = f
	e.Class = raw.Class
	e.Value = raw.Value
	return nil
}

// Def is one technology. Requires lists tech ids that must be owned first;
// the table is validated so prerequisites always appear earlier (acyclic).
type Def struct {
	ID       uint16   `json:"id"`
	Key      string   `json:"key"`
	Era      uint8    `json:"era"`
	Building uint16   `json:"building"`
	CostFood int32    `json:"costFood"`
	CostWood int32    `json:"costWood"`
	CostGold int32    `json:"costGold"`
	Requires []uint16 `json:"requires"`
	Ticks    int32    `json:"ticks"`
	Effects  []Effect `json:"effects"`
}

// Mods accumulates the stat effects a player owns. It is derived state:
// recomputing it from the owned-tech bitset always yields the same values.
type Mods struct {
	Dmg     [7]int32
	ArmorP  [7]int32
	ArmorC  [7]int32
	Speed16 [7]int32
	Carry   [7]int32
	Range16 [7]int32
}

// Apply folds one effect into the modifier set.
func (m *Mods) Apply(e Effect) {
	switch e.Field {
	case FieldDmg:
		m.Dmg[e.Class] += e.Value
	case FieldArmorP:
		m.ArmorP[e.Class] += e.Value
	case FieldArmorC:
		m.ArmorC[e.Class] += e.Value
	case FieldSpeed16:
		m.Speed16[e.Class] += e.Value
	case FieldCarry:
		m.Carry[e.Class] += e.Value
	case FieldRange16:
		m.Range16[e.Class] += e.Value
	}
}

// Table is a validated technology table with fast id lookup.
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

// Defs returns all technologies in table order (UI order).
func (t *Table) Defs() []Def { return t.defs }

// Parse validates and loads a technology table from raw JSON.
func Parse(raw []byte) (*Table, error) {
	var doc struct {
		Techs []Def `json:"techs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("tech: %w", err)
	}
	if len(doc.Techs) == 0 {
		return nil, errors.New("tech: empty table")
	}
	if len(doc.Techs) > MaxTechs {
		return nil, fmt.Errorf("tech: %d entries exceed bitset cap", len(doc.Techs))
	}
	t := &Table{}
	seen := make(map[uint16]bool)
	for _, d := range doc.Techs {
		switch {
		case d.ID == 0:
			return nil, errors.New("tech: id must be non-zero")
		case seen[d.ID]:
			return nil, fmt.Errorf("tech: duplicate id %d", d.ID)
		case d.Key == "":
			return nil, fmt.Errorf("tech %d: empty key", d.ID)
		case d.Era > 4:
			return nil, fmt.Errorf("tech %s: era out of range", d.Key)
		case d.Building == 0:
			return nil, fmt.Errorf("tech %s: needs a research building", d.Key)
		case d.Ticks < 1:
			return nil, fmt.Errorf("tech %s: ticks must be >= 1", d.Key)
		case d.CostFood < 0 || d.CostWood < 0 || d.CostGold < 0:
			return nil, fmt.Errorf("tech %s: negative cost", d.Key)
		case len(d.Effects) == 0:
			return nil, fmt.Errorf("tech %s: no effects", d.Key)
		}
		for _, r := range d.Requires {
			if !seen[r] {
				return nil, fmt.Errorf("tech %s: unknown or forward prerequisite %d", d.Key, r)
			}
		}
		for _, e := range d.Effects {
			if e.Class < 1 || e.Class > 5 {
				return nil, fmt.Errorf("tech %s: effect class out of range", d.Key)
			}
			max := int32(100)
			switch e.Field {
			case FieldArmorP, FieldArmorC:
				max = 20
			case FieldSpeed16:
				max = 8192
			case FieldCarry:
				max = 50
			case FieldRange16:
				max = 16384 * 8
			}
			if e.Value < 0 || e.Value > max {
				return nil, fmt.Errorf("tech %s: effect value out of range", d.Key)
			}
		}
		seen[d.ID] = true
		t.defs = append(t.defs, d)
	}
	return t, nil
}

// MustLoad parses the embedded technology table; a failure is a build-time bug.
func MustLoad() *Table {
	t, err := Parse(assets.TechsJSON)
	if err != nil {
		panic(err)
	}
	return t
}
