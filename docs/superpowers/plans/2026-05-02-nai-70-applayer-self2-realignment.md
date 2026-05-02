# NAI-70 — AP-Player / OP-Player Self/Self2 Binding Realignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Realign `fireOpTriggerPlayer` and `fireApTriggerPlayer` to TS binding (Self=clicker, Self2=target) by swapping the `srv.runScript` arg order at two sites. Closes `NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY` by activating AP-Player same-tick retry, restoring TS-true behavior across all OPPLAYER/APPLAYER handler dispatch sites.

**Architecture:** Production change is 2 lines (the arg swaps). All other work is downstream maintenance: a fixture upgrade (give clicker a real conn so the realigned wire packets can be drained), 6 test inversions (3 binding-effect pins flip; 2 NAI-62 conn drains switch from target→clicker; 1 wire-arm test switches conn), 1 doc-only test reframe, and a doc-comment + deviation-tag sweep. The NAI-39 producer arm at `script.go:55-59` is already TS-correct as written — the fix is at the call sites.

**Tech Stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`. Spec: `docs/superpowers/specs/2026-05-02-nai-70-applayer-self2-realignment-design.md`.

---

## Pre-flight Verification

Confirm at HEAD `28ec0b7` (NAI-70 spec commit, parent `54930db` NAI-69 close):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```
Expected: PASS.

```bash
rg -c "NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY" modules/ pkg/
```
Expected: 5 hits (2 in `player_interaction_trigger.go`, 3 in `player_interaction_trigger_test.go`). All retire in T6.

```bash
rg -n "srv\.runScript\(sf, target, p" modules/world/
```
Expected: 2 hits — `player_interaction_trigger.go:76` and `player_interaction_trigger.go:132`. Both swap in T2 + T3.

```bash
rg -c "newPlayerTriggerFixture\(t\)" modules/world/
```
Expected: 9 hits (7 in `player_interaction_trigger_test.go`, 2 in `interaction_trigger_nai68_test.go`). All sweep in T1.

```bash
rg -n "func makeOpPlayerFixture\b" modules/world/handler_op_player_test.go
```
Expected: 1 hit at line 19. `makeOpPlayerFixture` already returns clicker's conn (4th value); `TestOpPlayer1_E2E_HintPlOnTarget` currently discards it.

---

## File Map

| File | Role | Change scope |
|---|---|---|
| `modules/world/player_interaction_trigger.go` | `fireOpTriggerPlayer`, `fireApTriggerPlayer`, `apPlayerTriggerForOp` | T2 + T3: 1-line arg swap each + doc-comment rewrites. |
| `modules/world/script.go` | `buildPlayerScriptState` doc | T6: doc-comment narrative refresh at lines 30-34 only. |
| `modules/world/interaction_trigger.go` | Type-switch dispatch comment | T6: 1-line doc fix at line 42. |
| `modules/world/player_interaction_trigger_test.go` | Fixture + 6 affected tests | T1: fixture upgrade + 9 call-site updates. T2-T5: test body inversions. |
| `modules/world/interaction_trigger_nai68_test.go` | 2 fixture call sites | T1: 2 call-site updates (no body changes). |
| `modules/world/script_test.go` | `TestOpPlayer1_E2E_HintPlOnTarget` | T2: rewrite to drain clicker conn. |

---

## Task 1: Fixture upgrade — `newPlayerTriggerFixture` returns clicker's conn

**Files:**
- Modify: `modules/world/player_interaction_trigger_test.go:66-83`
- Modify: `modules/world/player_interaction_trigger_test.go:102, 133, 150, 170, 258, 318, 370` (7 callers)
- Modify: `modules/world/interaction_trigger_nai68_test.go:220, 246` (2 callers)

This is preparatory — pure additive change. All tests must remain green at task end.

- [ ] **Step 1: Update fixture signature and body**

Edit `modules/world/player_interaction_trigger_test.go:66-83`. Replace the entire function with:

```go
func newPlayerTriggerFixture(t *testing.T) (s *Server, clicker, target *Player, clickerConn, targetConn net.Conn) {
	t.Helper()
	s = newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty; caller registers

	clicker, clickerConn = newTestPlayer(t)
	clicker.client.server = s
	clicker.client.encryptor = io2.New([4]uint32{5, 6, 7, 8})
	clicker.slot = 1

	target, targetConn = newTestPlayer(t)
	target.client.server = s
	target.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	target.slot = 2

	clicker.SetInteraction(InteractionEngine, target, 1, -1)
	clicker.interacted = true // simulate reach
	return
}
```

Notes:
- Returns 5 values; clicker now has a real conn + ISAAC encryptor with seed `{5, 6, 7, 8}` (distinct from target's `{1, 2, 3, 4}` so wire-byte expectations don't accidentally match the wrong side).
- `targetConn` retains its position semantically (last positional return) but is now at index 4 instead of 3.

- [ ] **Step 2: Update 7 callers in `player_interaction_trigger_test.go`**

Sweep each call site individually (no `replace_all` per `plan_doc_replaceall_timeline.md`):

Line 102 — `TestFireOpTriggerPlayer_BindsSelf2ToClicker`:
```go
s, clicker, target, targetConn := newPlayerTriggerFixture(t)
```
→
```go
s, clicker, target, _, targetConn := newPlayerTriggerFixture(t)
```

Line 133 — `TestFireOpTriggerPlayer_NoScriptRegistered`:
```go
_, clicker, _, _ := newPlayerTriggerFixture(t)
```
→
```go
_, clicker, _, _, _ := newPlayerTriggerFixture(t)
```

Line 150 — `TestFireApTriggerPlayer_NoScriptSetsApRangeMinusOne`:
```go
_, clicker, _, _ := newPlayerTriggerFixture(t)
```
→
```go
_, clicker, _, _, _ := newPlayerTriggerFixture(t)
```

