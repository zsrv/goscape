# NAI-43 — NPC-AI OPPLAYER&lt;N&gt; Self2 binding audit

**Cadence:** Compressed (per `compressed_cadence.md`). Combined spec+plan;
no separate plan doc; no formal review stage. Single TDD-pass commit
adding test-only absence pins, plus a close commit retiring the
follow-up tag and recording the audit verdict.

## Motivation

`NAI-40-SB3` was opened at NAI-40 close (carry-forward 2026-04-27,
`a34cbdf`) as "NPC-AI OPPLAYER&lt;N&gt; Self2 binding audit; conditional on
grep showing aiOpPlayer&lt;N&gt; dispatch sets Self2 or not." The rationale
was: NAI-39 introduced `Self2` substrate and NAI-40 wired the first
production producer (Player→Player op-click), so the AI-side dispatch
needed verification that an implementer had not mis-wired the Npc→Player
arm to also set `Self2` (which would diverge from TS).

This sub-spec executes that audit and closes the tag.

### Audit verdict: NO production divergence

**TS reference behavior** — `ScriptRunner.init`
(`Engine-TS/src/engine/script/ScriptRunner.ts:66-118`) is asymmetric:

```ts
} else if (target instanceof Player) {
    if (self instanceof Player) {
        state._activePlayer2 = target;            // Self2
        state.pointerAdd(ScriptPointer.ActivePlayer2);
    } else {
        state._activePlayer = target;             // Self  (NOT Self2)
        state.pointerAdd(ScriptPointer.ActivePlayer);
    }
}
```

The `_activePlayer2` slot is set ONLY when self and target are both
`Player`. When `self instanceof Npc` and `target instanceof Player`
(the AI OPPLAYER&lt;N&gt; / APPLAYER&lt;N&gt; dispatch case), control falls into
the `else` arm and `_activePlayer = target` — `_activePlayer2` is left
unset. The same asymmetry holds for `_activeNpc2` (only when self is
also an Npc), `_activeLoc2`, `_activeObj2`.

**TS production call sites** — `Npc.tryInteract`
(`Engine-TS/src/engine/entity/Npc.ts:861-883`) at lines 871 and 878
calls `ScriptRunner.init(script, this, this.target)` for both OP and AP
trigger branches. `this`=Npc, `this.target`=Player → `_activePlayer`
populated, `_activePlayer2` left nil. This is the canonical TS shape
goscape must mirror.

**goscape behavior at HEAD `4b3cd58`** — `buildNpcScriptState`
(`modules/world/npc_script.go:225-261`) implements the TS asymmetry
correctly:

```go
state.ActiveNpc = npc                                 // line 233
state.Pointers |= script.PtrActiveNpc                 // line 234
…
switch t := target.(type) {
case nil:                                             // no secondary
case script.ActivePlayer:                             // line 245-248
    // TS: self=Npc, target=Player → _activePlayer = target, PtrActivePlayer.
    state.Self = t
    state.Pointers |= script.PtrActivePlayer
case script.ActiveLoc:                                // → ActiveLoc
    …
case script.ActiveObj:                                // → ActiveObj
    …
case script.ActiveNpc:                                // → OtherActiveNpc
    state.OtherActiveNpc = t
    state.Pointers |= script.PtrOtherActiveNpc
}
```

The `case script.ActivePlayer:` arm sets `state.Self` (primary
`_activePlayer` slot, consumed via `s.Self` in `handlers_player.go`),
NOT `state.Self2`. `Pointers |= script.PtrActivePlayer` — never
`PtrActivePlayer2`. **This matches TS exactly.**

**All AI-side dispatch funnels through this single seam.** Verified by
grep:

- `s.runNpcScript(sf, n, target, nil, nil)` is called at 8 sites in
  `modules/world/npc_interaction_trigger.go` (lines 60, 82, 121, 144,
  183, 207, 230, 250) — every AI_*PLAYER, AI_*NPC, AI_*LOC, AI_*OBJ
  fire goes through `runNpcScript` → `buildNpcScriptState`.
- `runNpcScript` at `npc_script.go:278-290` is the only path that wires
  the NPC self+target binding for trigger-fire dispatch.
- `npc_event_queue.go:46` and `npc_hunt.go:341` also call
  `runNpcScript` for AI_QUEUE&lt;N&gt; / event-queue paths, but always with
  `target == nil`.

**No regression risk at HEAD.** Self2 binding for NPC-AI OPPLAYER&lt;N&gt; is
correct: the slot is left unset, matching TS asymmetry.

