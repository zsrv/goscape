# NAI-93: RebuildNormal tick-order fix + tele cheat tightening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `updateMap()` from `processClientsOut` into `processInfo` so it runs before `rsbuf.ComputePlayer`, fixing stale-Origin tele leaves that crash the Java client; and tighten the `::tele` cheat handler with the TS pre-tele cleanup chain.

**Architecture:** Bundle 1 ports TS `World.processInfo`'s call order verbatim — for each player, `reorient()` then `updateMap()` then `ComputePlayer`. Bundle 2 replaces the `case "tele":` body with the TS-faithful cleanup sequence (`closeModal`, `canAccess` gate, `clearInteraction`, `unsetMapFlag`) ahead of bounds-check + `TeleJump`.

**Tech Stack:** Go 1.26+ / TS reference at `LostCityRS/Engine-TS/src/engine/World.ts:992-1056` and `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:491-524` / Rust reference at `2004scape/rsbuf` branch 225 (per `rust_source_canonical_path` memory).

**Spec:** `docs/superpowers/specs/2026-05-05-nai-93-rebuild-normal-tick-order-design.md` (commit `9888007`).

---

## Pre-flight (controller, before T1 dispatch)

Per `controller_preflight` memory. Verify against HEAD before each subagent dispatch:

- [ ] **PF-1:** `rg "\.updateMap\(\)" modules/world/` returns exactly two production sites: `modules/world/player.go:816` (the call from `processOut` — to be removed in T2) and zero other production callers. Test files (`modules/world/login_map_test.go:36, 74, 88`) call `p.updateMap()` directly and are unaffected by the move. **Verified at plan-write 2026-05-05.**

- [ ] **PF-2:** `tick.go` `processInfo` body is at lines 340-419 (current HEAD), with the per-player reorient loop at 349-351 (line 350: `p.reorient()`). The rsbuf ComputePlayer push loop is at 386-419. **Verified.**

- [ ] **PF-3:** `processOut` at `modules/world/player.go:815-825` currently calls in order: `updateMap → updatePlayers → updateNpcs → updateZones → updateInvs → updateStats → updateAfkZones → encodeOut → flushWrite`. Removing `updateMap` leaves the rest unchanged. **Verified.**

- [ ] **PF-4:** `handlers_game.go:371-411` `case "tele":` body matches the spec's quoted version (staffModLevel gate → args parse → `ClearInteraction` + `TeleJump`, with the DEVIATION block at 380-387). **Verified.**

- [ ] **PF-5:** Helpers exist at HEAD:
  - `(*Player).CloseModal(clearWeakQueue bool)` at `player_script.go:678`.
  - `(*Player).CanAccess() bool` at `player_script.go:283`.
  - `(*Player).MessageGame(msg string)` at `message_game.go:15`.
  - `(*Player).ClearInteraction()` (used at the existing tele site).
  - `sendUnsetMapFlag(p *Player)` at `interaction.go:44`.
  - `modalState` constants at `player.go:36-39`. **Verified.**

- [ ] **PF-6:** Test infra exists: `newTestServer(t)` at `server_test.go:311`; `newTestPlayer(t)` at `player_test.go:17`; `setupInfoPlayer(t, s, slot, x, z, level)` at `player_info_test.go:12-30`; `isaacPair`, `encryptOpcode`, `discardLogger` available. `s.gamemap = gamemap.New(discardLogger())` + `s.gamemap.Init(t.TempDir())` is the standard setup pattern (see `login_map_test.go:15-19`). **Verified.**

- [ ] **PF-7:** `pkg/io/packet.Packet` exposes `AccessBits()` (`packetbit.go:19`), `GBit(n int) uint8` (`packetbit.go:34`), `NewPacket(buf []byte)` (`buffer.go:373`). **Verified.**

- [ ] **PF-8:** `pkg/rsbuf.PlayerInfo.Encode(b *Buf, pid int32, renderer *Renderer) []byte` returns the bare PlayerInfo payload (no opcode/length prefix); resets scratch buffers internally so multiple calls in one test are safe (`pkg/rsbuf/playerinfo.go:61-103`). **Verified.**

- [ ] **PF-9:** Pre-existing test `TestUpdateMapAnchorsOriginToPlayer` at `login_map_test.go:57-95` exercises updateMap in isolation; its passing state is not invalidated by Bundle 1 (we only move the CALLER, not the function). Bundle 1's commit body should call out that the pre-existing test is preserved. **Verified.**

If any PF check fails at controller-time, abort and re-spec.

---

## Bundle 1 — Tick-order fix

Per spec §4.1. Three tasks; commit after each. Test-first per `superpowers:test-driven-development`.

### Task 1: Failing integration test for stale-Origin tele leaf

**Goal:** Pin the bug — pre-fix, a cross-window tele in one tick produces a tele-leaf with `localZ` outside the client's `[0, 104]` array bound. Test must FAIL on HEAD before T2.

**Files:**
- Create: `modules/world/tick_order_test.go`

- [ ] **Step 1.1: Read the existing test infra to confirm the imports and setup pattern**

Run:
```bash
sed -n '1,10p' modules/world/player_info_test.go
sed -n '12,30p' modules/world/player_info_test.go
```

Expected output: imports `gamemap` and `rsbuf`; `setupInfoPlayer` populates `p.x, p.z, p.level, p.originX, p.originZ` and reserves the rsbuf slot via `s.rsbuf.AddPlayer`.

