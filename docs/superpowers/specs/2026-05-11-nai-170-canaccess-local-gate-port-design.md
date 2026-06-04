# NAI-170 — `pathToPathingTarget` local-gate CanAccess port (compressed cadence)

**Status:** spec written 2026-05-11. Compressed cadence — single combined spec+plan doc + Stage 1 audit + Stage 2 fix in one cycle. TDD pair + close.

**Tech stack:** Go 1.26+ (per `go_version` memory).

**Lineage:** Retires `NAI-169-FU-CANACCESS-LOCAL-GATE-NARROWING` (seeded by NAI-169 fixup commit `b70dfbe` after a missed grep surfaced the divergence). Stage 1 audit: this doc §3. Stage 2 fix: §4-§5.

## 1. Goal

Port TS `Player.pathToPathingTarget`'s `!canAccess()` gate (Player.ts:1044-1046) to goscape's `(*Player).pathToPathingTarget` at `modules/world/interaction.go:789-795`. The inline goscape gate `p.delayed || p.protectedScriptActive()` is narrower than TS — TS `canAccess()` is `!protect && !busy()` where `busy() = delayed || containsModalInterface()`. The missing modal arm lets modal-open players queue pathing waypoints when TS would skip. PRIMARY: gate matches TS exactly; the canonical `(*Player).CanAccess()` (player_script.go:390-401) is invoked instead of the inlined narrow predicate.

## 2. TS source of truth

`/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts`:

| Line | Code | Bears on |
|---|---|---|
| 796-799 | `containsModalInterface() { return (this.modalState & (ModalState.MAIN \| ModalState.CHAT)) !== ModalState.NONE; }` | Defines TS modal predicate. |
| 801-803 | `busy() { return this.delayed \|\| this.containsModalInterface(); }` | TS busy. |
| 805-812 | `canAccess() { if (World.shutdown) return true; else return !this.protect && !this.busy(); }` | TS canAccess — the gate `pathToPathingTarget` calls. |
| 1044-1046 | `if (!this.canAccess()) { return; }` | The exact gate goscape must mirror. |

## 3. Goscape state at HEAD `b70dfbe` (Stage 1 audit)

### 3.1 The divergent gate

`modules/world/interaction.go:789-795`:
```go
if p.delayed || p.protectedScriptActive() {
    // canAccess gate (TS L1044-1046). Narrower than TS canAccess
    // (Player.ts:801-812 also tests !containsModalInterface); see
    // NAI-169-FU-CANACCESS-LOCAL-GATE-NARROWING.
    return
}
```

The local predicate omits the `containsModalInterface` arm. `(*Player).CanAccess()` at `player_script.go:390-401`:
```go
func (p *Player) CanAccess() bool {
    if p.delayed { return false }
    if p.modalState&(modalStateMain|modalStateChat) != 0 { return false }
    if p.protectedScriptActive() { return false }
    return true
}
```

`!p.CanAccess()` = `p.delayed || p.modalState&(Main|Chat) != 0 || p.protectedScriptActive()`. Identical to the local gate **plus** the modal check.

### 3.2 Origin

Introduced by NAI-98 Phase 2 (commit `2b047bc`, "fix(world): NAI-98 Phase 2 — port TS Player.pathToPathingTarget gate"). The deviation tag (`NAI-44-D-CANACCESS-NO-STUN-CHECK`) attached at landing time was already misframed — TS canAccess has never tested stun. NAI-169 retired the tag but discovered the genuine TS-fidelity gap underneath (modal arm missing).

### 3.3 Sibling-site enumeration

`rg "p\.delayed.*p\.protectedScriptActive" modules/ pkg/` at HEAD returns exactly 2 production hits:

| Site | Function | TS counterpart | Status |
|---|---|---|---|
| `interaction.go:344` | `processWalktrigger` | TS Player.ts:1062 `if (this.walktrigger !== -1 && !this.protect && !this.delayed)` | **TS-faithful** — TS L1062 also omits modal check at that specific call site. No fix needed. |
| `interaction.go:789-795` | `pathToPathingTarget` | TS Player.ts:1044 `if (!this.canAccess()) return;` | **TS-divergent** — TS calls full canAccess (which includes modal). **Fix target.** |

No other sibling sites exist. Bounded scope.

### 3.4 Production caller graph

`rg "\.pathToPathingTarget\(" modules/ pkg/` (non-test) returns 1 caller:

- `interaction.go:262` — `processInteraction` post-step branch (TS L1228-1229 mirror).

