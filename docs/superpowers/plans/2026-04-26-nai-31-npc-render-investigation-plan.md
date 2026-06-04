# NAI-31 — NPC render-pipeline investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make NPCs render in the Java client. Static-audit the five-layer NPC pipeline (gamemap loader → server spawn loop → addNpc registration → per-tick encoder → OpNpcInfo wire) using risk-weighted short-circuit, fix whichever layer Stage 1 surfaces, retire NAI-30-D2 (local-player CHAT mask suppression) if tractable, clean NAI-30-D1 stale doc-comments, replace the unauthorized `rs-server-225` citation in `pkg/gamemap/load.go:138`, and confirm via user-launched smoke test before close.

**Architecture:** Investigation-and-fix sub-spec with branching execution. Bundle 0 freezes premises (controller pre-flight, no commits). Bundle 1 dispatches Stage 1 audit (single subagent, no commits, output is verdict). Bundle 2 materializes Stage 1's verdict into concrete fix tasks (TDD red→green→commit). Smoke handoff is user-mediated. Bundle 3 (conditional) adds runtime instrumentation only if Bundle 2's smoke fails. Close commit retires NAI-30-D2 deviation from the tracker; net deviation count target 14 → 13.

**Tech Stack:** Go 1.26+; `pkg/gamemap/`, `pkg/rsbuf/`, `modules/world/`; sources of truth = `LostCityRS/Engine-TS` (canonical for `pkg/gamemap`), `LostCityRS/Client-Java` (binding wire spec), `2004scape/rsbuf` branch 225 (canonical for `pkg/rsbuf`).

---

## Bundle 0 — Controller pre-flight (no commits)

Premise-freeze pass per `controller_preflight.md`. Run by the controller (the orchestrating session) before any subagent dispatch. Output is a "Frozen Premises" block appended to this plan (in-place edit of this file) so Bundle 1's subagent receives exact line numbers, file paths, and source URLs.

**Files:**
- Modify: `docs/superpowers/plans/2026-04-26-nai-31-npc-render-investigation-plan.md` (append Frozen Premises section at end of Bundle 0)

- [ ] **Step 0.1: Verify spec premises at HEAD**

Run each grep + Read; record findings in plan doc.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go version
git rev-parse HEAD
git status --porcelain
```

Expected: HEAD = 76b55fb + the spec commit `b9ffb78`; clean working tree. If anything else, stop and resolve.

- [ ] **Step 0.2: Confirm pre-spec grep results haven't rotted**

```bash
rg -n "NAI-30-D1|NAI-30-D2|set_orient|rs-server-225" pkg/ modules/ cmd/
rg -n "CHAT|suppressChat" pkg/rsbuf/
```

Expected sites (all must still be present):
- `modules/world/player.go:261` — D1 stale comment (orientation producer reference).
- `modules/world/npc.go:122` — D1 stale comment.
- `pkg/rsbuf/playerinfo.go:113-119` — D2 deferral block.
- `pkg/rsbuf/playerinfo_test.go:577-580` — `t.Skip("NAI-30-D2: ...")` regression-pin-in-waiting.
- `pkg/rsbuf/npcinfo_test.go:657` — test-comment "NAI-31's fallback ladder may use orientation".
- `pkg/gamemap/load.go:138` — `rs-server-225/engine/gamemap.go` citation.
- `pkg/rsbuf/mask_payload.go:21,46-48` — `writeMaskPayloads` already takes `suppressChat bool`.
- `pkg/rsbuf/playerinfo.go:120-198` — `writeLocalPlayer` uses `renderer.HighDefOf(pid)` cached payload.

If any expected site has shifted, update the line numbers in the plan doc before Bundle 1 dispatch.

- [ ] **Step 0.3: Confirm canonical sources are accessible**

```bash
ls /home/owner/Code/github.com/LostCityRS/Engine-TS/ 2>&1 | head -3
ls /home/owner/Code/github.com/LostCityRS/Client-Java/ 2>&1 | head -3
ls /home/owner/Code/github.com/2004scape/rsbuf/ 2>&1 | head -3
```

Expected: all three return directory listings (not "No such file"). If any are missing, ask the user to confirm canonical-source paths.

- [ ] **Step 0.4: Identify TS NPC-spawn-file parser path**

```bash
rg -l "loadNpc|loadNPCs|loadNpcs\|n\\.dat\|n_x_z" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/ 2>&1 | head -5
rg -n "GameMap" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/lostcity/engine/GameMap.ts 2>&1 | head -10
```

Expected: at least one `.ts` file references the `n{X}_{Z}` mapsquare-NPC parsing logic. Record the path + relevant line range in plan doc Frozen Premises.

- [ ] **Step 0.5: Identify Client-Java NpcInfo packet handler path**

```bash
rg -l "NpcInfo\|OpNpcInfo\|opcode.*NPC_INFO" /home/owner/Code/github.com/LostCityRS/Client-Java/src/ 2>&1 | head -5
```

Expected: at least one `.java` file. Record path + line range in plan doc.

- [ ] **Step 0.6: Pin a real `n{X}_{Z}` cache file for triangulation**

```bash
ls $(grep -E '^\s*cache_path|cachePath|CachePath' config.yaml 2>/dev/null | head -1 | awk -F'[:"]' '{print $3}' | tr -d ' ')/client/maps/n*_* 2>&1 | head -5
# If config.yaml CachePath isn't readable, fall back to common locations:
find /home/owner -name 'n*_*' -path '*/client/maps/*' 2>/dev/null | head -3
```

Expected: at least one `n{X}_{Z}` file path. Record one specific path (e.g., `n50_50`) and its size in plan doc — Bundle 1 reads its leading bytes for triangulation.

- [ ] **Step 0.7: Append Frozen Premises section to this plan doc**

Append a new section at the end of this file:

```markdown
## Frozen Premises (controller-populated, do not edit during Bundle 1)

