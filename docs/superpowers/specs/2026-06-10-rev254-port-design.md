# rev-254 port — 245.2→254 server delta — design

**Date:** 2026-06-10
**Status:** APPROVED (design approved in-session 2026-06-10)
**Branch:** all work lands on `rev-254` (cut from `rev-245.2` at `1e7df180`)

## Goal

`rev-254` = `rev-245.2` + the translated Engine-TS delta. Work list = the
cross-pin diff `git -C Engine-TS diff 3c16994c..43e02957` (pins to be
recorded in `main:REFERENCES.md` §rev-254, matching the goscape-client
rev-254 pins captured 2026-06-04), **plus** the `@2004scape/rsbuf` crate
delta `origin/244..origin/254` — unlike rev-245.2, the rsbuf dependency
bumps (`^244.1.0` → `254.1.0`), so `pkg/rsbuf` needs a re-audit pass.

The upstream `254` branch shares lineage with `245.2` (merge-base
`cc487e8c`), so the net cross-pin diff is the real work list: **63 files,
+1262/−524**. The compiler does NOT move: RuneScriptKt release 26
(`COMPILER_VERSION = 26` at the 254 pin), `@2004scape/rsmod-pathfinder`
stays `^5.0.4`.

**Reference pins** (verified against local checkouts 2026-06-10; all match
goscape-client `main:REFERENCES.md` §rev-254 and the upstream `254` branch
tips):

| Repo | Branch | Pinned commit |
|---|---|---|
| Engine-TS (**primary**) | `254` | `43e02957f3559c4f1aaa5680c41e5305b7ca3bfe` |
| Content | `254` | `caee3f2eb3eb3df60126e2be88c436dc2dc98e43` |
| Client-Java | `254` | `2e629784c3dcb671ee3aab134f9cb91d614d8094` |
| rsbuf | `254` (crate `254.1.0`) | `304955d5cd6896dbcd76fb2bb17736ea426cae3e` (delta vs `origin/244` tip `1defefb1` = 2 commits: `af7b150`, `304955d`) |
| RuneScriptKt | release tag `26` | jar sha256 unchanged from §rev-244 |
| cloudflare/zlib | — | unchanged from §rev-244 (`886098f3`) |

**Definition of done** (user-approved: full rev-245.2 mirror):

- (a) The Go branch diff (`git diff rev-245.2 rev-254`) corresponds
  change-for-change to the TS cross-pin diff + the rsbuf crate diff
  (PORTING-LESSONS §2 audit).
- (b) A 254 client (Client-Java `2e629784`) logs in and plays against
  goscape (B6-style smoke: login/walk/shop/music/map-cross/npc-kill).
- (c) Pack output is full-tree byte-parity against a 254 reference cache
  produced by the upstream toolchain (Engine-TS 254 + Content 254 +
  RuneScriptKt-26 jar), including content-identical `ondemand.zip`.
- (d) All `goscape-cli unpack` families (16 existing + **varbit = 17th**)
  are TS-output parity green against manifests regenerated at the 254 pin.
- (e) Suite green incl. `-race` on touched packages.

**Structure decision (user-approved):** one spec (this document) + one
implementation plan with four dependency-ordered phases. The delta is
~2.5× rev-245.2's but ~10× smaller than rev-244's — no per-bundle specs.

## Infrastructure (before Phase 1)

- New pinned worktrees `~/Code/github.com/LostCityRS/Server254-ref/
  {engine,content,javaclient}` at the pins above, mirroring the
  Server245.2-ref convention. Reference reads go through
  `git show <pin>:<path>`, never the working tree (the local checkouts
  carry `254-GOSCAPE`/`274-GOSCAPE` branches).
- `main:REFERENCES.md` gains the `## rev-254` section (table above plus
  the rsbuf-bump note).
- `config.yaml` content path repoints to the Server254-ref content
  worktree.
- Parity-gate env var `GOSCAPE_REF245_DIR` → `GOSCAPE_REF254_DIR`;
  testdata `ref245` → `ref254` (one revision per branch).

