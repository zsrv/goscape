# NAI-57 — SeqType Config Port + NAI-56-D1 Retirement

## Motivation

`(*Player).PlayAnim` (NAI-56) shipped the TS L1842 `animProtect` early-return
but left the remaining two gates of TS `playAnimation` unported under
deviation **NAI-56-D1**:

- `anim >= SeqType.count` bounds-reject (TS `Player.ts:1841`, `Npc.ts:452`)
- priority-comparison overwrite gate (TS `Player.ts:1846`, `Npc.ts:457`)

Both consumers (`(*Player).PlayAnim` at `modules/world/player_script.go:547`
and `(*Npc).Animate` at `modules/world/npc_masks.go:8`) need access to a
`SeqType` config registry which is currently absent from goscape. This
sub-spec ports the registry (`SeqType` + its `SeqFrame` delay-only
dependency) and wires the two gates, retiring NAI-56-D1.

Bundled drive-by per `dead_api_polish.md`: retire `(*Player).Animate` at
`modules/world/player_masks.go:8` — a parallel un-gated entry point with
zero production callers (single test-only consumer in `player_masks_test.go`).
Leaving it would create a second method in the same package that bypasses
the new gates, inviting silent TS-divergence next time someone greps
"Player Animate".

**TS reference:** `Engine-TS/src/cache/config/SeqType.ts` (133 LOC),
`Engine-TS/src/cache/config/SeqFrame.ts` (43 LOC), and the two consumer
methods at `Engine-TS/src/engine/entity/Player.ts:1840-1851` /
`Engine-TS/src/engine/entity/Npc.ts:451-462`.

## Tech Stack

**Go 1.26+** (per `go_version.md`; modern Go via `use-modern-go` skill).

## Deviations

| Tag | Status | Notes |
|-----|--------|-------|
| **NAI-56-D1** | **Retired** | `anim >= SeqType.count` bounds-reject + priority-comparison gate now wired in `(*Player).PlayAnim` and `(*Npc).Animate` |

No new deviations opened. Decode codes 6 and 7 (`replaceheldleft`,
`replaceheldright`) and code 4 (`stretches`) are parsed and stored
faithfully but currently have zero consumers — that is TS-faithful
struct shape, not a deviation.

**Tally:** 21 → 20.

## Scope

**In scope:**

- `pkg/objtype/seqframe.go` — `SeqFrame` struct + `SeqFrameConfigs` registry + `LoadSeqFrames(dir)` loader
- `pkg/objtype/seqframe_test.go` — decode + parse unit tests
- `pkg/objtype/seqtype.go` — `SeqType` struct + `Decode` opcode dispatch + `SeqTypeConfigs` registry + `LoadSeqTypes(dir, frames)` loader
- `pkg/objtype/seqtype_test.go` — per-opcode decode + parse unit tests including SeqFrame.delay fallback (TS L97)
- `modules/world/server.go` — add `seqFrames` and `seqTypes` fields + load wire-in
- `modules/world/player.go` — add `seqTypes` field + seed in `newPlayer` from `c.server.seqTypes`
- `modules/world/player_masks.go` — delete `(*Player).Animate`
- `modules/world/player_masks_test.go` — delete the 2 obsolete `(*Player).Animate` tests
- `modules/world/player_script.go` — extend `(*Player).PlayAnim` with bounds-reject + priority gate
- `modules/world/player_anim_test.go` — extend with bounds-reject + priority arms; seed existing tests with `buildSeqTypes(...)`
- `modules/world/npc_masks.go` — extend `(*Npc).Animate` with bounds-reject + priority gate
- `modules/world/npc_test.go` — extend with bounds-reject + priority arms

**Out of scope (deferred):**

- `script.Configs.SeqType(id)` interface seam — no script handler needs direct
  registry access today (`handleAnim` calls `s.Self.PlayAnim`, which is the
  gate point). YAGNI per `dead_api_polish.md`. Add when a future SEQ-reading
  opcode lands.
