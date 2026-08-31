package world

import (
	"io"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestProcessLogins_LastStepXZ_InitialisedFromOnLogin pins TS
// Player.ts:511-512 — onLogin sets lastStepX = x - 1, lastStepZ = z
// to establish the "imaginary previous step from the west" so
// followX/Z reads a valid coord before the player takes their first
// step. Required for player-follow to converge when the leader is
// stationary post-login (NAI-174 Bug 1 — half of the
// NAI-173-FU-FOLLOW-MODE-INVESTIGATION cascade).
func TestProcessLogins_LastStepXZ_InitialisedFromOnLogin(t *testing.T) {
	s := newTestServer(t)

	p, conn := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, conn)
	p.x, p.z, p.level = 3200, 3210, 0
	// newTestPlayer initialises lastStepX/Z to -1, -1 (player.go:502-503).
	// Sanity-check the pre-condition so a future fixture change doesn't
	// silently invalidate the test.
	if p.lastStepX != -1 || p.lastStepZ != -1 {
		t.Fatalf("pre-condition: newTestPlayer should init lastStepX/Z to -1/-1; got %d/%d",
			p.lastStepX, p.lastStepZ)
	}

	// Queue into the newPlayers batch and run the login flow.
	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()
	s.processLogins()

	if p.lastStepX != 3199 {
		t.Errorf("lastStepX: got %d, want 3199 (= p.x - 1, TS Player.ts:511)", p.lastStepX)
	}
	if p.lastStepZ != 3210 {
		t.Errorf("lastStepZ: got %d, want 3210 (= p.z, TS Player.ts:512)", p.lastStepZ)
	}
}

func TestProcessLoginsSeedsAllSkillsToDefaults(t *testing.T) {
	s := newTestServer(t)
	p, conn := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, conn)
	s.newPlayers = []*Player{p}
	s.processLogins()

	// All non-Hitpoints skills: level 1, base level 1, XP 0.
	for i := range objtype.PlayerStatCount {
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
