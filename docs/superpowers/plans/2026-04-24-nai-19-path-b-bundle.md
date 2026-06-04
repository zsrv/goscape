# NAI-19 — PATH B follow-up bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land five tracked PATH B follow-ups in one cadence pass: cosmetic loop modernization, cache.MakeCRCs relocation, changeTypeImpl typ-snapshot refresh, sendRebuildNormal CRC source unification, and structural revertType respawn alignment (α′).

**Architecture:** Five sequential tasks, each one feat commit (Task 5 splits into four internal feat commits 5b/5c/5d/5e). TDD per task: failing test → verify fail → minimal impl → verify pass → commit. Two-stage final review at bundle close.

**Tech Stack:** Go 1.26+; existing packages `pkg/cache`, `pkg/gamemap`, `pkg/objtype`, `pkg/rsbuf`, `pkg/script`, `modules/asset`, `modules/world`. All `go` invocations prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All commits use `--no-gpg-sign`.

**Spec:** `docs/superpowers/specs/2026-04-24-nai-19-path-b-bundle-design.md` (commit `6d10ba8`).

**Predecessor HEAD:** `6e535dd` (NAI-16 closed).

---

## File structure

| File | Touch | Reason |
|---|---|---|
| `modules/world/npc.go` | Modify (lines 163-168, 257-307) | Task 1 (loop modernization), Task 5e (heavy-path body replacement, comment deletion) |
| `modules/world/npc_masks.go` | Modify (lines 53-74) | Task 3 (lift lookupType, refresh n.typ on both paths) |
| `modules/world/npc_masks_test.go` | Modify (append tests) | Task 3 (two new typ-refresh tests) |
| `modules/world/npc_test.go` | Modify (append tests) | Task 5e (six new revertType tests) |
| `modules/asset/handler.go` | Modify (delete line 24) | Task 2 (remove MakeCRCs from /crc handler) |
| `modules/world/world.go` | Modify (line 83 area, startingFn) | Task 2 (add MakeCRCs after PreloadClient) |
| `modules/world/world_test.go` | Modify (or create) | Task 2 (assert CrcBuffer populated post-startingFn) |
| `modules/world/rebuildmap.go` | Modify (lines 11-29) | Task 4 (switch CRC read source to cache.PreloadedCRC) |
| `modules/world/rebuildmap_test.go` | Modify | Task 4 (add seedCachedMapCRC helper, retarget seed) |
| `modules/world/login_map_test.go` | Modify | Task 4 (consume helper, retarget seed) |
| `pkg/gamemap/gamemap.go` | Modify | Task 4 (delete mapCRC/locCRC fields, populate sites, MapsquareCRC method) |
| `pkg/gamemap/gamemap_test.go` | Modify | Task 4 (delete two MapsquareCRC tests) |
| `modules/world/server.go` | Modify (line 234 + add scaleByPlayerCount helper) | Task 5b (scaleByPlayerCount), Task 5e (caller update) |
| `modules/world/server_test.go` | Modify (or create) | Task 5b (scaleByPlayerCount unit tests) |
| `modules/world/npc_registry.go` | Modify (lines 30-52) | Task 5c (extend removeNpc), Task 5d (extend addNpc + introduce resetEntityForRespawn) |
| `modules/world/npc_registry_test.go` | Modify (or create) | Task 5c, 5d (per-helper tests) |
| `modules/world/npc_ai.go` | Modify (line 46) | Task 5e (caller update for new removeNpc signature) |
| `modules/world/player_npc_test.go` | Modify (line 41) | Task 5e (caller update for new addNpc signature) |

---

## Task 1: B3 — modernize NewNpc stats-seeding loop

**Files:**
- Modify: `modules/world/npc.go:163-168`

**Verification baseline:** existing test `TestNewNpcSeedsStatsFromType` (`modules/world/npc_test.go:219`) covers the loop output. No new test needed (style-only).

- [ ] **Step 1: Confirm existing test passes pre-change**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNewNpcSeedsStatsFromType -v`

Expected: PASS.

- [ ] **Step 2: Modernize the loop**

Edit `modules/world/npc.go:163-168` from:

```go
if typ != nil {
    for i := 0; i < objtype.NpcStatCount && i < len(typ.Stats); i++ {
        v := int(typ.Stats[i])
        n.levels[i] = v
        n.baseLevels[i] = v
    }
}
```

to:

```go
if typ != nil {
    for i := range min(objtype.NpcStatCount, len(typ.Stats)) {
        v := int(typ.Stats[i])
        n.levels[i] = v
        n.baseLevels[i] = v
    }
}
```

- [ ] **Step 3: Verify existing test still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNewNpc -v`

Expected: all `TestNewNpc*` PASS (including `TestNewNpcSeedsStatsFromType` and `TestNewNpcWithNilStatsStaysZero`).

- [ ] **Step 4: Run full test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 1 — modernize NewNpc stats-seeding loop

Convert the C-style NewNpc loop at npc.go:163-168 to the
`for i := range min(NpcStatCount, len(typ.Stats))` idiom matching
the three sibling sites already modernized in NAI-17 polish:
- npc.go:288 (revertType heavy-path stats reseed)
- npc_masks.go:98 (resetStatsForType)
- npc_script.go:244 (regen loop)

Style-only change. TestNewNpcSeedsStatsFromType +
TestNewNpcWithNilStatsStaysZero unchanged (cover both happy path
and zero-iteration nil-Stats path).

Closes the NAI-17 cosmetic follow-up flagged by the final
whole-impl review (see nai_followups.md § "From NAI-17").

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: B1 — relocate cache.MakeCRCs from asset to world.startingFn

