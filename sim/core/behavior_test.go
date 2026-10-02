package core_test

import (
	"math"
	"testing"

	"rts/sim/angle"
	"rts/sim/core"
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/fog"
	"rts/sim/simtest"
)

// tileCenterOf is the fixed-point middle of a tile, mirroring sim/core's
// unexported tileCenter.
func tileCenterOf(t int32) fixed.F16 { return fixed.F16(t)<<16 + 1<<15 }

// onTileCenter reports whether a coordinate sits exactly in the middle
// of a tile.
func onTileCenter(v fixed.F16) bool { return int32(v)&0xffff == 1<<15 }

// TestFreeMoveNotGridAligned sends one villager 10 tiles across and 3 up.
// The flow field still picks the route, but the walker moves along its own
// heading, so it cuts through open ground instead of stepping centre to
// centre: some tick must sit off the 8-direction centre-to-centre lines
// (fractional offsets differ) and face a heading that is not one of the
// eight grid directions.
func TestFreeMoveNotGridAligned(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/free_move_not_grid_aligned")
	w := s.World()
	u := w.Units().Get(s.Labels().Unit("walker"))
	if u == nil {
		t.Fatal("no walker on the map")
	}
	u.State = core.UnitMove
	u.TX, u.TY = tileCenterOf(13), tileCenterOf(6)
	maxFracDiff, freeHeading, steps := 0.0, false, 0
	for ; steps < 1000 && u.State != core.UnitIdle; steps++ {
		w.Step(nil)
		fx, fy := fracOf(u.X), fracOf(u.Y)
		if d := math.Abs(fx - fy); d > maxFracDiff {
			maxFracDiff = d
		}
		if u.Heading%45 != 0 {
			freeHeading = true
		}
	}
	if !freeHeading {
		t.Fatalf("walker only ever faced the 8 grid directions (%d steps)", steps)
	}
	if maxFracDiff <= 0.25 {
		t.Fatalf("walker never left the centre-to-centre lines (max frac diff %.2f, %d steps)",
			maxFracDiff, steps)
	}
	if tx, ty := tileOfTest(u.X), tileOfTest(u.Y); tx != 13 || ty != 6 {
		t.Fatalf("walker stopped at tile (%d,%d) after %d steps, want (13,6)", tx, ty, steps)
	}
}

// fracOf is the position inside the tile, in [0,1).
func fracOf(v fixed.F16) float64 {
	return float64((int32(v)%65536+65536)%65536) / 65536
}

// TestTurnRateLimit sends a villager east, then back west: the half turn
// takes exactly 180 / turnRate ticks, no more and no less.
func TestTurnRateLimit(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/turn_rate_limit")
	w := s.World()
	u := w.Units().Get(s.Labels().Unit("walker"))
	if u == nil {
		t.Fatal("no walker on the map")
	}
	def := w.UnitDefs().Get(u.DefID)
	if def == nil {
		t.Fatal("no def for walker")
	}
	want := (180 + def.TurnRate - 1) / def.TurnRate // ticks for an about-face

	// East first, so the nose points along +X before the about-face.
	u.State = core.UnitMove
	u.TX, u.TY = tileCenterOf(20), tileCenterOf(10)
	for i := 0; i < 40 && u.Heading != 90; i++ {
		w.Step(nil)
	}
	if u.Heading != 90 {
		t.Fatalf("walker never faced east (heading %d)", u.Heading)
	}

	u.TX, u.TY = tileCenterOf(1), tileCenterOf(10)
	u.Field = entity.Nil // stale goal: the route must be rebuilt west
	var faced int32
	for i := int32(0); i < 3*want && u.Heading != 270; i++ {
		w.Step(nil)
		faced++
	}
	if u.Heading != 270 {
		t.Fatalf("walker never faced west after %d ticks (heading %d), want %d",
			faced, u.Heading, want)
	}
	if faced != want {
		t.Fatalf("about-face took %d ticks, want %d (180 deg at turnRate %d)",
			faced, want, def.TurnRate)
	}
}

