# rev-244 B3 — engine core — design

**Date:** 2026-06-04
**Status:** Approved
**Branch:** rev-244 (umbrella: `2026-06-03-rev244-port-design.md` §B3; resume
context: `docs/superpowers/handoffs/2026-06-04-RESUME-rev244-port-b3.md`)

## Goal

Port the engine-core slice of the 225→244 Engine-TS delta: `World.ts`
(262/232), the entity family, the new in-engine `OnDemand.ts` (+123),
`InputTrackingBlob.ts`, the CrcTable/PreloadedPacks rewiring deferred from B1,
the `web.ts` delivery delta, the MidiPack registry, and the world-side login
rate-limit removal. All TS citations refer to the 244 pin `9aadcec4`.

## Scope slice (extraction commands)

```
git -C ../Engine-TS diff e1dea19f..9aadcec4 -- \
  src/engine src/server/tcp/TcpServer.ts src/web.ts src/app.ts \
  src/cache/CrcTable.ts src/cache/PreloadedPacks.ts \
  ':!src/engine/script'
```

(`src/engine/script/**` is B4 surface; `src/server/{login,friend,logger,worker}`
is B5 surface per the worker eval.) Plus the `MidiPack` registry slice of
`tools/pack/PackFile.ts:206` (engine-imported at the pin by Player.ts) and the
goscape-side `login_ratelimit` removal (TS counterpart already inside the
World.ts diff).

## User decisions (recorded 2026-06-04)

1. **244 runtime cache: defer to B6.** B3 implements all FileStream-backed
   serving (OnDemand, CrcTable, HTTP routes) against synthetic FileStream
   fixtures only. The end-to-end 244-client login smoke — and with it the
   closure of the map-delivery, midi, and B1 format windows — moves to B6,
   when the pack pipeline produces a real 244 cache. This **amends the
   umbrella's "after B2+B3 client smoke" gate**; PORTING.md gets an explicit
   amendment row. The reference-cache generation (umbrella risk #1 de-risk,
   planned during B1, never executed) is now a B6 prerequisite.
2. **Adopt `pid` wholesale.** `Player.slot`/`Slot()` → `pid`/`Pid()` and all
   call sites, following "adopt 244 names on renames". Supersedes B2's
   narrow HintArrow keep-slot decision row (closure note required).

## Design

### 1. Foundation — PlayerList + pid rename (lands first)

New PlayerList type in `modules/world` mirroring TS
`EntityList`/`PlayerList` (EntityList.ts:6-113):

- `ids [2048]int32` (pid → storage index, −1 = free), storage slice, free
  set, `lastUsedIndex`; `Set`/`Get`/`Remove`/`Count`; iteration **in pid
  order** (EntityList.ts:37-48 iterates `ids` by id).
- `next(priority, start)` (base EntityList.ts:22-35; PlayerList override
  EntityList.ts:95-113):
  priority path scans the 100-wide window `[start, start+100)` with the
  `start==0 → init=1` quirk; fallback round-robin from `lastUsedIndex+1`,
  wrapping at `indexPadding=1`; exhaustion = error (TS throw) → WORLD_FULL.
- `getNextPid(remoteAddr)` (World.ts:1758-1773): IPv4 start
  `(last octet % 20) * 100`; IPv6 `(hextets[2] hex % 20) * 100`; no client →
  plain `next()`.

This **replaces both** `players [2048]*Player` and `playerLoop []*Player`
(modules/world/server.go:100/117), **closing `PORTING-EXCEPTION
(gap-db-datastruct-4)`** with a faithful port. Per-tick player processing
order changes from IP-bucket insertion order to pid order — pinned by test.
`npcs [8192]*Npc` stays (244 leaves NpcList semantics unchanged).

Then the mechanical rename: `slot`→`pid` across Player, faceEntity
composition (PathingEntity.ts:531-540), Zone LocMerge/ObjReveal
(Zone.ts:268,321), rsbuf bridge calls, hintPlayer (Player.ts:2181). Own
commit, before any behavioral change.

### 2. Entity family

**Player.ts:**

- Overlay plumbing: `overlay`/`lastOverlay` fields (Player.ts:358-359);
  `openOverlay(com)` with `clearComListeners(overlay)` when `com == -1`
  (Player.ts:1954-1964); NetworkPlayer modal-flush writes IF_OPENOVERLAY on
  change (NetworkPlayer.ts:192-195). B2 shipped encoder/table row; **B4**
  wires the script op; B3 ships entity state + flush only.
