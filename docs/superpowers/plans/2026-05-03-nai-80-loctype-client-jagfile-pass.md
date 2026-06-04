# NAI-80 — LocType client-jagfile pass + full TS-shape port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the missing client-jagfile decode pass into `LoadLocTypes`, port the full TS-shape `LocType` struct + `Decode` arms + `PostDecode`, and pin loc 3014/380/350 cascade-blocker fix with a real-cache regression test.

**Architecture:** Mechanical mirror of the existing `NpcType` dual-pass precedent (`npctype.go:348-403`). Each LocType gets two decode passes per-id: server blob (sparse: codes 61, 249, 250) then client jagfile entry `loc.dat` (dense: codes 1-73). `PostDecode` runs after both passes to infer `Active` from `Shapes`/`Op`. The `"hidden"→""` coercion in op-slot decode is preserved verbatim as tracked deviation NAI-80-D1.

**Tech Stack:** Go 1.26+

**Spec:** `docs/superpowers/specs/2026-05-03-nai-80-loctype-client-jagfile-pass-design.md`

**Predecessor close:** `604cdb7` (NAI-79 H4 close — Stage 2.5 routing).

---

## File Structure

| File | Type | Responsibility |
|---|---|---|
| `pkg/objtype/loctype.go` | modify | Extend `LocType` struct with TS render fields; extend `Decode` switch; add `PostDecode`; modify `LoadLocTypes`/`parseLocTypes` to dual-pass; update top-of-file comment. |
| `pkg/objtype/loctype_test.go` | modify | Split `buildLocDat` into `buildLocServerDat` + `buildLocClientJag`. Update existing tests to dual-pass. Add per-code-arm tests for new codes. Add `TestPostDecode_*` triple. Add `TestParseLocTypes_HiddenCoercion` D1 pin. |
| `pkg/objtype/loctype_realcache_test.go` | new | `TestLoadLocTypes_RealCache_CascadeBlockerLocs` — pin Op[0] non-empty for ids 3014/380/350; ID-shift sanity probe. |

`modules/world/server.go:228` (`objtype.LoadLocTypes(cfg.CachePath)`) — no change; `LoadLocTypes(dir)` signature is unchanged.

`pkg/objtype/configtype.go::DecodeType` — no change; the loop already calls `Decode(code, dat)` until terminator.

---

## Task 1: Extend `LocType` struct with TS render fields + defaults via `NewLocType`

**Files:**
- Modify: `pkg/objtype/loctype.go:18-26` (struct), `:62-70` (`NewLocType`)

This is a non-functional struct expansion. After this task, all new fields exist with TS-matching zero/default values, but no decoder writes to them yet (Decode arms ship in Task 3). Existing tests continue passing because struct literal partial-init is forward-compatible and existing tests don't assert on the new fields.

- [ ] **Step 1: Verify no existing struct literals assert zero values on new field names**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && grep -rn "LocType{" --include="*.go" | grep -v "loctype.go"`
Expected: 9 hits in test files, all using partial struct literals (e.g., `&objtype.LocType{Op: []string{...}}`). Confirm none reference `Models`, `Shapes`, `Name`, `BlockWalk`, `BlockRange`, `Active`, `HillSkew`, `ShareLight`, `Occlude`, `Anim`, `HasAlpha`, `WallWidth`, `Ambient`, `Contrast`, `RecolS`, `RecolD`, `MapFunction`, `MapScene`, `Mirror`, `Shadow`, `ResizeX`, `ResizeY`, `ResizeZ`, `ForceApproach`, `OffsetX`, `OffsetY`, `OffsetZ`, `ForceDecor`. If any do, note for later (no action needed unless they assert specific zero values).

- [ ] **Step 2: Replace the `LocType` struct definition**

Replace `pkg/objtype/loctype.go:18-26`:

```go
// LocType mirrors Engine-TS/src/cache/config/LocType.ts. Loaded via a
// dual-pass decode: server/loc.dat contributes codes 61/249/250 (category,
// params, debugname), and the client jagfile entry loc.dat contributes
// the render+gameplay fields (codes 1-73). PostDecode infers Active from
// Shapes/Op when the cache leaves it unset.
//
// "hidden" → "" coercion in code 30-34 (NAI-80-D1) is preserved from S6k
// for handler-gate simplicity; see follow-up note in spec §6.
type LocType struct {
	ConfigType

	// Client-side render + gameplay fields (codes 1-73)
	Models        []uint16 // code 1, paired with Shapes
	Shapes        []uint8  // code 1, paired with Models
	Name          string   // code 2
	Desc          string   // code 3
	Width         int      // code 14, default 1
	Length        int      // code 15, default 1
	BlockWalk     bool     // code 17 sets false; default true
	BlockRange    bool     // code 18 sets false; default true
	Active        int      // code 19; default -1, PostDecode coerces to 0/1
	HillSkew      bool     // code 21
	ShareLight    bool     // code 22
	Occlude       bool     // code 23
	Anim          int      // code 24, 65535 → -1; default -1
	HasAlpha      bool     // code 25
	WallWidth     int      // code 28; default 16
	Ambient       int8     // code 29 (G1B)
	Contrast      int8     // code 39 (G1B)
	Op            []string // codes 30-34, lazy 5-slot init; "hidden"→"" (D1)
	RecolS        []uint16 // code 40, paired with RecolD
	RecolD        []uint16 // code 40
	MapFunction   int      // code 60; default -1
	Mirror        bool     // code 62
	Shadow        bool     // code 64 sets false; default true
	ResizeX       int      // code 65; default 128
	ResizeY       int      // code 66; default 128
	ResizeZ       int      // code 67; default 128
	MapScene      int      // code 68; default -1
	ForceApproach int      // code 69
	OffsetX       int16    // code 70 (G2S)
	OffsetY       int16    // code 71 (G2S)
	OffsetZ       int16    // code 72 (G2S)
	ForceDecor    bool     // code 73

	// Server-side fields
	Category int      // code 61
	Params   ParamMap // code 249
}
```

- [ ] **Step 3: Replace `NewLocType` to set TS-matching defaults**

Replace `pkg/objtype/loctype.go:62-70`:

```go
func NewLocType(id int) *LocType {
	return &LocType{
		ConfigType: ConfigType{ID: id},
		Width:      1,
		Length:     1,
		BlockWalk:  true,
		BlockRange: true,
		Active:     -1,
		Anim:       -1,
		WallWidth:  16,
		Shadow:     true,
		ResizeX:    128,
		ResizeY:    128,
		ResizeZ:    128,
		MapFunction: -1,
		MapScene:    -1,
		Category:    -1,
		Params:      make(ParamMap),
	}
}
```

- [ ] **Step 4: Verify package still compiles**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/objtype/...`
Expected: clean build.

- [ ] **Step 5: Verify existing tests still pass**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...`
Expected: all pass (existing tests don't assert on new fields).

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/loctype.go
git commit --no-gpg-sign -m "feat(objtype): NAI-80 T1 — LocType struct expansion to full TS shape

Adds all TS LocType render+gameplay fields with TS-matching defaults via
NewLocType. No decoder changes yet — Decode arms ship in T3 once dual-pass
sig change lands in T2. Existing tests continue passing because no fixture
asserts on the new fields.

Refs: docs/superpowers/specs/2026-05-03-nai-80-loctype-client-jagfile-pass-design.md"
```