Inside `processInteraction`, the surrounding gates use full `p.CanAccess()` at L247, L261, L277 (TS L1210/L1232/L1244 mirrors). When a modal-open player enters `processInteraction`:
- L200-202 tick-math entry guard: skips entire function only if delayed *and* within delay window. Modal-only player passes through.
- L247 pre-step interact: skipped (CanAccess gate fires).
- L256 `!interacted`: enters post-step block.
- **L262 `p.pathToPathingTarget()` runs**. Pre-fix: narrow gate at L791 passes (modal-only) → function continues to queueWaypoint/pathToTarget. Post-fix: full-CanAccess gate at L791 catches → function returns.
- L261 walktrigger: skipped (CanAccess gate fires).
- L277 post-step interact: skipped (CanAccess gate fires).

The pathing-queue side-effect at L262 is the **only** observable divergence today for a modal-only player. All other interaction work is already correctly gated by NAI-155's `p.CanAccess()` calls.

### 3.5 Test coverage gap

`interaction_canaccess_gate_test.go:18-39` — `TestProcessInteraction_CanAccessGate_ModalChat_PreservesInteraction` pins that modal-Chat doesn't trigger ClearInteraction (TS L1244 fidelity). It does NOT assert anything about waypoints / pathing queue. The gap: pathToPathingTarget's modal-divergent behavior is unpinned.

### 3.6 Stage 1 verdict

Real but minor TS-fidelity gap. Symptom: a player with Main/Chat modal open AND a PathingEntity target gets a pathing queue installed; their character may begin walking on the client during the dialog modal until the modal closes or pathing exhausts. Not a crash, not a security issue. The fix is one-line and behaviorally aligned with TS. Safe to proceed to Stage 2.

## 4. Production change (1 LOC swap + comment update)

Replace `modules/world/interaction.go:789-795`:

**Before:**
```go
if p.delayed || p.protectedScriptActive() {
    // canAccess gate (TS L1044-1046). Narrower than TS canAccess
    // (Player.ts:801-812 also tests !containsModalInterface); see
    // NAI-169-FU-CANACCESS-LOCAL-GATE-NARROWING.
    return
}
```

**After:**
```go
if !p.CanAccess() {
    // canAccess gate (TS L1044-1046). Mirrors TS canAccess
    // (Player.ts:805-812: !protect && !busy(); busy includes
    // !containsModalInterface). NAI-170.
    return
}
```

Also update the narrative doc-comment at L754-758:

**Before:**
```go
//   - !canAccess: no-op (TS L1044-1046). Goscape uses the narrower local
//     predicate !p.delayed && !p.protectedScriptActive(); TS canAccess
//     additionally tests !containsModalInterface (Player.ts:801-812). The
//     missing modal arm is a real TS-fidelity gap pending investigation —
//     see NAI-169-FU-CANACCESS-LOCAL-GATE-NARROWING.
```

**After:**
```go
//   - !canAccess: no-op (TS L1044-1046). Gated on full p.CanAccess() —
//     delayed + modal-Main/Chat + protectedScriptActive (player_script.go:390).
```

## 5. Test pin (1 new pin in `modules/world/interaction_canaccess_gate_test.go`)

```go
// TestPathToPathingTarget_ModalChat_SkipsPathing pins TS Player.ts:1044
// fidelity: when CanAccess()=false due to modalState&Chat (no delay, no
// protected script), pathToPathingTarget must skip the waypoint-queue
// arm entirely. Pre-NAI-170 the local gate p.delayed || p.protectedScriptActive()
// passed through modal-only players, leaking a path queue mid-dialog.
//
// Fixture mirrors TestProcessInteractionRepathsAfterPathExhaustion (H8):
// NodeClientRoutefinder=true + NPC at cheb=15 forces the pathToTarget
// arm to fire and queue waypoints when the gate passes.
func TestPathToPathingTarget_ModalChat_SkipsPathing(t *testing.T) {
    s := newTestServer(t)
    s.cfg.NodeClientRoutefinder = true
    npc := makeInteractionNpc(t, s, 1, 115, 100, 0) // cheb=15 from player

    p, cc := newTestPlayer(t)
    _ = cc
    p.client.server = s
    p.x, p.z, p.level = 100, 100, 0

    p.SetInteraction(InteractionEngine, npc, 1, -1)
    p.modalState = modalStateChat
    p.waypointIndex = -1

    if p.CanAccess() {
        t.Fatal("test setup invalid: CanAccess should be false with modalStateChat")
    }
    if p.delayed || p.protectedScriptActive() {
        t.Fatal("test setup invalid: only modal-Chat should be active; narrow local gate must pass pre-fix")
    }

    p.pathToPathingTarget()

    if p.waypointIndex >= 0 {
        t.Fatalf("modalChat player got waypoints queued (waypointIndex=%d); TS L1044 canAccess gate must skip pathing", p.waypointIndex)
    }
}
```

