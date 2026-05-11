# NAI-159: LOC_FIND handler activation — design

## 1. Summary

Activate the `LOC_FIND` opcode handler stub at
`pkg/script/handlers_loc.go:107`. The current MVP pops both args and
unconditionally pushes 0; this sub-spec replaces it with the TS-faithful
implementation that looks up an exact-tile loc by type via
`World.getLoc`, binds it to `ActiveLoc` (or `OtherActiveLoc` per
`IntOperand`), and pushes 1 on hit / 0 on miss.

A new method `GetLoc` is added to the `script.LocOps` interface; the
`modules/world` adapter delegates to the existing
`Server.GetLoc(level, x, z, locId)` (loc_lookup.go:14) — no new world-side
infrastructure.

## 2. Source

- TS handler: `Engine-TS/src/engine/script/handlers/LocOps.ts:79-94`
  (`LOC_FIND` case in the dispatch table).
- TS underlying lookup: `Engine-TS/src/engine/World.ts:1321-1323`
  (`getLoc`) → `Engine-TS/src/engine/zone/Zone.ts:259-266` (zone-coord
  bucketed scan filtering by `loc.type === type`).
- TS validators: `check(coord, CoordValid)` →
  `ScriptValidators.ts:109` (range `[0, 2147483647]`); `check(locId,
  LocTypeValid)` → LocType non-null lookup.
- TS pointer-set: `state.pointerAdd(ActiveLoc[state.intOperand])` (same
  pattern as `LOC_FINDNEXT`).

Per `ts_source_canonical_path.md`, only `LostCityRS/Engine-TS` is the
porting reference.

## 3. Pre-flight findings (HEAD)

| # | Dep | Status |
|---|---|---|
| 1 | `Server.GetLoc(level, x, z, locId)` | OK — `modules/world/loc_lookup.go:14-25`; semantics already match TS (same-tile + type-equality scan over `zoneMap.Get(...).Locs`). Currently used only by `OpLocHandler` validation; this sub-spec plumbs it through `LocOps` |
| 2 | `script.LocOps` interface | OK — `pkg/script/loc_ops.go:19-26`; gains one method (`GetLoc`) |
| 3 | `serverLocOps` adapter | OK — `modules/world/script_loc_ops.go:17`; gains one method delegating to `Server.GetLoc` |
| 4 | LocOps mock satisfiers | enumerated: `fakeLocOps` (`pkg/script/loc_ops_test.go:55`) and `mapLocAddUnsafeOps` (`pkg/script/handlers_map_test.go:618`). Both need a `GetLoc` method to remain interface-conformant |
| 5 | `setActiveLocSlot` pointer routing | OK — `pkg/script/handlers_loc.go:29-41`; routes IntOperand 0/1 to `ActiveLoc`/`OtherActiveLoc` and sets `PtrActiveLoc`/`PtrActiveLoc2`. Reused by `LOC_FINDNEXT` |
| 6 | `requireConfigs` | OK — used by `LOC_ADD`, `LOC_CHANGE`, `LOC_TYPE`, etc. for the `LocTypeValid` analogue |
| 7 | `checkCoord(v, op)` | OK — `pkg/script/handlers_npc.go:13-19`; reused by `LOC_FINDALLZONE` (handlers_loc.go:52) |
| 8 | `ActiveLoc` interface | OK — `pkg/script/active.go:898-905`; `*entity.Loc` satisfies it (consumed by `LocsAtCoord` and `AllLocsInZone` already) |
| 9 | Existing adapter test pattern | OK — `TestServerLocOpsLocsAtCoord` at `modules/world/script_loc_ops_test.go:58` provides a copy-template for `TestServerLocOpsGetLoc` |

No spatial-index migration concerns (per `parallel_spatial_index_migration_pattern.md`): both `Server.GetLoc` and `LocsAtCoord` consume the same `zoneMap.Get(...).Locs` slice. No dual-path window exists here.

## 4. Cadence

Full cadence (brainstorm → spec → plan → subagent-driven TDD); compressed
not viable per `compressed_cadence.md` — although behavioral code is
~20 LOC, the change includes a cross-package interface addition, three
satisfier updates (adapter + two mocks), and a multi-case test surface
spanning two packages.

Execution per `execution_mode_default.md`: dispatch via
subagent-driven-development. Per `superpowers_clear_between_spec_and_impl.md`,
plan handoff prompts a `/clear` before implementation.

