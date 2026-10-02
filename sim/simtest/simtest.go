// Package simtest is the behavior-test kit: load a custom map folder,
// run the world tick by tick with optional brains, and assert on
// labeled entities. A passing test prints nothing; a failing one prints
// at most fifteen lines built from the per-tick ring buffer. With
// RTS_TRACE=1 the full history of labeled entities goes to
// build/trace/<TestName>.txt instead, never to stdout.
package simtest

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"rts/sim/bots"
	"rts/sim/core"
	"rts/sim/entity"
	"rts/sim/fixed"
	"rts/sim/mapfile"
	"rts/sim/proto"
	"rts/sim/resource"
)

// updateFlag regenerates golden snapshots when set.
var updateFlag = flag.Bool("update", false, "regenerate golden snapshot files")

// S is one scenario under test.
type S struct {
	t       *testing.T
	w       *core.World
	labels  core.Labels
	brains  map[uint8]*bots.Brain
	pending []proto.Order

	ring  [ringLen]ringSample
	head  int
	count int

	trace *os.File
}

const (
	ringLen   = 64
	failLines = 8
)

type ringSample struct {
	tick   uint32
	fields []string
}

// Drive steps w with brain-generated orders, stamping them for the next
// tick exactly like a lockstep peer would. It never touches S.
func Drive(w *core.World, brains map[uint8]*bots.Brain, ticks uint32) {
	var pending []proto.Order
	for t := uint32(0); t < ticks; t++ {
		next := pending
		pending = nil
		w.Step(next)
		var ids []uint8
		for p := range brains {
			ids = append(ids, p)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, p := range ids {
			for _, o := range brains[p].Step(w, w.Tick()) {
				o.Tick = w.Tick()
				pending = append(pending, o)
			}
		}
	}
}

// Load decodes dir (a custom map folder) and builds its world.
func Load(t *testing.T, dir string) *S {
	t.Helper()
	sc, err := mapfile.Load(dir)
	if err != nil {
		t.Fatalf("simtest: %v", err)
	}
	w, labels := core.NewWorldFromScenario(sc.Seed, sc)
	s := &S{t: t, w: w, labels: labels, brains: map[uint8]*bots.Brain{}}
	if os.Getenv("RTS_TRACE") == "1" {
		if err := os.MkdirAll("build/trace", 0o775); err != nil {
			t.Fatalf("simtest: trace dir: %v", err)
		}
		f, err := os.Create(filepath.Join("build/trace", sanitize(t.Name())+".txt"))
		if err != nil {
			t.Fatalf("simtest: trace file: %v", err)
		}
		s.trace = f
	}
	s.record()
	return s
}

func sanitize(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}

// Bots registers brains that drive their players every tick.
func (s *S) Bots(b ...*bots.Brain) *S {
	for _, brain := range b {
		s.brains[brain.Player] = brain
	}
	return s
}

// World exposes the raw world.
func (s *S) World() *core.World { return s.w }

// Labels exposes the scenario label table.
func (s *S) Labels() core.Labels { return s.labels }

// step applies pending orders, runs the world tick, then lets brains
// stamp orders for the next tick, exactly like a lockstep peer.
func (s *S) step() {
	next := s.pending
	s.pending = nil
	s.w.Step(next)
	var ids []uint8
	for p := range s.brains {
		ids = append(ids, p)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, p := range ids {
		for _, o := range s.brains[p].Step(s.w, s.w.Tick()) {
			o.Tick = s.w.Tick()
			s.pending = append(s.pending, o)
		}
	}
	s.record()
}

// record snapshots every labeled entity into the ring buffer (and the
// trace file when open).
func (s *S) record() {
	f := s.ring[s.head]
	f.tick = s.w.Tick()
	f.fields = f.fields[:0]
	for _, e := range s.labels.Entries() {
		if e.Kind == core.LabelUnit {
			if u := s.w.Units().Get(e.ID); u != nil {
				f.fields = append(f.fields, s.unitField(e.Name, u))
			} else {
				f.fields = append(f.fields, e.Name+" gone")
			}
		} else {
			if b := s.w.Buildings().Get(e.ID); b != nil {
				f.fields = append(f.fields, s.buildingField(e.Name, b))
			} else {
				f.fields = append(f.fields, e.Name+" gone")
			}
		}
	}
	s.ring[s.head] = f
	s.head = (s.head + 1) % ringLen
	if s.count < ringLen {
		s.count++
	}
	if s.trace != nil {
		fmt.Fprintf(s.trace, "t=%d %s\n", f.tick, strings.Join(f.fields, " | "))
	}
}

func (s *S) unitField(name string, u *core.Unit) string {
	x, y := tileOf(u.X), tileOf(u.Y)
	if u.HP <= 0 {
		return fmt.Sprintf("%-6s dead", name)
	}
	tgt := "-"
	if t := unitTarget(u); t != entity.Nil {
		if n := s.labels.Name(t); n != "-" {
			tgt = n
		}
	}
	return fmt.Sprintf("%-6s %-7s (%d,%d) hp=%-3d tgt=%s", name, StateName(u.State), x, y, u.HP, tgt)
}

func (s *S) buildingField(name string, b *core.Building) string {
	return fmt.Sprintf("%-6s build  (%d,%d) hp=%-3d", name, b.TileX, b.TileY, b.HP)
}

func unitTarget(u *core.Unit) entity.ID {
	if u.AttackTarget != entity.Nil {
		return u.AttackTarget
	}
	return u.Target
}

// ringLines renders the last n recorded samples, oldest to newest.
func (s *S) ringLines(n int) string {
	start := s.count - n
	if start < 0 {
		start = 0
	}
	var sb strings.Builder
	for i := start; i < s.count; i++ {
		idx := (s.head - s.count + i + ringLen) % ringLen
		sb.WriteString(fmt.Sprintf("  t=%-4d %s\n", s.ring[idx].tick, strings.Join(s.ring[idx].fields, " | ")))
	}
	return sb.String()
}

// fail ends the test with the standard at-most-15-line report.
func (s *S) fail(msg string) {
	s.t.Helper()
	if s.trace != nil {
		s.trace.Close()
	}
	s.t.Fatalf("FAIL %s\n%s", msg, s.ringLines(failLines))
}

// StateName renders a core unit state for failure messages.
func StateName(st uint8) string {
	switch st {
	case core.UnitIdle:
		return "idle"
	case core.UnitMove:
		return "move"
	case core.UnitGather:
		return "gather"
	case core.UnitReturn:
		return "return"
	case core.UnitBuild:
		return "build"
	case core.UnitAttack:
		return "attack"
	case core.UnitPatrol:
		return "patrol"
	case core.UnitWander:
		return "wander"
	case core.UnitFlee:
		return "flee"
	case core.UnitSleep:
		return "sleep"
	default:
		return fmt.Sprintf("state%d", st)
	}
}

func tileOf(v fixed.F16) int32 { return int32(v) >> 16 }

// UnitView is one labeled unit.
type UnitView struct {
	s     *S
	label string
	id    entity.ID
}

// U returns the labeled unit view. A missing label fails the test.
func (s *S) U(label string) UnitView {
	s.t.Helper()
	id := s.labels.Unit(label)
	if id == entity.Nil {
		s.t.Fatalf("simtest: no unit label %q", label)
	}
	return UnitView{s: s, label: label, id: id}
}

func (u UnitView) get() *core.Unit {
	u.s.t.Helper()
	un := u.s.w.Units().Get(u.id)
	if un == nil {
		u.s.t.Fatalf("simtest: unit %q (%v) missing from store", u.label, u.id)
	}
	return un
}

// State returns the unit state byte (core.Unit* constants).
func (u UnitView) State() uint8 { return u.get().State }

// ID returns the entity id, so a test can compare it against another
// unit's target.
func (u UnitView) ID() entity.ID { return u.id }

// X and Y return the unit's exact fixed-point position.
func (u UnitView) X() fixed.F16 { return u.get().X }
func (u UnitView) Y() fixed.F16 { return u.get().Y }

// Hunger returns the food meter of a hunter (100 sated to 0 starving).
func (u UnitView) Hunger() int32 { return u.get().Hunger }

// Cond returns the condition bitmask (core.CondFear, CondTerror, CondRage).
func (u UnitView) Cond() uint8 { return u.get().Cond }

// DefID returns the unit type id.
func (u UnitView) DefID() uint16 { return u.get().DefID }

// Heading returns the direction the unit faces, in degrees (0 = +Y, 90 =
// +X, clockwise).
func (u UnitView) Heading() int32 { return u.get().Heading }

// Tile returns the tile coordinate pair.
func (u UnitView) Tile() (int32, int32) {
	un := u.get()
	return tileOf(un.X), tileOf(un.Y)
}

// HP returns current hit points.
func (u UnitView) HP() int32 { return u.get().HP }

// Target returns the attacked or worked target id, entity.Nil if none.
func (u UnitView) Target() entity.ID { return unitTarget(u.get()) }

// Dead reports HP <= 0; a unit already freed from the store counts as
// dead.
func (u UnitView) Dead() bool {
	un := u.s.w.Units().Get(u.id)
	return un == nil || un.HP <= 0
}

// DistHome returns the Chebyshev tile distance from the unit's home
// point, the same metric the wildlife leash uses.
func (u UnitView) DistHome() int32 {
	un := u.get()
	dx, dy := distTile(un.X, un.HomeX), distTile(un.Y, un.HomeY)
	if dx > dy {
		return dx
	}
	return dy
}

// DistTo returns the Chebyshev tile distance to another labeled unit.
func (u UnitView) DistTo(o UnitView) int32 {
	a, b := u.get(), o.get()
	dx, dy := distTile(a.X, b.X), distTile(a.Y, b.Y)
	if dx > dy {
		return dx
	}
	return dy
}

func distTile(a, b fixed.F16) int32 {
	d := (int32(a) - int32(b)) >> 16
	if d < 0 {
		d = -d
	}
	return d
}

// BuildingView is one labeled building.
type BuildingView struct {
	s     *S
	label string
	id    entity.ID
}

// B returns the labeled building view. A missing label fails the test.
func (s *S) B(label string) BuildingView {
	s.t.Helper()
	id := s.labels.Building(label)
	if id == entity.Nil {
		s.t.Fatalf("simtest: no building label %q", label)
	}
	return BuildingView{s: s, label: label, id: id}
}

func (b BuildingView) get() *core.Building {
	b.s.t.Helper()
	bd := b.s.w.Buildings().Get(b.id)
	if bd == nil {
		b.s.t.Fatalf("simtest: building %q (%v) missing from store", b.label, b.id)
	}
	return bd
}

// HP returns current hit points.
// Tile returns the building's origin tile.
func (b BuildingView) Tile() (int32, int32) {
	bd := b.get()
	return bd.TileX, bd.TileY
}

func (b BuildingView) HP() int32 { return b.get().HP }

// Dead reports HP <= 0.
func (b BuildingView) Dead() bool { return b.get().HP <= 0 }

// Step drives one assertion window.
type Step struct {
	s      *S
	limit  uint32
	within bool
}

// Within steps until the condition turns true, failing at the deadline.
func (s *S) Within(ticks uint32) Step { return Step{s: s, limit: ticks, within: true} }

// During steps the whole window.
func (s *S) During(ticks uint32) Step { return Step{s: s, limit: ticks} }

// Expect requires the condition: within mode waits for it, during mode
// demands it after every tick.
func (st Step) Expect(cond func() bool, what string) {
	st.s.t.Helper()
	t0 := st.s.w.Tick()
	if st.within {
		for i := uint32(0); i < st.limit; i++ {
			if cond() {
				return
			}
			st.s.step()
		}
		if cond() {
			return
		}
		st.s.fail(fmt.Sprintf("%s: not true within %d ticks (t=%d..%d)",
			what, st.limit, t0, st.s.w.Tick()))
		return
	}
	for i := uint32(0); i < st.limit; i++ {
		st.s.step()
		if !cond() {
			st.s.fail(fmt.Sprintf("%s: false during window at t=%d (t=%d..%d)",
				what, st.s.w.Tick(), t0, st.s.w.Tick()))
			return
		}
	}
}

// Never requires the condition to stay false across the whole window.
func (st Step) Never(cond func() bool, what string) {
	st.s.t.Helper()
	t0 := st.s.w.Tick()
	for i := uint32(0); i < st.limit; i++ {
		if cond() {
			st.s.fail(fmt.Sprintf("%s: true at t=%d (t=%d..%d)", what, t0, t0, t0))
			return
		}
		st.s.step()
	}
	if cond() {
		st.s.fail(fmt.Sprintf("%s: true at t=%d (t=%d..%d)", what, st.s.w.Tick(), t0, st.s.w.Tick()))
	}
}

// ApproxEq fails when |got-want| exceeds tol.
func (s *S) ApproxEq(got, want, tol int32, what string) {
	s.t.Helper()
	d := got - want
	if d < 0 {
		d = -d
	}
	if d > tol {
		s.t.Fatalf("FAIL %s: got %d, want %d +/- %d", what, got, want, tol)
	}
}

// Snapshot compares the world summary against testdata/golden/<name>.txt;
// -update regenerates the file.
func (s *S) Snapshot(name string) {
	s.t.Helper()
	got := s.SnapshotText()
	path := filepath.Join("testdata", "golden", name+".txt")
	if *updateFlag {
		if err := os.MkdirAll(filepath.Dir(path), 0o775); err != nil {
			s.t.Fatalf("simtest: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			s.t.Fatalf("simtest: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		s.t.Fatalf("simtest: snapshot %s missing (run with -update): %v", name, err)
	}
	if got != string(want) {
		s.t.Fatalf("FAIL snapshot %s mismatch\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

// SnapshotText renders the sorted summary: players (era, resources)
// then every labeled entity (state, tile, HP).
func (s *S) SnapshotText() string {
	var sb strings.Builder
	for i := range core.MaxPlayers {
		pl := &s.w.Players()[i]
		if !pl.Active {
			continue
		}
		fmt.Fprintf(&sb, "player %d era=%d food=%d wood=%d stone=%d gold=%d oil=%d\n",
			i, pl.Era, pl.Res[resource.Food], pl.Res[resource.Wood],
			pl.Res[resource.Stone], pl.Res[resource.Gold], pl.Res[resource.Oil])
	}
	type row struct{ name, line string }
	var rows []row
	for _, e := range s.labels.Entries() {
		if e.Kind == core.LabelUnit {
			if u := s.w.Units().Get(e.ID); u != nil {
				rows = append(rows, row{e.Name, fmt.Sprintf("unit %s state=%s tile=(%d,%d) hp=%d\n",
					e.Name, StateName(u.State), tileOf(u.X), tileOf(u.Y), u.HP)})
			}
			continue
		}
		if b := s.w.Buildings().Get(e.ID); b != nil {
			rows = append(rows, row{e.Name, fmt.Sprintf("building %s tile=(%d,%d) hp=%d\n",
				e.Name, b.TileX, b.TileY, b.HP)})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	for _, r := range rows {
		sb.WriteString(r.line)
	}
	return sb.String()
}
