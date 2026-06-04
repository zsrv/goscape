# NAI-29 — pkg/rsbuf stateful core + entity model + parallel-write caller hooks

- **Sub-spec**: NAI-29
- **Date**: 2026-04-25
- **Scope label**: B (First sub-spec of a four-sub-spec series aligning goscape's `pkg/rsbuf` to the upstream `@2004scape/rsbuf` Rust crate's stateful API; introduces the entity-model + state-store + lifecycle/compute API skeleton; wires production caller hooks for parallel-write hygiene; does NOT change the existing `Encode`/`EncodeNpc` read path. Touches `pkg/rsbuf/` (new files: `idbitset.go`, `zonemap.go`, `player.go`, `npc.go`, `buildarea.go`, `buf.go`) + `modules/world/{server.go, npc_registry.go, tick.go}` for B4 caller hooks; ~1200 LOC production + ~1230 LOC tests across 4 bundles; introduces 0 new deviation tags; net deviation count 13 → 13)
- **Predecessors**: NAI-28 (Zone PathingEntity subscription primitive + huntNpcs/huntPlayers consumer migration) — last on `main` as `d29c1a0`
- **Source root**: `2004scape/rsbuf` branch `225` at `/home/owner/Code/github.com/2004scape/rsbuf/` (HEAD `1cbb2ce`)

## Motivation

Goscape's `pkg/rsbuf/` (2072 LOC at HEAD) is an **independent port** of the upstream `@2004scape/rsbuf` Rust/WASM crate, not a port of any TypeScript file in `LostCityRS/Engine-TS`. This was confirmed at brainstorm time: TS `PlayerInfo.ts` and `NpcInfo.ts` are 9-line stubs; `NetworkPlayer.ts:291,295` calls `rsbuf.playerInfo(...)` / `rsbuf.npcInfo(...)` against the native WASM binary. The WASM binary (`@2004scape/rsbuf` branch 225 = 4155 LOC of Rust) is therefore the canonical source of truth for goscape's pkg/rsbuf — not the TS stubs.

Audit at brainstorm time against the Rust source revealed that goscape's pkg/rsbuf is **architecturally divergent from upstream in a deeper way than the spatial-index choice**:

| Aspect | Upstream rsbuf (Rust/WASM) | Goscape pkg/rsbuf at HEAD |
|---|---|---|
| State model | Stateful module: register → push state → poll packet | Stateless monolithic `Encode(...)` taking all inputs |
| Spatial index | **Owned internally** (`ZONE_MAP` + `PLAYER_GRID`); never exposed | **Passed in** as `*grid.Grid` parameter |
| Per-tick state push | Distinct `compute_player(40+ args)` step writes to internal `Player` struct | Embedded in `Encode` via `PlayerSource` interface (caller-side state pull) |
| Build-area ownership | Internal to rsbuf (`build.rs` `BuildArea`) | External (`*buildarea.BuildArea` parameter) |
| Output API | Polled per-player: `playerInfo(pid)` returns `Vec<u8>` | Single shot: `Encode(self, all, ba, ...)` |
| Spatial-index of nearby slots/nids in encoder | Cached in rsbuf-internal Zone subscription | `g.NearbyPlayers`/`g.NearbyNpcs` calls into `pkg/grid` |

The brainstorm framing initially scoped a smaller "retire pkg/grid" sub-spec (encoder swaps `*grid.Grid` for caller-supplied `nearbySlots []int`). The Rust-source recon expanded the framing: aligning goscape's pkg/rsbuf to the upstream stateful API is a strictly larger and more upstream-faithful goal that subsumes pkg/grid retirement as a side-effect.

**The full alignment is too large for one sub-spec** (~2500-3500 LOC of new Go code + tests across the rsbuf entity model, state store, encoder loops, renderer parity, message-encoding parity, and dynamic optimizations). Scope was decomposed at brainstorm time into a four-sub-spec series:

| # | Sub-spec | Scope |
|---|---|---|
| **NAI-29** (this spec) | rsbuf stateful core + entity model | Port `Player`/`Npc`/`BuildArea`/`IdBitSet`/internal `ZoneMap`/`PLAYER_GRID`. Introduce `*Buf` instance + stateful API skeleton (`AddPlayer`/`RemovePlayer`/`AddNpc`/`RemoveNpc`/`ComputePlayer`/`ComputeNpc`/`Cleanup`/`HasPlayer`/`HasNpc`/`GetNpcObservers`). Wire parallel-write caller hooks. Existing `Encode`/`EncodeNpc` path **unchanged**. |
| **NAI-30** | rsbuf encoder loops + read-flip | Port `info.rs` (707 LOC). Replace `Encode`/`EncodeNpc` with `*Buf.PlayerInfo`/`NpcInfo`. Migrate `modules/world/{player_info.go, player_npc_info.go}`. Retire `pkg/grid/`, `pkg/buildarea/`, existing `npc_observers.go` shim, `PlayerSource`/`NpcSource` interfaces. |
| **NAI-31** | rsbuf renderer + message parity | Port `renderer.rs` (422 LOC) — close gaps in goscape's `renderer.go`. Port `message.rs` (629 LOC) — close gaps in `mask_payload.go`/`npc_mask_payload.go`. Independent of caller. |
| **NAI-32** | view_distance + rebuild + spiral | Port dynamic `view_distance` resize, `force_view_distance` flag, `rebuild` flag handling, `get_nearby_players_nearest` spiral fallback. |

NAI-29 is the foundation: at close, the `*Buf` state is fully populated by per-tick caller hooks (parallel-write window per `parallel_spatial_index_migration_pattern` memory) but **never read** by the existing encoder. This is intentional — it lets NAI-30 do a clean read-flip without entanglement, and lets B3's API design get exercised by integration tests in B4 against real per-tick state-push patterns rather than only by isolated unit tests.

