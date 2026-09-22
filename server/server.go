package server

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"rts/server/wire"
)

// Fast multiplies the tick rate (functional tests run games accelerated).
var Fast = false

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// Hub owns all rooms and the WebSocket endpoint.
type Hub struct {
	mu        sync.Mutex
	rooms     map[string]*Room
	codesUsed map[string]bool
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{rooms: map[string]*Room{}, codesUsed: map[string]bool{}}
}

func (h *Hub) uniqueCode() string {
	for {
		c := newCode()
		if !h.codesUsed[c] {
			h.codesUsed[c] = true
			return c
		}
	}
}

// Handler serves the WebSocket endpoint: /ws for lobby and game traffic.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.handleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	return mux
}

func (h *Hub) handleWS(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 256)}
	go c.writer()
	go h.readLoop(c, conn)
}

func (c *client) writer() {
	for b := range c.send {
		if c.conn.WriteMessage(websocket.TextMessage, b) != nil {
			return
		}
	}
}

func (h *Hub) readLoop(c *client, conn *websocket.Conn) {
	defer func() {
		h.leave(c)
		c.closeSend()
		conn.Close()
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		msg, err := wire.Decode(data)
		if err != nil {
			c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "bad json"}))
			continue
		}
		h.dispatch(c, msg)
	}
}

func (h *Hub) dispatch(c *client, m wire.ClientMsg) {
	switch m.Type {
	case "create":
		h.create(c, m)
	case "join":
		h.join(c, m)
	case "start":
		h.start(c)
	case "order":
		h.order(c, m)
	case "hash":
		h.hash(c, m)
	case "setbots":
		if m.Bots != nil {
			h.mu.Lock()
			room := h.rooms[strings.ToLower(strings.TrimSpace(m.Code))]
			h.mu.Unlock()
			if room != nil && c.slot == 0 {
				h.SetBots(room.Code, *m.Bots)
			}
		}
	default:
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "unknown type"}))
	}
}

func (h *Hub) create(c *client, m wire.ClientMsg) {
	h.mu.Lock()
	room := newRoom(h, seedOf(m))
	room.Code = h.uniqueCode()
	if m.Bots != nil && *m.Bots >= 0 && *m.Bots <= 7 {
		room.bots = *m.Bots
	}
	h.rooms[room.Code] = room
	h.mu.Unlock()
	room.mu.Lock()
	if room.state == "running" {
		room.mu.Unlock()
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "room in progress"}))
		return
	}
	slot, err := room.joinLocked(nameOf(m), c)
	if err != nil {
		room.mu.Unlock()
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: err.Error()}))
		return
	}
	c.token = newToken()
	wel := wire.ServerMsg{
		Type: "welcome", Code: room.Code, Player: &slot, Token: c.token,
		Players: room.infoLocked(),
	}
	lobby := wire.ServerMsg{Type: "lobby", Code: room.Code, Players: room.infoLocked()}
	room.mu.Unlock()
	c.push(wire.Encode(wel))
	room.broadcastMsg(lobby)
}

func (h *Hub) join(c *client, m wire.ClientMsg) {
	h.mu.Lock()
	room := h.rooms[strings.ToLower(strings.TrimSpace(m.Code))]
	h.mu.Unlock()
	if room == nil {
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "no such room"}))
		return
	}
	room.mu.Lock()
	// reconnect into a reserved slot?
	if m.Token != "" {
		for i := range room.players {
			p := room.players[i]
			if p != nil && p.client != nil && p.client.token == m.Token {
				p.client.abandon()
				p.client = c
				c.slot = i
				p.online = true
				slot := i
				msg := wire.ServerMsg{
					Type: "welcome", Code: room.Code, Player: &slot, Token: m.Token,
					Players: room.infoLocked(),
				}
				if room.state == "running" {
					hexSeed := room.seedHex()
					msg.Seed = &hexSeed
					msg.Tick = &room.tick
					msg.Orders = room.catchupLocked().Orders
				}
				lobby := wire.ServerMsg{Type: "lobby", Code: room.Code, Players: room.infoLocked()}
				room.mu.Unlock()
				c.push(wire.Encode(msg))
				room.broadcastMsg(lobby)
				return
			}
		}
	}
	if room.state == "running" {
		room.mu.Unlock()
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "room in progress"}))
		return
	}
	slot, err := room.joinLocked(nameOf(m), c)
	if err != nil {
		room.mu.Unlock()
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: err.Error()}))
		return
	}
	c.token = newToken()
	msg := wire.ServerMsg{
		Type: "welcome", Code: room.Code, Player: &slot, Token: c.token,
		Players: room.infoLocked(),
	}
	if room.state == "running" {
		hexSeed := room.seedHex()
		msg.Seed = &hexSeed
		msg.Tick = &room.tick
		msg.Orders = room.catchupLocked().Orders
	}
	lobby := wire.ServerMsg{Type: "lobby", Code: room.Code, Players: room.infoLocked()}
	room.mu.Unlock()
	c.push(wire.Encode(msg))
	room.broadcastMsg(lobby)
}

