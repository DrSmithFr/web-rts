package core

import (
	"testing"

	"rts/sim/biome"
	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
	"rts/sim/terrain"
	"rts/sim/tree"
	"rts/sim/worldgen"
)

// grassWorld builds a 40x40 all-grass world with one player and an
// explicit biome layer (nil = all temperate).
func grassWorld(t *testing.T, biomes []uint8) *World {
	t.Helper()
	grid := terrain.NewGrid(40)
	m := worldgen.CustomWithBiomes(grid, [][2]int{{5, 5}}, nil, nil, biomes)
	return NewWorldWithMap(0x7, m)
}

// addNode places one resource node directly into the world.
func addNode(w *World, kind resource.Kind, sp uint8, tx, ty, amt int32) entity.ID {
	id, e := w.nodes.Create()
	e.Kind, e.Tree = kind, sp
	e.TileX, e.TileY = tx, ty
	e.X, e.Y = tileCenter(tx), tileCenter(ty)
	e.Amount = amt
	return id
}

func nodeAmount(w *World, id entity.ID) int32 {
	return w.nodes.Get(id).Amount
}

// chopSetup returns a fresh world with one full-amount tree node at
// (10,5) and its gather order issued for the owner's first villager. A
// completed sawmill is prebuilt so sawmill-gated species can be chopped.
func chopSetup(t *testing.T, sp uint8) (*World, entity.ID, entity.ID) {
	t.Helper()
	w := grassWorld(t, nil)
	d := w.species.Get(sp)
	if d == nil {
		t.Fatalf("unknown species %d", sp)
	}
	_, sm := w.buildings.Create()
	sm.DefID, sm.Owner, sm.TileX, sm.TileY, sm.Size, sm.HP = BldSawmill, 0, 35, 35, 1, BuildingHP
	node := addNode(w, resource.Wood, sp, 10, 5, 1000)
	u := firstUnit(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(node))}})
	return w, u, node
}

// TestChopSpeedPerSpecies pins the chop-speed column of the species
// table: birch is faster than spruce, baobab slower (W4 config drives it).
// Each world starts identical, so time to take a fixed amount off a node
// compares species directly.
func TestChopSpeedPerSpecies(t *testing.T) {
	const want = 100
	depleteIn := func(sp uint8) int {
		w, _, node := chopSetup(t, sp)
		for i := 0; i < 5000; i++ {
			w.Step(nil)
			if 1000-nodeAmount(w, node) >= want {
				return i
			}
		}
		t.Fatalf("species %d never chopped %d in 5000 ticks", sp, want)
		return -1
	}
	spruce := depleteIn(tree.Spruce)
	birch := depleteIn(tree.Birch)
	baobab := depleteIn(tree.Baobab)
	if birch >= spruce {
		t.Fatalf("birch (%d ticks) should out-chop spruce (%d)", birch, spruce)
	}
	if baobab <= spruce {
		t.Fatalf("baobab (%d ticks) should lag spruce (%d)", baobab, spruce)
	}
}

// TestPalmGivesFoodOnly pins W2: a palm yields food, never wood.
func TestPalmGivesFoodOnly(t *testing.T) {
	w := grassWorld(t, nil)
	d := w.species.Get(tree.Palm)
	if d.Wood != 0 || d.Food == 0 {
		t.Fatalf("palm def wood=%d food=%d", d.Wood, d.Food)
	}
	node := addNode(w, resource.Food, tree.Palm, 10, 5, d.Food)
	woodBefore := w.Players()[0].Res[resource.Wood]
	u := firstUnit(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(node))}})
	for i := 0; i < 20*60 && nodeAmount(w, node) > 0; i++ {
		w.Step(nil)
	}
	if nodeAmount(w, node) != 0 {
		t.Fatal("palm not consumed")
	}
	if got := w.Players()[0].Res[resource.Wood]; got != woodBefore {
		t.Fatalf("wood changed %d -> %d chopping palm", woodBefore, got)
	}
	if w.Players()[0].Res[resource.Food] <= startFood {
		t.Fatal("no food deposited from palm")
	}
}

// TestBaobabNeedsSawmill pins W3: no completed sawmill means no baobab
// chops; building one unlocks them.
func TestBaobabNeedsSawmill(t *testing.T) {
	w := grassWorld(t, nil)
	node := addNode(w, resource.Wood, tree.Baobab, 10, 5, 1000)
	u := firstUnit(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(node))}})
	for i := 0; i < 300; i++ {
		w.Step(nil)
	}
	if nodeAmount(w, node) != 1000 {
		t.Fatal("baobab chopped without a sawmill")
	}
	if vu := w.units.Get(u); vu.State == UnitGather {
		t.Fatal("villager stuck gathering baobab")
	}

	_, sm := w.buildings.Create()
	sm.DefID, sm.Owner, sm.TileX, sm.TileY, sm.Size, sm.HP = BldSawmill, 0, 30, 30, 1, BuildingHP
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(node))}})
	for i := 0; i < 300; i++ {
		w.Step(nil)
	}
	if nodeAmount(w, node) >= 1000 {
		t.Fatal("baobab not chopped with a sawmill")
	}
}

