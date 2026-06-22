# NAI-57 — SeqType Config Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `SeqType` (and its `SeqFrame` delay-only dependency) from Engine-TS into `pkg/objtype/`, wire `(*Player).PlayAnim` and `(*Npc).Animate` with the TS bounds-reject + priority-comparison gates, and retire the un-gated `(*Player).Animate` parallel entry point. Closes deviation `NAI-56-D1`.

**Architecture:** New `pkg/objtype/seqframe.go` and `pkg/objtype/seqtype.go` follow the dual-source loader pattern established by `idktype.go` (server/seq.dat + Jagfile client/config → seq.dat). `SeqFrame` is a single-pass byte-stream parser (no opcode protocol). `SeqType` carries an unexported `frames *SeqFrameConfigs` back-reference set during parse so its `Decode` can resolve the TS L97 delay-fallback. Both registries are stored on `*Server`. `*Player` gains a `seqTypes *objtype.SeqTypeConfigs` field seeded conditionally inside `newPlayer`. `*Npc` reads via the existing `n.server *Server` back-reference. The `script.Configs` interface is **unchanged** (YAGNI — `mockPlayer.PlayAnim` is a recording mock that bypasses the real gate).

**Tech Stack:** Go 1.26+ with modern syntax (`for i := range N`). All `go` commands must be prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`. All commits use `--no-gpg-sign`.

---

## File Map

| Action | Path | What changes |
|--------|------|-------------|
| CREATE | `pkg/objtype/seqframe.go` | `SeqFrame` + `SeqFrameConfigs` + `LoadSeqFrames` + `parseSeqFrames` |
| CREATE | `pkg/objtype/seqframe_test.go` | Decode + loader unit tests |
| CREATE | `pkg/objtype/seqtype.go` | `SeqType` + `NewSeqType` + `Decode` + `SeqTypeConfigs` + `Count` + `LoadSeqTypes` + `parseSeqTypes` |
| CREATE | `pkg/objtype/seqtype_test.go` | Per-opcode decode + loader unit tests |
| MODIFY | `modules/world/server.go:85-87` | Add `seqFrames` and `seqTypes` fields |
| MODIFY | `modules/world/server.go:235-239` | Add `LoadSeqFrames` + `LoadSeqTypes` calls after `idkTypes` load |
| MODIFY | `modules/world/player.go` (anim-state field block + newPlayer body) | Add `seqTypes` field + conditional seed from `c.server.seqTypes` |
| MODIFY | `modules/world/player_masks.go:8-12` | Delete `(*Player).Animate` |
| MODIFY | `modules/world/player_masks_test.go` | Delete the 2 tests at lines 11 and 88 (or delete file if empty) |
| MODIFY | `modules/world/player_script.go:541-553` | Replace `PlayAnim` body with full bounds-reject + priority gate |
| MODIFY | `modules/world/player_anim_test.go` | Seed existing 2 tests with `buildSeqTypes(124)`; add 9 new tests; add `buildSeqTypes` helper |
| MODIFY | `modules/world/npc_masks.go:8-12` | Replace `(*Npc).Animate` body with bounds-reject + priority gate |
| MODIFY | `modules/world/npc_test.go` | Seed `s.seqTypes` for the 2 existing `Animate(123, 5)` callers; add 8 new gate tests |

---

## Pre-flight verification

Before starting Task 1, run this verification block. All must hold; if any fails, stop and report.

```bash
# 1. SeqType / SeqFrame symbols absent from goscape
rg -n "SeqType|SeqFrame" --type go pkg/ modules/ cmd/
# Expected: zero hits

# 2. PlayAnim has only the animProtect gate (NAI-56)
rg -n "func \(p \*Player\) PlayAnim" -A 6 modules/world/player_script.go
# Expected: 4-line body — animProtect early-return + 3 unconditional sets

# 3. (*Npc).Animate has zero gates
rg -n "func \(n \*Npc\) Animate" -A 4 modules/world/npc_masks.go
# Expected: 3-line body — animID, animDelay, masks

# 4. (*Player).Animate exists with zero production callers
rg -n "func \(p \*Player\) Animate" modules/world/player_masks.go
rg -n "\bp\.Animate\(" --type go pkg/ modules/ cmd/ | grep -v _test.go
# Expected: 1 method site; zero non-test callers

# 5. Data files staged
ls -la data/pack/server/seq.dat data/pack/server/frame_del.dat data/pack/client/config

# 6. animID initialised to -1 on both entities
rg -n "animID:\s*-1" modules/world/player.go modules/world/npc.go
# Expected: both files

# 7. Existing test counts (for post-T4 / post-T5 / post-T6 sanity)
rg -n "\bp\.Animate\(" modules/world/player_masks_test.go
rg -n "\bp\.PlayAnim\(" modules/world/player_anim_test.go
rg -n "\bn\.Animate\(" modules/world/npc_test.go
# Expected: 2 + 2 + 2

# 8. NAI-56-D1 doc-comment sites (for T6c grep-and-edit)
rg -n "NAI-56-D1" --type go --type md pkg/ modules/ cmd/ docs/
# Expected: at least player_script.go (PlayAnim doc-comment) + the spec
```

---

## Task 1 — `SeqFrame` port

**Files:**
- Create: `pkg/objtype/seqframe.go`
- Create: `pkg/objtype/seqframe_test.go`

TS reference: `Engine-TS/src/cache/config/SeqFrame.ts:28-43` (parse) and `:7-26` (load entrypoint).

`SeqFrame` is a partial loader by TS design (`// partial frame class - only delays, not loading transforms`). It does NOT use the `ConfigType` opcode dispatcher — it iterates byte-by-byte through `frame_del.dat`, where each byte is one frame's delay. So no `Decode(code, dat)` method; just a straight stream parse.

- [ ] **Step 1.1: Write failing tests for `parseSeqFrames` and `LoadSeqFrames`**

Create `pkg/objtype/seqframe_test.go`:

```go
package objtype

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestParseSeqFrames_EmptyBuffer(t *testing.T) {
	configs := parseSeqFrames(packet.NewPacket(nil))
	if configs == nil {
		t.Fatal("parseSeqFrames: got nil")
	}
	if len(configs.Instances) != 0 {
		t.Errorf("Instances len: got %d, want 0", len(configs.Instances))
	}
}

func TestParseSeqFrames_DelaysSequential(t *testing.T) {
	configs := parseSeqFrames(packet.NewPacket([]byte{1, 2, 3, 4}))
	if len(configs.Instances) != 4 {
		t.Fatalf("Instances len: got %d, want 4", len(configs.Instances))
	}
	for i, want := range []int{1, 2, 3, 4} {
		if configs.Instances[i].Delay != want {
			t.Errorf("Instances[%d].Delay: got %d, want %d", i, configs.Instances[i].Delay, want)
		}
	}
}

func TestLoadSeqFrames_MissingFile(t *testing.T) {
	dir := t.TempDir()
	configs, err := LoadSeqFrames(dir)
	if err != nil {
		t.Fatalf("LoadSeqFrames: want nil error on missing file, got %v", err)
	}
	if configs == nil {
		t.Fatal("configs: want non-nil registry, got nil")
	}
	if len(configs.Instances) != 0 {
		t.Errorf("Instances: want empty, got %d entries", len(configs.Instances))
	}
}

func TestLoadSeqFrames_FromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "frame_del.dat")); err != nil {
		t.Skipf("no pack data: %v", err)
	}
	configs, err := LoadSeqFrames(cacheDir)
	if err != nil {
		t.Fatalf("LoadSeqFrames: %v", err)
	}
	if len(configs.Instances) == 0 {
		t.Fatal("expected at least one SeqFrame, got 0")
	}
}
```

- [ ] **Step 1.2: Run tests — expect compile failure (`SeqFrame`, `SeqFrameConfigs`, `LoadSeqFrames`, `parseSeqFrames` undefined)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestParseSeqFrames|TestLoadSeqFrames' -v 2>&1 | head -20
```

Expected: compile error mentioning `SeqFrame` / `LoadSeqFrames` undefined.

- [ ] **Step 1.3: Create `pkg/objtype/seqframe.go`**

```go
package objtype

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// SeqFrame is the delay-only portion of a single seq frame record.
// Mirrors Engine-TS/src/cache/config/SeqFrame.ts. The TS class is
// intentionally partial (`// partial frame class - only delays, not loading
// transforms`); goscape preserves that shape.
type SeqFrame struct {
	Delay int
}

// SeqFrameConfigs holds all parsed frame records, indexed by frame id.
// TS exposes this as `SeqFrame.instances` static (Engine-TS/.../SeqFrame.ts:7).
type SeqFrameConfigs struct {
	Instances []*SeqFrame
}

// LoadSeqFrames reads data/server/frame_del.dat. Each byte in the file
// is one frame's delay (g1 per byte). Returns an empty registry with nil
// error when the file is absent (silent-on-missing, matching TS
// SeqFrame.load at Engine-TS/.../SeqFrame.ts:9-16).
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
	n := dat.Len()
	instances := make([]*SeqFrame, n)
	for i := range n {
		instances[i] = &SeqFrame{Delay: int(dat.G1())}
	}
	return &SeqFrameConfigs{Instances: instances}
}
```

- [ ] **Step 1.4: Run tests — expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestParseSeqFrames|TestLoadSeqFrames' -v 2>&1 | tail -20
```

Expected: 4 PASS (last test may be SKIP if pack data is absent).

- [ ] **Step 1.5: Commit**

```bash
git add pkg/objtype/seqframe.go pkg/objtype/seqframe_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-57 T1 — SeqFrame partial-loader port

Ports SeqFrame.ts (delay-only) into pkg/objtype/seqframe.go.
Iterates frame_del.dat byte-by-byte; silent-on-missing-file.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — `SeqType` port

**Files:**
- Create: `pkg/objtype/seqtype.go`
- Create: `pkg/objtype/seqtype_test.go`

TS reference: `Engine-TS/src/cache/config/SeqType.ts:80-131` (decode) and `:7-78` (struct + defaults + loader).

Pattern reference: `pkg/objtype/idktype.go` for the dual-source loader (`packet.Load(server) + jagfile.LoadJagfile(client) → DecodeType` loop) and `client.Pos = 2` for the count-header skip.

The `SeqType` struct carries an **unexported** `frames *SeqFrameConfigs` back-reference set by `parseSeqTypes` before each `DecodeType` call. This lets `SeqType.Decode` resolve the L97 delay-fallback (`SeqFrame.instances[frames[i]].delay`) without a global static.

- [ ] **Step 2.1: Write failing tests for `NewSeqType` and `Decode`**

Create `pkg/objtype/seqtype_test.go`:

```go
package objtype

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewSeqTypeDefaults(t *testing.T) {
	st := NewSeqType(7)
	if st.ID != 7 {
		t.Errorf("ID: got %d, want 7", st.ID)
	}
	if st.Loops != -1 {
		t.Errorf("Loops: got %d, want -1", st.Loops)
	}
	if st.Priority != 5 {
		t.Errorf("Priority: got %d, want 5", st.Priority)
	}
	if st.ReplaceHeldLeft != -1 {
		t.Errorf("ReplaceHeldLeft: got %d, want -1", st.ReplaceHeldLeft)
	}
	if st.ReplaceHeldRight != -1 {
		t.Errorf("ReplaceHeldRight: got %d, want -1", st.ReplaceHeldRight)
	}
	if st.MaxLoops != 99 {
		t.Errorf("MaxLoops: got %d, want 99", st.MaxLoops)
	}
	if st.Frames != nil || st.IFrames != nil || st.Delay != nil || st.WalkMerge != nil {
		t.Error("slice fields should be nil by default")
	}
	if st.Stretches {
		t.Error("Stretches: got true, want false")
	}
	if st.Duration != 0 {
		t.Errorf("Duration: got %d, want 0", st.Duration)
	}
}

