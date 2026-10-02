package core

import (
	"rts/sim/building"
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/fog"
	"rts/sim/orders"
	"rts/sim/resource"
	"rts/sim/units"
)

// applyAttack orders a group to chase and strike one hostile unit.
// Neutral animals (owner >= MaxPlayers) are valid targets for everyone.
func (w *World) applyAttack(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	n := int(r.Uvarint())
	if n == 0 || n > 128 {
		return false
	}
	ids := make([]entity.ID, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, entity.ID(r.Uvarint()))
	}
	target := entity.ID(r.Uvarint())
	kind := uint8(r.Uvarint())
	if !r.OK() || (kind != orders.TargetUnit && kind != orders.TargetBuilding) {
		return false
	}
	switch kind {
	case orders.TargetBuilding:
		b := w.buildings.Get(target)
		if b == nil || b.Owner >= MaxPlayers || b.Owner == player || b.HP <= 0 {
			return true // stale or own target: no-op
		}
	default:
		t := w.units.Get(target)
		if t == nil || t.Owner == player || t.HP <= 0 || w.zombieGhosted(t) {
			return true // stale target, or a ghost no one can see
		}
	}
	for _, id := range ids {
		u := w.units.Get(id)
		if u == nil || u.Owner != player {
			continue
		}
		if d := w.udefs.Get(u.DefID); d == nil || !d.Attacks() {
			continue
		}
		u.State = UnitAttack
		u.AttackTarget = target
		u.AttackKind = kind
	}
	return true
}

// applyPatrol sets a ping-pong patrol segment from each unit's current
// position to the destination tile.
func (w *World) applyPatrol(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	n := int(r.Uvarint())
	if n == 0 || n > 128 {
		return false
	}
	ids := make([]entity.ID, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, entity.ID(r.Uvarint()))
	}
	tx := int32(r.U16()) >> 4
	ty := int32(r.U16()) >> 4
	if !r.OK() {
		return false
	}
	if !w.tileWalkable(tx, ty) {
		return true
	}
	for _, id := range ids {
		u := w.units.Get(id)
		if u == nil || u.Owner != player {
			continue
		}
		u.PatrolX, u.PatrolY = u.X, u.Y
		u.TX, u.TY = tileCenter(tx), tileCenter(ty)
		u.Field, u.FieldX, u.FieldY = entity.Nil, -1, -1
		u.State = UnitPatrol
	}
	return true
}

// patrolStep walks one patrol leg, swapping the segment endpoints on
// arrival so the unit ping-pongs forever.
func (w *World) patrolStep(u *Unit) {
	gx, gy := tileOf(u.TX), tileOf(u.TY)
	if w.followField(u, gx, gy) == routeWalked {
		return
	}
	if near(u.X, u.Y, u.TX, u.TY, 2048) {
		u.PatrolX, u.PatrolY, u.TX, u.TY = u.TX, u.TY, u.PatrolX, u.PatrolY
		u.Field = entity.Nil
		return
	}
	w.stepToward(u, u.TX, u.TY, w.speedOf(u))
}

// nearestThreat returns the closest hostile unit within radius tiles
// (ties by lower id), or nil. Player units threaten any animal; neutral
// predators (species with a prey list) threaten prey species, so deer,
// rabbits and sheep scatter from wolves. Only threats inside the unit's
// vision arc count.
func (w *World) nearestThreat(u *Unit, radius int32) *Unit {
	return w.threatWithin(u, radius, true)
}

// nearestRouse is nearestThreat for a unit that cannot look around: a
// sleeping animal wakes to the footsteps of whatever gets close, whatever
// direction it is facing and whatever its eyes would have made of it.
func (w *World) nearestRouse(u *Unit, radius int32) *Unit {
	return w.threatWithin(u, radius, false)
}

// threatWithin is the shared threat scan; sight=false ignores the vision
// arc and the sleep state.
func (w *World) threatWithin(u *Unit, radius int32, sight bool) *Unit {
	var best *Unit
	var bestD2 int64 = int64(radius) << 16
	bestD2 *= bestD2
	udef := w.udefs.Get(u.DefID)
	preyish := udef != nil && udef.Hunted
	w.units.Each(func(_ entity.ID, o *Unit) {
		if o.HP <= 0 {
			return
		}
		if o.Owner == OwnerNeutral {
			if !preyish {
				return
			}
			odef := w.udefs.Get(o.DefID)
			if odef == nil || !odef.Predator() || o == u {
				return
			}
		} else if o.Owner == u.Owner {
			return
		}
		if sight && !w.inSight(u, radius, o.X, o.Y) {
			return // out of range, or behind the nose
		}
		d2 := dist2(u.X, u.Y, o.X, o.Y)
		if d2 < bestD2 || (d2 == bestD2 && best != nil && false) {
			bestD2, best = d2, o
		}
	})
	return best
}

