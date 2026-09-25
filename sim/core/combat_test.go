package core

import (
	"testing"

	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
	"rts/sim/terrain"
	"rts/sim/units"
	"rts/sim/worldgen"
)

// flatWorld gives a 40x40 all-grass island with two spawns and no nodes
// so combat tests control every entity.
func flatWorld(seed uint64) *World {
	grid := terrain.NewGrid(40)
	spawns := [][2]int{{8, 8}, {32, 32}}
	return NewWorldWithMap(seed, worldgen.Custom(grid, spawns, nil))
}

func spawnUnitAt(t *testing.T, w *World, owner uint8, defID uint16, tx, ty int32) entity.ID {
	t.Helper()
	return w.spawnUnit(owner, defID, tileCenter(tx), tileCenter(ty))
}

func TestAttackDamageFormula(t *testing.T) {
	w := flatWorld(1)
	lancer := w.udefs.Get(DefLancer)
	archer := w.udefs.Get(DefArcher)
	scout := w.udefs.Get(DefScout)
	villager := w.udefs.Get(DefVillager)

	target := func(defID uint16) *Unit {
		id := spawnUnitAt(t, w, 0, defID, 5, 5)
		return w.units.Get(id)
	}
	cases := []struct {
		name  string
		att   *units.Def
		defID uint16
		want  int32
	}{
		{"lancer vs scout", lancer, DefScout, 7 + 10 - 1},
		{"archer vs lancer", archer, DefLancer, 5 + 10 - 1},
		{"scout vs archer", scout, DefArcher, 6 + 10 - 0},
		{"lancer vs archer (no bonus)", lancer, DefArcher, 7 - 0},
		{"villager vs boar (armor floor)", villager, DefBoar, 1},
	}
	for _, c := range cases {
		if got := w.attackDamage(c.att, 0, target(c.defID)); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestAttackOrderKillsAndUpdatesPop(t *testing.T) {
	w := flatWorld(2)
	killer := spawnUnitAt(t, w, 0, DefLancer, 10, 10)
	victim := spawnUnitAt(t, w, 1, DefVillager, 11, 10)
	if w.players[1].Pop < 4 {
		t.Fatalf("expected victim pop, got %d", w.players[1].Pop)
	}
	popBefore := w.players[1].Pop
	w.Step([]proto.Order{{Player: 0, Kind: orders.Attack,
		Payload: orders.AttackPayload([]uint32{uint32(killer)}, uint32(victim), orders.TargetUnit)}})
	for i := 0; i < 20*60 && w.units.Alive(victim); i++ {
		w.Step(nil)
	}
	if w.units.Alive(victim) {
		t.Fatal("villager survived 60s against a lancer")
	}
	if w.players[1].Pop != popBefore-1 {
		t.Fatalf("pop not released: %d -> %d", popBefore, w.players[1].Pop)
	}
	ku := w.units.Get(killer)
	if ku == nil || ku.HP <= 0 {
		t.Fatal("lancer should survive attacking an unarmed villager")
	}
}

func TestBoarRetaliates(t *testing.T) {
	w := flatWorld(3)
	attacker := spawnUnitAt(t, w, 0, DefLancer, 10, 10)
	boar := spawnUnitAt(t, w, OwnerNeutral, DefBoar, 11, 10)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Attack,
		Payload: orders.AttackPayload([]uint32{uint32(attacker)}, uint32(boar), orders.TargetUnit)}})
	w.Step(nil)
	b := w.units.Get(boar)
	if b != nil && b.State == UnitAttack && b.AttackTarget == attacker {
		return
	}
	// allow the strike to land (cooldown 0 on first contact)
	for i := 0; i < 20; i++ {
		w.Step(nil)
		b := w.units.Get(boar)
		if b != nil && b.State == UnitAttack && b.AttackTarget == attacker {
			return
		}
	}
	t.Fatal("boar never retaliated")
}

