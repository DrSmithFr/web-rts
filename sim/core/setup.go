// Setup helpers for scenarios and tests. These functions are placement
// tools, never reachable from orders: a lockstep peer cannot call them,
// so they carry no determinism obligations beyond producing the same
// entities for the same scenario on every peer.
package core

import (
	"rts/sim/entity"
	"rts/sim/mapfile"
	"rts/sim/resource"
)

// Label kinds for the Labels lookup tables.
const (
	LabelUnit uint8 = iota + 1
	LabelBuilding
)

// LabelEntry is one row of the label table.
type LabelEntry struct {
	Kind uint8
	Name string
	ID   entity.ID
}

// Labels maps scenario labels to entity ids. Storage is parallel
// slices, looked up by linear scan: no Go map iteration anywhere.
type Labels struct {
	kinds []uint8
	names []string
	ids   []entity.ID
}

// Entries returns the label rows in insertion order.
func (l Labels) Entries() []LabelEntry {
	out := make([]LabelEntry, 0, len(l.ids))
	for i := range l.ids {
		out = append(out, LabelEntry{Kind: l.kinds[i], Name: l.names[i], ID: l.ids[i]})
	}
	return out
}

// Name returns the label owning an id, or "-" when the id is unknown.
func (l Labels) Name(id entity.ID) string {
	for i, eid := range l.ids {
		if eid == id {
			return l.names[i]
		}
	}
	return "-"
}

// Unit returns the unit id for a label, or entity.Nil when absent.
func (l Labels) Unit(name string) entity.ID { return l.lookup(LabelUnit, name) }

// Building returns the building id for a label, or entity.Nil when
// absent.
func (l Labels) Building(name string) entity.ID { return l.lookup(LabelBuilding, name) }

func (l *Labels) add(kind uint8, name string, id entity.ID) {
	l.kinds = append(l.kinds, kind)
	l.names = append(l.names, name)
	l.ids = append(l.ids, id)
}

func (l Labels) lookup(kind uint8, name string) entity.ID {
	for i, n := range l.names {
		if l.kinds[i] == kind && n == name {
			return l.ids[i]
		}
	}
	return entity.Nil
}

// NewWorldFromScenario builds a world from a decoded custom map: players
// with their era, resources and optional start kit, then the labeled
// buildings and units. It returns the world and its label table.
func NewWorldFromScenario(seed uint64, s *mapfile.Scenario) (*World, Labels) {
	w := newWorldEconomy(seed, s.Map, false)
	w.stampNodes()
	var labels Labels
	for i, p := range s.Players {
		if i >= MaxPlayers {
			break
		}
		pl := &w.players[i]
		pl.Active = true
		pl.Era = p.Era
		pl.Res = p.Res
		if p.StartKit == mapfile.StartKitDefault {
			w.applyStartKit(uint8(i))
		}
	}
	for _, b := range s.Buildings {
		id := w.PlaceBuilding(b.Owner, b.DefID, b.X, b.Y, b.Complete)
		labels.add(LabelBuilding, b.Label, id)
	}
	for _, u := range s.Units {
		id := w.SpawnUnit(u.Owner, u.DefID, u.X, u.Y)
		labels.add(LabelUnit, u.Label, id)
	}
	return w, labels
}

// SpawnUnit places one unit at the center of a tile and returns its id.
func (w *World) SpawnUnit(owner uint8, defID uint16, tx, ty int32) entity.ID {
	return w.spawnUnit(owner, defID, tileCenter(tx), tileCenter(ty))
}

// PlaceBuilding places one building at (tx, ty): complete buildings sit
// finished on the map, otherwise the site carries its build ticks left
// as progress. Building occupancy is marked and resource nodes under
// the footprint are cleared. Panics on unknown defs.
func (w *World) PlaceBuilding(owner uint8, defID uint16, tx, ty int32, complete bool) entity.ID {
	def := w.bdefs.Get(defID)
	if def == nil {
		panic("core: place building with unknown def")
	}
	id, b := w.buildings.Create()
	b.DefID, b.Owner, b.TileX, b.TileY = defID, owner, tx, ty
	b.Size = def.Size
	b.HP = buildingHP(def)
	if complete {
		b.Progress = 0
	} else {
		b.Progress = def.BuildTicks
	}
	w.markOccupied(b)
	w.clearNodesUnder(tx, ty, def.Size)
	return id
}

// SetEra forces a player's era (never an order; setup and tests only).
func (w *World) SetEra(player uint8, era uint8) {
	if player < MaxPlayers {
		w.players[player].Era = era
	}
}

// SetResources overwrites a player's stockpile (never an order; setup
// and tests only).
func (w *World) SetResources(player uint8, res [resource.Count]int32) {
	if player < MaxPlayers {
		w.players[player].Res = res
	}
}
