package core

import (
	"testing"

	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
)

func placeBuilding(t *testing.T, w *World, defID uint16, owner uint8, tx, ty int32) entity.ID {
	t.Helper()
	def := w.bdefs.Get(defID)
	if def == nil {
		t.Fatalf("no building def %d", defID)
	}
	id, b := w.buildings.Create()
	b.DefID, b.Owner, b.TileX, b.TileY = defID, owner, tx, ty
	b.Size = def.Size
	b.HP = buildingHP(def)
	b.Progress = def.BuildTicks
	w.markOccupied(b)
	return id
}

func buildingHPLeft(t *testing.T, w *World, id entity.ID) int32 {
	t.Helper()
	b := w.buildings.Get(id)
	if b == nil {
		return 0
	}
	return b.HP
}

func TestWatchtowerShootsEnemiesInRange(t *testing.T) {
	w := flatWorld(21)
	tower := placeBuilding(t, w, BldWatchtower, 0, 20, 20)
	victim := spawnUnitAt(t, w, 1, DefVillager, 24, 20)
	for i := 0; i < 20*60 && w.units.Alive(victim); i++ {
		w.Step(nil)
	}
	if w.units.Alive(victim) {
		t.Fatal("villager survived a watchtower")
	}
	if buildingHPLeft(t, w, tower) <= 0 {
		t.Fatal("tower should take no damage from a villager")
	}
}

func TestWatchtowerIgnoresFarAndNeutralTargets(t *testing.T) {
	w := flatWorld(22)
	placeBuilding(t, w, BldWatchtower, 0, 20, 20)
	far := spawnUnitAt(t, w, 1, DefVillager, 32, 20)
	animal := spawnUnitAt(t, w, OwnerNeutral, DefDeer, 21, 20)
	for i := 0; i < 20*120; i++ {
		w.Step(nil)
	}
	if !w.units.Alive(far) {
		t.Fatal("target outside tower range was shot")
	}
	if !w.units.Alive(animal) {
		t.Fatal("tower should not shoot wild animals")
	}
}

func TestSiegeCrushesBuildings(t *testing.T) {
	razorTicks := func(defID uint16) int {
		w := flatWorld(23)
		hut := placeBuilding(t, w, BldHut, 1, 20, 20)
		attacker := spawnUnitAt(t, w, 0, defID, 24, 20)
		if attacker == entity.Nil {
			t.Fatalf("spawn failed for def %d", defID)
		}
		w.Step([]proto.Order{{Player: 0, Kind: orders.Attack,
			Payload: orders.AttackPayload([]uint32{uint32(attacker)}, uint32(hut), orders.TargetBuilding)}})
		for i := 0; i < 20*300; i++ {
			w.Step(nil)
			if !w.buildings.Alive(hut) {
				return i
			}
		}
		return -1
	}
	lancer, ram := razorTicks(DefLancer), razorTicks(DefRam)
	if lancer < 0 || ram < 0 {
		t.Fatalf("building never razed: lancer=%d ram=%d", lancer, ram)
	}
	if ram*2 >= lancer {
		t.Fatalf("ram (%d ticks) is not faster than a lancer (%d ticks) against a hut", ram, lancer)
	}
}

func TestWallsBlockAndAbsorb(t *testing.T) {
	w := flatWorld(24)
	wall := placeBuilding(t, w, BldWall, 0, 20, 20)
	if got := buildingHPLeft(t, w, wall); got != 400 {
		t.Fatalf("wall hp = %d, want 400", got)
	}
	if !w.tileBlocked(20, 20) {
		t.Fatal("wall tile should block ground movement")
	}
	if w.tileBlocked(20, 21) {
		t.Fatal("neighbour tile of a 1x1 wall should stay walkable")
	}
}

func TestSiegeCombatDeterministic(t *testing.T) {
	run := func() uint64 {
		w := flatWorld(25)
		tower := placeBuilding(t, w, BldWatchtower, 0, 20, 20)
		for i := int32(0); i < 3; i++ {
			id := spawnUnitAt(t, w, 1, DefLancer, 22+i, 21)
			l := uint32(id)
			w.Step([]proto.Order{{Player: 1, Kind: orders.Attack,
				Payload: orders.AttackPayload([]uint32{l}, uint32(tower), orders.TargetBuilding)}})
		}
		for i := 0; i < 20*60; i++ {
			w.Step(nil)
		}
		return w.Hash()
	}
	if h1, h2 := run(), run(); h1 != h2 {
		t.Fatalf("siege combat nondeterministic: %x vs %x", h1, h2)
	}
}
