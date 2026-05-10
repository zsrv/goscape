# NAI-145 — zone triggers + SetMultiway (NAI-142-D-R-D2 + NAI-142-D-R-D3)

## §1 Scope

Port the two transition blocks of TS `NetworkPlayer.updateMap`
(`Engine-TS/src/engine/entity/NetworkPlayer.ts:255-287`) that NAI-142
explicitly deferred and NAI-144 §9 carry-forward staged for this
sub-spec:

- **NAI-142-D-R-D2:** `lastMapZone` tracking +
  `triggerMapzone(x,z)` / `triggerMapzoneExit(x,z)` dispatch
  (NetworkPlayer.ts:256-266; method bodies at Player.ts:561-574).
  Fires `[mapzone,…]` / `[mapzoneexit,…]` cache scripts on
  64-tile-grid (mapsquare) transitions.

- **NAI-142-D-R-D3:** zone-block enrichment — `triggerZone(level,x,z)` /
  `triggerZoneExit(level,x,z)` dispatch (NetworkPlayer.ts:280-285;
  method bodies at Player.ts:576-596) plus `SetMultiway(hidden)`
  packet emission on multi-flag transitions (NetworkPlayer.ts:274-278).
  The existing `lastZone`/`rebuildZones()` skeleton in
  `(*Player).updateBuildArea` (player.go:947-952, NAI-142) is
  enriched in place.

**Bundle rationale:** The TS source is one method (`updateMap`) with
two adjacent if-blocks sharing the same wiring point — splitting
fragments the single TS surface across two sub-specs. Confirmed
single-bundle at brainstorm 2026-05-10.

### Out of scope (explicitly)

- `DEVIATION-NAI-144-D4` (canAccess≈!Busy approximation in
  `processPlayerEngineQueues`) — already documented in code; no
  D2/D3 touch points.
- `NAI-144-D-MoveClickRequestSetter` — needs `World.ts:611-628`
  per-tick post-decode pathfinding pass; orthogonal to zone triggers.
  Movement gate at movement.go:64 stays inert.
- Smoke handoff — deferred per `cascade_theory_smoke_binding`.
  See §7.

---

## §2 TS source map

| TS source | Port target |
|---|---|
| `NetworkPlayer.ts:256-266` (mapzone block) | `(*Player).updateBuildArea` step 2 (new) |
| `NetworkPlayer.ts:268-287` (zone block) | `(*Player).updateBuildArea` step 3 (enrich existing :947) |
| `Player.ts:379` `lastMapZone: number = -1` | `Player.lastMapZone int` field (new) |
| `Player.ts:561-567` `triggerMapzone` | `(*Player).triggerMapzone` (new) |
| `Player.ts:569-574` `triggerMapzoneExit` | `(*Player).triggerMapzoneExit` (new) |
| `Player.ts:576-585` `triggerZone` | `(*Player).triggerZone` (new) |
| `Player.ts:587-596` `triggerZoneExit` | `(*Player).triggerZoneExit` (new) |
| `network/game/server/model/SetMultiway.ts` | `OpSetMultiway` opcode constant (new) |
| `network/game/server/codec/SetMultiwayEncoder.ts` (`buf.pbool(hidden)`) | inline 1-byte payload at emission site (no helper) |
| `ServerGameProt.ts:65` `SET_MULTIWAY = new ServerGameProt(254, 1)` | opcode 254, payload size 1 |

Trigger-key shapes (verified at HEAD against `LostCityRS/Engine-TS`
2026-05-10):

| Trigger | Key shape | Notes |
|---|---|---|
| mapzone | `[mapzone,0_{x>>6}_{z>>6}]` | level always 0 in TS |
| mapzoneexit | `[mapzoneexit,0_{x>>6}_{z>>6}]` | NO underscore after `mapzoneexit` |
| zone | `[zone,{level}_{x>>6}_{z>>6}_{(x&0x3f)>>3<<3}_{(z&0x3f)>>3<<3}]` | 5 segments |
| zoneexit | `[zoneexit,{level}_{x>>6}_{z>>6}_{(x&0x3f)>>3<<3}_{(z&0x3f)>>3<<3}]` | NO underscore after `zoneexit` |

Per memory `nai_followups` 2026-05-09 verification: 284 `[mapzone,…]`
+ 17 `[mapzoneexit,…]` + 100 `[zone,…]` + 5 `[zoneexit,…]` real
declarations in `LostCityRS/Content`.

---

## §3 Architecture

### §3.1 Field add

