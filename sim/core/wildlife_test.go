package core

import (
	"testing"

	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
	"rts/sim/terrain"
	"rts/sim/worldgen"
)

// forestWorld: a 40x40 grass island with a central forest block, for the
// wildlife species tests (habitat species stand on forest; night starts
// at tick 4800 per sim/day).
func forestWorld(seed uint64) *World {
	return NewWorldWithMap(seed, worldgen.Custom(forestGrid(), [][2]int{{2, 2}}, nil))
}

// soloWorld has a single player, so the tests control every unit the
// wild AI can see.
func soloWorld(seed uint64) *World {
	return NewWorldWithMap(seed, worldgen.Custom(forestGrid(), [][2]int{{4, 4}}, nil))
}

func forestGrid() *terrain.Grid {
	grid := terrain.NewGrid(40)
	for y := int32(14); y <= 26; y++ {
		for x := int32(14); x <= 26; x++ {
			grid.SetTile(int(x), int(y), terrain.Forest)
		}
	}
	return grid
}

func placeCampfire(t *testing.T, w *World, tx, ty int32) {
	t.Helper()
	_, b := w.buildings.Create()
	b.DefID = BldCampfire
	b.Owner = 0
	b.TileX, b.TileY = tx, ty
	b.Size = 1
	b.Progress = 0
	b.HP = BuildingHP
	w.markOccupied(b)
}

// attacking reports whether any live unit of defID is mid-attack on v.
func (w *World) attacking(defID uint16, v entity.ID) bool {
	ok := false
	w.units.Each(func(_ entity.ID, u *Unit) {
		if u.DefID == defID && u.State == UnitAttack && u.AttackTarget == v {
			ok = true
		}
	})
	return ok
}

// TestDeerSleepsAtNight: once night falls the deer stands still, even
// with a villager 3 tiles away — inside the panic radius but outside
// the reduced wake radius (5 - SleepWake = 2).
func TestDeerSleepsAtNight(t *testing.T) {
	w := forestWorld(11)
	deer := spawnUnitAt(t, w, OwnerNeutral, DefDeer, 20, 20)
	spawnUnitAt(t, w, 0, DefVillager, 23, 20)
	for w.Tick() < 4900 {
		w.Step(nil)
	}
	d := w.units.Get(deer)
	if d.State != UnitSleep {
		t.Fatalf("deer state %d at night, want sleep", d.State)
	}
	x0, y0 := d.X, d.Y
	for i := 0; i < 100; i++ {
		w.Step(nil)
	}
	d = w.units.Get(deer)
	if d.X != x0 || d.Y != y0 {
		t.Fatal("sleeping deer moved")
	}
}

// TestSleepingDeerWakesToClosePlayer: a player at 1 tile (< wake radius)
// rouses the deer and it flees.
func TestSleepingDeerWakesToClosePlayer(t *testing.T) {
	w := forestWorld(12)
	deer := spawnUnitAt(t, w, OwnerNeutral, DefDeer, 20, 20)
	for w.Tick() < 4900 {
		w.Step(nil)
	}
	if w.units.Get(deer).State != UnitSleep {
		t.Fatal("deer not asleep before the player arrives")
	}
	v := spawnUnitAt(t, w, 0, DefVillager, tileOf(w.units.Get(deer).X)+1, tileOf(w.units.Get(deer).Y))
	for i := 0; i < 5; i++ {
		w.Step(nil)
		if w.units.Get(deer).State != UnitSleep {
			break
		}
	}
	if w.units.Get(deer).State == UnitSleep {
		t.Fatal("deer slept through a villager at 1 tile")
	}
	dv := dist2(w.units.Get(deer).X, w.units.Get(deer).Y, w.units.Get(v).X, w.units.Get(v).Y)
	for i := 0; i < 60; i++ {
		w.Step(nil)
	}
	d := w.units.Get(deer)
	if dist2(d.X, d.Y, w.units.Get(v).X, w.units.Get(v).Y) <= dv {
		t.Fatal("woken deer did not run")
	}
}

// TestBoarChargesPlayer: a villager within 6 tiles gets charged without
// being struck first (aggression, not retaliation).
func TestBoarChargesPlayer(t *testing.T) {
	w := forestWorld(13)
	spawnUnitAt(t, w, OwnerNeutral, DefBoar, 20, 20)
	v := spawnUnitAt(t, w, 0, DefVillager, 24, 20)
	for i := 0; i < 40; i++ {
		w.Step(nil)
		if w.attacking(DefBoar, v) {
			return
		}
	}
	t.Fatal("boar never charged")
}

