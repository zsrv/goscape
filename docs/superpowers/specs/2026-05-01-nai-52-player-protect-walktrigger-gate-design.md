# NAI-52: Player.protect convergence — apply existing rule to processWalktrigger

> **Cadence:** Compressed (`compressed_cadence.md`). Single combined
> spec+plan doc, ≤~15 source LOC, no formal whole-impl review. Inline
> TDD.

## Goal

Close `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK` by applying
goscape's already-documented `TS Player.protect ↔ activeScript.Protect`
convergence to `(*Player).processWalktrigger`. Extract a small predicate
helper so the convergence rule is grep-discoverable for future
walktrigger-adjacent ports (closeModal divergence audit, future TS
`!this.protect` reads).

## Context: the deviation was misframed

The NAI-51 deviation comment at `modules/world/interaction.go:240`
asserts:

> "TS L1062 also gates on !this.protect. Player has no boolean protect
> field; the anim-protect block (player.go:166) is a separate concern."

This is wrong. Goscape **already documented** the
`TS Player.protect ↔ goscape (p.activeScript != nil &&
p.activeScript.Protect)` convergence at `player_script.go:232-238`
(`(*Player).CanAccess` doc-comment):

> "TS expresses as a single Player.protect bool from activeScript.Protect.
> They are equivalent: TS persists the flag onto the player at script
> suspension (Player.ts:2141) and clears it at script completion
> (:2103-2114), so 'is the player in a stored protected script?' and 'is
> the player-level protect flag set?' are the same condition — goscape
> just reads it from the stored state instead of a redundant bool
> field."

`(*Player).CanAccess` already gates on this. `processWalktrigger`
simply forgot to apply it. The fix is a one-clause extension, not an
architectural decision.

`Player.animProtect` is **a separate field** from `this.protect` —
it's the S7b `P_ANIMPROTECT`-set anim-suppression flag read by the
unported `playAnimation()` at TS `Player.ts:1842`. **Out of scope** for
NAI-52; tracked-deferred as `S7b-D1` in `nai_followups.md`, picked up
by the future anim-playback sub-spec.

## TS `this.protect` site enumeration (Player/Npc only)

Excludes `VarPlayerType.protect` / `InvType.protect` config-field
references — those are unrelated `protect` properties on config types.

| TS site | Role | goscape status |
|---|---|---|
| `Player.ts:460` (`resetEntity`) | Reset `protect = false` on respawn | Implicit: `tick.go:169` nulls `activeScript` on logout; respawn path nulls activeScript via downstream cleanup. **No gap.** |
| `Player.ts:746` (`closeModal` not-delayed branch) | Force-clear `protect = false` | Goscape `CloseModal` (`player_script.go:563`) does NOT null `activeScript` on countdialog/pausebutton (TS does at L763-766). **Separate divergence**, tracked as `NAI-52-F1` follow-up below; not in scope for NAI-52. |
| `Player.ts:810` (`canAccess`) | `!this.protect && !this.busy()` | ✅ ported — `CanAccess()` reads convergence. |
| `Player.ts:1062` (`processWalktrigger`) | `!this.protect && !this.delayed` gate | ❌ **missing** — `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK`. **Closed by this sub-spec.** |
| `Player.ts:2095, 2103, 2109` (Player.runScript set/clear) | TS-side bookkeeping for the convergence | Goscape's `resumeOrFinish` (`script.go:105-138`) drives the convergence by managing `activeScript` directly. **No gap.** |
| `Player.ts:2141` (Player.executeScript suspend persist) | Persist `protect = protect` on suspend | Goscape persists via `StoreActiveScript` keeping `state.Protect` intact. **No gap.** |
| `Npc.ts:231, 236` (Npc.runScript clear `_activePlayer.protect`) | Clear a secondary-bound player's protect post-run | Naturally a no-op in goscape: secondary `_activePlayer` bindings never set the player's own `activeScript`, so there's no protect-equivalent state to clear. **No gap.** |

## Convergence rule (codified)

A new private helper on `*Player`:

```go
// protectedScriptActive reports whether the player currently owns a
// suspended protected script — goscape's mapping of TS Player.protect.
// Used by CanAccess and processWalktrigger to gate operations that TS
// guards with !this.protect. See the CanAccess doc-comment for the
// activeScript.Protect ↔ TS Player.protect equivalence rationale.
func (p *Player) protectedScriptActive() bool {
	return p.activeScript != nil && p.activeScript.Protect
}
```

`CanAccess` is refactored to call this helper. `processWalktrigger`
gains `|| p.protectedScriptActive()` in its bail-out predicate.

