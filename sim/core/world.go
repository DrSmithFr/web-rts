// Package core holds the deterministic simulation world.
//
// Phase 1 ships the empty world: tick counter, per-domain random streams,
// and order accounting, but with the full contract every later system
// inherits — same seed plus same order stream implies identical hash
// sequences on every platform and every build.
//
// Invariants:
//   - no floats, no maps, no time, no goroutines;
//   - Step consumes exactly one draw per domain stream per tick;
//   - every value that can differ between peers is inside the hash.
package core

import (
	"fmt"

	"rts/sim/biome"
	"rts/sim/building"
	"rts/sim/entity"
	"rts/sim/era"
	"rts/sim/fog"
	"rts/sim/hash"
	"rts/sim/orders"
	"rts/sim/path"
	"rts/sim/proto"
	"rts/sim/resource"
	"rts/sim/rng"
	"rts/sim/tech"
	"rts/sim/tree"
	"rts/sim/units"
	"rts/sim/worldgen"
)

const (
	// WorldVersion bumps whenever the hash layout changes.
	WorldVersion uint64 = 9

	// HashPeriodTicks is how often peers exchange state hashes on the
	// wire (and how often replays checkpoint).
	HashPeriodTicks uint32 = 20

	// MaxPlayers is the fixed player table size.
	MaxPlayers = 8

	hashLabel = "rts.world/v10"
)

// World is the full simulation state.
type World struct {
	seed       uint64
	gameMap    *worldgen.Map
	tick       uint32
	streams    rng.Streams
	applied    uint64
	rejected   uint64
	bdefs      *building.Table
	udefs      *units.Table
	edefs      *era.Table
	tdefs      *tech.Table
	biomeDefs  *biome.Table
	species    *tree.Table
	treeStep   [tree.Count]int32
	players    [MaxPlayers]Player
	relics     [RelicCount]Relic
	winner     uint8
	winnerKind uint8
	units      *entity.Store[Unit]
	nodes      *entity.Store[Node]
	buildings  *entity.Store[Building]
	fields     *entity.Store[path.Field]
	occupied   []bool
	fogs       [MaxPlayers]*fog.Grid
}

// NewWorld creates the default-sized deterministic world for one seed.
func NewWorld(seed uint64) *World {
	return NewWorldSize(seed, worldgen.DefaultSize, worldgen.DefaultPlayers)
}

// NewWorldSize creates a deterministic world with a custom map size.
// Generation failure is a programming error and panics identically on
// every peer.
func NewWorldSize(seed uint64, size, players int) *World {
	m, err := worldgen.Generate(seed, size, players)
	if err != nil {
		panic(fmt.Sprintf("core: worldgen: %v", err))
	}
	return newWorld(seed, m)
}

// NewWorldWithMap builds a world around a hand-made map (tests and
// scenario tools).
func NewWorldWithMap(seed uint64, m *worldgen.Map) *World {
	return newWorld(seed, m)
}

func newWorld(seed uint64, m *worldgen.Map) *World {
	size := m.Grid().Size()
	w := &World{
		seed:      seed,
		gameMap:   m,
		streams:   rng.NewStreams(seed),
		bdefs:     building.MustLoad(),
		udefs:     units.MustLoad(),
		edefs:     era.MustLoad(),
		tdefs:     tech.MustLoad(),
		biomeDefs: biome.MustLoad(),
		species:   tree.MustLoad(),
		winner:    OwnerNeutral,
		units:     entity.New[Unit](),
		nodes:     entity.New[Node](),
		buildings: entity.New[Building](),
		fields:    entity.New[path.Field](),
		occupied:  make([]bool, size*size),
	}
	for i := 1; i < tree.Count; i++ {
		w.treeStep[i] = int32((int64(resource.GatherRate) << 16) * int64(w.species.Get(uint8(i)).GatherMul) / 1000)
	}
	mr := m.Relics()
	for i := range w.relics {
		w.relics[i] = Relic{X: mr[i][0], Y: mr[i][1], Owner: OwnerNeutral}
	}
	for i := 0; i < MaxPlayers; i++ {
		w.fogs[i] = fog.New(size)
	}
	w.initEconomy()
	w.updateVision()
	return w
}

// Map returns the shared generated map.
func (w *World) Map() *worldgen.Map { return w.gameMap }

// Units returns the unit store.
func (w *World) Units() *entity.Store[Unit] { return w.units }

// Nodes returns the resource node store.
func (w *World) Nodes() *entity.Store[Node] { return w.nodes }

