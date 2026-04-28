# NAI-46 — IdkType Config Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `IdkType` config from TS, wire it into the server, and retire deviations NAI-45-D3 (missing idk validation in `handleIdkSaveDesign`) and NAI-44-D-CONTINUEWALK-UNUSED (`tryInteract`'s unused parameter).

**Architecture:** New `pkg/objtype/idktype.go` follows the dual-source loader pattern established by `npctype.go` (server/idk.dat + Jagfile client/config → idk.dat). The registry is stored on `*Server` alongside `npcTypes`/`huntTypes`. `handleIdkSaveDesign` is promoted from a free function to a `(*Server)` method so it can access the registry; the `gameHandlers[52]` registration is updated to a package-level adapter that delegates to the method. `tryInteract`'s `continueWalk bool` parameter (never read) is removed.

**Tech Stack:** Go 1.26+ with modern syntax (`for i := range N`, range-over-slice, etc.). All `go` commands must be prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`. All commits use `--no-gpg-sign`.

---

## File Map

| Action | Path | What changes |
|--------|------|-------------|
| CREATE | `pkg/objtype/idktype.go` | `IdkType` struct + `NewIdkType` + `Decode` + `IdkTypeConfigs` + `LoadIdkTypes` + `parseIdkTypes` |
| CREATE | `pkg/objtype/idktype_test.go` | Decode + loader unit tests |
| MODIFY | `modules/world/server.go:85-86` | Add `idkTypes` field |
| MODIFY | `modules/world/server.go:228-232` | Add `LoadIdkTypes` call after `huntTypes` load |
| MODIFY | `modules/world/handler_interface.go:46-93` | Promote `handleIdkSaveDesign` to `(*Server)` method + add idk validation loop + remove NAI-45-D3 deviation comment |
| MODIFY | `modules/world/handlers_game.go:62` | Rename adapter to `handleIdkSaveDesignGame` + add adapter func |
| MODIFY | `modules/world/handler_interface_test.go` | Migrate 5 existing tests + add `buildIdkTypes` helper + 6 new validation tests |
| MODIFY | `modules/world/interaction.go:169,192,249-252,268` | Remove `continueWalk bool` param + update 2 call sites + retire deviation comment |

---

## Task 1 — `IdkType` struct, constructor, and `Decode`

**Files:**
- Create: `pkg/objtype/idktype.go`
- Create: `pkg/objtype/idktype_test.go`

TS reference: `Engine-TS/src/cache/config/IdkType.ts:62-89` (decode) and `:7-70` (struct + defaults).

- [ ] **Step 1.1: Write failing tests for `NewIdkType` and `Decode`**

Create `pkg/objtype/idktype_test.go`:

```go
package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewIdkTypeDefaults(t *testing.T) {
	idk := NewIdkType(5)
	if idk.ID != 5 {
		t.Errorf("ID: got %d, want 5", idk.ID)
	}
	if idk.Type != -1 {
		t.Errorf("Type: got %d, want -1", idk.Type)
	}
	if idk.Disable {
		t.Error("Disable: got true, want false")
	}
	if idk.Models != nil {
		t.Errorf("Models: got %v, want nil", idk.Models)
	}
	want := [5]uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF}
	if idk.Heads != want {
		t.Errorf("Heads: got %v, want %v", idk.Heads, want)
	}
	for i, v := range idk.RecolS {
		if v != 0 {
			t.Errorf("RecolS[%d]: got %d, want 0", i, v)
		}
	}
	for i, v := range idk.RecolD {
		if v != 0 {
			t.Errorf("RecolD[%d]: got %d, want 0", i, v)
		}
	}
}

// decodeIdk builds a writer packet, appends a 0-terminator, flips to reader,
// and runs DecodeType on a fresh NewIdkType(0). Mirrors hunttype_test.go style.
func decodeIdk(build func(*packet2.Packet)) (*IdkType, error) {
	w := packet2.NewPacket(nil)
	build(w)
	w.P1(0) // terminator
	r := packet2.NewPacket(w.Bytes())
	idk := NewIdkType(0)
	err := DecodeType(r, idk)
	return idk, err
}

func TestIdkTypeDecode_Type(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(1); p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.Type != 3 {
		t.Errorf("Type: got %d, want 3", idk.Type)
	}
}

