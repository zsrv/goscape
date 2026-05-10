# NAI-147 — `rebuildNormal` Rename + NAI-78 Fidelity Batch — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close 4 tracker entries — `NAI-142-D-R-D4` (rename `(*Player).updateMap` → `rebuildNormal`), `NAI-78-D-NULL-TYPE-GUARD-OMITTED`, `NAI-78-D-DEBUG-MSG-DEFERRED`, `NAI-78-D-HASINTERACTION-GUARD` — plus add defensive-only doc-comment labels on 8 fire-helper no-script branches.

**Architecture:** 5-task TDD bundle, bottom-up by risk. T1+T2 are zero-behavior (labels + cosmetic rename) and ship without TDD red/green pairs. T3+T4+T5 follow standard TDD red→green→commit per task. All changes within `modules/world/`. No new packages, no new exported APIs.

**Tech Stack:** Go 1.26+ per `go_version.md`. TS source canonical path: `/home/owner/Code/github.com/LostCityRS/Engine-TS` per `ts_source_canonical_path.md`.

**Spec:** `docs/superpowers/specs/2026-05-10-nai-147-rebuildnormal-rename-and-nai78-fidelity-design.md` (commit `0df61cb`).

**Pre-flight HEAD:** `32f4463` (NAI-146 close).

---

## File Structure

| File | Tasks | Created/Modified |
|------|-------|------------------|
| `modules/world/interaction_trigger.go` | T1, T3 | Modified — 6 doc-comment labels (T1); `triggerTypeAndCategory` signature + `getOpTrigger`/`getApTrigger` early-return (T3) |
| `modules/world/player_interaction_trigger.go` | T1 | Modified — 2 doc-comment labels |
| `modules/world/player.go` | T2 | Modified — rename `(*Player).updateMap` → `rebuildNormal` + retarget self-doc-comment refs |
| `modules/world/tick.go` | T2 | Modified — 1 caller retarget |
| `modules/world/login_map_test.go` | T2 | Modified — 3 call-site retargets + doc-comment retargets |
| `modules/world/tick_order_test.go` | T2 | Modified — doc-comment retargets only |
| `modules/world/player_zone_test.go` | T2 | Modified — doc-comment retargets only |
| `modules/world/interaction.go` | T4, T5 | Modified — `defaultOp` signature + NodeDebug-gated chat (T4); top-of-`tryInteract` follow-op short-circuit (T5) |
| `modules/world/interaction_test.go` | T4 | Modified — existing `TestDefaultOp_EmitsNIHAndClearsWaypoints` at line 1537 retargets to new `defaultOp(p, nil, nil)` signature |
| `modules/world/interaction_trigger_null_guard_test.go` | T3 | **Created** — NULL-guard + dual-pin tests |
| `modules/world/interaction_default_op_debug_test.go` | T4 | **Created** — NodeDebug-gated chat coverage |
| `modules/world/interaction_tryinteract_guard_test.go` | T5 | **Created** — HASINTERACTION-GUARD branch-routing pins |

---

## Task 1: Defensive-gate doc-comment labels (zero-behavior)

**Purpose:** Add doc-comment labels to 8 fire-helper `if sf == nil` branches that became defensive-only after NAI-78's pre-gate on resolved-trigger-non-nil. Per `defensive_gate_doc_comment_label.md`.

**Files:**
- Modify: `modules/world/interaction_trigger.go` (6 sites)
- Modify: `modules/world/player_interaction_trigger.go` (2 sites)

**Risk:** None (comment-only).

### Step 1: Re-verify the 8 sites at HEAD

- [ ] **Run:** `rg -n "if sf == nil" modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go`
- [ ] **Expected output (8 lines):**
  ```
  modules/world/interaction_trigger.go:79:	if sf == nil {
  modules/world/interaction_trigger.go:158:	if sf == nil {
  modules/world/interaction_trigger.go:345:	if sf == nil {
  modules/world/interaction_trigger.go:432:	if sf == nil {
  modules/world/interaction_trigger.go:674:	if sf == nil {
  modules/world/interaction_trigger.go:740:	if sf == nil {
  modules/world/player_interaction_trigger.go:64:	if sf == nil {
  modules/world/player_interaction_trigger.go:115:	if sf == nil {
  ```
- [ ] If line numbers drift, re-anchor each Edit on the surrounding fire-helper function name (`fireOpTriggerNpc`, `fireOpTriggerLoc`, etc.) instead of line number.

### Step 2: Add label at `interaction_trigger.go:79` (`fireOpTriggerNpc`)

- [ ] Read `interaction_trigger.go:75-90` for current context.
- [ ] **Edit** — find the unique line at 79 + surrounding context. Add the label as a doc-comment block IMMEDIATELY ABOVE `if sf == nil {`. Standard label text (use this verbatim for all 8 sites):

```go
		// Defensive-only post-NAI-78 (goscape defensive; TS skips this
		// re-check). tryInteract pre-gates on resolved-trigger-non-nil so
		// this branch is unreachable from the hot path. Preserved for
		// non-tryInteract callers and as a goscape belt-and-braces.
		if sf == nil {
```

The Edit's `old_string` should be the existing `\tsf := srv.scriptProvider.GetByTrigger(...)\n\tif sf == nil {` line(s) with enough surrounding context to be unique within the file. Match indentation (tabs) exactly.

### Step 3: Repeat for the remaining 7 sites

- [ ] Add the same label block at each of:
  - `interaction_trigger.go:158` (`fireOpTriggerLoc`)
  - `interaction_trigger.go:345` (`fireApTriggerNpc`)
  - `interaction_trigger.go:432` (`fireApTriggerLoc`)
  - `interaction_trigger.go:674` (`fireOpTriggerObj`)
  - `interaction_trigger.go:740` (`fireApTriggerObj`)
  - `player_interaction_trigger.go:64` (`fireOpTriggerPlayer`)
  - `player_interaction_trigger.go:115` (`fireApTriggerPlayer`)
- [ ] Each Edit MUST be a per-instance Edit (no global `replace_all`) per `plan_doc_replaceall_timeline.md`.

### Step 4: Verify build + vet green

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- [ ] **Expected:** exit 0, no output.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
- [ ] **Expected:** exit 0, no output.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- [ ] **Expected:** all green (no behavior change).

### Step 5: Commit