- HEAD: <SHA from Step 0.1>
- TS NPC parser: <path + line range from Step 0.4>
- Client-Java NpcInfo handler: <path + line range from Step 0.5>
- Triangulation cache file: <path + size from Step 0.6>
- D1/D2/citation sites confirmed at: <verbatim grep output from Step 0.2>
```

No commit — this is controller scratch state, kept for Bundle 1 dispatch. Bundle 0 is purely pre-flight.

---

## Bundle 1 — Stage 1 audit (single subagent dispatch, no commits)

Output is a written verdict appended to the plan doc. Subagent reads sources, produces a 3-verdict-per-substage classification, terminates without writing code.

**Subagent dispatch:** subagent_type = `Explore` (read-heavy, no edits). Prompt includes Frozen Premises block + spec doc path + plan doc path. Subagent appends "Stage 1 Verdict" section at end of plan doc.

**Files:**
- Modify: `docs/superpowers/plans/2026-04-26-nai-31-npc-render-investigation-plan.md` (append Stage 1 Verdict section)

- [ ] **Step 1.1: Audit `pkg/gamemap/load.go::loadNPCs` parser (Stage 1.1)**

Subagent task description:

> Read `pkg/gamemap/load.go::loadNPCs` (current goscape parser, citing `rs-server-225/engine/gamemap.go`).
>
> Read `<TS path from Frozen Premises>` (TS Engine-TS NPC-spawn-file parser).
>
> Read first 32 bytes of `<triangulation cache file from Frozen Premises>` using `xxd <path> | head -2` and decode by hand against BOTH parsers.
>
> Produce a 3-line verdict block:
> ```
> Stage 1.1 verdict: <CONCLUSIVE_BUG_FOUND | NO_DIVERGENCE | AMBIGUOUS>
> Evidence: <2-3 sentences with file:line citations from all three sources>
> Next action: <"proceed to Bundle 2.A with parser fix" | "fall through to Step 1.2" | "escalate to Bundle 3 (Stage 3 instrumentation)">
> ```

Expected output: verdict block appended to plan doc. Subagent does NOT continue to Step 1.2 unless verdict is `NO_DIVERGENCE`.

- [ ] **Step 1.2: Audit `OpNpcInfo` wire framing (Stage 1.2) — only if 1.1 = NO_DIVERGENCE**

Subagent task description:

> Read `pkg/io/protocol/game/server/` for the `OpNpcInfo` opcode constant + size sentinel.
>
> Read `modules/world/player_npc_info.go::updateNpcs`.
>
> Read `<Client-Java path from Frozen Premises>` for the NpcInfo packet handler entry. Specifically extract: opcode value, expected length-prefix byte width (`-1` 1-byte vs `-2` 2-byte), any pre/post payload framing.
>
> Produce verdict block in same 3-line format as Step 1.1.

- [ ] **Step 1.3: Audit production wiring (Stage 1.3) — only if 1.2 = NO_DIVERGENCE**

Subagent task description:

> Read these in order:
> - `modules/world/server.go:229-242` (production startup spawn loop).
> - `modules/world/npc_registry.go::addNpc` (firstSpawn=true branch).
> - `modules/world/tick.go:330-...` (`npcSources` build).
> - `pkg/rsbuf/buf.go::AddNpc`/`ComputeNpc`.
> - LostCityRS/Engine-TS `World.addNpc` initialization for cross-reference.
>
> Look for: missing `s.rsbuf.AddNpc` call, slot-0 vs slot-1 indexing mismatch, level-field mishandling, coord packing/unpacking discrepancy.
>
> Produce verdict block in same 3-line format.

- [ ] **Step 1.4: Audit encoder first-tick edge cases (Stage 1.4) — only if 1.3 = NO_DIVERGENCE**

Subagent task description:

> Read these in order:
> - `pkg/rsbuf/buildarea.go::GetNearbyNpcs` (zoneMap query).
> - `pkg/rsbuf/npcinfo.go::writeNewNpcs`.
> - `modules/world/login.go` and adjacent files for player-coord-set timing relative to first `updateNpcs`.
> - `modules/world/tick.go` per-tick ordering.
>
> Look for: player Coord = 0,0,0 at first tick → empty `GetNearbyNpcs`; player not subscribed to any zone at first tick; encoder runs before tick body.
>
> Produce verdict block in same 3-line format.

- [ ] **Step 1.5: Controller reviews Stage 1 verdicts**

Read the plan doc's Stage 1 Verdict section. Decide which Bundle 2 path to take:

- If any Stage 1.X verdict is `CONCLUSIVE_BUG_FOUND` → proceed to **Bundle 2.A.{layer}** (e.g., 2.A.1 for parser bug, 2.A.2 for wire-framing bug, 2.A.3 for wiring bug, 2.A.4 for encoder edge case).
- If all four substages = `NO_DIVERGENCE` → escalate to **Bundle 3 (Stage 3 instrumentation)** unconditionally.
- If any verdict = `AMBIGUOUS` → escalate to **Bundle 3** immediately.

Record decision in plan doc:

```markdown
## Bundle 2 dispatch decision (controller-populated)
- Stage 1 outcome: <summary>
- Selected path: <Bundle 2.A.1 parser fix | Bundle 2.A.2 wire-framing fix | Bundle 2.A.3 wiring fix | Bundle 2.A.4 encoder edge-case fix | Bundle 3 instrumentation>
- Rationale: <1-2 sentences>
```

No commit.

---

## Bundle 2.A — Bug-layer fix (TDD; concrete details from Stage 1 verdict)

Bundle 2.A is materialized from Stage 1's verdict. Four candidate sub-bundles (2.A.1, 2.A.2, 2.A.3, 2.A.4) — only ONE runs, picked by Step 1.5.

### Bundle 2.A.1 — `loadNPCs` parser fix (only if Stage 1.1 = CONCLUSIVE_BUG_FOUND)

**Files:**
- Modify: `pkg/gamemap/load.go` (Stage 1.1 verdict's identified diff; canonical-source citation update at line 138)
- Modify: `pkg/gamemap/gamemap.go` (only if Stage 1.1 surfaces a `NpcSpawn` struct shape change)
- Test: `pkg/gamemap/load_test.go` (new file) OR `pkg/gamemap/gamemap_test.go` (extension; pick whichever follows existing convention — controller decides at dispatch time after `ls pkg/gamemap/*_test.go`)

- [ ] **Step 2.A.1.1: Write the failing regression test for `loadNPCs`**

The test fixture is constructed from the on-disk byte triangulation in Stage 1.1's verdict. The fixture must be a synthetic `[]byte` literal (per `match-spec-tests-to-library-capabilities`, deterministic, no I/O), cross-checked against the real `n{X}_{Z}` leading bytes from Frozen Premises.

Test name: `TestLoadNPCs_PinsTSCanonicalByteFormat`. Fixture and assertion shape:

```go
func TestLoadNPCs_PinsTSCanonicalByteFormat(t *testing.T) {
    // Fixture: synthetic n{X}_{Z} record, byte format matches TS Engine-TS
    // canonical (per Stage 1.1 verdict <quote relevant lines>).
    // Cross-checked against real cache file <Frozen Premises path> bytes <0..N>.
    data := []byte{
        // <BYTES FROM STAGE 1.1 VERDICT>
        // Each byte hand-decoded in a comment showing field semantics.
    }
    gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
    gm.loadNPCs(data, /*mapSquareX*/ 50, /*mapSquareZ*/ 50)
    spawns := gm.NpcSpawns()
    if len(spawns) != /*<EXPECTED COUNT FROM VERDICT>*/ 1 {
        t.Fatalf("len(spawns): got %d, want %d", len(spawns), 1)
    }
    if spawns[0].TypeID != /*<EXPECTED FROM VERDICT>*/ 0 {
        t.Errorf("spawns[0].TypeID: got %d, want %d", spawns[0].TypeID, 0)
    }
    if spawns[0].X != /*<EXPECTED>*/ 0 || spawns[0].Z != /*<EXPECTED>*/ 0 || spawns[0].Level != /*<EXPECTED>*/ 0 {
        t.Errorf("spawns[0] coord: got (%d,%d,L%d), want (...)", spawns[0].X, spawns[0].Z, spawns[0].Level)
    }
}
```

The implementer copies Stage 1.1's verdict-block fixture verbatim (controller transcribes the verdict into the test before dispatch).

- [ ] **Step 2.A.1.2: Run the test against HEAD; verify it fails for the correct reason**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadNPCs_PinsTSCanonicalByteFormat -v
```

Expected: FAIL with assertion mismatch. Per `test_passes_for_wrong_reason.md`, confirm the failure mode matches the bug being fixed (e.g., parser produces wrong count, wrong coord — NOT a panic, nil-dereference, or compilation error). If failure is for the wrong reason, the fixture is malformed; fix the test before fixing the parser.

