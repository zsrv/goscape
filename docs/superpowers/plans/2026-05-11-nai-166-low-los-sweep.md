# NAI-166 — Iterator LOW+LOS arg-shape sweep + `handleLineOfWalk` wrapper-routing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Widen the four `pkg/script` iterator LV calls from `(1, 0, 0, 0)` to TS-canonical `(1, 1, 1, 0)`, and refactor `handleLineOfWalk` to route nil-LV handling through the `isLineOfWalk` wrapper (matching `handleLineOfSight`'s already-canonical wrapper routing).

**Architecture:** Two TDD tasks. T1 flips four arg-shape lines in `pkg/script/player_iterator.go` and `pkg/script/npc_iterator.go`, pinned by two new tests that reuse the existing `stubLineValidatorArgs` recorder (same package). T2 deletes the explicit nil-guard from `handleLineOfWalk`, swaps the direct LV call for an `isLineOfWalk(s, ...)` call, and inverts the existing `TestHandleLineOfWalkNilValidator` from expect-0 to expect-1. Stale doc-comments retire alongside their respective code.

**Tech Stack:** Go 1.26+ (`go_version.md`).

**Spec:** `docs/superpowers/specs/2026-05-11-nai-166-low-los-sweep-design.md` (commit `8073c77`).

**HEAD at plan-write:** `8073c77`.

**Scope correction during plan-write:** the brainstorm/spec initially cited 12 production sites across 4 files; plan-author re-grep at HEAD `8073c77` showed only 4 sites still carry the broken `(1, 0, 0, 0)` shape — all in `pkg/script/`. The `modules/world/` hunt sites already pass `(1, 1, 1, 0)` (likely fixed during NAI-9 / NAI-12). The spec was amended in commit `8073c77` to reflect verified HEAD.

---

## File Map

**Modify:**
- `pkg/script/player_iterator.go` — flip two arg-shape lines (71 LOS, 77 LOW); doc-comment scan around `passesFilter` (lines 52-80).
- `pkg/script/npc_iterator.go` — flip two arg-shape lines (127 LOS, 139 LOW); rewrite stale "(srcSize=1, destWidth=destLength=0, extraFlag=0) ... mirrors TS isLineOfSight wrapper" doc-comments at lines 117-122 and 130-134.
- `pkg/script/player_iterator_test.go` — add `TestPlayerIterator_LineValidatorArgShape` (covers both LOS and LOW branches).
- `pkg/script/npc_iterator_test.go` — add `TestNpcIterator_LineValidatorArgShape` (covers both helpers).
- `pkg/script/handlers_map.go` — `handleLineOfWalk` refactor (delete nil-guard block ~lines 432-435, swap direct call at ~line 436 to `isLineOfWalk(s, ...)`); rewrite preamble at ~lines 402-409 to drop "goscape defensive ... NAI-166 candidate" paragraph; trim wrapper preamble at ~lines 172-176 to drop the "iterator/hunt-site sweep ... tracked separately as a NAI-166 candidate" sentence.
- `pkg/script/handlers_map_test.go` — invert and rename `TestHandleLineOfWalkNilValidator` at ~line 945.

**No new files.** **No new stub types** — both new tests reuse the existing same-package `stubLineValidatorArgs` (declared at `pkg/script/handlers_map_test.go:972-992`).

---

## Per-task controller pre-flight (per `controller_preflight.md`)

Before each implementer dispatch, controller spends ~30s on:

```bash
# Verify the broken sites still match the plan's line refs at current HEAD
rg -n "1, 0, 0, 0" pkg/script/player_iterator.go pkg/script/npc_iterator.go
# Confirm modules/world hunt sites still passing — should print zero matches
rg -n "1, 0, 0, 0" modules/world/
# Verify stubLineValidatorArgs is still package-visible from iterator tests
grep -n "package script" pkg/script/handlers_map_test.go pkg/script/player_iterator_test.go pkg/script/npc_iterator_test.go
```

If any of these surface unexpected results, halt and update the plan before dispatching.

---

## Task 1: `pkg/script` iterator arg-shape sweep

