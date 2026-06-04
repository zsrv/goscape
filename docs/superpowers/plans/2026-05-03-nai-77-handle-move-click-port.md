# NAI-77 — `handleMoveClick` port: close modals on walk-click — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `MoveClickHandler.ts` to goscape, fixing the click-away modal-dismiss symptom (NAI-76 cascade-residual) and bringing handler-level + per-tick `WalkTriggerSetting` semantics to TS-fidelity.

**Architecture:** Wrapper-pattern split for opClick differentiation (`handleMoveGameClick` / `handleMoveOpClick` → shared `moveClickInner`); typed `WalkTriggerSetting` enum reusing existing `NodeWalktriggerSetting` config field; new `userPath []int` field on Player persisted by the handler and consumed by a new per-tick fallback phase in the tick driver.

**Tech Stack:** Go 1.26+; standard `testing` package; existing goscape patterns (`pkg/io/packet`, `coordgrid.PackCoord`, `sendUnsetMapFlag`, `newTestPlayer` fixture).

**Spec:** `docs/superpowers/specs/2026-05-03-nai-77-handle-move-click-port-design.md` (commit `fd6961f`).

**HEAD at plan-write:** `fd6961f`.

---

## Pre-flight verification (controller-preflight per `controller_preflight.md`)

| # | Premise | Verified |
|---|---------|----------|
| P1 | `handlers_game.go:21-22` wires opcodes 181 and 93 to same `handleMoveClick`. | ✓ |
| P2 | `handlers_game.go:193-218` is current `handleMoveClick` body; missing TS body. | ✓ |
| P3 | `player_script.go:824-829` `ClearPendingAction()` calls `CloseModal(true)`. | ✓ |
| P4 | `player_script.go:674` `CloseModal(true)` is immediate, not deferred via `requestModalClose`. | ✓ |
| P5 | `player.go:35-39` modal state constants (`modalStateNone`=0x0, `modalStateChat`=0x2, etc.). | ✓ |
| P6 | `interaction.go:42` `sendUnsetMapFlag(p)` helper exists. | ✓ |
| P7 | `interaction.go:282` `(*Player).processWalktrigger` exists with self-gates (delayed, !protected). | ✓ |
| P8 | `movement.go:123` `(*Player).pathToMoveClick(packed []int, needsFinding bool)` exists. | ✓ |
| P9 | `config.go:25` already has `NodeWalktriggerSetting int` field + flag at line 80 with `// TODO: replace default with enum`. **Reuse, don't add a new field.** | ✓ |
| P10 | `userPath` field does NOT exist on Player today. New field added in T3. | ✓ |
| P11 | `tick.go:36-46` phase ordering: processClientsIn → processWorldQueue → processActiveScripts → processPlayerTimers → processPathing → processInteractions → … | ✓ |
| P12 | `interaction.go:170-172` `processInteraction` returns early when `p.target == nil`. | ✓ |
| P13 | `player_test.go:15` `newTestPlayer(t) (*Player, net.Conn)` is the established handler-test fixture. | ✓ |
| P14 | `interaction_test.go:865` `newTestPlayerAt(t, s, slot, x, z, level) *Player` is the per-tick-test fixture. | ✓ |
| P15 | `coordgrid.PackCoord(level, x, z) int` is the existing helper used at `handlers_game.go:204, 208`. | ✓ |
| P16 | TS wire payloads identical between MOVE_GAMECLICK (181) and MOVE_OPCLICK (93); opClick from prot, not payload. | ✓ |
| P17 | `player.go:236` `moveClickRequest bool` field exists; not mutated per-tick today (only set false in `handler_opheld.go`). | ✓ |
| P18 | `coordgrid.DistanceToSW` (or equivalent) — verify before T2. | ⚠ Unverified — implementer Step T2.1a checks; if missing, write inline distance helper. |

---

## Task overview

| Task | Scope | LOC est | TDD? |
|------|-------|---------|------|
| T1 | `WalkTriggerSetting` typed enum + retype `NodeWalktriggerSetting` field. | ~25 | Compile-only (compressed cadence per `compressed_cadence.md` — pure type rename, no behavior change). |
| T2 | `moveClickInner` + `handleMoveGameClick` + `handleMoveOpClick` + dispatch rewire. The symptom-2 fix lands here. | ~140 + ~250 tests | Full TDD (red→green per test). |
| T3 | `userPath []int` field on Player + persist in `moveClickInner` + per-tick fallback in `tick.go`. | ~80 + ~120 tests | Full TDD. |

---

## Task 1: `WalkTriggerSetting` typed enum

