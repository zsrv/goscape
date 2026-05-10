# NAI-146 Post-Decode Block Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS World.ts:611-641 per-tick post-decode player block, activating the inert `moveClickRequest` gate at `modules/world/movement.go:64` and folding NAI-77 walktrigger fallback into the TS-faithful slot.

**Architecture:** Extend `(p *Player) processIn` to call a new `processPostDecode()` method between the decode loop and `processInputTracking()`. The new method ports TS L611-641 directly: outer gates → delayed→unsetMapFlag → faceEntity reset → moveClickRequest setter → pathToTarget → walktrigger fallback. Retire the standalone `processWalkTriggerFallbacks` tick step (its work is folded into `processPostDecode`).

**Tech Stack:** Go 1.26+ per `go_version.md`. TS source canonical: `Engine-TS` only.

**Spec:** `docs/superpowers/specs/2026-05-10-nai-146-post-decode-block-port-design.md`

---

## File map

**Created:**
- `modules/world/player_post_decode.go` — `(p *Player) processPostDecode` (T3)
- `modules/world/player_post_decode_test.go` — T3 + T1 + T2 + T5 tests (single test file for the new phase)

**Modified:**
- `modules/world/player.go` — add `decodedThisTick bool` field (T1); thread reset/set through `processIn`; call `p.processPostDecode()` (T1, T3)
- `modules/world/interaction.go` — add `(p *Player) unsetMapFlag()` helper sibling of `sendUnsetMapFlag` (T2)
- `modules/world/tick.go` — delete `s.processWalkTriggerFallbacks()` invocation (T4)
- `modules/world/movement.go` — replace tracker doc-comment block (lines 53-59) with closed-tracker reference (T6)

**Deleted:**
- `modules/world/walk_trigger_fallback.go` — folded into `processPostDecode` (T4)
- `modules/world/walk_trigger_fallback_test.go` — superseded by T3f tests in new file (T4)

---

## Task dependency graph

```
T1 (decodedThisTick field) ── T3 ── T4 ── T5 ── T6
                              ╱
T2 (unsetMapFlag helper) ────╱
```

T1 and T2 are independent and can be implemented in parallel. T3 depends on both. T4–T6 strictly serial.

---

### Task 1: `decodedThisTick` field + processIn integration

**Files:**
- Modify: `modules/world/player.go` (struct field around line 297; `processIn` around lines 1058-1097)
- Test: `modules/world/player_post_decode_test.go` (NEW; T1 tests prepended)

- [ ] **Step 1.1: Write the failing tests**

Create `modules/world/player_post_decode_test.go`. Imports are added
incrementally per task — Go rejects unused imports, so each task's
implementer step must add only the imports its tests use.

```go
package world

import (
	"testing"
)

// TestProcessIn_DecodedThisTickResetAtStart pins T1a: at the top of
// processIn, decodedThisTick is reset to false BEFORE the decode loop
// runs. Sentinel: pre-set to true; a no-read processIn must reset.
func TestProcessIn_DecodedThisTickResetAtStart(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.decodedThisTick = true // poison from prior tick
	// No bytes in c.in → decode loop reads zero packets.

	p.processIn(0)

	if p.decodedThisTick {
		t.Error("decodedThisTick: want false after no-read processIn (must reset before decode loop)")
	}
}

// TestProcessIn_DecodedThisTickStaysFalseOnNoRead pins T1c: after a
// processIn tick that read zero packets, decodedThisTick is false.
// Equivalent intent to TS decodeIn() returning false.
func TestProcessIn_DecodedThisTickStaysFalseOnNoRead(t *testing.T) {
	p, _ := newTestPlayer(t)
	// No bytes in c.in.

	p.processIn(0)

	if p.decodedThisTick {
		t.Error("decodedThisTick: want false on no-read tick")
	}
}

// TestProcessIn_DecodedThisTickSetAfterRead pins T1b: after processIn
// reads ≥1 packet, decodedThisTick is true. Uses NO_TIMEOUT (op 108,
// 0-payload) — same pattern as TestReadPacketNoTimeoutConsumesAndResetsOpcode.
func TestProcessIn_DecodedThisTickSetAfterRead(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// Op 108 = NO_TIMEOUT, payload 0.
	p.client.in.Write([]byte{encryptOpcode(enc, 108)})

	p.processIn(0)

	if !p.decodedThisTick {
		t.Error("decodedThisTick: want true after reading ≥1 packet")
	}
}
```

- [ ] **Step 1.2: Run tests; verify they fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessIn_DecodedThisTick' -v
```

Expected: build error — `p.decodedThisTick undefined`. (Or one of three failing assertions if the field exists but isn't wired.)

- [ ] **Step 1.3: Add the field**

In `modules/world/player.go` at line ~297 (existing `afkEventReady, moveClickRequest bool` declaration):

Replace:
```go
	afkEventReady, moveClickRequest              bool
```
With:
```go
	afkEventReady, moveClickRequest, decodedThisTick bool