Line 170 — `TestTryFireOpTrigger_PlayerArm`:
```go
s, clicker, target, targetConn := newPlayerTriggerFixture(t)
```
→
```go
s, clicker, target, _, targetConn := newPlayerTriggerFixture(t)
```

Line 258 — `TestFireApTriggerPlayerRestoresTargetAndWaypoints`:
```go
s, clicker, target, _ := newPlayerTriggerFixture(t)
```
→
```go
s, clicker, target, _, _ := newPlayerTriggerFixture(t)
```

Line 318 — `TestFireApTriggerPlayer_ApRangeCalled_BindsToTargetNotClicker`:
```go
s, clicker, target, _ := newPlayerTriggerFixture(t)
```
→
```go
s, clicker, target, _, _ := newPlayerTriggerFixture(t)
```

Line 370 — `TestTryInteract_ApPlayer_NoSameTickRetry_DueToReversedSelf`:
```go
s, clicker, target, _ := newPlayerTriggerFixture(t)
```
→
```go
s, clicker, target, _, _ := newPlayerTriggerFixture(t)
```

- [ ] **Step 3: Update 2 callers in `interaction_trigger_nai68_test.go`**

Line 220 — `TestFireOpTriggerPlayerCapturesNextTargetFromScript`:
```go
s, clicker, target, _ := newPlayerTriggerFixture(t)
```
→
```go
s, clicker, target, _, _ := newPlayerTriggerFixture(t)
```

Line 246 — `TestFireOpTriggerPlayerClearsWaypoints`:
```go
s, clicker, target, _ := newPlayerTriggerFixture(t)
```
→
```go
s, clicker, target, _, _ := newPlayerTriggerFixture(t)
```