**Files:**
- Test: `pkg/script/player_iterator_test.go`, `pkg/script/npc_iterator_test.go`
- Modify: `pkg/script/player_iterator.go:71, 77`; `pkg/script/npc_iterator.go:127, 139` (+ adjacent doc-comments)
- Modify: `pkg/script/handlers_map.go:172-176` (drop sweep-foreshadow sentence from `isLineOfWalk` wrapper preamble)

Rationale: TS canonical for both iterator families is the `isLineOfSight`/`isLineOfWalk` wrapper at `GameMap.ts:425-431` (`(srcSize=1, destWidth=1, destLength=1, extraFlag=0)`). goscape's iterator helpers currently pass `(srcSize=1, destWidth=0, destLength=0, extraFlag=0)` — the degenerate 0×0 dest changes how RayCast (`pkg/pathfinder/routefinder/linevalidator.go:21`) computes endpoint inclusion. Flipping to `(1, 1, 1, 0)` aligns with the wrapper and with NAI-163-D-LOS-ARG-SHAPE-FIX / NAI-165-D-LOW-ARG-SHAPE-FIX precedent.

### Step 1.1: Write the failing test for PlayerIterator

- [ ] **Edit `pkg/script/player_iterator_test.go`**. Append at end of file:

```go
// TestPlayerIterator_LineValidatorArgShape pins the TS-canonical
// (srcSize=1, destWidth=1, destLength=1, extraFlag=0) arg tuple at both
// LOS and LOW branches of PlayerIterator.passesFilter (player_iterator.go
// lines 71, 77). Mirrors NAI-165-D-LOW-ARG-SHAPE-FIX semantics, applied
// to the iterator family. NAI-166-D-LOW-ARG-SHAPE-SWEEP.
//
// TS canonical: ScriptIterators.ts:216 (LOS), :220 (LOW) — both route
// through the GameMap.ts:425-431 wrappers (1, 1, 1, 1, 0). goscape's
// srcSize collapses TS srcWidth+srcHeight into one arg via RayCast
// (linevalidator.go:21), so TS-faithful tuple at goscape's LV iface is
// (srcSize=1, destWidth=1, destLength=1, extraFlag=0).
func TestPlayerIterator_LineValidatorArgShape(t *testing.T) {
	t.Parallel()
	// LOS branch
	stubLOS := &stubLineValidatorArgs{losReturn: true}
	itLOS := NewHuntAllPlayerIterator(nil, stubLOS, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
	p := &mockPlayer{x: 3201, z: 3202}
	_ = itLOS.passesFilter(p)
	if len(stubLOS.losCalls) != 1 {
		t.Fatalf("LOS branch: expected 1 LV call, got %d", len(stubLOS.losCalls))
	}
	got := stubLOS.losCalls[0]
	want := losCall{level: 0, srcX: 3201, srcZ: 3202, destX: 3200, destZ: 3200, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("LOS arg shape:\n got=%+v\nwant=%+v", got, want)
	}

	// LOW branch
	stubLOW := &stubLineValidatorArgs{lowReturn: true}
	itLOW := NewHuntAllPlayerIterator(nil, stubLOW, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
	_ = itLOW.passesFilter(p)
	if len(stubLOW.lowCalls) != 1 {
		t.Fatalf("LOW branch: expected 1 LV call, got %d", len(stubLOW.lowCalls))
	}
	got = stubLOW.lowCalls[0]
	if got != want {
		t.Fatalf("LOW arg shape:\n got=%+v\nwant=%+v", got, want)
	}
}
```

Notes:
- `stubLineValidatorArgs` and `losCall` are declared in `pkg/script/handlers_map_test.go:972-992` and are package-visible (same `package script`).
- The fixture intentionally uses `(3201, 3202)` (asymmetric x/z) so the test would also catch a regression of the PlayerHuntAll src/dest swap pinned by `TestPlayerIterator_PassesFilter_LineOfSight_PlayerAsSrc` — but the primary assertion is the size/width/length tuple.
- `len(stubLOS.losCalls) != 1` guards against the "test passes for wrong reason" failure mode (`test_passes_for_wrong_reason.md`): if `passesFilter` returns via the distance gate before reaching the LV call, the slice stays empty and the test fails loudly.

### Step 1.2: Write the failing test for NpcIterator

- [ ] **Edit `pkg/script/npc_iterator_test.go`**. Append at end of file:

```go
// TestNpcIterator_LineValidatorArgShape pins the TS-canonical
// (srcSize=1, destWidth=1, destLength=1, extraFlag=0) arg tuple at both
// LOS and LOW branches of NpcIterator (npc_iterator.go lines 127, 139).
// Mirrors NAI-165-D-LOW-ARG-SHAPE-FIX semantics, applied to the iterator
// family. NAI-166-D-LOW-ARG-SHAPE-SWEEP.
//
// TS canonical: ScriptIterators.ts:284 (LOS), :287 (LOW) — both route
// through the GameMap.ts:425-431 wrappers (1, 1, 1, 1, 0). NpcHuntAll
// passes iterator-as-src + npc-as-dest (REVERSE of PlayerHuntAll).
func TestNpcIterator_LineValidatorArgShape(t *testing.T) {
	t.Parallel()
	// LOS branch
	stubLOS := &stubLineValidatorArgs{losReturn: true}
	itLOS := NewHuntAllNpcIterator(nil, stubLOS, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
	npc := &mockNpc{x: 3201, z: 3202, level: 0}
	_ = itLOS.passesFilter(npc)
	if len(stubLOS.losCalls) != 1 {
		t.Fatalf("LOS branch: expected 1 LV call, got %d", len(stubLOS.losCalls))
	}
	got := stubLOS.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3200, destX: 3201, destZ: 3202, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("LOS arg shape:\n got=%+v\nwant=%+v", got, want)
	}

	// LOW branch
	stubLOW := &stubLineValidatorArgs{lowReturn: true}
	itLOW := NewHuntAllNpcIterator(nil, stubLOW, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
	_ = itLOW.passesFilter(npc)
	if len(stubLOW.lowCalls) != 1 {
		t.Fatalf("LOW branch: expected 1 LV call, got %d", len(stubLOW.lowCalls))
	}
	got = stubLOW.lowCalls[0]
	if got != want {
		t.Fatalf("LOW arg shape:\n got=%+v\nwant=%+v", got, want)
	}
}
```

Notes:
- The `want` struct uses iterator-as-src (3200, 3200) / npc-as-dest (3201, 3202) — opposite of PlayerIterator. This mirrors `TestNpcIterator_PassesFilter_HuntAllMode_LineOfSight_IteratorAsSrc` at npc_iterator_test.go:345.
- Both LOS and LOW want the same `(1, 1, 1, 0)` tuple post-fix.

### Step 1.3: Run both failing tests to confirm RED

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestPlayerIterator_LineValidatorArgShape|TestNpcIterator_LineValidatorArgShape' -v`

Expected: both tests FAIL with a diagnostic of the form:

```
LOS arg shape:
 got={level:0 srcX:... srcZ:... destX:... destZ:... srcSize:1 destWidth:0 destLength:0 extraFlag:0}
want={... srcSize:1 destWidth:1 destLength:1 extraFlag:0}
```

The destWidth/destLength=0 mismatch confirms the pre-fix `(1, 0, 0, 0)` shape is in effect.

### Step 1.4: Flip the four production arg-shape lines

- [ ] **Edit `pkg/script/player_iterator.go`** to flip lines 71 and 77:

```go
// Line 71 (inside HuntVisLineOfSight case):
return it.lineValidator.HasLineOfSight(it.level, p.X(), p.Z(), it.x, it.z, 1, 1, 1, 0)

// Line 77 (inside HuntVisLineOfWalk case):
return it.lineValidator.HasLineOfWalk(it.level, p.X(), p.Z(), it.x, it.z, 1, 1, 1, 0)
```

Use Edit to replace `1, 0, 0, 0)` with `1, 1, 1, 0)` on each line. Two Edit calls (separate to avoid `replace_all` cross-contamination with future similar text).

- [ ] **Edit `pkg/script/npc_iterator.go`** to flip lines 127 and 139:

```go
// Line 127 (inside npcVisibleViaLineOfSight):
return it.lineValidator.HasLineOfSight(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 1, 1, 0)

// Line 139 (inside npcVisibleViaLineOfWalk):
return it.lineValidator.HasLineOfWalk(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 1, 1, 0)
```

Same two Edit calls.

### Step 1.5: Run the two new tests to confirm GREEN

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestPlayerIterator_LineValidatorArgShape|TestNpcIterator_LineValidatorArgShape' -v`

Expected: both PASS.

