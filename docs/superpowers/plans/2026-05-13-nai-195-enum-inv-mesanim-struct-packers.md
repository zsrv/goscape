# NAI-195 — `.enum` + `.inv` + `.mesanim` + `.struct` packer slice — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `tools/pack/config/{EnumConfig,InvConfig,MesAnimConfig,StructConfig}.ts` onto the NAI-191–194 `PackShared` infrastructure. Adds 4 server-only per-config packer branches to `PackConfigs`. Introduces a runtime `ParamType` registry load step between `.param` save and `.struct` parse (TS `PackShared.ts:334`). Hoists `lk *paramLookups` to function scope so `.enum`/`.struct` reuse the lookup tables built by `.param`.

**Architecture:** Four new `pkg/pack/<config>.go` files (parser + packer per config), one extension to `pkg/pack/pack_configs.go` (4 gated branches, lazy `objPack`/`seqPack`/`paramTypes` construction, `lk` hoisting), one new `pkg/pack/config_value.go` addition (`ParamValue` struct), one new deviation-pin file. Server-only — no client jagfile entries; no CRC validator callback (continues `VALIDATE-DEFERRED`).

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` + `pkg/io/jagfile` + NAI-191/192/193/194 `pkg/pack` foundation + `pkg/objtype` (ScriptVarType, InvType*Scope* constants, LoadParamTypes, LoadEnumTypes, LoadInvTypes, LoadMesanimTypes, LoadStructTypes).

**Spec:** `docs/superpowers/specs/2026-05-13-nai-195-enum-inv-mesanim-struct-packers-design.md` (commit `db0abfa`).
**HEAD at plan-write:** `db0abfa`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches `pkg/pack/varp_test.go`/`varn_test.go`/`param_test.go`: bare `if err != nil { t.Fatal(err) }`, `bytes.Equal` for byte comparison, `t.Fatalf("got % x, want % x", got, want)` for byte diffs, `t.TempDir()` for fixture roots, `ClearFsCache()` before tests that mutate the FS.
- **Existing helpers in `pkg/pack`** (use, do NOT redefine):
  - `writeFile(t *testing.T, path, content string)` — constants_test.go:10
  - `newTestPF(packType string, entries map[int]string) *PackFile` — param_test.go:54
  - `scanPackageDecls(t *testing.T) map[string]bool` — nai192_deviation_pins_test.go:15
- **Error envelope** matches `pkg/pack/parse.go`: `fmt.Errorf("<kind>: %s", detail)` or `fmt.Errorf("<context>: %w", err)` for wrapping. TS `packStepError(debugname, msg)` analogue: `fmt.Errorf("%s: %s", debugname, msg)`.
- **Modern Go**: `for id := range pf.Max`, `slices.Index`, `strconv.ParseInt(_, 0, 64)`, `strings.Cut`.
- **Identifier conventions**:
  - Per-config files: `enum.go`, `inv.go`, `mesanim.go`, `struct.go` (mirrors `param.go`/`varp.go`).
  - Parsers without closure deps: `parseEnumConfig`. Parsers with closure deps: `parseInvConfigFor(objPack) func(key, value string) (...)`, `parseMesAnimConfigFor(seqPack)`, `parseStructConfigFor(paramTypes)`.
  - Packers: `packEnumConfigs`, `packInvConfigs`, `packMesAnimConfigs`, `packStructConfigs`.
  - Orchestrator helpers: `packAndSaveEnum`, `packAndSaveInv`, `packAndSaveMesAnim`, `packAndSaveStruct`.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `db0abfa`:

| Premise | Verification |
|---|---|
| `pkg/objtype.InvTypeScopeShared = 2`, `InvTypeScopePerm = 1`, `InvTypeScopeTemp = 0` | ✅ `invtype.go:11-13` |
| `pkg/objtype.ScriptVarTypeFromName(name string) (ScriptVarType, bool)` exists | ✅ `scriptvartype.go:40` |
| `pkg/objtype.ScriptVarTypeAutoInt = 255`, `ScriptVarTypeInt = 105`, `ScriptVarTypeString = 115` | ✅ `scriptvartype.go:11-13` |
| `pkg/objtype.ParamTypeConfigs{ConfigNames map[string]int, Configs []*ParamType}` with `Configs[id].Type ScriptVarType` | ✅ `paramtype.go:27-29,79-83` |
| `pkg/objtype.LoadParamTypes(dir string) (*ParamTypeConfigs, error)` reads `dir/server/param.dat` | ✅ `paramtype.go:38-50` |
| `pkg/objtype.LoadEnumTypes(dir)` reads `dir/server/enum.dat` | ✅ `enumtype.go:71` |
| `pkg/objtype.LoadInvTypes(dir)` reads `dir/server/inv.dat` | ✅ `invtype.go` (function exists per grep) |
| `pkg/objtype.LoadMesanimTypes(dir)` reads `dir/server/mesanim.dat`; returns empty registry on missing | ✅ `mesanimtype.go:50` |
| `pkg/objtype.LoadStructTypes(dir)` reads `dir/server/struct.dat` | ✅ `structtype.go:44` |
| `pkg/objtype.EnumType` decodes opcodes 1/2/3/4/5/6/250; `5` reads count=G2 + per-entry G4+GJStrLF; `6` reads G4+G4 | ✅ `enumtype.go:22-49` |
| `pkg/objtype.InvType` decodes opcodes 1-9/250; opcode 4 reads count=G1 + per-entry G2+G2+G4 | ✅ `invtype.go:31-65` |
| `pkg/objtype.MesanimType` decodes opcodes 1-4 (`Len[code-1] = G2`) and 250 | ✅ `mesanimtype.go:36-43` |
| `pkg/objtype.StructType` decodes opcode 249 via `DecodeParams` (count=G1 + per-entry G3+GBool+GJStrLF\|G4) and 250 | ✅ `structtype.go:18-25`, `paramtype.go:12-25` |
| `pkg/pack.PackedData.P3(uint32)` exists; `PBool(bool)` exists; `Length()` is the write cursor | ✅ `packed_data.go:51-55` |
| `pkg/pack.lookupParamValue(typ ScriptVarType, value string, lk *paramLookups) (any, error)` exists; returns `int` or `string`; `null` sentinel returns `-1` or `""` | ✅ `param.go:92` |
| `pkg/pack.paramLookups` has `enumPF/objPF/locPF/interfacePF/structPF/categoryPF/spotanimPF/npcPF/invPF/synthPF/seqPF/varpPF/dbrowPF` (NO `paramPF`) | ✅ `param.go:55-69` |
| `pkg/pack.loadParamLookups(srcDir, varpPF) (*paramLookups, error)` builds 12 typed PackFiles + threads `varpPF` | ✅ `pack_configs.go:205` |
| `pkg/pack.PackFile.GetByName(name) int` returns `-1` for missing | ✅ `packfile.go:192` |
| `pkg/pack.PackFile.GetByID(id) string` returns name or `""` | ✅ `packfile.go:188` |
| `pkg/pack.NewPackFile(srcDir, packType, validator)` exists | ✅ `packfile.go` |
| `pkg/pack.ReadTypedConfigs(srcDir, ext, required []string, parseFn ParseFn, c Constants)` signature accepts a per-call closure | ✅ `read_typed.go:37` |
| `pkg/pack.ConfigValue = any`, `pkg/pack.ConfigLine{Key, Value}` | ✅ `config_value.go:9-14` |
| `pkg/pack.IsConfigBoolean(string) bool` / `GetConfigBoolean(string) bool` | ✅ `config_value.go:23-37` |
| `pkg/pack.ClearFsCache()` exists for FS-mutating tests | ✅ used in `crawl_test.go` |
| `pkg/pack.scanPackageDecls(t)` returns `map[string]bool` of top-level decls | ✅ `nai192_deviation_pins_test.go:15` |
| `pkg/pack.newTestPF(packType, entries map[int]string)` constructs in-memory `*PackFile` for byte-pin tests | ✅ `param_test.go:54` |
| `pkg/pack.PackConfigs(srcDir, outDir)` orchestrator currently declares `lk` inside the `.param` `if` block | ✅ `pack_configs.go:119` (will be hoisted in T6) |
| `pkg/pack.checkVarNameUniqueness` does NOT include enum/inv/mesanim/struct — only the var-name trio | ✅ `pack_configs.go:146` (intentional; spec §2 keeps this scope) |

**TS-side premises** (verified by reading `tools/pack/config/`):

| TS premise | Source line |
|---|---|
| `parseMesAnimConfig` accepts only `len*` keys; resolves via `SeqPack.getByName` | `MesAnimConfig.ts:45-51` |
| `packMesAnimConfigs` emits per-`lenN` opcode = `max(0, parsedLen-1)+1` then `p2(seqIdx)` | `MesAnimConfig.ts:69-78` |
| `parseEnumConfig` accepts `inputtype`/`outputtype` (→ScriptVarType char), `default`/`val` (raw strings) | `EnumConfig.ts:6-57` |
| `packEnumConfigs` pre-finds `inputtype` + `outputtype`; emits 1/2/3/4 during walk; AUTOINT inputtype writes INT byte | `EnumConfig.ts:60-100` |
| `packEnumConfigs` emits opcode 5 (STRING) or 6 (else) + `p2(val.length)` + per-val resolved key/value | `EnumConfig.ts:102-145` |
| `packEnumConfigs` AUTOINT inputtype: val key = `p4(i)` (loop index); else: split val on `,`, resolve keyPart via `lookupParamValue(inputtype, keyPart)` | `EnumConfig.ts:110-121` |
| `packEnumConfigs` AUTOINT outputtype: value = `lookupParamValue(outputtype, val[i])` (whole string); else: split on `,`, resolve `val[i].substring(indexOf(',')+1)` | `EnumConfig.ts:122-143` |
| `parseInvConfig` numeric `size` bounded `[0, 65535]`; booleans `stackall/restock/allstock/protect/runweight/dummyinv`; scope `shared\|perm\|temp` → SCOPE_* | `InvConfig.ts:8-66` |
| `parseInvConfig` `stockN` parses to `[objId, count]` or `[objId, count, respawn]`; objId from `ObjPack.getByName` | `InvConfig.ts:67-88` |
| `packInvConfigs` opcodes: 1=scope, 2=size, 3=stackall-true, 5=restock-true, 6=allstock-true, 7=**protect-false**, 8=runweight-true, 9=dummyinv-true | `InvConfig.ts:118-159` |
| `packInvConfigs` stockN error paths: duplicate stockN → throw; `index >= size` → throw | `InvConfig.ts:124-132` |
| `packInvConfigs` stock list trailer (when ≥1 stock present): opcode 4 + `p1(stock.length)` + per-slot present `p2(id)+p2(count)+p4(rate\|0)` or hole `p2(-1)+p2(0)+p4(0)` | `InvConfig.ts:162-183` |
| `parseStructConfig` accepts only `param=name,value`; resolves name via `ParamType.getByName(name)`; resolves value via `lookupParamValue(param.type, value)` | `StructConfig.ts:48-64` |
| `packStructConfigs` emits opcode 249 + `p1(params.length)` + per-param `p3(id)+pbool(type==STRING)+pjstr(value)\|p4(value)` | `StructConfig.ts:89-103` |
| All 4 packers emit 250-trailer + `pjstr(debugname)` when `debugname.length`; then `client.next()` + `server.next()` | `*.ts` per-file |

---

## File inventory

```
pkg/pack/
  config_value.go                          MODIFY (add ParamValue struct)
  mesanim.go                               NEW    (parseMesAnimConfigFor + packMesAnimConfigs)
  mesanim_test.go                          NEW    (parser + packer byte-pin tests)
  enum.go                                  NEW    (parseEnumConfig + packEnumConfigs)
  enum_test.go                             NEW    (parser + packer byte-pin tests)
  inv.go                                   NEW    (parseInvConfigFor + packInvConfigs)
  inv_test.go                              NEW    (parser + packer byte-pin tests, incl. error paths)
  struct.go                                NEW    (parseStructConfigFor + packStructConfigs)
  struct_test.go                           NEW    (parser + packer byte-pin tests)
  pack_configs.go                          MODIFY (hoist lk, ensure helpers, 4 branches,
                                                   ParamType runtime load, new deviation doc)
  pack_configs_test.go                     MODIFY (add TestPackConfigs_EightConfigsLand)
  mesanim_roundtrip_test.go                NEW    (LoadMesanimTypes round-trip)
  enum_roundtrip_test.go                   NEW    (LoadEnumTypes round-trip)
  inv_roundtrip_test.go                    NEW    (LoadInvTypes round-trip)
  struct_roundtrip_test.go                 NEW    (LoadStructTypes round-trip, exercises ParamType load)
  nai195_deviation_pins_test.go            NEW

