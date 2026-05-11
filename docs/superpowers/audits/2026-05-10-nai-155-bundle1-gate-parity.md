# NAI-155 Bundle 1 — canAccess Gate-Parity Audit

**Date:** 2026-05-10  
**Scope:** `processInteraction` canAccess gate divergence; `tryInteract` inner-guard relaxation.  
**Files:** `modules/world/interaction.go`, `modules/world/player_script.go`, `Engine-TS/src/engine/entity/Player.ts`

---

## 1. Verdict: RED

Three confirmed canAccess gate divergences in `processInteraction`. Two allow goscape to reach `ClearInteraction()` calls that TS would skip when `canAccess()=false`.

---

## 2. Gate-Divergence Table

| TS line | TS gate | Goscape line | Goscape gate | Divergent? |
|---------|---------|--------------|--------------|------------|
| L1210 | `this.target && this.canAccess()` (outer pre-step arm) | `interaction.go:244-249` | `!followOp` only (no canAccess; no target nil-check) | **Y** |
| L1212 | `!this.validateTarget()` inside canAccess arm | `interaction.go:230-240` | level-mismatch only, BEFORE canAccess gate, always fires | **Y** (ordering; see R2) |
| L1219 | `!followOp` inside canAccess arm | `interaction.go:245` | `!followOp` ungated | **Y** (walktrigger fires when canAccess=false) |
| L1223 | `this.tryInteract(false)` inside canAccess arm | `interaction.go:249` | `tryInteract(false)` ungated | **Y** (but tryInteract has inner guard at L387) |
| L1232 | `this.hasWaypoints() && this.canAccess()` (in-block walktrigger) | `interaction.go:256-257` | `p.hasWaypoints()` only (no canAccess) | **Y** |
| L1237 | `!this.hasWaypoints() && followOp` (ungated) | `interaction.go:261-262` | `!p.hasWaypoints() && followOp` (ungated) | N |
| L1244 | `this.target && this.canAccess() && !followOp` (post-step arm) | `interaction.go:267` | `p.target != nil && !followOp` (missing canAccess) | **Y** |
| L1248-1250 | `"I can't reach that!"` + clearInteraction inside canAccess arm | `interaction.go:270-273` | ungated (fires when canAccess=false) | **Y** (ClearInteraction destroyed) |

---

## 3. Risk Audits

### R1 — Production tryInteract callers

**Verified production Player.tryInteract callers:** exactly two.

- `interaction.go:249` — pre-step arm (ungated)
- `interaction.go:269` — post-step arm (ungated)

`npc_interaction.go:222,237` — `(*Npc).tryInteract(s, ...)` — different receiver, unaffected.

**Test sites that call `p.tryInteract(false/true)` directly:**

| File | Call site | Uses delayed/modal/protected setup? | Needs update when guard relaxes? |
|------|-----------|--------------------------------------|----------------------------------|
| `interaction_tryinteract_guard_test.go:26` | `TestTryInteract_FollowOp_ShortCircuits` | No | No (tests HasInteraction=false guard, not CanAccess) |
| `interaction_tryinteract_guard_test.go:60` | `TestTryInteract_NotFollowOp_NotShortCircuited` | No | No (happy path — CanAccess=true) |
| `interaction_tryinteract_guard_test.go:80` | `TestTryInteract_Delayed_ShortCircuits` | **Yes — `p.delayed=true`** | **YES** — pins `!CanAccess()` at tryInteract inner guard; once guard moves to call-sites, this test becomes a call-site test |
| `interaction_tryinteract_guard_test.go:94` | `TestTryInteract_NoTarget_ShortCircuits` | No | No (nil target) |
| `interaction_tryinteract_guard_test.go:117` | `TestTryInteract_FollowOpDelayed_BothGatesGuard` | **Yes — `p.delayed=true`** | **YES** — pins combined !HasInteraction + !CanAccess; delayed guard moves to call-site |
| `interaction_tryinteract_guard_test.go:146` | `TestTryInteract_HasInteractionTrue_ProceedsToBranch1` | No | No (happy path) |
| `interaction_test.go:1059,1083,1124` | `TestTryInteractNpcAllowsOpWhenSceneryGated`, `TestTryInteractLocBlocksOpWhenSceneryFalse`, `TestTryInteractLocAllowsOpWhenSceneryTrue` | No | No (CanAccess=true in all three) |
| `interaction_test.go:1594,1623,1636,1666,1712,1724,1739,1759` | Various branch-1/2/3/4 pin tests | No | No |
| `interaction_trigger_nai69_test.go:27,57` | NAI-69 AP-retry tests | No | No (CanAccess=true) |
| `player_interaction_trigger_test.go:398,469,494` | AP-Player trigger tests | No | No (CanAccess=true) |
| `interaction_debug_test.go:271,288` | Debug branch coverage | No | No |

