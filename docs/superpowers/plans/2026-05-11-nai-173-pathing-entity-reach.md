# NAI-173 PathingEntity reach Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Chebyshev≤1 fallthrough in both `inOperableDistance` entry points with a PathingEntity arm that calls `reach.Reached(..., 0, -2, 0)` (TS `reachedEntity`), retiring the `NAI-91-D-OPERABLE-CHEB-FALLBACK` deviation.

**Architecture:** Three production touch sites per spec §5.1 / §5.2: insert a `pathingEntity` type-assertion arm before the trailing `inOperableDistanceCheb(...)` return in `modules/world/interaction.go:663` (Player-side) and `modules/world/npc_interaction.go:720` (Npc-side). Use the existing `pathingEntity` interface (modules/world/pathing.go:12). Defensive nil-gamemap path keeps the Chebyshev fallback under the `defensive_gate_doc_comment_label.md` pattern. Doc-comments + one orphaned existing test get retirement edits in T3.

**Tech Stack:** Go 1.26+. No new dependencies. `pkg/pathfinder/reach.Reached` already imported at both production sites (verified `interaction.go:11`, `npc_interaction.go:10`).

**Spec:** `docs/superpowers/specs/2026-05-11-nai-173-pathing-entity-reach.md` (HEAD `aa165b0`).

---

## Task 1: Player-side PathingEntity reach arm

**Files:**
- Modify: `modules/world/interaction.go:623-664` (insert PathingEntity arm before final `return inOperableDistanceCheb`)
- Modify: `modules/world/interaction_test.go` (delete `TestPlayer_InOperableDistance_NpcTarget_UsesCheb` at lines 1944-1970; add new `TestPlayer_InOperableDistance_PathingEntity_Reach` table-test)

### TDD cadence

- [ ] **Step 1.1: Write the failing test table**

