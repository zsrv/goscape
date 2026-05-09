# NAI-133 — BOTH_MOVEINV (4301) + per-pointer-slot Protect refactor + FINDUID/P_FINDUID slot routing

**Predecessor:** NAI-132 close (`744a628`) §2 deferred set. NAI-132 punted `BOTH_MOVEINV` because TS InvOps.ts:373-495 indexes `ProtectedActivePlayer[secondary?1:0]` / `[secondary?0:1]`, and goscape's `s.Protect bool` was a single slot-0 flag. NAI-133 ports the prerequisite per-pointer-slot Protect tracking, the FINDUID/P_FINDUID slot routing that produces the slot-1 protect flag, and the BOTH_MOVEINV handler itself.

**Tech stack:** Go 1.26+. No new dependencies.

## §1 — Scope

NAI-133 ships:

- **T1** — Reify Protect as Pointer-bitmask flags (`PtrProtectedActivePlayer = 1 << 9`, `PtrProtectedActivePlayer2 = 1 << 10`). Retire `s.Protect bool`. Migrate ~30 production reads/writes + ~7 test fixture sites (`s.Protect = true|false` → `s.Pointers |= PtrProtectedActivePlayer` / drop the line for false).
- **T2** — `handleFindUID` + `handlePFindUID` slot routing on `s.Script.IntOperands[s.PC]`. Both opcodes: `intOperand == 0` → bind Self + `PtrActivePlayer` (and `PtrProtectedActivePlayer` for P_FINDUID success); `intOperand == 1` → bind Self2 + `PtrActivePlayer2` (and `PtrProtectedActivePlayer2` for P_FINDUID); other values → error. Closes a latent `.p_finduid` / `.finduid` clobber bug (current code always writes Self regardless of intOperand).
- **T3** — `handleBothMoveInv` (4301), TS InvOps.ts:373-495. Drains `fromInv` of `fromPlayer` into `toInv` of `toPlayer`, with overflow drops to `toPlayer`'s tile. Skip wealth-event tail (NAI-115-D1 reuse).
- **T4** — close commit, memory updates, NAI-132 follow-up retirement.

Estimated ~180 LOC including tests. Linear deps T1 → T2 → T3.

## §2 — Out of scope

- `INV_DROPITEM_DELAYED` (4310) — still NAI-134+ candidate (objDelayedQueue infra prerequisite). Unchanged from NAI-132 §2.
- WealthEvent emission subsystem — D1 reuse. Single-point retire when wired.
- Engine-side ProtectedActivePlayer2 setter — TS never sets it from the engine (only via `.p_finduid`). No goscape parallel needed.

## §3 — TS source (verbatim cites)

- BOTH_MOVEINV: `Engine-TS/src/engine/script/handlers/InvOps.ts:373-495`
- FINDUID: `Engine-TS/src/engine/script/handlers/PlayerOps.ts:60-72`
- P_FINDUID: `Engine-TS/src/engine/script/handlers/PlayerOps.ts:75-94`
- ScriptPointer enum (ProtectedActivePlayer/2 layout): `Engine-TS/src/engine/script/ScriptPointer.ts:7-39`
- ScriptState pointer methods (`pointerAdd`, `pointerGet`): `Engine-TS/src/engine/script/ScriptState.ts:148-194`
- `Player.runScript` (engine-side ProtectedActivePlayer set): `Engine-TS/src/engine/entity/Player.ts:2094-2123`
- `World.processLogout` (engine-side ProtectedActivePlayer set): `Engine-TS/src/engine/World.ts:786-816`
- Wealth-event tail (skipped per D1-reuse): `Engine-TS/src/engine/script/handlers/InvOps.ts:445-494`

## §4 — Per-task design

### Task 1 — Pointer-flag refactor (`Protect bool` → `PtrProtectedActivePlayer{,2}`)

**Pointer.go additions (`pkg/script/pointer.go`):**

```go
const (
    PtrActivePlayer            Pointer = 1 << 0
    PtrActivePlayer2           Pointer = 1 << 1
    PtrActiveNpc               Pointer = 1 << 2
    PtrActiveNpc2              Pointer = 1 << 3
    PtrActiveLoc               Pointer = 1 << 4
    PtrActiveLoc2              Pointer = 1 << 5
    PtrActiveObj               Pointer = 1 << 6
    PtrActiveObj2              Pointer = 1 << 7
    PtrFindDb                  Pointer = 1 << 8
    PtrProtectedActivePlayer   Pointer = 1 << 9  // NEW — slot 0 protect (TS ProtectedActivePlayer)
    PtrProtectedActivePlayer2  Pointer = 1 << 10 // NEW — slot 1 protect (TS ProtectedActivePlayer2)
)
```