// Buildings returns the building store.
func (w *World) Buildings() *entity.Store[Building] { return w.buildings }

// Players returns the player table.
func (w *World) Players() *[MaxPlayers]Player { return &w.players }

// BuildingDefs returns the loaded balance table.
func (w *World) BuildingDefs() *building.Table { return w.bdefs }

// UnitDefs returns the loaded unit balance table.
func (w *World) UnitDefs() *units.Table { return w.udefs }

// EraDefs returns the loaded era ladder.
func (w *World) EraDefs() *era.Table { return w.edefs }

// TechDefs returns the loaded technology table.
func (w *World) TechDefs() *tech.Table { return w.tdefs }

// BiomeDefs returns the loaded biome table.
func (w *World) BiomeDefs() *biome.Table { return w.biomeDefs }

// TreeDefs returns the loaded tree species table.
func (w *World) TreeDefs() *tree.Table { return w.species }

// Winner returns the game-over owner and kind (WinnerNone while open).
func (w *World) Winner() (uint8, uint8) { return w.winner, w.winnerKind }

// Relics returns the current relic placements.
func (w *World) Relics() [RelicCount]Relic { return w.relics }

// PopCap returns the population cap for a player.
func (w *World) PopCap(owner uint8) int32 { return w.popCap(owner) }

// Fields returns the flow-field store (debug tooling).
func (w *World) Fields() *entity.Store[path.Field] { return w.fields }

// Fog returns a player's fog layer.
func (w *World) Fog(player uint8) *fog.Grid { return w.fogs[player] }

// Tick returns the number of simulation ticks already executed.
func (w *World) Tick() uint32 { return w.tick }

// Seed returns the world seed.
func (w *World) Seed() uint64 { return w.seed }

// Stats returns executed and rejected order counters.
func (w *World) Stats() (applied, rejected uint64) { return w.applied, w.rejected }

// Step advances the simulation by exactly one tick: orders due at this
// tick are applied first (broadcast order), then units, then buildings.
// Malformed orders are counted as rejected; unknown kinds too. Every peer
// receives the same stream, so all reject identically.
func (w *World) Step(due []proto.Order) {
	for _, o := range due {
		var ok bool
		switch o.Kind {
		case proto.KindNoop:
			ok = true
		case orders.Move:
			ok = w.applyMove(o.Player, o.Payload)
		case orders.Gather:
			ok = w.applyGather(o.Player, o.Payload)
		case orders.Build:
			ok = w.applyBuild(o.Player, o.Payload)
		case orders.Stop:
			ok = w.applyStop(o.Player, o.Payload)
		case orders.Train:
			ok = w.applyTrain(o.Player, o.Payload)
		case orders.MoveGroup:
			ok = w.applyMoveGroup(o.Player, o.Payload)
		case orders.Attack:
			ok = w.applyAttack(o.Player, o.Payload)
		case orders.Patrol:
			ok = w.applyPatrol(o.Player, o.Payload)
		case orders.AgeUp:
			ok = w.applyAgeUp(o.Player, o.Payload)
		case orders.Research:
			ok = w.applyResearch(o.Player, o.Payload)
		case orders.Repair:
			ok = w.applyRepair(o.Player, o.Payload)
		default:
			w.rejected++
			continue
		}
		if ok {
			w.applied++
		} else {
			w.rejected++
		}
	}
	w.updateUnits()
	w.combatPhase()
	w.updateBuildings()
	w.progressPhase()
	w.updateVision()
	for d := rng.Domain(0); d < rng.DomainCount; d++ {
		w.streams.Next(d)
	}
	w.tick++
}

