package core

import (
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
		if t == nil || t.Owner == player || t.HP <= 0 {
			return true // well-formed order at a stale target: no-op
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
	if w.followField(u, gx, gy) {
		return
	}
	if near(u.X, u.Y, u.TX, u.TY, 2048) {
		u.PatrolX, u.PatrolY, u.TX, u.TY = u.TX, u.TY, u.PatrolX, u.PatrolY
		u.Field = entity.Nil
		return
	}
	stepToward(&u.X, &u.Y, u.TX, u.TY, w.speedOf(u))
}

// wanderStep is the idle animal loop: panic into flight when a threat
// gets close, otherwise retarget a few tiles away every so often.
func (w *World) wanderStep(id entity.ID, u *Unit) {
	def := w.udefs.Get(u.DefID)
	if def != nil && def.Flee {
		if t := w.nearestThreat(u, FleeRadius); t != nil {
			u.State = UnitFlee
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
		if w.tileWalkable(tx, ty) {
			u.TX, u.TY = tileCenter(tx), tileCenter(ty)
		}
	}
	if near(u.X, u.Y, u.TX, u.TY, 2048) {
		return
	}
	stepToward(&u.X, &u.Y, u.TX, u.TY, w.speedOf(u))
}

// fleeStep runs away from the closest threat until it is far enough away.
func (w *World) fleeStep(u *Unit) {
	t := w.nearestThreat(u, FleeClearRadius)
	if t == nil {
		u.State = UnitWander
		return
	}
	// Mirror point behind the unit; fall back to axis-away, then stand.
	ax, ay := u.X.Add(u.X.Sub(t.X)), u.Y.Add(u.Y.Sub(t.Y))
	if !w.tileWalkable(tileOf(ax), tileOf(ay)) {
		if w.tileWalkable(tileOf(ax), tileOf(u.Y)) {
			ay = u.Y
		} else if w.tileWalkable(tileOf(u.X), tileOf(ay)) {
			ax = u.X
		} else {
			return
		}
	}
	stepToward(&u.X, &u.Y, ax, ay, w.speedOf(u))
}

// nearestThreat returns the closest hostile player unit within radius
// tiles (ties by lower id), or nil.
func (w *World) nearestThreat(u *Unit, radius int32) *Unit {
	var best *Unit
	var bestD2 int64 = int64(radius) << 16
	bestD2 *= bestD2
	w.units.Each(func(_ entity.ID, o *Unit) {
		if o.Owner == OwnerNeutral || o.Owner == u.Owner || o.HP <= 0 {
			return
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
	markDead := func(id entity.ID) {
		for _, d := range dead {
			if d == id {
				return
			}
		}
		dead = append(dead, id)
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
		if t == nil || t.Owner == u.Owner || t.HP <= 0 {
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
			if u.Cooldown > 0 {
				return
			}
			t.HP -= w.attackDamage(def, u.Owner, t)
			u.Cooldown = def.Rate
			if t.HP <= 0 {
				markDead(u.AttackTarget)
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
		if !w.followField(u, tileOf(t.X), tileOf(t.Y)) {
			stepToward(&u.X, &u.Y, t.X, t.Y, w.speedOf(u))
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
		limit := int64(def.Vision) << 16
		limit *= limit
		w.units.Each(func(id entity.ID, o *Unit) {
			if o.HP <= 0 || o.Owner == u.Owner || o.Owner >= MaxPlayers {
				return
			}
			if fg.State(int(tileOf(o.X)), int(tileOf(o.Y))) != fog.Visible {
				return
			}
			d2 := dist2(u.X, u.Y, o.X, o.Y)
			if d2 > limit {
				return
			}
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

	w.processDeaths(dead)
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
	if !w.followField(u, b.TileX+b.Size>>1, b.TileY+b.Size>>1) {
		stepToward(&u.X, &u.Y, bx, by, w.speedOf(u))
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
