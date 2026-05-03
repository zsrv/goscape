# NAI-80 — LocType client-jagfile pass + full TS-shape port

**Date:** 2026-05-03
**Tech Stack:** Go 1.26+
**Cadence:** Compressed (spec+plan combined per `compressed_cadence.md`); root cause pinned statically before brainstorm — no Stage 1 investigation bundle.
**Predecessor:** NAI-79 Bundle H4 close (commit `9a96b8b`) pinned 3/3 Tutorial-Island OPLOC1 clicks (door 3014, bookcase 380, drawer 350) to gate `op_slot_empty`. NAI-80 is the routed Stage 2.5 fix.

---

## §1. Root cause

The 4-hypothesis menu in `2026-05-03-nai-79-door-cascade-blocker-investigation.md` last section collapses on a static read of TS source. Hypothesis **4 (refined)** wins without bin-diff:

**TS** `Engine-TS/src/cache/config/LocType.ts:25-46`:

```ts
static parse(server: Packet, jag: Jagfile) {
    const count = server.g2();
    const client = jag.read('loc.dat')!;
    client.pos = 2;

    for (let id = 0; id < count; id++) {
        const config = new LocType(id);
        config.decodeType(server);
        config.decodeType(client);    // ← Op[0..4] arrives here (codes 30-34)
        config.postDecode();
        ...
    }
}
```

**goscape** `pkg/objtype/loctype.go:77-106` reads only `data/pack/server/loc.dat`. The codes 30-34 branch (lines 36-49) is structurally correct but receives no input — the client jagfile pass that carries Op[0..4] is missing entirely. Codes 1, 2, 3, 14, 15, 17-29, 30-34, 39, 40, 60, 62, 64-73 all live in `client/config`'s `loc.dat` entry; the goscape `data/pack/client/config` blob (132,952 B) exists on disk but is never opened by `LoadLocTypes`.

**Cross-package precedent**: `NpcType` (`npctype.go:348-394`), `ObjType`, `IdkType`, `ComponentType`, `SeqType` all do the dual-pass (server file + client jagfile entry). LocType is the package's lone holdout. Existing `loctype_test.go` cases pass because they synthesize fake server-only payloads with codes 30-34 inline; they never load the real cache, so the gap is invisible to unit tests.

**Symptom path (3 doors → same gate):**
- Click loc 3014/380/350 → handler reaches `handler_oploc.go:89` → `len(locType.Op) < op || locType.Op[op-1] == ""` evaluates true (Op slice is `nil`) → emits `gate=op_slot_empty` → returns without `p.SetInteraction`. Player never starts walking.

**Adjacent observation** (from seed `nai80_seed_loctype_op_empty.md`): all three smoke records show `player_uid=-1`. Fresh-tutorial-stage chars have no uid assigned at this lifecycle stage. Not a NAI-80 blocker but flagged as data shape to expect if downstream uid-mismatches surface post-fix.

---

## §2. Scope

**In-scope:**
1. `pkg/objtype/loctype.go` — full TS-shape port:
   - Extend `LocType` struct with all TS render fields, with TS-matching defaults.
   - Extend `LocType.Decode` switch with all client-blob codes (1, 2, 17, 18, 19, 21, 22, 23, 24, 25, 28, 29, 39, 40, 60, 62, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73).
   - Add `LocType.PostDecode` mirroring TS `postDecode()` at `LocType.ts:202-214`.
   - Modify `LoadLocTypes` to load `client/config` jagfile and pass to `parseLocTypes`.
   - Modify `parseLocTypes` to dual-decode (server then client) per id and call `PostDecode`.
   - Update top-of-file doc comment: retire the "this server-only loader skips" sentence; replace with TS-source pointer.
2. `pkg/objtype/loctype_test.go` — extend per-code-arm coverage and add `TestPostDecode_*` cases.
3. `pkg/objtype/loctype_realcache_test.go` *(new)* — real-cache regression pinning `Op[0]` non-empty for loc ids 3014, 380, 350.
4. **Smoke handoff** to user: re-run Java client at close commit, repeat 3 OPLOC1 clicks, expect gate advancement past `op_slot_empty`.