Append to `modules/world/interaction_test.go` (after the existing NAI-91 NPC pin block; before line 1972's `pathfinderRecorder` block):

```go
// -- NAI-173 PathingEntity reach tests (replaces NAI-91-D fallback) -------
//
// Ports TS Player.inOperableDistance (Player.ts:1099-1111) PathingEntity arm
// to reach.Reached(..., 0, -2, 0) (TS reachedEntity). Retires
// NAI-91-D-OPERABLE-CHEB-FALLBACK for the *Player and *Npc target arms.
//
// reachRectangle1 (rectangularbounds.go:15-48) reads walk-flags AT THE SOURCE
// tile: every src tile must be AllocateIfAbsent'd to clear FlagNull (=-1, all
// bits set). Diagonals reject — reachRectangle1 has no diagonal arm; this is
// TS-faithful.

// TestPlayer_InOperableDistance_PathingEntity_Reach pins the production
// reach-based PathingEntity arm. Each row asserts the post-NAI-173 result.
func TestPlayer_InOperableDistance_PathingEntity_Reach(t *testing.T) {
	cases := []struct {
		name           string
		px, pz, plevel int
		tx, tz, tlevel int
		targetIsPlayer bool
		targetSize     int // npc.size (ignored when targetIsPlayer)
		want           bool
	}{
		{"npc same-tile", 100, 100, 0, 100, 100, 0, false, 1, false},
		{"npc adjacent N (orth)", 100, 100, 0, 100, 101, 0, false, 1, true},
		{"npc adjacent E (orth)", 100, 100, 0, 101, 100, 0, false, 1, true},
		{"npc adjacent NE (diag) — TS-faithful reject", 100, 100, 0, 101, 101, 0, false, 1, false},
		{"player adjacent N (orth)", 100, 100, 0, 100, 101, 0, true, 1, true},
		{"npc distance 2 east", 100, 100, 0, 102, 100, 0, false, 1, false},
		{"npc cross-level", 100, 100, 0, 100, 101, 1, false, 1, false},
		// Multi-tile NPC (size=2): occupies (100,100)-(101,101). Player one
		// tile west of the west edge at (99,100) reaches via reachRectangle1's
		// "srcX == destX-1" arm (destWidth=2, destLength=2 → east=101, north=101;
		// srcZ=100 ∈ [100,101] and srcX=99 == 100-1 ✓). Chebyshev would also
		// pass for srcX=99,srcZ=100 (|dx|=1) — non-divergent geometry; the
		// goal here is to exercise the destWidth/destLength path, not divergence.
		{"npc multi-tile (size=2) west of west edge", 99, 100, 0, 100, 100, 0, false, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newInOperableTestServer(t)
			p, _ := newTestPlayer(t)
			p.client.server = s
			p.x, p.z, p.level = tc.px, tc.pz, tc.plevel
			// Allocate src tile so unallocated FlagNull doesn't spuriously
			// block reach (per empty_flagmap_degenerate_routefinder.md).
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(tc.px, tc.pz, tc.plevel)

			var target entity
			if tc.targetIsPlayer {
				tp, _ := newTestPlayer(t)
				tp.client.server = s
				tp.x, tp.z, tp.level = tc.tx, tc.tz, tc.tlevel
				target = tp
			} else {
				typ := &objtype.NpcType{Size: byte(tc.targetSize)}
				n := NewNpc(1, 0, tc.tx, tc.tz, tc.tlevel, typ)
				n.server = s
				target = n
			}

			if got := inOperableDistance(p, target); got != tc.want {
				t.Errorf("inOperableDistance got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPlayer_InOperableDistance_PathingEntity_NilGamemap_FallsThroughToCheb
// pins the goscape-defensive nil-gamemap arm: when srv.gamemap is nil
// (narrow test fixtures), the PathingEntity arm falls back to Chebyshev≤1.
func TestPlayer_InOperableDistance_PathingEntity_NilGamemap_FallsThroughToCheb(t *testing.T) {
	p, _ := newTestPlayer(t)
	// Construct a Server with NO gamemap — exercises the defensive branch.
	s := &Server{quit: make(chan interface{}), log: discardLogger()}
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 0, 101, 101, 0, typ) // diagonal — Chebyshev says true
	n.server = s

	if !inOperableDistance(p, n) {
		t.Fatalf("nil gamemap: expected Chebyshev fallback to allow diagonal-adjacent (got false)")
	}

	n2 := NewNpc(2, 0, 100, 100, 0, typ) // same tile — Chebyshev says false
	n2.server = s
	if inOperableDistance(p, n2) {
		t.Fatalf("nil gamemap: expected Chebyshev fallback to reject same-tile (got true)")
	}
}
```

- [ ] **Step 1.2: Run new tests, verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayer_InOperableDistance_PathingEntity_(Reach|NilGamemap)' -v
```

Expected: `TestPlayer_InOperableDistance_PathingEntity_Reach` FAILS on the
"npc adjacent N", "npc adjacent E", "player adjacent N", "npc adjacent NE",
and "npc multi-tile" rows. Pre-NAI-173 code returns the Chebyshev result for
all of them — so adjacent-NE returns true (test wants false) and multi-tile
returns true (matches), adjacent-orth returns true (matches). Same-tile
returns false (matches), distance-2 false (matches), cross-level false
(matches). The diagonal-NE row is the binding fail.

NilGamemap test PASSES pre-NAI-173 (already hits Chebyshev fallthrough).

- [ ] **Step 1.3: Implement the PathingEntity arm**

Replace `modules/world/interaction.go:663` (the lone `return inOperableDistanceCheb(p.x, p.z, tx, tz)` line) with:

```go
	if t, ok := target.(pathingEntity); ok {
		srv := p.client.server
		// goscape defensive: production sets gamemap in Server.Init; test
		// fixtures may construct a *Server without one. Fall through to
		// Chebyshev so those tests keep compiling.
		if srv.gamemap == nil {
			return inOperableDistanceCheb(p.x, p.z, tx, tz)
		}
		flags := srv.gamemap.Pathfinder.Flags
		// TS Player.ts:1104 — reachedEntity (locShape=-2, blockAccessFlags=0).
		// reach.Reached selects rectangleExclusiveStrategy → same-tile rejects
		// via Collides; orthogonal-adjacent passes when the src tile's
		// matching wall-flag is clear; diagonals reject (no rect1 diag arm).
		return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			t.Width(), t.Length(), p.Width(), 0, -2, 0)
	}
	// Defensive: target is neither *Loc nor *Obj nor pathingEntity (test
	// doubles only — production target is always one of those types).
	return inOperableDistanceCheb(p.x, p.z, tx, tz)
```

- [ ] **Step 1.4: Run new tests, verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayer_InOperableDistance_PathingEntity_(Reach|NilGamemap)' -v
```

Expected: ALL rows PASS.

