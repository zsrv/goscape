# NAI-143 — `cameraPackets` accumulator + `cam_moveto` / `cam_lookat` / `cam_shake` opcode wiring (TS parity)

**Date:** 2026-05-09
**Status:** Brainstorm-approved spec.
**Cadence:** Mid (~200–300 LOC including tests) — spec + plan + single combined review per `compressed_cadence.md` 100+ LOC band.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Origin:** TS-fidelity port. Closes deferred follow-up `NAI-142-D-R-D1` from `nai_followups.md`. Surface today: three opcodes (`OpCamLookAt`=2007, `OpCamMoveTo`=2008, `OpCamShake`=2010) are declared in `pkg/script/opcode.go` but have no handler, no `ActivePlayer` interface method, no `Player` method, and no wire op — classic `protocol_stub_not_completed.md` pattern. `OpCamReset` (2009) is already complete (handler at `handlers_dialog.go:90-99`, wire op `OpCamReset = Op{239, 0}` at `prot.go:41`, direct-write method at `player_script.go:189-191`).
**TS source (canonical: `LostCityRS/Engine-TS`, per `ts_source_canonical_path.md`):**
- `src/engine/entity/CameraInfo.ts` — accumulator entry struct.
- `src/engine/entity/Player.ts:344` — `cameraPackets: LinkList<CameraInfo>` field declaration.
- `src/engine/entity/Player.ts:444` — `this.cameraPackets.clear()` inside `cleanup()` (logout/disconnect).
- `src/engine/entity/NetworkPlayer.ts:242-253` — drain loop in `updateMap()`; emits `CamMoveTo`/`CamLookAt` zone-relative against `originX/originZ`.
- `src/engine/script/handlers/PlayerOps.ts:206-228` — `CAM_LOOKAT`, `CAM_MOVETO`, `CAM_SHAKE`, `CAM_RESET` script handlers.
- `src/network/game/server/codec/CamMoveToEncoder.ts`, `CamLookAtEncoder.ts`, `CamShakeEncoder.ts`, `CamResetEncoder.ts` — wire formats.
- `src/network/game/server/ServerGameProt.ts:33-36` — opcode IDs and payload sizes:
  - `CAM_LOOKAT = (74, 6)` — `p1 x, p1 z, p2 height, p1 rate, p1 rate2`.
  - `CAM_SHAKE = (13, 4)` — `p1 axis, p1 random, p1 amplitude, p1 rate`.
  - `CAM_MOVETO = (3, 6)` — same shape as `CAM_LOOKAT`.
  - `CAM_RESET = (239, 0)` — already ported.

---

## §1 Acceptance criteria

### §1.1 PRIMARY (regression-fence smoke; user-launched server per `smoke_test_server_handoff.md`)

User-driven smoke at HEAD post-impl must observe:

1. **`cam_shake` reaches the client and renders camera shake.** Steps:
   1. Spawn, walk to the Falador `risingsun_barmaid` NPC (script `LostCityRS/Content/scripts/areas/area_falador/scripts/barmaid.rs2:34-35` calls `cam_shake(0, 0, 15, 2);` mid-dialogue).
   2. Trigger the dialogue branch that fires `cam_shake`.
   3. **Pin:** client camera shakes; no Java-client AIOOBE / disconnect / opcode-mismatch.
2. **`cam_moveto` + `cam_lookat` reaches the client without coord/window misalignment.** Steps (route choice depends on quest accessibility from spawn — pick first one reachable):
   1. Either: progress `quest_arena.rs2` to the cutscene starting at line 143 (sequence `cam_moveto`, `cam_lookat`).
   2. Or: dev-spawn a one-shot proc that calls `cam_moveto(spawncoord, 400, 0, 100); cam_lookat(spawncoord+1tile, 400, 0, 100);` and run it via `::` chat command.
   3. **Pin:** camera moves to and looks at the configured tile; no client AIOOBE / blackscreen / window-clip artifacts (which would indicate localX/Z arithmetic regression).
3. **`cam_reset` still works** (regression-fence: login.rs2 already exercises this; `NAI-N-1` smokes already pin it). Steps:
   1. After §1.1.2, observe: any `cam_reset;` sequence in the script restores normal camera. Login flow continues to work end-to-end.