`modules/world/player.go` field block (after `lastZone` at :358):

```go
// lastMapZone is the previously-witnessed packed mapzone coord
// (level=0, mapsquareX<<6, mapsquareZ<<6) used by updateBuildArea
// to detect per-tick mapzone (64-tile-grid) transitions. Sentinel
// -1 forces the first updateBuildArea call to fire triggerMapzone
// without firing triggerMapzoneExit (matches TS Player.ts:379
// `lastMapZone: number = -1` + NetworkPlayer.ts:259 `!== -1` guard).
lastMapZone int
```

Construction at player.go:562 — add `lastMapZone: -1,` next to
`lastZone: -1,`.

### §3.2 Four trigger methods — new file `modules/world/player_zone_triggers.go`

Cohesive group; mirrors precedent of `changeStat`/`advanceStat` in
`player_script.go:587-615`. Each method:

1. Nil-guard the script-provider chain (matches all other player-
   side trigger dispatchers — `changeStat`/`advanceStat` precedent).
2. Build the trigger-key string via `fmt.Sprintf`.
3. Call `scriptProvider.GetByName(name)` → returns `*ScriptFile` or `nil`.
4. Call `p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)` —
   the file-based variant nil-guards `sf` internally and routes to
   `p.engineQueue` per NAI-144 wiring. Closer to TS
   `enqueueScript(trigger, ENGINE)` shape than the ID-roundtrip
   `EnqueueScriptArgs`.

```go
package world

import (
    "fmt"
    "github.com/zsrv/goscape/pkg/script"
)

// triggerMapzone fires the [mapzone,0_X_Z] cache script when content
// is registered for the entered 64-tile mapzone. Mirrors TS
// Player.ts:561-567 (NAI-142-D-R-D2). Silent no-op when no script
// is registered (GetByName returns nil → EnqueueScriptFile no-ops).
func (p *Player) triggerMapzone(x, z int) {
    if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
        return
    }
    name := fmt.Sprintf("[mapzone,0_%d_%d]", x>>6, z>>6)
    sf := p.client.server.scriptProvider.GetByName(name)
    p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerMapzoneExit fires the [mapzoneexit,0_X_Z] cache script when
// content is registered for the exited 64-tile mapzone. Mirrors TS
// Player.ts:569-574. NOTE: exit key has NO underscore after
// `mapzoneexit` — verified against LostCityRS/Content 2026-05-09
// (17 [mapzoneexit,...] declarations).
func (p *Player) triggerMapzoneExit(x, z int) {
    if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
        return
    }
    name := fmt.Sprintf("[mapzoneexit,0_%d_%d]", x>>6, z>>6)
    sf := p.client.server.scriptProvider.GetByName(name)
    p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerZone fires the [zone,L_MX_MZ_LX_LZ] cache script for the
// entered 8-tile zone. Mirrors TS Player.ts:576-585. The 5-segment
// key encodes mapsquare (MX,MZ) + zone-local 8-tile-aligned offset
// (LX,LZ).
func (p *Player) triggerZone(level, x, z int) {
    if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
        return
    }
    mx := x >> 6
    mz := z >> 6
    lx := ((x & 0x3f) >> 3) << 3
    lz := ((z & 0x3f) >> 3) << 3
    name := fmt.Sprintf("[zone,%d_%d_%d_%d_%d]", level, mx, mz, lx, lz)
    sf := p.client.server.scriptProvider.GetByName(name)
    p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerZoneExit fires the [zoneexit,L_MX_MZ_LX_LZ] cache script
// for the exited 8-tile zone. Mirrors TS Player.ts:587-596. NO
// underscore after `zoneexit` — verified against LostCityRS/Content
// 2026-05-09 (5 [zoneexit,...] declarations).
func (p *Player) triggerZoneExit(level, x, z int) {
    if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
        return
    }
    mx := x >> 6
    mz := z >> 6
    lx := ((x & 0x3f) >> 3) << 3
    lz := ((z & 0x3f) >> 3) << 3
    name := fmt.Sprintf("[zoneexit,%d_%d_%d_%d_%d]", level, mx, mz, lx, lz)
    sf := p.client.server.scriptProvider.GetByName(name)
    p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}
```

### §3.3 SetMultiway opcode

`pkg/io/protocol/game/server/prot.go` — add adjacent to other state-
flag ops (target slot near `OpUpdateRunWeight` etc., or wherever
opcode order keeps the file scannable; concrete neighbor chosen at
plan time):

