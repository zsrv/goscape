---
status: brainstorm-approved
date: 2026-05-11
ts_source:
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1045-1048 (CLEARQUEUE)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:903-920 (GETQUEUE)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:381-383 (P_OPHELD)
  - LostCityRS/Engine-TS/src/engine/entity/Player.ts:833-852 (unlinkQueuedScript)
---

# NAI-161 — Trivial-handler sweep #3 (3 ops; queue-introspection cohort + P_OPHELD stub)

**Cadence:** ~30 LOC code + ~80 LOC tests = ~110 LOC. Sits in the 100-300 LOC band — separate spec + plan, single combined Sonnet reviewer at end-of-impl. Subagent-driven-development per `execution_mode_default.md`.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Cascade-tail context:** missing-handler audit at HEAD `06d98a0` reports **21 unhandled opcodes** (`missing_handler_audit.md` one-liner). This sub-spec ports 3; remaining 18 stay forward-routable to NAI-162+.

---

## §1 Symptom / motivation

NAI-160 shipped 7 handlers and effectively exhausted the strict "single-statement TS body + existing backing infra + ≥1 content callers" cohort. The remaining 21 ops each need at least some new backing surface. This sub-spec extends NAI-160's cohort shape by tolerating **minimal new infra** (≤10 LOC per method) on `*Player`, and picks the 3 ops where the infra is mechanical and the content surface is hottest.

**Cohort (3 ops):**

| Op | Opcode | Content callers | TS body | TS file:line |
|---|---|---|---|---|
| `OpClearQueue` | 2011 | 42 | `state.activePlayer.unlinkQueuedScript(state.popInt())` | PlayerOps.ts:1045-1048 |
| `OpGetQueue` | 2021 | 16 | `popInt → for(req of activePlayer.queue.all()) count++ + for(req of activePlayer.weakQueue.all()) count++; pushInt(count)` | PlayerOps.ts:903-920 |
| `OpPOpHeld` | 2076 | 0 actual usage (only command def at `Content/scripts/engine.rs2:176`) | `checkedHandler(ProtectedActivePlayer, () => { throw new Error('unimplemented'); })` | PlayerOps.ts:381-383 |

CLEARQUEUE is the hottest unhandled op by content-caller density after NAI-160's SAY landed. Its content callers include minigame state-machine teardowns (`game_duelarena/duel_arena_finish.rs2`, `game_trawler/trawler_sink.rs2`, `game_trawler/trawler_win.rs2`) — unhandled CLEARQUEUE means scripted state cleanup at minigame exit silently leaks queued scripts. GETQUEUE is the natural cohort sibling (queue-introspection write+read pair) and serves shop/tutorial scripts. P_OPHELD is a TS-fidelity freebie: TS itself throws `'unimplemented'`, so a Go-side `fmt.Errorf("P_OPHELD: unimplemented")` matches the TS surface 1-for-1.

**Three filters applied (relaxed-NAI-160):**
1. **Backing infra is mechanical** — `(*Player).queue` field exists; precedent filter pattern `clearWeakQueue` at `player_script.go:95-103` ports directly.
2. **Single-statement TS handler body** — each handler is ≤3 lines (popInt + delegate; the for-loop in GETQUEUE lives inside the new `QueueCount` method).
3. **Active content callers ≥1** OR **TS-fidelity stub** — CLEARQUEUE/GETQUEUE clear filter #3; P_OPHELD admitted as TS-faithful stub (converts current `no handler for P_OPHELD (opcode 2076)` WARN to TS-canonical `'unimplemented'` error).

**Re-confirmed deferred at HEAD** (no change vs NAI-160 §1):

