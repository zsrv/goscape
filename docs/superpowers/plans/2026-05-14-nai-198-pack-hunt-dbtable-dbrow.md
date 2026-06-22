# NAI-198 — `.hunt` + `.dbtable` + `.dbrow` packer slice — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the final three per-config packer branches from TS `tools/pack/config/{HuntConfig,DbTableConfig,DbRowConfig}.ts` onto the NAI-191–197 `PackShared` infrastructure. Closes the per-config layer (18/18 TS configs ported).

**Architecture:** Three new server-only-freshness-gated `pkg/pack/<config>.go` files (parser + packer per config) plus one shared `csv.go` helper. Additive extension of `pkg/pack/pack_configs.go`: two new lazy `ensureFooPack` helpers (`DbTable`, `DbRow`); two lazy-promotions (`invPack` from inline→`ensureInvPack`, `paramPack` from eager→`ensureParamPack`); one paired `.dbtable`+`.dbrow` branch (joint freshness gate, mid-pipeline `LoadDbTableTypes` between them) and one `.hunt` branch; doc-comment count phrasing extended from six→nine server-only freshness-gated branches. Atomic rewrite of `TestPackConfigs_FifteenConfigsLand` → `TestPackConfigs_EighteenConfigsLand`. Three new round-trip tests. One new deviation-pin file.

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` + `pkg/io/jagfile` + NAI-191–197 `pkg/pack` foundation + `pkg/objtype` (`LoadDbTableTypes`, `LoadDbRowTypes`, `LoadHuntTypes`, `ScriptVarTypeFromName`, `DbTableFlag*`).

**Spec:** `docs/superpowers/specs/2026-05-14-nai-198-pack-hunt-dbtable-dbrow-design.md` (commit `ed7cfd9`).
**HEAD at plan-write:** `ed7cfd9`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/pack/*_test.go`: bare `if err != nil { t.Fatal(err) }`, `bytes.Equal` for byte comparison, `t.Fatalf("got % x, want % x", got, want)` for byte diffs, `t.TempDir()` for fixture roots, `ClearFsCache()` before tests that mutate the FS.
- **Existing helpers in `pkg/pack`** (use, do NOT redefine):
  - `writeFile(t *testing.T, path, content string)` — `constants_test.go:10`
  - `newTestPF(packType string, entries map[int]string) *PackFile` — `param_test.go:54`
  - `scanPkgPack(t *testing.T) string` — `nai196_deviation_pins_test.go` (concatenated `.go` content of `pkg/pack/` excluding `_test.go`)
  - `lookupParamValue(typ objtype.ScriptVarType, value string, lk *paramLookups) (any, error)` — `param.go:92`. Returns `int` for non-STRING, `string` for STRING; returns `(int(-1), nil)` for STRING-`null` is `("", nil)`.
- **Modern Go** (per `[[use-modern-go]]`): `for id := range pf.Max`, `slices.Index`, `slices.Equal`, `strconv.ParseInt(_, 0, 64)`, `strings.HasPrefix`, `strings.HasSuffix`.
- **Identifier conventions** (mirroring NAI-196/197):
  - Per-config files: `hunt.go`, `dbtable.go`, `dbrow.go`, plus shared `csv.go`.
  - Parsers (closure-bound when registry deps exist): `parseDbTableConfig(key, value)` (no deps), `parseDbRowConfigFor(dbtablePack)`, `parseHuntConfigFor(categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack)`. Each returns `ParseFn` (`func(key, value string) (ConfigValue, bool, error)`).
  - Packers: `packDbTableConfigs(configs, dbtablePack, lk) (*PackedData, error)`, `packDbRowConfigs(configs, dbrowPack, dbtableTypes, lk) (*PackedData, error)`, `packHuntConfigs(configs, huntPack) (*PackedData, error)`. **Single-return server-only signature** (no client component) — TS allocates `client = new PackedData(...)` and calls `client.next()` per id but never `client.p<N>`; goscape omits the dead client buffer entirely per `NAI-195-D-DEADBRANCH-OMITTED` precedent.
  - Orchestrator helpers: `packAndSaveDbTable`, `packAndSaveDbRow`, `packAndSaveHunt`. **All take NO `clientJag` parameter** (server-only configs).
  - Registry helpers: `ensureDbTablePack`, `ensureDbRowPack` (NEW); `ensureInvPack`, `ensureParamPack` (NEW lazy promotions); existing `ensureCategoryPack`, `ensureHuntPack`, `ensureLocPack`, `ensureNpcPack`, `ensureObjPack` reused.
  - Shared error helper: `packStepError(debugname, format string, args ...any) error` — small helper in `pkg/pack/csv.go` (or `config_value.go`; see T1 step 1.2). Mirrors TS `packStepError(debugname, message)`.
- **TS-fidelity discipline** (per `[[true_to_ts_gate]]`): per-config tasks do NOT codify the 60+ HuntConfig `find_newmode` switch arms, Hunt opcode mutex-predicate tables, or NPCMode string-to-constant tables. Each task block cites the TS file:line range and instructs the implementer to read TS directly.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `ed7cfd9`:

| Premise | Verification |
|---|---|
| `pkg/objtype.LoadDbTableTypes(dir string) (*DbTableTypeConfigs, error)` reads `<dir>/server/dbtable.dat` | ✅ `dbtabletype.go:155-161` |
| `pkg/objtype.LoadDbRowTypes(dir string) (*DbRowTypeConfigs, error)` reads `<dir>/server/dbrow.dat` | ✅ `dbrowtype.go:88-95` |
| `pkg/objtype.LoadHuntTypes(dir string) (*HuntTypeConfigs, error)` reads `<dir>/server/hunt.dat` | ✅ `hunttype.go:168-179` |
| `pkg/objtype.ScriptVarTypeFromName(name) (ScriptVarType, bool)` exists; equivalent to TS `ScriptVarType.getTypeChar` | ✅ `scriptvartype.go:43-97` |
| `pkg/objtype.DbTableFlag{Indexed,Required,List,Clientside}` constants exist (`0x1`, `0x2`, `0x4`, `0x8`) | ✅ `dbtabletype.go:14-19` |
| `*DbTableTypeConfigs` exposes `.Configs []*DbTableType` (slice indexed by id) — no `.Get(id)` method, direct indexing | ✅ `dbtabletype.go:149-152` |
| `*DbTableType` fields: `Types [][]ScriptVarType`, `ColumnNames []string`, `Props []uint8` | ✅ `dbtabletype.go:30-37` |
| `pkg/pack.PackFile.GetByName(name) int` returns `-1` on miss; `GetByID(id) string` returns `""` on miss; `Max` is a struct field | ✅ `packfile.go:35,162,188,192` |
| `pkg/pack.ParseFn = func(key, value string) (ConfigValue, bool, error)` | ✅ `read_typed.go:19` |
| `pkg/pack.IsConfigBoolean(value) bool`, `GetConfigBoolean(value) bool` | ✅ `config_value.go:23-37` |
| `pkg/pack.lookupParamValue(typ, value, lk)` returns `(any, error)` — `int` for non-STRING, `string` for STRING | ✅ `param.go:92-179` |
| No `pkg/pack.parseCsv` helper currently exists | ✅ `grep -rn "parseCsv\|parseCSV" pkg/` → empty |
| No `pkg/pack.packStepError` helper currently exists | ✅ `grep -rn "packStepError\|PackStepError" pkg/` → only TS-comment references (`inv.go:81`) |
| `pkg/pack/pack_configs.go` current state: `huntPack` is lazy (line 103,187-197); `paramPack` is eager at line 256; `invPack` is inline at line 294 (inside `.inv` branch) | ✅ `pack_configs.go:103,256,294` |
| `NAI-192-D-NO-SRC-NO-OP` doc-comment currently says "the six server-only freshness-gated branches" and is NOT pinned by `nai192_deviation_pins_test.go` or `nai197_deviation_pins_test.go` | ✅ `pack_configs.go:49-52`; pin files do not assert the count |
| Existing `TestPackConfigs_FifteenConfigsLand` at `pack_configs_test.go:410-536` covers 15 configs, 9 client-jag entry pairs, post-NAI-197 ordering | ✅ verified |
| `setupPackRoots` in `loc_roundtrip_test.go:63` already stubs `dbrow.pack`; `dbtable.pack` is also already stubbed in `pack_configs_test.go:442-443` (interface/synth/dbrow loop); `hunt.pack` is already stubbed at `pack_configs_test.go:439` | ✅ verified |

**TS-side premises** (verified by reading the three TS files end-to-end):

| TS premise | Source line |
|---|---|
| `.dbtable` parser claims: `column` (raw string), `default` (raw string); zero registry deps; `stringKeys`/`numberKeys`/`booleanKeys` ALL empty | `DbTableConfig.ts:6-76` |
| `.dbtable` packer emits opcodes 1/251/252 ONLY when `columns.length > 0` (`DbTableConfig.ts:144,181,190`); 250-trailer gated on `debugname.length > 0`; server-only (`client.next()` per id, no `client.p<N>`) | `DbTableConfig.ts:78-224` |
| `.dbtable` validation throws via `packStepError`: INDEXED-without-REQUIRED (line 117), unknown-default-column (line 133), default-on-REQUIRED-column (line 137) | `DbTableConfig.ts:116-138` |
| `.dbtable` flag byte: `flags = i` (column index in low bits) `\| 0x80` when default exists; per-column type-array prefixed with `p1(types.length)`; defaults block prefixed with `p1(1)` (= 1 field count); per-typed-value emit via `lookupParamValue` (STRING→`pjstr`, else→`p4`); column end-marker is `p1(255)` | `DbTableConfig.ts:148-178` |
| `.dbrow` parser claims: `table` (→ `DbTablePack.getByName`), `data` (raw string); only `DbTablePack` dep at parse-time; `stringKeys`/`numberKeys`/`booleanKeys` ALL empty | `DbRowConfig.ts:7-82` |
| `.dbrow` packer resolves `table` to `*DbTableType` via `DbTableType.get(value as number)` (runtime cache populated by mid-pipeline `DbTableType.load`); throws `packStepError` if no table found (line 103); collects `data=` entries; emits opcode 3 + per-column tuple + opcode 4 + `p2(table.id)` + 250-trailer | `DbRowConfig.ts:84-185` |
| `.dbrow` validation throws via `packStepError`: REQUIRED-column-missing-data (line 142), non-LIST-column-with-multi-data (line 146), `lookupParamValue` returns null (line 156) | `DbRowConfig.ts:141-158` |
| `.hunt` parser claims: `rate` (number 1-255); `type`/`check_vis`/`check_nottoostrong`/`check_notbusy`/`find_keephunting`/`find_newmode`/`nobodynear`/`check_afk` (enum switches); `check_notcombat` (varp via `%name`); `check_notcombat_self` (varn via `%name`); `check_category`/`check_npc`/`check_obj`/`check_loc` (registry refs); `check_inv` (3-CSV, `{inv, obj, condition, val}`); `check_invparam` (3-CSV, `{inv, param, condition, val}`); `extracheck_var` (2-CSV, `{varp, condition, val}`); `stringKeys`/`booleanKeys` empty | `HuntConfig.ts:9-381` |
| `.hunt` packer: opcodes 1-17 plus `18+extracheckVarsCount` (max 3 → opcodes 18,19,20); per-opcode default-skip gates (e.g., opcode 7 only when `value !== HuntNobodyNear.PAUSEHUNT`); mutex predicates on opcodes 12-17 (each `config.every(x => ...)` with **different** exclusion lists per arm); 250-trailer per id; server-only | `HuntConfig.ts:383-545` |
| **TS bug at HuntConfig.ts:201-202**: `case 'opobj2': return NpcMode.OPOBJ1;` — `'opobj2'` maps to `NpcMode.OPOBJ1` (= goscape `NPCModeOpObj1`, value 27), not `NpcMode.OPOBJ2` (value 28). Reader-confirmed at HEAD by reading TS lines 199-204 directly | `HuntConfig.ts:201-202` |
| Goscape NPCMode constants: `NPCModeOpObj1 = 27`, `NPCModeOpObj2 = 28` | `npctype.go:83-84` |