// TestFarmBiomeMultiplier pins H1/H2/C7: identical farms produce at the
// biome multiplier of their tile.
func TestFarmBiomeMultiplier(t *testing.T) {
	size := 40
	biomes := make([]uint8, size*size)
	for i := range biomes {
		biomes[i] = biome.Taiga // 600 milli
	}
	for x := 20; x < size; x++ {
		for y := 0; y < size; y++ {
			biomes[y*size+x] = biome.Prairie // 1250 milli
		}
	}
	run := func(tx, ty int32) int32 {
		w := grassWorld(t, biomes)
		_, f := w.buildings.Create()
		f.DefID, f.Owner, f.TileX, f.TileY, f.Size, f.HP = BldFarm, 0, tx, ty, 1, BuildingHP
		for i := 0; i < 800; i++ {
			w.Step(nil)
		}
		return w.Players()[0].Res[resource.Food] - startFood
	}
	taiga := run(5, 30)   // left half: taiga
	prairie := run(25, 5) // right half: prairie
	if taiga <= 0 || prairie <= 0 {
		t.Fatalf("farms produced nothing: taiga=%d prairie=%d", taiga, prairie)
	}
	// 1250/600 ~= 2.08; allow rounding on 800 ticks.
	if prairie*10 < taiga*18 {
		t.Fatalf("prairie %d should roughly double taiga %d", prairie, taiga)
	}
}

// TestSpeciesTableLoaded checks the tables the client must also see.
func TestSpeciesTableLoaded(t *testing.T) {
	w := grassWorld(t, nil)
	if len(w.TreeDefs().Defs()) != tree.Count-1 {
		t.Fatalf("tree defs %d", len(w.TreeDefs().Defs()))
	}
	if len(w.BiomeDefs().Defs()) != biome.Count {
		t.Fatalf("biome defs %d", len(w.BiomeDefs().Defs()))
	}
}

// TestGatherChainsWood pins chaining: when the chopped node runs out the
// villager continues with the nearest wood node in vision, never another
// resource kind, with no new orders.
func TestGatherChainsWood(t *testing.T) {
	w := grassWorld(t, nil)
	a := addNode(w, resource.Wood, tree.Spruce, 12, 5, 10)
	g := addNode(w, resource.Gold, 0, 13, 5, 300)
	b := addNode(w, resource.Wood, tree.Birch, 14, 5, 1000)
	u := firstUnit(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(a))}})
	for i := 0; i < 20*300; i++ {
		w.Step(nil)
	}
	if nodeAmount(w, a) != 0 {
		t.Fatal("first node never depleted")
	}
	if nodeAmount(w, b) >= 1000 {
		t.Fatal("villager did not chain to the nearest wood node")
	}
	if nodeAmount(w, g) != 300 {
		t.Fatal("chain harvested the wrong resource kind")
	}
}

// TestGatherChainsBerries pins the same chaining for food nodes.
func TestGatherChainsBerries(t *testing.T) {
	w := grassWorld(t, nil)
	a := addNode(w, resource.Food, 0, 12, 8, 4)
	b := addNode(w, resource.Food, 0, 14, 9, 60)
	u := firstUnit(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(a))}})
	for i := 0; i < 20*300; i++ {
		w.Step(nil)
	}
	if nodeAmount(w, a) != 0 {
		t.Fatal("first bush never depleted")
	}
	if nodeAmount(w, b) >= 60 {
		t.Fatal("villager did not chain to the nearest bush")
	}
}

// TestGatherChainsSameSpecies pins the species preference: a felled
// spruce chains to another spruce in vision even when a birch stands
// closer.
func TestGatherChainsSameSpecies(t *testing.T) {
	w := grassWorld(t, nil)
	a := addNode(w, resource.Wood, tree.Spruce, 12, 5, 2)
	birch := addNode(w, resource.Wood, tree.Birch, 14, 5, 500)
	spruce := addNode(w, resource.Wood, tree.Spruce, 10, 7, 500)
	u := firstUnit(w, 0)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(a))}})
	for i := 0; i < 20*300; i++ {
		w.Step(nil)
	}
	if nodeAmount(w, a) != 0 {
		t.Fatal("first tree never felled")
	}
	if nodeAmount(w, spruce) >= 500 {
		t.Fatal("did not chain to the same species")
	}
	if nodeAmount(w, birch) != 500 {
		t.Fatal("closer birch stolen ahead of the same-species spruce")
	}
}

// TestHuntChainsToPrey pins the hunt chain: after a carcass is stripped
// the hunter picks the nearest live prey in vision without new orders.
// The carcass and prey are placed directly to keep the scene small.
func TestHuntChainsToPrey(t *testing.T) {
	w := grassWorld(t, nil)
	u := firstUnit(w, 0)
	corpse := addNode(w, resource.Food, 0, 12, 5, 5)
	w.nodes.Get(corpse).Corpse = true
	deer := spawnUnitAt(t, w, OwnerNeutral, DefDeer, 13, 5)
	w.Step([]proto.Order{{Player: 0, Kind: orders.Gather,
		Payload: orders.GatherPayload(uint32(u), uint32(corpse))}})
	for i := 0; i < 20*300; i++ {
		w.Step(nil)
		if nodeAmount(w, corpse) == 0 {
			break
		}
	}
	if nodeAmount(w, corpse) != 0 {
		t.Fatal("carcass never stripped")
	}
	for i := 0; i < 10 && w.units.Get(u).AttackTarget == entity.Nil; i++ {
		w.Step(nil)
	}
	if got := w.units.Get(u); got.AttackTarget == entity.Nil || got.AttackTarget != deer {
		t.Fatalf("hunter did not chain to the remaining prey: target=%d", got.AttackTarget)
	}
}