## 5. Architecture

```
ScriptState dispatch (LOC_FIND)
    ▼
pkg/script/handlers_loc.go::handleLocFind
    ├─ requireConfigs(s, "LOC_FIND")           // TS: check throws on nil-Configs
    ├─ locId := s.PopInt()                     // TS popInts(2)[1]
    ├─ coord := s.PopInt()                     // TS popInts(2)[0]
    ├─ level, x, z := checkCoord(coord, …)     // TS: check(coord, CoordValid)
    ├─ s.Configs.LocType(locId) != nil         // TS: check(locId, LocTypeValid)
    ├─ s.LocOps != nil                         // goscape defensive
    ├─ loc := s.LocOps.GetLoc(level, x, z, locId)
    └─ if loc == nil → push 0
       else         → setActiveLocSlot(s, loc) + push 1
                       (pointer flag set per IntOperand 0/1)
       ▼
script.LocOps.GetLoc                            (NEW interface method)
    ▼
modules/world/script_loc_ops.go::(*serverLocOps).GetLoc
    ▼
modules/world/loc_lookup.go::(*Server).GetLoc   (EXISTING — unchanged)
    ▼
zoneMap.Get(level, x, z).Locs                   (linear scan)
```

No changes to dispatcher tables, opcode registration, or `ActiveLoc`
interface. No new packages.

## 6. Components

### 6.1 `pkg/script/loc_ops.go` — interface extension

Add one method to the `LocOps` interface:

```go
type LocOps interface {
    ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error
    AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error)
    RemoveLoc(loc ActiveLoc, duration int) error
    AnimLoc(loc ActiveLoc, seq int) error
    LocsAtCoord(level, x, z int) []ActiveLoc
    AllLocsInZone(level, x, z int) []ActiveLoc
    GetLoc(level, x, z, typ int) ActiveLoc  // NEW
}
```

Method-level doc comment notes:
- Returns nil on no-match (no error path — caller distinguishes hit vs
  miss via nil-check).
- Mirrors TS `World.getLoc(x, z, level, locId)` exact-tile + type-equality
  semantics.
- Sole caller is `LOC_FIND` (LocOps.ts:79-94).

### 6.2 `modules/world/script_loc_ops.go` — adapter method

```go
// GetLoc delegates to Server.GetLoc (loc_lookup.go) and wraps nil
// *entity.Loc into nil script.ActiveLoc. Returning the typed-nil from
// Server.GetLoc directly would produce a non-nil ActiveLoc interface
// holding a typed-nil pointer (Go interface-nil gotcha); the explicit
// check avoids that.
func (o *serverLocOps) GetLoc(level, x, z, typ int) script.ActiveLoc {
    l := o.s.GetLoc(level, x, z, typ)
    if l == nil {
        return nil
    }
    return l
}
```

### 6.3 `pkg/script/handlers_loc.go` — handler

Replace the stub at line 107-112 with:

```go
// handleLocFind (LOC_FIND, opcode 3007) pops [coord, locId],
// looks up the matching loc at that tile, and either binds it to the
// ActiveLoc slot selected by IntOperand (0 → primary, 1 → secondary)
// and pushes 1, or pushes 0 on miss. Mirrors TS LocOps.ts:79-94:
//
//   const [coord, locId] = state.popInts(2);
//   const locType = check(locId, LocTypeValid);
//   const position = check(coord, CoordValid);
//   const loc = World.getLoc(position.x, position.z, position.level, locType.id);
//   if (!loc) { state.pushInt(0); return; }
//   state.activeLoc = loc;
//   state.pointerAdd(ActiveLoc[state.intOperand]);
//   state.pushInt(1);
//
// Pointer-set threads IntOperand 0/1 through setActiveLocSlot
// (handlers_loc.go:29) — same pattern as LOC_FINDNEXT.
//
// Miss-semantics: ActiveLoc / OtherActiveLoc are NOT mutated on miss
// (TS only writes activeLoc inside the hit arm). Pinned by test.
//
// Configs/LocOps nil are surfaced as errors (LOC_ADD / LOC_CHANGE
// precedent) rather than silent push-0, because the TS `check` operator
// throws on locType lookup failure.
func handleLocFind(s *ScriptState) error {
    if err := requireConfigs(s, "LOC_FIND"); err != nil {
        return err
    }
    locId := s.PopInt()
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "LOC_FIND")
    if err != nil {
        return err
    }
    if s.Configs.LocType(locId) == nil {
        return fmt.Errorf("LOC_FIND: unknown loc id %d", locId)
    }
    if s.LocOps == nil {
        return fmt.Errorf("LOC_FIND: LocOps unavailable")
    }
    loc := s.LocOps.GetLoc(level, x, z, locId)
    if loc == nil {
        s.PushInt(0)
        return nil
    }
    setActiveLocSlot(s, loc)
    s.PushInt(1)
    return nil
}
```

