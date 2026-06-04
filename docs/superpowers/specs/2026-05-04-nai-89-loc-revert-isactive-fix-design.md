# NAI-89 — Loc-revert IsActive fix + NAI-88 probe retire

**Status:** Spec
**Date:** 2026-05-04
**Predecessor:** NAI-88 (Stage 1 lifecycle-revert probe scaffold; closed at HEAD `0e6d83c`)
**Tech stack:** Go 1.26+. `pkg/zone`, `modules/world`, `pkg/entity`. No new deps.

## 1. Problem

NAI-88 Stage 1 installed probes around the loc lifecycle dispatch and a smoke run discriminated the bug:

- At tick 43 in the smoke trace, the changed Tutorial Island door at `(3098, 3107, level=0, type=83 inviswall)` had `lifecycle=RESPAWN`, `is_changed=true`, `is_active=false`.
- `Server.turnLoc` (modules/world/loc_turn.go:35-48) dispatched `AddLoc` (case-3: `RESPAWN && !IsActive`) instead of `RevertLoc` (case-2: `RESPAWN && IsChanged() && IsActive`).
- The P3 `revert_loc entry` probe never fired.

## 2. Root cause

The bug is **not** an over-restriction in the goscape switch. TS `Loc.turn` (Engine-TS/src/engine/entity/Loc.ts:54-74, line 60 specifically) requires `isActive` on the revert branch, identical to goscape's case-2. The goscape switch shape is TS-faithful.

The bug is that **goscape's `IsActive` field is `false` when TS's would be `true`**. TS sets `isActive` in three places inside `pkg/zone/Zone.ts` (Engine-TS/src/engine/zone/Zone.ts):

| TS site | Line | Effect |
|---|---|---|
| `Zone.addStaticLoc` | 208 | `loc.isActive = true` (static map locs at world init) |
| `Zone.addLoc` | 226 | `loc.isActive = true` |
| `Zone.changeLoc` | 232 | `loc.isActive = true` (forces inactive→active even if loc was inactive) |
| `Zone.removeLoc` | 254 | `loc.isActive = false` |

Goscape only writes `IsActive` in Server methods, and only at two of the four TS sites:

| Goscape site | Status |
|---|---|
| `Server.AddLoc` (modules/world/world_zone.go:25) | ✓ writes `IsActive = true` |
| `Server.RemoveLoc` (modules/world/world_zone.go:102) | ✓ writes `IsActive = false` |
| `populateStaticLocsIntoZones` → `Zone.AddStaticLoc` (server.go:316-321 → pkg/zone/zone.go:150) | ✗ **never sets IsActive=true** |
| `Server.ChangeLoc` (modules/world/world_zone.go:40-85) | ✗ **never sets IsActive=true** |

For the smoke door:
1. Loaded as a static map loc → `Zone.AddStaticLoc` runs, but `IsActive` stays at the zero-value `false`.
2. A `change_loc` script call runs `Server.ChangeLoc` → `IsActive` still not written.
3. At the revert tick, `loc.IsActive == false`, so case-2 (revert) cannot match. Case-3 (`!IsActive` → AddLoc) fires instead, re-adding the changed-info loc rather than reverting it to base.

## 3. Fix strategy

**Consolidate all `IsActive` writes into `pkg/zone` loc methods (TS-faithful site).** This was chosen over patching the two missing writes in their existing `Server` sites because:

- TS-faithful site eliminates the deviation, simplifying future audits.
- Goscape's existing convention ("set by Server.AddLoc / Server.RemoveLoc", per `pkg/entity/loc.go:16` doc-comment) is itself a pre-NAI-88 artifact that the static-loc loader silently violates.
- `pkg/zone` already mutates loc state (LocAddChange queueing, list-membership updates), so writing `IsActive` there is not a new layering violation.
- Smaller surface: one ownership site for IsActive lifecycle (the four Zone methods) vs. four scattered writes across `pkg/zone`/`modules/world`.

## 4. Bundles

The work decomposes into three independent-ish bundles. Bundle 1 ships the bug fix; Bundle 2 retires the NAI-88 probe scaffold; Bundle 3 hands off smoke to the user.

### Bundle 1 — IsActive consolidation in pkg/zone

Net diff: 4 zone-method additions, 2 Server-method deletions, 1 entity doc-comment update.

