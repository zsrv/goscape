# HitType validator port + NpcStat read-path validator coverage — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the four remaining raw call sites in goscape's simple enum-range script-input validator family: port the `HitType` enum + `checkHitType` validator and apply at NPC_DAMAGE + DAMAGE; apply the already-existing `checkNpcStatID` at NPC_STAT + NPC_BASESTAT.

**Architecture:** New file `pkg/objtype/hittype.go` with three exported constants + `HitTypeCount`. New free-function `checkHitType(v int, op string) error` in `pkg/script/handlers_npc.go` alongside the existing `checkNpcStatID` / `checkNpcMode` / `checkNpcType` validators. Four call-site wraps; one doc-comment refresh.

**Tech Stack:** Go 1.26. Project conventions per `CLAUDE.md`: invoke commands with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`; PATH set via `unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"`; commits use `git commit --no-gpg-sign`; stage explicitly (config.yaml has standing drift). Spec: `docs/superpowers/specs/2026-05-20-hit-type-validator-port-design.md`.

---

## File Structure

| File | Status | Purpose |
|---|---|---|
| `pkg/objtype/hittype.go` | **CREATE** | HitType wire-value const group + `HitTypeCount` sentinel. |
| `pkg/objtype/hittype_test.go` | **CREATE** | Pin the three constant values + the count. |
| `pkg/script/handlers_npc.go` | **MODIFY** | Add `checkHitType` validator (near L93 alongside `checkNpcType`). Wrap `dmgType` at L351 (NPC_DAMAGE), `stat` at L174 (NPC_STAT) and L184 (NPC_BASESTAT). Refresh stale doc comment at L341-342. |
| `pkg/script/handlers_npc_test.go` | **MODIFY** | Add `TestCheckHitType` (alongside `TestCheckHuntVis` at L77-88). Add `TestHandleNpcDamage_InvalidHitType`, `TestHandleNpcStat_InvalidStat`, `TestHandleNpcBaseStat_InvalidStat`. |
| `pkg/script/handlers_player.go` | **MODIFY** | Wrap `hitType` at L1518 (handleDamage / P_DAMAGE). |
| `pkg/script/handlers_player_test.go` | **MODIFY** | Add `TestDamage_InvalidHitType` (near `TestDamage_HappyPath` at L4638). |

---

## Pre-flight

- [ ] **Step 0: Verify clean working state**

Run:
```bash
cd $HOME/Code/github.com/zsrv/goscape
git log --oneline -1
git status
```

Expected: HEAD shows `052821a9 spec(script): HitType validator port + NpcStat read-path validator coverage` (or a more recent commit on top of it). `git status` shows only `config.yaml` modified plus standing untracked noise (`.bash_profile`, `.claude/`, `.vscode`, etc.). **Do not stage or modify any of that noise.**

---

## Task 1: HitType const group + value pin

**Files:**
- Create: `pkg/objtype/hittype.go`
- Test: `pkg/objtype/hittype_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/objtype/hittype_test.go`:

```go
package objtype

import "testing"

// TestHitTypeConstants pins the three wire values + count sentinel.
// Mirrors TS Engine-TS/src/engine/entity/HitType.ts:1-5
// (BLOCK=0, DAMAGE=1, POISON=2).
func TestHitTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"HitTypeBlock", HitTypeBlock, 0},
		{"HitTypeDamage", HitTypeDamage, 1},
		{"HitTypePoison", HitTypePoison, 2},
		{"HitTypeCount", HitTypeCount, 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestHitTypeConstants -count=1 2>&1 | tail -10
```

Expected: FAIL with compile error mentioning undefined `HitTypeBlock` / `HitTypeDamage` / `HitTypePoison` / `HitTypeCount`.

- [ ] **Step 3: Create the const group**

Create `pkg/objtype/hittype.go`:

```go
package objtype

// HitType wire values used by the client hitmark encoding and by
// RuneScript callers of NPC_DAMAGE / P_DAMAGE. Mirrors TS
// Engine-TS/src/engine/entity/HitType.ts:1-5.
//
// HitTypeCount is the exclusive upper bound consumed by the
// HitTypeValid range validator (TS ScriptValidators.ts:117 —
// ScriptInputRangeValidator(0, 3)).
const (
	HitTypeBlock  = 0
	HitTypeDamage = 1
	HitTypePoison = 2

	HitTypeCount = 3
)
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestHitTypeConstants -count=1 -v 2>&1 | tail -10
```