---

## Task 2: Refactor test fixtures + parseLocTypes signature change + dual-pass loop

**Files:**
- Modify: `pkg/objtype/loctype.go:77-106` (`LoadLocTypes`, `parseLocTypes`)
- Modify: `pkg/objtype/loctype_test.go:10-217` (split `buildLocDat`, update all 5 call sites of `parseLocTypes`)

This is the atomic sig-change task. After this task, `parseLocTypes` takes `(server *packet.Packet, clientJag *jagfile.Jagfile)`. Existing tests are updated to provide a synthetic jagfile. No new behavior is introduced — the existing tests assert the same fields they always have, just routed through the new dual-pass shape.

**Pre-task pre-flight (controller-side, not implementer):** Confirm `genHash("loc.dat")` constant value. Computed: `682978269` (decimal) / `0x28B56BDD` (hex). Verified via Python mirror of `pkg/io/jagfile/jagfile.go:18-25` algorithm. Implementer uses this constant in Step 2.

- [ ] **Step 1: Update `LoadLocTypes` and `parseLocTypes` signatures and bodies**

Replace `pkg/objtype/loctype.go:77-106`:

```go
func LoadLocTypes(dir string) (*LocTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "loc.dat"), false)
	if err != nil {
		return nil, err
	}
	clientJag, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}
	return parseLocTypes(server, clientJag)
}

func parseLocTypes(server *packet2.Packet, clientJag *jagfile.Jagfile) (*LocTypeConfigs, error) {
	count := int(server.G2())

	client, err := clientJag.Read("loc.dat")
	if err != nil {
		return nil, fmt.Errorf("client/config loc.dat: %w", err)
	}
	client.Pos = 2 // skip the 2-byte count header on the client side

	configs := make([]*LocType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewLocType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, fmt.Errorf("loc id %d (server): %w", id, err)
		}
		if err := DecodeType(client, config); err != nil {
			return nil, fmt.Errorf("loc id %d (client): %w", id, err)
		}
		// PostDecode wired in T4; struct method does not yet exist.
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &LocTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
```

Note: `PostDecode` is wired in T4 once the method exists; for now the loop body matches `parseNPCTypes` minus that line.

- [ ] **Step 2: Add `jagfile` import to loctype.go**

Replace `pkg/objtype/loctype.go:1-8`:

```go
package objtype

import (
	"fmt"
	"path/filepath"

	jagfile "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)
```

- [ ] **Step 3: Refactor test fixtures — replace `buildLocDat` with split server/client helpers**

Replace `pkg/objtype/loctype_test.go:1-68` (imports + `locEntry` + `buildLocDat`):

```go
package objtype

import (
	"path/filepath"
	"testing"

	jag "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type locEntry struct {
	debugName string
	desc      string
	category  int
	width     int
	length    int
	intParams map[uint32]uint32
	op        []string // codes 30-34
}

// hashLocDat is genHash("loc.dat") — pre-computed via the algorithm in
// pkg/io/jagfile/jagfile.go:18-25 (uppercase + h*61+c-32 reduction).
const hashLocDat uint32 = 682978269

// buildLocServerDat assembles the server-side loc.dat blob:
//
//	u16 count
//	for each entry: codes 61 (category), 249 (params), 250 (debugname),
//	terminated by code 0.
func buildLocServerDat(entries []locEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.category != 0 {
			pkt.P1(61)
			pkt.P2(uint16(e.category))
		}
		if len(e.intParams) > 0 {
			pkt.P1(249)
			pkt.P1(uint8(len(e.intParams)))
			for k, v := range e.intParams {
				pkt.P3(k)
				pkt.PBool(false)
				pkt.P4(v)
			}
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// buildLocClientDat assembles the inner client-side loc.dat payload (the
// blob that lives inside client/config jagfile under entry name "loc.dat"):
//
//	u16 count
//	for each entry: codes 3 (desc), 14 (width), 15 (length), 30-34 (op),
//	250 (debugname), terminated by code 0.
func buildLocClientDat(entries []locEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.desc != "" {
			pkt.P1(3)
			pkt.PJStrLF(e.desc)
		}
		if e.width != 0 {
			pkt.P1(14)
			pkt.P1(uint8(e.width))
		}
		if e.length != 0 {
			pkt.P1(15)
			pkt.P1(uint8(e.length))
		}
		for i, name := range e.op {
			if name == "" {
				continue
			}
			pkt.P1(uint8(30 + i))
			pkt.PJStrLF(name)
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// buildClientJag wraps a single-entry jagfile around the given client
// loc.dat blob and returns a parsed *jag.Jagfile ready for parseLocTypes.
// Mirrors componenttype_test.go:751 buildMinimalJagfile pattern.
func buildClientJag(t *testing.T, locDatBytes []byte) *jag.Jagfile {
	t.Helper()
	compressed, err := jag.BZip2Compress(locDatBytes, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}
	p := packet2.NewPacket(nil)
	p.P3(1)                       // unpackedSize (== packedSize → Unpacked=false outer path)
	p.P3(1)                       // packedSize
	p.P2(1)                       // fileCount = 1
	p.P4(hashLocDat)              // file hash
	p.P3(uint32(len(locDatBytes)))// unpacked size
	p.P3(uint32(len(compressed))) // packed size
	p.Data = append(p.Data, compressed...)

	jf, err := jag.NewJagfile(packet2.NewPacket(p.Data))
	if err != nil {
		t.Fatalf("NewJagfile: %v", err)
	}
	return jf
}

// buildLocFixture is a convenience that builds both server bytes and
// client jagfile from a single entries list, ready for parseLocTypes.
func buildLocFixture(t *testing.T, entries []locEntry) (*packet2.Packet, *jag.Jagfile) {
	t.Helper()
	server := packet2.NewPacket(buildLocServerDat(entries))
	clientJag := buildClientJag(t, buildLocClientDat(entries))
	return server, clientJag
}
```

- [ ] **Step 4: Update existing test call sites to dual-pass**

In `pkg/objtype/loctype_test.go`, update the 5 `parseLocTypes` call sites:

`TestParseLocTypes` (was line ~85):

