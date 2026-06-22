# NAI-58 SpotanimType Config Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `Engine-TS/src/cache/config/SpotanimType.ts` into `pkg/objtype`, expose it via `script.Configs`, and tighten `checkSpotAnimType` to mirror TS `SpotAnimTypeValid` (rejects negatives AND unregistered ids). Closes deviation NAI-36-D2; tally 20 → 19.

**Architecture:** Single-bundle, 6-task port following the IdkType (NAI-46) → SeqType (NAI-57) chain. `pkg/objtype/spotanimtype.go` adds the struct + dual-source loader; server bootstrap loads it; a `Configs.SpotAnimType(id)` accessor lets `pkg/script/handlers_map.go`'s `checkSpotAnimType` perform the presence check. Test infrastructure extends `runMapOp` to thread a `Configs` mock through SPOTANIM_MAP test paths.

**Tech Stack:** Go 1.26+ (per `go_version.md`). Existing in-tree helpers: `pkg/io/jagfile`, `pkg/io/packet`, `pkg/objtype/configtype.go` (`ConfigType` base, `DecodeType` polymorphic helper).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-58-spotanimtype-config-port-design.md`

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `pkg/objtype/spotanimtype.go` | **Create** | `SpotanimType` struct, `NewSpotanimType` constructor, `Decode` opcode dispatch, `SpotanimTypeConfigs` registry, `LoadSpotanimTypes` + `parseSpotanimTypes` |
| `pkg/objtype/spotanimtype_test.go` | **Create** | Per-opcode decode tests + parse/load tests |
| `modules/world/server.go` | **Modify** | Add `spotanimTypes *objtype.SpotanimTypeConfigs` field; load via `objtype.LoadSpotanimTypes` in bootstrap (alongside `seqTypes`) |
| `modules/world/server_configs.go` | **Modify** | Add `(c serverConfigsView) SpotAnimType(id int) *objtype.SpotanimType` accessor |
| `pkg/script/configs.go` | **Modify** | Extend `Configs` interface with `SpotAnimType(id int) *objtype.SpotanimType` |
| `pkg/script/handlers_db_test.go` | **Modify** | Add `SpotAnimType` stub to `fakeDbConfigs` |
| `pkg/script/handlers_loc_test.go` | **Modify** | Add `SpotAnimType` stub to `fakeConfigs` |
| `pkg/script/handlers_config_test.go` | **Modify** | Add `spotAnimTypes map[int]*objtype.SpotanimType` field + `SpotAnimType` accessor to `mockConfigs` |
| `pkg/script/handlers_map.go` | **Modify** | Tighten `checkSpotAnimType` (presence check via `s.Configs.SpotAnimType`); update sole call site in `handleSpotAnimMap`; retire NAI-36-D2 doc-comment |
| `pkg/script/handlers_map_test.go` | **Modify** | Extend `runMapOp` signature to take `Configs`; update existing callers; add positive/negative-arm SPOTANIM_MAP tests; retire NAI-36-D2 doc-comment |
| `nai_followups.md` (memory) | **Modify** | Append NAI-58 close section; mark NAI-36-D2 Resolved |

---

## Pre-flight (controller verifies before each implementer dispatch)

Per `controller_preflight.md`. Run before T1 dispatch:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
rg -on "NAI-36-D2" pkg modules cmd
```

Expected: clean build; NAI-36-D2 hits in exactly two locations (`pkg/script/handlers_map.go:207-211` doc-comment, `pkg/script/handlers_map_test.go:471` test doc-comment).

Re-grep before T3:
```bash
rg -n "IdkType\(id" pkg/ modules/
```
Expected: production accessor at `modules/world/server_configs.go:81`, interface declaration at `pkg/script/configs.go:18`, three test mocks (`handlers_db_test.go:26`, `handlers_loc_test.go:27`, `handlers_config_test.go:29`), one production caller at `pkg/script/handlers_player.go:172`. Any new implementor between spec-write and T3 needs the same `SpotAnimType` stub added.

Re-grep before T4:
```bash
rg -n '\bcheckSpotAnimType\(' pkg modules cmd
```
Expected: definition at `pkg/script/handlers_map.go:212`, sole caller at `pkg/script/handlers_map.go:233`. Any other caller needs the `s, ` prefix added at the call site.

---

## Task 1 — `pkg/objtype/spotanimtype.go` + tests (TDD)

**Files:**
- Create: `pkg/objtype/spotanimtype.go`
- Create: `pkg/objtype/spotanimtype_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/objtype/spotanimtype_test.go`:

