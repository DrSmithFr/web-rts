package units

import (
	"strings"
	"testing"
)

func TestEmbeddedTableLoads(t *testing.T) {
	tbl := MustLoad()
	for _, key := range []string{"villager", "lancer", "archer", "scout", "deer", "boar"} {
		var found *Def
		for i := range tbl.defs {
			if tbl.defs[i].Key == key {
				found = &tbl.defs[i]
			}
		}
		if found == nil {
			t.Fatalf("embedded table missing %s", key)
		}
	}
}

func TestCounterTriangle(t *testing.T) {
	tbl := MustLoad()
	lancer := tbl.Get(2)
	archer := tbl.Get(3)
	scout := tbl.Get(4)
	if lancer == nil || archer == nil || scout == nil {
		t.Fatal("expected ids 2, 3, 4")
	}
	if tbl.BonusFor(archer, lancer.Class) <= 0 {
		t.Error("archer should bonus against lancer")
	}
	if tbl.BonusFor(lancer, scout.Class) <= 0 {
		t.Error("lancer should bonus against scout")
	}
	if tbl.BonusFor(scout, archer.Class) <= 0 {
		t.Error("scout should bonus against archer")
	}
	if tbl.BonusFor(lancer, archer.Class) != 0 {
		t.Error("lancer should not bonus against archer")
	}
}

func TestParseRejectsBadRows(t *testing.T) {
	cases := map[string]string{
		"empty":          `{"units": []}`,
		"zero id":        `{"units": [{"id": 0, "key": "x", "class": 1, "hp": 1, "speed16": 1, "vision": 1, "rate": 1}]}`,
		"duplicate id":   `{"units": [{"id": 1, "key": "a", "class": 1, "hp": 1, "speed16": 1, "vision": 1, "rate": 1}, {"id": 1, "key": "b", "class": 1, "hp": 1, "speed16": 1, "vision": 1, "rate": 1}]}`,
		"bad class":      `{"units": [{"id": 1, "key": "x", "class": 9, "hp": 1, "speed16": 1, "vision": 1, "rate": 1}]}`,
		"damage no type": `{"units": [{"id": 1, "key": "x", "class": 1, "hp": 1, "speed16": 1, "vision": 1, "damage": 5, "rate": 1}]}`,
		"train no bld":   `{"units": [{"id": 1, "key": "x", "class": 1, "hp": 1, "speed16": 1, "vision": 1, "rate": 1, "trainTicks": 5}]}`,
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected parse error", name)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	_, err := Parse([]byte(`{`))
	if err == nil || !strings.Contains(err.Error(), "unit:") {
		t.Fatalf("expected wrapped unit error, got %v", err)
	}
}

func TestClassifyKinds(t *testing.T) {
	animal := &Def{Class: ClassAnimal}
	if animal.Kind() != KindAnimal || animal.Military() || animal.Attacks() {
		t.Error("default animal classification wrong")
	}
	soldier := &Def{Class: ClassArcher, Damage: 5, DamageType: TypePierce}
	if !soldier.Military() || !soldier.Attacks() || soldier.Kind() != KindMilitary {
		t.Error("archer should be attacking military")
	}
}