```go
func TestParseLocTypes(t *testing.T) {
	entries := []locEntry{
		{
			debugName: "door_basic",
			desc:      "A wooden door.",
			category:  17,
			width:     1,
			length:    2,
			intParams: map[uint32]uint32{1: 100},
		},
		{
			debugName: "bush",
		},
	}

	server, clientJag := buildLocFixture(t, entries)
	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("configs: got %d, want 2", len(cfgs.Configs))
	}

	door := cfgs.Configs[0]
	if door.DebugName != "door_basic" {
		t.Errorf("DebugName[0]: got %q", door.DebugName)
	}
	if door.Desc != "A wooden door." {
		t.Errorf("Desc[0]: got %q", door.Desc)
	}
	if door.Category != 17 {
		t.Errorf("Category[0]: got %d, want 17", door.Category)
	}
	if door.Width != 1 || door.Length != 2 {
		t.Errorf("Width/Length[0]: got %d/%d, want 1/2", door.Width, door.Length)
	}
	if got, _ := door.Params[1].(uint32); got != 100 {
		t.Errorf("Params[1]: got %v, want 100", door.Params[1])
	}

	bush := cfgs.Configs[1]
	if bush.Category != -1 {
		t.Errorf("Category default (bush): got %d, want -1", bush.Category)
	}
	if bush.Width != 1 || bush.Length != 1 {
		t.Errorf("Width/Length default (bush): got %d/%d, want 1/1", bush.Width, bush.Length)
	}

	if cfgs.ConfigNames["door_basic"] != 0 {
		t.Errorf("ConfigNames[door_basic]: got %d, want 0", cfgs.ConfigNames["door_basic"])
	}
}
```

`TestLocUnknownCode`: bogus code now goes in the client blob (server is left empty). Replace:

```go
func TestLocUnknownCode(t *testing.T) {
	server := packet2.NewPacket(nil)
	server.P2(1) // count = 1
	server.P1(0) // immediate terminator on server side

	clientInner := packet2.NewPacket(nil)
	clientInner.P2(1) // count = 1
	clientInner.P1(200) // bogus code in client blob
	clientInner.P1(0)

	clientJag := buildClientJag(t, clientInner.Bytes())

	_, err := parseLocTypes(packet2.NewPacket(server.Bytes()), clientJag)
	if err == nil {
		t.Fatal("expected error on unknown loc code, got nil")
	}
}
```

`TestLocTypeDecodeOpSingleEntry`:

```go
func TestLocTypeDecodeOpSingleEntry(t *testing.T) {
	entries := []locEntry{
		{debugName: "tree", op: []string{"Chop", "", "", "", ""}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if got := len(cfgs.Configs); got != 1 {
		t.Fatalf("Configs len: got %d, want 1", got)
	}

	tree := cfgs.Configs[0]
	if tree.Op == nil {
		t.Fatal("Op: got nil, want 5-slot slice")
	}
	if got := tree.Op[0]; got != "Chop" {
		t.Errorf("Op[0]: got %q, want \"Chop\"", got)
	}
	for i := 1; i < 5; i++ {
		if tree.Op[i] != "" {
			t.Errorf("Op[%d]: got %q, want \"\"", i, tree.Op[i])
		}
	}
}
```

`TestLocTypeDecodeOpAllFive`:

```go
func TestLocTypeDecodeOpAllFive(t *testing.T) {
	entries := []locEntry{
		{debugName: "multi", op: []string{"op0", "op1", "op2", "op3", "op4"}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	multi := cfgs.Configs[0]
	want := []string{"op0", "op1", "op2", "op3", "op4"}
	for i, w := range want {
		if got := multi.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}
}
```

`TestLocTypeDecodeOpHiddenCoercedToEmpty` (rename to keep existing name; D1 pin):

```go
func TestLocTypeDecodeOpHiddenCoercedToEmpty(t *testing.T) {
	entries := []locEntry{
		{debugName: "hidden_test", op: []string{"visible", "hidden", "", "", ""}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	entry := cfgs.Configs[0]
	if got := entry.Op[0]; got != "visible" {
		t.Errorf("Op[0]: got %q, want \"visible\"", got)
	}
	if got := entry.Op[1]; got != "" {
		t.Errorf("Op[1] (hidden-coerced, NAI-80-D1): got %q, want \"\"", got)
	}
}
```

`TestLoadRealLocCache` (existing) stays unchanged — it calls `LoadLocTypes(dir)` which still has the same signature.

- [ ] **Step 5: Run tests to verify refactor preserves behavior**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -v -run "TestParseLocTypes|TestLocUnknownCode|TestLocTypeDecodeOp|TestLoadRealLocCache"`
Expected: all 6 tests pass. `TestLoadRealLocCache` will hit the new client-jagfile path and load real `data/pack/client/config`. If this test starts failing at `LoadLocTypes` with an unknown code error, that's a real signal that the cache contains a code we haven't added an arm for yet — but T3 is where we add the arms, so this test may genuinely fail until T3 lands. **Expected interim state**: `TestLoadRealLocCache` returns an error from `parseLocTypes` like `loc id N (client): unrecognized loc config code M` for some M ∈ {1, 2, 17, ...}. That's fine — `t.Skipf` on err means this test soft-skips. Re-verify after T3.

If the existing 5 unit tests fail, the refactor is wrong; fix before commit.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/loctype.go pkg/objtype/loctype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-80 T2 — parseLocTypes dual-pass sig change

Splits buildLocDat helper into server/client variants matching the
real-cache file layout. Updates all 5 unit-test call sites to pass a
synthetic jagfile alongside server bytes. parseLocTypes now reads the
client jagfile entry loc.dat after the server blob, mirroring
parseNPCTypes (npctype.go:367-403). PostDecode wiring lands in T4 once
the method exists. Decode arms for client-blob codes (1-73 minus the
existing handful) ship in T3.

LoadLocTypes signature unchanged: (dir string) → (*LocTypeConfigs, error);
single production caller modules/world/server.go:228 needs no update.

Refs: spec §3.5"
```

---

## Task 3: Add Decode case arms for all client-side codes

**Files:**
- Modify: `pkg/objtype/loctype.go:28-60` (extend `Decode` switch)
- Modify: `pkg/objtype/loctype_test.go` (append per-code-arm tests)

This task TDDs the new code arms. Each arm gets a sub-test that builds a 1-loc client blob containing only that code's bytes, parses, asserts the corresponding struct field. Group sub-tests under `TestLocTypeDecodeNewArms` for one bulk go-test invocation.

- [ ] **Step 1: Write the failing per-code-arm test**

Append to `pkg/objtype/loctype_test.go`:

```go
// buildClientBlobRaw assembles a 1-entry client loc.dat with the given
// raw code-payload bytes inserted between the count header and the 0
// terminator. Used by per-arm decode tests in TestLocTypeDecodeNewArms.
func buildClientBlobRaw(payload []byte) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(1) // count = 1
	pkt.Data = append(pkt.Data, payload...)
	pkt.P1(0) // terminator
	return pkt.Bytes()
}

// withMinimalServer pairs a 1-entry server blob (no codes, just terminator)
// with the given client jagfile, returning both ready for parseLocTypes.
func withMinimalServer(t *testing.T, clientJag *jag.Jagfile) (*packet2.Packet, *jag.Jagfile) {
	t.Helper()
	srv := packet2.NewPacket(nil)
	srv.P2(1) // count = 1
	srv.P1(0) // terminator
	return packet2.NewPacket(srv.Bytes()), clientJag
}

func TestLocTypeDecodeNewArms(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		assert  func(t *testing.T, lt *LocType)
	}{
		{
			name: "code1_models_shapes_pair",
			payload: func() []byte {
				p := packet2.NewPacket(nil)
				p.P1(1)
				p.P1(2)         // count = 2
				p.P2(0x1111)    // models[0]
				p.P1(10)        // shapes[0]
				p.P2(0x2222)    // models[1]
				p.P1(11)        // shapes[1]
				return p.Bytes()
			}(),
			assert: func(t *testing.T, lt *LocType) {
				if len(lt.Models) != 2 || lt.Models[0] != 0x1111 || lt.Models[1] != 0x2222 {
					t.Errorf("Models: got %v, want [0x1111 0x2222]", lt.Models)
				}
				if len(lt.Shapes) != 2 || lt.Shapes[0] != 10 || lt.Shapes[1] != 11 {
					t.Errorf("Shapes: got %v, want [10 11]", lt.Shapes)
				}
			},
		},
		{
			name: "code2_name",
			payload: func() []byte {
				p := packet2.NewPacket(nil)
				p.P1(2)
				p.PJStrLF("oak tree")
				return p.Bytes()
			}(),
			assert: func(t *testing.T, lt *LocType) {
				if lt.Name != "oak tree" {
					t.Errorf("Name: got %q, want \"oak tree\"", lt.Name)
				}
			},
		},
		{
			name: "code17_blockwalk_false",
			payload: []byte{17},
			assert: func(t *testing.T, lt *LocType) {
				if lt.BlockWalk {
					t.Errorf("BlockWalk: got true, want false")
				}
			},
		},
		{
			name: "code18_blockrange_false",
			payload: []byte{18},
			assert: func(t *testing.T, lt *LocType) {
				if lt.BlockRange {
					t.Errorf("BlockRange: got true, want false")
				}
			},
		},
		{
			name: "code19_active_g1",
			payload: []byte{19, 7},
			assert: func(t *testing.T, lt *LocType) {
				if lt.Active != 7 {
					t.Errorf("Active: got %d, want 7", lt.Active)
				}
			},
		},
		{
			name: "code21_hillskew_true",
			payload: []byte{21},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.HillSkew {
					t.Errorf("HillSkew: got false, want true")
				}
			},
		},
		{
			name: "code22_sharelight_true",
			payload: []byte{22},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.ShareLight {
					t.Errorf("ShareLight: got false, want true")
				}
			},
		},
		{
			name: "code23_occlude_true",
			payload: []byte{23},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.Occlude {
					t.Errorf("Occlude: got false, want true")
				}
			},
		},
		{
			name: "code24_anim_g2_normal",
			payload: []byte{24, 0x12, 0x34}, // 0x1234 = 4660
			assert: func(t *testing.T, lt *LocType) {
				if lt.Anim != 4660 {
					t.Errorf("Anim: got %d, want 4660", lt.Anim)
				}
			},
		},
		{
			name: "code24_anim_65535_to_neg1",
			payload: []byte{24, 0xFF, 0xFF},
			assert: func(t *testing.T, lt *LocType) {
				if lt.Anim != -1 {
					t.Errorf("Anim: got %d, want -1 (65535 → -1)", lt.Anim)
				}
			},
		},
		{
			name: "code25_hasalpha_true",
			payload: []byte{25},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.HasAlpha {
					t.Errorf("HasAlpha: got false, want true")
				}
			},
		},
		{
			name: "code28_wallwidth_g1",
			payload: []byte{28, 32},
			assert: func(t *testing.T, lt *LocType) {
				if lt.WallWidth != 32 {
					t.Errorf("WallWidth: got %d, want 32", lt.WallWidth)
				}
			},
		},
		{
			name: "code29_ambient_g1b_negative",
			payload: []byte{29, 0xFF}, // -1 signed
			assert: func(t *testing.T, lt *LocType) {
				if lt.Ambient != -1 {
					t.Errorf("Ambient: got %d, want -1", lt.Ambient)
				}
			},
		},
		{
			name: "code39_contrast_g1b_negative",
			payload: []byte{39, 0xFE}, // -2 signed
			assert: func(t *testing.T, lt *LocType) {
				if lt.Contrast != -2 {
					t.Errorf("Contrast: got %d, want -2", lt.Contrast)
				}
			},
		},
		{
			name: "code40_recol_pair",
			payload: func() []byte {
				p := packet2.NewPacket(nil)
				p.P1(40)
				p.P1(2) // count = 2
				p.P2(0xAAAA)
				p.P2(0xBBBB)
				p.P2(0xCCCC)
				p.P2(0xDDDD)
				return p.Bytes()
			}(),
			assert: func(t *testing.T, lt *LocType) {
				if len(lt.RecolS) != 2 || lt.RecolS[0] != 0xAAAA || lt.RecolS[1] != 0xCCCC {
					t.Errorf("RecolS: got %v", lt.RecolS)
				}
				if len(lt.RecolD) != 2 || lt.RecolD[0] != 0xBBBB || lt.RecolD[1] != 0xDDDD {
					t.Errorf("RecolD: got %v", lt.RecolD)
				}
			},
		},
		{
			name: "code60_mapfunction_g2",
			payload: []byte{60, 0x01, 0x23},
			assert: func(t *testing.T, lt *LocType) {
				if lt.MapFunction != 0x0123 {
					t.Errorf("MapFunction: got %d, want 0x0123", lt.MapFunction)
				}
			},
		},
		{
			name: "code62_mirror_true",
			payload: []byte{62},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.Mirror {
					t.Errorf("Mirror: got false, want true")
				}
			},
		},
		{
			name: "code64_shadow_false",
			payload: []byte{64},
			assert: func(t *testing.T, lt *LocType) {
				if lt.Shadow {
					t.Errorf("Shadow: got true, want false")
				}
			},
		},
		{
			name: "code65_resizex_g2",
			payload: []byte{65, 0x00, 0x40}, // 64
			assert: func(t *testing.T, lt *LocType) {
				if lt.ResizeX != 64 {
					t.Errorf("ResizeX: got %d, want 64", lt.ResizeX)
				}
			},
		},
		{
			name: "code66_resizey_g2",
			payload: []byte{66, 0x00, 0x50}, // 80
			assert: func(t *testing.T, lt *LocType) {
				if lt.ResizeY != 80 {
					t.Errorf("ResizeY: got %d, want 80", lt.ResizeY)
				}
			},
		},
		{
			name: "code67_resizez_g2",
			payload: []byte{67, 0x00, 0x60}, // 96
			assert: func(t *testing.T, lt *LocType) {
				if lt.ResizeZ != 96 {
					t.Errorf("ResizeZ: got %d, want 96", lt.ResizeZ)
				}
			},
		},
		{
			name: "code68_mapscene_g2",
			payload: []byte{68, 0x04, 0x56},
			assert: func(t *testing.T, lt *LocType) {
				if lt.MapScene != 0x0456 {
					t.Errorf("MapScene: got %d, want 0x0456", lt.MapScene)
				}
			},
		},
		{
			name: "code69_forceapproach_g1",
			payload: []byte{69, 3},
			assert: func(t *testing.T, lt *LocType) {
				if lt.ForceApproach != 3 {
					t.Errorf("ForceApproach: got %d, want 3", lt.ForceApproach)
				}
			},
		},
		{
			name: "code70_offsetx_g2s_negative",
			payload: []byte{70, 0xFF, 0xFE}, // -2 as int16
			assert: func(t *testing.T, lt *LocType) {
				if lt.OffsetX != -2 {
					t.Errorf("OffsetX: got %d, want -2", lt.OffsetX)
				}
			},
		},
		{
			name: "code71_offsety_g2s_negative",
			payload: []byte{71, 0xFF, 0xFD}, // -3
			assert: func(t *testing.T, lt *LocType) {
				if lt.OffsetY != -3 {
					t.Errorf("OffsetY: got %d, want -3", lt.OffsetY)
				}
			},
		},
		{
			name: "code72_offsetz_g2s_positive",
			payload: []byte{72, 0x00, 0x05},
			assert: func(t *testing.T, lt *LocType) {
				if lt.OffsetZ != 5 {
					t.Errorf("OffsetZ: got %d, want 5", lt.OffsetZ)
				}
			},
		},
		{
			name: "code73_forcedecor_true",
			payload: []byte{73},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.ForceDecor {
					t.Errorf("ForceDecor: got false, want true")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clientJag := buildClientJag(t, buildClientBlobRaw(tc.payload))
			server, clientJag := withMinimalServer(t, clientJag)
			cfgs, err := parseLocTypes(server, clientJag)
			if err != nil {
				t.Fatalf("parseLocTypes: %v", err)
			}
			if len(cfgs.Configs) != 1 {
				t.Fatalf("Configs len: got %d, want 1", len(cfgs.Configs))
			}
			tc.assert(t, cfgs.Configs[0])
		})
	}
}
```

