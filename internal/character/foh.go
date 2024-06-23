package character

import (
	"sort"
	"time"
	
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/npc"
	"github.com/hectorgimenez/d2go/pkg/data/skill"
	"github.com/hectorgimenez/d2go/pkg/data/stat"
	"github.com/hectorgimenez/koolo/internal/action"
	"github.com/hectorgimenez/koolo/internal/action/step"
	"github.com/hectorgimenez/koolo/internal/game"
	"github.com/hectorgimenez/koolo/internal/helper"
	"github.com/hectorgimenez/koolo/internal/pather"
)

const (
	fohMaxAttacksLoop = 20
	fohMinDistance    = 15
	fohMaxDistance    = 20
)

type Foh struct {
	BaseCharacter
}

func (s Foh) KillMonsterSequence(
	monsterSelector func(d game.Data) (data.UnitID, bool),
	skipOnImmunities []stat.Resist,
	opts ...step.AttackOption,
) action.Action {
	completedAttackLoops := 0
	previousUnitID := 0

	return action.NewStepChain(func(d game.Data) []step.Step {
		id, found := monsterSelector(d)
		if !found {
			return nil
		}
		if previousUnitID != int(id) {
			completedAttackLoops = 0
		}

		if !s.preBattleChecks(d, id, skipOnImmunities) {
			return nil
		}

		if completedAttackLoops >= fohMaxAttacksLoop {
			return nil
		}

		steps := make([]step.Step, 0)
		if d.PlayerUnit.LeftSkill != skill.FistOfTheHeavens {
			steps = append(steps, s.switchToSkill(d, skill.FistOfTheHeavens))
		}

		monster, found := d.Monsters.FindByID(id)
		if !found {
			return nil
		}

		if resistance, ok := monster.Stats[stat.LightningResist]; ok && resistance > 100 {
			// Kill all non-lightning-immune monsters around the lightning-immune monster first
			nearbyMonsters := s.getNearbyMonsters(d, monster.Position)
			for _, m := range nearbyMonsters {
				if r, ok := m.Stats[stat.LightningResist]; !ok || r <= 100 {
					steps = append(steps, s.primaryAttack(m.UnitID))
				}
			}
			// Attack the lightning-immune monster
			steps = append(steps, s.switchAndAttack(d, id, skill.HolyBolt)...)
		} else {
			steps = append(steps, s.primaryAttack(id))
		}

		completedAttackLoops++
		previousUnitID = int(id)

		return steps
	}, action.RepeatUntilNoSteps())
}

func (s Foh) switchToSkill(d game.Data, sk skill.ID) step.Step {
	if key, found := d.KeyBindings.KeyBindingForSkill(sk); found {
		return step.SyncStep(func(_ game.Data) error {
			helper.Sleep(40)
			s.container.HID.PressKeyBinding(key)
			return nil
		})
	}
	return step.SyncStep(func(_ game.Data) error { return nil }) // NoOp replacement
}

func (s Foh) performAttack(d game.Data, monster data.Monster, id data.UnitID) step.Step {
	if resistance, ok := monster.Stats[stat.LightningResist]; ok && resistance > 100 {
		return s.switchAndAttack(d, id, skill.HolyBolt)[0]
	}
	return s.primaryAttack(id)
}

func (s Foh) switchAndAttack(d game.Data, id data.UnitID, sk skill.ID) []step.Step {
	if key, found := d.KeyBindings.KeyBindingForSkill(sk); found {
		return []step.Step{
			step.SyncStep(func(_ game.Data) error {
				helper.Sleep(40)
				s.container.HID.PressKeyBinding(key)
				return nil
			}),
			step.PrimaryAttack(
				id,
				3,
				true,
				step.Distance(fohMinDistance, fohMaxDistance),
				step.EnsureAura(skill.Conviction),
			),
		}
	}
	return []step.Step{step.SyncStep(func(_ game.Data) error { return nil })} // NoOp replacement
}

func (s Foh) primaryAttack(id data.UnitID) step.Step {
	return step.PrimaryAttack(
		id,
		3,
		true,
		step.Distance(fohMinDistance, fohMaxDistance),
		step.EnsureAura(skill.Conviction),
	)
}

func (s Foh) getNearbyMonsters(d game.Data, position data.Position) []data.Monster {
	var nearbyMonsters []data.Monster
	for _, monster := range d.Monsters {
		if pather.DistanceFromMe(d, monster.Position) <= fohMaxDistance {
			nearbyMonsters = append(nearbyMonsters, monster)
		}
	}
	return nearbyMonsters
}

func (s Foh) killBoss(npcID npc.ID, t data.MonsterType) action.Action {
	return action.NewStepChain(func(d game.Data) []step.Step {
		m, found := d.Monsters.FindOne(npcID, t)
		if !found || m.Stats[stat.Life] <= 0 {
			helper.Sleep(100)
			return nil
		}

		steps := s.attackWithSwitch(d, m.UnitID)

		return steps
	}, action.RepeatUntilNoSteps())
}