### Test gap (small, real)

The existing presence-pin
`TestBuildNpcScriptStateDispatchesActivePlayer`
(`modules/world/npc_script_test.go:475-489`) asserts only:

```go
if state.Self == nil { t.Error("Self: nil, want set") }
if state.Pointers&script.PtrActivePlayer == 0 { t.Error(…) }
```

It does NOT assert the conspicuous absence of `Self2` / `PtrActivePlayer2`.
Per the project convention `ts_asymmetry_dual_pin.md`:

&gt; When preserving a TS quirk, pin both the presence AND the conspicuous
&gt; absence with tests; the absence-pin escalates on upstream fixes.

The Npc→Player Self2 asymmetry is exactly this kind of TS shape. An
implementer working on a future AI-side feature (e.g., an AI-side
HINT_PL consumer, or any "dual-player" scripting context) might
"helpfully" wire `state.Self2 = t` here on a misread of NAI-39 / NAI-40
substrate, breaking TS fidelity silently. An absence-pin catches that
in a single test failure.

The same gap exists, less acutely, for the OtherActiveNpc arm
(line 530-544 `TestBuildNpcScriptStateDispatchesOtherActiveNpc`):
asserts `state.OtherActiveNpc != nil` but not `state.ActiveNpc != other`
(i.e., that the primary slot was NOT used as a fallback).

## Tech stack

- **Go 1.26+** (per `go_version.md`).
- TS source: `Engine-TS` only (per `ts_source_canonical_path.md`).
- HEAD baseline: `4b3cd58` (NAI-42 close).

## Scope

**In scope (all under `modules/world/`):**

1. **Test addition only** — extend
   `TestBuildNpcScriptStateDispatchesActivePlayer`
   (`modules/world/npc_script_test.go:475-489`) with an absence-pin
   block: assert `state.Self2 == nil` and
   `state.Pointers&script.PtrActivePlayer2 == 0`.
2. **Test addition only** — extend
   `TestBuildNpcScriptStateDispatchesOtherActiveNpc`
   (`modules/world/npc_script_test.go:530-544`) with an absence-pin
   block: assert `state.ActiveNpc == n` (primary slot still bound to
   self, not overwritten by target) and that no second-NPC pointer
   bleed occurred. The existing test variable `n` is the NPC self;
   `other` is the second NPC. Pin: `state.ActiveNpc == n &amp;&amp;
   state.OtherActiveNpc == other`.
3. **Retirement and audit-record** — in `nai_followups.md`:
   add a new `## NAI-43 (CLOSED 2026-04-28)` section documenting the
   audit verdict (no production divergence; tests added). Strike
   `NAI-40-SB3` from each open-followups list it currently appears in.

**Total: ~10-15 LOC test additions, 0 LOC production change.**

**Explicitly out of scope:**

- Any production-code change to `buildNpcScriptState` or its callers —
  audit verdict is no divergence.
- Any AP/OP arm coverage extension beyond what's already in the file —
  the Loc and Obj test arms (lines 491-525) already pin presence and
  the absence-pin would be redundant for those (no Loc2/Obj2 arms
  reachable from Npc-self anyway).
- Adding a separate `Self2 == nil` pin for the nil-target test
  (`TestBuildNpcScriptStateNilTargetSetsNoSecondaryPointer`,
  visible at lines 546-…). The existing test name + body should already
  cover Self2-stays-nil; if not, scope creep.
- Any NAI-40 deviation re-audit (NAI-40-D-OPCALLED-MISSING,
  NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED, etc.) — these have separate
  tracked closures.

## Plan (single task)

**Task 1.** Add the two absence-pin blocks. Order:

1. **Open** `modules/world/npc_script_test.go` and locate the existing
   `TestBuildNpcScriptStateDispatchesActivePlayer` (lines 475-489).
2. **Append** the absence-pin assertions inside the existing test body,
   after the existing presence-pin block (so the test stays one logical
   unit). Shape:

   ```go
   // Absence pin (NAI-43, ts_asymmetry_dual_pin):
   // Self2 / PtrActivePlayer2 must NOT be set when self=Npc, target=Player.
   // TS ScriptRunner.init:84-91 sets _activePlayer2 only when self and
   // target are both Player; the self=Npc branch falls into the else arm
   // at L89-90 and assigns _activePlayer (already pinned above).
   if state.Self2 != nil {
       t.Errorf("Self2: got %v, want nil (NPC-self → target.Player goes to Self, not Self2)", state.Self2)
   }
   if state.Pointers&script.PtrActivePlayer2 != 0 {
       t.Error("Pointers: PtrActivePlayer2 set, want unset")
   }
   ```

