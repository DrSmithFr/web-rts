// Package bots hosts deterministic AI players. A Brain reads only what
// its player can see through its fog (plus public relic positions) and
// returns plain orders, so bots are replayable, hash-stable and run
// identically on the server, in replays or in tests.
package bots

import (
	"rts/sim/core"
	"rts/sim/entity"
	"rts/sim/fog"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
)

// Difficulty changes reaction speed and aggressiveness only, never
// costs or yields.
type Difficulty uint8

const (
	Easy     Difficulty = 0
	Standard Difficulty = 1
	Hard     Difficulty = 2
)

const (
	fogExplored uint8 = 1
	fogVisible  uint8 = 2
)

func interval(d Difficulty) uint32 {
	switch d {
	case Easy:
		return 40
	case Hard:
		return 12
	default:
		return 20
	}
}

// Brain is one bot player's state machine: economy balancing, an era
// build order, tech research, military production and a probe / wave /
// retreat cycle.
type Brain struct {
	Player uint8
	Diff   Difficulty

	since      uint32
	builderOff int
	scoutOff   int
	explorers  [3]entity.ID
	explFrom   [3][2]int32
	explStuck  [3]int
	woodSeek   [3]entity.ID
	preyX      int32
	preyY      int32
	preySeen   uint32
	wave       bool
	lastArmy   int32
	fleeing    uint32
}

// New creates a bot brain for a player slot.
func New(player uint8, d Difficulty) *Brain {
	return &Brain{Player: player, Diff: d, scoutOff: 1}
}

type viewUnit struct {
	id        entity.ID
	x, y      int32
	military  bool
	enemy     bool
	gatherK   resource.Kind
	gathering bool
}

type view struct {
	w *core.World
	p uint8

	villagers []viewUnit
	army      []viewUnit
	idleVill  []entity.ID
	myBuild   [11]int
	buildTile [9][2]int32
	pop, cap  int32
	res       [resource.Count]int32
	era       int32
	seenEnemy []viewUnit
	prey      []viewUnit
	seenK     [4]bool
	size      int32
	fog       *fog.Grid
}

func (v *view) visible(tx, ty int32) bool {
	return v.fog.State(int(tx), int(ty)) == fogVisible
}

func (v *view) seen(tx, ty int32) bool {
	return v.fog.State(int(tx), int(ty)) >= fogExplored
}

func scan(w *core.World, p uint8) *view {
	v := &view{w: w, p: p, fog: w.Fog(p), size: int32(w.Map().Grid().Size())}
	pl := &w.Players()[p]
	v.pop, v.cap = pl.Pop, w.PopCap(p)
	v.res = pl.Res
	v.era = int32(pl.Era)
	w.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.HP <= 0 {
			return
		}
		tx, ty := int32(u.X)>>16, int32(u.Y)>>16
		def := w.UnitDefs().Get(u.DefID)
		mil := def != nil && def.Military()
		if u.Owner == p {
			vu := viewUnit{id: id, x: tx, y: ty, military: mil}
			if u.State == core.UnitGather && u.Target != entity.Nil {
				if n := w.Nodes().Get(u.Target); n != nil {
					vu.gatherK = n.Kind
					vu.gathering = true
				}
			}
			if mil {
				v.army = append(v.army, vu)
			} else {
				v.villagers = append(v.villagers, vu)
				if u.State == core.UnitIdle {
					v.idleVill = append(v.idleVill, id)
				}
			}
			return
		}
		if !v.visible(tx, ty) {
			return
		}
		if u.Owner == core.OwnerNeutral {
			// bears and wolves are hunters, not game; boars charge and
			// gore hunting parties — only harmless game is targeted
			switch u.DefID {
			case core.DefDeer, core.DefRabbit, core.DefSheep:
				v.prey = append(v.prey, viewUnit{id: id, x: tx, y: ty})
			}
			return
		}
		v.seenEnemy = append(v.seenEnemy, viewUnit{id: id, x: tx, y: ty, military: mil, enemy: true})
	})
	v.w.Nodes().Each(func(_ entity.ID, n *core.Node) {
		if n.Amount > 0 && int(n.Kind) < len(v.seenK) && v.seen(n.TileX, n.TileY) {
			v.seenK[n.Kind] = true
		}
	})
	w.Buildings().Each(func(_ entity.ID, b *core.Building) {
		if b.Owner == p && b.DefID < uint16(len(v.myBuild)) {
			if v.myBuild[b.DefID] == 0 {
				v.buildTile[b.DefID] = [2]int32{b.TileX, b.TileY}
			}
			v.myBuild[b.DefID]++
		}
	})
	return v
}

