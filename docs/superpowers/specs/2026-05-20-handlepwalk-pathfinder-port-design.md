# P_WALK pathfinder port — design

**Date:** 2026-05-20
**Slice:** Replace `handlePWalk` stub in `pkg/script/handlers_player.go:657-664` with
a real pathfinder-backed implementation. Mirrors TS `PlayerOps.ts:455-460`
(`ScriptOpcode.P_WALK`).

## Goal

Wire script opcode `P_WALK` (2076) to the existing `*Player` pathfinder
infrastructure so scripted movement (`p_walk(coord)`) actually queues a
walk route. Today the handler pops the coord, logs at debug, and returns
nil — content scripts that emit `P_WALK` produce no movement.

This is a real-stub retirement, not a TS-parity stub. Five other opcodes
in `handlers_b0_stubs.go` (PUSH_VARBIT, POP_VARBIT, SET_GENDER, LC_OP,
OC_IOP, OC_OP) stay unimplemented by design and are out of scope.

## TS reference

`Engine-TS/src/engine/script/handlers/PlayerOps.ts:455-460`:

```ts
[ScriptOpcode.P_WALK]: checkedHandler(ProtectedActivePlayer, state => {
    const coord: CoordGrid = check(state.popInt(), CoordValid);

    const player = state.activePlayer;
    player.queueWaypoints(findPath(player.level, player.x, player.z, coord.x, coord.z));
}),
```

Four lines. Pop coord → validate → run pathfinder at **player's level**
(not coord's level) → replace waypoint queue. No `UnsetMapFlag` (that is
P_EXACTMOVE-only).

## Approach

Adopted **Approach A** from brainstorm: add a single fat method
`Walk(destX, destZ int)` to the `pkg/script.ActivePlayer` interface.
Production `*Player` impl in `modules/world/movement.go` handles the
pathfinder lookup and waypoint replacement internally. The handler stays
a thin TS-faithful gate-and-dispatch.

Rejected alternatives:

- **B** (PathFinder seam on ScriptState + QueueWaypoints on ActivePlayer):
  doubles interface surface for one consumer.
- **C** (PathTo + QueueWaypoints, both on ActivePlayer): two methods
  always called together with no consumer ever holding the intermediate
  `[]int`.

**A** mirrors the existing `ExactMove(sX, sZ, eX, eZ, begin, finish, dir)`
seam shape (also a fat method that hides field-writes from the script
layer), and the production impl mirrors the nil-chain in
`pathToMoveClick` (`movement.go:234-256`).

## Architecture

### Interface — `pkg/script/active.go`

Add one method to `ActivePlayer`:

```go
// Walk queues a path from the player's current (level, x, z) to the
// destination (destX, destZ) at the player's level. Production impl
// runs the server pathfinder (FindPathPlain) and replaces the player's
// waypoint queue. Empty/failed routes leave the player stationary.
// Mirrors TS PlayerOps.P_WALK → player.queueWaypoints(findPath(...)).
Walk(destX, destZ int)
```

No `level` parameter — TS validates `coord.level` (via `CoordValid`) but
calls `findPath` with `player.level`. Walk receives only destX/destZ;
production reads `p.level` directly.

### Handler — `pkg/script/handlers_player.go:657-664`

Replace existing stub body with:

