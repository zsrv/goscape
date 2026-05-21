# SETGENDER body port + GenderValid validator — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Always invoke the `use-modern-go` skill at the start of every implementer dispatch.

**Goal:** Replace the `handleSetGender` TS-unimplemented stub with a faithful port of TS `PlayerOps.ts:1104-1118`. Add `checkGender(v int, op string) error` (TS `GenderValid`, inclusive `[0, 1]`). Port `Player.MALE_FEMALE_MAP` / `Player.FEMALE_MALE_MAP` body-recoloring lookup tables. Retire `NAI-162-D-STUB-SETGENDER`. Open `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED` (TS-literal `Map.get() ?? -1` writes -1 garbage idkit on unmapped keys).

**Architecture:** Thin handler + fat setter. Validator is a bare-number free function alongside `checkSkinColour` / `checkQueue`. Lookup maps are package-level `map[int]int` vars in a new `modules/world/player_gender.go` file alongside the `Player` struct (mirrors TS class-level statics at `Player.ts:110-188`). `(*Player).SetGender(gender int)` contains the for-loop + map lookups + slot-1 hardcode + final `p.gender = gender` write. SETGENDER does NOT flip `MaskAppearance` — TS-faithful deferred-rebuild pattern (callers follow with BUILDAPPEARANCE per `makeover_mage.rs2:58-64`).

**Tech Stack:** Go 1.26. Project conventions per `CLAUDE.md`: prefix Go commands with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`; PATH set via `unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"` if needed; commits use `git commit --no-gpg-sign`; stage explicitly (the working tree has standing noise — `config.yaml`, untracked dotfiles, `RUNESCRIPT.md` — **never stage these**). Spec: `docs/superpowers/specs/2026-05-20-setgender-genderval-port-design.md` (commit `46e3bd58`).

---

## File Structure

| File | Status | Purpose |
|---|---|---|
| `modules/world/player_gender.go` | **CREATE** | `maleFemaleMap` + `femaleMaleMap` + `lookupGenderIdkit` helper + `(*Player).SetGender(gender int)`. Carries `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED` pin in `SetGender` doc comment. |
| `modules/world/player_gender_test.go` | **CREATE** | 6 `TestPlayerSetGender_*` tests pinning the TS-faithful behavior. |
| `pkg/script/handlers_player.go` | **MODIFY** | Add `checkGender` (insert after `checkSkinColour` at L99). Add real `handleSetGender` (insert after `handleSetSkinColour` at L1678). |
| `pkg/script/handlers_player_test.go` | **MODIFY** | Add `TestCheckGender_Range` (near `TestCheckSkinColour_Range` at L1953-1972). Add 4 `TestHandleSetGender_*` tests (near `TestHandleSetSkinColour_*` at L5082+). |
| `pkg/script/handlers_b0_stubs.go` | **MODIFY** | Delete the `handleSetGender` stub block (L21-25). Update the file's top-of-file context comment if needed (the file describes 6 stubs; after this slice, 5 remain). |
| `pkg/script/handlers_b0_stubs_test.go` | **MODIFY** | Drop the `{"SET_GENDER", OpSetGender, "SET_GENDER: unimplemented"}` row (L20). Update the test's leading doc comment (L8-11) to reference 5 stubs, not 6. |
| `pkg/script/active.go` | **MODIFY** | Add `SetGender(gender int)` to the `ActivePlayer` interface (insert after `SetColorPart` at L743). |
| `pkg/script/runner_test.go` | **MODIFY** | Add `setGenderCalls []int` to `mockPlayer` struct (near L348-350 alongside `bodyParts` / `colorParts`). Add `(m *mockPlayer) SetGender` impl (near L771-773 alongside `Gender` / `SetBodyPart` / `SetColorPart`). |

No `pkg/objtype/` changes (gender is a 0/1 wire value with no named-enum types in TS).

---

## Pre-flight

- [ ] **Step 0: Verify clean working state**

Run:
```bash
cd /home/owner/Code/github.com/zsrv/goscape
git log --oneline -3
git status
```

Expected: HEAD shows `46e3bd58 docs(spec): SETGENDER body port + GenderValid validator` on top of `7badce7d chore(close): Queue + SkinColour validator port` on top of `996026d1 docs(script): SkinColour bracket-space consistency`. `git status` shows `config.yaml` modified plus standing untracked noise (`.bash_profile`, `.bashrc`, `.claude/`, `.gitconfig`, `.gitmodules`, `.mcp.json`, `.profile`, `.ripgreprc`, `.vscode`, `.zprofile`, `.zshrc`, `RUNESCRIPT.md`). **Do not stage or modify any of that noise.**

- [ ] **Step 1: Establish baseline gate**

Run:
```bash
unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... ./modules/world/... 2>&1 | tail -5
```

Expected: all packages OK. If anything fails BEFORE you start, stop and report — that's not your fault to fix.

Heads-up on env quirk (carry-forward from predecessor close memory): `/home/owner/go/current` symlink may point to a nonexistent Go version (`go1.26.2` while actual install is `go1.26.3`). Tests still run via fallback `GOROOT` resolution, but if you see "no such file or directory" from `go` invocations, refresh the symlink:
```bash
ls /home/owner/go/
ln -sfn /home/owner/go/go1.26.X /home/owner/go/current
```

- [ ] **Step 2: Pre-commit safety reminder**