4. **No regression in NAI-142 §1 criteria** (per-zone-change `rebuildZones`; cached-client cross-zone coverage). The bundle inserts the cam drain *before* the lastZone check in `updateBuildArea`; smoke re-witnesses the green NAI-142 path.

### §1.2 SECONDARY (unit-test pins)

Locked into automated tests (§3):

- **T1:** `TestCamMoveToHandler` — `cam_moveto` script with packed coord + 3 ints pops in TS order (rotationMultiplier, rotationSpeed, height, coord), validates coord via `checkCoord`, appends one `cameraInfo{kind:0, ...}` to `Player.cameraPackets`, no wire bytes written yet.
- **T2:** `TestCamLookAtHandler` — same as T1 but appends `kind:1`. Verifies the kind-byte distinction is wired correctly (per `handler_pop_order_test_masking.md`: explicit kind assertion against the appended struct, not just slice length).
- **T3:** `TestCamShakeHandler` — `cam_shake` pops 4 ints (rate, amplitude, random, axis — TS order), writes 4-byte payload directly via `writeOut(OpCamShake, [4]byte{axis, random, amplitude, rate})`, no entry appended to `cameraPackets`.
- **T4:** `TestCamMoveToHandler_invalidCoord` — `cam_moveto` with coord = -1 returns the same error shape as existing `checkCoord` callers (e.g. `TestLocFindAllZoneCoordValid` template).
- **T5:** `TestUpdateBuildAreaCameraDrain` — populated `cameraPackets = [{kind:0, camX:300, camZ:400, height:550, rotationSpeed:100, rotationMultiplier:100}]` + `originX:296, originZ:392` → assert exact 7-byte writeOut sequence: opcode `OpCamMoveTo` (3) + payload `[byte(300-zoneOrigin(296)), byte(400-zoneOrigin(392)), 0x02, 0x26, 100, 100]`. Per `rsbuf_roundtrip_tests.md`: pin big-endian on the p2 height field. After drain, `len(cameraPackets) == 0`.
- **T6:** `TestUpdateBuildAreaCameraDrain_lookatKind` — same shape as T5 but `kind:1` → assert opcode `OpCamLookAt` (74).
- **T7:** `TestUpdateBuildAreaCameraDrain_originFreshness` — set `Player.originX/originZ` to two distinct values across two ticks; populate `cameraPackets` between ticks; assert localX/Z computed against the FRESH origin (i.e. drain reads `originX/Z` at drain-time, not at append-time).
- **T8:** `TestUpdateBuildAreaCameraThenZone` — populate `cameraPackets` AND cross-zone in one tick (`p.lastZone != currentZone`) → assert cam packet emitted BEFORE any `rebuildZones`-triggered side effect (per TS line ordering: cam drain at lines 244-253, lastZone check at 269-271).
- **T9:** `TestCamMoveToHandler_noActivePlayer` — `s.Pointers&PtrActivePlayer == 0` returns the canonical error message shape (mirrors `handleCamReset` guard).

### §1.3 NEGATIVE / out-of-scope (NAI-N+1)

Explicitly NOT in this bundle:

- `lastMapZone` / `triggerMapzone` / `triggerMapzone_exit` (NAI-142-D-R-D2).
- `triggerZone` / `triggerZoneExit` / `SetMultiway` (NAI-142-D-R-D3).
- Cosmetic rename `updateMap → rebuildNormal` (NAI-142-D-R-D4).
- Any `[cam_*]` script-trigger registry (TS has none for camera; not applicable).
- Pooled-Player reuse / `cameraPackets.clear()` on logout — goscape allocates fresh `Player` per connection (`server.go:742-771`); slice is GC'd with the struct. Documented as deviation in §6.

---

## §2 Architecture

### §2.1 Components (full file inventory)