## Phase decomposition (dependency order)

### Phase 1 — engine core delta

| Item | TS source (at `43e02957`) | Go target |
|---|---|---|
| Wire opcode renumber, client→server | `ClientGameProt.ts` (every ID reassigned) | `pkg/io/protocol` client prot table + pin tests regenerated mechanically from the TS file |
| Wire opcode renumber, server→client + zone | `ServerGameProt.ts`, `ServerGameZoneProt.ts` | server/zone prot tables + pin tests; **also re-check `pkg/rsbuf` zone-op cross-table pin** (245.2 lesson: value-forks of renumbered tables) — zone values verified unchanged in this delta, the pin test re-run is the proof |
| **Input-event packet split** | `EVENT_TRACKING` monolith → 4 discrete packets: `EVENT_MOUSE_CLICK` (234, 4), `EVENT_MOUSE_MOVE` (232, −1), `EVENT_APPLET_FOCUS` (8, 1), `EVENT_CAMERA_POSITION` (91, 4); `EVENT_TRACKING` retained (142, −2). New decoder/handler/model per packet (`codec/Event*Decoder.ts`, `handler/Event*Handler.ts`, `model/Event*.ts`) | new models/decoders + handlers in `pkg/io/protocol/game/client` + `modules/world` |
| **InputTracking rewrite** | `entity/tracking/InputTracking.ts` (full rewrite: `active` flag, inline `buf` accumulation per `InputTrackingEvent` enum {CAMERA_POSITION=1, APPLET_FOCUS=2, MOUSE_CLICK=3, MOUSE_MOVE=4}, `seq` counter, 500-byte flush threshold, `onCycle()` tick flush); `Player.ts` drops `submitInput`, adds `input.flush()` in appearance reset; `World.ts` login state packet becomes `[2, min(staffModLevel,2), 1]` (was 3 staff-level variants) | goscape input-tracking equivalent in `modules/world` (rewrite to match), login flow update |
| `OPPLAYER5` | `ClientGameProt.ts` (230, 2) + `OpPlayerHandler.ts` op 5 → `ApPlayer5`/`OpPlayer5` triggers | client prot + `modules/world` player-op handler + trigger constants |
| `MAP_BUILD_COMPLETE`, `ANTICHEAT_CYCLELOGIC7` | `ClientGameProt.ts` (134, 0) / (182, 0) | client prot table (+ no-op handlers as TS has) |
| `FRIENDLIST_LOADED` packet | new `model/FriendlistLoaded.ts` + encoder (id 255, size 1: `p1 status` — 0 loading / 1 connecting / 2 online); sends at login (`Player.ts:498-500`) and friends reload (`World.ts:2006-2007`); the FRIEND_SERVER conditional **inverted** vs 245.2 | new model/encoder + registry bind + send sites in `modules/world` |
| `SET_PLAYER_OP` packet + script op | new `model/SetPlayerOp.ts` + encoder (id 204, size −1: `p1 op, p1 primary, pjstr text`); script op `SET_PLAYER_OP` (inserted after `IF_SETSCROLLPOS` → the four `*QUEUEVARARG` opcodes shift +1 again); handler pops 2 ints + 1 string, validators index∈[1,8], primary∈[0,7] | new model/encoder + `pkg/script` opcode/map/pointers/handler; regenerate the opcode-map pin |
| **Varbits (runtime)** | new `cache/config/VarBitType.ts` (server `varbit.dat` count g2; code 1 = `basevar g2, startbit g1, endbit g1`; code 250 = debugname); `Player.ts` `getVarBit`/`setVarBit` (`mask = Packet.bitmask[endbit−startbit+1]`; set: `(mask<<startbit & value<<startbit) \| (vars[basevar] & ~mask)`); script ops `PUSH_VARBIT = 25`, `POP_VARBIT = 27` (`CoreOps.ts`, secondary flag = operand bit 16, POP checks basevar protect); `VarBitValid` validator; `Packet.bitmask` made public | new `pkg/objtype/varbittype.go`; Player methods + ops in `pkg/script` handlers; validator; bitmask exposure in `pkg/io/packet` (or local table, matching existing Go idiom) |
| `STAT_TOTAL` script op | `ScriptOpcode.ts` (after STAT) + `PlayerOps.ts` (sum baseLevels) | `pkg/script` constant/map/pointers + handler |
| Pointer changes | `ScriptOpcodePointers.ts`: NPC_FINDHERO drops require2, set2 → `active_player`; OBJ_ADD/OBJ_TAKEITEM require2 → `active_player` | `pkg/script` pointers table + pin test |
| `random`/`randominc` clamp removal | `NumberOps.ts` drops `Math.max(0, …)`; JavaRandom validates (throws on ≤0) | `pkg/script` number handlers; verify Go JavaRandom port's negative handling matches |
| Cheat commands | `ClientCheatHandler.ts`: `::set`/`::get` extended to varbit names (with basevar protect guard incl. closeModal/canAccess/message); `::openoverlay <com>` / `::closeoverlay` | `modules/world` cheat handler |
| NPC stat width | `Npc.ts` levels/baseLevels `Uint8Array(6)` → `Uint16Array(6)` (storage only; wire unchanged) | `modules/world` npc entity arrays |
| Engine revision 245→254 | `Environment.ts` `ENGINE_REVISION` | `pkg/io/protocol/revision` `Expected = 254` + pin test; **grep `245` repo-wide in infra constants** (B6 lesson) |
| Max NPCs 8191→16383 | `Environment.ts` `NODE_MAX_NPCS` | the goscape max-NPCs constant (coupled to the Phase-2 rsbuf widening — land together) |
| LocType decode | `LocType.ts`: code 75 → `raiseobject` (gbool wire); code 5 → centrepiece-only model list (count g1, model g2 each, shape fixed 10) | `pkg/objtype/loctype.go` decoder + tests |
| NpcType decode | `NpcType.ts`: code 103 → `turnspeed` (g2, default 32) | `pkg/objtype/npctype.go` decoder + tests |

