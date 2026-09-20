package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"rts/sim/core"
	"rts/sim/proto"
)

const committedReplay = "../../tools/replays/phase1_noop.rts"

func TestReplayRoundTrip(t *testing.T) {
	r := &Replay{
		Seed:  0xabcd,
		Ticks: 10,
		Orders: []proto.Order{
			protoOrder(0, 0, nil),
			protoOrder(5, 1, []byte{0x01, 0x02}),
		},
		FinalHash: 0xdeadbeefcafebabe,
	}
	path := filepath.Join(t.TempDir(), "rt.rts")
	if err := r.Write(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seed != r.Seed || got.Ticks != r.Ticks || got.FinalHash != r.FinalHash {
		t.Fatalf("header mismatch: %+v", got)
	}
	if len(got.Orders) != 2 {
		t.Fatalf("orders = %d, want 2", len(got.Orders))
	}
	if !bytes.Equal(got.Orders[1].Payload, []byte{0x01, 0x02}) {
		t.Fatalf("payload mismatch: %v", got.Orders[1].Payload)
	}
}

func TestCommittedReplayVerifies(t *testing.T) {
	if _, err := os.Stat(committedReplay); err != nil {
		t.Fatalf("committed replay missing: %v", err)
	}
	r, err := Load(committedReplay)
	if err != nil {
		t.Fatal(err)
	}
	var checkpoints int
	final := r.Run(core.HashPeriodTicks, func(_ uint32, h uint64) {
		if h == 0 {
			t.Fatal("zero checkpoint hash")
		}
		checkpoints++
	})
	if final != r.FinalHash {
		t.Fatalf("committed replay desynced: %016x != %016x", final, r.FinalHash)
	}
	if want := int(r.Ticks / core.HashPeriodTicks); checkpoints != want {
		t.Fatalf("checkpoints = %d, want %d", checkpoints, want)
	}
}
