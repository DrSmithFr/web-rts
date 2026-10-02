package core

import (
	"rts/sim/day"
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/orders"
	"rts/sim/terrain"
	"rts/sim/units"
)

// Wild animal AI. Species behaviour is data-driven (sim/units fields):
// sleep schedules, campfire auras, flee panic, aggro charges and pack
// hunting all derive from the def table plus sim/day — no extra RNG
// streams, wander retargets stay hash-mix deterministic.

// wildSleep reports whether u's def sleeps through the current day phase.
func (w *World) wildSleep(def *units.Def) bool {
	switch def.Sleep {
	case "night":
		return day.IsNight(w.tick)
	case "day":
		return !day.IsNight(w.tick)
	}
	return false
}

// wildDetect is the species detection radius: the larger of its own
// sight and the aggro charge radius. While sleeping, wake-up only
// registers at SleepWake tiles closer (minus SleepWake, floor 1).
func wildDetect(def *units.Def) int32 {
	r := def.Vision
	if def.Aggro > r {
		r = def.Aggro
	}
	return r
}

// nearCampfire finds the center of the completed campfire nearest to
// (x, y) within CampfireAura tiles.
func (w *World) nearCampfire(x, y fixed.F16) (fixed.F16, fixed.F16, bool) {
	lim := int64(CampfireAura) << 16
	var bestD2 int64 = lim * lim
	var bx, by fixed.F16
	found := false
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if b.DefID != BldCampfire || b.Progress != 0 || b.HP <= 0 {
			return
		}
		cx, cy := buildingCenter(b)
		d2 := dist2(x, y, cx, cy)
		if d2 <= bestD2 {
			bestD2, bx, by, found = d2, cx, cy, true
		}
	})
	return bx, by, found
}

// habitatTile reports the terrain filter for habitat species (forest).
func habitatTile(def *units.Def, g *terrain.Grid, tx, ty int32) bool {
	if def.Habitat == "" {
		return true
	}
	if !g.InBounds(int(tx), int(ty)) {
		return false
	}
	return g.Tile(int(tx), int(ty)) == terrain.Forest
}

// wildTileOK combines walkability with the habitat filter.
func (w *World) wildTileOK(def *units.Def, tx, ty int32) bool {
	return w.tileWalkable(tx, ty) && habitatTile(def, w.gameMap.Grid(), tx, ty)
}

// leashTile keeps roam targets within def.Leash tiles of the spawn home.
func leashTile(u *Unit, def *units.Def, tx, ty int32) bool {
	if def.Leash <= 0 {
		return true
	}
	dx := tx - tileOf(u.HomeX)
	dy := ty - tileOf(u.HomeY)
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= def.Leash && dy <= def.Leash
}

// wanderTileOK gates new roam targets: habitat, and shy species also
// refuse tiles inside a campfire aura.
func (w *World) wanderTileOK(u *Unit, def *units.Def, tx, ty int32) bool {
	if !w.wildTileOK(def, tx, ty) || !leashTile(u, def, tx, ty) {
		return false
	}
	if !bold(u, def) && def.Aggro > 0 && w.nearSettlement(tileCenter(tx), tileCenter(ty), settlementFear) {
		return false // wolves bed down away from torchlight
	}
	if !def.CampfireImmune && !bold(u, def) {
		if _, _, ok := w.nearCampfire(tileCenter(tx), tileCenter(ty)); ok {
			return false // shy animals bed down away from the fire
		}
	}
	return true
}

// wakeRadius is the reduced detection radius for rousing sleepers.
func wakeRadius(def *units.Def) int32 {
	r := wildDetect(def) - SleepWake
	if r < 1 {
		return 1
	}
	return r
}

