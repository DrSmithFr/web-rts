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
	UnitSleep  uint8 = 9
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
	DefRabbit      uint16 = 9
	DefSheep       uint16 = 10
	DefBear        uint16 = 11
	DefWolf        uint16 = 12
	DefBlackWolf   uint16 = 13
	DefZombie      uint16 = 14
	DefRam         uint16 = 15
	DefTrebuchet   uint16 = 16
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
	BldWall       uint16 = 11
	BldWatchtower uint16 = 12
	BldSiegeWs    uint16 = 13
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

	// CampfireVision is the circle (in tiles) a completed campfire keeps
	// revealed; no other building reveals ground.
	CampfireVision = 10
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

	// CampfireAura is the radius in tiles where completed campfires
	// repulse shy animals and calm aggressive ones (bears are immune).
	CampfireAura int32 = 7

	// SleepWake is how much smaller the wake-up detection radius is
	// compared to the species detection radius while sleeping.
	SleepWake int32 = 3

	// PreyGuardRadius measures how close friendly units must stand to
	// count as escorting a prey; PreyEscortMax escorts means the prey is
	// grouped and predators look for an isolated target instead.
	PreyGuardRadius int32 = 4
	PreyEscortMax   int32 = 3

	// ZombieTrickleTicks is the period (ticks) between relic-den spawn
	// pulses, fired at ZombieTrickleOffset (deep midnight, one pulse a
	// night) only for UNCLAIMED relics, up to ZombieHordeCap per relic —
	// slow dread that a determined player can end, not an infinite siege.
	ZombieTrickleTicks  int32 = 9600
	ZombieTrickleOffset int32 = 6720
	ZombieHordeCap      int32 = 1

	// NoRelic marks a unit not bound to any relic.
	NoRelic uint8 = 255

	// RelicOwnerGrace is how long (ticks) a relic stays claimed after
	// the owner's garrison stops standing on it — enough to finish a
	// skirmish or walk a few tiles back, not enough to forget it.
	RelicOwnerGrace int32 = 4800

	// MaxFields caps concurrent flow fields; the oldest is evicted.
	MaxFields = 64

	// FieldMargin pads a flow-field window around its route bounds.
	FieldMargin int32 = 16

	// MaxFieldSpan caps one window axis. Walkers route around water and
	// rock through a field, so the span has to cover a cross-map walk too.
	MaxFieldSpan int32 = 384

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
	// Heading is the direction the actor faces, in whole degrees
	// clockwise from +Y (see sim/core/angle.go). Movement turns by at
	// most the def turnRate per tick, then advances along this heading.
	Heading int32
	// Hunger is a hunter's food meter: 100 (sated) to 0 (starving). Only
	// species with a prey list carry one (see sim/core/hunger.go).
	Hunger int32
	// Cond holds the condition bits (CondFear, CondTerror, CondRage).
	// Fear follows sight, terror runs on CondTicks (the remaining ticks
	// of panic a blow leaves behind).
	Cond      uint8
	CondTicks int32
	// HomeX/HomeY is the spawn point; leashed species roam near it.
	HomeX, HomeY fixed.F16
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
	// Relic is the relic index (0..RelicCount-1) a zombie is bound to,
	// NoRelic for every other unit.
	Relic    uint8
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
	// TowerCd is the shots' remaining cooldown for a completed tower.
	TowerCd int32
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
	// Hold counts down toward neutral while no military of Owner stands
	// on the tile (refreshed every tick presence shows) — a skirmish
	// that drags the garrison a few tiles off does not lose the relic,
	// but abandoning it does.
	Hold int32
}

// FogState returns the raw fog state (0 unexplored, 1 explored, 2
// visible) of a tile for a player, or Unexplored for inactive players.
func (w *World) FogState(player uint8, x, y int) uint8 {
	if player >= MaxPlayers || !w.players[player].Active {
		return fog.Unexplored
	}
	return w.fogs[player].State(x, y)
}