- [ ] **Run:**
  ```bash
  git add modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
  git commit --no-gpg-sign -m "$(cat <<'EOF'
  docs(world): NAI-147 T1 — defensive-only labels on 8 fire-helper no-script branches

  After NAI-78's tryInteract pre-gate on resolved-trigger-non-nil, the
  `if sf == nil` branches in fireOp{Op,Ap}Trigger{Npc,Loc,Obj,Player}
  are unreachable from the hot path. Label per
  defensive_gate_doc_comment_label.md so a future reader doesn't take
  them as live TS-divergent gates.

  Zero behavior change. NAI-78 close minor item closed.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 2: R-D4 rename — `(*Player).updateMap` → `rebuildNormal` (zero-behavior)

**Purpose:** Rename the goscape method whose body ports TS `BuildArea.rebuildNormal` so the symbol name reflects the TS source. Frees the `updateMap` name for future consolidation.

**Files:**
- Modify: `modules/world/player.go` (declaration + 5 self-doc-comment refs)
- Modify: `modules/world/tick.go` (1 caller)
- Modify: `modules/world/login_map_test.go` (3 call sites + doc comments)
- Modify: `modules/world/tick_order_test.go` (3 doc-comment refs)
- Modify: `modules/world/player_zone_test.go` (3 doc-comment refs)

**Critical distinction:** The phrase `NetworkPlayer.updateMap` (with `NetworkPlayer.` prefix) refers to the **TS** symbol that goscape's `updateBuildArea` ports — these refs MUST NOT be renamed. Only `Player.updateMap`, `goscape's updateMap`, `p.updateMap`, and bare `updateMap` (in goscape-local context) get renamed.

**Risk:** R7 doc-comment timeline divergence — per `plan_doc_replaceall_timeline.md`, do NOT use global `replace_all`. Each rename is a per-instance Edit anchored on enough context to disambiguate goscape-local from TS-symbol references.

### Step 1: Verify all references at HEAD

- [ ] **Run:** `rg -nE "\bupdateMap\b" modules/world/`
- [ ] **Expected: 19 lines** across 5 files (verified at HEAD `32f4463`):
  ```
  modules/world/login_map_test.go:12, 36, 51, 74, 88
  modules/world/tick.go:471
  modules/world/player.go:177, 341, 746, 771, 785, 904, 914, 1026, 1031
  modules/world/tick_order_test.go:48, 53, 82
  modules/world/player_zone_test.go:35, 38, 45
  ```
- [ ] Classify each line as RENAME (goscape-local) or KEEP (TS symbol). Use the table in Step 2 to drive per-instance Edits.

### Step 2: Classification table — drive Edits from this

| Site | Current text (excerpt) | Action |
|------|------------------------|--------|
| `player.go:177` | `// at top-of-tick (after Player.updateMap has refreshed originX/originZ` | RENAME → `Player.rebuildNormal` |
| `player.go:341` | `// BEFORE updateMap each tick, NAI-93: updateMap is in processInfo). Reusing originX as the sentinel would` | RENAME → `rebuildNormal` (both occurrences in this line) |
| `player.go:746` | `// Matches TS ordering (TS World.ts:1097 → NetworkPlayer.updateMap also` | KEEP (TS symbol) |
| `player.go:771` | `func (p *Player) updateMap() {` | RENAME → `func (p *Player) rebuildNormal() {` |
| `player.go:785` | `// already cached the STALE origin by the time updateMap ran, and the` | RENAME → `rebuildNormal` |
| `player.go:904` | `// NetworkPlayer.updateMap (NetworkPlayer.ts:243-287) end-to-end.` | KEEP (TS symbol) |
| `player.go:914` | `// Origin freshness is preserved by NAI-93 ordering: Player.updateMap` | RENAME → `Player.rebuildNormal` |
| `player.go:1026` | `// NAI-93: goscape's updateMap (=TS BuildArea.rebuildNormal slot) was` | RENAME → `goscape's rebuildNormal` (the parenthetical TS reference stays as-is) |
| `player.go:1031` | `// NAI-142: updateBuildArea is the TS NetworkPlayer.updateMap slot` | KEEP (TS symbol) |
| `tick.go:471` | `p.updateMap()` | RENAME → `p.rebuildNormal()` |
| `login_map_test.go:12` | `// TestLoginSendsRebuildNormal verifies that updateMap() sends a RebuildNormal` | RENAME → `rebuildNormal()` |
| `login_map_test.go:36` | `p.updateMap()` | RENAME → `p.rebuildNormal()` |
| `login_map_test.go:51` | `// TestUpdateMapAnchorsOriginToPlayer verifies that updateMap()'s rebuild` | RENAME → `rebuildNormal()'s` (also rename the test function — see Step 4) |
| `login_map_test.go:74` | `p.updateMap()` | RENAME → `p.rebuildNormal()` |
| `login_map_test.go:88` | `p.updateMap()` | RENAME → `p.rebuildNormal()` |
| `tick_order_test.go:48` | `// pre-fix, processInfo runs rsbuf.ComputePlayer BEFORE updateMap, so the` | RENAME → `rebuildNormal` |
| `tick_order_test.go:53` | `// The fix moves updateMap into processInfo, before ComputePlayer, per TS` | RENAME → `rebuildNormal` |
| `tick_order_test.go:82` | `// Drive one tick of processInfo. Post-fix: updateMap fires inside` | RENAME → `rebuildNormal` |
| `player_zone_test.go:35` | `// encoding (which runs in updatePlayers BEFORE updateMap each tick). An` | RENAME → `rebuildNormal` |
| `player_zone_test.go:38` | `// shouldRebuild then returned false on the first updateMap call, never` | RENAME → `rebuildNormal` |
| `player_zone_test.go:45` | `// login, before any updateMap call has run.` | RENAME → `rebuildNormal` |

Total: 16 RENAMEs + 3 KEEPs.

### Step 3: Apply Edits per row of the table