// attackDamage is the canonical strike formula:
// max(1, damage + tech bonus + class bonus - (armor + tech armor)).
func (w *World) attackDamage(att *units.Def, attOwner uint8, target *Unit) int32 {
	tdef := w.udefs.Get(target.DefID)
	armor := int32(0)
	tclass := uint8(0)
	if tdef != nil {
		tclass = tdef.Class
		switch att.DamageType {
		case units.TypePierce:
			armor = tdef.ArmorPierce
		case units.TypeCrush:
			armor = tdef.ArmorCrush
		}
	}
	if target.Owner < MaxPlayers {
		switch att.DamageType {
		case units.TypePierce:
			armor += w.players[target.Owner].Mods.ArmorP[tclass]
		case units.TypeCrush:
			armor += w.players[target.Owner].Mods.ArmorC[tclass]
		}
	}
	dmg := att.Damage + w.udefs.BonusFor(att, tclass) - armor
	if attOwner < MaxPlayers {
		dmg += w.players[attOwner].Mods.Dmg[att.Class]
	}
	if dmg < 1 {
		dmg = 1
	}
	return dmg
}

// combatPhase resolves strikes, retaliation, auto-acquire and deaths for
// one tick. Dead units are freed after all damage is applied so every
// peer resolves the same simultaneous volleys.
func (w *World) combatPhase() {
	w.cooldownTick()
	var dead []entity.ID
	type kill struct {
		killer entity.ID
		victim entity.ID
		human  bool
		food   int32 // corpse food the victim carries, for a hunter killer
		x, y   fixed.F16
	}
	var kills []kill
	markDead := func(attacker, victim entity.ID) {
		for _, d := range dead {
			if d == victim {
				return
			}
		}
		dead = append(dead, victim)
		if u := w.units.Get(victim); u != nil {
			vdef := w.udefs.Get(u.DefID)
			// the walking dead raise the helpless — unarmed villagers —
			// not armored soldiers, so a battlefield of troops does not
			// snowball into an unkillable horde
			human := u.Owner < MaxPlayers && u.DefID != DefZombie &&
				vdef != nil && !vdef.Military()
			food := int32(0)
			if vdef != nil {
				food = vdef.CorpseFood
			}
			kills = append(kills, kill{
				killer: attacker,
				victim: victim,
				human:  human,
				food:   food,
				x:      u.X,
				y:      u.Y,
			})
		}
	}

	w.units.Each(func(id entity.ID, u *Unit) {
		if u.State != UnitAttack {
			return
		}
		def := w.udefs.Get(u.DefID)
		if def == nil || !def.Attacks() {
			u.State = UnitIdle
			u.AttackTarget = entity.Nil
			u.AttackKind = 0
			return
		}
		if u.AttackKind == orders.TargetBuilding {
			if !w.attackBuilding(id, u, def) {
				u.State = UnitIdle
				u.AttackTarget = entity.Nil
				u.AttackKind = 0
			}
			return
		}
		t := w.units.Get(u.AttackTarget)
		if t == nil || t.HP <= 0 || (t.Owner == u.Owner && u.Owner != OwnerNeutral) {
			u.State = UnitIdle
			u.AttackTarget = entity.Nil
			u.AttackKind = 0
			return
		}
		reach := fixed.F16(def.Range16)
		if u.Owner < MaxPlayers {
			reach += fixed.F16(w.players[u.Owner].Mods.Range16[def.Class])
		}
		if near(u.X, u.Y, t.X, t.Y, reach) {
			w.faceToward(u, t.X, t.Y)
			if u.Cooldown > 0 {
				return
			}
			t.HP -= w.attackDamage(def, u.Owner, t)
			u.Cooldown = def.Rate
			w.terrify(t, u.X, u.Y) // a blow leaves panic with a direction
			if t.State == UnitSleep {
				t.State = UnitWander // a struck animal rouses at once
			}
			if t.HP <= 0 {
				markDead(id, u.AttackTarget)
				return
			}
			// retaliation: defenders that can fight back and are not
			// already attacking acquire their attacker
			if td := w.udefs.Get(t.DefID); td != nil && td.Attacks() && t.State != UnitAttack {
				t.State = UnitAttack
				t.AttackTarget = id
				t.AttackKind = orders.TargetUnit
			}
			return
		}
		if u.Owner == OwnerNeutral {
			w.wildChase(u, def, t) // animals step straight, no flow field
			return
		}
		if w.followField(u, tileOf(t.X), tileOf(t.Y)) != routeWalked {
			w.stepToward(u, t.X, t.Y, w.speedOf(u))
		}
	})

	// auto-acquire for idle military: nearest visible hostile unit within
	// vision, ties by lower id
	w.units.Each(func(_ entity.ID, u *Unit) {
		def := w.udefs.Get(u.DefID)
		if u.State != UnitIdle || def == nil || !def.Military() || u.Owner >= MaxPlayers {
			return
		}
		fg := w.fogs[u.Owner]
		var best entity.ID
		var bestD2 int64 = -1
		w.units.Each(func(id entity.ID, o *Unit) {
			if o.HP <= 0 || o.Owner == u.Owner {
				return
			}
			// wild animals stay wild to idle eyes, but night-walking
			// zombies are hostile to every garrison
			if o.Owner >= MaxPlayers && !isZombie(o) {
				return
			}
			if w.zombieGhosted(o) {
				return
			}
			if fg.State(int(tileOf(o.X)), int(tileOf(o.Y))) != fog.Visible {
				return
			}
			if !w.sees(u, o.X, o.Y) {
				return // hostile outside the vision cone
			}
			d2 := dist2(u.X, u.Y, o.X, o.Y)
			if bestD2 < 0 || d2 < bestD2 || (d2 == bestD2 && id < best) {
				bestD2, best = d2, id
			}
		})
		if best != entity.Nil {
			u.State = UnitAttack
			u.AttackTarget = best
			u.AttackKind = orders.TargetUnit
		}
	})

	w.towerPhase(markDead)
	// Starvation: a hunter that has not eaten in days stops running and
	// starts losing blood until something gives way.
	w.units.Each(func(id entity.ID, u *Unit) {
		def := w.udefs.Get(u.DefID)
		if u.HP <= 0 || def == nil || !def.Hungry() || u.Hunger > 0 {
			return
		}
		if (uint64(w.tick)+uint64(id))%uint64(StarveTicks) != 0 {
			return
		}
		u.HP -= StarveDamage
		if u.HP <= 0 {
			markDead(entity.Nil, id)
		}
	})

	// A hunter that brought something down eats it here and now.
	for _, k := range kills {
		if k.killer == entity.Nil || k.food <= 0 {
			continue
		}
		if killer := w.units.Get(k.killer); killer != nil {
			w.eat(killer, k.food)
		}
	}

	w.processDeaths(dead)

	// infection: every human a zombie kills rises again bound to the
	// same relic — the horde is made of its victims.
	for _, k := range kills {
		if !k.human {
			continue
		}
		killer := w.units.Get(k.killer)
		if killer == nil || killer.HP <= 0 {
			continue
		}
		kdef := w.udefs.Get(killer.DefID)
		if kdef == nil || !kdef.Infect {
			continue
		}
		w.spawnZombie(killer.Relic, tileCenter(tileOf(k.x)), tileCenter(tileOf(k.y)))
	}
}

