// Package mapfile reads and writes custom map folders in the
// "rts.map/custom/v1" format: one directory holding map.json plus
// optional terrain.png, height.png and biome.png layers.
//
// Parsing validates every field and quotes the offending path, index
// and value on failure (e.g. `units[2].x=40 out of bounds (width 24)`).
// Keys in the file are data/*.json keys, never numeric ids. map.json
// carries the map height twice: the scalar "height" and the height
// layer object, so both codecs are hand-rolled around the token stream.
// Saved maps use ASCII rows up to 64x64 tiles and PNG layers above.
package mapfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rts/sim/biome"
	"rts/sim/building"
	"rts/sim/era"
	"rts/sim/resource"
	"rts/sim/terrain"
	"rts/sim/tree"
	"rts/sim/units"
	"rts/sim/worldgen"
)

// Format is the only map.json format string this package accepts.
const Format = "rts.map/custom/v1"

// OwnerNeutral is the owner byte for wildlife and other neutral entities
// (mirrors core.OwnerNeutral; kept local to avoid importing core).
const OwnerNeutral uint8 = 0xFF

// Start kits for player starts: "default" is the procedural starting kit
// (storehouse plus three villagers), "none" starts bare.
const (
	StartKitDefault = "default"
	StartKitNone    = "none"
)

// PlayerStart is one custom player slot.
type PlayerStart struct {
	Spawn    [2]int32
	Era      uint8
	StartKit string
	Res      [resource.Count]int32
}

// UnitPlacement is one unit standing on the map at load.
type UnitPlacement struct {
	Label string
	DefID uint16
	Owner uint8 // player index or OwnerNeutral
	X, Y  int32
}

// BuildingPlacement is one building standing on the map at load.
type BuildingPlacement struct {
	Label    string
	DefID    uint16
	Owner    uint8 // player index or OwnerNeutral
	X, Y     int32
	Complete bool
}

// Scenario is a fully decoded custom map, ready for core setup.
type Scenario struct {
	Name      string
	Seed      uint64
	Map       *worldgen.Map
	Players   []PlayerStart
	Units     []UnitPlacement
	Buildings []BuildingPlacement
}

// Load reads dir/map.json plus every PNG layer it references.
func Load(dir string) (*Scenario, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "map.json"))
	if err != nil {
		return nil, fmt.Errorf("mapfile: %w", err)
	}
	files := map[string][]byte{}
	var probe struct {
		Terrain struct {
			PNG string `json:"png"`
		} `json:"terrain"`
		Biome struct {
			PNG string `json:"png"`
		} `json:"biome"`
	}
	if json.Unmarshal(raw, &probe) == nil {
		for _, name := range []string{probe.Terrain.PNG, probe.Biome.PNG} {
			if name == "" {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, fmt.Errorf("mapfile: %w", err)
			}
			files[name] = b
		}
	}
	// The height layer lives under the same "height" key as the scalar
	// map height; re-scan the tokens for an object value.
	if hp := heightLayerPNG(raw); hp != "" {
		b, err := os.ReadFile(filepath.Join(dir, hp))
		if err != nil {
			return nil, fmt.Errorf("mapfile: %w", err)
		}
		files[hp] = b
	}
	return Parse(raw, files)
}

// heightLayerPNG scans the token stream for a "height" key whose value
// is an object, and returns its "png" file name (empty when absent).
func heightLayerPNG(raw []byte) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return ""
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return ""
		}
		key, _ := t.(string)
		var val json.RawMessage
		if dec.Decode(&val) != nil {
			return ""
		}
		if key == "height" && isObject(val) {
			var l struct {
				PNG string `json:"png"`
			}
			if json.Unmarshal(val, &l) == nil {
				return l.PNG
			}
		}
	}
	return ""
}

func isObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{'
}

// ---- JSON shape ------------------------------------------------------

type terrainLayerJSON struct {
	Fill *string  `json:"fill"`
	Rows []string `json:"rows"`
	PNG  *string  `json:"png"`
}

type heightLayerJSON struct {
	Fill *int32   `json:"fill"`
	Rows []string `json:"rows"`
	PNG  *string  `json:"png"`
}

