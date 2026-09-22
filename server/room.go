// Package server implements the authoritative multiplayer host: lobbies
// with invitation codes, a WebSocket transport, order relay stamped T+2,
// per-tick order batches with server hashes, hash arbitration with desync
// events, latency pausing and reconnect catch-up by order replay.
package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"rts/server/wire"
	"rts/sim/core"
	"rts/sim/lockstep"
	"rts/sim/proto"
	"rts/sim/worldgen"
)

// TickInterval is the simulation cadence (20 Hz).
const TickInterval = 50 * time.Millisecond

// PauseAfterHashMisses is how many hash windows a connected player may
// fall behind before the room pauses for everyone.
const PauseAfterHashMisses = 4

var errRoomFull = errors.New("room is full")

func newCode() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func newToken() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type client struct {
	mu       sync.Mutex
	slot     int
	token    string
	name     string
	conn     *websocket.Conn
	send     chan []byte
	closed   bool
	hashTick uint32 // highest tick the client has confirmed with a hash
	desynced bool
}

func (c *client) push(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.send == nil {
		return
	}
	select {
	case c.send <- b:
	default:
	}
}

// abandon marks a client whose slot was handed to a new connection. Its
// own read loop sees closed on exit and leaves the new owner untouched.
func (c *client) abandon() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

func (c *client) closeSend() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	if c.send != nil {
		close(c.send)
	}
}

// logEntry is one executed tick: the exact order batch the world stepped
// with. Reconnecting clients replay the log from the seed to catch up.
type logEntry struct {
	Tick   uint32        `json:"tick"`
	Orders []proto.Order `json:"orders"`
}

// Room is one game: lobby phase, then an authoritative 20 Hz loop that
// broadcasts per-tick order batches and arbitrates client hashes.
type Room struct {
	Code string

	mu        sync.Mutex
	state     string // lobby, running
	seed      int64
	humans    int
	bots      int
	players   [core.MaxPlayers]*playerSlot
	world     *core.World
	queue     lockstep.Queue
	log       []logEntry
	tick      uint32
	hashes    map[uint32]uint64 // recent server hashes by tick
	paused    bool
	done      chan struct{}
	broadcast func(b []byte)
	hub       *Hub
}

type playerSlot struct {
	name   string
	color  int
	bot    bool
	client *client
	online bool
}

func newRoom(hub *Hub, seed int64) *Room {
	if seed == 0 {
		seed = int64(time.Now().UnixNano())
	}
	return &Room{
		Code:   "",
		seed:   seed,
		hashes: map[uint32]uint64{},
		done:   make(chan struct{}),
		hub:    hub,
	}
}

func (r *Room) infoLocked() []wire.PlayerInfo {
	var out []wire.PlayerInfo
	for i := 0; i < core.MaxPlayers; i++ {
		p := r.players[i]
		if p == nil {
			continue
		}
		out = append(out, wire.PlayerInfo{
			Slot: i, Name: p.name, Color: p.color, Bot: p.bot, Online: p.online,
		})
	}
	return out
}

// join reserves a slot for a human, returning the slot index.
func (r *Room) joinLocked(name string, c *client) (int, error) {
	for i := 0; i < core.MaxPlayers; i++ {
		if r.players[i] == nil {
			r.players[i] = &playerSlot{name: name, color: i, client: c, online: true}
			c.slot = i
			r.humans++
			return i, nil
		}
	}
	return 0, errRoomFull
}

// Start launches the authoritative loop with humans + bots as players.
func (r *Room) Start() error {
	r.mu.Lock()
	if r.state == "running" {
		r.mu.Unlock()
		return errors.New("already started")
	}
	r.state = "running"
	r.playersFillBots()
	total := 0
	for i := range r.players {
		if r.players[i] != nil {
			total++
		}
	}
	r.world = core.NewWorldSize(uint64(r.seed), worldgen.DefaultSize, total)
	r.log = nil
	r.tick = 0
	r.mu.Unlock()
	go r.loop()
	return nil
}

func (r *Room) playersFillBots() {
	for placed := 0; placed < r.bots; placed++ {
		for i := 0; i < core.MaxPlayers; i++ {
			if r.players[i] == nil {
				r.players[i] = &playerSlot{name: fmt.Sprintf("Bot %d", placed+1), color: i, bot: true}
				break
			}
		}
	}
}

// loop runs the authoritative tick loop until the room closes.
func (r *Room) loop() {
	tickEvery := TickInterval
	if Fast {
		tickEvery = TickInterval / 50
	}
	tk := time.NewTicker(tickEvery)
	defer tk.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-tk.C:
			r.mu.Lock()
			if !r.paused {
				r.stepLocked()
			}
			r.checkLatencyLocked()
			r.mu.Unlock()
		}
	}
}