func tileOfTest(v fixed.F16) int32 { return int32(v) >> 16 }

// TestBehaviorConeSeesAhead puts a rabbit 5 tiles in front of a wolf's
// nose and another 5 tiles behind it. Both are inside the wolf's vision
// radius; only the one inside its 140 degree cone is worth charging.
func TestBehaviorConeSeesAhead(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/cone_sees_ahead")
	hunter, ahead, behind := s.U("hunter"), s.U("ahead"), s.U("behind")
	s.During(10).Never(func() bool { return hunter.Target() == behind.ID() },
		"wolf charges the rabbit it cannot see")
	s.Within(20).Expect(func() bool { return hunter.Target() == ahead.ID() },
		"wolf charges the rabbit in front of it")
	s.During(120).Never(func() bool { return hunter.Target() == behind.ID() },
		"wolf notices the rabbit behind it")
}

// TestBehaviorWideArcSeesBehind walks a wolf up on a sheep from the
// rear quarter (123 deg off the sheep's nose, 3.6 tiles). Inside the
// sheep's 260 degree arc, outside the wolf's own 140 degree cone, so the
// prey reacts first.
func TestBehaviorWideArcSeesBehind(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/wide_arc_sees_behind")
	sheep := s.U("sheep")
	s.Within(50).Expect(func() bool { return sheep.State() == core.UnitFlee },
		"sheep does not flee a wolf coming from behind")
	s.During(100).Never(sheep.Dead, "fleeing sheep survives the wolf")
}

// TestFogConeOnly leaves a lone villager facing +Y and reads the fog: the
// tile in front of it is revealed, the tile behind it stays dark.
func TestFogConeOnly(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/fog_cone_only")
	w := s.World()
	walker := s.U("walker")
	w.Step(nil)
	if h := walker.Heading(); h != 0 {
		t.Fatalf("walker spawns at heading %d, want 0 (+Y)", h)
	}
	ux, uy := walker.Tile()
	if got := w.FogState(0, int(ux), int(uy)+4); got != fog.Visible {
		t.Fatalf("tile %d ahead of the walker reads %d, want visible", 4, got)
	}
	if got := w.FogState(0, int(ux), int(uy)-4); got == fog.Visible {
		t.Fatalf("tile behind the walker reads visible, want dark (arc %d deg)",
			w.UnitDefs().Get(walker.DefID()).VisionArc)
	}
}

// TestFogCampfireRadius keeps a campfire alone on the map and walks the
// fog edge: 10 tiles out is revealed, 11 tiles out is not.
func TestFogCampfireRadius(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/fog_campfire_radius")
	w := s.World()
	w.Step(nil)
	fire := s.B("fire")
	cx, cy := fire.Tile()
	if got := w.FogState(0, int(cx)+core.CampfireVision, int(cy)); got != fog.Visible {
		t.Fatalf("tile at campfire radius %d reads %d, want visible",
			core.CampfireVision, got)
	}
	if got := w.FogState(0, int(cx)+core.CampfireVision+1, int(cy)); got == fog.Visible {
		t.Fatalf("tile just past campfire radius %d reads visible, want dark",
			core.CampfireVision)
	}
}

// TestFogBuildingsDark parks a completed barracks with nobody around: a
// barracks reveals nothing at all.
func TestFogBuildingsDark(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/fog_buildings_dark")
	w := s.World()
	w.Step(nil)
	site, sy := s.B("barracks").Tile()
	for _, d := range []int32{0, 2, 5} {
		for _, o := range []struct{ dx, dy int32 }{{d, 0}, {-d, 0}, {0, d}, {0, -d}} {
			if got := w.FogState(0, int(site)+int(o.dx), int(sy)+int(o.dy)); got != fog.Unexplored {
				t.Fatalf("barracks revealed tile (%d,%d) as %d, want unexplored",
					site+o.dx, sy+o.dy, got)
			}
		}
	}
}

