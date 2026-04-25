# NAI-21 Follow-up Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land a 5-item NAI-series follow-up bundle in three logical-grouping bundles: production surgical fixes (LoS-path snapshot promotion + S7c-D1 closure), polish & doc cleanup (NewNpc loop modernize + retire stale NAI-17-D1 tracker), and test infra (NAI-3 weak-form strengthening).

**Architecture:** Three bundles, three commits. Bundle 1 = two production code-path edits with TDD (tests first → implementation → green). Bundle 2 = cosmetic + doc retirement with grep enumeration. Bundle 3 = test-only addition using already-exported `Provider.Register()` API; no new fixture infrastructure needed.

**Tech Stack:** Go 1.26+. Existing packages: `modules/world` (production + tests), `pkg/script` (consumed only via existing exported APIs). No new files, no new packages, no new exported types.

**Spec:** `docs/superpowers/specs/2026-04-25-nai-21-followup-bundle-design.md` (commit `adf500e`).

**Predecessor:** NAI-20 follow-up bundle (HEAD `af2c926` at plan-write time).

**Deviation accounting:** 16 → 16 net. Bundle 1 closes S7c-D1 (-1) and introduces NAI-21-D1 (+1, internal-mechanism only).

**Cadence per bundle:**
- Bundles 1 + 3: Two-stage review (Stage 1 code review by `superpowers:code-reviewer` + Stage 2 TS-fidelity whole-impl review).
- Bundle 2: Single light reviewer pass (doc-only + cosmetic, within `compressed_cadence` ~15-LOC threshold for the bundle as a unit).

**Per-implementer protocol:**
- 30-second grep+Read pre-flight against HEAD before any code edit (per `controller_preflight` memory).
- All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` (per global CLAUDE.md).
- All commits use `git commit --no-gpg-sign` (per global CLAUDE.md).
- Independent fresh `go test ./...` run after each bundle commit; never accept "pre-existing failures" without `HEAD~1` verification (per `verify_implementer_claims` memory).

---

## File structure

**Bundle 1:**
- Modify: `modules/world/npc_interaction.go` (lines 532, 581 — snapshot reads)
- Modify: `modules/world/npc_interaction_test.go` (append 2 tests)
- Modify: `modules/world/appearance.go` (lines 25-28 — sentinel-handling reader)
- Modify: `modules/world/appearance_test.go` (append 3 tests)

**Bundle 2:**
- Modify: `modules/world/npc.go` (line 164 — modernize loop)
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark NAI-17-D1 entry as Resolved)

**Bundle 3:**
- Modify: `modules/world/npc_script_test.go` (replace lines 280-297 weak-form test with strong form)
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark NAI-3 weak-form deferral as Resolved)

**No new files. No production code beyond `npc_interaction.go`, `appearance.go`, and `npc.go`.**

---

# Bundle 1 — Production surgical fixes

## Task 1: Write failing tests for LoS-path snapshot promotion (Task (a))

**Files:**
- Modify: `modules/world/npc_interaction_test.go` (append 2 tests at end of file)

**Pre-flight (implementer must verify before edit):**

```
grep -n "approachEntitySize\|inApproachDistance" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_interaction.go | head -10
```

Expected: confirms current call sites at line 532 (`size := int(t.typ.Size)` in `approachEntitySize`) and line 581 (`selfSize := int(n.typ.Size)` in `inApproachDistance`). If line numbers have shifted, adjust the Task 2 edit accordingly.

```
grep -n "TestSizeMorphRevertRestoresBaseFootprint" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_registry_test.go
```

Expected: test exists at `npc_registry_test.go:181` — reference template for the size-morph setup pattern.

- [ ] **Step 1: Read existing test patterns and HasLineOfSight signature for context**

Read these regions to confirm the test setup pattern:

```
modules/world/npc_registry_test.go:181-213    (size-morph test template)
modules/world/npc_interaction_test.go:863-1090 (existing inApproachDistance test patterns)
```

Determine the exact `gamemap.New()` setup, the LoS scenario configuration, and the `s.gamemap.Pathfinder.Flags.SetFlag(...)` API. The dual-pin approach: configure flags such that `selfSize=2` produces a different LoS result than `selfSize=1` (e.g., a tile flagged with `FlagBlockPlayers` lying within size=2's footprint but outside size=1's).

- [ ] **Step 2: Write `TestInApproachDistanceUsesSelfSizeSnapshotNotTyp`**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestInApproachDistanceUsesSelfSizeSnapshotNotTyp pins NAI-21 Task (a):
// after a size-morph, inApproachDistance must read self size from the
// NAI-20 snapshot (n.size) rather than live config (n.typ.Size). Mirrors
// TS PathingEntity.width ctor-snapshot semantics (PathingEntity.ts:402-405).
//
// Setup: NPC at base size=2; morph to size=1. n.size stays 2 (snapshot);
// n.typ.Size becomes 1 (live). Configure a LoS scenario where srcSize=2
// is blocked but srcSize=1 would pass — assert blocked (snapshot-honoring).
func TestInApproachDistanceUsesSelfSizeSnapshotNotTyp(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	baseTyp := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	morphTyp := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkAll}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: []*objtype.NpcType{nil, baseTyp, morphTyp},
	}

	n := newRegisteredNpc(t, s, baseTyp, true) // first-spawn at size=2; n.size=2
	n.ChangeType(2, 100)                       // morph to size-1 type; n.typ.Size=1, n.size stays 2

	// Sanity-pin the divergence the test depends on.
	if n.size != 2 {
		t.Fatalf("setup: n.size should still be 2 (snapshot), got %d", n.size)
	}
	if n.typ.Size != 1 {
		t.Fatalf("setup: n.typ.Size should be 1 (post-morph), got %d", n.typ.Size)
	}

	// Place a target 3 tiles east at same level.
	target := &mockEntity{x: n.x + 3, z: n.z, level: n.level, size: 1}

	// Configure a flag that intersects size=2's NPC footprint but not size=1's.
	// Implementer pins exact tile + flag bit per HasLineOfSight semantics.
	// The mechanism: HasLineOfSight's srcSize parameter expands the source
	// tile into a srcSize×srcSize square; flags inside that square block.
	// Set FlagBlockPlayers at (n.x+1, n.z) — inside size-2 footprint when
	// the source is anchored SW, but outside size-1 footprint.
	s.gamemap.Pathfinder.Flags.SetFlag(n.x+1, n.z, n.level, collision.FlagBlockPlayers)

	got := n.inApproachDistance(5, target)

	// snapshot=size-2: footprint includes (n.x+1, n.z) which is flagged → BLOCKED.
	// typ=size-1: footprint excludes (n.x+1, n.z) → would pass.
	if got {
		t.Errorf("inApproachDistance: got true, want false — selfSize must read " +
			"from n.size snapshot (=2, blocked by flagged tile), not n.typ.Size " +
			"(=1, would pass)")
	}
}
```

