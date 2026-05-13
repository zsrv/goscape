# NAI-192 — varn + vars packers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the first per-config packer slice on the NAI-191 foundation — parse and pack `.varn` / `.vars` source configs into byte-identical `<outDir>/server/{varn,vars}.{dat,idx}` outputs, with the once-per-arc infrastructure (PackedData, typed reader, constants, ScriptVarTypeFromName) that subsequent NAI-193+ packers reuse.

**Architecture:** New code lives entirely in `pkg/pack/` plus one new file in `pkg/objtype/`. No production callsite this slice — wired only from test code. `PackConfigs(srcDir, outDir)` is the public entry point; per-config packers (`packVarnConfigs`, `packVarsConfigs`) take an explicit `*PackFile` parameter (module-level singletons deferred).

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` + NAI-191 `pkg/pack` foundation.

**Spec:** `docs/superpowers/specs/2026-05-13-nai-192-varn-vars-packers-design.md` (commit `957b58f`).
**HEAD at plan-write:** `957b58f`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** mirrors `pkg/pack/parse_test.go`: bare `if err != nil { t.Fatal(err) }`, `slices.Equal` / `bytes.Equal` for comparison, `t.Fatalf("got %q, want %q", got, want)` envelope, `t.TempDir()` for fixture roots, `ClearFsCache()` before any test that mutates the filesystem.
- **Error envelope** matches existing goscape `pkg/pack/parse.go` style: `fmt.Errorf("<kind> in %s: %s", file, detail)`. NOT the TS `Error during parsing - see ${file}:${n+1}` format — see `NAI-192-D-PARSE-ERROR-ENVELOPE` deviation, Task 5.
- **Modern Go**: use range over integer (`for id := range pf.Max`) per `use-modern-go` skill conventions in the project.

---

## Pre-flight verification (controller, before dispatching tasks)

Already verified at plan-write against HEAD `957b58f`:

| Premise | Verification |
|---|---|
| `pkg/objtype/paramtype.go:27` declares `type ScriptVarType int` + 25 constants | ✅ Read |
| No `ScriptVarTypeFromName` exists anywhere in tree | ✅ Grep |
| No `pkg/pack/{packed_data,config_value,constants,read_typed,varn,vars,pack_configs}.go` | ✅ `ls pkg/pack/` |
| Existing flat `ReadConfigs(srcDir, ext) (map[string][]string, error)` lives in `pkg/pack/parse.go:177` | ✅ Read |
| `packet.Alloc(size int) *Packet` at `pkg/io/packet/packet.go:73` | ✅ Read |
| `(*packet.Packet).Save(filePath string, length int, start int) error` at `pkg/io/packet/packet.go:108` | ✅ Read |
| `(*packet.Packet).Length() int` returns `len(p.Data)` at `pkg/io/packet/packet.go:96` | ✅ Read |
| `(*packet.Packet).PJStrLF(str string)` writes `PJStr(str, 10)` at `pkg/io/packet/packet.go:395` | ✅ Read |
| `pkg/objtype/varntype.go:21` decodes `case 250: v.DebugName = dat.GJStrLF()` | ✅ Read |
| `pkg/objtype/varstype.go` mirrors varntype | ✅ Grep for `LoadVarsTypes` |
| TS `Parse.ts:readConfigs` vs `PackShared.ts:readConfigs` — two different functions; PackShared's typed reader uses raw readline (no comment strip beyond `//` line-prefix), `Parse.ts:readConfigs` uses `loadDirExtFull`. Goscape uses `LoadDirExtFull` for both — `NAI-192-D-COMMENT-STRIP-EAGER` documents this | ✅ Read both |
| `pkg/objtype` does NOT import `pkg/pack` — no circular-import risk when `pkg/pack` imports `pkg/objtype` | ✅ Grep |
| Existing test style uses `ClearFsCache()` before any FS-touching test | ✅ Read `pkg/pack/packfile_test.go` + `parse_test.go` |

---

## Task 1: `ScriptVarTypeFromName` in `pkg/objtype/scriptvartype.go`

**Why first:** Every downstream parse callback needs this; isolating the move-of-existing-decl into its own task de-risks the subsequent files.

**Files:**
- Create: `pkg/objtype/scriptvartype.go`
- Create: `pkg/objtype/scriptvartype_test.go`
- Modify: `pkg/objtype/paramtype.go:27-55` (remove `ScriptVarType int` declaration and 25 const block; they move into the new file)

### Steps

- [ ] **Step 1.1: Write the failing test**

Create `pkg/objtype/scriptvartype_test.go`:

```go
package objtype

import "testing"

func TestScriptVarTypeFromName_KnownNames(t *testing.T) {
	cases := []struct {
		name string
		want ScriptVarType
	}{
		{"int", 105},
		{"autoint", 255},
		{"string", 115},
		{"enum", 103},
		{"obj", 111},
		{"loc", 108},
		{"component", 73},
		{"namedobj", 79},
		{"struct", 74},
		{"boolean", 49},
		{"coord", 99},
		{"category", 121},
		{"spotanim", 116},
		{"npc", 110},
		{"inv", 118},
		{"synth", 80},
		{"seq", 65},
		{"stat", 83},
		{"varp", 86},
		{"player_uid", 112},
		{"npc_uid", 78},
		{"interface", 97},
		{"npc_stat", 254},
		{"idkit", 75},
		{"dbrow", 208},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ScriptVarTypeFromName(tc.name)
			if !ok {
				t.Fatalf("ok=false for known name %q", tc.name)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestScriptVarTypeFromName_Unknown(t *testing.T) {
	got, ok := ScriptVarTypeFromName("not_a_real_type")
	if ok {
		t.Fatalf("ok=true for unknown name, got %d", got)
	}
	if got != 0 {
		t.Fatalf("got %d, want 0 (zero value) for unknown name", got)
	}
}

func TestScriptVarTypeFromName_EmptyString(t *testing.T) {
	got, ok := ScriptVarTypeFromName("")
	if ok {
		t.Fatalf("ok=true for empty name, got %d", got)
	}
	if got != 0 {
		t.Fatalf("got %d, want 0 for empty name", got)
	}
}
```

- [ ] **Step 1.2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestScriptVarTypeFromName ./pkg/objtype/...
```

Expected: FAIL with `undefined: ScriptVarTypeFromName`.

- [ ] **Step 1.3: Create `pkg/objtype/scriptvartype.go`**

```go
package objtype

// ScriptVarType is the single-byte type code stored in cache .dat files.
// The numeric value equals the ASCII codepoint of the legacy type
// letter (e.g. 'i' = 105 for int).
//
// TS source: src/cache/config/ScriptVarType.ts.
type ScriptVarType int

const (
	ScriptVarTypeInt       ScriptVarType = 105 // i
	ScriptVarTypeAutoInt   ScriptVarType = 255 // ÿ
	ScriptVarTypeString    ScriptVarType = 115 // s
	ScriptVarTypeEnum      ScriptVarType = 103 // g
	ScriptVarTypeObj       ScriptVarType = 111 // o
	ScriptVarTypeLoc       ScriptVarType = 108 // l
	ScriptVarTypeComponent ScriptVarType = 73  // I
	ScriptVarTypeNamedObj  ScriptVarType = 79  // O
	ScriptVarTypeStruct    ScriptVarType = 74  // J
	ScriptVarTypeBoolean   ScriptVarType = 49  // 1
	ScriptVarTypeCoord     ScriptVarType = 99  // c
	ScriptVarTypeCategory  ScriptVarType = 121 // y
	ScriptVarTypeSpotanim  ScriptVarType = 116 // t
	ScriptVarTypeNPC       ScriptVarType = 110 // n
	ScriptVarTypeInv       ScriptVarType = 118 // v
	ScriptVarTypeSynth     ScriptVarType = 80  // P
	ScriptVarTypeSeq       ScriptVarType = 65  // A
	ScriptVarTypeStat      ScriptVarType = 83  // S
	ScriptVarTypeInterface ScriptVarType = 97  // a
	ScriptVarTypeVarp      ScriptVarType = 86  // V
	ScriptVarTypePlayerUid ScriptVarType = 112 // p
	ScriptVarTypeNpcUid    ScriptVarType = 78  // N
	ScriptVarTypeNpcStat   ScriptVarType = 254 // þ
	ScriptVarTypeIdkit     ScriptVarType = 75  // K
	ScriptVarTypeDbrow     ScriptVarType = 208 // Ð
)