**Out-of-scope:**
- The `"hidden"→""` coercion at `loctype.go:47-49` stays in place (preserved as tracked deviation **NAI-80-D1**; see §6). A separate sub-spec will verify TS OPLOC1 dispatch behavior on `"hidden"` slots and route a fix if needed.
- LC_* script-handler ports that consume the new struct fields (Active, BlockWalk, Anim, etc.). Fields land in NAI-80; consumers land per their own sub-specs.
- Any change to `pkg/io/jagfile/` or `pkg/io/packet/` — both already power Npc/Obj/Idk/Component/Seq dual-pass; no signature changes needed.
- The `player_uid=-1` adjacent observation — note only; not a NAI-80 deliverable.

---

## §3. Architecture

### §3.1. Components

| File | Change |
|---|---|
| `pkg/objtype/loctype.go` | Add render-field struct members + TS defaults via `NewLocType`. Add full TS code arms to `Decode`. Add `PostDecode` method. Modify `LoadLocTypes` to load client jagfile and `parseLocTypes` to dual-decode + call `PostDecode`. Update top-of-file doc comment. |
| `pkg/objtype/loctype_test.go` | Add per-code-arm decode tests for new codes; add `TestPostDecode_*` triple; add `TestParseLocTypes_DualPass` synthetic-jagfile test. |
| `pkg/objtype/loctype_realcache_test.go` *(new)* | `LoadLocTypes("../../data/pack")`, soft-skip on absence, assert `Op[0]` non-empty for 3014, 380, 350; sanity-pin one additional known-name loc to catch silent ID-shift regressions. |

`pkg/objtype/configtype.go::DecodeType` is unchanged — already correctly loops `code := buf.G1(); if code == 0 break; f.Decode(code, buf)` until terminator. Each blob (server + client) is self-terminating; the dual-pass is "decode-until-0 from server, then decode-until-0 from client."

### §3.2. Struct shape

After port, `LocType` mirrors TS `LocType` (`LocType.ts:69-106`):

```go
type LocType struct {
    ConfigType
    // Client-side render fields (mirror of TS LocType.ts:71-102)
    Models        []uint16  // code 1, paired with Shapes
    Shapes        []uint8   // code 1, paired with Models
    Name          string    // code 2
    Desc          string    // code 3
    RecolS        []uint16  // code 40, paired with RecolD
    RecolD        []uint16  // code 40
    Width         int       // code 14, default 1
    Length        int       // code 15, default 1
    BlockWalk     bool      // code 17 sets false; default true
    BlockRange    bool      // code 18 sets false; default true
    Active        int       // code 19 sets g1; default -1, postDecode coerces
    HillSkew      bool      // code 21 sets true; default false
    ShareLight    bool      // code 22 sets true; default false
    Occlude       bool      // code 23 sets true; default false
    Anim          int       // code 24, 65535 → -1 coercion; default -1
    HasAlpha      bool      // code 25 sets true; default false
    WallWidth     int       // code 28; default 16
    Ambient       int8      // code 29 (g1b)
    Contrast      int8      // code 39 (g1b)
    Op            []string  // codes 30-34, lazy 5-slot init; "hidden"→"" coercion (D1)
    MapFunction   int       // code 60; default -1
    MapScene      int       // code 68; default -1
    Mirror        bool      // code 62 sets true; default false
    Shadow        bool      // code 64 sets false; default true
    ResizeX       int       // code 65; default 128
    ResizeY       int       // code 66; default 128
    ResizeZ       int       // code 67; default 128
    ForceApproach int       // code 69; default 0
    OffsetX       int16     // code 70 (g2s)
    OffsetY       int16     // code 71 (g2s)
    OffsetZ       int16     // code 72 (g2s)
    ForceDecor    bool      // code 73 sets true; default false

    // Server-side fields (already present)
    Category int           // code 61
    Params   ParamMap      // code 249
}
```

