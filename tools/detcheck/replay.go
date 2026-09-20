// Package main implements detcheck: the native replay runner that drives
// a simulation world from a binary scenario file and checks the state
// hash, and the deterministic replay file format itself.
//
// Replay file layout (all little-endian):
//
//	magic    8 bytes  "RTSRPLY1"
//	seed     u64
//	ticks    u32
//	orders   u32
//	order    tick u32 | player u8 | kind u16 | payload uvarint-length-prefixed
//	final    u64  expected hash after the last tick
//
// Order.Tick is the tick at which the order is scheduled; execution
// happens at tick+lockstep.Delay, exactly like a live game.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"rts/sim/core"
	"rts/sim/lockstep"
	"rts/sim/proto"
)

const magic = "RTSRPLY1"

// Replay is a recorded deterministic scenario.
type Replay struct {
	Seed      uint64
	Ticks     uint32
	Orders    []proto.Order
	FinalHash uint64
}

// Load reads a replay file.
func Load(path string) (*Replay, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < len(magic)+20 {
		return nil, errors.New("replay: file too short")
	}
	if string(data[:len(magic)]) != magic {
		return nil, errors.New("replay: bad magic")
	}
	r := &Replay{}
	p := data[len(magic):]
	r.Seed = binary.LittleEndian.Uint64(p)
	r.Ticks = binary.LittleEndian.Uint32(p[8:])
	count := binary.LittleEndian.Uint32(p[12:])
	p = p[16:]
	for i := uint32(0); i < count; i++ {
		o, used, err := proto.Decode(p)
		if err != nil {
			return nil, fmt.Errorf("replay: order %d: %w", i, err)
		}
		payload := make([]byte, len(o.Payload))
		copy(payload, o.Payload)
		o.Payload = payload
		r.Orders = append(r.Orders, o)
		p = p[used:]
	}
	if len(p) < 8 {
		return nil, errors.New("replay: missing final hash")
	}
	r.FinalHash = binary.LittleEndian.Uint64(p)
	return r, nil
}

// Write saves the replay file.
func (r *Replay) Write(path string) error {
	var buf []byte
	buf = append(buf, magic...)
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], r.Seed)
	buf = append(buf, tmp[:]...)
	binary.LittleEndian.PutUint32(tmp[:4], r.Ticks)
	buf = append(buf, tmp[:4]...)
	binary.LittleEndian.PutUint32(tmp[:4], uint32(len(r.Orders)))
	buf = append(buf, tmp[:4]...)
	for _, o := range r.Orders {
		buf = o.EncodeAppend(buf)
	}
	binary.LittleEndian.PutUint64(tmp[:], r.FinalHash)
	buf = append(buf, tmp[:]...)
	return os.WriteFile(path, buf, 0o644)
}

// protoOrder builds a noop order for scenario generation.
func protoOrder(tick uint32, player uint8, payload []byte) proto.Order {
	return proto.Order{Tick: tick, Player: player, Kind: proto.KindNoop, Payload: payload}
}

// Run executes the replay, calling checkpoint(tick, hash) every
// hashPeriod ticks, and returns the final hash.
func (r *Replay) Run(hashPeriod uint32, checkpoint func(tick uint32, h uint64)) uint64 {
	w := core.NewWorld(r.Seed)
	var q lockstep.Queue
	oi := 0
	for t := uint32(0); t < r.Ticks; t++ {
		for oi < len(r.Orders) && r.Orders[oi].Tick <= t {
			q.Add(t+lockstep.Delay, r.Orders[oi])
			oi++
		}
		w.Step(q.Due(t))
		if hashPeriod > 0 && (t+1)%hashPeriod == 0 && checkpoint != nil {
			checkpoint(t+1, w.Hash())
		}
	}
	return w.Hash()
}
