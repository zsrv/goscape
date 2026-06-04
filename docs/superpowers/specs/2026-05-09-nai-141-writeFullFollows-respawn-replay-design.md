# NAI-141 — `writeFullFollows` Respawn-loc replay branches (TS parity)

**Date:** 2026-05-09
**Status:** Brainstorm-approved spec; combined spec+plan doc per `compressed_cadence.md`.
**Cadence:** Compressed (~10–15 LOC production delta) — single end-of-impl reviewer on Sonnet; no two-stage review.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Origin:** User-reported bug (2026-05-09): "open Lumbridge Castle kitchen door → climb down trapdoor → climb back up → door appears both open and closed simultaneously; closing the open door makes the duplicate closed-variant disappear."
**TS source:** `LostCityRS/Engine-TS/src/engine/zone/Zone.ts:133-165` (`writeFullFollows`).

---

## §1 PRIMARY criteria

User-driven smoke at HEAD post-T2 must observe:

1. **Lumbridge Castle kitchen door state survives plane round-trip.** Steps:
   1. Spawn or walk to Lumbridge Castle ground floor (level 0).
   2. Open kitchen door (`change_loc` fires via `~open_and_close_door` proc).
   3. Walk to the trapdoor in the kitchen; climb down (level 0 → -1).
   4. Climb back up (level -1 → 0).
   5. **Pin:** the door at the original coord renders in exactly one state (whichever state the server holds — open OR closed, not both). No duplicate "phantom closed door" alongside the open variant.
2. **Generalises to any Respawn-loc state mutation across plane change.** Spec-bound goldens for the unit-test surface (§3.1) cover: Respawn+`IsChanged()` replays as `LOC_ADD_CHANGE`, Respawn+`!IsActive` replays as `LOC_DEL`. The Lumbridge kitchen door exercises the `IsChanged()` branch; the `!IsActive` branch is unit-test-only (no fresh-tutorial smoke path mutates a static via `loc_del`).
3. **No regression in NAI-140 §1 criteria 0-3** (quest-color cascade) and NAI-139 §1 criteria 0-6 (tutorial-completion cascade). Door interaction itself is unrelated to those cascades; this is a regression-fence pin.

