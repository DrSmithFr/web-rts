//go:build js && wasm

// Command wasm compiles the deterministic simulation to WebAssembly and
// exposes it to JavaScript through the global object `RTSSim`.
//
// The bridge is deliberately thin: it owns the world instances and the
// lockstep queue only. All gameplay logic lives in sim/, the same packages
// the native server imports.
//
// JavaScript surface (window.RTSSim):
//
//	newWorld(seed)        -> world id (seed may be omitted, defaults to 0x5eed)
//	step(id)              -> execute one 20 Hz tick, applying due orders
//	order(id, player, kind, payloadUint8Array) -> schedule at tick+2
//	tick(id)              -> ticks executed
//	hashHex(id)           -> current state hash, 16 lowercase hex digits
//	seed(id)              -> world seed (safe up to 2^53)
//	mapSize(id)           -> tiles per side
//	mapTiles(id)          -> Uint8Array of tile type bytes, row-major
//	mapHeights(id)        -> Uint8Array of (size+1)^2 u16 LE vertex heights
//	mapHashHex(id)        -> map digest, 16 lowercase hex digits
//	spawnCount(id)        -> number of spawn points
//	spawnAt(id, i)        -> packed spawn i as (x << 16) | y
//	unitsBlob(id)         -> Uint8Array, 24-byte records: id u32, owner u8,
//	                          kind u8, state u8, carried u8, x16 u16, y16 u16,
//	                          hp u16, def u16, target u32, carriedAmt u16,
//	                          pad u16 (LE)
//	nodesBlob(id)         -> Uint8Array, 16-byte records: id u32, kind u8,
//	                          amount u16, x16 u16, y16 u16, tileX u16,
//	                          tileY u16, pad u8
//	buildingsBlob(id)     -> Uint8Array, 30-byte records: id u32, def u16,
//	                          owner u8, pad u8, progress u16, trainCur u16,
//	                          trainLeft u16, trainReq u16, tileX u16,
//	                          tileY u16, queue u16 * 5 (LE)
//	economyBlob(id)       -> Uint8Array, 40-byte records per player slot:
//	                          active u8, pop u16, popCap u16, res u32 * 6,
//	                          era u8, ageTarget u8, ageLeft u16,
//	                          techCur u16, techLeft u16
//	fogBlob(id, player)   -> Uint8Array of 2-bit fog states, row-major
//	defs(id)              -> array of building defs {id,key,costWood,
//	                          costStone,buildTicks,popRoom,size,dropoff,
//	                          trainFood,trainTicks,minEra}
//	unitDefs(id)          -> array of unit defs {id,key,class,hp,speed16,
//	                          vision,damage,damageType,armorPierce,
//	                          armorCrush,range16,rate,costFood,costWood,
//	                          costGold,popCost,trainBy,trainTicks,
//	                          corpseFood,flee,minEra,bonuses:[{class,bonus}]}
//	eras(id)              -> array of era defs {id,key,costFood,costGold,
//	                          reqBuilding,ageTicks}
//	techDefs(id)          -> array of tech defs {id,key,era,building,
//	                          costFood,costWood,costGold,requires:[ids],
//	                          ticks}
//	playerTechs(id, p)    -> array of owned tech ids for player p
//	victory(id)           -> {winner,kind}; winner 255 while the game is
//	                          open, kind 0 none / 1 conquest / 2 wonder /
//	                          3 relic
//	relicsBlob(id)        -> Uint8Array, 5-byte records: x u16, y u16,
//	                          owner u8 (255 = unclaimed)
//	version()             -> bridge version string
//
// Invalid world ids panic, which surfaces as a JavaScript exception; that
// is a programming error in the host, never game state.
package main

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"syscall/js"

	"rts/sim/bots"
	"rts/sim/core"
	"rts/sim/entity"
	"rts/sim/fog"
	"rts/sim/lockstep"
	"rts/sim/proto"
	"rts/sim/worldgen"
)

const bridgeVersion = "0.1.0"

type instance struct {
	world  *core.World
	queue  lockstep.Queue
	brains []*bots.Brain
}

var instances []*instance