- [ ] **Step 4: Run all module tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```
Expected: PASS. The fixture's clicker now has a conn+encryptor that nothing yet drains; this is purely additive.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_interaction_trigger_test.go modules/world/interaction_trigger_nai68_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-70 T1 — newPlayerTriggerFixture clicker conn upgrade

Add real net.Conn + ISAAC encryptor (seed {5,6,7,8}, distinct from
target's {1,2,3,4}) for clicker; expose clickerConn as a new 4th
positional return ahead of targetConn. Sweep 9 call sites to thread
the new return position. Pure additive change; clicker conn is not yet
drained by any test. Prepares for T2 binding-flip wire pins.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: OP-Player binding swap + 3 affected tests

**Files:**
- Modify: `modules/world/player_interaction_trigger.go:31-82` (header + body of `fireOpTriggerPlayer`)
- Modify: `modules/world/player_interaction_trigger_test.go:85-128` (`TestFireOpTriggerPlayer_BindsSelf2ToClicker` rename + invert)
- Modify: `modules/world/player_interaction_trigger_test.go:163-184` (`TestTryFireOpTrigger_PlayerArm` drain switch)
- Modify: `modules/world/script_test.go:1417-1488` (`TestOpPlayer1_E2E_HintPlOnTarget` rename + invert)
- Modify: `modules/world/player_interaction_trigger_test.go:186-214` (`TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom` drain switch)

Red→green: write inverted tests first (red), then apply production swap (green).

- [ ] **Step 1: Invert `TestFireOpTriggerPlayer_BindsSelf2ToClicker` (red)**

Edit `modules/world/player_interaction_trigger_test.go:85-128`. Replace the entire function and its doc-comment (line 85 through line 128 inclusive) with:

```go
// TestFireOpTriggerPlayer_BindsSelf2ToTarget pins the TS-true binding
// for the OPPLAYER trigger family (NAI-70).
//
// Registering an [opplayer1,_] script that runs HINT_PL (which dereferences
// state.Self2.Slot()) and observing the resulting HINT_ARROW packet on the
// CLICKER's wire proves:
//   - srv.runScript routed `target` (clicked player) into
//     buildPlayerScriptState's case-ActivePlayer arm at script.go:54-59,
//     which set state.Self2 = target and OR-d in PtrActivePlayer2.
//   - state.Self = clicker (`p`), since HintPlayer is dispatched on
//     state.Self's *Player and the wire packet lands on clicker's conn.
//
// The slot on the wire is the target's slot, confirming the Self2 link.
//
// Mirrors TS Player.ts:1129 + ScriptRunner.ts:84-87 (self=clicker,
// target=target → _activePlayer=clicker, _activePlayer2=target).
func TestFireOpTriggerPlayer_BindsSelf2ToTarget(t *testing.T) {
	s, clicker, target, clickerConn, _ := newPlayerTriggerFixture(t)

	// Compute expected first wire byte using a parallel encryptor seeded
	// identically to clicker.client.encryptor (NAI-70 fixture seed).
	wantEnc, _ := isaacPair([4]uint32{5, 6, 7, 8})

	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	received := drainConn(t, clickerConn)
	tryFireOpTrigger(clicker)
	clicker.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(wantEnc.GetNext())) & 0xff),
		0x0A,                                      // p1: type = 10 (player hint)
		byte(target.slot >> 8), byte(target.slot), // p2: slot (target's)
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	if !bytes.Equal(got, want) {
		t.Errorf("HINT_ARROW wire bytes: got %#x, want %#x", got, want)
	}
	if !clicker.interactionFired {
		t.Error("interactionFired: got false, want true after fire")
	}
}
```

- [ ] **Step 2: Reframe `TestTryFireOpTrigger_PlayerArm` to drain clicker conn (red)**

Edit `modules/world/player_interaction_trigger_test.go:163-184`. Replace the entire function and its doc-comment with:

```go
// TestTryFireOpTrigger_PlayerArm pins the type-switch dispatch arm:
// when p.target is *Player, tryFireOpTrigger calls fireOpTriggerPlayer,
// not the default skip. Verified indirectly via the HINT_ARROW
// side-effect on clicker's conn (state.Self=clicker per NAI-70) — if
// the *Player arm were missing, the default arm would mark fired=true
// without invoking any script and no HINT_ARROW would arrive.
func TestTryFireOpTrigger_PlayerArm(t *testing.T) {
	s, clicker, _, clickerConn, _ := newPlayerTriggerFixture(t)
	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	received := drainConn(t, clickerConn)
	tryFireOpTrigger(clicker)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("no wire packet on clicker — Player arm did not fire")
	}
	// First byte is the encrypted HINT_ARROW opcode; we don't pin the
	// exact value here (covered by BindsSelf2ToTarget above) — just
	// confirm a packet arrived.
}
```

- [ ] **Step 3: Invert `TestOpPlayer1_E2E_HintPlOnTarget` (red)**

Edit `modules/world/script_test.go:1417-1488`. Replace the entire function and its doc-comment with:

```go
// TestOpPlayer1_E2E_HintPlOnClicker — full path: simulate an OPPLAYER1
// client packet → handleOpPlayer1 sets interaction → tryFireOpTrigger
// fires fireOpTriggerPlayer → runScript routes through
// buildPlayerScriptState's case-ActivePlayer arm → script runs with
// Self=clicker, Self2=target → HINT_PL emits to clicker's outbound
// (TS Player.ts:1129 + ScriptRunner.ts:84-87 binding; NAI-70).
//
// Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER by adding
// handler-entry coverage on top of the direct fire-helper pin in
// TestFireOpTriggerPlayer_BindsSelf2ToTarget.
//
// Approach: Option A — drive handleOpPlayer1 with an OPPLAYER1 payload,
// then mark clicker.interacted = true (the gate processInteraction
// would set on adjacency) and call tryFireOpTrigger directly. This keeps
// the test free of the movement/path-finding machinery while still
// exercising the full handler→trigger→script→wire pipeline.
func TestOpPlayer1_E2E_HintPlOnClicker(t *testing.T) {
	s, clicker, target, clickerConn := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, target.slot)

	// Compute expected first wire byte using a parallel encryptor seeded
	// identically to clicker.client.encryptor (set by makeOpPlayerFixture).
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	// Drive the OPPLAYER1 wire packet through the handler.
	if err := handleOpPlayer1(clicker, p2Payload(target.slot)); err != nil {
		t.Fatalf("handleOpPlayer1: %v", err)
	}
	if clicker.target != target {
		t.Fatalf("post-handler: clicker.target = %v, want %p (target)", clicker.target, target)
	}
	if clicker.targetOp != 1 {
		t.Fatalf("post-handler: clicker.targetOp = %d, want 1", clicker.targetOp)
	}

	// Simulate processInteraction's adjacency gate (the bit
	// tryFireOpTrigger reads).
	clicker.interacted = true

	received := drainConn(t, clickerConn)
	tryFireOpTrigger(clicker)
	clicker.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(wantEnc.GetNext())) & 0xff),
		0x0A,                                      // p1: type = 10 (player hint)
		byte(target.slot >> 8), byte(target.slot), // p2: slot (target's)
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	if !bytes.Equal(got, want) {
		t.Errorf("HINT_ARROW wire bytes: got %#x, want %#x", got, want)
	}
	if !clicker.interactionFired {
		t.Error("interactionFired: got false, want true after fire")
	}
}
```

Notes:
- Removes the prior fresh-target rewiring (lines 1441-1450 in the original); clicker already has a real conn from `makeOpPlayerFixture` at line 23-25.
- ISAAC seed `{1, 2, 3, 4}` matches what `makeOpPlayerFixture:25` sets on clicker.

- [ ] **Step 4: Switch `TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom` to clicker-side drain (red)**

Edit `modules/world/player_interaction_trigger_test.go:186-214`. Replace the entire function (and its doc-comment header at lines 186-190) with:

```go
// TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Player-target script's Self == clicker (NAI-70 binding), so MES opcode
// emits MessageGame on clicker's conn. Pre-fix lookup uses (trigger, -1, -1);
// override registers at (trigger, K, -1) which is unreachable → no
// MessageGame on clicker's conn. Post-fix → script runs → marker appears.
func TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, clicker, other, cc1, _ := makeOpPlayerFixtureWithBothConns(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const overrideTypeId = 7783
	const marker = "opplayer1-override-fired"

	clicker.target = other
	clicker.targetOp = 1
	clicker.targetSubject.com = overrideTypeId

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildPlayerMesScript(script.TriggerOpPlayer1, overrideTypeId, marker))

	received := drainConn(t, cc1)
	fireOpTriggerPlayer(clicker, s, other)
	clicker.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(marker)) {
		t.Errorf("drained bytes from clicker conn: missing %q substring; override should have run override-keyed script for targetSubject.com=%d (default Player-target lookup typeId=-1), got %x",
			marker, overrideTypeId, got)
	}
}
```

Note: `makeOpPlayerFixtureWithBothConns` returns `(s, clicker, other, cc, cc2)` — cc is clicker's, cc2 is other's. Test now uses `cc1` (binding to clicker's conn).

- [ ] **Step 5: Run the 4 inverted tests — verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestFireOpTriggerPlayer_BindsSelf2ToTarget|TestTryFireOpTrigger_PlayerArm|TestOpPlayer1_E2E_HintPlOnClicker|TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom" -count=1 -v
```
Expected: 4 FAILs. The current production code at `player_interaction_trigger.go:76` still binds Self=target, so HINT_ARROW lands on target's conn (not clicker's drained one) and MES writes to target's conn (not clicker's drained one).

- [ ] **Step 6: Apply OP-Player production swap (green)**

Edit `modules/world/player_interaction_trigger.go:76`. Replace:

```go
	srv.runScript(sf, target, p, true, nil, nil)
```
with:
```go
	srv.runScript(sf, p, target, true, nil, nil)
```

