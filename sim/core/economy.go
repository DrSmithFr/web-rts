package core

import (
	"fmt"

	"rts/sim/building"
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/orders"
	"rts/sim/path"
	"rts/sim/resource"
	"rts/sim/terrain"
	"rts/sim/units"
)

// speedOf returns a unit's per-tick move speed, including tech bonuses.
func (w *World) speedOf(u *Unit) fixed.F16 {
	d := w.udefs.Get(u.DefID)
	if d == nil || d.Speed16 <= 0 {
		return VillagerSpeed
	}
	speed := d.Speed16
	if u.Owner < MaxPlayers {
		speed += w.players[u.Owner].Mods.Speed16[d.Class]
	}
	// A starving hunter runs faster than a well-fed one.
	speed = rageSpeed(speed, d, u)
	return fixed.F16(speed)
}

// visionOf returns a unit's fog reveal radius in tiles.
func (w *World) visionOf(u *Unit) int {
	if d := w.udefs.Get(u.DefID); d != nil && d.Vision > 0 {
		return int(d.Vision)
	}
	return VillagerVision
}

// carryBonus returns the extra carry capacity tech grants this unit.
func (w *World) carryBonus(u *Unit) int32 {
	if u.Owner >= MaxPlayers {
		return 0
	}
	d := w.udefs.Get(u.DefID)
	if d == nil {
		return 0
	}
	return w.players[u.Owner].Mods.Carry[d.Class]
}

// stampNodes turns the map's node table into harvestable Node entities.
func (w *World) stampNodes() {
	for _, n := range w.gameMap.Nodes() {
		_, e := w.nodes.Create()
		e.Kind = n.Kind
		e.Tree = n.Tree
		e.TileX, e.TileY = n.X, n.Y
		e.X = tileCenter(n.X)
		e.Y = tileCenter(n.Y)
		e.Amount = n.Amount
	}
}

// startBuildingOffsets are candidate storehouse footprints (top-left tile,
// relative to spawn) tried in order until one is walkable and free.
var startBuildingOffsets = [][2]int32{{-4, -1}, {-4, 0}, {3, -1}, {3, 0}, {-1, -4}, {0, -4}, {-1, 3}, {0, 3}}

// villagerOffsets place the starting villagers inside the stamped grass disc.
var villagerOffsets = [][2]int32{{1, 1}, {-1, 1}, {1, -1}}

// applyStartKit places the procedural starting kit for one player: a
// storehouse on the first free candidate offset around the spawn and
// three villagers inside the stamped grass disc.
func (w *World) applyStartKit(player uint8) {
	spawns := w.gameMap.Spawns()
	if int(player) >= len(spawns) {
		return
	}
	s := spawns[player]
	def := w.bdefs.Get(buildingStorehouse)
	for _, o := range startBuildingOffsets {
		tx, ty := int32(s[0])+o[0], int32(s[1])+o[1]
		if w.footprintFree(tx, ty, def.Size) {
			_, b := w.buildings.Create()
			b.DefID = buildingStorehouse
			b.Owner = player
			b.TileX, b.TileY = tx, ty
			b.Size = def.Size
			b.HP = buildingHP(def)
			w.markOccupied(b)
			w.clearNodesUnder(tx, ty, def.Size)
			break
		}
	}
	for _, o := range villagerOffsets {
		w.spawnVillagerAt(player, int32(s[0])+o[0], int32(s[1])+o[1])
	}
}

// initEconomy seeds nodes, players, a storehouse and three villagers per
// spawn. Must run after the map and stores exist.
func (w *World) initEconomy() {
	w.stampNodes()
	for i := range w.gameMap.Spawns() {
		p := &w.players[i]
		p.Active = true
		p.Res[resource.Wood] = startWood
		p.Res[resource.Stone] = startStone
		p.Res[resource.Food] = startFood
		p.Res[resource.Gold] = startGold
		w.applyStartKit(uint8(i))
	}
	for _, a := range w.gameMap.Animals() {
		w.spawnUnit(OwnerNeutral, a.Def, tileCenter(a.X), tileCenter(a.Y))
	}
}

const buildingStorehouse uint16 = 3

func tileCenter(t int32) fixed.F16 {
	return fixed.F16(t)<<16 + 1<<15
}

func (w *World) inBounds(x, y int32) bool {
	return w.gameMap.Grid().InBounds(int(x), int(y))
}

// footprintFree checks a size x size area for bounds, terrain and any
// already-placed building (construction sites block too).
// clearNodesUnder frees resource nodes covered by a starting building's
// footprint, so bases never spawn on top of trees or berries.
func (w *World) clearNodesUnder(tx, ty, size int32) {
	var dead []entity.ID
	w.nodes.Each(func(id entity.ID, n *Node) {
		if n.TileX >= tx && n.TileX < tx+size && n.TileY >= ty && n.TileY < ty+size {
			dead = append(dead, id)
		}
	})
	for _, id := range dead {
		w.nodes.Free(id)
	}
}

func (w *World) footprintFree(tx, ty, size int32) bool {
	for dy := int32(0); dy < size; dy++ {
		for dx := int32(0); dx < size; dx++ {
			if !w.inBounds(tx+dx, ty+dy) || !w.tileWalkable(tx+dx, ty+dy) {
				return false
			}
		}
	}
	return true
}

