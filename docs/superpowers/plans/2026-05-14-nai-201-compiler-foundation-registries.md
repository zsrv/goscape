# NAI-201 — Compiler-foundation registries — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the four missing prerequisite registries that NAI-202's `runServerCompiler` driver will consume: `NpcStatMap` and `NpcModeMap` in `pkg/objtype`, `ScriptOpcodeMap` and `ScriptOpcodePointers` in `pkg/script`. Pure data + parity tests; zero production consumers.

**Architecture:** Two new files in `pkg/objtype/` (`npcstat.go`, `npcmode.go`) mirroring the existing `playerstat.go` shape; two new files in `pkg/script/` (`opcode_map.go`, `opcode_pointers.go`) referencing existing `Op*` constants in `pkg/script/opcode.go`. Each registry ships alongside a `*_test.go` file with parity / spot-check / cross-registry validity tests. A package-level `nai201_deviation_pins_test.go` pins the two tracked deviations.

**Tech Stack:** Go 1.26+. Stdlib only (`testing`, `reflect`). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-14-nai-201-compiler-foundation-registries-design.md` (commit `1b25104`).
**HEAD at plan-write:** `1b25104`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/objtype/playerstat_test.go` and `pkg/script/*_test.go`: bare `if got != want { t.Fatalf(...) }`, `reflect.DeepEqual` for whole-map comparison, table-driven where the same shape repeats.
- **Modern Go** (per `[[use-modern-go]]`): `for k, v := range m`, no deprecated `ioutil.*`. No generics introduced unless required by a stdlib API.
- **Goscape Op* naming** is **NOT mechanical** from TS UPPER_SNAKE. Verified at plan-write for every spot-check name used below (`OpFindUID` not `OpFindUid`; `OpGetTimeSpent` not `OpGetTimespent`; etc.) — when porting the 393 `ScriptOpcodeMap` entries, the implementer must look up each goscape `Op*` name in `pkg/script/opcode.go`, **not** mechanically translate from TS.
- **TS source canon** is `/home/owner/Code/github.com/LostCityRS/Engine-TS` per `[[ts_source_canonical_path]]`. All TS line numbers below reference that tree.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `1b25104`:

| Premise | Verification | Result |
|---|---|---|
| Module path is `github.com/zsrv/goscape` | `head -1 go.mod` | ✅ |
| `pkg/objtype/npcstat.go` does not exist | `test ! -e pkg/objtype/npcstat.go` | ✅ |
| `pkg/objtype/npcmode.go` does not exist | `test ! -e pkg/objtype/npcmode.go` | ✅ |
| `pkg/script/opcode_map.go` does not exist | `test ! -e pkg/script/opcode_map.go` | ✅ |
| `pkg/script/opcode_pointers.go` does not exist | `test ! -e pkg/script/opcode_pointers.go` | ✅ |
| `pkg/script/Opcode` type exported and `Op*` constants present | `grep -c "^\tOp[A-Z]" pkg/script/opcode.go` | ✅ 394 |
| `pkg/objtype/PlayerStatMap` exists (reference shape) | `pkg/objtype/playerstat.go:37` | ✅ |
| TS Compiler.ts:109-367 will NOT be touched in this sub-spec | scope §2 of spec | ✅ |
| TS `ScriptOpcodeMap` entry count | `awk '/^export const ScriptOpcodeMap/,/^]\)/' src/engine/script/ScriptOpcode.ts \| grep -c "^\s*\['"` | ✅ 393 |
| TS `ScriptOpcodePointers` entry count | `grep -c "\[ScriptOpcode\." src/engine/script/ScriptOpcodePointers.ts` | ✅ 237 |
| TS `NpcMode` enum named-value count | awk over enum body | ✅ 68 |
| TS `NpcModeMap` active entry count | `awk '/^export const NpcModeMap/,/^]\)/' \| grep -c "^\s*\['"` | ✅ 48 |
| TS `POINTER_GROUP_FIND` spread sites | `grep -n POINTER_GROUP_FIND src/engine/script/ScriptOpcodePointers.ts` | ✅ 6 (lines 286, 301, 314, 370 simple; 569, 711 extended) |

The verified entry counts seed the test assertions in T2 (NpcMode), T3 (ScriptOpcodeMap), and T5 (ScriptOpcodePointers).

---

## File layout (created in this plan)

- Create: `pkg/objtype/npcstat.go`
- Create: `pkg/objtype/npcstat_test.go`
- Create: `pkg/objtype/npcmode.go`
- Create: `pkg/objtype/npcmode_test.go`
- Create: `pkg/script/opcode_map.go`
- Create: `pkg/script/opcode_map_test.go`
- Create: `pkg/script/opcode_pointers.go`
- Create: `pkg/script/opcode_pointers_test.go`
- Create: `pkg/script/nai201_deviation_pins_test.go`

No existing files are modified.

---

## Task 1: `NpcStatMap` foundation

**Files:**
- Create: `pkg/objtype/npcstat.go`
- Create: `pkg/objtype/npcstat_test.go`

**TS source:** `src/engine/entity/NpcStat.ts:1-17` (full file).

**Spec:** §5.2, §7.1.

### Step 1.1 — Write the failing parity test

Create `pkg/objtype/npcstat_test.go`:

```go
package objtype

import (
	"reflect"
	"testing"
)

// TestNpcStatMap_Parity pins spec §7.1: NpcStatMap mirrors TS
// src/engine/entity/NpcStat.ts:10-17 verbatim. All 6 uppercase stat
// names map to the canonical NpcStat* index values.
func TestNpcStatMap_Parity(t *testing.T) {
	expected := map[string]int{
		"ATTACK":    NpcStatAttack,
		"DEFENCE":   NpcStatDefence,
		"STRENGTH":  NpcStatStrength,
		"HITPOINTS": NpcStatHitpoints,
		"RANGED":    NpcStatRanged,
		"MAGIC":     NpcStatMagic,
	}
	if !reflect.DeepEqual(NpcStatMap, expected) {
		t.Fatalf("NpcStatMap mismatch\n got = %#v\nwant = %#v", NpcStatMap, expected)
	}
}

// TestNpcStat_IndexValues pins the canonical index values match TS
// enum NpcStat (NpcStat.ts:1-8) and the count constant matches.
func TestNpcStat_IndexValues(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"NpcStatAttack", NpcStatAttack, 0},
		{"NpcStatDefence", NpcStatDefence, 1},
		{"NpcStatStrength", NpcStatStrength, 2},
		{"NpcStatHitpoints", NpcStatHitpoints, 3},
		{"NpcStatRanged", NpcStatRanged, 4},
		{"NpcStatMagic", NpcStatMagic, 5},
		{"NpcStatCount", NpcStatCount, 6},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}
```

- [ ] **Step 1.2 — Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run "TestNpcStat"
```

Expected: build error — `undefined: NpcStatMap`, `undefined: NpcStatAttack` etc.

### Step 1.3 — Write `npcstat.go`

Create `pkg/objtype/npcstat.go`:

```go
package objtype

// NpcStat* are indices into Npc.stats / Npc.baseLevels for NPC combat
// stats. Index values match TS NpcStat enum (NpcStat.ts:1-8).
const (
	NpcStatAttack    = 0
	NpcStatDefence   = 1
	NpcStatStrength  = 2
	NpcStatHitpoints = 3
	NpcStatRanged    = 4
	NpcStatMagic     = 5

	NpcStatCount = 6
)