```go
// handlePWalk implements P_WALK (opcode 2076). Pops a packed coord,
// validates via CoordValid, and queues a path from the player's
// current position to (destX, destZ). The coord's level component is
// validated but not used — TS uses player.level for the pathfinder
// call (PlayerOps.ts:455-460). Gate: ProtectedActivePlayer.
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

Mirrors `handlePExactMove` shape (`handlers_player.go:635-655`) minus the
`UnsetMapFlag` call.

### Production impl — `modules/world/movement.go`

Add new method on `*Player`:

```go
// Walk implements pkg/script.ActivePlayer.Walk. Runs the server
// pathfinder at the player's current level and replaces the waypoint
// queue with the result. Mirrors TS Player.queueWaypoints(findPath(
// player.level, player.x, player.z, destX, destZ)). No-op when the
// pathfinder is unavailable (test fixtures without a wired server).
func (p *Player) Walk(destX, destZ int) {
    if p.client == nil || p.client.server == nil || p.client.server.gamemap == nil {
        return
    }
    route := p.client.server.gamemap.Pathfinder.FindPathPlain(p.level, p.x, p.z, destX, destZ)
    p.queueWaypoints(routeToPacked(route))
}
```

Nil-chain guard matches `pathToMoveClick`'s style at `movement.go:241`.
`routeToPacked` already returns nil on `!route.Success` (`movement.go:283-289`);
`queueWaypoints(nil)` sets `waypointIndex = -1` (`movement.go:26-29`).
Empty-route handling is therefore implicit — no new branches needed.

## Data flow

```
P_WALK opcode
  ↓
handlePWalk (pkg/script)
  ├─ requireProtectedActivePlayer gate         (returns error if not protected)
  ├─ PopInt → packed coord
  ├─ checkCoord(packed)                        (returns error if level OOB or x/z OOB)
  └─ s.Self.Walk(destX, destZ)
       ↓
       (*Player).Walk (modules/world)
        ├─ nil-guard: client / server / gamemap
        ├─ Pathfinder.FindPathPlain(p.level, p.x, p.z, destX, destZ) → Route
        └─ p.queueWaypoints(routeToPacked(route))
             ↓
             p.waypoints[…] populated, p.waypointIndex set
             ↓
             next tick's resolveMovement steps along queue
```

## Error handling

- **Gate failure** (script not in ProtectedActivePlayer mode): handler
  returns `requireProtectedActivePlayer` error — Script execution halts
  with the existing error path used by every other Protected* opcode.
- **CoordValid failure** (level out of [0,3] or x/z out of [0, 16383]):
  `checkCoord` returns a typed error — same path as P_EXACTMOVE.
- **Pathfinder unavailable** (no gamemap wired): production no-ops
  silently, leaving `p.waypointIndex` unchanged. Matches `pathToMoveClick`
  fallback behavior — test fixtures that exercise scripts without a real
  pathfinder won't crash.
- **Empty/failed route** (player already at dest, or no walkable path):
  `routeToPacked` returns nil, `queueWaypoints` clears the queue via
  `waypointIndex = -1`. TS-equivalent: TS `findPath` returns an empty
  result, `queueWaypoints([])` is a no-op.

## Testing

### Layer 1 — `pkg/script/handlers_player_test.go`

**Replace** existing `TestPWalkStubPopsAndLogs` (line 631) — its premise
(stub pops and returns nil) is no longer the contract.

**Three new tests** using a small fake `ActivePlayer` that records
`Walk(destX, destZ)` calls (pattern matches existing
`recordingActivePlayer`-style fixtures in this file):

1. **`TestHandlePWalk_RequiresProtectedActivePlayer`** —
   `OpPushConstantInt(packed); OpPWalk; OpReturn` with no
   `PtrProtectedActivePlayer`; asserts `Execute` returns a
   `requireProtectedActivePlayer` error and `Walk` not called.

2. **`TestHandlePWalk_RejectsInvalidCoord`** — gate satisfied, pushes
   an out-of-range packed coord (e.g. `-1`); asserts `Execute` returns
   a `checkCoord` error and `Walk` not called.

3. **`TestHandlePWalk_DispatchesWalkWithUnpackedXZ`** — gate satisfied,
   pushes `coordgrid.PackCoord(0, 3210, 3220)`; asserts `Walk` recorded
   once with `(destX=3210, destZ=3220)`, ISP back to 0,
   `Execution=Finished`. **Critically pins that the packed coord's
   level is NOT passed through** — production uses `player.level`.

### Layer 2 — `modules/world/movement_test.go` (extend if exists, else new)

1. **`TestPlayerWalk_PopulatesWaypointsViaPathfinder`** — `*Player` with
   `p.client.server.gamemap` set to a real or test-fixture
   `*gamemap.GameMap` with a working `Pathfinder`. Place player at
   (level=0, x=3200, z=3200); call `p.Walk(3205, 3200)`; assert
   `p.waypointIndex >= 0` and the queued waypoints unpack to a coord
   on the route. Mirrors the verification pattern in
   `interaction_test.go` for pathfinder-driven `queueWaypoints` calls.

2. **`TestPlayerWalk_NoGamemap_NoOp`** — `*Player` with `p.client = nil`
   (or `p.client.server.gamemap = nil`); call `p.Walk(...)`; assert no
   panic and `p.waypointIndex == -1` (initial state preserved).

### Layer 3 — fake-sweep

Every existing fake satisfying `pkg/script.ActivePlayer` gains a
`Walk(destX, destZ int)` method. Likely sites (subject to grep
confirmation during execute):

- `pkg/script/handlers_player_test.go`
- `pkg/script/handlers_inv_test.go`
- `pkg/script/handlers_npc_test.go`
- any other `_test.go` constructing an `ActivePlayer` mock

Each gets `func (f *fakeXxx) Walk(destX, destZ int) { … }` — usually
no-op or capture-into-slice, matching that fake's existing pattern
(e.g. how it handles `ExactMove`).

### Gates before commit

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` →
  57+ ok / 0 FAIL.