The scope-decomposition discipline draws on `runescript_cadence`, `parallel_spatial_index_migration_pattern`, `controller_preflight`, `enumerate_all_sites`, and `spec_followup_tracker_freshness` memories.

## Tech stack

- Go 1.26+
- Existing packages **read** from at brainstorm time:
  - `pkg/rsbuf/source.go` (PlayerSource interface — preserved in NAI-29; retired at NAI-30)
  - `pkg/rsbuf/visibility.go` (`Visibility` enum — already aligned with upstream `visibility.rs`; reused as-is)
  - `pkg/rsbuf/npc_observers.go` (singleton `var npcObservers map[int]int` — preserved in NAI-29 as parallel store; retired at NAI-30)
  - `pkg/rsbuf/playerinfo.go`, `pkg/rsbuf/npcinfo.go` (existing encoder — unchanged)
  - `pkg/rsbuf/renderer.go` (existing renderer — unchanged)
  - `pkg/rsbuf/mask_payload.go`, `pkg/rsbuf/npc_mask_payload.go` (existing mask encoders — unchanged)
  - `pkg/coordgrid/coordgrid.go` (`PackCoord(level,x,z) int`, `UnpackCoord(int) Position`, `ZoneIndex(worldX,worldZ,level) int`, `DistanceToSW`, `Fine` — coord-packing layout matches upstream `coord.rs` exactly: `(z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)`; zone packing `((x>>3) & 0x7FF) | (((z>>3) & 0x7FF) << 11) | ((level & 0x3) << 22)`)
  - `pkg/zone/zone.go` (PathingEntity subscription primitive from NAI-28 — **separate** from rsbuf-internal zoneMap; different purpose: pkg/zone is for game-event subscription, rsbuf zoneMap is for encoder spatial-index)
- New files in `pkg/rsbuf/`:
  - `idbitset.go` (~80 LOC — bit-pack containment + ordered `[]int32` for iteration; mirrors upstream `build.rs:8-62`)
  - `zonemap.go` (~80 LOC — rsbuf-internal `zoneMap`/`zone` types; mirrors upstream `grid.rs`)
  - `player.go` (~150 LOC — `rsbuf.Player` + `Chat` + `ExactMove` substructs + `cleanup()`; mirrors upstream `player.rs`)
  - `npc.go` (~80 LOC — `rsbuf.Npc` + `cleanup()`; mirrors upstream `npc.rs`)
  - `buildarea.go` (~120 LOC — `rsbuf.BuildArea` with `idBitSet` for players/npcs + `appearances [2048]uint32` tick-keyed; **fixed `viewDistance = 15`**, no resize logic; mirrors upstream `build.rs:64-96` minus resize/rebuild_players/get_nearby_*)
  - `buf.go` (~330 LOC — `*Buf` instance + full public API; mirrors upstream `lib.rs`)
- Modified files in `modules/world/`:
  - `server.go` — `*Server` gains `rsbuf *rsbuf.Buf` field; `addPlayer` (line `:599`) hooks `s.rsbuf.AddPlayer(int32(p.slot))` after Zone enter; `removePlayer` (line `:651`) hooks `s.rsbuf.CleanupPlayerBuildArea(int32(p.slot))` + `s.rsbuf.RemovePlayer(int32(p.slot))` before Zone leave (or after, equivalently — independent paths); `runTickLoop` initializer sets `s.rsbuf = rsbuf.New()`
  - `npc_registry.go` — `addNpc` (line `:48`) hooks `s.rsbuf.AddNpc(int32(n.nid), int32(n.typ.Index))` (or equivalent type-id field) after Zone enter; `removeNpc` (line `:151`) hooks `s.rsbuf.RemoveNpc(int32(n.nid))` after Zone leave
  - `tick.go` — per-tick state-push loop after movement processing: `for _, p := range s.playerLoop { s.rsbuf.ComputePlayer(...) }`; analogous for `s.npcs`; final `s.rsbuf.Cleanup()` at end-of-tick after info encoding completes
- New test files:
  - `pkg/rsbuf/idbitset_test.go` (B1)
  - `pkg/rsbuf/zonemap_test.go` (B1)
  - `pkg/rsbuf/player_test.go` (B2)
  - `pkg/rsbuf/npc_test.go` (B2)
  - `pkg/rsbuf/buildarea_test.go` (B3)
  - `pkg/rsbuf/buf_test.go` (B3)
  - `modules/world/rsbuf_lifecycle_test.go` (B4 — integration: AddPlayer/RemovePlayer hook firing under login/logout flows)
  - `modules/world/rsbuf_per_tick_test.go` (B4 — integration: ComputePlayer/ComputeNpc state-push correctness across multi-tick movement)
- Memory files:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — at NAI-29 close, replace the existing "Deferred: pkg/grid full retirement" entry with a "Deferred: pkg/rsbuf upstream alignment series" entry tracking NAI-30 → NAI-32; add a "From NAI-29" close entry mirroring the NAI-28 close entry pattern

## Scope

### Bundle 1 — primitives (`idBitSet`, `zoneMap`, `playerGrid`)

**Goal**: Land the rsbuf-internal collections that back `*Buf`'s state. No public API surface, no `*Buf` instance yet — pure foundation for B2/B3.

**Source mappings**:

- `pkg/rsbuf/idbitset.go` (new) — port of upstream `build.rs:8-62`:
  ```go
  // idBitSet pairs a bit-array for O(1) containment with an ordered ID list
  // for iteration. Mirrors upstream build.rs IdBitSet.
  //
  // Used by rsbuf.BuildArea to track per-player observed players + npcs.
  type idBitSet struct {
      bits []uint32      // bits[id>>5] & (1 << (id & 0x1f))
      ids  []int32       // insertion-ordered list of contained ids
  }

  func newIdBitSet(maxID int, capacity int) *idBitSet
  func (s *idBitSet) Contains(id int32) bool
  func (s *idBitSet) Insert(id int32)        // no-op if already contained
  func (s *idBitSet) Remove(id int32)        // no-op if not contained
  func (s *idBitSet) Len() int
  func (s *idBitSet) Iter() []int32          // returns a copy of ids slice
  func (s *idBitSet) Clear()                 // bits.fill(0); ids = ids[:0]
  ```
  Upstream uses unsafe-ptr arithmetic (`*self.bits.as_ptr().add((id >> 5) as usize)`); Go uses bounds-checked indexing — the Go-idiom translation is externally invisible per `true_to_ts_gate`.
- `pkg/rsbuf/zonemap.go` (new) — port of upstream `grid.rs`:
  ```go
  // zone holds the player + npc id sets for a single zone. Mirrors
  // upstream grid.rs Zone.
  type zone struct {
      players map[int32]struct{} // pids
      npcs    map[int32]struct{} // nids
  }

  func newZone() *zone
  func (z *zone) AddPlayer(pid int32)        // players[pid] = struct{}{}
  func (z *zone) RemovePlayer(pid int32)     // delete(players, pid)
  func (z *zone) AddNpc(nid int32)
  func (z *zone) RemoveNpc(nid int32)

  // zoneMap is the rsbuf-internal spatial index keyed by packed zone
  // index. Mirrors upstream grid.rs ZoneMap.
  type zoneMap struct {
      zones map[uint32]*zone
  }

  func newZoneMap() *zoneMap
  // Zone returns the zone at (x, level, z), creating an empty one on miss.
  // Coord packing matches upstream ZoneMap::zone_index exactly:
  //   ((x >> 3) & 0x7ff) | (((z >> 3) & 0x7ff) << 11) | ((level & 0x3) << 22)
  func (m *zoneMap) Zone(x, level, z int) *zone
  ```
  Upstream `IntSet<i32>` (nohash hasher) becomes Go `map[int32]struct{}`. Upstream `IntMap<u32, Zone>` with capacity-reserved `0xffffff` becomes Go `map[uint32]*zone`. The capacity-reservation behavior is dropped (Go map growth is amortized) — no observable difference.
