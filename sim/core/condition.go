package core

import (
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/units"
)

// Condition bits (Unit.Cond). Fear follows sight and vanishes with it;
// terror is the panic a blow leaves behind and runs on a timer; rage is
// the hunger-driven state added later.
const (
	CondFear   uint8 = 1
	CondTerror uint8 = 2
	CondRage   uint8 = 4
)

// feelsFear reports whether a species reacts to threats at all: hunted
// animals, non-combat humans (villagers) and any species the data gives a
// panic duration. Military classes stand their ground.
func feelsFear(def *units.Def) bool {
	if def == nil || def.Military() {
		return false
	}
	return def.Hunted || def.Class == units.ClassVillager || def.TerrorTicks > 0
}

// CivilianFearRange is how close (in tiles) a predator has to get before
// a non-combat human reads it as danger. Civilians keep their heads:
// a wolf on the horizon does not abandon a wood cut, and a town with a
// hearth is not cleared off the map by one animal.
const CivilianFearRange int32 = 4

// threatSeen is the entity that frightens u: a threat enters through the
// vision arc, then keeps the fear alive for as long as it stays inside
// the radius, whatever direction the frightened unit runs in. Prey that
// turned to flee does not stop being afraid because the wolf slid out of
// its peripheral view.
func (w *World) threatSeen(u *Unit, def *units.Def) *Unit {
	if !feelsFear(def) {
		return nil
	}
	sustain := u.Cond&CondFear != 0
	if def.Hunted {
		if sustain {
			return w.threatWithin(u, def.Vision, false) // running: it is close enough
		}
		return w.nearestThreat(u, def.Vision)
	}
	var best *Unit
	var bestID entity.ID
	radius := CivilianFearRange
	if radius > def.Vision {
		radius = def.Vision
	}
	var bestD2 int64 = int64(radius) << 16
	bestD2 *= bestD2
	w.units.Each(func(oid entity.ID, o *Unit) {
		if o.HP <= 0 || o == u {
			return
		}
		if o.Owner != OwnerNeutral && o.Owner == u.Owner {
			return // a villager is not scared of her neighbour's cattle
		}
		odef := w.udefs.Get(o.DefID)
		if odef == nil || !odef.Predator() {
			return // civilians do not spook at a soldier
		}
		if !sustain && !w.inSight(u, radius, o.X, o.Y) {
			return
		}
		d2 := dist2(u.X, u.Y, o.X, o.Y)
		if d2 > bestD2 {
			return
		}
		if best == nil || d2 < bestD2 || (d2 == bestD2 && oid < bestID) {
			bestD2, best, bestID = d2, o, oid
		}
	})
	return best
}

// updateCond runs one tick of the condition clock: fear tracks what the
// unit sees, terror counts down on its own.
func (w *World) updateCond(u *Unit) {
	def := w.udefs.Get(u.DefID)
	if u.Cond&CondTerror != 0 {
		u.CondTicks--
		if u.CondTicks <= 0 {
			u.CondTicks = 0
			u.Cond &^= CondTerror
		}
	}
	if u.State == UnitSleep {
		u.Cond &^= CondFear // a sleeper does not see the wolf at its throat
		return
	}
	if w.threatSeen(u, def) != nil {
		u.Cond |= CondFear
	} else {
		u.Cond &^= CondFear
	}
}

// terrify marks the panic a blow leaves behind: the hurt unit runs from
// where the blow came for def.TerrorTicks ticks. Only entities that feel
// fear at all are marked.
func (w *World) terrify(u *Unit, fromX, fromY fixed.F16) {
	def := w.udefs.Get(u.DefID)
	if !feelsFear(def) || def.TerrorTicks <= 0 {
		return
	}
	u.Cond |= CondTerror
	if def.TerrorTicks > u.CondTicks {
		u.CondTicks = def.TerrorTicks
	}
	// Panic needs a direction: run past the unit, straight away from the
	// blow, even if the attacker is no longer in sight.
	u.TX, u.TY = fleePoint(u.X, u.Y, fromX, fromY)
}

// fleePoint mirrors (ax, ay) through (x, y): the spot three body-lengths
// behind the frightened unit, opposite the threat.
func fleePoint(x, y, ax, ay fixed.F16) (fixed.F16, fixed.F16) {
	dx, dy := x.Sub(ax), y.Sub(ay)
	if dx == 0 && dy == 0 {
		return x, y
	}
	return x.Add(dx * 3), y.Add(dy * 3)
}

// panicStep runs one tick of flight for a frightened unit. It returns
// false when nothing frightens the unit any more, so the caller can put
// it back to what it was doing.
func (w *World) panicStep(u *Unit, def *units.Def) bool {
	t := w.threatSeen(u, def)
	if t == nil && u.Cond&CondTerror == 0 {
		return false
	}
	if u.Owner != OwnerNeutral {
		w.panicWorker(u, t)
		return true
	}
	if t != nil {
		u.State = UnitFlee
		w.fleeFrom(u, def, t.X, t.Y) // run from what it can see
		return true
	}
	// Terror with nothing in sight: keep walking to the spot picked when
	// the blow landed, then stand the rest of the panic out.
	if near(u.X, u.Y, u.TX, u.TY, 2048) {
		return true
	}
	w.stepToward(u, u.TX, u.TY, w.speedOf(u))
	return true
}

// ShelterRadius is how far (in tiles) a frightened civilian will walk to
// reach a hearth or a completed building of its own.
const ShelterRadius int32 = 12

// panicWorker is how a civilian flees: it goes home to its own finished
// building, because beasts keep off settled ground, and only backs away
// into the open when there is nothing to reach. This is also why fear
// costs an economy little - the worker is steps from its wood pile, not
// roaming the map.
func (w *World) panicWorker(u *Unit, t *Unit) {
	u.State = UnitFlee
	if cx, cy, ok := w.shelterFor(u); ok {
		if near(u.X, u.Y, cx, cy, 2048) {
			return // shelter is reached; the wolf stays out
		}
		w.stepToward(u, cx, cy, w.speedOf(u))
		return
	}
	if t != nil {
		w.fleeFrom(u, w.udefs.Get(u.DefID), t.X, t.Y)
		return
	}
	if near(u.X, u.Y, u.TX, u.TY, 2048) {
		return
	}
	w.stepToward(u, u.TX, u.TY, w.speedOf(u))
}

// shelterFor finds the centre of the completed building of u's own
// player nearest to u inside ShelterRadius tiles.
func (w *World) shelterFor(u *Unit) (fixed.F16, fixed.F16, bool) {
	if u.Owner >= MaxPlayers {
		return 0, 0, false
	}
	lim := int64(ShelterRadius) << 16
	bestD2 := lim * lim
	var bx, by fixed.F16
	found := false
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if b.Owner != u.Owner || b.Progress != 0 || b.HP <= 0 {
			return
		}
		cx, cy := buildingCenter(b)
		d2 := dist2(u.X, u.Y, cx, cy)
		if d2 <= bestD2 && (!found || d2 < bestD2) {
			bestD2, bx, by, found = d2, cx, cy, true
		}
	})
	return bx, by, found
}