func newWorld(this js.Value, args []js.Value) any {
	var seed uint64 = 0x5eed
	players := worldgen.DefaultPlayers
	if len(args) > 0 {
		switch args[0].Type() {
		case js.TypeString:
			v, err := strconv.ParseUint(strings.TrimPrefix(args[0].String(), "0x"), 16, 64)
			if err != nil {
				panic("RTSSim: bad seed " + args[0].String())
			}
			seed = v
		case js.TypeNumber:
			seed = uint64(args[0].Float())
		}
	}
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		players = args[1].Int()
	}
	instances = append(instances, &instance{world: core.NewWorldSize(seed, worldgen.DefaultSize, players)})
	return float64(len(instances) - 1)
}

func mustID(args []js.Value) *instance {
	if len(args) == 0 {
		panic("RTSSim: missing world id")
	}
	id := args[0].Int()
	if id < 0 || id >= len(instances) {
		panic("RTSSim: invalid world id")
	}
	return instances[id]
}

func order(this js.Value, args []js.Value) any {
	inst := mustID(args)
	if len(args) < 3 {
		panic("RTSSim: order requires (id, player, kind[, payload])")
	}
	o := proto.Order{
		Tick:   inst.world.Tick() + lockstep.Delay,
		Player: uint8(args[1].Int()),
		Kind:   proto.Kind(uint16(args[2].Int())),
	}
	if len(args) > 3 && args[3].Type() == js.TypeObject &&
		args[3].InstanceOf(js.Global().Get("Uint8Array")) {
		payload := make([]byte, args[3].Get("length").Int())
		js.CopyBytesToGo(payload, args[3])
		o.Payload = payload
	}
	inst.queue.Add(o.Tick, o)
	return nil
}

func step(this js.Value, args []js.Value) any {
	inst := mustID(args)
	due := inst.queue.Due(inst.world.Tick())
	tick := inst.world.Tick()
	for _, br := range inst.brains {
		due = append(due, br.Step(inst.world, tick)...)
	}
	inst.world.Step(due)
	return nil
}

// brainNew(id, player, difficulty) registers a bot brain on a world and
// returns its handle. Solo mode drives bots fully inside the wasm sim;
// their orders join every local step, keeping replays of the human
// queue deterministic.
// debugSpawn(id, owner, defID, count) mass-spawns units for the E2E
// performance scenario; mirrors core.DebugSpawnUnits.
func debugSpawn(this js.Value, args []js.Value) any {
	inst := mustID(args)
	if len(args) < 4 {
		panic("RTSSim: debugSpawn requires (id, owner, defID, count)")
	}
	inst.world.DebugSpawnUnits(uint8(args[1].Int()), uint16(args[2].Int()), args[3].Int())
	return nil
}

func brainNew(this js.Value, args []js.Value) any {
	inst := mustID(args)
	if len(args) < 3 {
		panic("RTSSim: brainNew requires (id, player, difficulty)")
	}
	var d bots.Difficulty
	switch strings.ToLower(args[2].String()) {
	case "easy":
		d = bots.Easy
	case "hard":
		d = bots.Hard
	default:
		d = bots.Standard
	}
	inst.brains = append(inst.brains, bots.New(uint8(args[1].Int()), d))
	return float64(len(inst.brains) - 1)
}

// stepWith(id, ptr, len) steps exactly one tick with an explicit order
// batch received from the authoritative server (online mode). Blob layout:
// count u16, then per order: player u8, kind u16, payloadLen u16, payload.
func stepWith(this js.Value, args []js.Value) any {
	inst := mustID(args)
	if len(args) < 2 {
		panic("RTSSim: stepWith requires (id, ptr)")
	}
	raw := make([]byte, args[1].Get("length").Int())
	js.CopyBytesToGo(raw, args[1])
	n := int(binary.LittleEndian.Uint16(raw))
	off := 2
	due := make([]proto.Order, 0, n)
	for i := 0; i < n; i++ {
		o := proto.Order{
			Tick:   inst.world.Tick(),
			Player: raw[off],
			Kind:   proto.Kind(binary.LittleEndian.Uint16(raw[off+1:])),
		}
		pl := int(binary.LittleEndian.Uint16(raw[off+3:]))
		off += 5
		if pl > 0 {
			o.Payload = append([]byte(nil), raw[off:off+pl]...)
			off += pl
		}
		due = append(due, o)
	}
	inst.world.Step(due)
	return nil
}

func tick(this js.Value, args []js.Value) any {
	return float64(mustID(args).world.Tick())
}

func hashHex(this js.Value, args []js.Value) any {
	return mustID(args).world.HashHex()
}

func seed(this js.Value, args []js.Value) any {
	return float64(mustID(args).world.Seed())
}