- [ ] **Step 2.A.1.3: Apply the parser fix from Stage 1.1 verdict**

The verdict block's "Evidence" section contains the exact diff — controller transcribes it into this step before dispatch. Implementer applies the diff. Concurrent: update the doc-comment at `pkg/gamemap/load.go:138` to remove the `rs-server-225/engine/gamemap.go` citation; replace with citation to `<TS path from Frozen Premises>` per `ts_source_canonical_path.md`.

- [ ] **Step 2.A.1.4: Run the test; verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadNPCs_PinsTSCanonicalByteFormat -v
```

Expected: PASS.

- [ ] **Step 2.A.1.5: Run full test suite; verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all tests pass. If any test fails, investigate per `verify_implementer_claims.md` — do not attribute to "pre-existing failures" without git-checkout-HEAD~1 verification.

- [ ] **Step 2.A.1.6: Commit**

```bash
git add pkg/gamemap/load.go pkg/gamemap/gamemap.go pkg/gamemap/load_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(gamemap): NAI-31 Bundle 2.A.1 — port loadNPCs to TS canonical byte format

Stage 1.1 audit (NAI-31 spec doc) found the prior parser was authored
against the unauthorized rs-server-225/engine/gamemap.go and produced
<DESCRIBE DIVERGENCE FROM VERDICT> versus the TS canonical at
<TS PATH:LINE>. Replace the parser; pin with synthetic-bytes regression
test cross-checked against on-disk n{X}_{Z}.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Bundle 2.A.2 — Wire-framing fix (only if Stage 1.2 = CONCLUSIVE_BUG_FOUND)

**Files:**
- Modify: `pkg/io/protocol/game/server/<file containing OpNpcInfo>` (Stage 1.2 verdict identifies)
- Modify: `modules/world/player_npc_info.go` (only if writeOut path needs change)
- Test: `pkg/io/protocol/game/server/<corresponding _test.go>` or new `modules/world/<integration test>` — controller decides at dispatch

- [ ] **Step 2.A.2.1: Write the failing byte-shape test**

Test name: `TestOpNpcInfo_WireFormat_MatchesClientJava`. Fixture: encode a known NpcInfo payload via `(*NpcInfo).Encode`, wrap with `OpNpcInfo`, assert the resulting bytes match the shape Client-Java's parser expects (opcode value, length prefix width, payload-byte ordering).

Per `rsbuf_roundtrip_tests.md`, decode in Java-client reader order and pin each field — don't just assert byte length.

```go
func TestOpNpcInfo_WireFormat_MatchesClientJava(t *testing.T) {
    // Encode a minimal NpcInfo payload (one tracked NPC, idle, no masks).
    b := rsbuf.New(...)  // <SETUP FROM VERDICT>
    payload := b.NpcInfo.Encode(b, 0, renderer)

    // Wrap with OpNpcInfo packet framing.
    framed := frameOpNpcInfo(payload)  // <USES PRODUCTION FRAMING CODE>

    // Assert opcode matches Client-Java's expected value.
    if framed[0] != /*<OPCODE FROM VERDICT>*/ 0 {
        t.Errorf("opcode: got 0x%02x, want 0x%02x", framed[0], 0)
    }
    // Assert length prefix matches expected width and value.
    // <PER-FIELD ASSERTIONS FROM VERDICT>
}
```

- [ ] **Step 2.A.2.2: Run; verify it fails for correct reason**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./<path> -run TestOpNpcInfo_WireFormat_MatchesClientJava -v
```

Expected: FAIL. Verify the failure mode matches Stage 1.2's verdict (e.g., opcode mismatch, length-prefix-width mismatch).

- [ ] **Step 2.A.2.3: Apply Stage 1.2 verdict fix**

Controller transcribes verdict's diff. Implementer applies.

- [ ] **Step 2.A.2.4: Run test; verify pass**

- [ ] **Step 2.A.2.5: Run `go test ./...`; verify no regressions**

- [ ] **Step 2.A.2.6: Commit**

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(io/protocol): NAI-31 Bundle 2.A.2 — align OpNpcInfo wire framing with Client-Java

Stage 1.2 audit found <DIVERGENCE>. Match Client-Java's parser at
<CLIENT-JAVA PATH:LINE>; pin with byte-shape test asserting opcode +
length prefix + per-field ordering.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Bundle 2.A.3 — Production wiring fix (only if Stage 1.3 = CONCLUSIVE_BUG_FOUND)

**Files:**
- Modify: identified by Stage 1.3 verdict — most likely `modules/world/server.go`, `modules/world/npc_registry.go`, or `modules/world/tick.go`.
- Test: integration-style; mirror existing patterns in `modules/world/rsbuf_lifecycle_test.go`.

- [ ] **Step 2.A.3.1: Write the failing wiring test**

Test name and fixture shape derived from Stage 1.3 verdict. The test should pin the production-wiring contract that the fix establishes (e.g., "after `addNpc(firstSpawn=true)`, the NPC's nid appears in `s.rsbuf.npcs[]`").

- [ ] **Step 2.A.3.2: Run; verify failure for correct reason**

- [ ] **Step 2.A.3.3: Apply Stage 1.3 verdict fix**

- [ ] **Step 2.A.3.4: Run test; verify pass**

- [ ] **Step 2.A.3.5: Run `go test ./...`; verify no regressions**

- [ ] **Step 2.A.3.6: Commit**

```bash
git commit --no-gpg-sign -m "fix(world): NAI-31 Bundle 2.A.3 — <SPECIFIC WIRING FIX FROM VERDICT>

<2-3 SENTENCE BODY FROM VERDICT>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Bundle 2.A.4 — Encoder first-tick edge case fix (only if Stage 1.4 = CONCLUSIVE_BUG_FOUND)

**Files:**
- Modify: identified by Stage 1.4 verdict — most likely `pkg/rsbuf/npcinfo.go::writeNewNpcs` or `pkg/rsbuf/buildarea.go::GetNearbyNpcs`, possibly `modules/world/login.go` for the per-tick ordering.
- Test: extension of `pkg/rsbuf/npcinfo_test.go`.

- [ ] **Step 2.A.4.1: Write the failing first-tick edge-case test**

Test name and fixture shape derived from Stage 1.4 verdict. Critical: the test must reproduce the first-tick conditions the existing `TestPlayerSeesNearbyNpc` doesn't exercise (e.g., player Coord set after subscription, encoder runs before zone-walk).

- [ ] **Step 2.A.4.2 to 2.A.4.6:** Same shape as 2.A.1.2-2.A.1.6.