**Files:**
- Create: `modules/world/walk_trigger_setting.go`
- Modify: `modules/world/config.go:25` (retype field) and `modules/world/config.go:80` (drop TODO; default already 0=PLAYERPACKET)

**Cadence:** Compressed — pure type rename, no behavioral change. Verification gate is `go build ./...` clean.

- [ ] **Step 1.1: Create the enum file**

Create `modules/world/walk_trigger_setting.go`:

```go
package world

// WalkTriggerSetting selects how walktriggers are dispatched relative to
// the per-tick interaction loop. Mirrors TS WalkTriggerSetting.ts.
//
// PLAYERPACKET (default): walktriggers fire from the move-click packet
// handler (handleMoveGameClick → processWalktrigger). The per-tick
// fallback path (TS World.ts:635-641) is skipped.
//
// PLAYERSETUP: walktriggers fire from the per-tick fallback when
// !opcalled && hasWaypoints; handler-side dispatch is skipped.
//
// PLAYERMOVEMENT: handler-side dispatch is skipped; per-tick fallback
// re-paths userPath each tick but does NOT fire walktriggers.
type WalkTriggerSetting int

const (
	WalkTriggerSettingPlayerpacket  WalkTriggerSetting = 0
	WalkTriggerSettingPlayersetup   WalkTriggerSetting = 1
	WalkTriggerSettingPlayermovement WalkTriggerSetting = 2
)
```

- [ ] **Step 1.2: Retype the config field at config.go:25**

Change line 25:

```go
NodeWalktriggerSetting           int           `yaml:"node_walktrigger_setting"`
```

to:

```go
NodeWalktriggerSetting           WalkTriggerSetting `yaml:"node_walktrigger_setting"`
```

- [ ] **Step 1.3: Update flag registration at config.go:80**

Change line 80 from:

```go
	f.IntVar(&c.NodeWalktriggerSetting, "world.node-walk-trigger-setting", 0, "") // TODO: replace default with enum
```

to:

```go
	f.IntVar((*int)(&c.NodeWalktriggerSetting), "world.node-walk-trigger-setting", int(WalkTriggerSettingPlayerpacket), "WalkTriggerSetting: 0=PLAYERPACKET (default), 1=PLAYERSETUP, 2=PLAYERMOVEMENT")
```

- [ ] **Step 1.4: Verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean exit (no compile errors).

- [ ] **Step 1.5: Verify existing tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS across the board.

- [ ] **Step 1.6: Commit**

```bash
git add modules/world/walk_trigger_setting.go modules/world/config.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-77 T1 — WalkTriggerSetting typed enum

Retypes the existing NodeWalktriggerSetting int field to the new
typed enum. Default (0=PLAYERPACKET) is unchanged. Enum values mirror
TS WalkTriggerSetting.ts. No behavioral change at this commit.
EOF
)"
```

---

## Task 2: `moveClickInner` + wrapper pair (handler-level port — symptom-2 fix)

**Files:**
- Modify: `modules/world/handlers_game.go:21-22` (rewire dispatch table)
- Modify: `modules/world/handlers_game.go:193-218` (replace `handleMoveClick` with `moveClickInner` + two wrappers)
- Modify: `modules/world/handlers_game_test.go` (extend with new tests)

**Cadence:** Full TDD per behavioral change.

### Sub-task 2.1: Verify distance helper

- [ ] **Step 2.1a: Check for existing distance helper**

Run: `grep -rn "DistanceToSW\|distanceToSW" /home/owner/Code/github.com/zsrv/goscape/pkg/coordgrid/`
- If found: use it directly in subsequent steps.
- If not found: in `handlers_game.go`, inline the Chebyshev distance check via `max(abs(dx), abs(dz))` (TS `CoordGrid.distanceToSW(player, point)` is `Math.max(Math.abs(dx), Math.abs(dz))` per LostCityRS source). Document choice with an inline comment cross-citing TS line.

### Sub-task 2.2: Symptom-2 pin test (RED first)

- [ ] **Step 2.2: Write failing symptom-2 pin test in `handlers_game_test.go`**

Append to `modules/world/handlers_game_test.go`:

```go
// TestHandleMoveGameClickClosesChatModal pins the symptom-2 fix.
// With modalChat set, a MOVE_GAMECLICK packet must clear modalChat
// via ClearPendingAction → CloseModal(true). Mirrors TS
// MoveClickHandler.ts:43-44 + Player.closeModal at Player.ts:741-794.
func TestHandleMoveGameClickClosesChatModal(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalChat = 100
	p.modalState |= modalStateChat

	// Payload: ctrlHeld(1) | startX(2) | startZ(2) — single-tile path.
	payload := []byte{
		0,            // ctrlHeld = 0
		0x18, 0x70,   // startX = 6256 (player default-ish in newTestPlayer)
		0x32, 0x18,   // startZ = 12824
	}
	// Match the actual newTestPlayer coords; if they differ, adjust here.
	startX, startZ := p.x, p.z
	payload[1] = byte(startX >> 8)
	payload[2] = byte(startX & 0xff)
	payload[3] = byte(startZ >> 8)
	payload[4] = byte(startZ & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (CloseModal should have cleared)", p.modalChat)
	}
	if p.modalState&modalStateChat != modalStateNone {
		t.Errorf("modalState chat bit: got %d, want cleared", p.modalState&modalStateChat)
	}
}
```

- [ ] **Step 2.3: Run; verify it fails (compile fail acceptable, since `handleMoveGameClick` not yet defined)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMoveGameClickClosesChatModal -count=1`
Expected: FAIL — `undefined: handleMoveGameClick`.

### Sub-task 2.4: Implement `moveClickInner` + wrappers + rewire dispatch

- [ ] **Step 2.4: Replace `handleMoveClick` body at `handlers_game.go:193-218`**

Replace the existing function (lines 193-218) with:

```go
// handleMoveGameClick is the dispatch entry for MOVE_GAMECLICK (opcode 181).
// Routes to the shared inner handler with opClick=false, which causes the
// !opClick body to fire (clearPendingAction + tempRun + walktrigger).
func handleMoveGameClick(p *Player, payload []byte) error {
	return moveClickInner(p, payload, false)
}

// handleMoveOpClick is the dispatch entry for MOVE_OPCLICK (opcode 93).
// Routes to the shared inner handler with opClick=true, which skips the
// !opClick body (the move was triggered by an op click, not a plain
// ground click — the op click already handled modal/interaction state).
func handleMoveOpClick(p *Player, payload []byte) error {
	return moveClickInner(p, payload, true)
}

// moveClickInner is the shared move-click implementation.
// Mirrors TS MoveClickHandler.ts:10-58.
//
// Wire payload (per TS MoveClickDecoder.ts; identical between opcodes
// 181 and 93):
//
//	byte 0:    ctrlHeld (G1, expected 0 or 1)
//	bytes 1-2: startX (G2)
//	bytes 3-4: startZ (G2)
//	bytes 5+:  up to 24 waypoints, each 2 bytes (dx:G1B, dz:G1B)
//
// Gates per TS MoveClickHandler.ts:11-22:
//  1. p.delayed → UnsetMapFlag, no-op
//  2. ctrlHeld out of [0,1] OR Chebyshev(player, start) > 104 → unsetMapFlag,
//     no-op (TS also clears userPath; goscape adds the userPath field
//     in T3 so the clear becomes meaningful then)
//
// On success:
//  3. Build packed waypoint slice (handler-local; T3 also persists to
//     p.userPath for the per-tick fallback)
//  4. cfg.WalkTriggerSetting==PLAYERPACKET → pathToMoveClick
//  5. !opClick:
//     a. ClearPendingAction (fires CloseModal(true) — symptom-2 fix)
//     b. tempRun = ctrlHeld; override to 0 if runenergy<100 && ctrlHeld==1
//     c. cfg.WalkTriggerSetting==PLAYERPACKET && hasWaypoints → processWalktrigger
func moveClickInner(p *Player, payload []byte, opClick bool) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 5 {
		return nil
	}

	r := packet.NewPacket(payload)
	ctrlHeld := int(r.G1())
	startX := int(r.G2())
	startZ := int(r.G2())

	// Gate 2: ctrlHeld range + Chebyshev distance ≤ 104.
	dx := startX - p.x
	if dx < 0 {
		dx = -dx
	}
	dz := startZ - p.z
	if dz < 0 {
		dz = -dz
	}
	chebyshev := dx
	if dz > chebyshev {
		chebyshev = dz
	}
	if ctrlHeld < 0 || ctrlHeld > 1 || chebyshev > 104 {
		sendUnsetMapFlag(p)
		// T3 will also clear p.userPath here.
		return nil
	}

	// Build packed waypoint slice. Mirrors existing handler decode
	// (handlers_game.go pre-NAI-77).
	pathLen := min((len(payload)-5)/2, 24) + 1
	packed := make([]int, 0, pathLen)
	packed = append(packed, coordgrid.PackCoord(p.level, startX, startZ))
	for range min((len(payload)-5)/2, 24) {
		ddx := int(r.G1B())
		ddz := int(r.G1B())
		packed = append(packed, coordgrid.PackCoord(p.level, startX+ddx, startZ+ddz))
	}

	p.client.log.Debug("move click", "ctrl_held", ctrlHeld, "dest_packed", packed[0], "op_click", opClick)

	// Step 4: handler-side path dispatch only fires under PLAYERPACKET.
	if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayerpacket {
		needsFinding := !s.cfg.NodeClientRoutefinder
		p.pathToMoveClick(packed, needsFinding)
	}

	// Step 5: !opClick body.
	if !opClick {
		p.ClearPendingAction()

		if p.runenergy < 100 && ctrlHeld == 1 {
			p.tempRun = 0
		} else {
			p.tempRun = ctrlHeld
		}

		if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayerpacket && p.hasWaypoints() {
			p.processWalktrigger()
		}
	}

	return nil
}
```

- [ ] **Step 2.5: Rewire dispatch table at `handlers_game.go:21-22`**

Replace lines 21-22:

```go
	gameHandlers[181] = handleMoveClick        // MOVE_GAMECLICK
	gameHandlers[93] = handleMoveClick         // MOVE_OPCLICK