func TestIdkTypeDecode_Models(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) {
		p.P1(2)
		p.P1(2)       // count = 2
		p.P2(0x0100) // model[0] = 256
		p.P2(0x0200) // model[1] = 512
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if len(idk.Models) != 2 {
		t.Fatalf("Models len: got %d, want 2", len(idk.Models))
	}
	if idk.Models[0] != 256 {
		t.Errorf("Models[0]: got %d, want 256", idk.Models[0])
	}
	if idk.Models[1] != 512 {
		t.Errorf("Models[1]: got %d, want 512", idk.Models[1])
	}
}

func TestIdkTypeDecode_Disable(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if !idk.Disable {
		t.Error("Disable: got false, want true")
	}
}

func TestIdkTypeDecode_RecolS(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(40); p.P2(0x0102) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.RecolS[0] != 0x0102 {
		t.Errorf("RecolS[0]: got %d, want %d", idk.RecolS[0], 0x0102)
	}
}

func TestIdkTypeDecode_RecolD(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(50); p.P2(0x0304) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.RecolD[0] != 0x0304 {
		t.Errorf("RecolD[0]: got %d, want %d", idk.RecolD[0], 0x0304)
	}
}

func TestIdkTypeDecode_Heads(t *testing.T) {
	// code 60 → Heads[0]
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(60); p.P2(0x0506) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.Heads[0] != 0x0506 {
		t.Errorf("Heads[0]: got %d, want %d", idk.Heads[0], 0x0506)
	}

	// code 64 → Heads[4]
	idk2, err := decodeIdk(func(p *packet2.Packet) { p.P1(64); p.P2(0x0708) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk2.Heads[4] != 0x0708 {
		t.Errorf("Heads[4]: got %d, want %d", idk2.Heads[4], 0x0708)
	}

	// code 65 → out-of-range, Heads unchanged (guard: slot=5 >= 5)
	idk3, err := decodeIdk(func(p *packet2.Packet) { p.P1(65); p.P2(0x090A) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	want := [5]uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF}
	if idk3.Heads != want {
		t.Errorf("Heads after code 65: got %v, want all 0xFFFF (out-of-range guard)", idk3.Heads)
	}
}

func TestIdkTypeDecode_DebugName(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(250); p.PJStrLF("test_idk") })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.DebugName != "test_idk" {
		t.Errorf("DebugName: got %q, want \"test_idk\"", idk.DebugName)
	}
}

func TestIdkTypeDecode_UnknownCode(t *testing.T) {
	_, err := decodeIdk(func(p *packet2.Packet) { p.P1(99) })
	if err == nil {
		t.Error("want error for unknown code 99, got nil")
	}
}
```

- [ ] **Step 1.2: Run tests — expect compile failure (IdkType undefined)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestNewIdkType|TestIdkTypeDecode' -v 2>&1 | head -20
```

Expected: compile error mentioning `NewIdkType` or `IdkType` undefined.

- [ ] **Step 1.3: Create `pkg/objtype/idktype.go` with struct, constructor, and Decode**

```go
package objtype

import (
	"fmt"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// IdkType is a single idk.dat config record (identity-kit / character design slot).
// Mirrors Engine-TS/src/cache/config/IdkType.ts.
type IdkType struct {
	ConfigType
	Type    int       // body-part slot; -1 = unset
	Models  []uint16  // nil = no models
	Heads   [5]uint16 // 0xFFFF = unset (TS Uint16Array(5).fill(-1))
	RecolS  [6]uint16
	RecolD  [6]uint16
	Disable bool
}

// NewIdkType returns an IdkType with TS-faithful defaults.
func NewIdkType(id int) *IdkType {
	return &IdkType{
		ConfigType: ConfigType{ID: id},
		Type:       -1,
		Heads:      [5]uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF},
	}
}

// Decode dispatches on the idk config opcode, matching TS IdkType.decode
// at Engine-TS/src/cache/config/IdkType.ts:62-89.
func (t *IdkType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		t.Type = int(dat.G1())
	case 2:
		count := dat.G1()
		t.Models = make([]uint16, count)
		for i := range count {
			t.Models[i] = dat.G2()
		}
	case 3:
		t.Disable = true
	case 40, 41, 42, 43, 44, 45, 46, 47, 48, 49:
		t.RecolS[code-40] = dat.G2()
	case 50, 51, 52, 53, 54, 55, 56, 57, 58, 59:
		t.RecolD[code-50] = dat.G2()
	case 60, 61, 62, 63, 64, 65, 66, 67, 68, 69:
		// TS heads[] is 5-element; codes 65-69 are out-of-range. Guard to
		// avoid panic; consume the G2 regardless so the packet cursor advances.
		slot := code - 60
		v := dat.G2()
		if slot < 5 {
			t.Heads[slot] = v
		}
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized idk config code %d", code)
	}
	return nil
}
```

- [ ] **Step 1.4: Run tests — expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestNewIdkType|TestIdkTypeDecode' -v 2>&1 | tail -20
```

Expected: all listed tests PASS.

- [ ] **Step 1.5: Commit**

```bash
git add pkg/objtype/idktype.go pkg/objtype/idktype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-46 T1 — IdkType struct, constructor, and Decode

Ports IdkType from Engine-TS/src/cache/config/IdkType.ts.
Includes bounds-guard for Heads codes 65-69 to avoid panic.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — `IdkTypeConfigs` registry and `LoadIdkTypes` loader

**Files:**
- Modify: `pkg/objtype/idktype.go` (add registry + loader)
- Modify: `pkg/objtype/idktype_test.go` (add loader tests)

Pattern reference: `npctype.go:343-402` (dual-source loader with `io.LoadJagfile` + `client.Pos = 2`).
Pattern reference: `hunttype.go:163-201` (silent-on-missing `ErrNotExist` handling).

- [ ] **Step 2.1: Write failing loader tests**

Add to `pkg/objtype/idktype_test.go`:

```go
import (
	"os"
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)
```

(Update the import block at the top of the file — add `"os"` and `"path/filepath"`.)

Then append these tests:

```go
// TestLoadIdkTypes_MissingServerDat pins that LoadIdkTypes returns an empty
// registry (not an error) when server/idk.dat is absent, matching TS
// IdkType.load's early-return on missing file.
func TestLoadIdkTypes_MissingServerDat(t *testing.T) {
	dir := t.TempDir()
	// No server/idk.dat created — directory exists but file is absent.
	configs, err := LoadIdkTypes(dir)
	if err != nil {
		t.Fatalf("LoadIdkTypes: want nil error on missing file, got %v", err)
	}
	if configs == nil {
		t.Fatal("configs: want non-nil registry, got nil")
	}
	if len(configs.Configs) != 0 {
		t.Errorf("Configs: want empty slice, got %d entries", len(configs.Configs))
	}
	if len(configs.ConfigNames) != 0 {
		t.Errorf("ConfigNames: want empty map, got %d entries", len(configs.ConfigNames))
	}
}

// TestLoadIdkTypes_FromPack loads IdkTypes from the real pack directory.
// Skipped when the pack data is absent (CI / clean checkout).
func TestLoadIdkTypes_FromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "idk.dat")); err != nil {
		t.Skipf("no pack data: %v", err)
	}
	configs, err := LoadIdkTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadIdkTypes: %v", err)
	}
	if len(configs.Configs) == 0 {
		t.Fatal("expected at least one IdkType, got 0")
	}
}
```

- [ ] **Step 2.2: Run tests — expect compile failure (LoadIdkTypes/IdkTypeConfigs undefined)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestLoadIdkTypes' -v 2>&1 | head -10
```

Expected: compile error.

- [ ] **Step 2.3: Add `IdkTypeConfigs`, `LoadIdkTypes`, and `parseIdkTypes` to `pkg/objtype/idktype.go`**

Add these imports at the top of `idktype.go` (replace the existing import block):

```go
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)
```

Append to `pkg/objtype/idktype.go`:

```go
// IdkTypeConfigs is the parsed registry of all identity-kit config records.
type IdkTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*IdkType
}

