# AI-queue fencepost tighten — runtime validator [0,19] → [1,20]

## Goal

Close the inherited TS fencepost bug pinned by `NAI-QUEUE-D-TS-FENCEPOST-INHERITED` by tightening `checkQueue` at the runtime validator gate. Eliminate the path where `queueID=0` produces `TriggerAiQueue1 - 1` (a garbage trigger one below the AI_QUEUE1..20 range).

## Background

`pkg/script/handlers_npc.go:106-120` validates the first-popped arg of NPC_QUEUE and NPC_WALKTRIGGER:

```go
func checkQueue(v int, op string) error {
    if v < 0 || v > 19 { ... }
    return nil
}
```

Two call sites then apply `queueID-1` arithmetic:

1. `handlers_npc.go:511` (NPC_QUEUE): `trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)` — `queueID=0` yields `TriggerAiQueue1 - 1` (garbage).
2. `handlers_npc.go:583` (NPC_WALKTRIGGER): `s.ActiveNpc.SetWalkTrigger(queueID - 1)` — `queueID=0` stores `walktrigger=-1`. The walktrigger consumer at `modules/world/npc_interaction.go:299` does `TriggerAiQueue1 + ServerTriggerType(n.walktrigger)`, so `walktrigger=-1` produces the same garbage trigger.

This was inherited verbatim from TS `ScriptValidators.ts:114` `ScriptInputRangeValidator(0, 19, 'AIQueue')` and pinned in the queue-skincolour slice ([[queue-skincolour-validator-slice-close]]) per audit that real LostCity/Content scripts use only `1..12`. The bug has zero real-world impact but represents a known correctness gap.

## Approach

**Runtime-only tighten.** Shift `checkQueue` from TS-literal `[0, 19]` inclusive to corrected `[1, 20]` inclusive. The `+queueId-1` arithmetic stays — `queueId=1` → `TriggerAiQueue1+0` = `TriggerAiQueue1` (unchanged); `queueId=20` → `TriggerAiQueue1+19` = `TriggerAiQueue20` (now reachable). No compiler change, no bytecode regen.

This is a **deliberate deviation from TS-literal**. Per goscape convention (see `Player.Damage` negative-amount clamp precedent), deliberate-deviation-for-correctness gets inline doc-comment explanation, **not** a fresh formal `NAI-XXX-D-*` pin. The pinned bug is retired, no replacement pin opened.

## Changes

### Production (`pkg/script/handlers_npc.go`)

**1. `checkQueue` body (line 116):** `if v < 0 || v > 19` → `if v < 1 || v > 20`.

**2. `checkQueue` doc-comment (lines 106-114):** rewrite from TS-literal framing to deliberate-deviation framing. Note this is no longer a faithful mirror of `ScriptValidators.ts:114` — cite that the inherited fencepost was retired, the corrected range matches actual LostCity/Content script usage, the `+queueId-1` arithmetic at call sites is now safe across the full validated range, and reference the close memory for audit data.

**3. `handleNpcQueue` doc-comment (lines 491-497):** drop "queueId ∈ [0, 19] (TS QueueValid)" → replace with "queueId ∈ [1, 20] (goscape deviation from TS-literal — see checkQueue doc)". Trim the fencepost discussion now redundant with checkQueue doc.

**4. `handleNpcWalkTrigger` doc-comment (lines 566-573):** parallel rewrite — same `[0, 19] → [1, 20]` range update + drop the redundant fencepost prose.

### Tests (`pkg/script/handlers_npc_test.go`)

**5. `TestCheckQueue_Range` (lines 118-137):** doc + table flip. Doc: drop "TS-literal" framing, cite new range. Table:
- Valid set: `{0, 1, 10, 19}` → `{1, 10, 19, 20}`
- Invalid set: `{-1, 20, 21, MinInt, MaxInt}` → `{-1, 0, 21, MinInt, MaxInt}` (drops `20`, adds `0`)

