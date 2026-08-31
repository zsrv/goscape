# rev-274 Engine Sync (Engine-TS 4c95f87e→1d25566c, Content 37607266→2b62ae68) — Design

**Status:** approved for planning
**Date:** 2026-08-31
**Branch:** `rev-274`
**Deviation ID prefix:** `SYNC289-D{n}` / scope boundaries `SYNC289-SB{n}`

---

## 1. Baseline correction

The task was framed as "diff `dee467c8..1d25566c`". That is **not** the work list.

goscape `rev-274` was re-pinned to Engine-TS `4c95f87e` on 2026-07-16
(`docs/superpowers/plans/2026-07-16-rev274-pin-update.md`; `main:REFERENCES.md`
§rev-274 note 5). The four commits in `dee467c8..4c95f87e` are already ported:

| Upstream | goscape commit |
|---|---|
| `3da10133` MoveClick opClick guard | `8cfab3f2` |
| `e31a8719` player/npc facing → OSRS | `3efb0ce7` |
| `3b653372` simplified model shape packing | `3e5c61fd` |
| `4c95f87e` stat-random clamp 99 | `8d9f6509` |

What did **not** happen in July was the `274-GOSCAPE` bookmark fast-forward in the
Engine-TS checkout, so that branch has read 4 commits stale ever since. It is
advanced as the final task of this plan, which is the original request's third
step.

**Actual work list: `4c95f87e..1d25566c`** — two commits.

- `1d25566c` "Updated client bundle with socket fix" — touches only
  `public/client/client.js` and `public/client/ondemandworker.js`, the minified
  JS-client bundle. goscape has no `public/client/` (it serves the Java client).
  **No-op; nothing to port.**
- `8139461a` "Synced engine with 289 improvements" — 94 files,
  +2,455/−6,716. **This is the entire port.**

Companion pin drift, both consumed by this work:

| Repo | Pinned | Now | Action |
|---|---|---|---|
| Engine-TS `274` | `4c95f87e` | `1d25566c` | advance |
| Content `274` | `37607266` | `2b62ae68d` | advance |
| Client-Java `274` | `32f30626` | `32f30626` | unchanged |

Client-Java standing still matters: it means every cache-format and wire change
below is backward-compatible with the already-pinned client. The `hasalpha` /
`hasanim` / `code9` opcodes being deleted are opcodes the client never
required, and transmogrification reuses the `-1`-sentinel appearance encoding
the client already understands.

## 2. Goal

Land upstream `8139461a` on `rev-274` true-to-TS, refresh the byte-parity
reference cache at the new Engine-TS + Content pins, and re-pin
`main:REFERENCES.md`. Every behavioral divergence gets a `SYNC289-D{n}` row.

## 3. Non-goals

- **`src/web.ts` Express→Fastify migration.** 571 lines of framework swap.
  goscape's HTTP surface is `pkg/dskit/server` + `modules/ondemand`; the
  framework is an implementation detail with no observable contract. The one
  *behavioral* change buried in that file — CRC-gated archive routes — **is** in
  scope (bundle G).
- **`public/client/**` JS-client bundle.** No goscape counterpart.
- **`eslint.config.js`, `package-lock.json`.** TS-toolchain only.
- **Mirroring the `tools/unpack/**` + `tools/render/sound/**` deletion.**
  See §5.

## 4. Bundles

Eight bundles. Ordering is forced: **A must land first** — the reference cache
cannot even be packed until the content key renames are understood, and
`smoke-pack` currently dies on stage 1 (`invalid property key in
../content/scripts/_unpack/225/all.seq: reachforward=yes`).

### A — Cache config decode/encode

Both halves of every config: the runtime decoder (`pkg/objtype/`) and the packer
(`pkg/pack/`).

