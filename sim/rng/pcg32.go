// Package rng implements the canonical PCG32 (XSH-RR 64/32) pseudorandom
// generator used by every simulation domain.
//
// The implementation follows the reference pcg_basic C code (Melissa
// O'Neill's pcg-c-basic, Apache-2.0) statement for statement so that every
// platform and every build shares one frozen number sequence. The output
// sequence is pinned by testdata/pcg32_vectors.txt; regenerating those
// vectors requires a deliberate review, never a silent refresh.
package rng

const (
	pcgMult          uint64 = 6364136223846793005
	defaultStreamInc uint64 = 1442695040888963407
)

// PCG32 is the state of one generator stream.
type PCG32 struct {
	state uint64
	inc   uint64
}

// Seed builds a generator from an initial state and stream sequence, using
// the reference seeding procedure (two advance steps, state injected
// between them).
func Seed(initState, initSeq uint64) PCG32 {
	p := PCG32{inc: initSeq<<1 | 1}
	p.Next()
	p.state += initState
	p.Next()
	return p
}

// Next returns the next 32-bit output and advances the generator.
func (p *PCG32) Next() uint32 {
	old := p.state
	p.state = old*pcgMult + p.inc
	xs := uint32(((old >> 18) ^ old) >> 27)
	rot := uint(old >> 59)
	return xs>>rot | xs<<((0-rot)&31)
}

// NextUint64 returns 64 bits, high word first. Used to derive child streams.
func (p *PCG32) NextUint64() uint64 {
	return uint64(p.Next())<<32 | uint64(p.Next())
}