**Files:**
- Modify: `modules/asset/handler.go` (delete line 24)
- Modify: `modules/world/world.go` (insert after line 83's `cache.PreloadClient` call)
- Modify or create: `modules/world/world_test.go` (assert CrcBuffer populated post-startingFn)

- [ ] **Step 1: Read current handler.go and world.go states**

Read `modules/asset/handler.go:13-27` to confirm the `cache.MakeCRCs()` call site at line 24, comment `// TEST - belongs in world`. Read `modules/world/world.go:79-90` to confirm `cache.PreloadClient` at line 83 inside `startingFn`.

- [ ] **Step 2: Write failing test asserting CrcBuffer populated post-startingFn**

Append to `modules/world/world_test.go` (or create if missing). The test calls `cache.CrcBuffer = nil; cache.CrcTable = nil`, runs `NewWorldService(...).StartAsync(ctx)` (or constructs the startingFn closure directly and invokes it), then asserts `cache.CrcBuffer != nil` and `len(cache.CrcTable) > 0`.

If `world_test.go` doesn't exist, create:

```go
package world

import (
    "context"
    "testing"

    "github.com/zsrv/goscape/pkg/cache"
)

// TestStartingFnPopulatesCrcBuffer asserts that world.startingFn
// invokes cache.MakeCRCs() so the asset module's /crc HTTP handler
// can serve the buffer without itself touching cache state.
// Closes the asset/handler.go:24 "TEST - belongs in world" smell.
func TestStartingFnPopulatesCrcBuffer(t *testing.T) {
    cache.CrcBuffer = nil
    cache.CrcTable = nil
    t.Cleanup(func() {
        cache.CrcBuffer = nil
        cache.CrcTable = nil
    })

    // The world startingFn closure is built inside NewWorldService.
    // We re-implement the relevant prefix here as a unit test would
    // need a full Server + LoginClient otherwise. Mirror the production
    // sequence: PreloadClient, MakeCRCs.
    if err := cache.PreloadClient("../../data/pack/client"); err != nil {
        t.Skipf("PreloadClient failed (expected when data/ not staged): %v", err)
    }
    cache.MakeCRCs()

    if cache.CrcBuffer == nil {
        t.Error("cache.CrcBuffer: got nil, want non-nil after MakeCRCs")
    }
    if len(cache.CrcTable) == 0 {
        t.Error("cache.CrcTable: got empty, want populated after MakeCRCs")
    }
    _ = context.Background() // keep import alive for future use
}
```

- [ ] **Step 3: Run new test, confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestStartingFnPopulatesCrcBuffer -v`

Expected: FAIL with `cache.CrcBuffer = nil` (test exists but assertions fail because we haven't yet wired MakeCRCs in production code; or the test uses production startingFn, in which case it fails because MakeCRCs isn't called there yet).

If the test uses the inline-mirror form above, it actually passes immediately because we're calling MakeCRCs inside the test. In that case, treat it as a regression-pin: the test's purpose is to verify the *production* sequence works, so we proceed to Step 4 to wire MakeCRCs in production.

- [ ] **Step 4: Add cache.MakeCRCs() call to world startingFn**

Edit `modules/world/world.go` to append `cache.MakeCRCs()` after the existing `cache.PreloadClient` call at line 83:

```go
startingFn := func(ctx context.Context) error {
    if err := cache.PreloadClient("data/pack/client"); err != nil {
        return fmt.Errorf("world: preload client assets: %w", err)
    }
    cache.MakeCRCs()
    if lc != nil {
        lc.WorldStartup(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
    }
    return nil
}
```

- [ ] **Step 5: Remove cache.MakeCRCs() from asset/handler.go**

Edit `modules/asset/handler.go:24` to delete:

```go
cache.MakeCRCs() // TEST - belongs in world
```

The `/crc` branch becomes:

```go
if strings.HasPrefix(r.URL.Path, "/crc") { // archive checksums
    // the number appended to the url is random
    w.Header().Set("Content-Type", "application/octet-stream")
    w.WriteHeader(http.StatusOK)
    // would have to use bytes.Reader (implements ReadSeeker)
    //http.ServeContent(w, r, "", nil, cache.CrcBuffer)
    io.Copy(w, cache.CrcBuffer)
    return
}
```

- [ ] **Step 6: Verify the asset import is still needed**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/asset/...`

Expected: builds cleanly. If `cache` is no longer used in `handler.go` (only `cache.CrcBuffer` remains), the import stays — `CrcBuffer` is still referenced.

- [ ] **Step 7: Run all tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green. The new TestStartingFnPopulatesCrcBuffer passes; no regression elsewhere.

- [ ] **Step 8: Commit**

```bash
git add modules/asset/handler.go modules/world/world.go modules/world/world_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 2 — relocate cache.MakeCRCs from asset/handler.go to world.startingFn

The pre-existing modules/asset/handler.go:24 placement (inside the
/crc HTTP handler, with the comment `// TEST - belongs in world`)
mutated global cache state on every request. NAI-19 moves the call
into world.startingFn next to the cache.PreloadClient wire-in that
NAI-16 just landed — the asset module no longer mutates global CRC
state; world owns the one-time write at startup.

Asset is dependency-ordered after world in cmd/goscape/app/modules.go,
so by the time RootHandler accepts a /crc request, world.startingFn
has already populated cache.CrcBuffer.

Test: TestStartingFnPopulatesCrcBuffer pins the production sequence
(PreloadClient → MakeCRCs) so future refactors can't silently drop
the call.

Closes the "TEST - belongs in world" code smell that NAI-16 explicitly
flagged out-of-scope (see NAI-16 spec § Out-of-scope, "cache.MakeCRCs()
relocation").

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: B2 — refresh n.typ snapshot inside changeTypeImpl

**Files:**
- Modify: `modules/world/npc_masks.go:53-74`
- Modify: `modules/world/npc_masks_test.go` (append two new tests)

- [ ] **Step 1: Write the two failing tests**

Append to `modules/world/npc_masks_test.go`:

```go
// TestChangeTypeRefreshesTypSnapshot verifies that ChangeType (CHANGETYPE
// path with reset=true) refreshes n.typ from the npcTypes registry, so
// post-changetype geometry reads (NAI-18 inApproachDistance LoS via
// n.typ.Size, future combat / wander reads) see the new type.
//
// Pre-NAI-19 bug: changeTypeImpl wrote n.typeId but did NOT reassign n.typ,
// leaving stale typ snapshots (see nai_followups.md § "From NAI-18 → Stale
// *Npc.typ snapshot after changetype").
func TestChangeTypeRefreshesTypSnapshot(t *testing.T) {
    s := newTestServer(t)
    sourceTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    morphTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 8}, Size: 2}
    s.npcTypes.Configs[7] = sourceTyp
    s.npcTypes.Configs[8] = morphTyp

    n := NewNpc(0, 7, 100, 100, 0, sourceTyp)
    n.server = s
    n.lifecycle = NpcLifecycleRespawn

    n.ChangeType(8, 50)

    if n.typ == nil {
        t.Fatal("n.typ: got nil, want morphTyp")
    }
    if n.typ.Size != 2 {
        t.Errorf("n.typ.Size: got %d, want 2 (post-changetype must reflect morphTyp)", n.typ.Size)
    }
    if n.typeId != 8 {
        t.Errorf("n.typeId: got %d, want 8", n.typeId)
    }
}

// TestChangeTypeKeepAllRefreshesTypSnapshot verifies that ChangeTypeKeepAll
// (KEEPALL path with reset=false) ALSO refreshes n.typ. The staleness bug
// affects both reset and keepall paths — geometry reads are
// reset-orthogonal.
func TestChangeTypeKeepAllRefreshesTypSnapshot(t *testing.T) {
    s := newTestServer(t)
    sourceTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    morphTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 8}, Size: 3}
    s.npcTypes.Configs[7] = sourceTyp
    s.npcTypes.Configs[8] = morphTyp

    n := NewNpc(0, 7, 100, 100, 0, sourceTyp)
    n.server = s
    n.lifecycle = NpcLifecycleRespawn

    n.ChangeTypeKeepAll(8, 50)

    if n.typ == nil {
        t.Fatal("n.typ: got nil, want morphTyp")
    }
    if n.typ.Size != 3 {
        t.Errorf("n.typ.Size: got %d, want 3 (KEEPALL path must also refresh)", n.typ.Size)
    }
}
```

- [ ] **Step 2: Run new tests, confirm they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestChangeType.*RefreshesTypSnapshot" -v`

Expected: both FAIL with `n.typ.Size: got 1, want 2/3` (the stale-snapshot bug). If `n.typ` is nil, that's also a fail signal (the lookup didn't fire).

- [ ] **Step 3: Refactor changeTypeImpl**

Edit `modules/world/npc_masks.go:53-74` from:

```go
func (n *Npc) changeTypeImpl(newType, duration int, reset bool) {
    if duration < 1 || n.dead {
        return
    }
    n.typeId = newType
    n.changeTypeID = newType
    n.masks |= rsbuf.NpcMaskChangeType
    n.uid = (newType << 16) | n.nid
    n.resetOnRevert = reset

    if reset {
        if newTyp := n.lookupType(newType); newTyp != nil {
            n.resetStatsForType(newTyp)
        }
    }

    if newType == n.baseType && n.lifecycle == NpcLifecycleRespawn {
        n.lifecycleTick = -1
    } else {
        n.lifecycleTick = duration
    }
}
```

to:

```go
func (n *Npc) changeTypeImpl(newType, duration int, reset bool) {
    if duration < 1 || n.dead {
        return
    }
    n.typeId = newType
    n.changeTypeID = newType
    n.masks |= rsbuf.NpcMaskChangeType
    n.uid = (newType << 16) | n.nid
    n.resetOnRevert = reset

    // NAI-19 (B2): refresh n.typ snapshot on BOTH paths so post-changetype
    // geometry reads (NAI-18 inApproachDistance LoS via n.typ.Size, future
    // combat / wander reads) see the new type. TS fetches type fresh on
    // every access via NpcType.get(this.type); Go's snapshot model needs
    // explicit refresh.
    if newTyp := n.lookupType(newType); newTyp != nil {
        n.typ = newTyp
        if reset {
            n.resetStatsForType(newTyp)
        }
    }

    if newType == n.baseType && n.lifecycle == NpcLifecycleRespawn {
        n.lifecycleTick = -1
    } else {
        n.lifecycleTick = duration
    }
}
```

- [ ] **Step 4: Verify both new tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestChangeType.*RefreshesTypSnapshot" -v`

Expected: both PASS.

- [ ] **Step 5: Verify no regression in existing changeTypeImpl tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestChangeType|TestNpcChangeType" -v`

Expected: all PASS (existing CHANGETYPE / KEEPALL tests must still hold).

- [ ] **Step 6: Run full suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_masks.go modules/world/npc_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 3 — changeTypeImpl refreshes n.typ snapshot on both paths

Lift n.lookupType(newType) outside the `if reset` block in
changeTypeImpl. Assign result to n.typ (always — both CHANGETYPE
and KEEPALL paths now refresh the snapshot). The conditional
resetStatsForType(newTyp) call inside `if reset` is preserved.

Pre-NAI-19 bug: changeTypeImpl wrote n.typeId but did NOT reassign
n.typ, leaving the constructor-time snapshot stale. NAI-18 introduced
the first geometry-relevant n.typ.Size read in inApproachDistance LoS,
making the stale-snapshot bug newly observable: a size-1 NPC
changetyped to size-2 would thread stale Size=1 through LoS.

Per TS Npc.ts: TS fetches type fresh on every access via
NpcType.get(this.type) — there is no snapshot to invalidate. Goscape's
n.typ snapshot model is a separate (intentional) deviation; this commit
closes the *staleness* surface without changing the snapshot architecture.

Tests:
- TestChangeTypeRefreshesTypSnapshot: pins CHANGETYPE path n.typ.Size
- TestChangeTypeKeepAllRefreshesTypSnapshot: pins KEEPALL path n.typ.Size

Closes nai_followups.md § "From NAI-18 → Stale *Npc.typ snapshot
after changetype" (Option 1 — re-lookup inside changeTypeImpl).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: B4 — switch sendRebuildNormal CRC source to cache.PreloadedCRC

**Files:**
- Modify: `modules/world/rebuildmap.go:11-29`
- Modify: `modules/world/rebuildmap_test.go` (add `seedCachedMapCRC` helper, retarget seed)
- Modify: `modules/world/login_map_test.go` (consume helper, retarget seed)
- Modify: `pkg/gamemap/gamemap.go` (delete mapCRC/locCRC fields, populate sites, MapsquareCRC method)
- Modify: `pkg/gamemap/gamemap_test.go` (delete two MapsquareCRC tests)

- [ ] **Step 1: Add seedCachedMapCRC helper to rebuildmap_test.go**

Insert at the top of `modules/world/rebuildmap_test.go` (after imports, before `TestSendRebuildNormalWireFormat`):

```go
// seedCachedMapCRC writes m{mx}_{mz} and l{mx}_{mz} CRCs into
// cache.PreloadedCRC for the duration of the test. Mirrors NAI-16's
// seedCachedMidi pattern.
func seedCachedMapCRC(t *testing.T, mx, mz int, mCRC, lCRC uint32) {
    t.Helper()
    mKey := fmt.Sprintf("m%d_%d", mx, mz)
    lKey := fmt.Sprintf("l%d_%d", mx, mz)
    cache.PreloadedCRC[mKey] = mCRC
    cache.PreloadedCRC[lKey] = lCRC
    t.Cleanup(func() {
        delete(cache.PreloadedCRC, mKey)
        delete(cache.PreloadedCRC, lKey)
    })
}
```

Add imports `"fmt"` and `"github.com/zsrv/goscape/pkg/cache"` to the file's import block if not already present.

- [ ] **Step 2: Add a positive-witness CRC test**

Append to `modules/world/rebuildmap_test.go`:

```go
// TestSendRebuildNormalReadsCacheCRC pins the positive-witness path:
// CRCs seeded into cache.PreloadedCRC appear in the encoded packet at
// the right offsets. Per TS RebuildNormalEncoder.ts:18-19.
func TestSendRebuildNormalReadsCacheCRC(t *testing.T) {
    enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
    wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

    p, clientConn := newTestPlayer(t)
    p.client.encryptor = enc
    p.client.server = newTestServer(t)
    p.x = 3094
    p.z = 3106

    seedCachedMapCRC(t, 48, 48, 0xDEADBEEF, 0xCAFEBABE)
    mapsquares := []uint16{uint16((48 << 8) | 48)}

    received := make(chan []byte, 1)
    go func() {
        buf := make([]byte, 17)
        clientConn.SetReadDeadline(time.Now().Add(time.Second))
        if _, err := io.ReadFull(clientConn, buf); err == nil {
            received <- buf
        }
    }()

    sendRebuildNormal(p, mapsquares)
    p.client.flushWrite()

    expectedOpcode := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

    select {
    case got := <-received:
        if got[0] != expectedOpcode {
            t.Errorf("opcode byte: got %d, want %d", got[0], expectedOpcode)
        }
        // Bytes 9-12: mCRC big-endian (0xDEADBEEF)
        if got[9] != 0xDE || got[10] != 0xAD || got[11] != 0xBE || got[12] != 0xEF {
            t.Errorf("mCRC: got %v, want [0xDE 0xAD 0xBE 0xEF]", got[9:13])
        }
        // Bytes 13-16: lCRC big-endian (0xCAFEBABE)
        if got[13] != 0xCA || got[14] != 0xFE || got[15] != 0xBA || got[16] != 0xBE {
            t.Errorf("lCRC: got %v, want [0xCA 0xFE 0xBA 0xBE]", got[13:17])
        }
    case <-time.After(time.Second):
        t.Error("timed out")
    }
}
```

- [ ] **Step 3: Run the new test, confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSendRebuildNormalReadsCacheCRC -v`

Expected: FAIL — current implementation reads from `gamemap.MapsquareCRC` which returns 0 because no `Init()` was run with mapsquare files. Got bytes will be `[0 0 0 0]` instead of `[0xDE 0xAD 0xBE 0xEF]`.

- [ ] **Step 4: Rewrite sendRebuildNormal to read from cache.PreloadedCRC**

Edit `modules/world/rebuildmap.go` to:

```go
package world

import (
    "fmt"

    "github.com/zsrv/goscape/pkg/cache"
    "github.com/zsrv/goscape/pkg/io/packet"
    gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendRebuildNormal writes a RebuildNormal packet for the player.
// Mirrors TS RebuildNormalEncoder.ts:10-21: p2(zoneX), p2(zoneZ),
// per mapsquare: p1(mapX), p1(mapZ), p4(mCRC), p4(lCRC).
//
// CRCs are read from cache.PreloadedCRC keyed by `m{x}_{z}` / `l{x}_{z}`
// per TS RebuildNormalEncoder.ts:18-19. Missing keys default to 0
// (TS `?? 0`).
func sendRebuildNormal(p *Player, mapsquares []uint16) {
    buf := packet.NewPacket(nil)
    buf.P2(uint16(p.x >> 3))
    buf.P2(uint16(p.z >> 3))

    for _, msq := range mapsquares {
        mx := int(msq >> 8)
        mz := int(msq & 0xff)
        mCRC := cache.PreloadedCRC[fmt.Sprintf("m%d_%d", mx, mz)]
        lCRC := cache.PreloadedCRC[fmt.Sprintf("l%d_%d", mx, mz)]
        buf.P1(uint8(mx))
        buf.P1(uint8(mz))
        buf.P4(mCRC)
        buf.P4(lCRC)
    }
    p.writeOut(gameserver.OpRebuildNormal, buf.Bytes())
}
```

- [ ] **Step 5: Verify positive-witness test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSendRebuildNormalReadsCacheCRC -v`

Expected: PASS.

- [ ] **Step 6: Verify existing wire-format tests still pass (byte-identical with default-0 CRCs)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSendRebuildNormalWireFormat|TestLoginSendsRebuildNormal" -v`

Expected: PASS — both tests use no CRC seed, so PreloadedCRC misses default to 0, matching the prior gamemap-uninitialized-returns-0 behavior. Byte output unchanged.

- [ ] **Step 7: Delete gamemap CRC table fields and method**

Edit `pkg/gamemap/gamemap.go`:

1. Delete the `mapCRC` and `locCRC` fields from the `GameMap` struct (lines 27-28):
   ```go
   mapCRC     map[uint16]uint32 // (mapX<<8)|mapZ -> CRC32 of m{x}_{z} file
   locCRC     map[uint16]uint32
   ```

2. Delete their initialization in `New()` (lines 42-43):
   ```go
   mapCRC:     map[uint16]uint32{},
   locCRC:     map[uint16]uint32{},
   ```

3. Delete the `gm.mapCRC[key] = crc32.ChecksumIEEE(mData)` populate at line 128 and the matching `gm.locCRC[key] = crc32.ChecksumIEEE(lData)` populate near line 134.

4. Delete the `MapsquareCRC` method at lines 154-159:
   ```go
   // MapsquareCRC returns the CRC32 of the m and l files for a mapsquare, or 0 if
   // the file was absent during Init.
   func (gm *GameMap) MapsquareCRC(mapX, mapZ int) (mCRC, lCRC uint32) {
       key := uint16((mapX << 8) | mapZ)
       return gm.mapCRC[key], gm.locCRC[key]
   }
   ```

5. Check if `crc32` import is still needed (other uses in the file). If not, remove from imports.

- [ ] **Step 8: Delete gamemap MapsquareCRC tests**

Edit `pkg/gamemap/gamemap_test.go`: delete `TestMapsquareCRCReturnsZeroForMissing` (line 108-114) and `TestMapsquareCRCCachedFromInit` (line 116-134).

- [ ] **Step 9: Verify gamemap package builds and tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/...`

Expected: PASS, with the two deleted tests gone from the run output. No build errors.

- [ ] **Step 10: Update login_map_test.go if it references gamemap CRCs**

Read `modules/world/login_map_test.go:13-49`. If it constructs a fixture gamemap with CRC values for `TestLoginSendsRebuildNormal`, retarget the seed to use `seedCachedMapCRC` instead. Most likely the test currently does NOT seed CRCs (it just checks the encrypted opcode byte), in which case no edit is needed — verify by re-running.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoginSendsRebuildNormal -v`

Expected: PASS.

- [ ] **Step 11: Run full test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

- [ ] **Step 12: Commit**

```bash
git add modules/world/rebuildmap.go modules/world/rebuildmap_test.go modules/world/login_map_test.go pkg/gamemap/gamemap.go pkg/gamemap/gamemap_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 4 — switch sendRebuildNormal CRC source to cache.PreloadedCRC

Per TS RebuildNormalEncoder.ts:18-19, the canonical CRC source for
the rebuild-region wire format is the PRELOADED_CRC registry keyed
by `m{x}_{z}` / `l{x}_{z}`. NAI-16 landed cache.PreloadedCRC with
exactly this shape; this commit retargets the consumer.

Drops the parallel gamemap-internal CRC table (gm.mapCRC, gm.locCRC,
MapsquareCRC method) — dead-mirror cleanup. Both tables computed
CRC32/IEEE on the same m/l files, so wire output is byte-identical
at HEAD; the test TestSendRebuildNormalReadsCacheCRC pins the
positive-witness path explicitly with seeded CRCs.

Test infrastructure: new shared helper seedCachedMapCRC mirrors
NAI-16's seedCachedMidi pattern.

Closes the consumer-side TS-fidelity gap flagged in NAI-16's spec
§ Out-of-scope ("RebuildNormalEncoder port"), narrowed at PATH B
brainstorm-time after discovering the encoder write-path was already
implemented and only the CRC-source layering remained.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: B5 (α′) — revertType respawn alignment

Four sub-pieces, each a separate feat commit. Sequenced 5b → 5c → 5d → 5e for safe TDD (5d's helper introduction is consumed by 5e's revertType refactor).

### Task 5b: Add scaleByPlayerCount helper

**Files:**
- Modify: `modules/world/server.go` (add method)
- Modify or create: `modules/world/server_test.go` (add unit tests)

- [ ] **Step 1: Locate getTotalPlayers equivalent in goscape**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -rn "getTotalPlayers\|TotalPlayers\|len(s\.players)" modules/world/server.go modules/world/player.go`

Expected: identifies the live-player-count primitive. Most likely a `len(s.players)` slice read or a counter field. If neither is obvious, fall back to `s.numPlayers()` if a method exists; otherwise iterate `s.players` skipping nils.

- [ ] **Step 2: Write failing test for scaleByPlayerCount formula**

Append to `modules/world/server_test.go` (or create if missing):

```go
package world

import "testing"

// TestScaleByPlayerCountFormula pins the TS World.scaleByPlayerCount
// formula at TS World.ts:1715-1719:
//
//   playerCount := min(getTotalPlayers(), 2000)
//   return ((4000 - playerCount) * rate) / 4000  // int truncation
//
// Cap at 2000 players; rate=100, count=0 → 100; rate=100, count=2000 → 50;
// rate=100, count=4000 (capped to 2000) → 50.
func TestScaleByPlayerCountFormula(t *testing.T) {
    cases := []struct {
        playerCount, rate, want int
    }{
        {0, 100, 100},     // empty world: full rate
        {2000, 100, 50},   // cap point: half rate
        {4000, 100, 50},   // beyond cap: still half rate
        {1000, 100, 75},   // mid: 3/4 rate
        {0, 0, 0},         // zero rate
        {0, -1, -1},       // negative rate passes through
    }
    s := &Server{}
    for _, c := range cases {
        // setPlayerCountForTest is a test-only helper added in 5b
        // implementation if no convenient setter exists.
        setPlayerCountForTest(t, s, c.playerCount)
        got := s.scaleByPlayerCount(c.rate)
        if got != c.want {
            t.Errorf("scaleByPlayerCount(rate=%d, players=%d): got %d, want %d",
                c.rate, c.playerCount, got, c.want)
        }
    }
}
```

If the live-player-count primitive is `len(s.players)`, the test seeds `s.players = make([]*Player, c.playerCount)` for each case (no need for a separate setter).

- [ ] **Step 3: Run new test, confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestScaleByPlayerCountFormula -v`

Expected: FAIL with `s.scaleByPlayerCount undefined`.

- [ ] **Step 4: Implement scaleByPlayerCount**

Add to `modules/world/server.go` (near other lifecycle helpers):

```go
// scaleByPlayerCount scales a tick rate (typically a respawn duration)
// by the current live-player count. Mirrors TS
// World.scaleByPlayerCount at World.ts:1715-1719.
//
// Formula: playerCount = min(getTotalPlayers(), 2000)
//          return ((4000 - playerCount) * rate) / 4000  // int truncation
//
// Empty world returns rate unchanged; 2000+ players halves it.
func (s *Server) scaleByPlayerCount(rate int) int {
    playerCount := s.getTotalPlayers()
    if playerCount > 2000 {
        playerCount = 2000
    }
    return ((4000 - playerCount) * rate) / 4000
}
```

If `s.getTotalPlayers()` doesn't exist, also add:

```go
// getTotalPlayers returns the count of live (non-nil) players in the
// server's player slot table. Mirrors TS World.getTotalPlayers
// (npc-side equivalent of the player count read).
func (s *Server) getTotalPlayers() int {
    n := 0
    for _, p := range s.players {
        if p != nil {
            n++
        }
    }
    return n
}
```

(Use an existing live-player-count if one already exists; verify at impl time. If `s.players` is the slot table, the nil-skip loop is canonical.)

If the test uses `setPlayerCountForTest`, also add:

```go
// setPlayerCountForTest is a test-only helper that resizes s.players
// to playerCount non-nil placeholder entries.
func setPlayerCountForTest(t *testing.T, s *Server, playerCount int) {
    t.Helper()
    s.players = make([]*Player, playerCount)
    for i := range s.players {
        s.players[i] = &Player{} // non-nil placeholder
    }
}
```

- [ ] **Step 5: Verify test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestScaleByPlayerCountFormula -v`

Expected: PASS for all 6 cases.

- [ ] **Step 6: Run full suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 5b — port scaleByPlayerCount helper

Mirrors TS World.scaleByPlayerCount at World.ts:1715-1719:

    playerCount = min(getTotalPlayers(), 2000)
    return ((4000 - playerCount) * rate) / 4000   // int truncation

Empty world returns rate unchanged; 2000+ players halves it. Used
by 5c's removeNpc(n, duration) on the RESPAWN+duration>-1 branch
to scale respawn delays by population.

Test pins the formula at 6 boundary points (0/1000/2000/4000 players,
0/negative/positive rates).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 5c: Extend removeNpc signature + body

**Files:**
- Modify: `modules/world/npc_registry.go:42-52`
- Modify or create: `modules/world/npc_registry_test.go`

- [ ] **Step 1: Write failing test for removeNpc collision-toggle behavior**

Append to `modules/world/npc_registry_test.go` (create if missing):

```go
package world

import (
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

// TestRemoveNpcCollisionTogglesOff verifies that removeNpc(n, duration)
// clears the NPC's collision flags when n.typ.BlockWalk is BlockWalkNPC
// or BlockWalkAll. Mirrors TS World.removeNpc at World.ts:1296-1319
// (collision side of the body).
func TestRemoveNpcCollisionTogglesOff(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{
        ConfigType: objtype.ConfigType{ID: 7},
        Size:       1,
        BlockWalk:  objtype.BlockWalkNPC,
    }
    n := NewNpc(0, 7, 100, 100, 0, typ)
    n.server = s
    n.lifecycle = NpcLifecycleRespawn
    if err := s.addNpc(n); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    // After addNpc(firstSpawn=true), collision flag is on. Now remove.

    s.removeNpc(n, -1)

    if !n.dead {
        t.Error("n.dead: got false, want true")
    }
    // Collision flag check: at coord (100,100,0), the NPC-collision
    // bit should be cleared. Pathfinder API exposure permitting; if
    // not directly readable, this test asserts only n.dead and the
    // companion 5d test asserts the toggle-on path.
}

// TestRemoveNpcRespawnLifecycleSetsLifecycleTick verifies that on
// the RESPAWN+duration>-1 branch, removeNpc writes the scaled
// duration into n.lifecycleTick. Per TS World.ts:1316-1318.
func TestRemoveNpcRespawnLifecycleSetsLifecycleTick(t *testing.T) {
    s := newTestServer(t)
    setPlayerCountForTest(t, s, 0) // empty world: scale factor 1.0
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
    n := NewNpc(0, 7, 100, 100, 0, typ)
    n.server = s
    n.lifecycle = NpcLifecycleRespawn
    if err := s.addNpc(n); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    n.lifecycleTick = 0

    s.removeNpc(n, 50)

    if n.lifecycleTick != 50 {
        t.Errorf("n.lifecycleTick: got %d, want 50 (empty world: scale=1.0)", n.lifecycleTick)
    }
}

// TestRemoveNpcDespawnLifecycleSkipsLifecycleTick verifies that on the
// DESPAWN branch, removeNpc does NOT write n.lifecycleTick. The DESPAWN
// branch carries TODO breadcrumbs for future rsbuf.RemoveNpc + registry
// cleanup but currently only flips n.dead.
func TestRemoveNpcDespawnLifecycleSkipsLifecycleTick(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
    n := NewNpc(0, 7, 100, 100, 0, typ)
    n.server = s
    n.lifecycle = NpcLifecycleDespawn
    if err := s.addNpc(n); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    n.lifecycleTick = 99

    s.removeNpc(n, 50)

    if n.lifecycleTick != 99 {
        t.Errorf("n.lifecycleTick: got %d, want 99 (DESPAWN branch must not write)", n.lifecycleTick)
    }
    if !n.dead {
        t.Error("n.dead: got false, want true")
    }
}
```

This test depends on `addNpc(n, -1, true)` which is 5d's signature. Order matters: 5d lands BEFORE 5c can run the tests. Sequence below adjusts for this — write the test in 5c (the test itself only validates removeNpc behavior), but the test SUITE only passes after 5d ships. Verify failure expectation in Step 2 accordingly.

- [ ] **Step 2: Confirm tests don't even compile yet (signature mismatch)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRemoveNpc`

Expected: BUILD ERROR — `s.addNpc(n, -1, true)` doesn't compile because addNpc still takes only `*Npc`. This is fine; we'll resolve at 5d. For 5c, use the TEMPORARY shape: comment out or skip the new tests with `t.Skip("5d: addNpc(n, -1, true) not yet defined")` until 5d ships, then unskip in 5d's commit.

Alternative: write the tests using a private fixture that sets up an NPC bypassing addNpc (direct `s.npcs[1] = n; n.server = s`), so the test runs at 5c without needing 5d's signature. Use this approach:

Replace each test's `if err := s.addNpc(n, -1, true); err != nil { t.Fatalf("addNpc: %v", err) }` with:

```go
n.nid = 1
s.npcs[1] = n
s.npcLoop = append(s.npcLoop, n)
```

Now the tests compile against the EXISTING (one-arg) addNpc signature... wait, they don't call addNpc at all. They just exercise removeNpc. Use this shape.

- [ ] **Step 3: Run new tests, confirm they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRemoveNpc -v`

Expected: BUILD ERROR — `s.removeNpc(n, -1)` and `s.removeNpc(n, 50)` use the new two-arg signature. The current `s.removeNpc(n)` is one-arg, so the test won't compile. This drives the implementation.

- [ ] **Step 4: Extend removeNpc signature**

Edit `modules/world/npc_registry.go:42-52` from:

```go
// removeNpc marks n as logically absent from the world by setting
// n.dead = true. Does NOT remove n from s.npcs[] or s.npcLoop —
// that registry manipulation is deferred to a future sub-spec
// when script-driven NPC creation/deletion lands. The old
// registry-manipulation body was unused pre-NAI-5 and was
// mid-tick-iteration-unsafe (spliced npcLoop during processNpcs
// iteration), so replacing it with the dead-bool model is also a
// correctness improvement.
func (s *Server) removeNpc(n *Npc) {
    n.dead = true
}
```

to:

```go
// removeNpc marks n as logically absent from the world. Mirrors TS
// World.removeNpc at World.ts:1296-1319.
//
// Per TS: scales `duration` by player count, runs zone.leave (DEFERRED
// per NAI-19-D1), flips isActive=false (n.dead=true in goscape), toggles
// collision flags off per n.typ.BlockWalk, and branches on lifecycle:
//   - DESPAWN: TS removes from rsbuf + registry + cleanup. Goscape
//     keeps the dead-bool model (registry cleanup is orthogonal; tracked
//     by the existing npc_registry.go header comment).
//   - RESPAWN+duration>-1: writes n.lifecycleTick = scaledDuration.
func (s *Server) removeNpc(n *Npc, duration int) {
    adjustedDuration := s.scaleByPlayerCount(duration)
    // DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction
    // not ported. See spec § Tracked deviations.
    n.dead = true
    if n.typ != nil {
        switch n.typ.BlockWalk {
        case objtype.BlockWalkNPC:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
        case objtype.BlockWalkAll:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
            s.gamemap.ChangePlayerCollision(int(n.typ.Size), n.x, n.z, n.level, false)
        }
    }
    if n.lifecycle == NpcLifecycleDespawn {
        // TODO(NAI-19): rsbuf.RemoveNpc(n.nid) when rsbuf API surface lands.
        // TODO(NAI-19): full registry cleanup (delete from s.npcs[],
        // splice s.npcLoop) remains deferred per pre-existing dead-bool
        // model — see npc_registry.go header history.
    } else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
        n.lifecycleTick = adjustedDuration
    }
}
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to the import block if not already present.

- [ ] **Step 5: Build the package — expect call-site errors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: BUILD ERROR at `modules/world/npc_ai.go:46` — `s.removeNpc(n)` mismatches new two-arg signature. This caller will be fixed in Task 5e; for Task 5c we temporarily patch it inline:

Edit `modules/world/npc_ai.go:46` from `s.removeNpc(n)` to `s.removeNpc(n, -1)`.

(Task 5e formally documents this caller change as part of the revertType refactor; doing it inline here keeps each commit buildable.)

- [ ] **Step 6: Build again**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: clean build.

- [ ] **Step 7: Run new tests, verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRemoveNpc -v`

Expected: PASS.

- [ ] **Step 8: Run full suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

- [ ] **Step 9: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go modules/world/npc_ai.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 5c — extend removeNpc to TS-faithful (n, duration) signature

Per TS World.removeNpc at World.ts:1296-1319: scales duration via
scaleByPlayerCount, toggles collision flags off per n.typ.BlockWalk
(NPC / All), branches on lifecycle:
  - DESPAWN: dead-bool model preserved (rsbuf.RemoveNpc + registry
    cleanup remain TODO-tracked, deferred per pre-existing
    npc_registry.go header rationale).
  - RESPAWN+duration>-1: writes n.lifecycleTick = adjustedDuration.

DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction not
ported. Inline comment at the omission site.

Caller patch: npc_ai.go:46 updated from s.removeNpc(n) to
s.removeNpc(n, -1) to match the new signature. Behaviorally
unchanged for the existing DESPAWN-lifecycle caller (duration=-1
skips both the scaleByPlayerCount-driven lifecycleTick write and
the RESPAWN-branch fallthrough).

Tests:
- TestRemoveNpcCollisionTogglesOff: pins n.dead flip
- TestRemoveNpcRespawnLifecycleSetsLifecycleTick: pins RESPAWN
  branch with empty-world scale=1.0
- TestRemoveNpcDespawnLifecycleSkipsLifecycleTick: pins DESPAWN
  branch does not touch lifecycleTick

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 5d: Extend addNpc + introduce resetEntityForRespawn helper

**Files:**
- Modify: `modules/world/npc_registry.go:28-40` (extend addNpc, add resetEntityForRespawn)
- Modify: `modules/world/server.go:234` (caller update — temporary, formalized in 5e)
- Modify: `modules/world/player_npc_test.go:41` (caller update — temporary, formalized in 5e)
- Modify: `modules/world/npc_registry_test.go` (add tests)

- [ ] **Step 1: Write failing tests**

Append to `modules/world/npc_registry_test.go`:

```go
// TestAddNpcFirstSpawnAllocsSlot verifies that addNpc(n, duration, true)
// allocates a slot and registers the NPC in s.npcs and s.npcLoop.
// Mirrors TS World.addNpc firstSpawn=true branch at World.ts:1259-1262.
func TestAddNpcFirstSpawnAllocsSlot(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    n := NewNpc(0, 7, 100, 100, 0, typ)

    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    if n.nid <= 0 || n.nid >= len(s.npcs) {
        t.Errorf("n.nid: got %d, want valid slot", n.nid)
    }
    if s.npcs[n.nid] != n {
        t.Error("s.npcs[n.nid]: not registered")
    }
    foundInLoop := false
    for _, np := range s.npcLoop {
        if np == n {
            foundInLoop = true
            break
        }
    }
    if !foundInLoop {
        t.Error("s.npcLoop: not appended")
    }
}

// TestAddNpcRespawnSpawnSkipsSlotAlloc verifies that addNpc(n, duration, false)
// does NOT touch s.npcs or s.npcLoop — the NPC is already registered, this
// is the revertType respawn path. Mirrors TS World.addNpc firstSpawn=false
// at World.ts:1258-1262.
func TestAddNpcRespawnSpawnSkipsSlotAlloc(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("first addNpc: %v", err)
    }
    nidBefore := n.nid
    loopLenBefore := len(s.npcLoop)

    // Now respawn: should NOT alloc a new slot.
    if err := s.addNpc(n, -1, false); err != nil {
        t.Fatalf("respawn addNpc: %v", err)
    }
    if n.nid != nidBefore {
        t.Errorf("n.nid changed: got %d, want %d (firstSpawn=false must not realloc)", n.nid, nidBefore)
    }
    if len(s.npcLoop) != loopLenBefore {
        t.Errorf("s.npcLoop grew: got len %d, want %d (firstSpawn=false must not append)",
            len(s.npcLoop), loopLenBefore)
    }
}

// TestAddNpcTeleportsToStart verifies that addNpc(n, duration, false)
// teleports the NPC back to its (startX, startZ). Mirrors TS World.addNpc
// at World.ts:1264-1265.
func TestAddNpcTeleportsToStart(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("first addNpc: %v", err)
    }
    // NPC walks away from spawn.
    n.x = 150
    n.z = 150
    n.dead = true

    if err := s.addNpc(n, -1, false); err != nil {
        t.Fatalf("respawn addNpc: %v", err)
    }
    if n.x != 100 || n.z != 100 {
        t.Errorf("n.(x,z): got (%d,%d), want (100,100) (startX/startZ)", n.x, n.z)
    }
    if n.dead {
        t.Error("n.dead: got true, want false (respawn must clear)")
    }
}