func dist2(ax, ay, bx, by int32) int64 {
	dx, dy := int64(ax-bx), int64(ay-by)
	return dx*dx + dy*dy
}

// nearestNode finds the closest node of kind on explored ground. Wood
// nodes of sawmill-gated species (baobab) are only eligible once the bot
// owns a completed sawmill.
func (v *view) nearestNode(x, y int32, kind resource.Kind) entity.ID {
	var best entity.ID
	bd := int64(1) << 62
	v.w.Nodes().Each(func(id entity.ID, n *core.Node) {
		if n.Kind != kind || n.Amount <= 0 || !v.seen(n.TileX, n.TileY) {
			return
		}
		if d := v.w.TreeDefs().Get(n.Tree); d != nil && d.Sawmill && v.myBuild[core.BldSawmill] == 0 {
			return
		}
		if d := dist2(x, y, n.TileX, n.TileY); d < bd {
			bd, best = d, id
		}
	})
	return best
}

// Step advances the brain and returns orders generated at this tick;
// the caller stamps them like any other order.
func (b *Brain) Step(w *core.World, tick uint32) []proto.Order {
	if b.since > 0 && tick%interval(b.Diff) != 0 {
		return nil
	}
	b.since++
	v := scan(w, b.Player)
	if len(v.villagers) == 0 && len(v.army) == 0 {
		return nil
	}
	if len(v.prey) > 0 {
		b.preyX, b.preyY = v.prey[0].x, v.prey[0].y
		b.preySeen = w.Tick()
	}
	var out []proto.Order
	out = append(out, b.ageUp(v)...)
	out = append(out, b.economy(v)...)
	out = append(out, b.explore(v)...)
	out = append(out, b.buildOrder(v)...)
	out = append(out, b.military(v)...)
	return out
}

