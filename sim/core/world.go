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

	"rts/sim/building"
	"rts/sim/entity"
	"rts/sim/hash"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/rng"
	"rts/sim/worldgen"
)

const (
	// WorldVersion bumps whenever the hash layout changes.
	WorldVersion uint64 = 3

	// HashPeriodTicks is how often peers exchange state hashes on the
	// wire (and how often replays checkpoint).
	HashPeriodTicks uint32 = 20

	// MaxPlayers is the fixed player table size.
	MaxPlayers = 8

	hashLabel = "rts.world/v3"
)

// World is the full simulation state.
type World struct {
	seed      uint64
	gameMap   *worldgen.Map
	tick      uint32
	streams   rng.Streams
	applied   uint64
	rejected  uint64
	bdefs     *building.Table
	players   [MaxPlayers]Player
	units     *entity.Store[Unit]
	nodes     *entity.Store[Node]
	buildings *entity.Store[Building]
}

// NewWorld creates the deterministic world for one game seed, including
// the shared map, resource nodes and per-player starting economy.
// Generation failure is a programming error and panics identically on
// every peer.
func NewWorld(seed uint64) *World {
	m, err := worldgen.Generate(seed, worldgen.DefaultSize, worldgen.DefaultPlayers)
	if err != nil {
		panic(fmt.Sprintf("core: worldgen: %v", err))
	}
	w := &World{
		seed:      seed,
		gameMap:   m,
		streams:   rng.NewStreams(seed),
		bdefs:     building.MustLoad(),
		units:     entity.New[Unit](),
		nodes:     entity.New[Node](),
		buildings: entity.New[Building](),
	}
	w.initEconomy()
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

// PopCap returns the population cap for a player.
func (w *World) PopCap(owner uint8) int32 { return w.popCap(owner) }

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
	w.updateBuildings()
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
	}
	h.WriteU32(uint32(w.units.Len()))
	w.units.Each(func(id entity.ID, u *Unit) {
		h.WriteU32(uint32(id))
		h.WriteU32(uint32(u.Kind))
		h.WriteU32(uint32(u.Owner))
		h.WriteU32(uint32(u.State))
		h.WriteU32(uint32(u.X))
		h.WriteU32(uint32(u.Y))
		h.WriteU32(uint32(u.Target))
		h.WriteU32(uint32(u.CarriedK))
		h.WriteU64(uint64(int64(u.Carried)))
		h.WriteU64(uint64(int64(u.HP)))
	})
	h.WriteU32(uint32(w.nodes.Len()))
	w.nodes.Each(func(id entity.ID, n *Node) {
		h.WriteU32(uint32(id))
		h.WriteU32(uint32(n.Kind))
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
		h.WriteU64(uint64(int64(b.TrainLeft)))
		h.WriteU64(uint64(int64(b.TrainReq)))
		h.WriteU64(uint64(int64(b.HP)))
	})
	h.Write(w.streams.AppendState(nil))
	return h.Sum()
}

// HashHex renders the state hash as 16 lowercase hex digits.
func (w *World) HashHex() string { return fmt.Sprintf("%016x", w.Hash()) }