// decodeSeq builds a writer packet, appends a 0-terminator, flips to reader,
// and runs DecodeType on a fresh NewSeqType(0) with the optional SeqFrame
// back-reference. Mirrors idktype_test's decodeIdk pattern.
func decodeSeq(frames *SeqFrameConfigs, build func(*packet.Packet)) (*SeqType, error) {
	w := packet.NewPacket(nil)
	build(w)
	w.P1(0) // terminator
	r := packet.NewPacket(w.Bytes())
	st := NewSeqType(0)
	st.frames = frames
	err := DecodeType(r, st)
	return st, err
}

func TestSeqTypeDecode_Frames(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(2)        // count = 2
		p.P2(0x0010)  // frames[0] = 16
		p.P2(0x0020)  // iframes[0] = 32
		p.P2(0x0003)  // delay[0] = 3
		p.P2(0x0011)  // frames[1] = 17
		p.P2(0x0021)  // iframes[1] = 33
		p.P2(0x0004)  // delay[1] = 4
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if got := st.Frames; len(got) != 2 || got[0] != 16 || got[1] != 17 {
		t.Errorf("Frames: got %v, want [16 17]", got)
	}
	if got := st.IFrames; len(got) != 2 || got[0] != 32 || got[1] != 33 {
		t.Errorf("IFrames: got %v, want [32 33]", got)
	}
	if got := st.Delay; len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Errorf("Delay: got %v, want [3 4]", got)
	}
	if st.Duration != 7 {
		t.Errorf("Duration: got %d, want 7 (3+4)", st.Duration)
	}
}

func TestSeqTypeDecode_IFrames65535ToMinusOne(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)        // count = 1
		p.P2(0x0001)  // frames[0] = 1
		p.P2(0xFFFF)  // iframes[0] = 65535 → normalised to -1
		p.P2(0x0005)  // delay[0] = 5
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.IFrames[0] != -1 {
		t.Errorf("IFrames[0]: got %d, want -1 (65535 normalisation)", st.IFrames[0])
	}
}

func TestSeqTypeDecode_DelayZeroFallbackToFrameDelay(t *testing.T) {
	frames := &SeqFrameConfigs{
		Instances: []*SeqFrame{
			{Delay: 0}, // frame 0
			{Delay: 7}, // frame 1
		},
	}
	st, err := decodeSeq(frames, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)        // count = 1
		p.P2(0x0001)  // frames[0] = 1
		p.P2(0x0000)  // iframes[0] = 0
		p.P2(0x0000)  // delay[0] = 0 → fallback to frames.Instances[1].Delay = 7
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Delay[0] != 7 {
		t.Errorf("Delay[0]: got %d, want 7 (SeqFrame.delay fallback)", st.Delay[0])
	}
	if st.Duration != 7 {
		t.Errorf("Duration: got %d, want 7", st.Duration)
	}
}

func TestSeqTypeDecode_DelayZeroNoFallbackUsesOne(t *testing.T) {
	// nil frames back-ref → fallback to TS L101 default of 1
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)
		p.P2(0x0001)
		p.P2(0x0000)
		p.P2(0x0000) // delay = 0; no fallback registry; final fallback = 1
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Delay[0] != 1 {
		t.Errorf("Delay[0]: got %d, want 1 (TS L101 default)", st.Delay[0])
	}
}

func TestSeqTypeDecode_Loops(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(2); p.P2(0x0007) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Loops != 7 {
		t.Errorf("Loops: got %d, want 7", st.Loops)
	}
}

func TestSeqTypeDecode_WalkMerge(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(3)
		p.P1(2)  // count = 2
		p.P1(11) // walkmerge[0]
		p.P1(22) // walkmerge[1]
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if got := st.WalkMerge; len(got) != 3 || got[0] != 11 || got[1] != 22 || got[2] != 9999999 {
		t.Errorf("WalkMerge: got %v, want [11 22 9999999]", got)
	}
}

func TestSeqTypeDecode_Stretches(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(4) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if !st.Stretches {
		t.Error("Stretches: got false, want true")
	}
}

func TestSeqTypeDecode_Priority(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(5); p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Priority != 3 {
		t.Errorf("Priority: got %d, want 3", st.Priority)
	}
}

func TestSeqTypeDecode_ReplaceHeldLeft(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(6); p.P2(0x0102) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.ReplaceHeldLeft != 0x0102 {
		t.Errorf("ReplaceHeldLeft: got %d, want %d", st.ReplaceHeldLeft, 0x0102)
	}
}

func TestSeqTypeDecode_ReplaceHeldRight(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(7); p.P2(0x0304) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.ReplaceHeldRight != 0x0304 {
		t.Errorf("ReplaceHeldRight: got %d, want %d", st.ReplaceHeldRight, 0x0304)
	}
}

func TestSeqTypeDecode_MaxLoops(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(8); p.P1(42) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.MaxLoops != 42 {
		t.Errorf("MaxLoops: got %d, want 42", st.MaxLoops)
	}
}

func TestSeqTypeDecode_DebugName(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(250); p.PJStrLF("test_seq") })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.DebugName != "test_seq" {
		t.Errorf("DebugName: got %q, want %q", st.DebugName, "test_seq")
	}
}

func TestSeqTypeDecode_UnknownCode(t *testing.T) {
	_, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(99) })
	if err == nil {
		t.Error("want error for unknown code 99, got nil")
	}
}

func TestSeqTypeConfigsCount_NilReceiver(t *testing.T) {
	var c *SeqTypeConfigs
	if got := c.Count(); got != 0 {
		t.Errorf("Count() on nil: got %d, want 0", got)
	}
}

func TestSeqTypeConfigsCount_Populated(t *testing.T) {
	c := &SeqTypeConfigs{Configs: make([]*SeqType, 5)}
	if got := c.Count(); got != 5 {
		t.Errorf("Count(): got %d, want 5", got)
	}
}