```go
// OpSetMultiway tells the client to show or hide the multi-combat
// overlay icon (top-right of the chatbox). Sent on transitions
// across multi-combat zone boundaries. 1-byte payload: pbool(hidden)
// — 0 if leaving multi (hide overlay), 1 if entering (show overlay).
// Mirrors TS ServerGameProt.SET_MULTIWAY (opcode 254, size 1) and
// SetMultiwayEncoder (`buf.pbool(message.hidden)`).
OpSetMultiway = Op{Opcode: 254, PayloadSize: 1}
```

Emission is inline in `updateBuildArea` (1 line — no helper, matching
`OpCamShake` direct-write at player_script.go:208 precedent).

### §3.4 `updateBuildArea` body enrichment

`modules/world/player.go:924-953`. After the camera drain (line 945),
insert mapzone block; in the existing lastZone block (lines 947-952),
add SetMultiway emission + triggerZoneExit/triggerZone before
`p.lastZone = zone`.

Reorganized layout (full method, post-NAI-145):

```go
func (p *Player) updateBuildArea() {
    // 1. drain cameraPackets — TS NetworkPlayer.ts:244-253 (NAI-143).
    for i := range p.cameraPackets { /* unchanged */ }
    p.cameraPackets = p.cameraPackets[:0]

    // 2. lastMapZone — TS NetworkPlayer.ts:255-266 (NAI-142-D-R-D2).
    mapZone := coordgrid.PackCoord(0, (p.x>>6)<<6, (p.z>>6)<<6)
    if p.lastMapZone != mapZone {
        if p.lastMapZone != -1 {
            prev := coordgrid.UnpackCoord(p.lastMapZone)
            p.triggerMapzoneExit(prev.X, prev.Z)
        }
        p.triggerMapzone((p.x>>6)<<6, (p.z>>6)<<6)
        p.lastMapZone = mapZone
    }

    // 3. lastZone — TS NetworkPlayer.ts:268-287
    //    (NAI-142 rebuildZones; NAI-145 SetMultiway + zone triggers).
    zone := coordgrid.PackCoord(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
    if p.lastZone != zone {
        p.rebuildZones()

        // SetMultiway emit on multi-flag transition. First-tick:
        // p.lastZone = -1 → UnpackCoord(-1) yields {Level:3, X:0x3FFF,
        // Z:0x3FFF} → IsMulti map-miss → false. This matches TS
        // World.gameMap.isMulti(-1) which also map-misses to false.
        // Net first-tick behavior: no SetMultiway emit unless the
        // current tile IS multi (then emits SetMultiway(1)).
        var lastWasMulti, nowIsMulti bool
        if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
            // (goscape defensive; TS skips this check — World.gameMap is
            // always present in TS but test fixtures here often omit it.)
            gm := p.client.server.gamemap
            prev := coordgrid.UnpackCoord(p.lastZone)
            lastWasMulti = gm.IsMulti(prev.X, prev.Z, prev.Level)
            nowIsMulti = gm.IsMulti(p.x, p.z, p.level)
        }
        if lastWasMulti != nowIsMulti {
            var hidden byte
            if nowIsMulti {
                hidden = 1
            }
            p.writeOut(gameserver.OpSetMultiway, []byte{hidden})
        }

        if p.lastZone != -1 {
            prev := coordgrid.UnpackCoord(p.lastZone)
            p.triggerZoneExit(prev.Level, prev.X, prev.Z)
        }
        p.triggerZone(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
        p.lastZone = zone
    }
}
```

**Step ordering matches TS exactly:**
- mapzone block fires `triggerMapzoneExit` BEFORE `triggerMapzone`
  (NetworkPlayer.ts:259-264).
- zone block fires `rebuildZones` first, then `SetMultiway` emit,
  then `triggerZoneExit`, then `triggerZone` (NetworkPlayer.ts:271-285).

The doc-comment block above `updateBuildArea` (player.go:894-923) is
updated to remove the "deferred follow-ups" pointer to D2/D3 (now
ported) and to describe the mapzone + zone trigger flow.

---

## §4 Test strategy

Tests live in `modules/world/player_zone_triggers_test.go` (new) and
extend `modules/world/player_camera_test.go` patterns for
updateBuildArea-level integration. ~7 tests, ~150 LOC.

