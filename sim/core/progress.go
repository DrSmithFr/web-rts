package core

// Phase 6 systems: age-up ladder, technology research, relic control
// and victory resolution. All order checks reject silently (the order is
// applied as a no-op), matching the other order handlers.

import (
	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/resource"
)

// techBit maps a tech id onto its owned-bit; index order equals table
// order, so prerequisites (validated earlier in the table) resolve first.
func (w *World) techBit(id uint16) (uint64, bool) {
	for i, t := range w.tdefs.Defs() {
		if t.ID == id {
			return uint64(1) << uint(i), true
		}
	}
	return 0, false
}

// applyAgeUp starts advancing to the next era: the target must be exactly
// era+1, affordable now (paid immediately) and backed by the era's
// prerequisite building, and no age-up may already run.
func (w *World) applyAgeUp(player uint8, p []byte) bool {
	if player >= MaxPlayers || !w.players[player].Active {
		return true
	}
	r := orders.NewReader(p)
	target := uint8(r.Uvarint())
	if !r.OK() {
		return false
	}
	pl := &w.players[player]
	def := w.edefs.Get(target)
	if def == nil || target <= pl.Era || target > pl.Era+1 || pl.AgeLeft > 0 {
		return true
	}
	if w.edefs.Get(pl.Era) == nil {
		return true
	}
	if pl.Res[resource.Food] < def.CostFood || pl.Res[resource.Gold] < def.CostGold {
		return true
	}
	if def.ReqBuilding != 0 && !w.hasBuilding(player, def.ReqBuilding) {
		return true
	}
	pl.Res[resource.Food] -= def.CostFood
	pl.Res[resource.Gold] -= def.CostGold
	pl.AgeTarget = target
	pl.AgeLeft = def.AgeTicks
	return true
}

// applyResearch queues a technology: era gate, not owned, not queued,
// prerequisites owned and the research building complete. Costs are paid
// when research actually starts.
func (w *World) applyResearch(player uint8, p []byte) bool {
	if player >= MaxPlayers || !w.players[player].Active {
		return true
	}
	r := orders.NewReader(p)
	id := uint16(r.Uvarint())
	if !r.OK() {
		return false
	}
	pl := &w.players[player]
	def := w.tdefs.Get(id)
	if def == nil || pl.TechReq >= MaxTechQueue {
		return true
	}
	if def.Era > pl.Era {
		return true
	}
	bit, ok := w.techBit(id)
	if !ok || pl.Techs&bit != 0 {
		return true
	}
	for i := int32(0); i < pl.TechReq; i++ {
		if pl.TechQueue[i] == id {
			return true
		}
	}
	if pl.TechCur == id {
		return true
	}
	for _, req := range def.Requires {
		rb, ok := w.techBit(req)
		if !ok || pl.Techs&rb == 0 {
			return true
		}
	}
	if !w.hasBuilding(player, def.Building) {
		return true
	}
	pl.TechQueue[pl.TechReq] = id
	pl.TechReq++
	return true
}

// hasBuilding reports whether the player owns a completed building def.
func (w *World) hasBuilding(player uint8, defID uint16) bool {
	found := false
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if b.Owner == player && b.DefID == defID && b.Progress == 0 {
			found = true
		}
	})
	return found
}

