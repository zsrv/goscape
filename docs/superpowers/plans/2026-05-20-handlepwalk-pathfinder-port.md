# P_WALK pathfinder port — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `handlePWalk` stub at `pkg/script/handlers_player.go:657-664` with a real pathfinder-backed P_WALK opcode implementation, so scripted `p_walk(coord)` actually queues a walk route via the existing pathfinder + waypoint infrastructure.

**Architecture:** Add one fat method `Walk(destX, destZ int)` to the `pkg/script.ActivePlayer` interface. The handler becomes a thin gate→checkCoord→Self.Walk dispatch. Production `*Player.Walk` runs `s.pathfinder().FindPathPlain(p.level, p.x, p.z, destX, destZ)` and replaces the waypoint queue via the existing `queueWaypoints(routeToPacked(route))` path.

**Tech Stack:** Go 1.26; `pkg/pathfinder/routefinder` (FindPathPlain, Route, NewRouteCoordinates); `pkg/coordgrid` (PackCoord); existing `*Server.pathfinder()` test seam at `modules/world/pathing.go:33`; existing `pathfinderRecorder` test helper at `modules/world/interaction_test.go:2167`.

---

## Spec source

`docs/superpowers/specs/2026-05-20-handlepwalk-pathfinder-port-design.md` (commit `5ecbe5fa`).

## Spec → plan deviations

1. **Production impl uses the `s.pathfinder()` seam, not a direct `p.client.server.gamemap.Pathfinder` reach.** The spec said the production impl mirrors `pathToMoveClick`'s nil-chain (direct gamemap reach); during planning we verified that `*Server.pathfinder()` (modules/world/pathing.go:33) returns a `pathfinderForTarget` interface with `FindPathPlain`, AND that this seam is already honored by all interaction-side callers, AND that tests inject a `pathfinderRecorder` via `s.testPathfinder` (interaction_test.go:2167). Using the seam (a) lets the integration tests reuse `pathfinderRecorder` without constructing a real `gamemap.GameMap`, (b) follows the newer pattern over the predating-seam pattern in `pathToMoveClick`. Zero functional difference in production.