All ⚠️ rows from spec §9 resolved at plan-write. Implementer tasks do not need to re-trace registry lookup APIs or loader signatures.

---

## Pre-flight resolution of spec §9 ⚠️ risk-register rows

| Row | Resolution |
|---|---|
| R1 (loader signatures) | ✅ All three are `func Load*Types(dir string) (*XConfigs, error)` reading `<dir>/server/<type>.dat`. T5 wiring uses `outDir` (parent), NOT `serverOut`. |
| R2 (paramPack/invPack promotion) | ✅ Plan-author chose **(a) lazy-promote both**. T1 promotes `paramPack` from eager-at-top to `ensureParamPack` closure, and `invPack` from inline-inside-`.inv`-branch to `ensureInvPack` closure. Net effect: Hunt's wiring at T5 calls `ensureParamPack()` and `ensureInvPack()` symmetrically with the other 6 deps. |
| R3 (TS bug `opobj2 → OPOBJ1`) | ✅ Port literally per `[[true_to_ts_gate]]`. New deviation tag `NAI-198-D-HUNT-OPOBJ2-TS-BUG`. T4 pins via byte-pin (asserts emitted byte = 27) and T7 pins via grep (asserts tag identifier present in `hunt.go`). |
| R4 (`ScriptVarType.getTypeChar` equivalent) | ✅ `objtype.ScriptVarTypeFromName(name)` exists at `scriptvartype.go:43-97`. Returns `(ScriptVarType, bool)`. T2 parses `column=name,type,PROPS` using this. |
| R5 (DbTableType accessor shape) | ✅ `*DbTableTypeConfigs.Configs []*DbTableType` direct-indexed by id. No `.Get(id)` method exists; T3 packer uses `dbtableTypes.Configs[tableID]` with bounds check. |
| R6 (`find_newmode` 60-line switch) | ✅ T4 task block cites `HuntConfig.ts:138-261` and instructs implementer to port the chain as a Go `switch`. Plan-author provides TWO concrete example mappings (`opplayer1` and `opobj2`-bug case) but NOT the full 60-arm table. |
| R7 (`parseCsv` reuse) | ✅ No existing `parseCsv` in `pkg/`. T1 creates `pkg/pack/csv.go`. |
| R8 (`packStepError` reuse) | ✅ No existing `packStepError` in `pkg/pack/`. T1 adds a tiny helper `packStepError(debugname, format, args...) error` that returns `fmt.Errorf("[%s] "+format, append([]any{debugname}, args...)...)`. |
| R9 (joint freshness gate semantics) | ✅ Pre-traced four corner cases (neither/dbtable-only/dbrow-only/both dirty). T5 wires `||` of two `ShouldBuild` calls inside an outer `||` of two `GetLatestModified > 0` guards. T5 byte-pin test covers all four corners. |
| R10 (`LoadDbTableTypes(outDir)` NOT `(serverOut)`) | ✅ Per `[[load_param_types_dir_arg]]`. Confirmed: `dbtabletype.go:156` joins `dir/server/dbtable.dat` — so `dir = outDir`. T5 wiring uses `outDir`. |
| R11 (`%` prefix on varp/varn refs) | ✅ T4 byte-pin test covers both `check_notcombat=%foo` and `check_notcombat_self=%bar` paths. |
| R12 (NAI-197 pin file dependency on "six" phrasing) | ✅ Verified: `nai197_deviation_pins_test.go` does NOT pin the `NAI-192-D-NO-SRC-NO-OP` count phrasing — only the `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` scope listing. NAI-197 pin file is unaffected by the 6→9 change. T5 still updates the production doc-comment count phrasing; T7 adds a NEW presence pin for the 9-branch listing. |
| R13 (round-trip field selection) | ✅ T6 picks 3 fields per config based on byte-pin coverage gaps. |
| R14 (PackFile.Max empty-file semantics) | ✅ Inherited from NAI-196. No re-verification needed. |
| R15 (NAI-191 follow-up #2 retirement timing) | ✅ Retired at NAI-198 close per §7.2. T8 documents. |

---

## File inventory

```
pkg/pack/
  csv.go                                    NEW    (parseCsv + packStepError helpers — shared)
  csv_test.go                               NEW    (parseCsv table-driven tests)
  dbtable.go                                NEW    (parseDbTableConfig + packDbTableConfigs)
  dbtable_test.go                           NEW    (byte-pin tests)
  dbtable_roundtrip_test.go                 NEW    (PackConfigs → LoadDbTableTypes round-trip)
  dbrow.go                                  NEW    (parseDbRowConfigFor + packDbRowConfigs)
  dbrow_test.go                             NEW    (byte-pin tests)
  dbrow_roundtrip_test.go                   NEW    (PackConfigs → LoadDbRowTypes round-trip)
  hunt.go                                   NEW    (parseHuntConfigFor + packHuntConfigs, incl. NAI-198-D-HUNT-OPOBJ2-TS-BUG tag-comment)
  hunt_test.go                              NEW    (byte-pin tests incl. OPOBJ2 TS-bug pin)
  hunt_roundtrip_test.go                    NEW    (PackConfigs → LoadHuntTypes round-trip)
  pack_configs.go                           MODIFY (lazy-promote paramPack+invPack; add 2 new lazy ensureFooPack; add 3 packAndSave functions; add 2 new branches: paired .dbtable/.dbrow + .hunt; extend NAI-192-D-NO-SRC-NO-OP doc-comment 6→9; audit adjacent paragraphs for stale counts)
  pack_configs_test.go                      MODIFY (in-place rewrite of TestPackConfigs_FifteenConfigsLand → TestPackConfigs_EighteenConfigsLand)
  nai198_deviation_pins_test.go             NEW    (3 pins: NAI-192-D scope-extension presence; NAI-198-D-HUNT-OPOBJ2-TS-BUG presence; NAI-196-D-UNCONDITIONAL-CLIENT-PACK non-growth absence)
```

---

## Task overview

| T | Subject | Test files | Production files | Est. commits |
|---|---|---|---|---|
| T1 | Shared helpers (`parseCsv`, `packStepError`) + lazy-promote `paramPack`/`invPack` + add `ensureDbTablePack`/`ensureDbRowPack` — additive, no new callers yet | `csv_test.go` | `csv.go`, `pack_configs.go` | 1–2 |
| T2 | `.dbtable` parser + packer + byte-pin tests | `dbtable_test.go` | `dbtable.go` | 1–2 |
| T3 | `.dbrow` parser + packer + byte-pin tests | `dbrow_test.go` | `dbrow.go` | 1–2 |
| T4 | `.hunt` parser + packer + byte-pin tests (largest task — 545 TS LOC; OPOBJ2 TS-bug deviation tag) | `hunt_test.go` | `hunt.go` | 2 |
| T5 | `PackConfigs` wiring: paired `.dbtable`+`.dbrow` branch with mid-pipeline `LoadDbTableTypes`; `.hunt` branch; atomic rename `_FifteenConfigsLand` → `_EighteenConfigsLand`; doc-comment count update 6→9 + adjacent-paragraph audit | `pack_configs_test.go` (in-place) | `pack_configs.go` | 1 |
| T6 | Round-trip tests for all 3 configs | `dbtable_roundtrip_test.go`, `dbrow_roundtrip_test.go`, `hunt_roundtrip_test.go` | (none — exercises T1–T5) | 1–2 |
| T7 | Deviation-tag pins | `nai198_deviation_pins_test.go` | (none) | 1 |
| T8 | NAI-191 follow-up #2 (`LoadFile` nil-vs-empty) retirement docs — no production change | (none) | (memory + tracker) | 1 |

---

## Task 1: Shared helpers (`parseCsv`, `packStepError`) + lazy-promotions

**Files:**
- Create: `pkg/pack/csv.go`
- Create: `pkg/pack/csv_test.go`
- Modify: `pkg/pack/pack_configs.go` (lazy-promote `paramPack` + `invPack`; add 2 new `ensureFooPack`)

This task is **additive — no new callers yet**. T2/T3/T4/T5 wire the helpers. The build must remain green at end of T1.

### Step 1.1: Write `parseCsv` tests (TDD-red)

- [ ] **Step 1.1: Create `pkg/pack/csv_test.go` with table-driven tests**

```go
package pack

import (
	"slices"
	"testing"
)

func TestParseCsv(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{""}},
		{"single", "abc", []string{"abc"}},
		{"two", "a,b", []string{"a", "b"}},
		{"three", "a,b,c", []string{"a", "b", "c"}},
		{"trailing-comma", "a,", []string{"a", ""}},
		{"leading-comma", ",a", []string{"", "a"}},
		{"quoted-comma", `"a,b",c`, []string{"a,b", "c"}},
		{"only-quotes", `"abc"`, []string{"abc"}},
		{"empty-quoted", `""`, []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCsv(tc.in)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("parseCsv(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 1.2: Run tests, confirm they fail (TDD red)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseCsv
```
Expected: FAIL with `undefined: parseCsv`.

### Step 1.3: Create `pkg/pack/csv.go`

- [ ] **Step 1.3: Write `csv.go`**

```go
package pack

import "fmt"

// parseCsv splits s on commas, respecting quoted fields. Mirrors the
// identical helpers duplicated in TS DbTableConfig.ts:6-26 and
// DbRowConfig.ts:7-27. Quotes are stripped from the output (TS toggles
// inQuotes on each '"' but does not emit the quote char).
//
// Always returns at least one element (the suffix after the last
// unquoted comma).
//
// TS source: tools/pack/config/DbTableConfig.ts:6-26.
func parseCsv(s string) []string {
	result := []string{}
	current := []byte{}
	inQuotes := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ',' && !inQuotes:
			result = append(result, string(current))
			current = current[:0]
		case c == '"':
			inQuotes = !inQuotes
		default:
			current = append(current, c)
		}
	}
	result = append(result, string(current))
	return result
}

// packStepError formats a typed error for per-config pack failures.
// Mirrors TS packStepError(debugname, message) — debugname appears in
// brackets prefix, followed by the format-applied message.
//
// TS source: tools/pack/config/PackShared.ts (packStepError export).
func packStepError(debugname, format string, args ...any) error {
	return fmt.Errorf("["+debugname+"] "+format, args...)
}
```

- [ ] **Step 1.4: Run tests, confirm pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseCsv
```
Expected: PASS.

### Step 1.5: Lazy-promote `paramPack` and `invPack`, add 2 new `ensureFooPack`

- [ ] **Step 1.5: Modify `pkg/pack/pack_configs.go`**

Two changes in the same edit:

**(a)** In the `var (...)` block at `pack_configs.go:95-109`, ADD four new `*PackFile` declarations (`paramPack`, `invPack`, `dbtablePack`, `dbrowPack`) — keep the existing 13:

```go
var (
	lk           *paramLookups
	objPack      *PackFile
	seqPack      *PackFile
	locPack      *PackFile
	npcPack      *PackFile
	modelPack    *PackFile
	categoryPack *PackFile
	huntPack     *PackFile
	texturePack  *PackFile
	animPack     *PackFile
	floPack      *PackFile
	spotanimPack *PackFile
	idkPack      *PackFile
	paramPack    *PackFile // NAI-198: lazy-promoted from eager at top of fn
	invPack      *PackFile // NAI-198: lazy-promoted from inline inside .inv branch
	dbtablePack  *PackFile // NAI-198
	dbrowPack    *PackFile // NAI-198
)
```

**(b)** Immediately after the existing `ensureIdkPack` closure (around line 252), ADD four new closures:

```go
ensureParamPack := func() error {
	if paramPack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "param", nil)
	if err != nil {
		return err
	}
	paramPack = pf
	return nil
}
ensureInvPack := func() error {
	if invPack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "inv", nil)
	if err != nil {
		return err
	}
	invPack = pf
	return nil
}
ensureDbTablePack := func() error {
	if dbtablePack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "dbtable", nil)
	if err != nil {
		return err
	}
	dbtablePack = pf
	return nil
}
ensureDbRowPack := func() error {
	if dbrowPack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "dbrow", nil)
	if err != nil {
		return err
	}
	dbrowPack = pf
	return nil
}
```

**(c)** REPLACE the eager `paramPack` construction at `pack_configs.go:256-259`:

```go
// BEFORE (delete these 4 lines):
paramPack, err := NewPackFile(srcDir, "param", nil)
if err != nil {
	return err
}

