# rev-274 port — 254→274 server delta — design

**Date:** 2026-06-12
**Status:** CODE-COMPLETE (2026-06-13) — automated DoD (a),(c),(d),(e) met + verified; (b) live client smoke PENDING user-driven run. See PORTING.md §rev-274.
**Branch:** all work lands on `rev-274` (cut from `rev-254` at `c3b0ed7a`)

## Goal

`rev-274` = `rev-254` + the translated Engine-TS delta. Work list = the
cross-pin diff `git -C Engine-TS diff 2e3bcf43..dee467c8` — **188 files,
+13,792/−3,694** (merge-base `93b6e557`), roughly 3× the rev-254 delta.
Unlike every prior port, two external dependencies were **internalized into
TS** at 274 (rsbuf and rsmod-pathfinder), and the upstream runtime swapped
**bun → Node 24**, which retires the cloudflare/zlib gzip byte-parity
baseline.

**Reference pins** (captured 2026-06-12 = upstream `274` branch tips after
fetch; **packability VERIFIED**: the upstream reference pack built
first-run at these pins — 11.8 s, 38 MB `data/pack`, exit 0, only the
3 long-standing missing-model warnings — the rev-254 "un-packable pinned
pair" failure mode is excluded):

| Repo | Branch | Pinned commit |
|---|---|---|
| Engine-TS (**primary**) | `274` | `dee467c868e694a2d5a931e3d19e580c83666cb2` |
| Content | `274` | `7f97b0a535a885bff9846631ca78438b6a731274` |
| Client-Java | `274` | `32f30626156783de9f142306eb73a2243909dacf` |
| rsbuf (reference-only, see note) | `274` | `669116109588ab5f5d9de8c24aace1d335da5399` |
| RuneScriptTS compiler | npm `@lostcityrs/runescript` | `0.9.6` — **unchanged from §rev-254** (no compiler churn this time) |
| gzip baseline | Node 24 stock zlib | `1.3.1` (node v24.14.1) — **replaces** bun/cloudflare-zlib `886098f3`; see Risk 1 |

Notes on the pins:

- **rsbuf crate dependency DROPPED at 274** — Engine-TS ports it into TS at
  `src/network/rsbuf/` (~2.0k LOC, 13 files). The crate's own `274` branch
  (2 behavioral commits over `254`: NPC `jump` flag) is the cross-check
  reference; the TS port is the authoritative source. Verified faithful on
  the key paths (jump bit present in both).
- **rsmod-pathfinder WASM dependency DROPPED at 274** — ported into TS at
  `src/engine/routefinder/` (~2.3k LOC, 14 files). Constants match
  goscape's existing `pkg/pathfinder` port (search map 128, ring buffer
  4096, same collision flag layout) — the algorithm did not change, only
  its home.
- Client-Java `274` tip is from 2026-03-20; it is the matching 274-protocol
  client (the engine-side renumber is March-era upstream work).
- Pinned worktrees `~/Code/github.com/LostCityRS/Server274-ref/
  {engine,content,javaclient}` created at these pins (Server254-ref
  convention). Reference reads go through `git show <pin>:<path>`, never
  working trees (local checkouts carry `274-GOSCAPE` branches).

**Definition of done** (proposed: full rev-254 mirror):

- (a) The Go branch diff (`git diff rev-254 rev-274`) corresponds
  change-for-change to the TS cross-pin diff (188-file disposition audit
  in PORTING.md §rev-274; PORTING-LESSONS §2).
- (b) A 274 client (Client-Java `32f30626`) logs in and plays against
  goscape (smoke: login/walk/shop/music/map-cross/npc-kill + minimap
  toggle + skill-level appearance).
- (c) Pack output is full-tree byte-parity against the 274 reference cache
  already produced by the upstream toolchain (Node 24 + runescript 0.9.6),
  including content-identical `ondemand.zip` — **subject to the gzip
  baseline outcome (Risk 1)**: if Node-zlib output proves not bit-equal to
  the gziputil port, the parity definition for gzip members may need a
  user-approved amendment (port stock-zlib deflate, or
  decompressed-content parity for gzip members only).