2. **`*Player.Walk` lives in `modules/world/player_script.go`, not `movement.go`.** The spec said `movement.go`; planning verified that `player_script.go` is the established home for `script.ActivePlayer`-satisfying methods on `*Player` (SetRun, Teleport, ExactMove sister-method's neighbours). `movement.go` houses lower-level movement primitives (queueWaypoints, stepOnce, resolveMovement).

3. **Fake-sweep is one site, not many.** The spec listed multiple "likely sites subject to grep confirmation." Confirmed: there is exactly **one** `script.ActivePlayer` mock — `*mockPlayer` in `pkg/script/runner_test.go:99` — shared across all of pkg/script's test files. Plus the production `*Player` (compile-time check at `modules/world/message_game.go:11`). Two sites total.

## File map

| Path | Action |
|---|---|
| `pkg/script/active.go` | Modify: add `Walk(destX, destZ int)` to `ActivePlayer` interface |
| `pkg/script/handlers_player.go:657-664` | Modify: replace stub body of `handlePWalk` |
| `pkg/script/runner_test.go:99-…` | Modify: add `walkCalls` field + `Walk(destX, destZ int)` method to `mockPlayer` |
| `pkg/script/handlers_player_test.go:631-…` | Modify: delete `TestPWalkStubPopsAndLogs`; add 3 new tests |
| `modules/world/player_script.go` | Modify: add `*Player.Walk(destX, destZ int)` method |
| `modules/world/player_test.go` or new file | Modify/Create: add 2 new tests for `*Player.Walk` |

---

## Phase 1 — Interface + handler (pkg/script side)

### Task 1: Add interface method + extend mock + stub production impl

**Files:**
- Modify: `pkg/script/active.go` — add `Walk` to `ActivePlayer` interface
- Modify: `pkg/script/runner_test.go:99` (mockPlayer struct) and `runner_test.go:490` (next to `ExactMove` method) — add `walkCalls` field + `Walk` method
- Modify: `modules/world/player_script.go` — add `*Player.Walk` empty-body stub

This task adds compile-clean stubs only. Real handler logic and real production impl come in later tasks. Existing tests must still pass after this task — no behavioral change yet (handlePWalk still drops the coord and returns nil).

- [ ] **Step 1: Add `Walk` to `ActivePlayer` interface**

In `pkg/script/active.go`, locate `ExactMove` declaration (~line 156). Add immediately below it:

```go
	// Walk queues a path from the player's current (level, x, z) to the
	// destination (destX, destZ) at the player's level. Production impl
	// runs the server pathfinder (FindPathPlain) and replaces the player's
	// waypoint queue. Empty/failed routes leave the player stationary.
	// Mirrors TS PlayerOps.P_WALK → player.queueWaypoints(findPath(
	// player.level, player.x, player.z, coord.x, coord.z)).
	Walk(destX, destZ int)
```

- [ ] **Step 2: Add `walkCall` capture type + field to mockPlayer**

In `pkg/script/runner_test.go`, locate the `mockPlayer` struct (~line 99) and the existing `exactMoveCall` capture-type declaration nearby. Add a parallel `walkCall` type adjacent to `exactMoveCall`:

```go
type walkCall struct {
	destX, destZ int
}
```

Add a `walkCalls []walkCall` field to `mockPlayer`, adjacent to the existing `exactMoveCalls`:

```go
walkCalls []walkCall
```

- [ ] **Step 3: Add `Walk` method to mockPlayer**

In `pkg/script/runner_test.go`, locate the `ExactMove` method (line 490). Add immediately below it:

```go
func (m *mockPlayer) Walk(destX, destZ int) {
	m.walkCalls = append(m.walkCalls, walkCall{destX, destZ})
}
```

- [ ] **Step 4: Add empty-body `*Player.Walk` to production**

In `modules/world/player_script.go`, locate the existing `SetRun` method (~line 458). Add a new method anywhere among the existing script-satisfying methods (a natural slot is immediately after `SetRun`):

```go
// Walk implements script.ActivePlayer.Walk. Empty body — replaced in
// Task 6 with the real pathfinder+queueWaypoints body. Present here so
// *Player still satisfies the interface after Task 1's interface delta.
func (p *Player) Walk(destX, destZ int) {}
```

The comment will be replaced in Task 6 with the real doc.

- [ ] **Step 5: Verify compile is clean and existing tests still pass**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestPWalkStubPopsAndLogs -v
```

Expected:
- `go build ./...` → exit 0, no output.
- `TestPWalkStubPopsAndLogs` → PASS (handler still drops, mock now has a walkCalls slice that stays empty).

- [ ] **Step 6: Commit Task 1**

```bash
git status
git add pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go
git commit --no-gpg-sign -m "feat(script): add ActivePlayer.Walk seam (P_WALK port T1)"
git show --stat HEAD
```

---

### Task 2: Write 3 RED handler tests

**Files:**
- Modify: `pkg/script/handlers_player_test.go` — append three new tests (do NOT delete `TestPWalkStubPopsAndLogs` yet; it'll be retired in Task 4).

All three tests should be observed failing **for the right reason** before any implementation in Task 3. Different failure modes per test — the assertion that catches it tells you which path is unwired.

- [ ] **Step 1: Add `TestHandlePWalk_RequiresProtectedActivePlayer`**

In `pkg/script/handlers_player_test.go`, append (placement adjacent to other `TestHandlePWalk*`/`TestPExactMove*` tests is conventional but anywhere in-file works):

```go
// TestHandlePWalk_RequiresProtectedActivePlayer pins the
// ProtectedActivePlayer gate on P_WALK. Mirrors TS PlayerOps.ts:455
// checkedHandler(ProtectedActivePlayer, …).
func TestHandlePWalk_RequiresProtectedActivePlayer(t *testing.T) {
	mp := &mockPlayer{}
	packed := coordgrid.PackCoord(0, 3210, 3220)
	sf := &ScriptFile{
		Name: "[p_walk_unprotected,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPWalk, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer // protect flag intentionally unset
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_WALK: script not protected")
	}
	if got := err.Error(); !strings.Contains(got, "P_WALK") || !strings.Contains(got, "script not protected") {
		t.Errorf("err: got %q, want substrings 'P_WALK' and 'script not protected'", got)
	}
	if got := len(mp.walkCalls); got != 0 {
		t.Errorf("walkCalls: got %d, want 0 (gate should reject before dispatch)", got)
	}
}
```

- [ ] **Step 2: Add `TestHandlePWalk_RejectsInvalidCoord`**

```go
// TestHandlePWalk_RejectsInvalidCoord pins that checkCoord rejects
// out-of-range packed coords before dispatch. Mirrors TS
// check(state.popInt(), CoordValid).
func TestHandlePWalk_RejectsInvalidCoord(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "[p_walk_badcoord,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPWalk, OpReturn,
		},
		// -1 is outside the valid packed-coord range (level/x/z all OOB).
		IntOperands:      []int32{-1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want CoordValid rejection")
	}
	if got := err.Error(); !strings.Contains(got, "P_WALK") {
		t.Errorf("err: got %q, want substring 'P_WALK'", got)
	}
	if got := len(mp.walkCalls); got != 0 {
		t.Errorf("walkCalls: got %d, want 0 (coord rejection should precede dispatch)", got)
	}
}
```

- [ ] **Step 3: Add `TestHandlePWalk_DispatchesWalkWithUnpackedXZ`**

```go
// TestHandlePWalk_DispatchesWalkWithUnpackedXZ pins the happy path:
// gate satisfied + valid packed coord → Self.Walk(destX, destZ) called
// once with the unpacked X/Z. Critically pins that the packed coord's
// level component is NOT forwarded — TS uses player.level for the
// pathfinder call (PlayerOps.ts:459).
func TestHandlePWalk_DispatchesWalkWithUnpackedXZ(t *testing.T) {
	mp := &mockPlayer{}
	packed := coordgrid.PackCoord(0, 3210, 3220)
	sf := &ScriptFile{
		Name: "[p_walk,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPWalk, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.Execution; got != Finished {
		t.Errorf("Execution: got %v, want Finished", got)
	}
	if got := state.ISP; got != 0 {
		t.Errorf("ISP: got %d, want 0", got)
	}
	if got := len(mp.walkCalls); got != 1 {
		t.Fatalf("walkCalls: got %d, want 1", got)
	}
	c := mp.walkCalls[0]
	if c.destX != 3210 || c.destZ != 3220 {
		t.Errorf("Walk dispatch: got (destX=%d, destZ=%d), want (3210, 3220)", c.destX, c.destZ)
	}
}
```

- [ ] **Step 4: Run all three tests and observe RED**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandlePWalk_' -v
```