| # | Test | Pin |
|---|---|---|
| T1 | `TestTriggerMapzone_RegisteredScriptEnqueued` | Register `[mapzone,0_50_60]` in scriptProvider; call `p.triggerMapzone(50<<6, 60<<6)`; assert `len(p.engineQueue) == 1` and the queued ScriptFile matches the registered one. |
| T2 | `TestTriggerMapzone_UnregisteredKeySilent` | No registration; call → `len(p.engineQueue) == 0`. |
| T3 | `TestTriggerZone_KeyShape_Bitmath` | At (x=3214, z=3398, level=0): `50=3214>>6`, `53=3398>>6`, `lx=((3214&0x3f)>>3)<<3 = ((14)>>3)<<3 = 1<<3 = 8`, `lz=((3398&0x3f)>>3)<<3 = ((6)>>3)<<3 = 0<<3 = 0`. Expected key `[zone,0_50_53_8_0]`. Plan-author re-verifies arithmetic at plan-write time per `tracker_expected_value_premise_pretrace`. |
| T4 | `TestTriggerZoneExit_NoUnderscore_KeyShape` | At same fixture coords (x=3214, z=3398, level=0): expected key `[zoneexit,0_50_53_8_0]`. Pins the absence of an underscore between `zoneexit` and the level segment (vs `mapzone_` having one). |
| T5 | `TestUpdateBuildArea_FirstTick_MapzoneFires_ExitDoesNot` | Fresh player (lastMapZone=-1) at (3200, 3200, 0); register `[mapzone,0_50_50]` AND `[mapzoneexit,0_50_50]`. After one `updateBuildArea` call: triggerMapzone enqueued (1 entry), triggerMapzoneExit NOT enqueued. `lastMapZone` updated. |
| T6 | `TestUpdateBuildArea_SetMultiwayEmitOnEntry` | Fresh player (lastZone=-1) in tile marked `gamemap.SetMulti(p.x, p.z, p.level, true)`. Call updateBuildArea. Assert exactly one outbound packet with `op == OpSetMultiway` and payload `[]byte{0x01}`. |
| T7 | `TestUpdateBuildArea_NoSetMultiwayWhenBothFalse` | Fresh player in non-multi tile (no SetMulti call). updateBuildArea. Assert NO `OpSetMultiway` packet appears in outbound stream. (Other packets — rebuildZones-related — may appear; pin only the absence of opcode 254.) |

**Test fixture parity:** Per `test_fixture_view_parity` memory, any
test that exercises `updateBuildArea`'s SetMultiway branch must
initialize `s.gamemap` (currently `pkg/gamemap.GameMap`). Use the
existing `gamemap.SetMulti(x, z, level, multi)` test setter (multimap.go:31).

---

## §5 Deviations / risk register

### Deviations introduced

None planned. Straight port from a single TS method. Both bitmask
arithmetic and key shapes are line-by-line transliterations of the
TS source.

### Risks

| # | Risk | Mitigation |
|---|---|---|
| R1 | gamemap nil-guard divergence — TS `World.gameMap` is always present; goscape test fixtures often skip gamemap init. | Explicit nil-guard with doc-comment label `(goscape defensive; TS skips this check)` per `defensive_gate_doc_comment_label` memory. Production gamemap is initialized at server start (`server.go:191`). |
| R2 | First-tick lastWasMulti semantics — `UnpackCoord(-1)` yields a defensive position; goscape's `IsMulti(0x3FFF, 0x3FFF, 3)` returns false (map miss) which matches TS `isMulti(-1)` map-miss. | Pin via T6 (entry into multi tile emits SetMultiway(1)) + T7 (no emit when both sides false). Doc-comment in updateBuildArea body explains the chain. |
| R3 | Trigger-key drift — four sites with subtly different shapes (mapzone has level=0 hardcoded, zone has runtime level; mapzoneexit/zoneexit drop the underscore separator). | Per-site key-shape tests (T3, T4) pin literal expected strings via known-coord fixtures. Plan-time recomputation per `tracker_expected_value_premise_pretrace`. |
| R4 | `EnqueueScriptFile` vs `EnqueueScriptArgs` choice — NAI-144 §9 carry-forward note named `EnqueueScriptArgs(scriptID, …)` but TS uses `enqueueScript(trigger, ENGINE)` (file-based). | Use `EnqueueScriptFile` to match TS shape AND existing `changeStat`/`advanceStat` precedent (player_script.go:592, 614). Avoids a redundant ID→file roundtrip; aligned with NAI-144 wiring. |
| R5 | Bit-arithmetic transliteration error — TS `(this.x & 0x3f) >> 3 << 3` precedence: `(((x&0x3f)>>3)<<3)`. Go has the same operator precedence, so direct port works. | T3 / T4 pin via known-coord fixtures. Plan-author re-derives expected values from raw TS arithmetic. |

