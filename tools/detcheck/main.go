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
		case "eras":
			err = recordEras(*record, *seed, uint32(*ticks), uint32(*noopTick))
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

func playerUnitIDs(w *core.World, owner uint8) []entity.ID {
	var ids []entity.ID
	w.Units().Each(func(id entity.ID, u *core.Unit) {
		if u.Owner == owner {
			ids = append(ids, id)
		}
	})
	return ids
}

func nearestNode(w *core.World, from *core.Unit, kind resource.Kind) entity.ID {
	var best entity.ID
	bd := int64(1) << 62
	w.Nodes().Each(func(id entity.ID, n *core.Node) {
		if n.Kind != kind || n.Amount <= 0 {
			return
		}
		dx, dy := int64(n.X-from.X), int64(n.Y-from.Y)
		if d := dx*dx + dy*dy; d < bd {
			bd, best = d, id
		}
	})
	return best
}

// buildOffsets are candidate building spots around a spawn, tried in order.
var buildOffsets = [][2]int{{-6, 4}, {4, -6}, {6, 4}, {-4, 6}, {-6, -4}, {4, 6}}

// staged is a recorder that mirrors its own order stream into a live probe
// world, so goals can depend on world state while the recorded stream stays
// reproducible for every peer.
type staged struct {
	r     *Replay
	probe *core.World
	t     uint32
	due   []proto.Order
}

func (s *staged) push(player uint8, kind uint16, payload []byte) {
	order := proto.Order{Tick: s.t, Player: player, Kind: proto.Kind(kind), Payload: payload}
	s.r.Orders = append(s.r.Orders, order)
	s.due = append(s.due, order)
}

// buildingExists reports any site (including under construction) for a
// player and def in the probe.
func (s *staged) buildingExists(player uint8, defID uint16) bool {
	found := false
	s.probe.Buildings().Each(func(_ entity.ID, b *core.Building) {
		if b.Owner == player && b.DefID == defID {
			found = true
		}
	})
	return found
}

func (s *staged) completeBuilding(player uint8, defID uint16) entity.ID {
	var id entity.ID
	s.probe.Buildings().Each(func(eid entity.ID, b *core.Building) {
		if b.Owner == player && b.DefID == defID && b.Progress == 0 {
			id = eid
		}
	})
	return id
}

// buildTry tracks one staged construction attempt across ticks.
type buildTry struct {
	next    int
	waited  int
	site    bool
	givenIn bool
}

// tryBuilding issues at most one build order every couple of ticks, trying
// the next candidate offset when the previous one failed. Returns true once
// the building is complete.
func (s *staged) tryBuilding(bt *buildTry, player uint8, builder entity.ID, spawn [2]int, defID uint16) bool {
	if !bt.site && s.buildingExists(player, defID) {
		bt.site = true
	}
	if bt.site {
		return s.completeBuilding(player, defID) != 0
	}
	if bt.givenIn {
		return false
	}
	if bt.waited > 0 {
		bt.waited--
		return false
	}
	if bt.next >= len(buildOffsets) {
		bt.givenIn = true
		return false
	}
	o := buildOffsets[bt.next]
	bt.next++
	tx, ty := spawn[0]+o[0], spawn[1]+o[1]
	if tx < 0 || ty < 0 || tx >= 0xFFFF || ty >= 0xFFFF {
		bt.waited = 2
		return false
	}
	s.push(player, orders.Build, orders.BuildPayload(uint32(builder), defID, uint16(tx), uint16(ty)))
	bt.waited = 2
	return false
}