**Summary for R1:** Two tests in `interaction_tryinteract_guard_test.go` will need updating:
- `TestTryInteract_Delayed_ShortCircuits` (line 80) — currently pins the `!CanAccess()` branch INSIDE `tryInteract`. After relaxation the delayed guard lives only at call-site gates in `processInteraction`; a direct `p.tryInteract(false)` with `p.delayed=true` will pass through to branch logic (which may return false for other reasons, but not via the CanAccess guard).
- `TestTryInteract_FollowOpDelayed_BothGatesGuard` (line 117) — same issue; the combined guard test fires on `!HasInteraction()` first (followOp), so this test STILL returns false — but the `!CanAccess()` pin is no longer what stops it. The test remains technically correct but its documented rationale is stale.

### R2 — validateTarget ordering

TS `validateTarget()` (L1212) runs INSIDE the canAccess-gated pre-step arm (`if this.target && this.canAccess()`). It checks: (a) same level, (b) type not changed via changetype, (c) `isValid()`. Goscape's level-mismatch check (`interaction.go:230-240`) is a **subset** of validateTarget, runs BEFORE the pre-step canAccess gate, and fires unconditionally.

**Consequence:** When `canAccess()=false`, TS skips validateTarget entirely (interaction preserved). Goscape still fires the level-mismatch subset unconditionally — this is a separate, pre-existing deviation (`DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK` comment at L197-202 documents the broader framing). The ordering of the level-mismatch check relative to a new pre-step canAccess gate is **not a conflict**: the level-mismatch check at L230-240 runs before the gate and will continue to do so. If `canAccess()=false` AND the target is cross-level, goscape currently clears interaction (correct — TS would also clear, eventually, once canAccess returns true). **This is a pre-existing acceptable deviation; the new canAccess gates do not conflict with it.**

TS validates changetype and `isValid()` only inside the canAccess arm. Goscape has no changetype or `isValid()` check — this is a separate gap not addressed by this NAI.

### R3 — followOp + waypoint-exhaustion clear

TS L1237-1239: `if (!this.hasWaypoints() && followOp) { this.clearInteraction(); }` — ungated (outside the canAccess block at L1244).

Goscape `interaction.go:261-263`: `if !p.hasWaypoints() && followOp { p.ClearInteraction() }` — matches. **No divergence. CONFIRMED.**

### R4 — Entry guard at L196-202

```go
if p.delayed && s.currentTick < p.delayedUntil {
    return
}
```

This guard uses tick-math (`currentTick < delayedUntil`) — a **strict subset** of `CanAccess()`. `CanAccess()` returns false for: (a) `p.delayed` (any), (b) modal state, (c) protectedScriptActive. The entry guard only catches `p.delayed=true` with future tick.

