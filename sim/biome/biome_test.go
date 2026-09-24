package biome

import (
	"testing"

	assets "rts"
)

func TestEmbeddedTableValid(t *testing.T) {
	tbl, err := Parse(assets.BiomesJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Defs()) != Count {
		t.Fatalf("defs %d", len(tbl.Defs()))
	}
	for _, d := range tbl.Defs() {
		if d.Key == "" || d.FarmMul < 1 {
			t.Fatalf("bad def %+v", d)
		}
	}
	// Prairie is the breadbasket, taiga the worst farmland (H1 varies).
	if tbl.Get(Prairie).FarmMul <= tbl.Get(Taiga).FarmMul {
		t.Fatal("prairie should out-produce taiga")
	}
	if tbl.Get(Temperate).FarmMul16() != 1<<16 {
		t.Fatalf("temperate should be 1.0x, got %d", int(tbl.Get(Temperate).FarmMul16()))
	}
}

func TestFarmMul16(t *testing.T) {
	d := &Def{FarmMul: 1250}
	if got := d.FarmMul16(); got != 81920 {
		t.Fatalf("1250 milli = %d, want 81920", int(got))
	}
}

func TestParseRejectsDuplicates(t *testing.T) {
	doc := `{"biomes":[{"id":0,"key":"a","farmMul":1000,"tree":0},` +
		`{"id":0,"key":"b","farmMul":1000,"tree":0},` +
		`{"id":1,"key":"c","farmMul":1000,"tree":0},` +
		`{"id":2,"key":"d","farmMul":1000,"tree":0},` +
		`{"id":3,"key":"e","farmMul":1000,"tree":0},` +
		`{"id":4,"key":"f","farmMul":1000,"tree":0}]}`
	if _, err := Parse([]byte(doc)); err == nil {
		t.Fatal("duplicate id accepted")
	}
}