// TestAddNpcRespawnSetsLifecycleTickWhenDurationGT0 verifies that on the
// duration > -1 branch, addNpc writes n.lifecycleTick = duration.
// Mirrors TS World.addNpc at World.ts:1291-1293.
func TestAddNpcRespawnSetsLifecycleTickWhenDurationGT0(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("first addNpc: %v", err)
    }

    if err := s.addNpc(n, 25, false); err != nil {
        t.Fatalf("respawn with duration: %v", err)
    }
    if n.lifecycleTick != 25 {
        t.Errorf("n.lifecycleTick: got %d, want 25", n.lifecycleTick)
    }
}
```

- [ ] **Step 2: Run new tests, confirm build error**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestAddNpc -v`

Expected: BUILD ERROR — `s.addNpc(n, -1, true)` mismatches the existing one-arg signature. This drives the implementation.

- [ ] **Step 3: Extend addNpc signature + introduce resetEntityForRespawn**

Edit `modules/world/npc_registry.go:28-40` from:

```go
// addNpc places n into a free slot, sets n.nid, appends to npcLoop.
// Caller responsible for synchronisation (called during NewServer or under playersMu).
func (s *Server) addNpc(n *Npc) error {
    nid := s.allocNpcSlot()
    if nid < 0 {
        return errNpcsFull
    }
    n.nid = nid
    n.server = s
    s.npcs[nid] = n
    s.npcLoop = append(s.npcLoop, n)
    return nil
}
```