- Modal-open re-shape: adopt renames `openChatModal→openChat`
  (Player.ts:1967), `openMainSideModal→openMainModalSide` (Player.ts:2009);
  **remove the "clear old suspended scripts" blocks at all 4 modal-open
  sites** (deleted upstream) — suspended COUNTDIALOG/PAUSEBUTTON scripts now
  survive modal opens. Pin test.
- Run-energy recovery `agility/9 → agility/6` (Player.ts:692).
- `setAnim` priority: `>=` replaces `> || priority === 0` — **BOTH forks**
  (Player.ts:1857, Npc.ts:461; shared-base-class rule). Pin test.
- Combat-level appearance rebuild → `buildAppearance(InvType.WORN)` (2
  sites: Player.ts:1823, 1841-1843); `cleanup()` adds `appearanceInv = -1`
  (Player.ts:471).
- Queue-cursor save/restore around `executeScript` in
  processQueues/processWeakQueue (Player.ts:892-906,910-919): TS LinkList
  has a single shared cursor; B4's getqueue/clearqueue clobber the outer
  loop mid-script. **Verify goscape's linklist iteration mechanics first**:
  port the guard if cursor-shared, else NO-OP row (Go iterator-local =
  never had the bug).
- onLogin masks-resync removal (225's `masks |= entitymask` /
  `masks |= APPEARANCE` lines deleted at 244).
- UpdateUid192 on the **reconnect** path (Player.ts:501): verify whether
  B2's producer wiring (`010ee146`) covered onReconnect; apply only if
  missing (no-double-apply).
- `account_id` field (Player.ts:306, default −1), sourced from
  `PlayerLoginResponse.account_id` (proto/login/login.proto:53 — already
  present, no B5 dependency); `addSessionLog`/`addWealthEvent` re-keys
  (Player.ts:633-642); NetworkPlayer overrides pass `client.uuid` vs
  `'headless'`/`'disconnected'` (NetworkPlayer.ts:252-263).
- playSong/playJingle → MidiPack-id producers (§8).

**NetworkPlayer.ts:** ctor drops `session` init (NetworkPlayer.ts:51-54);
`writeInner` null-client guard (NetworkPlayer.ts:200-203); length
placeholder `p1(0)`/`p2(0)` style (expected wire-identical NO-OP — verify).

**Npc.ts:**

- Regen rework (Npc.ts:513-530): `regenInterval` field deleted; countdown
  `regenClock`; proc on `regenrate != 0 && --regenClock <= 0`; reset to
  `type.regenrate`. NPCs regen on their **first turn alive** (clock init
  0). Pin test.
- `huntAll()` no-arg, deriving `HuntType.get(this.huntMode)` internally
  (Npc.ts:249-252); World caller updated (World.ts:613).
- `spawnTriggerPending` (Npc.ts:67): **NOT-PORTED, dead-at-pin** (zero
  consumers; B1 DoublyLinkList precedent).

**PathingEntity.ts:** faceEntity pid composition (rename); hitmark fields
were **B2** (`2afa543c`) — untouched; `validateDistanceWalked`
`return null → -1` (PathingEntity.ts:654) — expected NO-OP, verify.

**GameMap.ts:** npc/obj construction hoisted **above** the members gate
(GameMap.ts:127-133, 149-155) — `getNextNid()` consumed even for skipped
members-only NPCs → nid-sequence parity on non-members worlds. Pin test.

**Zone.ts:** DoublyLinkList→LinkList swap = structural NO-OP for Go
(pkg/zone keeps its list; `unlink2→unlink` is TS-internal); pid renames in
`mergeLoc`/`revealObj` covered by §1.

### 3. World.ts tick/login deltas

- All `playerLoop.all()` walks → PlayerList pid-order iteration (~15 sites:
  processInput, processPlayers, processLogouts, processInfo, processOut,
  processCleanup, processShutdown, savePlayers, rebootTimer, broadcastMes,
  getPlayerByUsername/Hash64, COORDLOGRATE check-in, temp-inv reload,
  removal sweep).
- AFK chance values **doubled and inlined**: `0.0833` normal / `0.1666`
  afk-zone (World.ts:631-636; 225 had 1/24 and 1/12). Pin constants.
- `getTotalPlayers()` = `players.count` (World.ts:1737-1739);
  `scaleByPlayerCount` unchanged (moved).
- Reconnect flow: drop `other.session = other.client.uuid` (field deleted);
  `rsbuf.cleanupPlayerBuildArea(other.pid)` (World.ts:874-880).
- Login replies: `staffModLevel >= 2` → byte **19** (new), `>= 1` → 18,
  else 2 (World.ts:943-949).
