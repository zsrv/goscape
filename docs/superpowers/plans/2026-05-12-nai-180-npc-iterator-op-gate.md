# NAI-180 — NpcIterator HuntAll `op[1]` operability gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the TS `NpcType.op[1]==""` reject filter to goscape's HuntAll-mode NPC iterator, retiring deviation tag `NAI-35-T3-D1`.

**Architecture:** Add a `configs Configs` field to `NpcIterator`, thread `s.Configs` from the 2 production call sites in `handlers_npc.go`, and insert an op-gate in `passesFilter`'s HuntAll branch (TS-faithful order: BEFORE distance check). Nil-Configs pessimistic-allow matches the existing `lineValidator == nil` convention.

**Tech Stack:** Go 1.26+ (per `go_version.md`). No new deps. Modules touched: `pkg/script/`.

**Spec:** `docs/superpowers/specs/2026-05-12-nai-180-npc-iterator-op-gate-design.md` (committed at `b7ee794`).

**TS canonical:** `LostCityRS/Engine-TS/src/engine/script/ScriptIterators.ts:274-280` (per `ts_source_canonical_path.md`).

**Test command prefix (per global CLAUDE.md):**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

**Commit prefix (per global CLAUDE.md):** `git commit --no-gpg-sign ...`

---

## Pre-flight verified (already done; controller pre-flight)

- **`Configs` interface** lives at `pkg/script/configs.go:13` with `NpcType(id int) *objtype.NpcType` method.
- **`mockConfigs` test fixture** at `pkg/script/handlers_config_test.go:11-29` has `npcs map[int]*objtype.NpcType` and `NpcType(id)` accessor at line 32; populate with test fixtures via `newTestConfigs().npcs[id] = &objtype.NpcType{...}`.
- **`NewHuntAllNpcIterator` current signature** (`pkg/script/npc_iterator.go:209`):
  ```go
  func NewHuntAllNpcIterator(lookup NpcLookup, lv LineValidator, tick, level, x, z, distance, huntvis int) *NpcIterator
  ```
- **Call sites in production** (both pass `s` ScriptState which has `s.Configs`):
  - `pkg/script/handlers_npc.go:879-882` (`handleNpcHuntAll`)
  - `pkg/script/handlers_npc.go:924` (`handleNpcHunt`)
- **`NewHuntAllNpcIterator` call sites in tests** (10 total, all in `pkg/script/npc_iterator_test.go`): lines 264, 293, 304, 314, 323, 332, 336, 348, 373, 387.
- **`mockNpc` test fixture** at `pkg/script/handlers_npc_test.go:200` has `typeID int` field and `NpcType() int` accessor at line 255 returning `m.typeID`.
- **`NpcType.Op []string`** at `pkg/objtype/npctype.go:149`; populated by `t.Op[code-30] = dat.GJStrLF()` for codes 30..34 (5 slots, indices 0-4).

---

## Bundle 0 — RED (struct field + test fixture wiring)

This bundle is **mechanical**: add the new field + parameter, thread `nil` through existing test calls. No behavior change yet. End state: all existing tests still green; build still passes.

### Task B0.T1 — Add `configs Configs` field to `NpcIterator`

**Files:**
- Modify: `pkg/script/npc_iterator.go` — struct definition at lines 32-64, append new field at the end of the struct.

- [ ] **Step 1: Add the field**

Edit `pkg/script/npc_iterator.go`. Find the end of the `NpcIterator` struct (line 63: `zoneIdx int` followed by `}` at line 64). Insert a new section AFTER `zoneIdx int` and BEFORE the closing `}`:

```go
	// configs is the cache-loaded NpcType/LocType/etc. provider used by
	// passesFilter's HuntAll-mode op[1] gate (TS ScriptIterators.ts:274-280).
	// Nil = test fixture without Configs wired; pessimistic-allow per
	// the lineValidator==nil convention. Production sets this from
	// s.Configs at NewHuntAllNpcIterator. NAI-180.
	configs Configs
```

- [ ] **Step 2: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS (field added but not yet used).

### Task B0.T2 — Add `configs Configs` parameter to `NewHuntAllNpcIterator`

**Files:**
- Modify: `pkg/script/npc_iterator.go:209` — constructor signature + body.

- [ ] **Step 1: Update signature + assign field**

