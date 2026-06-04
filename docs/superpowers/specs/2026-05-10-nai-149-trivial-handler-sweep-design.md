---
status: brainstorm-approved
date: 2026-05-10
ts_source:
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1057-1062 (AFK_EVENT)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1180-1182 (WEIGHT)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1050-1054 (HEALENERGY)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1121-1124 (SETSKINCOLOUR)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1211-1213 (PLAYERMEMBER)
  - LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts:34-38 (INV_DEBUGNAME)
  - LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts:41-54 (INV_STOCKBASE)
  - LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:109-111 (SEQLENGTH)
---

# NAI-149 — Trivial-handler sweep (8 ops; 3-from-logs + 5 cohort)

**Cadence:** 100-300 LOC band (8 handlers + ~5 ActivePlayer methods + tests + 1 helper) — separate spec + plan, single combined Sonnet reviewer at end-of-impl. Subagent-driven-development per `execution_mode_default.md`.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Cascade-tail context:** missing-handler audit at HEAD 8aa61b1 reports **47 unhandled opcodes** (`missing_handler_audit.md` one-liner). This sub-spec ports 8; remaining 39 stay forward-routable to NAI-150+.

---

## §1 Symptom / motivation

User-reported smoke logs (2026-05-09/10) show repeated `script execute error … no handler for X` WARN spam:

```
err="script \"[proc,bank_check_nobreak]\": no handler for PLAYERMEMBER (opcode 2090) at pc=11"
err="script \"[proc,price_mod]\": no handler for INV_STOCKBASE (opcode 4325) at pc=2"
err="script \"[label,attempt_pick_pocket]\": no handler for AFK_EVENT (opcode 2000) at pc=22"
err="script \"[proc,npc_projectile]\": no handler for PROJANIM_NPC (opcode 2546) at pc=20"
```

**Scope decision** (per `smoke_surfaces_adjacent_divergences.md` + brainstorm 2026-05-10):

- **3 of 4 in scope:** `PLAYERMEMBER`, `INV_STOCKBASE`, `AFK_EVENT` — all are 1-3-line config-lookup or field-read shapes; share the "trivial single-statement port" cohort with 5 sibling unhandled opcodes (`WEIGHT`, `HEAL_ENERGY`, `SEQ_LENGTH`, `INV_DEBUG_NAME`, `SETSKINCOLOUR`) all confirmed against TS source as 1-5-line bodies with infra already present in goscape.
- **PROJANIM_NPC out of scope, deferred to NAI-150:** body needs `Zone.MapProjAnim` wiring (exists at `pkg/zone/zone.go:355`) plus an NPC-by-slot lookup; not a 1-statement port. Routed forward per `smoke_surfaces_adjacent_divergences` ≤30-LOC heuristic.

**Excluded "near-trivial" candidates** (with reasons; deferred to later NAIs):

| Op | Defer reason |
|---|---|
| `MAP_INDOORS` | TS calls `isIndoors(x,z,level)` helper. **Helper does not exist anywhere in goscape** (rg `IsIndoors\|isIndoors` returned 0 hits). New map-flag/clip infra is its own bundle. |
| `LAST_LOGIN_INFO` | TS calls `p.lastLoginInfo()` method which surfaces a login-info dialog via packet. Method does not exist on goscape Player. |
| `SET_GENDER` | Body-mapping tables `MALE_FEMALE_MAP` / `FEMALE_MALE_MAP` (Player.ts) need porting; loops + idkit conversion is its own bundle. |
| `WEALTH_EVENT` | Adds RPC payload (`addWealthEvent` API surface) plus `ObjType.getByName` lookup. Outside cohort. |

## §2 Architecture

### §2.1 New `pkg/script.ActivePlayer` methods

Mirroring the existing accessor convention (each is field-read pass-through to `modules/world.player` — no logic):

| Method | Type | Backing field | Setter? |
|---|---|---|---|
| `Members() bool` | bool | `player.members` | no |
| `RunWeight() int` | int | `player.runweight` | no |
| `AfkEventReady() bool` | bool | `player.afkEventReady` | yes — `SetAfkEventReady(v bool)` |
| `SetRunEnergy(v int)` | — | `player.runenergy` | (paired with existing `RunEnergy() int`) |