**Note on the LoS scenario** — three approaches in priority order:

**Approach 1 (preferred): real LoS scenario with destination-side flag.**
The LoS call goes from target (src) to self (dest). The dest-size (= selfSize) controls how much area around (n.x, n.z) the validator considers. Try placing `FlagBlockPlayers` at a tile inside the size-2 dest footprint but outside the size-1 footprint (e.g., `(n.x+1, n.z)`). If `HasLineOfSight`'s implementation actually consults the dest footprint, this produces divergent results.

**Approach 2 (fallback if Approach 1 doesn't diverge): swap the validator for an args-recording test double.**
The current LineValidator is a concrete struct. Either (i) make a one-time small refactor to wrap it in an interface (~10 LOC production scope expansion, may need spec amendment review) and inject the test double, or (ii) construct a test gamemap whose LineValidator's underlying flags grid IS empty everywhere but whose return value is rigged to surface the args via some side-channel.

**Approach 3 (last resort if both above fail): reduce the self-side test to a `n.size`-pin sanity assertion.**
Acknowledge that `approachEntitySize` (Task 1 Step 3) IS the production-code site that proves the snapshot-read mechanism, and the self-side `selfSize := int(n.size)` change is a 1-line edit verified by code review + the source's snapshot semantics. The self-side test then becomes: construct n with `n.size=2` after morph; assert `n.size == 2` (snapshot held) and `n.typ.Size == 1` (live diverged). This is weaker (doesn't pin the actual read at line 581) but is testable without touching production. Tag as a remaining follow-up if used.

**Implementer should attempt Approach 1 first.** If `HasLineOfSight` doesn't surface the divergence after a brief investigation (e.g., reading `pkg/pathfinding/linevalidator.go` and confirming whether dest footprint is consulted), escalate to controller for an Approach 2 vs 3 decision. **Do not silently degrade to Approach 3 without surfacing the change.**

