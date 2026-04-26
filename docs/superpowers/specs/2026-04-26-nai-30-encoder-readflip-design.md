# NAI-30 — encoder loops port + read-flip

- **Sub-spec**: NAI-30
- **Date**: 2026-04-26
- **Scope label**: B (Second sub-spec of the four-sub-spec series aligning goscape's `pkg/rsbuf` to the upstream `@2004scape/rsbuf` Rust crate's stateful API; replaces the existing interface-based `Encode`/`EncodeNpc` package functions with `*Buf`-attached `PlayerInfo`/`NpcInfo` structs that read state directly from the NAI-29-landed `*Buf` slot tables; reconciles the two NAI-29 parallel-write divergences flagged inline in `tick.go`; ports the NPC-side spatial-discovery primitives from upstream; retires `pkg/grid/`, `pkg/buildarea/`, `pkg/rsbuf/source.go`, `pkg/rsbuf/npc_source.go`, `pkg/rsbuf/npc_observers.go`, plus `Player.AppearanceHash` + helper. Touches `pkg/rsbuf/` (rewrites `playerinfo.go` + `npcinfo.go`; extends `buildarea.go`, `player.go`, `npc.go`, `buf.go`) + `modules/world/` (extends `Player` + `Npc` with orientation + `lastAppearance` fields; flattens `pkg/buildarea` scenery fields onto `Player`; swaps encoder callers in `player_info.go` + `player_npc_info.go`; deletes the placeholder comments at `tick.go:373,376,412`); ~700 LOC production + ~700 LOC tests across 4 bundles; introduces 1 new deviation tag (NAI-30-D1: orientation field plumbed without producer); net deviation count 13 → 14)
- **Predecessors**: NAI-29 (rsbuf stateful core + entity model + parallel-write caller hooks) — last on `main` as `af8229d`
- **Source root**: `2004scape/rsbuf` branch `225` at `/home/owner/Code/github.com/2004scape/rsbuf/` (HEAD `1cbb2ce`)

## Motivation

NAI-29 landed the `*Buf` stateful core: per-tick `ComputePlayer`/`ComputeNpc` push player + NPC state into rsbuf-owned slot tables, `AddPlayer`/`AddNpc`/`RemovePlayer`/`RemoveNpc` maintain the lifecycle, and `Cleanup` runs end-of-tick. The state is **populated** but **never read** — the existing `Encode`/`EncodeNpc` package functions still pull from caller-supplied `PlayerSource`/`NpcSource` interfaces and `*grid.Grid`/`*buildarea.BuildArea` parameters. NAI-30 closes this loop by porting upstream `info.rs` (708 LOC) into `*Buf`-attached `PlayerInfo`/`NpcInfo` structs that read directly from the slot tables, and retiring the parallel-write infrastructure that no longer has a consumer.

The rewrite also reconciles two divergences flagged inline at NAI-29's parallel-write hooks:

1. `tick.go:373` passes `int32(p.AppearanceHash() & 0x7fffffff)` for `lastAppearance`. Upstream uses a tick counter set when `generateAppearance()` runs; the content-hash semantics differ in the "out-of-view → re-add" case and in the "equipment-change cycle (hat on/off/on)" case. NAI-30 ports to tick-when-changed semantics, restoring upstream's `last_appearance == -1` "never generated" guard at `info.rs:305`.
2. `tick.go:376,412` passes `int32(0), int32(0)` for `orientationX/Z`. Upstream initializes `-1` as "no value" sentinel and falls back to player coord at `info.rs:328-340`; `0` is a valid orientation tile and would silently corrupt face-coord. NAI-30 adds real `OrientationX/Z int` fields on goscape `Player` + `Npc`, default `-1`, with no producer wired (deviation NAI-30-D1; producer comes with the engine-port series).

The brainstorm-time recon also surfaced that the NAI-29 close memory's framing of `pkg/buildarea` retirement as "subsumed by `pkg/rsbuf.BuildArea`" was incorrect: goscape's `pkg/buildarea` carries *both* encoder state (`Players`, `Npcs`, `Appearance`, `HasAppearance`, `RecordAppearance` — already in `pkg/rsbuf.BuildArea` from NAI-29) *and* scenery-window state (`OriginX`, `OriginZ`, `LastBuild`, `LoadedZones`, `ActiveZones`, `Mapsquares`, `ShouldRebuild`, `Rebuild`) that has no rsbuf counterpart and is consumed by ~25 sites in `modules/world` (`data_map.go`, `player.go:436-476` LOC rebuild loop, `client.go`, `login_map_test.go`, `data_map_test.go`, `player_zone_test.go`, `player_npc_test.go`, `player_info_test.go`, `npc_event_queue_test.go`). The decision (Q1, approved): flatten the scenery-window fields onto `Player` directly, matching the structure of `class Player` in LostCityRS/Engine-TS where `buildArea` is a private field rather than a standalone struct.