Before EVERY commit in this plan, run `git status` first to confirm only your intended files are staged, and `git show --stat HEAD` after to confirm the commit landed cleanly. See memory `[[git-pre-commit-status-check]]`: concurrent shell activity can stage things between session-start and `git commit`; the safest recovery for an accidental stage is `git reset --mixed HEAD~1`, never `--amend`.

---

## Task 1: `checkGender` validator + unit tests

**Files:**
- Modify: `pkg/script/handlers_player.go` — add `checkGender` after `checkSkinColour` at line 99
- Modify: `pkg/script/handlers_player_test.go` — add `TestCheckGender_Range` after `TestCheckSkinColour_Range` at line 1972

- [ ] **Step 1.1: Write the failing test**

Open `pkg/script/handlers_player_test.go`. Find `TestCheckSkinColour_Range` ending at line 1972. Insert the following directly after (before the next test, `TestPAnimProtectHappyPathZero`):

```go
// TestCheckGender_Range pins the [0, 1] inclusive range check.
// Mirrors TS GenderValid (ScriptValidators.ts:136) —
// ScriptInputRangeValidator(0, 1, 'Gender').
func TestCheckGender_Range(t *testing.T) {
	for _, v := range []int{0, 1} {
		if err := checkGender(v, "TEST_OP"); err != nil {
			t.Errorf("checkGender(%d): unexpected error %v", v, err)
		}
	}
	for _, v := range []int{-1, 2, 100, math.MinInt, math.MaxInt} {
		err := checkGender(v, "TEST_OP")
		if err == nil {
			t.Errorf("checkGender(%d): want error, got nil", v)
			continue
		}
		if !strings.Contains(err.Error(), "TEST_OP") {
			t.Errorf("checkGender(%d): error %q missing op name TEST_OP", v, err)
		}
	}
}
```

- [ ] **Step 1.2: Run test to verify it fails (function not defined)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... -run TestCheckGender_Range 2>&1 | tail -10
```

Expected: BUILD FAIL with message like `undefined: checkGender`.

- [ ] **Step 1.3: Write the validator**

Open `pkg/script/handlers_player.go`. Find `checkSkinColour` ending at line 99 with closing `}`. Insert the following directly after that line:

```go

// checkGender validates a player gender wire value. Mirrors TS
// GenderValid (ScriptValidators.ts:136) —
// ScriptInputRangeValidator(0, 1, 'Gender'), inclusive range [0, 1].
// 0 = male, 1 = female.
func checkGender(v int, op string) error {
	if v < 0 || v > 1 {
		return fmt.Errorf("%s: gender out of range (%d)", op, v)
	}
	return nil
}
```

- [ ] **Step 1.4: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... -run TestCheckGender_Range 2>&1 | tail -5
```

Expected: PASS.

- [ ] **Step 1.5: Wider package gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... 2>&1 | tail -5
```

Expected: all green.

- [ ] **Step 1.6: Commit**

```bash
git status
```

Expected staged set (after `git add`): `pkg/script/handlers_player.go` and `pkg/script/handlers_player_test.go` only. `config.yaml` and untracked noise must remain unstaged.

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): add checkGender validator

Adds checkGender(v int, op string) error mirroring TS GenderValid
(ScriptValidators.ts:136) — ScriptInputRangeValidator(0, 1, 'Gender'),
inclusive [0, 1]. Sibling to checkSkinColour/checkQueue/checkHitType
in the bare-number free-function style.

T1 of SETGENDER body port + GenderValid slice (spec 46e3bd58).
EOF
)"
git show --stat HEAD
```

Expected: 2 files changed, ~30 insertions.

---

## Task 2: Gender lookup maps + `(*Player).SetGender` + setter unit tests

**Files:**
- Create: `modules/world/player_gender.go`
- Create: `modules/world/player_gender_test.go`

This task adds the worker — `(*Player).SetGender` containing the for-loop + map lookups + slot-1 hardcode + final gender write. Tests pin the TS-faithful behavior including the new deviation `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED` and the deferred-rebuild assertion.

- [ ] **Step 2.1: Write the failing tests**

Create `modules/world/player_gender_test.go`:

```go
package world

import "testing"

// TestPlayerSetGender_MaleToFemale_RewritesAllSlotsViaMap pins TS
// PlayerOps.ts:1104-1118 male→female direction. Body slots are
// rewritten via maleFemaleMap; gender field set to 1.
func TestPlayerSetGender_MaleToFemale_RewritesAllSlotsViaMap(t *testing.T) {
	p := &Player{}
	p.body = [7]int{0, 1, 2, 3, 4, 5, 6}
	p.gender = 0

	p.SetGender(1)

	want := [7]int{45, 47, 48, 49, 50, 51, 52}
	if p.body != want {
		t.Errorf("body: got %v, want %v", p.body, want)
	}
	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
}

// TestPlayerSetGender_FemaleToMale_Slot1HardcodedTo14 pins the
// TS slot-1 special case (PlayerOps.ts:1111-1113):
//
//	if (i === 1) { state.activePlayer.body[i] = 14; continue; }
//
// On female→male direction, body[1] is forced to 14 even when
// femaleMaleMap[body[1]] would yield a different value. Deliberate
// TS canon for the canonical male hair model — not a bug.
func TestPlayerSetGender_FemaleToMale_Slot1HardcodedTo14(t *testing.T) {
	p := &Player{}
	p.body = [7]int{45, 47, 48, 49, 50, 51, 52}
	p.gender = 1

	p.SetGender(0)

	// femaleMaleMap[47] == 1, but slot 1 is hardcoded to 14.
	want := [7]int{0, 14, 2, 3, 4, 5, 6}
	if p.body != want {
		t.Errorf("body: got %v, want %v (slot 1 must be 14 hardcode)", p.body, want)
	}
	if p.gender != 0 {
		t.Errorf("gender: got %d, want 0", p.gender)
	}
}

// TestPlayerSetGender_FemaleToMale_NonSlot1UsesMap pins that slots
// other than 1 go through femaleMaleMap on the female→male direction.
// Spot-checks a few keys including a lossy-collapse case.
func TestPlayerSetGender_FemaleToMale_NonSlot1UsesMap(t *testing.T) {
	p := &Player{}
	// Use female values that exercise both 1:1 keys and the {73, 74, 77}→36
	// lossy-collapse case in slot 0.
	p.body = [7]int{73, 47, 56, 65, 76, 77, 81}
	p.gender = 1

	p.SetGender(0)

	// slot 0: 73 → 36 (lossy, both 73 and 77 collapse to 36)
	// slot 1: hardcoded to 14
	// slot 2: 56 → 18
	// slot 3: 65 → 29
	// slot 4: 76 → 39
	// slot 5: 77 → 36 (lossy)
	// slot 6: 81 → 44
	want := [7]int{36, 14, 18, 29, 39, 36, 44}
	if p.body != want {
		t.Errorf("body: got %v, want %v", p.body, want)
	}
}

// TestPlayerSetGender_UnmappedKeysWriteMinusOne pins
// NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED: TS-literal
// `Map.get(k) ?? -1` writes -1 to the slot when the current
// body[i] is not present in the relevant lookup map.
//
// Real content cannot reach this case (the makeover_mage UI flow
// constrains body[] to mapped values), but the behavior is TS-literal
// and pinned for future TS sync.
func TestPlayerSetGender_UnmappedKeysWriteMinusOne(t *testing.T) {
	t.Run("male->female direction", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{999, 999, 999, 999, 999, 999, 999}
		p.gender = 0

		p.SetGender(1)

		want := [7]int{-1, -1, -1, -1, -1, -1, -1}
		if p.body != want {
			t.Errorf("body: got %v, want %v (unmapped keys must write -1)", p.body, want)
		}
	})
	t.Run("female->male direction (slot 1 still hardcoded)", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{999, 999, 999, 999, 999, 999, 999}
		p.gender = 1

		p.SetGender(0)

		// Slot 1 is hardcoded to 14 regardless of map lookup. Other slots → -1.
		want := [7]int{-1, 14, -1, -1, -1, -1, -1}
		if p.body != want {
			t.Errorf("body: got %v, want %v", p.body, want)
		}
	})
}

// TestPlayerSetGender_DoesNotFlipMaskAppearance pins the TS-faithful
// deferred-rebuild assertion. SETGENDER must NOT flip MaskAppearance —
// callers must invoke BUILDAPPEARANCE explicitly (per
// makeover_mage.rs2:58-64 content evidence).
func TestPlayerSetGender_DoesNotFlipMaskAppearance(t *testing.T) {
	t.Run("male->female", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{0, 1, 2, 3, 4, 5, 6}
		p.masks = 0

		p.SetGender(1)

		if p.masks != 0 {
			t.Errorf("masks: got %d, want 0 (SETGENDER must not flip MaskAppearance)", p.masks)
		}
	})
	t.Run("female->male", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{45, 47, 48, 49, 50, 51, 52}
		p.masks = 0

		p.SetGender(0)

		if p.masks != 0 {
			t.Errorf("masks: got %d, want 0", p.masks)
		}
	})
}

// TestPlayerSetGender_LossyCollapse documents that the TS map pair is
// intentionally NOT a full bijection. body[0]=19 (a male in the {18..25}
// cohort that all collapse to female 56) round-trips to canonical 18,
// not 19. Mirrors OSRS canon — the makeover-mage isn't fully reversible.
func TestPlayerSetGender_LossyCollapse(t *testing.T) {
	p := &Player{}
	p.body = [7]int{19, 0, 0, 0, 0, 0, 0}
	p.gender = 0

	p.SetGender(1) // body[0]: 19 → 56
	if p.body[0] != 56 {
		t.Fatalf("after M→F: body[0]=%d, want 56", p.body[0])
	}

	p.SetGender(0) // body[0]: 56 → 18 (canonical, NOT 19)
	if p.body[0] != 18 {
		t.Errorf("after F→M: body[0]=%d, want 18 (canonical lossy collapse, NOT 19)", p.body[0])
	}
}
```

- [ ] **Step 2.2: Run tests to verify they fail (file does not exist yet)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./modules/world/... -run TestPlayerSetGender 2>&1 | tail -10
```

Expected: BUILD FAIL with messages like `undefined: maleFemaleMap` and `p.SetGender undefined (type *Player has no field or method SetGender)`.

- [ ] **Step 2.3: Write the production code**

Create `modules/world/player_gender.go`:

```go
package world

