// Package wire defines the JSON message protocol between clients and the
// authoritative multiplayer server. Message *content* that affects the
// simulation (order payloads) is still the canonical binary format from
// sim/orders, base64-encoded here; JSON only frames the transport and
// lobby plane, never the simulation hot path.
package wire

import "encoding/json"

// Client -> server messages.
type ClientMsg struct {
	Type    string  `json:"t"`
	Name    string  `json:"name,omitempty"`
	Code    string  `json:"code,omitempty"`
	Token   string  `json:"token,omitempty"`
	Seed    *int64  `json:"seed,omitempty"`
	Bots    *int    `json:"bots,omitempty"`
	Kind    *int    `json:"kind,omitempty"`
	Payload string  `json:"payload,omitempty"` // base64
	Tick    *uint32 `json:"tick,omitempty"`
	Hash    *string `json:"hash,omitempty"` // decimal
}

// Server -> client messages.
type ServerMsg struct {
	Type    string       `json:"t"`
	Code    string       `json:"code,omitempty"`
	Player  *int         `json:"player,omitempty"`
	Token   string       `json:"token,omitempty"`
	Seed    *string      `json:"seed,omitempty"` // hex
	Players []PlayerInfo `json:"players,omitempty"`
	Orders  []OrderMsg   `json:"orders,omitempty"`
	Tick    *uint32      `json:"tick,omitempty"`
	Hash    *string      `json:"hash,omitempty"` // hex
	Reason  string       `json:"reason,omitempty"`
	Msg     string       `json:"msg,omitempty"`
	Kind    *int         `json:"kind,omitempty"`
}

// PlayerInfo lists one lobby or game slot.
type PlayerInfo struct {
	Slot   int    `json:"slot"`
	Name   string `json:"name"`
	Color  int    `json:"color"`
	Bot    bool   `json:"bot,omitempty"`
	Online bool   `json:"online"`
}

// OrderMsg is one order inside a per-tick batch.
type OrderMsg struct {
	Tick    uint32 `json:"tick"`
	Player  int    `json:"player"`
	Kind    int    `json:"kind"`
	Payload string `json:"payload,omitempty"` // base64
}

// Decode parses an incoming client message.
func Decode(b []byte) (ClientMsg, error) {
	var m ClientMsg
	err := json.Unmarshal(b, &m)
	return m, err
}

// Encode renders a server message as JSON bytes.
func Encode(m ServerMsg) []byte {
	b, err := json.Marshal(m)
	if err != nil {
		panic("wire: " + err.Error())
	}
	return b
}