// ScriptVarTypeFromName returns the ScriptVarType code for a type
// name, or (0, false) for unknown names. Matches TS
// ScriptVarType.getTypeChar.
//
// TS source: src/cache/config/ScriptVarType.ts:85-170.
func ScriptVarTypeFromName(name string) (ScriptVarType, bool) {
	switch name {
	case "int":
		return ScriptVarTypeInt, true
	case "autoint":
		return ScriptVarTypeAutoInt, true
	case "string":
		return ScriptVarTypeString, true
	case "enum":
		return ScriptVarTypeEnum, true
	case "obj":
		return ScriptVarTypeObj, true
	case "loc":
		return ScriptVarTypeLoc, true
	case "component":
		return ScriptVarTypeComponent, true
	case "namedobj":
		return ScriptVarTypeNamedObj, true
	case "struct":
		return ScriptVarTypeStruct, true
	case "boolean":
		return ScriptVarTypeBoolean, true
	case "coord":
		return ScriptVarTypeCoord, true
	case "category":
		return ScriptVarTypeCategory, true
	case "spotanim":
		return ScriptVarTypeSpotanim, true
	case "npc":
		return ScriptVarTypeNPC, true
	case "inv":
		return ScriptVarTypeInv, true
	case "synth":
		return ScriptVarTypeSynth, true
	case "seq":
		return ScriptVarTypeSeq, true
	case "stat":
		return ScriptVarTypeStat, true
	case "varp":
		return ScriptVarTypeVarp, true
	case "player_uid":
		return ScriptVarTypePlayerUid, true
	case "npc_uid":
		return ScriptVarTypeNpcUid, true
	case "interface":
		return ScriptVarTypeInterface, true
	case "npc_stat":
		return ScriptVarTypeNpcStat, true
	case "idkit":
		return ScriptVarTypeIdkit, true
	case "dbrow":
		return ScriptVarTypeDbrow, true
	}
	return 0, false
}
```

- [ ] **Step 1.4: Remove the moved declarations from `paramtype.go`**

Delete lines 27-55 of `pkg/objtype/paramtype.go` — the `type ScriptVarType int` line plus the entire 25-constant `const ( ... )` block. Keep everything else (imports, `ParamMap`, `DecodeParams`, `ParamTypeConfigs`, `LoadParams`, etc.).

Edit (exact `old_string` / `new_string`):

`old_string`:
```
type ScriptVarType int

const (
	ScriptVarTypeInt       ScriptVarType = 105 // i
	ScriptVarTypeAutoInt   ScriptVarType = 255 // ÿ
	ScriptVarTypeString    ScriptVarType = 115 // s
	ScriptVarTypeEnum      ScriptVarType = 103 // g
	ScriptVarTypeObj       ScriptVarType = 111 // o
	ScriptVarTypeLoc       ScriptVarType = 108 // l
	ScriptVarTypeComponent ScriptVarType = 73  // I
	ScriptVarTypeNamedObj  ScriptVarType = 79  // O
	ScriptVarTypeStruct    ScriptVarType = 74  // J
	ScriptVarTypeBoolean   ScriptVarType = 49  // 1
	ScriptVarTypeCoord     ScriptVarType = 99  // c
	ScriptVarTypeCategory  ScriptVarType = 121 // y
	ScriptVarTypeSpotanim  ScriptVarType = 116 // t
	ScriptVarTypeNPC       ScriptVarType = 110 // n
	ScriptVarTypeInv       ScriptVarType = 118 // v
	ScriptVarTypeSynth     ScriptVarType = 80  // P
	ScriptVarTypeSeq       ScriptVarType = 65  // A
	ScriptVarTypeStat      ScriptVarType = 83  // S
	ScriptVarTypeInterface ScriptVarType = 97  // a
	ScriptVarTypeVarp      ScriptVarType = 86  // V
	ScriptVarTypePlayerUid ScriptVarType = 112 // p
	ScriptVarTypeNpcUid    ScriptVarType = 78  // N
	ScriptVarTypeNpcStat   ScriptVarType = 254 // þ
	ScriptVarTypeIdkit     ScriptVarType = 75  // K
	ScriptVarTypeDbrow     ScriptVarType = 208 // Ð
)
```

`new_string`: empty (or a single blank line — verify resulting file has no dangling blank-block).

- [ ] **Step 1.5: Run all objtype tests + a build sanity check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```

Expected: PASS. Build must succeed package-wide because `ScriptVarType*` is referenced throughout `pkg/objtype/{enumtype,varntype,varstype,dbrowtype,varptype,dbtabletype,dbtableindex}.go` and `pkg/script/{handlers_db,handlers_vars,handlers_config,configs}.go` — these all stay valid since the moved declarations remain in package `objtype` (just a different file).

- [ ] **Step 1.6: Commit**

```bash
git add pkg/objtype/scriptvartype.go pkg/objtype/scriptvartype_test.go pkg/objtype/paramtype.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-192 T1 — extract ScriptVarType + add ScriptVarTypeFromName

Move ScriptVarType + 25 constants from paramtype.go into a dedicated
scriptvartype.go file. Add ScriptVarTypeFromName(name) (ScriptVarType,
bool) — TS getTypeChar port.

Required by NAI-192 varn/vars parse callbacks; subsequent packer slices
reuse it.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `PackedData` in `pkg/pack/packed_data.go`

**Files:**
- Create: `pkg/pack/packed_data.go`
- Create: `pkg/pack/packed_data_test.go`

### Steps

- [ ] **Step 2.1: Write the failing tests**

Create `pkg/pack/packed_data_test.go`:

```go
package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPackedData_WritesSizeHeader(t *testing.T) {
	pd := NewPackedData(7)

	// p2(size=7) in BE: 00 07
	if !bytes.Equal(pd.Dat.Data, []byte{0x00, 0x07}) {
		t.Fatalf("dat=% x, want 00 07", pd.Dat.Data)
	}
	if !bytes.Equal(pd.Idx.Data, []byte{0x00, 0x07}) {
		t.Fatalf("idx=% x, want 00 07", pd.Idx.Data)
	}
	if pd.Size != 7 {
		t.Fatalf("Size=%d, want 7", pd.Size)
	}
}

func TestPackedData_NextWritesTerminatorAndIdxOffset(t *testing.T) {
	pd := NewPackedData(2)
	pd.P1(1)
	pd.P1(105)
	// dat so far: 00 02 01 69  (4 bytes)
	pd.Next()
	// next() appends 0x00 to dat (5 bytes total) and writes p2(3) to idx
	// (3 = 5 - marker=2). Marker advances to 5.
	wantDat := []byte{0x00, 0x02, 0x01, 0x69, 0x00}
	wantIdx := []byte{0x00, 0x02, 0x00, 0x03}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x, want % x", pd.Dat.Data, wantDat)
	}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x, want % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackedData_NextTwiceTracksMarker(t *testing.T) {
	pd := NewPackedData(2)
	pd.P1(0xAA)
	pd.Next() // entry 0: 1-byte body + terminator = 2 bytes since marker
	pd.P1(0xBB)
	pd.P1(0xCC)
	pd.Next() // entry 1: 2-byte body + terminator = 3 bytes since marker

	wantDat := []byte{0x00, 0x02, 0xAA, 0x00, 0xBB, 0xCC, 0x00}
	wantIdx := []byte{0x00, 0x02, 0x00, 0x02, 0x00, 0x03}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x, want % x", pd.Dat.Data, wantDat)
	}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x, want % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackedData_PJStrUsesLFTerminator(t *testing.T) {
	// NAI-192 R2 pin: TS pjstr writes LF (0x0a), not NUL.
	pd := NewPackedData(1)
	pd.PJStr("hi")
	// dat: 00 01 'h' 'i' 0a
	want := []byte{0x00, 0x01, 0x68, 0x69, 0x0a}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("dat=% x, want % x", pd.Dat.Data, want)
	}
}

func TestPackedData_SaveWritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	pd := NewPackedData(1)
	pd.P1(1)
	pd.P1(105)
	pd.Next()
	datPath := filepath.Join(dir, "out.dat")
	idxPath := filepath.Join(dir, "out.idx")
	if err := pd.Save(datPath, idxPath); err != nil {
		t.Fatal(err)
	}
	gotDat, err := os.ReadFile(datPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotDat, []byte{0x00, 0x01, 0x01, 0x69, 0x00}) {
		t.Fatalf("dat file=% x", gotDat)
	}
	gotIdx, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotIdx, []byte{0x00, 0x01, 0x00, 0x03}) {
		t.Fatalf("idx file=% x", gotIdx)
	}
}

