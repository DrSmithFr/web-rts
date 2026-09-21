package core

import (
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/orders"
	"rts/sim/path"
	"rts/sim/resource"
	"rts/sim/terrain"
)

// startBuildingOffsets are candidate storehouse footprints (top-left tile,
// relative to spawn) tried in order until one is walkable and free.
var startBuildingOffsets = [][2]int32{{-4, -1}, {-4, 0}, {3, -1}, {3, 0}, {-1, -4}, {0, -4}, {-1, 3}, {0, 3}}

// villagerOffsets place the starting villagers inside the stamped grass disc.
var villagerOffsets = [][2]int32{{1, 1}, {-1, 1}, {1, -1}}

// initEconomy seeds nodes, players, a storehouse and three villagers per
// spawn. Must run after the map and stores exist.
func (w *World) initEconomy() {
	for _, n := range w.gameMap.Nodes() {
		_, e := w.nodes.Create()
		e.Kind = n.Kind
		e.TileX, e.TileY = n.X, n.Y
		e.X = tileCenter(n.X)
		e.Y = tileCenter(n.Y)
		e.Amount = n.Amount
	}
	for i, s := range w.gameMap.Spawns() {
		p := &w.players[i]
		p.Active = true
		p.Res[resource.Wood] = startWood
		p.Res[resource.Stone] = startStone
		p.Res[resource.Food] = startFood
		p.Res[resource.Gold] = startGold

		def := w.bdefs.Get(buildingStorehouse)
		for _, o := range startBuildingOffsets {
			tx, ty := int32(s[0])+o[0], int32(s[1])+o[1]
			if w.footprintFree(tx, ty, def.Size) {
				_, b := w.buildings.Create()
				b.DefID = buildingStorehouse
				b.Owner = uint8(i)
				b.TileX, b.TileY = tx, ty
				b.Size = def.Size
				b.HP = BuildingHP
				w.markOccupied(b)
				break
			}
		}
		for _, o := range villagerOffsets {
			w.spawnVillagerAt(uint8(i), int32(s[0])+o[0], int32(s[1])+o[1])
		}
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
	size := int32(w.gameMap.Grid().Size())
	if x < 0 || y < 0 || x >= size || y >= size {
		return false
	}
	if w.occupied[y*size+x] {
		return false
	}
	t := w.gameMap.Grid().Tile(int(x), int(y))
	return t != terrain.Shallow && t != terrain.Deep
}

// tileBlocked reports only building occupancy (units may share ground).
func (w *World) tileBlocked(x, y int32) bool {
	size := int32(w.gameMap.Grid().Size())
	if x < 0 || y < 0 || x >= size || y >= size {
		return true
	}
	return w.occupied[y*size+x]
}

func (w *World) markOccupied(b *Building) {
	size := w.gameMap.Grid().Size()
	for dy := int32(0); dy < b.Size; dy++ {
		for dx := int32(0); dx < b.Size; dx++ {
			x, y := int(b.TileX)+int(dx), int(b.TileY)+int(dy)
			if x >= 0 && y >= 0 && x < size && y < size {
				w.occupied[y*size+x] = true
			}
		}
	}
}

// updateVision advances fog of war: visible decays to explored, then the
// player's own units and buildings reveal fresh ground.
func (w *World) updateVision() {
	for i := 0; i < MaxPlayers; i++ {
		if w.players[i].Active {
			w.fogs[i].Decay()
		}
	}
	w.units.Each(func(_ entity.ID, u *Unit) {
		if w.players[u.Owner].Active {
			w.fogs[u.Owner].ApplyCircle(int(tileOf(u.X)), int(tileOf(u.Y)), VillagerVision)
		}
	})
	w.buildings.Each(func(_ entity.ID, b *Building) {
		if !w.players[b.Owner].Active {
			return
		}
		r := SiteVision
		if b.Progress == 0 {
			r = BuildingVision
		}
		cx := int(b.TileX) + int(b.Size-1)/2
		cy := int(b.TileY) + int(b.Size-1)/2
		w.fogs[b.Owner].ApplyCircle(cx, cy, r)
	})
}

func (w *World) spawnVillagerAt(owner uint8, tx, ty int32) {
	_, u := w.units.Create()
	u.Kind = UnitVillager
	u.Owner = owner
	u.X, u.Y = tileCenter(tx), tileCenter(ty)
	u.HP = VillagerHP
	w.players[owner].Pop++
}

// spawnVillagerNear places a trained villager on the first free tile in a
// widening ring around the building.
func (w *World) spawnVillagerNear(owner uint8, b *Building) {
	cx, cy := b.TileX+(b.Size-1)/2, b.TileY+(b.Size-1)/2
	for r := int32(1); r <= 4; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				tx, ty := cx+dx, cy+dy
				if !w.tileWalkable(tx, ty) {
					continue
				}
				if !w.occupied[int(ty)*w.gameMap.Grid().Size()+int(tx)] {
					w.spawnVillagerAt(owner, tx, ty)
					return
				}
			}
		}
	}
	w.spawnVillagerAt(owner, cx, cy)
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
	size := int32(w.gameMap.Grid().Size())
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
	if ox2 >= size {
		ox2 = size - 1
	}
	if oy2 >= size {
		oy2 = size - 1
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
		if n := w.nodes.Get(node); n != nil && n.Kind.Harvestable() && n.Amount > 0 {
			if u.Carried > 0 && u.CarriedK != n.Kind {
				u.Carried = 0 // dropping a different cargo is acceptable
			}
			u.CarriedK = n.Kind
			u.Target = node
			u.State = UnitGather
		}
	}
	return r.OK()
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
	b.HP = BuildingHP
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
		}
	}
	return r.OK()
}