Expected: `--- PASS: TestHitTypeConstants` + `ok` line.

- [ ] **Step 5: Verify no broader regression in objtype**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -count=1 2>&1 | tail -5
```

Expected: `ok  github.com/zsrv/goscape/pkg/objtype` (no failures).

- [ ] **Step 6: Commit**

```bash
git status
git add pkg/objtype/hittype.go pkg/objtype/hittype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): port HitType const group (T1)

Adds pkg/objtype/hittype.go with HitTypeBlock=0, HitTypeDamage=1,
HitTypePoison=2 + HitTypeCount=3 sentinel. Mirrors TS
Engine-TS/src/engine/entity/HitType.ts:1-5. HitTypeCount is the
exclusive upper bound consumed by the upcoming checkHitType
validator (ScriptValidators.ts:117 range [0, 3)).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `2 files changed, X insertions(+)`.

---

## Task 2: checkHitType validator + unit test

**Files:**
- Modify: `pkg/script/handlers_npc.go` (insert after `checkNpcType` at L93)
- Test: `pkg/script/handlers_npc_test.go` (insert near `TestCheckHuntVis` at L77-88)

- [ ] **Step 1: Write the failing test**

Insert into `pkg/script/handlers_npc_test.go` immediately after `TestCheckCategoryType` (currently ends around L100):

```go
// TestCheckHitType pins the [0, HitTypeCount) range check. Mirrors
// TS HitTypeValid (ScriptValidators.ts:117) — ScriptInputRangeValidator(0, 3).
func TestCheckHitType(t *testing.T) {
	for _, v := range []int{0, 1, 2} {
		if err := checkHitType(v, "TEST"); err != nil {
			t.Errorf("checkHitType(%d): unexpected error %v", v, err)
		}
	}
	for _, v := range []int{-1, 3, 100} {
		if err := checkHitType(v, "TEST"); err == nil {
			t.Errorf("checkHitType(%d): want error", v)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckHitType -count=1 2>&1 | tail -10
```

Expected: FAIL with compile error mentioning undefined `checkHitType`.

- [ ] **Step 3: Add the validator**

Insert into `pkg/script/handlers_npc.go` immediately after the `checkNpcType` function (currently ends at L93). The exact insertion point: after the closing `}` of `checkNpcType` and before the next blank line / next function.

```go
// checkHitType validates a hit-type wire value against
// objtype.HitTypeCount. Mirrors TS HitTypeValid (ScriptValidators.ts:117)
// — ScriptInputRangeValidator(0, 3). Accepts BLOCK / DAMAGE / POISON.
func checkHitType(v int, op string) error {
	if v < 0 || v >= objtype.HitTypeCount {
		return fmt.Errorf("%s: hit type out of range (%d)", op, v)
	}
	return nil
}
```

The `objtype` and `fmt` imports already exist in this file (used by `checkNpcStatID` and others).

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckHitType -count=1 -v 2>&1 | tail -10
```

Expected: `--- PASS: TestCheckHitType` + `ok` line.

- [ ] **Step 5: Commit**

```bash
git status
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): add checkHitType validator (T2)

Adds checkHitType(v int, op string) error in pkg/script/handlers_npc.go
alongside the existing checkNpcStatID / checkNpcMode / checkNpcType
free-function validators. Mirrors TS HitTypeValid
(ScriptValidators.ts:117) — ScriptInputRangeValidator(0, 3). Validator
is defined but not yet applied at call sites (T3/T4 wire it in).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `2 files changed, X insertions(+)`.

---

## Task 3: Apply checkHitType at NPC_DAMAGE + DAMAGE; refresh doc comment