### Step 1.6: Update stale doc-comments at the touched sites

The `npc_iterator.go` LOS and LOW helpers carry comments that claim `(srcSize=1, destWidth=destLength=0, extraFlag=0)` "mirrors TS isLineOfWalk wrapper" — a doc-vs-code mismatch (`doc_comment_vs_code_mismatch.md`).

- [ ] **Edit `pkg/script/npc_iterator.go`** to replace the LOS helper preamble (lines 117-122):

Replace:
```go
// npcVisibleViaLineOfSight returns true when the iterator's lineValidator
// passes a LoS check from the iterator's center coord to the NPC. Nil
// validator = pessimistically allow. NAI-35-T3.
// (srcSize=1, destWidth=destLength=0, extraFlag=0) — single-tile src
// against a zero-size NPC dest; mirrors TS isLineOfSight wrapper at
// ScriptIterators.ts:359-361.
```

With:
```go
// npcVisibleViaLineOfSight returns true when the iterator's lineValidator
// passes a LoS check from the iterator's center coord to the NPC. Nil
// validator = pessimistically allow. NAI-35-T3.
// Arg tuple (1, 1, 1, 0) mirrors TS isLineOfSight wrapper at
// GameMap.ts:429-431, invoked from ScriptIterators.ts:284.
// NAI-166-D-LOW-ARG-SHAPE-SWEEP closes the prior (1, 0, 0, 0) shape.
```

- [ ] **Edit `pkg/script/npc_iterator.go`** to replace the LOW helper preamble (lines 130-134):

Replace:
```go
// npcVisibleViaLineOfWalk returns true when the iterator's lineValidator
// passes a LoW check. Nil validator = pessimistically allow. NAI-35-T3.
// (srcSize=1, destWidth=destLength=0, extraFlag=0) — single-tile src
// against a zero-size NPC dest; mirrors TS isLineOfWalk wrapper at
// ScriptIterators.ts:359-361.
```

With:
```go
// npcVisibleViaLineOfWalk returns true when the iterator's lineValidator
// passes a LoW check. Nil validator = pessimistically allow. NAI-35-T3.
// Arg tuple (1, 1, 1, 0) mirrors TS isLineOfWalk wrapper at
// GameMap.ts:425-427, invoked from ScriptIterators.ts:287.
// NAI-166-D-LOW-ARG-SHAPE-SWEEP closes the prior (1, 0, 0, 0) shape.
```

- [ ] **Verify `pkg/script/player_iterator.go`** has no analogous stale arg-shape comments.

Run: `grep -n "destWidth=destLength=0\|destWidth=0\|destLength=0\|1, 0, 0, 0" pkg/script/player_iterator.go`

Expected: zero matches. The `TS-faithful: PlayerHuntAllCommandIterator passes player-as-src` comment block at lines 67-70 describes the src/dest swap, not the arg shape, and stays untouched.

### Step 1.7: Drop the "iterator/hunt-site sweep ... NAI-166 candidate" foreshadow from the wrapper preamble

- [ ] **Edit `pkg/script/handlers_map.go`** at lines 172-176:

Replace:
```go
// destLength are passed verbatim. NAI-165-D-LOW-ARG-SHAPE-FIX widens this
// wrapper from the pre-fix (1, 0, 0, 0) shape to TS-faithful (1, 1, 1, 0);
// existing MapFindSquareLineOfWalk callers at lines 117, 147 inherit the
// corrected endpoint semantics. Pessimistic-allow on nil validator.
// NAI-35-T6 (NAI-165). The iterator/hunt-site sweep at player_iterator.go,
// npc_iterator.go, npc_hunt_entities.go, and npc_hunt.go (still on
// (1, 0, 0, 0)) is tracked separately as a NAI-166 candidate.
```

With:
```go
// destLength are passed verbatim. NAI-165-D-LOW-ARG-SHAPE-FIX widens this
// wrapper from the pre-fix (1, 0, 0, 0) shape to TS-faithful (1, 1, 1, 0);
// existing MapFindSquareLineOfWalk callers at lines 117, 147 inherit the
// corrected endpoint semantics. Pessimistic-allow on nil validator.
// NAI-35-T6 (NAI-165). NAI-166-D-LOW-ARG-SHAPE-SWEEP retired the
// iterator-side stragglers in pkg/script/player_iterator.go and
// pkg/script/npc_iterator.go (the modules/world hunt sites were already
// canonical at HEAD when NAI-166 opened).
```