The third reframing: the followup memory mentioned only `getNearbyNpcs + filterNpc` for spatial discovery, but `pkg/grid.NearbyPlayers` is also a current consumer of pkg/grid (in the existing encoder's `writeNewPlayers` path). To retire pkg/grid wholesale, the player-side spatial discovery also needs a port — specifically the `get_nearby_players_zones` zone-walk fast path (build.rs:178-213), which is the only path used while `view_distance == PREFERRED` (always true in NAI-30; the spiral fallback `get_nearby_players_nearest` and the dispatcher `get_nearby_players` land in NAI-32 alongside the resize logic).

## Tech stack

- Go 1.26+
- Existing packages **read** from at brainstorm time:
  - `pkg/rsbuf/buf.go` (`*Buf` instance + `players [2048]*Player` + `npcs [8192]*Npc` + `zoneMap *zoneMap` + `playerGrid map[uint32][]int32` from NAI-29)
  - `pkg/rsbuf/buildarea.go` (`*BuildArea` with `Players`/`Npcs` `*idBitSet` + `appearances [2048]uint32` from NAI-29; gains 4 spatial-discovery methods in NAI-30 B1)
  - `pkg/rsbuf/player.go` (`Player` struct with `Coord`, `Origin`, `LastAppearance`, `OrientationX/Z`, etc. — already in NAI-29; encoder reads in NAI-30 B2)
  - `pkg/rsbuf/npc.go` (`Npc` struct with `Coord`, `OrientationX/Z`, `Observers`, etc. — already in NAI-29; encoder reads in NAI-30 B3)
  - `pkg/rsbuf/zonemap.go` (rsbuf-internal `zoneMap` with `Zone(x, level, z)` accessor + `Zone.players []int32` + `Zone.npcs []int32` from NAI-29)
  - `pkg/rsbuf/idbitset.go` (`*idBitSet` with `Contains/Insert/Remove/Iter/Len/Clear` from NAI-29)
  - `pkg/rsbuf/visibility.go` (`Visibility` enum, used by `filterPlayer` and PlayerInfo encoder)
  - `pkg/rsbuf/renderer.go` (existing goscape `*Renderer` — passed through into the new `Encode` methods; internal swap is NAI-31 scope)
  - `pkg/rsbuf/mask_payload.go`, `pkg/rsbuf/npc_mask_payload.go` (existing mask encoders — reused as-is by the new `PlayerInfo` + `NpcInfo` blocks; NAI-31 closes parity gaps)
  - `pkg/io/packet/packet.go` (`Packet` with `AccessBits`, `AccessBytes`, `PBit`, `P1`, etc.; reused for the encoder scratch buffers)
  - `pkg/coordgrid/coordgrid.go` (`PackCoord`, `UnpackCoord`, `Position` — already used by NAI-29 `*Buf` methods)
  - LostCityRS/Engine-TS `Player.buildArea` field structure (used as analogue for the scenery-window flatten onto `Player`; not a behavioral source)
- Modified files in `pkg/rsbuf/`:
  - `buildarea.go` — add `GetNearbyPlayers` (zone-walk variant, fixed view distance), `GetNearbyNpcs`, `filterPlayer`, `filterNpc` methods; B1
  - `playerinfo.go` — full rewrite: `PlayerInfo` struct with `buf *packet.Packet, updates *packet.Packet` scratch fields + `(pi *PlayerInfo) Encode(b *Buf, pid int32, renderer *Renderer) []byte` method; B2
  - `npcinfo.go` — full rewrite: `NpcInfo` struct + `(ni *NpcInfo) Encode(b *Buf, pid int32, renderer *Renderer) []byte` method; B3
  - `buf.go` — `*Buf` gains `PlayerInfo *PlayerInfo, NpcInfo *NpcInfo` fields; `New()` initializes both; B4
- Modified files in `modules/world/`:
  - `player.go` — add `lastAppearance int` field (default `-1`); add `OrientationX, OrientationZ int` fields (default `-1`); flatten scenery-window fields from `*buildarea.BuildArea` onto `Player` (`lastBuild int`, `loadedZones map[int]bool`, `activeZones map[int]bool`, `mapsquares map[uint16]bool`); add `(p *Player) shouldRebuild()` and `(p *Player) rebuildScenery(currentTick int)` methods; remove `buildArea *buildarea.BuildArea` field; B1 + B4
  - `npc.go` — add `OrientationX, OrientationZ int` fields (default `-1`); B1
  - `appearance.go` — `generateAppearance` sets `p.lastAppearance = currentTick` after writing `p.appearanceBuf`; B1
  - `tick.go` — swap `int32(p.AppearanceHash()&0x7fffffff)` → `int32(p.lastAppearance)`; swap two `int32(0), int32(0)` pairs → `int32(p.OrientationX), int32(p.OrientationZ)` (player) and `int32(n.OrientationX), int32(n.OrientationZ)` (npc); remove `s.grid.Add/Remove/AddNpc/RemoveNpc` calls (B4); remove `rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)` if redundant after `s.rsbuf.RemovePlayer` from NAI-29 (verify via grep)
  - `player_info.go` — swap `rsbuf.Encode(p, sources, p.buildArea, s.grid, s.renderer)` → `s.rsbuf.PlayerInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)`; B4
  - `player_npc_info.go` — swap `rsbuf.EncodeNpc(p, sources, p.buildArea, s.grid, s.renderer)` → `s.rsbuf.NpcInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)`; B4
  - `data_map.go` — `p.buildArea.LastBuild` → `p.lastBuild`; `p.buildArea.Mapsquares` → `p.mapsquares`; B4
  - `player.go` — `p.buildArea.ShouldRebuild(...)` → `p.shouldRebuild()`; `p.buildArea.Rebuild(...)` → `p.rebuildScenery(currentTick)`; `p.buildArea.LoadedZones`/`ActiveZones` → `p.loadedZones`/`activeZones`; B4
  - `client.go` — line `:59` doc-comment touch-up referencing `buildArea.ShouldRebuild`; B4
  - `server.go` — `s.grid = grid.New()` line removed (B4); `s.grid.AddNpc` at NPC-bootstrap removed (B4)
  - `player_source.go` — delete the file or trim it to only the methods that remain consumed (re-grep `(p *Player).Visibility()`, etc.); B4
  - `npc_source.go` — same pattern as `player_source.go`; B4
- New files in `modules/world/` — none (all changes are extensions of existing files)
- Modified test files in `modules/world/` (mechanical churn for buildarea flatten; B4):
  - `login_map_test.go` — `p.buildArea.OriginX/OriginZ` → `p.originX/originZ`
  - `data_map_test.go` — `p.buildArea.LastBuild` → `p.lastBuild`; `p.buildArea.Mapsquares[...]` → `p.mapsquares[...]`
  - `player_zone_test.go` — `p.buildArea.LoadedZones[idx]` → `p.loadedZones[idx]`
  - `player_npc_test.go` — `p.buildArea.Npcs[npc.nid]` → `s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid))` (cross-package read; encoder state lives on `*Buf` now)
  - `player_info_test.go` — `a.buildArea.Players[2]` → `s.rsbuf.HasPlayer(int32(a.slot), 2)`
  - `npc_event_queue_test.go` — `p.buildArea.Npcs[101] = struct{}{}` → use `*Buf`-side fixture or test-helper that subscribes via the new path
- Deleted files:
  - `pkg/grid/grid.go`, `pkg/grid/grid_test.go`, `pkg/grid/` directory; B4 T4.6
  - `pkg/buildarea/buildarea.go`, `pkg/buildarea/buildarea_test.go`, `pkg/buildarea/` directory; B4 T4.5 (after flatten)
  - `pkg/rsbuf/source.go`; B4 T4.6
  - `pkg/rsbuf/npc_source.go`; B4 T4.6
  - `pkg/rsbuf/npc_observers.go` + `pkg/rsbuf/npc_observers_test.go`; B4 T4.6 (after T4.4 consumer migration)
  - `appearance.go:appearanceHash` helper + `player_source.go:AppearanceHash` method (if package-level `appearanceHash` becomes dead after `AppearanceHash()` retires); B4 T4.6
- New test files in `pkg/rsbuf/`:
  - `playerinfo_test.go` — port + extend existing branch-level pins (B2)
  - `npcinfo_test.go` — port + extend existing branch-level pins (B3)
  - existing `pkg/rsbuf/playerinfo_test.go` + `npcinfo_test.go` are *replaced* in-place; the existing tests already pin the byte-level wire branches and serve as the regression net during the rewrite
- Memory files:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — at NAI-30 close, append a "From NAI-30 (2026-04-XX)" close entry mirroring the NAI-29 close entry pattern; update the "Deferred: pkg/rsbuf upstream alignment series" tracker entry to reflect NAI-30 closed and the corrected scope notes (pkg/buildarea was NOT subsumed by rsbuf BuildArea — flattened onto Player); add NAI-30-D1 to a deviation-tag reference if one exists; add `Closes memory:` trailer per `close_commit_memory_trailer` memory

## Scope

### Bundle 1 — BuildArea spatial discovery + entity field plumbing

**Goal**: Land the rsbuf-internal spatial discovery primitives that the new encoder will use; add real producer fields for `lastAppearance` (tick-when-changed) and `OrientationX/Z` on goscape `Player` + `Npc`, replacing the inline-divergence-flagged placeholders in `tick.go`.

No caller-side encoder changes; the old encoder still functions with the new field values (orientation `-1` is just the new sentinel, lastAppearance tick is just a different `int32` value).

**Source mappings**:

| Goscape symbol | Upstream Rust source |
|---|---|
| `(b *BuildArea) GetNearbyPlayers(players *[2048]*Player, zoneMap *zoneMap, pid int32, x, level, z int) []int32` | `BuildArea::get_nearby_players_zones` at `build.rs:178-213` (zone-walk variant; the dispatcher `get_nearby_players` at `build.rs:160-176` lands at NAI-32 alongside spiral fallback) |
| `(b *BuildArea) GetNearbyNpcs(npcs *[8192]*Npc, zoneMap *zoneMap, x, level, z int) []int32` | `BuildArea::get_nearby_npcs` at `build.rs:262-296` |
| `(b *BuildArea) filterPlayer(players *[2048]*Player, player int32, pid int32, x, level, z int) bool` | `BuildArea::filter_player` at `build.rs:298-312` |
| `(b *BuildArea) filterNpc(npcs *[8192]*Npc, npc int32, x, level, z int) bool` | `BuildArea::filter_npc` at `build.rs:314-327` |
| `Player.OrientationX/Z int` (modules/world) | upstream `Player.orientation_x/z: i32` at `player.rs:23-24` (initialized `-1`; persistent across cleanup per `player.rs:107-108`) |
| `Npc.OrientationX/Z int` (modules/world) | upstream `Npc.orientation_x/z: i32` at `npc.rs:16-17` (initialized `-1`; persistent across cleanup per `npc.rs:71-72`) |
| `Player.lastAppearance int` (modules/world) | upstream `Player.last_appearance: i32` at `player.rs:19` (initialized `-1`; set to `currentTick` when `generateAppearance` runs, equivalent to TS `Player.lastAppearance = World.currentTick` pattern) |

**Tasks**:

- **T1.1** — `(b *BuildArea) GetNearbyPlayers` zone-walk variant. Iterates the rectangular zone window `[startZX..endZX] × [startZZ..endZZ]` with `startZX = (x - viewDistance) >> 3` etc., calls `zoneMap.Zone(zoneX, level, zoneZ).players` for each cell, filters via `filterPlayer`, caps at `preferredPlayers - len(b.Players)` to bound output size. Note: upstream uses `view_distance` as a parameter; in NAI-30 this is the const `preferredViewDistance` (15). Doc-comment notes that NAI-32 will introduce the dispatcher that selects between this and the spiral path based on dynamic `view_distance`.
- **T1.2** — `(b *BuildArea) GetNearbyNpcs`. Same shape as T1.1 but for npcs; uses `BuildArea::PREFERRED_VIEW_DISTANCE = 15` always (upstream hardcodes it — NPCs don't shrink view distance even in upstream).
- **T1.3** — `(b *BuildArea) filterPlayer / filterNpc`. Pure predicates. Five reject conditions each (per upstream `build.rs:298-327`): `players.Contains(pid)` (already tracked), `!CoordGrid::within_distance_sw` (out of range), `pid == -1` (slot empty marker), `pid == self.pid` (self), `coord.y() != y` (different level). NPC variant swaps `npcs.Contains` for tracked check, drops the self check, adds `!active` check.
- **T1.4** — Add `OrientationX, OrientationZ int` (default `-1`) to `modules/world/Player` (in `player.go:newPlayer`) and `modules/world/Npc`. Wire into `tick.go:376,412` arg positions: replace `int32(0), int32(0)` → `int32(p.OrientationX), int32(p.OrientationZ)` and `int32(n.OrientationX), int32(n.OrientationZ)`. No producer in NAI-30 — fields stay at `-1` always; encoder fallback (NAI-30 B2/B3 ports `info.rs:328-340`) handles `-1` correctly. Add inline doc-comment on each field referencing **NAI-30-D1**.
- **T1.5** — Add `lastAppearance int` (default `-1`) to `modules/world/Player`. In `appearance.go:generateAppearance`, set `p.lastAppearance = currentTick` after the buffer is written. Wire into `tick.go:373` arg position: replace `int32(p.AppearanceHash()&0x7fffffff)` → `int32(p.lastAppearance)`. Note: `currentTick` is already a parameter of `generateAppearance` (per `appearance.go:22`).

**Tests** (B1 — all in `pkg/rsbuf/buildarea_test.go` extension + `modules/world/{appearance_test.go, player_test.go, npc_test.go}`):

- `TestBuildArea_GetNearbyPlayers_ZoneWalkBounds` — pin window math (start/end zone bounds with `(x - 15) >> 3`); coords near origin, near max coord; zero-length output for empty zoneMap.
- `TestBuildArea_GetNearbyPlayers_FilterEachRejectBranch` — 5 separate test cases, one per filter-reject branch (already tracked, out of range, slot empty, self, different level). Each pins that specific reject as the cause of exclusion.
- `TestBuildArea_GetNearbyPlayers_RespectsPreferredCap` — fill enough zones to overflow `preferredPlayers - len(b.Players)`; verify cap holds.
- `TestBuildArea_GetNearbyNpcs_ZoneWalkBounds` — analogous to T1.1 player counterpart.
- `TestBuildArea_GetNearbyNpcs_FilterEachRejectBranch` — 5 separate test cases per `filter_npc` branch.
- `TestBuildArea_GetNearbyNpcs_RespectsPreferredCap` — analogous.
- `TestNewPlayer_OrientationXZ_DefaultMinusOne` (modules/world) — pin `-1` defaults.
- `TestNewNpc_OrientationXZ_DefaultMinusOne` (modules/world) — pin `-1` defaults.
- `TestNewPlayer_LastAppearance_DefaultMinusOne` — pin `-1` default.
- `TestGenerateAppearance_SetsLastAppearanceToCurrentTick` — call `generateAppearance(objs, invs, 42)`; assert `p.lastAppearance == 42`.
- `TestTickComputePlayer_PassesRealOrientationFields` — set `p.OrientationX = 100, p.OrientationZ = 200`; run `processInfo`; assert `b.players[pid].OrientationX == 100, OrientationZ == 200`.
- `TestTickComputePlayer_PassesRealLastAppearance` — set `p.lastAppearance = 42`; run `processInfo`; assert `b.players[pid].LastAppearance == 42`.

### Bundle 2 — PlayerInfo encoder port

**Goal**: Rewrite `pkg/rsbuf/playerinfo.go` from interface-based package function to `*Buf`-attached `PlayerInfo` struct with reusable scratch buffers, mirroring upstream `info.rs:13-409` line-by-line. Old encoder still present (callers swap in B4); during this bundle the file holds both old `Encode(...)` (renamed to `EncodeLegacy`) and new `(pi *PlayerInfo).Encode(...)`.

**Source mappings**:

| Goscape symbol | Upstream Rust source |
|---|---|
| `type PlayerInfo struct { buf, updates *packet.Packet }` | `pub struct PlayerInfo { buf: Packet, updates: Packet }` at `info.rs:13-16` |
| `func NewPlayerInfo() *PlayerInfo` | `PlayerInfo::new` at `info.rs:24-30` |
| `(pi *PlayerInfo) Encode(b *Buf, pid int32, renderer *Renderer) []byte` | `PlayerInfo::encode` at `info.rs:32-70` |
| `(pi *PlayerInfo) writeLocalPlayer(...)` | `PlayerInfo::write_local_player` at `info.rs:72-100` |
| `(pi *PlayerInfo) writePlayers(...)` | `PlayerInfo::write_players` at `info.rs:102-134` |
| `(pi *PlayerInfo) writeNewPlayers(...)` | `PlayerInfo::write_new_players` at `info.rs:136-166` |
| `(pi *PlayerInfo) add/remove/teleport/run/walk/extend/idle` | `PlayerInfo::{add, remove, teleport, run, walk, extend, idle}` at `info.rs:168-280` |
| `(pi *PlayerInfo) highdefinition/lowdefinition/writeBlocks` | `PlayerInfo::{highdefinition, lowdefinition, write_blocks}` at `info.rs:282-402` |
| `(pi *PlayerInfo) fits` | `PlayerInfo::fits` at `info.rs:404-408` |
| `BITS_ADD = 23, BITS_RUN = 10, BITS_WALK = 7, BITS_EXTEND = 3` constants | `PlayerInfo::BITS_*` at `info.rs:19-22` |
| `MAX_PACKET_BYTES = 4997` | `4997` literal at `info.rs:407` |

**Encode signature divergences from upstream** (each gets an inline divergence note in code):

- `pos` upstream param dropped (NAI-30 hard-codes `0` — each `Encode` call starts a fresh OpPlayerInfo packet; goscape's `writeOut` adds the opcode + length wrapper after the encoder returns). Doc-comment: *"upstream allows multi-packet packing; goscape always starts at pos=0 since each player's info is wrapped as a standalone OpPlayerInfo by the caller."*
- `dx, dz, rebuild` upstream params dropped (NAI-30 doesn't run `rebuild_players` — view-distance resize is NAI-32 scope; rebuild detection lands then). Inline doc on the omission references NAI-32. The encoder skips the `if rebuild || dx > vd || dz > vd { build.rebuild_players(...) } else { build.resize() }` block at `info.rs:47-51` entirely.
- `players: &[Option<Player>]` and `grid: &HashMap<u32, Vec<i32>>` and `map: &mut ZoneMap` collapse into `b *Buf` (read all three from `b.players`, `b.playerGrid`, `b.zoneMap`).
- `player: &mut Player` collapses into `pid` + `b.players[pid]`.

**Tasks**:

- **T2.1** — Rename existing `func Encode(...)` to `func EncodeLegacy(...)` in-place. Update no callers (callers still call the old name — but the rename is preparation for B4 cleanup). Skip if grep shows the rename would touch many test names and shipping a co-existing dual API is cleaner — in that case, put the new code in `playerinfo_v2.go` and rename in B4 instead. Plan-author decides at writing-plans time based on existing test file structure.
- **T2.2** — Define `type PlayerInfo struct { buf, updates *packet.Packet }`, `NewPlayerInfo() *PlayerInfo` (allocates fresh packets sized for typical player info — `packet.NewPacket(make([]byte, 0, 5000))` analogous to upstream `Packet::new(5000)`), and the `Encode(b *Buf, pid int32, renderer *Renderer) []byte` method skeleton with the prologue (reset `buf.Pos`, `buf.BitPos`, `updates.Pos`, `updates.BitPos`; `buf.AccessBits()`).
- **T2.3** — Implement `writeLocalPlayer` mirroring `info.rs:72-100`. Branch ladder: `tele` → teleport(); else `runDir != -1` → run(); else `walkDir != -1` → walk(); else `len > 0` → extend(); else idle(). `len` = `renderer.HighdefinitionsOf(pid)`.
- **T2.4** — Implement `writePlayers` mirroring `info.rs:102-134`. Iterates `b.players[pid].Build.Players.Iter()`. Per-tracked-player branch: 6 reject conditions match `info.rs:114` exactly (`other.pid == -1`, `other.tele`, level mismatch, out-of-distance, `!active`, `Visibility::HARD`). Surviving entries take run/walk/extend/idle path with `fits()` budgeting.
- **T2.5** — Implement `writeNewPlayers` mirroring `info.rs:136-166`. Calls `b.players[pid].Build.GetNearbyPlayers(b.players, b.zoneMap, pid, x, level, z)` (B1 method). Adds each via `add()` until `BITS_ADD` budget would overflow, then break. Visibility::HARD reject inline at `info.rs:154`.
- **T2.6** — Implement `add/remove/teleport/run/walk/extend/idle` mirroring `info.rs:168-280`. Each is 5-15 lines of `PBit` calls; pure structural port.
- **T2.7** — Implement `highdefinition/lowdefinition` mirroring `info.rs:282-346`. `lowdefinition` includes the `last_appearance != -1` guard (`info.rs:305`) and the orientation fallback ladder (`info.rs:328-340`). For NAI-30 the renderer methods (`renderer.has`, `renderer.cache`, `renderer.write`) call into the existing goscape `*Renderer` — the per-mask-slot caching will be NAI-31 scope; in NAI-30 we can either (a) call existing goscape renderer methods that perform full re-encode each tick, or (b) port a stub render-cache. Decision: (a) — minimum-delta to existing renderer; NAI-31 closes the gap.
- **T2.8** — Implement `writeBlocks` mirroring `info.rs:348-402`. Mask order at `info.rs:362-401` (APPEARANCE, ANIM, FACE_ENTITY, SAY, DAMAGE, FACE_COORD, CHAT, SPOT_ANIM, EXACT_MOVE). EXACT_MOVE math at `info.rs:386-399` uses `player.origin` (the local player's origin, not the other's) — verify the existing goscape mask_payload encoder for ExactMove takes the same args, or extend it.
- **T2.9** — Implement `fits` mirroring `info.rs:404-408`: `((buf.BitPos + bitsToAdd + 7) >> 3) + bytes + bytesToAdd <= MAX_PACKET_BYTES`.

**Tests** (B2 — `pkg/rsbuf/playerinfo_test.go` rewritten in-place; existing tests are the byte-level pin set, port their fixtures from `PlayerSource`-based to `*Buf`-based):

For each branch, decode the produced bytes via Java-client reader-order and pin per-field. Per `rsbuf_roundtrip_tests` memory; per `plan_runnable_test_fixtures` plan-author dry-runs each fixture before dispatch.

- `TestPlayerInfo_LocalPlayer_Idle` — local at coord, no movement, no masks → 1 zero bit, no updates payload, total 1 byte aligned to 1 byte.
- `TestPlayerInfo_LocalPlayer_Walk` — set `walkDir = 0`; pin walk header (1+2+3 bits = walk-leaf 6 bits), no extend.
- `TestPlayerInfo_LocalPlayer_Run` — set `runDir = 1, walkDir = 0`; pin run header (1+2+3+3 bits = 9 bits), no extend.
- `TestPlayerInfo_LocalPlayer_Tele` — set `tele = true`; pin teleport header (1+2+2+7+7+1+1 = 21 bits), local-coord math from `info.rs:84-86`.
- `TestPlayerInfo_LocalPlayer_ExtendOnly` — set masks!=0 with appearance pending; pin extend header, payload bytes match upstream.
- `TestPlayerInfo_OtherPlayers_RemoveBranches` — 6 sub-cases pinning each remove-trigger (slot empty, tele, level mismatch, out-of-distance, !active, HARD vis).
- `TestPlayerInfo_OtherPlayers_VisibilitySoftStaffMod` — pin: `Visibility::SOFT` + `selfStaffModLevel < 1` → remove; `>= 1` → keep.
- `TestPlayerInfo_OtherPlayers_RunWalkExtendIdle` — 4 branches per kept-player mode (run, walk, extend, idle).
- `TestPlayerInfo_NewPlayers_AddBranch` — set up nearby-discovery returning a slot; pin add-header (BITS_ADD = 23 bits), low-def payload follows.
- `TestPlayerInfo_NewPlayers_RespectsBudget` — overflow `MAX_PACKET_BYTES`; pin early-return with no further adds.
- `TestPlayerInfo_NewPlayers_PreferredCap` — overflow `PREFERRED_PLAYERS = 250`; pin cap.
- `TestPlayerInfo_LastAppearance_FreshGuardSkipsAppearance` — set `b.players[other].LastAppearance = -1`; pin no APPEARANCE block written.
- `TestPlayerInfo_LastAppearance_BuildSavesOnFirstSend` — set `LastAppearance = 42`, build appearances all 0; first encode sends APPEARANCE, build saves 42; second encode same tick skips APPEARANCE (build matches).
- `TestPlayerInfo_LastAppearance_BuildResendsOnTickChange` — first encode saves 42; bump `LastAppearance = 43`; second encode resends.
- `TestPlayerInfo_FaceCoordOrientationFallback` — 3 branches per `info.rs:321-340` (face_x set → use face_x; face_x=-1 + orientation_x set → use orientation; both -1 → use coord fine).
- `TestPlayerInfo_ExactMoveSubtractsLocalOrigin` — pin upstream math at `info.rs:386-399`.
- `TestPlayerInfo_OutputBytesAreCopy` — call `Encode` twice with identical state; verify the returned `[]byte` for the first call is unchanged after the second call mutates internal scratch (i.e., `Encode` returns a copy, not a view into `pi.buf.Data`).
- `TestPlayerInfo_LocalPlayer_ChatStripped` — pin `info.rs:289-291`: local-player highdef strips CHAT mask bit (avoids self-echo).

### Bundle 3 — NpcInfo encoder port

**Goal**: Same as B2 but for NpcInfo. Mirrors `info.rs:411-708`.

**Source mappings**:

| Goscape symbol | Upstream Rust source |
|---|---|
| `type NpcInfo struct { buf, updates *packet.Packet }` | `pub struct NpcInfo { buf: Packet, updates: Packet }` at `info.rs:411-414` |
| `(ni *NpcInfo) Encode(b *Buf, pid int32, renderer *Renderer) []byte` | `NpcInfo::encode` at `info.rs:430-464` |
| `(ni *NpcInfo) writeNpcs(...)` | `NpcInfo::write_npcs` at `info.rs:466-499` |
| `(ni *NpcInfo) writeNewNpcs(...)` | `NpcInfo::write_new_npcs` at `info.rs:501-529` |
| `(ni *NpcInfo) add/remove/run/walk/extend/idle` | `NpcInfo::{add, remove, run, walk, extend, idle}` at `info.rs:531-613` |
| `(ni *NpcInfo) highdefinition/lowdefinition/writeBlocks` | `NpcInfo::{highdefinition, lowdefinition, write_blocks}` at `info.rs:615-701` |
| `BITS_ADD = 35, BITS_RUN = 10, BITS_WALK = 7, BITS_EXTEND = 3` | `NpcInfo::BITS_*` at `info.rs:417-420` |
| `NPC_TERMINATOR = 8191` | `8191` literal at `info.rs:457` |

**Notes specific to NpcInfo**:

- `writeNpcs` decrements `b.npcs[nid].Observers` on remove (per `info.rs:480` `other.observers = (other.observers - 1).max(0)`), incremented on add (per `info.rs:525` `other.observers = other.observers + 1`). This consolidates the observer counter onto `*Buf.npcs[nid].Observers` (already present from NAI-29) — replaces the package-level `npcObservers` shim entirely.
- The 8191 NPC_TERMINATOR is written into `buf` if `writeNewNpcs` exits early due to byte budget overflow at `info.rs:457`; pin as a separate test branch.
- NPC-info has no exact-move and no chat mask handling.
- `writeNewNpcs` at `info.rs:510` calls `b.players[pid].Build.GetNearbyNpcs(b.npcs, b.zoneMap, x, level, z)` (B1 method).

**Tasks**: Mirror B2's task structure (T3.1 rename old to legacy, T3.2 struct + Encode skeleton, T3.3 writeNpcs, T3.4 writeNewNpcs, T3.5 add/remove/run/walk/extend/idle, T3.6 highdefinition/lowdefinition/writeBlocks, T3.7 fits).

**Tests** (B3 — `pkg/rsbuf/npcinfo_test.go` rewritten in-place):

- `TestNpcInfo_TrackedNpc_RemoveBranches` — 5 sub-cases pinning each remove-trigger (slot empty, tele, level mismatch, out-of-distance, !active). Each verifies `b.npcs[nid].Observers` decrement (floored at 0).
- `TestNpcInfo_TrackedNpc_RunWalkExtendIdle` — 4 branches.
- `TestNpcInfo_NewNpc_AddBranch` — pin add-header (BITS_ADD = 35), observer count incremented.
- `TestNpcInfo_NewNpc_RespectsBudget` — overflow → write 8191 terminator + early return.
- `TestNpcInfo_NewNpc_PreferredCap` — `PREFERRED_NPCS = 255` cap.
- `TestNpcInfo_FaceCoordOrientationFallback` — 3 branches as in B2.
- `TestNpcInfo_OutputBytesAreCopy` — same as B2.
- `TestNpcInfo_ObserverCountFloorsAtZero` — verify `Observers` does not go negative.

### Bundle 4 — Cutover + retirements

**Goal**: Wire `*Buf.PlayerInfo` + `*Buf.NpcInfo` fields, swap modules/world encoder callers to the new methods, flatten `pkg/buildarea` scenery fields onto `Player`, retire all dead packages and files.

**Tasks** (ordered; each gated on the prior):

- **T4.1** — `*Buf` field extension. In `pkg/rsbuf/buf.go`:
  ```go
  type Buf struct {
      players    [2048]*Player
      npcs       [8192]*Npc
      zoneMap    *zoneMap
      playerGrid map[uint32][]int32
      PlayerInfo *PlayerInfo
      NpcInfo    *NpcInfo
  }

  func New() *Buf {
      return &Buf{
          zoneMap:    newZoneMap(),
          playerGrid: map[uint32][]int32{},
          PlayerInfo: NewPlayerInfo(),
          NpcInfo:    NewNpcInfo(),
      }
  }
  ```
  Pure additive change. Tests: extend `TestNew_*` to assert non-nil PlayerInfo + NpcInfo fields.
- **T4.2** — Swap `modules/world/player_info.go`. Replace:
  ```go
  payload := rsbuf.Encode(p, sources, p.buildArea, s.grid, s.renderer)
  ```
  with:
  ```go
  payload := s.rsbuf.PlayerInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)
  ```
  Delete the now-unused `sources := make([]rsbuf.PlayerSource, len(snapshot))` build-up loop and the `playersMu` snapshot copy if no other use remains in the function. Update the nil-guard `if s == nil || p.buildArea == nil || s.renderer == nil || s.grid == nil` to `if s == nil || s.rsbuf == nil || s.renderer == nil` (drop `buildArea` and `grid` checks; add `rsbuf` check).
- **T4.3** — Swap `modules/world/player_npc_info.go` analogously.
- **T4.4** — Migrate downstream consumers of the package-level `pkg/rsbuf/npc_observers.go` shim to the `*Buf` method form, so T4.5's deletion grep returns zero matches. Two consumer sites (verified at spec-write time):
  - `modules/world/npc_hunt.go:41` — `rsbuf.GetNpcObservers(n.nid)` → `s.rsbuf.GetNpcObservers(int32(n.nid))` (1-line change inside `(s *Server) processNpcHunt`; `s.rsbuf` is reachable; add nil-guard `if s.rsbuf == nil { observers = 0 } else { observers = ... }` if other sites in the file pattern this way).
  - Test-only seeding: 7 call sites in `modules/world/npc_event_queue_test.go` use `rsbuf.SetObserverForTest(nid, count)`. Migrate by adding a new method `(b *Buf) SetObserverForTest(nid int32, count int32)` to `pkg/rsbuf/buf.go` (mirrors the existing package-level shim's contract — writes directly to `b.npcs[nid].Observers`, floors at 0 and clamps to slot bounds), then sweep test sites: `rsbuf.SetObserverForTest(...)` → `s.rsbuf.SetObserverForTest(int32(...), int32(...))`. Optional: instead of porting the test-only API, expose `(b *Buf) NpcObservers() func(nid int32) *int32` accessor — but the symmetric port matches existing test-helper expectations and keeps the call sites mechanical.
- **T4.5** — Flatten `pkg/buildarea` scenery fields onto `Player`. Plan-author enumerates ALL call sites via `rg "p\.buildArea\." modules/ pkg/ cmd/` and lists each in the plan per `enumerate_all_sites` memory; controller pre-flight re-greps before dispatch per `controller_preflight`. Steps:
  - Add fields to `Player` struct in `modules/world/player.go`: `lastBuild int`, `loadedZones map[int]bool`, `activeZones map[int]bool`, `mapsquares map[uint16]bool` (note: `originX, originZ` already exist on `Player`).
  - Initialize in `newPlayer()`: `lastBuild: 0`, `loadedZones: map[int]bool{}`, `activeZones: map[int]bool{}`, `mapsquares: map[uint16]bool{}`.
  - Add `(p *Player) shouldRebuild() bool` method (port of `BuildArea.ShouldRebuild` from `pkg/buildarea/buildarea.go:51-69`).
  - Add `(p *Player) rebuildScenery(currentTick int) []uint16` method (port of `BuildArea.Rebuild` from `pkg/buildarea/buildarea.go:73-107`).
  - Update each call site enumerated above. Approximate list (pre-flight re-grep mandatory):
    - `modules/world/data_map.go:116` `p.buildArea.LastBuild` → `p.lastBuild`
    - `modules/world/data_map.go:129` `p.buildArea.Mapsquares[mapsquare]` → `p.mapsquares[mapsquare]`
    - `modules/world/player.go:436` `p.buildArea.ShouldRebuild(...)` → `p.shouldRebuild()`
    - `modules/world/player.go:439` `p.buildArea.Rebuild(p.x, p.z, ...)` → `p.rebuildScenery(s.currentTick)`
    - `modules/world/player.go:455-465,476` `p.buildArea.LoadedZones`/`ActiveZones` → `p.loadedZones`/`activeZones`
    - `modules/world/login_map_test.go:83,85,102,104` `p.buildArea.OriginX/OriginZ` → `p.originX/originZ`
    - `modules/world/data_map_test.go:27,126,149,171,187,217,232,234,267,268` `p.buildArea.X` → `p.X`
    - `modules/world/player_zone_test.go:83,86` `p.buildArea.LoadedZones[idx]` → `p.loadedZones[idx]`
    - `modules/world/player_npc_test.go:63-64` `p.buildArea.Npcs` → `s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid))` (encoder state moved)
    - `modules/world/player_info_test.go:48-49` `a.buildArea.Players[2]` → `s.rsbuf.HasPlayer(int32(a.slot), 2)`
    - `modules/world/npc_event_queue_test.go:664-665` `p.buildArea.Npcs[101] = struct{}{}` → use new test helper that subscribes via `s.rsbuf` (or assert via `HasNpc`)
    - `modules/world/tick.go:95` `p.buildArea = buildarea.New()` → delete (Player initializes its own scenery fields in `newPlayer()`)
    - `modules/world/tick.go:173-174` `if p.buildArea != nil { rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs) }` → delete (already replaced by `s.rsbuf.RemovePlayer(int32(p.slot))` in NAI-29 at server.go:672 — verify and remove redundant call)
  - Delete `pkg/buildarea/` directory (`buildarea.go` + `buildarea_test.go`).
- **T4.6** — Verify-and-delete dead packages/files. Plan-author lists each grep command with expected zero-match output:
  - `rg "grid\.New\b|s\.grid\b|grid\.Grid\b|\"github.com/zsrv/goscape/pkg/grid\"" pkg/ modules/ cmd/` → 0 → delete `pkg/grid/`.
  - `rg "rsbuf\.PlayerSource\b|rsbuf\.NpcSource\b|AppearanceHash\(\)" pkg/ modules/ cmd/` → 0 → delete `pkg/rsbuf/source.go`, `pkg/rsbuf/npc_source.go`. Delete `(p *Player) AppearanceHash()` and the `appearanceHash` helper in `modules/world/appearance.go` if present.
  - `rg "rsbuf\.GetNpcObservers\b|rsbuf\.RemovePlayer\b\(|rsbuf\.SetObserverForTest\b|rsbuf\.incNpcObserver\b|rsbuf\.decNpcObserver\b" pkg/ modules/ cmd/` → 0 → delete `pkg/rsbuf/npc_observers.go` + `pkg/rsbuf/npc_observers_test.go`. Note: `s.rsbuf.GetNpcObservers(int32(nid))` (the `*Buf` method) is the supported API; the package-level shim retires.
  - `rg "rsbuf\.Encode\b\(|rsbuf\.EncodeNpc\b\(|rsbuf\.EncodeLegacy\b\(" pkg/ modules/ cmd/` → 0 → delete `EncodeLegacy` (or old `Encode`) + `EncodeNpcLegacy` (or old `EncodeNpc`) functions and any helpers (`writeLocalPlayer`, `writeOtherPlayers`, `writeNewPlayers` package functions; `boolToInt`, `clamp`, `zoneDist`, `abs` helpers if not consumed by the new `PlayerInfo`/`NpcInfo` methods).
  - `rg "modules/world/player_source\.go|modules/world/npc_source\.go" pkg/ modules/ cmd/` not applicable; instead: review each function in `player_source.go` and `npc_source.go` — delete those with 0 consumers (the methods existed only to satisfy the now-dead PlayerSource/NpcSource interfaces). Methods that have other consumers (e.g., `p.Visibility()` may be used elsewhere) stay and the file can be renamed `player_accessors.go` if file purpose changes substantially, or just trimmed. Plan-author re-greps each accessor.
- **T4.7** — Polish + close. Run `go test -race ./...` (must be green); run `go build -trimpath ./cmd/goscape`; smoke-test against Java client (user-launched per `smoke_test_server_handoff` memory); update `nai_followups.md` per "memory files" tech-stack note; ensure `Closes memory:` trailer per `close_commit_memory_trailer` memory.

**Tests** (B4):
- `TestNew_HasPlayerInfoAndNpcInfo` — `*Buf.New()` returns non-nil PlayerInfo + NpcInfo fields.
- `TestUpdatePlayers_DispatchesToBufPlayerInfo` — integration: stub `s.rsbuf.PlayerInfo` with a recording shim, run `p.updatePlayers()`, assert call.
- `TestUpdateNpcs_DispatchesToBufNpcInfo` — analogous.
- `TestPlayer_ShouldRebuild_OriginUnsetReturnsTrue` — port of `pkg/buildarea/buildarea_test.go` test.
- `TestPlayer_ShouldRebuild_WithinWindowReturnsFalse` — port.
- `TestPlayer_ShouldRebuild_OutsideWindowReturnsTrue` — port.
- `TestPlayer_ShouldRebuild_ReconnectingForcesTrue` — port.
- `TestPlayer_RebuildScenery_PopulatesMapsquaresAndZones` — port of `Rebuild` test.
- After T4.6 delete tasks: `go vet ./...` clean, no `unused: ...` warnings, no `imported and not used: ...`. Per `verify_implementer_claims` 30-second protocol.

## True-to-TS / true-to-Rust gate

NAI-30 maintains the `true_to_ts_gate` discipline against the upstream Rust source per `rust_source_canonical_path` memory. Every file in B2 + B3 carries a header doc-comment naming its upstream source file + line range; every public method carries its `info.rs:<line>-<line>` mapping comment. Side-by-side review against the source is the primary correctness gate per `flat_arg_signature_for_cross_lang_parity` memory.

The single new deviation is **NAI-30-D1** (orientation field plumbed without producer) — required by the structural decision that producer wiring is engine-system scope, not encoder-port scope.

The `lastAppearance` semantic correction is NOT a deviation; it's a *correction* of an NAI-29 parallel-write divergence that was inline-flagged with a `NAI-30: revisit` comment but never assigned a tag. Same for `OrientationX/Z` placeholder. Both retire at NAI-30 close; deviation count delta = +1 (just D1).

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Plan-author test fixture bugs in B2/B3 (arg-count, sentinel arithmetic — recurrence pattern per `plan_runnable_test_fixtures`) | `go test`-dry-run each codified test fixture in plan; controller pre-flight grep against actual mock struct field names per `mock_recorder_field_naming_check`; cross-check spec test strategy against each plan task's code block per `plan_test_coverage_crosscheck`. |
| Stale-IDE-diagnostic noise (recurrent in NAI-23/27/28/29 per `verify_implementer_claims`) | Confirm every false interface-mismatch / false unused-function warning via fresh CLI `go test -count=1`. |
| `pkg/buildarea` flatten misses a call site → compile error mid-B4 | Plan B4 enumerates ALL `p.buildArea.` and `buildarea.` matches per `enumerate_all_sites`; controller pre-flight re-greps before T4.5 dispatch per `controller_preflight`. |
| Wire-byte divergence from upstream goes undetected at unit-test level | Port existing branch-level pins; extend with new branch coverage; smoke test against Java client at B4 close per `smoke_test_server_handoff`. |
| Old `Encode` package-function helpers (`boolToInt`, `clamp`, `zoneDist`, `abs`) become dead but not deleted | T4.6 grep enumerates each helper individually; delete those with zero remaining consumers. Per `dead_api_polish`. |
| `playerGrid` tile-keyed map populated per-tick but unread until NAI-32 | Document inline as "populated for NAI-32 spiral-search; NAI-30 encoder uses zone-walk via zoneMap"; no action needed. Tracked in followups. |
| EXACT_MOVE encoder uses `player.origin` (local-player origin) for coord-subtraction; existing goscape mask_payload encoder might take different args | T2.8 plan-author reads `pkg/rsbuf/mask_payload.go` ExactMove encoder before codifying; if signature differs, extend the encoder method or inline the math in `writeBlocks`. |
| Renderer-cache ports (NAI-31 scope) are partially used in B2/B3 (the `renderer.has`/`cache`/`write` calls) — NAI-30 needs SOMETHING here | T2.7 + T3.6: call existing goscape `*Renderer` methods (which currently re-encode every tick — not cache-aware). NAI-31 closes the gap; doc-comment notes the inefficiency. |
| `tick.go:174` `rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)` is the OLD package-level shim using a DIFFERENT counter (package-level `npcObservers map[int]int`) than NAI-29's `s.rsbuf.RemovePlayer(int32(p.slot))` at server.go:672 (uses `*Buf.npcs[nid].Observers` field) — they touch parallel counters per `parallel_spatial_index_migration_pattern` memory | T4.5 deletes the tick.go:174 call (paired with the buildarea flatten that removes `p.buildArea.Npcs` field access); the `*Buf` decrement at server.go:672 becomes the single source of truth. T4.6 then deletes the dead `npc_observers.go` shim. |

## Sequencing

- B1 → B2: B2 needs `BuildArea.GetNearbyPlayers` (T1.1) and `lastAppearance`/`OrientationX/Z` field semantics (T1.4 + T1.5).
- B1 → B3: B3 needs `BuildArea.GetNearbyNpcs` (T1.2) and `OrientationX/Z` on Npc (T1.4).
- B2 + B3 can run in parallel (no shared files).
- B4 needs B2 + B3 (encoder methods exist).
- B4 sub-tasks ordered: T4.1 (Buf field extension) → T4.2 + T4.3 (caller swaps; can be parallel) → T4.4 (npc_observers shim consumer migration) → T4.5 (buildarea flatten) → T4.6 (dead-code delete) → T4.7 (polish + close).

## Memory entries to potentially add at NAI-30 close

To be assessed at close based on actual lessons learned:

- A "wire-level test port" entry if porting fixtures from interface-based to `*Buf`-based reveals a generalizable pattern (capture for future read-flip migrations).
- A "package-level vs method-level shim retirement" entry if the npc_observers shim retirement reveals a non-obvious step (e.g., test helpers needing rewiring).
- An "encoder API parameter trimming" entry if dropping `pos`/`dx`/`dz`/`rebuild` upstream params requires non-obvious doc-comment patterns (informs NAI-32 brainstorm when those params re-enter scope).

`Closes memory:` trailer on the close commit references `parallel_spatial_index_migration_pattern.md` (now retired for the player+npc spatial-index migration), `dead_api_polish.md` (multiple retirements), and `enumerate_all_sites.md` (B4 buildarea flatten).