// tileWalkable reports terrain and building occupancy for a tile.
func (w *World) tileWalkable(x, y int32) bool {
	width, height := int32(w.gameMap.Grid().Width()), int32(w.gameMap.Grid().Height())
	if x < 0 || y < 0 || x >= width || y >= height {
		return false
	}
	if w.occupied[y*width+x] {
		return false
	}
	t := w.gameMap.Grid().Tile(int(x), int(y))
	return t != terrain.Shallow && t != terrain.Deep
}

// buildingHP returns the def HP, falling back to the historical default.
func buildingHP(def *building.Def) int32 {
	if def.HP > 0 {
		return def.HP
	}
	return BuildingHP
}

// tileBlocked reports only building occupancy (units may share ground).
func (w *World) tileBlocked(x, y int32) bool {
	width, height := int32(w.gameMap.Grid().Width()), int32(w.gameMap.Grid().Height())
	if x < 0 || y < 0 || x >= width || y >= height {
		return true
	}
	return w.occupied[y*width+x]
}

func (w *World) markOccupied(b *Building) {
	width, height := w.gameMap.Grid().Width(), w.gameMap.Grid().Height()
	for dy := int32(0); dy < b.Size; dy++ {
		for dx := int32(0); dx < b.Size; dx++ {
			x, y := int(b.TileX)+int(dx), int(b.TileY)+int(dy)
			if x >= 0 && y >= 0 && x < width && y < height {
				w.occupied[y*width+x] = true
			}
		}
	}
}

// updateVision advances fog of war: visible decays to explored, then the
// player's own units reveal fresh ground in their vision cone. Only
// completed campfires reveal ground on their own; every other building
// and every half-built site stays dark.
func (w *World) updateVision() {
	for i := 0; i < MaxPlayers; i++ {
		if w.players[i].Active {
			w.fogs[i].Decay()
		}
	}
	w.units.Each(func(_ entity.ID, u *Unit) {
		if u.Owner >= MaxPlayers || u.State == UnitSleep {
			return // sleepers keep their eyes shut, so the fog stays shut
		}
		if !w.players[u.Owner].Active {
			return
		}
		w.fogs[u.Owner].ApplyCone(int(tileOf(u.X)), int(tileOf(u.Y)),
			w.visionOf(u), int(u.Heading), int(w.visionArcOf(u)))
	})
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if !w.players[b.Owner].Active || b.DefID != BldCampfire || b.Progress != 0 || b.HP <= 0 {
			return
		}
		cx := int(b.TileX) + int(b.Size-1)/2
		cy := int(b.TileY) + int(b.Size-1)/2
		w.fogs[b.Owner].ApplyCircle(cx, cy, CampfireVision)
	})
}

func (w *World) spawnVillagerAt(owner uint8, tx, ty int32) {
	w.spawnUnit(owner, DefVillager, tileCenter(tx), tileCenter(ty))
}

// spawnUnit creates a unit of the given def, counts population for real
// players and returns its id. Unknown defs panic (data bug).
func (w *World) spawnUnit(owner uint8, defID uint16, x, y fixed.F16) entity.ID {
	def := w.udefs.Get(defID)
	if def == nil {
		panic(fmt.Sprintf("core: spawn unknown unit def %d", defID))
	}
	id, u := w.units.Create()
	u.DefID = defID
	u.Kind = def.Kind()
	u.Owner = owner
	u.X, u.Y = x, y
	u.HomeX, u.HomeY = x, y
	u.Relic = NoRelic
	u.HP = def.HP
	u.Hunger = HungerFull
	if owner < MaxPlayers {
		w.players[owner].Pop += def.PopCost
	}
	return id
}

// spawnVillagerNear places a trained unit on the first free tile in a
// widening ring around the building.
func (w *World) spawnUnitNear(owner uint8, udef *units.Def, b *Building) {
	cx, cy := b.TileX+(b.Size-1)/2, b.TileY+(b.Size-1)/2
	for r := int32(1); r <= 4; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				tx, ty := cx+dx, cy+dy
				if !w.tileWalkable(tx, ty) {
					continue
				}
				if !w.occupied[int(ty)*w.gameMap.Grid().Width()+int(tx)] {
					w.spawnUnit(owner, udef.ID, tileCenter(tx), tileCenter(ty))
					return
				}
			}
		}
	}
	w.spawnUnit(owner, udef.ID, tileCenter(cx), tileCenter(cy))
}

func (w *World) popCap(owner uint8) int32 {
	cap := StartPopCap
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if b.Owner == owner && b.Progress == 0 {
			if d := w.bdefs.Get(b.DefID); d != nil {
				cap += d.PopRoom
			}
		}
	})
	return cap
}

// applyMove applies a group move order: every listed unit gets a direct
// goal tile. For shared flow fields use MoveGroup.
func (w *World) applyMove(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	n := int(r.Uvarint())
	if n > 512 {
		return false
	}
	for i := 0; i < n; i++ {
		id := entity.ID(r.Uvarint())
		tx := fixed.F16(int32(r.U16()) << 12)
		ty := fixed.F16(int32(r.U16()) << 12)
		u := w.units.Get(id)
		if u == nil || u.Owner != player {
			continue
		}
		u.TX, u.TY = tx, ty
		u.Target = entity.Nil
		u.Field = entity.Nil
		u.State = UnitMove
	}
	return r.OK()
}

