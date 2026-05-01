# NAI-55 — executeScript error-path tail port

**Date:** 2026-05-01
**Tech stack:** Go 1.26+
**Cadence:** Compressed — combined spec + plan in one doc (per memory `compressed_cadence.md`); production delta ~6 LOC, test delta 4 add + 1 retire.
**Closes follow-ups:** `NAI-54-F1`.

## Problem

In TS, `ScriptRunner.execute(state)` (`ScriptRunner.ts:121-232`) catches every thrown error in its body, sets `state.execution = ScriptState.ABORTED` (L228), and returns it. `Player.executeScript` and `Npc.executeScript` then dispatch on the returned state — an Aborted return falls through into the same `else if (script === this.activeScript)` tail as a clean Finished/Aborted (`Player.ts:2143-2148`, `Npc.ts:226-228`).

Goscape's `pkg/script.Execute` mirrors that contract: every error path in the hot loop sets `s.Execution = Aborted` before returning a non-nil error (`pkg/script/runner.go:54-83`, doc-commented at L46-48).

But goscape's two server-side dispatch sites short-circuit around `OnScriptFinishedOrAborted` on error:

**`modules/world/script.go:106-110`:**
```go
if err := script.Execute(state); err != nil {
    s.log.Warn("script execute error",
        "script", state.Script.Name, "err", err)
    self.ClearActiveScript()
    return
}
```

**`modules/world/npc_script.go:297-301`:**
```go
if err := script.Execute(state); err != nil {
    s.log.Warn("npc script execute error",
        "script", state.Script.Name, "err", err)
    npc.ClearActiveScript()
    return
}
```

Both call `ClearActiveScript()` unconditionally, bypassing the match-guard implemented in `(*Player).OnScriptFinishedOrAborted` (`modules/world/player_script.go:161-169`) and `(*Npc).OnScriptFinishedOrAborted` (`modules/world/npc.go:236-241`) — both of which were ported in NAI-54.

### Wire-observable consequence

A fresh script Y fired on a player or NPC who already holds a Suspended / PauseButton / CountDialog / NpcSuspended `activeScript` X — and which then errors during `script.Execute` (e.g. opcount limit, PC out of range, missing-handler opcode, handler-returned error) — wipes X. TS preserves X because `Y !== this.activeScript`.

This is the same Suspended-clobber shape NAI-54 fixed for the clean-Finished/Aborted case. The error-path divergence predates NAI-54 (NAI-2 added the unconditional clear); NAI-55 closes it.

The player path additionally misses the chat-dialogue auto-close on no-MAIN-modal. NAI-54 ported that into `OnScriptFinishedOrAborted`; NAI-55 wires it onto the error path by reaching the same dispatch arm.

### Out of scope

`(*Server).resumeOrFinishWorld` (`modules/world/script.go:158-199`) also returns early on Execute error, but its non-error Aborted arm is already a silent fall-through with no rebind (TS `World.processWorld` has no Aborted branch — World.ts:530-560). Error-path and clean-Aborted are therefore already wire-equivalent for the world-queue dispatch. No change needed.

## Solution (Approach A: drop early return)

Drop the `self.ClearActiveScript(); return` and `npc.ClearActiveScript(); return` lines on Execute error. Keep the warn. Let the existing `switch state.Execution` route Aborted via `OnScriptFinishedOrAborted` — same path as a clean Aborted. Relies on the documented `script.Execute` invariant that error implies `Execution = Aborted`.

## Production change

### Task P1 — `(*Server).resumeOrFinish` (player path)

**File:** `modules/world/script.go`

Replace lines 106-111:

```go
func (s *Server) resumeOrFinish(state *script.ScriptState, self script.ActivePlayer) {
    if err := script.Execute(state); err != nil {
        s.log.Warn("script execute error",
            "script", state.Script.Name, "err", err)
        self.ClearActiveScript()
        return
    }
    switch state.Execution {
```

with:

```go
func (s *Server) resumeOrFinish(state *script.ScriptState, self script.ActivePlayer) {
    if err := script.Execute(state); err != nil {
        s.log.Warn("script execute error",
            "script", state.Script.Name, "err", err)
        // NAI-55: fall through. script.Execute sets state.Execution =
        // Aborted on every error path (pkg/script/runner.go:54-83),
        // so the switch routes via OnScriptFinishedOrAborted —
        // match-guarded, identical to a clean Aborted. Mirrors TS
        // ScriptRunner.execute setting state.execution = ABORTED on
        // throw (ScriptRunner.ts:228), then Player.executeScript
        // re-entering the (script === this.activeScript) guard
        // (Player.ts:2143-2148). Closes NAI-54-F1.
    }
    switch state.Execution {
```

