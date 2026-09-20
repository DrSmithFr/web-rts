// Package fixed implements signed 16.16 fixed-point arithmetic on int32.
//
// The simulation uses this type instead of floating point so that every
// platform (native amd64/arm64 and wasm) computes bit-identical results.
//
// Documented edge behaviour, frozen by tests:
//   - Arithmetic wraps around int32 instead of trapping (no saturation).
//   - Mul and MulRound shift with the sign, i.e. results round toward
//     negative infinity on truncation; MulRound adds half an ulp first,
//     rounding halves toward positive infinity.
//   - Div truncates toward zero and returns 0 when the divisor is zero,
//     so a bad division can never abort a lockstep tick.
package fixed

import "fmt"

const (
	fracBits = 16
	fracMask = 1<<fracBits - 1
)

// F16 is a signed fixed-point value with 16 integer bits and 16 fraction
// bits stored in an int32.
type F16 int32

const (
	Zero F16 = 0
	One  F16 = 1 << fracBits
)

// FromInt converts a whole number to F16. Values outside [-32768, 32767]
// wrap around, matching the documented int32 wrap behaviour.
func FromInt(i int32) F16 { return F16(int32(uint32(i) << fracBits)) }

// ToInt truncates toward negative infinity.
func (a F16) ToInt() int32 { return int32(a) >> fracBits }

// Round converts to the nearest whole number, halves toward positive
// infinity.
func (a F16) Round() int32 {
	return int32((int64(a) + 1<<(fracBits-1)) >> fracBits)
}

// Add returns a+b, wrapping on int32 overflow.
func (a F16) Add(b F16) F16 { return a + b }

// Sub returns a-b, wrapping on int32 overflow.
func (a F16) Sub(b F16) F16 { return a - b }

// Neg returns -a, wrapping for the minimum value.
func (a F16) Neg() F16 { return -a }

// Mul returns a*b, rounding toward negative infinity, wrapping on int32
// overflow.
func (a F16) Mul(b F16) F16 {
	return F16(int32((int64(a) * int64(b)) >> fracBits))
}

// MulRound returns a*b rounded to the nearest F16, halves toward positive
// infinity, wrapping on int32 overflow.
func (a F16) MulRound(b F16) F16 {
	return F16(int32((int64(a)*int64(b) + 1<<(fracBits-1)) >> fracBits))
}

// Div returns a/b truncating toward zero. It returns Zero when b is zero;
// the simulation must never panic on a bad divisor.
func (a F16) Div(b F16) F16 {
	if b == 0 {
		return Zero
	}
	return F16(int32((int64(a) << fracBits) / int64(b)))
}

// Min returns the smaller of a and b.
func Min(a, b F16) F16 {
	if a < b {
		return a
	}
	return b
}

// Max returns the larger of a and b.
func Max(a, b F16) F16 {
	if a > b {
		return a
	}
	return b
}

// String renders the value with exactly five decimal digits, rounding the
// last digit half away from zero. It is for debugging only; the wire format
// uses the raw int32.
func (a F16) String() string {
	x := int64(a)
	neg := x < 0
	if neg {
		x = -x
	}
	whole := x >> fracBits
	fracScaled := ((x&fracMask)*100000 + 1<<(fracBits-1)) >> fracBits
	s := fmt.Sprintf("%d.%05d", whole, fracScaled)
	if neg {
		s = "-" + s
	}
	return s
}
