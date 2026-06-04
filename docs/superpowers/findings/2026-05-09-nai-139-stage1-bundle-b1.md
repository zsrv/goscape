# NAI-139 Stage 1 Audit — Bundle B1: tutorial_complete cascade opcodes

**Date:** 2026-05-09  
**Source:** `LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2:296-330`  
**Scope:** depth-0 (proc bodies excluded)

## 1. TS source verbatim (lines 296–330)

```runescript
[label,tutorial_complete]
tut_close();
if_close;

%tutorial = ^tutorial_complete;
p_telejump(0_50_50_22_22);

inv_clear(inv);
inv_add(inv, bronze_axe, 1);
inv_add(inv, tinderbox, 1);
inv_add(inv, net, 1);
inv_add(inv, shrimp, 1);
inv_add(inv, bucket_empty, 1);
inv_add(inv, pot_empty, 1);
inv_add(inv, bread, 1);
inv_add(inv, bronze_pickaxe, 1);
inv_add(inv, bronze_dagger, 1);
inv_add(inv, bronze_sword, 1);
inv_add(inv, wooden_shield, 1);
inv_add(inv, shortbow, 1);
inv_add(inv, bronze_arrow, 25);
inv_add(inv, airrune, 25);
inv_add(inv, mindrune, 15);
inv_add(inv, waterrune, 6);
inv_add(inv, earthrune, 4);
inv_add(inv, bodyrune, 2);

inv_clear(worn);
inv_clear(bank);
inv_add(bank, coins, 25);

~stat_reset_all;

~initalltabs;
~update_all(inv_getobj(worn, ^wearpos_rhand));

session_log(^log_adventure, "Completed tutorial island");
```

Token list confirmed: `tut_close` (1), `if_close` (1), `%tutorial = ^tutorial_complete` (1 varp write), `inv_clear` (3 call sites: inv/worn/bank), `inv_add` (19 call sites: 18×inv + 1×bank).

---

## 2. Dispatch findings

| token | kind | ts_ref | goscape_dispatch | status | evidence |
|-------|------|--------|------------------|--------|----------|
| `tut_close` | opcode (no args) | tutorial.rs2:297 | `pkg/script/handlers.go:335` → `pkg/script/handlers_interface.go:111` | WIRED | `OpTutClose: handleTutClose,` (handlers.go:335); `func handleTutClose(s *ScriptState) error { … s.Self.CloseTutorial() … }` (handlers_interface.go:111–117) |
| `if_close` | opcode (no args) | tutorial.rs2:298 | `pkg/script/handlers.go:329` → `pkg/script/handlers_interface.go:15` | WIRED | `OpIfClose: handleIfClose,` (handlers.go:329); `func handleIfClose(s *ScriptState) error { … s.Self.CloseModal(true) … }` (handlers_interface.go:15–21) |
| `%tutorial = ^tutorial_complete` | varp write (OpPopVarp) | tutorial.rs2:300 | `pkg/script/handlers.go:207` → `pkg/script/handlers_vars.go:63` | WIRED | `OpPopVarp: handlePopVarp,` (handlers.go:207); `func handlePopVarp(…) { … s.Self.SetVarp(id, int32(s.PopInt())) }` (handlers_vars.go:63–78); wire path: `(*Player).SetVarp` (player_script.go:321–327) → `(*Player).writeVarp` (player_varp.go:14–43) → `gameserver.OpVarpSmall` / `OpVarpLarge` |
| `inv_clear(inv)` | opcode (1 arg) | tutorial.rs2:303 | `pkg/script/handlers.go:313` → `pkg/script/handlers_inv.go:522` | WIRED | `OpInvClear: handleInvClear,` (handlers.go:313); see §3 below |
| `inv_clear(worn)` | opcode (1 arg) | tutorial.rs2:323 | `pkg/script/handlers.go:313` → `pkg/script/handlers_inv.go:522` | WIRED | same handler; `worn` typeID routes via `invLookupView.Get` (server_invs.go:15–47) |
| `inv_clear(bank)` | opcode (1 arg) | tutorial.rs2:324 | `pkg/script/handlers.go:313` → `pkg/script/handlers_inv.go:522` | WIRED | same handler; `bank` typeID routes via `invLookupView.Get` |
| `inv_add(inv, …, …)` (×18) | opcode (3 args) | tutorial.rs2:304–322 | `pkg/script/handlers.go:309` → `pkg/script/handlers_inv.go:318` | WIRED | `OpInvAdd: handleInvAdd,` (handlers.go:309); see §4 below |
| `inv_add(bank, coins, 25)` | opcode (3 args) | tutorial.rs2:325 | `pkg/script/handlers.go:309` → `pkg/script/handlers_inv.go:318` | WIRED | same handler; `bank` typeID routes via `invLookupView.Get` |