```

- [ ] **Step 1.4: Wire reset + set in processIn**

In `modules/world/player.go` `processIn` (around lines 1060-1092), find the existing block:

```go
	p.userLimit = 0
	p.clientLimit = 0
	p.restrictedLimit = 0
	p.opcalled = false

	c.inMu.Lock()
	defer c.inMu.Unlock()

	readAny := false
	for p.userLimit < userEventLimit &&
		p.clientLimit < clientEventLimit &&
		p.restrictedLimit < restrictedEventLimit {
```

Insert `p.decodedThisTick = false` before `c.inMu.Lock()`:

```go
	p.userLimit = 0
	p.clientLimit = 0
	p.restrictedLimit = 0
	p.opcalled = false
	p.decodedThisTick = false // NAI-146 T1: reset before decode (TS decodeIn() return semantics)

	c.inMu.Lock()
	defer c.inMu.Unlock()

	readAny := false
	for p.userLimit < userEventLimit &&
		p.clientLimit < clientEventLimit &&
		p.restrictedLimit < restrictedEventLimit {
```

Then in the same function find the `if readAny` block (around line 1090):

```go
	if readAny {
		p.lastResponse = currentTick // mirrors TS decodeIn() line 80
	}

	// NAI-73: per-tick input-tracking dispatch. Mirrors TS World.ts:646
	// placement (last step of per-player client-input phase iteration).
	p.processInputTracking(currentTick)
```

Set `decodedThisTick` after the `if readAny` block:

```go
	if readAny {
		p.lastResponse = currentTick // mirrors TS decodeIn() line 80
	}
	p.decodedThisTick = readAny // NAI-146 T1: TS decodeIn() return value

	// NAI-73: per-tick input-tracking dispatch. Mirrors TS World.ts:646
	// placement (last step of per-player client-input phase iteration).
	p.processInputTracking(currentTick)
```

(The `processPostDecode()` call goes between these in T3 — leave a TODO placeholder NOT in the code, just mentally; T3 will edit again.)

- [ ] **Step 1.5: Run tests; verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessIn_DecodedThisTick' -v
```

Expected: 3 PASS.

- [ ] **Step 1.6: Run full package tests; verify no regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Expected: all green.

- [ ] **Step 1.7: Commit**

```
git add modules/world/player.go modules/world/player_post_decode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T1 — decodedThisTick field + processIn wiring

Mirrors TS NetworkPlayer.decodeIn() boolean return semantics. Reset
to false at top of processIn (before decode loop); set to readAny
after the loop. Read by NAI-146 T3 processPostDecode outer gate.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `(p *Player) unsetMapFlag()` helper

**Files:**
- Modify: `modules/world/interaction.go` (after line 46 `sendUnsetMapFlag` definition)
- Test: `modules/world/player_post_decode_test.go` (append T2 test)

- [ ] **Step 2.1: Write the failing test**

Append to `modules/world/player_post_decode_test.go`. T2 introduces
two new imports — merge into the existing import block (added in
Step 1.1):

```go
import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)
```

Append the test:

```go
// TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket pins T2: the
// new helper bundles clearWaypoints (waypointIndex=-1) + the
// OpUnsetMapFlag wire write. Mirrors TS Player.unsetMapFlag
// (Player.ts:2169-2172). Sibling pattern to
// TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket.
func TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.waypointIndex = 5

	// Sibling decoder seeded from same key for opcode comparison.
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	// Start drain BEFORE the action; drainConn requires this ordering.
	received := drainConn(t, cc)
	p.unsetMapFlag()
	p.client.flushWrite()
	emitted := <-received

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (clearWaypoints arm of bundle)", p.waypointIndex)
	}
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d)", gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag)", emitted[0], wantEnc)
	}
}
```

(`drainConn` is package-internal — defined in
`modules/world/stat_update_test.go:119` — and accessible from this
test file without import.)

- [ ] **Step 2.2: Run test; verify it fails**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket' -v
```

Expected: build error — `p.unsetMapFlag undefined`.

- [ ] **Step 2.3: Add the helper**

In `modules/world/interaction.go`, immediately after `func sendUnsetMapFlag` (around line 46), insert:

```go
// unsetMapFlag clears the player's waypoint queue and emits the
// OpUnsetMapFlag packet. Mirrors TS Player.unsetMapFlag
// (Engine-TS/.../Player.ts:2169-2172) — the bundled
// clearWaypoints + write helper. Distinct from the wire-only
// sendUnsetMapFlag(p), which is preserved for decode-time handler
// call sites that already manage waypoint state inline.
//
// Per memory ts_helper_method_bundles.md: when porting a TS site
// that calls unsetMapFlag(), use this method, not sendUnsetMapFlag.
func (p *Player) unsetMapFlag() {
	p.waypointIndex = -1
	sendUnsetMapFlag(p)
}
```

- [ ] **Step 2.4: Run test; verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket' -v
```

Expected: PASS.

- [ ] **Step 2.5: Run full package tests; verify no regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Expected: all green.

- [ ] **Step 2.6: Commit**

```
git add modules/world/interaction.go modules/world/player_post_decode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T2 — (p *Player) unsetMapFlag helper

Bundles clearWaypoints (waypointIndex=-1) + OpUnsetMapFlag wire
write per TS Player.unsetMapFlag. Sibling of wire-only
sendUnsetMapFlag, which is preserved for decode-time handler call
sites that manage waypoint state inline.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `processPostDecode` block port

**Files:**
- Create: `modules/world/player_post_decode.go`
- Modify: `modules/world/player.go` (insert call in `processIn` between `decodedThisTick = readAny` and `processInputTracking`)
- Test: `modules/world/player_post_decode_test.go` (append T3a–T3f tests)

This is the largest task. It has many branch tests; we write them in groups by sub-block, each group its own write-fail-implement-pass-commit cycle.

#### Task 3 setup: shared test fixture

- [ ] **Step 3.0: Add shared fixture helper to test file**

T3 setup adds `net` (for the `net.Conn` return type in the fixture
signature). Update the file's import block:

```go
import (
	"net"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)
```

Append the helpers to `modules/world/player_post_decode_test.go`
(above any T3 test):