**State.go (`pkg/script/state.go:315`):** Delete the `Protect bool` field. Doc-comment on the new flags explains they replace the bool.

**Init (`pkg/script/runner.go:12-38`):**

```go
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState {
    s := &ScriptState{
        // ... unchanged fields ...
        Self: self,
    }
    copy(s.IntLocals, intArgs)
    copy(s.StringLocals, stringArgs)
    if self != nil {
        s.Pointers |= PtrActivePlayer
        if protect {
            s.Pointers |= PtrProtectedActivePlayer
        }
    }
    return s
}
```

Note: `protect=true` with `self=nil` is now silently ignored (matches TS: `runScript` requires `_activePlayer` to set the flag). Existing call sites all pass non-nil players.

**Migration sites (production):**

| File | Line(s) | Current | New |
|---|---|---|---|
| `pkg/script/handlers_player.go` | 62 | `if !s.Protect` | `if s.Pointers&PtrProtectedActivePlayer == 0` |
| `pkg/script/handlers_player.go` | 915 | `s.Protect && s.Self != nil && s.Self.UID() == uid` | (T2 will rewrite for slot routing) |
| `pkg/script/handlers_player.go` | 930 | `s.Protect = true` | (T2 will rewrite for slot routing) |
| `pkg/script/handlers_inv.go` | 341, 431, 460, 502, 533, 583, 587, 639, 642, 896, 900, 958, 1026, 1030, 1106, 1110, 1180 | `&& !s.Protect` | `&& s.Pointers&PtrProtectedActivePlayer == 0` |
| `pkg/script/handlers_vars.go` | 69 | `if protect && !s.Protect` | `if protect && s.Pointers&PtrProtectedActivePlayer == 0` |
| `modules/world/player_script.go` | 303 | `p.activeScript.Protect` | `p.activeScript.Pointers&script.PtrProtectedActivePlayer != 0` |
| `modules/world/player_script.go` | 716 | `p.activeScript.Protect = false` | `p.activeScript.Pointers &^= script.PtrProtectedActivePlayer` |

(Doc-comment-only references at handlers_inv.go:760, player_script.go:277, player_script.go:300 are updated too.)

**New helper — `requireProtectedActivePlayer2` (`handlers_player.go`, after :66):**

```go
// requireProtectedActivePlayer2 is the slot-1 analogue of requireProtectedActivePlayer.
// Chains through requireActivePlayer2 first so error messages match the unprotected
// variant. NAI-133.
func requireProtectedActivePlayer2(s *ScriptState, op string) error {
    if err := requireActivePlayer2(s, op); err != nil {
        return err
    }
    if s.Pointers&PtrProtectedActivePlayer2 == 0 {
        return errors.New(op + ": script not protected")
    }
    return nil
}
```

`requireProtectedActivePlayer` (slot-0) gets the same migration:

```go
func requireProtectedActivePlayer(s *ScriptState, op string) error {
    if err := requireActivePlayer(s, op); err != nil {
        return err
    }
    if s.Pointers&PtrProtectedActivePlayer == 0 {
        return errors.New(op + ": script not protected")
    }
    return nil
}
```

**Test fixture migration (mechanical):**

- `pkg/script/handlers_inv_test.go` lines 853, 901, 998: `s.Protect = true` → `s.Pointers |= PtrProtectedActivePlayer`
- `pkg/script/handlers_inv_test.go` lines 930, 960: `s.Protect = false` → drop the line (zero-value default)
- Doc comments at `handlers_inv_test.go:184, 950` update to reference `PtrProtectedActivePlayer`.
- `handlers_inv_test.go:474, 482, 485, 495, 498, 1238, 1247, 1292, 1336, 1356, 1376, 1419, 1431, 1444, ...` are `mc.invs[...].Protect = true` — those are `*objtype.InvType.Protect`, NOT `ScriptState.Protect`. Untouched.
- Pre-flight grep at T1 dispatch must list every `\bs\.Protect\b` and `\.Protect\s*=` reference and distinguish ScriptState.Protect from objtype.InvType.Protect / VarpType.Protect / etc. by file + context. Apply `enumerate_all_sites.md`.