**Non-criterion (informational only):**
- The dynamic open-variant LOC_ADD (Despawn-lifecycle, added by `~open_and_close_door`'s `loc_add` call) replays correctly today (existing branch). Smoke pin (1) verifies the closed-variant side; the open-variant side is already-green production behavior re-witnessed.

## §2 Architecture

### §2.1 Root cause

- TS `Engine-TS/src/engine/zone/Zone.ts:133-165` `writeFullFollows` iterates `getAllLocsUnsafe()` and emits one of three branches per loc:
  1. `lifecycle === DESPAWN && isActive` → `LocAddChange` (line 153-155)
  2. `lifecycle === RESPAWN && !isActive` → `LocDel` (line 157-159)
  3. `lifecycle === RESPAWN && isChanged()` → `LocAddChange` (line 161-163)
- Goscape `modules/world/player_zone.go:57-68` `Player.writeFullFollows` iterates `z.Locs` and emits **only** branch (1):
  ```go
  if loc.Lifecycle == entitypkg.LifecycleDespawn && loc.CheckLifecycle(currentTick) {
      // LOC_ADD_CHANGE
  }
  ```
  Branches (2) and (3) are absent. The function carries an explicit TODO at `:23-24`: "handle Respawn-lifecycle (static) loc branches once static loading from cache maps is wired up." Static loading **is** wired (see §2.2); the TODO is stale.

### §2.2 State persistence is in place

The replay-side gap is the only missing piece. All upstream state-mutation paths that produce Respawn locs in the {`!IsActive`, `IsChanged`} states are already wired:

| State path | Source | Current behavior |
|---|---|---|
| Static load at boot | `modules/world/server.go:333-344` `populateStaticLocsIntoZones` | Calls `Zone.AddStaticLoc(loc)` for every parsed gamemap static; `IsActive=true`, `Lifecycle=Respawn`. |
| `change_loc` script | `pkg/script/handlers_loc.go:293-312` `handleLocChange` → `LocOps.ChangeLoc` | Mutates `Loc.CurrentInfo` (type/shape/angle), preserves `BaseInfo`, sets `IsActive=true`, `IsChanged()` returns true. Emits per-tick `LOC_ADD_CHANGE` event. |
| `loc_del` script | `pkg/script/handlers_loc.go:335-347` `handleLocDel` → `LocOps.RemoveLoc` | For Respawn locs, sets `IsActive=false` while keeping the loc in `z.Locs` (`pkg/zone/zone.go:191-211`). Emits per-tick `LOC_DEL` event. |
| Auto-revert | `modules/world/loc_turn.go:39-62` `Server.RevertLoc` | Restores `BaseInfo`, `IsActive=true`, `IsChanged()` returns false. Emits `LOC_ADD_CHANGE` event. |

The per-tick events flow through `writePartialFollows` (works correctly today). The full-state replay flows through `writeFullFollows` (the bug).

### §2.3 Symptom causation

After `change_loc(door, inviswall, 3)` fires on the kitchen door:
- Static Respawn loc at door's coord: `CurrentInfo=inviswall`, `BaseInfo=closed_door`, `IsActive=true`, `IsChanged=true`.
- Adjacent Despawn loc: `Type=opened_door`, `IsActive=true`.

Player descends trapdoor → REBUILD_NORMAL fires (level change always rebuilds) → client zone state reset to default for ground-floor zones. Player ascends → REBUILD_GETMAPS round-trip → `Player.rebuildZones` (`modules/world/player.go:701-722`) → `writeFullFollows` per zone in 7×7.

In the kitchen-door zone, `writeFullFollows` iterates `z.Locs`:
- Static Respawn `inviswall` (changed): branch (3) **missing** → no replay → client renders `BaseInfo=closed_door` from cache map data ⇒ **closed door visible**.
- Dynamic Despawn `opened_door`: branch (1) hit → `LOC_ADD_CHANGE` replayed ⇒ **open door visible**.

Result: both visible. User-described symptom matches exactly.

When the user clicks the visible open door, `~open_and_close_door` runs the close path:
- `loc_del` on Despawn `opened_door` → `LOC_DEL` event → client removes open variant.
- `loc_change` on Respawn loc back to closed-door type (or revert) → `LOC_ADD_CHANGE` event with closed-door type → client re-paints the static coord with the canonical closed-door type. The "phantom closed door" was the cache-default render; the explicit `LOC_ADD_CHANGE` overwrites the same tile with the correct value, and the artifact resolves into a single closed door. Matches user observation.

### §2.4 Fix

**`modules/world/player_zone.go:57-68`** — extend the loc loop to mirror TS three-branch shape. Diff sketch:

```go
// Before (current HEAD):
for _, loc := range z.Locs {
    if loc.LastLifecycleTick == currentTick {
        continue
    }
    if loc.Lifecycle == entitypkg.LifecycleDespawn && loc.CheckLifecycle(currentTick) {
        ensureHeader()
        pb := packet.NewPacket(nil)
        rsbuf.EncodeLocAddChange(pb, coordgrid.PackZoneCoord(loc.X, loc.Z),
            loc.Shape(), loc.Angle(), loc.Type())
        p.writeOut(gameserver.OpLocAddChange, pb.Bytes())
    }
}

// After:
for _, loc := range z.Locs {
    if loc.LastLifecycleTick == currentTick {
        continue
    }
    switch {
    case loc.Lifecycle == entitypkg.LifecycleDespawn && loc.IsActive:
        ensureHeader()
        pb := packet.NewPacket(nil)
        rsbuf.EncodeLocAddChange(pb, coordgrid.PackZoneCoord(loc.X, loc.Z),
            loc.Shape(), loc.Angle(), loc.Type())
        p.writeOut(gameserver.OpLocAddChange, pb.Bytes())
    case loc.Lifecycle == entitypkg.LifecycleRespawn && !loc.IsActive:
        ensureHeader()
        pb := packet.NewPacket(nil)
        rsbuf.EncodeLocDel(pb, coordgrid.PackZoneCoord(loc.X, loc.Z),
            loc.Shape(), loc.Angle())
        p.writeOut(gameserver.OpLocDel, pb.Bytes())
    case loc.Lifecycle == entitypkg.LifecycleRespawn && loc.IsChanged():
        ensureHeader()
        pb := packet.NewPacket(nil)
        rsbuf.EncodeLocAddChange(pb, coordgrid.PackZoneCoord(loc.X, loc.Z),
            loc.Shape(), loc.Angle(), loc.Type())
        p.writeOut(gameserver.OpLocAddChange, pb.Bytes())
    }
}
```

Three observable changes vs HEAD:

1. **Despawn-active gate switches from `CheckLifecycle(currentTick)` to `IsActive`.** Per `pkg/zone/zone.go:158-172` (`AddLoc`), `IsActive` is the field TS reads (`pkg/entity/loc.go:16` doc-comment confirms: "managed by pkg/zone Zone methods… mirrors TS Zone.ts isActive writes"). For Despawn locs the two are observably equivalent today — `AddLoc` sets `IsActive=true` and `CheckLifecycle` returns true while `LifecycleTick > currentTick`; `RemoveLoc` sets `IsActive=false` but also clears the loc from `z.Locs`, so the iterator never sees the post-remove state. The switch to `IsActive` is for TS-faithfulness and to share the same gate semantics across all three branches. Pinned by re-running existing despawn-replay test (T1.0) post-fix.
2. **Respawn-`!IsActive` branch added** → `LOC_DEL` emitted for `loc_del`'d statics on full-replay.
3. **Respawn-`IsChanged()` branch added** → `LOC_ADD_CHANGE` (with current type/shape/angle) emitted for `change_loc`'d statics on full-replay.

**Stale TODO retired:** `modules/world/player_zone.go:23-24` doc-comment removed (replaced by the actual implementation). The "TODO(beyond-4b)" tag tracking is purely descriptive; no separate bookkeeping retires.

### §2.5 Obj-side parity audit

TS line 142-146 emits `ObjAdd` for both `Despawn+isActive` and `Respawn+isActive` (parallel branches, same effect). Goscape line 41-55 uses a single unified gate `obj.CheckLifecycle(currentTick)`. Equivalence argument:

- `pkg/entity/entity.go:33-44` `CheckLifecycle`:
  - `LifecycleForever` → always true ⇒ matches TS "static obj always isActive=true".
  - `LifecycleRespawn` → `LifecycleTick < tick` ⇒ alive after respawn time. TS `isActive` is set true by `World.addObj` after the same respawn schedule.
  - `LifecycleDespawn` → `LifecycleTick > tick` ⇒ alive before despawn time. TS `isActive` is set true on add, false on remove.
- `pkg/entity/obj.go` has **no** `IsActive` field; obj liveness is derived from tick math. Loc carries `IsActive` because Loc-state can transition without rescheduling LifecycleTick (e.g., `loc_change` keeps the loc alive but mutates CurrentInfo); Obj has no parallel mutation surface, so the tick-based gate is sufficient.

**Decision:** No obj-side production change. Pin equivalence with one unit test (T1.4) covering both TS-branch shapes (Despawn-active obj at zone replay AND Respawn-active obj at zone replay both emit `ObjAdd`). If a future TS divergence surfaces (e.g., Respawn obj that's in non-active phase but tick-math-equivalent to active), pivot to porting the explicit two-branch shape.

### §2.6 Risks

- **R1 — `IsActive` vs `CheckLifecycle` divergence on Despawn locs.** Today's invariant: `AddLoc` writes both `IsActive=true` and `LifecycleTick=current+duration`; `RemoveLoc` writes `IsActive=false` AND removes from `z.Locs`. The change to `IsActive` gating is observably equivalent **only** while no code path leaves a Despawn loc in `z.Locs` with `IsActive=false`. Spec mitigates by:
  - T1.0 existing-behavior pin: Despawn+active replay still produces `LOC_ADD_CHANGE` post-fix.
  - T1.5 new pin: Despawn loc with `IsActive=false` (synthesized in test by direct field write — not reachable via production paths today) is **NOT** replayed. Test documents the invariant rather than the production path.
- **R2 — Auto-revert race.** `RevertLoc` (`loc_turn.go:39-62`) emits `ChangeLoc` event AND calls `SetLifeCycle(-1, currentTick, nil)`. Post-revert: `IsActive=true`, `IsChanged()` returns false. **No replay branch hits** — the loc is unchanged static, equivalent to a fresh boot. Confirmed correct by T1.6 (post-revert no replay).
- **R3 — `IsChanged()` semantics.** Per `pkg/entity/loc.go` audit at plan-author time: confirm `IsChanged()` returns true iff `CurrentInfo != BaseInfo`, and `Revert()` restores `CurrentInfo := BaseInfo`. If `IsChanged()` has additional preconditions (e.g., `IsActive` requirement), branch-3 condition may need adjustment. Plan-author MUST `Read pkg/entity/loc.go` and pin the `IsChanged()` body exactly before writing T2 production code.
- **R4 — Open NAI-84 deviations are orthogonal but adjacent.** `NAI-84-D-R-D2` (rebuildScenery activeZones dual-write) and `NAI-84-D-R-D3` (per-zone-change rebuildZones call site) govern *when* `writeFullFollows` fires. NAI-141 governs *what* it emits. Both can be true; this sub-spec does not touch NAI-84's deviation surface. If smoke (1) fails post-T2 with a different shape (e.g., door state correct after trapdoor round-trip but stale on adjacent plain-walking zone transitions), escalate to NAI-84-D-R-D3 as a follow-up sub-spec.

### §2.7 Deviations introduced

None. The fix closes a documented TS-asymmetry (the stale TODO in goscape source). All three branches mirror TS line-for-line. No new deviations.

**Closes existing tracker entry:** the writeFullFollows TODO at `modules/world/player_zone.go:23-24` is retired by this sub-spec. Document in close commit body.

## §3 Test strategy

### §3.1 Unit tests — `modules/world/player_zone_test.go` (extend or create)

Pre-flight check: confirm whether `player_zone_test.go` already exists at HEAD (`ls modules/world/player_zone_test.go`). If yes, append; if no, create. Either way the package is `world`.

| # | Name | Coverage |
|---|------|----------|
| T1.0 | `TestWriteFullFollows_DespawnActiveLoc_EmitsLocAddChange` | **Existing-behavior fence.** Add Despawn loc via `Zone.AddLoc`, advance tick, call `writeFullFollows`. Assert `OpLocAddChange` opcode + payload bytes for the loc. Fails if R1 invariant breaks. |
| T1.1 | `TestWriteFullFollows_RespawnInactiveLoc_EmitsLocDel` | **NEW PRIMARY.** Add static via `Zone.AddStaticLoc`, then `Zone.RemoveLoc` (sets `IsActive=false`, keeps loc in `z.Locs` per `zone.go:191-211`). **Advance tick** before replay so the `LastLifecycleTick == currentTick` guard doesn't skip the loc. Call `writeFullFollows` at the new tick. Assert `OpLocDel` opcode + payload bytes (`coord, shape, angle`). |
| T1.2 | `TestWriteFullFollows_RespawnChangedLoc_EmitsLocAddChange` | **NEW PRIMARY.** Add static via `Zone.AddStaticLoc`. Mutate `Loc.CurrentInfo` to a different type (via the same `LocOps.ChangeLoc` codepath the script handler uses, OR direct field mutation if the test fixture lacks a `LocOps` impl — plan-author chooses based on fixture availability). **Advance tick** before replay (same rationale as T1.1). Call `writeFullFollows` at the new tick. Assert `OpLocAddChange` opcode + payload with the **new** type/shape/angle. |
| T1.3 | `TestWriteFullFollows_RespawnUntouchedStatic_NoReplay` | **Negative pin.** Add static via `Zone.AddStaticLoc`; do NOT mutate. Call `writeFullFollows`. Assert no `OpLocAddChange` / `OpLocDel` for that loc. (Static-untouched locs are delivered via mapsquare download, not zone replay — see `zone.go:146-153`.) |
| T1.4 | `TestWriteFullFollows_ObjAdd_DespawnAndRespawnBothEmit` | **Obj-side equivalence pin (§2.5).** Two sub-cases in one test (or two tests; plan-author picks): (a) Despawn obj via `Zone.AddObj` + advance tick → `OpObjAdd` replays; (b) `LifecycleRespawn` obj constructed with `LifecycleTick=0`, current tick > 0 → `OpObjAdd` replays. Documents that the unified `CheckLifecycle` gate covers both TS branches. |
| T1.5 | `TestWriteFullFollows_DespawnInactiveLoc_NoReplay` | **R1 invariant pin.** Construct Despawn loc, manually set `IsActive=false`, manually leave it in `z.Locs` (direct field writes — bypassing production add/remove). Call `writeFullFollows`. Assert no replay for the loc. Documents the invariant rather than a production path. |
| T1.6 | `TestWriteFullFollows_PostRevertRespawnLoc_NoReplay` | **Revert path pin.** Add static, mutate via `ChangeLoc`, call `Loc.Revert()` directly to simulate `RevertLoc`'s state-side effect. Confirm `IsActive=true`, `IsChanged()=false`, no replay. |
| T1.7 | `TestWriteFullFollows_LocLastLifecycleTick_SkipsReplay` | **Per-tick skip pin (existing condition preserved).** Loc transitioned this tick (`LastLifecycleTick == currentTick`) is skipped regardless of branch. Cover the loc-side equivalent of the existing obj-side check. |

### §3.2 Integration test — none

The smoke (§4) is the integration-level binding. No need for an integration test in the test suite — `player_zone_test.go` exercises the full encoder path via `rsbuf.EncodeLocAddChange` / `EncodeLocDel`, and the wire-write path is one `writeOut` call deep.

### §3.3 Existing tests preserved

- All existing `pkg/zone/zone_test.go` tests stay green (this sub-spec changes no `pkg/zone` code).
- All existing `modules/world/loc_turn_test.go` tests stay green.
- `modules/world/server_zone_static_test.go` (the `populateStaticLocsIntoZones` test) stays green.
- Per `enumerate_all_sites.md`: plan-author MUST `rg "writeFullFollows" --type go` at HEAD pre-dispatch and re-grep post-T2 to confirm no other call sites or tests reference the old single-branch shape.

### §3.4 TDD sequencing (compressed cadence)

- **T1 (red)**: add T1.0–T1.7 to `modules/world/player_zone_test.go`. T1.0 may be green at HEAD (pre-fix); T1.1, T1.2, T1.4 (Respawn case), T1.5, T1.6, T1.7 fail. Verify red. Commit: `test(player_zone): NAI-141 T1 red — writeFullFollows respawn-replay branches fail`.
- **T2 (green)**: edit `modules/world/player_zone.go:25-69` per §2.4 sketch; remove stale TODO at `:23-24`. All 7+ tests green; full repo `go test ./...` and `go vet ./...` clean. Commit: `feat(player_zone): NAI-141 T2 green — port writeFullFollows respawn-loc replay branches`.
- **Reviewer (Sonnet)**: end-of-impl `superpowers:code-reviewer` agent on Sonnet (per `superpowers_code_reviewer_model.md`). Pass requires no Critical/Important issues.
- **T3 (close)**: smoke handoff per §4.

### §3.5 Pre-flight verification (controller pre-dispatch, per `controller_preflight.md`)

Before plan-author dispatch, controller verifies at HEAD `fcec331` (NAI-140 close):
- `rg -n "func \(p \*Player\) writeFullFollows" modules/world/player_zone.go` → confirm signature + line `:25`; confirm three-branch absence.
- `rg -n "TODO\(beyond-4b\)" modules/world/player_zone.go` → confirm stale TODO at `:23-24` still present.
- `Read pkg/entity/loc.go` body of `IsChanged()` and `Revert()` → confirm contract per R3.
- `Read pkg/entity/loc.go:1-50` `IsActive` field declaration → confirm doc-comment `:16`.
- `Read Engine-TS/src/engine/zone/Zone.ts:133-165` → confirm canonical three-branch shape unchanged at TS HEAD.
- `ls modules/world/player_zone_test.go` → if absent, T1 creates it.
- `rg -n "writeFullFollows" modules/world/ pkg/` → enumerate call sites for §3.3 invariant.
- `Read pkg/zone/zone.go:191-211` `RemoveLoc` body → confirm Respawn-loc deletion preserves `z.Locs` membership and only flips `IsActive` (T1.1 setup correctness).

## §4 Smoke handoff

**Binding per `cascade_theory_smoke_binding.md`:** user re-runs the bug-reproduction flow:

1. Spawn or walk to Lumbridge Castle ground floor (level 0, near coord 3209,3216).
2. Open the kitchen door (the door dividing the kitchen from the ground-floor great-hall).
3. Walk to the trapdoor inside the kitchen; click "Climb-down". Player lands on level -1 (basement).
4. Walk to the trapdoor in the basement; click "Climb-up". Player returns to level 0.
5. **PRIMARY pin (criterion 1)**: the kitchen door at the original coord renders in **exactly one state**. No phantom duplicate.
6. **Regression check**: NAI-140 §1 + NAI-139 §1 still PASS (no quest-color or tutorial-cascade regression).
7. No `script execute error`, opcode-dispatch warnings, panics, or stack traces during the round-trip.

**Per `smoke_test_server_handoff.md`**: server must be user-launched (Java client driving). Controller's role is to provide the build artifact + a paste-ready resume prompt.

**Adjacent consumers not in smoke path** but covered by theory of fix:
- Any door across any region after a plane round-trip (via trapdoor, ladder, stairs).
- Statics removed via `loc_del` script (covered by T1.1 unit; smoke not bound).
- Statics changed permanently (no auto-revert duration) and persisted across plane changes.

Future smoke surfacing a divergence in any of these adjacent paths routes as separate sub-spec, NOT in-scope-stretch (per `smoke_surfaces_adjacent_divergences.md`).

## §5 Implementation plan (compressed; combined into spec)

### §5.1 Files to modify

- `modules/world/player_zone.go` — extend the `z.Locs` loop in `writeFullFollows` per §2.4 sketch; retire stale TODO at `:23-24`. Net delta: ~12 LOC added (two new `case` arms), 2 LOC removed (TODO comment block).
- `modules/world/player_zone_test.go` — append T1.0–T1.7 (or create file if absent). Net delta: ~150 LOC of test code.

### §5.2 Files to create

- `modules/world/player_zone_test.go` only if it does not exist at HEAD. Otherwise append.

### §5.3 Files NOT to modify

- `pkg/zone/zone.go` — Loc mutator state surface is already correct.
- `pkg/entity/loc.go` — `IsActive` / `IsChanged()` / `Revert()` contracts are already correct.
- `pkg/script/handlers_loc.go` — script handlers already mutate state correctly.
- `modules/world/loc_turn.go` — `RevertLoc` already emits the correct event-side action.
- `modules/world/server.go` — `populateStaticLocsIntoZones` already wires statics into zones at boot.

### §5.4 Build sequence

1. **T1 red** — add tests T1.0–T1.7. Verify `go test ./modules/world/... -run TestWriteFullFollows_` shows the new tests fail (T1.1, T1.2, T1.5, T1.6 expected red; T1.0, T1.7 may be green; T1.4 obj cases green). Commit: `test(player_zone): NAI-141 T1 red — writeFullFollows respawn-replay branches fail`.
2. **T2 green** — edit `player_zone.go` per §2.4. Verify all green + `go test ./...` + `go vet ./...` clean. Commit: `feat(player_zone): NAI-141 T2 green — port writeFullFollows respawn-loc replay branches`.
3. **Reviewer (Sonnet)** — dispatch `superpowers:code-reviewer` agent (model: sonnet). Address Critical/Important only.
4. **Close** — smoke handoff per §4. On smoke PASS, commit `chore(close): NAI-141 — writeFullFollows respawn-replay; door-state survives plane round-trip`. Update `nai_followups.md`. Save memory entry per §6.

### §5.5 Implementer notes

- **Mirror TS branch shape exactly.** Keep the three-arm `switch` (or three sequential `if` blocks) in TS line order: Despawn+active first, Respawn+!active second, Respawn+changed third. Per `dispatch_order_audit_blind_spot.md`, dispatch order matters even if the branches are independent.
- **Use `loc.IsActive` not `loc.CheckLifecycle(currentTick)`** for all three branches. Per `pkg/entity/loc.go:16` doc-comment, `IsActive` is the canonical TS-mirror field.
- **Use `loc.IsChanged()` directly.** Plan-author confirms exact body at pre-flight; if signature is `func (l *Loc) IsChanged() bool { return l.CurrentInfo != l.BaseInfo }` (or equivalent), call it as written.
- **Retain the `loc.LastLifecycleTick == currentTick` guard** at the top of the loop. This filters locs that transitioned this tick (covered by the per-tick event stream, not the replay).
- **Encoder reuse**: `rsbuf.EncodeLocAddChange` and `rsbuf.EncodeLocDel` already exist (used by `pkg/zone/zone.go` mutators and the existing replay branch). No new encoder. Per `plan_grep_helper_patterns.md`: confirm via `rg "rsbuf.EncodeLocDel" pkg/ modules/` at pre-flight.
- **Test fixture pattern**: per `test_fixture_view_parity.md`, ensure any test that constructs a Player or Server uses the canonical `newTestServer` / `newTestPlayer` helpers if the test reaches them. T1.* tests can operate on a `*zone.Zone` + a minimal `*Player` stub if `writeFullFollows` doesn't require full server-side wiring; plan-author chooses based on what `writeOut` requires.
- **Skip pin format**: per `skip_pin_full_struct_capture.md`, if any test asserts a struct value verbatim (e.g., for the encoded byte payload), capture the value via `%+v` from a passing run rather than infer fields.

## §6 Closure criteria

- All tests green; `go test ./... && go vet ./...` clean.
- Reviewer (Sonnet) verdict: no Critical/Important issues.
- Smoke §4 PRIMARY criterion 1 met (door state survives plane round-trip without phantom duplicate); criterion 3 regression-fence holds.
- Memory entries written:
  - **Project memory (replaces if any older writeFullFollows-related entry exists)**: pin the three-branch parity invariant + the `IsActive` vs `CheckLifecycle` distinction (loc-side uses field, obj-side uses tick-math).
  - **`nai_followups.md`**: NAI-141 close entry; retire the stale `modules/world/player_zone.go:23-24` TODO note.
- Close commit body includes `Closes memory:` trailer per `close_commit_memory_trailer.md`.
- Cascade attribution: NAI-141 closes the user-reported door-duplicate-after-plane-change bug. No new cascade-blockers expected (the fix is universal across all `change_loc`/`loc_del`-mutated statics replayed via `writeFullFollows`). Adjacent NAI-84-D-R-D2 / R-D3 deviations remain open as separate follow-ups.