```

with:

```go
	gameHandlers[181] = handleMoveGameClick    // MOVE_GAMECLICK (opClick=false)
	gameHandlers[93] = handleMoveOpClick       // MOVE_OPCLICK (opClick=true)
```

- [ ] **Step 2.6: Run symptom-2 test; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMoveGameClickClosesChatModal -count=1 -v`
Expected: PASS.

### Sub-task 2.7: Negative pin — opClick=true preserves modal

- [ ] **Step 2.7: Add negative-pin test**

Append to `handlers_game_test.go`:

```go
// TestHandleMoveOpClickPreservesChatModal pins the !opClick gate:
// MOVE_OPCLICK (opcode 93) does NOT close the modal — the originating
// op-click already handled state. Per ts_asymmetry_dual_pin.md, this is
// the absence-pin paired with TestHandleMoveGameClickClosesChatModal.
func TestHandleMoveOpClickPreservesChatModal(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalChat = 100
	p.modalState |= modalStateChat

	payload := make([]byte, 5)
	payload[0] = 0
	payload[1] = byte(p.x >> 8)
	payload[2] = byte(p.x & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveOpClick(p, payload); err != nil {
		t.Fatalf("handleMoveOpClick: %v", err)
	}

	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (opClick=true must NOT close modal)", p.modalChat)
	}
	if p.modalState&modalStateChat == modalStateNone {
		t.Errorf("modalState chat bit cleared; opClick=true must preserve it")
	}
}
```

- [ ] **Step 2.8: Run; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMoveOpClickPreservesChatModal -count=1 -v`
Expected: PASS.

### Sub-task 2.9: Gate 1 — delayed gate

- [ ] **Step 2.9: Add delayed-gate test**

Append:

```go
// TestHandleMoveClickDelayedSendsUnsetMapFlag pins gate 1 of
// moveClickInner per TS MoveClickHandler.ts:11-13.
func TestHandleMoveClickDelayedSendsUnsetMapFlag(t *testing.T) {
	p, conn := newTestPlayer(t)
	p.modalChat = 100
	p.modalState |= modalStateChat
	p.delayed = true
	p.delayedUntil = p.client.server.currentTick + 5

	payload := make([]byte, 5)
	payload[1] = byte(p.x >> 8)
	payload[2] = byte(p.x & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	// Modal must NOT be cleared (handler short-circuits before
	// ClearPendingAction).
	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (delayed gate must short-circuit)", p.modalChat)
	}
	// UnsetMapFlag must have been emitted on the wire.
	flushClient(t, p)
	got := drainConn(t, conn)
	if !containsUnsetMapFlag(got) {
		t.Errorf("expected UnsetMapFlag opcode in conn output; got % x", got)
	}
}
```

**Note:** `flushClient`, `drainConn`, `containsUnsetMapFlag` may need to be reused from existing tests. Implementer should grep `handler_oploc_test.go` for the established UnsetMapFlag-assertion helpers and reuse them. If absent, inline the equivalent logic (the wire opcode for UNSET_MAPFLAG is grep-able from `gameserver` package).

- [ ] **Step 2.10: Run; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMoveClickDelayedSendsUnsetMapFlag -count=1 -v`
Expected: PASS.

### Sub-task 2.11: Gate 2 — ctrlHeld out of range

