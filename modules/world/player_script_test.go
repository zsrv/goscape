package world

import (
	"bytes"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// TestPlayerEffectiveFaceCoord_FallsBackToOrientation pins the player
// spawn-facing fix (twin of the NPC case): a player with no active faceSquare
// (-1) reports its resting orientation (faceAngle, south after unfocus) as the
// wire face coord, so the always-forced FACE_COORD low-def orients a fresh
// player south rather than the client's north-east default.
func TestPlayerEffectiveFaceCoord_FallsBackToOrientation(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z = 3200, 3300
	p.unfocus() // login seeds the default-south orientation
	p.faceSquareX, p.faceSquareZ = -1, -1

	wantX, wantZ := coordgrid.Fine(p.x, 1), coordgrid.Fine(p.z-1, 1)
	if x, z := p.effectiveFaceCoord(); x != wantX || z != wantZ {
		t.Errorf("resting player effectiveFaceCoord = (%d,%d), want faceAngle/south (%d,%d)", x, z, wantX, wantZ)
	}

	p.faceSquareX, p.faceSquareZ = 700, 800
	if x, z := p.effectiveFaceCoord(); x != 700 || z != 800 {
		t.Errorf("active player effectiveFaceCoord = (%d,%d), want faceSquare (700,800)", x, z)
	}
}

func TestAddXPNormalGainNoLevelUp(t *testing.T) {
	p, _ := newTestPlayer(t)
	// Level 2 threshold = GetExpByLevel(2) = 820.
	// Start at 820 (exactly level 2); adding 100 → 920, still below level-3
	// threshold (GetExpByLevel(3) = 1740), so baseLevels stays 2.
	p.stats[objtype.PlayerStatAttack] = int32(objtype.GetExpByLevel(2))
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 100, false)
	want := int32(objtype.GetExpByLevel(2)) + 100
	if p.stats[objtype.PlayerStatAttack] != want {
		t.Errorf("stats: got %d, want %d", p.stats[objtype.PlayerStatAttack], want)
	}
	// Still level 2 (level 3 threshold is GetExpByLevel(3) = 1740).
	if p.baseLevels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("baseLevels: got %d, want 2", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("levels: got %d, want 2 (no replenish)", p.levels[objtype.PlayerStatAttack])
	}
}

// TestAddXPAppliesNodeXPRate pins M7: with allowMulti=true the configured
// node_xp_rate (cfg.NodeXPRate) multiplies the granted XP (TS Player.ts:1751,
// `xp * (allowMulti ? Environment.NODE_XPRATE : 1)`); allowMulti=false bypasses
// it (exact-level grants like the setlevel cheat). Previously the multiplier
// was never applied and node_xp_rate was dead config.
func TestAddXPAppliesNodeXPRate(t *testing.T) {
	s := newServerForScriptTest(t)
	s.cfg.NodeXPRate = 3
	p := newTestPlayerAt(t, s, 1, 3094, 3106, 0)

	p.stats[objtype.PlayerStatAttack] = 0
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	p.AddXP(objtype.PlayerStatAttack, 100, true) // 100 * 3
	if p.stats[objtype.PlayerStatAttack] != 300 {
		t.Errorf("allowMulti=true rate=3: stats got %d, want 300", p.stats[objtype.PlayerStatAttack])
	}

	// allowMulti=false ignores the rate (exact grant).
	p.stats[objtype.PlayerStatAttack] = 0
	p.AddXP(objtype.PlayerStatAttack, 100, false)
	if p.stats[objtype.PlayerStatAttack] != 100 {
		t.Errorf("allowMulti=false rate=3: stats got %d, want 100 (no multiplier)", p.stats[objtype.PlayerStatAttack])
	}
}

// TestOpenModalPreservesTutAndClearsSuspendedDialog pins M8: opening a modal
// clears only the relevant modal bits (not the entire bitmap) so the TUT bit
// survives. Suspended-DIALOG clearing is back at rev-254 (TS 93ef2d7f, see
// TestOpenModalClearsSuspendedDialogAndResumeButtons); a Running activeScript
// is still preserved (the block only matches COUNTDIALOG/PAUSEBUTTON).
func TestOpenModalPreservesTutAndClearsSuspendedDialog(t *testing.T) {
	t.Run("OpenMain_preserves_TUT_clears_chat", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.modalState = modalStateChat | modalStateTut
		p.modalChat = 50
		p.OpenMain(1234)
		if p.modalState&modalStateTut == 0 {
			t.Error("TUT bit wiped by OpenMain; want preserved")
		}
		if p.modalState&modalStateChat != 0 {
			t.Error("CHAT bit not cleared by OpenMain")
		}
		if p.modalState&modalStateMain == 0 {
			t.Error("MAIN bit not set by OpenMain")
		}
		if p.modalChat != -1 || p.modalMain != 1234 {
			t.Errorf("slots: modalChat=%d modalMain=%d, want -1, 1234", p.modalChat, p.modalMain)
		}
	})

	t.Run("preserves_non_dialog_script", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		st := &script.ScriptState{Execution: script.Running}
		p.activeScript = st
		p.OpenMainModalSide(11, 22)
		if p.activeScript != st {
			t.Error("Running activeScript wrongly cleared by OpenMainModalSide")
		}
	})
}

// TestOpenModalClearsSuspendedDialogAndResumeButtons pins the rev-254
// contract (supersedes the 244 TestOpenModalSuspendedScriptSurvives pin):
// TS 93ef2d7f "fix: Clear old suspended scripts on modal open" RE-ADDED
// the block to all four modal-open methods, and 2dc4a811 extended it to
// also clear resumeButtons. At pin @2e3bcf43 (openMainModal
// Player.ts:2012-2016, openChatModal :2048-2052, openSideModal
// :2072-2076, openMainSideModal :2098-2102):
//
//	// clear old suspended scripts
//	if (this.activeScript?.execution === ScriptState.COUNTDIALOG ||
//	    this.activeScript?.execution === ScriptState.PAUSEBUTTON) {
//	    this.activeScript = null;
//	    this.resumeButtons = [];
//	}
//
// A suspended COUNTDIALOG or PAUSEBUTTON activeScript (and its pending
// resume buttons) is dropped when a new modal opens.
func TestOpenModalClearsSuspendedDialogAndResumeButtons(t *testing.T) {
	cases := []struct {
		name string
		exec script.Execution
		open func(p *Player)
	}{
		{
			name: "OpenMain_CountDialog_survives",
			exec: script.CountDialog,
			open: func(p *Player) { p.OpenMain(100) },
		},
		{
			name: "OpenMain_PauseButton_survives",
			exec: script.PauseButton,
			open: func(p *Player) { p.OpenMain(100) },
		},
		{
			name: "OpenChat_CountDialog_survives",
			exec: script.CountDialog,
			open: func(p *Player) { p.OpenChat(200) },
		},
		{
			name: "OpenChat_PauseButton_survives",
			exec: script.PauseButton,
			open: func(p *Player) { p.OpenChat(200) },
		},
		{
			name: "OpenSide_CountDialog_survives",
			exec: script.CountDialog,
			open: func(p *Player) { p.OpenSide(300) },
		},
		{
			name: "OpenSide_PauseButton_survives",
			exec: script.PauseButton,
			open: func(p *Player) { p.OpenSide(300) },
		},
		{
			name: "OpenMainModalSide_CountDialog_survives",
			exec: script.CountDialog,
			open: func(p *Player) { p.OpenMainModalSide(400, 401) },
		},
		{
			name: "OpenMainModalSide_PauseButton_survives",
			exec: script.PauseButton,
			open: func(p *Player) { p.OpenMainModalSide(400, 401) },
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newTestPlayer(t)
			p.activeScript = &script.ScriptState{Execution: tc.exec}
			p.resumeButtons = []int{101, 102}
			tc.open(p)
			if p.activeScript != nil {
				t.Errorf("%s: suspended dialog script must be cleared on modal open (TS 93ef2d7f @2e3bcf43)", tc.name)
			}
			if len(p.resumeButtons) != 0 {
				t.Errorf("%s: resumeButtons must be cleared on modal open (TS 2dc4a811 @2e3bcf43); got %v", tc.name, p.resumeButtons)
			}
		})
	}
}