// applyMoveGroup moves a group to one base tile with a shared flow field
// and deterministic spiral formation slots assigned in id order.
func (w *World) applyMoveGroup(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	n := int(r.Uvarint())
	if n == 0 || n > 128 {
		return false
	}
	ids := make([]entity.ID, 0, n)
	for i := 0; i < n; i++ {
		id := entity.ID(r.Uvarint())
		if u := w.units.Get(id); u != nil && u.Owner == player {
			ids = append(ids, id)
		}
	}
	goalX := int32(r.U16()) >> 4
	goalY := int32(r.U16()) >> 4
	if !r.OK() {
		return false
	}
	if !w.tileWalkable(goalX, goalY) {
		return true // well-formed order onto unwalkable ground: no-op
	}

	origins := make([][2]int32, 0, len(ids))
	for _, id := range ids {
		u := w.units.Get(id)
		origins = append(origins, [2]int32{tileOf(u.X), tileOf(u.Y)})
	}
	fieldID := w.addFlowField(origins, goalX, goalY)

	reserved := make([][2]int32, 0, len(ids))
	for i, id := range ids {
		u := w.units.Get(id)
		slot := w.reserveSlot(goalX, goalY, i, reserved)
		reserved = append(reserved, slot)
		u.TX, u.TY = tileCenter(slot[0]), tileCenter(slot[1])
		u.Target = entity.Nil
		u.Field = fieldID
		u.FieldX, u.FieldY = goalX, goalY
		u.State = UnitMove
	}
	return true
}

func tileOf(v fixed.F16) int32 {
	if v >= 0 {
		return int32(v) >> 16
	}
	return -((-int32(v) + 65535) >> 16)
}

// reserveSlot picks the i-th spiral offset from base that is walkable,
// in bounds and not already reserved in this order; falls back to base.
func (w *World) reserveSlot(baseX, baseY int32, i int, reserved [][2]int32) [2]int32 {
	for k := i; k < len(path.SpiralOffsets); k++ {
		cand := [2]int32{baseX + path.SpiralOffsets[k][0], baseY + path.SpiralOffsets[k][1]}
		if !w.tileWalkable(cand[0], cand[1]) {
			continue
		}
		taken := false
		for _, r := range reserved {
			if r == cand {
				taken = true
				break
			}
		}
		if !taken {
			return cand
		}
	}
	return [2]int32{baseX, baseY}
}

// addFlowField computes one BFS field covering origins and goal and
// stores it, evicting the oldest field when the pool is full.
func (w *World) addFlowField(origins [][2]int32, goalX, goalY int32) entity.ID {
	const half = MaxFieldSpan / 2
	clamp := func(v, g int32) int32 {
		if v < g-half {
			return g - half
		}
		if v > g+half {
			return g + half
		}
		return v
	}
	width, height := int32(w.gameMap.Grid().Width()), int32(w.gameMap.Grid().Height())
	minX, minY, maxX, maxY := goalX, goalY, goalX, goalY
	add := func(x, y int32) {
		x, y = clamp(x, goalX), clamp(y, goalY)
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	for _, o := range origins {
		add(o[0], o[1])
	}
	ox, oy := minX-FieldMargin, minY-FieldMargin
	ox2, oy2 := maxX+FieldMargin, maxY+FieldMargin
	if ox < 0 {
		ox = 0
	}
	if oy < 0 {
		oy = 0
	}
	if ox2 >= width {
		ox2 = width - 1
	}
	if oy2 >= height {
		oy2 = height - 1
	}

	if w.fields.Len() >= MaxFields {
		var oldest entity.ID
		w.fields.Each(func(id entity.ID, _ *path.Field) {
			if oldest == entity.Nil {
				oldest = id
			}
		})
		if oldest != entity.Nil {
			w.units.Each(func(_ entity.ID, u *Unit) {
				if u.Field == oldest {
					u.Field = entity.Nil
				}
			})
			w.fields.Free(oldest)
		}
	}

	fid, f := w.fields.Create()
	*f = path.ComputeField(w.tileWalkable, ox, oy, ox2-ox+1, oy2-oy+1, goalX, goalY)
	return fid
}

func (w *World) applyGather(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	unit := entity.ID(r.Uvarint())
	node := entity.ID(r.Uvarint())
	if u := w.units.Get(unit); u != nil && u.Owner == player {
		if u.Kind != KindVillager {
			return true // only villagers harvest; soldiers fight
		}
		if n := w.nodes.Get(node); n != nil && n.Kind.Harvestable() && n.Amount > 0 {
			if w.speciesGated(n) && !w.hasSawmill(u.Owner) {
				return true // well-formed but not allowed yet: no-op
			}
			if u.Carried > 0 && u.CarriedK != n.Kind {
				u.Carried = 0 // dropping a different cargo is acceptable
			}
			u.CarriedK = n.Kind
			u.Target = node
			u.AttackTarget = entity.Nil // a gather order drops the hunt
			u.AttackKind = 0
			u.State = UnitGather
		}
	}
	return r.OK()
}

// speciesGated reports whether a node's tree species (baobab) requires a
// completed sawmill before it may be chopped.
func (w *World) speciesGated(n *Node) bool {
	d := w.species.Get(n.Tree)
	return d != nil && d.Sawmill
}

// hasSawmill reports whether the owner has a completed sawmill.
func (w *World) hasSawmill(owner uint8) bool {
	found := false
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if b.Owner == owner && b.DefID == BldSawmill && b.Progress == 0 {
			found = true
		}
	})
	return found
}

