---
status: brainstorm-approved
date: 2026-05-11
ts_source:
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:462-464 (SAY)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:980-982 (HEADICONS_GET)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:984-986 (HEADICONS_SET)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:881-890 (P_EXACTMOVE)
  - LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts:20-24 (INV_ALLSTOCK)
  - LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:521-523 (NPC_ATTACKRANGE)
  - LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:556-558 (NPC_INRANGE)
---

# NAI-160 — Trivial-handler sweep #2 (7 ops; player + inv + npc cohort)

**Cadence:** 100-300 LOC band (7 handlers + ~6 ActivePlayer/ActiveNpc methods + adapters + tests) — separate spec + plan, single combined Sonnet reviewer at end-of-impl. Subagent-driven-development per `execution_mode_default.md`.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Cascade-tail context:** missing-handler audit at HEAD `5109c53` reports **28 unhandled opcodes** (`missing_handler_audit.md` one-liner). This sub-spec ports 7; remaining 21 stay forward-routable to NAI-161+.

---

## §1 Symptom / motivation

`say` is the single most-called unhandled opcode in `LostCityRS/Content/scripts`: **126 call sites** (NPC dialog speech bubbles, content-event chat). Every multi-bubble dialog cascade post-NAI-159's LOC_FIND-driven content fans through `say` and currently fails with `no handler for SAY (opcode 2097)`.

The remaining 6 ops in this bundle are picked from the 28-opcode unhandled set on three filters:
1. **Backing infra exists** at HEAD (field present on `*Player` / `*Npc`, or config field on `InvType`/`NpcType`) — verified by grep at brainstorm time.
2. **Single-statement TS body** — handler is a field read, field write with one `check`, or a 1-line method delegate, matching the NAI-149 trivial-sweep cohort shape.
3. **Active content callers** ≥1 (rg `\bopname\b` against `LostCityRS/Content/scripts` returns non-zero) — keeps the cohort grounded in user-observable surface, not stub work.

**Cohort (7 ops):**

| Op | Content callers | TS body | TS file:line |
|---|---|---|---|
| `OpSay` | 126 | `state.activePlayer.say(state.popString())` | PlayerOps.ts:462-464 |
| `OpHeadIconsGet` | 2 | `state.pushInt(state.activePlayer.headicons)` | PlayerOps.ts:980-982 |
| `OpHeadIconsSet` | 2 | `state.activePlayer.headicons = check(state.popInt(), NumberNotNull)` | PlayerOps.ts:984-986 |
| `OpPExactMove` | 10 | `popInts(5)` + 2× `CoordValid` + `unsetMapFlag()` + `exactMove(...)` | PlayerOps.ts:881-890 |
| `OpInvAllStock` | 2 | `pushInt(check(popInt, InvTypeValid).allstock ? 1 : 0)` | InvOps.ts:20-24 |
| `OpNpcAttackRange` | 1 | `pushInt(check(activeNpc.type, NpcTypeValid).attackrange)` | NpcOps.ts:521-523 |
| `OpNpcInRange` | 1 | `pushInt(activeNpc.targetWithinMaxRange() ? 1 : 0)` | NpcOps.ts:556-558 |

**Excluded from this bundle** (reasons; routed forward):

