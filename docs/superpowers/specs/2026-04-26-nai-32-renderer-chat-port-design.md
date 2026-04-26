# NAI-32 — Renderer dual-cache CHAT suppression port + rs-server-225 citation sweep

- **Sub-spec**: NAI-32
- **Date**: 2026-04-26
- **Scope label**: Two-bundle feature-port sub-spec — (Bundle 1) ports the upstream `info.rs:289-291` self-CHAT-mask suppression into goscape's eager-cache renderer architecture by adding a second high-def cache variant (`highDefWithChat`) consumed by `writePlayers` for tracked-other reads (sole site reading `HighDefOf` for non-self players; `writeNewPlayers` reads only the low-def caches and is unchanged); tightens `buildPayload` mask consistency by stripping CHAT from the mask set (header AND payload) when chat-suppression is requested rather than gating only the payload write; drops the now-redundant `suppressChat bool` parameter from `writeMaskPayloads`. (Bundle 2) sweeps three unauthorized `rs-server-225` provenance citations from production source files (`pkg/script/file.go:40`, `pkg/zone/grid.go:3`, `pkg/objtype/npctype.go:25,36`) and replaces them with `LostCityRS/Engine-TS` canonical-source citations per `ts_source_canonical_path.md`. Closes NAI-30-D2 (deviation count 14 → 13). Smoke gate: 2-client public-chat exchange in chat range (binding feature-correctness gate).
- **Predecessors**: NAI-31 (NPC render-pipeline investigation + D2 audit + D1 doc-cleanup) — last on `main` as `aaf4acd`
- **Source root**:
  - `LostCityRS/Engine-TS` (TS canonical for non-`pkg/rsbuf` packages per `ts_source_canonical_path.md`)
  - `2004scape/rsbuf` branch `225` (Rust canonical for `pkg/rsbuf` per `rust_source_canonical_path.md`)
  - `LostCityRS/Client-Java` (binding wire spec for OpPlayerInfo; receiver of dual-cache encoder output)

## Motivation

NAI-31 Bundle 2.B audited the NAI-30-D2 deferral and found that goscape's `pkg/rsbuf/renderer.go::ComputePlayers` calls `buildPayload(p, masks, suppressChat=true)` at all three sites (lines 36, 47, 53). All three high-def / low-def cache variants are built with CHAT stripped — meaning *every* player sees CHAT-stripped versions of *every other* player. Upstream Rust `info.rs:289-291` strips CHAT only when `player.pid == other.pid` (i.e. for self only); other players preserve their CHAT mask so the receiving client renders chat scrollback. NAI-30-D2 was deferred to NAI-31 with the comment "fix needs a 4th cache variant for other-player payloads, ~30-50 LOC"; NAI-31 audited and re-deferred to NAI-32. NAI-32 ports the fix.

