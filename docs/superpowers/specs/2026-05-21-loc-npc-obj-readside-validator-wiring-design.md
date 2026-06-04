# LocType/NpcType/ObjType read-side validator wiring (Shape A) — design

**Date:** 2026-05-21
**Status:** Draft
**Builds on:** [[registry-presence-validators-wiring-close]] (validator-wiring pattern, canonical wording), [[handlers-inv-readside-checkinvtype-wiring-close]] (sibling InvType slice — 12 sites in 1 file)
**Surfaced by:** [[inv-total-shortcircuit-reorder-close]] resume memo item #6 — "Other type-registry validator-vs-resolver gaps (LocType/NpcType/ObjType read-side opcodes)" audit. Audit motion identified 9 gaps total; this slice closes the 7 no-behavior-change ones.

## 1. Goal

Wire `checkNpcType` / `checkObjType` at every script-input call site outside `handlers_inv.go` / `handlers_config.go` that currently either (a) skips the registry check entirely, or (b) uses bespoke `"unknown X id"` wording. After this slice, every Npc/Obj type id reaching a Npc/Obj-shaped read-side opcode flows through the canonical validator first, matching TS `check(id, NpcTypeValid)` / `check(id, ObjTypeValid)`.

`handlers_loc.go` is **already clean** for read-side LocType wiring — no Loc-shaped sites in this slice. (All 7 in-scope handlers' LocType counterparts are already wired via [[hero-points-lifecycle-clear-close]] / [[registry-presence-validators-wiring-close]] / NAI-119 follow-up work.)

## 2. TS upstream

| Goscape site | TS reference | TS code |
|---|---|---|
| `handleNpcType` | `NpcOps.ts:260` | `state.pushInt(check(state.activeNpc.type, NpcTypeValid).id);` |
| `handleNpcChangeType` | `NpcOps.ts:459` | `const npcType: number = check(id, NpcTypeValid).id;` |
| `handleNpcChangeTypeKeepAll` | `NpcOps.ts:467` | `const npcType: number = check(id, NpcTypeValid).id;` |
| `handleObjName` | `ObjOps.ts:107` | `const objType = check(state.activeObj.type, ObjTypeValid); state.pushString(objType.name ?? objType.debugname ?? 'null');` |
| `handleObjParam` | `ObjOps.ts:98` | `const objType = check(state.activeObj.type, ObjTypeValid);` |
| `handleIfSetNpcHead` | `PlayerOps.ts:746` | `check(npc, NpcTypeValid);` |
| `handleIfSetObject` | `PlayerOps.ts:667` | `check(obj, ObjTypeValid);` |

`NpcTypeValid` / `ObjTypeValid` (`ScriptValidators.ts:111` / `:120`) are `ScriptInputConfigTypeValidator` instances over `NpcType` / `ObjType` — identical shape to InvTypeValid wired by [[handlers-inv-readside-checkinvtype-wiring-close]] and to the family wired by [[registry-presence-validators-wiring-close]]. Range check (`0 <= id < X.count`) collapses with registry-presence per `Configs` interface contract at `configs.go:7`.

## 3. Goscape current state

### 3.1 Validators

| Validator | File:Line | Canonical wording |
|---|---|---|
| `checkNpcType` | `handlers_npc.go:88-93` | `"%s: no NpcType with value (%d) found"` |
| `checkObjType` | `handlers_obj.go:44-49` | `"%s: no ObjType with value (%d) found"` |

Both state-aware (`s *ScriptState, id int, op string`); both absorb `s.Configs == nil` into the canonical error. Per project precedent ([[registry-presence-validators-wiring-close]] §5), `requireConfigs(s, op)` is layered BEFORE `checkXType` defensively even though `checkXType` handles nil-Configs internally.

### 3.2 Sibling precedents

| Precedent | File:Line | Shape match |
|---|---|---|
| `handleNpcParam` | `handlers_config.go:324-338` | `requireConfigs` → `requireActiveNpc` → `PopInt` → `ActiveNpc.NpcType()` → `checkNpcType` → field access — direct shape for NPC_TYPE |
| `handleOcName` | `handlers_config.go:450-466` | `requireConfigs` → `PopInt` → `checkObjType` → `s.Configs.ObjType(id)` local var → field access — direct shape for OBJ_NAME/OBJ_PARAM |
| `handleObjFind` | `handlers_obj.go:300-329` | `checkObjType` → downstream lookup with NO local var — direct shape for IF_SETOBJECT (id passed to delegate, not field-accessed) |

### 3.3 Baseline audit-grep counts at HEAD `4fd879b5`

```
grep -c "checkNpcType(s, " pkg/script/handlers_npc.go              → 5
grep -c "checkNpcType(s, " pkg/script/handlers_interface.go        → 0
grep -c "checkObjType(s, " pkg/script/handlers_obj.go              → 2
grep -c "checkObjType(s, " pkg/script/handlers_interface.go        → 0
grep -nE 'unknown obj id|unknown npc id' pkg/script/handlers_obj.go pkg/script/handlers_npc.go pkg/script/handlers_interface.go
  → 2 hits, both in handlers_obj.go (OBJ_NAME:388, OBJ_PARAM:414)
```

## 4. In-scope sites

### 4.1 `handlers_npc.go` — 3 sites

| # | Handler | Line | Opcode | Current shape | Post-slice shape |
|---|---|---|---|---|---|
| 1 | `handleNpcType` | `:180-187` | NPC_TYPE | `requireActiveNpc` → `PushInt(ActiveNpc.NpcType())` | + `requireConfigs` + `checkNpcType(id, "NPC_TYPE")` |
| 2 | `handleNpcChangeType` | `:358-366` | NPC_CHANGETYPE | `requireActiveNpc` → `PopInt×2` → `ActiveNpc.ChangeType` | + `requireConfigs` + `checkNpcType(newType, "NPC_CHANGETYPE")` |
| 3 | `handleNpcChangeTypeKeepAll` | `:371-378` | NPC_CHANGETYPE_KEEPALL | `requireActiveNpc` → `PopInt×2` → `ActiveNpc.ChangeTypeKeepAll` | + `requireConfigs` + `checkNpcType(newType, "NPC_CHANGETYPE_KEEPALL")` |

**Wiring pattern (NPC_TYPE example):**

```go
// BEFORE:
func handleNpcType(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_TYPE"); err != nil {
        return err
    }
    s.PushInt(s.ActiveNpc.NpcType())
    return nil
}

// AFTER:
func handleNpcType(s *ScriptState) error {
    if err := requireConfigs(s, "NPC_TYPE"); err != nil {
        return err
    }
    if err := requireActiveNpc(s, "NPC_TYPE"); err != nil {
        return err
    }
    id := s.ActiveNpc.NpcType()
    if err := checkNpcType(s, id, "NPC_TYPE"); err != nil {
        return err
    }
    s.PushInt(id)
    return nil
}
```

**No defensive comment** required for these 3 — they don't preserve a downstream Configs lookup with a fallthrough; once the validator passes, the next operation is the action (push id, or delegate to `ActiveNpc.ChangeType[KeepAll]`).

### 4.2 `handlers_obj.go` — 2 sites

| # | Handler | Line | Opcode | Current shape | Post-slice shape |
|---|---|---|---|---|---|
| 4 | `handleObjName` | `:379-397` | OBJ_NAME | `requireActiveObj` → `requireConfigs` → `Configs.ObjType` → bespoke nil error → field access | swap bespoke nil error for `checkObjType(id, "OBJ_NAME")` BEFORE the Configs lookup; preserve `ot` local var |
| 5 | `handleObjParam` | `:404-417` | OBJ_PARAM | `requireActiveObj` → `requireConfigs` → `PopInt` → `Configs.ObjType` → bespoke nil error → `paramLookup` | same: `checkObjType` BEFORE Configs lookup; preserve `ot` local var |

**Wiring pattern (OBJ_NAME example):**

```go
// BEFORE:
func handleObjName(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_NAME"); err != nil {
        return err
    }
    if err := requireConfigs(s, "OBJ_NAME"); err != nil {
        return err
    }
    ot := s.Configs.ObjType(s.ActiveObj.ObjType())
    if ot == nil {
        return fmt.Errorf("OBJ_NAME: unknown obj id %d", s.ActiveObj.ObjType())
    }
    // ... field access ...
}

// AFTER:
func handleObjName(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_NAME"); err != nil {
        return err
    }
    if err := requireConfigs(s, "OBJ_NAME"); err != nil {
        return err
    }
    id := s.ActiveObj.ObjType()
    if err := checkObjType(s, id, "OBJ_NAME"); err != nil {
        return err
    }
    ot := s.Configs.ObjType(id)
    // ... field access ...
}
```

The bespoke `if ot == nil { return ... }` block is **deleted outright** (not replaced with a defensive fallthrough) — `checkObjType` is the same lookup, so `ot` post-check is guaranteed non-nil. This matches the `handleOcName` precedent at `handlers_config.go:450-466` exactly.

**Why no defensive fallthrough here** (vs the InvType slice's `// Defensive: unreachable post-checkInvType ...` comment): the InvType slice preserved the defensive block because `resolveInv` had a *separate* nil-failure mode (`s.Inv == nil`) distinct from the registry check. OBJ_NAME/OBJ_PARAM have no such separate failure mode — the only nil source for `ot` is registry miss, which `checkObjType` already covers.

### 4.3 `handlers_interface.go` — 2 sites

| # | Handler | Line | Opcode | Current shape | Post-slice shape |
|---|---|---|---|---|---|
| 6 | `handleIfSetNpcHead` | `:189-201` | IF_SETNPCHEAD | `Pointers/Self` gate → `PopInt×2` → `checkNotNull(com)` → `Self.IfSetNpcHead` | + `requireConfigs` + `checkNpcType(npc, "IF_SETNPCHEAD")` after `checkNotNull(com)`; delete acknowledged-gap comment at `:198` |
| 7 | `handleIfSetObject` | `:279-295` | IF_SETOBJECT | `Pointers/Self` gate → `PopInt×3` → `checkNotNull(com)` → `checkNotNull(scale)` → `Self.IfSetObject` | + `requireConfigs` + `checkObjType(obj, "IF_SETOBJECT")` between the two checkNotNull calls; delete acknowledged-gap comment at `:289` |

**Wiring pattern (IF_SETNPCHEAD example):**

```go
// BEFORE (handlers_interface.go:189-201):
func handleIfSetNpcHead(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("IF_SETNPCHEAD: no active player")
    }
    npc := s.PopInt()
    com := s.PopInt()
    if err := checkNotNull(com, "IF_SETNPCHEAD"); err != nil {
        return err
    }
    // npc uses NpcTypeValid in TS (not NumberNotNull); no checkNotNull here (NAI-23 Bundle 4c).
    s.Self.IfSetNpcHead(com, npc)
    return nil
}

// AFTER:
func handleIfSetNpcHead(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("IF_SETNPCHEAD: no active player")
    }
    if err := requireConfigs(s, "IF_SETNPCHEAD"); err != nil {
        return err
    }
    npc := s.PopInt()
    com := s.PopInt()
    if err := checkNotNull(com, "IF_SETNPCHEAD"); err != nil {
        return err
    }
    if err := checkNpcType(s, npc, "IF_SETNPCHEAD"); err != nil {
        return err
    }
    s.Self.IfSetNpcHead(com, npc)
    return nil
}
```

The "no checkNotNull here (NAI-23 Bundle 4c)" stale comment is deleted — it was a forward-looking placeholder noting that NpcTypeValid was the right shape but the validator wasn't yet wired. Now wired, the comment is dead.

**No local var preservation** needed for either: both delegate the id directly to `Self.IfSet…` without field-accessing the registry entry. Mirrors `handleObjFind` "no local var" exception from [[registry-presence-validators-wiring-close]] §5.4.

## 5. Out of scope

### 5.1 Shape B sites (NPC_NAME, NPC_CATEGORY) — 2 sites

`handleNpcName` (`handlers_npc.go:234-255`) and `handleNpcCategory` (`handlers_npc.go:297-312`) currently silent-fallback on `cfg == nil`: NPC_NAME pushes `"null"`, NPC_CATEGORY pushes `-1`. Wiring `checkNpcType` would convert silent fallback to error throw — a **behavior-changing** semantic divergence.

Possible motivations for the silent fallback (unverified at spec-time):
1. NPC_NAME's `"null"` is a TS-style nullish-coalesce default that historically tolerated unregistered/morphed types.
2. NPC_CATEGORY's `-1` matches LocType.Category default; some scripts may depend on `npc_category = -1` for unconfigured NPCs.

Closing these requires brainstorm + verification that no script depends on the silent path. Deferred as separate XS-S brainstorm-shaped slice.

### 5.2 OBJ_TYPE intentional elision (`handlers_obj.go:430-436`)

Documented at `:425-429` as upstream-invariant defensive: `ActiveObj` is constructed via wire handlers that pre-validate, so re-validation is redundant. Spec-conformant per project convention; not a gap.

### 5.3 NPC_DEL cached `Respawnrate()` (`handlers_npc.go:420-429`)

TS does `check(state.activeNpc.type, NpcTypeValid).respawnrate` (registry lookup). Goscape uses `ActiveNpc.Respawnrate()` — a value cached on the Npc at creation/ChangeType. For valid types the two paths return identical values; for invalid types TS throws and goscape silently uses the cached value. With this slice's NPC_CHANGETYPE wiring closed, invalid types can no longer enter ActiveNpc.NpcType() via the script path, so the divergence shrinks to pre-NAI-engine edge cases. Not in scope.

### 5.4 MAP_LOCADDUNSAFE filter-shape elision (`handlers_map.go:340-394`)

Already documented at `:335-339` as intentional goscape-defensive deviation (silent skip vs TS throw on malformed loc cache entries). Spec-conformant per project convention; not a gap.

### 5.5 `handleNpcDel` World-nil defensive guard (DEVIATION-NAI-126-D1)

Already a tracked deviation; orthogonal to validator wiring.

## 6. Tests

### 6.1 Existing assertions to flip (2)

| File | Test | Line | Current | Post-slice |
|---|---|---|---|---|
| `handlers_obj_test.go` | `TestObjNameUnknownType` | `:1198-1199` | `"unknown obj id"` | `"no ObjType with value"` |
| `handlers_obj_test.go` | `TestObjParamUnknownType` | `:1340-1341` | `"unknown obj id"` | `"no ObjType with value"` |

Both tests register an unknown `objType: 999` against `newTestConfigs()` (which does NOT register 999), then call the handler and assert the error wording. The structural assertions (error not nil, contains `"OBJ_NAME"` / `"OBJ_PARAM"`) stay unchanged.

### 6.2 New tests

**None required.** Per [[handlers-inv-readside-checkinvtype-wiring-close]] non-obvious finding #3, validator-layer coverage at `TestCheckNpcType` / `TestCheckObjType` (`handlers_npc_test.go` / `handlers_obj_test.go` checkType test functions) is sufficient — every wired site flows through the same validator. Handler-layer "unknown id" assertions for the 5 sites that had no prior wording (NPC_TYPE, NPC_CHANGETYPE, NPC_CHANGETYPE_KEEPALL, IF_SETNPCHEAD, IF_SETOBJECT) would be redundant.

### 6.3 Impl-time audit-grep

Before commit, grep for:

- `"unknown obj id"` in `pkg/script/handlers_*.go` → expect 0 hits (down from 2).
- `"unknown obj id"` in `pkg/script/handlers_*_test.go` → expect 0 hits at the 2 flipped sites (the existing tests are the only hits).
- Any new `"unknown npc id"` introductions outside this slice's scope.
- Acknowledged-gap comments (`"no checkNotNull here (NAI-23 Bundle 4c)"`) at `handlers_interface.go:198,289` → expect 0 hits post-slice.

## 7. Risk / rollback

- **Behavior change:** error wording changes for 2 OBJ_NAME/OBJ_PARAM registry-miss paths (`"unknown obj id"` → `"no ObjType with value"`). For NPC_TYPE/NPC_CHANGETYPE/NPC_CHANGETYPE_KEEPALL/IF_SETNPCHEAD/IF_SETOBJECT, invalid type ids that previously pass through silently now produce canonical errors — but no test currently asserts the silent-pass behavior, and the canonical error path is the TS-faithful behavior.
- **No runtime perf cost:** `checkXType` is an O(1) map lookup; each handler now does one extra lookup per call, but the same lookup was already happening (or would have happened) post-validator at field access.
- **Test impact:** 2 assertion flips total, both in `handlers_obj_test.go`. Zero new tests.
- **Rollback:** trivial — `git revert` of the impl commit.

## 8. Gates

- `gofmt -l` clean on every edited file.
- `go test -race ./...` 0 FAIL.
- `go test -run TestPackAll_TwelveStageSmoke` PASS.
- Audit-grep expected post-slice (deltas vs HEAD `4fd879b5`):
  - `grep -c "checkNpcType(s, " pkg/script/handlers_npc.go` → expect **8** (+3: NPC_TYPE, NPC_CHANGETYPE, NPC_CHANGETYPE_KEEPALL).
  - `grep -c "checkNpcType(s, " pkg/script/handlers_interface.go` → expect **1** (+1: IF_SETNPCHEAD).
  - `grep -c "checkObjType(s, " pkg/script/handlers_obj.go` → expect **4** (+2: OBJ_NAME, OBJ_PARAM).
  - `grep -c "checkObjType(s, " pkg/script/handlers_interface.go` → expect **1** (+1: IF_SETOBJECT).
  - `grep -cE "unknown obj id|unknown npc id" pkg/script/handlers_npc.go pkg/script/handlers_obj.go pkg/script/handlers_interface.go` → expect **0** (−2: OBJ_NAME, OBJ_PARAM bespoke wordings canonicalized).
  - `grep -c '"no checkNotNull here (NAI-23 Bundle 4c)"' pkg/script/handlers_interface.go` → expect **0** (−2: deleted from IF_SETNPCHEAD, IF_SETOBJECT).
  - Total new `checkNpcType` calls: **+4**; total new `checkObjType` calls: **+3**.

## 9. Cadence

Single sonnet implementer + two-stage review (sonnet spec-conformance + sonnet code-quality) + opus whole-slice. Single task, single commit. Pre-existing validators + sibling precedents + minimal test churn justifies lighter cadence than [[registry-presence-validators-wiring-close]]'s 4-task split.

Refactor-shaped + pre-existing validator + sibling precedent matches the smoothest-cadence pattern from [[handlers-inv-readside-checkinvtype-wiring-close]] non-obvious finding #6 ("FOURTH consecutive clean cadence").

Implementation plan written separately via `writing-plans` skill.

## 10. Carry-forward delta

Net carry-forward menu after this slice:
- −1 item retired (Other type-registry validator-vs-resolver gaps — LocType/NpcType/ObjType read-side opcodes, **Shape A subset**).
- +1 item surfaced (Shape B subset: NPC_NAME/NPC_CATEGORY silent-fallback semantics — XS-S brainstorm slice when prioritized).

Remaining menu (post-slice): NAI-162 analytics RPC, combat-level read-site verification, deviation audit refresh, general world/runescript, OC_* Part B + most NC_* bespoke-unknown-id test coverage gap, **NPC_NAME/NPC_CATEGORY silent-fallback semantic divergence** (NEW), `handleInvTotalParamStack` player-impl InvType audit (carry-forward from [[handlers-inv-readside-checkinvtype-wiring-close]] §5.1).