- [ ] **Step 1.2: Write the failing test**

Create `modules/world/tick_order_test.go` with this exact content:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

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
```

- [ ] **Step 1.3: Run the test — confirm it FAILS pre-fix**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestProcessInfo_TeleAcrossWindow_LocalCoordsInRange ./modules/world/...
```

Expected: FAIL. Pre-fix produces `localX = 5` (low 7 bits of `-379`) and `localZ = 106`. Test reports both errors against the pinned (53, 50) targets and the bounds invariant flags `localZ` as OOB.

If the test passes pre-fix: STOP — diagnosis is wrong; re-open Stage 1.

- [ ] **Step 1.4: Commit**

```bash
git add modules/world/tick_order_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-93 B1 T1 — pin stale-Origin tele leaf bug

Failing integration test that reproduces the user-launched smoke
crash (`::tele 0,41,51,37,42` from Tutorial Island spawn). On HEAD,
processInfo runs rsbuf.ComputePlayer before updateMap, caching stale
Origin. The cross-window tele's PlayerInfo leaf encodes localX=5 (from
PBit(7) wrap of -379) and localZ=106 — outside the Java client's
[0, 104] active-window array bound.

Test fails on HEAD; will pass after T2 moves updateMap into processInfo.

Per Engine-TS/World.ts:996: "set origin before compute player is why
this is above."

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Move `updateMap()` into `processInfo`

**Goal:** Land the TS-faithful tick-order fix. T1's test must PASS post-T2.

**Files:**
- Modify: `modules/world/tick.go:349-351` (per-player reorient loop — extend with `updateMap`)
- Modify: `modules/world/player.go:815-825` (`processOut` — remove `updateMap` call)

- [ ] **Step 2.1: Modify `processInfo`'s per-player reorient loop**

Edit `modules/world/tick.go`. Replace the existing block at lines 346-351:

**Old:**
```go
	// NAI-66: TS World.ts:995 — per-tick refocus before rsbuf compute.
	// Refocuses on a moved PathingEntity target or clears the cached
	// Loc/Obj targetX/Z when the player took zero steps this tick.
	for _, p := range players {
		p.reorient()
	}
```

**New:**
```go
	// NAI-66: TS World.ts:995 — per-tick refocus before rsbuf compute.
	// Refocuses on a moved PathingEntity target or clears the cached
	// Loc/Obj targetX/Z when the player took zero steps this tick.
	//
	// NAI-93: TS World.ts:996 — buildArea.rebuildNormal() runs in this
	// loop, BEFORE the ComputePlayers/ComputePlayer calls below, so the
	// rsbuf-cached Origin matches the just-emitted RebuildNormal packet's
	// zoneX/zoneZ. Inverting this order produces stale-origin tele leaves
	// on cross-window teles → Java client AIOOBE in getHeightmapY/getTopLevel.
	// TS comment at World.ts:996 verbatim: "set origin before compute
	// player is why this is above."
	for _, p := range players {
		p.reorient()
		p.updateMap()
	}
```

- [ ] **Step 2.2: Remove the `updateMap` call from `processOut`**

Edit `modules/world/player.go`. Replace the existing block at lines 815-825:

**Old:**
```go
func (p *Player) processOut() {
	p.updateMap()
	p.updatePlayers()
	p.updateNpcs()
	p.updateZones()
	p.updateInvs()
	p.updateStats()
	p.updateAfkZones()
	p.encodeOut()
	p.client.flushWrite()
}
```

**New:**
```go
func (p *Player) processOut() {
	// NAI-93: updateMap moved to Server.processInfo per TS World.ts:996
	// ordering. processOut now starts with PlayerInfo encode against the
	// already-fresh rsbuf state.
	p.updatePlayers()
	p.updateNpcs()
	p.updateZones()
	p.updateInvs()
	p.updateStats()
	p.updateAfkZones()
	p.encodeOut()
	p.client.flushWrite()
}
```

- [ ] **Step 2.3: Run T1's test — confirm it PASSES**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestProcessInfo_TeleAcrossWindow_LocalCoordsInRange ./modules/world/...
```

Expected: PASS.

- [ ] **Step 2.4: Run the full `modules/world` test suite — confirm no regressions**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. In particular, `TestLoginSendsRebuildNormal` and `TestUpdateMapAnchorsOriginToPlayer` (`login_map_test.go`) must still pass — they call `p.updateMap()` directly and are unaffected by the call-site move.

If `TestProcessInfo_*` (rsbuf per-tick smoke tests at `rsbuf_per_tick_test.go`) start emitting RebuildNormal packets to bufw, they may begin draining bytes that previously didn't exist. The existing tests don't assert on bufw contents — they only assert on rsbuf state — so they should continue to pass. If any rsbuf-per-tick test fails, investigate before proceeding.

- [ ] **Step 2.5: Run the full repo test suite (race detector enabled per project convention)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS.

- [ ] **Step 2.6: Commit**

