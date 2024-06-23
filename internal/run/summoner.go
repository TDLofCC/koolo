package run

import (
	"slices"
	"github.com/hectorgimenez/d2go/pkg/data/npc"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/area"
	"github.com/hectorgimenez/koolo/internal/action"
)

type Summoner struct {
	baseRun
}

func (s Summoner) Name() string {
	return string(config.SummonerRun)
}
func (s Summoner) BuildActions() (actions []action.Action) {
	monsterFilter := data.MonsterAnyFilter()
	return []action.Action{
		s.builder.VendorRefill(true, false), // first time to buy keys
		s.builder.VendorRefill(true, true),  // second time to force IDs/TPs
		s.builder.WayPoint(area.ArcaneSanctuary), // Moving to starting point (Arcane Sanctuary)
		s.builder.Buff(),
		// Skip lightning spires
		s.builder.ClearArea(true, func(m data.Monsters) []data.Monster {
			var monsters []data.Monster
			monsters = monsterFilter(m)
			monsters = skipLightSpireFilter(monsters)
			return monsters
		}),
	}
}

func skipLightSpireFilter(monsters data.Monsters) []data.Monster {
	var lightSpireIds = []npc.ID{npc.LightningSpire}
	var filteredMonsters []data.Monster

	for _, m := range monsters {
		if !slices.Contains(lightSpireIds, m.Name) {
			filteredMonsters = append(filteredMonsters, m)
		}
	}

	return filteredMonsters
}

// Original function
//func (s Summoner) BuildActions() (actions []action.Action) {
//	return []action.Action{
//		s.builder.WayPoint(area.ArcaneSanctuary), // Moving to starting point (Arcane Sanctuary)
//		s.builder.MoveTo(func(d game.Data) (data.Position, bool) {
//			m, found := d.NPCs.FindOne(npc.Summoner)
//
//			return m.Positions[0], found
//		}), // Travel to boss position
//		s.char.KillSummoner(), // Kill Summoner
//	}
//}