func TestLoadSeqTypes_MissingFile(t *testing.T) {
	dir := t.TempDir()
	configs, err := LoadSeqTypes(dir, &SeqFrameConfigs{})
	if err != nil {
		t.Fatalf("LoadSeqTypes: want nil error on missing file, got %v", err)
	}
	if configs == nil {
		t.Fatal("configs: want non-nil registry, got nil")
	}
	if len(configs.Configs) != 0 {
		t.Errorf("Configs: want empty, got %d entries", len(configs.Configs))
	}
}

func TestLoadSeqTypes_FromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "seq.dat")); err != nil {
		t.Skipf("no pack data: %v", err)
	}
	frames, err := LoadSeqFrames(cacheDir)
	if err != nil {
		t.Fatalf("LoadSeqFrames: %v", err)
	}
	configs, err := LoadSeqTypes(cacheDir, frames)
	if err != nil {
		t.Fatalf("LoadSeqTypes: %v", err)
	}
	if len(configs.Configs) == 0 {
		t.Fatal("expected at least one SeqType, got 0")
	}
}
```

- [ ] **Step 2.2: Run tests — expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestNewSeqType|TestSeqTypeDecode|TestSeqTypeConfigsCount|TestLoadSeqTypes' -v 2>&1 | head -20
```

Expected: compile errors mentioning `SeqType`, `SeqTypeConfigs`, `LoadSeqTypes` undefined.

- [ ] **Step 2.3: Create `pkg/objtype/seqtype.go`**

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
	IFrames          []int32 // nil = unset; 65535 in source is normalised to -1 (TS L91)
	Delay            []int32 // per-frame delay; 0 → SeqFrame.Delay fallback (TS L97)
	Loops            int     // -1 default
	WalkMerge        []int32 // nil = unset; last entry = 9999999 sentinel (TS L116)
	Stretches        bool
	Priority         int // 5 default
	ReplaceHeldLeft  int // -1 default
	ReplaceHeldRight int // -1 default
	MaxLoops         int // 99 default
	Duration         int // sum of post-fallback Delay entries

	// frames is an unexported back-reference to the SeqFrame registry
	// used by Decode for the L97 delay-fallback. parseSeqTypes sets it
	// before invoking DecodeType.
	frames *SeqFrameConfigs
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
				v = -1 // TS L91 normalisation
			}
			t.IFrames[i] = v

			d := int32(dat.G2())
			if d == 0 {
				// TS L97: fallback to SeqFrame.instances[frames[i]].delay.
				if t.frames != nil && int(t.Frames[i]) >= 0 && int(t.Frames[i]) < len(t.frames.Instances) {
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

// SeqTypeConfigs is the parsed registry of all sequence config records.
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
// *SeqType for the L97 delay-fallback inside Decode. Returns an empty
// registry with nil error when server/seq.dat is absent (silent-on-missing,
// matching TS SeqType.load at Engine-TS/.../SeqType.ts:12-20).
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

- [ ] **Step 2.4: Run tests — expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestNewSeqType|TestSeqTypeDecode|TestSeqTypeConfigsCount|TestLoadSeqTypes' -v 2>&1 | tail -30
```

Expected: all listed tests PASS (last test may SKIP if pack data is absent).

- [ ] **Step 2.5: Run the full `pkg/objtype` test suite to confirm no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -v 2>&1 | tail -10
```

Expected: PASS (only the test counters change).

- [ ] **Step 2.6: Commit**

```bash
git add pkg/objtype/seqtype.go pkg/objtype/seqtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-57 T2 — SeqType config port

Ports SeqType.ts (codes 1-8 + 250) into pkg/objtype/seqtype.go
with the TS L97 SeqFrame.delay fallback resolved via an unexported
back-reference set by parseSeqTypes.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Server / Player wire-in

**Files:**
- Modify: `modules/world/server.go:85-87` (struct field block)
- Modify: `modules/world/server.go:235-239` (load wire-in)
- Modify: `modules/world/player.go:253-254` area (struct field) + `:343` area (newPlayer body)

### 3a. `modules/world/server.go` — add fields

- [ ] **Step 3.1: Insert `seqFrames` and `seqTypes` fields** after the existing `idkTypes` line in the Server struct.

The current block at `server.go:85-87` looks like:
```go
npcTypes      *objtype.NPCTypeConfigs
huntTypes     *objtype.HuntTypeConfigs
idkTypes      *objtype.IdkTypeConfigs
```

Replace with:
```go
npcTypes      *objtype.NPCTypeConfigs
huntTypes     *objtype.HuntTypeConfigs
idkTypes      *objtype.IdkTypeConfigs
seqFrames     *objtype.SeqFrameConfigs
seqTypes      *objtype.SeqTypeConfigs
```

### 3b. `modules/world/server.go` — add load calls

- [ ] **Step 3.2: Insert `LoadSeqFrames` + `LoadSeqTypes` after the `LoadIdkTypes` block** at server.go:235-239.

After the existing block:
```go
idkTypes, err := objtype.LoadIdkTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load idk types: %w", err)
}
s.idkTypes = idkTypes
```

Insert:
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

### 3c. `modules/world/player.go` — add `seqTypes` field

- [ ] **Step 3.3: Add field declaration on `Player` struct.**

Locate the existing anim-state field block (around `player.go:253-254`):
```go
animID, animDelay int
```

Append immediately above or below it (proximity to anim state aids future grep):
```go
seqTypes *objtype.SeqTypeConfigs // seeded conditionally in newPlayer; gates PlayAnim
```

Verify `objtype` is already imported in `player.go` (it is — `appearanceInv` doc-comment and other fields reference it). If not imported, add:
```go
"github.com/zsrv/goscape/pkg/objtype"
```

### 3d. `modules/world/player.go` — seed in `newPlayer`

- [ ] **Step 3.4: Add the conditional seed** inside `newPlayer(c *client) *Player`.

Pre-flight at HEAD `6950942` confirmed the function shape: `newPlayer` at `player.go:343` uses `p := &Player{...}` (struct literal at lines 344-430), then a `for i := 0; i < 21; i++` stats-init loop, then `p.afkZones[0] = packAfkCoord(...)`, then `return p` at line 440.

Insert the conditional seed immediately after the struct-literal closing `}` at line 430 and before the comment that introduces the stats-init loop ("Sentinel values so the first tick..." at line 431):

```go
    }
    if c.server != nil {
        p.seqTypes = c.server.seqTypes
    }
    // Sentinel values so the first tick of updateStats emits all 21 UpdateStat
    // packets. stats[i] is int32 (always >= 0 in gameplay); levels[i] is uint8
    // (max real value 99). -1 and 255 are unreachable legitimate values.
    for i := 0; i < 21; i++ {
        // ... existing loop body ...
    }