```bash
git add modules/world/tick.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-93 B1 T2 — move updateMap into processInfo per TS

Ports TS World.ts:996 ordering: per-player loop in processInfo now
calls reorient → updateMap → (downstream ComputePlayer push). The TS
source comment is verbatim: "set origin before compute player is why
this is above."

Pre-NAI-93: processInfo ran ComputePlayer before processClientsOut →
processOut → updateMap. On a cross-window tele, rsbuf-cached Origin
was stale; PlayerInfo's tele leaf encoded localX/Z relative to the
old origin, producing values >= 105 that crashed the Java client in
getHeightmapY (client.java:2640) and getTopLevel (client.java:1935).

Post-NAI-93: updateMap fires before ComputePlayer in the same loop,
so rsbuf captures the fresh origin set by rebuildScenery. PlayerInfo
tele leaf encodes localX, localZ in [0, 104] as expected.

T1's TestProcessInfo_TeleAcrossWindow_LocalCoordsInRange now passes
(was failing on HEAD).

Wire packet ordering preserved: sendRebuildNormal still appends to
p.client.bufw before processOut's updatePlayers; flushWrite is the
single per-tick flush.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Doc-comment cleanup at `updateMap` and `rebuildZones`

**Goal:** Update the doc comments that referenced the old call-site / ordering, so future readers see the post-NAI-93 picture.

**Files:**
- Modify: `modules/world/player.go:300-303` (the rebuiltOnce comment that mentions "BEFORE updateMap each tick")
- Modify: `modules/world/player.go:680-696` (the `rebuildZones` doc-comment block)
- Modify: `modules/world/player.go:720-733` (the `updateMap` doc-comment)

- [ ] **Step 3.1: Read current state of the three doc-comment regions**

Run:
```bash
sed -n '295,305p' modules/world/player.go
sed -n '680,696p' modules/world/player.go
sed -n '720,733p' modules/world/player.go
```

- [ ] **Step 3.2: Update the `rebuiltOnce` comment**

Edit `modules/world/player.go`. The comment currently says (around lines 298-303):

```go
	// rebuiltOnce gates shouldRebuild's first-build trigger. Legacy
	// pkg/buildarea encoded this via OriginX = -1, but Player.originX is
	// […]
	// BEFORE updateMap each tick). Reusing originX as the sentinel would
	// […]
```

Find the line containing `BEFORE updateMap each tick` and amend the parenthetical to reflect that updateMap now runs in `processInfo` (not `processOut`). Use Edit to change the specific phrase from `BEFORE updateMap each tick` to `BEFORE updateMap each tick, NAI-93: updateMap is in processInfo`. **Do NOT** rewrite the whole block — just the parenthetical phrase.

If the surrounding text doesn't read naturally after the edit, fix it inline; the goal is one accurate sentence, not a rewrite.

- [ ] **Step 3.3: Update the `rebuildZones` doc-comment block**

Edit `modules/world/player.go`. The current block at lines 680-696 says:

```go
// rebuildZones refreshes activeZones to a 7×7-zone window centered on
// the player's current zone, intersected with the 13×13-zone
// build-area window centered on origin. Mirrors TS
// BuildArea.rebuildZones (BuildArea.ts:31-55).
//
// Called at the end of handleRebuildGetMaps (after the client confirms
// maps loaded). Not called per-zone-change because goscape has not yet
// ported NetworkPlayer.ts:269-271 lastZone-transition tracking;
// deferred follow-up in nai84_rebuildzones_per_zone_change.md.
//
// Note: rebuildScenery (player.go:600-635) currently also writes
// activeZones (with a 13×13 set keyed at level=0). That pre-existing
// divergence is intentionally not touched here — see TS-fidelity
// ledger entry §6 R-D2. rebuildZones runs after rebuildScenery in the
// REBUILD path (rebuildScenery → sendRebuildNormal → client requests
// maps → handleRebuildGetMaps → rebuildZones), so the rebuildScenery
// preset is overwritten before zone deltas flow.
```

The "rebuildScenery → sendRebuildNormal → client requests maps → handleRebuildGetMaps → rebuildZones" chain is still accurate post-NAI-93. The sequence happens within the same tick (rebuildScenery + sendRebuildNormal in processInfo), then the client's REBUILD_GETMAPS request lands in next-tick processClientsIn. **No edit needed here.** Read the block to confirm, then mark this step complete.

- [ ] **Step 3.4: Update the `updateMap` doc-comment**

Edit `modules/world/player.go`. The current block at lines 720-733 says:

```go
func (p *Player) updateMap() {
	if p.client == nil || p.client.server == nil {
		return
	}
	if !p.shouldRebuild() {
		return
	}
	// rebuildScenery anchors p.originX/Z to the new rebuild position so the
	// next PlayerInfo teleport block produces local coords in range [0, 104].
	// Staleness would overflow the 7-bit PBit(7, localX) encoding.
	ms := p.rebuildScenery(p.client.server.currentTick)
	p.reconnecting = false
	sendRebuildNormal(p, ms)
}
```

Replace with (using Edit to amend the inner comment only):

```go
func (p *Player) updateMap() {
	if p.client == nil || p.client.server == nil {
		return
	}
	if !p.shouldRebuild() {
		return
	}
	// rebuildScenery anchors p.originX/Z to the new rebuild position so
	// the rsbuf-cached Origin captured by the IMMEDIATELY FOLLOWING
	// ComputePlayer call (in the same Server.processInfo per-player loop)
	// matches the just-emitted RebuildNormal packet's zoneX/zoneZ.
	//
	// NAI-93 moved this call from processOut to processInfo per TS
	// World.ts:996 ordering. Pre-NAI-93, the ComputePlayer call had
	// already cached the STALE origin by the time updateMap ran, and the
	// PlayerInfo tele leaf encoded localX = pos.X - (((staleOriginX>>3)
	// - 6) << 3) — which on a cross-window tele produced values outside
	// the Java client's 0..104 active-window array bound, crashing in
	// getHeightmapY and getTopLevel.
	ms := p.rebuildScenery(p.client.server.currentTick)
	p.reconnecting = false
	sendRebuildNormal(p, ms)
}
```

- [ ] **Step 3.5: Run the world test suite (no behavior changes; pure docs)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS.

- [ ] **Step 3.6: Commit**

```bash
git add modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-93 B1 T3 — updateMap doc-comment reflects new placement