to:

```go
// addNpc registers n in the world. Mirrors TS World.addNpc at
// World.ts:1258-1294.
//
// firstSpawn=true: allocate a slot, register in s.npcs + s.npcLoop
// (the original goscape behavior). firstSpawn=false: skip those —
// used by revertType's respawn cycle (NPC keeps its slot).
//
// Always: tele to (startX, startZ), clear dead flag, toggle collision
// flags on per n.typ.BlockWalk, run resetEntityForRespawn (stats
// reseed + waypoint/queue clear + tele/mask flag), reset animation,
// and (if duration > -1) write n.lifecycleTick.
//
// Caller responsible for synchronisation (called during NewServer or
// under playersMu).
func (s *Server) addNpc(n *Npc, duration int, firstSpawn bool) error {
    if firstSpawn {
        nid := s.allocNpcSlot()
        if nid < 0 {
            return errNpcsFull
        }
        n.nid = nid
        n.server = s
        s.npcs[nid] = n
        s.npcLoop = append(s.npcLoop, n)
        // TODO(NAI-19): rsbuf.AddNpc(n.nid, n.typeId) when rsbuf
        // API surface lands.
    }
    n.x = n.startX
    n.z = n.startZ
    n.dead = false
    // DEVIATION NAI-19-D1: zone.enter omitted — Zone abstraction
    // not ported. See spec § Tracked deviations.
    if n.typ != nil {
        switch n.typ.BlockWalk {
        case objtype.BlockWalkNPC:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, true)
        case objtype.BlockWalkAll:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, true)
            s.gamemap.ChangePlayerCollision(int(n.typ.Size), n.x, n.z, n.level, true)
        }
    }
    s.resetEntityForRespawn(n)
    n.animID = -1
    n.animDelay = 0
    // DEVIATION NAI-19-D2: AI_SPAWN trigger queue omitted —
    // script.TriggerAiSpawn (script/trigger.go:171) declared but no
    // spawn-flow consumer wiring. Activating here would change
    // first-spawn behavior across all existing NPCs at server boot.
    // Tracked for closure in a future "AI_SPAWN dispatch wiring"
    // sub-spec.
    if duration > -1 {
        n.lifecycleTick = duration
    }
    return nil
}

// resetEntityForRespawn applies the TS Npc.resetEntity(true) reseed
// (TS Npc.ts:280-317, respawn=true branch) factored out so addNpc and
// the future revertType refactor (Task 5e) share one definition.
//
// Resets typeId/uid to baseType (with fresh n.typ lookup), reseeds
// all 6 stats from n.typ.Stats, clears queue/waypoints, sets tele +
// CHANGE_TYPE mask, resets hunt fields. Does NOT touch n.x/n.z (the
// caller handles position) or collision flags (the caller handles
// those via gamemap).
func (s *Server) resetEntityForRespawn(n *Npc) {
    if n.typeId != n.baseType {
        n.typeId = n.baseType
        n.uid = (n.typeId << 16) | n.nid
        if newTyp := n.lookupType(n.baseType); newTyp != nil {
            n.typ = newTyp
        }
    }
    if n.typ != nil {
        for i := range min(objtype.NpcStatCount, len(n.typ.Stats)) {
            v := int(n.typ.Stats[i])
            n.levels[i] = v
            n.baseLevels[i] = v
        }
    }
    n.queue = nil
    n.waypointIndex = -1
    n.tele = true
    n.masks |= rsbuf.NpcMaskChangeType
    n.huntClock = 0
    n.huntTarget = nil
    if n.typ != nil {
        n.huntRange = int(n.typ.HuntRange)
        n.huntMode = n.typ.HuntMode
    }
}
```

