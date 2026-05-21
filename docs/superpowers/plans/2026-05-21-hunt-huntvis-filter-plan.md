# Hunt huntvis filter activation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Activate validated-but-unconsumed huntvis (LoS/LoW) filtering at the two `NAI-33-D1 / S7f-D1` pin sites: `NpcIterator` DISTANCE mode and `serverNpcLookup.FindClosestNpcByType/ByCategory`.

**Architecture:** Two independent prongs. **Prong A** restructures `NpcIterator.passesFilter` to lift the huntvis switch out of the HuntAll-only guard, adds a `LineValidator` arg to `NewDistanceNpcIterator`, and wires `s.LineValidator` into the two FINDALL/FINDALLANY handler call sites. **Prong B** adds a private `huntvisGate` helper to `serverNpcLookup` that reads `l.s.scriptLineValidator()` per call, and inserts a gate call into both FindClosest methods. A test-only `lineValidatorOverride` field on `Server` provides a stub-LineValidator seam for Prong B tests.

**Tech Stack:** Go 1.26.3 (via `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go` per env-quirk). Strict TDD with pre-stub strategy for Go's "package must compile" reality. Subagent-driven-development with sonnet implementers + two-stage review per task.

**Spec:** `docs/superpowers/specs/2026-05-21-hunt-huntvis-filter-design.md` (committed at `e3bb9b31`).

**Dependencies:** T1 → T2 sequential (signature change). T3 independent of T1/T2 (different package). T4 sweep after T1+T2+T3. T5 close after T4.

---

## File Structure

**Modified:**
- `pkg/script/npc_iterator.go` — `passesFilter` restructure + `NewDistanceNpcIterator` sig change + 5 doc-comment refreshes (T1)
- `pkg/script/npc_iterator_test.go` — 5 existing fixture updates + 6 new unit tests (T1)
- `pkg/script/handlers_npc.go` — 2 production call sites + 3 doc-comment refreshes (T2 + T4)
- `pkg/script/handlers_npc_test.go` — 2 new handler-level smoke tests (T2)
- `pkg/script/state.go` — 1 doc-comment refresh (T4)
- `modules/world/npc_script_lookup.go` — `huntvisGate` helper + 2 method body updates + 3 doc-comment refreshes (T3)
- `modules/world/npc_script_lookup_test.go` — 10 new unit tests (T3)
- `modules/world/server.go` — add `lineValidatorOverride` field on Server (T3)
- `modules/world/script.go` — `scriptLineValidator()` prefers override when non-nil (T3)

**Unchanged (verified):**
- `pkg/script/player_iterator.go` (single-mode by NAI-35-D2)
- `modules/world/npc_hunt*.go` (separate hunt-engine path)
- `pkg/script/handlers_npc.go` HuntAll handler (already wired)
- ~30 `serverNpcLookup{s: s}` test fixtures across `modules/world/*_test.go` (no struct shape change)

---

## Task 1: Iterator core — passesFilter restructure + signature change

**Files:**
- Modify: `pkg/script/npc_iterator.go` (lines 32-129 struct + passesFilter + line 175 NewDistanceNpcIterator + 5 doc-comment sites)
- Modify: `pkg/script/npc_iterator_test.go` (existing fixture updates at lines 73, 166, 207, 228, 244 + 6 new tests)

### Step 1.1: Pre-stub — widen signature without behavior change

Why pre-stub: Go's "package must compile" reality. Test fixtures using current `NewDistanceNpcIterator` sig must compile before we can observe RED on new tests. The pre-stub adds the `lv` parameter as ignored, preserving current `passesFilter` behavior.

- [ ] **Step 1.1.1: Add `lv LineValidator` parameter to `NewDistanceNpcIterator`, set struct field**

In `pkg/script/npc_iterator.go`, modify the function at line 175. Current signature:

```go
func NewDistanceNpcIterator(lookup NpcLookup, tick, level, x, z, distance, huntvis, typeID int) *NpcIterator {
```

New signature (lv as 2nd positional, mirroring `NewHuntAllNpcIterator` order `lookup, lv, configs, ...`):

```go
func NewDistanceNpcIterator(lookup NpcLookup, lv LineValidator, tick, level, x, z, distance, huntvis, typeID int) *NpcIterator {
```

Inside the function body, add `lineValidator: lv,` to the struct literal alongside `lookup: lookup,`.

- [ ] **Step 1.1.2: Update 5 existing test-call sites in `pkg/script/npc_iterator_test.go`**

Insert `nil` as 2nd positional arg at each site. Exact line numbers and replacements:

- Line 73: `NewDistanceNpcIterator(nil, 0, 0, tc.x, tc.z, tc.distance, 0, -1)` → `NewDistanceNpcIterator(nil, nil, 0, 0, tc.x, tc.z, tc.distance, 0, -1)`
- Line 166: `NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 0, 0, -1)` → `NewDistanceNpcIterator(lookup, nil, 0, 0, 3200, 3300, 0, 0, -1)`
- Line 207: `NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 5, 0, -1)` → `NewDistanceNpcIterator(lookup, nil, 0, 0, 3200, 3300, 5, 0, -1)`
- Line 228: `NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 5, 0, 42)` → `NewDistanceNpcIterator(lookup, nil, 0, 0, 3200, 3300, 5, 0, 42)`
- Line 244: `NewDistanceNpcIterator(lookup2, 0, 0, 3200, 3300, 5, 0, -1)` → `NewDistanceNpcIterator(lookup2, nil, 0, 0, 3200, 3300, 5, 0, -1)`

- [ ] **Step 1.1.3: Update 2 production call sites in `pkg/script/handlers_npc.go`**

Current at line 835 (`handleNpcFindAllAny`):

```go
	s.npcIterator = NewDistanceNpcIterator(
		s.Npcs, s.World.CurrentTick(),
		level, x, z, distance, checkVis, -1,
	)
```

Insert `s.LineValidator` as 2nd positional arg:

```go
	s.npcIterator = NewDistanceNpcIterator(
		s.Npcs, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis, -1,
	)
```

Same at line 873 (`handleNpcFindAll`):

```go
	s.npcIterator = NewDistanceNpcIterator(
		s.Npcs, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis, npcTypeID,
	)
```

- [ ] **Step 1.1.4: Verify all packages compile**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go build ./...`

Expected: clean exit, no errors.

- [ ] **Step 1.1.5: Verify no behavioral regression**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/script/... -run 'TestNpcIterator' -count=1`

Expected: all existing iterator tests PASS (sig change is behavior-preserving — `lv` not yet consumed).

### Step 1.2: RED — write new unit tests for Distance-mode huntvis filter

- [ ] **Step 1.2.1: Add `TestNpcIteratorDistance_HuntVisOff_NoFilter` to `npc_iterator_test.go`**

Insert at end of file (after the last existing test). Body:

```go
// TestNpcIteratorDistance_HuntVisOff_NoFilter — baseline regression:
// HuntVisOff disables LoS/LoW filtering regardless of validator return.
// Closes NAI-33-D1 for FINDALL family (TS ScriptIterators.ts:348-352 —
// Distance mode now consumes huntvis like HuntAll).
func TestNpcIteratorDistance_HuntVisOff_NoFilter(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
	lv := &stubLineValidator{losReturn: false, lowReturn: false} // would block if consulted
	it := NewDistanceNpcIterator(lookup, lv, 0, 0, 3200, 3300, 5, objtype.HuntVisOff, -1)

	got, ok := it.Next()
	if !ok || got != npc1 {
		t.Errorf("HuntVisOff should emit npc1; got (%v, %v)", got, ok)
	}
}
```

- [ ] **Step 1.2.2: Add `TestNpcIteratorDistance_HuntVisLineOfSight`**

```go
// TestNpcIteratorDistance_HuntVisLineOfSight — Distance mode now applies
// LoS filter per TS ScriptIterators.ts:348. Table-driven 2-way.
func TestNpcIteratorDistance_HuntVisLineOfSight(t *testing.T) {
	cases := []struct {
		name      string
		losReturn bool
		wantEmit  bool
	}{
		{"LoS passes → emit", true, true},
		{"LoS blocks → skip", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
			zoneKey := mockZoneKey(0, 3200, 3296)
			lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
			lv := &stubLineValidator{losReturn: tc.losReturn}
			it := NewDistanceNpcIterator(lookup, lv, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight, -1)

			got, ok := it.Next()
			if tc.wantEmit && (!ok || got != npc1) {
				t.Errorf("expected emit npc1; got (%v, %v)", got, ok)
			}
			if !tc.wantEmit && (ok || got != nil) {
				t.Errorf("expected skip (LoS blocked); got (%v, %v)", got, ok)
			}
		})
	}
}
```