| File | Change | Purpose |
| --- | --- | --- |
| `pkg/io/protocol/game/server/prot.go` | +3 wire op consts | `OpCamMoveTo = Op{3, 6}`, `OpCamLookAt = Op{74, 6}`, `OpCamShake = Op{13, 4}` — siblings of existing `OpCamReset`. Doc-comments reference TS `ServerGameProt.ts:33-36`. |
| `modules/world/player.go` | +`cameraPackets []cameraInfo` field on `Player`; +`cameraInfo` struct; modify `updateBuildArea` | Accumulator on player; struct mirrors TS `CameraInfo.ts` 1:1. Drain inserted at top of `updateBuildArea` (before lastZone check) per TS `NetworkPlayer.updateMap` line ordering. |
| `modules/world/player_script.go` | +3 methods on `*Player`: `CamMoveTo`, `CamLookAt`, `CamShake` | `CamMoveTo`/`CamLookAt` append to `cameraPackets`; `CamShake` direct-writes (sibling of existing `CamReset` at line 189). |
| `pkg/script/active.go` | +3 methods on `ActivePlayer` interface | `CamMoveTo`, `CamLookAt`, `CamShake` near existing `CamReset` at line 371. |
| `pkg/script/runner_test.go` | +mockPlayer fields + impls | `mockPlayer.lastCamMoveTo`/`lastCamLookAt`/`lastCamShake` capture per existing CamReset capture pattern at line 600-601. |
| `pkg/script/handlers_dialog.go` | +3 handlers: `handleCamMoveTo`, `handleCamLookAt`, `handleCamShake` | Mirror `handleCamReset` (line 90-99) shape: `PtrActivePlayer` guard + pop ints + `checkCoord` for coord-bearing variants + dispatch. |
| `pkg/script/handlers.go` | +3 entries in opcode→handler map | `OpCamMoveTo: handleCamMoveTo`, `OpCamLookAt: handleCamLookAt`, `OpCamShake: handleCamShake` (companion to existing `OpCamReset: handleCamReset` at line 131). |
| `pkg/script/handlers_dialog_test.go` | +T1–T4, T9 (handler-layer pins) | Per existing `TestCamReset` template at line 62-72. |
| `modules/world/player_zone_test.go` (or new `player_camera_test.go`) | +T5–T8 (drain-layer pins) | Per existing `newZoneTestPlayer` fixture; reuse if it provides client/server hooks, else extend. |

### §2.2 Data flow

```
Tick N script execution:
  cam_moveto(coord, height, rate, rate2)
    -> handleCamMoveTo (handlers_dialog.go)
       -> pop 4 ints in TS order
       -> checkCoord(coord)          [unpack to (level, x, z); ignore level for cam — matches TS CoordValid]
       -> s.Self.CamMoveTo(x, z, height, rate, rate2)
          -> p.cameraPackets = append(p.cameraPackets, cameraInfo{kind:0, camX:x, camZ:z, ...})

  cam_shake(axis, random, amplitude, rate)
    -> handleCamShake
       -> pop 4 ints in TS order
       -> s.Self.CamShake(axis, random, amplitude, rate)
          -> p.writeOut(OpCamShake, [4]byte{axis, random, amplitude, rate})    [direct write — NO accumulator]

Tick N end-of-tick (Server.processInfo, per NAI-93):
  Player.updateMap()       [TS BuildArea.rebuildNormal slot — sets fresh originX/originZ if rebuildScenery fires]

Tick N processOut:
  Player.updateBuildArea()
    NEW: drain p.cameraPackets:
      for _, info := range p.cameraPackets {
          localX := info.camX - coordgrid.ZoneOrigin(p.originX)
          localZ := info.camZ - coordgrid.ZoneOrigin(p.originZ)
          payload := []byte{
              byte(localX), byte(localZ),
              byte(info.height>>8), byte(info.height),  // p2 big-endian
              byte(info.rotationSpeed), byte(info.rotationMultiplier),
          }
          op := gameserver.OpCamMoveTo
          if info.kind == 1 { op = gameserver.OpCamLookAt }
          p.writeOut(op, payload)
      }
      p.cameraPackets = p.cameraPackets[:0]
    EXISTING: lastZone check → rebuildZones.
```

### §2.3 Why the accumulator pattern (vs direct write)

`cam_moveto`/`cam_lookat` send **zone-relative** coords against `originX/originZ`. The origin can update mid-tick when `rebuildScenery` fires. TS defers emission to `updateMap` (post-rebuild) so the localX/Z arithmetic uses the fresh origin. `cam_shake` and `cam_reset` send **no coord** and don't need deferral.