```

If the function shape has changed since pre-flight (e.g. file edits between spec-write and T3 dispatch), re-grep with `grep -n "func newPlayer\|return p" modules/world/player.go` and place the seed immediately before `return p` regardless.

### 3e. Build verification

- [ ] **Step 3.5: Verify build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1 | head -20
```

Expected: clean build (no errors).

- [ ] **Step 3.6: Run full test suite to verify no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -30
```

Expected: all tests PASS. If any fail unrelated to NAI-57, stop and investigate before proceeding.

- [ ] **Step 3.7: Commit**

```bash
git add modules/world/server.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-57 T3 — wire SeqFrame/SeqType registries into Server + Player

Adds Server.seqFrames + Server.seqTypes loaded after idkTypes at
bootstrap. Adds Player.seqTypes field seeded from c.server.seqTypes
inside newPlayer when c.server is non-nil.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — Retire `(*Player).Animate`

**Files:**
- Modify: `modules/world/player_masks.go:8-12` (delete method)
- Modify: `modules/world/player_masks_test.go` (delete the 2 tests)

### 4a. Pre-flight re-grep (per `enumerate_all_sites.md` memory)

- [ ] **Step 4.1: Re-grep for any new caller introduced since spec-write**

```bash
rg -n "\bp\.Animate\(" --type go pkg/ modules/ cmd/ | grep -v _test.go
rg -n "\(\*Player\)\.Animate\(" --type go pkg/ modules/ cmd/
```

Expected: zero non-test hits (only the method site at `player_masks.go:8`).

If any new caller appeared, **stop**: rewrite the caller to use `PlayAnim` first as a separate prep commit, then resume T4.

### 4b. Delete the method

- [ ] **Step 4.2: Remove `(*Player).Animate` from `modules/world/player_masks.go`**

Delete this block (currently at lines 8-12):
```go
func (p *Player) Animate(id, delay int) {
	p.animID = id
	p.animDelay = delay
	p.masks |= rsbuf.MaskAnim
}
```

After deletion, verify the file still compiles — `rsbuf` import may still be used by other functions (`Say`, `Chat`, `SetSpotAnim` etc.). If `rsbuf` becomes unused after deletion, the Go compiler will flag it; remove the import only if needed.

### 4c. Delete the test sites

- [ ] **Step 4.3: Delete the 2 tests in `modules/world/player_masks_test.go`**

Locate the tests at lines 11 and 88 that call `p.Animate(123, 5)`. Delete each test function entirely (the `func Test...(t *testing.T) { ... }` block).

If those are the only tests in the file (verify with `wc -l player_masks_test.go` after deletion — if all that remains is `package world` + imports, delete the file outright):

```bash
# Inspect remaining content first
cat modules/world/player_masks_test.go
# If only package + imports remain (or empty), delete:
rm modules/world/player_masks_test.go
```

### 4d. Build + test verification

- [ ] **Step 4.4: Verify build and full tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1 | head
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
```

Expected: clean build, all tests PASS.

- [ ] **Step 4.5: Re-grep to confirm zero remaining `p.Animate` references**

```bash
rg -n "\bp\.Animate\(" --type go pkg/ modules/ cmd/
rg -n "\(p \*Player\) Animate" --type go pkg/ modules/ cmd/
```

Expected: zero hits (or only references in comments — flag those for removal in this same commit).

- [ ] **Step 4.6: Commit**

```bash
git add modules/world/player_masks.go modules/world/player_masks_test.go
# If the test file was deleted:
# git rm modules/world/player_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): NAI-57 T4 — retire (*Player).Animate

Drive-by per dead_api_polish memory: (*Player).Animate had zero
production callers and was a parallel un-gated entry point that
would silently bypass the NAI-57 SeqType gates. PlayAnim is now
the sole entry. Deletes the method and its 2 obsolete tests.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — `(*Player).PlayAnim` bounds + priority gate

**Files:**
- Modify: `modules/world/player_script.go:541-553` (PlayAnim body + doc-comment)
- Modify: `modules/world/player_anim_test.go` (extend existing tests + add 9 new)

### 5a. Add `buildSeqTypes` helper

- [ ] **Step 5.1: Add `buildSeqTypes` helper to `modules/world/player_anim_test.go`**

Place it after the existing test imports and before the first test:

```go
// buildSeqTypes returns a SeqTypeConfigs with count entries.
// Each entry has Priority=5 (TS default) and DebugName empty. Tests that
// exercise the priority-comparison arm override per-entry Priority before
// invoking PlayAnim/Animate.
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

If `objtype` is not yet imported in this test file, add `"github.com/zsrv/goscape/pkg/objtype"` to the import block.

### 5b. Write failing tests for the new gate behavior

- [ ] **Step 5.2: Add 9 new tests + extend the 2 existing tests in `player_anim_test.go`**

First, **modify** the 2 existing tests (`TestPlayAnim_AnimProtectBlocksWrite` and `TestPlayAnim_AnimProtectZeroAllowsWrite`) to seed the registry. The existing pattern is roughly:
```go
func TestPlayAnim_AnimProtectZeroAllowsWrite(t *testing.T) {
    p, _ := newTestPlayer(t)
    // (existing setup)
    p.PlayAnim(123, 5)
    // (assertions)
}
```

In each, add `p.seqTypes = buildSeqTypes(124)` *before* the `p.PlayAnim(123, 5)` call so seqID=123 stays in-range.

Second, **append** these new tests:

```go
func TestPlayAnim_BoundsRejectAtCount(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(50)
	p.masks = 0
	p.PlayAnim(50, 5)
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1 (bounds-reject)", p.animID)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set on bounds-reject")
	}
}