// gap2 is the squared distance between two labeled units in fixed-point
// units squared: handy for "did they get further apart" without floats.
func gap2(a, b simtest.UnitView) int64 {
	dx, dy := int64(a.X())-int64(b.X()), int64(a.Y())-int64(b.Y())
	return dx*dx + dy*dy
}

// gapPoint2 is the squared distance from a labeled unit to a fixed
// fixed-point spot.
func gapPoint2(u simtest.UnitView, x, y fixed.F16) int64 {
	dx, dy := int64(u.X())-int64(x), int64(u.Y())-int64(y)
	return dx*dx + dy*dy
}

// TestFearAvoidsSeenWolf parks a wolf 3 tiles to a sheep's right, inside
// its 260 degree arc: the sheep takes fear and walks away from it.
func TestFearAvoidsSeenWolf(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/fear_avoids_seen_wolf")
	prey, wolf := s.U("prey"), s.U("wolf")
	s.Within(5).Expect(func() bool { return prey.Cond()&core.CondFear != 0 },
		"sheep does not fear a wolf it can see")
	startX, startGap := prey.X(), gap2(prey, wolf)
	s.Within(80).Expect(func() bool {
		return startX-prey.X() > 1<<16 && gap2(prey, wolf) > startGap &&
			prey.Cond()&core.CondFear != 0
	}, "sheep does not walk away from the wolf it fears")
}

// TestFearIgnoresUnseenWolf moves the same wolf into the sheep's blind
// quarter, straight behind its nose: no fear, no flight.
func TestFearIgnoresUnseenWolf(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/fear_ignores_unseen_wolf")
	prey := s.U("prey")
	s.During(20).Never(func() bool { return prey.Cond() != 0 },
		"sheep reacted to a wolf behind its back")
}

// TestTerrorAfterDamage: the wolf strikes, the deer runs away from where
// the blow landed even when it cannot see the wolf any more.
func TestTerrorAfterDamage(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/terror_after_damage")
	wolf, deer := s.U("wolf"), s.U("deer")
	full := deer.HP()
	s.Within(300).Expect(func() bool { return deer.HP() < full }, "wolf never strikes the deer")
	s.Within(2).Expect(func() bool { return deer.Cond()&core.CondTerror != 0 },
		"the blow leaves no panic")
	// Measure against the ground the wolf stood on when the blow landed:
	// the wolf runs at about the same speed, so a live gap never opens.
	wx, wy := wolf.X(), wolf.Y()
	at := gapPoint2(deer, wx, wy)
	s.Within(12).Expect(func() bool { return gapPoint2(deer, wx, wy) > at },
		"terrified deer does not run away from where it was struck")
}

// TestVillagerFearsPredator: a civilian takes fear at a wolf it sees.
func TestVillagerFearsPredator(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/villager_fears_predator")
	worker := s.U("worker")
	s.Within(5).Expect(func() bool { return worker.Cond()&core.CondFear != 0 },
		"villager does not fear a wolf in plain sight")
}

// TestSoldierNoFear: a lancer stands its ground at the same wolf.
func TestSoldierNoFear(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/soldier_no_fear")
	soldier := s.U("soldier")
	s.During(100).Never(func() bool { return soldier.Cond() != 0 },
		"lancer picked up a condition from a wolf")
}

// maxStride is the longest single-tick stride a unit takes over a window,
// in squared fixed-point units (direction independent, no floats).
func maxStride(w *core.World, u *core.Unit, ticks int) int64 {
	var best int64
	for i := 0; i < ticks; i++ {
		px, py := u.X, u.Y
		w.Step(nil)
		dx, dy := int64(u.X)-int64(px), int64(u.Y)-int64(py)
		if d := dx*dx + dy*dy; d > best {
			best = d
		}
	}
	return best
}