func TestAddXPLevelUpUnbuffedAdvancesLevels(t *testing.T) {
	// Un-buffed (levels == baseLevels) player levels up: TS advances BOTH
	// levels and baseLevels in lockstep so the stat display updates.
	// Matches TS Player.ts:1760-1763.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2         // un-buffed
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → 1800, crosses 1740 = level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("levels: got %d, want 3 (un-buffed, advanced with baseLevels)",
			p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPLevelUpWhileDrained(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 1         // drained below base
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	// Replenish: levels + (afterBase - beforeBase) = 1 + (3 - 2) = 2.
	if p.levels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("levels: got %d, want 2 (drained, replenished by 1)", p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPMultiLevelUpUnbuffed(t *testing.T) {
	// Un-buffed player jumps 9 levels in one call: both stats advance in
	// lockstep to the new level. Matches TS Player.ts:1760-1763.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 0
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1          // un-buffed
	p.AddXP(objtype.PlayerStatAttack, 11540, false) // GetExpByLevel(10)
	if p.baseLevels[objtype.PlayerStatAttack] != 10 {
		t.Errorf("baseLevels: got %d, want 10", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 10 {
		t.Errorf("levels: got %d, want 10 (un-buffed, advanced with baseLevels)",
			p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPClampsAtMaxXP(t *testing.T) {
	// XP accumulation cap is MaxXP (200m real = 2B ×10), NOT MaxSkillXP
	// (the level-99 threshold, 13m real). Matches TS Player.ts:1754-1757.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = int32(objtype.MaxXP - 10)
	p.baseLevels[objtype.PlayerStatAttack] = 99
	p.levels[objtype.PlayerStatAttack] = 99
	p.AddXP(objtype.PlayerStatAttack, 1000, false)
	if int(p.stats[objtype.PlayerStatAttack]) != objtype.MaxXP {
		t.Errorf("stats: got %d, want MaxXP %d",
			p.stats[objtype.PlayerStatAttack], objtype.MaxXP)
	}
	if p.baseLevels[objtype.PlayerStatAttack] != 99 {
		t.Errorf("baseLevels: got %d, want 99 (capped)", p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPAccumulatesPastLevel99ThresholdUpToMaxXP(t *testing.T) {
	// A level-99 player keeps accumulating XP past MaxSkillXP up to MaxXP.
	// Prestige / XP-chase gameplay depends on this.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = int32(objtype.MaxSkillXP) // at level-99 threshold
	p.baseLevels[objtype.PlayerStatAttack] = 99
	p.levels[objtype.PlayerStatAttack] = 99
	p.AddXP(objtype.PlayerStatAttack, 1000000, false) // 100k real XP past level 99
	want := int32(objtype.MaxSkillXP + 1000000)
	if p.stats[objtype.PlayerStatAttack] != want {
		t.Errorf("stats: got %d, want %d (accumulation past level-99 threshold)",
			p.stats[objtype.PlayerStatAttack], want)
	}
	// Level stays at 99.
	if p.baseLevels[objtype.PlayerStatAttack] != 99 {
		t.Errorf("baseLevels: got %d, want 99", p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPBuffedLevelUpPreservesBuff(t *testing.T) {
	// Buffed player (levels > baseLevels, e.g. super-strength) levels up:
	// TS only advances baseLevels; levels stays (buff preserved).
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 5         // buffed by +3
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 5 {
		t.Errorf("levels: got %d, want 5 (buff preserved across level-up)",
			p.levels[objtype.PlayerStatAttack])
	}
}

// TestAddXP_NegativeInputIsNoop pins player-core-3: TS Player.addXp
// (Player.ts:1742-1744) throws on negative xp BEFORE any stat
// mutation. goscape previously fell through to the min/clamp math,
// which silently REDUCED stored XP (50 → 0 here). Post-fix the entity
// method early-returns on negative input, leaving stats/baseLevels/
// levels untouched (closest in-method analog to TS's pre-mutation
// throw — the full script-error surface is a deferred deviation at
// handleStatAdvance). The prior pin, TestAddXPNegativeClampsAtZero,
// pinned the BUG (asserted stats=0); renamed and re-pointed here.
func TestAddXP_NegativeInputIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 50
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1
	p.AddXP(objtype.PlayerStatAttack, -100, false) // TS would throw; goscape now early-returns
	if got := p.stats[objtype.PlayerStatAttack]; got != 50 {
		t.Errorf("stats: got %d, want 50 (TS Player.ts:1742 throws before mutation; entity must early-return on negative)", got)
	}
	if got := p.baseLevels[objtype.PlayerStatAttack]; got != 1 {
		t.Errorf("baseLevels: got %d, want 1 (no mutation on negative xp)", got)
	}
	if got := p.levels[objtype.PlayerStatAttack]; got != 1 {
		t.Errorf("levels: got %d, want 1 (no mutation on negative xp)", got)
	}
}

func TestAddXPOOBIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	var before [21]int32
	copy(before[:], p.stats[:])
	p.AddXP(-1, 100, false)
	p.AddXP(21, 100, false)
	p.AddXP(100, 100, false)
	for i := range 21 {
		if p.stats[i] != before[i] {
			t.Errorf("OOB AddXP mutated stats[%d]: got %d, want %d", i, p.stats[i], before[i])
		}
	}
}

// containsSessionLogEvent returns true if any entry in logs has the given Event
// at the LoggerEventTypeAdventure level. Used by AddXP session-log tests.
func containsSessionLogEvent(logs []SessionLog, want string) bool {
	for _, lg := range logs {
		if lg.EventType == LoggerEventTypeAdventure && lg.Event == want {
			return true
		}
	}
	return false
}

// TestAddXPLevelUpEmitsAdventureLog pins the Levelled-up entry: TS Player.ts:1775.
// Single level-up (Attack 2 → 3) emits exactly one ADVENTURE entry with the
// expected message; no milestone/p2p/f2p messages fire at this small total.
func TestAddXPLevelUpEmitsAdventureLog(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → level 3

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1", got)
	}
	lg := s.sessionLogs[0]
	if lg.EventType != LoggerEventTypeAdventure {
		t.Errorf("EventType: got %d, want %d", lg.EventType, LoggerEventTypeAdventure)
	}
	if lg.Event != "Levelled up attack from 2 to 3" {
		t.Errorf("Event: got %q, want %q", lg.Event, "Levelled up attack from 2 to 3")
	}
}

// TestAddXPMultiLevelUpEmitsSingleLevelledUpMessage pins that a multi-level
// jump (1 → 10 in one AddXP call) emits exactly ONE Levelled-up message
// spanning the whole jump, not 9 separate messages. TS Player.ts:1775
// computes one before/after pair per AddXP regardless of level delta.
func TestAddXPMultiLevelUpEmitsSingleLevelledUpMessage(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	p.stats[objtype.PlayerStatAttack] = 0
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1
	p.AddXP(objtype.PlayerStatAttack, 11540, false) // GetExpByLevel(10) → level 10

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1 (single Levelled-up for multi-level jump)", got)
	}
	if lg := s.sessionLogs[0]; lg.Event != "Levelled up attack from 1 to 10" {
		t.Errorf("Event: got %q, want %q", lg.Event, "Levelled up attack from 1 to 10")
	}
}

// TestAddXPLevelUpCrossingMilestoneEmitsMilestone pins the milestone-250
// branch: total goes 249 → 250 crosses milestone-1, emits "Reached total
// level 250" alongside the Levelled-up entry (TS Player.ts:1791-1796).
// Fixture: 3 other stats summing to 247 + Attack at 2 (enabled total = 249);
// AddXP → Attack 3 → total = 250.
func TestAddXPLevelUpCrossingMilestoneEmitsMilestone(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	p.baseLevels[objtype.PlayerStatDefence] = 99
	p.baseLevels[objtype.PlayerStatStrength] = 99
	p.baseLevels[objtype.PlayerStatHitpoints] = 49
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → 1800, level 3

	if got := len(s.sessionLogs); got != 2 {
		t.Fatalf("sessionLogs: got %d, want 2 (Levelled-up + milestone)", got)
	}
	if !containsSessionLogEvent(s.sessionLogs, "Levelled up attack from 2 to 3") {
		t.Errorf("missing Levelled-up entry; logs = %+v", s.sessionLogs)
	}
	if !containsSessionLogEvent(s.sessionLogs, "Reached total level 250") {
		t.Errorf("missing milestone entry; logs = %+v", s.sessionLogs)
	}
}

// TestAddXPLevelUpNoMilestoneWithinBucket pins the milestone gate's lower
// edge: total goes 251 → 252 (within bucket [250, 500)), no milestone
// entry fires — only the Levelled-up entry. Guards against an off-by-one
// in the `currMilestone > prevMilestone` comparison.
func TestAddXPLevelUpNoMilestoneWithinBucket(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	p.baseLevels[objtype.PlayerStatDefence] = 99
	p.baseLevels[objtype.PlayerStatStrength] = 99
	p.baseLevels[objtype.PlayerStatHitpoints] = 51
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // total 251 → 252

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1 (Levelled-up only, no milestone within bucket)", got)
	}
	if lg := s.sessionLogs[0]; lg.Event != "Levelled up attack from 2 to 3" {
		t.Errorf("Event: got %q, want Levelled-up only", lg.Event)
	}
}

// TestAddXPLevelUpHitting1881EmitsP2PAndF2P pins both exact-equality
// sentinels: total = 1881 implies freeTotal = 1485 by construction, so
// BOTH "you beat p2p!" and "you beat f2p!" fire (TS Player.ts:1797-1802).
// Milestone-250 does NOT fire here: prevMilestone = 1880/250 = 7,
// currMilestone = 1881/250 = 7, no crossing.
func TestAddXPLevelUpHitting1881EmitsP2PAndF2P(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	// Set all 19 enabled stats to 99 except Attack at 98.
	for i := range objtype.PlayerStatCount {
		if !objtype.PlayerStatEnabled[i] {
			continue
		}
		if i == objtype.PlayerStatAttack {
			p.stats[i] = int32(objtype.GetExpByLevel(98))
			p.baseLevels[i] = 98
			p.levels[i] = 98
		} else {
			p.baseLevels[i] = 99
		}
	}
	delta := objtype.GetExpByLevel(99) - objtype.GetExpByLevel(98)
	p.AddXP(objtype.PlayerStatAttack, delta, false) // → Attack 99, total = 1881

	if !containsSessionLogEvent(s.sessionLogs, "Levelled up attack from 98 to 99") {
		t.Errorf("missing Levelled-up entry; logs = %+v", s.sessionLogs)
	}
	if !containsSessionLogEvent(s.sessionLogs, "Reached total level 1881 - you beat p2p!") {
		t.Errorf("missing p2p entry; logs = %+v", s.sessionLogs)
	}
	if !containsSessionLogEvent(s.sessionLogs, "Reached total level 1485 - you beat f2p!") {
		t.Errorf("missing f2p entry; logs = %+v", s.sessionLogs)
	}
	// No milestone-250 crossing: 1880 → 1881, both in bucket-7.
	for _, lg := range s.sessionLogs {
		if lg.Event == "Reached total level 1750" || lg.Event == "Reached total level 2000" {
			t.Errorf("unexpected milestone-250 entry: %q", lg.Event)
		}
	}
}

// TestAddXPLevelUpHitting1485F2POnlyEmitsF2POnly pins the f2p sentinel
// independent of p2p: freeTotal = 1485 but total ≠ 1881. Fixture: 15 f2p
// stats sum to 1485 (14 others at 99 + Attack 98→99), 4 members-only
// enabled stats at 1 → total = 1489.
func TestAddXPLevelUpHitting1485F2POnlyEmitsF2POnly(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	// 14 non-Attack f2p stats at 99.
	for _, idx := range []int{
		objtype.PlayerStatDefence, objtype.PlayerStatStrength,
		objtype.PlayerStatHitpoints, objtype.PlayerStatRanged,
		objtype.PlayerStatPrayer, objtype.PlayerStatMagic,
		objtype.PlayerStatCooking, objtype.PlayerStatWoodcutting,
		objtype.PlayerStatFishing, objtype.PlayerStatFiremaking,
		objtype.PlayerStatCrafting, objtype.PlayerStatSmithing,
		objtype.PlayerStatMining, objtype.PlayerStatRunecraft,
	} {
		p.baseLevels[idx] = 99
	}
	// 4 members-only enabled stats at 1 — keep total clear of 1881.
	for _, idx := range []int{
		objtype.PlayerStatFletching, objtype.PlayerStatHerblore,
		objtype.PlayerStatAgility, objtype.PlayerStatThieving,
	} {
		p.baseLevels[idx] = 1
	}
	p.stats[objtype.PlayerStatAttack] = int32(objtype.GetExpByLevel(98))
	p.baseLevels[objtype.PlayerStatAttack] = 98
	p.levels[objtype.PlayerStatAttack] = 98
	delta := objtype.GetExpByLevel(99) - objtype.GetExpByLevel(98)
	p.AddXP(objtype.PlayerStatAttack, delta, false) // → Attack 99, freeTotal = 1485, total = 1489

	if !containsSessionLogEvent(s.sessionLogs, "Reached total level 1485 - you beat f2p!") {
		t.Errorf("missing f2p entry; logs = %+v", s.sessionLogs)
	}
	if containsSessionLogEvent(s.sessionLogs, "Reached total level 1881 - you beat p2p!") {
		t.Errorf("unexpected p2p entry; logs = %+v", s.sessionLogs)
	}
}

// TestAddXPDisabledStatNotInTotal pins the PlayerStatEnabled gate inside
// the total/freeTotal accumulation loop. Fixture: 3 enabled non-Attack
// stats summing to 247 + Attack at 2 (enabled-only total = 249); disabled
// stats[18] and stats[19] set to 99 each. AddXP → Attack 3 → enabled total
// = 250, milestone-1 crosses, "Reached total level 250" fires.
// An incorrect impl summing disabled stats would compute 447 → 448 (both
// in bucket-1) and emit no milestone — this test would catch that.
func TestAddXPDisabledStatNotInTotal(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	p.baseLevels[objtype.PlayerStatDefence] = 99
	p.baseLevels[objtype.PlayerStatStrength] = 99
	p.baseLevels[objtype.PlayerStatHitpoints] = 49
	p.baseLevels[objtype.PlayerStat18] = 99 // disabled
	p.baseLevels[objtype.PlayerStat19] = 99 // disabled
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → Attack 3, enabled total = 250

	if !containsSessionLogEvent(s.sessionLogs, "Reached total level 250") {
		t.Errorf("missing milestone-250 entry (disabled stats erroneously counted?); logs = %+v",
			s.sessionLogs)
	}
}

func TestEnqueueScriptFileDirectPath(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "[test_direct]"}
	p.EnqueueScriptFile(sf, 3, []int{42}, nil, script.QueueNormal)
	if len(p.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(p.queue))
	}
	req := p.queue[0]
	if req.Script != sf {
		t.Errorf("queue[0].Script: got %v, want %v", req.Script, sf)
	}
	if req.Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3", req.Delay)
	}
	if !slices.Equal(req.IntArgs, []int{42}) {
		t.Errorf("queue[0].IntArgs: got %v, want [42]", req.IntArgs)
	}
	if req.StringArgs != nil {
		t.Errorf("queue[0].StringArgs: got %v, want nil", req.StringArgs)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[0].Type: got %v, want %v", req.Type, script.QueueNormal)
	}
}

func TestEnqueueScriptFileNilIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.EnqueueScriptFile(nil, 0, nil, nil, script.QueueNormal)
	if len(p.queue) != 0 {
		t.Errorf("queue len after nil enqueue: got %d, want 0", len(p.queue))
	}
}

// TestUnlinkQueuedScriptDropsMatchingEntries pins the basic filter
// behavior: enqueue 3 scripts at distinct IDs, unlink the middle one,
// assert the remaining two are preserved in original order. NAI-161 T1.
func TestUnlinkQueuedScriptDropsMatchingEntries(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	sf20 := &script.ScriptFile{Name: "[test_id20]"}
	sf30 := &script.ScriptFile{Name: "[test_id30]"}
	s.scriptProvider.Register(sf10) // id=0
	s.scriptProvider.Register(sf20) // id=1
	s.scriptProvider.Register(sf30) // id=2

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf20, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf30, 0, nil, nil, script.QueueNormal)

	p.UnlinkQueuedScript(1) // id=1 → sf20

	if len(p.queue) != 2 {
		t.Fatalf("queue len: got %d, want 2", len(p.queue))
	}
	if p.queue[0].Script != sf10 {
		t.Errorf("queue[0].Script: got %v, want sf10", p.queue[0].Script)
	}
	if p.queue[1].Script != sf30 {
		t.Errorf("queue[1].Script: got %v, want sf30", p.queue[1].Script)
	}
}

// TestUnlinkQueuedScriptWalksAllNonEngineTypes pins the TS-faithful
// default-NORMAL arm: walks BOTH queue and weakQueue, regardless of
// Type discriminator. Engine entries live in p.engineQueue (separate
// slice) and are untouched. NAI-161 T1 — deviation
// NAI-161-D-QUEUE-TYPE-MAPPING.
func TestUnlinkQueuedScriptWalksAllNonEngineTypes(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueWeak)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueStrong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueLong)

	p.UnlinkQueuedScript(0)

	if len(p.queue) != 0 {
		t.Errorf("queue len after unlink: got %d, want 0 (all 4 types should match)", len(p.queue))
	}
}

// TestUnlinkQueuedScriptLeavesEngineQueueIntact pins that the
// engineQueue (separate slice) is NOT walked by the default-NORMAL
// arm of unlinkQueuedScript. NAI-161 T1.
func TestUnlinkQueuedScriptLeavesEngineQueueIntact(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueEngine)

	p.UnlinkQueuedScript(0)

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (normal entry should be dropped)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Errorf("engineQueue len: got %d, want 1 (engine entry must be preserved)", len(p.engineQueue))
	}
}

// TestUnlinkQueuedScriptNilServerIsNoop pins the defensive guard:
// a Player with no client.server (or no scriptProvider) does not
// panic and is a no-op. Mirrors EnqueueScriptArgs defensive shape at
// player_script.go:127. NAI-161 T1 — deviation
// NAI-161-D-CLEARQUEUE-NIL-PROVIDER.
func TestUnlinkQueuedScriptNilServerIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	// p.client.server is nil by default — newTestPlayer doesn't wire a Server.
	p.UnlinkQueuedScript(99)
	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0", len(p.queue))
	}
}

