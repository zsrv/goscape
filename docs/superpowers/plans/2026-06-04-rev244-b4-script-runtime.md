# rev-244 Bundle 4: script runtime — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the B4 slice of the Engine-TS 225→244 delta — the full `ScriptOpcode.ts` renumber, handler-surface deltas, hunt-iterator unification, the IF_SETRECOL/IF_OPENOVERLAY deferral closures, and the world-side cycle-stats instrumentation backing the 13 new debug ops.

**Architecture:** Faithful TS→Go translation per `PORTING-LESSONS.md` (read first: `git show main:PORTING-LESSONS.md` — §3 gotchas, §4 citations, §5 gates). Renumber-first (user decision): one mechanical foundation task re-derives every opcode value from the 244 enum, then behavioral TDD slices, then world-surface tasks, then the audit. The TS cross-pin diff (`git -C /home/owner/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4 -- src/engine/script`) is the contract. All work lands on branch `rev-244`.

**Tech Stack:** Go 1.26 (modern idioms: `for range n`, `min`/`max`). Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix. Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: `-race` on touched packages (CGO_ENABLED=1). Every commit: `--no-gpg-sign` + Claude trailer.

**Spec:** `docs/superpowers/specs/2026-06-04-rev244-b4-script-runtime-design.md`

**References:** Engine-TS at the 244 pin: `/home/owner/Code/github.com/LostCityRS/Engine-TS` (checkout IS at `9aadcec4`).

**Scope decisions already made (do not relitigate):**
- **Cycle stats ported FULLY** (user decision) — real tick-section stopwatches + bandwidth counters, uint16-wrap faithful. Not stubs.
- **Renumber-first** (user decision) — Task 1 is all-or-nothing for the name→value table; the compiler (`pkg/pack/compiler/symbols.go`) consumes `script.ScriptOpcodeMap` + `script.ScriptOpcodePointers` and renumbers with it.
- B3-shipped hunks must NOT be double-applied: `OpenChat`/`OpenMainModalSide` renames (`d5a70fb1`), PROJANIM_PL pid (`fcc7e212`), `Player.OpenOverlay` + flush (`ebce9706`), `WealthEventParams.RecipientID` field (`07e44a61`).
- Verified NO-OPs (Task 13 documents them; do NOT "fix"): RANDOM/RANDOMINC clamp (JavaRandom.nextInt(0) returns 0 — power-of-two branch; `checkIsPositiveInt` rejects only n<0), STAT_RANDOM draw, GETQUEUE/CLEARQUEUE iteration style (B3 T12), handler file moves, ScriptFile STANDALONE_BUNDLE branch.
- script.dat numbering window closes at **B6** (extends rev244-b1-format-window; B3 user decision).
- Sandbox gotcha: `git status` shows phantom `??` dotfiles — device-node masks, NOT real files. Never stage them; never `git add -A`. Warn every subagent.

**Bake into every implementer prompt (recurring B2+B3 defects):**
1. Verify every `// TS <File>.ts:<lines>` citation against a numbered listing (`git -C /home/owner/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:<file> | cat -n | sed -n '<range>p'`) BEFORE writing.
2. Reject-path tests must seed earlier-gate prerequisites so the gate under test is the discriminating condition.
3. Final-review "missing X" findings can be false positives — verify directly before fixing.
4. Adding methods to `script.WorldVars`/`script.ActivePlayer` triggers test-fake compile cascades — follow PORTING.md §NEW-INTERFACE-METHOD-COMPILE-CASCADE.

---

## Slice 1 — Foundation: the renumber

### Task 1: Opcode renumber + renames + deletions + enum-only stubs

**Files:**
- Modify: `pkg/script/opcode.go` (every block from "Core language ops" down; `String()`)
- Modify: `pkg/script/opcode_map.go` (full table)
- Modify: `pkg/script/opcode_pointers.go` (renamed keys, removed IF_SETRECOL row, 4 new rows)
- Modify: `pkg/script/handlers.go` + `pkg/script/handlers_*.go` (table keys, renamed handler funcs, removed handlers)
- Modify: `pkg/script/handlers_b0_stubs.go` (delete varbit stubs; add 5 new enum-only stubs)
- Create: `pkg/script/opcode_map_244_pin_test.go` (generated table pin)
- Modify: every `pkg/script`/`pkg/pack/compiler` test pinning a numeric opcode value

The contract is the 244 enum + map: `git -C /home/owner/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:src/engine/script/ScriptOpcode.ts | cat -n` (enum lines 1-457, map lines 459-907).

Value anchors (sanity-check your derivation against these BEFORE editing — they were independently verified): `DISTANCE=1003, HUNTALL=1004, HUNTNEXT=1005, MAP_FINDSQUARE=1015, PROJANIM_NPC=1019, PROJANIM_PL=1020, SPLIT_INIT=1024, STRUCT_PARAM=1028, WORLD_DELAY=1029, NPCCOUNT=1030, ZONECOUNT=1031, LOCCOUNT=1032, OBJCOUNT=1033, MAP_MULTIWAY=1034, ALLOWDESIGN=2000, BAS_READYANIM=2002, BUFFER_FULL=2009, GETTIMER=2019, STAT_ADVANCE=2027, HINT_PLAYER=2033, IF_MULTIZONE=2037, IF_OPENOVERLAY=2041, PLAYER_FINDALLZONE=2091, PLAYER_FINDNEXT=2092, STAT=2101, IF_OPENMAINOVERLAY=2112, AFK_EVENT=2113, LOWMEMORY=2114, LAST_COORD=2126, STRONGQUEUEVARARG=2134, NPC_HUNTNEXT=2529, SPOTANIM_NPC=2542, NPC_INRANGE=2547, OBJ_FINDNEXT=3511, LC_LENGTH=4107, OC_WEIGHT=4216, INV_ALLSTOCK=4300, BOTH_MOVEINV=4318, BOTH_DROPSLOT=4328, INV_DEBUGNAME=4332, ERROR=10000, MAP_PRODUCTION=10001, MAP_LASTCLOCK=10002, MAP_LASTBANDWIDTHOUT=10013, TIMESPENT=10014, GETTIMESPENT=10015, CONSOLE=10016`. 413 enum entries total (225 had 393). Core-language ops 0-46 are UNCHANGED (RETURN=21..SWITCH=24 explicit, 31+ explicit) except PUSH_VARBIT(25)/POP_VARBIT(27) deleted → `isLargeOperand` (opcode.go:17-29) is untouched.

- [ ] **Step 1: Generate the pin-test table from the TS pin.**

Run this exactly (it parses the **map section** — the compiler-visible names, including the four `*`-suffixed vararg keys — and joins with enum values):

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS
git show 9aadcec4:src/engine/script/ScriptOpcode.ts | python3 - <<'EOF' > $TMPDIR/pin_entries.txt
import sys, re
src = sys.stdin.read()
# 1. enum name -> value
val, enum = -1, {}
in_enum = False
for line in src.splitlines():
    s = line.strip()
    if s.startswith('export const enum ScriptOpcode'): in_enum = True; continue
    if in_enum and s.startswith('}'): break
    if not in_enum: continue
    m = re.match(r'^([A-Z][A-Z0-9_]*)\s*(?:=\s*(\d+))?\s*,?(?:\s*//.*)?$', s)
    if m:
        val = int(m.group(2)) if m.group(2) else val + 1
        enum[m.group(1)] = val
# 2. map key -> enum name
pairs = re.findall(r"\['([A-Z0-9_*]+)',\s*ScriptOpcode\.([A-Z0-9_]+)\]", src)
assert len(pairs) > 0
for key, name in pairs:
    print(f'\t"{key}": {enum[name]},')
print(f'// COUNT {len(pairs)}', file=sys.stderr)
EOF
wc -l $TMPDIR/pin_entries.txt   # expect ~413 lines
```

- [ ] **Step 2: Write the failing pin test.**

Create `pkg/script/opcode_map_244_pin_test.go`; paste the generated entries into the literal:

```go
package script

import "testing"

// scriptOpcodeMap244Pin pins every compiler-visible opcode name to its 244
// numeric value. Generated from TS ScriptOpcode.ts at pin 9aadcec4 (enum
// values + ScriptOpcodeMap keys, including the four '*' vararg keys).
// Regenerate ONLY when REFERENCES.md moves the rev-244 pin.
var scriptOpcodeMap244Pin = map[string]Opcode{
	// <PASTE $TMPDIR/pin_entries.txt HERE — ~413 lines>
}

// removed244Names were deleted/renamed upstream between e1dea19f and
// 9aadcec4 and must NOT resolve.
var removed244Names = []string{
	"PUSH_VARBIT", "POP_VARBIT", "MAP_LIVE", "STAT_TOTAL", "IF_SETRECOL",
	"HINT_PL", "LOWMEM", "READYANIM", "RUNANIM", "TURNANIM",
	"WALKANIM", "WALKANIM_B", "WALKANIM_L", "WALKANIM_R",
}