Goscape ordering verified at brainstorm-time:
- `Player.updateMap` (TS `BuildArea.rebuildNormal` port; misnamed per NAI-142-D-R-D4) runs in `Server.processInfo` per NAI-93 (`player.go:733-755`, comment block at 745-751).
- `Player.updateBuildArea` (TS `NetworkPlayer.updateMap` port) runs at top of `processOut` (`player.go:898`).
- `processInfo` runs before `processOut` per goscape's per-tick orchestration.
- ⇒ origin is fresh by the time the cam drain runs. ✓

### §2.4 Struct shape

```go
// modules/world/player.go (near other Player transient slice fields)
type cameraInfo struct {
    kind                  uint8 // 0 = moveto, 1 = lookat (TS CameraInfo.type)
    camX, camZ            int   // world-space cam target coords (zone-relative computed at drain)
    height                int   // p2 (will be masked to 16 bits at encode)
    rotationSpeed         int   // p1 (will be masked to 8 bits at encode)
    rotationMultiplier    int   // p1
}
```

Slice over linked-list: drain-and-reset-each-tick fits slice better; goscape has no `LinkList` analogue. Field order matches TS `CameraInfo.ts:4-9` argument order for grep-symmetry. Identifier `cameraInfo` (lowercase; package-private) — only consumed inside `modules/world`.

### §2.5 Handler shape

Pop order (from TS `PlayerOps.ts:207`, `popInts(4)` then array-destructure `[coord, height, rotationSpeed, rotationMultiplier]`):

> `popInts(n)` in TS pops `n` ints from the stack and returns them in **forward order** (the int that was pushed first comes back at index 0). So the script pushes `coord`, `height`, `rotationSpeed`, `rotationMultiplier` left-to-right; goscape's `PopInt()` returns the most-recently-pushed first → reverse pop order is `rotationMultiplier`, `rotationSpeed`, `height`, `coord`.

```go
// pkg/script/handlers_dialog.go
func handleCamMoveTo(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("CAM_MOVETO: no active player")
    }
    rotationMultiplier := s.PopInt()
    rotationSpeed := s.PopInt()
    height := s.PopInt()
    coord := s.PopInt()
    _, x, z, err := checkCoord(coord, "CAM_MOVETO")
    if err != nil {
        return err
    }
    s.Self.CamMoveTo(x, z, height, rotationSpeed, rotationMultiplier)
    return nil
}
```

`handleCamLookAt` is identical except calling `CamLookAt`. `handleCamShake` pops `rate, amplitude, random, axis` (reverse of TS push order `axis, random, amplitude, rate`) and skips `checkCoord`.

### §2.6 Wire format

| Opcode | Wire op | Payload |
| --- | --- | --- |
| `OpCamMoveTo` | 3 | `[localX, localZ, height>>8, height, rotationSpeed, rotationMultiplier]` (6 bytes) |
| `OpCamLookAt` | 74 | same shape (6 bytes) |
| `OpCamShake` | 13 | `[axis, random, amplitude, rate]` (4 bytes) |
| `OpCamReset` | 239 | (already ported) 0 bytes |

Big-endian on `height` (p2) per `rsbuf_roundtrip_tests.md` — pinned in T5.

---

## §3 Test plan

### §3.1 Handler-layer (`pkg/script/handlers_dialog_test.go`)

Mirror `TestCamReset` template (line 62-72). Use mocked `s.Self` (`*mockPlayer` with capture fields per `runner_test.go:601`).

**Required mockPlayer additions (`runner_test.go`):**

```go
type mockPlayer struct {
    // ... existing fields ...
    camResetCalls   int
    lastCamMoveTo   *struct{ x, z, height, rate, rate2 int }
    lastCamLookAt   *struct{ x, z, height, rate, rate2 int }
    lastCamShake    *struct{ axis, random, amplitude, rate int }
}

func (m *mockPlayer) CamMoveTo(x, z, height, rate, rate2 int) {
    m.lastCamMoveTo = &struct{ x, z, height, rate, rate2 int }{x, z, height, rate, rate2}
}
// ... CamLookAt, CamShake similarly
```

Test bodies follow `TestCamReset` script-driven pattern: build a `Script` with bytecode `[push int args ...] [opcode] [Op.RETURN]`, run via `ScriptState`, assert capture fields.

