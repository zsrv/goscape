# NAI-197 — `.seq` + `.flo` + `.spotanim` + `.idk` packer slice — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `tools/pack/config/{SeqConfig,FloConfig,SpotAnimConfig,IdkConfig}.ts` onto the NAI-191–196 `PackShared` infrastructure. Adds four server+client+jagfile per-config packer branches to `PackConfigs` at TS-canonical positions, extending the NAI-196-D-UNCONDITIONAL-CLIENT-PACK pattern.

**Architecture:** Four new `pkg/pack/<config>.go` files (parser + packer per config). Additive extension of `pkg/pack/pack_configs.go` (four new lazy `ensureFoo` registry helpers; four new `packAndSaveFoo` functions; four new unconditional branches inserted in TS-canonical positions; one doc-comment scope extension). Atomic rewrite of `TestPackConfigs_ElevenConfigsLand` → `TestPackConfigs_FifteenConfigsLand`. Three new round-trip tests (`.flo` excluded — no `LoadFloTypes` loader exists). One new deviation-pin file.

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` + `pkg/io/jagfile` + NAI-191–196 `pkg/pack` foundation + `pkg/colorconv` (`Rgb15toHsl16`) + `pkg/objtype` (`LoadSeqTypes(dir, *SeqFrameConfigs)`, `LoadSpotanimTypes`, `LoadIdkTypes`).

**Spec:** `docs/superpowers/specs/2026-05-14-nai-197-seq-flo-spotanim-idk-packer-slice-design.md` (commit `d0f64a2`).
**HEAD at plan-write:** `d0f64a2`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/pack/*_test.go`: bare `if err != nil { t.Fatal(err) }`, `bytes.Equal` for byte comparison, `t.Fatalf("got % x, want % x", got, want)` for byte diffs, `t.TempDir()` for fixture roots, `ClearFsCache()` before tests that mutate the FS.
- **Existing helpers in `pkg/pack`** (use, do NOT redefine):
  - `writeFile(t *testing.T, path, content string)` — `constants_test.go:10`
  - `newTestPF(packType string, entries map[int]string) *PackFile` — `param_test.go:54`
  - `setupPackRoots(t, srcDir)` — `loc_roundtrip_test.go:63` — already stubs `varp/varn/vars/obj/npc/hunt/enum/interface/struct/spotanim/synth/dbrow`. **Round-trip tests in T7 must add `anim.pack`, `flo.pack`, `idk.pack` stubs themselves (or extend `setupPackRoots`).**
  - `scanPkgPack(t *testing.T) string` — `nai196_deviation_pins_test.go:13` — concatenated `.go` content of `pkg/pack/` excluding `_test.go`.
- **Modern Go** (per `[[use-modern-go]]`): `for id := range pf.Max`, `slices.Index`, `slices.Equal`, `strconv.ParseInt(_, 0, 64)`, `strings.HasPrefix`, `strings.HasSuffix`.
- **Identifier conventions** (mirroring NAI-196):
  - Per-config files: `seq.go`, `flo.go`, `spotanim.go`, `idk.go`.
  - Parsers (closure-bound by registries): `parseSeqConfigFor(animPack, objPack)`, `parseFloConfigFor(texturePack)`, `parseSpotAnimConfigFor(modelPack, seqPack)`, `parseIdkConfigFor(modelPack)`. Each returns `ParseFn` (`func(key, value string) (ConfigValue, bool, error)`).
  - Packers: `packSeqConfigs(configs, seqPack)`, `packFloConfigs(configs, floPack)`, `packSpotAnimConfigs(configs, spotanimPack)`, `packIdkConfigs(configs, idkPack)`. Each returns `(server, client *PackedData)`.
    > **Plan-author clarification of spec §4:** spec wrote `(server, client *PackedData, err error)`. None of the four TS packers have error conditions (no `param=` resolution, no type-assertion-on-ParamValue), so the new packers match TS by omitting the err return. This is a docstring-style narrowing, not a behavioral deviation.
  - Orchestrator helpers: `packAndSaveSeq`, `packAndSaveFlo`, `packAndSaveSpotAnim`, `packAndSaveIdk`.
  - Registry helpers (new): `ensureAnimPack`, `ensureFloPack`, `ensureSpotAnimPack`, `ensureIdkPack` (reuses existing `ensureSeqPack`, `ensureObjPack`, `ensureModelPack`, `ensureTexturePack`).
- **TS-fidelity discipline** (per `[[true_to_ts_gate]]`): per-config tasks do NOT codify TS opcode tables; each task block cites the TS file:line range and instructs the implementer to read TS directly.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `d0f64a2`:

| Premise | Verification |
|---|---|
| `pkg/pack.PackFile.Max int` is a struct field (not method), set by `RefreshNames()` | ✅ `packfile.go:35,162` |
| `pkg/pack.NewPackFile(srcDir, packType string, validator Validator) (*PackFile, error)` reads `<srcDir>/pack/<packType>.pack` | ✅ `packfile.go:45,76` |
| `pkg/pack.PackedData` has `NewPackedData(int)`, `Next()`, `P1/P2/P3/P4/PBool/PJStr`, `Save(dat, idx string)`, fields `Dat`/`Idx` | ✅ `packed_data.go:24-60` |
| `pkg/pack.PackFile.GetByName(name) int` returns `-1` for missing; `GetByID(id) string` returns `""` for missing | ✅ `packfile.go:188,192` |
| `pkg/pack.ParseFn = func(key, value string) (ConfigValue, bool, error)` (second return = "did we claim this key") | ✅ `read_typed.go:19` |
| `pkg/pack.ReadTypedConfigs(srcDir, ext, required, parseFn, c)` orchestrates the parse pass | ✅ `read_typed.go:37` |
| `pkg/pack.IsConfigBoolean(string) bool`, `GetConfigBoolean(string) bool` | ✅ `config_value.go:23-37` |
| `pkg/colorconv.Rgb15toHsl16(rgb int) int` exists | ✅ `colorconv.go:33` |
| `pkg/objtype.LoadSeqTypes(dir string, frames *SeqFrameConfigs)` — `dir` is parent of `server/` | ✅ `seqtype.go:126` |
| `pkg/objtype.SeqFrameConfigs{}` zero-value is valid (silent-on-missing) | ✅ `seqframe.go:21,33` |
| `pkg/objtype.LoadSpotanimTypes(dir string)` (lowercase `Spotanim` not `SpotAnim`) | ✅ `spotanimtype.go:93` |
| `pkg/objtype.LoadIdkTypes(dir string)` | ✅ `idktype.go:90` |
| No `pkg/objtype.LoadFloTypes` exists | ✅ `ls pkg/objtype/ \| grep -i flo` → no output |
| `pkg/pack.setupPackRoots(t, srcDir)` already stubs `spotanim.pack`; missing `anim.pack`/`flo.pack`/`idk.pack` | ✅ `loc_roundtrip_test.go:63-86` — confirmed |
| Existing `TestPackConfigs_ElevenConfigsLand` at `pack_configs_test.go:410` asserts 11 configs + 10 client jag entries with post-NAI-196 ordering | ✅ verified |
| Existing var-block at `pack_configs.go:92-102` holds 9 lazy `*PackFile` declarations and 9 `ensureFoo` closures | ✅ verified |
| `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment lives on `PackConfigs` (`pack_configs.go:37-44`) and enumerates 5 branches | ✅ verified |

**TS-side premises** (verified by reading the four TS files end-to-end):

| TS premise | Source line |
|---|---|
| `.seq` parser claims: `loops`, `priority`, `maxloops` (number); `stretches` (boolean); `frame{N}` / `iframe{N}` (AnimPack name → id); `delay{N}` (number); `walkmerge` (comma-split label list); `replaceheldleft` / `replaceheldright` (`'hide'` → 0 else `ObjPack.getByName + 512`) | `SeqConfig.ts:4-119` |
| `.seq` packer: opcodes 1,2,3,4,5,6,7,8 + server-side 250 trailer (gated `debugname.length`) | `SeqConfig.ts:121-208` |
| `.flo` parser claims: `colour` (number); `overlay`, `occlude` (boolean); `texture` (TexturePack name → id) | `FloConfig.ts:4-61` |
| `.flo` packer: client opcodes 1,2,3,5,6 (opcode 6 gated `!startsWith("flo_")`); **server side never writes opcodes — only `next()` per id** | `FloConfig.ts:63-104` — `server.p<N>(...)` does not appear anywhere |
| `.spotanim` parser claims: `resizeh`, `resizev`, `angle`, `ambient`, `contrast` (number with range checks); `hasalpha` (boolean); `model`, `anim` (registry names); `recol{N}{s,d}` (parser invokes `ColorConversion.rgb15toHsl16(parseInt(value))`) | `SpotAnimConfig.ts:5-90` |
| `.spotanim` packer: client opcodes 1,2,3,4,5,6,7,8 + (40+index)/(50+index) for recol s/d + server-side 250 trailer | `SpotAnimConfig.ts:92-152` |
| `.idk` parser claims: `disable` (boolean); `type` (man_hair/.../woman_feet 14-case switch → bodypart 0..13); `model{N}` / `head{N}` (ModelPack name → id); `recol{N}{s,d}` (parser invokes `ColorConversion.rgb15toHsl16(parseInt(value))`). `stringKeys` AND `numberKeys` are BOTH empty `[]` — both `NAI-195-D-DEADBRANCH-OMITTED` | `IdkConfig.ts:5-124` |
| `.idk` packer: per-id accumulates `models[]`/`heads[]`/`recol_s[]`/`recol_d[]`, emits recol_s opcodes 40+i, recol_d opcodes 50+i, heads opcodes 60+i, models opcode 2 + count + per-entry p2; client `type` → opcode 1+p1, `disable=true` → opcode 3; server-side 250 trailer | `IdkConfig.ts:126-206` |
| TS `packConfigs` body inserts the four branches inside `if (rebuildClient || shouldBuild(...))` blocks with `rebuildClient = true` at `PackShared.ts:337` — runs unconditionally | `PackShared.ts:454-475,500-521,523-544,592-613` |

---

## Pre-flight resolution of spec §9 ⚠️ risk-register rows

| Row | Resolution |
|---|---|
| R1 (`anim.pack` convention) | ✅ TS `PackFile.ts:193` declares `AnimPack` with `validateFilesPack`. `pack.load(`...`/pack/anim.pack`)` confirms the source-of-truth file is `<srcDir>/pack/anim.pack`. Goscape `ensureAnimPack` mirrors `ensureSeqPack` / `ensureObjPack` with `NewPackFile(srcDir, "anim", nil)`. |
| R2/R11 (`.flo` empty server emission) | ✅ TS `FloConfig.ts:63-104` server `PackedData` is constructed (line 65) and per-id `server.next()` is called (line 100), but no `server.p<N>(...)` calls appear anywhere in the body. **The four-branch shape is non-uniform: `.flo` server is opcode-empty; `.seq` / `.spotanim` / `.idk` emit the 250-trailer + pjstr(debugname) when `debugname.length > 0`.** Plan-author flags this asymmetry in T3's code block and asserts it with a byte-pin test. |
| R6 (ColorConversion sites) | ✅ TS `SpotAnimConfig.ts:86` and `IdkConfig.ts:120` invoke `ColorConversion.rgb15toHsl16` at the **parser** (not the writer). Goscape parsers must call `colorconv.Rgb15toHsl16(int(n))` and store the converted value in the `ConfigLine`. Goscape packers emit `client.P2(uint16(value.(int)))` without further conversion. NAI-196 `.loc` parser-side recol (`loc.go:145`) is the precedent; do NOT mimic the `.obj` / `.npc` writer-site conversion. |
| R7 (dead branches per config) | ✅ Verified: `.seq` has empty `stringKeys: []` (`SeqConfig.ts:5`); `.flo` has empty `stringKeys: []` (`FloConfig.ts:5`); `.spotanim` has empty `stringKeys: []` (`SpotAnimConfig.ts:6`); `.idk` has empty `stringKeys: []` **and** empty `numberKeys: []` (`IdkConfig.ts:6-8`). Goscape parsers omit these branches; `NAI-195-D-DEADBRANCH-OMITTED` covers them. |
| R9 (15-config fixture `.pack` list) | ✅ `setupPackRoots` already stubs `spotanim.pack` (line 83). T6's atomic test rewrite must add `anim.pack`, `flo.pack`, `idk.pack` to `TestPackConfigs_FifteenConfigsLand`'s fixture builder. |

All ⚠️ rows resolved at plan-write per `[[risk_register_premise_grep]]`. Implementer tasks do not need to re-trace.

---

## File inventory

```
pkg/pack/
  seq.go                                   NEW    (parseSeqConfigFor + packSeqConfigs)
  seq_test.go                              NEW    (parser + packer byte-pin tests)
  seq_roundtrip_test.go                    NEW    (PackConfigs → LoadSeqTypes round-trip)
  flo.go                                   NEW    (parseFloConfigFor + packFloConfigs)
  flo_test.go                              NEW    (parser + packer byte-pin tests, incl. empty-server pin + opcode-6 dual-asymmetry pin)
  spotanim.go                              NEW    (parseSpotAnimConfigFor + packSpotAnimConfigs)
  spotanim_test.go                         NEW    (parser + packer byte-pin tests)
  spotanim_roundtrip_test.go               NEW    (PackConfigs → LoadSpotanimTypes round-trip)
  idk.go                                   NEW    (parseIdkConfigFor + packIdkConfigs)
  idk_test.go                              NEW    (parser + packer byte-pin tests)
  idk_roundtrip_test.go                    NEW    (PackConfigs → LoadIdkTypes round-trip)
  pack_configs.go                          MODIFY (add 4 lazy vars + 4 ensureFoo closures + 4 packAndSaveFoo functions + 4 unconditional branches; extend NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment scope)
  pack_configs_test.go                     MODIFY (in-place rewrite of TestPackConfigs_ElevenConfigsLand → TestPackConfigs_FifteenConfigsLand)
  nai197_deviation_pins_test.go            NEW    (1 presence pin re-asserting the extended UNCONDITIONAL-CLIENT-PACK scope)
```

(No round-trip file for `.flo` — `pkg/objtype.LoadFloTypes` does not exist; byte-pin coverage is the contract per spec §8.2.)

---

## Task overview

| T | Subject | Test files | Production files | Est. commits |
|---|---|---|---|---|
| T1 | Add 4 lazy `ensureFoo` helpers (`Anim`/`Flo`/`SpotAnim`/`Idk`) — additive, no callers yet | (none) | `pack_configs.go` | 1 |
| T2 | `.seq` parser + packer + byte-pin tests | `seq_test.go` | `seq.go` | 1–2 |
| T3 | `.flo` parser + packer + byte-pin tests (incl. empty-server-bytes pin + opcode-6 dual-asymmetry pin) | `flo_test.go` | `flo.go` | 1–2 |
| T4 | `.spotanim` parser + packer + byte-pin tests | `spotanim_test.go` | `spotanim.go` | 1–2 |
| T5 | `.idk` parser + packer + byte-pin tests | `idk_test.go` | `idk.go` | 1–2 |
| T6 | `PackConfigs` wiring: 4 new branches in TS-canonical order + 4 new `packAndSaveFoo` + extend doc-comment scope; atomic rewrite of `_ElevenConfigsLand` → `_FifteenConfigsLand` | `pack_configs_test.go` (in-place) | `pack_configs.go` | 1 |
| T7 | Round-trip tests for `.seq`, `.spotanim`, `.idk` (3 files; `.flo` excluded per §8.2) | `seq_roundtrip_test.go`, `spotanim_roundtrip_test.go`, `idk_roundtrip_test.go` | (none — exercises T1–T6) | 1–2 |
| T8 | Deviation-tag pin: presence re-assertion that `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` references all 9 client+server branches | `nai197_deviation_pins_test.go` | (none) | 1 |

---

## Task 1: Lazy `ensureFoo` registry helpers (additive, no callers)

**Files:**
- Modify: `pkg/pack/pack_configs.go` (function `PackConfigs`)

This task additively introduces four new lazy registry helpers and accompanying `var` declarations. They are NOT yet called from any branch — T6 wires them. The build must remain green after this task.

- [ ] **Step 1.1: Inspect existing helpers**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -n "ensureLocPack\|ensureModelPack\|ensureTexturePack\|var (\|huntPack" /home/owner/Code/github.com/zsrv/goscape/pkg/pack/pack_configs.go
```
Expected: var-block around lines 92-102 (9 declarations), closures lines 103-201 (9 closures).

- [ ] **Step 1.2: Extend the var-block**

Locate the existing var-block at `pack_configs.go:92-102` (begins `var (` line 92, ends `)` line 102). After the existing `texturePack  *PackFile` line and before the closing `)`, add 4 new declarations:

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
)
```