func TestScriptOpcodeMap_244Pin(t *testing.T) {
	for name, want := range scriptOpcodeMap244Pin {
		got, ok := ScriptOpcodeMap[name]
		if !ok {
			t.Errorf("ScriptOpcodeMap missing %q", name)
			continue
		}
		if got != want {
			t.Errorf("ScriptOpcodeMap[%q] = %d, want %d", name, got, want)
		}
	}
	if len(ScriptOpcodeMap) != len(scriptOpcodeMap244Pin) {
		t.Errorf("ScriptOpcodeMap has %d entries, 244 pin has %d", len(ScriptOpcodeMap), len(scriptOpcodeMap244Pin))
	}
	for _, name := range removed244Names {
		if _, ok := ScriptOpcodeMap[name]; ok {
			t.Errorf("ScriptOpcodeMap still contains removed name %q", name)
		}
	}
}

func TestOpcodeString_244Pin(t *testing.T) {
	for name, op := range scriptOpcodeMap244Pin {
		if name[len(name)-1] == '*' {
			continue // vararg map keys; String() keeps the enum spelling
		}
		if got := op.String(); got != name {
			t.Errorf("Opcode(%d).String() = %q, want %q", op, got, name)
		}
	}
}
```

- [ ] **Step 3: Run it — expect FAIL** (hundreds of value mismatches against the 225 table).

`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run Test.*244Pin -count=1` → FAIL.

- [ ] **Step 4: Renumber `opcode.go`.** Re-derive every constant value from the enum listing (Step 1's enum dict is authoritative; the anchors above are your cross-check). Apply renames — adopt-244-names for constants AND their handler functions:

| 225 Go name | 244 Go name | 244 value |
|---|---|---|
| `OpHintPl` / `handleHintPl` | `OpHintPlayer` / `handleHintPlayer` | 2033 |
| `OpLowMem` / `handleLowMem` | `OpLowMemory` / `handleLowMemory` | 2114 |
| `OpReadyAnim` | `OpBasReadyAnim` | 2002 |
| `OpRunAnim` | `OpBasRunning` | 2003 |
| `OpTurnAnim` | `OpBasTurnOnSpot` | 2004 |
| `OpWalkAnimB` | `OpBasWalkB` | 2005 |
| `OpWalkAnim` | `OpBasWalkF` | 2006 |
| `OpWalkAnimL` | `OpBasWalkL` | 2007 |
| `OpWalkAnimR` | `OpBasWalkR` | 2008 |

Delete constants: `OpPushVarbit`, `OpPopVarbit`, `OpMapLive`, `OpStatTotal`, `OpIfSetRecol`. Add constants (handlers come in later tasks; values from the table): `OpNpcCount` 1030, `OpZoneCount` 1031, `OpLocCount` 1032, `OpObjCount` 1033, `OpBufferFull` 2009, `OpIfMultizone` 2037, `OpIfOpenOverlay` 2041, `OpPlayerFindAllZone` 2091, `OpPlayerFindNext` 2092, `OpIfOpenMainOverlay` 2112, `OpLastCoord` 2126, `OpNpcHuntNext` 2529, `OpMapProduction` 10001, `OpMapLastClock` 10002, `OpMapLastWorld` 10003, `OpMapLastClientIn` 10004, `OpMapLastNpc` 10005, `OpMapLastPlayer` 10006, `OpMapLastLogout` 10007, `OpMapLastLogin` 10008, `OpMapLastZone` 10009, `OpMapLastClientOut` 10010, `OpMapLastCleanup` 10011, `OpMapLastBandwidthIn` 10012, `OpMapLastBandwidthOut` 10013. Update `String()` for every add/rename/remove.

- [ ] **Step 5: Update `opcode_map.go`** to the full 244 table (renamed keys `HINT_PLAYER`/`LOWMEMORY`/`BAS_*`; removed keys per `removed244Names`; new keys for every added constant — including the enum-only five, which TS keeps compiler-visible).

- [ ] **Step 6: Update `opcode_pointers.go`.** Contract: `git -C /home/owner/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4 -- src/engine/script/ScriptOpcodePointers.ts`. Rename keys (BAS_*×7, `OpHintPlayer`, `OpLowMemory`); delete the `OpIfSetRecol` row; add:

```go
	OpBufferFull: {Require: []string{"active_player"}},
	OpNpcHuntNext: {
		Require:     []string{"find_npc"},
		Require2:    []string{"find_npc"},
		Set:         []string{"active_npc"},
		Set2:        []string{"active_npc2"},
		Conditional: true,
	},
	OpIfOpenOverlay: {Require: []string{"active_player"}, Require2: []string{"active_player2"}},
	OpLastCoord:     {Require: []string{"active_player"}, Require2: []string{"active_player2"}},
```

Pin the four new rows in `opcode_pointers_test.go` (extend the existing table-pin test — one entry each asserting the Require/Set/Conditional shape above). For the IF_SETRECOL-row absence, add an orphan check: every `ScriptOpcodePointers` key must round-trip through `String()` to a name present in `scriptOpcodeMap244Pin` — a leftover row for a deleted opcode surfaces as an orphan. (Checking the old numeric key 2046 would be meaningless post-renumber.)

- [ ] **Step 7: Handler-table + stub surgery.**
  - Remove from `handlers` map + delete functions: `handlePushVarbit`, `handlePopVarbit` (handlers_b0_stubs.go — closes NAI-162-D-STUB-PUSHVARBIT/POPVARBIT; leave a closure note: "244 deleted the enum entries, ScriptOpcode.ts:18-19"), `handleMapLive` (handlers_server.go:41-47 — MAP_PRODUCTION lands in Task 12), `handleStatTotal` (handlers.go:243 row + body in handlers_player.go), `handleIfSetRecol` (handlers_interface.go:349-362 + handlers.go:386 row; the seam/wire removal is Task 2).
  - Add 5 enum-only stubs to `handlers_b0_stubs.go`, registered in the `handlers` map (NAI-162 posture — typed error, not no-op):

```go
// handleIfMultizone (IF_MULTIZONE) — TS-unimplemented at 244: declared in
// ScriptOpcode.ts ("moved to engine, remove this") with no handlers/* entry.
// rev244-b4 stub posture per NAI-162.
func handleIfMultizone(s *ScriptState) error {
	return fmt.Errorf("IF_MULTIZONE: unimplemented")
}

// handleIfOpenMainOverlay (IF_OPENMAINOVERLAY) — TS-unimplemented at 244.
func handleIfOpenMainOverlay(s *ScriptState) error {
	return fmt.Errorf("IF_OPENMAINOVERLAY: unimplemented")
}

// handlePlayerFindAllZone (PLAYER_FINDALLZONE) — TS-unimplemented at 244
// ("todo: replace with huntall").
func handlePlayerFindAllZone(s *ScriptState) error {
	return fmt.Errorf("PLAYER_FINDALLZONE: unimplemented")
}

// handlePlayerFindNext (PLAYER_FINDNEXT) — TS-unimplemented at 244.
func handlePlayerFindNext(s *ScriptState) error {
	return fmt.Errorf("PLAYER_FINDNEXT: unimplemented")
}

// handleLastCoord (LAST_COORD) — TS-unimplemented at 244 (pointer row
// exists upstream, ScriptOpcodePointers.ts:528-531; handler does not).
func handleLastCoord(s *ScriptState) error {
	return fmt.Errorf("LAST_COORD: unimplemented")
}
```

Add stub tests in `handlers_b0_stubs_test.go` mirroring the existing varbit-stub test shape (each returns the "unimplemented" error).

- [ ] **Step 8: Test fallout sweep.** Symbolic constant references auto-track. For literal numeric pins: update to the 244 value — EXCEPT tests that decode 225-era packed blobs (e.g. `pkg/script/file_test.go` real-cache fixtures): those keep the historical literal with a `// 225-era blob; rev244-b4 format window (closes B6)` comment. Survey: `grep -rn "Opcode(2\|Opcode(1\|Opcode(4\|Opcode(7\|Opcode(10" pkg/script pkg/pack/compiler --include="*_test.go"` plus failures from the suite run.

