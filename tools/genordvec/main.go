// Command genordvec regenerates sim/orders/testdata/order_vectors.txt, the
// canonical order payload vectors shared between Go and the TypeScript
// client. Line format: KIND|ARGS|HEX.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"rts/sim/orders"
)

type vec struct {
	name string
	args string
	hex  string
}

func main() {
	vecs := []vec{
		{"move", "7:320,160;8:321,161", hexOf(orders.MovePayload([]orders.MoveItem{
			{ID: 7, X16: 320, Y16: 160}, {ID: 8, X16: 321, Y16: 161},
		}))},
		{"move", "65537:0,0", hexOf(orders.MovePayload([]orders.MoveItem{
			{ID: 65537, X16: 0, Y16: 0},
		}))},
		{"gather", "65537:131072", hexOf(orders.GatherPayload(65537, 131072))},
		{"build", "3:1:7:9", hexOf(orders.BuildPayload(3, 1, 7, 9))},
		{"stop", "1;2;3", hexOf(orders.StopPayload([]uint32{1, 2, 3}))},
		{"train", "196608", hexOf(orders.TrainPayload(196608))},
		{"train", "196608:3", hexOf(orders.TrainPayload(196608, 3))},
		{"move-group", "7;8;9:112,128", hexOf(orders.MoveGroupPayload([]uint32{7, 8, 9}, 112, 128))},
		{"attack", "7;8:65537", hexOf(orders.AttackPayload([]uint32{7, 8}, 65537))},
		{"patrol", "7;8:96,200", hexOf(orders.PatrolPayload([]uint32{7, 8}, 96, 200))},
		{"move", "", hexOf(orders.MovePayload(nil))},
	}
	for _, v := range vecs {
		fmt.Printf("%s|%s|%s\n", v.name, v.args, v.hex)
	}
	if err := os.WriteFile("sim/orders/testdata/order_vectors.txt",
		[]byte(joinLines(vecs)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func joinLines(vecs []vec) string {
	out := ""
	for _, v := range vecs {
		out += fmt.Sprintf("%s|%s|%s\n", v.name, v.args, v.hex)
	}
	return out
}

func hexOf(b []byte) string { return hex.EncodeToString(b) }
