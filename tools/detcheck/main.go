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
		case "paths":
			err = recordPaths(*record, *seed, uint32(*ticks), uint32(*noopTick))
		case "combat":
			err = recordCombat(*record, *seed, uint32(*ticks), uint32(*noopTick))
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

// firstWorldWithBarracks probes candidate barracks offsets against a fresh
// world per candidate and returns the first that actually gets built, plus
// the coordinates used.
func firstWorldWithBarracks(seed uint64, player uint8, builder entity.ID, spawn [2]int) ([2]int, error) {
	candidates := [][2]int{{-6, 4}, {4, -6}, {6, 4}, {-4, 6}, {-6, -4}, {4, 6}}
	for _, o := range candidates {
		tx, ty := spawn[0]+o[0], spawn[1]+o[1]
		w := core.NewWorld(seed)
		w.Step([]proto.Order{{Player: player, Kind: orders.Build,
			Payload: orders.BuildPayload(uint32(builder), 4, uint16(tx), uint16(ty))}})
		for i := 0; i < 260; i++ {
			w.Step(nil)
		}
		ok := false
		w.Buildings().Each(func(_ entity.ID, b *core.Building) {
			if b.DefID == 4 && b.Owner == player {
				ok = true
			}
		})
		if ok {
			return [2]int{tx, ty}, nil
		}
	}
	return [2]int{}, fmt.Errorf("combat probe: no barracks-buildable offset")
}

func playerUnitIDs(w *core.World, owner uint8) []entity.ID {
	var ids []entity.ID
	w.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.Owner == owner {
			ids = append(ids, id)
		}
	})
	return ids
}

