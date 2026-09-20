// Package lockstep schedules orders for future ticks. The server stamps
// every received order with a due tick of `current + Delay`; all peers then
// execute the exact same set of orders at that tick.
//
// Determinism rules: entries keep a stable sorted order (insertion order
// breaks ties) and the queue is a slice, never a Go map.
package lockstep

import (
	"sort"

	"rts/sim/proto"
)

// Delay is the number of ticks between receiving an order and executing
// it. T+2 hides one client round-trip of latency while staying responsive.
const Delay uint32 = 2

type entry struct {
	due uint32
	o   proto.Order
}

// Queue holds orders awaiting their due tick.
type Queue struct {
	items []entry
}

// Add schedules an order at an absolute due tick, keeping the queue sorted
// by due with FIFO order among equal due ticks.
func (q *Queue) Add(due uint32, o proto.Order) {
	i := sort.Search(len(q.items), func(i int) bool { return q.items[i].due > due })
	q.items = append(q.items, entry{})
	copy(q.items[i+1:], q.items[i:])
	q.items[i] = entry{due: due, o: o}
}

// Due removes and returns every order due at or before tick, in due order
// (FIFO per due tick). Returns nil when nothing is due.
func (q *Queue) Due(tick uint32) []proto.Order {
	n := 0
	for n < len(q.items) && q.items[n].due <= tick {
		n++
	}
	if n == 0 {
		return nil
	}
	out := make([]proto.Order, n)
	for i := range n {
		out[i] = q.items[i].o
	}
	q.items = q.items[n:]
	return out
}

// Len returns the number of pending orders.
func (q *Queue) Len() int { return len(q.items) }