func TestDeerFleesFromVillager(t *testing.T) {
	w := flatWorld(4)
	deer := spawnUnitAt(t, w, OwnerNeutral, DefDeer, 20, 20)
	villager := spawnUnitAt(t, w, 0, DefVillager, 23, 20) // 3 tiles away
	start := w.units.Get(deer)
	d0 := dist2(start.X, start.Y, w.units.Get(villager).X, w.units.Get(villager).Y)
	for i := 0; i < 30; i++ {
		w.Step(nil)
	}
	d := w.units.Get(deer)
	if d == nil {
		t.Fatal("deer vanished")
	}
	if d.State != UnitFlee && d.State != UnitWander {
		t.Fatalf("unexpected deer state %d", d.State)
	}
	d1 := dist2(d.X, d.Y, w.units.Get(villager).X, w.units.Get(villager).Y)
	if d1 <= d0 {
		t.Fatalf("deer did not run: d0=%d d1=%d", d0, d1)
	}
}

func TestHuntLeavesGatherableCorpse(t *testing.T) {
	w := flatWorld(5)
	villager := spawnUnitAt(t, w, 0, DefVillager, 20, 20)
	boar := spawnUnitAt(t, w, OwnerNeutral, DefDeer, 22, 20)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Attack,
		Payload: orders.AttackPayload([]uint32{uint32(villager)}, uint32(boar), orders.TargetUnit)}})
	for i := 0; i < 20*300 && w.units.Alive(boar); i++ {
		w.Step(nil)
	}
	if w.units.Alive(boar) {
		t.Fatal("villager never killed the deer")
	}
	v := w.units.Get(villager)
	if v == nil || v.State != UnitGather || v.Target == entity.Nil {
		t.Fatalf("hunter should gather the corpse, state=%d", v.State)
	}
	n := w.nodes.Get(v.Target)
	if n == nil || n.Kind != resource.Food || n.Amount <= 0 {
		t.Fatalf("no corpse node at gather target")
	}
	food0 := w.players[0].Res[resource.Food]
	for i := 0; i < 20*200 && w.players[0].Res[resource.Food] <= food0; i++ {
		w.Step(nil)
	}
	if w.players[0].Res[resource.Food] <= food0 {
		t.Fatal("corpse food never reached the stockpile")
	}
}

func TestPatrolLoops(t *testing.T) {
	w := flatWorld(6)
	id := spawnUnitAt(t, w, 0, DefScout, 8, 8)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Patrol,
		Payload: orders.PatrolPayload([]uint32{uint32(id)}, 20*16, 8*16)}})
	sawEast, sawWest := false, false
	prevX := int32(w.units.Get(id).X)
	for i := 0; i < 20*120; i++ {
		w.Step(nil)
		x := int32(w.units.Get(id).X)
		if x > prevX {
			sawEast = true
		}
		if sawEast && x < prevX {
			sawWest = true
		}
		prevX = x
	}
	if !sawEast || !sawWest {
		t.Fatalf("patrol never ping-ponged (east=%v west=%v)", sawEast, sawWest)
	}
	if st := w.units.Get(id).State; st != UnitPatrol {
		t.Fatalf("patrol state dropped to %d", st)
	}
}

func TestAutoAcquireNearestEnemy(t *testing.T) {
	w := flatWorld(7)
	near := spawnUnitAt(t, w, 1, DefLancer, 24, 10)
	spawnUnitAt(t, w, 1, DefLancer, 26, 10)
	scout := spawnUnitAt(t, w, 0, DefScout, 22, 10)
	for i := 0; i < 3; i++ {
		w.Step(nil) // reveal via vision, then acquire
	}
	s := w.units.Get(scout)
	if s.State != UnitAttack {
		t.Fatalf("scout did not auto-acquire, state=%d", s.State)
	}
	if s.AttackTarget != near {
		t.Fatalf("acquired %d, nearest was %d", s.AttackTarget, near)
	}
}

func TestCombatDeterminism(t *testing.T) {
	run := func() uint64 {
		w := flatWorld(8)
		lancers := make([]uint32, 0, 3)
		for i := int32(0); i < 3; i++ {
			id := spawnUnitAt(t, w, 0, DefLancer, 8+i, 10)
			lancers = append(lancers, uint32(id))
		}
		archers := make([]uint32, 0, 3)
		for i := int32(0); i < 3; i++ {
			id := spawnUnitAt(t, w, 1, DefArcher, 20, 10+i)
			archers = append(archers, uint32(id))
		}
		w.Step([]proto.Order{
			{Player: 0, Kind: orders.Attack,
				Payload: orders.AttackPayload(lancers, archers[0], orders.TargetUnit)},
		})
		for i := 0; i < 20*90; i++ {
			w.Step(nil)
		}
		return w.Hash()
	}
	if h1, h2 := run(), run(); h1 != h2 {
		t.Fatalf("combat nondeterministic: %x vs %x", h1, h2)
	}
}

