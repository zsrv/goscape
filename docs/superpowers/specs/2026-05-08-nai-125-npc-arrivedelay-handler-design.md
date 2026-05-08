# NAI-125 — NPC_ARRIVEDELAY (opcode 2502) handler implementation

**Status:** spec — draft 1
**Date:** 2026-05-08
**Predecessor:** NAI-124 close (`7fc798c`); NAI-123 residual B (`[proc,npc_death]` server-log WARN at NAI-123 close `df95032`).
**Cadence:** subagent-driven-development — single bundle (additive port), Sonnet code-reviewer pass, user-launched smoke. Standard-shape spec, not investigation cadence.
**Tech stack:** Go 1.26+.

## §0 — One-line summary

Port TS `NpcOps.ts:542-555` `NPC_ARRIVEDELAY` (opcode 2502) into goscape. The opcode constant is reserved (`pkg/script/opcode.go:239`) but has no dispatch entry; ScriptRunner aborts `[proc,npc_death]` at pc=4 every NPC kill. Net surface: `ActiveNpc.LastMovement()` getter + handler + dispatch entry + `*Npc` impl + mockNpc plumbing + 7 tests.

## §1 — Symptom and binding evidence

**Smoke (NAI-123 close, `df95032`, 2026-05-07):**
- Tutorial Island fresh char kills giant rat (post-NAI-123 non-zero red hitsplats).
- Server log emits `WARN: script "[proc,npc_death]": no handler for NPC_ARRIVEDELAY (opcode 2502) at pc=4`.
- Whatever ops follow `NPC_ARRIVEDELAY` at pc≥5 in `npc_death` (loot drop / respawn timer / kill-count XP / etc.) are not running because `Execute` aborts at the first unknown opcode (`pkg/script/runner.go:55-77`).

**Verification at HEAD (`7fc798c`):**

```
$ rg "OpNpcArriveDelay" pkg/ modules/
pkg/script/opcode.go:239:	OpNpcArriveDelay       Opcode = 2502
pkg/script/opcode.go:877:	case OpNpcArriveDelay:
pkg/script/opcode.go:878:		return "NPC_ARRIVEDELAY"
```

No dispatch entry in `handlers.go:262-468`. No handler function. No reference in any test.

The producer-side field (`Npc.lastMovement`) is wired as of NAI-82 but currently unread:

```
$ rg "lastMovement" modules/world/
modules/world/npc.go:74:	// NAI-82: TS PathingEntity.lastMovement (Engine-TS/.../PathingEntity.ts:56).
modules/world/npc.go:75:	// Written to currentTick + 1 at end of updateMovement when position changed;
modules/world/npc.go:76:	// read by AI_ARRIVEDELAY / AI_TARGETMOVED (deferred — see NAI-82 spec §6.1).
modules/world/npc.go:77:	lastMovement int
modules/world/npc_interaction.go:334:		n.lastMovement = s.currentTick + 1
modules/world/npc_movement_test.go:27:	if n.lastMovement != 51 {
```

The `:76` "deferred" framing is stale — NAI-125 retires it.

## §2 — TS reference (verbatim)

`Engine-TS/src/engine/script/handlers/NpcOps.ts:542-555`:

```typescript
// https://x.com/JagexAsh/status/1432296606376906752
[ScriptOpcode.NPC_ARRIVEDELAY]: checkedHandler(ActiveNpc, state => {
    if (state.activeNpc.lastMovement < World.currentTick - 1) {
        return;
    }
    // If npc moved 1 tick ago, delay for 1 tick. If npc moved this tick, delay for 2 ticks
    state.activeNpc.delayed = true;
    if (state.activeNpc.lastMovement === World.currentTick - 1) {
        state.activeNpc.delayedUntil = World.currentTick + 1;
    } else {
        state.activeNpc.delayedUntil = World.currentTick + 2;
    }
    state.execution = ScriptState.NPC_SUSPENDED;
}),
```

