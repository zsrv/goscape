# NAI-194 — .param packer slice — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `tools/pack/config/ParamConfig.ts` onto the NAI-192/193 PackShared infrastructure. First per-config packer with cross-domain `*PackFile` typed-id lookups for default-value resolution. Includes a loader-side `NewParamType().AutoDisable = true` fix on the slice's critical path.

**Architecture:** New code in `pkg/pack/param.go` (parse + lookup + pack) + extension to `pkg/pack/pack_configs.go` (`.param` branch + 13-PackFile `loadParamLookups`). One loader-side fix in `pkg/objtype/paramtype.go` (`AutoDisable` default-true). TS-faithful empty-client buffer written to client jagfile.

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` + `pkg/io/jagfile` + NAI-191/192/193 `pkg/pack` foundation + `pkg/coordgrid` (PackCoord) + `pkg/objtype` (ScriptVarType, ParamType, LoadParamTypes).

**Spec:** `docs/superpowers/specs/2026-05-13-nai-194-param-packer-design.md` (commit `334e380`).
**HEAD at plan-write:** `334e380`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/pack/varp_test.go` / `varn_test.go`: bare `if err != nil { t.Fatal(err) }`, `bytes.Equal` for byte-level comparison, `t.Fatalf("got % x, want % x", got, want)` for byte diffs, `t.TempDir()` for fixture roots, `ClearFsCache()` before any test that mutates the filesystem, `writeFile(t, path, content)` already in `pkg/pack/constants_test.go`.
- **Error envelope** matches existing `pkg/pack/parse.go` style: `fmt.Errorf("<kind>: %s", detail)` or `fmt.Errorf("<context>: %w", err)` for wrapping.
- **Modern Go**: `for id := range pf.Max`, `slices.Index`, `strconv.ParseInt(_, 0, 64)`.
- **Param identifiers** use `paramPF`, `paramLookups` (the struct), `lk` (the local var) — consistent across all tasks.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `334e380`:

| Premise | Verification |
|---|---|
| `pkg/objtype/paramtype.go:148-154` initializes `NewParamType` without setting `AutoDisable` (Go zero `false`) | ✅ Read |
| `pkg/objtype/paramtype.go:82` declares `AutoDisable bool` on ParamType | ✅ Read |
| `pkg/objtype/paramtype.go:93` sets `AutoDisable = false` in opcode-4 case | ✅ Read |
| `pkg/objtype/paramtype_test.go` has 3 tests — all use `Decode(2, ...)`; none read `AutoDisable` directly | ✅ Read |
| `pkg/objtype/objtype.go:113` reads `ptc.Configs[k].AutoDisable` (the only non-test consumer) | ✅ Read |
| `pkg/objtype/scriptvartype.go:11-35` exports ScriptVarType constants: Int, String, Boolean, Coord, Enum, Obj, Loc, Component, NamedObj, Struct, Category, Spotanim, NPC, Inv, Synth, Seq, Stat, NpcStat, Interface, Varp, Dbrow | ✅ Read |
| `pkg/objtype.ScriptVarTypeFromName(name string) (ScriptVarType, bool)` exists (NAI-192 T1) | ✅ Grep |
| `pkg/coordgrid/coordgrid.go:158` `PackCoord(level, x, z) = (z & 0x3fff) \| ((x & 0x3fff) << 14) \| ((level & 0x3) << 28)` — matches TS `z \| (x<<14) \| (level<<28)` within valid ranges | ✅ Read |
| `pkg/pack/packfile.go:28-36` defines `PackFile{Type, SrcDir, Validator, Pack, Names, NameToID, Max}`; line 188 `GetByID`; line 192 `GetByName` (returns -1 when absent) | ✅ Read |
| `pkg/pack.NewPackFile(srcDir, packType string, validator Validator) (*PackFile, error)` exists | ✅ Read |
| `pkg/pack.NewPackedData(size int) *PackedData` constructs with 2-byte count header; `Next()` writes 0x00 terminator and records idx entry-length | ✅ Read |
| `pkg/pack.PackedData` exposes `P1/P2/P3/P4/PBool/PJStr` and `.Dat *packet.Packet`, `.Idx *packet.Packet`, `.Save(datPath, idxPath)` | ✅ Read |
| `pkg/pack.ConfigValue = any`; `pkg/pack.ConfigLine{Key, Value}`; `IsConfigBoolean`/`GetConfigBoolean` exist | ✅ Read |
| `pkg/pack.ReadTypedConfigs(srcDir, ext, required []string, parseFn ParseFn, c Constants) (map[string][]ConfigLine, error)` exists | ✅ Read |
| `pkg/pack.LoadConstants(srcDir) (Constants, error)` + `pkg/pack.GetLatestModified` + `pkg/pack.ShouldBuild` exist (NAI-191/192 infra) | ✅ Read |
| `pkg/pack.checkVarNameUniqueness(...)` exists (NAI-193); pattern reused by new `.param` branch | ✅ Read |
| `pkg/objtype.LoadParamTypes(dir)` returns `*ParamTypeConfigs{ConfigNames map[string]int, Configs []*ParamType}` | ✅ Read |
| `pkg/io/jagfile.Jagfile.Write(name, *packet.Packet)` + `.Save(path, doNotCompressWhole bool)` exist (NAI-193 fix landed) | ✅ Read |
| `Content/pack/{param,enum,obj,loc,interface,struct,category,spotanim,npc,inv,synth,seq,dbrow}.pack` exist (real-content reference only — tests use hand-crafted in t.TempDir) | ✅ ls |
| `pkg/pack/nai192_deviation_pins_test.go:scanPackageDecls(t *testing.T) map[string]bool` helper exists for NAI-194 absence pins | ✅ Read |
| `pkg/pack/varp.go` is the closest structural template for `pkg/pack/param.go` | ✅ Read |
| TS `ParamConfig.ts` schema (autodisable, type, default) and `lookupParamValue` 20-arm switch + null sentinel | ✅ Read |

---

## File inventory

```
pkg/objtype/
  paramtype.go                            MODIFY (NewParamType: AutoDisable = true)
  paramtype_test.go                       MODIFY (add positive default-true test)

pkg/pack/
  param.go                                NEW  (parseParamConfig + lookupParamValue
                                                + parseParamCoord + paramIndexOrErr
                                                + paramLookups + paramStats/paramNpcStats
                                                + packParamConfigs)
  param_test.go                           NEW  (parse + lookup + byte-pin tests)
  pack_configs.go                         MODIFY (param branch — gated PackFile + lookups
                                                  construction + packAndSaveParam +
                                                  clientJagDirty flip; loadParamLookups helper)
  pack_configs_test.go                    MODIFY (add param integration tests + round-trip)
  nai194_deviation_pins_test.go           NEW

docs/superpowers/plans/
  2026-05-13-nai-194-param-packer.md       NEW (this file)
```

---

## Task 1: Loader-side `AutoDisable` default fix in `pkg/objtype/paramtype.go`

**Files:**
- Modify: `pkg/objtype/paramtype.go:148-154`
- Test: `pkg/objtype/paramtype_test.go` (extend)

This is a 1-line fix to surface latent TS-divergence on the loader side. The packer slice's round-trip (T6) cannot pass without it. Lands before any pack code so all downstream packer work assumes TS-parity defaults.

- [ ] **Step 1: Write the failing test**

Append to `pkg/objtype/paramtype_test.go`:

```go
// TestNewParamType_DefaultAutoDisableTrue pins TS parity:
// src/cache/config/ParamType.ts:64 declares `autodisable = true` as the
// default. Goscape NewParamType previously omitted the field,
// silently producing AutoDisable=false (Go zero). NAI-194 fix.
func TestNewParamType_DefaultAutoDisableTrue(t *testing.T) {
	pt := NewParamType(0)
	if !pt.AutoDisable {
		t.Fatalf("AutoDisable default = false, want true (TS ParamType.ts:64)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestNewParamType_DefaultAutoDisableTrue -v`
Expected: FAIL — `AutoDisable default = false, want true (TS ParamType.ts:64)`.

- [ ] **Step 3: Apply the loader fix**

Edit `pkg/objtype/paramtype.go:148-154`:

```go
// NewParamType allocates a ParamType slot. AutoDisable defaults to
// true per TS src/cache/config/ParamType.ts:64 (NAI-194 fix — goscape
// previously omitted the field and inherited Go zero false, silently
// diverging from TS for params that emit no opcode-4).
func NewParamType(id int) *ParamType {
	return &ParamType{
		ConfigType: ConfigType{
			ID: id,
		},
		AutoDisable: true,
	}
}
```