- [ ] **Step 1.2.3: Add `TestNpcIteratorDistance_HuntVisLineOfWalk`**

```go
// TestNpcIteratorDistance_HuntVisLineOfWalk — Distance mode now applies
// LoW filter per TS ScriptIterators.ts:351. Table-driven 2-way.
func TestNpcIteratorDistance_HuntVisLineOfWalk(t *testing.T) {
	cases := []struct {
		name      string
		lowReturn bool
		wantEmit  bool
	}{
		{"LoW passes → emit", true, true},
		{"LoW blocks → skip", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
			zoneKey := mockZoneKey(0, 3200, 3296)
			lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
			lv := &stubLineValidator{lowReturn: tc.lowReturn}
			it := NewDistanceNpcIterator(lookup, lv, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfWalk, -1)

			got, ok := it.Next()
			if tc.wantEmit && (!ok || got != npc1) {
				t.Errorf("expected emit npc1; got (%v, %v)", got, ok)
			}
			if !tc.wantEmit && (ok || got != nil) {
				t.Errorf("expected skip (LoW blocked); got (%v, %v)", got, ok)
			}
		})
	}
}
```

- [ ] **Step 1.2.4: Add `TestNpcIteratorDistance_NilLineValidator_PessimisticAllow`**

```go
// TestNpcIteratorDistance_NilLineValidator_PessimisticAllow — when no
// validator is wired (lv=nil), huntvis=LoS/LoW pessimistically allows.
// Matches the HuntAll convention at npc_iterator.go:138-141.
func TestNpcIteratorDistance_NilLineValidator_PessimisticAllow(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
	it := NewDistanceNpcIterator(lookup, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight, -1)

	got, ok := it.Next()
	if !ok || got != npc1 {
		t.Errorf("nil-validator + LoS should pessimistically allow; got (%v, %v)", got, ok)
	}
}
```

- [ ] **Step 1.2.5: Add `TestNpcIteratorDistance_LineOfSightArgShape`**

Guards `NAI-166-D-LOW-ARG-SHAPE-SWEEP` precedent (iterator-as-src, NOT player-iterator-reversed shape at TS line 216).

```go
// TestNpcIteratorDistance_LineOfSightArgShape pins the LoS arg tuple per
// TS ScriptIterators.ts:348: isLineOfSight(level, this.x, this.z, npc.x, npc.z).
// Iterator-as-src ordering — NOT the player-iterator-reversed shape at TS
// line 216 (see PlayerIterator.passesFilter for that variant).
// Guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent.
func TestNpcIteratorDistance_LineOfSightArgShape(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3201, z: 3302, level: 7}
	zoneKey := mockZoneKey(7, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
	rec := &recordingLineValidator{losReturn: true}
	it := NewDistanceNpcIterator(lookup, rec, 0, 7, 3200, 3300, 5, objtype.HuntVisLineOfSight, -1)

	_, _ = it.Next()
	if rec.losLevel != 7 || rec.losSrcX != 3200 || rec.losSrcZ != 3300 || rec.losDestX != 3201 || rec.losDestZ != 3302 {
		t.Errorf("LoS arg shape: got (level=%d, src=%d,%d, dst=%d,%d), want (7, 3200, 3300, 3201, 3302) — iterator-as-src",
			rec.losLevel, rec.losSrcX, rec.losSrcZ, rec.losDestX, rec.losDestZ)
	}
}
```

- [ ] **Step 1.2.6: Add `TestNpcIteratorZone_HuntVisStillUnfiltered`**

```go
// TestNpcIteratorZone_HuntVisStillUnfiltered — Zone mode must remain
// unfiltered after Distance-mode huntvis activation, matching TS
// ScriptIterators.ts:329-335 (zero filtering in the ZONE branch).
// Note: Zone-mode iterator can't actually receive huntvis via the
// constructor (NewZoneNpcIterator takes no huntvis arg), so this pin
// directly mutates the struct field to prove passesFilter ignores it.
func TestNpcIteratorZone_HuntVisStillUnfiltered(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
	it := NewZoneNpcIterator(lookup, 0, 0, 3200, 3300)
	// Force huntvis + a blocking validator into the Zone-mode iterator
	// to prove passesFilter early-returns true regardless.
	it.huntvis = objtype.HuntVisLineOfSight
	it.lineValidator = &stubLineValidator{losReturn: false}

	got, ok := it.Next()
	if !ok || got != npc1 {
		t.Errorf("Zone mode must ignore huntvis; got (%v, %v), want (npc1, true)", got, ok)
	}
}
```

- [ ] **Step 1.2.7: Run new tests — verify all 6 FAIL**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/script/... -run 'TestNpcIteratorDistance_HuntVis|TestNpcIteratorDistance_NilLineValidator|TestNpcIteratorDistance_LineOfSightArgShape|TestNpcIteratorZone_HuntVisStill' -count=1 -v`

Expected: All 6 tests FAIL — `TestNpcIteratorDistance_HuntVisLineOfSight/LoS blocks → skip` fails because the current `passesFilter` only runs the huntvis switch in HuntAll mode (line 111), so Distance mode emits the blocked NPC. Pass tests (`losReturn: true`, `NilLineValidator`) coincidentally PASS but for the wrong reason — they pass because the switch is skipped entirely. `LineOfSightArgShape` FAILS because the validator never gets called.

The arg-shape test failing the loudest is the strongest RED signal: `rec.losLevel == 0` (zero-value) instead of 7. `TestNpcIteratorZone_HuntVisStillUnfiltered` actually passes (Zone mode early-returns), but include it for future-regression coverage.

### Step 1.3: GREEN — restructure `passesFilter`

- [ ] **Step 1.3.1: Lift huntvis switch out of HuntAll-only guard**

In `pkg/script/npc_iterator.go`, replace `passesFilter` (lines 92-129) with:

```go
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS ScriptIterators.ts:329-335
	}
	// HuntAll-mode op[1] reject runs BEFORE distance check per TS order
	// at ScriptIterators.ts:274-282. NAI-180 closes NAI-35-T3-D1.
	if it.mode == NpcIteratorHuntAll && it.configs != nil {
		// (goscape defensive; TS throws on missing NpcType) — when the
		// configs lookup returns nil (unknown NPC type), pessimistically
		// allow to match the lineValidator==nil convention at
		// npcVisibleViaLineOfSight. Production NPCs always have a type.
		npcType := it.configs.NpcType(npc.NpcType())
		if npcType != nil && (len(npcType.Op) <= 1 || npcType.Op[1] == "") {
			return false
		}
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	// Huntvis filter applies in Distance + HuntAll modes (TS
	// ScriptIterators.ts:348-352 for DISTANCE, :284-287 for HuntAll —
	// identical arg shape). Zone mode early-returns above per TS line
	// 329-335 (unfiltered yield).
	switch it.huntvis {
	case objtype.HuntVisOff:
		// no LoS/LoW gate
	case objtype.HuntVisLineOfSight:
		if !it.npcVisibleViaLineOfSight(npc) {
			return false
		}
	case objtype.HuntVisLineOfWalk:
		if !it.npcVisibleViaLineOfWalk(npc) {
			return false
		}
	}
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}
```

Key change: the `switch it.huntvis` block is no longer guarded by `if it.mode == NpcIteratorHuntAll`. Zone mode keeps early-return at top; HuntAll-mode op[1] gate stays as-is.

- [ ] **Step 1.3.2: Run new tests — verify all 6 PASS**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/script/... -run 'TestNpcIteratorDistance_HuntVis|TestNpcIteratorDistance_NilLineValidator|TestNpcIteratorDistance_LineOfSightArgShape|TestNpcIteratorZone_HuntVisStill' -count=1 -v`

Expected: All 6 PASS.

- [ ] **Step 1.3.3: Run full pkg/script tests — verify no regression**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/script/... -count=1`

Expected: PASS across all pkg/script tests.

### Step 1.4: Doc-comment refreshes in `npc_iterator.go`

- [ ] **Step 1.4.1: Refresh `huntvis` field comment (lines 41-46)**

Current:

```go
	// huntvis is the LoS/LoW gate level (HuntVisOff/LineOfSight/
	// LineOfWalk). Consumed by passesFilter ONLY in HuntAll mode
	// (NAI-35-T3); Distance and Zone modes validate but do not filter,
	// preserving NAI-33-D1's deferred-not-consumed posture for the
	// FINDALL/FINDALLANY/FINDALLZONE family. Audit if those families
	// gain LoS/LoW content-script consumers.
	huntvis int