// demand ranks resource lanes by remaining need; scarcest first.
func (v *view) demand() []resource.Kind {
	order := []resource.Kind{resource.Food, resource.Wood, resource.Stone, resource.Gold}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && needOf(v, order[j]) > needOf(v, order[j-1]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	return order
}

func needOf(v *view, k resource.Kind) int32 {
	var target int32
	switch k {
	case resource.Food:
		target = 550
	case resource.Wood:
		target = 250
	case resource.Stone:
		target = 150
	case resource.Gold:
		target = 150
	}
	n := v.res[k]
	if n > target {
		n = target
	}
	return target - n
}

// nearestPrey picks visible huntable wildlife; deer are harmless while
// boars fight back, so deer win ties and are strictly preferred.
func (b *Brain) nearestPrey(v *view, x, y int32) entity.ID {
	var best entity.ID
	bd := int64(1) << 62
	for _, pr := range v.prey {
		w := v.w.Units().Get(pr.id)
		if w == nil || w.HP <= 0 {
			continue
		}
		d := dist2(x, y, pr.x, pr.y)
		if d < bd {
			bd, best = d, pr.id
		}
	}
	return best
}

// foodOrHunt returns a gatherable food node id, or a visible prey id
// when berries are exhausted and food is low. Exactly one of the two
// may be non-zero.
func foodOrHunt(v *view, b *Brain, x, y int32) (node, prey entity.ID) {
	if n := v.nearestNode(x, y, resource.Food); n != 0 {
		return n, 0
	}
	if v.res[resource.Food] < 250 && len(v.prey) > 0 {
		return 0, b.nearestPrey(v, x, y)
	}
	return 0, 0
}

// phaseSeek derives a stable phase from an entity id so wandering
// workers do not all walk the same corridor.
func phaseSeek(id entity.ID) uint32 {
	return uint32(uint32(id) * 2654435761 >> 24)
}

// huntTarget picks prey for the idle squad: deer, sheep and rabbits are
// free kills (nearest first); boars (40 HP, armor, bite back at 6/crush)
// are left alone. Returns ok=false when no harmless game is visible.
func (b *Brain) huntTarget(v *view, squad int) (entity.ID, bool) {
	var game entity.ID
	dd := int64(1) << 62
	for _, pr := range v.prey {
		u := v.w.Units().Get(pr.id)
		if u == nil || u.HP <= 0 {
			continue
		}
		if u.DefID != core.DefDeer && u.DefID != core.DefSheep && u.DefID != core.DefRabbit {
			continue
		}
		d := dist2(int32(u.X)>>16, int32(u.Y)>>16, pr.x, pr.y)
		if d < dd {
			dd, game = d, pr.id
		}
	}
	_ = squad
	if game != 0 {
		return game, true
	}
	return 0, false
}

func (b *Brain) economy(v *view) []proto.Order {
	var out []proto.Order
	need := v.demand()
	home := b.homeTile(v)
	cx, cy := b.mapBias(v, home)
	// Wood seekers: if no wood is in sight and stores are low, two
	// villagers leave everything and walk out until trees appear —
	// otherwise the economy tunnels on whatever single node is visible.
	woodSeen := v.seenK[resource.Wood]
	for i := range b.woodSeek {
		if b.woodSeek[i] != 0 && !v.owns(b.woodSeek[i]) {
			b.woodSeek[i] = 0
		}
	}
	if !woodSeen && v.res[resource.Wood] < 350 {
		for i := range b.woodSeek {
			if b.woodSeek[i] != 0 {
				continue
			}
			if int32(len(v.villagers)) < 4+int32(i) {
				continue
			}
			for k := int32(len(v.villagers)) - 1; k >= 0; k-- {
				cand := v.villagers[k].id
				if !b.isExplorer(cand) && !b.isWoodSeeker(cand) {
					b.woodSeek[i] = cand
					break
				}
			}
		}
	} else if woodSeen || v.res[resource.Wood] >= 350 {
		for i := range b.woodSeek {
			b.woodSeek[i] = 0
		}
	}
	for i := range b.woodSeek {
		e := b.woodSeek[i]
		if e == 0 {
			continue
		}
		u := v.w.Units().Get(e)
		if u == nil || u.State != core.UnitIdle {
			continue
		}
		ux, uy := int32(u.X)>>16, int32(u.Y)>>16
		b.scoutOff++
		bx, by := bearingTo(cx-ux, cy-uy)
		wob := scoutDirs[(uint32(b.scoutOff)+uint32(i)*3)%8]
		tx := clamp32(ux+bx*12+wob[1]*6, 1, v.size-2)
		ty := clamp32(uy+by*12+wob[0]*6, 1, v.size-2)
		out = append(out, mk(v.p, orders.MoveGroup,
			orders.MoveGroupPayload([]uint32{uint32(e)},
				uint16(tx*16+8), uint16(ty*16+8))))
	}

	// Rebalance: workers stuck on a resource we already have plenty of
	// move to the scarcest visible lane. Without this, one visible
	// gold seam absorbs the whole village forever.
	var busy []uint32
	for _, vu := range v.villagers {
		if !vu.gathering || b.isExplorer(vu.id) || b.isWoodSeeker(vu.id) {
			continue
		}
		if needOf(v, vu.gatherK) > 40 {
			continue
		}
		best, bestNeed := resource.Food, needOf(v, resource.Food)
		for _, k := range []resource.Kind{resource.Wood, resource.Stone, resource.Gold} {
			if nd := needOf(v, k); nd > bestNeed {
				best, bestNeed = k, nd
			}
		}
		if best == vu.gatherK || bestNeed <= 40 {
			continue
		}
		if n := v.nearestNode(vu.x, vu.y, best); n != 0 {
			busy = append(busy, uint32(vu.id))
			out = append(out, mk(v.p, orders.Gather, orders.GatherPayload(uint32(vu.id), uint32(n))))
		}
	}

	// Group hunt first: food on this map is finite (berries + wildlife),
	// so killing an animal with a squad of villagers — never a solo
	// one against a boar — is the main food engine once berries end.
	// Attackers auto-resume harvesting the corpse on kill.
	hunting := map[entity.ID]bool{}
	if v.res[resource.Food] < 290 {
		// hunt visible prey with a squad
		var hunters []uint32
		for _, vid := range v.idleVill {
			if !b.isExplorer(vid) && !b.isWoodSeeker(vid) && len(hunters) < 3 {
				hunters = append(hunters, uint32(vid))
			}
		}
		if tgt, ok := b.huntTarget(v, len(hunters)); ok && len(hunters) > 0 {
			for _, h := range hunters {
				hunting[entity.ID(h)] = true
			}
			out = append(out, mk(v.p, orders.Attack, orders.AttackPayload(hunters, uint32(tgt), orders.TargetUnit)))
		} else {
			// no prey visible: up to two idle villagers drift toward
			// the last kill site (if fresh) looking for wildlife
			drifters := 0
			for _, vid := range v.idleVill {
				if drifters >= 2 {
					break
				}
				if b.isExplorer(vid) || b.isWoodSeeker(vid) {
					continue
				}
				px, py := b.preyX, b.preyY
				if b.preySeen == 0 || w2tick(v.w)-b.preySeen > 3000 {
					px, py = v.size/2, v.size/2
				}
				tx := clamp32(px+int32(drifters%3)*2-2, 1, v.size-2)
				ty := clamp32(py+int32((drifters/3)%3)*2-2, 1, v.size-2)
				hunting[vid] = true
				out = append(out, mk(v.p, orders.MoveGroup,
					orders.MoveGroupPayload([]uint32{uint32(vid)},
						uint16(tx*16+8), uint16(ty*16+8))))
				drifters++
			}
		}
	}
	for _, vid := range v.idleVill {
		if b.isExplorer(vid) || b.isWoodSeeker(vid) || hunting[vid] {
			continue
		}
		u := v.w.Units().Get(vid)
		if u == nil {
			continue
		}
		ux, uy := int32(u.X)>>16, int32(u.Y)>>16
		if node := v.nearestNode(ux, uy, resource.Food); node != 0 {
			out = append(out, mk(v.p, orders.Gather,
				orders.GatherPayload(uint32(vid), uint32(node))))
			continue
		}
		seeks := false
		for _, r := range need[1:] {
			if n := v.nearestNode(ux, uy, r); n != 0 {
				out = append(out, mk(v.p, orders.Gather,
					orders.GatherPayload(uint32(vid), uint32(n))))
				seeks = true
				break
			}
		}
		if !seeks {
			// home region exhausted: walk the worker toward mid until
			// fresh nodes come into sight
			b.scoutOff++
			bx, by := bearingTo(cx-ux, cy-uy)
			wob := scoutDirs[(phaseSeek(vid)+uint32(b.scoutOff))%8]
			hop := int32(14)
			tx := clamp32(ux+bx*hop-wob[1]*5, 1, v.size-2)
			ty := clamp32(uy+by*hop+wob[0]*5, 1, v.size-2)
			out = append(out, mk(v.p, orders.MoveGroup,
				orders.MoveGroupPayload([]uint32{uint32(vid)},
					uint16(tx*16+8), uint16(ty*16+8))))
		}
	}
	// Food on this map is finite (berries + deer), so villagers are
	// only trained from genuine surpluses; a lean village is what
	// makes the Antiquity age-up and the relic rush affordable.
	store := v.firstBuilding(core.BldStorehouse)
	want := 6 + int32(b.Diff)*2
	if store != 0 && int32(len(v.villagers)) < want && v.pop+1 <= v.cap &&
		v.res[resource.Food] >= 350 {
		out = append(out, mk(v.p, orders.Train, orders.TrainPayload(uint32(store))))
	}
	return out
}

func (v *view) firstBuilding(def uint16) entity.ID {
	var id entity.ID
	v.w.Buildings().Each(func(eid entity.ID, bl *core.Building) {
		if id == 0 && bl.Owner == v.p && bl.DefID == def && bl.Progress == 0 {
			id = eid
		}
	})
	return id
}

const (
	uintSCOUT  = 16
	uintRADIUS = 4
)

var scoutDirs = [16][2]int32{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1},
	{-1, 0}, {-1, -1}, {0, -1}, {1, -1},
	{2, 1}, {1, 2}, {-1, 2}, {-2, 1},
	{-2, -1}, {-1, -2}, {1, -2}, {2, -1},
}