- [ ] **Step 2: Run tests — they should all fail with "unrecognized loc config code N"**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLocTypeDecodeNewArms -v`
Expected: every sub-case fails at `parseLocTypes` with `loc id 0 (client): unrecognized loc config code N` for the corresponding code.

- [ ] **Step 3: Add the new Decode arms**

Replace `pkg/objtype/loctype.go:28-60` (the entire `LocType.Decode` method):

```go
func (lt *LocType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		count := int(dat.G1())
		lt.Models = make([]uint16, count)
		lt.Shapes = make([]uint8, count)
		for i := range count {
			lt.Models[i] = dat.G2()
			lt.Shapes[i] = dat.G1()
		}
	case 2:
		lt.Name = dat.GJStrLF()
	case 3:
		lt.Desc = dat.GJStrLF()
	case 14:
		lt.Width = int(dat.G1())
	case 15:
		lt.Length = int(dat.G1())
	case 17:
		lt.BlockWalk = false
	case 18:
		lt.BlockRange = false
	case 19:
		lt.Active = int(dat.G1())
	case 21:
		lt.HillSkew = true
	case 22:
		lt.ShareLight = true
	case 23:
		lt.Occlude = true
	case 24:
		lt.Anim = int(dat.G2())
		if lt.Anim == 65535 {
			lt.Anim = -1
		}
	case 25:
		lt.HasAlpha = true
	case 28:
		lt.WallWidth = int(dat.G1())
	case 29:
		lt.Ambient = dat.G1B()
	case 30, 31, 32, 33, 34:
		// Op-name slots. Lazy 5-slot init mirrors NpcType.Op
		// (npctype.go:124-132). TS LocType.ts:152-157 uses
		// `code >= 30 && < 35`. The "hidden" keyword in the cache
		// marks a disabled op slot; we coerce to "" here so the
		// handler gate in modules/world/handler_oploc.go can do a
		// single empty-string check at runtime (NAI-80-D1).
		if lt.Op == nil {
			lt.Op = make([]string, 5)
		}
		lt.Op[code-30] = dat.GJStrLF()
		if lt.Op[code-30] == "hidden" {
			lt.Op[code-30] = ""
		}
	case 39:
		lt.Contrast = dat.G1B()
	case 40:
		count := int(dat.G1())
		lt.RecolS = make([]uint16, count)
		lt.RecolD = make([]uint16, count)
		for i := range count {
			lt.RecolS[i] = dat.G2()
			lt.RecolD[i] = dat.G2()
		}
	case 60:
		lt.MapFunction = int(dat.G2())
	case 61:
		lt.Category = int(dat.G2())
	case 62:
		lt.Mirror = true
	case 64:
		lt.Shadow = false
	case 65:
		lt.ResizeX = int(dat.G2())
	case 66:
		lt.ResizeY = int(dat.G2())
	case 67:
		lt.ResizeZ = int(dat.G2())
	case 68:
		lt.MapScene = int(dat.G2())
	case 69:
		lt.ForceApproach = int(dat.G1())
	case 70:
		lt.OffsetX = dat.G2S()
	case 71:
		lt.OffsetY = dat.G2S()
	case 72:
		lt.OffsetZ = dat.G2S()
	case 73:
		lt.ForceDecor = true
	case 249:
		lt.Params = DecodeParams(dat)
	case 250:
		lt.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized loc config code %d", code)
	}
	return nil
}
```

- [ ] **Step 4: Run new-arm tests; expect all pass**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLocTypeDecodeNewArms -v`
Expected: all sub-cases pass.

- [ ] **Step 5: Run full package test to verify no regressions**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...`
Expected: all pass. `TestLoadRealLocCache` should now succeed against real cache (or `t.Skipf` if cache missing).

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/loctype.go pkg/objtype/loctype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-80 T3 — LocType.Decode arms for client-blob codes

Adds 25 new code arms (1, 2, 17, 18, 19, 21, 22, 23, 24, 25, 28, 29, 39,
40, 60, 62, 64-73) mirroring TS LocType.ts:108-199 line-by-line. The
existing 30-34 Op block including the 'hidden'→'' D1 coercion is
preserved verbatim, just regrouped within the new switch.

Per-arm TestLocTypeDecodeNewArms covers each code in isolation:
boolean-set arms, signed-byte (G1B) arms, signed-2byte (G2S) arms,
paired-array arms (codes 1 & 40), and the 65535→-1 Anim coercion.

Refs: spec §3.3 + §7.1"
```

---

## Task 4: Add `PostDecode` method + wire into `parseLocTypes`

**Files:**
- Modify: `pkg/objtype/loctype.go` (add `PostDecode` method)
- Modify: `pkg/objtype/loctype.go::parseLocTypes` (call `config.PostDecode()` in loop)
- Modify: `pkg/objtype/loctype_test.go` (add `TestPostDecode_ActiveInference`)

- [ ] **Step 1: Write the failing PostDecode test**

Append to `pkg/objtype/loctype_test.go`:

```go
func TestPostDecode_ActiveInference(t *testing.T) {
	t.Run("op_nonnil_sets_active_1", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Op = []string{"Open", "", "", "", ""}
		lt.PostDecode()
		if lt.Active != 1 {
			t.Errorf("Active: got %d, want 1 (Op != nil branch)", lt.Active)
		}
	})

	t.Run("shapes_single_10_sets_active_1", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Shapes = []uint8{10}
		lt.PostDecode()
		if lt.Active != 1 {
			t.Errorf("Active: got %d, want 1 (Shapes==[10] branch)", lt.Active)
		}
	})

	t.Run("neither_sets_active_0", func(t *testing.T) {
		lt := NewLocType(0)
		lt.PostDecode()
		if lt.Active != 0 {
			t.Errorf("Active: got %d, want 0 (default fallthrough)", lt.Active)
		}
	})

	t.Run("active_already_set_unchanged", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Active = 5
		lt.Op = []string{"Open", "", "", "", ""}
		lt.PostDecode()
		if lt.Active != 5 {
			t.Errorf("Active: got %d, want 5 (already-set guard)", lt.Active)
		}
	})

	t.Run("shapes_multi_no_active_inference", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Shapes = []uint8{10, 11}
		lt.PostDecode()
		if lt.Active != 0 {
			t.Errorf("Active: got %d, want 0 (Shapes len != 1)", lt.Active)
		}
	})

	t.Run("shapes_single_non10_no_active_inference", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Shapes = []uint8{5}
		lt.PostDecode()
		if lt.Active != 0 {
			t.Errorf("Active: got %d, want 0 (Shapes[0] != 10)", lt.Active)
		}
	})
}
```

- [ ] **Step 2: Run test — expect compile error (PostDecode undefined)**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestPostDecode -v`
Expected: compile error `lt.PostDecode undefined`.

- [ ] **Step 3: Add `PostDecode` method**

Append to `pkg/objtype/loctype.go` (after the `Decode` method, before `NewLocType`):

```go
// PostDecode mirrors TS LocType.postDecode (LocType.ts:202-214).
// Coerces the Active default (-1) to 0/1 based on Shapes/Op presence.
// Called after both server and client decode passes complete in
// parseLocTypes.
func (lt *LocType) PostDecode() {
	if lt.Active == -1 {
		lt.Active = 0
		if len(lt.Shapes) == 1 && lt.Shapes[0] == 10 {
			lt.Active = 1
		}
		if lt.Op != nil {
			lt.Active = 1
		}
	}
}
```

- [ ] **Step 4: Wire `PostDecode` into `parseLocTypes` loop**

In `pkg/objtype/loctype.go::parseLocTypes`, replace the existing per-id loop body's interior:

Find:
```go
		if err := DecodeType(client, config); err != nil {
			return nil, fmt.Errorf("loc id %d (client): %w", id, err)
		}
		// PostDecode wired in T4; struct method does not yet exist.
		configs[id] = config
```

Replace with:
```go
		if err := DecodeType(client, config); err != nil {
			return nil, fmt.Errorf("loc id %d (client): %w", id, err)
		}
		config.PostDecode()
		configs[id] = config
```

- [ ] **Step 5: Run PostDecode tests; expect pass**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestPostDecode -v`
Expected: all 6 sub-cases pass.

- [ ] **Step 6: Run full package; verify no regressions**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add pkg/objtype/loctype.go pkg/objtype/loctype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-80 T4 — LocType.PostDecode + parseLocTypes wiring

Mirrors TS postDecode (LocType.ts:202-214): infers Active from Shapes/Op
when cache leaves it -1. Wired into parseLocTypes per-id loop after
both decode passes complete, matching parseNPCTypes shape.

TestPostDecode_ActiveInference covers all 4 input shapes plus 2 negative
fallthrough cases (Shapes len > 1, Shapes[0] != 10).

Refs: spec §3.4"
```

---

## Task 5: Real-cache cascade-blocker regression test

**Files:**
- Create: `pkg/objtype/loctype_realcache_test.go`

This test pins the actual NAI-80 fix: loc 3014/380/350 must have non-empty `Op[0]` after `LoadLocTypes` against the real `data/pack` cache. Soft-skips if the cache is absent (matching the existing `TestLoadRealLocCache` precedent).

Plus an ID-shift sanity probe: load by `ConfigNames["..."]` and assert a tree-shaped loc has `Op[0]=="Chop"`. The exact debugname depends on the cache contents — implementer probes by loading the cache once, listing the first ~5 known-tree IDs, and pinning whichever debugname exists with `Op[0]=="Chop"`. This guards against silent ID-shift regressions where everything decodes but indexes shift.

- [ ] **Step 1: Write the cascade-blocker regression**

Create `pkg/objtype/loctype_realcache_test.go`:

```go
package objtype

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadLocTypes_RealCache_CascadeBlockerLocs pins the NAI-80 fix:
// after the client-jagfile pass is wired, loc ids 3014 (RS Guide door),
// 380 (bookcase), and 350 (drawer) must all have non-empty Op[0].
//
// NAI-79 H4 re-smoke at HEAD 604cdb7 captured 3/3 OPLOC1 clicks gating
// at op_slot_empty for these locs because the goscape decoder never
// loaded client/config's loc.dat entry. This test is the regression
// guard against re-introducing that gap.
func TestLoadLocTypes_RealCache_CascadeBlockerLocs(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); errors.Is(err, fs.ErrNotExist) {
		t.Skip("data/pack/server/loc.dat absent; skipping real-cache regression")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "config")); errors.Is(err, fs.ErrNotExist) {
		t.Skip("data/pack/client/config absent; skipping real-cache regression")
	}

	cfgs, err := LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}

	for _, tc := range []struct {
		id   int
		name string
	}{
		{3014, "RS Guide door"},
		{380, "bookcase"},
		{350, "drawer"},
	} {
		if tc.id >= len(cfgs.Configs) {
			t.Errorf("loc %d (%s): id out of range (configs len=%d)", tc.id, tc.name, len(cfgs.Configs))
			continue
		}
		cfg := cfgs.Configs[tc.id]
		if cfg == nil {
			t.Errorf("loc %d (%s): nil config", tc.id, tc.name)
			continue
		}
		if cfg.Op == nil || len(cfg.Op) < 1 || cfg.Op[0] == "" {
			t.Errorf("loc %d (%s): expected Op[0] non-empty (NAI-80 cascade-blocker pin); got DebugName=%q Op=%v",
				tc.id, tc.name, cfg.DebugName, cfg.Op)
		}
	}
}
```

- [ ] **Step 2: Run the regression test; expect pass against real cache**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLoadLocTypes_RealCache_CascadeBlockerLocs -v`
Expected: PASS. If cache is absent, test soft-skips. If any of the 3 locs still has empty `Op[0]`, real cause is something other than the missing client-jag pass — investigate before commit.

- [ ] **Step 3: Add ID-shift sanity probe (interactive — implementer determines probe value)**

Implementer probes the cache by adding a temporary t.Log inside the test:

```go
// (temporary) print Op[0] for first 5 ConfigNames containing "tree" or "door"
for name, id := range cfgs.ConfigNames {
    if cfg := cfgs.Configs[id]; cfg.Op != nil && cfg.Op[0] != "" {
        t.Logf("DEBUGPROBE id=%d name=%q op0=%q", id, name, cfg.Op[0])
    }
}
```

Run with `-v` to see output. Pick one well-known loc (preferably a tree with `Op[0]=="Chop"`, or the RS Guide door with `Op[0]=="Open"` etc.) and add a sanity assertion to the test. Then remove the debug probe.

