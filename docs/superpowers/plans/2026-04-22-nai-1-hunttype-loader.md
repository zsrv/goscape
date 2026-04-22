# NAI-1 HuntType Cache Loader — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the TypeScript `HuntType` cache-config loader to Go at `pkg/objtype/hunttype.go`, and wire the resulting `HuntTypeConfigs` onto `*Server`. Downstream consumer is NAI-7; NAI-1 ships data shape + loader + server field only.

**Architecture:** Follows the existing `pkg/objtype` cache-loader pattern (see `npctype.go`, `varptype.go`). Top-level enum consts; `HuntType` struct embeds `ConfigType`; `Decode(code, dat)` method; `LoadHuntTypes(dir)` entry point with `parseHuntTypes` count-loop. Server-only load (no client jag). Silent on missing `hunt.dat` per TS reference.

**Tech Stack:** Go, `pkg/io/packet` for binary I/O, `pkg/objtype/configtype.go` for the decode framework.

**Spec:** `docs/superpowers/specs/2026-04-22-nai-1-hunttype-loader-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

---

## File Structure

**Created:**
- `pkg/objtype/hunttype.go` — enums, types, decode, load
- `pkg/objtype/hunttype_test.go` — all tests for above

**Modified:**
- `modules/world/server.go` — add `huntTypes` field at ~line 82; add load call after existing NPC-type load at ~line 191

Three files total, one clear responsibility each.

---

## Task 1: Scaffold enums, struct, and NewHuntType with defaults test

**Files:**
- Create: `pkg/objtype/hunttype.go`
- Create: `pkg/objtype/hunttype_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/objtype/hunttype_test.go` with:

```go
package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestHuntTypeDefaults(t *testing.T) {
	ht := NewHuntType(42)

	if ht.ID != 42 {
		t.Errorf("ID: got %d, want 42", ht.ID)
	}
	if ht.Type != HuntModeOff {
		t.Errorf("Type: got %d, want HuntModeOff (%d)", ht.Type, HuntModeOff)
	}
	if ht.CheckVis != HuntVisOff {
		t.Errorf("CheckVis: got %d, want HuntVisOff", ht.CheckVis)
	}
	if ht.CheckNotTooStrong != HuntCheckNotTooStrongOff {
		t.Errorf("CheckNotTooStrong: got %d, want HuntCheckNotTooStrongOff", ht.CheckNotTooStrong)
	}
	if ht.CheckNotBusy {
		t.Errorf("CheckNotBusy: got true, want false")
	}
	if ht.FindKeepHunting {
		t.Errorf("FindKeepHunting: got true, want false")
	}
	if ht.FindNewMode != NPCModeNull {
		t.Errorf("FindNewMode: got %d, want NPCModeNull (%d)", ht.FindNewMode, NPCModeNull)
	}
	if ht.NobodyNear != HuntNobodyNearPauseHunt {
		t.Errorf("NobodyNear: got %d, want HuntNobodyNearPauseHunt", ht.NobodyNear)
	}
	if ht.CheckNotCombat != -1 {
		t.Errorf("CheckNotCombat: got %d, want -1", ht.CheckNotCombat)
	}
	if ht.CheckNotCombatSelf != -1 {
		t.Errorf("CheckNotCombatSelf: got %d, want -1", ht.CheckNotCombatSelf)
	}
	if !ht.CheckAfk {
		t.Errorf("CheckAfk: got false, want true")
	}
	if ht.Rate != 1 {
		t.Errorf("Rate: got %d, want 1", ht.Rate)
	}
	for name, got := range map[string]int{
		"CheckCategory": ht.CheckCategory,
		"CheckNpc":      ht.CheckNpc,
		"CheckObj":      ht.CheckObj,
		"CheckLoc":      ht.CheckLoc,
		"CheckInv":      ht.CheckInv,
		"CheckObjParam": ht.CheckObjParam,
		"CheckInvVal":   ht.CheckInvVal,
	} {
		if got != -1 {
			t.Errorf("%s: got %d, want -1", name, got)
		}
	}
	if ht.CheckInvCondition != "" {
		t.Errorf("CheckInvCondition: got %q, want empty", ht.CheckInvCondition)
	}
	if ht.CheckVars != nil {
		t.Errorf("CheckVars: got %v, want nil", ht.CheckVars)
	}

	// Silence unused-import warning until Task 2 uses it.
	_ = packet2.NewPacket
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestHuntTypeDefaults
```

Expected: compile error — `undefined: NewHuntType` / `undefined: HuntModeOff` / etc.

- [ ] **Step 3: Create the scaffold in hunttype.go**

Create `pkg/objtype/hunttype.go`:

```go
package objtype

// HuntModeType values mirror TS HuntModeType.
// See Engine-TS/src/engine/entity/hunt/HuntModeType.ts.
const (
	HuntModeOff     = 0
	HuntModePlayer  = 1
	HuntModeNpc     = 2
	HuntModeObj     = 3
	HuntModeScenery = 4
)

// HuntVis values mirror TS HuntVis.
const (
	HuntVisOff         = 0
	HuntVisLineOfSight = 1
	HuntVisLineOfWalk  = 2
)

// HuntNobodyNear values mirror TS HuntNobodyNear.
const (
	HuntNobodyNearKeepHunting = 0
	HuntNobodyNearPauseHunt   = 1
)

// HuntCheckNotTooStrong values mirror TS HuntCheckNotTooStrong.
const (
	HuntCheckNotTooStrongOff               = 0
	HuntCheckNotTooStrongOutsideWilderness = 1
)

// HuntCheckVar is one entry in HuntType.CheckVars: a variable-ID plus a
// comparison operator and constant. Populated by decode codes 18/19/20.
type HuntCheckVar struct {
	VarID     int
	Condition string
	Val       int
}

// HuntType is a single `hunt.dat` record.
type HuntType struct {
	ConfigType
	Type               int
	CheckVis           int
	CheckNotTooStrong  int
	CheckNotBusy       bool
	FindKeepHunting    bool
	FindNewMode        int
	NobodyNear         int
	CheckNotCombat     int
	CheckNotCombatSelf int
	CheckAfk           bool
	Rate               int
	CheckCategory      int
	CheckNpc           int
	CheckObj           int
	CheckLoc           int
	CheckInv           int
	CheckObjParam      int
	CheckInvCondition  string
	CheckInvVal        int
	CheckVars          []HuntCheckVar
}

// NewHuntType returns a HuntType populated with TS defaults.
func NewHuntType(id int) *HuntType {
	return &HuntType{
		ConfigType: ConfigType{
			ID: id,
		},
		Type:               HuntModeOff,
		CheckVis:           HuntVisOff,
		CheckNotTooStrong:  HuntCheckNotTooStrongOff,
		FindNewMode:        NPCModeNull,
		NobodyNear:         HuntNobodyNearPauseHunt,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckAfk:           true,
		Rate:               1,
		CheckCategory:      -1,
		CheckNpc:           -1,
		CheckObj:           -1,
		CheckLoc:           -1,
		CheckInv:           -1,
		CheckObjParam:      -1,
		CheckInvVal:        -1,
	}
}

// HuntTypeConfigs is the parsed registry.
type HuntTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*HuntType
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestHuntTypeDefaults -v
```

Expected: `--- PASS: TestHuntTypeDefaults`.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/hunttype.go pkg/objtype/hunttype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-1 HuntType struct + enum consts + defaults

Scaffold pkg/objtype/hunttype.go with the four hunt-related enum
families (HuntModeType, HuntVis, HuntNobodyNear, HuntCheckNotTooStrong)
as top-level consts, the HuntType struct with ConfigType embed, and
NewHuntType factory seeding TS defaults. Defaults test covers every
non-zero default explicitly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Decode method covering all 20 opcodes + 250

**Files:**
- Modify: `pkg/objtype/hunttype.go` (add `Decode` method)
- Modify: `pkg/objtype/hunttype_test.go` (add three tests)

- [ ] **Step 1: Write the failing decode-per-opcode test**

Append to `pkg/objtype/hunttype_test.go`:

```go
func TestHuntTypeDecodeAllOpcodes(t *testing.T) {
	tests := []struct {
		name   string
		build  func(*packet2.Packet)
		verify func(*testing.T, *HuntType)
	}{
		{
			name:  "code 1 Type",
			build: func(p *packet2.Packet) { p.P1(1); p.P1(uint8(HuntModePlayer)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.Type != HuntModePlayer {
					t.Errorf("Type: got %d, want HuntModePlayer", ht.Type)
				}
			},
		},
		{
			name:  "code 2 CheckVis",
			build: func(p *packet2.Packet) { p.P1(2); p.P1(uint8(HuntVisLineOfSight)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckVis != HuntVisLineOfSight {
					t.Errorf("CheckVis: got %d, want HuntVisLineOfSight", ht.CheckVis)
				}
			},
		},
		{
			name:  "code 3 CheckNotTooStrong",
			build: func(p *packet2.Packet) { p.P1(3); p.P1(uint8(HuntCheckNotTooStrongOutsideWilderness)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNotTooStrong != HuntCheckNotTooStrongOutsideWilderness {
					t.Errorf("CheckNotTooStrong: got %d", ht.CheckNotTooStrong)
				}
			},
		},
		{
			name:  "code 4 CheckNotBusy",
			build: func(p *packet2.Packet) { p.P1(4) },
			verify: func(t *testing.T, ht *HuntType) {
				if !ht.CheckNotBusy {
					t.Errorf("CheckNotBusy: got false, want true")
				}
			},
		},
		{
			name:  "code 5 FindKeepHunting",
			build: func(p *packet2.Packet) { p.P1(5) },
			verify: func(t *testing.T, ht *HuntType) {
				if !ht.FindKeepHunting {
					t.Errorf("FindKeepHunting: got false, want true")
				}
			},
		},
		{
			name:  "code 6 FindNewMode",
			build: func(p *packet2.Packet) { p.P1(6); p.P1(uint8(NPCModeWander)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.FindNewMode != NPCModeWander {
					t.Errorf("FindNewMode: got %d", ht.FindNewMode)
				}
			},
		},
		{
			name:  "code 7 NobodyNear",
			build: func(p *packet2.Packet) { p.P1(7); p.P1(uint8(HuntNobodyNearKeepHunting)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.NobodyNear != HuntNobodyNearKeepHunting {
					t.Errorf("NobodyNear: got %d", ht.NobodyNear)
				}
			},
		},
		{
			name:  "code 8 CheckNotCombat",
			build: func(p *packet2.Packet) { p.P1(8); p.P2(1234) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNotCombat != 1234 {
					t.Errorf("CheckNotCombat: got %d", ht.CheckNotCombat)
				}
			},
		},
		{
			name:  "code 9 CheckNotCombatSelf",
			build: func(p *packet2.Packet) { p.P1(9); p.P2(5678) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNotCombatSelf != 5678 {
					t.Errorf("CheckNotCombatSelf: got %d", ht.CheckNotCombatSelf)
				}
			},
		},
		{
			name:  "code 10 CheckAfk=false",
			build: func(p *packet2.Packet) { p.P1(10) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckAfk {
					t.Errorf("CheckAfk: got true, want false")
				}
			},
		},
		{
			name:  "code 11 Rate",
			build: func(p *packet2.Packet) { p.P1(11); p.P2(42) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.Rate != 42 {
					t.Errorf("Rate: got %d", ht.Rate)
				}
			},
		},
		{
			name:  "code 12 CheckCategory",
			build: func(p *packet2.Packet) { p.P1(12); p.P2(7) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckCategory != 7 {
					t.Errorf("CheckCategory: got %d", ht.CheckCategory)
				}
			},
		},
		{
			name:  "code 13 CheckNpc",
			build: func(p *packet2.Packet) { p.P1(13); p.P2(99) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNpc != 99 {
					t.Errorf("CheckNpc: got %d", ht.CheckNpc)
				}
			},
		},
		{
			name:  "code 14 CheckObj",
			build: func(p *packet2.Packet) { p.P1(14); p.P2(100) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckObj != 100 {
					t.Errorf("CheckObj: got %d", ht.CheckObj)
				}
			},
		},
		{
			name:  "code 15 CheckLoc",
			build: func(p *packet2.Packet) { p.P1(15); p.P2(55) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckLoc != 55 {
					t.Errorf("CheckLoc: got %d", ht.CheckLoc)
				}
			},
		},
		{
			name: "code 16 CheckInv + CheckObj + cond + val",
			build: func(p *packet2.Packet) {
				p.P1(16)
				p.P2(10) // CheckInv
				p.P2(20) // CheckObj
				p.PJStrLF(">")
				p.P4(uint32(int32(-5))) // signed -5
			},
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckInv != 10 {
					t.Errorf("CheckInv: got %d", ht.CheckInv)
				}
				if ht.CheckObj != 20 {
					t.Errorf("CheckObj: got %d", ht.CheckObj)
				}
				if ht.CheckInvCondition != ">" {
					t.Errorf("CheckInvCondition: got %q", ht.CheckInvCondition)
				}
				if ht.CheckInvVal != -5 {
					t.Errorf("CheckInvVal: got %d", ht.CheckInvVal)
				}
			},
		},
		{
			name: "code 17 CheckInv + CheckObjParam + cond + val",
			build: func(p *packet2.Packet) {
				p.P1(17)
				p.P2(11) // CheckInv
				p.P2(22) // CheckObjParam
				p.PJStrLF("<")
				p.P4(uint32(int32(7)))
			},
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckInv != 11 {
					t.Errorf("CheckInv: got %d", ht.CheckInv)
				}
				if ht.CheckObjParam != 22 {
					t.Errorf("CheckObjParam: got %d", ht.CheckObjParam)
				}
				if ht.CheckInvCondition != "<" {
					t.Errorf("CheckInvCondition: got %q", ht.CheckInvCondition)
				}
				if ht.CheckInvVal != 7 {
					t.Errorf("CheckInvVal: got %d", ht.CheckInvVal)
				}
			},
		},
		{
			name: "code 18 single CheckVar",
			build: func(p *packet2.Packet) {
				p.P1(18)
				p.P2(33) // VarID
				p.PJStrLF("=")
				p.P4(uint32(int32(-1)))
			},
			verify: func(t *testing.T, ht *HuntType) {
				if len(ht.CheckVars) != 1 {
					t.Fatalf("CheckVars: got %d entries, want 1", len(ht.CheckVars))
				}
				v := ht.CheckVars[0]
				if v.VarID != 33 || v.Condition != "=" || v.Val != -1 {
					t.Errorf("CheckVars[0]: got %+v", v)
				}
			},
		},
		{
			name:  "code 250 DebugName",
			build: func(p *packet2.Packet) { p.P1(250); p.PJStrLF("boss_hunt") },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.DebugName != "boss_hunt" {
					t.Errorf("DebugName: got %q", ht.DebugName)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := packet2.NewPacket(nil)
			tc.build(pkt)
			pkt.P1(0) // terminator

			reader := packet2.NewPacket(pkt.Bytes())
			ht := NewHuntType(0)
			if err := DecodeType(reader, ht); err != nil {
				t.Fatalf("DecodeType: %v", err)
			}
			tc.verify(t, ht)
		})
	}
}

func TestHuntTypeDecodeUnknownOpcode(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(42) // undefined opcode
	reader := packet2.NewPacket(pkt.Bytes())

	err := DecodeType(reader, NewHuntType(0))
	if err == nil {
		t.Fatalf("DecodeType: want error, got nil")
	}
	if got := err.Error(); got != "unrecognized hunt config code 42" {
		t.Errorf("error message: got %q", got)
	}
}

func TestHuntTypeDecodeCheckVarsAppend(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	// Three consecutive CheckVars via codes 18, 19, 20.
	for i, code := range []uint8{18, 19, 20} {
		pkt.P1(code)
		pkt.P2(uint16(100 + i))
		pkt.PJStrLF("=")
		pkt.P4(uint32(int32(i + 1)))
	}
	pkt.P1(0) // terminator

	reader := packet2.NewPacket(pkt.Bytes())
	ht := NewHuntType(0)
	if err := DecodeType(reader, ht); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if len(ht.CheckVars) != 3 {
		t.Fatalf("CheckVars: got %d entries, want 3", len(ht.CheckVars))
	}
	for i, v := range ht.CheckVars {
		wantID := 100 + i
		wantVal := i + 1
		if v.VarID != wantID || v.Condition != "=" || v.Val != wantVal {
			t.Errorf("CheckVars[%d]: got %+v, want {VarID:%d Cond:= Val:%d}", i, v, wantID, wantVal)
		}
	}
}
```

Remove the `_ = packet2.NewPacket` line from `TestHuntTypeDefaults` since `packet2` is now used by the new tests.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestHuntTypeDecode' -v
```

Expected: compile error — `ht has no method Decode`, or all three tests FAIL because `DecodeType` can't dispatch on `HuntType`.

- [ ] **Step 3: Add the Decode method to hunttype.go**

Add these imports at the top of `hunttype.go` (replacing the current `package objtype` line with the import block):

```go
package objtype

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/io/packet"
)
```

Then append this method to `hunttype.go` (after `NewHuntType`):

```go
// Decode dispatches on the hunt-config opcode, matching TS HuntType.decode
// at Engine-TS/src/cache/config/HuntType.ts:99-147.
func (t *HuntType) Decode(code uint8, dat *packet.Packet) error {
	switch code {
	case 1:
		t.Type = int(dat.G1())
	case 2:
		t.CheckVis = int(dat.G1())
	case 3:
		t.CheckNotTooStrong = int(dat.G1())
	case 4:
		t.CheckNotBusy = true
	case 5:
		t.FindKeepHunting = true
	case 6:
		t.FindNewMode = int(dat.G1())
	case 7:
		t.NobodyNear = int(dat.G1())
	case 8:
		t.CheckNotCombat = int(dat.G2())
	case 9:
		t.CheckNotCombatSelf = int(dat.G2())
	case 10:
		t.CheckAfk = false
	case 11:
		t.Rate = int(dat.G2())
	case 12:
		t.CheckCategory = int(dat.G2())
	case 13:
		t.CheckNpc = int(dat.G2())
	case 14:
		t.CheckObj = int(dat.G2())
	case 15:
		t.CheckLoc = int(dat.G2())
	case 16:
		t.CheckInv = int(dat.G2())
		t.CheckObj = int(dat.G2())
		t.CheckInvCondition = dat.GJStrLF()
		t.CheckInvVal = int(int32(dat.G4()))
	case 17:
		t.CheckInv = int(dat.G2())
		t.CheckObjParam = int(dat.G2())
		t.CheckInvCondition = dat.GJStrLF()
		t.CheckInvVal = int(int32(dat.G4()))
	case 18, 19, 20:
		t.CheckVars = append(t.CheckVars, HuntCheckVar{
			VarID:     int(dat.G2()),
			Condition: dat.GJStrLF(),
			Val:       int(int32(dat.G4())),
		})
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized hunt config code %d", code)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestHuntType' -v
```

Expected: all tests pass — `TestHuntTypeDefaults`, every row of `TestHuntTypeDecodeAllOpcodes`, `TestHuntTypeDecodeUnknownOpcode`, `TestHuntTypeDecodeCheckVarsAppend`.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/hunttype.go pkg/objtype/hunttype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-1 HuntType.Decode — all 20 opcodes + 250

Port TS HuntType.decode switch. Cases 16/17 read quad-tuples ending in
a signed int32 (int(int32(dat.G4())) matches the enumtype.go/invtype.go
pattern for signed reads). Cases 18/19/20 share a single append into
CheckVars. Unknown opcodes return a formatted error matching the
existing npctype.go convention.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: LoadHuntTypes + parseHuntTypes + file-level tests

**Files:**
- Modify: `pkg/objtype/hunttype.go` (add load functions)
- Modify: `pkg/objtype/hunttype_test.go` (add three tests)

- [ ] **Step 1: Write the failing load tests**

Append to `pkg/objtype/hunttype_test.go`:

```go
import (
	"os"
	"path/filepath"
)
```

(If the file already has an import block, merge these in rather than adding a second block.)

Add the tests:

```go
// buildHuntDat assembles a hunt.dat wire blob: u16 count, then for each
// record a sequence of (code, payload) pairs terminated by code 0.
func buildHuntDat(records []func(*packet2.Packet)) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(records)))
	for _, r := range records {
		r(pkt)
		pkt.P1(0) // record terminator
	}
	return pkt.Bytes()
}

func TestLoadHuntTypesTwoRecords(t *testing.T) {
	blob := buildHuntDat([]func(*packet2.Packet){
		func(p *packet2.Packet) {
			p.P1(1)
			p.P1(uint8(HuntModePlayer))
			p.P1(11)
			p.P2(4)
			p.P1(250)
			p.PJStrLF("player_hunt")
		},
		func(p *packet2.Packet) {
			p.P1(1)
			p.P1(uint8(HuntModeNpc))
			p.P1(250)
			p.PJStrLF("npc_hunt")
		},
	})

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server", "hunt.dat"), blob, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfgs, err := LoadHuntTypes(dir)
	if err != nil {
		t.Fatalf("LoadHuntTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("Configs: got %d, want 2", len(cfgs.Configs))
	}
	if cfgs.Configs[0].Type != HuntModePlayer {
		t.Errorf("Configs[0].Type: got %d", cfgs.Configs[0].Type)
	}
	if cfgs.Configs[0].Rate != 4 {
		t.Errorf("Configs[0].Rate: got %d", cfgs.Configs[0].Rate)
	}
	if cfgs.Configs[0].DebugName != "player_hunt" {
		t.Errorf("Configs[0].DebugName: got %q", cfgs.Configs[0].DebugName)
	}
	if cfgs.Configs[1].Type != HuntModeNpc {
		t.Errorf("Configs[1].Type: got %d", cfgs.Configs[1].Type)
	}
	if cfgs.ConfigNames["player_hunt"] != 0 {
		t.Errorf("ConfigNames[player_hunt]: got %d, want 0", cfgs.ConfigNames["player_hunt"])
	}
	if cfgs.ConfigNames["npc_hunt"] != 1 {
		t.Errorf("ConfigNames[npc_hunt]: got %d, want 1", cfgs.ConfigNames["npc_hunt"])
	}
}

func TestLoadHuntTypesMissingFile(t *testing.T) {
	dir := t.TempDir() // no server/hunt.dat created

	cfgs, err := LoadHuntTypes(dir)
	if err != nil {
		t.Fatalf("LoadHuntTypes: got error %v, want nil", err)
	}
	if cfgs == nil {
		t.Fatalf("LoadHuntTypes: cfgs is nil, want empty registry")
	}
	if len(cfgs.Configs) != 0 {
		t.Errorf("Configs: got %d, want 0", len(cfgs.Configs))
	}
	if cfgs.ConfigNames == nil {
		t.Errorf("ConfigNames: got nil, want empty map")
	}
}

func TestLoadHuntTypesParseError(t *testing.T) {
	// count=1 but no record bytes → Decode will read past end.
	pkt := packet2.NewPacket(nil)
	pkt.P2(1)
	pkt.P1(1)
	// missing payload byte for code 1

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server", "hunt.dat"), pkt.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := LoadHuntTypes(dir); err == nil {
		t.Fatalf("LoadHuntTypes: got nil error, want parse error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestLoadHuntTypes' -v
```

Expected: compile error — `undefined: LoadHuntTypes`.

- [ ] **Step 3: Add LoadHuntTypes and parseHuntTypes**

Update the import block at the top of `hunttype.go` to:

```go
package objtype

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/packet"
)
```

Then append at the bottom of `hunttype.go`:

```go
// LoadHuntTypes parses server/hunt.dat at dir into a HuntTypeConfigs
// registry. Silent on missing hunt.dat: returns an empty registry with
// nil error. Matches TS HuntType.load at
// Engine-TS/src/cache/config/HuntType.ts:16-22 — hunt-less caches are a
// supported scenario in the reference implementation.
func LoadHuntTypes(dir string) (*HuntTypeConfigs, error) {
	server, err := packet.Load(filepath.Join(dir, "server", "hunt.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &HuntTypeConfigs{
				ConfigNames: map[string]int{},
			}, nil
		}
		return nil, err
	}
	return parseHuntTypes(server)
}

func parseHuntTypes(server *packet.Packet) (*HuntTypeConfigs, error) {
	count := int(server.G2())
	configs := make([]*HuntType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewHuntType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &HuntTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestLoadHuntTypes' -v
```

Expected: `TestLoadHuntTypesTwoRecords`, `TestLoadHuntTypesMissingFile`, `TestLoadHuntTypesParseError` all PASS.

Also run the whole package to confirm no regressions:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -v
```

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/hunttype.go pkg/objtype/hunttype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-1 LoadHuntTypes with silent-missing behaviour

Add LoadHuntTypes + parseHuntTypes following the parseVarpTypes pattern
(server-only, no client jag). Missing hunt.dat yields an empty
HuntTypeConfigs with nil error, matching TS HuntType.load at
cache/config/HuntType.ts:16-22 — hunt-less caches are a supported
scenario in the reference.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire `huntTypes` onto `*Server`

**Files:**
- Modify: `modules/world/server.go` — add field near `npcTypes`, add load call after existing NPC load.

- [ ] **Step 1: Add the `huntTypes` field**

In `modules/world/server.go`, locate this block (around line 82):

```go
	npcTypes    *objtype.NPCTypeConfigs
	npcs        [8192]*Npc
```

Change it to:

```go
	npcTypes    *objtype.NPCTypeConfigs
	huntTypes   *objtype.HuntTypeConfigs
	npcs        [8192]*Npc
```

- [ ] **Step 2: Add the load call in `NewServer`**

In `modules/world/server.go`, locate this block (around line 187–191):

```go
	npcTypes, err := objtype.LoadNPCTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load npc types: %w", err)
	}
	s.npcTypes = npcTypes
```

Insert immediately after it:

```go
	huntTypes, err := objtype.LoadHuntTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load hunt types: %w", err)
	}
	s.huntTypes = huntTypes
```

- [ ] **Step 3: Build and verify the whole world module still compiles and passes tests**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```

Expected: build succeeds; all existing tests pass (the new field is unreferenced apart from the load call, so no behavioural change).

- [ ] **Step 4: Run full suite with race detector**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: full suite passes.

- [ ] **Step 5: Grep for unexpected `huntTypes` references**

Run:
```
rg -n '\bhuntTypes\b' modules/ pkg/
```

Expected: exactly two matches, both in `modules/world/server.go` (field declaration + load assignment). No other file should reference it yet — NAI-7 is the first consumer.

- [ ] **Step 6: Commit, closing NAI-1**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-1 wire huntTypes onto Server — closes NAI-1

Load HuntTypeConfigs at world startup alongside existing NPC-type load.
Field is unreferenced until NAI-7 adds the hunt dispatcher, by design:
the roadmap calls for data-layer-first ordering so hunt behaviour has
its lookup backing ready by the time NAI-7 lands.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist results

**1. Spec coverage:**
- Enum consts (spec § "Enum definitions") → Task 1
- `HuntCheckVar` + `HuntType` structs (spec § "Type definitions") → Task 1
- `NewHuntType` defaults (spec § "NewHuntType defaults" table) → Task 1 + TestHuntTypeDefaults
- `Decode` switch opcodes 1–20 + 250 (spec § "Decode cases") → Task 2
- Unknown-opcode error (spec § "Decode cases" default row) → Task 2 + TestHuntTypeDecodeUnknownOpcode
- `LoadHuntTypes` / `parseHuntTypes` (spec § "LoadHuntTypes / parseHuntTypes") → Task 3
- Silent-missing-file behaviour (spec § "Fidelity notes" first bullet) → Task 3 + TestLoadHuntTypesMissingFile
- `HuntTypeConfigs` struct (spec § "Type definitions") → Task 1
- `huntTypes` field on Server (spec § "Server wiring") → Task 4
- Load call after existing LoadNPCTypes (spec § "Server wiring" step 2) → Task 4
- No accessor method / script-VM wiring (spec § "Non-goals") → honoured by omission from Task 4
- 7 planned tests (spec § "Test strategy") → all 7 accounted for: TestHuntTypeDefaults, TestHuntTypeDecodeAllOpcodes, TestHuntTypeDecodeUnknownOpcode, TestHuntTypeDecodeCheckVarsAppend, TestLoadHuntTypesTwoRecords, TestLoadHuntTypesMissingFile, TestLoadHuntTypesParseError.

No gaps.

**2. Placeholder scan:** No TBDs, TODOs, or vague steps. Every code step contains the exact code and every run step contains the exact command with expected output.

**3. Type consistency:** `HuntCheckVar`, `HuntType`, `HuntTypeConfigs`, `HuntCheckNotTooStrongOff`, `NPCModeNull`, `HuntModePlayer/Npc`, `HuntNobodyNearPauseHunt` all match spelling between spec, test code, and implementation code across all four tasks. `LoadHuntTypes` / `parseHuntTypes` match across Task 3.

---

## Commit trail (for reference)

Four commits close NAI-1:

1. `feat(objtype): NAI-1 HuntType struct + enum consts + defaults`
2. `feat(objtype): NAI-1 HuntType.Decode — all 20 opcodes + 250`
3. `feat(objtype): NAI-1 LoadHuntTypes with silent-missing behaviour`
4. `feat(world): NAI-1 wire huntTypes onto Server — closes NAI-1`

Each commit leaves the tree green (build + all tests pass) and ships self-contained work.