| # | Change | TS |
|---|---|---|
| A1 | HuntType: new `checkObjCat` field, new server opcode **18** (`inv`,`category`,`condition`,`val`); `extracheck_var` shifts **18–20 → 19–21**, decode guard `code > 17 && code < 21` → `code > 18 && code < 22`; pack-side `check_invcat` key + mutual exclusion against the other five `check_*` keys | `HuntType.ts:93,139`; `HuntConfig.ts` |
| A2 | NpcType: `wanderrange`/`maxrange` move server opcodes **200/201 → 26/27**; `maxrange` default `7 → -1`; new `postDecode()` — `maxrange == -1 ⇒ wanderrange + 2`, then clamp `maxrange < wanderrange ⇒ wanderrange`; drop `hasanim` (client opcode 16) and `hasalpha` | `NpcType.ts:99,115,143,150,212`; `NpcConfig.ts` |
| A3 | ObjType: drop `code9` (client opcode 9); rename `manwearOffsetY`→`manwearOffset`, `womanwearOffsetY`→`womanwearOffset` (cosmetic) | `ObjType.ts:142,198`; `ObjConfig.ts` |
| A4 | SeqType: `stretches`→`reachforward` (opcode 4), `duplicatebehavior`→`duplicatebehaviour` (opcode 11) — **content-facing key renames** | `SeqType.ts:79,129,143`; `SeqConfig.ts` |
| A5 | SpotanimType: drop `hasalpha` (client opcode 3); rename `orientation`→`angle` (opcode 6) | `SpotanimType.ts:68,81`; `SpotAnimConfig.ts` |
| A6 | LocType: drop `hasalpha` (client opcode 25) | `LocType.ts:86,153`; `LocConfig.ts` |

A2 is the only entry with runtime behaviour beyond byte layout: `maxrange`
defaulting to `wanderrange + 2` changes NPC hunt/retreat geometry for every NPC
that does not set `maxrange` explicitly.

The dropped opcodes are wire-format deletions from the **client** config
archives. Because Client-Java is unchanged at `32f30626`, these opcodes were
already inert — but the CRC gates in bundle F pin the exact resulting bytes, so
they must be dropped precisely, not left unemitted-but-parsed.

### B — Collision / routefinder restructure

The largest fidelity risk in the sync. Flag semantics change meaning, and
`memory:collision_flag_naming_trap` already records that goscape's flag names
diverge from a naive reading.

| # | Change | TS |
|---|---|---|
| B1 | `NPC`→`NPC_OCC`, `PLAYER`→`BLOCK_NPC_AND_PLAYERS` (same bit values 0x80000/0x100000); new `PLAYER_OCC = 0x400000`; delete all 9 `WALL_*_ROUTE_BLOCKER` + `LOC_ROUTE_BLOCKER` flags and all 12 `BLOCK_*_ROUTE_BLOCKER` composites | `flags.ts` |
| B2 | `CollisionEngine` / `RouteFinder`: drop the `breakroutefinding` parameter from `changeLoc`/`changeWall`/`changeWallStraight`/`changeWallCorner`/`changeWallL`/`wallMask`; `changePlayer`→`changeBlock`; new `changePlayerOcc` | `CollisionEngine.ts`, `routefinder/index.ts` |
| B3 | `CollisionStrategy`: delete `LINE_OF_SIGHT_ROUTE`; `CollisionType.LINE_OF_SIGHT` no longer ORs the `>>> 13` route-flag term | `CollisionStrategy.ts` |
| B4 | New `BlockWalk.PLAYER`; `PathingEntity` gains its case (clear npc+player occupancy on the old tile, set **only** `PLAYER_OCC` on the new); `Player` constructs with `BlockWalk.PLAYER`; `Player.blockWalkFlag()` → `BLOCK_NPC_AND_PLAYERS` | `BlockWalk.ts`, `PathingEntity.ts:169`, `Player.ts:421,725` |
| B5 | `Npc.blockWalkFlag()` rewritten as a switch with two orthogonal opt-outs: `blockWalk == NONE ⇒ npcOcc = OPEN` (walks through npcs); `moverestrict == PASSTHRU ⇒ playerOcc = OPEN` (walks through players); base is always `BLOCK_NPC_AND_PLAYERS` | `Npc.ts:394` |
| B6 | `World`: npc add/remove uses `changeBlockCollision`; player removal additionally calls `changePlayerOccCollision(..., false)`; `isApproached` uses `BLOCK_NPC_AND_PLAYERS` | `World.ts:1288,1323,1617`, `GameMap.ts:468` |
| B7 | `Player.setVisibility` uses `changePlayerOccCollision` on both arms | `Player.ts:1968` |