type biomeLayerJSON struct {
	Fill *string  `json:"fill"`
	Rows []string `json:"rows"`
	PNG  *string  `json:"png"`
}

type resourcesJSON struct {
	Food  *int32 `json:"food"`
	Wood  *int32 `json:"wood"`
	Stone *int32 `json:"stone"`
	Gold  *int32 `json:"gold"`
	Oil   *int32 `json:"oil"`
}

type playerJSON struct {
	Spawn     [2]int32      `json:"spawn"`
	Era       string        `json:"era"`
	StartKit  string        `json:"startKit"`
	Resources resourcesJSON `json:"resources"`
}

type nodeJSON struct {
	Kind   string `json:"kind"`
	Tree   string `json:"tree"`
	X      int32  `json:"x"`
	Y      int32  `json:"y"`
	Amount *int32 `json:"amount"`
}

type unitJSON struct {
	Label string          `json:"label"`
	Def   string          `json:"def"`
	Owner json.RawMessage `json:"owner"`
	X     int32           `json:"x"`
	Y     int32           `json:"y"`
}

type buildingJSON struct {
	Label    string          `json:"label"`
	Def      string          `json:"def"`
	Owner    json.RawMessage `json:"owner"`
	X        int32           `json:"x"`
	Y        int32           `json:"y"`
	Complete bool            `json:"complete"`
}

type fileJSON struct {
	Format    string
	Name      string
	Width     int
	Height    int
	Seed      uint64
	Terrain   terrainLayerJSON
	HeightLay heightLayerJSON
	Biome     biomeLayerJSON
	Players   []playerJSON
	Nodes     []nodeJSON
	Units     []unitJSON
	Buildings []buildingJSON
	Relics    *[][2]int32
}

// UnmarshalJSON walks the object token by token so the "height" key can
// carry both the scalar map height and the height layer object, and so
// unknown fields are reported by name.
func (m *fileJSON) UnmarshalJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return errors.New("want a JSON object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := t.(string)
		switch key {
		case "format":
			err = dec.Decode(&m.Format)
		case "name":
			err = dec.Decode(&m.Name)
		case "width":
			err = dec.Decode(&m.Width)
		case "height":
			var v json.RawMessage
			if err = dec.Decode(&v); err == nil {
				if isObject(v) {
					err = decodeStrict(v, &m.HeightLay)
				} else {
					err = json.Unmarshal(v, &m.Height)
				}
			}
		case "origin":
			var o struct {
				Seed uint64 `json:"seed"`
			}
			err = decodeStrict2(dec, &o)
			m.Seed = o.Seed
		case "terrain":
			err = dec.Decode(&m.Terrain)
		case "biome":
			err = dec.Decode(&m.Biome)
		case "players":
			err = dec.Decode(&m.Players)
		case "nodes":
			err = dec.Decode(&m.Nodes)
		case "units":
			err = dec.Decode(&m.Units)
		case "buildings":
			err = dec.Decode(&m.Buildings)
		case "relics":
			err = dec.Decode(&m.Relics)
		default:
			return fmt.Errorf("unknown field %q", key)
		}
		if err != nil {
			return fmt.Errorf("map.json %s: %v", key, err)
		}
	}
	return nil
}

// decodeStrict2 decodes the next decoder value with unknown fields
// rejected.
func decodeStrict2(dec *json.Decoder, target any) error {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	return decodeStrict(raw, target)
}