func (s Foh) attackWithSwitch(d game.Data, id data.UnitID) []step.Step {
	hbKey, holyBoltFound := d.KeyBindings.KeyBindingForSkill(skill.HolyBolt)
	fohKey, fohFound := d.KeyBindings.KeyBindingForSkill(skill.FistOfTheHeavens)

	if holyBoltFound && fohFound {
		return []step.Step{
			step.PrimaryAttack(
				id,
				1,
				true,
				step.Distance(fohMinDistance, fohMaxDistance),
				step.EnsureAura(skill.Conviction),
			),
			step.SyncStep(func(_ game.Data) error {
				s.container.HID.PressKeyBinding(hbKey)
				helper.Sleep(40)
				return nil
			}),
			step.PrimaryAttack(
				id,
				3,
				true,
				step.Distance(fohMinDistance, fohMaxDistance),
				step.EnsureAura(skill.Conviction),
			)
			step.SyncStep(func(_ game.Data) error {
				helper.Sleep(40)
				s.container.HID.PressKeyBinding(fohKey)
				return nil
			}),
		}
	}

	return []step.Step{
		s.primaryAttack(id),
	}
}

func (s Foh) BuffSkills(d game.Data) []skill.ID {
	if _, found := d.KeyBindings.KeyBindingForSkill(skill.HolyShield); found {
		return []skill.ID{skill.HolyShield}
	}
	return []skill.ID{}
}

func (s Foh) PreCTABuffSkills(_ game.Data) []skill.ID {
	return []skill.ID{}
}

func (s Foh) KillCountess() action.Action {
	return s.killMonster(npc.DarkStalker, data.MonsterTypeSuperUnique)
}

func (s Foh) KillAndariel() action.Action {
	return s.killBoss(npc.Andariel, data.MonsterTypeNone)
}

func (s Foh) KillSummoner() action.Action {
	return s.killMonster(npc.Summoner, data.MonsterTypeNone)
}

func (s Foh) KillDuriel() action.Action {
	return s.killBoss(npc.Duriel, data.MonsterTypeNone)
}

func (s Foh) KillPindle(_ []stat.Resist) action.Action {
	return s.killMonster(npc.DefiledWarrior, data.MonsterTypeSuperUnique)
}

func (s Foh) KillMephisto() action.Action {
	return s.killBoss(npc.Mephisto, data.MonsterTypeNone)
}

func (s Foh) KillNihlathak() action.Action {
	return s.killMonster(npc.Nihlathak, data.MonsterTypeSuperUnique)
}

func (s Foh) KillSeis() action.Action {
	return s.killBoss(npc.DoomKnight3, data.MonsterTypeSuperUnique)
}

func (s Foh) KillDiablo() action.Action {
	timeout := time.Second * 20
	startTime := time.Time{}
	diabloFound := false
	return action.NewChain(func(d game.Data) []action.Action {
		if startTime.IsZero() {
			startTime = time.Now()
		}

		if time.Since(startTime) > timeout && !diabloFound {
			s.logger.Error("Diablo was not found, timeout reached")
			return nil
		}

		diablo, found := d.Monsters.FindOne(npc.Diablo, data.MonsterTypeNone)
		if !found || diablo.Stats[stat.Life] <= 0 {
			if diabloFound {
				return nil
			}

			return []action.Action{action.NewStepChain(func(d game.Data) []step.Step {
				return []step.Step{step.Wait(time.Millisecond * 100)}
			})}
		}

		diabloFound = true
		s.logger.Info("Diablo detected, attacking")

		return []action.Action{
			s.killBoss(npc.Diablo, data.MonsterTypeNone),
			s.killBoss(npc.Diablo, data.MonsterTypeNone),
			s.killBoss(npc.Diablo, data.MonsterTypeNone),
		}
	}, action.RepeatUntilNoSteps())
}

func (s Foh) KillIzual() action.Action {
	return s.killBoss(npc.Izual, data.MonsterTypeNone)
}

func (s Foh) KillCouncil() action.Action {
	return action.NewStepChain(func(d game.Data) []step.Step {
		var councilMembers []data.Monster
		for _, m := range d.Monsters {
			if m.Name == npc.CouncilMember || m.Name == npc.CouncilMember2 || m.Name == npc.CouncilMember3 {
				councilMembers = append(councilMembers, m)
			}
		}

		sort.Slice(councilMembers, func(i, j int) bool {
			distanceI := pather.DistanceFromMe(d, councilMembers[i].Position)
			distanceJ := pather.DistanceFromMe(d, councilMembers[j].Position)
			return distanceI < distanceJ
		})

		var steps []step.Step
		for _, m := range councilMembers {
			for range fohMaxAttacksLoop {
				steps = append(steps,
					step.PrimaryAttack(
						m.UnitID,
						3,
						true,
						step.Distance(fohMinDistance, fohMaxDistance),
						step.EnsureAura(skill.Conviction),
					),
				)
			}
		}
		return steps
	}, action.CanBeSkipped())
}

func (s Foh) KillBaal() action.Action {
	return s.killBoss(npc.BaalCrab, data.MonsterTypeNone)
}

func (s Foh) killMonster(npcID npc.ID, t data.MonsterType) action.Action {
	return action.NewStepChain(func(d game.Data) []step.Step {
		m, found := d.Monsters.FindOne(npcID, t)
		if !found {
			return nil
		}

		helper.Sleep(100)
		var steps []step.Step
		for range fohMaxAttacksLoop {
			steps = append(steps,
				step.PrimaryAttack(
					m.UnitID,
					3,
					true,
					step.Distance(fohMinDistance, fohMaxDistance),
					step.EnsureAura(skill.Conviction),
				),
			)
		}

		return steps
	}, action.CanBeSkipped())
}