```

Replace with:

```go
	// huntvis is the LoS/LoW gate level (HuntVisOff/LineOfSight/
	// LineOfWalk). Consumed by passesFilter in Distance + HuntAll modes
	// (TS ScriptIterators.ts:348-352 for DISTANCE, :284-287 for HuntAll
	// — identical arg shape). Zone mode unfiltered per TS line 329-335
	// (NpcIterator ZONE branch yields without huntvis checks; the
	// npc_findallzone command takes no huntvis arg either, per
	// engine.rs2:605).
	huntvis int
```

- [ ] **Step 1.4.2: Refresh `lineValidator` field comment (lines 48-52)**

Current:

```go
	// lineValidator is the LoS/LoW validator used by HuntAll-mode
	// passesFilter when huntvis ∈ {LineOfSight, LineOfWalk}. Nil = no
	// validator wired (test stub or pre-wiring); production sets via
	// the constructor. NAI-35-T3.
	lineValidator LineValidator
```

Replace with:

```go
	// lineValidator is the LoS/LoW validator used by passesFilter in
	// Distance + HuntAll modes when huntvis ∈ {LineOfSight, LineOfWalk}.
	// Nil = no validator wired (test stub or pre-wiring) → pessimistic
	// allow; production sets via the constructor. NAI-35-T3 (HuntAll),
	// extended to Distance per TS ScriptIterators.ts:348-352.
	lineValidator LineValidator
```

- [ ] **Step 1.4.3: Refresh `passesFilter` doc (above the function, lines 83-91)**

Current:

```go
// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// HuntAll mode (NAI-35-T3) activates the huntvis branch — ZONE mode
// remains unfiltered (matches TS line 329-335). Distance mode keeps
// the pre-NAI-35 deferred behavior (huntvis validated but not consumed;
// tracked as NAI-33-D1 / S7f-D1). TS DOES filter Distance mode by
// huntvis (ScriptIterators.ts:348-352); goscape's deferred posture is
// intentional pending FINDALL-family consumer audit.
// Accessor names match pkg/script/active.go:400-408 ActiveNpc interface
// (NpcX/NpcZ/NpcType, NOT X/Z/Type).
```

Replace with:

```go
// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// Both Distance and HuntAll modes consume huntvis (TS
// ScriptIterators.ts:348-352 / :284-287); ZONE mode early-returns
// unfiltered (TS line 329-335). HuntAll-mode op[1] reject (NAI-180)
// runs before the distance check; otherwise the chain is shared:
// distance → huntvis → typeID. Accessor names match
// pkg/script/active.go:400-408 ActiveNpc interface (NpcX/NpcZ/NpcType,
// NOT X/Z/Type).
```

- [ ] **Step 1.4.4: Refresh `NewDistanceNpcIterator` doc (lines 156-174)**

Current:

```go
// NewDistanceNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by typeID (-1 = no filter).
// Mirrors TS NpcIterator constructor at ScriptIterators.ts:310-326 with
// type=DISTANCE.
//
// huntvis is stored at construction (validated upstream by handlers via
// checkHuntVis) but NOT consumed by passesFilter — preserves the
// NAI-33-D1 deferred posture for FINDALL family (no LoS/LoW
// content-script consumers identified). HuntAll mode (NAI-35-T3) is the
// only iterator-mode that activates the huntvis filter.
//
// Bounds math (per TS line 312-321):
//
//	centerX = x >> 3
//	radius  = 1 + distance/8       // integer division
//	zone bounds = [center - radius, center + radius]
//
// Cursor starts at (maxZoneX, maxZoneZ) per TS line 337-340; advances
// outer X descending, inner Z descending in advanceZone (Task 6).
```

Replace with:

```go
// NewDistanceNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by typeID (-1 = no filter).
// Mirrors TS NpcIterator constructor at ScriptIterators.ts:310-326 with
// type=DISTANCE.
//
// huntvis is consumed by passesFilter per TS ScriptIterators.ts:348-352
// (Distance mode filters by LoS/LoW like HuntAll). lv may be nil
// (pessimistic-allow per the lineValidator==nil convention at
// npcVisibleViaLineOfSight); production passes s.LineValidator from the
// handler call site.
//
// Bounds math (per TS line 312-321):
//
//	centerX = x >> 3
//	radius  = 1 + distance/8       // integer division
//	zone bounds = [center - radius, center + radius]
//
// Cursor starts at (maxZoneX, maxZoneZ) per TS line 337-340; advances
// outer X descending, inner Z descending in advanceZone.
```

- [ ] **Step 1.4.5: Refresh `NewHuntAllNpcIterator` doc (lines 215-219)**

Current:

```go
// NewHuntAllNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (ACTIVE per
// NAI-35-T3 — partially closes NAI-33-D1 for HuntAll mode; Distance
// mode + FindClosest* still residual) and the NpcType.Op[1] operability
// gate (NAI-180 closes NAI-35-T3-D1; TS ScriptIterators.ts:274-280).
```

Replace with:

```go
// NewHuntAllNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (TS
// ScriptIterators.ts:284-287) and the NpcType.Op[1] operability gate
// (NAI-180 closes NAI-35-T3-D1; TS ScriptIterators.ts:274-280).
```

- [ ] **Step 1.4.6: Run full pkg/script tests after doc refreshes**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/script/... -count=1`

Expected: PASS.

### Step 1.5: Commit T1

- [ ] **Step 1.5.1: Pre-commit hygiene check**

Run: `git status` and `git diff --stat`

Expected: Modified `pkg/script/npc_iterator.go`, `pkg/script/npc_iterator_test.go`, `pkg/script/handlers_npc.go` (just the 2 call sites). No staged file should be `config.yaml` or standing untracked noise.

- [ ] **Step 1.5.2: Stage and commit**

