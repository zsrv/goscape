# Registry-presence validators wiring (Inv/Npc/Obj) — design spec

**Date:** 2026-05-21
**Predecessor close memory:** `[[doc-comment-sweep-close]]` (slice 2 of 4-sequential-mini-slices bundle); validator-family precedent: `[[config-registry-validator-family-close]]`
**Branch base:** `main` at `b380f52c`

## 1. Goal

Wire the existing `checkNpcType` / `checkObjType` validators at the 25 remaining inline `Configs.X(id) == nil` script-input call sites across `handlers_config.go` (24 sites) + `handlers_obj.go` (1 site). Pure refactor — same error semantics, canonical error wording.

This closes the **last open slice (slice 4 of 4)** in the 2026-05-21 sequential mini-slices bundle (slices 1/2 shipped, slice 3 retired as phantom gap).

## 2. Scope

### In scope

- Wire `checkNpcType(s, id, "OP")` at 9 `NC_*` and `NPC_PARAM` handler sites in `handlers_config.go`.
- Wire `checkObjType(s, id, "OP")` at 15 `OC_*` handler sites in `handlers_config.go`.
- Wire `checkObjType(s, objId, "OBJ_FIND")` at 1 site in `handlers_obj.go:313`.
- Adopt canonical error wording `"%s: no NpcType with value (%d) found"` / `"%s: no ObjType with value (%d) found"` (replaces 25 bespoke `"unknown npc id %d"` / `"unknown obj id %d"` strings).
- Add `TestCheckObjType` to `handlers_obj_test.go` (sibling-location to `checkObjType` per file-pairing convention; matches predecessor `TestCheckNpcType` at `handlers_npc_test.go:55`).
- Update 25 handler-test assertion strings to canonical wording.

**Total: 25 production wires + 25 handler-test wording updates + 1 new `TestCheckObjType` unit test.**

### Out of scope (deferred)

- **InvType opcode wiring follow-up.** `checkInvType` is already extensively wired across `handlers_inv.go` (BUILDAPPEARANCE + 20+ INV_* sites). A separate audit may find additional script-input call sites that warrant `checkInvType` wiring; outside this slice.
- **handlers_inv.go data-integrity guards.** 6 sites with `"invalid obj/inv id at slot"` (INV_DROPSLOT, BOTH_DROPSLOT, INV_DROPALL, BOTH_MOVEINV). These are data-integrity guards on stored `inv[slot]` contents — a different error class from script-input registry-presence (same class distinction predecessor preserved with the 2 retained `"param lookup:"` data-corruption guards).
- **`s.ActiveNpc.NpcType()` / `s.ActiveObj.ObjType()` reads.** ID comes from stored entity state, not script input. Examples: `handlers_obj.go` OBJ_NAME (:386-388), OBJ_PARAM (:410-414); `handlers_npc.go` :241/:272/:305/:1333; `handlers_player.go` :1365.
- **Defensive labeled gates.** None exist in the 25 enumerated sites — predecessor's labeled-defensive convention (`defensive_gate_doc_comment_label.md`) does not apply here.

## 3. Predecessor framing correction

**Important inheritance bug.** The predecessor `[[config-registry-validator-family-close]]` close memo §10 listed this slice as carry-forward item #7 with the framing:

> "Wider validator-family ports: InvType/NpcType/ObjType **registry-presence** validator family (sibling to this slice — these have `count` constants pre-loaded so range-only-no-registry, **distinct shape** from the Loc/Param/Enum/Struct/Idk/Mesanim/Font registry-only family this slice closed)"

This framing was **wrong**. Empirical TS-side verification at `Engine-TS/src/engine/script/ScriptValidators.ts`:

| Line | Validator | TS shape |
|---|---|---|
| 111 | `NpcTypeValid` | `ScriptInputConfigTypeValidator(NpcType.get, (input) => input >= 0 && input < NpcType.count, 'Npc')` |
| 120 | `ObjTypeValid` | `ScriptInputConfigTypeValidator(ObjType.get, (input) => input >= 0 && input < ObjType.count, 'Obj')` |
| 122 | `InvTypeValid` | `ScriptInputConfigTypeValidator(InvType.get, (input) => input >= 0 && input < InvType.count, 'Inv')` |