- [ ] **Step 1.3: Add four new `ensureFoo` closures**

Immediately after the existing `ensureTexturePack` closure (around `pack_configs.go:201`), add the four new closures:

```go
ensureAnimPack := func() error {
	if animPack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "anim", nil)
	if err != nil {
		return err
	}
	animPack = pf
	return nil
}
ensureFloPack := func() error {
	if floPack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "flo", nil)
	if err != nil {
		return err
	}
	floPack = pf
	return nil
}
ensureSpotAnimPack := func() error {
	if spotanimPack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "spotanim", nil)
	if err != nil {
		return err
	}
	spotanimPack = pf
	return nil
}
ensureIdkPack := func() error {
	if idkPack != nil {
		return nil
	}
	pf, err := NewPackFile(srcDir, "idk", nil)
	if err != nil {
		return err
	}
	idkPack = pf
	return nil
}
```

- [ ] **Step 1.4: Suppress "declared and not used" diagnostics**

Immediately after the four new closures, still inside `PackConfigs`, add:

```go
// NAI-197 T1: helpers landed without callers; T6 wires them. Suppress
// unused-variable diagnostics until then.
_ = ensureAnimPack
_ = ensureFloPack
_ = ensureSpotAnimPack
_ = ensureIdkPack
```

- [ ] **Step 1.5: Verify build**

Run:
```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: clean exit, no errors.

- [ ] **Step 1.6: Verify existing tests pass**

Run:
```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```
Expected: all green.

- [ ] **Step 1.7: Commit**

```bash
git add pkg/pack/pack_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-197 T1 — ensureFoo helpers for anim/flo/spotanim/idk

Adds four lazy registry-helper closures + var declarations inside
PackConfigs. Suppressed via _ = ensureFoo until T6 wires the new branches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `.seq` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/seq.go`
- Create: `pkg/pack/seq_test.go`

**TS source (READ END-TO-END BEFORE STARTING):** `Engine-TS/tools/pack/config/SeqConfig.ts:1-208`. Parser at `:4-119`; packer at `:121-207`.

**Implementer authority:** the TS file is the canonical opcode/key reference. Do NOT rely on this plan for the opcode table. Read TS, port literally. Notable shape details:

- Parser claims: `loops`, `priority`, `maxloops` (numeric with range checks 0..1000 / 0..10 / 0..1000); `stretches` (boolean); `frame{N}` / `iframe{N}` (AnimPack name → id, `-1` rejects); `delay{N}` (number, `isNaN` rejects); `walkmerge` (comma-split list; each token has form `label_<digits>`, the parser parses `parseInt(after_underscore)` per token, returning `[]int`); `replaceheldleft` / `replaceheldright` (literal `'hide'` → 0; else `ObjPack.getByName + 512`, `-1` rejects). Dead branch: `stringKeys: []` — omit (per `NAI-195-D-DEADBRANCH-OMITTED`).
- Packer (per `SeqConfig.ts:121-208`): per-id accumulates `frames[]`/`iframes[]`/`delays[]` arrays then emits opcode 1 with length + per-element `p2(frame) + p2(iframe ?? -1) + p2(delay ?? 0)`. Loose-emit opcodes: `loops` → 2+p2; `walkmerge` → 3 + p1(len) + per-label p1; `stretches=true` → 4 (no payload); `priority` → 5+p1; `replaceheldleft` → 6+p2; `replaceheldright` → 7+p2; `maxloops` → 8+p1. Server-side 250-trailer + pjstr(debugname) when `debugname.length > 0`.

**Plan-author dead-branch directive (per `[[true_to_ts_gate]]`):** the parser MUST omit the `stringKeys` empty branch. Tag the omission with a doc-comment referencing `NAI-195-D-DEADBRANCH-OMITTED`.

### Step 2.1: Implement parser

- [ ] **Step 2.1.1: Write parser-side test skeleton (`pkg/pack/seq_test.go`)**

```go
package pack

import (
	"bytes"
	"testing"
)

// seqTestRegistries returns AnimPack + ObjPack fixtures.
func seqTestRegistries(t *testing.T) (animPack, objPack *PackFile) {
	t.Helper()
	animPack = newTestPF("anim", map[int]string{
		0: "frame_zero",
		1: "frame_one",
		2: "iframe_zero",
	})
	objPack = newTestPF("obj", map[int]string{
		0: "sword",
		1: "shield",
	})
	return
}

func TestParseSeqConfig_Loops(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	val, accepted, err := parse("loops", "5")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("loops should be accepted")
	}
	if val.(int) != 5 {
		t.Fatalf("got %#v, want int(5)", val)
	}
}

func TestParseSeqConfig_LoopsRangeReject(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	if _, _, err := parse("loops", "1001"); err == nil {
		t.Fatal("loops=1001 should reject (TS upper bound 1000)")
	}
}

func TestParseSeqConfig_Frame(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	val, accepted, err := parse("frame1", "frame_zero")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("frame1 should be accepted")
	}
	if val.(int) != 0 {
		t.Fatalf("got %#v, want int(0)", val)
	}
}

func TestParseSeqConfig_FrameUnknown(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	if _, _, err := parse("frame1", "no_such_anim"); err == nil {
		t.Fatal("unknown anim should reject")
	}
}

func TestParseSeqConfig_Walkmerge(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	val, accepted, err := parse("walkmerge", "label_3,label_7,label_11")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("walkmerge should be accepted")
	}
	got, ok := val.([]int)
	if !ok {
		t.Fatalf("got %#v, want []int", val)
	}
	want := []int{3, 7, 11}
	if len(got) != len(want) || got[0] != 3 || got[1] != 7 || got[2] != 11 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseSeqConfig_ReplaceHeldLeft_Hide(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	val, accepted, err := parse("replaceheldleft", "hide")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("replaceheldleft=hide should be accepted")
	}
	if val.(int) != 0 {
		t.Fatalf("got %#v, want int(0)", val)
	}
}

func TestParseSeqConfig_ReplaceHeldRight_ObjPlus512(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	val, accepted, err := parse("replaceheldright", "shield")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("replaceheldright should be accepted")
	}
	// shield is obj id 1 → 1 + 512 = 513.
	if val.(int) != 513 {
		t.Fatalf("got %#v, want int(513)", val)
	}
}

func TestParseSeqConfig_UnknownKey(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	_, accepted, err := parse("zzz_unknown", "anything")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("unknown key should not be claimed")
	}
}
```

- [ ] **Step 2.1.2: Run tests, observe red**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseSeqConfig -count=1
```
Expected: compile error — `parseSeqConfigFor` undefined.