**Three-tick acceptance window** (vs `P_ARRIVEDELAY`'s two-tick): given `lastMovement = T+1` is written at end of the tick the NPC stepped (TS `PathingEntity.ts:56`), and currentTick = T:
- `lastMovement = T+1` ("moved this tick"): gate `T+1 < T-1` = false → continue. Branch `T+1 === T-1` = false → `delayedUntil = T+2`.
- `lastMovement = T` ("moved last tick"): gate `T < T-1` = false → continue. Branch `T === T-1` = false → `delayedUntil = T+2`.
- `lastMovement = T-1` ("moved 2 ticks ago"): gate `T-1 < T-1` = false → continue. Branch `T-1 === T-1` = true → `delayedUntil = T+1`.
- `lastMovement = T-2` ("moved 3 ticks ago"): gate `T-2 < T-1` = true → no-op return.
- `lastMovement = 0` (never moved): gate `0 < T-1` = true → no-op return.

## §3 — Goscape mapping

### §3.1 — SetDelayed contract

Per `pkg/script/active.go:694-698` (the existing `ActiveNpc.SetDelayed(ticks int)` contract):

> SetDelayed marks the NPC as suspended for `ticks` more ticks starting next tick. Implementations compute `delayedUntil = currentTick + 1 + ticks`.

Translating TS branches:
- TS `delayedUntil = currentTick + 2` → goscape `SetDelayed(1)`.
- TS `delayedUntil = currentTick + 1` → goscape `SetDelayed(0)`.

Cross-checked against `handleNpcDelay` (NPC_DELAY-N, `handlers_npc.go:319-330`) which uses identical primitives: `s.ActiveNpc.SetDelayed(ticks)` + `s.Execution = NpcSuspended`.

### §3.2 — Pointer gate

TS uses `checkedHandler(ActiveNpc, ...)`. Goscape's analog is `requireActiveNpc(s, "NPC_ARRIVEDELAY")` (`handlers_npc.go:98`), which checks `s.Pointers & PtrActiveNpc != 0` and returns `errors.New("<OP>: no active npc")` otherwise. Used by every `handlers_npc.go` handler.

### §3.3 — World access

`s.World.CurrentTick()` is the canonical reader, used by `handlePArriveDelay` (`handlers.go:740-746`), `handleMapClock`, `handlePlayerCount`, etc. NAI-125 mirrors P_ARRIVEDELAY's defensive `s.World == nil` guard — see §6 deviation.

## §4 — Files touched

Five files. All changes additive; no existing code paths modified.

### §4.1 — `pkg/script/active.go` (interface)

Add `LastMovement() int` to the `ActiveNpc` interface, sibling to `ActivePlayer.LastMovement()`:

```go
// LastMovement returns the NPC's TS-PathingEntity.lastMovement value
// (set to currentTick + 1 at the end of any tick the NPC stepped, else
// 0). Read by NPC_ARRIVEDELAY (NpcOps.ts:542-555). Mirrors
// ActivePlayer.LastMovement.
LastMovement() int
```

Placement: alphabetic-by-grouping near other NPC reader-getters (between `Nid()` and the var* methods).

### §4.2 — `pkg/script/handlers_npc.go` (handler)

Add `handleNpcArriveDelay` near `handleNpcDelay` (the structurally-closest sibling):

```go
// handleNpcArriveDelay implements NPC_ARRIVEDELAY (opcode 2502): if the
// active NPC has moved within the past 3 ticks (this tick, last tick, or
// 2 ticks ago), suspend the script with a delay computed from the
// movement recency; otherwise no-op. TS NpcOps.ts:542-555.
//
// The 3-tick window arises from the TS lastMovement contract (written
// to currentTick + 1 after a moving tick): the gate accepts moves from
// this tick (lastMovement = T+1), last tick (lastMovement = T), and
// 2 ticks ago (lastMovement = T-1) but rejects moves from 3+ ticks ago.
//
// Inner branch: if NPC moved 2 ticks ago (lastMovement = T-1), suspend
// for 1 tick (delayedUntil = T+1). Otherwise (this tick or last tick),
// suspend for 2 ticks (delayedUntil = T+2). Mapped to goscape's
// SetDelayed(ticks) which writes delayedUntil = currentTick + 1 + ticks.
//
// Vs P_ARRIVEDELAY (handlers.go:739): NPC variant has a 3-tick window
// (vs 2) and a recency-dependent suspend duration (vs always 1 tick),
// per TS NpcOps.ts asymmetry.
func handleNpcArriveDelay(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_ARRIVEDELAY"); err != nil {
        return err
    }
    if s.World == nil {
        return errors.New("NPC_ARRIVEDELAY: no world")
    }
    last := s.ActiveNpc.LastMovement()
    tick := s.World.CurrentTick()
    if last < tick-1 {
        return nil
    }
    if last == tick-1 {
        s.ActiveNpc.SetDelayed(0) // delayedUntil = T+1
    } else {
        s.ActiveNpc.SetDelayed(1) // delayedUntil = T+2
    }
    s.Execution = NpcSuspended
    return nil
}
```

### §4.3 — `pkg/script/handlers.go` (dispatch)

Insert in alphabetic order (the table is loosely opcode-grouped); add near `OpNpcDelay`:

```go
OpNpcArriveDelay:       handleNpcArriveDelay,
```

### §4.4 — `modules/world/npc.go` (Npc impl + stale-comment retire)

Update doc comment at `:74-77` (drop "deferred" framing):

```go
// NAI-82: TS PathingEntity.lastMovement (Engine-TS/.../PathingEntity.ts:56).
// Written to currentTick + 1 at end of updateMovement when position
// changed; read by NPC_ARRIVEDELAY (NAI-125, handlers_npc.go).
lastMovement int
```

Add the getter to `modules/world/npc_script.go` alongside the other ActiveNpc reader-getters (the file holds `NpcType`, `NpcX`, `NpcZ`, `NpcLevel`, etc. at lines 14-26). Placement: after the `NpcCategory` block (`:29-37`), before the stat-reader block, matching the alphabetic-by-grouping convention used by `mockNpc` in §4.5. Method body:

```go
// LastMovement returns n.lastMovement, satisfying script.ActiveNpc.
// Used by NPC_ARRIVEDELAY (handlers_npc.go). The field is written by
// (*Npc).updateMovement at npc_interaction.go:334 to currentTick + 1
// after any tick the NPC stepped.
func (n *Npc) LastMovement() int { return n.lastMovement }
```

### §4.5 — `pkg/script/handlers_npc_test.go` (mockNpc + tests)

**mockNpc** — add field and method (sibling to existing setDelayedCalls etc.):

```go
type mockNpc struct {
    // ...existing fields...
    lastMovement int
    // ...
}

func (m *mockNpc) LastMovement() int { return m.lastMovement }
```

**Test family** — append after the last NPC test in this file (matching the P_ARRIVEDELAY family's structure at `handlers_test.go:892+`):

```go
// -- NPC_ARRIVEDELAY tests (NAI-125) ---------------------------------------
//
// TS NpcOps.ts:542-555: 3-tick acceptance window with recency-dependent
// suspend duration. Asymmetric vs P_ARRIVEDELAY (which has a 2-tick
// window and always SetDelayed(0)).
//
// lastMovement is written by Npc.updateMovement to currentTick + 1
// after any tick the NPC stepped (npc_interaction.go:334).

// TestNpcArriveDelaySuspendsWhenMovedThisTick: lastMovement = currentTick + 1.
// Gate condition: 101 < 99 is false ⇒ continue. Branch: 101 === 99 is false
// ⇒ SetDelayed(1) (delayedUntil = T+2).
func TestNpcArriveDelaySuspendsWhenMovedThisTick(t *testing.T) { ... }

// TestNpcArriveDelaySuspendsWhenMovedLastTick: lastMovement = currentTick.
// Gate: 100 < 99 is false ⇒ continue. Branch: 100 === 99 is false ⇒
// SetDelayed(1) (delayedUntil = T+2). Mid-of-window.
func TestNpcArriveDelaySuspendsWhenMovedLastTick(t *testing.T) { ... }

// TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo: lastMovement = currentTick - 1.
// Gate: 99 < 99 is false ⇒ continue. Branch: 99 === 99 is true ⇒
// SetDelayed(0) (delayedUntil = T+1). NPC-unique branch (no equivalent
// in P_ARRIVEDELAY).
func TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo(t *testing.T) { ... }

// TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo: lastMovement = currentTick - 2.
// Gate: 98 < 99 is true ⇒ no-op return.
func TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo(t *testing.T) { ... }

// TestNpcArriveDelayNoOpWhenNeverMoved: lastMovement = 0 (zero-value).
// Gate: 0 < 99 is true ⇒ no-op return. Pins zero-value.
func TestNpcArriveDelayNoOpWhenNeverMoved(t *testing.T) { ... }

// TestNpcArriveDelayRequiresActiveNpc: no PtrActiveNpc set ⇒
// requireActiveNpc returns "NPC_ARRIVEDELAY: no active npc".
func TestNpcArriveDelayRequiresActiveNpc(t *testing.T) { ... }

// TestNpcArriveDelayRequiresWorld: handler reads s.World.CurrentTick()
// to evaluate gate; missing world must return clean error rather than
// nil-deref. Mirrors P_ARRIVEDELAY's defensive guard.
func TestNpcArriveDelayRequiresWorld(t *testing.T) { ... }
```

Each test follows the established NPC-handler fixture pattern (e.g. `handlers_npc_test.go:412+`):

```go
sf := &ScriptFile{
    Name:    "npc_arrivedelay_<case>",
    Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
}
mn := &mockNpc{lastMovement: <value>}
w := &mockWorld{tick: 100}

state := Init(sf, nil, false, nil, nil)
state.ActiveNpc = mn
state.Pointers |= PtrActiveNpc
state.World = w

err := Execute(state)
// post-conditions on state.Execution + mn.setDelayedCalls + err
```

For the gate-error tests, omit the `state.Pointers |= PtrActiveNpc` (RequiresActiveNpc) or the `state.World = w` (RequiresWorld) line.

## §5 — Test strategy

| Test | `lastMovement` | tick | Expected `Execution` | Expected `setDelayedCalls` | Branch covered |
|---|---|---|---|---|---|
| `…SuspendsWhenMovedThisTick` | 101 | 100 | `NpcSuspended` | `[1]` | else-branch (T+2) |
| `…SuspendsWhenMovedLastTick` | 100 | 100 | `NpcSuspended` | `[1]` | else-branch (T+2) |
| `…SuspendsWhenMovedTwoTicksAgo` | 99 | 100 | `NpcSuspended` | `[0]` | if-branch (T+1) ← **NPC-unique** |
| `…NoOpWhenMovedThreeTicksAgo` | 98 | 100 | `Finished` | `[]` | gate-rejects |
| `…NoOpWhenNeverMoved` | 0 | 100 | `Finished` | `[]` | gate-rejects (zero-value) |
| `…RequiresActiveNpc` | — | — | (err) | `[]` | gate-error |
| `…RequiresWorld` | 101 | (no world) | (err) | `[]` | defensive-guard error |

All seven tests sit in `handlers_npc_test.go` (same file as `mockNpc`). Each constructs `sf := newSingleOp("npc_arrivedelay_<case>", OpNpcArriveDelay)` and uses `Execute(state)` + post-conditions on `state.Execution`, `mn.setDelayedCalls`, and (for error cases) `err.Error()`.

## §6 — Tracked deviations

### DEVIATION-NAI-125-D1 — `s.World == nil` defensive gate

**Site:** `pkg/script/handlers_npc.go:handleNpcArriveDelay`.

**TS behavior:** `checkedHandler` body reads `World.currentTick` directly with no nil-check. TS `World` is a singleton; nil-deref impossible.

**Goscape behavior:** Returns `errors.New("NPC_ARRIVEDELAY: no world")` when `s.World == nil`.

**Rationale:** Mirrors the established sibling-handler convention (`handlePArriveDelay`, `handlePushVars`, `handleMapClock`, `handlePlayerCount` etc.) per `defensive_gate_doc_comment_label`. Goscape's `s.World` is set by callers (test fixtures, OPNPC routing) and can legitimately be nil in unit tests; the gate produces a clean error rather than a panic. Doc-comment labels it "(goscape defensive; TS skips this check)".

**Retire condition:** N/A — self-retiring (matches sibling-convention permanent shape).

### DEVIATION-NAI-125-D2 — `delayed = true` boolean assignment skipped

**Site:** `pkg/script/handlers_npc.go:handleNpcArriveDelay`.

**TS behavior:** Sets `state.activeNpc.delayed = true` before computing `delayedUntil`.

**Goscape behavior:** No `delayed` boolean assignment; only `SetDelayed(ticks)` (which writes `delayedUntil`).

**Rationale:** Goscape's `Npc.delayed bool` field exists (`npc.go:82`) and is written by `SetDelayed` itself, per the established `handleNpcDelay` precedent (NPC_DELAY-N, NAI-20). Confirmed at HEAD `7fc798c` — `(*Npc).SetDelayed` body at `modules/world/npc.go:323-326`:

```go
func (n *Npc) SetDelayed(ticks int) {
    n.delayed = true
    n.delayedUntil = n.server.currentTick + 1 + ticks
}
```

The TS pattern of "set delayed=true, then set delayedUntil=…" is collapsed into the single `SetDelayed(ticks)` primitive at field-write time, so calling `SetDelayed(0)` or `SetDelayed(1)` produces TS-faithful state.

**Retire condition:** N/A — self-retiring (architectural convention, not a temporary divergence).

## §7 — Cadence

**Single-bundle subagent-driven-development** (per `execution_mode_default`):

1. **Bundle 1 (additive port, 5 files, ~50 prod LOC + ~150 test LOC):**
   1. RED — add 7 tests in `handlers_npc_test.go` (will fail compilation pending mockNpc method + handler).
   1. GREEN-1 — extend `mockNpc` with `lastMovement int` + `LastMovement() int` method; tests now fail at runtime.
   1. GREEN-2 — add `LastMovement() int` to `ActiveNpc` interface; impl on `*Npc` in `modules/world/npc_script.go` (or `npc.go`); retire stale `:76` doc-comment.
   1. GREEN-3 — add `handleNpcArriveDelay` + register in dispatch table; tests pass.
1. **Sonnet code-reviewer pass** (per `superpowers_code_reviewer_model`).
1. **Reviewer-fix sub-commit** if needed.
1. **User-launched smoke** (per `smoke_test_server_handoff`).
1. **Close commit** with `Closes memory:` trailer (per `close_commit_memory_trailer`).

**Why single bundle:** The 5 files are interface + 1 impl + 1 mock + tests + dispatch wiring — all required for any subset of tests to pass. Splitting into stages adds no review value.

## §8 — Smoke binding

**PRIMARY (binding):** kill Tutorial Island giant rat as fresh char with bronze dagger; server log emits no `WARN: no handler for NPC_ARRIVEDELAY` line. NAI-125 closes regardless of cascade outcomes.

**SECONDARY (cascade hypothesis, not binding):** with `npc_death` proc body now executing past pc=4, observable user-side effects may surface — loot drop visibility, respawn timing, kill-count XP arithmetic, drop-table dispatch, etc. Per `cascade_theory_smoke_binding` and `smoke_surfaces_adjacent_divergences`:
- Newly-surfaced symptoms ≤30 LOC fit → in-scope-stretch.
- Newly-surfaced symptoms >30 LOC → route to NAI-126 candidate queue.

## §9 — Risk register

| Risk | Likelihood | Detection | Mitigation |
|---|---|---|---|
| `s.World` nil in some production NPC-script path (D1 false-positive) | low | sibling P_ARRIVEDELAY exercise + integration tests | gate-error path is logged, not silent; surfaces immediately |
| Stale `npc.go:76` "deferred" comment is referenced by another doc | low | grep `NAI-82 spec §6.1` | replace cross-references at retire time |
| Branch ordering bug (off-by-one in T+1 vs T+2 mapping) | low | the `…TwoTicksAgo` test is the unique-to-NPC branch and pins `setDelayedCalls=[0]` exactly | RED-test-first catches at GREEN |

## §10 — Pattern memories applied

- `consume_reserved_constant` — `OpNpcArriveDelay = 2502` reserved at NAI-82 brain-time; new consumer (NAI-125) owns the full dispatch path: handler + interface getter + impl + mock + tests + dispatch entry.
- `audit_full_method_against_ts` — verbatim TS port: 3-tick window, inner T+1 vs T+2 branch, NpcSuspended transition.
- `defensive_gate_doc_comment_label` — `s.World == nil` guard labeled "(goscape defensive; TS skips this check)" in DEVIATION-NAI-125-D1.
- `verify_implementer_claims` — independent fresh `go test ./... && go vet ./... && go build ./...` after each commit.
- `execution_mode_default` — subagent-driven-development without offering menu.
- `superpowers_code_reviewer_model` — Sonnet reviewer (never Opus).
- `superpowers_clear_between_spec_and_impl` — user `/clear`s between this spec being written and plan-writing.
- `cascade_theory_smoke_binding` — PRIMARY closes on log-line silence; cascade outcomes route forward.
- `smoke_surfaces_adjacent_divergences` — newly-surfaced symptoms route per LOC threshold.
- `close_commit_memory_trailer` — `Closes memory:` trailer on close commit.
- `feedback_subagent_wt_path` — `git status` post-commit confirms no worktree-stray writes.
- `controller_preflight` — every plan premise (file paths, line numbers, helper signatures, struct literals) re-grepped at HEAD before each implementer dispatch.

## §11 — Cross-references

- TS source: `Engine-TS/src/engine/script/handlers/NpcOps.ts:542-555` (NPC_ARRIVEDELAY); `Engine-TS/src/engine/entity/PathingEntity.ts:56` (lastMovement contract); `Engine-TS/src/engine/script/ScriptState.ts:32` (`NPC_SUSPENDED = 5`).
- Sibling handler: `pkg/script/handlers.go:729-752` (`handlePArriveDelay`, NAI-82); `pkg/script/handlers_test.go:892-1027` (P_ARRIVEDELAY test family).
- Producer-side wiring: `modules/world/npc_interaction.go:334` (`n.lastMovement = s.currentTick + 1`); `modules/world/npc_movement_test.go:24-50` (movement-write tests).
- NPC-suspension precedent: `pkg/script/handlers_npc.go:313-330` (`handleNpcDelay`, NAI-20); `modules/world/npc_ai.go:19` (resume from `NpcSuspended`); `modules/world/npc.go:309-316` (Npc.turn dispatch).
- NAI-123 close memo: `nai_followups.md:6233-6286`; NAI-124 close memo: `nai_followups.md:6290-6326`.

## §12 — Out-of-scope

- **Paramtype sign-extension bundle** (NAI-125 candidate alt #1) — real divergence but not load-bearing on any current smoke; routed forward to a future NAI sub-spec.
- **Style-cleanup modernization warnings** (NAI-125 candidate alt #2) — `state.go:380/385/403/407`, `runner.go:30/33`, `handlers_npc.go:923/957`, `handlers_npc_test.go:2109` — folded into any future sub-spec touching those files.
- **`npc_death` proc body cascade fixes** — surfacing symptom-by-symptom in NAI-126+ once `NPC_ARRIVEDELAY` lands and the proc executes past pc=4.
- **All NAI-119/117/115/111/121 carryovers** — unchanged routing.