`SetColorPart(slot, color int)` already exists (`pkg/script/active.go:631`) — SETSKINCOLOUR reuses it.

`StaffModLevel() int32` already exists (`active.go:420`) — AFK_EVENT reuses it.

`NodeDebug` already exposed via `s.NodeDebug` per-state field (`pkg/script/state.go:229`).

### §2.2 New check helper

Add to `pkg/script/check.go` (or wherever `checkInvType` lives):

```go
// checkSeqType returns the SeqType for id, or an error if the lookup
// returns nil. Mirrors TS check(id, SeqTypeValid) (ScriptValidators.ts).
func checkSeqType(s *ScriptState, id int, op string) (*objtype.SeqType, error) {
    st := s.Configs.SeqType(id)
    if st == nil {
        return nil, fmt.Errorf("%s: invalid seq type %d", op, id)
    }
    return st, nil
}
```

(Plan-author MUST grep `checkInvType\b` and reproduce its exact pattern — see `plan_grep_helper_patterns.md` and `plan_sibling_site_guard_audit.md`.)

### §2.3 Handler bodies (per opcode)

All handlers register in `pkg/script/handlers.go` map and live in the existing per-domain file (`handlers_player.go`, `handlers_inv.go`, `handlers_server.go`).

**1. `handlePlayerMember` → OpPlayerMember** (`handlers_player.go`)

```go
// TS PlayerOps.ts:1211-1213 — checkedHandler(ActivePlayer, …)
//   state.pushInt(state.activePlayer.members ? 1 : 0)
func handlePlayerMember(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("PLAYERMEMBER: no active player")
    }
    if s.Self.Members() {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    return nil
}
```

**2. `handleAfkEvent` → OpAfkEvent** (`handlers_player.go`)

```go
// TS PlayerOps.ts:1057-1062 — NO checkedHandler in TS (would NPE on
// missing activePlayer; goscape adds defensive guard, see DEVIATION
// label below).
//   state.pushInt(
//     (Environment.NODE_DEBUG || state.activePlayer.staffModLevel < 2)
//       && state.activePlayer.afkEventReady ? 1 : 0
//   )
//   state.activePlayer.afkEventReady = false
func handleAfkEvent(s *ScriptState) error {
    // (goscape defensive; TS skips this check — see defensive_gate_doc_comment_label)
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("AFK_EVENT: no active player")
    }
    eligible := (s.NodeDebug || s.Self.StaffModLevel() < 2) && s.Self.AfkEventReady()
    if eligible {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    s.Self.SetAfkEventReady(false)
    return nil
}
```

**3. `handleWeight` → OpWeight** (`handlers_player.go`)

```go
// TS PlayerOps.ts:1180-1182 — checkedHandler(ProtectedActivePlayer, …)
func handleWeight(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("WEIGHT: no active player")
    }
    if s.Pointers&PtrProtectedActivePlayer == 0 {
        return errors.New("WEIGHT: requires protected access")
    }
    s.PushInt(s.Self.RunWeight())
    return nil
}
```

**4. `handleHealEnergy` → OpHealEnergy** (`handlers_player.go`)

```go
// TS PlayerOps.ts:1050-1054 — no checkedHandler. Pops amount,
// validates NumberNotNull (≠0), clamps runenergy + amount to [0,10000].
//   const amount = check(state.popInt(), NumberNotNull) // 100=1%, 10000=100%
//   player.runenergy = Math.min(Math.max(player.runenergy + amount, 0), 10000)
func handleHealEnergy(s *ScriptState) error {
    // (goscape defensive; TS skips active-player check)
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("HEAL_ENERGY: no active player")
    }
    amount := s.PopInt()
    if err := checkNotNull(amount, "HEAL_ENERGY"); err != nil {
        return err
    }
    next := s.Self.RunEnergy() + amount
    if next < 0 {
        next = 0
    } else if next > 10000 {
        next = 10000
    }
    s.Self.SetRunEnergy(next)
    return nil
}
```

**5. `handleSeqLength` → OpSeqLength** (`handlers_server.go`)

