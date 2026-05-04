# NAI-90: Door throughwalk investigation

**Status:** Draft (brainstorm output)
**Date:** 2026-05-04
**Cadence:** Investigation sub-spec (Stage 1 instrumentation → smoke handoff → Bundle 2 memorialize). NAI-91 owns the conditional fix.
**Predecessor:** NAI-89 (loc-revert IsActive consolidation; smoke green for revert dispatch but surfaced throughwalk gap at RS Guide door).
**Tech stack:** Go 1.26+ (project) / TS reference at LostCityRS/Engine-TS / runescript content reference at zsrv/rs-server-225 `data/src/scripts/` (revision-225 content matching the Java client; identical to zsrv/rs-server-225 door content).

---

## 1. Goal

Bind which of four hypotheses owns the post-NAI-89 throughwalk gap at the Tutorial Island RS Guide door (loc_3014, ~3098,3107,0): the door visually opens and reverts on the correct tick, but the player ends up on the door tile instead of one tile through, then is wedged when `BlockWalk=true` is restored on revert.

**Symptom (re-confirmed at NAI-89 close):** Player clicks the RS Guide door. Visual: door form transitions 3014→83 then back 83→3014 at the scheduled tick. Player position: lands ON (3098,3107). Subsequent OPLOC1 clicks log `cheb_dist=0` + `ap_trigger=false` + `branch_post=0` (player IS the loc; can't path zero-distance, can't apProx-trigger from inside).

Per `cascade_theory_smoke_binding` and `investigation_subspec_cadence`: NAI-90 produces the smoke-binding evidence; NAI-91 ships the fix scoped to whichever hypothesis binds.

## 2. Context: re-framing of the seed memory

The `door_throughwalk_gap` memory authored at NAI-89 close framed throughwalk as a TS-engine concern: *"TS door _open does change_loc + walk player through; goscape implements only change_loc."*

Re-read against TS canonical (`Engine-TS/src/engine/entity/PathingEntity.ts`, `Player.ts`, `Npc.ts`, `GameMap.ts` — grep `door` returns zero hits in `engine/`), the throughwalk is **content-side**, not engine-side:

- `[oploc1,loc_3014]` (zsrv/rs-server-225 `data/src/scripts/tutorial/scripts/tut_doors_and_gates.rs2`) calls `~open_and_close_door(loc_param(next_loc_stage), $is_outside, false)`.
- `[proc,open_and_close_door]` (`data/src/scripts/doors/scripts/open_and_close_doors.rs2`) computes `$dest = movecoord($loc_coord, $x, 0, $z)` (where `$x, $z = ~door_open($angle, loc_shape)`) when `$entering=true`, then `p_teleport($dest)`.
- `~check_axis(coord, loc_coord, loc_angle)` returns the boolean controlling `$entering` — it depends on the player's pre-script `coord` relative to `loc_coord`.

Engine-side opcode coverage in goscape:

- `P_TELEPORT` is implemented at `pkg/script/handlers_player.go:543`.
- `MOVECOORD` is implemented at `pkg/script/handlers_server.go:69`.
- `P_WALK` is **stubbed** at `pkg/script/handlers_player.go:562` (pops coord, logs, returns nil — "pathfinder integration pending"). Door scripts do not use `p_walk`; this is unrelated.

The framing the memory implied (engine-side throughwalk) is therefore not factually grounded. The actual gap chain is one of four hypotheses (§4). NAI-90 binds which.

## 3. Cadence

Per `investigation_subspec_cadence`:

1. **Bundle 0 — controller pre-flight.** Read HEAD-current shape of `pkg/script/handlers_player.go:543` (handlePTeleport), `pkg/script/state.go:136+` (ScriptState struct), `modules/world/interaction.go:525-530` (pathToTarget), `modules/world/interaction_debug.go:32+` (existing frame A/B at handler level), `modules/world/movement.go:129-143` (pathToMoveClick→FindPathDefault). Done as part of brainstorm; no findings invalidate the seed.
2. **Bundle 1 — Stage 1 instrumentation.** Add frame T at `handlePTeleport`. Permanent under `NodeDebug`. One feat commit. Tests pin gate behavior + frame field population.
3. **Smoke handoff (out-of-band).** User runs server with default config (`NodeDebug=true`). Smoke scope β: RS Guide door (loc_3014) + Survival-tutor gate (loc_3015). User attaches captured `goscape.log` + observed pre/post-click player positions for each target.
4. **Bundle 2 — memorialize.** Read captured log against §4 routing rules, identify binding hypothesis, write `nai_followups.md` "From NAI-90" entry with binding evidence + paste-ready NAI-91 resume prompt. No production fix.
5. **Close commit.** `chore(close): NAI-90 — door throughwalk investigation [Hk binding]` with `Closes memory:` trailer per `close_commit_memory_trailer`.

