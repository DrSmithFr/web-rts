package tree

import (
	"strings"
	"testing"

	assets "rts"
)

func TestEmbeddedTableValid(t *testing.T) {
	tbl, err := Parse(assets.TreesJSON)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range tbl.Defs() {
		if d.Key == "" || d.GatherMul < 1 {
			t.Fatalf("bad def %+v", d)
		}
	}
	if tbl.Get(Spruce).GatherMul != 1000 {
		t.Fatalf("spruce is the 1.0x reference, got %d", tbl.Get(Spruce).GatherMul)
	}
	if tbl.Get(Birch).GatherMul <= tbl.Get(Spruce).GatherMul {
		t.Error("birch should chop faster than spruce")
	}
	if tbl.Get(Baobab).GatherMul >= tbl.Get(Spruce).GatherMul {
		t.Error("baobab should chop slower than spruce")
	}
	if !tbl.Get(Baobab).Sawmill {
		t.Error("baobab must require a sawmill")
	}
	if tbl.Get(Palm).Wood != 0 || tbl.Get(Palm).Food <= 0 {
		t.Error("palm must yield food only")
	}
}

func TestParseRejectsBadRows(t *testing.T) {
	base := `{"trees":[{"id":1,"key":"a","gatherMul":1000,"wood":10,"food":0,"sawmill":false},` +
		`{"id":2,"key":"b","gatherMul":1000,"wood":10,"food":0,"sawmill":false},` +
		`{"id":3,"key":"c","gatherMul":1000,"wood":10,"food":0,"sawmill":false},`
	for _, tc := range []struct{ name, doc string }{
		{"dup", base + `{"id":1,"key":"d","gatherMul":1000,"wood":10,"food":0,"sawmill":false}]}`},
		{"zeroMul", base + `{"id":4,"key":"d","gatherMul":0,"wood":10,"food":0,"sawmill":false}]}`},
		{"noYield", base + `{"id":4,"key":"d","gatherMul":1000,"wood":0,"food":0,"sawmill":false}]}`},
	} {
		if _, err := Parse([]byte(tc.doc)); err == nil {
			t.Errorf("%s accepted", tc.name)
		} else if !strings.Contains(err.Error(), "tree") {
			t.Errorf("%s: error %q lacks package prefix", tc.name, err)
		}
	}
}