### 6.4 Mock satisfier updates

Two files need a one-line addition each:

`pkg/script/loc_ops_test.go` — `fakeLocOps` gains a recorded-call slice
and a canned-return field, matching the existing recorder idiom (see
`changeCalls`/`addCalls` slices + `addReturn` canned value at lines 7-13):

```go
type getLocCall struct {
    level, x, z, typ int
}

// new fields on fakeLocOps:
getLocCalls  []getLocCall
getLocReturn ActiveLoc

func (f *fakeLocOps) GetLoc(level, x, z, typ int) ActiveLoc {
    f.getLocCalls = append(f.getLocCalls, getLocCall{level, x, z, typ})
    return f.getLocReturn
}
```

Default zero-value `getLocReturn` is nil-`ActiveLoc` (the miss case);
hit tests set `f.getLocReturn = stub` before driving the handler. The
recorded-call slice supports the pop-order pin (§9.1 Test #8) — per
`mock_recorder_field_naming_check.md` field names mirror existing
recorder conventions in the same file.

`pkg/script/handlers_map_test.go:618` — `mapLocAddUnsafeOps` gains:

```go
func (m *mapLocAddUnsafeOps) GetLoc(level, x, z, typ int) ActiveLoc { return nil }
```

(One-line no-op; matches the file's existing pattern for unused-by-this-test
LocOps methods.)

## 7. Data flow

1. Script bytecode dispatches LOC_FIND with [coord, locId] on the int
   stack and IntOperand ∈ {0, 1} from the bytecode's int-operand slot.
2. Handler pops top-of-stack (locId) then next (coord), preserving TS
   `popInts(2)` array ordering convention used elsewhere in goscape (see
   `handleLocChange` for the equivalent two-arg case).
3. `checkCoord` validates and unpacks to (level, x, z) — same path
   as `LOC_FINDALLZONE`.
4. `Configs.LocType` lookup gates on a known type; mirrors TS
   `LocTypeValid` non-null check.
5. `LocOps.GetLoc` scans `zoneMap.Get(level, x, z).Locs` linearly,
   returning the first loc with matching (level, x, z) AND `Type() ==
   typ`. Nil on no-match.
6. On hit: `setActiveLocSlot` writes to `ActiveLoc`/`OtherActiveLoc` per
   IntOperand and OR's the matching pointer flag. Push 1.
7. On miss: push 0; no state mutation.

## 8. Error handling

Stack-on-error semantics: TS uses `check(...)` which throws — leaving the
JS stack "as-is" relative to the host (unhandled). Goscape's
`requireConfigs` runs BEFORE the pops; a nil-Configs error therefore
leaves operands on the stack. This matches the established
LOC_ADD/LOC_CHANGE convention and is acceptable per the existing
codebase pattern.

Error messages follow goscape `"LOC_FIND: %s"` prefix convention,
mirroring sibling handlers:

| Failure | Message | Precedent |
|---|---|---|
| nil-Configs | `"LOC_FIND: Configs not set on ScriptState"` | `requireConfigs` (handlers_config.go:60) |
| bad coord | `"LOC_FIND: coord out of range (%d)"` | `checkCoord` (handlers_npc.go:15) |
| unknown locId | `"LOC_FIND: unknown loc id %d"` | LOC_CHANGE (handlers_loc.go:306) |
| nil-LocOps | `"LOC_FIND: LocOps unavailable"` | LOC_ADD (handlers_loc.go:403) |

ActiveLoc-on-miss invariant: spec audited the TS method line-by-line per
`audit_full_method_against_ts.md` — TS writes `state.activeLoc` ONLY
inside the hit arm; miss arm contains exactly `state.pushInt(0); return;`.
Test 3 below pins this.

## 9. Testing

### 9.1 `pkg/script/handlers_loc_test.go` — handler unit tests

Eight cases (table-driven where it pays off):

| # | Case | Setup | Assert |
|---|---|---|---|
| 1 | hit, slot 0 | fakeLocOps.GetLocFn returns a stub ActiveLoc; IntOperand=0; push coord then locId | `s.ActiveLoc == stub`; `s.Pointers & PtrActiveLoc != 0`; top of int stack == 1 |
| 2 | hit, slot 1 | same as #1, IntOperand=1 | `s.OtherActiveLoc == stub`; `s.Pointers & PtrActiveLoc2 != 0`; top of int stack == 1 |
| 3 | miss leaves ActiveLoc unchanged | pre-populate `s.ActiveLoc = sentinel`; fakeLocOps.GetLocFn returns nil | `s.ActiveLoc == sentinel`; `s.Pointers` unchanged; top of int stack == 0 |
| 4 | nil-Configs errors | `s.Configs = nil` | err == `"LOC_FIND: Configs not set on ScriptState"` (verbatim `requireConfigs` output); no push |
| 5 | nil-LocOps errors | Configs populated, `s.LocOps = nil` | err == `"LOC_FIND: LocOps unavailable"`; no push |
| 6 | unknown locId errors | Configs returns nil for typ=999 | err == `"LOC_FIND: unknown loc id 999"`; no push |
| 7 | bad coord errors | coord = -1 | err contains `"LOC_FIND: coord out of range"`; no push |
| 8 | pop-order locId-then-coord | fakeLocOps records its call args; push coord=PackCoord(0,3094,3106), then locId=42 | recorded args == `(level=0, x=3094, z=3106, typ=42)` |

Case 8 is the key regression pin — catches accidental pop-order swap
(per `handler_pop_order_test_masking.md`). The recorded-args check
verifies BOTH that the right value lands in the right parameter AND that
checkCoord unpacks correctly.

### 9.2 `pkg/script/loc_ops_test.go` — fakeLocOps extension

Add the `GetLocFn` field and method per §6.4. No new test cases here —
the surface is exercised via handler tests.

### 9.3 `pkg/script/handlers_map_test.go` — mock conformance

Add the one-line `GetLoc` no-op to `mapLocAddUnsafeOps`. Existing
MAP_LOCADDUNSAFE tests must continue to pass unchanged.

### 9.4 `modules/world/script_loc_ops_test.go` — adapter integration

Add `TestServerLocOpsGetLoc` mirroring `TestServerLocOpsLocsAtCoord`
(line 58). Three sub-cases:

1. **hit** — construct Server with one loc at (level=0, x=3094, z=3106,
   typ=42). Assert `ops.GetLoc(0, 3094, 3106, 42)` returns non-nil
   ActiveLoc with matching `Coords()` and `LocType() == 42`.
2. **wrong type** — same fixture; `ops.GetLoc(0, 3094, 3106, 99)`
   returns nil.
3. **wrong coord** — same fixture; `ops.GetLoc(0, 9999, 9999, 42)`
   returns nil.

Verifies the typed-nil → interface-nil wrapping in §6.2 (a regression
here would surface as `loc != nil` in Test #6.3 above failing at the
typed-nil hand-off).

## 10. Deviations

**No active deviations.** Every TS branch maps directly:

- Pop order: identical (TS `popInts(2)[0]=coord, [1]=locId` ↔ goscape
  `PopInt`-first=locId, second=coord).
- Validators: `check(coord, CoordValid)` → `checkCoord`;
  `check(locId, LocTypeValid)` → `Configs.LocType(...) != nil`.
- Lookup: TS `zone.getLoc` ↔ goscape `Server.GetLoc` (both: exact tile
  + `Type() == typ`).
- Pointer-set: TS `pointerAdd(ActiveLoc[intOperand])` ↔ goscape
  `setActiveLocSlot` (already used by LOC_FINDNEXT).
- Push values + miss-arm no-mutation: identical.

Goscape adds two defensive guards not present in TS: nil-Configs and
nil-LocOps. Both are labeled "(goscape defensive; TS reaches via static
accessor)" in the doc comment per `defensive_gate_doc_comment_label.md`.
These are not deviations — they are infrastructure shims for the
test-runtime convention.