// LoadIdkTypes parses server/idk.dat + client/config jag → idk.dat into
// an IdkTypeConfigs registry. Returns an empty registry with nil error when
// server/idk.dat is absent (silent-on-missing, matching TS IdkType.load at
// Engine-TS/src/cache/config/IdkType.ts:14-18).
func LoadIdkTypes(dir string) (*IdkTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "idk.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &IdkTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}

	clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}

	return parseIdkTypes(server, clientJag)
}

func parseIdkTypes(server *packet2.Packet, clientJag *io.Jagfile) (*IdkTypeConfigs, error) {
	count := int(server.G2())
	configs := make([]*IdkType, count)
	configNames := make(map[string]int, count)

	client, err := clientJag.Read("idk.dat")
	if err != nil {
		return nil, err
	}
	client.Pos = 2 // skip client-side count header (same as npctype.go:377)

	for id := range count {
		config := NewIdkType(id)
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

	return &IdkTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

- [ ] **Step 2.4: Run tests — expect pass (missing-file test passes; pack test skips or passes)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestLoadIdkTypes' -v 2>&1 | tail -10
```

Expected: `TestLoadIdkTypes_MissingServerDat` PASS; `TestLoadIdkTypes_FromPack` either PASS or SKIP.

- [ ] **Step 2.5: Run full objtype suite to check for regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -v 2>&1 | grep -E 'PASS|FAIL|SKIP' | tail -20
```

Expected: all PASS or SKIP, no FAIL.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/objtype/idktype.go pkg/objtype/idktype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-46 T2 — IdkTypeConfigs registry and LoadIdkTypes loader

Dual-source loader (server/idk.dat + Jagfile client/config→idk.dat)
following npctype.go pattern. Silent-on-missing matching TS IdkType.load.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Wire registry into server + promote handler to Server method

**Files:**
- Modify: `modules/world/server.go`
- Modify: `modules/world/handler_interface.go`
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/handler_interface_test.go` (5 existing tests — migration required to compile)

**Important:** After the handler signature change, the 5 existing tests in `handler_interface_test.go` call `handleIdkSaveDesign(p, ...)` which no longer exists as a free function. The tests get a compile error. Steps 3.6-3.10 fix them as part of this task. All changes are committed together.

- [ ] **Step 3.1: Add `idkTypes` field to `Server` in `modules/world/server.go`**

After `huntTypes *objtype.HuntTypeConfigs` at line 86, add:

```go
idkTypes     *objtype.IdkTypeConfigs
```

Result at lines 85-87:
```go
npcTypes      *objtype.NPCTypeConfigs
huntTypes     *objtype.HuntTypeConfigs
idkTypes      *objtype.IdkTypeConfigs
```

- [ ] **Step 3.2: Add `LoadIdkTypes` call in `NewServer` in `modules/world/server.go`**

After `s.huntTypes = huntTypes` at line 232, add:

```go
idkTypes, err := objtype.LoadIdkTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load idk types: %w", err)
}
s.idkTypes = idkTypes
```

- [ ] **Step 3.3: Promote `handleIdkSaveDesign` to a `(*Server)` method in `modules/world/handler_interface.go`**

Replace the doc-comment block and signature at lines 46-55. The entire block to replace is:

```go
// handleIdkSaveDesign handles client opcode 52 (IDK_SAVEDESIGN).
// Body: u8 gender | u8[7] idkit (255 → -1) | u8[5] color.
//
// Validates allowDesign, gender ≤ 1, and color ranges. On pass: updates
// p.gender/body/colors and calls SetAppearanceInv to flag MaskAppearance
// (mirrors TS buildAppearance(player.appearanceInv) at IdkSaveDesignHandler.ts:37).
//
// DEVIATION NAI-45-D3: IdkType.get(idkit[i]) disable+type checks skipped —
// no IdkType config registry. Closure: IdkType-config sub-spec.
func handleIdkSaveDesign(p *Player, payload []byte) error {
```

Replace with:

```go
// handleIdkSaveDesign handles client opcode 52 (IDK_SAVEDESIGN).
// Body: u8 gender | u8[7] idkit (255 → -1) | u8[5] color.
//
// Validates allowDesign, gender ≤ 1, idk disable+type (via IdkType registry),
// and color ranges. On pass: updates p.gender/body/colors and calls
// SetAppearanceInv to flag MaskAppearance.
// Mirrors TS IdkSaveDesignHandler.ts:7-38.
func (s *Server) handleIdkSaveDesign(p *Player, payload []byte) error {
```

- [ ] **Step 3.4: Insert IdkType validation loop BEFORE the color loop in `handler_interface.go`**

After `if gender > 1 { return nil }` (currently at lines 64-65) and after the idkit decode loop (currently ending at line 75), insert the validation block. The insertion point is after line 75 (`idkit[i] = v`) and before `var color [5]int` (line 77).

Insert between the idkit decode block and the color decode block:

```go
	// IdkType validation — mirrors TS IdkSaveDesignHandler.ts:18-33.
	// TS order: idk loop before color loop.
	if s.idkTypes != nil {
		for i := range 7 {
			typ := i + gender*7
			if typ == 8 && idkit[i] == -1 { // female jaw exception (TS L21-23)
				continue
			}
			if idkit[i] < 0 || idkit[i] >= len(s.idkTypes.Configs) {
				return nil
			}
			idk := s.idkTypes.Configs[idkit[i]]
			if idk.Disable || idk.Type != typ {
				return nil
			}
		}
	}
```

The final function body (reference for verification):

```go
func (s *Server) handleIdkSaveDesign(p *Player, payload []byte) error {
	if len(payload) < 13 {
		return nil
	}
	if !p.allowDesign {
		return nil
	}

	gender := int(payload[0])
	if gender > 1 {
		return nil
	}

	var idkit [7]int
	for i := range 7 {
		v := int(payload[1+i])
		if v == 255 {
			v = -1
		}
		idkit[i] = v
	}

	// IdkType validation — mirrors TS IdkSaveDesignHandler.ts:18-33.
	// TS order: idk loop before color loop.
	if s.idkTypes != nil {
		for i := range 7 {
			typ := i + gender*7
			if typ == 8 && idkit[i] == -1 { // female jaw exception (TS L21-23)
				continue
			}
			if idkit[i] < 0 || idkit[i] >= len(s.idkTypes.Configs) {
				return nil
			}
			idk := s.idkTypes.Configs[idkit[i]]
			if idk.Disable || idk.Type != typ {
				return nil
			}
		}
	}

	var color [5]int
	for i := range 5 {
		color[i] = int(payload[8+i])
	}

	for i, c := range color {
		if c >= designBodyColorCount[i] {
			return nil
		}
	}

	p.gender = gender
	p.body = idkit
	p.colors = color
	p.SetAppearanceInv(p.appearanceInv)
	return nil
}
```

- [ ] **Step 3.5: Update adapter in `modules/world/handlers_game.go`**

At line 62, replace:
```go
gameHandlers[52] = handleIdkSaveDesign  // IDK_SAVEDESIGN
```
with:
```go
gameHandlers[52] = handleIdkSaveDesignGame // IDK_SAVEDESIGN
```

Then add the adapter function after `handleIfButton` (around line 107):

```go
func handleIdkSaveDesignGame(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleIdkSaveDesign(p, payload)
}
```

- [ ] **Step 3.6: Attempt compile — expect 5 errors from migrated tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/... 2>&1 | head -20
```

Expected: compile errors in `handler_interface_test.go` referencing `handleIdkSaveDesign` (now undefined as a free function).

- [ ] **Step 3.7: Migrate `TestHandleIdkSaveDesignAllowDesignFalse`**

In `handler_interface_test.go`, replace:
```go
func TestHandleIdkSaveDesignAllowDesignFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.allowDesign = false

	_ = handleIdkSaveDesign(p, idkPayload(0, [7]byte{0, 1, 2, 3, 4, 5, 6}, [5]byte{0, 0, 0, 0, 0}))

	if p.gender != 0 || p.body != [7]int{0, 10, 18, 26, 33, 36, 42} {
		t.Error("player state changed despite allowDesign=false")
	}
}
```
with:
```go
func TestHandleIdkSaveDesignAllowDesignFalse(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = false

	_ = s.handleIdkSaveDesign(p, idkPayload(0, [7]byte{0, 1, 2, 3, 4, 5, 6}, [5]byte{0, 0, 0, 0, 0}))

	if p.gender != 0 || p.body != [7]int{0, 10, 18, 26, 33, 36, 42} {
		t.Error("player state changed despite allowDesign=false")
	}
}
```

- [ ] **Step 3.8: Migrate `TestHandleIdkSaveDesignInvalidGender`**

Replace:
```go
func TestHandleIdkSaveDesignInvalidGender(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.allowDesign = true

	_ = handleIdkSaveDesign(p, idkPayload(2, [7]byte{}, [5]byte{}))

	if p.gender != 0 {
		t.Errorf("gender changed: got %d, want 0", p.gender)
	}
}
```
with:
```go
func TestHandleIdkSaveDesignInvalidGender(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true

	_ = s.handleIdkSaveDesign(p, idkPayload(2, [7]byte{}, [5]byte{}))

	if p.gender != 0 {
		t.Errorf("gender changed: got %d, want 0", p.gender)
	}
}
```

- [ ] **Step 3.9: Migrate `TestHandleIdkSaveDesignColorOutOfBounds`**

Replace:
```go
func TestHandleIdkSaveDesignColorOutOfBounds(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.allowDesign = true

	// color[0] max is 11 (count=12); send 12 → out of bounds.
	_ = handleIdkSaveDesign(p, idkPayload(0, [7]byte{}, [5]byte{12, 0, 0, 0, 0}))

	if p.gender != 0 {
		t.Errorf("state changed despite invalid color: gender=%d", p.gender)
	}
}
```
with:
```go
func TestHandleIdkSaveDesignColorOutOfBounds(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true

	// color[0] max is 11 (count=12); send 12 → out of bounds.
	// s.idkTypes is nil so the idk loop is skipped; color check still fires.
	_ = s.handleIdkSaveDesign(p, idkPayload(0, [7]byte{}, [5]byte{12, 0, 0, 0, 0}))

	if p.gender != 0 {
		t.Errorf("state changed despite invalid color: gender=%d", p.gender)
	}
}
```

- [ ] **Step 3.10: Migrate `TestHandleIdkSaveDesignSuccess` and restructure `TestHandleIdkSaveDesignIdkit255ConvertedToMinus1`**

Replace both tests. `Success` needs a seeded registry (use `buildIdkTypes`; that helper is added in Task 5 — for now inline a minimal fixture). `Idkit255ConvertedToMinus1` is restructured as `FemaleJaw255Accepted` since idkit=-1 is now valid ONLY for the female jaw slot.

Replace `TestHandleIdkSaveDesignSuccess`:

```go
func TestHandleIdkSaveDesignSuccess(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	p.appearanceInv = 0

	// Seed a minimal registry: 14 entries, entry[i].Type = i.
	s.idkTypes = buildIdkTypes(14)

	// gender=1 (female): expected types per slot = i+7 ∈ [7..13].
	// idkit values must match registry IDs whose Type == i+7.
	body := [7]byte{7, 8, 9, 10, 11, 12, 13}
	colors := [5]byte{0, 1, 2, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(1, body, colors))

	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
	for i, v := range body {
		if p.body[i] != int(v) {
			t.Errorf("body[%d]: got %d, want %d", i, p.body[i], v)
		}
	}
	for i, v := range colors {
		if p.colors[i] != int(v) {
			t.Errorf("colors[%d]: got %d, want %d", i, p.colors[i], v)
		}
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Error("MaskAppearance: want set, got unset")
	}
}
```

Replace `TestHandleIdkSaveDesignIdkit255ConvertedToMinus1`:

```go
// TestHandleIdkSaveDesignFemaleJaw255Accepted pins that wire value 255 is
// decoded to -1 AND accepted for the female jaw slot (gender=1, i=1, type=8).
// With idk validation active, idkit=-1 is only allowed at this slot.
func TestHandleIdkSaveDesignFemaleJaw255Accepted(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true

	// Seed registry: 14 entries (IDs 0..13, Type=0..13).
	s.idkTypes = buildIdkTypes(14)

	// gender=1, idkit[1]=255 → -1 (female jaw exception, type=8 skipped).
	// Other slots use valid female IDs (type = i+7).
	body := [7]byte{7, 255, 9, 10, 11, 12, 13} // slot 1 = 255 → -1
	colors := [5]byte{0, 0, 0, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(1, body, colors))

	if p.body[1] != -1 {
		t.Errorf("body[1]: got %d, want -1 (female jaw 255→-1)", p.body[1])
	}
	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
}
```

Note: `buildIdkTypes` is defined in Task 5. Since Tasks 3 and 5 are in the same test file, this order works: the helper is defined alongside the new tests in Task 5, and `TestHandleIdkSaveDesignSuccess` / `TestHandleIdkSaveDesignFemaleJaw255Accepted` reference it. If committing this task before Task 5, temporarily inline the helper or commit after Task 5's test additions.

**Practical sequencing:** commit Tasks 3+4 (the migration) together, then Task 5 adds `buildIdkTypes` + new tests in the same commit that also adds the validation loop. If there is a compile dependency on `buildIdkTypes`, do Tasks 3 and 5 atomically.

**Simpler approach:** Just add `buildIdkTypes` inline in this step rather than deferring to Task 5:

```go
// buildIdkTypes returns an IdkTypeConfigs with count entries.
// Entry i has Type=i and Disable=false.
// Covers male slots 0..6 (types 0..6) and female slots 0..6 (types 7..13)
// when count=14.
func buildIdkTypes(count int) *objtype.IdkTypeConfigs {
	configs := make([]*objtype.IdkType, count)
	for i := range count {
		c := objtype.NewIdkType(i)
		c.Type = i
		configs[i] = c
	}
	return &objtype.IdkTypeConfigs{
		ConfigNames: map[string]int{},
		Configs:     configs,
	}
}
```

Add this helper to `handler_interface_test.go` and also add `"github.com/zsrv/goscape/pkg/objtype"` to the import block.

- [ ] **Step 3.11: Compile check — no errors**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/... 2>&1
```

Expected: no output (clean build).

- [ ] **Step 3.12: Run migrated tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestHandleIdkSaveDesign' -v 2>&1 | tail -15
```

Expected: all PASS.

- [ ] **Step 3.13: Run full world suite for regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: PASS with no FAIL.

- [ ] **Step 3.14: Commit**

```bash
git add modules/world/server.go modules/world/handler_interface.go \
        modules/world/handlers_game.go modules/world/handler_interface_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-46 T3 — wire idkTypes registry + retire NAI-45-D3

- Server.idkTypes field + LoadIdkTypes call in NewServer.
- handleIdkSaveDesign promoted to (*Server) method; idk validation loop
  inserted before color loop (TS-faithful order, IdkSaveDesignHandler.ts:18-33).
- gameHandlers[52] updated to handleIdkSaveDesignGame adapter.
- 5 existing handler tests migrated to s.handleIdkSaveDesign calls.
- buildIdkTypes helper + 2 tests updated to seed registry.
Retires deviation NAI-45-D3.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — New IdkType validation tests

**Files:**
- Modify: `modules/world/handler_interface_test.go` (add 4 new rejection tests)

Note: `buildIdkTypes` was added in Task 3. `TestHandleIdkSaveDesignValidMale` and `TestHandleIdkSaveDesignValidFemaleWithJaw` extend the happy-path coverage; the rejection tests pin the validation branches.

- [ ] **Step 4.1: Write 4 new rejection tests (expected to PASS since validation is already wired)**

Append to `handler_interface_test.go`:

```go
// TestHandleIdkSaveDesignValidMale pins the male happy path with a seeded registry.
func TestHandleIdkSaveDesignValidMale(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)

	// gender=0: types 0..6, use registry IDs 0..6.
	body := [7]byte{0, 1, 2, 3, 4, 5, 6}
	colors := [5]byte{0, 1, 2, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, colors))

	if p.gender != 0 {
		t.Errorf("gender: got %d, want 0", p.gender)
	}
	for i, v := range body {
		if p.body[i] != int(v) {
			t.Errorf("body[%d]: got %d, want %d", i, p.body[i], v)
		}
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Error("MaskAppearance: want set, got unset")
	}
}

// TestHandleIdkSaveDesignValidFemaleWithJaw pins the female happy path
// where all 7 slots including jaw are valid IDs.
func TestHandleIdkSaveDesignValidFemaleWithJaw(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)

	// gender=1: types 7..13, use registry IDs 7..13.
	body := [7]byte{7, 8, 9, 10, 11, 12, 13}
	colors := [5]byte{0, 0, 0, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(1, body, colors))

	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
	for i, v := range body {
		if p.body[i] != int(v) {
			t.Errorf("body[%d]: got %d, want %d", i, p.body[i], v)
		}
	}
}