- Loading transforms / non-delay SeqFrame fields — TS `SeqFrame` is also
  intentionally a "partial" loader (`// partial frame class - only delays,
  not loading transforms`, TS L5). Goscape mirrors that.
- Encoding / wire-format use of SeqType beyond the playAnimation gate.

---

## Pre-flight (verified at HEAD `6950942`)

| Claim | Result |
|---|---|
| `(*Player).PlayAnim` at `modules/world/player_script.go:547` with `animProtect` gate only | ✓ |
| `(*Npc).Animate` at `modules/world/npc_masks.go:8`, no gates | ✓ |
| `(*Player).Animate` at `modules/world/player_masks.go:8`, ungated | ✓ |
| Production callers of `(*Player).Animate`: zero (`s.ActiveNpc.Animate` at `pkg/script/handlers_npc.go:243` is the Npc method) | ✓ |
| Test callers of `(*Player).Animate`: 2 in `player_masks_test.go:11,88` | ✓ |
| `Player.animID` initialised to `-1` at `modules/world/player.go:403` | ✓ |
| `Npc.animID` initialised to `-1` at `modules/world/npc.go:184` | ✓ |
| `SeqType` / `SeqFrame` symbols absent from `pkg/objtype/`, `modules/world/`, `cmd/goscape/` | ✓ absent |
| Server data files staged: `data/pack/server/{seq.dat,seq.idx,frame_del.dat}` (sizes 18415 / ? / 7783 bytes) | ✓ |
| `pkg/objtype/idktype.go` template available for ConfigType + dual-source loader pattern | ✓ |
| `pkg/objtype/configtype.go` exposes `ConfigType` base + `DecodeType(buf, decoder)` polymorphic helper | ✓ |
| `Player.client.server` reachable for `newPlayer` seeding (mirrors NAI-46 `c.server.invTypes` precedent at `client.go:121-126`) | ✓ |
| `n.server *Server` back-reference at `modules/world/npc.go:67` | ✓ |
| Sole script-handler caller path: `handleAnim` (`pkg/script/handlers_player.go:543`) → `s.Self.PlayAnim` | ✓ — no Configs interface change needed |
| `mockPlayer.PlayAnim` in `pkg/script/runner_test.go:389` is a recording mock that bypasses the real gate | ✓ — script-package tests unaffected |

---

## Task 1 — `pkg/objtype/seqframe.go` + tests

Port `Engine-TS/src/cache/config/SeqFrame.ts` (the partial delay-only loader).

### Struct + registry

```go
package objtype

import (
    "errors"
    "os"
    "path/filepath"

    "github.com/zsrv/goscape/pkg/io/packet"
)

// SeqFrame is the delay-only portion of a single seq frame record.
// Mirrors Engine-TS/src/cache/config/SeqFrame.ts (loader is partial by
// design — TS comment: "only delays, not loading transforms").
type SeqFrame struct {
    Delay int
}

// SeqFrameConfigs holds all parsed frame records, indexed by frame id.
// TS exposes this as `SeqFrame.instances` static.
type SeqFrameConfigs struct {
    Instances []*SeqFrame
}

// LoadSeqFrames reads data/server/frame_del.dat. Each byte in the file
// is one frame's delay (g1 per byte). Silent-on-missing-file matches
// TS SeqFrame.load.
func LoadSeqFrames(dir string) (*SeqFrameConfigs, error) {
    dat, err := packet.Load(filepath.Join(dir, "server", "frame_del.dat"), false)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return &SeqFrameConfigs{}, nil
        }
        return nil, err
    }
    return parseSeqFrames(dat), nil
}

func parseSeqFrames(dat *packet.Packet) *SeqFrameConfigs {
    n := len(dat.Data)
    instances := make([]*SeqFrame, n)
    for i := range n {
        instances[i] = &SeqFrame{Delay: int(dat.G1())}
    }
    return &SeqFrameConfigs{Instances: instances}
}
```