Pre-flight grep at HEAD `aaf4acd` also surfaces a second, latent correctness bug on the same surface: `pkg/rsbuf/mask_header.go::writeMaskHeader` writes ALL bits set in `masks` to the header byte (or 2-byte big-mask form) and does NOT consult `suppressChat`. `pkg/rsbuf/mask_payload.go::writeMaskPayloads` gates ONLY the CHAT body bytes on `!suppressChat` (line 46). When `Renderer.ComputePlayers` calls `buildPayload(p, masks, suppressChat=true)` for a player whose `p.Masks() & MaskChat != 0`, the cached high-def bytes have the CHAT header bit SET but the CHAT payload bytes ABSENT — a wire-format-level mismatch the receiving client would mis-parse (read CHAT header bit, attempt to consume CHAT body bytes, get the next player's data instead). This bug is undiscovered because (a) no test currently pins the suppressed-with-CHAT-set wire shape, and (b) NAI-31's smoke test did not include a chatting player. NAI-32's `buildPayload` consistency fix retires this bug as a side effect.

NAI-31 Bundle 0 also surfaced 3 production-source `rs-server-225` provenance citations beyond the one it retired in `pkg/gamemap/load.go:138`. Per `ts_source_canonical_path.md` and `rust_source_canonical_path.md`, only `LostCityRS/Engine-TS` (everything outside `pkg/rsbuf`) and `2004scape/rsbuf` branch 225 (only `pkg/rsbuf`) are authorized canonical sources for goscape. The 3 outstanding citations are off-tracker provenance fixes deferred from NAI-31 close. NAI-32 sweeps them in a small dedicated bundle.

NAI-30-D1 (orientation field plumbed without producer) is **out of scope** for NAI-32. The producer chain is the per-tick `reorient()` method called from `Engine-TS/src/engine/World.ts:995` (player) and `Engine-TS/src/engine/World.ts:1046` (npc), which lives downstream of step-completion in the pathing chain. D1 retires when the engine-port series wires the pathing/movement chain — much larger work than NAI-32's envelope. The "set_orient script" framing in the original D1 deferral text is misleading: there is no `SET_ORIENT` script opcode in `Engine-TS/src/engine/script/handlers/`; the producer is the `reorient()` step-completion update.

## Tech stack

- Go 1.26+
- Existing packages **read** from at brainstorm time:
  - `pkg/rsbuf/renderer.go` (`Renderer` struct lines 7-14; `ComputePlayers` lines 20-55 with `buildPayload` call sites at lines 36, 47, 53; `HighDefOf` accessor lines 58-63; `buildPayload` helper lines 122-128)
  - `pkg/rsbuf/mask_header.go` (`writeMaskHeader` lines 7-13 — does NOT consult `suppressChat`; writes header bits as-is)
  - `pkg/rsbuf/mask_payload.go` (`writeMaskPayloads` lines 21-49 — `suppressChat` gates only the CHAT body write at line 46-48; per-mask write helpers at lines 51-108)
  - `pkg/rsbuf/playerinfo.go` (`(*PlayerInfo).Encode` line 61; `writeLocalPlayer` line 125 reading `renderer.HighDefOf(int(self.PID))` at line 128; `writePlayers` line 208 reading `HighDefOf` at line 246 for tracked others; `writeNewPlayers` line 301 reading `LowDefFullOf` at line 321 + `LowDefNoAppOf` at line 352 for newly-visible others — NO `HighDefOf` consumption, so no swap surface; NAI-30-D2 doc-comment block lines 113-124)
  - `pkg/rsbuf/playerinfo_test.go` (`TestPlayerInfo_LocalPlayer_ChatMaskStripped` line 581 — currently `t.Skip("NAI-30-D2: requires renderer cache port for per-mask suppression; audited NAI-31, deferred to NAI-32")`)
  - `pkg/rsbuf/mask_payload_test.go` line 125 — sole external caller of `writeMaskPayloads(...suppressChat=false)`; signature-change site
  - `pkg/rsbuf/renderer_test.go` (existing `ComputePlayers` test surface — `TestComputePlayersSkipsZeroMask`, `TestComputePlayersHighDef`, `TestComputePlayersLowDefForcesAppearance`, `TestComputePlayersLowDefNoApp`)
  - `2004scape/rsbuf/src/info.rs` (lines 282-293 — upstream `highdefinition()` with `if player.pid == other.pid { masks &= !(PlayerInfoProt::CHAT as u32); }` self-strip semantic)
  - `LostCityRS/Engine-TS/src/engine/script/ScriptFile.ts` (Bundle 2 site 1 canonical citation source)
  - `LostCityRS/Engine-TS/src/engine/zone/ZoneGrid.ts` (Bundle 2 site 2 canonical citation source)
  - `LostCityRS/Engine-TS/src/engine/entity/MoveRestrict.ts` + `BlockWalk.ts` (Bundle 2 site 3 canonical citation sources)
- Modified files in `pkg/rsbuf/`:
  - `renderer.go` — Bundle 1: add `highDefWithChat [2048][]byte` field, populate in `ComputePlayers` (parallel to `highDef`), expose via new `HighDefWithChatOf(slot int) []byte` accessor (mirrors `HighDefOf` shape including bounds check). Fix `buildPayload` consistency: when `suppressChat=true`, strip CHAT from `masks` BEFORE both `writeMaskHeader` and `writeMaskPayloads` (`if suppressChat { masks &^= MaskChat }` at line 122-123 entry).
  - `mask_payload.go` — Bundle 1: drop the now-redundant `suppressChat bool` parameter from `writeMaskPayloads` (lines 19-21 signature) and the `&& !suppressChat` guard at line 46. After Bundle 1's `buildPayload` fix the bit is pre-stripped from `forceMasks`, so the guard is dead.
  - `mask_payload_test.go` line 125 — Bundle 1: update the sole external `writeMaskPayloads` call site to drop the `suppressChat` arg.
  - `playerinfo.go` — Bundle 1: at `writePlayers` line 246, swap `renderer.HighDefOf(int(otherPid))` to `renderer.HighDefWithChatOf(int(otherPid))` for the tracked-other byte read. `writeLocalPlayer` (line 128) keeps `renderer.HighDefOf(int(self.PID))` — chat-stripped, correct for self per `info.rs:289-291`. `writeNewPlayers` is NOT touched: it reads `LowDefFullOf` / `LowDefNoAppOf` only; new-adds carry low-def baseline (appearance + face) without CHAT per upstream `info.rs::write_new_players`. Strike the NAI-30-D2 doc-comment block at lines 113-124 entirely; replace with a brief inline `// CHAT bit stripped per info.rs:289-291` comment near the new `masks &^= MaskChat` step in `buildPayload`.
  - `renderer_test.go` — Bundle 1: 3 new tests pinning the dual-cache contract.
  - `playerinfo_test.go` — Bundle 1: un-skip `TestPlayerInfo_LocalPlayer_ChatMaskStripped` at line 581 and implement the body (self vs. other dual-pin).
- Modified files outside `pkg/rsbuf/`:
  - `pkg/script/file.go` line 40 — Bundle 2: doc-comment edit only.
  - `pkg/zone/grid.go` lines 3-4 — Bundle 2: doc-comment edit only.
  - `pkg/objtype/npctype.go` lines 25 and 36 — Bundle 2: doc-comment edits only.
- New files: none.
- Test files modified or created:
  - `pkg/rsbuf/renderer_test.go` (3 new tests — see § Bundle 1 testing below)
  - `pkg/rsbuf/mask_payload_test.go` (1 new test pinning header/payload consistency under chat-strip + 1 call-site signature update)
  - `pkg/rsbuf/playerinfo_test.go` (1 un-skip + body)

## Scope

In scope:
- Renderer dual-cache architecture (`highDef` chat-stripped + new `highDefWithChat` chat-preserved) for the high-def variant only. Low-def variants (`lowDefFull`, `lowDefNoApp`) remain single-cache: per upstream `info.rs::lowdefinition()` (starts line 296), low-def is for newly-visible players (always "other"), and the upstream low-def path does not branch on self vs. other for CHAT. Plan-writer should read the full `lowdefinition()` body to confirm before dispatch.
- `buildPayload` consistency fix: pre-strip CHAT from `masks` when `suppressChat=true` so header AND payload are mutually consistent.
- `writeMaskPayloads` signature simplification: drop the now-dead `suppressChat` parameter and `&& !suppressChat` guard at line 46.
- `writePlayers` swap to read from the new `HighDefWithChatOf` accessor for tracked OTHER players (sole `HighDefOf` swap site for non-self players). `writeNewPlayers` is unchanged — it reads only `LowDefFullOf` / `LowDefNoAppOf`, so the high-def CHAT-suppression asymmetry does not apply.
- `writeLocalPlayer` continues to read from `HighDefOf` for SELF (chat-stripped — correct per `info.rs:289-291`).
- Strike NAI-30-D2 doc-comment block at `playerinfo.go:113-124`; replace with brief inline cite near the new chat-strip step.
- Un-skip + implement `TestPlayerInfo_LocalPlayer_ChatMaskStripped` per `ts_asymmetry_dual_pin.md` (assert both self has no CHAT in high-def AND other has CHAT in high-def).
- 3 new `Renderer.ComputePlayers` tests pinning the dual-cache contract (chat-present → diverges; no-chat → byte-identical; masks-zero → both nil).
- 1 new `buildPayload` regression test pinning header/payload consistency under chat-strip.
- 3 doc-comment edits sweeping `rs-server-225` provenance citations to `LostCityRS/Engine-TS` canonical paths.
- 2-client smoke gate post-Bundle-1 (binding feature-correctness gate per `smoke_test_server_handoff.md`).

Out of scope:
- NAI-30-D1 retirement. Producer chain (`reorient()` per-tick step-completion in `World.ts:995/1046` + `PathingEntity.ts:128,318,336,349`) requires the engine-port series' movement/pathing chain. Defers to a future sub-spec.
- Drop the high-def cache entirely (per-mask cache + per-pair masks compute, mirroring upstream more closely). Multi-NAI architectural rewrite (~600-900 LOC). NAI-32's dual-cache approach achieves TS-canonical CHAT semantics inside the existing eager-cache architecture; full per-pair compute is deferred.
- Renaming-history cleanup at `pkg/zone/grid.go` ("renamed Grid → ZoneGrid for clarity" rationale becomes a no-op once provenance points at TS `ZoneGrid.ts`). Bundle 2 drops the rationale line.
- Self-echo behavior at the local-display layer (immediate `Player.say` echo, separate from per-tick info encoder). Out of D2's surface; verify in Bundle 0 pre-flight that this is TS-canonical and not affected by NAI-32.
- `npcinfo.go` / `npcinfo_test.go` changes. NPCs do not have a CHAT mask; the dual-cache asymmetry does not apply.

## Bundle structure

| Bundle | Surface | Cadence | Reviews | Commits |
|---|---|---|---|---|
| 0 | Controller pre-flight: re-grep all 3 citation sites + 5 D2 touchpoint lines vs. HEAD; freeze `writeMaskPayloads` / `writeMaskHeader` / `writeLocalPlayer` / `writePlayers` / `writeNewPlayers` signatures + line numbers; pre-pull canonical TS paths for the 3 citation replacements; read full `info.rs::lowdefinition()` body (starts line 296) to verify low-def-self-vs-other CHAT branching is absent (inform low-def-stays-single-cache decision); enumerate ALL `writeMaskPayloads(` call sites at HEAD (expected 1 production + 1 test = 2; fail loud if a third surfaces) per `enumerate_all_sites.md` | n/a (no commits, no subagent) | n/a | 0 |
| 1 | D2 dual-cache + `buildPayload` consistency fix + `writeMaskPayloads` signature cleanup + 5 new/changed tests + strike NAI-30-D2 doc-comment block | Full TDD (red→green→commit) per `runescript_cadence.md`; subagent-driven-development per `execution_mode_default.md` | Two-stage (spec-compliance + code-quality) per `runescript_cadence.md` | 1-3 (one main feature commit; possible follow-up for layered surface if surfaces) |
| 2 | rs-server-225 citation sweep (3 doc-comment edits) | Compressed per `compressed_cadence.md` (≤~15 LOC, doc-only) | Single pass | 1 |
| Smoke | 2-client public-chat exchange in chat range; user-launched server per `smoke_test_server_handoff.md` | Binding feature-correctness gate | n/a | 0 (or 1 follow-up if Bundle 3 needed) |
| 3 (conditional) | Smoke-failure investigation + fix per `investigation_subspec_cadence.md` template. Materialized only if smoke surfaces a layered bug. Plan-writer pre-writes a Bundle 3 template gated on smoke verdict. | Stage 1 audit → Stage 2 fix per `investigation_subspec_cadence.md` | Per audit shape | Conditional |
| Close | Standard close commit per `close_commit_memory_trailer.md` (`Closes memory:` trailer if any new entries learned) | n/a | n/a | 1 |

## Bundle 1 — D2 dual-cache port

### Architecture

`Renderer` (`pkg/rsbuf/renderer.go`) gains a parallel high-def cache slot:

```go
type Renderer struct {
    highDef         [2048][]byte // CHAT stripped from header AND payload (consumed by writeLocalPlayer for self)
    highDefWithChat [2048][]byte // CHAT preserved (consumed by writePlayers for tracked others; writeNewPlayers reads low-def caches only)
    lowDefFull      [2048][]byte // unchanged (low-def has no self-vs-other CHAT branch per info.rs:296-330)
    lowDefNoApp     [2048][]byte // unchanged
    npcHighDef      [8192][]byte // unchanged (NPC has no CHAT mask)
    npcLowDef       [8192][]byte // unchanged
}
```

`ComputePlayers` populates both player high-def variants in lockstep:

```go
if masks == 0 {
    r.highDef[slot] = nil
    r.highDefWithChat[slot] = nil
} else {
    r.highDef[slot] = buildPayload(p, masks, true)         // CHAT stripped
    r.highDefWithChat[slot] = buildPayload(p, masks, false) // CHAT preserved
}
```

(Existing `lowDefFull` / `lowDefNoApp` blocks unchanged.)

Accessor:

```go
func (r *Renderer) HighDefWithChatOf(slot int) []byte {
    if slot < 1 || slot >= len(r.highDefWithChat) {
        return nil
    }
    return r.highDefWithChat[slot]
}
```

`buildPayload` (`pkg/rsbuf/renderer.go:122`) gains the consistency fix:

```go
func buildPayload(p PlayerSource, masks int, suppressChat bool) []byte {
    if suppressChat {
        masks &^= MaskChat // CHAT bit stripped per info.rs:289-291; header AND payload omit CHAT
    }
    buf := packet.NewPacket(nil)
    writeMaskHeader(buf, masks)
    writeMaskPayloads(buf, p, masks)
    return append([]byte(nil), buf.Data...)
}
```

`writeMaskPayloads` (`pkg/rsbuf/mask_payload.go:21`) loses the parameter:

```go
func writeMaskPayloads(buf *packet.Packet, p PlayerSource, forceMasks int) {
    // ... unchanged per-mask writes ...
    if forceMasks&MaskChat != 0 {
        writeChat(buf, p)
    }
}
```

(Drops `suppressChat bool` param; drops `&& !suppressChat` guard at line 46. Doc-comment at lines 19-20 updated to match new signature.)

`PlayerInfo` (`pkg/rsbuf/playerinfo.go`):

- `writeLocalPlayer` line 128 — keep `renderer.HighDefOf(int(self.PID))` (chat-stripped; correct for self).
- `writePlayers` line 246 — swap to `renderer.HighDefWithChatOf(int(otherPid))` for tracked others.
- `writeNewPlayers` (line 301) — UNCHANGED. Reads `LowDefFullOf` (line 321) + `LowDefNoAppOf` (line 352) only; no high-def consumption, so no swap surface.
- Strike doc-comment block at lines 113-124 entirely. Add brief inline `// CHAT bit stripped per info.rs:289-291` near the new `masks &^= MaskChat` step in `buildPayload`.

### Components touched

| File | Change | Approx LOC delta |
|---|---|---|
| `pkg/rsbuf/renderer.go` | + `highDefWithChat` field; + `HighDefWithChatOf` accessor; + dual-build in `ComputePlayers`; + `masks &^= MaskChat` in `buildPayload` | +20 / -1 |
| `pkg/rsbuf/mask_payload.go` | − `suppressChat` arg + guard | +0 / -3 |
| `pkg/rsbuf/mask_payload_test.go` | call-site update (line 125) | +0 / -1 |
| `pkg/rsbuf/playerinfo.go` | strike doc-comment block lines 113-124; 1 swap site (`writePlayers` line 246); brief inline cite | +1 / -12 |
| **Production subtotal** | | ~+22 / -17 |
| `pkg/rsbuf/renderer_test.go` | 3 new tests | +60 |
| `pkg/rsbuf/mask_payload_test.go` | 1 new test | +20 |
| `pkg/rsbuf/playerinfo_test.go` | un-skip + implement | +35 / -1 |
| **Test subtotal** | | ~+115 |

Total Bundle 1: ~+137 / -18 LOC. (LOC envelope larger than NAI-31 spec's "~30-50 LOC" estimate because that estimate covered only production-code change; it excluded the consistency-fix surface and tests.)

### Data flow (encode time)

```
ComputePlayers(players)        # once per tick, before any encoder
  for each player:
    if masks == 0:
      highDef[slot] = nil
      highDefWithChat[slot] = nil
    else:
      highDef[slot]         = buildPayload(p, masks, true)   # CHAT-stripped header AND payload
      highDefWithChat[slot] = buildPayload(p, masks, false)  # CHAT preserved
    # lowDefFull / lowDefNoApp unchanged

PlayerInfo.Encode(buf, pid_self, renderer):
  writeLocalPlayer(self, renderer):
    bytes = renderer.HighDefOf(self.PID)         # CHAT stripped (info.rs:289-291)
    ...
  writePlayers(self, renderer):
    for each tracked_other:
      bytes = renderer.HighDefWithChatOf(other.PID)  # CHAT preserved
      ...
  writeNewPlayers(self, renderer):                   # UNCHANGED
    for each new_other:
      lowDef = renderer.LowDefFullOf(other.PID)      # baseline payload (no CHAT)
      ...                                            # appearance-dedup branch reads LowDefNoAppOf
```

### Testing

3 new `pkg/rsbuf/renderer_test.go` tests:

1. **`TestComputePlayers_DualHighDef_ChatPresent`** — player at slot=1 with `masks = MaskChat | MaskAnim` and chat data set. Run `ComputePlayers([]PlayerSource{p})`. Decode `r.HighDefOf(1)` and `r.HighDefWithChatOf(1)` independently:
   - `HighDefOf(1)`: header byte does NOT have CHAT bit set; payload bytes are anim-only (no chat color/effect/rights/text).
   - `HighDefWithChatOf(1)`: header byte HAS CHAT bit set; payload bytes contain anim + chat (color, effect, rights, length-prefix, text).

2. **`TestComputePlayers_DualHighDef_NoChat_Identical`** — player at slot=1 with `masks = MaskAnim` only (no chat). Run `ComputePlayers`. Assert `bytes.Equal(r.HighDefOf(1), r.HighDefWithChatOf(1))` — both byte-identical when CHAT is not in masks. Canary: ensures the dual-cache change does not drift non-CHAT outputs.

3. **`TestComputePlayers_DualHighDef_MasksZero_BothNil`** — player at slot=1 with `masks = 0`. Run `ComputePlayers`. Assert both `r.HighDefOf(1) == nil` and `r.HighDefWithChatOf(1) == nil`. Regression-pin against accidentally building a CHAT-less payload for the no-mask case.

1 new `pkg/rsbuf/mask_payload_test.go` test:

4. **`TestBuildPayload_HeaderPayloadConsistent_ChatStripped`** — call `buildPayload(p_with_chat_data, MaskChat | MaskAnim, suppressChat=true)`. Decode the result:
   - Header byte: CHAT bit NOT set (only MaskAnim bit set).
   - Payload bytes: anim block only, no chat block.
   - Pins the consistency fix as a regression invariant. Without the fix this test would fail at HEAD (header would have CHAT bit, payload would lack it).

1 un-skipped `pkg/rsbuf/playerinfo_test.go` test:

5. **`TestPlayerInfo_LocalPlayer_ChatMaskStripped`** (line 581) — un-skip; implement body. Setup: `Buf` with self at PID=1 + tracked other at PID=2; both `masks = MaskChat`; both have distinct chat strings (e.g. "self-chat" / "other-chat"). Run `pi.Encode(b, 1, r)`. Decode the output bytes per the high-def block layout in `info.rs::write_blocks` order:
   - Self's high-def block (PID=1): header byte has CHAT bit clear; no chat body bytes follow.
   - Other's high-def block (PID=2): header byte has CHAT bit set; chat body bytes equal "other-chat" payload (color/effect/rights/length/text) at the per-mask wire encoding.

   Per `rsbuf_roundtrip_tests.md`: decode in Java-client reader order using `pkg/io/packet`'s read-side methods (`G1`, `G2`, `GData`, etc.), not by hand-indexing the byte slice. Per `ts_asymmetry_dual_pin.md`: the test pins BOTH the presence (other's CHAT preserved) AND the conspicuous absence (self's CHAT stripped) — symmetry escalates if upstream ever changes either side.

Pre-existing test guard (per `latent_bug_at_migration_boundary.md`):

At Bundle 1 green, run `go test ./pkg/rsbuf/... -count=1` (cache-busted) and verify the full test surface stays green. Any test that flips RED at this cutover is investigated as a possible latent-bug surface, NOT migration noise. Particular candidates: tests pinning tracked-other or new-other byte shapes against a `masks = MaskChat`-bearing fixture (the latent header/payload mismatch was undiscovered, so any test now-failing on the CHAT header bit is the bug surfacing, not breaking).

## Bundle 2 — rs-server-225 citation sweep

3 doc-comment-only edits, no behavior change. Compressed cadence per `compressed_cadence.md`. Single review pass.

### Site 1 — `pkg/script/file.go:40`

```diff
- //   - lookupKey is u32 (rs-server-225 had a u16 bug).
+ //   - lookupKey is u32 (per Engine-TS/src/engine/script/ScriptFile.ts).
```

### Site 2 — `pkg/zone/grid.go:3-4`

```diff
- // Ported from /home/owner/Code/github.com/zsrv/rs-server-225/engine/zone/grid.go,
- // renamed Grid → ZoneGrid for clarity in the package-qualified zone.ZoneGrid form.
+ // Ported from /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/zone/ZoneGrid.ts.
```

### Site 3 — `pkg/objtype/npctype.go:25` and `:36`

```diff
- // MoveRestrict values (mirror of rs-server-225/entity.MoveRestrict).
+ // MoveRestrict values (mirror of Engine-TS/src/engine/entity/MoveRestrict.ts).
...
- // BlockWalk values (mirror of rs-server-225/entity.BlockWalk).
+ // BlockWalk values (mirror of Engine-TS/src/engine/entity/BlockWalk.ts).
```

### Verification (compressed-cadence single pass)

- `rg "rs-server-225" pkg/ modules/ cmd/` returns zero hits at green.
- `rg "rs-server-225"` (full-repo) returns hits ONLY in `docs/superpowers/` (historical spec/plan content) and memory-system files (out-of-source-tree). If a 4th `rs-server-225` reference surfaces in a production-source path, plan-writer flags as a Bundle 2 scope-expansion candidate per `retire_deviation_grep_all_comments.md`.
- `go build ./...` and `go test ./...` stay green (doc-comment-only edits — should be a no-op).

## Smoke gate (post-Bundle 1, pre-Bundle 2)

User-launched server per `smoke_test_server_handoff.md`. Two Java clients connect, log in, walk into chat range of each other. Each says public chat. Expected:

1. Each client renders BOTH chat lines (own + other's). The own-chat line may render via the local-display-layer immediate echo channel (separate from the per-tick info encoder); pre-flight Bundle 0 verifies whether this is TS-canonical and not affected by NAI-32. If self's own-chat line does NOT render at all, that's a NAI-32 regression (means the local-display channel was reading from the high-def cache, not a separate path).
2. Other's chat preserves correctly through the per-tick info encoder — the new `HighDefWithChatOf` bytes carry CHAT mask + body, the receiving client parses and renders.
3. NPCs continue to render at expected positions (NAI-31 regression-cover).
4. Walk + zone updates continue to function (broader regression-cover).
5. No client-side parsing crashes.

If smoke surfaces a layered bug (NAI-31 pattern), spawn conditional Bundle 3 per `investigation_subspec_cadence.md`. Plan-writer pre-writes a Bundle 3 template for fast materialization.

## Risk register

| ID | Risk | Mitigation | Memory tag |
|---|---|---|---|
| R1 | Latent CHAT header/payload mismatch lurks at HEAD; un-pinned by current tests; could surface as parser-crash at smoke | Bundle 1 `buildPayload` fix addresses it; pinned forward by `TestBuildPayload_HeaderPayloadConsistent_ChatStripped` | `latent_bug_at_migration_boundary.md` |
| R2 | Existing tests pinning current (CHAT-mask-set in header / no body) byte shapes flip RED at green | Treat any RED-flip as bug-surfacing not migration noise; investigate per protocol; check pre-cutover whether any test fixture has `MaskChat` set + assertion on header byte | `latent_bug_at_migration_boundary.md` |
| R3 | Implementer claims spec scope X but commits scope Y | Post-commit `git show <SHA> --stat` + `git status` per protocol | `implementer_commit_content_verify.md`, `verify_implementer_claims.md` |
| R4 | Audit subagent fabrication near-miss carryover (NAI-31 Stage 1.2) | NAI-32 does not dispatch audit subagents (no investigation phase). If smoke-fail Bundle 3 IS triggered, controller-side independent verification of any audit verdict before fix dispatch | `audit_subagent_fabrication.md` |
| R5 | `writeMaskPayloads` signature drop breaks an unenumerated caller | Bundle 0 enumerates ALL `writeMaskPayloads(` call sites at HEAD (expected 1 production + 1 test = 2); fail loud if a third surfaces | `enumerate_all_sites.md` |
| R6 | Bundle 2 misses a 4th `rs-server-225` citation outside `pkg/`/`modules/`/`cmd/` | Bundle 0 greps `rs-server-225` across the whole repo and decides per-site whether to retire (in production source) or leave (in `docs/superpowers/` historical specs/plans, where the citation is part of audit-trail context) | `retire_deviation_grep_all_comments.md` |
| R7 | `lowDefFull` / `lowDefNoApp` actually DO need self-vs-other CHAT branching (low-def-stays-single-cache assumption wrong) | Bundle 0 re-reads the full `info.rs::lowdefinition()` body (starts line 296) to verify CHAT is not branched on self-vs-other in the low-def path. If the assumption is wrong, plan-writer extends the dual-cache pattern to low-def. | `spec_test_runtime_behavior_verify.md` (analogous: verify spec assertions against canonical source at plan-write) |
| R8 | Smoke surfaces self-echo regression because immediate `Player.say` channel was reading from the high-def cache, not a separate local-display path | Bundle 0 pre-flight checks the immediate-say echo pathway; if it reads from `HighDefOf`, NAI-32 needs a 4th cache variant (OR the immediate-say channel is reading correctly from a separate local-display path and the smoke validates this). | `latent_bug_at_migration_boundary.md` (analogous: dual-path masking) |

## Deviation accounting

- Pre-NAI-32 baseline: 14 tracked deviations (per NAI-31 close: `Net deviation count: 14 → 14`, with NAI-30-D2 re-deferred to NAI-32).
- Bundle 1 retires NAI-30-D2: **14 → 13**.
- Bundle 2: no deviation impact (3 unauthorized `rs-server-225` citations are off-tracker provenance fixes carried over from NAI-31 Bundle 0; not deviations).
- Latent CHAT header/payload mismatch (R1) becomes a positive correctness improvement; NOT a new tracked deviation.
- **Net: 14 → 13.**

## Cadence summary

- Bundle 0: controller pre-flight, no commits.
- Bundle 1: full TDD cadence per `runescript_cadence.md`; subagent-driven-development per `execution_mode_default.md`; two-stage review (spec-compliance + code-quality).
- Bundle 2: compressed cadence per `compressed_cadence.md` (≤~15 LOC, doc-only); single review pass.
- Smoke: 2-client public-chat exchange post-Bundle-1; binding gate per `smoke_test_server_handoff.md`.
- Bundle 3 (conditional): templated by plan-writer per `investigation_subspec_cadence.md`; materialized only on smoke failure.
- Close: `Closes memory:` trailer per `close_commit_memory_trailer.md` if any new memory entries learned during execution.

## Memory entries anticipated at close

Defer specifics until execution. Possible candidates:

- "Header-and-payload-must-be-mutually-consistent-when-stripping-mask-bits" — feedback memory, if R1's header/payload mismatch generalizes to the NPC mask-header surface or other mask-strip code paths (Bundle 0 / Bundle 1 surfaces).
- "Dual-cache pattern for self-vs-other-asymmetric mask suppression" — project memory, if the dual-cache pattern recurs for other masks (e.g., a future SAY suppression with the same self-echo asymmetry).
- "Smoke gate for chat-channel features needs 2-client verification" — feedback memory, if the 2-client smoke proves harder to set up or surfaces a layered bug requiring Bundle 3.