// AFTER (replace with):
if err := ensureParamPack(); err != nil {
	return err
}
```

**(d)** REPLACE the inline `invPack` construction at `pack_configs.go:291-301` (inside `.inv` branch):

```go
// BEFORE (delete these 5 lines from inside the .inv branch):
if err := ensureObjPack(); err != nil {
	return err
}
invPack, err := NewPackFile(srcDir, "inv", nil)
if err != nil {
	return err
}
if err := packAndSaveInv(srcDir, serverOut, invPack, objPack, constants); err != nil {

// AFTER:
if err := ensureObjPack(); err != nil {
	return err
}
if err := ensureInvPack(); err != nil {
	return err
}
if err := packAndSaveInv(srcDir, serverOut, invPack, objPack, constants); err != nil {
```

- [ ] **Step 1.6: Run full pkg/pack test suite, confirm green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...
```
Expected: PASS (all existing tests). T1 is purely additive + zero-behavior-change refactor.

- [ ] **Step 1.7: Commit**

```bash
git add pkg/pack/csv.go pkg/pack/csv_test.go pkg/pack/pack_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-198 T1 — shared csv.go + lazy-promote paramPack/invPack

Adds parseCsv and packStepError helpers in pkg/pack/csv.go (shared
between forthcoming .dbtable and .dbrow packers). Lazy-promotes
paramPack and invPack from inline/eager to ensureFooPack closures, and
adds ensureDbTablePack/ensureDbRowPack stubs for the upcoming .dbtable
and .dbrow branches. Purely additive + zero-behavior-change refactor.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `.dbtable` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/dbtable.go`
- Create: `pkg/pack/dbtable_test.go`

**TS source to read end-to-end before coding**: `tools/pack/config/DbTableConfig.ts:1-224` (224 LOC).

### Step 2.1: Read TS source

- [ ] **Step 2.1: Read `tools/pack/config/DbTableConfig.ts:1-224` completely**

Note the following while reading:
- Parser (lines 28-76) has NO registry deps — only routes `column` and `default` as raw strings. **All three dead-key arrays (`stringKeys`, `numberKeys`, `booleanKeys`) are empty** — apply `NAI-195-D-DEADBRANCH-OMITTED`.
- Packer (lines 78-224) emits opcodes 1/251/252 ONLY when `columns.length > 0` (lines 144, 181, 190).
- Flag byte for opcode 1 per-column: low bits = column index `i`, bit `0x80` = "has default".
- The defaults block (line 162) writes `p1(1)` as field-count (always 1), then per-typed-value via `lookupParamValue`.
- Property bits (opcode 252): INDEXED=0x1, REQUIRED=0x2, LIST=0x4, CLIENTSIDE=0x8 — match `objtype.DbTableFlag*` constants.

### Step 2.2: Write byte-pin tests (TDD-red)

- [ ] **Step 2.2: Create `pkg/pack/dbtable_test.go`**

Write tests covering (each is one `t.Run` subtest):

```go
package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// buildParamLookups constructs a paramLookups with empty PackFiles for
// the table types not exercised by the test. Used when the dbtable
// fixtures use only primitive types (int/string/boolean).
func buildParamLookupsForDbTableTest(t *testing.T) *paramLookups {
	t.Helper()
	lk := &paramLookups{}
	for _, dst := range []**PackFile{
		&lk.enumPF, &lk.objPF, &lk.locPF, &lk.interfacePF, &lk.structPF,
		&lk.categoryPF, &lk.spotanimPF, &lk.npcPF, &lk.invPF, &lk.synthPF,
		&lk.seqPF, &lk.varpPF, &lk.dbrowPF,
	} {
		*dst = newTestPF("dummy", map[int]string{})
	}
	return lk
}

func TestPackDbTableConfigs_EmptyConfigDebugnameOnly(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_empty"})
	configs := map[string][]ConfigLine{
		"t_empty": {},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// columns.length == 0 → no opcode 1/251/252 emission.
	// 250-trailer + pjstr("t_empty") (8 bytes: opcode + len byte + 7 bytes "t_empty" — pjstr uses 0-terminator).
	// Verify by checking that the only bytes for id 0 are: 250 + "t_empty\0".
	// (Exact byte assertion: use pd.Dat slice up to the first Next boundary.)
	got := pd.Dat
	// Build expected: opcode 250, then "t_empty" terminated.
	want := []byte{250}
	want = append(want, "t_empty\x00"...)
	if !bytes.Equal(got, want) {
		t.Fatalf("got % x, want % x", got, want)
	}
}

func TestPackDbTableConfigs_SingleColumnNoDefault(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_one"})
	configs := map[string][]ConfigLine{
		"t_one": {
			{Key: "column", Value: "id,int"},
		},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// Expected layout for id 0:
	//   opcode 1, total-cols=1, flags=0 (i=0, no default), types-len=1, type=105('i'), 255 (end-tuple)
	//   opcode 251, count=1, pjstr("id")
	//   opcode 252, count=1, props=0
	//   opcode 250, pjstr("t_one")
	want := []byte{
		1, 1, 0x00, 1, 105, 255,
		251, 1, 'i', 'd', 0x00,
		252, 1, 0,
		250, 't', '_', 'o', 'n', 'e', 0x00,
	}
	if !bytes.Equal(pd.Dat, want) {
		t.Fatalf("got % x, want % x", pd.Dat, want)
	}
}

func TestPackDbTableConfigs_SingleColumnWithIntDefault(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_def"})
	configs := map[string][]ConfigLine{
		"t_def": {
			{Key: "column", Value: "score,int"},
			{Key: "default", Value: "score,42"},
		},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// Expected layout:
	//   opcode 1, total-cols=1, flags=0x80, types-len=1, type=105, field-count=1, p4(42)=0,0,0,42, 255
	//   opcode 251, 1, pjstr("score")
	//   opcode 252, 1, props=0
	//   opcode 250, pjstr("t_def")
	want := []byte{
		1, 1, 0x80, 1, 105, 1, 0, 0, 0, 42, 255,
		251, 1, 's', 'c', 'o', 'r', 'e', 0x00,
		252, 1, 0,
		250, 't', '_', 'd', 'e', 'f', 0x00,
	}
	if !bytes.Equal(pd.Dat, want) {
		t.Fatalf("got % x, want % x", pd.Dat, want)
	}
}

func TestPackDbTableConfigs_AllPropertyBits(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_props"})
	// REQUIRED+INDEXED+LIST+CLIENTSIDE column. Cannot have default (REQUIRED rule).
	configs := map[string][]ConfigLine{
		"t_props": {
			{Key: "column", Value: "key,int,INDEXED,REQUIRED,LIST,CLIENTSIDE"},
		},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// Check opcode 252 emits 0x0F (1|2|4|8).
	if !bytes.Contains(pd.Dat, []byte{252, 1, 0x0F}) {
		t.Fatalf("expected opcode 252 + count=1 + props=0x0F, got %x", pd.Dat)
	}
}

func TestPackDbTableConfigs_IndexedWithoutRequiredErrors(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_bad"})
	configs := map[string][]ConfigLine{
		"t_bad": {
			{Key: "column", Value: "x,int,INDEXED"},
		},
	}
	_, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err == nil {
		t.Fatal("want error for INDEXED without REQUIRED, got nil")
	}
	if !strings.Contains(err.Error(), "t_bad") || !strings.Contains(err.Error(), "INDEXED") {
		t.Fatalf("err=%q, want substrings 't_bad' and 'INDEXED'", err)
	}
}

func TestPackDbTableConfigs_DefaultOnRequiredErrors(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_bad"})
	configs := map[string][]ConfigLine{
		"t_bad": {
			{Key: "column", Value: "x,int,REQUIRED"},
			{Key: "default", Value: "x,7"},
		},
	}
	_, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err == nil {
		t.Fatal("want error for default on REQUIRED, got nil")
	}
	if !strings.Contains(err.Error(), "t_bad") || !strings.Contains(err.Error(), "REQUIRED") {
		t.Fatalf("err=%q, want substrings 't_bad' and 'REQUIRED'", err)
	}
}

func TestPackDbTableConfigs_UnknownDefaultColumnErrors(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_bad"})
	configs := map[string][]ConfigLine{
		"t_bad": {
			{Key: "column", Value: "x,int"},
			{Key: "default", Value: "z,7"},
		},
	}
	_, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err == nil {
		t.Fatal("want error for unknown default column, got nil")
	}
	if !strings.Contains(err.Error(), "t_bad") {
		t.Fatalf("err=%q, want debugname 't_bad'", err)
	}
}

func TestParseDbTableConfig_AcceptsColumnAndDefault(t *testing.T) {
	v, claimed, err := parseDbTableConfig("column", "id,int")
	if err != nil || !claimed {
		t.Fatalf("column key: v=%v claimed=%v err=%v", v, claimed, err)
	}
	if s, ok := v.(string); !ok || s != "id,int" {
		t.Fatalf("column value=%v, want raw string", v)
	}
	v, claimed, err = parseDbTableConfig("default", "x,42")
	if err != nil || !claimed {
		t.Fatalf("default key: v=%v claimed=%v err=%v", v, claimed, err)
	}
	if s, ok := v.(string); !ok || s != "x,42" {
		t.Fatalf("default value=%v, want raw string", v)
	}
}

func TestParseDbTableConfig_UnknownKey(t *testing.T) {
	v, claimed, err := parseDbTableConfig("foo", "bar")
	if claimed || err != nil || v != nil {
		t.Fatalf("got v=%v claimed=%v err=%v, want all-zero", v, claimed, err)
	}
}

// Suppress import-unused complaints for the objtype import used by future round-trip tests.
var _ = objtype.DbTableFlagIndexed
```

- [ ] **Step 2.3: Run tests, confirm they fail (TDD red)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackDbTableConfigs
```
Expected: FAIL with `undefined: packDbTableConfigs` / `undefined: parseDbTableConfig`.

### Step 2.4: Implement `pkg/pack/dbtable.go`

- [ ] **Step 2.4: Create `pkg/pack/dbtable.go`**

Implementation guidance:
- `parseDbTableConfig(key, value string) (ConfigValue, bool, error)` — switch on `key` ∈ `{"column", "default"}`, return `(value, true, nil)` for both; else `(nil, false, nil)`. Per `NAI-195-D-DEADBRANCH-OMITTED`, all three TS dead-key arrays are empty — no other branches.
- `packDbTableConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (*PackedData, error)`:
  - Loop `for id := range pf.Max`.
  - `name := pf.GetByID(id)`; `cfg, ok := configs[name]`.
  - If `ok`: two-pass walk — pass 1 collects columns (`parseCsv` + classify uppercase-parts as properties, lowercase-parts via `objtype.ScriptVarTypeFromName`), pass 2 collects defaults (CSV-split, index by column name). Apply INDEXED-without-REQUIRED, unknown-default-column, default-on-REQUIRED validations via `packStepError(name, ...)`.
  - Emit opcodes 1/251/252 ONLY when `len(columns) > 0`. Opcode 1 layout per TS lines 145-179 (flags byte, types-array, optional defaults block). Opcode 251 = column-name list. Opcode 252 = property-bits via `objtype.DbTableFlag*`.
  - 250-trailer + `pd.PJStr(name)` when `len(name) > 0`.
  - `pd.Next()` per id.
- Use `lookupParamValue(typ, value, lk)` for typed default-value resolution; STRING → `pd.PJStr`, else → `pd.P4(uint32(intVal))`.

Use `column` data type representation in-memory:

```go
type dbTableColumn struct {
	name       string
	types      []objtype.ScriptVarType
	properties []string // raw uppercase tokens; used for opcode 252 bit-flags and INDEXED/REQUIRED gates
}
```

Skeleton (DO NOT copy verbatim — read TS source and adapt):

```go
package pack

import (
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseDbTableConfig is the per-key=value parser for .dbtable config
// blocks. Both 'column' and 'default' return their raw values; CSV
// parsing is deferred to packDbTableConfigs because column resolution
// depends on column-list state.
//
// NAI-195-D-DEADBRANCH-OMITTED: TS DbTableConfig.ts:29-31 declares
// empty stringKeys/numberKeys/booleanKeys arrays — all branches dead.
//
// TS source: tools/pack/config/DbTableConfig.ts:28-76.
func parseDbTableConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "column", "default":
		return value, true, nil
	}
	return nil, false, nil
}

type dbTableColumn struct {
	name       string
	types      []objtype.ScriptVarType
	properties []string
}

// packDbTableConfigs walks every id, two-pass-walks each per-id config
// (columns then defaults), and emits opcodes 1/251/252 + 250-trailer
// per TS DbTableConfig.ts:78-224.
//
// Server-only — TS allocates a client PackedData but never writes to
// it. Goscape omits the client buffer entirely.
//
// TS source: tools/pack/config/DbTableConfig.ts:78-224.
func packDbTableConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			columns := []dbTableColumn{}
			defaults := map[int][]string{} // column index → raw default values

			// Pass 1: columns
			for _, line := range cfg {
				if line.Key != "column" {
					continue
				}
				parts := parseCsv(line.Value.(string))
				if len(parts) == 0 {
					continue
				}
				col := dbTableColumn{name: parts[0]}
				for _, part := range parts[1:] {
					if part == strings.ToUpper(part) && part != "" {
						col.properties = append(col.properties, part)
					} else {
						t, ok := objtype.ScriptVarTypeFromName(part)
						if !ok {
							return nil, packStepError(name, "unknown column type %q", part)
						}
						col.types = append(col.types, t)
					}
				}
				hasIndexed := false
				hasRequired := false
				for _, p := range col.properties {
					if p == "INDEXED" {
						hasIndexed = true
					}
					if p == "REQUIRED" {
						hasRequired = true
					}
				}
				if hasIndexed && !hasRequired {
					return nil, packStepError(name, "INDEXED columns must be marked REQUIRED as well")
				}
				columns = append(columns, col)
			}

			// Pass 2: defaults
			for _, line := range cfg {
				if line.Key != "default" {
					continue
				}
				parts := parseCsv(line.Value.(string))
				if len(parts) == 0 {
					continue
				}
				colName := parts[0]
				values := parts[1:]
				colIdx := -1
				for i, c := range columns {
					if c.name == colName {
						colIdx = i
						break
					}
				}
				if colIdx == -1 {
					return nil, packStepError(name, "unknown default column %q", colName)
				}
				for _, p := range columns[colIdx].properties {
					if p == "REQUIRED" {
						return nil, packStepError(name, "%s cannot have a default value because it is marked REQUIRED", colName)
					}
				}
				defaults[colIdx] = values
			}

			if len(columns) > 0 {
				// Opcode 1: column-type block.
				pd.P1(1)
				pd.P1(uint8(len(columns)))
				for i, col := range columns {
					flags := uint8(i)
					if defaults[i] != nil {
						flags |= 0x80
					}
					pd.P1(flags)
					pd.P1(uint8(len(col.types)))
					for _, t := range col.types {
						pd.P1(uint8(t))
					}
					if flags&0x80 != 0 {
						pd.P1(1) // field-count = 1
						for j, t := range col.types {
							resolved, err := lookupParamValue(t, defaults[i][j], lk)
							if err != nil {
								return nil, packStepError(name, "default[%d]: %v", j, err)
							}
							if t == objtype.ScriptVarTypeString {
								pd.PJStr(resolved.(string))
							} else {
								pd.P4(uint32(int32(resolved.(int))))
							}
						}
					}
				}
				pd.P1(255) // end-tuple

				// Opcode 251: column-name list.
				pd.P1(251)
				pd.P1(uint8(len(columns)))
				for _, col := range columns {
					pd.PJStr(col.name)
				}

				// Opcode 252: property bits.
				pd.P1(252)
				pd.P1(uint8(len(columns)))
				for _, col := range columns {
					var props uint8
					for _, p := range col.properties {
						switch p {
						case "INDEXED":
							props |= objtype.DbTableFlagIndexed
						case "REQUIRED":
							props |= objtype.DbTableFlagRequired
						case "LIST":
							props |= objtype.DbTableFlagList
						case "CLIENTSIDE":
							props |= objtype.DbTableFlagClientside
						}
					}
					pd.P1(props)
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

- [ ] **Step 2.5: Run tests, confirm pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestPackDbTableConfigs|TestParseDbTableConfig"
```
Expected: PASS.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/pack/dbtable.go pkg/pack/dbtable_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-198 T2 — .dbtable parser + packer

Ports tools/pack/config/DbTableConfig.ts:1-224. Server-only — TS
client buffer is dead (never written) and is omitted. Opcodes 1/251/252
gated on columns.length > 0; 250-trailer per id. INDEXED-without-
REQUIRED, default-on-REQUIRED, and unknown-default-column validations
return packStepError. NAI-195-D-DEADBRANCH-OMITTED applies.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `.dbrow` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/dbrow.go`
- Create: `pkg/pack/dbrow_test.go`

**TS source to read end-to-end before coding**: `tools/pack/config/DbRowConfig.ts:1-185` (185 LOC).

### Step 3.1: Read TS source

- [ ] **Step 3.1: Read `tools/pack/config/DbRowConfig.ts:1-185` completely**

Note:
- Parser (lines 29-82): `table` resolves to id via `DbTablePack.getByName`; `data` returns raw value. All three dead-key arrays empty.
- Packer (lines 84-185): first pass finds `table` line, resolves to `*DbTableType` via `DbTableType.get(value as number)`. Goscape equivalent: `dbtableTypes.Configs[tableID]` (`*DbTableTypeConfigs.Configs` direct-indexed).
- `packStepError("No table defined for dbrow")` if no `table` line found.
- Emit opcode 3 per-id ONLY when `data.length > 0`. For each table column (in column-index order), emit column-id + types-tuple + field-count + per-field typed values. Apply REQUIRED-column-missing and non-LIST-column-multi-value validations. End-of-tuple marker `p1(255)`.
- Opcode 4 emitted ALWAYS (regardless of `data.length`): `p1(4); p2(table.id);`.
- 250-trailer per id when `len(name) > 0`.

### Step 3.2: Write byte-pin tests (TDD-red)

- [ ] **Step 3.2: Create `pkg/pack/dbrow_test.go`** covering:

- `TestPackDbRowConfigs_RowWithSingleColumn`: dbtableTypes has 1 row-shaped column `[int]`; config has `table=<name>` + `data=<col>,42`. Asserts: `opcode 3, types-count=1, col-id=0, types-len=1, type=105, field-count=1, p4(42), 255, opcode 4, p2(table-id), opcode 250, pjstr(name)`.
- `TestPackDbRowConfigs_NoTableDefinedError`: config without `table=` line → error containing debugname.
- `TestPackDbRowConfigs_RequiredColumnMissingError`: dbtableTypes column has REQUIRED prop; config has `table=` but no `data=` for that column → error.
- `TestPackDbRowConfigs_NonListColumnWithMultipleDataError`: column lacks LIST; config has 2 `data=col,...` lines → error.
- `TestPackDbRowConfigs_InvalidDataReferenceError`: column typed as `npc`, `data=col,bogus_name` → error (lookupParamValue returns err).
- `TestPackDbRowConfigs_OnlyTableNoData`: `table=` line only, no `data=` → opcode 3 NOT emitted, opcode 4 + 250-trailer emitted.
- `TestParseDbRowConfigFor_TableResolution`: registers `t_table` at id=7 in DbTablePack; parsing `table=t_table` returns 7.
- `TestParseDbRowConfigFor_UnknownTableRejected`: returns `(nil, true, err)` for unknown table name.

Use a helper to construct fake `*DbTableTypeConfigs` from a literal slice of column-specs. Example fixture builder:

```go
// buildDbTableTypes constructs a *DbTableTypeConfigs with one DbTableType
// having the given column tuples (parallel slices: types[i], colNames[i],
// props[i]). DefaultInts/DefaultStrs are zero (no stored defaults).
func buildDbTableTypes(t *testing.T, tableID int, types [][]objtype.ScriptVarType, colNames []string, props []uint8) *objtype.DbTableTypeConfigs {
	t.Helper()
	cfgs := make([]*objtype.DbTableType, tableID+1)
	cfgs[tableID] = &objtype.DbTableType{
		ConfigType:  objtype.ConfigType{ID: tableID, DebugName: "t_test"},
		Types:       types,
		DefaultInts: make([][]int32, len(types)),
		DefaultStrs: make([][]string, len(types)),
		ColumnNames: colNames,
		Props:       props,
	}
	return &objtype.DbTableTypeConfigs{
		ConfigNames: map[string]int{"t_test": tableID},
		Configs:     cfgs,
	}
}
```

- [ ] **Step 3.3: Run tests, confirm fail (TDD red)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestPackDbRowConfigs|TestParseDbRowConfig"
```
Expected: FAIL with `undefined: packDbRowConfigs` / `undefined: parseDbRowConfigFor`.

### Step 3.4: Implement `pkg/pack/dbrow.go`

- [ ] **Step 3.4: Create `pkg/pack/dbrow.go`**

Implementation guidance:
- `parseDbRowConfigFor(dbtablePack *PackFile) ParseFn` — closure-bound parser. Routes:
  - `key == "table"`: `idx := dbtablePack.GetByName(value)`; reject with `(nil, true, err)` if `idx == -1`; else `(idx, true, nil)`.
  - `key == "data"`: `(value, true, nil)`.
  - Otherwise `(nil, false, nil)`.
- `packDbRowConfigs(configs, pf, dbtableTypes, lk) (*PackedData, error)`:
  - Loop `for id := range pf.Max`.
  - `name := pf.GetByID(id)`; `cfg, ok := configs[name]`.
  - If `ok`:
    - First pass: find `table` line; `tableID := line.Value.(int)`; bounds-check `tableID >= 0 && tableID < len(dbtableTypes.Configs)`. If not found → `packStepError(name, "No table defined for dbrow")`.
    - `table := dbtableTypes.Configs[tableID]`.
    - Second pass: collect `data` lines as `{column string, values []string}` from `parseCsv(line.Value.(string))`.
    - If `len(data) > 0`:
      - `pd.P1(3); pd.P1(uint8(len(table.Types)))`.
      - For each column `i` in `range len(table.Types)`:
        - `pd.P1(uint8(i)); pd.P1(uint8(len(table.Types[i])))`; emit each type.
        - `colName := table.ColumnNames[i]`; `fields := filter(data, d.column == colName)`.
        - If `props & REQUIRED != 0 && len(fields) == 0` → error.
        - If `props & LIST == 0 && len(fields) > 1` → error.
        - `pd.P1(uint8(len(fields)))`; for each field, for each type-index `k`: resolve `lookupParamValue(table.Types[i][k], fields[j].values[k], lk)`; STRING → `PJStr`, else → `P4(uint32(int32(.)))`. Error from lookup → `packStepError(name, "Data invalid in row, double-check the reference exists: data=%s,%s", fields[j].column, strings.Join(fields[j].values, ","))`.
      - `pd.P1(255)`.
    - `pd.P1(4); pd.P2(uint16(tableID))`. (Always emitted when `ok && table != nil`.)
  - 250-trailer + `PJStr(name)` when `len(name) > 0`.
  - `pd.Next()` per id.

- [ ] **Step 3.5: Run tests, confirm pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestPackDbRowConfigs|TestParseDbRowConfig"
```
Expected: PASS.

- [ ] **Step 3.6: Commit**

```bash
git add pkg/pack/dbrow.go pkg/pack/dbrow_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-198 T3 — .dbrow parser + packer

Ports tools/pack/config/DbRowConfig.ts:1-185. Resolves table reference
via parser-side DbTablePack.GetByName; packer consumes mid-pipeline-
loaded *DbTableTypeConfigs for column schema and validates REQUIRED /
non-LIST data shape. Server-only (TS client is dead). Opcode 3 gated on
data.length > 0; opcode 4 (table-id) always emitted when table resolved.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `.hunt` parser + packer + byte-pin tests (largest task)

**Files:**
- Create: `pkg/pack/hunt.go`
- Create: `pkg/pack/hunt_test.go`

**TS source to read end-to-end before coding**: `tools/pack/config/HuntConfig.ts:1-545` (545 LOC).

This is the **largest single-config port** of the entire NAI-191..198 arc (Hunt parser = 372 LOC; packer = 163 LOC). The packer is uniquely complex due to its **mutex-predicate** opcodes (12-17) — each with a slightly different `config.every(x => x.key !== ...)` exclusion list, per `[[asymmetric_predicate_or_chain]]`. **DO NOT refactor the per-arm predicates to a shared helper.**

### Step 4.1: Read TS source

- [ ] **Step 4.1: Read `tools/pack/config/HuntConfig.ts:1-545` completely**

Key reading notes:
- **Parser (lines 9-381)** — 372 LOC, dispatches on `key`. Notable arms:
  - `numberKeys: ['rate']` only — most keys are enum/struct/registry-typed.
  - `find_newmode` (lines 138-261): 60-arm string→`NpcMode.*` chain. Implement as Go `switch`. Includes the TS bug at **line 201-202**: `case 'opobj2': return NpcMode.OPOBJ1;`. Port literally.
  - `check_inv` (lines 304-327): CSV with 3 parts `(inv, obj, condition+val)`. `condition` is the first char of part[2], `val` is the rest as int. Result struct: `{inv, obj, condition, val}`.
  - `check_invparam` (lines 328-353): CSV with 3 parts `(inv, param, condition+val)`. Different from `check_inv` — second part is param, not obj.
  - `extracheck_var` (lines 354-377): CSV with 2 parts `(%varp, condition+val)`. **Important**: `parts[0]` must start with `%` (strip it before `VarpPack.GetByName`).
  - `check_notcombat` (lines 94-106): `%varp` prefix required.
  - `check_notcombat_self` (lines 107-119): `%varn` prefix required.
- **Packer (lines 383-545)** — 163 LOC, 17 opcodes + 18+extracheckVarsCount. Default-skip gates on each (e.g., opcode 7 skipped when `value === HuntNobodyNear.PAUSEHUNT`).
- **Mutex predicates** (opcodes 12-17, lines 449-507): each `config.every(x => x.key !== A && x.key !== B && ...)` has a **different** exclusion set. Port literally; do NOT refactor.
- **Type-gate predicates** (opcodes 12-17): each requires a matching `type=...` line. E.g., opcode 12 (check_category) requires `type` value in `{NPC, OBJ, SCENERY}`; opcode 15 (check_loc) requires `type=SCENERY`; opcodes 16/17 (check_inv/check_invparam) require `type=PLAYER`.
- **extracheck_var ceiling**: max 3 entries → opcodes 18, 19, 20. `extracheckVarsCount > 2` → error.

### Step 4.2: Plan-author concrete example mappings

The implementer reads TS for the 60-arm `find_newmode` table; this plan gives only TWO mappings as reference:

```go
// Example arms (NOT exhaustive — read HuntConfig.ts:138-261 for the full 60):
case "opplayer1":
    return objtype.NPCModeOpPlayer1, true, nil
case "opobj2":
    // NAI-198-D-HUNT-OPOBJ2-TS-BUG: TS HuntConfig.ts:201-202 maps
    // 'opobj2' to NpcMode.OPOBJ1 (typo in upstream). Port literally per
    // [[true_to_ts_gate]]. Goscape NPCModeOpObj1 = 27.
    return objtype.NPCModeOpObj1, true, nil
```

### Step 4.3: Define Go struct types for parser-produced multi-field values

Three new private struct types in `hunt.go` (TS PackShared.ts equivalents):

```go
// huntCheckInv: parser-produced value for check_inv=inv,obj,condition+val
type huntCheckInv struct {
	inv       int
	obj       int
	condition string
	val       int
}

// huntCheckInvParam: parser-produced value for check_invparam=inv,param,condition+val
type huntCheckInvParam struct {
	inv       int
	param     int
	condition string
	val       int
}

// huntCheckVarParsed: parser-produced value for extracheck_var=%varp,condition+val
type huntCheckVarParsed struct {
	varp      int
	condition string
	val       int
}
```

### Step 4.4: Write byte-pin tests (TDD-red)

- [ ] **Step 4.4: Create `pkg/pack/hunt_test.go`** covering:

Tests should establish per-opcode coverage:
- `TestPackHuntConfigs_OpcodeTypeOnly`: `type=npc` → emits `1, 2` (HuntModeNpc = 2).
- `TestPackHuntConfigs_OpcodeTypeOffSkipped`: `type=off` → does NOT emit opcode 1 (TS default-skip).
- `TestPackHuntConfigs_OpcodeCheckVis`: `check_vis=lineofsight` → emits `2, 1`.
- `TestPackHuntConfigs_OpcodeRate`: `rate=5` → emits `11, 0, 5`. `rate=1` → no emit.
- `TestPackHuntConfigs_OpcodeNobodynearPauseSkipped`: `nobodynear=pausehunt` → no emit (TS-equivalent default).
- `TestPackHuntConfigs_OpcodeNobodynearKeephunting`: `nobodynear=keephunting` → emits `7, 0`.
- `TestPackHuntConfigs_OpcodeCheckNotcombatVarp`: with varp `vp1` at id=42, parses `check_notcombat=%vp1` → emits `8, 0, 42`.
- `TestPackHuntConfigs_OpcodeCheckNotcombatSelfVarn`: with varn `vn1` at id=11, parses `check_notcombat_self=%vn1` → emits `9, 0, 11`.
- `TestPackHuntConfigs_OpcodeCheckCategoryWithMatchingType`: `type=npc` + `check_category=cat1` (with cat1 in CategoryPack id=3) → emits opcode 12.
- `TestPackHuntConfigs_OpcodeCheckCategoryWithoutTypeErrors`: `check_category=cat1` but no matching `type` → packer error.
- `TestPackHuntConfigs_OpcodeCheckInvWithType`: `type=player` + valid `check_inv=bank,coins,>10` → emits opcode 16 with `p2(bank.id) p2(coins.id) pjstr(">") p4(10)`.
- `TestPackHuntConfigs_OpcodeExtraCheckVar1Through3`: 3 `extracheck_var=%vp1,>10` lines → emits opcodes 18, 19, 20.
- `TestPackHuntConfigs_OpcodeExtraCheckVarOverflow`: 4 lines → error.
- `TestPackHuntConfigs_OPOBJ2BugPin` (`NAI-198-D-HUNT-OPOBJ2-TS-BUG`): parses `find_newmode=opobj2`; assert the emitted byte for opcode 6 equals `27` (= goscape `NPCModeOpObj1`), NOT `28` (which would be the "correct" NPCModeOpObj2).
- `TestPackHuntConfigs_DebugnameTrailer`: any config → emits `250` + `pjstr(name)` at the end of the id's bytes.
- `TestPackHuntConfigs_EmptyDebugname_No250Trailer`: id with empty name (`""`) → no `250` byte at end.

For each "mutex error" case (opcodes 12-17), also include positive-control cases.

- [ ] **Step 4.5: Run tests, confirm fail (TDD red)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestPackHuntConfigs|TestParseHuntConfig"
```
Expected: FAIL with `undefined: packHuntConfigs` / `undefined: parseHuntConfigFor`.

### Step 4.6: Implement `pkg/pack/hunt.go`

- [ ] **Step 4.6: Create `pkg/pack/hunt.go`**

**Read TS HuntConfig.ts:9-381 end-to-end and port the parser arm-by-arm.** Use the three private struct types from Step 4.3 for `check_inv` / `check_invparam` / `extracheck_var` results.

**Parser signature**:

```go
// parseHuntConfigFor returns the per-key=value parser for .hunt config
// blocks. Routes 17 keys to enum/registry/struct values per TS
// HuntConfig.ts:9-381.
//
// NAI-195-D-DEADBRANCH-OMITTED: TS stringKeys: [] and booleanKeys: []
// arrays are empty — branches omitted.
//
// NAI-198-D-HUNT-OPOBJ2-TS-BUG: the find_newmode arm faithfully ports
// the TS bug at HuntConfig.ts:201-202 (string 'opobj2' maps to
// NPCModeOpObj1 = 27, not NPCModeOpObj2 = 28). See deviation-tag pin
// in nai198_deviation_pins_test.go.
//
// TS source: tools/pack/config/HuntConfig.ts:9-381.
func parseHuntConfigFor(
	categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack *PackFile,
) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		// ... 17-arm switch ...
	}
}
```

**Packer signature**:

```go
// packHuntConfigs walks every id and emits opcodes 1-17 plus
// 18+extracheckVarsCount (max 3 → opcodes 18, 19, 20), gated by
// per-arm default-skip and mutex-predicate logic.
//
// Server-only — TS allocates a client PackedData but never writes to
// it. Goscape omits the client buffer entirely.
//
// TS source: tools/pack/config/HuntConfig.ts:383-545.
func packHuntConfigs(configs map[string][]ConfigLine, pf *PackFile) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			extracheckVarsCount := 0
			// ... 17-opcode dispatch + extracheck_var counter ...
			_ = extracheckVarsCount
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

**Mutex-predicate helper (per `[[asymmetric_predicate_or_chain]]`)** — do NOT refactor the per-arm predicates into a single shared helper. Each arm of opcodes 12-17 has a slightly different exclusion set. Use one inline tiny helper:

```go
// hasKey reports whether any line in cfg has the given key.
func hasKey(cfg []ConfigLine, key string) bool {
	for _, line := range cfg {
		if line.Key == key {
			return true
		}
	}
	return false
}

// findTypeValue returns the value of the first 'type' line in cfg, or -1
// if absent. (Hunt opcodes 12-17 require a matching type for context.)
func findTypeValue(cfg []ConfigLine) int {
	for _, line := range cfg {
		if line.Key == "type" {
			if v, ok := line.Value.(int); ok {
				return v
			}
		}
	}
	return -1
}
```

Then per-arm inline checks like:

```go
case "check_category":
	// TS HuntConfig.ts:449-458: predicate excludes check_npc/obj/loc/inv/invparam,
	// requires type in {NPC, OBJ, SCENERY}.
	if hasKey(cfg, "check_npc") || hasKey(cfg, "check_obj") || hasKey(cfg, "check_loc") ||
		hasKey(cfg, "check_inv") || hasKey(cfg, "check_invparam") {
		return nil, packStepError(name, "unable to pack line!!!.\nInvalid property value: %s=%v", line.Key, line.Value)
	}
	typeVal := findTypeValue(cfg)
	if typeVal != objtype.HuntModeNpc && typeVal != objtype.HuntModeObj && typeVal != objtype.HuntModeScenery {
		return nil, packStepError(name, "unable to pack line!!!.\nInvalid property value: %s=%v", line.Key, line.Value)
	}
	pd.P1(12)
	pd.P2(uint16(line.Value.(int)))
```

**OPOBJ2 deviation-tag comment** placement — adjacent to the `find_newmode` `case "opobj2":` arm. Per `[[pin_test_self_trigger_production_doc]]`, the tag comment may contain the identifier `NAI-198-D-HUNT-OPOBJ2-TS-BUG` (which the pin grep matches) but the comment should explain the TS source line via "HuntConfig.ts:201-202" rather than echoing the bare TS identifier. Example comment shape:

```go
// NAI-198-D-HUNT-OPOBJ2-TS-BUG: TS HuntConfig.ts:201-202 maps the
// 'opobj2' string to NpcMode.OPOBJ1 (typo in upstream — should be
// OPOBJ2). Ported literally per [[true_to_ts_gate]]. Goscape constant
// NPCModeOpObj1 = 27. Tracked for upstream reconciliation in
// [[nai_followups]].
case "opobj2":
	return objtype.NPCModeOpObj1, true, nil
```

The phrase `'opobj2'` appears once above only in `case "opobj2":` (as a Go string literal, NOT as the standalone TS identifier `opobj2` referenced in the deviation-tag comment block). The OPOBJ2 pin in T7 reads `hunt.go` as bytes and matches against the tag identifier `NAI-198-D-HUNT-OPOBJ2-TS-BUG` only — it does NOT substring-search for the bare string `opobj2`, so the production case-line is safe.

- [ ] **Step 4.7: Run tests, confirm pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestPackHuntConfigs|TestParseHuntConfig"
```
Expected: PASS.

- [ ] **Step 4.8: Commit**

```bash
git add pkg/pack/hunt.go pkg/pack/hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-198 T4 — .hunt parser + packer

Ports tools/pack/config/HuntConfig.ts:1-545 (largest single-config port
of NAI-191..198 arc — 545 LOC). 17 opcodes + 18..20 for extracheck_var,
with TS-asymmetric mutex predicates per opcode arm preserved literally
(see [[asymmetric_predicate_or_chain]]). Server-only — TS client buffer
dead. New deviation tag NAI-198-D-HUNT-OPOBJ2-TS-BUG records the TS
typo at HuntConfig.ts:201-202 (opobj2 → OPOBJ1); ported faithfully.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `PackConfigs` wiring + atomic 15→18 test rename + doc-comment update

**Files:**
- Modify: `pkg/pack/pack_configs.go`
- Modify: `pkg/pack/pack_configs_test.go`

This task wires the three new branches into `PackConfigs` at TS-canonical positions, atomically renames the integration test, and updates the production doc-comment from "six server-only freshness-gated branches" to "nine" with adjacent-paragraph audit per `[[adjacent_doc_paragraph_count_drift]]`.

### Step 5.1: Add three `packAndSave*` orchestrator helpers

- [ ] **Step 5.1: Append three new functions at the bottom of `pkg/pack/pack_configs.go` (after `packAndSaveIdk`)**

```go
// packAndSaveDbTable reads .dbtable sources, packs them, and writes
// server .dat/.idx. Server-only — does NOT contribute to clientJag.
//
// TS source: tools/pack/config/DbTableConfig.ts:78-224.
func packAndSaveDbTable(srcDir, serverOut string, dbtablePack *PackFile, lk *paramLookups, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".dbtable", nil, parseDbTableConfig, c)
	if err != nil {
		return err
	}
	pd, err := packDbTableConfigs(cfgs, dbtablePack, lk)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "dbtable.dat"), filepath.Join(serverOut, "dbtable.idx"))
}