`NewLocType` initialises non-zero defaults: `Width=1, Length=1, BlockWalk=true, BlockRange=true, Active=-1, WallWidth=16, Anim=-1, Shadow=true, ResizeX=128, ResizeY=128, ResizeZ=128, MapFunction=-1, MapScene=-1, Category=-1, Params=make(ParamMap)`.

### §3.3. Decode arms (TS-mirroring)

Each new arm reads bytes per TS arithmetic. The trickiest:

- **Code 1 (Models+Shapes)**: `count := dat.G1(); Models = make([]uint16, count); Shapes = make([]uint8, count); for i := 0; i < count; i++ { Models[i] = dat.G2(); Shapes[i] = dat.G1() }`
- **Code 24 (Anim)**: `Anim = int(dat.G2()); if Anim == 65535 { Anim = -1 }`
- **Code 29/39 (Ambient/Contrast)**: signed byte via `int8(dat.G1())` — TS uses `g1b()` which is `g1() | (g1() & 0x80 ? -256 : 0)`; goscape's `Packet` exposes `G1B()` if present, else the cast pattern.
- **Code 40 (RecolS+RecolD)**: same shape as code 1 but with two `G2()` reads per i.
- **Codes 70/71/72 (offsets)**: signed 2-byte via `dat.G2S()` — TS uses `g2s()`.

Verified at spec-write: `pkg/io/packet/packet.go:155 G1B() int8` and `:170 G2S() int16` both exist. No new packet helpers needed.

### §3.4. PostDecode

Port of TS `postDecode()` (`LocType.ts:202-214`):

```go
func (lt *LocType) PostDecode() {
    if lt.Active == -1 {
        lt.Active = 0
        if len(lt.Shapes) == 1 && lt.Shapes[0] == 10 {
            lt.Active = 1
        }
        if lt.Op != nil {
            lt.Active = 1
        }
    }
}
```

The `Op != nil` branch in particular is what gets exercised by all 3 cascade-blocker doors after the fix — they have `Op[0]` populated, so `Active=1`. Pre-fix this never ran (Op was nil → no inference effect), but it's a coupled invariant the future LC_* port will rely on, so it ships in NAI-80.

### §3.5. Dual-pass call sites

