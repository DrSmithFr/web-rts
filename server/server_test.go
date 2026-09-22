// Functional tests: real WebSocket clients against the server, covering
// the lobby cycle, T+2 order relay, induced desync detection, reconnect
// catch-up and a scripted game to relic victory against three idle bots.
package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"rts/server/wire"
	"rts/sim/core"
	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
	"rts/sim/worldgen"
)

func TestMain(m *testing.M) {
	Fast = true
	os.Exit(m.Run())
}

type wsTest struct {
	t     *testing.T
	name  string
	conn  *websocket.Conn
	inbox chan wire.ServerMsg
}

func dial(t *testing.T, url string, name string) *wsTest {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(wsURL(url)+"/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	w := &wsTest{t: t, name: name, conn: c, inbox: make(chan wire.ServerMsg, 4096)}
	go func() {
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				close(w.inbox)
				return
			}
			var m wire.ServerMsg
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			w.inbox <- m
		}
	}()
	return w
}

func (w *wsTest) send(m wire.ClientMsg) {
	w.t.Helper()
	if err := w.conn.WriteJSON(m); err != nil {
		w.t.Fatalf("%s send %s: %v", w.name, m.Type, err)
	}
}

// expect returns the next message of the given type, skipping others.
func (w *wsTest) expect(typ string, timeout time.Duration) wire.ServerMsg {
	w.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case m, ok := <-w.inbox:
			if !ok {
				w.t.Fatalf("%s: connection closed waiting for %s", w.name, typ)
			}
			if m.Type == typ {
				return m
			}
		case <-deadline:
			w.t.Fatalf("%s: timeout waiting for %s", w.name, typ)
		}
	}
}

func wsURL(httpURL string) string {
	return "ws" + httpURL[len("http"):]
}

func newTestHub(t *testing.T) (string, func()) {
	srv := httptest.NewServer(NewHub().Handler())
	return srv.URL, srv.Close
}

func iptr(i int) *int         { return &i }
func iptr64(v int64) *int64   { return &v }
func u32ptr(v uint32) *uint32 { return &v }

func TestLobbyCycle(t *testing.T) {
	url, done := newTestHub(t)
	defer done()

	alice := dial(t, url, "alice")
	alice.send(wire.ClientMsg{Type: "create", Name: "Alice"})
	wel := alice.expect("welcome", 5*time.Second)
	if wel.Player == nil || *wel.Player != 0 || wel.Code == "" {
		t.Fatalf("bad welcome %+v", wel)
	}
	code := wel.Code

	bob := dial(t, url, "bob")
	bob.send(wire.ClientMsg{Type: "join", Code: code, Name: "Bob"})
	bwel := bob.expect("welcome", 5*time.Second)
	if bwel.Player == nil || *bwel.Player != 1 {
		t.Fatalf("bob got slot %v", bwel.Player)
	}
	lm := alice.expect("lobby", 5*time.Second)
	for len(lm.Players) != 2 && lm.Msg == "" {
		lm = alice.expect("lobby", 5*time.Second)
	}
	if len(lm.Players) != 2 {
		t.Fatalf("lobby players = %d, want 2", len(lm.Players))
	}

	bob.conn.Close()
	for lm = alice.expect("lobby", 5*time.Second); len(lm.Players) != 1; {
		lm = alice.expect("lobby", 5*time.Second)
	}

	alice.send(wire.ClientMsg{Type: "start", Code: code})
	st := alice.expect("start", 5*time.Second)
	if st.Seed == nil || *st.Seed == "0" || *st.Seed == "" {
		t.Fatalf("start without seed: %+v", st)
	}
	alice.expect("tick", 5*time.Second)
}

func TestJoinMissingRoom(t *testing.T) {
	url, done := newTestHub(t)
	defer done()
	c := dial(t, url, "solo")
	c.send(wire.ClientMsg{Type: "join", Code: "fffff", Name: "Ghost"})
	em := c.expect("error", 5*time.Second)
	if em.Msg != "no such room" {
		t.Fatalf("msg = %q", em.Msg)
	}
}

