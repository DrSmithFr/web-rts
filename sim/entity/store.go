// Package entity stores simulation entities in fixed slots with stable
// generation-carrying ids. No maps: iteration is always slot-ordered, and
// freeing a slot never shifts another entity's id.
//
// Id layout: generation in the high 16 bits, slot index in the low 16
// bits; id 0 is reserved (invalid).
package entity

// ID addresses one entity slot plus its generation counter.
type ID uint32

// Nil is the zero id, never assigned to a live entity.
const Nil ID = 0

// MaxEntities is the number of concurrent slots.
const MaxEntities = 1 << 16

// Slot returns the storage index of an id.
func (id ID) Slot() int { return int(id & 0xffff) }

// Generation returns the generation tag of an id.
func (id ID) Generation() uint16 { return uint16(id >> 16) }

// Entry is one storage slot; T is the payload type.
type Entry[T any] struct {
	alive bool
	gen   uint16
	Value T
}

// Store allocates slots on demand up to MaxEntities.
type Store[T any] struct {
	slots []Entry[T]
	free  []uint16
}

// New returns an empty store with a sensible initial capacity.
func New[T any]() *Store[T] {
	return &Store[T]{}
}

// Create marks a fresh (or recycled) slot alive with gen bumped and
// returns its id. The value is zeroed before use.
func (s *Store[T]) Create() (ID, *T) {
	var slot int
	if len(s.free) > 0 {
		slot = int(s.free[len(s.free)-1])
		s.free = s.free[:len(s.free)-1]
	} else {
		if len(s.slots) == MaxEntities {
			panic("entity: out of slots")
		}
		slot = len(s.slots)
		s.slots = append(s.slots, Entry[T]{})
	}
	e := &s.slots[slot]
	e.alive = true
	e.gen++
	e.Value = *new(T)
	return ID(uint32(e.gen)<<16 | uint32(slot)), &e.Value
}

// Alive reports whether the id still refers to its slot generation.
func (s *Store[T]) Alive(id ID) bool {
	if id == Nil {
		return false
	}
	slot := id.Slot()
	if slot >= len(s.slots) {
		return false
	}
	e := &s.slots[slot]
	return e.alive && e.gen == id.Generation()
}

// Get returns the payload for a live id, or nil.
func (s *Store[T]) Get(id ID) *T {
	if !s.Alive(id) {
		return nil
	}
	return &s.slots[id.Slot()].Value
}

// Free releases the slot; a stale id is ignored. The slot's generation
// makes old ids fail Alive after reuse.
func (s *Store[T]) Free(id ID) {
	if !s.Alive(id) {
		return
	}
	slot := id.Slot()
	s.slots[slot].alive = false
	s.slots[slot].Value = *new(T)
	s.free = append(s.free, uint16(slot))
}

// Each visits every alive payload in slot order. Removing or creating
// entities during iteration is not allowed.
func (s *Store[T]) Each(fn func(id ID, v *T)) {
	for i := range s.slots {
		if s.slots[i].alive {
			fn(ID(uint32(s.slots[i].gen)<<16|uint32(i)), &s.slots[i].Value)
		}
	}
}

// Len returns the number of live entities.
func (s *Store[T]) Len() int {
	n := 0
	for i := range s.slots {
		if s.slots[i].alive {
			n++
		}
	}
	return n
}
