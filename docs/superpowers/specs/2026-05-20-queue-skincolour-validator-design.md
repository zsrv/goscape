# Queue + SkinColour validator port

Author: zsrv + Claude (Opus 4.7)
Date: 2026-05-20
Status: Approved
Predecessor: [HitType validator port](2026-05-20-hit-type-validator-port-design.md) — §9 explicitly named both items as out-of-scope cosmetic extractions.

---

## 1. Goal

Port two `ScriptInputRangeValidator` family entries from TS `ScriptValidators.ts` into goscape's sibling free-function style:

- `QueueValid = (0, 19, 'AIQueue')` → `checkQueue(v int, op string) error`
- `SkinColourValid = (0, 7, 'SkinColour')` → `checkSkinColour(v int, op string) error`

Apply at three call sites (`NPC_QUEUE`, `NPC_WALKTRIGGER`, `SETSKINCOLOUR`).

Queue receives a **deliberate range shift** from goscape's current `[1, 20]` to TS-literal `[0, 19]` — this turns the cosmetic extraction into a TS-fidelity correction at the validator layer. SkinColour is a clean direct mirror; TS and goscape already agree on `[0, 7]`.

## 2. Why now

Predecessor close memory `[[hit-type-validator-slice-close]]` names these as the only remaining "simple enum-range" validators from TS `ScriptValidators.ts` that goscape inline-validates without a wrapping free-function. Out-of-scope §9 of `2026-05-20-hit-type-validator-port-design.md` calls them cosmetic, but the Queue side carries a quiet pre-existing range divergence from TS that deserves explicit handling — closing it as TS-literal aligns the validator surface and surfaces the upstream TS fencepost bug via an inherited deviation pin.

The slice is small (one new validator each, three call sites, one new pin, ~6 new tests) so bundling is the right granularity. Splitting into a SkinColour slice plus a Queue slice would be over-decomposition.

## 3. TS reference

### 3.1 Validators

```ts
// Engine-TS/src/engine/script/ScriptValidators.ts:114
export const QueueValid: ScriptValidator<number, number>
    = new ScriptInputRangeValidator(0, 19, 'AIQueue');

// Engine-TS/src/engine/script/ScriptValidators.ts:137
export const SkinColourValid: ScriptValidator<number, number>
    = new ScriptInputRangeValidator(0, 7, 'SkinColour');
```

The `ScriptInputRangeValidator` body (`ScriptValidators.ts:74-91`) uses INCLUSIVE bounds:

```ts
constructor(min: number, max: number, name: string) { ... }
validate(input: number): T {
    if (input >= this.min && input <= this.max) {
        return input as T;
    }
    throw new Error(...);
}
```

Per carry-forward `[[hit-type-validator-slice-close]]` finding #1: the second constructor arg is the inclusive max, NOT an exclusive count. Inclusive ranges: Queue `[0, 19]`, SkinColour `[0, 7]`.

### 3.2 Call sites

```ts
// Engine-TS/src/engine/script/handlers/NpcOps.ts:144-150 (NPC_QUEUE)
[ScriptOpcode.NPC_QUEUE]: checkedHandler(ActiveNpc, state => {
    const delay = check(state.popInt(), NumberNotNull);
    const arg = state.popInt();
    const queueId = check(state.popInt(), QueueValid);
    state.activeNpc.enqueueScript(ServerTriggerType.AI_QUEUE1 + queueId - 1, delay, arg);
}),

// Engine-TS/src/engine/script/handlers/NpcOps.ts:483-490 (NPC_WALKTRIGGER)
[ScriptOpcode.NPC_WALKTRIGGER]: checkedHandler(ActiveNpc, state => {
    const [queueId, arg] = state.popInts(2);
    check(queueId, QueueValid);
    state.activeNpc.walktrigger = queueId - 1;
    state.activeNpc.walktriggerArg = arg;
}),

// Engine-TS/src/engine/script/handlers/PlayerOps.ts:1121-1124 (SETSKINCOLOUR)
[ScriptOpcode.SETSKINCOLOUR]: state => {
    const skin = check(state.popInt(), SkinColourValid);
    state.activePlayer.colors[4] = skin;
},
```