| Op | Defer reason |
|---|---|
| `OpClearQueue` / `OpGetQueue` | Need new ActivePlayer surface for queue-list inspection (`p.queue` walk by scriptID + `weakQueue` equivalent). Goscape unifies QueueWeak via `Type` discriminator (`player_script.go:96-102`), but no listing/unlink methods are exposed yet. Discrete cohort; defer to NAI-161+. |
| `OpNpcStatHeal` | Body is non-trivial arithmetic (`(constant + (base * percent) / 100) | 0`) plus needs `HeroPoints.Clear()` (modules/world/heropoints.go has `AddHero`/`TopContributor` only). 2 new surfaces + non-1-line body. Defer. |
| `OpPLocMerge` | Calls `World.mergeLoc(activeLoc, activePlayer, …)` — needs ActivePlayer `unsetMapFlag` (would be in-scope here) PLUS `MergeLoc` on ActivePlayer and active-loc binding semantics. TS marks it `TODO: check active loc too`. Forward-route. |
| `OpLcOp` / `OpOcOp` / `OpOcIop` | Each fires a server-trigger against a loc/obj op — needs interaction-trigger plumbing audit. Defer. |
| `OpPOpHeld` | TS body is literally `throw new Error('unimplemented')` (PlayerOps.ts:381-383). Could port as same-shape error stub for completeness, but no content caller and no behavioral effect; out-of-cohort. |
| `OpPushVarbit` / `OpPopVarbit` | 0 content callers. Varbit infra in goscape needs separate audit (varbit register file + bit-pack ops on packed varps). Forward-route. |
| `OpLineOfSight` | 0 content callers; needs LoS clip-flag traversal infra. Forward-route. |
| `OpNpcAdd` / `OpNpcHunt` | Spawn / aggro mechanics; not 1-line ports. |
| `OpMapIndoors` / `OpLastLoginInfo` / `OpSetGender` / `OpWealthEvent` | Excluded already at NAI-149 with documented reasons (helper-missing / method-missing / mapping-tables-missing / RPC-payload-missing). Re-confirmed at HEAD: nothing has changed those positions. |
| `OpBothDropSlot` / `OpInvDropAll` / `OpInvTotalParamStack` | Each needs new Obj/Inv timed-drop surface or new ActivePlayer method. Distinct discrete-port cohort. Forward-route. |
| `OpNpcAttackRange` / `OpNpcInRange` | **Included** despite low (1 each) caller counts — these complete NAI-120's NPC-introspection cohort (`NPC_HASOP`, `NPC_TYPE`, `NPC_NAME` already shipped) and add zero new infra (single new ActiveNpc accessor). |
| `OpPOpPlayerT` | TS `popInt → check NotNull → stopAction + setInteraction(SCRIPT, activePlayer2, APPLAYERT, spellId)` — needs ActivePlayer 4-arg `SetInteractionScriptPlayer` overload with spell payload. Defer; tracked at NAI-162. |
| `OpSay` (NPC variant) | Already handled at `handleNpcSay` (`handlers_npc.go:231`). |

---

## §2 Architecture

### §2.1 New `pkg/script.ActivePlayer` methods

Mirroring the existing accessor convention (each is field-read / method-delegate pass-through to `modules/world.Player` — no logic):

| Method | Type | Backing | Setter? | Notes |
|---|---|---|---|---|
| `Say(text []byte)` | — | `(*Player).Say([]byte)` `player_masks.go:8-11` | — | Method already exists; just expose. |
| `HeadIcons() int` | int | `Player.headicons int` `player.go:209` | yes — `SetHeadIcons(v int)` | TS does direct field read/write. |
| `ExactMove(sX, sZ, eX, eZ, begin, finish, dir int)` | — | `(*Player).ExactMove` `player_masks.go:28-37` | — | Method already exists. |
| `UnsetMapFlag()` | — | wraps `sendUnsetMapFlag(p)` `handler_oploc.go` | — | TS `unsetMapFlag` writes a packet to clear the map-click destination flag. Goscape already has the packet helper; needs an `*Player` method that calls it. |

### §2.2 New `pkg/script.ActiveNpc` methods

| Method | Type | Backing | Notes |
|---|---|---|---|
| `TargetWithinMaxRange() bool` | bool | `(*Npc).targetWithinMaxRange()` `npc_interaction.go:591` | Lowercase method already exists; expose via exported wrapper. |

### §2.3 New `*Player` adapter methods (modules/world)

Live in `modules/world/player_script.go` or `player_masks.go` (whichever matches existing convention for the named field — plan-author resolves at write time):

```go
// HeadIcons exposes p.headicons for ActivePlayer.HeadIcons().
// Mirrors TS direct read at PlayerOps.ts:981 (HEADICONS_GET).
func (p *Player) HeadIcons() int { return p.headicons }

// SetHeadIcons writes the validated head-icon bitmask. Caller is
// responsible for NumberNotNull gating (handler does that). Mirrors TS
// direct write at PlayerOps.ts:985 (HEADICONS_SET).
func (p *Player) SetHeadIcons(v int) { p.headicons = v }

// UnsetMapFlag clears the player's map-click destination by sending the
// matching client packet. Mirrors TS Player.unsetMapFlag (used by
// P_EXACTMOVE at PlayerOps.ts:888 and adjacent server-script paths).
func (p *Player) UnsetMapFlag() { sendUnsetMapFlag(p) }
```