| Change | File:Line | Before | After |
|---|---|---|---|
| Add `loc.IsActive = true` after the append | `pkg/zone/zone.go AddStaticLoc` (~line 150-152) | append only | append + IsActive write |
| Add `loc.IsActive = true` (top of method, before list/event work) | `pkg/zone/zone.go AddLoc` (~line 157-170) | no IsActive write | top-of-method IsActive write |
| Add `loc.IsActive = true` (top of method) | `pkg/zone/zone.go ChangeLoc` (~line 174-184) | no IsActive write | top-of-method IsActive write |
| Add `loc.IsActive = false` (after the unlink/event work, mirroring TS Zone.ts:254 placement after queued-event clear) | `pkg/zone/zone.go RemoveLoc` (~line 188-207) | no IsActive write | trailing IsActive write |
| Delete `loc.IsActive = true` line | `modules/world/world_zone.go AddLoc` (line 25) | present | removed |
| Delete `loc.IsActive = false` line | `modules/world/world_zone.go RemoveLoc` (line 102) | present | removed |
| Update doc-comment | `pkg/entity/loc.go:16` | `// true after Server.AddLoc, false after Server.RemoveLoc` | `// managed by pkg/zone loc methods (AddStaticLoc/AddLoc/ChangeLoc/RemoveLoc); mirrors TS Zone.ts isActive writes` |

Each Server-method comment that references `Sets loc.IsActive=true` (world_zone.go:13, etc.) updates to point at the Zone method.

### Bundle 2 — NAI-88 probe retire

Strict revert. 7 probe-emit blocks deleted, 1 struct doc-comment block stripped (8 total NAI-88 marker sites); `locObjTracker` constructor reverted to no-args; 6 call sites updated.

**Probe-emit blocks to delete** (7 sites, each preceded by `// NAI-88 probe; remove at Stage 2 close`):
- `modules/world/loc_turn.go:16-31` — P2 `turn_loc entry`
- `modules/world/loc_turn.go:56-66` — P3 `revert_loc entry`
- `modules/world/tick.go:483` block — P1 `process_zones lifecycle iter`
- `modules/world/tick.go:493` block — second tick.go probe
- `modules/world/world_zone.go:61-79` — P4 `change_loc setlifecycle` decision (the entire `armRegister` debug computation + emit; the production `armRegister` boolean used for branching is preserved by inlining)
- `modules/world/loc_tracker.go:54-61` — P5 `tracker register`
- `modules/world/loc_tracker.go:72-85` — P6 `tracker unregister`

**Doc-comment block to strip:**
- `modules/world/loc_tracker.go:23-29` — struct doc-comment about probe fields (also bears the NAI-88 marker)

**locObjTracker constructor revert:**
- Drop fields: `log *slog.Logger`, `nodeDebug bool`
- Revert signature: `newLocObjTracker() *locObjTracker`
- Drop body init for the dropped fields
- Drop imports: `fmt`, `log/slog`, `runtime` (verify all three are now unused)

**Call site updates (6 total):**
- `modules/world/server.go:167` — `newLocObjTracker(logger, cfg.NodeDebug)` → `newLocObjTracker()`
- `modules/world/server_test.go:318` — `newLocObjTracker(nil, false)` → `newLocObjTracker()`
- `modules/world/loc_tracker_test.go:10,23,37,51` — same revert × 4

**Doc-comment retire:**
- Strip the "NAI-88 Stage 1" provenance from `locObjTracker` struct comment
- Strip the "NAI-88 probe; remove at Stage 2 close" markers (one per emit block; deletion of the block subsumes this)

