# S7f — NPC_FIND Family Design (NPC_FIND / NPC_FINDCAT / NPC_FINDEXACT)

> **Sub-spec context:** Thirty-second runescript sub-spec; sixth of S7. Implements the **closest-single-NPC cluster** of the NPC_FIND family: `NPC_FIND` (2513), `NPC_FINDCAT` (2517), and `NPC_FINDEXACT` (2518). Unblocks `[proc,set_hint_newbie_basics_instructor]` which stalled at `pc=8` on `NPC_FIND` after S7e cleared ALLOWDESIGN. Also delivers the reusable script→world NPC-lookup bridge and three new validators (`checkCoord`, `checkNpcType`, `checkHuntVis`) that the rest of the NPC_FIND* family (iterator cluster, NPC_FINDHERO/UID) will consume in later sub-specs.

> **TS-faithfulness gate:** Matches `LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts` lines 336-367 (NPC_FIND), 369-400 (NPC_FINDCAT), and 94-112 (NPC_FINDEXACT); `ScriptOpcodePointers.ts:576-605`; `ScriptValidators.ts:102-141` for `CoordValid / NpcTypeValid / HuntVisValid`. Three tracked deviations — all filter-layer omissions that degrade gracefully (finds more NPCs, not fewer).

> **Scope:** Three opcodes; three validators; one new script-package interface (`NpcLookup`); one world-package implementation (`serverNpcLookup`); one pointer-slot helper (`setActiveNpcSlot`). ~700 LOC total across 8 files. Standard cadence per `execution_mode_default` memory: brainstorm (done) → spec (this doc) → plan (writing-plans next) → subagent-driven-development with two-stage review per task.

## 1. Goal

Unblock `[proc,set_hint_newbie_basics_instructor]` past `pc=8` by implementing the three closest-single-NPC opcodes. Each opcode:

1. Pops 2-4 ints depending on variant.
2. Validates every input via the appropriate validator.
3. Asks the world for a matching NPC via a new `NpcLookup` interface.
4. On hit: writes the returned NPC to `ActiveNpc` or `OtherActiveNpc` based on the opcode's `IntOperand`, sets the corresponding `PtrActiveNpc` / `PtrActiveNpc2` flag, and pushes 1.
5. On miss: pushes 0, leaves pointer state untouched (TS conditional pointer — set only if push=1).

Bundled here because they share the same shape, validators, bridge interface, and pointer-slot semantics — separating them would duplicate infrastructure across three sub-specs that review and test similarly.

Unblocks all three opcodes and delivers the bridge foundation for the NPC_FIND* iterator cluster (NPC_FINDALL/FINDALLANY/FINDALLZONE/FINDNEXT) and the direct-lookup cluster (NPC_FINDHERO/FINDUID) as independent future sub-specs.

## 2. TS reference

### 2.1 Handlers

- **NPC_FIND** — `src/engine/script/handlers/NpcOps.ts:336-367`:
    ```ts
    [ScriptOpcode.NPC_FIND]: state => {
        const [coord, npc, distance, checkVis] = state.popInts(4);
        const position: CoordGrid = check(coord, CoordValid);
        const npcType: NpcType = check(npc, NpcTypeValid);
        check(distance, NumberNotNull);
        const huntvis: HuntVis = check(checkVis, HuntVisValid);
        // NpcIterator over (tick, level, x, z, distance, huntvis, DISTANCE)
        // filter: npc.type === npcType.id; closest by euclideanSquaredDistance
        // miss: pushInt(0); hit: activeNpc = closest; pointerAdd(ActiveNpc[intOperand]); pushInt(1)
    }
    ```
- **NPC_FINDCAT** — `NpcOps.ts:369-400`: same shape as NPC_FIND but `check(npcCategory, CategoryTypeValid)` and filter predicate `NpcType.get(npc.type).category === npcCategory`.
- **NPC_FINDEXACT** — `NpcOps.ts:94-112`: pops 2 ints `(coord, id)`; validates `CoordValid` + `NpcTypeValid`; iterates `NpcIterator(..., 0, 0, NpcIteratorType.ZONE)`; returns first NPC where `npc.type === npcType.id && npc.x === pos.x && npc.level === pos.level && npc.z === pos.z`.