- [ ] **Step 2.1.3: Implement parser in `pkg/pack/seq.go`**

Read `Engine-TS/tools/pack/config/SeqConfig.ts:1-119` end-to-end first. Then port literally. Skeleton:

```go
package pack

import (
	"fmt"
	"strconv"
	"strings"
)

// seqNumberKeys mirrors TS SeqConfig.ts:7-9 numberKeys[].
var seqNumberKeys = map[string]struct{}{
	"loops":    {},
	"priority": {},
	"maxloops": {},
}

// seqBooleanKeys mirrors TS SeqConfig.ts:11-13 booleanKeys[].
var seqBooleanKeys = map[string]struct{}{
	"stretches": {},
}

// parseSeqConfigFor returns the per-key=value parser for .seq config
// blocks. Closure-captures animPack (for frame{N}/iframe{N}) and
// objPack (for replaceheldleft/replaceheldright).
//
// NAI-195-D-DEADBRANCH-OMITTED: TS SeqConfig.ts:5 declares empty
// stringKeys[] — omitted here; revives if schema adds string keys.
//
// TS source: tools/pack/config/SeqConfig.ts:4-119.
func parseSeqConfigFor(animPack, objPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := seqNumberKeys[key]; ok {
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s", key, value)
			}
			// Per-key range checks: SeqConfig.ts:44-54.
			switch key {
			case "loops", "maxloops":
				if n < 0 || n > 1000 {
					return nil, true, fmt.Errorf("%s out of range [0,1000]: %d", key, n)
				}
			case "priority":
				if n < 0 || n > 10 {
					return nil, true, fmt.Errorf("%s out of range [0,10]: %d", key, n)
				}
			}
			return int(n), true, nil
		}
		if _, ok := seqBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s", key, value)
			}
			return GetConfigBoolean(value), true, nil
		}
		if strings.HasPrefix(key, "iframe") {
			idx := animPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown anim: %s", value)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "frame") {
			idx := animPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown anim: %s", value)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "delay") {
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, true, fmt.Errorf("invalid delay: %s", value)
			}
			return n, true, nil
		}
		if key == "walkmerge" {
			// TS SeqConfig.ts:84-93: split on ',', parseInt(part.substring(part.indexOf('_')+1)).
			parts := strings.Split(value, ",")
			labels := make([]int, 0, len(parts))
			for _, part := range parts {
				underscore := strings.Index(part, "_")
				if underscore == -1 {
					// TS parseInt returns NaN here; mirror by rejecting.
					return nil, true, fmt.Errorf("invalid walkmerge label: %s", part)
				}
				n, err := strconv.Atoi(part[underscore+1:])
				if err != nil {
					return nil, true, fmt.Errorf("invalid walkmerge label: %s", part)
				}
				labels = append(labels, n)
			}
			return labels, true, nil
		}
		if key == "replaceheldleft" || key == "replaceheldright" {
			if value == "hide" {
				return 0, true, nil
			}
			idx := objPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown obj: %s", value)
			}
			return idx + 512, true, nil
		}
		return nil, false, nil
	}
}
```

> **Plan-author note (per `[[asymmetric_test_pattern_audit]]`):** TS parses `iframe` BEFORE `frame` (else `frame` would consume `iframe1` via `startsWith`). Go port must mirror — see `iframe` branch above placed before `frame`.

- [ ] **Step 2.1.4: Run parser tests, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseSeqConfig -count=1
```
Expected: all parser tests pass.

### Step 2.2: Implement packer

- [ ] **Step 2.2.1: Add packer-side byte-pin tests to `pkg/pack/seq_test.go`**

```go
// seqOneSlotPack mirrors objOneSlotPack — single-slot fixture.
func seqOneSlotPack(name string) *PackFile {
	return newTestPF("seq", map[int]string{0: name})
}

// seqServerDebugTrailer: 2-byte size header + opcode 250 + pjstr(name) + Next 0x00.
func seqServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01}
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

func TestPackSeqConfigs_Loops(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "loops", Value: 5}},
	}
	_, client := packSeqConfigs(configs, seqPack)
	// opcode 2 + p2(5) + Next 0x00, with 2-byte size header.
	want := []byte{0x00, 0x01, 0x02, 0x00, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_ServerDebugTrailer(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{}
	server, _ := packSeqConfigs(configs, seqPack)
	if !bytes.Equal(server.Dat.Data, seqServerDebugTrailer("walk")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, seqServerDebugTrailer("walk"))
	}
}

func TestPackSeqConfigs_NoDebugnameNoTrailer(t *testing.T) {
	// Empty debugname → server should be only Next() boundary (no 250+pjstr).
	seqPack := newTestPF("seq", map[int]string{}) // empty map → Max=0
	_ = seqPack
	// Use a 1-slot pack with empty name via newTestPF tricks:
	pf := newTestPF("seq", map[int]string{0: ""}) // explicit empty name
	configs := map[string][]ConfigLine{}
	server, _ := packSeqConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x00} // size header + Next() only
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackSeqConfigs_FramesIframesDelays(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {
			{Key: "frame1", Value: 10},
			{Key: "frame2", Value: 11},
			{Key: "iframe1", Value: 20},
			{Key: "delay2", Value: 7},
		},
	}
	_, client := packSeqConfigs(configs, seqPack)
	// opcode 1 + p1(2) + per-element [p2(frame), p2(iframe ?? -1), p2(delay ?? 0)]:
	//   element 0: frame=10, iframe=20, delay=0  → 00 0A 00 14 00 00
	//   element 1: frame=11, iframe=-1, delay=7  → 00 0B FF FF 00 07
	want := []byte{
		0x00, 0x01, // size header
		0x01, 0x02, // opcode 1, length 2
		0x00, 0x0A, 0x00, 0x14, 0x00, 0x00,
		0x00, 0x0B, 0xFF, 0xFF, 0x00, 0x07,
		0x00, // Next()
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_ReplaceHeldRight_Hide(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "replaceheldright", Value: 0}}, // 'hide' literal parser-stored as 0
	}
	_, client := packSeqConfigs(configs, seqPack)
	want := []byte{0x00, 0x01, 0x07, 0x00, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_ReplaceHeldRight_ObjPlus512(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "replaceheldright", Value: 513}}, // obj id 1 + 512
	}
	_, client := packSeqConfigs(configs, seqPack)
	want := []byte{0x00, 0x01, 0x07, 0x02, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_Walkmerge(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "walkmerge", Value: []int{3, 7, 11}}},
	}
	_, client := packSeqConfigs(configs, seqPack)
	// opcode 3 + p1(3) + p1(3) + p1(7) + p1(11) + Next 0x00
	want := []byte{0x00, 0x01, 0x03, 0x03, 0x03, 0x07, 0x0B, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}
```

- [ ] **Step 2.2.2: Run tests, observe red**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackSeqConfigs -count=1
```
Expected: `packSeqConfigs undefined`.

- [ ] **Step 2.2.3: Implement packer in `pkg/pack/seq.go`**

Read `Engine-TS/tools/pack/config/SeqConfig.ts:121-208` first. Port literally. Skeleton:

```go
// packSeqConfigs emits the per-id body for .seq configs. Per-id loop
// accumulates frames/iframes/delays arrays then emits opcode 1 at the
// end; loose-emit opcodes 2..8 are emitted in-loop. Server side emits
// opcode 250 + pjstr(debugname) when debugname.length > 0.
//
// TS source: tools/pack/config/SeqConfig.ts:121-208.
func packSeqConfigs(configs map[string][]ConfigLine, seqPack *PackFile) (server, client *PackedData) {
	server = NewPackedData(seqPack.Max)
	client = NewPackedData(seqPack.Max)

	for id := range seqPack.Max {
		debugname := seqPack.GetByID(id)
		cfg := configs[debugname]

		// Per-id accumulators (TS SeqConfig.ts:131-133).
		var frames, iframes, delays []int
		hasIframe := map[int]bool{}
		hasDelay := map[int]bool{}

		for _, line := range cfg {
			switch {
			case strings.HasPrefix(line.Key, "iframe"):
				// Read index = atoi(after "iframe") - 1.
				idx, err := strconv.Atoi(line.Key[len("iframe"):])
				if err != nil {
					continue
				}
				idx--
				for len(iframes) <= idx {
					iframes = append(iframes, 0)
				}
				iframes[idx] = line.Value.(int)
				hasIframe[idx] = true
			case strings.HasPrefix(line.Key, "frame"):
				idx, err := strconv.Atoi(line.Key[len("frame"):])
				if err != nil {
					continue
				}
				idx--
				for len(frames) <= idx {
					frames = append(frames, 0)
				}
				frames[idx] = line.Value.(int)
			case strings.HasPrefix(line.Key, "delay"):
				idx, err := strconv.Atoi(line.Key[len("delay"):])
				if err != nil {
					continue
				}
				idx--
				for len(delays) <= idx {
					delays = append(delays, 0)
				}
				delays[idx] = line.Value.(int)
				hasDelay[idx] = true
			case line.Key == "loops":
				client.P1(2)
				client.P2(uint16(line.Value.(int)))
			case line.Key == "walkmerge":
				labels := line.Value.([]int)
				client.P1(3)
				client.P1(uint8(len(labels)))
				for _, lab := range labels {
					client.P1(uint8(lab))
				}
			case line.Key == "stretches":
				if line.Value.(bool) {
					client.P1(4)
				}
			case line.Key == "priority":
				client.P1(5)
				client.P1(uint8(line.Value.(int)))
			case line.Key == "replaceheldleft":
				client.P1(6)
				client.P2(uint16(line.Value.(int)))
			case line.Key == "replaceheldright":
				client.P1(7)
				client.P2(uint16(line.Value.(int)))
			case line.Key == "maxloops":
				client.P1(8)
				client.P1(uint8(line.Value.(int)))
			}
		}

		if len(frames) > 0 {
			client.P1(1)
			client.P1(uint8(len(frames)))
			for j := range frames {
				client.P2(uint16(frames[j]))
				if hasIframe[j] {
					client.P2(uint16(iframes[j]))
				} else {
					client.P2(uint16(0xFFFF)) // -1
				}
				if hasDelay[j] {
					client.P2(uint16(delays[j]))
				} else {
					client.P2(0)
				}
			}
		}

		if len(debugname) > 0 {
			server.P1(250)
			server.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}
	return server, client
}
```

> **Plan-author hazard (per `[[plan_var_name_collision]]`):** `seqPack` is the parameter; do NOT shadow it with `:=` inside this function. The outer `PackConfigs` lazy `seqPack *PackFile` variable is unrelated to this scope.

- [ ] **Step 2.2.4: Run all seq tests, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestParseSeqConfig|TestPackSeqConfigs" -count=1
```
Expected: all pass.

- [ ] **Step 2.2.5: Run full pkg/pack suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```
Expected: all green.

- [ ] **Step 2.3: Commit**

```bash
git add pkg/pack/seq.go pkg/pack/seq_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-197 T2 — .seq parser + packer + byte-pin tests

Ports tools/pack/config/SeqConfig.ts. parseSeqConfigFor closure-binds
animPack + objPack; packSeqConfigs emits client opcodes 1..8 and the
server-side 250-trailer when debugname is non-empty.

NAI-195-D-DEADBRANCH-OMITTED: empty TS stringKeys[] branch omitted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `.flo` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/flo.go`
- Create: `pkg/pack/flo_test.go`

**TS source (READ END-TO-END):** `Engine-TS/tools/pack/config/FloConfig.ts:1-104`. Parser at `:4-61`; packer at `:63-104`.

