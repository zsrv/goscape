---
status: brainstorm-approved
date: 2026-05-11
ts_source:
  # B0 (stubs — no handlers/* case-label in TS)
  - LostCityRS/Engine-TS/src/engine/script/ScriptOpcode.ts:22-23 (PUSH_VARBIT, POP_VARBIT)
  - LostCityRS/Engine-TS/src/engine/script/ScriptOpcode.ts:296 (LC_OP)
  - LostCityRS/Engine-TS/src/engine/script/ScriptOpcode.ts:306,309 (OC_IOP, OC_OP)
  - LostCityRS/Engine-TS/src/engine/script/ScriptOpcode.ts (SET_GENDER — declared, no handler)
  # B1 (trivial + small — TS bodies)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:931-933 (LAST_LOGIN_INFO)
  - LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts:792-796 (INV_TOTALPARAM_STACK)
  - LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:139-143 (MAP_INDOORS)
  - LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:241-257 (NPC_STATHEAL)
  # B2 (WealthEvent + player-interaction + NAI-115-D1 retirement)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1191-1202 (WEALTH_EVENT)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:922-929 (P_LOCMERGE)
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1127-1135 (P_OPPLAYERT)
  # B3 (inv-drops)
  - LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts:672-723 (BOTH_DROPSLOT)
  - LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts:726-790 (INV_DROPALL)
---

# NAI-162 — Trivial-handler sweep #4 (15 ops; 4 bundles)

**Cadence:** ~270 LOC handler code + ~100 LOC new methods + ~250 LOC tests = ~620 LOC across four bundles. Sits well above NAI-161's 110 LOC envelope; runs as one sub-spec with per-bundle close commits (B0 → B1 → B2 → B3) and a final NAI-162 roll-up close.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Cascade-tail context:** missing-handler audit at HEAD `2058b72` reports **18 unhandled opcodes** (`missing_handler_audit.md` one-liner). This sub-spec ports 15; the 3 heavy ops (LineOfSight 1005, NpcAdd 2500, NpcHunt 2525) forward-route to NAI-163. Post-close cascade-tail: 18 → 3.

---

## §1 Symptom / motivation

NAI-161 closed sweep #3 (3 ops, 21 → 18). The remaining 18 split sharply by binding cost; ground-truthed against TS at brainstorm time:

| Class | Ops | Count | TS body present? |
|---|---|---:|---|
| **Stubs (TS-unimplemented)** | PushVarbit 25, PopVarbit 27, SetGender 2099, LcOp 4105, OcIop 4205, OcOp 4208 | 6 | No `handlers/*` case-label in TS — stub-with-pin pattern (NAI-161 T6 P_OPHELD shape) |
| **Trivial+small** | LastLoginInfo 2054, InvTotalParamStack 4329, MapIndoors 1010, NpcStatHeal 2539 | 4 | Yes — 1–5 line bodies + 1 new helper method each |
| **WealthEvent + player-interaction** | WealthEvent 2131, PLocMerge 2074, POpPlayerT 2082 | 3 | Yes — small bodies; WealthEvent lands `(*Player).AddWealthEvent` |
| **Inv drops** | BothDropSlot 4300, InvDropAll 4309 | 2 | Yes — moderate bodies; both call `addWealthEvent` for SCOPE_PERM (depends on B2 landing first) |
| **Deferred to NAI-163** | LineOfSight 1005, NpcAdd 2500, NpcHunt 2525 | 3 | Yes — each is a discrete new subsystem (raycast / entity-create / closest-NPC scan + HuntVis) |