```bash
git add pkg/script/npc_iterator.go pkg/script/npc_iterator_test.go pkg/script/handlers_npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): activate huntvis filter in NpcIterator DISTANCE mode (TS:348-352)

Lift the huntvis LoS/LoW switch out of the HuntAll-only guard in
NpcIterator.passesFilter so both DISTANCE and HuntAll modes consume
huntvis (TS ScriptIterators.ts:348-352 / :284-287 — identical arg
shape). ZONE mode early-returns unfiltered (TS line 329-335). Adds
`lv LineValidator` as 2nd positional arg to NewDistanceNpcIterator;
handlers_npc.go FINDALL / FINDALLANY call sites pass s.LineValidator.

+6 tests in npc_iterator_test.go (huntvis-off, LoS pass/fail, LoW
pass/fail, nil-validator pessimistic-allow, arg-shape pin, Zone-mode
still-unfiltered). 5 existing fixture call sites updated with nil lv.

Part 1 of NAI-33-D1 / S7f-D1 retire (Prong A — iterator). Prong B
(FindClosest*) follows in T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 1.5.3: Post-commit verification**

Run: `git show --stat HEAD` and `git status`

Expected: commit shows ~3 files changed; working tree clean apart from standing noise.

---

## Task 2: Iterator production wiring — handler smoke tests

**Depends on:** T1 (signature change).

**Files:**
- Modify: `pkg/script/handlers_npc.go` (doc comments at lines 805-813 + 842-849 — production code already wired in T1)
- Modify: `pkg/script/handlers_npc_test.go` (2 new smoke tests)

### Step 2.1: Locate existing handler-level FINDALL/FINDALLANY tests for fixture patterns

- [ ] **Step 2.1.1: Read existing handler test patterns**

Run: `grep -n "TestHandleNpcFindAll\|TestNpcFindAll\|handleNpcFindAll\|handleNpcFindAllAny" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_npc_test.go | head -20`

Read the matching test(s) to understand the fixture pattern (ScriptState setup, mock npcs, FINDNEXT loop drive).

Expected: existing tests show the shape — `s := &ScriptState{Npcs: ..., World: ..., LineValidator: nil}`, then `s.IntStack = []int{coord, npcType, distance, huntvis}` push, then `handleNpcFindAll(s)`, then loop `handleNpcFindNext(s)` collecting yielded npcs.

### Step 2.2: RED — write 2 new smoke tests

- [ ] **Step 2.2.1: Add `TestHandleNpcFindAll_PlumbsLineValidatorToIterator`**

Insert at end of `handlers_npc_test.go`. Adapt the ScriptState fixture pattern from existing tests; key bits: set `s.LineValidator = &stubLineValidator{losReturn: false}`, push huntvis=`objtype.HuntVisLineOfSight`, then assert FINDNEXT yields zero hits.

```go
// TestHandleNpcFindAll_PlumbsLineValidatorToIterator — proves that
// handleNpcFindAll passes ScriptState.LineValidator into the iterator,
// so vis_lineofsight + always-false validator yields zero matches.
// Plumbing-only; iterator behavior covered by npc_iterator_test.go.
func TestHandleNpcFindAll_PlumbsLineValidatorToIterator(t *testing.T) {
	npc1 := &mockNpc{typeID: 42, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
	s := &ScriptState{
		Npcs:          lookup,
		World:         &mockWorld{tick: 1},
		LineValidator: &stubLineValidator{losReturn: false}, // always block
		Configs:       newConfigsWithNpcType(42),
	}
	// Push args bottom-to-top per TS popInts(4): coord, npc, distance, huntvis
	coord := packCoord(0, 3200, 3300)
	s.PushInt(coord)
	s.PushInt(42) // npcType
	s.PushInt(5)  // distance
	s.PushInt(objtype.HuntVisLineOfSight)

	if err := handleNpcFindAll(s); err != nil {
		t.Fatalf("handleNpcFindAll: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("expected npcIterator to be set")
	}
	// Drive FINDNEXT loop; collect yielded npcs.
	yielded := 0
	for s.npcIterator != nil {
		got, ok := s.npcIterator.Next()
		if !ok {
			break
		}
		if got != nil {
			yielded++
		}
	}
	if yielded != 0 {
		t.Errorf("LineValidator returns false (always-block) + LoS huntvis: expected 0 yields, got %d", yielded)
	}
}
```

**Implementer note:** If `mockWorld`, `newConfigsWithNpcType`, or `packCoord` helpers don't exist with those exact names, grep `handlers_npc_test.go` for the local equivalents and substitute. The pattern (push args, call handler, drive iterator) is the shape that matters; arg-pack helper names are local conventions. If the test file uses a different fixture-builder name (e.g., `newScriptStateForTest`), use it.

- [ ] **Step 2.2.2: Add `TestHandleNpcFindAllAny_PlumbsLineValidatorToIterator`**

Same pattern, with `handleNpcFindAllAny` and 3 stack pushes (coord, distance, huntvis — no npc filter). Use `objtype.HuntVisLineOfWalk` here to cover the LoW path through the handler too.

```go
// TestHandleNpcFindAllAny_PlumbsLineValidatorToIterator — proves
// handleNpcFindAllAny plumbs ScriptState.LineValidator. Uses LoW path
// for handler-test coverage parity with LoS in FindAll's smoke test.
func TestHandleNpcFindAllAny_PlumbsLineValidatorToIterator(t *testing.T) {
	npc1 := &mockNpc{typeID: 42, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1}}}
	s := &ScriptState{
		Npcs:          lookup,
		World:         &mockWorld{tick: 1},
		LineValidator: &stubLineValidator{lowReturn: false}, // always block
	}
	coord := packCoord(0, 3200, 3300)
	s.PushInt(coord)
	s.PushInt(5)  // distance
	s.PushInt(objtype.HuntVisLineOfWalk)

	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("handleNpcFindAllAny: %v", err)
	}
	yielded := 0
	for s.npcIterator != nil {
		got, ok := s.npcIterator.Next()
		if !ok {
			break
		}
		if got != nil {
			yielded++
		}
	}
	if yielded != 0 {
		t.Errorf("LineValidator returns false + LoW huntvis: expected 0 yields, got %d", yielded)
	}
}
```

- [ ] **Step 2.2.3: Run new tests — verify both PASS**

T1 already wired `s.LineValidator` into the iterator, so these smoke tests should pass immediately on first run. This is intentional — they're regression guards verifying T1's wiring persists, not RED tests in the traditional sense.

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/script/... -run 'TestHandleNpcFindAll_PlumbsLineValidator|TestHandleNpcFindAllAny_PlumbsLineValidator' -count=1 -v`

Expected: Both PASS.

**Implementer note — if they FAIL:** check that `s.LineValidator` is non-nil after construction (vs zero-value), and that `s.npcIterator` is the iterator created by the handler (not a leftover). Refer back to T1 step 1.1.3 to verify the production wiring took effect.

### Step 2.3: Doc-comment refreshes in `handlers_npc.go`

- [ ] **Step 2.3.1: Refresh `handleNpcFindAllAny` doc (lines 805-813)**

Current:

```go
// handleNpcFindAllAny (NPC_FINDALLANY, opcode 2515) pops (coord, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator on
// state.npcIterator with no type filter. Mirrors TS NpcOps.ts:403-411.
// Pointer-set is `set ['find_npc']` (ScriptOpcodePointers.ts:586-588);
// goscape encodes the find_npc pointer as state.npcIterator != nil.
// No push (TS doesn't push either). NAI-33-D1: huntvis validated but
// not consumed by passesFilter (Distance mode preserves the
// deferred-not-consumed posture; HuntAll mode at NAI-35-T3 is the only
// mode that activates LoS/LoW filtering).
```

Replace with:

```go
// handleNpcFindAllAny (NPC_FINDALLANY, opcode 2515) pops (coord, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator on
// state.npcIterator with no type filter. Mirrors TS NpcOps.ts:403-411.
// Pointer-set is `set ['find_npc']` (ScriptOpcodePointers.ts:586-588);
// goscape encodes the find_npc pointer as state.npcIterator != nil.
// No push (TS doesn't push either). huntvis filtering is active per TS
// ScriptIterators.ts:348-352 (Distance-mode LoS/LoW consumed by
// passesFilter); s.LineValidator is plumbed into the iterator.
```

- [ ] **Step 2.3.2: Refresh `handleNpcFindAll` doc (lines 842-849)**

Current:

```go
// handleNpcFindAll (NPC_FINDALL, opcode 2514) pops (coord, npc, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator with
// typeID set to filter by NPC type. Mirrors TS NpcOps.ts:413-422.
// Pop order matches TS popInts(4): top → bottom = checkVis, distance,
// npcTypeID, coord. NAI-33-D1: huntvis validated but not consumed by
// passesFilter (Distance mode preserves the deferred-not-consumed
// posture; HuntAll mode at NAI-35-T3 is the only mode that activates
// LoS/LoW filtering).
```

Replace with:

```go
// handleNpcFindAll (NPC_FINDALL, opcode 2514) pops (coord, npc, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator with
// typeID set to filter by NPC type. Mirrors TS NpcOps.ts:413-422.
// Pop order matches TS popInts(4): top → bottom = checkVis, distance,
// npcTypeID, coord. huntvis filtering is active per TS
// ScriptIterators.ts:348-352; s.LineValidator is plumbed into the
// iterator.
```

- [ ] **Step 2.3.3: Run tests one more time after doc refreshes**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/script/... -count=1`

Expected: PASS.

### Step 2.4: Commit T2

- [ ] **Step 2.4.1: Pre-commit hygiene check**

Run: `git status` and `git diff --stat`

Expected: modified `pkg/script/handlers_npc.go` (doc comments) + `pkg/script/handlers_npc_test.go` (2 new tests). No `config.yaml` etc.

- [ ] **Step 2.4.2: Stage and commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): handler-level smoke tests for huntvis plumbing in FINDALL family

Add two plumbing-guard smoke tests (TestHandleNpcFindAll_PlumbsLineValidatorToIterator
+ TestHandleNpcFindAllAny_PlumbsLineValidatorToIterator) that drive the
handler → iterator → FINDNEXT chain with always-block validators and
assert zero yields. LoS path covers FINDALL; LoW covers FINDALLANY for
parity coverage.

Doc-comment refreshes on both handlers drop the NAI-33-D1
deferred-not-consumed framing now that Distance mode actively filters.

Part 2 of NAI-33-D1 / S7f-D1 retire (Prong A handler wiring); Prong B
follows in T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2.4.3: Post-commit verification**

Run: `git show --stat HEAD` and `git status`

Expected: 2 files changed; clean working tree.

---

## Task 3: FindClosest core — huntvisGate helper + production wiring + tests

**Independent of T1/T2** (different package, different prong).

**Files:**
- Modify: `modules/world/server.go` (add `lineValidatorOverride` field on Server struct)
- Modify: `modules/world/script.go` (`scriptLineValidator()` prefers override)
- Modify: `modules/world/npc_script_lookup.go` (huntvisGate helper + 2 method updates + doc refreshes)
- Modify: `modules/world/npc_script_lookup_test.go` (10 new tests)

### Step 3.1: Add test-only `lineValidatorOverride` seam on `Server`

- [ ] **Step 3.1.1: Locate Server struct field block**

Run: `sed -n '94,98p' /home/owner/Code/github.com/zsrv/goscape/modules/world/server.go`

Expected: shows `gamemap *gamemap.GameMap` at line 94. We'll add `lineValidatorOverride` immediately below it.

- [ ] **Step 3.1.2: Add field to Server struct**

In `modules/world/server.go`, after the `gamemap *gamemap.GameMap` line (line 94), insert:

```go

	// lineValidatorOverride is a test-only seam: when non-nil,
	// scriptLineValidator() returns it instead of
	// gamemap.Pathfinder.LineValidator. Production New() never sets
	// this; only test fixtures that need to wire a stub LineValidator
	// without a real gamemap (e.g., FindClosestNpcBy* tests in
	// modules/world/npc_script_lookup_test.go) write to it directly.
	// Nil = production path (read from gamemap).
	lineValidatorOverride script.LineValidator