**6. `TestHandleNpcQueueOutOfRangeErrors` (lines 1001-1043):** doc + cases. Doc: drop "TS-literal range shift (was [1, 20] pre-NAI-QUEUE-D port)" — replace with deviation framing. Cases:
- `{"negative", -1, ...}` (kept, range error message unchanged structure)
- `{"twenty", 20, ...}` → DROP (20 is now valid)
- New: `{"zero", 0, "NPC_QUEUE: queue id out of range (0)"}` 
- New: `{"twentyone", 21, "NPC_QUEUE: queue id out of range (21)"}`

**7. `TestHandleNpcQueueAcceptsZeroEdge` (lines 1045-1081):** rename to `TestHandleNpcQueueRejectsZero`. Invert assertion: expect error containing `"NPC_QUEUE: queue id out of range (0)"`, expect zero `enqueueCalls`. Doc: drop "TS-faithful fencepost" framing — replace with retirement framing ("Pins the tightened validator that closes NAI-QUEUE-D-TS-FENCEPOST-INHERITED").

**8. `TestHandleNpcQueueRejectsTwenty` (lines 1083-1112):** rename to `TestHandleNpcQueueAcceptsTwenty`. Invert assertion: expect no error, expect 1 `enqueueCall` with `trigger == TriggerAiQueue20`. Doc rewrite parallel to #7.

**9. `TestNpcWalkTrigger_QueueIDAboveNineteen_Errors` (lines 3224-3237):** rename to `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors`. Body: `queueID=20` → `queueID=21`. Error-substring expectations track the rename.

**10. `TestNpcWalkTrigger_QueueIDBelowZero_Errors` (lines 3205-3222):** body unchanged (still tests `-1`). Doc/comment: shift "invalid under [0, 19]" → "invalid under [1, 20]" for accuracy.

**11. `TestHandleNpcWalkTriggerAcceptsZeroEdge` (lines 3239-3258):** rename to `TestHandleNpcWalkTriggerRejectsZero`. Invert assertion: expect error, expect zero `walkTriggerCalls`. Doc: parallel rewrite to #7.

**12. `TestHandleNpcWalkTriggerRejectsTwenty` (lines 3260-3283):** rename to `TestHandleNpcWalkTriggerAcceptsTwenty`. Invert: expect no error, expect `walkTriggerCalls == []int{19}` (queueID=20 → walktrigger=19). Doc parallel rewrite.

**13. `TestNpcWalkTrigger_BoundaryQueueIDs` (lines 3307-3339+):** shift sub-tests:
- `"queueID=0"` sub-test → `"queueID=1"`, queueID `0`→`1`, expected walktrigger `-1`→`0`, comment "queueID=0 → walktrigger=-1 (queueID-1)" → "queueID=1 → walktrigger=0 (queueID-1, lower boundary)"
- `"queueID=19"` sub-test → `"queueID=20"`, queueID `19`→`20`, expected walktrigger `18`→`19`, comment parallel update

**14. `TestNpcWalkTrigger_PopOrderAndTransform` (lines 3285-3305):** unchanged (queueID=7 → walktrigger=6 still valid under new range).

## Out of scope

- **Compiler changes** — runtime-only per Design A pick. The compiler still emits `npc_queue` first-arg unchanged from source; no bytecode format change.
- **`hunt.FindNewMode` path at `npc_hunt.go:340-341`** — uses 0-indexed config field `NPCModeQueueN` enum + `objtype.NPCModeQueue1` constant offset. Different code path, no `+queueId-1` fencepost, no shift needed.
- **The `+queueId-1` arithmetic at the 2 call sites** — kept verbatim. The validator gate is sufficient; changing arithmetic would shift the whole bytecode-runtime contract for no behavior gain.
- **`objtype.NPCModeQueue*` constants** — unrelated to script-side queueId; these are config-driven hunt-mode fields.
- **Memory entry refresh for `queue_skincolour_validator_slice_close.md`** — point-in-time record per goscape convention; do not retroactively edit. The new close memory will cite the retirement.

## Risk