- `smoke-pack` → 12 OK / 0 ERR / 0 SKIP.
- Targeted: `go test ./pkg/script/... -run TestHandlePWalk -v` +
  `go test ./modules/world/... -run TestPlayerWalk -v` all PASS.

### TDD discipline

**RED first.** This is a real production code path (script→pathfinder→
movement). Write the three handler tests + two integration tests first,
observe them failing for the right reason (missing `Walk` method, stub
returns without dispatching), then implement §Architecture. The
`handleConsole` slice swapped TDD order because it was a one-liner with
zero behavioral risk; this slice does not.

## Out of scope

- The five other entries in `pkg/script/handlers_b0_stubs.go` — they
  are TS-parity stubs that intentionally raise; they stay.
- `handlePOpHeld` (`handlers_player.go:1729-1741`) — also a TS-parity
  stub, also stays.
- Naive-path or entity-targeted pathfinder variants
  (`FindNaivePath`/`FindPathToEntity`/`FindPathToLoc`) — only
  `FindPathPlain` is needed; the others are reached from `interaction.go`
  via different opcodes (interaction triggers, not script opcodes).
- Any `UnsetMapFlag` call — TS P_WALK does not call it.

## Memory / deviation bookkeeping

- **No new deviation tags.** The current handler comment ("handlePWalk
  is a stub. Real implementation requires pathfinder + waypoint queue
  integration") does not carry an `NAI-XXX-D-*` tag — it's plain English.
  Retiring it requires no tag-sweep.
- **No retirement bookkeeping.** Same reason — no tag exists to retire.
- A new memory entry may be written at close describing the seam shape
  (ActivePlayer.Walk + production *Player.Walk + fake-sweep pattern)
  for future "this pattern was reused" discoverability, but only if the
  slice surfaces anything non-obvious. The pattern itself is
  `ExactMove`-precedent so likely no entry needed.

## Risk

Low. All pieces (pathfinder, routeToPacked, queueWaypoints, checkCoord,
requireProtectedActivePlayer) are production-tested; the slice is
*composition*, not invention. The only novel surface is the one-method
interface delta + its fake-sweep ripple, which is mechanical.

Behavioral risk: scripts that previously emitted `P_WALK` and got
silent no-op now actually move the player. If any content script relied
on the silent-stub behavior, it would regress — but TS-faithfulness
means TS scripts are already authored expecting movement.