- [ ] For each RENAME row, run a per-instance Edit. Choose `old_string` with enough surrounding context (typically the full line + 1-2 adjacent lines if line content alone isn't unique) to disambiguate.
- [ ] **Do NOT use** `replace_all` on `updateMap` — the global form would corrupt the 3 TS-symbol references.
- [ ] If a line contains BOTH goscape-local and TS-symbol refs (none in this list, but check), Edit only the goscape-local portion.

### Step 4: Rename the test function `TestUpdateMapAnchorsOriginToPlayer`

- [ ] In `login_map_test.go`, the test function name itself contains `UpdateMap`. Rename it for consistency:
  - Find `func TestUpdateMapAnchorsOriginToPlayer(t *testing.T) {` (around line 53 after Step 3 Edits — `rg -n "TestUpdateMap" modules/world/`).
  - Rename to `func TestRebuildNormalAnchorsOriginToPlayer(t *testing.T) {`.
- [ ] `TestLoginSendsRebuildNormal` already has the post-rename name shape — no change to its identifier.

### Step 5: Verify build + tests green

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- [ ] **Expected:** exit 0.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
- [ ] **Expected:** exit 0.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- [ ] **Expected:** all green. Pay particular attention to `TestLoginSendsRebuildNormal`, `TestRebuildNormalAnchorsOriginToPlayer`, and any tick-order / zone-test that ran before.

### Step 6: Re-grep to confirm no stragglers

- [ ] **Run:** `rg -n "\bupdateMap\b" modules/world/`
- [ ] **Expected: 3 lines remaining** — `player.go:746`, `player.go:904`, `player.go:1031` (all TS-symbol refs, KEEP per Step 2 table).
- [ ] If any other lines remain, re-classify and Edit per Step 3.

### Step 7: Commit

- [ ] **Run:**
  ```bash
  git add modules/world/player.go modules/world/tick.go modules/world/login_map_test.go modules/world/tick_order_test.go modules/world/player_zone_test.go
  git commit --no-gpg-sign -m "$(cat <<'EOF'
  refactor(player): NAI-147 T2 — rename updateMap → rebuildNormal

  Goscape's (*Player).updateMap ports TS BuildArea.rebuildNormal, not
  TS NetworkPlayer.updateMap. Mismatch hurt grep-discoverability and
  reserved the updateMap name for future consolidation. R-D1/R-D2/R-D3
  closed by NAI-143/NAI-145; trigger condition for R-D4 met.

  Renamed: declaration + 1 prod caller + 3 test call sites + 13
  goscape-local doc-comment refs. The 3 TS-symbol references
  (NetworkPlayer.updateMap) preserved verbatim as TS source pointers.

  Test function TestUpdateMapAnchorsOriginToPlayer renamed to
  TestRebuildNormalAnchorsOriginToPlayer.

  Zero behavior change. NAI-142-D-R-D4 closed.

  Closes memory: nai_followups.md NAI-142-D-R-D4

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 3: NAI-78-D-NULL-TYPE-GUARD-OMITTED — TS-fidelity port (TDD)

**Purpose:** Port TS `Player.ts:986-988` / `:1020-1022` `if (!type) return null` guard. When `triggerTypeAndCategory` cannot resolve the target's type config (Npc with `typ==nil`, Loc/Obj with out-of-range or nil config), `getOpTrigger`/`getApTrigger` return nil instead of falling through to a 3-tier `GetByTrigger` fallback.

**Files:**
- Modify: `modules/world/interaction_trigger.go:551-585` (signature + body), `:603` and `:618` (callers add `!ok` early return)
- Create: `modules/world/interaction_trigger_null_guard_test.go`

**TS source:** `Player.ts:966-998` (getOpTrigger), `:1000-1032` (getApTrigger).

**Risk:** R1 cascade — verified at HEAD: only 2 production callers (`getOpTrigger`, `getApTrigger`). Re-grep at task close to confirm no test files call `triggerTypeAndCategory` directly.

### Step 1: Re-grep callers at HEAD (after T2 commit)

- [ ] **Run:** `rg -n "triggerTypeAndCategory\(" modules/`
- [ ] **Expected (2 production lines + possibly 0+ test lines):**
  ```
  modules/world/interaction_trigger.go:603:	typeId, categoryId := triggerTypeAndCategory(p, srv)
  modules/world/interaction_trigger.go:618:	typeId, categoryId := triggerTypeAndCategory(p, srv)
  ```
- [ ] If any test files reference it directly, list them — they'll need signature updates in Step 5.

### Step 2: Write the failing tests

- [ ] **Create file:** `modules/world/interaction_trigger_null_guard_test.go` with the following content:

```go
package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// NAI-147 T3 — TS Player.ts:986-988 / :1020-1022 null-type guard.
// `triggerTypeAndCategory` returns ok=false when the target's type
// config is unresolvable; getOpTrigger/getApTrigger short-circuit to
// nil before reaching GetByTrigger.

func TestTriggerTypeAndCategory_NpcWithNilType_OkFalse(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	// Npc with typeId set but typ==nil (unloaded/missing type config).
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = nil
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Npc{typ:nil} → ok=false per TS L986-988)")
	}
}

func TestTriggerTypeAndCategory_LocOutOfRange_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 10)}
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	// Loc with Type=999999 — out of locTypes bounds.
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 999999, 10, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Loc{Type:OOB} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_LocNilConfig_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 10)}
	// Configs[5] left as nil.
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 5, 10, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Loc{Type:5,Configs[5]:nil} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_ObjOutOfRange_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 10)}
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	obj := &entitypkg.Obj{Type: 999999, Count: 1, X: 100, Z: 100, Level: 0}
	p.SetInteraction(InteractionEngine, obj, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Obj{Type:OOB} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_ObjNilConfig_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 10)}
	// Configs[5] left as nil.
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	obj := &entitypkg.Obj{Type: 5, Count: 1, X: 100, Z: 100, Level: 0}
	p.SetInteraction(InteractionEngine, obj, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Obj{Type:5,Configs[5]:nil} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_NpcOk(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId}, Category: 7}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	typeId, categoryId, ok := triggerTypeAndCategory(p, s)
	if !ok {
		t.Errorf("ok: got false, want true")
	}
	if typeId != npc.typeId {
		t.Errorf("typeId: got %d, want %d", typeId, npc.typeId)
	}
	if categoryId != 7 {
		t.Errorf("categoryId: got %d, want 7", categoryId)
	}
}

func TestTriggerTypeAndCategory_PlayerOk(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 1, -1)

	typeId, categoryId, ok := triggerTypeAndCategory(p, s)
	if !ok {
		t.Errorf("ok: got false, want true (Player target has no type lookup)")
	}
	if typeId != -1 {
		t.Errorf("typeId: got %d, want -1 (Player branch)", typeId)
	}
	if categoryId != -1 {
		t.Errorf("categoryId: got %d, want -1 (Player branch)", categoryId)
	}
}