| Op | Defer reason (re-confirmed) |
|---|---|
| `OpNpcStatHeal` | Non-trivial arithmetic + needs `HeroPoints.Clear()` (modules/world/heropoints.go has `AddHero`/`TopContributor` only). |
| `OpPLocMerge` | Calls `World.mergeLoc` — not present in goscape. NAI-160's `UnsetMapFlag` shipping is **not** a P_LOCMERGE unblocker: TS body at `PlayerOps.ts:922-929` does NOT call `unsetMapFlag` (NAI-160 spec §1 incorrectly listed it as a dep). |
| `OpLcOp` / `OpOcOp` / `OpOcIop` | 0 content callers each; interaction-trigger plumbing audit pending. |
| `OpPushVarbit` / `OpPopVarbit` | Opcodes 25 and 27 — inline-bytecode core ops handled in ScriptRunner, not user-handler ports. Distinct cohort. |
| `OpLineOfSight` | 0 content callers; needs LoS clip-flag traversal infra. |
| `OpNpcAdd` (187 callers — very hot) / `OpNpcHunt` (6) | Spawn/aggro mechanics; not 1-line ports. Discrete cohort. |
| `OpMapIndoors` / `OpLastLoginInfo` / `OpSetGender` / `OpWealthEvent` | NAI-149-deferred; re-grepped at HEAD — blockers unchanged: needs `isIndoors()` helper / new `LastLoginInfo` packet+`lastLoginTime` field / `MALE_FEMALE_MAP` mapping table / `addWealthEvent` RPC payload. |
| `OpBothDropSlot` / `OpInvDropAll` / `OpInvTotalParamStack` | Each needs new Obj timed-drop surface or new ActivePlayer method (0 content callers for INV_TOTALPARAM_STACK). Discrete cohort. |
| `OpPOpPlayerT` | Tracked at NAI-162 — needs 4-arg `SetInteractionScriptPlayer` overload with spell payload. |

---

## §2 Architecture

### §2.1 New `(*Player)` methods (`modules/world/player_script.go`)

Both methods follow the existing defensive pattern from `EnqueueScriptArgs` (`player_script.go:127`): nil-guard `p.client / server / scriptProvider`, resolve `scriptProvider.GetByID(scriptID)` once, then filter `p.queue` via pointer-equality against the resolved `*ScriptFile`.

```go
// UnlinkQueuedScript removes every p.queue entry whose Script resolves
// to the script at scriptID (default-NORMAL TS arm). Walks the entire
// p.queue regardless of Type discriminator — this matches TS
// unlinkQueuedScript's default branch which walks both `queue` and
// `weakQueue` (Player.ts:843-851). p.engineQueue is intentionally
// untouched: TS gates engineQueue iteration behind type=ENGINE, which
// CLEARQUEUE never passes.
//
// No-op when scriptID does not resolve to a registered script
// (zero possible matches — TS finds zero in the same scenario; goscape
// matches by `req.Script == target` pointer-equality after a single
// provider lookup).
//
// (goscape defensive; TS skips this check) The nil-server guard mirrors
// EnqueueScriptArgs at player_script.go:127 — load-bearing for test
// fixtures that don't wire a Server.
//
// Mirrors TS Player.unlinkQueuedScript(scriptId, type=NORMAL) at
// Engine-TS/src/engine/entity/Player.ts:833-852.
func (p *Player) UnlinkQueuedScript(scriptID int) {
    if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
        return
    }
    target := p.client.server.scriptProvider.GetByID(uint32(scriptID))
    if target == nil {
        return
    }
    out := p.queue[:0]
    for _, req := range p.queue {
        if req.Script != target {
            out = append(out, req)
        }
    }
    p.queue = out
}

// QueueCount returns the number of p.queue entries whose Script
// resolves to the script at scriptID. Mirrors TS GETQUEUE at
// PlayerOps.ts:903-920 which walks BOTH `state.activePlayer.queue.all()`
// AND `state.activePlayer.weakQueue.all()`, counting any match in either.
// Goscape's unified p.queue holds Normal/Strong/Long/Weak entries, so a
// single loop covers both TS queues. p.engineQueue is a separate slice
// and is intentionally excluded.
//
// (goscape defensive; TS skips this check) See UnlinkQueuedScript.
func (p *Player) QueueCount(scriptID int) int {
    if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
        return 0
    }
    target := p.client.server.scriptProvider.GetByID(uint32(scriptID))
    if target == nil {
        return 0
    }
    n := 0
    for _, req := range p.queue {
        if req.Script == target {
            n++
        }
    }
    return n
}

// Plan-author erratum: original code block above had `Type != QueueWeak`
// filter; re-reading PlayerOps.ts:903-920 shows TS walks BOTH queue AND
// weakQueue. Filter removed; expected count 3→4 in §5 QueueCount test #1.
```