Expected: all three FAIL.
- `TestHandlePWalk_RequiresProtectedActivePlayer` → FAIL with "got nil err" (current stub returns nil regardless of gate).
- `TestHandlePWalk_RejectsInvalidCoord` → FAIL with "got nil err" (current stub doesn't call checkCoord).
- `TestHandlePWalk_DispatchesWalkWithUnpackedXZ` → FAIL with "walkCalls: got 0, want 1" (current stub doesn't call Self.Walk).

**Do not commit yet** — RED tests live in the working tree until Task 4.

---

### Task 3: Implement handler body (GREEN)

**Files:**
- Modify: `pkg/script/handlers_player.go:657-664`

- [ ] **Step 1: Replace `handlePWalk` body**

In `pkg/script/handlers_player.go`, replace the existing `handlePWalk` function (lines 657-664) with:

```go
// handlePWalk implements P_WALK (opcode 2076). Pops a packed coord,
// validates via CoordValid, and queues a path from the player's current
// position to (destX, destZ). The coord's level component is validated
// but not used — TS uses player.level for the pathfinder call
// (PlayerOps.ts:455-460). Gate: ProtectedActivePlayer.
func handlePWalk(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_WALK"); err != nil {
		return err
	}
	packed := s.PopInt()
	_, destX, destZ, err := checkCoord(packed, "P_WALK")
	if err != nil {
		return err
	}
	s.Self.Walk(destX, destZ)
	return nil
}
```

Note: `slog` is still imported by other handlers in this file; do not remove the `log/slog` import.

- [ ] **Step 2: Run the three RED tests and verify GREEN**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandlePWalk_' -v
```

Expected: all three PASS.

- [ ] **Step 3: Run the existing `TestPWalkStubPopsAndLogs` and verify it now FAILS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestPWalkStubPopsAndLogs -v
```

Expected: PASS or FAIL depending on what the test asserts. The test was written against the stub's "pops one int, executes cleanly" contract; the new handler still pops one int with the gate flag set, and the test's `state.Pointers` should be checked. If the test passes only because it didn't set the gate flag, behavior may now differ. **Read the test body and confirm:** if it's a tautological pop-and-finish check that still holds, leave it; if it asserts the slog.Debug call ("stub invoked; pathfinder integration pending"), it fails. Either way Task 4 retires it.

---

### Task 4: Retire stub test + commit Phase 1

**Files:**
- Modify: `pkg/script/handlers_player_test.go:631-…` — delete `TestPWalkStubPopsAndLogs`

- [ ] **Step 1: Delete `TestPWalkStubPopsAndLogs`**

In `pkg/script/handlers_player_test.go`, locate `TestPWalkStubPopsAndLogs` (line 631) and delete the entire function. The replacement coverage is the three new tests added in Task 2.

- [ ] **Step 2: Run all pkg/script tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -v -run 'TestHandlePWalk_|TestPWalk'
```

Expected: 3 new tests PASS; `TestPWalkStubPopsAndLogs` not found (deleted).

Then full pkg/script suite:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: ok.

- [ ] **Step 3: Commit Phase 1**

```bash
git status
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): wire P_WALK to ActivePlayer.Walk (P_WALK port T2-T4)

Replaces handlePWalk stub with a real ProtectedActivePlayer-gated
dispatch through checkCoord to Self.Walk(destX, destZ). Retires
TestPWalkStubPopsAndLogs; adds three new handler tests pinning the
gate, coord rejection, and unpack-X/Z-without-level dispatch.
Mirrors TS PlayerOps.ts:455-460.

Production *Player.Walk impl follows in Phase 2.
EOF
)"
git show --stat HEAD
```

---

## Phase 2 — Production `*Player.Walk` impl

### Task 5: Write 2 integration tests as RED

**Files:**
- Modify: `modules/world/player_test.go` (preferred home — sibling of existing `*Player` tests). If the file doesn't have a `_walk_test.go` analogue and you prefer file-per-method, a new `player_walk_test.go` is acceptable.

The integration tests reuse the existing `pathfinderRecorder` infrastructure in `modules/world/interaction_test.go:2167` via the `newPathToTargetTestServer` / `newPathToTargetTestPlayer` helpers (also in interaction_test.go).

- [ ] **Step 1: Add `TestPlayerWalk_PopulatesWaypointsViaPathfinder`**

In `modules/world/player_test.go` (append at end of file), add:

```go
// TestPlayerWalk_PopulatesWaypointsViaPathfinder pins that *Player.Walk
// routes through the server's pathfinder seam (FindPathPlain at the
// player's current level), converts the route via routeToPacked, and
// replaces the waypoint queue via queueWaypoints. Mirrors TS
// Player.queueWaypoints(findPath(this.level, this.x, this.z, destX,
// destZ)).
func TestPlayerWalk_PopulatesWaypointsViaPathfinder(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 3200, 3200, 0)

	// Seed the recorder with a two-step route. routeToPacked iterates
	// route.Waypoints in order, packing each via coordgrid.PackCoord.
	rec.returnRoute = routefinder.Route{
		Success: true,
		Waypoints: []routefinder.RouteCoordinates{
			routefinder.NewRouteCoordinates(3201, 3200, 0),
			routefinder.NewRouteCoordinates(3202, 3200, 0),
		},
	}

	p.Walk(3205, 3200)

	// Verify the pathfinder was called with the player's (level, x, z)
	// and the destination's (x, z) — destination level is NOT forwarded
	// (player.level is used).
	call, ok := rec.lastFindPathPlain()
	if !ok {
		t.Fatalf("FindPathPlain: not called")
	}
	if call.level != 0 || call.srcX != 3200 || call.srcZ != 3200 {
		t.Errorf("FindPathPlain src: got (level=%d, srcX=%d, srcZ=%d), want (0, 3200, 3200)",
			call.level, call.srcX, call.srcZ)
	}
	if call.destX != 3205 || call.destZ != 3200 {
		t.Errorf("FindPathPlain dest: got (destX=%d, destZ=%d), want (3205, 3200)",
			call.destX, call.destZ)
	}

	// Verify queueWaypoints was called with the packed route. The
	// waypoint queue stores [dest, …, first_step] (reverse order per
	// queueWaypoints contract — see modules/world/movement.go:15-36).
	// Two waypoints → waypointIndex = 1.
	if got := p.waypointIndex; got != 1 {
		t.Fatalf("waypointIndex: got %d, want 1 (2-waypoint route)", got)
	}
	// waypoints[0] holds the LAST input (route.Waypoints[1] → (3202,3200))
	// because queueWaypoints reverses on copy.
	wantFirstStored := coordgrid.PackCoord(0, 3202, 3200)
	if got := p.waypoints[0]; got != wantFirstStored {
		t.Errorf("waypoints[0]: got %d, want %d (packed (0,3202,3200))",
			got, wantFirstStored)
	}
	wantSecondStored := coordgrid.PackCoord(0, 3201, 3200)
	if got := p.waypoints[1]; got != wantSecondStored {
		t.Errorf("waypoints[1]: got %d, want %d (packed (0,3201,3200))",
			got, wantSecondStored)
	}
}
```

If `routefinder` is not already imported in this test file, add the import: `"github.com/zsrv/goscape/pkg/pathfinder/routefinder"` and `"github.com/zsrv/goscape/pkg/coordgrid"` (likely already present).

- [ ] **Step 2: Add `TestPlayerWalk_NoPathfinder_NoOp`**

In the same file, append:

```go
// TestPlayerWalk_NoPathfinder_NoOp pins the nil-guard: if the player
// has no client/server/pathfinder wiring, Walk no-ops silently. Test
// fixtures that exercise scripts without a real pathfinder rely on
// this — mirrors pathToMoveClick's nil-chain at movement.go:241.
func TestPlayerWalk_NoPathfinder_NoOp(t *testing.T) {
	p := &Player{} // bare Player: client == nil, no server, no pathfinder.
	// Sanity: initial waypointIndex is the zero value (0) since the
	// queue is a fixed-size array; the queueWaypoints contract sets
	// it to -1 for empty input. Either way, no panic is the contract.
	initialIndex := p.waypointIndex

	// Must not panic.
	p.Walk(3205, 3200)

	if got := p.waypointIndex; got != initialIndex {
		t.Errorf("waypointIndex: got %d, want %d (unchanged — Walk should be a no-op)",
			got, initialIndex)
	}
}
```

- [ ] **Step 3: Run both integration tests and observe RED**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestPlayerWalk_' -v
```