// packAndSaveDbRow reads .dbrow sources, packs them, and writes server
// .dat/.idx. Server-only. Consumes the *DbTableTypeConfigs loaded from
// the just-written dbtable.dat for schema lookup.
//
// TS source: tools/pack/config/DbRowConfig.ts:84-185.
func packAndSaveDbRow(srcDir, serverOut string, dbrowPack, dbtablePack *PackFile, dbtableTypes *objtype.DbTableTypeConfigs, lk *paramLookups, c Constants) error {
	parse := parseDbRowConfigFor(dbtablePack)
	cfgs, err := ReadTypedConfigs(srcDir, ".dbrow", nil, parse, c)
	if err != nil {
		return err
	}
	pd, err := packDbRowConfigs(cfgs, dbrowPack, dbtableTypes, lk)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "dbrow.dat"), filepath.Join(serverOut, "dbrow.idx"))
}

// packAndSaveHunt reads .hunt sources, packs them, and writes server
// .dat/.idx. Server-only — does NOT contribute to clientJag. Takes
// nine *PackFile parameters (eight reference registries + the Hunt
// pack itself); largest registry-dependency surface of any NAI-198
// config.
//
// TS source: tools/pack/config/HuntConfig.ts:383-545.
func packAndSaveHunt(srcDir, serverOut string, huntPack, categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack *PackFile, c Constants) error {
	parse := parseHuntConfigFor(categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".hunt", nil, parse, c)
	if err != nil {
		return err
	}
	pd, err := packHuntConfigs(cfgs, huntPack)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "hunt.dat"), filepath.Join(serverOut, "hunt.idx"))
}
```

**Plan-author mental-compile check** (per `[[plan_var_name_collision]]`): `packAndSaveHunt` has 11 string/*PackFile parameters. Parameter names (`huntPack`, `categoryPack`, ..., `varpPack`) shadow outer-scope `var` declarations of the same names in `PackConfigs`. This is intentional — within the function body, the parameter is the only in-scope binding. No `:=` redeclarations occur inside the body. Compiles clean.

### Step 5.2: Insert the paired `.dbtable`+`.dbrow` branch

- [ ] **Step 5.2: Insert paired branch in `PackConfigs` between `.struct` and `.seq`**

Locate the `.struct` block ending at `pack_configs.go:328` (current `if GetLatestModified...struct...` block). Insert IMMEDIATELY after, before the `.seq` block at line 330:

```go
	// .dbtable + .dbrow — paired server-only joint freshness-gated.
	// TS PackShared.ts:393-414 — joint shouldBuild gate, DbTableType.load
	// between packers. Goscape mirrors via mid-pipeline objtype.LoadDbTableTypes.
	if GetLatestModified(scriptsDir, ".dbrow") > 0 || GetLatestModified(scriptsDir, ".dbtable") > 0 {
		if ShouldBuild(scriptsDir, ".dbrow", filepath.Join(serverOut, "dbrow.dat")) ||
			ShouldBuild(scriptsDir, ".dbtable", filepath.Join(serverOut, "dbtable.dat")) {
			if err := ensureDbTablePack(); err != nil {
				return err
			}
			if err := packAndSaveDbTable(srcDir, serverOut, dbtablePack, lk, constants); err != nil {
				return err
			}

			// Mid-pipeline DbTableType cache load — .dbrow needs to resolve
			// table=NAME → *DbTableType at pack time. Per
			// [[load_param_types_dir_arg]]: LoadDbTableTypes takes outDir
			// (parent of server/), NOT serverOut.
			dbtableTypes, err := objtype.LoadDbTableTypes(outDir)
			if err != nil {
				return fmt.Errorf("load dbtable types between dbtable/dbrow packers: %w", err)
			}

			if err := ensureDbRowPack(); err != nil {
				return err
			}
			if err := packAndSaveDbRow(srcDir, serverOut, dbrowPack, dbtablePack, dbtableTypes, lk, constants); err != nil {
				return err
			}
		}
	}