// TestCampfireShieldsVillagerFromBoar: the boar charges, but once the
// villager shelters at a campfire the boar gives up.
func TestCampfireShieldsVillagerFromBoar(t *testing.T) {
	w := forestWorld(14)
	spawnUnitAt(t, w, OwnerNeutral, DefBoar, 20, 20)
	v := spawnUnitAt(t, w, 0, DefVillager, 24, 20)
	charged := false
	for i := 0; i < 40; i++ {
		w.Step(nil)
		if w.attacking(DefBoar, v) {
			charged = true
			break
		}
	}
	if !charged {
		t.Fatal("boar did not charge initially")
	}
	// the villager reaches the torchlight: campfire lit beside them
	placeCampfire(t, w, tileOf(w.units.Get(v).X), tileOf(w.units.Get(v).Y)+1)
	for i := 0; i < 80; i++ {
		w.Step(nil)
	}
	if w.attacking(DefBoar, v) {
		t.Fatal("boar kept attacking through the campfire shield")
	}
}

// TestCampfireRepelsDeer: a shy animal inside the aura flees outward.
func TestCampfireRepelsDeer(t *testing.T) {
	w := forestWorld(15)
	deer := spawnUnitAt(t, w, OwnerNeutral, DefDeer, 20, 20)
	placeCampfire(t, w, 23, 20)
	for i := 0; i < 5; i++ {
		w.Step(nil)
	}
	d := w.units.Get(deer)
	if d.State != UnitFlee {
		t.Fatalf("deer state %d in campfire aura, want flee", d.State)
	}
	d0 := dist2(d.X, d.Y, tileCenter(23), tileCenter(20))
	for i := 0; i < 100; i++ {
		w.Step(nil)
	}
	d = w.units.Get(deer)
	if dist2(d.X, d.Y, tileCenter(23), tileCenter(20)) <= d0 {
		t.Fatal("deer did not back out of the campfire aura")
	}
}

// TestBearStaysOnCampfireVictim: bears ignore the campfire shield.
func TestBearStaysOnCampfireVictim(t *testing.T) {
	w := forestWorld(16)
	placeCampfire(t, w, 21, 19)
	spawnUnitAt(t, w, OwnerNeutral, DefBear, 20, 20)
	v := spawnUnitAt(t, w, 0, DefVillager, 21, 20)
	for i := 0; i < 60; i++ {
		w.Step(nil)
		if w.attacking(DefBear, v) {
			return
		}
	}
	t.Fatal("bear never attacked next to a campfire")
}

// TestBearLeashPullsHome: a bear fighting far from its spawn drops the
// chase and heads back inside its leash.
func TestBearLeashPullsHome(t *testing.T) {
	w := forestWorld(17)
	bear := spawnUnitAt(t, w, OwnerNeutral, DefBear, 5, 5)
	v := spawnUnitAt(t, w, 0, DefVillager, 33, 34)
	b := w.units.Get(bear)
	// teleport the bear far from home mid-chase (white-box leash test)
	b.X, b.Y = tileCenter(30), tileCenter(30)
	b.State = UnitAttack
	b.AttackTarget = v
	b.AttackKind = orders.TargetUnit
	w.Step(nil)
	b = w.units.Get(bear)
	if b.State == UnitAttack {
		t.Fatal("bear ignored its leash")
	}
	if b.TX != b.HomeX || b.TY != b.HomeY {
		t.Fatalf("leashed bear does not head home: TX=%d TY=%d home=%d,%d", b.TX, b.TY, b.HomeX, b.HomeY)
	}
}

// TestWolfHuntsLoneVillager: wolves hunt humans at night.
func TestWolfHuntsLoneVillager(t *testing.T) {
	w := soloWorld(18)
	spawnUnitAt(t, w, OwnerNeutral, DefWolf, 20, 20)
	v := spawnUnitAt(t, w, 0, DefVillager, 24, 20)
	for i := 0; i < 40; i++ {
		w.Step(nil)
		if w.attacking(DefWolf, v) {
			return
		}
	}
	t.Fatal("wolf did not hunt a lone villager at night")
}