- [ ] **Step 9: Run gates.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ ./pkg/pack/... ./modules/world/ -count=1
```

All green (pin test now PASSES).

- [ ] **Step 10: Commit 1.**

```bash
git add -- pkg/script pkg/pack
git commit --no-gpg-sign -m "feat(script): 244 opcode renumber — full table, renames, deletions, enum-only stubs [rev-244 B4]"
```

- [ ] **Step 11: Doc-comment opcode-number sweep (commit 2).** `grep -rn "opcode [0-9]" pkg/script --include="*.go" | grep -v _test` — every `(NAME, opcode N)` doc comment gets N updated to the 244 value from the pin table (the adjacent NAME is the join key; do NOT blind-sed). Verify with a re-grep cross-checked against `scriptOpcodeMap244Pin`. Commit: `docs(script): re-point doc-comment opcode numbers at the 244 table [rev-244 B4]`.

### Task 2: IF_SETRECOL seam + wire-row removal (closes the B2 deferral)

**Files:**
- Modify: `pkg/script/active.go` (delete `IfSetRecol` from the ActivePlayer interface, ~:400-403)
- Modify: `modules/world/player_interface.go` (delete the `IfSetRecol` method, :111-118)
- Modify: `pkg/io/protocol/game/server/prot.go` (delete `OpIfSetRecol` row :39, the `{"IF_SETRECOL", OpIfSetRecol}` name row :245, and the doc-comment mention :29)
- Modify: any test referencing the removed symbols (grep `IfSetRecol|IF_SETRECOL` across pkg + modules)

TS contract: 244 deletes `IfSetRecolEncoder.ts` + model and removes the op from PlayerOps.ts (B2 decision row: "stayed wired pending B4's removal of the script op"). Task 1 already removed the script-op surface; this task removes everything below the seam.

- [ ] **Step 1: Write the failing absence test** (in `pkg/io/protocol/game/server/` test file alongside the existing table tests):

```go
func TestIfSetRecolRemoved244(t *testing.T) {
	// TS 244 deletes IfSetRecolEncoder + model; the 103/6 row goes with it.
	for _, e := range NameTable() { // use the existing exported name-table accessor in prot.go tests
		if e.Name == "IF_SETRECOL" {
			t.Fatalf("IF_SETRECOL wire row still registered")
		}
	}
}
```

(Adapt the iteration to prot.go's actual name-table test accessor — read the existing table test in that package first and reuse its pattern.)

- [ ] **Step 2: Run — FAIL** (row present). 
- [ ] **Step 3: Delete the wire row, name row, seam method, world impl.** Grep sweep: `grep -rn "IfSetRecol\|IF_SETRECOL" pkg modules --include="*.go"` must return only historical comments (PORTING-CLOSED references); update or delete test references.
- [ ] **Step 4: Run** package tests for `pkg/io/protocol/...`, `pkg/script`, `modules/world` — green.
- [ ] **Step 5: Commit.** `feat(protocol): remove IF_SETRECOL wire row + seam — 244 deletes encoder/model (closes B2 deferral row) [rev-244 B4]`

---

## Slice 2 — Hunt-iterator unification

### Task 3: unified huntIterator + NPC_HUNTNEXT

**Files:**
- Modify: `pkg/script/state.go` (replace `playerIterator *PlayerIterator` field :415-419 with `huntIterator any`)
- Modify: `pkg/script/handlers_player.go` (`handleHuntAll` :1379, `handleHuntNext` :1418)
- Modify: `pkg/script/handlers_npc.go` (`handleNpcHuntAll` :952 re-target; new `handleNpcHuntNext`)
- Modify: `pkg/script/handlers.go` (register `OpNpcHuntNext`)
- Test: `pkg/script/handlers_player_test.go`, `pkg/script/handlers_npc_test.go`

TS contracts (verify each with `cat -n` first):
- `git -C /home/owner/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:src/engine/script/ScriptState.ts | cat -n | sed -n '118,130p'` — single `huntIterator: IterableIterator<Entity>` replaces `playerIterator`; `npcIterator`/`locIterator`/`objIterator` survive.
- `git -C /home/owner/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:src/engine/script/handlers/ServerOps.ts | cat -n | sed -n '53,135p'` — HUNTALL/HUNTNEXT/NPC_HUNT/NPC_HUNTALL/NPC_HUNTNEXT.
- Equivalence note (verified during spec): TS `HuntIterator` PLAYER branch (ScriptIterators.ts:77-97) is line-identical to the 225 `PlayerHuntAllCommandIterator` goscape's `PlayerIterator` mirrors — same descending zone scan, same `getAllPlayersSafe(true)`, same player-as-src LOS order. `PlayerIterator` stays as the HUNTALL engine; record this equivalence in its doc comment and re-cite to ScriptIterators.ts HuntIterator + HuntModeType.PLAYER.

- [ ] **Step 1: Write failing tests.** Three pins in `handlers_npc_test.go` / `handlers_player_test.go` (reuse the existing hunt-test scaffolding — `handlers_npc_test.go` already builds NpcLookup/PlayerLookup fakes for NPC_HUNTALL/HUNTALL; copy a neighbouring test's setup):

```go
// TS 244: NPC_HUNTALL feeds state.huntIterator (ServerOps.ts:109-119);
// NPC_FINDNEXT still consumes state.npcIterator → a huntall result is
// no longer visible to findnext.
func TestNpcHuntAll_NoLongerFeedsNpcFindNext(t *testing.T) {
	s := newHuntAllNpcState(t) // adapt: the existing NPC_HUNTALL test fixture w/ 1 matching npc
	if err := handleNpcHuntAll(s); err != nil {
		t.Fatal(err)
	}
	if err := handleNpcFindNext(s); err != nil {
		t.Fatal(err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("NPC_FINDNEXT after NPC_HUNTALL = %d, want 0 (huntIterator split)", got)
	}
}

// TS 244 ServerOps.ts:121-135 — NPC_HUNTNEXT consumes huntIterator.
func TestNpcHuntNext_ConsumesHuntIterator(t *testing.T) {
	s := newHuntAllNpcState(t)
	if err := handleNpcHuntAll(s); err != nil {
		t.Fatal(err)
	}
	if err := handleNpcHuntNext(s); err != nil {
		t.Fatal(err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("NPC_HUNTNEXT = %d, want 1", got)
	}
	if s.ActiveNpc == nil {
		t.Fatal("NPC_HUNTNEXT did not bind active_npc")
	}
}

// TS 244 type guards: HUNTNEXT requires a Player-yielding iterator
// (ServerOps.ts:71-73), NPC_HUNTNEXT an Npc-yielding one (:129-131).
func TestHuntNext_TypeMismatchErrors(t *testing.T) {
	s := newHuntAllNpcState(t)
	if err := handleNpcHuntAll(s); err != nil { // huntIterator now holds *NpcIterator
		t.Fatal(err)
	}
	if err := handleHuntNext(s); err == nil {
		t.Fatal("HUNTNEXT over an npc iterator: want error, got nil")
	}
}
```

Plus the inverse mismatch (HUNTALL then `handleNpcHuntNext` → error) and a HUNTALL/HUNTNEXT happy-path re-run against `huntIterator` (the existing HUNTNEXT tests, re-pointed).

- [ ] **Step 2: Run — FAIL** (`handleNpcHuntNext` undefined; split not yet made).
- [ ] **Step 3: Implement.**
  - `state.go`: replace the `playerIterator` field with:

```go
	// huntIterator holds the active hunt-command iterator: *PlayerIterator
	// (set by HUNTALL) or *NpcIterator (set by NPC_HUNTALL). Consumers
	// type-switch and error on mismatch, reproducing TS's instanceof
	// guards (ServerOps.ts:71-73,129-131). Single-tick lifetime — Stale()
	// enforced by the consumers. Mirrors TS ScriptState.huntIterator
	// (ScriptState.ts:124, IterableIterator<Entity>); replaces the 225
	// playerIterator field.
	huntIterator any
```

  - `handleHuntAll`: store into `s.huntIterator`; re-cite to ServerOps.ts:53-61 + the PlayerIterator≡HuntIterator(PLAYER) equivalence note.
  - `handleHuntNext`: 

```go
	it, ok := s.huntIterator.(*PlayerIterator)
	if s.huntIterator != nil && !ok {
		return fmt.Errorf("HUNTNEXT: command must result instance of Player") // TS ServerOps.ts:72
	}
	if it == nil {
		s.PushInt(0)
		return nil
	}
	// ... existing Stale check + Next + operand-slot binding unchanged
```

  - `handleNpcHuntAll`: `s.huntIterator = NewHuntAllNpcIterator(...)` (was `s.npcIterator`); re-cite ServerOps.ts:109-119.
  - New `handleNpcHuntNext` modeled on `handleNpcFindNext` (handlers_npc.go:1066-1083) but reading `s.huntIterator`:

```go
// handleNpcHuntNext (NPC_HUNTNEXT, opcode 2529) advances the unified hunt
// iterator and binds the next NPC to the operand-selected active slot.
// Mirrors TS ServerOps.ts:121-135. Errors when the iterator holds players
// (TS instanceof Npc guard, :129-131).
func handleNpcHuntNext(s *ScriptState) error {
	it, ok := s.huntIterator.(*NpcIterator)
	if s.huntIterator != nil && !ok {
		return fmt.Errorf("NPC_HUNTNEXT: command must result instance of Npc")
	}
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("NPC_HUNTNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	npc, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}
```

  - Register `OpNpcHuntNext: handleNpcHuntNext` in handlers.go.
- [ ] **Step 4: Run** `go test ./pkg/script/ -count=1` — green (including re-pointed HUNTNEXT tests).
- [ ] **Step 5: Commit.** `feat(script): unify huntIterator — HUNTALL/NPC_HUNTALL feed it, new NPC_HUNTNEXT, NPC_FINDNEXT split pinned [rev-244 B4]`

---

## Slice 3 — Per-op behavioral deltas

### Task 4: HINT_NPC / HINT_PLAYER + activePlayer2 deletion

**Files:**
- Modify: `pkg/script/handlers_player.go` (`handleHintNpc` :1456, `handleHintPl`→`handleHintPlayer` :1495)
- Modify: `pkg/script/active_player.go` (delete `activePlayer2()` :46-52 if zero callers remain)
- Test: `pkg/script/handlers_player_test.go`

TS contract: `... show 9aadcec4:src/engine/script/handlers/PlayerOps.ts | cat -n | sed -n '963,975p'`:
HINT_NPC pops the nid (`check(state.popInt(), NumberNotNull)`); HINT_PLAYER pops a uid, `World.getPlayerByUid(uid)`, silent return on miss, `hintPlayer(player.pid)`. TS also deletes the `activePlayer2` getter (ScriptState.ts 225:222-230) — its sole consumer was HINT_PL.

- [ ] **Step 1: Failing tests.**

```go
// TS 244 PlayerOps.ts:963-965 — HINT_NPC pops the nid from the stack.
func TestHintNpc_PopsNidFromStack(t *testing.T) {
	s, rec := newHintTestState(t) // adapt the existing HINT_NPC fixture; rec records HintNpc calls
	s.PushInt(42)
	if err := handleHintNpc(s); err != nil {
		t.Fatal(err)
	}
	if rec.hintNpcNid != 42 {
		t.Fatalf("HintNpc nid = %d, want 42 (popped)", rec.hintNpcNid)
	}
}

// TS 244 PlayerOps.ts:967-974 — HINT_PLAYER pops a uid; lookup miss is a
// silent no-op; hit hints by pid.
func TestHintPlayer_UidLookup(t *testing.T) {
	s, rec := newHintTestState(t) // fixture's PlayerLookup resolves uid 7 → player with pid 3
	s.PushInt(7)
	if err := handleHintPlayer(s); err != nil {
		t.Fatal(err)
	}
	if rec.hintPlayerPid != 3 {
		t.Fatalf("HintPlayer pid = %d, want 3", rec.hintPlayerPid)
	}
	s.PushInt(9999) // unknown uid
	if err := handleHintPlayer(s); err != nil {
		t.Fatalf("missing player must be silent, got %v", err)
	}
}
```

(Build `newHintTestState` from the existing HINT_* test scaffolding; the recorder fake already exists for HintNpc/HintPlayer assertions — extend it, don't duplicate.)

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.** `handleHintNpc`: keep `requireActivePlayer`; DROP `requireActiveNpc` (TS no longer touches activeNpc — the POINTER row keeps active_npc for the compiler, opcode_pointers.go untouched); pop nid with `checkNotNull`. `handleHintPlayer`: keep `requireActivePlayer`; DROP `requireActivePlayer2`; pop uid with `checkNotNull`; `p := s.PlayerLookup.LookupPlayerByUID(uid)` (nil `s.PlayerLookup` → silent return, matching sibling convention); `if p == nil { return nil }`; `s.activePlayer().HintPlayer(p.Pid())` — `Pid()` is the B3 player accessor. Then `grep -rn "activePlayer2()" pkg/script --include="*.go" | grep -v _test` — if `handleHintPlayer` was the last caller, delete the helper with a note mirroring TS's getter deletion; if `requireActivePlayer2` still has other callers, leave it.
- [ ] **Step 4: Run** pkg/script tests — green.
- [ ] **Step 5: Commit.** `feat(script): 244 HINT_NPC pops nid, HINT_PLAYER pops uid via LookupPlayerByUID; drop activePlayer2 getter [rev-244 B4]`

### Task 5: DB_GETFIELD tuple-index removal

**Files:**
- Modify: `pkg/script/handlers_db.go` (`handleDbGetField` :76-122)
- Test: `pkg/script/handlers_db_test.go`

TS contract: `... show 9aadcec4:src/engine/script/handlers/DbOps.ts | cat -n | sed -n '95,120p'` — packed key carries table+column only (`tableColumnPacked`), tuple sub-selection gone; pushes ALL column values.

- [ ] **Step 1: Failing test.** Reuse the existing DB fixture that has a multi-tuple column (handlers_db_test.go already builds these for GETFIELDCOUNT):

```go
// TS 244 DbOps.ts:97-119 — the tupleIndex nibble is ignored; GETFIELD
// pushes every value of the column.
func TestDbGetField_TupleIndexIgnored244(t *testing.T) {
	s := newDbTestState(t) // adapt: fixture with a 2-type column
	packed := (testTableID << 12) | (testColumn << 4) | 2 // 225 would sub-select tuple 1
	s.PushInt(testRowID)
	s.PushInt(packed)
	s.PushInt(0) // listIndex
	if err := handleDbGetField(s); err != nil {
		t.Fatal(err)
	}
	// 225 sub-selected tuple value 1; 244 pushes BOTH column values in
	// order. Assert both pops against the fixture's seeded tuple — e.g.
	// for a column seeded [11, 22]: second pop = 11, first pop = 22
	// (LIFO). Bind the literals to the fixture you reuse.
	got2 := s.PopInt()
	got1 := s.PopInt()
	if got1 != 11 || got2 != 22 {
		t.Fatalf("pushed (%d,%d), want full column (11,22)", got1, got2)
	}
}
```

- [ ] **Step 2: Run — FAIL** (225 code sub-selects).
- [ ] **Step 3: Implement.** Rename locals `packed`→`tableColumnPacked`, `fieldTable`→`table`, `fieldColumn`→`column`; delete the `tupleIndex` derivation, bounds check, and `off/length` windowing; loop `for i := range valueTypes`. Update the doc comment + citation (DbOps.ts:97-119). The old "tuple index out-of-bounds" error string goes — delete any test pinning it.
- [ ] **Step 4: Run** — green. **Step 5: Commit.** `feat(script): 244 DB_GETFIELD drops tuple-index sub-selection — pushes full column [rev-244 B4]`

### Task 6: InvOps — untradeable-stop + wealth re-keys

**Files:**
- Modify: `pkg/script/active.go` (ActivePlayer gains `RecipientSession() string`)
- Modify: `modules/world/player_script.go` (implement `RecipientSession`)
- Modify: `pkg/script/handlers_inv.go` (BOTH_DROPSLOT :2106-2122, INV_DROPALL drop loop :2235-2250, STAKE :1724, TRADE :1767)
- Test: `pkg/script/handlers_inv_test.go`

TS contracts (verify all with `cat -n`):
- `... show 9aadcec4:src/engine/script/handlers/InvOps.ts | cat -n | sed -n '440,500p'` (toSession + STAKE/TRADE re-keys), `sed -n '700,735p'` (BOTH_DROPSLOT: recipient_id/p2Session + untradeable `return`), `sed -n '760,785p'` (INV_DROPALL: untradeable `continue`).
- `isClientConnected(toPlayer) ? toPlayer.client.uuid : 'disconnected'` — goscape's single-Player-type adaptation (B3-documented): `client != nil → p.session`, else `"disconnected"`.

- [ ] **Step 1: Failing tests.**

```go
// TS 244 InvOps.ts:719-721 — untradeables stop after delete; nothing drops.
func TestBothDropSlot_UntradeableStops244(t *testing.T) {
	s, world := newBothDropSlotState(t, withUntradeableObj()) // adapt existing fixture
	pushBothDropSlotArgs(s)
	if err := handleBothDropSlot(s); err != nil {
		t.Fatal(err)
	}
	if len(world.addedObjs) != 0 {
		t.Fatalf("untradeable dropped %d objs, want 0 (244 stop-after-delete)", len(world.addedObjs))
	}
	// the inv slot must still be cleared (delete happens before the gate)
}

// TS 244 InvOps.ts:771-775 — INV_DROPALL skips untradeables after delete.
func TestInvDropAll_UntradeableStops244(t *testing.T) { /* same shape over the slot walk */ }

// TS 244 InvOps.ts:706-714 — PVP wealth event carries recipient_id +
// recipient_session ('disconnected' fallback).
func TestBothDropSlot_WealthRecipientRekey244(t *testing.T) {
	s, _ := newBothDropSlotState(t, withTradeableObj())
	pushBothDropSlotArgs(s)
	if err := handleBothDropSlot(s); err != nil {
		t.Fatal(err)
	}
	evt := lastWealthEvent(s) // fixture recorder
	if evt.RecipientID != testToPlayerAccountID {
		t.Fatalf("RecipientID = %d, want %d", evt.RecipientID, testToPlayerAccountID)
	}
	if evt.RecipientSession != "disconnected" { // fixture player has no client
		t.Fatalf("RecipientSession = %q, want \"disconnected\"", evt.RecipientSession)
	}
}
```

(Adapt names to the existing BOTH_DROPSLOT/INV_DROPALL fixtures in handlers_inv_test.go — they already record `World.AddObj` calls and wealth events.)

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.**
  - `active.go` ActivePlayer interface (next to `AccountID()` :587):

```go
	// RecipientSession returns this player's per-login session UUID when a
	// client is attached, else "disconnected". Used when this player is
	// the COUNTERPARTY of a wealth event. Mirrors TS InvOps.ts:446
	// `isClientConnected(toPlayer) ? toPlayer.client.uuid : 'disconnected'`
	// (single-Player-type adaptation, rev-244 B3 row).
	RecipientSession() string
```

  - `modules/world/player_script.go`:

```go
// RecipientSession implements script.ActivePlayer. TS InvOps.ts:446 /
// NetworkPlayer isClientConnected — connected → session UUID, else
// 'disconnected'. Distinct from AddSessionLog's 'headless' fallback
// (Player.ts:641), which covers the never-had-a-client path.
func (p *Player) RecipientSession() string {
	if p.client != nil {
		return p.session
	}
	return "disconnected"
}
```

Follow §NEW-INTERFACE-METHOD-COMPILE-CASCADE for pkg/script test fakes.
  - BOTH_DROPSLOT (handlers_inv.go:2079-2122): wealth event gains `RecipientID: toPlayer.AccountID(), RecipientSession: toPlayer.RecipientSession()` (delete the NAI-162 deferral comment — closure note: "rev-244 B4 closes the Session-not-exposed deferral"); replace the untradeable→fromPlayer branch with:

```go
	if !objType.Tradeable {
		return nil // TS 244 InvOps.ts:720 — stop untradables after delete.
	}
	s.World.AddObj(level, x, z, objID, completed, duration, toPlayer.UID(), s.activePlayer().AccountID())
```

  - INV_DROPALL drop loop (:2240-2250): replace the untradeable→self branch with `continue` (TS :772-774); tradeable path keeps `Obj.NO_RECEIVER`.
  - STAKE (:1724) and TRADE (:1767) events: add `RecipientID: toPlayer.AccountID(), RecipientSession: toPlayer.RecipientSession()`; delete the two deferral comments.
- [ ] **Step 4: Run** pkg/script + modules/world tests — green. **Step 5: Commit.** `feat(script): 244 InvOps — untradeables stop after delete; wealth events re-keyed recipient_id/recipient_session [rev-244 B4]`

### Task 7: NPC_STATHEAL + MAP_BLOCKED + P_OPOBJ verification

**Files:**
- Modify: `pkg/script/handlers_npc.go` (`handleNpcStatHeal` :1418-1445)
- Modify: `pkg/script/handlers_map.go` (`handleMapBlocked` :249-266)
- Verify-only: `pkg/script/handlers_player.go` (`handleP_OpObj` :1547-1577)
- Test: `pkg/script/handlers_npc_test.go`, `pkg/script/handlers_map_test.go`

TS contracts: NpcOps.ts:243-252 (heroPoints.clear branch deleted); ServerOps.ts:281-285 (F2P gate deleted); PlayerOps.ts:986-1001 (`!objType.op || !objType.op[type]`).

- [ ] **Step 1: Failing tests.**

```go
// TS 244 NpcOps.ts:243-252 — full heal no longer clears the hero ledger.
func TestNpcStatHeal_FullHealKeepsHeroPoints244(t *testing.T) {
	s := newStatHealState(t) // existing fixture; npc has hero credit + damaged HP
	pushStatHealArgs(s, hitpointsStat, bigConstant, 0)
	if err := handleNpcStatHeal(s); err != nil {
		t.Fatal(err)
	}
	if recorder(s).heroPointsCleared {
		t.Fatal("heroPoints cleared on full heal — 244 deleted that branch")
	}
}

// TS 244 ServerOps.ts:281-285 — MAP_BLOCKED loses the F2P-world gate.
func TestMapBlocked_NoF2PGate244(t *testing.T) {
	s := newMapBlockedState(t, freeWorld(), nonF2PCoord(), unblockedTile())
	s.PushInt(testCoord)
	if err := handleMapBlocked(s); err != nil {
		t.Fatal(err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("MAP_BLOCKED = %d, want 0 (gate removed; tile itself unblocked)", got)
	}
}
```

- [ ] **Step 2: Run — FAIL** (old branches fire). Note the MAP_BLOCKED test seeds an UNBLOCKED tile in a non-F2P zone on a free world so the deleted gate is the only discriminating condition (mandate #2).
- [ ] **Step 3: Implement.** Delete the `HeroPointsClear` branch (:1441-1443) + doc-comment line; delete the F2P gate (:255-259) + doc-comment lines; update citations (NpcOps.ts / ServerOps.ts 244 line numbers from your `cat -n`). If `HeroPointsClear` on ActiveNpc now has zero callers, leave the seam (other consumers exist — verify with grep; only delete on zero callers).
- [ ] **Step 4: P_OPOBJ verification (no code expected).** Compare handleP_OpObj's guard against PlayerOps.ts:986-1001: goscape's `objType == nil || op-1 >= len(objType.Op) || objType.Op[op-1] == ""` already covers TS 244's `!objType.op || !objType.op[type]` falsy semantics (the L17 comment documents the silent skip). Update the citation/locals only if the `cat -n` check shows drift; record a NO-OP verdict for the audit (Task 13).
- [ ] **Step 5: Run + commit.** `feat(script): 244 NPC_STATHEAL keeps heroPoints on full heal; MAP_BLOCKED drops F2P gate; P_OPOBJ guard verified [rev-244 B4]`

### Task 8: BUFFER_FULL + IF_OPENOVERLAY dispatch

**Files:**
- Modify: `pkg/script/active.go` (ActivePlayer gains `OpenOverlay(com int)` — next to `OpenChat` :324-326)
- Modify: `pkg/script/handlers_player.go` (new `handleBufferFull`)
- Modify: `pkg/script/handlers_interface.go` (new `handleIfOpenOverlay`, next to handleIfOpenChat :37)
- Modify: `pkg/script/handlers.go` (register both)
- Verify-only: `modules/world/player_script.go:1351` (`OpenOverlay` exists from B3 — the world side needs NO change; the interface method addition makes `*Player` satisfy it)
- Test: `pkg/script/handlers_player_test.go`, `pkg/script/handlers_interface_test.go`

TS contracts: PlayerOps.ts:198-203 (BUFFER_FULL pushes 0, checkedHandler ActivePlayer, Ash-tweet todo) and :709-712 (IF_OPENOVERLAY — **raw popInt, no NumberNotNull**; −1 must reach openOverlay to clear).

- [ ] **Step 1: Failing tests.**

```go
// TS 244 PlayerOps.ts:198-203 — BUFFER_FULL pushes 0 (upstream todo posture).
func TestBufferFull_PushesZero(t *testing.T) {
	s := newPlayerOpState(t)
	if err := handleBufferFull(s); err != nil {
		t.Fatal(err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("BUFFER_FULL = %d, want 0", got)
	}
}

// TS 244 PlayerOps.ts:709-712 — IF_OPENOVERLAY dispatches to openOverlay
// WITHOUT a NumberNotNull wrap: -1 passes through (clears the overlay).
func TestIfOpenOverlay_DispatchesIncludingMinusOne(t *testing.T) {
	s, rec := newInterfaceOpState(t)
	s.PushInt(523)
	if err := handleIfOpenOverlay(s); err != nil {
		t.Fatal(err)
	}
	if rec.openOverlayCom != 523 {
		t.Fatalf("OpenOverlay com = %d, want 523", rec.openOverlayCom)
	}
	s.PushInt(-1)
	if err := handleIfOpenOverlay(s); err != nil {
		t.Fatalf("-1 must pass through (clear), got %v", err)
	}
	if rec.openOverlayCom != -1 {
		t.Fatalf("OpenOverlay com = %d, want -1", rec.openOverlayCom)
	}
}
```

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.**

```go
// handleBufferFull (BUFFER_FULL, opcode 2009) pushes 0. TS 244 stubs the
// bandwidth soft-limit ("todo: should we have this yet?") — PlayerOps.ts:198-203.
func handleBufferFull(s *ScriptState) error {
	if err := requireActivePlayer(s, "BUFFER_FULL"); err != nil {
		return err
	}
	s.PushInt(0)
	return nil
}

// handleIfOpenOverlay implements IF_OPENOVERLAY. TS PlayerOps.ts:709-712 —
// raw popInt (no NumberNotNull: -1 clears). Dispatches to the B3 overlay
// state (Player.OpenOverlay, player_script.go; flushed by encodeOut).
// Closes the B2 (0ef495fb wire row) → B3 (ebce9706 entity state+flush) →
// B4 chain.
func handleIfOpenOverlay(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_OPENOVERLAY"); err != nil {
		return err
	}
	s.activePlayer().OpenOverlay(s.PopInt())
	return nil
}
```

Interface addition mirrors the OpenChat doc-comment shape; follow the fake cascade convention.
- [ ] **Step 4: Run + commit.** `feat(script): 244 BUFFER_FULL stub + IF_OPENOVERLAY dispatch — closes B2/B3 overlay chain [rev-244 B4]`

### Task 9: Runner error-shape deltas

**Files:**
- Modify: `pkg/script/runner.go` (dispatch message; `Backtrace` :~178-200)
- Modify: `modules/world/script.go` (`logScriptExecuteError` :297 + player call site :151)
- Test: `pkg/script/runner_test.go`, `modules/world/script_test.go` (or the existing script-error test home — locate with `grep -rn "script execute error" modules/world --include="*_test.go"`)

TS contract: `... show 9aadcec4:src/engine/script/ScriptRunner.ts | cat -n | sed -n '145,230p'` — three deltas: (1) `Unknown opcode ${opcode}` (name-mapping dropped); (2) player error line gains pid (`Player script error - pid:${pid} name:${username}`); (3) BOTH debug-trace loops change `i >= 0` → `i > 0` (frame 0 — the script that started the chain — is skipped).

- [ ] **Step 1: Failing tests.**

```go
// TS 244 ScriptRunner.ts:151 — unknown-opcode error drops the name map.
func TestExecute_UnknownOpcodeMessage244(t *testing.T) {
	s := newRunnerState(t, withOpcode(Opcode(60000)))
	err := Execute(s)
	if err == nil || !strings.Contains(err.Error(), "unknown opcode 60000") {
		t.Fatalf("err = %v, want 'unknown opcode 60000'", err)
	}
}

// TS 244 ScriptRunner.ts:196,221 — backtrace loops skip frame 0.
func TestBacktrace_SkipsFrameZero244(t *testing.T) {
	s := newRunnerStateWithFrames(t, 3) // frames[0..2] populated, FrameSP=3
	lines := Backtrace(s)
	// header + current script + frames 2,1 (frame 0 skipped) = 4 lines
	if len(lines) != 4 {
		t.Fatalf("Backtrace lines = %d, want 4 (frame 0 skipped)", len(lines))
	}
}
```

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.** (1) Dispatch: `fmt.Errorf("script %q: unknown opcode %d at pc=%d", s.Script.Name, op, s.PC)` — drop `op.String()`; cite ScriptRunner.ts:151. (2) `Backtrace`: `for i := s.FrameSP - 1; i > 0; i--` with a citation note ("244 skips frame 0 — ScriptRunner.ts:196/221; Go shares one impl for the player and console paths"). (3) `logScriptExecuteError` gains variadic attrs `extra ...any` appended to `s.log.Warn`; the player call site (script.go:151) passes `"pid", <pid>, "name", <username>` (extract from `state.Self` via the ActivePlayer accessors used nearby — `Pid()`/`Username()`; verify the accessor names in active.go first). Cite ScriptRunner.ts:187.
- [ ] **Step 4: Run + commit.** `feat(script): 244 runner deltas — unknown-opcode message, pid in player error log, backtrace skips frame 0 [rev-244 B4]`

---

## Slice 4 — World surfaces

### Task 10: Count ops (NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT)

**Files:**
- Modify: `pkg/script/state.go` (WorldVars :57 gains 4 methods)
- Modify: `modules/world/server_varp.go` (worldVarsView impls)
- Create: handlers in `pkg/script/handlers_server.go` + registration in `handlers.go`
- Test: `pkg/script/handlers_server_test.go`, `modules/world/server_varp_test.go` (or the worldVarsView test home)

TS contracts: ServerOps.ts:402-417; `World.getTotalNpcs` = `this.npcs.count` (World.ts:1734-1736); `GameMap.getTotalZones/Locs/Objs` = `zonemap.zoneCount()/locCount()/objCount()` (GameMap.ts:102-112). goscape analogs: `pkg/zone` ZoneMap already has `ZoneCount()` :51, `LocCount()` :102, `ObjCount()` :111; npcs live in `Server.npcs [8192]*Npc` (server.go:215) — count non-nil entries.

- [ ] **Step 1: Failing handler tests** (fake WorldVars returns canned totals; assert each op pushes it):

```go
// TS 244 ServerOps.ts:402-417.
func TestCountOps(t *testing.T) {
	s := newServerOpState(t, withTotals(12, 34, 56, 78)) // npcs, zones, locs, objs
	for _, tc := range []struct {
		h    func(*ScriptState) error
		want int
	}{
		{handleNpcCount, 12}, {handleZoneCount, 34}, {handleLocCount, 56}, {handleObjCount, 78},
	} {
		if err := tc.h(s); err != nil {
			t.Fatal(err)
		}
		if got := s.PopInt(); got != tc.want {
			t.Fatalf("= %d, want %d", got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.** WorldVars additions (cascade convention for fakes):

```go
	// 244 count ops (ServerOps.ts:402-417). TotalNpcs mirrors TS
	// World.getTotalNpcs (npcs.count, World.ts:1734); the rest mirror
	// GameMap.getTotalZones/Locs/Objs (GameMap.ts:102-112).
	TotalNpcs() int
	TotalZones() int
	TotalLocs() int
	TotalObjs() int
```

worldVarsView: `TotalZones/TotalLocs/TotalObjs` delegate to the server's ZoneMap (`ZoneCount()`/`LocCount()`/`ObjCount()` — locate the server's zonemap field via the existing `IsMapBlocked` impl's path); `TotalNpcs` counts non-nil `s.npcs` entries (tick-goroutine read, same ownership as `PlayerCount()` — copy its locking posture). Handlers in handlers_server.go (next to handleInZone):

```go
// handleNpcCount (NPCCOUNT, opcode 1030) — TS ServerOps.ts:403-405.
func handleNpcCount(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("NPCCOUNT: %w", ErrNoWorld)
	}
	s.PushInt(s.World.TotalNpcs())
	return nil
}
```

(ZONECOUNT/LOCCOUNT/OBJCOUNT identical shape.) Register all four.
- [ ] **Step 4: Run + commit.** `feat(script): 244 NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT + WorldVars totals [rev-244 B4]`

### Task 11: Cycle-stats instrumentation (modules/world)

**Files:**
- Create: `modules/world/world_stats.go` + `modules/world/world_stats_test.go`
- Modify: `modules/world/server.go` (two `[12]uint16` fields on Server)
- Modify: `modules/world/tick.go` (section timing + snapshot)
- Modify: `modules/world/player.go` (`processIn` bandwidth-in :1177-1216; `writeOut` bandwidth-out :576)

TS contract (verify every site): `... show 9aadcec4:src/engine/World.ts | cat -n | sed -n '480,505p;615,635p;685,695p;715,725p;770,780p;840,850p;970,980p;1000,1010p;1105,1115p;1140,1148p;1215,1222p'` and `... show 9aadcec4:src/engine/entity/NetworkPlayer.ts | cat -n | sed -n '75,90p;235,245p'`. Shape: `cycleStats`/`lastCycleStats: Uint16Array(12)`; each section assigns `Date.now() - start` at its end; CYCLE at :487 ("before telemetry"); snapshot copy :489-500; BANDWIDTH_IN reset :629 + `+= bytesRead` (NetworkPlayer.ts:83); BANDWIDTH_OUT reset :1111 + `+= buf.pos` (NetworkPlayer.ts:241).

- [ ] **Step 1: Write `world_stats.go`** (constants + helpers first — they're pure and testable):

```go
package world

import "time"

// WorldStat indexes the per-cycle stats arrays. Order mirrors TS
// WorldStat.ts:1-14 exactly (identical at both pins).
const (
	statCycle = iota
	statWorld
	statClientIn
	statNpc
	statPlayer
	statLogout
	statLogin
	statZone
	statClientOut
	statCleanup
	statBandwidthIn
	statBandwidthOut
	numWorldStats // 12
)

// addCycleTime accumulates elapsed wall-clock ms into cycleStats[stat].
// TS assigns once per section (cycleStats[X] = Date.now() - start);
// goscape's tick pipeline splits several TS sections into multiple passes
// (documented deviations NAI-93/NAI-122/NAI-217 et al.), so the Go shape
// zeroes the timing stats at tick start (resetCycleTimes) and ACCUMULATES
// per pass — the per-section total is the same sum TS measures. uint16
// arithmetic wraps mod 65536, matching TS Uint16Array truncation.
func (s *Server) addCycleTime(stat int, start time.Time) {
	s.cycleStats[stat] += uint16(time.Since(start).Milliseconds())
}

// resetCycleTimes zeroes the ten timing entries at tick start. The two
// bandwidth counters have their own TS-cited reset points (World.ts:629,
// :1111) and are NOT touched here.
func (s *Server) resetCycleTimes() {
	for i := statCycle; i <= statCleanup; i++ {
		s.cycleStats[i] = 0
	}
}

// snapshotCycleStats copies cycleStats into lastCycleStats at cycle end.
// Mirrors TS World.ts:489-500. Tick-goroutine-only.
func (s *Server) snapshotCycleStats() {
	s.lastCycleStats = s.cycleStats
}

// LastCycleStat returns lastCycleStats[stat], the surface the MAP_LAST*
// debug script ops read (DebugOps.ts:20-68). Tick-goroutine-only (script
// execution runs on-tick).
func (s *Server) LastCycleStat(stat int) int {
	if stat < 0 || stat >= numWorldStats {
		return 0
	}
	return int(s.lastCycleStats[stat])
}
```

Server fields (server.go, near `npcs`):

```go
	// cycleStats/lastCycleStats mirror TS World.cycleStats /
	// lastCycleStats (Uint16Array(12), World.ts — both pins; the surface
	// is new to goscape at rev-244 B4, closing a pre-existing 225-era
	// gap). Tick-goroutine-owned; uint16 wrap is TS-faithful.
	cycleStats     [numWorldStats]uint16
	lastCycleStats [numWorldStats]uint16
```

- [ ] **Step 2: Failing unit tests** (`world_stats_test.go`): `addCycleTime` accumulates; uint16 wrap (`s.cycleStats[statWorld] = 65530; addCycleTime` with a 10ms-ago start → wraps); `resetCycleTimes` zeroes timing entries but NOT bandwidth; `snapshotCycleStats` copies; `LastCycleStat` bounds. Run — FAIL (file new) → implement → PASS.

- [ ] **Step 3: Wire the tick loop** (tick.go). Mapping (each goscape pass → the TS section whose citation it already carries; record this table in a comment above the tick body):

| Stat | goscape passes |
|---|---|
| CLIENT_IN | `processClientsIn` (+ BANDWIDTH_IN reset immediately before, TS World.ts:629) |
| WORLD | `processWorldQueue`, `processNpcEventQueue`, `processNpcHuntPlayers`, `processActiveScripts`, `processObjDelayedQueue` (TS processWorld umbrella, W.ts:556-619; goscape splits/reorders these passes per pre-existing pinned deviations) |
| NPC | `processNpcs` |
| PLAYER | `processPlayerTimers`, `processPlayerEngineQueues`, `processInteractionsPreMove`, `processPathing`, `processInteractions`, `processEnergy`, `processValidateDistanceWalked` (TS processPlayers, W.ts:723-775) |
| LOGOUT | `processLogouts` |
| LOGIN | `processLogins` |
| ZONE | `processZones` |
| CLIENT_OUT | `processInfo` + `processClientsOut` (+ BANDWIDTH_OUT reset at the head of this group, TS W.ts:1111) — TS computes player-info inside its client-out phase (W.ts:1090-1144) |
| CLEANUP | `processCleanup` (`processSessionLogs` counts only toward CYCLE, like TS's session-log block W.ts:428-442) |
| CYCLE | `time.Since(start)` measured after `processSessionLogs`, "before telemetry" (W.ts:487); then `snapshotCycleStats()` |

Implementation shape per group (after the existing `start := time.Now()` add `s.resetCycleTimes()`):

```go
		t0 := time.Now()
		s.cycleStats[statBandwidthIn] = 0 // TS World.ts:629
		s.processClientsIn()
		s.addCycleTime(statClientIn, t0)

		t0 = time.Now()
		s.processWorldQueue()
		s.processNpcEventQueue()
		s.addCycleTime(statWorld, t0)
		// ... (keep every existing comment block in place; only the timing
		// lines are added around the existing calls, preserving call order)
```

For split groups (WORLD's hunt pass sits lower in the body), re-snapshot `t0` before each member and `addCycleTime` after — the accumulate semantics handle non-contiguity. At the end:

```go
		s.cycleStats[statCycle] = uint16(time.Since(start).Milliseconds()) // TS W.ts:487
		s.snapshotCycleStats()                                            // TS W.ts:489-500
```

(before `s.currentTick++`).

- [ ] **Step 4: Bandwidth counters.**
  - IN (player.go `processIn`): the decode loop already computes the TS-equivalent delta (`c.in.Pos - posBefore` — the gap-configs-snapshot-netbase-3 comment cites NetworkPlayer.ts:69,78-83, the same lines that feed `bytesRead`). After the existing `if c.in.Pos-posBefore > 0` block add:

```go
	if delta := c.in.Pos - posBefore; delta > 0 && c.server != nil {
		// TS NetworkPlayer.ts:83 — World.cycleStats[BANDWIDTH_IN] += bytesRead.
		c.server.cycleStats[statBandwidthIn] += uint16(delta)
	}
```

(Adapt the server-access path to the client struct's actual field — verify `c.server` exists; processIn runs on the tick goroutine, so the write is single-writer.)
  - OUT (player.go `writeOut` :576): after the frame is staged, add

```go
	// TS NetworkPlayer.ts:241 — World.cycleStats[BANDWIDTH_OUT] += buf.pos
	// (the framed length: opcode byte + length prefix + payload).
	if c.server != nil {
		n := 1 + len(payload)
		switch op.PayloadSize {
		case -1:
			n++
		case -2:
			n += 2
		}
		c.server.cycleStats[statBandwidthOut] += uint16(n)
	}
```

First verify with `cat -n` what TS `buf.pos` covers at NetworkPlayer.ts:235-245 (expected: the staged out-buffer position = framed bytes; adjust if the listing shows otherwise).
- [ ] **Step 5: Integration pin** (in an existing tick-capable test harness — modules/world has server tick tests; reuse their constructor):

```go
// Deterministic (no timing assertions): seed sentinels, run one tick, and
// assert the reset→measure→snapshot pipeline ran.
func TestCycleStats_ResetAndSnapshotPipeline(t *testing.T) {
	s := newTickTestServer(t) // adapt to the existing tick-test fixture
	s.cycleStats[statWorld] = 9999     // stale value from a "previous" tick
	s.lastCycleStats[statWorld] = 7777 // stale snapshot
	s.runOneTick()                     // the harness's single-tick driver
	// resetCycleTimes zeroed the stale 9999 before re-measuring, and
	// snapshotCycleStats copied the fresh measurement (sub-second test
	// tick → 0..few ms, never the stale sentinels).
	if got := s.LastCycleStat(statWorld); got == 7777 || got == 9999 {
		t.Fatalf("lastCycleStats[WORLD] = %d — reset/snapshot pipeline did not run", got)
	}
}
```
- [ ] **Step 6: Gates incl. `-race`.** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=1 go test -race ./modules/world/ -count=1` — green (bandwidth writes are tick-goroutine-only; a race here means the access path was wrong).
- [ ] **Step 7: Commit.** `feat(world): port World cycleStats/lastCycleStats — tick-section timings + bandwidth counters (backs 244 MAP_LAST* ops) [rev-244 B4]`

### Task 12: MAP_PRODUCTION + 12 MAP_LAST* debug ops

**Files:**
- Modify: `pkg/script/state.go` (WorldVars: rename `MapLive()` → `MapProduction()`; add `LastCycleStat(stat int) int`)
- Modify: `modules/world/server_varp.go` (rename impl :79-85; add `LastCycleStat` delegating to the Task 11 method)
- Modify: `pkg/script/handlers_debug.go` (13 new handlers) + `handlers.go` (registration)
- Test: `pkg/script/handlers_debug_test.go`

TS contract: `... show 9aadcec4:src/engine/script/handlers/DebugOps.ts | cat -n | sed -n '13,68p'`.

- [ ] **Step 1: Failing tests.**

```go
// TS 244 DebugOps.ts:16-18 (MAP_PRODUCTION) + :20-68 (MAP_LAST*).
func TestDebugWorldStatOps(t *testing.T) {
	s := newDebugOpState(t, withCycleStats([12]int{7, 1, 2, 3, 4, 5, 6, 8, 9, 10, 11, 12}), withProduction(true))
	if err := handleMapProduction(s); err != nil {
		t.Fatal(err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("MAP_PRODUCTION = %d, want 1", got)
	}
	for _, tc := range []struct {
		h    func(*ScriptState) error
		want int
	}{
		{handleMapLastClock, 7}, {handleMapLastWorld, 1}, {handleMapLastClientIn, 2},
		{handleMapLastNpc, 3}, {handleMapLastPlayer, 4}, {handleMapLastLogout, 5},
		{handleMapLastLogin, 6}, {handleMapLastZone, 8}, {handleMapLastClientOut, 9},
		{handleMapLastCleanup, 10}, {handleMapLastBandwidthIn, 11}, {handleMapLastBandwidthOut, 12},
	} {
		if err := tc.h(s); err != nil {
			t.Fatal(err)
		}
		if got := s.PopInt(); got != tc.want {
			t.Fatalf("= %d, want %d", got, tc.want)
		}
	}
}
```

(Stat-index order is the TS WorldStat enum: CYCLE=0, WORLD=1, CLIENT_IN=2, NPC=3, PLAYER=4, LOGOUT=5, LOGIN=6, ZONE=7, CLIENT_OUT=8, CLEANUP=9, BANDWIDTH_IN=10, BANDWIDTH_OUT=11 — the fake's array is keyed by that order; note MAP_LASTCLOCK reads CYCLE.)

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.** WorldVars rename + addition (cascade for fakes; `MapLive()` has zero remaining consumers after Task 1 deleted handleMapLive — the rename is safe). pkg/script needs the stat indexes — define them in handlers_debug.go as a mirror block (pkg/script cannot import modules/world):

```go
// WorldStat indexes (TS WorldStat.ts:1-14; mirrored in
// modules/world/world_stats.go — keep in sync).
const (
	worldStatCycle = iota
	worldStatWorld
	worldStatClientIn
	worldStatNpc
	worldStatPlayer
	worldStatLogout
	worldStatLogin
	worldStatZone
	worldStatClientOut
	worldStatCleanup
	worldStatBandwidthIn
	worldStatBandwidthOut
)

// handleMapProduction (MAP_PRODUCTION, opcode 10001) pushes the
// NODE_PRODUCTION flag. TS DebugOps.ts:16-18 — the 225 MAP_LIVE body,
// relocated to DebugOps and renamed at 244.
func handleMapProduction(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_PRODUCTION: %w", ErrNoWorld)
	}
	s.PushInt(s.World.MapProduction())
	return nil
}

// handleMapLastClock (MAP_LASTCLOCK, opcode 10002) — TS DebugOps.ts:20-22.
func handleMapLastClock(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTCLOCK: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatCycle))
	return nil
}
```

(remaining 11 identical shape, each with its DebugOps.ts citation line). Register all 13. In modules/world, change the Task 11 `LastCycleStat` only if its signature differs from the interface; `worldVarsView.LastCycleStat` delegates: `return w.s.LastCycleStat(stat)`.
- [ ] **Step 4: Run + commit.** `feat(script): 244 MAP_PRODUCTION + 12 MAP_LAST* debug ops over the cycle-stats surface [rev-244 B4]`

---

## Slice 5 — Audit

### Task 13: NO-OP verification batch + PORTING.md §B4 audit trail + full gates

**Files:**
- Modify: `PORTING.md` (new `### B4 — script runtime (2026-06-04)` subsection in the rev-244 Bundle audit trail; closure notes on the B2 IF_SETRECOL + IF_OPENOVERLAY rows; strike-notes where superseded)
- Modify: stale citations flagged during verification (citation-only edits)

- [ ] **Step 1: Verify each NO-OP with the `cat -n` listing; record verdict + evidence (file:line both sides) for the audit table.**
  1. RANDOM/RANDOMINC (NumberOps.ts:32-41 vs handlers_number.go:290-308): JavaRandom.ts:58-62 `checkIsPositiveInt` rejects only n<0; `nextInt(0)` → power-of-two branch → 0. goscape's `n<=0→0` / `n<0→0` ≡ 244 clamp. Go `rand.IntN` vs Java stream = pre-existing distribution-equivalent posture — cite the existing comment at handlers_number.go.
  2. STAT_RANDOM draw (`chance = nextInt(256)` vs goscape `rand.IntN(256)` — handlers_player.go ~:644-651): distribution-identical; existing comment already documents the draw substitution.
  3. GETQUEUE/CLEARQUEUE (PlayerOps.ts:894-906): iteration-style only; Go routes through `QueueCount`/`UnlinkQueuedScript` (handlers_player.go:2038,2051); B3 T12 pinned re-entry semantics (`player_queue_reentry_244_test.go`).
  4. ScriptFile STANDALONE_BUNDLE branch (ScriptFile.ts:136-143): NOT-PORTED, platform-inapplicable.
  5. Handler file moves + enum-position-only moves (NPC_HUNT→ServerOps, SPLIT_*→ServerOps, STRUCT_PARAM→ServerOps + StructOps.ts deleted, AFK_EVENT/GETTIMER/STAT_ADVANCE/TUT_*/WALKTRIGGER/STAT/STAT_HEAL/STAT_SUB/etc. position moves): values covered by Task 1; goscape file organization is its own. Update the moved handlers' `// TS <File>` citations (NPC_HUNT, SPLIT_*, STRUCT_PARAM now cite ServerOps.ts).
  6. IF_OPENCHAT/IF_OPENMAIN_SIDE + PROJANIM_PL pid: B3-shipped (`d5a70fb1`/`fcc7e212`) — audit-listed, not re-applied.
  7. InvOps whitespace hunks; DbOps/ScriptRunner/PlayerOps import churn; P_OPOBJ guard (Task 7 verdict).
  8. World.addPlayer (flag #4) and staffModLevel≥2 (flag #5): no B4-slice consumer — B3 rows stand.
- [ ] **Step 2: Write the §B4 audit subsection** — decision rows + the correspondence table mapping every scope-diff file to a commit/decision:

| TS file | maps to |
|---|---|
| ScriptOpcode.ts (226/206) | Task 1 commits |
| ScriptOpcodePointers.ts (27/12) | Task 1 |
| ScriptRunner.ts (4/6) | Task 9 (+StructOps import churn NO-OP) |
| ScriptState.ts (1/10) | Task 3 (huntIterator) + Task 4 (activePlayer2) |
| ScriptFile.ts (6/1) | NOT-PORTED (STANDALONE_BUNDLE) |
| ScriptIterators.ts (0/58) | Task 3 (PlayerHuntAllCommandIterator removal; PlayerIterator≡HuntIterator-PLAYER equivalence note) |
| ServerOps.ts (175/10) | Tasks 3, 7 (MAP_BLOCKED), 10, + MAP_LIVE deletion (Task 1) + moves (NO-OP) |
| DebugOps.ts (55/0) | Task 12 |
| PlayerOps.ts (40/72) | Tasks 1 (renames/STAT_TOTAL/IF_SETRECOL), 4, 8 + B3-shipped rows + GETQUEUE NO-OP |
| InvOps.ts (35/31) | Task 6 + whitespace NO-OP |
| NpcOps.ts (1/52) | Task 3 (moves) + Task 7 (STATHEAL) |
| StringOps.ts (1/51) / StructOps.ts (0/22) | moves — NO-OP (values Task 1) |
| DbOps.ts (9/21) | Task 5 |
| NumberOps.ts (4/4) | NO-OP (verified clamp equivalence) |

Plus externals: IF_SETRECOL wire (Task 2, closes B2 row), IF_OPENOVERLAY (Task 8, closes B2→B3 chain), cycle stats (Task 11, pre-existing-gap closure row with the section-mapping table + uint16-wrap + bandwidth-point notes), count accessors (Task 10). Tracker rows: (1) script.dat numbering window → B6; (2) cycle-stats gap closed; (3) NPC_FINDNEXT/npc_huntall split documented for content authors. Closed: NAI-162-D-STUB-PUSHVARBIT/POPVARBIT.
- [ ] **Step 3: Full gates.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 ; echo "exit=$?"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=1 go test -race ./modules/world/ ./pkg/script/ ./pkg/pack/... -count=1
```

Capture REAL exit codes. Marker audit: `grep -rn "PORTING-EXCEPTION" modules pkg cmd internal | wc -l` — record the count delta in the audit section.
- [ ] **Step 4: Commit.** `docs(porting): rev-244 B4 audit trail — script-runtime correspondence, NO-OP verdicts, tracker rows [rev-244 B4]`

---

## Final gate (process, not a task)

Whole-bundle integration review per the B1-B3 cadence: one reviewer subagent reads the full `git diff <pre-B4-commit>..HEAD` against the B4 scope diff + spec, hunting unmapped hunks, double-applied B3 work, and citation drift. Fix-or-document findings (verify "missing X" claims first — mandate #3), then hand off to B5/B6 per the umbrella.