- The `playerGrid` tile-keyed map (used by NAI-32's spiral search) is **not a separate type**; declared inline inside `*Buf` as `playerGrid map[uint32][]int32`. B1 includes the zero-init plumbing only; no helpers in B1.

**Test strategy** (B1, ~250 LOC):
- `idbitset_test.go`: insert/contains/remove round-trip; double-insert idempotency; double-remove idempotency; iter ordering matches insertion order; clear resets both bits and ids; bounds — ids 0, 1, 31, 32, max-1 all addressable.
- `zonemap_test.go`: zone create-on-miss returns an empty zone; same coord returns same zone; zone-index packing — `(x=8, level=0, z=0)` differs from `(x=0, level=0, z=8)`; level differentiates same-(x,z); player/npc sets are independent.

**Deviation tags**: 0 introduced. The unsafe-ptr-vs-bounds-check substitution and IntSet→map substitution are Go-idiom translations per `true_to_ts_gate`.

### Bundle 2 — entity structs (`Player`, `Npc`)

**Goal**: Land the per-entity state-snapshot structs that `ComputePlayer`/`ComputeNpc` will populate. No `*Buf`, no `BuildArea`, no public API — just the structs + their `cleanup()` methods.

**Source mappings**:

- `pkg/rsbuf/player.go` (new) — port of upstream `player.rs`:
  ```go
  // Player is the per-tick state snapshot the encoder reads from.
  // Mirrors upstream player.rs Player. Field order matches upstream
  // for side-by-side review.
  type Player struct {
      Coord    int                // pkg/coordgrid.PackCoord(level, x, z) packed
      Origin   int                // pkg/coordgrid.PackCoord(level, originX, originZ) packed
      PID      int32
      Tele     bool
      Jump     bool
      RunDir   int8               // -1 sentinel = no run
      WalkDir  int8               // -1 sentinel = no walk
      Visibility Visibility
      Active   bool
      Build    *BuildArea         // forward-declared; populated by *Buf.New
      Masks    uint32
      Appearance      []byte
      LastAppearance  int32
      FaceEntity      int32
      FaceX, FaceZ    int32
      OrientationX, OrientationZ int32
      DamageTaken, DamageType int32
      CurrentHitpoints, BaseHitpoints int32
      AnimID, AnimDelay int32
      Say     *string             // nil = no say this tick
      Chat    *Chat
      GraphicID, GraphicHeight, GraphicDelay int32
      ExactMove *ExactMove
  }

  // Chat carries chat-message payload + formatting.
  // Mirrors upstream player.rs Chat.
  type Chat struct {
      Bytes        []byte
      Color, Effect, Ignored uint8
  }

  // ExactMove carries exact-move animation parameters.
  // Mirrors upstream player.rs ExactMove.
  type ExactMove struct {
      StartX, StartZ, EndX, EndZ int32
      Begin, Finish, Dir         int32
  }

  // newPlayer constructs a Player at zero-coord with sentinel defaults.
  // Mirrors upstream Player::new at player.rs:60.
  func newPlayer(pid int32) *Player

  // cleanup zeros transient per-tick state but keeps persistent state
  // (appearance, lastAppearance, faceEntity, orientationX/Z) per
  // upstream player.rs:96-121 commented-out lines.
  func (p *Player) cleanup()
  ```
  Note `Build *BuildArea` field — declared here for layout, populated by `(*Buf).AddPlayer` at B3. B2's player_test.go uses a stub `&BuildArea{}` literal until B3 lands the real type.
- `pkg/rsbuf/npc.go` (new) — port of upstream `npc.rs`:
  ```go
  // Npc is the per-tick state snapshot the encoder reads from.
  // Mirrors upstream npc.rs Npc. Field order matches upstream.
  type Npc struct {
      Coord    int
      NID      int32
      NType    int32
      Tele     bool
      RunDir   int8
      WalkDir  int8
      Active   bool
      Masks    uint32
      FaceEntity int32
      FaceX, FaceZ int32
      OrientationX, OrientationZ int32
      DamageTaken, DamageType int32
      CurrentHitpoints, BaseHitpoints int32
      AnimID, AnimDelay int32
      Say     *string
      GraphicID, GraphicHeight, GraphicDelay int32
      Observers int32
  }

  // newNpc constructs an Npc at zero-coord with sentinel defaults.
  // Mirrors upstream Npc::new at npc.rs:32.
  func newNpc(nid, ntype int32) *Npc

  // cleanup zeros transient per-tick state but keeps faceEntity +
  // orientationX/Z per upstream npc.rs:62-83 commented-out lines.
  func (n *Npc) cleanup()
  ```

**Test strategy** (B2, ~280 LOC):
- `player_test.go`: zero-init produces sentinel defaults (Coord=0, RunDir=-1, WalkDir=-1, FaceEntity=-1, all `int32` *id*-fields = -1, Masks=0, slices/pointers nil); `cleanup()` zeros transient (walkDir/runDir → -1, jump/tele/masks/chat/exactMove → zero/nil) and **preserves** persistent (appearance, lastAppearance, faceEntity, orientationX/Z); Chat and ExactMove construction round-trips.
- `npc_test.go`: zero-init defaults; `cleanup()` preserves faceEntity + orientationX/Z; transient cleared; Observers field is **not** touched by cleanup (observer count persists across ticks).

**Deviation tags**: 0 introduced. Field naming follows Go idiom (PascalCase, not snake_case); coord stored as packed `int` instead of dedicated `CoordGrid` substruct (Go-idiom; the packing layout is identical via `pkg/coordgrid.PackCoord`).

### Bundle 3 — `BuildArea`, `*Buf`, full public API

**Goal**: Land the complete `*Buf` instance handle and stateful API. After B3 close, `*Buf` is fully constructable, `AddPlayer`/`ComputePlayer`/`HasPlayer`/`Cleanup` operate as upstream-faithful state machines, and B1's primitives + B2's structs are wired together. No production caller yet — exercised entirely via unit tests.

**Source mappings**:

- `pkg/rsbuf/buildarea.go` (new) — port of upstream `build.rs:64-158` minus resize/rebuild_players/get_nearby_*:
  ```go
  // BuildArea tracks per-player encoder state: the set of currently-
  // observed players + npcs, and a tick-keyed map of recently-sent
  // appearance hashes. Mirrors upstream build.rs BuildArea.
  //
  // Sizing constants match upstream:
  const (
      preferredPlayers     = 250
      preferredNpcs        = 255
      preferredViewDistance uint8 = 15
      // viewDistanceResizeInterval = 10  // NAI-32: dynamic resize
  )

  type BuildArea struct {
      Players       *idBitSet         // 2048-bit bit-array, capacity 250
      Npcs          *idBitSet         // 8192-bit bit-array, capacity 255
      appearances   [2048]uint32      // appearances[pid] = tick-when-last-sent
      // forceViewDistance bool       // NAI-32
      ViewDistance  uint8             // fixed 15 in NAI-29-30; resize-able in NAI-32
      // lastResize uint32             // NAI-32
  }

  func newBuildArea() *BuildArea
  func (b *BuildArea) Cleanup()                     // clear players/npcs; appearances.fill(0)
  func (b *BuildArea) HasAppearance(pid int32, tick uint32) bool
  func (b *BuildArea) SaveAppearance(pid int32, tick uint32)
  ```
  Deferred (NAI-32): `forceViewDistance` field, `lastResize` field, `Resize()` method, `RebuildPlayers()`, `RebuildNpcs()`, `getNearbyPlayers*`, `getNearbyNpcs`, `filterPlayer`, `filterNpc`. NAI-29's BuildArea is the stripped-down "tracking + appearance" core; the spatial-discovery + view-distance optimization layer comes later.
- `pkg/rsbuf/buf.go` (new) — port of upstream `lib.rs`:
  ```go
  // Buf is the rsbuf instance handle. One per world. Mirrors the
  // upstream lib.rs unsafe-static globals collected onto a single
  // value type. All methods are tick-goroutine-owned — no internal
  // synchronization (matches upstream's WASM single-threaded model).
  type Buf struct {
      players        [2048]*Player
      npcs           [8192]*Npc
      zoneMap        *zoneMap
      playerGrid     map[uint32][]int32 // tile-keyed (NAI-32 spiral search)
      // playerRenderer / npcRenderer / playerInfo / npcInfo — owned by
      // the existing Renderer/Encoder until NAI-30/31
  }

  // New constructs an empty Buf with all slot tables nil-initialized,
  // empty zoneMap, empty playerGrid.
  func New() *Buf

  // Slot lifecycle ----------------------------------------------------

  // AddPlayer registers pid by allocating a *Player at slot[pid] with
  // sentinel defaults. Caller must call this before any subsequent
  // ComputePlayer for the same pid. Mirrors lib.rs:179.
  // No-op if pid == -1 or pid >= 2048.
  func (b *Buf) AddPlayer(pid int32)

  // RemovePlayer unregisters pid. Steps (mirroring lib.rs:187-203):
  //   1. Remove pid from the zoneMap zone at the player's last coord
  //   2. For each nid in player.Build.Npcs.Iter(), decrement npcs[nid].Observers (floor at 0)
  //   3. Call player.Build.Cleanup() (clears tracking + appearances)
  //   4. (NAI-30) PLAYER_RENDERER.removePermanent(pid)
  //   5. Set slot[pid] = nil
  //
  // No-op if pid == -1 or slot[pid] is nil.
  func (b *Buf) RemovePlayer(pid int32)

  func (b *Buf) AddNpc(nid, ntype int32)        // mirrors lib.rs:306
  func (b *Buf) RemoveNpc(nid int32)            // mirrors lib.rs:313 — removes from zoneMap

  // Per-tick state push -----------------------------------------------

  // ComputePlayer writes ALL per-tick state for pid in one call. Mirrors
  // upstream lib.rs:39-153 compute_player. Argument order matches upstream
  // verbatim for side-by-side review.
  //
  // Side effects:
  //   1. If new coord crosses zone boundary: zoneMap.Zone(old).RemovePlayer(pid)
  //      then zoneMap.Zone(new).AddPlayer(pid)
  //   2. Write all 35+ fields onto players[pid]
  //   3. (NAI-30) PLAYER_RENDERER.compute_info(player) — currently skipped
  //   4. playerGrid[player.Coord] append pid (tile-keyed; NAI-32 spiral search)
  //
  // No-op if pid == -1 or slot[pid] is nil.
  func (b *Buf) ComputePlayer(
      pid int32,
      x, level, z int,
      originX, originZ int,
      tele, jump bool,
      runDir, walkDir int8,
      visibility Visibility,
      active bool,
      masks uint32,
      appearance []byte,
      lastAppearance int32,
      faceEntity, faceX, faceZ int32,
      orientationX, orientationZ int32,
      damageTaken, damageType int32,
      currentHitpoints, baseHitpoints int32,
      animID, animDelay int32,
      say *string,
      message []byte, color, effect, ignored uint8,
      graphicID, graphicHeight, graphicDelay int32,
      exactStartX, exactStartZ int32,
      exactEndX, exactEndZ int32,
      exactMoveStart, exactMoveEnd, exactMoveDirection int32,
  )

  // ComputeNpc analogous; mirrors lib.rs:217-281 compute_npc.
  func (b *Buf) ComputeNpc(
      nid, ntype int32,
      x, level, z int,
      tele bool,
      runDir, walkDir int8,
      active bool,
      masks uint32,
      faceEntity, faceX, faceZ int32,
      orientationX, orientationZ int32,
      damageTaken, damageType int32,
      currentHitpoints, baseHitpoints int32,
      animID, animDelay int32,
      say *string,
      graphicID, graphicHeight, graphicDelay int32,
  )

  // End-of-tick -------------------------------------------------------

  // Cleanup resets the tile-keyed playerGrid and calls cleanup() on
  // every populated Player + Npc. Mirrors lib.rs:348.
  // (NAI-30) PLAYER_RENDERER.removeTemporary + NPC_RENDERER.removeTemporary
  // are skipped here pending NAI-31.
  func (b *Buf) Cleanup()

  // CleanupPlayerBuildArea calls Cleanup on the named player's BuildArea
  // (clears tracking + appearances). Used at logout pre-flush. Mirrors
  // lib.rs:365.
  func (b *Buf) CleanupPlayerBuildArea(pid int32)

  // Observer queries --------------------------------------------------

  // HasPlayer reports whether pid currently observes other.
  // Mirrors lib.rs:205.
  func (b *Buf) HasPlayer(pid, other int32) bool

  // HasNpc reports whether pid currently observes nid.
  // Mirrors lib.rs:326.
  func (b *Buf) HasNpc(pid, nid int32) bool

  // GetNpcObservers returns the count of players currently observing nid.
  // Mirrors lib.rs:337.
  func (b *Buf) GetNpcObservers(nid int32) int32
  ```
  **Coord arg order — `x, level, z`**: upstream Rust uses `x, y, z` (`y = level` in RS coord parlance) but goscape's existing pkg/coordgrid uses `level, x, z` for `PackCoord`. The plan resolves this with explicit named-arg discipline at call sites and a doc-comment note. This is **not** a deviation tag (it's an internal-Go-idiom binding).