// TestUnlinkQueuedScriptUnknownIDIsNoop pins TS-equivalent "scriptId
// has no matches → zero iterations": when GetByID returns nil for an
// out-of-range scriptID, the queue is unchanged. NAI-161 T1.
func TestUnlinkQueuedScriptUnknownIDIsNoop(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)

	p.UnlinkQueuedScript(99) // id=99 is out of range

	if len(p.queue) != 1 {
		t.Errorf("queue len: got %d, want 1 (bogus scriptID is no-op)", len(p.queue))
	}
}

// TestQueueCountIncludesAllQueueTypes pins TS GETQUEUE semantics: walks
// BOTH queue.all() AND weakQueue.all() (PlayerOps.ts:903-920). Goscape's
// unified p.queue holds all four types; the loop counts all matches
// regardless of Type. NAI-161 T2.
func TestQueueCountIncludesAllQueueTypes(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueStrong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueLong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueWeak)

	got := p.QueueCount(0)
	if got != 4 {
		t.Errorf("QueueCount(0): got %d, want 4 (Normal+Strong+Long+Weak all counted)", got)
	}
}

// TestQueueCountExcludesEngineQueue pins that engineQueue is a
// separate slice and is never counted by QueueCount. NAI-161 T2.
func TestQueueCountExcludesEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueEngine)

	got := p.QueueCount(0)
	if got != 1 {
		t.Errorf("QueueCount(0): got %d, want 1 (engine entry excluded)", got)
	}
}

// TestQueueCountUnknownIDReturnsZero pins that an out-of-range
// scriptID resolves to nil → returns 0. Mirrors TS finding zero
// matches in the same scenario. NAI-161 T2.
func TestQueueCountUnknownIDReturnsZero(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)

	got := p.QueueCount(99)
	if got != 0 {
		t.Errorf("QueueCount(99): got %d, want 0 (bogus scriptID)", got)
	}
}

// TestQueueCountNilServerReturnsZero pins the defensive guard.
// NAI-161 T2 — deviation NAI-161-D-CLEARQUEUE-NIL-PROVIDER.
func TestQueueCountNilServerReturnsZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	// p.client.server is nil by default.
	got := p.QueueCount(99)
	if got != 0 {
		t.Errorf("QueueCount on nil-server player: got %d, want 0", got)
	}
}

func TestAddXPFiresChangeStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [changestat,attack=0] — keyed by trigger(165) | (0x2<<8) | (0<<10).
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{
		Name:      "[changestat,attack]",
		LookupKey: key,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	// NAI-144: changeStat now uses QueueEngine, not QueueNormal.
	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → level 3

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (changestat must NOT land in primary queue post-NAI-144)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue+1 {
		t.Fatalf("p.engineQueue len: got %d, want %d (+1 changestat via QueueEngine)",
			len(p.engineQueue), beforeEngineQueue+1)
	}
	req := p.engineQueue[beforeEngineQueue]
	if req.Script != sf {
		t.Errorf("p.engineQueue[%d].Script: got %v, want [changestat,attack] (%v)", beforeEngineQueue, req.Script, sf)
	}
	if req.Type != script.QueueEngine {
		t.Errorf("p.engineQueue[%d].Type: got %v, want QueueEngine (NAI-144 closes S6h deviation)", beforeEngineQueue, req.Type)
	}
}

func TestAddXPDoesNotFireChangeStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[changestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold (830)
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 100, false) // → 200, still level 1 (< 830)

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (no level-up = no changestat fire)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (no level-up = no changestat fire)",
			len(p.engineQueue), beforeEngineQueue)
	}
}

func TestAddXPChangeStatNoScriptIsNoop(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty — no changestat script registered

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // level up, but no script registered

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (no registered script = silent no-op)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (no registered script = silent no-op)",
			len(p.engineQueue), beforeEngineQueue)
	}
	// Verify the level-up math still happened.
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3 (level-up math independent of changeStat)",
			p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPFiresAdvanceStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [advancestat,attack=0] at the type-specific lookup key.
	key := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{
		Name:      "[advancestat,attack]",
		LookupKey: key,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	// NAI-144: advanceStat now uses QueueEngine, not QueueNormal.
	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // → level 3

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (advancestat must NOT land in primary queue post-NAI-144)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue+1 {
		t.Fatalf("p.engineQueue len: got %d, want %d (+1 advancestat via QueueEngine)",
			len(p.engineQueue), beforeEngineQueue+1)
	}
	req := p.engineQueue[beforeEngineQueue]
	if req.Script != sf {
		t.Errorf("p.engineQueue[%d].Script: got %v, want [advancestat,attack] (%v)", beforeEngineQueue, req.Script, sf)
	}
	if req.Type != script.QueueEngine {
		t.Errorf("p.engineQueue[%d].Type: got %v, want QueueEngine (NAI-144 closes S6h deviation)", beforeEngineQueue, req.Type)
	}
}

func TestAddXPDoesNotFireAdvanceStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[advancestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 100, false) // → 200, still level 1

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (no level-up = no advancestat fire)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (no level-up = no advancestat fire)",
			len(p.engineQueue), beforeEngineQueue)
	}
}

func TestAddXPAdvanceStatNoFallbackToGlobal(t *testing.T) {
	// Register a GLOBAL [advancestat,_] script. AdvanceStat uses
	// GetByTriggerSpecific which does NOT fall back, so the global script
	// should NOT fire on a per-skill level-up.
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalKey := script.LookupKeyForGlobal(script.TriggerAdvanceStat)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[advancestat,_]", LookupKey: globalKey})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // level up

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (global script must NOT fire — advancestat is type-specific only)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (global script must NOT fire — advancestat is type-specific only)",
			len(p.engineQueue), beforeEngineQueue)
	}
	// Verify level-up math still happened.
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3 (level-up math independent of advancestat fire)",
			p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPFiresBothChangeAndAdvanceStatOnLevelUp(t *testing.T) {
	// Both triggers should enqueue when both scripts are registered.
	// Validates that S6h's changeStat and S6i's advanceStat coexist
	// AND that they fire in TS order (changeStat first, advanceStat second).
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	changeKey := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	advKey := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	changeSF := &script.ScriptFile{Name: "[changestat,attack]", LookupKey: changeKey}
	advSF := &script.ScriptFile{Name: "[advancestat,attack]", LookupKey: advKey}
	s.scriptProvider.Register(changeSF)
	s.scriptProvider.Register(advSF)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	// NAI-144: both changeStat and advanceStat now use QueueEngine.
	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000, false) // level up

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (neither changestat nor advancestat must land in primary queue post-NAI-144)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue+2 {
		t.Fatalf("p.engineQueue len: got %d, want %d (+2 — both changestat and advancestat via QueueEngine)",
			len(p.engineQueue), beforeEngineQueue+2)
	}
	// Order: changeStat before advanceStat (matches TS Player.ts:1772, 1804).
	if p.engineQueue[beforeEngineQueue].Script != changeSF {
		t.Errorf("p.engineQueue[%d].Script: got %v, want changestat first", beforeEngineQueue, p.engineQueue[beforeEngineQueue].Script)
	}
	if p.engineQueue[beforeEngineQueue+1].Script != advSF {
		t.Errorf("p.engineQueue[%d].Script: got %v, want advancestat second", beforeEngineQueue+1, p.engineQueue[beforeEngineQueue+1].Script)
	}
}

