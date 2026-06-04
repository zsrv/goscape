# NAI-193 — .varp packer slice — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `tools/pack/config/VarpConfig.ts` onto the NAI-192 PackShared infrastructure. First dual-output packer (server `.dat`/`.idx` + client jagfile entry under `<outDir>/client/config`). Retires `NAI-192-D-VARP-UNIQUENESS-DEFERRED` via the cross-domain `{VarpPack, VarnPack, VarsPack}` name-uniqueness check at the top of `PackConfigs`.

**Architecture:** New code in `pkg/pack/varp.go` (parse + pack) + extension to `pkg/pack/pack_configs.go` (uniqueness check, fresh jagfile, varp branch). One mechanical infrastructure fix in `pkg/io/jagfile/jagfile.go` (`Save` panics on fresh-empty Jagfile — TS-faithful auto-grow fix). No production callsite this slice — wired only from test code.

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` + `pkg/io/jagfile` + NAI-191 `pkg/pack` foundation + NAI-192 PackShared infrastructure.

**Spec:** `docs/superpowers/specs/2026-05-13-nai-193-varp-packer-design.md` (commit `acc9133`).
**HEAD at plan-write:** `acc9133`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/pack/varn_test.go`: bare `if err != nil { t.Fatal(err) }`, `bytes.Equal` for byte-level comparison, `t.Fatalf("got % x, want % x", got, want)` for byte-diff envelopes, `t.TempDir()` for fixture roots, `ClearFsCache()` before any test that mutates the filesystem, `writeFile(t, path, content)` helper already in `pkg/pack/constants_test.go`.
- **Error envelope** matches existing goscape `pkg/pack/parse.go` style: `fmt.Errorf("<kind> in %s: %s", file, detail)`. The TS `Error during parsing - see ${file}:${n+1}` envelope is already documented as `NAI-192-D-PARSE-ERROR-ENVELOPE`.
- **Modern Go**: use range over integer (`for id := range pf.Max`) per `use-modern-go` skill.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `acc9133`:

| Premise | Verification |
|---|---|
| `pkg/io/jagfile/jagfile.go:122-167` indexes `jf.FileHash[index]=...` without growing slices | ✅ Read |
| `pkg/io/jagfile/jagfile.go:268` (`NewJagfile(nil)`) returns `&Jagfile{}` (empty slices) | ✅ Read |
| `pkg/io/jagfile.Jagfile.Write(name, *packet.Packet)` exists at line 85; queues into `FileQueue` | ✅ Read |
| `pkg/io/jagfile.LoadJagfile(path) (*Jagfile, error)` exists at line 321 | ✅ Read |
| `pkg/io/jagfile.Jagfile.Read(name) (*packet.Packet, error)` exists at line 73 | ✅ Read |
| `pkg/io/jagfile.Jagfile.Save(path, doNotCompressWhole bool) error` exists at line 119 | ✅ Read |
| `pkg/objtype.VarpScopePerm = 1`, `VarpScopeTemp = 0` exported at `varptype.go:11-13` | ✅ Read |
| `pkg/objtype.ScriptVarTypeFromName(name string) (ScriptVarType, bool)` exported at `scriptvartype.go` (NAI-192 T1) | ✅ Grep |
| `pkg/objtype.LoadVarpTypes(dir)` reads `<dir>/server/varp.dat` + `<dir>/client/config` jagfile + `varp.dat` entry | ✅ Read varptype.go:60-118 |
| `pkg/pack.PackFile.Type` field exists (use for uniqueness-check error message — replaces hypothetical `SourcePath()` from spec §4.4) | ✅ Read packfile.go:31 |
| `pkg/pack.PackFile.SourcePath()` does NOT exist — drop spec §4.4's reference; use `pf.Type` | ✅ Grep |
| `pkg/pack.packAndSaveVarn(srcDir, serverOut, c)` and `packAndSaveVars(srcDir, serverOut, c)` are private (only `PackConfigs` calls them) | ✅ Grep `packAndSaveV` in pkg/ |
| `pkg/pack.NewPackedData(size).Dat.Data` contains the 2-byte count header `00 NN` after construction; `marker=2` | ✅ Read packed_data.go |
| `NAI-192-D-VARP-UNIQUENESS-DEFERRED` referenced in `pack_configs.go:13-17` (the comment block to delete) AND in `nai192_deviation_pins_test.go:TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator` | ✅ Read both |
| `NewVarPlayerType(id)` defaults: `Scope=VarpScopeTemp`, `Type=ScriptVarTypeInt`, `Protect=true`, `Transmit=false`, `ClientCode=0` | ✅ Read varptype.go:47-53 — binds the "absent opcode → default" asymmetry for protect/transmit byte-pin tests |
| `*Packet.Save(path, length, start)` writes `os.MkdirAll(filepath.Dir(path), 0o755)` then `os.WriteFile(path, p.Data[start:start+length], 0o644)` | ✅ Read packet.go:108-130 |

---

## Task 1: Jagfile auto-grow fix in `pkg/io/jagfile/jagfile.go`

**Why first:** Surface of the fix is isolated (one function), and every downstream task relies on Jagfile.Save not panicking on a fresh-empty Jagfile. Red-green is clean — the test panics pre-fix, passes post-fix.

