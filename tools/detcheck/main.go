package main

import (
	"flag"
	"fmt"
	"os"

	"rts/sim/core"
)

// Exit codes: 0 ok, 1 file or usage error, 2 hash mismatch.
func main() {
	var (
		replay   = flag.String("replay", "", "run a replay, stream checkpoints, verify final hash")
		verify   = flag.String("verify", "", "run a replay silently, only report verification")
		record   = flag.String("record", "", "record the built-in noop scenario to this path")
		seed     = flag.Uint64("seed", 0x5eed, "world seed for -record")
		ticks    = flag.Uint64("ticks", 400, "tick count for -record")
		noopTick = flag.Uint64("noop-every", 37, "schedule a noop order every N ticks for -record")
	)
	flag.Parse()

	switch {
	case *record != "":
		if err := recordScenario(*record, *seed, uint32(*ticks), uint32(*noopTick)); err != nil {
			fail(err)
		}
	case *replay != "":
		r, err := Load(*replay)
		if err != nil {
			fail(err)
		}
		final := r.Run(core.HashPeriodTicks, func(tick uint32, h uint64) {
			fmt.Printf("tick=%d hash=%016x\n", tick, h)
		})
		fmt.Printf("final=%016x\n", final)
		os.Exit(verifyAgainst(r, final))
	case *verify != "":
		r, err := Load(*verify)
		if err != nil {
			fail(err)
		}
		final := r.Run(0, nil)
		os.Exit(verifyAgainst(r, final))
	default:
		flag.Usage()
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func verifyAgainst(r *Replay, final uint64) int {
	if final != r.FinalHash {
		fmt.Fprintf(os.Stderr, "desync: final=%016x expected=%016x\n", final, r.FinalHash)
		return 2
	}
	fmt.Fprintln(os.Stderr, "verify: ok")
	return 0
}

// recordScenario builds the built-in phase 1 scenario: world seeded with
// seed runs for `ticks` ticks while player (k % 2) schedules one noop
// order at every tick multiple of noopEvery.
func recordScenario(path string, seed uint64, ticks, noopEvery uint32) error {
	r := &Replay{Seed: seed, Ticks: ticks}
	for k, t := uint8(0), uint32(0); t < ticks; t += noopEvery {
		r.Orders = append(r.Orders, protoOrder(t, k%2, []byte{'N', k}))
		k++
	}
	r.FinalHash = r.Run(core.HashPeriodTicks, nil)
	if err := r.Write(path); err != nil {
		return err
	}
	fmt.Printf("recorded %s seed=%#x ticks=%d orders=%d final=%016x\n",
		path, seed, ticks, len(r.Orders), r.FinalHash)
	return nil
}