- [ ] **Step 1.5: Delete the now-misleading TestPlayer_InOperableDistance_NpcTarget_UsesCheb**

The test at `modules/world/interaction_test.go:1944-1970` is named "_UsesCheb" but post-NAI-173 the production path no longer uses Chebyshev for NPC targets. Its "adjacent" row would also FAIL because the test fixture's gamemap has no flags allocated at the src tile (FlagNull blocks reach). Coverage is fully replaced by Step 1.1's table.

Delete the lines (verify the exact range first via `git diff -U0 HEAD -- modules/world/interaction_test.go` after the next step's edit). The block to delete starts with the `// TestPlayer_InOperableDistance_NpcTarget_UsesCheb pins…` comment at line 1944 (or wherever it sits after the Step 1.1 insertion) and ends at the closing `}` before the `pathfinderRecorder` block.

- [ ] **Step 1.6: Run full modules/world test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. If any unrelated test fails, halt and investigate per `verify_implementer_claims.md` before proceeding.

- [ ] **Step 1.7: Commit Task 1**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-173 T1 — port player-side PathingEntity reach

Replace the Chebyshev≤1 fallthrough in inOperableDistance with a
pathingEntity-arm dispatch to reach.Reached(..., 0, -2, 0), matching
TS Player.inOperableDistance (Player.ts:1099-1111) reachedEntity call.

Retires the *Player and *Npc target arms of NAI-91-D-OPERABLE-CHEB-FALLBACK
on the player side. Defensive nil-gamemap fallback preserved per
defensive_gate_doc_comment_label.md.

Pins the new reach-based table (8 rows incl. diagonal-rejects + multi-tile
NPC + cross-level). Deletes the now-misleading NpcTarget_UsesCheb test —
its "adjacent" row would have broken on FlagNull-blocked reach in the
empty test fixture (per empty_flagmap_degenerate_routefinder.md).
EOF
)"
```

---

## Task 2: Npc-side PathingEntity reach arm

**Files:**
- Modify: `modules/world/npc_interaction.go:687-721` (insert PathingEntity arm before final `return inOperableDistanceCheb`)
- Modify: `modules/world/npc_interaction_test.go` (rename `TestNpcInOperableDistance` doc-comment + body to make defensive-arm role explicit; add new `TestNpc_InOperableDistance_PathingEntity_Reach`)

### TDD cadence

- [ ] **Step 2.1: Write the failing test table**

Append to `modules/world/npc_interaction_test.go` (after `TestNpcInOperableDistance` at line 763; before `TestNpcInApproachDistance` at line 765):

```go
// TestNpc_InOperableDistance_PathingEntity_Reach pins the production
// reach-based PathingEntity arm on the Npc side. Mirrors NAI-173 player-side
// table from interaction_test.go.
//
// reachRectangle1 reads walk-flags at the SOURCE tile; each row allocates
// the src tile to clear FlagNull. Diagonals reject (TS-faithful).
func TestNpc_InOperableDistance_PathingEntity_Reach(t *testing.T) {
	cases := []struct {
		name           string
		nx, nz, nlevel int
		nsize          int
		tx, tz, tlevel int
		targetIsPlayer bool
		targetSize     int
		want           bool
	}{
		{"npc->npc same-tile", 100, 100, 0, 1, 100, 100, 0, false, 1, false},
		{"npc->npc adjacent N (orth)", 100, 100, 0, 1, 100, 101, 0, false, 1, true},
		{"npc->npc adjacent E (orth)", 100, 100, 0, 1, 101, 100, 0, false, 1, true},
		{"npc->npc adjacent NE (diag) — TS-faithful reject", 100, 100, 0, 1, 101, 101, 0, false, 1, false},
		{"npc->player adjacent N (orth)", 100, 100, 0, 1, 100, 101, 0, true, 1, true},
		{"npc->npc distance 2 east", 100, 100, 0, 1, 102, 100, 0, false, 1, false},
		{"npc->npc cross-level", 100, 100, 0, 1, 100, 101, 1, false, 1, false},
		// Multi-tile SOURCE npc (size=2) occupies (100,100)-(101,101). Target
		// player one tile north of the north edge at (100,102) reaches via
		// reachRectangleN's "srcZ == destNorth" arm (srcSize=2). Pins the
		// srcSize divergence — Chebyshev (center-coord) would say |dz|=2 false.
		{"npc multi-tile (size=2) -> player N of N edge", 100, 100, 0, 2, 100, 102, 0, true, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newInOperableTestServer(t)
			ntyp := &objtype.NpcType{Size: byte(tc.nsize)}
			n := NewNpc(1, 0, tc.nx, tc.nz, tc.nlevel, ntyp)
			n.server = s
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(tc.nx, tc.nz, tc.nlevel)

			var target entity
			if tc.targetIsPlayer {
				tp, _ := newTestPlayer(t)
				tp.client.server = s
				tp.x, tp.z, tp.level = tc.tx, tc.tz, tc.tlevel
				target = tp
			} else {
				ttyp := &objtype.NpcType{Size: byte(tc.targetSize)}
				tn := NewNpc(2, 0, tc.tx, tc.tz, tc.tlevel, ttyp)
				tn.server = s
				target = tn
			}

			if got := n.inOperableDistance(target); got != tc.want {
				t.Errorf("n.inOperableDistance got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNpc_InOperableDistance_PathingEntity_NilServer_FallsThroughToCheb
// pins the goscape-defensive nil-server arm. Mirrors player-side
// nil-gamemap pin.
func TestNpc_InOperableDistance_PathingEntity_NilServer_FallsThroughToCheb(t *testing.T) {
	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 0, 100, 100, 0, typ) // n.server == nil

	target := &Npc{x: 101, z: 101, level: 0} // diagonal — Chebyshev says true
	if !n.inOperableDistance(target) {
		t.Fatalf("nil server: expected Chebyshev fallback to allow diagonal-adjacent (got false)")
	}

	sameTile := &Npc{x: 100, z: 100, level: 0}
	if n.inOperableDistance(sameTile) {
		t.Fatalf("nil server: expected Chebyshev fallback to reject same-tile (got true)")
	}
}
```

- [ ] **Step 2.2: Run new tests, verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc_InOperableDistance_PathingEntity_(Reach|NilServer)' -v
```

Expected: `TestNpc_InOperableDistance_PathingEntity_Reach` FAILS on the
"npc->npc adjacent NE (diag)" row (pre-NAI-173 returns Chebyshev=true; spec
wants false) and the "npc multi-tile" row (pre-NAI-173 returns
Chebyshev=false for |dz|=2; spec wants true via srcSize=2 reach). NilServer
test PASSES pre-NAI-173 (already on Chebyshev path via nil-server guard).

- [ ] **Step 2.3: Implement the PathingEntity arm**

Replace `modules/world/npc_interaction.go:720` (the trailing `return
inOperableDistanceCheb(n.x, n.z, tx, tz)` line; keep the doc-comment above
the function for now, T3 retires the NAI-91-D bullet) with:

```go
	if t, ok := target.(pathingEntity); ok && n.server != nil && n.server.gamemap != nil {
		flags := n.server.gamemap.Pathfinder.Flags
		srcSize := n.size
		if srcSize <= 0 {
			srcSize = 1
		}
		// TS PathingEntity.ts:383 — reachedEntity (locShape=-2,
		// blockAccessFlags=0). Npc inherits this from the base class
		// (no Player-style Obj override; that's npc_interaction.go:705-716).
		return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
			t.Width(), t.Length(), srcSize, 0, -2, 0)
	}
	// Defensive: nil server / nil gamemap (test fixtures), or non-pathing
	// non-Loc non-Obj target (test doubles only). Production target is always
	// one of those types and production server always has a gamemap.
	return inOperableDistanceCheb(n.x, n.z, tx, tz)
```

- [ ] **Step 2.4: Run new tests, verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc_InOperableDistance_PathingEntity_(Reach|NilServer)' -v
```

Expected: ALL rows PASS.

- [ ] **Step 2.5: Rename existing TestNpcInOperableDistance to make defensive role explicit**

The existing test at `modules/world/npc_interaction_test.go:734-763` constructs `n := NewNpc(1, 42, 100, 100, 0, typ)` with no `n.server` assignment, so post-NAI-173 it still hits the defensive Chebyshev arm. All four rows (same-tile, adjacent N, diagonal NE, two tiles away) keep passing. Update the doc-comment to make the defensive role explicit per `defensive_gate_doc_comment_label.md`:

Find the function header at line 734:

```go
func TestNpcInOperableDistance(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
```

Replace with:

```go
// TestNpcInOperableDistance_DefensiveFallback pins the goscape-defensive
// Chebyshev arm (n.server == nil) for npc->npc operable distance. Post-NAI-173
// the production path uses reach.Reached for PathingEntity targets — see
// TestNpc_InOperableDistance_PathingEntity_Reach for that coverage.
func TestNpcInOperableDistance_DefensiveFallback(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ) // n.server == nil → defensive Cheb
```

(Body unchanged — the existing 4-row table + cross-level subtest stay.)

- [ ] **Step 2.6: Run full modules/world test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. The renamed test continues to pass (defensive arm). New
table tests pass.

- [ ] **Step 2.7: Commit Task 2**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-173 T2 — port npc-side PathingEntity reach

Mirror T1 on the Npc side: insert pathingEntity arm in
(*Npc).inOperableDistance dispatching to reach.Reached(..., 0, -2, 0),
matching TS PathingEntity.inOperableDistance (PathingEntity.ts:378-390)
which Npc inherits unchanged.

srcSize threads n.size through the new arm with the existing
"if srcSize <= 0 { srcSize = 1 }" guard from the Loc/Obj branches.

Pins the new reach-based table (mirrors NAI-173 T1) plus a multi-tile
SOURCE npc row (size=2) that exercises the srcSize divergence from
Chebyshev. Renames the existing TestNpcInOperableDistance to
_DefensiveFallback per defensive_gate_doc_comment_label.md (n.server==nil
fixture exercises the defensive Chebyshev arm post-NAI-173).
EOF
)"
```

---

## Task 3: Doc-comment retirement + tracker cleanup

**Files:**
- Modify: `modules/world/interaction.go:604-622` (function header doc-comment for `inOperableDistance`)
- Modify: `modules/world/interaction.go:666-669` (function header doc-comment for `inOperableDistanceCheb`)
- Modify: `modules/world/npc_interaction.go:671-686` (function header doc-comment for `(*Npc).inOperableDistance`)
- Modify: `modules/world/npc_interaction.go:718-720` (drop the `// Chebyshev fallback (NAI-91-D-OPERABLE-CHEB-FALLBACK)…` comment, replace with defensive-only framing)
- Modify: `modules/world/interaction_test.go:268-271` (doc-comment for `TestInOperableDistanceCheb_PathingEntityFallback`)
- Modify: `docs/superpowers/specs/NAI_FOLLOWUPS.md` *(or wherever the tracker entry lives)* — verify and remove the `NAI-91-D-OPERABLE-CHEB-FALLBACK` line

### Steps

- [ ] **Step 3.1: Locate the tracker entry**

Run:
```bash
rg -n "NAI-91-D-OPERABLE-CHEB-FALLBACK" docs/ | grep -v "specs/2026-05-11-nai-173\|plans/2026-05-11-nai-173\|plans/2026-05-10-nai-152\|plans/2026-05-04-nai-9"
```

Expected: one or two lines pointing at the active tracker doc (likely
`docs/superpowers/specs/NAI_FOLLOWUPS.md` or similar — confirm by reading
the path each match comes from). The historical plan files at
`plans/2026-05-04-nai-91*`, `plans/2026-05-10-nai-152*`, and
`plans/2026-05-04-nai-92*` are immutable — leave them alone.

Save the tracker file path; the next step deletes the entry from it.

- [ ] **Step 3.2: Update production doc-comments**

In `modules/world/interaction.go`, replace the existing `inOperableDistance`
function-header doc-comment block (currently `interaction.go:604-622`) with:

```go
// inOperableDistance reports whether p is in contact range of target.
// Mirrors TS Player.inOperableDistance (Player.ts:1099-1111):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached for shape /
//     angle / forceapproach-aware reach (NAI-91).
//   - Obj targets dispatch to reach.Reached twice — locShape=-2
//     (reachedEntity) OR locShape=-1 (reachedObj). Same-tile pickup
//     succeeds via the locShape=-1 short-circuit at strategy.go:37
//     (NAI-152 B2). 1×1 Obj invariant: NewObj sets Width=Length=1
//     unconditionally (pkg/entity/obj.go:39).
//   - PathingEntity (Player, Npc) targets dispatch to reach.Reached with
//     locShape=-2 (reachedEntity) (NAI-173). reachRectangle1 has no
//     diagonal arm, so diagonally-adjacent entity targets reject —
//     TS-faithful semantic divergence from the pre-NAI-173 Chebyshev
//     fallback.
//
// target.level mismatch returns false (TS guard preserved at all arms).
//
// INVARIANT: pkg/entity/Loc.Width / Loc.Length store ABSOLUTE (un-rotated)
// dimensions — verified at modules/world/script_loc_ops.go:35-43 and
// pkg/gamemap/load.go:128. reach.Reached rotates internally via
// rotation.Rotate(locAngle, destWidth, destLength); no double-rotation.
```

In the same file, replace the `inOperableDistanceCheb` doc-comment block
(currently at the lines immediately above `func inOperableDistanceCheb`, was
`interaction.go:666-669`) with:

```go
// inOperableDistanceCheb is the goscape-defensive Chebyshev≤1 fallback
// (excludes same-tile) used only by the nil-gamemap test-fixture paths in
// inOperableDistance and (*Npc).inOperableDistance. Production never
// reaches this since NAI-91 (Loc), NAI-152 B2 (Obj), and NAI-173
// (PathingEntity) cover all production target types via reach.Reached.
//
// (goscape defensive; TS skips this check.)
```

In `modules/world/npc_interaction.go`, replace the `(*Npc).inOperableDistance`
function-header doc-comment block (currently `npc_interaction.go:671-686`)
with:

```go
// inOperableDistance reports whether n is in contact range of target.
// Mirrors TS PathingEntity.inOperableDistance (PathingEntity.ts:378-390):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached (shape /
//     angle / forceapproach-aware) with srcSize=n.size (NAI-91).
//   - Obj targets dispatch to reach.Reached with locShape=-1
//     (reachedObj). No OR-chain — TS base class uses reachedObj only;
//     Player.ts:1110 overrides to OR reachedEntity but Npc inherits the
//     base (NAI-152 B2 T3). Same-tile pickup succeeds via the
//     strategy.go:37 short-circuit.
//   - PathingEntity (Player, Npc) targets dispatch to reach.Reached with
//     locShape=-2 (reachedEntity) (NAI-173). srcSize=n.size with the
//     "if srcSize <= 0 { srcSize = 1 }" defensive guard mirrored from the
//     Loc/Obj branches.
//
// Defensive: nil n.server / nil gamemap falls through to Chebyshev so test
// fixtures constructing minimal *Npc without a server keep working
// (goscape defensive; production Server.Init always sets gamemap).
```

In the same file, find the trailing `return inOperableDistanceCheb(n.x, n.z, tx, tz)`
at the bottom of `(*Npc).inOperableDistance` (post-Task-2 insertion). The
comment block immediately above that return came from Task 2 Step 2.3 and
already reads:

```go
	// Defensive: nil server / nil gamemap (test fixtures), or non-pathing
	// non-Loc non-Obj target (test doubles only). Production target is always
	// one of those types and production server always has a gamemap.
	return inOperableDistanceCheb(n.x, n.z, tx, tz)
```

If a leftover `// Chebyshev fallback (NAI-91-D-OPERABLE-CHEB-FALLBACK); shared with`
comment block from pre-NAI-173 still sits anywhere in the function (Task 2
should have replaced it), delete it. Net state: exactly one defensive
comment block, no NAI-91-D references.

- [ ] **Step 3.3: Update the standalone Cheb test doc-comment**

In `modules/world/interaction_test.go`, find the doc-comment block above
`TestInOperableDistanceCheb_PathingEntityFallback` (line 268-271 at HEAD):

```go
// TestInOperableDistanceCheb_PathingEntityFallback pins the Chebyshev≤1
// excluding-same-tile predicate for PathingEntity (Player, Npc) targets.
// Lives under NAI-91-D-OPERABLE-CHEB-FALLBACK pending entity-shape port.
// Renamed from TestInOperableDistance at NAI-91 T1.
```

Replace with:

```go
// TestInOperableDistanceCheb_DefensiveFallback pins the goscape-defensive
// Chebyshev≤1 (excluding same-tile) predicate retained for the nil-gamemap
// test-fixture paths in inOperableDistance / (*Npc).inOperableDistance.
// Production never reaches inOperableDistanceCheb post-NAI-173 — see
// TestPlayer_InOperableDistance_PathingEntity_NilGamemap_FallsThroughToCheb
// for the production-path defensive-arm coverage.
```

And rename the function name on the next line accordingly:
`func TestInOperableDistanceCheb_PathingEntityFallback(t *testing.T) {` →
`func TestInOperableDistanceCheb_DefensiveFallback(t *testing.T) {`

- [ ] **Step 3.4: Retire the tracker entry**

Open the tracker file located in Step 3.1. Find the line containing
`NAI-91-D-OPERABLE-CHEB-FALLBACK — Chebyshev≤1 fallback retained for
*Player / *Npc / *Obj targets` (or close paraphrase) and delete the entire
bullet/row. The Obj remainder was already closed by NAI-152 B2; this
sub-spec closes the Player + Npc remainder, retiring the entry entirely.

If the tracker entry mentions other deviation tags being retired together,
preserve those — only delete the NAI-91-D line.

- [ ] **Step 3.5: Verify tag retirement is complete**

Run:
```bash
rg -n "NAI-91-D-OPERABLE-CHEB-FALLBACK" pkg/ modules/ cmd/
```

Expected: ZERO matches. Any remaining hit means a doc-comment or live test
was missed — fix before proceeding.

Then verify the docs/ residual:
```bash
rg -n "NAI-91-D-OPERABLE-CHEB-FALLBACK" docs/
```

Expected: only matches in immutable historical plans
(`plans/2026-05-04-nai-91-shape-aware-inoperabledistance.md`,
`plans/2026-05-04-nai-92-smart-pathfinding-port.md`,
`plans/2026-05-10-nai-152-b2-pickup-chain-plan.md`) and the NAI-173 spec/plan
themselves. No matches in the active tracker file.

- [ ] **Step 3.6: Run full test suite + build**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: clean.

- [ ] **Step 3.7: Commit Task 3**

```bash
git add modules/world/interaction.go modules/world/npc_interaction.go modules/world/interaction_test.go docs/
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-173 T3 — retire NAI-91-D-OPERABLE-CHEB-FALLBACK

Update doc-comments for inOperableDistance (player + npc) to reflect the
NAI-173 PathingEntity arm. Re-label inOperableDistanceCheb +
TestInOperableDistanceCheb_DefensiveFallback as goscape-defensive only
(production never reaches them post-NAI-173).

Drop the tracker entry for NAI-91-D-OPERABLE-CHEB-FALLBACK — the Obj
clause was closed by NAI-152 B2; the *Player + *Npc clauses are now
closed by T1 + T2.

Per retire_deviation_grep_all_comments.md +
defensive_gate_doc_comment_label.md.

Closes memory: NAI-91-D-OPERABLE-CHEB-FALLBACK
EOF
)"
```

---

## Self-Review Checklist (controller, before dispatching T1)

- [ ] **Spec coverage:** §5.1 Player arm → T1; §5.2 Npc arm → T2; §5.3 doc-comment retirement → T3; §6 behavioral diff → §7.1/§7.2 pin tables in T1/T2; §7.3 existing pin updates → T1.5 (delete UsesCheb), T2.5 (rename TestNpcInOperableDistance), T3.3 (Cheb test rename); §8 risks → addressed in test fixtures.
- [ ] **Placeholder scan:** zero "TBD"/"TODO"/"similar to" patterns.
- [ ] **Type consistency:** `pathingEntity` interface used in both T1 + T2; `entity` interface used as test variable type (matches `inOperableDistance` signature in interaction.go:623 + `(n *Npc).inOperableDistance` in npc_interaction.go:687); `objtype.NpcType.Size` is `byte` per `npc.go:173` (`size: int(typ.Size)`) — fixtures cast `byte(tc.targetSize)`.
- [ ] **Test runnability:** all fixtures use already-existing helpers (`newInOperableTestServer`, `newTestPlayer`, `discardLogger`, `NewNpc`); imports already present (`objtype`, `entitypkg` — though `entitypkg` is unused in npc_interaction_test.go's new code, only `objtype` is needed there); `s.gamemap.Pathfinder.Flags.AllocateIfAbsent` pattern matches `interaction_test.go:2601` (NAI-152 B2 precedent).
- [ ] **Sibling-site guard parity:** Npc-side `n.server != nil && n.server.gamemap != nil` guard mirrors the existing Loc arm at `npc_interaction.go:692` and Obj arm at `npc_interaction.go:705` (per `plan_sibling_site_guard_audit.md`).
- [ ] **Variable name collisions:** in T1.3 the new arm uses `srv := p.client.server` — collides with no inner `:=` in the function (the Loc/Obj arms above also use `srv := p.client.server` in their own scopes; safe).