### §2.2 ActivePlayer interface widening (`pkg/script/active.go`)

Two new methods on the `ActivePlayer` interface:

```go
// UnlinkQueuedScript drops queued fresh-run requests whose script
// resolves to scriptID. Mirrors TS Player.unlinkQueuedScript with the
// default NORMAL arm (queue + weakQueue; engineQueue untouched).
UnlinkQueuedScript(scriptID int)

// QueueCount returns the count of queued requests (Normal/Strong/Long/Weak)
// whose script resolves to scriptID. Mirrors TS GETQUEUE iteration over
// queue.all() + weakQueue.all() (PlayerOps.ts:903-920).
QueueCount(scriptID int) int
```

### §2.3 Handlers (`pkg/script/handlers_player.go`)

```go
// handleClearQueue — OpClearQueue (2011).
// TS PlayerOps.ts:1045-1048:
//   const scriptId = state.popInt();
//   state.activePlayer.unlinkQueuedScript(scriptId);
func handleClearQueue(s *ScriptState) error {
    if err := requireActivePlayer(s, "CLEARQUEUE"); err != nil {
        return err
    }
    s.Self.UnlinkQueuedScript(s.PopInt())
    return nil
}

// handleGetQueue — OpGetQueue (2021).
// TS PlayerOps.ts:903-920:
//   const scriptId = state.popInt();
//   let count = 0;
//   for (const request of state.activePlayer.queue.all()) {
//     if (request.script.id === scriptId) { count++; }
//   }
//   for (const request of state.activePlayer.weakQueue.all()) {
//     if (request.script.id === scriptId) { count++; }
//   }
//   state.pushInt(count);
//
// Both loops live inside (*Player).QueueCount per §2.1 — handler
// is a 3-line popInt → delegate → pushInt.
func handleGetQueue(s *ScriptState) error {
    if err := requireActivePlayer(s, "GETQUEUE"); err != nil {
        return err
    }
    s.PushInt(s.Self.QueueCount(s.PopInt()))
    return nil
}

// handlePOpHeld — OpPOpHeld (2076).
// TS PlayerOps.ts:381-383:
//   checkedHandler(ProtectedActivePlayer, () => {
//     throw new Error('unimplemented');
//   });
//
// TS-faithful stub: protected-gate check fires first, then returns an
// 'unimplemented' error. Stub remains until OPHELD trigger plumbing is
// ported (separate cohort with OcOp/LcOp/OcIop).
func handlePOpHeld(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "P_OPHELD"); err != nil {
        return err
    }
    return fmt.Errorf("P_OPHELD: unimplemented")
}
```

### §2.4 Handler registration

3 lines added to the `handlers` map in `pkg/script/handlers.go`. Plan-author confirms numeric ordering by reading the existing map header and matches it:

```go
OpClearQueue: handleClearQueue,
OpGetQueue:   handleGetQueue,
OpPOpHeld:    handlePOpHeld,
```

### §2.5 Test mock additions

`pkg/script/runner_test.go` `mockPlayer`:

```go
// unlinkScriptCalls records every UnlinkQueuedScript call's scriptID.
unlinkScriptCalls []int

// queueCountByScript maps scriptID → return value for QueueCount.
// Unset entries return 0 (zero-value).
queueCountByScript map[int]int

func (m *mockPlayer) UnlinkQueuedScript(scriptID int) {
    m.unlinkScriptCalls = append(m.unlinkScriptCalls, scriptID)
}

func (m *mockPlayer) QueueCount(scriptID int) int {
    return m.queueCountByScript[scriptID]
}
```