```go
// newPostDecodeTestPlayerWithConn wires a Player with the minimum
// scaffolding required to drive (p *Player) processPostDecode
// end-to-end:
//   - p.client.server (with default cfg: PLAYERPACKET, routefinder=true)
//   - p.decodedThisTick = true       (outer gate satisfied)
//   - p.userPath set to one packed coord (outer gate satisfied)
//   - p.faceEntity = -1              (faceEntity branch no-op by default)
//   - p.moveClickRequest = false     (sentinel)
//
// Returns (player, server, conn). Most branch tests don't need the
// conn (use the wrapper newPostDecodeTestPlayer). Wire-asserting
// branches (T3b delayed) drain the conn directly.
func newPostDecodeTestPlayerWithConn(t *testing.T) (*Player, *Server, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	s := &Server{
		log: discardLogger(),
		cfg: Config{
			NodeWalktriggerSetting: WalkTriggerSettingPlayerpacket,
			NodeClientRoutefinder:  true,
		},
	}
	p.client.server = s
	p.decodedThisTick = true
	p.userPath = []int{0x12345}
	p.faceEntity = -1
	p.moveClickRequest = false
	return p, s, cc
}

// newPostDecodeTestPlayer is a wrapper that discards the conn for
// branch tests that don't drain wire output.
func newPostDecodeTestPlayer(t *testing.T) (*Player, *Server) {
	t.Helper()
	p, s, _ := newPostDecodeTestPlayerWithConn(t)
	return p, s
}
```

(No test runs against this helper alone; it's consumed by 3a–3f tests below.)

#### T3a — outer gates

- [ ] **Step 3.1a: Write T3a tests**

Append:

```go
// TestProcessPostDecode_OuterGateSkipsWhenNotDecoded pins TS L611
// `decodeIn()` short-circuit. With decodedThisTick=false the entire
// block is skipped — moveClickRequest is NOT set even when userPath
// AND opcalled would otherwise satisfy L613.
func TestProcessPostDecode_OuterGateSkipsWhenNotDecoded(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.decodedThisTick = false
	p.opcalled = true // would otherwise satisfy L613

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (block skipped on !decodedThisTick)")
	}
}

// TestProcessPostDecode_OuterGateSkipsWhenIdle pins TS L613 outer
// gate. With userPath empty AND !opcalled, the block returns early.
// moveClickRequest stays at its sentinel.
func TestProcessPostDecode_OuterGateSkipsWhenIdle(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.userPath = nil
	p.opcalled = false

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (block skipped on !userPath && !opcalled)")
	}
}
```

- [ ] **Step 3.2a: Run; expect build error**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessPostDecode_OuterGate' -v
```

Expected: build error — `p.processPostDecode undefined`.

- [ ] **Step 3.3a: Create the file with outer gates only**

Create `modules/world/player_post_decode.go` (no imports yet — T3c
adds `entitypkg`):

```go
package world

// processPostDecode runs the per-tick post-decode block at TS
// Engine-TS/src/engine/World.ts:611-641. Called from end of processIn,
// before processInputTracking (matching TS L611-646 ordering).
//
// Activates the NAI-144 moveClickRequest gate at movement.go:64 by
// porting the L624-628 setter. Folds in the NAI-77 walktrigger
// fallback (L635-641), retiring processWalkTriggerFallbacks; this
// also closes NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE by shifting
// the fallback from after-processPathing to before-processPathing
// (TS-faithful slot).
func (p *Player) processPostDecode() {
	// TS L611: isClientConnected(player) && player.decodeIn()
	if !p.decodedThisTick {
		return
	}
	// TS L613: userPath.length > 0 || opcalled
	if len(p.userPath) == 0 && !p.opcalled {
		return
	}

	// (goscape defensive; TS skips this check) — server may be nil in
	// fixtures that don't wire p.client.server. Bail safely.
	if p.client == nil || p.client.server == nil {
		return
	}
}
```

- [ ] **Step 3.4a: Run T3a tests; verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessPostDecode_OuterGate' -v
```

Expected: 2 PASS.

- [ ] **Step 3.5a: Commit**

```
git add modules/world/player_post_decode.go modules/world/player_post_decode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T3a — processPostDecode outer gates

Skeleton with TS World.ts:611+613 outer gates (decodedThisTick +
userPath/opcalled). Defensive nil-server bail for test fixtures.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

#### T3b — delayed branch

- [ ] **Step 3.1b: Write T3b test**

Append to test file:

```go
// TestProcessPostDecode_DelayedFiresUnsetMapFlagAndReturns pins TS
// L614-617: when delayed AND outer gate satisfied (userPath set),
// unsetMapFlag fires (waypointIndex=-1 + OpUnsetMapFlag) and the
// block returns BEFORE the faceEntity reset / moveClickRequest setter.
//
// newPostDecodeTestPlayer (defined below) returns the conn alongside
// the player so this test can drainConn the wire output.
func TestProcessPostDecode_DelayedFiresUnsetMapFlagAndReturns(t *testing.T) {
	p, _, cc := newPostDecodeTestPlayerWithConn(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	p.delayed = true
	p.waypointIndex = 5    // would clear under unsetMapFlag bundle
	p.faceEntity = 42      // would reset if delayed branch DIDN'T return
	p.moveClickRequest = true // sentinel — must NOT be touched after return

	received := drainConn(t, cc)
	p.processPostDecode()
	p.client.flushWrite()
	emitted := <-received

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (delayed → unsetMapFlag bundle)", p.waypointIndex)
	}
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d)", gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag)", emitted[0], wantEnc)
	}
	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (delayed branch must return BEFORE faceEntity reset)", p.faceEntity)
	}
	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (sentinel preserved — delayed branch must return BEFORE setter)")
	}
}
```

- [ ] **Step 3.2b: Run; expect failure**

Expected: assertions fail (delayed branch not implemented).

- [ ] **Step 3.3b: Implement delayed branch**

In `modules/world/player_post_decode.go`, append the delayed branch after the nil-server defensive bail:

```go
	// TS L614-617: delayed → unsetMapFlag and skip the rest of the block.
	if p.delayed {
		p.unsetMapFlag()
		return
	}
