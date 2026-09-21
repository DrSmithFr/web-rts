package main

import (
	"flag"
	"fmt"
	"os"

	"rts/sim/core"
	"rts/sim/entity"
	"rts/sim/orders"
	"rts/sim/proto"
	"rts/sim/resource"
	"sort"
)

// Exit codes: 0 ok, 1 file or usage error, 2 hash mismatch.
func main() {
	var (
		replay   = flag.String("replay", "", "run a replay, stream checkpoints, verify final hash")
		verify   = flag.String("verify", "", "run a replay silently, only report verification")
		record   = flag.String("record", "", "record a built-in scenario to this path")
		scenario = flag.String("scenario", "noop", "record scenario: noop or economy")
		seed     = flag.Uint64("seed", 0x5eed, "world seed for -record")
		ticks    = flag.Uint64("ticks", 400, "tick count for -record")
		noopTick = flag.Uint64("noop-every", 37, "schedule a noop order every N ticks for -record")
	)
	flag.Parse()

	switch {
	case *record != "":
		var err error
		switch *scenario {
		case "noop":
			err = recordScenario(*record, *seed, uint32(*ticks), uint32(*noopTick))
		case "economy":
			err = recordEconomy(*record, *seed, uint32(*ticks), uint32(*noopTick))
		default:
			fail(fmt.Errorf("unknown scenario %q", *scenario))
		}
		if err != nil {
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

// recordEconomy exercises the phase 3 loop: one villager gathers berries,
// another builds a hut at a fixed offset from the spawn, and the
// storehouse trains a fourth villager. Entity ids come from a probe world
// with the same seed, so they are reproducible on every peer.
func recordEconomy(path string, seed uint64, ticks, noopEvery uint32) error {
	probe := core.NewWorld(seed)
	var unitIDs []entity.ID
	probe.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.Owner == 0 && len(unitIDs) < 3 {
			unitIDs = append(unitIDs, id)
		}
	})
	var nodeID entity.ID
	if len(unitIDs) == 3 {
		u := probe.Units().Get(unitIDs[0])
		var bd int64 = 1 << 62
		probe.Nodes().Each(func(id entity.ID, n *core.Node) {
			if n.Kind != resource.Food || n.Amount <= 0 {
				return
			}
			dx, dy := int64(n.X-u.X), int64(n.Y-u.Y)
			if d := dx*dx + dy*dy; d < bd {
				bd, nodeID = d, id
			}
		})
		var store entity.ID
		probe.Buildings().Each(func(id entity.ID, b *core.Building) {
			if b.Owner == 0 && b.Progress == 0 {
				store = id
			}
		})
		_ = store
		if store != 0 && nodeID != 0 {
			spawn := probe.Map().Spawns()[0]
			r := &Replay{Seed: seed, Ticks: ticks}
			add := func(tick uint32, kind uint16, payload []byte) {
				r.Orders = append(r.Orders, proto.Order{
					Tick: tick, Player: 0, Kind: proto.Kind(kind), Payload: payload,
				})
			}
			add(1, orders.Gather, orders.GatherPayload(uint32(unitIDs[0]), uint32(nodeID)))
			add(1, orders.Build, orders.BuildPayload(uint32(unitIDs[1]), 1,
				uint16(spawn[0]-6), uint16(spawn[1]-6)))
			add(2, orders.Train, orders.TrainPayload(uint32(store)))
			for k, t := uint8(0), uint32(0); t < ticks; t += noopEvery {
				add(t, 0, []byte{'N', k})
				k++
			}
			sortOrders(r.Orders)
			r.FinalHash = r.Run(core.HashPeriodTicks, nil)
			if err := r.Write(path); err != nil {
				return err
			}
			fmt.Printf("recorded %s seed=%#x ticks=%d orders=%d final=%016x\n",
				path, seed, ticks, len(r.Orders), r.FinalHash)
			return nil
		}
	}
	return fmt.Errorf("economy probe: units=%d node=%d", len(unitIDs), nodeID)
}

func sortOrders(os []proto.Order) {
	sort.SliceStable(os, func(i, j int) bool { return os[i].Tick < os[j].Tick })
}