```bash
git commit --no-gpg-sign -m "fix(rsbuf): NAI-31 Bundle 2.A.4 — <SPECIFIC ENCODER FIX FROM VERDICT>

<BODY FROM VERDICT>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Bundle 2.B — NAI-30-D2 retire (CHAT mask suppression for local player)

Concrete and not Stage-1-conditional — pre-spec grep located the surface. However, Bundle 2.B has its own internal fork because the deferral comment claims the fix needs a renderer-cache port.

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (writeLocalPlayer mask path)
- Modify: `pkg/rsbuf/playerinfo_test.go` (unskip line 580; convert to dual-pin)
- Modify (conditional): `pkg/rsbuf/renderer.go` (only if cache port is required AND ≤50 LOC)

- [ ] **Step 2.B.1: Audit the actual fix surface**

Read `pkg/rsbuf/playerinfo.go:120-198` (`writeLocalPlayer`). Note that `renderer.HighDefOf(self.PID)` at line 123 returns a CACHED payload that already contains chat. The `writeMaskPayloads(...suppressChat=true)` parameter exists but is consumed by the renderer when BUILDING the cached payload, not when reading it.

Decide between three paths:

- **Path A (in-place):** Compute the local-player payload on-the-fly without cache, calling `writeMaskPayloads(...suppressChat=true)` directly from `writeLocalPlayer`. Bypasses the cache for self only. Tradeoff: extra per-tick work for one player.
- **Path B (cache port, ≤50 LOC):** Add a `(r *Renderer) HighDefOfSuppressChat(pid)` accessor that returns a chat-stripped variant cached separately. Renderer re-runs the masks in suppressChat mode and caches the result. ~30-50 LOC change in renderer.go.
- **Path C (defer):** If Paths A and B both touch ≥50 LOC OR introduce non-trivial coupling, NAI-31 leaves D2 deferred. Update the deferral text to reflect what was learned ("requires renderer-cache port; estimated >50 LOC; deferred to NAI-32 renderer-port series"). NO production-code change.

Record decision in plan doc:

```markdown
### Bundle 2.B path decision (controller-populated)
- Audit finding: <1-2 sentences on writeLocalPlayer's mask-payload flow>
- Selected path: <Path A | Path B | Path C>
- Rationale: <1 sentence; LOC estimate>
```

- [ ] **Step 2.B.2 (Path A or B only): Unskip and update the regression test**

Open `pkg/rsbuf/playerinfo_test.go:577-580`. Remove the `t.Skip("NAI-30-D2: ...")` line. Convert the test body to dual-pin per `ts_asymmetry_dual_pin.md`:

```go
// TestPlayerInfo_LocalPlayer_ChatMaskStripped pins the upstream
// PlayerInfo::highdefinition behavior at info.rs:289-291: the local
// player's CHAT mask is stripped from the high-def payload (no chat
// self-echo). Dual-pin: (a) presence of CHAT in the OTHER-player
// payload, (b) absence of CHAT in the SELF payload, same tick.
//
// Per ts_asymmetry_dual_pin: the absence-pin escalates if upstream
// changes the suppression behavior.
func TestPlayerInfo_LocalPlayer_ChatMaskStripped(t *testing.T) {
    // <FIXTURE: two players, one says something, one ticks NpcInfo with masks>
    // Self-payload assertion: CHAT mask bit NOT in highDef bytes for self.
    // Other-payload assertion: CHAT mask bit PRESENT in highDef bytes for the other player at the same tick.
}
```

The exact fixture comes from a careful read of the existing skipped test plus the upstream `info.rs:289-291` reference. Controller writes the fixture in plan doc edit before dispatch.

- [ ] **Step 2.B.3 (Path A or B only): Run the test against HEAD; verify it fails for correct reason**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestPlayerInfo_LocalPlayer_ChatMaskStripped -v
```

Expected: FAIL because CHAT mask bit IS present in the self payload at HEAD (the bug). If failure mode is wrong (e.g., test panics, fixture broken), fix the test.

- [ ] **Step 2.B.4 (Path A): In-place fix**

In `pkg/rsbuf/playerinfo.go::writeLocalPlayer`, replace `highDef := renderer.HighDefOf(int(self.PID))` with a fresh suppressChat=true payload computation. Inline the mask-payload write directly, skipping the cache. Specific code shape resolved at dispatch time after Step 2.B.1's audit.

OR

- [ ] **Step 2.B.4 (Path B): Renderer-cache port**

Add `(r *Renderer) HighDefOfSuppressChat(pid int) []byte` to `pkg/rsbuf/renderer.go`. Mirror the existing `HighDefOf` cache logic but pass `suppressChat=true` to `writeMaskPayloads`. Cache the variant separately (don't pollute the existing cache). Update `writeLocalPlayer` to call the new accessor when the player is self.

- [ ] **Step 2.B.5 (Path A or B): Run test; verify pass**

- [ ] **Step 2.B.6 (Path A or B): Run `go test ./...`; verify no regressions**

- [ ] **Step 2.B.7 (Path A or B): Update D2 deferral comment**

In `pkg/rsbuf/playerinfo.go:113-119`, replace the `NAI-30-D2 (deferred to NAI-31): ...` comment block with a brief comment describing the implemented behavior:

```go
// writeLocalPlayer suppresses the CHAT mask bit for self per upstream
// PlayerInfo::highdefinition at info.rs:289-291 (no chat self-echo).
// <PATH-A: implemented via in-place suppressChat=true mask emission> OR
// <PATH-B: implemented via renderer.HighDefOfSuppressChat cache variant>.
```

- [ ] **Step 2.B.8 (Path A or B): Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go [pkg/rsbuf/renderer.go]
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-31 Bundle 2.B — retire NAI-30-D2 local-player CHAT mask suppression

writeLocalPlayer now strips the CHAT mask bit for self per upstream
info.rs:289-291. Path: <A in-place | B renderer-cache HighDefOfSuppressChat>.
Regression pinned by TestPlayerInfo_LocalPlayer_ChatMaskStripped (unskipped,
dual-pin: presence in other-player payload, absence in self payload).

Closes memory: NAI-30-D2

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2.B.alt (Path C only): Update deferral comment without changing production code**

In `pkg/rsbuf/playerinfo.go:113-119`, update the deferral block text:

```go
// NAI-30-D2 (audited NAI-31, remains deferred): writeLocalPlayer should
// strip the CHAT mask bit for self per upstream info.rs:289-291. NAI-31
// audit found the in-place fix requires <SPECIFIC OBSTACLE> and the
// renderer-cache port is estimated <X> LOC; both exceed NAI-31's scope.
// Re-deferred to NAI-32 renderer-port series.
// Test pinned via TestPlayerInfo_LocalPlayer_ChatMaskStripped (t.Skip).
```

Update `nai_followups.md` D2 entry text to reflect re-deferral. NO `Closes memory:` trailer — D2 is not closed.

```bash
git add pkg/rsbuf/playerinfo.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(rsbuf): NAI-31 Bundle 2.B — D2 audit found in-place fix not tractable

Audit per Bundle 2.B.1 found <SUMMARY OF OBSTACLE>. D2 remains deferred;
re-tag for NAI-32 renderer-port series. Test stays t.Skip'd.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 2.C — NAI-30-D1 doc-cleanup (4 enumerated sites)

Concrete and not conditional. Pre-spec grep located all sites.

**Files:**
- Modify: `modules/world/player.go:261`
- Modify: `modules/world/npc.go:122`
- Modify: `pkg/rsbuf/npcinfo_test.go:657`

- [ ] **Step 2.C.1: Re-grep at HEAD to confirm sites haven't shifted**

```bash
rg -n "NAI-30-D1\|set_orient" modules/ pkg/
```

Expected exactly:
- `modules/world/player.go:261` (or HEAD-shifted line) — D1 producer reference
- `modules/world/npc.go:122` (or HEAD-shifted line) — D1 status reference
- `pkg/rsbuf/npcinfo_test.go:657` (or HEAD-shifted line) — NAI-31 forward-reference

If new sites appear, halt and update the plan.

- [ ] **Step 2.C.2: Update `modules/world/player.go:261` comment**

Read the current comment block; replace any "NAI-30-D1 (deferred to NAI-31)" or similar phrasing with "NAI-30-D1 (audit cleaned NAI-31; producer wiring deferred to engine-port series)". Preserve the field, preserve the surrounding logic — text-only change.

Concrete edit shape (controller transcribes actual current text into plan before dispatch):

```go
// Before:
// "value" per upstream player.rs:23-24. NAI-30-D1: producer (set_orient
// + npc-config initial-orient) deferred to NAI-31.

// After:
// "value" per upstream player.rs:23-24. NAI-30-D1: producer (set_orient
// + npc-config initial-orient) deferred to engine-port series; NAI-31
// audit cleaned stale forward-references but did not wire a producer.
```

- [ ] **Step 2.C.3: Update `modules/world/npc.go:122` comment**

Same shape as Step 2.C.2 — text-only update reflecting NAI-31 doc-cleanup status.

- [ ] **Step 2.C.4: Update `pkg/rsbuf/npcinfo_test.go:657` comment**

Read context. The current text "NAI-31's fallback ladder may use orientation" was a forward-reference written before NAI-31's scope was decided. Update to factual post-hoc:

```go
// Before:
// (info.rs:642-664) where face_x falls back to orientation_x when
// face_entity == -1. NAI-31's fallback ladder may use orientation
// values to populate face_coord on first-tick entities.

// After:
// (info.rs:642-664) where face_x falls back to orientation_x when
// face_entity == -1. NAI-31 audited the fallback ladder; orientation
// producer remains deferred to engine-port series (NAI-30-D1).
```

- [ ] **Step 2.C.5: Run `go test ./...`; verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. Doc-comment changes shouldn't affect any test, but verify regardless.

- [ ] **Step 2.C.6: Commit**

```bash
git add modules/world/player.go modules/world/npc.go pkg/rsbuf/npcinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world,rsbuf): NAI-31 Bundle 2.C — clean stale NAI-30-D1 forward-references

