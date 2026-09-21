package building

import (
	"strings"
	"testing"
)

const sample = `{
  "buildings": [
    {"id":1,"key":"hut","costWood":50,"costStone":0,"buildTicks":60,"popRoom":5,"size":2,"dropoff":false,"trainFood":0,"trainTicks":0},
    {"id":3,"key":"storehouse","costWood":60,"costStone":0,"buildTicks":80,"popRoom":0,"size":2,"dropoff":true,"trainFood":50,"trainTicks":20}
  ]
}`

func TestParseSample(t *testing.T) {
	tab, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	hut := tab.Get(1)
	if hut == nil || hut.Key != "hut" || hut.PopRoom != 5 {
		t.Fatalf("hut def wrong: %+v", hut)
	}
	sh := tab.Get(3)
	if sh == nil || !sh.Dropoff || !sh.TrainsVillager() {
		t.Fatalf("storehouse def wrong: %+v", sh)
	}
	if tab.Get(99) != nil {
		t.Fatal("unknown id must be nil")
	}
	if len(tab.Defs()) != 2 {
		t.Fatalf("defs len = %d", len(tab.Defs()))
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct{ name, json, want string }{
		{"empty", `{"buildings":[]}`, "empty"},
		{"zero id", `{"buildings":[{"id":0,"key":"x","buildTicks":1,"size":1}]}`, "non-zero"},
		{"dup", `{"buildings":[{"id":1,"key":"a","buildTicks":1,"size":1},{"id":1,"key":"b","buildTicks":1,"size":1}]}`, "duplicate"},
		{"neg cost", `{"buildings":[{"id":1,"key":"a","costWood":-1,"buildTicks":1,"size":1}]}`, "negative"},
		{"ticks", `{"buildings":[{"id":1,"key":"a","buildTicks":0,"size":1}]}`, "buildTicks"},
		{"size", `{"buildings":[{"id":1,"key":"a","buildTicks":1,"size":9}]}`, "size"},
	}
	for _, tt := range tests {
		if _, err := Parse([]byte(tt.json)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got %v, want containing %q", tt.name, err, tt.want)
		}
	}
}

func TestEmbeddedDataValid(t *testing.T) {
	tab := MustLoad()
	for _, id := range []uint16{1, 2, 3} {
		if tab.Get(id) == nil {
			t.Fatalf("embedded table missing id %d", id)
		}
	}
}