**T1 ships with no behavior change.** All existing tests pass after migration. T1 is GREEN-only — verifies the refactor didn't break anything.

### Task 2 — FINDUID + P_FINDUID slot routing

**handleFindUID (`handlers_player.go:885-900`)** ports TS PlayerOps.ts:60-72:

```go
func handleFindUID(s *ScriptState) error {
    operand := s.Script.IntOperands[s.PC]
    if operand != 0 && operand != 1 {
        return fmt.Errorf("FINDUID: invalid intOperand %d", operand)
    }
    uid := s.PopInt()
    if s.PlayerLookup == nil {
        s.PushInt(0)
        return nil
    }
    target := s.PlayerLookup.LookupPlayerByUID(uid)
    if target == nil {
        s.PushInt(0)
        return nil
    }
    if operand == 0 {
        s.Self = target
        s.Pointers |= PtrActivePlayer
    } else {
        s.Self2 = target
        s.Pointers |= PtrActivePlayer2
    }
    s.PushInt(1)
    return nil
}
```

**handlePFindUID (`handlers_player.go:912-933`)** ports TS PlayerOps.ts:75-94:

```go
func handlePFindUID(s *ScriptState) error {
    operand := s.Script.IntOperands[s.PC]
    if operand != 0 && operand != 1 {
        return fmt.Errorf("P_FINDUID: invalid intOperand %d", operand)
    }
    uid := s.PopInt()

    // Self-reacquire fast-path: already protected on this slot's player.
    if operand == 0 {
        if s.Pointers&PtrProtectedActivePlayer != 0 && s.Self != nil && s.Self.UID() == uid {
            s.PushInt(1)
            return nil
        }
    } else {
        if s.Pointers&PtrProtectedActivePlayer2 != 0 && s.Self2 != nil && s.Self2.UID() == uid {
            s.PushInt(1)
            return nil
        }
    }

    if s.PlayerLookup == nil {
        s.PushInt(0)
        return nil
    }
    target := s.PlayerLookup.LookupPlayerByUID(uid)
    if target == nil || !target.CanAccess() {
        s.PushInt(0)
        return nil
    }

    if operand == 0 {
        s.Self = target
        s.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
    } else {
        s.Self2 = target
        s.Pointers |= PtrActivePlayer2 | PtrProtectedActivePlayer2
    }
    s.PushInt(1)
    return nil
}
```

**T2 tests (new):**

- `TestFindUID_Slot0_BindsSelf` — operand=0, target found → Self bound, PtrActivePlayer set, push 1.
- `TestFindUID_Slot1_BindsSelf2` — operand=1, target found → Self2 bound, PtrActivePlayer2 set, push 1, Self UNTOUCHED.
- `TestFindUID_Slot1_LookupMiss_PushesZero` — operand=1, target nil → push 0, no state change.
- `TestFindUID_InvalidOperand_Errors` — operand=2 → error; operand=-1 → error.
- `TestPFindUID_Slot0_Success` — operand=0, target found+CanAccess → Self bound, both Ptr flags set, push 1.
- `TestPFindUID_Slot1_Success` — operand=1 analogue → Self2 bound, both Ptr*2 flags set.
- `TestPFindUID_Slot0_SelfReacquire` — Self already bound + PtrProtectedActivePlayer set, operand=0, popped UID == Self.UID() → push 1, no state change.
- `TestPFindUID_Slot1_SelfReacquire` — Self2 already bound + PtrProtectedActivePlayer2 set, operand=1, popped UID == Self2.UID() → push 1, no state change.
- `TestPFindUID_Slot0_NoFastPathWhenSlot1Protected` — sanity pin: only the matching slot's protect flag triggers the fast-path.
- `TestPFindUID_Slot1_LookupMiss` / `TestPFindUID_Slot1_CanAccessFalse` — both push 0.

**Test fixture pattern** (per `scriptstate_test_fixture_idioms.md`):

```go
s := &ScriptState{
    Script: &ScriptFile{
        Opcodes:     []Opcode{OpFindUID},
        IntOperands: []int32{0}, // or 1 for slot-1 tests
    },
    PC:           0,
    PlayerLookup: ...,
    IntStack:     make([]int, StackCapacity),
    StringStack:  make([]string, StackCapacity),
}
s.PushInt(targetUID)
```