- [ ] **Step 3: Write `TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp`**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp pins NAI-21 Task (a)
// target side: after a size-morph, approachEntitySize must read target
// size from the NAI-20 snapshot (t.size) rather than live config
// (t.typ.Size). Mirrors TS PathingEntity.width ctor-snapshot semantics.
//
// Setup: target NPC at base size=2; morph to size=1. t.size stays 2;
// t.typ.Size becomes 1. Assert approachEntitySize returns (2, 2).
func TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp(t *testing.T) {
	s := newTestServer(t)

	baseTyp := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	morphTyp := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkAll}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: []*objtype.NpcType{nil, baseTyp, morphTyp},
	}

	target := newRegisteredNpc(t, s, baseTyp, true) // size=2, n.size=2
	target.ChangeType(2, 100)                       // morph to size-1; n.typ.Size=1, n.size stays 2

	// Sanity-pin the divergence.
	if target.size != 2 {
		t.Fatalf("setup: target.size should still be 2 (snapshot), got %d", target.size)
	}
	if target.typ.Size != 1 {
		t.Fatalf("setup: target.typ.Size should be 1 (post-morph), got %d", target.typ.Size)
	}

	w, l := approachEntitySize(target)

	if w != 2 || l != 2 {
		t.Errorf("approachEntitySize: got (%d, %d), want (2, 2) — must read " +
			"t.size snapshot, not t.typ.Size live", w, l)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail at HEAD (production still reads typ.Size)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestInApproachDistanceUsesSelfSizeSnapshotNotTyp ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp ./modules/world/...
```

Expected: BOTH tests FAIL — production reads `t.typ.Size`/`n.typ.Size` (=1 post-morph) so `inApproachDistance` returns true (LoS passes for size=1) and `approachEntitySize` returns (1, 1).

If tests pass at this stage, the test setup is wrong (likely the LoS scenario doesn't actually distinguish size=1 vs size=2 in the chosen flag configuration). Fix the test setup before proceeding to Task 2.

## Task 2: Implement LoS-path snapshot promotion

**Files:**
- Modify: `modules/world/npc_interaction.go:532` (1 line)
- Modify: `modules/world/npc_interaction.go:581` (1 line)

**Pre-flight (implementer must verify before edit):**

```
grep -n "n\.size\|n\.blockWalk" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc.go | head -10
```

Expected: confirms `n.size` and `n.blockWalk` are present as `*Npc` fields and seeded at `NewNpc` (NAI-20 Task 2, commit `df71250`). If absent, NAI-20 has been reverted and this task is blocked.

- [ ] **Step 1: Edit `approachEntitySize` to read snapshot**

In `modules/world/npc_interaction.go`, find the `*Npc` branch in `approachEntitySize` (around line 531-533):

Before:
```go
case *Npc:
	size := int(t.typ.Size)
	return size, size
```

After:
```go
case *Npc:
	size := int(t.size)
	return size, size
```

- [ ] **Step 2: Edit `inApproachDistance` to read snapshot**

In `modules/world/npc_interaction.go`, find the `selfSize` line in `inApproachDistance` (around line 581):

Before:
```go
selfSize := int(n.typ.Size)
```

After:
```go
selfSize := int(n.size)
```

- [ ] **Step 3: Run new tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestInApproachDistanceUsesSelfSizeSnapshotNotTyp ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp ./modules/world/...
```

Expected: BOTH tests PASS.

- [ ] **Step 4: Run full `npc_interaction_test.go` suite to verify no regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestApproach -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestInApproach -v
```

Expected: ALL tests in those families PASS. If any pre-existing test fails, verify at `HEAD~1` to confirm the failure is not new (per `verify_implementer_claims` memory).

## Task 3: Write failing tests for appearanceInv reader fix (Task (d))

**Files:**
- Modify: `modules/world/appearance_test.go` (append 3 tests at end of file)

**Pre-flight (implementer must verify before edit):**

```
grep -n "p\.invs\[invs\.Worn\]\|appearanceInv" /home/owner/Code/github.com/zsrv/goscape/modules/world/appearance.go
```

Expected: confirms the current reader reads `p.invs[invs.Worn]` at line 27.

```
grep -n "func synthesizeTypes\|func newTestPlayer" /home/owner/Code/github.com/zsrv/goscape/modules/world/*_test.go
```

Expected: confirms `synthesizeTypes` is at `appearance_test.go:10` and `newTestPlayer` is at `player_test.go:14`. Used by all 3 new tests.

- [ ] **Step 1: Read existing test patterns**

Read `modules/world/appearance_test.go:1-77` to confirm the `synthesizeTypes` shape (only one inv-id "worn" exists by default; the customInvId test will need to extend the synthesized invs slice).

- [ ] **Step 2: Write `TestGenerateAppearanceSentinelDefaultReadsWorn`**

Append to `modules/world/appearance_test.go`:

```go
// TestGenerateAppearanceSentinelDefaultReadsWorn pins NAI-21 Task (d) /
// NAI-21-D1: when p.appearanceInv == -1 (the default sentinel from
// newPlayer), the reader must fall back to invs.Worn — preserving
// pre-fix behavior for production callers that haven't yet invoked
// SetAppearanceInv (initial login, fresh players).
func TestGenerateAppearanceSentinelDefaultReadsWorn(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	if p.appearanceInv != -1 {
		t.Fatalf("setup: p.appearanceInv should default to -1, got %d", p.appearanceInv)
	}
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Equip a platebody at slot 4 (the platebody synthesized in synthesizeTypes).
	p.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}

	p.generateAppearance(objs, invs, 0)

	// The platebody (id=1) at slot 4 must surface in the appearance buffer.
	// Buffer layout: byte 0 = gender, byte 1 = headicons, then 12 slot bytes.
	// Equipped items use 2-byte form: 0x0200 | (id & 0x1FF). Slot 4 starts at
	// offset 2 + (sum of bytes for slots 0..3). For this test we only assert
	// the buffer is non-trivial and contains the platebody's encoded id.
	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty (sentinel must fall back to Worn)")
	}
	wantSlot4Hi := byte((0x200 | (1 & 0x1FF)) >> 8)
	wantSlot4Lo := byte((0x200 | (1 & 0x1FF)) & 0xFF)
	found := false
	for i := 0; i < len(p.appearanceBuf)-1; i++ {
		if p.appearanceBuf[i] == wantSlot4Hi && p.appearanceBuf[i+1] == wantSlot4Lo {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("appearanceBuf missing platebody encoded bytes (0x%02x 0x%02x); " +
			"sentinel mapping to Worn appears broken", wantSlot4Hi, wantSlot4Lo)
	}
}
```

- [ ] **Step 3: Write `TestGenerateAppearanceExplicitWornIdMatchesSentinel`**

Append to `modules/world/appearance_test.go`:

```go
// TestGenerateAppearanceExplicitWornIdMatchesSentinel pins NAI-21 Task (d):
// explicit p.appearanceInv = invs.Worn produces byte-identical output
// to the sentinel-default path. Proves the new explicit-set codepath
// matches the sentinel-mapped codepath for the common case.
func TestGenerateAppearanceExplicitWornIdMatchesSentinel(t *testing.T) {
	objs, invs := synthesizeTypes(t)

	// Player A: sentinel default (-1).
	pA, _ := newTestPlayer(t)
	pA.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	pA.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}
	pA.generateAppearance(objs, invs, 0)

	// Player B: explicit appearanceInv = invs.Worn.
	pB, _ := newTestPlayer(t)
	pB.appearanceInv = invs.Worn
	pB.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	pB.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}
	pB.generateAppearance(objs, invs, 0)

	if len(pA.appearanceBuf) != len(pB.appearanceBuf) {
		t.Fatalf("buffer length mismatch: sentinel=%d explicit=%d",
			len(pA.appearanceBuf), len(pB.appearanceBuf))
	}
	for i := range pA.appearanceBuf {
		if pA.appearanceBuf[i] != pB.appearanceBuf[i] {
			t.Errorf("byte %d differs: sentinel=0x%02x explicit=0x%02x",
				i, pA.appearanceBuf[i], pB.appearanceBuf[i])
		}
	}
}
```

- [ ] **Step 4: Write `TestGenerateAppearanceCustomInvIdHonored`**

Append to `modules/world/appearance_test.go`:

```go
// TestGenerateAppearanceCustomInvIdHonored pins NAI-21 Task (d) S7c-D1
// closure: when p.appearanceInv is set to a non-Worn inv id, the reader
// must read FROM that inv (not from invs.Worn). This is the actual S7c-D1
// bug-fix proof — the pre-fix reader at appearance.go:27 read p.invs[invs.Worn]
// regardless of p.appearanceInv, so custom-outfit scripts had no effect.
func TestGenerateAppearanceCustomInvIdHonored(t *testing.T) {
	objs, invs := synthesizeTypes(t)

	// Extend the synthesized invs to add a "custom" inv at id=1 (Worn is id=0).
	customInvId := 1
	invs.Configs = append(invs.Configs, &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: customInvId, DebugName: "custom"},
		Size:       14,
	})

	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn:    inventory.FromType(invs.Configs[invs.Worn]),    // empty
		customInvId: inventory.FromType(invs.Configs[customInvId]),
	}
	// Worn is empty; custom has a platebody at slot 4.
	p.invs[customInvId].Items[4] = &inventory.Item{Id: 1, Count: 1}
	p.appearanceInv = customInvId

	p.generateAppearance(objs, invs, 0)

	// The platebody must surface in the appearance buffer because the
	// reader read from p.invs[customInvId], NOT from p.invs[invs.Worn].
	wantSlot4Hi := byte((0x200 | (1 & 0x1FF)) >> 8)
	wantSlot4Lo := byte((0x200 | (1 & 0x1FF)) & 0xFF)
	found := false
	for i := 0; i < len(p.appearanceBuf)-1; i++ {
		if p.appearanceBuf[i] == wantSlot4Hi && p.appearanceBuf[i+1] == wantSlot4Lo {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("appearanceBuf missing platebody from custom inv; reader is " +
			"still reading from invs.Worn (S7c-D1 NOT closed)")
	}
}
```

- [ ] **Step 5: Run new tests to verify they fail at HEAD**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestGenerateAppearanceSentinelDefaultReadsWorn ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestGenerateAppearanceExplicitWornIdMatchesSentinel ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestGenerateAppearanceCustomInvIdHonored ./modules/world/...
```

Expected:
- Test 1 (`SentinelDefaultReadsWorn`): PASSES at HEAD (current reader reads invs.Worn unconditionally; sentinel doesn't matter to it).
- Test 2 (`ExplicitWornIdMatchesSentinel`): PASSES at HEAD (current reader ignores appearanceInv, so explicit and sentinel produce identical buffers).
- Test 3 (`CustomInvIdHonored`): **FAILS at HEAD** — current reader reads from empty `invs.Worn` so the platebody is not in the buffer.

This is the asymmetric "HEAD pass" pattern: Tests 1+2 are forward-compatibility / regression-equivalence tests that pass under both pre-fix and post-fix code. Test 3 is the actual bug-fix proof that only passes after the fix lands.

## Task 4: Implement appearanceInv reader fix

**Files:**
- Modify: `modules/world/appearance.go:25-28` (5 lines added incl. comment)

**Pre-flight (implementer must verify before edit):**

```
grep -n "p\.invs\[invs\.Worn\]" /home/owner/Code/github.com/zsrv/goscape/modules/world/appearance.go
```

Expected: confirms only one site reads `p.invs[invs.Worn]` (at line 27). If multiple sites exist, the spec needs an update.

- [ ] **Step 1: Edit reader to honor appearanceInv with sentinel mapping**

In `modules/world/appearance.go`, find the worn-inv read block (lines 25-28):

Before:
```go
	var worn *inventory.Inventory
	if p.invs != nil {
		worn = p.invs[invs.Worn]
	}
```

After:
```go
	// NAI-21-D1: TS init binds appearanceInv to Worn at ctor; goscape uses
	// -1 sentinel and maps it here for behavioral parity. Internal mechanism
	// only — observationally identical for production callers because every
	// production caller either (i) passes through SetAppearanceInv before
	// generateAppearance fires, or (ii) is a fresh player whose first read
	// must surface worn-inv items.
	var worn *inventory.Inventory
	if p.invs != nil {
		inventoryId := p.appearanceInv
		if inventoryId < 0 {
			inventoryId = invs.Worn
		}
		worn = p.invs[inventoryId]
	}
```

- [ ] **Step 2: Run new tests to verify all three pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestGenerateAppearanceSentinelDefaultReadsWorn ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestGenerateAppearanceExplicitWornIdMatchesSentinel ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestGenerateAppearanceCustomInvIdHonored ./modules/world/...
```

Expected: ALL THREE pass.

- [ ] **Step 3: Run full appearance test suite to verify no regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestGenerateAppearance ./modules/world/... -v
```

Expected: ALL tests in `TestGenerateAppearance*` pass, including the pre-existing `TestGenerateAppearanceNakedPlayer` and `TestGenerateAppearancePlatebodyEquipped` which both rely on the sentinel-default behavior.

## Task 5: Run full test suite and commit Bundle 1

- [ ] **Step 1: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: ALL tests pass. If any test fails, verify at `HEAD~1` per `verify_implementer_claims` memory before claiming "pre-existing failure."

- [ ] **Step 2: Run with race detector**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: ALL tests pass. No data races.

- [ ] **Step 3: Stage Bundle 1 files**

```
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go \
        modules/world/appearance.go modules/world/appearance_test.go
git status
```

Expected `git status` shows exactly those four files staged, nothing else.

- [ ] **Step 4: Commit Bundle 1**

```
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-21 Bundle 1 — LoS snapshot LoS-path completion + S7c-D1 closure

Task (a) — LoS-path snapshot promotion (closes NAI-20 deferred follow-up):
- npc_interaction.go:532 — approachEntitySize *Npc branch reads t.size
- npc_interaction.go:581 — inApproachDistance reads n.size for selfSize
- 2 tests: dual-pin (snapshot-honoring AND typ-following-absence) per
  ts_asymmetry_dual_pin memory.

Task (d) — S7c-D1 closure (appearanceInv reader fix):
- appearance.go:25-28 — reader reads p.invs[p.appearanceInv] with
  sentinel-handling for -1 (maps to invs.Worn). Mirrors TS Player.ts:1318
  this.getInventory(this.appearanceInv).
- Sentinel handling tagged NAI-21-D1: TS ctor binds appearanceInv to
  Worn; goscape uses -1 sentinel + reader-side fallback for behavioral
  parity (internal mechanism only, no script-visible difference).
- 3 tests: regression equivalence + explicit-Worn parity + custom-inv
  bug-fix proof.

Deviation accounting: -1 (S7c-D1 closes) +1 (NAI-21-D1 introduced) = 0 net.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: Verify commit landed**

```
git log -1 --stat
```

Expected: shows the new commit with the four files modified, expected line counts (~+90 / -3 across the two modified prod files + two test files).

---

# Bundle 2 — Polish & doc cleanup

## Task 6: Modernize NewNpc stats-seeding loop

**Files:**
- Modify: `modules/world/npc.go:164` (2 lines changed)

**Pre-flight (implementer must verify before edit):**

```
grep -n "for i := 0; i < objtype.NpcStatCount" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc.go
```

Expected: confirms the C-style loop at line 164.

```
grep -n "for i := range min" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc.go /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_masks.go /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_script.go
```

Expected: confirms three sibling `for i := range min(...)` loops at the sites named in the spec (revertType heavy-path reseed at npc.go:288, resetStatsForType at npc_masks.go:98, processNpcRegen at npc_script.go:244). If absent, the spec premise about "modernization siblings" is wrong and the cosmetic justification doesn't apply.

- [ ] **Step 1: Edit NewNpc loop to modern Go idiom**

In `modules/world/npc.go`, find the stats-seeding loop in `NewNpc` (around line 164):

Before:
```go
	for i := 0; i < objtype.NpcStatCount && i < len(typ.Stats); i++ {
		n.NpcStat[i] = typ.Stats[i]
	}
```

After:
```go
	for i := range min(objtype.NpcStatCount, len(typ.Stats)) {
		n.NpcStat[i] = typ.Stats[i]
	}
```

- [ ] **Step 2: Run NewNpc-touching test suite to verify no regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNpc ./modules/world/...
```

Expected: ALL tests pass. Behaviorally identical to the C-style loop (both iterate `min(NpcStatCount, len(typ.Stats))` indices).

## Task 7: Retire stale NAI-17-D1 follow-up tracker

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark NAI-17-D1 entry as Resolved, preserve historical body)

**Pre-flight (implementer must run grep enumeration FIRST per `retire_deviation_grep_all_comments` memory):**

```
grep -rn "NAI-17-D1" /home/owner/Code/github.com/zsrv/goscape/pkg \
                     /home/owner/Code/github.com/zsrv/goscape/modules \
                     /home/owner/Code/github.com/zsrv/goscape/cmd \
                     /home/owner/Code/github.com/zsrv/goscape/docs
```

Expected (per spec pre-flight at HEAD `af2c926`): zero production-code references; only `nai_followups.md` has the entry (the entry being retired) and possibly the spec doc itself. If additional sites surface, they MUST be enumerated and updated as part of this task — do not proceed past Step 1 until accounted for.

- [ ] **Step 1: Verify grep enumeration result**

Run the grep above. Document the result (expected: 0 hits in pkg/modules/cmd; some hits in docs/superpowers/specs/ for the NAI-21 spec itself, which is expected and does not need editing). If any pkg/modules/cmd hit appears, surface the issue before continuing — the spec didn't anticipate it and may need amendment.

- [ ] **Step 2: Edit the NAI-17-D1 follow-up entry to mark Resolved**

In `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`, find the section header at line 676:

```
### NAI-17-D1 closure track: revertType despawn+respawn alignment
```

Insert after this line a new "Resolved" marker line, then preserve the original body. Use the established NAI-20 resolution-pattern format (see e.g. `nai_followups.md:31, 698, 973` for example formatting):

```markdown
### NAI-17-D1 closure track: revertType despawn+respawn alignment

**Resolved 2026-04-25 (NAI-21 Bundle 2; superseded by NAI-19's structural `removeNpc+addNpc` port at `npc.go:285-286`; remaining deviation surface tracked as NAI-19-D1 (no zone state, `npc_registry.go:63, 136`) and NAI-19-D2 (no AI_SPAWN re-trigger, `npc_registry.go:77`)).**

---

_Original deferral body (preserved for historical context):_

NAI-17 explicitly annotated (at `modules/world/npc.go:276`) that Go's
... (rest of original body preserved verbatim)
```

The existing body text from line 678 to end of section (line 694) is preserved verbatim under the new heading. Do not delete the original text.

- [ ] **Step 3: Verify the edit landed cleanly**

```
sed -n '676,710p' /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
```

Expected: shows the new "Resolved" marker line followed by the preserved historical body.

## Task 8: Run full test suite and commit Bundle 2

- [ ] **Step 1: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: ALL tests pass. (Bundle 2 has no behavioral changes; this is regression-only.)

- [ ] **Step 2: Stage Bundle 2 files**

```
git add modules/world/npc.go
git status
```

Expected: shows `npc.go` staged. **`nai_followups.md` is OUTSIDE the goscape repo** (it lives in `/home/owner/.claude/projects/.../memory/`), so it does NOT get staged into goscape's git. The memory file lives in Claude's persistent memory store; the edit at Task 7 is recorded by the Edit tool itself.

- [ ] **Step 3: Commit Bundle 2**

```
git commit --no-gpg-sign -m "$(cat <<'EOF'
polish(world): NAI-21 Bundle 2 — modernize NewNpc stats loop + retire NAI-17-D1 tracker

- npc.go:164 — modernize C-style loop to `for i := range min(...)` matching
  three sibling loops modernized in NAI-17's polish passes.
  Behaviorally identical; pure style consistency.

- nai_followups.md NAI-17-D1 entry marked Resolved (memory-side edit).
  The underlying gap (revertType inline reset vs structural despawn+respawn)
  was closed by NAI-19's structural removeNpc+addNpc port at npc.go:285-286.
  Remaining deviation surface tracked separately as NAI-19-D1 (zone state)
  and NAI-19-D2 (AI_SPAWN re-trigger).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Verify commit landed**

```
git log -1 --stat
```

Expected: shows the new commit with `modules/world/npc.go` modified, ~2 lines changed.

---

# Bundle 3 — NAI-3 weak-form NPC queue test strengthening

## Task 9: Replace weak-form NAI-3 test with strong form

**Files:**
- Modify: `modules/world/npc_script_test.go:280-297` (replace weak-form test with strong form)

**Pre-flight (implementer must verify before edit):**

```
grep -n "TestNpcTurnReentryQueueAppendDuringIteration\|buildNpcForIntegration\|newServerForScriptTest" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_script_test.go
```

Expected: confirms the weak-form test at line 284, `buildNpcForIntegration` at line 228, and `newServerForScriptTest` at line 54.

```
grep -n "func handleNpcQueue\|func handleNpcSetTimer" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_npc.go
```

Expected: confirms `handleNpcQueue` and `handleNpcSetTimer` handlers exist with the operand encoding documented in the spec (`OpNpcQueue` pops delay/arg/queueID; `OpNpcSetTimer` pops interval).

```
grep -n "scriptProvider:" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_script.go
```

Expected: confirms `processNpcQueue` calls `s.scriptProvider.GetByTrigger(...)` at line 286 with the `if s.scriptProvider == nil { continue }` short-circuit at line 283.

- [ ] **Step 1: Read existing weak-form test and surrounding helpers**

Read `modules/world/npc_script_test.go:226-300` to confirm `buildNpcForIntegration`'s output (a server with `scriptProvider == nil`). The strong-form test will need to seed `s.scriptProvider = script.NewProvider()` after calling it.

Read `pkg/script/handlers_npc_test.go:725-770` for the `OpNpcQueue` bytecode pattern template.

- [ ] **Step 2: Replace weak-form test with strong form**

In `modules/world/npc_script_test.go`, find and replace the weak-form test at lines 280-297:

Before (delete lines 280-297):
```go
// TestNpcTurnReentryQueueAppendDuringIteration — multiple ready
// entries (delay=0) fire in one processNpcQueue pass.
// Weaker form of the "speedup quirk" test — doesn't prove mid-fire
// append, only multi-entry same-pass drain.
func TestNpcTurnReentryQueueAppendDuringIteration(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Two entries, both ready (delay=0). The iteration should
	// process both in one turn() call.
	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 0, 0)
	n.EnqueueScriptForTrigger(script.TriggerAiQueue2, 0, 0)

	n.turn(s)

	if len(n.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (both entries should fire in one pass)", len(n.queue))
	}
}
```

After (insert in same location):
```go
// TestNpcTurnReentryQueueAppendDuringIteration — strong form: a
// script fired mid-iteration of processNpcQueue can append a new
// entry that is visible to the same iteration. Mirrors TS Npc.ts:538-560
// "speedup quirk" semantics.
//
// Setup: register an "amplifier" script for TriggerAiQueue1 whose
// bytecode (i) calls OpNpcQueue to enqueue a TriggerAiQueue2 entry,
// and (ii) calls OpNpcSetTimer with interval=42 as an observable
// side-effect proving the amplifier actually executed (distinguishes
// from a silent dispatch failure).
//
// Pre-enqueue ONE entry: TriggerAiQueue1 (delay=0). Call turn(). Assert:
//   - len(n.queue) == 0 — proves both the original AND the amplifier-
//     appended TriggerAiQueue2 entry drained in the same pass.
//   - n.timerInterval == 42 — proves the amplifier actually ran.
//
// Failure modes covered:
//   A: processNpcQueue switches to snapshot-len iteration → queue len = 1 after turn.
//   B: amplifier silently no-ops (dispatch wired wrong) → timerInterval unchanged.
func TestNpcTurnReentryQueueAppendDuringIteration(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// buildNpcForIntegration returns a server with scriptProvider == nil
	// (newServerForScriptTest only sets `log`). Seed an empty provider so
	// processNpcQueue can dispatch.
	s.scriptProvider = script.NewProvider()

	// Amplifier: bytecode = OpNpcQueue(TriggerAiQueue2, 0, 0) + OpNpcSetTimer(42) + OpReturn.
	// Pop order for OpNpcQueue: delay (top), arg, queueID (bottom).
	// Bytecode push order: queueID, arg, delay (matching handlers_npc_test.go:734).
	// queueID=2 maps to TriggerAiQueue2 via TriggerAiQueue1 + queueID - 1.
	amplifier := &script.ScriptFile{
		Name:      "nai21_amplifier_aiqueue1",
		LookupKey: uint32(script.TriggerAiQueue1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt, // push queueID (2 → TriggerAiQueue2)
			script.OpPushConstantInt, // push arg (0)
			script.OpPushConstantInt, // push delay (0)
			script.OpNpcQueue,
			script.OpPushConstantInt, // push interval (42)
			script.OpNpcSetTimer,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0, 0, 42, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	s.scriptProvider.Register(amplifier)

	// Pre-flight wiring guard: ensure the lookup actually resolves to the amplifier.
	// Without this, a wrong LookupKey computation would silently fall through to
	// nil-script handling and the queue would still drain (Bundle 3 spec § failure
	// modes), masking the wiring bug.
	if got := s.scriptProvider.GetByTrigger(script.TriggerAiQueue1, n.typeId, n.typ.Category); got != amplifier {
		t.Fatalf("setup: GetByTrigger(TriggerAiQueue1, ...) = %v, want amplifier", got)
	}

	// Pre-enqueue ONE entry. Amplifier will append the second mid-iteration.
	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 0, 0)
	if len(n.queue) != 1 {
		t.Fatalf("setup: queue should have 1 entry, got %d", len(n.queue))
	}

	n.turn(s)

	// Assertion 1: queue fully drained — proves mid-iteration append visible.
	if len(n.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 — amplifier-appended TriggerAiQueue2 " +
			"entry did not drain in same pass (regression to snapshot-len iteration?)",
			len(n.queue))
	}

	// Assertion 2: amplifier side-effect fired — proves amplifier actually ran
	// (not silent dispatch failure).
	if n.timerInterval != 42 {
		t.Errorf("n.timerInterval: got %d, want 42 — amplifier did not execute " +
			"(scriptProvider lookup or runNpcScript may be silently no-op'ing)",
			n.timerInterval)
	}
}
```

**Note on operand-count count `IntOperands: []int32{2, 0, 0, 0, 42, 0, 0}`:** mirrors the `handlers_npc_test.go:734` pattern of one IntOperand slot per opcode position. The 7 entries correspond to: 3 push slots (queueID, arg, delay) + OpNpcQueue (no operand) + 1 push slot (interval) + OpNpcSetTimer (no operand) + OpReturn (no operand). If `Execute()` reports operand-mismatch errors, the implementer adjusts the slot count to match the dispatcher's actual indexing.

**Note on `StringOperands` and `InstructionCount`:** `defaultTestProvider`'s example at `server_test.go:292-303` populates these. Plan-time inclusion mirrors that pattern; if `Execute()` is tolerant of nil StringOperands (likely), they can be omitted to simplify.

- [ ] **Step 3: Run the new strong-form test**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNpcTurnReentryQueueAppendDuringIteration ./modules/world/... -v
```

Expected: PASSES. Both assertions green.

If the test fails:
- **Failure pattern A** ("queue len: got 1, want 0"): the amplifier ran (timer was set) but the appended entry didn't drain. Investigate `processNpcQueue` — likely a regression in the snapshot-len iteration discipline.
- **Failure pattern B** ("n.timerInterval: got 0, want 42"): the amplifier didn't run. Likely the wiring guard would have caught this (it asserts the lookup resolves), so if the guard passed but the test failed at this assertion, it's an Execute-side issue (operand encoding wrong, ScriptState init wrong, etc.).
- **Operand-count error during Execute**: adjust `IntOperands` length per the dispatcher's actual indexing. Refer to `pkg/script/handlers_npc_test.go:734` for the precedent shape.

- [ ] **Step 4: Run full npc-script test suite to verify no regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNpc ./modules/world/... -v
```

Expected: ALL `TestNpc*` tests pass.

## Task 10: Retire NAI-3 weak-form deferral

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark NAI-3 weak-form deferral as Resolved)

**Pre-flight (implementer must run grep enumeration per `retire_deviation_grep_all_comments` memory):**

```
grep -rn "weak-form\|TestNpcTurnReentryQueueAppendDuringIteration" /home/owner/Code/github.com/zsrv/goscape/pkg \
                                                                   /home/owner/Code/github.com/zsrv/goscape/modules \
                                                                   /home/owner/Code/github.com/zsrv/goscape/cmd \
                                                                   /home/owner/Code/github.com/zsrv/goscape/docs
```

Expected: only the test name itself (now in its strong form post-Task 9), the spec doc, and possibly Plan doc references. No production-code references to "weak-form."

- [ ] **Step 1: Edit the NAI-3 weak-form deferral entry**

In `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`, find the section header at line 90:

```
### Fidelity audit (unassigned): strengthen NPC queue "speedup quirk" test
```

Insert after this line a Resolved marker, then preserve the original body:

```markdown
### Fidelity audit (unassigned): strengthen NPC queue "speedup quirk" test

**Resolved 2026-04-25 (NAI-21 Bundle 3).** Strong-form test landed at
`modules/world/npc_script_test.go` `TestNpcTurnReentryQueueAppendDuringIteration`.
Pre-flight finding at HEAD `af2c926`: the original deferral's "neither fixture
exists today" claim was fully stale — `Provider.Register()` is exported
(`pkg/script/provider.go:182`, docstring explicitly mentions test usage),
`*ScriptFile` is exported with public `LookupKey` field, and the
`buildNpcForIntegration(t)` helper just needs a one-line `s.scriptProvider =
script.NewProvider()` seed before the test registers scripts. No new fixture
infrastructure (RegisterForTest method, test-only opcode, scripttest
subpackage) was required.

The strong-form test uses an "amplifier" script registered for TriggerAiQueue1
whose bytecode (i) calls OpNpcQueue to append a TriggerAiQueue2 entry mid-fire,
and (ii) calls OpNpcSetTimer(42) as an observable side-effect distinguishing
"amplifier ran" from "silent dispatch failure." Two assertions: queue fully
drained (proves mid-iteration append visible) AND timer set (proves amplifier
executed).

---

_Original deferral body (preserved for historical context):_

NAI-3's `TestNpcTurnReentryQueueAppendDuringIteration` ships in the
... (rest of original body preserved verbatim)
```

The existing body text from line 92 to end of section (line 115) is preserved verbatim under the new heading.

- [ ] **Step 2: Verify the edit landed cleanly**

```
sed -n '88,130p' /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
```

Expected: shows the new Resolved marker followed by the preserved historical body.

## Task 11: Run full test suite and commit Bundle 3

- [ ] **Step 1: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: ALL tests pass.

- [ ] **Step 2: Run with race detector**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: ALL tests pass. No data races.

- [ ] **Step 3: Stage Bundle 3 files**

```
git add modules/world/npc_script_test.go
git status
```

Expected: shows `npc_script_test.go` staged. (Memory edit lives outside goscape's git, same as Bundle 2.)

- [ ] **Step 4: Commit Bundle 3**

```
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-21 Bundle 3 — strengthen NAI-3 weak-form NPC queue test + retire deferral

Replace weak-form TestNpcTurnReentryQueueAppendDuringIteration with strong
form proving mid-iteration append. Mirrors TS Npc.ts:538-560 "speedup quirk"
semantics: a script fired mid-pass of processNpcQueue can append a new
entry that's visible to the same iteration.

Test design: register an "amplifier" *script.ScriptFile for TriggerAiQueue1
whose bytecode calls OpNpcQueue (appends TriggerAiQueue2 entry) and
OpNpcSetTimer(42) as an observable side-effect. Pre-enqueue 1 entry, call
turn(), assert (i) queue fully drained AND (ii) timer set. The dual
assertion catches both regression patterns: snapshot-len iteration (queue
not drained) and silent dispatch failure (timer unchanged).

Pre-flight finding: original NAI-3 deferral claimed "neither fixture exists
today" but Provider.Register() and *ScriptFile have been exported all along.
No new fixture infrastructure required; one-line `s.scriptProvider =
script.NewProvider()` seed in the test is sufficient.

NAI-3 weak-form deferral marked Resolved (memory-side edit).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: Verify commit landed**

```
git log -1 --stat
```

Expected: shows the new commit with `modules/world/npc_script_test.go` modified, ~+60/-18 lines.

---

# NAI-21 close commit (after all three bundles land and review approves)

## Task 12: NAI-21 close commit

After all three bundles have landed AND both stages of review have approved, write a `Closes memory:` trailer commit per `close_commit_memory_trailer` memory. This commit is empty-content (no code change) but enumerates the memory entries this sub-spec validates or invalidates.

- [ ] **Step 1: Prepare the memory trailer**

Enumerate memory entries this sub-spec touched (final list pinned at close-commit time, but candidates from spec):

- `controller_preflight.md` — caught NAI-17-D1 stale premise + NAI-3 stale fixture-claim + spec-write factual errors (scriptProvider wiring, marker-script rationale).
- `retire_deviation_grep_all_comments.md` — Bundle 2 + Bundle 3 explicitly invoked the grep enumeration step.
- `ts_asymmetry_dual_pin.md` — Bundle 1 Task (a) tests follow the dual-pin pattern.
- `dead_api_polish.md` — Bundle 1 Task (d) closure of the helper-without-consumer (SetAppearanceInv writing appearanceInv that the reader ignored).
- `compressed_cadence.md` — Bundle 2's light-review divergence is justified by the compressed-cadence threshold for the bundle as a unit.

- [ ] **Step 2: Create empty close commit with memory trailer**

```
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world): NAI-21 closed — three-bundle follow-up

Bundles landed:
  Bundle 1 (feat): LoS-path snapshot promotion + S7c-D1 closure
  Bundle 2 (polish): NewNpc loop modernize + retire NAI-17-D1 tracker
  Bundle 3 (test): NAI-3 weak-form NPC queue test strengthening

Deviation accounting: 16 → 16 net (S7c-D1 closes; NAI-21-D1 introduced
for appearanceInv sentinel-handling, internal-mechanism only).

Closes memory:
  controller_preflight.md
  retire_deviation_grep_all_comments.md
  ts_asymmetry_dual_pin.md
  dead_api_polish.md
  compressed_cadence.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Verify NAI-21 close**

```
git log --oneline -5
```

Expected: shows the close commit, the three bundle commits, and the prior NAI-20 close at `af2c926`.

---

# Spec coverage check

Tracing each spec section to a task:

| Spec section | Task |
|---|---|
| Bundle 1 Task (a) snapshot promotion | Tasks 1 + 2 |
| Bundle 1 Task (d) appearanceInv reader | Tasks 3 + 4 |
| Bundle 1 commit shape | Task 5 |
| Bundle 2 Item 1 modernize loop | Task 6 |
| Bundle 2 Item 2 retire NAI-17-D1 | Task 7 |
| Bundle 2 commit shape | Task 8 |
| Bundle 3 strong-form test | Task 9 |
| Bundle 3 NAI-3 deferral retire | Task 10 |
| Bundle 3 commit shape | Task 11 |
| Memory trailer + close commit | Task 12 |

**Spec sections deferred to plan-time pinned in this plan:**
- ✓ `OpNpcQueue` operand encoding (Task 9 Step 2 bytecode)
- ✓ Side-effect opcode for amplifier (`OpNpcSetTimer` chosen, Task 9 Step 2)
- ✓ scriptProvider seed location (inline in Task 9 Step 2; not lifted into `buildNpcForIntegration`)
- ✓ Marker-script inclusion (omitted; the queue-drain + timer-set assertions are sufficient)
- ✓ LoS scenario for (a)'s self-side test (Task 1 Step 2 with fallback note)

**Coverage gaps:** none.
