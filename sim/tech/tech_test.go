package tech

import (
	"testing"
)

func TestEmbeddedTree(t *testing.T) {
	tab := MustLoad()
	defs := tab.Defs()
	if len(defs) < 10 {
		t.Fatalf("want a full tree, got %d", len(defs))
	}
	for i, d := range defs {
		if d.ID == 0 {
			t.Fatalf("def %d: zero id", i)
		}
		if d.Building == 0 || d.Ticks < 1 || len(d.Effects) == 0 {
			t.Fatalf("def %s: incomplete", d.Key)
		}
		// prerequisites must exist (Parse already enforces ordering)
		for _, r := range d.Requires {
			if tab.Get(r) == nil {
				t.Fatalf("%s: unknown req %d", d.Key, r)
			}
		}
	}
	if tab.Get(1).Key != "forging" {
		t.Fatalf("first tech changed: %s", tab.Get(1).Key)
	}
}

func TestParseRejectsBrokenTrees(t *testing.T) {
	cases := map[string]string{
		"empty": `{"techs":[]}`,
		"forward req": `{"techs":[
			{"id":2,"key":"b","era":1,"building":4,"ticks":10,"effects":[{"field":"dmg","class":2,"value":1}],"requires":[9]},
			{"id":1,"key":"a","era":1,"building":4,"ticks":10,"effects":[{"field":"dmg","class":2,"value":1}]}]}`,
		"bad field": `{"techs":[
			{"id":1,"key":"a","era":1,"building":4,"ticks":10,"effects":[{"field":"lasers","class":2,"value":1}]}]}`,
		"no building": `{"techs":[
			{"id":1,"key":"a","era":1,"building":0,"ticks":10,"effects":[{"field":"dmg","class":2,"value":1}]}]}`,
		"dup id": `{"techs":[
			{"id":1,"key":"a","era":1,"building":4,"ticks":10,"effects":[{"field":"dmg","class":2,"value":1}]},
			{"id":1,"key":"b","era":1,"building":4,"ticks":10,"effects":[{"field":"dmg","class":2,"value":1}]}]}`,
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s: expected parse error", name)
		}
	}
}

func TestModsApply(t *testing.T) {
	var m Mods
	m.Apply(Effect{Field: FieldDmg, Class: 2, Value: 3})
	m.Apply(Effect{Field: FieldDmg, Class: 2, Value: 1})
	m.Apply(Effect{Field: FieldArmorC, Class: 4, Value: 2})
	if m.Dmg[2] != 4 || m.ArmorC[4] != 2 || m.ArmorC[3] != 0 {
		t.Fatalf("mods not folded: %+v", m)
	}
}