updateMap now runs in Server.processInfo before ComputePlayer (T2).
The doc-comment at the rebuildScenery anchor explains why: rsbuf
cache must be populated AFTER the origin update so the encoded
PlayerInfo tele leaf encodes local coords in range [0, 104].

Cross-references the TS World.ts:996 ordering and the pre-fix
client crash signature (getHeightmapY at client.java:2640).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 2 — `::tele` cheat handler tightening

Per spec §4.2. Three tasks; commit after each.

### Task 4: Failing tests for the cheat-handler cleanup chain

**Goal:** Pin the four new behaviors (`closeModal`, `canAccess` gate + message, waypoint clear, ordering vs bounds-check). All four tests must FAIL on HEAD before T5.

**Files:**
- Modify: `modules/world/handlers_game_test.go` — append four tests at end of file.

- [ ] **Step 4.1: Locate the existing handlers_game_test.go file end + check imports**

Run:
```bash
wc -l modules/world/handlers_game_test.go
sed -n '1,30p' modules/world/handlers_game_test.go
```

Read the imports — note whether `gameserver`, `bytes`, `strings` are already imported. If not, the test additions below will need import-block extension.

- [ ] **Step 4.2: Locate the cheat-handler dispatch entry point used in existing tests**

Run:
```bash
grep -n "handleClientCheat\|case \"tele\"" modules/world/handlers_game.go modules/world/handlers_game_test.go | head -10
```

Determine the function that contains the `case "tele":` dispatch. This is the function the new tests call (likely `handleClientCheat` or similar — confirm name from grep output before authoring).

If the existing `handlers_game_test.go` already exercises the cheat handler (e.g., for `say` or `getcoord`), reuse the same call pattern. If not, the new tests construct the input bytes for the `OpClientCheat` opcode and route them through `p.processIn` or call the dispatch function directly — whichever the existing tests for adjacent handlers use.

**If no existing pattern exists for cheat-handler tests:** the four new tests below directly call the cheat-dispatch function (referred to below as `handleClientCheat`; substitute the actual name from grep output). Reading `handlers_game.go:330-413` first will confirm the exact entry-point signature.

- [ ] **Step 4.3: Add the four failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
// --- NAI-93 Bundle 2: ::tele cheat-handler tightening ---

// TestTeleCheat_CallsCloseModal — pins that ::tele invokes p.CloseModal,
// closing any open modal. Mirrors TS ClientCheatHandler.ts:504.
func TestTeleCheat_CallsCloseModal(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	p.staffModLevel = 2
	p.modalState = modalStateMain

	if err := dispatchCheat(t, p, "tele 0,50,50,32,32"); err != nil {
		t.Fatalf("dispatchCheat: %v", err)
	}

	if p.modalState != modalStateNone {
		t.Errorf("modalState after ::tele: got %d, want %d (modalStateNone)",
			p.modalState, modalStateNone)
	}
}

// TestTeleCheat_CanAccessGate_RejectsAndMessagesGame — pins that the
// canAccess gate rejects with the TS message and DOES NOT teleport.
// Mirrors TS ClientCheatHandler.ts:506-509.
func TestTeleCheat_CanAccessGate_RejectsAndMessagesGame(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	p.staffModLevel = 2
	// Force CanAccess() = false via p.delayed (player_script.go:284).
	p.delayed = true

	startX, startZ := p.x, p.z
	bufBefore := drainBufw(t, p)
	if err := dispatchCheat(t, p, "tele 0,50,50,32,32"); err != nil {
		t.Fatalf("dispatchCheat: %v", err)
	}
	bufAfter := drainBufw(t, p)
	emitted := bufAfter[len(bufBefore):]

	if p.x != startX || p.z != startZ {
		t.Errorf("position changed despite CanAccess=false: (%d, %d) → (%d, %d)",
			startX, startZ, p.x, p.z)
	}
	if !containsMessageGame(emitted, "Please finish what you are doing first.") {
		t.Errorf("expected MessageGame %q in emitted bytes; not found", "Please finish what you are doing first.")
	}
}

// TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket — pins that
// the cheat handler clears p.waypointIndex AND emits OpUnsetMapFlag,
// matching the TS Player.unsetMapFlag bundle (Player.ts:2169-2172):
// clearWaypoints + write(UnsetMapFlag).
func TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	p.staffModLevel = 2
	p.waypointIndex = 5

	bufBefore := drainBufw(t, p)
	if err := dispatchCheat(t, p, "tele 0,50,50,32,32"); err != nil {
		t.Fatalf("dispatchCheat: %v", err)
	}
	bufAfter := drainBufw(t, p)
	emitted := bufAfter[len(bufBefore):]

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex after ::tele: got %d, want -1", p.waypointIndex)
	}
	if !containsOpcode(emitted, gameserver.OpUnsetMapFlag.Opcode, p) {
		t.Errorf("expected OpUnsetMapFlag (%d) in emitted bytes; not found",
			gameserver.OpUnsetMapFlag.Opcode)
	}
}