// decodeStrict decodes raw JSON with unknown fields rejected.
func decodeStrict(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

// MarshalJSON writes the object with both "height" members in order.
func (m *fileJSON) MarshalJSON() ([]byte, error) {
	var sb strings.Builder
	sb.WriteByte('{')
	put := func(key string, v any) error {
		if key != "" {
			sb.WriteString(strconv.Quote(key))
			sb.WriteByte(':')
		}
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		sb.Write(b)
		return nil
	}
	fields := []func() error{
		func() error { return put("format", m.Format) },
		func() error { return put("name", m.Name) },
		func() error { return put("width", m.Width) },
		func() error { return put("height", m.Height) },
		func() error { return put("origin", map[string]uint64{"seed": m.Seed}) },
		func() error { return put("terrain", m.Terrain) },
		func() error { return put("height", m.HeightLay) },
		func() error { return put("biome", m.Biome) },
		func() error { return put("players", m.Players) },
		func() error {
			if m.Nodes == nil {
				return put("nodes", []nodeJSON{})
			}
			return put("nodes", m.Nodes)
		},
		func() error {
			if m.Units == nil {
				return put("units", []unitJSON{})
			}
			return put("units", m.Units)
		},
		func() error {
			if m.Buildings == nil {
				return put("buildings", []buildingJSON{})
			}
			return put("buildings", m.Buildings)
		},
		func() error { return put("relics", m.Relics) },
	}
	for i, f := range fields {
		if i > 0 {
			sb.WriteByte(',')
		}
		if err := f(); err != nil {
			return nil, err
		}
	}
	sb.WriteByte('}')
	return []byte(sb.String()), nil
}

// ---- Key tables ------------------------------------------------------

func unitKeys() map[string]uint16 {
	m := make(map[string]uint16)
	for _, d := range units.MustLoad().Defs() {
		m[d.Key] = d.ID
	}
	return m
}

func buildingKeys() map[string]uint16 {
	m := make(map[string]uint16)
	for _, d := range building.MustLoad().Defs() {
		m[d.Key] = d.ID
	}
	return m
}

func eraKeys() map[string]uint8 {
	m := make(map[string]uint8)
	for _, d := range era.MustLoad().Defs() {
		m[d.Key] = d.ID
	}
	return m
}

func biomeKeys() map[string]uint8 {
	m := make(map[string]uint8)
	for _, d := range biome.MustLoad().Defs() {
		m[d.Key] = uint8(d.ID)
	}
	return m
}

func treeKeys() map[string]uint8 {
	m := make(map[string]uint8)
	for _, d := range tree.MustLoad().Defs() {
		m[d.Key] = uint8(d.ID)
	}
	return m
}

func resourceKeys() map[string]resource.Kind {
	return map[string]resource.Kind{
		"wood":  resource.Wood,
		"stone": resource.Stone,
		"food":  resource.Food,
		"gold":  resource.Gold,
		"oil":   resource.Oil,
	}
}

var terrainKeys = map[string]terrain.Type{
	"grass":   terrain.Grass,
	"forest":  terrain.Forest,
	"rock":    terrain.Rock,
	"sand":    terrain.Sand,
	"shallow": terrain.Shallow,
	"deep":    terrain.Deep,
}

var terrainRunes = map[terrain.Type]rune{
	terrain.Grass:   '.',
	terrain.Forest:  'F',
	terrain.Rock:    'R',
	terrain.Sand:    's',
	terrain.Shallow: '~',
	terrain.Deep:    'W',
}

// TerrainRune renders a terrain type in the map's ASCII layer charset.
func TerrainRune(t terrain.Type) rune { return terrainRunes[t] }

// ---- Parse -----------------------------------------------------------

// defaultNodeAmount fills in the standard capacity for a node written
// without an explicit "amount", matching the procedural generator.
func defaultNodeAmount(kind resource.Kind, sp uint8) int32 {
	switch kind {
	case resource.Wood:
		if d := tree.MustLoad().Get(sp); d != nil {
			return d.Wood
		}
		return resource.TreeAmount
	case resource.Food:
		if d := tree.MustLoad().Get(sp); d != nil {
			return d.Food
		}
		return resource.BerryAmount
	case resource.Stone:
		return resource.StoneAmount
	case resource.Gold:
		return resource.GoldAmount
	case resource.Oil:
		return resource.OilAmount
	}
	return resource.TreeAmount
}

// Parse decodes map.json plus the raw bytes of every PNG it references
// (keyed by file name). Every validation error quotes the field path,
// index and offending value.
func Parse(mapJSON []byte, files map[string][]byte) (*Scenario, error) {
	var m fileJSON
	if err := decodeStrict(mapJSON, &m); err != nil {
		return nil, fmt.Errorf("mapfile: %v", err)
	}
	if m.Format != Format {
		return nil, fmt.Errorf("mapfile: format %q: want %q", m.Format, Format)
	}
	if m.Width < 16 || m.Width > 2048 {
		return nil, fmt.Errorf("mapfile: width=%d: want 16..2048", m.Width)
	}
	if m.Height < 16 || m.Height > 2048 {
		return nil, fmt.Errorf("mapfile: height=%d: want 16..2048", m.Height)
	}
	if len(m.Players) == 0 {
		return nil, errors.New("mapfile: players: need at least one")
	}
	if len(m.Players) > 8 {
		return nil, fmt.Errorf("mapfile: players=%d: want at most 8", len(m.Players))
	}

	grid := terrain.NewGrid(m.Width, m.Height)
	if err := decodeTerrain(grid, m.Terrain, m.Width, m.Height, files); err != nil {
		return nil, err
	}
	if err := decodeHeightLayer(grid, m.HeightLay, m.Width, m.Height, files); err != nil {
		return nil, err
	}
	biomes, err := decodeBiomes(m.Biome, m.Width, m.Height, files)
	if err != nil {
		return nil, err
	}

	uks := unitKeys()
	bks := buildingKeys()
	eks := eraKeys()
	rks := resourceKeys()
	tks := treeKeys()

	var nodes []worldgen.Node
	for i, n := range m.Nodes {
		kind, ok := rks[n.Kind]
		if !ok {
			return nil, fmt.Errorf("mapfile: nodes[%d].kind %q: unknown resource key", i, n.Kind)
		}
		if err := boundsTile(fmt.Sprintf("nodes[%d]", i), n.X, n.Y, m.Width, m.Height); err != nil {
			return nil, err
		}
		if n.Amount != nil && *n.Amount < 0 {
			return nil, fmt.Errorf("mapfile: nodes[%d].amount=%d: want >= 0", i, *n.Amount)
		}
		var sp uint8
		if n.Tree != "" {
			id, ok := tks[n.Tree]
			if !ok {
				return nil, fmt.Errorf("mapfile: nodes[%d].tree %q: unknown tree key", i, n.Tree)
			}
			sp = id
		} else if kind == resource.Wood {
			return nil, fmt.Errorf("mapfile: nodes[%d]: wood node needs \"tree\"", i)
		} else if kind != resource.Food {
			return nil, fmt.Errorf("mapfile: nodes[%d]: %s node must not carry \"tree\"", i, kind)
		}
		amount := int32(0)
		if n.Amount != nil {
			amount = *n.Amount
		}
		if amount == 0 {
			amount = defaultNodeAmount(kind, sp)
		}
		nodes = append(nodes, worldgen.Node{X: n.X, Y: n.Y, Kind: kind, Tree: sp, Amount: amount})
	}

	var units []UnitPlacement
	for i, u := range m.Units {
		def, ok := uks[u.Def]
		if !ok {
			return nil, fmt.Errorf("mapfile: units[%d].def %q: unknown unit key", i, u.Def)
		}
		owner, err := parseOwner(fmt.Sprintf("units[%d].owner", i), u.Owner, len(m.Players))
		if err != nil {
			return nil, err
		}
		if err := boundsTile(fmt.Sprintf("units[%d]", i), u.X, u.Y, m.Width, m.Height); err != nil {
			return nil, err
		}
		if u.Label == "" {
			return nil, fmt.Errorf("mapfile: units[%d].label: empty", i)
		}
		units = append(units, UnitPlacement{Label: u.Label, DefID: def, Owner: owner, X: u.X, Y: u.Y})
	}

	var buildings []BuildingPlacement
	for i, b := range m.Buildings {
		def, ok := bks[b.Def]
		if !ok {
			return nil, fmt.Errorf("mapfile: buildings[%d].def %q: unknown building key", i, b.Def)
		}
		owner, err := parseOwner(fmt.Sprintf("buildings[%d].owner", i), b.Owner, len(m.Players))
		if err != nil {
			return nil, err
		}
		if err := boundsTile(fmt.Sprintf("buildings[%d]", i), b.X, b.Y, m.Width, m.Height); err != nil {
			return nil, err
		}
		if b.Label == "" {
			return nil, fmt.Errorf("mapfile: buildings[%d].label: empty", i)
		}
		buildings = append(buildings, BuildingPlacement{
			Label: b.Label, DefID: def, Owner: owner, X: b.X, Y: b.Y, Complete: b.Complete,
		})
	}

	var players []PlayerStart
	spawns := make([][2]int, 0, len(m.Players))
	for i, p := range m.Players {
		er, ok := eks[p.Era]
		if !ok {
			return nil, fmt.Errorf("mapfile: players[%d].era %q: unknown era key", i, p.Era)
		}
		if p.StartKit != StartKitDefault && p.StartKit != StartKitNone {
			return nil, fmt.Errorf("mapfile: players[%d].startKit %q: want %q or %q",
				i, p.StartKit, StartKitDefault, StartKitNone)
		}
		if err := boundsTile(fmt.Sprintf("players[%d].spawn", i), p.Spawn[0], p.Spawn[1], m.Width, m.Height); err != nil {
			return nil, err
		}
		var res [resource.Count]int32
		for kind, v := range map[resource.Kind]*int32{
			resource.Food: p.Resources.Food, resource.Wood: p.Resources.Wood,
			resource.Stone: p.Resources.Stone, resource.Gold: p.Resources.Gold,
			resource.Oil: p.Resources.Oil,
		} {
			if v != nil {
				if *v < 0 {
					return nil, fmt.Errorf("mapfile: players[%d].resources.%s=%d: want >= 0", i, kind, *v)
				}
				res[kind] = *v
			}
		}
		players = append(players, PlayerStart{Spawn: p.Spawn, Era: er, StartKit: p.StartKit, Res: res})
		spawns = append(spawns, [2]int{int(p.Spawn[0]), int(p.Spawn[1])})
	}

	var relics [][2]int32
	if m.Relics != nil {
		relics = make([][2]int32, 0, len(*m.Relics))
		for i, r := range *m.Relics {
			if r[0] == -1 && r[1] == -1 {
				relics = append(relics, r)
				continue
			}
			if err := boundsTile(fmt.Sprintf("relics[%d]", i), r[0], r[1], m.Width, m.Height); err != nil {
				return nil, err
			}
			relics = append(relics, r)
		}
	}

	return &Scenario{
		Name:      m.Name,
		Seed:      m.Seed,
		Map:       worldgen.FromLayers(grid, biomes, spawns, nodes, nil, relics),
		Players:   players,
		Units:     units,
		Buildings: buildings,
	}, nil
}

func boundsTile(what string, x, y int32, w, h int) error {
	if x < 0 || x >= int32(w) {
		return fmt.Errorf("mapfile: %s.x=%d out of bounds (width %d)", what, x, w)
	}
	if y < 0 || y >= int32(h) {
		return fmt.Errorf("mapfile: %s.y=%d out of bounds (height %d)", what, y, h)
	}
	return nil
}

func parseOwner(what string, raw json.RawMessage, players int) (uint8, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("mapfile: %s: missing", what)
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s != "neutral" {
			return 0, fmt.Errorf("mapfile: %s %q: want a player index or \"neutral\"", what, s)
		}
		return OwnerNeutral, nil
	}
	var n int
	if json.Unmarshal(raw, &n) != nil {
		return 0, fmt.Errorf("mapfile: %s: want a player index or \"neutral\"", what)
	}
	if n < 0 || n >= players {
		return 0, fmt.Errorf("mapfile: %s=%d: want 0..%d or \"neutral\"", what, n, players-1)
	}
	return uint8(n), nil
}