func TestPackedData_SaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	pd := NewPackedData(0)
	// Empty: just the 2-byte header in both buffers.
	deepDat := filepath.Join(dir, "a", "b", "c.dat")
	deepIdx := filepath.Join(dir, "a", "b", "c.idx")
	if err := pd.Save(deepDat, deepIdx); err != nil {
		t.Fatalf("Save with missing parent dir: %v", err)
	}
	if _, err := os.Stat(deepDat); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(deepIdx); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2.2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPackedData ./pkg/pack/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNewPackedData ./pkg/pack/...
```

Expected: FAIL with `undefined: NewPackedData` / `undefined: PackedData`.

- [ ] **Step 2.3: Create `pkg/pack/packed_data.go`**

```go
package pack

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// PackedData is a paired dat+idx Packet buffer used by per-config
// packers. The dat buffer holds per-entry bodies separated by a 0x00
// terminator; the idx buffer holds the per-entry byte length as a
// big-endian uint16.
//
// Not safe for concurrent use.
//
// TS source: tools/pack/config/PackShared.ts:39-84.
type PackedData struct {
	Dat    *packet.Packet
	Idx    *packet.Packet
	Size   int
	marker int
}

// NewPackedData allocates a fresh dat+idx pair, writes p2(size) into
// both as the count header, and sets marker=2 (past the header).
func NewPackedData(size int) *PackedData {
	pd := &PackedData{
		Dat:  packet.Alloc(5),
		Idx:  packet.Alloc(3),
		Size: size,
	}
	pd.Dat.P2(uint16(size))
	pd.Idx.P2(uint16(size))
	pd.marker = 2
	return pd
}

// Next writes one terminator (0x00) to dat, records the bytes-since-marker
// to idx as a p2, and advances marker to the new dat write cursor.
//
// NAI-192-D-PACKET-WRITE-CURSOR: TS uses dat.pos; goscape's Packet.Pos
// is the read pointer (memory packet_rw_pointer_gotcha). Use
// Dat.Length() — i.e. len(Dat.Data) — for the write cursor.
func (pd *PackedData) Next() {
	pd.Dat.P1(0)
	pd.Idx.P2(uint16(pd.Dat.Length() - pd.marker))
	pd.marker = pd.Dat.Length()
}

func (pd *PackedData) P1(v uint8)     { pd.Dat.P1(v) }
func (pd *PackedData) P2(v uint16)    { pd.Dat.P2(v) }
func (pd *PackedData) P3(v uint32)    { pd.Dat.P3(v) }
func (pd *PackedData) P4(v uint32)    { pd.Dat.P4(v) }
func (pd *PackedData) PBool(v bool)   { pd.Dat.PBool(v) }

// PJStr writes a JagString with an LF (0x0a) terminator, matching
// TS Packet.pjstr at io/Packet.ts:336.
func (pd *PackedData) PJStr(s string) { pd.Dat.PJStrLF(s) }

// Save writes the full dat and idx buffers to disk. Parent directories
// are created via packet.Packet.Save's os.MkdirAll.
func (pd *PackedData) Save(dataPath, idxPath string) error {
	if err := pd.Dat.Save(dataPath, pd.Dat.Length(), 0); err != nil {
		return err
	}
	return pd.Idx.Save(idxPath, pd.Idx.Length(), 0)
}
```

- [ ] **Step 2.4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestPackedData|TestNewPackedData' ./pkg/pack/...
```

Expected: PASS for all 6 tests.

- [ ] **Step 2.5: Commit**

```bash
git add pkg/pack/packed_data.go pkg/pack/packed_data_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-192 T2 — PackedData (dat+idx Packet pair)

PackedData holds a dat+idx Packet pair with per-entry marker
bookkeeping. Used by per-config packers to emit cache .dat/.idx
output. PJStr uses LF terminator matching TS Packet.pjstr.

NAI-192-D-PACKET-WRITE-CURSOR: TS uses dat.pos for marker arithmetic;
goscape's Packet.Pos is the read pointer (memory
packet_rw_pointer_gotcha), so Next() uses Dat.Length() instead.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `ConfigValue` / `ConfigLine` / boolean helpers in `pkg/pack/config_value.go`

**Files:**
- Create: `pkg/pack/config_value.go`
- Create: `pkg/pack/config_value_test.go`

### Steps

- [ ] **Step 3.1: Write the failing tests**

Create `pkg/pack/config_value_test.go`:

```go
package pack

import "testing"

func TestIsConfigBoolean(t *testing.T) {
	yes := []string{"yes", "no", "true", "false", "1", "0"}
	for _, v := range yes {
		if !IsConfigBoolean(v) {
			t.Errorf("IsConfigBoolean(%q)=false, want true", v)
		}
	}
	no := []string{"", "Yes", "TRUE", "2", "maybe", "y"}
	for _, v := range no {
		if IsConfigBoolean(v) {
			t.Errorf("IsConfigBoolean(%q)=true, want false", v)
		}
	}
}

func TestGetConfigBoolean(t *testing.T) {
	trueCases := []string{"yes", "true", "1"}
	for _, v := range trueCases {
		if !GetConfigBoolean(v) {
			t.Errorf("GetConfigBoolean(%q)=false, want true", v)
		}
	}
	falseCases := []string{"no", "false", "0", "Yes", "TRUE"}
	for _, v := range falseCases {
		if GetConfigBoolean(v) {
			t.Errorf("GetConfigBoolean(%q)=true, want false", v)
		}
	}
}

