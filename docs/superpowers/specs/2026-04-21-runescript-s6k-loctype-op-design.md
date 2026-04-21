# S6k — LocType.Op + LOC_OP Script Opcode Design

> **Sub-spec context:** Eleventh sub-spec in the runescript-s* series. Natural follow-up to S6j (OPLOC routing) — closes two of S6j's documented deviations (S6j-D1 handler op-gate + S6j-D7 defaultOp message) by adding the `LocType.Op []string` config field + wiring the `handleLocOp` script opcode. Mirrors the NpcType.Op / handleNpcOp template verbatim.

> **TS-faithfulness gate:** User requires "true to TS." All behavioral claims cite TS line numbers in `/home/owner/Code/github.com/LostCityRS/Engine-TS`. No new deviations introduced.

> **Scope:** Approach 1 (full bundle — LocType.Op + handler gate + loc_op opcode + defaultOp message). ~150 LOC impl + ~200 LOC tests across 3 tasks.

## 1. Goal

Add `LocType.Op []string` so scripts and the OPLOC handler can read per-loc click-option names. Two observable improvements:
1. Handler gate becomes TS-faithful — clicks on locs whose `Op[N]` is unset get rejected at click time (UnsetMapFlag) rather than falling through to the silent-no-op branch on the next tick.
2. The scripted `loc_op(op)` read returns the op name string, matching what `npc_op(op)` does for NPCs.

Plus: the `defaultOp` fallback ("Nothing interesting happens.") fires when the player reaches a loc with a populated op but no registered trigger — the TS-native message for "I see the option but nothing's wired to it."

## 2. Architecture

Four coordinated changes across three package boundaries, no new module/package infrastructure needed:

```
pkg/objtype/loctype.go              →  add Op []string field + decoder cases 30-34
pkg/entity/loc.go                    →  add Loc.LocType() method (interface satisfaction)
pkg/script/active.go                 →  expand ActiveLoc interface with LocType() int
pkg/script/opcode.go                 →  register OpLocOp = 3014
pkg/script/handlers_loc.go           →  add requireActiveLoc + handleLocOp
pkg/script/handlers.go               →  wire OpLocOp → handleLocOp dispatch
modules/world/handler_oploc.go       →  restore S6j-D1 op-validation gate
modules/world/interaction_trigger.go →  defaultOp message in fireOpTriggerLoc no-script path
```

**Existing infrastructure already in place (no changes needed):**
- `Player.MessageGame(msg string)` — `modules/world/message_game.go:1-25`
- `NpcType.Op` decoder pattern — `pkg/objtype/npctype.go:124-132`
- `handleNpcOp` handler shape — template for `handleLocOp`
- `ScriptState.ActiveLoc` field (currently `interface{}`) — shipped S6j Task 1
- `Configs.LocType(id int) *LocType` — already on interface at `pkg/script/configs.go:11`
- `requireActiveNpc` gate pattern — template for `requireActiveLoc`
- `OpLocAdd..OpLocType = 3000..3013` range — `OpLocOp = 3014` is next free slot

## 3. File Map

| File | Action | Purpose |
|---|---|---|
| `pkg/objtype/loctype.go` | Modify | Add `Op []string` field + cache-decode cases 30-34 with lazy 5-slot init + `"hidden"` → `""` coercion |
| `pkg/objtype/loctype_test.go` | Modify | 3 decoder tests |
| `pkg/entity/loc.go` | Modify | Add `Loc.LocType() int` method (alias for `Type()`) satisfying `pkg/script.ActiveLoc` |
| `pkg/script/active.go` | Modify | Expand `ActiveLoc` from empty interface to `interface { LocType() int }` |
| `pkg/script/opcode.go` | Modify | Register `OpLocOp Opcode = 3014` |
| `pkg/script/handlers_loc.go` | Modify | Add `requireActiveLoc` + `handleLocOp`; refresh file-level comment |
| `pkg/script/handlers.go` | Modify | Wire `OpLocOp → handleLocOp` in dispatch table |
| `pkg/script/handlers_loc_test.go` | Create | 5 `handleLocOp` tests |
| `modules/world/handler_oploc.go` | Modify | Restore S6j-D1 op-validation gate; update deviation comment to "CLOSED in S6k" |
| `modules/world/handler_oploc_test.go` | Modify | 2 new gate tests + fixture LocType.Op extensions |
| `modules/world/interaction_trigger.go` | Modify | `p.MessageGame("Nothing interesting happens.")` before no-script clear in `fireOpTriggerLoc` |
| `modules/world/interaction_trigger_test.go` | Modify | Update `TestTryFireOpTriggerLocNoScript` to assert `MessageGame` call |