```

### Step 5.3: Insert the `.hunt` branch

- [ ] **Step 5.3: Insert `.hunt` branch between `.varp` and `.varn`**

Locate the `.varp` block at `pack_configs.go:441-444`. Insert IMMEDIATELY after, before the `.varn` block at line 446:

```go
	// .hunt — server-only, freshness-gated.
	// TS PackShared.ts:638-645. Eight reference registries — largest
	// fan-out of any single config.
	if GetLatestModified(scriptsDir, ".hunt") > 0 &&
		ShouldBuild(scriptsDir, ".hunt", filepath.Join(serverOut, "hunt.dat")) {
		if err := ensureCategoryPack(); err != nil {
			return err
		}
		if err := ensureHuntPack(); err != nil {
			return err
		}
		if err := ensureInvPack(); err != nil {
			return err
		}
		if err := ensureLocPack(); err != nil {
			return err
		}
		if err := ensureNpcPack(); err != nil {
			return err
		}
		if err := ensureObjPack(); err != nil {
			return err
		}
		// paramPack already lazy-promoted (T1); ensure it's loaded.
		if err := ensureParamPack(); err != nil {
			return err
		}
		if err := packAndSaveHunt(srcDir, serverOut, huntPack, categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack, constants); err != nil {
			return err
		}
	}
