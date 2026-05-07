package world

import (
	"os"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// TestTickPhaseOrder_NpcEventQueueBeforeInteractions pins the NAI-122 fix:
// processNpcEventQueue (which dispatches AI_SPAWN scripts queued by
// Server.addNpc) MUST run before processInteractions (where combat scripts
// read npc varns). Mirrors TS World.ts:356 (processNpcEventQueue) running
// before TS World.ts:376 (processPlayers / interactions).
//
// Pre-fix (buggy) order from NAI-5: processInteractions at tick.go:40 ran
// before processNpcEventQueue at tick.go:42. AI_SPAWN scripts didn't
// populate %npc_combat_xp_multiplier and similar varns until AFTER combat
// had already read the zero-init value, producing the V-PARTIAL parked at
// NAI-120 / NAI-121.
//
// This is a structural pin: it source-scans tick.go to assert call-site
// ordering. Fragile to mass refactors; intentional, since the invariant
// itself is structural.
func TestTickPhaseOrder_NpcEventQueueBeforeInteractions(t *testing.T) {
	src, err := os.ReadFile("tick.go")
	if err != nil {
		t.Fatalf("read tick.go: %v", err)
	}
	text := string(src)
	qIdx := strings.Index(text, "s.processNpcEventQueue()")
	iIdx := strings.Index(text, "s.processInteractions()")
	if qIdx < 0 {
		t.Fatalf("s.processNpcEventQueue() call not found in tick.go")
	}
	if iIdx < 0 {
		t.Fatalf("s.processInteractions() call not found in tick.go")
	}
	if qIdx > iIdx {
		t.Errorf("NAI-122: s.processNpcEventQueue() at offset %d must precede s.processInteractions() at offset %d (TS World.ts:356 < World.ts:376)", qIdx, iIdx)
	}
}

// TestProcessInfo_TeleAcrossWindow_LocalCoordsInRange pins the NAI-93 bug:
// pre-fix, processInfo runs rsbuf.ComputePlayer BEFORE updateMap, so the
// rsbuf-cached Origin is stale on a cross-window tele tick. The PlayerInfo
// tele leaf encodes localX, localZ relative to the stale Origin, producing
// values outside the Java client's 0..104 active-window array bounds.
//
// The fix moves updateMap into processInfo, before ComputePlayer, per TS
// World.ts:996 ("set origin before compute player is why this is above.").
//
// Pre-fix: localZ = 3306 - ((406-6)*8) = 106 — OOB on the Java client's
// [105][105] tile arrays, crashing in getHeightmapY (client.java:2640).
// Post-fix: origin = (2661, 3306) by the time ComputePlayer runs, so
// localX = 2661 - (332-6)*8 = 53; localZ = 3306 - (413-6)*8 = 50. In range.
func TestProcessInfo_TeleAcrossWindow_LocalCoordsInRange(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	// Player at Tutorial Island spawn. setupInfoPlayer pins originX/Z to
	// (x, z), matching the post-processLogins state where origin = spawn.
	p := setupInfoPlayer(t, s, 1, 3094, 3106, 0)

	// First-tick settle: origin pinned to (3094, 3106), rebuiltOnce flips.
	s.processInfo()

	// Tele to Ardougne center — far enough to cross the 13x13 reload window.
	// Mirrors the user-launched smoke `::tele 0,41,51,37,42` reproducer.
	p.x = 2661
	p.z = 3306
	p.tele = true
	p.jump = true

	// Drive one tick of processInfo. Post-fix: updateMap fires inside
	// processInfo BEFORE ComputePlayer, so rsbuf cache holds the FRESH
	// origin (2661, 3306). Pre-fix: rsbuf cache holds STALE (3094, 3106).
	s.processInfo()

	payload := s.rsbuf.PlayerInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)
	if len(payload) == 0 {
		t.Fatal("PlayerInfo.Encode returned empty payload")
	}

	buf := packet.NewPacket(payload)
	buf.AccessBits()

	// Tele leaf shape (pkg/rsbuf/playerinfo.go:131-142):
	//   PBit(1, 1) PBit(2, 3) PBit(2, level) PBit(7, localX) PBit(7, localZ)
	//   PBit(1, jump) PBit(1, extend)
	if got := buf.GBit(1); got != 1 {
		t.Fatalf("hasUpdate bit: got %d, want 1 (local player has update)", got)
	}
	if got := buf.GBit(2); got != 3 {
		t.Fatalf("update kind: got %d, want 3 (tele leaf)", got)
	}
	if got := buf.GBit(2); got != 0 {
		t.Errorf("level bit: got %d, want 0", got)
	}
	localX := int(buf.GBit(7))
	localZ := int(buf.GBit(7))

	// Post-fix pinned values: window base = ((zoneX-6)*8, (zoneZ-6)*8) =
	// (2608, 3256). localX = 2661 - 2608 = 53; localZ = 3306 - 3256 = 50.
	if localX != 53 {
		t.Errorf("localX = %d, want 53 (post-rebuild origin (2661, 3306))", localX)
	}
	if localZ != 50 {
		t.Errorf("localZ = %d, want 50 (post-rebuild origin (2661, 3306))", localZ)
	}
	// Bounds invariant: client tile-flag arrays are [4][105][105]; values
	// >= 105 produce ArrayIndexOutOfBoundsException at client.java:2640.
	if localX < 0 || localX > 104 {
		t.Errorf("localX = %d outside [0, 104] (Java client array bound)", localX)
	}
	if localZ < 0 || localZ > 104 {
		t.Errorf("localZ = %d outside [0, 104] (Java client array bound)", localZ)
	}

	if got := buf.GBit(1); got != 1 {
		t.Errorf("jump bit: got %d, want 1 (p.jump = true)", got)
	}
}