func TestConfigLine_StructShape(t *testing.T) {
	// Sanity: a ConfigLine can hold any ConfigValue.
	line := ConfigLine{Key: "type", Value: 105}
	if line.Key != "type" {
		t.Fatalf("Key=%q", line.Key)
	}
	if v, ok := line.Value.(int); !ok || v != 105 {
		t.Fatalf("Value=%v", line.Value)
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestIsConfigBoolean|TestGetConfigBoolean|TestConfigLine' ./pkg/pack/...
```

Expected: FAIL with `undefined: IsConfigBoolean` / `undefined: ConfigLine`.

- [ ] **Step 3.3: Create `pkg/pack/config_value.go`**

```go
package pack

// ConfigValue is the typed value of a parsed key=value line.
// TS uses a discriminated union (string | number | boolean | ...);
// Go uses `any` plus per-packer type assertions. The set of permitted
// runtime types grows as more per-config packers land — NAI-193+ will
// add LocModelShape, ParamValue, HuntCheckVar, etc.
//
// TS source: tools/pack/config/PackShared.ts:131 (ConfigValue union).
type ConfigValue = any

// ConfigLine is one key=value pair parsed from a [name]-headed config
// block.
//
// TS source: tools/pack/config/PackShared.ts:132.
type ConfigLine struct {
	Key   string
	Value ConfigValue
}

// IsConfigBoolean reports whether v is one of the six accepted boolean
// literals (yes/no/true/false/1/0). Case-sensitive.
//
// TS source: tools/pack/config/PackShared.ts:31-33.
func IsConfigBoolean(v string) bool {
	return v == "yes" || v == "no" || v == "true" || v == "false" || v == "1" || v == "0"
}

// GetConfigBoolean returns true for "yes"/"true"/"1", false otherwise.
// Case-sensitive. Caller is expected to gate on IsConfigBoolean first.
//
// TS source: tools/pack/config/PackShared.ts:35-37.
func GetConfigBoolean(v string) bool {
	return v == "yes" || v == "true" || v == "1"
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestIsConfigBoolean|TestGetConfigBoolean|TestConfigLine' ./pkg/pack/...
```

Expected: PASS.

- [ ] **Step 3.5: Commit**

```bash
git add pkg/pack/config_value.go pkg/pack/config_value_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-192 T3 — ConfigValue / ConfigLine + boolean helpers

ConfigValue (any-typed) + ConfigLine{Key,Value} match the shape that
per-config parse callbacks emit. IsConfigBoolean / GetConfigBoolean
port TS PackShared.ts:31-37 — case-sensitive yes/no/true/false/1/0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `Constants` + `LoadConstants` + `substituteConstants` in `pkg/pack/constants.go`

**Files:**
- Create: `pkg/pack/constants.go`
- Create: `pkg/pack/constants_test.go`

### Steps

- [ ] **Step 4.1: Write the failing tests**

Create `pkg/pack/constants_test.go`:

```go
package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConstants_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "a.constant"),
		"^FOO=100\nBAR=hello\n")
	writeFile(t, filepath.Join(dir, "scripts", "sub", "b.constant"),
		"BAZ=world\n")
	ClearFsCache()
	c, err := LoadConstants(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c["FOO"] != "100" {
		t.Errorf("FOO=%q, want 100", c["FOO"])
	}
	if c["BAR"] != "hello" {
		t.Errorf("BAR=%q, want hello", c["BAR"])
	}
	if c["BAZ"] != "world" {
		t.Errorf("BAZ=%q, want world", c["BAZ"])
	}
}

func TestLoadConstants_SkipsBlankAndLineComments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "a.constant"),
		"\n// a comment\nFOO=1\n   \nBAR=2\n")
	ClearFsCache()
	c, err := LoadConstants(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c["FOO"] != "1" || c["BAR"] != "2" {
		t.Fatalf("c=%v", c)
	}
}

func TestLoadConstants_DuplicateNameErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "a.constant"),
		"FOO=1\nFOO=2\n")
	ClearFsCache()
	_, err := LoadConstants(dir)
	if err == nil {
		t.Fatal("want duplicate-constant error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "FOO") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadConstants_MissingScriptsDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	c, err := LoadConstants(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 0 {
		t.Fatalf("want empty, got %v", c)
	}
}

func TestSubstituteConstants_Table(t *testing.T) {
	c := Constants{"FOO": "100", "BAR": "hello"}
	cases := []struct {
		in   string
		want string
	}{
		{"^FOO", "100"},
		{"^FOO\n", "100\n"},
		{"^FOO\r", "100\r"},
		{"^FOO,extra", "100,extra"},
		{"^FOO extra", "100 extra"},
		{"prefix ^FOO", "prefix 100"},
		{"^FOO,^BAR", "100,hello"},
		{"^MISSING", "^MISSING"},          // absent — literal preserved
		{"no_sub_here", "no_sub_here"},    // no caret at all
		{"^FOOBAR_no_match", "^FOOBAR_no_match"}, // FOOBAR not in map; consumed terminator absent
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := substituteConstants(tc.in, c)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestLoadConstants|TestSubstituteConstants' ./pkg/pack/...
```

Expected: FAIL with `undefined: LoadConstants` / `undefined: substituteConstants`.

- [ ] **Step 4.3: Create `pkg/pack/constants.go`**

```go
package pack

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Constants maps a constant name (without leading "^") to its raw
// textual value. Substitution into config values is done via
// substituteConstants.
//
// TS source: tools/pack/config/PackShared.ts:86 (CONSTANTS module-level
// map). NAI-192 keeps the map non-global — caller threads it through
// LoadConstants → ReadTypedConfigs.
type Constants map[string]string

// LoadConstants walks <srcDir>/scripts recursively for *.constant
// files, parses `name=value` lines, and returns the aggregated map.
// Blank lines and lines starting with `//` are skipped. A leading `^`
// on a name is stripped. Duplicate names across all files error.
//
// Missing <srcDir>/scripts directory returns an empty map without
// error.
//
// TS source: tools/pack/config/PackShared.ts:262-289.
func LoadConstants(srcDir string) (Constants, error) {
	c := Constants{}
	scriptsDir := filepath.Join(srcDir, "scripts")
	if !FileExists(scriptsDir) {
		return c, nil
	}
	var outerErr error
	LoadDirExt(scriptsDir, ".constant", func(lines []string, file string) {
		if outerErr != nil {
			return
		}
		for _, line := range lines {
			if len(line) == 0 || strings.HasPrefix(line, "//") {
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				outerErr = fmt.Errorf("bad constant declaration in %s: %s", file, line)
				return
			}
			name := strings.TrimSpace(line[:eq])
			value := strings.TrimSpace(line[eq+1:])
			if strings.HasPrefix(name, "^") {
				name = name[1:]
			}
			if name == "" {
				outerErr = fmt.Errorf("empty constant name in %s: %s", file, line)
				return
			}
			if _, dup := c[name]; dup {
				outerErr = fmt.Errorf("duplicate constant in %s: %s", file, name)
				return
			}
			c[name] = value
		}
	})
	if outerErr != nil {
		return nil, outerErr
	}
	return c, nil
}

// substituteConstants scans value for `^NAME` runs (terminators: '\r'
// '\n' ',' ' ' end-of-string) and replaces with c[NAME] when present.
// Absent names leave the literal `^NAME` in place — TS parity, no
// error.
//
// TS source: tools/pack/config/PackShared.ts:200-223.
func substituteConstants(value string, c Constants) string {
	if !strings.ContainsRune(value, '^') {
		return value
	}
	var b strings.Builder
	b.Grow(len(value))
	i := 0
	for i < len(value) {
		if value[i] != '^' {
			b.WriteByte(value[i])
			i++
			continue
		}
		// Scan to terminator.
		end := i + 1
		for end < len(value) {
			ch := value[end]
			if ch == '\r' || ch == '\n' || ch == ',' || ch == ' ' {
				break
			}
			end++
		}
		name := value[i+1 : end]
		if sub, ok := c[name]; ok {
			b.WriteString(sub)
		} else {
			b.WriteString(value[i:end])
		}
		i = end
	}
	return b.String()
}
```

- [ ] **Step 4.4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestLoadConstants|TestSubstituteConstants' ./pkg/pack/...
```

Expected: PASS for all 5 tests.

- [ ] **Step 4.5: Commit**

```bash
git add pkg/pack/constants.go pkg/pack/constants_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-192 T4 — Constants map + LoadConstants + ^foo substitution

LoadConstants walks <srcDir>/scripts/**/*.constant, parses name=value
lines (skips blank + // prefix, strips leading ^, errors on duplicate).
substituteConstants performs ^FOO → value substitution with TS-parity
terminators (\r \n , ' ' end-of-string); absent names left literal.

TS source: PackShared.ts:200-223, 262-289.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `ReadTypedConfigs` in `pkg/pack/read_typed.go`

**Why this shape:** Per-config packers (Tasks 6-7) need a `map[string][]ConfigLine` shape — the existing flat `ReadConfigs` returns `map[string][]string` and stays untouched. The two coexist.

**TS-divergence tags introduced this task:**
- `NAI-192-D-COMMENT-STRIP-EAGER` — goscape uses `LoadDirExtFull` (comment-stripped); TS PackShared uses raw readline (only `//` line-prefix skip). Harmless for varn/vars where values can't contain `/* */`. Foundation-wide convention.
- `NAI-192-D-PARSE-ERROR-ENVELOPE` — error messages use `"<kind> in <file>: <detail>"` not TS `"\nError during parsing - see <file>:<n+1>\n<msg>"`. Matches existing `pkg/pack/parse.go:191,203` style.

**Files:**
- Create: `pkg/pack/read_typed.go`
- Create: `pkg/pack/read_typed_test.go`

### Steps

- [ ] **Step 5.1: Write the failing tests**

Create `pkg/pack/read_typed_test.go`:

```go
package pack

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// trivialParse is a ParseFn used in read_typed tests: it accepts every
// key and returns the substituted value verbatim.
func trivialParse(key, value string) (ConfigValue, bool, error) {
	return value, true, nil
}

func TestReadTypedConfigs_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	ClearFsCache()
	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 2 {
		t.Fatalf("got %d configs, want 2", len(cfgs))
	}
	if got := cfgs["npctier"]; len(got) != 1 || got[0].Key != "type" || got[0].Value != "int" {
		t.Fatalf("npctier=%v", got)
	}
	if got := cfgs["npchealth"]; len(got) != 1 || got[0].Key != "type" || got[0].Value != "int" {
		t.Fatalf("npchealth=%v", got)
	}
}

func TestReadTypedConfigs_ConstantsSubstitution(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=^MYTYPE\n")
	ClearFsCache()
	c := Constants{"MYTYPE": "int"}
	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, c)
	if err != nil {
		t.Fatal(err)
	}
	if cfgs["npctier"][0].Value != "int" {
		t.Fatalf("substituted value=%v, want \"int\"", cfgs["npctier"][0].Value)
	}
}

func TestReadTypedConfigs_MissingSeparatorErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nno_equals_here\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "missing property separator") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_DuplicateNameErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npctier]\ntype=string\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_MissingClosingBracketErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier\ntype=int\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "missing closing bracket") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_EmptyNameErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[]\ntype=int\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "empty config name") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_ParseFnOkFalseInvalidKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nbad_key=anything\n")
	ClearFsCache()
	parseFn := func(key, value string) (ConfigValue, bool, error) {
		return nil, false, nil
	}
	_, err := ReadTypedConfigs(dir, ".varn", nil, parseFn, Constants{})
	if err == nil || !strings.Contains(err.Error(), "invalid property key") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_ParseFnErrorInvalidValue(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=bogus\n")
	ClearFsCache()
	parseFn := func(key, value string) (ConfigValue, bool, error) {
		return nil, true, errors.New("rejected")
	}
	_, err := ReadTypedConfigs(dir, ".varn", nil, parseFn, Constants{})
	if err == nil || !strings.Contains(err.Error(), "invalid property value") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_RequiredPropertyMissingErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nother=ignored\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", []string{"type"}, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "missing required property") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_RequiredPropertyPresentAtFileEnd(t *testing.T) {
	// Required-property check must run at file-end, not just on the next
	// [header] line.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nother=ignored\n") // file ends mid-config
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", []string{"type"}, trivialParse, Constants{})
	if err == nil {
		t.Fatal("want missing-required-property error at file end")
	}
}

func TestReadTypedConfigs_MissingScriptsDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 0 {
		t.Fatalf("want empty, got %v", cfgs)
	}
}
```

- [ ] **Step 5.2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestReadTypedConfigs ./pkg/pack/...
```

Expected: FAIL with `undefined: ReadTypedConfigs`.

- [ ] **Step 5.3: Create `pkg/pack/read_typed.go`**

```go
package pack

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ParseFn is the per-key=value callback used by ReadTypedConfigs.
//
// Return convention:
//   - ok=true, err=nil  → accepted; value goes into the ConfigLine
//   - ok=false, err=nil → invalid key  (TS parity for `undefined` →
//                         "Invalid property key")
//   - err != nil        → invalid value (TS parity for `null`      →
//                         "Invalid property value")
//
// TS source: tools/pack/config/PackShared.ts:135 (ConfigParseCallback).
type ParseFn func(key, value string) (ConfigValue, bool, error)

// ReadTypedConfigs walks <srcDir>/scripts/*.<ext>, splits each file
// into [name]-delimited blocks, applies constants substitution to
// every value, calls parseFn per key=value line, and enforces
// required-properties at block close. Returns map[debugname][]ConfigLine.
//
// NAI-192-D-COMMENT-STRIP-EAGER: goscape uses LoadDirExtFull which
// strips // and /* */ comments. TS PackShared.readConfigs uses raw
// readline (only // line-prefix skip). Harmless for varn/vars whose
// values cannot contain comment markers.
//
// NAI-192-D-PARSE-ERROR-ENVELOPE: error messages use
// "<kind> in <file>: <detail>" rather than TS
// "\nError during parsing - see <file>:<n+1>\n<msg>". Matches existing
// pkg/pack/parse.go convention.
//
// TS source: tools/pack/config/PackShared.ts:141-247.
func ReadTypedConfigs(srcDir, ext string, required []string, parseFn ParseFn, c Constants) (map[string][]ConfigLine, error) {
	configs := map[string][]ConfigLine{}
	scriptsDir := filepath.Join(srcDir, "scripts")
	if !FileExists(scriptsDir) {
		return configs, nil
	}
	var outerErr error
	err := LoadDirExtFull(scriptsDir, ext, func(lines []string, file string) {
		if outerErr != nil {
			return
		}
		current := ""
		var block []ConfigLine

		flush := func() bool {
			if current == "" {
				return true
			}
			if _, dup := configs[current]; dup {
				outerErr = fmt.Errorf("duplicate config in %s: %s", file, current)
				return false
			}
			for _, prop := range required {
				found := false
				for _, ln := range block {
					if ln.Key == prop {
						found = true
						break
					}
				}
				if !found {
					outerErr = fmt.Errorf("missing required property %q for %s in %s", prop, current, file)
					return false
				}
			}
			configs[current] = block
			return true
		}

		for _, line := range lines {
			if strings.HasPrefix(line, "[") {
				if !strings.HasSuffix(line, "]") {
					outerErr = fmt.Errorf("missing closing bracket in %s: %s", file, line)
					return
				}
				if !flush() {
					return
				}
				name := line[1 : len(line)-1]
				if name == "" {
					outerErr = fmt.Errorf("empty config name in %s", file)
					return
				}
				current = name
				block = nil
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				outerErr = fmt.Errorf("missing property separator in %s: %s", file, line)
				return
			}
			key := line[:eq]
			value := substituteConstants(line[eq+1:], c)

			parsed, ok, err := parseFn(key, value)
			if err != nil {
				outerErr = fmt.Errorf("invalid property value in %s: %s", file, line)
				return
			}
			if !ok {
				outerErr = fmt.Errorf("invalid property key in %s: %s", file, line)
				return
			}
			block = append(block, ConfigLine{Key: key, Value: parsed})
		}
		flush()
	})
	if err != nil {
		return nil, err
	}
	if outerErr != nil {
		return nil, outerErr
	}
	return configs, nil
}
```

- [ ] **Step 5.4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestReadTypedConfigs ./pkg/pack/...
```

Expected: PASS for all 11 tests.

- [ ] **Step 5.5: Commit**

```bash
git add pkg/pack/read_typed.go pkg/pack/read_typed_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-192 T5 — ReadTypedConfigs (typed key=value reader)

ReadTypedConfigs walks <srcDir>/scripts/*.<ext>, splits each file into
[name]-delimited blocks, applies constants substitution, calls a
ParseFn callback per key=value line, and enforces required-properties
at block close. Returns map[debugname][]ConfigLine.

Coexists with the existing flat ReadConfigs(map[string][]string) — TS
PackShared.readConfigs vs Parse.readConfigs are different functions
with different return shapes.

NAI-192-D-COMMENT-STRIP-EAGER: goscape uses LoadDirExtFull (strips //
and /* */); TS PackShared uses raw readline (only // line-prefix).
NAI-192-D-PARSE-ERROR-ENVELOPE: error messages use "<kind> in <file>:
<detail>" not TS "Error during parsing - see <file>:<n+1>".

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `parseVarnConfig` + `packVarnConfigs` in `pkg/pack/varn.go`

**Files:**
- Create: `pkg/pack/varn.go`
- Create: `pkg/pack/varn_test.go`

### Steps

- [ ] **Step 6.1: Write the failing tests**

Create `pkg/pack/varn_test.go`:

```go
package pack

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseVarnConfig_TypeAccepted(t *testing.T) {
	v, ok, err := parseVarnConfig("type", "int")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false for known type")
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("v=%v, want ScriptVarTypeInt", v)
	}
}

func TestParseVarnConfig_UnknownTypeIsInvalidValue(t *testing.T) {
	_, ok, err := parseVarnConfig("type", "bogus")
	if err == nil {
		t.Fatal("want err for unknown type")
	}
	// ok=true with err!=nil is the contract for invalid value.
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil (invalid value)")
	}
}

func TestParseVarnConfig_UnknownKeyReturnsOkFalse(t *testing.T) {
	v, ok, err := parseVarnConfig("not_a_key", "whatever")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok=true for unknown key; want false")
	}
	if v != nil {
		t.Fatalf("v=%v, want nil", v)
	}
}

