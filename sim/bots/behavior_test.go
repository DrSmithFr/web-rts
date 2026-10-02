package bots_test

import (
	"testing"

	"rts/sim/bots"
	"rts/sim/resource"
	"rts/sim/simtest"
)

func TestBehaviorBotEconomyGathers(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/bot_gathers_wood").
		Bots(bots.New(0, bots.Standard).Only(bots.ModEconomy))
	res := s.World().Players()[0].Res[resource.Wood]
	s.Within(2000).Expect(func() bool {
		return s.World().Players()[0].Res[resource.Wood] > res
	}, "bot economy gathers wood")
}

func TestBehaviorBotWavesAttackVisibleEnemy(t *testing.T) {
	s := simtest.Load(t, "testdata/maps/bot_waves_attack").
		Bots(bots.New(0, bots.Easy).Only(bots.ModWaves))
	victim := s.U("victim")
	s.Within(2000).Expect(victim.Dead, "wave army attacks visible enemy")
}