var scoutRadii = [4]int32{10, 16, 22, 28}

var buildSpots = [][2]int32{{-5, 4}, {4, -5}, {5, 4}, {-4, -5}, {6, 0}, {-6, 0}, {0, 6}, {0, -6}}

type plan struct {
	def    uint16
	minEra int32
	need   func(v *view) bool
}

var plans = []plan{
	{core.BldCampfire, 0, func(v *view) bool { return v.myBuild[core.BldCampfire] < 2 && v.res[resource.Food] >= 60 }},
	{core.BldHut, 0, func(v *view) bool {
		return v.myBuild[core.BldHut] < 8 && v.res[resource.Wood] >= 60 &&
			v.pop+1 >= v.cap && v.res[resource.Food] >= 120
	}},
	{core.BldBarracks, 1, func(v *view) bool { return v.myBuild[core.BldBarracks] < 1 }},
	{core.BldMarket, 1, func(v *view) bool { return v.myBuild[core.BldMarket] < 1 && v.res[resource.Wood] >= 150 }},
	{core.BldForge, 2, func(v *view) bool { return v.myBuild[core.BldForge] < 1 }},
	{core.BldBank, 3, func(v *view) bool { return v.myBuild[core.BldBank] < 1 }},
	{core.BldWonder, 4, func(v *view) bool {
		return v.myBuild[core.BldWonder] < 1 && v.res[resource.Wood] >= 900 && v.res[resource.Stone] >= 600
	}},
}