**Test strategy** (B3, ~450 LOC):
- `buildarea_test.go`: zero-init produces empty `Players`/`Npcs` bitsets + zeroed appearances + viewDistance=15; `Cleanup()` clears all three; `HasAppearance` returns false for tick 0 on fresh buildarea; `SaveAppearance(pid, tick)` then `HasAppearance(pid, tick)` round-trips.
- `buf_test.go`:
  - **Slot lifecycle**: `AddPlayer(5)` then `players[5] != nil`; `RemovePlayer(5)` then nil; double-add is overwrite (matches upstream `*PLAYERS.as_mut_ptr().add(pid as usize) = Some(Player::new(pid))`); add at `pid=-1` is no-op; `AddNpc(3, 100)` populates `npcs[3].NType == 100`.
  - **ComputePlayer write-through**: AddPlayer then ComputePlayer with non-zero fields; verify all 35+ fields are populated correctly on `players[pid]`.
  - **Cross-zone migration**: Add at coord=(x=10, z=10), Compute at (x=10, z=10), then Compute at (x=10, z=20) — assert old zone has 0 players, new zone has pid; `playerGrid[oldPacked]` has 0 entries for pid, `playerGrid[newPacked]` has pid. Same-zone move (x=10,z=11→x=10,z=12) does **not** trigger zoneMap remove+add (matches upstream `lib.rs:116`'s zone-bound check) but **does** push the new tile into playerGrid (matches `lib.rs:151`'s unconditional grid push).
  - **Observer increment + decrement**: B3's API doesn't yet do this directly (encoder owns the increment); but `RemovePlayer` must iterate `player.Build.Npcs` and decrement each `npcs[nid].Observers` (lib.rs:194-198). Test by hand-seeding a BuildArea with `Npcs.Insert(nid)` for some nids, AddNpc'ing those, then RemovePlayer — assert each npc.Observers decremented by 1 (floor 0).
  - **Cleanup transient/persistent invariants**: Compute populates Player.Appearance with bytes; `Cleanup()` does NOT clear Appearance (per upstream `// self.appearance = vec![]` comment); does clear walkDir/runDir/jump/tele/masks/chat/exactMove.
  - **HasPlayer / HasNpc**: hand-seed BuildArea via `Players.Insert(other)` then `HasPlayer(pid, other) == true`; `HasPlayer(pid, neverInsertedID) == false`; `HasPlayer(-1, anything) == false`; `HasPlayer(pid, -1) == false`.
  - **GetNpcObservers**: AddNpc(50, 100), seed `npcs[50].Observers = 3`, assert `GetNpcObservers(50) == 3`; never-added returns 0.