### Test list

`pkg/objtype/seqframe_test.go`:

- `TestParseSeqFrames_EmptyBuffer` — zero-length packet → empty Instances, no error
- `TestParseSeqFrames_DelaysSequential` — 4-byte buffer `{1, 2, 3, 4}` → Instances[0..3].Delay = 1..4
- `TestLoadSeqFrames_MissingFileSilent` — point at a tmp dir without `server/frame_del.dat` → empty registry, nil error

---

## Task 2 — `pkg/objtype/seqtype.go` + tests

Port `Engine-TS/src/cache/config/SeqType.ts` following the dual-source
(server + client jag) pattern from `idktype.go`.

### Struct + constructor

```go
package objtype

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"

    io "github.com/zsrv/goscape/pkg/io/jagfile"
    "github.com/zsrv/goscape/pkg/io/packet"
)

// SeqType is a single seq.dat config record (animation sequence).
// Mirrors Engine-TS/src/cache/config/SeqType.ts.
type SeqType struct {
    ConfigType
    Frames           []int32 // nil = unset
    IFrames          []int32 // nil = unset; 65535 in source is normalised to -1 per TS L92
    Delay            []int32 // per-frame delay; 0 → SeqFrame.Delay fallback per TS L97
    Loops            int     // -1 default
    WalkMerge        []int32 // nil = unset
    Stretches        bool
    Priority         int     // 5 default
    ReplaceHeldLeft  int     // -1 default
    ReplaceHeldRight int     // -1 default
    MaxLoops         int     // 99 default
    Duration         int     // sum of Delay (for non-zero entries; post-fallback)
}

// NewSeqType returns a SeqType with TS-faithful defaults.
func NewSeqType(id int) *SeqType {
    return &SeqType{
        ConfigType:       ConfigType{ID: id},
        Loops:            -1,
        Priority:         5,
        ReplaceHeldLeft:  -1,
        ReplaceHeldRight: -1,
        MaxLoops:         99,
    }
}
```

### Decode

`Decode` is a method that takes the SeqFrame registry as a parameter so
the L97 fallback can resolve. The polymorphic `DecodeType` helper expects a
`ConfigTypeDecoder`-compatible signature, so wrap the SeqFrame-aware
`decode` method in a closure when the SeqFrame dependency is needed; pass
`nil` when only basic codes are exercised.

Approach (cleanest, one-call-site): expose a `Decode(code uint8, dat *packet.Packet)`
method on `*SeqType` that resolves the fallback through a back-reference
`SeqType.frames *SeqFrameConfigs` set during parse — i.e. the loader pre-populates
each `*SeqType` instance's frame-registry pointer before invoking `DecodeType`.

```go
// Decode dispatches on the seq config opcode, matching TS SeqType.decode
// at Engine-TS/src/cache/config/SeqType.ts:80-131.
func (t *SeqType) Decode(code uint8, dat *packet.Packet) error {
    switch code {
    case 1:
        count := int(dat.G1())
        t.Frames = make([]int32, count)
        t.IFrames = make([]int32, count)
        t.Delay = make([]int32, count)
        for i := range count {
            t.Frames[i] = int32(dat.G2())

            v := int32(dat.G2())
            if v == 65535 {
                v = -1 // TS L92 normalisation
            }
            t.IFrames[i] = v

            d := int32(dat.G2())
            if d == 0 {
                // TS L97: fallback to SeqFrame.instances[frames[i]].delay.
                // Resolved via t.frames back-reference set by parseSeqTypes.
                if t.frames != nil && int(t.Frames[i]) < len(t.frames.Instances) {
                    d = int32(t.frames.Instances[t.Frames[i]].Delay)
                }
            }
            if d == 0 {
                d = 1 // TS L101
            }
            t.Delay[i] = d
            t.Duration += int(d)
        }
    case 2:
        t.Loops = int(dat.G2())
    case 3:
        count := int(dat.G1())
        t.WalkMerge = make([]int32, count+1)
        for i := range count {
            t.WalkMerge[i] = int32(dat.G1())
        }
        t.WalkMerge[count] = 9999999 // TS L116
    case 4:
        t.Stretches = true
    case 5:
        t.Priority = int(dat.G1())
    case 6:
        t.ReplaceHeldLeft = int(dat.G2())
    case 7:
        t.ReplaceHeldRight = int(dat.G2())
    case 8:
        t.MaxLoops = int(dat.G1())
    case 250:
        t.DebugName = dat.GJStrLF()
    default:
        return fmt.Errorf("unrecognized seq config code %d", code)
    }
    return nil
}
```