Placed adjacent to NAI-155's `TestProcessInteraction_CanAccessGate_ModalChat_PreservesInteraction` (line 18). Sibling style.

### 5.1 RED phase semantics

Before T2, the test should FAIL: `p.pathToPathingTarget()` proceeds past the narrow gate (modal-only player, no delay, no protect), the PathingEntity-target arm runs, NODE_CLIENT_ROUTEFINDER check finds no intersect at cheb=15, `isLastOrNoWaypoint` arm calls `p.pathToTarget()`, pathfinder queues waypoints. `waypointIndex` transitions from -1 to ≥0.

If the test fails to RED for fixture reasons (no pathfinder configured, NPC unreachable), the implementer adjusts coords or invokes the pathfinder-init helper used by `TestProcessInteractionRepathsAfterPathExhaustion`. The fixture is intentionally cloned from that working H8 test to minimize fixture risk.

### 5.2 GREEN phase

Post-T2, `!p.CanAccess()` returns true at the modal-Chat path, function returns at L791, `waypointIndex` stays -1.

## 6. Tests intentionally NOT included (with rationale)

| Skipped test | Rationale |
|---|---|
| Sibling test for modal-Main (not just modal-Chat) | `CanAccess` treats Main and Chat identically (`modalState&(Main\|Chat) != 0`); one pin covers both. |
| Pin for protected-script path | Already covered by the symmetric NAI-155 pattern in this file (`TestProcessInteraction_CanAccessGate_ProtectedScript_PreservesInteraction`). The gate's protectedScriptActive arm is unchanged by NAI-170. |
| Pin for delayed path | The L200-202 tick-math entry guard already short-circuits delayed-window players from reaching pathToPathingTarget. Test would never reach the L791 gate. |
| End-to-end pin via processInteraction (not direct call) | NAI-155's `TestProcessInteraction_CanAccessGate_ModalChat_PreservesInteraction` already exercises the path; adding a waypoint assertion to it would change the existing test's scope. Cleaner to add a focused direct-call pin. |

Per `helper_as_oracle_test_anti_pattern`: direct field assertion on `p.waypointIndex` (no helper-as-oracle).

## 7. Deviations expected

None. TS-faithful one-line gate swap.

## 8. Tracker retirement

`nai_followups.md` line ~6975 (NAI-169-FU-CANACCESS-LOCAL-GATE-NARROWING) struck through with `RETIRED 2026-05-11 by NAI-170` annotation. Close commit carries `Closes memory: NAI-169-FU-CANACCESS-LOCAL-GATE-NARROWING` trailer.

## 9. Risk register

