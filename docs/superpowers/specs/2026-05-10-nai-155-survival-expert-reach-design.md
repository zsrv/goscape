# NAI-155 — Survival Expert second-contact reach failure (investigation)

## 1. Scope

Investigation sub-spec, per `investigation_subspec_cadence`. Symptom
surfaced by NAI-154 Java-client smoke: after one successful talk with
the Tutorial Island Survival Expert (NPC #943), a subsequent `OPNPC1`
click on the same NPC at `cheb_dist=1` with `op_trigger=true` fails
to fire the talk trigger. The interaction is destroyed by the
`"I can't reach that!"` branch at `modules/world/interaction.go:271`.

Pinned reproduction frame (paste from user's NAI-154 smoke,
2026-05-10 21:09:51):

```
tick=222 player_uid=2232170497 target_kind=Npc target_type_id=943
  target_x=3105 target_z=3095 player_x=3105 player_z=3096 cheb_dist=1
  op_trigger=true ap_trigger=false ap_range=10 waypoint_idx=-1
  branch_pre=0 branch_post=0 interacted=false interaction_fired=false
  steps_taken=1 repathed=false target_still_set=true
tick=223 (same coords) branch_pre=0 branch_post=0 interacted=false
  interaction_fired=false steps_taken=0 repathed=false
  target_still_set=false
```

Stage 1 audit → Stage 2 fix → Java-client smoke → conditional Stage 3.

## 2. Tech stack

Go 1.26+. No new deps. Investigation-only; Stage 2 fix touches
`modules/world/interaction.go` (`processInteraction`) and very likely
relaxes the inner guard in `(*Player).tryInteract` at `interaction.go:387`.

## 3. TS source

`Engine-TS/src/engine/entity/Player.ts:1200-1268` (`processInteraction`).
The three load-bearing gate sites:

```ts
// L1209-1224 — pre-step arm gated on canAccess
if (this.target && this.canAccess()) {
    if (!this.validateTarget()) {
        this.clearInteraction();
        this.unsetMapFlag();
        return;
    }
    if (!followOp) { this.processWalktrigger(); }
    interacted = this.tryInteract(false);
}

// L1232 — walktrigger gated on canAccess
if (this.hasWaypoints() && this.canAccess()) {
    this.processWalktrigger();
}

// L1241 — movement (goscape runs equivalent in processPathing at tick.go:38)
this.updateMovement();

// L1244-1252 — post-step interact + "I can't reach" gated on canAccess
if (this.target && this.canAccess() && !followOp) {
    interacted = this.tryInteract(this.stepsTaken === 0);
    if (!interacted && !this.hasWaypoints() && this.stepsTaken === 0) {
        this.messageGame("I can't reach that!");
        this.clearInteraction();
    }
}
```

`Player.canAccess()` (`Player.ts:805-812`) returns false when the
player is delayed, has a Main/Chat modal open, or is mid-protected-
script. Per TS L1244 it gates the **entire** post-step block —
including the user-visible "I can't reach" message and
`clearInteraction()`.

## 4. Goscape divergence (Bundle 0 — resolved from Frame B)

`branch_pre=0 && branch_post=0` on **both** ticks means
`(*Player).tryInteract` returned at its first-line guard
(`interaction.go:387`): `p.target == nil || !p.HasInteraction() || !p.CanAccess()`.

- `target_still_set=true` and `target_kind=Npc` → `p.target != nil`.
- `target_kind=Npc` with `targetOp=1` (OPNPC1) → `isFollowOp(p)` false
  → `HasInteraction()` returns true (`player_script.go:1080`).
- ⇒ **the guard trips on `!CanAccess()`.**

`CanAccess()` (`player_script.go:324-335`) returns false when any of:

1. `p.delayed`
2. `p.modalState & (Main|Chat) != 0`
3. `p.protectedScriptActive()` (`activeScript.Pointers & PtrProtectedActivePlayer`)

The packet log shows the OPNPC1 second-click decode at 21:09:51.833
within tick 222 (the failing tick). A 4-byte packet
`[31 76 43 44]` (opcode 31) arrives at 21:09:52.216 — a candidate
dialog-close packet between ticks 222 and 223.

## 5. Bundles

### Bundle 0 — Frame B routing (resolved)

Both ticks guard-trip on `!CanAccess()`. Bundles 1 and 2 are both P0;
Bundle 3 is post-smoke verification.

### Bundle 1 (P0) — TS-fidelity gate parity at `processInteraction`

**Hypothesis:** the most likely root cause is `processInteraction`
lacking the three `canAccess()` gates that TS uses. Even if the residual
`CanAccess=false` state in Bundle 2 is "correct" (e.g., player still
mid-dialog when OPNPC1 fires), TS would simply skip the interact block
and let the interaction PERSIST to the next tick — goscape destroys it
with `"I can't reach!"` + `ClearInteraction()`.

**Sites:**

| Goscape line | TS line | Current | TS-faithful |
|---|---|---|---|
| `interaction.go:245-249` (pre-step) | L1210 | runs unconditionally | gate on `p.target != nil && p.CanAccess()` |
| `interaction.go:256-258` (walktrigger) | L1232 | gates on `hasWaypoints()` only | add `&& p.CanAccess()` |
| `interaction.go:267-273` (post-step + "can't reach") | L1244 | gates on `target != nil && !followOp` | add `&& p.CanAccess()` |

**Inner guard at `interaction.go:387`** becomes redundant under
Bundle 1's site-gates; either remove (matches TS shape: `tryInteract`
has no canAccess re-check) or keep as defensive (label with deviation
tag per `defensive_gate_doc_comment_label`). Recommend remove for
TS-fidelity; the `HasInteraction()` half of the guard is preserved
where TS doesn't need it (TS dispatches at the call site via
`!followOp` and validateTarget), so the inner guard collapse needs
careful audit of the `!HasInteraction()` predicate's call paths.