func TestPackVarnConfigs_BytePin(t *testing.T) {
	dir := t.TempDir()
	// scripts/test.varn declaring two configs:
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	// pack/varn.pack declaring the id mapping:
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n1=npchealth\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, parseVarnConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarnConfigs(cfgs, pf)

	// Expected dat:
	//   p2(size=2)             — 00 02
	//   id=0 (npctier) body:
	//     p1(1), p1(105)       — 01 69
	//     p1(250), pjstrlf("npctier")  — fa 6e 70 63 74 69 65 72 0a
	//   next() terminator:     — 00
	//   id=1 (npchealth) body:
	//     p1(1), p1(105)       — 01 69
	//     p1(250), pjstrlf("npchealth")— fa 6e 70 63 68 65 61 6c 74 68 0a
	//   next() terminator:     — 00
	wantDat := []byte{
		0x00, 0x02,
		0x01, 0x69,
		0xfa, 0x6e, 0x70, 0x63, 0x74, 0x69, 0x65, 0x72, 0x0a,
		0x00,
		0x01, 0x69,
		0xfa, 0x6e, 0x70, 0x63, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x\nwant % x", pd.Dat.Data, wantDat)
	}

	// Expected idx:
	//   p2(size=2)             — 00 02
	//   id=0 entry length 12   — 00 0c   (2 type-bytes + 1 name-opcode + 8 name+LF + 1 terminator)
	//   id=1 entry length 14   — 00 0e   (2 + 1 + 10 + 1)
	wantIdx := []byte{0x00, 0x02, 0x00, 0x0c, 0x00, 0x0e}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x\nwant % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackVarnConfigs_EmptySlotEmitsTerminatorOnly(t *testing.T) {
	// Slot id=1 has no [name] in pack — pf.GetByID(1)=="" — so it should
	// write only the next() terminator with no body.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, parseVarnConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarnConfigs(cfgs, pf)

	wantDat := []byte{
		0x00, 0x01,
		0x01, 0x69,
		0xfa, 0x6e, 0x70, 0x63, 0x74, 0x69, 0x65, 0x72, 0x0a,
		0x00,
	}
	wantIdx := []byte{0x00, 0x01, 0x00, 0x0c}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x\nwant % x", pd.Dat.Data, wantDat)
	}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x\nwant % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackVarnConfigs_RoundtripThroughObjtypeLoader(t *testing.T) {
	// Bind: pack output → write to disk → objtype.LoadVarnTypes parses
	// it correctly.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n1=npchealth\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, parseVarnConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarnConfigs(cfgs, pf)

	outDir := filepath.Join(dir, "out")
	serverDir := filepath.Join(outDir, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pd.Save(filepath.Join(serverDir, "varn.dat"), filepath.Join(serverDir, "varn.idx")); err != nil {
		t.Fatal(err)
	}

	vc, err := objtype.LoadVarnTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vc.Configs) != 2 {
		t.Fatalf("Configs len=%d, want 2", len(vc.Configs))
	}
	if vc.Configs[0].DebugName != "npctier" {
		t.Fatalf("Configs[0].DebugName=%q", vc.Configs[0].DebugName)
	}
	if vc.Configs[0].Type != objtype.ScriptVarTypeInt {
		t.Fatalf("Configs[0].Type=%d", vc.Configs[0].Type)
	}
	if vc.Configs[1].DebugName != "npchealth" {
		t.Fatalf("Configs[1].DebugName=%q", vc.Configs[1].DebugName)
	}
}

// Sanity: keep an explicit reference to the parseFn's invalid-value
// signature so the compiler catches contract regressions.
var _ = func() error {
	_, _, err := parseVarnConfig("type", "bogus")
	return err
}
var _ = errors.New
```

- [ ] **Step 6.2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestParseVarnConfig|TestPackVarnConfigs' ./pkg/pack/...
```

Expected: FAIL with `undefined: parseVarnConfig` / `undefined: packVarnConfigs`.

- [ ] **Step 6.3: Create `pkg/pack/varn.go`**

```go
package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseVarnConfig is the per-key=value parser for .varn config blocks.
// Only `type` is accepted; all other keys are reported as invalid via
// ok=false.
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseVarnConfig contains empty
// stringKeys/numberKeys/booleanKeys arrays — dead code preserved by
// the TS author. Goscape omits the empty branches; they revive when a
// future schema addition needs them.
//
// TS source: tools/pack/config/VarnConfig.ts:5-51.
func parseVarnConfig(key, value string) (ConfigValue, bool, error) {
	if key == "type" {
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	}
	return nil, false, nil
}

// packVarnConfigs walks every id in [0, pf.Max), pulls the debugname
// from the PackFile, emits the parsed config body (currently just the
// 1-byte `type` opcode), then writes the debugname trailer (opcode
// 250 + LF-terminated string) when the slot has a name. Each slot
// ends with PackedData.Next() — a single 0x00 terminator + idx offset.
//
// TS source: tools/pack/config/VarnConfig.ts:53-82.
func packVarnConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			for _, line := range cfg {
				if line.Key == "type" {
					pd.P1(1)
					pd.P1(uint8(line.Value.(objtype.ScriptVarType)))
				}
			}
		}
		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}
	return pd
}
```

- [ ] **Step 6.4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestParseVarnConfig|TestPackVarnConfigs' ./pkg/pack/...
```

Expected: PASS for all 6 tests, including the roundtrip-through-loader test.

- [ ] **Step 6.5: Commit**

```bash
git add pkg/pack/varn.go pkg/pack/varn_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-192 T6 — parseVarnConfig + packVarnConfigs

Per-key=value parser accepts only `type=<scriptvartype-name>`; all
other keys return ok=false (invalid key). Packer walks every id in
[0, pf.Max), emits the type opcode + 1-byte type code when present
and the debugname trailer (opcode 250 + LF-terminated string), then
calls PackedData.Next() per slot.

Includes a roundtrip pin: pack output → disk → objtype.LoadVarnTypes
decodes 2 entries with correct DebugName + Type. Binds wire-format
parity (rsbuf_roundtrip_tests memory).

NAI-192-D-DEADBRANCH-OMITTED: TS empty stringKeys/numberKeys/
booleanKeys branches omitted; revive when a schema addition needs them.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `parseVarsConfig` + `packVarsConfigs` in `pkg/pack/vars.go`

**Why a separate task and not a generic helper:** TS keeps them as separate functions with identical bodies; future schema additions will diverge (varn is npc-scoped, vars is shared-scoped — different deviation tracks). Mirror the TS shape verbatim.

**Files:**
- Create: `pkg/pack/vars.go`
- Create: `pkg/pack/vars_test.go`

### Steps

- [ ] **Step 7.1: Write the failing tests**

Create `pkg/pack/vars_test.go`:

```go
package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseVarsConfig_TypeAccepted(t *testing.T) {
	v, ok, err := parseVarsConfig("type", "int")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false for known type")
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("v=%v, want ScriptVarTypeInt", v)
	}
}

func TestParseVarsConfig_UnknownKey(t *testing.T) {
	_, ok, err := parseVarsConfig("not_a_key", "whatever")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok=true; want false for unknown key")
	}
}

func TestParseVarsConfig_UnknownType(t *testing.T) {
	_, _, err := parseVarsConfig("type", "bogus")
	if err == nil {
		t.Fatal("want err for unknown type")
	}
}

func TestPackVarsConfigs_BytePin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.vars"),
		"[shared_quest]\ntype=int\n[shared_score]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=shared_quest\n1=shared_score\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".vars", nil, parseVarsConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "vars", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarsConfigs(cfgs, pf)

	// shared_quest: 12 chars + LF = 13 bytes name; +1 opcode = 14; + 2 type = 16; + 1 terminator = 17 body bytes
	// Wait — recompute: 2 (type opcode+code) + 1 (250 opcode) + 12+1 (name+LF) + 1 (terminator) = 17.
	// idx entry-length = bytes since marker NOT counting the size header that's only written once.
	// shared_score: 12 chars + LF = 13; same totals = 17.
	wantDat := []byte{
		0x00, 0x02,
		0x01, 0x69,
		0xfa, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x5f, 0x71, 0x75, 0x65, 0x73, 0x74, 0x0a,
		0x00,
		0x01, 0x69,
		0xfa, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x5f, 0x73, 0x63, 0x6f, 0x72, 0x65, 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x\nwant % x", pd.Dat.Data, wantDat)
	}
	// 2 (type) + 1 (250) + 13 (name+LF) + 1 (terminator) = 17 = 0x11
	wantIdx := []byte{0x00, 0x02, 0x00, 0x11, 0x00, 0x11}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x\nwant % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackVarsConfigs_RoundtripThroughObjtypeLoader(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.vars"),
		"[shared_quest]\ntype=int\n[shared_score]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=shared_quest\n1=shared_score\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".vars", nil, parseVarsConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "vars", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarsConfigs(cfgs, pf)

	outDir := filepath.Join(dir, "out")
	serverDir := filepath.Join(outDir, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pd.Save(filepath.Join(serverDir, "vars.dat"), filepath.Join(serverDir, "vars.idx")); err != nil {
		t.Fatal(err)
	}

	vc, err := objtype.LoadVarsTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vc.Configs) != 2 {
		t.Fatalf("Configs len=%d, want 2", len(vc.Configs))
	}
	if vc.Configs[0].DebugName != "shared_quest" {
		t.Fatalf("Configs[0].DebugName=%q", vc.Configs[0].DebugName)
	}
	if vc.Configs[0].Type != objtype.ScriptVarTypeInt {
		t.Fatalf("Configs[0].Type=%d", vc.Configs[0].Type)
	}
}
```

- [ ] **Step 7.2: Verify `LoadVarsTypes` exists with the same shape as `LoadVarnTypes`**

```bash
grep -n 'func LoadVarsTypes\|type VarsTypeConfigs\|type VarSharedType' pkg/objtype/varstype.go
```

Expected: `func LoadVarsTypes(dir string) (*VarsTypeConfigs, error)` exists. If the shape differs from `LoadVarnTypes`, adjust the test's struct-field accesses accordingly. (At HEAD `957b58f`, `varstype.go` mirrors `varntype.go` modulo type names.)

- [ ] **Step 7.3: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestParseVarsConfig|TestPackVarsConfigs' ./pkg/pack/...
```

Expected: FAIL with `undefined: parseVarsConfig` / `undefined: packVarsConfigs`.

- [ ] **Step 7.4: Create `pkg/pack/vars.go`**

```go
package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseVarsConfig is structurally identical to parseVarnConfig — same
// schema (`type=<scriptvartype-name>`), same accept/reject contract.
// Kept as a separate function to mirror TS one-per-config-domain shape;
// future schema additions are expected to diverge (e.g. var-shared
// scope vs var-npc scope).
//
// TS source: tools/pack/config/VarsConfig.ts:5-51.
func parseVarsConfig(key, value string) (ConfigValue, bool, error) {
	if key == "type" {
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	}
	return nil, false, nil
}

// packVarsConfigs writes the .vars cache buffer using the same opcode
// shape as varn: 0x01 + 1-byte type code, then 0xfa + LF-terminated
// debugname, then terminator. See packVarnConfigs.
//
// TS source: tools/pack/config/VarsConfig.ts:53-82.
func packVarsConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			for _, line := range cfg {
				if line.Key == "type" {
					pd.P1(1)
					pd.P1(uint8(line.Value.(objtype.ScriptVarType)))
				}
			}
		}
		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}
	return pd
}
```

- [ ] **Step 7.5: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestParseVarsConfig|TestPackVarsConfigs' ./pkg/pack/...
```