// nearestThreatID is nearestThreat keyed by id (for attack targets).
// Predators (species with a prey list) skip escorted players even when
// roused — a wolf bites back at packs of one, never at marching fours.
func (w *World) nearestThreatID(u *Unit, def *units.Def, radius int32) entity.ID {
	var best entity.ID
	var bestD2 int64 = int64(radius) << 16
	bestD2 *= bestD2
	w.units.Each(func(id entity.ID, o *Unit) {
		if o.Owner == OwnerNeutral || o.Owner == u.Owner || o.HP <= 0 {
			return
		}
		if o.Owner < MaxPlayers && def.Predator() && w.guardsNear(id, o) >= int(PreyEscortMax) {
			return
		}
		if !w.inSight(u, radius, o.X, o.Y) {
			return // the threat is out of sight
		}
		d2 := dist2(u.X, u.Y, o.X, o.Y)
		if d2 <= bestD2 {
			if best == entity.Nil || d2 < bestD2 {
				bestD2, best = d2, id
			}
		}
	})
	return best
}

// wildWoken reports whether a sleeping animal rouses: a player unit
// within the reduced wake radius, or (shy species) any campfire aura.
func (w *World) wildWoken(u *Unit, def *units.Def) bool {
	if w.nearestRouse(u, wakeRadius(def)) != nil {
		return true // something walked up to it
	}
	if !def.CampfireImmune {
		if _, _, ok := w.nearCampfire(u.X, u.Y); ok {
			return true
		}
	}
	return false
}

// wildlifeStep drives one tick of a neutral animal that is not fighting.
func (w *World) wildlifeStep(id entity.ID, u *Unit) {
	def := w.udefs.Get(u.DefID)
	if def == nil {
		return
	}
	if u.State == UnitAttack {
		return // combatPhase drives the strike loop
	}
	if u.State == UnitIdle {
		u.State = UnitWander
	}
	sleepWin := w.wildSleep(def)

	if u.State == UnitSleep {
		if sleepWin && !w.wildWoken(u, def) {
			return
		}
		u.State = UnitWander
		if def.Aggro > 0 {
			// roused attackers bite back at whatever woke them, even
			// inside their sleep window
			if tid := w.nearestThreatID(u, def, wakeRadius(def)); tid != entity.Nil {
				u.State = UnitAttack
				u.AttackTarget = tid
				u.AttackKind = orders.TargetUnit
			}
		}
		return
	}

	if u.State == UnitFlee {
		if sleepWin && !w.wildWoken(u, def) {
			u.State = UnitSleep
			return
		}
		if !def.CampfireImmune {
			if cx, cy, ok := w.nearCampfire(u.X, u.Y); ok {
				w.fleeFrom(u, def, cx, cy)
				return
			}
		}
		if w.panicStep(u, def) {
			return // still scared: keep running
		}
		u.State = UnitWander
		return
	}

	// UnitWander: sleep, panic, charge or roam — in that order.
	if sleepWin && !w.wildWoken(u, def) {
		u.State = UnitSleep
		u.TX, u.TY = u.X, u.Y
		return
	}
	if !def.CampfireImmune {
		if cx, cy, ok := w.nearCampfire(u.X, u.Y); ok {
			u.State = UnitFlee
			w.fleeFrom(u, def, cx, cy)
			return
		}
	}
	if t := w.threatSeen(u, def); t != nil {
		u.Cond |= CondFear
		u.State = UnitFlee
		w.fleeFrom(u, def, t.X, t.Y)
		return
	}
	if def.Aggro > 0 {
		var t entity.ID
		if sleepWin {
			// disturbed sleepers only defend; they do not begin hunting
			t = w.nearestThreatID(u, def, wakeRadius(def))
		} else {
			t = w.wildAcquire(id, u, def)
		}
		if t != entity.Nil {
			u.State = UnitAttack
			u.AttackTarget = t
			u.AttackKind = orders.TargetUnit
			return
		}
	}
	if u.TX == 0 && u.TY == 0 {
		u.TX, u.TY = u.X, u.Y // fresh spawn: no destination yet
	}
	if (uint32(id)+uint32(w.tick)*7)%40 == 0 {
		h := mix64(uint64(w.seed) ^ uint64(id)*0x9e3779b97f4a7c15 ^ uint64(w.tick)*0xff51afd7ed558ccd)
		tx := tileOf(u.X) + int32(h%7) - 3
		ty := tileOf(u.Y) + int32((h>>3)%7) - 3
		if w.wanderTileOK(u, def, tx, ty) {
			u.TX, u.TY = tileCenter(tx), tileCenter(ty)
		}
	}
	if near(u.X, u.Y, u.TX, u.TY, 2048) {
		return
	}
	w.stepToward(u, u.TX, u.TY, w.speedOf(u))
}