Bundle 1's instrumentation is **permanent** (matching NAI-79 pattern): ships under `Cfg.NodeDebug` (default-on). Production deployments setting `NodeDebug=false` silence it. Retired only when the Loc-target throughwalk is fully ported (NAI-91 close, conditional on H1 binding).

## 4. Hypotheses + routing rules

Frame A is NAI-79's permanent frame at `handleOpLoc` tail (interaction_debug.go) capturing `op_loc_typeid`, `player_coord_pre_step`, `player_coord_post_step`, `loc_coord`, `loc_shape`, `loc_angle`. Frame T is NAI-90's new frame at `handlePTeleport` (§5).

**H1 — pathToTarget shape-blind (door + gate fail).**
- Frame A (door): `player_coord_post_step == loc_coord` (player paths onto door tile).
- Frame A (gate): `player_coord_post_step == loc_coord` (player paths onto gate tile).
- Frame T (door): `arg_coord == loc_coord` (script's `~check_axis` returned `false` because `coord==loc_coord`; `$entering=false` left `$dest` at `$loc_coord`).
- Frame T (gate): two firings — first at `loc_coord` (bare), second at `movecoord(loc_coord, 1, 0, 0)` (different dispatch). If gate's two p_teleports execute but player still ends on `loc_coord`, the bug is upstream of the script — confirms shape-blind path.
- **Bind:** root cause is `Player.pathToTarget` using `FindPathDefault` (shape-blind). NAI-91 = full TS `PathingEntity.pathToTarget` port (per brainstorm option iii: B2 SMART+Loc, B3 SMART+PathingEntity, B4 SMART+Obj+else, B5 NAIVE+PUBLIC, B6 retire FindPathDefault).

**H2 — proc execution / branch divergence (door fails, gate passes).**
- Frame A (door + gate): both show `player_coord_post_step != loc_coord` (player paths to adjacent tile correctly).
- Frame T (door): `arg_coord == loc_coord` (`~check_axis` or `~door_open` proc returned wrong value, OR multi-return arg-stack handling broken).
- Frame T (gate): `arg_coord` matches expected throughwalk tile.
- **Bind:** bug in goscape's proc-call / multi-return / `def_coord` handling. NAI-91 = proc-execution debug.

**H3 — teleport not applied (door + gate frame T correct, post-script position wrong).**
- Frame A (door + gate): `player_coord_post_step != loc_coord`.
- Frame T (door): `arg_coord` adjacent to `loc_coord` (correct throughwalk dest).
- Frame T (gate): both firings correct.
- Observed Java-client position: still `loc_coord` post-script.
- **Bind:** `handlePTeleport` writes the coord but the waypoint queue or post-tick movement reverts. NAI-91 = waypoint-queue clear on teleport / teleport write-timing investigation.

**H4 — script halts before p_teleport (frame T never fires for door).**
- Frame A (door): present (script dispatched).
- Frame T (door): never fires.
- **Bind:** script suspended before p_teleport (e.g., `p_delay(1)` semantics, error swallow, opcode missing). NAI-91 = script-suspension/error investigation. Sub-bind by which intermediate opcode (`def_coord`, `if`, `~door_open` gosub, `movecoord`) executed before suspension — this is implementer-discovery work in NAI-91, not pre-defined here.

**H5 — none of H1–H4 (smoke surprises with door-throughwalk working OR neither failing).**
- **Bind:** `door_throughwalk_gap` memory was over-attribution. Retire that memory entry. NAI-90 closes empty-handed. No NAI-91.

## 5. Bundle 1 — Stage 1 instrumentation

### 5.1 Frame T fields

slog at INFO when gated; emitted from `pkg/script/handlers_player.go:handlePTeleport`:

| Field | Source |
|---|---|
| `event` | constant `"p_teleport"` |
| `script_name` | `s.Script.Name` (file-relative) |
| `script_pc` | `s.PC` at handler entry |
| `self_pid` | identifying field exposed by `s.Self` (existing accessor; do not introduce one) |
| `self_coord_pre` | `s.Self.CoordPacked()` BEFORE the teleport call |
| `arg_coord` | popped packed coord (raw) |
| `arg_x`, `arg_z`, `arg_level` | from `unpackCoord(arg_coord)` |

If `s.Self` accessor does not exist for `pid`/equivalent, omit and rely on `self_coord_pre` for correlation against frame A.

### 5.2 Gating

Add `NodeDebug bool` field to `ScriptState`. Plan-author **MUST** grep all `&ScriptState{` and `ScriptState{` literals (per `plan_enumerate_struct_literals`) — zero-value is `false`, so existing fixtures stay silent without modification. Production wiring sets `NodeDebug = s.cfg.NodeDebug` at the script-runner factory site (grep `modules/world/script.go` or equivalent for the construction site).

### 5.3 Logger plumbing

`pkg/script` does not currently take a `*slog.Logger`. Two viable shapes:

- **(A) Add `Log *slog.Logger` field on ScriptState.** Constructor in `modules/world/script.go` injects the world's logger. Nil-safe: `handlePTeleport` skips emission if `s.Log == nil`.
- **(B) Use package-level `slog.Default()`.** Matches `handlePWalk` stub's `slog.Debug(...)` shape. Simpler. Tests intercept via `slog.SetDefault` with a record handler.

Plan defaults to (B) — matches existing convention at `handlePWalk`, no new struct field for plumbing. If a future sub-spec needs script-scoped logger redirection it can refactor.

### 5.4 Tests (TDD per `test-driven-development`)

In `pkg/script/handlers_player_test.go`:

1. `TestPTeleport_FrameT_EmittedWhenNodeDebugTrue` — set `s.NodeDebug=true`, install slog record-handler, run `OpPTeleport` after pushing a packed coord, assert one record with `event="p_teleport"` and the seven fields populated correctly.
2. `TestPTeleport_FrameT_SuppressedWhenNodeDebugFalse` — same fixture but `NodeDebug=false` (zero-value); assert zero records.
3. `TestPTeleport_FrameT_FieldValues` — known coord `(level=0, x=3098, z=3107)`, assert `arg_x=3098`, `arg_z=3107`, `arg_level=0`, `arg_coord` matches packed input.

Existing `TestPTeleport*` tests remain green: `NodeDebug` is zero-value `false`, no record handler installed. No fixture mass-update.

### 5.5 Out of scope for Bundle 1

- No fix for any of H1–H4. Fix lands in NAI-91.
- No instrumentation at `handlePWalk` (stub, no door content uses it).
- No instrumentation at other p_teleport-adjacent opcodes (`P_TELEJUMP`, etc.) — narrow surface to door-throughwalk routing.
- No new frame at `handleOpLoc` — frame A from NAI-79 is sufficient for Bundle 2 cross-foot.

## 6. Smoke protocol

Per `smoke_test_server_handoff` memory: user-launched, not Claude.

1. User starts server: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml` with default `NodeDebug=true`.
2. Java client login at default Tutorial Island spawn (outside RS Guide's house, near (3094,3107,0)).
3. Walk to RS Guide door (3098,3107,0) and click once. Observe player position post-tick.
4. (If still on Tutorial Island and Survival-tutor gate accessible) walk to Survival-tutor gate (loc_3015, ~around (3093,3107,0) — coords from `2004Scape-Server` configs) and click once. Observe post-tick position.
5. (Java-client-side note per `java_client_coord_chat_suppression`: chat may be suppressed on Tutorial Island; this smoke does not depend on chat.)
6. Capture `goscape.log` for the click ticks. Attach to NAI-90 Bundle 2 hand-off.

If smoke cannot reach the gate (e.g. RS Guide door wedge still occurs and blocks the rest of Tutorial Island), capture door-only frames; H1/H2 disambiguation degrades to single-target evidence and Bundle 2 routes accordingly (favoring H1 since the gate is upstream of the door wedge).

## 7. Bundle 2 — memorialize

Inputs: captured smoke log + Java-client observation report.

Tasks:

1. Apply §4 routing rules to bind exactly one of H1/H2/H3/H4/H5.
2. Write `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` "From NAI-90" entry with:
   - Binding evidence (one log excerpt per relevant frame, field values interpreted).
   - The bound hypothesis number + name.
   - Scope ceiling for NAI-91 (e.g., for H1: "full TS `PathingEntity.pathToTarget` port per NAI-90 spec §4 H1; closes `pathfinder_api_loc_aware`").
3. Author NAI-91 paste-ready resume prompt per `post_task_handoff` memory.
4. If H5 binds: retire `door_throughwalk_gap` memory entry (delete file + drop MEMORY.md line); add a memory entry capturing what *did* happen if non-derivable.
5. Memory entry for the door+gate Tutorial-Island throughwalk smoke fixtures becoming canonical (only if the Bundle 2 evidence shows they're reproducible across re-runs — speculative-cement-avoidance).

No production code change in Bundle 2.

## 8. Tracked deviations

None new. NAI-90 ships only Stage 1 instrumentation; behavior unchanged for `NodeDebug=false`. Pre-existing divergences referenced (P_WALK stub, FindPathDefault, etc.) remain under their existing trackers — NAI-90 does not close any.

## 9. Risk register

- **R1 — `&ScriptState{}` fixture sweep.** ~30+ literals in `pkg/script/handlers_player_test.go` alone. Mitigation: `NodeDebug` zero-value is `false`, so no fixture update is required. Plan-author runs `rg "ScriptState\{|&ScriptState\b" pkg/ modules/` at plan-write to confirm count and document the no-update justification.
- **R2 — smoke unreachable due to wedge.** RS Guide door wedge may block the rest of Tutorial Island; gate may not be reachable in the same session. Mitigation: smoke captures door-only evidence; §6 step 6 documents the degraded-evidence routing rule. Stage 1 frame T fires per p_teleport invocation regardless of which target the click reached.
- **R3 — frame T fires for unrelated p_teleport calls.** Login teleport, RESPAWN_PLAYER, etc. also call p_teleport. Mitigation: `script_name` field disambiguates; Bundle 2 filters on `script_name` matching `oploc1,loc_3014` or `open_and_close_door` proc. Cost: minor log volume in default-on production; acceptable given investigation lifetime is ≤ NAI-91 close.
- **R4 — slog.Default() interception fragility in tests.** Approach (B) for logger plumbing relies on `slog.SetDefault` reset between tests. Mitigation: use `t.Cleanup(...)` to restore the prior default; pattern used elsewhere in goscape — plan-author greps for an existing example before authoring.
- **R5 — over-attribution to memory.** `door_throughwalk_gap` memory was authored with the engine-side framing this spec re-frames. If H5 binds, memory must be retired in Bundle 2; if H1–H4 binds, memory must be edited to point at the actual root cause + NAI-91. Mitigation: §7 task 4 explicitly handles H5; for H1–H4 the post-NAI-91-close edit is the natural site.

## 10. Out of scope (explicit deferrals)

- Any production fix for door throughwalk → NAI-91 (conditional on Bundle 2 binding).
- Full TS `PathingEntity.pathToTarget` port → NAI-91 if H1 binds.
- `P_WALK` opcode implementation → unrelated; deferred indefinitely until a content-script smoke surfaces a real consumer.
- Ladder/stairs/bookcase/drawer throughwalk smoke coverage → potential NAI-N+1 if NAI-91 fix doesn't close them as a downstream effect; routed per `smoke_surfaces_adjacent_divergences`.
- Frame instrumentation at non-p_teleport opcodes → NAI-91 if H4 binds and needs deeper script-suspension visibility.

## 11. Test strategy summary

Bundle 1: 3 unit tests in `pkg/script/handlers_player_test.go` (§5.4).
Bundle 2: no code tests — memory + log-evidence work only.
Smoke: out-of-band, β scope, user-driven (§6).

No regression risk to existing tests: zero-value `NodeDebug=false` on every existing `&ScriptState{}` literal preserves silence; new tests opt in explicitly.