docs/superpowers/plans/
  2026-05-13-nai-195-enum-inv-mesanim-struct-packers.md   NEW (this file)
```

---

## Task 1 — `ParamValue` struct prep

**Files:**
- Modify: `pkg/pack/config_value.go` (append struct).

**Rationale:** TS `parseStructConfig` returns `ParamValue {id, type, value}`. NAI-194 introduced `lookupParamValue` returning bare `any`. T5 (`struct.go`) needs a small struct to carry the triplet through pack-time.

- [ ] **Step 1.1: Append `ParamValue` to `pkg/pack/config_value.go`**

```go
// ParamValue is the parser output of a `param=name,value` line in
// .struct (and any future config that emits typed-param values).
//
// Type is included so packStructConfigs can decide between p4 and
// pjstr at pack time without re-querying the ParamType registry.
//
// TS source: tools/pack/config/PackShared.ts ParamValue.
type ParamValue struct {
	ID    int
	Type  objtype.ScriptVarType
	Value any
}
```

Required import: add `"github.com/zsrv/goscape/pkg/objtype"` to the file's import block if not already present.

- [ ] **Step 1.2: Verify build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/pack/...
```

Expected: no errors.

- [ ] **Step 1.3: Commit**

```bash
git add pkg/pack/config_value.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-195 T1 — ParamValue struct

Carries (id, type, value) triplet from parseStructConfig through to
packStructConfigs. Mirrors TS PackShared.ts ParamValue.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — `.mesanim` parser + packer

**Files:**
- Create: `pkg/pack/mesanim.go`
- Create: `pkg/pack/mesanim_test.go`

**TS source:** `tools/pack/config/MesAnimConfig.ts:1-92`.

### 2.1 — Parser

- [ ] **Step 2.1.1: Write failing parser test (`pkg/pack/mesanim_test.go`)**

```go
package pack

import (
	"strings"
	"testing"
)

func TestParseMesAnimConfig_LenKey(t *testing.T) {
	seqPF := newTestPF("seq", map[int]string{0: "idle", 1: "walk", 2: "death"})
	parse := parseMesAnimConfigFor(seqPF)

	v, ok, err := parse("len0", "walk")
	if err != nil || !ok {
		t.Fatalf("len0: ok=%v err=%v", ok, err)
	}
	if v.(int) != 1 {
		t.Fatalf("len0: got %d, want 1", v.(int))
	}
}

func TestParseMesAnimConfig_UnknownSeqName(t *testing.T) {
	seqPF := newTestPF("seq", map[int]string{0: "idle"})
	parse := parseMesAnimConfigFor(seqPF)

	_, ok, err := parse("len3", "doesnotexist")
	if !ok {
		t.Fatalf("len3: ok=false, want true (recognized key)")
	}
	if err == nil || !strings.Contains(err.Error(), "doesnotexist") {
		t.Fatalf("len3 unknown seq: err=%v, want containing 'doesnotexist'", err)
	}
}

func TestParseMesAnimConfig_UnknownKey(t *testing.T) {
	seqPF := newTestPF("seq", map[int]string{0: "idle"})
	parse := parseMesAnimConfigFor(seqPF)

	_, ok, err := parse("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v, want (false, nil)", ok, err)
	}
}
```

- [ ] **Step 2.1.2: Run — expect FAIL (parser undefined)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseMesAnimConfig -v
```

Expected: compile failure citing `parseMesAnimConfigFor` undefined.

- [ ] **Step 2.1.3: Implement parser (`pkg/pack/mesanim.go`)**

```go
package pack

import (
	"fmt"
	"strings"
)

// parseMesAnimConfigFor returns the per-key=value parser for .mesanim
// config blocks. Only `len*` keys are accepted; the value is the
// debug name of a seq looked up via seqPack.
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseMesAnimConfig declares empty
// stringKeys/numberKeys/booleanKeys arrays — dead branches preserved
// by the TS author. Goscape omits the empty branches; they revive when
// a future schema addition needs them.
//
// TS source: tools/pack/config/MesAnimConfig.ts:4-55.
func parseMesAnimConfigFor(seqPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if strings.HasPrefix(key, "len") {
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown seq: %s", value)
			}
			return idx, true, nil
		}
		return nil, false, nil
	}
}
```

- [ ] **Step 2.1.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseMesAnimConfig -v
```

Expected: 3 tests PASS.

### 2.2 — Packer

- [ ] **Step 2.2.1: Append packer test to `pkg/pack/mesanim_test.go`**

```go
import "bytes" // add to import block at top of file

func TestPackMesAnimConfigs_ByteExact_SingleLen(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "test_anim"})
	cfgs := map[string][]ConfigLine{
		"test_anim": {
			// len0 → opcode max(0, 0-1)+1 = 1
			{Key: "len0", Value: 7},
		},
	}
	pd := packMesAnimConfigs(cfgs, pf)
	// dat header (p2 size=1) + opcode 1 + p2(7) + 250 + pjstr("test_anim\n") + 0x00 terminator
	want := []byte{
		0x00, 0x01, // size=1
		0x01,       // opcode 1
		0x00, 0x07, // seq idx 7
		0xfa,                                                           // 250
		't', 'e', 's', 't', '_', 'a', 'n', 'i', 'm', 0x0a,              // pjstr LF
		0x00,                                                           // Next() terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("dat:\n got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackMesAnimConfigs_OpcodeFormula(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "x"})
	for _, tc := range []struct {
		key    string
		wantOp byte
	}{
		{"len0", 1}, // max(0, -1)+1 = 1
		{"len1", 1}, // max(0,  0)+1 = 1
		{"len2", 2}, // max(0,  1)+1 = 2
		{"len5", 5},
	} {
		cfgs := map[string][]ConfigLine{"x": {{Key: tc.key, Value: 0}}}
		pd := packMesAnimConfigs(cfgs, pf)
		// dat is: [00 01 OP 00 00 fa 'x' 0a 00]
		got := pd.Dat.Data[2]
		if got != tc.wantOp {
			t.Errorf("%s: opcode = %d, want %d", tc.key, got, tc.wantOp)
		}
	}
}

func TestPackMesAnimConfigs_NonNumericLenSkipped(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "x"})
	cfgs := map[string][]ConfigLine{
		"x": {{Key: "lenABC", Value: 0}},
	}
	pd := packMesAnimConfigs(cfgs, pf)
	// dat = [00 01 fa 'x' 0a 00] — no len opcode emitted
	want := []byte{0x00, 0x01, 0xfa, 'x', 0x0a, 0x00}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("non-numeric len: got % x, want % x", pd.Dat.Data, want)
	}
}

func TestPackMesAnimConfigs_NoConfigEmitsOnlyTrailer(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "named", 1: ""}) // slot 1 unnamed
	cfgs := map[string][]ConfigLine{} // no config for either
	pd := packMesAnimConfigs(cfgs, pf)
	want := []byte{
		0x00, 0x02, // size=2
		0xfa, 'n', 'a', 'm', 'e', 'd', 0x0a, 0x00, // slot 0: trailer + Next
		0x00, // slot 1: empty name → no trailer; just Next
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x, want % x", pd.Dat.Data, want)
	}
}
```

- [ ] **Step 2.2.2: Run — expect FAIL (packer undefined)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackMesAnimConfigs -v
```

Expected: compile failure citing `packMesAnimConfigs` undefined.

- [ ] **Step 2.2.3: Implement packer (append to `pkg/pack/mesanim.go`)**