// fleeFrom runs away from a threat point (unit position or campfire).
// Habitat bands are preferred, but survival outranks them: a cornered
// deer bolts across open ground rather than standing still.
func (w *World) fleeFrom(u *Unit, def *units.Def, fx, fy fixed.F16) {
	ax, ay := u.X.Add(u.X.Sub(fx)), u.Y.Add(u.Y.Sub(fy))
	tx, ty := tileOf(ax), tileOf(ay)
	switch {
	case w.wildTileOK(def, tx, ty):
		w.stepToward(u, ax, ay, w.speedOf(u))
	case w.wildTileOK(def, tx, tileOf(u.Y)):
		w.stepToward(u, ax, u.Y, w.speedOf(u))
	case w.wildTileOK(def, tileOf(u.X), ty):
		w.stepToward(u, u.X, ay, w.speedOf(u))
	case w.tileWalkable(tx, ty):
		w.stepToward(u, ax, ay, w.speedOf(u))
	case w.tileWalkable(tx, tileOf(u.Y)):
		w.stepToward(u, ax, u.Y, w.speedOf(u))
	case w.tileWalkable(tileOf(u.X), ty):
		w.stepToward(u, u.X, ay, w.speedOf(u))
	}
}

// isPrey reports whether the hunter def takes the animal defID as food
// at its current food level: preferred species always, opportunistic
// species only once the fallback hunger threshold is crossed.
func isPrey(def *units.Def, defID uint16, hunger int32) bool {
	for _, p := range def.PreyPreferred {
		if p == defID {
			return true
		}
	}
	if def.PreyFallbackAt < 1 {
		return false
	}
	if hunger > HungerFull*def.PreyFallbackAt/100 {
		return false // not hungry enough to bother
	}
	for _, p := range def.PreyFallback {
		if p == defID {
			return true
		}
	}
	return false
}

// guardsNear counts live same-owner units within PreyGuardRadius tiles
// of oid (excluding oid itself) — the escort size of a potential prey.
func (w *World) guardsNear(oid entity.ID, o *Unit) int {
	lim := int64(PreyGuardRadius) << 16
	sq := lim * lim
	n := 0
	w.units.Each(func(id entity.ID, p *Unit) {
		if id == oid || p.HP <= 0 || p.Owner != o.Owner {
			return
		}
		if dist2(o.X, o.Y, p.X, p.Y) <= sq {
			n++
		}
	})
	return n
}

// SettlementFear keeps wolves away from towns: predators neither target
// nor chase prey that stands within this many tiles of a completed
// building, and wolf dens stay outside it too.
const settlementFear int32 = 16

// nearSettlement reports any completed building within radius tiles.
func (w *World) nearSettlement(x, y fixed.F16, radius int32) bool {
	lim := int64(radius) << 16
	sq := lim * lim
	found := false
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if found || b.Progress != 0 || b.HP <= 0 {
			return
		}
		bx, by := buildingCenter(b)
		if dist2(x, y, bx, by) <= sq {
			found = true
		}
	})
	return found
}

// wildAcquire picks the nearest target for an aggressive wild animal:
// people of any banner always, neutral game only when it stands on the
// menu (see isPrey). Preferred species are looked for first, so a hungry
// wolf with a deer and a sheep in sight still goes for the deer.
// Predators skip escorted humans: a pack does not charge a formation.
func (w *World) wildAcquire(uid entity.ID, u *Unit, def *units.Def) entity.ID {
	if t := w.acquireNear(uid, u, def, true); t != entity.Nil {
		return t
	}
	return w.acquireNear(uid, u, def, false)
}