// attackBuilding drives one attacker assigned to an enemy building,
// approaching it, trading blows with its footprint and levelling it.
// It returns false when AttackTarget is not a live enemy building.
func (w *World) attackBuilding(id entity.ID, u *Unit, def *units.Def) bool {
	b := w.buildings.Get(u.AttackTarget)
	if b == nil || b.Owner >= MaxPlayers || b.Owner == u.Owner || b.HP <= 0 {
		return false
	}
	bx, by := buildingCenter(b)
	reach := fixed.F16(def.Range16)
	if u.Owner < MaxPlayers {
		reach += fixed.F16(w.players[u.Owner].Mods.Range16[def.Class])
	}
	reach += fixed.F16((b.Size>>1)+1) << 16
	if near(u.X, u.Y, bx, by, reach) {
		w.faceToward(u, bx, by)
		if u.Cooldown > 0 {
			return true
		}
		dmg := def.Damage + w.udefs.BonusFor(def, 0)
		if u.Owner < MaxPlayers {
			dmg += w.players[u.Owner].Mods.Dmg[def.Class]
		}
		if dmg < 1 {
			dmg = 1
		}
		b.HP -= dmg
		u.Cooldown = def.Rate
		if b.HP <= 0 {
			w.buildings.Free(u.AttackTarget)
			u.State = UnitIdle
			u.AttackTarget = entity.Nil
			u.AttackKind = 0
		}
		return true
	}
	if w.followField(u, b.TileX+b.Size>>1, b.TileY+b.Size>>1) != routeWalked {
		w.stepToward(u, bx, by, w.speedOf(u))
	}
	return true
}