```

Note: This requires importing `github.com/zsrv/goscape/pkg/script` in `server.go`. Check imports first.

Run: `grep -n '"github.com/zsrv/goscape/pkg/script"' /home/owner/Code/github.com/zsrv/goscape/modules/world/server.go`

If no hit, add `script "github.com/zsrv/goscape/pkg/script"` to the imports block (matching the existing import-alias convention if any; otherwise plain import).

- [ ] **Step 3.1.3: Update `scriptLineValidator()` to prefer override**

In `modules/world/script.go`, replace the function body (lines 13-17):

```go
func (s *Server) scriptLineValidator() script.LineValidator {
	if s.gamemap == nil {
		return nil
	}
	return s.gamemap.Pathfinder.LineValidator
}
```

With:

```go
func (s *Server) scriptLineValidator() script.LineValidator {
	if s.lineValidatorOverride != nil {
		return s.lineValidatorOverride
	}
	if s.gamemap == nil {
		return nil
	}
	return s.gamemap.Pathfinder.LineValidator
}
```

- [ ] **Step 3.1.4: Refresh `scriptLineValidator` doc-comment (lines 7-12)**

Current:

```go
// scriptLineValidator returns the LineValidator wired into all script
// state-init sites. Returns nil if the gamemap has not been initialized
// (only happens in unit-test fixtures that build a stripped-down
// Server). Production callers always have gamemap set via New().
// HuntAll-mode passesFilter pessimistically allows on a nil validator,
// so the test path degrades gracefully. NAI-35-T3.
```

Replace with:

```go
// scriptLineValidator returns the LineValidator wired into all script
// state-init sites and into serverNpcLookup's huntvisGate (Prong B of
// the NAI-33-D1 retire). Lookup order: lineValidatorOverride (test
// seam) → gamemap.Pathfinder.LineValidator → nil. Production callers
// always have gamemap set via New(); the test seam is for fixtures
// that need a stub validator without a real gamemap. Distance + HuntAll
// passesFilter and huntvisGate all pessimistically allow on a nil
// return, so the test path degrades gracefully. NAI-35-T3.
```

- [ ] **Step 3.1.5: Verify modules/world compiles**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go build ./modules/world/...`

Expected: clean exit.

### Step 3.2: Pre-stub — widen FindClosest signatures (already correct), add helper stub

The existing method signatures already accept `huntvis int` (just discarded as `_ int`); we'll restore the name and add the helper as a stub returning `true` so all FindClosest tests still pass.

- [ ] **Step 3.2.1: Restore `huntvis` parameter name in `FindClosestNpcByType`**

In `modules/world/npc_script_lookup.go` line 24, change:

```go
func (l serverNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, _ int) script.ActiveNpc {
```

To:

```go
func (l serverNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, huntvis int) script.ActiveNpc {
```

- [ ] **Step 3.2.2: Restore `huntvis` parameter name in `FindClosestNpcByCategory`**

Line 62, change:

```go
func (l serverNpcLookup) FindClosestNpcByCategory(level, x, z, dist, cat, _ int) script.ActiveNpc {
```

To:

```go
func (l serverNpcLookup) FindClosestNpcByCategory(level, x, z, dist, cat, huntvis int) script.ActiveNpc {
```

- [ ] **Step 3.2.3: Add `huntvisGate` as a return-true stub**

At the bottom of `npc_script_lookup.go` (after `ZoneNpcs`), add:

```go
// huntvisGate applies the HuntVisOff/LineOfSight/LineOfWalk filter
// using the server's scriptLineValidator. Nil-validator → pessimistic
// allow, matching the pkg/script iterator convention at
// npc_iterator.go:138-141 (npcVisibleViaLineOfSight).
//
// Arg tuple (1, 1, 1, 0) and iterator-as-src ordering mirror TS
// NpcIterator DISTANCE-mode at ScriptIterators.ts:348/351 — NOT the
// player-iterator-reversed shape at line 216 (see PlayerIterator
// passesFilter for that variant). Closes NAI-33-D1 / S7f-D1 for the
// FindClosestNpc* family.
func (l serverNpcLookup) huntvisGate(level, srcX, srcZ, dstX, dstZ, huntvis int) bool {
	// STUB — wired in step 3.5. Behavior-preserving here (returns true
	// for all huntvis values, matching pre-slice behavior).
	return true
}
```

- [ ] **Step 3.2.4: Verify compile + existing tests pass**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./modules/world/... -run 'TestServerNpcLookup' -count=1`

Expected: existing 3 lookup tests PASS (`TestServerNpcLookup_FindClosestByType`, `TestServerNpcLookup_FindClosestByCategory`, `TestServerNpcLookup_FindAtExactCoord`).

### Step 3.3: RED — write 10 new unit tests for FindClosest huntvis filtering

- [ ] **Step 3.3.1: Add `TestFindClosestNpcByType_HuntVisOff_Baseline`**

Insert in `modules/world/npc_script_lookup_test.go` after the existing FindClosestByType test (around line 60). Pattern follows `setupLookupServer`.

```go
// TestFindClosestNpcByType_HuntVisOff_Baseline — regression guard:
// HuntVisOff continues to return the closest type-matched NPC even
// when an always-block validator is wired. Pre-slice behavior preserved.
func TestFindClosestNpcByType_HuntVisOff_Baseline(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false, lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisOff))
	if got == nil {
		t.Fatal("HuntVisOff with blocking validator should still emit")
	}
}
```

- [ ] **Step 3.3.2: Add `fakeLineValidator` helper at top of file**

After the existing imports/package declaration, add:

```go
// fakeLineValidator is a script.LineValidator test double for
// FindClosestNpc* huntvis tests. Mirrors pkg/script's stubLineValidator
// in shape; defined locally because the script package's stub isn't
// exported.
type fakeLineValidator struct {
	losReturn bool
	lowReturn bool
}

func (f *fakeLineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return f.losReturn
}

func (f *fakeLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return f.lowReturn
}

// recordingFakeLineValidator captures args for arg-shape pin tests.
type recordingFakeLineValidator struct {
	losLevel, losSrcX, losSrcZ, losDestX, losDestZ int
	losReturn                                      bool
}

func (r *recordingFakeLineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	r.losLevel, r.losSrcX, r.losSrcZ, r.losDestX, r.losDestZ = level, srcX, srcZ, destX, destZ
	return r.losReturn
}

