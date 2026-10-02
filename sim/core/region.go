package core

// Regions split the walkable map into landmasses so walkers are sent
// only where they can actually arrive: a berry bush behind a lake is
// worthless, and a unit that presses into the shore forever is worse
// than one that stays idle. Tile walkability comes from the same rules
// the flow fields use, so routing and reachability agree.
//
// Regions are derived from terrain (plus the buildings stamped at world
// setup), never hashed: every peer rebuilds them identically from the map.

// buildRegions labels every walkable tile with its landmass id.
// Unwalkable tiles keep 0, and so does anything past the last region id
// (Reachable then answers true, the conservative case).
func (w *World) buildRegions() {
	width, height := w.gameMap.Grid().Width(), w.gameMap.Grid().Height()
	w.regions = make([]uint16, width*height)
	queue := make([]int32, 0, width*height)
	next := uint16(1)
	for start := 0; start < len(w.regions); start++ {
		if w.regions[start] != 0 || !w.tileWalkable(int32(start%width), int32(start/width)) {
			continue
		}
		id := next
		w.regions[start] = id
		queue = queue[:0]
		queue = append(queue, int32(start))
		for head := 0; head < len(queue); head++ {
			i := queue[head]
			x, y := i%int32(width), i/int32(width)
			for _, n := range regionNeighbors {
				nx, ny := x+n[0], y+n[1]
				if !w.tileWalkable(nx, ny) {
					continue
				}
				j := int(ny)*width + int(nx)
				if w.regions[j] != 0 {
					continue
				}
				w.regions[j] = id
				queue = append(queue, int32(j))
			}
		}
		next++
		if next == 0 {
			// labels exhausted: the remaining ground stays unlabelled
			return
		}
	}
}

var regionNeighbors = [4][2]int32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// regionAt returns the landmass id of a tile, 0 for unwalkable,
// unlabelled or out-of-bounds ground.
func (w *World) regionAt(x, y int32) uint16 {
	width := int32(w.gameMap.Grid().Width())
	if x < 0 || y < 0 || x >= width || y >= int32(w.gameMap.Grid().Height()) {
		return 0
	}
	return w.regions[int(y)*int(width)+int(x)]
}

// Reachable reports whether (toX, toY) sits on the same landmass as
// (fromX, fromY). Unwalkable or unlabelled endpoints answer true: the
// unknown case must not veto a route a flow field could still walk.
func (w *World) Reachable(fromX, fromY, toX, toY int32) bool {
	a, b := w.regionAt(fromX, fromY), w.regionAt(toX, toY)
	return a == 0 || b == 0 || a == b
}
