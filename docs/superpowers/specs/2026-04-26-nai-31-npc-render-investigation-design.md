# NAI-31 — NPC render-pipeline investigation + D2 retire + D1 doc-cleanup

- **Sub-spec**: NAI-31
- **Date**: 2026-04-26
- **Scope label**: Investigation-and-fix sub-spec — risk-weighted short-circuit static audit (Stage 1) of the five-layer NPC pipeline (gamemap loader → server spawn loop → addNpc registration → per-tick encoder → OpNpcInfo wire), targeted fix(es) for whatever layer Stage 1 surfaces (Stage 2), retirement of NAI-30-D2 (local-player CHAT mask suppression in `pkg/rsbuf/playerinfo.go::writeLocalPlayer`), opportunistic doc-cleanup of NAI-30-D1 stale comments without retiring the underlying deviation, replacement of the unauthorized `rs-server-225/engine/gamemap.go` citation in `pkg/gamemap/load.go:138` with a TS-canonical-path citation, user-mediated smoke handoff (Java client launch by user) as the binding feature-correctness gate, and an explicit Stage 3 fallback (gated runtime instrumentation, second smoke iteration) only if Stage 2's fix doesn't render NPCs in the client. Touches `pkg/gamemap/`, `pkg/rsbuf/`, `modules/world/` (Stage-1-conditional surfaces; concrete file list resolved at plan-author time after audit verdicts). Produces zero new deviations in the most-likely outcome (parser was authored against unauthorized sibling repo, port to TS-canonical fixes it). Net deviation count target: 14 → 13 (D2 retired).
- **Predecessors**: NAI-30 (encoder loops port + production read-flip) — last on `main` as `c980628`
- **Source root**:
  - `LostCityRS/Engine-TS` (TS canonical for `pkg/gamemap` per `ts_source_canonical_path.md`)
  - `LostCityRS/Client-Java` (binding wire spec for OpNpcInfo + NpcInfo packet handler)
  - `2004scape/rsbuf` branch `225` (Rust canonical for `pkg/rsbuf` per `rust_source_canonical_path.md`)

## Motivation

NAI-30 closed cleanly with `(*PlayerInfo).Encode` + `(*NpcInfo).Encode` wired into production via T4.2/T4.3. The user's smoke test on 2026-04-26 confirms world-map render works (REBUILD_GETMAPS arrives post-`ecbe311` first-build sentinel restore), players are visible, login flows cleanly. **NPCs do not render in the Java client.** The user reports this predates NAI-30 — the bug existed even before the encoder swap, but was previously masked by other smoke-blocking issues that NAI-30 resolved.

`pkg/rsbuf/npcinfo_test.go::TestPlayerSeesNearbyNpc` exercises spawn → tick → encoder side-effect (`HasNpc` → true) and passes at HEAD. So the bug is at a layer the encoder unit test doesn't exercise: the gamemap NPC-spawn loader (parses `n{X}_{Z}` files into `gm.npcSpawns`), the production startup spawn loop (`server.go:229-242`), the per-tick `npcSources` build (`tick.go:330-...`), the OpNpcInfo wire framing (opcode + dynamic length prefix), or a first-tick edge case in the encoder that the spawn-then-tick test doesn't capture (e.g., `Build.GetNearbyNpcs` zoneMap subscription state on a fresh-login player).

A canonical-source violation already lives at `pkg/gamemap/load.go:138`: the `loadNPCs` parser cites "rs-server-225/engine/gamemap.go" as its source. Per `ts_source_canonical_path.md` and `rust_source_canonical_path.md`, the only authoritative sources for goscape are `LostCityRS/Engine-TS` (everything except `pkg/rsbuf`) and `2004scape/rsbuf` branch 225 (only `pkg/rsbuf`). The `rs-server-225` repository is an unauthorized sibling. The parser may or may not match what the LostCity world data files on disk actually contain — that's the leading audit hypothesis.

Two NAI-30 deferrals also live in this surface area:

- **NAI-30-D2** — local-player CHAT mask suppression. `pkg/rsbuf/playerinfo.go::writeLocalPlayer` doc-comment at lines 113-119 says: "upstream PlayerInfo::highdefinition at info.rs:289-291 strips the CHAT mask bit for self (no chat self-echo). Goscape's existing eager Renderer doesn't expose per-mask suppression, so the local player's own chat may echo back to its own client by one chat block per say." Deferral cites "fix lands when NAI-31 ports the renderer cache." Pre-spec grep finds that `pkg/rsbuf/mask_payload.go::writeMaskPayloads` already accepts a `suppressChat bool` parameter and respects it (line 46-48). The actual fix surface is therefore narrower than the deferral comment implies: plumb `suppressChat=true` through `writeLocalPlayer`'s mask path. Whether this requires a renderer-cache port or a smaller in-place fix is a Stage 2 audit question. The skipped test at `pkg/rsbuf/playerinfo_test.go:577-580` (`TestPlayerInfo_LocalPlayer_ChatMaskStripped`) becomes the regression pin.

- **NAI-30-D1** — orientation field plumbed without producer. NAI-31 does NOT retire D1's underlying deviation; "engine-port series will retire" the actual `set_orient` + npc-config initial-orientation wiring later. NAI-31 only cleans stale doc-comments at `modules/world/player.go:261` and `modules/world/npc.go:122` that reference D1's status, plus the test-comment forward-reference at `pkg/rsbuf/npcinfo_test.go:657` ("NAI-31's fallback ladder may use orientation"). The deviation entry's text is updated to clarify "doc-comments cleaned at NAI-31; producer wiring remains deferred to engine-port series."

## Tech stack