## 11. Task split (subagent-driven-TDD)

**T1: LocOps interface + adapter + mock conformance** —
`pkg/script/loc_ops.go` interface extension;
`modules/world/script_loc_ops.go` adapter method;
`pkg/script/loc_ops_test.go` fakeLocOps extension;
`pkg/script/handlers_map_test.go` mapLocAddUnsafeOps one-liner;
`modules/world/script_loc_ops_test.go` TestServerLocOpsGetLoc with 3
sub-cases. Compiles + tests green standalone (handler stub still
push-0). Risk: mock surface ripple — plan-author re-greps `LocOps`
interface satisfiers at plan-write per `enumerate_all_sites.md`.

**T2: Handler activation** —
`pkg/script/handlers_loc.go` replace stub with real implementation;
add `TestHandleLocFind` (8 cases) to `pkg/script/handlers_loc_test.go`.
Imports T1's interface surface. Independent commit.

Each task is independently compilable and shippable in its own commit.
T1 first (interface change must land before T2 reads from it).

## 12. Risk register

| # | Risk | Mitigation |
|---|---|---|
| 1 | Mock satisfier missed → compile break on `LocOps` interface | Plan §pre-flight: `rg "LocsAtCoord\|AllLocsInZone" pkg/script/` enumerates satisfiers; T1 acceptance criterion includes `go build ./...` green |
| 2 | Typed-nil → interface-nil gotcha in adapter | Explicit nil-check in `serverLocOps.GetLoc`; pinned by §9.4 Test #2/#3 |
| 3 | Pop-order swap silent regression | §9.1 Test #8 records (level, x, z, typ) and asserts unpacked values, not just call-count |
| 4 | ActiveLoc-on-miss mutation regression | §9.1 Test #3 pre-populates sentinel; per `audit_full_method_against_ts.md` |
| 5 | Content branches flip post-activation | Stub doc-comment noted "check_chest_macro_gas proc early-returns on LOC_FIND=0"; activating real lookup may flip these branches. Smoke surface §13 binds via Java client per `smoke_test_server_handoff.md` |
| 6 | Stale stub comment elsewhere | `rg "LOC_FIND.*later S6\|LOC_FIND.*stub"` at close-time per `retire_deviation_grep_all_comments.md` |