func (b *Brain) buildOrder(v *view) []proto.Order {
	for _, pl := range plans {
		if v.era < pl.minEra || !pl.need(v) || !affordable(v, pl.def) || b.hasBuilding(v, pl.def) {
			continue
		}
		builder, ok := b.pickBuilder(v)
		if !ok {
			continue
		}
		bu := v.w.Units().Get(builder)
		if bu == nil {
			continue
		}
		bx, by := int32(bu.X)>>16, int32(bu.Y)>>16
		spot := buildSpots[b.builderOff%len(buildSpots)]
		b.builderOff++
		tx, ty := bx+spot[0], by+spot[1]
		if tx < 1 || ty < 1 || tx >= v.size-1 || ty >= v.size-1 {
			continue
		}
		return []proto.Order{mk(v.p, orders.Build, orders.BuildPayload(uint32(builder), pl.def,
			uint16(tx), uint16(ty)))}
	}
	return nil
}

func (b *Brain) hasBuilding(v *view, def uint16) bool {
	found := false
	v.w.Buildings().Each(func(_ entity.ID, bl *core.Building) {
		if bl.Owner == v.p && bl.DefID == def {
			found = true
		}
	})
	return found
}

func (b *Brain) pickBuilder(v *view) (entity.ID, bool) {
	for _, id := range v.idleVill {
		return id, true
	}
	if len(v.villagers) > 4 {
		return v.villagers[len(v.villagers)-1].id, true
	}
	return 0, false
}

func affordable(v *view, def uint16) bool {
	d := v.w.BuildingDefs().Get(def)
	if d == nil {
		return false
	}
	return v.res[resource.Wood] >= d.CostWood && v.res[resource.Stone] >= d.CostStone
}

func (b *Brain) ageUp(v *view) []proto.Order {
	target := uint8(v.era + 1)
	def := v.w.EraDefs().Get(target)
	if def == nil {
		return nil
	}
	if v.res[resource.Food] < def.CostFood || v.res[resource.Gold] < def.CostGold {
		return nil
	}
	if def.ReqBuilding != 0 && v.myBuild[def.ReqBuilding] == 0 {
		return nil
	}
	if v.w.Players()[v.p].AgeTarget != 0 {
		return nil
	}
	return []proto.Order{mk(v.p, orders.AgeUp, orders.AgeUpPayload(target))}
}

func (b *Brain) military(v *view) []proto.Order {
	var out []proto.Order
	bar := v.firstBuilding(core.BldBarracks)
	if bar == 0 {
		return out
	}
	armyCap := int32(2) + int32(b.Diff)
	if int32(len(v.army)) < armyCap && v.pop+1 <= v.cap &&
		v.res[resource.Food] >= 130 && v.res[resource.Gold] >= 30 {
		out = append(out, mk(v.p, orders.Train, orders.TrainPayload(uint32(bar), core.DefLancer)))
	}
	b.research(v, &out)
	out = append(out, b.relicRush(v)...)
	out = append(out, b.waves(v)...)
	return out
}