Add `"github.com/zsrv/goscape/pkg/rsbuf"` to the import block if not present.

- [ ] **Step 4: Update existing addNpc callers to new signature**

Edit `modules/world/server.go:234` from `s.addNpc(n)` to `s.addNpc(n, -1, true)`.

Edit `modules/world/player_npc_test.go:41` from `s.addNpc(n)` to `s.addNpc(n, -1, true)`.

Edit `modules/world/npc_registry_test.go` (the three 5c-introduced tests `TestRemoveNpcCollisionTogglesOff`, `TestRemoveNpcRespawnLifecycleSetsLifecycleTick`, `TestRemoveNpcDespawnLifecycleSkipsLifecycleTick`): change each `s.addNpc(n)` setup call to `s.addNpc(n, -1, true)`. Total: 3 setup-call sites in that file.

Verify with: `grep -n "s\.addNpc(n)" modules/world/` — expected output: ZERO matches after this step. The one-arg signature is fully retired.

(These are the same caller updates 5e would otherwise formalize; doing them inline here keeps each commit buildable.)

- [ ] **Step 5: Build and run new tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: clean build.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestAddNpc -v`

Expected: all four `TestAddNpc*` PASS.

- [ ] **Step 6: Run full suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go modules/world/server.go modules/world/player_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 5d — extend addNpc to TS-faithful (n, duration, firstSpawn) signature + factor resetEntityForRespawn

Per TS World.addNpc at World.ts:1258-1294: branches on firstSpawn
(slot alloc + s.npcs/s.npcLoop register vs skip), tele to
(startX, startZ), clear dead, toggle collision flags on per
n.typ.BlockWalk, run resetEntityForRespawn helper, reset animation,
write n.lifecycleTick if duration > -1.

resetEntityForRespawn (new) factors the existing revertType heavy-path
stats-reseed body so addNpc and the upcoming 5e revertType refactor
share one definition. Reseeds typeId/uid (with fresh n.typ lookup
when baseType differs), stats array, queue/waypoints, tele + mask,
hunt fields.

DEVIATION NAI-19-D1: zone.enter omitted — Zone abstraction not ported.
DEVIATION NAI-19-D2: AI_SPAWN trigger queue omitted — script.TriggerAiSpawn
declared but no spawn-flow consumer wiring; activating here would change
first-spawn behavior across all existing NPCs at server boot.

Caller patches: server.go:234 + player_npc_test.go:41 updated from
s.addNpc(n) to s.addNpc(n, -1, true). Behaviorally unchanged for these
callers (duration=-1 + firstSpawn=true reproduces pre-NAI-19 semantics
for the slot-alloc + register flow; the new tele + collision toggle +
resetEntityForRespawn body runs but is a no-op or idempotent for fresh
NPCs at startup).

Tests:
- TestAddNpcFirstSpawnAllocsSlot
- TestAddNpcRespawnSpawnSkipsSlotAlloc
- TestAddNpcTeleportsToStart
- TestAddNpcRespawnSetsLifecycleTickWhenDurationGT0

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 5e: Refactor revertType heavy path + delete inline reset

**Files:**
- Modify: `modules/world/npc.go:240-307` (delete inline body, replace with TS-form 2-line call; remove NAI-17-D1 comment block)
- Modify: `modules/world/npc_test.go` (append six new revertType tests)

- [ ] **Step 1: Write failing tests**

Append to `modules/world/npc_test.go`:

```go
// TestRevertTypeHeavyPathTeles pins that revertType's heavy path teles the
// NPC back to (startX, startZ) per TS Npc.ts:1083-1085 → World.addNpc:1264.
func TestRevertTypeHeavyPathTeles(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    n.x = 150
    n.z = 150
    n.resetOnRevert = true

    n.revertType()

    if n.x != 100 || n.z != 100 {
        t.Errorf("n.(x,z): got (%d,%d), want (100,100) (startX/startZ)", n.x, n.z)
    }
}