**Verification gates:**
- `rg "NAI-88 probe; remove at Stage 2 close" modules/` returns 0
- `rg "nai88" modules/` returns 0
- `rg "NAI-88" modules/` returns 0 (Stage 1 commit messages and plan doc still reference NAI-88; that's expected)
- `go build ./...` green
- `go test ./...` green

### Bundle 3 — Smoke handoff

User-launched smoke per `smoke_test_server_handoff.md`. Two checks:

**Check 1 — door revert:** Walk to Tutorial Island door at `(3098, 3107, level=0)`. Open it (script invokes `change_loc`). Wait through the script-encoded duration. Confirm the door reverts to its closed (baseInfo) state in the client viewport.

**Check 2 — Tutorial Island chat suppression awareness:** Per `java_client_coord_chat_suppression.md`, do **not** rely on chat for confirmation inside the chat-suppressed coord box. Visual confirmation (door rendering, animation) is the gate.

If smoke surfaces an adjacent untracked divergence (per `smoke_surfaces_adjacent_divergences.md`), routing rule is pre-stated: ≤30 LOC fix → in-scope NAI-89 stretch; else NAI-90.

## 5. Test strategy

### New TS-faithfulness tests in pkg/zone (Bundle 1)

| Test | Asserts |
|---|---|
| `TestZoneAddStaticLocSetsIsActive` | After `z.AddStaticLoc(loc)`, `loc.IsActive == true`. |
| `TestZoneAddLocSetsIsActive` | After `z.AddLoc(loc)`, `loc.IsActive == true`. |
| `TestZoneChangeLocSetsIsActiveWhenInactive` | Inactive loc → `z.ChangeLoc(loc)` → `loc.IsActive == true`. Pins TS Zone.ts:231 comment "If a loc is inactive, it should be set to active when we call a change". |
| `TestZoneChangeLocPreservesActive` | Active loc → `z.ChangeLoc(loc)` → still active. |
| `TestZoneRemoveLocSetsInactive` | `z.RemoveLoc(loc)` → `loc.IsActive == false`. |

### New regression-pin in modules/world (Bundle 1)

`TestTurnLocRevertChangedStaticMapLoc` — the smoke-equivalent unit test that catches the NAI-88 bug at HEAD without the IsActive fix:

1. Build a static loc via `z.AddStaticLoc(loc)` (mimic `populateStaticLocsIntoZones`). Assert `loc.IsActive == true`.
2. Call `s.ChangeLoc(loc, newType, ..., duration=5)`. Assert `loc.IsActive == true && loc.IsChanged() == true`.
3. Advance 5 ticks (per the project's tick-loop test fixture). 
4. Assert post-tick: `loc.IsChanged() == false` (revert ran), `loc.IsActive == true` (TS Zone.changeLoc semantics held through revert), `loc.LifecycleTick == -1` (untracked).

This test fails on HEAD `0e6d83c` (case-3 AddLoc fires; loc remains changed). It passes after Bundle 1.

### Existing-test audit

| Test | Action |
|---|---|
| `pkg/entity/loc_test.go:144 TestLocIsActiveDefaultFalse` | **Keep.** Field zero value is still `false`; the change is in *which methods write*. |
| `modules/world/world_zone_test.go:77 TestAddLocSetsIsActiveTrue` | **Keep.** `Server.AddLoc` still produces `IsActive=true` (via the called `Zone.AddLoc` write). The test assertion is integration-level. |
| `modules/world/world_zone_test.go:142 TestRemoveLocSetsIsActiveFalse` | **Keep.** Same logic for `Server.RemoveLoc`. |
| `modules/world/loc_turn_test.go` (all) | **Keep, re-run.** Setup uses `s.AddLoc`; the IsActive write happens via Zone.AddLoc now but the observable state is unchanged. |
| `pkg/zone/zone_test.go` ad-hoc `loc.IsActive = true` writes (if any) | Audit at Bundle 1 plan-write; drop if redundant after the new Zone-method writes; keep if they document distinct intent. |

### Probe-retire smoke (Bundle 2)

- `go build ./...` green.
- `go test ./...` green.
- 0 grep hits on the three NAI-88-marker patterns.

## 6. Risks & mitigations

| Risk | Mitigation |
|---|---|
| A non-Server caller reads `loc.IsActive` between the Server-method entry and the Zone-method call (relying on the old write site's timing) | Bundle 1 plan includes a grep audit of every `loc.IsActive` reader to confirm no such caller exists. The `IsActive` field is read only by `turnLoc` switch arms, `Server.ChangeLoc`/`RemoveLoc` early-returns, and `world_zone_test.go` assertions — none span the window. |
| `pkg/zone/zone_test.go` has manual `loc.IsActive` writes that become redundant or misleading | Audit during Bundle 1 task-write; keep documentary writes, drop pure-duplication. |
| Tutorial Island chat suppression masks smoke result | `java_client_coord_chat_suppression.md` already memorialized; smoke prompt is visual-confirmation only inside the suppressed coord box. |
| Probe retire breaks an unrelated test that grew to depend on the constructor signature post-NAI-88 | Pre-flight grep confirmed all 6 `newLocObjTracker` call sites are NAI-88 era; no other consumers. |
| Adjacent untracked TS-fidelity divergences surface during Bundle 3 smoke | Routing pre-stated (§4 Bundle 3): ≤30 LOC stretch in NAI-89, else NAI-90. |
| Subagent writes to main working tree instead of worktree (per `feedback_subagent_wt_path.md`) | Controller runs `git status` on main before each merge; stash stray content if found. |

## 7. Out of scope

- No changes to `EntityLifeCycle` semantics, `setLifeCycle` plumbing, or the `locObjTracker` data structure beyond the constructor-signature revert.
- No refactor of `Server.AddLoc` / `Server.ChangeLoc` / `Server.RemoveLoc` ordering — only the IsActive write is moved.
- No port of any TS-side changes that aren't on HEAD of the canonical TS source.

## 8. Closes

`Closes memory:` trailer on the Bundle 3 smoke-confirmation commit (per `close_commit_memory_trailer.md`):
- NAI-88 (Stage 1 probe scaffold) — fully retired.

Memory entries to update if surfaced during execution: none anticipated; the IsActive consolidation is a one-time cleanup, not a recurring pattern worth saving.
