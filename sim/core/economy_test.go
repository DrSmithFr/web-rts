package core

import (
	"testing"

	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
)

func firstUnit(w *World, owner uint8) entity.ID {
	var id entity.ID
	w.units.Each(func(cid entity.ID, u *Unit) {
		if id == entity.Nil && u.Owner == owner {
			id = cid
		}
	})
	return id
}

func dropoffID(w *World, owner uint8) entity.ID {
	var id entity.ID
	w.buildings.Each(func(cid entity.ID, b *Building) {
		if id == entity.Nil && b.Owner == owner && b.Progress == 0 {
			if d := w.bdefs.Get(b.DefID); d != nil && d.Dropoff {
				id = cid
			}
		}
	})
	return id
}

func TestStartingEconomy(t *testing.T) {
	w := NewWorld(0x5eed)
	ps := w.Players()
	for i := 0; i < len(w.Map().Spawns()); i++ {
		p := &ps[i]
		if !p.Active {
			t.Fatalf("player %d inactive", i)
		}
		if p.Pop != 3 {
			t.Fatalf("player %d pop %d, want 3", i, p.Pop)
		}
		if p.Res[resource.Wood] != startWood || p.Res[resource.Food] != startFood ||
			p.Res[resource.Stone] != startStone || p.Res[resource.Gold] != startGold {
			t.Fatalf("player %d bad start res %+v", i, p.Res)
		}
		if dropoffID(w, uint8(i)) == entity.Nil {
			t.Fatalf("player %d has no storehouse", i)
		}
	}
	if w.Nodes().Len() < 1000 {
		t.Fatalf("node count %d", w.Nodes().Len())
	}
}

func nearestNode(w *World, owner uint8, kind resource.Kind) entity.ID {
	var best entity.ID
	var bd int64 = 1 << 62
	u := w.units.Get(firstUnit(w, owner))
	w.nodes.Each(func(id entity.ID, n *Node) {
		if n.Kind != kind || n.Amount <= 0 {
			return
		}
		d := int64(n.X-u.X)*int64(n.X-u.X) + int64(n.Y-u.Y)*int64(n.Y-u.Y)
		if d < bd {
			bd, best = d, id
		}
	})
	return best
}

func TestGatherAndDeposit(t *testing.T) {
	w := NewWorld(0x5eed)
	u := firstUnit(w, 0)
	node := nearestNode(w, 0, resource.Wood)
	if node == entity.Nil {
		t.Fatal("no wood node")
	}
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather, Payload: orders.GatherPayload(uint32(u), uint32(node))}})
	before := w.Players()[0].Res[resource.Wood]
	for i := 0; i < 20*120; i++ { // 2 minutes of sim time
		w.Step(nil)
		if w.Players()[0].Res[resource.Wood] > before+resource.CarryCapacity {
			return // multiple trips deposited: full loop works
		}
	}
	t.Fatalf("no wood deposited: %d -> %d", before, w.Players()[0].Res[resource.Wood])
}

func TestBuildCompletes(t *testing.T) {
	w := NewWorld(0x5eed)
	spawn := w.Map().Spawns()[0]
	u := firstUnit(w, 0)
	// pick a free tile just outside the disc, walkable ground is stamped.
	tx, ty := int32(spawn[0])-6, int32(spawn[1])-6
	w.Step([]proto.Order{{Player: 0, Kind: orders.Build, Payload: orders.BuildPayload(uint32(u), 1, uint16(tx), uint16(ty))}})
	var site entity.ID
	w.buildings.Each(func(id entity.ID, b *Building) {
		if b.DefID == 1 {
			site = id
		}
	})
	if site == entity.Nil {
		t.Fatalf("no hut site at %d,%d", tx, ty)
	}
	if w.Players()[0].Res[resource.Wood] != startWood-50 {
		t.Fatalf("cost not charged: %d", w.Players()[0].Res[resource.Wood])
	}
	// one villager building: 60 ticks of work plus walk time
	for i := 0; i < 20*300; i++ {
		w.Step(nil)
		if b := w.buildings.Get(site); b == nil || b.Progress == 0 {
			if w.popCap(0) != StartPopCap+5 {
				t.Fatalf("hut built but no pop room")
			}
			return
		}
	}
	t.Fatal("hut never completed")
}

func TestTrainVillager(t *testing.T) {
	w := NewWorld(0x5eed)
	sh := dropoffID(w, 0)
	foodBefore := w.Players()[0].Res[resource.Food]
	w.Step([]proto.Order{{Player: 0, Kind: orders.Train, Payload: orders.TrainPayload(uint32(sh))}})
	for i := 0; i < 40; i++ {
		w.Step(nil)
	}
	if w.Players()[0].Pop != 4 {
		t.Fatalf("pop %d, want 4", w.Players()[0].Pop)
	}
	if w.Players()[0].Res[resource.Food] != foodBefore-50 {
		t.Fatalf("train food not charged: %d -> %d", foodBefore, w.Players()[0].Res[resource.Food])
	}
	// pop cap blocks training beyond 5 without huts
	for i := 0; i < 10; i++ {
		w.Step([]proto.Order{{Player: 0, Kind: orders.Train, Payload: orders.TrainPayload(uint32(sh))}})
	}
	for i := 0; i < 20*60; i++ {
		w.Step(nil)
	}
	if w.Players()[0].Pop != StartPopCap {
		t.Fatalf("pop %d exceeded cap %d", w.Players()[0].Pop, StartPopCap)
	}
}

func TestMoveOrder(t *testing.T) {
	w := NewWorld(0x5eed)
	u := firstUnit(w, 0)
	start := w.units.Get(u)
	sx, sy := start.X, start.Y
	x16 := uint16((int32(sx) >> 12) + 80) // +5 tiles
	w.Step([]proto.Order{{Player: 0, Kind: orders.Move, Payload: orders.MovePayload([]orders.MoveItem{
		{ID: uint32(u), X16: x16, Y16: uint16(int32(sy) >> 12)},
	})}})
	for i := 0; i < 20*10; i++ {
		w.Step(nil)
	}
	v := w.units.Get(u)
	if v.State != UnitIdle {
		t.Fatalf("state %d, want idle", v.State)
	}
	dx := int32(v.X-sx) - 5*65536
	if dx < -700 || dx > 700 {
		t.Fatalf("x moved %d, want ~5 tiles", dx)
	}
}

func TestDeterminismWithOrders(t *testing.T) {
	build := func() uint64 {
		w := NewWorld(0x1234)
		u := firstUnit(w, 0)
		node := nearestNode(w, 0, resource.Food)
		w.Step([]proto.Order{
			{Player: 0, Kind: orders.Gather, Payload: orders.GatherPayload(uint32(u), uint32(node))},
		})
		for i := 0; i < 500; i++ {
			w.Step([]proto.Order{{Player: 0, Kind: proto.KindNoop}})
		}
		return w.Hash()
	}
	if a, b := build(), build(); a != b {
		t.Fatalf("hash mismatch %x vs %x", a, b)
	}
}
