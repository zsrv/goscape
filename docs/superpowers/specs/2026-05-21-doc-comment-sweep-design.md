# Doc-comment sweep — stale "future X" framing — design

**Status:** approved (brainstormed 2026-05-21)
**Predecessor:** [[low-arg-shape-pin-close]] (slice 1 of 4-sequential-mini-slices bundle)
**Scope size:** XS (~1 hour, single commit in-session)
**Slice cadence:** spec + 1 impl commit, in-session execution by main thread, no plan, no subagent dispatch — matches [[low-arg-shape-pin-close]] XS precedent.

## Goal

Retire stale "future X / future sub-spec / not yet wired" doc-comment framing at 4 sites in `modules/world/` where the cited future work has already shipped. Pure documentation; zero behavior change. No `NAI-XXX-D-*` pin churn.

## Pre-slice premise correction

Predecessor [[hero-points-lifecycle-clear-close]] memo enumerated 4 candidate anchors for this sweep:

1. Stale `// the future revertType refactor (Task 5e)` phrase at `modules/world/npc_registry.go:114`
2. gofmt-alignment drift across `pkg/script/active.go:22-37` (WealthEventType consts)
3. gofmt-alignment drift across `pkg/script/handlers_npc_test.go:252-280` (mockNpc fields)
4. gofmt-alignment drift across `pkg/script/handlers_player_test.go:49-82` (mockActiveNpc receivers)

**Empirical verification at slice start:**

- `gofmt -l pkg/script/active.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go modules/world/npc_registry.go` returns NO output — all 4 files are gofmt-clean.
- Commit `d0f55eac Run gofmt` (between [[combat-sub-spec-framing-doc-cleanup-close]] and [[hero-points-lifecycle-clear-close]]) had already normalized the alignment drift.
- The visible "two-tier" alignment in `mockActiveNpc` methods (`handlers_player_test.go:45-84`) is gofmt's natural behavior: the blank line at L60 separates two alignment groups (methods-with-return-values block vs methods-without-return-values block), each aligned independently. This is NOT drift.

**Conclusion:** Anchors #2/#3/#4 from predecessor memo are FALSE POSITIVES. Only anchor #1 is real. The carry-forward memo class needs an explicit note (delivered via this slice's close memo) so future predecessors stop regenerating the claim.

## Broader sweep finding

Grepping for related stale-framing patterns surfaced 3 additional sites the predecessor memo missed:

- `modules/world/player.go:336` — "consumed by P_PAUSEBUTTON (future sub-spec)" — P_PAUSEBUTTON is shipped at `pkg/script/handlers_dialog.go:10`.
- `modules/world/script.go:157` — "NpcSuspended — future sub-spec (T11)" — NpcSuspended is shipped (`pkg/script/execution.go:15`, set by `handlers_npc.go:446` NPC_DELAY and `:492` NPC_ARRIVEDELAY), and handled for world-queue scripts at `modules/world/script.go:202` (`resumeOrFinishWorld`).
- `modules/world/npc.go:417` — "varn resets (future; VarNpc subsystem not yet wired)" — VarNpc subsystem IS wired (`pkg/objtype/varntype.go` + `resetEntityForRespawn` varn-reseed loop at `npc_registry.go:157`).

Slice scope expands to all 4 stale sites (1 from predecessor enumeration + 3 from broader sweep), per user-confirmed "all 4 stale phrases" scope choice.

## Non-goals

- **No behavior change.** All edits are doc-comment text only.
- **No formal `NAI-XXX-D-*` pin churn.**
- **No actual gofmt running.** Anchors #2/#3/#4 are already clean; no work to do.
- **No other "future" phrases.** `pkg/script/active.go:1054` ("extend as future sub-specs need it"), `pkg/script/handlers_npc.go:1094` ("size>1 audit deferred to a future sub-spec"), and `pkg/script/handlers_player.go:578` ("probability tuning TBD") are generic forward-looking framing, not stale. Deferred to future doc-sweep slices if needed.

## Edits

### Site 1 — `modules/world/npc_registry.go:112-114`

Current:

```go
// resetEntityForRespawn applies the TS Npc.resetEntity(true) reseed
// (TS Npc.ts:280-317, respawn=true branch) factored out so addNpc and
// the future revertType refactor (Task 5e) share one definition.
```

New:

```go
// resetEntityForRespawn applies the TS Npc.resetEntity(true) reseed
// (TS Npc.ts:280-317, respawn=true branch) factored out so addNpc and
// revertType (NAI-19 Task 5e) share one definition.
```

**Justification:** revertType is shipped per NAI-19 Task 5e. Verified by `npc_ai_test.go:50` ("NAI-19 Task 5e: revertType's heavy path now calls through") and 4 hits in `npc_event_queue_test.go` ("NAI-19 Task 5e: heavy path needs server"). The cited Task tag is preserved (audit-trail value); only "future" + "refactor" wording is dropped.

### Site 2 — `modules/world/player.go:335-337`

Current:

```go
// === resume buttons (sub-spec 5f) ===
// Stored by IF_SETRESUMEBUTTONS; consumed by P_PAUSEBUTTON (future sub-spec).
resumeButtons [5]int
```

New:

```go
// === resume buttons (sub-spec 5f) ===
// Stored by IF_SETRESUMEBUTTONS; consumed by P_PAUSEBUTTON (handlers_dialog.go:10).
resumeButtons [5]int
```

**Justification:** P_PAUSEBUTTON handler exists at `pkg/script/handlers_dialog.go:10` (calls `requireProtectedActivePlayer`); test coverage at `handlers_dialog_test.go:123-131`. Replace "(future sub-spec)" with a concrete pointer to the live handler — keeps the "where is this consumed" navigation aid.

### Site 3 — `modules/world/script.go:156-160`

Current:

```go
default:
    // NpcSuspended — future sub-spec (T11).
    s.log.Warn("script in unsupported execution state",
        "script", state.Script.Name, "execution", state.Execution)
    self.ClearActiveScript()
```

New:

```go
default:
    // Defensive: player-side scripts cannot reach NpcSuspended
    // (NPC_DELAY / NPC_ARRIVEDELAY require ActiveNpc, set at
    // handlers_npc.go:446/:492) and there are no other unhandled
    // Execution states. NpcSuspended is dispatched for world-queue
    // scripts at resumeOrFinishWorld (script.go:202).
    s.log.Warn("script in unsupported execution state",
        "script", state.Script.Name, "execution", state.Execution)
    self.ClearActiveScript()
```

**Justification:** This is the `default` arm of `resumeOrFinish(state, self script.ActivePlayer)` — the PLAYER-side post-Execute switch. NpcSuspended is set only by NPC_DELAY (`handlers_npc.go:446`) and NPC_ARRIVEDELAY (`:492`), both of which call `requireActiveNpc` first — so a player-bound script cannot reach NpcSuspended. The existing "future sub-spec (T11)" framing is doubly stale: NpcSuspended is shipped, AND it isn't even reachable here. The rewrite frames the arm as a defensive log + cross-references where NpcSuspended IS handled (`resumeOrFinishWorld:202`).

### Site 4 — `modules/world/npc.go:416-419`

Current:

```go
// What revertType does NOT do on either branch (intentional):
//   - varn resets (future; VarNpc subsystem not yet wired)
//   - activeScript clear (TS behaviour: a revert does not cancel an
//     in-flight script)
```

New:

```go
// What revertType does NOT do on either branch (intentional):
//   - activeScript clear (TS behaviour: a revert does not cancel an
//     in-flight script)
//
// What revertType does NOT do on the KEEPALL light path only:
//   - varn resets (heavy path reseeds all varns via
//     resetEntityForRespawn at npc_registry.go:157; KEEPALL
//     deliberately preserves varn state to match TS Npc.ts:1086-1090).
```

**Justification:** VarNpc subsystem IS wired (`pkg/objtype/varntype.go` defines `VarNpcType` + `VarNpcTypeConfigs`; `resetEntityForRespawn` reseeds varns at `npc_registry.go:157-163`). The current comment placement under "either branch" is wrong: the heavy path's call to `removeNpc + addNpc + resetEntityForRespawn` DOES reset varns. Only the KEEPALL light path (resetOnRevert=false at npc.go:406-409) preserves varns. Restructure the comment into "either branch" vs "KEEPALL only" groups so each statement is accurate for the branch it claims.