**Files:**
- Modify: `pkg/io/jagfile/jagfile.go:122-167` (auto-grow per-field slices on demand inside Save's write branch)
- Modify: `pkg/io/jagfile/jagfile_test.go` (add `TestJagfile_FreshEmptyWriteSaveRoundTrip`)

### Steps

- [ ] **Step 1.1: Write the failing test**

Append to `pkg/io/jagfile/jagfile_test.go`:

```go
func TestJagfile_FreshEmptyWriteSaveRoundTrip(t *testing.T) {
	jf, err := NewJagfile(nil)
	if err != nil {
		t.Fatal(err)
	}

	a := packet.NewPacket([]byte{0xAA, 0xBB})
	b := packet.NewPacket([]byte{0xCC, 0xDD, 0xEE})
	jf.Write("a.dat", a)
	jf.Write("b.dat", b)

	path := filepath.Join(t.TempDir(), "config")
	if err := jf.Save(path, false); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadJagfile(path)
	if err != nil {
		t.Fatal(err)
	}
	gotA, err := reloaded.Read("a.dat")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotA.Data, []byte{0xAA, 0xBB}) {
		t.Fatalf("a.dat=% x, want AA BB", gotA.Data)
	}
	gotB, err := reloaded.Read("b.dat")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotB.Data, []byte{0xCC, 0xDD, 0xEE}) {
		t.Fatalf("b.dat=% x, want CC DD EE", gotB.Data)
	}
}
```

Confirm the test file imports include `bytes`, `filepath`, `testing`, and `github.com/zsrv/goscape/pkg/io/packet`. If any are absent, add them.

- [ ] **Step 1.2: Run the test to verify it panics (red phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/jagfile/ -run TestJagfile_FreshEmptyWriteSaveRoundTrip -v
```

Expected: **PANIC** with `index out of range [0] with length 0` (or similar slice-bounds panic) at the line where `jf.FileHash[index] = queued.Hash` executes. If the test passes pre-fix, something else is going on — STOP and re-read `pkg/io/jagfile/jagfile.go:122-143`.

- [ ] **Step 1.3: Apply the auto-grow fix**

In `pkg/io/jagfile/jagfile.go`, locate the `Save` method's write branch (currently around line 126):

```go
		if queued.Write {
			if index == -1 {
				index = jf.FileCount
				jf.FileCount++

				jf.FileHash[index] = queued.Hash
				jf.FileName[index] = queued.Name
			}
```

Replace with:

```go
		if queued.Write {
			if index == -1 {
				index = jf.FileCount
				jf.FileCount++

				// Grow per-field slices on demand. TS arrays auto-grow
				// on indexed assignment; goscape slices need explicit
				// append. Constructing a fresh Jagfile via
				// NewJagfile(nil) leaves all per-field slices nil/zero;
				// without this growth the first write panics.
				if len(jf.FileHash) < jf.FileCount {
					jf.FileHash = append(jf.FileHash, 0)
					jf.FileName = append(jf.FileName, "")
					jf.FileUnpackedSize = append(jf.FileUnpackedSize, 0)
					jf.FilePackedSize = append(jf.FilePackedSize, 0)
					jf.FilePos = append(jf.FilePos, 0)
					jf.FileWrite = append(jf.FileWrite, nil)
				}

				jf.FileHash[index] = queued.Hash
				jf.FileName[index] = queued.Name
			}
```

- [ ] **Step 1.4: Run the test to verify it passes (green phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/jagfile/ -run TestJagfile_FreshEmptyWriteSaveRoundTrip -v
```

Expected: **PASS**.

- [ ] **Step 1.5: Run the full jagfile test suite to confirm no regression**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/jagfile/ -count=1 -race
```

Expected: **PASS** (all existing tests still green).

- [ ] **Step 1.6: Commit**

```bash
git add pkg/io/jagfile/jagfile.go pkg/io/jagfile/jagfile_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(jagfile): NAI-193 T1 — auto-grow Save write branch on fresh-empty

Constructing a fresh Jagfile via NewJagfile(nil) and calling
Write+Save previously panicked at jf.FileHash[index]=... because
the per-field slices were nil/zero-length. TS arrays auto-grow on
indexed assignment; goscape slices need explicit append.

Append a zero element to FileHash/FileName/FileUnpackedSize/
FilePackedSize/FilePos/FileWrite when index==FileCount and the
slice length is below FileCount+1.

Unblocks NAI-193 .varp client jagfile writes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `parseVarpConfig` in `pkg/pack/varp.go`

**Files:**
- Create: `pkg/pack/varp.go`
- Create: `pkg/pack/varp_test.go`

### Steps

- [ ] **Step 2.1: Write the failing tests**

Create `pkg/pack/varp_test.go`:

```go
package pack

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseVarpConfig_ClientCodeDecimal(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "7")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 7 {
		t.Fatalf("v=%v, want 7", v)
	}
}

func TestParseVarpConfig_ClientCodeHex(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "0x42")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 66 {
		t.Fatalf("v=%v, want 66", v)
	}
}

func TestParseVarpConfig_ClientCodeNegative(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "-5")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != -5 {
		t.Fatalf("v=%v, want -5", v)
	}
}

func TestParseVarpConfig_ClientCodeNonNumericRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("clientcode", "abc")
	if err == nil {
		t.Fatal("want err for non-numeric clientcode")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_ProtectBoolean(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"yes", true},
		{"no", false},
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
	} {
		v, ok, err := parseVarpConfig("protect", tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if !ok {
			t.Fatalf("%s: ok=false", tc.in)
		}
		if v.(bool) != tc.want {
			t.Fatalf("%s: v=%v, want %v", tc.in, v, tc.want)
		}
	}
}

func TestParseVarpConfig_ProtectInvalidRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("protect", "maybe")
	if err == nil {
		t.Fatal("want err for non-boolean protect")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_TransmitBoolean(t *testing.T) {
	v, ok, err := parseVarpConfig("transmit", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(bool) != true {
		t.Fatalf("v=%v, want true", v)
	}
}

func TestParseVarpConfig_ScopePerm(t *testing.T) {
	v, ok, err := parseVarpConfig("scope", "perm")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.VarpScopePerm {
		t.Fatalf("v=%v, want VarpScopePerm=%d", v, objtype.VarpScopePerm)
	}
}

func TestParseVarpConfig_ScopeTemp(t *testing.T) {
	v, ok, err := parseVarpConfig("scope", "temp")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.VarpScopeTemp {
		t.Fatalf("v=%v, want VarpScopeTemp=%d", v, objtype.VarpScopeTemp)
	}
}

func TestParseVarpConfig_ScopeUnknownRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("scope", "global")
	if err == nil {
		t.Fatal("want err for unknown scope")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_TypeAccepted(t *testing.T) {
	v, ok, err := parseVarpConfig("type", "int")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("v=%v, want ScriptVarTypeInt", v)
	}
}

func TestParseVarpConfig_TypeUnknownRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("type", "bogus")
	if err == nil {
		t.Fatal("want err for unknown type")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_UnknownKeyReturnsOkFalse(t *testing.T) {
	v, ok, err := parseVarpConfig("not_a_key", "whatever")
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
```

The `bytes`, `path/filepath` imports are placed in advance for Task 3's byte-pin test additions. Keep them; otherwise the next task's edit becomes noisier.

- [ ] **Step 2.2: Run tests to verify they fail (red phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseVarpConfig -v
```

Expected: **FAIL** — `parseVarpConfig` undefined.

- [ ] **Step 2.3: Write the implementation**

Create `pkg/pack/varp.go`:

```go
package pack

import (
	"fmt"
	"strconv"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseVarpConfig is the per-key=value parser for .varp config blocks.
//
// Accepted keys:
//   - clientcode  (number; decimal or 0x-prefixed hex)
//   - protect     (boolean; yes/no/true/false/1/0)
//   - transmit    (boolean; same value set as protect)
//   - scope       ("perm" | "temp" → VarpScopePerm | VarpScopeTemp)
//   - type        (ScriptVarType name → ScriptVarType code)
//
// Return contract (matches NAI-192 ParseFn):
//   - (value, true, nil)  → accepted
//   - (nil, true, err)    → recognized key with invalid value
//   - (nil, false, nil)   → unrecognized key
//
// TS source: tools/pack/config/VarpConfig.ts:5-67.
func parseVarpConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "clientcode":
		// strconv.ParseInt(value, 0, 64) accepts decimal AND 0x-prefixed
		// hex with a single call — equivalent to the TS branch on
		// value.startsWith('0x') plus regex validation plus NaN check.
		n, err := strconv.ParseInt(value, 0, 64)
		if err != nil {
			return nil, true, fmt.Errorf("invalid clientcode: %s", value)
		}
		return int(n), true, nil
	case "protect", "transmit":
		if !IsConfigBoolean(value) {
			return nil, true, fmt.Errorf("invalid boolean: %s", value)
		}
		return GetConfigBoolean(value), true, nil
	case "scope":
		switch value {
		case "perm":
			return objtype.VarpScopePerm, true, nil
		case "temp":
			return objtype.VarpScopeTemp, true, nil
		default:
			return nil, true, fmt.Errorf("invalid scope: %s", value)
		}
	case "type":
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	}
	return nil, false, nil
}
```

- [ ] **Step 2.4: Run tests to verify they pass (green phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestParseVarpConfig -v
```

Expected: **PASS** — all 13 parseVarpConfig tests green.

- [ ] **Step 2.5: Commit**

```bash
git add pkg/pack/varp.go pkg/pack/varp_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-193 T2 — parseVarpConfig

Per-key=value parser for .varp config blocks. Handles clientcode
(decimal + 0x hex), protect/transmit (boolean set), scope (perm/temp),
type (ScriptVarType name lookup).

strconv.ParseInt(value, 0, 64) handles both decimal and 0x-hex in one
call — equivalent to TS parseVarpConfig's branch on
value.startsWith('0x') plus regex + NaN check.

TS source: tools/pack/config/VarpConfig.ts:5-67.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `packVarpConfigs` byte-pin in `pkg/pack/varp.go`

**Why a separate task:** Byte-level expected outputs are easy to get wrong (entry-size includes the terminator, opcode-emission is asymmetric for protect/transmit). Isolating the byte-pin tests forces an explicit, table-able expected-output that the reviewer can cross-check against `pkg/pack/varn_test.go`'s established encoding.

**Files:**
- Modify: `pkg/pack/varp.go` (add `packVarpConfigs`)
- Modify: `pkg/pack/varp_test.go` (add byte-pin tests)

### Steps

- [ ] **Step 3.1: Write the failing byte-pin tests**

Append to `pkg/pack/varp_test.go`:

```go
// Byte-pin reference computation (id=0 = "run" with scope=temp, type=int,
// transmit=yes, clientcode=7; id=1 = empty slot):
//
// Server dat:
//   p2(size=2)                  → 00 02
//   id=0 body:
//     scope opcode + value      → 01 00     (scope=temp=0)
//     type opcode + value       → 02 69     (type=int=105=0x69)
//     transmit opcode (no val)  → 06        (only when value==true)
//     debugname opcode + LFstr  → fa 72 75 6e 0a   ("run" + LF)
//   Next() terminator           → 00
//   id=1 (empty) terminator     → 00
//
// Server idx:
//   p2(size=2)                  → 00 02
//   id=0 entry length 11        → 00 0b     (2+2+1+5 body + 1 terminator)
//   id=1 entry length 1         → 00 01     (terminator only)
//
// Client dat:
//   p2(size=2)                  → 00 02
//   id=0 body:
//     clientcode + p2 value     → 05 00 07
//   Next() terminator           → 00
//   id=1 terminator             → 00
//
// Client idx:
//   p2(size=2)                  → 00 02
//   id=0 entry length 4         → 00 04     (3 body + 1 terminator)
//   id=1 entry length 1         → 00 01

func TestPackVarpConfigs_BytePin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[run]\nscope=temp\ntype=int\ntransmit=yes\nclientcode=7\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=run\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	// pf.Max is len(pf.Pack) which excludes empty trailing slots; force
	// Max=2 so the second slot's empty-terminator is emitted (matches
	// TS packVarpConfigs which iterates [0, VarpPack.max)). The cleanest
	// way is to bump pf.Max directly for the test fixture.
	pf.Max = 2

	server, client := packVarpConfigs(cfgs, pf)

	wantServerDat := []byte{
		0x00, 0x02,
		0x01, 0x00,
		0x02, 0x69,
		0x06,
		0xfa, 0x72, 0x75, 0x6e, 0x0a,
		0x00,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}

	wantServerIdx := []byte{0x00, 0x02, 0x00, 0x0b, 0x00, 0x01}
	if !bytes.Equal(server.Idx.Data, wantServerIdx) {
		t.Fatalf("server.idx=% x\nwant % x", server.Idx.Data, wantServerIdx)
	}

	wantClientDat := []byte{
		0x00, 0x02,
		0x05, 0x00, 0x07,
		0x00,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, wantClientDat) {
		t.Fatalf("client.dat=% x\nwant % x", client.Dat.Data, wantClientDat)
	}

	wantClientIdx := []byte{0x00, 0x02, 0x00, 0x04, 0x00, 0x01}
	if !bytes.Equal(client.Idx.Data, wantClientIdx) {
		t.Fatalf("client.idx=% x\nwant % x", client.Idx.Data, wantClientIdx)
	}
}

func TestPackVarpConfigs_ProtectFalseEmitsOpcode(t *testing.T) {
	// protect=true is the DEFAULT (NewVarPlayerType). TS pack emits
	// opcode 4 ONLY when value==false. Verify the asymmetry.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[v]\nprotect=no\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=v\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}

	server, _ := packVarpConfigs(cfgs, pf)

	// Server dat for id=0 with protect=false:
	//   p2(1)               → 00 01
	//   protect opcode 4    → 04
	//   debugname trailer   → fa 76 0a   ("v" + LF)
	//   Next() terminator   → 00
	wantServerDat := []byte{0x00, 0x01, 0x04, 0xfa, 0x76, 0x0a, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}
}

func TestPackVarpConfigs_ProtectTrueOmitsOpcode(t *testing.T) {
	// protect=true should NOT emit opcode 4. Only the debugname trailer
	// + terminator are written.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[v]\nprotect=yes\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=v\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}

	server, _ := packVarpConfigs(cfgs, pf)

	// Server dat for id=0 with protect=true (no opcode emitted):
	//   p2(1)               → 00 01
	//   debugname trailer   → fa 76 0a
	//   Next() terminator   → 00
	wantServerDat := []byte{0x00, 0x01, 0xfa, 0x76, 0x0a, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}
}

func TestPackVarpConfigs_TransmitFalseOmitsOpcode(t *testing.T) {
	// transmit=false is the DEFAULT. TS pack emits opcode 6 ONLY when
	// value==true. Inverse asymmetry vs protect.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[v]\ntransmit=no\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=v\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}

	server, _ := packVarpConfigs(cfgs, pf)

	// Server dat for id=0 with transmit=false (no opcode emitted):
	//   p2(1)               → 00 01
	//   debugname trailer   → fa 76 0a
	//   Next() terminator   → 00
	wantServerDat := []byte{0x00, 0x01, 0xfa, 0x76, 0x0a, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail (red phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackVarpConfigs -v
```

Expected: **FAIL** — `packVarpConfigs` undefined.

- [ ] **Step 3.3: Write the implementation**

Append to `pkg/pack/varp.go`:

```go
// packVarpConfigs walks every id in [0, pf.Max), pulls the debugname
// from the PackFile, emits per-config opcodes on the server buffer
// (scope=1, type=2, protect-when-false=4, transmit-when-true=6,
// debugname-trailer=250) and on the client buffer (clientcode=5+p2).
// Each slot ends with PackedData.Next() on both buffers — a single
// 0x00 terminator + idx entry-length.
//
// Returns server first to match parseVarpTypes' read order in
// pkg/objtype/varptype.go (server count first, then per-slot
// server-decode then client-decode).
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarpPack; goscape takes *PackFile as a parameter (continuation of
// NAI-191 §2 / NAI-192 deferral).
//
// TS source: tools/pack/config/VarpConfig.ts:69-110.
// TS author note at VarpConfig.ts:97 — "// todo: maybe this was
// opcode 10?" — preserved here as a TS-author uncertainty about the
// 250 trailer opcode, not a goscape deviation.
func packVarpConfigs(configs map[string][]ConfigLine, pf *PackFile) (server, client *PackedData) {
	server = NewPackedData(pf.Max)
	client = NewPackedData(pf.Max)

	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			for _, line := range cfg {
				switch line.Key {
				case "scope":
					server.P1(1)
					server.P1(uint8(line.Value.(int)))
				case "type":
					server.P1(2)
					server.P1(uint8(line.Value.(objtype.ScriptVarType)))
				case "protect":
					if !line.Value.(bool) {
						server.P1(4)
					}
				case "clientcode":
					client.P1(5)
					client.P2(uint16(line.Value.(int)))
				case "transmit":
					if line.Value.(bool) {
						server.P1(6)
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
	return server, client
}
```

- [ ] **Step 3.4: Run tests to verify they pass (green phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run "TestParseVarpConfig|TestPackVarpConfigs" -v
```

Expected: **PASS** — all parseVarpConfig + packVarpConfigs tests green.

If the byte-pin test fails with a one-off difference in the idx sizes, suspect the `Next()` cursor math — re-read `pkg/pack/packed_data.go` line 38-44 and `pkg/pack/varn_test.go`'s established encoding to confirm the entry-size IS measured including the terminator.

- [ ] **Step 3.5: Commit**

```bash
git add pkg/pack/varp.go pkg/pack/varp_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-193 T3 — packVarpConfigs

Walks pf.Max slots, emitting server opcodes (1=scope, 2=type,
4=protect-when-false, 6=transmit-when-true, 250=debugname trailer)
and client opcodes (5+p2=clientcode). Both buffers Next() per slot,
including empty slots.

Returns server first to match parseVarpTypes' read order in
pkg/objtype/varptype.go.

Three byte-pin tests: full-slot, protect=false (opcode 4 emitted),
protect=true and transmit=false (opcodes 4 and 6 omitted). Pins
the asymmetric-emission contract for both fields.

NAI-193-D-PACKFILE-SINGLETONS-DEFERRED tag inline.

TS source: tools/pack/config/VarpConfig.ts:69-110.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `checkVarNameUniqueness` in `pkg/pack/pack_configs.go`

**Why a separate task:** The function is pure (takes `*PackFile`s, returns error) and the unit tests bind the rejection contract before the orchestrator integration test (Task 5) wires it in. Retires `NAI-192-D-VARP-UNIQUENESS-DEFERRED` — varp is the third and final of the var-name trio.

**Files:**
- Modify: `pkg/pack/pack_configs.go` (add `checkVarNameUniqueness` function only — orchestrator wiring lands in Task 5)
- Create: `pkg/pack/check_unique_test.go` (unit tests for `checkVarNameUniqueness`)

### Steps

- [ ] **Step 4.1: Write the failing tests**

Create `pkg/pack/check_unique_test.go`:

```go
package pack

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckVarNameUniqueness_DistinctNamesAcrossPacks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"), "0=health\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"), "0=npctier\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"), "0=shared_a\n")
	ClearFsCache()

	varp, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	varn, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := NewPackFile(dir, "vars", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := checkVarNameUniqueness(varp, varn, vars); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
}

func TestCheckVarNameUniqueness_DuplicateAcrossPacks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"), "0=dup_name\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"), "0=dup_name\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"), "0=shared_a\n")
	ClearFsCache()

	varp, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	varn, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := NewPackFile(dir, "vars", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = checkVarNameUniqueness(varp, varn, vars)
	if err == nil {
		t.Fatal("want error for duplicate name across packs")
	}
	if !strings.Contains(err.Error(), "dup_name") {
		t.Fatalf("err=%q, want it to mention dup_name", err)
	}
}

func TestCheckVarNameUniqueness_EmptySlotsIgnored(t *testing.T) {
	// Sparse pack file with gaps — empty slots must not collide.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"), "0=a\n5=b\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"), "0=c\n3=d\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"), "")
	ClearFsCache()

	varp, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	varn, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := NewPackFile(dir, "vars", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := checkVarNameUniqueness(varp, varn, vars); err != nil {
		t.Fatalf("want no error for sparse packs, got %v", err)
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail (red phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestCheckVarNameUniqueness -v
```

Expected: **FAIL** — `checkVarNameUniqueness` undefined.

- [ ] **Step 4.3: Write the implementation**

Append to `pkg/pack/pack_configs.go` (do NOT touch the orchestrator yet — that's Task 5):

```go
// checkVarNameUniqueness rejects when any debugname appears in more
// than one of the supplied PackFiles. Sparse slots (empty name) are
// ignored. Error message names the duplicated identifier and the
// pack-type ("varp", "varn", "vars") of the first declaration.
//
// Retires NAI-192-D-VARP-UNIQUENESS-DEFERRED — varp is the third and
// final of the var-name trio, so this check can land now.
//
// TS source: tools/pack/config/PackShared.ts:292-310.
func checkVarNameUniqueness(pfs ...*PackFile) error {
	seen := map[string]string{} // name → pack type that first declared it
	for _, pf := range pfs {
		for id := range pf.Max {
			name := pf.GetByID(id)
			if name == "" {
				continue
			}
			if prior, dup := seen[name]; dup {
				return fmt.Errorf("non-unique var name %q (declared in %s and again in %s)", name, prior, pf.Type)
			}
			seen[name] = pf.Type
		}
	}
	return nil
}
```

If `pack_configs.go` does not already import `fmt`, add it. Check existing imports first.

- [ ] **Step 4.4: Run tests to verify they pass (green phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestCheckVarNameUniqueness -v
```

Expected: **PASS** — all three uniqueness tests green.

- [ ] **Step 4.5: Commit**

```bash
git add pkg/pack/pack_configs.go pkg/pack/check_unique_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-193 T4 — checkVarNameUniqueness

Pure function — takes any number of *PackFile, returns error on
duplicate debugname across packs. Sparse slots (empty name) ignored.

Lands here as a unit; orchestrator wiring follows in T5.

Retires NAI-192-D-VARP-UNIQUENESS-DEFERRED — the deviation tag's
in-source comment is removed in T5 when the orchestrator wires the
call; the pin test in nai192_deviation_pins_test.go is removed in T7.

TS source: tools/pack/config/PackShared.ts:292-310.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Extend `PackConfigs` orchestrator with fresh jagfile + varp branch + uniqueness call

**Why a single task:** The four changes (refactor varn/vars helpers to accept `*PackFile`, construct three `*PackFile` up-front, call `checkVarNameUniqueness`, add fresh-jagfile + varp branch + conditional client save) all touch `pack_configs.go` and must land together to keep NAI-192's existing integration tests green.

**Files:**
- Modify: `pkg/pack/pack_configs.go` (orchestrator rewrite + varp branch + helper signature refactor)
- Modify: `pkg/pack/pack_configs_test.go` (add varp-only, mixed, no-client-branch, uniqueness-rejection tests)

### Steps

- [ ] **Step 5.1: Write the failing integration tests**

Append to `pkg/pack/pack_configs_test.go`:

```go
func TestPackConfigs_VarpOnly_ProducesServerAndClientJagfile(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[run]\nscope=perm\ntype=int\ntransmit=yes\nclientcode=7\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=run\n")
	// Empty varn/vars packs so the orchestrator's PackFile construction
	// for the uniqueness check has all three available.
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(outDir, "server", "varp.dat"),
		filepath.Join(outDir, "server", "varp.idx"),
		filepath.Join(outDir, "client", "config"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}
}

func TestPackConfigs_MixedVarpVarnVars(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "a.varp"),
		"[health]\nscope=perm\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "c.vars"),
		"[shared_a]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=health\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npctier\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=shared_a\n")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(outDir, "server", "varp.dat"),
		filepath.Join(outDir, "server", "varp.idx"),
		filepath.Join(outDir, "server", "varn.dat"),
		filepath.Join(outDir, "server", "varn.idx"),
		filepath.Join(outDir, "server", "vars.dat"),
		filepath.Join(outDir, "server", "vars.idx"),
		filepath.Join(outDir, "client", "config"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}
}

func TestPackConfigs_NoVarpSource_NoClientJagfileWritten(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// .varn only. No .varp source ⇒ no client-side branch fires ⇒
	// client/config jagfile must NOT be written.
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npctier\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "server", "varn.dat")); err != nil {
		t.Fatalf("expected varn.dat to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "client", "config")); !os.IsNotExist(err) {
		t.Fatalf("expected client/config to NOT exist; got err=%v", err)
	}
}

func TestPackConfigs_CrossDomainUniquenessRejection(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "a.varp"),
		"[dup_name]\nscope=perm\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varn"),
		"[dup_name]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=dup_name\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=dup_name\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatal("want error for cross-domain duplicate name")
	}
	if !strings.Contains(err.Error(), "dup_name") {
		t.Fatalf("err=%q, want it to mention dup_name", err)
	}

	// And no server-side outputs should exist (uniqueness check runs
	// before any branch fires).
	if _, err := os.Stat(filepath.Join(outDir, "server", "varp.dat")); !os.IsNotExist(err) {
		t.Fatalf("expected no varp.dat after early reject; got err=%v", err)
	}
}
```

If `pkg/pack/pack_configs_test.go` does not already import `strings`, add it. The `os` and `path/filepath` imports should already be present from NAI-192 tests.

- [ ] **Step 5.2: Run tests to verify they fail (red phase)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs -v
```

Expected: NEW tests **FAIL** (because the orchestrator hasn't been rewritten yet). Existing NAI-192 PackConfigs tests should still **PASS** at this point (no behavior change in the helpers yet).

- [ ] **Step 5.3: Rewrite the orchestrator**

Replace the entire body of `pkg/pack/pack_configs.go` with:

```go
package pack

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// PackConfigs runs the per-config packing pipeline. NAI-193 wires
// .varp (server + client jagfile), .varn (server), and .vars (server).
// Subsequent NAI-194+ sub-specs add the remaining per-config branches.
//
// Each branch is freshness-gated via ShouldBuild against the relevant
// source extension. Server outputs land at <outDir>/server/<type>.{dat,idx}.
// Client outputs land in a fresh jagfile at <outDir>/client/config —
// saved only if at least one client-side branch fires.
//
// All three var-domain PackFiles are constructed up-front so the
// cross-domain uniqueness check (which retires
// NAI-192-D-VARP-UNIQUENESS-DEFERRED) has all three name maps
// available. Each *.pack file is small (<1 KB); cost is fixed.
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarpPack/VarnPack/VarsPack singletons; goscape constructs *PackFile
// from srcDir per call (continuation of NAI-191 §2 / NAI-192
// deferral of all 26 module-level pack singletons).
//
// NAI-193-D-FRESH-CLIENT-JAGFILE: client jagfile starts fresh
// (NewJagfile(nil)). Pre-existing entries in <outDir>/client/config
// are truncated if only a subset of client-side branches rebuild.
// Mirrors TS Jagfile.new() at PackShared.ts:336.
//
// NAI-193-D-VALIDATE-DEFERRED: TS BUILD_VERIFY callback (.varp magic
// 705633567 at PackShared.ts:631-633) deferred — continuation of
// NAI-191 §2.
//
// NAI-192-D-NO-SRC-NO-OP: goscape-only `GetLatestModified > 0`
// pre-guard suppresses output when no source files exist. TS would
// enter ShouldBuild's output-missing arm and write a zero-entry
// .dat/.idx pair; goscape elides that write.
//
// TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).
func PackConfigs(srcDir, outDir string) error {
	constants, err := LoadConstants(srcDir)
	if err != nil {
		return err
	}

	// Construct all three var-domain PackFiles up-front for the
	// cross-domain uniqueness check (retires
	// NAI-192-D-VARP-UNIQUENESS-DEFERRED).
	varpPack, err := NewPackFile(srcDir, "varp", nil)
	if err != nil {
		return err
	}
	varnPack, err := NewPackFile(srcDir, "varn", nil)
	if err != nil {
		return err
	}
	varsPack, err := NewPackFile(srcDir, "vars", nil)
	if err != nil {
		return err
	}

	if err := checkVarNameUniqueness(varpPack, varnPack, varsPack); err != nil {
		return err
	}

	scriptsDir := filepath.Join(srcDir, "scripts")
	serverOut := filepath.Join(outDir, "server")
	clientOut := filepath.Join(outDir, "client")

	// Fresh client jagfile per NAI-193-D-FRESH-CLIENT-JAGFILE. Saved
	// only when at least one client-side branch contributes a write.
	clientJag, err := jagfile.NewJagfile(nil)
	if err != nil {
		return err
	}
	clientJagDirty := false

	if GetLatestModified(scriptsDir, ".varp") > 0 &&
		ShouldBuild(scriptsDir, ".varp", filepath.Join(serverOut, "varp.dat")) {
		if err := packAndSaveVarp(srcDir, serverOut, varpPack, constants, clientJag); err != nil {
			return err
		}
		clientJagDirty = true
	}

	if GetLatestModified(scriptsDir, ".varn") > 0 &&
		ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
		if err := packAndSaveVarn(srcDir, serverOut, varnPack, constants); err != nil {
			return err
		}
	}

	if GetLatestModified(scriptsDir, ".vars") > 0 &&
		ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
		if err := packAndSaveVars(srcDir, serverOut, varsPack, constants); err != nil {
			return err
		}
	}

	if clientJagDirty {
		if err := clientJag.Save(filepath.Join(clientOut, "config"), false); err != nil {
			return err
		}
	}
	return nil
}

// checkVarNameUniqueness rejects when any debugname appears in more
// than one of the supplied PackFiles. Sparse slots (empty name) are
// ignored. Error message names the duplicated identifier and the
// pack-type ("varp", "varn", "vars") of the first declaration.
//
// Retires NAI-192-D-VARP-UNIQUENESS-DEFERRED — varp is the third and
// final of the var-name trio, so this check can land now.
//
// TS source: tools/pack/config/PackShared.ts:292-310.
func checkVarNameUniqueness(pfs ...*PackFile) error {
	seen := map[string]string{}
	for _, pf := range pfs {
		for id := range pf.Max {
			name := pf.GetByID(id)
			if name == "" {
				continue
			}
			if prior, dup := seen[name]; dup {
				return fmt.Errorf("non-unique var name %q (declared in %s and again in %s)", name, prior, pf.Type)
			}
			seen[name] = pf.Type
		}
	}
	return nil
}

func packAndSaveVarp(srcDir, serverOut string, pf *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".varp", nil, parseVarpConfig, c)
	if err != nil {
		return err
	}
	server, client := packVarpConfigs(cfgs, pf)
	if err := server.Save(
		filepath.Join(serverOut, "varp.dat"),
		filepath.Join(serverOut, "varp.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("varp.dat", client.Dat)
	clientJag.Write("varp.idx", client.Idx)
	return nil
}

func packAndSaveVarn(srcDir, serverOut string, pf *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".varn", nil, parseVarnConfig, c)
	if err != nil {
		return err
	}
	pd := packVarnConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "varn.dat"), filepath.Join(serverOut, "varn.idx"))
}

func packAndSaveVars(srcDir, serverOut string, pf *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".vars", nil, parseVarsConfig, c)
	if err != nil {
		return err
	}
	pd := packVarsConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "vars.dat"), filepath.Join(serverOut, "vars.idx"))
}
```

Key points the reviewer must check:
1. `checkVarNameUniqueness` declared once. If Task 4 already added it and this rewrite duplicates it, **delete the Task-4 placement** (the new orchestrator file IS the canonical home). The function body in this task is identical to Task 4's — by keeping the function definition only here, the file stays single-source-of-truth.
2. `packAndSaveVarn` / `packAndSaveVars` signatures changed: now take `*PackFile` as a parameter. Old signature was `(srcDir, serverOut, constants)` with internal `NewPackFile` call. New signature is `(srcDir, serverOut, pf, constants)`.
3. `packAndSaveVarp` signature: `(srcDir, serverOut, pf, c, clientJag)`. Note `clientJag` is `*jagfile.Jagfile`, threaded by the orchestrator.
4. All three NAI-192-D-* deviation tag comments and the NAI-192-D-VARP-UNIQUENESS-DEFERRED comment block from the old `pack_configs.go` body are dropped. The new doc comment on `PackConfigs` enumerates the surviving deviation tags (NAI-193-D-*, NAI-192-D-NO-SRC-NO-OP) and the retirement of NAI-192-D-VARP-UNIQUENESS-DEFERRED.

- [ ] **Step 5.4: Run all `pkg/pack` tests**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -count=1 -v
```

Expected: **PASS** — both new NAI-193 tests AND all existing NAI-192 tests.

If `TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator` FAILS at this point, that's expected behavior — the deviation is retired; the pin test will be deleted in Task 7. The plan-author intent here is for ALL OTHER tests to remain green; this one specific pin test failure is the signal that the retirement landed. To keep `go test ./pkg/pack/` green between T5 commit and T7 commit, delete that single pin test now as part of this same task:

In `pkg/pack/nai192_deviation_pins_test.go`, locate `TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator` and delete the whole function. Leave the other NAI-192-D pin tests intact.

Re-run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -count=1 -race
```

Expected: **PASS** — fully green with race detector.

- [ ] **Step 5.5: Run the whole repo to confirm no consumer of `packAndSaveV*` regressed**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: **PASS** — `packAndSaveVarn` / `packAndSaveVars` are private to `pkg/pack`, so the signature refactor is internal-only.

- [ ] **Step 5.6: Commit**

```bash
git add pkg/pack/pack_configs.go pkg/pack/pack_configs_test.go pkg/pack/nai192_deviation_pins_test.go pkg/pack/check_unique_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-193 T5 — PackConfigs varp branch + uniqueness check

Extends PackConfigs orchestrator:
- Constructs varp/varn/vars PackFiles up-front for the cross-domain
  name-uniqueness check.
- Calls checkVarNameUniqueness across all three — retires
  NAI-192-D-VARP-UNIQUENESS-DEFERRED.
- Opens a fresh client-side jagfile via NewJagfile(nil); .varp branch
  threads server (.dat/.idx) to disk and queues varp.dat/varp.idx into
  the jagfile.
- Saves the client jagfile to <outDir>/client/config only when at
  least one client-side branch contributed a write (avoids an empty
  jagfile artifact when only .varn/.vars rebuild).

packAndSaveVarn/packAndSaveVars signatures take *PackFile as a
parameter (helpers no longer construct their own PackFile). New
packAndSaveVarp helper for the dual-output path.

Retires the pin test TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator
(the deviation it pinned is now retired).

Four new integration tests: varp-only, mixed varp+varn+vars, no-varp
(asserts client jagfile NOT written), cross-domain uniqueness rejection.

NAI-193-D-FRESH-CLIENT-JAGFILE + NAI-193-D-PACKFILE-SINGLETONS-DEFERRED
+ NAI-193-D-VALIDATE-DEFERRED tags inline.

TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Cross-package round-trip via `LoadVarpTypes`

**Why a separate task:** Binds packer output ↔ existing-loader parity. Independent of the integration tests in T5 (which only check file existence + uniqueness). The round-trip test exercises the full byte-format contract: `PackConfigs` writes both server `.dat`/`.idx` and the client jagfile; `LoadVarpTypes` decodes both back and recovers every field of `VarPlayerType`.

**Files:**
- Create: `pkg/pack/varp_roundtrip_test.go`

### Steps

- [ ] **Step 6.1: Write the failing test**

Create `pkg/pack/varp_roundtrip_test.go`:

```go
package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestVarpPacker_LoaderRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[run]\nscope=perm\ntype=int\ntransmit=yes\nclientcode=7\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=run\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadVarpTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs.Configs) != 1 {
		t.Fatalf("len(Configs)=%d, want 1", len(cfgs.Configs))
	}
	got := cfgs.Configs[0]
	if got.DebugName != "run" {
		t.Errorf("DebugName=%q, want %q", got.DebugName, "run")
	}
	if got.Scope != objtype.VarpScopePerm {
		t.Errorf("Scope=%d, want VarpScopePerm=%d", got.Scope, objtype.VarpScopePerm)
	}
	if got.Type != objtype.ScriptVarTypeInt {
		t.Errorf("Type=%v, want ScriptVarTypeInt", got.Type)
	}
	if !got.Transmit {
		t.Errorf("Transmit=false, want true")
	}
	if got.ClientCode != 7 {
		t.Errorf("ClientCode=%d, want 7", got.ClientCode)
	}
	if cfgs.RunID != 0 {
		t.Errorf("RunID=%d, want 0 (clientcode==7 ⇒ engine run-mode varp)", cfgs.RunID)
	}
}

func TestVarpPacker_LoaderRoundTrip_ProtectFalse(t *testing.T) {
	// Tighter test for protect=false → opcode 4 emission → loader sets
	// Protect=false (overriding the NewVarPlayerType default of true).
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[unprotected]\nscope=temp\ntype=int\nprotect=no\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=unprotected\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadVarpTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	got := cfgs.Configs[0]
	if got.Protect {
		t.Errorf("Protect=true, want false (opcode 4 should have flipped the default)")
	}
}

func TestVarpPacker_LoaderRoundTrip_ProtectDefaultsTrue(t *testing.T) {
	// Inverse check — when protect is unset, loader should preserve the
	// NewVarPlayerType default (true).
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[protected]\nscope=temp\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=protected\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadVarpTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	got := cfgs.Configs[0]
	if !got.Protect {
		t.Errorf("Protect=false, want true (default)")
	}
}
```

- [ ] **Step 6.2: Run tests to verify they pass (should be green already)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestVarpPacker_LoaderRoundTrip -v
```

Expected: **PASS** — packer + loader are both already implemented; this task binds the end-to-end contract.

If any sub-test fails, it indicates a bug in either the packer (T3) or in `pkg/objtype/varptype.go`'s `DecodeType` opcode handling. Re-read `varptype.go:21-47` and the byte-pin reference at T3 to triangulate.

- [ ] **Step 6.3: Commit**

```bash
git add pkg/pack/varp_roundtrip_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-193 T6 — varp round-trip via LoadVarpTypes

End-to-end test: PackConfigs writes server .dat/.idx + client jagfile;
LoadVarpTypes decodes both back and recovers Scope, Type, Transmit,
ClientCode, DebugName, RunID, and Protect.

Three test functions: full-field (clientcode=7 ⇒ RunID=0), explicit
protect=false (opcode 4 flips default), absent protect (preserves
NewVarPlayerType default of true). Asymmetric protect/transmit
emission is the most likely byte-pin bug; this round-trip is the
defense in depth.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: NAI-193 deviation-tag pin tests + final scan

**Why last:** Pins must reflect post-implementation source. Premature pins block legitimate edits during T1-T6.

**Files:**
- Create: `pkg/pack/nai193_deviation_pins_test.go`

### Steps

- [ ] **Step 7.1: Write the pin tests**

Create `pkg/pack/nai193_deviation_pins_test.go`:

```go
package pack

import (
	"os"
	"strings"
	"testing"
)

// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: no top-level VarpPack decl in
// pkg/pack (mirrors the NAI-192 absence-pin for VarnPack/VarsPack).
// scanPackageDecls helper lives in nai192_deviation_pins_test.go.
func TestNAI193_PackFileSingletonsDeferred_NoModuleLevelVarpPack(t *testing.T) {
	decls := scanPackageDecls(t)
	if decls["VarpPack"] {
		t.Errorf("found top-level decl \"VarpPack\" in pkg/pack — violates NAI-193-D-PACKFILE-SINGLETONS-DEFERRED")
	}
}

// NAI-193-D-VALIDATE-DEFERRED: pkg/pack/varp.go must NOT reference any
// BUILD_VERIFY-style validate callback identifiers.
func TestNAI193_ValidateDeferred_NoBuildVerifyInVarpSource(t *testing.T) {
	body, err := os.ReadFile("varp.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"BuildVerify", "BUILD_VERIFY", "validateVarp", "checkCRC", "checkcrc"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("found %q in pkg/pack/varp.go — violates NAI-193-D-VALIDATE-DEFERRED", banned)
		}
	}
}

// NAI-193-D-FRESH-CLIENT-JAGFILE: PackConfigs must construct the client
// jagfile via NewJagfile(nil) and must NOT call LoadJagfile (which
// would indicate the deviation has flipped to "preserve existing
// entries").
func TestNAI193_FreshClientJagfile_NewNotLoad(t *testing.T) {
	body, err := os.ReadFile("pack_configs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "NewJagfile(nil)") {
		t.Errorf("pkg/pack/pack_configs.go must call NewJagfile(nil) — NAI-193-D-FRESH-CLIENT-JAGFILE")
	}
	if strings.Contains(string(body), "LoadJagfile") {
		t.Errorf("pkg/pack/pack_configs.go must NOT call LoadJagfile — flipping NAI-193-D-FRESH-CLIENT-JAGFILE would require a new deviation tag")
	}
}

// Verifies that NAI-192-D-VARP-UNIQUENESS-DEFERRED has been retired:
// PackConfigs source must contain the uniqueness-check call (not the
// TODO comment).
func TestNAI193_UniquenessCheckRetiredVarpDeferral(t *testing.T) {
	body, err := os.ReadFile("pack_configs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "checkVarNameUniqueness") {
		t.Errorf("pkg/pack/pack_configs.go must call checkVarNameUniqueness — NAI-192-D-VARP-UNIQUENESS-DEFERRED is retired")
	}
}
```

- [ ] **Step 7.2: Run the pin tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestNAI193 -v
```

Expected: **PASS** — all four pin tests green.

- [ ] **Step 7.3: Verify the NAI-192 uniqueness pin is GONE**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestNAI192_VarpUniquenessDeferred -v
```

Expected: `no tests to run` (test was deleted in T5).

- [ ] **Step 7.4: Grep for any residual NAI-192-D-VARP-UNIQUENESS-DEFERRED references**

Per `retire_deviation_grep_all_comments` memory: enumerate every reference, not just production code.

Run:
```
rg "NAI-192-D-VARP-UNIQUENESS-DEFERRED" pkg/ modules/ cmd/
```

Expected: **zero matches**. If any remain, delete them with a one-line edit and re-grep. (The spec doc and plan doc are intentionally permitted to mention the retired tag — they document the historical state. Grep is scoped to source directories `pkg/ modules/ cmd/`.)

- [ ] **Step 7.5: Run the full goscape test suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race
```

Expected: **PASS** — entire repo green with race detector.

- [ ] **Step 7.6: Run go vet + gofmt audit**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
gofmt -l pkg/io/jagfile pkg/pack pkg/objtype
```

Expected: vet clean (no output); gofmt list empty (no output).

- [ ] **Step 7.7: Commit**

```bash
git add pkg/pack/nai193_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-193 T7 — deviation-tag absence pins

Per ts_asymmetry_dual_pin memory: presence-pin AND absence-pin for
every deviation. A drive-by re-adding any deviated-against TS feature
will fail one of these.

Four pins:
- PACKFILE-SINGLETONS-DEFERRED: no top-level VarpPack decl
- VALIDATE-DEFERRED: pkg/pack/varp.go has no BUILD_VERIFY identifiers
- FRESH-CLIENT-JAGFILE: pack_configs.go calls NewJagfile(nil), not
  LoadJagfile
- (Retirement pin) checkVarNameUniqueness present in pack_configs.go
  — confirms NAI-192-D-VARP-UNIQUENESS-DEFERRED retirement

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| §4.1 Jagfile auto-grow fix | T1 |
| §4.2 parseVarpConfig | T2 |
| §4.3 packVarpConfigs | T3 |
| §4.4 PackConfigs extension (uniqueness, fresh jagfile, varp branch, varn/vars helper refactor) | T4 (uniqueness unit) + T5 (orchestrator) |
| §4.5 NAI-193 deviation-tag pins + NAI-192 uniqueness pin retirement | T7 (pin file) + T5 (pin deletion) |
| §7.1 Jagfile fresh-empty round-trip | T1.1 |
| §7.2 parseVarpConfig per-key coverage | T2.1 |
| §7.3 packVarpConfigs byte-pin (server + client) | T3.1 |
| §7.4 Cross-package loader round-trip | T6 |
| §7.5 PackConfigs integration tests (varp-only, mixed, no-client-branch, uniqueness rejection) | T5.1 |
| §7.6 Deviation-tag pins (NAI-193 file + NAI-192 deletion) | T7 + T5.4 |
| §10 NAI-193 deviation tags (PACKFILE-SINGLETONS-DEFERRED, VALIDATE-DEFERRED, FRESH-CLIENT-JAGFILE) | All — codified in code comments + T7 pins |
| §10 retired tag (NAI-192-D-VARP-UNIQUENESS-DEFERRED) | T5 (orchestrator removes the TODO + comment; deletes the pin test) |

**Placeholder scan:** No `TBD`/`TODO`/`fill in later`/"similar to" — every code block contains actual implementation. The single `TODO(NAI-VARP+)` in the NAI-192 `pack_configs.go` is REMOVED in T5's rewrite (the new orchestrator wires the uniqueness check directly).

**Type consistency:**
- `parseVarpConfig` signature `(key, value string) (ConfigValue, bool, error)` — declared in T2.3, called identically in T3 (via `packVarpConfigs`) and T5 (via `ReadTypedConfigs`).
- `packVarpConfigs(configs map[string][]ConfigLine, pf *PackFile) (server, client *PackedData)` — declared in T3.3, called identically in T5 (`packAndSaveVarp`).
- `checkVarNameUniqueness(pfs ...*PackFile) error` — declared in T4 (transient) and T5 (canonical location). Task 5 explicitly notes the Task-4 placement is replaced by the orchestrator rewrite — single source of truth in `pack_configs.go`.
- `packAndSaveVarn(srcDir, serverOut string, pf *PackFile, c Constants) error` and `packAndSaveVars(...)` — signature change ratified in T5; both helpers are private to `pkg/pack`.
- `packAndSaveVarp(srcDir, serverOut string, pf *PackFile, c Constants, clientJag *jagfile.Jagfile) error` — new helper in T5; threads `*jagfile.Jagfile`.
- `jagfile.NewJagfile(nil)` returns `*Jagfile, error` — verified at packfile.go grep.
- `*jagfile.Jagfile.Write(name string, data *packet.Packet)` — verified at jagfile.go:85.
- `*jagfile.Jagfile.Save(path string, doNotCompressWhole bool) error` — verified at jagfile.go:119.
- `objtype.VarpScopePerm` / `VarpScopeTemp` — both are untyped `int` constants per `pkg/objtype/varptype.go:11-13`. parseVarpConfig returns them as `int`; packVarpConfigs accesses as `line.Value.(int)`. Consistent.

All good. Plan ready to execute.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-13-nai-193-varp-packer.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task, two-stage review between tasks, fast iteration. Best for this plan because the task chain has strict file-scope dependencies (T5 depends on T1-T4; T6 binds the full chain; T7 pins post-implementation state).

2. **Inline Execution** — execute all tasks in this session with executing-plans, batch checkpoints for review.

Which approach?