func TestPlayerLowMemoryGetter(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.LowMemory() {
		t.Errorf("default: want LowMemory()=false, got true")
	}
	p.lowMemory = true
	if !p.LowMemory() {
		t.Errorf("after set: want LowMemory()=true, got false")
	}
}

// TestNormalizeSongNameB3 pins normalizeSongName (B3 TS-faithful):
// lowercase + spaces→underscores + strip /[^a-z0-9_-]/g.
// TS ref: Player.ts:1922 at 244.
func TestNormalizeSongNameB3(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Harmony 1", "harmony_1"},
		{"already_lower", "already_lower"},
		{"ALLCAPS", "allcaps"},
		{"Mixed CASE With Spaces", "mixed_case_with_spaces"},
		// Strip: special chars removed after lowercase+underscore step.
		{"Scape Main!", "scape_main"},
		{"church music 1", "church_music_1"},
		{"quest.complete", "questcomplete"},
		{"a-b_c", "a-b_c"}, // hyphens and underscores are allowed
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeSongName(tc.in); got != tc.want {
				t.Errorf("normalizeSongName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeSongNameEmptyReturnsEmpty(t *testing.T) {
	if got := normalizeSongName(""); got != "" {
		t.Errorf("normalizeSongName(\"\") = %q, want \"\"", got)
	}
}

// TestPlaySong_EmptyNameIsNoOp pins that PlaySong("") is a no-op
// (empty name guard fires after normalization, before registry lookup).
func TestPlaySong_EmptyNameIsNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlaySong("")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("empty name: PlaySong wrote %d bytes; want 0", n)
	}
}

// TestPlaySong_NilServerIsNoOp pins that PlaySong with a non-empty name
// is silent when p.client.server is nil (bare test player).
// Mirrors TS Player.ts:1921 `if (id !== -1)` guard — nil server degrades
// to id==-1 posture. Player.ts:1919-1925 at 244.
func TestPlaySong_NilServerIsNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	// p.client.server is nil; PlaySong must not panic and must write nothing.
	p.PlaySong("adventure")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong(nil server) wrote %d bytes; want 0", n)
	}
}

// TestNormalizeJingleNameB3 pins normalizeJingleName (B3 TS-faithful):
// lowercase ONLY — no underscore conversion.
// TS ref: Player.ts:1929 at 244 (name.toLowerCase()).
func TestNormalizeJingleNameB3(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Underscores are preserved (TS does NOT convert them to spaces).
		{"a_quick_jingle", "a_quick_jingle"},
		// Spaces are preserved as-is (only casing changes).
		{"Space Already", "space already"},
		{"ALLCAPS", "allcaps"},
		{"Mixed_CASE_With_Underscores", "mixed_case_with_underscores"},
		// Real pack keys (from Content/pack/midi.pack): spaces preserved.
		{"Sailing Journey", "sailing journey"},
		{"Quest Complete 1", "quest complete 1"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeJingleName(tc.in); got != tc.want {
				t.Errorf("normalizeJingleName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeJingleNameEmptyReturnsEmpty(t *testing.T) {
	if got := normalizeJingleName(""); got != "" {
		t.Errorf("normalizeJingleName(\"\") = %q, want \"\"", got)
	}
}

// TestPlayJingle_EmptyNameIsNoOp pins that PlayJingle("", ...) is a
// no-op (empty-name guard fires after normalization, before registry lookup).
func TestPlayJingle_EmptyNameIsNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("empty name: PlayJingle wrote %d bytes; want 0", n)
	}
}

// TestPlayJingle_NilServerIsNoOp pins that PlayJingle with a non-empty name
// is silent when p.client.server is nil (bare test player).
// Mirrors TS Player.ts:1929 `if (id !== -1)` guard — nil server degrades
// to id==-1 posture. Player.ts:1928-1933 at 244.
func TestPlayJingle_NilServerIsNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "fanfare")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlayJingle(nil server) wrote %d bytes; want 0", n)
	}
}

// TestClearWeakQueueRemovesOnlyWeakEntries pins (*Player).clearWeakQueue:
// drops every QueueWeak entry from p.queue, preserves relative order
// of remaining entries. Mirrors TS Player.weakQueue.clear() (Player.ts:743).
func TestClearWeakQueueRemovesOnlyWeakEntries(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueNormal},
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueLong},
	}

	p.clearWeakQueue()

	if got, want := len(p.queue), 3; got != want {
		t.Fatalf("queue len after clearWeakQueue: got %d, want %d", got, want)
	}
	wantTypes := []script.PlayerQueueType{
		script.QueueStrong, script.QueueNormal, script.QueueLong,
	}
	for i, want := range wantTypes {
		if got := p.queue[i].Type; got != want {
			t.Errorf("queue[%d].Type: got %v, want %v (order must be preserved)", i, got, want)
		}
	}
}

// TestClearWeakQueueEmptyQueueNoOp pins clearWeakQueue is safe on empty queue.
func TestClearWeakQueueEmptyQueueNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.queue = nil

	p.clearWeakQueue()

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0", len(p.queue))
	}
}

// TestClearWeakQueueAllWeakEntries pins clearWeakQueue empties a queue
// of all-weak entries.
func TestClearWeakQueueAllWeakEntries(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueWeak},
	}

	p.clearWeakQueue()

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (all weak entries should be removed)", len(p.queue))
	}
}

// TestClearWeakQueueIdempotent pins repeated clearWeakQueue is a no-op
// after the first call.
func TestClearWeakQueueIdempotent(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.clearWeakQueue()
	p.clearWeakQueue()

	if got, want := len(p.queue), 1; got != want {
		t.Errorf("queue len after 2× clearWeakQueue: got %d, want %d", got, want)
	}
	if p.queue[0].Type != script.QueueStrong {
		t.Errorf("queue[0].Type: got %v, want QueueStrong", p.queue[0].Type)
	}
}

func TestPlayerTeleportCrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.Teleport(4000, 4000, 0)
	newZone := s.zoneMap.Get(0, 4000, 4000)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount after Teleport: got %d, want 0", prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount after Teleport: got %d, want 1", newZone.PlayersCount())
	}
}

func TestPlayerTeleJumpCrossLevelRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.TeleJump(3200, 3200, 1) // same xy, level=0→1
	newZone := s.zoneMap.Get(1, 3200, 3200)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount after cross-level TeleJump: got %d, want 0", prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount after cross-level TeleJump: got %d, want 1", newZone.PlayersCount())
	}
}

func TestPlayerTeleportSameZoneNoRefresh(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	prevElement := p.zoneListElement
	p.Teleport(3201, 3201, 0) // same zone (400, 400)
	if z.PlayersCount() != 1 {
		t.Errorf("same-zone Teleport PlayersCount: got %d, want 1", z.PlayersCount())
	}
	// Same-zone teleport should NOT re-subscribe (no leave/enter dance).
	if p.zoneListElement != prevElement {
		t.Error("same-zone Teleport should preserve zoneListElement (no leave/enter)")
	}
}

// --- NAI-36 Task 7: Player.Teleport partial parity ----------------------
//
// Closes NAI-34-D1 (level clamp), NAI-34-D2 (unallocated-zone reject),
// body-order alignment to refresh-then-tele, and NAI-34-D5 (level-change
// INSTANT + jump branch) for Player. Mirrors TS PathingEntity.teleport
// at PathingEntity.ts:267-298.

// TestPlayerTeleport_LevelClampNegative pins D1: level=-1 clamps to 0
// per TS PathingEntity.ts:268-271 (TS uses Math.max(0, ...)).
func TestPlayerTeleport_LevelClampNegative(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	p.Teleport(3210, 3310, -1)

	if p.level != 0 {
		t.Errorf("level after Teleport(level=-1): got %d, want 0 (clamp)", p.level)
	}
	if p.x != 3210 || p.z != 3310 {
		t.Errorf("x/z after Teleport: got (%d, %d), want (3210, 3310)", p.x, p.z)
	}
}

// TestPlayerTeleport_LevelClampHigh pins D1 upper bound: level=4 clamps
// to 3 per TS PathingEntity.ts:271 (TS uses Math.min(level, 3)).
func TestPlayerTeleport_LevelClampHigh(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	p.Teleport(3210, 3310, 4)

	if p.level != 3 {
		t.Errorf("level after Teleport(level=4): got %d, want 3 (clamp)", p.level)
	}
}

// TestPlayerTeleport_UnallocatedZoneRejects pins D2: a teleport to a zone
// where IsZoneAllocated returns false is silently ignored — no coord
// mutation, no tele flag write. Per TS PathingEntity.ts:273-278.
func TestPlayerTeleport_UnallocatedZoneRejects(t *testing.T) {
	s := newTestServer(t)
	// Wire a real gamemap so IsZoneAllocated returns false for any
	// un-allocated zone (test default: all zones unallocated).
	s.gamemap = gamemap.New(discardLogger())
	// Allocate the starting zone so the player can be placed there.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3300, 0)

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.tele = false

	// Target zone (3210, 3310) is NOT allocated → reject.
	p.Teleport(3210, 3310, 0)

	if p.x != prevX || p.z != prevZ || p.level != prevLevel {
		t.Errorf("Teleport to unallocated zone: state changed (%d,%d,%d) → (%d,%d,%d), want unchanged",
			prevX, prevZ, prevLevel, p.x, p.z, p.level)
	}
	if p.tele {
		t.Errorf("tele flag: got true, want false (rejected teleport must not set flag)")
	}
}

// TestPlayerTeleport_AllocatedZoneAccepts pins the D2 positive case: a
// teleport to a zone where IsZoneAllocated returns true completes
// normally. Pairs with TestPlayerTeleport_UnallocatedZoneRejects to
// guard against a degenerate "always-reject" implementation.
func TestPlayerTeleport_AllocatedZoneAccepts(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3300, 0)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3210, 3310, 0)

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	p.Teleport(3210, 3310, 0)

	if p.x != 3210 || p.z != 3310 || p.level != 0 {
		t.Errorf("Teleport to allocated zone: got (%d,%d,%d), want (3210,3310,0)",
			p.x, p.z, p.level)
	}
	if !p.tele {
		t.Error("tele flag: got false, want true (accepted teleport must set flag)")
	}
}

// TestPlayerTeleport_SameLevelNoMoveSpeedChange pins the D5 negative
// case: a same-level teleport leaves moveSpeed/jump untouched per TS
// PathingEntity.ts:295 (the `previousLevel != level` guard).
func TestPlayerTeleport_SameLevelNoMoveSpeedChange(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.moveSpeed = MoveSpeedWalk
	p.jump = false

	p.Teleport(3210, 3310, 0) // same level (0 → 0)

	if p.moveSpeed != MoveSpeedWalk {
		t.Errorf("same-level moveSpeed: got %v, want MoveSpeedWalk (unchanged)", p.moveSpeed)
	}
	if p.jump {
		t.Errorf("same-level jump: got true, want false (unchanged)")
	}
}