**Cohort definition (sweep #4 — relaxed-NAI-161):**
1. **Bounded new infra** — each new method ≤30 LOC; no new subsystems. Excludes raycast, entity-create, and closest-NPC scan (NAI-163).
2. **TS body ≤30 lines** OR **TS-unimplemented stub** — B0 admits TS-unimplemented ops as stub-with-pin per NAI-161 T6 precedent.
3. **No content-caller-density gate** — sweep #4 prioritises closing the cascade-tail over content-callers (sweep #3 already drained the high-density tail).

**Bundle-level dependencies (drives ordering):**

* B1 NPC_STATHEAL calls `npc.heroPoints.clear()` when HP fully heals (TS NpcOps.ts:254-256). Goscape has no `(*HeroPoints).Clear()` — **B1 lands it as a prerequisite**.
* B3 BothDropSlot and InvDropAll both call `state.activePlayer.addWealthEvent(...)` for `InvType.SCOPE_PERM` (TS InvOps.ts:695-700, 759-767, 781-789). **B2 must land `(*Player).AddWealthEvent` before B3 ships inv-drop handlers.**
* B2 WealthEvent retires the **NAI-115-D1 deviation chain** in `handlers_inv.go` and `handlers_obj.go` (deviation comments at handlers_inv.go:776, 784-786, 852-853, 1245-1247 + handlers_obj.go sites). Retirement flips behavior: the SCOPE_PERM drop path that currently skips `addWealthEvent` will start firing it.

**Bundle ordering: B0 → B1 → B2 → B3.** Each bundle has its own close commit; final NAI-162 close commit rolls them up and cites the audit recount 18 → 3.

**Re-confirmed at HEAD `2058b72`:**

| Op | Re-confirmed status |
|---|---|
| PushVarbit / PopVarbit (25, 27) | Declared in TS at ScriptOpcode.ts:22-23 with comment `// official, see cs2`. No `handlers/*` case-label exists. NAI-161 §1 mis-classified these as "inline-bytecode core ops handled in ScriptRunner"; re-grep of TS engine ScriptRunner shows no special-case path. Treat as TS-unimplemented stubs. |
| SetGender, LcOp, OcOp, OcIop | Declared in TS ScriptOpcode.ts; verified absent from handlers/*. Stub-with-pin. |
| WealthEvent | TS body exists (PlayerOps.ts:1191-1202); B2 ports the handler + the new `(*Player).AddWealthEvent` method. Retires NAI-115-D1 in same bundle. |
| NpcStatHeal | TS body (NpcOps.ts:241-257) includes `npc.heroPoints.clear()` HP-full branch. NAI-161 deferral note ("needs HeroPoints.Clear()") confirmed; B1 lands `(*HeroPoints).Clear()`. |
| BothDropSlot, InvDropAll | TS bodies (InvOps.ts:672-723, 726-790) far more complex than handleInvDropSlot mirror — both call `addWealthEvent`; BothDropSlot has secondary-player swap semantics; INV_DROPALL builds a wealth-log map across slots. **Do not assume sibling-helper applies** (per `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings.md`); each handler is a fresh port. |
| MapIndoors | TS calls `isIndoors(x, z, level)`. No corresponding helper in goscape; B1 lands it. |
| LastLoginInfo, InvTotalParamStack | One-line TS delegations to `(*Player)` methods that don't yet exist; B1 lands both methods. |
| PLocMerge | TS calls `World.mergeLoc(...)` — goscape has `(*Zone).MergeLoc` (pkg/zone/zone.go:228) and `(*Server).MergeLoc` (modules/world/world_zone.go:113). Verified handler-callable. |
| POpPlayerT | TS sets `Interaction.SCRIPT` + `ServerTriggerType.APPLAYERT`. Goscape has `(*Player).SetInteractionScriptPlayer` (handlers_player.go:1409) — exists from prior cohort. Wire-up only. |

---

## §2 Architecture

### §2.1 Bundle B0 — Stub sweep (6 handlers)

**Pattern:** mirror NAI-161 T6 `handlePOpHeld` shape. Per stub:

```go
// handlePushVarbit is a TS-unimplemented stub (TS has no
// handlers/* case-label for opcode 25). Per NAI-162 §3
// deviation NAI-162-D-STUB-PUSHVARBIT, this stub returns an
// 'unimplemented' error rather than no-op so future TS sync
// re-ports the handler explicitly.
//
// Mirrors absent TS handler at
// Engine-TS/src/engine/script/handlers/* (no entry).
func handlePushVarbit(s *ScriptState) error {
	return fmt.Errorf("PUSH_VARBIT: unimplemented")
}
```

Six handlers, six entries in the dispatch map (`pkg/script/handlers.go`):
- `OpPushVarbit (25)` → `handlePushVarbit`
- `OpPopVarbit (27)` → `handlePopVarbit`
- `OpSetGender (2099)` → `handleSetGender`
- `OpLcOp (4105)` → `handleLcOp`
- `OpOcIop (4205)` → `handleOcIop`
- `OpOcOp (4208)` → `handleOcOp`

No new infra; no `ActivePlayer` widening; no `mockPlayer` changes. Behavior tests: a single `TestNAI162B0StubsReturnUnimplemented` table-driven test asserts each opcode returns an error containing `"unimplemented"`. No `Pointers` / `Self` setup needed beyond minimal `ScriptState{}`.

**Close commit B0:** audit recount 18 → 12.

---

### §2.2 Bundle B1 — Trivial + small (4 handlers + 4 new methods)

#### §2.2.1 New `(*HeroPoints).Clear()` (`modules/world/heropoints.go`)

NPC_STATHEAL's HP-full branch (TS NpcOps.ts:254-256) clears every contributor entry. Goscape's `HeroPoints` is a map+slice ledger; `Clear()` resets it to zero contributors.

```go
// Clear zeroes the contributor ledger. Mirrors TS HeroPoints.clear()
// (NpcOps.ts:255 caller). Called when an NPC's HP fully heals to its
// base level — TS resets the credit attribution at that moment so
// subsequent damage starts fresh.
func (h *HeroPoints) Clear() {
	// Implementation: zero the contributor map + reset top-contributor
	// cache. Plan-author reads existing struct fields and matches the
	// reset shape used by HeroPoints construction.
}
```

Plan-author pre-flight per `plan_grep_helper_patterns.md`: grep `type HeroPoints struct` to confirm field names before codifying the Clear body.

#### §2.2.2 New `(*Player).LastLoginInfo()` (`modules/world/player_script.go`)

TS PlayerOps.ts:931-933 delegates to `state.activePlayer.lastLoginInfo()`. The TS Player.lastLoginInfo emits an outgoing packet with the player's previous login timestamp + IP. Goscape's analogue queues a server packet via the existing outgoing-packet pattern.

```go
// LastLoginInfo emits a LAST_LOGIN_INFO server packet to this player's
// client with the previous-login timestamp and IP. Mirrors TS
// Player.lastLoginInfo (PlayerOps.ts:932 caller).
//
// (goscape defensive; TS skips this check) Nil-client guard mirrors
// EnqueueScriptArgs at player_script.go:127.
func (p *Player) LastLoginInfo() {
	// Implementation: nil-guard p.client; write a LAST_LOGIN_INFO
	// packet via the outgoing-prot dispatcher with the session-stored
	// previous-login data.
}
```

Plan-author pre-flight: grep `pkg/io/protocol/server` for an existing `LAST_LOGIN_INFO` ServerProt entry. If absent: B1 also lands the prot stub (or the handler is admitted with a deviation `NAI-162-D-LASTLOGIN-NO-PACKET` and skipped to no-op — plan-author decides based on prot availability).

#### §2.2.3 New `(*Player).InvTotalParamStack(invID, paramID int) int` (`modules/world/player_script.go`)

TS PlayerOps.ts:792-796 delegates to `state.activePlayer.invTotalParamStack(inv, param)`. The TS body sums `slot.count * param-value` across non-empty slots.

```go
// InvTotalParamStack sums (slot.count * objType.Param(paramID))
// across every non-empty slot of the named inventory. Param values
// resolve via the existing ObjType.Param accessor; missing param
// contributes zero. Mirrors TS Player.invTotalParamStack
// (InvOps.ts:795 caller).
func (p *Player) InvTotalParamStack(invID, paramID int) int {
	// Implementation: nil-guard p.client; resolve inventory by id;
	// iterate slots; for each non-empty obj: type-lookup → param
	// resolve → multiply by count → accumulate.
}
```

Plan-author pre-flight per `audit_full_method_against_ts.md`: read the full TS body in `Player.ts` (search `invTotalParamStack`) to confirm exact filter semantics (any stackable filter? scope filter?).

#### §2.2.4 New `pkg/collision` predicate `IsIndoors(x, z, level int) bool`

TS ServerOps.ts:142 calls `isIndoors(coord.x, coord.z, coord.level)`. The TS helper reads a roof-flag bit from the map data.

```go
// IsIndoors reports whether the tile at (x, z, level) carries the
// indoor/roof flag. Mirrors TS isIndoors (ServerOps.ts:142 caller).
func IsIndoors(x, z, level int) bool {
	// Implementation: resolve the tile's collision bitmask; return
	// (flag & FlagRoof) != 0. Plan-author confirms the bit constant
	// against TS isIndoors implementation (canonical path:
	// src/engine/util/IsIndoors.ts — read at plan-time).
}
```

Plan-author pre-flight per `spec_ts_source_read.md`: read TS `isIndoors` source verbatim before codifying. The flag-bit constant must match.

#### §2.2.5 Four B1 handlers

```go
// handleLastLoginInfo (LAST_LOGIN_INFO, opcode 2054). Mirrors TS
// PlayerOps.ts:931-933 — single delegation to (*Player).LastLoginInfo.
// No popInt; no gate beyond requireActivePlayer.
func handleLastLoginInfo(s *ScriptState) error { ... }

// handleInvTotalParamStack (INV_TOTALPARAM_STACK, opcode 4329).
// Mirrors TS InvOps.ts:792-796 — popInts(2) → delegate → pushInt.
func handleInvTotalParamStack(s *ScriptState) error { ... }

// handleMapIndoors (MAP_INDOORS, opcode 1010). Mirrors TS
// ServerOps.ts:139-143 — popInt → CoordValid check → IsIndoors → pushInt.
func handleMapIndoors(s *ScriptState) error { ... }

// handleNpcStatHeal (NPC_STATHEAL, opcode 2539). Mirrors TS
// NpcOps.ts:241-257. Pop order matches TS popInts(3) destructuring:
// [stat, constant, percent].  Arithmetic:
//   healed = current + ((constant + (base * percent) / 100) | 0)
//   npc.levels[stat] = min(healed, base)
//   if stat == NpcStatHitpoints && npc.levels[stat] >= base:
//       npc.heroPoints.Clear()
// The `| 0` TS truncation maps to Go integer division (no cast needed).
func handleNpcStatHeal(s *ScriptState) error { ... }
```

**Close commit B1:** audit recount 12 → 8.

---

### §2.3 Bundle B2 — WealthEvent + player-interaction + NAI-115-D1 retirement (3 handlers + 1 new method + retirement)

#### §2.3.1 New `(*Player).AddWealthEvent(evt WealthEvent)` (`modules/world/player_script.go`)

TS PlayerOps.ts:1197-1201 calls `state.activePlayer.addWealthEvent({event_type, account_items, account_value, recipient_session?})`. Per design §2 confirmation, the goscape impl is **in-memory log only** (no analytics sink); deferring real RPC integration.

```go
// WealthEvent captures a single wealth-affecting event for analytics.
// Defined in package modules/world (or modules/world/wealth.go).
type WealthEvent struct {
	EventType        int            // WealthEventType enum
	AccountItems     []WealthItem
	AccountValue     int
	RecipientSession string         // optional (BothDropSlot PVP path)
}

type WealthItem struct {
	ID    int
	Name  string
	Count int
}

// AddWealthEvent appends the event to this player's in-memory wealth
// log. Mirrors TS Player.addWealthEvent (PlayerOps.ts:1197 caller, plus
// inv-drop callers at InvOps.ts:695-700 and 781-789).
//
// (goscape deviation; TS emits an analytics RPC) Analytics sink
// deferred — see NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY.
func (p *Player) AddWealthEvent(evt WealthEvent) {
	// Implementation: append to p.wealthLog ([]WealthEvent field on
	// Player). Plan-author confirms the field name & adds it to
	// Player struct if absent.
}
```

WealthEventType enum constants (`WealthEventTypePVP`, `WealthEventTypeDeath`, etc.) — plan-author enumerates by grep on TS `WealthEventType` definition.

#### §2.3.2 NAI-115-D1 retirement

Per memory `retire_deviation_grep_all_comments.md`: plan-author runs `rg "NAI-115-D1" pkg/ modules/ cmd/` at plan-write to enumerate every site. Known sites at brainstorm time:

* `pkg/script/handlers_inv.go:776, 784-786, 852-853, 1245-1247` — 4 deviation comment blocks
* `pkg/script/handlers_obj.go` — sites count to be confirmed by plan-author grep

Retirement = delete the deviation comment block + flip the SCOPE_PERM branch from skip-AddWealthEvent to call-AddWealthEvent inline. Mirrors TS InvOps.ts inline addWealthEvent in INV_DROPSLOT. Behaviour delta: SCOPE_PERM drops via INV_DROPSLOT now emit wealth events.

#### §2.3.3 Three B2 handlers

```go
// handleWealthEvent (WEALTH_EVENT, opcode 2131). Mirrors TS
// PlayerOps.ts:1191-1202. Pop order: popString(name), popInts(3)
// → [eventType, count, value]. ObjType.GetByName resolves; missing
// name maps to objID = -1 (matching TS `objType?.id` undefined→null).
// Calls (*Player).AddWealthEvent.
func handleWealthEvent(s *ScriptState) error { ... }

// handlePLocMerge (P_LOCMERGE, opcode 2074). Mirrors TS
// PlayerOps.ts:922-929. requireProtectedActivePlayer gate. popInts(4):
// [startCycle, endCycle, southEast, northWest]. Resolve coords via
// CoordValid. Call (*Server).MergeLoc with TS arg order
// (level, activeLoc, activePlayer, startCycle, endCycle, se.z, se.x, nw.z, nw.x).
func handlePLocMerge(s *ScriptState) error { ... }

// handlePOpPlayerT (P_OPPLAYERT, opcode 2082). Mirrors TS
// PlayerOps.ts:1127-1135. requireProtectedActivePlayer gate. popInt
// → spellId. Self2 (state._activePlayer2) nil-check → return early
// (silent — matches TS line 1130-1132). Then:
//   self.StopAction()
//   self.SetInteractionScriptPlayer(self2, spellId)
// Wire-up only — no new method.
func handlePOpPlayerT(s *ScriptState) error { ... }
```

**Close commit B2:** audit recount 8 → 5; cite NAI-115-D1 retirement.

---

### §2.4 Bundle B3 — Inv drops (2 handlers)

These two are the most complex in the sweep. Each is a fresh port from TS (do **not** template off handleInvDropSlot — TS bodies diverge significantly).

#### §2.4.1 `handleBothDropSlot` (BOTH_DROPSLOT, opcode 4300)

Faithful port of TS InvOps.ts:672-723. Sequence (TS-line preserved):
1. popInts(4) → `[invID, coord, slot, duration]`
2. Validate: InvTypeValid, DurationValid, CoordValid
3. `secondary := s.IntOperand == 1` — per memory `parallel_slice_convention_for_mixed_type_args.md` semantic doesn't apply; this is a single int operand carried on the bytecode
4. from/to player swap: `fromPlayer, toPlayer = secondary ? (Self2, Self) : (Self, Self2)`
5. Nil-check both → error `"BOTH_DROPSLOT: player is null"`
6. Protect gate: `requireProtectedActivePlayer` if `!secondary` else `requireProtectedActivePlayer2`. Skip when `invType.protect == false` or `invType.scope == ScopeShared`
7. Look up slot obj on fromPlayer → if empty, error
8. SCOPE_PERM: `Self.AddWealthEvent(WealthEvent{EventType: PVP, AccountItems: [{...}], AccountValue: count*cost, RecipientSession: toPlayer.session})`
9. `fromPlayer.InvDel(invID, objID, count, slot)` → `completed`
10. If `completed == 0`: return
11. Construct dropObj at coord
12. Untradeable: `World.AddObj(dropObj, fromPlayer.Hash64, duration)`; else: `World.AddObj(dropObj, toPlayer.Hash64, duration)`

Plan-author pre-flight per `plan_sibling_site_guard_audit.md`: grep `requireProtectedActivePlayer2` and `Self2` for existing-caller patterns; mirror their nil-check shape.

#### §2.4.2 `handleInvDropAll` (INV_DROPALL, opcode 4309)

Faithful port of TS InvOps.ts:726-790. Sequence:
1. popInts(3) → `[invID, coord, duration]`
2. Validate: InvTypeValid, DurationValid, CoordValid
3. Protect gate per `intOperand`-indexed slot
4. Resolve inventory; nil → return
5. Walk every slot; per non-empty:
   * If SCOPE_PERM: accumulate into wealth-log map keyed by objID; sum `totalValue += count * cost`
   * Delete from inventory
   * Construct dropObj; untradeable → addObj to self; else → addObj to `Obj.NO_RECEIVER`
6. After loop: if wealth-log non-empty, `Self.AddWealthEvent(WealthEvent{EventType: Death, AccountItems: <log values>, AccountValue: totalValue})`

Plan-author pre-flight per `audit_full_method_against_ts.md`: read full body line-by-line; the wealth-log accumulation pattern (Map keyed by objID with running count) is easy to miss summarising from neighbours.

**Close commit B3:** audit recount 5 → 3; final NAI-162 roll-up.

---

## §3 Deviation register

| Tag | Site | Deviation | Rationale |
|---|---|---|---|
| `NAI-162-D-STUB-PUSHVARBIT` | `handlePushVarbit` | TS has no handler; goscape stub returns `fmt.Errorf("PUSH_VARBIT: unimplemented")`. | TS-faithful at semantic level (both raise at this opcode). Mirrors NAI-161 `NAI-161-D-POPHELD-STUB` shape. Re-port when TS lands a real body. |
| `NAI-162-D-STUB-POPVARBIT` | `handlePopVarbit` | Same as above for opcode 27. | Same. |
| `NAI-162-D-STUB-SETGENDER` | `handleSetGender` | Same as above for opcode 2099. | Same. |
| `NAI-162-D-STUB-LCOP` | `handleLcOp` | Same as above for opcode 4105. | TS-PARITY STUB (final). **The "future OPHELD trigger-plumbing cohort" referenced previously does not exist in TS** — see memory `nai164_declined_cohort.md` (2026-05-11 NAI-164-declined audit). Re-port only if upstream TS lands a real body. |
| `NAI-162-D-STUB-OCIOP` | `handleOcIop` | Same as above for opcode 4205. | Same. |
| `NAI-162-D-STUB-OCOP` | `handleOcOp` | Same as above for opcode 4208. | Same. |
| `NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY` | `(*Player).AddWealthEvent` | TS emits an analytics RPC payload. Goscape appends to in-memory `p.wealthLog` only; no analytics sink wired. | User-confirmed at brainstorm: defer analytics. The retirement of NAI-115-D1 is unblocked by in-memory append; consumer (analytics) is orthogonal. Follow-up: NAI-N analytics-sink port (un-tracked at this commit; create when an analytics consumer is wired). |
| `NAI-162-D-LASTLOGIN-NO-PACKET` (conditional) | `(*Player).LastLoginInfo` | If `LAST_LOGIN_INFO` ServerProt is absent at plan-write, the method writes nothing (silent no-op). | Plan-author decides at grep. Tracked as a deviation IFF the prot is absent. |
| `NAI-115-D1` | `handlers_inv.go` + `handlers_obj.go` SCOPE_PERM paths | **Retired** in B2. Deviation comment blocks deleted; SCOPE_PERM drop path now calls `Self.AddWealthEvent` inline (TS-faithful). | Retirement is the goal of B2 + B3 together: B2 lands the method, B3 (and the in-bundle inv-drop callers it depends on) drive it from the new BOTH_DROPSLOT / INV_DROPALL handlers, AND from the existing INV_DROPSLOT path being un-skipped at retirement time. |

---

## §4 Risk register

| Tag | Risk | Likelihood | Mitigation |
|---|---|---|---|
| `R1 [high]` | NAI-115-D1 retirement flips INV_DROPSLOT behavior (SCOPE_PERM drops start emitting wealth events). Existing INV_DROPSLOT tests may assert "no event emitted" — those become regressions. | Med | Plan-author runs `rg "wealthEvent\|WealthEvent\|wealth_event" pkg/script/handlers_inv_test.go pkg/script/handlers_obj_test.go` at plan-write; lists every test that asserts (no-)emission and updates expectations as part of the retirement task. |
| `R2 [med]` | `(*HeroPoints).Clear()` semantics: NAI-161 spec §1 cited "HeroPoints.Clear()" but the actual TS method shape may not be a bare reset — could also unwind credit history. | Med | Plan-author reads `npc.heroPoints.clear()` callers across TS to confirm shape. If shape is anything beyond "zero contributor map", deviation `NAI-162-D-HEROPOINTS-CLEAR-SCOPE` is tracked. |
| `R3 [med]` | BOTH_DROPSLOT secondary-swap protect-gate indexing — `ProtectedActivePlayer[secondary ? 1 : 0]` must map to goscape's `requireProtectedActivePlayer` vs `requireProtectedActivePlayer2` correctly. Inverted mapping silently breaks PVP drops in either direction. | Low | Test §5 §5.4 #1 pins both `secondary=0` AND `secondary=1` paths with their respective slot-protect requirements; failure mode is explicit error mismatch. |
| `R4 [med]` | `isIndoors` flag-bit constant divergence. TS reads a specific bit; if goscape's flagmap uses a different bit layout, MAP_INDOORS reports the wrong answer silently (no error, just wrong int). | Med | Plan-author reads TS canonical `isIndoors` source verbatim per `spec_ts_source_read.md` — the bit constant MUST match. Test §5 §5.2 #3 pins both true and false on known-indoor / known-outdoor coords. |
| `R5 [low]` | TS-unimplemented stubs (B0) become divergence if TS lands real bodies between brainstorm and plan-write. | Low | Plan-author re-runs the "MISSING-IN-TS" check from this brainstorm (per-op `rg "ScriptOpcode\.<NAME>\]" --type ts src/engine/script/handlers/`) before codifying B0. |
| `R6 [low]` | `LAST_LOGIN_INFO` ServerProt absence forces deviation `NAI-162-D-LASTLOGIN-NO-PACKET`. If the prot exists but with a different field shape, the handler silently writes garbage. | Low | Plan-author greps `pkg/io/protocol/server` for `LastLoginInfo` / `LAST_LOGIN_INFO` byte-level definition before codifying the method. |
| `R7 [low]` | `mockPlayer` interface gap — B1 adds `LastLoginInfo`, `InvTotalParamStack`; B2 adds `AddWealthEvent`; if `ActivePlayer` interface is widened, every consumer mock must add the methods. | Low | Plan task per bundle adds the mock methods. Per `mock_recorder_field_naming_check.md`, plan-author greps the actual `mockPlayer` struct and matches the dominant recorder convention. |
| `R8 [low]` | INV_DROPALL wealth-log Map keying — TS uses `Map<number, ...>` keyed by objID. Go-side, naive use of `map[int]WealthItem` mutates only the copied struct, not the map entry. | Low | Plan-author uses `map[int]*wealthLogEntry` pointer values, OR direct re-assign pattern `m[k] = updated`. Test §5 §5.4 #2 pins the "stack of identical objIDs accumulates count" path. |
| `R9 [low]` | Cascade-tail leakage: B1 NPC_STATHEAL's HP-full branch will start calling `(*HeroPoints).Clear()`. If any content script relies on stale HeroPoints state across heals (unlikely), behavior changes. | Low | Smoke per §6 covers a content-pinned NPC heal path if available; otherwise unit-pinned. |
| `R10 [low]` | NAI-115-D1 site count under-enumerated. Grep at brainstorm hit `pkg/script/handlers_inv.go` and `pkg/script/handlers_obj.go`; further sites may exist in tests or other handler files. | Low | Plan-author re-runs `rg "NAI-115-D1" pkg/ modules/ cmd/` at plan-write and enumerates every site in the retirement task. |

---

## §5 Test strategy

Per memory `scriptstate_test_fixture_idioms.md`: each fixture initialises `StackCapacity`, pushes args in TS pop-order, sets `Pointers` per-handler.

### §5.1 B0 — stub sweep (one table-driven test)

`TestNAI162B0StubsReturnUnimplemented` (`pkg/script/handlers_test.go` or `_b0_test.go`):

* Table: `[]struct{ name string; op Opcode; want string }` covering all 6 ops + their `"unimplemented"` substring.
* Per row: build minimal `ScriptState` with the single opcode + `OpReturn`; `Execute` → assert returned error contains the substring.
* No `Pointers` setup; no `Self`. Stubs return before any check.

### §5.2 B1 — trivial + small

Handler tests in `pkg/script/handlers_player_test.go` / `handlers_npc_test.go` / new `handlers_server_test.go`:

1. **TestHandleLastLoginInfo**: `mockPlayer.lastLoginInfoCalls` recorder asserts single call. No-active-player branch folds into `TestHandlersRequireActivePlayer` table.
2. **TestHandleInvTotalParamStack**: mock returns `42` for `(inv=5, param=7)` → assert pushed int == 42. No-active-player folds into table.
3. **TestHandleMapIndoors_True**, **TestHandleMapIndoors_False**: build coord at known indoor vs outdoor tile (test fixture: stub `collision.IsIndoors` via package-level var seam OR rely on a hand-set flagmap fixture). Assert pushed int 1 vs 0.
4. **TestHandleMapIndoors_InvalidCoord**: coord fails `CoordValid` → error.
5. **TestHandleNpcStatHeal_PartialHeal**: stat=Attack, base=10, current=2, constant=3, percent=50 → healed = 2 + (3 + 10*50/100) = 2 + 8 = 10. Assert `npc.levels[stat] == 10`. (Capped at base.)
6. **TestHandleNpcStatHeal_HpFullClearsHeroPoints**: stat=Hitpoints, base=20, current=18, constant=5, percent=50 → healed = 18 + (5 + 20*50/100) = 33 → min(33, 20) = 20. Assert `npc.levels[Hitpoints] == 20` AND `mockNpc.heroPointsClearCalls == 1`.
7. **TestHandleNpcStatHeal_HpHealsButNotFull**: stat=Hitpoints, base=20, current=10, constant=2, percent=10 → healed = 10 + (2 + 2) = 14. Assert `heroPointsClearCalls == 0` (gate only fires when HP at base).
8. **TestHandleNpcStatHeal_NonHpStatNeverClears**: stat=Attack, base=10, current=5, fully heals to 10. Assert `heroPointsClearCalls == 0`.

Unit tests for new methods (`modules/world/heropoints_test.go`, `player_script_test.go`, `pkg/collision/indoors_test.go`):

* `(*HeroPoints).Clear()`: add 2 entries → Clear → assert 0 entries, top-contributor cache cleared.
* `(*Player).LastLoginInfo()`: assert outgoing packet queued (or no-op if prot absent — deviation path).
* `(*Player).InvTotalParamStack`: mixed-slot inv, some non-empty, some empty; assert sum matches.
* `collision.IsIndoors`: known indoor coord → true; known outdoor coord → false.

### §5.3 B2 — WealthEvent + interaction + retirement

1. **TestHandleWealthEvent**: push string `"abyssal_whip"`, push 3 ints [eventType=Death, count=1, value=120000]. Mock `ObjType.GetByName("abyssal_whip")` returns id=4151. `handleWealthEvent` → assert `mockPlayer.addWealthEventCalls[0] == WealthEvent{EventType: Death, AccountItems: [{ID:4151, Name:"abyssal_whip", Count:1}], AccountValue: 120000}`.
2. **TestHandleWealthEvent_UnknownObj**: `GetByName` returns nil → assert AccountItems[0].ID == -1 (matching TS `objType?.id` undefined).
3. **TestHandlePLocMerge_Happy**: setup ActiveLoc + Self; popInts(4); assert `mockServer.mergeLocCalls[0]` matches arg order.
4. **TestHandlePLocMerge_InvalidCoord**: bad coord → error.
5. **TestHandlePLocMerge_NotProtected**: omit `PtrProtectedActivePlayer` → folds into `TestHandlersRequireProtectedActivePlayer` table.
6. **TestHandlePOpPlayerT_Happy**: Self + Self2 set; popInt → spellId. Assert `mockPlayer.stopActionCalls == 1` AND `mockPlayer.setInteractionScriptPlayerCalls[0] == {target: self2, op: spellId}`.
7. **TestHandlePOpPlayerT_NilSelf2**: Self2 unset → handler returns nil (silent), assert no `stopAction` call, no interaction call. Mirrors TS line 1130-1132 silent return.
8. **TestHandlePOpPlayerT_NotProtected**: folds into `TestHandlersRequireProtectedActivePlayer` table.
9. **TestNAI115D1Retirement_InvDropSlotEmitsWealthEvent**: build INV_DROPSLOT against an inv with `Scope=ScopePerm`; pre-NAI-162 expectation was `addWealthEventCalls == 0`; post-retirement expectation is `addWealthEventCalls == 1` with `EventType=Drop` (TS event_type for INV_DROPSLOT SCOPE_PERM — plan-author confirms from TS). Per R1, this test replaces the pre-retirement no-emission assertion. Plan-author updates all existing tests asserting no-emission accordingly.

### §5.4 B3 — inv drops

1. **TestHandleBothDropSlot_PrimaryFromSelf_NonProtected**: secondary=0, inv.protect=false → succeeds without protect-gate. Drop obj to fromPlayer (Self). Assert `mockServer.addObjCalls[0]` carries `Self.hash64`.
2. **TestHandleBothDropSlot_PrimaryFromSelf_Protected_HasProtect**: secondary=0, inv.protect=true, scope=ScopeNormal, Pointers has `PtrProtectedActivePlayer`. Succeeds.
3. **TestHandleBothDropSlot_PrimaryFromSelf_Protected_NoProtect**: same but Pointers lacks the flag. Error path.
4. **TestHandleBothDropSlot_SecondaryFromSelf2**: secondary=1, Self2 holds the inv. Drop goes via Self2's inv; addObj receiver = Self2.hash64 (untradeable). Pins the swap.
5. **TestHandleBothDropSlot_SecondaryProtectViaSlot1**: secondary=1, Pointers needs `PtrProtectedActivePlayer2` (NOT PtrProtectedActivePlayer). Pins R3.
6. **TestHandleBothDropSlot_ScopePerm_EmitsPVPWealthEvent**: secondary=0, inv.scope=ScopePerm, obj at slot=3 with count=5, cost=1000. Assert `Self.addWealthEventCalls[0]` carries `EventType=PVP, AccountItems=[{...}], AccountValue=5000, RecipientSession=Self2.session`.
7. **TestHandleBothDropSlot_TradeableGoesToReceiver**: secondary=0, obj.tradeable=true → addObj receiver = Self2.hash64. Untradeable → receiver = Self.hash64.
8. **TestHandleBothDropSlot_NullPlayer**: Self2 unset, secondary=0 still has Self2 as toPlayer (per TS swap mapping) → error.
9. **TestHandleBothDropSlot_EmptySlot**: slot lookup returns nil → error.
10. **TestHandleBothDropSlot_InvDelZero**: `InvDel` returns 0 (slot already vacated mid-handler) → return silently, no addObj call.
11. **TestHandleInvDropAll_EmptyInv**: zero non-empty slots → no calls.
12. **TestHandleInvDropAll_MixedSlots**: 3 non-empty slots (objIDs 10, 20, 10), all SCOPE_NORMAL → 3 addObj calls; no wealth event.
13. **TestHandleInvDropAll_ScopePerm_AccumulatesWealthLog**: 3 non-empty slots, two with same objID. SCOPE_PERM. Assert `addWealthEventCalls[0].AccountItems` has 2 entries (one accumulated count); `AccountValue` = sum across all 3 slots.
14. **TestHandleInvDropAll_TradeableSplit**: tradeable obj → addObj receiver = `Obj.NO_RECEIVER` constant; untradeable → receiver = Self.hash64.

---

## §6 Smoke binding

**Bundle-level smoke matrix:**

| Bundle | Smoke required? | Target |
|---|---|---|
| B0 | No | All 6 stubs return errors; unit-pinned. |
| B1 | Optional | Content-pinned MAP_INDOORS (e.g., any indoor/outdoor switch script) and NPC_STATHEAL (any NPC self-heal script). Plan-author greps `LostCity/Content/scripts/**/*.rs2` for `map_indoors` and `npc_stat_heal` callers. |
| B2 | **Required** | NAI-115-D1 retirement smoke — drop a SCOPE_PERM item via INV_DROPSLOT (the previously-skipped path now fires AddWealthEvent). User-launched server per `smoke_test_server_handoff.md`; we grep server logs for "wealth event" or the in-memory log via NodeDebug-style probe per `nodedebug_gateway_probe_pattern.md`. |
| B3 | **Required** | BOTH_DROPSLOT and/or INV_DROPALL via a content path that exercises one of them (e.g., a PVP drop or full-inv-clear script). Plan-author identifies the content pin; if none reachable, fall back to: smoke a PVP-drop scenario by manual server launch + Java-client trade-drop interaction. |

Per `cascade_theory_smoke_binding.md`, the NAI-115-D1 retirement signal (wealth event now fires on SCOPE_PERM drop) is the binding cascade-attribution signal for B2. B3's binding signal is the absence of the `no handler for BOTH_DROPSLOT (4300)` / `INV_DROPALL (4309)` WARN under the content-pin smoke.

---

## §7 Cadence routing

Standard cadence per `runescript_cadence.md` with per-bundle close commits:

1. **Spec** (this doc) → user review gate.
2. **Plan** (writing-plans skill, one doc covering all 4 bundles; per-bundle task headings). Plan-author per memory `controller_preflight.md` re-runs the missing-handler audit + per-op MISSING-IN-TS check at plan-write time.
3. **`/clear`** between plan and impl per `superpowers_clear_between_spec_and_impl.md`.
4. **Implementation** (subagent-driven-development per `execution_mode_default.md`), executed bundle-by-bundle:
   * **B0 impl + close commit** (audit 18 → 12)
   * **B1 impl + close commit** (audit 12 → 8)
   * **B2 impl + close commit** (audit 8 → 5; NAI-115-D1 retired)
   * **B3 impl + close commit** (audit 5 → 3)
5. **Single combined Sonnet** `superpowers:code-reviewer` per bundle close per `superpowers_code_reviewer_model.md`. Sonnet, not Opus.
6. **Smoke handoffs**: B2 binding smoke (NAI-115-D1 retirement); B3 binding smoke (inv-drops content pin). User-launched per `smoke_test_server_handoff.md`.
7. **Final NAI-162 close commit** after B3 — rolls up all 15 ops, cites audit recount 18 → 3, hands off the 3 deferred ops to NAI-163 brainstorm.

---

## §8 Out of scope

* **NAI-163 deferred ops:** LineOfSight 1005 (raycast subsystem), NpcAdd 2500 (entity-create path + `getNextNid`), NpcHunt 2525 (closest-NPC scan + HuntVis enum). Each is a discrete sub-spec.
* **Analytics sink for `(*Player).AddWealthEvent`** — in-memory log only; analytics RPC integration deferred to a future NAI-N (no tracker entry created at this commit).
* **`LAST_LOGIN_INFO` ServerProt definition** — if absent at plan-write, the prot port is its own sub-spec; B1 lands with deviation `NAI-162-D-LASTLOGIN-NO-PACKET`.
* **OPHELD trigger plumbing** — NAI-161 forward-route still pending; pairs with the eventual NAI-N OcOp/LcOp/OcIop content trigger cohort.
* **Re-implementing PUSH_VARBIT / POP_VARBIT as inline bytecode ops** — NAI-161 spec §1 hypothesized these were ScriptRunner inline ops; this spec confirms TS has no special-case handler, so they're plain `handlers/*` entries (stubs at B0).
* **`mockPlayer.wealthLog` for unit-test consumers in `modules/world`** — orthogonal; tests at `pkg/script` use the recorder mock.

---

## §9 Memory hits

Cited in close-commit `Closes memory:` trailer per `close_commit_memory_trailer.md`:

* `runescript_cadence.md` — full cadence routing
* `controller_preflight.md` — preflight audit before brainstorm; per-bundle preflight before each implementer dispatch
* `missing_handler_audit.md` — 18-opcode cascade-tail measurement; per-bundle recount
* `execution_mode_default.md` — subagent-driven-development
* `superpowers_clear_between_spec_and_impl.md` — `/clear` between plan and impl
* `superpowers_code_reviewer_model.md` — Sonnet per bundle close
* `spec_iteration_scope_audit.md` — full TS handler-body re-read at brainstorm (NPC_STATHEAL `heroPoints.clear()` branch; BOTH_DROPSLOT swap + AddWealthEvent; INV_DROPALL wealth-log accumulation)
* `audit_full_method_against_ts.md` — every TS body audited line-by-line; especially BOTH_DROPSLOT and INV_DROPALL
* `spec_ts_source_read.md` — every TS body read verbatim (no analogy)
* `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings.md` — B3 inv-drops are NOT templated off handleInvDropSlot
* `retire_deviation_grep_all_comments.md` — NAI-115-D1 enumerated at plan-write across `pkg/`, `modules/`, `cmd/`
* `true_to_ts_gate.md` — every deviation tracked; stub-with-pin pattern + AddWealthEvent in-memory-only
* `defensive_gate_doc_comment_label.md` — nil-client guards labeled "(goscape defensive; TS skips this check)"
* `mock_recorder_field_naming_check.md` — mock fields grep-verified at plan-write
* `plan_grep_helper_patterns.md` — `requireProtectedActivePlayer` / `requireProtectedActivePlayer2` / `requireActivePlayer` reuse
* `plan_sibling_site_guard_audit.md` — Self2 + secondary-swap callers audited
* `scriptstate_test_fixture_idioms.md` — fixture builders match TS pop-order
* `cascade_theory_smoke_binding.md` — B2 retirement-emit and B3 no-WARN as binding cascade signals
* `smoke_test_server_handoff.md` — B2, B3 smokes user-launched
* `nodedebug_gateway_probe_pattern.md` — B2 smoke uses in-memory wealth-log gateway probe
* `enumerate_all_sites.md` — handler registration sites enumerated (single `handlers` map; 15 lines across 4 bundles)
* `close_commit_memory_trailer.md` — per-bundle + final close commits cite memory

---

## §10 No-deviations audit

Per `spec_ts_init_value_audit.md`, `spec_diagram_order_divergence.md`, and `spec_iteration_scope_audit.md`, this spec was authored from TS source line reads:

* **B0 stubs** (PUSH_VARBIT, POP_VARBIT, SET_GENDER, LC_OP, OC_IOP, OC_OP): TS ScriptOpcode.ts declarations read; absence of `handlers/*` case-label verified via per-op `rg "ScriptOpcode\.<NAME>\]" --type ts src/engine/script/handlers/` (all six MISSING). Stub-with-pin is TS-faithful by construction.
* **LAST_LOGIN_INFO** (PlayerOps.ts:931-933) — 3 lines, single delegation. No branches.
* **INV_TOTALPARAM_STACK** (InvOps.ts:792-796) — 4 lines, popInts(2) → delegate → pushInt.
* **MAP_INDOORS** (ServerOps.ts:139-143) — 4 lines, popInt → CoordValid → `isIndoors` → pushInt(1/0).
* **NPC_STATHEAL** (NpcOps.ts:241-257) — 17 lines including HP-full `heroPoints.clear()` branch. **HP-full branch caught at brainstorm** (would have been missed if NAI-161's deferral note was taken at face value without re-read).
* **WEALTH_EVENT** (PlayerOps.ts:1191-1202) — 12 lines, popString + popInts(3) + ObjType.getByName + AddWealthEvent.
* **P_LOCMERGE** (PlayerOps.ts:922-929) — 8 lines, requireProtectedActivePlayer + popInts(4) + 2 CoordValid + World.mergeLoc.
* **P_OPPLAYERT** (PlayerOps.ts:1127-1135) — 9 lines, requireProtectedActivePlayer + popInt + Self2 nil-check (silent return) + stopAction + setInteraction. Silent-return on nil-Self2 caught at brainstorm; test §5.3 #7 pins.
* **BOTH_DROPSLOT** (InvOps.ts:672-723) — 52 lines, full body read line-by-line. Secondary-swap semantics, SCOPE_PERM-PVP AddWealthEvent, tradeable/untradeable receiver split all pinned in §2.4.1 and §5.4.
* **INV_DROPALL** (InvOps.ts:726-790) — 65 lines, full body read line-by-line. Wealth-log Map accumulation pattern (R8) caught at brainstorm.

No init-value, diagram-order, or asymmetric-predicate divergences detected. The retirement of NAI-115-D1 in B2 is a TS-fidelity move (un-skip an originally-skipped path), not a new deviation.
