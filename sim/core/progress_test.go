package core

import (
	"testing"

	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
)

// addBuilding plants a completed building def for a player (test helper).
func addBuilding(w *World, owner uint8, defID uint16, tx, ty int32) entity.ID {
	id, b := w.buildings.Create()
	b.DefID = defID
	b.Owner = owner
	b.TileX, b.TileY = tx, ty
	b.Size = 2
	b.Progress = 0
	b.HP = BuildingHP
	w.markOccupied(b)
	return id
}

func TestAgeUpHappyPath(t *testing.T) {
	w := flatWorld(1)
	pl := &w.players[0] // initEconomy already gives a storehouse
	pl.Res[resource.Food] = 400

	// unaffordable: rejected without side effects
	pl.Res[resource.Food] = 249
	w.Step([]proto.Order{{Player: 0, Kind: orders.AgeUp, Payload: orders.AgeUpPayload(1)}})
	if pl.AgeLeft != 0 || pl.Res[resource.Food] != 249 {
		t.Fatal("unaffordable age-up must no-op")
	}

	// skipping an era is rejected
	pl.Res[resource.Food] = 600
	w.Step([]proto.Order{{Player: 0, Kind: orders.AgeUp, Payload: orders.AgeUpPayload(3)}})
	if pl.AgeTarget != 0 || pl.AgeLeft != 0 {
		t.Fatal("era skip must be rejected")
	}

	w.Step([]proto.Order{{Player: 0, Kind: orders.AgeUp, Payload: orders.AgeUpPayload(1)}})
	// progressPhase ticks the timer down in the same tick
	if pl.AgeLeft != w.edefs.Get(1).AgeTicks-1 || pl.Res[resource.Food] != 350 {
		t.Fatalf("age-up did not start: left=%d food=%d", pl.AgeLeft, pl.Res[resource.Food])
	}
	for i := int32(0); i < w.edefs.Get(1).AgeTicks+1; i++ {
		w.Step(nil)
	}
	if pl.Era != 1 {
		t.Fatalf("era=%d after ageing", pl.Era)
	}
}

func TestTrainEraGate(t *testing.T) {
	w := flatWorld(1)
	bid := addBuilding(w, 0, BldBarracks, 14, 14)
	pl := &w.players[0]
	pl.Res[resource.Food] = 500
	pl.Res[resource.Gold] = 100

	w.Step([]proto.Order{{Player: 0, Kind: orders.Train,
		Payload: orders.TrainPayload(uint32(bid), DefLancer)}})
	b := w.buildings.Get(bid)
	if b.TrainCur != 0 || b.TrainReq != 0 {
		t.Fatal("lancer must be gated out of prehistory")
	}

	pl.Era = 1
	w.Step([]proto.Order{{Player: 0, Kind: orders.Train,
		Payload: orders.TrainPayload(uint32(bid), DefLancer)}})
	if b.TrainCur == 0 && b.TrainReq == 0 {
		t.Fatal("lancer must be trainable in antiquity")
	}
}

func TestBuildEraGate(t *testing.T) {
	w := flatWorld(1)
	villagerID := spawnUnitAt(t, w, 0, DefVillager, 20, 20)
	pl := &w.players[0]
	pl.Res[resource.Wood] = 200

	countBarracks := func() int {
		n := 0
		w.buildings.Each(func(_ entity.ID, b *Building) {
			if b.DefID == BldBarracks {
				n++
			}
		})
		return n
	}
	w.Step([]proto.Order{{Player: 0, Kind: orders.Build,
		Payload: orders.BuildPayload(uint32(villagerID), BldBarracks, 18, 18)}})
	if countBarracks() != 0 {
		t.Fatal("barracks must be gated out of prehistory")
	}

	pl.Era = 1
	w.Step([]proto.Order{{Player: 0, Kind: orders.Build,
		Payload: orders.BuildPayload(uint32(villagerID), BldBarracks, 18, 18)}})
	if countBarracks() != 1 {
		t.Fatal("barracks must build in antiquity")
	}
}