// TestPlayerTeleport_LevelChangeSetsInstantAndJump pins the D5 positive
// case: a level-change teleport sets moveSpeed=Instant + jump=true per
// TS PathingEntity.ts:295-298.
func TestPlayerTeleport_LevelChangeSetsInstantAndJump(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.moveSpeed = MoveSpeedWalk
	p.jump = false

	p.Teleport(3210, 3310, 1) // level changed 0 → 1

	if p.moveSpeed != MoveSpeedInstant {
		t.Errorf("level-change moveSpeed: got %v, want MoveSpeedInstant", p.moveSpeed)
	}
	if !p.jump {
		t.Errorf("level-change jump: got false, want true")
	}
}

// TestPlayerTeleport_OrderRefreshThenFlag pins the body-order alignment.
// Note: refreshPlayerZone reads only previous + current coords, never
// p.tele; and p.tele = true never reads zone state. The two writes are
// runtime-commutative — order is purely structural and invisible at the
// observable-state layer. This test is a behavior witness rather than a
// strict order-pin: it asserts that BOTH effects are applied (refresh
// happened AND tele=true) so a regression that drops one would surface.
// Source-level order is enforced by the doc comment on Teleport plus
// code review; the runtime-equivalent claim is documented at
// modules/world/player_script.go in the Teleport doc-block. NAI-36-T7.
func TestPlayerTeleport_OrderRefreshThenFlag(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.tele = false

	p.Teleport(4000, 4000, 0) // cross-zone so refresh actually runs

	newZone := s.zoneMap.Get(0, 4000, 4000)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("refresh effect missing: prevZone PlayersCount=%d, want 0",
			prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("refresh effect missing: newZone PlayersCount=%d, want 1",
			newZone.PlayersCount())
	}
	if !p.tele {
		t.Errorf("tele flag write missing: got false, want true")
	}
}

// --- NAI-37 Task 5: HintNpc payload byte-pin test --------------------------

// TestHintNpcPayloadBytes pins the type=1 HintArrow encoder branch
// byte-for-byte. nid=0x1234 chosen so each byte position is
// distinguishable from the zero-fill (catches type-byte regression,
// nid endianness, and field misordering — see rsbuf_roundtrip_tests.md).
// OpHintArrow has PayloadSize=6 (fixed), so the wire is
// [encrypted_opcode, p1(type=1), p2(nid_hi), p2(nid_lo), p2(0)_hi, p2(0)_lo, p1(0)] — 7 bytes total.
func TestHintNpcPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0x01,       // p1: type = 1 (NPC hint)
		0x12, 0x34, // p2: nid (big-endian)
		0x00, 0x00, // p2: 0 (unused playerSlot for type=1)
		0x00, // p1: 0 (unused y for type=1)
	}

	received := drainConn(t, cc)
	p.HintNpc(0x1234)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintNpc(0x1234) wire: got %#x, want %#x", got, want)
	}
}

// --- NAI-39 Task 2: HintCoord / HintPlayer / HintStop byte-pin tests -------

// TestHintCoordPayloadBytes pins the type=2..6 (TILE) HintArrow encoder
// branch byte-for-byte. Per HintArrowEncoder.ts:17-27 the wire shape is
// p1(type=offset), p2(x), p2(z), p1(height). The encoder name "y" is
// the script-author-facing "height".
func TestHintCoordPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0x03,       // p1: type=offset=3
		0x12, 0x34, // p2: x = 0x1234
		0x56, 0x78, // p2: z = 0x5678
		0x42, // p1: height=0x42
	}

	received := drainConn(t, cc)
	p.HintCoord(3, 0x1234, 0x5678, 0x42)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintCoord(3, 0x1234, 0x5678, 0x42) wire: got %#x, want %#x", got, want)
	}
}

// TestHintCoordOffsetBoundaries pins both ends of the TILE-branch range
// (offset=2 = far-left, offset=6 = top-left). Both must produce well-formed
// 6-byte payloads with the offset in byte[0] post-encryption.
func TestHintCoordOffsetBoundaries(t *testing.T) {
	for _, offset := range []int{2, 6} {
		t.Run(fmt.Sprintf("offset=%d", offset), func(t *testing.T) {
			p, cc := newTestPlayer(t)
			enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
			p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

			want := []byte{
				byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
				byte(offset), // p1: type=offset
				0x00, 0x01,   // p2: x=1
				0x00, 0x02, // p2: z=2
				0x00, // p1: height=0
			}

			received := drainConn(t, cc)
			p.HintCoord(offset, 1, 2, 0)
			p.client.flushWrite()
			got := <-received
			if !bytes.Equal(got, want) {
				t.Errorf("HintCoord(%d,1,2,0) wire: got %#x, want %#x", offset, got, want)
			}
		})
	}
}

// TestHintPlayerPayloadBytes pins the type=10 (PL) HintArrow encoder
// branch byte-for-byte. Per HintArrowEncoder.ts:28-32 the wire shape is
// p1(0x0A), p2(playerSlot), p2(0), p1(0). slot=0xABCD chosen so each
// byte position is distinguishable from the zero-fill.
func TestHintPlayerPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0x0A,       // p1: type = 10 (player hint)
		0xAB, 0xCD, // p2: slot=0xABCD (big-endian)
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}

	received := drainConn(t, cc)
	p.HintPlayer(0xABCD)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintPlayer(0xABCD) wire: got %#x, want %#x", got, want)
	}
}

// TestHintStopPayloadBytes pins the type=-1 (STOP) HintArrow encoder
// branch byte-for-byte. Per HintArrowEncoder.ts:33-38 the wire shape is
// p1(-1), p2(0), p2(0), p1(0). p1(-1) is 0xFF on the wire (low byte of
// two's-complement). The 0xFF asymmetry is the conspicuous-pin per
// ts_asymmetry_dual_pin.md.
func TestHintStopPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0xFF,       // p1: type = -1 sentinel (two's-complement low byte)
		0x00, 0x00, // p2: 0
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}

	received := drainConn(t, cc)
	p.HintStop()
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintStop() wire: got %#x, want %#x", got, want)
	}
}

// TestPlayerOnScriptFinishedOrAborted_MatchNoMain pins the player-path
// Finished/Aborted tail where state matches activeScript and no MAIN
// modal is open: activeScript is nulled and CloseModal(false) fires.
// Mirrors TS Player.ts:2143-2148. NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_MatchNoMain(t *testing.T) {
	p, _ := newTestPlayer(t)
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "match-no-main"}}
	p.activeScript = state
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(state)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (state matched and was cleared)")
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want %#x (CloseModal must reset)", p.modalState, modalStateNone)
	}
	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (CloseModal fired)")
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (CloseModal must reset slot)", p.modalChat)
	}
}

// TestPlayerOnScriptFinishedOrAborted_MatchWithMain pins the
// MAIN-modal-preserving branch: activeScript clears but CloseModal does
// NOT fire because (modalState & MAIN) != NONE. Mirrors TS Player.ts:2146.
// NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_MatchWithMain(t *testing.T) {
	p, _ := newTestPlayer(t)
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "match-with-main"}}
	p.activeScript = state
	p.modalState = modalStateMain | modalStateChat
	p.modalMain = 200
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(state)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (state matched and was cleared)")
	}
	if p.modalState != modalStateMain|modalStateChat {
		t.Errorf("modalState: got %#x, want %#x (MAIN bit set must skip CloseModal)",
			p.modalState, modalStateMain|modalStateChat)
	}
	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (CloseModal must not fire with MAIN open)")
	}
	if p.modalMain != 200 {
		t.Errorf("modalMain: got %d, want 200 (slot must be preserved)", p.modalMain)
	}
	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (slot must be preserved)", p.modalChat)
	}
}

// TestPlayerOnScriptFinishedOrAborted_Mismatch pins the guard: when the
// supplied state is NOT p.activeScript, activeScript is preserved and
// CloseModal does not fire. Closes the silent Suspended-clobber bug.
// NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_Mismatch(t *testing.T) {
	p, _ := newTestPlayer(t)
	stored := &script.ScriptState{Script: &script.ScriptFile{Name: "stored"}}
	other := &script.ScriptState{Script: &script.ScriptFile{Name: "other"}}
	p.activeScript = stored
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(other)

	if p.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p", p.activeScript, stored)
	}
	if p.modalState != modalStateChat {
		t.Errorf("modalState: got %#x, want %#x (mismatch must not fire CloseModal)",
			p.modalState, modalStateChat)
	}
	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (mismatch must not fire CloseModal)")
	}
}

// TestPlayerOnScriptFinishedOrAborted_NilActive pins the nil-active
// guard: p.activeScript == nil + non-nil arg → no-op (no panic, no
// state change). NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_NilActive(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.activeScript = nil
	other := &script.ScriptState{Script: &script.ScriptFile{Name: "other"}}
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(other) // must not panic

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil")
	}
	if p.modalState != modalStateChat {
		t.Errorf("modalState: got %#x, want %#x (no-op)", p.modalState, modalStateChat)
	}
	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (no-op)")
	}
}

// TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant pins TS
// PathingEntity.focus (PathingEntity.ts:321-333). instant=false sets
// faceAngleX/Z only — does NOT touch faceSquareX/Z or masks.
// instant=true ALSO writes faceSquareX = fineX, faceSquareZ = fineZ,
// and ORs MaskFaceCoord into masks.
//
// Per ts_asymmetry_dual_pin.md: dual-pin both branches. The
// instant=false absence-pin escalates if upstream changes the focus()
// shape; the instant=true presence-pin escalates if the wire writes
// regress.
func TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant(t *testing.T) {
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.faceAngleX = -1
	p.faceAngleZ = -1
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	// instant=false — faceAngle written; faceSquare/mask untouched.
	p.focus(123, 456, false)
	if p.faceAngleX != 123 || p.faceAngleZ != 456 {
		t.Errorf("instant=false faceAngle: got (%d, %d), want (123, 456)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("instant=false faceSquare: got (%d, %d), want (-1, -1) unchanged", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks != 0 {
		t.Errorf("instant=false masks: got %d, want 0 unchanged", p.masks)
	}

	// instant=true — faceAngle written; faceSquare = (fx, fz);
	// MaskFaceCoord ORed in.
	p.focus(789, 1011, true)
	if p.faceAngleX != 789 || p.faceAngleZ != 1011 {
		t.Errorf("instant=true faceAngle: got (%d, %d), want (789, 1011)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != 789 || p.faceSquareZ != 1011 {
		t.Errorf("instant=true faceSquare: got (%d, %d), want (789, 1011)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&rsbuf.MaskFaceCoord == 0 {
		t.Errorf("instant=true masks: MaskFaceCoord bit not set (masks=%d)", p.masks)
	}
}

// TestPlayerTeleport_FocusFromDirection pins NAI-65 D3-Player. Teleport
// from (3200, 3200, 0) to (3300, 3300, 0): direction is NE, so MoveX/MoveZ
// each return prevDest+1. faceAngleX = Fine(3301, 1) = 3301*2 + 1 = 6603.
// Mirrors TS PathingEntity.ts:286-289 + TS CoordGrid.ts:125-127.
func TestPlayerTeleport_FocusFromDirection(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.faceAngleX = -1
	p.faceAngleZ = -1

	p.Teleport(3300, 3300, 0)

	wantX := 3301*2 + 1
	wantZ := 3301*2 + 1
	if p.faceAngleX != wantX {
		t.Errorf("faceAngleX after Teleport(NE): got %d, want %d (Fine(3301, 1))", p.faceAngleX, wantX)
	}
	if p.faceAngleZ != wantZ {
		t.Errorf("faceAngleZ after Teleport(NE): got %d, want %d (Fine(3301, 1))", p.faceAngleZ, wantZ)
	}
}

// TestPlayerTeleport_LastStepAdjust pins NAI-65 D4-Player. After Teleport,
// p.lastStepX = p.x - 1 and p.lastStepZ = p.z per TS PathingEntity.ts:291-292.
func TestPlayerTeleport_LastStepAdjust(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.lastStepX = -999
	p.lastStepZ = -999

	p.Teleport(3300, 3300, 0)

	if p.lastStepX != 3299 {
		t.Errorf("lastStepX after Teleport: got %d, want 3299 (x - 1)", p.lastStepX)
	}
	if p.lastStepZ != 3300 {
		t.Errorf("lastStepZ after Teleport: got %d, want 3300 (z)", p.lastStepZ)
	}
}

// TestPlayerTeleJump_LastStepAdjust pins the rev-254 A7 contract: TeleJump
// routes through the shared teleport tail, so lastStep = (x-1, z) — TS
// PathingEntity.ts:313-314 @2e3bcf43 (teleport writes it after
// refreshZonePresence; TS has ALWAYS done this — the old goscape "stale
// lastStep on TeleJump" posture died when lastStep moved inside
// refreshZonePresence at f0ccbe8a).
func TestPlayerTeleJump_LastStepAdjust(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.lastStepX = -999
	p.lastStepZ = -999

	p.TeleJump(3300, 3300, 0)

	if p.lastStepX != 3299 {
		t.Errorf("lastStepX after TeleJump: got %d, want 3299 (x - 1)", p.lastStepX)
	}
	if p.lastStepZ != 3300 {
		t.Errorf("lastStepZ after TeleJump: got %d, want 3300 (z)", p.lastStepZ)
	}
}

// TestPlayerTeleport_InPlaceFocusUsesSelfCenter pins the in-place edge case.
// When prev == new, coordgrid.Face returns -1; coordgrid.MoveX/MoveZ no-op
// (DeltaX/Z default-case = 0). focus uses self-center coords:
// Fine(p.x, 1), Fine(p.z, 1). lastStep adjust still applies (x-1, z).
func TestPlayerTeleport_InPlaceFocusUsesSelfCenter(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.faceAngleX = -1
	p.faceAngleZ = -1

	p.Teleport(3200, 3200, 0)

	wantSelf := 3200*2 + 1
	if p.faceAngleX != wantSelf {
		t.Errorf("in-place faceAngleX: got %d, want %d (Fine(3200, 1) self-center)", p.faceAngleX, wantSelf)
	}
	if p.faceAngleZ != wantSelf {
		t.Errorf("in-place faceAngleZ: got %d, want %d (Fine(3200, 1) self-center)", p.faceAngleZ, wantSelf)
	}
	if p.lastStepX != 3199 {
		t.Errorf("in-place lastStepX: got %d, want 3199 (x - 1 still applies)", p.lastStepX)
	}
	if p.lastStepZ != 3200 {
		t.Errorf("in-place lastStepZ: got %d, want 3200", p.lastStepZ)
	}
	if !p.tele {
		t.Error("in-place tele flag: got false, want true")
	}
}

// TestOpenTutorial_SetsFieldsWithoutClosingOthers pins TS-fidelity:
// opening the tutorial overlay does NOT close any other modal.
// TS Player.ts:1999-2003 — `this.modalState |= ModalState.TUT;
// this.modalTutorial = com;`. No clear of modalMain/Chat/Side.
func TestOpenTutorial_SetsFieldsWithoutClosingOthers(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.encryptor = enc
	p.modalMain = 5
	p.modalChat = 7
	p.modalSide = 9
	p.modalState = modalStateMain | modalStateChat | modalStateSide

	p.OpenTutorial(42)

	if p.modalTutorial != 42 {
		t.Errorf("modalTutorial: got %d, want 42", p.modalTutorial)
	}
	wantState := modalStateMain | modalStateChat | modalStateSide | modalStateTut
	if p.modalState != wantState {
		t.Errorf("modalState: got %#x, want %#x", p.modalState, wantState)
	}
	if p.modalMain != 5 {
		t.Errorf("modalMain: got %d, want 5 (must not be cleared)", p.modalMain)
	}
	if p.modalChat != 7 {
		t.Errorf("modalChat: got %d, want 7 (must not be cleared)", p.modalChat)
	}
	if p.modalSide != 9 {
		t.Errorf("modalSide: got %d, want 9 (must not be cleared)", p.modalSide)
	}
}

// TestOpenTutorial_RefreshFlagsUntouched pins that OpenTutorial does
// NOT touch the refreshModal/refreshModalClose flags; those remain
// reserved for the main/chat/side modal switch in encodeOut.
// (Pre-NAI-112 OpenTutorial deferred via the lastModalTutorial diff;
// post-NAI-112 OpenTutorial writes the wire packet directly per TS
// Player.ts:1999-2003 — neither shape involved the refresh flags.)
func TestOpenTutorial_RefreshFlagsUntouched(t *testing.T) {
	enc, _ := isaacPair([4]uint32{5, 6, 7, 8})
	p, _ := newTestPlayer(t)
	p.client.encryptor = enc
	p.refreshModal = false
	p.refreshModalClose = false

	p.OpenTutorial(42)

	if p.refreshModal {
		t.Error("refreshModal should remain false after OpenTutorial")
	}
	if p.refreshModalClose {
		t.Error("refreshModalClose should remain false after OpenTutorial")
	}
}

// TestPlaySynthWritesOut pins NAI-87 T3: (*Player).PlaySynth issues
// a writeOut to the client for the OpSynthSound opcode. Failure
// signal = "wire-out broken or encoder mis-wired."
func TestPlaySynthWritesOut(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlaySynth(123, 1, 0)
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlaySynth wrote 0 bytes to c.bufw; want >0 (NAI-87 positive pin)")
	}
}

// TestHasInteraction_NoTarget pins (*Player).HasInteraction → false when
// there is no interaction target. NAI-120 Bundle 2B; mirrors TS
// Player.hasInteraction Player.ts:955-957.
func TestHasInteraction_NoTarget(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.HasInteraction() {
		t.Error("HasInteraction with nil target: got true, want false")
	}
}

// TestHasInteraction_NormalInteraction pins HasInteraction → true for
// a non-follow interaction (e.g., targetOp=1 / OPPLAYER1).
func TestHasInteraction_NormalInteraction(t *testing.T) {
	p, _ := newTestPlayer(t)
	other, _ := newTestPlayer(t)
	p.target = other
	p.targetOp = 1
	if !p.HasInteraction() {
		t.Error("HasInteraction with normal player interaction: got false, want true")
	}
}

// TestHasInteraction_FollowOp_Excluded pins TS-faithful exclusion of the
// follow interaction (targetOp=3 with player target = APPLAYER3/OPPLAYER3).
// Mirrors TS Player.ts:959-962 — "the follow interaction doesn't do
// anything" so HasInteraction reports false. NAI-120 Bundle 2B.
func TestHasInteraction_FollowOp_Excluded(t *testing.T) {
	p, _ := newTestPlayer(t)
	other, _ := newTestPlayer(t)
	p.target = other
	p.targetOp = 3
	if p.HasInteraction() {
		t.Error("HasInteraction with follow-op (targetOp=3, player target): got true, want false (TS Player.ts:959-962)")
	}
}

// TestHasInteraction_Op3_NonPlayerTarget pins that the follow-op exclusion
// only applies to player targets. targetOp=3 against a non-player (e.g., NPC)
// is a regular interaction; reports true. The isFollowOp helper at
// interaction.go:145-151 narrows to *Player targets.
func TestHasInteraction_Op3_NonPlayerTarget(t *testing.T) {
	p, _ := newTestPlayer(t)
	npc := &Npc{}
	p.target = npc
	p.targetOp = 3
	if !p.HasInteraction() {
		t.Error("HasInteraction with op=3 against non-player target: got false, want true (follow-op narrows to *Player)")
	}
}

// TestChangeStatUsesQueueEngine is a direct regression fence for the
// NAI-144 migration: changeStat must enqueue to p.engineQueue with
// Type=QueueEngine, not the previous S6h QueueNormal-as-ENGINE shape.
func TestChangeStatUsesQueueEngine(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{Name: "[changestat,attack]", LookupKey: key}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.changeStat(objtype.PlayerStatAttack)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (changeStat must NOT land in primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (changeStat uses QueueEngine)", len(p.engineQueue))
	}
	if p.engineQueue[0].Type != script.QueueEngine {
		t.Errorf("Type: got %v, want QueueEngine", p.engineQueue[0].Type)
	}
	if p.engineQueue[0].Script != sf {
		t.Errorf("Script: got %v, want %v", p.engineQueue[0].Script, sf)
	}
}

// TestAdvanceStatUsesQueueEngine pins NAI-144 migration: advanceStat
// uses QueueEngine to match TS Player.ts:1804-1807.
func TestAdvanceStatUsesQueueEngine(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{Name: "[advancestat,attack]", LookupKey: key}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.advanceStat(objtype.PlayerStatAttack)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (advanceStat must NOT land in primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (advanceStat uses QueueEngine)", len(p.engineQueue))
	}
	if p.engineQueue[0].Type != script.QueueEngine {
		t.Errorf("Type: got %v, want QueueEngine", p.engineQueue[0].Type)
	}
	if p.engineQueue[0].Script != sf {
		t.Errorf("Script: got %v, want %v", p.engineQueue[0].Script, sf)
	}
}

// --- NAI-181: LAST_LOGIN_INFO server packet byte-pin tests -----------------

// TestLastLoginInfo_FirstCall_EmitsExactByteSequence pins all 5 encoder
// fields' positions, ordering, and endianness for the first-call branch
// (lastLoginTime==0 → daysSinceLogin==0). 244 wire: p4+p2+p1+p2+pbool = 10 bytes.
// TS LastLoginInfoEncoder.ts (244): p4(lastLoginIp) p2(daysSinceLogin)
// p1(daysSinceRecoveryChange) p2(unreadMessageCount) pbool(warnMembersInNonMembers).
func TestLastLoginInfo_FirstCall_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.messageCount = 0x0007
	p.lastLoginTime = 0 // first-call branch
	// p.members=false (default); p.client.server=nil → warnMembersInNonMembers=false → 0x00.

	want := []byte{
		byte((int(gameserver.OpLastLoginInfo.Opcode) + int(enc.GetNext())) & 0xff),
		0x7F, 0x00, 0x00, 0x01, // p4: lastIp = 2130706433 (127.0.0.1)
		0x00, 0x00, // p2: daysSinceLogin = 0 (first-call branch)
		0xC9,       // p1: daysSinceRecoveriesChanged = 201
		0x00, 0x07, // p2: messageCount = 7
		0x00, // pbool: warnMembersInNonMembers = false
	}
	received := drainConn(t, cc)
	p.LastLoginInfo()
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("LastLoginInfo first-call wire: got %#x, want %#x", got, want)
	}
}