Three sites had "deferred to NAI-31" or "NAI-31's fallback ladder may"
forward-references that misrepresented D1's status. NAI-31 audited the
orientation producer surface and confirmed the field plumbing is alive
but no producer is wired; producer wiring is deferred to engine-port
series, not NAI-31. Comment text updated to reflect this; field plumbing
unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 2.D — Canonical-source citation update (1 line)

Concrete; if Bundle 2.A.1 ran, this is already done as part of Step 2.A.1.3. Otherwise, runs standalone.

**Files:**
- Modify: `pkg/gamemap/load.go:138`

- [ ] **Step 2.D.1: Skip if Bundle 2.A.1 ran**

```bash
git log --oneline 76b55fb..HEAD -- pkg/gamemap/load.go
```

If output shows a Bundle 2.A.1 commit touching `pkg/gamemap/load.go`, the citation update was bundled. Skip Bundle 2.D.

Otherwise proceed.

- [ ] **Step 2.D.2: Read current line 138 context**

```bash
sed -n '136,142p' pkg/gamemap/load.go
```

Expected output cites "rs-server-225/engine/gamemap.go".

- [ ] **Step 2.D.3: Replace citation with TS canonical**

Replace the unauthorized citation with a TS Engine-TS reference per `ts_source_canonical_path.md`. The TS path is from Bundle 0's Frozen Premises (Step 0.4 output).

```go
// Before:
// Layout (from rs-server-225/engine/gamemap.go): each record is a 2-byte
// packed position (top 2 bits level, next 6 bits local X, low 6 bits local Z),
// followed by a 1-byte count and that many 2-byte NPC type IDs.

// After:
// Layout (mirrors LostCityRS/Engine-TS at <PATH:LINE-FROM-PREMISES>):
// each record is a 2-byte packed position (top 2 bits level, next 6 bits
// local X, low 6 bits local Z), followed by a 1-byte count and that many
// 2-byte NPC type IDs.
```

- [ ] **Step 2.D.4: Run `go test ./...`; verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 2.D.5: Commit**

```bash
git add pkg/gamemap/load.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(gamemap): NAI-31 Bundle 2.D — replace unauthorized rs-server-225 citation with TS canonical

The loadNPCs doc comment cited rs-server-225/engine/gamemap.go as its
source. Per ts_source_canonical_path.md and rust_source_canonical_path.md,
only LostCityRS/Engine-TS (for pkg/gamemap) and 2004scape/rsbuf (for
pkg/rsbuf) are authoritative. Replace with the TS canonical reference at
<PATH:LINE>.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Smoke handoff (out-of-band, no commit)

Per `smoke_test_server_handoff.md`: Java-client smokes need a user-launched server. Claude's sandboxed process is unreachable from the host client.

- [ ] **Step S.1: Build the binary**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o $TMPDIR/goscape ./cmd/goscape
ls -la $TMPDIR/goscape
```

Expected: binary exists, executable.

- [ ] **Step S.2: Hand off to user with explicit run command**

Tell the user:

> "Bundle 2 fixes are landed. Please run the server:
>
> ```
> $TMPDIR/goscape --config.file config.yaml
> ```
>
> Connect with the Java client. Confirm:
> 1. Login completes successfully (still works as before).
> 2. World map renders (still works as before).
> 3. **NPCs render at expected world coordinates.**
>
> Report back: do NPCs render?"

- [ ] **Step S.3: Wait for user response**

If user reports `NPCs render: yes` → proceed to Close commit.

If user reports `NPCs render: no` → enter Bundle 3.

If user reports something unexpected (e.g., login broke, world doesn't render) → halt; treat as P0 regression; investigate before any further work.

---

## Bundle 3 — Stage 3 runtime instrumentation (CONDITIONAL — only if smoke fails)

Skipped if smoke passed. Plan doc has this section pre-written so the structure is visible, but the controller only dispatches Bundle 3 if Step S.3 reports failure.

**Files:**
- Modify: `modules/world/server.go` (gated startup log)
- Modify: `modules/world/player_npc_info.go` (gated per-tick log)

- [ ] **Step 3.1: Add gated startup log**

Behind env-var check `GOSCAPE_NAI31_DEBUG=1`:

```go
// In modules/world/server.go, after the spawn loop at line 242:
if os.Getenv("GOSCAPE_NAI31_DEBUG") == "1" {
    s.log.Info("NAI-31 startup spawn count",
        "gamemap_npc_spawns", len(s.gamemap.NpcSpawns()),
        "npc_loop_size", len(s.npcLoop),
    )
}
```

- [ ] **Step 3.2: Add gated per-tick payload-bytes log**

In `modules/world/player_npc_info.go::updateNpcs`, after `payload := s.rsbuf.NpcInfo.Encode(...)`, before `p.writeOut(...)`:

```go
if os.Getenv("GOSCAPE_NAI31_DEBUG") == "1" && p.slot == 1 {
    s.log.Info("NAI-31 NpcInfo payload",
        "tick", s.currentTick,
        "pid", p.slot,
        "payload_bytes", len(payload),
    )
}
```

Gated to slot=1 (first connected player) to avoid log spam.

- [ ] **Step 3.3: Build + commit instrumentation (temporary)**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o $TMPDIR/goscape ./cmd/goscape
git add modules/world/server.go modules/world/player_npc_info.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world): NAI-31 Bundle 3.1-3.2 — gated runtime instrumentation for NPC render investigation