## 4. TS Reference Map

- **Cache decoder:** `src/cache/config/LocType.ts:152-157` — `code >= 30 && < 35` with lazy 5-slot init, `dat.gjstr()` read
- **Handler gate:** `src/network/game/client/handler/OpLocHandler.ts:38-42` — `locType.op[op-1] != null && != 'hidden'`
- **defaultOp:** `src/engine/entity/Player.ts:~1095` — `player.messageGame('Nothing interesting happens.')`
- **NpcType.Op (goscape template):** `pkg/objtype/npctype.go:124-132`
- **handleNpcHasOp (goscape closest template):** `pkg/script/handlers_npc.go:87` — the `NPC_HASOP` handler reads `Configs.NpcType(ActiveNpc.NpcType()).Op[op-1]`, returns bool. `handleLocOp` uses the same read path but pushes the string instead of a bool. No existing `handleNpcOp` (string-returning NPC opcode) in goscape — `handleLocOp` is the first of its kind in this codebase; a future sub-spec may add the NPC sibling.

## 5. Component Details

### 5.1 `LocType.Op []string` field + decoder

**Field addition** to the `LocType` struct:

```go
type LocType struct {
    ConfigType
    Category int
    Desc     string
    Width    int
    Length   int
    Op       []string  // NEW — 5 option names; nil until decoded
    Params   ParamMap
}
```

**Decoder extension** — append cases to the existing decode loop in `loctype.go` (the current loop handles codes 3, 14, 15, 61, 249, 250):

```go
case 30, 31, 32, 33, 34:
    if t.Op == nil {
        t.Op = make([]string, 5)
    }
    t.Op[code-30] = dat.GJStrLF()
    if t.Op[code-30] == "hidden" {
        t.Op[code-30] = ""
    }
```

TS: `src/cache/config/LocType.ts:152-157`. Same opcodes, same coercion, same 5-slot lazy init.

### 5.2 ActiveLoc interface expansion + Loc.LocType method

**Interface** (`pkg/script/active.go:303`):

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j) and LOC_FIND (future).
type ActiveLoc interface {
    LocType() int  // returns the LocType ID (from packed Info bitfield)
}
```

**Loc method** (`pkg/entity/loc.go`, after existing `Type()` method):

```go
// LocType returns the LocType ID for this loc. Satisfies the
// pkg/script.ActiveLoc interface. Alias for Type() with a
// less-ambiguous name when the loc is bound to script state.
func (l *Loc) LocType() int { return l.Type() }
```

**Backward compatibility:** The existing `state.ActiveLoc = loc` assignment in `fireOpTriggerLoc` (S6j) still compiles after the interface expands, because `*entity.Loc` gains `LocType()` in the same task. Building Tasks 2 and 3 in order keeps the build green at every commit.

### 5.3 handleLocOp script opcode

**Handler** (append to `pkg/script/handlers_loc.go`):

```go
// requireActiveLoc returns an error tagged with the opcode name if the
// script has no ActiveLoc bound. All LOC_* read handlers start with
// this check to mirror TS `checkedHandler(ActiveLoc, ...)`.
func requireActiveLoc(s *ScriptState, op string) error {
    if s.ActiveLoc == nil {
        return fmt.Errorf("%s: no ActiveLoc bound", op)
    }
    return nil
}