// applyRepair assigns villagers to an interrupted construction. Only
// villagers build, and only buildings of one's own that are still
// unfinished (Progress > 0) accept workers.
func (w *World) applyRepair(player uint8, p []byte) bool {
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
	if !r.OK() {
		return false
	}
	b := w.buildings.Get(target)
	if b == nil || b.Owner != player || b.Progress <= 0 {
		return true // well-formed order at a finished building: no-op
	}
	for _, id := range ids {
		u := w.units.Get(id)
		if u == nil || u.Owner != player || u.Kind != KindVillager {
			continue
		}
		u.State = UnitBuild
		u.Target = target
	}
	return true
}

func (w *World) applyBuild(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	unit := entity.ID(r.Uvarint())
	defID := r.U16()
	tx := int32(r.U16())
	ty := int32(r.U16())
	if !r.OK() {
		return false
	}
	def := w.bdefs.Get(defID)
	if def == nil {
		return true
	}
	u := w.units.Get(unit)
	if u == nil || u.Owner != player {
		return true
	}
	if int32(w.players[u.Owner].Era) < def.MinEra {
		return true
	}
	if !w.footprintFree(tx, ty, def.Size) {
		return true
	}
	pl := &w.players[u.Owner]
	if pl.Res[resource.Wood] < def.CostWood || pl.Res[resource.Stone] < def.CostStone {
		return true
	}
	pl.Res[resource.Wood] -= def.CostWood
	pl.Res[resource.Stone] -= def.CostStone
	bid, b := w.buildings.Create()
	b.DefID = defID
	b.Owner = u.Owner
	b.TileX, b.TileY = tx, ty
	b.Size = def.Size
	b.Progress = def.BuildTicks
	b.HP = buildingHP(def)
	u.State = UnitBuild
	u.Target = bid
	return true
}

func (w *World) applyStop(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	n := int(r.Uvarint())
	if n > 512 {
		return false
	}
	for i := 0; i < n; i++ {
		id := entity.ID(r.Uvarint())
		if u := w.units.Get(id); u != nil && u.Owner == player {
			u.State = UnitIdle
			u.Target = entity.Nil
			u.AttackTarget = entity.Nil
			u.AttackKind = 0
		}
	}
	return r.OK()
}

// applyTrain queues one unit def at a building. Payloads without a unit
// def keep the historical "train a villager" meaning. Costs and the
// population cap are checked when training actually starts.
func (w *World) applyTrain(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	id := entity.ID(r.Uvarint())
	defID := uint16(DefVillager)
	if r.Remaining() > 0 {
		defID = uint16(r.Uvarint())
	}
	if !r.OK() {
		return false
	}
	b := w.buildings.Get(id)
	if b == nil || b.Owner != player {
		return true
	}
	if b.TrainReq >= MaxTrainQueue {
		return true
	}
	udef := w.udefs.Get(defID)
	if udef == nil || udef.TrainBy != b.DefID {
		return true
	}
	if int32(w.players[player].Era) < udef.MinEra {
		return true
	}
	b.TrainQueue[b.TrainReq] = defID
	b.TrainReq++
	return true
}

// resumeWork puts a worker that ran from something back on its job: the
// node or site it left if it still stands, the drop-off trip if it is
// carrying, otherwise idle for the brain to re-task.
func (w *World) resumeWork(u *Unit) {
	u.State = UnitIdle
	if u.Carried > 0 {
		u.State = UnitReturn
		return
	}
	if n := w.nodes.Get(u.Target); n != nil && n.Amount > 0 {
		u.State = UnitGather
		return
	}
	if b := w.buildings.Get(u.Target); b != nil && b.Progress != 0 && b.HP > 0 {
		u.State = UnitBuild
	}
}