Expected: PASS for all 5 tests.

- [ ] **Step 7.6: Commit**

```bash
git add pkg/pack/vars.go pkg/pack/vars_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-192 T7 — parseVarsConfig + packVarsConfigs

Structurally identical to varn: type=<name> accepted, others ok=false,
packer emits opcode 0x01 + type code + 0xfa + name+LF, terminator
per slot. Kept as separate function per TS one-per-config-domain
shape; future schema additions expected to diverge.

Includes a roundtrip pin: pack output → disk → objtype.LoadVarsTypes
decodes correctly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `PackConfigs` orchestrator + integration test + cross-package consumer test

**Files:**
- Create: `pkg/pack/pack_configs.go`
- Create: `pkg/pack/pack_configs_test.go`

### Steps

- [ ] **Step 8.1: Write the failing tests**

Create `pkg/pack/pack_configs_test.go`:

```go
package pack

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_EndToEnd_VarnAndVars(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"),
		"[shared_quest]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n1=npchealth\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=shared_quest\n")
	ClearFsCache()

	outDir := filepath.Join(dir, "out")
	if err := PackConfigs(dir, outDir); err != nil {
		t.Fatal(err)
	}

	// varn outputs exist
	for _, p := range []string{"varn.dat", "varn.idx", "vars.dat", "vars.idx"} {
		full := filepath.Join(outDir, "server", p)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("missing %s: %v", full, err)
		}
	}

	// Roundtrip through loaders.
	vnc, err := objtype.LoadVarnTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vnc.Configs) != 2 {
		t.Fatalf("varn configs len=%d", len(vnc.Configs))
	}
	if vnc.Configs[0].DebugName != "npctier" || vnc.Configs[1].DebugName != "npchealth" {
		t.Fatalf("varn names=%q,%q", vnc.Configs[0].DebugName, vnc.Configs[1].DebugName)
	}

	vsc, err := objtype.LoadVarsTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vsc.Configs) != 1 {
		t.Fatalf("vars configs len=%d", len(vsc.Configs))
	}
	if vsc.Configs[0].DebugName != "shared_quest" {
		t.Fatalf("vars name=%q", vsc.Configs[0].DebugName)
	}
}