// TestHungerDecays: a lone wolf burns one point of food per hungerDecay
// ticks.
func TestHungerDecays(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/hunger_decays")
	w := s.World()
	hunter := w.Units().Get(s.U("hunter").ID())
	def := w.UnitDefs().Get(hunter.DefID)
	if hunter.Hunger != core.HungerFull {
		t.Fatalf("a fresh hunter stands at hunger %d, want %d", hunter.Hunger, core.HungerFull)
	}
	before := hunter.Hunger
	for i := int32(0); i < def.HungerDecay; i++ {
		w.Step(nil)
	}
	if hunter.Hunger != before-1 {
		t.Fatalf("after %d ticks hunger reads %d, want %d",
			def.HungerDecay, hunter.Hunger, before-1)
	}
}

// TestEatingRefills: the wolf that brings a rabbit down eats it, and its
// food meter jumps by the carcass food the rabbit carried.
func TestEatingRefills(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/eating_refills")
	w := s.World()
	hunter := w.Units().Get(s.U("hunter").ID())
	rabbit := s.U("rabbit")
	food := w.UnitDefs().Get(rabbit.DefID()).CorpseFood
	hunter.Hunger = 40
	before := hunter.Hunger
	s.Within(200).Expect(rabbit.Dead, "wolf never brings the rabbit down")
	// The meter may also have ticked down a point or two while hunting.
	if hunter.Hunger < before+food-3 {
		t.Fatalf("after eating a rabbit hunger reads %d, want at least %d",
			hunter.Hunger, before+food-3)
	}
}

// onAura reports whether a unit stands within radius tiles of a labeled
// building's centre, the metric the campfire repulse uses.
func onAura(u simtest.UnitView, b simtest.BuildingView, radius int32) bool {
	bx, by := b.Tile()
	cx, cy := fixed.F16(bx)<<16+1<<15, fixed.F16(by)<<16+1<<15
	r := int64(radius) << 16
	dx, dy := int64(u.X())-int64(cx), int64(u.Y())-int64(cy)
	return dx*dx+dy*dy <= r*r
}

// TestHungryIgnoresCampfire: at 15% food a wolf roams into the campfire
// aura its sated cousins stay out of.
func TestHungryIgnoresCampfire(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/hungry_ignores_campfire")
	s.World().Units().Get(s.U("hunter").ID()).Hunger = core.HungerFull * 15 / 100
	fire := s.B("fire")
	s.Within(600).Expect(func() bool {
		return onAura(s.U("hunter"), fire, core.CampfireAura)
	}, "starving wolf never walks into the campfire aura")
}

// TestSatedKeepsOffCampfire is the control for the test above: same map,
// same wolf, full belly, and the fire stays ringed by empty ground.
func TestSatedKeepsOffCampfire(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/sated_keeps_off_campfire")
	fire := s.B("fire")
	s.During(600).Never(func() bool {
		return onAura(s.U("hunter"), fire, core.CampfireAura)
	}, "a sated wolf roamed into the campfire aura")
}

// TestRageAt10: at 10% food the wolf turns enraged and its stride gets
// longer.
func TestRageAt10(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/rage_at_10")
	w := s.World()
	hunter := w.Units().Get(s.U("hunter").ID())
	calm := maxStride(w, hunter, 60)
	hunter.Hunger = core.HungerFull * core.HungerRage / 100
	w.Step(nil)
	if hunter.Cond&core.CondRage == 0 {
		t.Fatalf("wolf at %d%% food shows no rage (cond %d)", core.HungerRage, hunter.Cond)
	}
	angry := maxStride(w, hunter, 60)
	if angry <= calm {
		t.Fatalf("enraged stride %d is no longer than the sated one %d", angry, calm)
	}
}