## 13. Smoke surface (post-merge)

Content invocations of LOC_FIND that flip from "not found" → "found" at
activation. Pre-flight `rg "loc_find\b" /home/owner/Code/github.com/LostCityRS/Content/scripts/` at plan-write
to enumerate. Stub comment names one (`check_chest_macro_gas`); user
launches server per `smoke_test_server_handoff.md` and drives the
relevant flow to confirm.

**Smoke is binding** (per `cascade_theory_smoke_binding.md`) on
hit-arm activation — any content branch that newly takes the hit path
must produce TS-faithful behavior or surface an in-scope-stretch /
NAI-N+1 follow-up.

## 14. Memory hits

Cited in close-commit `Closes memory:` trailer per
`close_commit_memory_trailer.md`:

- `runescript_cadence.md` — full cadence routing
- `controller_preflight.md` — preflight performed before brainstorm
- `true_to_ts_gate.md` — every TS branch mapped to live goscape infra
- `compressed_cadence.md` — cadence sized to cross-package scope
- `enumerate_all_sites.md` — LocOps satisfiers enumerated at plan-write
- `audit_full_method_against_ts.md` — TS method audited line-by-line; miss-arm invariant pinned
- `defensive_gate_doc_comment_label.md` — labeled nil-checks
- `handler_pop_order_test_masking.md` — recorded-args test
- `retire_deviation_grep_all_comments.md` — stale stub comments swept at close
- `smoke_test_server_handoff.md` — user-launched server for content smoke
- `cascade_theory_smoke_binding.md` — smoke binds hit-arm activation
- `spec_ts_source_read.md` — design reads TS source verbatim, no analogy

## 15. Out of scope

- Refactoring `LOC_ADD`'s same-tile branch to call the new
  `LocOps.GetLoc` instead of iterating `LocsAtCoord`. The two have
  different semantics (LOC_ADD filters by layer, LOC_FIND by type);
  consolidation is not warranted and would introduce churn against the
  TS-faithful per-handler shape.
- A coord-keyed loc index. `Server.GetLoc`'s doc-comment notes "If
  profiling later shows hot zones, a coord-keyed map can replace the
  slice." That optimization is a separate concern decoupled from
  LOC_FIND's correctness.
- Content-side `loc_find` follow-ups beyond confirming the §13 smoke.
  Any divergence routed per `smoke_surfaces_adjacent_divergences.md`.