func (b *Brain) research(v *view, out *[]proto.Order) {
	pl := &v.w.Players()[v.p]
	if pl.TechCur != 0 || v.res[resource.Gold] < 60 {
		return
	}
	for _, t := range v.w.TechDefs().Defs() {
		if t.Building != core.BldBarracks || int32(t.Era) > v.era {
			continue
		}
		if pl.Techs&(uint64(1)<<t.ID) != 0 || !techReady(pl, t.ID, v.w) {
			continue
		}
		if v.res[resource.Food] >= t.CostFood && v.res[resource.Wood] >= t.CostWood &&
			v.res[resource.Gold] >= t.CostGold {
			*out = append(*out, mk(v.p, orders.Research, orders.ResearchPayload(t.ID)))
			return
		}
	}
}

func techReady(pl *core.Player, id uint16, w *core.World) bool {
	for _, t := range w.TechDefs().Defs() {
		if t.ID == id {
			for _, req := range t.Requires {
				if pl.Techs&(uint64(1)<<req) == 0 {
					return false
				}
			}
			return true
		}
	}
	return false
}

// waves runs the probe / attack / retreat cycle from relative strength.
func (b *Brain) waves(v *view) []proto.Order {
	army := int32(len(v.army))
	if b.fleeing > 0 {
		b.fleeing--
		if b.fleeing == 0 {
			b.wave = false
		}
		return nil
	}
	if b.wave && b.lastArmy > 3 && army*2 < b.lastArmy {
		b.wave = false
		b.fleeing = 60
		home := b.homeTile(v)
		return b.moveArmy(v, home[0], home[1])
	}
	b.lastArmy = army
	th := int32(4 + int32(b.Diff)*2)
	if !b.wave && army >= th {
		b.wave = true
	}
	if !b.wave {
		if army >= 2 {
			cx, cy := b.probeTarget(v)
			return b.moveArmy(v, cx, cy)
		}
		return nil
	}
	var target *viewUnit
	td := int64(1) << 62
	for i := range v.seenEnemy {
		e := &v.seenEnemy[i]
		score := dist2(0, 0, e.x, e.y)
		if !e.military {
			score /= 4
		}
		if score < td {
			td, target = score, e
		}
	}
	if target == nil {
		cx, cy := b.probeTarget(v)
		return b.moveArmy(v, cx, cy)
	}
	ids := make([]uint32, 0, len(v.army))
	for _, a := range v.army {
		ids = append(ids, uint32(a.id))
	}
	return []proto.Order{mk(v.p, orders.Attack, orders.AttackPayload(ids, uint32(target.id), orders.TargetUnit))}
}

func (b *Brain) relicRush(v *view) []proto.Order {
	if v.era < 1 || len(v.army) < 2 {
		return nil
	}
	rels := v.w.Relics()
	for ri := 0; ri < core.RelicCount; ri++ {
		// relic positions are public knowledge (drawn on every minimap),
		// so contesting them needs no fog sight
		if rels[ri].Owner == v.p {
			continue
		}
		solo := v.army[len(v.army)-1]
		return []proto.Order{mk(v.p, orders.MoveGroup,
			orders.MoveGroupPayload([]uint32{uint32(solo.id)},
				uint16(rels[ri].X*16+8), uint16(rels[ri].Y*16+8)))}
	}
	return nil
}

func (b *Brain) homeTile(v *view) [2]int32 {
	if v.myBuild[core.BldStorehouse] > 0 {
		return v.buildTile[core.BldStorehouse]
	}
	return [2]int32{int32(v.size / 2), int32(v.size / 2)}
}

func (b *Brain) probeTarget(v *view) (int32, int32) {
	home := b.homeTile(v)
	size := v.size
	b.scoutOff++
	dx, dy := size/2-home[0], size/2-home[1]
	step := int32(b.scoutOff)
	cx := clamp32(home[0]+dx/8+step%8, 1, size-2)
	cy := clamp32(home[1]+dy/8+step%7, 1, size-2)
	return cx, cy
}

func clamp32(x, lo, hi int32) int32 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func (b *Brain) moveArmy(v *view, x, y int32) []proto.Order {
	ids := make([]uint32, 0, len(v.army))
	for _, a := range v.army {
		ids = append(ids, uint32(a.id))
	}
	if len(ids) == 0 {
		return nil
	}
	return []proto.Order{mk(v.p, orders.MoveGroup,
		orders.MoveGroupPayload(ids, uint16(x*16+8), uint16(y*16+8)))}
}