// TestGetOpTrigger_NilTypeReturnsNil — pin: even if a script is
// registered at the global category=0 fallback, getOpTrigger returns
// nil when type is unresolvable. Proves the !ok early-return fires.
func TestGetOpTrigger_NilTypeReturnsNil(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register a global category=0 script that WOULD match if the
	// !ok guard didn't fire (3-tier fallback would resolve to this).
	globalScript := &script.ScriptFile{
		Name:      "[opnpc1,_global]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerOpNpc1),
	}
	s.scriptProvider.Register(globalScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = nil // unresolvable
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	got := getOpTrigger(p, s)
	if got != nil {
		t.Errorf("getOpTrigger: got %v, want nil (Npc{typ:nil} short-circuits)", got)
	}
}

// TestGetApTrigger_NilTypeReturnsNil — parallel for AP-side.
func TestGetApTrigger_NilTypeReturnsNil(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalScript := &script.ScriptFile{
		Name:      "[apnpc1,_global]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApNpc1),
	}
	s.scriptProvider.Register(globalScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = nil
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	got := getApTrigger(p, s)
	if got != nil {
		t.Errorf("getApTrigger: got %v, want nil", got)
	}
}

// TestGetOpTrigger_TypeKnownResolvesAtCategoryFallback — dual-pin
// (per ts_asymmetry_dual_pin.md): when type IS known and a script is
// registered at category-fallback, getOpTrigger resolves it. Without
// this pin, the regression direction is one-sided.
func TestGetOpTrigger_TypeKnownResolvesAtCategoryFallback(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	// Resolvable type with category=0.
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId}, Category: 0}

	// Register at category=0 — 3-tier fallback resolves type→category→global.
	categoryScript := &script.ScriptFile{
		Name:      "[opnpc1,_category0]",
		LookupKey: script.LookupKeyForCategory(script.TriggerOpNpc1, 0),
	}
	s.scriptProvider.Register(categoryScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	got := getOpTrigger(p, s)
	if got == nil {
		t.Errorf("getOpTrigger: got nil, want categoryScript (type known → 3-tier fallback fires)")
	}
}
```

### Step 3: Run tests — expect compile failure

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run NullGuard`
- [ ] **Expected:** compile error — `triggerTypeAndCategory` returns 2 values but test asserts 3 (`_, _, ok :=`).
- [ ] This is the RED state. Proceed to Step 4.

### Step 4: Modify `triggerTypeAndCategory` signature + body

- [ ] Read `modules/world/interaction_trigger.go:530-590` for current state.
- [ ] **Edit** — replace the whole `triggerTypeAndCategory` function with:

```go
// triggerTypeAndCategory derives (typeId, categoryId, ok) from the target's
// type registry, applying the targetSubject.com override per TS
// Player.getOpTrigger:993-995 / Player.getApTrigger:1027-1029.
//
// `ok` reports whether the target's type config is resolvable. Mirrors
// TS Player.ts:986-988 / :1020-1022 `if (!type) return null` guard:
// caller (getOpTrigger / getApTrigger) returns nil when ok==false,
// short-circuiting the GetByTrigger 3-tier fallback. NAI-147 T3 closes
// NAI-78-D-NULL-TYPE-GUARD-OMITTED.
//
// Player target: typeId stays -1 (TS Player.ts:971-972 default — Player
// branch doesn't set type) and categoryId stays -1 (provider falls
// through LookupKeyForType / LookupKeyForCategory to LookupKeyForGlobal).
// ok=true for Player targets — TS has no type lookup on the Player
// branch.
//
// Internal — used by getOpTrigger and getApTrigger.
func triggerTypeAndCategory(p *Player, srv *Server) (typeId, categoryId int, ok bool) {
	typeId = -1
	categoryId = -1
	ok = true

	switch tgt := p.target.(type) {
	case *Npc:
		if tgt.typ == nil {
			ok = false
			break
		}
		typeId = tgt.typeId
		categoryId = tgt.typ.Category
	case *entitypkg.Loc:
		locId := tgt.Type()
		if srv.locTypes == nil || locId < 0 || locId >= len(srv.locTypes.Configs) || srv.locTypes.Configs[locId] == nil {
			ok = false
			break
		}
		typeId = locId
		categoryId = srv.locTypes.Configs[locId].Category
	case *entitypkg.Obj:
		if srv.objTypes == nil || tgt.Type < 0 || tgt.Type >= len(srv.objTypes.Configs) || srv.objTypes.Configs[tgt.Type] == nil {
			ok = false
			break
		}
		typeId = tgt.Type
		categoryId = srv.objTypes.Configs[tgt.Type].Category
	case *Player:
		// typeId, categoryId stay -1; ok=true (TS L975-984 — Player
		// branch has no type lookup).
	}

	if !ok {
		return -1, -1, false
	}

	typeId = resolveTriggerTypeId(p, typeId)
	return typeId, categoryId, true
}
```

### Step 5: Update both callers (`getOpTrigger`, `getApTrigger`)

- [ ] In `getOpTrigger` at `interaction_trigger.go:603`, replace:
  ```go
  	typeId, categoryId := triggerTypeAndCategory(p, srv)
  	return srv.scriptProvider.GetByTrigger(apTrigger+7, typeId, categoryId)
  ```
  with:
  ```go
  	typeId, categoryId, ok := triggerTypeAndCategory(p, srv)
  	if !ok {
  		// NAI-147 T3 — TS Player.ts:986-988 short-circuit on
  		// unresolvable type.
  		return nil
  	}
  	return srv.scriptProvider.GetByTrigger(apTrigger+7, typeId, categoryId)
  ```

- [ ] Repeat at `getApTrigger:618` with the same shape (calling `srv.scriptProvider.GetByTrigger(apTrigger, typeId, categoryId)` per TS L1031 — note: AP-side does NOT add the `+7` offset).

### Step 6: Update the deviation doc-comment block

- [ ] Read `interaction_trigger.go:530-550` for the current `DEVIATION NAI-78-D-NULL-TYPE-GUARD-OMITTED` block.
- [ ] **Edit** — replace it with a closure note:

```go
// triggerTypeAndCategory derives (typeId, categoryId, ok) — see
// function doc-comment below. NAI-78-D-NULL-TYPE-GUARD-OMITTED was
// closed by NAI-147 T3: TS Player.ts:986-988 / :1020-1022 guard now
// ported via the `ok` return.
```

(This replaces the existing `DEVIATION NAI-78-D-NULL-TYPE-GUARD-OMITTED:` paragraph block immediately above the function.)

### Step 7: Run tests — expect green

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TriggerTypeAndCategory`
- [ ] **Expected:** 7 tests PASS.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "GetOpTrigger|GetApTrigger"`
- [ ] **Expected:** 3 tests PASS (NilType ones + dual-pin).
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- [ ] **Expected:** all green. Existing fire-helper tests should be unaffected (they don't call `triggerTypeAndCategory`).

### Step 8: Verify no straggler callers

- [ ] **Run:** `rg -n "triggerTypeAndCategory\(" modules/`
- [ ] **Expected:** only `interaction_trigger.go:551` (declaration), `:603` (getOpTrigger updated), `:618` (getApTrigger updated), plus the new test file.
- [ ] If any other call site appears, update it to handle the 3-value return.

### Step 9: Commit

- [ ] **Run:**
  ```bash
  git add modules/world/interaction_trigger.go modules/world/interaction_trigger_null_guard_test.go
  git commit --no-gpg-sign -m "$(cat <<'EOF'
  feat(world): NAI-147 T3 — port NULL-TYPE-GUARD-OMITTED

  TS Player.ts:986-988 / :1020-1022 — `if (!type) return null` guard
  now ported. triggerTypeAndCategory returns (typeId, categoryId, ok);
  getOpTrigger and getApTrigger short-circuit to nil when ok=false,
  matching TS short-circuit before GetByTrigger 3-tier fallback.

  Behavior delta: Npc{typ:nil}, Loc with OOB/nil config, Obj with
  OOB/nil config now return nil from getOpTrigger/getApTrigger
  immediately instead of falling through to category=0 fallback. Test
  pin: 7 NULL-guard tests + 2 short-circuit pins + 1 dual-pin
  (ts_asymmetry_dual_pin: known-type still resolves at category fallback).

  NAI-78-D-NULL-TYPE-GUARD-OMITTED closed.

  Closes memory: nai_followups.md NAI-78-D-NULL-TYPE-GUARD-OMITTED

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 4: NAI-78-D-DEBUG-MSG-DEFERRED — TS-fidelity port (TDD)

**Purpose:** Port TS `Player.ts:1076-1093` — when both opTrigger and apTrigger resolved nil and `Cfg.NodeDebug` is enabled, emit `[debug] No trigger for [<targetOp+7>,<debugname>]` chat before the existing `Nothing interesting happens.` message. Numeric trigger-name fallback per `NAI-147-D-TRIGGER-NAME-NUMERIC` (declared in spec §6).

**Files:**
- Modify: `modules/world/interaction.go:443` (caller passes opTrigger, apTrigger), `:463-466` (defaultOp body)
- Modify: `modules/world/interaction_test.go:1545` (existing `TestDefaultOp_EmitsNIHAndClearsWaypoints` retargets new signature)
- Create: `modules/world/interaction_default_op_debug_test.go`

**TS source:** `Player.ts:1072-1097`.

**Risk:** R2 caller cascade — verified at HEAD: 1 prod caller (`interaction.go:443`), 1 test caller (`interaction_test.go:1545`). Both update.

### Step 1: Re-grep callers at HEAD (after T3 commit)

- [ ] **Run:** `rg -n "defaultOp\(" modules/`
- [ ] **Expected:**
  ```
  modules/world/interaction.go:443:		defaultOp(p)
  modules/world/interaction.go:463:func defaultOp(p *Player) {
  modules/world/interaction_test.go:1545:	defaultOp(p)
  ```
- [ ] If additional callers appear, list them — they all need signature updates.

### Step 2: Verify T-trigger sentinels at HEAD

- [ ] **Run:** `rg -nE "targetOp\w*T\s*=" modules/world/interaction.go`
- [ ] **Expected (4 lines):** `targetOpLocT = 6`, `targetOpNpcT = 8`, `targetOpPlayerT = 10`, `targetOpObjT = 12` (verified at HEAD `32f4463`).
- [ ] If any are missing, escalate as a deviation — TS L1086 covers all 4.

### Step 3: Write the failing tests

- [ ] **Create file:** `modules/world/interaction_default_op_debug_test.go` with:

```go
package world

import (
	"bytes"
	"strconv"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// NAI-147 T4 — TS Player.ts:1076-1093 debug chat under NodeDebug.
// Numeric trigger-name fallback per NAI-147-D-TRIGGER-NAME-NUMERIC.

// makeDefaultOpFixture builds a server with all type configs seeded
// and a player wired with encryptor (required for MessageGame writes).
func makeDefaultOpFixture(t *testing.T) (*Server, *Player, chan []byte) {
	t.Helper()
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 100)}
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 100)}
	s.componentTypes = &objtype.ComponentTypeConfigs{Configs: make([]*objtype.ComponentType, 1000)}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)
	return s, p, received
}

func TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1) // targetOp=1 → ApNpc1; +7 → OpNpc1

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	// Trigger numeric form: ApNpc1 + 7 = OpNpc1 (numeric value
	// derived at runtime, not hardcoded to keep the test stable
	// under future trigger-id renumbering).
	wantDebug := []byte("No trigger for [" + strconv.Itoa(int(script.TriggerOpNpc1)) + ",test_npc]")
	if !bytes.Contains(got, wantDebug) {
		t.Errorf("missing debug message %q on wire; got %x", wantDebug, got)
	}
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("missing NIH message on wire; got %x", got)
	}
}

func TestDefaultOp_NoTriggerSuppressed_NodeDebugFalse(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = false

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("No trigger for")) {
		t.Errorf("debug message leaked under NodeDebug=false; got %x", got)
	}
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("missing NIH message; got %x", got)
	}
}

func TestDefaultOp_DebugSuppressed_OpTriggerPresent(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	stub := &script.ScriptFile{Name: "[opnpc1,test_npc]"}
	defaultOp(p, stub, nil)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("No trigger for")) {
		t.Errorf("debug message leaked when opTrigger non-nil; got %x", got)
	}
}

func TestDefaultOp_DebugSuppressed_ApTriggerPresent(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	stub := &script.ScriptFile{Name: "[apnpc1,test_npc]"}
	defaultOp(p, nil, stub)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("No trigger for")) {
		t.Errorf("debug message leaked when apTrigger non-nil; got %x", got)
	}
}

func TestDefaultOp_DebugnameNpc_FallbackToTypeId(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: ""}} // empty
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	want := []byte("," + strconv.Itoa(npc.typeId) + "]")
	if !bytes.Contains(got, want) {
		t.Errorf("debug message debugname fallback to typeId: missing %q; got %x", want, got)
	}
}

func TestDefaultOp_DebugnameLoc(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.locTypes.Configs[42] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "newbie_door1"},
	}
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",newbie_door1]")) {
		t.Errorf("debug message Loc debugname missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameObj(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.objTypes.Configs[42] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "bones"},
	}
	obj := &entitypkg.Obj{Type: 42, Count: 1, X: 100, Z: 100, Level: 0}
	p.SetInteraction(InteractionEngine, obj, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",bones]")) {
		t.Errorf("debug message Obj debugname missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameComOverride_TBranch(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.componentTypes.Configs[200] = &objtype.ComponentType{
		RootLayer: 200,
		ComName:   "spell_blast",
	}
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	// targetOp=targetOpNpcT (8) and com set: T-branch fires, com-name wins.
	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, 200)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",spell_blast]")) {
		t.Errorf("debug message com-name (T-branch) missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameSubjectTypeOverride(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.objTypes.Configs[42] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "bones"},
	}
	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 1, -1)
	// Manually set targetSubject.typ — TS targetSubject.type analogue.
	p.targetSubject.typ = 42

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",bones]")) {
		t.Errorf("debug message subjectType debugname missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameDefault_Underscore(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 1, -1)
	p.targetSubject.com = -1
	p.targetSubject.typ = -1

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",_]")) {
		t.Errorf("debug message default underscore missing; got %x", got)
	}
}

func TestDefaultOp_ClearWaypointsAlwaysFires(t *testing.T) {
	s, p, _ := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = false

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.waypointIndex = 5

	defaultOp(p, nil, nil)

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (TS Player.ts:1096)", p.waypointIndex)
	}
}

func TestDefaultOp_NothingInteresting_AlwaysFires(t *testing.T) {
	for _, debug := range []bool{true, false} {
		debug := debug
		t.Run(strconv.FormatBool(debug), func(t *testing.T) {
			s, p, received := makeDefaultOpFixture(t)
			s.cfg.NodeDebug = debug

			npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
			npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
			p.SetInteraction(InteractionEngine, npc, 1, -1)

			defaultOp(p, nil, nil)
			p.client.flushWrite()
			got := <-received

			if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
				t.Errorf("NodeDebug=%v: missing NIH message; got %x", debug, got)
			}
		})
	}
}
```

### Step 4: Run tests — expect compile failure

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run DefaultOp`
- [ ] **Expected:** compile error — `defaultOp` takes 1 arg but tests pass 3.
- [ ] This is the RED state. Proceed to Step 5.