// acquireNear is one sweep of the unit store. preferred=true restricts it
// to the preferred species (people stay eligible either way);
// preferred=false adds the fallback list, which is already hunger-gated
// in isPrey.
func (w *World) acquireNear(uid entity.ID, u *Unit, def *units.Def, preferred bool) entity.ID {
	var best entity.ID
	var bestD2 int64 = -1
	w.units.Each(func(id entity.ID, o *Unit) {
		if id == uid || o.HP <= 0 {
			return
		}
		if o.Owner < MaxPlayers {
			if !w.players[o.Owner].Active {
				return
			}
			if !bold(u, def) && w.nearSettlement(o.X, o.Y, settlementFear) {
				return // wolves leave the torchlight alone
			}
		} else if preferred {
			if !inList(def.PreyPreferred, o.DefID) {
				return
			}
		} else if !isPrey(def, o.DefID, u.Hunger) {
			return // not on the menu at this hunger level
		}
		if !w.inSight(u, rageAggro(def.Aggro, def, u), o.X, o.Y) {
			return // prey or escort outside the cone
		}
		d2 := dist2(u.X, u.Y, o.X, o.Y)
		if !bold(u, def) && def.Predator() && o.Owner < MaxPlayers &&
			w.guardsNear(id, o) >= int(PreyEscortMax) {
			return // a starving hunter goes for the guarded prey too
		}
		if bestD2 < 0 || d2 < bestD2 || (d2 == bestD2 && id < best) {
			bestD2, best = d2, id
		}
	})
	return best
}

// inList reports whether defID sits in a prey list.
func inList(list []uint16, defID uint16) bool {
	for _, p := range list {
		if p == defID {
			return true
		}
	}
	return false
}

// wildDrop releases a wild attacker back to roaming; leashed animals
// head home (the caller sets TX/TY for the return walk).
func wildDrop(u *Unit) {
	u.State = UnitWander
	u.AttackTarget = entity.Nil
	u.AttackKind = 0
}

// wildChase steps one aggro charge toward t, honouring the campfire
// shield, the habitat band and the spawn leash. It returns false when
// the animal gave up the chase (state already updated).
func (w *World) wildChase(u *Unit, def *units.Def, t *Unit) bool {
	// Zombies hunt by scent, not sense: no leash, no habitat, no fear
	// of torchlight once their relic is claimed. Only outrunning them
	// (chaseLost tiles) breaks the trail, and dormant guardians still
	// won't follow prey into a campfire's glow.
	if def.Infect {
		limit := int64(def.ChaseLost) << 16
		if dist2(u.X, u.Y, t.X, t.Y) > limit*limit {
			wildDrop(u)
			u.TX, u.TY = u.HomeX, u.HomeY // shamble back toward the den
			return false
		}
		if !w.relicActivated(u.Relic) {
			if _, _, ok := w.nearCampfire(u.X, u.Y); ok {
				wildDrop(u)
				return false
			}
			if _, _, ok := w.nearCampfire(t.X, t.Y); ok {
				wildDrop(u) // prey sheltered by the campfire: give up
				return false
			}
		}
		w.stepToward(u, t.X, t.Y, w.speedOf(u))
		return true
	}
	if !def.CampfireImmune && !bold(u, def) {
		if _, _, ok := w.nearCampfire(u.X, u.Y); ok {
			wildDrop(u)
			return false
		}
		if _, _, ok := w.nearCampfire(t.X, t.Y); ok {
			wildDrop(u) // prey sheltered by the campfire: give up
			return false
		}
	}
	if def.Leash > 0 {
		dx := tileOf(u.X) - tileOf(u.HomeX)
		dy := tileOf(u.Y) - tileOf(u.HomeY)
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx > def.Leash || dy > def.Leash {
			wildDrop(u)
			u.TX, u.TY = u.HomeX, u.HomeY // leash pulls the bear home
			return false
		}
	}
	if !habitatTile(def, w.gameMap.Grid(), tileOf(t.X), tileOf(t.Y)) {
		wildDrop(u) // prey crossed out of the habitat band
		return false
	}
	if w.nearSettlement(t.X, t.Y, settlementFear) {
		wildDrop(u) // the prey reached the torchlight
		return false
	}
	w.stepToward(u, t.X, t.Y, w.speedOf(u))
	return true
}
