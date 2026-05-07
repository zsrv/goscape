# NAI-116 — NpcInfo Orphan Mask-Header Byte Fix

**Type**: P0 fix sub-spec (compressed cadence — spec + plan in one doc)
**Routes from**: NAI-115 close residual queue (P0: door-teleport "T2" client crash)
**Cadence**: ~5 LOC production change + ~30 LOC test + 2 doc-comments. Single bundle, single task.
**Tech stack**: Go 1.26+

---

## 1. Problem

After `NAI-115` close, Tutorial Island progression past Master Chef is blocked: walking out of Master Chef's room and clicking `newbie_door3` triggers `[oploc1,newbie_door3] → [proc,open_and_close_door] → p_teleport($dest)`, after which the Java client crashes mid-tick with:

```
Error: T2 - 1,184,162 - 4,3072,3090 - 1,-97,-1,0,
```

The server-side log captured P_TELEPORT executing successfully (handler ran, coords correct), then immediately the client closed its connection.

## 2. Root cause (byte-level + line-level)

The client error format is at `LostCityRS/Client-Java/src/main/java/deob/client.java:10376`:

```
T2 - <packetType>,<lastPacketType1>,<lastPacketType2> - <packetSize>,<absX>,<absZ> - <bytes...>
```

So our crash decodes as:

- `packetType = 1` → `OpNpcInfo` (`pkg/io/protocol/game/server/prot.go:54`).
- `lastPacketType1 = 184` → `OpPlayerInfo` (decoded fine just before).
- `lastPacketType2 = 162` → `OpUpdateZonePartialEnclosed` (decoded fine before that).
- `packetSize = 4` → NpcInfo payload was 4 bytes.
- absolute coord `(3072, 3090)` matches the post-teleport position.
- payload bytes `[01 9F FF 00]` (signed `1, -97, -1, 0`).

**Bit-level decode of `[01 9F FF 00]` against `pkg/rsbuf/npcinfo.go`** (MSB-first):

| Bits   | Value             | Encoder operation                            |
| ------ | ----------------- | -------------------------------------------- |
| 0–7    | `0x01`            | `PBit(8, 1)` — 1 tracked NPC                 |
| 8–10   | `100`             | `PBit(1,1) PBit(2,0)` — Extend leaf (idle+mask) |
| 11–23  | `1_1111_1111_1111` | `PBit(13, 8191)` — terminator                 |
| 24–31  | `0x00`            | first byte of `ni.updates.Data` (mask payload) |

The 4th byte is `0x00`, which the Java client opcode-1 reader interprets as the NPC's mask header — **mask = 0 with no following payload bytes is invalid** (Lost City protocol uses `0` as the "extended-mask follows" sentinel; reading the next byte over-reads end-of-packet → exception → `T2`).

**The orphan `0x00` mask byte is produced by `pkg/rsbuf/renderer.go:96-117`** (`Renderer.ComputeNpcs`):

```go
masks := n.Masks()
if masks == 0 && n.EntityMask() == 0 {
    r.npcHighDef[nid] = nil
} else {
    buf := packet.NewPacket(nil)
    writeNpcMaskHeader(buf, masks)        // writes uint8(masks) = 0
    writeNpcMaskPayloads(buf, n, masks)   // writes nothing for masks=0
    r.npcHighDef[nid] = append([]byte(nil), buf.Data...)  // = [0x00]
}
```

When `masks == 0` but `EntityMask() != 0` (persistent FaceEntity carried across ticks via the cleanup-preserved field at `pkg/rsbuf/npc.go:74`), the `else` branch fires and caches a 1-byte payload of `[0x00]`. The encoder's `writeNpcs` switch (`npcinfo.go:139-177`) sees `hdLen > 0`, picks the Extend (or Walk/Run) leaf with `extend=1`, and `Encode` appends the orphan byte after the terminator + AccessBytes alignment — producing exactly `[0x01, 0x9F, 0xFF, 0x00]`.

**Why door3 surfaced it now**: Master Chef's interaction sets `FaceEntity` targeting the player, and `pkg/rsbuf/Npc.cleanup()` deliberately preserves `FaceEntity` across ticks (mirrors upstream `npc.rs:68-71`). Through the cutscene, Chef has additional transient masks each tick (Say, Anim) so `Masks() != 0` and the bug doesn't trigger. The first tick where Chef is still tracked but all transient masks have cleared (player walking out of the room toward newbie_door3), `Masks() == 0 && EntityMask() != 0` → orphan byte → crash. Earlier doors (`newbie_door1/2`) had no nearby persistent-FaceEntity NPC.