## Test plan

Three new tests in `modules/world/interaction_test.go`, paralleling the
existing `TestProcessWalktrigger_DelayedNoOp` shape:

1. `TestProcessWalktrigger_ProtectedScriptActiveNoOp` — `walktrigger=7`,
   `activeScript=&ScriptState{Protect: true}`, `delayed=false`. Assert
   field unchanged at 7 (TS L1062 `!this.protect` gate).
2. `TestProcessWalktrigger_ActiveScriptUnprotectedFires` — `walktrigger=42`,
   `activeScript=&ScriptState{Protect: false}` + script registered at
   slot 42. Assert script fires (sayText / mes), `walktrigger=-1`.
   Pins that `activeScript != nil` alone is NOT the gate; the `Protect`
   sub-field is.
3. `TestProcessWalktrigger_NilActiveScriptFires` — `walktrigger=42`,
   `activeScript=nil` + script registered at 42. Assert script fires.
   Pins the `activeScript != nil` short-circuit.

One direct unit test on the helper in `modules/world/player_test.go`:

4. `TestPlayer_ProtectedScriptActive_TruthTable` — table-driven over
   `(activeScript=nil)` / `(activeScript=&{Protect:false})` /
   `(activeScript=&{Protect:true})`. Assert `false / false / true`.

`(*Player).CanAccess` already has full coverage; refactoring the
internal expression to call the helper is behavior-preserving and
shouldn't need new tests there. Run the existing CanAccess suite as
regression.

## Tracked deviations

**Closed:**
- `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK`

**Introduced:** none.

**New follow-ups (tracked, not closed here):**
- **NAI-52-F1** — `(*Player).CloseModal` does NOT null `activeScript`
  when the active execution is `CountDialog` or `PauseButton`; TS
  `Player.ts:763-766` does. Side-effect: a player closing a dialog modal
  manually (CLOSE_MODAL packet) does not get their suspended-protected
  script cleared, leaving them stuck in the protected state from
  `protectedScriptActive`'s perspective. **Closure:** future modal-close
  fidelity sub-spec.

## Net deviation tally

21 → 20 (one closed, none introduced).

---

# Plan

Subagent-driven-development per `execution_mode_default.md`. Single
bundle, four tasks. Each task is its own commit. No bundle-close commit
needed — close-trailer in T4's commit message per
`close_commit_memory_trailer.md`.

## Task 1: Add `(*Player).protectedScriptActive` helper

**Files:**
- Modify: `modules/world/player_script.go` (append after `CanAccess` at
  line 250)
- Test: `modules/world/player_test.go` (append in the CanAccess test
  cluster around line 720, or wherever `TestPlayerCanAccess*` tests live;
  fall through to end-of-file otherwise)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/player_test.go`:

```go
// TestPlayer_ProtectedScriptActive_TruthTable pins the goscape mapping
// of TS Player.protect: protectedScriptActive iff activeScript != nil
// AND activeScript.Protect. Mirrors the convergence documented at
// CanAccess (player_script.go:232-238).
func TestPlayer_ProtectedScriptActive_TruthTable(t *testing.T) {
	cases := []struct {
		name   string
		active *script.ScriptState
		want   bool
	}{
		{"nil-active", nil, false},
		{"active-unprotected", &script.ScriptState{Protect: false}, false},
		{"active-protected", &script.ScriptState{Protect: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{activeScript: tc.active}
			if got := p.protectedScriptActive(); got != tc.want {
				t.Errorf("protectedScriptActive: got %v, want %v", got, tc.want)
			}
		})
	}
}
```

Verify the test file imports `"github.com/zsrv/goscape/pkg/script"`; if
absent, add it.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_ProtectedScriptActive_TruthTable -v`
Expected: FAIL — `protectedScriptActive undefined`.

- [ ] **Step 3: Add the helper**

In `modules/world/player_script.go`, append immediately after the
closing brace of `CanAccess` at line 250:

```go
// protectedScriptActive reports whether the player currently owns a
// suspended protected script — goscape's mapping of TS Player.protect.
// Used by CanAccess (above) and processWalktrigger to gate operations
// that TS guards with !this.protect. See the CanAccess doc-comment for
// the activeScript.Protect ↔ TS Player.protect equivalence rationale.
// NAI-52.
func (p *Player) protectedScriptActive() bool {
	return p.activeScript != nil && p.activeScript.Protect
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_ProtectedScriptActive_TruthTable -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_script.go modules/world/player_test.go
git commit --no-gpg-sign -m "feat(world): NAI-52 T1 — add (*Player).protectedScriptActive helper"
```