**Files:**
- Modify: `pkg/script/handlers_npc.go` (L341-354, handleNpcDamage)
- Modify: `pkg/script/handlers_player.go` (L1504-1529, handleDamage)
- Test: `pkg/script/handlers_npc_test.go` (new test near existing `TestHandleNpcDamageNullAmountRejected` at L1723)
- Test: `pkg/script/handlers_player_test.go` (new test near existing `TestDamage_HappyPath` at L4638)

- [ ] **Step 1: Write the failing tests**

Insert into `pkg/script/handlers_npc_test.go` immediately after `TestHandleNpcDamageNullAmountRejected` (currently ends around L1755):

```go
// TestHandleNpcDamage_InvalidHitType pins that NPC_DAMAGE rejects
// dmgType outside [0, HitTypeCount). Mirrors TS NpcOps.ts:265 —
// check(state.popInt(), HitTypeValid).
func TestHandleNpcDamage_InvalidHitType(t *testing.T) {
	npc := &mockNpc{}
	// Pop order: amount (top), dmgType. Push dmgType=3 (out of range), amount=5.
	sf := &ScriptFile{
		Name: "npc_damage_invalid_hittype",
		Opcodes: []Opcode{
			OpPushConstantInt, // push dmgType (3 — out of range)
			OpPushConstantInt, // push amount (5)
			OpNpcDamage,
			OpReturn,
		},
		IntOperands: []int32{3, 5, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for dmgType=3, got nil")
	}
	want := "NPC_DAMAGE: hit type out of range (3)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(npc.damageCalls) != 0 {
		t.Errorf("damageCalls: got %d, want 0 (must not damage on rejection)", len(npc.damageCalls))
	}
}
```

Insert into `pkg/script/handlers_player_test.go` immediately after `TestDamage_NoPointerGate` (currently ends around L4675):

```go
// TestDamage_InvalidHitType pins that DAMAGE (P_DAMAGE) rejects
// hitType outside [0, HitTypeCount). Mirrors TS PlayerOps.ts:778 —
// check(state.popInt(), HitTypeValid). The validator short-circuits
// before the uid pop, so no UID lookup or ApplyDamage occurs.
func TestDamage_InvalidHitType(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	// uid=42, hitType=3 (out of range), amount=5
	s := newDamageState(mw, 42, 3, 5)
	err := handleDamage(s)
	if err == nil {
		t.Fatalf("handleDamage: want error for hitType=3, got nil")
	}
	want := "DAMAGE: hit type out of range (3)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if got := len(target.applyDamageCalls); got != 0 {
		t.Errorf("applyDamageCalls: got %d, want 0 (must not damage on rejection)", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcDamage_InvalidHitType|TestDamage_InvalidHitType' -count=1 -v 2>&1 | tail -20
```

Expected: both tests FAIL — they expect `err != nil` and a specific "hit type out of range" message, but the handlers currently accept any dmgType/hitType. The TS-test will fail because `Execute(state)` succeeds (returns nil) and the NPC actually gets damaged.

- [ ] **Step 3: Apply checkHitType at NPC_DAMAGE**

In `pkg/script/handlers_npc.go`, locate `handleNpcDamage` (currently around L343-354). Replace the function:

```go
// handleNpcDamage pops (type, amount) in TS order (amount on top) and
// applies damage. The concrete Npc impl manages HP; this handler stays thin.
// Mirrors TS NpcOps.ts NPC_DAMAGE: check(amount, NumberNotNull) +
// check(dmgType, HitTypeValid). Goscape mirrors via checkNotNull +
// checkHitType.
func handleNpcDamage(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DAMAGE"); err != nil {
		return err
	}
	amount := s.PopInt()
	if err := checkNotNull(amount, "NPC_DAMAGE"); err != nil {
		return err
	}
	dmgType := s.PopInt()
	if err := checkHitType(dmgType, "NPC_DAMAGE"); err != nil {
		return err
	}
	s.ActiveNpc.Damage(amount, dmgType)
	return nil
}
```

Changes vs current: (1) doc comment refreshed — removed "wrapped with HitTypeValid (not NumberNotNull) and stays raw (NAI-23 Bundle 4a)" wording; (2) new `checkHitType` call inserted after the `PopInt` for `dmgType`.

- [ ] **Step 4: Apply checkHitType at DAMAGE / P_DAMAGE**