// updateUnits runs one tick of the unit state machines.
func (w *World) updateUnits() {
	w.units.Each(func(id entity.ID, u *Unit) {
		w.updateCond(u) // fear follows sight, terror counts down
		w.hungerTick(id, u, w.udefs.Get(u.DefID))
		if u.State == UnitIdle && u.Owner == OwnerNeutral {
			u.State = UnitWander
		}
		if u.Owner == OwnerNeutral && u.Kind == KindAnimal {
			if def := w.udefs.Get(u.DefID); def != nil && def.Infect {
				w.zombieStep(id, u)
				return
			}
			w.wildlifeStep(id, u)
			return
		}
		if u.State == UnitFlee || u.Cond&(CondFear|CondTerror) != 0 {
			if w.panicStep(u, w.udefs.Get(u.DefID)) {
				return // a frightened civilian runs before it works
			}
			if u.State == UnitFlee {
				w.resumeWork(u)
			}
		}
		switch u.State {
		case UnitMove:
			w.moveUnit(u)
		case UnitPatrol:
			w.patrolStep(u)
		case UnitGather:
			n := w.nodes.Get(u.Target)
			if n == nil || n.Amount <= 0 {
				kind := u.CarriedK
				var tree uint8
				corpse := false
				if n != nil {
					tree = n.Tree
					corpse = n.Corpse
				}
				next := w.chainTarget(u, kind, tree)
				u.Target = entity.Nil
				if u.Carried > 0 {
					u.State = UnitReturn
					if next != entity.Nil {
						u.Target = next // deposit, then walk to the next node
					} else if corpse && kind == resource.Food {
						// pick the next prey while still beside the carcass
						u.AttackTarget = w.nearestPrey(u)
						u.AttackKind = orders.TargetUnit
					}
				} else if next != entity.Nil {
					u.CarriedK = kind
					u.Target = next
					u.State = UnitGather
				} else if corpse && kind == resource.Food && w.chainPrey(u) {
					// hunter continues the hunt on live prey
				} else {
					u.State = UnitIdle
				}
				break
			}
			if w.speciesGated(n) && !w.hasSawmill(u.Owner) {
				u.State = UnitIdle
				u.Target = entity.Nil
				break
			}
			if !w.Reachable(tileOf(u.X), tileOf(u.Y), n.TileX, n.TileY) {
				// ground the walker cannot reach: stop short instead of
				// pressing into the shore, the caller re-assigns
				u.State = UnitIdle
				u.Target = entity.Nil
				break
			}
			if near(u.X, u.Y, n.X, n.Y, WorkRange) {
				w.faceToward(u, n.X, n.Y)
				if u.Carried == 0 {
					u.CarriedK = n.Kind
				}
				carryCap := resource.CarryCapacity + w.carryBonus(u)
				room := carryCap - u.Carried
				if n.Tree != 0 {
					u.GatherAcc += w.treeStep[n.Tree]
				}
				amt := resource.GatherRate
				if n.Tree != 0 {
					amt = u.GatherAcc >> 16
					u.GatherAcc -= amt << 16
				}
				amt = min32(amt, n.Amount, room)
				if amt > 0 {
					n.Amount -= amt
					u.Carried += amt
				}
				if n.Amount <= 0 {
					// node runs dry: remember the next one of this kind
					// while the unit still stands beside it
					if next := w.chainTarget(u, u.CarriedK, n.Tree); next != entity.Nil {
						u.Target = next
					} else if n.Corpse {
						if u.Carried == 0 {
							w.chainPrey(u)
						} else {
							u.AttackTarget = w.nearestPrey(u)
							u.AttackKind = orders.TargetUnit
						}
					}
				}
				if u.Carried >= carryCap {
					u.State = UnitReturn
				}
			} else if !w.walkTo(u, n.X, n.Y, w.speedOf(u)) {
				// no route to the node at all: let it go, the worker
				// gets re-assigned to ground it can actually walk
				u.State = UnitIdle
				u.Target = entity.Nil
			}
		case UnitReturn:
			d := w.nearestDropoff(u)
			if d == entity.Nil {
				u.State = UnitIdle
				break
			}
			b := w.buildings.Get(d)
			bx, by := buildingCenter(b)
			if near(u.X, u.Y, bx, by, WorkRange) {
				pl := &w.players[u.Owner]
				pl.Res[u.CarriedK] += u.Carried
				carryK := u.CarriedK
				u.Carried = 0
				u.CarriedK = resource.None
				n := w.nodes.Get(u.Target)
				if n != nil && n.Amount > 0 {
					u.CarriedK = carryK
					u.State = UnitGather
				} else {
					var tree uint8
					corpse := false
					if n != nil {
						tree = n.Tree
						corpse = n.Corpse
					}
					u.Target = entity.Nil
					next := w.chainTarget(u, carryK, tree)
					if next != entity.Nil {
						u.CarriedK = carryK
						u.Target = next
						u.State = UnitGather
					} else if u.AttackTarget != entity.Nil {
						u.State = UnitAttack // prey picked beside the carcass
					} else if corpse && carryK == resource.Food && w.chainPrey(u) {
						// hunt resumes after the deposit
					} else {
						u.State = UnitIdle
					}
				}
			} else if !w.walkToBuilding(u, b) {
				// dropoff unreachable: stand still with the load
				// rather than press ground, the caller re-assigns
				u.State = UnitIdle
			}
		case UnitBuild:
			b := w.buildings.Get(u.Target)
			if b == nil {
				u.State = UnitIdle
				u.Target = entity.Nil
				break
			}
			if b.Progress == 0 {
				u.State = UnitIdle
				u.Target = entity.Nil
				break
			}
			bx, by := buildingCenter(b)
			if !near(u.X, u.Y, bx, by, WorkRange+fixed.F16(b.Size>>1)<<16) {
				w.walkToBuilding(u, b)
			} else {
				w.faceToward(u, bx, by)
			}
		}
	})
}

// moveUnit advances one moving unit: the flow field (shared by a group,
// built on demand for a lone walker) keeps the route off water and
// obstructions, then the unit finishes with a direct approach to its
// exact (formation) slot.
func (w *World) moveUnit(u *Unit) {
	if near(u.X, u.Y, u.TX, u.TY, 4096) {
		u.X, u.Y = u.TX, u.TY
		u.State = UnitIdle
		return
	}
	gx, gy := tileOf(u.TX), tileOf(u.TY)
	if u.Field != entity.Nil {
		gx, gy = u.FieldX, u.FieldY // a group field aims at the group base
	}
	if !w.Reachable(tileOf(u.X), tileOf(u.Y), gx, gy) {
		// ordered onto the far side of water: hold instead of marching
		// into the shore, the caller re-issues when it wants otherwise
		u.Field = entity.Nil
		u.State = UnitIdle
		return
	}
	if w.followField(u, gx, gy) == routeWalked {
		return
	}
	w.stepToward(u, u.TX, u.TY, w.speedOf(u))
}

