package core

import (
	"rts/sim/day"
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/orders"
	"rts/sim/units"
)

// Zombie AI: relic-bound guardians that live only at night. By day they
// are ghosts — dormant, invisible and untargetable. At night each relic
// den trickles out a new guardian while the horde grows from every
// human it kills. A dormant relic keeps its guardians shy of campfires;
// once a player captures the relic the bound zombies shrug off the
// torchlight and shamble onto it, turning every held relic into a
// nightly siege. Behaviour is data-driven (units.Def.Infect) and uses
// no RNG: dens and spawn tiles resolve by deterministic ring scans.

// isZombie reports the species without a def lookup on the hot path.
func isZombie(u *Unit) bool { return u.DefID == DefZombie }

// zombieGhosted reports whether the zombie is day-dormant: still on the
// field (its tile holds) but untargetable by anyone.
func (w *World) zombieGhosted(u *Unit) bool {
	return isZombie(u) && !day.IsNight(w.tick)
}

// relicActivated reports whether a player currently holds relic ri.
func (w *World) relicActivated(ri uint8) bool {
	return ri < RelicCount && w.relics[ri].Owner != OwnerNeutral
}

// zombieDen returns the den tile for relic ri: the first walkable tile
// found by a widening ring scan around the relic (never the relic tile
// itself), so every seed has a fixed staging ground per relic.
func (w *World) zombieDen(ri uint8) (int32, int32) {
	rel := w.relics[ri]
	for r := int32(4); r <= 12; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx != -r && dx != r && dy != -r && dy != r {
					continue
				}
				tx, ty := rel.X+dx, rel.Y+dy
				if w.tileWalkable(tx, ty) {
					return tx, ty
				}
			}
		}
	}
	return rel.X, rel.Y
}

// spawnZombie places a zombie of relic ri at a tile center.
func (w *World) spawnZombie(ri uint8, x, y fixed.F16) {
	id := w.spawnUnit(OwnerNeutral, DefZombie, x, y)
	w.units.Get(id).Relic = ri
}

// zombieTrickle pulses one guardian out of every UNCLAIMED relic den
// each night, up to ZombieHordeCap per relic. A player who holds the
// relic seals its wellspring: the bound horde already walking there is
// the whole siege, killable and not endlessly refilled — that is the
// challenge of keeping it, not an unbounded doom.
func (w *World) zombieTrickle() {
	if int32(w.tick)%ZombieTrickleTicks != ZombieTrickleOffset {
		return
	}
	counts := [RelicCount]int32{}
	w.units.Each(func(_ entity.ID, u *Unit) {
		if isZombie(u) && u.HP > 0 && u.Relic < RelicCount {
			counts[u.Relic]++
		}
	})
	for ri := uint8(0); ri < RelicCount; ri++ {
		if w.relics[ri].Owner != OwnerNeutral {
			continue // sealed: a held relic's den sleeps
		}
		if counts[ri] >= ZombieHordeCap {
			continue
		}
		tx, ty := w.zombieDen(ri)
		w.spawnZombie(ri, tileCenter(tx), tileCenter(ty))
	}
}