func TestPlayAnim_BoundsRejectAboveCount(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(50)
	p.masks = 0
	p.PlayAnim(99, 5)
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1", p.animID)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set")
	}
}

func TestPlayAnim_NilRegistryRejectsAllNonClear(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = nil
	p.masks = 0
	p.PlayAnim(0, 5) // count==0; bounds 0>=0 → reject
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1 (nil registry → count=0 → bounds-reject)", p.animID)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set")
	}
}

func TestPlayAnim_NilRegistryAllowsClear(t *testing.T) {
	// Registry never loaded; animID is fresh (-1). Clear with -1 must succeed
	// because the priority arm short-circuits via seqID==-1 before any slice deref.
	p, _ := newTestPlayer(t)
	p.seqTypes = nil
	p.animID = -1 // fresh state
	p.masks = 0
	p.PlayAnim(-1, 0)
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1", p.animID)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim should be set on clear")
	}
}

func TestPlayAnim_PriorityHigherOverwrites(t *testing.T) {
	p, _ := newTestPlayer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 3
	cfg.Configs[10].Priority = 7
	p.seqTypes = cfg
	p.animID = 5
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (higher priority overwrites)", p.animID)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim should be set on overwrite")
	}
}

func TestPlayAnim_PriorityLowerRejected(t *testing.T) {
	p, _ := newTestPlayer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 7
	cfg.Configs[10].Priority = 3
	p.seqTypes = cfg
	p.animID = 5
	p.animDelay = 99
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 5 {
		t.Errorf("animID: got %d, want 5 (lower priority rejected)", p.animID)
	}
	if p.animDelay != 99 {
		t.Errorf("animDelay: got %d, want 99 (preserved)", p.animDelay)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set on rejection")
	}
}

func TestPlayAnim_PriorityEqualRejected(t *testing.T) {
	// Both Priority=5 (default). TS uses strict `>` so equal is rejected.
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(20)
	p.animID = 5
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 5 {
		t.Errorf("animID: got %d, want 5 (equal priority rejected)", p.animID)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set")
	}
}

func TestPlayAnim_CurrentAnimZeroPriorityOverwrites(t *testing.T) {
	// TS L1846 third disjunct: when current anim's priority is 0, any new
	// anim overwrites regardless of its own priority.
	p, _ := newTestPlayer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 0
	cfg.Configs[10].Priority = 5
	p.seqTypes = cfg
	p.animID = 5
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (current zero-priority overwrite)", p.animID)
	}
}

func TestPlayAnim_FreshAnimIDMinusOneAlwaysOverwrites(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(20)
	p.animID = -1 // fresh
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (fresh animID=-1 short-circuit)", p.animID)
	}
}
```

Note: `newTestPlayer(t)` returns `(*Player, net.Conn)` — discard the conn with `_`. Verify the test file already uses this signature.

- [ ] **Step 5.3: Run new tests — expect failures (gates not yet wired)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestPlayAnim_(BoundsReject|NilRegistry|Priority|FreshAnimID|CurrentAnimZero)' -v 2>&1 | tail -30
```

Expected: most new tests FAIL. The bounds-reject and priority tests will fail because the current `PlayAnim` body has no such gates — it sets animID unconditionally (after the animProtect check). The nil-registry tests may panic on nil deref or fail assertions.

Specifically expect:
- `TestPlayAnim_BoundsRejectAtCount` FAIL (animID becomes 50)
- `TestPlayAnim_PriorityLowerRejected` FAIL (animID becomes 10)
- `TestPlayAnim_NilRegistryAllowsClear` may PASS already (clear with -1 sets animID=-1 even without gates)

Compiles cleanly is the minimum bar.

### 5c. Replace the `PlayAnim` body

- [ ] **Step 5.4: Edit `modules/world/player_script.go` PlayAnim**

Locate the current `PlayAnim` (currently at `player_script.go:541-553`):

```go
// PlayAnim schedules sequence seqID with the given client-side delay on
// the player's primary animation slot. seqID=-1 clears. Mirrors TS
// Player.playAnimation (Player.ts:1841-1851); the animProtect early-return
// is the TS L1842 gate (NAI-56). The remaining TS gates
// (anim >= SeqType.count bounds and priority comparison at L1846) depend
// on the unported SeqType config registry — tracked as NAI-56-D1.
func (p *Player) PlayAnim(seqID, delay int) {
	if p.animProtect != 0 {
		return // TS Player.ts:1842 — animProtect gate (NAI-56)
	}
	p.animID = seqID
	p.animDelay = delay
	p.masks |= rsbuf.MaskAnim
}
```

Replace with:

```go
// PlayAnim schedules sequence seqID with the given client-side delay on
// the player's primary animation slot. seqID=-1 clears. Mirrors TS
// Player.playAnimation (Player.ts:1840-1851): bounds-reject on
// seqID >= SeqType.count, animProtect early-return, and priority-comparison
// overwrite gate. The seqID==-1 / animID==-1 short-circuits in the priority
// arm guard the slice dereferences. Closes deviation NAI-56-D1.
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

### 5d. Run tests — expect pass

- [ ] **Step 5.5: Run all PlayAnim tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestPlayAnim' -v 2>&1 | tail -40
```

Expected: all 11 tests PASS (2 existing AnimProtect tests + 9 new gate tests).

- [ ] **Step 5.6: Run the full `modules/world` test suite to catch any regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -10
```

Expected: PASS. If any unrelated test fails because it called `p.PlayAnim(N>0, ...)` against a default-nil-registry Player, seed `p.seqTypes = buildSeqTypes(...)` in that test as a separate fixup — but flag it in the commit message rather than rolling silently.

- [ ] **Step 5.7: Commit**

```bash
git add modules/world/player_script.go modules/world/player_anim_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-57 T5 — gate (*Player).PlayAnim on SeqType bounds + priority