// maleFemaleMap is the male-body-idkit → female-body-idkit lookup used by
// (*Player).SetGender when gender == 1 (target female). Mirrors TS
// Player.MALE_FEMALE_MAP at Engine-TS/src/engine/entity/Player.ts:110-148.
// Sparse: keys are real OSRS male idkit ids; unmapped keys lookup as -1
// (see lookupGenderIdkit).
//
// Lossiness is canonical OSRS: males {18..25} all collapse to female 56;
// males {27, 31} both → female 63; the makeover-mage is not fully
// reversible.
var maleFemaleMap = map[int]int{
	0: 45, 1: 47, 2: 48, 3: 49, 4: 50, 5: 51, 6: 52, 7: 53, 8: 54, 9: 55,
	18: 56, 19: 56, 20: 56, 21: 56, 22: 56, 23: 56, 24: 56, 25: 56,
	26: 61, 27: 63, 28: 62, 29: 65, 30: 64, 31: 63, 32: 66, 33: 67,
	34: 68, 35: 69, 36: 70, 37: 71, 38: 72, 39: 76, 40: 75, 41: 78,
	42: 79, 43: 80, 44: 81,
}

// femaleMaleMap mirrors TS Player.FEMALE_MALE_MAP (Player.ts:150-188).
// See maleFemaleMap doc comment for sparseness + lossiness notes; the
// female→male direction has its own collapse cases ({45, 46}→0;
// {73, 74, 77}→36).
var femaleMaleMap = map[int]int{
	45: 0, 46: 0, 47: 1, 48: 2, 49: 3, 50: 4, 51: 5, 52: 6, 53: 7, 54: 8, 55: 9,
	56: 18, 57: 18, 58: 18, 59: 18, 60: 18,
	61: 26, 62: 27, 63: 28, 64: 29, 65: 29, 66: 32, 67: 33, 68: 34, 69: 35,
	70: 36, 71: 37, 72: 38, 73: 36, 74: 36, 75: 40, 76: 39, 77: 36,
	78: 41, 79: 42, 80: 43, 81: 44,
}

// lookupGenderIdkit returns m[k] when present, -1 otherwise. Mirrors TS
// expression `Map.get(k) ?? -1` at PlayerOps.ts:1109,1115.
func lookupGenderIdkit(m map[int]int, k int) int {
	if v, ok := m[k]; ok {
		return v
	}
	return -1
}

// SetGender rewrites the player's 7-slot body[] idkit array via the
// gender lookup map and writes the gender field. Mirrors TS
// PlayerOps.ts:1104-1118 SETGENDER handler.
//
// Does NOT flip MaskAppearance — TS-faithful deferred-rebuild pattern:
// real content (LostCityRS/Content/scripts/areas/area_falador/scripts/
// makeover_mage.rs2:58-64) follows SETGENDER + SETSKINCOLOUR with
// BUILDAPPEARANCE, which is the explicit rebuild trigger. Mirrors the
// established SetBodyPart precedent.
//
// When gender == 1 (target female): every body slot is rewritten via
// maleFemaleMap; unmapped keys produce -1.
//
// When gender == 0 (target male): slot 1 is hardcoded to 14 (intentional
// TS canon for canonical male hair model, see PlayerOps.ts:1111-1113);
// all other slots are rewritten via femaleMaleMap; unmapped keys
// produce -1.
//
// Deviation NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED: when a body
// slot holds an idkit value not present in the relevant lookup map, the
// slot is overwritten with -1 (idkit "null"). This is TS-literal
// behavior (Map.get() ?? -1 at PlayerOps.ts:1109,1115). Real content
// scripts only invoke SETGENDER from controlled UI flows where the
// player's body already came from the same-direction lookup, so the
// -1 case is content-unreachable today; pinned for future TS sync.
// Pinned by TestPlayerSetGender_UnmappedKeysWriteMinusOne.
func (p *Player) SetGender(gender int) {
	for i := range 7 {
		if gender == 1 {
			p.body[i] = lookupGenderIdkit(maleFemaleMap, p.body[i])
		} else {
			if i == 1 {
				p.body[i] = 14
				continue
			}
			p.body[i] = lookupGenderIdkit(femaleMaleMap, p.body[i])
		}
	}
	p.gender = gender
}
```

- [ ] **Step 2.4: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./modules/world/... -run TestPlayerSetGender 2>&1 | tail -10
```

Expected: PASS for all 6 tests (including 2 subtests in `TestPlayerSetGender_UnmappedKeysWriteMinusOne` and 2 in `TestPlayerSetGender_DoesNotFlipMaskAppearance`).

- [ ] **Step 2.5: Wider package gate (modules/world is the big test surface)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./modules/world/... 2>&1 | tail -5
```

Expected: all green. (modules/world world-tests take ~150s; budget accordingly.)

- [ ] **Step 2.6: Commit**

```bash
git status
```

Expected staged set: `modules/world/player_gender.go` and `modules/world/player_gender_test.go` only.

```bash
git add modules/world/player_gender.go modules/world/player_gender_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): port Player gender body-recoloring maps + SetGender

Adds (*Player).SetGender(gender int) mirroring TS PlayerOps.ts:1104-1118
SETGENDER handler body. The for-loop + map lookups + slot-1 hardcode
(PlayerOps.ts:1111-1113) + final gender field write live on Player
alongside the TS class-level static lookup tables (TS Player.MALE_FEMALE_MAP
and FEMALE_MALE_MAP at Engine-TS/.../entity/Player.ts:110-188), now
ported as package-level maleFemaleMap / femaleMaleMap vars.

Helper lookupGenderIdkit mirrors TS `Map.get(k) ?? -1` expression.

