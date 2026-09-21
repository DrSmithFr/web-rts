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
	obj.Set("version", js.FuncOf(version))
	js.Global().Set("RTSSim", obj)
	select {}
}