```

- [ ] **Step 3.4b: Run T3b; verify pass**

Expected: PASS.

- [ ] **Step 3.5b: Commit**

```
git add modules/world/player_post_decode.go modules/world/player_post_decode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T3b — delayed → unsetMapFlag → return

Ports TS World.ts:614-617. Bundles waypoint clear + wire write via
the NAI-146 T2 unsetMapFlag helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

#### T3c — faceEntity reset

- [ ] **Step 3.1c: Write T3c tests**

T3c adds `entitypkg` to test file imports:

```go
import (
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)
```

Append the tests:

```go
// TestProcessPostDecode_FaceEntityResetForLocTarget pins TS L619-622
// for *Loc target: faceEntity reset to -1, masks |= entitymask.
func TestProcessPostDecode_FaceEntityResetForLocTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Loc{} // any *Loc satisfies the type-switch
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true // satisfies outer L613 gate

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Loc target → reset)", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit (%d) not set; got masks=%d", p.entitymask, p.masks)
	}
}

// TestProcessPostDecode_FaceEntityResetForObjTarget pins same for *Obj.
func TestProcessPostDecode_FaceEntityResetForObjTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Obj{}
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Obj target → reset)", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit (%d) not set; got masks=%d", p.entitymask, p.masks)
	}
}

// TestProcessPostDecode_FaceEntityResetForNilTarget pins TS L619 nil
// target arm: nil target + faceEntity!=-1 → reset.
func TestProcessPostDecode_FaceEntityResetForNilTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = nil
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (nil target → reset)", p.faceEntity)
	}
}

// TestProcessPostDecode_FaceEntityPreservedForPlayerTarget pins the
// negative arm: PathingEntity targets (Player/Npc) do NOT trigger
// the faceEntity reset.
func TestProcessPostDecode_FaceEntityPreservedForPlayerTarget(t *testing.T) {
	s := newTestServer(t)
	other := newTestPlayerAt(t, s, 2, 3200, 3200, 0)
	p, _ := newPostDecodeTestPlayer(t)
	p.target = other
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (Player target → preserved)", p.faceEntity)
	}
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0 (Player target → masks NOT touched)", p.masks)
	}
}

// TestProcessPostDecode_FaceEntityNoOpWhenAlreadyMinusOne pins TS L620
// guard: when faceEntity is already -1, masks is NOT touched.
func TestProcessPostDecode_FaceEntityNoOpWhenAlreadyMinusOne(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Loc{}
	p.faceEntity = -1 // guard: already cleared
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (no-op preserves)", p.faceEntity)
	}
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0 (faceEntity already -1 → masks NOT touched)", p.masks)
	}
}
```

- [ ] **Step 3.2c: Run; expect failure**

Expected: faceEntity assertions fail (branch not implemented).

- [ ] **Step 3.3c: Implement faceEntity reset**

In `modules/world/player_post_decode.go`, add the `entitypkg` import
to the file's import block:

```go
import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)
```

Then append the faceEntity branch INSIDE `processPostDecode` after the
delayed branch (just before the closing brace):

```go
	// TS L619-622: faceEntity reset for non-PathingEntity targets.
	if p.faceEntity != -1 {
		switch p.target.(type) {
		case nil, *entitypkg.Loc, *entitypkg.Obj:
			p.faceEntity = -1
			p.masks |= p.entitymask
		}
	}
```

- [ ] **Step 3.4c: Run T3c; verify pass**

Expected: 5 PASS.

- [ ] **Step 3.5c: Commit**

```
git add modules/world/player_post_decode.go modules/world/player_post_decode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T3c — faceEntity reset for non-PathingEntity

Ports TS World.ts:619-622. Type-switch on p.target covers nil /
*Loc / *Obj; PathingEntity targets (Player/Npc) preserve faceEntity.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

#### T3d — moveClickRequest setter

- [ ] **Step 3.1d: Write T3d tests**

Append:

```go
// TestProcessPostDecode_MoveClickRequest_NotBusyOpcalled pins TS L624-625
// branch: !busy() && opcalled → moveClickRequest = false.
func TestProcessPostDecode_MoveClickRequest_NotBusyOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = true
	p.delayed = false
	p.modalState = modalStateNone
	p.moveClickRequest = true // sentinel — must flip to false
	p.targetOp = -1           // disable pathToTarget branch (-1 ≠ 3, but opcalled=true → would fire)
	// Need to also block the pathToTarget branch from firing first:
	// !followingPlayer && opcalled && (len(userPath)==0 || !routefinder).
	// With routefinder=true (default) AND len(userPath)>0, the gate fails →
	// pathToTarget skipped. userPath is set in the fixture; routefinder=true default.

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (!Busy + opcalled)")
	}
}

// TestProcessPostDecode_MoveClickRequest_BusyOpcalled pins TS L626-627
// branch: Busy + opcalled → moveClickRequest = true.
func TestProcessPostDecode_MoveClickRequest_BusyOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = true
	p.modalState = modalStateMain // Busy() = true
	p.delayed = false
	p.moveClickRequest = false // sentinel — must flip to true

	p.processPostDecode()

	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (Busy + opcalled)")
	}
}