**Subagent:** Sonnet. Reads TS L1200-1268, goscape `processInteraction`
+ `tryInteract` + `CanAccess` + `HasInteraction`. Output: GREEN/RED on
gate-parity, exact line-level patch shape, list of call sites that
relied on the inner guard for `!HasInteraction()` rejection.

**Expected output:** RED; concrete patch shape for Stage 2.

### Bundle 2 (P0) — WHY `CanAccess()` is false on the second contact

**Hypothesis space:** the `CanAccess()` false state on tick 222
originates from one of three residual fields after the first chatnpc:

- **`modalState & Chat`:** sole setter at `player_script.go:867`;
  sole clearer is `CloseModal` at `player_script.go:799`. Audit the
  dialog-close path: which opcode handler calls `CloseModal` on
  chatnpc dismiss? Cross-reference TS chatnpc lifecycle + the
  opcode-31 packet at 21:09:52.216 in the smoke log.
- **`protectedScriptActive()` (`activeScript.Pointers & PtrProtectedActivePlayer`):**
  per memory `protect_over_clear` (NAI-111-D1), `CloseModal` must NOT
  strip PAP. Verify the chatnpc resume-to-end path clears
  `activeScript`. If chatnpc suspends with PAP set and resumes to
  completion, `activeScript` should be nil afterward.
- **`p.delayed`:** sole clearer at `tick.go:278`. Does chatnpc set
  delayed? `tryDelay` / `p_delay` opcodes set `delayed=true` and
  `delayedUntil=currentTick+1+ticks` (`player_script.go:53-54`). If
  the delay-expiry path doesn't reset `delayed=false` correctly, this
  field can dangle.

**Subagent:** Sonnet. Reads `CanAccess` + all three field lifecycles +
chatnpc handler chain + dialog-close packet handlers + TS counterparts.
Output: identify which of the three fields is residual on tick 222,
plus the root cause (missing clear-site OR a packet-ordering bug
between dialog-close and OPNPC1).