Per `mock_recorder_field_naming_check.md`, plan-author greps the actual `mockPlayer` struct shape and matches existing recorder naming. Field names above are tentative — plan-author adjusts to match the dominant convention if it differs.

---

## §3 Deviation register

| Tag | Site | Deviation | Rationale |
|---|---|---|---|
| `NAI-161-D-QUEUE-TYPE-MAPPING` | `(*Player).UnlinkQueuedScript` / `QueueCount` | TS data model: 3 separate LinkLists (`queue`, `weakQueue`, `engineQueue`). Goscape: unified `p.queue` with `Type` discriminator + separate `p.engineQueue`. Both `UnlinkQueuedScript` and `QueueCount` walk the entire `p.queue` (Normal/Strong/Long/Weak — matches TS `queue ∪ weakQueue`). TS GETQUEUE (PlayerOps.ts:903-920) iterates BOTH `queue.all()` AND `weakQueue.all()`; goscape's single `p.queue` loop is equivalent. | TS-faithful at semantic level. Goscape's discriminator-based queue unification was set at queue-system introduction (NAI-144 era). Doc-comments on both methods cite TS line + the mapping rule. No follow-up. |
| `NAI-161-D-CLEARQUEUE-NIL-PROVIDER` | `(*Player).UnlinkQueuedScript` / `QueueCount` | Defensive early-return when `p.client / server / scriptProvider` is nil. TS has no such guard. | Per `defensive_gate_doc_comment_label.md` — load-bearing for test fixtures that don't wire a Server (mirrors `EnqueueScriptArgs` at `player_script.go:127`). Labeled "(goscape defensive; TS skips this check)". No follow-up. |
| `NAI-161-D-CLEARQUEUE-RESOLVE-FIRST` | `(*Player).UnlinkQueuedScript` / `QueueCount` | Goscape resolves `scriptProvider.GetByID(scriptID)` once and pointer-equality compares `req.Script == target`. TS compares `request.script.id === scriptId` per-iteration. | Same set semantics via goscape's by-index storage model — `playerQueueRequest.Script` is a `*ScriptFile`; no `ScriptID` field exists on the struct. Alternative (adding a `ScriptID` field) would force schema change + retrofit at 4 `EnqueueScriptFile` callers in `player_zone_triggers.go`, none of which carry a scriptID (trigger-resolved via `GetByTrigger`). No follow-up. |
| `NAI-161-D-POPHELD-STUB` | `handlePOpHeld` | TS literally throws `Error('unimplemented')`; goscape returns `fmt.Errorf("P_OPHELD: unimplemented")`. | TS-PARITY STUB (final) — both raise at the same point; only the surface differs. **The previously-cited "OPHELD trigger plumbing cohort with OcOp/LcOp/OcIop" does not exist in TS** (OPHELD1..5/U/T triggers fire from inv-click *packet* handlers, not from this script opcode — TS confirms by stubbing it). Re-port only if upstream TS lands a real body. See memory `nai164_declined_cohort.md` (2026-05-11 NAI-164-declined audit). |

---

## §4 Risk register