```

### Step 5.4: Update `PackConfigs` doc-comment scope phrasing

- [ ] **Step 5.4: Update `NAI-192-D-NO-SRC-NO-OP` doc-comment count phrasing**

Locate `pack_configs.go:49-52`:

```go
// NAI-192-D-NO-SRC-NO-OP: applies only to the six server-only
// freshness-gated branches. The nine unconditional branches always
// run; an empty source directory produces an empty .dat/.idx pair
// (matching TS shouldBuild-output-missing arm).
```

REPLACE with (count goes 6→9; enumerate explicitly per spec §7.4):

```go
// NAI-192-D-NO-SRC-NO-OP: applies only to the nine server-only
// freshness-gated branches (.enum, .inv, .mesanim, .struct, .dbtable,
// .dbrow, .hunt, .varn, .vars). The nine unconditional branches always
// run; an empty source directory produces an empty .dat/.idx pair
// (matching TS shouldBuild-output-missing arm).
```

**Adjacent-paragraph audit** per `[[adjacent_doc_paragraph_count_drift]]`: re-read the four other paragraphs in the `PackConfigs` doc-block (`pack_configs.go:11-57`).

- The `// NAI-191–195 wired...` summary paragraph (lines 11-15) mentions specific config types but no enumerated count — UNAFFECTED.
- The `// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` paragraph (lines 24-26) — UNAFFECTED.
- The `// NAI-191-D-VALIDATE-FLAGS-DEFERRED` paragraph (lines 28-30) — UNAFFECTED.
- The `// NAI-196-D-UNCONDITIONAL-CLIENT-PACK` paragraph (lines 37-47) — enumerates "9 unconditional branches" (`.param`, `.seq`, `.loc`, `.flo`, `.spotanim`, `.npc`, `.obj`, `.idk`, `.varp`). The NAI-198 cohort is server-only so this paragraph's count and enumeration are UNAFFECTED. **However**: the NAI-192-D paragraph's neighboring sentence ("The nine unconditional branches always run...") restates the same nine — leave that phrasing alone (it remains accurate). **Confirm**: both "nine" references in the post-edit doc-block refer to the SAME nine unconditional branches; the new "nine" introduced for freshness-gated reads ALSO as nine. The doc-block ends up with two distinct uses of "nine" — explicit enumeration in the NAI-192-D paragraph (server-only freshness-gated branches) and contextual reference in the same paragraph's second sentence (unconditional branches). Acceptable; both numbers are correct.
- The summary paragraph at the top of the doc-block (lines 11-15) currently reads "NAI-191–195 wired .varp/.varn/.vars/.param/.enum/.inv/.mesanim/.struct. NAI-196 wires .loc/.npc/.obj and re-orders the pipeline to TS-canonical layout per tools/pack/config/PackShared.ts:261-669 (filtered to currently implemented configs)." This was updated at NAI-197 to mention `.seq/.flo/.spotanim/.idk`. **Audit**: does this paragraph need extending to mention `.hunt/.dbtable/.dbrow`? **Yes** — per `[[adjacent_doc_paragraph_count_drift]]`, the historical-summary paragraph drifts if not updated. UPDATE to:

```go
// PackConfigs runs the per-config packing pipeline. NAI-191–195 wired
// .varp/.varn/.vars/.param/.enum/.inv/.mesanim/.struct. NAI-196 wires
// .loc/.npc/.obj and re-orders the pipeline to TS-canonical layout per
// tools/pack/config/PackShared.ts:261-669. NAI-197 wires
// .seq/.flo/.spotanim/.idk. NAI-198 wires .hunt/.dbtable/.dbrow and
// closes the per-config layer (18/18 TS configs ported).
```

### Step 5.5: Atomic rename integration test

- [ ] **Step 5.5: Rewrite `TestPackConfigs_FifteenConfigsLand` → `TestPackConfigs_EighteenConfigsLand`**

Locate `pack_configs_test.go:410-536` (the entire function body). REWRITE in-place with the following changes:

1. Rename `TestPackConfigs_FifteenConfigsLand` → `TestPackConfigs_EighteenConfigsLand`.
2. Add three source-file writes (after the existing `d.idk` line ~line 474):

```go
	writeFile(t, filepath.Join(scripts, "t.dbtable"),
		"[t_simple]\ncolumn=score,int\n")
	writeFile(t, filepath.Join(scripts, "r.dbrow"),
		"[r_one]\ntable=t_simple\ndata=score,7\n")
	writeFile(t, filepath.Join(scripts, "h.hunt"),
		"[h_off]\ntype=off\n")
```

3. Extend the `dbtable.pack` and `hunt.pack` registrations. The current test (line 442-444) writes `interface`, `synth`, `dbrow` as empty stubs. CHANGE the loop to include `dbtable` AND add NAMED entries for `dbtable` and `dbrow` and `hunt`:

```go
	// Replace existing line 439:
	writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=h_off\n")
	// Replace existing lines 442-444:
	writeFile(t, filepath.Join(srcDir, "pack", "dbtable.pack"), "0=t_simple\n")
	writeFile(t, filepath.Join(srcDir, "pack", "dbrow.pack"), "0=r_one\n")
	for _, p := range []string{"interface", "synth"} {
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}
```

4. Extend the server-side `.dat`/`.idx` existence assertion (lines 483-486) by adding `"dbtable", "dbrow", "hunt"`:

```go
	for _, typ := range []string{
		"varp", "varn", "vars", "param", "enum", "inv", "mesanim", "struct",
		"dbtable", "dbrow", "loc", "npc", "obj", "seq", "flo", "spotanim", "idk", "hunt",
	} {
		// ...
	}
```

5. **Client jagfile entries do NOT grow** (the three new configs are server-only). Current `expected` and `wantOrder` arrays (lines 503-513 / 522-532) hold 18 entries (9 unconditional client branches × 2). Both stay UNCHANGED. This asserts the non-growth invariant per spec §7.4.

### Step 5.6: Run integration tests

- [ ] **Step 5.6: Run full pkg/pack tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...
```
Expected: PASS.

- [ ] **Step 5.7: Joint freshness-gate four-corner-case test**

Add four targeted tests as part of T5 (per R9 spec §9 pre-trace):

```go
func TestPackConfigs_JointDbTableDbRowFreshness_NeitherDirty(t *testing.T) {
	// No .dbtable, no .dbrow source files. Outer GetLatestModified guard
	// fails → joint branch skipped. dbtable.dat / dbrow.dat NOT written.
	// ... (set up only varn fixture; PackConfigs; assert os.IsNotExist for both .dat files)
}

func TestPackConfigs_JointDbTableDbRowFreshness_OnlyDbtableDirty(t *testing.T) {
	// Only .dbtable source present. Outer guard fires (dbtable > 0).
	// Joint ShouldBuild fires (either could). Both packers run.
	// ... (assert both dbtable.dat and dbrow.dat exist)
}

func TestPackConfigs_JointDbTableDbRowFreshness_OnlyDbrowDirty(t *testing.T) {
	// Only .dbrow source present (no .dbtable). Joint branch fires,
	// but .dbtable packing produces empty bytes — *DbTableTypeConfigs
	// is empty — .dbrow with table=anything will error at parse time.
	// Test asserts the parse error surface (not the joint-gate-not-firing).
}

func TestPackConfigs_JointDbTableDbRowFreshness_BothDirty(t *testing.T) {
	// Both source files. Joint branch fires; round-trip via LoadDbTableTypes
	// + LoadDbRowTypes resolves table refs.
}
```

- [ ] **Step 5.8: Commit**

```bash
git add pkg/pack/pack_configs.go pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-198 T5 — wire .dbtable/.dbrow/.hunt + 18-configs test

Inserts paired .dbtable+.dbrow branch (joint freshness gate, mid-
pipeline LoadDbTableTypes between packers) between .struct and .seq,
and .hunt branch between .varp and .varn — TS-canonical positions per
PackShared.ts:393-414 and :638-645. Atomically renames
TestPackConfigs_FifteenConfigsLand → TestPackConfigs_EighteenConfigsLand
and extends fixtures by three configs + three .pack registries. Client
jag entry count unchanged at 18 (three new configs are server-only).
NAI-192-D-NO-SRC-NO-OP doc-comment scope grows from 6 to 9 branches with
explicit enumeration; adjacent paragraphs audited per
[[adjacent_doc_paragraph_count_drift]].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Round-trip tests for `.dbtable`, `.dbrow`, `.hunt`

**Files:**
- Create: `pkg/pack/dbtable_roundtrip_test.go`
- Create: `pkg/pack/dbrow_roundtrip_test.go`
- Create: `pkg/pack/hunt_roundtrip_test.go`

### Step 6.1: `dbtable_roundtrip_test.go`

- [ ] **Step 6.1: Create test**

Pattern (per `[[plan_runnable_test_fixtures]]` — mentally compile before dispatching):

```go
package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_DbTableRoundTrip(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// Build minimal fixture: one dbtable with two columns, one of them
	// REQUIRED+LIST.
	writeFile(t, filepath.Join(srcDir, "scripts", "t.dbtable"),
		"[t_demo]\ncolumn=hp,int\ncolumn=loot,obj,REQUIRED,LIST\n")
	writeFile(t, filepath.Join(srcDir, "pack", "dbtable.pack"), "0=t_demo\n")

	// All other required .pack stubs (param dependency tree etc.):
	writeAllOtherEmptyPacks_NAI198(t, srcDir)

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadDbTableTypes(outDir)
	if err != nil {
		t.Fatalf("LoadDbTableTypes: %v", err)
	}
	if len(cfgs.Configs) != 1 {
		t.Fatalf("got %d dbtable configs, want 1", len(cfgs.Configs))
	}
	dt := cfgs.Configs[0]
	if dt.DebugName != "t_demo" {
		t.Errorf("DebugName=%q, want %q", dt.DebugName, "t_demo")
	}
	if len(dt.ColumnNames) != 2 || dt.ColumnNames[0] != "hp" || dt.ColumnNames[1] != "loot" {
		t.Errorf("ColumnNames=%v, want [hp loot]", dt.ColumnNames)
	}
	if len(dt.Props) != 2 || dt.Props[0] != 0 || dt.Props[1] != (objtype.DbTableFlagRequired|objtype.DbTableFlagList) {
		t.Errorf("Props=%v, want [0 0x06]", dt.Props)
	}
}
```

Define `writeAllOtherEmptyPacks_NAI198` (test-local helper) to write empty `varp/varn/vars/param/enum/inv/mesanim/struct/loc/npc/obj/seq/flo/spotanim/idk/dbrow/hunt/model/category/texture/anim/interface/synth/dbrow.pack`. Per `[[plan_runnable_test_fixtures]]`, this must be complete enough that `PackConfigs` doesn't fail on missing .pack files. Reuse the pattern from `writeEmptyTypedPacks` and extend.

### Step 6.2: `dbrow_roundtrip_test.go`

- [ ] **Step 6.2: Create test**