func TestRelayDelayTPlus2(t *testing.T) {
	url, done := newTestHub(t)
	defer done()
	c := dial(t, url, "host")
	c.send(wire.ClientMsg{Type: "create", Name: "Host", Seed: iptr64(0x5eed)})
	c.expect("welcome", 5*time.Second)
	c.send(wire.ClientMsg{Type: "start"})
	c.expect("start", 5*time.Second)

	tk := c.expect("tick", 5*time.Second)
	sentTick := *tk.Tick
	payload := base64.StdEncoding.EncodeToString(orders.StopPayload([]uint32{1}))
	c.send(wire.ClientMsg{Type: "order", Kind: iptr(int(orders.Stop)), Payload: payload})

	for {
		tk = c.expect("tick", 5*time.Second)
		for _, o := range tk.Orders {
			if o.Kind == int(orders.Stop) {
				if *tk.Tick < sentTick+2 {
					t.Fatalf("order executed at tick %d, sent at %d: T+2 violated", *tk.Tick, sentTick)
				}
				if *tk.Tick > sentTick+6 {
					t.Fatalf("order executed at tick %d, sent at %d: stamped too late", *tk.Tick, sentTick)
				}
				return
			}
		}
	}
}

func TestDesyncDetection(t *testing.T) {
	url, done := newTestHub(t)
	defer done()
	c := dial(t, url, "host")
	c.send(wire.ClientMsg{Type: "create", Name: "Host", Seed: iptr64(0x5eed)})
	c.expect("welcome", 5*time.Second)
	c.send(wire.ClientMsg{Type: "start"})
	c.expect("start", 5*time.Second)

	tk := c.expect("tick", 5*time.Second)
	good, err := strconv.ParseUint(*tk.Hash, 16, 64)
	if err != nil {
		t.Fatalf("bad hash %q", *tk.Hash)
	}
	wrong := strconv.FormatUint(good^0xdeadbeef, 10)
	c.send(wire.ClientMsg{Type: "hash", Tick: tk.Tick, Hash: &wrong})
	dm := c.expect("desync", 5*time.Second)
	if dm.Tick == nil || *dm.Tick != *tk.Tick {
		t.Fatalf("desync tick %v want %v", dm.Tick, tk.Tick)
	}
}

// mirror steps a local authoritative copy from the server's tick batches
// and asserts hash agreement on every single tick.
type mirror struct {
	w    *core.World
	tick uint32
}

func newMirror(seedHex string, players int) *mirror {
	v, err := strconv.ParseUint(seedHex, 16, 64)
	if err != nil {
		panic(err)
	}
	return &mirror{w: core.NewWorldSize(v, worldgen.DefaultSize, players)}
}

func (m *mirror) stepMsg(msg wire.ServerMsg) {
	if msg.Tick == nil || *msg.Tick < m.tick {
		return
	}
	if *msg.Tick != m.tick {
		panic(fmt.Sprintf("mirror gap: have %d want %d", m.tick, *msg.Tick))
	}
	var due []proto.Order
	for _, o := range msg.Orders {
		p, err := base64.StdEncoding.DecodeString(o.Payload)
		if err != nil {
			panic(err)
		}
		due = append(due, proto.Order{Tick: m.tick, Player: uint8(o.Player), Kind: proto.Kind(o.Kind), Payload: p})
	}
	m.w.Step(due)
	if msg.Hash != nil && fmt.Sprintf("%016x", m.w.Hash()) != *msg.Hash {
		panic(fmt.Sprintf("hash mismatch at tick %d: local %016x server %s", m.tick, m.w.Hash(), *msg.Hash))
	}
	m.tick++
}

func TestReconnectCatchup(t *testing.T) {
	url, done := newTestHub(t)
	defer done()
	host := dial(t, url, "host")
	host.send(wire.ClientMsg{Type: "create", Name: "Host", Seed: iptr64(0x5eed)})
	wel := host.expect("welcome", 5*time.Second)
	token := wel.Token
	code := wel.Code
	host.send(wire.ClientMsg{Type: "start"})
	host.expect("start", 5*time.Second)

	for i := 0; i < 20; i++ {
		host.expect("tick", 5*time.Second)
	}
	host.conn.Close()

	j := dial(t, url, "rejoin")
	j.send(wire.ClientMsg{Type: "join", Code: code, Token: token})
	wel = j.expect("welcome", 5*time.Second)
	if wel.Seed == nil || wel.Tick == nil {
		t.Fatalf("reconnect welcome lacks catch-up: %+v", wel)
	}
	m := newMirror(*wel.Seed, 1)
	for t := uint32(0); t < *wel.Tick; t++ {
		var due []proto.Order
		for _, o := range wel.Orders {
			if o.Tick == t {
				p, _ := base64.StdEncoding.DecodeString(o.Payload)
				due = append(due, proto.Order{Tick: t, Player: uint8(o.Player), Kind: proto.Kind(o.Kind), Payload: p})
			}
		}
		m.w.Step(due)
	}
	m.tick = m.w.Tick()
	if m.w.Tick() != *wel.Tick {
		t.Fatalf("catch-up ended at tick %d, server said %d", m.w.Tick(), *wel.Tick)
	}
	for i := 0; i < 30; i++ {
		tk := j.expect("tick", 5*time.Second)
		m.stepMsg(tk)
		good := strconv.FormatUint(m.w.Hash(), 10)
		j.send(wire.ClientMsg{Type: "hash", Tick: tk.Tick, Hash: &good})
	}
}