In `pkg/script/handlers_player.go`, locate `handleDamage` (currently around L1516-1529). Replace the function body (keeping the existing doc comment block at L1504-1515 intact):

```go
func handleDamage(s *ScriptState) error {
	amount := s.PopInt()
	hitType := s.PopInt()
	if err := checkHitType(hitType, "DAMAGE"); err != nil {
		return err
	}
	uid := s.PopInt()
	if s.World == nil {
		return nil
	}
	player := s.World.LookupPlayerByUID(uid)
	if player == nil {
		return nil
	}
	player.ApplyDamage(amount, hitType)
	return nil
}
```

The only change is the inserted three-line `if err := checkHitType(...) { return err }` block between the `hitType := s.PopInt()` and `uid := s.PopInt()` lines. All other lines untouched.

Note: `checkHitType` lives in package `pkg/script/` (same package as `handlers_player.go`), so no import change is needed.

- [ ] **Step 5: Run new tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcDamage_InvalidHitType|TestDamage_InvalidHitType' -count=1 -v 2>&1 | tail -15
```

Expected: both tests PASS.

- [ ] **Step 6: Verify no regression in pkg/script/**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1 2>&1 | tail -5
```

Expected: `ok  github.com/zsrv/goscape/pkg/script`. In particular, the existing `TestHandleNpcDamageNullAmountRejected`, `TestDamage_HappyPath`, `TestDamage_UnknownUID`, `TestDamage_NoPointerGate` must still pass.

- [ ] **Step 7: Commit**

```bash
git status
git add pkg/script/handlers_npc.go pkg/script/handlers_player.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): apply checkHitType at NPC_DAMAGE + DAMAGE (T3)

Wraps dmgType pop at handleNpcDamage with checkHitType and refreshes
the stale "stays raw (NAI-23 Bundle 4a)" doc comment to reflect the
new validator. Wraps hitType pop at handleDamage (P_DAMAGE) with
checkHitType. The DAMAGE validator runs before the uid pop, so
invalid hitType halts the script before any UID lookup or
ApplyDamage call.

Adds 2 new tests: TestHandleNpcDamage_InvalidHitType and
TestDamage_InvalidHitType. Existing happy-path tests
(TestHandleNpcDamageNullAmountRejected, TestDamage_HappyPath,
TestDamage_UnknownUID, TestDamage_NoPointerGate) continue to pass.

Mirrors TS NpcOps.ts:265 + PlayerOps.ts:778.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `4 files changed, X insertions(+), Y deletions(-)`.

---

## Task 4: Apply checkNpcStatID at NPC_STAT + NPC_BASESTAT

**Files:**
- Modify: `pkg/script/handlers_npc.go` (L170-187, handleNpcStat + handleNpcBaseStat)
- Test: `pkg/script/handlers_npc_test.go` (new tests near `TestNpcStatHP` at L473 and `TestNpcBaseStat` at L489)

- [ ] **Step 1: Write the failing tests**

Insert into `pkg/script/handlers_npc_test.go` immediately after `TestNpcStatOtherReturnsZero` (currently ends around L487):

```go
// TestHandleNpcStat_InvalidStat pins that NPC_STAT rejects a stat id
// outside [0, NpcStatCount). Mirrors TS NpcOps.ts NPC_STAT —
// check(state.popInt(), NpcStatValid).
func TestHandleNpcStat_InvalidStat(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(objtype.NpcStatCount) // 6 — exclusive upper bound, must reject
	err := handleNpcStat(s)
	if err == nil {
		t.Fatalf("handleNpcStat: want error for stat=%d, got nil", objtype.NpcStatCount)
	}
	want := "NPC_STAT: npc stat id out of range"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
}
```

Insert immediately after `TestNpcBaseStat` (currently ends around L495):

```go
// TestHandleNpcBaseStat_InvalidStat pins that NPC_BASESTAT rejects a
// stat id outside [0, NpcStatCount). Mirrors TS NpcOps.ts NPC_BASESTAT
// — check(state.popInt(), NpcStatValid).
func TestHandleNpcBaseStat_InvalidStat(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1) // null sentinel, must reject
	err := handleNpcBaseStat(s)
	if err == nil {
		t.Fatalf("handleNpcBaseStat: want error for stat=-1, got nil")
	}
	want := "NPC_BASESTAT: npc stat id out of range"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
}
```

(One test pins the upper-bound rejection, the other the lower-bound — both cover the same validator from different angles. `objtype` is already imported in `handlers_npc_test.go`; `strings` import too.)

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcStat_InvalidStat|TestHandleNpcBaseStat_InvalidStat' -count=1 -v 2>&1 | tail -20
```

