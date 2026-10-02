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
	ClassSiege    uint8 = 6
	// BonusBuilding (0) marks a bonus entry as bonus-vs-buildings.
	BonusBuilding uint8 = 0
)

// DefaultTurnRate is the turn speed (degrees per tick) a def gets when
// the JSON omits turnRate.
const DefaultTurnRate int32 = 30

// DefaultVisionArc is the field of view (degrees) a def gets when the
// JSON omits visionArc: a full circle.
const DefaultVisionArc int32 = 360

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
	ID          uint16 `json:"id"`
	Key         string `json:"key"`
	Class       uint8  `json:"class"`
	HP          int32  `json:"hp"`
	Speed16     int32  `json:"speed16"`
	TurnRate    int32  `json:"turnRate"` // degrees per tick the actor can pivot
	Vision      int32  `json:"vision"`
	VisionArc   int32  `json:"visionArc"` // degrees of field of view, 360 = all around
	Damage      int32  `json:"damage"`
	DamageType  uint8  `json:"damageType"`
	ArmorPierce int32  `json:"armorPierce"`
	ArmorCrush  int32  `json:"armorCrush"`
	Range16     int32  `json:"range16"`
	Rate        int32  `json:"rate"`
	CostFood    int32  `json:"costFood"`
	CostWood    int32  `json:"costWood"`
	CostGold    int32  `json:"costGold"`
	PopCost     int32  `json:"popCost"`
	TrainBy     uint16 `json:"trainBy"`
	TrainTicks  int32  `json:"trainTicks"`
	CorpseFood  int32  `json:"corpseFood"`
	// TerrorTicks is how long an entity that was struck runs for. Only
	// hunted animals and non-combat humans panic (see core.feelsFear).
	TerrorTicks int32 `json:"terrorTicks"`
	// Wild AI fields (class 5 only; ignored by other classes).
	Sleep string `json:"sleep"` // ""|"day"|"night" activity window
	Aggro int32  `json:"aggro"` // wild attacker charges within this radius
	Leash int32  `json:"leash"` // roam this many tiles from spawn (0 = free)
	// Habitat confines the animal to one terrain band; "forest" today.
	Habitat string `json:"habitat"`
	// CampfireImmune species ignore the campfire repulse aura.
	CampfireImmune bool `json:"campfireImmune"`
	// PreyPreferred lists neutral animal def ids a hunter takes the
	// moment it sees them. PreyFallback lists the opportunistic meals it
	// only goes for when its food meter is low; PreyFallbackAt is the
	// hunger percentage (1..100) that opens that list.
	PreyPreferred  []uint16 `json:"preyPreferred"`
	PreyFallback   []uint16 `json:"preyFallback"`
	PreyFallbackAt int32    `json:"preyFallbackAt"`
	// HungerDecay is the ticks a hunter goes without food per point of
	// hunger (0 = the species is never hungry, so it never hunts for
	// hunger reasons). Hunger runs from 100 (sated) to 0 (starving).
	HungerDecay int32 `json:"hungerDecay"`
	// RageSpeed/RageAggro are percentages applied while enraged (hunger
	// at or below 10%): a starving hunter moves further and bites wider.
	RageSpeed int32 `json:"rageSpeed"`
	RageAggro int32 `json:"rageAggro"`
	// Hunted is derived at load time: the species stands on some
	// predator's menu, so it is prey and reacts to what it sees.
	Hunted bool `json:"-"`
	// ChaseLost: a wild attacker abandons the chase once the quarry is
	// this many tiles away (0 = the species never gives up this way).
	ChaseLost int32 `json:"chaseLost"`
	// Infect marks the zombie kind: every human it kills rises again as
	// a zombie linked to the same relic.
	Infect  bool    `json:"infect"`
	MinEra  int32   `json:"minEra"`
	Bonuses []Bonus `json:"bonuses"`
}

// Attacks reports whether this unit can deal damage at all.
func (d *Def) Attacks() bool { return d.Damage > 0 }