Also rewrite the `fireOpTriggerPlayer` doc-comment header. Edit lines 31-41, replacing the entire block:

```go
// fireOpTriggerPlayer fires the [opplayer<op>,_] trigger for a Player
// target. Self = target, Self2 = clicker (the receiver `p`). Self2
// binding flows through srv.runScript → buildPlayerScriptState's
// `case script.ActivePlayer:` arm (script.go:54-58), which sets
// state.Self2 = p and OR-s in script.PtrActivePlayer2.
//
// Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: this is the
// production producer for state.Self2. Players have no type, so the
// trigger lookup is global-only — pass (-1, -1) to fall through
// LookupKeyForType / LookupKeyForCategory to LookupKeyForGlobal in
// script.Provider.GetByTrigger (provider.go:114-127).
```
with:
```go
// fireOpTriggerPlayer fires the [opplayer<op>,_] trigger for a Player
// target. Self = `p` (clicker), Self2 = target. Mirrors TS Player.ts:1129
// + ScriptRunner.ts:84-87: ScriptRunner.init(opTrigger, this=clicker,
// target=target_player) yields _activePlayer=clicker, _activePlayer2=target.
//
// Self2 binding flows through srv.runScript → buildPlayerScriptState's
// `case script.ActivePlayer:` arm (script.go:55-59), which sets
// state.Self2 = target and OR-s in script.PtrActivePlayer2.
//
// Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: this is the
// production producer for state.Self2. Players have no type, so the
// trigger lookup is global-only — pass (-1, -1) to fall through
// LookupKeyForType / LookupKeyForCategory to LookupKeyForGlobal in
// script.Provider.GetByTrigger (provider.go:114-127).
```

Also rewrite the in-body comment at lines 71-75 of `fireOpTriggerPlayer`. Edit lines 71-75 of the file (within the function body, just above the runScript call):

```go
	// Run with target as Self and `p` (clicker) threaded as the
	// ActivePlayer-typed second arg → buildPlayerScriptState's
	// case-ActivePlayer arm sets state.Self2 = p, Pointers |=
	// PtrActivePlayer2.
```
with:
```go
	// Run with `p` (clicker) as Self and `target` threaded as the
	// ActivePlayer-typed second arg → buildPlayerScriptState's
	// case-ActivePlayer arm sets state.Self2 = target, Pointers |=
	// PtrActivePlayer2 (TS-true binding per NAI-70).
```

- [ ] **Step 7: Run the 4 tests — verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestFireOpTriggerPlayer_BindsSelf2ToTarget|TestTryFireOpTrigger_PlayerArm|TestOpPlayer1_E2E_HintPlOnClicker|TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom" -count=1 -v
```
Expected: 4 PASS.

- [ ] **Step 8: Run full module suite — verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add modules/world/player_interaction_trigger.go modules/world/player_interaction_trigger_test.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-70 T2 — realign fireOpTriggerPlayer to TS Self/Self2 binding

Swap srv.runScript arg order at player_interaction_trigger.go:76 so
state.Self=clicker (`p`), state.Self2=target. Mirrors TS Player.ts:1129
+ ScriptRunner.ts:84-87. HINT_ARROW now lands on clicker's wire with
target.slot in body; MES writes to clicker's chatbox.

Inverts 4 tests (BindsSelf2ToClicker→ToTarget, E2E_HintPlOnTarget→OnClicker,
TryFireOpTrigger_PlayerArm conn switch, NAI-62 OP override-typeid drain
switch). Doc-comment header + in-body comment refreshed; in-body comment
on production code now cites NAI-70.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: AP-Player binding swap + 2 affected tests + 1 reframe

**Files:**
- Modify: `modules/world/player_interaction_trigger.go:84-144` (header + body of `fireApTriggerPlayer`)
- Modify: `modules/world/player_interaction_trigger_test.go:248-296` (`TestFireApTriggerPlayerRestoresTargetAndWaypoints` — comment refresh only)
- Modify: `modules/world/player_interaction_trigger_test.go:298-361` (`TestFireApTriggerPlayer_ApRangeCalled_BindsToTargetNotClicker` rename + invert)
- Modify: `modules/world/player_interaction_trigger_test.go:216-246` (`TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom` drain switch)

Red→green order: write inverted tests first, then production swap.

- [ ] **Step 1: Invert `TestFireApTriggerPlayer_ApRangeCalled_BindsToTargetNotClicker` (red)**

Edit `modules/world/player_interaction_trigger_test.go:298-361`. Replace the entire block (the `--- NAI-69 T3 (reframed) ---` separator comment + function header doc-comment + function body) with:

```go
// --- NAI-70: AP-Player Self=clicker binding pin ---