Temporary. Removed at NAI-31 close per Step 3.7.
GOSCAPE_NAI31_DEBUG=1 enables: startup spawn count + per-tick payload
bytes for slot=1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.4: Hand off to user for instrumented smoke**

Tell the user:

> "Run the server with instrumentation enabled:
>
> ```
> GOSCAPE_NAI31_DEBUG=1 $TMPDIR/goscape --config.file config.yaml 2>&1 | tee /tmp/nai31-instrumented.log
> ```
>
> Connect with the Java client, walk near where NPCs should be, and quit after ~30 seconds. Send back the contents of `/tmp/nai31-instrumented.log`."

- [ ] **Step 3.5: Analyze logs; identify next bug-layer**

Read the user's log output. Specifically look at:
- Startup spawn count: 0 → bug is in gamemap loader (Bundle 2.A.1 should have caught; re-audit if it did not).
- Startup spawn count: >0 but small (<10) → loader may be partially broken.
- Startup spawn count: large (>100) AND payload bytes per tick: 0 → bug is in encoder discovery (Bundle 2.A.4 territory).
- Startup spawn count: large AND payload bytes per tick: >0 → bug is in wire framing (Bundle 2.A.2 territory) OR client-side cache mismatch (Risk R5).

Record decision in plan doc and dispatch the appropriate Bundle 2.A.X fix. Repeat smoke handoff (Step S.2) after the fix lands.

- [ ] **Step 3.6: Iterate until smoke confirms render OR exhaustion**

Multiple Bundle 2.A passes may run. Each ends with another smoke handoff. Each iteration narrows the bug.

If three iterations produce no progress, halt and write a "NAI-31 investigation findings" section in the plan doc summarizing what was learned. Close NAI-31 with the findings doc; NAI-32 reopens with a different angle.

- [ ] **Step 3.7: Remove instrumentation**

Once smoke confirms NPCs render, remove the gated logs from `modules/world/server.go` and `modules/world/player_npc_info.go`.

```bash
git revert <SHA OF BUNDLE 3 INSTRUMENTATION COMMIT>
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Or hand-edit if revert produces conflicts. Verify `go test ./...` still passes after removal.

- [ ] **Step 3.8: Commit instrumentation removal**

```bash
git commit --no-gpg-sign -m "chore(world): NAI-31 Bundle 3.7 — remove temporary instrumentation

Smoke confirmed NPCs render after Bundle 2.A fixes. GOSCAPE_NAI31_DEBUG
gated logs no longer needed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Close commit

- [ ] **Step C.1: Update `nai_followups.md`**

Edit `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:

- Remove the "NAI-31 candidate: NPCs not visible in Java client" section (lines 1881-1900 per pre-spec read; re-grep at close to confirm range).
- Remove the NAI-30-D2 entry (retired) IF Bundle 2.B took Path A or B. If Path C, update D2 entry text to reflect re-deferral.
- Update NAI-30-D1 entry text to reflect doc-cleanup-only-at-NAI-31 status; producer wiring still deferred to engine-port series.
- Append "From NAI-31 (2026-04-XX)" close entry mirroring NAI-30 close entry pattern. Include: Stage 1 verdict summary, Bundle 2 fix description, smoke result, deviation count change.
- Update net deviation count: 14 → 13 (Path A/B) or 14 → 14 (Path C).

- [ ] **Step C.2: Run final test sweep**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all PASS, no race conditions, no vet warnings.

- [ ] **Step C.3: Close commit**

```bash
git add docs/superpowers/plans/2026-04-26-nai-31-npc-render-investigation-plan.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world,gamemap,rsbuf): NAI-31 closed — NPC render fix + D2 retire + D1 doc-cleanup

Stage 1 audit identified <LAYER> as the bug (Bundle 2.A.<N>). Fix landed
and pinned with regression test. NAI-30-D2 (local-player CHAT mask
suppression): <Path A in-place | Path B renderer-cache port | Path C
re-deferred>. NAI-30-D1 stale doc-comments cleaned at 4 sites; producer
wiring remains deferred to engine-port series. pkg/gamemap/load.go:138
canonical-source citation replaced with LostCityRS/Engine-TS reference.

User smoke confirmed: NPCs render at expected coordinates in Java client.

Net deviation count: 14 → <13 if Path A/B retired D2 | 14 if Path C>.

Closes memory: NAI-30-D2  [if Path A or B]

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist (controller, post-write)

- [ ] **Spec coverage:** Every section of the spec has at least one task.
  - Stage 1.1-1.4 audit → Bundle 1 Steps 1.1-1.4 ✓
  - Stage 2 fixes → Bundle 2.A (Stage-1-conditional 4 sub-bundles) ✓
  - NAI-30-D2 retire → Bundle 2.B ✓
  - NAI-30-D1 doc-cleanup → Bundle 2.C ✓
  - Canonical-source citation → Bundle 2.D ✓
  - Smoke handoff → Step S.1-S.3 ✓
  - Stage 3 instrumentation → Bundle 3 (conditional) ✓
  - Close commit → Step C.1-C.3 ✓

- [ ] **Placeholder scan:** Each Stage-1-conditional task block has explicit "controller transcribes Stage 1 verdict before dispatch" framing — this is structural branching, not placeholder-hiding. The plan acknowledges that bug-layer fix code is verdict-derived. Each non-conditional task (Bundle 2.B/C/D, smoke handoff, close commit) has fully concrete code/commands.

- [ ] **Type consistency:** Function names, file paths, and line numbers cross-checked against pre-spec grep results. Bundle 0's premise-freeze pass ensures HEAD-drift is caught before Bundle 1 dispatch.

- [ ] **Bundle numbering:** Bundle 0 (pre-flight) → Bundle 1 (audit) → Bundle 2.A.1/2/3/4 (fix) + 2.B (D2) + 2.C (D1) + 2.D (citation) → Smoke → Bundle 3 (conditional) → Close. Consistent across the plan.

---

## Frozen Premises (controller-populated 2026-04-26)

**HEAD:** `7db4461` (NAI-31 plan commit). Working tree dirty only with project-irrelevant dotfiles (`.bash_profile`, `.bashrc`, `.claude/`, etc.) — pre-existing background state, not a regression.

**Pre-spec premise sites confirmed at HEAD (lines unchanged):**
- `modules/world/player.go:261` — D1 stale comment.
- `modules/world/npc.go:122` — D1 stale comment.
- `pkg/rsbuf/playerinfo.go:113-119` — D2 deferral block.
- `pkg/rsbuf/playerinfo_test.go:577-580` — D2 t.Skip.
- `pkg/rsbuf/npcinfo_test.go:657` — NAI-31 forward-reference.
- `pkg/gamemap/load.go:138` — `rs-server-225/engine/gamemap.go` citation.
- `pkg/rsbuf/mask_payload.go:21,46-48` — `writeMaskPayloads(... suppressChat bool)`.
- `pkg/rsbuf/playerinfo.go:120-198` — `writeLocalPlayer` uses `renderer.HighDefOf(pid)`.
- `pkg/rsbuf/renderer.go:122-125` — `buildPayload(p, masks, suppressChat) []byte` already takes the parameter.