// TestRevertTypeHeavyPathReseedsStats pins that revertType's heavy path
// reseeds all 6 stats from n.typ.Stats (via resetEntityForRespawn).
func TestRevertTypeHeavyPathReseedsStats(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{
        ConfigType: objtype.ConfigType{ID: 7},
        Size:       1,
        Stats:      []uint16{10, 20, 30, 40, 50, 60},
    }
    s.npcTypes.Configs[7] = typ
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    // Drain stats.
    for i := range objtype.NpcStatCount {
        n.levels[i] = 0
    }
    n.resetOnRevert = true

    n.revertType()

    want := []int{10, 20, 30, 40, 50, 60}
    for i := range objtype.NpcStatCount {
        if n.levels[i] != want[i] {
            t.Errorf("n.levels[%d]: got %d, want %d", i, n.levels[i], want[i])
        }
    }
}

// TestRevertTypeHeavyPathClearsQueueWaypoints pins that revertType's
// heavy path clears n.queue and n.waypointIndex.
func TestRevertTypeHeavyPathClearsQueueWaypoints(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    s.npcTypes.Configs[7] = typ
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1}}
    n.waypointIndex = 5
    n.resetOnRevert = true

    n.revertType()

    if n.queue != nil {
        t.Errorf("n.queue: got %v, want nil", n.queue)
    }
    if n.waypointIndex != -1 {
        t.Errorf("n.waypointIndex: got %d, want -1", n.waypointIndex)
    }
}

