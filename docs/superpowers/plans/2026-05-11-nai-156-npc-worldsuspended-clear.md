# NAI-156 — NPC-path WorldSuspended ClearActiveScript (compressed spec+plan)

**Cadence:** compressed (≤~15 LOC behavioral change + 2 test inversions). One Sonnet implementer. No formal code review.

**Tech stack:** Go 1.26+, project root `/home/owner/Code/github.com/zsrv/goscape`.

## 1. Motivation

NAI-155 (commit `94a7615`) retired the **player-path** NAI-44 retention at
`modules/world/script.go`'s `resumeOrFinish` WorldSuspended arm by inserting
`self.ClearActiveScript()` before the `EnqueueWorldScript` call. That fix
unblocked `CanAccess` during world-queue waits.

The **NPC analog** at `modules/world/npc_script.go:387 (resumeOrFinishNpc)`
still retains `n.activeScript` past the WORLD_SUSPENDED transition. This is
not currently load-bearing — NPCs have no `CanAccess`-equivalent gate, so
the retention has no observable effect — but the asymmetry is a TS-fidelity
divergence worth retiring for uniformity.

**TS reference:** `Engine-TS/.../Npc.ts:219-220` (WORLD_SUSPENDED arm) does
NOT assign `script.activeNpc.activeScript`. Only `Npc.ts:227-228` (the
NPC_SUSPENDED branch via `executeScript` tail) assigns
`activeNpc.activeScript = script`. By symmetry with the player path, the
WorldSuspended arm of `resumeOrFinishNpc` should clear, not preserve.

## 2. Risk

LOW. NPCs have no protect-boolean / `CanAccess` analog; clearing
`n.activeScript` at WorldSuspended has no behavioral effect on production
NPC paths. Resume is guarded by `Execution==Suspended` (not WorldSuspended),
so a nil `activeScript` produces no false-resume. This is a TS-fidelity
parity patch, not a bug fix.

## 3. Scope

### 3.1 Production patch — `modules/world/npc_script.go:403-410`

Replace the existing WorldSuspended arm:

```go
case script.WorldSuspended:
    // NAI-37: npc-bound script suspended to world queue. Symmetric
    // to resumeOrFinish (player path). Mirrors TS Npc.ts:226-228.
    //
    // NAI-44: TS Npc.executeScript (L226-228) only nulls activeScript
    // on FINISHED/ABORTED. Same logic as the player-path: holding
    // the pointer is safe because Npc.turn() does not re-fire
    // WorldSuspended states.
    delay := state.PopInt()
    s.EnqueueWorldScript(state, delay)
```

with:

```go
case script.WorldSuspended:
    // NAI-37 / NAI-156: npc-bound script suspended to world queue.
    // Symmetric to resumeOrFinish (player path, NAI-155).
    //
    // Clear n.activeScript BEFORE enqueue. TS Npc.executeScript
    // (Npc.ts:219-220) does NOT assign script.activeNpc.activeScript
    // in the WORLD_SUSPENDED arm — only NPC_SUSPENDED (L227-228) does.
    // Retires NAI-44 retention rationale on the NPC path for
    // TS-fidelity uniformity with the player path. The resume gate
    // (tick.go processActiveScripts) is doubly guarded
    // (non-nil AND Execution==Suspended), so a nil activeScript
    // produces no false-resume. Retiring NAI-155-D-NPC-RESUMEORFINISHNPC-
    // WORLDSUSPENDED-HOLD.
    npc.ClearActiveScript()
    delay := state.PopInt()
    s.EnqueueWorldScript(state, delay)
```

Note: the parameter name is `npc` (interface `script.ActiveNpc`), not
`self`. Confirmed at `npc_script.go:387`. `ClearActiveScript()` is on the
`ActiveNpc` interface (`pkg/script/active.go:760`) and is already called
in the `default:` arm at `npc_script.go:418`, proving the call shape.

### 3.2 Test inversion 1 — `modules/world/npc_script_test.go:910-952`

`TestResumeOrFinishNpc_WorldSuspended_EnqueuesAndPreservesActiveScript`:

- Rename: `…EnqueuesAndPreservesActiveScript` → `…EnqueuesAndClearsActiveScript`.
- Update header doc comment: the test now pins the post-NAI-156 behavior —
  WorldSuspended dispatch (a) pops wakeup-tick, (b) enqueues, (c) **clears**
  `n.activeScript` (mirrors TS `Npc.ts:219-220` non-assignment, symmetric to
  player path NAI-155).
- Replace the L945-951 cascade comment + assertion:
  - Old: comment cites "NAI-44 T1 cascade: post-T1 the WorldSuspended arm
    no longer calls ClearActiveScript()" and asserts `got != state` is
    error.
  - New: comment cites "NAI-156: the WorldSuspended arm now calls
    ClearActiveScript() to mirror TS Npc.ts:219-220 (no assignment in
    WORLD_SUSPENDED arm). Symmetric to player-path NAI-155 fix at
    script.go:148."
  - New assertion:
    ```go
    if got := n.activeScript; got != nil {
        t.Errorf("npc.activeScript: got %p, want nil (WorldSuspended must clear; NAI-156)", got)
    }
    ```

### 3.3 Test inversion 2 — `modules/world/npc_script_test.go:985-1010`

`TestResumeOrFinishNpcWorldSuspendedDoesNotClearActiveScript`:

- Rename: `…DoesNotClearActiveScript` → `…ClearsActiveScript`.
- Update header doc comment: cite NAI-156 instead of NAI-44 T1; TS
  `Npc.ts:219-220` non-assignment in WORLD_SUSPENDED arm is the rationale.
- Replace L1004-1006 assertion:
  - Old: `if n.activeScript != state { t.Errorf(..."WorldSuspended must NOT clear"...) }`
  - New:
    ```go
    if n.activeScript != nil {
        t.Errorf("activeScript: got %p, want nil (WorldSuspended must clear; NAI-156)", n.activeScript)
    }
    ```
- Keep the existing `len(s.worldScriptQueue) != 1` assertion at L1007-1009
  — enqueue contract unchanged.

### 3.4 Scope explicitly excluded

- No new "fresh pin" test — the two inverted tests fully cover the
  post-NAI-156 contract (Test 3.2 verifies enqueue+clear together, Test 3.3
  is the redundant symmetric NAI-44-style pin which now flips polarity).
- No changes to `resumeOrFinish` (player path; already fixed NAI-155).
- No changes to default-arm behavior at L416-420.
- No changes to `Finished`/`Aborted`/`NpcSuspended` arms.

## 4. Verification

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1`
   — both inverted tests pass; no other test fails.
2. Reverify player-side NAI-155 non-regression: same command (player-side
   fixtures in `worldsuspended_pap_test.go` and `script_test.go` are part
   of the `modules/world` package).
3. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...` —
   clean.

## 5. Risk register

| Premise | Verification |
|---|---|
| `npc.ClearActiveScript()` compiles on the `ActiveNpc` interface receiver | ✅ pre-flight: `active.go:760` declares it on interface; `npc_script.go:418` already calls `npc.ClearActiveScript()` in default arm |
| No production-side `n.activeScript` reader depends on retention past WorldSuspended | ✅ pre-flight grep: only test-file readers; production reads are in resume path (tick.go) gated on `Execution==Suspended` |
| Player-side NAI-155 fix landed at HEAD | ✅ confirmed at commit `94a7615`; `script.go:148` has `self.ClearActiveScript()` |

## 6. Memory follow-up

- Update `nai_followups.md`: mark
  `NAI-155-D-NPC-RESUMEORFINISHNPC-WORLDSUSPENDED-HOLD` as closed (cite
  NAI-156 commit).
- No new memory entry needed — this is symmetric closure of a known
  follow-up; behavioral pattern already memorialized in
  `ts_protect_independent_from_activescript.md`.

## 7. Dispatch

One Sonnet implementer. Tasks (sequential, single agent):

- T1: Patch `modules/world/npc_script.go:403-410` per §3.1.
- T2: Invert test at `modules/world/npc_script_test.go:910-952` per §3.2.
- T3: Invert test at `modules/world/npc_script_test.go:985-1010` per §3.3.
- T4: Run §4 verification commands; report PASS/FAIL.