Expected:
- `TestPlayerWalk_PopulatesWaypointsViaPathfinder` → FAIL with "FindPathPlain: not called" (empty-body Walk from Task 1 ignores its args).
- `TestPlayerWalk_NoPathfinder_NoOp` → PASS immediately (empty body doesn't panic and doesn't touch waypointIndex). This test is a regression guard, not RED — that's OK. Its job is to fail loudly if Task 6's impl forgets the nil-guard.

---

### Task 6: Implement production `*Player.Walk` (GREEN)

**Files:**
- Modify: `modules/world/player_script.go` — replace the empty-body `Walk` from Task 1

- [ ] **Step 1: Replace empty-body Walk with real impl**

In `modules/world/player_script.go`, locate the `Walk` method added in Task 1 (immediately after `SetRun`). Replace with:

```go
// Walk implements script.ActivePlayer.Walk. Runs the server pathfinder
// at the player's current level via the s.pathfinder() test seam and
// replaces the waypoint queue with the result. Mirrors TS
// Player.queueWaypoints(findPath(player.level, player.x, player.z,
// destX, destZ)). No-op when no client/server/pathfinder is wired
// (test fixtures without a real or injected pathfinder).
func (p *Player) Walk(destX, destZ int) {
	if p.client == nil || p.client.server == nil {
		return
	}
	pf := p.client.server.pathfinder()
	if pf == nil {
		return
	}
	route := pf.FindPathPlain(p.level, p.x, p.z, destX, destZ)
	p.queueWaypoints(routeToPacked(route))
}
```