- World-side `addSessionLog(event_type, account_id, session_uuid, coord,
  …)` (World.ts:2250-2261); wealth dedup key re-keyed to
  `account_id`/`recipient_id` (World.ts:2276-2284).
- `savePlayers`: per-player autosave unchanged; **`world_heartbeat`
  message → B5 tracker row** (gRPC proto surface; World.ts:1252-1275 cited
  in the row). The `typeof self` guard is NOT-PORTED (browser-only).
- `World.addPlayer()` (World.ts:1607-1610): **NOT-PORTED, dead-at-pin**
  (zero callers at `9aadcec4` across src/tools).

### 4. Login handshake re-shape (modules/world)

TS World.ts:2115-2245 + TcpServer.ts:21-37.

- **Delete the connect-time seed send** (server.go:883-896; TS removed it
  from TcpServer.ts:21-26 and web.ts WS-open).
- New `ClientStateOndemand` (TS `client.state = 2`).
- `handleLogin` dispatch:
  - **op 14** (1-byte payload, the loginServer byte, discarded): reply
    8 zero bytes, then byte `0`, then a fresh 8-byte seed (first word
    24-bit-masked, second full 32-bit) — World.ts:2146-2156.
  - **op 15** (0-byte payload): state → Ondemand; reply 8 zero bytes —
    World.ts:2240-2242. Subsequent reads route to OnDemand (§5), mirroring
    TcpServer.ts:33-37's `state !== 0` branch.
  - **op 16/18**: framing unchanged (1-byte length); goscape already
    validates the plaintext revision byte pre-RSA → reply 6. Expected
    no-change — verify against World.ts:2157-2162.
  - default: terminate (World.ts:2243-2244).
- **Rate-limit removal** (TS deleted `loginAddressAttempts`/
  `loginDeviceAttempts` TTLCaches + both gates +
  `NODE_RATELIMIT_ADDRESS_LOGIN`/`NODE_RATELIMIT_DEVICE_LOGIN`): delete
  the gates in server.go, the ttlcache fields (server.go:91-98,414-415),
  the `world.node_ratelimit_*` config fields, their tests, and
  `pkg/util/ttlcache` (sole consumer is server.go). **Tracker row → B5**
  (login-server 3-in-5s + hop-timer replacement, per the worker eval §5):
  the protection gap is explicit and accepted on a dev branch.

### 5. In-engine OnDemand (modules/world, new component)

TS OnDemand.ts:1-123 (new at 244).

- Struct: one `filestream.FileStream` (B1 port) opened on
  `cfg.CachePath`; three FIFO queues (urgent/extra/ingame); queue mutex.
- Request path (conn read goroutine, state==Ondemand): parse 4-byte
  requests — `archive g1, file g2, priority g1`; `archive > 3 ||
  priority > 2` → close (OnDemand.ts:42-85); enqueue by priority
  (2=urgent, 1=extra, 0=ingame).
- Cycle: 50ms ticker goroutine, started with the world service, stopped at
  shutdown (TS `setTimeout(50)` self-rescheduling ≈ `time.Ticker` —
  decision note). Drains urgent→extra→ingame FIFO; `send()` =
  `cache.Read(archive+1, file)` → 500-byte chunks, 6-byte header each
  (`p1 archive, p2 file, p2 totalLen, p1 part`), or a 6-byte zero-length
  rejection frame when missing (OnDemand.ts:87-120).
- Concurrency: FileStream behind its own mutex (B1 port is documented
  not-concurrency-safe; cycle sends + `MakeCRCs` both read it); state-2
  connections have a single writer (the cycle goroutine) after the op-15
  reply; reuse the existing off-conn-goroutine write mechanism the tick
  loop already uses for game packets.

### 6. CrcTable + PreloadedPacks (pkg/cache; B1-deferred)

- `MakeCRCs` switches from loose-file loop to FileStream:
  `Count(0)` entries, `Read(0, i)` each, `p4(crc)` (or `p4(0)` when
  missing) — CrcTable.ts:12-27. The NAI-215 atomic-snapshot pattern stays.
  TS's module-init guard (`fs.existsSync('data/pack/client/')`,
  CrcTable.ts:29-33) maps to goscape's existing world-start + `::reload`
  call sites — decision row.
- **Delete `pkg/cache/preloaded.go`** (PreloadedPacks.ts deleted upstream).
  Consumer survey first; goscape-carried routes with no 244 analog (e.g.
  the ondemand maps/`.mid` paths) are re-verified against their original
  225 citations before removal — anything 225-faithful that 244 removed
  goes; anything goscape-specific gets its own decision row.