## Cadence

Single in-session commit by main thread. No plan, no subagent dispatch.

## Gates

- `gofmt -l modules/world/npc_registry.go modules/world/player.go modules/world/script.go modules/world/npc.go` — clean (no output expected; files are already gofmt-clean, edits are inside `//` comment lines so cannot break gofmt).
- `GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test -race ./...` — 57+ pkgs / 0 FAIL (no behavior change so all tests should remain green from cached state).
- `TestPackAll_TwelveStageSmoke` — PASS.
- Audit-greps post-commit (each must return 0 hits in production `.go` files):
  - `future revertType refactor`
  - `P_PAUSEBUTTON \(future sub-spec\)` (regex-escaped)
  - `NpcSuspended — future sub-spec`
  - `VarNpc subsystem not yet wired`

## Memory + closes

- **No formal `NAI-XXX-D-*` pin** opened or retired (pure doc edit).
- **Close memo:** `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/doc_comment_sweep_close.md`.
- **MEMORY.md index:** prepend a 1-line entry pointing at the close memo.
- **Carry-forward menu:** retire "doc-comment sweep slice" item from next pivot menu in successor close memos.
- **Explicit false-positive retirement in close memo:** the close memo MUST state that anchors #2/#3/#4 from predecessor memo (gofmt drift in `active.go` / `handlers_npc_test.go` / `handlers_player_test.go`) were false-positives normalized by commit `d0f55eac Run gofmt`, and that the visible "two-tier" alignment in `mockActiveNpc` is gofmt's natural blank-line-separated alignment behavior NOT drift. Future close memos must NOT regenerate this claim.

## Patterns worth carrying forward

- **Pre-slice empirical verification beats predecessor-memo inheritance:** the predecessor memo's "gofmt drift" claim was wrong — `gofmt -l` came back clean on all 4 files. Before scoping a "fix X" slice from a predecessor's carry-forward enumeration, run the relevant check tool (`gofmt -l`, `grep`, `go vet`) and confirm the issue still exists. False-positive carry-forward is a known failure mode (see also [[hero-points-lifecycle-clear-close]] non-obvious finding #5: "predecessor resume-memo framing for this slice was wrong premise").
- **Broader-grep sweep at start surfaces hidden scope:** the original anchor #1 was the only real item in the predecessor enumeration, but a `grep -rn "future.*sub-spec\|will ship\|not yet ported\|not yet wired"` against the codebase surfaced 3 more genuinely-stale sites. Don't trust the predecessor enumeration to be exhaustive for sweep slices.
- **"Either branch" claims in dual-path docs need branch-by-branch verification:** the `npc.go:417` "varn resets (future)" claim was placed under "What revertType does NOT do on either branch" but only applied to the KEEPALL light path. Pattern: when documenting a function with a fork (e.g., resetOnRevert), every "always" claim needs verification against both arms.
- **Defensive-log doc-comments should cite WHY the arm is unreachable AND where the case IS handled:** the rewritten `script.go:157` default arm now cites both (a) why NpcSuspended is unreachable from the player path, and (b) where it IS dispatched (`resumeOrFinishWorld:202`). This makes the arm self-documenting against future refactors that might widen the switch's reachability.

## Out of scope (deliberately deferred)

- Other `// future` framing in `pkg/script/active.go:1054`, `pkg/script/handlers_npc.go:1094`, `pkg/script/handlers_player.go:578` — these are generic forward-looking, not pointing at shipped work.
- Any source-code-internal comment refactors not matching the stale-framing pattern (e.g., outdated TS line references, renamed variable mentions, etc.). Different sweep class; no current memo-tracked anchors.
- Anti-bundling: slice 3 (hit-splat multi-hit NAI-127 Bundle 2) is the next slice in the bundled session — do NOT touch hit-splat or rsbuf encoder in this slice.
