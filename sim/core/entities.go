package core

import (
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/resource"
)

// Unit kinds; villagers are the only one until combat lands.
const (
	UnitVillager uint8 = 1
)

// Unit states (state machine: idle -> move/gather/build -> return -> ...).
const (
	UnitIdle   uint8 = 0
	UnitMove   uint8 = 1
	UnitGather uint8 = 2
	UnitReturn uint8 = 3
	UnitBuild  uint8 = 4
)

// Phase-3 balance constants. Building numbers live in data/buildings.json.
const (
	VillagerHP int32 = 40
	BuildingHP int32 = 200

	// VillagerSpeed is ~1 tile/s at 20 Hz.
	VillagerSpeed fixed.F16 = 3277

	// WorkRange is how close a worker must stand to a node or site.
	WorkRange fixed.F16 = 1<<16 + 1<<15 // 1.5 tiles

	// Vision radii in tiles (fog of war).
	VillagerVision = 6
	BuildingVision = 8
	SiteVision     = 4

	startWood  int32 = 100
	startStone int32 = 50
	startFood  int32 = 200
	startGold  int32 = 50

	// StartPopCap is the food-population cap before any hut is built.
	StartPopCap int32 = 5

	// MaxTrainQueue bounds queued villagers per building.
	MaxTrainQueue int32 = 5

	// MaxFields caps concurrent flow fields; the oldest is evicted.
	MaxFields = 64

	// FieldMargin pads a flow-field window around its route bounds.
	FieldMargin int32 = 16

	// MaxFieldSpan caps one window axis.
	MaxFieldSpan int32 = 192
)

// Unit is a worker (villager) entity payload.
type Unit struct {
	Kind     uint8
	Owner    uint8
	State    uint8
	X, Y     fixed.F16
	TX, TY   fixed.F16
	Target   entity.ID
	Field    entity.ID
	CarriedK resource.Kind
	Carried  int32
	HP       int32
}

// Node is a harvestable resource node payload.
type Node struct {
	Kind   resource.Kind
	X, Y   fixed.F16
	TileX  int32
	TileY  int32
	Amount int32
}

// Building is a constructed or constructing building payload. Progress is
// the remaining build ticks (0 = complete).
type Building struct {
	DefID     uint16
	Owner     uint8
	TileX     int32
	TileY     int32
	Size      int32
	Progress  int32
	TrainLeft int32
	TrainReq  int32
	HP        int32
}

// Player is one player's economy state.
type Player struct {
	Active bool
	Res    [resource.Count]int32
	Pop    int32
}