Gate: build/vet/test/`-race` on touched packages, plus the modules/world
suite.

### Phase 2 — pkg/rsbuf re-audit (rsbuf `origin/244..origin/254`)

Small and surgical — the crate delta is 2 commits; the behavioral one is
`304955d` "Compatibility for 254":

| Item | rsbuf source (at `origin/254`) | Go target |
|---|---|---|
| NPC id wire width 13→14 bits | `info.rs:420` `BITS_ADD` 13→14; `:460` terminator `pbit(13, 8191)` → `pbit(14, 16383)`; `:545` add `pbit(13, nid)` → `pbit(14, nid)` | `pkg/rsbuf/npcinfo.go` (`NpcTerminator`, `npcBitsAdd`, the three `PBit(13, …)` sites) + comments |
| NPC capacity 8192→16384 | `lib.rs:33`, `build.rs:84`, `renderer.rs:251-263` | `pkg/rsbuf/renderer.go` cache arrays, `pkg/rsbuf/buildarea.go` bitset init |
| Renderer autoref fix | `renderer.rs:141` (Rust-only safety, no behavior) | NOT-PORTED (no Go analog) |

Pin tests updated to the 254 values; zone-op opcodes and PlayerInfo are
unchanged in this delta (verified — the cross-table pin re-run is the
regression proof). Coupled with `NODE_MAX_NPCS` from Phase 1.

### Phase 3 — pack pipeline + reference cache + byte parity + live smoke