// recordCombat drives a two-player skirmish through the era ladder: deer
// hunt with corpse harvest, age-ups to Antiquity, barracks on both sides,
// training queues, a patrol and cross-base attacks producing counterattacks,
// deaths and population release. Era gates make static tick schedules
// impossible, so goals fire off live probe state.
func recordCombat(path string, seed uint64, ticks, noopEvery uint32) error {
	probe := core.NewWorld(seed)
	spawns := probe.Map().Spawns()
	if len(spawns) < 2 {
		return fmt.Errorf("combat probe: need 2 spawns, got %d", len(spawns))
	}
	p0 := playerUnitIDs(probe, 0)
	p1 := playerUnitIDs(probe, 1)
	if len(p0) < 3 || len(p1) < 3 {
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
	s := &staged{r: &Replay{Seed: seed, Ticks: ticks}, probe: probe}

	// tick 1: deer hunt plus full gather on both sides
	s.push(0, orders.Attack, orders.AttackPayload([]uint32{uint32(p0[0])}, uint32(deer)))
	for _, id := range []entity.ID{p0[1], p0[2]} {
		if n := nearestNode(probe, probe.Units().Get(id), resource.Food); n != 0 {
			s.push(0, orders.Gather, orders.GatherPayload(uint32(id), uint32(n)))
		}
	}
	for _, id := range p1 {
		if n := nearestNode(probe, probe.Units().Get(id), resource.Food); n != 0 {
			s.push(1, orders.Gather, orders.GatherPayload(uint32(id), uint32(n)))
		}
	}
	s.probe.Step(s.due)
	s.due = nil

	var (
		age0, age1, patrol0, train0, train1, atk0 bool
		bar0, bar1                                buildTry
	)
	s0 := [2]int{spawns[0][0], spawns[0][1]}
	s1 := [2]int{spawns[1][0], spawns[1][1]}

	for t := uint32(1); t < ticks; t++ {
		s.t = t
		s.due = nil

		ps := s.probe.Players()
		if !age0 && ps[0].Res[resource.Food] >= 250 {
			age0 = true
			s.push(0, orders.AgeUp, orders.AgeUpPayload(1))
		}
		if !age1 && ps[1].Res[resource.Food] >= 250 {
			age1 = true
			s.push(1, orders.AgeUp, orders.AgeUpPayload(1))
		}
		if ps[0].Era >= 1 {
			if s.tryBuilding(&bar0, 0, p0[1], s0, 4) && !train0 {
				train0 = true
				s.push(0, orders.Train, orders.TrainPayload(uint32(s.completeBuilding(0, 4)), 3))
			}
			if !patrol0 && t > 1500 {
				patrol0 = true
				s.push(0, orders.Patrol, orders.PatrolPayload([]uint32{uint32(p0[2])},
					uint16((s1[0]-10)*16+8), uint16(s1[1]*16+8)))
			}
		}
		if ps[1].Era >= 1 {
			if s.tryBuilding(&bar1, 1, p1[0], s1, 4) && !train1 {
				train1 = true
				bid := uint32(s.completeBuilding(1, 4))
				s.push(1, orders.Train, orders.TrainPayload(bid, 2))
				s.push(1, orders.Train, orders.TrainPayload(bid, 2))
			}
		}
		if !atk0 && t >= 2200 {
			var atkIDs, tgt []uint32
			s.probe.Units().Each(func(id entity.ID, u *core.Unit) {
				if u.Owner == 0 && (u.DefID == 3 || u.DefID == 2) && len(atkIDs) < 2 {
					atkIDs = append(atkIDs, uint32(id))
				}
				if u.Owner == 1 && u.DefID == 2 && len(tgt) == 0 {
					tgt = append(tgt, uint32(id))
				}
			})
			if len(atkIDs) > 0 && len(tgt) > 0 {
				atk0 = true
				s.push(0, orders.Attack, orders.AttackPayload(atkIDs, tgt[0]))
			}
		}
		s.probe.Step(s.due)
	}

	sortOrders(s.r.Orders)
	s.r.FinalHash = s.r.Run(core.HashPeriodTicks, nil)
	if err := s.r.Write(path); err != nil {
		return err
	}
	fmt.Printf("recorded %s seed=%#x ticks=%d orders=%d final=%016x\n",
		path, seed, ticks, len(s.r.Orders), s.r.FinalHash)
	return nil
}

// recordEras exercises the phase 6 ladder: age up to Antiquity, build a
// barracks and a market, research a tech chain (prerequisites included),
// train the medieval roster after ageing to Medieval, patrol toward the
// relics and contest one with military.
// recordEras exercises the phase 6 ladder within reach of the prehistory
// economy: gather, age up to Antiquity, build barracks and market, chop
// wood, research a prerequisite chain (forging then casting once affordable),
// train a lancer and patrol.
func recordEras(path string, seed uint64, ticks, noopEvery uint32) error {
	probe := core.NewWorld(seed)
	spawns := probe.Map().Spawns()
	if len(spawns) < 1 {
		return fmt.Errorf("eras probe: no spawns")
	}
	p0 := playerUnitIDs(probe, 0)
	if len(p0) < 3 {
		return fmt.Errorf("eras probe: p0 has %d units", len(p0))
	}
	s := &staged{r: &Replay{Seed: seed, Ticks: ticks}, probe: probe}

	for _, id := range p0 {
		if n := nearestNode(probe, probe.Units().Get(id), resource.Food); n != 0 {
			s.push(0, orders.Gather, orders.GatherPayload(uint32(id), uint32(n)))
		}
	}
	s.probe.Step(s.due)
	s.due = nil

	var (
		age1, lumber, miner, techF, techS, train, patrol bool
		bar, mar                                         buildTry
	)
	s0 := [2]int{spawns[0][0], spawns[0][1]}

	for t := uint32(1); t < ticks; t++ {
		s.t = t
		s.due = nil
		pl := s.probe.Players()[0]

		if !age1 && pl.Res[resource.Food] >= 250 {
			age1 = true
			s.push(0, orders.AgeUp, orders.AgeUpPayload(1))
		}
		if pl.Era >= 1 {
			barracksOK := s.tryBuilding(&bar, 0, p0[1], s0, 4)
			if !lumber && pl.Res[resource.Wood] < 125 {
				lumber = true
				if w := nearestNode(probe, probe.Units().Get(p0[2]), resource.Wood); w != 0 {
					s.push(0, orders.Gather, orders.GatherPayload(uint32(p0[2]), uint32(w)))
				}
			}
			marketOK := false
			if pl.Res[resource.Wood] >= 125 {
				marketOK = s.tryBuilding(&mar, 0, p0[2], s0, 5)
			}
			if barracksOK && !miner {
				miner = true
				if g := nearestNode(probe, probe.Units().Get(p0[1]), resource.Gold); g != 0 {
					s.push(0, orders.Gather, orders.GatherPayload(uint32(p0[1]), uint32(g)))
				}
			}
			if barracksOK && !techF && pl.Res[resource.Gold] >= 50 {
				techF = true
				s.push(0, orders.Research, orders.ResearchPayload(1))
			}
			if techF && !techS && pl.Res[resource.Gold] >= 50 {
				techS = true
				s.push(0, orders.Research, orders.ResearchPayload(2))
			}
			if barracksOK && !train && pl.Res[resource.Food] >= 60 && pl.Res[resource.Gold] >= 20 {
				train = true
				s.push(0, orders.Train, orders.TrainPayload(uint32(s.completeBuilding(0, 4)), 2))
			}
			if !patrol && t > 2000 {
				patrol = true
				s.push(0, orders.Patrol, orders.PatrolPayload([]uint32{uint32(p0[0])},
					uint16((s0[0]+10)*16+8), uint16(s0[1]*16+8)))
			}
			if marketOK && pl.Res[resource.Food] < 150 {
				s.push(0, orders.Gather, orders.GatherPayload(uint32(p0[2]),
					uint32(nearestNode(probe, probe.Units().Get(p0[2]), resource.Food))))
			}
		}
		s.probe.Step(s.due)
	}
	sortOrders(s.r.Orders)
	s.r.FinalHash = s.r.Run(core.HashPeriodTicks, nil)
	if err := s.r.Write(path); err != nil {
		return err
	}
	fmt.Printf("recorded %s seed=%#x ticks=%d orders=%d final=%016x\n",
		path, seed, ticks, len(s.r.Orders), s.r.FinalHash)
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