```go
import "strconv" // add to import block

// packMesAnimConfigs emits the per-id body for .mesanim configs. Each
// id walks the config block once, emitting per-`lenN` opcodes via
// max(0, parsedLen-1)+1 followed by p2(seqIdx). The 250-trailer fires
// when the slot has a non-empty debugname. Each id ends with Next()
// (terminator + idx length).
//
// TS source: tools/pack/config/MesAnimConfig.ts:57-90.
func packMesAnimConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			for _, line := range cfg {
				if !strings.HasPrefix(line.Key, "len") {
					continue
				}
				lenN, err := strconv.Atoi(line.Key[3:])
				if err != nil {
					// non-numeric `lenN` suffix → TS isNaN-continue
					continue
				}
				opcode := lenN - 1
				if opcode < 0 {
					opcode = 0
				}
				opcode++
				pd.P1(uint8(opcode))
				pd.P2(uint16(line.Value.(int)))
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

- [ ] **Step 2.2.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackMesAnimConfigs -v
```

Expected: 4 packer tests PASS.

- [ ] **Step 2.2.5: Run full pack package — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...
```

Expected: all pre-existing tests still PASS.

- [ ] **Step 2.2.6: Commit**

```bash
git add pkg/pack/mesanim.go pkg/pack/mesanim_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-195 T2 — .mesanim parser + packer

parseMesAnimConfigFor(seqPack) returns per-key parser for len* keys,
resolving via seqPack.GetByName. packMesAnimConfigs emits per-lenN
opcode = max(0, parsedLen-1)+1 + p2(seqIdx) + 250 trailer.

NAI-192-D-DEADBRANCH-OMITTED applied (empty TS stringKeys/numberKeys
/booleanKeys arrays omitted).

TS source: tools/pack/config/MesAnimConfig.ts:1-92.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — `.enum` parser + packer

**Files:**
- Create: `pkg/pack/enum.go`
- Create: `pkg/pack/enum_test.go`

**TS source:** `tools/pack/config/EnumConfig.ts:1-157`.

### 3.1 — Parser

- [ ] **Step 3.1.1: Write failing parser test (`pkg/pack/enum_test.go`)**

```go
package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseEnumConfig_InputType(t *testing.T) {
	v, ok, err := parseEnumConfig("inputtype", "int")
	if err != nil || !ok {
		t.Fatalf("inputtype: ok=%v err=%v", ok, err)
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("inputtype got %v, want Int", v)
	}
}

func TestParseEnumConfig_OutputType(t *testing.T) {
	v, ok, err := parseEnumConfig("outputtype", "string")
	if err != nil || !ok {
		t.Fatalf("outputtype: ok=%v err=%v", ok, err)
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeString {
		t.Fatalf("outputtype got %v, want String", v)
	}
}

func TestParseEnumConfig_UnknownScriptVarType(t *testing.T) {
	_, ok, err := parseEnumConfig("inputtype", "notatype")
	if !ok {
		t.Fatalf("inputtype unknown: ok=false, want true")
	}
	if err == nil || !strings.Contains(err.Error(), "notatype") {
		t.Fatalf("inputtype unknown: err=%v", err)
	}
}

func TestParseEnumConfig_DefaultAndVal_PassThrough(t *testing.T) {
	v, ok, err := parseEnumConfig("default", "raw_string_value")
	if err != nil || !ok || v.(string) != "raw_string_value" {
		t.Fatalf("default: ok=%v err=%v v=%v", ok, err, v)
	}
	v, ok, err = parseEnumConfig("val", "1,foo")
	if err != nil || !ok || v.(string) != "1,foo" {
		t.Fatalf("val: ok=%v err=%v v=%v", ok, err, v)
	}
}

func TestParseEnumConfig_UnknownKey(t *testing.T) {
	_, ok, err := parseEnumConfig("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v, want (false, nil)", ok, err)
	}
}
```

(The `bytes` import in the file header will be used by the packer tests appended in Step 3.2.1; if the implementer authors the parser tests first and the linter complains about unused imports, drop `bytes` and re-add it when appending packer tests.)

- [ ] **Step 3.1.2: Run — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseEnumConfig -v
```

Expected: compile failure citing `parseEnumConfig` undefined.

- [ ] **Step 3.1.3: Implement parser (`pkg/pack/enum.go`)**

```go
package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseEnumConfig is the per-key=value parser for .enum config blocks.
//
// Accepted keys:
//   - inputtype, outputtype  (ScriptVarType name → ScriptVarType code)
//   - default, val           (raw string; resolved at pack time)
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseEnumConfig declares empty
// stringKeys/numberKeys/booleanKeys arrays — dead branches preserved
// by the TS author. Goscape omits the empty branches.
//
// TS source: tools/pack/config/EnumConfig.ts:6-57.
func parseEnumConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "inputtype", "outputtype":
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	case "default", "val":
		return value, true, nil
	}
	return nil, false, nil
}
```

- [ ] **Step 3.1.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseEnumConfig -v
```

Expected: 5 tests PASS.

### 3.2 — Packer

- [ ] **Step 3.2.1: Append packer tests to `pkg/pack/enum_test.go`**

```go
// helper: build a paramLookups with a single enumPF for AUTOINT outputtype tests.
func newEnumLk(t *testing.T) *paramLookups {
	t.Helper()
	return &paramLookups{
		enumPF: newTestPF("enum", map[int]string{0: "first", 1: "second"}),
		objPF:  newTestPF("obj", map[int]string{0: "egg", 1: "bone"}),
	}
}

func TestPackEnumConfigs_IntOutputType_DefaultAndOneVal(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "test_enum"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"test_enum": {
			{Key: "inputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "outputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "default", Value: "42"},
			{Key: "val", Value: "7,99"},
		},
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packEnumConfigs: %v", err)
	}
	// dat: [size=1] [op1 INT] [op2 INT] [op4 p4(42)] [op6 p2(1) p4(7) p4(99)] [op250 pjstr] [Next 0x00]
	want := []byte{
		0x00, 0x01, // size=1
		0x01, 105,  // op1 inputtype=INT(105)
		0x02, 105,  // op2 outputtype=INT(105)
		0x04, 0x00, 0x00, 0x00, 0x2a, // op4 default=p4(42)
		0x06,                         // op6 (non-STRING trailer)
		0x00, 0x01,                   // p2(val count=1)
		0x00, 0x00, 0x00, 0x07,       // p4 key=7
		0x00, 0x00, 0x00, 0x63,       // p4 value=99
		0xfa, 't','e','s','t','_','e','n','u','m', 0x0a, // op250 + pjstr LF
		0x00, // Next terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("dat:\n got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackEnumConfigs_StringOutputType_StringDefaultAndVal(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "e"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"e": {
			{Key: "inputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "outputtype", Value: objtype.ScriptVarTypeString},
			{Key: "default", Value: "hi"},
			{Key: "val", Value: "1,abc"},
		},
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packEnumConfigs: %v", err)
	}
	want := []byte{
		0x00, 0x01,
		0x01, 105,
		0x02, 115,                          // outputtype=STRING(115)
		0x03, 'h','i', 0x0a,                // op3 default=pjstr("hi")
		0x05,                               // op5 (STRING trailer)
		0x00, 0x01,                         // count=1
		0x00, 0x00, 0x00, 0x01,             // p4 key=1
		'a','b','c', 0x0a,                  // pjstr("abc")
		0xfa, 'e', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackEnumConfigs_AutoIntInputType_RequiresCommaInVal(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "e"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"e": {
			{Key: "inputtype", Value: objtype.ScriptVarTypeAutoInt},
			{Key: "outputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "val", Value: "ignored,555"},
		},
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packEnumConfigs: %v", err)
	}
	// AUTOINT inputtype → key = p4(0). outputtype INT (not AUTOINT) → value = p4(555).
	want := []byte{
		0x00, 0x01,
		0x01, 105, // collapsed AUTOINT→INT
		0x02, 105,
		0x06,
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, // p4(loop index 0)
		0x00, 0x00, 0x02, 0x2b, // p4(555)
		0xfa, 'e', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackEnumConfigs_MissingInputType_Errors(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "e"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"e": {
			// inputtype missing
			{Key: "outputtype", Value: objtype.ScriptVarTypeInt},
		},
	}
	_, err := packEnumConfigs(cfgs, pf, lk)
	if err == nil || !strings.Contains(err.Error(), "inputtype") {
		t.Fatalf("missing inputtype: err=%v", err)
	}
}
```

- [ ] **Step 3.2.2: Run — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackEnumConfigs -v
```

Expected: compile failure (packer undefined).

- [ ] **Step 3.2.3: Implement packer (append to `pkg/pack/enum.go`)**

```go
import (
	"strings"
)

