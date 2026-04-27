# NAI-39 — HINT_PL + HINT_COORD + HINT_STOP + activePlayer2 substrate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close `NAI-37-D-HINTARROW-PARTIAL-ENCODER` by porting the three remaining HintArrow opcode handlers (HINT_COORD/HINT_PL/HINT_STOP) and the four remaining `HintArrowEncoder.ts` branches (type=2..6 / 10 / -1), with activePlayer2 substrate (Self2 field + requireActivePlayer2 helper + buildPlayerScriptState target dispatch) on the side.

**Architecture:** Bottom-up by layer — script-state substrate first (Task 1), then `(*Player)` wire-emit methods + interface extension (Task 2), then `runScript` refactor with target dispatch (Task 3), then three handlers (Task 4), then deviation retirement (Task 5). Mirrors NAI-11's `buildNpcScriptState` shape for the player side. No production producer for `Self2` lands in this sub-spec — the new `case script.ActivePlayer:` rails are exercised only by tests, tracked under new deviation `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER`.

**Tech Stack:** Go 1.26+ (per `go_version.md`; use `use-modern-go` skill). TS source: `Engine-TS` only per `ts_source_canonical_path.md`. HEAD baseline: `dc12465` (NAI-39 spec commit).

---

## Spec reference

Spec at `docs/superpowers/specs/2026-04-27-nai-39-hint-pl-coord-stop-design.md`. Test buckets A–E map to tasks as:
- **A** (9 handler tests) → Task 4
- **B** (3 requireActivePlayer2 helper tests) → Task 1
- **C** (4 byte-pin tests) → Task 2
- **D** (5 buildPlayerScriptState dispatch tests) → Task 3
- **E** (mockPlayer extensions) → Task 2 substrate

## File structure

| File | Status | Responsibility |
|------|--------|----------------|
| `pkg/script/state.go` | modify | +1 field `Self2 ActivePlayer` (Task 1) |
| `pkg/script/handlers_player.go` | modify | +`requireActivePlayer2` helper (Task 1); +3 handlers (Task 4); narrative refresh (Task 5) |
| `pkg/script/handlers_player_test.go` | modify | +3 helper tests (Task 1); +9 handler tests (Task 4) |
| `pkg/script/active.go` | modify | +`Slot()`, +`HintPlayer`, +`HintCoord`, +`HintStop` to `ActivePlayer` interface (Task 2) |
| `pkg/script/runner_test.go` | modify | mockPlayer fields + 4 method impls (Task 2) |
| `pkg/script/handlers.go` | modify | +3 dispatch entries (Task 4) |
| `modules/world/player_script.go` | modify | +3 `(*Player)` wire-emit methods (Task 2); narrative refresh (Task 5) |
| `modules/world/player_script_test.go` | modify | +4 byte-pin tests (Task 2) |
| `modules/world/script.go` | modify | extract `buildPlayerScriptState`; thread `target any` through `runScript` (Task 3) |
| `modules/world/script_test.go` | modify | +5 dispatch tests (Task 3); thread `nil` through 16 callers (Task 3) |
| `modules/world/tick.go` | modify | thread `nil` through 3 callers (Task 3) |
| `pkg/io/protocol/game/server/prot.go` | modify | narrative refresh (Task 5) |

## Pre-flight checks (controller)

Per `controller_preflight.md`: re-grep each premise against HEAD before dispatching each task. Specifically:

| Task | Pre-dispatch verification |
|------|--------------------------|
| 1 | `rg -n "PtrActivePlayer2" pkg/script/` returns 1 hit (pointer.go:9 declaration). `rg -n "type ScriptState struct" pkg/script/state.go` returns line 136. |
| 2 | `(*Player).HintNpc` exists at `modules/world/player_script.go:158`. `(*Player).Slot` exists at `modules/world/player.go:434`. `mockPlayer.hintNpcCalls` exists at `pkg/script/runner_test.go:206`. |
| 3 | `runScript` exists at `modules/world/script.go:27` with signature `(sf, self, protect, intArgs, stringArgs)`. `buildNpcScriptState` exists at `modules/world/npc_script.go:225`. `s.runScript(` returns 19 hits across `tick.go` (3 sites: 135, 246, 291) and `script_test.go` (16 sites: 46, 66, 97, 151, 175, 200, 312, 399, 427, 455, 493, 538, 650, 682, 722, 771). |
| 4 | `OpHintCoord/OpHintPl/OpHintStop` declared at `opcode.go:127-130`. Dispatch table is at `pkg/script/handlers.go:end`. `requireActivePlayer` at `handlers_player.go:35`. `checkCoord` at `handlers_npc.go:13`. |
| 5 | `rg -n "NAI-37-D-HINTARROW-PARTIAL-ENCODER" pkg/ modules/` returns exactly 3 hits: `pkg/script/handlers_player.go:842`, `modules/world/player_script.go:154`, `pkg/io/protocol/game/server/prot.go:44`. |

---

### Task 1: `Self2` field + `requireActivePlayer2` helper

**Goal:** Add the script-state substrate that activates `PtrActivePlayer2` (declared since NAI-S1 with zero consumers).

**Files:**
- Modify: `pkg/script/state.go` (insert one field below `Target` at line 188)
- Modify: `pkg/script/handlers_player.go` (insert `requireActivePlayer2` helper directly below `requireActivePlayer` at line 40)
- Modify: `pkg/script/handlers_player_test.go` (append 3 unit tests at end of file)

- [ ] **Step 1.1: Write the 3 failing helper tests**

Append to `pkg/script/handlers_player_test.go`:

```go
// --- NAI-39 Task 1: requireActivePlayer2 unit tests ----------------------

// TestRequireActivePlayer2_NoBit_Errors pins the pointer-bit check:
// Self2 is set but PtrActivePlayer2 is unset → error. Without this direct
// helper test, a bug that drops the bit-mask check could pass the
// handler-level "Self2 set" path silently (per test_passes_for_wrong_reason.md).
func TestRequireActivePlayer2_NoBit_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self2:       &mockPlayer{},
		Pointers:    PtrActivePlayer, // PtrActivePlayer2 NOT set
	}
	if err := requireActivePlayer2(s, "TEST"); err == nil {
		t.Fatal("expected error when PtrActivePlayer2 unset")
	}
}

// TestRequireActivePlayer2_NilSelf2_Errors pins the nil-receiver check:
// PtrActivePlayer2 is set but Self2 is nil → error. Defends against the
// flag/state mismatch case that buildPlayerScriptState's atomic seeding
// is supposed to prevent.
func TestRequireActivePlayer2_NilSelf2_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
		// Self2 nil
	}
	if err := requireActivePlayer2(s, "TEST"); err == nil {
		t.Fatal("expected error when Self2 nil")
	}
}

// TestRequireActivePlayer2_Both_OK pins the both-present happy path.
func TestRequireActivePlayer2_Both_OK(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self2:       &mockPlayer{},
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
	}
	if err := requireActivePlayer2(s, "TEST"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 1.2: Run tests and verify they fail with compile errors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestRequireActivePlayer2" ./pkg/script/`

Expected: FAIL — `undefined: requireActivePlayer2` and `unknown field Self2 in struct literal of type ScriptState`.

- [ ] **Step 1.3: Add `Self2` field to `ScriptState`**

In `pkg/script/state.go`, locate the existing `Self/Target` block (around line 187-188):

```go
Pointers Pointer
Self     ActivePlayer
Target   ActivePlayer
```

Insert directly below `Target`:

```go
// Self2 is the secondary active-player slot consumed by HINT_PL and
// (future) BOTH_HEROPOINTS / OPPLAYER triggers. Mirrors TS
// _activePlayer2 (ScriptState.ts:223-241). Producer wiring lives in
// buildPlayerScriptState's target type-switch (when self is also Player).
//
// DEVIATION NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: no production
// trigger seeds Self2 yet; the rails are exercised only by tests.
// Closure when OPPLAYER triggers are ported.
Self2 ActivePlayer
```

- [ ] **Step 1.4: Add `requireActivePlayer2` helper**

In `pkg/script/handlers_player.go`, directly below `requireActivePlayer` (which ends at line 40):

```go
// requireActivePlayer2 is the dual-pin validator for the secondary
// active-player slot (Self2). Every handler that dereferences s.Self2
// calls this first. NAI-39.
func requireActivePlayer2(s *ScriptState, op string) error {
	if s.Pointers&PtrActivePlayer2 == 0 || s.Self2 == nil {
		return errors.New(op + ": no active player2")
	}
	return nil
}
```

- [ ] **Step 1.5: Run tests and verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestRequireActivePlayer2" ./pkg/script/`

Expected: PASS — all 3 tests green.

- [ ] **Step 1.6: Run full pkg/script test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/...`

Expected: all pre-existing tests pass; no compile errors elsewhere in the package.

- [ ] **Step 1.7: Commit**

```bash
git add pkg/script/state.go pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-39 T1 — Self2 field + requireActivePlayer2 helper

Adds the ScriptState.Self2 field and requireActivePlayer2 validator.
PtrActivePlayer2 was declared at pointer.go:9 since NAI-S1 with zero
consumers; T1 lights up the slot. No producer wiring yet (lands in T3).

3 direct helper unit tests (B-bucket per spec test strategy):
- TestRequireActivePlayer2_NoBit_Errors
- TestRequireActivePlayer2_NilSelf2_Errors
- TestRequireActivePlayer2_Both_OK

Pinning both branches of the OR-condition independently (per
test_passes_for_wrong_reason.md memory).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `(*Player)` wire-emit methods + `ActivePlayer` interface extension + 4 byte-pin tests

**Goal:** Port the three remaining `HintArrowEncoder.ts` branches (type=2..6 TILE, type=10 PL, type=-1 STOP) as `(*Player)` methods, with byte-pin tests against the wire output.

**Files:**
- Modify: `pkg/script/active.go` (extend `ActivePlayer` interface with 4 new methods)
- Modify: `pkg/script/runner_test.go` (extend `mockPlayer` with capture fields + 4 method impls)
- Modify: `modules/world/player_script.go` (3 new `(*Player)` methods)
- Modify: `modules/world/player_script_test.go` (4 new byte-pin tests)

- [ ] **Step 2.1: Write the 4 failing byte-pin tests**

Append to `modules/world/player_script_test.go` (after the existing `TestHintNpcPayloadBytes` at line 819):

```go
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
				0x00, 0x02,   // p2: z=2
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
```

If the file does not already import `"fmt"`, add it. (Check with `rg -n '^import|"fmt"' modules/world/player_script_test.go`.)

- [ ] **Step 2.2: Run tests and verify they fail with compile errors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestHintCoord|TestHintPlayer|TestHintStop" ./modules/world/`

Expected: FAIL — `p.HintCoord undefined`, `p.HintPlayer undefined`, `p.HintStop undefined`.

- [ ] **Step 2.3: Add the 3 `(*Player)` wire-emit methods**

In `modules/world/player_script.go`, append directly below the existing `(*Player).HintNpc` (which ends at line 166):