func (h *Hub) start(c *client) {
	room := c.room(h)
	if room == nil {
		return
	}
	room.mu.Lock()
	if c.slot != 0 {
		room.mu.Unlock()
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "only the host starts"}))
		return
	}
	if room.state == "running" {
		room.mu.Unlock()
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "already running"}))
		return
	}
	seed := room.seed
	room.mu.Unlock()
	if err := room.Start(); err != nil {
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: err.Error()}))
		return
	}
	hexSeed := fmt.Sprintf("%x", uint64(seed))
	room.mu.Lock()
	room.broadcastMsg(wire.ServerMsg{Type: "start", Code: room.Code, Seed: &hexSeed,
		Players: room.infoLocked()})
	room.mu.Unlock()
}

func (h *Hub) order(c *client, m wire.ClientMsg) {
	room := c.room(h)
	if room == nil || m.Kind == nil {
		return
	}
	payload, err := base64.StdEncoding.DecodeString(m.Payload)
	if err != nil {
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "bad payload"}))
		return
	}
	room.addOrder(c.slot, uint16(*m.Kind), payload)
}

func (h *Hub) hash(c *client, m wire.ClientMsg) {
	room := c.room(h)
	if room == nil || m.Tick == nil || m.Hash == nil {
		return
	}
	v, err := strconv.ParseUint(*m.Hash, 10, 64)
	if err != nil {
		c.push(wire.Encode(wire.ServerMsg{Type: "error", Msg: "bad hash"}))
		return
	}
	room.reportHash(c.slot, *m.Tick, v)
}

// leave frees lobby slots or marks in-game players as disconnected.
func (h *Hub) leave(c *client) {
	if c.slot < 0 {
		return
	}
	room := c.room(h)
	if room == nil {
		return
	}
	room.mu.Lock()
	p := room.players[c.slot]
	if p == nil || p.client != c {
		room.mu.Unlock()
		return
	}
	if room.state != "running" && p != nil {
		room.players[c.slot] = nil
		room.humans--
	} else if p != nil {
		p.online = false
		if p.client != nil {
			p.client.closeSend()
		}
	}
	info := room.infoLocked()
	code := room.Code
	room.mu.Unlock()
	room.broadcastMsg(wire.ServerMsg{Type: "lobby", Code: code, Players: info})
}

func (c *client) room(h *Hub) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.rooms {
		r.mu.Lock()
		p := r.players[c.slot]
		ok := p != nil && p.client == c
		r.mu.Unlock()
		if ok {
			return r
		}
	}
	return nil
}

// SetBots configures the number of bot slots for a room in lobby phase.
func (h *Hub) SetBots(code string, n int) {
	h.mu.Lock()
	room := h.rooms[code]
	h.mu.Unlock()
	if room == nil {
		return
	}
	room.mu.Lock()
	if room.state == "lobby" {
		room.bots = n
	}
	room.mu.Unlock()
}

func seedOf(m wire.ClientMsg) int64 {
	if m.Seed != nil {
		return *m.Seed
	}
	return 0
}

func nameOf(m wire.ClientMsg) string {
	if m.Name != "" {
		return m.Name
	}
	return "Player"
}