func TestPackConfigs_FreshnessGateSkipsSecondRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n")
	ClearFsCache()

	outDir := filepath.Join(dir, "out")
	if err := PackConfigs(dir, outDir); err != nil {
		t.Fatal(err)
	}
	datPath := filepath.Join(outDir, "server", "varn.dat")
	info1, err := os.Stat(datPath)
	if err != nil {
		t.Fatal(err)
	}
	mtime1 := info1.ModTime()

	// Sleep slightly to ensure mtime resolution can distinguish writes.
	time.Sleep(20 * time.Millisecond)

	// Re-run — ShouldBuild should return false because varn.dat is
	// fresher than the source files.
	ClearFsCache()
	if err := PackConfigs(dir, outDir); err != nil {
		t.Fatal(err)
	}
	info2, err := os.Stat(datPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info2.ModTime().Equal(mtime1) {
		t.Fatalf("file rewritten unexpectedly: mtime1=%v mtime2=%v", mtime1, info2.ModTime())
	}
}

func TestPackConfigs_NoSourceFilesReturnsNoError(t *testing.T) {
	dir := t.TempDir()
	// No scripts at all.
	ClearFsCache()
	if err := PackConfigs(dir, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	// No outputs expected.
	if _, err := os.Stat(filepath.Join(dir, "out", "server", "varn.dat")); !os.IsNotExist(err) {
		t.Fatalf("varn.dat should not exist; err=%v", err)
	}
}

func TestPackConfigs_PropagatesParseError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[npctier]\nbad_key=anything\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n")
	ClearFsCache()
	err := PackConfigs(dir, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
}
```

- [ ] **Step 8.2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPackConfigs ./pkg/pack/...
```

Expected: FAIL with `undefined: PackConfigs`.

- [ ] **Step 8.3: Create `pkg/pack/pack_configs.go`**

```go
package pack

import (
	"path/filepath"
)

// PackConfigs runs the per-config packing pipeline. NAI-192 wires only
// .varn and .vars; subsequent NAI-193+ sub-specs add branches.
//
// Each branch is freshness-gated via ShouldBuild against the relevant
// source extension. Outputs land at <outDir>/server/<type>.{dat,idx}.
//
// NAI-192-D-VARP-UNIQUENESS-DEFERRED: TS PackShared.packConfigs runs
// a cross-domain var-name uniqueness check across {VarpPack, VarnPack,
// VarsPack}. Deferred — lands with whichever of {varp, varn, vars} is
// last to ship. No production callsite this slice, so fixture-driven
// duplicates cannot reach the orchestrator.
//
// NAI-192-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarnPack/VarsPack singletons; goscape constructs *PackFile from
// srcDir per call (NAI-191 §2 deferred all 26 singletons).
//
// TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).
func PackConfigs(srcDir, outDir string) error {
	constants, err := LoadConstants(srcDir)
	if err != nil {
		return err
	}

	// TODO(NAI-VARP+): var-name uniqueness across {VarpPack, VarnPack, VarsPack}.

	scriptsDir := filepath.Join(srcDir, "scripts")
	serverOut := filepath.Join(outDir, "server")

	if ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
		if err := packAndSaveVarn(srcDir, serverOut, constants); err != nil {
			return err
		}
	}

	if ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
		if err := packAndSaveVars(srcDir, serverOut, constants); err != nil {
			return err
		}
	}

	return nil
}

func packAndSaveVarn(srcDir, serverOut string, c Constants) error {
	pf, err := NewPackFile(srcDir, "varn", nil)
	if err != nil {
		return err
	}
	cfgs, err := ReadTypedConfigs(srcDir, ".varn", nil, parseVarnConfig, c)
	if err != nil {
		return err
	}
	pd := packVarnConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "varn.dat"), filepath.Join(serverOut, "varn.idx"))
}

func packAndSaveVars(srcDir, serverOut string, c Constants) error {
	pf, err := NewPackFile(srcDir, "vars", nil)
	if err != nil {
		return err
	}
	cfgs, err := ReadTypedConfigs(srcDir, ".vars", nil, parseVarsConfig, c)
	if err != nil {
		return err
	}
	pd := packVarsConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "vars.dat"), filepath.Join(serverOut, "vars.idx"))
}
```

- [ ] **Step 8.4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPackConfigs ./pkg/pack/...
```

Expected: PASS for all 4 tests.

- [ ] **Step 8.5: Full package test + race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... ./pkg/objtype/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/...
```

Expected: All PASS. Race detector run is the binding evidence that the shared `FsCache` + `ClearFsCache` pattern is safe under the parallel-test default (`-race` toggles t.Parallel-safe execution; we don't use t.Parallel() here but `-race` still catches accidental shared state).

- [ ] **Step 8.6: Commit**

```bash
git add pkg/pack/pack_configs.go pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-192 T8 — PackConfigs orchestrator + integration tests

PackConfigs(srcDir, outDir) freshness-gates each per-config branch via
ShouldBuild and writes <outDir>/server/<type>.{dat,idx}. NAI-192 wires
.varn + .vars; subsequent sub-specs add branches.

Integration tests cover: end-to-end pack + roundtrip through
objtype.LoadVarnTypes / LoadVarsTypes, freshness gate skips second
run (mtime unchanged), no source files = no-op, parse errors
propagate to caller.

NAI-192-D-VARP-UNIQUENESS-DEFERRED: cross-domain var-name uniqueness
check across {VarpPack, VarnPack, VarsPack} deferred to the last of
the three to ship.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Deviation-tag absence pins

**Why:** Per `ts_asymmetry_dual_pin`, deviation tags need both a presence-pin (the code itself) AND an absence-pin (regression catches if a future change accidentally adds the deviated-against TS feature). Without this, a drive-by adding `var VarnPack *PackFile` at the package level would silently violate `NAI-192-D-PACKFILE-SINGLETONS-DEFERRED`.

**Files:**
- Create: `pkg/pack/nai192_deviation_pins_test.go`

### Steps

- [ ] **Step 9.1: Write the absence-pin tests**

Create `pkg/pack/nai192_deviation_pins_test.go`:

```go
package pack

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scanPackageDecls parses every non-test .go file in pkg/pack and
// returns all top-level identifier names declared as var/const/type/func.
func scanPackageDecls(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	// Resolve the package directory relative to this test file.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(wd, e.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, n := range s.Names {
							names[n.Name] = true
						}
					case *ast.TypeSpec:
						names[s.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					names[d.Name.Name] = true
				}
			}
		}
	}
	return names
}