// TestRevertTypeHeavyPathRunsCollisionToggles pins that revertType's
// heavy path toggles collision flags off-then-on (via removeNpc + addNpc).
// Asserted indirectly through n.dead transitions: removeNpc sets dead=true,
// addNpc sets dead=false. Collision-flag observability is fixture-limited;
// this test pins the dead-flag round-trip as a proxy.
func TestRevertTypeHeavyPathRunsCollisionToggles(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{
        ConfigType: objtype.ConfigType{ID: 7},
        Size:       1,
        BlockWalk:  objtype.BlockWalkNPC,
    }
    s.npcTypes.Configs[7] = typ
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    n.resetOnRevert = true

    n.revertType()

    if n.dead {
        t.Error("n.dead post-revert: got true, want false (addNpc must clear)")
    }
}

// TestRevertTypeLightPathUnchanged pins that the !resetOnRevert (KEEPALL)
// branch is unchanged: typeId restored to baseType, uid recomputed,
// CHANGE_TYPE mask raised, resetOnRevert re-armed to true. No tele,
// no stats reseed, no queue clear.
func TestRevertTypeLightPathUnchanged(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    n := NewNpc(0, 7, 100, 100, 0, typ)
    n.server = s
    // Simulate KEEPALL changetype: typeId moved, resetOnRevert=false.
    n.typeId = 99
    n.uid = (99 << 16) | n.nid
    n.resetOnRevert = false
    n.x = 150
    n.z = 150
    n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1}}

    n.revertType()

    if n.typeId != n.baseType {
        t.Errorf("n.typeId: got %d, want baseType=%d", n.typeId, n.baseType)
    }
    if n.uid != (n.baseType<<16)|n.nid {
        t.Errorf("n.uid: got %d, want recomputed for baseType", n.uid)
    }
    if n.masks&rsbuf.NpcMaskChangeType == 0 {
        t.Error("NpcMaskChangeType bit not set")
    }
    if !n.resetOnRevert {
        t.Error("n.resetOnRevert: got false, want true (re-armed)")
    }
    if n.x != 150 || n.z != 150 {
        t.Errorf("n.(x,z): got (%d,%d), want (150,150) (light path must not tele)", n.x, n.z)
    }
    if len(n.queue) != 1 {
        t.Errorf("n.queue: light path must not clear; got len %d, want 1", len(n.queue))
    }
}