Edit `pkg/script/npc_iterator.go`. Replace the existing function header at line 209 and body at lines 213-230. Find:

```go
// NewHuntAllNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (ACTIVE per
// NAI-35-T3 — partially closes NAI-33-D1 for HuntAll mode; Distance
// mode + FindClosest* still residual) and no typeID filter (-1).
// Mirrors TS NpcHuntAllCommandIterator at ScriptIterators.ts:234-295.
// Bounds math identical to NewDistanceNpcIterator. HuntAll mode is
// distinguished only by passesFilter activating huntvis-based LoS/LoW
// filtering.
func NewHuntAllNpcIterator(lookup NpcLookup, lv LineValidator, tick, level, x, z, distance, huntvis int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:          NpcIteratorHuntAll,
		creationTick:  tick,
		lookup:        lookup,
		lineValidator: lv,
		level:         level,
		x:             x,
		z:             z,
		distance:      distance,
		huntvis:       huntvis,
		typeID:        -1,
		minZoneX:      centerX - radius,
		maxZoneX:      centerX + radius,
		minZoneZ:      centerZ - radius,
		maxZoneZ:      centerZ + radius,
		curZoneX:      centerX + radius,
		curZoneZ:      centerZ + radius,
	}
}
```

Replace with:

```go
// NewHuntAllNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (ACTIVE per
// NAI-35-T3 — partially closes NAI-33-D1 for HuntAll mode; Distance
// mode + FindClosest* still residual) and the NpcType.Op[1] operability
// gate (NAI-180 closes NAI-35-T3-D1; TS ScriptIterators.ts:274-280).
// No typeID filter (-1). Mirrors TS NpcHuntAllCommandIterator at
// ScriptIterators.ts:234-295. Bounds math identical to
// NewDistanceNpcIterator.
//
// configs is the cache-loaded NpcType provider; production passes
// s.Configs. Nil-Configs path is goscape defensive (TS throws on
// missing NpcType) — test fixtures pessimistically allow.
func NewHuntAllNpcIterator(lookup NpcLookup, lv LineValidator, configs Configs, tick, level, x, z, distance, huntvis int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:          NpcIteratorHuntAll,
		creationTick:  tick,
		lookup:        lookup,
		lineValidator: lv,
		configs:       configs,
		level:         level,
		x:             x,
		z:             z,
		distance:      distance,
		huntvis:       huntvis,
		typeID:        -1,
		minZoneX:      centerX - radius,
		maxZoneX:      centerX + radius,
		minZoneZ:      centerZ - radius,
		maxZoneZ:      centerZ + radius,
		curZoneX:      centerX + radius,
		curZoneZ:      centerZ + radius,
	}
}
```