Does NOT flip MaskAppearance — TS-faithful deferred-rebuild pattern
(callers follow with BUILDAPPEARANCE per makeover_mage.rs2:58-64).

Opens NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED — when a body slot
holds an idkit value not present in the relevant lookup map, the slot
is overwritten with -1 (TS-literal `Map.get() ?? -1`). Content-unreachable
today; pinned for future TS sync.

T2 of SETGENDER body port + GenderValid slice (spec 46e3bd58).
EOF
)"
git show --stat HEAD
```

Expected: 2 files changed (new player_gender.go ~70 lines, new player_gender_test.go ~130 lines).

---

## Task 3: `ActivePlayer.SetGender` interface + `mockPlayer` impl

**Files:**
- Modify: `pkg/script/active.go` — add `SetGender(int)` method after `SetColorPart` (L743)
- Modify: `pkg/script/runner_test.go` — add `setGenderCalls []int` field (near L348-350) + `(m *mockPlayer) SetGender` impl (near L771-773)

Confirmed by predecessor close memory: `mockPlayer` is the only `ActivePlayer` impl outside `modules/world.Player`. Adding `SetGender` to the interface breaks compile only in `runner_test.go`.

- [ ] **Step 3.1: Verify the one-site fake-sweep claim**

Run:
```bash
grep -rn "ActivePlayer interface\|var _ ActivePlayer\|var _ script.ActivePlayer\|implements ActivePlayer" --include="*.go" /home/owner/Code/github.com/zsrv/goscape/ 2>&1 | head
```

Expected: only `pkg/script/active.go` (the interface declaration) and `modules/world/...` (production impl on `*Player`). If a third site appears (other than test files), STOP and reassess — `mockPlayer` is the documented sole test fake.

- [ ] **Step 3.2: Run the build to confirm it's currently green**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1 | tail -5
```

Expected: clean. (No new methods on `ActivePlayer` yet, so no missing-method errors.)

- [ ] **Step 3.3: Add the interface method**

Open `pkg/script/active.go`. Find `SetColorPart` at line 743 (the `SetColorPart(slot, color int)` declaration after the doc comment at L739-742). Insert a new method declaration directly after, before whatever follows. The next existing thing is a blank line and likely the next method group; insert this:

```go

// SetGender rewrites the player's 7-slot body[] idkit array via the
// MALE_FEMALE / FEMALE_MALE lookup maps and writes the gender field.
// Called by SETGENDER after checkGender pre-validates v ∈ [0, 1].
// Does NOT flip MaskAppearance — TS pattern requires a subsequent
// BUILDAPPEARANCE for the change to reach the client (mirrors
// SETIDKIT/SETSKINCOLOUR deferred-rebuild precedent).
// Mirrors TS PlayerOps.ts:1104-1118.
SetGender(gender int)
```

- [ ] **Step 3.4: Run the build to confirm it now fails in runner_test.go**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1 | tail -5
```

Expected: clean (production builds; `*Player` already satisfies via Task 2's `SetGender` method).

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... 2>&1 | tail -10
```

Expected: BUILD FAIL with a message like `*mockPlayer does not implement ActivePlayer (missing method SetGender)`.

- [ ] **Step 3.5: Add the mockPlayer impl + capture field**

Open `pkg/script/runner_test.go`. Find the `mockPlayer` struct field declarations near line 348 (`genderValue`, `bodyParts`, `colorParts`). Insert `setGenderCalls` directly after `colorParts`:

```go
	genderValue int
	bodyParts   [7]int
	colorParts  [5]int
	setGenderCalls []int
```

Find the `Gender` / `SetBodyPart` / `SetColorPart` method block near line 770-773. Insert the new `SetGender` method directly after `SetColorPart`:

```go
// NAI-47: SETIDKIT appearance-mutation captures.
func (m *mockPlayer) Gender() int                  { return m.genderValue }
func (m *mockPlayer) SetBodyPart(slot, idkit int)  { m.bodyParts[slot] = idkit }
func (m *mockPlayer) SetColorPart(slot, color int) { m.colorParts[slot] = color }

// SetGender captures SETGENDER dispatches for handler tests. The setter's
// real body-rewriting logic lives on modules/world.Player.SetGender; the
// mock only records the gender argument so handler-level tests can pin
// the popInt + checkGender + dispatch flow.
func (m *mockPlayer) SetGender(gender int) { m.setGenderCalls = append(m.setGenderCalls, gender) }
```

- [ ] **Step 3.6: Run pkg/script gate to confirm green**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... 2>&1 | tail -5
```

Expected: all green. (No new behavior tests yet — that's T4.)

- [ ] **Step 3.7: Commit**

```bash
git status
git add pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): add ActivePlayer.SetGender + mockPlayer impl

Adds SetGender(gender int) to the ActivePlayer interface so script
handlers can dispatch the SETGENDER opcode through the existing seam.
The interface doc comment cross-refs the deferred-rebuild contract
(no MaskAppearance flip) per the SETIDKIT/SETSKINCOLOUR sibling
pattern.

mockPlayer impl records the gender argument on a setGenderCalls slice
for handler tests. Production *Player impl was added in T2.