```go
// TS ServerOps.ts:109-111 —
//   state.pushInt(check(state.popInt(), SeqTypeValid).duration)
func handleSeqLength(s *ScriptState) error {
    id := s.PopInt()
    st, err := checkSeqType(s, id, "SEQ_LENGTH")
    if err != nil {
        return err
    }
    s.PushInt(st.Duration)
    return nil
}
```

**6. `handleInvStockBase` → OpInvStockBase** (`handlers_inv.go`)

```go
// TS InvOps.ts:41-54 — validates inv + obj, returns -1 if no stockobj/stockcount,
// else stockcount[stockobj.indexOf(objId)] (or -1 if obj not in list).
func handleInvStockBase(s *ScriptState) error {
    inv := s.PopInt()
    obj := s.PopInt()
    if err := checkInvType(s, inv, "INV_STOCKBASE"); err != nil {
        return err
    }
    if err := checkObjType(s, obj, "INV_STOCKBASE"); err != nil {
        return err
    }
    invType := s.Configs.InvType(inv)
    objType := s.Configs.ObjType(obj)
    if len(invType.StockObj) == 0 || len(invType.StockCount) == 0 {
        s.PushInt(-1)
        return nil
    }
    idx := -1
    for i, id := range invType.StockObj {
        if int(id) == objType.ID {
            idx = i
            break
        }
    }
    if idx < 0 {
        s.PushInt(-1)
        return nil
    }
    s.PushInt(int(invType.StockCount[idx]))
    return nil
}
```

> **Plan-author task:** verify `checkObjType` exists (likely yes, used by INV_ADD); if not, mirror `checkInvType`. Verify `objtype.ObjType.ID` field name (rg `type ObjType struct`).

**7. `handleInvDebugName` → OpInvDebugName** (`handlers_inv.go`)

```go
// TS InvOps.ts:34-38 —
//   const invType = check(state.popInt(), InvTypeValid)
//   state.pushString(invType.debugname ?? 'null')
func handleInvDebugName(s *ScriptState) error {
    inv := s.PopInt()
    if err := checkInvType(s, inv, "INV_DEBUG_NAME"); err != nil {
        return err
    }
    invType := s.Configs.InvType(inv)
    if invType.DebugName == "" {
        s.PushString("null")
    } else {
        s.PushString(invType.DebugName)
    }
    return nil
}
```

**8. `handleSetSkinColour` → OpSetSkinColour** (`handlers_player.go`)

```go
// TS PlayerOps.ts:1121-1124 —
//   const skin = check(state.popInt(), SkinColourValid)  // range 0..7
//   state.activePlayer.colors[4] = skin
func handleSetSkinColour(s *ScriptState) error {
    // (goscape defensive; TS skips active-player check)
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("SETSKINCOLOUR: no active player")
    }
    skin := s.PopInt()
    if skin < 0 || skin > 7 {
        return fmt.Errorf("SETSKINCOLOUR: invalid skin colour %d (range 0..7)", skin)
    }
    s.Self.SetColorPart(4, skin)
    return nil
}
```

### §2.4 Registry wiring

Add 8 entries to the `var handlers = map[Opcode]Handler{…}` block in `pkg/script/handlers.go`. Plan-author groups by domain (player block / inv block / server block) to match existing layout.

## §3 Test strategy

**Per handler:** one positive table-driven test in the matching `handlers_<domain>_test.go` file pinning the TS-faithful behavior. Tests use the existing fixture pattern (`ScriptState{}` + StackCapacity init + Pointers flag + push args + Execute → assert popped value or error; mock `Self`/`Configs` per `scriptstate_test_fixture_idioms.md`).

**TS-asymmetry pins** (per `ts_asymmetry_dual_pin.md`):

- `AFK_EVENT`: pin BOTH (a) NodeDebug=true ⇒ ignores staffMod gate, AND (b) NodeDebug=false + staffMod≥2 ⇒ pushes 0 even when afkEventReady=true. Pin afkEventReady is cleared in BOTH branches.
- `WEIGHT`: pin error path when PtrProtectedActivePlayer=0 (defensive Protected guard).
- `INV_STOCKBASE`: three branches — (a) nil StockObj ⇒ -1, (b) obj not in list ⇒ -1, (c) obj in list ⇒ StockCount[idx]. Per `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings.md` analogue: don't share fixture across branches.
- `HEAL_ENERGY`: pin BOTH clamp directions (overflow ⇒ 10000, underflow ⇒ 0) and pin NumberNotNull error on amount=0.
- `SETSKINCOLOUR`: pin range error on skin=-1 AND skin=8 (off-by-one boundary).