// handleLocOp pushes the ActiveLoc's Op[op-1] string. Pops the op
// index (1-5). Pushes empty string if:
//   - the Op slot is unset (nil Op slice, out of range, or "" entry)
//   - LocType is not loaded (Configs.LocType returns nil)
// Closest goscape sibling is handleNpcHasOp (handlers_npc.go:87)
// which uses the same Configs lookup path but returns bool; this
// handler pushes the string directly.
func handleLocOp(s *ScriptState) error {
    if err := requireActiveLoc(s, "LOC_OP"); err != nil {
        return err
    }
    op := s.PopInt()
    cfg := s.Configs.LocType(s.ActiveLoc.LocType())
    if cfg == nil || op < 1 || op > len(cfg.Op) {
        s.PushString("")
        return nil
    }
    s.PushString(cfg.Op[op-1])
    return nil
}
```

**Opcode registration** (`pkg/script/opcode.go:303`, after `OpLocType`):

```go
OpLocOp Opcode = 3014
```

**Dispatch wiring** (`pkg/script/handlers.go`):

```go
handlers[OpLocOp] = handleLocOp
```

### 5.4 Handler gate restore (closes S6j-D1)

**Current shape** in `modules/world/handler_oploc.go` (the lookup + LocType-exists check):

```go
if s.locTypes.Configs[locId] == nil {
    sendUnsetMapFlag(p)
    return nil
}
```

**New shape** (bind the pointer, add the op-validation gate):

```go
locType := s.locTypes.Configs[locId]
if locType == nil {
    sendUnsetMapFlag(p)
    return nil
}
// Per-op validation gate. TS OpLocHandler.ts:38-42 rejects clicks
// where locType.op is nil, too short, or the slot is empty. The
// decoder coerces "hidden" to "" before storage, so the runtime
// check is just `== ""`. Closes deviation S6j-D1 (deferred
// from OPLOC routing).
if len(locType.Op) < op || locType.Op[op-1] == "" {
    sendUnsetMapFlag(p)
    return nil
}
```

Also: update the big `DEVIATION (S6j-D1)` block in the function godoc (lines 18-23 of `handler_oploc.go`). Replace with a 1-line note: `// S6j-D1 closed in S6k: per-op validation gate restored below.`

### 5.5 defaultOp message (closes S6j-D7)

In `modules/world/interaction_trigger.go`'s `fireOpTriggerLoc`, the "no script found" branch currently reads:

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
    p.ClearInteraction()
    p.interactionFired = true
    return
}
```

**Insert `MessageGame` before the clear:**

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
    // defaultOp fallback. TS Player.ts:~1095 fires this message when
    // the player reaches contact range and no op-trigger is
    // registered for this loc. Closes deviation S6j-D7 (deferred
    // from OPLOC routing — message infra was already in place via
    // Player.MessageGame; S6j's "needs message infra" concern was
    // spurious).
    p.MessageGame("Nothing interesting happens.")
    p.ClearInteraction()
    p.interactionFired = true
    return
}
```

Timing note: `fireOpTriggerLoc` is gated upstream by `inOperableDistance` in `processInteraction` (S6j), so the message only fires when the player has reached the loc — TS-faithful.

## 6. Test Plan

### 6.1 `pkg/objtype/loctype_test.go` — decoder (3 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestLocTypeDecodeOpSingleEntry` | Decoder with opcode 30 only → `Op[0]` populated, `Op[1..4]` empty |
| 2 | `TestLocTypeDecodeOpAllFive` | Decoder with opcodes 30-34 → all 5 slots populated with distinct strings |
| 3 | `TestLocTypeDecodeOpHiddenCoercedToEmpty` | Decoder reads `"hidden"` at opcode 31 → `Op[1] == ""` after decode |

### 6.2 `pkg/script/handlers_loc_test.go` — handleLocOp (5 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestHandleLocOpRequiresActiveLoc` | `ActiveLoc == nil` → returns error tagged `"LOC_OP"` |
| 2 | `TestHandleLocOpOutOfRangeLow` | `op=0` on valid LocType → pushes `""` |
| 3 | `TestHandleLocOpOutOfRangeHigh` | `op=6` on valid LocType → pushes `""` |
| 4 | `TestHandleLocOpEmptySlot` | Valid op index but `LocType.Op[op-1] == ""` → pushes `""` |
| 5 | `TestHandleLocOpHappyPath` | `op=1`, LocType loaded with `Op[0] == "Chop"` → pushes `"Chop"` |

### 6.3 `modules/world/handler_oploc_test.go` — gate restore (2 new tests + fixture updates)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestHandleOpLocRejectsEmptyOpSlot` | Fixture LocType with `Op[0] == ""` → UnsetMapFlag, target stays nil |
| 2 | `TestHandleOpLocAcceptsPopulatedOpSlot` | Fixture LocType with `Op[0] == "Action"` → normal interaction state set |