`SeqType` carries an unexported `frames *SeqFrameConfigs` back-reference
populated by `parseSeqTypes`. It is intentionally not part of the
struct-literal initialisation surface — only the parser sets it. This
matches TS's static-class lookup from inside `decode`.

```go
// (unexported) — set by parseSeqTypes; carries the SeqFrame.instances
// reference for the L97 fallback inside Decode.
type SeqType struct {
    // ... public fields above ...
    frames *SeqFrameConfigs
}
```

### Registry + loader

```go
type SeqTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*SeqType
}

// Count returns len(Configs) as the TS-equivalent SeqType.count static
// (Player.ts L1841 / Npc.ts L452 read this directly). Returns 0 on a
// nil receiver so consumers don't have to nil-guard separately.
func (c *SeqTypeConfigs) Count() int {
    if c == nil {
        return 0
    }
    return len(c.Configs)
}

// LoadSeqTypes parses server/seq.dat + client/config jag → seq.dat into
// a SeqTypeConfigs registry. The frames argument is captured by each
// *SeqType for the L97 delay-fallback. Returns an empty registry with
// nil error when server/seq.dat is absent (silent-on-missing).
func LoadSeqTypes(dir string, frames *SeqFrameConfigs) (*SeqTypeConfigs, error) {
    server, err := packet.Load(filepath.Join(dir, "server", "seq.dat"), false)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return &SeqTypeConfigs{ConfigNames: map[string]int{}}, nil
        }
        return nil, err
    }

    clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
    if err != nil {
        return nil, err
    }

    return parseSeqTypes(server, clientJag, frames)
}

func parseSeqTypes(server *packet.Packet, clientJag *io.Jagfile, frames *SeqFrameConfigs) (*SeqTypeConfigs, error) {
    count := int(server.G2())
    configs := make([]*SeqType, count)
    configNames := make(map[string]int, count)

    client, err := clientJag.Read("seq.dat")
    if err != nil {
        return nil, err
    }
    client.Pos = 2 // skip client-side count header (matches idktype.go pattern)

    for id := range count {
        config := NewSeqType(id)
        config.frames = frames
        if err := DecodeType(server, config); err != nil {
            return nil, err
        }
        if err := DecodeType(client, config); err != nil {
            return nil, err
        }
        configs[id] = config
        if config.DebugName != "" {
            configNames[config.DebugName] = id
        }
    }

    return &SeqTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

### Test list

`pkg/objtype/seqtype_test.go`:

- `TestNewSeqTypeDefaults` — `NewSeqType(7)` → ID=7, Loops=-1, Priority=5,
  ReplaceHeldLeft=-1, ReplaceHeldRight=-1, MaxLoops=99, all slices nil
- `TestSeqTypeDecode_Frames` — code 1 + count=2 + 6 g2 reads → Frames, IFrames, Delay populated
- `TestSeqTypeDecode_IFrames65535ToMinusOne` — code 1 with iframes=65535 → IFrames[i]=-1
- `TestSeqTypeDecode_DelayZeroFallback` — code 1, delay=0, frames-pointer set, SeqFrame[frameID].Delay=7 → Delay[i]=7
- `TestSeqTypeDecode_DelayZeroNoFrameFallback` — code 1, delay=0, no SeqFrame fallback (nil frames or out-of-range) → Delay[i]=1 (TS L101)
- `TestSeqTypeDecode_DurationAccumulates` — code 1 with 3 frames → Duration = sum of post-fallback Delays
- `TestSeqTypeDecode_Loops` — code 2 + g2 → Loops
- `TestSeqTypeDecode_WalkMerge` — code 3 + count=2 → WalkMerge[2]=9999999 sentinel
- `TestSeqTypeDecode_Stretches` — code 4 → Stretches=true
- `TestSeqTypeDecode_Priority` — code 5 + g1=3 → Priority=3
- `TestSeqTypeDecode_ReplaceHeldLeft` — code 6 + g2 → ReplaceHeldLeft
- `TestSeqTypeDecode_ReplaceHeldRight` — code 7 + g2 → ReplaceHeldRight
- `TestSeqTypeDecode_MaxLoops` — code 8 + g1 → MaxLoops
- `TestSeqTypeDecode_DebugName` — code 250 + null-terminated string → DebugName
- `TestSeqTypeDecode_UnknownCode` — code 99 → error returned
- `TestParseSeqTypes_EmptyServerCount` — server G2=0, dummy client jag → empty Configs, nil error
- `TestLoadSeqTypes_MissingFileSilent` — tmp dir without `server/seq.dat` → empty registry, nil error

---

## Task 3 — Server / Player wire-in

### 3a. `modules/world/server.go`

Add fields alongside `idkTypes`:

```go
seqFrames *objtype.SeqFrameConfigs
seqTypes  *objtype.SeqTypeConfigs
```

In the bootstrap path that already loads `idkTypes` (currently around
`server.go:235`), add — sequenced so frames load before types:

```go
seqFrames, err := objtype.LoadSeqFrames(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load seq frames: %w", err)
}
s.seqFrames = seqFrames