Append to `TestLoadLocTypes_RealCache_CascadeBlockerLocs` (after the cascade-blocker loop):

```go
	// ID-shift sanity probe: a known-name loc should resolve to its expected Op[0].
	// Implementer-derived probe — see commit body for which loc was pinned.
	if id, ok := cfgs.ConfigNames["<DEBUGNAME>"]; ok {
		cfg := cfgs.Configs[id]
		if cfg.Op == nil || cfg.Op[0] != "<EXPECTED_OP0>" {
			t.Errorf("ID-shift probe: ConfigNames[%q]=%d, Op[0]=%q, want %q",
				"<DEBUGNAME>", id, cfg.Op[0], "<EXPECTED_OP0>")
		}
	} else {
		t.Logf("ID-shift probe: ConfigNames[%q] not found — skipping", "<DEBUGNAME>")
	}
```

Replace `<DEBUGNAME>` and `<EXPECTED_OP0>` with the values discovered in the probe step. If the probe surfaces no obvious well-known name (e.g., cache uses generic debugnames), pick whatever loc with non-empty `Op[0]` has the most stable-looking name and pin it.

- [ ] **Step 4: Run final test; expect pass**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLoadLocTypes_RealCache_CascadeBlockerLocs -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/loctype_realcache_test.go
git commit --no-gpg-sign -m "test(objtype): NAI-80 T5 — real-cache cascade-blocker regression

Pins loc 3014/380/350 Op[0] non-empty after LoadLocTypes against
data/pack. Soft-skips if cache absent. Adds an ID-shift sanity probe
on a known-name loc (<DEBUGNAME>=<EXPECTED_OP0>, derived during
plan execution) to catch silent index drift.

Pre-NAI-80, all 3 locs had Op=nil and OPLOC1 clicks gated at
op_slot_empty (NAI-79 H4 re-smoke evidence at HEAD 604cdb7).

Refs: spec §7.2"
```

---

## Task 6: Top-of-file comment update (verification fixup)

**Files:**
- Modify: `pkg/objtype/loctype.go` (top-of-file doc block)

The struct expansion in T1 already replaced the doc comment as part of the struct edit. T6 is a sanity check that the comment matches the spec §3.6 and that no stale "this server-only loader skips" wording remains anywhere in the file.

- [ ] **Step 1: Grep for stale "server-only" wording**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && grep -n "server-only\|skips" pkg/objtype/loctype.go`
Expected: no hits. If T1 left any stale comment, fix here.

- [ ] **Step 2: Verify the comment matches spec §3.6**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && head -30 pkg/objtype/loctype.go`
Expected: top comment block reads as in T1 Step 2 (mirrors Engine-TS reference + dual-pass description + D1 follow-up note). If wording diverges from the spec, edit to match.

- [ ] **Step 3: If any edit was needed, commit; else skip**

If grep found stale wording or comment was edited:

```bash
git add pkg/objtype/loctype.go
git commit --no-gpg-sign -m "polish(objtype): NAI-80 T6 — retire stale loctype.go server-only comment"
```

If nothing to fix, this task is a no-op — skip the commit and proceed to T7.

---

## Task 7: Full verification + close commit

**Files:** none (verification-only)

- [ ] **Step 1: Full unit test suite**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all packages pass. Pay particular attention to `pkg/script/...`, `modules/world/...` (downstream LocType consumers) — no regressions.

If any test fails, attribute carefully: `verify_implementer_claims.md` says re-run at HEAD~N to distinguish pre-existing failures from NAI-80-introduced ones. Run `git stash && go test ./failing-pkg/... && git stash pop` if needed.

- [ ] **Step 2: Full build**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Expected: clean build, no errors.

- [ ] **Step 3: Race detector**

Run: `cd /home/owner/Code/github.com/zsrv/goscape && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/objtype/...`
Expected: pass with race detector clean.

- [ ] **Step 4: Close commit (docs-only — plan close + smoke handoff)**

Append a closing section to `docs/superpowers/plans/2026-05-03-nai-80-loctype-client-jagfile-pass.md` documenting the close-out and smoke handoff:

```markdown
---

# NAI-80 — Close — full-port complete + smoke handoff