**Behavior changes at boundaries:**
- `queueID=0` was silently accepted and produced garbage trigger `TriggerAiQueue1 - 1`. Now rejected with explicit error.
- `queueID=20` was silently rejected. Now accepted and dispatches `TriggerAiQueue20` (previously unreachable from script).

**Real-world impact:** Zero, per prior LostCity/Content script audit captured in [[queue-skincolour-validator-slice-close]] (real first-args ∈ `{1..7, 10, 11, 12}` for `npc_queue`, `{8}` for `npc_walktrigger`). No script in the corpus uses 0 or 20.

**The widened upper bound admitting `TriggerAiQueue20`** is the "previously unreachable trigger becoming reachable" axis. A future content script that legitimately wants AI_QUEUE20 is now able to dispatch it (previously would have errored). This is a net positive — no behavior regression possible for existing scripts.

**The narrowed lower bound rejecting queueID=0** is the safety improvement. A future content bug that emits 0 (or a malformed bytecode) is now caught at validator-time with a clear error message instead of silently dispatching a garbage trigger.

## Pin board effect

- **Close:** `NAI-QUEUE-D-TS-FENCEPOST-INHERITED` (the inherited bug pin)
- **Open:** none — deliberate deviation gets inline doc-comment per `Player.Damage` precedent

Net: -1 live pin on the board.

## Gates

- `gofmt`
- `GOROOT=/home/owner/go/go1.26.3 ... go build ./...`
- `... go test -race ./...` (57+ pkgs, expect 0 FAIL)
- `TestPackAll_TwelveStageSmoke` (12 OK / 0 ERR / 0 SKIP)
- Audit-grep: `grep -rEn 'NAI-QUEUE-D-TS-FENCEPOST-INHERITED' --include='*.go'` should return **zero hits** post-edit (both production and test). All current references describe the live bug — once the bug is closed, the references become misleading. Provenance lives in this spec + the close memory entry; test-side retirement provenance would require fresh wording and is not worth the cognitive overhead vs. clean removal.

## Commit plan

Single commit:

```
fix(script): tighten AI-queue validator [0,19] → [1,20] — close NAI-QUEUE-D-TS-FENCEPOST-INHERITED

Deliberate deviation from TS-literal ScriptValidators.ts:114 ([0, 19])
to closed range [1, 20] inclusive. Eliminates the inherited TS
fencepost where queueId=0 produced TriggerAiQueue1-1 garbage via the
+queueId-1 arithmetic at NPC_QUEUE and NPC_WALKTRIGGER call sites.

Real-world impact zero: LostCity/Content script audit (per
queue_skincolour_validator_slice_close memory) shows actual first-args
∈ {1..7, 10, 11, 12} for npc_queue + {8} for npc_walktrigger. No
script uses 0 or 20. The corrected range now admits queueId=20 →
TriggerAiQueue20 (previously unreachable from script).

Per goscape convention for deliberate-deviation-for-correctness
(see Player.Damage negative-amount clamp precedent), no new
NAI-XXX-D-* pin is opened — inline doc-comment explains the
deviation at checkQueue.

Production: pkg/script/handlers_npc.go (validator body + 3 doc
comments).
Tests: pkg/script/handlers_npc_test.go (9 tests touched: range table,
OutOfRange-cases shift, 4 inverted boundary tests with renames,
1 above-threshold rename + 1 doc-only comment shift, 1 boundary
sub-test shift).
```

## References

- Predecessor close memory: `[[queue-skincolour-validator-slice-close]]` (`queue_skincolour_validator_slice_close.md`) — opened the pin, contains the script-audit data.
- Precedent for deviation-without-pin: `Player.Damage` doc-comment (`modules/world/player_masks.go:130-134`) — negative-amount clamp deviates from TS without a formal pin, explained inline.
- TS source: `LostCityRS/Engine-TS/src/engine/script/ScriptValidators.ts:114` (the QueueValid definition), `LostCityRS/RuneScriptTS/src/runescript/trigger/ServerTriggerType.ts:764-...` (AI_QUEUE1..20 enum).
