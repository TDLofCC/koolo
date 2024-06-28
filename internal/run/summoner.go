package run

import (
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/area"
	"github.com/hectorgimenez/d2go/pkg/data/npc"
	"github.com/hectorgimenez/koolo/internal/action"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/koolo/internal/game"
)

type Summoner struct {
	baseRun
}

func (s Summoner) Name() string {
	return string(config.SummonerRun)
}

func (s Summoner) BuildActions() (actions []action.Action) {
	return []action.Action{
		s.builder.WayPoint(area.ArcaneSanctuary), // Moving to starting point (Arcane Sanctuary)
		s.builder.MoveTo(func(d game.Data) (data.Position, bool) {
			m, found := d.NPCs.FindOne(npc.Summoner)

			return m.Positions[0], found
		}), // Travel to boss position
		s.char.KillSummoner(), // Kill Summoner
	}
}