- [ ] **Step 2: Compile check (will FAIL — callers haven't been updated)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: build FAILS at `pkg/script/handlers_npc.go:879-882` and `:924` — `not enough arguments in call to NewHuntAllNpcIterator`.

### Task B0.T3 — Thread `s.Configs` at production call sites

**Files:**
- Modify: `pkg/script/handlers_npc.go:879-882` and `:924`.

- [ ] **Step 1: Update `handleNpcHuntAll` call site**

Edit `pkg/script/handlers_npc.go`. Find at line 879:

```go
	s.npcIterator = NewHuntAllNpcIterator(
		s.Npcs, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis,
	)
```

Replace with:

```go
	s.npcIterator = NewHuntAllNpcIterator(
		s.Npcs, s.LineValidator, s.Configs, s.World.CurrentTick(),
		level, x, z, distance, checkVis,
	)
```

- [ ] **Step 2: Update `handleNpcHunt` call site**

Edit `pkg/script/handlers_npc.go`. Find at line 924:

```go
	it := NewHuntAllNpcIterator(s.Npcs, s.LineValidator, tick, level, x, z, distance, huntvis)
```

Replace with:

```go
	it := NewHuntAllNpcIterator(s.Npcs, s.LineValidator, s.Configs, tick, level, x, z, distance, huntvis)
```

- [ ] **Step 3: Compile check (will FAIL — test fixtures still on old signature)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: build FAILS at `pkg/script/npc_iterator_test.go` (10 call sites).

### Task B0.T4 — Thread `nil` through 10 existing test fixtures

**Files:**
- Modify: `pkg/script/npc_iterator_test.go` — 10 `NewHuntAllNpcIterator` calls at lines 264, 293, 304, 314, 323, 332, 336, 348, 373, 387.

- [ ] **Step 1: Update each call to insert `nil` as the 3rd argument**

Each existing call has shape `NewHuntAllNpcIterator(lookup, lv, tick, ...)` (3rd arg = `tick`). After the change, shape becomes `NewHuntAllNpcIterator(lookup, lv, nil, tick, ...)`.

Use this command to verify the 10 call sites BEFORE editing:

```bash
grep -n "NewHuntAllNpcIterator" pkg/script/npc_iterator_test.go
```

For each line, replace the prefix `NewHuntAllNpcIterator(<lookup>, <lv>, ` with `NewHuntAllNpcIterator(<lookup>, <lv>, nil, ` (preserving the rest of the args). Concrete edits:

Line 264:
```go
	it := NewHuntAllNpcIterator(nil, nil, 99, 0, 3200, 3300, 10, objtype.HuntVisLineOfSight)
```
→
```go
	it := NewHuntAllNpcIterator(nil, nil, nil, 99, 0, 3200, 3300, 10, objtype.HuntVisLineOfSight)
```

Line 293:
```go
	it := NewHuntAllNpcIterator(nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisOff)
```
→
```go
	it := NewHuntAllNpcIterator(nil, nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisOff)
```

Line 304:
```go
	it := NewHuntAllNpcIterator(nil, stub, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
```
→
```go
	it := NewHuntAllNpcIterator(nil, stub, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
```

Line 314:
```go
	it := NewHuntAllNpcIterator(nil, stub, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
```
→
```go
	it := NewHuntAllNpcIterator(nil, stub, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
```

Line 323:
```go
	it := NewHuntAllNpcIterator(nil, stub, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfWalk)
```
→
```go
	it := NewHuntAllNpcIterator(nil, stub, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfWalk)
```

Line 332:
```go
	it := NewHuntAllNpcIterator(nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
```
→
```go
	it := NewHuntAllNpcIterator(nil, nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
```

Line 336:
```go
	it2 := NewHuntAllNpcIterator(nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfWalk)
```
→
```go
	it2 := NewHuntAllNpcIterator(nil, nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfWalk)
```

Line 348:
```go
	it := NewHuntAllNpcIterator(nil, rec, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
```
→
```go
	it := NewHuntAllNpcIterator(nil, rec, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
```

Line 373:
```go
	itLOS := NewHuntAllNpcIterator(nil, stubLOS, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
```
→
```go
	itLOS := NewHuntAllNpcIterator(nil, stubLOS, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
```

Line 387:
```go
	itLOW := NewHuntAllNpcIterator(nil, stubLOW, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
```
→
```go
	itLOW := NewHuntAllNpcIterator(nil, stubLOW, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
```

**IMPORTANT — re-check before commit:** the absolute line numbers may have shifted by ±1-3 since plan-write if earlier tasks edited the file. Run `grep -n "NewHuntAllNpcIterator" pkg/script/npc_iterator_test.go` and verify there are exactly 10 hits, all converted to the 9-arg shape.

- [ ] **Step 2: Compile check + run existing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1`
Expected: PASS (no behavior change; nil Configs = no-op gate).

- [ ] **Step 3: Commit B0**

```bash
git add pkg/script/npc_iterator.go pkg/script/handlers_npc.go pkg/script/npc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): NAI-180 B0 — plumb Configs onto NpcIterator

Adds configs Configs field to NpcIterator + new parameter on
NewHuntAllNpcIterator. Threads s.Configs at the 2 production call
sites (handleNpcHuntAll, handleNpcHunt) and nil at the 10 existing
test fixture call sites. No behavior change yet — passesFilter gate
lands in B1.

Closes memory: spec_followup_tracker_freshness (consumed at audit)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 1 — RED tests + GREEN gate

### Task B1.T1 — Write the 3 failing test pins

**Files:**
- Modify: `pkg/script/npc_iterator_test.go` — append new test functions after the existing HuntAll-mode test block.

- [ ] **Step 1: Locate insertion point**

Run: `grep -n "TestNpcIterator_LineValidatorArgShape" pkg/script/npc_iterator_test.go`

The new tests go AFTER this function ends (its body extends to approximately line 400+). Find the next blank line after the function's closing `}` brace and insert the new tests there.