### Task 3 — handleBothMoveInv

**Source (TS InvOps.ts:373-495 transcribed for goscape):**

```go
// handleBothMoveInv ports TS InvOps.ts:373-495 (BOTH_MOVEINV, opcode 4301).
//
// Dispatch shape: `state.intOperand` selects primary (0) vs secondary (1).
// Primary: from = active_player (Self), to = .active_player (Self2).
// Secondary (`.both_moveinv`): pointers swap — from = Self2, to = Self.
//
// TS-faithful protect gates: the slot corresponding to fromPlayer must be
// Protected if fromInvType.Protect && fromInvType.Scope != Shared. The slot
// corresponding to toPlayer must be Protected if toInvType.Protect &&
// fromInvType.Scope != Shared (TS quirk preserved: to-gate gates on FROM
// scope; see InvOps.ts:397).
//
// DEVIATION-NAI-115-D1 (reuse): TS InvOps.ts:445-494 emits addWealthEvent
// for dueloffer/STAKE and trade/TRADE. Goscape skips inline emission;
// content can emit via OpWealthEvent (2131). Single-point retire when
// WealthEvent subsystem lands. NAI-115-D1.
func handleBothMoveInv(s *ScriptState) error {
    // checkedHandler(ActivePlayer) gate. TS uses ActivePlayer[intOperand];
    // the `secondary` resolution below also requires Self2/PtrActivePlayer2
    // when intOperand==1.
    operand := s.Script.IntOperands[s.PC]
    if operand != 0 && operand != 1 {
        return fmt.Errorf("BOTH_MOVEINV: invalid intOperand %d", operand)
    }
    secondary := operand == 1

    if secondary {
        if err := requireActivePlayer2(s, "BOTH_MOVEINV"); err != nil {
            return err
        }
    } else {
        if err := requireActivePlayer(s, "BOTH_MOVEINV"); err != nil {
            return err
        }
    }

    to := s.PopInt()
    from := s.PopInt()

    if err := checkInvType(s, from, "BOTH_MOVEINV"); err != nil {
        return err
    }
    if err := checkInvType(s, to, "BOTH_MOVEINV"); err != nil {
        return err
    }

    fromInvType := s.Configs.InvType(from)
    toInvType := s.Configs.InvType(to)

    // Resolve fromPlayer / toPlayer per `secondary`. Both MUST be bound;
    // when secondary, the *non-active* slot (Self when secondary, Self2
    // when primary) must also be bound — TS asserts `if (!fromPlayer || !toPlayer)`.
    var fromPlayer, toPlayer ActivePlayer
    var fromProtectedFlag, toProtectedFlag Pointer
    if secondary {
        fromPlayer = s.Self2
        toPlayer = s.Self
        fromProtectedFlag = PtrProtectedActivePlayer2
        toProtectedFlag = PtrProtectedActivePlayer
        if toPlayer == nil || s.Pointers&PtrActivePlayer == 0 {
            return errors.New("BOTH_MOVEINV: no active player")
        }
    } else {
        fromPlayer = s.Self
        toPlayer = s.Self2
        fromProtectedFlag = PtrProtectedActivePlayer
        toProtectedFlag = PtrProtectedActivePlayer2
        if toPlayer == nil || s.Pointers&PtrActivePlayer2 == 0 {
            return errors.New("BOTH_MOVEINV: no active player2")
        }
    }

    // From-protect gate.
    if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared &&
        s.Pointers&fromProtectedFlag == 0 {
        return fmt.Errorf("BOTH_MOVEINV: $from_inv requires protected access: %s", fromInvType.DebugName)
    }
    // TS quirk preserved: to-gate gates on FROM scope (InvOps.ts:397).
    if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared &&
        s.Pointers&toProtectedFlag == 0 {
        return fmt.Errorf("BOTH_MOVEINV: $to_inv requires protected access: %s", toInvType.DebugName)
    }

    if s.Inv == nil {
        return errors.New("BOTH_MOVEINV: no inv lookup")
    }
    fromInv := s.Inv.Get(fromPlayer, from)
    toInv := s.Inv.Get(toPlayer, to)
    if fromInv == nil || toInv == nil {
        return errors.New("BOTH_MOVEINV: inv is null")
    }

    // Drain loop. TS InvOps.ts:413-443.
    capacity := fromInv.Capacity()
    for slot := range capacity {
        it := fromInv.Get(slot)
        if it == nil {
            continue
        }
        objID := it.Id
        count := it.Count

        objType := s.Configs.ObjType(objID)
        if objType == nil {
            return fmt.Errorf("BOTH_MOVEINV: invalid obj id at slot (id=%d)", objID)
        }

        fromInv.Delete(slot)

        stackable, stockObj := lookupStackableStockObj(s, toInv.Type, objID)
        tx := toInv.Add(objID, count, inventory.AddOpts{
            BeginSlot:           -1,
            AssureFullInsertion: false,
            Stackable:           stackable,
            StockObj:            stockObj,
        })
        overflow := count - tx.Completed
        if overflow > 0 && s.World != nil {
            level := (toPlayer.CoordPacked() >> 28) & 0x3
            x := toPlayer.X()
            z := toPlayer.Z()
            receiverID := toPlayer.UID()
            if !objType.Stackable || overflow == 1 {
                for range overflow {
                    s.World.AddObj(level, x, z, objID, 1, 200, receiverID)
                }
            } else {
                s.World.AddObj(level, x, z, objID, overflow, 200, receiverID)
            }
        }
    }

    // NAI-115-D1 (reuse): TS InvOps.ts:445-494 emits addWealthEvent for
    // dueloffer/STAKE and trade/TRADE. Skipped — content emits via
    // OpWealthEvent (2131).

    return nil
}
```