### §2.4 New `*Npc` adapter method (modules/world)

```go
// TargetWithinMaxRange exposes the unexported targetWithinMaxRange for
// ActiveNpc.TargetWithinMaxRange() (NPC_INRANGE at NpcOps.ts:557).
func (n *Npc) TargetWithinMaxRange() bool { return n.targetWithinMaxRange() }
```

### §2.5 Handler bodies (per opcode)

All handlers register in `pkg/script/handlers.go` map and live in the existing per-domain file (`handlers_player.go`, `handlers_inv.go`, `handlers_npc.go`).

**1. `handleSay` → OpSay** (`handlers_player.go`)

```go
// TS PlayerOps.ts:462-464 — checkedHandler(ActivePlayer, …)
//   state.activePlayer.say(state.popString())
func handleSay(s *ScriptState) error {
    if err := requireActivePlayer(s, "SAY"); err != nil {
        return err
    }
    text := s.PopString()
    s.Self.Say([]byte(text))
    return nil
}
```

**2. `handleHeadIconsGet` → OpHeadIconsGet** (`handlers_player.go`)

```go
// TS PlayerOps.ts:980-982 — state.pushInt(state.activePlayer.headicons)
func handleHeadIconsGet(s *ScriptState) error {
    if err := requireActivePlayer(s, "HEADICONS_GET"); err != nil {
        return err
    }
    s.PushInt(s.Self.HeadIcons())
    return nil
}
```

**3. `handleHeadIconsSet` → OpHeadIconsSet** (`handlers_player.go`)

```go
// TS PlayerOps.ts:984-986 — activePlayer.headicons = check(popInt, NumberNotNull)
func handleHeadIconsSet(s *ScriptState) error {
    if err := requireActivePlayer(s, "HEADICONS_SET"); err != nil {
        return err
    }
    v := s.PopInt()
    if err := checkNotNull(v, "HEADICONS_SET"); err != nil {
        return err
    }
    s.Self.SetHeadIcons(v)
    return nil
}
```

**4. `handlePExactMove` → OpPExactMove** (`handlers_player.go`)

```go
// TS PlayerOps.ts:881-890 — checkedHandler(ProtectedActivePlayer, …)
//   const [start, end, startCycle, endCycle, direction] = state.popInts(5);
//   const startPos = check(start, CoordValid);
//   const endPos   = check(end,   CoordValid);
//   state.activePlayer.unsetMapFlag();
//   state.activePlayer.exactMove(startPos.x, startPos.z, endPos.x, endPos.z, startCycle, endCycle, direction);
//
// Pop order: TS popInts(5) returns [start, end, startCycle, endCycle, direction]
// from the bottom of the popped slice (top-of-stack is direction). Goscape
// PopInt() returns top-of-stack, so pop in reverse: direction, endCycle,
// startCycle, end, start. (Pinned by recorded-args test per
// `handler_pop_order_test_masking.md`.)
func handlePExactMove(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "P_EXACTMOVE"); err != nil {
        return err
    }
    direction := s.PopInt()
    endCycle := s.PopInt()
    startCycle := s.PopInt()
    endPacked := s.PopInt()
    startPacked := s.PopInt()
    _, sX, sZ, err := checkCoord(startPacked, "P_EXACTMOVE")
    if err != nil {
        return err
    }
    _, eX, eZ, err := checkCoord(endPacked, "P_EXACTMOVE")
    if err != nil {
        return err
    }
    s.Self.UnsetMapFlag()
    s.Self.ExactMove(sX, sZ, eX, eZ, startCycle, endCycle, direction)
    return nil
}
```

**5. `handleInvAllStock` → OpInvAllStock** (`handlers_inv.go`)

```go
// TS InvOps.ts:20-24 — pushInt(check(popInt, InvTypeValid).allstock ? 1 : 0)
func handleInvAllStock(s *ScriptState) error {
    typeID := s.PopInt()
    if err := checkInvType(s, typeID, "INV_ALLSTOCK"); err != nil {
        return err
    }
    if s.Configs.InvType(typeID).AllStock {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    return nil
}
```

**6. `handleNpcAttackRange` → OpNpcAttackRange** (`handlers_npc.go`)