Expected: both tests FAIL — the current handlers accept any stat and silently push 0 (the `mockNpc.NpcStat` / `NpcBaseStat` defaults), so `err` comes back nil.

- [ ] **Step 3: Wrap NPC_STAT and NPC_BASESTAT with checkNpcStatID**

In `pkg/script/handlers_npc.go`, replace `handleNpcStat` (currently L170-177) with:

```go
// handleNpcStat pops a stat id and pushes the NPC's current (boosted)
// level for that stat. Mirrors TS NpcOps.ts NPC_STAT —
// check(state.popInt(), NpcStatValid). Goscape mirrors via
// checkNpcStatID.
func handleNpcStat(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STAT"); err != nil {
		return err
	}
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_STAT"); err != nil {
		return err
	}
	s.PushInt(s.ActiveNpc.NpcStat(stat))
	return nil
}
```

Replace `handleNpcBaseStat` (currently L180-187) with:

```go
// handleNpcBaseStat pops a stat id and pushes the NPC's base level.
// Mirrors TS NpcOps.ts NPC_BASESTAT — check(state.popInt(),
// NpcStatValid). Goscape mirrors via checkNpcStatID.
func handleNpcBaseStat(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_BASESTAT"); err != nil {
		return err
	}
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_BASESTAT"); err != nil {
		return err
	}
	s.PushInt(s.ActiveNpc.NpcBaseStat(stat))
	return nil
}
```

Both changes: insert a `if err := checkNpcStatID(stat, "..."); err != nil { return err }` block between the `PopInt` and the `PushInt`, and add a TS-reference sentence to the doc comment.

- [ ] **Step 4: Run new tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcStat_InvalidStat|TestHandleNpcBaseStat_InvalidStat' -count=1 -v 2>&1 | tail -10
```

Expected: both tests PASS.

- [ ] **Step 5: Verify no regression in existing NPC_STAT / NPC_BASESTAT tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNpcStat|TestNpcBaseStat' -count=1 -v 2>&1 | tail -20
```

Expected: existing `TestNpcStatHP`, `TestNpcStatOtherReturnsZero` (stat=5 is in-range), `TestNpcBaseStat` all PASS plus the two new tests PASS.

Note: `TestNpcStatOtherReturnsZero` uses `stat=5` which IS in range `[0, 6)`, so it continues to pass the validator and pushes 0 (mockNpc default).

- [ ] **Step 6: Commit**

```bash
git status
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): apply checkNpcStatID at NPC_STAT + NPC_BASESTAT (T4)

Wraps the stat-id pop at handleNpcStat and handleNpcBaseStat with the
existing checkNpcStatID validator (introduced for the write-path
handlers in NAI-120 Bundle 2C but never wired to the read-path).
Mirrors TS NpcOps.ts NPC_STAT / NPC_BASESTAT —
check(state.popInt(), NpcStatValid).

Adds 2 new tests: TestHandleNpcStat_InvalidStat (upper-bound rejection
at stat=NpcStatCount=6) and TestHandleNpcBaseStat_InvalidStat
(lower-bound rejection at stat=-1). Existing TestNpcStatHP,
TestNpcStatOtherReturnsZero (stat=5 in-range), TestNpcBaseStat all
continue to pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `2 files changed, X insertions(+), Y deletions(-)`.

---

## Task 5: Full-suite verification + close

**Files:** none modified.

- [ ] **Step 1: Run full `-race ./...`**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./... 2>&1 | tail -20
```

Expected: every package shows `ok` (or `?` for packages without tests). No `FAIL` lines. Typical runtime: ~150-160s.