// TestLastLoginInfo_SubsequentCall_DaysSinceLoginAdvances pins the
// integer-truncation formula and non-zero lastDate branch. Sets
// lastLoginTime to 5 days + 100 ms ago; asserts bytes[5:7] decode as
// daysSinceLogin >= 5 && <= 6 (tolerant for CI jitter).
// 244 wire: opcode(1) + p4+p2+p1+p2+pbool = 11 bytes total.
func TestLastLoginInfo_SubsequentCall_DaysSinceLoginAdvances(t *testing.T) {
	const dayMillis = int64(1000 * 60 * 60 * 24)
	p, cc := newTestPlayer(t)
	_, dec := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.lastLoginTime = time.Now().UnixMilli() - 5*dayMillis - 100

	received := drainConn(t, cc)
	p.LastLoginInfo()
	p.client.flushWrite()
	got := <-received

	// Decrypt the opcode byte and skip it, then read bytes[1:] as the payload.
	// Wire layout: got[0]=enc-opcode, got[1..4]=lastIp p4, got[5..6]=daysSinceLogin p2,
	// got[7]=daysSinceRecoveries, got[8..9]=messageCount p2, got[10]=warnMembersInNonMembers pbool.
	if len(got) < 11 {
		t.Fatalf("wire too short: got %d bytes, want 11", len(got))
	}
	// Skip the opcode; decrypt it to verify it matches OpLastLoginInfo.
	encOpcode := int(got[0])
	decOpcode := (encOpcode - int(dec.GetNext())) & 0xff
	if decOpcode != int(gameserver.OpLastLoginInfo.Opcode) {
		t.Errorf("opcode: got %d, want %d", decOpcode, gameserver.OpLastLoginInfo.Opcode)
	}
	// got[0]=encrypted opcode; payload at got[1..10]
	// got[1..4]=lastIp p4, got[5..6]=daysSinceLogin p2, got[7]=daysSinceRecoveries,
	// got[8..9]=messageCount p2, got[10]=warnMembersInNonMembers pbool
	daysSinceLogin := int(got[5])<<8 | int(got[6])
	if daysSinceLogin < 5 || daysSinceLogin > 6 {
		t.Errorf("daysSinceLogin: got %d, want 5 or 6", daysSinceLogin)
	}
}

// TestLastLoginInfo_UpdatesLastLoginTime pins the lastLoginTime field
// update (mirrors TS Player.ts:2199 `this.lastLoginTime = nextDate`).
func TestLastLoginInfo_UpdatesLastLoginTime(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	before := time.Now().UnixMilli()
	received := drainConn(t, cc)
	p.LastLoginInfo()
	p.client.flushWrite()
	<-received
	after := time.Now().UnixMilli()

	if p.lastLoginTime < before || p.lastLoginTime > after {
		t.Errorf("lastLoginTime: got %d, want in [%d, %d]", p.lastLoginTime, before, after)
	}
}

// TestLastLoginInfo_MessageCountSerialization pins bytes [8:10] as
// big-endian p2 for messageCount=0xABCD. Disambiguates messageCount
// slot from daysSinceRecoveriesChanged endianness regression.
// 244 wire: opcode(1) + 10 payload bytes = 11 total.
func TestLastLoginInfo_MessageCountSerialization(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.messageCount = 0xABCD
	p.lastLoginTime = 0

	received := drainConn(t, cc)
	p.LastLoginInfo()
	p.client.flushWrite()
	got := <-received

	if len(got) < 11 {
		t.Fatalf("wire too short: got %d bytes, want 11", len(got))
	}
	// got[0] = encrypted opcode; payload bytes at got[1..10]
	// messageCount p2 is at payload[7..8] → got[8..9]
	if got[8] != 0xAB || got[9] != 0xCD {
		t.Errorf("messageCount bytes: got %#x %#x, want 0xAB 0xCD", got[8], got[9])
	}
}

// TestLastLoginInfo_WarnMembersInNonMembers pins the 10th payload byte
// (got[10]): pbool(warnMembersInNonMembers). Derivation mirrors TS
// Player.ts:2197 `!Environment.NODE_MEMBERS && this.members`:
// when NodeMembers=false and p.members=true → warn=true → 0x01.
// When NodeMembers=true, warn=false regardless of p.members.
func TestLastLoginInfo_WarnMembersInNonMembers(t *testing.T) {
	t.Run("non-members-world+members-player=warn", func(t *testing.T) {
		p, cc := newTestPlayer(t)
		s := newTestServer(t)
		s.cfg.NodeMembers = false // non-members world
		p.client.server = s
		p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
		p.members = true
		p.lastLoginTime = 0

		received := drainConn(t, cc)
		p.LastLoginInfo()
		p.client.flushWrite()
		got := <-received

		if len(got) < 11 {
			t.Fatalf("wire too short: %d bytes", len(got))
		}
		if got[10] != 0x01 {
			t.Errorf("warnMembersInNonMembers byte: got 0x%02x, want 0x01", got[10])
		}
	})
	t.Run("members-world+members-player=no-warn", func(t *testing.T) {
		p, cc := newTestPlayer(t)
		s := newTestServer(t)
		s.cfg.NodeMembers = true // members world
		p.client.server = s
		p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
		p.members = true
		p.lastLoginTime = 0

		received := drainConn(t, cc)
		p.LastLoginInfo()
		p.client.flushWrite()
		got := <-received

		if len(got) < 11 {
			t.Fatalf("wire too short: %d bytes", len(got))
		}
		if got[10] != 0x00 {
			t.Errorf("warnMembersInNonMembers byte: got 0x%02x, want 0x00", got[10])
		}
	})
}

// TestPlayer_InvTotalParamStack pins the sum formula. Build a player
// with an inv containing two slots: {id=10, count=3} + {id=20, count=5}.
// paramType(7).defaultInt=0; obj 10 has Params[7]=100 (uint32);
// obj 20 has Params[7]=50 (uint32). Expected: 3*100 + 5*50 = 550.
//
// Adaptation vs plan: helpers newTestPlayerWithInv/invSlotFixture do
// not exist; test builds the fixture inline matching the
// buildRunWeightServer pattern from player_runweight_test.go.
func TestPlayer_InvTotalParamStack(t *testing.T) {
	const invID = 5
	const paramID = 7

	invConfigs := make([]*objtype.InvType, invID+1)
	invConfigs[invID] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: invID},
		Scope:      objtype.InvTypeScopeTemp,
		Size:       10,
	}
	objConfigs := make([]*objtype.ObjType, 21)
	objConfigs[10] = &objtype.ObjType{
		Params: objtype.ParamMap{uint32(paramID): uint32(100)},
	}
	objConfigs[20] = &objtype.ObjType{
		Params: objtype.ParamMap{uint32(paramID): uint32(50)},
	}
	paramConfigs := make([]*objtype.ParamType, paramID+1)
	paramConfigs[paramID] = &objtype.ParamType{DefaultInt: 0}

	p, _ := newTestPlayer(t)
	p.client.server = &Server{
		log:        discardLogger(),
		invTypes:   &objtype.InvTypeConfigs{Configs: invConfigs},
		objTypes:   &objtype.ObjTypeConfigs{Configs: objConfigs},
		paramTypes: &objtype.ParamTypeConfigs{Configs: paramConfigs},
	}
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(invID, 10, inventory.StackNormal)
	inv.Items[0] = &inventory.Item{Id: 10, Count: 3}
	inv.Items[1] = &inventory.Item{Id: 20, Count: 5}
	p.invs[invID] = inv

	got := p.InvTotalParamStack(invID, paramID)
	if got != 550 {
		t.Errorf("InvTotalParamStack: got %d, want 550", got)
	}
}

// TestPlayer_InvTotalParamStack_EmptyInv pins zero return on empty inv.
func TestPlayer_InvTotalParamStack_EmptyInv(t *testing.T) {
	const invID = 5
	const paramID = 7

	invConfigs := make([]*objtype.InvType, invID+1)
	invConfigs[invID] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: invID},
		Scope:      objtype.InvTypeScopeTemp,
		Size:       5,
	}
	paramConfigs := make([]*objtype.ParamType, paramID+1)
	paramConfigs[paramID] = &objtype.ParamType{DefaultInt: 0}

	p, _ := newTestPlayer(t)
	p.client.server = &Server{
		log:        discardLogger(),
		invTypes:   &objtype.InvTypeConfigs{Configs: invConfigs},
		objTypes:   &objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{}},
		paramTypes: &objtype.ParamTypeConfigs{Configs: paramConfigs},
	}
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	p.invs[invID] = inventory.New(invID, 5, inventory.StackNormal)

	if got := p.InvTotalParamStack(invID, paramID); got != 0 {
		t.Errorf("empty inv: got %d, want 0", got)
	}
}

// TestPlayer_InvTotalParamStack_NilClient pins nil-client returns zero.
func TestPlayer_InvTotalParamStack_NilClient(t *testing.T) {
	p := &Player{}
	if got := p.InvTotalParamStack(999, 7); got != 0 {
		t.Errorf("nil client: got %d, want 0", got)
	}
}

// TestPlayer_AddWealthEvent pins append-to-log behaviour. Two events
// append to the log in order.
func TestPlayer_AddWealthEvent(t *testing.T) {
	p := &Player{}
	e1 := script.WealthEvent{EventType: script.WealthEventTypeDrop, AccountValue: 1000}
	e2 := script.WealthEvent{EventType: script.WealthEventTypePVP, AccountValue: 5000}
	p.AddWealthEvent(e1)
	p.AddWealthEvent(e2)
	if got := len(p.wealthLog); got != 2 {
		t.Fatalf("len(wealthLog): got %d, want 2", got)
	}
	if p.wealthLog[0].AccountValue != 1000 || p.wealthLog[1].AccountValue != 5000 {
		t.Errorf("wealthLog values: got %v", p.wealthLog)
	}
}

func TestSetStat_WritesBaseCurAndXPClamped(t *testing.T) {
	cases := []struct {
		name    string
		level   int
		wantLvl uint8
		wantXP  int32
	}{
		{"normal mid", 50, 50, int32(objtype.GetExpByLevel(50))},
		{"clamps to 1 from 0", 0, 1, int32(objtype.GetExpByLevel(1))},
		{"clamps to 1 from -5", -5, 1, int32(objtype.GetExpByLevel(1))},
		{"clamps to 99 from 100", 100, 99, int32(objtype.GetExpByLevel(99))},
		{"clamps to 99 from 150", 150, 99, int32(objtype.GetExpByLevel(99))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{}
			p.SetStat(objtype.PlayerStatAttack, tc.level)
			if p.baseLevels[objtype.PlayerStatAttack] != tc.wantLvl {
				t.Errorf("baseLevels = %d, want %d", p.baseLevels[objtype.PlayerStatAttack], tc.wantLvl)
			}
			if p.levels[objtype.PlayerStatAttack] != tc.wantLvl {
				t.Errorf("levels = %d, want %d", p.levels[objtype.PlayerStatAttack], tc.wantLvl)
			}
			if p.stats[objtype.PlayerStatAttack] != tc.wantXP {
				t.Errorf("stats = %d, want %d", p.stats[objtype.PlayerStatAttack], tc.wantXP)
			}
		})
	}
}