// processDeaths frees the fallen, drops huntable corpses and redirects
// former attackers: villagers resume the hunt by harvesting the corpse.
func (w *World) processDeaths(dead []entity.ID) {
	for _, id := range dead {
		u := w.units.Get(id)
		if u == nil {
			continue
		}
		def := w.udefs.Get(u.DefID)
		var corpse entity.ID
		if def != nil && def.CorpseFood > 0 {
			cid, n := w.nodes.Create()
			n.Kind = resource.Food
			n.Corpse = true
			n.X, n.Y = u.X, u.Y
			n.TileX, n.TileY = tileOf(u.X), tileOf(u.Y)
			n.Amount = def.CorpseFood
			corpse = cid
		}
		if u.Owner < MaxPlayers && def != nil {
			w.players[u.Owner].Pop -= def.PopCost
		}
		w.units.Free(id)

		w.units.Each(func(_ entity.ID, g *Unit) {
			if g.AttackTarget != id {
				return
			}
			g.AttackTarget = entity.Nil
			g.AttackKind = 0
			if corpse != entity.Nil && g.Owner != OwnerNeutral && g.Kind == KindVillager {
				g.Target = corpse
				g.State = UnitGather
			} else {
				g.State = UnitIdle
			}
		})
	}
}

// cooldownTick decays every unit's weapon cooldown; called first in the
// combat phase by combatPhase itself (kept separate for readability).
func (w *World) cooldownTick() {
	w.units.Each(func(_ entity.ID, u *Unit) {
		if u.Cooldown > 0 {
			u.Cooldown--
		}
	})
}

func mix64(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// towerPhase is the watchtower volley: each completed tower shoots the
// nearest enemy unit in range, oldest-shot-first via its cooldown.
func (w *World) towerPhase(markDead func(attacker, victim entity.ID)) {
	w.buildings.Each(func(id entity.ID, b *Building) {
		def := w.bdefs.Get(b.DefID)
		if def == nil || !def.IsTower() || b.Owner >= MaxPlayers ||
			b.Progress < def.BuildTicks {
			return
		}
		if b.TowerCd > 0 {
			b.TowerCd--
			return
		}
		cx, cy := buildingCenter(b)
		limit := int64(def.TowerRange) << 16
		limit *= limit
		var best entity.ID
		var bestD2 int64 = -1
		w.units.Each(func(uid entity.ID, u *Unit) {
			if u.HP <= 0 || u.Owner == b.Owner || u.Owner == OwnerNeutral {
				return
			}
			d2 := dist2(cx, cy, u.X, u.Y)
			if d2 > limit {
				return
			}
			if bestD2 < 0 || d2 < bestD2 || (d2 == bestD2 && uid < best) {
				bestD2, best = d2, uid
			}
		})
		if best == entity.Nil {
			return
		}
		t := w.units.Get(best)
		dmg := w.towerDamage(def, t)
		t.HP -= dmg
		b.TowerCd = def.TowerRate
		// Tower bolts do not panic anybody: a civilian only takes flight
		// from living threats it can see, not from stone that shoots.
		if t.State == UnitSleep {
			t.State = UnitWander
		}
		if t.HP <= 0 {
			markDead(entity.Nil, best)
		}
	})
}

// towerDamage resolves one tower shot against a unit: towers shoot
// arrows (pierce), without class bonuses or tech, floored at 1.
func (w *World) towerDamage(def *building.Def, target *Unit) int32 {
	armor := int32(0)
	if tdef := w.udefs.Get(target.DefID); tdef != nil {
		armor = tdef.ArmorPierce
		if target.Owner < MaxPlayers {
			armor += w.players[target.Owner].Mods.ArmorP[tdef.Class]
		}
	}
	dmg := def.TowerDamage - armor
	if dmg < 1 {
		dmg = 1
	}
	return dmg
}