// NpcStatMap maps uppercase NPC-stat name → stat index. Mirrors TS
// NpcStatMap (NpcStat.ts:10-17). Consumed by the bytecode compiler's
// LoadMap call in pkg/pack/compiler (NAI-202 runServerCompiler) and by
// any future ::npc cheat handlers that parse stat names.
var NpcStatMap = map[string]int{
	"ATTACK":    NpcStatAttack,
	"DEFENCE":   NpcStatDefence,
	"STRENGTH":  NpcStatStrength,
	"HITPOINTS": NpcStatHitpoints,
	"RANGED":    NpcStatRanged,
	"MAGIC":     NpcStatMagic,
}
```

- [ ] **Step 1.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run "TestNpcStat" -v
```

Expected: PASS for `TestNpcStatMap_Parity` and `TestNpcStat_IndexValues`.

- [ ] **Step 1.5 — Full package test (no regressions)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```

Expected: PASS, no other tests affected.

- [ ] **Step 1.6 — Commit**

```bash
git add pkg/objtype/npcstat.go pkg/objtype/npcstat_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-201 T1 — NpcStat constants + NpcStatMap

Mirrors TS src/engine/entity/NpcStat.ts:1-17. 6 entries (ATTACK through
MAGIC); zero production consumers yet (NAI-202's runServerCompiler will
be the first via LoadMap).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `NpcModeMap` foundation + queue-todo deviation pin

**Files:**
- Create: `pkg/objtype/npcmode.go`
- Create: `pkg/objtype/npcmode_test.go`

**TS source:** `src/engine/entity/NpcMode.ts:1-168` (full file). Lines 1-96 are the enum; 98-168 are the map (with lines 147-167 commented-out QUEUE entries).

**Spec:** §5.3, §7.2, §7.3, §10 deviation `NAI-201-D-NPCMODE-QUEUE-TODO`.

### Step 2.1 — Write the failing parity + deviation-pin tests

Create `pkg/objtype/npcmode_test.go`:

```go
package objtype

import (
	"reflect"
	"testing"
)

// TestNpcModeMap_Parity pins spec §7.2: NpcModeMap mirrors TS
// src/engine/entity/NpcMode.ts:99-146 verbatim (48 active entries —
// NULL through APNPC5). Queue entries are absent (see deviation
// NAI-201-D-NPCMODE-QUEUE-TODO; tested separately in T2 step 2.7).
func TestNpcModeMap_Parity(t *testing.T) {
	expected := map[string]int{
		"NULL":            NpcModeNull,
		"NONE":            NpcModeNone,
		"WANDER":          NpcModeWander,
		"PATROL":          NpcModePatrol,
		"PLAYERESCAPE":    NpcModePlayerEscape,
		"PLAYERFOLLOW":    NpcModePlayerFollow,
		"PLAYERFACE":      NpcModePlayerFace,
		"PLAYERFACECLOSE": NpcModePlayerFaceClose,
		"OPPLAYER1":       NpcModeOpPlayer1,
		"OPPLAYER2":       NpcModeOpPlayer2,
		"OPPLAYER3":       NpcModeOpPlayer3,
		"OPPLAYER4":       NpcModeOpPlayer4,
		"OPPLAYER5":       NpcModeOpPlayer5,
		"APPLAYER1":       NpcModeApPlayer1,
		"APPLAYER2":       NpcModeApPlayer2,
		"APPLAYER3":       NpcModeApPlayer3,
		"APPLAYER4":       NpcModeApPlayer4,
		"APPLAYER5":       NpcModeApPlayer5,
		"OPLOC1":          NpcModeOpLoc1,
		"OPLOC2":          NpcModeOpLoc2,
		"OPLOC3":          NpcModeOpLoc3,
		"OPLOC4":          NpcModeOpLoc4,
		"OPLOC5":          NpcModeOpLoc5,
		"APLOC1":          NpcModeApLoc1,
		"APLOC2":          NpcModeApLoc2,
		"APLOC3":          NpcModeApLoc3,
		"APLOC4":          NpcModeApLoc4,
		"APLOC5":          NpcModeApLoc5,
		"OPOBJ1":          NpcModeOpObj1,
		"OPOBJ2":          NpcModeOpObj2,
		"OPOBJ3":          NpcModeOpObj3,
		"OPOBJ4":          NpcModeOpObj4,
		"OPOBJ5":          NpcModeOpObj5,
		"APOBJ1":          NpcModeApObj1,
		"APOBJ2":          NpcModeApObj2,
		"APOBJ3":          NpcModeApObj3,
		"APOBJ4":          NpcModeApObj4,
		"APOBJ5":          NpcModeApObj5,
		"OPNPC1":          NpcModeOpNpc1,
		"OPNPC2":          NpcModeOpNpc2,
		"OPNPC3":          NpcModeOpNpc3,
		"OPNPC4":          NpcModeOpNpc4,
		"OPNPC5":          NpcModeOpNpc5,
		"APNPC1":          NpcModeApNpc1,
		"APNPC2":          NpcModeApNpc2,
		"APNPC3":          NpcModeApNpc3,
		"APNPC4":          NpcModeApNpc4,
		"APNPC5":          NpcModeApNpc5,
	}
	if len(expected) != 48 {
		t.Fatalf("expected has %d entries; this test fixture itself is broken (want 48)", len(expected))
	}
	if !reflect.DeepEqual(NpcModeMap, expected) {
		t.Fatalf("NpcModeMap mismatch\n got = %#v\nwant = %#v", NpcModeMap, expected)
	}
}

// TestNpcModeMap_QueueEntriesOmitted pins deviation NAI-201-D-NPCMODE-QUEUE-TODO:
// TS NpcMode.ts:147-167 has 20 QUEUE1..QUEUE20 entries commented out
// with "// TODO: these are not used?". Goscape's NpcModeMap omits them.
// The NpcMode* constants themselves DO exist (so the NPC AI state
// machine port — out of NAI-201 scope — can reference them); they just
// have no name-string mapping.
func TestNpcModeMap_QueueEntriesOmitted(t *testing.T) {
	for i := 1; i <= 20; i++ {
		key := "QUEUE" + itoa(i)
		if _, present := NpcModeMap[key]; present {
			t.Errorf("NpcModeMap[%q]: present, want absent (NAI-201-D-NPCMODE-QUEUE-TODO)", key)
		}
	}
}

// itoa is a local helper (avoids stdlib strconv import in a test that
// only needs single-digit-to-two-digit conversion for QUEUE1..QUEUE20).
func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// TestNpcMode_IndexValues spot-checks the canonical index values match
// TS enum NpcMode (NpcMode.ts:1-96). Full coverage is unnecessary —
// TestNpcModeMap_Parity transitively verifies the 48 mapped values, and
// the 20 unmapped QUEUE constants are anchored here.
func TestNpcMode_IndexValues(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"NpcModeNull", NpcModeNull, -1},
		{"NpcModeNone", NpcModeNone, 0},
		{"NpcModeWander", NpcModeWander, 1},
		{"NpcModeApNpc5", NpcModeApNpc5, 46},
		{"NpcModeQueue1", NpcModeQueue1, 47},
		{"NpcModeQueue20", NpcModeQueue20, 66},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}
```

- [ ] **Step 2.2 — Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run "TestNpcMode"
```

Expected: build error — `undefined: NpcModeMap`, `undefined: NpcModeNull` etc.

### Step 2.3 — Write `npcmode.go`

Create `pkg/objtype/npcmode.go`:

```go
package objtype