// TestProcessPostDecode_MoveClickRequest_NotBusyNotOpcalled pins TS
// L626-627 else-branch: !Busy + !opcalled + userPath set → moveClickRequest = true.
func TestProcessPostDecode_MoveClickRequest_NotBusyNotOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = false
	p.delayed = false
	p.modalState = modalStateNone
	p.moveClickRequest = false // sentinel

	p.processPostDecode()

	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (!Busy + !opcalled + userPath set)")
	}
}
```

- [ ] **Step 3.2d: Run; expect failure**

Expected: assertions fail (setter not implemented).

- [ ] **Step 3.3d: Implement setter**

In `modules/world/player_post_decode.go`, append after the faceEntity branch:

```go
	// TS L624-628: moveClickRequest setter. Activates the gate at
	// modules/world/movement.go:64 (NAI-144 — previously inert at HEAD).
	if !p.Busy() && p.opcalled {
		p.moveClickRequest = false
	} else {
		p.moveClickRequest = true
	}
```

- [ ] **Step 3.4d: Run T3d; verify pass**

Expected: 3 PASS.

- [ ] **Step 3.5d: Commit**

```
git add modules/world/player_post_decode.go modules/world/player_post_decode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T3d — moveClickRequest setter (gate activation)

Ports TS World.ts:624-628. Activates the NAI-144 gate at
movement.go:64 by populating moveClickRequest based on busy() +
opcalled. Closes NAI-144-D-MoveClickRequestSetter as a side-effect
(tracker doc-comment retired in T6).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

#### T3e — pathToTarget branch

The pathToTarget branch is tricky to test directly because `(p *Player) pathToTarget()` mutates waypoint state through the pathfinder (which may not be wired in unit tests). We use **side-effect signals**: pathToTarget short-circuits to a no-op when target is nil (interaction.go:672). To detect "pathToTarget was called", we use a different signal — the TS L633 `continue` (which we map to `return` in goscape) means the walktrigger fallback at L635-641 does NOT run. We pin "pathToTarget branch fires" by asserting the walktrigger fallback's PLAYERSETUP path does NOT run (via the `walktrigger` field staying intact when it would otherwise be consumed).

- [ ] **Step 3.1e: Write T3e tests**

T3e adds `coordgrid` to test file imports:

```go
import (
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/coordgrid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)
```

Append the tests:

```go
// TestProcessPostDecode_PathToTargetFiresAndReturns pins TS L630-633:
// when !followingPlayer && opcalled && (userPath==0 || !routefinder),
// pathToTarget runs and the block returns BEFORE the walktrigger
// fallback. Sentinel: walktrigger=42 with PLAYERSETUP cfg would be
// consumed by the fallback if it ran; we assert it survives.
func TestProcessPostDecode_PathToTargetFiresAndReturns(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	s.cfg.NodeClientRoutefinder = false // forces the pathToTarget gate
	p.opcalled = true
	p.targetOp = 1 // not 3 → !followingPlayer
	p.target = nil // pathToTarget short-circuits to no-op (interaction.go:672)
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42 // sentinel — would be consumed by fallback if it ran
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	p.processPostDecode()

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (pathToTarget branch must return BEFORE walktrigger fallback)", p.walktrigger)
	}
}

// TestProcessPostDecode_PathToTargetSkippedForFollowingPlayer pins
// TS L630 followingPlayer guard: targetOp==3 → pathToTarget NOT called
// → walktrigger fallback proceeds.
//
// Signal: waypointIndex starts at -1; if fallback runs,
// pathToMoveClick → queueWaypoints sets it to >=0.
// (We can't use walktrigger here: PLAYERSETUP fallback only fires
// processWalktrigger when !opcalled, and we need opcalled=true to
// satisfy the other clauses of the pathToTarget gate.)
func TestProcessPostDecode_PathToTargetSkippedForFollowingPlayer(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayermovement // re-paths but no walktrigger
	s.cfg.NodeClientRoutefinder = false                              // gate's userPath/routefinder clause WOULD pass
	p.opcalled = true                                                // satisfies opcalled clause
	p.targetOp = 3                                                   // followingPlayer → BLOCKS gate at first clause
	p.delayed = false
	p.modalState = modalStateNone
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.waypointIndex = -1 // sentinel

	p.processPostDecode()

	if !p.hasWaypoints() {
		t.Errorf("waypointIndex: got %d, want >= 0 (followingPlayer skips pathToTarget → fallback's pathToMoveClick re-paths)", p.waypointIndex)
	}
}

// TestProcessPostDecode_PathToTargetSkippedWhenRoutefinderAndUserPath
// pins TS L630 third gate-clause: with NodeClientRoutefinder=true AND
// userPath non-empty, the pathToTarget branch is skipped (the
// disjunction `len(userPath)==0 || !routefinder` is false →
// pathToTarget gate fails) → walktrigger fallback proceeds and
// re-paths userPath via pathToMoveClick.
//
// Signal: waypointIndex starts at -1 sentinel; if fallback runs,
// pathToMoveClick → queueWaypoints sets it to >=0.
// (We can't use walktrigger as signal here: opcalled=true is required
// to satisfy the OTHER clauses of the pathToTarget gate, but the
// PLAYERSETUP walktrigger sub-branch requires !opcalled — so
// walktrigger would NOT be consumed even if fallback proceeded.)
func TestProcessPostDecode_PathToTargetSkippedWhenRoutefinderAndUserPath(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayermovement // re-path only; no walktrigger
	s.cfg.NodeClientRoutefinder = true
	p.opcalled = true                    // satisfies pathToTarget opcalled clause
	p.targetOp = 1                       // !followingPlayer
	p.delayed = false
	p.modalState = modalStateNone
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.waypointIndex = -1                 // sentinel; fallback's pathToMoveClick should set >=0

	p.processPostDecode()

	if !p.hasWaypoints() {
		t.Errorf("waypointIndex: got %d, want >= 0 (routefinder+userPath skips pathToTarget → fallback's pathToMoveClick re-paths)", p.waypointIndex)
	}
}
```