Round-trips `.dbtable` + `.dbrow` together (DbRow can't standalone). Fixture has one dbtable with one int column and one dbrow with `data=col,42`. Assertions:

- `len(rcfgs.Configs) == 1`
- `rcfgs.Configs[0].DebugName == "r_one"`
- `rcfgs.Configs[0].TableID == 0`
- `rcfgs.Configs[0].IntValues[0]` contains `42` at index 0.

### Step 6.3: `hunt_roundtrip_test.go`

- [ ] **Step 6.3: Create test**

Pattern:

```go
func TestPackConfigs_HuntRoundTrip(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "h.hunt"),
		"[h_test]\ntype=npc\ncheck_vis=lineofsight\nrate=5\nfind_newmode=opobj2\n")
	writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=h_test\n")
	writeAllOtherEmptyPacks_NAI198(t, srcDir)

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadHuntTypes(outDir)
	if err != nil {
		t.Fatalf("LoadHuntTypes: %v", err)
	}
	if len(cfgs.Configs) != 1 {
		t.Fatalf("got %d hunt configs, want 1", len(cfgs.Configs))
	}
	h := cfgs.Configs[0]
	if h.Type != objtype.HuntModeNpc {
		t.Errorf("Type=%d, want %d", h.Type, objtype.HuntModeNpc)
	}
	if h.CheckVis != objtype.HuntVisLineOfSight {
		t.Errorf("CheckVis=%d, want %d", h.CheckVis, objtype.HuntVisLineOfSight)
	}
	if h.Rate != 5 {
		t.Errorf("Rate=%d, want 5", h.Rate)
	}
	// NAI-198-D-HUNT-OPOBJ2-TS-BUG round-trip: find_newmode=opobj2 in
	// the source resolves to NPCModeOpObj1 (= 27) in the decoded type,
	// NOT NPCModeOpObj2 (= 28).
	if h.FindNewMode != objtype.NPCModeOpObj1 {
		t.Errorf("FindNewMode=%d, want NPCModeOpObj1=%d (TS bug ported per NAI-198-D-HUNT-OPOBJ2-TS-BUG)",
			h.FindNewMode, objtype.NPCModeOpObj1)
	}
}
```

### Step 6.4: Run round-trip tests

- [ ] **Step 6.4: Run all NAI-198 round-trip tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "RoundTrip"
```
Expected: PASS.

### Step 6.5: Commit

- [ ] **Step 6.5: Commit**

```bash
git add pkg/pack/dbtable_roundtrip_test.go pkg/pack/dbrow_roundtrip_test.go pkg/pack/hunt_roundtrip_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-198 T6 — round-trip tests for .dbtable/.dbrow/.hunt

Sources → PackConfigs → objtype.Load{DbTable,DbRow,Hunt}Types. Asserts
3 representative fields per config. .hunt round-trip pins the TS-bug
behavior (find_newmode=opobj2 decodes to NPCModeOpObj1 = 27) per
NAI-198-D-HUNT-OPOBJ2-TS-BUG.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Deviation-tag pins (`nai198_deviation_pins_test.go`)

**Files:**
- Create: `pkg/pack/nai198_deviation_pins_test.go`

### Step 7.1: Write the three pin tests

- [ ] **Step 7.1: Create `pkg/pack/nai198_deviation_pins_test.go`**

Per `[[pin_test_self_trigger_production_doc]]`: the OPOBJ2 pin grep matches the tag identifier `NAI-198-D-HUNT-OPOBJ2-TS-BUG` only — it does NOT substring-search for `opobj2`, which would self-trigger when the implementer adds the production case-mapping in `hunt.go`.

```go
package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNAI198_PresencePin_NoSrcNoOpScopeExtended re-asserts that the
// NAI-192-D-NO-SRC-NO-OP doc-comment in pkg/pack/pack_configs.go now
// enumerates the nine server-only freshness-gated branches (six
// existing + .dbtable + .dbrow + .hunt added in NAI-198).
//
// Guards against:
//
//	(a) accidental retirement of the tag identifier;
//	(b) regression of the count phrasing back to "six";
//	(c) doc-comment refactors that drop one or more of the nine configs.
func TestNAI198_PresencePin_NoSrcNoOpScopeExtended(t *testing.T) {
	src := scanPkgPack(t)
	if !strings.Contains(src, "NAI-192-D-NO-SRC-NO-OP") {
		t.Fatal("NAI-192-D-NO-SRC-NO-OP tag should be present in pkg/pack production code")
	}
	idx := strings.Index(src, "NAI-192-D-NO-SRC-NO-OP")
	window := src[idx:]
	end := strings.Index(window, "\n//\n")
	if end == -1 || end > 2000 {
		end = 2000
	}
	if end > len(window) {
		end = len(window)
	}
	block := window[:end]

	if !strings.Contains(block, "nine") {
		t.Errorf("NAI-192-D-NO-SRC-NO-OP scope phrasing should say 'nine' branches; block:\n%s", block)
	}
	for _, cfg := range []string{".enum", ".inv", ".mesanim", ".struct", ".dbtable", ".dbrow", ".hunt", ".varn", ".vars"} {
		if !strings.Contains(block, cfg) {
			t.Errorf("NAI-192-D-NO-SRC-NO-OP scope is missing config %q; block:\n%s", cfg, block)
		}
	}
}

// TestNAI198_PresencePin_HuntOpObj2TsBugTagged asserts the
// NAI-198-D-HUNT-OPOBJ2-TS-BUG deviation tag appears in pkg/pack/hunt.go
// as a comment. The tag flags goscape's literal port of the TS typo
// at HuntConfig.ts:201-202 ('opobj2' string maps to NpcMode.OPOBJ1,
// not OPOBJ2). Per [[pin_test_self_trigger_production_doc]], the pin
// matches against the tag identifier ONLY — not the bare string
// 'opobj2', which appears in the production case statement.
func TestNAI198_PresencePin_HuntOpObj2TsBugTagged(t *testing.T) {
	huntPath := filepath.Join("hunt.go")
	bytes, err := os.ReadFile(huntPath)
	if err != nil {
		t.Fatalf("read hunt.go: %v", err)
	}
	src := string(bytes)
	if !strings.Contains(src, "NAI-198-D-HUNT-OPOBJ2-TS-BUG") {
		t.Fatal("expected NAI-198-D-HUNT-OPOBJ2-TS-BUG tag-comment in pkg/pack/hunt.go")
	}
}

// TestNAI198_AbsencePin_UnconditionalClientCohortDoesNotGrow asserts
// the NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment scope does NOT
// list .hunt, .dbtable, or .dbrow. The three NAI-198 configs are
// server-only-freshness-gated, never unconditional-client. Guards
// against accidental scope expansion.
func TestNAI198_AbsencePin_UnconditionalClientCohortDoesNotGrow(t *testing.T) {
	src := scanPkgPack(t)
	idx := strings.Index(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK")
	if idx == -1 {
		t.Fatal("NAI-196-D-UNCONDITIONAL-CLIENT-PACK tag not present in pkg/pack")
	}
	window := src[idx:]
	end := strings.Index(window, "\n//\n")
	if end == -1 || end > 2000 {
		end = 2000
	}
	if end > len(window) {
		end = len(window)
	}
	block := window[:end]
	for _, forbidden := range []string{".hunt", ".dbtable", ".dbrow"} {
		if strings.Contains(block, forbidden) {
			t.Errorf("NAI-196-D-UNCONDITIONAL-CLIENT-PACK scope MUST NOT contain %q (server-only config); block:\n%s",
				forbidden, block)
		}
	}
}
```

### Step 7.2: Run pin tests

- [ ] **Step 7.2: Run pin tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "NAI198"
```
Expected: PASS (all three).

### Step 7.3: Commit

- [ ] **Step 7.3: Commit**

```bash
git add pkg/pack/nai198_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-198 T7 — deviation-tag pins

Adds three deviation-tag pins:
  - NAI-192-D-NO-SRC-NO-OP scope-extension: doc-comment now lists
    nine server-only freshness-gated branches.
  - NAI-198-D-HUNT-OPOBJ2-TS-BUG: tag-comment is present in
    pkg/pack/hunt.go adjacent to the literal TS-bug port.
  - NAI-196-D-UNCONDITIONAL-CLIENT-PACK non-growth: server-only
    cohort .hunt/.dbtable/.dbrow do NOT appear in the unconditional
    scope listing.

Per [[pin_test_self_trigger_production_doc]], OPOBJ2 pin matches the
tag identifier only, not the bare string 'opobj2' that appears in the
production case statement.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: NAI-191 follow-up #2 retirement (documentation-only)

**Files:**
- (none in production)
- Auto-memory: write a close-trailer note and update tracker

This task is **documentation-only** — no production code change. NAI-191 follow-up #2 (`LoadFile` returns `nil` on missing vs TS `[]`) has now survived 7 sub-spec slices (NAI-192 through NAI-198) without a distinguishing consumer. Per spec §7.2, retire it at NAI-198 close.

### Step 8.1: Verify no production action needed

- [ ] **Step 8.1: Re-verify `pkg/pack/parse.go` LoadFile body is unchanged from NAI-191**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -nE "LoadFile|nil-vs-empty" $HOME/Code/github.com/zsrv/goscape/pkg/pack/parse.go
```
Expected: `LoadFile` exists at parse.go:15-23 (or similar); body returns `nil` on missing; no follow-up TODO/deviation comment is in the body. Confirm: no production change for retirement.

### Step 8.2: Update auto-memory tracker

- [ ] **Step 8.2: Update `[[nai_followups]]` memory entry**

If the memory entry exists, edit it to:
- Remove the NAI-191 follow-up #2 line.
- Add an NAI-198 entry: "upstream HuntConfig.ts:201-202 OPOBJ2 typo — file LostCityRS/Engine-TS issue + reconcile NAI-198-D-HUNT-OPOBJ2-TS-BUG".

If the memory entry doesn't exist or doesn't currently track NAI-191 #2, the retirement is still confirmed in the close-commit body (Step 8.3).

### Step 8.3: Close commit (NAI-198 close)

- [ ] **Step 8.3: Close commit with retirement trailer**

Per `[[close_commit_memory_trailer]]`:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-198 — .hunt + .dbtable + .dbrow packer slice + per-config layer complete

Closes the per-config layer of the pack pipeline: 18/18 TS configs
ported. Pipeline order (TS-canonical): param → enum → inv → mesanim →
struct → dbtable → dbrow → seq → loc → flo → spotanim → npc → obj →
idk → varp → hunt → varn → vars.

Retires NAI-191 follow-up #2 (LoadFile returns nil on missing vs TS []):
has survived 7 sub-spec slices (NAI-192..198) without a distinguishing
consumer. The three NAI-198 configs (.dbtable/.dbrow/.hunt) use plain
range iteration over config — nil-vs-empty distinction is behaviorally
invisible. If a future config DOES distinguish, re-open as a new tag.

Arc momentum: specials slice (frame_del / category.pack writers) is
NAI-199; bytecode compiler arc opens NAI-200+; PackAll wiring NAI-201+;
validate cohort NAI-202+.

Closes memory: nai_followups (NAI-191 #2 retirement; NAI-198 OPOBJ2
upstream reconcile follow-up added)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review (controller, before dispatch)

**Spec coverage check** (each spec §/requirement → task that implements it):

| Spec § | Task | Implemented? |
|---|---|---|
| §2 in-scope: `parseCsv` shared | T1 | ✅ |
| §2 in-scope: 2 new lazy `ensureFooPack` + 2 lazy promotions | T1 | ✅ |
| §2 in-scope: `.dbtable` parser+packer | T2 | ✅ |
| §2 in-scope: `.dbrow` parser+packer | T3 | ✅ |
| §2 in-scope: `.hunt` parser+packer | T4 | ✅ |
| §2 in-scope: PackConfigs re-ordering (3 new branches) | T5 | ✅ |
| §2 in-scope: 18-config integration test atomic rename | T5.5 | ✅ |
| §2 in-scope: Round-trip tests (3) | T6 | ✅ |
| §2 in-scope: New deviation-tag pin test file | T7 | ✅ |
| §2 in-scope: NAI-191 follow-up #2 retirement | T8 | ✅ |
| §4.4 mid-pipeline `LoadDbTableTypes` | T5.2 | ✅ |
| §5.1 NAI-198-D-HUNT-OPOBJ2-TS-BUG deviation | T4.6 + T7 | ✅ |
| §5.2 INDEXED-without-REQUIRED error | T2.4 | ✅ |
| §5.3 No-table-defined error | T3.4 | ✅ |
| §6 joint freshness gate semantics | T5.2 + T5.7 4-corner tests | ✅ |
| §7.2 NAI-192-D scope extension 6→9 | T5.4 | ✅ |
| §7.3 New NAI-198-D-HUNT-OPOBJ2-TS-BUG tag | T4.6 | ✅ |
| §7.4 NAI-192-D presence pin (scope-extension) | T7.1 | ✅ |
| §7.5 NAI-196-D non-growth absence pin | T7.1 | ✅ |
| §8.1 byte-pin tests (3 files) | T2.2, T3.2, T4.4 | ✅ |
| §8.2 Round-trip tests (3 files) | T6 | ✅ |
| §8.3 18-configs integration test | T5.5 | ✅ |
| §8.4 Deviation-tag pin file | T7 | ✅ |
| §9 R1-R15 risk register | Pre-flight (above) | ✅ |
| §10 follow-up entries | T8 (NAI-191 #2) + close commit (NAI-198 upstream OPOBJ2) | ✅ |

**Placeholder scan**: no "TBD"/"TODO"/"implement later" appear in plan task bodies. (One TODO in §4.6 production code comment is intentional — references the future upstream-bug reconciliation.)

**Type consistency check**:
- Parser fn signatures: `parseDbTableConfig` (no closure), `parseDbRowConfigFor(dbtablePack)` (closure), `parseHuntConfigFor(8 packs)` (closure). All return `ParseFn`.
- Packer fn signatures: `packDbTableConfigs(configs, pf, lk) (*PackedData, error)`, `packDbRowConfigs(configs, pf, dbtableTypes, lk) (*PackedData, error)`, `packHuntConfigs(configs, pf) (*PackedData, error)`. All return single `*PackedData` (server-only).
- Orchestrator helpers: `packAndSaveDbTable(srcDir, serverOut, dbtablePack, lk, c)`, `packAndSaveDbRow(srcDir, serverOut, dbrowPack, dbtablePack, dbtableTypes, lk, c)`, `packAndSaveHunt(srcDir, serverOut, huntPack, categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack, c)`. None take `clientJag` (server-only).
- New helpers in `csv.go`: `parseCsv(s string) []string`, `packStepError(debugname, format string, args ...any) error`.
- Lazy ensurers: `ensureDbTablePack`, `ensureDbRowPack`, `ensureInvPack`, `ensureParamPack` (NEW); `ensureCategoryPack`, `ensureHuntPack`, `ensureLocPack`, `ensureNpcPack`, `ensureObjPack` (EXISTING) — all `func() error` shape.

**`packAndSaveHunt` mental-compile** (per `[[plan_var_name_collision]]`): 11 parameters (`srcDir`, `serverOut`, 9 `*PackFile`, `Constants`). Within the body: parser-closure constructor + `ReadTypedConfigs` + `packHuntConfigs` + `pd.Save`. Single `err :=` declaration per call; no redeclarations. Parameter names shadow outer-scope `var` declarations in `PackConfigs` but are the only in-scope binding within the function body. Compiles clean.

Plan ready for execution.