If any package shows FAIL, STOP and investigate — do not proceed to close until clean.

- [ ] **Step 2: Run smoke-pack**

Run:
```bash
ls scripts/ 2>/dev/null | grep -iE 'smoke|pack'
```

Locate the smoke-pack script in the repo (typically `scripts/smoke-pack.sh` or similar). Run it. Expected: `12 OK / 0 ERR / 0 SKIP`.

If the smoke-pack script is unobvious, check `Makefile` for a `smoke` or `smoke-pack` target:

```bash
grep -nE 'smoke|smoke-pack' Makefile 2>/dev/null
```

If no smoke-pack target exists in this repo, skip this step and note it in the close commit. Recent close commits all reference smoke-pack passing — confirm with the user before skipping.

- [ ] **Step 3: Final state inspection**

Run:
```bash
git log --oneline 052821a9..HEAD
git status
```

Expected: 4 commits since spec (`feat(objtype): port HitType const group`, `feat(script): add checkHitType validator`, `feat(script): apply checkHitType at NPC_DAMAGE + DAMAGE`, `feat(script): apply checkNpcStatID at NPC_STAT + NPC_BASESTAT`). `git status` shows only the standing `config.yaml` drift + untracked noise.

- [ ] **Step 4: Final close commit**

This is an empty commit that marks slice closure for future audit/grep, following the project's `chore(close)` convention seen in recent NAI-XX closes.

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): HitType validator port + NpcStat read-path validator coverage

Closes the 4 remaining raw call sites in the simple enum-range
validator family:
- NPC_DAMAGE dmgType  → checkHitType
- DAMAGE  hitType     → checkHitType
- NPC_STAT stat       → checkNpcStatID
- NPC_BASESTAT stat   → checkNpcStatID

New surface: pkg/objtype/hittype.go (3 wire constants + count
sentinel); pkg/script/handlers_npc.go::checkHitType validator. Five
new tests (1 const pin + 1 validator unit + 4 handler invalid-input).
Stale doc comment at handlers_npc.go:341-342 ("stays raw (NAI-23
Bundle 4a)") refreshed.

Pure script-input safety; no behavioral change for valid inputs.
-race ./... clean; smoke-pack 12 OK / 0 ERR / 0 SKIP.

Spec: docs/superpowers/specs/2026-05-20-hit-type-validator-port-design.md
Plan: docs/superpowers/plans/2026-05-20-hit-type-validator-port.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: empty commit landed with the close message.

---

## Acceptance criteria checklist

After all tasks complete, verify each spec §10 acceptance criterion:

- [ ] AC1: `pkg/objtype/hittype.go` exists with `HitTypeBlock=0`, `HitTypeDamage=1`, `HitTypePoison=2`, `HitTypeCount=3`.
- [ ] AC2: `checkHitType` exists in `pkg/script/handlers_npc.go`.
- [ ] AC3: Four handler call sites apply their respective validator immediately after the relevant `PopInt`:
  - `handleNpcDamage` → `checkHitType(dmgType, "NPC_DAMAGE")`
  - `handleDamage` → `checkHitType(hitType, "DAMAGE")`
  - `handleNpcStat` → `checkNpcStatID(stat, "NPC_STAT")`
  - `handleNpcBaseStat` → `checkNpcStatID(stat, "NPC_BASESTAT")`
- [ ] AC4: Doc comment at `handlers_npc.go::handleNpcDamage` no longer says "stays raw" or "NAI-23 Bundle 4a".
- [ ] AC5: 6 new tests added and passing: `TestHitTypeConstants` (T1), `TestCheckHitType` (T2), `TestHandleNpcDamage_InvalidHitType` (T3), `TestDamage_InvalidHitType` (T3), `TestHandleNpcStat_InvalidStat` (T4), `TestHandleNpcBaseStat_InvalidStat` (T4). Spec §8 counted 5; plan adds one more (the const-pin in T1) as a regression guard for the wire-value contract.
- [ ] AC6: `go test -race ./...` clean.
- [ ] AC7: Smoke-pack 12 OK / 0 ERR / 0 SKIP (or noted as N/A with user confirmation).