3. **Locate** `TestBuildNpcScriptStateDispatchesOtherActiveNpc`
   (lines 530-544).
4. **Append** the absence-pin assertions after the existing presence-pin
   block:

   ```go
   // Absence pin (NAI-43, ts_asymmetry_dual_pin):
   // ActiveNpc (primary slot, set to self=n at L233) must remain bound
   // to self — the target's Npc must land in OtherActiveNpc, not
   // overwrite the primary slot. TS ScriptRunner.init:92-95 sets
   // _activeNpc2 only when self is also Npc; the primary _activeNpc
   // (already set to self) must be untouched.
   if state.ActiveNpc != n {
       t.Errorf("ActiveNpc: got %v, want self n (target overwrote primary slot)", state.ActiveNpc)
   }
   ```

5. **Verify** with:

   ```bash
   GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestBuildNpcScriptStateDispatches -count=1
   GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
   GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
   ```

6. **Single commit** for the test additions:
   `test(world): NAI-43 T1 — pin Npc→Player Self2 absence + Npc→Npc primary-slot non-overwrite`

**Task 2.** Audit-record + retirement.

1. **Edit** `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:
   - Strike `NAI-40-SB3` from each `## NAI-N (CLOSED …)` deferred-items
     list it currently appears in (lines 2313, 2335, 2384, 2438 per
     the brainstorm grep — re-grep at execution time and enumerate ALL
     hits per `enumerate_all_sites.md`).
   - Append a new `## NAI-43 (CLOSED 2026-04-28)` section recording:
     - Scope: closed `NAI-40-SB3` audit.
     - Verdict: no production divergence; goscape's
       `buildNpcScriptState` correctly mirrors TS `ScriptRunner.init`
       asymmetry (NPC self + Player target → Self only, never Self2).
     - Implementation: 2 absence-pin tests added on existing test
       functions; 0 production LOC.
     - Memories applied: `ts_asymmetry_dual_pin.md`,
       `compressed_cadence.md`, `enumerate_all_sites.md`,
       `close_commit_memory_trailer.md`.
     - Net deviation tally: unchanged.

2. **Close commit** with `Closes memory:` trailer per
   `close_commit_memory_trailer.md`:
   `chore(world,docs): NAI-43 closed — NPC-AI OPPLAYER<N> Self2 binding audit (no production divergence)`

   With trailer:
   ```
   Closes memory: NAI-40-SB3 (NPC-AI OPPLAYER<N> Self2 binding audit)
   ```

## Risks / non-issues

- **Risk: stale grep at edit-time** — the brainstorm enumerated 4 hit
  locations for `NAI-40-SB3` in `nai_followups.md` (lines 2313, 2335,
  2384, 2438). Re-grep at Task 2 execution time per
  `enumerate_all_sites.md`; do not trust the line numbers.
- **Non-issue: Self / Self2 type matching** — `state.Self` is typed
  `script.ActivePlayer` (interface). The `case script.ActivePlayer: t`
  binding compiles cleanly because `*Player` satisfies the interface;
  the absence-pin `state.Self2 != nil` is a nil-interface check on a
  zero-value field, not a value comparison.
- **Non-issue: `state.ActiveNpc != n` comparison** — `n` is
  `*Npc`; `state.ActiveNpc` is `script.ActiveNpc` interface. The
  comparison works because `*Npc` satisfies `ActiveNpc` and Go interface
  equality compares concrete type + value. The existing test pattern at
  `script_test.go:1334` uses the same pattern (`state.Self2 != p2`),
  confirming the idiom.
- **Non-issue: existing nil-target test** — line 546-… already pins
  Self2-stays-nil for the nil-target arm; the new absence-pins target a
  different arm (Player target) and don't conflict.

## Verification checklist (per `verification_before_completion.md`)

After Task 1 commit:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

After Task 2 commit:

```bash
git log --oneline -3
git show HEAD --stat
rg "NAI-40-SB3" .  # should return 0 hits in active follow-ups (only inside the new NAI-43 section as audit-history)
```

## Outcome

- 1 closed: `NAI-40-SB3`.
- 0 opened: no new deviations.
- 0 production LOC changed; ~10-15 test LOC added.
- 1 follow-up tracker updated; 1 NAI section added.