**Critical asymmetry — flagged at plan-author level per spec §9 R2/R11:** the `.flo` server-side `PackedData` has ZERO opcode writes anywhere in its body — only `server.next()` is called per id (`FloConfig.ts:100`). **The 250-trailer + pjstr(debugname) pattern present in `.seq` / `.spotanim` / `.idk` does NOT apply here.** `.flo`'s debugname instead emits as **client-side opcode 6** with gate `debugname.length && !startsWith('flo_')` (`FloConfig.ts:93-97`).

> **Implementer directive:** do NOT add a `server.P1(250)` block to `packFloConfigs` "by analogy" with the other three configs. The pin tests below explicitly enforce empty-server-bytes-per-id.

### Step 3.1: Implement parser

- [ ] **Step 3.1.1: Write parser-side tests in `pkg/pack/flo_test.go`**

```go
package pack

import (
	"bytes"
	"testing"
)

func floTestRegistries(t *testing.T) (texturePack *PackFile) {
	t.Helper()
	texturePack = newTestPF("texture", map[int]string{
		0: "wood",
		1: "stone",
	})
	return
}

func TestParseFloConfig_Colour(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)

	val, accepted, err := parse("colour", "0xFF00AA")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("colour should be accepted")
	}
	if val.(int) != 0xFF00AA {
		t.Fatalf("got %#v, want int(0xFF00AA)", val)
	}
}

func TestParseFloConfig_Overlay(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)

	val, accepted, err := parse("overlay", "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("overlay should be accepted")
	}
	if val.(bool) != true {
		t.Fatalf("got %#v, want true", val)
	}
}

func TestParseFloConfig_Texture(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)

	val, accepted, err := parse("texture", "stone")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("texture should be accepted")
	}
	if val.(int) != 1 {
		t.Fatalf("got %#v, want int(1)", val)
	}
}

func TestParseFloConfig_UnknownKey(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)

	_, accepted, err := parse("zzz", "value")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("unknown key should not be claimed")
	}
}
```

- [ ] **Step 3.1.2: Implement parser in `pkg/pack/flo.go`**

Read `Engine-TS/tools/pack/config/FloConfig.ts:4-61` first. Port:

```go
package pack

import (
	"fmt"
	"strconv"
	"strings"
)

// floNumberKeys mirrors TS FloConfig.ts:7-9 numberKeys[].
var floNumberKeys = map[string]struct{}{
	"colour": {},
}

// floBooleanKeys mirrors TS FloConfig.ts:11-13 booleanKeys[].
var floBooleanKeys = map[string]struct{}{
	"overlay": {},
	"occlude": {},
}

// parseFloConfigFor returns the per-key=value parser for .flo config
// blocks. Closure-captures texturePack (for the `texture` key).
//
// NAI-195-D-DEADBRANCH-OMITTED: TS FloConfig.ts:5 declares empty
// stringKeys[] — omitted.
//
// TS source: tools/pack/config/FloConfig.ts:4-61.
func parseFloConfigFor(texturePack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := floNumberKeys[key]; ok {
			// TS parseInt(value, 16) when value.startsWith('0x'); else parseInt(value).
			var n int64
			var err error
			if strings.HasPrefix(value, "0x") {
				n, err = strconv.ParseInt(value[2:], 16, 64)
			} else {
				n, err = strconv.ParseInt(value, 10, 64)
			}
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s", key, value)
			}
			return int(n), true, nil
		}
		if _, ok := floBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s", key, value)
			}
			return GetConfigBoolean(value), true, nil
		}
		if key == "texture" {
			idx := texturePack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown texture: %s", value)
			}
			return idx, true, nil
		}
		return nil, false, nil
	}
}
```

- [ ] **Step 3.1.3: Run parser tests, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseFloConfig -count=1
```
Expected: all pass.

### Step 3.2: Implement packer

- [ ] **Step 3.2.1: Add packer tests to `pkg/pack/flo_test.go`**

```go
func floOneSlotPack(name string) *PackFile {
	return newTestPF("flo", map[int]string{0: name})
}

func TestPackFloConfigs_Colour(t *testing.T) {
	floPack := floOneSlotPack("red")
	configs := map[string][]ConfigLine{
		"red": {{Key: "colour", Value: 0xFF0000}},
	}
	_, client := packFloConfigs(configs, floPack)
	// opcode 1 + p3(0xFF0000) + opcode 6 (debugname "red" doesn't start with "flo_")
	// + pjstr("red") + Next 0x00.
	want := []byte{
		0x00, 0x01, // size header
		0x01, 0xFF, 0x00, 0x00,
		0x06, 'r', 'e', 'd', 0x0A,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_Texture(t *testing.T) {
	floPack := floOneSlotPack("flo_dirt") // starts with "flo_" → opcode 6 suppressed
	configs := map[string][]ConfigLine{
		"flo_dirt": {{Key: "texture", Value: 3}},
	}
	_, client := packFloConfigs(configs, floPack)
	// opcode 2 + p1(3) + (no opcode 6) + Next 0x00.
	want := []byte{0x00, 0x01, 0x02, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OverlayTrueEmits(t *testing.T) {
	floPack := floOneSlotPack("flo_x") // starts with "flo_" → opcode 6 suppressed
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "overlay", Value: true}},
	}
	_, client := packFloConfigs(configs, floPack)
	want := []byte{0x00, 0x01, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OverlayFalseNoEmit(t *testing.T) {
	// TS FloConfig.ts:82-84 emits opcode 3 ONLY when value === true.
	floPack := floOneSlotPack("flo_x")
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "overlay", Value: false}},
	}
	_, client := packFloConfigs(configs, floPack)
	want := []byte{0x00, 0x01, 0x00} // empty body
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OccludeFalseEmits(t *testing.T) {
	// TS FloConfig.ts:86-88: opcode 5 fires when occlude === false.
	floPack := floOneSlotPack("flo_x")
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "occlude", Value: false}},
	}
	_, client := packFloConfigs(configs, floPack)
	want := []byte{0x00, 0x01, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OccludeTrueNoEmit(t *testing.T) {
	floPack := floOneSlotPack("flo_x")
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "occlude", Value: true}},
	}
	_, client := packFloConfigs(configs, floPack)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// Dual-asymmetry pin for opcode 6: presence + absence based on the
// debugname.startsWith('flo_') gate. Per [[ts_asymmetry_dual_pin]].
func TestPackFloConfigs_Opcode6_Asymmetry(t *testing.T) {
	cases := []struct {
		name    string
		want    []byte
	}{
		{"red", []byte{0x00, 0x01, 0x06, 'r', 'e', 'd', 0x0A, 0x00}},
		{"flo_dirt", []byte{0x00, 0x01, 0x00}}, // starts with "flo_" → no opcode 6
	}
	for _, tc := range cases {
		floPack := floOneSlotPack(tc.name)
		_, client := packFloConfigs(map[string][]ConfigLine{}, floPack)
		if !bytes.Equal(client.Dat.Data, tc.want) {
			t.Errorf("client[%s]:\n got % x\nwant % x", tc.name, client.Dat.Data, tc.want)
		}
	}
}

// Spec §9 R2: .flo server PackedData NEVER emits opcode bytes.
// For an N-id flo source, server.Dat = 2-byte size header + N × Next() byte (0x00).
func TestPackFloConfigs_EmptyServerBytes(t *testing.T) {
	// Three slots, all with non-empty debugnames (would trigger 250-trailer on
	// .seq/.spotanim/.idk). Server must STILL have only size-header + 3 × 0x00.
	floPack := newTestPF("flo", map[int]string{
		0: "red",
		1: "blue",
		2: "green",
	})
	server, _ := packFloConfigs(map[string][]ConfigLine{}, floPack)
	want := []byte{0x00, 0x03, 0x00, 0x00, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server (empty-bytes invariant):\n got % x\nwant % x", server.Dat.Data, want)
	}
}
```

- [ ] **Step 3.2.2: Run, observe red**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackFloConfigs -count=1
```
Expected: `packFloConfigs undefined`.

- [ ] **Step 3.2.3: Implement packer in `pkg/pack/flo.go`**

Read `Engine-TS/tools/pack/config/FloConfig.ts:63-104` first. Port:

```go
// packFloConfigs emits the per-id body for .flo configs.
//
// CRITICAL: server-side has NO opcode emission anywhere in the body.
// TS FloConfig.ts:64-65 constructs `server: PackedData` and the only
// server call inside the per-id loop is `server.next()` (line 100).
// Do NOT add a 250-trailer "by analogy" with .seq/.spotanim/.idk.
//
// The debugname trailer for .flo lives on the CLIENT side as opcode 6,
// gated `debugname.length && !startsWith('flo_')` (TS FloConfig.ts:93-97).
//
// TS source: tools/pack/config/FloConfig.ts:63-104.
func packFloConfigs(configs map[string][]ConfigLine, floPack *PackFile) (server, client *PackedData) {
	server = NewPackedData(floPack.Max)
	client = NewPackedData(floPack.Max)

	for id := range floPack.Max {
		debugname := floPack.GetByID(id)
		cfg := configs[debugname]

		for _, line := range cfg {
			switch line.Key {
			case "colour":
				client.P1(1)
				client.P3(uint32(line.Value.(int)))
			case "texture":
				client.P1(2)
				client.P1(uint8(line.Value.(int)))
			case "overlay":
				if line.Value.(bool) {
					client.P1(3)
				}
			case "occlude":
				if !line.Value.(bool) {
					client.P1(5)
				}
			}
		}

		if len(debugname) > 0 && !strings.HasPrefix(debugname, "flo_") {
			// TS FloConfig.ts:93 — "yes, this was originally transmitted!"
			client.P1(6)
			client.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}
	return server, client
}
```

- [ ] **Step 3.2.4: Run all flo tests, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestParseFloConfig|TestPackFloConfigs" -count=1
```
Expected: all pass.

- [ ] **Step 3.2.5: Run full pkg/pack**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```
Expected: all green.

- [ ] **Step 3.3: Commit**

```bash
git add pkg/pack/flo.go pkg/pack/flo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-197 T3 — .flo parser + packer + byte-pin tests

Ports tools/pack/config/FloConfig.ts. parseFloConfigFor closure-binds
texturePack; packFloConfigs emits client opcodes 1,2,3,5,6 with the
opcode-6 debugname gate (!startsWith("flo_")). Server side emits zero
opcodes — only per-id Next() boundaries — diverging from the
250-trailer pattern of the other three configs in this slice.

Byte-pin tests cover the empty-server-bytes invariant and the
dual-asymmetry of the opcode-6 emission gate.

NAI-195-D-DEADBRANCH-OMITTED: empty TS stringKeys[] branch omitted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `.spotanim` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/spotanim.go`
- Create: `pkg/pack/spotanim_test.go`

**TS source (READ END-TO-END):** `Engine-TS/tools/pack/config/SpotAnimConfig.ts:1-152`. Parser at `:5-90`; packer at `:92-151`.

**Critical detail per spec §9 R6:** TS `SpotAnimConfig.ts:86` invokes `ColorConversion.rgb15toHsl16(parseInt(value))` at the **parser**. The packer just emits the stored converted value via `p2`. Goscape parser must call `colorconv.Rgb15toHsl16(int(n))` and store the converted result in the `ConfigLine`. (Precedent: `loc.go:145`.)

**Range checks (per `SpotAnimConfig.ts:47-57`):** `resizeh`/`resizev` must be `0..512`; `angle` must be `0..360`; `ambient`/`contrast` must be `-128..127`.

### Step 4.1: Implement parser