// TestTeleCheat_BoundsCheck_RejectsAfterCleanup — pins TS-faithful
// ordering: closeModal/clearInteraction/unsetMapFlag run BEFORE the
// numeric bounds check. An invalid coord still triggers cleanup side
// effects but does NOT teleport. Matches TS lines 504-522 ordering.
func TestTeleCheat_BoundsCheck_RejectsAfterCleanup(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	p.staffModLevel = 2
	p.modalState = modalStateMain
	p.waypointIndex = 5
	startX, startZ := p.x, p.z

	// Level 9 is OOB; bounds check at the end of the case rejects.
	if err := dispatchCheat(t, p, "tele 9,50,50,32,32"); err != nil {
		t.Fatalf("dispatchCheat: %v", err)
	}

	// Cleanup side effects fire BEFORE bounds check (TS-faithful).
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %d, want modalStateNone (cleanup runs before bounds check)", p.modalState)
	}
	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (cleanup runs before bounds check)", p.waypointIndex)
	}
	// Bounds check rejects: position unchanged.
	if p.x != startX || p.z != startZ {
		t.Errorf("position changed despite OOB level: (%d, %d) → (%d, %d)",
			startX, startZ, p.x, p.z)
	}
}

// dispatchCheat sends "::<cmd args>" through the cheat-handler entry
// point. Substitutes for direct `handleClientCheat` invocation; the
// implementer adapts to the actual entry-point pattern at
// handlers_game.go (read the existing tele case at lines 371-411 to
// determine whether the dispatch is a method on Player, a free
// function, or routed through processIn). If a sibling test of `say`
// or `getcoord` already exists in handlers_game_test.go, reuse its
// pattern.
func dispatchCheat(t *testing.T, p *Player, line string) error {
	t.Helper()
	// IMPLEMENTER-NOTE: replace the body below with the project's
	// established cheat-dispatch test pattern.
	return p.handleClientCheat(line) // PLACEHOLDER — see note above.
}

// drainBufw reads all currently-buffered bytes from p.client.bufw
// (via p.client.flushWrite() draining to the test connection's
// reader). Returns the cumulative bytes seen by the test conn.
//
// IMPLEMENTER-NOTE: see existing message_game_test.go and
// movement_test.go for the established pattern of reading bytes from
// the test conn after flushWrite. If a `drainConn(t, p) []byte`
// helper already exists, use it; otherwise inline the read here.
func drainBufw(t *testing.T, p *Player) []byte {
	t.Helper()
	// IMPLEMENTER-NOTE: see note above; replace with the established
	// drain pattern.
	return nil // PLACEHOLDER — see note above.
}

// containsMessageGame reports whether `data` contains an
// OpMessageGame packet whose JagString payload equals `want`.
func containsMessageGame(data []byte, want string) bool {
	// IMPLEMENTER-NOTE: see message_game_test.go for the existing
	// MessageGame decode pattern (ISAAC opcode + 1-byte length + JStrLF).
	// Inline here or extract to a shared helper.
	_ = data
	_ = want
	return false // PLACEHOLDER — see note above.
}

// containsOpcode reports whether `data` contains an emission of the
// given (pre-encryption) opcode, decrypted via p.client.encryptor's
// state at the time of emission.
func containsOpcode(data []byte, opcode uint8, p *Player) bool {
	// IMPLEMENTER-NOTE: ISAAC stream is consumed-by-position; the
	// implementer may choose to either (a) re-derive a sibling
	// encryptor at test setup and walk it forward by N opcodes, or
	// (b) check the bufw payload before encryption. Pattern (b) is
	// simpler — call drainBufw BEFORE flushWrite (read directly from
	// p.client.bufw's buffer if accessible) — but the existing tests
	// favor (a). Pick whichever matches the surrounding style.
	_ = data
	_ = opcode
	_ = p
	return false // PLACEHOLDER — see note above.
}
```

**IMPLEMENTER-NOTE for Task 4:** The four named test functions above are concrete and runnable. The four trailing helpers (`dispatchCheat`, `drainBufw`, `containsMessageGame`, `containsOpcode`) deliberately ship as `// PLACEHOLDER` stubs because the established test patterns for cheat-dispatch + bufw-drain + opcode-decode are sibling-specific to this codebase, and the plan-author has not pinned which sibling test to mirror. **Step 4.4 below requires the implementer to read the sibling tests, fill in the four helpers, and then run the four named tests to confirm they fail in the right way.**