type buildTry struct {
	site, givenIn bool
	waited, next  int
}

var testBuildOffsets = [][2]int{{-6, 4}, {4, -6}, {6, 4}, {-4, 6}, {-6, -4}, {4, 6}}

// driver mirrors the authority and scripts orders from mirror state.
type driver struct {
	t     *testing.T
	m     *mirror
	order func(kind uint16, payload []byte)
}

func (d *driver) completeBuilding(player uint8, defID uint16) entity.ID {
	var id entity.ID
	d.m.w.Buildings().Each(func(eid entity.ID, b *core.Building) {
		if b.Owner == player && b.DefID == defID && b.Progress == 0 {
			id = eid
		}
	})
	return id
}

func (d *driver) buildingExists(player uint8, defID uint16) bool {
	found := false
	d.m.w.Buildings().Each(func(_ entity.ID, b *core.Building) {
		if b.Owner == player && b.DefID == defID {
			found = true
		}
	})
	return found
}

func (d *driver) tryBuilding(bt *buildTry, player uint8, builder entity.ID, spawn [2]int, defID uint16) bool {
	if !bt.site && d.buildingExists(player, defID) {
		bt.site = true
	}
	if bt.site {
		return d.completeBuilding(player, defID) != 0
	}
	if bt.givenIn || bt.waited > 0 {
		if bt.waited > 0 {
			bt.waited--
		}
		return false
	}
	if bt.next >= len(testBuildOffsets) {
		bt.givenIn = true
		return false
	}
	o := testBuildOffsets[bt.next]
	bt.next++
	tx, ty := spawn[0]+o[0], spawn[1]+o[1]
	d.order(orders.Build, orders.BuildPayload(uint32(builder), defID, uint16(tx), uint16(ty)))
	bt.waited = 4
	return false
}

func playerUnitIDs(w *core.World, owner uint8) []entity.ID {
	var ids []entity.ID
	w.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.Owner == owner {
			ids = append(ids, id)
		}
	})
	return ids
}

func nearestNode(w *core.World, from *core.Unit, kind resource.Kind) entity.ID {
	var best entity.ID
	bd := int64(1) << 62
	w.Nodes().Each(func(id entity.ID, n *core.Node) {
		if n.Kind != kind || n.Amount <= 0 {
			return
		}
		dx, dy := int64(n.X-from.X), int64(n.Y-from.Y)
		if d := dx*dx + dy*dy; d < bd {
			bd, best = d, id
		}
	})
	return best
}