// TestAttackEnemyBuilding pins that attack orders work on buildings:
// the lancer walks to the enemy hut, damages it and razes it.
func TestAttackEnemyBuilding(t *testing.T) {
	w := flatWorld(1)
	_, b := w.buildings.Create()
	b.DefID, b.Owner, b.TileX, b.TileY, b.Size, b.HP = BldHut, 1, 20, 20, 1, BuildingHP
	b.Progress = 0
	lancer := spawnUnitAt(t, w, 0, DefLancer, 10, 20)
	var bid entity.ID
	w.buildings.Each(func(id entity.ID, e *Building) {
		if e.Owner == 1 {
			bid = id
		}
	})
	w.Step([]proto.Order{{Player: 0, Kind: orders.Attack,
		Payload: orders.AttackPayload([]uint32{uint32(lancer)}, uint32(bid), orders.TargetBuilding)}})
	l0 := w.units.Get(lancer)
	t.Logf("lancer st=%d atk=%d bid=%d", l0.State, l0.AttackTarget, bid)
	hit := false
	for i := 0; i < 4000; i++ {
		w.Step(nil)
		if eb := w.buildings.Get(bid); eb != nil {
			if eb.HP < BuildingHP {
				hit = true
			}
		} else {
			break
		}
	}
	l1 := w.units.Get(lancer)
	t.Logf("after: lancer st=%d atk=%d pos=(%d,%d)", l1.State, l1.AttackTarget, tileOf(l1.X), tileOf(l1.Y))
	w.buildings.Each(func(id entity.ID, e *Building) {
		t.Logf("bld id=%d def=%d owner=%d hp=%d tile=(%d,%d)", id, e.DefID, e.Owner, e.HP, e.TileX, e.TileY)
	})
	if !hit {
		t.Fatal("building never took damage")
	}
	if w.buildings.Get(bid) != nil {
		t.Fatal("building never razed")
	}
	if got := w.units.Get(lancer); got == nil || got.State != UnitIdle {
		t.Fatal("attacker not idled after razing")
	}
}

// TestRepairResumesConstruction pins bug 2: a stopped builder can be
// sent back with a Repair order and the construction finishes.
func TestRepairResumesConstruction(t *testing.T) {
	w := grassWorld(t, nil)
	u := firstUnit(w, 0)
	w.players[0].Res[resource.Wood] += 500
	w.Step([]proto.Order{{Player: 0, Kind: orders.Build,
		Payload: orders.BuildPayload(uint32(u), BldHut, 12, 5)}})
	var bid entity.ID
	w.buildings.Each(func(id entity.ID, b *Building) {
		if b.DefID == BldHut && b.Owner == 0 {
			bid = id
		}
	})
	if bid == entity.Nil {
		t.Fatal("no hut placed")
	}
	for i := 0; i < 60; i++ {
		w.Step(nil)
	}
	w.Step([]proto.Order{{Player: 0, Kind: orders.Stop,
		Payload: orders.StopPayload([]uint32{uint32(u)})}})
	for i := 0; i < 5; i++ {
		w.Step(nil)
	}
	frozen := w.buildings.Get(bid).Progress
	if frozen <= 0 {
		t.Fatal("nothing left to repair")
	}
	for i := 0; i < 100; i++ {
		w.Step(nil)
	}
	if w.buildings.Get(bid).Progress != frozen {
		t.Fatal("progress moved while interrupted")
	}
	w.Step([]proto.Order{{Player: 0, Kind: orders.Repair,
		Payload: orders.RepairPayload([]uint32{uint32(u)}, uint32(bid))}})
	for i := 0; i < 5000 && w.buildings.Get(bid).Progress > 0; i++ {
		w.Step(nil)
	}
	if b := w.buildings.Get(bid); b == nil || b.Progress != 0 {
		t.Fatal("repair never finished the construction")
	}
}