**Dispatch wiring** (`pkg/script/handlers.go:307` neighborhood — the registration table is `OpcodeName: handlerFn` map literal entries). Add `OpBothMoveInv: handleBothMoveInv,` near the existing `OpInvAdd:`/`OpInvDropSlot:` entries. `String()` case at `opcode.go:1089` is already in place.

**T3 tests (`pkg/script/handlers_inv_test.go`):**

| Name | Pin |
|---|---|
| `TestBothMoveInv_Primary_DrainsFromSelfToSelf2` | operand=0; populate Self's main inv with [{id=A,n=3},{id=B,n=1}], Self2's main empty → both items move to Self2's main; Self's main empty post. |
| `TestBothMoveInv_Secondary_DrainsFromSelf2ToSelf` | operand=1; pointers flip; Self2's main → Self's main. Self.invs[main] populated; Self2.invs[main] empty. |
| `TestBothMoveInv_Overflow_StackableDropsSingleStack` | toInv full of stackable item, fromInv has stackable count=N+M → toInv absorbs N, World.AddObj called once with count=M at toPlayer's tile. |
| `TestBothMoveInv_Overflow_NonStackableDropsPerUnit` | non-stackable, overflow=K → World.AddObj called K times, count=1 each. |
| `TestBothMoveInv_FromProtectGate_FiresWhenSlotUnprotected` | fromInvType.Protect=true, Scope=TEMP, fromPlayer slot Pointers MISSING ProtectedActivePlayer → error "$from_inv requires protected access: ...". |
| `TestBothMoveInv_ToProtectGate_UsesFromInvScope` | TS quirk pin: toInv.Protect=true, toInv.Scope=Shared, fromInv.Scope=TEMP, toPlayer slot UNPROTECTED → gate FIRES (because gate reads fromInv.Scope). Inverse pin: same toInv but fromInv.Scope=Shared → gate does NOT fire. |
| `TestBothMoveInv_NoSelf2_Errors` | operand=0 with PtrActivePlayer2 unset / Self2 nil → error "no active player2". |
| `TestBothMoveInv_NoSelf_Secondary_Errors` | operand=1 with PtrActivePlayer unset → error "no active player". |
| `TestBothMoveInv_FromInvNil_Errors` / `TestBothMoveInv_ToInvNil_Errors` | InvLookup returns nil → "inv is null". |
| `TestBothMoveInv_InvalidOperand_Errors` | operand=2 → error. |
| `TestBothMoveInv_WealthEventSkip_NoEmission` | D1 absence pin: fromInvType.DebugName="dueloffer" with non-empty drain → no `OpWealthEvent` recorder fires. Even with secondary=false (TRADE case), no recorder fires. (Per `ts_asymmetry_dual_pin.md`: pin the absence so a future WealthEvent wiring escalates this test.) |

### Task 4 — Close