### 2.2 Pointers

`src/engine/script/ScriptOpcodePointers.ts`:
- Line 576-580 (NPC_FIND): `set: ['active_npc'], set2: ['active_npc2'], conditional: true`.
- Line 581-585 (NPC_FINDCAT): same.
- Line 601-605 (NPC_FINDEXACT): same.

All three share the `conditional: true` pointer-set: the `active_npc` / `active_npc2` pointer is added only when the opcode pushes 1.

### 2.3 Validators

`src/engine/script/ScriptValidators.ts`:
- **CoordValid** (line 109): `ScriptInputCoordValidator` over `[0, 2147483647]`, returns `CoordGrid.unpackCoord(input)`.
- **NpcTypeValid** (line 111): `ScriptInputConfigTypeValidator` over `NpcType`; range `[0, NpcType.count)` + registry lookup.
- **HuntVisValid** (line 125): `ScriptInputRangeValidator` over `[HuntVis.OFF=0, HuntVis.LINEOFWALK=2]`.
- **CategoryTypeValid** (line 123): `ScriptInputConfigTypeValidator` over `CategoryType`; range `[0, CategoryType.count)` + registry lookup. **Partially ported — see S7f-D3.**
- **NumberNotNull** (line 102): already ported as `checkNotNull` (handlers_player.go:61, S7b). Reused.

### 2.4 Euclidean-squared distance