- [ ] **Step 4.1.1: Write parser tests in `pkg/pack/spotanim_test.go`**

```go
package pack

import (
	"bytes"
	"testing"
)

func spotanimTestRegistries(t *testing.T) (modelPack, seqPack *PackFile) {
	t.Helper()
	modelPack = newTestPF("model", map[int]string{
		0: "m_zero",
		1: "m_one",
	})
	seqPack = newTestPF("seq", map[int]string{
		0: "anim_zero",
	})
	return
}

func TestParseSpotAnimConfig_Model(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)

	val, accepted, err := parse("model", "m_one")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("model should be accepted")
	}
	if val.(int) != 1 {
		t.Fatalf("got %#v, want int(1)", val)
	}
}

func TestParseSpotAnimConfig_AngleRange(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)

	if _, _, err := parse("angle", "361"); err == nil {
		t.Fatal("angle=361 should reject (TS range 0..360)")
	}
}

func TestParseSpotAnimConfig_AmbientNegativeOK(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)

	val, _, err := parse("ambient", "-100")
	if err != nil {
		t.Fatal(err)
	}
	if val.(int) != -100 {
		t.Fatalf("got %#v, want int(-100)", val)
	}
}

func TestParseSpotAnimConfig_Recol_ColorConvertedAtParser(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)

	// TS SpotAnimConfig.ts:86: rgb15toHsl16(parseInt(value)).
	// Don't hardcode the converted value — call the same conversion
	// the production code path calls, and compare against it.
	val, accepted, err := parse("recol1s", "0x1234")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("recol1s should be accepted")
	}
	// Confirm the stored value was passed through colorconv.Rgb15toHsl16.
	// Re-derive from the literal input the parser saw.
	wantInputRgb15 := 0x1234
	wantConverted := /* colorconv.Rgb15toHsl16 */ recolExpected(wantInputRgb15)
	if val.(int) != wantConverted {
		t.Fatalf("got %#v, want Rgb15toHsl16(0x1234)=%d", val, wantConverted)
	}
}

// recolExpected wraps colorconv.Rgb15toHsl16 so tests don't import the
// package at the top (they don't need it elsewhere). Use directly if the
// import is already present.
func recolExpected(rgb int) int {
	// import "github.com/zsrv/goscape/pkg/colorconv" at top of file
	// or inline the import in spotanim_test.go.
	return colorconv.Rgb15toHsl16(rgb)
}
```

> **Plan-author note:** the test file imports `github.com/zsrv/goscape/pkg/colorconv`. Add to imports.

- [ ] **Step 4.1.2: Implement parser in `pkg/pack/spotanim.go`**

Read `Engine-TS/tools/pack/config/SpotAnimConfig.ts:5-90` first. Port:

```go
package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
)

// spotanimNumberKeys mirrors TS SpotAnimConfig.ts:8-12 numberKeys[].
var spotanimNumberKeys = map[string]struct{}{
	"resizeh":  {},
	"resizev":  {},
	"angle":    {},
	"ambient":  {},
	"contrast": {},
}

// spotanimBooleanKeys mirrors TS SpotAnimConfig.ts:14-16 booleanKeys[].
var spotanimBooleanKeys = map[string]struct{}{
	"hasalpha": {},
}

// parseSpotAnimConfigFor returns the per-key=value parser for .spotanim
// config blocks. Closure-captures modelPack + seqPack.
//
// NAI-195-D-DEADBRANCH-OMITTED: empty TS stringKeys[] branch omitted.
//
// TS source: tools/pack/config/SpotAnimConfig.ts:5-90.
func parseSpotAnimConfigFor(modelPack, seqPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := spotanimNumberKeys[key]; ok {
			var n int64
			var err error
			if strings.HasPrefix(value, "0x") {
				n, err = strconv.ParseInt(value[2:], 16, 64)
			} else {
				n, err = strconv.ParseInt(value, 10, 64)
			}
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s", key, value)
			}
			// Per-key range checks: SpotAnimConfig.ts:47-57.
			switch key {
			case "resizeh", "resizev":
				if n < 0 || n > 512 {
					return nil, true, fmt.Errorf("%s out of range [0,512]: %d", key, n)
				}
			case "angle":
				if n < 0 || n > 360 {
					return nil, true, fmt.Errorf("%s out of range [0,360]: %d", key, n)
				}
			case "ambient", "contrast":
				if n < -128 || n > 127 {
					return nil, true, fmt.Errorf("%s out of range [-128,127]: %d", key, n)
				}
			}
			return int(n), true, nil
		}
		if _, ok := spotanimBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s", key, value)
			}
			return GetConfigBoolean(value), true, nil
		}
		if key == "model" {
			idx := modelPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown model: %s", value)
			}
			return idx, true, nil
		}
		if key == "anim" {
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown anim: %s", value)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "recol") && len(key) >= 6 {
			// TS SpotAnimConfig.ts:81-86: parseInt(key[5]) > 9 → reject.
			idxChar := key[5]
			if idxChar < '0' || idxChar > '9' {
				return nil, false, nil
			}
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid recol value: %s", value)
			}
			return colorconv.Rgb15toHsl16(int(n)), true, nil
		}
		return nil, false, nil
	}
}
```

- [ ] **Step 4.1.3: Run parser tests, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseSpotAnimConfig -count=1
```
Expected: all pass.

### Step 4.2: Implement packer

- [ ] **Step 4.2.1: Add packer tests to `pkg/pack/spotanim_test.go`**

```go
func spotanimOneSlotPack(name string) *PackFile {
	return newTestPF("spotanim", map[int]string{0: name})
}

func spotanimServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01}
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

