package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestProcessLoginsSeedsHitpoints(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.newPlayers = []*Player{p}
	s.processLogins()
	if p.baseLevels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("baseLevels[Hitpoints]: got %d, want 10",
			p.baseLevels[objtype.PlayerStatHitpoints])
	}
	if p.levels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("levels[Hitpoints]: got %d, want 10",
			p.levels[objtype.PlayerStatHitpoints])
	}
}