func (r *recordingFakeLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return false
}
```

- [ ] **Step 3.3.3: Add `TestFindClosestNpcByType_HuntVisLineOfSight_FiltersBlocked`**

```go
// TestFindClosestNpcByType_HuntVisLineOfSight_FiltersBlocked — proves
// LoS-blocked NPCs are skipped; only the LoS-passing NPC at same dist
// is returned. Closes NAI-33-D1 for NPC_FIND via huntvisGate wiring.
func TestFindClosestNpcByType_HuntVisLineOfSight_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	// Always-block: nothing should pass except via pessimistic-allow,
	// which only triggers on nil validator. With validator present,
	// always-false → all candidates filtered → nil result.
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false}
	npc1 := setupNpc(t, s, 50, 50, 0)
	npc1.typeId = 7
	npc2 := setupNpc(t, s, 51, 50, 0)
	npc2.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 49, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got != nil {
		t.Errorf("all candidates LoS-blocked: expected nil, got %v", got)
	}

	// Now flip to always-pass and verify the closer one wins.
	s.lineValidatorOverride = &fakeLineValidator{losReturn: true}
	got = s.npcLookup.FindClosestNpcByType(0, 49, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Fatal("LoS-passing: expected emit, got nil")
	}
	if got.(*Npc) != npc1 {
		t.Errorf("LoS-passing: expected closer npc1 (at 50,50 vs lookup at 49,50), got npc2 (at 51,50)")
	}
}
```

- [ ] **Step 3.3.4: Add `TestFindClosestNpcByType_HuntVisLineOfWalk_FiltersBlocked`**

```go
// TestFindClosestNpcByType_HuntVisLineOfWalk_FiltersBlocked — LoW variant.
func TestFindClosestNpcByType_HuntVisLineOfWalk_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfWalk))
	if got != nil {
		t.Errorf("LoW-blocked: expected nil, got %v", got)
	}

	s.lineValidatorOverride = &fakeLineValidator{lowReturn: true}
	got = s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfWalk))
	if got == nil {
		t.Fatal("LoW-passing: expected emit, got nil")
	}
}
```

- [ ] **Step 3.3.5: Add `TestFindClosestNpcByType_NilLineValidator_PessimisticAllow`**

```go
// TestFindClosestNpcByType_NilLineValidator_PessimisticAllow — when
// scriptLineValidator returns nil (no gamemap + no override), huntvis
// filter pessimistically allows. Matches HuntAll-mode iterator
// convention at pkg/script/npc_iterator.go:138-141.
func TestFindClosestNpcByType_NilLineValidator_PessimisticAllow(t *testing.T) {
	s := setupLookupServer(t)
	// s.gamemap == nil (newTestServer doesn't wire one), s.lineValidatorOverride == nil
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Error("nil-validator + LoS huntvis should pessimistically allow")
	}
}
```

- [ ] **Step 3.3.6: Add `TestFindClosestNpcByType_HuntVisAfterDistance_ClosestStillWins`**

```go
// TestFindClosestNpcByType_HuntVisAfterDistance_ClosestStillWins — 2
// LoS-passing NPCs at different distances; closer wins. Validates
// huntvis filter doesn't disturb the closest-by-euclidean-squared
// selection or the later-match-wins (<=) tie-break.
func TestFindClosestNpcByType_HuntVisAfterDistance_ClosestStillWins(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: true}
	near := setupNpc(t, s, 50, 50, 0)
	near.typeId = 7
	far := setupNpc(t, s, 60, 50, 0)
	far.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Fatal("expected emit")
	}
	if got.(*Npc) != near {
		t.Errorf("expected closer NPC; got far one")
	}
}
```

- [ ] **Step 3.3.7: Add `TestFindClosestNpcByType_LineOfSightArgShape`**

```go
// TestFindClosestNpcByType_LineOfSightArgShape pins the LoS arg tuple
// per TS NpcIterator DISTANCE-mode at ScriptIterators.ts:348:
// isLineOfSight(level, lookupX, lookupZ, npc.x, npc.z) — iterator-as-src
// ordering. Guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent at the
// FindClosest call site.
func TestFindClosestNpcByType_LineOfSightArgShape(t *testing.T) {
	s := setupLookupServer(t)
	rec := &recordingFakeLineValidator{losReturn: true}
	s.lineValidatorOverride = rec
	npc := setupNpc(t, s, 51, 52, 3)
	npc.typeId = 7

	_ = s.npcLookup.FindClosestNpcByType(3, 50, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if rec.losLevel != 3 || rec.losSrcX != 50 || rec.losSrcZ != 50 || rec.losDestX != 51 || rec.losDestZ != 52 {
		t.Errorf("LoS arg shape: got (level=%d, src=%d,%d, dst=%d,%d), want (3, 50, 50, 51, 52) — lookup-as-src",
			rec.losLevel, rec.losSrcX, rec.losSrcZ, rec.losDestX, rec.losDestZ)
	}
}
```

- [ ] **Step 3.3.8: Add `TestFindClosestNpcByCategory_HuntVisOff_Baseline`**

```go
// TestFindClosestNpcByCategory_HuntVisOff_Baseline — regression guard
// for NPC_FINDCAT, mirror of TestFindClosestNpcByType_HuntVisOff_Baseline.
func TestFindClosestNpcByCategory_HuntVisOff_Baseline(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false, lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7 // category 5 per setupLookupServer

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisOff))
	if got == nil {
		t.Fatal("HuntVisOff with blocking validator should still emit")
	}
}
```

- [ ] **Step 3.3.9: Add `TestFindClosestNpcByCategory_HuntVisLineOfSight_FiltersBlocked`**

```go
// TestFindClosestNpcByCategory_HuntVisLineOfSight_FiltersBlocked — LoS
// filter wiring on NPC_FINDCAT closes NAI-33-D1 for the category variant.
func TestFindClosestNpcByCategory_HuntVisLineOfSight_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7 // category 5

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))
	if got != nil {
		t.Errorf("LoS-blocked: expected nil, got %v", got)
	}

	s.lineValidatorOverride = &fakeLineValidator{losReturn: true}
	got = s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Error("LoS-passing: expected emit, got nil")
	}
}
```

- [ ] **Step 3.3.10: Add `TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked`**

```go
// TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked — LoW
// variant for NPC_FINDCAT.
func TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfWalk))
	if got != nil {
		t.Errorf("LoW-blocked: expected nil, got %v", got)
	}
}
```

- [ ] **Step 3.3.11: Add `TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow`**

```go
// TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow — nil
// validator + LoS huntvis → emit (pessimistic-allow convention).
func TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow(t *testing.T) {
	s := setupLookupServer(t)
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Error("nil-validator + LoS huntvis should pessimistically allow")
	}
}
```

- [ ] **Step 3.3.12: Run new tests — verify 6 of them FAIL (blocked-validator tests)**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./modules/world/... -run 'TestFindClosestNpc' -count=1 -v`

Expected:
- PASS (no-filter behavior is current default): `HuntVisOff_Baseline`, `NilLineValidator_PessimisticAllow`, `HuntVisAfterDistance_ClosestStillWins`, and the always-pass sub-cases inside LoS/LoW filter tests
- FAIL: the always-block sub-cases of `LineOfSight_FiltersBlocked` + `LineOfWalk_FiltersBlocked` (×2 type + ×2 category = 4 sub-cases) — current `huntvisGate` stub returns true → blocked candidates incorrectly emit
- FAIL: `LineOfSightArgShape` — stub never calls the validator → recorder fields stay at zero

Note: imports needed in the test file: add `"github.com/zsrv/goscape/pkg/objtype"` if not already present. Run `goimports -w modules/world/npc_script_lookup_test.go` or add manually.

### Step 3.4: GREEN — wire `huntvisGate` real implementation

- [ ] **Step 3.4.1: Replace stub `huntvisGate` body with real logic**

In `modules/world/npc_script_lookup.go`, replace the stub body (the `return true // STUB ...` line) with:

```go
func (l serverNpcLookup) huntvisGate(level, srcX, srcZ, dstX, dstZ, huntvis int) bool {
	switch huntvis {
	case objtype.HuntVisOff:
		return true
	case objtype.HuntVisLineOfSight:
		lv := l.s.scriptLineValidator()
		if lv == nil {
			return true
		}
		return lv.HasLineOfSight(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0)
	case objtype.HuntVisLineOfWalk:
		lv := l.s.scriptLineValidator()
		if lv == nil {
			return true
		}
		return lv.HasLineOfWalk(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0)
	}
	return true
}
```

Note: This requires `"github.com/zsrv/goscape/pkg/objtype"` import in `npc_script_lookup.go`. Check imports first — if not present, add to import block.

Run: `grep -n '"github.com/zsrv/goscape/pkg/objtype"' /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_script_lookup.go`

If no hit, add the import.

- [ ] **Step 3.4.2: Wire `huntvisGate` call into `FindClosestNpcByType`**

In `npc_script_lookup.go`, after the `if dx > dist || dz > dist { continue }` block (was line 39-41), and before the `d := ...` line (was line 42), insert:

```go
		if !l.huntvisGate(level, x, z, n.x, n.z, huntvis) {
			continue
		}
```

- [ ] **Step 3.4.3: Wire `huntvisGate` call into `FindClosestNpcByCategory`**

Same insertion at the corresponding location in `FindClosestNpcByCategory` — after the `dx/dz > dist` check (was line 87-89), before the `d := ...` line (was line 90):

```go
		if !l.huntvisGate(level, x, z, n.x, n.z, huntvis) {
			continue
		}
```

- [ ] **Step 3.4.4: Run new FindClosest tests — verify all PASS**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./modules/world/... -run 'TestFindClosestNpc' -count=1 -v`

Expected: All 10 new tests PASS.

- [ ] **Step 3.4.5: Run full modules/world tests — verify no regression**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./modules/world/... -count=1 -timeout 5m`

Expected: PASS across modules/world (existing tests have nil-LineValidator + huntvis=0 fixtures, so they pessimistic-allow and behavior is preserved).

