# NAI-71 — OPHELD handler family port (OPHELD1-5, OPHELDT, OPHELDU)

**Status:** Spec written 2026-05-02.
**Predecessor:** NAI-70 (HEAD `6976188`). Net deviation tally entering: 12.
**Opens:** `NAI-71-D-OPHELD-NO-SESSION-LOG` (one new deviation).
**Closes:** none.
**Tech stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`.

## 1. Background

Seven game opcodes in `pkg/io/protocol/game/client/prot.go` accept payload
sizes today but have no entry in `gameHandlers[]`, so `(*Player).readPacket`
silently discards them at `modules/world/player.go:771-775`:

| Opcode | Name | Payload | TS handler |
|---|---|---|---|
| 195 | OPHELD1 | 6 | `OpHeldHandler.ts` |
| 71 | OPHELD2 | 6 | `OpHeldHandler.ts` |
| 133 | OPHELD3 | 6 | `OpHeldHandler.ts` |
| 157 | OPHELD4 | 6 | `OpHeldHandler.ts` |
| 211 | OPHELD5 | 6 | `OpHeldHandler.ts` |
| 48 | OPHELDT | 8 | `OpHeldTHandler.ts` |
| 130 | OPHELDU | 12 | `OpHeldUHandler.ts` |

These are the inventory-side counterparts of OPLOC/OPNPC/OPOBJ. The user
right-clicks a held inventory item (OPHELD1-5), drags a spell onto an
inventory item (OPHELDT), or drags one inventory item onto another
(OPHELDU). Unlike OPLOC/OPNPC/OPOBJ, OPHELD does **not** engage the
walk-to-target interaction system — the target is in the player's own
inventory, so TS fires the script directly via
`player.executeScript(ScriptRunner.init(script, player), true)` without
calling `setInteraction`.

The closest goscape analog is `handler_inv_button.go`:
both decode `(obj, slot, com)`, validate via `invListeners +
resolveListenerInv + inv.HasAt`, and fire a script directly via
`s.runScript(sf, p, nil, protect, nil, nil)`. OPHELD differs from
INV_BUTTON in:

- Component gate: `Operable` (OPHELD1-5) or `Usable` (OPHELDT/U)
  vs `Iop[op-1] != ""` (INV_BUTTON).
- Per-op item gate: `objType.IOp[op-1] != ""` (OPHELD1-5 only).
- Mask emission: `faceEntity = -1; masks |= entitymask` (INV_BUTTON
  does neither).
- moveClickRequest: cleared on OPHELD1-5 only.
- Trigger lookup: keyed on `objType.id` with `objType.Category`
  fallback (OPHELD1-5), or `getByTriggerSpecific` with 4-step
  fallback chain (OPHELDU).

## 2. Current state at HEAD

### 2.1 Dependencies (all verified present)

| Dependency | goscape location |
|---|---|
| Trigger constants `TriggerOpHeld1..5/T/U` | `pkg/script/trigger.go:142-148` |
| `ObjType.IOp []string` | `pkg/objtype/objtype.go:109` |
| `ObjType.Category int` | `pkg/objtype/objtype.go:132` |
| `ObjType.Members bool` | `pkg/objtype/objtype.go:107` |
| `Component.Operable bool` | `pkg/objtype/componenttype.go:64` |
| `Component.Usable bool` | already in use (`handler_inv_button.go`, `handler_opobj.go`) |
| `ComActionTargetHeld = 16` | `pkg/objtype/componenttype.go:42` |
| `Component.RootLayer int` | `pkg/objtype/componenttype.go:49` |
| `Player.lastItem / lastSlot / lastUseItem / lastUseSlot` | `modules/world/player.go:227` |
| `Player.faceEntity / entitymask / masks` | `modules/world/player.go` (used by `interaction.go:107-114`) |
| `Player.moveClickRequest` | `modules/world/player.go:204` |
| `Player.modalMain` | `modules/world/player.go:212` |
| `(*Player).IsComponentVisible(com)` | used in `handler_inv_button.go:39` |
| `(*Player).ClearPendingAction()` | used in `handler_oploc.go:88` |
| `(*Server).lookupComponent(comId)` | used in `handler_inv_button.go:35` |
| `(*Player).invListeners` map | used in `handler_inv_button.go:46` |
| `resolveListenerInv(s, listener)` | used in `handler_inv_button.go:50` |
| `inv.HasAt(slot, obj)` | used in `handler_inv_button.go:54` |
| `(*Server).runScript(sf, p, nil, protect, nil, nil)` | used in `handler_inv_button.go:65` |
| `(*Server).scriptProvider.GetByTrigger / GetByTriggerSpecific` | `pkg/script/provider.go:114, 145` |
| `(*Server).cfg.NodeMembers` | used in `handler_oploc.go:283` |
| `(*Server).objTypes.Configs[id]` | used in `handler_opobj.go:62-66` |
| `(*Player).MessageGame(s)` | used in `handler_oploc.go:284` |

### 2.2 Goscape conventions (confirmed via grep)

- **No-script fallback message.** "Nothing interesting happens." is
  the established no-script-found message in goscape
  (`interaction_trigger.go:160, 558`); also used in OPOBJ tests at
  `handler_opobj_test.go:601`. OPHELDT and OPHELDU port this directly.
- **NODE_DEBUG "No trigger for [...]"** debug message — TS conditional
  on `Environment.NODE_DEBUG`. Goscape does **not** adopt this — every
  existing handler skips the NODE_DEBUG-only debug line. NAI-71
  follows the existing convention (no deviation tag — established
  precedent).
- **addSessionLog(LoggerEventType.MODERATOR, ...)** — no session-log
  subsystem exists in goscape. The TS calls at `OpHeldHandler.ts:64`
  and `OpHeldTHandler.ts:61` are skipped. Tagged as deviation
  `NAI-71-D-OPHELD-NO-SESSION-LOG`. Closure path: future moderator-
  logging sub-spec ports the LoggerEventType registry and session-log
  buffer; OPHELD is one of many call sites that will activate. Not
  a behavioral divergence on the wire — purely admin-tooling deferral.

### 2.3 Verified-absent claims (premise grep evidence)

Per `risk_register_premise_grep.md` and `controller_preflight.md`:

```
$ rg -n "handleOpHeld|TriggerOpHeld[1-5UT]\b" modules/world/ | grep -v test
modules/world/handler_oploc.go: ... (no OPHELD hits)
$ rg -n "session.*log|sessionLog|LoggerEventType" pkg/ modules/
(no hits — confirms TS addSessionLog has no goscape counterpart)
$ rg -n "ObjType.*Category|ObjType\.Category" pkg/objtype/
pkg/objtype/objtype.go:132:    Category    int    (verified present)
```

### 2.4 Outstanding TS-fidelity gaps (NOT in scope for NAI-71)

- **Player.input subsystem** — required for full EVENT_TRACKING port
  (opcode 81). Out of scope.
- **Session log / LoggerEventType** — needs full moderator-logging
  port. Out of scope; tracked under the new deviation.

## 3. Scope

### 3.1 Production changes

#### File: `modules/world/handler_opheld.go` (new, ~280 LOC)

Three handler functions plus five 1-line dispatch wrappers:

```go
// handleOpHeld is the shared implementation for OPHELD1..OPHELD5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Gates per TS OpHeldHandler.ts:
//  1. delayed player → drop
//  2. payload < 6 bytes → drop
//  3. nil component or !Operable → drop
//  4. !IsComponentVisible → drop
//  5. comId not in invListeners → drop
//  6. listener's inventory unresolved → drop
//  7. inv.HasAt(slot, obj) false → drop
//  8. ObjType not registered or IOp[op-1] == "" → drop
//
// On pass: lastItem/Slot snapshot → if com.RootLayer != p.modalMain
// then ClearPendingAction → moveClickRequest=false → faceEntity=-1
// + emit entitymask → fire [opheld<op>,<objId>] with category fallback
// via runScript(sf, p, nil, true, nil, nil).
//
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: TS OpHeldHandler.ts:62-65
// calls addSessionLog(MODERATOR, ...) for op != 5. Skipped — no session
// log subsystem in goscape.
func handleOpHeld(p *Player, payload []byte, op int) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    s := p.client.server
    if p.delayed && s.currentTick < p.delayedUntil {
        return nil
    }
    if len(payload) < 6 {
        return nil
    }
    r := packet.NewPacket(payload)
    obj := int(r.G2())
    slot := int(r.G2())
    comId := int(r.G2())

    com := s.lookupComponent(comId)
    if com == nil || !com.Operable {
        return nil
    }
    if !p.IsComponentVisible(com) {
        return nil
    }

    listener, ok := p.invListeners[comId]
    if !ok {
        return nil
    }
    inv := resolveListenerInv(s, listener)
    if inv == nil {
        return nil
    }
    if !inv.HasAt(slot, obj) {
        return nil
    }

    if s.objTypes == nil || obj < 0 || obj >= len(s.objTypes.Configs) {
        return nil
    }
    objType := s.objTypes.Configs[obj]
    if objType == nil {
        return nil
    }
    if len(objType.IOp) < op || objType.IOp[op-1] == "" {
        return nil
    }

    p.lastItem = obj
    p.lastSlot = slot

    if com.RootLayer != p.modalMain {
        p.ClearPendingAction()
    }

    p.moveClickRequest = false
    if p.faceEntity != -1 {
        p.faceEntity = -1
    }
    p.masks |= p.entitymask

    trigger := script.TriggerOpHeld1 + script.ServerTriggerType(op-1)
    sf := s.scriptProvider.GetByTrigger(trigger, obj, objType.Category)
    s.runScript(sf, p, nil, true, nil, nil)
    return nil
}