### Task P2 — `(*Server).resumeOrFinishNpc` (NPC path)

**File:** `modules/world/npc_script.go`

Replace lines 297-302:

```go
func (s *Server) resumeOrFinishNpc(state *script.ScriptState, npc script.ActiveNpc) {
    if err := script.Execute(state); err != nil {
        s.log.Warn("npc script execute error",
            "script", state.Script.Name, "err", err)
        npc.ClearActiveScript()
        return
    }
    switch state.Execution {
```

with:

```go
func (s *Server) resumeOrFinishNpc(state *script.ScriptState, npc script.ActiveNpc) {
    if err := script.Execute(state); err != nil {
        s.log.Warn("npc script execute error",
            "script", state.Script.Name, "err", err)
        // NAI-55: fall through. Symmetric to resumeOrFinish; routes
        // Aborted via (*Npc).OnScriptFinishedOrAborted (NPCs have no
        // modals, so the method is just the match-guard). Mirrors TS
        // Npc.executeScript tail at Npc.ts:226-228.
    }
    switch state.Execution {
```

## Test changes

### Task T1 — Player-path error+mismatch (preserve)

**File:** `modules/world/script_test.go` (append)

Mirror `TestResumeOrFinish_PreservesUnrelatedSuspendedScript` (line 1585) but with a bad-opcode fresh script that errors in `script.Execute`:

```go
// TestResumeOrFinish_ExecuteError_PreservesUnrelatedSuspendedScript pins
// the NAI-55 error-path match-guard: a fresh script Y that errors during
// script.Execute must NOT null an unrelated stored activeScript X on the
// player. Mirrors TS ScriptRunner.execute setting Execution=ABORTED on
// throw (ScriptRunner.ts:228), then Player.executeScript re-entering the
// (script === this.activeScript) guard (Player.ts:2143).
func TestResumeOrFinish_ExecuteError_PreservesUnrelatedSuspendedScript(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

    stored := &script.ScriptState{
        Script:    &script.ScriptFile{Name: "stored-paused"},
        Execution: script.PauseButton,
    }
    p.activeScript = stored

    // Y: bad-opcode script. Execute hits the "no handler" arm at
    // runner.go:69-72, which sets Execution=Aborted and returns the error.
    sf := &script.ScriptFile{
        Name:    "[err,test]",
        Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
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
        t.Errorf("activeScript: got %p, want preserved %p (NAI-55 error-path guard: fresh-Y erroring must not null unrelated stored X)",
            p.activeScript, stored)
    }
}
```

### Task T2 — Player-path error+match (clear + CloseModal)

**File:** `modules/world/script_test.go` (append)

```go
// TestResumeOrFinish_ExecuteError_ClearsMatchingActiveScript pins
// the NAI-55 error-path match arm: when the fresh state IS the player's
// activeScript and Execute errors, activeScript is nulled AND
// CloseModal(false) fires when no MAIN modal is open. Mirrors TS
// Player.ts:2143-2148 reached after ScriptRunner.execute returned ABORTED.
func TestResumeOrFinish_ExecuteError_ClearsMatchingActiveScript(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

    sf := &script.ScriptFile{
        Name:    "[err,match]",
        Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
    }
    state := script.Init(sf, p, false, nil, nil)
    state.Provider = s.scriptProvider
    state.World = s.worldVars
    state.Configs = s.configsView
    state.Inv = s.invLookup
    state.Npcs = s.npcLookup
    state.LineValidator = s.scriptLineValidator()

    p.activeScript = state // match-arm: state IS the player's activeScript
    p.modalState = modalStateChat
    p.modalChat = 100
    p.refreshModalClose = false

    s.resumeOrFinish(state, p)

    if p.activeScript != nil {
        t.Errorf("activeScript: got non-nil, want nil (match-arm must clear on error)")
    }
    if p.modalState != modalStateNone {
        t.Errorf("modalState: got %#x, want %#x (CloseModal(false) must fire on no-MAIN error)",
            p.modalState, modalStateNone)
    }
    if !p.refreshModalClose {
        t.Errorf("refreshModalClose: got false, want true (CloseModal must fire)")
    }
    if p.modalChat != -1 {
        t.Errorf("modalChat: got %d, want -1 (CloseModal must reset slot)", p.modalChat)
    }
}
```

### Task T3 — NPC-path error+mismatch (preserve)

**File:** `modules/world/npc_script_test.go` (append)