**Fixture updates:** Existing tests (`TestHandleOpLoc1SetsInteraction`, `TestHandleOpLocCoordValidationBoundary`, `TestHandleOpLocAllFiveOpsRouteIndependently`, `TestHandleOpLocClearsExistingInteraction`) need their `makeOpLocFixture` LocType extended with `Op: []string{"op1", "op2", "op3", "op4", "op5"}` so they continue passing under the new gate. Update `makeOpLocFixture` once; all callers inherit.

### 6.4 `modules/world/interaction_trigger_test.go` — defaultOp update

Extend `TestTryFireOpTriggerLocNoScript` to drain the player's connection and assert a `MessageGame("Nothing interesting happens.")` packet was sent (in addition to the existing `target==nil` and `interactionFired==true` assertions).

### 6.5 Totals

**11 new tests + 4-5 fixture one-line updates.** Mirrors S6j coverage density proportional to scope.

## 7. Task Split

Three tasks, layer-isolated. Each commits cleanly; build green at every commit.

### Task 1 — `LocType.Op` field + decoder

**Pure additive in `pkg/objtype`. No consumer changes.**

- Files: `pkg/objtype/loctype.go`, `pkg/objtype/loctype_test.go`
- 3 decoder tests
- Commit: `feat(objtype): LocType.Op field + cache decoder (S6k-1)`

### Task 2 — `handleLocOp` + ActiveLoc interface + Loc.LocType method

**`pkg/entity` + `pkg/script` changes. Depends on Task 1 for the field being present to read.**

- Files: `pkg/entity/loc.go`, `pkg/script/active.go`, `pkg/script/opcode.go`, `pkg/script/handlers.go`, `pkg/script/handlers_loc.go`, `pkg/script/handlers_loc_test.go`
- ActiveLoc interface expansion is binary-compat because the only existing setter is `state.ActiveLoc = loc` and `*entity.Loc` gains `LocType()` in the same task
- 5 handleLocOp tests
- Commit: `feat(script): handleLocOp + ActiveLoc.LocType binding (S6k-2)`

### Task 3 — Handler gate restore + defaultOp message

**`modules/world` only. Depends on Task 1 for `LocType.Op` to check.**

- Files: `modules/world/handler_oploc.go`, `modules/world/handler_oploc_test.go`, `modules/world/interaction_trigger.go`, `modules/world/interaction_trigger_test.go`
- Gate restore + 2 new gate tests + fixture updates
- defaultOp MessageGame call + test update
- After this task: `[oploc<n>,<locType>]` with populated `Op` routes TS-faithfully; no-script locs emit defaultOp
- Commit: `feat(world): restore OPLOC op-gate + defaultOp (S6k-3)`

## 8. Deviations from TS — Summary

S6k closes **2 deviations from S6j** with zero new ones.

| ID | Status after S6k | Notes |
|---|---|---|
| **S6j-D1** | ✅ **CLOSED** — op-validation gate restored (§5.4) | Per-op `locType.op[N]` check faithful to `OpLocHandler.ts:38-42` |
| **S6j-D2** | Still open (APLOC fallback) | Future sub-spec: "approach-vs-operate range gating" |
| **S6j-D3** | Still open (`targetOp` stores 1-5) | No follow-up — pure storage convention |
| **S6j-D4** | Still open (`locStillValid` zone check) | Defensive addition; no follow-up needed |
| **S6j-D5** | Still open (OpLocT/OpLocU) | Future sub-spec |
| **S6j-D6** | Still open (apRange/focus) | Bundled with S6j-D2 |
| **S6j-D7** | ✅ **CLOSED** — defaultOp message on no-script path (§5.5) | Faithful to `Player.ts:~1095` |

## 9. Scope Estimate

- **Implementation:** ~150 LOC across 8 files
- **Tests:** ~200 LOC (11 new + fixture updates)
- **Commits:** 3 (one per task)
- **Build/test green:** at every commit
- **Ship gain:** script-readable `loc_op(op)`, TS-faithful click validation, defaultOp message

## 10. Out-of-Scope Reminders

These are explicitly NOT in S6k (each tracked in §8 as a future sub-spec):

- APLOC approach-range gating + `+7` APLOC→OPLOC fallback (S6j-D2/D6)
- OpLocT / OpLocU sibling opcodes (S6j-D5)
- `lc_op(locType, op)` free-function variant (our handler is ActiveLoc-binding only; a 2-arg config-scoped read can be added if needed)
- Other LOC_* script opcodes beyond `loc_op` — `loc_category`, `loc_name`, etc., already exist; this spec touches only `loc_op`