- [ ] **Step 2: Add the 3 new test pins**

Insert this block at the chosen insertion point:

```go
// --- NAI-180: HuntAll-mode op[1] operability gate ----------------------

// TestPassesFilter_HuntAll_OpEmpty_Rejects pins the TS reject filter at
// ScriptIterators.ts:274-280: NpcType with empty Op[1] is rejected
// regardless of distance/huntvis. NAI-180 closes NAI-35-T3-D1.
func TestPassesFilter_HuntAll_OpEmpty_Rejects(t *testing.T) {
	mc := &mockConfigs{npcs: map[int]*objtype.NpcType{
		42: {Op: []string{"Talk-to", "", "", "", ""}}, // Op[1]="" → reject
	}}
	npc := &mockNpc{typeID: 42, x: 3203, z: 3300, level: 0}
	it := NewHuntAllNpcIterator(nil, nil, mc, 0, 0, 3200, 3300, 5, objtype.HuntVisOff)
	if it.passesFilter(npc) {
		t.Errorf("passesFilter(Op[1]=\"\"): got true, want false (TS reject filter)")
	}
}

// TestPassesFilter_HuntAll_OpNonEmpty_Allows pins acceptance when Op[1]
// is populated. Mirrors a typical attackable NPC. NAI-180.
func TestPassesFilter_HuntAll_OpNonEmpty_Allows(t *testing.T) {
	mc := &mockConfigs{npcs: map[int]*objtype.NpcType{
		42: {Op: []string{"Talk-to", "Attack", "", "", ""}}, // Op[1] populated
	}}
	npc := &mockNpc{typeID: 42, x: 3203, z: 3300, level: 0}
	it := NewHuntAllNpcIterator(nil, nil, mc, 0, 0, 3200, 3300, 5, objtype.HuntVisOff)
	if !it.passesFilter(npc) {
		t.Errorf("passesFilter(Op[1]=\"Attack\"): got false, want true")
	}
}

// TestPassesFilter_HuntAll_NilConfigs_Allows pins the defensive
// pessimistic-allow path for test fixtures lacking Configs. Mirrors the
// lineValidator==nil convention. NAI-180.
func TestPassesFilter_HuntAll_NilConfigs_Allows(t *testing.T) {
	npc := &mockNpc{typeID: 42, x: 3203, z: 3300, level: 0}
	it := NewHuntAllNpcIterator(nil, nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisOff)
	if !it.passesFilter(npc) {
		t.Errorf("passesFilter(nil-Configs): got false, want true (defensive pessimistic-allow)")
	}
}
```

- [ ] **Step 3: Run tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestPassesFilter_HuntAll_Op|TestPassesFilter_HuntAll_NilConfigs' -v`

Expected:
- `TestPassesFilter_HuntAll_OpEmpty_Rejects`: **FAIL** (current `passesFilter` has no op-gate; returns true).
- `TestPassesFilter_HuntAll_OpNonEmpty_Allows`: PASS (incidental — no gate to break it).
- `TestPassesFilter_HuntAll_NilConfigs_Allows`: PASS (incidental — no gate to break it).

This confirms the RED state: exactly one new test fails, and that test pins the missing TS-fidelity behavior.

### Task B1.T2 — Implement the op-gate in `passesFilter`

**Files:**
- Modify: `pkg/script/npc_iterator.go` — `passesFilter` function at lines 85-115.

- [ ] **Step 1: Update `passesFilter` body**

Edit `pkg/script/npc_iterator.go`. Find the current `passesFilter` at lines 85-115:

```go
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS line 329-335
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	if it.mode == NpcIteratorHuntAll {
		// NAI-35-T3-D1 deviation: TS NpcHuntAllCommandIterator
		// (ScriptIterators.ts:274-280) ALSO rejects NPCs whose
		// NpcType.Op[1] is empty (operability gate). Goscape skips this
		// filter pending plumbing Configs onto NpcIterator. Content-script
		// audit will decide port-vs-keep; tracked in nai_followups.md.
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
	}
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}
```

Replace the entire function body (keeping the doc-comment above it) with:

```go
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS line 329-335
	}
	// HuntAll-mode op[1] reject runs BEFORE distance check per TS order
	// at ScriptIterators.ts:274-282. NAI-180 closes NAI-35-T3-D1.
	if it.mode == NpcIteratorHuntAll && it.configs != nil {
		// (goscape defensive; TS throws on missing NpcType) — test
		// fixtures may pass nil Configs and pessimistically allow,
		// matching the lineValidator==nil convention at
		// npcVisibleViaLineOfSight (line 123-128).
		npcType := it.configs.NpcType(npc.NpcType())
		if npcType == nil || len(npcType.Op) <= 1 || npcType.Op[1] == "" {
			return false
		}
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	if it.mode == NpcIteratorHuntAll {
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
	}
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}
```

The functional changes:
1. Removed the multi-line `NAI-35-T3-D1 deviation:` block (the deviation is now closed).
2. Added a NEW `if it.mode == NpcIteratorHuntAll && it.configs != nil` block BEFORE the distance check, implementing the TS-faithful reject.
3. The `switch it.huntvis` block remains where it was (after distance).

- [ ] **Step 2: Run the 3 new tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestPassesFilter_HuntAll_Op|TestPassesFilter_HuntAll_NilConfigs' -v`

Expected: PASS (3 tests).

- [ ] **Step 3: Run the full package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... -count=1`

Expected: PASS. No existing test should regress because all existing test fixtures pass `nil` Configs, hitting the pessimistic-allow path.

- [ ] **Step 4: Verify no other code references the old `NAI-35-T3-D1` tag**

Run: `rg "NAI-35-T3-D1" pkg/ modules/ cmd/`

Expected: 0 hits in production code (the only remaining hit was the doc-comment we just removed). Tracker entries in `nai_followups.md` are NOT in this scope — they get retired in B2.

- [ ] **Step 5: Commit B1**

```bash
git add pkg/script/npc_iterator.go pkg/script/npc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-180 B1 — NpcIterator HuntAll op[1] gate (closes NAI-35-T3-D1)

Ports TS ScriptIterators.ts:274-280 reject filter: HuntAll-mode iterator
now skips NPCs whose NpcType.Op[1] is empty. Op-gate runs BEFORE the
distance check per TS order. Nil-Configs path pessimistically allows
(test-fixture defensive; TS throws on missing NpcType).

Three new pins in npc_iterator_test.go: OpEmpty_Rejects, OpNonEmpty_Allows,
NilConfigs_Allows. No regression in the 10 existing HuntAll-mode tests
(all pass nil Configs → pessimistic-allow path).

Retires NAI-35-T3-D1 deviation in production. Tracker entries cleaned
up at B2.

Closes memory: true_to_ts_gate

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 2 — CLOSE (tracker housekeeping + final close commit)

### Task B2.T1 — Update tracker entries

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — 3 stale entries.

- [ ] **Step 1: Verify current tracker line numbers**