### 7. HTTP delivery (modules/ondemand; web.ts 54/27)

- Asset routes switch to FileStream reads: `/title`→(0,1),
  `/config`→(0,2), `/interface`→(0,3), `/media`→(0,4),
  **`/versionlist`→(0,5) — NEW, replaces `/models`**, `/textures`→(0,6),
  `/wordenc`→(0,7), `/sounds`→(0,8) (web.ts:63-84). `/crc` keeps serving
  the CRC snapshot bytes.
- **`.mid` route removed** (web.ts:63-69 deleted; midi flows over TCP
  OnDemand archive 3). Arc-31 M28's `.mid` 404 handling goes with it.
- `/ondemand.zip` + `/build` new static file routes (web.ts:78-81).
- The module opens its **own read-only FileStream** (+mutex) — it can run
  as a separate process under `--target ondemand`; no cross-module
  FileStream sharing.
- rs2.cgi template gains `per_deployment_token` (B1 `pkg/util/pemtoken`)
  behind a new config gate (TS `WEB_SOCKET_TOKEN_PROTECTION`,
  web.ts:101-104).
- WS routing: `state == 2` → OnDemand data only under a new
  `NODE_WS_ONDEMAND`-equivalent config, else terminate (web.ts:165-176);
  WS connect-time seed send removed (web.ts:152-158 deleted).
- **PORTING-EXCEPTION (planned):** TS 244 left the WS origin check
  commented out mid-refactor (web.ts:125-152 TODO block). goscape **keeps**
  its origin check — matching a TODO-commented upstream regression is
  wrong; security posture wins. Marker + row.

### 8. MidiPack registry (closes rev244-b2-midi-window code-side)

TS engine imports `MidiPack` (PackFile.ts:206; name→id from
`${BUILD_SRC_DIR}/pack/midi.pack`). Go: world loads
`<cfg.ContentPath>/pack/midi.pack` into a name→id map at start;
`midiIDByName` (modules/world/midi_encoders.go:35, currently stubbed −1)
consults it. Producers per Player.ts:1919-1933: songs normalize
`lower → spaces→'_' → strip [^a-z0-9_-]`; jingles just lowercase;
`id != -1` guard; `MidiSong(id)` / `MidiJingle(id, delay)` (B2 encoders).
Absent midi.pack → empty registry → −1 → silent no-op (TS posture).
The `PORTING-EXCEPTION (rev244-b2-midi-window)` marker is removed; the
closure row notes **live verification rides B6** (user decision §1).

### 9. Tracking / account_id types

- `InputTrackingBlob` (InputTrackingBlob.ts:1-11): Go struct
  `{Seq int, Data string /*base64*/, Coord int}`.
- `InputTracking.record` wraps raw blobs:
  `InputTrackingBlob(raw, len(recorded)+1, player.coord)`
  (InputTracking.ts:132-136); `recordedBlobs []InputTrackingBlob`; submit
  passes **all** blobs + `username` + `session_uuid` (`client.uuid` or
  `'headless'`) — InputTracking.ts:141-149, World.ts:2343-2352 (225 sent
  only `recordedBlobs[0]`).
- SessionLog gains `account_id` (SessionLog.ts:1-2); WealthEvent re-keyed
  `account_id`/`account_session`/`recipient_id` (WealthEvent.ts:10-22).
- The logger bridge stays **dormant**: adapters at the seam keep
  `proto/events/v1` shapes unchanged and compiling; message-shape
  consumption is B5/private-sibling (tracker row; worker eval §4).

### 10. buildArea.clear + residual NO-OP batch

- buildArea.clear: identical call sites at BOTH pins (`cleanup()` →
  `clear(false)`, Player.ts:452; `onReconnect` → `clear(true)`,
  Player.ts:541 — a **no-op by TS definition**, BuildArea.ts:23-29). So
  this is a pre-existing rev-225 gap: wire `cleanup()` → `clear(false)`
  (drive-by row citing the B3 handoff flag); the reconnect side needs no
  code — fix the `login_resync.go:54-57` "(c)" comment citation instead.
- Residual NO-OP verifications (one leaf task; decision rows): writeInner
  placeholder style; `getInventoryFromListener` tightening
  (Player.ts:1413-1420); `validateDistanceWalked` −1; `processHuntFollow`
  undefined-check drop (Npc.ts:898); Zone list swap; GameMap CSV-split
  rewrite (GameMap.ts:55-60); `app.ts` worker/exception deltas (dskit owns
  lifecycle); LinkList `.all()`→`head()/next()` style; shop-restock
  `item?.id` optional chaining (World.ts:1193-1210).