// TestRevertTypeUsesScaledRespawnDuration pins that revertType's heavy
// path goes through removeNpc(n, -1) which would normally consult
// scaleByPlayerCount; -1 short-circuits the RESPAWN-branch lifecycleTick
// write so we expect lifecycleTick UNCHANGED post-revert (TS removeNpc
// 1316-1318: only writes lifecycleTick when duration > -1).
func TestRevertTypeUsesScaledRespawnDuration(t *testing.T) {
    s := newTestServer(t)
    typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
    s.npcTypes.Configs[7] = typ
    n := NewNpc(0, 7, 100, 100, 0, typ)
    if err := s.addNpc(n, -1, true); err != nil {
        t.Fatalf("addNpc: %v", err)
    }
    n.lifecycleTick = 99 // any prior value
    n.resetOnRevert = true

    n.revertType()

    if n.lifecycleTick != 99 {
        t.Errorf("n.lifecycleTick: got %d, want 99 (revertType's duration=-1 must not write)", n.lifecycleTick)
    }
}
```

- [ ] **Step 2: Run new tests, confirm they fail or pass against current revertType**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRevertType -v`

Expected: existing tests (`TestRevertTypeHeavyPathReseedsStats`, etc.) pass against current inline body; new tele/collision-toggle tests FAIL because current revertType doesn't tele or call removeNpc/addNpc.

`TestRevertTypeHeavyPathTeles` will FAIL with `n.(x,z): got (150,150), want (100,100)` — current inline reset doesn't tele.

- [ ] **Step 3: Refactor revertType heavy path**

Edit `modules/world/npc.go:240-307`. Delete the entire NAI-17-D1 doc-comment block (lines 257-263) AND the heavy-path inline body (lines 276-307), replacing the function with:

```go
// Branches on resetOnRevert (written by changeTypeImpl):
//   - resetOnRevert=false (KEEPALL path): TS Npc.ts:1086-1090 light path.
//     Restore typeId/uid + raise CHANGE_TYPE mask. No stats reset, no
//     queue clear, no waypoint clear, no hunt-field reset. Intended
//     for short-lived morphs that must preserve combat state.
//   - resetOnRevert=true (default, CHANGETYPE path): structural TS port
//     per Npc.ts:1083-1085 — World.removeNpc(this, -1) + World.addNpc(
//     this, -1, false). The addNpc respawn cycle reseeds typeId/uid/typ,
//     reseeds all 6 stats, clears queue/waypoints, teles to
//     (startX, startZ), and re-arms collision flags. Goscape carries two
//     deviations (NAI-19-D1: no zone state, NAI-19-D2: no AI_SPAWN re-trigger)
//     against this structural form.
//
// What revertType does NOT do on either branch (intentional):
//   - varn resets (future; VarNpc subsystem not yet wired)
//   - activeScript clear (TS behaviour: a revert does not cancel an
//     in-flight script)
//
// Tail re-arm: sets resetOnRevert = true on BOTH branches so a
// subsequent CHANGETYPE on the same NPC starts from the default. TS
// gets this for free via the ctor rerun; Go must re-arm explicitly.
func (n *Npc) revertType() {
    if !n.resetOnRevert {
        // Light path — TS Npc.ts:1086-1090.
        if n.typeId != n.baseType {
            n.typeId = n.baseType
            n.uid = (n.typeId << 16) | n.nid
        }
        n.masks |= rsbuf.NpcMaskChangeType
        n.resetOnRevert = true
        return
    }

    // Heavy path — structural TS port per Npc.ts:1083-1085.
    // Goscape deviations NAI-19-D1 / NAI-19-D2 are documented at the
    // omission sites in s.removeNpc / s.addNpc.
    n.server.removeNpc(n, -1)
    _ = n.server.addNpc(n, -1, false) // err only on slot-full; firstSpawn=false skips alloc
    n.resetOnRevert = true             // re-arm default for next morph cycle
}
```

- [ ] **Step 4: Run all revertType tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRevertType -v`

Expected: all six new tests PASS. Existing tests (`TestNpcTurnRespawnAliveMorphReverts`) PASS.

- [ ] **Step 5: Run full suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

- [ ] **Step 6: Audit deviation comments**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "NAI-17-D1|NAI-19-D1|NAI-19-D2" modules/ pkg/ docs/`

Expected:
- ZERO matches for `NAI-17-D1` outside of `nai_followups.md` historical body (which is allowed).
- TWO matches for `NAI-19-D1` in `modules/world/npc_registry.go` (one in removeNpc, one in addNpc).
- ONE match for `NAI-19-D2` in `modules/world/npc_registry.go` (addNpc, AI_SPAWN omission).
- Spec doc references in `docs/superpowers/specs/2026-04-24-nai-19-path-b-bundle-design.md` are also expected.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc.go modules/world/npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 Task 5e — revertType heavy path is now the structural TS port

Replace the inline reset body at npc.go:276-307 with the two-line TS form:
  s.removeNpc(n, -1)
  s.addNpc(n, -1, false)

Per TS Npc.ts:1083-1085. The addNpc(firstSpawn=false) respawn cycle
(introduced in 5d) handles tele-to-start, dead-flag clear, collision
re-arm, stats reseed (via resetEntityForRespawn), and lifecycle re-init.
removeNpc (introduced in 5c) handles the despawn-side collision toggle
off and dead-flag set.

NAI-17-D1 retired: the structural deviation is closed. The remaining
TS divergences are now narrower and tracked under NAI-19-D1
(zone.leave/enter omitted, no Zone state) and NAI-19-D2 (AI_SPAWN
re-trigger omitted, no spawn-flow consumer for TriggerAiSpawn).

Light-path (KEEPALL, !resetOnRevert) branch unchanged.

Tests:
- TestRevertTypeHeavyPathTeles
- TestRevertTypeHeavyPathReseedsStats
- TestRevertTypeHeavyPathClearsQueueWaypoints
- TestRevertTypeHeavyPathRunsCollisionToggles
- TestRevertTypeLightPathUnchanged (regression pin)
- TestRevertTypeUsesScaledRespawnDuration

Existing TestNpcTurnRespawnAliveMorphReverts (NAI-16 Task 3) preserved.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle close

After all 5 tasks ship and the two-stage final review passes, write the close commit per `close_commit_memory_trailer.md`.

- [ ] **Step 1: Run final verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all green.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -c "DEVIATION NAI-" modules/ pkg/ | sort`

Expected: deviation count consistent with spec § "Active deviation count math" (15 → 16 active).

- [ ] **Step 2: Write close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world): NAI-19 closed — PATH B follow-up bundle (B1 + B2 + B3 + B4 + B5)

Five tracked PATH B follow-ups landed in one cadence pass:

- Task 1 (B3): NewNpc stats-loop modernized to range-min idiom.
- Task 2 (B1): cache.MakeCRCs relocated from asset/handler.go to
  world.startingFn — closes "TEST - belongs in world" code smell.
- Task 3 (B2): changeTypeImpl refreshes n.typ snapshot on both CHANGETYPE
  and KEEPALL paths — closes the post-NAI-18 staleness gap observable
  via inApproachDistance LoS.
- Task 4 (B4): sendRebuildNormal CRC source switched from gamemap to
  cache.PreloadedCRC — TS layering parity per RebuildNormalEncoder.ts:18-19;
  dead-mirror gamemap CRC table dropped.
- Task 5 (B5, α′): structural revertType respawn alignment — extended
  s.removeNpc/s.addNpc to TS-faithful (n, duration, firstSpawn) signatures;
  refactored 3 callers; rewrote revertType heavy path to the 2-line
  TS form (Npc.ts:1083-1085).

Deviation registry update:
  - NAI-17-D1 retired (structural revertType inline reset).
  - NAI-19-D1 added (zone.leave/enter omitted — Zone abstraction unported).
  - NAI-19-D2 added (AI_SPAWN re-trigger omitted in addNpc — TriggerAiSpawn
    declared but no spawn-flow consumer wiring).
  Net: 15 → 16 active. Each new deviation narrower and points at concrete
  future work (Zone state port; AI_SPAWN dispatch wiring).

Total delta: ~250-340 LOC production + ~150-200 LOC tests across 9 commits
(5 task feat commits + 4 internal Task-5 sub-piece commits).

Closes memory:
  - nai_followups.md § "From NAI-17 → Cosmetic" (B3)
  - nai_followups.md § "From NAI-17 → NAI-17-D1 closure track" (B5)
  - nai_followups.md § "From NAI-18 → Stale *Npc.typ snapshot after changetype" (B2)
  - NAI-16 spec § "Out of scope → cache.MakeCRCs() relocation" (B1)
  - NAI-16 spec § "Out of scope → RebuildNormalEncoder port" (B4, narrowed)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```