// TestRelicVictoryVsBots plays a scripted run against three idle bots:
// gather, age up, barracks, lancer, collect all five relics and hold the
// lead until the relic victory timer fires on the mirror world.
func TestRelicVictoryVsBots(t *testing.T) {
	if testing.Short() {
		t.Skip("long game")
	}
	url, done := newTestHub(t)
	defer done()
	c := dial(t, url, "host")
	c.send(wire.ClientMsg{Type: "create", Name: "Host", Seed: iptr64(0x5eed), Bots: iptr(3)})
	c.expect("welcome", 5*time.Second)
	c.send(wire.ClientMsg{Type: "start"})
	st := c.expect("start", 5*time.Second)
	players := len(st.Players)
	if players != 4 {
		t.Fatalf("players = %d, want 4", players)
	}
	m := newMirror(*st.Seed, players)
	d := &driver{t: t, m: m, order: func(kind uint16, payload []byte) {
		c.send(wire.ClientMsg{Type: "order", Kind: iptr(int(kind)),
			Payload: base64.StdEncoding.EncodeToString(payload)})
	}}

	spawns := m.w.Map().Spawns()
	v0 := playerUnitIDs(m.w, 0)
	gather := func(id entity.ID, kind resource.Kind) {
		if n := nearestNode(m.w, m.w.Units().Get(id), kind); n != 0 {
			d.order(orders.Gather, orders.GatherPayload(uint32(id), uint32(n)))
		}
	}

	var (
		bar          buildTry
		aged, lanced bool
		relicStage   int
		lastRegath   uint32
		lastMarch    uint32
	)
	for _, id := range v0[:2] {
		gather(id, resource.Food)
	}
	gather(v0[2], resource.Gold)

	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		tk := c.expect("tick", 60*time.Second)
		m.stepMsg(tk)
		good := strconv.FormatUint(m.w.Hash(), 10)
		c.send(wire.ClientMsg{Type: "hash", Tick: tk.Tick, Hash: &good})
		tick := *tk.Tick
		pl := &m.w.Players()[0]

		if tick-lastRegath > 300 && (pl.Res[resource.Food] < 320 || pl.Res[resource.Gold] < 20) {
			lastRegath = tick
			gather(v0[1], resource.Food)
			gather(v0[2], resource.Gold)
		}
		if !aged && pl.Res[resource.Food] >= 250 {
			aged = true
			d.order(orders.AgeUp, orders.AgeUpPayload(1))
		}
		if pl.Era >= 1 {
			doneBarracks := d.tryBuilding(&bar, 0, v0[1], spawns[0], 4)
			if doneBarracks && !lanced &&
				pl.Res[resource.Food] >= 60 && pl.Res[resource.Gold] >= 20 {
				lanced = true
				d.order(orders.Train, orders.TrainPayload(uint32(d.completeBuilding(0, 4)), 2))
			}
		}
		if lanced && relicStage < core.RelicCount && tick-lastMarch > 100 {
			var lancer entity.ID
			m.w.Units().Each(func(id entity.ID, u *core.Unit) {
				if u.Owner == 0 && u.DefID == 2 && u.HP > 0 {
					lancer = id
				}
			})
			if lancer != 0 {
				rels := m.w.Relics()
				if rels[relicStage].Owner == 0 {
					relicStage++
				} else {
					lastMarch = tick
					d.order(orders.MoveGroup, orders.MoveGroupPayload([]uint32{uint32(lancer)},
						uint16(rels[relicStage].X*16+8), uint16(rels[relicStage].Y*16+8)))
				}
			}
		}
		if w, kind := m.w.Winner(); w != core.OwnerNeutral {
			if kind != core.WinnerRelic || w != 0 {
				t.Fatalf("unexpected winner %d kind %d at tick %d", w, kind, tick)
			}
			t.Logf("relic victory at tick %d", tick)
			return
		}
	}
	t.Fatal("no victory before deadline")
}

// TestBotsHostedOnServer runs a room whose bot slots are driven by the
// sim brains: a mirror client (sending no orders) must hash-match the
// server every tick, and the log must carry real bot orders.
func TestBotsHostedOnServer(t *testing.T) {
	if testing.Short() {
		t.Skip("long game")
	}
	url, done := newTestHub(t)
	defer done()
	c := dial(t, url, "host")
	c.send(wire.ClientMsg{Type: "create", Name: "Host", Seed: iptr64(0x5eed), Bots: iptr(2)})
	c.expect("welcome", 5*time.Second)
	c.send(wire.ClientMsg{Type: "start", Difficulty: "standard"})
	st := c.expect("start", 5*time.Second)
	if len(st.Players) != 3 {
		t.Fatalf("players = %d, want 3", len(st.Players))
	}
	m := newMirror(*st.Seed, len(st.Players))
	botOrders := 0
	for m.tick < 3000 {
		tk := c.expect("tick", 60*time.Second)
		m.stepMsg(tk)
		good := strconv.FormatUint(m.w.Hash(), 10)
		c.send(wire.ClientMsg{Type: "hash", Tick: tk.Tick, Hash: &good})
		for _, o := range tk.Orders {
			if o.Player > 0 {
				botOrders++
			}
		}
	}
	if botOrders == 0 {
		t.Fatal("no bot orders seen on the wire")
	}
}