- [ ] **Step 2.11: Add ctrlHeld-bound test**

Append:

```go
// TestHandleMoveClickInvalidCtrlHeldRejects pins gate 2a per TS
// MoveClickHandler.ts:17 — ctrlHeld must be 0 or 1.
func TestHandleMoveClickInvalidCtrlHeldRejects(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalChat = 100
	p.modalState |= modalStateChat

	payload := make([]byte, 5)
	payload[0] = 5 // out of range
	payload[1] = byte(p.x >> 8)
	payload[2] = byte(p.x & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (gate 2 must short-circuit)", p.modalChat)
	}
}
```

- [ ] **Step 2.12: Run; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMoveClickInvalidCtrlHeldRejects -count=1 -v`
Expected: PASS.

### Sub-task 2.13: Gate 2 — distance > 104

- [ ] **Step 2.13: Add distance-bound test**

Append:

```go
// TestHandleMoveClickStartTooFarRejects pins gate 2b per TS
// MoveClickHandler.ts:17 — Chebyshev distance to start must be ≤ 104.
func TestHandleMoveClickStartTooFarRejects(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalChat = 100
	p.modalState |= modalStateChat

	farX := p.x + 200
	payload := make([]byte, 5)
	payload[0] = 0
	payload[1] = byte(farX >> 8)
	payload[2] = byte(farX & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (distance-gate must short-circuit)", p.modalChat)
	}
}
```

- [ ] **Step 2.14: Run; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMoveClickStartTooFarRejects -count=1 -v`
Expected: PASS.

### Sub-task 2.15: tempRun assignment + runenergy override

- [ ] **Step 2.15: Add tempRun tests**

Append:

```go
// TestHandleMoveGameClickSetsTempRunFromCtrlHeld pins TS
// MoveClickHandler.ts:46-50 — tempRun = ctrlHeld unless runenergy<100.
func TestHandleMoveGameClickSetsTempRunFromCtrlHeld(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.runenergy = 1000

	payload := make([]byte, 5)
	payload[0] = 1 // ctrlHeld = 1 (run requested)
	payload[1] = byte(p.x >> 8)
	payload[2] = byte(p.x & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}
	if p.tempRun != 1 {
		t.Errorf("tempRun: got %d, want 1", p.tempRun)
	}
}

// TestHandleMoveGameClickRunenergyLowSuppressesTempRun pins the
// runenergy<100 && ctrlHeld==1 → tempRun=0 override at TS L46-49.
func TestHandleMoveGameClickRunenergyLowSuppressesTempRun(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.runenergy = 50

	payload := make([]byte, 5)
	payload[0] = 1
	payload[1] = byte(p.x >> 8)
	payload[2] = byte(p.x & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}
	if p.tempRun != 0 {
		t.Errorf("tempRun: got %d, want 0 (runenergy<100 must suppress)", p.tempRun)
	}
}
```

- [ ] **Step 2.16: Run; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleMoveGameClick(SetsTempRunFromCtrlHeld|RunenergyLowSuppressesTempRun)" -count=1 -v`
Expected: 2 PASS.

### Sub-task 2.17: Walktrigger firing (PLAYERPACKET path)

- [ ] **Step 2.17: Add walktrigger-fires test**

Append:

```go
// TestHandleMoveGameClickFiresWalktriggerWhenPlayerpacket pins TS
// MoveClickHandler.ts:52-54 — under PLAYERPACKET (default), the move
// click fires processWalktrigger when hasWaypoints. The walktrigger
// field must be cleared (consumed) post-handler.
func TestHandleMoveGameClickFiresWalktriggerWhenPlayerpacket(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.walktrigger = 42 // queued walktrigger script id

	// Ensure cfg is PLAYERPACKET (default).
	if p.client.server.cfg.NodeWalktriggerSetting != WalkTriggerSettingPlayerpacket {
		t.Fatalf("test precondition: cfg must default to PLAYERPACKET")
	}

	// Move-click to an adjacent tile so hasWaypoints becomes true after
	// pathToMoveClick.
	dest := p.x + 1
	payload := make([]byte, 5)
	payload[0] = 0
	payload[1] = byte(dest >> 8)
	payload[2] = byte(dest & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	// processWalktrigger clears the field BEFORE the script-found check
	// (interaction.go:282-290). Even if no real script is registered, the
	// field gets cleared.
	if p.walktrigger != -1 {
		t.Errorf("walktrigger: got %d, want -1 (processWalktrigger should have consumed)", p.walktrigger)
	}
}

// TestHandleMoveGameClickSkipsWalktriggerWhenSettingNotPlayerpacket is
// the absence-pin: with cfg=PLAYERSETUP, handler-side walktrigger does
// NOT fire. Per ts_asymmetry_dual_pin.md.
func TestHandleMoveGameClickSkipsWalktriggerWhenSettingNotPlayerpacket(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.walktrigger = 42
	p.client.server.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup

	dest := p.x + 1
	payload := make([]byte, 5)
	payload[0] = 0
	payload[1] = byte(dest >> 8)
	payload[2] = byte(dest & 0xff)
	payload[3] = byte(p.z >> 8)
	payload[4] = byte(p.z & 0xff)

	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERSETUP must NOT fire handler-side)", p.walktrigger)
	}
}
```