Mirror `TestResumeOrFinishNpc_PreservesUnrelatedSuspendedScript` (line 970) with a bad-opcode fresh script:

```go
// TestResumeOrFinishNpc_ExecuteError_PreservesUnrelatedSuspendedScript
// pins the NAI-55 NPC-path error+mismatch match-guard: a fresh script Y
// that errors during script.Execute must NOT null an unrelated stored
// activeScript X on the NPC. Mirrors TS Npc.ts:226 guard reached after
// ScriptRunner.execute returned ABORTED (ScriptRunner.ts:228).
func TestResumeOrFinishNpc_ExecuteError_PreservesUnrelatedSuspendedScript(t *testing.T) {
    s, n := buildNpcForIntegration(t)

    stored := &script.ScriptState{
        Script:    &script.ScriptFile{Name: "stored-npc-suspended"},
        Execution: script.NpcSuspended,
    }
    n.activeScript = stored

    sf := &script.ScriptFile{
        Name:    "[err,npc-test]",
        Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
    }
    errState := script.Init(sf, nil, false, nil, nil)
    errState.ActiveNpc = n

    s.resumeOrFinishNpc(errState, n)

    if n.activeScript != stored {
        t.Errorf("activeScript: got %p, want preserved %p (NAI-55 NPC error-path guard)",
            n.activeScript, stored)
    }
}
```

### Task T4 — NPC-path error+match (clear)

**File:** `modules/world/npc_script_test.go` (append)

```go
// TestResumeOrFinishNpc_ExecuteError_ClearsMatchingActiveScript pins
// the NAI-55 NPC-path error+match arm: when the fresh state IS the NPC's
// activeScript and Execute errors, activeScript is nulled. Mirrors TS
// Npc.ts:226-228 tail reached after ScriptRunner.execute returned ABORTED.
func TestResumeOrFinishNpc_ExecuteError_ClearsMatchingActiveScript(t *testing.T) {
    s, n := buildNpcForIntegration(t)

    sf := &script.ScriptFile{
        Name:    "[err,npc-match]",
        Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
    }
    state := script.Init(sf, nil, false, nil, nil)
    state.ActiveNpc = n
    n.activeScript = state // match-arm

    s.resumeOrFinishNpc(state, n)

    if n.activeScript != nil {
        t.Errorf("activeScript: got non-nil, want nil (NPC match-arm must clear on error)")
    }
}
```

### Task T5 — Retire `TestResumeOrFinishNpcErrorPathClearsScript`

**File:** `modules/world/npc_script_test.go`

Delete lines 365-389 (the NAI-2 test that pins the divergent unconditional clear). The new T3 + T4 subsume it: T4 covers the match-clear semantics correctly (with the match-guard); T3 pins the preserve-on-mismatch behavior the old test would have failed.

The accompanying `TestResumeOrFinishNpcDefaultBranchClearsScript` at line 402 stays — it pins the `default:` arm of the switch (a synthetic non-Running, non-terminal Execution value), which is unrelated to the error path.

## Verification

Run after each task:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected outcome at HEAD: T5 retires a passing-but-divergent test; T1-T4 fail before P1/P2 (red), pass after (green). Run full repo test suite at close:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

## Closure rollup

- **Closes follow-ups:** NAI-54-F1 (error-path match-guard divergence in resumeOrFinish/resumeOrFinishNpc).
- **Deviations opened:** none.
- **Deviations closed:** none.
- **Deviation tally:** 21 → 21 (no change).
- **Observable wire-behavior delta:** A fresh script Y fired on a player/NPC with a stored Suspended / PauseButton / CountDialog / NpcSuspended activeScript X, which then errors during Execute, no longer wipes X — the resume loop will correctly fire X when its delay expires. Player path additionally fires `CloseModal(false)` on the error+match arm when no MAIN modal is open, auto-closing chat / side dialogues — matches TS Player.ts:2146-2148 reached on ABORTED return.

## Resume prompt (for the implementing session)

After this spec is approved, the implementing session should be brought up clean and fed:

> Execute NAI-55 from `docs/superpowers/specs/2026-05-01-nai-55-execute-script-error-tail.md`. Compressed cadence: combined spec+plan, single bundle, 7 tasks (P1, P2, T1-T5). Per memory `execution_mode_default.md` dispatch via `superpowers:subagent-driven-development`. Per `verify_implementer_claims.md` run a fresh `go test ./modules/world/...` after each implementer commit. Per `close_commit_memory_trailer.md` add a `Closes memory:` trailer on the close commit citing this spec path.