Upstream reclaims bits 22–30 because the route-blocker subsystem was write-only:
`changeLocCollision` hardcoded `false` for `breakroutefinding`, so no
route-blocker flag was ever set.

**Verified true on the goscape side during scoping**, so B1's deletion is
dead-code removal with no behaviour change: `pkg/gamemap/gamemap.go:114-119`
passes a literal `false` as `breakRouteFinding` to every `ChangeWall`/`ChangeLoc`
call, and `ChangeLocCollision` is the only caller of either
(`modules/world/loc_turn.go:42,49`, `modules/world/world_zone.go:22,51,58,84`,
`modules/world/server.go:877`). The masks in `pkg/pathfinder/routefinder/api.go`
(lines 87, 151-154, 216-219, 281-282) and `lineOfSightBlockRoute` in
`pkg/pathfinder/collision/strategies.go:13-21` are therefore unreachable.

Not to be confused: `pkg/pack/loc.go:459` and `pkg/unpack/config/loc.go:437`
handle `breakroutefinding` as a **pack config key** (loc opcode 74). That is a
separate thing from the collision flag and stays.

### C — Entity / world correctness

| # | Change | TS |
|---|---|---|
| C1 | `World.removeNpc` early-returns when `!npc.isActive` (idempotence); the `npc.turn()` catch path passes duration **`0`, not `-1`** | `World.ts:669,1308` |
| C2 | `Entity.setLifeCycle` clamps to `max(1, tick)` unless `tick === -1` | `Entity.ts:36` |
| C3 | `EntityList`: `get`/`remove` bounds-check and return early; `set` throws on out-of-range **or already-occupied** id | `EntityList.ts:53,60,78` |
| C4 | `Npc` reset/respawn additionally clears `activeScript`, `delayed`, `delayedUntil` (both the `resetEntity` path and the respawn path) | `Npc.ts:195,300` |
| C5 | Login-bucket keying: IPv4 forced unsigned via `>>> 0`; IPv6 rewritten from "3rd hextet mod 256" to a real left-packed 128-bit key with `::`-expansion and `%zone` stripping | `World.ts:909` |
| C6 | `Zone.updatedThisTick(obj, player)` replaces the `obj.lastLifecycleTick === currentTick` test when deciding whether to skip an obj during a full zone refresh — now inspects queued `ObjDel`/`ObjAdd`/`ObjReveal` events with receiver matching | `Zone.ts:121,162` |
| C7 | `HashTable`: `findnext` deleted; `all()` iterates against the bucket **sentinel** rather than a `key !== 0n` test | `HashTable.ts:30,48` |

C6 is a real bug fix with observable effect: objects added and removed in the
same tick were being skipped on zone refresh by a test that only looked at the
lifecycle tick.

C5 changes which players share a login-throttle bucket. goscape's equivalent
lives in the login/world loop; it is behavioural but not wire-visible.

### D — Script engine

| # | Change | TS |
|---|---|---|
| D1 | Four new opcodes, each needing enum slot + `ScriptOpcodeMap` name + handler: `P_TEMPRUN`, `P_TRANSMOGRIFY`, `DATE_MINUTES`, `DATE_RUNEDAY` | `ScriptOpcode.ts:156,160,430` |
| D2 | **Transmogrification.** New `Player.npcId = -1`. In the appearance block, when `npcId != -1` the 12-slot equipment loop writes `p2(-1); p2(npcId)` and breaks. Removes the long-standing `// todo: transmog support` comment. **Wire-visible.** | `Player.ts:412,1390` |
| D3 | `ScriptOpcodePointers`: `FINDHERO` gains `require`/`require2` `active_player`; `P_TEMPRUN` and `P_TRANSMOGRIFY` require `p_active_player`; `PLAYERMEMBER`/`STAT_TOTAL`/`SESSION_LOG`/`WEALTH_EVENT` gain `require: active_player`; `OBJ_FIND` becomes `conditional: true` | `ScriptOpcodePointers.ts` |
| D4 | `checkedHandler` wrapper deleted along with `ScriptState.pointerCheck`, `pointerPrint`, `ScriptPointerNameMap`, and the `_LAST` enum member — the declarative pointer table is now the single enforcement point | `ScriptPointer.ts`, `ScriptState.ts:182`, all five `*Ops.ts` |
| D5 | **`map_findsquare` rewritten.** Old: two strategies (random-sampling ≤50 attempts when `maxRadius < 10`; a west-biased column scan otherwise), each triplicated per `MapFindSquareType`. New: one scan collecting up to `MAX_TILES = 100` eligible tiles, then a uniform roll; returns the input coord when none qualify. The west bias and the `isWithinDistanceSW` term are **gone**; filter order is now ring → f2p → blocked → reachability | `ServerOps.ts` |
| D6 | `Compiler.ts` bug fix: `commandInfo.corrupt2[opcode]` was assigning into `corrupt[opcode]` | `Compiler.ts:144` |