Per `int32_hex_literal_overflow.md`: cast scriptIDs as `int32(uint32(0x...))` if any test fixture's script ID > `0x7FFFFFFF`.

### §3.2 Drain-layer (`modules/world/player_camera_test.go` — new file, sibling of `player_zone_test.go`)

Reuse `newZoneTestPlayer` fixture from `player_zone_test.go` if it provides:
- A real `*Player` with initialized `client.bufw` (or a buffer recorder for `writeOut` capture).
- Configurable `originX`, `originZ`, `lastZone`, `level`, `x`, `z`.

Per `test_fixture_view_parity.md`: verify the fixture gives `writeOut` a valid `c.bufw` (else writes are silent no-ops). If not, extend the fixture or stand up a thin recorder analogous to NAI-142's `newZoneTestPlayer`.

T5 (drain bytes pin):
```go
p := newZoneTestPlayer(t)
p.originX = 296
p.originZ = 392
p.cameraPackets = []cameraInfo{{
    kind: 0, camX: 300, camZ: 400, height: 550, rotationSpeed: 100, rotationMultiplier: 100,
}}
p.updateBuildArea()
// assert: c.bufw recorded one writeOut for OpCamMoveTo (3) with payload:
//   localX = 300 - coordgrid.ZoneOrigin(296)
//   localZ = 400 - coordgrid.ZoneOrigin(392)
//   height: [0x02, 0x26]   (550 = 0x0226, big-endian)
//   rotationSpeed: 100
//   rotationMultiplier: 100
// assert: len(p.cameraPackets) == 0
```

T7 (origin freshness): two-tick sequence — append on tick 1 with `originX/Z = A`, mutate origin to `B` between tick 1 emit and tick 2, append on tick 2; assert tick 2's localX/Z computed against B.

Per `plan_runnable_test_fixtures.md`: every test code block above must be runnable as-is once translated to a `func TestX(t *testing.T)` shell. The implementer plan task carries verbatim copies.

### §3.3 Smoke

Per `cascade_theory_smoke_binding.md` and `smoke_test_server_handoff.md`:
- Server is **user-launched** (sandbox unreachable).
- Smoke pin §1.1.1 (barmaid `cam_shake`) is the cheapest reach; do it first.
- Smoke pin §1.1.2 (`cam_moveto`+`cam_lookat`) chooses between quest_arena progression and a dev-spawn `::` proc — the implementer can pick whichever is wired at HEAD.

---

## §4 Risk register

| # | Risk | Likelihood | Mitigation |
| --- | --- | --- | --- |
| R1 | TS pop order mis-translated (script pushes left-to-right; goscape pops right-to-left) | Medium | T1–T3 explicitly assert the kind/field mapping per `handler_pop_order_test_masking.md`. Do NOT hand-tune the test fixture's push order to match a buggy pop order. |
| R2 | `height` p2 written little-endian instead of big-endian | Medium | T5 pins exact bytes `[0x02, 0x26]` for height=550 per `rsbuf_roundtrip_tests.md`. |
| R3 | Drain reads stale origin (e.g. snapshotted at append-time) | Low | T7 explicitly mutates origin between append and drain. The design reads `p.originX/Z` at drain-time inside the loop, no closures. |
| R4 | `cam_shake` accidentally routed through accumulator (TS does direct-write) | Low | T3 asserts `len(cameraPackets) == 0` after `cam_shake`. |
| R5 | Drain inserted in wrong place — runs after lastZone rebuildZones, breaking TS ordering | Low | T8 pins ordering. Drain code goes at the *very top* of `updateBuildArea`, before any other line. |
| R6 | `checkCoord` accepts `coord = 0` (origin); some scripts may pass it inadvertently. TS `CoordValid` allows [0, 2147483647] — keep parity. | Negligible | Not a divergence — match TS exactly. |
| R7 | Interface bloat: `ActivePlayer` already has 60+ methods; adding 3 more pushes coupling | Low | Per `interface_at_cyclic_import_boundary.md` — `ActivePlayer` is the canonical seam between `pkg/script` and `modules/world`; siblings (`CamReset`, `HintNpc`, `HintCoord`) live there. Stay consistent. |