// packEnumConfigs walks every id, emits the enum body per
// EnumConfig.ts:60-156. Pre-scans for inputtype/outputtype because
// the val list trailer's opcode (5 vs 6) and the per-val emission
// shape depend on outputtype, and the val-key emission depends on
// inputtype. Returns an error when either is missing (TS '!' non-null
// assert ported to explicit error).
//
// AUTOINT collapse: TS writes INT byte when inputtype is AUTOINT.
// AUTOINT inputtype: val key is p4(loopIndex), no key-half lookup.
// AUTOINT outputtype: value = lookupParamValue(outputtype, valStr)
// over the WHOLE string (no comma split).
//
// TS source: tools/pack/config/EnumConfig.ts:60-156.
func packEnumConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			var (
				inputtype  objtype.ScriptVarType
				outputtype objtype.ScriptVarType
				gotIn      bool
				gotOut     bool
				vals       []string
			)
			for _, line := range cfg {
				switch line.Key {
				case "inputtype":
					inputtype = line.Value.(objtype.ScriptVarType)
					gotIn = true
				case "outputtype":
					outputtype = line.Value.(objtype.ScriptVarType)
					gotOut = true
				}
			}
			if !gotIn {
				return nil, fmt.Errorf("%s: missing inputtype", name)
			}
			if !gotOut {
				return nil, fmt.Errorf("%s: missing outputtype", name)
			}

			for _, line := range cfg {
				switch line.Key {
				case "inputtype":
					pd.P1(1)
					if inputtype == objtype.ScriptVarTypeAutoInt {
						pd.P1(uint8(objtype.ScriptVarTypeInt))
					} else {
						pd.P1(uint8(inputtype))
					}
				case "outputtype":
					pd.P1(2)
					pd.P1(uint8(outputtype))
				case "default":
					rawDefault := line.Value.(string)
					if outputtype == objtype.ScriptVarTypeString {
						pd.P1(3)
						v, err := lookupParamValue(outputtype, rawDefault, lk)
						if err != nil {
							return nil, fmt.Errorf("%s: default: %w", name, err)
						}
						pd.PJStr(v.(string))
					} else {
						pd.P1(4)
						v, err := lookupParamValue(outputtype, rawDefault, lk)
						if err != nil {
							return nil, fmt.Errorf("%s: default: %w", name, err)
						}
						pd.P4(uint32(v.(int)))
					}
				case "val":
					vals = append(vals, line.Value.(string))
				}
			}

			if outputtype == objtype.ScriptVarTypeString {
				pd.P1(5)
			} else {
				pd.P1(6)
			}
			pd.P2(uint16(len(vals)))
			for i, raw := range vals {
				// key half
				if inputtype == objtype.ScriptVarTypeAutoInt {
					pd.P4(uint32(i))
				} else {
					comma := strings.Index(raw, ",")
					if comma < 0 {
						return nil, fmt.Errorf("%s: val missing comma: %s", name, raw)
					}
					keyPart := raw[:comma]
					v, err := lookupParamValue(inputtype, keyPart, lk)
					if err != nil {
						return nil, fmt.Errorf("%s: val key %q: %w", name, raw, err)
					}
					pd.P4(uint32(v.(int)))
				}
				// value half
				if outputtype == objtype.ScriptVarTypeAutoInt {
					v, err := lookupParamValue(outputtype, raw, lk)
					if err != nil {
						return nil, fmt.Errorf("%s: val whole %q: %w", name, raw, err)
					}
					pd.P4(uint32(v.(int)))
				} else {
					comma := strings.Index(raw, ",")
					if comma < 0 {
						return nil, fmt.Errorf("%s: val missing comma: %s", name, raw)
					}
					valuePart := raw[comma+1:]
					v, err := lookupParamValue(outputtype, valuePart, lk)
					if err != nil {
						return nil, fmt.Errorf("%s: val value %q: %w", name, raw, err)
					}
					if outputtype == objtype.ScriptVarTypeString {
						pd.PJStr(v.(string))
					} else {
						pd.P4(uint32(v.(int)))
					}
				}
			}
		}
		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}
	return pd, nil
}
```

- [ ] **Step 3.2.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackEnumConfigs -v
```

Expected: all 4 named packer tests PASS (`IntOutputType_DefaultAndOneVal`, `StringOutputType_StringDefaultAndVal`, `AutoIntInputType_RequiresCommaInVal`, `MissingInputType_Errors`).

- [ ] **Step 3.2.5: Commit**

```bash
git add pkg/pack/enum.go pkg/pack/enum_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-195 T3 — .enum parser + packer

parseEnumConfig accepts inputtype/outputtype (→ScriptVarType) and
default/val (raw strings). packEnumConfigs pre-scans for both type
keys (errors on missing), emits opcodes 1/2/3/4 during walk, then
opcode 5 (STRING) or 6 (else) trailer with p2(val.length) and per-val
resolved key/value via lookupParamValue.

AUTOINT inputtype collapses to INT byte. AUTOINT inputtype val key
= p4(loopIndex). AUTOINT outputtype value = whole-string resolution.

NAI-192-D-DEADBRANCH-OMITTED applied.

TS source: tools/pack/config/EnumConfig.ts:1-157.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — `.inv` parser + packer

**Files:**
- Create: `pkg/pack/inv.go`
- Create: `pkg/pack/inv_test.go`

**TS source:** `tools/pack/config/InvConfig.ts:1-197`.

### 4.1 — Parser

- [ ] **Step 4.1.1: Write failing parser tests (`pkg/pack/inv_test.go`)**

```go
package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseInvConfig_Size(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	v, ok, err := parse("size", "28")
	if err != nil || !ok || v.(int) != 28 {
		t.Fatalf("size: ok=%v err=%v v=%v", ok, err, v)
	}
}

func TestParseInvConfig_SizeOutOfRange(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	if _, _, err := parse("size", "-1"); err == nil {
		t.Errorf("size=-1: want error")
	}
	if _, _, err := parse("size", "70000"); err == nil {
		t.Errorf("size=70000: want error")
	}
}

func TestParseInvConfig_Scope(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	for in, want := range map[string]int{
		"shared": objtype.InvTypeScopeShared,
		"perm":   objtype.InvTypeScopePerm,
		"temp":   objtype.InvTypeScopeTemp,
	} {
		v, ok, err := parse("scope", in)
		if err != nil || !ok || v.(int) != want {
			t.Errorf("scope=%q: ok=%v err=%v v=%v want %d", in, ok, err, v, want)
		}
	}
	if _, _, err := parse("scope", "bad"); err == nil {
		t.Errorf("scope=bad: want error")
	}
}

func TestParseInvConfig_Booleans(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	for _, key := range []string{"stackall", "restock", "allstock", "protect", "runweight", "dummyinv"} {
		v, ok, err := parse(key, "yes")
		if err != nil || !ok || v.(bool) != true {
			t.Errorf("%s=yes: ok=%v err=%v v=%v", key, ok, err, v)
		}
		v, ok, err = parse(key, "no")
		if err != nil || !ok || v.(bool) != false {
			t.Errorf("%s=no: ok=%v err=%v v=%v", key, ok, err, v)
		}
	}
}

func TestParseInvConfig_Stock_WithoutRespawn(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg", 1: "bone"})
	parse := parseInvConfigFor(objPF)

	v, ok, err := parse("stock1", "bone,5")
	if err != nil || !ok {
		t.Fatalf("stock1: ok=%v err=%v", ok, err)
	}
	parts := v.([]int)
	if len(parts) != 2 || parts[0] != 1 || parts[1] != 5 {
		t.Fatalf("stock1: got %v, want [1, 5]", parts)
	}
}

func TestParseInvConfig_Stock_WithRespawn(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg", 1: "bone"})
	parse := parseInvConfigFor(objPF)

	v, ok, err := parse("stock2", "egg,3,100")
	if err != nil || !ok {
		t.Fatalf("stock2: ok=%v err=%v", ok, err)
	}
	parts := v.([]int)
	if len(parts) != 3 || parts[0] != 0 || parts[1] != 3 || parts[2] != 100 {
		t.Fatalf("stock2: got %v, want [0, 3, 100]", parts)
	}
}

func TestParseInvConfig_Stock_UnknownObj(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	_, ok, err := parse("stock1", "ghost,2")
	if !ok {
		t.Fatalf("stock1: ok=false, want true (recognized key)")
	}
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("stock1 unknown obj: err=%v", err)
	}
}

func TestParseInvConfig_UnknownKey(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	_, ok, err := parse("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 4.1.2: Run — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseInvConfig -v
```

Expected: compile failure (`parseInvConfigFor` undefined).

- [ ] **Step 4.1.3: Implement parser (`pkg/pack/inv.go`)**

```go
package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseInvConfigFor returns the per-key=value parser for .inv config
// blocks. Returns a closure capturing the obj name-map for stockN
// resolution.
//
// Accepted keys:
//   - size       (number, bounded [0, 65535])
//   - scope      ("shared"|"perm"|"temp" → InvTypeScope*)
//   - stackall, restock, allstock, protect, runweight, dummyinv  (boolean)
//   - stockN     ("objName,count[,respawn]" → []int{objId, count[, respawn]})
//
// TS source: tools/pack/config/InvConfig.ts:5-92.
func parseInvConfigFor(objPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		switch key {
		case "size":
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid size: %s", value)
			}
			if n < 0 || n > 65535 {
				return nil, true, fmt.Errorf("size out of range [0, 65535]: %d", n)
			}
			return int(n), true, nil
		case "scope":
			switch value {
			case "shared":
				return objtype.InvTypeScopeShared, true, nil
			case "perm":
				return objtype.InvTypeScopePerm, true, nil
			case "temp":
				return objtype.InvTypeScopeTemp, true, nil
			}
			return nil, true, fmt.Errorf("invalid scope: %s", value)
		case "stackall", "restock", "allstock", "protect", "runweight", "dummyinv":
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean: %s", value)
			}
			return GetConfigBoolean(value), true, nil
		}
		if strings.HasPrefix(key, "stock") {
			parts := strings.Split(value, ",")
			if len(parts) < 2 {
				return nil, true, fmt.Errorf("stockN expects 'obj,count[,respawn]': %s", value)
			}
			objIdx := objPack.GetByName(parts[0])
			if objIdx == -1 {
				return nil, true, fmt.Errorf("unknown obj: %s", parts[0])
			}
			count, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, true, fmt.Errorf("invalid stock count: %s", parts[1])
			}
			if len(parts) == 2 {
				return []int{objIdx, count}, true, nil
			}
			respawn, err := strconv.Atoi(parts[2])
			if err != nil {
				return nil, true, fmt.Errorf("invalid stock respawn: %s", parts[2])
			}
			return []int{objIdx, count, respawn}, true, nil
		}
		return nil, false, nil
	}
}
```

- [ ] **Step 4.1.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseInvConfig -v
```

Expected: 8 parser tests PASS.

### 4.2 — Packer

- [ ] **Step 4.2.1: Append packer tests to `pkg/pack/inv_test.go`**