```go
// HintCoord sends a HINT_ARROW (type=2..6, TILE variant) wire packet to
// the client. Encodes 6 bytes matching TS HintArrowEncoder type=2..6
// branch (HintArrowEncoder.ts:17-27): p1(offset), p2(x), p2(z),
// p1(height). Called by the HINT_COORD (opcode 2027) script handler.
// Mirrors TS Player.hintTile at Player.ts:2178-2180.
//
// Out-of-range offset (not in [2,6]) is TS-faithful: the wire packet
// is emitted with the offset as byte[0]. Script-authors are responsible
// for offset bounds; the entity-method does not validate.
func (p *Player) HintCoord(offset, x, z, height int) {
	payload := []byte{
		byte(offset),                 // p1: type = offset (2..6)
		byte(x >> 8), byte(x),        // p2: x (big-endian)
		byte(z >> 8), byte(z),        // p2: z (big-endian)
		byte(height),                 // p1: height
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintPlayer sends a HINT_ARROW (type=10, PL variant) wire packet to
// the client. Encodes 6 bytes matching TS HintArrowEncoder type=10
// branch (HintArrowEncoder.ts:28-32): p1(10), p2(slot), p2(0), p1(0).
// Called by the HINT_PL (opcode 2029) script handler. Mirrors TS
// Player.hintPlayer at Player.ts:2182-2184.
func (p *Player) HintPlayer(slot int) {
	payload := []byte{
		0x0A,                            // p1: type = 10 (player hint)
		byte(slot >> 8), byte(slot),     // p2: slot (big-endian)
		0x00, 0x00,                      // p2: 0
		0x00,                            // p1: 0
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintStop sends a HINT_ARROW (type=-1, STOP variant) wire packet to
// the client, clearing any active hint arrow. Encodes 6 bytes matching
// TS HintArrowEncoder type=-1 branch (HintArrowEncoder.ts:33-38):
// p1(-1), p2(0), p2(0), p1(0). p1(-1) on the wire is 0xFF (low byte of
// two's-complement). Called by the HINT_STOP (opcode 2030) script
// handler. Mirrors TS Player.stopHint at Player.ts:2186-2188.
func (p *Player) HintStop() {
	payload := []byte{
		0xFF,        // p1: type = -1 (stop sentinel; two's-complement low byte)
		0x00, 0x00,  // p2: 0
		0x00, 0x00,  // p2: 0
		0x00,        // p1: 0
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}
```

- [ ] **Step 2.4: Extend `ActivePlayer` interface**

In `pkg/script/active.go`, locate the existing `HintNpc(nid int)` declaration (within the `ActivePlayer` interface, search for `HintNpc(`). Directly below that declaration, insert:

```go
// HintCoord directs the client to render a hint arrow at the (x, z) tile
// with the given offset (2..6, sub-tile arrow position) and height.
// Mirrors TS Player.hintTile at Player.ts:2178-2180; called by the
// HINT_COORD (opcode 2027) handler. NAI-39.
HintCoord(offset, x, z, height int)

// HintPlayer directs the client to render a hint arrow pointing at the
// player in slot `slot`. Mirrors TS Player.hintPlayer at
// Player.ts:2182-2184; called by the HINT_PL (opcode 2029) handler.
// NAI-39.
HintPlayer(slot int)

// HintStop directs the client to clear any active hint arrow. Mirrors
// TS Player.stopHint at Player.ts:2186-2188; called by the HINT_STOP
// (opcode 2030) handler. NAI-39.
HintStop()

// Slot returns the player's authoritative slot id (the index into the
// world's player array). Mirrors TS Player.slot. Consumed by HINT_PL,
// which reads activePlayer2.slot. NAI-39.
Slot() int
```

- [ ] **Step 2.5: Extend `mockPlayer` with capture fields and method impls**

In `pkg/script/runner_test.go`, locate the existing `hintNpcCalls []int` field (line ~206). Insert directly below it:

```go
// NAI-39: HintCoord / HintPlayer / HintStop captures (mirrors hintNpcCalls
// shape). hintCoordCalls captures all 4 args via a struct slice;
// hintPlayerCalls captures the slot int; hintStopCalls counts invocations.
hintCoordCalls  []mockHintCoord
hintPlayerCalls []int
hintStopCalls   int

// slot is the value returned by mockPlayer.Slot(); tests pre-seed it.
slot int
```

Also add the helper struct above `mockPlayer` (search for `type mockPlayer struct` at line 95 and place the new struct above it):

```go
// mockHintCoord captures the 4 args of a single HintCoord call for
// handler-test inspection. NAI-39.
type mockHintCoord struct{ offset, x, z, height int }
```

In the same file, locate the existing `HintNpc` capture method (line ~457):

```go
func (m *mockPlayer) HintNpc(nid int) { m.hintNpcCalls = append(m.hintNpcCalls, nid) }
```

Append directly below:

```go
// NAI-39: HintCoord / HintPlayer / HintStop / Slot capture impls.
func (m *mockPlayer) HintCoord(offset, x, z, height int) {
	m.hintCoordCalls = append(m.hintCoordCalls, mockHintCoord{offset, x, z, height})
}
func (m *mockPlayer) HintPlayer(s int) { m.hintPlayerCalls = append(m.hintPlayerCalls, s) }
func (m *mockPlayer) HintStop()        { m.hintStopCalls++ }
func (m *mockPlayer) Slot() int        { return m.slot }
```

- [ ] **Step 2.6: Run byte-pin tests and verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestHintCoord|TestHintPlayer|TestHintStop" ./modules/world/`

Expected: PASS — all 4 (well, 5 with the table-driven offset-boundaries) tests green.

- [ ] **Step 2.7: Run full module test suites to confirm no compile errors elsewhere**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: all tests pass. The `ActivePlayer` interface extension forces both `*Player` and `mockPlayer` to satisfy the new methods; both have impls now, so compilation succeeds.

- [ ] **Step 2.8: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "feat(world,script): NAI-39 T2 — (*Player).HintCoord/HintPlayer/HintStop + iface