**Defensive-guard pins** (DEVIATION labels): for each goscape-only active-player guard added (AFK_EVENT, HEAL_ENERGY, SETSKINCOLOUR), add a single test pinning the error message — flags the deviation in test output if upstream TS later adds `checkedHandler`.

**Runtime budget:** ~16 tests × ~30 LOC each = ~480 LOC test code. Acceptable for cohort size.

## §4 Risk register

| ID | Risk | Mitigation |
|---|---|---|
| R1 | `Player.members` is set from login but never re-read elsewhere — could be vestigial / always-zero | `vestigial_field_misread.md` — plan-author runs `rg "\.members\b" modules/world/` to confirm it IS written from login (already verified at brainstorm: `pkg/loginpb/login.pb.go:271`, login wire field). |
| R2 | `runweight` field exists (modules/world/player.go:233) but only updated under specific tick branches — could be stale at WEIGHT-read time | Out of scope; WEIGHT just pushes the field. If smoke shows wrong weights, that's an NPC-side spec. Pin tests against the field, not against any computed weight invariant. |
| R3 | `checkObjType` may not exist (only checkInvType confirmed) | Plan-author **MUST verify** with `rg "func checkObjType\b" pkg/script/`; if missing, mirror checkInvType pattern in same commit. Do NOT proceed with INV_STOCKBASE task until confirmed. |
| R4 | `objtype.ObjType` field name for the object id may not be `.ID` (could be `.Id` or unnamed) | Plan-author runs `rg "^	I[Dd]\b" pkg/objtype/objtype.go` before codifying. |
| R5 | `NumberNotNull` semantics — TS treats `-1` as null sentinel, `checkNotNull` in goscape may differ | Verified: `checkNotNull(op, "NC_OP")` at `handlers_config.go:379` is the canonical helper; TS `NumberNotNull` rejects `id === -1`. Plan-author re-reads the helper body to confirm. |
| R6 | `s.Self.SetAfkEventReady(false)` will panic if Self is nil after the defensive guard erroneously passes | The guard `s.Self == nil` returns first — write order matters. Plan-author must place SetAfkEventReady AFTER the guard, before the PushInt? Order doesn't actually matter as long as both come after the guard. |
| R7 | `Self.RunEnergy() + amount` overflow on extreme amount inputs (Go `int`) | Goscape `int` is platform-dependent (≥32-bit, typically 64-bit). amount comes from RuneScript (32-bit values). Sum cannot overflow 64-bit int. Range-clamp post-add is the TS shape; preserve verbatim. |

## §5 Out of scope (forward-routed)

- **NAI-150 candidate:** PROJANIM_NPC + PROJANIM_MAP + PROJANIM_PL cluster (3 ops sharing `Zone.MapProjAnim` infra; brainstorm 2026-05-10).
- **NAI-151+ candidates:** remaining 36 unhandled opcodes — `OBJ_FIND*` cluster (8 ops, iterator-state pattern per `iterator_state_pattern.md`), `NPC_*` cluster (5 ops), `VARBIT` push/pop (2 ops, foundational), etc. Not enumerated here; smoke-driven prioritization at next brainstorm.
- **`MAP_INDOORS`:** needs `isIndoors(x,z,level)` map-flag helper (does not exist).
- **`LAST_LOGIN_INFO`:** needs `Player.lastLoginInfo()` packet-emitting wrapper.
- **`SET_GENDER`:** needs `MALE_FEMALE_MAP` / `FEMALE_MALE_MAP` body-mapping tables.

## §6 Closing artifact

Close commit will use `Closes memory:` trailer per `close_commit_memory_trailer.md` if any memory entries are produced (e.g., a "trivial-cohort sweep template" entry covering the field-read + new-method + check-helper recipe — to be decided at close, not now).