// NAI-192-D-PACKFILE-SINGLETONS-DEFERRED: no module-level VarnPack /
// VarsPack *PackFile decls.
func TestNAI192_PackFileSingletonsDeferred_NoModuleLevelVarnPack(t *testing.T) {
	decls := scanPackageDecls(t)
	for _, name := range []string{"VarnPack", "VarsPack"} {
		if decls[name] {
			t.Errorf("found top-level decl %q in pkg/pack — violates NAI-192-D-PACKFILE-SINGLETONS-DEFERRED", name)
		}
	}
}

// NAI-192-D-VARP-UNIQUENESS-DEFERRED: PackConfigs source must NOT
// reference a cross-domain uniqueness check identifier.
func TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator(t *testing.T) {
	body, err := os.ReadFile("pack_configs.go")
	if err != nil {
		t.Fatal(err)
	}
	// Heuristic: any identifier containing "Unique" in the orchestrator
	// would imply the check landed early.
	if strings.Contains(string(body), "checkVarNameUniqueness") ||
		strings.Contains(string(body), "uniqueVarNames") {
		t.Error("PackConfigs references a uniqueness-check identifier — violates NAI-192-D-VARP-UNIQUENESS-DEFERRED")
	}
}

// NAI-192-D-DEADBRANCH-OMITTED: parseVarnConfig / parseVarsConfig
// source must NOT contain stringKeys / numberKeys / booleanKeys
// identifiers. (The empty TS branches are intentionally omitted.)
func TestNAI192_DeadBranchOmitted_NoEmptyKeyArrays(t *testing.T) {
	for _, fn := range []string{"varn.go", "vars.go"} {
		body, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		s := string(body)
		for _, banned := range []string{"stringKeys", "numberKeys", "booleanKeys"} {
			if strings.Contains(s, banned) {
				t.Errorf("%s contains %q — violates NAI-192-D-DEADBRANCH-OMITTED", fn, banned)
			}
		}
	}
}

// NAI-192-D-PACKET-WRITE-CURSOR: PackedData.Next must use Dat.Length()
// for write-cursor arithmetic, NOT Dat.Pos. A regression to Dat.Pos
// would silently produce wrong idx offsets.
func TestNAI192_PacketWriteCursor_UsesLengthNotPos(t *testing.T) {
	body, err := os.ReadFile("packed_data.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	// Permissive: any read of "Pos" anywhere in Next() would be a bug.
	// Locate the Next() function body and inspect it.
	startMarker := "func (pd *PackedData) Next()"
	start := strings.Index(s, startMarker)
	if start < 0 {
		t.Fatal("Next() not found")
	}
	// Find the end of the function (first `}` at column 0 after start).
	end := strings.Index(s[start:], "\n}")
	if end < 0 {
		t.Fatal("Next() end not found")
	}
	body_ := s[start : start+end]
	if strings.Contains(body_, "Dat.Pos") || strings.Contains(body_, ".Pos") {
		t.Error("PackedData.Next() references Dat.Pos — violates NAI-192-D-PACKET-WRITE-CURSOR")
	}
	if !strings.Contains(body_, "Dat.Length()") {
		t.Error("PackedData.Next() must use Dat.Length() per NAI-192-D-PACKET-WRITE-CURSOR")
	}
}
```

- [ ] **Step 9.2: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI192 ./pkg/pack/...
```

Expected: PASS for all 4 absence-pin tests.

- [ ] **Step 9.3: Final full-package run**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: All PASS, `go vet` clean. If anything outside `pkg/pack` / `pkg/objtype` fails, it is a pre-existing failure unrelated to NAI-192 — verify by running the same suite at `HEAD~N` (where N = task count) and comparing.

- [ ] **Step 9.4: Commit**

```bash
git add pkg/pack/nai192_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-192 T9 — deviation-tag absence pins

Four absence-pin tests catch regressions against the NAI-192-D-*
deviation tags:

- PACKFILE-SINGLETONS-DEFERRED: no top-level VarnPack/VarsPack decl
- VARP-UNIQUENESS-DEFERRED: PackConfigs has no uniqueness identifier
- DEADBRANCH-OMITTED: varn.go/vars.go have no stringKeys/numberKeys/
  booleanKeys identifiers
- PACKET-WRITE-CURSOR: PackedData.Next() uses Dat.Length(), not Dat.Pos

Per ts_asymmetry_dual_pin memory: presence-pin AND absence-pin for
every deviation. A drive-by re-adding any deviated-against TS feature
will fail one of these.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| §4.1 PackedData | T2 |
| §4.2 ConfigValue / ConfigLine / IsConfigBoolean / GetConfigBoolean | T3 |
| §4.3 Constants + LoadConstants + substituteConstants | T4 |
| §4.4 ReadTypedConfigs + ParseFn | T5 |
| §4.5 ScriptVarTypeFromName | T1 |
| §4.6 parseVarnConfig + packVarnConfigs | T6 |
| §4.7 parseVarsConfig + packVarsConfigs | T7 |
| §4.8 PackConfigs orchestrator | T8 |
| §7.1 unit tests (each component) | T2-T5 |
| §7.2 byte-pin tests (varn + vars) | T6, T7 |
| §7.3 integration test (end-to-end) | T8 |
| §7.4 cross-package consumer test | T6 (varn) + T7 (vars) + T8 (orchestrator-level) |
| §7.5 deviation-tag pins | T9 |
| §10 deviation tags | All — codified in code comments + T9 absence-pins |

**Placeholder scan:** No `TBD`/`TODO`/`fill in later`/"similar to" — every code block contains the actual implementation. The single `TODO(NAI-VARP+)` comment in `pack_configs.go` Step 8.3 is an intentional in-code marker for the deferred uniqueness check (matches the pattern of other goscape deferral comments).

**Type consistency:**
- `ParseFn func(key, value string) (ConfigValue, bool, error)` declared in T5; used identically in T6 and T7.
- `PackedData.PJStr` uses `PJStrLF` consistently (T2 step 2.3, T6 byte-pin uses 0x0a terminator, T7 byte-pin uses 0x0a terminator).
- `packVarnConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData` signature identical between T6 declaration and T8 caller.
- `Constants = map[string]string` declared in T4 step 4.3; threaded through T5 and T8 with consistent type.
- `ScriptVarTypeFromName(name string) (ScriptVarType, bool)` declared in T1; called identically in T6 step 6.3 and T7 step 7.4.

All good. Plan ready to execute.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-13-nai-192-varn-vars-packers.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task, two-stage review between tasks, fast iteration. Best for this plan because tasks are sequential with strict file-scope (one subagent per task) and the integration test (T8) binds everything via roundtrip.

2. **Inline Execution** — execute all tasks in this session with executing-plans, batch checkpoints for review.

Which approach?