Extends ActivePlayer with 4 methods (Slot + 3 hint variants) and ports
the 3 remaining HintArrowEncoder.ts branches (type=2..6, 10, -1) as
(*Player) wire-emit methods. mockPlayer extended with capture fields
mirroring the existing hintNpcCalls shape.

4 byte-pin tests (C-bucket per spec) pin every byte of every encoder
branch (per rsbuf_roundtrip_tests.md). The 0xFF in TestHintStopPayloadBytes
is the conspicuous-pin for the type=-1 → 0xFF asymmetry.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: `buildPlayerScriptState` extraction + `runScript` signature change + 19-callsite threading

**Goal:** Mirror NAI-11's `buildNpcScriptState` shape on the player side. Extract the inline state-init body of `runScript` into a parallel helper with a `target any` parameter and a 5-branch type-switch (nil/Player/Npc/Loc/Obj).

**Files:**
- Modify: `modules/world/script.go` (extract `buildPlayerScriptState`; change `runScript` signature)
- Modify: `modules/world/script_test.go` (5 new dispatch tests; thread `nil` through 16 existing callers)
- Modify: `modules/world/tick.go` (thread `nil` through 3 callers)

- [ ] **Step 3.1: Write the 5 failing dispatch tests**

Append to `modules/world/script_test.go` (preferably grouped at end of file):

```go
// --- NAI-39 Task 3: buildPlayerScriptState target-dispatch tests ----------
//
// Direct mirror of buildNpcScriptState target-dispatch coverage at
// npc_script_test.go:472-560. Verifies the rails work even though no
// production producer fires through them yet — closes the dual-pin
// (presence-of-rails) per ts_asymmetry_dual_pin.md.

// TestBuildPlayerScriptState_NilTarget — nil target leaves Self2 nil and
// PtrActivePlayer2 unset; only the primary PtrActivePlayer is set.
func TestBuildPlayerScriptState_NilTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, nil, false, nil, nil)

	if state.Self2 != nil {
		t.Error("Self2: non-nil, want nil")
	}
	if state.Pointers&script.PtrActivePlayer2 != 0 {
		t.Error("Pointers: PtrActivePlayer2 flag set, want unset")
	}
	if state.Pointers&script.PtrActivePlayer == 0 {
		t.Error("Pointers: PtrActivePlayer flag unset, want set (primary)")
	}
}

// TestBuildPlayerScriptState_PlayerTarget — *Player target lands in
// state.Self2 with PtrActivePlayer2 set; Self (primary) is unchanged.
// Mirrors TS ScriptRunner.init: self=Player, target=Player → _activePlayer2.
func TestBuildPlayerScriptState_PlayerTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p2, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, p2, false, nil, nil)

	if state.Self == nil {
		t.Error("Self: nil, want primary set")
	}
	if state.Self2 == nil {
		t.Error("Self2: nil, want set (ActivePlayer target)")
	}
	if state.Pointers&script.PtrActivePlayer2 == 0 {
		t.Error("Pointers: PtrActivePlayer2 flag unset, want set")
	}
	if state.Self2 != p2 {
		t.Errorf("Self2: got %v, want %v (target Player)", state.Self2, p2)
	}
}

// TestBuildPlayerScriptState_NpcTarget — *Npc target lands in
// state.ActiveNpc with PtrActiveNpc set.
func TestBuildPlayerScriptState_NpcTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	npc := newNpcForScriptTest(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, npc, false, nil, nil)

	if state.ActiveNpc == nil {
		t.Error("ActiveNpc: nil, want set")
	}
	if state.Pointers&script.PtrActiveNpc == 0 {
		t.Error("Pointers: PtrActiveNpc flag unset, want set")
	}
}

// TestBuildPlayerScriptState_LocTarget — *Loc target lands in
// state.ActiveLoc with PtrActiveLoc set.
func TestBuildPlayerScriptState_LocTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleRespawn, 42, 10, 0)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, loc, false, nil, nil)

	if state.ActiveLoc == nil {
		t.Error("ActiveLoc: nil, want set")
	}
	if state.Pointers&script.PtrActiveLoc == 0 {
		t.Error("Pointers: PtrActiveLoc flag unset, want set")
	}
}

// TestBuildPlayerScriptState_ObjTarget — *Obj target lands in
// state.ActiveObj with PtrActiveObj set.
func TestBuildPlayerScriptState_ObjTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleRespawn, 42, 1)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, obj, false, nil, nil)

	if state.ActiveObj == nil {
		t.Error("ActiveObj: nil, want set")
	}
	if state.Pointers&script.PtrActiveObj == 0 {
		t.Error("Pointers: PtrActiveObj flag unset, want set")
	}
}
```

If the file does not import `entitypkg "github.com/zsrv/goscape/pkg/entity"`, add the import. (Check via `rg -n '"github.com/zsrv/goscape/pkg/entity"' modules/world/script_test.go`.)

- [ ] **Step 3.2: Run tests and verify they fail with compile errors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestBuildPlayerScriptState" ./modules/world/`

Expected: FAIL — `s.buildPlayerScriptState undefined`, plus the existing 16 `s.runScript(sf, p, true, nil, nil)` test calls become wrong-arity once the signature changes. Per `enumerate_all_sites.md`, all 19 callers must change in lockstep with the signature.

- [ ] **Step 3.3: Refactor `runScript` to extract `buildPlayerScriptState` and add `target` parameter**

Replace the existing `runScript` function body (`modules/world/script.go:27-40`) with:

```go
// buildPlayerScriptState initialises a ScriptState for a player-anchored
// fresh run. Pure — no side effects on server state — so callers can
// test the target-dispatch logic in isolation.
//
// NAI-39: target may be nil (the common case — no secondary entity), or
// a concrete value satisfying one of the Active* interfaces. The
// type-switch wires the matching ScriptState field and pointer flag,
// mirroring buildNpcScriptState's NAI-11 shape (npc_script.go:225-261)
// and the TS ScriptRunner.init target-dispatch at ScriptRunner.ts:84-116.
//
// DEVIATION NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: the
// case script.ActivePlayer branch lays the rails for OPPLAYER triggers
// (Player→Player invocations). No production trigger seeds Self2 yet;
// closure when OPPLAYER triggers are ported.
func (s *Server) buildPlayerScriptState(
	sf *script.ScriptFile,
	self script.ActivePlayer,
	target any,
	protect bool,
	intArgs []int,
	stringArgs []string,
) *script.ScriptState {
	state := script.Init(sf, self, protect, intArgs, stringArgs)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.PlayerLookup = s
	state.LineValidator = s.scriptLineValidator()

	switch t := target.(type) {
	case nil:
		// No secondary pointer.
	case script.ActivePlayer:
		// TS: self=Player, target=Player → _activePlayer2 = target,
		// PtrActivePlayer2 (ScriptRunner.ts:84-87).
		state.Self2 = t
		state.Pointers |= script.PtrActivePlayer2
	case script.ActiveNpc:
		state.ActiveNpc = t
		state.Pointers |= script.PtrActiveNpc
	case script.ActiveLoc:
		state.ActiveLoc = t
		state.Pointers |= script.PtrActiveLoc
	case script.ActiveObj:
		state.ActiveObj = t
		state.Pointers |= script.PtrActiveObj
	}

	return state
}

// runScript initialises a ScriptState for a fresh invocation and routes
// the result via resumeOrFinish. Safe to call with a nil scriptFile
// (no-op) so callers don't have to nil-check the trigger lookup.
//
// If the script suspends (Execution == Suspended), the state is stored
// on the active player and the tick loop will resume it when the
// player's delay expires via processActiveScripts.
//
// NAI-39: target is the secondary-entity binding for triggers that
// dispatch through an active_player2 / active_npc / active_loc /
// active_obj slot. Pass nil when there is no secondary binding (the
// common case — engine-dispatched timers, queue runs, login).
func (s *Server) runScript(
	sf *script.ScriptFile,
	self script.ActivePlayer,
	target any,
	protect bool,
	intArgs []int,
	stringArgs []string,
) {
	if sf == nil {
		return
	}
	state := s.buildPlayerScriptState(sf, self, target, protect, intArgs, stringArgs)
	s.resumeOrFinish(state, self)
}
```

- [ ] **Step 3.4: Thread `nil` through all 19 `s.runScript` callers**

Per `plan_doc_replaceall_timeline.md`: per-instance Edits with surrounding context. Don't use `replace_all` — the literal `s.runScript(sf, p, true, nil, nil)` is not a unique anchor across the 16 test sites.

Production callers (3, all in `modules/world/tick.go`):

| Line | Old | New |
|------|-----|-----|
| 135 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 246 | `s.runScript(sf, p, false, intArgs, stringArgs)` | `s.runScript(sf, p, nil, false, intArgs, stringArgs)` |
| 291 | `s.runScript(sf, p, false, t.IntArgs, t.StringArgs)` | `s.runScript(sf, p, nil, false, t.IntArgs, t.StringArgs)` |

Test callers (16, all in `modules/world/script_test.go`):

| Line | Old | New |
|------|-----|-----|
| 46 | `s.runScript(nil, p, true, nil, nil)` | `s.runScript(nil, p, nil, true, nil, nil)` |
| 66 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 97 | `s.runScript(bad, p, true, nil, nil)` | `s.runScript(bad, p, nil, true, nil, nil)` |
| 151 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 175 | `s.runScript(buildDelayScript(), p, true, nil, nil)` | `s.runScript(buildDelayScript(), p, nil, true, nil, nil)` |
| 200 | `s.runScript(buildDelayScript(), p, true, nil, nil)` | `s.runScript(buildDelayScript(), p, nil, true, nil, nil)` |
| 312 | `s.runScript(sf, p, false, nil, nil)` | `s.runScript(sf, p, nil, false, nil, nil)` |
| 399 | `s.runScript(popVarpScript(42), p, false, nil, nil)` | `s.runScript(popVarpScript(42), p, nil, false, nil, nil)` |
| 427 | `s.runScript(popVarpScript(10000), p, false, nil, nil)` | `s.runScript(popVarpScript(10000), p, nil, false, nil, nil)` |
| 455 | `s.runScript(popVarpScript(42), p, false, nil, nil)` | `s.runScript(popVarpScript(42), p, nil, false, nil, nil)` |
| 493 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 538 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 650 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 682 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 722 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |
| 771 | `s.runScript(sf, p, true, nil, nil)` | `s.runScript(sf, p, nil, true, nil, nil)` |

**Validation** — after threading: `rg -n "s\.runScript\(" modules/world/ | wc -l` returns exactly 19; `rg -n "s\.runScript\(.+, true, nil, nil\)" modules/world/ | wc -l` returns 0 (every call has been threaded).

- [ ] **Step 3.5: Run tests and verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestBuildPlayerScriptState" ./modules/world/`

Expected: PASS — all 5 dispatch tests green.

- [ ] **Step 3.6: Run full module test suite to confirm threading didn't break existing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`

Expected: all pre-existing tests pass (the threaded `nil` is a no-op for the type-switch). If any test fails, re-grep for missed callers and verify all 19 lines were threaded.

- [ ] **Step 3.7: Commit**