// Route outcomes of followField.
const (
	routeWalked  uint8 = iota // the field steered the walker this tick
	routeClose                // the goal tile is reached: walk the last stretch
	routeBlocked              // no path to the goal tile
)

// followField advances u one step along a flow field toward goal tile
// (gx, gy), (re)creating the field when missing or stale.
func (w *World) followField(u *Unit, gx, gy int32) uint8 {
	if u.Field != entity.Nil && (u.FieldX != gx || u.FieldY != gy) {
		u.Field = entity.Nil // stale goal; the pool evicts old fields
	}
	if u.Field == entity.Nil {
		u.Field = w.addFlowField([][2]int32{{tileOf(u.X), tileOf(u.Y)}}, gx, gy)
		u.FieldX, u.FieldY = gx, gy
	}
	f := w.fields.Get(u.Field)
	if f == nil {
		return routeBlocked
	}
	cx, cy := tileOf(u.X), tileOf(u.Y)
	d := f.Distance(cx, cy)
	if d == path.Unreachable {
		w.fields.Free(u.Field)
		u.Field = entity.Nil
		return routeBlocked
	}
	if d == 0 || f.AtGoal(cx, cy) {
		w.fields.Free(u.Field)
		u.Field = entity.Nil
		return routeClose
	}
	nx, ny, ok := path.NextStepTo(f, w.tileWalkable, w.tileBlocked, cx, cy, gx, gy)
	if !ok {
		u.Field = entity.Nil
		return routeBlocked
	}
	w.stepToward(u, tileCenter(nx), tileCenter(ny), w.speedOf(u))
	return routeWalked
}

func buildingCenter(b *Building) (fixed.F16, fixed.F16) {
	c := b.TileX + (b.Size-1)/2
	d := b.TileY + (b.Size-1)/2
	x, y := tileCenter(c), tileCenter(d)
	if b.Size > 1 {
		x += 1 << 15
		y += 1 << 15
	}
	return x, y
}

// rechain continues a gatherer whose node ran out: it retargets the
// nearest still-harvestable node of the same resource kind inside the
// unit's vision, so villagers chain nearby trees or berries instead of
// standing idle. Sawmill-gated species are skipped without a sawmill.
// chainTarget picks the next node for a gatherer whose node ran dry:
// first the nearest node of the same resource kind AND the same tree
// species (pine chains pine, palm chains palm), else the nearest node
// of the same kind of any species. Sawmill-gated species are skipped
// without a sawmill.
func (w *World) chainTarget(u *Unit, kind resource.Kind, tree uint8) entity.ID {
	if kind == resource.None || !kind.Harvestable() || u.Owner >= MaxPlayers {
		return entity.Nil
	}
	ux, uy := tileOf(u.X), tileOf(u.Y)
	r := int32(w.visionOf(u))
	var best entity.ID
	var bestD int32 = 1 << 30
	for pass := 0; pass < 2; pass++ {
		sameSpecies := pass == 0
		w.nodes.Each(func(id entity.ID, n *Node) {
			if n.Kind != kind || n.Amount <= 0 {
				return
			}
			if sameSpecies && n.Tree != tree {
				return
			}
			if w.speciesGated(n) && !w.hasSawmill(u.Owner) {
				return
			}
			if !w.Reachable(ux, uy, n.TileX, n.TileY) {
				return
			}
			dx, dy := n.TileX-ux, n.TileY-uy
			d := dx*dx + dy*dy
			if d <= r*r && d < bestD {
				bestD, best = d, id
			}
		})
		if best != entity.Nil {
			return best
		}
	}
	return entity.Nil
}

// nearestPrey returns the live huntable animal closest to the unit's
// feet inside its vision, or entity.Nil. Dangerous game (anything that
// fights back, like boars) is never auto-chained: villagers would walk
// to their deaths.
func (w *World) nearestPrey(u *Unit) entity.ID {
	ux, uy := tileOf(u.X), tileOf(u.Y)
	r := int32(w.visionOf(u))
	var best entity.ID
	var bestD int32 = 1 << 30
	w.units.Each(func(id entity.ID, a *Unit) {
		if a.Owner != OwnerNeutral || a.HP <= 0 {
			return
		}
		if !w.Reachable(ux, uy, tileOf(a.X), tileOf(a.Y)) {
			return
		}
		d := w.udefs.Get(a.DefID)
		if d == nil || d.CorpseFood <= 0 || d.Attacks() {
			return
		}
		dx, dy := tileOf(a.X)-ux, tileOf(a.Y)-uy
		dd := dx*dx + dy*dy
		if dd <= r*r && dd < bestD {
			bestD, best = dd, id
		}
	})
	return best
}

// chainPrey continues a hunt: a gatherer that finished a corpse goes
// back to attacking the nearest live huntable animal in vision.
func (w *World) chainPrey(u *Unit) bool {
	prey := w.nearestPrey(u)
	if prey == entity.Nil {
		return false
	}
	u.AttackTarget = prey
	u.AttackKind = orders.TargetUnit
	u.State = UnitAttack
	return true
}