TS `CoordGrid.euclideanSquaredDistance(a, b) = (a.x-b.x)² + (a.z-b.z)²` — ignores level (level-match is handled upstream by the iterator's level-filter). Tie-break `closestDistance = <=` means **later-iterated NPC wins ties** (last-write-wins).

## 3. Architecture

### 3.1 New validators (`pkg/script/handlers_npc.go`)

All four land next to the existing `checkNotNull` / `checkStatID` precedent in `handlers_player.go`. Kept in `handlers_npc.go` to colocate with the NPC_FIND handlers, mirroring where S7c landed `checkInvType` next to its consumer.

```go
// checkCoord mirrors TS CoordValid (ScriptValidators.ts:109) — validates
// the packed int is in [0, 2147483647] and unpacks to (level, x, z). Returns
// an error tagged with the opcode name on negative input. Uses the existing
// unpackCoord helper at handlers_player.go:18.
func checkCoord(v int, op string) (level, x, z int, err error) {
    if v < 0 || v > 2147483647 {
        return 0, 0, 0, fmt.Errorf("%s: coord out of range (%d)", op, v)
    }
    level, x, z = unpackCoord(v)
    return
}

// checkNpcType mirrors TS NpcTypeValid (ScriptValidators.ts:111) — range
// check via Configs.NpcType presence. Follows the S7c checkInvType pattern
// (handlers_player.go:75) where range + registry collapse into a single nil
// check on the config lookup.
func checkNpcType(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.NpcType(id) == nil {
        return fmt.Errorf("%s: no NpcType with value (%d) found", op, id)
    }
    return nil
}

// checkHuntVis mirrors TS HuntVisValid (ScriptValidators.ts:125) — range
// [HuntVisOff=0, HuntVisLineOfWalk=2]. Goscape's HuntVis constants live in
// pkg/objtype/hunttype.go:22-26 and match TS values exactly.
func checkHuntVis(v int, op string) error {
    if v < 0 || v > 2 {
        return fmt.Errorf("%s: huntvis out of range (%d)", op, v)
    }
    return nil
}

// checkCategoryType mirrors TS CategoryTypeValid (ScriptValidators.ts:123)
// PARTIALLY. Goscape has no CategoryType config loader, so the count-bound
// check is absent — only the null-sentinel rejection survives. Deviation
// S7f-D3. Follow-up: wire count-bound when CategoryType loader lands.
func checkCategoryType(v int, op string) error {
    if v == -1 {
        return fmt.Errorf("%s: category null(-1)", op)
    }
    return nil
}
```

### 3.2 `NpcLookup` bridge (`pkg/script/state.go`)

New interface colocated with `PlayerLookup` (state.go:27-29) and `InvLookup` (state.go:52-56):

```go
// NpcLookup is the script→world bridge for NPC_FIND family opcodes. All
// methods return the matching NPC as script.ActiveNpc or nil when no match.
// Implementations iterate the world NPC registry; see serverNpcLookup
// (modules/world/npc_script_lookup.go) for the production impl.
//
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*)
// but the current implementation does not filter on it (S7f-D1). Callers
// must still validate the input via checkHuntVis.
type NpcLookup interface {
    // FindClosestNpcByType: NPC_FIND semantics. Square-bounded by dist
    // from (level, x, z); filter by typeID; closest by euclidean-squared
    // distance with later-match-wins on ties.
    FindClosestNpcByType(level, x, z, dist, typeID, huntvis int) ActiveNpc

    // FindClosestNpcByCategory: NPC_FINDCAT semantics. Same shape as
    // FindClosestNpcByType but filter via NpcType.Category == cat.
    FindClosestNpcByCategory(level, x, z, dist, cat, huntvis int) ActiveNpc

    // FindNpcAtExactCoord: NPC_FINDEXACT semantics. Returns the first
    // NPC at exactly (level, x, z) whose type matches typeID, or nil.
    FindNpcAtExactCoord(level, x, z, typeID int) ActiveNpc
}
```

New field on `ScriptState`:
```go
// Npcs is the NPC-lookup surface for NPC_FIND family opcodes. Callers
// set this after Init if the script uses find opcodes. Nil disables
// (handlers treat a nil surface as "no match found", pushing 0).
Npcs NpcLookup
```

The nil-Npcs-degrades-to-not-found semantics mirror TS's null-safe behavior when the world has no NPCs, and lets existing test fixtures that don't wire NpcLookup continue to pass without modification. Handlers still validate all inputs before consulting the bridge.

### 3.3 World-side implementation (`modules/world/npc_script_lookup.go` — new file)

```go
package world

import (
    "github.com/zsrv/goscape/pkg/script"
)

// serverNpcLookup implements script.NpcLookup by linearly iterating
// s.npcs. See S7f spec §3.3 and deviations S7f-D1 (huntvis validated-only)
// and S7f-D2 (linear iteration — future: route via s.grid.NearbyNpcs).
type serverNpcLookup struct{ s *Server }

func (l *serverNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, _ int) script.ActiveNpc {
    var closest *Npc
    bestDist := int(^uint(0) >> 1) // max int
    for _, n := range l.s.npcs {
        if n == nil || n.level != level || n.typeId != typeID {
            continue
        }
        dx, dz := n.x-x, n.z-z
        if dx < 0 { dx = -dx }
        if dz < 0 { dz = -dz }
        if dx > dist || dz > dist {
            continue
        }
        d := (n.x-x)*(n.x-x) + (n.z-z)*(n.z-z)
        if d <= bestDist {  // TS uses <=; later-match-wins
            closest = n
            bestDist = d
        }
    }
    if closest == nil {
        return nil
    }
    return closest
}

// FindClosestNpcByCategory — same shape but category predicate.
// Lookups NpcType.Category via l.s.npcTypes.Configs (with nil guards).

// FindNpcAtExactCoord — linear over s.npcs; filter exact (level, x, z)
// and type match; return first hit.
```

Wire-up — ALL sites enumerated here per `enumerate_all_sites` memory. Mirrors the exact pattern of the existing `invLookup invLookupView` at `server.go:77` / `s.invLookup = invLookupView{s: s}` at `server.go:198`:

| Site | Current line | Add |
|---|---|---|
| `modules/world/server.go:77` (Server struct field) | `invLookup invLookupView` | `npcLookup serverNpcLookup` (parallel line) |
| `modules/world/server.go:198` (production init) | `s.invLookup = invLookupView{s: s}` | `s.npcLookup = serverNpcLookup{s: s}` |
| `modules/world/interaction_trigger.go:85` | `state.Inv = srv.invLookup` | `state.Npcs = srv.npcLookup` |
| `modules/world/interaction_trigger.go:157` | same | same |
| `modules/world/interaction_trigger.go:318` | same | same |
| `modules/world/interaction_trigger.go:387` | same | same |
| `modules/world/npc_script.go:110` | `state.Inv = s.invLookup` | `state.Npcs = s.npcLookup` |
| `modules/world/script_test.go:618` (test fixture) | `s.invLookup = invLookupView{s: s}` | `s.npcLookup = serverNpcLookup{s: s}` |
| `modules/world/script_test.go:663` | same | same |
| `modules/world/script_test.go:696` | same | same |
| `modules/world/script_test.go:743` | same | same |
| `modules/world/script_test.go:806` | same | same |

Twelve call sites total (one field decl + one production init + five production wire-up + five test wire-up). Implementer Task 3 re-greps `invLookup` pre-commit to verify no new sites have landed since this enumeration.

`serverNpcLookup` follows the `invLookupView` pattern as a value-typed struct holding `s *Server` (not a pointer); confirmed by `server.go:77` field type. Keeps the field-copy semantics matching the existing lookup surface.

### 3.4 Handlers (`pkg/script/handlers_npc.go`)

All three share the same spine: validate → lookup → branch on hit/miss.

```go
// handleNpcFind (NPC_FIND, opcode 2513) pops (coord, npc, distance, huntvis),
// validates each, asks NpcLookup for the closest NPC of that type within
// square-bounded distance, and either sets the active NPC slot + pushes 1
// or pushes 0. Mirrors TS NpcOps.ts:336-367.
func handleNpcFind(s *ScriptState) error {
    checkVis := s.PopInt()
    distance := s.PopInt()
    npcTypeID := s.PopInt()
    coord := s.PopInt()

    level, x, z, err := checkCoord(coord, "NPC_FIND")
    if err != nil { return err }
    if err := checkNpcType(s, npcTypeID, "NPC_FIND"); err != nil { return err }
    if err := checkNotNull(distance, "NPC_FIND"); err != nil { return err }
    if err := checkHuntVis(checkVis, "NPC_FIND"); err != nil { return err }

    var npc ActiveNpc
    if s.Npcs != nil {
        npc = s.Npcs.FindClosestNpcByType(level, x, z, distance, npcTypeID, checkVis)
    }
    if npc == nil {
        s.PushInt(0)
        return nil
    }
    setActiveNpcSlot(s, npc)
    s.PushInt(1)
    return nil
}

// handleNpcFindCat — identical spine, checkCategoryType replaces
// checkNpcType on the category slot, FindClosestNpcByCategory replaces
// FindClosestNpcByType. Mirrors TS NpcOps.ts:369-400.

// handleNpcFindExact — two popInts (coord, npcTypeID), two validators
// (checkCoord, checkNpcType), FindNpcAtExactCoord. Mirrors TS
// NpcOps.ts:94-112.
```

TS `popInts(n)` pops in destructured order `[a, b, c, d]` where `a` was pushed first — i.e., stack order is LIFO, so `popInts(4)` returns `[bottom, ..., top]`. In goscape's `PopInt()` model, we pop one at a time and order matters: arguments pushed first are at the bottom of the stack, so we pop in reverse-declaration order. The `checkVis → distance → npcTypeID → coord` sequence in `handleNpcFind` above matches this — rightmost arg (huntvis) pops first, leftmost (coord) pops last.

### 3.5 Pointer-slot helper (`pkg/script/handlers_npc.go`)

```go
// setActiveNpcSlot writes the found NPC to either ActiveNpc or
// OtherActiveNpc based on the handler's IntOperand and sets the
// corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveNpc[state.intOperand]) at NpcOps.ts:365, 398, 105.
//
// IntOperand==0 → ActiveNpc / PtrActiveNpc (primary, .npc syntax).
// IntOperand==1 → OtherActiveNpc / PtrActiveNpc2 (secondary, .npc2 syntax).
// Any other value panics (compiler invariant; compiled bytecode only emits 0/1).
func setActiveNpcSlot(s *ScriptState, npc ActiveNpc) {
    operand := s.Script.IntOperands[s.PC]
    switch operand {
    case 0:
        s.ActiveNpc = npc
        s.Pointers |= PtrActiveNpc
    case 1:
        s.OtherActiveNpc = npc
        s.Pointers |= PtrActiveNpc2
    default:
        panic(fmt.Sprintf("setActiveNpcSlot: invalid IntOperand %d", operand))
    }
}
```

### 3.6 Registry (`pkg/script/handlers.go`)

```go
// NPC find (S7f) — closest-single cluster.
OpNpcFind:      handleNpcFind,
OpNpcFindCat:   handleNpcFindCat,
OpNpcFindExact: handleNpcFindExact,
```

Placement: existing NPC_* registrations live at `handlers.go:314-331` (NPC read ops at 314-321, NPC_SAY at 324, NPC state mutators at 327-331). Insert the new S7f block immediately after the NPC-mutator block (around line 332) with a blank-line separator and the `// NPC find (S7f) — closest-single cluster.` section comment above.

## 4. File map

| File | Change | Est. LOC |
|---|---|---|
| `pkg/script/handlers_npc.go` | + 4 validators + 3 handlers + slot helper | +170 |
| `pkg/script/handlers_npc_test.go` | + 17 handler test cases + validator unit tests | +280 |
| `pkg/script/state.go` | + `NpcLookup` interface + `Npcs` field | +25 |
| `pkg/script/handlers.go` | + 3 registry entries + section comment | +5 |
| `pkg/script/runner_test.go` | + `mockNpcLookup` for handler tests | +50 |
| `modules/world/npc_script_lookup.go` | **new file** — `serverNpcLookup` impl | +100 |
| `modules/world/npc_script_lookup_test.go` | **new file** — 3 integration tests | +130 |
| `modules/world/server.go` | + `npcLookup serverNpcLookup` field + init | +2 |
| `modules/world/interaction_trigger.go` | + `state.Npcs = srv.npcLookup` at 4 sites (85, 157, 318, 387) | +4 |
| `modules/world/npc_script.go` | + `state.Npcs = s.npcLookup` at site 110 | +1 |
| `modules/world/script_test.go` | + `s.npcLookup = serverNpcLookup{s: s}` at 5 test-fixture sites | +5 |

**Production total:** ~350 LOC.
**Test total:** ~410 LOC (280 handler + 130 integration).
**Grand total:** ~760 LOC across 8 files.

## 5. Test plan

All tests pass `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/... ./modules/world/...`.

### 5.1 Validator unit tests (`pkg/script/handlers_npc_test.go`)

Table-driven `TestCheckCoord` / `TestCheckNpcType` / `TestCheckHuntVis` / `TestCheckCategoryType`. Each covers: happy path, min boundary, max boundary, rejection case (-1 or out-of-range), error message contains opcode-name prefix and offending value.

### 5.2 NPC_FIND handler tests (9 cases)

Each test uses a `mockNpcLookup` with pre-seeded return values:

1. `TestNpcFind_SingleMatch` — mockLookup returns one NPC; handler pushes 1, sets ActiveNpc, sets PtrActiveNpc.
2. `TestNpcFind_ClosestWinsOnTies` — mockLookup (or real-world fixture in 5.5) returns the later-iterated of two equidistant NPCs (pins `<=` semantics).
3. `TestNpcFind_NoMatch` — mockLookup returns nil; handler pushes 0, ActiveNpc untouched, PtrActiveNpc unset.
4. `TestNpcFind_NilNpcLookup` — `state.Npcs == nil`; handler pushes 0, validators still run (assert they DO run via invalid-input variant).
5. `TestNpcFind_IntOperandZero` — IntOperands[PC]=0, successful match → ActiveNpc + PtrActiveNpc, NOT OtherActiveNpc.
6. `TestNpcFind_IntOperandOne` — IntOperands[PC]=1 → OtherActiveNpc + PtrActiveNpc2, NOT ActiveNpc.
7. `TestNpcFind_InvalidCoord` — coord=-1 → error containing "NPC_FIND: coord out of range", NO mockLookup call, NO stack change beyond the 4 pops.
8. `TestNpcFind_InvalidNpcType` — npcTypeID unloaded → error, no lookup.
9. `TestNpcFind_NullDistance` / `TestNpcFind_InvalidHuntVis` — two sub-cases of validator-fail; error, no lookup.

### 5.3 NPC_FINDCAT handler tests (4 cases)

1. `TestNpcFindCat_SingleMatch` — mockLookup returns NPC with matching category; push 1, pointer-set.
2. `TestNpcFindCat_NoMatch` — nil; push 0.
3. `TestNpcFindCat_NullCategory` — category=-1 → error (pins partial CategoryType validator).
4. `TestNpcFindCat_NonNegativeCategory` — category=0 validates (no registry count bound — pins S7f-D3 behavior).

### 5.4 NPC_FINDEXACT handler tests (4 cases)

1. `TestNpcFindExact_Match` — coord + type match → push 1, pointer-set.
2. `TestNpcFindExact_NoNpcAtCoord` — empty / off-coord → push 0.
3. `TestNpcFindExact_TypeMismatchAtCoord` — NPC present at coord but wrong type → push 0.
4. `TestNpcFindExact_InvalidInputs` — coord=-1 or invalid type → error.

### 5.5 World-side integration tests (`modules/world/npc_script_lookup_test.go`)

Using the existing `setupNpc(s, x, z, level)` helper at `modules/world/player_npc_test.go:33`:

1. `TestServerNpcLookup_FindClosestByType` — place 3 NPCs (2 of target type, 1 other); assert the closer of the two target-type NPCs is returned; assert the wrong-type NPC is never returned regardless of distance.
2. `TestServerNpcLookup_FindClosestByCategory` — place 2 NPCs with matching category, 1 non-matching; assert category-match wins.
3. `TestServerNpcLookup_FindAtExactCoord` — place NPC at (50, 50, 0); query exact match, assert returns the NPC; query off-by-one in x/z/level, assert nil each time.

### 5.6 Mock extension (`pkg/script/runner_test.go`)

```go
type mockNpcLookup struct {
    // Per-method return value + call-capture.
    byType     ActiveNpc
    byCategory ActiveNpc
    atCoord    ActiveNpc
    byTypeCalls, byCategoryCalls, atCoordCalls int
    lastArgs   []int // captures the most recent call's args for cross-check
}
```

Each method records its args into `lastArgs` and returns the corresponding field. Tests assert both the return behavior and the args passed through from handler to lookup (catches arg-order mistakes in the handler code).

### 5.7 Smoke re-verification (implicit)

`[proc,set_hint_newbie_basics_instructor]` advancing past `pc=8` after S7f merges is the integration test. User-driven per `smoke_test_server_handoff` memory.

## 6. Task split

Three tasks per user's collapse of original 2+3.

### Task 1 — pkg/script foundation

Land in `pkg/script/` only. Files: `handlers_npc.go` (validators + slot helper), `state.go` (NpcLookup interface + field), `runner_test.go` (mockNpcLookup), `handlers_npc_test.go` (validator unit tests only).

Deliverable: validators tested and green; NpcLookup surface present and compilable; mock ready. Handlers NOT yet landed — they come in Task 2.

Est. ~250 LOC. Single commit.

### Task 2 — All three handlers + handler tests

Files: `handlers_npc.go` (handlers), `handlers.go` (registry), `handlers_npc_test.go` (tests 5.2-5.4).

Depends on Task 1's NpcLookup + mock.

Deliverable: all three handlers working against `mockNpcLookup`; all 17 handler tests pass; ActiveNpc / OtherActiveNpc slot switching verified; validator errors tested end-to-end.

Est. ~350 LOC. Single commit.

### Task 3 — World-side bridge + integration + close

Files:
- `modules/world/npc_script_lookup.go` (new) — `serverNpcLookup` impl
- `modules/world/npc_script_lookup_test.go` (new) — 3 integration tests
- `modules/world/server.go` — field decl + production init (2 lines)
- `modules/world/interaction_trigger.go` — 4 production wire-up sites
- `modules/world/npc_script.go` — 1 production wire-up site
- `modules/world/script_test.go` — 5 test-fixture wire-up sites
- Close commit.

Implementer MUST re-grep `state\.Inv = srv\.invLookup` and `state\.Inv = s\.invLookup` and `s\.invLookup = invLookupView` at Task-3 start to catch any sites that landed after this spec was written (per `enumerate_all_sites` memory).

Depends on Task 2's handler tests being green.

Deliverable: `serverNpcLookup` implements all three methods against `s.npcs`; 3 integration tests pass using real `*Npc` fixtures; wire-up lands so the real tutorial script can now find NPCs.

Est. ~250 LOC including the close commit. Close commit message:
```
chore(script): S7f closed — NPC_FIND / FINDCAT / FINDEXACT + NpcLookup bridge

Closes: unblocks [proc,set_hint_newbie_basics_instructor] past pc=8.
Deviations added: S7f-D1 (huntvis validated-only), S7f-D2 (linear
iteration), S7f-D3 (CategoryType count-bound absent). Follow-ups in
spec §8.
```

## 7. Deviations

| ID | Status |
|---|---|
| **S7a-D1/D2, S7b-D1, S7c-D1, S7d-D1/D2/D3/D4, S7e-D1** | Pre-existing — carried, unrelated. |
| **S7f-D1** | **NEW** — `HuntVis` parameter is validated (`checkHuntVis`) but not filtered on in lookups. All three values (OFF/LINEOFSIGHT/LINEOFWALK) behave as OFF. Effect: NPC_FIND/FINDCAT find NPCs even behind obstacles that TS's LineValidator would reject. Graceful (finds more, not fewer); only matters for scripts that use NPC_FIND to check visibility explicitly. Follow-up: wire `s.gamemap.Pathfinder.LineValidator.HasLineOfSight/Walk` at the world-side lookup, mirroring `huntNpcs` at `modules/world/npc_hunt_entities.go:63-72`. |
| **S7f-D2** | **NEW** — NPC iteration is linear O(n) over `s.npcs` registry rather than zone-indexed via `s.grid.NearbyNpcs`. Correct but not optimal for large worlds. Follow-up: route through `s.grid.NearbyNpcs(x, z, level, zoneRadius)` when NPC counts scale; tutorial area has <20 NPCs so there's no perf pressure now. |
| **S7f-D3** | **NEW** — `checkCategoryType` rejects only -1 (NumberNotNull-equivalent); the count-bound check TS does via `CategoryType.count` is absent because goscape's config loader doesn't populate `CategoryType`. Effect: an invalid category id that TS would reject pre-lookup will instead produce a valid-args lookup that returns nil (no NPCs match), pushing 0 — same observable behavior as a no-match, just via a different path. Follow-up: add count-bound when the CategoryType loader lands. |

**Closures:** None.

## 8. Follow-ups

- **NPC_FIND* iterator cluster** — `NPC_FINDALL` (2514), `NPC_FINDALLANY` (2515), `NPC_FINDALLZONE` (2516), `NPC_FINDNEXT` (2520). These need a persistent `NpcQuery` iterator on `ScriptState` (parallel to `DbRowQuery` from S7d). Separate sub-spec. Can reuse this sub-spec's validators and most of the world-side iteration logic.
- **NPC_FINDHERO / NPC_FINDUID** — direct-lookup cluster; NPC_FINDHERO (2519) reads `activeNpc.heroPoints.findHero()` then world-finds a player; NPC_FINDUID (2521) is `s.npcs[uid]`-style direct lookup. Different semantics from the iteration cluster; may collapse into one short sub-spec.
- **HuntVis filtering (S7f-D1 closure)** — wire `LineValidator.HasLineOfSight/Walk` at the three `serverNpcLookup` methods. Blocking: none. Trigger: when a script calls NPC_FIND expecting visibility filtering to matter.
- **Zone-indexed iteration (S7f-D2 closure)** — swap naive loop for `s.grid.NearbyNpcs`. Trigger: profiling shows NPC_FIND is hot, or NPC count scales past tutorial area.
- **CategoryType config loader (S7f-D3 closure)** — add a `CategoryType` registry to the cache package; update `checkCategoryType` to check `s.Configs.CategoryType(v) != nil`. Trigger: a separate sub-spec that ports the CategoryType loader.
- **Bundled polish from S7e code review** (still pending next `handlers.go` / `runner_test.go` touch): (a) `handlers.go:348` comment style consistency; (b) `runner_test.go:430-432` mock docstring duplication. **Both files are touched in S7f Tasks 1-2** — these polish items should ride in the S7f commits, same bundling pattern as S7e bundled S7d's polish.
- **S7d re-verification** — still inferential. Watching for `combat_get_damagetype` smoke confirmation on the next tutorial run past S7f.

## 9. Self-review notes

- **Placeholders:** None. Every TS line-reference was verified via direct read during brainstorm: `NpcOps.ts:94-112, 336-367, 369-400`; `ScriptOpcodePointers.ts:576-605`; `ScriptValidators.ts:102-141`; `Player.ts:323` (not applicable here but the read-path for analogous ports was confirmed). Every goscape cite was verified: `HuntVis*` at `pkg/objtype/hunttype.go:22-26`, `PtrActiveNpc / PtrActiveNpc2` at `pkg/script/pointer.go:10-11`, `OtherActiveNpc` at `state.go:128`, `s.npcs` iteration precedent at `npc_hunt_entities.go:23-72`.
- **Internal consistency:** §3.4 handlers ↔ §5.2-§5.4 test assertions (every validator check, both pointer-slot branches, hit/miss/nil-Npcs paths). §3.2 `NpcLookup` interface ↔ §3.3 `serverNpcLookup` impl shape ↔ §5.5 integration tests (one test per method). §3.5 slot helper ↔ §5.2 IntOperand=0/1 tests. §4 file map LOC sums align with §3/§5 bodies. §7 deviations cross-reference §3.1 (D3 at checkCategoryType docstring) and §3.3 (D1/D2 at serverNpcLookup).
- **Scope:** Three tasks, three opcodes, one bridge. Larger than compressed cadence but aligned with S-series standard cadence (S7a also had a 4-task plan). The bundle size is deliberately chosen to land the shared infrastructure once — splitting would require reviewing and testing the validators/bridge across three sub-specs with identical review checklists.
- **Ambiguity:** Four checked. (1) **NpcLookup three methods vs one predicate-callback** — resolved in favor of three methods: matches existing `InvLookup` shape at state.go:52-56 (small, typed, method-per-semantic), avoids passing Go closures across the pkg/script → modules/world boundary. (2) **Euclidean-squared helper placement** — inlined in `serverNpcLookup` (per §3.3); extracting to `pkg/coordgrid` deferred until a second caller surfaces. (3) **HuntVis fidelity level** — validated but not filtered (S7f-D1), explicit deviation rather than a silent "approximate" implementation. (4) **CategoryType partial validator** — explicit deviation (S7f-D3), handler behavior stays correct for hit-path (NpcType.Category == cat filter); no silent divergence.
- **Test-coverage crosscheck (per `plan_test_coverage_crosscheck` memory):** every code path in §3 has a corresponding test in §5: validators (§5.1 covers all four), handler hit/miss paths (§5.2 #1-#3, §5.3 #1-#2, §5.4 #1-#2), IntOperand switching (§5.2 #5-#6), nil-NpcLookup degradation (§5.2 #4), each validator rejection (§5.2 #7-#9, §5.3 #3, §5.4 #4). World-side integration in §5.5 covers each lookup method end-to-end against real `*Npc` fixtures.
- **Plan-helper coverage (per `plan_helper_coverage` memory):** three new helpers introduced — `checkCoord` has one consumer per handler (3 sites), `checkNpcType` has 2 consumers (NPC_FIND + NPC_FINDEXACT), `checkHuntVis` has 2 consumers (NPC_FIND + NPC_FINDCAT), `setActiveNpcSlot` has 3 consumers. Unit tests in §5.1 exercise the helpers in isolation; handler tests in §5.2-§5.4 exercise them from each consumer site.
- **TS source read (per `spec_ts_source_read` memory):** all §3 code blocks drafted against directly-read TS source at `NpcOps.ts:94-112, 336-367, 369-400` — not by analogy from the huntNpcs world-side implementation (which has similar iteration shape but different validation/pointer semantics).
- **Enumerate-all-sites (per `enumerate_all_sites` memory):** the new `NpcLookup` field adds a new nil-guard branch in every NPC_FIND-family handler. All current consumer handlers are in this same sub-spec. Once Task 2 lands, subsequent NPC_FIND* sub-specs must also nil-guard on `state.Npcs` or share a common helper. Flagged for the implementer's Task 2 self-review.
- **True-to-TS gate:** three explicit deviations (D1 huntvis, D2 iteration perf, D3 CategoryType). No silent divergences. All three are graceful-degradation (finds more NPCs, not fewer, vs TS).