```bash
git add modules/world/script.go modules/world/script_test.go modules/world/tick.go
git commit --no-gpg-sign -m "feat(world): NAI-39 T3 — buildPlayerScriptState + runScript target dispatch

Mirrors NAI-11's buildNpcScriptState shape on the player side. Extracts
the inline state-init body of runScript into a pure helper with a
target any parameter and a 5-branch type-switch (nil/Player/Npc/Loc/Obj).
runScript becomes a thin wrapper.

Threads target=nil through all 19 callers (3 production: tick.go:135/246/291;
16 test: script_test.go).

5 dispatch tests (D-bucket per spec) directly mirror the buildNpcScriptState
test pattern. The PlayerTarget test pins TS-faithful seeding semantics
(self=Player + target=Player → Self2, NOT Self overwrite).

Opens deviation NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: the rails
have no production producer until OPPLAYER triggers are ported.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Three handlers (`handleHintCoord`, `handleHintPl`, `handleHintStop`) + dispatcher registration

**Goal:** Wire the three new handlers into the dispatch table. Each handler validates pointers, pops args (where applicable), calls the entity-method.

**Files:**
- Modify: `pkg/script/handlers_player.go` (3 handler functions appended after `handleHintNpc`)
- Modify: `pkg/script/handlers.go` (3 dispatch entries)
- Modify: `pkg/script/handlers_player_test.go` (9 handler unit tests)

- [ ] **Step 4.1: Write the 9 failing handler unit tests**

Append to `pkg/script/handlers_player_test.go` (after the Task 1 helper tests):

```go
// --- NAI-39 Task 4: HINT_COORD / HINT_PL / HINT_STOP handler unit tests ---

func TestHintCoord_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	if err := handleHintCoord(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

func TestHintCoord_InvalidCoord_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// Push offset=3, coord=-1 (invalid), height=0. Pop order is height,
	// coord, offset — so push offset FIRST.
	s.PushInt(3)
	s.PushInt(-1)
	s.PushInt(0)
	if err := handleHintCoord(s); err == nil {
		t.Fatal("expected error for invalid coord")
	}
	if len(pl.hintCoordCalls) != 0 {
		t.Errorf("hintCoordCalls: got %d, want 0 on validation failure", len(pl.hintCoordCalls))
	}
}