Run: `grep -n "NAI-35-T3-D1" /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

Expected: 3 hits. Line numbers may have shifted from the spec's quoted 2484/2542/2606 by ±5 lines. Use the actual `grep` output as ground truth.

- [ ] **Step 2: Retire the primary deviation listing**

Find the line that begins with `- **NAI-35-T3-D1** (active residual):` and replace with:

```markdown
- ~~**NAI-35-T3-D1**~~ — RETIRED 2026-05-12 by NAI-180. NpcIterator HuntAll mode now ports the TS `npcType.op[1]` operability gate (`ScriptIterators.ts:274-280`). `Configs` plumbed onto `NpcIterator`; production threads `s.Configs` from both `handleNpcHuntAll` and `handleNpcHunt` call sites.
```

- [ ] **Step 3: Retire the 2 carry-forward references**

Both carry-forward lines have shape:

```markdown
N. **NAI-35-T3-D1 audit** — op[1] operability gate; revisit when HUNTALL smoke surfaces a real-content miss.
```

(One has `deferred from NAI-35` suffix, one doesn't.) Use `replace_all=true` for the literal common prefix. Replace with:

```markdown
N. ~~**NAI-35-T3-D1 audit**~~ — RETIRED 2026-05-12 by NAI-180.
```

(Preserve the leading list number `N.` — the actual numbers vary; the Edit tool's match will preserve it because `N.` is part of the replaced string. Two separate `Edit` calls with `replace_all=false` are safer than one `replace_all=true`; use whichever is cleanest.)

- [ ] **Step 4: Verify zero remaining stale references**

Run: `grep -n "NAI-35-T3-D1" /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md | grep -v RETIRED`

Expected: 0 hits.

### Task B2.T2 — Final close commit

- [ ] **Step 1: Verify HEAD state**

Run: `git log --oneline -5`

Expected: top of log shows B1 commit (`feat(script): NAI-180 B1 — ...`) and B0 commit (`refactor(script): NAI-180 B0 — ...`).

Run: `git status`

Expected: clean (or with only the pre-existing untracked dotfiles per `feedback_subagent_wt_path`).

- [ ] **Step 2: Final test run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS across all packages.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 3: Empty roll-up close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-180 — NpcIterator HuntAll op[1] operability gate

Closes the NAI-35-T3-D1 audit-and-port sub-spec. Stage 1 audit (13
content-script consumers surveyed) showed no consumer depends on the
op[1] filter, but the port shipped for TS-fidelity completeness per
true_to_ts_gate.md.

Production diff: ~30 LOC (NpcIterator field + parameter + passesFilter
gate + 2 call-site threads). Tests: 3 new pins + nil-Configs threaded
through 10 existing fixtures.

Net deviation tally: -1 (NAI-35-T3-D1 retired).

Closes memory: NAI-35-T3-D1 audit

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Verify final state**

Run: `git log --oneline -5`

Expected: top 3 commits are NAI-180 B0, B1, and close.

Run: `rg NAI-35-T3-D1 pkg/ modules/ cmd/`

Expected: 0 hits across production + test code.

---

## Self-review checklist (run after writing this plan)

### Spec coverage

- §1 problem statement — covered in plan header `Goal`.
- §2 content audit — already done at spec-write time; plan does not re-audit.
- §3 op-index mapping — encoded in B1.T1 test fixtures (`Op[1]=""` rejects; `Op[1]="Attack"` allows).
- §4.1 struct field addition — B0.T1.
- §4.2 passesFilter HuntAll branch — B1.T2.
- §4.3 constructor signature — B0.T2.
- §4.4 call sites — B0.T3.
- §4.5 test fixture wiring — B0.T4.
- §5.1/5.2/5.3 three new test pins — B1.T1.
- §5.4 no-regression on existing tests — B0.T4 step 2.
- §6 bundle structure — B0/B1/B2.
- §7 risk register — R1 audit-evidence pre-flighted at spec-write; R2 nil-Configs path pinned by B1.T1 `NilConfigs_Allows` test; R3 order divergence avoided (gate BEFORE distance).
- §8 out-of-scope items not addressed by this plan (Distance/ZONE modes).
- §9 closure criteria — verified at B2.T2 final test run.

### Placeholder scan

No "TODO", "implement later", or "fill in details" — all code blocks contain compilable content.

### Type consistency

- `Configs` interface — referenced consistently across B0.T1 (field), B0.T2 (parameter), B0.T3 (`s.Configs`), B1.T2 (`it.configs.NpcType(...)`), B1.T1 (`mockConfigs.npcs` map).
- `*objtype.NpcType` and its `Op []string` field — referenced in B1.T1 and B1.T2.
- `NewHuntAllNpcIterator` signature change — consistent: 3rd parameter is `configs Configs`, after `lv LineValidator` and before `tick int`.

### Plan-author runtime pre-flight reminders (re-grep before dispatch)

- **B0.T4 step 1** — line numbers for the 10 test calls may have shifted between plan-write and execution. Always run `grep -n "NewHuntAllNpcIterator" pkg/script/npc_iterator_test.go` first and verify count = 10.
- **B1.T1 step 1** — insertion point is "after `TestNpcIterator_LineValidatorArgShape`". If a newer test landed between plan-write and execution, append after the latest HuntAll-mode test instead.
- **B2.T1 step 1** — tracker line numbers may have shifted by housekeeping or other writes; trust the live `grep` output.

---

## Final note

This is compressed-cadence — 3 commits expected (B0 + B1 + B2.T2 close). No two-stage reviewer per `compressed_cadence.md`'s 15-100 LOC band guidance; the controller's pre-flight + post-task verification per `controller_preflight.md` + `verify_implementer_claims.md` is sufficient.

If smoke-testing surfaces a behavioral surprise (e.g., a content script unexpectedly relies on the op[1] filter being ABSENT), the close commit's audit evidence + R1 mitigation framing supports reverting B1 cleanly.