func handleOpHeld1(p *Player, payload []byte) error { return handleOpHeld(p, payload, 1) }
func handleOpHeld2(p *Player, payload []byte) error { return handleOpHeld(p, payload, 2) }
func handleOpHeld3(p *Player, payload []byte) error { return handleOpHeld(p, payload, 3) }
func handleOpHeld4(p *Player, payload []byte) error { return handleOpHeld(p, payload, 4) }
func handleOpHeld5(p *Player, payload []byte) error { return handleOpHeld(p, payload, 5) }
```

```go
// handleOpHeldT — TS OpHeldTHandler.ts. Wire format:
//   obj:G2 | slot:G2 | com:G2 | spellCom:G2 (8 bytes).
//
// Gates per TS:
//  1. delayed → drop
//  2. payload < 8 → drop
//  3. spellCom: nil or (ActionTarget & HELD) == 0 → drop
//  4. spellCom: !IsComponentVisible → drop
//  5. com: nil or !Usable → drop
//  6. com: !IsComponentVisible → drop
//  7. comId not in invListeners → drop
//  8. listener's inventory unresolved → drop
//  9. inv.HasAt(slot, obj) false → drop
//
// On pass: lastItem/Slot snapshot → ClearPendingAction (unconditional)
// → faceEntity=-1 + emit entitymask → look up
// [opheldt,<spellComId>] via GetByTrigger(typeID=spellComId, cat=-1) →
// fire with runScript(...,protect=true...). On no-script: emit
// "Nothing interesting happens.".
//
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG also covers
// OpHeldTHandler.ts:61.
func handleOpHeldT(p *Player, payload []byte) error { ... }
```

```go
// handleOpHeldU — TS OpHeldUHandler.ts. Wire format:
//   obj:G2 | slot:G2 | com:G2 | useObj:G2 | useSlot:G2 | useCom:G2
//   (12 bytes).
//
// Gates per TS:
//  1. delayed → drop
//  2. payload < 12 → drop
//  3. comId != useComId → drop
//  4. com: nil or !Usable → drop, !IsComponentVisible → drop
//  5. useCom: nil or !Usable → drop, !IsComponentVisible → drop
//  6. comId not in invListeners → drop; inv unresolved → drop;
//     !validSlot(slot) → drop; !inv.HasAt(slot, obj) → moveClickRequest=
//     false + ClearPendingAction + drop
//  7. useComId not in invListeners → drop; useInv unresolved → drop;
//     !validSlot(useSlot) → drop; !useInv.HasAt(useSlot, useObj) →
//     moveClickRequest=false + ClearPendingAction + drop
//  8. members-only items on free world → MessageGame(...) + drop
//
// On pass: lastItem/lastSlot/lastUseItem/lastUseSlot snapshot →
// ClearPendingAction → faceEntity=-1 + emit entitymask → 4-step
// trigger fallback:
//   (a) [opheldu,objType.id]   via GetByTriggerSpecific
//   (b) [opheldu,useObjType.id] via GetByTriggerSpecific
//       — on hit: swap (lastItem,lastUseItem) and (lastSlot,lastUseSlot)
//   (c) [opheldu,-1,objType.Category] via GetByTriggerSpecific
//   (d) [opheldu,-1,useObjType.Category] via GetByTriggerSpecific
//       — on hit: swap (lastItem,lastUseItem) and (lastSlot,lastUseSlot)
// On any hit: runScript(...,protect=true...). On miss: emit
// "Nothing interesting happens.".
//
// Note: TS calls (a) "[opheldu,b]" and (b) "[opheldu,a]" because TS
// passes the inventory-listed item as "b" and the dragged-onto item
// as "a". Goscape mirrors the lookup sequence; comments cite TS
// labels.
func handleOpHeldU(p *Player, payload []byte) error { ... }
```

#### File: `modules/world/handlers_game.go`

Add 7 registrations to `init()`:

```go
gameHandlers[195] = handleOpHeld1 // OPHELD1
gameHandlers[71] = handleOpHeld2  // OPHELD2
gameHandlers[133] = handleOpHeld3 // OPHELD3
gameHandlers[157] = handleOpHeld4 // OPHELD4
gameHandlers[211] = handleOpHeld5 // OPHELD5
gameHandlers[48] = handleOpHeldT  // OPHELDT
gameHandlers[130] = handleOpHeldU // OPHELDU
```

All seven handler entry points (`handleOpHeld1..5`, `handleOpHeldT`,
`handleOpHeldU`) live in `handler_opheld.go` and are referenced
directly by the dispatch table. No adapter wrappers in
`handlers_game.go` — pattern matches `handler_oploc.go` /
`handler_opobj.go`, not the `handler_inv_button.go` Server-method
shape.

### 3.2 Test changes

#### File: `modules/world/handler_opheld_test.go` (new, ~600 LOC)

Test patterns mirror `handler_inv_button_test.go` and
`handler_opobj_test.go`. Use the existing `mockScriptProvider` /
`newTestPlayer` helpers.

For each handler, pin:

1. **Reject gates** — one test per gate (delayed, short payload,
   nil/non-visible com, com-not-operable [or non-usable], no
   listener, no inv, slot-mismatch, ObjType not registered, IOp[op-1]
   empty, etc.). Assertion: no `runScript` call recorded; lastItem /
   lastSlot unchanged.
2. **Happy path script-fire** — verify `runScript` is called with
   the expected `(trigger, typeID, categoryID)` triple. Pin
   lastItem/lastSlot post-snapshot. Pin `moveClickRequest=false`,
   `faceEntity=-1`, mask emit (`p.masks & p.entitymask != 0`).
3. **OPHELD1-5 modal-vs-rootLayer** — dual-pin:
   - `com.RootLayer == p.modalMain` ⇒ `ClearPendingAction` NOT called
     (use a sentinel; e.g., set `p.target` non-nil and verify it
     persists post-handler).
   - `com.RootLayer != p.modalMain` ⇒ `ClearPendingAction` called.
4. **OPHELDU 4-step trigger fallback** — separate test per arm:
   - hit on (a) only — no swap
   - hit on (b) only — pin swap (`lastItem ↔ lastUseItem`)
   - hit on (c) only — no swap
   - hit on (d) only — pin swap
   - all four miss — emit "Nothing interesting happens."
5. **OPHELDU members gate** — members-only object on
   `cfg.NodeMembers=false` ⇒ MessageGame emits "To use this item
   please login to a members' server." + no script call.
6. **OPHELDU comId != useComId reject** — pin separately.
7. **OPHELDT / OPHELDU no-script "Nothing interesting happens."**
   absence/presence pins.
8. **dispatch-table registration** — table-driven test pinning all 7
   opcodes resolve to non-nil `gameHandlers[]` entries.

### 3.3 Deviation tag

Open `NAI-71-D-OPHELD-NO-SESSION-LOG`. Inline doc-comments at three
sites: handle_opheld.go's `handleOpHeld` (covering op=1..5
addSessionLog skip) and `handleOpHeldT` (covering OpHeldTHandler.ts:61
addSessionLog skip). Plan-author records the LOC count post-write.

### 3.4 Net deviation tally

12 → 13 (open 1, close 0).

## 4. Files diff summary

| File | Production | Test |
|---|---|---|
| `modules/world/handler_opheld.go` (new) | +280 prod | — |
| `modules/world/handlers_game.go` | +9 (7 registrations + comment block) | — |
| `modules/world/handler_opheld_test.go` (new) | — | +600 |

Approximate total: ~290 production / ~600 test.

## 5. Cadence

Per `runescript_cadence.md`. Three implementation tasks + close.

| Task | Scope | Sub-agent | Reviewer |
|---|---|---|---|
| T1 | OPHELD1-5 (shared core + 5 wrappers + happy-path + reject gates) | implementer | code-reviewer (Sonnet, pattern-lock) |
| T2 | OPHELDT (handler + reject gates + no-script fallback + dispatch-registration test) | implementer | (no review — small + bracketed by T1/T3) |
| T3 | OPHELDU (handler + 4-step fallback + members gate + reject gates) | implementer | code-reviewer (Sonnet, pre-close) |
| T4 | Close commit + nai_followups.md entry | controller | — |

Two-stage review reasoning: T1 locks the file structure and naming
for T2/T3 to mirror; T3 is the most-complex handler in the bundle
(4-step fallback + lastItem swap) and warrants pre-close audit.

Dispatch via `superpowers:subagent-driven-development` per
`execution_mode_default.md`. Code review via Sonnet only per
`superpowers_code_reviewer_model.md`.

## 6. Risk register (premise verification at spec-write)

Per `risk_register_premise_grep.md`, `controller_preflight.md`, and
`spec_test_runtime_behavior_verify.md`:

| Premise | Verification at spec-write |
|---|---|
| `Component.Operable bool` exists | ✅ `pkg/objtype/componenttype.go:64` |
| `Component.Usable bool` exists | ✅ used in `handler_opobj.go:238`, `handler_inv_button.go` |
| `ComActionTargetHeld = 16` exists | ✅ `pkg/objtype/componenttype.go:42` |
| `ObjType.IOp []string` (capital I, lowercase op) | ✅ `pkg/objtype/objtype.go:109` — note lowercase-`Op` after capital `I` |
| `ObjType.Category int` exists | ✅ `pkg/objtype/objtype.go:132` |
| `ObjType.Members bool` exists | ✅ `pkg/objtype/objtype.go:107` |
| `Player.modalMain int` exists | ✅ `modules/world/player.go:212` |
| `Player.lastItem` etc. exist | ✅ `modules/world/player.go:227` |
| `Player.moveClickRequest` exists | ✅ `modules/world/player.go:204` |
| `Player.faceEntity / entitymask / masks` exist | ✅ used in `interaction.go:107-114`, `player_source.go:15` |
| Trigger constants `TriggerOpHeld1..5/T/U` exist | ✅ `pkg/script/trigger.go:142-148` |
| `(*Server).runScript` signature | ✅ `runScript(sf, p, nil, protect, nil, nil)` per `handler_inv_button.go:65` |
| `(*Server).scriptProvider.GetByTrigger / GetByTriggerSpecific` exist | ✅ `pkg/script/provider.go:114, 145` |
| OPHELDU's "comId === useComId" gate (not "≠") | ✅ TS `OpHeldUHandler.ts:21` (`if (comId !== useComId) return false`) — spec uses `!=` reject |
| OPHELDU swap pairs: `(lastItem, lastUseItem)` AND `(lastSlot, lastUseSlot)` | ✅ TS `OpHeldUHandler.ts:101-102, 115-116` (both pairs swap together) |
| TS no-`addSessionLog` for OPHELD op=5 | ✅ `OpHeldHandler.ts:63` (`if (message.op !== 5)` gate) |
| TS unconditional `clearPendingAction` for OPHELDT/U | ✅ `OpHeldTHandler.ts:57`, `OpHeldUHandler.ts:86` |
| TS conditional `clearPendingAction` for OPHELD1-5 (only when rootLayer mismatch) | ✅ `OpHeldHandler.ts:54-56` |
| TS unconditional `masks |= entitymask` (not `if faceEntity != -1`) | ✅ `OpHeldHandler.ts:60`, `OpHeldTHandler.ts:59`, `OpHeldUHandler.ts:88` — all unconditional |
| Goscape "Nothing interesting happens." convention | ✅ `interaction_trigger.go:160, 558` |
| `handler_inv_button.go` is the structural template | ✅ closest analog (no SetInteraction; direct runScript fire) |

### 6.1 Premise: faceEntity emit semantics

TS does `player.faceEntity = -1; player.masks |= player.entitymask;`
unconditionally — even if `faceEntity` was already -1. Goscape's
existing `interaction.go:107-114` pattern is conditional
(`if p.faceEntity != X { p.faceEntity = X; p.masks |= p.entitymask }`).
Per `true_to_ts_gate.md`, the spec mirrors TS exactly: emit the mask
unconditionally. The conditional-set on `faceEntity` is harmless
(value already -1 most commonly), but the mask `|=` is unconditional.

Rationale: TS's mask field is only consumed by the next encoder
flush; setting `entitymask` redundantly when already-set has no wire
effect (idempotent OR). Spec codifies the TS-faithful unconditional
emit.

### 6.2 Premise: ScriptProvider lookup keys

OPHELD1-5 uses `GetByTrigger(trigger, objType.id, objType.Category)`.
The third argument is `categoryID`, used for the category-fallback
arm in `provider.go:114-127`. Confirmed shape matches TS
`ScriptProvider.getByTrigger(trigger, id, category)`.

OPHELDU uses `GetByTriggerSpecific` (no category fallback) for each
of the 4 explicit lookup arms. Confirmed shape matches TS
`getByTriggerSpecific(trigger, id, category)` at `OpHeldUHandler.ts:96-114`.

OPHELDT uses `GetByTrigger(trigger, spellComId, -1)` per
`OpHeldTHandler.ts:63`.

### 6.3 Goscape-only gates not in TS

OPHELD1-5: TS does NOT include an explicit `objType == undefined`
gate (TS's `ObjType.get(objId)` throws on unknown id, killing the
handler tick). Goscape's `objTypes.Configs[id] == nil` defensive
check is required because goscape has tolerant config loading.
Tagged inline as "(goscape defensive; TS throws here)" per
`defensive_gate_doc_comment_label.md`.

### 6.4 Test-fixture runtime check (per `plan_runnable_test_fixtures.md`)

Each test fixture in the plan must be mentally-runnable. Implementer
prompts include "compile each test function in your head before
committing". Specifically:
- OPHELDU swap pin tests must verify post-call values match the
  expected swap direction; an off-by-one swap (item vs slot) silently
  passes when both are equal.
- "Nothing interesting happens." absence-pin tests must drain the
  player's connection bytes; per `ts_asymmetry_dual_pin.md`,
  pair every absence-pin with a presence-pin in a different test
  to escalate on regression.

## 7. Out of scope

- EVENT_TRACKING (opcode 81) handler. Requires Player.input port —
  unrelated subsystem.
- ANTICHEAT_OPLOGIC1-9 / ANTICHEAT_CYCLELOGIC1-6 / EVENT_CAMERA_POSITION
  silent-consume opcodes. Already TS-fidelity match — adding stubs
  would be cosmetic with no behavioral payoff (per
  `dead_api_polish.md`).
- FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL, MESSAGE_PRIVATE,
  CHAT_SETMODE, REPORT_ABUSE handlers. Each is its own NAI sub-spec.
- Session-log / LoggerEventType subsystem port. Tracked as new
  deviation `NAI-71-D-OPHELD-NO-SESSION-LOG`.
- `objType.iop[op-1] === null` (TS) vs `objType.IOp[op-1] == ""`
  (goscape). Goscape's loctype/objtype decoders coerce "hidden" to ""
  at load time — same semantics, different sentinel. No deviation
  tag (consistent with `handler_oploc.go:83` precedent).

## 8. Memory / lessons applied

- `runescript_cadence.md` — full sub-spec cadence, 3 tasks + close.
- `true_to_ts_gate.md` — every divergence cited against TS source line.
- `controller_preflight.md` + `risk_register_premise_grep.md` —
  every premise verified with grep+Read at spec-write (§6).
- `dead_api_polish.md` — drove rejection of Scope A (anticheat
  stubs with zero behavioral payoff).
- `defensive_gate_doc_comment_label.md` — `objType == nil` gate
  flagged as goscape-defensive.
- `plan_grep_helper_patterns.md` — reuses
  `lookupComponent / IsComponentVisible / resolveListenerInv /
  inv.HasAt / runScript` rather than inlining.
- `enumerate_all_sites.md` — all 7 OPHELD opcode sites enumerated
  in §1 + §3.
- `superpowers_code_reviewer_model.md` — code review on Sonnet only.
- `execution_mode_default.md` — dispatch via subagent-driven-development.
- `close_commit_memory_trailer.md` — close commit will carry
  `Closes memory: NAI-71-D-OPHELD-NO-SESSION-LOG` (note: tag is
  *opened* not closed; trailer phrasing is "Opens memory:" — confirm
  convention at close-commit time).
- `ts_asymmetry_dual_pin.md` — applied to OPHELDU 4-step fallback
  arms and "Nothing interesting happens." pins.
- `plan_doc_replaceall_timeline.md` — per-site Edits, no
  `replace_all` across the 3 handlers.
- `verify_implementer_claims.md` — fresh `go test ./... -count=1`
  after each task, not just package-scoped.
- `spec_followup_tracker_freshness.md` — at plan-write time,
  re-grep all §6 premises against HEAD (NAI-70 close commit may
  shift line numbers cited above).

## 9. Close-commit deviations summary (template)

```
NAI-71 — OPHELD handler family port

Adds 7 game-opcode handlers (OPHELD1-5, OPHELDT, OPHELDU) closing
the "missing handler" gap in modules/world/handlers_game.go for the
inventory-side held-item interaction family. Mirrors TS
OpHeldHandler / OpHeldTHandler / OpHeldUHandler line-by-line.

No production behavioral change for opcodes already handled.
Activates inventory-item right-click / spell-on-item / item-on-item
script fires that previously silent-consumed at player.go:771-775.

Net deviation tally: 12 → 13 (opens 1, closes 0)
  Opens: NAI-71-D-OPHELD-NO-SESSION-LOG (TS addSessionLog skipped
         pending session-log subsystem port)

Implementation: T1 OPHELD1-5, T2 OPHELDT, T3 OPHELDU.

Spec: docs/superpowers/specs/2026-05-02-nai-71-opheld-handler-port-design.md
Plan: docs/superpowers/plans/2026-05-02-nai-71-opheld-handler-port.md

Opens memory: NAI-71-D-OPHELD-NO-SESSION-LOG
```