(Plan-author note: T3e signals are subtle. Each test is documented to explain why the chosen assertion isolates the branch.)

- [ ] **Step 3.2e: Run; expect failure**

Expected: assertions fail (pathToTarget branch + walktrigger fallback not implemented).

- [ ] **Step 3.3e: Implement pathToTarget branch**

In `modules/world/player_post_decode.go`, append after the moveClickRequest setter:

```go
	s := p.client.server

	// TS L630-633: pathToTarget when op-driven and not following a
	// player. followingPlayer = (targetOp == 3) per
	// modules/world/interaction.go:140-146 (goscape stores raw op
	// slot 1..4; APPLAYER3 / OPPLAYER3 both map to 3).
	followingPlayer := p.targetOp == 3
	if !followingPlayer && p.opcalled &&
		(len(p.userPath) == 0 || !s.cfg.NodeClientRoutefinder) {
		p.pathToTarget()
		return
	}
```

- [ ] **Step 3.4e: Run T3e; verify pass**

Expected: 3 PASS (note: `T3e_SkippedWhenRoutefinder` test also needs T3f walktrigger fallback; if this fails, defer to step 3.3f).

If `TestProcessPostDecode_PathToTargetSkippedWhenRoutefinderAndUserPath` fails because the walktrigger fallback isn't yet implemented, that's expected — proceed to T3f and re-run after.

- [ ] **Step 3.5e: Commit**

```
git add modules/world/player_post_decode.go modules/world/player_post_decode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T3e — pathToTarget branch

Ports TS World.ts:630-633. followingPlayer = (targetOp==3) per
goscape's raw-op-slot convention. Returns from block before
walktrigger fallback when branch fires.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

#### T3f — walktrigger fallback (folded from NAI-77)

- [ ] **Step 3.1f: Write T3f tests**

T3f adds `script` to test file imports:

```go
import (
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/coordgrid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/script"
)
```

Append the tests:

```go
// TestProcessPostDecode_WalktriggerFallback_PlayerpacketNoOp pins TS
// L635 cfg gate: under PLAYERPACKET (default) the fallback is a no-op.
// Sentinel: walktrigger=42 + hasWaypoints — walktrigger MUST survive.
func TestProcessPostDecode_WalktriggerFallback_PlayerpacketNoOp(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	// Default cfg in fixture is PLAYERPACKET.
	p.opcalled = false
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42
	p.waypointIndex = 0 // hasWaypoints → true

	p.processPostDecode()

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERPACKET cfg → fallback no-op)", p.walktrigger)
	}
}

// TestProcessPostDecode_WalktriggerFallback_Playersetup_FiresWhenNotOpcalled
// pins TS L638: PLAYERSETUP + !opcalled + hasWaypoints →
// processWalktrigger fires (consumes walktrigger field).
func TestProcessPostDecode_WalktriggerFallback_Playersetup_FiresWhenNotOpcalled(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	p.opcalled = false
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(&script.ScriptFile{
		Name:             "[walk_test_setup_fires]",
		LookupKey:        42,
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	})

	p.processPostDecode()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger: got %d, want -1 (PLAYERSETUP + !opcalled → processWalktrigger consumed)", p.walktrigger)
	}
}

// TestProcessPostDecode_WalktriggerFallback_Playersetup_SkipsWhenOpcalled
// pins TS L638 !opcalled guard: opcalled=true → re-path runs but
// processWalktrigger does NOT fire.
func TestProcessPostDecode_WalktriggerFallback_Playersetup_SkipsWhenOpcalled(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	s.cfg.NodeClientRoutefinder = true // also forces pathToTarget gate to skip (userPath set)
	p.opcalled = true
	p.targetOp = 3 // followingPlayer → bypasses pathToTarget branch
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	p.processPostDecode()

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERSETUP + opcalled=true → walktrigger NOT fired)", p.walktrigger)
	}
}
```

- [ ] **Step 3.2f: Run; expect failure**

Expected: assertions fail.

- [ ] **Step 3.3f: Implement walktrigger fallback**

In `modules/world/player_post_decode.go`, append at the end of the function:

```go
	// TS L635-641: non-PLAYERPACKET re-path + PLAYERSETUP walktrigger.
	// Folded from NAI-77 processWalkTriggerFallbacks; restores the
	// TS-faithful pre-processPathing slot, closing
	// NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE.
	if s.cfg.NodeWalktriggerSetting != WalkTriggerSettingPlayerpacket {
		p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)
		if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayersetup &&
			!p.opcalled && p.hasWaypoints() {
			p.processWalktrigger()
		}
	}
```

- [ ] **Step 3.4f: Run all T3 tests; verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessPostDecode_' -v
```

Expected: all T3a–T3f tests PASS.

- [ ] **Step 3.5f: Wire processPostDecode into processIn**

In `modules/world/player.go` `processIn`, find the line added in T1.4 `p.decodedThisTick = readAny` and insert the call after it:

```go
	if readAny {
		p.lastResponse = currentTick // mirrors TS decodeIn() line 80
	}
	p.decodedThisTick = readAny // NAI-146 T1: TS decodeIn() return value

	p.processPostDecode() // NAI-146 T3: TS World.ts:611-641

	// NAI-73: per-tick input-tracking dispatch …
	p.processInputTracking(currentTick)
```

- [ ] **Step 3.6f: Run full package tests; verify no regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Expected: all green.

NOTE: at this checkpoint the OLD `processWalkTriggerFallbacks` step is STILL invoked from tick.go:60 — the new phase fires alongside it. T4 retires the old step. The duplication is intentional during T3 to keep T3 commits self-contained; T4 cleans up.