// ---- Layer decoding --------------------------------------------------

func layerFormCount(fillSet, rowsSet, pngSet bool) int {
	n := 0
	for _, set := range []bool{fillSet, rowsSet, pngSet} {
		if set {
			n++
		}
	}
	return n
}

func decodeTerrain(g *terrain.Grid, l terrainLayerJSON, w, h int, files map[string][]byte) error {
	if layerFormCount(l.Fill != nil, l.Rows != nil, l.PNG != nil) != 1 {
		return errors.New("mapfile: terrain: exactly one of fill, rows, png")
	}
	switch {
	case l.Fill != nil:
		t, ok := terrainKeys[*l.Fill]
		if !ok {
			return fmt.Errorf("mapfile: terrain.fill %q: unknown terrain key", *l.Fill)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				g.SetTile(x, y, t)
			}
		}
	case l.Rows != nil:
		if len(l.Rows) != h {
			return fmt.Errorf("mapfile: terrain.rows: %d rows, want %d", len(l.Rows), h)
		}
		for y, row := range l.Rows {
			rs := []rune(row)
			if len(rs) != w {
				return fmt.Errorf("mapfile: terrain.rows[%d]: len %d, want %d", y, len(rs), w)
			}
			for x, r := range rs {
				t, ok := runeTerrain(r)
				if !ok {
					return fmt.Errorf("mapfile: terrain.rows[%d][%d] %q: unknown terrain rune", y, x, r)
				}
				g.SetTile(x, y, t)
			}
		}
	case l.PNG != nil:
		pix, err := decodeGray(*l.PNG, w, h, files)
		if err != nil {
			return fmt.Errorf("mapfile: terrain.png: %v", err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := pix[y*w+x]
				if v >= byte(terrain.TypeCount) {
					return fmt.Errorf("mapfile: terrain.png pixel (%d,%d)=%d: unknown terrain id", x, y, v)
				}
				g.SetTile(x, y, terrain.Type(v))
			}
		}
	}
	return nil
}