If the package needs an import of `routefinder` for any reason (it shouldn't — `routeToPacked` and `*Server.pathfinder()` already wrap the type), confirm `goimports` reconciles automatically. The `pathfinderForTarget` interface declared at `modules/world/pathing.go:19` already includes `FindPathPlain`, so no signature changes needed.

- [ ] **Step 2: Run both integration tests and verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestPlayerWalk_' -v
```

Expected: both PASS.

- [ ] **Step 3: Run full modules/world test suite (it's slow — ~2-3 min)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: ok. No regressions from the interface delta + impl.

---

### Task 7: Full gate sweep + commit Phase 2

- [ ] **Step 1: Full `-race` test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all packages ok / 0 FAIL. Tally: should match the resume note's "57 ok / 0 FAIL" baseline (count may have changed slightly with new files but no failures).

Verify count with:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... 2>&1 | grep -cE '^(ok|FAIL|---\sFAIL)'
```

- [ ] **Step 2: Smoke-pack**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

Expected: 12 OK / 0 ERR / 0 SKIP (matches baseline at HEAD `4ee9ffaf`).

- [ ] **Step 3: Targeted reruns**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandlePWalk_' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestPlayerWalk_' -v
```

Expected: all PASS.

- [ ] **Step 4: Pre-commit `git status` confirmation (per feedback `[[git-pre-commit-status-check]]`)**

```bash
git status
```

Expected staged: nothing (the production impl was edited but not staged yet). Expected unstaged-modified: `modules/world/player_script.go` (Walk body replaced); `modules/world/player_test.go` (or new `player_walk_test.go`) with new tests. Confirm only these two files are modified — `config.yaml` standing drift remains untracked.

- [ ] **Step 5: Commit Phase 2**

```bash
git add modules/world/player_script.go modules/world/player_test.go
# If you created modules/world/player_walk_test.go instead, swap that path in.
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): *Player.Walk runs pathfinder + queueWaypoints (P_WALK port T5-T7)