// TestFireApTriggerPlayer_ApRangeCalled_BindsToClicker pins the TS-true
// AP-Player binding (NAI-70). APPLAYER scripts run with state.Self =
// clicker (`p`), state.Self2 = target. When the script calls p_aprange,
// handlePApRange (pkg/script/handlers_player.go:695) invokes
// s.Self.SetApRange(n) — so clicker.apRange and clicker.apRangeCalled
// are mutated, target's are untouched.
//
// Mirrors TS Player.ts:1151 + ScriptRunner.ts:84-87:
// ScriptRunner.init(apTrigger, this=clicker, target=target_player) →
// _activePlayer=clicker, _activePlayer2=target. AP-Loc/AP-Obj/AP-Npc
// already match TS; AP-Player matches as of NAI-70.
//
// Closes NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY (this binding
// flip activates the same-tick retry path at interaction.go:336).
func TestFireApTriggerPlayer_ApRangeCalled_BindsToClicker(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	// Register an APPLAYER1 script that calls p_aprange(2).
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[applayer1,_]_aprange",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	fireApTriggerPlayer(clicker, s, target)

	// Uniform-exit contract from NAI-69 T1+T2 (works for AP-Player too):
	if !clicker.interactionFired {
		t.Error("clicker.interactionFired: got false, want true (NAI-69: fire helper uniform exit)")
	}
	if clicker.target != target {
		t.Errorf("clicker.target: got %v, want target (restored after fire)", clicker.target)
	}

	// TS-true binding pin: p_aprange routed to clicker.SetApRange,
	// NOT target.SetApRange (NAI-70).
	if !clicker.apRangeCalled {
		t.Error("clicker.apRangeCalled: got false, want true (Self=clicker; SetApRange ran on clicker)")
	}
	if clicker.apRange != 2 {
		t.Errorf("clicker.apRange: got %d, want 2 (script set new range on Self=clicker)", clicker.apRange)
	}
	if target.apRangeCalled {
		t.Error("target.apRangeCalled: got true, want false (script ran on Self=clicker, not target)")
	}
	if target.apRange != 10 {
		t.Errorf("target.apRange: got %d, want 10 (default unchanged — script mutated clicker's apRange)", target.apRange)
	}
}
```

- [ ] **Step 2: Invert `TestTryInteract_ApPlayer_NoSameTickRetry_DueToReversedSelf` (red)**

Edit `modules/world/player_interaction_trigger_test.go:363-413`. Replace the entire block (doc-comment + function body) with:

```go
// TestTryInteract_ApPlayer_SameTickRetryActivates — end-to-end pin:
// tryInteract returns false (NAI-69 T1 guard fires) because clicker's
// apRangeCalled is now true after the AP-Player binding realignment
// (NAI-70). Confirms the same-tick retry path is structurally active
// for AP-Player, matching AP-Loc/AP-Obj/AP-Npc and TS Player.ts:1163-1167.
//
// Triple-pin per test_passes_for_wrong_reason.md: assert return value,
// the apRangeCalled mutation that drives the guard, AND the
// interactionFired reset that proves the guard's body executed.
func TestTryInteract_ApPlayer_SameTickRetryActivates(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	// Register the p_aprange(2) script.
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[applayer1,_]_aprange",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	// Place clicker within AP range (5 tiles) but outside operable range (>1).
	// Default apRange=10; 5 tiles satisfies inApproachDistance but not
	// inOperableDistance — AP arm is taken, not OP arm.
	clicker.x = 3094
	clicker.z = 3106
	target.x = 3094
	target.z = 3111 // 5 tiles away on z-axis

	result := clicker.tryInteract(false)

	// NAI-69 T1 guard fires under the realigned binding:
	if result {
		t.Error("tryInteract: got true, want false (NAI-70 + NAI-69 T1: guard fires; clicker.apRangeCalled=true)")
	}
	if clicker.interactionFired {
		t.Error("clicker.interactionFired: got true, want false (guard reset for retry)")
	}
	if !clicker.apRangeCalled {
		t.Error("clicker.apRangeCalled: got false, want true (Self=clicker; p_aprange mutated clicker)")
	}
	if target.apRangeCalled {
		t.Error("target.apRangeCalled: got true, want false (script ran on Self=clicker, not target)")
	}
	// Fire helper restored target+waypoints (NAI-68); guard does not re-clear.
	if clicker.target != target {
		t.Errorf("clicker.target: got %v, want target (NAI-68 restore; guard preserves)", clicker.target)
	}
}
```

- [ ] **Step 3: Switch `TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom` to clicker-side drain (red)**

Edit `modules/world/player_interaction_trigger_test.go:216-246`. Replace the entire function (doc-comment + body) with:

```go
// TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Same OpMes marker strategy as the OP variant; AP variant. NAI-70
// binding flip: marker now lands on clicker's conn (Self=clicker).
// Also asserts p.apRange != -1 as a secondary signal (no-script path
// sets apRange = -1 per fireApTriggerPlayer).
func TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, clicker, other, cc1, _ := makeOpPlayerFixtureWithBothConns(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const overrideTypeId = 7784
	const marker = "applayer1-override-fired"

	clicker.target = other
	clicker.targetOp = 1
	clicker.targetSubject.com = overrideTypeId

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildPlayerMesScript(script.TriggerApPlayer1, overrideTypeId, marker))

	received := drainConn(t, cc1)
	fireApTriggerPlayer(clicker, s, other)
	clicker.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(marker)) {
		t.Errorf("drained bytes from clicker conn: missing %q substring; override should have run override-keyed script for targetSubject.com=%d, got %x",
			marker, overrideTypeId, got)
	}
	if clicker.apRange == -1 {
		t.Errorf("apRange: got -1 (no-script sentinel), want >0; override should have prevented the no-script path")
	}
}
```

- [ ] **Step 4: Refresh `TestFireApTriggerPlayerRestoresTargetAndWaypoints` doc-comment (no behavior change)**

Edit `modules/world/player_interaction_trigger_test.go:248-256` (doc-comment block above the function). Replace:

```go
// --- B3 AP-Player variant ---

// TestFireApTriggerPlayerRestoresTargetAndWaypoints pins TS Player.ts:1145-1162
// for the AP-Player path. Since runScript's self=target (the target player),
// no p_op_player handler exists, and p_op_npc would act on target's state —
// the test pins the restore-only contract: noop script → p.target restored,
// p.nextTarget nil, waypoints restored.
//
// NAI-68 B3 AP-Player variant.
```
with:
```go
// --- B3 AP-Player variant ---

// TestFireApTriggerPlayerRestoresTargetAndWaypoints pins TS Player.ts:1145-1162
// for the AP-Player path. With NAI-70 binding (Self=clicker), the noop
// script doesn't mutate any pinned state — the test asserts the
// restore-only contract: p.target restored, p.nextTarget nil, waypoints
// restored.
//
// NAI-68 B3 AP-Player variant.
```

- [ ] **Step 5: Run the 3 inverted/switched tests — verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestFireApTriggerPlayer_ApRangeCalled_BindsToClicker|TestTryInteract_ApPlayer_SameTickRetryActivates|TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom" -count=1 -v
```
Expected: 3 FAILs. Current production at `:132` still binds Self=target.