Note the TS Queue pattern: validator accepts `[0, 19]` but arithmetic subtracts 1 (`AI_QUEUE1 + queueId - 1` and `walktrigger = queueId - 1`). This is the upstream TS fencepost bug — queueId=0 passes the validator and produces `AI_QUEUE1 - 1` (an invalid trigger one before AI_QUEUE1). The bug is dormant because real scripts never push 0; see §4 audit.

## 4. Goscape baseline

| Piece | Where | State |
| --- | --- | --- |
| `checkQueue` | (nowhere) | Unported. |
| `checkSkinColour` | (nowhere) | Unported. |
| `handleNpcQueue` | `pkg/script/handlers_npc.go:482-498` | Inline-validates `[1, 20]` (range divergence from TS `[0, 19]`); error string `"NPC_QUEUE: invalid queueId %d (want 1..20)"`. Trigger arithmetic `TriggerAiQueue1 + ServerTriggerType(queueID - 1)`. |
| `handleNpcWalkTrigger` | `pkg/script/handlers_npc.go:557-569` | Inline-validates `[1, 20]`; error string `"NPC_WALKTRIGGER: invalid queueId %d (want 1..20)"`. Stores `queueID - 1` via `SetWalkTrigger`. |
| `handleSetSkinColour` | `pkg/script/handlers_player.go:1657-1667` | Inline-validates `[0, 7]` (matches TS exactly); error string `"SETSKINCOLOUR: invalid skin colour %d (range 0..7)"`. `SetColorPart(4, skin)` write. |

### 4.1 Content audit — script-side queueId usage

A grep across `LostCityRS/Content/scripts/` enumerates the queueId values real RuneScript content pushes:

- `npc_queue(N, …)` first arg ∈ `{1, 2, 3, 4, 5, 6, 7, 10, 11, 12}`
- `npc_walktrigger(N, …)` first arg ∈ `{8}`

No script pushes `0` (would be illegal under goscape's `[1, 20]` and produces garbage under TS's `[0, 19]`). No script pushes `20` (legal under goscape, rejected under TS). Therefore the TS-literal range shift is safe against all existing in-the-wild Content; the slice does not require any companion script-recompile or compiler change.

The goscape script compiler (`pkg/pack/compiler/`) emits queue IDs unchanged — it has no special-case transformation for `npc_queue` / `npc_walktrigger` arguments (verified by absence of `NPC_QUEUE` / `NpcQueue` references in non-test compiler code). So pushed values are 1-based human-readable identifiers, exactly matching `LostCityRS/Content` source.

### 4.2 Test baseline

| Test | Where | Asserts |
| --- | --- | --- |
| `TestHandleNpcQueueEnqueues` | `handlers_npc_test.go:917-955` | Happy-path pop order + trigger arithmetic with queueID=3. Doc comment references "queueID (1-20)". |
| `TestHandleNpcQueueInvalidQueueIDErrors` | `handlers_npc_test.go:979-1015` | Pins reject at `{0, 21}` with exact `"NPC_QUEUE: invalid queueId %d (want 1..20)"` error strings. |
| `TestHandleNpcQueueNullDelayRejected` | `handlers_npc_test.go:1630+` | Pins null-delay rejection orthogonal to queueId. Doc comment references "queueId 1..20 range check". |
| `TestNpcWalkTrigger_QueueIDBelowOne_Errors` | `handlers_npc_test.go:3109-3127` | Pins reject at queueID=0. |
| `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors` | `handlers_npc_test.go:3128-3142` | Pins reject at queueID=21. |
| `TestNpcWalkTrigger_PopOrderAndTransform` | `handlers_npc_test.go:3143+` | Happy-path pop order + `SetWalkTrigger(queueID - 1)`. |
| `TestHandleSetSkinColour_WritesColors4` | `handlers_player_test.go:5061-5099` | Happy-path color write. |
| `TestHandleSetSkinColour_RejectsOutOfRange` | `handlers_player_test.go:5101-5136` | Pins reject at `{-1, 8, -100, 100}` with `strings.Contains(err, "SETSKINCOLOUR")`. Loose assertion — survives error-message rewording. |
| `TestHandleSetSkinColour_RequiresActivePlayer` | `handlers_player_test.go:5138+` | Goscape-defensive guard pin. |

Net gap: two new validators, three call-site wraps, ~5 doc-comment refreshes, three existing-test rewrites, six new tests, one new deviation pin.

## 5. Design

### 5.1 `checkQueue` — `pkg/script/handlers_npc.go`