Use Edit to perform this replacement; the surrounding text is unique enough to avoid `replace_all` ambiguity.

- [ ] **Verify the analogous LOS wrapper preamble has no symmetric foreshadow.** Read `pkg/script/handlers_map.go` lines 184-200 and confirm — the LOS wrapper at `isLineOfSight` is expected to have no NAI-166-candidate sentence (the LOS-side sweep wasn't explicitly foreshadowed; the LOS arg-shape gap landed by NAI-163). If you find one, retire it analogously; otherwise leave as-is.

### Step 1.8: Run the full pkg/script test suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`

Expected: all tests PASS, including the two new ones from Steps 1.1-1.2 and the existing iterator + LOS/LOW arg-shape tests (NAI-163 / NAI-165 pins).

If any existing test (e.g. `TestPlayerIterator_PassesFilter_LineOfSight_PlayerAsSrc`, `TestNpcIterator_PassesFilter_HuntAllMode_LineOfSight_IteratorAsSrc`) flips red, the production flip touched an unintended branch — halt and investigate before continuing.

### Step 1.9: Run the full repo test suite for non-regression

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: full GREEN. modules/world hunt tests use real `s.gamemap.Pathfinder.LineValidator` (no stub recording), so the production change in pkg/script does not affect them. Any failure here suggests a cross-package consequence that the spec missed.

### Step 1.10: Commit T1

- [ ] Run:

```bash
git add pkg/script/player_iterator.go pkg/script/npc_iterator.go pkg/script/player_iterator_test.go pkg/script/npc_iterator_test.go pkg/script/handlers_map.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(script): NAI-166 — iterator LV arg-shape sweep (1, 0, 0, 0) → (1, 1, 1, 0)

Flips the four pkg/script iterator LineValidator call sites to the
TS-canonical (srcSize=1, destWidth=1, destLength=1, extraFlag=0) tuple,
matching the isLineOfWalk/isLineOfSight wrappers at GameMap.ts:425-431.
Adds per-iterator arg-shape pins reusing the existing
stubLineValidatorArgs recorder. Retires stale doc-comments at the two
npc_iterator helpers and drops the NAI-166-candidate foreshadow from
the isLineOfWalk wrapper preamble.

Closes NAI-166-D-LOW-ARG-SHAPE-SWEEP.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `handleLineOfWalk` wrapper-routing + nil-LV test inversion

**Files:**
- Modify: `pkg/script/handlers_map.go` (`handleLineOfWalk` body ~lines 432-440; preamble ~lines 402-409)
- Modify: `pkg/script/handlers_map_test.go` (`TestHandleLineOfWalkNilValidator` ~line 945)

Rationale: `handleLineOfSight` already routes through `isLineOfSight(s, ...)` (handlers_map.go:236); the wrapper carries an `if s.LineValidator == nil { return true }` guard (pessimistic-allow). `handleLineOfWalk` has a parallel explicit `if s.LineValidator == nil { s.PushInt(0); return nil }` block followed by a direct `s.LineValidator.HasLineOfWalk(...)` call. Removing the explicit guard and routing through `isLineOfWalk(s, ...)` makes the two opcodes symmetric and brings goscape's nil-LV semantics in line with TS (both wrapper-routed, both pessimistic-allow).

### Step 2.1: Invert and rename `TestHandleLineOfWalkNilValidator`

- [ ] **Edit `pkg/script/handlers_map_test.go`** at lines 945-961:

Replace:
```go
func TestHandleLineOfWalkNilValidator(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = nil

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(coordgrid.PackCoord(0, 3201, 3200))

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("LINEOFWALK nil validator: got %d, want 0", got)
	}
}
```

With:
```go
// TestHandleLineOfWalkNilValidatorPessimisticAllow pins the post-NAI-166
// wrapper-routed semantics: nil LineValidator pushes 1 (pessimistic-allow)
// via the isLineOfWalk wrapper, mirroring handleLineOfSight's behavior at
// handlers_map.go:236. Pre-NAI-166 the handler had an explicit nil-guard
// that pushed 0 (pessimistic-deny) — that asymmetry was tracked as
// NAI-166-D-LOW-WRAPPER-ROUTING.
func TestHandleLineOfWalkNilValidatorPessimisticAllow(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = nil

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(coordgrid.PackCoord(0, 3201, 3200))

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("LINEOFWALK nil validator: got %d, want 1 (pessimistic-allow via isLineOfWalk wrapper)", got)
	}
}
```

### Step 2.2: Run the inverted test to confirm RED

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandleLineOfWalkNilValidatorPessimisticAllow' -v`