- Go 1.26+
- Existing packages **read** from at brainstorm time:
  - `pkg/gamemap/load.go::loadNPCs` (parses `n{X}_{Z}` cache files; format: 2-byte packed pos, 1-byte count, N×2-byte type IDs per record, per current doc comment citing unauthorized `rs-server-225` source)
  - `pkg/gamemap/gamemap.go` (`NpcSpawn` struct, `npcSpawns []NpcSpawn` slice, `NpcSpawns()` accessor)
  - `modules/world/server.go:229-242` (production startup spawn loop iterating `s.gamemap.NpcSpawns()` and calling `s.addNpc(n, -1, true)`)
  - `modules/world/npc_registry.go::addNpc` (firstSpawn=true branch: alloc slot, register in `s.npcs[]` + `s.npcLoop`, call `s.rsbuf.AddNpc(...)`)
  - `modules/world/tick.go:330-...` (`npcSources` builder iterating `s.npcLoop`, calling `s.rsbuf.ComputeNpc(...)` per NPC)
  - `modules/world/player_npc_info.go` (`updateNpcs` calls `s.rsbuf.NpcInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)`, writes via `p.writeOut(gameserver.OpNpcInfo, payload)`)
  - `pkg/io/protocol/game/server/` (`OpNpcInfo` opcode constant + size sentinel — `-1` 1-byte length prefix or `-2` 2-byte length prefix)
  - `pkg/rsbuf/npcinfo.go` (`(*NpcInfo).Encode` — encoder, runs writeNpcs + writeNewNpcs; structurally pinned by `TestPlayerSeesNearbyNpc` + sibling tests)
  - `pkg/rsbuf/playerinfo.go::writeLocalPlayer` (D2 surface — local-player movement-bits + mask-emission path)
  - `pkg/rsbuf/mask_payload.go::writeMaskPayloads` (already takes `suppressChat bool`; D2's plumbing target)
  - `pkg/rsbuf/playerinfo_test.go::TestPlayerInfo_LocalPlayer_ChatMaskStripped` (currently `t.Skip("NAI-30-D2: requires NAI-31 renderer cache port for per-mask suppression")` — D2's regression pin once unskipped)
  - `LostCityRS/Engine-TS` NPC-spawn-file parser (Stage 1.1 audit source — exact file path resolved at plan-author time; likely `Engine-TS/src/engine/GameMap.ts` or its NPC-loading helper)
  - `LostCityRS/Client-Java` `NpcInfo` packet handler entry (Stage 1.2 audit source — exact class path resolved at plan-author time)
  - On-disk `n{X}_{Z}` cache files (Stage 1.1 triangulation source — read raw bytes from a sample mapsquare to verify either TS or goscape's parser matches reality)
- Modified files in `pkg/gamemap/`:
  - `load.go` — Stage-1-conditional: if 1.1 finds parser-format divergence, fix `loadNPCs` byte-format. Doc-comment at line 138 updated regardless to remove unauthorized `rs-server-225/engine/gamemap.go` citation; replaced with TS-canonical citation per `ts_source_canonical_path.md`. Stage 1 verdict.
  - `gamemap.go` — Stage-1-conditional: if 1.1 surfaces a struct-shape change (e.g., `NpcSpawn` needs an extra field), update here. Most-likely outcome: no change.
- Modified files in `pkg/rsbuf/`:
  - `playerinfo.go` — Stage-2: plumb `suppressChat=true` through `writeLocalPlayer`'s mask path (D2 retire). Mechanical surface depends on whether the fix is in-place (call `writeMaskPayloads(... suppressChat=true)` directly) or requires renderer-cache port (resolved by Stage-2 audit; see Risk R2).
  - `playerinfo_test.go` — Stage-2: unskip `TestPlayerInfo_LocalPlayer_ChatMaskStripped` (line 580); ensure it asserts both presence (CHAT to other players) and absence (no CHAT to self) per `ts_asymmetry_dual_pin.md`.
  - `npcinfo.go` — Stage-1.4-conditional: only touched if 1.4 surfaces a first-tick encoder edge case. Most-likely outcome: no change.
  - `npcinfo_test.go` — Stage-2: clean test-comment forward-reference at line 657 ("NAI-31's fallback ladder may use orientation") to reflect post-NAI-31 status.
- Modified files in `modules/world/`:
  - `player.go:261` — clean stale D1 comment (text update only; no production-code change).
  - `npc.go:122` — clean stale D1 comment (text update only).
  - `server.go:229-242` — Stage-1.3-conditional: if 1.3 finds spawn-loop wiring gap, fix here. Most-likely outcome: no change (loop is straightforward).
  - `tick.go:330-...` — Stage-1.3-conditional: if 1.3 finds per-tick `npcSources` build gap, fix here. Most-likely outcome: no change.
  - `player_npc_info.go` — Stage-1.2-conditional: if 1.2 finds wire-framing gap (opcode constant, length-prefix byte width), fix here. Most-likely outcome: no change.
- New files: none anticipated. Stage 3 (if reached) adds a gated logger import to `modules/world/server.go` and `modules/world/player_npc_info.go`; both flagged-by-env-var, removed at NAI-31 close.
- Test files modified or created (Stage-1-conditional):
  - `pkg/gamemap/load_test.go` (new, if absent) or `pkg/gamemap/gamemap_test.go` extension — synthetic-bytes regression test for `loadNPCs`. Per `plan_runnable_test_fixtures.md`, fixture mentally executed before dispatch. Per `match-spec-tests-to-library-capabilities`, synthetic bytes (deterministic, no I/O), not real cache files.
  - `pkg/rsbuf/playerinfo_test.go` — D2 regression test unskipped, asserts dual-pin (presence+absence) per `ts_asymmetry_dual_pin.md`.
  - Wire-framing byte-shape test (Stage-1.2-conditional) in `pkg/io/protocol/game/server/` or `modules/world/`.
- Memory files:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — at NAI-31 close: append "From NAI-31 (2026-04-XX)" close entry; remove NAI-31 candidate section (NPCs not visible) since investigation is complete; remove NAI-30-D2 entry (retired); update NAI-30-D1 entry text to reflect doc-cleanup-only at NAI-31 + producer-wiring-still-deferred status; net deviation count update 14 → 13.
  - Close commit body carries `Closes memory: NAI-30-D2` per `close_commit_memory_trailer.md`.
  - Potentially-new memory entries (added at close if surfaced): see "Memory entries to potentially add at NAI-31 close" section below.

## Scope

### Stage 1 — Static audit (risk-weighted short-circuit)

Stage 1 is read-only. No production code changes, no commits. Output is a written verdict that drives Stage 2 dispatch decisions. Each Stage 1 substage has three possible verdicts:

- **Conclusive bug found** → proceed to Stage 2 with this layer's fix as primary work; skip remaining substages.
- **No divergence** → fall through to next substage.
- **Ambiguous** (e.g., upstream source unclear, divergence found but functional impact unknown) → escalate immediately to Stage 3 (runtime instrumentation), skip remaining substages.

#### Stage 1.1 — `pkg/gamemap/load.go::loadNPCs` parser audit

Highest-suspicion path. Smoking-gun prior: doc-comment cites unauthorized `rs-server-225/engine/gamemap.go`.

**Sources read:**
- `pkg/gamemap/load.go::loadNPCs` (current parser implementation).
- `LostCityRS/Engine-TS` NPC-spawn-file parser (exact path resolved at plan-author time via grep for "loadNPCs" / "n{X}_{Z}" / mapsquare-NPC handlers).
- A real `n{X}_{Z}` file's leading bytes from `cfg.CachePath/client/maps/` — triangulation source. If goscape's parser disagrees with TS's parser AND on-disk bytes match TS's expected format, that's conclusive.

**Diff axes:**
- Record header layout: byte order of packed-coord field, bit-shift positions for level/localX/localZ.
- Count field width (1 byte vs 2 bytes vs other).
- Type-ID field width (2 bytes vs other).
- Per-record vs whole-file termination.

**Output:** written verdict with file:line citations from all three sources; a 3-line summary classifies as `CONCLUSIVE_BUG_FOUND`, `NO_DIVERGENCE`, or `AMBIGUOUS`.

#### Stage 1.2 — `OpNpcInfo` wire framing audit

Run if 1.1 lands `NO_DIVERGENCE`.

**Sources read:**
- `pkg/io/protocol/game/server/` `OpNpcInfo` opcode constant + size sentinel.
- `modules/world/player_npc_info.go::updateNpcs` (writeOut path).
- `LostCityRS/Client-Java`'s `NpcInfo` packet-handler entry — opcode value, expected length-prefix byte width, expected payload-byte ordering.

**Diff axes:**
- Opcode value (numeric).
- Length-prefix byte width (`-1` for 1-byte, `-2` for 2-byte).
- Any pre/post payload framing the goscape side omits or adds.

**Output:** same 3-verdict classification.

#### Stage 1.3 — Production wiring audit

Run if 1.2 lands `NO_DIVERGENCE`.

**Sources read:**
- `modules/world/server.go:229-242` (spawn loop).
- `modules/world/npc_registry.go::addNpc` (firstSpawn=true branch).
- `modules/world/tick.go:330-...` (`npcSources` build path).
- `pkg/rsbuf/buf.go::AddNpc`/`ComputeNpc` (state-population path).
- `LostCityRS/Engine-TS` `World.addNpc` initialization (cross-reference for missing collision toggles, missing zone enters, missing initial-orient).

**Diff axes:**
- Missing `s.rsbuf.AddNpc` call.
- Slot-0 vs slot-1 indexing mismatch (NPC slot 0 reserved per `npc_registry.go:20`).
- Level field mishandling (e.g., `gm.npcSpawns` uses `level` from packed coord but downstream code expects 0).
- Coord packing/unpacking discrepancy between gamemap and rsbuf.

**Output:** same 3-verdict classification.

#### Stage 1.4 — Encoder first-tick edge cases

Run if 1.3 lands `NO_DIVERGENCE`.

**Sources read:**
- `pkg/rsbuf/buildarea.go::GetNearbyNpcs` (zoneMap query path).
- `pkg/rsbuf/npcinfo.go::writeNewNpcs` (first-tick discovery branch).
- `modules/world/login.go` and adjacent files for player-coord-set timing relative to first `updateNpcs` call.
- `modules/world/tick.go` per-tick ordering: when does the player's zoneMap subscription land relative to the encoder's `GetNearbyNpcs` call?

**Diff axes:**
- Player `Coord` is `0,0,0` at first tick → `GetNearbyNpcs` returns empty.
- Player not subscribed to any zone at first tick → `GetNearbyNpcs` returns empty.
- Encoder runs before tick body — observers never increment.

**Output:** same 3-verdict classification.

If all four substages return `NO_DIVERGENCE`, escalate to Stage 3 unconditionally. The bug exists; one of the four layers must contain it; if static analysis can't surface it, runtime instrumentation is the remaining tool.

### Stage 2 — Targeted fix(es) + D2 retire + D1 doc-cleanup

Stage 2 follows TDD discipline per `superpowers:test-driven-development`: red → green → refactor.

**Task 2.1 — Implement Stage 1's primary finding fix.** Concrete file/line list resolved by Stage 1 verdict. Each bug-layer fix is a separate task with its own failing-test-first cycle. If multiple layers are broken, they ship as separate commits in dispatch order (most-suspect first).

**Task 2.2 — NAI-30-D2 retire.** Plumb `suppressChat=true` through `writeLocalPlayer`'s mask-emission path. Unskip `TestPlayerInfo_LocalPlayer_ChatMaskStripped` and convert to dual-pin (presence + absence) per `ts_asymmetry_dual_pin.md`. Stage 2 audit decides whether the fix requires the renderer-cache port mentioned in the deferral comment OR is a smaller in-place change.

**Task 2.3 — NAI-30-D1 doc-cleanup.** Update the four sites enumerated by pre-spec grep:
- `modules/world/player.go:261` — comment text update.
- `modules/world/npc.go:122` — comment text update.
- `pkg/rsbuf/npcinfo_test.go:657` — test-comment text update (NAI-31's fallback ladder reference).
- `nai_followups.md` D1 entry — update text to reflect doc-cleanup-only-at-NAI-31 status.

**Task 2.4 — Canonical-source citation update.** `pkg/gamemap/load.go:138` doc-comment replaces unauthorized `rs-server-225/engine/gamemap.go` with TS-canonical-path citation. Trivial 1-line change; ships in same commit as 2.1 if 2.1 touches the same file, otherwise standalone.

### Smoke handoff (between Stage 2 and close commit)

Per `smoke_test_server_handoff.md`: ask the user to launch the server with the latest binary and connect with the Java client. User reports:

- **NPCs render** → proceed to close commit with `Closes memory:` trailer for D2.
- **NPCs don't render** → spec stays open, enter Stage 3.

### Stage 3 — Runtime instrumentation (conditional)

Only created if Stage 2's smoke test fails. Plan doc adds Bundle 3 at that point.

**Task 3.1.** Add gated `slog.Info` logs (env-var-controlled) at:
- `modules/world/server.go` startup: `len(s.gamemap.NpcSpawns())`, count of registered slots in `s.npcLoop` post-spawn-loop.
- `modules/world/player_npc_info.go::updateNpcs`: per-tick `len(payload)` for the first connected player only (gated to avoid log spam).

**Task 3.2.** User runs server again with the env var set, captures log output, sends back.

**Task 3.3.** Analyze logs, identify root cause, iterate Stage 2 with new findings. Run smoke handoff again.

**Task 3.4.** Once smoke confirms render, Stage 3 instrumentation removed (gated logs deleted, env-var conditional removed) before close commit.

### Bundle structure (working hypothesis)

Concrete bundles resolved at plan-author time after Stage 1 verdicts are known. Working hypothesis assumes Stage 1.1 surfaces a parser bug (highest-prior path).

- **Bundle 1 — Stage 1 audit.** Single subagent dispatch reads all relevant sources, returns written verdict. No commits.
- **Bundle 2 — Stage 2 fixes (TDD).** One commit per fix layer + D2 retire + D1 doc-cleanup. Per `compressed_cadence.md`, if the parser fix is ≤15 LOC AND D2 fix is ≤15 LOC AND D1 cleanup is ≤15 LOC, all three may compress into a single commit. Otherwise standard cadence.
- **Smoke handoff.** Out-of-band; no commit.
- **Bundle 3 (conditional).** Stage 3 instrumentation + iterate. Only exists if Bundle 2 smoke fails.
- **Close commit.** chore-tagged, `Closes memory: NAI-30-D2` trailer. Updates `nai_followups.md`. Net deviation count 14 → 13.

## True-to-TS / true-to-Rust gate

Per `true_to_ts_gate.md`: every behavioral divergence needs a tracked deviation with rationale + follow-up. NAI-31's gate behavior:

**Source-of-truth precedence (when sources disagree):**
1. `LostCityRS/Client-Java` for any wire-format question (it's the binding consumer of OpNpcInfo bytes).
2. `LostCityRS/Engine-TS` for everything not in `pkg/rsbuf` (per `ts_source_canonical_path.md`).
3. `2004scape/rsbuf` branch 225 for `pkg/rsbuf` (per `rust_source_canonical_path.md`).
4. On-disk `n{X}_{Z}` cache files as triangulation when (1) and (2) disagree (those bytes are what TS Engine-TS authored, so on-disk is an Engine-TS proxy).

**Most-likely outcome (no new deviations):** Engine-TS authored the LostCity world data, so TS-canonical and on-disk-canonical align; goscape's parser was written against the wrong upstream and just needs to be ported correctly. No tracked deviation created.

**Conditional new deviations:**
- **NAI-31-D1 [conditional].** If Stage 1.1 finds gamemap parser must diverge from TS to render correctly, the divergence is tracked.
- **NAI-31-D2 [conditional].** If Stage 1.2 finds wire framing must diverge from Client-Java to render correctly, the divergence is tracked.

Plan-author must call out any chosen divergence in the close commit body and append the deviation entry to `nai_followups.md` under "Active deviations."

## Risks & mitigations

- **R1 — Stage 1.1 inconclusive.** TS NPC-loading code may use abstractions obscuring the byte-format truth. **Mitigation:** triangulate against on-disk bytes from a real `n{X}_{Z}` file (third source); if TS code and on-disk bytes agree but goscape disagrees, conclusive verdict.

- **R2 — D2's renderer-cache requirement is real.** The deferral comment claims the fix needs a renderer-cache port. Pre-spec grep finds `writeMaskPayloads` already takes `suppressChat bool` — but the local-player payload may flow through `renderer.HighDefOf(pid)` which is a cached payload that already includes chat. **Mitigation:** Stage 2 audit reads `writeLocalPlayer` end-to-end; if the in-place fix isn't tractable, D2 falls back to "remains deferred" status and the close commit explains why. NAI-31 does not fail to close because of D2; the render-fix is the primary ship.

- **R3 — Multiple bugs across multiple layers.** Stage 1 risk-weighted-short-circuit handles this implicitly: smoke fails after first fix → Stage 3 surfaces next layer → second fix. We don't preemptively audit layers that may not be broken.

- **R4 — Smoke fails with no clear next step.** Stage 3 instrumentation produces inconclusive logs (e.g., spawn count 1500, payload bytes non-zero, but client doesn't render). **Mitigation:** widen audit to remaining static surfaces (1.2, 1.3, 1.4); if all four exhausted without finding bug, NAI-31 closes with a written "investigation findings" doc and NAI-32 reopens with a different angle (e.g., capture wire bytes via tcpdump, decode by hand against Client-Java's parser).

- **R5 — User's local Java client cache out of date.** If `LostCityRS/Client-Java`'s commit hash has drifted from what the project expects, the symptom is identical to a server bug. **Mitigation:** Bundle 1 (Stage 1) opens by asking the user to confirm the Client-Java commit hash matches the project's expected version (per `CLAUDE.md`'s reference to the Client-Java repo).

- **R6 — Latent-bug-at-migration-boundary recurrence (per `latent_bug_at_migration_boundary.md`).** NAI-30 retired the legacy NpcInfo encoder. The bug pre-existed by user assertion, but the read-flip may have changed the surface. **Mitigation:** Stage 1.4 explicitly considers whether the legacy encoder code differed in a way that masked the bug. Probability low (user confirms pre-NAI-30), called out for completeness.

- **R7 — Plan-author premise rot (per `controller_preflight.md`).** If audit takes long enough that file paths drift, Stage 2 dispatch must re-grep at HEAD. Standard discipline.

- **R8 — Test-passes-for-wrong-reason (per `test_passes_for_wrong_reason.md`).** A regression test fixture that's malformed in a different way than the production input may produce expected output by coincidence. **Mitigation:** the fixture for `loadNPCs` regression must be either real on-disk leading bytes from an `n{X}_{Z}` file OR a synthetic fixture cross-checked by a second independent grep+Read of the byte format spec. Implementer runs `git checkout HEAD~1 -- <test file>` (or equivalent) and confirms test fails for the *correct reason* before applying fix.

- **R9 — Implementer claim verification (per `verify_implementer_claims.md`).** Stage 2 implementer reports test-green from package scope; controller verifies with `go test ./...` from a fresh checkout before close commit. Three failure modes the controller watches for: stale IDE diagnostics, package-scoped green masking cross-package breakage, false "pre-existing failures" attributions.

## Sequencing

Stage 2's bundle-1 → bundle-2 → smoke → close ordering is fixed. Stage 1's substage ordering is risk-weighted short-circuit (1.1 → 1.2 → 1.3 → 1.4 only on `NO_DIVERGENCE` propagation).

If Stage 1.1 produces `CONCLUSIVE_BUG_FOUND`, the rest of Stage 1 is skipped. If it produces `AMBIGUOUS`, Stage 1 is abandoned and Stage 3 is entered immediately. The plan author records the chosen path in the plan doc with the verdict text quoted inline.

## Open questions for plan-author

- **Q1.** Pre-Stage-1 grep+Read pass (canonical-source citation, D2 location, D1 stale-comment enumeration) folded into Bundle 1 as Task 1.0, or done by controller before any subagent dispatch? **Recommendation:** controller-side, before any dispatch — it's metadata gathering. Recorded in plan doc as a frozen "premises" block.
- **Q2.** If Stage 1.1 verdict is `CONCLUSIVE_BUG_FOUND` and the parser fix is ≤15 LOC, does Bundle 2 use compressed cadence per `compressed_cadence.md`? **Recommendation:** yes — combine spec-update + plan + fix into a single commit if all three are individually <15 LOC.
- **Q3.** Does the plan cover Stage 3 instrumentation upfront, or wait to add it as Bundle 3 only if smoke fails? **Recommendation:** wait — Stage 3 is conditional; preemptive coverage bloats the plan with dead branches.

## Memory entries to potentially add at NAI-31 close

The following candidates surfaced during NAI-31 brainstorming and may warrant memory entries at close (decision deferred to close-time review):

- **canonical-source-violation-watch** — the `rs-server-225` citation in `loadNPCs` was authored before goscape's canonical-source rules were memorialized. Pattern to capture: at every NAI brainstorm, run `rg "rs-server-225|other-fork-name"` across `pkg/` + `modules/` to catch new unauthorized citations before they ship.
- **investigation-sub-spec-cadence** — NAI-31 is goscape's first investigation-only sub-spec (every prior NAI was port/feature/audit). If the Stage-1-static-first → Stage-2-fix → smoke → Stage-3-conditional shape works well, capture as a reusable cadence pattern.
- **smoke-handoff-binding-feature-gate** — the lesson of NAI-31's existence is that test-suite green did not equal feature-correct. Strengthen `verify_implementer_claims.md` or add a sibling entry pinning "any feature contract that's only verifiable visually MUST gate close on user-mediated smoke; package-test green is necessary but never sufficient."

These are candidates only; close-time review decides which actually become memory entries based on whether NAI-31's experience confirmed the pattern.