D4 is mechanical in TS (≈340 of the ~800 changed handler lines are pure
`checkedHandler(X, state => {…})` → `state => {…}` unwrapping across
InvOps/LocOps/NpcOps/PlayerOps/ServerOps). goscape's `pkg/script` uses a
different enforcement shape; the port is "make the goscape pointer table the
single source of truth", not a literal transcription. This bundle must confirm
goscape does not lose a check that the TS table does not restore.

D5 changes observable script results for every `map_findsquare` call. It is the
single largest content-visible behaviour change in the sync.

### E — Pack pipeline

Byte-output-affecting. Gated end-to-end by `smoke-pack --reference-dir`.

| # | Change | TS |
|---|---|---|
| E1 | `PixPack` rewritten: crop bounds auto-derived by scanning for non-magenta (`0xff00ff`) pixels instead of read from `.opt`; pixel order chosen by **transition count** (`row !== previousRow`) instead of a summed-delta score, with the return polarity flipped; `writeImage` loses its `Sprite`/meta parameter; `convertImage` gains optional `source`/`palette` injection; the `.opt` sidecar now carries **only** `WxH` tiling and is validated (`img.w % tileX === 0`) | `PixPack.ts` |
| E2 | `Pix.ts` (unpack side): PNG written only when it differs from what is on disk; `.opt` written only for multi-tile sheets and **deleted** otherwise | `Pix.ts:50` |
| E3 | Jag member ordering changed: `textures` writes `index.dat` **last**; `title` reorders to p11/p12/b12/q8/logo/title/titlebox/titlebutton/runes/index | `sprite/textures.ts`, `sprite/title.ts` |
| E4 | `versionlist` `anim_index` now emits real frame-base values (parse each `.anim`, `frameBase[frameId] = animsetId + 1`) instead of a hardcoded `0` | `versionlist/pack.ts:98` |
| E5 | `LocConfig` sorts models so `LocShapeSuffix._8` sorts first, others by shape | `LocConfig.ts:350` |
| E6 | `loadConstants()` extracted from `packConfigs`; `rebuildModelFlags` now calls it plus `ParamType.load('data/pack')` before crawling scripts | `PackShared.ts:320`, `ModelFlags.ts:117` |

E1 is the highest-risk item: it silently changes every sprite's crop offsets,
declared width/height and pixel order, which flows into the `title`, `textures`
and `media` archive CRCs pinned in bundle F.

### F — Build-verify CRC gates

Upstream moved every `checkcrc` from "over the in-memory `Packet` before
`jag.save`" to "over the **file bytes** read back before `cache.write`", and
added four archives that had no gate. New constants:

| Archive | Slot | CRC |
|---|---|---|
| `title` | `(0,1)` | `410306098` |
| `interface` | `(0,3)` | `2135735991` (was `2041671134`, checked pre-save) |
| `textures` | `(0,6)` | `915347346` |
| `wordenc` | `(0,7)` | `1386621111` |
| `sounds` | `(0,8)` | `-759577225` (was `2127412105`, checked pre-save) |

`-759577225` is a signed int32; goscape must compare in the same signedness or
the gate is vacuous. `memory:test_passes_for_wrong_reason` applies.

These constants are **empirically confirmed**: the reference rebuild at
Engine-TS `1d25566c` + Content `2b62ae68d` ran with `BUILD_VERIFY=true` and all
five gates passed.

