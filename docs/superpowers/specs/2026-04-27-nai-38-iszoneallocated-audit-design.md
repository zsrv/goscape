# NAI-38 — Retire dead `script.WorldVars.IsZoneAllocated` interface method

**Cadence:** Compressed (per `compressed_cadence.md`). Combined spec+plan; no
separate plan doc; no formal review stage. Single deletion-only commit.

## Motivation

`WorldVars.IsZoneAllocated(level, x, z int) bool` was added on the
`pkg/script.WorldVars` interface during NAI-36-T7 ("PathingEntity.teleport
partial parity") to satisfy a then-anticipated script-handler-side teleport
plumbing. The handler plumbing landed instead inside `modules/world/` — both
`*Player.Teleport`-driven (`modules/world/player_script.go:265`) and
`*Npc.Teleport`-driven (`modules/world/npc_script.go:136`) call sites use
`*Server.IsZoneAllocated` directly, bypassing the `script.World` interface
entirely. NAI-37 close noted the interface method had no consumer; the
`dead_api_polish.md` audit deadline was set to "verify before NAI-39 or
remove."

Verified at HEAD `03d2255`:

- `pkg/script/state.go:82-86` — interface declaration on `WorldVars`.
- `modules/world/server_varp.go:110-123` — `worldVarsView.IsZoneAllocated`
  implementation; delegates to `*Server.IsZoneAllocated`.
- `pkg/script/handlers_vars_test.go:48` — `mockWorld.IsZoneAllocated` returns
  `true` (default-allocated for tests that don't care).
- `pkg/script/handlers_map_test.go` — `mapFindSquareWorld` and
  `mapBlockedWorld` embed `mockWorld` and inherit `IsZoneAllocated`
  transitively. No type defines its own override.
- **Production callers via the interface: zero.**
  `rg -n '\.IsZoneAllocated\(' --type go | grep -v '_test.go|world_zone.go|flagmap.go|server_varp.go'`
  returns only `npc_script.go:136` and `player_script.go:265`, both of which
  bind to `*Server.IsZoneAllocated` (concrete-receiver call), not to the
  interface method.

The concrete `*Server.IsZoneAllocated`
(`modules/world/world_zone.go:96-110`) and `(*FlagMap).IsZoneAllocated`
(`pkg/pathfinder/collision/flagmap.go:142`) are NOT dead and stay
untouched. Only the script-side interface route is fossilized speculation.

Per `dead_api_polish.md`: helpers shipped with zero consumers are caught at
sub-spec close, not deferred indefinitely. Per YAGNI: if a future
script-handler-only teleport-safety consumer surfaces, the interface method
can be re-added in one line.

## Tech stack

- **Go 1.26+** (per `go_version.md`).
- TS source: `Engine-TS` only (per `ts_source_canonical_path.md`). No TS
  changes; this is goscape-side dead-code retirement, not a port.
- HEAD baseline: `03d2255` (NAI-37 close).

## Scope

**In scope (all under `modules/world/` and `pkg/script/`):**

1. Delete `IsZoneAllocated` declaration from the `WorldVars` interface in
   `pkg/script/state.go:82-86` (5 lines: 4 doc comment + 1 signature).
2. Delete `worldVarsView.IsZoneAllocated` implementation in
   `modules/world/server_varp.go:110-123` (14 lines: 4 doc comment + 10
   method body, including the trailing blank line of the method block).
3. Delete `mockWorld.IsZoneAllocated` and the 4-line doc comment in
   `pkg/script/handlers_vars_test.go:44-48` (5 lines).

**Total: ~24 LOC deleted, 0 LOC added.**

**Explicitly out of scope:**

- `*Server.IsZoneAllocated` at `modules/world/world_zone.go:96-110` — STAYS.
  Two production callers (`npc_script.go:136`, `player_script.go:265`).
- `(*FlagMap).IsZoneAllocated` at
  `pkg/pathfinder/collision/flagmap.go:142` — STAYS. Underlying collision-
  layer query.
- All `pkg/pathfinder/collision/flagmap_test.go` tests — STAY. They test
  the `*FlagMap` method, not the script interface.
- The deviation comment at `modules/world/npc_script.go:103` referencing
  "D2 (unallocated-zone reject via IsZoneAllocated)" — STAYS. It refers to
  the still-live `*Server.IsZoneAllocated` consumer at line 136.
- Comments at `modules/world/player_script_test.go:662, 666, 695` — STAY.
  They refer to behavior implemented via `*Server.IsZoneAllocated`, not the
  interface method.

## Plan (single task)

**Task 1.** Delete the three sites in this exact order, then verify build
+ tests:

1. `pkg/script/state.go` — remove the `IsZoneAllocated` declaration plus
   its 4-line doc comment from the `WorldVars` interface block. The
   interface block's closing `}` at line 87 stays.
2. `modules/world/server_varp.go` — remove the entire
   `worldVarsView.IsZoneAllocated` method including its leading blank
   line and 4-line doc comment. End-of-file is line 124 (one trailing
   newline); after deletion the file's last function is `AnimMap`.
3. `pkg/script/handlers_vars_test.go` — remove the `IsZoneAllocated`
   method on `mockWorld` plus its 4-line `NAI-36-T7:` doc comment.

**Verification commands** (run after the three deletions):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

**Expected results:**

- `go build` clean. The `script.WorldVars` interface no longer declares
  `IsZoneAllocated`, and `worldVarsView` no longer implements it — they
  remain mutually consistent.
- `go vet` clean.
- All 23 packages green. `mapFindSquareWorld` and `mapBlockedWorld`
  inherit `mockWorld`'s field set unchanged; deletion of the (unused)
  inherited method does not affect their satisfaction of `WorldVars`
  because `WorldVars` no longer requires the method.

**Failure-mode catches** (per `controller_preflight.md`):

- If `go build` fails with `worldVarsView does not implement WorldVars`:
  one of the two interface-side / impl-side deletions was missed. Fix by
  re-checking both edits.
- If `go build` fails with `cannot use w (variable of type mockWorld)
  as WorldVars`: the mock-side delete was skipped, OR a hidden caller
  was missed. Re-grep `\.IsZoneAllocated\(` and inspect any new hit.
- If `go test ./...` surfaces a previously-passing test failing on a
  different signal: that test was relying on `IsZoneAllocated` indirectly
  via the interface; investigate (this is unexpected at HEAD).

## Tests

No new tests. No test fixtures change. The deletion is verified by:

1. **Build**: `go build` proves interface ↔ impl ↔ mock signatures stay
   consistent post-deletion.
2. **Existing test suite**: `go test ./...` proves no behavior pin
   transitively depended on the interface route.

The two deviation-pin tests at `modules/world/player_script_test.go`
(`TestPlayerTeleportToUnallocatedZone*`, names approximate) and the
NPC-side equivalent at `modules/world/npc_script_test.go:776-region`
exercise `*Server.IsZoneAllocated`, not the interface method. They stay
green unchanged.

## Acceptance criteria

- [ ] `pkg/script/state.go` no longer declares `IsZoneAllocated` on
      `WorldVars`.
- [ ] `modules/world/server_varp.go` no longer defines
      `worldVarsView.IsZoneAllocated`.
- [ ] `pkg/script/handlers_vars_test.go` no longer defines
      `mockWorld.IsZoneAllocated`.
- [ ] `*Server.IsZoneAllocated` at `modules/world/world_zone.go:96-110`
      is unchanged.
- [ ] `(*FlagMap).IsZoneAllocated` at
      `pkg/pathfinder/collision/flagmap.go:142` is unchanged.
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean.
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
      across all 23 packages.
- [ ] Single close-commit with `Closes memory:` trailer (per
      `close_commit_memory_trailer.md`) referencing
      NAI-37-D-deferred-IsZoneAllocated-audit.

## Risks / non-risks

**Non-risks:**

- No behavior change. The interface method had no production consumer; its
  removal is observably equivalent to leaving it in place.
- No TS-fidelity divergence. The TS `World.isZoneAllocated` query is
  consumed via `PathingEntity.teleport` → which we mirror via
  `*Server.IsZoneAllocated` direct calls in
  `modules/world/{npc,player}_script.go`.

**Risks:**

- **A grep miss for an interface-only consumer.** Mitigated: triple grep
  pattern (`\.IsZoneAllocated\(`, `IsZoneAllocated`, and method-receiver-
  agnostic). If `go build` fails post-deletion, the failure points
  directly at the missed consumer.

## Future re-add path (if needed)

If a script-handler-only teleport-safety consumer surfaces in a future
NAI sub-spec, re-introduce the interface method in three steps:

1. Add `IsZoneAllocated(level, x, z int) bool` back to `WorldVars` in
   `pkg/script/state.go`.
2. Re-add `worldVarsView.IsZoneAllocated` delegating to
   `s.IsZoneAllocated(...)` in `modules/world/server_varp.go`.
3. Add a return-stub or recorder method to `mockWorld` in
   `pkg/script/handlers_vars_test.go`.

Total restoration cost: ~20 LOC. The concrete `*Server.IsZoneAllocated`
needs no changes — it has been preserved precisely to keep this path
cheap.