Inserted alongside `checkHitType` (~line 99), `checkHuntVis` (~line 109):

```go
// checkQueue validates an AI-queue identifier. Mirrors TS QueueValid
// (ScriptValidators.ts:114) — ScriptInputRangeValidator(0, 19, 'AIQueue'),
// inclusive range [0, 19]. Note: the call-site arithmetic at NPC_QUEUE /
// NPC_WALKTRIGGER then subtracts 1 to index TriggerAiQueue1..20, which
// means queueId=0 produces TriggerAiQueue1-1 (a garbage trigger one
// before AI_QUEUE1). That fencepost is inherited from upstream TS and is
// not exercised by any LostCityRS/Content script (audit: real callers
// push 1..12); the inherited bug is pinned by
// NAI-QUEUE-D-TS-FENCEPOST-INHERITED.
func checkQueue(v int, op string) error {
    if v < 0 || v > 19 {
        return fmt.Errorf("%s: queue id out of range (%d)", op, v)
    }
    return nil
}
```

### 5.2 `checkSkinColour` — `pkg/script/handlers_player.go`

Inserted alongside `checkNotNull` (~line 84):

```go
// checkSkinColour validates a player skin-colour wire value. Mirrors TS
// SkinColourValid (ScriptValidators.ts:137) —
// ScriptInputRangeValidator(0, 7, 'SkinColour'), inclusive range [0, 7].
func checkSkinColour(v int, op string) error {
    if v < 0 || v > 7 {
        return fmt.Errorf("%s: skin colour out of range (%d)", op, v)
    }
    return nil
}
```

No `pkg/objtype/queue.go` or `pkg/objtype/skincolour.go` — TS has no named enum constants for either (`AIQueue` and `SkinColour` are only string labels in the validator constructor). Follows the `checkHuntVis` precedent of bare-number range, no const file.

### 5.3 Apply at three call sites

| Site | Edit |
| --- | --- |
| `handleNpcQueue` (handlers_npc.go:491-494) | Replace<br>`if queueID < 1 \|\| queueID > 20 { return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID) }`<br>with<br>`if err := checkQueue(queueID, "NPC_QUEUE"); err != nil { return err }`.<br>Arithmetic `TriggerAiQueue1 + ServerTriggerType(queueID - 1)` unchanged. |
| `handleNpcWalkTrigger` (handlers_npc.go:562-565) | Replace<br>`if queueID < 1 \|\| queueID > 20 { return fmt.Errorf("NPC_WALKTRIGGER: invalid queueId %d (want 1..20)", queueID) }`<br>with<br>`if err := checkQueue(queueID, "NPC_WALKTRIGGER"); err != nil { return err }`.<br>`s.ActiveNpc.SetWalkTrigger(queueID - 1)` unchanged. |
| `handleSetSkinColour` (handlers_player.go:1661-1664) | Replace<br>`if skin < 0 \|\| skin > 7 { return fmt.Errorf("SETSKINCOLOUR: invalid skin colour %d (range 0..7)", skin) }`<br>with<br>`if err := checkSkinColour(skin, "SETSKINCOLOUR"); err != nil { return err }`.<br>`s.Self.SetColorPart(4, skin)` unchanged. |

### 5.4 Doc-comment refresh (production)

**`handlers_npc.go:475-481` (`handleNpcQueue` doc)** — replace:

> `// queueId ∈ [1, 20] maps to TriggerAiQueue1..20 via`<br>
> `// arithmetic: trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS`<br>
> `// NpcOps.ts:144-150, including the NumberNotNull check on delay`<br>
> `// (closed in NAI-20). The Go-side queueId 1..20 range check`<br>
> `// corresponds to TS QueueValid; the arg pop is unwrapped per TS.`

with:

> `// queueId ∈ [0, 19] (TS QueueValid) maps to TriggerAiQueue1..20 via`<br>
> `// arithmetic: trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS`<br>
> `// NpcOps.ts:144-150, including the NumberNotNull check on delay`<br>
> `// (closed in NAI-20). Validated via checkQueue (TS-literal [0, 19]`<br>
> `// inclusive); the arg pop is unwrapped per TS.`

**`handlers_npc.go:550-556` (`handleNpcWalkTrigger` doc)** — replace:

> `// queueID ∈ [1, 20] mirrors TS QueueValid range, transformed`<br>
> `// to [0, 19] via queueID-1 to match TS NpcOps.ts:488 storage. Mirrors`<br>
> `// TS NpcOps.ts:483-490.`

with:

> `// queueId ∈ [0, 19] (TS QueueValid), then queueId-1 mirrors TS`<br>
> `// NpcOps.ts:488 storage (walktrigger = queueId - 1). Validated via`<br>
> `// checkQueue. Mirrors TS NpcOps.ts:483-490.`

**`handlers_player.go:1648-1656` (`handleSetSkinColour` doc)** — light touch, append validator citation:

> Add one sentence: `// Validated via checkSkinColour (TS SkinColourValid, inclusive [0, 7]).`

### 5.5 Carry-forward stale-comment sweep (test side)

Per carry-forward `[[hit-type-validator-slice-close]]` finding #2: when wiring a previously-deferred validator, sweep BOTH production AND test doc comments for stale phrasings. Grep keywords for this slice:

- `"want 1..20"` — pinpoints expected-error strings in invalid-id tests
- `"1..20"` / `"1-20"` — doc-comment range citations
- `"range 0..7"` — SkinColour error-message citations
- `"stays raw"` — generic carry-forward keyword from predecessor slice

Targets identified:

- `handlers_npc_test.go:917-919` (`TestHandleNpcQueueEnqueues` doc — "queueID (1-20) to TriggerAiQueue1 + queueID - 1")
- `handlers_npc_test.go:1630-1632` (`TestHandleNpcQueueNullDelayRejected` doc — "queueId 1..20 range check is orthogonal")
- `handlers_npc_test.go:3093` block header for NAI-37 walktrigger tests — may reference 1..20
- `handlers_npc_test.go:979` (`TestHandleNpcQueueInvalidQueueIDErrors` doc — "queueID out of [1,20]")

Plan implementer should grep the targets above plus an extra `rg "want 1\.\.20|1\.\.20|1-20|range 0\.\.7"` sweep over `pkg/script/` and update all hits consistent with the new ranges. Production-vs-test boundary explicitly INCLUDED — the predecessor slice's reviewer caught a test-side miss that the spec hadn't named.

### 5.6 Existing-test rewrites

Range shift mandates these edits (no test deletion — boundary coverage moves, not disappears):

| Test | Edits |
| --- | --- |
| `TestHandleNpcQueueInvalidQueueIDErrors` (handlers_npc_test.go:979-1015) | Rename: optional but recommended → `TestHandleNpcQueueOutOfRangeErrors`. Cases shift from `{zero=0, twentyone=21}` to `{negative=-1, twenty=20}`. Expected error strings update to `"NPC_QUEUE: queue id out of range (-1)"` / `"NPC_QUEUE: queue id out of range (20)"`. Doc comment updates "[1,20]" → "[0,19]". |
| `TestNpcWalkTrigger_QueueIDBelowOne_Errors` (handlers_npc_test.go:3109) | Rename → `TestNpcWalkTrigger_QueueIDBelowZero_Errors`. `PushInt(0)` → `PushInt(-1)`. Any error-string literal assertion updated. |
| `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors` (handlers_npc_test.go:3128) | Rename → `TestNpcWalkTrigger_QueueIDAboveNineteen_Errors`. `PushInt(21)` → `PushInt(20)`. Any error-string literal assertion updated. |
| `TestHandleNpcQueueEnqueues` (handlers_npc_test.go:917) | Doc-comment-only edit per §5.5. Body unchanged (queueID=3 happy-path still valid under [0, 19]). |
| `TestHandleNpcQueueNullDelayRejected` (handlers_npc_test.go:1630+) | Doc-comment-only edit per §5.5. Body unchanged (queueID=5 in fixture still valid under [0, 19]). |
| `TestNpcWalkTrigger_PopOrderAndTransform` (handlers_npc_test.go:3143+) | No edit (queueID happy-path value should already be ∈ [1, 19] which is valid under both old and new range; implementer to verify). |
| `TestHandleSetSkinColour_RejectsOutOfRange` (handlers_player_test.go:5103) | **No edit required.** Assertion is `strings.Contains(err.Error(), "SETSKINCOLOUR")` — new error string `"SETSKINCOLOUR: skin colour out of range (-1)"` still satisfies. Implementer verifies during GREEN-confirm. |
| `TestHandleSetSkinColour_WritesColors4` + `TestHandleSetSkinColour_RequiresActivePlayer` | No edit. Range-orthogonal. |