---

## Task 2: Refactor `CanAccess` to use the helper

**Files:**
- Modify: `modules/world/player_script.go` (`CanAccess` body at line
  239-250)

Behavior-preserving refactor. The existing `TestPlayerCanAccess*` suite
is the regression gate.

- [ ] **Step 1: Verify the existing CanAccess suite is green at HEAD**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerCanAccess -v`
Expected: PASS.

- [ ] **Step 2: Replace the third branch with the helper call**

In `modules/world/player_script.go`, locate `CanAccess` at line 239.
Replace the body block:

```go
	if p.activeScript != nil && p.activeScript.Protect {
		return false
	}
```

with:

```go
	if p.protectedScriptActive() {
		return false
	}
```

- [ ] **Step 3: Re-run the CanAccess suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerCanAccess -v`
Expected: PASS — all pre-existing cases still green.

- [ ] **Step 4: Run the full world-package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_script.go
git commit --no-gpg-sign -m "refactor(world): NAI-52 T2 — CanAccess uses protectedScriptActive helper"
```

---

## Task 3: Apply the gate to `processWalktrigger`

**Files:**
- Modify: `modules/world/interaction.go` (lines 232-256: replace the
  `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK` deviation comment
  block + extend the bail-out predicate)
- Test: `modules/world/interaction_test.go` (append after the existing
  `TestProcessWalktrigger_*` cluster from NAI-51 T1.7)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/interaction_test.go`. (Existing imports cover
all three tests; verify `script` and `io2` are present in the import
block.)

```go
// TestProcessWalktrigger_ProtectedScriptActiveNoOp — NAI-52. With a
// suspended protected script anchored on the player, the walktrigger
// consumer must bail without firing. Mirrors TS Player.ts:1062 gate
// !this.protect via goscape's activeScript.Protect convergence.
func TestProcessWalktrigger_ProtectedScriptActiveNoOp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	p.walktrigger = 7
	p.activeScript = &script.ScriptState{Protect: true}

	p.processWalktrigger()

	if p.walktrigger != 7 {
		t.Errorf("walktrigger after protected-bail: got %d, want 7 (unchanged)", p.walktrigger)
	}
}

// TestProcessWalktrigger_ActiveScriptUnprotectedFires — NAI-52. Pins
// that activeScript != nil alone does NOT block the consumer; only
// activeScript.Protect == true does. activeScript with Protect=false
// must allow the walktrigger to fire and clear.
func TestProcessWalktrigger_ActiveScriptUnprotectedFires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-unprot", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(42, sf)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)

	p.walktrigger = 42
	p.activeScript = &script.ScriptState{Protect: false}

	p.processWalktrigger()
	p.client.flushWrite()
	pkt := <-received

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after unprotected fire: got %d, want -1", p.walktrigger)
	}
	if !bytes.Contains(pkt, []byte("wt-unprot")) {
		t.Errorf("payload: did not contain wt-unprot: %q", pkt)
	}
}

// TestProcessWalktrigger_NilActiveScriptFires — NAI-52. activeScript=nil
// short-circuit pin: protectedScriptActive returns false on nil
// activeScript, so the consumer must fire.
func TestProcessWalktrigger_NilActiveScriptFires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-nilactive", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(42, sf)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)

	p.walktrigger = 42
	// activeScript is already nil from newPlayer.

	p.processWalktrigger()
	p.client.flushWrite()
	pkt := <-received

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after nil-active fire: got %d, want -1", p.walktrigger)
	}
	if !bytes.Contains(pkt, []byte("wt-nilactive")) {
		t.Errorf("payload: did not contain wt-nilactive: %q", pkt)
	}
}
```

- [ ] **Step 2: Run tests to verify the protected-bail test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessWalktrigger_(ProtectedScriptActiveNoOp|ActiveScriptUnprotectedFires|NilActiveScriptFires)" -v`
Expected: `ProtectedScriptActiveNoOp` FAILS (consumer fires; walktrigger
becomes -1 against the want=7 assertion). The other two tests pass —
they verify post-fix behavior that the current code already exhibits
(activeScript != nil but unprotected: current code doesn't check
activeScript at all, so it fires; nil-active: same).

- [ ] **Step 3: Replace the deviation-tagged comment block + extend the gate**

In `modules/world/interaction.go`, locate the
`processWalktrigger` function block (currently lines ~232-256).
Replace the function (comment block + body) with:

```go
// processWalktrigger is the per-tick walktrigger consumption hook
// invoked by processInteraction's pre-step and post-step arms. Looks up
// the queued script id, clears the field BEFORE the script-found check
// (TS clear-before-check semantics at Player.ts:1064), then dispatches
// via runScript with protect=true. Mirrors TS Player.processWalktrigger
// at Player.ts:1057-1070.
//
// The !p.protectedScriptActive() gate mirrors TS L1062 !this.protect via
// goscape's documented activeScript.Protect convergence (see CanAccess
// doc-comment in player_script.go).
func (p *Player) processWalktrigger() {
	if p.walktrigger == -1 || p.delayed || p.protectedScriptActive() {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	sf := s.scriptProvider.GetByID(uint32(p.walktrigger))
	p.walktrigger = -1
	if sf == nil {
		return
	}
	s.runScript(sf, p, nil, true, nil, nil)
}
```

Note: the only behavioural change is the added `|| p.protectedScriptActive()` clause; everything else is comment swap and unchanged code preserved verbatim.

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessWalktrigger_(ProtectedScriptActiveNoOp|ActiveScriptUnprotectedFires|NilActiveScriptFires)" -v`
Expected: all three PASS.

- [ ] **Step 5: Run the full processWalktrigger + processInteraction test cluster (regression gate)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessWalktrigger|TestProcessInteraction" -v`
Expected: PASS.

- [ ] **Step 6: Run the full world-package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-52 T3 — gate processWalktrigger on protectedScriptActive"
```

---

## Task 4: Retire `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK` (verification + close-trailer)

**Files:**
- (No code changes — the deviation tag was removed inline by Task 3's
  comment swap. This task is a verification + close-trailer commit.)

- [ ] **Step 1: Verify no stale `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK` references remain**

Run: `rg "NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK" pkg/ modules/ cmd/ docs/`
Expected: zero matches in `pkg/`, `modules/`, `cmd/`. Matches in
`docs/superpowers/specs/2026-05-01-nai-52-*.md` (this file) and
`docs/superpowers/plans/2026-05-01-nai-51-walktrigger-consumers.md`
(historical) are expected and acceptable.

If any stale match surfaces in source dirs, edit it out and amend Task
3's commit (or add a fixup commit).

- [ ] **Step 2: Run the full repo suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 3: Verify the helper has the expected two callers**

Run: `rg "protectedScriptActive\(\)" modules/ pkg/`
Expected: exactly two matches in `modules/world/`:
- `player_script.go` (CanAccess body)
- `interaction.go` (processWalktrigger body)
Plus the helper definition itself in `player_script.go`.

- [ ] **Step 4: Close commit (memory trailer)**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-52 — Player.protect convergence applied to processWalktrigger

Closes deviation: NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK.
No deviations introduced.
Net deviation tally: 21 → 20.

New follow-up: NAI-52-F1 (CloseModal does not null activeScript on
CountDialog/PauseButton; TS Player.ts:763-766 does). Closure: future
modal-close fidelity sub-spec.

Closes memory: nai_followups.md
EOF
)"
```

---

## Self-review

**Spec coverage:**
- Helper `(*Player).protectedScriptActive` definition: T1 ✓
- `CanAccess` refactored to call helper: T2 ✓
- `processWalktrigger` gated on helper: T3 ✓
- Retire `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK`: T3 (inline
  comment swap) + T4 (verification) ✓
- Test plan items 1-3 (interaction tests): T3 ✓
- Test plan item 4 (helper unit test): T1 ✓
- Audit-only items (closeModal, respawn, Npc.runScript): documented in
  context table; closeModal divergence filed as NAI-52-F1.

**Type/signature consistency:**
- `protectedScriptActive() bool` is unexported (lowercase). Both callers
  (`CanAccess`, `processWalktrigger`) live in package `world`. No
  `ActivePlayer` interface change required.
- `script.ScriptState` has exported `Protect bool` field
  (`pkg/script/state.go:238`); the helper reads it without indirection.
- Test fixtures construct `&script.ScriptState{Protect: bool}`
  literally; same shape as `runner_test.go:57` and existing
  `server_test.go:615` usage.

**Placeholder scan:** No TBD / TODO / "implement later" / "similar to"
language. Every code step shows full code; every test shows full test
body.

**Deviation-tag consistency:** `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK`
retired in T3 (comment swap) + asserted-absent in T4. No new tags.

**Compressed-cadence justification:** Source-LOC budget — helper (4 LOC
body) + CanAccess refactor (−2 / +2) + processWalktrigger predicate
(+1) + comment-block swap (~6 LOC of comment text) ≈ 13 source LOC. ≤15
threshold. No formal whole-impl review required. Inline TDD per task.