Per `risk_register_premise_grep.md`: every "Low" / "Negligible" claim above is grep-verified at spec-write time:
- R5: grep shows `updateBuildArea` body is exactly `lastZone` check + `rebuildZones` + `lastZone = zone` (player.go:880-886). Cam drain prepends — no insertion conflict.
- R7: grep `func (p *Player) Cam` in `player_script.go` shows only `CamReset` exists; siblings live in same file.

---

## §5 Out-of-scope items routed to NAI-N+1

Already enumerated in §1.3. None of these block NAI-143's primary or secondary acceptance.

If §1.1 smoke surfaces an adjacent divergence (per `smoke_surfaces_adjacent_divergences.md`), route stretch fixes ≤30 LOC inline and larger items to NAI-144+.

---

## §6 Tracked deviations

- **D1: Slice instead of LinkList for `cameraPackets`.** TS uses `LinkList<CameraInfo>`; goscape uses `[]cameraInfo`. Rationale: goscape has no `LinkList` type; the linked-list semantics (ordered append + iterate-and-clear) are equivalent for this drain pattern. No behavioral effect; doc-comment notes the divergence per `defensive_gate_doc_comment_label.md` style: `// (slice for goscape; TS uses LinkList<CameraInfo>)`.
- **D2: No explicit `cameraPackets` clear on logout.** TS clears in `Player.cleanup()` (`Player.ts:444`) because TS pools Player slots. Goscape allocates fresh Player per connection (`server.go:742-771` drops the player on disconnect; new login allocates) — slice is GC'd with the struct. No follow-up needed unless the lifecycle changes.
- **D3: Method-name shape `CamMoveTo` (vs TS `cameraPackets.addTail`).** Goscape exposes a fluent helper on Player rather than reaching into the slice from the handler. Sibling `CamReset` already follows this shape. No behavioral effect.

Per `true_to_ts_gate.md`: every deviation has a rationale; D1 and D3 have no follow-up planned (idiomatic Go; not behavioral); D2 is conditional (only revisits if Player pooling lands).

---

## §7 Pattern memories applied

- `compressed_cadence.md` — single-task TDD bundle; combined plan-author + implementer review (no separate split given LOC band).
- `superpowers_code_reviewer_model.md` — both reviewers dispatched on Sonnet.
- `smoke_test_server_handoff.md` — server user-launched.
- `close_commit_memory_trailer.md` — `Closes memory:` trailer expected on close commit.
- `protocol_stub_not_completed.md` — confirms the pre-existing opcode-table declarations are stubs, not live; enumerates dispatch gap.
- `handler_pop_order_test_masking.md` — T1–T3 pin pop order via kind-byte/field assertion, not slice-length-only.
- `rsbuf_roundtrip_tests.md` — T5 pins big-endian on `height`.
- `plan_runnable_test_fixtures.md` — all test code blocks must be runnable as-is.
- `verify_implementer_claims.md` — controller verifies post-impl claims via fresh test runs and grep.
- `nai_followups.md` — NAI-142-D-R-D1 closes; D-R-D2/D3/D4 remain open.

---

## §8 Estimated LOC

| Component | Production | Test | Total |
| --- | --- | --- | --- |
| `prot.go` wire ops (3) | ~12 | 0 | ~12 |
| `cameraInfo` struct + field | ~10 | 0 | ~10 |
| Drain in `updateBuildArea` | ~15 | 0 | ~15 |
| `Player.CamMoveTo`/`CamLookAt`/`CamShake` | ~30 | 0 | ~30 |
| `ActivePlayer` interface (3 methods + doc) | ~15 | 0 | ~15 |
| `mockPlayer` capture fields + impls | 0 | ~25 | ~25 |
| 3 handlers in `handlers_dialog.go` | ~50 | 0 | ~50 |
| Handler dispatch table (3 entries) | ~3 | 0 | ~3 |
| `handlers_dialog_test.go` T1–T4, T9 | 0 | ~80 | ~80 |
| `player_camera_test.go` T5–T8 | 0 | ~100 | ~100 |
| **Total** | **~135** | **~205** | **~340** |

Slightly above the 100-LOC compressed-cadence ceiling but still within the single-bundle TDD band — splitting into separate handler/drain bundles would create artificial seams (the drain has no production callers without the handlers).