// Predator reports whether the species hunts other animals at all.
func (d *Def) Predator() bool {
	return len(d.PreyPreferred) > 0 || len(d.PreyFallback) > 0
}

// Hungry reports whether the species carries a hunger clock: only hunters
// do, and only when the data gives a decay rate.
func (d *Def) Hungry() bool { return d.Predator() && d.HungerDecay > 0 }

// Military reports whether the unit auto-acquires visible enemies while
// idle (the combat classes).
func (d *Def) Military() bool {
	return d.Class == ClassInfantry || d.Class == ClassArcher || d.Class == ClassScout || d.Class == ClassSiege
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
		if d.TurnRate == 0 {
			d.TurnRate = DefaultTurnRate // absent in JSON
		}
		if d.VisionArc == 0 {
			d.VisionArc = DefaultVisionArc // absent in JSON
		}
		switch {
		case d.ID == 0:
			return nil, errors.New("unit: id must be non-zero")
		case d.ID < 0x100 && seen[d.ID]:
			return nil, fmt.Errorf("unit: duplicate id %d", d.ID)
		case d.Key == "":
			return nil, fmt.Errorf("unit %d: empty key", d.ID)
		case d.Class < ClassVillager || d.Class > ClassSiege:
			return nil, fmt.Errorf("unit %s: class out of range", d.Key)
		case d.HP < 1:
			return nil, fmt.Errorf("unit %s: hp must be >= 1", d.Key)
		case d.Speed16 < 1:
			return nil, fmt.Errorf("unit %s: speed16 must be >= 1", d.Key)
		case d.TurnRate < 1 || d.TurnRate > 359:
			return nil, fmt.Errorf("unit %s: turnRate must be 1..359", d.Key)
		case d.VisionArc < 1 || d.VisionArc > 360:
			return nil, fmt.Errorf("unit %s: visionArc must be 1..360", d.Key)
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
		case d.HungerDecay < 0 || d.RageSpeed < 0 || d.RageAggro < 0:
			return nil, fmt.Errorf("unit %s: negative hunger setting", d.Key)
		case d.HungerDecay > 0 && !d.Predator():
			return nil, fmt.Errorf("unit %s: hunger without a prey list", d.Key)
		case d.PreyFallbackAt < 0 || d.PreyFallbackAt > 100:
			return nil, fmt.Errorf("unit %s: preyFallbackAt must be 0..100", d.Key)
		case len(d.PreyFallback) > 0 && d.PreyFallbackAt < 1:
			return nil, fmt.Errorf("unit %s: preyFallback without preyFallbackAt", d.Key)
		case d.TerrorTicks < 0 || d.Aggro < 0 || d.Leash < 0 || d.ChaseLost < 0:
			return nil, fmt.Errorf("unit %s: negative wild AI radius", d.Key)
		case d.Aggro > 0 && d.Damage == 0:
			return nil, fmt.Errorf("unit %s: aggro without damage", d.Key)
		case d.Sleep != "" && d.Sleep != "day" && d.Sleep != "night":
			return nil, fmt.Errorf("unit %s: sleep must be day or night", d.Key)
		case d.Habitat != "" && d.Habitat != "forest":
			return nil, fmt.Errorf("unit %s: unknown habitat %q", d.Key, d.Habitat)
		case d.Infect && d.Sleep != "day":
			return nil, fmt.Errorf("unit %s: infect species must sleep by day", d.Key)
		case d.Infect && d.ChaseLost < 1:
			return nil, fmt.Errorf("unit %s: infect species need a chaseLost radius", d.Key)
		}
		for _, b := range d.Bonuses {
			if b.Class > ClassSiege {
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
	// Hunted is derived: a species listed as prey by any def is prey, and
	// prey is what feels fear.
	hunted := map[uint16]bool{}
	for _, d := range t.defs {
		for _, p := range append(append([]uint16(nil), d.PreyPreferred...), d.PreyFallback...) {
			hunted[p] = true
		}
	}
	for i := range t.defs {
		t.defs[i].Hunted = hunted[t.defs[i].ID]
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