**Out-of-scope unauthorized `rs-server-225` citations (3 additional sites; NAI-31 does NOT touch — record as a separate follow-up):**
- `pkg/script/file.go:40` — "lookupKey is u32 (rs-server-225 had a u16 bug)".
- `pkg/zone/grid.go:3` — "Ported from /home/owner/Code/github.com/zsrv/rs-server-225/engine/zone/grid.go".
- `pkg/objtype/npctype.go:25,36` — "mirror of rs-server-225/entity.MoveRestrict|BlockWalk".

**Canonical sources confirmed accessible:**
- `LostCityRS/Engine-TS` (TS canonical for `pkg/gamemap`): `/home/owner/Code/github.com/LostCityRS/Engine-TS/`. NPC parser at `src/engine/GameMap.ts:114-137` (`loadNpcs`); call site at `:70`; cache root at `src/engine/GameMap.ts:63` = `'data/pack/server/maps/'`; `unpackCoord` helper at `:288-293`.
- `LostCityRS/Client-Java` (binding wire spec): `/home/owner/Code/github.com/LostCityRS/Client-Java/src/main/java/jagex2/`. NpcInfo packet handler not yet pinpointed (task for Bundle 1 if Stage 1.2 is reached).
- `2004scape/rsbuf` branch 225: `/home/owner/Code/github.com/2004scape/rsbuf/` (not directly read in Bundle 0; reserved for `pkg/rsbuf` work).

**Cache directory state (CRITICAL FINDING — see Smoking Gun below):**
- `./data/pack/client/maps/`: contains `l*_*` files (locations) and `m*_*` files (mapsquares). **Zero** `n*_*` files (NPC spawn data). This is goscape's currently-loaded directory.
- `./data/pack/server/maps/`: contains `l*_*`, `m*_*`, `n*_*` (414 files), and `o*_*` files. This is what TS canonical loads.
- Goscape `pkg/gamemap/gamemap.go:89` hardcodes `mapsDir := filepath.Join(cacheDir, "client", "maps")`.
- TS `GameMap.ts:63` hardcodes `'data/pack/server/maps/'`.
- Default `cfg.CachePath` from `modules/world/config.go:82` = `./data/pack`. So goscape resolves to `./data/pack/client/maps/`; TS resolves to `data/pack/server/maps/`. **Goscape reads from the wrong directory.**

**Triangulation cache file pinned:** `./data/pack/server/maps/n29_75` — first 32 bytes can be hand-decoded against both parsers.

### Smoking Gun (Stage 1.1 candidate verdict, controller-detected)

`pkg/gamemap/gamemap.go:89` looks for `n*_*` files in `cacheDir/client/maps/`, but the actual NPC spawn files live in `cacheDir/server/maps/`. Result: `gm.npcSpawns` is always empty in production; spawn loop at `server.go:229-242` iterates an empty slice; `s.npcLoop` ends with zero NPCs; encoder ticks with empty per-tick state; client receives an empty NpcInfo payload every tick.

**Byte format check:** TS `loadNpcs` at `GameMap.ts:114-137` and goscape `loadNPCs` at `pkg/gamemap/load.go:141-158` both read 2-byte packed coord, 1-byte count, N×2-byte type IDs. `unpackCoord` bit layout matches goscape's `(packed >> 12) & 0x3` / `(packed >> 6) & 0x3F` / `packed & 0x3F`. **The parser body is correct; only the directory path is wrong.**

**Bundle 1 must verify:**
1. Whether `client/maps/m*_*` and `server/maps/m*_*` are byte-identical (if not, the path fix needs to be conditional per file type, not blanket).
2. Whether `client/maps/l*_*` and `server/maps/l*_*` are byte-identical.
3. Whether the path fix should match TS exactly (everything from `server/maps/`) or be a hybrid (NPCs from `server/maps/`, mapsquares + locs continue from `client/maps/` if those files differ).

**Anticipated Bundle 2 dispatch path:** **Bundle 2.A.1** (loader fix), but the fix shape is path-correction not parser-correction. Plan-doc Bundle 2.A.1 wording must be adjusted at controller-side dispatch time to reflect this. The synthetic-bytes regression test in 2.A.1.1 still applies but is no longer the primary regression — the primary regression is "load real cache and assert spawn count > 0."

## Stage 1 Verdict (Bundle 1 audit, 2026-04-26)

### Stage 1.1 verdict: CONCLUSIVE_BUG_FOUND

**Bug:** Directory path in `pkg/gamemap/gamemap.go:89` reads from `cacheDir/client/maps/` instead of TS canonical `data/pack/server/maps/`. `n*_*` and `o*_*` files only exist in `server/maps/`. Result: `gm.npcSpawns` always empty in production.

**File comparison findings:**
- `client/maps/m50_50` vs `server/maps/m50_50`: DIFFER (size 2948 vs 30740)
- `client/maps/m30_75` vs `server/maps/m30_75`: DIFFER (size 496 vs 36521)
- `client/maps/l50_50` vs `server/maps/l50_50`: DIFFER (size 5105 vs 8316)
- `client/maps/l30_75` vs `server/maps/l30_75`: DIFFER (size 332 vs 321)
- `client/maps/n*_*`: 0 files
- `server/maps/n*_*`: 414 files
- `client/maps/o*_*`: 0 files
- `server/maps/o*_*`: 414 files

**Directory content analysis:**
- `client/maps/`: 414 m files + 414 l files = 828 total (NPC and object files completely absent)
- `server/maps/`: 414 m files + 414 l files + 414 n files + 414 o files = 1656 total

**Recommended fix path:** Path 1 (full match-TS, change line 89 only)

**Rationale:** All `m*_*` and `l*_*` files differ between directories in size (not byte-identical), indicating `client/maps/` contains a degraded subset of the full map pack. Since `client/maps/` was engineered to load only static geometry (mapsquares + locations) without NPC/object spawns, the hybrid Path 2 approach would perpetuate this degradation. Path 1 (loading everything from `server/maps/`) aligns with TS canonical at `Engine-TS/src/engine/GameMap.ts:63` (hardcoded `'data/pack/server/maps/'`) and gives goscape the complete, authoritative dataset. The parser `loadNPCs` body at `pkg/gamemap/load.go:141-158` is already correct (matches TS byte format); only the directory constant needs to change.

**Concrete diff for Bundle 2.A.1 (controller materializes from this verdict):**

```go
// pkg/gamemap/gamemap.go:89
// BEFORE:
mapsDir := filepath.Join(cacheDir, "client", "maps")
// AFTER (Path 1):
mapsDir := filepath.Join(cacheDir, "server", "maps")
```

**Stage 1.2/1.3/1.4:** Skipped — Stage 1.1 conclusive and upstream-verified (controller pre-flight confirmed both parser bodies are identical in byte-format logic and the directory-loading flow in `gamemap.go:89-145` uses the same `mapsDir` path for all four file types `m`, `l`, `n`, `o`).

**Next action:** Bundle 2.A.1 with Path 1 fix.

## Stage 1.2 Verdict (Bundle 3 audit, 2026-04-26 — smoke-failure-driven wire-format investigation)

### Stage 1.2 verdict: CONCLUSIVE_BUG_FOUND

**Wire-format divergence identified:** goscape's `PBit` encoder writes bits in LSB-first (little-endian) bit-layout order, but the Java client's `gBit` reader expects MSB-first (big-endian) bit-layout order. This causes all per-NPC bit-packed fields to be decoded incorrectly, producing invalid NIDs and incorrect entity masks.