func (r *Room) stepLocked() {
	due := r.queue.Due(r.tick)
	r.world.Step(due)
	r.log = append(r.log, logEntry{Tick: r.tick, Orders: due})
	h := r.world.Hash()
	r.hashes[r.tick] = h
	for t := range r.hashes {
		if t+256 < r.tick {
			delete(r.hashes, t)
		}
	}
	hexHash := fmt.Sprintf("%016x", h)
	msg := wire.ServerMsg{Type: "tick", Tick: &r.tick, Hash: &hexHash}
	for _, o := range due {
		msg.Orders = append(msg.Orders, wire.OrderMsg{
			Tick:    r.tick,
			Player:  int(o.Player),
			Kind:    int(o.Kind),
			Payload: base64.StdEncoding.EncodeToString(o.Payload),
		})
	}
	r.broadcastMsg(msg)
	r.tick++
}

// checkLatencyLocked pauses when a connected, non-desynced player falls
// far behind on hash confirmations, and resumes once they catch up.
func (r *Room) checkLatencyLocked() {
	if r.state != "running" {
		return
	}
	horizon := r.tick
	if horizon < core.HashPeriodTicks*PauseAfterHashMisses {
		return
	}
	worst := ^uint32(0)
	for i := range r.players {
		p := r.players[i]
		if p == nil || p.bot || p.client == nil || p.client.closed || p.client.desynced {
			continue
		}
		if p.client.hashTick < worst {
			worst = p.client.hashTick
		}
	}
	lagging := worst != ^uint32(0) && worst+core.HashPeriodTicks*PauseAfterHashMisses < horizon
	if lagging && !r.paused {
		r.paused = true
		r.broadcastMsg(wire.ServerMsg{Type: "pause", Tick: &r.tick, Reason: "latency"})
	} else if !lagging && r.paused {
		r.paused = false
		r.broadcastMsg(wire.ServerMsg{Type: "resume", Tick: &r.tick})
	}
}

func (r *Room) broadcastMsg(m wire.ServerMsg) {
	b := wire.Encode(m)
	for i := range r.players {
		p := r.players[i]
		if p != nil && p.client != nil && !p.client.closed {
			p.client.push(b)
		}
	}
}

// addOrder stamps a received order for execution at T+2.
func (r *Room) addOrder(slot int, kind uint16, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != "running" {
		return
	}
	o := proto.Order{Tick: r.tick + lockstep.Delay, Player: uint8(slot), Kind: proto.Kind(kind), Payload: payload}
	r.queue.Add(r.tick+lockstep.Delay, o)
}

// reportHash records a client hash confirmation and detects desyncs.
func (r *Room) reportHash(slot int, tick uint32, h uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.players[slot]
	if p == nil || p.client == nil {
		return
	}
	if tick > p.client.hashTick {
		p.client.hashTick = tick
	}
	if want, ok := r.hashes[tick]; ok && want != h && !p.client.desynced {
		p.client.desynced = true
		r.desyncReportLocked(tick)
	}
}

func (r *Room) desyncReportLocked(tick uint32) {
	var slots []int
	for i := range r.players {
		if p := r.players[i]; p != nil && p.client != nil && p.client.desynced {
			slots = append(slots, i)
		}
	}
	m := wire.ServerMsg{Type: "desync", Tick: &tick}
	for _, s := range slots {
		m.Player = &s
	}
	m.Msg = fmt.Sprintf("desync at tick %d players %v", tick, slots)
	r.broadcastMsg(m)
}

// catchup renders the full order log for a (re)joining client so it can
// rebuild state deterministically from the seed.
func (r *Room) seedHex() string { return fmt.Sprintf("%x", uint64(r.seed)) }

func (r *Room) catchupLocked() wire.ServerMsg {
	hexSeed := r.seedHex()
	m := wire.ServerMsg{Type: "catchup", Seed: &hexSeed, Tick: &r.tick}
	for _, e := range r.log {
		for _, o := range e.Orders {
			m.Orders = append(m.Orders, wire.OrderMsg{
				Tick:    e.Tick,
				Player:  int(o.Player),
				Kind:    int(o.Kind),
				Payload: base64.StdEncoding.EncodeToString(o.Payload),
			})
		}
	}
	return m
}

func (r *Room) close() {
	close(r.done)
}

// MarshalJSON snapshot used by desync diagnostics dumps.
func (r *Room) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, _ := json.Marshal(r.log)
	return b
}