These are **identical** `ScriptInputConfigTypeValidator` shape to the family the predecessor closed. The `< X.count` bound collapses with the registry-presence check via the goscape `Configs` interface contract at `pkg/script/configs.go:7` ("return nil when the type isn't loaded or the id is out of range").

The wrong framing was inherited unchanged across `[[low-arg-shape-pin-close]]` / `[[doc-comment-sweep-close]]` / `[[hit-splat-multi-hit-phantom-gap]]` carry-forward menus. This slice's brainstorm caught and corrected it.

## 4. Wiring pattern

Mirror predecessor's exact pattern at every site:

**Before:**

```go
id := s.PopInt()
nt := s.Configs.NpcType(id)
if nt == nil {
    return fmt.Errorf("NC_NAME: unknown npc id %d", id)
}
// ... use nt
```

**After:**

```go
id := s.PopInt()
if err := checkNpcType(s, id, "NC_NAME"); err != nil {
    return err
}
nt := s.Configs.NpcType(id)  // guaranteed non-nil
// ... use nt
```

**No behavior change** at any site:
- `requireConfigs` early-return retained (its sentinel runs FIRST; `checkXType`'s redundant `Configs == nil` second-layer check is harmless when handler already early-returned).
- Local var (`nt` / `ot`) RETAINED at every wired site — every handler accesses one or more fields on the looked-up type (Name, Category, Desc, Params, Tradeable, CertLink, etc.). The predecessor's "dead-var removal" pattern (T2 `handleLocType` opt-in) does **not apply** here because LOC_TYPE was a degenerate-no-field-access site; the 25 sites in this slice all access fields.
- Error wording canonicalized: `"OP: unknown npc id %d"` → `"OP: no NpcType with value (%d) found"`; same for obj. No error code change, no error-class change.

## 5. Site enumeration

All line numbers verified at brainstorm time against `HEAD=b380f52c`. Per the predecessor `[[doc-comment-sweep-close]]` non-obvious finding #1, re-verify before commit if any in-flight edit lands first.

### 5.1 NC_* + NPC_PARAM family — 9 sites in `handlers_config.go` (Task 1)

| # | Handler | Func line | Wire pre-test at | Op name | Fields used post-wire |
|---|---|---|---|---|---|
| 1 | `handleNcName` | :287 | :292-295 | `NC_NAME` | `nt.Name`, `nt.DebugName` |
| 2 | `handleNcParam` | :307 | :313-316 | `NC_PARAM` | `nt.Params` (via `paramLookup`) |
| 3 | `handleNpcParam` | :324 | :333-336 | `NPC_PARAM` | `nt.Params` (via `paramLookup`) |
| 4 | `handleNcCategory` | :341 | :346-349 | `NC_CATEGORY` | `nt.Category` |
| 5 | `handleNcDesc` | :355 | :360-363 | `NC_DESC` | `nt.Desc` |
| 6 | `handleNcDebugName` | :373 | :378-381 | `NC_DEBUGNAME` | `nt.DebugName` |
| 7 | `handleNcOp` | :395 | :404-407 | `NC_OP` | `nt.Op` |
| 8 | `handleNcSize` | :418 | :423-426 | `NC_SIZE` | `nt.Size` |
| 9 | `handleNcVisLevel` | :433 | :438-441 | `NC_VISLEVEL` | `nt.VisLevel` |

**Note:** `NPC_PARAM` (handler #3) uses ID from `s.ActiveNpc.NpcType()` (stored entity state, not script input). The existing nil-check at :334 is still useful defense-in-depth — wiring `checkNpcType` here is borderline (active-entity reads are explicitly excluded in §2). **Inclusion rationale:** the existing inline guard is already there with bespoke `"unknown npc id %d"` wording; wiring through `checkNpcType` canonicalizes the wording without altering the data-source semantics. Treated as a wording-unification site, not a script-input-validation expansion.

### 5.2 OC_* family Part A — 10 sites in `handlers_config.go` (Task 2)

| # | Handler | Func line | Wire pre-test at | Op name | Fields used post-wire |
|---|---|---|---|---|---|
| 1 | `handleOcName` | :450 | :455-458 | `OC_NAME` | `ot.Name`, `ot.DebugName` |
| 2 | `handleOcParam` | :470 | :476-479 | `OC_PARAM` | `ot.Params` (via `paramLookup`) |
| 3 | `handleOcCategory` | :484 | :489-492 | `OC_CATEGORY` | `ot.Category` |
| 4 | `handleOcDesc` | :498 | :503-506 | `OC_DESC` | `ot.Desc` |
| 5 | `handleOcMembers` | :516 | :521-524 | `OC_MEMBERS` | `ot.Members` |
| 6 | `handleOcWeight` | :534 | :539-542 | `OC_WEIGHT` | `ot.Weight` |
| 7 | `handleOcWearPos` | :548 | :553-556 | `OC_WEARPOS` | `ot.WearPos` |
| 8 | `handleOcWearPos2` | :562 | :567-570 | `OC_WEARPOS2` | `ot.WearPos2` |
| 9 | `handleOcWearPos3` | :576 | :581-584 | `OC_WEARPOS3` | `ot.WearPos3` |
| 10 | `handleOcCost` | :590 | :595-598 | `OC_COST` | `ot.Cost` |

### 5.3 OC_* family Part B — 5 sites in `handlers_config.go` (Task 3)

| # | Handler | Func line | Wire pre-test at | Op name | Fields used post-wire |
|---|---|---|---|---|---|
| 1 | `handleOcTradeable` | :604 | :609-612 | `OC_TRADEABLE` | `ot.Tradeable` |
| 2 | `handleOcDebugName` | :622 | :627-630 | `OC_DEBUGNAME` | `ot.DebugName` |
| 3 | `handleOcCert` | :644 | :649-652 | `OC_CERT` | `ot.CertTemplate`, `ot.CertLink`, `ot.ID` |
| 4 | `handleOcUncert` | :666 | :671-674 | `OC_UNCERT` | `ot.CertTemplate`, `ot.CertLink`, `ot.ID` |
| 5 | `handleOcStackable` | :684 | :689-692 | `OC_STACKABLE` | `ot.Stackable` |

**Resume-memo correction:** the resume memo claimed T3 had "6 sites" with "(one more OC_*, audit at impl time)" placeholder. Actual count is **5** — `handleOcStackable` (:684) is the last `OC_*` handler in the file. T3 enumeration is closed at 5.

### 5.4 OBJ_FIND — 1 site in `handlers_obj.go` (Task 4)

| # | Handler | Func line | Wire pre-test at | Op name | Notes |
|---|---|---|---|---|---|
| 1 | `handleObjFind` | :300 | :313-315 | `OBJ_FIND` | `objId` is from `PopInt()` (script input). Nil-check at :313 is inline `s.Configs.ObjType(objId) == nil` — replace with `checkObjType` pre-test. No local var var existed (was `s.Configs.ObjType(objId)` direct nil-check) — no var to preserve. |

## 6. Non-candidate sites (explicitly NOT wired)

Documented for audit completeness. NO code change at these sites; T4 OPTIONALLY adds a brief doc-comment clarifying the exclusion class at one representative site of each kind (NOT all of them).

### 6.1 Data-integrity guards on stored inventory contents

`handlers_inv.go` — 6 sites (lines pending verification at impl time):
- INV_DROPSLOT, BOTH_DROPSLOT, INV_DROPALL, BOTH_MOVEINV variants.
- Error class: `"invalid obj/inv id at slot %d"` — guards corruption of stored `inv[slot]`, NOT script-input validation.
- Same class distinction predecessor preserved at `handlers_config.go:27/:33` (`"param lookup: ... expected string, got %T"`).

### 6.2 Stored-entity-state reads (`ActiveNpc.NpcType()` / `ActiveObj.ObjType()`)

- `handlers_obj.go` OBJ_NAME (:386-388), OBJ_PARAM (:410-414) — `s.ActiveObj.ObjType()`.
- `handlers_npc.go` :241, :272, :305, :1333 — `s.ActiveNpc.NpcType()`.
- `handlers_player.go` :1365 — `s.ActiveObj.ObjType()`.
- ID source is stored entity state, not script input. Wiring through script-input validator would be wrong semantically.

## 7. Tests

### 7.1 New validator unit test

**`TestCheckObjType`** in `handlers_obj_test.go` — sibling-location to `checkObjType` per file-pairing convention (predecessor T1 reviewer Minor finding `fed7bf02` corrected validator unit tests to their def-file's paired test file).

Mirror `TestCheckNpcType` at `handlers_npc_test.go:55` exactly: 4 sub-cases (valid id / unknown id / negative id / nil Configs).

**No new `TestCheckNpcType`** — already exists at `handlers_npc_test.go:55`.

**No new `TestCheckInvType`** — already exists at `handlers_player_test.go:2360`; this slice does not add InvType wires.

### 7.2 Handler-test wording updates

25 existing handler-test error-string assertions flipped from `"unknown npc id %d"` / `"unknown obj id %d"` to canonical `"no NpcType with value (%d) found"` / `"no ObjType with value (%d) found"`.

Test-file distribution (estimated, verify at impl time):
- `handlers_config_test.go` — 24 NC_* + OC_* assertions.
- `handlers_obj_test.go` — 1 OBJ_FIND assertion.

## 8. Gates

- `gofmt -l pkg/script/handlers_config.go pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go pkg/script/handlers_config_test.go` clean post-slice.
- `go test -race ./pkg/script/ ./modules/world/` clean (modules/world the long pole at ~155s).
- `TestPackAll_TwelveStageSmoke` PASS (smoke-test gate).
- Audit-grep zero-hit post-slice:
  - `"unknown npc id %d"` → 0 hits in pkg/script production (only in `handlers_inv.go` data-integrity guards if any).
  - `"unknown obj id %d"` → 0 hits in pkg/script production (only in `handlers_inv.go` data-integrity guards if any).
- Audit-grep canonical-wording presence:
  - `"no NpcType with value"` → ≥25 hits across production + tests (9 NC wires + 9 test assertions + existing `handlers_npc.go` validator + 9 OC unrelated context, etc.).
  - `"no ObjType with value"` → ≥27 hits (15 OC wires + 1 OBJ_FIND + 15+1 test assertions + new TestCheckObjType cases).

## 9. Deviations / pins

**Zero `NAI-XXX-D-*` pin churn.** This is a pure refactor + uniform error wording — same shape as predecessor `[[config-registry-validator-family-close]]` which opened/retired zero pins.

## 10. Execution cadence

Mirror `[[hero-points-lifecycle-clear-close]]` precedent (most recent clean subagent-driven slice):

- 4 tasks T1-T4 (NC_* / OC_*-A / OC_*-B / OBJ_FIND + non-candidate doc-comments).
- Sonnet ×4 implementers, one task each.
- Per-task two-stage review: sonnet spec-reviewer + sonnet code-quality reviewer.
- Opus whole-slice reviewer at close.
- Estimated 4-5 impl commits + 1 spec commit + 1 plan commit + close memo.

## 11. Non-obvious caveats (carry forward to plan)

- **`NPC_PARAM` ID-source semantic.** Handler #3 in §5.1 reads `ActiveNpc.NpcType()` (stored entity state) but has a bespoke `"unknown npc id %d"` guard with the same wording shape as the script-input handlers. Wire-through is for wording-canonicalization only; do not interpret as semantic expansion of script-input validation.
- **`OBJ_FIND` has no local var.** Site at :313 uses `if s.Configs.ObjType(objId) == nil { ... }` directly without a local `ot` var (handler doesn't access type fields — just gates the World lookup). The post-wire code does NOT re-introduce a local var.
- **No `defensive_gate_doc_comment_label.md` sites in the 25.** Every site has an active error-return guard, no silent-skip. Predecessor's "don't wire labeled defensive gates" rule does not apply.
- **`paramLookup` cascade already done.** Predecessor T3 cascaded the `op string` 4th arg through all 7 callers including `handleNcParam` / `handleNpcParam` / `handleOcParam`. Verify at impl time but no signature change expected this slice.
- **Compile-time interface gate** at `modules/world/message_game.go:11` is not relevant — no interface widening expected.