Expected: FAIL with `LINEOFWALK nil validator: got 0, want 1` — current handler still has the explicit nil-guard pushing 0.

### Step 2.3: Refactor `handleLineOfWalk` to route nil-handling through the wrapper

- [ ] **Edit `pkg/script/handlers_map.go`** at the `handleLineOfWalk` body (~lines 432-440):

Replace:
```go
	if s.LineValidator == nil {
		s.PushInt(0)
		return nil
	}
	if s.LineValidator.HasLineOfWalk(fromLevel, fromX, fromZ, toX, toZ, 1, 1, 1, 0) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
```

With:
```go
	if isLineOfWalk(s, fromLevel, fromX, fromZ, toX, toZ) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
```

Notes:
- `isLineOfWalk` is the same-file wrapper at `pkg/script/handlers_map.go:177-183`. Its body is `if s.LineValidator == nil { return true }; return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)` — so the new handler:
  - on nil LV: wrapper returns `true` → handler pushes 1 (was: pushed 0)
  - on non-nil LV: wrapper passes `(1, 1, 1, 0)` (same arg shape as the pre-refactor direct call)
- The existing `TestHandleLineOfWalk_ArgShape` at `handlers_map_test.go:1160` (which sets `s.LineValidator = stub` non-nil and asserts the recorded tuple) continues to pass — the wrapper passes through the exact same `(1, 1, 1, 0)` tuple.

### Step 2.4: Update the `handleLineOfWalk` preamble doc-comment

- [ ] **Edit `pkg/script/handlers_map.go`** at lines 397-411:

Replace:
```go
// handleLineOfWalk (LINEOFWALK, opcode 1006) reports whether a 1-tile
// entity at c1 has line-of-walk to c2. Pop order: top-of-stack is c2,
// c1 below. Pushes 1 on success, 0 on fail.
//
// Same-level guard: differing levels push 0 immediately.
// F2P short-circuit: in a non-members world, destination tile not in
// an F2P zone pushes 0.
// Nil-LineValidator: pushes 0 (fail closed) when no validator wired
// (goscape defensive; TS routes through isLineOfWalk wrapper which is
// pessimistic-ALLOW on nil — pre-existing asymmetry vs handleLineOfSight
// which delegates nil-handling to isLineOfSight (pessimistic-allow),
// tracked separately as a NAI-166 candidate).
//
// Arg shape: HasLineOfWalk(..., 1, 1, 1, 0) per NAI-165-D-LOW-ARG-SHAPE-FIX;
// matches the isLineOfWalk wrapper above and TS GameMap.ts:425-427.
//
// Mirrors TS ServerOps.ts:65-82.
```

With:
```go
// handleLineOfWalk (LINEOFWALK, opcode 1006) reports whether a 1-tile
// entity at c1 has line-of-walk to c2. Pop order: top-of-stack is c2,
// c1 below. Pushes 1 on success, 0 on fail.
//
// Same-level guard: differing levels push 0 immediately.
// F2P short-circuit: in a non-members world, destination tile not in
// an F2P zone pushes 0.
// Nil-LineValidator: routes through the isLineOfWalk wrapper
// (pessimistic-allow), matching handleLineOfSight. NAI-166-D-LOW-WRAPPER-ROUTING
// closed the prior explicit nil-guard / pessimistic-deny divergence.
//
// Arg shape: HasLineOfWalk(..., 1, 1, 1, 0) via isLineOfWalk wrapper;
// matches TS GameMap.ts:425-427. NAI-165-D-LOW-ARG-SHAPE-FIX.
//
// Mirrors TS ServerOps.ts:65-82.
```

### Step 2.5: Run the inverted test to confirm GREEN

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandleLineOfWalkNilValidatorPessimisticAllow' -v`

Expected: PASS.