---

## §6 Memories applied

- `nai_followups` — checked at brainstorm 2026-05-10; D2/D3 are listed
  for NAI-145 pickup.
- `ts_source_canonical_path` — only `LostCityRS/Engine-TS` cited; no
  sibling repos.
- `spec_ts_source_read` — TS bodies read line-by-line at HEAD, not
  derived "by analogy" from the carry-forward note.
- `spec_followup_tracker_freshness` — NAI-144 §9 prescription
  (`EnqueueScriptArgs`) cross-checked against TS `Player.ts:561-596`
  and against existing `changeStat`/`advanceStat` precedent; corrected
  to `EnqueueScriptFile` per R4.
- `runescript_cadence` — full cadence (not compressed) given
  ~80 production LOC + ~150 test LOC + 4-task split.
- `compressed_cadence` — considered and rejected; surface exceeds the
  ~15 LOC threshold.
- `defensive_gate_doc_comment_label` — gamemap nil-guard doc-comment
  per R1.
- `tracker_expected_value_premise_pretrace` — T3 expected key recomputed
  at plan-write rather than asserted from spec memory.
- `cascade_theory_smoke_binding` — smoke deferral acceptable; carry-
  forward in §7.
- `superpowers_code_reviewer_model` — reviewer dispatches Sonnet-only.
- `close_commit_memory_trailer` — bundle close commit will carry
  `Closes memory:` trailer.
- `superpowers_clear_between_spec_and_impl`, `session_context_management`
  — NAI-145 close = fresh session boundary before NAI-146 brainstorm.
- `execution_mode_default` — subagent-driven-development, no execution-
  mode menu.
- `verify_implementer_claims` — controller verifies T5/T6/T7
  assertions post-Task 3 dispatch.

---

## §7 Smoke handoff

**DEFERRED** per user choice + `cascade_theory_smoke_binding`.
Foundational/secondary infra acceptable to ship test-only.

**Carry-forward:** Bind on next D2/D3-adjacent observable symptom.
Concrete bind candidates for a future smoke:

- **Wilderness boundary crossing** — Player walks across a
  wilderness multi-combat boundary; client-side multi-combat overlay
  icon should flip on/off. Requires `multiway.csv` to mark the
  target tile as multi (verified at HEAD: `pkg/gamemap/multimap.go`
  loads it).
- **`[zone,…]` content fire** — A player walks into an 8-tile zone
  that has a registered `[zone,…]` script in the cache (e.g.,
  hypothetical `[zone,0_50_50_0_0]` quest-trigger entry). Server
  log captures the engine-queue script fire.
- **`[mapzone,…]` content fire** — Mapsquare entry equivalent
  (e.g., crossing into a city's mapsquare).

---

## §8 Cadence

Full cadence per `runescript_cadence`:

1. **Spec** — this doc; commit.
2. **Plan** — 4-task TDD plan (next session, post `/clear`):
   - **T1:** Add `lastMapZone` field + four trigger methods +
     red unit tests (T1, T2, T3, T4).
   - **T2:** Add `OpSetMultiway` opcode + green T3 / T4 trigger-key
     tests + smoke compile.
   - **T3:** Wire mapzone + zone blocks into `updateBuildArea` +
     green integration tests (T5, T6, T7).
   - **T4:** Reviewer fixups (Sonnet-only per
     `superpowers_code_reviewer_model`).
3. **Subagent-driven implementation** per `execution_mode_default`.
4. **Single end-of-impl reviewer subagent** on Sonnet.
5. **Close commit** with `Closes memory:` trailer per
   `close_commit_memory_trailer`.

Surface estimate: ~80 production LOC (4 trigger methods @ ~10 LOC
each + 1 opcode constant + ~35 LOC updateBuildArea body changes) +
~150 test LOC.

---

## §9 Carry-forward to NAI-146+

- `DEVIATION-NAI-144-D4` (canAccess≈!Busy) — still open; needs a
  TS-faithful `canAccess` port. Tracker entry remains in
  `processPlayerEngineQueues` doc-comment.
- `NAI-144-D-MoveClickRequestSetter` — still open; needs
  `World.ts:611-628` per-tick post-decode pathfinding pass to set
  `moveClickRequest = true`. Movement gate at movement.go:64 stays
  inert until then.
- Zone/mapzone smoke — bind on next adjacent symptom (see §7).