seqTypes, err := objtype.LoadSeqTypes(cfg.CachePath, seqFrames)
if err != nil {
    return nil, fmt.Errorf("load seq types: %w", err)
}
s.seqTypes = seqTypes
```

### 3b. `modules/world/player.go`

Add field on `Player` struct (alongside other registry-pointer fields if
present, otherwise next to `appearanceInv` for visual proximity to anim
state):

```go
seqTypes *objtype.SeqTypeConfigs // seeded at newPlayer; gates PlayAnim
```

The field zero-value (nil) is fine in the struct literal — no explicit
seed line needed. After the `newPlayer(c *client)` struct literal returns
the `*Player`, mirror the existing nil-guarded server-access block at
`client.go:121-126`:

```go
if c.server != nil {
    p.seqTypes = c.server.seqTypes
}
```

The conditional shape preserves the existing test-path invariant that
`newPlayer` may be invoked with `c.server == nil` (test fixtures via
`newTestPlayer`). Tests that need the registry seed it directly on `p`
or via `s.seqTypes` after constructing.

### 3c. No `script.Configs` change

Per spec § Out of scope — `Configs` interface is unchanged. `mockPlayer`
in `pkg/script/runner_test.go` unchanged.

---

## Task 4 — Retire `(*Player).Animate`

### 4a. `modules/world/player_masks.go`

Delete the `Animate` method entirely:

```go
// DELETE:
func (p *Player) Animate(id, delay int) {
    p.animID = id
    p.animDelay = delay
    p.masks |= rsbuf.MaskAnim
}
```

### 4b. `modules/world/player_masks_test.go`

Delete the 2 tests at lines 11 and 88 that exercise `(*Player).Animate`.
If those are the only tests in the file, delete the file. Verify with
`grep -n "p\.Animate\b" modules/world/player_masks_test.go` at
implementation time (and re-verify post-T4) that no other test references
remain.

### 4c. Pre-flight re-check at T4 dispatch

Re-grep `(*Player)\.Animate\(` and `\bp\.Animate\(` against HEAD before
implementing T4. Any new caller introduced between spec-write and T4
dispatch flips this from drive-by retirement to API-conversion (rewrite
the new caller to `PlayAnim`, since gate parity is the goal). If no new
callers: proceed with deletion.

---

## Task 5 — `(*Player).PlayAnim` bounds + priority gate

### 5a. Edit `modules/world/player_script.go`

Replace the current 3-statement body with the full TS L1841-L1851 port:

```go
// PlayAnim schedules sequence seqID with the given client-side delay on
// the player's primary animation slot. seqID=-1 clears. Mirrors TS
// Player.playAnimation (Player.ts:1840-1851): bounds-reject on
// seqID >= SeqType.count, animProtect early-return, and priority-comparison
// overwrite gate. Closes deviation NAI-56-D1.
func (p *Player) PlayAnim(seqID, delay int) {
    if seqID >= p.seqTypes.Count() || p.animProtect != 0 {
        return // TS Player.ts:1841
    }
    if seqID == -1 || p.animID == -1 ||
        p.seqTypes.Configs[seqID].Priority > p.seqTypes.Configs[p.animID].Priority ||
        p.seqTypes.Configs[p.animID].Priority == 0 {
        p.animID = seqID
        p.animDelay = delay
        p.masks |= rsbuf.MaskAnim
    }
}
```

`SeqTypeConfigs.Count()` returns 0 on a nil receiver, so the bounds-reject
fires correctly when `p.seqTypes == nil` (test fixtures that haven't
seeded the registry). The priority short-circuit `seqID == -1 ||
p.animID == -1` guards the slice indexes — TS-faithful (TS does the
same short-circuit before the slice dereference). The bounds-reject
`seqID >= Count()` ensures `Configs[seqID]` is in-range whenever the
priority arm reaches it; `p.animID` is invariant-bounded by the prior
PlayAnim call that set it (and the `-1` short-circuit covers fresh
state).

Refresh the doc-comment on `(*Player).SetAnimProtect` at
`player_script.go:813` — drop the trailing reference to "the L1842
gate" if the line numbers shifted, but keep the NAI-56 reference.

### 5b. Edit `modules/world/player_anim_test.go`

The two existing tests (`TestPlayAnim_AnimProtectBlocksWrite` and
`TestPlayAnim_AnimProtectZeroAllowsWrite`) need fixture seeding so
seqID=123 is in-range. Both cases:

```go
p.seqTypes = buildSeqTypes(124) // seqID=123 valid; default Priority=5 each
```

Add new tests covering the bounds-reject and priority arms (all use a
non-nil seeded registry; `p.animProtect = 0` unless noted):

- `TestPlayAnim_BoundsRejectAtCount` — `p.seqTypes = buildSeqTypes(50)`, call `p.PlayAnim(50, 5)` → no-op (animID unchanged at -1, no MaskAnim)
- `TestPlayAnim_BoundsRejectAboveCount` — `p.seqTypes = buildSeqTypes(50)`, call `p.PlayAnim(99, 5)` → no-op
- `TestPlayAnim_NilRegistryRejectsAllNonClear` — `p.seqTypes = nil`, call `p.PlayAnim(0, 5)` → no-op (count=0)
- `TestPlayAnim_NilRegistryAllowsClear` — `p.seqTypes = nil`, set
  `p.animID = -1` (Player default; registry never loaded) then call
  `p.PlayAnim(-1, 0)`. Bounds: `-1 >= 0` is false → bounds-OK. Priority
  arm: `seqID == -1` short-circuits (Go `||` left-to-right), so the
  slice deref of `p.seqTypes.Configs[...]` is never evaluated. animID
  stays -1, MaskAnim set. Pins the bypass-on-clear-with-no-registry
  invariant.
- `TestPlayAnim_PriorityHigherOverwrites` — seed registry where `seqTypes.Configs[5].Priority = 3`, `Configs[10].Priority = 7`. Set `p.animID = 5`, call `p.PlayAnim(10, 3)` → animID=10, MaskAnim set
- `TestPlayAnim_PriorityLowerRejected` — seed registry where `Configs[5].Priority = 7`, `Configs[10].Priority = 3`. Set `p.animID = 5`, call `p.PlayAnim(10, 3)` → animID stays 5, MaskAnim NOT newly set (clear `p.masks` before the call to make the assertion crisp)
- `TestPlayAnim_PriorityEqualRejected` — both priorities = 5 (TS default). Set `p.animID = 5`, call `p.PlayAnim(10, 3)` → animID stays 5 (TS uses strict `>`)
- `TestPlayAnim_CurrentAnimZeroPriorityOverwrites` — `Configs[5].Priority = 0` (the zero-priority sentinel), `Configs[10].Priority = 5`. Set `p.animID = 5`, call `p.PlayAnim(10, 3)` → animID=10 (TS L1846 third disjunct)
- `TestPlayAnim_FreshAnimIDMinusOneAlwaysOverwrites` — registry seeded but `p.animID = -1` (initial). Call `p.PlayAnim(10, 3)` → animID=10
- `TestPlayAnim_AnimProtectBlocksAboveBounds` — sanity: with both bounds-fail AND animProtect=1, still no-op (the gate's `||` ordering makes either condition sufficient)

Existing animProtect tests need to keep `seqID=123` in-range (seed
`buildSeqTypes(124)`).

### 5c. `buildSeqTypes` helper

Place in `modules/world/player_anim_test.go` (or a shared `_test.go` file
if a sibling test wants to reuse it — Task 6 will). Modeled after
`buildIdkTypes` at `handler_interface_test.go:303`:

```go
// buildSeqTypes returns a SeqTypeConfigs with count entries.
// Each entry has Priority=5 (TS default) and DebugName empty. Tests
// that exercise the priority-comparison arm override per-entry priority
// before invoking PlayAnim/Animate.
func buildSeqTypes(count int) *objtype.SeqTypeConfigs {
    configs := make([]*objtype.SeqType, count)
    for i := range count {
        configs[i] = objtype.NewSeqType(i)
    }
    return &objtype.SeqTypeConfigs{
        ConfigNames: map[string]int{},
        Configs:     configs,
    }
}
```

`NewSeqType` already defaults Priority=5, so per-entry priority overrides
are simply `configs[5].Priority = 3` after the helper returns.

---

## Task 6 — `(*Npc).Animate` bounds + priority gate + close

### 6a. Edit `modules/world/npc_masks.go`

Symmetric port of TS `Npc.playAnimation` at `Npc.ts:451-462`. NPC has no
`animProtect` flag (TS-faithful — the field is Player-only). Registry
access via `n.server.seqTypes`:

```go
func (n *Npc) Animate(id, delay int) {
    if n.server == nil {
        return // defensive: covers test-only Npcs constructed without a server backref
    }
    count := n.server.seqTypes.Count()
    if id >= count {
        return // TS Npc.ts:452
    }
    if id == -1 || n.animID == -1 ||
        n.server.seqTypes.Configs[id].Priority > n.server.seqTypes.Configs[n.animID].Priority ||
        n.server.seqTypes.Configs[n.animID].Priority == 0 {
        n.animID = id
        n.animDelay = delay
        n.masks |= rsbuf.NpcMaskAnim
    }
}
```

The `n.server == nil` guard is a goscape testing concession (some NPC
test fixtures construct `&Npc{}` without registering through `addNpc`).
TS has no equivalent guard — its registry is static. Document in the
doc-comment as a goscape-only nil-guard, not a TS divergence.

### 6b. Edit `modules/world/npc_test.go`

Existing `n.Animate(123, 5)` callers at lines 33 and 195 need
fixture seeding. The npc test fixture pattern uses `newTestServer(t)` +
`addNpc`, so `n.server.seqTypes` is reachable — seed the server's
registry before calling Animate:

```go
s := newTestServer(t)
s.seqTypes = buildSeqTypes(200) // covers id=123 used in test
// ... existing addNpc / Animate pattern continues unchanged ...
```

For tests that construct a bare `&Npc{}` without `n.server` set, either
seed `n.server = newTestServer(t); n.server.seqTypes = buildSeqTypes(...)`
or rely on the nil-guard early-return (only acceptable for tests
asserting unrelated invariants — flag at TDD review).

Add new tests symmetric to T5 (Npc has no animProtect arm):

- `TestNpcAnimate_BoundsRejectAtCount` — count=50, id=50 → no-op
- `TestNpcAnimate_NilServerEarlyReturn` — `n.server = nil`, id=0 → no-op (no panic)
- `TestNpcAnimate_PriorityHigherOverwrites` — seed Configs[5].Priority=3, Configs[10].Priority=7; n.animID=5; Animate(10,3) → 10
- `TestNpcAnimate_PriorityLowerRejected` — Configs[5].Priority=7, Configs[10].Priority=3; n.animID=5; Animate(10,3) → 5
- `TestNpcAnimate_PriorityEqualRejected` — both 5; → 5 (rejected)
- `TestNpcAnimate_CurrentZeroPriorityOverwrites` — Configs[5].Priority=0; n.animID=5; Animate(10,3) → 10
- `TestNpcAnimate_FreshAnimIDMinusOneAlwaysOverwrites` — n.animID=-1; Animate(10,3) → 10
- `TestNpcAnimate_ClearWithMinusOneSucceeds` — n.animID=5 (registry seeded); Animate(-1,0) → -1, MaskAnim set

### 6c. Close commit (T6 final)

After all green:

1. **Stale-deviation grep** — `rg "NAI-56-D1" pkg/ modules/ cmd/` per
   `retire_deviation_grep_all_comments.md`. Surface every stale doc-comment
   reference and edit out. Expected sites at HEAD: at least
   `modules/world/player_script.go:543-547` (the NAI-56 doc-comment block
   that mentions "the remaining TS gates ... tracked as NAI-56-D1") plus
   the `(*Npc).Animate` doc-comment if it picked up a similar narration.
   Re-grep zero-hit before committing.

2. **Tally update** — verify final deviation count via the active deviation
   list (currently at 21 entering NAI-57); expected post-close: 20.

3. **Memory updates** — append NAI-57 entry to `nai_followups.md` summarising
   the close (analogous to NAI-56's section). Mark NAI-56-D1 as Resolved
   with closure pointer to NAI-57.

4. **Close commit message** — include trailer:
   ```
   Closes memory: NAI-56-D1
   ```

---

## Bundle / cadence

Single bundle, 6 tasks. Full sub-spec (not compressed; LOC > 15 by an
order of magnitude). NAI-46 IdkType-shaped.

Estimated LOC at close: ~150 production + ~120 test. Each task ships
green at HEAD; no inter-task rebases planned.

Implementation mode: **subagent-driven-development** (per
`execution_mode_default.md` memory).

## Memory entries that apply

- `dead_api_polish.md` — drives Task 4 retirement of `(*Player).Animate`.
- `retire_deviation_grep_all_comments.md` — drives Task 6c grep at close.
- `enumerate_all_sites.md` — drives Task 4c pre-flight re-grep at T4 dispatch.
- `controller_preflight.md` — drives the per-task pre-flight grep+Read
  pass against HEAD before each implementer dispatch.
- `verify_implementer_claims.md` — drives the 30-second post-commit
  verification at each task close.
- `plan_runnable_test_fixtures.md` — drives mental-execution of the
  buildSeqTypes-seeded tests at plan-author time.
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:
  NAI-56-D1`.
- `session_context_management.md` — natural `/compact` boundary at NAI-57
  close.
- `true_to_ts_gate.md` — every behavioural divergence (the goscape-only
  `n.server == nil` nil-guard in `(*Npc).Animate`) is documented in its
  doc-comment.