```go
// TS NpcOps.ts:521-523 — checkedHandler(ActiveNpc, …)
//   pushInt(check(activeNpc.type, NpcTypeValid).attackrange)
//
// Goscape: ActiveNpc.NpcType() returns the type id; s.Configs.NpcType(id)
// looks up the config. checkNpcType (analogous to checkInvType) validates.
func handleNpcAttackRange(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_ATTACKRANGE"); err != nil {
        return err
    }
    typeID := s.ActiveNpc.NpcType()
    if err := checkNpcType(s, typeID, "NPC_ATTACKRANGE"); err != nil {
        return err
    }
    s.PushInt(int(s.Configs.NpcType(typeID).AttackRange))
    return nil
}
```

(Plan-author MUST grep `checkNpcType\b` to confirm presence and signature. If absent, add it adjacent to `checkInvType` mirroring the same shape. Per `plan_grep_helper_patterns.md`.)

**7. `handleNpcInRange` → OpNpcInRange** (`handlers_npc.go`)

```go
// TS NpcOps.ts:556-558 — checkedHandler(ActiveNpc, …)
//   pushInt(activeNpc.targetWithinMaxRange() ? 1 : 0)
func handleNpcInRange(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_INRANGE"); err != nil {
        return err
    }
    if s.ActiveNpc.TargetWithinMaxRange() {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    return nil
}
```

### §2.6 Handler registration

Add 7 lines to the `handlers` map in `pkg/script/handlers.go`:

```go
OpSay:             handleSay,
OpHeadIconsGet:    handleHeadIconsGet,
OpHeadIconsSet:    handleHeadIconsSet,
OpPExactMove:      handlePExactMove,
OpInvAllStock:     handleInvAllStock,
OpNpcAttackRange:  handleNpcAttackRange,
OpNpcInRange:      handleNpcInRange,
```

(Plan-author confirms alphabetical / numeric ordering convention by reading the existing map header; matches it.)

### §2.7 Test mock additions

`pkg/script/runner_test.go` `mockPlayer`:
- `sayCalls [][]byte` + `func (m *mockPlayer) Say(text []byte) { m.sayCalls = append(m.sayCalls, append([]byte(nil), text...)) }` — defensive copy avoids the caller-mutates-buffer trap.
- `headiconsValue int` + `headiconsSetCalls []int` + `HeadIcons() int { return m.headiconsValue }` + `SetHeadIcons(v int) { m.headiconsSetCalls = append(...); m.headiconsValue = v }`.
- `exactMoveCalls []struct{ sX, sZ, eX, eZ, begin, finish, dir int }` + `ExactMove(...)` recorder.
- `unsetMapFlagCalls int` + `UnsetMapFlag() { m.unsetMapFlagCalls++ }`.

`pkg/script/handlers_npc_test.go` `mockNpc`:
- `targetWithinMaxRangeValue bool` + `TargetWithinMaxRange() bool { return m.targetWithinMaxRangeValue }`.

(The `Say` field on `mockPlayer` does NOT collide with `mockNpc.sayCalls` — they live in different mock structs.)

### §2.8 InvType / NpcType mock backing

`mockConfigs` already supports per-id `InvType` and `NpcType` lookups (used by existing handler tests). The new tests register InvTypes/NpcTypes with the relevant field set:

```go
mc.invTypes[42] = &objtype.InvType{AllStock: true}
mc.npcTypes[7]  = &objtype.NpcType{AttackRange: 5}
```

Plan-author verifies the exact `mockConfigs` struct shape by grep at plan-write time per `mock_recorder_field_naming_check.md`.

---

## §3 Deviation register