func TestPackSpotAnimConfigs_Model(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	configs := map[string][]ConfigLine{
		"flame": {{Key: "model", Value: 7}},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x01, 0x00, 0x07, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_HasAlpha_TrueEmits(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	configs := map[string][]ConfigLine{
		"flame": {{Key: "hasalpha", Value: true}},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_HasAlpha_FalseNoEmit(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	configs := map[string][]ConfigLine{
		"flame": {{Key: "hasalpha", Value: false}},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_RecolSrcDst(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	// Values are already-converted hsl16 ints (parser stored them post-conversion).
	configs := map[string][]ConfigLine{
		"flame": {
			{Key: "recol1s", Value: 0x1234}, // → opcode 40 (40+0)
			{Key: "recol1d", Value: 0x5678}, // → opcode 50 (50+0)
		},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{
		0x00, 0x01,
		40, 0x12, 0x34,
		50, 0x56, 0x78,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_ServerDebugTrailer(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	server, _ := packSpotAnimConfigs(map[string][]ConfigLine{}, pf)
	if !bytes.Equal(server.Dat.Data, spotanimServerDebugTrailer("flame")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, spotanimServerDebugTrailer("flame"))
	}
}
```

- [ ] **Step 4.2.2: Run, observe red**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackSpotAnimConfigs -count=1
```
Expected: undefined.

- [ ] **Step 4.2.3: Implement packer**

Read `Engine-TS/tools/pack/config/SpotAnimConfig.ts:92-152` first. Port:

```go
// packSpotAnimConfigs emits the per-id body for .spotanim configs.
//
// Per TS, the recol opcode index is decoded from the key suffix as
// `parseInt(key[5:len-1]) - 1`; `*s` keys map to opcode 40+index,
// `*d` keys map to opcode 50+index. Note this is independent of the
// parser's single-char gate (`parseInt(key[5]) > 9`).
//
// Server-side: opcode 250 + pjstr(debugname) when debugname.length > 0.
//
// TS source: tools/pack/config/SpotAnimConfig.ts:92-152.
func packSpotAnimConfigs(configs map[string][]ConfigLine, spotanimPack *PackFile) (server, client *PackedData) {
	server = NewPackedData(spotanimPack.Max)
	client = NewPackedData(spotanimPack.Max)

	for id := range spotanimPack.Max {
		debugname := spotanimPack.GetByID(id)
		cfg := configs[debugname]

		for _, line := range cfg {
			key := line.Key
			switch {
			case key == "model":
				client.P1(1)
				client.P2(uint16(line.Value.(int)))
			case key == "anim":
				client.P1(2)
				client.P2(uint16(line.Value.(int)))
			case key == "hasalpha":
				if line.Value.(bool) {
					client.P1(3)
				}
			case key == "resizeh":
				client.P1(4)
				client.P2(uint16(line.Value.(int)))
			case key == "resizev":
				client.P1(5)
				client.P2(uint16(line.Value.(int)))
			case key == "angle":
				client.P1(6)
				client.P2(uint16(line.Value.(int)))
			case key == "ambient":
				client.P1(7)
				client.P1(uint8(line.Value.(int)))
			case key == "contrast":
				client.P1(8)
				client.P1(uint8(line.Value.(int)))
			case strings.HasPrefix(key, "recol") && len(key) >= 7:
				// TS SpotAnimConfig.ts:130: parseInt(key.substring(5, len-1)) - 1.
				idx, err := strconv.Atoi(key[5 : len(key)-1])
				if err != nil {
					continue
				}
				idx--
				suffix := key[len(key)-1]
				switch suffix {
				case 's':
					client.P1(uint8(40 + idx))
					client.P2(uint16(line.Value.(int)))
				case 'd':
					client.P1(uint8(50 + idx))
					client.P2(uint16(line.Value.(int)))
				}
			}
		}

		if len(debugname) > 0 {
			server.P1(250)
			server.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}
	return server, client
}
```

- [ ] **Step 4.2.4: Run, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestParseSpotAnimConfig|TestPackSpotAnimConfigs" -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```
Expected: all pass.

- [ ] **Step 4.3: Commit**

```bash
git add pkg/pack/spotanim.go pkg/pack/spotanim_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-197 T4 — .spotanim parser + packer + byte-pin tests

Ports tools/pack/config/SpotAnimConfig.ts. parseSpotAnimConfigFor
closure-binds modelPack + seqPack; recol{N}{s,d} values pass through
colorconv.Rgb15toHsl16 at the parser (matches TS line 86). Packer emits
client opcodes 1..8 and recol opcodes (40+index)/(50+index) for s/d;
server side emits the 250-trailer when debugname is non-empty.

NAI-195-D-DEADBRANCH-OMITTED: empty TS stringKeys[] branch omitted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `.idk` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/idk.go`
- Create: `pkg/pack/idk_test.go`

**TS source (READ END-TO-END):** `Engine-TS/tools/pack/config/IdkConfig.ts:1-206`. Parser at `:5-124`; packer at `:126-205`.

**Critical details:**

- TS has BOTH `stringKeys: []` AND `numberKeys: []` empty (`IdkConfig.ts:6-8`). Both omitted; both `NAI-195-D-DEADBRANCH-OMITTED`. Tag the omissions.
- `type` key (`IdkConfig.ts:50-99`) maps 14 bodypart names (`man_hair`, `man_jaw`, `man_torso`, `man_arms`, `man_hands`, `man_legs`, `man_feet`, `woman_hair`, `woman_jaw`, `woman_torso`, `woman_arms`, `woman_hands`, `woman_legs`, `woman_feet`) to integers 0..13. Unknown names reject.
- `model{N}` AND `head{N}` BOTH resolve through `ModelPack.getByName` (`IdkConfig.ts:100-113`).
- `recol{N}{s,d}` uses parser-side `ColorConversion.rgb15toHsl16` per `IdkConfig.ts:114-122` (same as `.spotanim`).
- Packer per-id accumulators: `recol_s[]`, `recol_d[]`, `models[]`, `heads[]` (`IdkConfig.ts:136-139`). Emission order (`IdkConfig.ts:165-193`): recol_s opcodes (40+i) → recol_d opcodes (50+i) → heads opcodes (60+i) → models opcode 2 with count then per-entry p2. Loose-emit: `type` → opcode 1 + p1(bodypart), `disable=true` → opcode 3 (no payload).

### Step 5.1: Implement parser

- [ ] **Step 5.1.1: Write parser tests in `pkg/pack/idk_test.go`**

```go
package pack

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/colorconv"
)

func idkTestRegistries(t *testing.T) (modelPack *PackFile) {
	t.Helper()
	modelPack = newTestPF("model", map[int]string{
		0: "m_zero",
		1: "m_one",
	})
	return
}

func TestParseIdkConfig_Type(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)

	cases := map[string]int{
		"man_hair":    0,
		"man_jaw":     1,
		"woman_torso": 9,
		"woman_feet":  13,
	}
	for name, want := range cases {
		val, accepted, err := parse("type", name)
		if err != nil {
			t.Errorf("type=%s: %v", name, err)
			continue
		}
		if !accepted {
			t.Errorf("type=%s should be accepted", name)
			continue
		}
		if val.(int) != want {
			t.Errorf("type=%s: got %#v, want int(%d)", name, val, want)
		}
	}
}

func TestParseIdkConfig_TypeUnknown(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)

	if _, _, err := parse("type", "no_such_part"); err == nil {
		t.Fatal("unknown type should reject")
	}
}

func TestParseIdkConfig_Model(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)

	val, accepted, err := parse("model1", "m_one")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("model1 should be accepted")
	}
	if val.(int) != 1 {
		t.Fatalf("got %#v, want int(1)", val)
	}
}

func TestParseIdkConfig_Head(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)

	val, accepted, err := parse("head1", "m_zero")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("head1 should be accepted")
	}
	if val.(int) != 0 {
		t.Fatalf("got %#v, want int(0)", val)
	}
}

func TestParseIdkConfig_RecolColorConvertedAtParser(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)

	val, accepted, err := parse("recol1s", "0x1234")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("recol1s should be accepted")
	}
	want := colorconv.Rgb15toHsl16(0x1234)
	if val.(int) != want {
		t.Fatalf("got %#v, want Rgb15toHsl16(0x1234)=%d", val, want)
	}
}

func TestParseIdkConfig_Disable(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)

	val, accepted, err := parse("disable", "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("disable should be accepted")
	}
	if val.(bool) != true {
		t.Fatalf("got %#v, want true", val)
	}
}
```

- [ ] **Step 5.1.2: Implement parser in `pkg/pack/idk.go`**

Read `Engine-TS/tools/pack/config/IdkConfig.ts:5-124` first. Port:

```go
package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
)

// idkBooleanKeys mirrors TS IdkConfig.ts:10-12 booleanKeys[].
var idkBooleanKeys = map[string]struct{}{
	"disable": {},
}

// idkTypeBodypart mirrors TS IdkConfig.ts:50-97 switch statement.
var idkTypeBodypart = map[string]int{
	"man_hair":    0,
	"man_jaw":     1,
	"man_torso":   2,
	"man_arms":    3,
	"man_hands":   4,
	"man_legs":    5,
	"man_feet":    6,
	"woman_hair":  7,
	"woman_jaw":   8,
	"woman_torso": 9,
	"woman_arms":  10,
	"woman_hands": 11,
	"woman_legs":  12,
	"woman_feet":  13,
}

// parseIdkConfigFor returns the per-key=value parser for .idk config
// blocks. Closure-captures modelPack (for model{N}/head{N}).
//
// NAI-195-D-DEADBRANCH-OMITTED: TS IdkConfig.ts:6-8 declares BOTH empty
// stringKeys[] AND empty numberKeys[] — both omitted here.
//
// TS source: tools/pack/config/IdkConfig.ts:5-124.
func parseIdkConfigFor(modelPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := idkBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s", key, value)
			}
			return GetConfigBoolean(value), true, nil
		}
		if key == "type" {
			bp, ok := idkTypeBodypart[value]
			if !ok {
				return nil, true, fmt.Errorf("unknown idk type: %s", value)
			}
			return bp, true, nil
		}
		if strings.HasPrefix(key, "model") {
			idx := modelPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown model: %s", value)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "head") {
			idx := modelPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown model: %s", value)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "recol") && len(key) >= 6 {
			idxChar := key[5]
			if idxChar < '0' || idxChar > '9' {
				return nil, false, nil
			}
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid recol value: %s", value)
			}
			return colorconv.Rgb15toHsl16(int(n)), true, nil
		}
		return nil, false, nil
	}
}
```

- [ ] **Step 5.1.3: Run parser tests, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseIdkConfig -count=1
```

### Step 5.2: Implement packer

- [ ] **Step 5.2.1: Add packer tests to `pkg/pack/idk_test.go`**

```go
func idkOneSlotPack(name string) *PackFile {
	return newTestPF("idk", map[int]string{0: name})
}

func idkServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01}
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

func TestPackIdkConfigs_Type(t *testing.T) {
	pf := idkOneSlotPack("man_hair_0")
	configs := map[string][]ConfigLine{
		"man_hair_0": {{Key: "type", Value: 0}},
	}
	_, client := packIdkConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x01, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_DisableTrueEmits(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {{Key: "disable", Value: true}},
	}
	_, client := packIdkConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_Models(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "model1", Value: 7},
			{Key: "model2", Value: 9},
		},
	}
	_, client := packIdkConfigs(configs, pf)
	// opcode 2 + p1(2) + p2(7) + p2(9) + Next 0x00
	want := []byte{0x00, 0x01, 0x02, 0x02, 0x00, 0x07, 0x00, 0x09, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_Heads(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "head1", Value: 4},
			{Key: "head2", Value: 5},
		},
	}
	_, client := packIdkConfigs(configs, pf)
	// opcodes 60 + p2(4), 61 + p2(5) + Next 0x00
	want := []byte{0x00, 0x01, 60, 0x00, 0x04, 61, 0x00, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_RecolSrcDst(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "recol1s", Value: 0x1234},
			{Key: "recol1d", Value: 0x5678},
		},
	}
	_, client := packIdkConfigs(configs, pf)
	// recol_s emitted before recol_d (TS IdkConfig.ts:165-178).
	want := []byte{0x00, 0x01, 40, 0x12, 0x34, 50, 0x56, 0x78, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_ServerDebugTrailer(t *testing.T) {
	pf := idkOneSlotPack("man_hair_0")
	server, _ := packIdkConfigs(map[string][]ConfigLine{}, pf)
	if !bytes.Equal(server.Dat.Data, idkServerDebugTrailer("man_hair_0")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, idkServerDebugTrailer("man_hair_0"))
	}
}
```

- [ ] **Step 5.2.2: Implement packer**

Read `Engine-TS/tools/pack/config/IdkConfig.ts:126-205` first. Port:

```go
// packIdkConfigs emits the per-id body for .idk configs.
//
// Per-id accumulators (TS IdkConfig.ts:136-139): recol_s/recol_d/models/heads.
// In-loop loose-emit: `type` → opcode 1 + p1(bodypart); `disable=true` → opcode 3.
// End-of-id emission order (TS IdkConfig.ts:165-193):
//   recol_s[i] → opcode 40+i + p2
//   recol_d[i] → opcode 50+i + p2
//   heads[i]   → opcode 60+i + p2
//   models     → opcode 2 + p1(len) + per-entry p2
// Server side: opcode 250 + pjstr(debugname) when debugname.length > 0.
//
// TS source: tools/pack/config/IdkConfig.ts:126-205.
func packIdkConfigs(configs map[string][]ConfigLine, idkPack *PackFile) (server, client *PackedData) {
	server = NewPackedData(idkPack.Max)
	client = NewPackedData(idkPack.Max)

	for id := range idkPack.Max {
		debugname := idkPack.GetByID(id)
		cfg := configs[debugname]

		var recolS, recolD, models, heads []int

		for _, line := range cfg {
			key := line.Key
			switch {
			case strings.HasPrefix(key, "model"):
				idx, err := strconv.Atoi(key[len("model"):])
				if err != nil {
					continue
				}
				idx--
				for len(models) <= idx {
					models = append(models, 0)
				}
				models[idx] = line.Value.(int)
			case strings.HasPrefix(key, "head"):
				idx, err := strconv.Atoi(key[len("head"):])
				if err != nil {
					continue
				}
				idx--
				for len(heads) <= idx {
					heads = append(heads, 0)
				}
				heads[idx] = line.Value.(int)
			case strings.HasPrefix(key, "recol") && strings.HasSuffix(key, "s"):
				idx, err := strconv.Atoi(key[5 : len(key)-1])
				if err != nil {
					continue
				}
				idx--
				for len(recolS) <= idx {
					recolS = append(recolS, 0)
				}
				recolS[idx] = line.Value.(int)
			case strings.HasPrefix(key, "recol") && strings.HasSuffix(key, "d"):
				idx, err := strconv.Atoi(key[5 : len(key)-1])
				if err != nil {
					continue
				}
				idx--
				for len(recolD) <= idx {
					recolD = append(recolD, 0)
				}
				recolD[idx] = line.Value.(int)
			case key == "type":
				client.P1(1)
				client.P1(uint8(line.Value.(int)))
			case key == "disable":
				if line.Value.(bool) {
					client.P1(3)
				}
			}
		}

		for i, v := range recolS {
			client.P1(uint8(40 + i))
			client.P2(uint16(v))
		}
		for i, v := range recolD {
			client.P1(uint8(50 + i))
			client.P2(uint16(v))
		}
		for i, v := range heads {
			client.P1(uint8(60 + i))
			client.P2(uint16(v))
		}
		if len(models) > 0 {
			client.P1(2)
			client.P1(uint8(len(models)))
			for _, v := range models {
				client.P2(uint16(v))
			}
		}

		if len(debugname) > 0 {
			server.P1(250)
			server.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}
	return server, client
}
```

- [ ] **Step 5.2.3: Run, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run "TestParseIdkConfig|TestPackIdkConfigs" -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```

- [ ] **Step 5.3: Commit**

```bash
git add pkg/pack/idk.go pkg/pack/idk_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-197 T5 — .idk parser + packer + byte-pin tests

Ports tools/pack/config/IdkConfig.ts. parseIdkConfigFor closure-binds
modelPack; type key maps 14 bodypart names to integers 0..13; recol
values pass through colorconv.Rgb15toHsl16 at the parser. Packer
accumulates models/heads/recol_s/recol_d per id, emitting in TS order
(recol_s → recol_d → heads → models opcode 2 + count + entries).
Server side emits the 250-trailer when debugname is non-empty.

NAI-195-D-DEADBRANCH-OMITTED: TS declares BOTH empty stringKeys[] AND
empty numberKeys[]; both branches omitted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `PackConfigs` wiring + atomic integration-test rewrite

**Files:**
- Modify: `pkg/pack/pack_configs.go`
- Modify: `pkg/pack/pack_configs_test.go`

This task wires the four new branches into `PackConfigs` at TS-canonical positions, adds the four new `packAndSaveFoo` helpers, extends the `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment scope, and atomically rewrites `TestPackConfigs_ElevenConfigsLand` to `TestPackConfigs_FifteenConfigsLand` (per `[[file_scoped_audits_miss_cross_file_ts]]` — single source of truth, matches NAI-196's atomic rewrite of NAI-195's eight-config test).

### Step 6.1: Add four new `packAndSaveFoo` helper functions

- [ ] **Step 6.1.1: Append the four helpers to `pkg/pack/pack_configs.go`**

Below the existing `packAndSaveObj` function (ends around line 606), append:

```go
// packAndSaveSeq reads .seq sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness, matching
// TS PackShared.ts:460 (rebuildClient=true ungates shouldBuild).
//
// TS source: tools/pack/config/SeqConfig.ts:121-208.
func packAndSaveSeq(srcDir, serverOut string, seqPack, animPack, objPack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseSeqConfigFor(animPack, objPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".seq", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packSeqConfigs(cfgs, seqPack)
	if err := server.Save(
		filepath.Join(serverOut, "seq.dat"),
		filepath.Join(serverOut, "seq.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("seq.dat", client.Dat)
	clientJag.Write("seq.idx", client.Idx)
	return nil
}

// packAndSaveFlo reads .flo sources, packs them, writes server
// .dat/.idx (which contain only per-id Next() boundaries — no opcode
// bytes), and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/FloConfig.ts:63-104.
func packAndSaveFlo(srcDir, serverOut string, floPack, texturePack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseFloConfigFor(texturePack)
	cfgs, err := ReadTypedConfigs(srcDir, ".flo", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packFloConfigs(cfgs, floPack)
	if err := server.Save(
		filepath.Join(serverOut, "flo.dat"),
		filepath.Join(serverOut, "flo.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("flo.dat", client.Dat)
	clientJag.Write("flo.idx", client.Idx)
	return nil
}

// packAndSaveSpotAnim reads .spotanim sources, packs them, writes
// server .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/SpotAnimConfig.ts:92-152.
func packAndSaveSpotAnim(srcDir, serverOut string, spotanimPack, modelPack, seqPack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseSpotAnimConfigFor(modelPack, seqPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".spotanim", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packSpotAnimConfigs(cfgs, spotanimPack)
	if err := server.Save(
		filepath.Join(serverOut, "spotanim.dat"),
		filepath.Join(serverOut, "spotanim.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("spotanim.dat", client.Dat)
	clientJag.Write("spotanim.idx", client.Idx)
	return nil
}

// packAndSaveIdk reads .idk sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/IdkConfig.ts:126-205.
func packAndSaveIdk(srcDir, serverOut string, idkPack, modelPack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseIdkConfigFor(modelPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".idk", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packIdkConfigs(cfgs, idkPack)
	if err := server.Save(
		filepath.Join(serverOut, "idk.dat"),
		filepath.Join(serverOut, "idk.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("idk.dat", client.Dat)
	clientJag.Write("idk.idx", client.Idx)
	return nil
}
```

### Step 6.2: Wire branches into `PackConfigs` at TS-canonical positions

- [ ] **Step 6.2.1: Remove the four `_ = ensureFoo` suppression lines added in T1**

Locate the suppression block (around the new closures) and delete the four `_ = ensureAnimPack` / `_ = ensureFloPack` / `_ = ensureSpotAnimPack` / `_ = ensureIdkPack` lines together with their leading comment.

- [ ] **Step 6.2.2: Insert `.seq` branch between `.struct` and `.loc`**

Locate the `.struct` block (currently ending around `pack_configs.go:278`) and the `.loc` block (currently starting around `:280`). Between them, insert:

```go
	// .seq — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:454-475.
	if err := ensureSeqPack(); err != nil {
		return err
	}
	if err := ensureAnimPack(); err != nil {
		return err
	}
	if err := ensureObjPack(); err != nil {
		return err
	}
	if err := packAndSaveSeq(srcDir, serverOut, seqPack, animPack, objPack, constants, clientJag); err != nil {
		return err
	}
```

- [ ] **Step 6.2.3: Insert `.flo` branch between `.loc` and (existing position of `.npc`)**

After the existing `.loc` block (ends with `packAndSaveLoc` and `return err` block; around `:298`) and BEFORE the existing `.npc` block (around `:300`), insert:

```go
	// .flo — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:500-521.
	if err := ensureFloPack(); err != nil {
		return err
	}
	if err := ensureTexturePack(); err != nil {
		return err
	}
	if err := packAndSaveFlo(srcDir, serverOut, floPack, texturePack, constants, clientJag); err != nil {
		return err
	}

	// .spotanim — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:523-544.
	if err := ensureSpotAnimPack(); err != nil {
		return err
	}
	if err := ensureModelPack(); err != nil {
		return err
	}
	if err := ensureSeqPack(); err != nil {
		return err
	}
	if err := packAndSaveSpotAnim(srcDir, serverOut, spotanimPack, modelPack, seqPack, constants, clientJag); err != nil {
		return err
	}
```

> **Plan-author note:** the spec's TS-canonical order places `.spotanim` immediately after `.flo` and before `.npc`. Spec §6 codifies this; verified at plan-write against TS `PackShared.ts:500-547`.

- [ ] **Step 6.2.4: Insert `.idk` branch between `.obj` and `.varp`**

After the existing `.obj` block (ends around `:335`) and BEFORE the existing `.varp` block (around `:337`), insert:

```go
	// .idk — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:592-613.
	if err := ensureIdkPack(); err != nil {
		return err
	}
	if err := ensureModelPack(); err != nil {
		return err
	}
	if err := packAndSaveIdk(srcDir, serverOut, idkPack, modelPack, constants, clientJag); err != nil {
		return err
	}
```

### Step 6.3: Extend the `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment scope

- [ ] **Step 6.3.1: Update the in-place doc-comment**

In `pack_configs.go`, locate the doc-comment block at lines 37-44 (the `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` paragraph on `PackConfigs`). Update it from the 5-branch enumeration to a 9-branch enumeration. Use per-instance `Edit` (per `[[plan_doc_replaceall_timeline]]` — do not use `replace_all`):

Find this exact paragraph:

```
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: .param, .loc, .npc, .obj, .varp
// run on EVERY PackConfigs invocation regardless of source freshness,
// matching TS PackShared.ts:337 (`const rebuildClient = true`) which
// ungates shouldBuild on the four configs that write to client jag
// (loc/npc/obj/varp) and — per NAI-196 §"R5 resolution" — also on
// .param so that all client-jagfile entries are always present.
// The server-only six (.enum, .inv, .mesanim, .struct, .varn, .vars)
// retain their ShouldBuild + GetLatestModified freshness gates.
```

Replace with:

```
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: .param, .seq, .loc, .flo,
// .spotanim, .npc, .obj, .idk, .varp run on EVERY PackConfigs
// invocation regardless of source freshness, matching TS
// PackShared.ts:337 (`const rebuildClient = true`) which ungates
// shouldBuild on the eight configs that write to client jag and —
// per NAI-196 §"R5 resolution" — also on .param so that all
// client-jagfile entries are always present. NAI-197 extends the
// scope to the four additional client+server configs ported in that
// slice (.seq, .flo, .spotanim, .idk). The server-only six (.enum,
// .inv, .mesanim, .struct, .varn, .vars) retain their ShouldBuild +
// GetLatestModified freshness gates.
```

### Step 6.4: Build, then atomically rewrite the integration test

- [ ] **Step 6.4.1: Verify production-side build is green**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: clean.

- [ ] **Step 6.4.2: Rewrite `TestPackConfigs_ElevenConfigsLand` → `TestPackConfigs_FifteenConfigsLand`**

In `pkg/pack/pack_configs_test.go`, locate `TestPackConfigs_ElevenConfigsLand` (around line 410). Replace the entire function with:

```go
func TestPackConfigs_FifteenConfigsLand(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pack files for all referenced typed-ids (20 in total: 15 configs
	// plus model/category/hunt/texture/anim/interface/synth/dbrow
	// support files; spotanim/idk are configs in this slice).
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=quest_points\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npc_state\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=server_clock\n")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")
	writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "0=days\n")
	writeFile(t, filepath.Join(srcDir, "pack", "inv.pack"), "0=bank\n")
	writeFile(t, filepath.Join(srcDir, "pack", "mesanim.pack"), "0=hero_chat\n")
	writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "0=goblin_loot\n")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
	writeFile(t, filepath.Join(srcDir, "pack", "npc.pack"), "0=rat\n")
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=egg\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=walk\n")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "0=red\n")
	writeFile(t, filepath.Join(srcDir, "pack", "spotanim.pack"), "0=flame\n")
	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "0=man_hair_default\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "")
	for _, p := range []string{"interface", "synth", "dbrow"} {
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
	writeFile(t, filepath.Join(scripts, "l.loc"),
		"[table]\nname=Table\n")
	writeFile(t, filepath.Join(scripts, "k.npc"),
		"[rat]\nname=Rat\n")
	writeFile(t, filepath.Join(scripts, "o.obj"),
		"[egg]\nname=Egg\n")
	writeFile(t, filepath.Join(scripts, "q.seq"),
		"[walk]\nloops=1\n")
	writeFile(t, filepath.Join(scripts, "f.flo"),
		"[red]\ncolour=0xFF0000\n")
	writeFile(t, filepath.Join(scripts, "a.spotanim"),
		"[flame]\nangle=180\n")
	writeFile(t, filepath.Join(scripts, "d.idk"),
		"[man_hair_default]\ntype=man_hair\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	// All 15 server-side .dat/.idx pairs landed.
	server := filepath.Join(outDir, "server")
	for _, typ := range []string{
		"varp", "varn", "vars", "param", "enum", "inv", "mesanim", "struct",
		"loc", "npc", "obj", "seq", "flo", "spotanim", "idk",
	} {
		if _, err := os.Stat(filepath.Join(server, typ+".dat")); err != nil {
			t.Errorf("%s.dat missing: %v", typ, err)
		}
		if _, err := os.Stat(filepath.Join(server, typ+".idx")); err != nil {
			t.Errorf("%s.idx missing: %v", typ, err)
		}
	}

	// Client jagfile contains all 18 client-side entries
	// (9 client+server configs × .dat/.idx pair = 18):
	//   param, seq, loc, flo, spotanim, npc, obj, idk, varp.
	jagPath := filepath.Join(outDir, "client", "config")
	if _, err := os.Stat(jagPath); err != nil {
		t.Fatalf("client/config jagfile missing: %v", err)
	}
	jag, err := jagfile.LoadJagfile(jagPath)
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	expected := []string{
		"param.dat", "param.idx",
		"seq.dat", "seq.idx",
		"loc.dat", "loc.idx",
		"flo.dat", "flo.idx",
		"spotanim.dat", "spotanim.idx",
		"npc.dat", "npc.idx",
		"obj.dat", "obj.idx",
		"idk.dat", "idk.idx",
		"varp.dat", "varp.idx",
	}
	for _, name := range expected {
		if _, err := jag.Read(name); err != nil {
			t.Errorf("client jagfile missing entry %q: %v", name, err)
		}
	}
	if jag.FileCount != len(expected) {
		t.Errorf("client jagfile has %d entries, want %d (names=%v)", jag.FileCount, len(expected), jag.FileName)
	}
	// Entry order must match Write insertion order, which mirrors the
	// TS-canonical PackConfigs branch order extended in NAI-197:
	//   param → seq → loc → flo → spotanim → npc → obj → idk → varp.
	wantOrder := []string{
		"param.dat", "param.idx",
		"seq.dat", "seq.idx",
		"loc.dat", "loc.idx",
		"flo.dat", "flo.idx",
		"spotanim.dat", "spotanim.idx",
		"npc.dat", "npc.idx",
		"obj.dat", "obj.idx",
		"idk.dat", "idk.idx",
		"varp.dat", "varp.idx",
	}
	if !slices.Equal(jag.FileName, wantOrder) {
		t.Errorf("client jag entry order: got %v, want %v", jag.FileName, wantOrder)
	}
}
```

> **Plan-author note (per `[[plan_runnable_test_fixtures]]`):** mentally execute the fixture before dispatching. `man_hair_default` is the debugname; the `.idk` source body uses `type=man_hair` which maps to bodypart 0.

- [ ] **Step 6.4.3: Run integration test, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackConfigs_FifteenConfigsLand -count=1 -v
```
Expected: pass.

- [ ] **Step 6.4.4: Run full pkg/pack suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```
Expected: all green project-wide.

- [ ] **Step 6.5: Commit**

```bash
git add pkg/pack/pack_configs.go pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-197 T6 — wire .seq/.flo/.spotanim/.idk into PackConfigs

Inserts four new unconditional client+server branches in TS-canonical
positions (.seq between .struct and .loc; .flo between .loc and
.spotanim; .spotanim between .flo and .npc; .idk between .obj and
.varp). Adds four packAndSaveFoo orchestrators following the NAI-196
shape.

Extends the NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment to
enumerate all 9 client+server branches (was 5).

Atomically rewrites TestPackConfigs_ElevenConfigsLand to
TestPackConfigs_FifteenConfigsLand: 15 server-side .dat/.idx pairs,
18 client jagfile entries, full TS-canonical ordering assertion.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Round-trip tests for `.seq`, `.spotanim`, `.idk`

**Files:**
- Create: `pkg/pack/seq_roundtrip_test.go`
- Create: `pkg/pack/spotanim_roundtrip_test.go`
- Create: `pkg/pack/idk_roundtrip_test.go`

These tests exercise the full `PackConfigs` → `objtype.LoadXxxTypes` path. They use `setupPackRoots` for the shared boilerplate and add the new `anim.pack` / `flo.pack` / `idk.pack` stubs per-test (since `setupPackRoots` predates this slice and stubs only `spotanim.pack` from the four new configs).

> **No `.flo` round-trip** — `pkg/objtype.LoadFloTypes` does not exist (verified at plan-write per spec §9 R3). Byte-pin coverage in T3 is the contract.

### Step 7.1: `.seq` round-trip

- [ ] **Step 7.1.1: Create `pkg/pack/seq_roundtrip_test.go`**

```go
package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackSeqRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	// .seq registry plus its dependencies (anim, obj already stubbed by
	// setupPackRoots; anim.pack is new in NAI-197).
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=walk\n")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "0=walk_frame_1\n")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "")
	// loc/model/category/texture stubs — T6's branches require them.
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.seq"),
		"[walk]\nloops=2\npriority=5\nframe1=walk_frame_1\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	frames := &objtype.SeqFrameConfigs{}
	seqs, err := objtype.LoadSeqTypes(outDir, frames)
	if err != nil {
		t.Fatal(err)
	}
	seq := seqs.Configs[0]
	// Pin a handful of fields that the byte-pin tests in T2 exercised.
	// Field names below mirror objtype.SeqType — verify shape against
	// pkg/objtype/seqtype.go at implementation time. If the field name
	// for "loops" differs, follow the actual struct exactly.
	_ = seq // placeholder — implementer reads pkg/objtype/seqtype.go
	// Replace this block with concrete field assertions, e.g.:
	//   if seq.ReplayCount != 2 { t.Errorf("ReplayCount: got %d, want 2", seq.ReplayCount) }
	//   if seq.Priority != 5 { t.Errorf("Priority: got %d, want 5", seq.Priority) }
}
```

> **Plan-author directive:** the field-name comments above are intentional. Implementer reads `pkg/objtype/seqtype.go` at task time to map `loops` / `priority` / `frame1` to the actual exported field names on `*SeqType`. Replace the `_ = seq` placeholder with concrete assertions before committing.

- [ ] **Step 7.1.2: Run, fix field names, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackSeqRoundTrip -count=1 -v
```
Expected: pass after concrete assertions replace the placeholder.

### Step 7.2: `.spotanim` round-trip

- [ ] **Step 7.2.1: Create `pkg/pack/spotanim_roundtrip_test.go`**

```go
package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackSpotAnimRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	// spotanim's registry deps: model, seq.
	writeFile(t, filepath.Join(srcDir, "pack", "spotanim.pack"), "0=flame\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=flame_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=flame_anim\n")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.spotanim"),
		"[flame]\nmodel=flame_model\nanim=flame_anim\nangle=180\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	spots, err := objtype.LoadSpotanimTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	spot := spots.Configs[0]
	_ = spot
	// Concrete assertions — implementer maps to actual SpotanimType field names.
	// Example shape (verify pkg/objtype/spotanimtype.go):
	//   if spot.Model != 0 { t.Errorf("Model: got %d, want 0", spot.Model) }
	//   if spot.SeqID != 0 { t.Errorf("SeqID: got %d, want 0", spot.SeqID) }
}
```

- [ ] **Step 7.2.2: Run, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackSpotAnimRoundTrip -count=1 -v
```

### Step 7.3: `.idk` round-trip

- [ ] **Step 7.3.1: Create `pkg/pack/idk_roundtrip_test.go`**

```go
package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackIdkRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "0=man_hair_default\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=hair_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.idk"),
		"[man_hair_default]\ntype=man_hair\nmodel1=hair_model\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	idks, err := objtype.LoadIdkTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	idk := idks.Configs[0]
	_ = idk
	// Concrete assertions — implementer maps to actual IdkType field names.
	// Example shape (verify pkg/objtype/idktype.go):
	//   if idk.Bodypart != 0 { t.Errorf("Bodypart: got %d, want 0", idk.Bodypart) }
	//   if idk.Models[0] != 0 { t.Errorf("Models[0]: got %d, want 0", idk.Models[0]) }
}
```

- [ ] **Step 7.3.2: Run, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackIdkRoundTrip -count=1 -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```

- [ ] **Step 7.4: Commit**

```bash
git add pkg/pack/seq_roundtrip_test.go pkg/pack/spotanim_roundtrip_test.go pkg/pack/idk_roundtrip_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-197 T7 — round-trip tests for .seq/.spotanim/.idk

Each test writes minimal sources, runs PackConfigs, then loads the
emitted server/<type>.dat via the matching objtype.LoadXxxTypes.

No .flo round-trip — pkg/objtype.LoadFloTypes does not exist; T3's
byte-pin coverage (incl. empty-server-bytes and opcode-6 dual-asymmetry)
is the contract per spec §8.2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Deviation-tag pins

**Files:**
- Create: `pkg/pack/nai197_deviation_pins_test.go`

This task adds ONE presence pin (no retirements this slice). The pin re-asserts that the `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment now enumerates all 9 client+server configs (5 original + 4 new). It guards against doc-comment scope rot if a future refactor edits the comment.

> Per `[[pin_test_self_trigger_production_doc]]`: the pin uses existing goscape concept names (`unconditional client pack`, config-type extensions like `.seq`) rather than TS-side identifiers — no self-trigger risk.

- [ ] **Step 8.1: Create `pkg/pack/nai197_deviation_pins_test.go`**

```go
package pack

import (
	"strings"
	"testing"
)

// TestNAI197_PresencePin_UnconditionalClientPackExtended re-asserts that
// the NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment lives in pkg/pack
// production code AND that its scope enumeration now includes the four
// configs ported in NAI-197.
//
// Guards against:
//   (a) accidental retirement of the tag identifier (the tag still applies);
//   (b) doc-comment refactors that drop one or more of the four NAI-197
//       configs from the scope list.
func TestNAI197_PresencePin_UnconditionalClientPackExtended(t *testing.T) {
	src := scanPkgPack(t)
	if !strings.Contains(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK") {
		t.Fatal("NAI-196-D-UNCONDITIONAL-CLIENT-PACK tag should be documented in pkg/pack production code but is absent")
	}
	// Locate the doc-comment block containing the tag identifier. We assert
	// the four NAI-197 config names appear within the block.
	idx := strings.Index(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK")
	if idx == -1 {
		t.Fatal("tag identifier not found")
	}
	// Look at a generous window after the tag for the scope enumeration.
	window := src[idx:]
	end := strings.Index(window, "\n//\n") // paragraph boundary in Go doc-comments
	if end == -1 || end > 2000 {
		end = 2000
	}
	if end > len(window) {
		end = len(window)
	}
	block := window[:end]

	for _, cfg := range []string{".seq", ".flo", ".spotanim", ".idk"} {
		if !strings.Contains(block, cfg) {
			t.Errorf("NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment scope is missing config %q (got block:\n%s)", cfg, block)
		}
	}
}
```

- [ ] **Step 8.2: Run pin tests, observe green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestNAI197_ -count=1 -v
```
Expected: pass.

- [ ] **Step 8.3: Final full-suite check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```
Expected: all green.

- [ ] **Step 8.4: Commit**

```bash
git add pkg/pack/nai197_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-197 T8 — deviation-tag pins

Adds one presence pin asserting the NAI-196-D-UNCONDITIONAL-CLIENT-PACK
doc-comment in pkg/pack/pack_configs.go enumerates all 9 client+server
configs after NAI-197's scope extension (the 5 NAI-196-era plus the
4 NAI-197 additions: .seq, .flo, .spotanim, .idk).

No retirements this slice.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Plan-author self-review notes

**Spec coverage:**
- ✅ Spec §2 in-scope: parsers+packers (T2–T5), four `ensureFoo` helpers (T1), `PackConfigs` re-ordering (T6), round-trips for 3 of 4 (T7), 15-config integration (T6), deviation pins (T8).
- ✅ Spec §4 file inventory: matches plan File Inventory.
- ✅ Spec §5 per-config design: T2–T5 each cite TS file:line and follow `[[true_to_ts_gate]]` (no opcode tables codified).
- ✅ Spec §6 pipeline integration: T6 wires four branches at TS-canonical positions; doc-comment scope extension.
- ✅ Spec §7 deviations: T8 presence pin; no new tags; carryforwards (`NAI-195-D-DEADBRANCH-OMITTED`) documented in per-config files.
- ✅ Spec §8 tests: byte-pin tests in T2–T5, round-trips in T7, integration in T6, pins in T8.
- ✅ Spec §9 risk register ⚠️ rows: all pre-flighted against HEAD at plan-write (see Pre-flight resolution table).
- ✅ Spec §10 out-of-scope follow-ups: not in plan (correctly).
- ✅ Spec §12 task-count estimate (8 tasks, 9–13 commits): plan honors.

**Type consistency:**
- ✅ All four new packer functions return `(server, client *PackedData)` — no `err` (plan-author narrowing of spec §4; documented).
- ✅ All four `ensureFoo` closure signatures match existing pattern (`func() error`).
- ✅ `parseFooConfigFor` consistent across configs: `(deps...) ParseFn`.
- ✅ `packAndSaveFoo` consistent: `(srcDir, serverOut string, <packs...>, c Constants, clientJag *jagfile.Jagfile) error`.
- ✅ Test helper naming consistent (`fooOneSlotPack`, `fooServerDebugTrailer`, `fooTestRegistries`).

**Placeholder scan:**
- T7 contains intentional `_ = seq` / `_ = spot` / `_ = idk` placeholders with explicit plan-author directives instructing the implementer to replace with concrete field assertions after reading the matching `pkg/objtype/*.go`. This is the LoadParamTypes-style assertion pattern from NAI-196 T6 (`loc_roundtrip_test.go:46-57`). Implementer must replace, not leave as-is.
- No other placeholders / TBDs.

**Pre-flight discharge:**
- ✅ All ⚠️ rows in spec §9 resolved at plan-write: R1 (anim.pack convention), R2/R11 (.flo empty server), R6 (ColorConversion at parser), R7 (.idk dead branches), R9 (test fixture pack list).
- ✅ No geometry/math premises (`[[plan_geometry_premise_pretrace]]` n/a).
