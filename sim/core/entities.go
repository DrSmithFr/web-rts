package core

import (
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/fog"
	"rts/sim/resource"
	"rts/sim/tech"
)

// Unit kinds select the body shape on the client.
const (
	KindVillager uint8 = 1
	KindMilitary uint8 = 2
	KindAnimal   uint8 = 3
)

// Unit states. Workers cycle idle/move/gather/return/build; military use
// attack and patrol; animals wander and flee.
const (
	UnitIdle   uint8 = 0
	UnitMove   uint8 = 1
	UnitGather uint8 = 2
	UnitReturn uint8 = 3
	UnitBuild  uint8 = 4
	UnitAttack uint8 = 5
	UnitPatrol uint8 = 6
	UnitWander uint8 = 7
	UnitFlee   uint8 = 8
)

// Unit def ids fixed by data/units.json order.
const (
	DefVillager    uint16 = 1
	DefLancer      uint16 = 2
	DefArcher      uint16 = 3
	DefScout       uint16 = 4
	DefDeer        uint16 = 5
	DefBoar        uint16 = 6
	DefKnight      uint16 = 7
	DefCrossbowman uint16 = 8
)

// Building def ids fixed by data/buildings.json order.
const (
	BldHut        uint16 = 1
	BldCampfire   uint16 = 2
	BldStorehouse uint16 = 3
	BldBarracks   uint16 = 4
	BldMarket     uint16 = 5
	BldForge      uint16 = 6
	BldBank       uint16 = 7
	BldWonder     uint16 = 8
	BldSawmill    uint16 = 9
	BldFarm       uint16 = 10
)

// WinnerKind values for the game-over banner.
const (
	WinnerNone     uint8 = 0
	WinnerConquest uint8 = 1
	WinnerWonder   uint8 = 2
	WinnerRelic    uint8 = 3
)

// OwnerNeutral marks wild animals; not an index into the player table.
const OwnerNeutral uint8 = 0xFF

// Phase-3 balance constants. Building numbers live in data/buildings.json,
// unit numbers in data/units.json.
const (
	BuildingHP int32 = 200

	// VillagerSpeed is the fallback walk speed (~1 tile/s at 20 Hz);
	// units with a def table entry move at their def speed.
	VillagerSpeed fixed.F16 = 3277

	// WorkRange is how close a worker must stand to a node or site.
	WorkRange fixed.F16 = 1<<16 + 1<<15 // 1.5 tiles

	// Vision radii in tiles (fog of war).
	BuildingVision = 8
	SiteVision     = 4
	// VillagerVision is the fallback when a unit has no def entry.
	VillagerVision = 6

	startWood  int32 = 100
	startStone int32 = 50
	startFood  int32 = 200
	startGold  int32 = 50

	// StartPopCap is the food-population cap before any hut is built.
	StartPopCap int32 = 5

	// MaxTrainQueue bounds queued units per building.
	MaxTrainQueue int32 = 5

	// FleeRadius is how close a threat must be to panic a fleeing animal.
	FleeRadius int32 = 4
	// FleeClearRadius is the distance at which a fleeing animal calms down.
	FleeClearRadius int32 = 7

	// MaxFields caps concurrent flow fields; the oldest is evicted.
	MaxFields = 64

	// FieldMargin pads a flow-field window around its route bounds.
	FieldMargin int32 = 16

	// MaxFieldSpan caps one window axis.
	MaxFieldSpan int32 = 192

	// RelicCount is the number of map relics (mirrors worldgen).
	RelicCount = 5

	// WonderGoalTicks is how long a completed Wonder must stand before
	// its owner wins (10 in-game minutes at 20 Hz).
	WonderGoalTicks int32 = 12000

	// RelicGoalTicks is how long one player must hold every relic before
	// winning (10 in-game minutes at 20 Hz).
	RelicGoalTicks int32 = 12000

	// EliminationGrace disables conquest checks for the opening ticks so
	// nobody wins while units are still spawning.
	EliminationGrace uint32 = 200

	// MaxTechQueue bounds queued research per player.
	MaxTechQueue int32 = 4
)

// Unit is a movable actor payload (worker, soldier or animal).
type Unit struct {
	Kind   uint8
	DefID  uint16
	Owner  uint8
	State  uint8
	X, Y   fixed.F16
	TX, TY fixed.F16
	// PatrolX/Y is the other end of a patrol segment.
	PatrolX, PatrolY fixed.F16
	// Target is a gather node or build site.
	Target entity.ID
	// AttackTarget is the unit or building id named by AttackKind
	// (orders.TargetUnit/TargetBuilding, 0 = none), chased while
	// State == UnitAttack.
	AttackTarget entity.ID
	AttackKind   uint8
	// Field is the active flow field; FieldX/Y its goal tile.
	Field  entity.ID
	FieldX int32
	FieldY int32
	// Cooldown ticks down between weapon strikes.
	Cooldown int32
	CarriedK resource.Kind
	Carried  int32
	// GatherAcc is the fixed-point remainder of the per-tick chop amount
	// when the target node is a tree species whose speed differs from
	// the base gather rate (fractional part, 16.16).
	GatherAcc int32
	HP        int32
}

// Node is a harvestable resource node payload. Tree is the species id
// for wood and palm nodes (see sim/tree), 0 for other kinds. Corpse
// marks a hunted-animal carcass, whose gatherer chains to live prey.
type Node struct {
	Kind   resource.Kind
	Tree   uint8
	Corpse bool
	X, Y   fixed.F16
	TileX  int32
	TileY  int32
	Amount int32
}

// Building is a constructed or constructing building payload. Progress is
// the remaining build ticks (0 = complete). The train queue holds def ids
// in TrainQueue[:TrainReq]; TrainCur is the def currently training.
type Building struct {
	DefID      uint16
	Owner      uint8
	TileX      int32
	TileY      int32
	Size       int32
	Progress   int32
	TrainCur   uint16
	TrainLeft  int32
	TrainReq   int32
	TrainQueue [MaxTrainQueue]uint16
	// FarmAcc is the fixed-point food accumulated between whole units by
	// a completed farm, at the biome multiplier of its tile (16.16).
	FarmAcc int32
	HP      int32
}

// Player is one player's economy and progress state. Mods is derived
// from Techs (recomputed when a tech completes), never hashed directly.
type Player struct {
	Active bool
	Res    [resource.Count]int32
	Pop    int32

	// Era ladder: AgeLeft counts down while AgeTarget is being reached.
	Era       uint8
	AgeTarget uint8
	AgeLeft   int32

	// Tech state: Techs holds one bit per table entry; TechCur/TechLeft
	// track the running research and TechQueue[:TechReq] the backlog.
	Techs     uint64
	TechCur   uint16
	TechLeft  int32
	TechQueue [MaxTechQueue]uint16
	TechReq   int32
	Mods      tech.Mods

	// Victory bookkeeping (recomputed each progress tick).
	Relics      int32
	RelicTicks  int32
	WonderTicks int32
}

// Relic is one map relic; Owner is OwnerNeutral until a military unit
// stands on its tile.
type Relic struct {
	X, Y  int32
	Owner uint8
}

// FogState returns the raw fog state (0 unexplored, 1 explored, 2
// visible) of a tile for a player, or Unexplored for inactive players.
func (w *World) FogState(player uint8, x, y int) uint8 {
	if player >= MaxPlayers || !w.players[player].Active {
		return fog.Unexplored
	}
	return w.fogs[player].State(x, y)
}