func runeTerrain(r rune) (terrain.Type, bool) {
	for t, c := range terrainRunes {
		if r == c {
			return t, true
		}
	}
	return 0, false
}

func decodeHeightLayer(g *terrain.Grid, l heightLayerJSON, w, h int, files map[string][]byte) error {
	if layerFormCount(l.Fill != nil, l.Rows != nil, l.PNG != nil) != 1 {
		return errors.New("mapfile: height: exactly one of fill, rows, png")
	}
	vw := w + 1
	setH := func(x, y int, v int32) error {
		if v < 0 || v > terrain.MaxHeight {
			return fmt.Errorf("mapfile: height[%d,%d]=%d: want 0..%d", x, y, v, terrain.MaxHeight)
		}
		g.SetVertex(x, y, v)
		return nil
	}
	switch {
	case l.Fill != nil:
		for y := 0; y <= h; y++ {
			for x := 0; x <= w; x++ {
				if err := setH(x, y, *l.Fill); err != nil {
					return err
				}
			}
		}
	case l.Rows != nil:
		if len(l.Rows) != h+1 {
			return fmt.Errorf("mapfile: height.rows: %d rows, want %d", len(l.Rows), h+1)
		}
		for y, row := range l.Rows {
			fs := strings.Fields(row)
			if len(fs) != vw {
				return fmt.Errorf("mapfile: height.rows[%d]: %d values, want %d", y, len(fs), vw)
			}
			for x, f := range fs {
				v, err := strconv.Atoi(f)
				if err != nil {
					return fmt.Errorf("mapfile: height.rows[%d][%d] %q: not an integer", y, x, f)
				}
				if err := setH(x, y, int32(v)); err != nil {
					return err
				}
			}
		}
	case l.PNG != nil:
		pix, err := decodeGray(*l.PNG, vw, h+1, files)
		if err != nil {
			return fmt.Errorf("mapfile: height.png: %v", err)
		}
		for y := 0; y <= h; y++ {
			for x := 0; x <= w; x++ {
				if err := setH(x, y, int32(pix[y*vw+x])); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func decodeBiomes(l biomeLayerJSON, w, h int, files map[string][]byte) ([]uint8, error) {
	if layerFormCount(l.Fill != nil, l.Rows != nil, l.PNG != nil) != 1 {
		return nil, errors.New("mapfile: biome: exactly one of fill, rows, png")
	}
	out := make([]uint8, w*h)
	switch {
	case l.Fill != nil:
		id, ok := biomeKeys()[*l.Fill]
		if !ok {
			return nil, fmt.Errorf("mapfile: biome.fill %q: unknown biome key", *l.Fill)
		}
		for i := range out {
			out[i] = id
		}
	case l.Rows != nil:
		if len(l.Rows) != h {
			return nil, fmt.Errorf("mapfile: biome.rows: %d rows, want %d", len(l.Rows), h)
		}
		for y, row := range l.Rows {
			rs := []rune(row)
			if len(rs) != w {
				return nil, fmt.Errorf("mapfile: biome.rows[%d]: len %d, want %d", y, len(rs), w)
			}
			for x, r := range rs {
				if r < '0' || r > '9' {
					return nil, fmt.Errorf("mapfile: biome.rows[%d][%d] %q: want a digit 0..9", y, x, r)
				}
				id := uint8(r - '0')
				if id >= biome.Count {
					return nil, fmt.Errorf("mapfile: biome.rows[%d][%d] %d: unknown biome id", y, x, id)
				}
				out[y*w+x] = id
			}
		}
	case l.PNG != nil:
		pix, err := decodeGray(*l.PNG, w, h, files)
		if err != nil {
			return nil, fmt.Errorf("mapfile: biome.png: %v", err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := pix[y*w+x]
				if v >= biome.Count {
					return nil, fmt.Errorf("mapfile: biome.png pixel (%d,%d)=%d: unknown biome id", x, y, v)
				}
				out[y*w+x] = v
			}
		}
	}
	return out, nil
}

// decodeGray decodes a named file from files as an 8-bit gray image of
// exactly w x h pixels, returned row-major.
func decodeGray(name string, w, h int, files map[string][]byte) ([]byte, error) {
	raw, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("%s: not provided", name)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	if b := img.Bounds(); b.Dx() != w || b.Dy() != h {
		return nil, fmt.Errorf("%s: size %dx%d, want %dx%d", name, b.Dx(), b.Dy(), w, h)
	}
	out := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			switch c := img.At(x, y).(type) {
			case color.Gray:
				out[y*w+x] = c.Y
			case color.Gray16:
				out[y*w+x] = uint8(c.Y >> 8)
			default:
				_, g, _, _ := c.RGBA()
				out[y*w+x] = uint8(g >> 8)
			}
		}
	}
	return out, nil
}

// ---- Save ------------------------------------------------------------

// Save writes dir/map.json plus PNG layers when the map is larger than
// 64x64 tiles (layers ride in rows below that). The written folder
// re-Parses to the same Map hash.
func Save(dir string, s *Scenario) error {
	if s == nil || s.Map == nil {
		return errors.New("mapfile: save nil scenario")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mapfile: %w", err)
	}
	g := s.Map.Grid()
	w, h := g.Width(), g.Height()
	pngLayers := w > 64 || h > 64
	m := toFileJSON(s, pngLayers)
	if pngLayers {
		if err := writePNG(filepath.Join(dir, "terrain.png"), w, h, tileBytes(g)); err != nil {
			return err
		}
		if err := writePNG(filepath.Join(dir, "height.png"), w+1, h+1, vertexBytes(g)); err != nil {
			return err
		}
		if err := writePNG(filepath.Join(dir, "biome.png"), w, h, s.Map.Biomes()); err != nil {
			return err
		}
	}

	raw, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return fmt.Errorf("mapfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "map.json"), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("mapfile: %w", err)
	}
	return nil
}

// toFileJSON renders the scenario's non-image parts; the caller picks
// rows or PNG file names for the three layers.
func toFileJSON(s *Scenario, pngLayers bool) fileJSON {
	g := s.Map.Grid()
	m := fileJSON{Format: Format, Name: s.Name, Width: g.Width(), Height: g.Height(), Seed: s.Seed}
	if pngLayers {
		m.Terrain.PNG = strptr("terrain.png")
		m.HeightLay.PNG = strptr("height.png")
		m.Biome.PNG = strptr("biome.png")
	} else {
		m.Terrain.Rows = terrainRows(g)
		m.HeightLay.Rows = heightRows(g)
		m.Biome.Rows = biomeRows(s.Map.Biomes(), g.Width(), g.Height())
	}

	for _, p := range s.Players {
		m.Players = append(m.Players, playerJSON{
			Spawn: p.Spawn, Era: eraKey(p.Era), StartKit: p.StartKit,
			Resources: resourcesJSON{
				Food:  resp(p.Res, resource.Food),
				Wood:  resp(p.Res, resource.Wood),
				Stone: resp(p.Res, resource.Stone),
				Gold:  resp(p.Res, resource.Gold),
				Oil:   resp(p.Res, resource.Oil),
			},
		})
	}
	for _, n := range s.Map.Nodes() {
		amount := n.Amount
		nj := nodeJSON{Kind: n.Kind.String(), X: n.X, Y: n.Y, Amount: &amount}
		if n.Tree != tree.None {
			nj.Tree = treeKey(n.Tree)
		}
		m.Nodes = append(m.Nodes, nj)
	}
	for _, u := range s.Units {
		m.Units = append(m.Units, unitJSON{
			Label: u.Label, Def: unitKey(u.DefID), Owner: ownerJSON(u.Owner), X: u.X, Y: u.Y,
		})
	}
	for _, b := range s.Buildings {
		m.Buildings = append(m.Buildings, buildingJSON{
			Label: b.Label, Def: buildingKey(b.DefID), Owner: ownerJSON(b.Owner),
			X: b.X, Y: b.Y, Complete: b.Complete,
		})
	}
	relics := make([][2]int32, 0, worldgen.RelicCount)
	for _, r := range s.Map.Relics() {
		relics = append(relics, r)
	}
	m.Relics = &relics
	return m
}

func resp(res [resource.Count]int32, kind resource.Kind) *int32 {
	v := res[kind]
	return &v
}

func strptr(s string) *string { return &s }

func tileBytes(g *terrain.Grid) []byte {
	out := make([]byte, len(g.Tiles()))
	for i, t := range g.Tiles() {
		out[i] = byte(t)
	}
	return out
}

func vertexBytes(g *terrain.Grid) []byte {
	out := make([]byte, len(g.Vertices()))
	for i, v := range g.Vertices() {
		out[i] = byte(v)
	}
	return out
}

func writePNG(path string, w, h int, vals []byte) error {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{Y: vals[y*w+x]})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("mapfile: %w", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("mapfile: %w", err)
	}
	return nil
}

func terrainRows(g *terrain.Grid) []string {
	w, h := g.Width(), g.Height()
	out := make([]string, h)
	var sb strings.Builder
	for y := 0; y < h; y++ {
		sb.Reset()
		for x := 0; x < w; x++ {
			sb.WriteRune(terrainRunes[g.Tile(x, y)])
		}
		out[y] = sb.String()
	}
	return out
}

func heightRows(g *terrain.Grid) []string {
	w, h := g.Width(), g.Height()
	out := make([]string, h+1)
	for y := 0; y <= h; y++ {
		fs := make([]string, w+1)
		for x := 0; x <= w; x++ {
			fs[x] = strconv.Itoa(int(g.Vertex(x, y)))
		}
		out[y] = strings.Join(fs, " ")
	}
	return out
}

func biomeRows(biomes []uint8, w, h int) []string {
	out := make([]string, h)
	for y := 0; y < h; y++ {
		fs := make([]rune, w)
		for x := 0; x < w; x++ {
			fs[x] = rune('0' + biomes[y*w+x])
		}
		out[y] = string(fs)
	}
	return out
}

func eraKey(id uint8) string {
	for _, d := range era.MustLoad().Defs() {
		if d.ID == id {
			return d.Key
		}
	}
	return ""
}

func unitKey(id uint16) string {
	for _, d := range units.MustLoad().Defs() {
		if d.ID == id {
			return d.Key
		}
	}
	return ""
}

func buildingKey(id uint16) string {
	for _, d := range building.MustLoad().Defs() {
		if d.ID == id {
			return d.Key
		}
	}
	return ""
}

func treeKey(id uint8) string {
	for _, d := range tree.MustLoad().Defs() {
		if uint8(d.ID) == id {
			return d.Key
		}
	}
	return ""
}

func ownerJSON(owner uint8) json.RawMessage {
	if owner == OwnerNeutral {
		return json.RawMessage(`"neutral"`)
	}
	return json.RawMessage(strconv.Itoa(int(owner)))
}