```go
func LoadLocTypes(dir string) (*LocTypeConfigs, error) {
    server, err := packet.Load(filepath.Join(dir, "server", "loc.dat"), false)
    if err != nil {
        return nil, err
    }
    clientJag, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "config"))
    if err != nil {
        return nil, err
    }
    return parseLocTypes(server, clientJag)
}

func parseLocTypes(server *packet.Packet, clientJag *jagfile.Jagfile) (*LocTypeConfigs, error) {
    count := int(server.G2())

    client, err := clientJag.Read("loc.dat")
    if err != nil {
        return nil, err
    }
    client.Pos = 2

    configs := make([]*LocType, count)
    configNames := make(map[string]int, count)

    for id := range count {
        config := NewLocType(id)
        if err := DecodeType(server, config); err != nil {
            return nil, fmt.Errorf("loc id %d (server): %w", id, err)
        }
        if err := DecodeType(client, config); err != nil {
            return nil, fmt.Errorf("loc id %d (client): %w", id, err)
        }
        config.PostDecode()
        configs[id] = config
        if config.DebugName != "" {
            configNames[config.DebugName] = id
        }
    }

    return &LocTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

Mirrors `npctype.go:348-403` exactly.

### §3.6. Top-of-file comment update

Replace the existing `pkg/objtype/loctype.go:10-17` block:

```go
// LocType is the server-side subset of a cache loc (scenery/door/etc.). The
// full TS LocType decodes many more fields from the client jagfile (models,
// shapes, recolours, resizes, etc.) which this server-only loader skips.
//
// server/loc.dat in the real cache contains only codes 61 (category), 249
// (params), and 250 (debugname); Desc/Width/Length are defined here so the
// LC_* handlers have a place to read from even if the packer never writes
// them to the server blob.
```

with:

```go
// LocType mirrors Engine-TS/src/cache/config/LocType.ts. Loaded via a
// dual-pass decode: server/loc.dat contributes codes 61/249/250 (category,
// params, debugname), and the client jagfile entry loc.dat contributes
// the render+gameplay fields (codes 1-73). PostDecode infers Active from
// Shapes/Op when the cache leaves it unset.
//
// "hidden" → "" coercion in code 30-34 (NAI-80-D1) is preserved from S6k
// for handler-gate simplicity; see docs for follow-up sub-spec routing.
```

---

## §4. Data flow

```
LoadLocTypes("data/pack")
  │
  ├── packet.Load("data/pack/server/loc.dat")
  │     → server *Packet (sparse: codes 61, 249, 250 per id)
  │
  ├── jagfile.LoadJagfile("data/pack/client/config")
  │     → *Jagfile
  │
  └── parseLocTypes(server, clientJag)
        │
        ├── count = server.G2()                    // e.g., 6900
        ├── client = clientJag.Read("loc.dat"); client.Pos = 2
        │     (skip the 2-byte count header on the client side; server's count is authoritative)
        │
        └── for id in 0..count:
              cfg = NewLocType(id)              // TS-default values
              DecodeType(server, cfg)           // sparse: 61/249/250
              DecodeType(client, cfg)           // dense: 1, 2, 14, 15, 17-29, 30-34, 39, 40, 60, 62, 64-73, 250
              cfg.PostDecode()                  // active inference
              configs[id] = cfg
              if cfg.DebugName != "" → configNames[name] = id
```

`DebugName` (code 250) appears in both files; the second `DecodeType` overwrites with the same string — no special-casing needed.

---

## §5. Error handling / edge cases

| Case | Behavior | Justification |
|---|---|---|
| `data/pack/server/loc.dat` missing | `LoadLocTypes` returns error from `packet.Load`. | Existing behavior, unchanged. TS `LocType.load` early-returns silently; goscape opted to surface (project precedent in npctype/objtype). |
| `data/pack/client/config` missing | `LoadLocTypes` returns error from `LoadJagfile`. | Matches `LoadNPCTypes` precedent (`npctype.go:354-357`). TS does not guard the jagfile path either; both treat its absence as fatal. |
| `clientJag.Read("loc.dat")` returns error | Propagated. | Matches `parseNPCTypes` shape. |
| `count` mismatch (server count > client entries) | `DecodeType(client, cfg)` either decodes garbage codes (if any bytes remain) or no-ops if at terminator. Eventually fails on unknown code or `count` overrun. | Same failure mode as Npc/Obj/Idk; no goscape-specific safety net. |
| Unknown opcode in client blob | Existing `default: return fmt.Errorf("unrecognized loc config code %d", code)` in `LocType.Decode` catches it; load fails at id N with attributable message. | After γ port, every TS code is handled; if a future cache version contains a newer code, loader fails loudly (correct fail-fast). |
| `PostDecode` called twice | Not possible — single per-id call site. | n/a |
| `Op` slice nil, `len(Op) < op` | Handler at `handler_oploc.go:89` already gates `len(locType.Op) < op || locType.Op[op-1] == ""`; both arms tolerated. | No handler change needed for this spec. |

---

## §6. Tracked deviations

### NAI-80-D1 — `"hidden" → ""` coercion in op-slot decoder

**Location:** `pkg/objtype/loctype.go:47-49` (preserved verbatim).

**TS counterpart:** `Engine-TS/src/cache/config/LocType.ts:152-157` stores the literal `"hidden"` string with no coercion.

**Why preserved:** The S6k port introduced this coercion so the runtime gate at `modules/world/handler_oploc.go:89` can do a single `Op[op-1] == ""` check. Removing the coercion requires verifying TS OPLOC1 dispatch behavior on `"hidden"`-labeled slots — TS may *dispatch* to such slots via a different code path (menu label suppressed but action wired). Bundling that investigation into NAI-80 widens scope past the cascade-blocker fix.

**Follow-up:** Future investigation sub-spec to:
1. Read TS OPLOC1/OPLOC2/.../OPLOC5 dispatch path with `op = "hidden"` input.
2. If TS rejects → confirm coercion, drop deviation tag.
3. If TS dispatches → drop coercion, retarget `handler_oploc.go:89` gate to also reject `"hidden"` literal (or whatever TS rejects on).

**Impact while preserved:** none of the 3 cascade-blocker doors (3014/380/350) carry `"hidden"` slots — they had *no* Op data at all, the bug NAI-80 fixes. Other locs may have hidden-slot mismatches that show only at content-driven smoke; if surfaced, route to the follow-up sub-spec.

---

## §7. Testing strategy

### §7.1. Unit tests (`pkg/objtype/loctype_test.go` extensions)

**Per-code-arm decode tests:** for each newly-handled code, synthesize a 1-loc payload `[code byte][arg bytes][0x00 terminator]`, instantiate via `NewLocType(0)`, run `DecodeType(synthPacket, cfg)`, assert the corresponding struct field matches expected value.

Tabular per-arm coverage:

| Code | Arm | Pin |
|---|---|---|
| 1 | Models+Shapes pair | count=2, distinct values; assert paired indexing |
| 2 | Name | non-empty string round-trip |
| 17 | BlockWalk | sets false (default true) |
| 18 | BlockRange | sets false (default true) |
| 19 | Active | sets g1 value |
| 21 | HillSkew | sets true |
| 22 | ShareLight | sets true |
| 23 | Occlude | sets true |
| 24 | Anim | non-65535 value preserved; 65535 → -1 |
| 25 | HasAlpha | sets true |
| 28 | WallWidth | sets g1 value |
| 29 | Ambient | `dat.G1B()`, signed (test 0xFF → -1) |
| 39 | Contrast | `dat.G1B()`, signed (test 0xFF → -1) |
| 40 | RecolS+RecolD pair | count=2, distinct values |
| 60 | MapFunction | g2 value |
| 62 | Mirror | sets true |
| 64 | Shadow | sets false (default true) |
| 65/66/67 | ResizeX/Y/Z | g2 values |
| 68 | MapScene | g2 value |
| 69 | ForceApproach | g1 value |
| 70/71/72 | OffsetX/Y/Z | `dat.G2S()` (test negative) |
| 73 | ForceDecor | sets true |

**`TestPostDecode_ActiveInference`** (3 sub-cases):
- `Active` left at -1, `Op != nil` → `Active = 1`.
- `Active` left at -1, `Shapes = []uint8{10}` → `Active = 1`.
- `Active` left at -1, neither → `Active = 0`.
- (Sanity) `Active = 5` (already set) → `Active = 5` (unchanged).

**`TestParseLocTypes_DualPass`**: synthetic 1-entry server.dat + 1-entry jagfile (using the `buildMinimalJagfile` helper pattern from `componenttype_test.go:751`):
- Server payload: `[code 61: category=42][code 250: debugname="testloc"][0x00]`
- Client payload (inside jag's loc.dat entry, prefixed with 2-byte count `0x00 0x01`): `[code 30: op="Open"][code 14: width=2][code 15: length=3][code 250: debugname="testloc"][0x00]`
- Assert: `Configs[0].Op[0] == "Open"`, `Width == 2`, `Length == 3`, `Category == 42`, `DebugName == "testloc"`, `ConfigNames["testloc"] == 0`.

**`TestParseLocTypes_HiddenCoercion`** (D1 pin): client payload has `[code 30: op="hidden"]`; assert `Configs[0].Op[0] == ""`. Locks in the deviation against accidental removal.

### §7.2. Real-cache regression (`pkg/objtype/loctype_realcache_test.go`, new file)

```go
func TestLoadLocTypes_RealCache_CascadeBlockerLocs(t *testing.T) {
    cacheDir := "../../data/pack"
    if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); errors.Is(err, fs.ErrNotExist) {
        t.Skip("data/pack not present; skipping real-cache regression")
    }
    cfgs, err := LoadLocTypes(cacheDir)
    if err != nil {
        t.Fatalf("LoadLocTypes: %v", err)
    }

    for _, tc := range []struct {
        id   int
        name string
    }{
        {3014, "RS Guide door"},
        {380, "bookcase"},
        {350, "drawer"},
    } {
        cfg := cfgs.Configs[tc.id]
        if cfg == nil {
            t.Errorf("loc %d (%s): nil config", tc.id, tc.name)
            continue
        }
        if cfg.Op == nil || len(cfg.Op) < 1 || cfg.Op[0] == "" {
            t.Errorf("loc %d (%s): expected Op[0] non-empty (NAI-80 cascade-blocker pin); got Op=%v", tc.id, tc.name, cfg.Op)
        }
    }
}
```

Plus one ID-shift sanity pin: load a loc by `ConfigNames["<known-debugname>"]` and assert its expected `Op[0]` value matches what the bin-decode of the cache shows. Implementer derives the expected debugname by inspecting the cache during plan execution. Suggested probe: a tree-shaped loc whose `Op[0]=="Chop"` matches the existing test fixture at `loctype_test.go:153`, but pinned against real cache rather than synthetic.

### §7.3. Smoke handoff (post-impl, user-driven)

Ask the user to:
1. Restart `goscape` world server at NAI-80 close commit.
2. Java client login as Tutorial Island fresh char.
3. Repeat the 3 OPLOC1 clicks captured in NAI-79 H4 re-smoke: RS Guide door, bookcase, drawer.
4. Capture session-log `oploc gate` records.

**Expected gate signal at fix:** `gate=script_dispatch` (or no gate record at all → full dispatch success). Walking-on-click resumes for all 3 locs.

**Routing if smoke does NOT advance gate:**
- All 3 still at `op_slot_empty` → real-cache regression test should have caught this; check what unit pinned vs what the cache actually contains. Bin-decode loc 3014 directly.
- Different gate fires (e.g., `getloc_nil`, `viewport`, `delayed`) → genuine second blocker exposed; per `smoke_unchanged_means_multiple_blockers.md`, route to NAI-81 brainstorm with the new gate name as the routing target.
- Gate=`script_dispatch` but no walking → SetInteraction reaching but pathing/visual blocked downstream; route to a movement-side investigation.

---

## §8. Build sequence (compressed cadence — implementation steps)

Implementation is a single subagent-driven-development bundle, no inter-task dependencies that need staging:

1. **T1 — Struct + defaults.** Extend `LocType` struct with all TS render fields; update `NewLocType` to set TS defaults. No decoder changes yet. Verify package still compiles (zero new fields are referenced; default values land in struct literals).
2. **T2 — Decode arms.** Add all new `case` arms to `LocType.Decode`. Per arm: byte-arithmetic mirrors TS line-by-line. Add per-arm unit test alongside.
3. **T3 — PostDecode.** Add `PostDecode` method and wire into `parseLocTypes`. Add `TestPostDecode_ActiveInference` (3 branches + sanity).
4. **T4 — Dual-pass loader.** Modify `LoadLocTypes` and `parseLocTypes` to load+thread `clientJag`. Add `TestParseLocTypes_DualPass` and `TestParseLocTypes_HiddenCoercion`.
5. **T5 — Top-of-file comment update.** Retire "this server-only loader skips" sentence; replace with TS-source pointer + D1 follow-up note.
6. **T6 — Real-cache regression.** Add `loctype_realcache_test.go` with `TestLoadLocTypes_RealCache_CascadeBlockerLocs` + ID-shift sanity pin.
7. **T7 — Verification.** Full `go test ./pkg/objtype/...` clean; full `go test ./...` clean (no consumer break). Verify `go build ./...`.
8. **T8 — Smoke handoff.** Close commit body cites `Closes memory: nai80_seed_loctype_op_empty.md` and the predecessor's H4 commit (`9a96b8b`). Hand off smoke task to user with the script in §7.3.

Per `controller_preflight.md`: before each task dispatch, controller spends ~30s grepping the named symbols (`G1B`, `G2S`, `LoadJagfile`, `Read`, `buildMinimalJagfile`) at HEAD to confirm signature shape before writing implementer code blocks. Per `plan_helper_coverage.md`: re-grep for any test helpers that synthesize `LocType` literals (e.g., in `modules/world/*_test.go`) so new struct fields don't break existing fixtures via missing zero-value fields — tests that use `&LocType{Width: 1}` style literals continue compiling because Go zero-init handles new fields, but any literal that *expects* zero values for newly-defaulted fields (e.g., test code that asserts `lt.WallWidth == 0` after default construction) needs updating. Per `plan_enumerate_struct_literals.md`: spec mandates the implementer grep `LocType{` across `pkg/`, `modules/`, `cmd/` and report any literals that need updates before T1 lands.

---

## §9. Risk register

| Risk | Severity | Mitigation |
|---|---|---|
| Byte-arithmetic miscount in any new Decode arm desyncs decoder for entire cache | High | Per-arm unit tests with TS-derived golden bytes; real-cache test would surface gross desyncs (panic or unknown-code error during full load); implementer cross-references TS line-by-line per arm. |
| `Active` postDecode coupling to Shapes/Op surprises future LC_* handler ports | Medium | Spec ships PostDecode + 3-branch test; TS comment cited in Decode-method doc. |
| `LocType{...}` struct literals scattered across test fixtures break with new fields | Low | Go zero-init tolerates new fields; plan §8 mandates pre-T1 grep for literals that *assert* zero-value defaults. |
| `data/pack/client/config` jagfile drift between Engine-TS and goscape repo | Low | Sizes match (132,952 B both). Real-cache test soft-skips on absence; if values drift, test surfaces immediately. |
| `"hidden"→""` D1 coercion masks an unrelated dispatch divergence at smoke | Low-Medium | Documented in §6 with explicit follow-up routing; smoke is content-driven and would surface symptoms. |
| `clientJag.Read("loc.dat")` returns short or empty entry for some cache version | Low | Error propagated with attributable message ("loc id N (client): unrecognized code M"). |

---

## §10. Acceptance criteria

1. `go test ./pkg/objtype/...` passes including all new per-code-arm tests + `TestPostDecode_*` triple + `TestParseLocTypes_DualPass` + `TestParseLocTypes_HiddenCoercion`.
2. `TestLoadLocTypes_RealCache_CascadeBlockerLocs` passes against `data/pack` (loc 3014/380/350 all have non-empty `Op[0]`).
3. `go test ./...` and `go build ./...` clean — no consumer break (existing `handler_oploc.go`, `npc_hunt_entities.go`, `script/handlers_loc.go` continue compiling and passing).
4. `pkg/objtype/loctype.go` top-of-file comment updated to retire the "server-only loader skips" sentence.
5. Smoke handoff issued to user with §7.3 script.
6. Close commit carries `Closes memory: nai80_seed_loctype_op_empty.md` trailer.

---

## §11. Out-of-scope follow-ups

- **NAI-80-D1 follow-up sub-spec**: TS OPLOC dispatch behavior on `"hidden"` slots; route on finding (reject vs dispatch) to either drop coercion or retain.
- **LC_* handler ports** that consume the new struct fields (Active, BlockWalk, Anim, Models/Shapes, RecolS/D, etc.) — each lands per its own NAI-N when the script-handler triggering it gets ported.
- **`player_uid=-1` adjacent observation** — flagged in §1; not a NAI-80 deliverable. If a downstream uid-mismatch surfaces post-fix, route to a fresh sub-spec.
- **Tutorial-Island full smoke chain** — once the cascade-blocker clears, expect the NAI-79→78→77 chain's downstream interactions to surface their own issues (per `cascade_theory_smoke_binding.md`). Each gets routed individually.