### Step 5: Update `defaultOp` signature + body

- [ ] Read `interaction.go:451-466` for current state.
- [ ] **Edit** — replace the existing function block (including the `DEVIATION NAI-78-D-DEBUG-MSG-DEFERRED:` doc-comment paragraph) with:

```go
// defaultOp implements the NIH (Not-Implemented-Here) fallback fired by
// tryInteract branch 4 when the player reaches operable distance but no
// [op…] script is registered. Mirrors LostCityRS/Engine-TS
// Player.ts:1072-1097.
//
// NAI-147 T4 closes NAI-78-D-DEBUG-MSG-DEFERRED: under cfg.NodeDebug
// (TS !NODE_PRODUCTION analogue) and both triggers nil, emit the TS
// L1076-1093 debug chat. NAI-147-D-TRIGGER-NAME-NUMERIC: trigger name
// emitted in numeric form because pkg/script.ServerTriggerType has no
// String() table — adding a 50+ entry name table for one debug-only
// chat is over-investment.
func defaultOp(p *Player, opTrigger, apTrigger *script.ScriptFile) {
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if s.cfg.NodeDebug && opTrigger == nil && apTrigger == nil {
			debugname := defaultOpDebugname(p, s)
			p.MessageGame(fmt.Sprintf("No trigger for [%d,%s]", p.targetOp+7, debugname))
		}
	}
	p.MessageGame("Nothing interesting happens.")
	p.waypointIndex = -1 // TS Player.ts:1096 — clearWaypoints()
}

// defaultOpDebugname mirrors TS Player.ts:1077-1090 fan-out, returning
// a human-readable name for the player's current target. Used only by
// defaultOp's NodeDebug-gated chat. Internal — not exported.
func defaultOpDebugname(p *Player, s *Server) string {
	switch tgt := p.target.(type) {
	case *Npc:
		if tgt.typ != nil && tgt.typ.DebugName != "" {
			return tgt.typ.DebugName
		}
		return strconv.Itoa(tgt.typeId)
	case *entitypkg.Loc:
		typeId := tgt.Type()
		if s.locTypes != nil && typeId >= 0 && typeId < len(s.locTypes.Configs) {
			if lt := s.locTypes.Configs[typeId]; lt != nil && lt.DebugName != "" {
				return lt.DebugName
			}
		}
		return strconv.Itoa(typeId)
	case *entitypkg.Obj:
		if s.objTypes != nil && tgt.Type >= 0 && tgt.Type < len(s.objTypes.Configs) {
			if ot := s.objTypes.Configs[tgt.Type]; ot != nil && ot.DebugName != "" {
				return ot.DebugName
			}
		}
		return strconv.Itoa(tgt.Type)
	}

	// T-trigger com-branch (TS L1086).
	if p.targetSubject.com != -1 && isApTTrigger(p.targetOp) {
		com := p.targetSubject.com
		if s.componentTypes != nil && com >= 0 && com < len(s.componentTypes.Configs) {
			if ct := s.componentTypes.Configs[com]; ct != nil && ct.ComName != "" {
				return ct.ComName
			}
		}
		return strconv.Itoa(com)
	}

	// targetSubject.typ override branch (TS L1088 — TS field name `type`,
	// goscape field name `typ` per player.go:143).
	if p.targetSubject.typ != -1 {
		typ := p.targetSubject.typ
		if s.objTypes != nil && typ >= 0 && typ < len(s.objTypes.Configs) {
			if ot := s.objTypes.Configs[typ]; ot != nil && ot.DebugName != "" {
				return ot.DebugName
			}
		}
		return strconv.Itoa(typ)
	}

	return "_"
}

// isApTTrigger reports whether targetOp is one of the four T-trigger
// dispatch markers (APNPCT/APPLAYERT/APLOCT/APOBJT analogues). Mirrors
// TS Player.ts:1086 trigger-list. Goscape uses dispatch-marker
// sentinels (interaction.go:33-39) rather than per-T trigger numerics
// — the four sentinels are exhaustive.
func isApTTrigger(targetOp int) bool {
	return targetOp == targetOpLocT || targetOp == targetOpNpcT ||
		targetOp == targetOpPlayerT || targetOp == targetOpObjT
}
```

### Step 6: Add `fmt` and `strconv` imports if missing

- [ ] Read the import block of `interaction.go` (top of file).
- [ ] Confirm `"fmt"` and `"strconv"` are imported. If not, add them.
- [ ] Confirm `"github.com/zsrv/goscape/pkg/script"` is imported (for `*script.ScriptFile`).

### Step 7: Update the caller at line 443

- [ ] Find `defaultOp(p)` at `interaction.go:443` (inside `tryInteract` branch 4).
- [ ] **Edit** — replace with `defaultOp(p, opTrigger, apTrigger)`. Both vars are already in scope at that point (resolved at lines 389-390).

### Step 8: Update the existing `TestDefaultOp_EmitsNIHAndClearsWaypoints` test

- [ ] Read `modules/world/interaction_test.go:1530-1560`.
- [ ] **Edit** — replace `defaultOp(p)` (line 1545) with `defaultOp(p, nil, nil)`.
- [ ] **Edit** the doc-comment if it explicitly says "Goscape skips the NODE_PRODUCTION-gated dev `No trigger for [...]` debug line." (line 1536) — this is now NO LONGER TRUE post-T4. Replace with:
  ```
  // Goscape ports the NODE_PRODUCTION-gated dev "No trigger for [...]"
  // debug line under cfg.NodeDebug (NAI-147 T4 closed
  // NAI-78-D-DEBUG-MSG-DEFERRED). This test runs with NodeDebug=false
  // (the makeOpLocTriggerFixture default) so the debug line is suppressed
  // and only "Nothing interesting happens." is asserted.
  ```
- [ ] If `makeOpLocTriggerFixture` sets `s.cfg.NodeDebug=true`, the test will need NodeDebug forced to false: add `p.client.server.cfg.NodeDebug = false` after the fixture call.