- [ ] **Step 3.7f: Commit**

```
git add modules/world/player_post_decode.go modules/world/player_post_decode_test.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-146 T3f — walktrigger fallback fold + processIn wiring

Ports TS World.ts:635-641 by folding NAI-77 processWalkTriggerFallback
into the new processPostDecode phase at the TS-faithful slot
(before processPathing). processIn now invokes processPostDecode
between the decode loop and processInputTracking.

The standalone processWalkTriggerFallbacks tick step at tick.go:60
remains live alongside the folded fallback during this commit; T4
retires it.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Retire `processWalkTriggerFallbacks` step

The standalone `processWalkTriggerFallbacks` tick step is now redundant — its logic is folded into `processPostDecode` at a TS-faithful slot. We delete the file, the tick.go invocation, and the dedicated test file (the new T3f tests cover the same scope).

**Files:**
- Modify: `modules/world/tick.go` (delete line 60 invocation)
- Delete: `modules/world/walk_trigger_fallback.go`
- Delete: `modules/world/walk_trigger_fallback_test.go`

- [ ] **Step 4.1: Delete tick.go invocation**

In `modules/world/tick.go` around line 60, remove the line:

```go
		s.processWalkTriggerFallbacks() // NAI-77 T3: TS World.ts:635-641 per-tick re-path + PLAYERSETUP walktrigger
```

(Make sure surrounding lines for `processInteractions` and `processEnergy` remain.)

- [ ] **Step 4.2: Delete the old file**

```
git rm modules/world/walk_trigger_fallback.go modules/world/walk_trigger_fallback_test.go
```

- [ ] **Step 4.3: Run full package tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Expected: all green. The new T3f tests (in `player_post_decode_test.go`) cover the PLAYERPACKET / PLAYERSETUP branches that the deleted file's tests covered.

- [ ] **Step 4.4: Confirm no dangling references**

```
grep -rE "processWalkTriggerFallback" modules/ pkg/ cmd/
```

Expected: zero hits.

- [ ] **Step 4.5: Commit**

```
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(player): NAI-146 T4 — retire processWalkTriggerFallbacks step

Folded into NAI-146 T3 processPostDecode at the TS-faithful
pre-processPathing slot. Closes
NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE: the standalone step ran
AFTER processPathing (declared phase-choice deviation); the folded
fallback restores TS World.ts:611-641 ordering.

Tests for PLAYERPACKET / PLAYERSETUP / PLAYERMOVEMENT cfg branches
are preserved in modules/world/player_post_decode_test.go (T3f).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: End-to-end gate-activation test

Verify the full path: setting up a player who would normally walk → driving processPostDecode → driving resolveMovement → asserting movement is suppressed.

**Files:**
- Modify: `modules/world/player_movement_gate_test.go` (append T5 test)

- [ ] **Step 5.1: Write the failing test**

Append to `modules/world/player_movement_gate_test.go`:

```go
// TestProcessPostDecodeActivatesGate is the end-to-end pin closing
// NAI-144-D-MoveClickRequestSetter: with a busy player who issued a
// move-click (userPath set) and has queued script work, the
// post-decode block sets moveClickRequest=true and the gate at
// resolveMovement returns early.
func TestProcessPostDecodeActivatesGate(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = &Server{
		log: discardLogger(),
		cfg: Config{
			NodeWalktriggerSetting: WalkTriggerSettingPlayerpacket,
			NodeClientRoutefinder:  true,
		},
	}
	p.x, p.z, p.level = 3200, 3200, 0
	p.lastTickX, p.lastTickZ, p.lastLevel = 3200, 3200, 0

	// Stage gate-firing prerequisites.
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0
	p.delayed = true // makes Busy() true
	p.opcalled = false
	p.queue = append(p.queue, playerQueueRequest{
		Script: &script.ScriptFile{Name: "[blocker]"},
		Type:   script.QueueNormal,
	})
	p.decodedThisTick = true
	p.walkDir = 7
	p.runDir = 7
	p.moveClickRequest = false // sentinel — will be flipped by processPostDecode

	// Drive the post-decode block.
	p.processPostDecode()

	if !p.moveClickRequest {
		t.Fatalf("moveClickRequest after processPostDecode: got false, want true (Busy + !opcalled + userPath set)")
	}

	// Drive movement; gate should fire and suppress.
	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (gate fires → walkDir cleared)", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (gate fires → runDir cleared)", p.runDir)
	}
	if p.x != 3200 {
		t.Errorf("p.x: got %d, want 3200 (gate fires → no step taken)", p.x)
	}
}
```

- [ ] **Step 5.2: Run test; verify pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessPostDecodeActivatesGate' -v
```

Expected: PASS. (Note: the test passes even before T6 — T6 is doc-only.)

If it fails, investigate: most likely the `Busy()` truth-table path or a missing prerequisite for `resolveMovement`. Check `TestResolveMovementGateOnPrimaryQueue` for the canonical setup pattern.

- [ ] **Step 5.3: Run race detector**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/
```

Expected: clean.

- [ ] **Step 5.4: Commit**

