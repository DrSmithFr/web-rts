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
//	unitsBlob(id)         -> Uint8Array, 20-byte records: id u32, owner u8,
//	                          kind u8, state u8, carried u8, x16 u16, y16 u16,
//	                          pad u16, target u32, carriedAmt u16 (LE)
//	nodesBlob(id)         -> Uint8Array, 16-byte records: id u32, kind u8,
//	                          amount u16, x16 u16, y16 u16, tileX u16,
//	                          tileY u16, pad u8
//	buildingsBlob(id)     -> Uint8Array, 16-byte records: id u32, def u16,
//	                          owner u8, progress u16, trainLeft u8,
//	                          trainReq u8, tileX u16, tileY u16, pad u8
//	economyBlob(id)       -> Uint8Array, 32-byte records per player slot:
//	                          active u8, pop u16, popCap u16, res u32 * 6
//	fogBlob(id, player)   -> Uint8Array of 2-bit fog states, row-major
//	defs(id)              -> array of building defs {id,key,costWood,
//	                          costStone,buildTicks,popRoom,size,dropoff,
//	                          trainFood,trainTicks}
//	version()             -> bridge version string
//
// Invalid world ids panic, which surfaces as a JavaScript exception; that
// is a programming error in the host, never game state.
package main

import (
	"encoding/binary"
	"fmt"
	"syscall/js"

	"rts/sim/core"
	"rts/sim/entity"
	"rts/sim/fog"
	"rts/sim/lockstep"
	"rts/sim/proto"
)

const bridgeVersion = "0.1.0"

type instance struct {
	world *core.World
	queue lockstep.Queue
}

var instances []*instance

func newWorld(this js.Value, args []js.Value) any {
	var seed uint64 = 0x5eed
	if len(args) > 0 {
		seed = uint64(args[0].Float())
	}
	instances = append(instances, &instance{world: core.NewWorld(seed)})
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
	inst.world.Step(inst.queue.Due(inst.world.Tick()))
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
	return blob(units.Len()*20, func(buf []byte) {
		off := 0
		units.Each(func(id entity.ID, u *core.Unit) {
			binary.LittleEndian.PutUint32(buf[off:], uint32(id))
			buf[off+4] = u.Owner
			buf[off+5] = u.Kind
			buf[off+6] = u.State
			buf[off+7] = byte(u.CarriedK)
			binary.LittleEndian.PutUint16(buf[off+8:], u16v(int32(u.X)>>12))
			binary.LittleEndian.PutUint16(buf[off+10:], u16v(int32(u.Y)>>12))
			binary.LittleEndian.PutUint32(buf[off+12:], uint32(u.Target))
			binary.LittleEndian.PutUint16(buf[off+16:], u16v(u.Carried))
			off += 20
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
	return blob(bs.Len()*16, func(buf []byte) {
		off := 0
		bs.Each(func(id entity.ID, b *core.Building) {
			binary.LittleEndian.PutUint32(buf[off:], uint32(id))
			binary.LittleEndian.PutUint16(buf[off+4:], b.DefID)
			buf[off+6] = b.Owner
			binary.LittleEndian.PutUint16(buf[off+7:], u16v(b.Progress))
			buf[off+9] = byte(b.TrainLeft)
			buf[off+10] = byte(b.TrainReq)
			binary.LittleEndian.PutUint16(buf[off+11:], u16v(b.TileX))
			binary.LittleEndian.PutUint16(buf[off+13:], u16v(b.TileY))
			off += 16
		})
	})
}

func economyBlob(this js.Value, args []js.Value) any {
	w := mustID(args).world
	ps := w.Players()
	return blob(core.MaxPlayers*32, func(buf []byte) {
		for i := 0; i < core.MaxPlayers; i++ {
			off := i * 32
			if ps[i].Active {
				buf[off] = 1
			}
			binary.LittleEndian.PutUint16(buf[off+1:], u16v(ps[i].Pop))
			binary.LittleEndian.PutUint16(buf[off+3:], u16v(w.PopCap(uint8(i))))
			for k := 0; k < 6; k++ {
				binary.LittleEndian.PutUint32(buf[off+5+k*4:], uint32(ps[i].Res[k]))
			}
		}
	})
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
		arr.Call("push", o)
	}
	return arr
}

func main() {
	obj := js.Global().Get("Object").New()
	obj.Set("newWorld", js.FuncOf(newWorld))
	obj.Set("order", js.FuncOf(order))
	obj.Set("step", js.FuncOf(step))
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
	obj.Set("fogBlob", js.FuncOf(fogBlob))
	obj.Set("version", js.FuncOf(version))
	js.Global().Set("RTSSim", obj)
	select {}
}