### Step 3.5: Doc-comment refreshes in `npc_script_lookup.go`

- [ ] **Step 3.5.1: Refresh package comment (lines 7-11)**

Current:

```go
// serverNpcLookup implements script.NpcLookup by linearly iterating
// s.npcs. See S7f spec §3.3 and the residual deviation NAI-33-D1 /
// S7f-D1 (huntvis validated-only on FindClosest* — partially closed by
// NAI-35 for HuntAll-mode iterators) and S7f-D2 (linear iteration).
type serverNpcLookup struct{ s *Server }
```

Replace with:

```go
// serverNpcLookup implements script.NpcLookup by linearly iterating
// s.npcs (S7f-D2 — direct-scan vs TS's NpcIterator(DISTANCE)-then-closest
// shape at NpcOps.ts:347-380). huntvis filtering on FindClosest* is
// active via huntvisGate (closes NAI-33-D1 / S7f-D1).
type serverNpcLookup struct{ s *Server }
```

- [ ] **Step 3.5.2: Refresh `FindClosestNpcByType` doc (lines 16-23)**

Current:

```go
// FindClosestNpcByType returns the NPC of typeID closest (euclidean-squared)
// to (level, x, z) within a square-bounded dist. Later-iterated NPCs win
// ties (TS NpcOps.ts:353 uses `<=`). Mirrors TS NpcOps.ts:336-367.
//
// huntvis is validated upstream but NOT filtered on here — preserves the
// NAI-33-D1 / S7f-D1 deferred posture (audit if NPC_FIND consumers gain
// LoS/LoW gating). HuntAll-mode iterators (NewHuntAllNpcIterator) DO
// filter; this method does not.
```

Replace with:

```go
// FindClosestNpcByType returns the NPC of typeID closest (euclidean-squared)
// to (level, x, z) within a square-bounded dist. Later-iterated NPCs win
// ties (TS NpcOps.ts:353 uses `<=`). Mirrors TS NpcOps.ts:336-367.
//
// huntvis is consumed via huntvisGate (LoS/LoW filter applied after the
// square-bounds check, before euclidean-squared distance compute);
// nil-LineValidator pessimistically allows. Semantic-equivalent to TS
// NPC_FIND's NpcIterator(DISTANCE)-then-closest shape at NpcOps.ts:347-365.
```

- [ ] **Step 3.5.3: Refresh `FindClosestNpcByCategory` doc (lines 54-61)**

Current:

```go
// FindClosestNpcByCategory is the NPC_FINDCAT analogue of FindClosestNpcByType.
// Filters on NpcType.Category == cat rather than typeID. Looks up Category
// via l.s.npcTypes.Configs (with nil guards). Mirrors TS NpcOps.ts:369-400.
//
// huntvis is validated upstream but NOT filtered on here — preserves the
// NAI-33-D1 / S7f-D1 deferred posture (audit if NPC_FINDCAT consumers
// gain LoS/LoW gating). HuntAll-mode iterators (NewHuntAllNpcIterator)
// DO filter; this method does not.
```

Replace with:

```go
// FindClosestNpcByCategory is the NPC_FINDCAT analogue of FindClosestNpcByType.
// Filters on NpcType.Category == cat rather than typeID. Looks up Category
// via l.s.npcTypes.Configs (with nil guards). Mirrors TS NpcOps.ts:369-400.
//
// huntvis is consumed via huntvisGate (same shape as FindClosestNpcByType);
// semantic-equivalent to TS NPC_FINDCAT's NpcIterator(DISTANCE)-then-closest
// shape at NpcOps.ts:380-396.
```

- [ ] **Step 3.5.4: Run modules/world tests after doc refreshes**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./modules/world/... -count=1 -timeout 5m`

Expected: PASS.

### Step 3.6: Commit T3

- [ ] **Step 3.6.1: Pre-commit hygiene check**

Run: `git status` and `git diff --stat`

Expected: Modified `modules/world/server.go`, `modules/world/script.go`, `modules/world/npc_script_lookup.go`, `modules/world/npc_script_lookup_test.go`. No `config.yaml` or noise.

- [ ] **Step 3.6.2: Stage and commit**

```bash
git add modules/world/server.go modules/world/script.go modules/world/npc_script_lookup.go modules/world/npc_script_lookup_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): activate huntvis filter in FindClosestNpc* (NpcOps.ts:347-380 parity)

Add huntvisGate helper to serverNpcLookup that reads
l.s.scriptLineValidator() per call and applies the LoS/LoW filter at
arg tuple (1,1,1,0) iterator-as-src ordering. Wire into
FindClosestNpcByType + FindClosestNpcByCategory after the existing
square-bounds check, before the euclidean-squared distance compute.

Test seam: new lineValidatorOverride field on Server, preferred by
scriptLineValidator() when non-nil, lets FindClosest tests inject a
stub LineValidator without wiring a full gamemap. Production New()
never sets it.

+10 tests covering: HuntVisOff baseline preserves behavior; LoS/LoW
always-block returns nil; nil-validator pessimistically allows;
closest-still-wins among LoS-passing candidates; LoS arg-shape pin
(guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent at the FindClosest
call site); category variant mirrors type variant.

Part 3 of NAI-33-D1 / S7f-D1 retire (Prong B). T4 follows with
doc-comment sweep across pkg/script/state.go and handlers_npc.go.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.6.3: Post-commit verification**

Run: `git show --stat HEAD` and `git status`

Expected: 4 files changed; clean working tree.

---

## Task 4: Doc-comment sweep across pkg/script + audit-grep verification

**Depends on:** T1, T2, T3 (consolidates remaining framing across files touched in prior tasks).

**Files:**
- Modify: `pkg/script/state.go` (line 195-200 NpcLookup interface comment)
- Modify: `pkg/script/handlers_npc.go` (line 905-908 handleNpcHuntAll doc)

### Step 4.1: Sweep `pkg/script/state.go`

- [ ] **Step 4.1.1: Refresh NpcLookup interface comment (lines 189-200)**

Current:

```go
// NpcLookup is the script→world bridge for NPC_FIND family opcodes. All
// methods return the matching NPC as script.ActiveNpc or nil when no
// match. Implementations iterate the world NPC registry; see
// serverNpcLookup (modules/world/npc_script_lookup.go) for the
// production impl.
//
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*).
// FindClosestNpcByType / FindClosestNpcByCategory currently validate
// huntvis but do not filter on it (NAI-33-D1 / S7f-D1 residual after
// NAI-35 — HuntAll-mode iterators NewHuntAllNpcIterator /
// NewHuntAllPlayerIterator DO filter). Callers must still validate via
// checkHuntVis.
```

Replace with:

```go
// NpcLookup is the script→world bridge for NPC_FIND family opcodes. All
// methods return the matching NPC as script.ActiveNpc or nil when no
// match. Implementations iterate the world NPC registry; see
// serverNpcLookup (modules/world/npc_script_lookup.go) for the
// production impl.
//
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*).
// Implementations apply LoS/LoW filtering per TS Distance-mode semantics
// (ScriptIterators.ts:348-352); callers validate via checkHuntVis upstream.
```

### Step 4.2: Sweep `pkg/script/handlers_npc.go` `handleNpcHuntAll`

- [ ] **Step 4.2.1: Refresh `handleNpcHuntAll` doc (lines 905-908)**

Current:

```go
// NAI-35-T3: partially closes NAI-33-D1 (huntvis becomes a live
// consumer of LoS/LoW filtering via passesFilter HuntAll branch);
// Distance mode + FindClosestNpc* still residual.
```

Replace with:

```go
// NAI-35-T3: HuntAll-mode iterator activated LoS/LoW filtering at the
// passesFilter HuntAll branch. Distance mode and FindClosestNpc*
// followed at NAI-33-D1 retire (TS ScriptIterators.ts:348-352).
```

### Step 4.3: Audit-grep verification

- [ ] **Step 4.3.1: Run zero-hit audit greps in production .go**

Each command must return zero hits:

```bash
echo "=== NAI-33-D1 ==="
grep -rn "NAI-33-D1" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go
echo "=== S7f-D1 ==="
grep -rn "S7f-D1" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go
echo "=== deferred-not-consumed posture ==="
grep -rn "deferred-not-consumed posture" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go
echo "=== HuntAll is the only mode / only mode that activates ==="
grep -rn "HuntAll is the only mode\|only mode that activates" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go
echo "=== Distance mode + FindClosest* still residual ==="
grep -rn "Distance mode + FindClosestNpc\* still residual\|Distance mode + FindClosest\* still residual" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go
echo "=== validated upstream but NOT filtered ==="
grep -rn "validated upstream but NOT filtered" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go
echo "=== validated upstream by handlers via checkHuntVis) but NOT consumed ==="
grep -rn "validated upstream by handlers via checkHuntVis) but NOT consumed" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go
```