func (w *World) nearestDropoff(u *Unit) entity.ID {
	best := entity.Nil
	var bestD2 int64 = 1 << 62
	w.buildings.Each(func(id entity.ID, b *Building) {
		if b.Owner != u.Owner || b.Progress != 0 {
			return
		}
		def := w.bdefs.Get(b.DefID)
		if def == nil || !def.Dropoff {
			return
		}
		bx, by := buildingCenter(b)
		d2 := dist2(u.X, u.Y, bx, by)
		if d2 < bestD2 {
			bestD2, best = d2, id
		}
	})
	return best
}

// updateBuildings advances construction and training queues.
func (w *World) updateBuildings() {
	w.buildings.Each(func(id entity.ID, b *Building) {
		if b.Progress > 0 {
			bx, by := buildingCenter(b)
			reach := WorkRange + fixed.F16(b.Size>>1)<<16
			w.units.Each(func(_ entity.ID, u *Unit) {
				if u.State == UnitBuild && u.Target == id && u.Owner == b.Owner &&
					near(u.X, u.Y, bx, by, reach) {
					b.Progress--
				}
			})
			if b.Progress < 0 {
				b.Progress = 0
			}
		}
		def := w.bdefs.Get(b.DefID)
		if def == nil {
			return
		}
		if def.FarmFoodMilli > 0 && b.Progress == 0 && b.Owner < MaxPlayers && w.players[b.Owner].Active {
			w.farmTick(b)
		}
		pl := &w.players[b.Owner]
		switch {
		case b.TrainCur != 0:
			b.TrainLeft--
			if b.TrainLeft <= 0 {
				if udef := w.udefs.Get(b.TrainCur); udef != nil {
					w.spawnUnitNear(b.Owner, udef, b)
				}
				b.TrainCur = 0
			}
		case b.TrainReq > 0:
			udef := w.udefs.Get(b.TrainQueue[0])
			if udef == nil {
				shiftTrainQueue(b)
				break
			}
			if pl.Pop+udef.PopCost <= w.popCap(b.Owner) &&
				pl.Res[resource.Food] >= udef.CostFood &&
				pl.Res[resource.Wood] >= udef.CostWood &&
				pl.Res[resource.Gold] >= udef.CostGold {
				pl.Res[resource.Food] -= udef.CostFood
				pl.Res[resource.Wood] -= udef.CostWood
				pl.Res[resource.Gold] -= udef.CostGold
				shiftTrainQueue(b)
				b.TrainCur = udef.ID
				b.TrainLeft = udef.TrainTicks
			}
		}
	})
}

// farmTick produces one farm's food increment for this tick. The base
// yield comes from the building def and is multiplied by the farm
// multiplier of the biome under the farm's tile (H1/H2).
func (w *World) farmTick(b *Building) {
	def := w.bdefs.Get(b.DefID)
	yield16 := def.FarmYield16()
	bi := w.biomeDefs.Get(w.gameMap.BiomeAt(int(b.TileX), int(b.TileY)))
	mul16 := fixed.F16(1 << 16)
	if bi != nil {
		mul16 = bi.FarmMul16()
	}
	b.FarmAcc += int32((int64(yield16) * int64(mul16)) >> 16)
	food := b.FarmAcc >> 16
	if food > 0 {
		b.FarmAcc -= food << 16
		w.players[b.Owner].Res[resource.Food] += food
	}
}

// shiftTrainQueue pops the front queue entry.
func shiftTrainQueue(b *Building) {
	for i := int32(0); i+1 < MaxTrainQueue; i++ {
		b.TrainQueue[i] = b.TrainQueue[i+1]
	}
	b.TrainQueue[MaxTrainQueue-1] = 0
	if b.TrainReq > 0 {
		b.TrainReq--
	}
}

// turnOf returns the def turn rate (degrees per tick) for a unit.
func (w *World) turnOf(u *Unit) int32 {
	if d := w.udefs.Get(u.DefID); d != nil && d.TurnRate > 0 {
		return d.TurnRate
	}
	return units.DefaultTurnRate
}

// stepToward walks u towards (tx, ty): the goal only picks the heading
// to steer along, the unit turns by at most its turn rate and then
// advances at `speed` along that heading, so paths curve instead of
// snapping to tile centers. Within one step of the goal it lands exactly
// on it. Unwalkable ground makes the step slide along the free axis, or
// keeps the unit where it is.
func (w *World) stepToward(u *Unit, tx, ty, speed fixed.F16) bool {
	dx, dy := tx.Sub(u.X), ty.Sub(u.Y)
	if near(u.X, u.Y, tx, ty, speed) {
		if dx != 0 || dy != 0 {
			u.Heading = headingTo(dx, dy)
		}
		u.X, u.Y = tx, ty
		return true
	}
	u.Heading = turnToward(u.Heading, headingTo(dx, dy), w.turnOf(u))
	nx := u.X.Add(speed.Mul(sinTable[u.Heading]))
	ny := u.Y.Add(speed.Mul(cosTable[u.Heading]))
	switch {
	case w.tileWalkable(tileOf(nx), tileOf(ny)):
		u.X, u.Y = nx, ny
	case w.tileWalkable(tileOf(nx), tileOf(u.Y)):
		u.X = nx
	case w.tileWalkable(tileOf(u.X), tileOf(ny)):
		u.Y = ny
	default:
		// Wedged in a corner: pull back to the middle of the tile the
		// unit stands on, so the next tick approaches from ground that
		// has room to turn instead of leaning into the wall.
		mx, my := tileCenter(tileOf(u.X)), tileCenter(tileOf(u.Y))
		if !w.tileWalkable(tileOf(u.X), tileOf(u.Y)) {
			break
		}
		if d := abs16(mx.Sub(u.X)); d <= speed {
			u.X = mx
		} else {
			u.X = u.X.Add(mx.Sub(u.X).Mul(speed.Div(d)))
		}
		if d := abs16(my.Sub(u.Y)); d <= speed {
			u.Y = my
		} else {
			u.Y = u.Y.Add(my.Sub(u.Y).Mul(speed.Div(d)))
		}
	}
	return false
}

