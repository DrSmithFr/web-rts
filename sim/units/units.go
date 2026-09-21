// Package units holds the balance table for units, loaded from the
// embedded JSON data file. Speeds and ranges are 16.16 fixed-point raw
// integers (speed16, range16) so parsing stays float-free.
package units

import (
	"encoding/json"
	"errors"
	"fmt"

	assets "rts"
)

// Unit classes for combat bonuses and armor targeting.
const (
	ClassVillager uint8 = 1
	ClassInfantry uint8 = 2
	ClassArcher   uint8 = 3
	ClassScout    uint8 = 4
	ClassAnimal   uint8 = 5
)

// Damage types; armor is chosen by the attack's type.
const (
	TypePierce uint8 = 1
	TypeCrush  uint8 = 2
)

// RenderKind groups classes for client-side body shapes.
const (
	KindVillager uint8 = 1
	KindMilitary uint8 = 2
	KindAnimal   uint8 = 3
)

// Bonus is extra flat damage against one class.
type Bonus struct {
	Class uint8 `json:"class"`
	Bonus int32 `json:"bonus"`
}

// Def is one unit definition. TrainBy is the building def id that can
// train this unit (0 for trainable-nowhere animals).
type Def struct {
	ID          uint16  `json:"id"`
	Key         string  `json:"key"`
	Class       uint8   `json:"class"`
	HP          int32   `json:"hp"`
	Speed16     int32   `json:"speed16"`
	Vision      int32   `json:"vision"`
	Damage      int32   `json:"damage"`
	DamageType  uint8   `json:"damageType"`
	ArmorPierce int32   `json:"armorPierce"`
	ArmorCrush  int32   `json:"armorCrush"`
	Range16     int32   `json:"range16"`
	Rate        int32   `json:"rate"`
	CostFood    int32   `json:"costFood"`
	CostWood    int32   `json:"costWood"`
	CostGold    int32   `json:"costGold"`
	PopCost     int32   `json:"popCost"`
	TrainBy     uint16  `json:"trainBy"`
	TrainTicks  int32   `json:"trainTicks"`
	CorpseFood  int32   `json:"corpseFood"`
	Flee        bool    `json:"flee"`
	MinEra      int32   `json:"minEra"`
	Bonuses     []Bonus `json:"bonuses"`
}

// Attacks reports whether this unit can deal damage at all.
func (d *Def) Attacks() bool { return d.Damage > 0 }

// Military reports whether the unit auto-acquires visible enemies while
// idle (the combat classes).
func (d *Def) Military() bool {
	return d.Class == ClassInfantry || d.Class == ClassArcher || d.Class == ClassScout
}

// Kind returns the render kind for the class.
func (d *Def) Kind() uint8 {
	switch d.Class {
	case ClassAnimal:
		return KindAnimal
	case ClassVillager:
		return KindVillager
	default:
		return KindMilitary
	}
}

// Table is a validated unit table with fast id lookup.
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

// BonusFor returns the attack bonus def deals against a target class.
func (t *Table) BonusFor(def *Def, targetClass uint8) int32 {
	if def == nil {
		return 0
	}
	for _, b := range def.Bonuses {
		if b.Class == targetClass {
			return b.Bonus
		}
	}
	return 0
}

// Parse validates and loads a table from raw JSON.
func Parse(raw []byte) (*Table, error) {
	var doc struct {
		Units []Def `json:"units"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("unit: %w", err)
	}
	if len(doc.Units) == 0 {
		return nil, errors.New("unit: empty table")
	}
	seen := make([]bool, 0x100)
	t := &Table{}
	for _, d := range doc.Units {
		switch {
		case d.ID == 0:
			return nil, errors.New("unit: id must be non-zero")
		case d.ID < 0x100 && seen[d.ID]:
			return nil, fmt.Errorf("unit: duplicate id %d", d.ID)
		case d.Key == "":
			return nil, fmt.Errorf("unit %d: empty key", d.ID)
		case d.Class < ClassVillager || d.Class > ClassAnimal:
			return nil, fmt.Errorf("unit %s: class out of range", d.Key)
		case d.HP < 1:
			return nil, fmt.Errorf("unit %s: hp must be >= 1", d.Key)
		case d.Speed16 < 1:
			return nil, fmt.Errorf("unit %s: speed16 must be >= 1", d.Key)
		case d.Vision < 1:
			return nil, fmt.Errorf("unit %s: vision must be >= 1", d.Key)
		case d.Damage < 0:
			return nil, fmt.Errorf("unit %s: negative damage", d.Key)
		case d.Damage > 0 && d.DamageType != TypePierce && d.DamageType != TypeCrush:
			return nil, fmt.Errorf("unit %s: damage needs a valid type", d.Key)
		case d.Rate < 1:
			return nil, fmt.Errorf("unit %s: rate must be >= 1", d.Key)
		case d.CostFood < 0 || d.CostWood < 0 || d.CostGold < 0:
			return nil, fmt.Errorf("unit %s: negative cost", d.Key)
		case d.PopCost < 0:
			return nil, fmt.Errorf("unit %s: negative pop cost", d.Key)
		case d.TrainTicks < 0:
			return nil, fmt.Errorf("unit %s: negative train time", d.Key)
		case d.TrainTicks > 0 && d.TrainBy == 0:
			return nil, fmt.Errorf("unit %s: trainable without a building", d.Key)
		case d.CorpseFood < 0:
			return nil, fmt.Errorf("unit %s: negative corpse food", d.Key)
		}
		for _, b := range d.Bonuses {
			if b.Class < ClassVillager || b.Class > ClassAnimal {
				return nil, fmt.Errorf("unit %s: bonus class out of range", d.Key)
			}
			if b.Bonus < 0 || b.Bonus > 100 {
				return nil, fmt.Errorf("unit %s: bonus out of range", d.Key)
			}
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
	t, err := Parse(assets.UnitsJSON)
	if err != nil {
		panic(err)
	}
	return t
}