// NpcMode* are NPC AI mode identifiers. Index values match TS NpcMode
// enum (NpcMode.ts:1-96). The full enum has 68 named values
// (NpcModeNull = -1 through NpcModeQueue20 = 66); goscape mirrors all 68
// so the NPC AI state machine port (a separate sub-spec arc, out of
// NAI-201 scope) can reference any of them.
const (
	NpcModeNull = -1

	NpcModeNone            = 0
	NpcModeWander          = 1
	NpcModePatrol          = 2
	NpcModePlayerEscape    = 3
	NpcModePlayerFollow    = 4
	NpcModePlayerFace      = 5
	NpcModePlayerFaceClose = 6

	NpcModeOpPlayer1 = 7
	NpcModeOpPlayer2 = 8
	NpcModeOpPlayer3 = 9
	NpcModeOpPlayer4 = 10
	NpcModeOpPlayer5 = 11
	NpcModeApPlayer1 = 12
	NpcModeApPlayer2 = 13
	NpcModeApPlayer3 = 14
	NpcModeApPlayer4 = 15
	NpcModeApPlayer5 = 16

	NpcModeOpLoc1 = 17
	NpcModeOpLoc2 = 18
	NpcModeOpLoc3 = 19
	NpcModeOpLoc4 = 20
	NpcModeOpLoc5 = 21
	NpcModeApLoc1 = 22
	NpcModeApLoc2 = 23
	NpcModeApLoc3 = 24
	NpcModeApLoc4 = 25
	NpcModeApLoc5 = 26

	NpcModeOpObj1 = 27
	NpcModeOpObj2 = 28
	NpcModeOpObj3 = 29
	NpcModeOpObj4 = 30
	NpcModeOpObj5 = 31
	NpcModeApObj1 = 32
	NpcModeApObj2 = 33
	NpcModeApObj3 = 34
	NpcModeApObj4 = 35
	NpcModeApObj5 = 36

	NpcModeOpNpc1 = 37
	NpcModeOpNpc2 = 38
	NpcModeOpNpc3 = 39
	NpcModeOpNpc4 = 40
	NpcModeOpNpc5 = 41
	NpcModeApNpc1 = 42
	NpcModeApNpc2 = 43
	NpcModeApNpc3 = 44
	NpcModeApNpc4 = 45
	NpcModeApNpc5 = 46

	// Queue modes 47-66. TS NpcMode.ts:75-95 declares these as enum
	// values; TS NpcModeMap (NpcMode.ts:147-167) has the corresponding
	// name-string entries commented out with `// TODO: these are not
	// used?`. Goscape preserves the enum values (for NPC AI state
	// machine port) but omits the map entries (see NAI-201-D-NPCMODE-QUEUE-TODO).
	NpcModeQueue1  = 47
	NpcModeQueue2  = 48
	NpcModeQueue3  = 49
	NpcModeQueue4  = 50
	NpcModeQueue5  = 51
	NpcModeQueue6  = 52
	NpcModeQueue7  = 53
	NpcModeQueue8  = 54
	NpcModeQueue9  = 55
	NpcModeQueue10 = 56
	NpcModeQueue11 = 57
	NpcModeQueue12 = 58
	NpcModeQueue13 = 59
	NpcModeQueue14 = 60
	NpcModeQueue15 = 61
	NpcModeQueue16 = 62
	NpcModeQueue17 = 63
	NpcModeQueue18 = 64
	NpcModeQueue19 = 65
	NpcModeQueue20 = 66
)

// NpcModeMap maps uppercase NPC-mode name → mode index. Mirrors TS
// NpcModeMap (NpcMode.ts:98-146 active entries; lines 147-167 are
// commented-out TODOs that are NOT ported per NAI-201-D-NPCMODE-QUEUE-TODO).
//
// 48 entries: NULL plus the 47 explicit names through APNPC5. Consumed
// by the bytecode compiler's LoadMap call in pkg/pack/compiler (NAI-202
// runServerCompiler).
var NpcModeMap = map[string]int{
	"NULL":            NpcModeNull,
	"NONE":            NpcModeNone,
	"WANDER":          NpcModeWander,
	"PATROL":          NpcModePatrol,
	"PLAYERESCAPE":    NpcModePlayerEscape,
	"PLAYERFOLLOW":    NpcModePlayerFollow,
	"PLAYERFACE":      NpcModePlayerFace,
	"PLAYERFACECLOSE": NpcModePlayerFaceClose,
	"OPPLAYER1":       NpcModeOpPlayer1,
	"OPPLAYER2":       NpcModeOpPlayer2,
	"OPPLAYER3":       NpcModeOpPlayer3,
	"OPPLAYER4":       NpcModeOpPlayer4,
	"OPPLAYER5":       NpcModeOpPlayer5,
	"APPLAYER1":       NpcModeApPlayer1,
	"APPLAYER2":       NpcModeApPlayer2,
	"APPLAYER3":       NpcModeApPlayer3,
	"APPLAYER4":       NpcModeApPlayer4,
	"APPLAYER5":       NpcModeApPlayer5,
	"OPLOC1":          NpcModeOpLoc1,
	"OPLOC2":          NpcModeOpLoc2,
	"OPLOC3":          NpcModeOpLoc3,
	"OPLOC4":          NpcModeOpLoc4,
	"OPLOC5":          NpcModeOpLoc5,
	"APLOC1":          NpcModeApLoc1,
	"APLOC2":          NpcModeApLoc2,
	"APLOC3":          NpcModeApLoc3,
	"APLOC4":          NpcModeApLoc4,
	"APLOC5":          NpcModeApLoc5,
	"OPOBJ1":          NpcModeOpObj1,
	"OPOBJ2":          NpcModeOpObj2,
	"OPOBJ3":          NpcModeOpObj3,
	"OPOBJ4":          NpcModeOpObj4,
	"OPOBJ5":          NpcModeOpObj5,
	"APOBJ1":          NpcModeApObj1,
	"APOBJ2":          NpcModeApObj2,
	"APOBJ3":          NpcModeApObj3,
	"APOBJ4":          NpcModeApObj4,
	"APOBJ5":          NpcModeApObj5,
	"OPNPC1":          NpcModeOpNpc1,
	"OPNPC2":          NpcModeOpNpc2,
	"OPNPC3":          NpcModeOpNpc3,
	"OPNPC4":          NpcModeOpNpc4,
	"OPNPC5":          NpcModeOpNpc5,
	"APNPC1":          NpcModeApNpc1,
	"APNPC2":          NpcModeApNpc2,
	"APNPC3":          NpcModeApNpc3,
	"APNPC4":          NpcModeApNpc4,
	"APNPC5":          NpcModeApNpc5,
}
```

**Implementer cross-check before running tests:** open `LostCityRS/Engine-TS/src/engine/entity/NpcMode.ts:99-146` side-by-side and verify the 48 entries match name-for-name and the NpcMode* constant numbering matches TS-line ordering. Single-digit suffix mismatches (e.g., `OPPLAYER1` vs `OPPLAYER2`) are the highest-likelihood typo class.

- [ ] **Step 2.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run "TestNpcMode" -v
```

Expected: PASS for `TestNpcModeMap_Parity`, `TestNpcModeMap_QueueEntriesOmitted`, `TestNpcMode_IndexValues`.

- [ ] **Step 2.5 — Full package test (no regressions)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```

Expected: PASS.

- [ ] **Step 2.6 — Commit**

```bash
git add pkg/objtype/npcmode.go pkg/objtype/npcmode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-201 T2 — NpcMode constants + NpcModeMap