func TestSetStat_OOBStatDropsSilently(t *testing.T) {
	p := &Player{}
	p.SetStat(-1, 50)
	p.SetStat(21, 50)
	// No state mutation expected, no panic.
	for i := 0; i < objtype.PlayerStatCount; i++ {
		if p.baseLevels[i] != 0 || p.levels[i] != 0 || p.stats[i] != 0 {
			t.Errorf("stat %d mutated after OOB SetStat", i)
		}
	}
}

// TestCalcCombatLevel_* pin the goscape port of TS Player.getCombatLevel
// (Engine-TS/.../Player.ts:1302-1308). The formula uses baseLevels[]
// (not levels[]) — buffs/drains don't move combat level. NAI-184 T1.
//
// "Fresh stats" convention: baseLevels = all 1 except HP = 10, mirroring
// the empty-save bootstrap at player_load.go:79-85.

func TestCalcCombatLevel_FreshStats(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	if got := p.calcCombatLevel(); got != 3 {
		t.Errorf("calcCombatLevel(fresh): got %d, want 3", got)
	}
}

func TestCalcCombatLevel_Maxed(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 99
	}
	if got := p.calcCombatLevel(); got != 126 {
		t.Errorf("calcCombatLevel(maxed): got %d, want 126", got)
	}
}

func TestCalcCombatLevel_PureMelee99(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatAttack] = 99
	p.baseLevels[objtype.PlayerStatStrength] = 99
	if got := p.calcCombatLevel(); got != 67 {
		t.Errorf("calcCombatLevel(att=str=99, rest fresh): got %d, want 67", got)
	}
}

func TestCalcCombatLevel_PureRanged99(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatRanged] = 99
	if got := p.calcCombatLevel(); got != 50 {
		t.Errorf("calcCombatLevel(range=99, rest fresh): got %d, want 50", got)
	}
}

func TestCalcCombatLevel_PureMagic99(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatMagic] = 99
	if got := p.calcCombatLevel(); got != 50 {
		t.Errorf("calcCombatLevel(mage=99, rest fresh): got %d, want 50", got)
	}
}

func TestCalcCombatLevel_PrayerLeveraged(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatDefence] = 99
	p.baseLevels[objtype.PlayerStatHitpoints] = 99
	p.baseLevels[objtype.PlayerStatPrayer] = 99
	if got := p.calcCombatLevel(); got != 62 {
		t.Errorf("calcCombatLevel(def=hp=prayer=99, rest=1): got %d, want 62", got)
	}
}

func TestCalcCombatLevel_UsesBaseLevelsNotLevels(t *testing.T) {
	// Critical regression guard: drinking a strength potion does NOT
	// change combat level. baseLevels is fresh; levels[STR] is boosted.
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
		p.levels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatStrength] = 99 // boosted ONLY in levels, not baseLevels
	if got := p.calcCombatLevel(); got != 3 {
		t.Errorf("calcCombatLevel(potion-boosted): got %d, want 3 (must ignore levels[])", got)
	}
}

// TestRecomputeCombatLevel_* pin the guarded-rebuild semantics.
// Mirrors the inline `if (combatLevel != getCombatLevel()) { ...
// buildAppearance(...); }` blocks at TS Player.ts:1820-1824 and
// 1840-1844 (244 pin). The 244 delta — the rebuild arg changed from
// appearanceInv to InvType.WORN — is pinned by
// TestRecomputeCombatLevel_Change_RebuildTrue_UsesWornInv below.
// NAI-184 T2.

func TestRecomputeCombatLevel_NoChange_NoMaskFlip(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.combatLevel = 3 // matches calcCombatLevel() for these stats
	p.masks = 0
	p.recomputeCombatLevel(true)
	if p.combatLevel != 3 {
		t.Errorf("combatLevel: got %d, want 3 (no-op when value unchanged)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set when CL didn't change")
	}
}

func TestRecomputeCombatLevel_Change_RebuildTrue_FlipsMask(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatAttack] = 99   // att=str=99 → CL 67
	p.baseLevels[objtype.PlayerStatStrength] = 99 // (plan note: str alone gives 35, not 67)
	p.combatLevel = 3                             // stale
	p.appearanceInv = 42                          // arbitrary, must remain unchanged
	p.masks = 0
	p.recomputeCombatLevel(true)
	if p.combatLevel != 67 {
		t.Errorf("combatLevel: got %d, want 67", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("masks: MaskAppearance not set after CL change with triggerRebuild=true")
	}
	if p.appearanceInv != 42 {
		t.Errorf("appearanceInv: got %d, want 42 (must not be reset)", p.appearanceInv)
	}
}

func TestRecomputeCombatLevel_Change_RebuildFalse_NoMaskFlip(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatAttack] = 99   // att=str=99 → CL 67
	p.baseLevels[objtype.PlayerStatStrength] = 99 // (plan note: str alone gives 35, not 67)
	p.combatLevel = 3
	p.masks = 0
	p.recomputeCombatLevel(false)
	if p.combatLevel != 67 {
		t.Errorf("combatLevel: got %d, want 67 (field still updates)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set when triggerRebuild=false")
	}
}

// TestSetStat_RecomputesCombatLevel* pin the SetStat hook into the
// guarded combat-level rebuild. Retires DEVIATION-NAI-184-D1-SETSTAT-
// NO-COMBAT-REBUILD. NAI-184 T3.

func TestSetStat_RecomputesCombatLevelAndFlipsAppearance(t *testing.T) {
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.combatLevel = 3 // fresh
	p.masks = 0
	p.SetStat(objtype.PlayerStatStrength, 99)
	if p.combatLevel <= 3 {
		t.Errorf("combatLevel: got %d, want > 3 after STR→99", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("masks: MaskAppearance not set after combat-stat SetStat")
	}
}

func TestSetStat_NonCombatStat_NoMaskFlip(t *testing.T) {
	// Cooking is not a combat stat; SetStat(cooking, 50) must NOT change
	// combatLevel and must NOT flip MaskAppearance.
	p := &Player{}
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.combatLevel = 3
	p.masks = 0
	p.SetStat(objtype.PlayerStatCooking, 50)
	if p.combatLevel != 3 {
		t.Errorf("combatLevel: got %d, want 3 (non-combat stat must not move CL)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set after non-combat-stat SetStat")
	}
}

// TestAddXP_*CombatLevel pin the AddXP hook into the guarded combat-
// level rebuild. Retires the informal "Does NOT recompute combat
// level (future combat sub-spec)" deferral in AddXP's doc-block.
// NAI-184 T4.

func TestAddXP_LevelUp_RecomputesCombatLevel(t *testing.T) {
	p, _ := newTestPlayer(t)
	// Pre-load fresh baseLevels (newTestPlayer leaves them at zero).
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
		p.levels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatHitpoints] = 10
	// Start STR at level 1 with 0 XP; add enough XP to reach level 99.
	p.stats[objtype.PlayerStatStrength] = 0
	p.baseLevels[objtype.PlayerStatStrength] = 1
	p.levels[objtype.PlayerStatStrength] = 1
	p.combatLevel = 3
	p.masks = 0
	p.AddXP(objtype.PlayerStatStrength, objtype.GetExpByLevel(99), false)
	if p.baseLevels[objtype.PlayerStatStrength] != 99 {
		t.Fatalf("baseLevels[STR]: got %d, want 99 (precondition for CL recompute)",
			p.baseLevels[objtype.PlayerStatStrength])
	}
	if p.combatLevel <= 3 {
		t.Errorf("combatLevel: got %d, want > 3 after STR level-up to 99", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("masks: MaskAppearance not set after level-up combat-stat AddXP")
	}
}

func TestAddXP_NoLevelUp_NoRecompute(t *testing.T) {
	// Adding XP without crossing a level threshold must NOT trigger
	// recomputeCombatLevel — the guard short-circuits on no-change.
	// More importantly, the AddXP code only calls recompute inside the
	// afterBase > beforeBase branch.
	p, _ := newTestPlayer(t)
	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
		p.levels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatHitpoints] = 10
	// Start STR exactly at level 2; add a small amount that stays in [830, 1740).
	// GetExpByLevel(2)=830, GetExpByLevel(3)=1740. +100 → 930, still level 2.
	p.stats[objtype.PlayerStatStrength] = int32(objtype.GetExpByLevel(2))
	p.baseLevels[objtype.PlayerStatStrength] = 2
	p.levels[objtype.PlayerStatStrength] = 2
	p.combatLevel = 3
	p.masks = 0
	p.AddXP(objtype.PlayerStatStrength, 100, false) // → 930 XP, still level 2
	if p.baseLevels[objtype.PlayerStatStrength] != 2 {
		t.Fatalf("baseLevels[STR]: got %d, want 2 (precondition: no level-up)",
			p.baseLevels[objtype.PlayerStatStrength])
	}
	if p.combatLevel != 3 {
		t.Errorf("combatLevel: got %d, want 3 (no level-up → no recompute)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set without level-up")
	}
}

// TestRecomputeCombatLevel_Change_RebuildTrue_UsesWornInv pins TS Player.ts:1821-1824
// and 1841-1843 (rev-244): on combat-level change with triggerRebuild=true,
// buildAppearance must use InvType.WORN (not the current p.appearanceInv).
// 225 called buildAppearance(this.appearanceInv); 244 calls buildAppearance(InvType.WORN).
// Observable: after the rebuild, p.appearanceInv must equal invTypes.Worn,
// even when the player had appearanceInv bound to a different inv id beforehand.
func TestRecomputeCombatLevel_Change_RebuildTrue_UsesWornInv(t *testing.T) {
	const wornId = 0
	const customInvId = 5 // arbitrary non-Worn inv bound before the stat change

	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.invTypes = &objtype.InvTypeConfigs{
		Configs: make([]*objtype.InvType, customInvId+1),
		Worn:    wornId,
	}
	p.client.server = s

	for i := range objtype.PlayerStatCount {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatAttack] = 99 // att=str=99 → CL 67
	p.baseLevels[objtype.PlayerStatStrength] = 99
	p.combatLevel = 3             // stale, differs from calcCombatLevel()
	p.appearanceInv = customInvId // bound to non-Worn inv before the change
	p.masks = 0

	p.recomputeCombatLevel(true)

	// Combat level must update.
	if p.combatLevel != 67 {
		t.Errorf("combatLevel: got %d, want 67", p.combatLevel)
	}
	// MaskAppearance must be set.
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("masks: MaskAppearance not set after CL change")
	}
	// 244 delta: appearanceInv must be updated to invTypes.Worn, not kept at customInvId.
	if p.appearanceInv != wornId {
		t.Errorf("appearanceInv: got %d, want %d (244 must use InvType.WORN on rebuild, was customInvId=%d)",
			p.appearanceInv, wornId, customInvId)
	}
}