## Decision-row taxonomy (PORTING.md §B3 audit trail, established up front)

- **NOT-PORTED, dead-at-pin:** `World.addPlayer`, `Npc.spawnTriggerPending`.
- **NOT-PORTED, platform-inapplicable:** all `STANDALONE_BUNDLE` branches,
  `WorkerFactory`/`createWorker`, `savePlayers` `typeof self` guard
  (worker-eval verdict).
- **NO-OP:** import-path moves; Linkable base swap; Zone list swap;
  LinkList iteration style; CSV split; `item?.id`; app.ts lifecycle deltas;
  (pending verification) writeInner placeholders, getInventoryFromListener,
  validateDistanceWalked, hunt undefined-check.
- **B2-shipped, NOT double-applied (audit-listed):** damage2 entity hunks +
  tick.go compute feed (`2afa543c`); UpdateUid192 encoder + members
  derivation (`010ee146`; reconnect-path producer verified here);
  LastLoginInfo warn flag; IF_OPENOVERLAY table row/op (`0ef495fb`).
- **Deferred:** `world_heartbeat` → B5; `messageCount` real query → B5;
  friends `public_message` re-key + logger `report`/`input_track` shapes →
  B5/private-sibling; `BUILD_STARTUP_UPDATE`/`updateCompiler` +
  `packAll(modelFlags)` → B6; live verification of OnDemand/CrcTable/HTTP/
  midi serving → B6 (user decision).
- **Superseded:** B2's HintArrow keep-slot row (pid adopted wholesale).

## Tracker rows to create

1. Login rate limiting absent world-side; replacement (3-in-5s + hop timer)
   lands B5 — protection gap explicit.
2. `world_heartbeat` producer deferred to B5 (proto change).
3. Map-delivery + midi + B1 format windows now ALL close at **B6** (client
   smoke moved; umbrella gate amendment).
4. 244 reference-cache generation = B6 prerequisite (missed B1 de-risk).
5. Logger/friends message-shape consumption = B5/private-sibling.

## Testing

TDD pins per behavioral delta: pid allocation (IPv4/IPv6 start derivation,
priority window incl. `start==0` quirk, round-robin fallback, world-full);
pid-order tick iteration; NPC first-turn regen; setAnim `>=` both forks;
run-energy `/6`; suspended-script survival across modal opens; AFK
constants; nid-hoist sequence; handshake byte-exactness (op 14 →
`8×00, 00, seed[8]`; op 15 → `8×00` + state transition); OnDemand request
parse + reject-close + 500-byte chunk boundaries (0/1/500/501/1000-byte
payloads, header fields, rejection frame); CrcTable-from-FileStream; HTTP
routes against fixture cache; midi registry + producers; overlay flush;
account_id re-keys; queue-cursor guard (if ported).

Fixtures: synthetic FileStream caches written via B1 `FileStream.Write`
into temp dirs. Fixture-backed tests must remain valid against the real 244
cache (B6 re-runs them unchanged).

## Gates & process

B1/B2-proven cycle: plan via writing-plans (bite-sized TDD, exact TS
extraction commands as contracts) → subagent-driven execution (implementer
sonnet → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks) → full-suite gate + PORTING.md §B3
correspondence audit (every scope-diff file → commit/decision row) → final
whole-bundle integration review.

Implementer-prompt mandates (recurring B2 defects): citations verified
against `cat -n` listings BEFORE writing; reject-path tests seed
earlier-gate prerequisites; final-review "missing X" findings verified
before fixing.

Gates: `CGO_ENABLED=0 go build -trimpath ./...`; `go vet ./...`; full
`go test ./... -count=1` (real exit codes); `-race` (CGO_ENABLED=1) on
modules/world + modules/ondemand + pkg/cache (+ pkg/zone if touched).
Commits on `rev-244` only, `--no-gpg-sign`; subagents warned about phantom
`??` dotfiles (never `git add -A`).

## Risks

1. **PlayerList ordering regressions** — pid-order processing touches every
   per-tick loop; mitigated by landing first + iteration-order pin +
   full-suite gate.
2. **OnDemand concurrency** — first multi-goroutine writer surface for
   state-2 conns; mitigated by single-writer design + `-race` gate.
3. **Hidden PreloadedPacks consumers** — consumer survey before deletion;
   routes re-verified against 225 citations.
4. **Deferred-smoke blind spot** — handshake/OnDemand byte-level mistakes
   surface only at B6; mitigated by byte-exact pins now, accepted by user
   decision §1.
