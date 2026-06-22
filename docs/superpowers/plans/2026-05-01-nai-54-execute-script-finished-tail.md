# NAI-54 — executeScript Finished/Aborted tail port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the full TS `Player.executeScript` Finished/Aborted tail (`Player.ts:2143-2148`) and `Npc.executeScript` Finished/Aborted tail (`Npc.ts:226-228`) into goscape's `resumeOrFinish` / `resumeOrFinishNpc` via a new `OnScriptFinishedOrAborted(state)` interface method, closing NAI-53-F1 and NAI-53-F2.

**Architecture:** Add `OnScriptFinishedOrAborted(state *ScriptState)` to `script.ActivePlayer` and `script.ActiveNpc` interfaces. `*Player` impl performs `if p.activeScript != state { return }; p.activeScript = nil; if p.modalState&modalStateMain == modalStateNone { p.CloseModal(false) }`. `*Npc` impl performs the matched-clear without modal handling. `resumeOrFinish` / `resumeOrFinishNpc` swap the unconditional `ClearActiveScript()` call in their Finished/Aborted arm for the new method. Three test mocks (`mockPlayer`, `mockActiveNpc`, `mockNpc`) gain stub impls.

**Tech Stack:** Go 1.26+. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-01-nai-54-execute-script-finished-tail-design.md` (commit `0f369f0`).

---

## Pre-flight context (verified at HEAD `0f369f0`)

**Production interface impls:**
- `(*Player)` — `modules/world/player_script.go:137,143` (StoreActiveScript, ClearActiveScript).
- `(*Npc)` — `modules/world/npc.go:217,224` (StoreActiveScript, ClearActiveScript).

**Test-mock impls (must update in T3):**
- `mockPlayer` — `pkg/script/runner_test.go:99` (struct), `:319-320` (current StoreActiveScript / ClearActiveScript impl).
- `mockActiveNpc` — `pkg/script/handlers_player_test.go:20` (struct), `:43-44` (impls).
- `mockNpc` — `pkg/script/handlers_npc_test.go:186` (struct), `:277-278` (impls).

**Call-site swap targets:**
- `modules/world/script.go:113-114` — Finished/Aborted arm in `resumeOrFinish`.
- `modules/world/npc_script.go:304-305` — Finished/Aborted arm in `resumeOrFinishNpc`.

**Existing test infra to reuse:**
- `newTestPlayer(t)` — `modules/world/player_test.go:15`.
- `newTestServer(t)` — `modules/world/server_test.go:311`.
- `buildNpcForIntegration(t)` — `modules/world/npc_script_test.go:231`.
- `IF_CLOSE` dispatch fixture pattern — `TestCloseModalIfCloseDispatchMain` at `modules/world/modal_close_test.go:297-325`.

**Re-verify at HEAD before each implementer dispatch (per `controller_preflight.md`):**

```
git rev-parse HEAD                        # confirm 0f369f0 or descendant
rg -n 'StoreActiveScript|ClearActiveScript' pkg/script/active.go
rg -n 'func \(m \*mockPlayer\) ClearActiveScript' pkg/script/runner_test.go
rg -n 'func \(m \*mockActiveNpc\) ClearActiveScript' pkg/script/handlers_player_test.go
rg -n 'func \(m \*mockNpc\) ClearActiveScript' pkg/script/handlers_npc_test.go
sed -n '110,120p' modules/world/script.go
sed -n '300,310p' modules/world/npc_script.go
```

If any line numbers have drifted, update the per-task references before dispatching.

---

## Task 1: `(*Player).OnScriptFinishedOrAborted` matrix

**Files:**
- Test: `modules/world/player_script_test.go` (append at end of file).
- Modify: `modules/world/player_script.go` (append after `ClearActiveScript` at line 145).

**TS reference:** `Engine-TS/src/engine/entity/Player.ts:2143-2148`.

**Test fixture cases (4):**
- `match-no-MAIN` — `p.activeScript = X`; `modalState = chat`; `modalChat = 100`. Call `p.OnScriptFinishedOrAborted(X)`. Assert: `activeScript == nil`; `modalState == NONE`; `refreshModalClose == true`; `modalChat == -1`.
- `match-with-MAIN` — `p.activeScript = X`; `modalState = main|chat`; `modalMain = 200`; `modalChat = 100`. Call `p.OnScriptFinishedOrAborted(X)`. Assert: `activeScript == nil`; `modalState == main|chat` (UNCHANGED); `refreshModalClose == false`; `modalMain == 200` (UNCHANGED); `modalChat == 100` (UNCHANGED).
- `mismatch` — `p.activeScript = X`; `modalState = chat`; `modalChat = 100`. Build a separate `Y := &script.ScriptState{...}` (not `X`). Call `p.OnScriptFinishedOrAborted(Y)`. Assert: `activeScript == X` (preserved); `modalState == chat` (unchanged); `refreshModalClose == false`.
- `nil-active` — `p.activeScript = nil`; `modalState = chat`; `modalChat = 100`. Build `Y := &script.ScriptState{...}`. Call `p.OnScriptFinishedOrAborted(Y)`. Assert: `activeScript == nil`; `modalState == chat` (unchanged); no panic.

The `match-no-MAIN` test reaches the `CloseModal(false)` branch → resets slots to -1, sets `refreshModalClose=true`. The fixture does NOT need a real Server (the slot-reset path runs in CloseModal's `else` branch at `player_script.go:639-643` when `p.client == nil || p.client.server == nil`). `newTestPlayer(t)` returns a Player whose `p.client.server` is nil by default — confirmed.

- [ ] **Step 1: Write the failing test**

Append to `modules/world/player_script_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail (RED — undefined method)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestPlayerOnScriptFinishedOrAborted' ./modules/world/...
```

Expected: build fails with `p.OnScriptFinishedOrAborted undefined (type *Player has no field or method OnScriptFinishedOrAborted)`.

- [ ] **Step 3: Implement the minimal code to make tests pass**

Append to `modules/world/player_script.go` immediately after `ClearActiveScript` (currently ends at line 145):

```go
// OnScriptFinishedOrAborted handles the Finished/Aborted post-Execute
// tail for a player-anchored script. If state is the player's current
// activeScript, nulls it; and if no MAIN modal is open, fires
// CloseModal(false) to auto-close any open chat / side dialogue.
//
// Mirrors TS Player.executeScript Finished/Aborted tail
// (Player.ts:2143-2148). The match-guard preserves a Suspended /
// PauseButton / CountDialog activeScript when a different fresh script
// Finishes on the same player in the same tick. The MAIN-bit gate on
// CloseModal preserves any open main modal while dropping chat /
// side dialogues — TS comment: "close chat dialogues automatically
// and leave main modals alone".
//
// NAI-54 T1.
func (p *Player) OnScriptFinishedOrAborted(state *script.ScriptState) {
	if p.activeScript != state {
		return
	}
	p.activeScript = nil
	if p.modalState&modalStateMain == modalStateNone {
		p.CloseModal(false)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestPlayerOnScriptFinishedOrAborted' ./modules/world/...
```

Expected: PASS for all 4 tests.

Then run the full module to confirm no regression:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS for all existing tests.

- [ ] **Step 5: Commit**

```
git add modules/world/player_script_test.go modules/world/player_script.go
git commit --no-gpg-sign -m "feat(world): NAI-54 T1 — (*Player).OnScriptFinishedOrAborted"
```

---

## Task 2: `(*Npc).OnScriptFinishedOrAborted` matrix

**Files:**
- Test: `modules/world/npc_script_test.go` (append).
- Modify: `modules/world/npc.go` (append after `ClearActiveScript` at line 226).

**TS reference:** `Engine-TS/src/engine/entity/Npc.ts:226-228`.

**Test fixture cases (2):** match → null; mismatch → preserved.

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_script_test.go`:

```go
// TestNpcOnScriptFinishedOrAborted_Match pins the npc-path Finished/Aborted
// tail where state matches activeScript: activeScript is nulled.
// Mirrors TS Npc.ts:226-228. NAI-54 T2.
func TestNpcOnScriptFinishedOrAborted_Match(t *testing.T) {
	n := &Npc{}
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "match"}}
	n.activeScript = state

	n.OnScriptFinishedOrAborted(state)

	if n.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (match must clear)")
	}
}

// TestNpcOnScriptFinishedOrAborted_Mismatch pins the guard: when state
// is NOT n.activeScript, activeScript is preserved. Closes the silent
// NpcSuspended-clobber bug symmetric to the player path. NAI-54 T2.
func TestNpcOnScriptFinishedOrAborted_Mismatch(t *testing.T) {
	n := &Npc{}
	stored := &script.ScriptState{Script: &script.ScriptFile{Name: "stored"}}
	other := &script.ScriptState{Script: &script.ScriptFile{Name: "other"}}
	n.activeScript = stored

	n.OnScriptFinishedOrAborted(other)

	if n.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p", n.activeScript, stored)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcOnScriptFinishedOrAborted' ./modules/world/...
```

Expected: build fails with `n.OnScriptFinishedOrAborted undefined`.

- [ ] **Step 3: Implement the minimal code to make tests pass**

Append to `modules/world/npc.go` immediately after `ClearActiveScript` (currently ends at line 226):

```go
// OnScriptFinishedOrAborted handles the Finished/Aborted post-Execute
// tail for an npc-anchored script. If state matches the npc's
// activeScript, nulls it; otherwise no-op. Mirrors TS
// Npc.executeScript tail (Npc.ts:226-228). The match-guard preserves
// an NpcSuspended-stored activeScript when a different fresh script
// Finishes on the same npc in the same tick. NPCs have no modals.
//
// NAI-54 T2.
func (n *Npc) OnScriptFinishedOrAborted(state *script.ScriptState) {
	if n.activeScript != state {
		return
	}
	n.activeScript = nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcOnScriptFinishedOrAborted' ./modules/world/...
```

Expected: PASS for both tests.

Full module:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add modules/world/npc_script_test.go modules/world/npc.go
git commit --no-gpg-sign -m "feat(world): NAI-54 T2 — (*Npc).OnScriptFinishedOrAborted"
```

---

## Task 3: Interface plumbing + mock updates + call-site swaps + integration tests

**Files:**
- Modify: `pkg/script/active.go` (add interface method to ActivePlayer near line 40; to ActiveNpc near line 530).
- Modify: `pkg/script/runner_test.go` (`mockPlayer` impl after line 320).
- Modify: `pkg/script/handlers_player_test.go` (`mockActiveNpc` impl after line 44).
- Modify: `pkg/script/handlers_npc_test.go` (`mockNpc` impl after line 278).
- Modify: `modules/world/script.go:113-114` (swap call).
- Modify: `modules/world/npc_script.go:304-305` (swap call).
- Test: `modules/world/script_test.go` (append integration test).
- Test: `modules/world/npc_script_test.go` (append integration test).

**Why bundled:** Adding the method to the interface requires every concrete impl to satisfy it AT THE SAME COMMIT, otherwise the build breaks. T1 and T2 already added `OnScriptFinishedOrAborted` to `*Player` and `*Npc`, so adding it to the interfaces here only requires updating the three test mocks. Call-site swaps go in the same commit because they only become safe to run once the interface gains the method.

**Pre-flight grep** (run first; per `controller_preflight.md` and `enumerate_all_sites.md`):

```
rg -n 'func \([a-z]+ \*?mock' pkg/script/runner_test.go pkg/script/handlers_player_test.go pkg/script/handlers_npc_test.go | rg 'StoreActiveScript|ClearActiveScript'
```

Expected output (verifies the 3 mock locations):
```
pkg/script/runner_test.go:319:func (m *mockPlayer) StoreActiveScript(s *ScriptState) { m.stored = s }
pkg/script/runner_test.go:320:func (m *mockPlayer) ClearActiveScript()               { m.cleared++ }
pkg/script/handlers_player_test.go:43:func (m *mockActiveNpc) StoreActiveScript(_ *ScriptState)                      {}
pkg/script/handlers_player_test.go:44:func (m *mockActiveNpc) ClearActiveScript()                                    {}
pkg/script/handlers_npc_test.go:277:func (m *mockNpc) StoreActiveScript(_ *ScriptState) {}
pkg/script/handlers_npc_test.go:278:func (m *mockNpc) ClearActiveScript()               {}
```

If the line numbers have shifted, update the per-step Edit anchors before applying.

- [ ] **Step 1: Add the interface method to `ActivePlayer`**

Edit `pkg/script/active.go`. Find the existing `ClearActiveScript()` declaration in `ActivePlayer` (around line 38-40):

```go
	// ClearActiveScript discards any stored ScriptState. Called after
	// Finished/Aborted runs and on logout/cleanup.
	ClearActiveScript()
```

Insert AFTER it (still inside `ActivePlayer`, before the next field):

```go

	// OnScriptFinishedOrAborted is the post-Execute tail for the
	// Finished or Aborted execution states. If state matches the
	// player's currently stored activeScript, nulls activeScript;
	// additionally calls CloseModal(false) when no MAIN modal is
	// open. Mirrors TS Player.executeScript tail (Player.ts:2143-2148).
	// Player-only modal clause; the symmetric ActiveNpc method has no
	// modal handling.
	//
	// NAI-54 closure of NAI-53-F1.
	OnScriptFinishedOrAborted(state *ScriptState)
```

- [ ] **Step 2: Add the interface method to `ActiveNpc`**

Find the existing `ClearActiveScript()` in `ActiveNpc` (around line 528-530):

```go
	// ClearActiveScript discards any stored ScriptState. Called after
	// Finished/Aborted runs. Mirrors ActivePlayer.ClearActiveScript.
	ClearActiveScript()
```

Insert AFTER it:

```go

	// OnScriptFinishedOrAborted is the post-Execute tail for the
	// Finished or Aborted execution states. Nulls activeScript only
	// if state matches the npc's currently stored value. Mirrors TS
	// Npc.executeScript tail (Npc.ts:226-228). NPCs have no modals.
	//
	// NAI-54.
	OnScriptFinishedOrAborted(state *ScriptState)
```

- [ ] **Step 3: Verify build now fails (mocks don't satisfy interface)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: PASS (production code already provides the method on `*Player` and `*Npc`).

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected: build FAILS in `pkg/script` test-package compile because `mockPlayer`, `mockActiveNpc`, `mockNpc` are passed to interfaces that now require `OnScriptFinishedOrAborted`. Errors like:
```
*mockPlayer does not implement ActivePlayer (missing OnScriptFinishedOrAborted method)
```

- [ ] **Step 4: Add stub method to `mockPlayer`**

Edit `pkg/script/runner_test.go` immediately after line 320 (`func (m *mockPlayer) ClearActiveScript() { m.cleared++ }`). Insert:

```go
func (m *mockPlayer) OnScriptFinishedOrAborted(_ *ScriptState) {}
```

(No call-recording: no test in `pkg/script/` exercises the new method directly; the production-side tests in `modules/world/` use real `*Player`. Add a simple stub so the mock satisfies the interface. Per `mock_recorder_field_naming_check.md`: confirmed no callers in `pkg/script/*_test.go` need to assert against this method — re-verify with `rg -n 'OnScriptFinishedOrAborted' pkg/script/` after editing; expected: zero matches outside the new stub.)

- [ ] **Step 5: Add stub method to `mockActiveNpc`**

Edit `pkg/script/handlers_player_test.go` immediately after line 44 (`func (m *mockActiveNpc) ClearActiveScript() {}`). Insert:

```go
func (m *mockActiveNpc) OnScriptFinishedOrAborted(_ *ScriptState)              {}
```

(Match the existing alignment of the surrounding stub bodies — column-aligned `{}`. The exact alignment is whatever `gofmt` produces; let `go fmt ./...` adjust if needed.)

- [ ] **Step 6: Add stub method to `mockNpc`**

Edit `pkg/script/handlers_npc_test.go` immediately after line 278 (`func (m *mockNpc) ClearActiveScript() {}`). Insert:

```go
func (m *mockNpc) OnScriptFinishedOrAborted(_ *ScriptState) {}
```

- [ ] **Step 7: Verify the build is green again**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go fmt ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected: PASS for ALL packages. The interface method is now declared and every impl satisfies it. The Finished/Aborted call sites still call `ClearActiveScript()` (unchanged from before this task) so existing tests remain green.

- [ ] **Step 8: Swap the call site in `resumeOrFinish`**

Edit `modules/world/script.go`. Find lines 113-114:

```go
	case script.Finished, script.Aborted:
		self.ClearActiveScript()
```

Replace with:

```go
	case script.Finished, script.Aborted:
		// NAI-54: TS Player.ts:2143-2148 — only nulls activeScript when
		// state matches, and additionally fires CloseModal(false) on
		// no-MAIN-modal. Both behaviors live in OnScriptFinishedOrAborted.
		self.OnScriptFinishedOrAborted(state)
```

- [ ] **Step 9: Swap the call site in `resumeOrFinishNpc`**

Edit `modules/world/npc_script.go`. Find lines 304-305:

```go
	case script.Finished, script.Aborted:
		npc.ClearActiveScript()
```

Replace with:

```go
	case script.Finished, script.Aborted:
		// NAI-54: TS Npc.ts:226-228 — only nulls activeScript when
		// state matches.
		npc.OnScriptFinishedOrAborted(state)
```

- [ ] **Step 10: Add the player-path Suspended-preservation integration test**

Append to `modules/world/script_test.go`:

```go
// TestResumeOrFinish_PreservesUnrelatedSuspendedScript pins the
// NAI-54 Suspended-clobber bug fix end-to-end via resumeOrFinish.
// A fresh script Y that Finished must NOT null an unrelated suspended
// activeScript X already stored on the player. Mirrors TS
// Player.ts:2143 `if (script === this.activeScript)` guard.
func TestResumeOrFinish_PreservesUnrelatedSuspendedScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Pre-seed: an unrelated PauseButton-suspended X stored on the player.
	stored := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "stored-paused"},
		Execution: script.PauseButton,
	}
	p.activeScript = stored

	// Y: a fresh script that returns immediately (Finished after Execute).
	sf := &script.ScriptFile{
		Name: "[fresh,test]",
		Opcodes: []script.Opcode{
			script.OpReturn,
		},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	s.resumeOrFinish(state, p)

	if p.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p (NAI-54 guard: fresh-Y finishing must not null unrelated stored X)",
			p.activeScript, stored)
	}
}
```

(Note: this test imports `io2 "github.com/zsrv/goscape/pkg/io/isaac"`; that import is already present in `script_test.go` as confirmed by `TestResumeOrFinish_WorldSuspended_EnqueuesAndPreservesActiveScript` at line 1199.)

- [ ] **Step 11: Add the npc-path Suspended-preservation integration test**

Append to `modules/world/npc_script_test.go`:

```go
// TestResumeOrFinishNpc_PreservesUnrelatedSuspendedScript pins the
// NAI-54 NpcSuspended-clobber bug fix end-to-end via resumeOrFinishNpc.
// Symmetric to the player-path test. Mirrors TS Npc.ts:226 guard.
func TestResumeOrFinishNpc_PreservesUnrelatedSuspendedScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Pre-seed: an unrelated NpcSuspended X stored on the npc.
	stored := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "stored-suspended"},
		Execution: script.NpcSuspended,
	}
	n.activeScript = stored

	// Y: a fresh npc-bound script that returns immediately.
	sf := &script.ScriptFile{
		Name: "[fresh-npc,test]",
		Opcodes: []script.Opcode{
			script.OpReturn,
		},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	state := s.buildNpcScriptState(sf, n, nil, nil, nil)

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p (NAI-54 guard: fresh-Y finishing must not null unrelated stored X)",
			n.activeScript, stored)
	}
}
```

(`s.buildNpcScriptState(sf, npc, target, intArgs, stringArgs)` is verified to exist at `modules/world/npc_script.go:225` and wires `Provider`, `World`, `Configs`, `Inv`, `Npcs`, `LineValidator` internally. There is no `script.InitNpc`.)

- [ ] **Step 12: Run the full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go fmt ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected: PASS in every package. The Finished/Aborted swaps are now active and exercised by both the new integration tests AND every existing test that traverses the Finished/Aborted arm (e.g. `TestResumeOrFinish_WorldSuspended_EnqueuesAndPreservesActiveScript` for the WorldSuspended arm; `TestProcessPlayerQueueDeliversAllArgs` and others for end-to-end fresh-fire flows).

- [ ] **Step 13: Run the race detector on the world package**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./modules/world/...
```

Expected: PASS, no data races.

- [ ] **Step 14: Commit**

```
git add pkg/script/active.go pkg/script/runner_test.go pkg/script/handlers_player_test.go pkg/script/handlers_npc_test.go modules/world/script.go modules/world/npc_script.go modules/world/script_test.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "feat(world): NAI-54 T3 — interface OnScriptFinishedOrAborted + call-site swaps"
```

---

## Task 4: NAI-53-F2 combined-fixture test

**Files:**
- Test: `modules/world/modal_close_test.go` (append).

**Goal:** Pin in a single fixture both (a) the COUNTDIALOG/PAUSEBUTTON `activeScript`-null branch (NAI-53 T5) and (b) per-slot `IF_CLOSE` dispatch (NAI-53 T5). The current modal_close tests cover each branch independently; their interaction is unverified.

**Reference fixture:** `TestCloseModalIfCloseDispatchMain` at `modules/world/modal_close_test.go:297-325` — server + scriptProvider + registered IF_CLOSE script. Combine with the activeScript-state shape from `TestCloseModalNullsActiveScriptOnPauseButton` at `:257-271`.

- [ ] **Step 1: Write the test**

Append to `modules/world/modal_close_test.go`:

```go
// TestCloseModalCombinedPauseButtonNullAndPerSlotDispatch pins the
// interaction of two NAI-53 T5 branches in a single fixture:
//   (a) PAUSEBUTTON-suspended activeScript is nulled (NAI-52-F1 closure
//       branch, modal_close_test.go:257-271 covers in isolation).
//   (b) Per-slot IF_CLOSE dispatch fires for the open chat slot (T5
//       per-slot trigger-script port, modal_close_test.go:329-352
//       covers in isolation).
//
// NAI-53 T5's quality review surfaced this as a coverage gap: the null
// tests use newTestPlayer without a server, and the dispatch tests use
// fresh ScriptStates left at zero-value Running execution. This test
// puts them in the same fixture. NAI-54 T4 (closes NAI-53-F2).
func TestCloseModalCombinedPauseButtonNullAndPerSlotDispatch(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,77]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 77),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateChat
	p.modalChat = 77
	pausedState := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "paused-dialog"},
		Execution: script.PauseButton,
	}
	p.activeScript = pausedState

	p.CloseModal(true)

	// (a) PauseButton-state activeScript was nulled.
	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (PauseButton must be cleared)")
	}
	// (b) Per-slot dispatch fired (slot reset, modalState cleared,
	// refreshModalClose set; the dispatched IF_CLOSE script is OpReturn
	// so it Finishes immediately and does not re-store activeScript).
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (slot reset)", p.modalChat)
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want %#x (NONE)", p.modalState, modalStateNone)
	}
	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true")
	}
}
```

- [ ] **Step 2: Run the test**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestCloseModalCombinedPauseButtonNullAndPerSlotDispatch' ./modules/world/...
```

Expected: PASS.

(This test is not RED-then-GREEN — it's a coverage-gap pin that exercises already-shipped behavior. If it fails, that means the existing CloseModal body has a bug under the combined input shape; investigate before touching CloseModal itself.)

- [ ] **Step 3: Run the full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```
git add modules/world/modal_close_test.go
git commit --no-gpg-sign -m "test(world): NAI-54 T4 — combined PauseButton-null + IF_CLOSE dispatch fixture (closes NAI-53-F2)"
```

---

## Task 5: Close commit

**Files:** none (metadata-only commit).

- [ ] **Step 1: Verify the working tree is clean and the suite is green**

```
git status
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./modules/world/...
```

Expected: clean working tree (after T4); all tests PASS; no data races.

- [ ] **Step 2: Verify the spec's claimed behavioral deltas hold**

Spot-check the wire-observable matrix from the spec via dedicated greps:

```
# Match-no-MAIN: activeScript nulled, CloseModal fires.
go test -run 'TestPlayerOnScriptFinishedOrAborted_MatchNoMain' ./modules/world/... -v

# Match-with-MAIN: activeScript nulled, CloseModal NOT fires.
go test -run 'TestPlayerOnScriptFinishedOrAborted_MatchWithMain' ./modules/world/... -v

# Mismatch (player + npc): preserves stored activeScript.
go test -run 'TestPlayerOnScriptFinishedOrAborted_Mismatch' ./modules/world/... -v
go test -run 'TestNpcOnScriptFinishedOrAborted_Mismatch' ./modules/world/... -v

# Integration: fresh fire-and-finish preserves stored Suspended X.
go test -run 'TestResumeOrFinish_PreservesUnrelatedSuspendedScript' ./modules/world/... -v
go test -run 'TestResumeOrFinishNpc_PreservesUnrelatedSuspendedScript' ./modules/world/... -v

# F2 combined fixture.
go test -run 'TestCloseModalCombinedPauseButtonNullAndPerSlotDispatch' ./modules/world/... -v
```

Expected: every command PASS.

- [ ] **Step 3: Compose the close commit**

Per `close_commit_memory_trailer.md`, include a `Closes memory:` trailer naming the memory entries that drove this sub-spec. Per the `nai_followups.md` entry for NAI-54, document: F1 + F2 closed, no new deviations, deviation tally 21 → 21.

```
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-54 — executeScript Finished/Aborted tail port; closes NAI-53-F1, NAI-53-F2

Ports TS Player.ts:2143-2148 (script === activeScript guard +
gated CloseModal(false) on no-MAIN-modal) and TS Npc.ts:226-228
(script === activeScript guard) into goscape's resumeOrFinish /
resumeOrFinishNpc via new OnScriptFinishedOrAborted interface
method.

Tasks:
- T1 *Player.OnScriptFinishedOrAborted (4-case matrix).
- T2 *Npc.OnScriptFinishedOrAborted (2-case matrix).
- T3 interface plumbing + 3 mock updates + call-site swaps +
  Suspended-preservation integration tests (player + npc).
- T4 NAI-53-F2 combined fixture (PauseButton-null + IF_CLOSE dispatch).
- T5 close.

Wire-observable deltas:
- Fresh fire-and-finish no longer wipes existing Suspended /
  PauseButton / CountDialog activeScript on either player or npc.
- Chat dialogue no longer lingers after a script Finished when no
  MAIN modal is open (closes NAI-53-F1).

Deviations: 21 → 21 (no change).

Closes memory: audit_full_method_against_ts.md, true_to_ts_gate.md,
ts_asymmetry_dual_pin.md, enumerate_all_sites.md,
mock_recorder_field_naming_check.md, controller_preflight.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Update `nai_followups.md` (memory)**

Open `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` and append a new `## NAI-54 — CLOSED <date>` block following the NAI-53 template (around line 2732). Include: scope, cadence (full sub-spec, single bundle, 5 tasks), close commit SHA, follow-ups closed (NAI-53-F1, NAI-53-F2), deviations opened (none) / closed (none), net deviation tally (21 → 21 = 0), follow-up candidates (none anticipated; the underlying surface is now fully ported).

The implementer should perform this update as part of T5 (memory write) before declaring the sub-spec closed.

---

## Self-review

**Spec coverage:**
- Interface additions on both `ActivePlayer` and `ActiveNpc` — T3 Steps 1-2.
- `(*Player).OnScriptFinishedOrAborted` impl — T1 Step 3.
- `(*Npc).OnScriptFinishedOrAborted` impl — T2 Step 3.
- Player 4-case matrix (`ts_asymmetry_dual_pin.md`) — T1 Step 1.
- Npc 2-case matrix — T2 Step 1.
- Call-site swap in `resumeOrFinish` — T3 Step 8.
- Call-site swap in `resumeOrFinishNpc` — T3 Step 9.
- Three mock updates (`mockPlayer`, `mockActiveNpc`, `mockNpc`) — T3 Steps 4-6.
- Integration tests via `resumeOrFinish` and `resumeOrFinishNpc` — T3 Steps 10-11.
- NAI-53-F2 combined-fixture — T4 Step 1.
- Deviation tally bookkeeping + close trailer — T5 Steps 3-4.

**Placeholder scan:** none. Every step has concrete code, exact file paths, and runnable commands.

**Type consistency:** method name `OnScriptFinishedOrAborted` used identically in interface declarations, *Player impl, *Npc impl, all mocks, all call sites, and all tests. Parameter type `*ScriptState` (in `pkg/script` interface context) and `*script.ScriptState` (in `modules/world` impl context) are the same type via the `script` import.

**Risks identified:**
- T3's interface ordering: production `*Player` and `*Npc` already implement the method (from T1, T2). Mocks must update in the SAME commit as the interface change to keep the build green; the per-step ordering enforces this (Steps 1-2 add interface, Step 3 verifies the expected mock-fail signature, Steps 4-6 fix mocks, Step 7 confirms green).
- The `match-no-MAIN` test does NOT bind a Server. The CloseModal body's per-slot dispatch path checks `p.client != nil && p.client.server != nil`; `newTestPlayer` returns `p.client != nil` but `p.client.server == nil`, so CloseModal takes the `else` branch at `player_script.go:639-643` (just resets slots). The slot-reset-and-refreshModalClose-set assertions still hold in this branch. Verified against `TestCloseModalNonNoneResetsAllSlots` at `modal_close_test.go:182-207` which uses the same fixture shape.

---

## Execution

Per `execution_mode_default.md`: dispatch via `superpowers:subagent-driven-development`. Skip the user mode-choice prompt.

Per `superpowers_clear_between_spec_and_impl.md`: the controller emits a resume prompt and stops here so the user can `/clear` before kicking off the implementation session.