| Tag | Site | Deviation | Rationale |
|---|---|---|---|
| `NAI-160-D-EXACTMOVE-COORDLEVEL-IGNORE` | `handlePExactMove` | TS unpacks `start.level` and `end.level` but only passes `x` and `z` into `exactMove(...)` — the level component is decoded then discarded. Goscape mirrors this by binding `_, sX, sZ` / `_, eX, eZ` and passing only horizontal coords to `ExactMove`. | TS-faithful per `audit_full_method_against_ts.md` — discarded-decode is intentional; the existing `(*Player).ExactMove(sX, sZ, eX, eZ, begin, finish, dir)` signature already omits level. No follow-up. |
| `NAI-160-D-INV-ALLSTOCK-NIL-DEFENSIVE` | `handleInvAllStock` | TS `invType.allstock` reads directly; goscape `s.Configs.InvType(typeID)` returns `*InvType` and could theoretically race-nil between `checkInvType` and dereference. Both calls happen on the same `s.Configs` reference in a single tick — race is impossible by goscape's tick model. No defensive re-check. | Per `defensive_gate_doc_comment_label.md` — only label "(goscape defensive; TS skips)" where the gate is load-bearing. Here it's not. No comment, no follow-up. |
| `NAI-160-D-NPC-ATTACKRANGE-WIDEN` | `handleNpcAttackRange` | `NpcType.AttackRange` is `uint16` on goscape (`npctype.go:168`); push as `int`. TS uses `number` (no integer width). Widen at the push site. | TS-faithful at value level; width is a Go-side artifact. No follow-up. |
| `NAI-160-D-HEADICONS-INT-WIDTH` | `handleHeadIconsSet` | `Player.headicons` is `int` (`player.go:209`); TS uses `number`. Setter accepts the popped `int` directly with no bounds check beyond `NumberNotNull`. TS does likewise. | TS-faithful. The on-wire encoder at `appearance.go:65` (`buf.P1(uint8(p.headicons))`) does the byte-truncation downstream; same byte-shape as TS Player.ts:1314 `stream.p1(this.headicons)`. No follow-up. |

---

## §4 Risk register

| Tag | Risk | Likelihood | Mitigation |
|---|---|---|---|
| `R1 [low]` | Pop-order inversion on P_EXACTMOVE (5-int handler matches `handler_pop_order_test_masking.md` pattern). | Med | Recorded-args test with 5 distinct integer values per slot (e.g. `start=10, end=20, startCycle=30, endCycle=40, direction=50`) — assertion checks each field by name, not by position. |
| `R2 [low]` | `sendUnsetMapFlag(p)` is unexported in `modules/world/handler_oploc.go` — plan-author needs to confirm the new `(*Player).UnsetMapFlag` method can call it from the same package. | Low | Same package (`world`); call is internal. Grep confirms `sendUnsetMapFlag` is package-local. |
| `R3 [resolved]` | `checkNpcType` existence — confirmed at brainstorm: `pkg/script/handlers_npc.go:45` (signature `func checkNpcType(s *ScriptState, id int, op string) error`). Reused as-is. | — | — |
| `R4 [low]` | Mock `Say` byte-buffer aliasing: tests that pop a string then mutate the underlying byte slice could corrupt prior `sayCalls` entries. | Low | Recorder uses `append([]byte(nil), text...)` for defensive copy. |
| `R5 [low]` | `OpHeadIconsSet` could write through `checkNotNull`-failing values if the gate runs after the field write. | Low | Pop → check → set order codified in §2.5 #3; mirror existing `handleIfOpenChat` pattern. |
| `R6 [low]` | `OpNpcInRange` reads `targetWithinMaxRange()` which depends on `n.target` and per-mode state. Calling it on a freshly-spawned NPC with `n.target == nil` returns `false` (verified at `npc_interaction.go:591-...`). | Low | TS-equivalent: `targetWithinMaxRange()` returns `false` when no target. No defensive gate needed. |
| `R7 [low]` | Adding methods to ActivePlayer interface may break existing in-package mocks beyond `mockPlayer`. | Low | Grep `pkg/script` for ActivePlayer impls — only `mockPlayer` (runner_test.go:99) and `*Player` (modules/world). NAI-159 just added LocOps surface; convention is well-trodden. |

---

## §5 Test strategy

**Per-handler positive test** (mirrors `TestNpcSay` `handlers_npc_test.go:602` template):
- Build `ScriptFile` with operand stack pre-loaded via `OpPushConstantInt` / `OpPushConstantString` push ops + the SUT opcode + `OpReturn`.
- `Init(sf, mockPlayer{...}, false, mockConfigs, nil)` → `state.Pointers |= PtrActivePlayer` (or `PtrActiveNpc`) → `Execute`.
- Assert recorder field (Say: `sayCalls`; HeadIconsSet: `headiconsSetCalls` + final `headiconsValue`; ExactMove: `exactMoveCalls[0]` matches all 7 named fields; InvAllStock: popped value via `state.PopInt()`).