Expected: each section emits no lines (just the header echo). If ANY hit found, locate and refresh the comment before proceeding.

- [ ] **Step 4.3.2: Verify allowed citations still present (sanity check)**

```bash
echo "=== NAI-35-T3 (citation retained) ==="
grep -rn "NAI-35-T3" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go | wc -l
echo "=== NAI-180 (citation retained) ==="
grep -rn "NAI-180" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | grep -v _test.go | wc -l
echo "=== ScriptIterators.ts:348-352 ==="
grep -rn "ScriptIterators.ts:348-352\|ScriptIterators.ts:348" /home/owner/Code/github.com/zsrv/goscape --include="*.go" | wc -l
```

Expected: NAI-35-T3 ≥ 5 (preserved across iterator/lookup/state files), NAI-180 ≥ 2 (op[1] gate), ScriptIterators.ts:348 citations ≥ 3 (new citations added across iterator + lookup + state).

### Step 4.4: Race + smoke gates

- [ ] **Step 4.4.1: Run race detector across all packages**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test -race ./... -count=1 -timeout 10m`

Expected: 0 FAIL across all packages.

- [ ] **Step 4.4.2: Run pack smoke test**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/pack/... -run TestPackAll_TwelveStageSmoke -count=1 -v`

Expected: PASS.

### Step 4.5: Commit T4

- [ ] **Step 4.5.1: Pre-commit hygiene check**

Run: `git status` and `git diff --stat`

Expected: Modified `pkg/script/state.go`, `pkg/script/handlers_npc.go`. Audit-grep verifications produce no file changes.

- [ ] **Step 4.5.2: Stage and commit**

```bash
git add pkg/script/state.go pkg/script/handlers_npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(script): sweep NAI-33-D1/S7f-D1 framing across NpcLookup + handleNpcHuntAll

NpcLookup interface doc now describes huntvis as actively filtered per
TS ScriptIterators.ts:348-352. handleNpcHuntAll doc updated to drop
"Distance mode + FindClosestNpc* still residual" wording — both prongs
retired in T1-T3.

Audit-grep gates pass with zero hits in production .go:
  NAI-33-D1, S7f-D1, "deferred-not-consumed posture",
  "HuntAll is the only mode", "only mode that activates",
  "Distance mode + FindClosest* still residual",
  "validated upstream but NOT filtered",
  "validated upstream by handlers via checkHuntVis) but NOT consumed".

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4.5.3: Post-commit verification**

Run: `git show --stat HEAD`

Expected: 2 files changed.

---

## Task 5: Close — memory entry + final gate verification

**Depends on:** T1, T2, T3, T4.

**Files:**
- Create: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/hunt_huntvis_filter_close.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (prepend index line)

### Step 5.1: Final gate verification

- [ ] **Step 5.1.1: Re-run race detector + smoke (sanity check)**

```bash
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test -race ./... -count=1 -timeout 10m
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test ./pkg/pack/... -run TestPackAll_TwelveStageSmoke -count=1
```

Expected: 0 FAIL; smoke PASS.

- [ ] **Step 5.1.2: Re-run audit-grep gates**

(Re-run the audit-grep commands from Step 4.3.1.)

Expected: zero hits.

- [ ] **Step 5.1.3: Verify commit chain**

Run: `git log --oneline -6`

Expected: T1 commit → T2 commit → T3 commit → T4 commit on top of `e3bb9b31` (spec) on top of `56517545` (predecessor HEAD).

### Step 5.2: Write memory entry

- [ ] **Step 5.2.1: Create memory file**

Write to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/hunt_huntvis_filter_close.md` using the file frontmatter format documented in CLAUDE.md (or in the global memory instructions). The content should include:

- Slice date (2026-05-21) + commit range (T1..T4 hashes + spec hash `e3bb9b31`)
- Predecessor: `[[addxp-session-log-half-port-close]]` (HEAD `56517545`)
- Pin retirements: NAI-33-D1 + S7f-D1 fully retired (-2 board). NAI-35-T3 framing refreshed (citation retained).
- Audit gate finding: 4 NPC_FIND consumers (chompy_bird, rantz, quest_fluffs × 3 in 2 files), 1 NPC_FINDALL consumer (ducks) in LostCityRS/Content
- Two prongs: iterator side (passesFilter restructure + sig change) + FindClosest side (huntvisGate helper)
- Test counts: 6 iterator unit + 2 handler smoke + 10 lookup unit = 18 new tests; 5 existing fixture updates; 0 existing test behavioral assertion changes
- Non-obvious findings: any surfaces that turned out different from the spec's prediction (note inline as discovered during implementation)
- New seam: `Server.lineValidatorOverride` test-only field (test fixture documented inline)
- Race + smoke: clean
- Carry-forward menu: next pivots from predecessor minus this slice's item

- [ ] **Step 5.2.2: Prepend MEMORY.md index entry**

Add at the top of `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`:

```markdown
- [Hunt huntvis filter activation close](hunt_huntvis_filter_close.md) — <one-line hook with key facts, ~150 char max>
```

The hook should mention: 5-commit slice, T1..T4 + spec, NAI-33-D1/S7f-D1 retired (-2), iterator DISTANCE + FindClosest both prongs, 18 new tests, audit gate satisfied with named Content consumers.

- [ ] **Step 5.2.3: Resume prompt update (if convention persists)**

Write `/home/owner/Code/github.com/zsrv/goscape/.claude/resume/2026-05-21-hunt-huntvis-filter-close-resume.md` following the predecessor's resume format. Include: state at rest, what shipped, carry-forward menu (predecessor minus this item), recommended opening move, what NOT to do at session start.

Note: `.claude/` is in the untracked-noise list (not committed). The resume file is intentionally non-committed working notes.

### Step 5.3: No-commit for memory + resume

Memory + resume files live outside the repo's tracked tree (in `~/.claude/` and untracked `.claude/`). No git operations.

- [ ] **Step 5.3.1: Final verification**

Run: `git log --oneline -5 && echo "---" && git status`

Expected: 4 T-task commits on top of spec commit. Working tree clean apart from standing noise.

---

## Self-Review

**1. Spec coverage:**

- Prong A iterator restructure → T1.3
- Prong A signature change → T1.1.1
- Prong A doc refreshes (5 sites in npc_iterator.go) → T1.4.1–T1.4.5
- Prong A production wiring → T1.1.3 (code) + T2.3 (docs)
- Prong A handler smoke tests → T2.2
- Prong B huntvisGate helper → T3.2.3 (stub) + T3.4.1 (real)
- Prong B FindClosest method updates → T3.2.1, T3.2.2 (sig restore) + T3.4.2, T3.4.3 (filter wire)
- Prong B doc refreshes (3 sites in npc_script_lookup.go) → T3.5
- Prong B test infrastructure (Server.lineValidatorOverride) → T3.1
- Prong B 10 new tests → T3.3
- Sweep state.go + handlers_npc.go residual framing → T4.1, T4.2
- Audit-grep gates → T4.3.1
- Race + smoke → T4.4 + T5.1.1
- Memory entry + retire → T5.2

All 18 spec test items covered; all doc-refresh sites covered.

**2. Placeholder scan:** No TBD, no "implement later", no "similar to Task N", no abstract "add validation". Each step has either exact replacement code or exact command + expected output.

**3. Type consistency:**
- `lv LineValidator` arg name consistent across T1 sig, T2 wiring, T1.4 docs.
- `huntvisGate(level, srcX, srcZ, dstX, dstZ, huntvis int) bool` signature consistent across T3.2.3 stub, T3.4.1 real impl, T3.4.2/T3.4.3 call sites, T3.5.2 docs.
- `lineValidatorOverride script.LineValidator` field name consistent across T3.1.2 declaration, T3.1.3 scriptLineValidator read, T3.3.1+ test fixtures.
- `fakeLineValidator` / `recordingFakeLineValidator` test helpers consistent across all Prong B test fixtures (T3.3.2 declaration + T3.3.3-T3.3.11 uses).

**4. Spec ↔ plan deltas:** None material. Plan made one concrete decision the spec deferred: Prong B test infrastructure approach is `lineValidatorOverride` seam (T3.1), not the alternative "minimal gamemap fixture" option. This is the cheaper path and the spec explicitly authorized deferring this decision to T3 implementer.