| Tag | Risk | Likelihood | Mitigation |
|---|---|---|---|
| `R1 [med]` | Pointer-equality vs ID-equality semantic drift: if `scriptProvider.scripts` is ever rebuilt in place between enqueue and CLEARQUEUE/GETQUEUE (e.g., cache hot-reload), enqueued requests would silently stop matching. | Low at HEAD | Goscape `Provider.scripts` is built once at module init. Plan-author pre-flight: `rg "p\.scripts\s*=" pkg/script/provider.go` to confirm no in-place rebuild. Add invariant note to doc-comments on both methods. |
| `R2 [low]` | `requireProtectedActivePlayer` signature drift. | Resolved | Already exercised by NAI-160's `handlePExactMove` and NAI-141's `handlePTeleport`. Plan-author re-greps `requireProtectedActivePlayer\b` for current signature. |
| `R3 [low]` | `out := p.queue[:0]` aliasing in `UnlinkQueuedScript`. | Low | Mirror `clearWeakQueue` precedent at `player_script.go:95-103` verbatim. Test §5 #3 (enqueue 3 entries, unlink middle, assert remaining order preserved) pins the filter shape. |
| `R4 [low]` | `mockPlayer` (runner_test.go:99) lacks the two new methods — interface gap will fail compile in the package's mock-consumer tests. | Low | Plan task adds them. Per `mock_recorder_field_naming_check.md`, plan-author greps the actual `mockPlayer` struct and matches the dominant recorder convention. |
| `R5 [low]` | P_OPHELD gate-ordering: protected-gate check must precede the `'unimplemented'` error. | Low | Codified in §2.3 — `requireProtectedActivePlayer` returns first. Test §5 (P_OPHELD #2) pins this with a non-protected fixture expecting `"P_OPHELD: script not protected"`. |
| `R6 [low]` | NPC-side mock or other `ActivePlayer` impls beyond `mockPlayer` and `*Player` could exist and break compile. | Low | Plan-author greps `pkg/script` for `ActivePlayer` impls. NAI-160 spec §4 R7 confirmed only `mockPlayer` (`runner_test.go:99`) and `*Player` (`modules/world`) — re-verified. |
| `R7 [low]` | `p.queue` slice aliasing in tests — `out := p.queue[:0]` shares backing array with the original slice; if tests assert the original slice content after `UnlinkQueuedScript`, they'd see mutated entries. | Low | Test assertions read `p.queue` (the post-mutation field), not a pre-call snapshot. Mirrors `clearWeakQueue` test patterns. |

---

## §5 Test strategy

**Handler tests** (`pkg/script/handlers_player_test.go`):

1. **CLEARQUEUE positive** (mirrors `TestSetTimer` template): Build `ScriptFile` with `OpPushConstantInt(42) + OpClearQueue + OpReturn`. `Init(sf, mockPlayer{}, false, mockConfigs, nil) → Pointers |= PtrActivePlayer → Execute`. Assert `mockPlayer.unlinkScriptCalls == []int{42}`.
2. **CLEARQUEUE no-active-player**: Omit `PtrActivePlayer` → expect error `"CLEARQUEUE: no active player"`. Folded into the existing `TestHandlersRequireActivePlayer` table at `handlers_player_test.go:831`.
3. **GETQUEUE positive**: `mockPlayer.queueCountByScript = map[int]int{7: 3}`. Build `OpPushConstantInt(7) + OpGetQueue + OpReturn`. After `Execute`, assert `state.PopInt() == 3`.
4. **GETQUEUE no-match returns zero**: `queueCountByScript` empty, push `99`, run GETQUEUE → assert `state.PopInt() == 0`.
5. **GETQUEUE no-active-player**: Folded into `TestHandlersRequireActivePlayer` table.
6. **P_OPHELD returns unimplemented**: Build `OpPOpHeld + OpReturn` with `Pointers |= PtrActivePlayer | PtrProtectedActivePlayer`. `Execute` → expect error containing `"P_OPHELD: unimplemented"`.
7. **P_OPHELD not-protected gate**: Set `PtrActivePlayer` only → expect error `"P_OPHELD: script not protected"`. Precedence: protected gate fires before unimplemented stub. Folded into the existing `TestHandlersRequireProtectedActivePlayer` table (if present; if not, plan-author adds a parallel table).

**Unit tests for `(*Player).UnlinkQueuedScript`** (`modules/world/player_script_test.go`):

1. **3-entry queue, unlink middle by ID**: Register 3 scripts at IDs 10/20/30, enqueue all three (Type=QueueNormal). Call `UnlinkQueuedScript(20)`. Assert `len(p.queue) == 2` and remaining `Script` pointers are the ID=10 and ID=30 scripts in original order.
2. **Mixed-Type queue, unlink walks all non-engine**: Register script at ID=10. Enqueue 3 entries all `Script=sf10` with Types `[Normal, Weak, Strong]`. Call `UnlinkQueuedScript(10)`. Assert `len(p.queue) == 0`. Pins TS-faithful "walks queue + weakQueue" mapping (deviation `NAI-161-D-QUEUE-TYPE-MAPPING`).
3. **EngineQueue untouched**: Enqueue `id=10` to `p.queue` (Normal) and `p.engineQueue` (Engine). Call `UnlinkQueuedScript(10)`. Assert `len(p.queue) == 0` AND `len(p.engineQueue) == 1`.
4. **Nil-server defensive no-op**: Fresh `*Player{}` (no `client`) → `UnlinkQueuedScript(99)` doesn't panic; `p.queue` (nil/empty) unchanged.
5. **Bogus scriptID is no-op**: Register scripts at IDs 10/11, enqueue both. Call `UnlinkQueuedScript(99)` (out-of-range). Assert `len(p.queue) == 2`.

**Unit tests for `(*Player).QueueCount`** (`modules/world/player_script_test.go`):

1. **Counts all queue types including Weak**: Register script at ID=10. Enqueue 4 entries all `Script=sf10` with Types `[Normal, Strong, Long, Weak]`. Call `QueueCount(10)` → returns `4`. Pins TS-faithful `queue.all() + weakQueue.all()` semantics (PlayerOps.ts:903-920).
2. **EngineQueue excluded**: Enqueue `id=10` to both `p.queue` (Normal) and `p.engineQueue` (Engine). `QueueCount(10)` → returns `1`.
3. **Bogus scriptID returns 0**: Enqueue `id=10`. `QueueCount(99)` → returns `0`.
4. **Nil-server returns 0**: Fresh `*Player{}` → `QueueCount(99) == 0` and doesn't panic.

---

## §6 Smoke binding

**Smoke target (binding):** Tutorial Island bury-bone step (`Content/scripts/tutorial/scripts/skills/tut_bury_bone.rs2`) — uses `getqueue` to gate "is the queued bury-bone event still pending?" logic.

Expected smoke signal: post-NAI-161, `no handler for GETQUEUE (opcode 2021)` WARN no longer appears in server logs during the bury-bone step. Plan-author handoff to user per `smoke_test_server_handoff.md`.

**Fallback smoke targets** (if bury-bone step unreachable in current setup): NPC-shop interaction at any of `Content/scripts/areas/area_varrock/scripts/tea_seller.rs2`, `area_ardougne_east/spice_seller.rs2`, `silver_merchant.rs2`. Walking up to and clicking the shop NPC should fire the shop's GETQUEUE gate.

**`clearqueue` smoke** is **non-binding** for this sub-spec: its content callers (minigame teardowns — `game_duelarena/duel_arena_finish.rs2`, `game_trawler/trawler_sink.rs2`) require traveling deep into minigame state, which is impractical in a fresh smoke session. CLEARQUEUE is **unit-pinned** by §5.

**P_OPHELD** is unit-pinned (no real content callers — only the command definition at `Content/scripts/engine.rs2:176`). No smoke required.

Per `cascade_theory_smoke_binding.md`, GETQUEUE-bubble visibility (or, more precisely, the absence of the GETQUEUE WARN during the bury-bone tutorial step) is the binding cascade-attribution signal for this sub-spec.

---

## §7 Cadence routing

Standard cadence per `runescript_cadence.md`:

1. **Spec** (this doc) → user review gate.
2. **Plan** (writing-plans skill, ~5 tasks: 2 `(*Player)` methods + 3 handlers + ActivePlayer interface widening + mock recorder additions) → user review gate.
3. **`/clear`** between plan and impl per `superpowers_clear_between_spec_and_impl.md`.
4. **Implementation** (subagent-driven-development per `execution_mode_default.md`).
5. **Single combined Sonnet** `superpowers:code-reviewer` at end-of-impl per `superpowers_code_reviewer_model.md`.
6. **Smoke handoff** to user for the GETQUEUE-binding signal.
7. **Close commit** with `Closes memory:` trailer per `close_commit_memory_trailer.md`.

---

## §8 Out of scope

- The remaining 18 unhandled opcodes (21 → 18 cascade-tail remainder after this sub-spec lands). All forward-routable per §1 defer table.
- `(*Player).UnlinkQueuedScript` ENGINE-type overload (TS exposes `unlinkQueuedScript(id, type=ENGINE)` walking `engineQueue`). Goscape's CLEARQUEUE handler uses default-NORMAL; no current caller needs the ENGINE branch. Add when a future op needs it.
- P_OPHELD trigger plumbing — separate cohort with OcOp/LcOp/OcIop interaction-trigger work. The stub remains until that cohort lands.
- `unlinkQueuedScript`-cascade content audits (minigame teardown side effects). Smoke surfaces these as NAI-N+1 if observed.
- Refactoring the `missing_handler_audit.md` one-liner — orthogonal.
- Any change to `playerQueueRequest` schema (no `ScriptID` field added — pointer-equality is sufficient per `NAI-161-D-CLEARQUEUE-RESOLVE-FIRST`).

---

## §9 Memory hits

Cited in close-commit `Closes memory:` trailer per `close_commit_memory_trailer.md`:

- `runescript_cadence.md` — full cadence routing
- `controller_preflight.md` — preflight performed before brainstorm (audit one-liner re-run at HEAD)
- `missing_handler_audit.md` — 21-opcode cascade-tail measurement; defer rationale per op
- `audit_full_method_against_ts.md` — each TS body audited line-by-line; queue-type-mapping pinned as deviation
- `plan_grep_helper_patterns.md` / `plan_sibling_site_guard_audit.md` — `requireActivePlayer` / `requireProtectedActivePlayer` / nil-server-guard precedent reuse
- `mock_recorder_field_naming_check.md` — mock fields grep-verified at plan-write
- `defensive_gate_doc_comment_label.md` — nil-server guard labeled
- `true_to_ts_gate.md` — 4 deviations tracked with rationale + follow-up disposition
- `spec_ts_source_read.md` — every TS body read verbatim (no analogy)
- `smoke_test_server_handoff.md` / `cascade_theory_smoke_binding.md` — GETQUEUE-bubble binding signal at Tutorial Island bury-bone step
- `enumerate_all_sites.md` — handler registration sites enumerated (single `handlers` map; 3 lines)
- `superpowers_clear_between_spec_and_impl.md` — `/clear` between plan and impl
- `superpowers_code_reviewer_model.md` — single Sonnet reviewer at end-of-impl
- `execution_mode_default.md` — subagent-driven-development

---

## §10 No-deviations audit

Per `spec_ts_init_value_audit.md` and `spec_diagram_order_divergence.md`, this spec was authored from TS source line reads (not from neighbor-handler analogy):

- **CLEARQUEUE**: `PlayerOps.ts:1045-1048` — 2 lines. `Player.unlinkQueuedScript(id, type=NORMAL)` body at `Player.ts:833-852` read verbatim — default branch walks both `queue` and `weakQueue`. Goscape mapping codified in deviation `NAI-161-D-QUEUE-TYPE-MAPPING`.
- **GETQUEUE**: `PlayerOps.ts:903-920` — walks BOTH `state.activePlayer.queue.all()` AND `state.activePlayer.weakQueue.all()`; counts matches in either. Goscape's unified `p.queue` (Normal/Strong/Long/Weak) covers both; no Type filter. p.engineQueue excluded. (Plan-author erratum: original spec stated `queue.all()` only; re-read of PlayerOps.ts:903-920 corrects this.)
- **P_OPHELD**: `PlayerOps.ts:381-383` — `checkedHandler(ProtectedActivePlayer, () => { throw new Error('unimplemented'); })`. Two gates: protected check + unimplemented body. Goscape preserves both; gate ordering pinned by §5 test #7.

No init-value, diagram-order, or asymmetric-predicate divergences detected.