```go
func TestPackInvConfigs_AllOpcodes(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "bank"})
	cfgs := map[string][]ConfigLine{
		"bank": {
			{Key: "scope", Value: objtype.InvTypeScopeShared}, // op1, p1(2)
			{Key: "size", Value: 28},                          // op2, p2(28)
			{Key: "stackall", Value: true},                    // op3
			{Key: "restock", Value: true},                     // op5
			{Key: "allstock", Value: true},                    // op6
			{Key: "protect", Value: false},                    // op7 (only fires on false)
			{Key: "runweight", Value: true},                   // op8
			{Key: "dummyinv", Value: true},                    // op9
		},
	}
	pd, err := packInvConfigs(cfgs, pf)
	if err != nil {
		t.Fatalf("packInvConfigs: %v", err)
	}
	want := []byte{
		0x00, 0x01, // size=1
		0x01, 0x02, // scope shared (2)
		0x02, 0x00, 0x1c, // size=28
		0x03, // stackall
		0x05, // restock
		0x06, // allstock
		0x07, // protect=false
		0x08, // runweight
		0x09, // dummyinv
		0xfa, 'b','a','n','k', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackInvConfigs_ProtectTrueDoesNotEmit(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {{Key: "protect", Value: true}}, // op7 should NOT fire
	}
	pd, err := packInvConfigs(cfgs, pf)
	if err != nil {
		t.Fatalf("%v", err)
	}
	want := []byte{0x00, 0x01, 0xfa, 'i', 0x0a, 0x00}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackInvConfigs_StockListWithHoles(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "shop"})
	cfgs := map[string][]ConfigLine{
		"shop": {
			{Key: "size", Value: 3},
			{Key: "stock1", Value: []int{42, 10, 200}}, // index 0 → present
			// stock2 absent → hole
			{Key: "stock3", Value: []int{99, 5}},        // index 2 → present, no respawn
		},
	}
	pd, err := packInvConfigs(cfgs, pf)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Order of opcodes during walk: size(op2), stock collection (no immediate emission),
	// then stock-trailer (op4) at end-of-config (TS L162-184).
	want := []byte{
		0x00, 0x01,
		0x02, 0x00, 0x03, // size=3
		0x04,                              // op4 stock trailer
		0x03,                              // p1(stock.length=3)
		0x00, 0x2a, 0x00, 0x0a, 0x00, 0x00, 0x00, 0xc8, // slot 0: 42, 10, 200
		0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // slot 1: hole (-1, 0, 0)
		0x00, 0x63, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, // slot 2: 99, 5, 0
		0xfa, 's','h','o','p', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackInvConfigs_DuplicateStockErrors(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {
			{Key: "size", Value: 2},
			{Key: "stock1", Value: []int{0, 1}},
			{Key: "stock1", Value: []int{0, 2}},
		},
	}
	_, err := packInvConfigs(cfgs, pf)
	if err == nil || !strings.Contains(err.Error(), "stock1") {
		t.Fatalf("duplicate stock: err=%v", err)
	}
}

func TestPackInvConfigs_StockBeyondSizeErrors(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {
			{Key: "size", Value: 1},
			{Key: "stock2", Value: []int{0, 1}}, // index 1 >= size 1
		},
	}
	_, err := packInvConfigs(cfgs, pf)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("stock beyond size: err=%v", err)
	}
}
```

- [ ] **Step 4.2.2: Run — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackInvConfigs -v
```

Expected: `packInvConfigs` undefined.

- [ ] **Step 4.2.3: Implement packer (append to `pkg/pack/inv.go`)**

```go
// packInvConfigs walks every id, pre-finds size, then walks config
// lines emitting opcodes 1/2/3/5/6/7/8/9 inline. Stock entries are
// collected into a sparse []*[]int slot map by stockN index, then the
// stock-list trailer (opcode 4) is emitted at end of config.
//
// Error paths (TS packStepError analogue):
//   - duplicate stockN line     → "%s: duplicate stockN"
//   - stockN index >= size      → "%s: stockN exceeds size"
//
// TS source: tools/pack/config/InvConfig.ts:94-197.
func packInvConfigs(configs map[string][]ConfigLine, pf *PackFile) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			size := 0
			for _, line := range cfg {
				if line.Key == "size" {
					size = line.Value.(int)
				}
			}

			var stock [][]int
			for _, line := range cfg {
				switch {
				case line.Key == "scope":
					pd.P1(1)
					pd.P1(uint8(line.Value.(int)))
				case line.Key == "size":
					pd.P1(2)
					pd.P2(uint16(line.Value.(int)))
				case strings.HasPrefix(line.Key, "stock"):
					n, err := strconv.Atoi(line.Key[5:])
					if err != nil {
						return nil, fmt.Errorf("%s: invalid stock key: %s", name, line.Key)
					}
					index := n - 1
					if index >= size {
						return nil, fmt.Errorf("%s: stock%d exceeds size=%d", name, n, size)
					}
					for len(stock) <= index {
						stock = append(stock, nil)
					}
					if stock[index] != nil {
						return nil, fmt.Errorf("%s: duplicate stock%d", name, n)
					}
					stock[index] = line.Value.([]int)
				case line.Key == "stackall":
					if line.Value.(bool) {
						pd.P1(3)
					}
				case line.Key == "restock":
					if line.Value.(bool) {
						pd.P1(5)
					}
				case line.Key == "allstock":
					if line.Value.(bool) {
						pd.P1(6)
					}
				case line.Key == "protect":
					if !line.Value.(bool) {
						pd.P1(7)
					}
				case line.Key == "runweight":
					if line.Value.(bool) {
						pd.P1(8)
					}
				case line.Key == "dummyinv":
					if line.Value.(bool) {
						pd.P1(9)
					}
				}
			}

			if len(stock) > 0 {
				pd.P1(4)
				pd.P1(uint8(len(stock)))
				for _, slot := range stock {
					if slot == nil {
						pd.P2(uint16(0xffff)) // -1 as uint16
						pd.P2(0)
						pd.P4(0)
						continue
					}
					pd.P2(uint16(slot[0]))
					pd.P2(uint16(slot[1]))
					if len(slot) == 3 {
						pd.P4(uint32(slot[2]))
					} else {
						pd.P4(0)
					}
				}
			}
		}
		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}
	return pd, nil
}
```

- [ ] **Step 4.2.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackInvConfigs -v
```

Expected: 5 packer tests PASS.

- [ ] **Step 4.2.5: Commit**

```bash
git add pkg/pack/inv.go pkg/pack/inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-195 T4 — .inv parser + packer

parseInvConfigFor(objPack) returns per-key parser: size [0,65535],
scope shared/perm/temp via objtype.InvTypeScope* constants, 6 booleans,
stockN as []int{objId, count[, respawn]} via objPack.GetByName.

packInvConfigs emits opcodes 1/2/3/5/6/7/8/9 inline; collects stock
entries by index; emits opcode 4 stock list with -1 holes at
end-of-config. Errors on duplicate stockN and stockN >= size.

TS source: tools/pack/config/InvConfig.ts:1-197.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — `.struct` parser + packer

**Files:**
- Create: `pkg/pack/struct.go`
- Create: `pkg/pack/struct_test.go`

**TS source:** `tools/pack/config/StructConfig.ts:1-117`.

### 5.1 — Parser

- [ ] **Step 5.1.1: Write failing parser tests (`pkg/pack/struct_test.go`)**

```go
package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func newStructParamTypes() *objtype.ParamTypeConfigs {
	intParam := &objtype.ParamType{Type: objtype.ScriptVarTypeInt, DefaultInt: 0}
	intParam.ID = 5
	intParam.DebugName = "myint"
	strParam := &objtype.ParamType{Type: objtype.ScriptVarTypeString, DefaultString: ""}
	strParam.ID = 6
	strParam.DebugName = "mystr"
	return &objtype.ParamTypeConfigs{
		ConfigNames: map[string]int{"myint": 5, "mystr": 6},
		Configs:     []*objtype.ParamType{nil, nil, nil, nil, nil, intParam, strParam},
	}
}

func TestParseStructConfig_IntParam(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	v, ok, err := parse("param", "myint,42")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	pv := v.(ParamValue)
	if pv.ID != 5 || pv.Type != objtype.ScriptVarTypeInt || pv.Value.(int) != 42 {
		t.Fatalf("got %+v, want {5, Int, 42}", pv)
	}
}

func TestParseStructConfig_StringParam(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	v, ok, err := parse("param", "mystr,hello world")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	pv := v.(ParamValue)
	if pv.ID != 6 || pv.Type != objtype.ScriptVarTypeString || pv.Value.(string) != "hello world" {
		t.Fatalf("got %+v", pv)
	}
}

