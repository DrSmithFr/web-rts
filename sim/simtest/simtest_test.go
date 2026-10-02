package simtest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"rts/sim/core"
)

func TestKitSanity(t *testing.T) {
	s := Load(t, "testdata/maps/kitsanity")
	wolf, rabbit := s.U("wolf"), s.U("rabbit")
	if x, y := wolf.Tile(); x != 18 || y != 8 {
		t.Fatalf("wolf tile = %d,%d", x, y)
	}
	if rabbit.HP() <= 0 || rabbit.Dead() {
		t.Fatal("rabbit should start alive")
	}
	if got := wolf.DistTo(rabbit); got != 6 {
		t.Fatalf("dist wolf-rabbit = %d, want 6", got)
	}
	if s.B("fire").HP() <= 0 {
		t.Fatal("campfire should start alive")
	}
	s.During(50).Never(func() bool { return wolf.Dead() }, "wolf dies early")
	s.Within(1).Expect(func() bool { return true }, "immediate")
	s.ApproxEq(int32(s.World().Tick()), 50, 0, "ticks stepped")
}

func TestSnapshotText(t *testing.T) {
	s := Load(t, "testdata/maps/kitsanity")
	got := s.SnapshotText()
	for _, want := range []string{
		"player 0 era=0",
		"building fire tile=(6,6)",
		"unit rabbit state=",
		"unit wolf state=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "rabbit") > strings.Index(got, "wolf") {
		t.Fatal("snapshot rows must be sorted by label")
	}
}

// TestFailOutputBudget re-runs itself with DEMOFAIL=1; the failing
// message must stay within the 15-line budget.
func TestFailOutputBudget(t *testing.T) {
	const demo = "TestKitSanity"
	if os.Getenv("SIMTEST_DEMOFAIL") == "1" {
		s := Load(t, "testdata/maps/kitsanity")
		wolf, rabbit := s.U("wolf"), s.U("rabbit")
		_ = wolf
		s.Within(20).Expect(func() bool { return rabbit.Dead() }, "rabbit flees wolf")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestFailOutputBudget$", "-test.v")
	cmd.Env = append(os.Environ(), "SIMTEST_DEMOFAIL=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("demo run must fail")
	}
	text := string(out)
	_ = demo
	start := strings.Index(text, "FAIL rabbit flees wolf")
	if start < 0 {
		t.Fatalf("failure header missing in:\n%s", text)
	}
	block := strings.Split(text[start:], "\n")
	for i := 0; i < len(block); i++ {
		if strings.HasPrefix(block[i], "--- FAIL") || strings.HasPrefix(block[i], "PASS") {
			block = block[:i]
			break
		}
	}
	if len(block) > 15 {
		t.Fatalf("failure message is %d lines, want <= 15:\n%s", len(block), strings.Join(block, "\n"))
	}
	if !strings.Contains(block[1], "t=") {
		t.Fatalf("sample lines missing:\n%s", strings.Join(block, "\n"))
	}
}

// TestUnitStateNames checks the state renderer covers the core states.
func TestUnitStateNames(t *testing.T) {
	for _, st := range []uint8{
		core.UnitIdle, core.UnitMove, core.UnitGather, core.UnitReturn,
		core.UnitBuild, core.UnitAttack, core.UnitPatrol, core.UnitWander,
		core.UnitFlee, core.UnitSleep,
	} {
		if StateName(st) == "" || strings.HasPrefix(StateName(st), "state") {
			t.Fatalf("StateName(%d) = %q", st, StateName(st))
		}
	}
}