| Item | TS source | Go target |
|---|---|---|
| **Varbit pack family** (new) | `PackFile.ts` `VarbitPack` (`'varbit'`, `.varbit`, transmit); `config/VarbitConfig.ts` (parse `basevar` via VarpPack name-lookup, `startbit`/`endbit` numbers; client = code 1 + g2/g1/g1, server = code 250 + debugname); `PackShared.ts` readConfigs block (server `varbit.dat`/`.idx`, client jag `varbit.dat`, CRC `−1387031023`) + name-uniqueness check; `CompilerSymbols.ts` `varbit.sym` (id, debugname, basevar script type, protect) | new `pkg/pack` varbit packer + PackFile registration + symbols + CRC; map-iteration-order care (sort, per ValidateConfigPackNames pattern) |
| Loc packer | `LocConfig.ts`: stringKeys +`model2/3/4`, booleanKeys +`raiseobject` (code 75 pbool); model encode forks code 1 (model+shape pairs) vs **code 5** (centrepiece-only: count + g2 models, no shape byte) | `pkg/pack` loc config |
| Npc packer | `NpcConfig.ts`: numberKeys +`turnspeed`, code 103 p2 | `pkg/pack` npc config |
| Interface packer | `interface/PackShared.ts`: script ops 14–20 (`push_varbit` → p2 varbit id, `subtract`, `divide`, `multiply`, `coordx`, `coordz`, `push_constant` → p2 int) + opcount updates; `PackClient.ts` CRC `587792799` → `1728499832` | `pkg/pack` interface packer + CRC |
| Config CRC bumps (×8) | `config/PackShared.ts`: seq `−716271600`, loc `−826309209`, flo `−1566957964`, spotanim `−555849646`, npc `1077655221`, obj `535204494`, idk **unchanged** `−359342366`, varp `1039564548` | the config-jag CRC constants |
| Sound CRC bump | `sound/pack.ts` → `831919863` | `pkg/pack/audio` CRC |
| Map pack robustness | `map/Pack.js` skip `/`-comment lines | `pkg/pack` maps parser |
| **Generate the 254 reference cache EARLY** (first Phase-3 task) | bun run in Server254-ref engine+content | reference cache for all parity gates |
| Re-point parity gates | — | `GOSCAPE_REF245_DIR` → `GOSCAPE_REF254_DIR` (gziputil corpus, symbols-export parity incl. **new varbit.sym → 33 symbol files**, packall full-tree parity); doc strings |
| Full-tree byte parity | — | all cache files identical; `ondemand.zip` content-identical (raw-zip-bytes exception carries forward) |
| Live client smoke | Client-Java at `2e629784` | login/walk/shop/music/map-cross/npc-kill-despawn-respawn against goscape; input-event packets exercised implicitly (mouse/camera/focus) |

Notes: README, `bun.lock`, `package.json` version strings, and the
deleted `public/client/bzip2.wasm` + `client.js` hunks are NOT-PORTED (no
Go surface). Content 254 adds 6 `.varbit` files (quest_troll, quest_eadgar,
quest_horror, quest_mortton, boardgames, `_unpack/274/all.varbit`) — these
are pack INPUTS, the new family must find them via the dir tree like any
other config.

### Phase 4 — unpack + manifest regeneration

| Item | TS source | Go target |
|---|---|---|
| Driver refactor | `config/Unpack.ts`: `modelRenameOffset` computed from `cache.count(1)` (or compare cache) and threaded through every family; compare-path guard (`config2.has(type + '.idx')`); revision tag `'245'` → `'254'`; varbit wired into `unpackConfigNames` + `unpackConfig` | `pkg/unpack` driver + `cmd/goscape-cli` `-revision` default 245 → 254 |
| **Varbit unpack family** (new, 17th) | `config/VarbitConfig.ts` (code 1 → `basevar=`/`startbit=`/`endbit=`, varp resolved by name) | new `pkg/unpack` varbit family |
| Loc unpack | `LocConfig.ts`: code 5 (model/model2/…N naming via `exclusiveAdd`), code 75 (`raiseobject=yes/no`), **ldModels/code-77 path removed** | `pkg/unpack` loc family |
| Npc unpack | `NpcConfig.ts`: code 103 `turnspeed=`, model-rename guard via compare/`modelRenameOffset` | `pkg/unpack` npc family |
| Signature parity | `ObjConfig.ts`/`IdkConfig.ts`/`SpotAnimConfig.ts` (+ `interface/Unpack.ts` hunk) | corresponding families (mostly mechanical) |
| Regenerate manifests at the 254 pin | bun runs of each `tools/unpack` entrypoint vs scratch Content + the Phase-3 parity cache | `pkg/unpack/testdata/ref254` (replaces `ref245`) |
| All 17 unpack parity tests green | — | `pkg/unpack/unpacktest` harness |
| Re-verify pack parity after unpack runs | — | Phase-3 gates re-run green |

