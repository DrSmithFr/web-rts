// Package path implements deterministic flow-field pathfinding over a
// tile grid. A field is a BFS distance map computed backwards from a goal
// inside a rectangular window; walkers pick the neighbour with the lowest
// distance, which makes every unit in a group share one field.
//
// All routines are pure Go integers over callbacks: no maps, no floats,
// no time. Given the same walkability the result is identical on every
// platform.
package path

// Unreachable is the sentinel distance for cells with no route to goal.
const Unreachable uint16 = 65535

// WalkFn reports whether a unit may occupy tile (x, y).
type WalkFn func(x, y int32) bool

// Field is a flow field window. Dist is row-major, width*height cells.
type Field struct {
	OriginX, OriginY int32
	Width, Height    int32
	GoalX, GoalY     int32
	Dist             []uint16
}

// eight neighbour offsets, fixed order: N, NE, E, SE, S, SW, W, NW.
var neighborDX = [8]int32{0, 1, 1, 1, 0, -1, -1, -1}
var neighborDY = [8]int32{-1, -1, 0, 1, 1, 1, 0, -1}

// ComputeField BFS-expands from (goalX, goalY) inside the closed-open
// window [ox, ox+w) x [oy, oy+h). Cells outside the window or unwalkable
// keep Unreachable. The goal itself is reachable iff walkable.
func ComputeField(walked WalkFn, ox, oy, w, h int32, goalX, goalY int32) Field {
	f := Field{
		OriginX: ox, OriginY: oy,
		Width: w, Height: h,
		GoalX: goalX, GoalY: goalY,
		Dist: make([]uint16, w*h),
	}
	for i := range f.Dist {
		f.Dist[i] = Unreachable
	}
	if !walked(goalX, goalY) {
		return f
	}
	idx := func(x, y int32) int32 { return (y-oy)*w + (x - ox) }
	f.Dist[idx(goalX, goalY)] = 0

	// BFS queue over the window; distances only increase, so a plain
	// FIFO gives exact shortest steps.
	queue := make([]int32, 0, w*h)
	start := idx(goalX, goalY)
	queue = append(queue, start)
	for head := 0; head < len(queue); head++ {
		c := queue[head]
		cx := ox + c%w
		cy := oy + c/w
		d := f.Dist[c]
		for n := 0; n < 8; n++ {
			nx, ny := cx+neighborDX[n], cy+neighborDY[n]
			if nx < ox || ny < oy || nx >= ox+w || ny >= oy+h {
				continue
			}
			ni := idx(nx, ny)
			if f.Dist[ni] != Unreachable {
				continue
			}
			// No corner cutting: a diagonal needs both shared
			// orthogonal cells walkable, so units never clip
			// through diagonal gaps.
			if n%2 == 1 {
				if !walked(cx+neighborDX[n], cy) || !walked(cx, cy+neighborDY[n]) {
					continue
				}
			}
			if !walked(nx, ny) {
				continue
			}
			f.Dist[ni] = d + 1
			queue = append(queue, ni)
		}
	}
	return f
}

// Distance returns the step count from (x, y) to the goal.
func (f *Field) Distance(x, y int32) uint16 {
	if x < f.OriginX || y < f.OriginY ||
		x >= f.OriginX+f.Width || y >= f.OriginY+f.Height {
		return Unreachable
	}
	return f.Dist[(y-f.OriginY)*f.Width+(x-f.OriginX)]
}

// AtGoal reports whether the walker stands on the goal tile.
func (f *Field) AtGoal(x, y int32) bool { return x == f.GoalX && y == f.GoalY }

// NextStep returns the neighbour tile (or the same tile to wait) with the
// lowest distance that is also walkable. False when the walker cannot see
// the field or has no finite route; callers fall back to direct motion.
//
// Ties break on the fixed neighbour order, so all platforms decide alike.
// BlockFn, when not nil, additionally reports tiles blocked by entities
// (buildings); the walker's own tile is never considered blocked.
func NextStep(f *Field, walked WalkFn, blocked WalkFn, x, y int32) (int32, int32, bool) {
	return NextStepTo(f, walked, blocked, x, y, -1, -1)
}

// NextStepTo is NextStep with a goal-aware tie-break: among the
// neighbours that all shorten the route by one step, the one whose tile
// lies closest to the goal (squared tile distance, ties on the fixed
// scan order) wins. Straight lines and shallow angles walk smoothly
// instead of staircases. A negative goal disables the tie-break.
func NextStepTo(f *Field, walked WalkFn, blocked WalkFn, x, y, goalX, goalY int32) (int32, int32, bool) {
	d := f.Distance(x, y)
	if d == Unreachable {
		return x, y, false
	}
	if d == 0 {
		return x, y, true
	}
	bestN := -1
	var bestD2 int64
	for n := 0; n < 8; n++ {
		nx, ny := x+neighborDX[n], y+neighborDY[n]
		dn := f.Distance(nx, ny)
		if dn == Unreachable || dn+1 != d {
			continue
		}
		if !walked(nx, ny) {
			continue
		}
		if blocked != nil && blocked(nx, ny) {
			continue
		}
		if goalX < 0 || goalY < 0 {
			return nx, ny, true
		}
		ex, ey := int64(nx-goalX), int64(ny-goalY)
		d2 := ex*ex + ey*ey
		if bestN < 0 || d2 < bestD2 {
			bestN, bestD2 = n, d2
		}
	}
	if bestN >= 0 {
		return x + neighborDX[bestN], y + neighborDY[bestN], true
	}
	// Every ideal neighbour is blocked: wait in place rather than walk
	// into the obstruction.
	return x, y, true
}

// SpiralOffsets are the deterministic formation slots around a base tile,
// ring by ring, clockwise from north. Index 0 is the base itself.
var SpiralOffsets = buildSpiral(64)

func buildSpiral(max int) [][2]int32 {
	out := make([][2]int32, 0, max)
	out = append(out, [2]int32{0, 0})
	for r := int32(1); len(out) < max; r++ {
		// four sides, clockwise from north-east: top, right, bottom, left
		for side := 0; side < 4 && len(out) < max; side++ {
			var a, b, da, db int32
			switch side {
			case 0: // top edge, east: (-r+1, -r) .. (r, -r)
				a, b, da, db = -r+1, -r, 1, 0
			case 1: // right edge, south: (r, -r+1) .. (r, r)
				a, b, da, db = r, -r+1, 0, 1
			case 2: // bottom edge, west: (r-1, r) .. (-r, r)
				a, b, da, db = r-1, r, -1, 0
			default: // left edge, north: (-r, r-1) .. (-r, -r+1)
				a, b, da, db = -r, r-1, 0, -1
			}
			for i := int32(0); i < 2*r && len(out) < max; i++ {
				out = append(out, [2]int32{a + da*i, b + db*i})
			}
		}
	}
	return out
}