T3 of SETGENDER body port + GenderValid slice (spec 46e3bd58).
EOF
)"
git show --stat HEAD
```

Expected: 2 files changed, ~15 insertions.

---

## Task 4: Real `handleSetGender` + retire stub + handler unit tests

**Files:**
- Modify: `pkg/script/handlers_player.go` — add real `handleSetGender` after `handleSetSkinColour` at line 1678
- Modify: `pkg/script/handlers_b0_stubs.go` — delete `handleSetGender` stub (L21-25)
- Modify: `pkg/script/handlers_b0_stubs_test.go` — delete `SET_GENDER` table row (L20); update doc comment (L8-11) to reference 5 stubs not 6
- Modify: `pkg/script/handlers_player_test.go` — add 4 `TestHandleSetGender_*` tests after `TestHandleSetSkinColour_RequiresActivePlayer` at line 5174

- [ ] **Step 4.1: Write the failing handler tests**

Open `pkg/script/handlers_player_test.go`. Find `TestHandleSetSkinColour_RequiresActivePlayer` ending at line 5174 (the closing `}`). Insert the following directly after:

```go

// TestHandleSetGender_RequiresActivePlayer pins the goscape-only
// defensive active-player guard. TS skips this check; the guard
// follows the defensive_gate_doc_comment_label convention.
func TestHandleSetGender_RequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)
	err := handleSetGender(s)
	if err == nil {
		t.Fatalf("handleSetGender: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "SETGENDER") {
		t.Errorf("error: got %q, want to contain \"SETGENDER\"", err.Error())
	}
}

// TestHandleSetGender_RejectsOutOfRange pins TS check(gender, GenderValid)
// — inclusive [0, 1]. Tests both off-by-one boundaries.
func TestHandleSetGender_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name   string
		gender int
	}{
		{"-1 below min", -1},
		{"2 above max", 2},
		{"large negative", -100},
		{"large positive", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Self:        mp,
				Pointers:    PtrActivePlayer,
			}
			s.PushInt(tc.gender)
			err := handleSetGender(s)
			if err == nil {
				t.Fatalf("handleSetGender(%d): expected error, got nil", tc.gender)
			}
			if !strings.Contains(err.Error(), "SETGENDER") {
				t.Errorf("error: got %q, want to contain \"SETGENDER\"", err.Error())
			}
			if len(mp.setGenderCalls) != 0 {
				t.Errorf("setGenderCalls: got %v, want empty (no dispatch on validator error)", mp.setGenderCalls)
			}
		})
	}
}

// TestHandleSetGender_DispatchesToSetter pins the happy-path dispatch.
// PopInt + checkGender(1) + s.Self.SetGender(1).
func TestHandleSetGender_DispatchesToSetter(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(1)
	if err := handleSetGender(s); err != nil {
		t.Fatalf("handleSetGender: %v", err)
	}
	if got, want := mp.setGenderCalls, []int{1}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("setGenderCalls: got %v, want %v", got, want)
	}
	if s.ISP != 0 {
		t.Errorf("ISP: got %d, want 0 (stack should be fully drained)", s.ISP)
	}
}

// TestHandleSetGender_AcceptsZeroEdge pins the lower boundary of the
// inclusive [0, 1] range. Mirrors the predecessor slice's boundary-pin
// pattern (TestHandleNpcQueueAcceptsZeroEdge).
func TestHandleSetGender_AcceptsZeroEdge(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(0)
	if err := handleSetGender(s); err != nil {
		t.Fatalf("handleSetGender(0): %v", err)
	}
	if got, want := mp.setGenderCalls, []int{0}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("setGenderCalls: got %v, want %v", got, want)
	}
}
```

- [ ] **Step 4.2: Run the new tests to verify the stub still fails them**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... -run 'TestHandleSetGender_' 2>&1 | tail -20
```

Expected: tests FAIL. The current stub returns `"SET_GENDER: unimplemented"` which doesn't contain `"SETGENDER"` (different format — the stub uses `SET_GENDER` with underscore, the real handler will use `SETGENDER` per the SETSKINCOLOUR sibling pattern). At minimum `TestHandleSetGender_RequiresActivePlayer` should fail (stub doesn't guard) and `TestHandleSetGender_DispatchesToSetter` should fail (stub returns error instead of dispatching).

- [ ] **Step 4.3: Delete the stub**

Open `pkg/script/handlers_b0_stubs.go`. Find the `handleSetGender` block at lines 21-25:

```go
// handleSetGender (SET_GENDER, opcode 2099) — TS-unimplemented stub.
// NAI-162-D-STUB-SETGENDER.
func handleSetGender(s *ScriptState) error {
	return fmt.Errorf("SET_GENDER: unimplemented")
}
```

Delete those 5 lines AND the blank line that separates them from the next stub. The file's resulting layout: `handlePushVarbit` (L11-13), then `handlePopVarbit` (L17-19), then `handleLcOp`, then `handleOcIop`, then `handleOcOp`. (Five stubs remain after this slice.)

- [ ] **Step 4.4: Write the real handler**

Open `pkg/script/handlers_player.go`. Find `handleSetSkinColour` ending at line 1678 (its closing `}`). Insert the following directly after:

```go

// handleSetGender (SETGENDER, opcode 2099) rewrites the player's body[]
// idkit array via gender lookup maps and writes the gender field.
// Mirrors TS PlayerOps.ts:1104-1118:
//
//	const gender = check(state.popInt(), GenderValid)
//	for (let i = 0; i < 7; i++) { ... }
//	state.activePlayer.gender = gender
//
// Validated via checkGender (TS GenderValid, inclusive [0, 1]). The
// body-rewrite loop + lookup maps + slot-1 special case live on
// (*Player).SetGender in modules/world/player_gender.go alongside the
// TS-mirror class-level constants (TS Player.MALE_FEMALE_MAP /
// FEMALE_MALE_MAP).
//
// Does NOT flip MaskAppearance — callers must invoke BUILDAPPEARANCE
// for the change to reach the client (TS-faithful deferred-rebuild;
// see modules/world/player_gender.go::SetGender doc comment for the
// content-evidence cite at makeover_mage.rs2:58-64).
//
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
func handleSetGender(s *ScriptState) error {
	if err := requireActivePlayer(s, "SETGENDER"); err != nil {
		return err
	}
	gender := s.PopInt()
	if err := checkGender(gender, "SETGENDER"); err != nil {
		return err
	}
	s.Self.SetGender(gender)
	return nil
}
```

- [ ] **Step 4.5: Update the b0_stubs_test row deletion + doc-comment**

Open `pkg/script/handlers_b0_stubs_test.go`. Update the leading doc comment (L8-11):

```go
// TestNAI162B0StubsReturnUnimplemented pins the 5 TS-unimplemented
// stubs (PUSH_VARBIT, POP_VARBIT, LC_OP, OC_IOP, OC_OP). Each returns
// an error containing "unimplemented" without mutating any pointer
// state. Mirrors NAI-161 P_OPHELD stub-with-pin shape.
```

(Removed `SET_GENDER` from the list and changed `6` → `5`.)

Then delete the row at line 20:
```go
		{"SET_GENDER", OpSetGender, "SET_GENDER: unimplemented"},
```

Resulting table: 5 rows in the order `PUSH_VARBIT`, `POP_VARBIT`, `LC_OP`, `OC_IOP`, `OC_OP`.

- [ ] **Step 4.6: Run tests to verify they all pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... -run 'TestHandleSetGender_|TestNAI162B0Stubs' 2>&1 | tail -20
```

Expected: all PASS. The 5 remaining `TestNAI162B0Stubs/...` subtests still pass (`PUSH_VARBIT`, `POP_VARBIT`, `LC_OP`, `OC_IOP`, `OC_OP`); the 4 new `TestHandleSetGender_*` pass.

- [ ] **Step 4.7: Wider pkg/script gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... 2>&1 | tail -5
```

Expected: all green.

- [ ] **Step 4.8: Commit**

```bash
git status
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers_b0_stubs.go pkg/script/handlers_b0_stubs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): wire real handleSetGender at SETGENDER (T4)

Replaces the TS-unimplemented stub at handlers_b0_stubs.go with the
real TS PlayerOps.ts:1104-1118 port. Handler is thin: requireActivePlayer
+ PopInt + checkGender + Self.SetGender dispatch. The body-rewrite loop
+ lookup maps + slot-1 hardcode (PlayerOps.ts:1111-1113) + final gender
write live on (*Player).SetGender in modules/world/ (added in T2).

Does NOT flip MaskAppearance — TS-faithful deferred-rebuild pattern
(callers follow with BUILDAPPEARANCE per makeover_mage.rs2:58-64).

Retires NAI-162-D-STUB-SETGENDER. The b0_stubs.go file + its table-driven
test now describe 5 stubs (down from 6): PUSH_VARBIT, POP_VARBIT, LC_OP,
OC_IOP, OC_OP.

T4 of SETGENDER body port + GenderValid slice (spec 46e3bd58).
EOF
)"
git show --stat HEAD
```

Expected: 4 files changed (handlers_player.go +~25 lines, handlers_player_test.go +~85 lines, handlers_b0_stubs.go -5 lines and possibly -1 blank, handlers_b0_stubs_test.go ~-2 lines).

---

## Task 5: Audit-grep carry-forward sweep + close

This task verifies the slice's audit-grep keywords show 0/expected-only hits, then commits an empty close marker. No production code changes.

- [ ] **Step 5.1: Run the audit-grep sweep**

Run each of these and confirm the expected outcome:

```bash
# Expect: 0 hits (stub error string gone from production)
grep -rn "SET_GENDER: unimplemented" pkg/ modules/

# Expect: 0 hits (deviation pin fully retired)
grep -rn "NAI-162-D-STUB-SETGENDER" pkg/ modules/

# Expect: 5 lines (PUSH_VARBIT, POP_VARBIT, LC_OP, OC_IOP, OC_OP), not 6
grep -c "TS-unimplemented" pkg/script/handlers_b0_stubs.go

# Expect: at least 1 hit on the new pin doc comment in modules/world/player_gender.go
grep -rn "NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED" pkg/ modules/
```

If any of these has unexpected output, STOP and investigate. Do not silently move on — audit-grep "zero hits expected" is a zero-tolerance gate per `[[queue-skincolour-validator-slice-close]]` non-obvious finding #3.

- [ ] **Step 5.2: Run the full gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./... 2>&1 | tail -10
```

Expected: all packages PASS. modules/world world-tests take ~150s; total ~180s. If anything fails, report and fix the offending task — do not commit close.

- [ ] **Step 5.3: Smoke-pack regression check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/pack/compiler/... -run TestPackAll_TwelveStageSmoke 2>&1 | tail -5
```

Expected: PASS in ~1.0s, output shape matches baseline (12 OK / 0 ERR / 0 SKIP).

- [ ] **Step 5.4: Close commit**

```bash
git status
```

Expected: no staged changes (everything from T1-T4 is already committed), `config.yaml` modified, standing untracked noise present. Do not stage any of that.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): SETGENDER body port + GenderValid