### Step 9: Run tests — expect green

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run DefaultOp`
- [ ] **Expected:** all 12 new tests + the updated `TestDefaultOp_EmitsNIHAndClearsWaypoints` PASS.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- [ ] **Expected:** all green.

### Step 10: Commit

- [ ] **Run:**
  ```bash
  git add modules/world/interaction.go modules/world/interaction_test.go modules/world/interaction_default_op_debug_test.go
  git commit --no-gpg-sign -m "$(cat <<'EOF'
  feat(world): NAI-147 T4 — port DEBUG-MSG-DEFERRED under NodeDebug

  TS Player.ts:1076-1093 — under !NODE_PRODUCTION && both triggers nil,
  emit `No trigger for [<targetOp+7>,<debugname>]`. Goscape ports under
  cfg.NodeDebug.

  defaultOp signature now defaultOp(p, opTrigger, apTrigger). Caller at
  interaction.go:443 updated. defaultOpDebugname mirrors TS L1077-1090
  fan-out (Npc/Loc/Obj DebugName, T-trigger ComName, subjectType
  override, default `_`). isApTTrigger covers the 4 T-trigger
  dispatch-marker sentinels (TS L1086 list).

  Trigger name emitted in numeric form per
  NAI-147-D-TRIGGER-NAME-NUMERIC (declared at spec): ServerTriggerType
  has no String() table; 50+ entry name table for one debug-only chat
  is over-investment.

  NAI-78-D-DEBUG-MSG-DEFERRED closed.

  Closes memory: nai_followups.md NAI-78-D-DEBUG-MSG-DEFERRED

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 5: NAI-78-D-HASINTERACTION-GUARD — TS-fidelity port (TDD)

**Purpose:** Port TS `Player.ts:1114` 3-part guard `if (!this.target || !this.hasInteraction() || !this.canAccess())` at top of `tryInteract`. Follow-op targets (APPLAYER3/OPPLAYER3 — `targetOp==3` && target is `*Player`) short-circuit; `CanAccess()` blocks delayed/modal/protected-script states.

**Files:**
- Modify: `modules/world/interaction.go:374-378` (tryInteract early-return)
- Create: `modules/world/interaction_tryinteract_guard_test.go`

**TS source:** `Player.ts:1113-1116`.

**Risk:** R3 branch-routing semantics. R4 CanAccess stricter than `processInteraction:196` `delayed && currentTick<delayedUntil` — modal-state and protected-script gates may now short-circuit cases that previously reached branches 1-4. If any existing test surfaces a delta, escalate as `NAI-147-D-CANACCESS-MODAL-GATE`.

### Step 1: Re-verify CanAccess semantics + isFollowOp at HEAD

- [ ] **Run:** `rg -n "func \(p \*Player\) CanAccess|func isFollowOp|func \(p \*Player\) HasInteraction" modules/world/`
- [ ] **Expected (3 lines):**
  ```
  modules/world/player_script.go:324:func (p *Player) CanAccess() bool {
  modules/world/player_script.go:1066:func (p *Player) HasInteraction() bool {
  modules/world/interaction.go:159:func isFollowOp(p *Player) bool {
  ```
- [ ] If line numbers drift, re-anchor on the function names.

### Step 2: Audit existing tryInteract callers for CanAccess implications

- [ ] **Run:** `rg -n "tryInteract\(" modules/world/`
- [ ] List the callers. If any test seeds `p.modalState != 0` or `p.activeScript` with `PtrProtectedActivePlayer` AND expects tryInteract to PROCEED (rather than short-circuit), that's a regression risk under the new gate. Note such tests in this audit comment.
- [ ] Run baseline: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TryInteract -v` and capture which tests pass at HEAD pre-T5.

### Step 3: Write the failing tests

- [ ] **Create file:** `modules/world/interaction_tryinteract_guard_test.go` with:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// NAI-147 T5 — TS Player.ts:1114 3-part guard
// `!target || !hasInteraction() || !canAccess()`. Pins the new
// short-circuit branch + regression-fences existing happy paths.

func TestTryInteract_FollowOp_ShortCircuits(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()

	// Follow-op: targetOp=3, target=*Player. isFollowOp returns true,
	// HasInteraction returns false.
	p.SetInteraction(InteractionEngine, other, 3, -1)
	priorApRange := p.apRange

	got := p.tryInteract(false)

	if got {
		t.Errorf("tryInteract: got true, want false (follow-op short-circuit)")
	}
	if p.interactionFired {
		t.Errorf("interactionFired: got true, want false (no dispatch on short-circuit)")
	}
	if p.apRange != priorApRange {
		t.Errorf("apRange: got %d, want %d unchanged (no branch-3 mutation under guard)", p.apRange, priorApRange)
	}
	if p.lastInteractBranchPre != 0 && p.lastInteractBranchPost != 0 {
		// Acceptable: existing branch-0 recorder fires for the early-return path.
		// This assertion is informational — uncomment if branch=0 is expected:
		// if p.lastInteractBranchPost != 0 {
		// 	t.Errorf("expected branch=0 (early-return), got %d", p.lastInteractBranchPost)
		// }
	}
}

func TestTryInteract_NotFollowOp_NotShortCircuited(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register an opnpc1 script so branch 1 is reachable.
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	noopScript := &script.ScriptFile{
		Name:      "[opnpc1,_]",
		LookupKey: script.LookupKeyForType(script.TriggerOpNpc1, 7),
	}
	s.scriptProvider.Register(noopScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = npcType
	npc.typeId = 7
	p.SetInteraction(InteractionEngine, npc, 1, -1) // non-follow-op (targetOp=1, NPC)

	// Non-delayed, modal-clear, no protected script → CanAccess=true.
	if !p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be true")
	}

	got := p.tryInteract(false)
	if !got {
		t.Errorf("tryInteract: got false, want true (non-follow-op proceeds to branch 1)")
	}
	if !p.interacted {
		t.Errorf("interacted: got false, want true (branch 1 fired)")
	}
}

func TestTryInteract_Delayed_ShortCircuits(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 0
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.delayed = true
	p.delayedUntil = 1

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract: got true, want false (delayed → !CanAccess → short-circuit)")
	}
	if p.interactionFired {
		t.Errorf("interactionFired: got true, want false (no dispatch on guard short-circuit)")
	}
}

func TestTryInteract_NoTarget_ShortCircuits(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	// p.target left nil.

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract: got true, want false (no target)")
	}
}

func TestTryInteract_FollowOpDelayed_BothGatesGuard(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 0
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 3, -1) // follow-op
	p.delayed = true
	p.delayedUntil = 1

	// Both !HasInteraction() AND !CanAccess() — short-circuit must
	// hit cleanly (no panic).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("tryInteract panicked under combined guard: %v", r)
		}
	}()

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract: got true, want false")
	}
}

func TestTryInteract_HasInteractionTrue_ProceedsToBranch1(t *testing.T) {
	// Regression fence — non-follow-op + CanAccess=true must still
	// reach branches 1-4. Mirrors TestTryInteract_NotFollowOp_NotShortCircuited
	// but explicitly asserts HasInteraction()==true.
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	noopScript := &script.ScriptFile{
		Name:      "[opnpc1,_]",
		LookupKey: script.LookupKeyForType(script.TriggerOpNpc1, 7),
	}
	s.scriptProvider.Register(noopScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	npc.typeId = 7
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if !p.HasInteraction() {
		t.Fatal("test setup invalid: HasInteraction should be true for non-follow-op")
	}
	if !p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be true")
	}

	got := p.tryInteract(false)
	if !got {
		t.Errorf("tryInteract: got false, want true (regression fence — guard must not break happy path)")
	}
}
```