// Hash returns the canonical state digest used for desync detection.
func (w *World) Hash() uint64 {
	h := hash.New()
	h.WriteString(hashLabel)
	h.WriteU64(WorldVersion)
	h.WriteU64(w.seed)
	h.WriteU64(w.gameMap.Hash())
	h.WriteU32(w.tick)
	h.WriteU64(w.applied)
	h.WriteU64(w.rejected)
	for i := 0; i < MaxPlayers; i++ {
		p := &w.players[i]
		if p.Active {
			h.WriteU32(1)
		} else {
			h.WriteU32(0)
		}
		for k := 0; k < len(p.Res); k++ {
			h.WriteU64(uint64(int64(p.Res[k])))
		}
		h.WriteU64(uint64(int64(p.Pop)))
		h.WriteU32(uint32(p.Era))
		h.WriteU32(uint32(p.AgeTarget))
		h.WriteU64(uint64(int64(p.AgeLeft)))
		h.WriteU64(p.Techs)
		h.WriteU32(uint32(p.TechCur))
		h.WriteU64(uint64(int64(p.TechLeft)))
		h.WriteU32(uint32(p.TechReq))
		for i := int32(0); i < p.TechReq && i < MaxTechQueue; i++ {
			h.WriteU32(uint32(p.TechQueue[i]))
		}
		h.WriteU64(uint64(int64(p.Relics)))
		h.WriteU64(uint64(int64(p.RelicTicks)))
		h.WriteU64(uint64(int64(p.WonderTicks)))
		if p.Active {
			w.fogs[i].AppendState(h, "rts.fog/v1")
		}
	}
	for i := 0; i < RelicCount; i++ {
		h.WriteU32(uint32(w.relics[i].X))
		h.WriteU32(uint32(w.relics[i].Y))
		h.WriteU32(uint32(w.relics[i].Owner))
	}
	h.WriteU32(uint32(w.winner))
	h.WriteU32(uint32(w.winnerKind))
	h.WriteU32(uint32(w.units.Len()))
	w.units.Each(func(id entity.ID, u *Unit) {
		h.WriteU32(uint32(id))
		h.WriteU32(uint32(u.Kind))
		h.WriteU32(uint32(u.DefID))
		h.WriteU32(uint32(u.Owner))
		h.WriteU32(uint32(u.State))
		h.WriteU32(uint32(u.X))
		h.WriteU32(uint32(u.Y))
		h.WriteU32(uint32(u.Target))
		h.WriteU32(uint32(u.AttackTarget))
		h.WriteU32(uint32(u.AttackKind))
		h.WriteU64(uint64(int64(u.Cooldown)))
		h.WriteU32(uint32(u.CarriedK))
		h.WriteU64(uint64(int64(u.Carried)))
		h.WriteU64(uint64(int64(u.GatherAcc)))
		h.WriteU64(uint64(int64(u.HP)))
	})
	h.WriteU32(uint32(w.nodes.Len()))
	w.nodes.Each(func(id entity.ID, n *Node) {
		h.WriteU32(uint32(id))
		h.WriteU32(uint32(n.Kind))
		h.WriteU32(uint32(n.Tree))
		if n.Corpse {
			h.WriteU32(1)
		} else {
			h.WriteU32(0)
		}
		h.WriteU64(uint64(int64(n.Amount)))
		h.WriteU32(uint32(n.TileX))
		h.WriteU32(uint32(n.TileY))
	})
	h.WriteU32(uint32(w.buildings.Len()))
	w.buildings.Each(func(id entity.ID, b *Building) {
		h.WriteU32(uint32(id))
		h.WriteU32(uint32(b.DefID))
		h.WriteU32(uint32(b.Owner))
		h.WriteU32(uint32(b.TileX))
		h.WriteU32(uint32(b.TileY))
		h.WriteU32(uint32(b.Size))
		h.WriteU64(uint64(int64(b.Progress)))
		h.WriteU64(uint64(int64(b.FarmAcc)))
		h.WriteU32(uint32(b.TrainCur))
		h.WriteU64(uint64(int64(b.TrainLeft)))
		h.WriteU64(uint64(int64(b.TrainReq)))
		for i := int32(0); i < b.TrainReq && i < MaxTrainQueue; i++ {
			h.WriteU32(uint32(b.TrainQueue[i]))
		}
		h.WriteU64(uint64(int64(b.HP)))
	})
	h.Write(w.streams.AppendState(nil))
	return h.Sum()
}

// HashHex renders the state hash as 16 lowercase hex digits.
func (w *World) HashHex() string { return fmt.Sprintf("%016x", w.Hash()) }

// DebugSpawnUnits fills walkable ground scanning from the owner's map
// corner with count units of defID, ignoring costs and population. Dev
// tooling and E2E performance scenarios only; never reachable from
// orders.
func (w *World) DebugSpawnUnits(owner uint8, defID uint16, count int) {
	spawns := w.gameMap.Spawns()
	if int(owner) >= len(spawns) && len(spawns) == 0 {
		return
	}
	size := int32(w.gameMap.Grid().Size())
	placed := 0
	for y := int32(0); y < size && placed < count; y++ {
		for x := int32(0); x < size && placed < count; x++ {
			if !w.tileWalkable(x, y) {
				continue
			}
			w.spawnUnit(owner, defID, tileCenter(x), tileCenter(y))
			placed++
		}
	}
}
