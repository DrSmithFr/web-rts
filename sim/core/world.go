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

	"rts/sim/hash"
	"rts/sim/proto"
	"rts/sim/rng"
	"rts/sim/worldgen"
)

const (
	// WorldVersion bumps whenever the hash layout changes.
	WorldVersion uint64 = 2

	// HashPeriodTicks is how often peers exchange state hashes on the
	// wire (and how often replays checkpoint).
	HashPeriodTicks uint32 = 20

	hashLabel = "rts.world/v2"
)

// World is the full simulation state.
type World struct {
	seed     uint64
	gameMap  *worldgen.Map
	tick     uint32
	streams  rng.Streams
	applied  uint64
	rejected uint64
}

// NewWorld creates the deterministic world for one game seed, including
// the shared map. Generation failure is a programming error (it never
// happens for supported sizes) and panics identically on every peer.
func NewWorld(seed uint64) *World {
	m, err := worldgen.Generate(seed, worldgen.DefaultSize, worldgen.DefaultPlayers)
	if err != nil {
		panic(fmt.Sprintf("core: worldgen: %v", err))
	}
	return &World{seed: seed, gameMap: m, streams: rng.NewStreams(seed)}
}

// Map returns the shared generated map.
func (w *World) Map() *worldgen.Map { return w.gameMap }

// Tick returns the number of simulation ticks already executed.
func (w *World) Tick() uint32 { return w.tick }

// Seed returns the world seed.
func (w *World) Seed() uint64 { return w.seed }

// Stats returns executed and rejected order counters.
func (w *World) Stats() (applied, rejected uint64) { return w.applied, w.rejected }

// Step advances the simulation by exactly one tick, applying the orders
// due at this tick in the given (broadcast) order. Unknown order kinds are
// counted as rejected without failing the tick: every peer receives the
// same stream, so all reject identically.
func (w *World) Step(due []proto.Order) {
	for _, o := range due {
		switch o.Kind {
		case proto.KindNoop:
			w.applied++
		default:
			w.rejected++
		}
	}
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
	h.Write(w.streams.AppendState(nil))
	return h.Sum()
}

// HashHex renders the state hash as 16 lowercase hex digits.
func (w *World) HashHex() string { return fmt.Sprintf("%016x", w.Hash()) }