func mapSize(this js.Value, args []js.Value) any {
	return float64(mustID(args).world.Map().Grid().Size())
}

func mapTiles(this js.Value, args []js.Value) any {
	tiles := mustID(args).world.Map().Grid().Tiles()
	buf := make([]byte, len(tiles))
	for i, t := range tiles {
		buf[i] = byte(t)
	}
	out := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(out, buf)
	return out
}

func mapHeights(this js.Value, args []js.Value) any {
	verts := mustID(args).world.Map().Grid().Vertices()
	buf := make([]byte, len(verts)*2)
	for i, v := range verts {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
	}
	out := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(out, buf)
	return out
}

func mapHashHex(this js.Value, args []js.Value) any {
	return fmt.Sprintf("%016x", mustID(args).world.Map().Hash())
}

func spawnCount(this js.Value, args []js.Value) any {
	return float64(len(mustID(args).world.Map().Spawns()))
}

func spawnAt(this js.Value, args []js.Value) any {
	spawns := mustID(args).world.Map().Spawns()
	i := args[1].Int()
	if i < 0 || i >= len(spawns) {
		panic("RTSSim: spawn index out of range")
	}
	return float64(spawns[i][0]<<16 | spawns[i][1])
}

func version(this js.Value, args []js.Value) any {
	return bridgeVersion
}

func u16v(v int32) uint16 {
	if v < 0 {
		return 0
	}
	if v > 0xffff {
		return 0xffff
	}
	return uint16(v)
}

func blob(n int, fill func(buf []byte)) js.Value {
	buf := make([]byte, n)
	fill(buf)
	out := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(out, buf)
	return out
}

func unitsBlob(this js.Value, args []js.Value) any {
	units := mustID(args).world.Units()
	return blob(units.Len()*24, func(buf []byte) {
		off := 0
		units.Each(func(id entity.ID, u *core.Unit) {
			binary.LittleEndian.PutUint32(buf[off:], uint32(id))
			buf[off+4] = u.Owner
			buf[off+5] = u.Kind
			buf[off+6] = u.State
			buf[off+7] = byte(u.CarriedK)
			binary.LittleEndian.PutUint16(buf[off+8:], u16v(int32(u.X)>>12))
			binary.LittleEndian.PutUint16(buf[off+10:], u16v(int32(u.Y)>>12))
			binary.LittleEndian.PutUint16(buf[off+12:], u16v(u.HP))
			binary.LittleEndian.PutUint16(buf[off+14:], u.DefID)
			binary.LittleEndian.PutUint32(buf[off+16:], uint32(u.Target))
			binary.LittleEndian.PutUint16(buf[off+20:], u16v(u.Carried))
			off += 24
		})
	})
}

func nodesBlob(this js.Value, args []js.Value) any {
	nodes := mustID(args).world.Nodes()
	return blob(nodes.Len()*16, func(buf []byte) {
		off := 0
		nodes.Each(func(id entity.ID, n *core.Node) {
			binary.LittleEndian.PutUint32(buf[off:], uint32(id))
			buf[off+4] = byte(n.Kind)
			binary.LittleEndian.PutUint16(buf[off+5:], u16v(n.Amount))
			binary.LittleEndian.PutUint16(buf[off+7:], u16v(int32(n.X)>>12))
			binary.LittleEndian.PutUint16(buf[off+9:], u16v(int32(n.Y)>>12))
			binary.LittleEndian.PutUint16(buf[off+11:], u16v(n.TileX))
			binary.LittleEndian.PutUint16(buf[off+13:], u16v(n.TileY))
			off += 16
		})
	})
}

func buildingsBlob(this js.Value, args []js.Value) any {
	bs := mustID(args).world.Buildings()
	return blob(bs.Len()*30, func(buf []byte) {
		off := 0
		bs.Each(func(id entity.ID, b *core.Building) {
			binary.LittleEndian.PutUint32(buf[off:], uint32(id))
			binary.LittleEndian.PutUint16(buf[off+4:], b.DefID)
			buf[off+6] = b.Owner
			binary.LittleEndian.PutUint16(buf[off+8:], u16v(b.Progress))
			binary.LittleEndian.PutUint16(buf[off+10:], b.TrainCur)
			binary.LittleEndian.PutUint16(buf[off+12:], u16v(b.TrainLeft))
			binary.LittleEndian.PutUint16(buf[off+14:], u16v(b.TrainReq))
			binary.LittleEndian.PutUint16(buf[off+16:], u16v(b.TileX))
			binary.LittleEndian.PutUint16(buf[off+18:], u16v(b.TileY))
			for i := 0; i < 5; i++ {
				binary.LittleEndian.PutUint16(buf[off+20+i*2:], b.TrainQueue[i])
			}
			off += 30
		})
	})
}