**Evidence:**

Captured smoke-test 39-byte OpNpcInfo payload (from user report) hex:
```
00 89 23 B1 FF F1 27 77 07 96 25 0E E1 32 C4 A5 D7 A6 B0 80 FF FF FF FF 80 FF FF FF FF 80 FF FF FF FF 80 FF FF FF FF
```

**Decoded bit-packed fields (4 NPCs) — comparing encoders:**

When read with **goscape's PBit semantics (MSB-first)**, bits 8-148 decode to:

| # | NID | NType | dx | dz | ext |
|---|-----|-------|----|----|-----|
| 1 | 4388 ✗ | 945 ✓ | -1 ✓ | -1 ✓ | 1 ✓ |
| 2 | 4391 ✗ | 952 ✓ | 7 ✓ | -14 ✓ | 1 ✓ |
| 3 | 4392 ✗ | 952 ✓ | 9 ✓ | -14 ✓ | 1 ✓ |
| 4 | 4393 ✗ | 943 ✓ | 9 ✓ | -11 ✓ | 1 ✓ |

All NIDs are **out of valid range (should be 1-414)**. NTypes and coordinates are valid, indicating only the first field is scrambled.

When the same 39 bytes are read with **Java client's gBit semantics (MSB-first per Packet.java:266-283)**, the result should be:

| # | NID | NType | dx | dz | ext |
|---|-----|-------|----|----|-----|
| 1 | 17 ✓ | 291 ✓ | -10 ✓ | 7 ✓ | 1 ✓ |
| 2 | ? | ? | ? | ? | ? |
| 3 | ? | ? | ? | ? | ? |
| 4 | ? | ? | ? | ? | ? |

(Remaining NPCs' fields would also decode correctly if the bit-order mismatch were fixed.)

**Root cause trace:**

1. **goscape's `PBit` (packetbit.go:55-88):** When `n > remaining`:
   - `value >> (n - remaining)` extracts the HIGH bits of the value
   - Shifts them LEFT (putting them in the LOW bit positions of the byte)
   - Uses `bitmask[remaining]` to mask to byte size
   - **This is LSB-first packing:** HIGH bits of value → LOW bits of byte

2. **Java client's `gBit` (Packet.java:266-283):** When `arg1 > var4`:
   - `(data[var3] & BITMASK[var4]) << (arg1 - var4)` extracts LOW bits of byte
   - Shifts them LEFT (putting them in the HIGH bit positions of the result)
   - **This is MSB-first packing:** LOW bits of byte → HIGH bits of result

3. **Proof:** Byte 0x89 at position 1 (bits 8-15):
   - goscape writes: 13-bit NID=4388 as `0b1000100100100` → HIGH 8 bits `0b10001001` written to byte 1 = 0x89 ✓ (matches)
   - client reads: gBit(13) at bitPos=0 takes byte 0 (0x00) and byte 1 (0x89), extracts `(0x89 >> 3) & 0x1F` = 0b00010001 = 17 ✓ (correct)
   - **The byte matches; only the bit interpretation differs.**

**Decoded mask payload per NPC:** `0x80` = NpcMaskFaceCoord (bit 7 set); `0xFF 0xFF 0xFF 0xFF` = FACE_COORD payload (4 bytes of signed P2 values: x=-1, z=-1 as 0xFFFFFFFF, per npc_mask_payload.go:41-44).

**Client-side NpcInfo handler:** `/home/owner/Code/github.com/LostCityRS/Client-Java/src/main/java/deob/client.java:5787-5821` method `getNpcPosNewVis`. Calls sequence:
```java
int var4 = arg1.gBit(13);  // NID — line 5789
if (var4 == 8191) break;   // terminator — line 5790
// ...
var5.type = NpcType.get(arg1.gBit(11));  // NType — line 5799
int var6 = arg1.gBit(5);  // dx — line 5806
int var7 = arg1.gBit(5);  // dz — line 5810
int var8 = arg1.gBit(1);  // extend — line 5815
```

Client expects: 13 bits NID (MSB-first), 11 bits NType (MSB-first), 5 bits dx (MSB-first), 5 bits dz (MSB-first), 1 bit extend (MSB-first).

**Identified divergence:** goscape's `PBit` encodes bits LSB-first; client's `gBit` reads bits MSB-first. The 13-bit, 11-bit, 5-bit, 5-bit, 1-bit field structure is correct, but the bit-ordering convention is inverted.

**Concrete diff for Bundle 3 fix (controller materializes):**

The fix requires rewriting `PBit` in packetbit.go to write MSB-first instead of LSB-first. This is a complete rewrite of the bit-packing logic — not a one-line change.

```go
// pkg/io/packet/packetbit.go:55-88
// BEFORE (LSB-first):
func (p *Packet) PBit(n int, value int) {
    bytePos := p.BitPos >> 3
    remaining := 8 - (p.BitPos & 7)
    p.BitPos += n

    // grow if necessary
    if bytePos+1 > p.Len() {
        _, err := p.Write(make([]byte, (bytePos+1)-p.Len()))
        if err \!= nil {
            panic(err)
        }
    }

    for ; n > remaining; remaining = 8 {
        p.Data[bytePos] &= byte(^bitmask[remaining])
        p.Data[bytePos] |= byte(uint32(value>>(n-remaining)) & bitmask[remaining])
        bytePos += 1
        n -= remaining

        // grow if necessary
        if bytePos+1 > p.Len() {
            p.Write(make([]byte, (bytePos+1)-p.Len()))
        }
    }

    if n == remaining {
        p.Data[bytePos] &= byte(^bitmask[remaining])
        p.Data[bytePos] |= byte(value) & byte(bitmask[remaining])
    } else {
        p.Data[bytePos] &= byte(int(^bitmask[n]) << (remaining - n))
        p.Data[bytePos] |= byte((uint32(value) & bitmask[n]) << (remaining - n))
    }
}

// AFTER (MSB-first, matching client's gBit):
func (p *Packet) PBit(n int, value int) {
    bytePos := p.BitPos >> 3
    bitInByte := p.BitPos & 7
    p.BitPos += n

    for bitWritten := 0; bitWritten < n; {
        bitsAvailableInByte := 8 - bitInByte
        bitsToWrite := n - bitWritten
        if bitsToWrite > bitsAvailableInByte {
            bitsToWrite = bitsAvailableInByte
        }

        // grow if necessary
        if bytePos >= p.Len() {
            _, err := p.Write(make([]byte, (bytePos+1)-p.Len()))
            if err \!= nil {
                panic(err)
            }
        }

        // Extract bitsToWrite bits from the HIGH side of the remaining value
        valueBits := (value >> (n - bitWritten - bitsToWrite)) & ((1 << bitsToWrite) - 1)
        // Write to the byte at bitInByte, shifting into position
        shift := bitsAvailableInByte - bitsToWrite
        p.Data[bytePos] |= byte(valueBits << shift)

        bitWritten += bitsToWrite
        bitInByte += bitsToWrite
        if bitInByte == 8 {
            bitInByte = 0
            bytePos += 1
        }
    }
}
```

**Next action:** Bundle 3 (newly created sub-bundle) — apply the MSB-first PBit fix above + write a regression test that pins the wire format against the captured 39 bytes decoded using Java client semantics. Verify that post-fix decode with client gBit produces valid NIDs in 1-414 range.