---

## 3. varp write — NAI-138 cross-reference

`%tutorial = ^tutorial_complete` compiles to `PUSH_CONSTANT_INT(^tutorial_complete) + POP_VARP(<tutorial varp id>)`.

`handlePopVarp` (handlers_vars.go:63–78):
- Reads varp id from `Script.IntOperands[s.PC] & 0xffff`.
- Checks protect gate; for `%tutorial`, protect=false (not guarded in TS runescript).
- Calls `s.Self.SetVarp(id, int32(s.PopInt()))`.

`(*Player).SetVarp` (player_script.go:321–327) writes `p.varps[id] = val` then calls `p.writeVarp(id, val)`.

`(*Player).writeVarp` (player_varp.go:14–43) gates on `cfg.Transmit`; if transmit=true, emits `OpVarpSmall` (value in −128..127) or `OpVarpLarge` (otherwise) via `p.writeOut`.

NAI-138 commit ee54c84 fixed varp configs loading both server+client streams. Varp 4 (`tutorial`) and varp 173 (`option_run`) are loaded from the same `VarPlayerType` config path. The `writeVarp` probe test (player_varp_probe_test.go:27,110,133,179) covers this path directly with varp 173; the same `writeVarp` call is used for all varp IDs including tutorial varp 4.

The wire-emit path is: `handlePopVarp` → `SetVarp` → `writeVarp` → `p.writeOut(OpVarpSmall/Large, payload)`. No gaps.

---

## 4. inv_clear / inv_add — inv-type resolution

`handleInvClear` (handlers_inv.go:522–543):
1. Pops `typeID` (int) from the stack — the compiled integer constant for inv/worn/bank.
2. `checkInvType(s, typeID, "INV_CLEAR")` validates via `s.Configs.InvType(typeID)`.
3. Protect/scope gate: `invType.Protect && invType.Scope != InvTypeScopeShared && PtrProtectedActivePlayer == 0`.
4. `resolveInv(s, typeID)` → `s.Inv.Get(s.Self, typeID)` → `invLookupView.Get`.
5. `inv.Clear()`.

`handleInvAdd` (handlers_inv.go:318–~400):
1. Pops `count`, `obj`, `typeID` in TS order.
2. Validates InvTypeValid → ObjTypeValid → ObjStackValid → protect/scope → dummyitem gate.
3. `resolveInv(s, typeID)` → `inv.Add(obj, count, …)`.
4. Overflow drops to world tile.

`invLookupView.Get` (server_invs.go:15–47) routes by `InvType.Scope`:
- `ScopeShared` → world-level shared inv (lazily allocated in `v.s.invs[typeID]`).
- Per-player → `p.invs[typeID]` (lazily allocated on player).

`inv`, `worn`, and `bank` are compiled to integer typeIDs by the runescript compiler; the handler receives the integer and the scope routing is determined entirely by `invTypes.Configs[typeID].Scope`. No symbol-name lookup occurs at runtime — the resolution is by numeric ID, which is correct.

---

## 5. Summary

All five opcode families in the `tutorial_complete` cascade are **WIRED** in goscape:

- `tut_close` → `handleTutClose` → `(*Player).CloseTutorial()`
- `if_close` → `handleIfClose` → `(*Player).CloseModal(true)`
- `%tutorial = ^tutorial_complete` → `handlePopVarp` → `SetVarp` → `writeVarp` → wire packet (VARP_SMALL/LARGE, transmit-gated)
- `inv_clear` (3 sites) → `handleInvClear` → `inv.Clear()`
- `inv_add` (19 sites) → `handleInvAdd` → `inv.Add(…)` with overflow drop

No STUB, MISSING, or UNKNOWN rows. The NAI-138 varp wire path is shared across all varp IDs and is intact for varp 4 (`tutorial`).