func economyBlob(this js.Value, args []js.Value) any {
	w := mustID(args).world
	ps := w.Players()
	return blob(core.MaxPlayers*40, func(buf []byte) {
		for i := 0; i < core.MaxPlayers; i++ {
			off := i * 40
			if ps[i].Active {
				buf[off] = 1
			}
			binary.LittleEndian.PutUint16(buf[off+1:], u16v(ps[i].Pop))
			binary.LittleEndian.PutUint16(buf[off+3:], u16v(w.PopCap(uint8(i))))
			for k := 0; k < 6; k++ {
				binary.LittleEndian.PutUint32(buf[off+5+k*4:], uint32(ps[i].Res[k]))
			}
			buf[off+32] = ps[i].Era
			buf[off+33] = ps[i].AgeTarget
			binary.LittleEndian.PutUint16(buf[off+34:], u16v(ps[i].AgeLeft))
			binary.LittleEndian.PutUint16(buf[off+36:], ps[i].TechCur)
			binary.LittleEndian.PutUint16(buf[off+38:], u16v(ps[i].TechLeft))
		}
	})
}

func relicsBlob(this js.Value, args []js.Value) any {
	w := mustID(args).world
	rels := w.Relics()
	return blob(len(rels)*5, func(buf []byte) {
		for i, r := range rels {
			off := i * 5
			binary.LittleEndian.PutUint16(buf[off:], uint16(r.X))
			binary.LittleEndian.PutUint16(buf[off+2:], uint16(r.Y))
			buf[off+4] = r.Owner
		}
	})
}

func victory(this js.Value, args []js.Value) any {
	w := mustID(args).world
	owner, kind := w.Winner()
	o := js.Global().Get("Object").New()
	o.Set("winner", int(owner))
	o.Set("kind", int(kind))
	return o
}

func playerTechs(this js.Value, args []js.Value) any {
	w := mustID(args)
	if len(args) < 2 {
		panic("RTSSim: playerTechs requires (id, player)")
	}
	ps := w.world.Players()
	player := uint8(args[1].Int())
	arr := js.Global().Get("Array").New()
	if player < core.MaxPlayers {
		bits := ps[player].Techs
		for i, t := range w.world.TechDefs().Defs() {
			if bits&(uint64(1)<<uint(i)) != 0 {
				arr.Call("push", int(t.ID))
			}
		}
	}
	return arr
}

func eras(this js.Value, args []js.Value) any {
	w := mustID(args)
	arr := js.Global().Get("Array").New()
	for _, d := range w.world.EraDefs().Defs() {
		o := js.Global().Get("Object").New()
		o.Set("id", int(d.ID))
		o.Set("key", d.Key)
		o.Set("costFood", d.CostFood)
		o.Set("costGold", d.CostGold)
		o.Set("reqBuilding", d.ReqBuilding)
		o.Set("ageTicks", d.AgeTicks)
		arr.Call("push", o)
	}
	return arr
}

func techDefs(this js.Value, args []js.Value) any {
	w := mustID(args)
	arr := js.Global().Get("Array").New()
	for _, d := range w.world.TechDefs().Defs() {
		o := js.Global().Get("Object").New()
		o.Set("id", int(d.ID))
		o.Set("key", d.Key)
		o.Set("era", int(d.Era))
		o.Set("building", d.Building)
		o.Set("costFood", d.CostFood)
		o.Set("costWood", d.CostWood)
		o.Set("costGold", d.CostGold)
		o.Set("ticks", d.Ticks)
		reqs := js.Global().Get("Array").New()
		for _, r := range d.Requires {
			reqs.Call("push", int(r))
		}
		o.Set("requires", reqs)
		arr.Call("push", o)
	}
	return arr
}

func fogBlob(this js.Value, args []js.Value) any {
	w := mustID(args)
	if len(args) < 2 {
		panic("RTSSim: fogBlob requires (id, player)")
	}
	g := w.world.Fog(uint8(args[1].Int()))
	buf := g.Packed(make([]byte, 0, fog.PackedLen(g.Size())))
	out := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(out, buf)
	return out
}

