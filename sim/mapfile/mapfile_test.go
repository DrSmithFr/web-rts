package mapfile

import (
	"bytes"
	"encoding/json"
	"regexp"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rts/sim/terrain"
	"rts/sim/worldgen"
)

var terrainObjRE = regexp.MustCompile(`"terrain": \{ "rows": \[[^\]]*\] \}`)

func validJSON() string {
	rows := ""
	for y := 0; y < 16; y++ {
		row := strings.Repeat(".", 24)
		if y == 1 {
			row = strings.Repeat(".", 4) + "FFFF" + strings.Repeat(".", 16)
		}
		if y == 8 {
			row = strings.Repeat(".", 11) + "~~~" + strings.Repeat(".", 10)
		}
		rows += `"` + row + "\",\n"
	}
	rows = strings.TrimSuffix(rows, ",\n")
	return `{
  "format": "rts.map/custom/v1",
  "name": "wolf_hunts_rabbit",
  "width": 24, "height": 16,
  "origin": { "seed": 7 },
  "terrain": { "rows": [
` + rows + `
  ] },
  "height": { "fill": 3 },
  "biome":  { "fill": "temperate" },
  "players": [
    { "spawn": [3, 3], "era": "prehistory", "startKit": "none",
      "resources": { "food": 0, "wood": 0, "stone": 0, "gold": 0, "oil": 0 } }
  ],
  "nodes": [ { "kind": "wood", "tree": "birch", "x": 5, "y": 1, "amount": 100 } ],
  "units": [ { "label": "wolf", "def": "wolf", "owner": "neutral", "x": 18, "y": 8 },
             { "label": "rabbit", "def": "rabbit", "owner": "neutral", "x": 12, "y": 8 } ],
  "buildings": [ { "label": "fire", "def": "campfire", "owner": 0, "x": 6, "y": 6, "complete": true } ],
  "relics": []
}`
}