func (w *World) applyTrain(player uint8, p []byte) bool {
	r := orders.NewReader(p)
	id := entity.ID(r.Uvarint())
	if b := w.buildings.Get(id); b != nil && b.Owner == player {
		if def := w.bdefs.Get(b.DefID); def != nil && def.TrainTicks > 0 && b.TrainReq < MaxTrainQueue {
			b.TrainReq++
		}
	}
	return r.OK()
}

// updateUnits runs one tick of the villager state machine.
func (w *World) updateUnits() {
	w.units.Each(func(_ entity.ID, u *Unit) {
		switch u.State {
		case UnitMove:
			w.moveUnit(u)
		case UnitGather:
			n := w.nodes.Get(u.Target)
			if n == nil || n.Amount <= 0 {
				u.State = UnitIdle
				u.Target = entity.Nil
				break
			}
			if near(u.X, u.Y, n.X, n.Y, WorkRange) {
				if u.Carried == 0 {
					u.CarriedK = n.Kind
				}
				room := resource.CarryCapacity - u.Carried
				amt := min32(resource.GatherRate, n.Amount, room)
				if amt > 0 {
					n.Amount -= amt
					u.Carried += amt
				}
				if u.Carried >= resource.CarryCapacity {
					u.State = UnitReturn
				}
			} else {
				stepToward(&u.X, &u.Y, n.X, n.Y)
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
				u.Carried = 0
				u.CarriedK = resource.None
				n := w.nodes.Get(u.Target)
				if n != nil && n.Amount > 0 {
					u.State = UnitGather
				} else {
					u.State = UnitIdle
					u.Target = entity.Nil
				}
			} else {
				stepToward(&u.X, &u.Y, bx, by)
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
				stepToward(&u.X, &u.Y, bx, by)
			}
		}
	})
}

// moveUnit advances one moving unit: follow the shared flow field while
// it covers the walker, then finish with a direct approach to the exact
// (formation) slot.
func (w *World) moveUnit(u *Unit) {
	if u.Field != entity.Nil {
		f := w.fields.Get(u.Field)
		if f == nil {
			u.Field = entity.Nil
		} else {
			cx, cy := tileOf(u.X), tileOf(u.Y)
			d := f.Distance(cx, cy)
			switch {
			case d == 0 || f.AtGoal(cx, cy):
				w.fields.Free(u.Field)
				u.Field = entity.Nil
			case d == path.Unreachable:
				w.fields.Free(u.Field)
				u.Field = entity.Nil
			default:
				nx, ny, ok := path.NextStep(f, w.tileWalkable, w.tileBlocked, cx, cy)
				if ok {
					stepToward(&u.X, &u.Y, tileCenter(nx), tileCenter(ny))
					return
				}
				w.fields.Free(u.Field)
				u.Field = entity.Nil
			}
		}
	}
	if near(u.X, u.Y, u.TX, u.TY, 4096) {
		u.X, u.Y = u.TX, u.TY
		u.State = UnitIdle
		return
	}
	stepToward(&u.X, &u.Y, u.TX, u.TY)
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
		if def == nil || def.TrainTicks <= 0 {
			return
		}
		pl := &w.players[b.Owner]
		switch {
		case b.TrainLeft > 0:
			b.TrainLeft--
			if b.TrainLeft == 0 {
				w.spawnVillagerNear(b.Owner, b)
			}
		case b.TrainReq > 0:
			if pl.Pop < w.popCap(b.Owner) && pl.Res[resource.Food] >= def.TrainFood {
				pl.Res[resource.Food] -= def.TrainFood
				b.TrainLeft = def.TrainTicks
				b.TrainReq--
			}
		}
	})
}

func stepToward(x, y *fixed.F16, tx, ty fixed.F16) bool {
	step := VillagerSpeed
	dx := tx.Sub(*x)
	dy := ty.Sub(*y)
	ax, ay := abs16(dx), abs16(dy)
	if ax <= step && ay <= step {
		*x, *y = tx, ty
		return true
	}
	m := ax
	if ay > m {
		m = ay
	}
	t := step.Div(m)
	*x = x.Add(dx.Mul(t))
	*y = y.Add(dy.Mul(t))
	return false
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