```go
package objtype

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewSpotanimTypeDefaults(t *testing.T) {
	c := NewSpotanimType(7)
	if c.ID != 7 {
		t.Errorf("ID: got %d, want 7", c.ID)
	}
	if c.Anim != -1 {
		t.Errorf("Anim default: got %d, want -1", c.Anim)
	}
	if c.Resizeh != 128 {
		t.Errorf("Resizeh default: got %d, want 128", c.Resizeh)
	}
	if c.Resizev != 128 {
		t.Errorf("Resizev default: got %d, want 128", c.Resizev)
	}
	if c.Model != 0 || c.Orientation != 0 || c.Ambient != 0 || c.Contrast != 0 {
		t.Errorf("expected zero ints for Model/Orientation/Ambient/Contrast: got %+v", c)
	}
	if c.HasAlpha {
		t.Errorf("HasAlpha default: got true, want false")
	}
	for i := range 6 {
		if c.RecolS[i] != 0 || c.RecolD[i] != 0 {
			t.Errorf("RecolS/D[%d] default: want 0/0, got %d/%d", i, c.RecolS[i], c.RecolD[i])
		}
	}
}

func TestSpotanimTypeDecode_Model(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0x12, 0x34}) // g2 = 0x1234
	if err := c.Decode(1, buf); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Model != 0x1234 {
		t.Errorf("Model: got %d, want 0x1234", c.Model)
	}
}

func TestSpotanimTypeDecode_Anim(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0x00, 0x05})
	if err := c.Decode(2, buf); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Anim != 5 {
		t.Errorf("Anim: got %d, want 5", c.Anim)
	}
}

func TestSpotanimTypeDecode_HasAlpha(t *testing.T) {
	c := NewSpotanimType(0)
	if err := c.Decode(3, packet.NewPacket(nil)); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !c.HasAlpha {
		t.Errorf("HasAlpha: want true")
	}
}

func TestSpotanimTypeDecode_Resizeh(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0x01, 0x00})
	if err := c.Decode(4, buf); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Resizeh != 256 {
		t.Errorf("Resizeh: got %d, want 256", c.Resizeh)
	}
}

func TestSpotanimTypeDecode_Resizev(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0x00, 0x40})
	if err := c.Decode(5, buf); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Resizev != 64 {
		t.Errorf("Resizev: got %d, want 64", c.Resizev)
	}
}

func TestSpotanimTypeDecode_Orientation(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0x00, 0x5A})
	if err := c.Decode(6, buf); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Orientation != 90 {
		t.Errorf("Orientation: got %d, want 90", c.Orientation)
	}
}

func TestSpotanimTypeDecode_Ambient(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0x32})
	if err := c.Decode(7, buf); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Ambient != 50 {
		t.Errorf("Ambient: got %d, want 50", c.Ambient)
	}
}

func TestSpotanimTypeDecode_Contrast(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0x14})
	if err := c.Decode(8, buf); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Contrast != 20 {
		t.Errorf("Contrast: got %d, want 20", c.Contrast)
	}
}

func TestSpotanimTypeDecode_RecolSInRange(t *testing.T) {
	c := NewSpotanimType(0)
	if err := c.Decode(40, packet.NewPacket([]byte{0x00, 0x07})); err != nil {
		t.Fatalf("Decode 40: %v", err)
	}
	if err := c.Decode(45, packet.NewPacket([]byte{0x00, 0x09})); err != nil {
		t.Fatalf("Decode 45: %v", err)
	}
	if c.RecolS[0] != 7 {
		t.Errorf("RecolS[0]: got %d, want 7", c.RecolS[0])
	}
	if c.RecolS[5] != 9 {
		t.Errorf("RecolS[5]: got %d, want 9", c.RecolS[5])
	}
}

func TestSpotanimTypeDecode_RecolSOutOfRangeDiscarded(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0xAB, 0xCD, 0x00, 0x00})
	if err := c.Decode(47, buf); err != nil {
		t.Fatalf("Decode 47: %v", err)
	}
	for i, v := range c.RecolS {
		if v != 0 {
			t.Errorf("RecolS[%d]: out-of-range slot leaked, got %d", i, v)
		}
	}
	if buf.Pos != 2 {
		t.Errorf("packet cursor: got %d, want 2 (g2 must still consume bytes)", buf.Pos)
	}
}

func TestSpotanimTypeDecode_RecolDInRange(t *testing.T) {
	c := NewSpotanimType(0)
	if err := c.Decode(50, packet.NewPacket([]byte{0x00, 0x11})); err != nil {
		t.Fatalf("Decode 50: %v", err)
	}
	if err := c.Decode(55, packet.NewPacket([]byte{0x00, 0x22})); err != nil {
		t.Fatalf("Decode 55: %v", err)
	}
	if c.RecolD[0] != 0x11 {
		t.Errorf("RecolD[0]: got %d, want 0x11", c.RecolD[0])
	}
	if c.RecolD[5] != 0x22 {
		t.Errorf("RecolD[5]: got %d, want 0x22", c.RecolD[5])
	}
}

func TestSpotanimTypeDecode_RecolDOutOfRangeDiscarded(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{0xAB, 0xCD})
	if err := c.Decode(57, buf); err != nil {
		t.Fatalf("Decode 57: %v", err)
	}
	for i, v := range c.RecolD {
		if v != 0 {
			t.Errorf("RecolD[%d]: out-of-range slot leaked, got %d", i, v)
		}
	}
}

func TestSpotanimTypeDecode_DebugName(t *testing.T) {
	c := NewSpotanimType(0)
	buf := packet.NewPacket([]byte{'f', 'i', 'r', 'e', '\n'})
	if err := c.Decode(250, buf); err != nil {
		t.Fatalf("Decode 250: %v", err)
	}
	if c.DebugName != "fire" {
		t.Errorf("DebugName: got %q, want %q", c.DebugName, "fire")
	}
}

func TestSpotanimTypeDecode_UnknownCode(t *testing.T) {
	c := NewSpotanimType(0)
	if err := c.Decode(99, packet.NewPacket(nil)); err == nil {
		t.Errorf("Decode 99: expected error, got nil")
	}
}

func TestParseSpotanimTypes_EmptyServerCount(t *testing.T) {
	server := packet.NewPacket([]byte{0x00, 0x00}) // g2 = 0
	clientJag := writeOneEntryJag(t, "spotanim.dat", []byte{0x00, 0x00, 0x00})
	got, err := parseSpotanimTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseSpotanimTypes: %v", err)
	}
	if len(got.Configs) != 0 {
		t.Errorf("Configs: got %d, want 0", len(got.Configs))
	}
	if len(got.ConfigNames) != 0 {
		t.Errorf("ConfigNames: got %d, want 0", len(got.ConfigNames))
	}
}

func TestLoadSpotanimTypes_MissingFileSilent(t *testing.T) {
	dir := t.TempDir()
	got, err := LoadSpotanimTypes(dir)
	if err != nil {
		t.Fatalf("LoadSpotanimTypes: %v", err)
	}
	if len(got.Configs) != 0 {
		t.Errorf("Configs: got %d, want 0", len(got.Configs))
	}
	_ = filepath.Join // keep import live if helpers move
}
```