**Expected output:** RED on at least one of the three; concrete
explanation of state residue.

**Routing:** if Bundle 1's gate-parity fix alone makes the Java-client
smoke pass (interaction persists until `CanAccess()` returns true,
talk eventually fires), Bundle 2's findings route to a separate
sub-spec (NAI-156). If smoke fails post-Bundle-1 (interaction NEVER
re-enters because `CanAccess()` is *permanently* false), Bundle 2 is
in-scope Stage 3 of NAI-155.

### Bundle 3 (cascade adjacency) — NPC wander + target-clear

Symptom mentions: "NPC then wanders one tile, target clears". The
target-clear is downstream of the spurious `ClearInteraction()` at
`interaction.go:272` (fixed by Bundle 1). The NPC wander is independent
random-walk AI unaffected by player interaction. Cascade should
resolve with Bundle 1 fix.

**Verification:** Java-client smoke post-Stage-2. If the
"NPC steps away after first dialog" still occurs and obstructs second
contact (e.g., target moves to `cheb_dist>1` between SetInteraction
and tryInteract), route to NAI-156 alongside Bundle 2's residue.

## 6. Stage 2 fix shape

Conditional on Bundle 1 RED (expected). Single-task TDD port:

**T1 — `processInteraction` gate-parity (TS L1209/1232/1244).**

```go
// Pre-step arm (TS L1209-1224).
if p.target != nil && p.CanAccess() {
    if !followOp {
        p.processWalktrigger()
    }
    p.interactCallSlot = 0
    interacted = p.tryInteract(false)
}

// Post-step arm (TS L1227-1252). Skipped when pre-step interacted.
if !interacted {
    p.pathToPathingTarget()

    if p.hasWaypoints() && p.CanAccess() {
        p.processWalktrigger()
    }

    if !p.hasWaypoints() && followOp {
        p.ClearInteraction()
    }

    // Post-step interact (TS L1244-1252). Gated on CanAccess: when
    // a modal/protected/delayed state transiently blocks interaction,
    // PRESERVE the anchor — do NOT fire "I can't reach!". TS preserves
    // interaction across the gate; next tick's CanAccess()=true re-enters.
    if p.target != nil && p.CanAccess() && !followOp {
        p.interactCallSlot = 1
        interacted = p.tryInteract(p.stepsTaken == 0)
        if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
            p.MessageGame("I can't reach that!")
            p.ClearInteraction()
        }
    }
}
```

Inner guard at `interaction.go:387`: relax to
`if p.target == nil || !p.HasInteraction()` (drop `!p.CanAccess()`
half) so the call-site gate becomes load-bearing. Preserve
`!HasInteraction()` half because callers like `processWalktrigger` /
test fixtures may invoke `tryInteract` without going through
`processInteraction`.

**Test pins (TDD, before patch):**

- `processInteraction` with `modalState=Chat` set: NOT fire
  `"I can't reach"`, NOT call `ClearInteraction`, leave
  `p.target` non-nil, leave waypoint/interactionFired untouched.
- `processInteraction` with `protectedScriptActive=true`: same
  preservation invariant.
- `processInteraction` with `p.delayed=true && currentTick<delayedUntil`:
  preserved (current behavior at `interaction.go:200-202` already
  returns early before reaching the post-step; pin to lock in TS shape).
- `processInteraction` with `CanAccess=true` and target at
  `cheb_dist=1` with `opTrigger` present: branch 1 fires, OP trigger
  fires, interaction succeeds (regression pin for the happy path).
- Negative pin: `processInteraction` with `target=nil` after some
  other path: post-step block does not run.

Each pin documents the TS line it mirrors. Single Sonnet implementer
+ single Sonnet reviewer per project default.

## 7. Smoke

User-driven Java-client smoke per `smoke_test_server_handoff` memory:

1. Log in on Tutorial Island.
2. Right-click → Talk-to Survival Expert (#943). Dialog opens.
3. Complete dialog (continue through all chatnpc options).
4. After dialog closes, right-click → Talk-to Survival Expert again.
5. **Pass:** dialog reopens (OPNPC1 fires opnpc1 trigger).
6. **Fail:** "I can't reach that!" or no response.

Capture Frame B (`"interaction tick"` slog line) for both the
post-dismiss tick and the second-contact tick. Compare against pre-fix
ticks 222/223 above.

## 8. Stage 3 (conditional)

If smoke passes after Bundle 1 alone:
- Close NAI-155 PRIMARY.
- Open NAI-156 from Bundle 2 findings (residual `CanAccess()=false`
  is masked but not fixed; future scripts/scenarios may surface).

If smoke fails (CanAccess permanently false → interaction stuck
across many ticks until manual cancel):
- In-scope Stage 3: Bundle 2's root-cause fix. Identify the residual
  field (modalState / activeScript / delayed), find the missing
  clear-site (TS counterpart), patch with TDD pins on the lifecycle
  boundary.

If smoke surfaces adjacent untracked divergences (per
`smoke_surfaces_adjacent_divergences` memory):
- ≤30 LOC in-scope stretch; else route to NAI-156.

## 9. Risk register

- **R1 — inner guard relaxation breaks unrelated tryInteract callers.**
  Mitigation: grep `tryInteract` call sites; verify each goes through
  `processInteraction` OR keeps its own `CanAccess` gate. Bundle 1
  subagent enumerates.
- **R2 — `validateTarget()` not ported.** TS L1212 calls `validateTarget`
  inside the pre-step canAccess gate; goscape has only the
  level-mismatch subset at `interaction.go:230-240`. Bundle 1 spec
  preserves goscape's pre-pre-step level check (runs unconditionally,
  before the new pre-step gate) so the level-clear path is unchanged.
- **R3 — followOp + waypoint-exhaustion clear at `interaction.go:261-263`
  unchanged.** TS L1237-1239 runs UNGATED on canAccess; goscape matches.
  Verify in Bundle 1.
- **R4 — `processInteraction` entry guard at L196-202 also checks
  `!p.delayed`.** Now redundant with `CanAccess()` site-gates, but
  the entry guard returns *immediately* (skipping path/Frame B emit).
  Keep entry guard to preserve current early-return + Frame B
  semantics.
- **R5 — Frame B routing miss.** If Bundle 2's residual is
  `p.protectedScriptActive`, the fix shape mirrors NAI-111. Bundle 2
  subagent reads `nai_111_protect_over_clear.md` first.

## 10. Acceptance

PRIMARY (gate-parity fix, NAI-155 close):
- All Stage 2 test pins pass.
- Java-client smoke passes: second-talk reopens dialog cleanly.
- No regression in NAI-154 / NAI-152 B2 / NAI-148 / NAI-147 smoke.

SECONDARY (Bundle 2 cascade — conditional NAI-156):
- Frame B post-fix shows `branch_pre` or `branch_post` ∈ {1,2,3,4}
  on the second-contact tick (NOT 0/0). If still 0/0 across many
  ticks, Bundle 2 residue is load-bearing.

## 11. Memory hits to apply

- `investigation_subspec_cadence` — Stage 1/2/3 shape.
- `audit_full_method_against_ts` — Bundle 1 audits the whole
  `processInteraction` method, not just L267.
- `cascade_theory_smoke_binding` — close NAI-155 on smoke green; route
  residue to NAI-156.
- `defensive_gate_doc_comment_label` — if inner guard kept, label as
  "(goscape defensive; TS skips this check)".
- `protect_over_clear` (NAI-111) — Bundle 2 prerequisite read.
- `smoke_test_server_handoff` — user runs the smoke; we don't.
- `verification_before_completion` — no green claim without explicit
  smoke Frame B paste.
