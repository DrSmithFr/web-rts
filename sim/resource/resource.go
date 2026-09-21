// Package resource defines the five tradeable resources and node balance.
package resource

// Kind is a resource type; 0 means "none".
type Kind uint8

const (
	None  Kind = 0
	Wood  Kind = 1
	Stone Kind = 2
	Food  Kind = 3
	Gold  Kind = 4
	Oil   Kind = 5

	// Count is KindCount; kind indexes [Count]int32 arrays.
	Count = 6
)

// String returns the debug name (UI names live in i18n).
func (k Kind) String() string {
	switch k {
	case Wood:
		return "wood"
	case Stone:
		return "stone"
	case Food:
		return "food"
	case Gold:
		return "gold"
	case Oil:
		return "oil"
	default:
		return "none"
	}
}

// Harvestable reports whether a villager may gather this node kind in the
// current phase. Oil requires a derrick, which lands with the Modern era.
func (k Kind) Harvestable() bool {
	return k != None && k != Oil
}

// Node balance constants; capacities are whole resource units.
const (
	TreeAmount  int32 = 100
	StoneAmount int32 = 150
	BerryAmount int32 = 60
	GoldAmount  int32 = 300
	OilAmount   int32 = 200
)

// CarryCapacity is the maximum a single villager carries per trip.
const CarryCapacity int32 = 10

// GatherRate is resource units per tick while working.
const GatherRate int32 = 1