- [ ] **Step 4: Run the new test + all pkg/objtype tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -count=1`
Expected: PASS (new test green; existing `TestParamType_DecodeNegativeDefault`/`Positive`/`MaxInt32` still pass because they assert `DefaultInt`, not `AutoDisable`).

- [ ] **Step 5: Grep for any other AutoDisable readers that might depend on the false-default**

Run:
```bash
rg -n "AutoDisable" pkg/ modules/ cmd/
```
Expected: 5 hits — the field declaration at `paramtype.go:82`, the opcode-4 setter at line 93, the new test, the new doc-comment, and `pkg/objtype/objtype.go:113` (the lone production reader). Verify the production reader's semantics: `if ptc.Configs[k].AutoDisable { … }`. With the fix, this is now correctly `true` by default — matching TS production behavior. No fixture in `pkg/objtype/objtype_test.go` constructs a `ParamType` without explicit `AutoDisable` setting in a way that would be regressed.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/paramtype.go pkg/objtype/paramtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(objtype): NAI-194 T1 — NewParamType AutoDisable default-true

Surfaces latent TS-divergence: ParamType.ts:64 declares
`autodisable = true` but goscape NewParamType omitted the field,
inheriting Go zero false. NAI-194 .param packer round-trip
cannot pass without this fix.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `parseParamConfig` in `pkg/pack/param.go`

**Files:**
- Create: `pkg/pack/param.go`
- Create: `pkg/pack/param_test.go`

`parseParamConfig` is the per-key=value parser. Only three keys are recognized: `autodisable` (boolean), `type` (ScriptVarType lookup), `default` (raw passthrough — resolution deferred to pack stage).

- [ ] **Step 1: Write the failing tests**

Create `pkg/pack/param_test.go`:

```go
package pack

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseParamConfig(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantVal ConfigValue
		wantOK  bool
		wantErr bool
	}{
		{"autodisable yes", "autodisable", "yes", true, true, false},
		{"autodisable no", "autodisable", "no", false, true, false},
		{"autodisable true", "autodisable", "true", true, true, false},
		{"autodisable false", "autodisable", "false", false, true, false},
		{"autodisable invalid", "autodisable", "maybe", nil, true, true},
		{"type int", "type", "int", objtype.ScriptVarTypeInt, true, false},
		{"type loc", "type", "loc", objtype.ScriptVarTypeLoc, true, false},
		{"type string", "type", "string", objtype.ScriptVarTypeString, true, false},
		{"type bogus", "type", "bogus", nil, true, true},
		{"default raw passthrough", "default", "anything", "anything", true, false},
		{"default null", "default", "null", "null", true, false},
		{"unknown key", "unknownkey", "x", nil, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseParamConfig(tt.key, tt.value)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err: got %v, want error=%v", err, tt.wantErr)
			}
			if err == nil && got != tt.wantVal {
				t.Errorf("value: got %#v (%T), want %#v (%T)", got, got, tt.wantVal, tt.wantVal)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseParamConfig -v`
Expected: FAIL — `undefined: parseParamConfig`.

- [ ] **Step 3: Implement `parseParamConfig` in `pkg/pack/param.go`**

Create `pkg/pack/param.go`:

```go
package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseParamConfig is the per-key=value parser for .param config blocks.
//
// Accepted keys:
//   - autodisable  (boolean; yes/no/true/false/1/0)
//   - type         (ScriptVarType name → ScriptVarType code)
//   - default      (raw string; resolution deferred to packParamConfigs
//                   after `type` is known — mirrors TS comment
//                   "defer lookup to pack callback")
//
// Return contract (matches NAI-192 ParseFn):
//   - (value, true, nil)  → accepted
//   - (nil, true, err)    → recognized key with invalid value
//   - (nil, false, nil)   → unrecognized key
//
// TS source: tools/pack/config/ParamConfig.ts parseParamConfig (~190-240).
func parseParamConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "autodisable":
		if !IsConfigBoolean(value) {
			return nil, true, fmt.Errorf("invalid boolean: %s", value)
		}
		return GetConfigBoolean(value), true, nil
	case "type":
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	case "default":
		return value, true, nil
	}
	return nil, false, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseParamConfig -v -count=1`
Expected: PASS — all 12 sub-tests green.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/param.go pkg/pack/param_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-194 T2 — parseParamConfig

Per-key parser for .param config blocks: autodisable (bool), type
(ScriptVarType), default (raw passthrough — resolution deferred to
pack stage). Mirrors NAI-192/193 ParseFn contract.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `lookupParamValue` + `parseParamCoord` + `paramIndexOrErr` in `pkg/pack/param.go`

**Files:**
- Modify: `pkg/pack/param.go` (extend)
- Modify: `pkg/pack/param_test.go` (extend)

The bulk of the slice — 20-arm switch over ScriptVarType + null sentinel + COORD math + 13 typed-id PackFile lookups + 2 hardcoded slice indices + INTERFACE colon-reject.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/pack/param_test.go`:

```go
import (
	"strings"
	// (keep existing imports — testing + objtype)
)

// newTestPF builds an in-memory PackFile fixture for lookup tests.
// Avoids touching the filesystem.
func newTestPF(packType string, entries map[int]string) *PackFile {
	pack := make(map[int]string, len(entries))
	names := make(map[string]struct{}, len(entries))
	nameToID := make(map[string]int, len(entries))
	maxID := -1
	for id, name := range entries {
		pack[id] = name
		names[name] = struct{}{}
		nameToID[name] = id
		if id > maxID {
			maxID = id
		}
	}
	return &PackFile{
		Type:     packType,
		Pack:     pack,
		Names:    names,
		NameToID: nameToID,
		Max:      maxID + 1,
	}
}

func TestLookupParamValue_NullSentinel(t *testing.T) {
	lk := &paramLookups{}
	got, err := lookupParamValue(objtype.ScriptVarTypeInt, "null", lk)
	if err != nil {
		t.Fatalf("INT null: %v", err)
	}
	if got != int(-1) {
		t.Errorf("INT null: got %#v, want -1", got)
	}
	got, err = lookupParamValue(objtype.ScriptVarTypeString, "null", lk)
	if err != nil {
		t.Fatalf("STRING null: %v", err)
	}
	if got != "" {
		t.Errorf("STRING null: got %#v, want \"\"", got)
	}
}

func TestLookupParamValue_Int(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		val     string
		want    int
		wantErr bool
	}{
		{"42", 42, false},
		{"-5", -5, false},
		{"0xFF", 255, false},
		{"0x10", 16, false},
		{"abc", 0, true},
		{"0xQQ", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeInt, c.val, lk)
		if (err != nil) != c.wantErr {
			t.Errorf("INT %q: err=%v, wantErr=%v", c.val, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("INT %q: got %#v, want %d", c.val, got, c.want)
		}
	}
}

func TestLookupParamValue_String(t *testing.T) {
	lk := &paramLookups{}
	got, err := lookupParamValue(objtype.ScriptVarTypeString, "hello", lk)
	if err != nil {
		t.Fatalf("STRING hello: %v", err)
	}
	if got != "hello" {
		t.Errorf("STRING hello: got %#v, want \"hello\"", got)
	}

	long := strings.Repeat("a", 1001)
	if _, err := lookupParamValue(objtype.ScriptVarTypeString, long, lk); err == nil {
		t.Errorf("STRING %d chars: want error, got nil", len(long))
	}

	// 1000 exactly is accepted.
	at := strings.Repeat("a", 1000)
	if _, err := lookupParamValue(objtype.ScriptVarTypeString, at, lk); err != nil {
		t.Errorf("STRING 1000 chars: unexpected error %v", err)
	}
}

func TestLookupParamValue_Boolean(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		val     string
		want    int
		wantErr bool
	}{
		{"yes", 1, false},
		{"true", 1, false},
		{"1", 1, false},
		{"no", 0, false},
		{"false", 0, false},
		{"0", 0, false},
		{"maybe", 0, true},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeBoolean, c.val, lk)
		if (err != nil) != c.wantErr {
			t.Errorf("BOOL %q: err=%v, wantErr=%v", c.val, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("BOOL %q: got %#v, want %d", c.val, got, c.want)
		}
	}
}

func TestLookupParamValue_Coord(t *testing.T) {
	lk := &paramLookups{}
	// 0_50_50_32_32 → x = 50*64+32 = 3232, z = 50*64+32 = 3232, level = 0
	// pack = z | (x<<14) | (level<<28) = 3232 | (3232<<14) = 3232 | 52953088 = 52956320
	got, err := lookupParamValue(objtype.ScriptVarTypeCoord, "0_50_50_32_32", lk)
	if err != nil {
		t.Fatalf("COORD 0_50_50_32_32: %v", err)
	}
	want := 3232 | (3232 << 14)
	if got != want {
		t.Errorf("COORD 0_50_50_32_32: got %d, want %d", got, want)
	}

	// level=3 high bit set
	got, err = lookupParamValue(objtype.ScriptVarTypeCoord, "3_0_0_0_0", lk)
	if err != nil {
		t.Fatalf("COORD 3_0_0_0_0: %v", err)
	}
	if got != (3 << 28) {
		t.Errorf("COORD 3_0_0_0_0: got %d, want %d", got, 3<<28)
	}

	// Error cases
	errCases := []string{
		"0_50_50_32",       // 4 parts
		"0_50_50_32_32_99", // 6 parts
		"a_b_c_d_e",        // non-numeric
		"4_0_0_0_0",        // level > 3
		"0_256_0_0_0",      // mX > 255
		"0_0_256_0_0",      // mZ > 255
		"0_0_0_64_0",       // lX > 63
		"0_0_0_0_64",       // lZ > 63
		"-1_0_0_0_0",       // level < 0
	}
	for _, c := range errCases {
		if _, err := lookupParamValue(objtype.ScriptVarTypeCoord, c, lk); err == nil {
			t.Errorf("COORD %q: want error, got nil", c)
		}
	}
}

func TestLookupParamValue_TypedIDs(t *testing.T) {
	lk := &paramLookups{
		enumPF:      newTestPF("enum", map[int]string{0: "myenum"}),
		objPF:       newTestPF("obj", map[int]string{7: "myobj"}),
		locPF:       newTestPF("loc", map[int]string{3: "myloc"}),
		interfacePF: newTestPF("interface", map[int]string{42: "myiface"}),
		structPF:    newTestPF("struct", map[int]string{1: "mystruct"}),
		categoryPF:  newTestPF("category", map[int]string{5: "mycat"}),
		spotanimPF:  newTestPF("spotanim", map[int]string{2: "myspot"}),
		npcPF:       newTestPF("npc", map[int]string{99: "mynpc"}),
		invPF:       newTestPF("inv", map[int]string{4: "myinv"}),
		synthPF:     newTestPF("synth", map[int]string{6: "mysynth"}),
		seqPF:       newTestPF("seq", map[int]string{8: "myseq"}),
		varpPF:      newTestPF("varp", map[int]string{0: "myvarp"}),
		dbrowPF:     newTestPF("dbrow", map[int]string{10: "mydbrow"}),
	}
	cases := []struct {
		typ  objtype.ScriptVarType
		name string
		want int
	}{
		{objtype.ScriptVarTypeEnum, "myenum", 0},
		{objtype.ScriptVarTypeObj, "myobj", 7},
		{objtype.ScriptVarTypeNamedObj, "myobj", 7}, // NAMEDOBJ shares ObjPack
		{objtype.ScriptVarTypeLoc, "myloc", 3},
		{objtype.ScriptVarTypeComponent, "myiface", 42}, // COMPONENT → interfacePF
		{objtype.ScriptVarTypeStruct, "mystruct", 1},
		{objtype.ScriptVarTypeCategory, "mycat", 5},
		{objtype.ScriptVarTypeSpotanim, "myspot", 2},
		{objtype.ScriptVarTypeNPC, "mynpc", 99},
		{objtype.ScriptVarTypeInv, "myinv", 4},
		{objtype.ScriptVarTypeSynth, "mysynth", 6},
		{objtype.ScriptVarTypeSeq, "myseq", 8},
		{objtype.ScriptVarTypeVarp, "myvarp", 0},
		{objtype.ScriptVarTypeDbrow, "mydbrow", 10},
	}
	for _, c := range cases {
		got, err := lookupParamValue(c.typ, c.name, lk)
		if err != nil {
			t.Errorf("type=%d name=%q: %v", c.typ, c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("type=%d name=%q: got %#v, want %d", c.typ, c.name, got, c.want)
		}
	}

	// Missing name → error
	if _, err := lookupParamValue(objtype.ScriptVarTypeNPC, "nonexistent", lk); err == nil {
		t.Errorf("NPC nonexistent: want error, got nil")
	}

	// Nil PackFile → error (defensive — TS would crash on undefined.getByName)
	emptyLk := &paramLookups{}
	if _, err := lookupParamValue(objtype.ScriptVarTypeLoc, "myloc", emptyLk); err == nil {
		t.Errorf("LOC with nil locPF: want error, got nil")
	}
}

func TestLookupParamValue_Stat(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		name string
		want int
	}{
		{"attack", 0},
		{"defence", 1},
		{"hitpoints", 3},
		{"runecraft", 20},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeStat, c.name, lk)
		if err != nil {
			t.Errorf("STAT %q: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("STAT %q: got %#v, want %d", c.name, got, c.want)
		}
	}
	if _, err := lookupParamValue(objtype.ScriptVarTypeStat, "fakeskill", lk); err == nil {
		t.Errorf("STAT fakeskill: want error, got nil")
	}
}

func TestLookupParamValue_NpcStat(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		name string
		want int
	}{
		{"hitpoints", 0},
		{"attack", 1},
		{"strength", 2},
		{"defence", 3},
		{"magic", 4},
		{"ranged", 5},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeNpcStat, c.name, lk)
		if err != nil {
			t.Errorf("NPC_STAT %q: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("NPC_STAT %q: got %#v, want %d", c.name, got, c.want)
		}
	}
	// Player-only stat is NOT in npcStats.
	if _, err := lookupParamValue(objtype.ScriptVarTypeNpcStat, "agility", lk); err == nil {
		t.Errorf("NPC_STAT agility: want error, got nil")
	}
}

func TestLookupParamValue_InterfaceColonReject(t *testing.T) {
	lk := &paramLookups{
		interfacePF: newTestPF("interface", map[int]string{42: "myiface"}),
	}
	// No colon → resolves via interfacePF
	got, err := lookupParamValue(objtype.ScriptVarTypeInterface, "myiface", lk)
	if err != nil {
		t.Fatalf("INTERFACE myiface: %v", err)
	}
	if got != 42 {
		t.Errorf("INTERFACE myiface: got %#v, want 42", got)
	}
	// Colon → reject before lookup
	if _, err := lookupParamValue(objtype.ScriptVarTypeInterface, "myiface:component", lk); err == nil {
		t.Errorf("INTERFACE with ':' want error, got nil")
	}
}

func TestLookupParamValue_UnsupportedType(t *testing.T) {
	lk := &paramLookups{}
	// PlayerUid is a valid ScriptVarType but not a default-resolvable param type.
	if _, err := lookupParamValue(objtype.ScriptVarTypePlayerUid, "anything", lk); err == nil {
		t.Errorf("PlayerUid: want unsupported-type error, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestLookupParamValue -v`
Expected: FAIL — `undefined: paramLookups`, `undefined: lookupParamValue`, `undefined: newTestPF`.

- [ ] **Step 3: Implement `paramLookups`, `lookupParamValue`, `parseParamCoord`, `paramIndexOrErr` in `pkg/pack/param.go`**

Append to `pkg/pack/param.go`:

```go
import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// paramLookups bundles every *PackFile that lookupParamValue may need
// to resolve a typed-id default. Constructed once per PackConfigs call
// via loadParamLookups (only when .param source is present).
//
// NAI-194-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level *Pack
// singletons; goscape threads pointers explicitly.
type paramLookups struct {
	enumPF      *PackFile
	objPF       *PackFile
	locPF       *PackFile
	interfacePF *PackFile
	structPF    *PackFile
	categoryPF  *PackFile
	spotanimPF  *PackFile
	npcPF       *PackFile
	invPF       *PackFile
	synthPF     *PackFile
	seqPF       *PackFile
	varpPF      *PackFile
	dbrowPF     *PackFile
}

// paramStats / paramNpcStats are TS-hardcoded ordered lists from
// tools/pack/config/ParamConfig.ts:6-30. The slice index becomes the
// packed DefaultInt. Order is load-bearing and must stay synced.
var paramStats = []string{
	"attack", "defence", "strength", "hitpoints", "ranged", "prayer",
	"magic", "cooking", "woodcutting", "fletching", "fishing", "firemaking",
	"crafting", "smithing", "mining", "herblore", "agility", "thieving",
	"slayer", "farming", "runecraft",
}

var paramNpcStats = []string{
	"hitpoints", "attack", "strength", "defence", "magic", "ranged",
}

// lookupParamValue resolves a raw `default=` value against a
// ScriptVarType. Returns the resolved scalar (int for indexed/primitive
// types, string for STRING) or an error. The "null" string is a
// sentinel: returns -1 for non-STRING types and "" for STRING.
//
// TS source: tools/pack/config/ParamConfig.ts lookupParamValue
// (~33-180). 20 arms over ScriptVarType + 1 null-sentinel early-return.
// NAMEDOBJ and OBJ share an arm. COMPONENT routes through interfacePF.
// INTERFACE rejects values containing ':' before the pack lookup.
func lookupParamValue(typ objtype.ScriptVarType, value string, lk *paramLookups) (any, error) {
	if value == "null" {
		if typ == objtype.ScriptVarTypeString {
			return "", nil
		}
		return int(-1), nil
	}

	switch typ {
	case objtype.ScriptVarTypeInt:
		n, err := strconv.ParseInt(value, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int default %q", value)
		}
		return int(n), nil

	case objtype.ScriptVarTypeString:
		if len(value) > 1000 {
			return nil, fmt.Errorf("string default exceeds 1000 chars")
		}
		return value, nil

	case objtype.ScriptVarTypeBoolean:
		if !IsConfigBoolean(value) {
			return nil, fmt.Errorf("invalid boolean default %q", value)
		}
		if GetConfigBoolean(value) {
			return int(1), nil
		}
		return int(0), nil

	case objtype.ScriptVarTypeCoord:
		return parseParamCoord(value)

	case objtype.ScriptVarTypeEnum:
		return paramIndexOrErr(lk.enumPF, value, "enum")
	case objtype.ScriptVarTypeNamedObj, objtype.ScriptVarTypeObj:
		return paramIndexOrErr(lk.objPF, value, "obj")
	case objtype.ScriptVarTypeLoc:
		return paramIndexOrErr(lk.locPF, value, "loc")
	case objtype.ScriptVarTypeComponent:
		return paramIndexOrErr(lk.interfacePF, value, "component")
	case objtype.ScriptVarTypeStruct:
		return paramIndexOrErr(lk.structPF, value, "struct")
	case objtype.ScriptVarTypeCategory:
		return paramIndexOrErr(lk.categoryPF, value, "category")
	case objtype.ScriptVarTypeSpotanim:
		return paramIndexOrErr(lk.spotanimPF, value, "spotanim")
	case objtype.ScriptVarTypeNPC:
		return paramIndexOrErr(lk.npcPF, value, "npc")
	case objtype.ScriptVarTypeInv:
		return paramIndexOrErr(lk.invPF, value, "inv")
	case objtype.ScriptVarTypeSynth:
		return paramIndexOrErr(lk.synthPF, value, "synth")
	case objtype.ScriptVarTypeSeq:
		return paramIndexOrErr(lk.seqPF, value, "seq")
	case objtype.ScriptVarTypeVarp:
		return paramIndexOrErr(lk.varpPF, value, "varp")
	case objtype.ScriptVarTypeDbrow:
		return paramIndexOrErr(lk.dbrowPF, value, "dbrow")

	case objtype.ScriptVarTypeStat:
		i := slices.Index(paramStats, value)
		if i < 0 {
			return nil, fmt.Errorf("unknown stat %q", value)
		}
		return i, nil

	case objtype.ScriptVarTypeNpcStat:
		i := slices.Index(paramNpcStats, value)
		if i < 0 {
			return nil, fmt.Errorf("unknown npc_stat %q", value)
		}
		return i, nil

	case objtype.ScriptVarTypeInterface:
		if strings.Contains(value, ":") {
			return nil, fmt.Errorf("interface default may not contain ':': %q", value)
		}
		return paramIndexOrErr(lk.interfacePF, value, "interface")
	}

	return nil, fmt.Errorf("unsupported default ScriptVarType %d", typ)
}

// paramIndexOrErr resolves `value` against pf. Returns the id, or an
// error if pf is nil (not loaded) or name is unknown.
//
// goscape stricter than TS: TS crashes on undefined.getByName when the
// *Pack singleton hasn't been initialized; goscape returns a typed
// error so the failure mode is named.
func paramIndexOrErr(pf *PackFile, value, kind string) (int, error) {
	if pf == nil {
		return 0, fmt.Errorf("%s pack not loaded", kind)
	}
	i := pf.GetByName(value)
	if i < 0 {
		return 0, fmt.Errorf("unknown %s %q", kind, value)
	}
	return i, nil
}

// parseParamCoord splits `level_mX_mZ_lX_lZ` and packs via
// coordgrid.PackCoord. Bounds: level ∈ [0,3], mX/mZ ∈ [0,255],
// lX/lZ ∈ [0,63]. All parts must be non-negative integers.
//
// TS source: ParamConfig.ts:74-90.
func parseParamCoord(value string) (int, error) {
	parts := strings.Split(value, "_")
	if len(parts) != 5 {
		return 0, fmt.Errorf("coord must be 5 parts (level_mX_mZ_lX_lZ): %q", value)
	}
	level, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("coord level: %w", err)
	}
	mX, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("coord mX: %w", err)
	}
	mZ, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("coord mZ: %w", err)
	}
	lX, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, fmt.Errorf("coord lX: %w", err)
	}
	lZ, err := strconv.Atoi(parts[4])
	if err != nil {
		return 0, fmt.Errorf("coord lZ: %w", err)
	}
	if level < 0 || mX < 0 || mZ < 0 || lX < 0 || lZ < 0 {
		return 0, fmt.Errorf("coord parts must be non-negative: %q", value)
	}
	if level > 3 || mX > 255 || mZ > 255 || lX > 63 || lZ > 63 {
		return 0, fmt.Errorf("coord part out of range (level≤3, m*≤255, l*≤63): %q", value)
	}
	x := mX*64 + lX
	z := mZ*64 + lZ
	return coordgrid.PackCoord(level, x, z), nil
}
```

**Note on imports**: the `import` block above is the union of what `parseParamConfig` needed (T2: `fmt`, `objtype`) and what T3 adds (`slices`, `strconv`, `strings`, `coordgrid`). Replace the existing T2 import block with this consolidated form.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestLookupParamValue -v -count=1`
Expected: PASS — all 9 sub-suites green.

- [ ] **Step 5: Run all `pkg/pack` tests to ensure no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1`
Expected: PASS (existing NAI-191/192/193 tests stay green).

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/param.go pkg/pack/param_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-194 T3 — lookupParamValue + parseParamCoord

20-arm switch over ScriptVarType + null sentinel. 4 primitives
(INT/STRING/BOOLEAN/COORD) + 13 typed-id lookups via paramLookups
struct + STAT/NPC_STAT hardcoded slice indices + INTERFACE
colon-reject. NAMEDOBJ shares ObjPack; COMPONENT routes through
interfacePF.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `packParamConfigs` byte-pin in `pkg/pack/param.go`

**Files:**
- Modify: `pkg/pack/param.go` (extend)
- Modify: `pkg/pack/param_test.go` (extend)

The packer walks every id ∈ [0, paramPF.Max), pre-scans for `type` to enable default-value lookup, emits opcodes 1/2/5/4/250 + debugname trailer, terminates each slot with `Next()` on both server and client buffers. Client buffer is initialized but never `.p1()`'d (TS-faithful empty client per `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL`).

- [ ] **Step 1: Write the failing tests**

Append to `pkg/pack/param_test.go`:

```go
import (
	// keep existing imports + add:
	"bytes"
)

// TestPackParamConfigs_IntDefaultAutodisableFalse pins one slot with
// type=int, default=100, autodisable=no (opcode 4 emitted).
func TestPackParamConfigs_IntDefaultAutodisableFalse(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "health_param", 1: ""})
	cfgs := map[string][]ConfigLine{
		"health_param": {
			{Key: "type", Value: objtype.ScriptVarTypeInt},
			{Key: "default", Value: "100"},
			{Key: "autodisable", Value: false},
		},
	}
	server, client, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}

	// Server dat:
	//   00 02              count header (size=2)
	//   slot 0 body: 01 69 (type=int=105), 02 00 00 00 64 (default p4(100)),
	//                04 (autodisable=false),
	//                fa <"health_param" + LF> (debugname trailer)
	//   slot 0 terminator: 00
	//   slot 1 (empty, no name): 00
	wantServerDat := []byte{
		0x00, 0x02,
		0x01, 0x69, // type=int (105)
		0x02, 0x00, 0x00, 0x00, 0x64, // default=p4(100)
		0x04, // autodisable=false
		0xfa,
		'h', 'e', 'a', 'l', 't', 'h', '_', 'p', 'a', 'r', 'a', 'm', '\n',
		0x00, // slot 0 terminator
		0x00, // slot 1 terminator
	}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, wantServerDat)
	}

	// Server idx: 00 02 (count) | 00 <slot 0 byte count incl. terminator>
	// | 00 01 (slot 1: terminator only).
	// Slot 0 byte count = 2 (type) + 5 (default p4) + 1 (autodisable=false)
	//                   + 14 (0xfa + 12-byte name + 0x0a LF) + 1 (terminator) = 23 = 0x17
	wantServerIdx := []byte{
		0x00, 0x02,
		0x00, 0x17,
		0x00, 0x01,
	}
	if !bytes.Equal(server.Idx.Data, wantServerIdx) {
		t.Fatalf("server.Idx:\n  got: % x\n  want: % x", server.Idx.Data, wantServerIdx)
	}

	// Client dat: empty content per NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL.
	// 00 02 (count) | 00 (slot 0 terminator) | 00 (slot 1 terminator)
	wantClientDat := []byte{0x00, 0x02, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, wantClientDat) {
		t.Fatalf("client.Dat:\n  got: % x\n  want: % x", client.Dat.Data, wantClientDat)
	}
	wantClientIdx := []byte{0x00, 0x02, 0x00, 0x01, 0x00, 0x01}
	if !bytes.Equal(client.Idx.Data, wantClientIdx) {
		t.Fatalf("client.Idx:\n  got: % x\n  want: % x", client.Idx.Data, wantClientIdx)
	}
}

// TestPackParamConfigs_StringDefault pins the STRING path: opcode 5
// instead of opcode 2; payload is pjstr instead of p4.
func TestPackParamConfigs_StringDefault(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "name_param"})
	cfgs := map[string][]ConfigLine{
		"name_param": {
			{Key: "type", Value: objtype.ScriptVarTypeString},
			{Key: "default", Value: "hello"},
			// no autodisable → default true → no opcode 4
		},
	}
	server, _, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}

	// Slot 0 body:
	//   01 73                                type=string (115)
	//   05 'h' 'e' 'l' 'l' 'o' 0x0a          default=pjstr("hello")
	//   fa "name_param" 0x0a                 debugname trailer
	//   00                                   slot terminator
	wantSlot0 := []byte{
		0x01, 0x73,
		0x05, 'h', 'e', 'l', 'l', 'o', '\n',
		0xfa, 'n', 'a', 'm', 'e', '_', 'p', 'a', 'r', 'a', 'm', '\n',
		0x00,
	}
	want := append([]byte{0x00, 0x01}, wantSlot0...)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, want)
	}
}

// TestPackParamConfigs_CoordDefault pins the COORD path: opcode 2 with
// the packed coord integer.
func TestPackParamConfigs_CoordDefault(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "start"})
	cfgs := map[string][]ConfigLine{
		"start": {
			{Key: "type", Value: objtype.ScriptVarTypeCoord},
			{Key: "default", Value: "0_50_50_32_32"},
		},
	}
	server, _, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	packed := uint32((3232) | (3232 << 14))
	wantSlot0 := []byte{
		0x01, 0x63, // type=coord (99)
		0x02,
		byte(packed >> 24), byte(packed >> 16), byte(packed >> 8), byte(packed),
		0xfa, 's', 't', 'a', 'r', 't', '\n',
		0x00,
	}
	want := append([]byte{0x00, 0x01}, wantSlot0...)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, want)
	}
}

// TestPackParamConfigs_TypedDefaultViaPackFile pins NPC default
// resolution via lookupParamValue + paramIndexOrErr.
func TestPackParamConfigs_TypedDefaultViaPackFile(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "boss"})
	lk := &paramLookups{
		npcPF: newTestPF("npc", map[int]string{42: "kalphite_queen"}),
	}
	cfgs := map[string][]ConfigLine{
		"boss": {
			{Key: "type", Value: objtype.ScriptVarTypeNPC},
			{Key: "default", Value: "kalphite_queen"},
		},
	}
	server, _, err := packParamConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	wantSlot0 := []byte{
		0x01, 0x6e, // type=npc (110)
		0x02, 0x00, 0x00, 0x00, 0x2a, // default=p4(42)
		0xfa, 'b', 'o', 's', 's', '\n',
		0x00,
	}
	want := append([]byte{0x00, 0x01}, wantSlot0...)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, want)
	}
}

// TestPackParamConfigs_MissingType errors with a clear message —
// goscape stricter than TS's implicit `!`-assertion crash.
func TestPackParamConfigs_MissingType(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "broken"})
	cfgs := map[string][]ConfigLine{
		"broken": {
			{Key: "default", Value: "42"},
		},
	}
	_, _, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err == nil {
		t.Fatalf("missing type: want error, got nil")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the param: got %v", err)
	}
}

// TestPackParamConfigs_UnknownTypedDefault propagates lookupParamValue
// errors with the param debugname in scope.
func TestPackParamConfigs_UnknownTypedDefault(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "boss"})
	lk := &paramLookups{
		npcPF: newTestPF("npc", map[int]string{42: "kalphite_queen"}),
	}
	cfgs := map[string][]ConfigLine{
		"boss": {
			{Key: "type", Value: objtype.ScriptVarTypeNPC},
			{Key: "default", Value: "nonexistent_npc"},
		},
	}
	_, _, err := packParamConfigs(cfgs, pf, lk)
	if err == nil {
		t.Fatalf("unknown npc default: want error, got nil")
	}
	if !strings.Contains(err.Error(), "boss") {
		t.Errorf("error should name the param: got %v", err)
	}
}

// TestPackParamConfigs_EmptyClientFaithful pins NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL:
// regardless of payload, client.Dat must be exactly count-header + N×0x00.
func TestPackParamConfigs_EmptyClientFaithful(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "a", 1: "b", 2: "c"})
	cfgs := map[string][]ConfigLine{
		"a": {{Key: "type", Value: objtype.ScriptVarTypeInt}, {Key: "default", Value: "1"}},
		"b": {{Key: "type", Value: objtype.ScriptVarTypeString}, {Key: "default", Value: "x"}},
		"c": {{Key: "type", Value: objtype.ScriptVarTypeInt}, {Key: "default", Value: "99"}},
	}
	_, client, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	want := []byte{0x00, 0x03, 0x00, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("empty-client violated: got % x, want % x", client.Dat.Data, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackParamConfigs -v`
Expected: FAIL — `undefined: packParamConfigs`.

- [ ] **Step 3: Implement `packParamConfigs` in `pkg/pack/param.go`**

Append to `pkg/pack/param.go`:

```go
// packParamConfigs walks every id ∈ [0, pf.Max), pre-scans for the
// `type` key (needed before `default` can resolve via lookupParamValue),
// then emits per-config opcodes on the server buffer:
//
//   type        → P1(1) P1(typechar)
//   default     → P1(2) P4(int)        for non-STRING
//                  P1(5) PJStr(value)   for STRING
//   autodisable → P1(4)                 only when value is false
//   debugname   → P1(250) PJStr(name)   when slot has a name
//
// The client buffer is initialized but never written between Next()
// calls — TS-faithful per NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL.
//
// Returns (server, client, err). err propagates from missing-type
// assertion or from lookupParamValue's default-value resolution.
//
// TS source: tools/pack/config/ParamConfig.ts:184-248. TS uses `!`
// non-null assertion on the type-find; goscape returns an explicit
// error to name the failure mode.
func packParamConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (server, client *PackedData, err error) {
	server = NewPackedData(pf.Max)
	client = NewPackedData(pf.Max)

	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			var typ objtype.ScriptVarType
			typFound := false
			for _, line := range cfg {
				if line.Key == "type" {
					typ = line.Value.(objtype.ScriptVarType)
					typFound = true
					break
				}
			}
			if !typFound {
				return nil, nil, fmt.Errorf("param %q missing type", name)
			}

			for _, line := range cfg {
				switch line.Key {
				case "type":
					server.P1(1)
					server.P1(uint8(typ))
				case "default":
					raw := line.Value.(string)
					resolved, lookupErr := lookupParamValue(typ, raw, lk)
					if lookupErr != nil {
						return nil, nil, fmt.Errorf("param %q default: %w", name, lookupErr)
					}
					if typ == objtype.ScriptVarTypeString {
						server.P1(5)
						server.PJStr(resolved.(string))
					} else {
						server.P1(2)
						server.P4(uint32(resolved.(int)))
					}
				case "autodisable":
					if !line.Value.(bool) {
						server.P1(4)
					}
				}
			}
		}
		if len(name) > 0 {
			server.P1(250)
			server.PJStr(name)
		}
		server.Next()
		client.Next()
	}
	return server, client, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackParamConfigs -v -count=1`
Expected: PASS — all 7 byte-pin sub-suites green.

- [ ] **Step 5: Run all `pkg/pack` tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/param.go pkg/pack/param_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-194 T4 — packParamConfigs byte-pin

Pre-scans for `type` (defer-resolved default depends on it), emits
opcodes 1/2/5/4/250 on the server buffer; client buffer initialized
but never written between Next() calls (NAI-194-D-PARAM-EMPTY-CLIENT-
FAITHFUL). NAMEDOBJ/OBJ share ObjPack; COMPONENT routes through
interfacePF. Errors name the offending param debugname.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `PackConfigs` orchestrator extension with `.param` branch + `loadParamLookups`

**Files:**
- Modify: `pkg/pack/pack_configs.go`
- Modify: `pkg/pack/pack_configs_test.go`

Adds the `.param` branch after the var-domain trio. Constructs paramPF + the 12 typed-id PackFiles only when `.param` source is present (varpPF reused from the up-front trio).

- [ ] **Step 1: Write the failing tests**

Append to `pkg/pack/pack_configs_test.go`:

```go
// helper: write a fixture .param + supporting .pack files into srcDir.
// Returns the srcDir for convenience. Caller must call ClearFsCache().
func setupParamFixture(t *testing.T, srcDir string, slotName, typeName, defaultVal string, extraPacks map[string]map[int]string) {
	t.Helper()
	scriptsDir := filepath.Join(srcDir, "scripts")
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// .param source
	src := fmt.Sprintf("[%s]\ntype=%s\ndefault=%s\n", slotName, typeName, defaultVal)
	writeFile(t, filepath.Join(scriptsDir, "test.param"), src)

	// param.pack (slot 0 → slotName)
	writeFile(t, filepath.Join(packDir, "param.pack"), fmt.Sprintf("0=%s\n", slotName))

	// Default-required typed-id packs (caller supplies). varp/varn/vars
	// are always written so the up-front PackConfigs constructions don't fail.
	writeFile(t, filepath.Join(packDir, "varp.pack"), "")
	writeFile(t, filepath.Join(packDir, "varn.pack"), "")
	writeFile(t, filepath.Join(packDir, "vars.pack"), "")

	for kind, entries := range extraPacks {
		var body strings.Builder
		for id, name := range entries {
			body.WriteString(fmt.Sprintf("%d=%s\n", id, name))
		}
		writeFile(t, filepath.Join(packDir, kind+".pack"), body.String())
	}
}

// emptyTypedPacks creates the 12 non-varp typed-id .pack files as
// empty stubs, so loadParamLookups doesn't fail when the .param branch
// fires. Used when the test's param uses a primitive default.
func writeEmptyTypedPacks(t *testing.T, srcDir string) {
	t.Helper()
	packDir := filepath.Join(srcDir, "pack")
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "npc", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(packDir, kind+".pack"), "")
	}
}

func TestPackConfigs_ParamOnly_PrimitiveDefault(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "health_param", "int", "100", nil)
	writeEmptyTypedPacks(t, srcDir)

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "server", "param.dat")); err != nil {
		t.Errorf("server/param.dat missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "server", "param.idx")); err != nil {
		t.Errorf("server/param.idx missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "client", "config")); err != nil {
		t.Errorf("client/config jagfile missing: %v", err)
	}
}

func TestPackConfigs_ParamWithTypedDefault(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "boss_param", "npc", "kalphite_queen", map[string]map[int]string{
		"npc": {42: "kalphite_queen"},
	})
	// Stub the remaining 11 typed packs so loadParamLookups doesn't fail.
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(srcDir, "pack", kind+".pack"), "")
	}

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}
	// Round-trip via LoadParamTypes confirms DefaultInt=42.
	ptc, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatalf("LoadParamTypes: %v", err)
	}
	if len(ptc.Configs) != 1 {
		t.Fatalf("got %d configs, want 1", len(ptc.Configs))
	}
	if got, want := ptc.Configs[0].DefaultInt, int32(42); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
	if got, want := ptc.Configs[0].DebugName, "boss_param"; got != want {
		t.Errorf("DebugName: got %q, want %q", got, want)
	}
}

func TestPackConfigs_ParamMissingTypedPackFile(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "x", "npc", "kalphite_queen", nil)
	// Do NOT write npc.pack — loadParamLookups should fail.

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatalf("missing npc.pack: want error, got nil")
	}
	if !strings.Contains(err.Error(), "npc") {
		t.Errorf("error should mention npc: got %v", err)
	}
}

func TestPackConfigs_ParamNoSrcNoOp(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	// Only var-domain .pack files; no .param source.
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	// Intentionally omit param.pack and all 12 typed-id .pack files.
	// loadParamLookups must not run.

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("no .param source: PackConfigs should be no-op for param branch, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "server", "param.dat")); !os.IsNotExist(err) {
		t.Errorf("server/param.dat should NOT exist when no .param source")
	}
}

func TestPackConfigs_ParamUnknownTypedDefault(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "boss", "npc", "nonexistent_npc", map[string]map[int]string{
		"npc": {0: "kalphite_queen"}, // doesn't include nonexistent_npc
	})
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(srcDir, "pack", kind+".pack"), "")
	}

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatalf("unknown npc default: want error, got nil")
	}
	if !strings.Contains(err.Error(), "boss") {
		t.Errorf("error should name the param: got %v", err)
	}
}
```

Imports to add (top of file): `"os"`, `"strings"`, `"github.com/zsrv/goscape/pkg/objtype"` (check first — may already be imported via existing tests).

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackConfigs_Param -v`
Expected: FAIL — `PackConfigs` ignores `.param` branch; no `param.dat` produced. Or compile-fails because `setupParamFixture`/`writeEmptyTypedPacks` reference existing helpers but `objtype.LoadParamTypes` import line may be missing.

- [ ] **Step 3: Extend `pkg/pack/pack_configs.go` with `.param` branch + `loadParamLookups` + `packAndSaveParam`**

Edit `pkg/pack/pack_configs.go`. After the existing var-domain branches in `PackConfigs` (after the `.vars` branch closes, **before** the `if clientJagDirty { … }` block), insert:

```go
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

Append two new helpers at the bottom of `pkg/pack/pack_configs.go` (after `packAndSaveVars`):

```go
// loadParamLookups constructs the 12 typed-id PackFiles needed by
// lookupParamValue (the 13th, varpPF, is reused from the up-front
// var-domain trio). Called only when .param source is present so the
// cost is amortized for the no-source case.
//
// NAI-194-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// EnumPack/ObjPack/etc.; goscape constructs from srcDir per call.
func loadParamLookups(srcDir string, varpPF *PackFile) (*paramLookups, error) {
	lk := &paramLookups{varpPF: varpPF}
	for _, t := range []struct {
		name string
		dst  **PackFile
	}{
		{"enum", &lk.enumPF},
		{"obj", &lk.objPF},
		{"loc", &lk.locPF},
		{"interface", &lk.interfacePF},
		{"struct", &lk.structPF},
		{"category", &lk.categoryPF},
		{"spotanim", &lk.spotanimPF},
		{"npc", &lk.npcPF},
		{"inv", &lk.invPF},
		{"synth", &lk.synthPF},
		{"seq", &lk.seqPF},
		{"dbrow", &lk.dbrowPF},
	} {
		pf, err := NewPackFile(srcDir, t.name, nil)
		if err != nil {
			return nil, fmt.Errorf("load %s pack: %w", t.name, err)
		}
		*t.dst = pf
	}
	return lk, nil
}

// packAndSaveParam reads .param sources, packs them, writes server
// .dat/.idx, and queues the empty-client param entries into clientJag.
//
// TS source: tools/pack/PackShared.ts (param branch of packConfigs).
func packAndSaveParam(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants, clientJag *jagfile.Jagfile) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".param", nil, parseParamConfig, c)
	if err != nil {
		return err
	}
	server, client, err := packParamConfigs(cfgs, pf, lk)
	if err != nil {
		return err
	}
	if err := server.Save(
		filepath.Join(serverOut, "param.dat"),
		filepath.Join(serverOut, "param.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("param.dat", client.Dat)
	clientJag.Write("param.idx", client.Idx)
	return nil
}
```

Also update the top-of-file doc comment for `PackConfigs` to mention the new `.param` branch (1-line addition).

- [ ] **Step 4: Run param integration tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackConfigs_Param -v -count=1`
Expected: PASS — all 5 sub-tests green.

- [ ] **Step 5: Run all `pkg/pack` tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1`
Expected: PASS (existing NAI-191/192/193 tests stay green).

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/pack_configs.go pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-194 T5 — PackConfigs .param branch + loadParamLookups

Wires .param into the orchestrator after the var-domain trio. Lazily
constructs the 12 typed-id PackFiles (varp reused) only when .param
source is present. packAndSaveParam writes server .dat/.idx and
queues empty-client param entries into the shared clientJag.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Loader round-trip integration test via `LoadParamTypes`

**Files:**
- Modify: `pkg/pack/param_test.go`

Cross-package end-to-end binding: hand-crafted fixture covering 4 primitives + 1 typed-id + autodisable both arms → `PackConfigs` → `objtype.LoadParamTypes` → assert all fields.

- [ ] **Step 1: Write the failing test**

Append to `pkg/pack/param_test.go`:

```go
import (
	// keep existing + add:
	"os"
	"path/filepath"
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestParamPacker_LoaderRoundTrip binds end-to-end byte-format parity
// through the production loader for 4 primitives + 1 typed-id, plus
// the AutoDisable default-true fix (T1).
func TestParamPacker_LoaderRoundTrip(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scriptsDir := filepath.Join(srcDir, "scripts")
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// .param source — 5 slots.
	writeFile(t, filepath.Join(scriptsDir, "test.param"), `[int_p]
type=int
default=42

[str_p]
type=string
default=hello

[bool_p]
type=boolean
default=yes
autodisable=no

[coord_p]
type=coord
default=0_50_50_32_32

[npc_p]
type=npc
default=man
autodisable=yes
`)

	// param.pack — slot order is load-bearing.
	writeFile(t, filepath.Join(packDir, "param.pack"), `0=int_p
1=str_p
2=bool_p
3=coord_p
4=npc_p
`)

	// npc.pack — single entry.
	writeFile(t, filepath.Join(packDir, "npc.pack"), "0=man\n")

	// Stub the other 11 typed packs + var-domain packs.
	for _, kind := range []string{"varp", "varn", "vars", "enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(packDir, kind+".pack"), "")
	}

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	ptc, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatalf("LoadParamTypes: %v", err)
	}
	if got, want := len(ptc.Configs), 5; got != want {
		t.Fatalf("len(Configs): got %d, want %d", got, want)
	}

	// Slot 0: int_p — type=INT, default=42, autodisable default-true.
	c0 := ptc.Configs[0]
	if got, want := c0.DebugName, "int_p"; got != want {
		t.Errorf("c0.DebugName: got %q, want %q", got, want)
	}
	if got, want := c0.Type, objtype.ScriptVarTypeInt; got != want {
		t.Errorf("c0.Type: got %d, want %d", got, want)
	}
	if got, want := c0.DefaultInt, int32(42); got != want {
		t.Errorf("c0.DefaultInt: got %d, want %d", got, want)
	}
	if !c0.AutoDisable {
		t.Errorf("c0.AutoDisable: got false, want true (default-true per T1)")
	}

	// Slot 1: str_p — type=STRING, default="hello", autodisable default-true.
	c1 := ptc.Configs[1]
	if got, want := c1.Type, objtype.ScriptVarTypeString; got != want {
		t.Errorf("c1.Type: got %d, want %d", got, want)
	}
	if got, want := c1.DefaultString, "hello"; got != want {
		t.Errorf("c1.DefaultString: got %q, want %q", got, want)
	}
	if !c1.AutoDisable {
		t.Errorf("c1.AutoDisable: got false, want true")
	}

	// Slot 2: bool_p — type=BOOLEAN, default=yes→1, autodisable=no → AutoDisable=false.
	c2 := ptc.Configs[2]
	if got, want := c2.Type, objtype.ScriptVarTypeBoolean; got != want {
		t.Errorf("c2.Type: got %d, want %d", got, want)
	}
	if got, want := c2.DefaultInt, int32(1); got != want {
		t.Errorf("c2.DefaultInt: got %d, want %d", got, want)
	}
	if c2.AutoDisable {
		t.Errorf("c2.AutoDisable: got true, want false (opcode 4 emitted)")
	}

	// Slot 3: coord_p — type=COORD, default=PackCoord(0, 50*64+32, 50*64+32).
	c3 := ptc.Configs[3]
	if got, want := c3.Type, objtype.ScriptVarTypeCoord; got != want {
		t.Errorf("c3.Type: got %d, want %d", got, want)
	}
	wantCoord := int32((3232) | (3232 << 14))
	if got := c3.DefaultInt; got != wantCoord {
		t.Errorf("c3.DefaultInt: got %d, want %d", got, wantCoord)
	}

	// Slot 4: npc_p — type=NPC, default="man"→0, autodisable=yes → AutoDisable=true.
	c4 := ptc.Configs[4]
	if got, want := c4.Type, objtype.ScriptVarTypeNPC; got != want {
		t.Errorf("c4.Type: got %d, want %d", got, want)
	}
	if got, want := c4.DefaultInt, int32(0); got != want {
		t.Errorf("c4.DefaultInt: got %d, want %d", got, want)
	}
	if !c4.AutoDisable {
		t.Errorf("c4.AutoDisable: got false, want true")
	}
}
```

- [ ] **Step 2: Run the round-trip test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParamPacker_LoaderRoundTrip -v -count=1`
Expected: PASS — all 5 slots round-trip correctly through `LoadParamTypes`. The T1 fix's AutoDisable-default-true is exercised on slots 0/1/4 (no opcode 4 emitted, loader returns true).

- [ ] **Step 3: Run full repo tests with `-race`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... ./pkg/objtype/... -count=1 -race`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/pack/param_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-194 T6 — param round-trip via LoadParamTypes

5-slot fixture exercising 4 primitives + 1 typed-id (NPC) + both
autodisable arms. Binds end-to-end byte-format parity through the
production loader; surfaces AutoDisable default-true fix from T1
on slots that emit no opcode 4.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: NAI-194 deviation-tag absence pins + final scan

**Files:**
- Create: `pkg/pack/nai194_deviation_pins_test.go`

Mirrors the NAI-192/193 pattern: source-level grep tests that fail loudly if a deferred concern accidentally lands.

- [ ] **Step 1: Write the pin tests**

Create `pkg/pack/nai194_deviation_pins_test.go`:

```go
package pack

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// NAI-194-D-PACKFILE-SINGLETONS-DEFERRED: no module-level ParamPack or
// any of the 12 typed-id *Pack singletons in pkg/pack. scanPackageDecls
// helper lives in nai192_deviation_pins_test.go.
func TestNAI194_PackFileSingletonsDeferred_NoModuleLevelParamPack(t *testing.T) {
	decls := scanPackageDecls(t)
	for _, banned := range []string{
		"ParamPack", "EnumPack", "ObjPack", "LocPack", "InterfacePack",
		"StructPack", "CategoryPack", "SpotAnimPack", "NpcPack", "InvPack",
		"SynthPack", "SeqPack", "DbRowPack",
	} {
		if decls[banned] {
			t.Errorf("found top-level decl %q in pkg/pack — violates NAI-194-D-PACKFILE-SINGLETONS-DEFERRED", banned)
		}
	}
}

// NAI-194-D-VALIDATE-DEFERRED: pkg/pack/param.go must NOT reference any
// BUILD_VERIFY-style validate callback identifiers.
func TestNAI194_ValidateDeferred_NoBuildVerifyInParamSource(t *testing.T) {
	body, err := os.ReadFile("param.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"BuildVerify", "BUILD_VERIFY", "validateParam", "checkCRC", "checkcrc"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("found %q in pkg/pack/param.go — violates NAI-194-D-VALIDATE-DEFERRED", banned)
		}
	}
}

// NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL: packParamConfigs's client
// buffer must contain only the 2-byte count header + N×0x00 terminators
// for any number of slots, regardless of payload. Pin tests the
// invariant against a multi-slot mixed-payload fixture.
func TestNAI194_ParamEmptyClientFaithful(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "a", 1: "b", 2: "c", 3: "d"})
	cfgs := map[string][]ConfigLine{
		"a": {{Key: "type", Value: scriptVarTypeIntForTest()}, {Key: "default", Value: "1"}},
		"b": {{Key: "type", Value: scriptVarTypeStringForTest()}, {Key: "default", Value: "x"}},
		"c": {{Key: "type", Value: scriptVarTypeBooleanForTest()}, {Key: "default", Value: "yes"}, {Key: "autodisable", Value: false}},
		// "d" is empty.
	}
	_, client, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	want := []byte{0x00, 0x04, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client.Dat violates EMPTY-CLIENT-FAITHFUL: got % x, want % x", client.Dat.Data, want)
	}
}
```

Append these tiny helper accessors to `pkg/pack/nai194_deviation_pins_test.go` (kept local to avoid leaking pkg/objtype types into pin test naming):

```go
// scriptVarType*ForTest accessors keep the pin test self-contained.
// They return the int8 ScriptVarType code used by packParamConfigs.
func scriptVarTypeIntForTest() any     { return objtypeScriptVarTypeInt }
func scriptVarTypeStringForTest() any  { return objtypeScriptVarTypeString }
func scriptVarTypeBooleanForTest() any { return objtypeScriptVarTypeBoolean }
```

…or simpler — since `param_test.go` already imports `pkg/objtype`, just inline:

```go
import "github.com/zsrv/goscape/pkg/objtype"

// (and use objtype.ScriptVarTypeInt / String / Boolean directly in the test body)
```

**Pick the inline form.** Simplify by replacing the three `scriptVarType*ForTest()` calls in the pin test with direct `objtype.ScriptVarTypeInt`, `objtype.ScriptVarTypeString`, `objtype.ScriptVarTypeBoolean` and adding `"github.com/zsrv/goscape/pkg/objtype"` to the imports. Remove the helper-accessor block entirely.

The final `nai194_deviation_pins_test.go` should look like:

```go
package pack

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestNAI194_PackFileSingletonsDeferred_NoModuleLevelParamPack(t *testing.T) {
	decls := scanPackageDecls(t)
	for _, banned := range []string{
		"ParamPack", "EnumPack", "ObjPack", "LocPack", "InterfacePack",
		"StructPack", "CategoryPack", "SpotAnimPack", "NpcPack", "InvPack",
		"SynthPack", "SeqPack", "DbRowPack",
	} {
		if decls[banned] {
			t.Errorf("found top-level decl %q in pkg/pack — violates NAI-194-D-PACKFILE-SINGLETONS-DEFERRED", banned)
		}
	}
}

func TestNAI194_ValidateDeferred_NoBuildVerifyInParamSource(t *testing.T) {
	body, err := os.ReadFile("param.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"BuildVerify", "BUILD_VERIFY", "validateParam", "checkCRC", "checkcrc"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("found %q in pkg/pack/param.go — violates NAI-194-D-VALIDATE-DEFERRED", banned)
		}
	}
}

func TestNAI194_ParamEmptyClientFaithful(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "a", 1: "b", 2: "c", 3: "d"})
	cfgs := map[string][]ConfigLine{
		"a": {{Key: "type", Value: objtype.ScriptVarTypeInt}, {Key: "default", Value: "1"}},
		"b": {{Key: "type", Value: objtype.ScriptVarTypeString}, {Key: "default", Value: "x"}},
		"c": {{Key: "type", Value: objtype.ScriptVarTypeBoolean}, {Key: "default", Value: "yes"}, {Key: "autodisable", Value: false}},
	}
	_, client, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	want := []byte{0x00, 0x04, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client.Dat violates EMPTY-CLIENT-FAITHFUL: got % x, want % x", client.Dat.Data, want)
	}
}
```

- [ ] **Step 2: Run the pin tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestNAI194 -v -count=1`
Expected: PASS — all 3 pin tests green.

- [ ] **Step 3: Final scan — verify deviation-tag references match spec §10**

Run: `rg "NAI-194-D-" pkg/`
Expected: at least 3 distinct tag families appear in code comments or test bodies:
- `NAI-194-D-PACKFILE-SINGLETONS-DEFERRED` — referenced in `pkg/pack/param.go` (`paramLookups` doc comment, `loadParamLookups` doc comment) and `pkg/pack/nai194_deviation_pins_test.go`
- `NAI-194-D-VALIDATE-DEFERRED` — referenced in `pkg/pack/nai194_deviation_pins_test.go`
- `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL` — referenced in `pkg/pack/param.go` (`packParamConfigs` doc comment) and `pkg/pack/nai194_deviation_pins_test.go`

If any tag is missing from production source, add a 1-line doc-comment reference to the relevant function/variable.

- [ ] **Step 4: Verify retired tags (none this slice)**

Per spec §10 "Retired this slice: none."

Run: `rg "NAI-19[123]-D-" pkg/` and confirm output matches the post-NAI-193 baseline (no accidental retirements).

- [ ] **Step 5: Run full repo tests with `-race`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... ./pkg/objtype/... ./pkg/io/jagfile/... -count=1 -race`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

Run: `gofmt -l pkg/objtype pkg/pack`
Expected: empty output.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/nai194_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-194 T7 — deviation-tag absence pins

Source-level grep tests pin NAI-194-D-PACKFILE-SINGLETONS-DEFERRED
(no module-level *Pack singletons for any of the 13 typed-id
families), NAI-194-D-VALIDATE-DEFERRED (no BUILD_VERIFY-style
identifiers in param.go), and NAI-194-D-PARAM-EMPTY-CLIENT-
FAITHFUL (client.Dat is count-header + N×0x00 for any input).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Close-out (post-T7)

After all 7 tasks land green, the controller writes one more commit to close the slice:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-194 — .param packer slice

First per-config packer with cross-domain *PackFile typed-id lookups
for default-value resolution (paramLookups bundle: 13 PackFiles).
Establishes the multi-PackFile-in-PackConfigs pattern that NAI-195+
will inherit. Also fixes a latent loader-side AutoDisable default
divergence (T1 — ParamType.ts:64 declares default-true; goscape
inherited Go zero false).

Deviations introduced:
- NAI-194-D-PACKFILE-SINGLETONS-DEFERRED
- NAI-194-D-VALIDATE-DEFERRED
- NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL

Retired: none.

Closes memory: jagfile_write_save_latent_bugs (continues the auto-grow
fix landed in NAI-193 — no new bugs surfaced); plan_geometry_premise_
pretrace (coordgrid.PackCoord bit layout pre-traced against TS at
plan-write).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage — every §4/§7 requirement maps to a task:**

| Spec section | Task |
|---|---|
| §4.1 Loader AutoDisable fix | T1 |
| §4.2 parseParamConfig | T2 |
| §4.3 lookupParamValue + parseParamCoord + paramIndexOrErr + paramLookups + paramStats/paramNpcStats | T3 |
| §4.4 packParamConfigs + missing-type error | T4 |
| §4.5 PackConfigs param branch + loadParamLookups + packAndSaveParam | T5 |
| §4.6 Tracker entries | not a task — close-out commit body |
| §7.1 Loader fix test | T1 step 1 |
| §7.2 parseParamConfig per-key table | T2 step 1 |
| §7.3 lookupParamValue 20-arm coverage | T3 step 1 (9 sub-suites) |
| §7.4 packParamConfigs byte-pin (int/string/coord/typed/missing-type/unknown-default/empty-client) | T4 step 1 (7 sub-tests) |
| §7.5 PackConfigs integration (5 scenarios) | T5 step 1 (5 sub-tests) |
| §7.6 Loader round-trip | T6 |
| §7.7 Deviation-tag pins | T7 |

All 13 requirements covered. No gaps.

**Placeholder scan:** searched for "TBD"/"TODO"/"fill in"/"appropriate"/"similar to" — clean.

**Type consistency:** `paramLookups` (struct) / `paramPF` / `lk` / `pf` used consistently. `packParamConfigs` returns `(server, client *PackedData, err error)` throughout. `lookupParamValue` returns `(any, error)` throughout. `paramIndexOrErr` returns `(int, error)` throughout. No drift.

**Spec-divergence check:** the spec §3.1 mentioned "20 TS arms"; this plan's T3 test enumerates 13 typed-id branches + 4 primitives + 2 hardcoded slices + INTERFACE colon-reject (special case) + null sentinel + unsupported-type fall-through. Counts agree.

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-13-nai-194-param-packer.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — fresh subagent per task, two-stage Sonnet reviewer per `superpowers_code_reviewer_model`, controller pre-flight per `controller_preflight`. Matches NAI-191/192/193 cadence.

**2. Inline Execution** — execute tasks in this session using executing-plans; batch with checkpoints.

**Which approach?**