func TestResearchChainAndMods(t *testing.T) {
	w := flatWorld(1)
	bid := addBuilding(w, 0, BldBarracks, 14, 14)
	_ = bid
	pl := &w.players[0]
	pl.Era = 1
	pl.Res[resource.Food] = 500
	pl.Res[resource.Gold] = 200

	// casting requires forging: rejected up-front (era gate lifted so the
	// prerequisite is the only thing failing)
	pl.Era = 2
	w.Step([]proto.Order{{Player: 0, Kind: orders.Research, Payload: orders.ResearchPayload(6)}})
	if pl.TechCur != 0 || pl.TechReq != 0 {
		t.Fatal("missing prerequisite must reject")
	}

	w.Step([]proto.Order{{Player: 0, Kind: orders.Research, Payload: orders.ResearchPayload(1)}})
	if pl.TechCur == 0 && pl.TechReq == 0 {
		t.Fatal("forging must queue")
	}
	for i := 0; i < 200 && pl.Mods.Dmg[2] == 0; i++ {
		w.Step(nil)
	}
	if pl.Mods.Dmg[2] != 1 {
		t.Fatal("forging never completed")
	}

	// now casting is accepted (medieval era tech)
	pl.Era = 2
	w.Step([]proto.Order{{Player: 0, Kind: orders.Research, Payload: orders.ResearchPayload(6)}})
	for i := 0; i < 300 && pl.Mods.Dmg[2] == 1; i++ {
		w.Step(nil)
	}
	if pl.Mods.Dmg[2] != 2 {
		t.Fatalf("casting bonus missing: %d", pl.Mods.Dmg[2])
	}

	// duplicate research rejected
	w.Step([]proto.Order{{Player: 0, Kind: orders.Research, Payload: orders.ResearchPayload(6)}})
	if pl.TechReq != 0 || pl.TechCur != 0 {
		t.Fatal("duplicate research must no-op")
	}
}

func TestRelicControlAndVictory(t *testing.T) {
	w := flatWorld(1)
	for _, r := range w.relics {
		spawnUnitAt(t, w, 0, DefScout, r.X, r.Y)
	}
	w.Step(nil)
	for i := 0; i < RelicCount; i++ {
		if w.relics[i].Owner != 0 {
			t.Fatalf("relic %d not claimed", i)
		}
	}
	pl := &w.players[0]
	if pl.Relics != RelicCount {
		t.Fatalf("relics=%d", pl.Relics)
	}
	for i := int32(0); i < RelicGoalTicks+2; i++ {
		w.Step(nil)
	}
	owner, kind := w.Winner()
	if owner != 0 || kind != WinnerRelic {
		t.Fatalf("winner=(%d,%d), want (0,relic)", owner, kind)
	}
}

func TestWonderVictory(t *testing.T) {
	w := flatWorld(1)
	addBuilding(w, 0, BldWonder, 10, 24)
	for i := int32(0); i < WonderGoalTicks+2; i++ {
		w.Step(nil)
	}
	owner, kind := w.Winner()
	if owner != 0 || kind != WinnerWonder {
		t.Fatalf("winner=(%d,%d), want (0,wonder)", owner, kind)
	}
}

func TestConquestVictory(t *testing.T) {
	w := flatWorld(2)
	// wipe player 1's presence
	var dead []entity.ID
	w.units.Each(func(id entity.ID, u *Unit) {
		if u.Owner == 1 {
			dead = append(dead, id)
		}
	})
	for _, id := range dead {
		w.units.Free(id)
	}
	dead = nil
	w.buildings.Each(func(id entity.ID, b *Building) {
		if b.Owner == 1 {
			dead = append(dead, id)
		}
	})
	for _, id := range dead {
		w.buildings.Free(id)
	}
	for i := uint32(0); i < EliminationGrace+2; i++ {
		w.Step(nil)
	}
	owner, kind := w.Winner()
	if owner != 0 || kind != WinnerConquest {
		t.Fatalf("winner=(%d,%d), want (0,conquest)", owner, kind)
	}
}