// pathProbeMax caps how far ahead pathClear looks: the ground within
// 64 tiles decides whether a walker goes straight or takes the field,
// and beyond that a flow field is the cheaper answer anyway.
const pathProbeMax int32 = 128

// pathClear reports whether the straight run from (x0, y0) to (x1, y1)
// crosses only walkable ground, sampled every half tile. It gates the
// cheap direct walk: anything in the way sends the walker onto a flow
// field so it goes around water and rocks instead of leaning on them.
// Sampling is dropped past pathProbeMax steps, so ground under or right
// in front of the unit always decides first.
func (w *World) pathClear(x0, y0, x1, y1 fixed.F16) bool {
	dx, dy := x1.Sub(x0), y1.Sub(y0)
	distance := abs16(dx)
	if abs16(dy) > distance {
		distance = abs16(dy)
	}
	steps := int32(distance) / (1 << 15) // one probe per half tile
	if steps < 1 {
		steps = 1
	}
	if steps > pathProbeMax {
		steps = pathProbeMax
	}
	for i := int32(1); i <= steps; i++ {
		t := fixed.F16(int64(i) * (1 << 16) / int64(steps))
		if !w.tileWalkable(tileOf(x0.Add(dx.Mul(t))), tileOf(y0.Add(dy.Mul(t)))) {
			return false
		}
	}
	return true
}

// walkTo sends u towards the exact point (tx, ty): straight ahead when
// nothing stands between, otherwise along a flow field around the
// obstruction.
// walkTo returns false when no route to (tx, ty) exists at all, so the
// caller can let go of the goal instead of leaning on the scenery.
func (w *World) walkTo(u *Unit, tx, ty, speed fixed.F16) bool {
	ux, uy := tileOf(u.X), tileOf(u.Y)
	if !w.Reachable(ux, uy, tileOf(tx), tileOf(ty)) {
		return false // different landmass: hold, the caller re-assigns
	}
	if w.pathClear(u.X, u.Y, tx, ty) {
		w.stepToward(u, tx, ty, speed)
		return true
	}
	switch w.followField(u, tileOf(tx), tileOf(ty)) {
	case routeWalked:
		return true
	case routeClose:
		w.stepToward(u, tx, ty, speed)
		return true
	}
	return false
}

// approachTile picks the walkable tile just outside b's footprint that
// sits closest to (x, y); ties go to the earlier scanned tile. Buildings
// own their tiles, so workers route to one of these before closing in.
func (w *World) approachTile(b *Building, x, y fixed.F16) (int32, int32, bool) {
	var bestX, bestY int32
	var bestD2 int64 = -1
	for dy := int32(-1); dy <= b.Size; dy++ {
		for dx := int32(-1); dx <= b.Size; dx++ {
			if dx >= 0 && dx < b.Size && dy >= 0 && dy < b.Size {
				continue
			}
			tx, ty := b.TileX+dx, b.TileY+dy
			if !w.tileWalkable(tx, ty) {
				continue
			}
			if d2 := dist2(x, y, tileCenter(tx), tileCenter(ty)); bestD2 < 0 || d2 < bestD2 {
				bestD2, bestX, bestY = d2, tx, ty
			}
		}
	}
	return bestX, bestY, bestD2 >= 0
}

// walkToBuilding sends u to the building's doorstep tile, straight when
// clear and along a flow field when ground stands in the way.
func (w *World) walkToBuilding(u *Unit, b *Building) bool {
	bx, by := buildingCenter(b)
	ax, ay, ok := w.approachTile(b, u.X, u.Y)
	switch {
	case !ok, w.pathClear(u.X, u.Y, tileCenter(ax), tileCenter(ay)):
		w.stepToward(u, bx, by, w.speedOf(u))
		return true
	case w.followField(u, ax, ay) == routeWalked:
		return true
	}
	return false
}

// faceToward points u at (tx, ty) without moving it: attacking, gathering
// and building units look at what they work on.
func (w *World) faceToward(u *Unit, tx, ty fixed.F16) {
	if dx, dy := tx.Sub(u.X), ty.Sub(u.Y); dx != 0 || dy != 0 {
		u.Heading = headingTo(dx, dy)
	}
}

func abs16(v fixed.F16) fixed.F16 {
	if v < 0 {
		return -v
	}
	return v
}

func dist2(ax, ay, bx, by fixed.F16) int64 {
	dx := int64(bx - ax)
	dy := int64(by - ay)
	return dx*dx + dy*dy
}

func near(ax, ay, bx, by, r fixed.F16) bool {
	return dist2(ax, ay, bx, by) <= int64(r)*int64(r)
}

func min32(a, b, c int32) int32 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