func defs(this js.Value, args []js.Value) any {
	arr := js.Global().Get("Array").New()
	for _, d := range mustID(args).world.BuildingDefs().Defs() {
		o := js.Global().Get("Object").New()
		o.Set("id", d.ID)
		o.Set("key", d.Key)
		o.Set("costWood", d.CostWood)
		o.Set("costStone", d.CostStone)
		o.Set("buildTicks", d.BuildTicks)
		o.Set("popRoom", d.PopRoom)
		o.Set("size", d.Size)
		o.Set("dropoff", d.Dropoff)
		o.Set("trainFood", d.TrainFood)
		o.Set("trainTicks", d.TrainTicks)
		o.Set("minEra", d.MinEra)
		arr.Call("push", o)
	}
	return arr
}

func unitDefs(this js.Value, args []js.Value) any {
	arr := js.Global().Get("Array").New()
	for _, d := range mustID(args).world.UnitDefs().Defs() {
		o := js.Global().Get("Object").New()
		o.Set("id", d.ID)
		o.Set("key", d.Key)
		o.Set("class", d.Class)
		o.Set("hp", d.HP)
		o.Set("speed16", d.Speed16)
		o.Set("vision", d.Vision)
		o.Set("damage", d.Damage)
		o.Set("damageType", d.DamageType)
		o.Set("armorPierce", d.ArmorPierce)
		o.Set("armorCrush", d.ArmorCrush)
		o.Set("range16", d.Range16)
		o.Set("rate", d.Rate)
		o.Set("costFood", d.CostFood)
		o.Set("costWood", d.CostWood)
		o.Set("costGold", d.CostGold)
		o.Set("popCost", d.PopCost)
		o.Set("trainBy", d.TrainBy)
		o.Set("trainTicks", d.TrainTicks)
		o.Set("corpseFood", d.CorpseFood)
		o.Set("flee", d.Flee)
		o.Set("minEra", d.MinEra)
		bonuses := js.Global().Get("Array").New()
		for _, b := range d.Bonuses {
			bo := js.Global().Get("Object").New()
			bo.Set("class", b.Class)
			bo.Set("bonus", b.Bonus)
			bonuses.Call("push", bo)
		}
		o.Set("bonuses", bonuses)
		arr.Call("push", o)
	}
	return arr
}

func main() {
	obj := js.Global().Get("Object").New()
	obj.Set("newWorld", js.FuncOf(newWorld))
	obj.Set("order", js.FuncOf(order))
	obj.Set("step", js.FuncOf(step))
	obj.Set("stepWith", js.FuncOf(stepWith))
	obj.Set("brainNew", js.FuncOf(brainNew))
	obj.Set("debugSpawn", js.FuncOf(debugSpawn))
	obj.Set("tick", js.FuncOf(tick))
	obj.Set("hashHex", js.FuncOf(hashHex))
	obj.Set("seed", js.FuncOf(seed))
	obj.Set("mapSize", js.FuncOf(mapSize))
	obj.Set("mapTiles", js.FuncOf(mapTiles))
	obj.Set("mapHeights", js.FuncOf(mapHeights))
	obj.Set("mapHashHex", js.FuncOf(mapHashHex))
	obj.Set("spawnCount", js.FuncOf(spawnCount))
	obj.Set("spawnAt", js.FuncOf(spawnAt))
	obj.Set("unitsBlob", js.FuncOf(unitsBlob))
	obj.Set("nodesBlob", js.FuncOf(nodesBlob))
	obj.Set("buildingsBlob", js.FuncOf(buildingsBlob))
	obj.Set("economyBlob", js.FuncOf(economyBlob))
	obj.Set("defs", js.FuncOf(defs))
	obj.Set("unitDefs", js.FuncOf(unitDefs))
	obj.Set("eras", js.FuncOf(eras))
	obj.Set("techDefs", js.FuncOf(techDefs))
	obj.Set("playerTechs", js.FuncOf(playerTechs))
	obj.Set("victory", js.FuncOf(victory))
	obj.Set("relicsBlob", js.FuncOf(relicsBlob))
	obj.Set("fogBlob", js.FuncOf(fogBlob))
	obj.Set("version", js.FuncOf(version))
	js.Global().Set("RTSSim", obj)
	select {}
}