> **Note on `writeOneEntryJag`:** if no such helper exists in `pkg/objtype/` test files, the implementer should mirror the equivalent helper used by `idktype_test.go` / `seqtype_test.go`. Pre-flight should grep `rg -n "writeOneEntryJag\|jagfileFromBytes" pkg/objtype/` to identify the local idiom and reuse the same helper name. If sibling tests use a different shape (e.g. a per-test `*io.Jagfile` constructor), copy that pattern verbatim. The TestParseSpotanimTypes test only needs an empty client jag entry — keep the fixture minimal.

- [ ] **Step 2: Run test to verify it fails (compile error: package doesn't exist yet)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```
Expected: FAIL with "undefined: NewSpotanimType" / "undefined: SpotanimType" / etc.

- [ ] **Step 3: Implement `pkg/objtype/spotanimtype.go`**

Create `pkg/objtype/spotanimtype.go`:

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

// SpotanimType is a single spotanim.dat config record (graphic / spot animation).
// Mirrors Engine-TS/src/cache/config/SpotanimType.ts.
type SpotanimType struct {
	ConfigType
	Model       int
	Anim        int // -1 default
	HasAlpha    bool
	RecolS      [6]uint16
	RecolD      [6]uint16
	Resizeh     int // 128 default
	Resizev     int // 128 default
	Orientation int
	Ambient     int
	Contrast    int
}

// NewSpotanimType returns a SpotanimType with TS-faithful defaults.
// TS defaults: model=0, anim=-1, resizeh=128, resizev=128.
func NewSpotanimType(id int) *SpotanimType {
	return &SpotanimType{
		ConfigType: ConfigType{ID: id},
		Anim:       -1,
		Resizeh:    128,
		Resizev:    128,
	}
}

// Decode dispatches on the spotanim config opcode, matching TS
// SpotanimType.decode at Engine-TS/src/cache/config/SpotanimType.ts:78-104.
func (t *SpotanimType) Decode(code uint8, dat *packet.Packet) error {
	switch code {
	case 1:
		t.Model = int(dat.G2())
	case 2:
		t.Anim = int(dat.G2())
	case 3:
		t.HasAlpha = true
	case 4:
		t.Resizeh = int(dat.G2())
	case 5:
		t.Resizev = int(dat.G2())
	case 6:
		t.Orientation = int(dat.G2())
	case 7:
		t.Ambient = int(dat.G1())
	case 8:
		t.Contrast = int(dat.G1())
	case 40, 41, 42, 43, 44, 45, 46, 47, 48, 49:
		// TS recol_s is 6-element; codes 46-49 are out-of-range. Guard
		// matches TS Uint16Array silent-discard behavior. Same shape as
		// idktype.go.
		slot := code - 40
		v := dat.G2()
		if slot < 6 {
			t.RecolS[slot] = v
		}
	case 50, 51, 52, 53, 54, 55, 56, 57, 58, 59:
		slot := code - 50
		v := dat.G2()
		if slot < 6 {
			t.RecolD[slot] = v
		}
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized spotanim config code %d", code)
	}
	return nil
}

// SpotanimTypeConfigs is the parsed registry of all spotanim records.
type SpotanimTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*SpotanimType
}