- (d) All `goscape-cli unpack` family manifests regenerated at the 274 pin
  and parity green (16 manifests; **no new family at 274** — varbit stays
  inside config).
- (e) Suite green incl. `-race` on touched packages.

**Structure (proposed):** one spec (this document) + one implementation
plan with four dependency-ordered phases mirroring rev-254's shape. The
delta is large but ~60% of the added lines are the two internalized
libraries (already ported in Go) and infra goscape maps onto its own
architecture — the *net new* game-logic surface is comparable to ~1.5×
the rev-254 port.

## Architecture-mapping dispositions (the genuinely new 274 surface)

These follow goscape's standing convention: TS infra maps onto goscape's
Go architecture; only Java-client-visible wire/save/cache behavior must
be byte-faithful.

| TS 274 change | Disposition |
|---|---|
| bun → Node 24 runtime, `NodeSqliteDialect`, `bcrypt-ts`, `package-lock.json` | NOT-PORTED (Go-native equivalents already exist) — **except** the gzip-baseline consequence (Risk 1) |
| `WorldConfig.ts` (`data/config/world.json`) + `src/setup.ts` + `view/setup.ejs` setup web UI + `.env` migration | NOT-PORTED (goscape has layered config.yaml); new knobs that gate ported behavior (`node.clientRoutefinder`, `engine.revision`) map to existing goscape config |
| ws-sync internal transport for login/friend/logger threads (`InternalClient`, `ws` dep) | NOT-PORTED (goscape: gRPC login, native friends module). Message *semantics* unchanged at 274 (verified: same JSON shapes, save format SAV v7 unchanged) |
| kysely 0.29/mysql2/prisma multi-world DB backend (`db.backend`, `prisma-multi.ts`) | NOT-PORTED (goscape stays SQLite; TS default is sqlite too) |
| prom-client metrics + management port | NOT-PORTED (goscape has its own dskit instrumentation conventions; no client-visible surface) |
| `OnDemand.ts` worker-thread refactor + `OnDemandThread.ts` | Internal threading NOT-PORTED (goscape ondemand module keeps its own concurrency); **client-visible pieces ARE ported**: CRC-table format change (rolling hash, Phase 1) and any chunk-format deltas verified at plan time against goscape's ondemand serving paths |
| `src/network/rsbuf/` TS internalization | pkg/rsbuf re-audit vs the TS port (Phase 2) — small: NPC `jump` bit |
| `src/engine/routefinder/` TS internalization | pkg/pathfinder spot-audit (constants verified identical); caller-side behavior changes (AllowRepath deletion, #100) ARE ported (Phase 1) |
| `FileStream.ts` write-skip/equalsData/clearArchive, `ArtifactCache`/`FsCache`/`SourceSnapshot`/`ModelFlags` incremental-build infra | NOT-PORTED if output-neutral (they cache build artifacts; cache *bytes* unchanged) — verified during Phase 3 parity (any output drift shows up in the byte diff) |

## Phase decomposition (dependency order)

### Phase 1 — engine core delta

| Item | TS source (at `dee467c8`) | Go target |
|---|---|---|
| **Full wire renumber ×3 tables** | `ClientGameProt.ts` (every ID), `ServerGameProt.ts` (every ID), `ServerGameZoneProt.ts` (all 10 zone ops: LOC_MERGE 218→176, LOC_ANIM 30→48, OBJ_DEL 115→52, OBJ_REVEAL 8→219, LOC_ADD_CHANGE 70→138, MAP_PROJANIM 37→107, LOC_DEL 88→173, OBJ_COUNT 98→95, MAP_ANIM 114→85, OBJ_ADD 120→81) | prot tables + pin tests regenerated mechanically from `git show dee467c8:…`; **`pkg/rsbuf/zone_encoders.go` zone-table fork updated in the SAME task** (245.2 lesson e89f62fb); cross-table pin test is the regression proof |
| **EVENT_TRACKING family deleted** | client `EVENT_TRACKING` packet gone (the 4 discrete event packets survive, renumbered: MOUSE_CLICK 20, MOUSE_MOVE 222, APPLET_FOCUS 73, CAMERA_POSITION 53); server `ENABLE_TRACKING`/`FINISH_TRACKING` gone; `InputTracking.ts` itself unchanged | delete the goscape tracking-blob decoder/handler + the two server packets; keep the 4 event handlers (renumber) |
| **Login extended-revision read** | `World.ts:2135-2140` — `rev = g1(); if (rev === 0xff) rev = g2()`; revision compare vs 274 | goscape login decoder + revision pin test (274 doesn't fit u1 — this is how the client transmits it) |
| `MINIMAP_TOGGLE` packet + script op | new `MinimapToggleEncoder.ts` (194, size 1: p1 type 0/1/2) + `model/MinimapToggle.ts`; script op `MINIMAP_TOGGLE` (after MIDI_SONG) handled in `PlayerOps.ts:858` | new model/encoder + registry bind; `pkg/script` opcode/map/pointers/handler; regenerate whole opcode-map pin (implicit-numbering shifts — never hand-shift) |
| New script ops `MAP_LOC`, `SET_SKILL_LEVEL`, `NPC_DESTINATION`; `SETSKINCOLOUR`→`SETIDKCOLOUR` | `ScriptOpcode.ts` (4 inserts at different enum points + 1 rename), `ServerOps.ts:214-270` (MAP_LOCADDUNSAFE reworked to 9-zone neighborhood + new MAP_LOC), `PlayerOps.ts:1168-1177` (skillLevel; SETIDKCOLOUR pops slot+color, validates slot, `SkinColourValid` validator deleted), `NpcOps.ts:574-581` (first waypoint or current coord) | `pkg/script` constants/map/pointers/handlers + validator removal; opcode-map pin regenerated from TS |
| **Player `skillLevel` appearance field** | `Player.ts:317` new field; `Player.ts:1422` appearance stream gains `p2(skillLevel)` after combatLevel | player entity + appearance serializer (+2 bytes on the wire; SAV format v7 UNCHANGED — verified) |
| **Inventory rewrite: dirty-slot tracking** | `Inventory.ts` (−202): `InventoryTransaction` class deleted; `add`/`remove` lose `assureFullInsertion`/`forceNoStack`/`dryRun` params, return counts; `dirtySlots` set + `markDirty`/`getDirtySlots`/`resetTracking`; `NetworkPlayer.ts:339-383` sends `UpdateInvPartial` with dirty slots unless `firstSeen`; `World.ts` `inv.update=false` → `resetTracking()`; all `invAdd(…, false)` call sites simplified (`InvOps.ts`, `ClientCheatHandler.ts`, `Player.ts`) | goscape inventory + all call sites; this ripples through every inv script handler — port the TS file whole, then chase compile errors (the signature change finds every site) |
| NPC patrol/stuck/escape rework | `Npc.ts` (±159): `nextPatrolTick`+`delayedPatrol`+`wanderCounter` → `patrolDelayTicksRemaining` countdown + `stuckCounter`; patrol force-teleports at 32 stuck ticks OR level mismatch; `playerEscapeMode()` rewritten (per-axis `canTravel` validation + maxrange-aware fallback + 5-tick stuck reset) | `modules/world` npc entity + modes; pin tests on countdown/teleport edges |
| **AllowRepath deletion (#100 fix)** | `AllowRepath.ts` DELETED; `PathingEntity.ts` drops field+setter+SCRIPT-interaction arm; `Player.ts:1077` naive repath now unconditional at last waypoint; `MoveClickHandler.ts` drops the click-own-tile special case | delete goscape `AllowRepath*` (movement_consts/movement/handlers_game/interaction sites) — compile errors enumerate them |
| `exactMove` early teleport | `Player.ts:2105` — `teleport(endX,endZ,level)` up front replaces inline x/z+tele tweaking | goscape exactMove |
| IF_SETANIM allows −1; IF_SETTEXT color persistence; STRONGQUEUE `popInts(3)` | `PlayerOps.ts:699-702`, `:731-771` (per-line `@col@` carry incl. `@str@@bla@` special case), `:100-110` | corresponding `pkg/script` handlers (read the TS hunks directly — small but fiddly string logic) |
| Hunt `&` condition | `HuntType.ts:72-73` — bitwise-AND comparator | `pkg/objtype` hunt condition eval |
| **FontType 256-glyph rework** | `FontType.ts` (±88): fonts load as `p11_full/p12_full/b12_full/q8_full`, 256-glyph arrays, `CHAR_LOOKUP`/`drawWidth` deleted, quill space-advance rule, `splitText` color persistence across breaks | `pkg/objtype` fonttype + any goscape text-wrap users (chat split paths) |
| **CrcTable rolling hash** | `CrcTable.ts` (±17): CrcBuffer 36→40 bytes; exported per-archive `CrcTable[]`; `hash = 1234; hash = (hash<<1) + crc[i]` appended p4; final CRC over len−4 | goscape CRC-table builder feeding the ondemand `/crc` surface — client-visible, must be byte-faithful |
| GameMap: zip-first map load + active-loc gate | `GameMap.ts:48-98` (`.cache/maps-server.zip` via fflate, fallback dir reads), `:284-286` (static loc added to zone only if `type.active === 1`; collision regardless) | goscape map loader (zip path optional if goscape's pack layout differs — verify which artifact goscape serves; the ACTIVE gate is behavioral and must be ported) |
| World perf gates | `World.ts:573-590` hunt loop skipped when 0 players; `:979-981` info early-return when 0 players; `npc.jump` threaded into computeNpc (`:1049`) | `modules/world` tick |
| Engine revision 254→274 | `WorldConfig.ts:91` | `pkg/io/protocol/revision` Expected = 274 + pin test; **grep `254` repo-wide in infra constants** (B6 lesson) |
| Max players hardcode | `World.ts:118` — `NODE_MAX_PLAYERS` env → hardcoded 2047 | verify goscape constant + drop config knob if one exists |

Gate: build/vet/test/`-race` on touched packages + modules/world suite.

### Phase 2 — pkg/rsbuf re-audit (vs TS `src/network/rsbuf/` at the pin; crate `274` as cross-check)

| Item | Source | Go target |
|---|---|---|
| **NPC `jump` flag** | crate `npc.rs` + `info.rs` (add-leaf gains `pbit(1, jump)` between z and extend → 37-bit add leaf); TS `info.ts` add() identical; `World.ts:1049` passes `npc.jump` | `pkg/rsbuf` npc struct (field+init+cleanup), npcinfo add-leaf encoder, ComputeNpc signature + caller |
| Zone-table fork renumber | TS `prot.ts`/zone values (= ServerGameZoneProt) | `pkg/rsbuf/zone_encoders.go` (done in the same Phase-1 task; re-verified here by the cross-table pin test) |
| Everything else | verified UNCHANGED 254→274 (bit widths 11/14, capacities 2048/16384, info mask tables, view distance 15, visibility enum) | pin-test re-run only |

Player add-leaf already carries jump at 254 — NPC-side only.

### Phase 3 — pack pipeline + byte parity + live smoke

The 274 reference cache **already exists** (built this session at the
pins, first task done early per the rev-254 lesson).

| Item | TS source | Go target |
|---|---|---|
| **Gzip baseline decision (Risk 1, FIRST task)** | `GZip.ts`/`Packet.ts` unchanged code, but Node 24 zlib 1.3.1 replaces bun/cloudflare-zlib at runtime | byte-diff gziputil output vs the reference cache's gzip members across the corpus; outcomes: (i) bit-equal → no work; (ii) not equal → port stock-zlib level-6 deflate the way cloudflare/zlib was ported (gziputil keeps the old impl for rev≤254 branches), or escalate a parity-definition amendment to the user |
| Config CRC bumps (×9, idk excluded) | `config/PackShared.ts`: seq `−753410077`, loc `452815002`, flo `960212554`, spotanim `−1587698939`, npc `−1249602232`, obj `128627047`, idk unchanged `−359342366`, varp `703279713`, varbit `−234977015` | config-jag CRC constants |
| Interface + sound CRCs | `PackClient.ts` `2041671134`; `sound/pack.ts` `2127412105` | corresponding constants |
| **Font renames** | `p11/p12/b12/q8` → `*_full` through `Compiler.ts` fontmetrics, `interface/PackShared.ts` nameToFont, title/sprite packers | `pkg/pack` font-name tables + symbols (affects .if text and fontmetrics resolution) |
| Worldmap refColors LUT | `Worldmap.ts`: 78 entries changed + 2 added (slayer_tower, morytania_dark_green) | `pkg/pack` worldmap colors (regenerate from `git show`, don't hand-edit) |
| map Pack.js churn (±201) + `PackFile.ts` (±302) + `PackShared.ts` (±378) output-affecting hunks | read the raw diffs at plan time (lesson: agents miss behavioral hunks) — separate output-affecting from incremental-build-only hunks file by file | `pkg/pack` correspondingly |
| Re-point parity gates | — | `GOSCAPE_REF254_DIR` → `GOSCAPE_REF274_DIR`; gziputil corpus, symbols parity (33 .sym — varbit count unchanged), packall full-tree |
| Full-tree byte parity | — | all cache files identical; `ondemand.zip` content-identical (raw-zip-bytes exception carries forward) |
| Live client smoke | Client-Java `32f30626` | login (extended-revision handshake!)/walk/shop/music/map-cross/npc-kill; minimap toggle + setidkcolour via test script if reachable |

### Phase 4 — unpack + manifest regeneration

| Item | TS source | Go target |
|---|---|---|
| Revision tag `'254'` → `'274'` | `config/Unpack.ts:356` | `pkg/unpack` driver + `cmd/goscape-cli` default |
| Font-name output (`font=p11_full`) | `interface/Unpack.ts` | interface family |
| Signature/formatting deltas | per-family diffs (no new config codes at 274 — verified) | mostly mechanical |
| Regenerate manifests at the 274 pin | Node runs of each `tools/unpack` entrypoint vs scratch Content + the Phase-3 parity cache | `pkg/unpack/testdata/ref274` (replaces `ref254`); `Server274-ref/unpack-ref` snapshot persisted |
| All 16 unpack parity manifests green; pack parity re-verified after | — | `pkg/unpack/unpacktest` harness |

Manifest-regeneration cautions carried forward (vacuous-merge trap,
WROTE-noise subtraction, `maps/ignore.csv` presence check).

## Method (fixed by PORTING-LESSONS §2/§3)

Slice the cross-pin diff per phase → translate each hunk into the
corresponding Go region applying the §3 gotcha rules → pin tests
(RED→GREEN) → tracker rows in PORTING.md §rev-274 for any deviation →
gates (`CGO_ENABLED=0 go build -trimpath ./...`, `go vet`, `go test
./...`, `-race` on touched packages). `// TS <File>.ts:<lines>` citations
refer to the 274 pin `dee467c8`, verified against `git show` before
writing. Reviewers read TS via `git show <pin>:…`, never working trees.
Implementer briefs carry the standing directives: FOREGROUND tests only,
COMMIT YOURSELF.

## Deviation policy

The 274 pin is authoritative. Carry-forwards and notes:

1. Existing PORTING-EXCEPTION markers carry forward unreviewed unless a
   274 hunk touches their site.
2. Architecture-mapping dispositions in the table above are standing
   deviations recorded once in PORTING.md §rev-274, not per-file rows.
3. The Inventory `add`/`remove` simplification deletes validation
   parameters — goscape must delete the corresponding Go parameters, not
   keep them "for safety" (dead-divergence trap).
4. New deviations get new tracker rows in PORTING.md §rev-274.

## Gzip baseline — RESOLVED (2026-06-13, T17 investigation + user decision)

The Risk 1 fork resolved decisively. Findings:

- **Real target = the ORIGINAL r274 cache** at
  `/home/owner/Code/_runescape/r274/original-cache` (user-stated goal:
  goscape's packed cache byte-identical to the original).
- The original cache's gzip members were produced by **stock zlib 1.3.1,
  level 6 (default), gzip header XFL=0 with the OS byte (offset 9)
  zeroed** — verified: Node 24 `zlib.gzipSync` level-6 and python zlib
  1.3.1 level-6 both reproduce the original **6201/6201** non-empty gzip
  members byte-for-byte (after OS=0). goscape's current cloudflare/zlib
  `gziputil` matches only ~6% — it is the blocker.
- The Node-rebuilt reference (`Server274-ref/engine/data/pack`) is a valid
  proxy for the original: **6304/6306 members byte-identical** (compressed).
  The only divergence is **2 empty arch4 (client-maps) slots — files 704
  and 994** — which the ORIGINAL leaves empty (idx size=0) but the Content
  repo packs real map data into. That is a CONTENT difference (the Content
  map inputs include 2 maps the original omits), not a compression one;
  documented as a known original-cache deviation (goscape faithfully packs
  Content, so goscape-pack == Node-reference for those 2; only
  goscape-vs-ORIGINAL shows the 2 expected empty-slot diffs).
- **Decision (user, 2026-06-13): port a bit-exact stock-zlib-1.3.1
  level-6 deflate into `gziputil`** (CGO is off the table — the project
  ships `CGO_ENABLED=0`, so cgo-binding real zlib is not viable; a pure-Go
  port is required, the rev-244 B6 cloudflare playbook redone against stock
  zlib). The gzip wrapper (OS=0) is already correct. The Huffman/trees half
  of the existing cloudflare port is likely reusable; the new work is the
  match-finder (multiply-shift `UPDATE_HASH`, `MIN_MATCH=3`, the level-6
  config table good_length/max_lazy/nice_length/max_chain, `deflate_slow`).
  Acceptance: 6201/6201 ORIGINAL gzip members byte-exact (oracle: python
  zlib 1.3.1, already 100%). On the rev-274 branch gziputil produces
  stock-zlib output; the cloudflare path stays available for rev≤254
  branches (separate branches, self-contained).

## Risks (ordered)

1. **Stock-zlib-1.3.1 deflate port correctness (was: gzip baseline)** —
   RESOLVED-as-port (see above). The remaining risk is port fidelity: a
   match-finder/config-table deviation silently produces non-bit-exact
   output. Mitigation: corpus convergence loop against ALL 6201 original
   gzip members with a python-zlib-1.3.1 oracle; the cloudflare port's
   trees half is reused; pin a handful of members as fast unit tests +
   the full corpus as the acceptance gate.
2. **Inventory rewrite breadth** — the signature change touches every inv
   call site including script handlers and cheats; a missed
   `assureFullInsertion` semantic (TS deleted the *validation*, not just
   the param) silently changes add/remove outcomes. Mitigation: port
   `Inventory.ts` whole; pin tests on partial-add/partial-remove edges;
   UpdateInvPartial dirty-slot encoding pin test.
3. **Renumber typos** — ~90 wire IDs change across three tables plus the
   rsbuf zone fork. Regenerate mechanically from `git show`, never
   eyeball; cross-table pin test re-run.
4. **CRC-table format change** — client-visible via ondemand `/crc`; the
   40-byte buffer + rolling hash must be byte-exact or clients refuse the
   cache. Pin test against the reference cache's actual crc response.
5. **Login extended-revision handshake** — get the 0xff+u2 read wrong and
   no 274 client logs in at all (first smoke blocker). Pin test both
   forms (≤254 single-byte path must still work for older branches? No —
   each branch is revision-locked; only 0xff+274 matters here).
6. **Reference-cache staleness vs Content drift** — Content `274` moved
   as recently as 2026-06-06; the cache built today is at the pin, but
   any future pin advance re-opens Phase 3 (rev-254 lesson — re-run the
   reference build immediately after ANY pin change).

## Testing

Per-phase pins + suite + `-race`; Phase-3 exit = full-tree byte parity +
live 274-client smoke; Phase-4 exit = 16/16 manifests green + pack parity
re-verified. Close-out: PORTING.md §rev-274 audit trail with
correspondence table (a)-(e) + the close-out correspondence audit (it
caught 3 real gaps at rev-254), REFERENCES.md §rev-274 on `main`.