Closes the SETGENDER body port slice from spec 46e3bd58. Five-task
slice that:

- Adds checkGender(v int, op string) error — TS GenderValid mirror,
  inclusive [0, 1] (T1)
- Ports Player.MALE_FEMALE_MAP / FEMALE_MALE_MAP body-recoloring
  lookup tables (TS Player.ts:110-188) + (*Player).SetGender method
  containing the for-loop + map lookups + slot-1 hardcode (TS
  PlayerOps.ts:1111-1113) + final gender field write (T2)
- Adds ActivePlayer.SetGender(int) interface method + mockPlayer
  capture (T3)
- Replaces handleSetGender TS-unimplemented stub with real TS
  PlayerOps.ts:1104-1118 port (T4)
- Retires NAI-162-D-STUB-SETGENDER
- Opens NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED — TS-literal
  `Map.get() ?? -1` writes -1 garbage idkit on unmapped keys (content-
  unreachable today via makeover_mage UI flow; pinned for future TS sync)
- Asserts (via TestPlayerSetGender_DoesNotFlipMaskAppearance) that
  SETGENDER does NOT flip MaskAppearance — TS-faithful deferred-rebuild
  pattern (real content follows with BUILDAPPEARANCE per
  makeover_mage.rs2:58-64)

Spec: docs/superpowers/specs/2026-05-20-setgender-genderval-port-design.md
Plan: docs/superpowers/plans/2026-05-20-setgender-genderval-port.md
EOF
)"
git show --stat HEAD
```

Expected: empty commit, no files changed.

- [ ] **Step 5.5: Write the close memory**

Write a new memory file at `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/setgender_genderval_port_close.md` capturing the slice digest, audit findings, and pivot menu for the next session. Use the predecessor `queue_skincolour_validator_slice_close.md` as a structural template — include:
- Slice commit range
- TS source citations (PlayerOps.ts:1104-1118 + Player.ts:110-188)
- New pin opened + pin retired
- Audit findings (single content callsite at `makeover_mage.rs2:58`; round-trip lossiness; slot-1 hardcode)
- Non-obvious findings worth carrying forward
- Out-of-scope / next-slice candidates

Then prepend a one-line index entry to MEMORY.md above `[[queue-skincolour-validator-slice-close]]`.

- [ ] **Step 5.6: Final sanity**

Run:
```bash
git log --oneline -8
```

Expected: 8-commit slice on top of `46e3bd58`:
- `chore(close): SETGENDER body port + GenderValid` (T5 close)
- `feat(script): wire real handleSetGender at SETGENDER (T4)`
- `feat(script): add ActivePlayer.SetGender + mockPlayer impl` (T3)
- `feat(world): port Player gender body-recoloring maps + SetGender` (T2)
- `feat(script): add checkGender validator` (T1)
- `docs(spec): SETGENDER body port + GenderValid validator` (46e3bd58)
- `chore(close): Queue + SkinColour validator port` (7badce7d)
- `docs(script): SkinColour bracket-space consistency` (996026d1)

No `config.yaml` or untracked noise in the slice's commits.

---

## Plan self-review notes

**Spec coverage** (cross-check each section of `2026-05-20-setgender-genderval-port-design.md`):
- §1 Goal — covered T1 (validator) + T2 (maps + setter) + T4 (handler + retire pin)
- §3 TS reference — citations embedded in T2 setter doc comment + T4 handler doc comment
- §4.1 Content audit — cited in T2's setter doc comment + close commit message
- §4.2 Map round-trip audit — pinned by `TestPlayerSetGender_LossyCollapse` in T2
- §4.3 -1 garbage behavior — pinned by `TestPlayerSetGender_UnmappedKeysWriteMinusOne` (T2) + new deviation pin opened on `SetGender` doc comment
- §5.3 Faithful-port assertions 1-5 — each has a pinning test: #1 `DoesNotFlipMaskAppearance`, #2 `Slot1HardcodedTo14`, #3 `UnmappedKeysWriteMinusOne`, #4 (identity loop — implicit in implementation, no test needed since #3 covers the no-guard observable), #5 `LossyCollapse`
- §6 File layout — task-by-task mapping matches §6 table
- §7 Test plan — all 13 listed tests present in T1+T2+T4
- §8 Audit-grep keywords — Step 5.1 runs all four
- §9 Pre-acknowledged watchpoints — addressed: T3 Step 3.1 verifies one-site fake-sweep; T4 Step 4.5 handles the table doc-comment AND row deletion; modules/world masks field confirmed at player.go:198

**Placeholder scan**: no TBD/TODO/FIXME/XXX/??? in this plan; every code step shows the actual code; every command step shows the exact command + expected output shape.

**Type consistency**: `checkGender(v int, op string) error` (T1) matches the handler call site `checkGender(gender, "SETGENDER")` (T4); `SetGender(gender int)` interface method (T3) matches the production impl signature `func (p *Player) SetGender(gender int)` (T2) and the mock impl `func (m *mockPlayer) SetGender(gender int)` (T3); the maps are typed `map[int]int` consistently across T2 and the lookup helper.

**Out-of-order task hazards**: T2 must complete before T3 (production `*Player` must satisfy the new interface before the interface is added). T3 must complete before T4 (handler uses `s.Self.SetGender(...)` which dispatches through the interface). T1 can run independently — placed first because it's the smallest and lowest-risk task.