**Per-handler require-active negative test** (mirrors `TestNpcSayRequiresActiveNpc` `handlers_npc_test.go:622`):
- Single-op `ScriptFile`, omit pointer set, `Execute` → expect error containing `"<OPNAME>: no active player"` (or `"no active npc"`).
- Folded into the existing `TestHandlersRequireActivePlayer` cases table at `handlers_player_test.go:831` for the 4 player ops, and into a parallel NPC table for the 2 NPC ops.

**P_EXACTMOVE protected-gate test:**
- Pre-load 5 ints + set `PtrActivePlayer` but NOT `PtrProtectedActivePlayer` → expect `"P_EXACTMOVE: script not protected"` error (matches `requireProtectedActivePlayer`'s error format at `handlers_player.go:62`; mirrors existing `requireProtectedActivePlayer` callers, e.g. `TestPTeleportRequiresProtected`).

**P_EXACTMOVE coord-validation test:**
- Pre-load `start = -1` (invalid packed coord) → expect `"P_EXACTMOVE: coord out of range (-1)"` error.

**HEADICONS_SET null-gate test:**
- Pre-load `-1` (goscape `checkNotNull` rejects `-1`; mirrors TS `NumberNotNull` which rejects `null` ≡ `-1` sentinel — verified at `handlers_player.go` `checkNotNull` body: `if v == -1 { return fmt.Errorf("%s: input number was null(-1)", op) }`) → expect error message `"HEADICONS_SET: input number was null(-1)"`.

**INV_ALLSTOCK invalid-type test:**
- Pre-load id with no registered InvType → expect `"INV_ALLSTOCK: no InvType with value (X) found"`.

**Sayempty-string test** (mirrors `TestNpcSay` empty-bubble convention at `player_masks.go:8` doc-comment):
- Push `""` then SAY → expect `sayCalls[0] == []byte{}` (or `len(sayCalls[0]) == 0`) and no error.

**Recorded-args test for P_EXACTMOVE pop order** (per `handler_pop_order_test_masking.md`):
- Push 5 distinct integers (e.g. 10, 20, 30, 40, 50) corresponding to `start=10, end=20, startCycle=30, endCycle=40, direction=50` (in TS popInts(5) order, which is push order). Assert `exactMoveCalls[0]` has `begin==30, finish==40, dir==50` and that `start=10` unpacks to `(sX, sZ)` via `coordgrid.UnpackCoord(10)` (and same for `end=20`).
- Critical: use `coordgrid.PackCoord(level, x, z)` to construct valid packed coords for `start` and `end` — passing raw `10`/`20` will fail `checkCoord` for some edge cases but is in range; verify the packed form via `coordgrid_test.go` conventions.

---

## §6 Smoke binding

Content callers exist for all 7 ops, but only `say` has high enough caller density (126) to produce a non-flaky observable smoke signal. Plan-author handoff to user requests:

1. Run goscape + Java client per `smoke_test_server_handoff.md`.
2. Trigger any NPC dialog (e.g., `talkto_man` outside Tutorial Island, or any chat-driven content). Expected: NPC's speech bubble appears with the line text (no `no handler for SAY` WARN in server logs).
3. Spot-check: trigger `headicons_set` via content (e.g., quest-state head-icon flag change) if reachable in a fresh tutorial. If not reachable, skip — handler is unit-pinned.

**Smoke is binding** on the SAY handler only (per `cascade_theory_smoke_binding.md`) — bubble visibility is the binding signal. The other 6 ops are unit-pinned with no additional smoke required; they have ≤10 callers each and most exist in conditional content paths (quest gates, combat ranges).

---

## §7 Cadence routing

Standard cadence per `runescript_cadence.md`:

1. **Spec** (this doc) → user review gate.
2. **Plan** (writing-plans skill, one task per opcode + setup/teardown tasks) → user review gate, `/clear` between plan and impl per `superpowers_clear_between_spec_and_impl.md`.
3. **Implementation** (subagent-driven-development, one subagent per task) — TDD with `RED → GREEN → REFACTOR`.
4. **Two-stage review** — single combined Sonnet `superpowers:code-reviewer` (per `superpowers_code_reviewer_model.md`) at end-of-impl, NOT per-task.
5. **Smoke handoff** to user for the SAY-binding signal.
6. **Close commit** with `Closes memory:` trailer (see §10 below).

`subagent-driven-development` mode mandatory per `execution_mode_default.md`. No menu, no in-session implementation.

---

## §8 Out of scope

- The 21 remaining unhandled opcodes (see §1 cohort filter table). Forward-routable.
- Any change to `(*Player).Say` body — the existing impl at `player_masks.go:8-11` matches TS Player.ts:1893-1896 exactly (2-field write + mask bit).
- Any change to `(*Player).ExactMove` body — exists at `player_masks.go:28-37`, sets 7 fields + mask bit.
- `sendUnsetMapFlag(p)` internals — unchanged; the new `(*Player).UnsetMapFlag` is a single-line wrapper.
- Refactoring the unhandled-handler audit one-liner (`missing_handler_audit.md`) — orthogonal.
- Content-side `say` audits beyond confirming the smoke signal. Per `smoke_surfaces_adjacent_divergences.md`, content divergences route to NAI-161+.

---

## §9 Memory hits

Cited in close-commit `Closes memory:` trailer per `close_commit_memory_trailer.md`:

- `runescript_cadence.md` — full cadence routing
- `controller_preflight.md` — preflight performed before brainstorm
- `compressed_cadence.md` — considered (and rejected) for ≤15 LOC framing; full cadence chosen for 70+ LOC scope
- `missing_handler_audit.md` — 28-opcode cascade-tail measurement
- `audit_full_method_against_ts.md` — each TS body audited line-by-line; EXACTMOVE level-discard pinned as deviation
- `handler_pop_order_test_masking.md` — P_EXACTMOVE recorded-args test with 5 distinct ints
- `plan_grep_helper_patterns.md` / `plan_sibling_site_guard_audit.md` — `checkNpcType` / `checkInvType` / `requireActivePlayer` shape reuse
- `mock_recorder_field_naming_check.md` — mockConfigs / mockPlayer / mockNpc field names grep-verified at plan-write
- `defensive_gate_doc_comment_label.md` — InvType / NpcType nil-check labeled (or omitted) by rule
- `true_to_ts_gate.md` — 4 deviations tracked with rationale + follow-up disposition
- `spec_ts_source_read.md` — every TS body read verbatim, no analogy
- `smoke_test_server_handoff.md` / `cascade_theory_smoke_binding.md` — SAY-bubble binding signal
- `enumerate_all_sites.md` — handler registration sites enumerated (single `handlers` map; 7 lines)
- `superpowers_clear_between_spec_and_impl.md` — `/clear` between plan and impl
- `superpowers_code_reviewer_model.md` — single Sonnet reviewer at end-of-impl
- `execution_mode_default.md` — subagent-driven-development

---

## §10 No-deviations audit

Per `spec_ts_init_value_audit.md` and `spec_diagram_order_divergence.md`, this spec was authored from TS source line reads (not from neighbor-handler analogy):

- SAY: PlayerOps.ts:462-464 read verbatim; `Player.say` body read at Player.ts:1893-1896 (`this.sayMessage = message; this.masks |= PlayerInfoProt.SAY`). Goscape `(*Player).Say` at `player_masks.go:8-11` does the same. No divergence.
- HEADICONS_GET / HEADICONS_SET: PlayerOps.ts:980-986 read verbatim. `Player.headicons` field at `Player.ts:314` defaults `= 0`. Goscape `Player.headicons` defaults zero by Go's zero-value rule. No divergence.
- P_EXACTMOVE: PlayerOps.ts:881-890 read verbatim. TS popInts(5) order: `[start, end, startCycle, endCycle, direction]` — `direction` is top-of-stack. Goscape pop order codified in §2.5 #4 with comment.
- INV_ALLSTOCK: InvOps.ts:20-24 read verbatim. `InvType.allstock` parsed at goscape `invtype.go:54` (`t.AllStock = true`). No divergence.
- NPC_ATTACKRANGE: NpcOps.ts:521-523 read verbatim. `NpcType.attackrange` parsed at goscape `npctype.go:273` (`t.AttackRange = dat.G2()`). Returns `uint16`; widened to `int` at push site (deviation `NAI-160-D-NPC-ATTACKRANGE-WIDEN`).
- NPC_INRANGE: NpcOps.ts:556-558 read verbatim. `targetWithinMaxRange` body at goscape `npc_interaction.go:591` ports `Npc.targetWithinMaxRange` at `Npc.ts` line range (plan-author re-confirms TS line at plan-write).

No init-value or diagram-order divergences detected.