Replaces the empty-body Walk stub from T1 with the production impl:
guards client/server/pathfinder nil-chain, calls FindPathPlain via
the s.pathfinder() seam at the player's current level, replaces the
waypoint queue via routeToPacked + queueWaypoints. Two integration
tests pin the happy-path and the nil-pathfinder no-op.

Retires the long-standing "P_WALK stub; pathfinder integration
pending" deferral. No NAI-XXX-D-* tag — the deferral was plain
English with no tag attached. Mirrors TS PlayerOps.ts:455-460.
EOF
)"
git show --stat HEAD
```

---

## Self-review checklist (run after writing this plan)

- [x] **Spec coverage:** Every section in the spec maps to a task.
  - Interface delta → Task 1 step 1.
  - Handler shape → Task 3.
  - Production impl → Task 6 (note: uses `s.pathfinder()` seam per documented deviation §Spec→plan deviations #1).
  - All three handler tests → Task 2.
  - Both integration tests → Task 5.
  - Fake-sweep → Task 1 (one site only, confirmed during planning §Spec→plan deviations #3).
  - Retire `TestPWalkStubPopsAndLogs` → Task 4.
  - TDD discipline (RED first) → Tasks 2, 5 explicitly run RED-observation steps.
  - Out-of-scope items (b0 stubs, UnsetMapFlag, NaivePath variants) → not addressed (correct — out of scope).
- [x] **Placeholder scan:** No TBDs, no "add appropriate X", no "similar to Task N" without re-stating, no missing code blocks.
- [x] **Type consistency:**
  - `Walk(destX, destZ int)` signature used identically in interface (T1S1), mock (T1S3), handler dispatch (T3), production impl (T6), and all tests (T2, T5).
  - `walkCall` struct fields `destX, destZ` consistent throughout.
  - `pathfinderRecorder.lastFindPathPlain()` returns `(findPathPlainCall, bool)` — used in T5 step 1 matches the existing helper at `interaction_test.go:2167+`.
  - `routefinder.NewRouteCoordinates(x, z, level)` arg order matches `pkg/pathfinder/routefinder/routecoordinates.go:29`.
  - `queueWaypoints` reverse-on-copy contract (input `[a, b]` → stored `[b, a]`) cited correctly in T5 step 1.
