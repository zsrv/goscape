# LoW arg-shape pin at FindClosestNpc* — design

**Status:** approved (brainstormed 2026-05-21)
**Predecessor:** [[hunt-huntvis-filter-close]] (carry-forward menu item #1)
**Scope size:** XS (~hours, single commit in-session)
**Slice cadence:** spec + 1 impl commit, no plan, no subagent dispatch — matches [[ai-queue-fencepost-tighten-close]] / [[addxp-session-log-half-port-close]] precedent for XS slices.

## Goal

Close the LoW arg-shape pin deferral that [[hunt-huntvis-filter-close]] left in the carry-forward menu. The huntvis filter slice landed `huntvisGate` at `modules/world/npc_script_lookup.go:156` and a single LoS arg-shape pin test (`TestFindClosestNpcByType_LineOfSightArgShape` at `npc_script_lookup_test.go:312`), but left three pre-existing test-side asymmetries:

1. No LoW arg-shape test for `FindClosestNpcByType`.
2. No LoS arg-shape test for `FindClosestNpcByCategory`.
3. No LoW arg-shape test for `FindClosestNpcByCategory`.

`recordingFakeLineValidator` (`npc_script_lookup_test.go:27-40`) records only LoS args; its `HasLineOfWalk` returns a hardcoded `false` and records nothing.

## Non-goals

- **No production change.** Pure test addition. `huntvisGate` already passes arg-shape via the existing LoS-ByType test and the live huntvis filter tests at lines 222/252/342/362.
- **No widening of the pin width.** Existing LoS test pins only the first 5 args `(level, srcX, srcZ, destX, destZ)`; new tests stay narrow for parity. The trailing `1, 1, 1, 0` (srcSize/destWidth/destLength/extraFlag) in `huntvisGate` (`npc_script_lookup.go:165, 171`) is not asserted. Can be widened in a separate slice if a regression motivates it.
- **No formal NAI-XXX-D-* pin churn.** Closes a memo-tracked reviewer deferral, not a formal deviation tag.

## Deviation from TS

None. Pure test addition; production paths unchanged. Tests pin the same arg ordering TS uses at `Engine-TS/src/engine/script/ScriptIterators.ts:348` (LoS) and `:351` (LoW): `isLineOf{Sight,Walk}(this.level, this.x, this.z, npc.x, npc.z)` — iterator-as-src ordering. goscape's `huntvisGate` passes `(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0)` where `(srcX, srcZ)` is the lookup position and `(dstX, dstZ)` is the candidate NPC position — same ordering.

## File touched

Only `modules/world/npc_script_lookup_test.go`. No production files modified.

## Recorder extension (npc_script_lookup_test.go:27-40)

Add 6 fields to `recordingFakeLineValidator`:

```go
type recordingFakeLineValidator struct {
	losLevel, losSrcX, losSrcZ, losDestX, losDestZ int
	losReturn                                      bool
	lowLevel, lowSrcX, lowSrcZ, lowDestX, lowDestZ int
	lowReturn                                      bool
}
```

Turn `HasLineOfWalk` into a recorder mirror of `HasLineOfSight`:

```go
func (r *recordingFakeLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	r.lowLevel, r.lowSrcX, r.lowSrcZ, r.lowDestX, r.lowDestZ = level, srcX, srcZ, destX, destZ
	return r.lowReturn
}
```

**Safety:** the existing huntvis LoW filter tests (`TestFindClosestNpcByType_HuntVisLineOfWalk_FiltersBlocked` at line 252, `TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked` at line 362) use `fakeLineValidator` (not the recorder), so changing the recorder's LoW behaviour cannot regress them.

## New tests (3 functions)

Each mirrors the existing `TestFindClosestNpcByType_LineOfSightArgShape` (`npc_script_lookup_test.go:312`) fixture pattern:

- `setupLookupServer(t)` — gives Server with `npcLookup` bound and NpcTypes 7 (category 5) / 8 (category 9).
- `s.lineValidatorOverride = rec` — wire recorder before lookup call.
- One NPC at `(51, 52, level=3)`, `typeId = 7`.
- Lookup at `(level=3, x=50, z=50, dist=30, ...)`.
- Assert recorded `(level, srcX, srcZ, destX, destZ) == (3, 50, 50, 51, 52)` — iterator-as-src.

### Test 1 — `TestFindClosestNpcByType_LineOfWalkArgShape`

```go
// TestFindClosestNpcByType_LineOfWalkArgShape pins the LoW arg tuple
// per TS NpcIterator DISTANCE-mode at ScriptIterators.ts:351:
// isLineOfWalk(level, lookupX, lookupZ, npc.x, npc.z) — iterator-as-src
// ordering. Guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent at the
// FindClosest call site. Mirror of TestFindClosestNpcByType_LineOfSightArgShape.
func TestFindClosestNpcByType_LineOfWalkArgShape(t *testing.T) {
	s := setupLookupServer(t)
	rec := &recordingFakeLineValidator{lowReturn: true}
	s.lineValidatorOverride = rec
	npc := setupNpc(t, s, 51, 52, 3)
	npc.typeId = 7

	_ = s.npcLookup.FindClosestNpcByType(3, 50, 50, 30, 7, int(objtype.HuntVisLineOfWalk))
	if rec.lowLevel != 3 || rec.lowSrcX != 50 || rec.lowSrcZ != 50 || rec.lowDestX != 51 || rec.lowDestZ != 52 {
		t.Errorf("LoW arg shape: got (level=%d, src=%d,%d, dst=%d,%d), want (3, 50, 50, 51, 52) — lookup-as-src",
			rec.lowLevel, rec.lowSrcX, rec.lowSrcZ, rec.lowDestX, rec.lowDestZ)
	}
}
```

### Test 2 — `TestFindClosestNpcByCategory_LineOfSightArgShape`

Mirror of `TestFindClosestNpcByType_LineOfSightArgShape` (`npc_script_lookup_test.go:312`) for the Category variant. Doc cites `ScriptIterators.ts:348`. Calls `FindClosestNpcByCategory(3, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))` (category 5 matches typeId 7 per fixture). Asserts `rec.losLevel/losSrcX/losSrcZ/losDestX/losDestZ`. Closes the LoS-ByCategory pre-existing asymmetry.

### Test 3 — `TestFindClosestNpcByCategory_LineOfWalkArgShape`

Mirror of Test 1 for the Category variant. Doc cites `ScriptIterators.ts:351`. Calls `FindClosestNpcByCategory(3, 50, 50, 30, 5, int(objtype.HuntVisLineOfWalk))`. Asserts `rec.lowLevel/lowSrcX/lowSrcZ/lowDestX/lowDestZ`.

### Test placement

Insert new tests adjacent to their LoS counterparts. Existing file structure (line numbers approximate):

- Line 312: `TestFindClosestNpcByType_LineOfSightArgShape` (existing)
- Line 326: `TestFindClosestNpcByCategory_HuntVisOff_Baseline`
- Line 340: `TestFindClosestNpcByCategory_HuntVisLineOfSight_FiltersBlocked`
- Line 360: `TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked`
- Line 374: `TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow` (last in file)

Insertions:

- **Test 1** (`TestFindClosestNpcByType_LineOfWalkArgShape`) — immediately after the closing brace of `TestFindClosestNpcByType_LineOfSightArgShape` (~line 324), BEFORE the ByCategory baseline tests block.
- **Test 2** (`TestFindClosestNpcByCategory_LineOfSightArgShape`) — at end of file, after `TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow`.
- **Test 3** (`TestFindClosestNpcByCategory_LineOfWalkArgShape`) — at end of file, immediately after Test 2.

Final file order: ByType-LoS-ArgShape (existing) → ByType-LoW-ArgShape (new Test 1) → ByCategory baseline/filter tests (existing) → ByCategory-LoS-ArgShape (new Test 2) → ByCategory-LoW-ArgShape (new Test 3). The ByType arg-shape pair stays grouped at top; the ByCategory arg-shape pair stays grouped at bottom. Alternative placements (all-new-at-EOF, or interleaved with the ByCategory baseline block) were rejected because this layout best mirrors the existing "ByType block above ByCategory block" file convention.

## Tests / gates

- `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` — 57+ pkgs / 0 FAIL (expected ~155s, modules/world ~153s long pole).
- `TestPackAll_TwelveStageSmoke` — PASS.
- `gofmt -l modules/world/npc_script_lookup_test.go` — clean (no output).
- Audit-greps post-commit:
  - `recordingFakeLineValidator` — present with `lowLevel/lowSrcX/lowSrcZ/lowDestX/lowDestZ/lowReturn` fields and recording `HasLineOfWalk` body.
  - `TestFindClosestNpcByType_LineOfWalkArgShape` — 1 hit (new test definition).
  - `TestFindClosestNpcByCategory_LineOfSightArgShape` — 1 hit (new test definition).
  - `TestFindClosestNpcByCategory_LineOfWalkArgShape` — 1 hit (new test definition).
  - `ScriptIterators.ts:351` citation — 2 hits in production test file (Test 1 + Test 3 doc-comments).
  - `ScriptIterators.ts:348` citation — 2 hits in production test file (existing LoS-ByType + new LoS-ByCategory doc-comments).

## Memory + closes

- **No formal `NAI-XXX-D-*` pin** opened or retired. Pure test addition closes a memo-tracked reviewer deferral.
- **Close memo:** `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/low_arg_shape_pin_close.md`.
- **MEMORY.md index:** prepend a 1-line entry pointing at the close memo.
- **Carry-forward menu:** remove "LoW arg-shape pin at FindClosest" item #1 from next pivot menu in successor close memos.

## Out of scope (deliberately deferred)

- Widening pin to 9-arg shape (asserts `srcSize, destWidth, destLength, extraFlag = 1, 1, 1, 0`). Possible follow-up slice if `huntvisGate`'s trailing args ever need to vary by lookup mode.
- Mirroring this pin pattern for `NpcIterator` / `NpcHuntAllCommandIterator` consumers in `npc_iterator.go`. Different call site, different scope.
- Anti-bundling: gofmt-alignment drift across `pkg/script/active.go:22-37`, `pkg/script/handlers_npc_test.go:252-280`, `pkg/script/handlers_player_test.go:49-82` is the next slice (Doc-comment sweep) in the bundled session — do NOT touch in this slice per the user-confirmed 4-sequential-mini-slices decomposition.

## Patterns worth carrying forward

- **Iterator-as-src arg ordering** is the load-bearing invariant TS exports across all 14 sites in `ScriptIterators.ts` (`grep "isLineOf"` finds entries at lines 88/92/113/116/137/140/160/163/216/220/284/287/348/351). Future port-of-iterator slices should pin the same `(level, this.x, this.z, target.x, target.z)` ordering.
- **Narrow-pin convention from existing LoS test** is followed for parity; widening is a separate decision that needs an explicit motivator (current `1, 1, 1, 0` is implementation detail of `huntvisGate`, not iterator contract).
