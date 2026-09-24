package day

import (
	"math"
	"testing"
)

func TestPhaseBounded(t *testing.T) {
	for _, tick := range []uint32{0, 1, 959, 4799, 9600, 9601, 1234567} {
		p := Phase(tick)
		if p < 0 || p >= 1 {
			t.Fatalf("phase(%d)=%v out of [0,1)", tick, p)
		}
	}
}

func TestPhasePeriodic(t *testing.T) {
	for _, tick := range []uint32{0, 313, 7777} {
		if math.Abs(Phase(tick)-Phase(tick+DayTicks)) > 1e-12 {
			t.Fatalf("phase not periodic at %d", tick)
		}
	}
}

func TestGameStartsInMorningLight(t *testing.T) {
	if SunElevation(0) <= 0 {
		t.Fatalf("tick 0 should be daylight, elevation %v", SunElevation(0))
	}
}

func TestSunArc(t *testing.T) {
	// noon (phase 0.5) needs tick where phase lands exactly 0.5:
	// (tick+2880) % 9600 == 4800 -> tick 1920
	if math.Abs(SunElevation(1920)-1) > 1e-9 {
		t.Fatalf("tick 1920 should be solar noon, got %v", SunElevation(1920))
	}
	// midnight: phase 0 -> tick 6720
	if math.Abs(SunElevation(6720)+1) > 1e-9 {
		t.Fatalf("tick 6720 should be midnight, got %v", SunElevation(6720))
	}
	if !IsNight(6720) {
		t.Fatal("midnight should be night")
	}
	if IsNight(1920) {
		t.Fatal("noon should not be night")
	}
}