// progressPhase advances age-ups and research, then resolves relic
// control and victory. Fixed order: players by slot, relics by index,
// claimers by unit id.
func (w *World) progressPhase() {
	for i := 0; i < MaxPlayers; i++ {
		pl := &w.players[i]
		if !pl.Active {
			continue
		}
		if pl.AgeLeft > 0 {
			pl.AgeLeft--
			if pl.AgeLeft == 0 {
				pl.Era = pl.AgeTarget
			}
		}
		if pl.TechCur != 0 {
			pl.TechLeft--
			if pl.TechLeft <= 0 {
				w.completeTech(pl)
			}
		} else if pl.TechReq > 0 {
			w.startNextTech(pl)
		}
	}

	// relic control is presence, not a flag: standing military refreshes
	// the claim each tick, and while nobody of the owner stands on the
	// tile the hold drains — abandon it and it reverts to neutral.
	for ri := 0; ri < RelicCount; ri++ {
		rel := &w.relics[ri]
		claimed := OwnerNeutral
		w.units.Each(func(_ entity.ID, u *Unit) {
			if claimed != OwnerNeutral || u.Owner >= MaxPlayers || u.HP <= 0 {
				return
			}
			def := w.udefs.Get(u.DefID)
			if def == nil || !def.Military() {
				return
			}
			if tileOf(u.X) == rel.X && tileOf(u.Y) == rel.Y {
				claimed = u.Owner
			}
		})
		switch {
		case claimed != OwnerNeutral:
			rel.Owner, rel.Hold = claimed, RelicOwnerGrace
		case rel.Owner != OwnerNeutral:
			rel.Hold--
			if rel.Hold <= 0 {
				rel.Owner, rel.Hold = OwnerNeutral, 0
			}
		}
	}

	// per-player victory bookkeeping
	for i := 0; i < MaxPlayers; i++ {
		pl := &w.players[i]
		if !pl.Active {
			continue
		}
		held := int32(0)
		for ri := 0; ri < RelicCount; ri++ {
			if w.relics[ri].Owner == uint8(i) {
				held++
			}
		}
		pl.Relics = held
		if held == RelicCount {
			pl.RelicTicks++
		} else {
			pl.RelicTicks = 0
		}
		if w.hasBuilding(uint8(i), BldWonder) {
			pl.WonderTicks++
		} else {
			pl.WonderTicks = 0
		}
		if w.winner != OwnerNeutral {
			continue
		}
		if pl.RelicTicks >= RelicGoalTicks {
			w.winner, w.winnerKind = uint8(i), WinnerRelic
		} else if pl.WonderTicks >= WonderGoalTicks {
			w.winner, w.winnerKind = uint8(i), WinnerWonder
		}
	}

	// conquest: exactly one player still has buildings or units
	if w.winner == OwnerNeutral && w.tick > EliminationGrace {
		live, count := uint8(0), 0
		for i := 0; i < MaxPlayers; i++ {
			if !w.players[i].Active {
				continue
			}
			if w.playerHasPresence(uint8(i)) {
				live = uint8(i)
				count++
			}
		}
		if count == 1 {
			w.winner, w.winnerKind = live, WinnerConquest
		}
	}
}

// playerHasPresence reports whether the owner still owns a unit or a
// building (conquest survival test).
func (w *World) playerHasPresence(owner uint8) bool {
	present := false
	w.units.Each(func(_ entity.ID, u *Unit) {
		if u.Owner == owner && u.HP > 0 {
			present = true
		}
	})
	if present {
		return true
	}
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if b.Owner == owner {
			present = true
		}
	})
	return present
}

func (w *World) startNextTech(pl *Player) {
	def := w.tdefs.Get(pl.TechQueue[0])
	if def == nil {
		shiftTechQueue(pl)
		return
	}
	if pl.Res[resource.Food] < def.CostFood ||
		pl.Res[resource.Wood] < def.CostWood ||
		pl.Res[resource.Gold] < def.CostGold {
		return
	}
	pl.Res[resource.Food] -= def.CostFood
	pl.Res[resource.Wood] -= def.CostWood
	pl.Res[resource.Gold] -= def.CostGold
	shiftTechQueue(pl)
	pl.TechCur = def.ID
	pl.TechLeft = def.Ticks
}

// completeTech marks the current tech owned, folds its effects into the
// player's modifier set and clears the slot.
func (w *World) completeTech(pl *Player) {
	if bit, ok := w.techBit(pl.TechCur); ok {
		pl.Techs |= bit
	}
	if def := w.tdefs.Get(pl.TechCur); def != nil {
		for _, e := range def.Effects {
			pl.Mods.Apply(e)
		}
	}
	pl.TechCur = 0
	pl.TechLeft = 0
}

func shiftTechQueue(pl *Player) {
	for i := int32(0); i+1 < MaxTechQueue; i++ {
		pl.TechQueue[i] = pl.TechQueue[i+1]
	}
	pl.TechQueue[MaxTechQueue-1] = 0
	if pl.TechReq > 0 {
		pl.TechReq--
	}
}