// zombieStep advances one zombie's state machine during updateUnits.
func (w *World) zombieStep(id entity.ID, u *Unit) {
	def := w.udefs.Get(u.DefID)
	if def == nil {
		return
	}
	if !day.IsNight(w.tick) {
		u.State = UnitSleep
		u.TX, u.TY = u.X, u.Y
		return
	}
	if u.State == UnitSleep {
		u.State = UnitWander
	}
	if u.State == UnitAttack {
		// chase movement and the chaseLost leash live in combatPhase
		// (wildChase); nothing to steer here.
		return
	}
	if u.State == UnitFlee {
		cx, cy, ok := w.nearCampfire(u.X, u.Y)
		if !ok {
			u.State = UnitWander
			return
		}
		w.fleeFrom(u, def, cx, cy)
		return
	}
	// dormant guardians refuse the torchlight; an activated relic's
	// horde walks straight through it.
	if !w.relicActivated(u.Relic) {
		if cx, cy, ok := w.nearCampfire(u.X, u.Y); ok {
			u.State = UnitFlee
			w.fleeFrom(u, def, cx, cy)
			return
		}
	}
	// night hunt: any human within aggro, escorted or not — zombies
	// never skip a target for company.
	if tid := w.nearestHumanID(u, def.Aggro); tid != entity.Nil {
		u.State = UnitAttack
		u.AttackTarget = tid
		u.AttackKind = orders.TargetUnit
		u.Field, u.FieldX, u.FieldY = entity.Nil, -1, -1
		return
	}
	if w.relicActivated(u.Relic) {
		// the relic is claimed: every bound guardian walks onto it
		// slowly — they arrived as the garrison's problem.
		rel := w.relics[u.Relic]
		if tileOf(u.X) == rel.X && tileOf(u.Y) == rel.Y {
			u.TX, u.TY = u.X, u.Y
			return
		}
		stepToward(&u.X, &u.Y, tileCenter(rel.X), tileCenter(rel.Y), w.speedOf(u))
		return
	}
	w.zombieRoam(id, u, def)
}

// nearestHumanID is the nearest live player unit (any owner) within
// radius tiles, ties by lower id — zombies hunt people, not animals,
// and never look for an easier isolated target first.
func (w *World) nearestHumanID(u *Unit, radius int32) entity.ID {
	var best entity.ID
	var bestD2 int64 = int64(radius) << 16
	bestD2 *= bestD2
	w.units.Each(func(id entity.ID, o *Unit) {
		if o.Owner >= MaxPlayers || o.HP <= 0 {
			return
		}
		d2 := dist2(u.X, u.Y, o.X, o.Y)
		if d2 <= bestD2 && (best == entity.Nil || d2 < bestD2 || (d2 == bestD2 && id < best)) {
			bestD2, best = d2, id
		}
	})
	return best
}

// zombieRoam keeps a dormant guardian wandering around its den home,
// refusing tiles inside a campfire aura.
func (w *World) zombieRoam(id entity.ID, u *Unit, def *units.Def) {
	tx, ty := tileOf(u.TX), tileOf(u.TY)
	if tileOf(u.X) != tx || tileOf(u.Y) != ty || !w.zombieRoamTile(u, def, tx, ty) {
		tx, ty = w.zombieWanderTarget(id, u, def)
		u.TX, u.TY = tileCenter(tx), tileCenter(ty)
	}
	stepToward(&u.X, &u.Y, u.TX, u.TY, w.speedOf(u))
}

// zombieRoamTile gates roam targets: walkable, leashed to the den and —
// while the relic sleeps unclaimed — outside any campfire aura.
func (w *World) zombieRoamTile(u *Unit, def *units.Def, tx, ty int32) bool {
	if !w.tileWalkable(tx, ty) || !leashTile(u, def, tx, ty) {
		return false
	}
	if !w.relicActivated(u.Relic) {
		if _, _, ok := w.nearCampfire(tileCenter(tx), tileCenter(ty)); ok {
			return false
		}
	}
	return true
}

// zombieWanderTarget picks the next roam tile by a deterministic hash
// mix around the den (no RNG stream involved).
func (w *World) zombieWanderTarget(id entity.ID, u *Unit, def *units.Def) (int32, int32) {
	hx, hy := tileOf(u.X), tileOf(u.Y)
	span := def.Leash
	if span < 1 {
		span = 6
	}
	m := mix64(uint64(w.tick^0x20b1) + uint64(uint32(id)) + uint64(tileOf(u.HomeX))<<7 + uint64(tileOf(u.HomeY)))
	dx := int32(m%uint64(2*span+1)) - span
	dy := int32((m >> 16) % uint64(2*span+1))
	dy -= span
	tx, ty := hx+dx, hy+dy
	if w.zombieRoamTile(u, def, tx, ty) {
		return tx, ty
	}
	return tileOf(u.HomeX), tileOf(u.HomeY)
}
