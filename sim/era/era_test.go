package era

import (
	"strings"
	"testing"
)

func TestEmbeddedLadder(t *testing.T) {
	tab := MustLoad()
	if len(tab.Defs()) != NumEras {
		t.Fatalf("want %d eras", NumEras)
	}
	if tab.Get(0).Key != "prehistory" || tab.Get(LastEra).Key != "modern" {
		t.Fatalf("unexpected ladder keys")
	}
	if tab.Get(FirstEra).CostFood != 0 || tab.Get(FirstEra).AgeTicks != 0 {
		t.Fatal("prehistory must be free and instant")
	}
	for i := uint8(1); i <= LastEra; i++ {
		d := tab.Get(i)
		if d.AgeTicks < 1 || d.ReqBuilding == 0 {
			t.Fatalf("era %d: ageTicks=%d req=%d", i, d.AgeTicks, d.ReqBuilding)
		}
		if d.CostFood <= 0 {
			t.Fatalf("era %d: age-up must cost food", i)
		}
	}
	// total costs must climb with the ladder
	prev := int32(-1)
	for i := uint8(1); i <= LastEra; i++ {
		c := tab.Get(i).CostFood + tab.Get(i).CostGold
		if c <= prev {
			t.Fatalf("era %d: total cost %d does not climb", i, c)
		}
		prev = c
	}
}

func TestParseRejectsBrokenLadders(t *testing.T) {
	cases := map[string]string{
		"missing era":   `{"eras":[{"id":0,"key":"a","ageTicks":0}]}`,
		"out of order":  `{"eras":[{"id":1,"key":"a","ageTicks":1,"reqBuilding":3},{"id":0,"key":"b","ageTicks":0},{}]}`,
		"paid start":    `{"eras":[{"id":0,"key":"a","costFood":5,"ageTicks":0},{}]}`,
		"no req bld":    `{"eras":[{"id":0,"key":"a","ageTicks":0},{"id":1,"key":"b","costFood":1,"ageTicks":10}]}`,
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s: expected parse error", name)
		}
	}
	if _, err := Parse([]byte("nope")); err == nil || !strings.Contains(err.Error(), "era") {
		t.Errorf("bad json: %v", err)
	}
}