**Implementation HEAD:** `<close-SHA>` (substitute the most recent commit's SHA after T5).

**Acceptance gates met:**
- [x] §10.1 — `go test ./pkg/objtype/...` green (T7 Step 1)
- [x] §10.2 — `TestLoadLocTypes_RealCache_CascadeBlockerLocs` passes against `data/pack` (T5 Step 2)
- [x] §10.3 — `go test ./...` + `go build ./...` clean (T7 Steps 1+2)
- [x] §10.4 — top-of-file comment retired (T6)
- [x] §10.5 — smoke handoff issued (this section)
- [x] §10.6 — Closes memory trailer (final close commit)

**Smoke handoff (user-driven):**
1. Restart goscape world server at this commit.
2. Java client login as Tutorial Island fresh char (avoid coord-based chat-suppression zones per `java_client_coord_chat_suppression.md`).
3. Repeat the 3 OPLOC1 clicks captured in NAI-79 H4 re-smoke:
   - RS Guide door (loc 3014)
   - bookcase (loc 380)
   - drawer (loc 350)
4. Capture session-log `oploc gate` records for each click.

**Expected gate signal at fix:** `gate=script_dispatch` (or no gate record at all → full dispatch success). Walking-on-click should resume for all 3 locs.

**Routing if smoke does NOT advance gate:**
- All 3 still at `op_slot_empty` → real-cache regression should have caught this; bin-decode the cache for loc 3014 and audit.
- Different gate fires (e.g., `getloc_nil`, `viewport`, `delayed`) → second blocker exposed; route to NAI-81 brainstorm with new gate name as routing target (per `smoke_unchanged_means_multiple_blockers.md`).
- Gate=`script_dispatch` but no walking → SetInteraction reaches but pathing/visual blocked downstream; route to a movement-side investigation.
```

Then commit:

```bash
git add docs/superpowers/plans/2026-05-03-nai-80-loctype-client-jagfile-pass.md
git commit --no-gpg-sign -m "docs(plan): NAI-80 close — full TS-shape port + smoke handoff

All §10 acceptance gates met:
  ✓ Unit tests green (per-arm + PostDecode triple + dual-pass)
  ✓ Real-cache cascade-blocker regression pins loc 3014/380/350 Op[0]
  ✓ go test ./... + go build ./... clean
  ✓ Top-of-file loctype.go comment retired
  ✓ Smoke handoff section published

Smoke pass criterion: gate=script_dispatch (or absent) for all 3 locs.
Cascade routing: smoke_unchanged_means_multiple_blockers.md applies if
gate signal does not advance.

Closes memory: nai80_seed_loctype_op_empty.md"
```

---

## Self-review

**Spec coverage map:**
- §1 Root cause — informational; no task needed.
- §2 Scope — covered by T1 (struct), T3 (decode), T4 (postdecode), T5 (real-cache test), T6 (comment).
- §3.1 Components — T1, T2, T3, T4, T5, T6.
- §3.2 Struct shape — T1.
- §3.3 Decode arms — T3.
- §3.4 PostDecode — T4.
- §3.5 Dual-pass call sites — T2.
- §3.6 Comment — T1 inline + T6 verification.
- §4 Data flow — implemented across T1+T2+T3+T4.
- §5 Error handling — covered by T2's `parseLocTypes` body + existing `default:` arm in `Decode`.
- §6 NAI-80-D1 deviation — preserved verbatim in T3 Step 3; pinned by `TestLocTypeDecodeOpHiddenCoercedToEmpty` (kept from prior tests, now dual-pass routed in T2).
- §7.1 Unit tests — T2 (refactored existing), T3 (per-arm), T4 (PostDecode).
- §7.2 Real-cache regression — T5.
- §7.3 Smoke handoff — T7 Step 4.
- §8 Build sequence — this plan IS the build sequence.
- §9 Risk register — risks mitigated by per-arm tests (T3), pre-flight grep (T1 Step 1), real-cache test (T5).
- §10 Acceptance criteria — verified across T7.
- §11 Out-of-scope follow-ups — informational; no task needed.

No spec section uncovered.

**Placeholder scan:**
- T5 Step 3 has explicit `<DEBUGNAME>` and `<EXPECTED_OP0>` placeholders — these are intentional probe-derived values, not "TBD". The step instructs the implementer how to discover them. Acceptable.
- T7 Step 4 has `<close-SHA>` — also intentional, derived at close time.
- No "TODO", "implement later", "appropriate error handling", or unspecified test code.

**Type consistency:**
- `parseLocTypes(server *packet2.Packet, clientJag *jagfile.Jagfile)` — used consistently in T2, T3, T4.
- `LocType.Op` is `[]string` — used consistently in T2 D1 test, T3 op-decode, T4 PostDecode test.
- `LocType.Active` is `int` (default `-1`) — used consistently in T1 NewLocType, T3 code 19 arm, T4 PostDecode.
- `jag` test alias for `pkg/io/jagfile` package — used consistently in T2 test imports.
- `jagfile` production alias — used consistently in T2 production imports (avoids name collision with the import path's leaf).

No type drift across tasks.

---

# NAI-80 — Close — full-port complete + smoke handoff

**Implementation HEAD:** `00e9eb0` (most recent commit at close).

**Commit chain:**
- `c1de23a` T1 — struct expansion
- `49b2f7b` T2 — parseLocTypes dual-pass sig change
- `b0cf375` T3 — Decode arms for client-blob codes
- `1ee7ea4` T4 — PostDecode + parseLocTypes wiring
- `05365fb` T4 fixup — pin parseLocTypes→PostDecode wiring
- `ec4a4b7` T5 — real-cache cascade-blocker regression
- `7e9dd56` T5 fixup — drop redundant nil-check before len()
- `00e9eb0` T5 fixup — defensive Op[0] read + skip-pattern parity

**Acceptance gates met:**
- [x] §10.1 — `go test ./pkg/objtype/...` green (T7 Step 1).
- [x] §10.2 — `TestLoadLocTypes_RealCache_CascadeBlockerLocs` passes against `data/pack` (T5 + cascade-blocker `Op[0]` discovered: 3014→"Open", 380→"Search", 350→"Open"; ID-shift probe `oaktree`→"Chop down").
- [x] §10.3 — `go test ./...` + `go build ./...` clean (T7 Steps 1+2). `go test -race ./pkg/objtype/...` also clean.
- [x] §10.4 — top-of-file comment retired in T1 (verified at T6: no "server-only"/"skips" wording remaining).
- [x] §10.5 — smoke handoff issued (this section).
- [x] §10.6 — Closes memory trailer (this close commit).

**Smoke handoff (user-driven):**
1. Restart goscape world server at this commit (or any commit at/after `00e9eb0`).
2. Java client login as Tutorial Island fresh char (avoid coord-based chat-suppression zones per `java_client_coord_chat_suppression.md`).
3. Repeat the 3 OPLOC1 clicks captured in NAI-79 H4 re-smoke:
   - RS Guide door (loc 3014, DebugName=`newbie_door1`, Op[0]=`"Open"`)
   - bookcase (loc 380, Op[0]=`"Search"`)
   - drawer (loc 350, DebugName=`drawers2`, Op[0]=`"Open"`)
4. Capture session-log `oploc gate` records for each click.

**Expected gate signal at fix:** `gate=script_dispatch` (or no gate record at all → full dispatch success). Walking-on-click should resume for all 3 locs.

**Routing if smoke does NOT advance gate:**
- All 3 still at `op_slot_empty` → real-cache regression should have caught this; bin-decode the cache for loc 3014 and audit.
- Different gate fires (e.g., `getloc_nil`, `viewport`, `delayed`) → second blocker exposed; route to NAI-81 brainstorm with new gate name as routing target (per `smoke_unchanged_means_multiple_blockers.md`).
- Gate=`script_dispatch` but no walking → SetInteraction reaches but pathing/visual blocked downstream; route to a movement-side investigation.

---

## Smoke result (2026-05-03, user-driven at HEAD `099de62`)

**Outcome:** ✅ NAI-80 cascade-blocker silenced. All 3 OPLOC1 clicks advance past `op_slot_empty` to **script_dispatch**:

| Loc | DebugName | Op[1] | Script invoked | New gate |
|---|---|---|---|---|
| 3014 | `newbie_door1` | `Open` | `[oploc1,newbie_door1]` | `no handler for LOC_COORD (opcode 3005) at pc=9` |
| 380 | `bookcase` | `Search` | `[oploc1,_bookcase]` | `no handler for P_ARRIVEDELAY (opcode 2068) at pc=0` |
| 350 | `drawers2` | `Open` | `[oploc1,_drawer]` | `no handler for LOC_COORD (opcode 3005) at pc=4` |

Plus a bonus surfaced via player exploration: loc 375 (`chestclosed`) → `[oploc1,_chest_closed]` → same `LOC_COORD` gap at pc=4.

**Adjacent observation (orthogonal):** "I can't reach that" message fires when the player clips into the bookcase footprint. Reach/pathing-side concern; not part of NAI-80 chain.

**Cascade routing per `cascade_theory_smoke_binding.md`:** smoke binds the attribution. NAI-80 closed. Two follow-up sub-specs:

- **NAI-81 — port `LOC_COORD` (opcode 3005) script handler.** Higher leverage: 3 distinct script consumers in this smoke alone (`_drawer`, `_chest_closed`, `newbie_door1`). TS reference: `Engine-TS` ScriptOpcode.LOC_COORD handler. Constants already declared at `pkg/script/opcode.go:980` (`OpLocCoord`).
- **NAI-82 — port `P_ARRIVEDELAY` (opcode 2068) script handler.** 1 consumer (`_bookcase`). Constants already declared at `pkg/script/opcode.go:744` (`OpPArriveDelay`).

Both are `protocol_stub_not_completed`-pattern: opcode constants declared but no dispatch wiring.