Manifest-regeneration cautions carried from B7/245.2: inspect output
substance (vacuous-merge trap — rerun with `data/pack` moved aside);
WROTE-noise subtraction; check `maps/ignore.csv` presence at the Content
254 pin before assuming the vanilla worldmap path.

## Method (fixed by PORTING-LESSONS §2/§3)

Slice the cross-pin diff per phase → translate each hunk into the
corresponding Go region applying the §3 gotcha rules → pin tests
(RED→GREEN) → tracker rows in `PORTING.md` for any deviation → gates
(`CGO_ENABLED=0 go build -trimpath ./...`, `go vet`, `go test ./...`,
`-race` on touched packages). `// TS <File>.ts:<lines>` citations refer to
the **254 pin** (`43e02957`); TS citations verified against `git show`
output before writing. Reviewers read TS via `git show <pin>:…`, never the
working tree.

## Deviation policy

The 254 pin is authoritative. Known carry-forwards and notes:

1. Existing PORTING-EXCEPTION markers carry forward unreviewed unless a
   254 hunk touches their site.
2. `EVENT_TRACKING` survives at 254 as the raw tracking blob alongside the
   4 new discrete event packets — do not delete the old packet, renumber it.
3. The NPC stat array widening (Uint16) is storage-only; serialization is
   unchanged — widening the wire would be a deviation.
4. `Player.ts:1334` transmog TODO is upstream-unimplemented — comment-only,
   no Go work.
5. New deviations get new tracker rows in `PORTING.md` §rev-254.

## Risks (ordered)

1. **InputTracking rewrite correctness** — this is the one real *rewrite*
   (not delta) in the port; the event encodings (type byte + payload per
   `InputTrackingEvent`) and the 500-byte flush must match exactly or the
   254 client's tracking round-trip breaks. Mitigation: port the TS file
   whole rather than patching the old state machine; pin tests on the
   buffer encoding per event type.
2. **Renumber typos** — ~80 wire IDs change again; regenerate pin tables
   mechanically from `git show 43e02957:…`, never by eyeballing the diff.
3. **Varbit end-to-end coupling** — runtime decode (objtype), script ops,
   pack family, symbols, and unpack family must agree on the wire format;
   a mismatch only surfaces at pack-parity or smoke time. Mitigation:
   byte-level pin tests on `varbit.dat` encode/decode both sides; parity
   gate covers the rest.
4. **Reference-cache regeneration surprises** — Content moved
   (`cbcfe670..caee3f2e`) with new `.varbit` inputs and new fonts/sprites.
   Mitigation: generate the cache as the FIRST Phase-3 task; byte-diff
   loop + `.sym` anchors isolate compiler-input vs packer-logic deltas.
5. **rsbuf bit-width landmine** — a missed 13→14 PBit site corrupts every
   NPC add block after the first; smoke shows it as garbled NPCs.
   Mitigation: the Phase-2 table lists all three sites; pin test encodes a
   known NpcInfo block and compares bytes vs hand-computed 254 layout.
6. **Save-sized arrays** (245.2 lesson) — varbits live inside varps so
   they add no new persisted arrays, but the varp registry itself may
   still grow across the boundary. Grep `make(` in save-load paths on the
   revision bump regardless.

## Testing

Per-phase pins + suite + `-race`; Phase-3 exit = full-tree byte parity +
live 254 client smoke; Phase-4 exit = 17/17 unpack manifests green + pack
parity re-verified. Close-out: PORTING.md §rev-254 audit trail with
correspondence table (a)-(e), REFERENCES.md §rev-254 on `main`.