### Step 2.6: Verify `TestHandleLineOfWalk_ArgShape` still passes

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandleLineOfWalk_ArgShape' -v`

Expected: PASS. The wrapper still passes `(1, 1, 1, 0)` to `HasLineOfWalk`, so the existing arg-shape pin (which records the call with a non-nil stub) is unaffected by the wrapper-routing refactor.

### Step 2.7: Verify the other handleLineOfWalk tests still pass

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandleLineOfWalk' -v`

Expected: PASS for `TestHandleLineOfWalkSameLevelTrue`, `TestHandleLineOfWalkDifferentLevels`, `TestHandleLineOfWalkF2PShortCircuit`, `TestHandleLineOfWalkNilValidatorPessimisticAllow`, `TestHandleLineOfWalk_ArgShape`. Each exercises a different gate of `handleLineOfWalk`; none of them depend on the deleted explicit nil-guard except the (now-inverted) nil-validator pin.

### Step 2.8: Run full pkg/script + repo test suites

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`

Expected: all tests PASS.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests PASS across the repo.

### Step 2.9: Close-out grep — verify no stale "NAI-166 candidate" foreshadow remains

- [ ] Run: `rg "NAI-166 candidate|tracked separately as a NAI-166" pkg/ modules/ cmd/`

Expected: zero matches. All foreshadow text retired by T1 step 1.7 and T2 step 2.4.

- [ ] Run: `rg "NAI-166" pkg/ modules/ cmd/`

Expected: only matches are the new deviation-tag headers introduced in T1 + T2 doc-comments (`NAI-166-D-LOW-ARG-SHAPE-SWEEP` and `NAI-166-D-LOW-WRAPPER-ROUTING`). No "candidate" / "tracked separately" residue.

### Step 2.10: Commit T2

- [ ] Run:

```bash
git add pkg/script/handlers_map.go pkg/script/handlers_map_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(script): NAI-166 — handleLineOfWalk routes nil-LV through isLineOfWalk wrapper

Deletes the explicit `if s.LineValidator == nil` pessimistic-deny block
from handleLineOfWalk and swaps the direct LV call for an
`isLineOfWalk(s, ...)` wrapper call. Nil-LV semantics flip from
pessimistic-deny (push 0) to pessimistic-allow (push 1), matching
handleLineOfSight's already-canonical wrapper-routed behavior.

TestHandleLineOfWalkNilValidator renamed to
TestHandleLineOfWalkNilValidatorPessimisticAllow and inverted from
expect-0 to expect-1. TestHandleLineOfWalk_ArgShape unaffected — the
wrapper passes the same (1, 1, 1, 0) tuple to HasLineOfWalk.

Closes NAI-166-D-LOW-WRAPPER-ROUTING.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Close commit (after T2 lands)

Per `runescript_cadence.md` and `close_commit_memory_trailer.md`:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-166 — iterator LOW+LOS arg-shape sweep + handleLineOfWalk wrapper-routing

Two pkg/script tails from NAI-165 close-out, bundled per brainstorm:
- 4 production lines flipped (1, 0, 0, 0) → (1, 1, 1, 0) in
  player_iterator.go and npc_iterator.go; modules/world hunt sites
  found already canonical at HEAD (likely fixed during NAI-9 / NAI-12).
- handleLineOfWalk refactored to route nil-LV through isLineOfWalk
  wrapper, matching handleLineOfSight's wrapper-routed pessimistic-allow.

Closes memory:
- NAI-166-D-LOW-ARG-SHAPE-SWEEP — iterator-side stragglers
- NAI-166-D-LOW-WRAPPER-ROUTING — LOW handler nil-guard asymmetry

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Verification summary

After both tasks land:

```bash
# All four pre-fix call sites in pkg/script gone
rg -n "1, 0, 0, 0" pkg/script/ modules/world/
# Expected: zero matches

# Both new deviation tags grep-visible
rg -n "NAI-166-D-LOW-ARG-SHAPE-SWEEP|NAI-166-D-LOW-WRAPPER-ROUTING" pkg/ modules/ cmd/
# Expected: matches at the doc-comments introduced by T1/T2 plus the close commit body

# No "candidate" foreshadow residue
rg "NAI-166 candidate|tracked separately as a NAI-166" pkg/ modules/ cmd/
# Expected: zero matches

# All tests green
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