- **No integration tests in B3** — B4 owns those.

**Deviation tags**: 0 introduced. Static-mut globals → struct fields, `Option<Player>` → `*Player` with nil-check, `Vec<u8>` → `[]byte`, `String` → `*string` for nullable — all Go-idiom translations.

### Bundle 4 — caller wiring (parallel-write window)

**Goal**: Wire `*Buf` into production. After B4 close, every player position update + npc spawn flows through `*Buf` alongside the existing pkg/grid + pkg/zone updates. Existing encoder is **unchanged** — `*Buf` state is populated but never read.

**Source mappings**:

- `modules/world/server.go`:
  - `*Server` struct gains `rsbuf *rsbuf.Buf` field (between existing `npcLookup` and other lookup fields, alphabetic order).
  - In `(*Server).runTickLoop` initializer (current location of `s.zoneMap = ...` from NAI-28 setup): add `s.rsbuf = rsbuf.New()`.
  - `(*Server).addPlayer` at `:599-619`: after the existing `if s.zoneMap != nil { ... p.zoneListElement = z.EnterPlayer(...) }` block (around line `:611`), add:
    ```go
    if s.rsbuf != nil {
        s.rsbuf.AddPlayer(int32(p.slot))
    }
    ```
  - `(*Server).removePlayer` at `:643-665`: after the existing `if s.zoneMap != nil && p.zoneListElement != nil { ... LeavePlayer(...) }` block, add:
    ```go
    if s.rsbuf != nil {
        s.rsbuf.CleanupPlayerBuildArea(int32(p.slot))
        s.rsbuf.RemovePlayer(int32(p.slot))
    }
    ```
- `modules/world/npc_registry.go`:
  - `(*Server).addNpc` at `:48`: after the existing Zone EnterNpc wiring (from NAI-28), add `s.rsbuf.AddNpc(int32(n.nid), int32(n.typ.Index))` — verify the type-id field name at plan time (could be `n.typ.ID`, `n.typ.NID`, etc.).
  - `(*Server).removeNpc` at `:151`: after Zone LeaveNpc, add `s.rsbuf.RemoveNpc(int32(n.nid))`.
- `modules/world/tick.go`:
  - After the per-tick movement block (player loop ending around line `:325`, npc loop ending around line `:360`), insert per-tick state-push:
    ```go
    // Push per-tick state into the rsbuf stateful core (parallel-write
    // window — state is populated but not yet read by the encoder; that
    // happens at NAI-30). See docs/superpowers/specs/2026-04-25-nai-29-...
    if s.rsbuf != nil {
        for _, p := range s.playerLoop {
            if p == nil { continue }
            s.rsbuf.ComputePlayer(int32(p.slot),
                p.x, p.level, p.z,
                p.originX, p.originZ,
                /* ... */)
        }
        for _, n := range s.npcs {
            if n == nil || !n.active { continue }
            s.rsbuf.ComputeNpc(int32(n.nid), int32(n.typ.Index),
                n.x, n.level, n.z,
                /* ... */)
        }
    }
    ```
  - At end-of-tick (after info encoding completes, around current `currentTick++` site): `if s.rsbuf != nil { s.rsbuf.Cleanup() }`.
  - **Plan-author task**: enumerate every field on `*Player` and `*Npc` that maps to a ComputePlayer/ComputeNpc arg; produce the verbatim call site. Per `enumerate_all_sites` and `controller_preflight` memories, this is brainstorm-time discovery work, not implementer-time discovery.
- **Test fixture impact**: ~25+ existing test fixtures in `modules/world/*_test.go` initialize `*Server` (some with `s.grid = grid.New()` from earlier tests). Those fixtures may or may not need `s.rsbuf = rsbuf.New()` — depends on whether the test exercises a code path that hits the rsbuf hooks. **Plan-author task**: pre-enumerate which test fixtures hit `addPlayer`/`removePlayer`/`addNpc`/`removeNpc`/`tick.go` movement loops, and identify a shared helper (`addPlayerToServer` and friends already exist post-NAI-28) for the wire-up. Per `plan_helper_coverage` memory: cross-check the helper's flag set against every consumer.

**Test strategy** (B4, ~250 LOC):
- `modules/world/rsbuf_lifecycle_test.go` (new):
  - Login flow: addPlayer → assert `s.rsbuf.HasPlayer(slot, slot) == false` initially (BuildArea empty), `s.rsbuf` slot[pid] is non-nil.
  - Logout flow: removePlayer → assert slot[pid] is nil (state cleaned).
  - Multi-add/remove cycles: add 5 players, remove 2, assert remaining 3 are still tracked.