func TestHintCoord_Success_RecordsArgs(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// coord = pack(level=0, x=100, z=200) = (0<<28)|(100<<14)|200
	coord := (100 << 14) | 200
	s.PushInt(3)      // offset
	s.PushInt(coord)  // coord
	s.PushInt(42)     // height
	if err := handleHintCoord(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []mockHintCoord{{offset: 3, x: 100, z: 200, height: 42}}
	if !slices.Equal(pl.hintCoordCalls, want) {
		t.Errorf("hintCoordCalls: got %v, want %v", pl.hintCoordCalls, want)
	}
}

// TestHintCoord_PopOrderDistinctValues pins which popped value lands in
// which dispatch arg. Distinct values rule out symmetric off-by-one.
func TestHintCoord_PopOrderDistinctValues(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// coord = pack(0, 1, 2)
	coord := (1 << 14) | 2
	s.PushInt(2)      // offset (push first, popped last)
	s.PushInt(coord)
	s.PushInt(99)     // height (push last, popped first)
	if err := handleHintCoord(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []mockHintCoord{{offset: 2, x: 1, z: 2, height: 99}}
	if !slices.Equal(pl.hintCoordCalls, want) {
		t.Errorf("hintCoordCalls: got %v, want %v", pl.hintCoordCalls, want)
	}
}

func TestHintPl_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self, no Self2
	if err := handleHintPl(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

// TestHintPl_NoActivePlayer2_Errors pins the second guard: Self set +
// PtrActivePlayer set, but Self2 nil + PtrActivePlayer2 unset.
func TestHintPl_NoActivePlayer2_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer, // PtrActivePlayer2 NOT set
	}
	if err := handleHintPl(s); err == nil {
		t.Fatal("expected error for no active player2")
	}
	if len(pl.hintPlayerCalls) != 0 {
		t.Errorf("hintPlayerCalls: got %d, want 0 on validation failure", len(pl.hintPlayerCalls))
	}
}

func TestHintPl_Success_RecordsSlot(t *testing.T) {
	pl := &mockPlayer{}
	pl2 := &mockPlayer{slot: 7}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Self2:       pl2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
	}
	if err := handleHintPl(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int{7}; !slices.Equal(pl.hintPlayerCalls, want) {
		t.Errorf("hintPlayerCalls: got %v, want %v", pl.hintPlayerCalls, want)
	}
}

func TestHintStop_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	if err := handleHintStop(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

func TestHintStop_Success_IncrementsCounter(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	if err := handleHintStop(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pl.hintStopCalls != 1 {
		t.Errorf("hintStopCalls: got %d, want 1", pl.hintStopCalls)
	}
}
```

If the test file does not import `"slices"`, add it. (Check via `rg -n '"slices"' pkg/script/handlers_player_test.go`.)

- [ ] **Step 4.2: Run tests and verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestHint(Coord|Pl|Stop)_" ./pkg/script/`

Expected: FAIL — `undefined: handleHintCoord`, `undefined: handleHintPl`, `undefined: handleHintStop`.

- [ ] **Step 4.3: Add the three handler functions**

In `pkg/script/handlers_player.go`, append directly after `handleHintNpc` (which ends at line 854):

```go
// handleHintCoord (HINT_COORD, opcode 2027) sends a HintArrow type=2..6
// (TILE) wire packet to the active player at the unpacked coord. Pop
// order: [offset, coord, height] (per TS popInts(3) destructuring at
// PlayerOps.ts:867); goscape's PopInt order is height, coord, offset.
// Mirrors TS PlayerOps.ts:866-871. NAI-39.
func handleHintCoord(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_COORD"); err != nil {
		return err
	}
	height := s.PopInt()
	coord := s.PopInt()
	offset := s.PopInt()
	_, x, z, err := checkCoord(coord, "HINT_COORD")
	if err != nil {
		return err
	}
	s.Self.HintCoord(offset, x, z, height)
	return nil
}

// handleHintPl (HINT_PL, opcode 2029) sends a HintArrow type=10 (PL)
// wire packet to the active player, pointing at the secondary
// active_player2 by slot. Mirrors TS PlayerOps.ts:976-978:
//
//	state.activePlayer.hintPlayer(state.activePlayer2.slot)
//
// Requires both active_player and active_player2 to be bound. NAI-39.
func handleHintPl(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_PL"); err != nil {
		return err
	}
	if err := requireActivePlayer2(s, "HINT_PL"); err != nil {
		return err
	}
	s.Self.HintPlayer(s.Self2.Slot())
	return nil
}

// handleHintStop (HINT_STOP, opcode 2030) sends a HintArrow type=-1
// (STOP) wire packet to the active player, clearing any active hint.
// Mirrors TS PlayerOps.ts:873-875. NAI-39.
func handleHintStop(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_STOP"); err != nil {
		return err
	}
	s.Self.HintStop()
	return nil
}
```

- [ ] **Step 4.4: Register the three handlers in the dispatch table**

In `pkg/script/handlers.go`, locate the existing NAI-37 hint-arrow block (around line 403):

```go
// NAI-37 T6: hint-arrow — HINT_NPC (type=1) only.
OpHintNpc: handleHintNpc,
```

Refresh the comment and add the three new entries directly below:

```go
// NAI-37 T6 + NAI-39: hint-arrow — full HintArrowEncoder coverage.
//   - HINT_NPC   (type=1)     — NAI-37
//   - HINT_COORD (type=2..6)  — NAI-39
//   - HINT_PL    (type=10)    — NAI-39
//   - HINT_STOP  (type=-1)    — NAI-39
OpHintNpc:   handleHintNpc,
OpHintCoord: handleHintCoord,
OpHintPl:    handleHintPl,
OpHintStop:  handleHintStop,
```

- [ ] **Step 4.5: Run handler tests and verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run "TestHint(Coord|Pl|Stop)_" ./pkg/script/`

Expected: PASS — all 9 tests green.

- [ ] **Step 4.6: Run full pkg/script test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/...`

Expected: all tests pass (NAI-37 HintNpc tests still green; new handlers integrated into dispatch table).

- [ ] **Step 4.7: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-39 T4 — handleHintCoord/HintPl/HintStop handlers

Wires the three new handlers into the dispatch table:
- HINT_COORD (opcode 2027) — pops [offset, coord, height], unpacks coord,
  calls (*Player).HintCoord(offset, x, z, height). Mirrors PlayerOps.ts:866-871.
- HINT_PL    (opcode 2029) — reads Self2.Slot(), calls (*Player).HintPlayer(slot).
  Requires both PtrActivePlayer + PtrActivePlayer2. Mirrors PlayerOps.ts:976-978.
- HINT_STOP  (opcode 2030) — calls (*Player).HintStop(). Mirrors PlayerOps.ts:873-875.

9 handler unit tests (A-bucket per spec) pin every guard branch and
dispatch-arg position. TestHintCoord_PopOrderDistinctValues uses
asymmetric values (offset=2, x=1, z=2, height=99) to rule out
symmetric off-by-one in the pop-order chain.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Retire `NAI-37-D-HINTARROW-PARTIAL-ENCODER` deviation (3 code-tree sites)

**Goal:** Remove the deviation tag at every code-tree site (per `retire_deviation_grep_all_comments.md`) and refresh narratives to present-state.

**Files:**
- Modify: `pkg/script/handlers_player.go` — refresh `handleHintNpc` doc-block
- Modify: `modules/world/player_script.go` — refresh `(*Player).HintNpc` doc-block
- Modify: `pkg/io/protocol/game/server/prot.go` — refresh `OpHintArrow` doc-block

This task does not add any new tests. Verification is via `rg` returning zero hits.

- [ ] **Step 5.1: Refresh `handleHintNpc` doc-block in `pkg/script/handlers_player.go`**

Replace lines 836-844 (the existing doc-block):

```go
// handleHintNpc (HINT_NPC, opcode 2028) sends a HintArrow type=1 wire
// packet to the active player, pointing at the active NPC. Mirrors TS
// PlayerOps.ts:972-974:
//
//	state.activePlayer.hintNpc(state.activeNpc.nid)
//
// DEVIATION NAI-37-D-HINTARROW-PARTIAL-ENCODER: only the type=1 (NPC)
// hint variant is wired. HINT_PL, HINT_COORD, HINT_STOP handlers
// land in a future sub-spec.
```

With:

```go
// handleHintNpc (HINT_NPC, opcode 2028) sends a HintArrow type=1 wire
// packet to the active player, pointing at the active NPC. Mirrors TS
// PlayerOps.ts:972-974:
//
//	state.activePlayer.hintNpc(state.activeNpc.nid)
//
// Full HintArrowEncoder coverage: HINT_NPC (type=1, NAI-37 T6),
// HINT_COORD (type=2..6, NAI-39), HINT_PL (type=10, NAI-39),
// HINT_STOP (type=-1, NAI-39).
```

- [ ] **Step 5.2: Refresh `(*Player).HintNpc` doc-block in `modules/world/player_script.go`**

Replace lines 149-157 (the existing doc-block):

```go
// HintNpc sends a HINT_ARROW (type=1, NPC variant) wire packet to the
// client. Encodes 6 bytes matching TS HintArrowEncoder type=1 branch:
// p1(type=1), p2(nid), p2(0), p1(0). Called by the HINT_NPC (opcode
// 2028) script handler. Mirrors TS Player.hintNpc at Player.ts:2174-2176.
//
// DEVIATION NAI-37-D-HINTARROW-PARTIAL-ENCODER: only the type=1 branch
// of TS HintArrowEncoder is implemented. Closure when HINT_PL,
// HINT_COORD, HINT_STOP handlers and their respective encoder branches
// land.
```

With:

```go
// HintNpc sends a HINT_ARROW (type=1, NPC variant) wire packet to the
// client. Encodes 6 bytes matching TS HintArrowEncoder type=1 branch:
// p1(type=1), p2(nid), p2(0), p1(0). Called by the HINT_NPC (opcode
// 2028) script handler. Mirrors TS Player.hintNpc at Player.ts:2174-2176.
//
// Sibling encoder branches: (*Player).HintCoord (type=2..6, NAI-39),
// (*Player).HintPlayer (type=10, NAI-39), (*Player).HintStop
// (type=-1, NAI-39). Closes the partial-encoder follow-up from NAI-37.
```

- [ ] **Step 5.3: Refresh `OpHintArrow` doc-block in `pkg/io/protocol/game/server/prot.go`**

Replace lines 41-44 (the existing doc-block):

```go
// HINT_ARROW — directs the client to render a hint indicator pointing
// at an NPC, player, tile, or to clear (one of 5 type variants in
// TS HintArrowEncoder; goscape ships only the type=1 NPC variant
// at NAI-37 — tracked deviation NAI-37-D-HINTARROW-PARTIAL-ENCODER).
// TS ServerGameProt.HINT_ARROW = (25, 6).
```

With:

```go
// HINT_ARROW — directs the client to render a hint indicator pointing
// at an NPC, player, tile, or to clear. All 5 TS HintArrowEncoder
// type variants are wired: type=1 NPC (NAI-37), type=2..6 TILE (NAI-39),
// type=10 PL (NAI-39), type=-1 STOP (NAI-39).
// TS ServerGameProt.HINT_ARROW = (25, 6).
```

- [ ] **Step 5.4: Verify no code-tree sites remain**

Run: `rg -n "NAI-37-D-HINTARROW-PARTIAL-ENCODER" pkg/ modules/`

Expected: **zero hits**. (Doc-tree matches in `docs/` are historical record and stay.)

- [ ] **Step 5.5: Run full test suite to confirm doc-only changes don't break anything**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`

Expected: all tests pass.

- [ ] **Step 5.6: Run with race detector for final closure check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...`

Expected: all tests pass with no race warnings.

- [ ] **Step 5.7: Commit (close commit per close_commit_memory_trailer.md)**

```bash
git add pkg/script/handlers_player.go modules/world/player_script.go pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "chore(script,world,io): NAI-39 closed — HINT_PL + HINT_COORD + HINT_STOP

Retires NAI-37-D-HINTARROW-PARTIAL-ENCODER at three code-tree sites:
- pkg/script/handlers_player.go:842 (handleHintNpc doc-block)
- modules/world/player_script.go:154 (*Player.HintNpc doc-block)
- pkg/io/protocol/game/server/prot.go:44 (OpHintArrow doc-block)

Each site refreshed to present-state narrative listing the full
HintArrowEncoder coverage (type=1 NAI-37 + type=2..6/10/-1 NAI-39).

Doc-tree matches in docs/ are historical record and intentionally
preserved.

Closes memory: NAI-39 close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Self-review checklist

Per `superpowers:writing-plans` self-review and `plan_test_coverage_crosscheck.md`.

**Spec coverage** (B1–B10 from spec):
- B1 (encoder closure 4 branches): Task 2 (3 (*Player) methods + 4 byte-pin tests; offset-boundaries test covers TILE range)
- B2 (ActivePlayer interface +4): Task 2
- B3 (Self2 field): Task 1
- B4 (requireActivePlayer2): Task 1
- B5 (buildPlayerScriptState extract + dispatch): Task 3
- B6 (runScript signature + 19-callsite threading): Task 3
- B7 (3 handler functions): Task 4
- B8 (mockPlayer extensions): Task 2 step 2.5
- B9 (close NAI-37 deviation, 3 code sites): Task 5
- B10 (open NAI-39 deviation): Task 3 step 3.3 (in `buildPlayerScriptState` doc-comment)

**Test coverage** (A–E from spec test strategy):
- A (9 handler tests): Task 4 step 4.1
- B (3 helper tests): Task 1 step 1.1
- C (4 byte-pin tests): Task 2 step 2.1
- D (5 dispatch tests): Task 3 step 3.1
- E (mockPlayer extensions): Task 2 step 2.5

**Type/signature consistency:**
- `(*Player).HintCoord(offset, x, z, height int)` — Task 2 declaration matches Task 4 dispatch call `s.Self.HintCoord(offset, x, z, height)` ✓
- `(*Player).HintPlayer(slot int)` — Task 2 declaration matches Task 4 dispatch call `s.Self.HintPlayer(s.Self2.Slot())` ✓
- `(*Player).HintStop()` — Task 2 declaration matches Task 4 dispatch call `s.Self.HintStop()` ✓
- `mockHintCoord{offset, x, z, height int}` — Task 2 step 2.5 matches Task 4 test fixture `[]mockHintCoord{{offset: 3, x: 100, z: 200, height: 42}}` ✓
- `runScript(sf, self, target, protect, intArgs, stringArgs)` — Task 3 signature matches all 19 caller updates ✓

**Placeholder scan:** No TBD / TODO / "fill in details" / "add appropriate error handling". Every code step has a concrete code block.

## HEAD baseline

`dc12465` (NAI-39 spec).