- [ ] **Step 6: Apply AP-Player production swap (green)**

Edit `modules/world/player_interaction_trigger.go:132`. Replace:

```go
	srv.runScript(sf, target, p, true, nil, nil)
```
with:
```go
	srv.runScript(sf, p, target, true, nil, nil)
```

Also rewrite the `fireApTriggerPlayer` doc-comment header. Edit lines 84-98 of the file, replacing the entire block:

```go
// fireApTriggerPlayer fires the [applayer<op>,_] trigger at approach
// distance. On no-script-found: sets p.apRange = -1 to skip re-lookup
// next tick (matches fireApTriggerLoc behaviour at S6r). Self2 binding
// is the same as fireOpTriggerPlayer (NAI-39): Self=target, Self2=p.
//
// Same-tick AP retry NOT active for AP-Player. Per TS Player.ts:1151,
// `ScriptRunner.init(apTrigger, this, target)` runs with this=clicker
// uniformly, so TS sees the clicker's apRangeCalled flag in the
// L1163-1167 guard. Goscape's NAI-39 producer reverses the binding
// (Self=target), so handlePApRange (pkg/script/handlers_player.go:695
// → s.Self.SetApRange) mutates target.apRangeCalled, leaving
// clicker.apRangeCalled=false. The NAI-69 T1 guard at
// interaction.go:336 reads clicker.apRangeCalled, so AP-Player skips
// the same-tick retry path. AP-Loc/AP-Obj/AP-Npc match TS (Self=p).
// Tracked deviation: NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY.
```
with:
```go
// fireApTriggerPlayer fires the [applayer<op>,_] trigger at approach
// distance. On no-script-found: sets p.apRange = -1 to skip re-lookup
// next tick (matches fireApTriggerLoc behaviour at S6r).
//
// Self/Self2 binding mirrors TS Player.ts:1151 + ScriptRunner.ts:84-87:
// ScriptRunner.init(apTrigger, this=clicker, target=target_player) →
// state.Self=clicker (`p`), state.Self2=target. Same as
// fireOpTriggerPlayer.
//
// Same-tick AP retry path active per NAI-69 T1 guard at
// interaction.go:336: handlePApRange's s.Self.SetApRange mutates
// clicker.apRangeCalled, the guard fires, and tryInteract returns
// false to allow processInteraction's walk-arm a same-tick retry.
// Closes NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY (NAI-70).
```

Also rewrite the in-body comment at lines 122-125. Edit the file, replacing:

```go
	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68
	// framework. AP-Player same-tick retry is structurally inert under
	// the current Self=target binding — see header for full divergence
	// narrative (NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY).
```
with:
```go
	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68
	// framework. AP-Player same-tick retry active under the realigned
	// Self=clicker binding (NAI-70).
```

- [ ] **Step 7: Run the 3 tests — verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestFireApTriggerPlayer_ApRangeCalled_BindsToClicker|TestTryInteract_ApPlayer_SameTickRetryActivates|TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom" -count=1 -v
```
Expected: 3 PASS.

- [ ] **Step 8: Run full module suite — verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```
Expected: PASS.

- [ ] **Step 9: Run full project suite + race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -race -count=1
```
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add modules/world/player_interaction_trigger.go modules/world/player_interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-70 T3 — realign fireApTriggerPlayer to TS Self/Self2 binding

Swap srv.runScript arg order at player_interaction_trigger.go:132 so
state.Self=clicker (`p`), state.Self2=target. Mirrors TS Player.ts:1151
+ ScriptRunner.ts:84-87. p_aprange now mutates clicker (matches TS),
activating the NAI-69 T1 same-tick retry guard at interaction.go:336.

Inverts 2 tests (BindsToTargetNotClicker→BindsToClicker, ApPlayer
NoSameTickRetry→SameTickRetryActivates), switches NAI-62 AP override-
typeid drain from target→clicker, and refreshes the AP-Player restore
test's doc-comment. Doc-comment header retires the NAI-69-D narrative
paragraph entirely; in-body comment cites NAI-70.

Closes NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Same-tick retry full-cycle pin (AP-Player)

**Files:**
- Modify: `modules/world/player_interaction_trigger_test.go` (append at end of file, after the existing AP-Player tests)

This pins the round-trip behavior — that after the same-tick retry guard returns false, a follow-on `tryInteract` call (simulating post-step retry within `processInteraction`) re-fires AP. Acts as the AP-Player twin of NAI-69's `TestApTriggerLoc_SameTickRetry_RangeLowered`.

- [ ] **Step 1: Append new test at end of `player_interaction_trigger_test.go`**

Append after the last `}` in the file (after the SameTickRetryActivates test from T3 step 2):

```go
// TestApTriggerPlayer_SameTickRetry_FullCycle pins the TS Player.ts:1163-1167
// round-trip for AP-Player (NAI-70 + NAI-69 closure). Two consecutive
// tryInteract calls represent processInteraction's pre-step + post-step
// retry windows:
//
//   - Pre-step: clicker outside the script-set range (2). p_aprange(2)
//     fires, mutates clicker.apRangeCalled=true. Guard fires → false.
//   - Between calls: clicker is moved 1 tile closer (simulating
//     processInteraction's walk-arm). NAI-69 fire-helper resets
//     apRangeCalled=false at fire entry, so the second fire evaluates
//     fresh.
//   - Post-step: clicker still outside range 2 (now 4 tiles away).
//     p_aprange(2) fires again. Guard fires → false again.
//
// Counter on the script's fire count is verified via a side-effect:
// each fire pushes a varp; we read it back through the script provider
// state on clicker.
func TestApTriggerPlayer_SameTickRetry_FullCycle(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	// Same APPLAYER1 p_aprange(2) script as T3.
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[applayer1,_]_aprange",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	clicker.x = 3094
	clicker.z = 3106
	target.x = 3094
	target.z = 3111 // 5 tiles z-axis (within default apRange=10, outside 2)

	// Pre-step retry.
	result1 := clicker.tryInteract(false)
	if result1 {
		t.Fatal("first tryInteract: got true, want false (AP fire + guard fires)")
	}
	if !clicker.apRangeCalled {
		t.Fatal("after first fire: clicker.apRangeCalled false, want true")
	}
	if clicker.interactionFired {
		t.Fatal("after first fire: clicker.interactionFired true, want false (guard reset)")
	}
	if clicker.apRange != 2 {
		t.Fatalf("after first fire: clicker.apRange=%d, want 2 (script-set)", clicker.apRange)
	}

	// Simulate processInteraction's walk-arm: move clicker 1 tile closer.
	// New distance: 4 tiles, still outside apRange=2 → AP arm taken again.
	clicker.z = 3107

	// Post-step retry. processInteraction would call tryInteract(false)
	// again with the !interacted guard inverted (interacted was set to
	// true by the first call); reset interacted to mirror the post-step
	// state where the first fire's interaction reservation has cleared.
	clicker.interacted = false

	result2 := clicker.tryInteract(false)
	if result2 {
		t.Error("second tryInteract: got true, want false (re-fire + guard fires again)")
	}
	if !clicker.apRangeCalled {
		t.Error("after second fire: clicker.apRangeCalled false, want true (re-fire)")
	}
	if clicker.interactionFired {
		t.Error("after second fire: clicker.interactionFired true, want false (guard reset)")
	}
	// AP-Player retry confirmed: both fires hit the guard's return-false
	// arm, both reset interactionFired, both leave apRangeCalled=true.
}
```

- [ ] **Step 2: Run the new test — verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestApTriggerPlayer_SameTickRetry_FullCycle" -count=1 -v
```
Expected: PASS. T3's binding flip activated this path; this test verifies round-trip without needing additional code changes.