- `modules/world/rsbuf_per_tick_test.go` (new):
  - Single-tick state push: addPlayer + 1 tick + ComputePlayer fires + assert players[pid].Coord matches.
  - Cross-zone movement: tick 1 at (x=50, z=50), tick 2 moves to (x=64, z=50). Zone of x=50 is `50>>3=6` (zone spans `x=48..x=55`); zone of x=64 is `64>>3=8` (zone spans `x=64..x=71`). Cross-zone confirmed. Assert old-zone player set excludes pid post-move; new-zone includes pid. Also pin a same-zone move test: tick 1 at (x=50, z=50), tick 2 to (x=55, z=50) — both in zone 6 (`50>>3=55>>3=6`); zoneMap state must be unchanged but `playerGrid` (tile-keyed) still receives the new entry per upstream `lib.rs:151`'s unconditional grid push.
  - End-of-tick cleanup: `Cleanup()` fires → next tick's ComputePlayer overwrites; transient state (RunDir, WalkDir, Tele, Jump, Masks) reset to defaults between ticks.
  - Negative-pin (per `ts_asymmetry_dual_pin`): existing pkg/grid state remains independently maintained — verify both indexes contain pid after tick 1 (parallel-write invariant). NAI-30 will retire this dual maintenance.
- **No byte-level encoder tests** — encoder unchanged; existing encoder tests stay green.

**Risk/scope checks for B4**:
- Hook ordering: `s.rsbuf.AddPlayer` must follow `EnterPlayer` (not precede) so the rsbuf state push at the **next** tick's ComputePlayer writes to a slot that already exists. If hook ordering is reversed, a between-add-and-Compute window exists where `slot[pid]` is allocated but coord is zero — harmless but plan should document.
- `addPlayer` is called under `s.playersMu.Lock()` (server.go:599). The `s.rsbuf` state writes happen while holding the player lock. This is fine (rsbuf is tick-owned; login goroutine handing off to tick goroutine via `newPlayers` channel → tick goroutine drains → addPlayer is called from tick goroutine context). Plan should pin: `s.rsbuf` state mutation is **always** on the tick goroutine.

**Deviation tags**: 0 introduced.

## Out of scope (deferred to subsequent NAI sub-specs)

Tracked in `nai_followups.md` "Deferred: pkg/rsbuf upstream alignment series" entry at NAI-29 close:

| Item | Target | Reason |
|---|---|---|
| `BuildArea.Resize` view-distance dynamic shrink/grow | NAI-32 | Optimization; encoder works with fixed 15 in NAI-29-30 |
| `BuildArea.RebuildPlayers` / `RebuildNpcs` (full repaint optimization) | NAI-32 | Tied to mapsquare-shift + force-rebuild flag |
| `BuildArea.getNearbyPlayersZones` / `getNearbyPlayersNearest` / `getNearbyNpcs` | NAI-32 | Spatial-discovery used by encoder; encoder doesn't migrate until NAI-30 |
| `BuildArea.filterPlayer` / `filterNpc` | NAI-32 | Used by getNearby* |
| `BuildArea.forceViewDistance` flag | NAI-32 | Engine-override hook; no caller today |
| `BuildArea.lastResize` field | NAI-32 | Resize bookkeeping |
| `rebuild` flag handling in encoder | NAI-30 | Encoder concern (full repaint signal at mapsquare shift) |
| `Renderer.compute_info(&player)` wiring inside `ComputePlayer` | NAI-30 or NAI-31 | Existing renderer is independent; cache wiring matters when encoder reads it |
| `Encode` / `EncodeNpc` retirement (`pkg/rsbuf/{playerinfo.go, npcinfo.go}`) | NAI-30 | Read-flip + caller migration |
| `pkg/grid` retirement (entire package) | NAI-30 | Falls out when last consumer migrates |
| `pkg/buildarea` retirement (entire package) | NAI-30 | Falls out when encoder migrates |
| `pkg/rsbuf/npc_observers.go` retirement | NAI-30 | Parallel-write at NAI-29; canonical at NAI-30 (the new `*Buf.GetNpcObservers` becomes authoritative) |
| `pkg/rsbuf/source.go` (`PlayerSource`) + `pkg/rsbuf/npc_source.go` (`NpcSource`) interfaces retirement | NAI-30 | Encoder no longer reads via interface — caller pushes via ComputePlayer |
| `renderer.rs` (422 LOC) full port | NAI-31 | Independent of stateful core |
| `message.rs` (629 LOC) full mask-payload parity | NAI-31 | Independent of stateful core |

## Test strategy summary

- **No byte-level wire-format tests in NAI-29.** Existing tests in `pkg/rsbuf/playerinfo_test.go`, `pkg/rsbuf/npcinfo_test.go`, etc. stay green and unchanged. The encoder is not touched; therefore wire-format invariants are preserved by construction.
- **Per-bundle unit tests** for B1/B2/B3 cover the new types/API in isolation. Coverage targets: every public method has at least a happy-path + edge-case test.
- **Integration tests in B4** cover the lifecycle hooks + per-tick state push under realistic `*Server` setup. Use existing `addPlayerToServer` / `addNpcToServerAt` test helpers (post-NAI-28).
- **Negative-pin tests** in B4 confirm the parallel-write invariant: both pkg/grid and `*Buf` state are populated on entity moves. NAI-30 will retire this dual maintenance — at NAI-30 the negative-pin tests flip into deletion candidates.
- **Stale-IDE-diagnostic discipline** per `verify_implementer_claims` failure-mode #1: fresh `go test ./... -count=1 -race` + `go build ./...` after each bundle close. gopls cache lag has produced false warnings across every NAI sub-spec so far on this project.