func TestParseValid(t *testing.T) {
	s, err := Parse([]byte(validJSON()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "wolf_hunts_rabbit" || s.Seed != 7 {
		t.Fatalf("name=%q seed=%d", s.Name, s.Seed)
	}
	g := s.Map.Grid()
	if g.Width() != 24 || g.Height() != 16 {
		t.Fatalf("grid %dx%d", g.Width(), g.Height())
	}
	if g.Vertex(24, 16) != 3 {
		t.Fatalf("vertex fill = %d", g.Vertex(24, 16))
	}
	if g.Tile(4, 1) != terrain.Forest || g.Tile(11, 8) != terrain.Shallow || g.Tile(0, 0) != terrain.Grass {
		t.Fatal("terrain rows decoded wrong")
	}
	if len(s.Units) != 2 || s.Units[0].Label != "wolf" || s.Units[0].Owner != OwnerNeutral {
		t.Fatalf("units %v", s.Units)
	}
	if s.Units[1].DefID != worldgen.DefRabbit {
		t.Fatalf("rabbit def = %d, want %d", s.Units[1].DefID, worldgen.DefRabbit)
	}
	if len(s.Buildings) != 1 || s.Buildings[0].Owner != 0 || !s.Buildings[0].Complete {
		t.Fatalf("buildings %v", s.Buildings)
	}
	for _, r := range s.Map.Relics() {
		if r != [2]int32{-1, -1} {
			t.Fatalf("empty relics list must leave slots absent, got %v", r)
		}
	}
	if len(s.Map.Nodes()) != 1 {
		t.Fatalf("nodes %v", s.Map.Nodes())
	}
}

func TestSaveLoadRoundTripRowsAndPNG(t *testing.T) {
	s, err := Parse([]byte(validJSON()), nil)
	if err != nil {
		t.Fatal(err)
	}
	rowsHash := s.Map.Hash()

	dir := t.TempDir()
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "terrain.png")); err == nil {
		t.Fatal("small map must save rows, not png")
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Map.Hash() != rowsHash {
		t.Fatalf("rows round trip hash %016x, want %016x", got.Map.Hash(), rowsHash)
	}
	if got.Seed != s.Seed || len(got.Units) != 2 || len(got.Buildings) != 1 {
		t.Fatalf("scenario fields lost: %+v", got)
	}

	// PNG round trip through Parse.
	pj, pfiles := pngForm(t, s)
	sp, err := Parse(pj, pfiles)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Map.Hash() != rowsHash {
		t.Fatalf("png round trip hash %016x, want %016x", sp.Map.Hash(), rowsHash)
	}

	// Above 64x64, Save must emit PNG layers and reload identically.
	big := bigScenario()
	before := big.Map.Hash()
	dir2 := t.TempDir()
	if err := Save(dir2, big); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"terrain.png", "height.png", "biome.png"} {
		if _, err := os.Stat(filepath.Join(dir2, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	got2, err := Load(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Map.Hash() != before {
		t.Fatalf("png-mode round trip hash %016x, want %016x", got2.Map.Hash(), before)
	}
}

func bigScenario() *Scenario {
	g := terrain.NewGrid(70, 70)
	for y := 0; y < 70; y++ {
		for x := 0; x < 70; x++ {
			if (x*y)%7 == 0 {
				g.SetTile(x, y, terrain.Forest)
			}
		}
	}
	for y := 0; y <= 70; y++ {
		for x := 0; x <= 70; x++ {
			g.SetVertex(x, y, int32((x+y)%60))
		}
	}
	biomes := make([]uint8, 70*70)
	for i := range biomes {
		biomes[i] = uint8(i % 5)
	}
	spawns := [][2]int{{8, 8}}
	nodes := []worldgen.Node{{X: 9, Y: 9, Kind: 1, Tree: 2, Amount: 60}}
	m := worldgen.FromLayers(g, biomes, spawns, nodes, nil, nil)
	return &Scenario{
		Name: "big", Seed: 1, Map: m,
		Players:   []PlayerStart{{Spawn: [2]int32{8, 8}, Era: 0, StartKit: StartKitNone}},
		Units:     []UnitPlacement{{Label: "u", DefID: 1, Owner: 0, X: 10, Y: 10}},
		Buildings: nil,
	}
}

// pngForm rewrites a scenario as map.json with PNG layers plus the raw
// image bytes.
func pngForm(t *testing.T, s *Scenario) ([]byte, map[string][]byte) {
	t.Helper()
	g := s.Map.Grid()
	w, h := g.Width(), g.Height()
	files := map[string][]byte{
		"terrain.png": grayPNG(t, w, h, tileVals(g)),
		"height.png":  grayPNG(t, w+1, h+1, vertexVals(g)),
		"biome.png":   grayPNG(t, w, h, biomeVals(s.Map)),
	}
	mjson := toFileJSON(s, true)
	raw, err := json.Marshal(&mjson)
	if err != nil {
		t.Fatal(err)
	}
	return raw, files
}

func tileVals(g *terrain.Grid) []uint8 {
	out := make([]uint8, len(g.Tiles()))
	for i, t := range g.Tiles() {
		out[i] = byte(t)
	}
	return out
}

func vertexVals(g *terrain.Grid) []uint8 {
	out := make([]uint8, len(g.Vertices()))
	for i, v := range g.Vertices() {
		out[i] = byte(v)
	}
	return out
}

func biomeVals(m *worldgen.Map) []uint8 { return m.Biomes() }

func grayPNG(t *testing.T, w, h int, vals []uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	i := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{Y: vals[i]})
			i++
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name    string
		patch   func(string) string
		wantErr string
	}{
		{"format", func(s string) string { return strings.Replace(s, `"rts.map/custom/v1"`, `"rts.map/v6"`, 1) },
			`format "rts.map/v6": want "rts.map/custom/v1"`},
		{"width small", func(s string) string { return strings.Replace(s, `"width": 24`, `"width": 15`, 1) },
			`width=15: want 16..2048`},
		{"height big", func(s string) string {
			return strings.Replace(s, `"width": 24, "height": 16`, `"width": 24, "height": 2049`, 1)
		}, `height=2049: want 16..2048`},
		{"two forms", func(s string) string {
			return strings.Replace(s, `"terrain": { "rows": [`, `"terrain": { "fill": "grass", "rows": [`, 1)
		}, `terrain: exactly one of fill, rows, png`},
		{"no form", func(s string) string { return strings.Replace(s, `"biome":  { "fill": "temperate" }`, `"biome": {}`, 1) },
			`biome: exactly one of fill, rows, png`},
		{"terrain rows count", func(s string) string { return strings.Replace(s, `"width": 24, "height": 16`, `"width": 24, "height": 17`, 1) },
			`terrain.rows: 16 rows, want 17`},
		{"terrain row len", func(s string) string {
			return strings.Replace(s, `"....FFFF................"`, `"....FFF................"`, 1)
		}, `terrain.rows[1]: len 23, want 24`},
		{"terrain rune", func(s string) string {
			return strings.Replace(s, `"....FFFF................"`, `"....FXXF................"`, 1)
		}, `unknown terrain rune`},
		{"terrain fill key", func(s string) string {
			return terrainObjRE.ReplaceAllString(s, `"terrain": { "fill": "jungle" }`)
		}, `terrain.fill "jungle": unknown terrain key`},
		{"height fill range", func(s string) string { return strings.Replace(s, `"height": { "fill": 3 }`, `"height": { "fill": 65 }`, 1) },
			`want 0..64`},
		{"height rows count", func(s string) string {
			return strings.Replace(s, `"height": { "fill": 3 }`, `"height": { "rows": ["0"] }`, 1)
		}, `height.rows: 1 rows, want 17`},
		{"height row values", func(s string) string {
			hrows := make([]string, 17)
			hrows[0] = `"` + strings.Repeat("0 ", 24) + `0"`
			for i := 1; i < 17; i++ {
				hrows[i] = `"1 2 3"`
			}
			return strings.Replace(s, `"height": { "fill": 3 }`, `"height": { "rows": [`+strings.Join(hrows, ",")+`] }`, 1)
		}, `height.rows[1]: 3 values, want 25`},
		{"biome id", func(s string) string { return strings.Replace(s, `"biome":  { "fill": "temperate" }`, `"biome":  { "rows": ["9"] }`, 1) },
			`biome.rows: 1 rows, want 16`},
		{"era key", func(s string) string { return strings.Replace(s, `"era": "prehistory"`, `"era": "bronze"`, 1) },
			`era "bronze": unknown era key`},
		{"start kit", func(s string) string { return strings.Replace(s, `"startKit": "none"`, `"startKit": "full"`, 1) },
			`startKit "full"`},
		{"spawn bounds", func(s string) string { return strings.Replace(s, `"spawn": [3, 3]`, `"spawn": [40, 3]`, 1) },
			`players[0].spawn.x=40 out of bounds (width 24)`},
		{"unit bounds", func(s string) string {
			return strings.Replace(s, `"label": "rabbit", "def": "rabbit", "owner": "neutral", "x": 12`, `"label": "rabbit", "def": "rabbit", "owner": "neutral", "x": 40`, 1)
		}, `units[1].x=40 out of bounds (width 24)`},
		{"unknown unit", func(s string) string { return strings.Replace(s, `"def": "wolf"`, `"def": "wolff"`, 1) },
			`units[0].def "wolff": unknown unit key`},
		{"owner string", func(s string) string { return strings.Replace(s, `"owner": "neutral"`, `"owner": "wild"`, 1) },
			`units[0].owner "wild"`},
		{"owner index", func(s string) string { return strings.Replace(s, `"owner": 0,`, `"owner": 5,`, 1) },
			`buildings[0].owner=5: want 0..0 or "neutral"`},
		{"building key", func(s string) string { return strings.Replace(s, `"def": "campfire"`, `"def": "bonfire"`, 1) },
			`buildings[0].def "bonfire": unknown building key`},
		{"node kind", func(s string) string { return strings.Replace(s, `"kind": "wood"`, `"kind": "iron"`, 1) },
			`nodes[0].kind "iron": unknown resource key`},
		{"node tree", func(s string) string { return strings.Replace(s, `"tree": "birch"`, `"tree": "oak"`, 1) },
			`nodes[0].tree "oak": unknown tree key`},
		{"wood needs tree", func(s string) string { return strings.Replace(s, `"kind": "wood", "tree": "birch", `, `"kind": "wood", `, 1) },
			`wood node needs "tree"`},
		{"no players", func(s string) string { return strings.Replace(s, `"players": [`, `"players": [] //`, 1) }, ""},
		{"unknown field", func(s string) string { return strings.Replace(s, `"name": "wolf_hunts_rabbit",`, `"name": "x", "legend": 3,`, 1) },
			`unknown field "legend"`},
		{"missing png", func(s string) string {
			return strings.Replace(s, `"terrain": { "rows"`, `"terrain": { "png": "terrain.png", "rows"`, 1)
		}, `terrain: exactly one of fill, rows, png`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.patch(validJSON())), nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestParsePNGSizeMismatch(t *testing.T) {
	files := map[string][]byte{"terrain.png": grayPNG(t, 8, 8, make([]uint8, 64))}
	m := fileJSON{Format: Format, Width: 16, Height: 16}
	m.Terrain.PNG = strptr("terrain.png")
	m.HeightLay.Fill = int32p(0)
	m.Biome.Fill = strptr("temperate")
	m.Players = []playerJSON{{Spawn: [2]int32{1, 1}, Era: "prehistory", StartKit: StartKitNone}}
	raw, _ := json.Marshal(&m)
	_, err := Parse(raw, files)
	if err == nil || !strings.Contains(err.Error(), "terrain.png: size 8x8, want 16x16") {
		t.Fatalf("got %v", err)
	}
}

func int32p(v int32) *int32 { return &v }