### 5.7 New tests

**Validator unit tests** (table-driven, mirror `TestCheckHitType_*` shape):

- `TestCheckQueue_Range` in `handlers_npc_test.go` near `checkHitType` test block:
  - Accept: `{0, 1, 10, 19}`
  - Reject: `{-1, 20, 21, math.MinInt, math.MaxInt}`
  - Verify error message contains the op name (e.g., `"TEST_OP"`) and the offending value.

- `TestCheckSkinColour_Range` in `handlers_player_test.go` near `checkNotNull` tests:
  - Accept: `{0, 1, 4, 7}`
  - Reject: `{-1, 8, 100, math.MinInt}`
  - Same op-name + value assertion shape.

**Handler boundary pins** (regression guards for the TS-literal shift):

- `TestHandleNpcQueueAcceptsZeroEdge` (handlers_npc_test.go): pins TS-faithful fencepost. queueID=0 now passes the validator. Verify the EnqueueScriptForTrigger call fires with `TriggerAiQueue1 - 1` (i.e., one less than `TriggerAiQueue1`). Doc-comment cites `NAI-QUEUE-D-TS-FENCEPOST-INHERITED`. This test exists to prevent a future contributor from "fixing" the validator back to `[1, 20]` without recognizing the deliberate divergence call.
- `TestHandleNpcQueueRejectsTwenty` (handlers_npc_test.go): pins shift in upper-bound rejection. queueID=20 now rejected (was accepted under `[1, 20]`).
- `TestHandleNpcWalkTriggerAcceptsZeroEdge` (handlers_npc_test.go): same as above for the walktrigger site. Verify `SetWalkTrigger(-1)` (queueID - 1 = -1).
- `TestHandleNpcWalkTriggerRejectsTwenty` (handlers_npc_test.go): queueID=20 now rejected at walktrigger.

### 5.8 Deviation pin

Open one new pin in production code, tagged for future audit greps:

- **`NAI-QUEUE-D-TS-FENCEPOST-INHERITED`** — on the `checkQueue` doc comment in `pkg/script/handlers_npc.go`. Records that the TS-literal `[0, 19]` validator combined with the `-1` arithmetic admits `queueId=0` (produces `TriggerAiQueue1 - 1` garbage trigger). Faithfully ports upstream TS bug. Not exercised by any LostCityRS/Content script (per §4.1 audit). **Retires when** upstream TS Engine fixes either the validator (to `[1, 20]`) or the arithmetic (drops the `-1`).

No retirement of any existing pin — the goscape `[1, 20]` range was inline-implemented without a tag (silent goscape-side fencepost fix), so there is no tag to retire.

## 6. Data flow

No behavioral change at the trigger / storage layer. The change is concentrated in the validation step:

- `handleNpcQueue`: `PopInt(delay) → checkNotNull → PopInt(arg) → PopInt(queueId) → checkQueue → EnqueueScriptForTrigger(TriggerAiQueue1 + queueId - 1, delay, arg)`
- `handleNpcWalkTrigger`: `PopInt(arg) → PopInt(queueId) → checkQueue → SetWalkTrigger(queueId - 1); SetWalkTriggerArg(arg)`
- `handleSetSkinColour`: `requireActivePlayer → PopInt(skin) → checkSkinColour → SetColorPart(4, skin)`

## 7. Error semantics

- All three handlers continue to return early on validator error before any mutation. Order matters at SETSKINCOLOUR (validator before `SetColorPart`); already correct.
- Error wrapping uses the existing `fmt.Errorf("%s: …")` pattern. Matches sibling `checkHitType` / `checkHuntVis` / `checkNpcStatID` style.
- No typed errors / `errors.Is` introduced — TS throws a plain `Error` which goscape mirrors with plain `fmt.Errorf`.

## 8. Testing

### 8.1 Unit tests for validators

Two table-driven tests per §5.7. Cover boundaries (`min`, `min+1`, `max-1`, `max`, `min-1`, `max+1`), plus extreme values (`math.MinInt`, `math.MaxInt`) for the int parameter type.

### 8.2 Handler invalid-input tests

Four new boundary-pin tests per §5.7 plus three rewritten existing tests per §5.6.