Ports the full TS Player.playAnimation gate (Player.ts:1840-1851):
bounds-reject on seqID >= SeqType.count, animProtect early-return,
and priority-comparison overwrite gate. SeqType.Count() is nil-safe;
seqID==-1 / animID==-1 short-circuits guard the slice derefs.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6 — `(*Npc).Animate` bounds + priority gate + close

**Files:**
- Modify: `modules/world/npc_masks.go:8-12` (Animate body + doc-comment)
- Modify: `modules/world/npc_test.go` (seed existing tests; add 8 new)
- Append memory entry to `nai_followups.md`
- Update active deviation tally if tracked separately

### 6a. Write failing tests for Npc.Animate gates

- [ ] **Step 6.1: Add 8 new tests + seed the 2 existing `n.Animate(123, 5)` tests in `modules/world/npc_test.go`**

The existing pattern likely uses `newTestServer(t)` + `s.addNpc(...)` to construct an Npc with `n.server` set. For the existing tests at `npc_test.go:33` and `npc_test.go:195`, ensure `s.seqTypes = buildSeqTypes(200)` is set on the test-server before the `Animate` call. (200 covers id=123 with headroom for priority overrides.)

If `buildSeqTypes` is currently only in `player_anim_test.go`, it's accessible from `npc_test.go` because both are in the same `world` test package. No re-declaration needed.

Append these new tests:

```go
func TestNpcAnimate_BoundsRejectAtCount(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(50)
	n := &Npc{server: s, animID: -1}
	n.Animate(50, 5)
	if n.animID != -1 {
		t.Errorf("animID: got %d, want -1 (bounds-reject)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim != 0 {
		t.Error("NpcMaskAnim should not be set on bounds-reject")
	}
}

func TestNpcAnimate_NilServerEarlyReturn(t *testing.T) {
	// Goscape-only nil-guard (test-fixture concession; no TS analogue).
	n := &Npc{server: nil, animID: -1}
	n.Animate(0, 5)
	if n.animID != -1 {
		t.Errorf("animID: got %d, want -1 (nil server → no-op)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim != 0 {
		t.Error("NpcMaskAnim should not be set when server is nil")
	}
}

func TestNpcAnimate_PriorityHigherOverwrites(t *testing.T) {
	s := newTestServer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 3
	cfg.Configs[10].Priority = 7
	s.seqTypes = cfg
	n := &Npc{server: s, animID: 5}
	n.Animate(10, 3)
	if n.animID != 10 {
		t.Errorf("animID: got %d, want 10 (higher priority overwrites)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim should be set on overwrite")
	}
}

func TestNpcAnimate_PriorityLowerRejected(t *testing.T) {
	s := newTestServer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 7
	cfg.Configs[10].Priority = 3
	s.seqTypes = cfg
	n := &Npc{server: s, animID: 5, animDelay: 99}
	n.Animate(10, 3)
	if n.animID != 5 {
		t.Errorf("animID: got %d, want 5 (lower priority rejected)", n.animID)
	}
	if n.animDelay != 99 {
		t.Errorf("animDelay: got %d, want 99 (preserved)", n.animDelay)
	}
	if n.masks&rsbuf.NpcMaskAnim != 0 {
		t.Error("NpcMaskAnim should not be set on rejection")
	}
}

func TestNpcAnimate_PriorityEqualRejected(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(20) // all default Priority=5
	n := &Npc{server: s, animID: 5}
	n.Animate(10, 3)
	if n.animID != 5 {
		t.Errorf("animID: got %d, want 5 (equal priority rejected)", n.animID)
	}
}

func TestNpcAnimate_CurrentZeroPriorityOverwrites(t *testing.T) {
	s := newTestServer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 0
	cfg.Configs[10].Priority = 5
	s.seqTypes = cfg
	n := &Npc{server: s, animID: 5}
	n.Animate(10, 3)
	if n.animID != 10 {
		t.Errorf("animID: got %d, want 10 (current zero-priority overwrite)", n.animID)
	}
}

func TestNpcAnimate_FreshAnimIDMinusOneAlwaysOverwrites(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(20)
	n := &Npc{server: s, animID: -1}
	n.Animate(10, 3)
	if n.animID != 10 {
		t.Errorf("animID: got %d, want 10 (fresh animID=-1 short-circuit)", n.animID)
	}
}

func TestNpcAnimate_ClearWithMinusOneSucceeds(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(20)
	n := &Npc{server: s, animID: 5}
	n.Animate(-1, 0)
	if n.animID != -1 {
		t.Errorf("animID: got %d, want -1 (clear)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim should be set on clear")
	}
}
```

If `&Npc{...}` literal construction requires extra fields (e.g. `slot`, `typ`) to compile or to satisfy invariants, add them per existing test fixture patterns at `npc_test.go`. Inspect the existing `n.Animate(123, 5)` test fixtures for the minimal required field set.