// explore walks up to three villagers outward from base on a fan of
// compass spirals so fog, berries, gold and prey come into sight.
func (b *Brain) explore(v *view) []proto.Order {
	var out []proto.Order
	need := 1 + int(b.Diff)
	live := 0
	for i := range b.explorers {
		if b.explorers[i] != 0 && !v.owns(b.explorers[i]) {
			b.explorers[i] = 0
		}
		if b.explorers[i] != 0 {
			live++
		}
	}
	for i := range b.explorers {
		if b.explorers[i] != 0 {
			continue
		}
		if live >= need || int32(len(v.villagers)) < 4+int32(i) {
			continue
		}
		for k := int32(len(v.villagers)) - 1; k >= 0; k-- {
			cand := v.villagers[k].id
			if !b.isExplorer(cand) {
				b.explorers[i] = cand
				live++
				break
			}
		}
	}
	home := b.homeTile(v)
	size := v.size
	cx, cy := b.mapBias(v, home)
	for i := range b.explorers {
		e := b.explorers[i]
		if e == 0 {
			continue
		}
		u := v.w.Units().Get(e)
		if u == nil || u.State != core.UnitIdle {
			continue
		}
		ux, uy := int32(u.X)>>16, int32(u.Y)>>16
		phase := uint32(b.scoutOff) + uint32(i)*5
		b.scoutOff++
		stuck := 0
		cur := [2]int32{ux, uy}
		if b.explFrom[i] != [2]int32{} && b.explFrom[i] == cur {
			b.explStuck[i]++
			stuck = int(b.explStuck[i]%3) + 1
		} else {
			b.explStuck[i] = 0
		}
		b.explFrom[i] = cur
		// bearing from the explorer to mid, fanned by explorer index
		// and wobbling on the compass; when stuck, arc sideways
		bx, by := bearingTo(cx-ux, cy-uy)
		bx, by = rotate(bx, by, int(i))
		wob := scoutDirs[phase%8]
		hop := int32(10) + int32(phase%3)*4
		perp := int32(stuck) * 8
		tx := clamp32(ux+bx*hop-wob[1]*perp, 1, size-2)
		ty := clamp32(uy+by*hop+wob[0]*perp, 1, size-2)
		out = append(out, mk(v.p, orders.MoveGroup,
			orders.MoveGroupPayload([]uint32{uint32(e)},
				uint16(tx*16+8), uint16(ty*16+8))))
	}
	return out
}

// mapBias returns the middle of the map: map size is known at placement
// time, so contesting mid needs no fog sight.
func (b *Brain) mapBias(v *view, home [2]int32) (int32, int32) {
	size := int32(v.w.Map().Grid().Size())
	return size / 2, size / 2
}

func (b *Brain) isWoodSeeker(vid entity.ID) bool {
	for _, e := range b.woodSeek {
		if e == vid {
			return true
		}
	}
	return false
}

func (b *Brain) isExplorer(vid entity.ID) bool {
	for _, e := range b.explorers {
		if e == vid {
			return true
		}
	}
	return false
}

// rotate fans a bearing by 45 degrees times n for explorer spread.
func rotate(dx, dy int32, n int) (int32, int32) {
	for k := 0; k < n; k++ {
		dx, dy = dy, -dx
		switch n {
		case 1:
			// quarter turn is too wide; blend back toward original
			dx, dy = (dx*3)/4, (dy*3)/4
		}
	}
	if dx == 0 && dy == 0 {
		return 1, 0
	}
	return dx, dy
}

// bearingTo quantizes a vector to the best of 8 unit compass bearings.
func bearingTo(vx, vy int32) (int32, int32) {
	dirs := [8][2]int32{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	best, bd := dirs[0], int64(-1)
	for _, d := range dirs {
		p := int64(d[0])*int64(vx) + int64(d[1])*int64(vy)
		if p > bd {
			bd, best = p, d
		}
	}
	return best[0], best[1]
}

func (v *view) owns(id entity.ID) bool {
	u := v.w.Units().Get(id)
	return u != nil && u.Owner == v.p && u.HP > 0
}

func w2tick(w *core.World) uint32 { return w.Tick() }

func mk(p uint8, kind uint16, payload []byte) proto.Order {
	return proto.Order{Player: p, Kind: proto.Kind(kind), Payload: payload}
}