Mirrors TS src/engine/entity/NpcMode.ts. 68 NpcMode* constants (NULL=-1
through QUEUE20=66); 48 NpcModeMap entries (NULL through APNPC5). The 20
QUEUE1..QUEUE20 name-string entries are omitted per
NAI-201-D-NPCMODE-QUEUE-TODO (TS comments them out as "// TODO: these
are not used?"). NpcModeQueue* constants are retained for the NPC AI
state machine port (out of NAI-201 scope).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `ScriptOpcodeMap`

**Files:**
- Create: `pkg/script/opcode_map.go`
- Create: `pkg/script/opcode_map_test.go`

**TS source:** `src/engine/script/ScriptOpcode.ts:445-857` (the `ScriptOpcodeMap` literal — 393 entries).

**Spec:** §5.4, §7.4, §7.5, §7.6.

### Step 3.1 — Write the failing structural tests

Create `pkg/script/opcode_map_test.go`:

```go
package script

import (
	"strings"
	"testing"
)

// TestScriptOpcodeMap_LengthParity pins spec §7.4: 393 entries verified
// at plan-write against TS ScriptOpcode.ts via
//   awk '/^export const ScriptOpcodeMap/,/^]\)/' ScriptOpcode.ts |
//   grep -c "^\s*\['"
// If TS upstream adds opcodes, this count rises and the test fails —
// implementer updates the count after re-running the awk against
// LostCityRS/Engine-TS HEAD.
func TestScriptOpcodeMap_LengthParity(t *testing.T) {
	const wantLen = 393
	if got := len(ScriptOpcodeMap); got != wantLen {
		t.Fatalf("len(ScriptOpcodeMap) = %d, want %d (re-verify against TS ScriptOpcode.ts:445)", got, wantLen)
	}
}

// TestScriptOpcodeMap_NoDuplicates pins spec §7.6: no two distinct
// uppercase names map to the same Opcode value. Catches copy-paste
// regression during the 393-entry literal port.
func TestScriptOpcodeMap_NoDuplicates(t *testing.T) {
	seen := make(map[Opcode]string, len(ScriptOpcodeMap))
	for name, op := range ScriptOpcodeMap {
		if other, dup := seen[op]; dup {
			t.Errorf("Opcode %d mapped from BOTH %q and %q", op, other, name)
		}
		seen[op] = name
	}
}

// TestScriptOpcodeMap_NamesUppercase pins the convention that every
// key is ALL-UPPERCASE (no mixed case). TS uses UPPER_SNAKE_CASE
// uniformly; goscape mirrors. Catches typos like "Push_constant_int".
func TestScriptOpcodeMap_NamesUppercase(t *testing.T) {
	for name := range ScriptOpcodeMap {
		if name != strings.ToUpper(name) {
			t.Errorf("name %q is not uppercase (TS source is UPPER_SNAKE_CASE)", name)
		}
		if name == "" {
			t.Errorf("empty key in ScriptOpcodeMap")
		}
	}
}

// TestScriptOpcodeMap_SpotChecks pins spec §7.5: ~12 representative
// entries from across the file. Non-exhaustive by design —
// LengthParity + NoDuplicates anchor the overall shape; individual
// entry typos surface during NAI-202 driver tests against real .pack
// data.
//
// Each pair below is verified at plan-write against TS ScriptOpcode.ts
// line numbers in the test message. The goscape Op* constant name is
// looked up via `grep -n "^\tOp[A-Za-z]*" pkg/script/opcode.go` —
// NOT mechanically derived from the TS name (e.g., FINDUID →
// OpFindUID, not OpFindUid).
func TestScriptOpcodeMap_SpotChecks(t *testing.T) {
	cases := []struct {
		name string
		want Opcode
	}{
		{"PUSH_CONSTANT_INT", OpPushConstantInt},     // TS:446 (opcode 0; first entry)
		{"PUSH_CONSTANT_STRING", OpPushConstantString}, // TS:449 (non-contiguous numbering)
		{"BRANCH", OpBranch},                         // core control flow
		{"RETURN", OpReturn},                         // TS opcode 21 (gap handling)
		{"GOSUB", OpGosub},                           // core control flow
		{"JUMP", OpJump},                             // core control flow
		{"ALLOWDESIGN", OpAllowDesign},               // Player op (~2001)
		{"ANIM", OpAnim},                             // Player op
		{"FINDUID", OpFindUID},                       // Player op; verifies OpFindUID acronym casing
		{"HUNTALL", OpHuntAll},                       // Server op
		{"HUNTNEXT", OpHuntNext},                     // Server op
		{"GETTIMESPENT", OpGetTimeSpent},             // custom debug (~10002); end-of-enum smoke
		{"TIMESPENT", OpTimeSpent},                   // custom debug
	}
	for _, c := range cases {
		got, present := ScriptOpcodeMap[c.name]
		if !present {
			t.Errorf("ScriptOpcodeMap[%q]: missing", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("ScriptOpcodeMap[%q]: got %d, want %d", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 3.2 — Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestScriptOpcodeMap"
```

Expected: build error — `undefined: ScriptOpcodeMap`.

### Step 3.3 — Write `opcode_map.go`

Create `pkg/script/opcode_map.go` with the literal `ScriptOpcodeMap` declaration. **Implementer instructions:**

1. Open TS `LostCityRS/Engine-TS/src/engine/script/ScriptOpcode.ts:445-857` in one pane.
2. Open `pkg/script/opcode.go` in another pane (for looking up goscape Op* names).
3. For each TS line `['NAME', ScriptOpcode.NAME],`, write one Go line `"NAME": OpFooBar,` where `OpFooBar` is the goscape constant name from `pkg/script/opcode.go` that has the matching numeric value.
4. **Goscape naming irregularities** observed at plan-write:
   - `FINDUID` → `OpFindUID` (all-caps acronym)
   - `P_FINDUID` → `OpPFindUID`
   - `NPC_FINDUID` → `OpNpcFindUID`
   - `UID` → `OpUID`
   - `NPC_UID` → `OpNpcUID`
   - `GETTIMESPENT` → `OpGetTimeSpent` (CamelCase split with `Spent` capitalized)
   - `TIMESPENT` → `OpTimeSpent`
   - All other names follow mechanical UPPER_SNAKE → OpUpperCamel (`PUSH_CONSTANT_INT` → `OpPushConstantInt`).
5. Preserve TS section-header comments (`// Player ops`, `// Server ops`, etc.) for review parity.

File skeleton:

```go
package script

// ScriptOpcodeMap maps uppercase opcode name → Opcode value. Mirrors TS
// ScriptOpcodeMap (src/engine/script/ScriptOpcode.ts:445-857). Consumed
// by the bytecode compiler's allCommands construction in
// pkg/pack/compiler (NAI-202 runServerCompiler).
//
// Naming: TS UPPER_SNAKE_CASE → goscape Op* constant in
// pkg/script/opcode.go. Most names follow mechanical UPPER_SNAKE →
// OpUpperCamel translation; the goscape file uses all-caps for
// acronyms (UID, ID, …) so e.g. FINDUID → OpFindUID, GETTIMESPENT →
// OpGetTimeSpent.
//
// Insertion order: Go map iteration is randomized. TS Map<string,
// number> is insertion-ordered, but runServerCompiler sorts by opcode
// value before iteration (Compiler.ts:111). Goscape iteration order
// does not matter to consumers.
var ScriptOpcodeMap = map[string]Opcode{
	"PUSH_CONSTANT_INT":    OpPushConstantInt,
	"PUSH_VARP":            OpPushVarp,
	"POP_VARP":             OpPopVarp,
	"PUSH_CONSTANT_STRING": OpPushConstantString,
	"PUSH_VARN":            OpPushVarn,
	"POP_VARN":             OpPopVarn,
	"BRANCH":               OpBranch,
	"BRANCH_NOT":           OpBranchNot,
	"BRANCH_EQUALS":        OpBranchEquals,
	"BRANCH_LESS_THAN":     OpBranchLessThan,
	"BRANCH_GREATER_THAN":  OpBranchGreaterThan,
	"PUSH_VARS":            OpPushVars,
	"POP_VARS":             OpPopVars,

	"RETURN": OpReturn,
	"GOSUB":  OpGosub,
	"JUMP":   OpJump,
	"SWITCH": OpSwitch,

	// ... continue line-for-line through TS ScriptOpcode.ts:451-857 ...

	"GETTIMESPENT": OpGetTimeSpent,
	"TIMESPENT":    OpTimeSpent,
}
```

The skeleton above shows the first ~16 entries verbatim. **The implementer must port all 393 entries**, preserving TS line ordering and section-header comments.

- [ ] **Step 3.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestScriptOpcodeMap" -v
```

Expected: PASS for all four `TestScriptOpcodeMap_*` tests. If `TestScriptOpcodeMap_LengthParity` fails with `got = 392`, an entry was dropped; if `got = 394`, an extra entry was added; if `TestScriptOpcodeMap_NoDuplicates` fails, two names collapsed to the same Opcode (copy-paste error).

- [ ] **Step 3.5 — Full package test (no regressions)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS.

- [ ] **Step 3.6 — Commit**

```bash
git add pkg/script/opcode_map.go pkg/script/opcode_map_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-201 T3 — ScriptOpcodeMap (name → Opcode)

Mirrors TS src/engine/script/ScriptOpcode.ts:445-857. 393 entries
mapping uppercase opcode name (e.g. "PUSH_CONSTANT_INT") to the goscape
Op* constant (OpPushConstantInt). Goscape naming has all-caps acronyms
(OpFindUID, OpGetTimeSpent — see in-file note) so the port is by
goscape constant lookup, not mechanical UPPER_SNAKE translation.

Consumed by NAI-202's runServerCompiler.allCommands construction.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `Pointers` struct + `PointerGroupFind` + `corruptExceptActive` helper

**Files:**
- Create: `pkg/script/opcode_pointers.go` (initial — types + helper + empty map)
- Create: `pkg/script/opcode_pointers_test.go` (helper + PointerGroupFind tests)

**TS source:** `src/engine/script/ScriptOpcodePointers.ts:1-15` (constant + type signature).

**Spec:** §5.5 (struct + helper definitions); §7.11 (`PointerGroupFind` order pin).

### Step 4.1 — Write the failing helper / constant tests

Create `pkg/script/opcode_pointers_test.go`:

```go
package script

import (
	"reflect"
	"testing"
)

// TestPointerGroupFind_Content pins spec §7.11: 5 elements in TS order
// (find_player, find_npc, find_loc, find_obj, find_db). Order matters
// because corrupt-slice content is concatenated in this order.
func TestPointerGroupFind_Content(t *testing.T) {
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db"}
	if !reflect.DeepEqual(PointerGroupFind, want) {
		t.Fatalf("PointerGroupFind\n got = %v\nwant = %v", PointerGroupFind, want)
	}
}

// TestCorruptExceptActive_Behavior pins the helper's contract: returns
// PointerGroupFind ++ extras as a fresh slice (caller mutations must
// not aliase PointerGroupFind).
func TestCorruptExceptActive_Behavior(t *testing.T) {
	got := corruptExceptActive("last_com", "last_int")
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db", "last_com", "last_int"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got = %v, want = %v", got, want)
	}

	// Independence: mutating the returned slice must not corrupt
	// PointerGroupFind.
	got[0] = "MUTATED"
	if PointerGroupFind[0] != "find_player" {
		t.Fatalf("PointerGroupFind[0] = %q after caller mutation; want %q (helper must return a fresh slice)", PointerGroupFind[0], "find_player")
	}
}

// TestPointers_ZeroValue pins that a missing entry returns a useful
// zero value (all-nil slices, Conditional=false) — this is what
// callers do when ScriptOpcodePointers[op] miss returns. Matches TS
// `undefined` semantics via Go map miss.
func TestPointers_ZeroValue(t *testing.T) {
	var p Pointers
	if p.Require != nil || p.Require2 != nil || p.Set != nil || p.Set2 != nil ||
		p.Corrupt != nil || p.Corrupt2 != nil || p.Conditional {
		t.Fatalf("Pointers{} should be all-zero; got %+v", p)
	}
}
```

- [ ] **Step 4.2 — Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestPointer|TestCorrupt"
```

Expected: build error — `undefined: PointerGroupFind`, `undefined: corruptExceptActive`, `undefined: Pointers`.

### Step 4.3 — Write `opcode_pointers.go` (types + helper + empty map)

Create `pkg/script/opcode_pointers.go`:

```go
package script

// PointerGroupFind is the 5-element list of find_* pointer names that
// many opcodes spread into their corrupt list. Mirrors TS
// POINTER_GROUP_FIND (ScriptOpcodePointers.ts:3).
//
// Order matters: corrupt-slice content is concatenated in this exact
// order on all 6 TS spread sites.
var PointerGroupFind = []string{
	"find_player", "find_npc", "find_loc", "find_obj", "find_db",
}

// Pointers holds the pointer-gate flags for one script opcode. Mirrors
// the inline TS type at ScriptOpcodePointers.ts:5-14.
//
// Field semantics:
//   - Require / Require2: pointer names that MUST be set when the opcode
//     executes. Variant *2 applies in 2-active-entity contexts.
//   - Set / Set2: pointer names the opcode SETS on success.
//   - Corrupt / Corrupt2: pointer names the opcode invalidates.
//   - Conditional: true if Set takes effect only on a successful branch
//     (e.g., FINDUID conditional on lookup hit).
//
// Nil slice == "no entries" (matches TS optional-field omitted). Map
// miss on ScriptOpcodePointers returns the Pointers zero-value (all-nil
// slices, Conditional=false), which is the goscape equivalent of TS
// `ScriptOpcodePointers[opcode]` returning `undefined` — both mean
// "no constraints".
type Pointers struct {
	Require     []string
	Require2    []string
	Set         []string
	Set2        []string
	Corrupt     []string
	Corrupt2    []string
	Conditional bool
}

// corruptExceptActive returns PointerGroupFind ++ extras as a fresh
// slice. Mirrors TS spread pattern `[...POINTER_GROUP_FIND, ...extras]`
// used in 4 simple-spread entries:
//   - P_ARRIVEDELAY     (ScriptOpcodePointers.ts:286)
//   - P_COUNTDIALOG     (ScriptOpcodePointers.ts:301)
//   - P_DELAY           (ScriptOpcodePointers.ts:314)
//   - P_PAUSEBUTTON     (ScriptOpcodePointers.ts:370)
//
// TWO additional sites use a longer prefix
// (`['p_active_player', 'p_active_player2', ...POINTER_GROUP_FIND,
// 'last_com', ...]`):
//   - NPC_DELAY        (ScriptOpcodePointers.ts:569)
//   - NPC_ARRIVEDELAY  (ScriptOpcodePointers.ts:711)
// Those two are ported as literal slice expansions (NOT via
// corruptExceptActive) because the prefix breaks the helper symmetry.
// The pin test in opcode_pointers_test.go anchors both shapes — see
// NAI-201-D-POINTERS-SPREAD-HELPER.
func corruptExceptActive(extras ...string) []string {
	out := make([]string, 0, len(PointerGroupFind)+len(extras))
	out = append(out, PointerGroupFind...)
	out = append(out, extras...)
	return out
}

// ScriptOpcodePointers maps Opcode → Pointers describing the
// pointer-gate flags consumed by the bytecode compiler's typechecker
// (NAI-203+ arc). Mirrors TS ScriptOpcodePointers
// (ScriptOpcodePointers.ts:1-984).
//
// Opcodes not listed here have an absent / empty Pointers (TS:
// `ScriptOpcodePointers[opcode]` returns undefined, treated as "no
// constraints"). Mirrored in goscape via map miss (zero-value
// Pointers{}).
//
// 237 entries; verified by TestScriptOpcodePointers_LengthParity in T5.
// Entry order in this literal mirrors TS line ordering to support
// side-by-side review per [[flat_arg_signature_for_cross_lang_parity]];
// Go map iteration order itself is randomized but unobservable to
// callers.
var ScriptOpcodePointers = map[Opcode]Pointers{
	// T5 populates this with 237 entries from TS ScriptOpcodePointers.ts:17-981.
}
```

The empty `ScriptOpcodePointers` declaration above is intentional. T5 fills it. T4 lands only the type + helper + PointerGroupFind so this commit is testable in isolation.

- [ ] **Step 4.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestPointer|TestCorrupt" -v
```

Expected: PASS for `TestPointerGroupFind_Content`, `TestCorruptExceptActive_Behavior`, `TestPointers_ZeroValue`.

- [ ] **Step 4.5 — Full package test (no regressions)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS. (T3's `ScriptOpcodeMap` tests should still pass.)

- [ ] **Step 4.6 — Commit**

```bash
git add pkg/script/opcode_pointers.go pkg/script/opcode_pointers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-201 T4 — Pointers struct + helper scaffolding

Mirrors TS src/engine/script/ScriptOpcodePointers.ts:1-15. Types,
PointerGroupFind constant, corruptExceptActive helper. ScriptOpcodePointers
itself is declared empty; T5 populates it with 237 entries.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `ScriptOpcodePointers` data port (237 entries)

**Files:**
- Modify: `pkg/script/opcode_pointers.go` (replace empty literal body with 237 entries)
- Modify: `pkg/script/opcode_pointers_test.go` (add length-parity + spot-check tests)

**TS source:** `src/engine/script/ScriptOpcodePointers.ts:17-981` (237 entries).

**Spec:** §5.5; §7.7 (length parity); §7.8 (spot checks).

### Step 5.1 — Append failing length-parity + spot-check tests

Append to `pkg/script/opcode_pointers_test.go`:

```go
// TestScriptOpcodePointers_LengthParity pins spec §7.7: 237 entries
// verified at plan-write via
//   grep -c "\[ScriptOpcode\." src/engine/script/ScriptOpcodePointers.ts
// If TS upstream adds entries this count rises; implementer updates
// after re-running the grep.
func TestScriptOpcodePointers_LengthParity(t *testing.T) {
	const wantLen = 237
	if got := len(ScriptOpcodePointers); got != wantLen {
		t.Fatalf("len(ScriptOpcodePointers) = %d, want %d (re-verify against TS ScriptOpcodePointers.ts)", got, wantLen)
	}
}

// TestScriptOpcodePointers_SpotChecks pins spec §7.8: representative
// entries across the file. Each case below was verified at plan-write
// against the TS line cited in the case label.
func TestScriptOpcodePointers_SpotChecks(t *testing.T) {
	cases := []struct {
		op   Opcode
		want Pointers
		desc string
	}{
		{
			op:   OpAllowDesign,
			want: Pointers{Require: []string{"active_player"}},
			desc: "TS:17 — simplest Require-only shape",
		},
		{
			op: OpAnim,
			want: Pointers{
				Require:  []string{"active_player"},
				Require2: []string{"active_player2"},
			},
			desc: "TS:20 — Require + Require2",
		},
		{
			op: OpFindUID,
			want: Pointers{
				Set:         []string{"active_player"},
				Set2:        []string{"active_player2"},
				Conditional: true,
			},
			desc: "TS:103 — Set + Set2 + Conditional=true",
		},
		{
			op:   OpHuntAll,
			want: Pointers{Set: []string{"find_player"}},
			desc: "TS:140 — Set-only shape",
		},
		{
			op: OpHuntNext,
			want: Pointers{
				Require:     []string{"find_player"},
				Require2:    []string{"find_player"},
				Set:         []string{"active_player"},
				Set2:        []string{"active_player2"},
				Conditional: true,
			},
			desc: "TS:143 — full quartet + Conditional",
		},
		{
			op: OpPArriveDelay,
			want: Pointers{
				Require: []string{"p_active_player"},
				Corrupt: corruptExceptActive(
					"last_com", "last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				),
			},
			desc: "TS:282 — simple POINTER_GROUP_FIND spread via helper",
		},
		{
			op: OpPPauseButton,
			want: Pointers{
				Require: []string{"p_active_player"},
				Set:     []string{"last_com"},
				Corrupt: corruptExceptActive(
					"last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				),
			},
			desc: "TS:365 — Require + Set + simple spread",
		},
		{
			op: OpNpcDelay,
			want: Pointers{
				Require:  []string{"active_npc"},
				Require2: []string{"active_npc2"},
				Corrupt: []string{
					"p_active_player", "p_active_player2",
					"find_player", "find_npc", "find_loc", "find_obj", "find_db",
					"last_com", "last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				},
			},
			desc: "TS:567 — extended spread (literal expansion, NOT helper)",
		},
		{
			op: OpNpcArriveDelay,
			want: Pointers{
				Require:  []string{"active_npc"},
				Require2: []string{"active_npc2"},
				Corrupt: []string{
					"p_active_player", "p_active_player2",
					"find_player", "find_npc", "find_loc", "find_obj", "find_db",
					"last_com", "last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				},
			},
			desc: "TS:709 — extended spread (literal expansion, NOT helper)",
		},
		{
			op:   OpDbListAll,
			want: Pointers{Set: []string{"find_db"}},
			desc: "TS:976 — late-file entry (Db family)",
		},
		{
			op:   OpDbListAllWithCount,
			want: Pointers{Set: []string{"find_db"}},
			desc: "TS:979 — last entry in TS",
		},
	}
	for _, c := range cases {
		got, present := ScriptOpcodePointers[c.op]
		if !present {
			t.Errorf("%s: ScriptOpcodePointers[op=%d]: missing", c.desc, c.op)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got = %+v\nwant = %+v", c.desc, got, c.want)
		}
	}
}
```

- [ ] **Step 5.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestScriptOpcodePointers_(LengthParity|SpotChecks)"
```

Expected: `TestScriptOpcodePointers_LengthParity` fails with `len(ScriptOpcodePointers) = 0, want 237`; `TestScriptOpcodePointers_SpotChecks` fails with missing-key errors on every spot-check.

### Step 5.3 — Populate `ScriptOpcodePointers` with all 237 entries

Modify `pkg/script/opcode_pointers.go` — replace the empty literal body with the 237 entries from TS `ScriptOpcodePointers.ts:17-981`.

**Implementer instructions:**

1. Open TS `LostCityRS/Engine-TS/src/engine/script/ScriptOpcodePointers.ts:17-981` in one pane.
2. Open `pkg/script/opcode.go` in another pane (for Op* name lookups; same naming irregularities as T3 apply — `FINDUID` → `OpFindUID`, `GETTIMESPENT` → `OpGetTimeSpent`, etc.).
3. For each TS block:
   ```ts
   [ScriptOpcode.NAME]: {
       require: ['active_player'],
       require2: ['active_player2'],
       ...
   },
   ```
   write a Go entry:
   ```go
   OpName: {
       Require:  []string{"active_player"},
       Require2: []string{"active_player2"},
   },
   ```
4. **Spread-site translation**:
   - **Simple spread** (TS lines 286, 301, 314, 370 — P_ARRIVEDELAY, P_COUNTDIALOG, P_DELAY, P_PAUSEBUTTON): use the helper
     ```go
     Corrupt: corruptExceptActive("last_com", "last_int", ...),
     ```
   - **Extended spread** (TS lines 569, 711 — NPC_DELAY, NPC_ARRIVEDELAY): use literal expansion
     ```go
     Corrupt: []string{
         "p_active_player", "p_active_player2",
         "find_player", "find_npc", "find_loc", "find_obj", "find_db",
         "last_com", "last_int", "last_item", "last_slot",
         "last_targetslot", "last_useitem", "last_useslot",
     },
     ```
5. Preserve TS section-header comments (`// Player ops`, `// Npc ops`, `// Loc ops`, etc.) inline for side-by-side review.
6. **Single-line entries** with one field (most common shape) are formatted on one line:
   ```go
   OpAllowDesign: {Require: []string{"active_player"}},
   ```
   Multi-field entries break across lines.
7. Field-order convention inside `Pointers{}` literal: `Require`, `Require2`, `Set`, `Set2`, `Corrupt`, `Corrupt2`, `Conditional`. (Mirrors the struct declaration order in T4.)
8. Omit zero-value fields (don't write `Set: nil` or `Conditional: false`).

The file becomes ~500-600 LOC. The implementer should commit ONCE at the end of the data port — the partial-state intermediate isn't independently testable.

- [ ] **Step 5.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestScriptOpcodePointers" -v
```

Expected: PASS for `TestScriptOpcodePointers_LengthParity` and `TestScriptOpcodePointers_SpotChecks`. If LengthParity fails:
- `got < 237` → entries dropped during port; cross-check TS line count.
- `got > 237` → duplicate Opcode key (Go map silently overwrites; the count goes UP only if a key was misspelled to a non-existing duplicate — unlikely with `Op*` constants since the compiler rejects undefined names. More likely failure mode: a TS entry was ported twice).

If SpotChecks fails on a specific entry: re-read the corresponding TS line(s) and confirm field-by-field.

- [ ] **Step 5.5 — Full package test (no regressions)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS.

- [ ] **Step 5.6 — Run with `-race`**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script/...
```

Expected: PASS. (No goroutines in NAI-201; -race is paranoid hygiene per spec §13.)

- [ ] **Step 5.7 — Commit**

```bash
git add pkg/script/opcode_pointers.go pkg/script/opcode_pointers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-201 T5 — ScriptOpcodePointers data port (237 entries)

Mirrors TS src/engine/script/ScriptOpcodePointers.ts:17-981. 237 entries
across Player / Npc / Loc / Obj / Db / Server op families. 4 entries
use the corruptExceptActive helper for the simple POINTER_GROUP_FIND
spread (P_ARRIVEDELAY, P_COUNTDIALOG, P_DELAY, P_PAUSEBUTTON); 2 use
literal expansion for the extended spread with p_active_player prefix
(NPC_DELAY, NPC_ARRIVEDELAY) — see NAI-201-D-POINTERS-SPREAD-HELPER.

Consumed by NAI-203+'s bytecode typechecker.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Cross-registry validity tests + spread-site pin

**Files:**
- Modify: `pkg/script/opcode_pointers_test.go` (append validity + spread-site pin)
- Create: `pkg/script/nai201_deviation_pins_test.go` (deviation-tag grep pins)

**Spec:** §5.6 (cross-registry validity); §7.9, §7.10, §7.12.

### Step 6.1 — Append validity + spread-site tests

Append to `pkg/script/opcode_pointers_test.go`:

```go
// TestScriptOpcodePointers_KeysAreBoundedOpcodes pins spec §7.9: every
// ScriptOpcodePointers key is in the valid Opcode range — i.e. ≤ the
// max Op* constant defined in pkg/script/opcode.go. Enumerating all
// 394 Op* constants in this test would be brittle; the weaker bound
// (≤ OpTimeSpent, the highest goscape constant at HEAD `1b25104`)
// catches typo cases that would assign a wildly-out-of-range value.
//
// If pkg/script/opcode.go adds a new Op* constant with value >
// OpTimeSpent, update this constant.
func TestScriptOpcodePointers_KeysAreBoundedOpcodes(t *testing.T) {
	const maxOp = OpTimeSpent // 10003 at HEAD 1b25104
	for op := range ScriptOpcodePointers {
		if op > maxOp {
			t.Errorf("ScriptOpcodePointers[op=%d]: exceeds known max Op* = %d", op, maxOp)
		}
	}
}

// TestScriptOpcodePointers_CorruptExceptActiveCallSites pins deviation
// NAI-201-D-POINTERS-SPREAD-HELPER. Spec §7.10 asserts BOTH:
//   (a) the helper is called exactly 4 times in opcode_pointers.go
//       (matching TS simple-spread sites at lines 286, 301, 314, 370),
//   (b) the 2 extended-spread entries (NPC_DELAY, NPC_ARRIVEDELAY)
//       contain the expected 14-element Corrupt slice via literal
//       expansion.
//
// If a future entry adds another spread site, the count check fails
// and the author updates after re-grepping TS.
func TestScriptOpcodePointers_CorruptExceptActiveCallSites(t *testing.T) {
	// (a) Helper-call count.
	src, err := readPkgFile(t, "opcode_pointers.go")
	if err != nil {
		t.Fatal(err)
	}
	const wantHelperCalls = 4
	got := strings.Count(src, "corruptExceptActive(")
	if got != wantHelperCalls {
		t.Errorf("corruptExceptActive( call count in opcode_pointers.go: got %d, want %d (re-verify TS POINTER_GROUP_FIND simple-spread sites)", got, wantHelperCalls)
	}

	// (b) Extended-spread entries: NPC_DELAY and NPC_ARRIVEDELAY share
	// the same 14-element Corrupt slice shape. Pin the exact contents.
	wantExtendedCorrupt := []string{
		"p_active_player", "p_active_player2",
		"find_player", "find_npc", "find_loc", "find_obj", "find_db",
		"last_com", "last_int", "last_item", "last_slot",
		"last_targetslot", "last_useitem", "last_useslot",
	}
	for _, op := range []Opcode{OpNpcDelay, OpNpcArriveDelay} {
		entry := ScriptOpcodePointers[op]
		if !reflect.DeepEqual(entry.Corrupt, wantExtendedCorrupt) {
			t.Errorf("Op=%d extended-spread Corrupt: got %v, want %v", op, entry.Corrupt, wantExtendedCorrupt)
		}
	}
}

// readPkgFile reads a source file from this package's directory. Used
// by the spread-site count assertion above. Uses os.ReadFile relative
// to the test binary's working dir, which `go test` runs from the
// package directory — so the bare filename suffices.
func readPkgFile(t *testing.T, filename string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
```

**Important:** the new `TestScriptOpcodePointers_CorruptExceptActiveCallSites` test uses `os.ReadFile`. Add `"os"` and `"strings"` to the import block of `opcode_pointers_test.go` if not already present.

### Step 6.2 — Create the deviation-pin test file

Create `pkg/script/nai201_deviation_pins_test.go`:

```go
package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNAI201Deviations_Pinned pins spec §7.12: each NAI-201 deviation
// tag has at least one in-source reference. Tracks
// [[retire_deviation_grep_all_comments]] convention — if a tag is
// retired, the test fails and the implementer reviews scope before
// updating the wantTags map.
//
// Greps both pkg/script/ (Pointers / ScriptOpcodeMap-side deviations)
// and pkg/objtype/ (NpcMode-side deviation). Implementer adjusts the
// grep root if a new deviation lands in a different package.
func TestNAI201Deviations_Pinned(t *testing.T) {
	wantTags := []string{
		"NAI-201-D-NPCMODE-QUEUE-TODO",
		"NAI-201-D-POINTERS-SPREAD-HELPER",
	}

	roots := []string{".", "../objtype"}

	counts := make(map[string]int, len(wantTags))
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			s := string(data)
			for _, tag := range wantTags {
				counts[tag] += strings.Count(s, tag)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	for _, tag := range wantTags {
		if counts[tag] < 1 {
			t.Errorf("deviation tag %q: 0 references found; want >=1 (search roots: pkg/script/, pkg/objtype/)", tag)
		}
	}
}
```

- [ ] **Step 6.3 — Run new tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestScriptOpcodePointers_KeysAreBoundedOpcodes|TestScriptOpcodePointers_CorruptExceptActiveCallSites|TestNAI201Deviations_Pinned" -v
```

Expected: PASS for all three.

If `TestScriptOpcodePointers_CorruptExceptActiveCallSites` fails on the count check with `got = N, want 4`: the implementer ported N spread sites via the helper instead of the expected 4. Re-verify against TS lines 286, 301, 314, 370 (simple) vs 569, 711 (extended).

If `TestNAI201Deviations_Pinned` fails: the deviation tag is not present in the expected location. Confirm doc-comments in `pkg/objtype/npcmode.go` (NPCMODE-QUEUE-TODO) and `pkg/script/opcode_pointers.go` (POINTERS-SPREAD-HELPER) contain the literal tag strings.

- [ ] **Step 6.4 — Full repository test sweep**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. No regressions anywhere — NAI-201 lands no production wiring, so only the new tests should change pass/fail state.

- [ ] **Step 6.5 — Run with `-race`**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script/... ./pkg/objtype/...
```

Expected: PASS.

- [ ] **Step 6.6 — Commit**

```bash
git add pkg/script/opcode_pointers_test.go pkg/script/nai201_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-201 T6 — cross-registry validity + deviation pins

Adds three tests to lock the NAI-201 deviations and cross-registry
invariants:
  - TestScriptOpcodePointers_KeysAreBoundedOpcodes — bound check on
    Pointers keys vs known max Op* constant.
  - TestScriptOpcodePointers_CorruptExceptActiveCallSites — pins both
    the helper-call count (4) and the literal-expansion entry contents
    (NPC_DELAY, NPC_ARRIVEDELAY) per NAI-201-D-POINTERS-SPREAD-HELPER.
  - TestNAI201Deviations_Pinned — greps pkg/script/ + pkg/objtype/ for
    each NAI-201 deviation tag; >=1 reference required.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Close — verify full test suite + close commit

**Files:** No source changes. Commit is metadata-only (close ceremony per `[[close_commit_memory_trailer]]`).

### Step 7.1 — Final verification sweep

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: PASS / no output / no errors.

- [ ] **Step 7.2 — Audit deliverables against acceptance criteria (spec §13)**

Run each check, log result inline:

```bash
test -f pkg/objtype/npcstat.go && echo "OK: npcstat.go"
test -f pkg/objtype/npcmode.go && echo "OK: npcmode.go"
test -f pkg/script/opcode_map.go && echo "OK: opcode_map.go"
test -f pkg/script/opcode_pointers.go && echo "OK: opcode_pointers.go"

grep -c "^\s*\"" pkg/objtype/npcstat.go     # ≥ 6 (map entries)
grep -c "^\s*\"" pkg/objtype/npcmode.go     # ≥ 48 (map entries)
grep -c "^\s*\"" pkg/script/opcode_map.go   # ≥ 393 (map entries)
grep -c "Op[A-Z][A-Za-z]*: " pkg/script/opcode_pointers.go   # ≥ 237 (struct-literal entry starters)

grep -c "NAI-201-D-NPCMODE-QUEUE-TODO" pkg/objtype/ pkg/script/ -r   # ≥ 1
grep -c "NAI-201-D-POINTERS-SPREAD-HELPER" pkg/objtype/ pkg/script/ -r   # ≥ 1
```

(The `grep -c` heuristics on map entries may over- or under-count slightly depending on formatting; they're smoke checks, not authoritative. The Go tests in T1-T6 are authoritative.)

- [ ] **Step 7.3 — Empty close commit with memory trailer**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-201 — compiler-foundation registries complete (slice 1 of 3)

Lands NpcStatMap (6 entries) + NpcModeMap (48 entries) + ScriptOpcodeMap
(393 entries) + ScriptOpcodePointers (237 entries) with zero production
consumers. NAI-202 (runServerCompiler driver) is the first consumer.

Deviations:
  - NAI-201-D-NPCMODE-QUEUE-TODO: 20 QUEUE1..QUEUE20 NpcModeMap entries
    omitted (TS-faithful — TS comments them out as TODO).
  - NAI-201-D-POINTERS-SPREAD-HELPER: 4 simple-spread sites use the
    corruptExceptActive helper; 2 extended-spread sites use literal
    expansion (asymmetric prefix breaks helper symmetry).

Spec:  docs/superpowers/specs/2026-05-14-nai-201-compiler-foundation-registries-design.md
Plan:  docs/superpowers/plans/2026-05-14-nai-201-compiler-foundation-registries.md

Closes memory: scope_gate_prerequisite_chain spec_followup_tracker_freshness

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

The `Closes memory:` trailer per `[[close_commit_memory_trailer]]` lists the memory entries this sub-spec's outcomes most directly applied.

---

## Post-completion review checklist (controller, before NAI-202 dispatch)

1. **Test sweep is green.** `go test ./...` passes. `go vet ./...` clean.
2. **No production wiring.** `grep -rn "NpcStatMap\|NpcModeMap\|ScriptOpcodeMap\|ScriptOpcodePointers" modules/ cmd/` returns zero hits.
3. **Deviation tags grep-discoverable.** `grep -rn "NAI-201-D-" pkg/` returns ≥2 distinct tag mentions (one in pkg/objtype/, one in pkg/script/).
4. **Both spread-port shapes present.** `grep -c "corruptExceptActive(" pkg/script/opcode_pointers.go` returns 4; `grep -c "p_active_player2" pkg/script/opcode_pointers.go` returns ≥2 (one per extended-spread entry).
5. **Memory follow-ups review.** Anything surprising surfaced during implementation? If yes, save per `[[post_task_handoff]]`.
6. **Resume prompt for NAI-202.** Emit a fresh-session resume prompt per `[[superpowers_clear_between_spec_and_impl]]` listing the four registries now landed, the runServerCompiler scope, and the entity-type loader signatures the implementer will encounter.