- [ ] **Step 2.18: Run; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleMoveGameClick(FiresWalktriggerWhenPlayerpacket|SkipsWalktriggerWhenSettingNotPlayerpacket)" -count=1 -v`
Expected: 2 PASS.

### Sub-task 2.19: Full-suite green + commit

- [ ] **Step 2.19: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS across the board (no regressions in any package).

- [ ] **Step 2.20: Commit T2**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-77 T2 — moveClickInner + opClick wrapper pair

Replaces handleMoveClick with moveClickInner + handleMoveGameClick
(opClick=false) + handleMoveOpClick (opClick=true). Lands the !opClick
body (ClearPendingAction → CloseModal, tempRun + runenergy override,
processWalktrigger gated on PLAYERPACKET + hasWaypoints) plus the
delayed and ctrlHeld/distance gates currently absent. Mirrors TS
MoveClickHandler.ts:10-58. Symptom-2 (chatnpc click-away dismiss) is
fixed by this commit.
EOF
)"
```

---

## Task 3: `userPath` field + per-tick fallback

**Files:**
- Modify: `modules/world/player.go` (add `userPath []int` field)
- Modify: `modules/world/handlers_game.go:moveClickInner` (persist userPath; clear on gate-2 reject)
- Modify: `modules/world/tick.go` (add new per-player fallback phase invocation)
- Create or modify: `modules/world/walk_trigger_fallback.go` (new file for the fallback function) OR inline in `interaction.go` — implementer's call based on file-size discipline (see CLAUDE.md guidance: "files that change together should live together; split by responsibility").
- Modify: `modules/world/interaction_test.go` and/or new `modules/world/walk_trigger_fallback_test.go`

**Cadence:** Full TDD per behavioral change.

### Sub-task 3.1: Add `userPath` field + persistence

- [ ] **Step 3.1: Add `userPath []int` field to Player struct**

In `modules/world/player.go`, locate the Player struct (search for `type Player struct {`). Add the field next to other slice fields (near `runenergy`, `tempRun` cluster). Add a doc-comment:

```go
	// userPath is the most recent move-click path packed via
	// coordgrid.PackCoord. Persisted by moveClickInner for the per-tick
	// WalkTriggerSetting fallback (NAI-77 T3). Mirrors TS Player.userPath.
	// Default: nil (no pending path).
	userPath []int
```

- [ ] **Step 3.2: Persist userPath in moveClickInner**

In `modules/world/handlers_game.go`, locate `moveClickInner` (T2). Modify the gate-2 reject branch to clear userPath, and add the persist line after building `packed`:

Locate the gate-2 reject block:

```go
	if ctrlHeld < 0 || ctrlHeld > 1 || chebyshev > 104 {
		sendUnsetMapFlag(p)
		// T3 will also clear p.userPath here.
		return nil
	}
```

Replace with:

```go
	if ctrlHeld < 0 || ctrlHeld > 1 || chebyshev > 104 {
		sendUnsetMapFlag(p)
		p.userPath = nil
		return nil
	}
```

After the `packed` slice is built (just before the `p.client.log.Debug` line), add:

```go
	// Persist for per-tick WalkTriggerSetting fallback (T3).
	// Under client-routefinder, store the full path; otherwise store
	// only the dest. Mirrors TS MoveClickHandler.ts:23-37.
	if s.cfg.NodeClientRoutefinder {
		p.userPath = append(p.userPath[:0], packed...)
	} else {
		dest := packed[len(packed)-1]
		if cap(p.userPath) > 0 {
			p.userPath = p.userPath[:0]
		}
		p.userPath = append(p.userPath, dest)
	}
```

- [ ] **Step 3.3: Verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 3.4: Verify T2 tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMove -count=1 -v`
Expected: all T2 tests still PASS.

### Sub-task 3.5: Per-tick fallback — write failing test FIRST (RED)

- [ ] **Step 3.5: Create `walk_trigger_fallback_test.go` with the four setting-variant tests**

Create `modules/world/walk_trigger_fallback_test.go`:

```go
package world

import "testing"

// TestProcessWalkTriggerFallback_PlayerSetupFiresWhenNoOpCalled pins
// TS World.ts:638 — PLAYERSETUP fires walktrigger when !opcalled
// && hasWaypoints.
func TestProcessWalkTriggerFallback_PlayerSetupFiresWhenNoOpCalled(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	p.opcalled = false
	p.walktrigger = 42
	// Force hasWaypoints true: set userPath to a one-tile path AND
	// run pathToMoveClick once so waypointIndex >= 0.
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)
	if !p.hasWaypoints() {
		t.Fatalf("test precondition: hasWaypoints must be true")
	}

	processWalkTriggerFallback(p)

	if p.walktrigger != -1 {
		t.Errorf("walktrigger: got %d, want -1 (PLAYERSETUP fallback must consume)", p.walktrigger)
	}
}

// TestProcessWalkTriggerFallback_PlayerSetupSkipsWhenOpCalled is the
// absence-pin: with opcalled=true, fallback must NOT fire walktrigger.
func TestProcessWalkTriggerFallback_PlayerSetupSkipsWhenOpCalled(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	p.opcalled = true
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)

	processWalkTriggerFallback(p)

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (opcalled=true must skip)", p.walktrigger)
	}
}

// TestProcessWalkTriggerFallback_PlayerMovementSkipsWalktrigger pins
// TS World.ts:638 — PLAYERMOVEMENT re-paths but does NOT fire
// walktrigger (the gate is PLAYERSETUP-specific).
func TestProcessWalkTriggerFallback_PlayerMovementSkipsWalktrigger(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayermovement
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	processWalkTriggerFallback(p)

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERMOVEMENT must NOT fire walktrigger)", p.walktrigger)
	}
}

// TestProcessWalkTriggerFallback_PlayerPacketSkipsBranch pins TS
// World.ts:635 — under PLAYERPACKET the entire fallback is skipped.
func TestProcessWalkTriggerFallback_PlayerPacketSkipsBranch(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayerpacket
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	processWalkTriggerFallback(p)

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERPACKET fallback must skip entirely)", p.walktrigger)
	}
}
```

If `coordgrid` import isn't picked up by goimports, add manually:
```go
import "github.com/zsrv/goscape/pkg/coordgrid"
```

- [ ] **Step 3.6: Run; verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessWalkTriggerFallback -count=1 -v`
Expected: FAIL — `undefined: processWalkTriggerFallback`.

### Sub-task 3.7: Implement fallback function

- [ ] **Step 3.7: Create `walk_trigger_fallback.go`**

Create `modules/world/walk_trigger_fallback.go`:

```go
package world

// processWalkTriggerFallback is the per-player tick phase that mirrors
// TS World.ts:635-641. Skipped under WalkTriggerSettingPlayerpacket
// (the default — handler-side dispatch already covered the work).
//
// Under non-PLAYERPACKET settings:
//   - re-path from p.userPath each tick (mirrors TS L636
//     `player.pathToMoveClick(player.userPath, !NODE_CLIENT_ROUTEFINDER)`)
//   - PLAYERSETUP additionally fires processWalktrigger when
//     !opcalled && hasWaypoints (TS L638).
//   - PLAYERMOVEMENT re-paths only.
//
// Insertion phase: invoked from tick.go after processInteractions, NOT
// inside processInteraction itself (which is target-gated and would
// skip target-less players — wrong for plain move-click flows).
func processWalkTriggerFallback(p *Player) {
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayerpacket {
		return
	}

	p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)

	if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayersetup &&
		!p.opcalled && p.hasWaypoints() {
		p.processWalktrigger()
	}
}
```

- [ ] **Step 3.8: Run fallback tests; verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessWalkTriggerFallback -count=1 -v`
Expected: 4 PASS.