### Step 4: Run tests — expect failures

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TryInteract_FollowOp_ShortCircuits`
- [ ] **Expected:** FAIL — current `tryInteract` doesn't short-circuit on `!HasInteraction()`, so a follow-op target may reach branch 1-4 dispatch and `interactionFired` may be set.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TryInteract_Delayed_ShortCircuits`
- [ ] **Expected:** FAIL similarly (existing path may not short-circuit through tryInteract — delayed is gated higher up at `processInteraction:196`).

### Step 5: Update `tryInteract` early-return

- [ ] Read `interaction.go:374-389` for current state.
- [ ] **Edit** — replace the existing 5-line early-return block:

```go
func (p *Player) tryInteract(allowOpScenery bool) bool {
	if p.target == nil {
		recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (no-target early-return)
		return false
	}
	// DEVIATION NAI-78-D-HASINTERACTION-GUARD: TS Player.ts:1114 also
	// gates on `!this.hasInteraction()` (false for follow-op:
	// APPLAYER3 / OPPLAYER3). Pre-existing gap — was absent in the
	// 2-branch shape too. NAI-78 shifts the path the case follows (now
	// branch 3 → followOp post-step gate at processInteraction:221
	// rather than direct OP-block dispatch) but the underlying gap
	// is unchanged. Defer port alongside the rest of the follow-op
	// semantics.
	srv := p.client.server
```

with:

```go
func (p *Player) tryInteract(allowOpScenery bool) bool {
	// NAI-147 T5 closes NAI-78-D-HASINTERACTION-GUARD — TS
	// Player.ts:1114 3-part guard. !HasInteraction() filters follow-op
	// (targetOp=3 with *Player target). !CanAccess() filters delayed,
	// modal, and protected-script states. CanAccess() is STRICTER than
	// processInteraction:196's delayed-only check; modal/protected-script
	// short-circuits previously reachable through tryInteract are now
	// blocked here. Mirrors TS canAccess semantics via NAI-111 narrowed
	// convergence.
	if p.target == nil || !p.HasInteraction() || !p.CanAccess() {
		recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (combined early-return)
		return false
	}
	srv := p.client.server
```

### Step 6: Run new tests — expect green

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TryInteract_FollowOp_ShortCircuits -v`
- [ ] **Expected:** PASS.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TryInteract_(NotFollowOp|Delayed|NoTarget|FollowOpDelayed|HasInteractionTrue)" -v`
- [ ] **Expected:** all PASS.

### Step 7: Run full repo tests — surface any regression

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- [ ] **Expected:** all green.
- [ ] **If any existing test fails:**
  - Capture the failing test name + diff between pre-T5 baseline (Step 2) and now.
  - If the failure is due to a test that seeds modal-state or protected-script and expected tryInteract to proceed: open `NAI-147-D-CANACCESS-MODAL-GATE`. The TS-faithful gate is the new behavior; the test was relying on the old looser path. Update the test to either (a) clear modal-state before tryInteract or (b) assert the new short-circuit is the correct result.
  - Document the deviation in the commit body.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
- [ ] **Expected:** all green.

### Step 8: Commit

- [ ] **Run:**
  ```bash
  git add modules/world/interaction.go modules/world/interaction_tryinteract_guard_test.go
  git commit --no-gpg-sign -m "$(cat <<'EOF'
  feat(world): NAI-147 T5 — port HASINTERACTION-GUARD (TS Player.ts:1114)

  Top-of-tryInteract 3-part guard now ports TS:
    if (!target || !hasInteraction() || !canAccess()) return false;

  Goscape's HasInteraction (player_script.go:1066) excludes follow-op
  (targetOp==3, *Player target) per NAI-120 / TS Player.ts:955-964.
  CanAccess (player_script.go:324-335) covers delayed + modal-state +
  protectedScriptActive — TS-faithful via NAI-111 narrowed convergence.

  Behavior delta: follow-op targets short-circuit at top instead of
  reaching branch 3 (which previously set apRange=-1). CanAccess
  modal/protected-script gates may surface short-circuits the inline
  delayed-check at processInteraction:196 did not catch — defense in
  depth, TS-faithful.

  Test pin: 6 tests covering all 3 short-circuit conditions + happy
  path + combined-guard regression fence.

  NAI-78-D-HASINTERACTION-GUARD closed.

  Closes memory: nai_followups.md NAI-78-D-HASINTERACTION-GUARD

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Final verification + close

After all 5 task commits land:

- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
- [ ] **Expected:** all green.
- [ ] **Run:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
- [ ] **Expected:** exit 0.
- [ ] **Run:** `git log --oneline -7`
- [ ] **Expected:** 5 NAI-147 commits (T1-T5) + 2 prior (spec + NAI-146 close).
- [ ] **Run:** `rg -nE "NAI-78-D-NULL-TYPE-GUARD-OMITTED|NAI-78-D-DEBUG-MSG-DEFERRED|NAI-78-D-HASINTERACTION-GUARD" pkg/ modules/`
- [ ] **Expected:** zero hits in production code (deviation tags retired). Grep may surface refs in commit messages or memory files — those are fine.
- [ ] **Run:** `rg -n "\bupdateMap\b" modules/world/`
- [ ] **Expected:** 3 lines remaining — only TS-symbol references (`NetworkPlayer.updateMap`).

### Code review (Sonnet) + close commit

Single Sonnet reviewer at end of bundle per `superpowers_code_reviewer_model.md`. Review covers:
- T1 — labels match `defensive_gate_doc_comment_label.md` exact format.
- T2 — no goscape-local `updateMap` stragglers; TS-symbol refs preserved.
- T3 — `triggerTypeAndCategory` 3-tier signature TS-fidel; dual-pin in place.
- T4 — debug message format matches TS (modulo numeric trigger fallback per NAI-147-D-TRIGGER-NAME-NUMERIC); all 5 debugname branches covered.
- T5 — guard order matches TS; CanAccess regressions documented if any.

Close commit per `close_commit_memory_trailer.md`:
- Title: `chore(close): NAI-147 — rebuildNormal rename + NAI-78 fidelity batch`
- Body summarizes the 4 closed tracker entries + 1 opened sub-deviation (NAI-147-D-TRIGGER-NAME-NUMERIC) + smoke-deferral note.
- `Closes memory: nai_followups.md NAI-142-D-R-D4 NAI-78-D-NULL-TYPE-GUARD-OMITTED NAI-78-D-DEBUG-MSG-DEFERRED NAI-78-D-HASINTERACTION-GUARD` trailer.

Smoke deferred per spec §7 + `cascade_theory_smoke_binding.md` — joins the deferred batch alongside NAI-143/144/145/146.

---

## Execution

Per `superpowers_clear_between_spec_and_impl.md`: stop here. User `/clear`s before implementer dispatch. Per `execution_mode_default.md`: dispatch via `superpowers:subagent-driven-development`. Per `superpowers_code_reviewer_model.md`: implementer + reviewer both on Sonnet.