## 3. Existing precedent (binding line-level diff)

The PlayerInfo path at `pkg/rsbuf/renderer.go:34-40` already gates on `masks == 0` alone, with a doc-comment that articulates exactly this hazard:

```go
// High-def carries only the player's live mask updates. When masks
// is 0, leave highDef nil so the encoder takes the idle path with
// no extend bit and no orphan mask-header byte leaking into the
// packet (mirrors TS PlayerRenderer.computeInfo: `if masks === 0
// return;`, renderer.ts:41-43).
if masks == 0 {
    r.highDef[slot] = nil
    r.highDefWithChat[slot] = nil
} else {
    ...
}
```

Upstream Rust `NpcRenderer::compute_info` at `2004scape/rsbuf/src/renderer.rs:258-260`:

```rust
if nid == -1 || masks == 0 {
    return;
}
```

Both gate exclusively on `masks == 0`. Goscape's `ComputeNpcs` is the only path that adds `&& EntityMask() == 0`, and that addition is the bug.

## 4. Fix

**Production change** at `pkg/rsbuf/renderer.go:103`:

```go
// High-def carries only the NPC's live mask updates. When masks
// is 0, leave highDef nil so the encoder takes the idle path with
// no extend bit and no orphan mask-header byte leaking into the
// packet (mirrors upstream NpcRenderer::compute_info early-return
// at 2004scape/rsbuf/src/renderer.rs:258-260, and parallels the
// PlayerInfo gate at renderer.go:34-40).
//
// NAI-116: a persistent FaceEntity (EntityMask != 0) on a tick
// with masks==0 previously fell through to the else branch, writing
// a single 0x00 mask header byte. The encoder's Walk/Run/Extend
// leaves saw hdLen=1 and appended it to the wire, producing a
// 4-byte NpcInfo payload [0x01, 0x9F, 0xFF, 0x00] (count + Extend
// leaf + terminator + orphan 0x00) → Java client `Error: T2` on
// opcode 1.
if masks == 0 {
    r.npcHighDef[nid] = nil
} else {
    buf := packet.NewPacket(nil)
    writeNpcMaskHeader(buf, masks)
    writeNpcMaskPayloads(buf, n, masks)
    r.npcHighDef[nid] = append([]byte(nil), buf.Data...)
}