// TestHandleIdkSaveDesignDisabledIdk pins that a disabled IdkType rejects
// the whole packet (TS IdkSaveDesignHandler.ts:30: idk.disable check).
func TestHandleIdkSaveDesignDisabledIdk(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)
	s.idkTypes.Configs[0].Disable = true // slot 0 disabled

	// gender=0, idkit[0]=0 → disabled → rejected.
	body := [7]byte{0, 1, 2, 3, 4, 5, 6}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, [5]byte{}))

	if p.gender != 0 || p.masks&rsbuf.MaskAppearance != 0 {
		t.Error("state changed despite disabled idk: expected rejection")
	}
}

// TestHandleIdkSaveDesignWrongType pins that a type mismatch rejects the packet
// (TS IdkSaveDesignHandler.ts:30: idk.type != type check).
func TestHandleIdkSaveDesignWrongType(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)
	s.idkTypes.Configs[0].Type = 99 // wrong type for slot 0 (expected 0)

	body := [7]byte{0, 1, 2, 3, 4, 5, 6}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, [5]byte{}))

	if p.gender != 0 || p.masks&rsbuf.MaskAppearance != 0 {
		t.Error("state changed despite wrong idk type: expected rejection")
	}
}

// TestHandleIdkSaveDesignOutOfRangeIdkit pins that an idkit value >= registry
// length is rejected (bounds check before Configs[idkit[i]] dereference).
func TestHandleIdkSaveDesignOutOfRangeIdkit(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14) // IDs 0..13

	// idkit[0] = 14 → out of range (len=14).
	body := [7]byte{14, 1, 2, 3, 4, 5, 6}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, [5]byte{}))

	if p.gender != 0 || p.masks&rsbuf.MaskAppearance != 0 {
		t.Error("state changed despite out-of-range idkit: expected rejection")
	}
}
```

- [ ] **Step 4.2: Run new tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... \
  -run 'TestHandleIdkSaveDesignValid|TestHandleIdkSaveDesignDisabled|TestHandleIdkSaveDesignWrongType|TestHandleIdkSaveDesignOutOfRange' \
  -v 2>&1 | tail -15
```