```
git add modules/world/player_movement_gate_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(player): NAI-146 T5 — end-to-end gate-activation pin

Closes NAI-144-D-MoveClickRequestSetter: drives processPostDecode →
resolveMovement and asserts the gate fires (walkDir/runDir cleared,
no step taken) when Busy() + queue-non-empty + userPath set.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Tracker doc-comment retirement

Replace the "INERT AT HEAD" tracker block in `movement.go` with a closed-tracker reference. This is doc-only; no tests.

**Files:**
- Modify: `modules/world/movement.go` (lines 53-59 — the `INERT AT HEAD` paragraph)

- [ ] **Step 6.1: Update the doc-comment**

In `modules/world/movement.go`, find the existing block (around lines 48-68):

```go
	// NAI-144: TS Player.ts:657 movement gate. When the player has an
	// outstanding move-click request AND is busy (modal/delayed) AND has
	// unfinished primary-queue OR engineQueue work, suppress movement
	// for this tick.
	//
	// INERT AT HEAD: goscape currently has zero `moveClickRequest = true`
	// assignment sites (verified at HEAD pre-NAI-144). TS sets it in
	// World.ts:611-628 (per-tick post-decode pathfinding pass); goscape's
	// structural equivalent lives in moveClickInner (handlers_game.go),
	// which runs at decode-time, not per-tick. The gate is wired
	// TS-faithful and ready to fire as soon as a setter port lands —
	// see tracker NAI-144-D-MoveClickRequestSetter.
	//
	// Gate body explicitly clears walkDir/runDir to avoid stale prior-tick
	// values bleeding into the current tick's outbound info block (the
	// existing "no waypoints" branch below sets the same pattern).
	if p.moveClickRequest && p.Busy() && (len(p.queue) > 0 || len(p.engineQueue) > 0) {
```

Replace with:

```go
	// NAI-144: TS Player.ts:657 movement gate. When the player has an
	// outstanding move-click request AND is busy (modal/delayed) AND has
	// unfinished primary-queue OR engineQueue work, suppress movement
	// for this tick.
	//
	// Setter source: NAI-146 (*Player).processPostDecode (TS World.ts:
	// 611-641 port). Closes the previously-tracked
	// NAI-144-D-MoveClickRequestSetter; gate is now live in production.
	//
	// Gate body explicitly clears walkDir/runDir to avoid stale prior-tick
	// values bleeding into the current tick's outbound info block (the
	// existing "no waypoints" branch below sets the same pattern).
	if p.moveClickRequest && p.Busy() && (len(p.queue) > 0 || len(p.engineQueue) > 0) {
```

- [ ] **Step 6.2: Verify no dangling tracker references**

```
grep -rE "MoveClickRequestSetter|WALKTRIGGER-FALLBACK-PHASE-CHOICE" modules/ pkg/ cmd/
```

Expected: zero hits in production code (`docs/` and memory may still reference the closed trackers; that's fine — they're historical).

- [ ] **Step 6.3: Run full package tests + race detector**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/
```

Expected: all green; race-clean.

- [ ] **Step 6.4: Commit**

```
git add modules/world/movement.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(player): NAI-146 T6 — close MoveClickRequestSetter tracker

The "INERT AT HEAD" paragraph at movement.go:53-59 referenced the
NAI-144-D-MoveClickRequestSetter follow-up. The setter landed in
NAI-146 T3d via processPostDecode (TS World.ts:611-641 port).
Replace the tracker reference with a setter-source pointer.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final verification (post-all-tasks)

- [ ] **Run package tests + race detector**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/
```

Expected: all green; race-clean.

- [ ] **Run repo-wide tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all green.

- [ ] **Confirm grep audit**

```
grep -rnE "moveClickRequest\s*=" modules/ | grep -v _test.go
```

Expected hits:
- `modules/world/handler_opheld.go` — 3 sites (existing `=false` rejection paths)
- `modules/world/player_post_decode.go` — 2 sites (`=false` and `=true` setter, NAI-146 T3d)

Zero unintended new sites.

```
grep -rnE "decodedThisTick" modules/
```

Expected hits:
- `modules/world/player.go` — field decl + 2 assignments in processIn (T1)
- `modules/world/player_post_decode.go` — 1 read in outer gate (T3a)
- `modules/world/player_post_decode_test.go` — fixture + branch tests (T1, T3, T5)

```
grep -rnE "processWalkTriggerFallback|walk_trigger_fallback" modules/ pkg/ cmd/ docs/
```

Expected: only `docs/` historical references; zero production hits.

- [ ] **Author close commit**

After T6 lands, draft a `chore(close): NAI-146 — …` commit summarizing the bundle (per `close_commit_memory_trailer.md` convention with `Closes memory:` trailer for any new memory entries).

---

## Risk notes (for plan executor)

1. **R3 spec audit (decode-time `sendUnsetMapFlag` callers)** — out of scope for THIS bundle. The NAI-146 T2 helper is added but no decode-time call site is migrated. If during T2 implementation you spot a call site that obviously needs the bundle (e.g., a test fails because waypoints aren't cleared), STOP and surface to the user — do NOT in-scope-stretch without approval.

2. **T3e signal subtlety** — the pathToTarget branch tests use indirect signals (walktrigger/waypointIndex side-effects) because `(p *Player) pathToTarget()` is hard to spy on directly. Each test's assertion is documented to explain why it isolates the branch. If a test fails ambiguously, re-read the comment before debugging.

3. **T4 ordering** — T4 is downstream of T3. Do NOT delete `walk_trigger_fallback.go` before T3f's tests are green — the redundant phase running in parallel is harmless during T3.

4. **`Busy()` semantics** — per `interaction.go:646-648` Busy() = `delayed || (modalState & (modalStateMain|modalStateChat))`. The SIDE modal bit is intentionally excluded. T3d / T5 set `delayed=true` or `modalState=modalStateMain` to drive Busy()=true.

5. **Per memory `verify_implementer_claims.md`**: at every "tests PASS" assertion in this plan, the implementer should run the cited command in a fresh shell and verify the output before claiming success. Stale IDE diagnostics or package-scoped green can mask failures.

6. **Per memory `controller_preflight.md`**: before dispatching each task, the controller should grep+Read the cited line numbers to verify the plan's premises haven't drifted at HEAD.