// Low-def: always recomputed. lowMasks always includes
// NpcMaskFaceCoord (line below), so the orphan-byte hazard
// doesn't apply here — the cache always has at least the
// FACE_COORD payload (4 bytes) behind its 1-byte mask header.
lowMasks := masks | NpcMaskFaceCoord
buf := packet.NewPacket(nil)
writeNpcMaskHeader(buf, lowMasks)
writeNpcMaskPayloads(buf, n, lowMasks)
r.npcLowDef[nid] = append([]byte(nil), buf.Data...)
```

Diff vs HEAD: removes `&& n.EntityMask() == 0` from line 103; adds NAI-116 attribution to the doc-comment; adds the low-def safety doc-comment.

## 5. Tests

### 5.1 Regression unit test (renderer-level)

Append to `pkg/rsbuf/renderer_npc_test.go` (alongside existing `TestComputeNpcsHighDefSkipsZero` which covers `masks==0 && entityMask==0`):

```go
// TestComputeNpcsHighDef_PersistentEntityMaskMasksZero pins the NAI-116
// regression: when an NPC has Masks()==0 but EntityMask()!=0 (e.g.
// persistent FaceEntity carried across ticks per pkg/rsbuf/npc.go:74),
// ComputeNpcs MUST produce a nil highDef so the NpcInfo encoder takes
// the idle leaf and emits no orphan 0x00 mask-header byte.
//
// Pre-NAI-116, the renderer wrote writeNpcMaskHeader(buf, 0) → [0x00]
// (1-byte orphan), which the encoder appended to the wire as a Walk/Run/
// Extend leaf payload. Java client opcode 1 (NpcInfo) decoded the leaf,
// read mask=0 with no following payload bytes, and crashed "Error: T2".
//
// Reproducer: Tutorial Island Master Chef has FaceEntity set across the
// cutscene; first tick after walking out where transient masks are clear
// → orphan byte → T2.
func TestComputeNpcsHighDef_PersistentEntityMaskMasksZero(t *testing.T) {
    r := NewRenderer()
    n := &fakeNpcSource{
        nid: 5, masks: 0, entityMask: NpcMaskFaceEntity,
        faceEntity: 12345, active: true,
    }
    r.ComputeNpcs([]NpcSource{n})
    if got := r.NpcHighDefOf(5); got != nil {
        t.Errorf("HighDef should be nil for masks==0 even when EntityMask!=0; got %#v", got)
    }
    // Low-def safety pin: lowMasks always includes NpcMaskFaceCoord, so
    // npcLowDef must always be at least header(1) + FACE_COORD payload(4)
    // = 5 bytes. Pins the doc-comment claim at renderer.go (line below
    // the gate fix).
    low := r.NpcLowDefOf(5)
    if len(low) < 5 {
        t.Errorf("LowDef should include FACE_COORD payload (header+4 bytes); got %#v", low)
    }
}
```

### 5.2 Round-trip end-to-end test (encoder-level)

Append to `pkg/rsbuf/npcinfo_test.go`:

```go
// TestNpcInfo_Encode_NoOrphanByteOnPersistentFaceEntity pins the NAI-116
// wire-output: with the renderer's masks==0 gate in place, a tracked NPC
// in the Idle branch produces a 2-byte NpcInfo payload [0x01, 0x00]:
// PBit(8,1) [count=1] + PBit(1,0) [Idle leaf] = 9 bits → AccessBytes pads
// to 2 bytes. No Extend bit, no terminator (updates stays empty), no
// orphan 0x00 mask-header byte.
//
// Pre-NAI-116, the same setup produced 4 bytes [0x01, 0x9F, 0xFF, 0x00]:
// count + Extend leaf "1 00" + 13-bit terminator 8191 + orphan mask byte
// — the exact bytes the Java client crashed decoding (T2 - 1,184,162 -
// 4,3072,3090 - 1,-97,-1,0).
func TestNpcInfo_Encode_NoOrphanByteOnPersistentFaceEntity(t *testing.T) {
    b := New()
    setupLocalPlayer(b, 1, nil)
    setupNpc(b, 7, 100, nil) // masks=0, faceEntity=-1 in *pkg/rsbuf/Npc
    // Pre-track nid=7 so writeNpcs handles it (skip writeNewNpcs path).
    b.players[1].Build.Npcs.Insert(7)

    ni := NewNpcInfo()
    r := NewRenderer()
    // Source for ComputeNpcs: masks=0, entityMask!=0 (FaceEntity carrier).
    // Pre-fix this populated r.npcHighDef[7] = [0x00].
    // Post-fix r.npcHighDef[7] = nil.
    n := &fakeNpcSource{
        nid: 7, masks: 0, entityMask: NpcMaskFaceEntity,
        faceEntity: 12345, active: true,
    }
    r.ComputeNpcs([]NpcSource{n})

    out := ni.Encode(b, 1, r)
    want := []byte{0x01, 0x00}
    if !bytes.Equal(out, want) {
        t.Errorf("Encode: got % x, want % x (NAI-116: orphan 0x00 mask byte must not leak)", out, want)
    }
}
```

### 5.3 Existing tests (no expected breakage)

- `TestComputeNpcsHighDefSkipsZero` (`renderer_npc_test.go:35-42`): `masks=0, entityMask=0` → nil. **Unchanged behavior** — gate still produces nil because `masks == 0`.
- `TestComputeNpcsHighDef` (`renderer_npc_test.go:5-20`): `masks=NpcMaskAnim` (non-zero) → header + payload. **Unchanged behavior** — gate is false, else branch still writes payload.
- `TestComputeNpcsLowDefForcesFaceCoord` (`renderer_npc_test.go:22-33`): `masks=0` → low-def includes FaceCoord. **Unchanged behavior** — low-def path is untouched.
- `TestNpcInfo_Encode_EmitsTerminatorBeforeMaskPayloads` (`npcinfo_test.go:754`): direct-assigns `r.npcLowDef[nid]`, doesn't go through `ComputeNpcs`. **Unchanged behavior**.

Plan-author MUST grep `pkg/rsbuf/*_test.go` for any test asserting `NpcHighDefOf(...)` is non-nil with `masks=0` (other than the four above). If any exist, they pre-pinned the orphan-byte bug and need updating to expect nil.

## 6. Verification gates

1. `go test ./pkg/rsbuf/...` passes (full rsbuf suite).
2. `go test ./...` passes (no downstream consumer relied on the orphan byte).
3. Diff at line 103 matches Section 4 verbatim.
4. Smoke (user-driven, post-merge per memory `smoke_test_server_handoff`): fresh Tutorial Island session → walk through Master Chef cutscene → walk through `newbie_door3` → no client crash, door step-through completes.

## 7. Risks

- **R1 — Existing test masking** (mitigated): grep step in §5.3 covers any pre-pin of buggy behavior.
- **R2 — Low-def assumption** (mitigated by §5.1's `len(low) >= 5` assertion): pins the doc-comment claim that `lowMasks` always includes FaceCoord; future removal of `| NpcMaskFaceCoord` would fail this assertion.
- **R3 — Adjacent T2 surfaces in smoke** (per memory `smoke_surfaces_adjacent_divergences`): possible but not predictable. Routing rule: in-scope-stretch if ≤30 LOC, else NAI-117 brainstorm queue.
- **R4 — Cache-staleness misdiagnosis** (per memory `cache_staleness_masquerades_as_encoder_bug`): rejected. Byte pattern decodes statically against the encoder; renderer's else branch at line 105-110 unambiguously produces the orphan byte; no compute/encode phase mismatch needed to explain the symptom.

## 8. Out of scope (deferred)

- NAI-116 P1: firemaking ashes-no-drop after fire despawn → NAI-117 brainstorm queue.
- NAI-116 P2: LOWMEM mismatch (server pushes 1 when client high-mem) → NAI-117 brainstorm queue.
- NAI-111: P_TELEJUMP `[label,tutorial_complete]` investigation → NAI-117 brainstorm queue.
- Any unrelated `compute_info` parity audit (Rust `header()` size-tracking, `lows` vs `highs` accounting) — strictly out of scope; this is a single-line gate fix.

## 9. Plan (single bundle, single task)

### Task 1 — TDD fix (compressed cadence)

**Step 1**: Append the regression test from §5.1 to `pkg/rsbuf/renderer_npc_test.go` and the round-trip test from §5.2 to `pkg/rsbuf/npcinfo_test.go`. Run `go test ./pkg/rsbuf/...` — both new tests fail (regression: highDef = `[0x00]` not nil; round-trip: `[0x01, 0x9F, 0xFF, 0x00]` not `[0x01, 0x00]`).

**Step 2**: Apply the production change from §4 to `pkg/rsbuf/renderer.go:96-117`. Re-run `go test ./pkg/rsbuf/...` — all green. Then `go test ./...` — full suite green.

**Step 3**: Commit on `main`:

```
fix(rsbuf): NAI-116 — drop EntityMask gate on NpcHighDef to suppress orphan mask byte

Closes memory: nai_116_orphan_mask_byte
```

(Closes-memory trailer per memory `close_commit_memory_trailer`.)

### No further tasks

Compressed cadence — single TDD cycle, no subagent dispatch needed for a 1-line fix + 2 unit tests. Per memory `compressed_cadence`: spec change ≤ ~15 LOC → spec+plan combined, formal review skipped, controller can implement directly OR dispatch a single Haiku/Sonnet TDD subagent.

**Recommended execution mode** (per memory `execution_mode_default`): subagent-driven-development single-task dispatch (Sonnet) with this spec as the brief. Implementer writes tests → fails → applies fix → green → commits. Estimated wall-time: 5-10 minutes.

## 10. Memory entries to add at close

- `nai_116_orphan_mask_byte.md` — type: `project`. Body: root cause (orphan 0x00 mask byte from masks==0 + EntityMask!=0), fix line (`pkg/rsbuf/renderer.go:103`), reproducer (Master Chef → newbie_door3), smoke binding date.

(Other adjacent memory entries — e.g., extending `dispatch_order_audit_blind_spot` — judged at close-time based on what the smoke surfaces.)