### 8.3 Regression coverage

Three existing happy-path tests (`TestHandleNpcQueueEnqueues`, `TestNpcWalkTrigger_PopOrderAndTransform`, `TestHandleSetSkinColour_WritesColors4`) plus two ancillary tests (`TestHandleNpcQueueNullDelayRejected`, `TestHandleSetSkinColour_RequiresActivePlayer`) continue to assert pop order, trigger arithmetic, and downstream mutation. All must stay green; doc-comment-only edits per §5.5 do not affect assertion logic.

### 8.4 Gate commands

```bash
# Focused
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... 2>&1 | tail -5

# Full
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...

# Smoke-pack must remain 12 OK / 0 ERR / 0 SKIP
```

## 9. Out of scope

- `GenderValid` (TS `ScriptValidators.ts:136`, range `[0, 1]`) — blocked. `handleSetGender` is a TS-unimplemented stub (`handlers_b0_stubs.go:21-25`, pin `NAI-162-D-STUB-SETGENDER`). Despite TS `PlayerOps.ts:1104-1118` containing a real implementation, the goscape stub deferral predates this slice and the unstub effort is a separate concern (port the body, then apply the validator). Reaches scope when SETGENDER body is ported.
- Config-registry validators (`InvTypeValid`, `NpcTypeValid`, `ObjTypeValid`, `EnumTypeValid`, etc.) — same scope-exclusion as the HitType slice §9; these require live `Configs.XxxType(id)` registry-presence lookup, not range checks. A different porting story.
- Compiler-side changes to ai_queueN emission. The TS-literal `[0, 19]` port aligns the runtime validator with TS but does not change what `pkg/pack/compiler/` emits. Real scripts continue pushing 1-based queue IDs (per §4.1 audit); arithmetic continues compensating via `-1`. A future bug-fix slice could choose to shift the validator+compiler+arithmetic to a fully 0-based or fully 1-based design and break the fencepost cycle entirely; this slice preserves the existing emission convention and pins the inherited bug.
- `pkg/objtype/queue.go` / `pkg/objtype/skincolour.go` const files — bare-number range checks are idiomatic per the `checkHuntVis` precedent. TS has no named enum constants for either; introducing `QueueCount` / `SkinColourCount` would be over-symmetry with the HitType slice.
- Behavioral change at `Player.SetColorPart`, `Npc.EnqueueScriptForTrigger`, or `Npc.SetWalkTrigger` — unchanged.

## 10. Acceptance criteria

1. New `checkQueue` (~6 LOC body + doc) in `pkg/script/handlers_npc.go`; new `checkSkinColour` (~6 LOC body + doc) in `pkg/script/handlers_player.go`. Both bare-number range, no objtype dependency.
2. Three handler call sites wire their respective validator via the new free-functions. Trigger / storage arithmetic unchanged at all three (`TriggerAiQueue1 + queueID - 1`, `SetWalkTrigger(queueID - 1)`, `SetColorPart(4, skin)`).
3. Doc comments at `handleNpcQueue`, `handleNpcWalkTrigger`, `handleSetSkinColour` refreshed per §5.4 to reflect new `[0, 19]` (Queue) ranges and validator function names.
4. Stale `"1..20"` / `"want 1..20"` / `"1-20"` / `"range 0..7"` doc-comment phrasings swept from BOTH production and test files per §5.5. Final grep `rg "want 1\.\.20|range 0\.\.7"` over `pkg/script/` returns empty.
5. Existing tests `TestHandleNpcQueueInvalidQueueIDErrors`, `TestNpcWalkTrigger_QueueIDBelowOne_Errors`, `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors` rewritten per §5.6. `TestHandleSetSkinColour_RejectsOutOfRange` verified green unmodified.
6. Six new tests added: `TestCheckQueue_Range`, `TestCheckSkinColour_Range`, `TestHandleNpcQueueAcceptsZeroEdge`, `TestHandleNpcQueueRejectsTwenty`, `TestHandleNpcWalkTriggerAcceptsZeroEdge`, `TestHandleNpcWalkTriggerRejectsTwenty`. All passing.
7. One new deviation pin `NAI-QUEUE-D-TS-FENCEPOST-INHERITED` opened in `checkQueue` doc comment.
8. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean across all packages.
9. Smoke-pack `12 OK / 0 ERR / 0 SKIP`.