### Sub-task 3.9: Wire fallback into tick driver

- [ ] **Step 3.9: Add per-tick fallback phase invocation in `tick.go`**

In `modules/world/tick.go`, locate the line:

```go
		s.processInteractions()
```

(near line ~41 of the `runTickLoopWithRate` function).

Add a new method invocation immediately AFTER `processInteractions()`:

```go
		s.processInteractions()
		s.processWalkTriggerFallbacks() // NAI-77 T3: TS World.ts:635-641 per-tick re-path + PLAYERSETUP walktrigger
```

- [ ] **Step 3.10: Add the `Server.processWalkTriggerFallbacks` method**

In `modules/world/walk_trigger_fallback.go`, add:

```go
// processWalkTriggerFallbacks runs processWalkTriggerFallback once
// per active player per tick. Under default (PLAYERPACKET) cfg this
// is a per-player no-op; the iteration cost is negligible.
func (s *Server) processWalkTriggerFallbacks() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processWalkTriggerFallback", s.log)
			processWalkTriggerFallback(p)
		}(p)
	}
}
```

This mirrors the existing `processClientsIn` pattern at `tick.go:64-78` (player-loop iteration with recoverPlayer guard).

- [ ] **Step 3.11: Run full suite — verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS across the board.

- [ ] **Step 3.12: Commit T3**