- [ ] **Step 6.2: Run new tests — expect failures**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestNpcAnimate_(BoundsReject|NilServer|Priority|CurrentZero|FreshAnimID|ClearWith)' -v 2>&1 | tail -30
```

Expected: most new tests FAIL — the current `Animate` has no gates.

### 6b. Replace the `Npc.Animate` body

- [ ] **Step 6.3: Edit `modules/world/npc_masks.go`**

Replace the current method (currently at npc_masks.go:8-12):

```go
func (n *Npc) Animate(id, delay int) {
	n.animID = id
	n.animDelay = delay
	n.masks |= rsbuf.NpcMaskAnim
}
```

With:

```go
// Animate schedules sequence id with the given client-side delay on the
// NPC's primary animation slot. id=-1 clears. Mirrors TS Npc.playAnimation
// (Npc.ts:451-462): bounds-reject on id >= SeqType.count and
// priority-comparison overwrite gate. NPCs have no animProtect equivalent
// (TS-faithful — Player-only field). The n.server == nil guard is a
// goscape-only nil-safe concession for test fixtures that construct a
// bare *Npc without registering through addNpc; TS has no analogue
// (its registry is a static class). Closes deviation NAI-56-D1.
func (n *Npc) Animate(id, delay int) {
	if n.server == nil {
		return // goscape-only nil-guard for test fixtures
	}
	if id >= n.server.seqTypes.Count() {
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

### 6c. Run tests — expect pass

- [ ] **Step 6.4: Run all Npc.Animate tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestNpcAnimate' -v 2>&1 | tail -40
```

Expected: all new tests + 2 seeded existing tests PASS.

- [ ] **Step 6.5: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
```

Expected: all tests PASS.

### 6d. Stale-deviation grep + edit

- [ ] **Step 6.6: Grep all stale `NAI-56-D1` references** (per `retire_deviation_grep_all_comments.md` memory)

```bash
rg -n "NAI-56-D1" --type go --type md pkg/ modules/ cmd/
```

Expected sites at this point:
- `modules/world/player_script.go` — the original NAI-56 doc-comment that mentions "tracked as NAI-56-D1" (now retired by T5's doc-comment rewrite — verify it's gone)
- `docs/superpowers/specs/2026-05-01-nai-57-seqtype-config-port-design.md` — keep (provenance)
- `docs/superpowers/plans/2026-05-01-nai-57-seqtype-config-port.md` — keep (provenance)

For each remaining doc-comment site outside `docs/`, edit the comment to either retire or update the reference. After edits, re-grep and confirm zero non-`docs/` hits remain. Common stale-comment shapes to look for and rewrite:
- "tracked as NAI-56-D1" → drop the trailing tracker phrase
- "deferred under NAI-56-D1" → replace with reference to the closure (NAI-57)

### 6e. Append `nai_followups.md` close entry

- [ ] **Step 6.7: Append a `## NAI-57 — CLOSED YYYY-MM-DD` section to `nai_followups.md`**

The memory file is at `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. Append the section at the end. Per memory `memory_write_sandbox_quirk.md` precedent: use the Write/Edit tool, not Bash redirection.

Template (substitute the close date and final commit SHA):

```markdown
## NAI-57 — CLOSED <YYYY-MM-DD>

**Scope:** Port `SeqType` (Engine-TS/.../SeqType.ts) and its `SeqFrame`
(Engine-TS/.../SeqFrame.ts) delay-only dependency into pkg/objtype.
Wire `(*Player).PlayAnim` and `(*Npc).Animate` with TS bounds-reject +
priority-comparison gates. Drive-by retirement of `(*Player).Animate`
(zero production callers, parallel un-gated entry point).

**Cadence:** Full sub-spec, single bundle, 6 tasks. NAI-46 IdkType-shaped.

**Close commit:** `<SHA>` (T1: `<sha>`, T2: `<sha>`, T3: `<sha>`,
T4: `<sha>`, T5: `<sha>`, T6: `<sha>`).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-57-seqtype-config-port-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-01-nai-57-seqtype-config-port.md`.

**Follow-ups closed:**
- NAI-56-D1 (anim-playback bounds + priority gates).

**Deviations opened:** none.

**Deviations closed:** NAI-56-D1.

**Deviation tally:** 21 → 20.

**Follow-up candidates:** None identified.
```

### 6f. Close commit (T6 final)

- [ ] **Step 6.8: Final build + test sweep**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... 2>&1 | tail -10
```

Expected: all clean.

- [ ] **Step 6.9: Commit T6 implementation**

```bash
git add modules/world/npc_masks.go modules/world/npc_test.go modules/world/player_script.go
# (player_script.go only if T6d's grep surfaced stale comments to edit)
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-57 T6 — gate (*Npc).Animate on SeqType bounds + priority

Ports the TS Npc.playAnimation gate (Npc.ts:451-462): bounds-reject on
id >= SeqType.count and priority-comparison overwrite gate. Includes
goscape-only n.server == nil nil-guard for test fixtures (no TS analogue).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 6.10: Commit close — memory + close trailer**

The memory file lives outside the repo (in `~/.claude/projects/...`); the
close commit only carries the trailer + a no-op chore message. If the
project tracks a deviation tally in a repo file, update it as part of
this close commit.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-57 — SeqType config port; closes NAI-56-D1

Closes the anim-playback gate cluster: bounds-reject + priority-overwrite
now wired in (*Player).PlayAnim and (*Npc).Animate. Retires the un-gated
(*Player).Animate parallel entry point (drive-by per dead_api_polish).

Closes memory: NAI-56-D1
EOF
)"
```

(Use `--allow-empty` only if no repo files were modified for the close.
If `nai_followups.md` is the only memory artifact and it's outside the repo,
the close commit will be empty by design.)

- [ ] **Step 6.11: Final sanity grep**

```bash
rg -n "NAI-56-D1" --type go pkg/ modules/ cmd/
# Expected: zero hits (docs/ provenance excluded)

rg -n "\bp\.Animate\(" --type go pkg/ modules/ cmd/
# Expected: zero hits

GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -5
# Expected: PASS
```

---

## Memory entries that apply

- `dead_api_polish.md` — drives Task 4.
- `retire_deviation_grep_all_comments.md` — drives Task 6d.
- `enumerate_all_sites.md` — drives Task 4.1 pre-flight re-grep.
- `controller_preflight.md` — drives the pre-flight verification block at top of plan.
- `verify_implementer_claims.md` — drives the per-task post-commit verification (`git show <SHA> --stat` on each commit; full-suite test at T3, T5, T6).
- `plan_runnable_test_fixtures.md` — every test in this plan was mentally compiled at plan-write time; fixture seeding is explicit (no inferred field names).
- `close_commit_memory_trailer.md` — close commit carries `Closes memory: NAI-56-D1`.
- `mock_recorder_field_naming_check.md` — N/A this plan; no recording-mock additions.
- `int32_hex_literal_overflow.md` — N/A this plan; no high hex literals.
- `plan_var_name_collision.md` — verified: no parameter-name vs `:=` collisions in the gate code.