// recordCombat drives a two-player skirmish: deer hunt with corpse
// harvest, barracks on both sides, a training queue that stalls on gold,
// a long patrol into enemy territory and cross-base attack orders that
// produce counterattacks, deaths and population release.
func recordCombat(path string, seed uint64, ticks, noopEvery uint32) error {
	probe := core.NewWorld(seed)
	spawns := probe.Map().Spawns()
	if len(spawns) < 2 {
		return fmt.Errorf("combat probe: need 2 spawns, got %d", len(spawns))
	}
	p0 := playerUnitIDs(probe, 0)
	p1 := playerUnitIDs(probe, 1)
	if len(p0) < 3 || len(p1) < 1 {
		return fmt.Errorf("combat probe: units p0=%d p1=%d", len(p0), len(p1))
	}
	v0 := probe.Units().Get(p0[0])
	var deer entity.ID
	bd := int64(1) << 62
	probe.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.Owner == core.OwnerNeutral && u.DefID == 5 {
			dx, dy := int64(u.X-v0.X), int64(u.Y-v0.Y)
			if d := dx*dx + dy*dy; d < bd {
				bd, deer = d, id
			}
		}
	})
	if deer == 0 {
		return fmt.Errorf("combat probe: no deer found")
	}
	s0 := [2]int{spawns[0][0], spawns[0][1]}
	s1 := [2]int{spawns[1][0], spawns[1][1]}
	bar0, err := firstWorldWithBarracks(seed, 0, p0[1], s0)
	if err != nil {
		return err
	}
	bar1, err := firstWorldWithBarracks(seed, 1, p1[0], s1)
	if err != nil {
		return err
	}

	r := &Replay{Seed: seed, Ticks: ticks}
	add := func(tick uint32, player uint8, kind uint16, payload []byte) {
		r.Orders = append(r.Orders, proto.Order{
			Tick: tick, Player: player, Kind: proto.Kind(kind), Payload: payload,
		})
	}
	add(1, 0, orders.Attack, orders.AttackPayload([]uint32{uint32(p0[0])}, uint32(deer)))
	add(1, 0, orders.Build, orders.BuildPayload(uint32(p0[1]), 4, uint16(bar0[0]), uint16(bar0[1])))
	add(1, 1, orders.Build, orders.BuildPayload(uint32(p1[0]), 4, uint16(bar1[0]), uint16(bar1[1])))
	add(1, 0, orders.Patrol, orders.PatrolPayload([]uint32{uint32(p0[2])},
		uint16((s1[0]-10)*16+8), uint16(s1[1]*16+8)))

	// mirror the order stream in a probe world to resolve the barracks ids
	pr := core.NewWorld(seed)
	for _, o := range r.Orders {
		// orders all target tick 1 here; replay semantics handled below
		pr.Step(withTick(o, 1))
	}
	for i := 0; i < 300; i++ {
		pr.Step(nil)
	}
	var barID0, barID1 entity.ID
	pr.Buildings().Each(func(id entity.ID, b *core.Building) {
		if b.DefID == 4 {
			if b.Owner == 0 {
				barID0 = id
			}
			if b.Owner == 1 {
				barID1 = id
			}
		}
	})
	if barID0 == 0 || barID1 == 0 {
		return fmt.Errorf("combat probe: barracks missing p0=%d p1=%d", barID0, barID1)
	}
	add(301, 1, orders.Train, orders.TrainPayload(uint32(barID1), 2))
	add(302, 1, orders.Train, orders.TrainPayload(uint32(barID1), 2))
	add(303, 0, orders.Train, orders.TrainPayload(uint32(barID0), 3))
	add(304, 0, orders.Train, orders.TrainPayload(uint32(barID0), 3))
	add(305, 0, orders.Train, orders.TrainPayload(uint32(barID0), 2))
	for _, o := range r.Orders {
		if o.Tick >= 301 {
			pr.Step([]proto.Order{o})
		}
	}
	for i := 0; i < 1200; i++ {
		pr.Step(nil)
	}
	var p1Lancers, p0Military []entity.ID
	pr.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.Owner == 1 && u.DefID == 2 && len(p1Lancers) < 2 {
			p1Lancers = append(p1Lancers, id)
		}
		if u.Owner == 0 && (u.DefID == 3 || u.DefID == 2) {
			p0Military = append(p0Military, id)
		}
	})
	if len(p1Lancers) == 0 {
		return fmt.Errorf("combat probe: p1 trained no lancers")
	}
	add(1501, 1, orders.Attack, orders.AttackPayload(unit32s(p1Lancers), uint32(p0[2])))
	if len(p0Military) > 0 {
		add(1501, 0, orders.Attack, orders.AttackPayload(unit32s(p0Military), uint32(p1Lancers[0])))
	}
	for k, t := uint8(0), uint32(0); t < ticks; t += noopEvery {
		add(t, 0, 0, []byte{'N', k})
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

func unit32s(ids []entity.ID) []uint32 {
	out := make([]uint32, 0, len(ids))
	for _, id := range ids {
		out = append(out, uint32(id))
	}
	return out
}

func withTick(o proto.Order, tick uint32) []proto.Order {
	o.Tick = tick
	return []proto.Order{o}
}

// recordPaths drives a group move across rough terrain plus a follow-up
// scatter move, exercising flow fields, formations and fog reveal.
func recordPaths(path string, seed uint64, ticks, noopEvery uint32) error {
	probe := core.NewWorld(seed)
	var ids []uint32
	probe.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.Owner == 0 && len(ids) < 3 {
			ids = append(ids, uint32(id))
		}
	})
	if len(ids) != 3 {
		return fmt.Errorf("paths probe: only %d units", len(ids))
	}
	spawn := probe.Map().Spawns()[0]
	r := &Replay{Seed: seed, Ticks: ticks}
	add := func(tick uint32, kind uint16, payload []byte) {
		r.Orders = append(r.Orders, proto.Order{
			Tick: tick, Player: 0, Kind: proto.Kind(kind), Payload: payload,
		})
	}
	add(1, orders.MoveGroup, orders.MoveGroupPayload(ids,
		uint16((spawn[0]-20)*16+8), uint16((spawn[1]+14)*16+8)))
	add(240, orders.MoveGroup, orders.MoveGroupPayload(ids,
		uint16((spawn[0]+16)*16+8), uint16((spawn[1]+16)*16+8)))
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