// TestWolfSleepsAtNight: wolves den at night, hunting with the daylight.
func TestWolfSleepsAtNight(t *testing.T) {
	w := soloWorld(19)
	wolf := spawnUnitAt(t, w, OwnerNeutral, DefWolf, 20, 20)
	v := spawnUnitAt(t, w, 0, DefVillager, 28, 20)
	for w.Tick() < 4900 {
		w.Step(nil)
	}
	x0, y0 := w.units.Get(wolf).X, w.units.Get(wolf).Y
	for i := 0; i < 100; i++ {
		w.Step(nil)
	}
	if w.attacking(DefWolf, v) {
		t.Fatal("wolf hunted during the day")
	}
	u := w.units.Get(wolf)
	if u.X != x0 || u.Y != y0 {
		t.Fatal("wolf prowled during the day")
	}
}

// TestWolfAvoidsGroups: an escorted villager (three herd-mates within
// guard distance) is left alone; the wolf takes the isolated one.
func TestWolfAvoidsGroups(t *testing.T) {
	w := soloWorld(20)
	spawnUnitAt(t, w, OwnerNeutral, DefWolf, 20, 20)
	for _, p := range [][2]int32{{24, 20}, {24, 21}, {25, 20}, {25, 21}} {
		spawnUnitAt(t, w, 0, DefVillager, p[0], p[1])
	}
	lone := spawnUnitAt(t, w, 0, DefVillager, 20, 15)
	for i := 0; i < 200; i++ {
		w.Step(nil)
		if w.attacking(DefWolf, lone) {
			return
		}
	}
	t.Fatal("wolf did not pick the isolated villager")
}

// TestBlackWolfHuntsAtNight: the dark wolf hunts at night and den-sleeps
// through the day — the mirror schedule of its grey cousin.
func TestBlackWolfHuntsAtNight(t *testing.T) {
	w := soloWorld(22)
	black := spawnUnitAt(t, w, OwnerNeutral, DefBlackWolf, 20, 20)
	v := spawnUnitAt(t, w, 0, DefVillager, 27, 20)
	for w.Tick() < 4330 {
		w.Step(nil)
	}
	if u := w.units.Get(black); u.X == 0 {
		t.Fatal("black wolf vanished")
	}
	for i := 0; i < 120; i++ {
		w.Step(nil)
		if w.attacking(DefBlackWolf, v) {
			break
		}
	}
	if !w.attacking(DefBlackWolf, v) {
		t.Fatal("black wolf did not hunt at night")
	}
}

// TestBlackWolfSleepsByDay: morning to dusk the black wolf stands fast.
func TestBlackWolfSleepsByDay(t *testing.T) {
	w := soloWorld(23)
	black := spawnUnitAt(t, w, OwnerNeutral, DefBlackWolf, 20, 20)
	x0, y0 := w.units.Get(black).X, w.units.Get(black).Y
	for i := 0; i < 200; i++ {
		w.Step(nil)
	}
	u := w.units.Get(black)
	if u.State != UnitSleep {
		t.Fatalf("black wolf state %d by day, want sleep", u.State)
	}
	if u.X != x0 || u.Y != y0 {
		t.Fatal("black wolf prowled during the day")
	}
}

// TestRabbitHuntedLeavesCorpse: rabbits feed like the bigger game.
func TestRabbitHuntedLeavesCorpse(t *testing.T) {
	w := forestWorld(21)
	villager := spawnUnitAt(t, w, 0, DefVillager, 20, 20)
	rabbit := spawnUnitAt(t, w, OwnerNeutral, DefRabbit, 21, 20)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Attack,
		Payload: orders.AttackPayload([]uint32{uint32(villager)}, uint32(rabbit), orders.TargetUnit)}})
	for i := 0; i < 20*60 && w.units.Alive(rabbit); i++ {
		w.Step(nil)
	}
	if w.units.Alive(rabbit) {
		t.Fatal("villager never caught the rabbit")
	}
	corpses := 0
	w.nodes.Each(func(_ entity.ID, n *Node) {
		if n.Corpse && n.Kind == resource.Food && n.Amount > 0 {
			corpses++
		}
	})
	if corpses == 0 {
		t.Fatal("rabbit hunt left no corpse")
	}
}