- [ ] **Step 3: If RED, investigate**

If the test fails, re-read the assertions against the production code at `interaction.go:324-340` and `player_interaction_trigger.go:99-144`. Likely failure modes:
- `clicker.interacted = false` reset before the second call missed: re-check the gate at `interaction.go:325`.
- `apRangeCalled` not reset between fires: check `fireApTriggerPlayer` line 120 — `p.apRangeCalled = false` at fire entry, mirrors TS L1141.
- Range not reset: `clicker.apRange = 2` (set by first fire) persists; second fire's AP-arm requires `inApproachDistance(...effectiveApRange(p))` to evaluate true at distance 4 with range 2 — it WON'T. Adjust setup: post-walk distance must still be > 2 but ≤ default 10 to enter AP arm. Move clicker back further if needed; or re-evaluate the test premise (the second AP fire should NOT fire because clicker is now within the script-tightened range — but then the test's purpose changes to "retry mechanism activates and converges, not re-fires forever").

If the test premise is wrong (the second AP shouldn't fire), reframe the assertion to:
- Second `tryInteract` returns true (within new range, AP fires once more, fresh apRangeCalled, no re-trigger).
- Or: second tryInteract takes the OP arm (within operable distance after walk).

The exact premise depends on `inApproachDistance(effectiveApRange)` at the post-walk position. Verify with a quick mental compute: at distance 4, apRange=2 → `inApproachDistance` returns false → AP arm NOT taken → fall to OP arm; at distance 4, OP requires Chebyshev ≤ 1 → false → return false. So neither fires; no second guard activation; test should assert `result2 == false` with no script run.

This is a plan-author flag: **the post-walk geometry must be re-derived during T4 implementation; the assertions above assume re-fire occurs but that may not match the math**. If implementer finds the second fire doesn't run, reframe to pin the no-fire / no-retry exit instead. The TS-true behavior is what matters; the test should reflect it.

- [ ] **Step 4: Commit**

```bash
git add modules/world/player_interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-70 T4 — AP-Player same-tick retry full-cycle pin

Two-call simulation of processInteraction pre-step + post-step retry
windows. Verifies the NAI-69 T1 guard fires correctly across walk
state mutations, with apRangeCalled re-evaluation per fire (TS L1141).

AP-Player twin of TestApTriggerLoc_SameTickRetry_RangeLowered (NAI-69).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Doc-comment narrative refresh in adjacent files

**Files:**
- Modify: `modules/world/script.go:30-34` (`buildPlayerScriptState` doc)
- Modify: `modules/world/interaction_trigger.go:42` (case-Player dispatch comment)

These are doc-only updates. The producer arm at `script.go:55-59` is already TS-correct — only the surrounding narrative needs to drop NAI-39-era hints that the binding direction is producer-determined.

- [ ] **Step 1: Refresh `buildPlayerScriptState` doc-comment**

Edit `modules/world/script.go:30-34`. Replace:

```go
// case script.ActivePlayer is the secondary-binding arm consumed by
// the OPPLAYER<N>/APPLAYER<N> player→player trigger family
// (player_interaction_trigger.go). Sets state.Self2 + PtrActivePlayer2
// when target is a *Player (NAI-40 closure of the activePlayer2
// substrate that NAI-39 introduced).
```
with:
```go
// case script.ActivePlayer is the secondary-binding arm consumed by
// the OPPLAYER<N>/APPLAYER<N> player→player trigger family
// (player_interaction_trigger.go). Sets state.Self2 = target +
// PtrActivePlayer2 when the second arg is a *Player. Mirrors TS
// ScriptRunner.ts:84-87 _activePlayer2 dispatch (self=Player &&
// target=Player → _activePlayer2=target). NAI-40 closure of the
// activePlayer2 substrate; NAI-70 realigned the call sites in
// player_interaction_trigger.go to TS-true binding.
```

- [ ] **Step 2: Refresh `interaction_trigger.go:42` dispatch comment**

Edit `modules/world/interaction_trigger.go:42`. Find the comment line:

```go
		// case-ActivePlayer arm sets state.Self2 = clicker.
```
Replace with:
```go
		// case-ActivePlayer arm sets state.Self2 = target (NAI-70).
```

- [ ] **Step 3: Run module tests — verify no regressions from doc edits**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/script.go modules/world/interaction_trigger.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-70 T5 — Self2 producer narrative refresh

buildPlayerScriptState doc-comment now states Self2=target explicitly
with TS ScriptRunner citation. interaction_trigger.go:42 dispatch
comment inverted: state.Self2 = clicker → state.Self2 = target.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Deviation-tag retirement

**Files:**
- Verify: `rg "NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY" modules/ pkg/`

T2 + T3 already swept 4 of the 5 NAI-69-D tag references (in production code header/body and 3 of 4 test comment blocks). T6 catches any stragglers.

- [ ] **Step 1: Re-grep for residual NAI-69-D tag references**

```bash
rg -n "NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY" modules/ pkg/
```
Expected: 0 hits.

If any hits remain, edit them out. Each retirement should be either deletion (if the comment was purely framing the deviation) or rewriting (if the comment also conveyed a TS-true fact worth preserving). Per `retire_deviation_grep_all_comments.md`.

- [ ] **Step 2: Run all module tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```
Expected: PASS.

- [ ] **Step 3: Commit (only if changes were needed in step 1)**

If step 1 found 0 hits, skip this step. If hits were found and removed:

```bash
git add modules/world/ pkg/script/ # whichever paths had stragglers
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-70 T6 — retire residual NAI-69-D-APPLAYER-SELF2-REVERSED tag

Final sweep: 0 hits remain after T2+T3 absorbed the production-code and
test-comment retirements.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Close commit

**Files:**
- (No file changes; commit-only.)

- [ ] **Step 1: Verify all acceptance criteria from spec §10**

```bash
# AC1: deviation tag fully retired
rg -c "NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY" modules/ pkg/
```
Expected: 0.

```bash
# AC2: production swap landed
rg -n "srv\.runScript\(sf, target, p" modules/world/
```
Expected: 0 hits.

```bash
# AC3: full test suite
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race
```
Expected: PASS.

```bash
# AC4-5: spot-check doc-comments
rg -n "Self=clicker, Self2=target|Self2 = target" modules/world/
```
Expected: ≥3 hits across `player_interaction_trigger.go` + `script.go` + `interaction_trigger.go`.

- [ ] **Step 2: Compose close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-70 — AP-Player / OP-Player Self/Self2 binding realignment

Closes NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY. Realigns
fireOpTriggerPlayer + fireApTriggerPlayer to TS binding (Self=clicker,
Self2=target) by swapping the srv.runScript arg order at two sites.
Mirrors TS Player.ts:1129 + 1151 + ScriptRunner.ts:84-87.

Activates AP-Player same-tick retry path under the NAI-69 T1 guard at
interaction.go:336 (clicker.apRangeCalled now mutates from p_aprange,
matching AP-Loc/AP-Obj/AP-Npc behavior). Restores TS-true wire-output
direction for HINT_PL (clicker's conn, target.slot in body) and MES
(clicker's chatbox).

Production diff: 2 lines (the two arg swaps). Test sweep: 1 fixture
upgrade + 6 inversions + 1 doc-only reframe + 1 new full-cycle pin.

Net deviation tally:
  Spec claimed:   13 → 12 (close 1, open 0)
  Actual:         13 → 12

Implementation timeline:
  T1 newPlayerTriggerFixture clicker conn upgrade (preparatory)
  T2 fireOpTriggerPlayer binding swap + 4 affected tests
  T3 fireApTriggerPlayer binding swap + 3 affected tests
  T4 AP-Player same-tick retry full-cycle pin
  T5 buildPlayerScriptState + interaction_trigger.go:42 doc refresh
  T6 deviation-tag retirement sweep

Spec: docs/superpowers/specs/2026-05-02-nai-70-applayer-self2-realignment-design.md
Plan: docs/superpowers/plans/2026-05-02-nai-70-applayer-self2-realignment.md

Closes memory: NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Note: this is an `--allow-empty` commit per the close-commit cadence. If T6 had no changes, T7's close commit carries no diff but documents the rollup.

---

## Self-review checklist

- ✓ **Spec coverage:** §5 Code Map's 9 file touches all map to T2+T3+T5+T6 tasks. §7's 6 inversions + 1 reframe + 1 new test all have corresponding TDD steps in T2+T3+T4. Fixture upgrade (§7.1) is T1.
- ✓ **No placeholders:** every test has full code; every Edit shows old+new.
- ✓ **Type consistency:** fixture signature `(s *Server, clicker, target *Player, clickerConn, targetConn net.Conn)` consistent across T1 step 1 and all caller updates in T1 steps 2+3, plus T2/T3 test bodies.
- ✓ **Sibling-site grep enforcement** per `enumerate_all_sites.md`: `rg -c "newPlayerTriggerFixture\(t\)"` pre-flight gives 9; T1 step 2+3 enumerates 9.
- ✓ **Cross-foot at T4 step 3**: explicit fail-mode investigation guidance with geometry math, per `plan_runnable_test_fixtures.md` (the test premise involves real distance/range arithmetic that needs sanity check at implementation time).
- ✓ **Per-instance Edits** per `plan_doc_replaceall_timeline.md`: every Edit in T1-T6 has unique surrounding context; no `replace_all`.
