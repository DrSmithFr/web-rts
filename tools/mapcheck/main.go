// Command mapcheck validates one or more custom map directories
// (map.json plus any PNG layers) with rts/sim/mapfile and prints a
// one-screen summary. With -ascii it also renders the terrain layers
// as text: '.' grass, 'F' forest, 'R' rock, 's' sand, '~' shallow,
// 'W' deep, '*' node, 'B' building, and a digit per placed unit.
//
// Usage:
//
//	mapcheck [-ascii] dir [dir...]
package main

import (
	"flag"
	"fmt"
	"os"

	"rts/sim/mapfile"
)

func main() {
	ascii := flag.Bool("ascii", false, "render the terrain layers as text")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mapcheck [-ascii] dir [dir...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	bad := 0
	for _, dir := range flag.Args() {
		s, err := mapfile.Load(dir)
		if err != nil {
			fmt.Printf("%s: %v\n", dir, err)
			bad++
			continue
		}
		report(dir, s)
		if *ascii {
			draw(s)
		}
	}
	if bad > 0 {
		os.Exit(1)
	}
}

func report(dir string, s *mapfile.Scenario) {
	g := s.Map.Grid()
	fmt.Printf("%s: ok %s %dx%d seed=%d players=%d nodes=%d units=%d buildings=%d\n",
		dir, s.Name, g.Width(), g.Height(), s.Seed,
		len(s.Players), len(s.Map.Nodes()), len(s.Units), len(s.Buildings))
}

func draw(s *mapfile.Scenario) {
	g := s.Map.Grid()
	for y := 0; y < g.Height(); y++ {
		line := make([]rune, g.Width())
		for x := 0; x < g.Width(); x++ {
			line[x] = mapfile.TerrainRune(g.Tile(x, y))
		}
		for _, n := range s.Map.Nodes() {
			if n.Y == int32(y) {
				line[n.X] = '*'
			}
		}
		for _, b := range s.Buildings {
			if b.Y == int32(y) {
				line[b.X] = 'B'
			}
		}
		for i, u := range s.Units {
			if u.Y == int32(y) {
				line[u.X] = rune('1' + i%9)
			}
		}
		fmt.Println(string(line))
	}
}
