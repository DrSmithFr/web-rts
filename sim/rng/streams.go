package rng

import "encoding/binary"

// Domain identifies an independent random stream. Splitting streams per
// concern keeps unrelated gameplay changes (for example adding one more
// combat roll) from shifting the numbers of other domains.
type Domain uint8

const (
	DomainMapGen Domain = iota
	DomainCombat
	DomainBot
	DomainMisc

	domainCount = 4
)

// Streams holds one generator per domain, all derived deterministically
// from a single world seed.
type Streams struct {
	states [domainCount]PCG32
}

// NewStreams derives every domain stream from the world seed.
func NewStreams(seed uint64) Streams {
	root := Seed(seed, defaultStreamInc)
	var s Streams
	for i := range s.states {
		s.states[i] = Seed(root.NextUint64(), root.NextUint64())
	}
	return s
}

// Next draws one 32-bit value from the given domain stream. It panics on an
// invalid domain, which is a programming error and behaves identically on
// every platform.
func (s *Streams) Next(d Domain) uint32 {
	if d >= domainCount {
		panic("rng: invalid domain")
	}
	return s.states[d].Next()
}

// AppendState appends every generator's full state, little-endian, for
// state hashing.
func (s *Streams) AppendState(buf []byte) []byte {
	var tmp [8]byte
	for _, st := range s.states {
		binary.LittleEndian.PutUint64(tmp[:], st.state)
		buf = append(buf, tmp[:]...)
		binary.LittleEndian.PutUint64(tmp[:], st.inc)
		buf = append(buf, tmp[:]...)
	}
	return buf
}