- Add `Closes memory:` trailer per `close_commit_memory_trailer.md` listing memory entries created by NAI-133 (T2 latent-bug correction; T3 TS-quirk preservation).
- Update `nai_followups.md` to mark BOTH_MOVEINV closed; refresh NAI-115-D1 entry to list BOTH_MOVEINV among reusing sites (alongside INV_DROPSLOT and INV_DROPITEM).
- Update NAI-132 spec/plan §2 cross-reference to point at NAI-133 close commit.

## §5 — Deviations & risks

### Deviations

- **D1-reuse (NAI-115-D1)**: Wealth-event tail (TS InvOps.ts:445-494) skipped. Doc-comment in `handleBothMoveInv` cites NAI-115-D1 with this site as a new reuse point. Test `TestBothMoveInv_WealthEventSkip_NoEmission` is the absence-pin per `ts_asymmetry_dual_pin.md`.
- **TS quirk preservation (no D-tag)**: BOTH_MOVEINV's to-side protect gate uses `fromInvType.Scope` (not `toInvType.Scope`). Preserved verbatim per `true_to_ts_gate.md`. Inline comment marks as "TS quirk preserved (InvOps.ts:397)". Test `TestBothMoveInv_ToProtectGate_UsesFromInvScope` is the dual-pin (FIRES + does-NOT-fire variants).
- **Engine-side ProtectedActivePlayer2 set: unreachable**: TS sets `ProtectedActivePlayer` (slot 0) in two engine-side paths (`Player.runScript`, `World.processLogout`); never sets `ProtectedActivePlayer2` from the engine. Goscape mirrors: `runner.Init`'s `protect bool` arg sets slot 0 only. No deviation; doc-comment on `PtrProtectedActivePlayer2` notes this.
- **`Init(... protect=true, self=nil)` silently drops the flag** (NEW behavior — pre-NAI-133, `s.Protect` was set unconditionally). All existing call sites pass non-nil players; this is observable only in tests. Audit: pre-flight grep for `script.Init(...nil...true...)` → expect zero hits.

### Risks

- **Test fixture migration scope (~7 sites in handlers_inv_test.go)**: Mechanical but volume-y. Pre-flight grep at T1 dispatch lists every `\bs\.Protect\b` reference; implementer treats as enumerate-and-apply per `enumerate_all_sites.md`. Confused-grep risk: `handlers_inv_test.go` also contains `mc.invs[...].Protect` / `objtype.InvType.Protect` references — context-distinguish, do NOT migrate those.
- **Module-tree cross-cut (`modules/world/player_script.go`)**: Three sites (lines 277, 300, 303, 716 — comments + read + write). Verified at controller pre-flight (`controller_preflight.md`). After T1, run `rg '\.Protect\b' modules/ pkg/` and confirm no `ScriptState.Protect` reference remains.
- **handlePFindUID slot-1 self-reacquire latent**: Easy to forget — current code's fast-path always reads Self. After T2, slot-1 fast-path needs Self2/UID/PtrProtectedActivePlayer2. Add explicit unit test `TestPFindUID_Slot1_SelfReacquire` per `latent_bug_at_migration_boundary.md`.
- **Plan-codified `intOperand` test fixtures must include `IntOperands`**: `&ScriptState{}` without `Script.IntOperands` panics on `s.Script.IntOperands[s.PC]`. Per `scriptstate_test_fixture_idioms.md`, all new T2/T3 tests must initialize `Script: &ScriptFile{Opcodes: []Opcode{...}, IntOperands: []int32{...}}` plus `PC: 0`.
- **AddObj signature audit at plan-write**: `s.World.AddObj(level, x, z, objID, count, duration, receiverID)` — verified at NAI-115/NAI-132 close. Re-verify at T3 dispatch per `controller_preflight.md`.
- **`fromInv.Capacity()` / `Inventory.Get(slot)` / `Delete(slot)` API surface**: Used by INV_DROPSLOT and INV_DROPALL today. Re-grep at T3 dispatch to confirm method names unchanged.

### Verification

- T1: `go test ./pkg/script/... ./modules/world/...` green with no behavior change.
- T2: T1 still green + new FINDUID/P_FINDUID slot-routing tests green.
- T3: T1+T2 still green + new BOTH_MOVEINV tests green.
- T4: full repo `go test -race ./...` (timeout) per project convention.
