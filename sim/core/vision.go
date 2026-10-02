package core

import (
	"rts/sim/fixed"
)

// sees reports whether a point sits inside a unit's sight: inside the
// unit's vision radius and inside the vision arc centred on its heading.
// A sleeping unit sees nothing, which is why every detection in the sim
// (fear, aggro, chase, attack acquisition) goes through here instead of
// a bare radius test.
func (w *World) sees(u *Unit, x, y fixed.F16) bool {
	return w.inSight(u, int32(w.visionOf(u)), x, y)
}

// inSight is sees with an explicit radius, so a caller that works at a
// shorter range (a flee radius, an aggro leash) keeps its own limit and
// still respects the arc.
func (w *World) inSight(u *Unit, radius int32, x, y fixed.F16) bool {
	if u.State == UnitSleep || radius < 1 {
		return false
	}
	d2 := dist2(u.X, u.Y, x, y)
	l := int64(radius) << 16
	if d2 > l*l {
		return false
	}
	arc := w.visionArcOf(u)
	if arc >= 360 || (u.X == x && u.Y == y) {
		return true
	}
	if d2 <= ear2() {
		return true // close enough to hear it, arc or not
	}
	return angleDiff(u.Heading, headingTo(x.Sub(u.X), y.Sub(u.Y))) <= arc/2
}

// EarRadius is how close something has to be (in tiles) to be noticed
// outside the vision arc: an animal does not need eyes behind its head to
// feel a footstep right next to it.
const EarRadius int32 = 2

func ear2() int64 { return int64(EarRadius) * int64(EarRadius) << 32 }

// visionArcOf is the field of view of a unit, in degrees.
func (w *World) visionArcOf(u *Unit) int32 {
	if def := w.udefs.Get(u.DefID); def != nil {
		return def.VisionArc
	}
	return 360
}