### G — Networking

| # | Change | TS |
|---|---|---|
| G1 | `OnDemand.onClientClosed(client)` — on socket close, drop the client from the OnDemand registry and post `client_closed` to the worker; worker `exit` also clears the whole registry | `OnDemand.ts:87,128`, `TcpServer.ts:45` |
| G2 | `TcpClientSocket.close()` / `WSClientSocket.close()` drop the 1000 ms `setTimeout` before ending the socket | `TcpClientSocket.ts:20`, `WSClientSocket.ts` |
| G3 | JS-client archive HTTP routes are now **CRC-gated**: `/title:crc` … `/sounds:crc` parse the trailing crc and 404 unless it equals `CrcTable[n]`. Previously any `/title*` prefix served the archive unconditionally | `web.ts:164-250` |

G2 collides with shipped goscape work. `memory:sec1_hardening_2026_08_22`
records SEC1-D3: goscape moved socket writes behind a bounded outbound queue
with a drain-on-close path (`modules/world/…` outbound writer, 250 ms fallback
drain on zero write timeout). Upstream deleting its 1000 ms grace does **not**
mean goscape should delete its drain — TS's `setTimeout` was a crude stand-in
for exactly the flush guarantee goscape implements properly. Expected outcome:
**`SYNC289-D1`** — goscape keeps the bounded drain, documented as a
deliberate divergence, because adopting the TS shape would reintroduce the
truncated-final-packet bug SEC1-D3 fixed.

G3 is a genuine goscape-side change: `modules/ondemand/handler.go` currently
matches on prefix (`archiveRoutes` table, checked in order) with no CRC
validation.

### H — Data + pins

| # | Change |
|---|---|
| H1 | `data/raw/wordenc` blob refresh, 13,310 → 13,463 bytes. `data/raw/README.md` provenance line currently reads `Engine-TS@9aadcec4` and must be re-stamped. The runtime filter loader (`pkg/wordenc/encfilter`) reads this file at startup, so the new blob changes live chat filtering |
| H2 | `@lostcityrs/runescript` `0.9.6` → `0.9.7`. goscape's compiler is a hand-port pinned to 0.9.6 semantics with proven byte-parity (`docs/PORTING.md:1416-1505`, four REVERSING pins). Any 0.9.7 codegen delta must be identified and either ported or recorded |
| H3 | `main:REFERENCES.md` §rev-274 re-pin: Engine-TS `1d25566c…`, Content `2b62ae68d…`; add note 6 superseding note 5's work list |
| H4 | Fast-forward `274-GOSCAPE` → `274` in the Engine-TS checkout, and `274-GOSCAPE` → `274` in Content (also 4 commits stale by the same mechanism) |

H2 is the one item that could invalidate the whole `RunServerCompiler` parity
stage. It is sized as an investigation, not a known port.

## 5. `tools/unpack` deletion — resolved as an exception

Upstream deleted `tools/unpack/**` (30 files) and `tools/render/sound/**`
(3 files), ~4,900 lines.

**Decision: goscape keeps `pkg/unpack` + `goscape-cli unpack`.** Recorded as a
`PORTING-EXCEPTION`, following the precedent already set by
`symbols-export-go-only` (`docs/PORTING.md:32`), where goscape retains tooling
upstream removed.

Rationale: the deletion is repo housekeeping with zero server-fidelity impact —
no runtime code path, no packed byte, no wire message depends on it. goscape's
counterpart is a shipped capability (16 unpack families, a documented CLI
subcommand, and a decoder-conformance fixture corpus at
`Server274-ref/unpack-ref/`). Deleting it would remove working functionality and
its test corpus to track a change that means nothing on the Go side.

Consequence: `pkg/unpack` no longer has an upstream counterpart to track. From
this pin forward it is goscape-owned code, and future syncs must not treat its
divergence from a (now absent) TS reference as drift. Item E2 (`Pix.ts` unpack
side) is the last upstream unpack change goscape will ever port.

## 6. Reference cache — already rebuilt

Done during scoping, so the plan's pack tasks have a target to diff against:

- `Server274-ref/engine` → `1d25566c`, `Server274-ref/content` → `2b62ae68d`
  (both clean git worktrees, were exactly at the recorded pins).
- `npm install` + `npm run clean` + `npm run build` → `pack: 9.633s`, exit 0,
  `BUILD_VERIFY=true`.
- Three `missing model` warnings, all pre-existing and renamed by the
  already-ported `3b653372` shape-packing change
  (`woman_legs_model_434`, `npc_mummy`, `skill_slayer_wall_cave_2`).

Baseline goscape parity against that fresh reference:

```
$ goscape-cli smoke-pack --content-dir ../content --reference-dir data/pack
PackConfigs  ERR  invalid property key in ../content/scripts/_unpack/225/all.seq: reachforward=yes
… 13 stages SKIP
Result: 0 OK, 1 ERR, 13 SKIP
```

Stage 1 dies on A4. Every later stage is unreachable until bundle A lands —
which is why A is not merely first by preference but by necessity.

`Server274-ref/unpack-ref/cache` stays **un-re-pinned**: per
`pkg/unpack/unpacktest/harness.go:80-107` and the 2026-07-16 plan it is a fixed
decoder-conformance fixture, deliberately decoupled from cache provenance.
`memory:rev274_pin_update_2026_07_16` also records the `runfam.sh`
`mv`-into-existing-directory nesting trap for anyone regenerating it anyway.

## 7. Deviation register

| ID | Item | Expected disposition |
|---|---|---|
| `SYNC289-D1` | G2 socket-close grace | goscape keeps the SEC1-D3 bounded drain; upstream's `setTimeout` deletion not adopted |
| `SYNC289-D2` | `tools/unpack` retention | `PORTING-EXCEPTION (unpack-tools-go-only)`, §5 |
| `SYNC289-D3` | reserved — `web.ts` framework | goscape keeps `pkg/dskit/server`; only G3's CRC gate ported |

Any further divergence discovered during implementation gets a new
`SYNC289-D{n}` row per `memory:true_to_ts_gate`. A divergence without a row is a
critical review blocker.

## 8. Verification

- **Per bundle:** TDD (RED→GREEN) per `memory:runescript_cadence`; touched
  packages run with `-race` (`memory:race_detector_now_available`, needs
  `CGO_ENABLED=1`).
- **Compile-all gate:** `go test -run '^$' ./...`
  (`memory:cross_rev_port_methodology`).
- **Pack parity:** `smoke-pack --reference-dir` must reach **14 OK, 0 ERR** with
  zero byte diffs.
- **Reference-backed suites:** `GOSCAPE_REF274_DIR=…/Server274-ref/engine go
  test ./...`. `TestNAI128_RatLootCascade` fails whenever that variable is set —
  **pre-existing** at the `dee467c8` baseline and unchanged since
  (`memory:rev274_pin_update_2026_07_16`); it is not a regression signal for
  this work.
- **Full suite:** `go test ./...` green before the re-pin commit.
- **Smoke:** the wire-visible changes (D2 transmog appearance, G3 CRC-gated
  archive routes) warrant a user-launched Java-client smoke per
  `memory:smoke_test_server_handoff`. Server must be user-launched, not
  agent-launched.

## 9. Cross-branch policy

`memory:no_forward_port_deviations` — do **not** forward-port any of this to
`rev-254` / `rev-245.2` / `rev-244` / `rev-225`. These are upstream-274-branch
commits; the earlier branches' upstreams have not moved. Their pins stay put.

## 10. Open risks

1. **E1 `PixPack`** — a sprite-packing rewrite whose output feeds three CRC-gated
   archives. Highest chance of a long byte-diff loop.
2. **H2 compiler 0.9.7** — unknown codegen delta against four documented
   REVERSING pins. Could invalidate `RunServerCompiler` parity.
3. ~~**B route-blocker deletion**~~ — **retired during scoping.** Proven
   unreachable on the goscape side (§B); the deletion is safe.
4. **D5 `map_findsquare`** — content-visible behaviour change with no byte gate
   to catch a mistake; needs its own tests.
5. **A2 `maxrange` default** — silently changes hunt geometry for every NPC that
   omits `maxrange`.