// LoadSpotanimTypes parses server/spotanim.dat + client/config jag →
// spotanim.dat into a SpotanimTypeConfigs registry. Returns an empty
// registry with nil error when server/spotanim.dat is absent
// (silent-on-missing, matching TS SpotanimType.load).
func LoadSpotanimTypes(dir string) (*SpotanimTypeConfigs, error) {
	server, err := packet.Load(filepath.Join(dir, "server", "spotanim.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &SpotanimTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}

	clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}

	return parseSpotanimTypes(server, clientJag)
}

func parseSpotanimTypes(server *packet.Packet, clientJag *io.Jagfile) (*SpotanimTypeConfigs, error) {
	count := int(server.G2())
	configs := make([]*SpotanimType, count)
	configNames := make(map[string]int, count)

	client, err := clientJag.Read("spotanim.dat")
	if err != nil {
		return nil, err
	}
	client.Pos = 2 // skip client-side count header (matches idktype.go pattern)

	for id := range count {
		config := NewSpotanimType(id)
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

	return &SpotanimTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

> **Implementer note on `packet.NewPacket` and `packet.Load`:** verify the exact constructor names against `pkg/io/packet/packet.go` at HEAD. The tests above assume `packet.NewPacket([]byte) *packet.Packet` (used in sibling `idktype_test.go`/`seqtype_test.go`). If the constructor differs (e.g. `&packet.Packet{Data: ...}` literal or `packet.New(...)`), match the local idiom verbatim — same goes for `Pos` field accessor. Do **not** invent a new helper.

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run SpotanimType -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/objtype/...
```
Expected: PASS — all 16 SpotanimType tests pass; race detector clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/spotanimtype.go pkg/objtype/spotanimtype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-58 T1 — SpotanimType config port

Ports Engine-TS/src/cache/config/SpotanimType.ts (105 LOC) into
pkg/objtype following the idktype.go template. Dual-source loader
(server/spotanim.dat + client/config jag spotanim.dat) builds a
SpotanimTypeConfigs registry with 6-slot RecolS/RecolD silent-discard
guards on out-of-range opcodes. Tests cover all 14 decode arms +
parse-empty-count + missing-file-silent.

Refs NAI-36-D2 (closure pending T4)."
```

---

## Task 2 — `modules/world/server.go` wire-in

**Files:**
- Modify: `modules/world/server.go` (add field at line 87-88 region; add load after line 247 region)

- [ ] **Step 1: Pre-flight — read current server.go bootstrap layout**

Run:
```bash
rg -n 'idkTypes\|seqTypes\|seqFrames' modules/world/server.go
```
Expected: confirms field declaration block (~line 87-88) and load wire-in block (~line 236-247). Use these line numbers as edit anchors; do not assume the exact numbers — match by surrounding context.

- [ ] **Step 2: Add field declaration**

In `modules/world/server.go`, locate the `seqTypes  *objtype.SeqTypeConfigs` field declaration (around line 88) and add immediately after:

```go
spotanimTypes *objtype.SpotanimTypeConfigs
```

- [ ] **Step 3: Add load wire-in**

In the same file, locate the `s.seqTypes = seqTypes` assignment (around line 247) and add immediately after:

```go
spotanimTypes, err := objtype.LoadSpotanimTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load spotanim types: %w", err)
}
s.spotanimTypes = spotanimTypes
```

- [ ] **Step 4: Verify compile**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: clean build (no consumers yet — the field is wire-in only).

- [ ] **Step 5: Verify world tests still green**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```
Expected: all existing tests pass; SpotanimType registry is loaded but unread.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "feat(world): NAI-58 T2 — wire SpotanimType registry into Server

Adds Server.spotanimTypes field; bootstrap loads via
objtype.LoadSpotanimTypes(cfg.CachePath) alongside seqTypes. No
consumer yet — T3 adds the Configs.SpotAnimType accessor; T4 wires
checkSpotAnimType.

Refs NAI-36-D2 (closure pending T4)."
```

---

## Task 3 — `Configs.SpotAnimType` accessor + mock stubs

**Files:**
- Modify: `pkg/script/configs.go`
- Modify: `modules/world/server_configs.go`
- Modify: `pkg/script/handlers_db_test.go`
- Modify: `pkg/script/handlers_loc_test.go`
- Modify: `pkg/script/handlers_config_test.go`

- [ ] **Step 1: Pre-flight re-grep**

Run:
```bash
rg -n "IdkType\(id" pkg/ modules/
```
Expected: 1 production accessor (`server_configs.go:81`), 1 interface decl (`configs.go:18`), 1 production caller (`handlers_player.go:172`), 3 test mocks (`handlers_db_test.go:26`, `handlers_loc_test.go:27`, `handlers_config_test.go:29`). Any extra implementor surfaced here needs a stub added in this task.

- [ ] **Step 2: Extend the `Configs` interface**

In `pkg/script/configs.go`, locate the `IdkType(id int) *objtype.IdkType` line (around line 18) and add immediately after:

```go
SpotAnimType(id int) *objtype.SpotanimType
```

- [ ] **Step 3: Add production accessor**

In `modules/world/server_configs.go`, locate the `IdkType` accessor (lines 81-89) and append after it (before the `DbTableType` accessor on line 91):

```go
func (c serverConfigsView) SpotAnimType(id int) *objtype.SpotanimType {
	if c.s == nil || c.s.spotanimTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.spotanimTypes.Configs) {
		return nil
	}
	return c.s.spotanimTypes.Configs[id]
}
```

- [ ] **Step 4: Add `fakeDbConfigs` stub**

In `pkg/script/handlers_db_test.go`, locate the `IdkType` stub (line 26) and add immediately after:

```go
func (f *fakeDbConfigs) SpotAnimType(id int) *objtype.SpotanimType { return nil }
```

- [ ] **Step 5: Add `fakeConfigs` stub**

In `pkg/script/handlers_loc_test.go`, locate the `IdkType` stub (line 27) and add immediately after:

```go
func (f *fakeConfigs) SpotAnimType(id int) *objtype.SpotanimType { return nil }
```

- [ ] **Step 6: Add `mockConfigs` field + accessor**

In `pkg/script/handlers_config_test.go`:

(a) Locate the `mockConfigs` struct definition (around line 11) and add a `spotAnimTypes` field alongside `idks`:

```go
spotAnimTypes map[int]*objtype.SpotanimType
```

(b) Locate the `IdkType` accessor (line 29) and add immediately after:

```go
func (m *mockConfigs) SpotAnimType(id int) *objtype.SpotanimType    { return m.spotAnimTypes[id] }
```

> **Note:** the trailing whitespace + `{ return m.<map>[id] }` form mirrors the existing `IdkType` accessor exactly. If `gofmt` re-aligns the column, accept the re-alignment — do not fight gofmt.

- [ ] **Step 7: Verify compile**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: clean build; all existing tests still pass (the new interface method is satisfied by every implementor; no consumers yet).

- [ ] **Step 8: Commit**

```bash
git add pkg/script/configs.go modules/world/server_configs.go \
  pkg/script/handlers_db_test.go pkg/script/handlers_loc_test.go \
  pkg/script/handlers_config_test.go
git commit --no-gpg-sign -m "feat(script): NAI-58 T3 — Configs.SpotAnimType accessor

Extends the Configs interface with SpotAnimType(id) -> *objtype.SpotanimType.
Adds production impl on serverConfigsView (mirrors IdkType shape) and
stubs on the three test mocks. mockConfigs gains a spotAnimTypes
map[int]*objtype.SpotanimType for per-test seeding.

Refs NAI-36-D2 (closure pending T4)."
```

---

## Task 4 — Tighten `checkSpotAnimType` (TDD)

**Files:**
- Modify: `pkg/script/handlers_map.go` (lines 207-217 + line 233)

- [ ] **Step 1: Pre-flight re-grep**

Run:
```bash
rg -n '\bcheckSpotAnimType\(' pkg modules cmd
```
Expected: definition at `handlers_map.go:212`, sole caller at `handlers_map.go:233`. Any other caller needs the `s, ` prefix added at the call site in this task.

- [ ] **Step 2: Write the failing tests**

In `pkg/script/handlers_map_test.go`, append to the file (we'll add the full test bodies in T5; for T4 we just need a red→green seam). Skip this step — T5 covers the test extension. T4's red→green is the existing `TestSpotAnimMap_PopsValidatesAndDelegates` failing under the new presence-check requirement.

- [ ] **Step 3: Tighten `checkSpotAnimType`**

In `pkg/script/handlers_map.go`, replace lines 207-217 (the `// checkSpotAnimType ...` doc-comment block + function body) with:

```go
// checkSpotAnimType validates a spotanim type id by mirroring TS
// SpotAnimTypeValid (ScriptValidators.ts). Rejects negatives and
// any id not present in the SpotanimType config registry. Closes
// deviation NAI-36-D2.
func checkSpotAnimType(s *ScriptState, id int, op string) error {
	if id < 0 {
		return fmt.Errorf("%s: invalid spotanim id (%d)", op, id)
	}
	if s.Configs.SpotAnimType(id) == nil {
		return fmt.Errorf("%s: invalid spotanim id (%d)", op, id)
	}
	return nil
}
```

- [ ] **Step 4: Update sole caller in `handleSpotAnimMap`**

In the same file, locate line 233 (`if err := checkSpotAnimType(spotanim, "SPOTANIM_MAP"); err != nil {`) and change to:

```go
if err := checkSpotAnimType(s, spotanim, "SPOTANIM_MAP"); err != nil {
```

- [ ] **Step 5: Verify compile (tests will fail until T5 wires Configs into the test path)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: clean compile.

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run SpotAnim -v
```
Expected: `TestSpotAnimMap_PopsValidatesAndDelegates` and `TestSpotAnimMap_ZeroDelayPassesThrough` FAIL with nil-pointer dereference on `s.Configs.SpotAnimType` (since `runMapOp` does not yet wire `Configs:`). `TestSpotAnimMap_NegativeSpotanimIDErrors` and `TestSpotAnimMap_InvalidCoordErrors` may still PASS (they short-circuit before the Configs deref).

This RED state is the T4→T5 boundary. **Do not commit yet** — T5 lands the test wiring required for green. Continue to T5 in the same dispatch.

> **Implementer convention note:** This task intentionally lands a temporarily-red state. Per `latent_bug_at_migration_boundary.md`, the clean-cutover-then-fix shape is OK when the next task immediately fixes it. The T4 commit lands together with T5's test changes as a single commit (see T5 Step 8) — do **not** create a separate T4 commit.

---

## Task 5 — Extend `handlers_map_test.go` (lands T4+T5 together)

**Files:**
- Modify: `pkg/script/handlers_map_test.go`

- [ ] **Step 1: Extend `runMapOp` signature**

In `pkg/script/handlers_map_test.go`, locate the `runMapOp` function (around line 324) and change its signature to thread a `Configs`:

```go
// runMapOp executes a single map opcode against the given world fixture
// and returns the post-execution state. Pass c=nil for tests that
// don't exercise the Configs lookup.
func runMapOp(t *testing.T, w WorldVars, c Configs, op Opcode, intInputs []int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := &ScriptState{
		Script:      sf,
		World:       w,
		Configs:     c,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return state
}
```

- [ ] **Step 2: Update existing `runMapOp` callers**

Update all `runMapOp` callers in this file to pass a `Configs` argument:

- `TestMapBlocked_MembersWorldClearTilePushes0` (around line 360):
  ```go
  state := runMapOp(t, w, nil, OpMapBlocked, []int{...})
  ```
- `TestMapBlocked_MembersWorldBlockedTilePushes1` (around line 370): same shape, `nil` for Configs.
- `TestMapBlocked_F2PWorldNonF2PTilePushes1` (around line 387): `nil` for Configs.
- `TestMapBlocked_F2PWorldF2PTilePushesIsBlocked` (around line 402): `nil` for Configs.
- `TestSpotAnimMap_PopsValidatesAndDelegates` (around line 425): pass a seeded `mockConfigs`:
  ```go
  m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}
  state := runMapOp(t, w, m, OpSpotAnimMap, []int{spotanim, coord, height, delay})
  ```
- `TestSpotAnimMap_ZeroDelayPassesThrough` (around line 491): same shape, `m` seeded with `spotanim=200`:
  ```go
  m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}
  _ = runMapOp(t, w, m, OpSpotAnimMap, []int{spotanim, coord, height, delay})
  ```

- [ ] **Step 3: Update direct `&ScriptState{}` callers**

For the two SPOTANIM_MAP tests that build ScriptState directly:

- `TestSpotAnimMap_InvalidCoordErrors` (around line 449): add `Configs: &mockConfigs{}` to the struct literal (the coord check fires before any Configs deref, so an empty mock is fine).
- `TestSpotAnimMap_NegativeSpotanimIDErrors` (around line 473): add `Configs: &mockConfigs{}` (the `id < 0` short-circuit fires first).

Both edits look like:

```go
state := &ScriptState{
	World:       w,
	Configs:     &mockConfigs{},
	IntStack:    make([]int, StackCapacity),
	StringStack: make([]string, StackCapacity),
}
```

- [ ] **Step 4: Retire NAI-36-D2 doc-comment**

Locate the `// NAI-36-D2: SpotAnimType config-port absent at HEAD. ...` comment block at line 471 (above `TestSpotAnimMap_NegativeSpotanimIDErrors`). Replace with:

```go
// Pins post-NAI-58 negative-id rejection: checkSpotAnimType errors on
// id < 0 before any Configs lookup.
```

- [ ] **Step 5: Add new positive-arm test**

Append to `pkg/script/handlers_map_test.go` (after `TestSpotAnimMap_ZeroDelayPassesThrough`):

```go
// TestSpotAnimMap_RegisteredIdPasses pins the positive arm of the
// post-NAI-58 SpotAnimTypeValid mirror: a registered id reaches
// World.AnimMap with the spotanim untouched.
func TestSpotAnimMap_RegisteredIdPasses(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{
		spotAnimTypes: map[int]*objtype.SpotanimType{
			7: objtype.NewSpotanimType(7),
		},
	}

	const spotanim, height, delay = 7, 50, 5
	const level, x, z = 0, 3200, 3300
	coord := (level << 28) | (x << 14) | z

	_ = runMapOp(t, w, m, OpSpotAnimMap, []int{spotanim, coord, height, delay})

	if len(w.animMapCalls) != 1 {
		t.Fatalf("animMapCalls: got %d, want 1", len(w.animMapCalls))
	}
	got := w.animMapCalls[0]
	if got.spotanim != spotanim {
		t.Errorf("spotanim: got %d, want %d", got.spotanim, spotanim)
	}
}
```

- [ ] **Step 6: Add new negative-arm tests**

Append to `pkg/script/handlers_map_test.go`:

```go
// TestSpotAnimMap_UnregisteredIdRejects pins the post-NAI-58
// SpotAnimTypeValid mirror: an id that's non-negative but absent
// from the registry is rejected.
func TestSpotAnimMap_UnregisteredIdRejects(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{
		spotAnimTypes: map[int]*objtype.SpotanimType{
			7: objtype.NewSpotanimType(7),
		},
	}
	state := &ScriptState{
		World:       w,
		Configs:     m,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(8) // unregistered spotanim id
	state.PushInt((0 << 28) | (3200 << 14) | 3300)
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("unregistered spotanim id: got %v, want SPOTANIM_MAP error", err)
	}
	if len(w.animMapCalls) != 0 {
		t.Errorf("animMapCalls on error path: got %d, want 0", len(w.animMapCalls))
	}
}

// TestSpotAnimMap_NilEntryRejects covers the registry-has-key-but-nil-value
// edge: mockConfigs.spotAnimTypes[7] = nil → SpotAnimType(7) returns nil
// → validation rejects.
func TestSpotAnimMap_NilEntryRejects(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{
		spotAnimTypes: map[int]*objtype.SpotanimType{
			7: nil,
		},
	}
	state := &ScriptState{
		World:       w,
		Configs:     m,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(7) // key present but value is nil
	state.PushInt((0 << 28) | (3200 << 14) | 3300)
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("nil-value spotanim id: got %v, want SPOTANIM_MAP error", err)
	}
}
```

- [ ] **Step 7: Run all tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run SpotAnim -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run MapBlocked -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```
Expected:
- All 7 SpotAnim tests pass (4 existing updated + 3 new).
- All 4 MapBlocked tests still pass (only Configs argument changed to nil).
- Full race-detector run clean.

- [ ] **Step 8: Commit T4 + T5 together**

```bash
git add pkg/script/handlers_map.go pkg/script/handlers_map_test.go
git commit --no-gpg-sign -m "feat(script): NAI-58 T4+T5 — tighten checkSpotAnimType to mirror SpotAnimTypeValid

Replaces the NAI-36-D2 range-only validator with a presence check via
s.Configs.SpotAnimType(id). Threads Configs through runMapOp and the
two direct ScriptState constructors in handlers_map_test.go. Adds
positive-arm (registered id passes), negative-arm (unregistered id
rejects), and nil-entry (registry-has-key-but-nil) tests; retires the
NAI-36-D2 attribution doc-comment.

Refs NAI-36-D2 (close commit at T6)."
```

---

## Task 6 — Close commit

**Files:**
- Modify: `nai_followups.md` (memory at `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`)

- [ ] **Step 1: Stale-deviation grep**

Run:
```bash
rg -n "NAI-36-D2" pkg/ modules/ cmd/
```
Expected: zero hits. If any surface, edit them out before continuing.

- [ ] **Step 2: Final test sweep**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```
Expected: all green; no vet warnings.

- [ ] **Step 3: Append NAI-58 close section to `nai_followups.md`**

Use the Write/Edit tool (per memory-write sandbox quirk — never `bash` redirects) to append a new `## NAI-58 — CLOSED 2026-05-XX` section to `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`, modeled on the NAI-57 section at line ~2824. Required fields:

- **Scope:** one sentence
- **Cadence:** Full sub-spec, single bundle, 6 tasks
- **Close commit:** `<close-sha>` (T1: ..., T2: ..., T3: ..., T4+T5: ..., T6: ...)
- **Spec:** `docs/superpowers/specs/2026-05-01-nai-58-spotanimtype-config-port-design.md`
- **Plan:** `docs/superpowers/plans/2026-05-01-nai-58-spotanimtype-config-port.md`
- **Follow-ups closed:** NAI-36-D2 (anim-playback consumer of `SpotanimType` registry now wired in `checkSpotAnimType`)
- **Deviations opened:** none
- **Deviations closed:** NAI-36-D2
- **Deviation tally:** 20 → 19
- **Plan-author misses caught:** capture any controller pre-flight catches surfaced during T1-T6 dispatch (e.g. constructor name mismatch, mock shape divergence, etc.)

Also locate the existing `### NAI-36 close` section (around line 2083 of nai_followups.md) and update the NAI-36-D2 status footnote (or wherever NAI-36-D2 is mentioned) with: "**Resolved 2026-05-XX (NAI-58, commits ...)**".

- [ ] **Step 4: Commit close**

```bash
git add docs/ # in case any spec/plan polish was needed
# nai_followups.md is in ~/.claude/projects/... — outside the repo, no `git add` needed
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-58 — SpotanimType config port; closes NAI-36-D2

Six implementation commits + this close commit. Tally 20 → 19.

Closes memory: NAI-36-D2
EOF
)"
```

> **Note:** if T1-T5 have already produced commits with all production changes, T6 may legitimately be `--allow-empty` (close-only). Verify with `git status` and `git log --oneline -10` before committing. If working-tree changes remain (e.g. nai_followups.md is somehow tracked, or polish was needed), drop the `--allow-empty` flag.

- [ ] **Step 5: Final verification**

Run:
```bash
git log --oneline -10
rg -n "NAI-36-D2" pkg/ modules/ cmd/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```
Expected:
- Last 7 commits: T1 spotanimtype port, T2 server wire-in, T3 Configs accessor, T4+T5 checkSpotAnimType + tests, T6 close + (optional spec/plan), all `Refs NAI-36-D2` chained to `Closes memory: NAI-36-D2` on the close.
- Zero NAI-36-D2 hits.
- Race-detector clean.

---

## Self-review

- **Spec coverage:** Every "In scope" item from the spec maps to a task — T1 covers `pkg/objtype/spotanimtype.go` + tests; T2 covers `server.go` field + load; T3 covers `Configs` interface + production accessor + 3 mock stubs; T4 covers `handlers_map.go` tightening; T5 covers `handlers_map_test.go` extension + NAI-36-D2 doc-comment retirement; T6 covers close + memory + tally.
- **Placeholder scan:** Two implementer-notes flag verify-against-HEAD checks for things this controller cannot fully pin (the `writeOneEntryJag` test helper name, the `packet.NewPacket`/`Pos` constructor idiom). Both are explicit "match the local idiom" instructions, not TBDs — the implementer must read sibling test files at HEAD before writing.
- **Type consistency:** `SpotanimType` (struct, file casing) vs `SpotAnimType` (interface method, validator-mirror casing) is intentional and called out in spec § Task 3. Field name `spotanimTypes` (server) vs `spotAnimTypes` (mockConfigs map) is similarly intentional — the server uses `spotanim` to match `idkTypes`/`seqTypes` (lowercase noun), while the mockConfigs map uses `spotAnim` to match the interface method name (which is the natural test-author reach pattern).
- **Test wiring:** T4 lands a temporarily-red state; T5 produces the green commit. Single combined commit at T5 Step 8 per `latent_bug_at_migration_boundary.md`.
- **Pre-flight gates:** controller verifies before T1 (NAI-36-D2 sites count = 2), before T3 (Configs implementor count), before T4 (checkSpotAnimType caller count = 1).