| ID | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | A test fixture elsewhere relies on the pre-fix behavior (modal player gets path queue) | Low | `rg -n "modalState.*(Main\|Chat)|modalStateChat|modalStateMain" modules/world/*_test.go` enumerates ~50 test sites; spot-check the pathToPathingTarget-adjacent ones. Pre-flight grep confirms no test asserts modal+target=*PathingEntity → waypointIndex >= 0. |
| R2 | Real-world content opens modal-Main during walking and depends on path continuing | Low | Tutorial Island dialog content typically delays the script (=> goscape's tick-math guard catches it) or uses queue-driven dialogs (=> activeScript with PtrProtectedActivePlayer => protectedScriptActive() catches it). The pure "modal-only, no delay, no protected script" scenario is rare in well-formed content. Smoke (§14) validates. |
| R3 | `(*Player).CanAccess()` has subtle short-circuit semantics not equivalent to inlined OR | Trivial | Read of player_script.go:390-401 confirms it's a straight sequence of `if X return false` guards — short-circuit evaluates left-to-right, same as `delayed \|\| modal \|\| protectedScriptActive`. Identical to inline. |
| R4 | Adjacent test `TestProcessInteraction_CanAccessGate_Delayed_EarlyReturnsBeforePathing` expects the tick-math entry guard at L200-202 to fire before pathToPathingTarget; NAI-170 doesn't touch that pin | None | Pin is unaffected. Re-run confirms green. |
| R5 | TS `World.shutdown` arm of `canAccess()` (Player.ts:806-808 returns true unconditionally) — goscape `CanAccess()` lacks this | Low | NAI-170 inherits the pre-existing posture: shutdown is not handled at this layer. If the goscape shutdown sequence needs canAccess=true semantics elsewhere, open a separate NAI. Out of scope. |

## 10. Cadence + commits

Per `compressed_cadence` + `investigation_subspec_cadence`: combined Stage 1 audit + Stage 2 fix in one doc; single TDD commit pair + close.

| Step | Commit | Body |
|---|---|---|
| Spec | `docs(spec): NAI-170 — pathToPathingTarget canAccess local-gate port` | This file. |
| T1 RED | `test(world): NAI-170 T1 — pathToPathingTarget modal-Chat skip pin (RED)` | Adds `TestPathToPathingTarget_ModalChat_SkipsPathing` per §5. FAILs at this commit (waypointIndex transitions). |
| T2 GREEN | `fix(world): NAI-170 T2 — pathToPathingTarget gate uses full CanAccess (GREEN)` | Swaps L791 gate + updates comment per §4. T1 PASSes. |
| Close | `chore(close): NAI-170 — pathToPathingTarget canAccess local-gate port` | Empty marker; trailer `Closes memory: NAI-169-FU-CANACCESS-LOCAL-GATE-NARROWING`. |

## 11. Verification protocol (per `verification_before_completion`)

Pre-T1 baseline: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...` clean at HEAD `b70dfbe`.

Expected RED-phase outcome after T1 commit (gate still narrow):

| Pin | RED outcome | Notes |
|---|---|---|
| `TestPathToPathingTarget_ModalChat_SkipsPathing` | **FAIL** | `waypointIndex != -1` after the call: narrow gate let modal-only player through to pathToTarget. |

Post-T2 (after gate swap): pin PASSes. All other tests still PASS (no behavior change for non-modal players).

Verification: re-run `go test ./modules/world/...` AND `go test ./...` (broader regression). Both clean.

Per `verify_implementer_claims`: controller verifies fresh `git show <SHA>` post-T1 and post-T2 to confirm content matches stated diff. No reliance on stale IDE diagnostics.

## 12. Pattern memories applied

- `compressed_cadence` — combined spec+plan + Stage 1 audit + Stage 2 fix in one doc.
- `runescript_cadence` — preserved spec → impl → close phasing.
- `investigation_subspec_cadence` — Stage 1 audit produced §3 findings; Stage 2 fix (§4-§5) derived directly from audit conclusions; conditional Stage 3 (smoke) at §14.
- `controller_preflight` — pre-impl grep+Read pass against HEAD `b70dfbe` enumerated 2 sibling `delayed || protectedScriptActive` sites (1 divergent at L789, 1 TS-faithful at L344) and 1 production caller of `pathToPathingTarget` (`interaction.go:262`); re-grep at impl confirms.
- `tracker_entry_framing_can_be_incomplete` — NAI-44-D-CANACCESS-NO-STUN-CHECK was fact-correct on "goscape has no stun" but framing-wrong on "TS canAccess tests stun"; NAI-169 retired the misframing, NAI-170 closes the genuine underlying gap.
- `retire_deviation_grep_all_comments` — NAI-169 fixup commit `b70dfbe` swept missed doc-comment references after primary commit; NAI-170 builds on the cleaner state.
- `audit_full_method_against_ts` — TS Player.ts:1034-1055 read line-by-line for §3 audit; not just the L1044 line that brought us here.
- `helper_as_oracle_test_anti_pattern` — test §5 uses direct `p.waypointIndex` assertion; no helper-as-oracle.
- `close_commit_memory_trailer` — close commit carries `Closes memory:` trailer.
- `verify_implementer_claims` — `go test` and `git show` at each commit boundary.

## 13. Out of scope

- `processWalktrigger` (L344) gate: TS-faithful at TS L1062; NAI-170 leaves it.
- TS `canAccess` `World.shutdown` arm: not currently mirrored in goscape `CanAccess`; pre-existing posture, separate concern.
- Other inline `delayed`-only or `protectedScriptActive`-only gates elsewhere in `modules/world/`: orthogonal; if a similar audit finds another narrowed predicate, open a fresh NAI.
- NAI-59-D-MODALTUTORIAL-NO-PRODUCER and other active deviation tags: each warrants its own investigation per `tracker_entry_framing_can_be_incomplete`.

## 14. Smoke handoff (Stage 3 conditional)

Post-close smoke is RECOMMENDED but not gating — the unit pin is binding. Java-client scenario:

1. Walk to a chatnpc (e.g., Tutorial Island Survival Expert NPC #943).
2. Initiate dialog (modal-Main opens).
3. While modal is open, click on a distant NPC or coord.
4. **Pre-fix observation:** character may begin walking during dialog modal.
5. **Post-fix observation (TS-faithful):** character stays put; pathing queue not installed. Click is buffered or dropped depending on client-side modal handling.

If smoke surfaces any adjacent unhandled regression ≤30 LOC per `smoke_surfaces_adjacent_divergences`, route into NAI-170 in-scope-stretch; else open NAI-171 separately.
