package entity

import "testing"

func TestCreateAliveGet(t *testing.T) {
	s := New[int]()
	id, v := s.Create()
	*v = 7
	if !s.Alive(id) {
		t.Fatal("fresh id not alive")
	}
	if got := s.Get(id); got == nil || *got != 7 {
		t.Fatalf("Get = %v", got)
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d", s.Len())
	}
}

func TestNilId(t *testing.T) {
	s := New[int]()
	if s.Alive(Nil) {
		t.Fatal("Nil alive")
	}
	if s.Get(Nil) != nil {
		t.Fatal("Get(Nil) not nil")
	}
	s.Free(Nil) // must not panic
}

func TestFreeAndStaleId(t *testing.T) {
	s := New[int]()
	a, av := s.Create()
	*av = 1
	s.Free(a)
	if s.Alive(a) {
		t.Fatal("freed id alive")
	}
	if s.Get(a) != nil {
		t.Fatal("Get on freed")
	}
	// Recycle: same slot, new generation.
	b, bv := s.Create()
	if b.Slot() != a.Slot() {
		t.Fatal("slot not recycled")
	}
	if b == a {
		t.Fatal("generation did not bump")
	}
	*bv = 2
	if s.Alive(a) {
		t.Fatal("stale id alive after recycle")
	}
	if got := s.Get(b); got == nil || *got != 2 {
		t.Fatalf("recycled payload = %v", got)
	}
}

func TestEachSlotOrder(t *testing.T) {
	s := New[int]()
	var ids []ID
	for i := 0; i < 5; i++ {
		id, v := s.Create()
		*v = i
		ids = append(ids, id)
	}
	s.Free(ids[2])
	var seen []int
	s.Each(func(id ID, v *int) {
		for i, want := range ids {
			if id == want && i == 2 {
				t.Fatal("freed entity visited")
			}
		}
		seen = append(seen, *v)
	})
	if len(seen) != 4 {
		t.Fatalf("visited %d, want 4", len(seen))
	}
	// Slot order: payload values 0,1,3,4.
	for i, want := range []int{0, 1, 3, 4} {
		if seen[i] != want {
			t.Fatalf("order %d: got %d want %d", i, seen[i], want)
		}
	}
}

func TestZeroedPayload(t *testing.T) {
	type payload struct {
		a int32
		b fixed32
	}
	s := New[payload]()
	id, p := s.Create()
	p.a = 42
	p.b = 99
	s.Free(id)
	_, p2 := s.Create()
	if p2.a != 0 || p2.b != 0 {
		t.Fatalf("recycled payload not zeroed: %+v", p2)
	}
}

type fixed32 int32