// TestStarveAt0: at zero food the wolf starts losing blood.
func TestStarveAt0(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/starve_at_0")
	w := s.World()
	hunter := w.Units().Get(s.U("hunter").ID())
	hunter.Hunger = 0
	full := hunter.HP
	bleed := false
	for i := 0; i < int(core.StarveTicks)+2 && !bleed; i++ {
		w.Step(nil)
		bleed = hunter.HP < full
	}
	if !bleed {
		t.Fatalf("starving wolf kept all %d HP over %d ticks", full, core.StarveTicks+2)
	}
}

// TestPreferredPrey offers a wolf a sheep 2 tiles away and a rabbit 5
// tiles away: the deer-and-rabbit menu wins over the closer opportunistic
// meal.
func TestPreferredPrey(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/preferred_prey")
	hunter, sheep, rabbit := s.U("hunter"), s.U("sheep"), s.U("rabbit")
	s.Within(10).Expect(func() bool { return hunter.Target() == rabbit.ID() },
		"wolf went for the closer fallback prey instead of the preferred one")
	s.During(60).Never(func() bool { return hunter.Target() == sheep.ID() },
		"wolf switched from its preferred prey to the fallback one")
}

// TestFallbackOnlyWhenHungry: a full wolf walks past a sheep; a wolf down
// to the fallback threshold takes it.
func TestFallbackOnlyWhenHungry(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/fallback_only_when_hungry")
	w := s.World()
	hunter := w.Units().Get(s.U("hunter").ID())
	sheep := s.U("sheep")
	s.During(60).Never(func() bool { return s.U("hunter").Target() == sheep.ID() },
		"a sated wolf hunted a fallback species")
	// Point the nose at the sheep before opening the fallback menu, so the
	// test measures appetite rather than which way the wind blew.
	hunter.Hunger = core.HungerFull * 40 / 100
	hunter.Heading = angle.HeadingTo(sheep.X()-hunter.X, sheep.Y()-hunter.Y)
	s.Within(3).Expect(func() bool { return s.U("hunter").Target() == sheep.ID() },
		"a hungry wolf will not take the fallback prey")
}

func TestBehaviorWolfHuntsRabbit(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/wolf_hunts_rabbit")
	wolf, rabbit := s.U("wolf"), s.U("rabbit")
	s.Within(100).Expect(func() bool { return wolf.State() == core.UnitAttack }, "wolf charges rabbit")
	s.Within(600).Expect(rabbit.Dead, "wolf kills rabbit")
	s.ApproxEq(wolf.HP(), 30, 0, "wolf took no damage")
}

func TestBehaviorSheepFleesWolf(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/sheep_flees_wolf")
	sheep := s.U("sheep")
	s.Within(50).Expect(func() bool { return sheep.State() == core.UnitFlee }, "sheep flees wolf")
	s.During(200).Never(sheep.Dead, "sheep stays alive")
}

func TestBehaviorBearStaysInTerritory(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/bear_stays_in_territory")
	bear := s.U("bear")
	s.During(2000).Never(func() bool { return bear.DistHome() > 20 }, "bear stays on leash")
}

func TestBehaviorAnimalsSleepAtNight(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/animals_sleep_at_night")
	rabbit := s.U("rabbit")
	s.Within(4400).Expect(func() bool { return rabbit.State() == core.UnitSleep }, "rabbit sleeps at night")
}

func TestBehaviorCampfireShelters(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/campfire_shelters")
	rabbit := s.U("rabbit")
	s.During(4000).Never(func() bool {
		x, _ := rabbit.Tile()
		return x < 11
	}, "rabbit crosses the campfire")
}

func TestBehaviorPackAvoidsGuardedVillager(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/pack_avoids_guarded_villager")
	wolf := s.U("wolf")
	vid := s.Labels().Unit("villager")
	s.During(1000).Never(func() bool {
		return wolf.State() == core.UnitAttack && wolf.Target() == vid
	}, "wolf attacks guarded villager")
	s.During(100).Never(s.U("villager").Dead, "guarded villager stays alive")
}
