package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestProcessLoginsSeedsAllSkillsToDefaults(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.newPlayers = []*Player{p}
	s.processLogins()

	// All non-Hitpoints skills: level 1, base level 1, XP 0.
	for i := 0; i < objtype.PlayerStatCount; i++ {
		if i == objtype.PlayerStatHitpoints {
			continue
		}
		if p.levels[i] != 1 {
			t.Errorf("levels[%d]: got %d, want 1", i, p.levels[i])
		}
		if p.baseLevels[i] != 1 {
			t.Errorf("baseLevels[%d]: got %d, want 1", i, p.baseLevels[i])
		}
		if p.stats[i] != 0 {
			t.Errorf("stats[%d]: got %d, want 0", i, p.stats[i])
		}
	}

	// Hitpoints overridden to level 10 with matching XP.
	if p.levels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("levels[Hitpoints]: got %d, want 10",
			p.levels[objtype.PlayerStatHitpoints])
	}
	if p.baseLevels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("baseLevels[Hitpoints]: got %d, want 10",
			p.baseLevels[objtype.PlayerStatHitpoints])
	}
	if int(p.stats[objtype.PlayerStatHitpoints]) != objtype.GetExpByLevel(10) {
		t.Errorf("stats[Hitpoints]: got %d, want %d (XP for level 10)",
			p.stats[objtype.PlayerStatHitpoints], objtype.GetExpByLevel(10))
	}
}
