// Package angle is the integer-only compass shared by the simulation and
// the fog layer: whole degrees, 0 pointing at +Y and growing clockwise.
// The atan table is generated once and hardcoded, so heading maths stays
// bit-identical on every platform (no float, no math library).
package angle

import "rts/sim/fixed"

// FullTurn is one revolution in degrees.
const FullTurn int32 = 360

// atanMilliTable[k] is atan(k/64) in thousandths of a degree, k = 0..64.
// Linear interpolation between neighbours stays below 0.02 degrees.
var atanMilliTable = [65]int32{
	0, 895, 1790, 2684, 3576, 4467, 5356, 6242, 7125,
	8005, 8881, 9752, 10620, 11482, 12339, 13191, 14036, 14876,
	15709, 16535, 17354, 18166, 18970, 19767, 20556, 21337, 22109,
	22874, 23629, 24376, 25115, 25844, 26565, 27277, 27979, 28673,
	29358, 30033, 30700, 31357, 32005, 32645, 33275, 33896, 34509,
	35112, 35707, 36293, 36870, 37439, 37999, 38550, 39094, 39629,
	40156, 40675, 41186, 41689, 42184, 42672, 43152, 43625, 44091,
	44549, 45000,
}

// WrapDeg folds any degree value into [0, 360).
func WrapDeg(deg int32) int32 {
	d := deg % FullTurn
	if d < 0 {
		d += FullTurn
	}
	return d
}

// atanDeg returns atan(num/den) in whole degrees, rounded; num and den
// are non-negative and num <= den with den > 0 (the octant form).
func atanDeg(num, den int32) int32 {
	k := int32((int64(num) * 64) / int64(den))
	if k > 63 {
		k = 63
	}
	rem := int64(num)*64 - int64(k)*int64(den)
	milli := int64(atanMilliTable[k])
	if k < 64 {
		milli += (int64(atanMilliTable[k+1]) - int64(atanMilliTable[k])) * rem / int64(den)
	}
	return int32((milli + 500) / 1000)
}

// HeadingTo returns the compass heading of the vector (dx, dy): degrees
// clockwise from +Y. The zero vector reads as 0.
func HeadingTo(dx, dy fixed.F16) int32 {
	if dx == 0 && dy == 0 {
		return 0
	}
	ax, ay := abs(dx), abs(dy)
	if ax >= ay {
		off := atanDeg(int32(ay), int32(ax))
		switch {
		case dx > 0 && dy >= 0:
			return WrapDeg(90 - off)
		case dx > 0:
			return WrapDeg(90 + off)
		case dy >= 0:
			return WrapDeg(270 + off)
		default:
			return WrapDeg(270 - off)
		}
	}
	off := atanDeg(int32(ax), int32(ay))
	switch {
	case dy > 0 && dx >= 0:
		return WrapDeg(off)
	case dy > 0:
		return WrapDeg(360 - off)
	case dx >= 0:
		return WrapDeg(180 - off)
	default:
		return WrapDeg(180 + off)
	}
}

// TurnToward rotates heading towards target along the short way by at
// most rate degrees (one tick of a species turnRate).
func TurnToward(heading, target, rate int32) int32 {
	diff := WrapDeg(target-heading+180) - 180
	if diff > rate {
		diff = rate
	}
	if diff < -rate {
		diff = -rate
	}
	return WrapDeg(heading + diff)
}

// AngleDiff is the unsigned angle between two headings, 0..180 degrees.
func AngleDiff(a, b int32) int32 {
	d := WrapDeg(a-b+180) - 180
	if d < 0 {
		return -d
	}
	return d
}

// abs is the magnitude of a fixed-point component.
func abs(v fixed.F16) fixed.F16 {
	if v < 0 {
		return -v
	}
	return v
}