## Risk register

| Risk | Mitigation |
|---|---|
| `ComputePlayer`'s 40-arg signature is ugly and easy to misuse | Doc comment with field correspondence; verbatim upstream order; B3 test cases use named-arg discipline (every arg labeled in the call site comment); B4 plan task generates the verbatim production call site |
| `rsbuf.Npc.Observers` and existing `npc_observers.go` double-count | Both updated in NAI-29 via parallel-write. NAI-30 retires the duplicate. B3 test pins the invariant: counts match across both stores after add/remove sequences |
| Stale-IDE-diagnostic noise from gopls (recurrent across NAI-25/26/27/28) | Fresh `go test` + `go build` after each bundle per `verify_implementer_claims` failure-mode #1 |
| B4 caller-site hook locations may be more complex than expected (e.g., logout cleanup sequence ordering vs Zone leave) | Pre-grep call sites in plan; controller_preflight before B4 dispatch per `controller_preflight` memory; enumerate verbatim call sites in plan tasks per `enumerate_all_sites` |
| Test fixtures in `modules/world` may break when `s.rsbuf` is required | B4 plan-author task: enumerate fixture sites that hit rsbuf hooks; cross-check shared helper coverage per `plan_helper_coverage`; pre-list affected fixtures |
| `n.typ.Index` field name guess for AddNpc — actual field may be `ID` / `NID` / etc. | Plan-author verifies at plan time via `rg "func \(.*Npc\)" pkg/entity/` and `rg "n\.typ\." modules/world/`; spec will be revised if guess is wrong |
| Goscape's existing `pkg/rsbuf/Renderer` doesn't sync with upstream's `compute_info(&player)` cache | Out of scope — NAI-31 owns renderer parity. NAI-29 leaves existing renderer untouched |
| `s.playersMu.Lock()` held during `addPlayer`'s rsbuf hook — concurrency assumption | Plan documents that all rsbuf state mutation is on the tick goroutine; handoff from connection goroutines goes through the existing `s.newPlayers` channel; no rsbuf-internal lock needed |
| Plan author may codify "by analogy" code blocks instead of reading upstream Rust line-by-line | Per `spec_ts_source_read` (and its source-agnostic generalization to Rust): plan author quotes the corresponding Rust struct/method body verbatim in each plan task; no analogical inference |

## TS-faithfulness gates (Rust-source-faithfulness)

Per `true_to_ts_gate` and `spec_ts_source_read` memories (generalized to Rust source for this sub-spec since `pkg/rsbuf` is a port of the Rust crate, not TS):

- **Read primary source for each new type.** Plan author reads `build.rs`, `player.rs`, `npc.rs`, `grid.rs`, `lib.rs` line-by-line; codifies field-by-field correspondence in plan task code blocks.
- **No "by analogy" code blocks.** Each plan task that creates a new type or method quotes the corresponding Rust struct definition or fn body verbatim and pins the Go translation.
- **Deferred features are tagged in the spec, not the code.** `view_distance` resize / `get_nearby_*` / `rebuild` / `force_view_distance` / renderer.compute_info wiring go into the "Out of scope" register — no per-line `DEVIATION:` comments in production code (those are reserved for true asymmetries, not future work).
- **Externally-invisible Go-idiom translations introduce no deviation tags.** `Option<Player>` → `*Player`, `IntSet<i32>` → `map[int32]struct{}`, `static mut` → `*Buf` field, snake_case → PascalCase field naming, `Vec<u8>` → `[]byte`, `Option<String>` → `*string`. Same observable semantics.

## Memory entries to add at NAI-29 close

Per `close_commit_memory_trailer` memory: NAI-29 close commit body adds `Closes memory:` trailer naming each new entry.

Anticipated new entries (subject to revision based on actual findings during execution):

- `nai_followups.md`: Replace existing "Deferred: pkg/grid full retirement" with broader "Deferred: pkg/rsbuf upstream alignment series" entry tracking NAI-30 → NAI-32 with cross-references.
- `nai_followups.md`: Standard "From NAI-29" close entry summarizing the four-bundle execution + new memory entries + lessons learned.
- Possible new memory entry on **Rust-source canonical-path discipline** (analogue to `ts_source_canonical_path` for the rsbuf-port subset): never read `Engine-TS_274` rsbuf forks; always `2004scape/rsbuf` branch 225.
- Possible new memory entry on **flat-arg signature discipline for cross-language API parity** — when porting a 40-arg Rust function, keep the flat arg list and explicit positional order rather than refactoring to a struct, so side-by-side review against the source remains tractable.

## Bundle ordering rationale

B1 → B2 → B3 → B4 is the only sane order:

- **B1** lands the foundation primitives (`idBitSet`, `zoneMap`/`zone`). No dependencies on B2 or B3.
- **B2** lands `Player` + `Npc` structs. `Player` declares a `Build *BuildArea` field; since `BuildArea` is the type introduced in B3, B2's package-level forward-declaration via pointer-to-incomplete-type works in Go (the compiler resolves `*BuildArea` once B3 lands the concrete type in the same package). B2's `player_test.go` uses an empty `&BuildArea{}` stub literal until B3 fleshes out the type. **Cross-bundle test compile dependency**: B2's tests must remain green after B3 introduces the real `BuildArea` — the stub literal must be replaceable with the real constructor without semantic shift.
- **B3** depends on B1 (`zoneMap` for `*Buf`'s spatial index, `idBitSet` for `BuildArea`'s tracking sets) and B2 (`Player` and `Npc` structs allocated in `*Buf`'s slot tables). B3 itself introduces `BuildArea` and the full `*Buf` public API.
- **B4** depends on B3's full public API.

There is no productive parallelism across bundles. Single-implementer serial execution.
