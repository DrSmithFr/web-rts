package lockstep

import (
	"testing"

	"rts/sim/proto"
)

func ord(player uint8, tick uint32) proto.Order {
	return proto.Order{Tick: tick, Player: player, Kind: proto.KindNoop}
}

func TestDelayConstant(t *testing.T) {
	if Delay != 2 {
		t.Fatalf("Delay = %d, want 2 (spec: T+2)", Delay)
	}
}

func TestDueOrdering(t *testing.T) {
	var q Queue
	q.Add(10, ord(0, 8))
	q.Add(12, ord(1, 10))
	q.Add(10, ord(2, 8)) // same due as first, inserted later
	q.Add(11, ord(3, 9))

	if q.Len() != 4 {
		t.Fatalf("Len = %d, want 4", q.Len())
	}
	got := q.Due(10)
	if len(got) != 2 || got[0].Player != 0 || got[1].Player != 2 {
		t.Fatalf("Due(10) = %+v, want players [0 2] in insertion order", got)
	}
	got = q.Due(11)
	if len(got) != 1 || got[0].Player != 3 {
		t.Fatalf("Due(11) = %+v, want player 3", got)
	}
	if q.Due(11) != nil {
		t.Fatal("expected no orders due twice")
	}
	got = q.Due(99)
	if len(got) != 1 || got[0].Player != 1 {
		t.Fatalf("Due(99) = %+v, want player 1", got)
	}
}

func TestAddSortedInsertion(t *testing.T) {
	var q Queue
	for _, d := range []uint32{50, 10, 30, 20, 40} {
		q.Add(d, ord(uint8(d), d))
	}
	var order []uint32
	for _, d := range []uint32{10, 20, 30, 40, 50} {
		got := q.Due(d)
		if len(got) != 1 || uint32(got[0].Player) != d {
			t.Fatalf("Due(%d) = %+v, want single order for tick %d", d, got, d)
		}
		order = append(order, d)
	}
	if len(order) != 5 {
		t.Fatalf("bad drain: %v", order)
	}
}

func TestFutureNotLeaked(t *testing.T) {
	var q Queue
	q.Add(100, ord(0, 98))
	if got := q.Due(99); got != nil {
		t.Fatalf("Due(99) leaked %+v", got)
	}
}