This deviates from the standard "complete code in every step" rule of the writing-plans skill. The deviation is justified: codifying inline would risk the `plan_runnable_test_fixtures` failure mode (plan-author writes a fixture that doesn't actually compile/run as-is). Better to scope this as an implementer-side adaptation step with a TS-faithful target.

- [ ] **Step 4.4: Implementer fills in the four helpers**

Read the following sibling tests to determine the established patterns:

```bash
sed -n '1,50p' modules/world/message_game_test.go
sed -n '120,170p' modules/world/movement_test.go
sed -n '100,130p' modules/world/player_interaction_trigger_test.go
```

For `dispatchCheat`: locate the function in `handlers_game.go` that contains the `case "tele":` dispatch — that's the entry point. If no sibling test exists for `::say` / `::getcoord`, dispatch via `p.processIn` with a constructed `OpClientCheat` packet, OR call the dispatch function directly with the `parts[0] = cmd, parts[1] = args` arguments. Pick the simpler path.

For `drainBufw`: the standard pattern (per `login_map_test.go:14-48` and `message_game_test.go`) is `p.client.flushWrite()` followed by reading from the test conn. For tests that don't need wire-byte assertions (e.g., the modalState and waypointIndex tests), `drainBufw` is unused — strip it from those tests if simpler.

For `containsMessageGame` and `containsOpcode`: the cleanest pattern is to read `p.client.bufw`'s underlying buffer BEFORE `flushWrite` — but goscape's `bufio.Writer` doesn't expose its buffer. So either (a) flush + read from conn + ISAAC-decrypt, or (b) replace `p.client.bufw` with an intercepting `bytes.Buffer`-wrapped `bufio.Writer` at test setup. Pick whichever pattern is already established; if neither, prefer (a) for consistency with `login_map_test.go`.

Replace each `// PLACEHOLDER` body with the worked impl. Compile-check after each helper:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

Expected: no vet errors.

- [ ] **Step 4.5: Run the four new tests — confirm they FAIL pre-fix**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestTeleCheat ./modules/world/...
```

Expected:
- `TestTeleCheat_CallsCloseModal` — FAIL (current code does not call `CloseModal`).
- `TestTeleCheat_CanAccessGate_RejectsAndMessagesGame` — FAIL (current code does not gate on `CanAccess`; player tele's anyway despite `delayed=true`).
- `TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket` — FAIL (current code does not call `sendUnsetMapFlag`; waypointIndex unchanged).
- `TestTeleCheat_BoundsCheck_RejectsAfterCleanup` — FAIL (modalState and waypointIndex unchanged because cleanup chain isn't wired).

If any test passes pre-fix: STOP — assertion is wrong; revisit fixture.

- [ ] **Step 4.6: Commit**

```bash
git add modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-93 B2 T4 — pin ::tele pre-tele cleanup chain

Four failing tests for the TS-faithful pre-tele cleanup chain at
ClientCheatHandler.ts:504-512: closeModal, canAccess gate with
"Please finish what you are doing first." message, unsetMapFlag
(clearWaypoints + write packet), and TS-faithful ordering where
cleanup runs BEFORE the numeric bounds check.

Tests fail on HEAD; will pass after T5 wires the cleanup chain.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Implement the cleanup chain in the cheat handler

**Goal:** Wire the TS-faithful pre-tele cleanup. T4's four tests must PASS post-T5.

**Files:**
- Modify: `modules/world/handlers_game.go:371-411` (the `case "tele":` body).

- [ ] **Step 5.1: Replace the `case "tele":` body**

Edit `modules/world/handlers_game.go`. Replace the existing block at lines 371-411:

**Old:**
```go
	case "tele":
		// staffModLevel >= 2 gate mirrors TS ClientCheatHandler.ts:483.
		if p.staffModLevel < 2 {
			return nil
		}
		// Mirrors TS `::tele level,mapX,mapZ[,localX,localZ]` at
		// ClientCheatHandler.ts:491-523. Single-arg form:
		// "::tele 0,50,50,32,32".
		//
		// DEVIATION: TS pre-tele cleanup calls player.closeModal(),
		// player.canAccess() (with "Please finish what you are doing
		// first." gate), and player.unsetMapFlag() — none of which exist
		// on goscape Player yet. We call ClearInteraction (the one that
		// does exist) and skip the others; tele is a staff-only debug
		// op so the cleanup gap is acceptable for the smoke-test
		// enabler scope. Track in nai_followups.md with the
		// pathing-entity-teleport-parity sub-spec.
		if args == "" {
			return nil
		}
		coord := strings.Split(args, ",")
		if len(coord) < 3 {
			return nil
		}
		level := parseIntOr(coord[0], 0)
		mx := parseIntOr(coord[1], 50)
		mz := parseIntOr(coord[2], 50)
		lx := 32
		if len(coord) > 3 {
			lx = parseIntOr(coord[3], 32)
		}
		lz := 32
		if len(coord) > 4 {
			lz = parseIntOr(coord[4], 32)
		}
		if level < 0 || level > 3 || mx < 0 || mx > 255 || mz < 0 || mz > 255 || lx < 0 || lx > 63 || lz < 0 || lz > 63 {
			return nil
		}
		p.ClearInteraction()
		p.TeleJump((mx<<6)+lx, (mz<<6)+lz, level)
```

**New:**
```go
	case "tele":
		// staffModLevel >= 2 gate mirrors TS ClientCheatHandler.ts:483.
		if p.staffModLevel < 2 {
			return nil
		}
		// Mirrors TS `::tele level,mapX,mapZ[,localX,localZ]` at
		// ClientCheatHandler.ts:491-524. Single-arg form:
		// "::tele 0,50,50,32,32".
		//
		// NAI-93 closed the prior DEVIATION block here: closeModal,
		// canAccess gate, and the unsetMapFlag bundle (sendUnsetMapFlag
		// + waypointIndex reset, per TS Player.unsetMapFlag at
		// Player.ts:2169-2172) are now wired. ClearInteraction
		// preserved.
		if args == "" {
			return nil
		}
		coord := strings.Split(args, ",")
		if len(coord) < 3 {
			return nil
		}

		// Pre-tele cleanup chain — order per TS lines 504-512.
		p.CloseModal(true) // TS closeModal() default-arg.
		if !p.CanAccess() {
			p.MessageGame("Please finish what you are doing first.")
			return nil
		}
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		p.waypointIndex = -1 // TS Player.unsetMapFlag → clearWaypoints.

		level := parseIntOr(coord[0], 0)
		mx := parseIntOr(coord[1], 50)
		mz := parseIntOr(coord[2], 50)
		lx := 32
		if len(coord) > 3 {
			lx = parseIntOr(coord[3], 32)
		}
		lz := 32
		if len(coord) > 4 {
			lz = parseIntOr(coord[4], 32)
		}
		if level < 0 || level > 3 || mx < 0 || mx > 255 || mz < 0 || mz > 255 || lx < 0 || lx > 63 || lz < 0 || lz > 63 {
			return nil
		}
		p.TeleJump((mx<<6)+lx, (mz<<6)+lz, level)
```

- [ ] **Step 5.2: Run T4's four tests — confirm they PASS**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestTeleCheat ./modules/world/...
```

Expected: all four PASS.

- [ ] **Step 5.3: Run the full `modules/world` test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS.

- [ ] **Step 5.4: Run with race detector**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS.

- [ ] **Step 5.5: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-93 B2 T5 — ::tele cheat handler TS-faithful cleanup chain

Wires the TS pre-tele cleanup at ClientCheatHandler.ts:504-512:
  player.closeModal();
  if (!player.canAccess()) { messageGame(...); return false; }
  player.clearInteraction();
  player.unsetMapFlag();

goscape now matches: CloseModal(true) → CanAccess gate with the TS
message → ClearInteraction → sendUnsetMapFlag bundle (waypointIndex
reset + OpUnsetMapFlag emit per the ts_helper_method_bundles
memory).

Closes the DEVIATION block previously documented at
handlers_game.go:380-387.

T4's four TestTeleCheat_* tests now pass (was failing on HEAD).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Doc-comment audit + cross-grep

**Goal:** Confirm no stale references to the closed DEVIATION remain anywhere in the repo. Per `retire_deviation_grep_all_comments` memory.

**Files:**
- (Audit only; no production code changes expected. If audit finds stale comments, edit them.)

- [ ] **Step 6.1: Cross-grep for the closed deviation's prose**

Run:
```bash
rg "DEVIATION.*closeModal|DEVIATION.*canAccess|DEVIATION.*unsetMapFlag" modules/ pkg/ cmd/
rg "pathing-entity-teleport-parity" modules/ pkg/ cmd/
rg "tele is a staff-only debug" modules/ pkg/ cmd/
```

Expected: no hits (the only previous occurrences were inside the now-removed DEVIATION block at `handlers_game.go:380-387`).

- [ ] **Step 6.2: If grep returns hits — edit each site**

Each remaining hit either belongs to a different deviation (leave alone) or is a stale reference (rewrite the comment to reflect post-NAI-93 reality). Use Read + Edit per site; do not `replace_all` (per `plan_doc_replaceall_timeline` memory).

- [ ] **Step 6.3: Run the test suite (no behavior change expected)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS.

- [ ] **Step 6.4: Commit (only if Step 6.2 made edits)**

If Step 6.2 made no edits, skip this commit and proceed to Smoke. Otherwise:

```bash
git add -p
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-93 B2 T6 — retire stale DEVIATION cross-references

Cross-grep per retire_deviation_grep_all_comments memory. <Describe
each edited site here in 1-2 lines.>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Smoke (user-launched, after T6)

Per `smoke_test_server_handoff` memory: Claude's sandboxed server is unreachable from the host Java client; the user runs the smoke. After T6 lands, present the user with this resume prompt.

**Smoke handoff prompt for the user:**

> NAI-93 implementation is complete (T1–T6 committed). The fix moves `updateMap` into `processInfo` (Bundle 1) and tightens the `::tele` cheat handler (Bundle 2). Run the smoke yourself — Claude's process is sandboxed.
>
> 1. Build: `CGO_ENABLED=0 go build -trimpath -o ./goscape ./cmd/goscape`
> 2. Start: `./goscape --config.file config.yaml`
> 3. Connect with the Java client (LostCityRS/Client-Java branch 225), log in.
> 4. Run reproducers:
>    - `::tele 0,41,51,37,42` — expect: no client crash; Ardougne market square renders.
>    - `::tele 0,40,51,37,42` — expect: no crash.
>    - `::tele 0,41,51,0,0` then click any tile northwest — expect: no crash.
> 5. Tutorial Island sanity: `::tele 0,48,50,21,28` and walk around — expect: no crash. (Note: the +23-tile-north drift at first tele after region load is a separate residual, NAI-92 #2; not a NAI-93 bail trigger.)
>
> Report which steps pass/fail. If 1-3 still crash, NAI-93 closes as Stage 1+2 complete-but-incomplete and routes residual to NAI-94 / Client-Java repo (β bail-out).

---

## Close commit (after smoke)

If smoke passes all of steps 4.1-4.4, run the close commit:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-93 — RebuildNormal tick-order + tele cheat tightening [smoke confirmed]

User-launched smoke 2026-MM-DD confirmed all three reproducers fixed:
  ::tele 0,41,51,37,42 — Ardougne market square renders, no AIOOBE.
  ::tele 0,40,51,37,42 — same, no crash.
  ::tele 0,41,51,0,0 + click NW — no crash.

Tutorial Island regression: walk around post-tele, no crash.

Bundle 1: moved updateMap into Server.processInfo per TS World.ts:996
ordering ("set origin before compute player is why this is above.").
Bundle 2: ::tele cheat handler now matches TS ClientCheatHandler.ts:
504-512 cleanup chain (closeModal, canAccess gate, unsetMapFlag).

NAI-92 residuals #1 (pathfinder reach for through-door routes) and #2
(first-tele-after-region-load drift) remain open; tracked in
nai_followups.md.

Closes memory:
  - nai_followups.md "From NAI-92" residual #3 (Java client AIOOBE).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If smoke fails: replace `[smoke confirmed]` with `[smoke partial]` or `[β bail to NAI-94]`, document which step failed in the body, and route the residual per `cascade_theory_smoke_binding`.

---

## Self-review (done at plan-write 2026-05-05)

**1. Spec coverage:**
- Spec §4.1 (Bundle 1 fix) → T1, T2, T3.
- Spec §4.2 (Bundle 2 cleanup chain) → T4, T5.
- Spec §5.1 (T1 unit test pinning post-fix localX=53, localZ=50) → T1.
- Spec §5.2 (four cheat-handler tests) → T4 (each named test corresponds 1:1 to a §5.2 case).
- Spec §6 (smoke) → smoke handoff prompt + close commit.
- Spec §7 (risk register) — R1 (wire ordering), R2 (other call sites), R3 (test helper drift), R4 (CloseModal side effects), R5 (MessageGame nil server) — addressed in PF-1, PF-3, T1's encoder-direct approach (sidesteps tickOnce drift), T4's CanAccessGate test, and PF-5 respectively.
- Spec §8 (out of scope) — preserved; nothing in plan touches NAI-92 #1, NAI-92 #2, ::teleto, or NAI-91 residual.
- Spec §9 (TS-fidelity ledger) — closed by T3 (B1 doc) and T6 (B2 cross-grep).
- Spec §10 (memory updates) — close commit `Closes memory:` trailer.

**2. Placeholder scan:** Four explicit `// PLACEHOLDER` markers in T4's auxiliary helpers (`dispatchCheat`, `drainBufw`, `containsMessageGame`, `containsOpcode`). Each is paired with an `IMPLEMENTER-NOTE` directing the implementer to a sibling test pattern. The four primary test functions are concrete and runnable. Step 4.4 explicitly requires the implementer to fill in the helpers with sibling-pattern code.

This is a deliberate scope-limited placeholder to avoid `plan_runnable_test_fixtures` failure (codifying invented helpers that don't compile). Justified inline at Step 4.3's IMPLEMENTER-NOTE.

**3. Type consistency:** All identifiers used in the plan match HEAD (verified in PF-5):
- `(*Player).CloseModal(bool)` — present.
- `(*Player).CanAccess() bool` — present.
- `(*Player).MessageGame(string)` — present.
- `(*Player).ClearInteraction()` — used at existing tele site.
- `sendUnsetMapFlag(*Player)` — present.
- `p.waypointIndex int` — field exists at `player.go:100`.
- `gameserver.OpUnsetMapFlag.Opcode` — `prot.go:89`.
- `modalStateMain`, `modalStateNone` — `player.go:36-39`.

**4. Test-coverage cross-check** (per `plan_test_coverage_crosscheck`): every code branch added or moved is tested:
- B1's reordering — T1's TestProcessInfo_TeleAcrossWindow_LocalCoordsInRange.
- B2's `CloseModal(true)` — T4 #1.
- B2's `CanAccess()` gate + MessageGame — T4 #2.
- B2's `sendUnsetMapFlag + waypointIndex=-1` — T4 #3.
- B2's TS-faithful ordering (cleanup before bounds check) — T4 #4.

**5. Plan-author audit of sibling-site guards** (per `plan_sibling_site_guard_audit`): the existing `case "tele":` body has a `staffModLevel < 2` early-return; the new body preserves it. The new `CloseModal(true) → CanAccess() → sendUnsetMapFlag(p)` chain mirrors `handler_opnpc.go:37-59`'s sibling pattern (multiple `sendUnsetMapFlag(p)` calls; goscape pattern is the bare call without nil-server guard because the cheat handler is reached only after the server-side `processIn` plumbing has already validated `p.client.server`). Verified.

**6. Variable-name collision check** (per `plan_var_name_collision`): no new `:=` declarations shadow existing parameters in T2 or T5. T1's test names are unique to the test file. ✓.

---

## Resume prompt (for fresh implementer session after `/clear`)

Per `superpowers_clear_between_spec_and_impl` memory: after this plan is committed, paste the following into a fresh session:

> **Implement NAI-93 per the plan at `docs/superpowers/plans/2026-05-05-nai-93-rebuild-normal-tick-order.md`.**
>
> The plan is committed; the spec is at `docs/superpowers/specs/2026-05-05-nai-93-rebuild-normal-tick-order-design.md`. Both are HEAD-current.
>
> Use **subagent-driven development**: dispatch one fresh subagent per task (T1, T2, T3, T4, T5, T6), with the controller running pre-flight (the PF-1..PF-9 checklist) before T1 dispatch and a 30-second `git status` + grep verification after each subagent's commit (per `controller_preflight` and `verify_implementer_claims` memories).
>
> Bundle 1 = T1+T2+T3 (tick-order fix); Bundle 2 = T4+T5+T6 (tele cheat tightening). After T6 commits, surface the smoke handoff prompt (in the plan's "Smoke" section) for me to run myself; then await my confirmation before the close commit.
>
> Per `execution_mode_default`: skip the "which mode?" menu — go straight to subagent-driven dispatch.
