package core

import (
	"rts/sim/entity"
	"rts/sim/units"
)

// Hunger is the drive a hunter carries: 100 (sated) down to 0 (starving).
// Two thresholds change behaviour. Past 20% the animal stops avoiding
// danger - a campfire's glow or a prey standing next to armed villagers
// no longer holds it back. Past 10% it turns enraged (CondRage): it
// covers ground faster and reads aggression wider. At 0 it loses blood
// until it eats or drops.
const (
	HungerFull   int32 = 100
	HungerBold   int32 = 20 // percent: avoidance behaviour falls away
	HungerRage   int32 = 10 // percent: CondRage comes on
	StarveTicks  int32 = 40 // ticks between starvation wounds
	StarveDamage int32 = 1  // HP lost per starvation wound
)

// hungerTick moves one unit's hunger clock and switches the rage flag.
// The tick/id mix keeps the decay spread over the population without
// borrowing randomness, so two replays of the same seed rot at the same
// tick.
func (w *World) hungerTick(id entity.ID, u *Unit, def *units.Def) {
	if !def.Hungry() {
		return
	}
	if (uint64(w.tick)+uint64(id))%uint64(def.HungerDecay) == 0 && u.Hunger > 0 {
		u.Hunger--
	}
	if enraged(u, def) {
		u.Cond |= CondRage
	} else {
		u.Cond &^= CondRage
	}
}

// enraged reports whether hunger drove the animal past the rage point.
func enraged(u *Unit, def *units.Def) bool {
	return def != nil && def.Hungry() && u.Hunger <= HungerFull*HungerRage/100
}

// bold reports whether hunger pushed the animal past the point where it
// still cares about hearth fires and escorted prey.
func bold(u *Unit, def *units.Def) bool {
	return def != nil && def.Hungry() && u.Hunger <= HungerFull*HungerBold/100
}

// eat feeds a hunter on what it just brought down: the carcass food of
// the prey, capped at a full meter.
func (w *World) eat(u *Unit, food int32) {
	def := w.udefs.Get(u.DefID)
	if !def.Hungry() || food <= 0 {
		return
	}
	u.Hunger += food
	if u.Hunger > HungerFull {
		u.Hunger = HungerFull
	}
}

// rageSpeed scales a hunter's walk while enraged.
func rageSpeed(speed int32, def *units.Def, u *Unit) int32 {
	if !enraged(u, def) || def.RageSpeed <= 100 {
		return speed
	}
	return speed * def.RageSpeed / 100
}

// rageAggro scales how wide an enraged hunter reads a threat or prey.
func rageAggro(radius int32, def *units.Def, u *Unit) int32 {
	if !enraged(u, def) || def.RageAggro <= 100 {
		return radius
	}
	return radius * def.RageAggro / 100
}