func TestParseStructConfig_UnknownParam(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	_, ok, err := parse("param", "doesnotexist,1")
	if !ok {
		t.Fatalf("ok=false, want true (recognized key)")
	}
	if err == nil || !strings.Contains(err.Error(), "doesnotexist") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseStructConfig_UnknownKey(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	_, ok, err := parse("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 5.1.2: Run — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseStructConfig -v
```

Expected: `parseStructConfigFor` undefined.

- [ ] **Step 5.1.3: Implement parser (`pkg/pack/struct.go`)**

```go
package pack

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseStructConfigFor returns the per-key=value parser for .struct
// config blocks. Only `param=name,value` is accepted. Param names are
// resolved against the runtime ParamType registry (loaded between
// .param save and .struct parse in PackConfigs); values are resolved
// via lookupParamValue using the param's typed code.
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseStructConfig declares empty
// stringKeys/numberKeys/booleanKeys arrays — dead branches preserved
// by the TS author. Goscape omits the empty branches.
//
// TS source: tools/pack/config/StructConfig.ts:7-67.
func parseStructConfigFor(paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if key != "param" {
			return nil, false, nil
		}
		comma := strings.Index(value, ",")
		if comma < 0 {
			return nil, true, fmt.Errorf("param expects 'name,value': %s", value)
		}
		name := value[:comma]
		raw := value[comma+1:]
		id, ok := paramTypes.ConfigNames[name]
		if !ok {
			return nil, true, fmt.Errorf("unknown param: %s", name)
		}
		pt := paramTypes.Configs[id]
		resolved, err := lookupParamValue(pt.Type, raw, lk)
		if err != nil {
			return nil, true, fmt.Errorf("param %s value: %w", name, err)
		}
		return ParamValue{ID: id, Type: pt.Type, Value: resolved}, true, nil
	}
}
```

- [ ] **Step 5.1.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseStructConfig -v
```

Expected: 4 parser tests PASS.

### 5.2 — Packer

- [ ] **Step 5.2.1: Append packer tests to `pkg/pack/struct_test.go`**

```go
func TestPackStructConfigs_IntParam(t *testing.T) {
	pf := newTestPF("struct", map[int]string{0: "s"})
	cfgs := map[string][]ConfigLine{
		"s": {
			{Key: "param", Value: ParamValue{ID: 5, Type: objtype.ScriptVarTypeInt, Value: 42}},
		},
	}
	pd := packStructConfigs(cfgs, pf)
	want := []byte{
		0x00, 0x01,
		0xf9,                                 // op249
		0x01,                                 // p1(param count=1)
		0x00, 0x00, 0x05,                     // p3(id=5)
		0x00,                                 // pbool(false) — not STRING
		0x00, 0x00, 0x00, 0x2a,               // p4(42)
		0xfa, 's', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackStructConfigs_StringParam(t *testing.T) {
	pf := newTestPF("struct", map[int]string{0: "s"})
	cfgs := map[string][]ConfigLine{
		"s": {
			{Key: "param", Value: ParamValue{ID: 6, Type: objtype.ScriptVarTypeString, Value: "hi"}},
		},
	}
	pd := packStructConfigs(cfgs, pf)
	want := []byte{
		0x00, 0x01,
		0xf9,
		0x01,
		0x00, 0x00, 0x06, // p3(6)
		0x01,             // pbool(true)
		'h', 'i', 0x0a,   // pjstr("hi")
		0xfa, 's', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackStructConfigs_EmptyParamList_NoOp249(t *testing.T) {
	pf := newTestPF("struct", map[int]string{0: "s"})
	cfgs := map[string][]ConfigLine{
		"s": {},  // present block but no params
	}
	pd := packStructConfigs(cfgs, pf)
	// No op249 emitted (TS L89 `if (params.length > 0)`). Just 250-trailer + Next.
	want := []byte{
		0x00, 0x01,
		0xfa, 's', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}
```

- [ ] **Step 5.2.2: Run — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackStructConfigs -v
```

Expected: `packStructConfigs` undefined.

- [ ] **Step 5.2.3: Implement packer (append to `pkg/pack/struct.go`)**

```go
// packStructConfigs walks every id, collects all `param=` lines, and
// emits opcode 249 + p1(count) + per-param p3(id) + pbool(isString)
// + pjstr(value)|p4(value) when at least one param is present. 250
// trailer + Next() always.
//
// TS source: tools/pack/config/StructConfig.ts:70-117.
func packStructConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			var params []ParamValue
			for _, line := range cfg {
				if line.Key == "param" {
					params = append(params, line.Value.(ParamValue))
				}
			}
			if len(params) > 0 {
				pd.P1(249)
				pd.P1(uint8(len(params)))
				for _, p := range params {
					pd.P3(uint32(p.ID))
					isString := p.Type == objtype.ScriptVarTypeString
					pd.PBool(isString)
					if isString {
						pd.PJStr(p.Value.(string))
					} else {
						pd.P4(uint32(p.Value.(int)))
					}
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

- [ ] **Step 5.2.4: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackStructConfigs -v
```

Expected: 3 packer tests PASS.

- [ ] **Step 5.2.5: Commit**

```bash
git add pkg/pack/struct.go pkg/pack/struct_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-195 T5 — .struct parser + packer

parseStructConfigFor(paramTypes, lk) returns per-key parser: only
`param=name,value` accepted; name resolved against runtime
ParamTypeConfigs registry; value resolved via lookupParamValue.

packStructConfigs collects params, emits opcode 249 + p1(count) +
per-param p3(id) + pbool(isString) + pjstr|p4(value) when ≥1 present.

NAI-192-D-DEADBRANCH-OMITTED applied.

TS source: tools/pack/config/StructConfig.ts:1-117.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6 — PackConfigs orchestrator integration

**Files:**
- Modify: `pkg/pack/pack_configs.go`

This task: (a) hoists `lk *paramLookups` to function scope; (b) adds 4 new branches after the `.param` branch in `enum → inv → mesanim → struct` order; (c) introduces `ensureObjPack`/`ensureSeqPack`/`ensureParamTypes` lazy-construction helpers; (d) adds the `packAndSaveEnum/Inv/MesAnim/Struct` helpers; (e) adds the `NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS` doc-comment.

This task does NOT have its own failing test before implementation — the per-config packers are already tested. The integration is exercised in T7 (round-trip) and T8 (8-config integration test). The implementer should run all existing tests after this task to confirm no regression.

### 6.1 — Edit `pkg/pack/pack_configs.go`

- [ ] **Step 6.1.1: Read current `pack_configs.go`** to confirm it matches the pre-flight assumption (`lk` declared inside `.param` block).

- [ ] **Step 6.1.2: Replace the existing `PackConfigs` function body**

Old function structure (verbatim — current HEAD `db0abfa`):

```
func PackConfigs(srcDir, outDir string) error {
    constants, err := LoadConstants(srcDir)
    ...
    if GetLatestModified(scriptsDir, ".param") > 0 && ShouldBuild(...) {
        paramPack, err := NewPackFile(srcDir, "param", nil)
        ...
        lk, err := loadParamLookups(srcDir, varpPack)
        ...
        if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
            return err
        }
        clientJagDirty = true
    }

    if clientJagDirty {
        if err := clientJag.Save(...); err != nil { ... }
    }
    return nil
}
```

Replacement structure: **hoist `lk` to function scope** (declared as `var lk *paramLookups` before the var-domain pack files) and append 4 new branches between the `.param` branch and the `clientJagDirty` save block. Add `objPack`, `seqPack`, `paramTypes` declarations + ensure-helpers at the same scope.

- [ ] **Step 6.1.3: Replacement insertion (apply via Edit)**

Find this exact block (current):

```go
	clientJagDirty := false

	if GetLatestModified(scriptsDir, ".varp") > 0 &&
```

Replace with:

```go
	clientJagDirty := false

	// Lazy lookups reused across .enum/.inv/.mesanim/.struct branches.
	var (
		lk         *paramLookups
		objPack    *PackFile
		seqPack    *PackFile
		paramTypes *objtype.ParamTypeConfigs
	)
	ensureLk := func() error {
		if lk != nil {
			return nil
		}
		newLk, err := loadParamLookups(srcDir, varpPack)
		if err != nil {
			return err
		}
		lk = newLk
		return nil
	}
	ensureObjPack := func() error {
		if objPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "obj", nil)
		if err != nil {
			return err
		}
		objPack = pf
		return nil
	}
	ensureSeqPack := func() error {
		if seqPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "seq", nil)
		if err != nil {
			return err
		}
		seqPack = pf
		return nil
	}
	ensureParamTypes := func() error {
		if paramTypes != nil {
			return nil
		}
		pt, err := objtype.LoadParamTypes(serverOut)
		if err != nil {
			return fmt.Errorf("load param types: %w", err)
		}
		paramTypes = pt
		return nil
	}

	if GetLatestModified(scriptsDir, ".varp") > 0 &&
```

Add the import for `"github.com/zsrv/goscape/pkg/objtype"` to the file's import block if not already present.

- [ ] **Step 6.1.4: Replace the existing `.param` branch** so its `lk` assignment is via `ensureLk` (so `lk` survives for later branches)

Find:

```go
	// NAI-194-D-PARAM-AFTER-VARS: see PackConfigs doc-comment.
	if GetLatestModified(scriptsDir, ".param") > 0 &&
		ShouldBuild(scriptsDir, ".param", filepath.Join(serverOut, "param.dat")) {
		paramPack, err := NewPackFile(srcDir, "param", nil)
		if err != nil {
			return err
		}
		lk, err := loadParamLookups(srcDir, varpPack)
		if err != nil {
			return err
		}
		if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
			return err
		}
		clientJagDirty = true
	}
```

Replace with:

```go
	// NAI-194-D-PARAM-AFTER-VARS: see PackConfigs doc-comment.
	if GetLatestModified(scriptsDir, ".param") > 0 &&
		ShouldBuild(scriptsDir, ".param", filepath.Join(serverOut, "param.dat")) {
		paramPack, err := NewPackFile(srcDir, "param", nil)
		if err != nil {
			return err
		}
		if err := ensureLk(); err != nil {
			return err
		}
		if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
			return err
		}
		clientJagDirty = true
	}
```

- [ ] **Step 6.1.5: Append 4 new branches** (insert before the `if clientJagDirty {` block)

```go
	// NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS: see PackConfigs doc-comment.
	if GetLatestModified(scriptsDir, ".enum") > 0 &&
		ShouldBuild(scriptsDir, ".enum", filepath.Join(serverOut, "enum.dat")) {
		if err := ensureLk(); err != nil {
			return err
		}
		enumPack, err := NewPackFile(srcDir, "enum", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveEnum(srcDir, serverOut, enumPack, lk, constants); err != nil {
			return err
		}
	}

	if GetLatestModified(scriptsDir, ".inv") > 0 &&
		ShouldBuild(scriptsDir, ".inv", filepath.Join(serverOut, "inv.dat")) {
		if err := ensureObjPack(); err != nil {
			return err
		}
		invPack, err := NewPackFile(srcDir, "inv", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveInv(srcDir, serverOut, invPack, objPack, constants); err != nil {
			return err
		}
	}

	if GetLatestModified(scriptsDir, ".mesanim") > 0 &&
		ShouldBuild(scriptsDir, ".mesanim", filepath.Join(serverOut, "mesanim.dat")) {
		if err := ensureSeqPack(); err != nil {
			return err
		}
		mesPack, err := NewPackFile(srcDir, "mesanim", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveMesAnim(srcDir, serverOut, mesPack, seqPack, constants); err != nil {
			return err
		}
	}

	if GetLatestModified(scriptsDir, ".struct") > 0 &&
		ShouldBuild(scriptsDir, ".struct", filepath.Join(serverOut, "struct.dat")) {
		if err := ensureParamTypes(); err != nil {
			return err
		}
		if err := ensureLk(); err != nil {
			return err
		}
		structPack, err := NewPackFile(srcDir, "struct", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveStruct(srcDir, serverOut, structPack, paramTypes, lk, constants); err != nil {
			return err
		}
	}
```

- [ ] **Step 6.1.6: Append `packAndSave*` helpers at end of `pkg/pack/pack_configs.go`**

```go
func packAndSaveEnum(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".enum", nil, parseEnumConfig, c)
	if err != nil {
		return err
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "enum.dat"), filepath.Join(serverOut, "enum.idx"))
}

func packAndSaveInv(srcDir, serverOut string, pf, objPack *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".inv", nil, parseInvConfigFor(objPack), c)
	if err != nil {
		return err
	}
	pd, err := packInvConfigs(cfgs, pf)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "inv.dat"), filepath.Join(serverOut, "inv.idx"))
}

func packAndSaveMesAnim(srcDir, serverOut string, pf, seqPack *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".mesanim", nil, parseMesAnimConfigFor(seqPack), c)
	if err != nil {
		return err
	}
	pd := packMesAnimConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "mesanim.dat"), filepath.Join(serverOut, "mesanim.idx"))
}

func packAndSaveStruct(srcDir, serverOut string, pf *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".struct", nil, parseStructConfigFor(paramTypes, lk), c)
	if err != nil {
		return err
	}
	pd := packStructConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "struct.dat"), filepath.Join(serverOut, "struct.idx"))
}
```

- [ ] **Step 6.1.7: Update the `PackConfigs` doc-comment** to add the new deviation paragraph

Find:

```go
// NAI-194-D-PARAM-AFTER-VARS: TS processes .param FIRST (before jag
// creation and before .varp/.varn/.vars) so other configs can resolve
// param defaults during packing — see PackShared.ts:315 "We have to
// pack params for other configs to parse correctly". Goscape runs
// .param after the var-domain trio because NAI-194 introduces .param
// in isolation (no other config packer yet exists that would consume
// param outputs). NAI-195+ may need to re-evaluate ordering when
// .loc/.obj/.npc packers land.
//
// TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).
```

Append (immediately before the `TS source:` line):

```go
// NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS: TS interleaves
// .enum/.inv/.mesanim/.struct before .varp (PackShared.ts:417/425/
// 434/443); goscape places all four AFTER .param (which is itself
// after .varp via PARAM-AFTER-VARS). Retire together with
// PARAM-AFTER-VARS when .loc/.obj/.npc force a full ordering rewrite.
//
// Between .param save and .struct parse, goscape calls
// objtype.LoadParamTypes(serverOut) to populate a runtime registry
// consumed by parseStructConfig — direct port of TS
// PackShared.ts:334 (ParamType.load). Lazy-loaded on first .struct
// consumer when .param did not rebuild this run.
//
```

- [ ] **Step 6.1.8: Run all tests — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...
```

Expected: all existing tests still PASS (no new tests added in this task; T7/T8 will exercise the integration).

- [ ] **Step 6.1.9: Commit**

```bash
git add pkg/pack/pack_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-195 T6 — PackConfigs .enum/.inv/.mesanim/.struct branches

Adds 4 server-only per-config branches after .param. Hoists
lk *paramLookups to function scope; lazy-builds objPack/seqPack/
paramTypes via ensure-helpers. ParamType runtime registry load
(objtype.LoadParamTypes(serverOut)) between .param save and .struct
parse — direct port of TS PackShared.ts:334.

New deviation: NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS —
TS orders these 4 before .varp; goscape lands them after .param.
Retire together with PARAM-AFTER-VARS when .loc/.obj/.npc force
full reorder.

TS source: tools/pack/config/PackShared.ts:417/425/434/443.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7 — Per-config round-trip tests

**Files:**
- Create: `pkg/pack/enum_roundtrip_test.go`
- Create: `pkg/pack/inv_roundtrip_test.go`
- Create: `pkg/pack/mesanim_roundtrip_test.go`
- Create: `pkg/pack/struct_roundtrip_test.go`

Pattern: write source files into `t.TempDir()`, run `PackConfigs(srcDir, outDir)`, then load with the corresponding `objtype.Load<Type>Types(outDir)` and assert source-declared fields survive.

### 7.1 — `.mesanim` round-trip

- [ ] **Step 7.1.1: Create `pkg/pack/mesanim_roundtrip_test.go`**

```go
package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_MesanimRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	// seq.pack: id→debugname mapping needed by .mesanim parser
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n1=walk\n2=death\n")
	writeFile(t, filepath.Join(srcDir, "pack", "mesanim.pack"), "0=hero_chat\n")
	writeFile(t, filepath.Join(scripts, "test.mesanim"),
		"[hero_chat]\nlen0=walk\nlen2=death\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	cfgs, err := objtype.LoadMesanimTypes(outDir)
	if err != nil {
		t.Fatalf("LoadMesanimTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["hero_chat"]
	if !ok {
		t.Fatalf("hero_chat not found in ConfigNames")
	}
	m := cfgs.Configs[id]
	// len0 → Len[0] = walk id (1); len2 → Len[1] = death id (2); others stay at -1
	if m.Len[0] != 1 {
		t.Errorf("Len[0] = %d, want 1", m.Len[0])
	}
	if m.Len[1] != 2 {
		t.Errorf("Len[1] = %d, want 2", m.Len[1])
	}
}
```

- [ ] **Step 7.1.2: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs_MesanimRoundTrip -v
```

Expected: PASS.

### 7.2 — `.enum` round-trip

- [ ] **Step 7.2.1: Create `pkg/pack/enum_roundtrip_test.go`**

```go
package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_EnumRoundTrip_IntInt(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "0=days_per_month\n")
	// Empty .pack files for the other 12 paramLookups slots so loadParamLookups succeeds.
	for _, p := range []string{"obj", "loc", "interface", "struct", "category", "spotanim", "npc", "inv", "synth", "seq", "varp", "dbrow", "param"} {
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}
	writeFile(t, filepath.Join(scripts, "test.enum"),
		"[days_per_month]\n"+
			"inputtype=int\n"+
			"outputtype=int\n"+
			"default=30\n"+
			"val=1,31\n"+
			"val=2,28\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	cfgs, err := objtype.LoadEnumTypes(outDir)
	if err != nil {
		t.Fatalf("LoadEnumTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["days_per_month"]
	if !ok {
		t.Fatalf("days_per_month not found")
	}
	e := cfgs.Configs[id]
	if e.InputType != objtype.ScriptVarTypeInt || e.OutputType != objtype.ScriptVarTypeInt {
		t.Errorf("types: in=%v out=%v", e.InputType, e.OutputType)
	}
	if e.DefaultInt != 30 {
		t.Errorf("DefaultInt = %d, want 30", e.DefaultInt)
	}
	if v, ok := e.Values[1]; !ok || v.(int32) != 31 {
		t.Errorf("Values[1] = %v, want 31", v)
	}
	if v, ok := e.Values[2]; !ok || v.(int32) != 28 {
		t.Errorf("Values[2] = %v, want 28", v)
	}
}
```

- [ ] **Step 7.2.2: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs_EnumRoundTrip -v
```

Expected: PASS.

### 7.3 — `.inv` round-trip

- [ ] **Step 7.3.1: Create `pkg/pack/inv_roundtrip_test.go`**

```go
package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_InvRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "inv.pack"), "0=bank\n")
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=egg\n1=bone\n")
	writeFile(t, filepath.Join(scripts, "test.inv"),
		"[bank]\n"+
			"scope=shared\n"+
			"size=2\n"+
			"stackall=yes\n"+
			"stock1=egg,5,100\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	cfgs, err := objtype.LoadInvTypes(outDir)
	if err != nil {
		t.Fatalf("LoadInvTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["bank"]
	if !ok {
		t.Fatalf("bank not found")
	}
	inv := cfgs.Configs[id]
	if inv.Scope != objtype.InvTypeScopeShared {
		t.Errorf("Scope = %d, want %d", inv.Scope, objtype.InvTypeScopeShared)
	}
	if inv.Size != 2 {
		t.Errorf("Size = %d, want 2", inv.Size)
	}
	if !inv.StackAll {
		t.Errorf("StackAll = false, want true")
	}
	if len(inv.StockObj) != 1 || inv.StockObj[0] != 0 || inv.StockCount[0] != 5 || inv.StockRate[0] != 100 {
		t.Errorf("stock: obj=%v count=%v rate=%v", inv.StockObj, inv.StockCount, inv.StockRate)
	}
}
```

- [ ] **Step 7.3.2: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs_InvRoundTrip -v
```

Expected: PASS.

### 7.4 — `.struct` round-trip (exercises ParamType runtime load)

- [ ] **Step 7.4.1: Create `pkg/pack/struct_roundtrip_test.go`**

```go
package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_StructRoundTrip_ExercisesParamRuntimeLoad(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"obj", "loc", "interface", "struct", "category", "spotanim", "npc", "inv", "synth", "seq", "varp", "dbrow", "enum"} {
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")
	writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "0=goblin_loot\n")

	writeFile(t, filepath.Join(scripts, "params.param"),
		"[damage]\ntype=int\ndefault=10\n",
	)
	writeFile(t, filepath.Join(scripts, "structs.struct"),
		"[goblin_loot]\nparam=damage,99\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	// Verify .param landed (precondition for ParamType.load).
	if _, err := os.Stat(filepath.Join(outDir, "server", "param.dat")); err != nil {
		t.Fatalf("param.dat missing: %v", err)
	}

	cfgs, err := objtype.LoadStructTypes(outDir)
	if err != nil {
		t.Fatalf("LoadStructTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["goblin_loot"]
	if !ok {
		t.Fatalf("goblin_loot not found")
	}
	s := cfgs.Configs[id]
	if len(s.Params) != 1 {
		t.Fatalf("Params count = %d, want 1", len(s.Params))
	}
	// Param key is `damage` → param id 0. Value is uint32(99) per DecodeParams.
	v, ok := s.Params[0]
	if !ok {
		t.Fatalf("Params[0] missing")
	}
	if v.(uint32) != 99 {
		t.Errorf("Params[0] = %v, want uint32(99)", v)
	}
}
```

- [ ] **Step 7.4.2: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs_StructRoundTrip -v
```

Expected: PASS (validates the ParamType runtime load path end-to-end).

- [ ] **Step 7.4.3: Run all pack tests** — expect PASS

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...
```

- [ ] **Step 7.4.4: Commit**

```bash
git add pkg/pack/mesanim_roundtrip_test.go pkg/pack/enum_roundtrip_test.go pkg/pack/inv_roundtrip_test.go pkg/pack/struct_roundtrip_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-195 T7 — round-trip tests for 4 new configs

Each writes source files into t.TempDir, runs PackConfigs, then loads
via objtype.Load<Type>Types and asserts source fields survive.

struct_roundtrip_test exercises the ParamType runtime load
(objtype.LoadParamTypes between .param save and .struct parse).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8 — Eight-config integration test

**Files:**
- Modify: `pkg/pack/pack_configs_test.go` (append)

- [ ] **Step 8.1: Read existing `pkg/pack/pack_configs_test.go`** to find a `writeFile`-and-`PackConfigs` precedent.

- [ ] **Step 8.2: Append integration test**

```go
func TestPackConfigs_EightConfigsLand(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pack files for all referenced typed-ids
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=quest_points\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npc_state\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=server_clock\n")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")
	writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "0=days\n")
	writeFile(t, filepath.Join(srcDir, "pack", "inv.pack"), "0=bank\n")
	writeFile(t, filepath.Join(srcDir, "pack", "mesanim.pack"), "0=hero_chat\n")
	writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "0=goblin_loot\n")
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=egg\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=walk\n")
	for _, p := range []string{"loc", "interface", "category", "spotanim", "npc", "synth", "dbrow"} {
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}

	writeFile(t, filepath.Join(scripts, "v.varp"),
		"[quest_points]\ntype=int\nscope=perm\n")
	writeFile(t, filepath.Join(scripts, "n.varn"),
		"[npc_state]\ntype=int\n")
	writeFile(t, filepath.Join(scripts, "s.vars"),
		"[server_clock]\ntype=int\n")
	writeFile(t, filepath.Join(scripts, "p.param"),
		"[damage]\ntype=int\ndefault=10\n")
	writeFile(t, filepath.Join(scripts, "e.enum"),
		"[days]\ninputtype=int\noutputtype=int\ndefault=0\nval=1,1\n")
	writeFile(t, filepath.Join(scripts, "i.inv"),
		"[bank]\nscope=shared\nsize=1\n")
	writeFile(t, filepath.Join(scripts, "m.mesanim"),
		"[hero_chat]\nlen1=walk\n")
	writeFile(t, filepath.Join(scripts, "x.struct"),
		"[goblin_loot]\nparam=damage,7\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	// All 8 server-side .dat/.idx pairs landed
	server := filepath.Join(outDir, "server")
	for _, typ := range []string{"varp", "varn", "vars", "param", "enum", "inv", "mesanim", "struct"} {
		if _, err := os.Stat(filepath.Join(server, typ+".dat")); err != nil {
			t.Errorf("%s.dat missing: %v", typ, err)
		}
		if _, err := os.Stat(filepath.Join(server, typ+".idx")); err != nil {
			t.Errorf("%s.idx missing: %v", typ, err)
		}
	}

	// Client jagfile contains only varp + param entries (per NAI-193 / NAI-194)
	if _, err := os.Stat(filepath.Join(outDir, "client", "config")); err != nil {
		t.Errorf("client/config jagfile missing: %v", err)
	}
}
```

If the test file lacks `os`/`filepath` imports, add them.

- [ ] **Step 8.3: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs_EightConfigsLand -v
```

Expected: PASS.

- [ ] **Step 8.4: Commit**

```bash
git add pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-195 T8 — eight-config integration test

TestPackConfigs_EightConfigsLand: write source files for all 8 of
varp/varn/vars/param/enum/inv/mesanim/struct, run PackConfigs, assert
all 16 server .dat/.idx outputs land + client jagfile exists.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9 — Deviation-tag absence pins

**Files:**
- Create: `pkg/pack/nai195_deviation_pins_test.go`

Per `pin_test_self_trigger_production_doc` memory: phrase the absence checks in terms of the deviation tag's CONCEPT name, not TS identifier substrings — the test must NOT trigger on its own banned-literal text.

- [ ] **Step 9.1: Create `pkg/pack/nai195_deviation_pins_test.go`**

```go
package pack

import (
	"os"
	"strings"
	"testing"
)

// NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS: presence pin —
// the deviation tag must appear in pack_configs.go production
// doc-comment.
func TestNAI195_ConfigOrderDeviationDocumented(t *testing.T) {
	body, err := os.ReadFile("pack_configs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS") {
		t.Errorf("pack_configs.go missing NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS doc-comment")
	}
}

// NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS: absence pin —
// the carryforward deviation NAI-194-D-PARAM-AFTER-VARS must still
// be referenced (NAI-195 extends, does not retire it).
func TestNAI195_ParamAfterVarsCarryforwardRetained(t *testing.T) {
	body, err := os.ReadFile("pack_configs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "NAI-194-D-PARAM-AFTER-VARS") {
		t.Errorf("NAI-194-D-PARAM-AFTER-VARS reference removed; retiring it requires moving .param before .varp/.varn/.vars")
	}
}

// NAI-195-D-PACKFILE-SINGLETONS-DEFERRED: no top-level *Pack decls
// for enum/inv/mesanim/struct (continues NAI-193/194 deferral).
func TestNAI195_PackFileSingletonsDeferred_FourNewConfigs(t *testing.T) {
	decls := scanPackageDecls(t)
	for _, banned := range []string{"EnumPack", "InvPack", "MesAnimPack", "StructPack"} {
		if decls[banned] {
			t.Errorf("found top-level decl %q in pkg/pack — violates PACKFILE-SINGLETONS-DEFERRED", banned)
		}
	}
}

// NAI-195-D-VALIDATE-DEFERRED extension: no BUILD_VERIFY-style
// callback identifiers in any of the 4 new packer sources.
func TestNAI195_ValidateDeferred_NoBuildVerifyInNewSources(t *testing.T) {
	for _, src := range []string{"enum.go", "inv.go", "mesanim.go", "struct.go"} {
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"BuildVerify", "BUILD_VERIFY", "checkCRC", "checkcrc"} {
			if strings.Contains(string(body), banned) {
				t.Errorf("found %q in pkg/pack/%s — violates VALIDATE-DEFERRED", banned, src)
			}
		}
	}
}
```

- [ ] **Step 9.2: Run — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestNAI195 -v
```

Expected: 4 pin tests PASS.

- [ ] **Step 9.3: Run full repo test suite — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all tests PASS.

- [ ] **Step 9.4: Commit**

```bash
git add pkg/pack/nai195_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-195 T9 — deviation-tag pins

Presence pin for NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS.
Carryforward retention pin for NAI-194-D-PARAM-AFTER-VARS.
PACKFILE-SINGLETONS-DEFERRED extension for the 4 new configs.
VALIDATE-DEFERRED no-BUILD_VERIFY scan across the 4 new packer files.

Pin tag concepts (CONFIG-ORDER-EXTENDS, etc.) per
pin_test_self_trigger_production_doc memory — avoid TS-identifier
substrings that would self-trigger.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10 — Close commit

- [ ] **Step 10.1: Run full repo test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all PASS.

- [ ] **Step 10.2: Run with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/...
```

Expected: all PASS, no race warnings.

- [ ] **Step 10.3: Create close commit (no source change)**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-195 — enum/inv/mesanim/struct packer slice

4 server-only per-config packers landed (T1-T9). Spec at
docs/superpowers/specs/2026-05-13-nai-195-enum-inv-mesanim-struct
-packers-design.md (db0abfa).

New deviation: NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS.
Carryforward: PACKFILE-SINGLETONS-DEFERRED, VALIDATE-DEFERRED,
FRESH-CLIENT-JAGFILE, PARAM-EMPTY-CLIENT-FAITHFUL, PARAM-AFTER-VARS,
NO-SRC-NO-OP, DEADBRANCH-OMITTED (applied to 3 new parsers).
Retired: none.

Closes memory: 2026-05-13 NAI-195

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review (controller, before handoff)

| Check | Status |
|---|---|
| Spec §2 (in-scope: 4 packers + orchestrator wiring + ParamType runtime load + tests + pins) → tasks T1-T9 cover | ✅ |
| Spec §4 (file layout) ↔ T2/T3/T4/T5 new files | ✅ |
| Spec §5.1 (.enum AUTOINT collapse, val list, default emission) ↔ T3 packer with branches | ✅ |
| Spec §5.2 (.inv duplicate-stockN + size-bound errors) ↔ T4 packer error tests | ✅ |
| Spec §5.3 (.mesanim opcode formula `max(0, len-1)+1`) ↔ T2 packer test cases | ✅ |
| Spec §5.4 (.struct opcode 249 + p1 count + per-param triplet) ↔ T5 packer test | ✅ |
| Spec §6 (lk hoisting + 4 branches + ParamType runtime load) ↔ T6 explicit edits | ✅ |
| Spec §7.2 (NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS new tag) ↔ T6 doc-comment + T9 presence pin | ✅ |
| Spec §8 (per-config + integration + pin tests) ↔ T7 + T8 + T9 | ✅ |
| `pin_test_self_trigger_production_doc` memory honored — T9 uses concept names not TS-source substrings | ✅ |
| `plan_runnable_test_fixtures` memory honored — all fixtures use explicit `*PackFile`, `ParamValue{...}`, etc. with named struct fields | ✅ |
| Modern Go idioms (`for id := range pf.Max`, `strings.Cut`) used throughout | ✅ |
| Type consistency: `parseEnumConfig` (no closure factory), `parseInvConfigFor`/`parseMesAnimConfigFor`/`parseStructConfigFor` (closures) — matches across all references | ✅ |
| Function-scope `lk` hoisting in T6 not duplicated anywhere else | ✅ |
| `packAndSaveEnum/Inv/MesAnim/Struct` helpers all in T6.1.6 — referenced only by orchestrator | ✅ |
| Tests reference real public APIs: `objtype.LoadEnumTypes/LoadInvTypes/LoadMesanimTypes/LoadStructTypes/LoadParamTypes`, `InvTypeScopeShared/Perm/Temp`, `ScriptVarTypeFromName` | ✅ |
| Integration test exercises ParamType runtime load via `.struct` round-trip (T7.4) | ✅ |

No gaps found.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-13-nai-195-enum-inv-mesanim-struct-packers.md`. Per `superpowers_clear_between_spec_and_impl` memory: emit resume prompt for fresh-session implementation rather than auto-dispatching.
