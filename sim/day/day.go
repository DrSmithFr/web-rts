// Package day defines the deterministic day/night cycle as a pure
// function of the simulation tick: no world state, no hash impact.
// The client mirrors these constants (client/src/render/daylight.ts).
package day

import "math"

// DayTicks is one full cycle at 20 Hz: 8 real minutes.
const DayTicks = 9600

// DawnOffset shifts tick 0 into early morning so matches start in
// daylight (phase 0 is midnight).
const DawnOffset = 2880

// Phase returns the position in the current cycle, 0..1, midnight 0.
func Phase(tick uint32) float64 {
	return float64((tick+DawnOffset)%DayTicks) / float64(DayTicks)
}

// SunElevation is the sine of the sun altitude: +1 noon, -1 midnight,
// negative values mean the sun is below the horizon.
func SunElevation(tick uint32) float64 {
	return math.Sin((Phase(tick) - 0.25) * 2 * math.Pi)
}

// IsNight reports whether the sun is at or below the horizon.
func IsNight(tick uint32) bool { return SunElevation(tick) <= 0 }