Expected: all 4 new tests PASS (validation is already wired from Task 3).

- [ ] **Step 4.3: Run full world suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: PASS.

- [ ] **Step 4.4: Commit**

```bash
git add modules/world/handler_interface_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-46 T4 — IdkType validation coverage (disabled, wrong-type, OOB)

Happy-path male+female, disabled-idk, wrong-type, and out-of-range idkit
rejection tests for handleIdkSaveDesign.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — `tryInteract` dead-API polish (NAI-44-D-CONTINUEWALK-UNUSED)

**Files:**
- Modify: `modules/world/interaction.go`

The `continueWalk bool` parameter of `tryInteract` has no reader (`_ = continueWalk` at line 268). Remove it and update the 2 call sites.

- [ ] **Step 5.1: Remove the `continueWalk bool` parameter from `tryInteract`**

In `modules/world/interaction.go`, replace the function signature and doc-comment block at lines 241-252. The full block to replace:

```go
// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined.
// Returns true when an OP or AP trigger fired this tick.
//
// continueWalk is reserved for TS Player.ts:1245's stepsTaken-aware
// retry timing. Goscape's per-tick movement order makes it currently a
// no-op (the post-step arm only runs once anyway).
//
// DEVIATION NAI-44-D-CONTINUEWALK-UNUSED: parameter kept for symmetry
// with TS shape; closure is dead-API-polish at next sub-spec close per
// dead_api_polish.md if no consumer materializes.
func (p *Player) tryInteract(continueWalk bool) bool {
```

Replace with:

```go
// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined.
// Returns true when an OP or AP trigger fired this tick.
func (p *Player) tryInteract() bool {
```

- [ ] **Step 5.2: Remove the `_ = continueWalk` dead-API line**

Inside `tryInteract`, remove the line `_ = continueWalk` (currently at line 268).

- [ ] **Step 5.3: Update call site 1 — `interaction.go:169`**

Replace:
```go
interacted = p.tryInteract(false)
```
with:
```go
interacted = p.tryInteract()
```

- [ ] **Step 5.4: Update call site 2 — `interaction.go:192`**

Replace:
```go
interacted = p.tryInteract(p.stepsTaken == 0)
```
with:
```go
interacted = p.tryInteract()
```

- [ ] **Step 5.5: Compile check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/... 2>&1
```

Expected: no output.

- [ ] **Step 5.6: Run full suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
```

Expected: all PASS, no FAIL.

- [ ] **Step 5.7: Commit**

```bash
git add modules/world/interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
polish(world): NAI-46 T5 — remove continueWalk dead API from tryInteract

Parameter had no reader (_ = continueWalk). Updated 2 call sites.
Retires deviation NAI-44-D-CONTINUEWALK-UNUSED.

Closes memory: NAI-45-D3, NAI-44-D-CONTINUEWALK-UNUSED

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Post-task verification

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | grep -E 'ok|FAIL'
```

Expected: all packages `ok`, no `FAIL`.