**Load-bearing case the entry guard catches that new call-site gates miss:** If `p.delayed=true` and `currentTick >= delayedUntil` (delay expired), the entry guard does NOT fire, but `CanAccess()` still returns false (because `p.delayed` is still true until something clears it). So the entry guard filters "still-counting-down delay" (early return before Frame B), while the call-site canAccess gates filter "delayed=true regardless of tick math." These are complementary, not redundant. The entry guard remains load-bearing as an optimization: it short-circuits the entire function (including Frame B emit) without needing to reach the pre-step arm. Removing it would be incorrect.

---

## 4. Patch Shape — processInteraction

Replace `interaction.go:244-274` (current pre-step + post-step arms). Full replacement for those two blocks:

```go
// Pre-step interact arm (TS L1209-1224). Gated on target + canAccess.
if p.target != nil && p.CanAccess() {
    if !followOp {
        p.processWalktrigger()
    }
    p.interactCallSlot = 0
    interacted = p.tryInteract(false)
}

// Post-step arm (TS L1227-1252). Skipped when pre-step interacted.
if !interacted {
    // Recalc path (TS L1228-1229).
    p.pathToPathingTarget()

    // Process walktrigger if there are waypoints AND canAccess (TS L1232).
    if p.hasWaypoints() && p.CanAccess() {
        p.processWalktrigger()
    }

    // followOp + waypoint exhaustion → clear (TS L1237-1239). Ungated.
    if !p.hasWaypoints() && followOp {
        p.ClearInteraction()
    }

    // Post-step interact (TS L1244-1252). Gated on target + canAccess + !followOp.
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

Note: The existing level-mismatch check at `interaction.go:230-240` stays unchanged before the pre-step arm — consistent with R2 finding.

---

## 5. Patch Shape — tryInteract inner guard

Current `interaction.go:387`:
```go
if p.target == nil || !p.HasInteraction() || !p.CanAccess() {
```

**Relax to:**
```go
if p.target == nil || !p.HasInteraction() {
```

Rationale: With canAccess gates now at both call sites in `processInteraction`, `tryInteract` will only be reached when `CanAccess()=true`. The inner guard's `!CanAccess()` arm is redundant post-patch and its removal matches TS `tryInteract`'s own guard at L1114: `!this.target || !this.hasInteraction() || !this.canAccess()` — TS has it there because `tryInteract` is called directly without outer gates; once goscape's outer gates are added, the inner check becomes defence-in-depth only. For strict TS-parity (where the guard IS present in tryInteract), it is acceptable to keep it; however the bug report shows it is the source of observable failure (Frame B `branch_pre=0 && branch_post=0` with `target_still_set=true`). Removing it at the inner guard lets processInteraction's outer canAccess gate be the authoritative barrier.

**If strict TS parity is preferred:** keep the `!p.CanAccess()` arm in `tryInteract` AND ensure both call sites at `interaction.go:249,269` are behind the new outer gates. The inner guard then becomes a defence-in-depth that matches TS L1114.

---

## 6. Production call sites + test call sites affected by gate shift

**Production call sites:**

| Site | Current behavior | After patch |
|------|-----------------|-------------|
| `interaction.go:249` (pre-step) | Unconditional | Gated by `p.target != nil && p.CanAccess()` |
| `interaction.go:269` (post-step) | Gated by `p.target != nil && !followOp` | Gated by `p.target != nil && p.CanAccess() && !followOp` |

**Test call sites requiring update:**

| Test | File | Change needed |
|------|------|--------------|
| `TestTryInteract_Delayed_ShortCircuits` | `interaction_tryinteract_guard_test.go:69` | If inner `!CanAccess()` guard removed: test must call `p.processInteraction()` instead, or be reframed as a processInteraction-level test. If inner guard kept: no change. |
| `TestTryInteract_FollowOpDelayed_BothGatesGuard` | `interaction_tryinteract_guard_test.go:100` | Rationale comment becomes stale (returns false via `!HasInteraction()`, not `!CanAccess()`). Test passes regardless; update comment only. |

All other `tryInteract` test call sites are unaffected (CanAccess=true at call time).