```bash
git add modules/world/player.go modules/world/handlers_game.go modules/world/walk_trigger_fallback.go modules/world/walk_trigger_fallback_test.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-77 T3 — userPath persistence + per-tick walktrigger fallback

Adds Player.userPath []int field persisted by moveClickInner. Adds
processWalkTriggerFallback per-player tick phase that mirrors TS
World.ts:635-641 — under non-PLAYERPACKET settings, re-paths from
userPath each tick; PLAYERSETUP additionally fires processWalktrigger
when !opcalled && hasWaypoints. Fallback invoked from tick.go after
processInteractions per the same target-gated reasoning that pinned
processInteraction's early-return for target-less players.

Default cfg (PLAYERPACKET) is a per-player no-op.
EOF
)"
```

---

## Smoke handoff

After T3 commits successfully and `go test ./... -count=1` is green at HEAD, the controller hands off to the user for smoke per `smoke_test_server_handoff.md`. Smoke matrix:

1. **Symptom-2 (click-away):** Log in to Tutorial Island. Wait for chatnpc dialog (NAI-75/76 cascade prerequisite). Click any reachable ground tile. **PASS:** chatnpc dialog dismisses immediately + player walks to the clicked tile.
2. **Symptom-1 (door, NAI-78 territory):** Click the RS Guide door. Expected: still broken (NAI-78 routes the investigation). NAI-77 close should NOT block on this.
3. **Smoke regressions:** verify no new login errors in server log; verify movement under non-tutorial flows still works (open ::tele staff console if needed; walk around; click around).

If symptom-2 PASSES, NAI-77 closes regardless of symptom-1 status (binding by `cascade_theory_smoke_binding.md`).

---

## Close criteria checklist

- [ ] All three task commits land on `main`.
- [ ] `go test ./... -count=1` green from clean shell.
- [ ] Smoke matrix above: symptom-2 PASS pinned in close-commit body.
- [ ] Net deviation tally update (default 14 → 14 unless R3 forces a `NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE` open).
- [ ] `Closes memory:` trailer per `close_commit_memory_trailer.md`.
- [ ] NAI-78 candidate carry-forward listed in close commit body.

---

## Self-review (per writing-plans skill)

**Spec coverage:**
- §3.1 wrapper pattern → T2 sub-tasks 2.4-2.6 ✓
- §3.2 three gates → T2 sub-tasks 2.9-2.14 ✓
- §3.3 !opClick body (ClearPendingAction + tempRun + walktrigger) → T2 sub-tasks 2.2-2.6, 2.15-2.18 ✓
- §3.4 enum + config → T1 ✓
- §3.5 per-tick fallback → T3 sub-tasks 3.5-3.10 ✓
- §6 test strategy: all 9 handler tests + 4 fallback tests covered ✓
- §7 R1 (CloseModal immediate): verified at plan-write; no separate task needed ✓
- §7 R3 (insertion phase): codified as T3.9-3.10 with `processInteractions` follow-up ordering + recoverPlayer guard ✓
- §7 R5 (cfg access via `p.client.server.cfg`): T2 and T3 both use this pattern ✓

**Placeholder scan:** None found. All code blocks complete.

**Type consistency:**
- `WalkTriggerSetting` enum value names: `WalkTriggerSettingPlayerpacket / Playersetup / Playermovement` — consistent across T1 declaration, T2 handler refs, T3 fallback refs ✓
- `processWalkTriggerFallback` (free fn) vs `(*Server).processWalkTriggerFallbacks` (method) — naming pair consistent across T3.7, T3.9, T3.10 ✓
- `moveClickInner` signature `(p *Player, payload []byte, opClick bool) error` — consistent across T2.4-2.5 ✓
- `userPath []int` field name — consistent across T3.1, T3.2, T3.5 ✓

No issues found.
