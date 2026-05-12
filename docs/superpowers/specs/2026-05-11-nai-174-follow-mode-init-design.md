# NAI-174 — Player follow `followX/Z` plumbing (login init + unconditional refresh)

**Date:** 2026-05-11
**Status:** Design
**Tracker:** Retires `NAI-173-FU-FOLLOW-MODE-INVESTIGATION` (carry-forward from NAI-173 close at `nai_followups.md` "## From NAI-173")
**Predecessor:** NAI-173 (PathingEntity reach arm — `9f72196` close)
**HEAD at design:** `9f72196` (top of main, post-NAI-173 close)

## 1. Problem

NAI-173 smoke (Target 2) surfaced a player-follow regression: when one
player `/follow`s another, the follower runs in a fixed direction until a
~5-tile gap opens, then keeps face-updating without actually following when
the leader moves. The mechanism is unrelated to NAI-173's reach-arm
dispatch — `inOperableDistance` is the stop-gate predicate, not the
destination-tile chooser. The destination-tile chooser is the
`pathToPathingTarget` follow-op arm at `interaction.go:802-809`, which
calls `queueWaypoint(t.followX, t.followZ)`.

The bug is in how `followX/Z` is populated on the leader. Two independent
TS-fidelity gaps stack:

### 1.1 Bug 1 — Missing login init of `lastStepX/Z`

TS `Player.onLogin` (`Engine-TS/src/engine/entity/Player.ts:511-512`) sets
```ts
this.lastStepX = this.x - 1;
this.lastStepZ = this.z;
```
after the player's coords are loaded — establishing an "imaginary previous
step from the west" so a freshly-logged-in player has a valid
`lastStepX/Z` even before they take any step (also makes the player face
east).

Goscape `processLogins` (`modules/world/tick.go:99-167`) — the equivalent
of `onLogin` — never sets `lastStepX/Z`. `NewPlayer`
(`modules/world/player.go:502-503`) initializes them to `-1, -1`. A player
who logs in and never moves has `lastStepX/Z = (-1, -1)` permanently.

Teleport already mirrors TS: `player_script.go:565-566` sets
`p.lastStepX = p.x - 1; p.lastStepZ = p.z` per TS `Player.ts:2037-2038`
and `PathingEntity.ts:291-292`. Per-step transitions also work:
`movement.go:140-141` sets `lastStepX/Z = (p.x, p.z)` before incrementing
`p.x/p.z` (capturing the pre-step coord, equivalent to TS
`PathingEntity.ts:177-178`'s `lastStepX = previousX`). The remaining
gap is exclusively the initial-login set.

### 1.2 Bug 2 — `processInteraction` early-return short-circuits unconditional writes

TS `Player.processInteraction` (`Engine-TS/src/engine/entity/Player.ts:1200-1207`):

```ts
processInteraction() {
    this.followX = this.lastStepX;
    this.followZ = this.lastStepZ;
    this.nextTarget = null;

    const followOp = this.targetOp === ServerTriggerType.APPLAYER3 || this.targetOp === ServerTriggerType.OPPLAYER3;

    let interacted = false;

    if (this.target && this.canAccess()) {
        // ...
    }
    // ...
}
```

The `followX/Z` and `nextTarget` writes are UNCONDITIONAL — they fire
every tick for every player, regardless of whether the player has a
target. The target-gated branches follow.

Goscape `(*Player).processInteraction` (`modules/world/interaction.go:189-228`):

```go
func (p *Player) processInteraction() {
    if p.target == nil {
        return
    }
    if p.client == nil || p.client.server == nil {
        return
    }
    s := p.client.server
    // ... entry-guard short-circuit, Frame B capture ...

    // TS L1201-1202.
    p.followX = p.lastStepX
    p.followZ = p.lastStepZ
    // TS L1203.
    p.nextTarget = nil
    // ...
}
```

The early-return at line 190-192 happens BEFORE the `followX/Z` writes at
lines 227-228. Result: a player without a target never updates their
`followX/Z`. Their values stay frozen at whatever they were last assigned
(which, due to Bug 1, is `(-1, -1)` from `NewPlayer` for a never-moved
post-login leader).

### 1.3 Cascade

Combined: a stationary post-login leader has `followX/Z = (-1, -1)`. A
follower's `pathToPathingTarget` reads `t.followX = -1, t.followZ = -1`
and calls `queueWaypoint(-1, -1)`. The pathfinder returns a best-effort
partial path from the follower's coord toward `(-1, -1)` (off-map, SW
direction), which stalls after ~5 tiles. The "facing updates without
following" symptom is the follower's `processInteraction` continuing to
fire (target=leader, faceEntity logic still runs) while the follower has
exhausted their waypoint and the repath gate at `interaction.go:802`
re-queues `(-1, -1)` — same stale destination → same partial stall.

The bug is invisible if the leader has moved at least one tile AND has a
target — but a `/follow` leader typically has no target (they're being
followed, not interacting), so Bug 2 keeps their `followX/Z` frozen.

## 2. Goal

Port both TS-fidelity gaps so player-follow converges to orthogonal-adjacent
(NAI-173 diagonal-reject contract preserved):

1. `processLogins` sets `lastStepX = p.x - 1; lastStepZ = p.z` after the
   player's coord is finalized (TS `Player.ts:511-512`).
2. `(*Player).processInteraction` writes `followX/Z` and `nextTarget`
   above the `p.target == nil` early-return (TS `Player.ts:1200-1207`).

Retire the `NAI-173-FU-FOLLOW-MODE-INVESTIGATION` carry-forward.

## 3. Out of scope

- NPC follow `followX/Z` plumbing — TS `PathingEntity.ts:51-52` declares
  the fields on the base class so NPCs inherit them, but goscape declares
  them on `*Player` only. The `isFollowOp` predicate at
  `interaction.go:145-151` enforces `*Player`-only follow-op routing
  (`APPLAYER3`/`OPPLAYER3` `targetOp` identity), making the `*Npc` arm of
  the follow-op queue-waypoint branch dead-code-by-construction in both
  engines. See `NAI-98-D` family for the prior analysis. Adding NPC
  `followX/Z` fields is YAGNI absent a content-side smoke that exercises
  an NPC-on-player follow.
- Combat / NPC aggression on diagonal (NAI-173 smoke Target 3) — a
  separate downstream issue, deferred per NAI-173 close.
- The other unconditional-write at TS `Player.ts:1203`
  (`this.nextTarget = null`) is already covered by the Bug 2 fix since
  the three top writes move together. No separate sub-spec needed.
- `pathToPathingTarget`'s `(t.followX, t.followZ)` consumer at
  `interaction.go:807` — already TS-faithful (TS `Player.ts:1040`).
  Whether the leader's `followX/Z` is currently valid is what NAI-174
  fixes; the consumer's call shape stays.

## 4. TS source of truth

### 4.1 `onLogin` lastStep init (Player.ts:506-513)

```ts
const loginTrigger = ScriptProvider.getByTriggerSpecific(ServerTriggerType.LOGIN, -1, -1);
if (loginTrigger) {
    this.executeScript(ScriptRunner.init(loginTrigger, this), true);
}

this.lastStepX = this.x - 1;
this.lastStepZ = this.z;
this.isActive = true;
```

Order matters: LOGIN trigger runs first (it may teleport, which in TS
sets its own `lastStepX/Z` per `Player.ts:2037-2038`), then the
unconditional `lastStepX = x - 1; lastStepZ = z;` finalizes the
post-login state.

### 4.2 `processInteraction` top writes (Player.ts:1200-1207)

```ts
processInteraction() {
    this.followX = this.lastStepX;
    this.followZ = this.lastStepZ;
    this.nextTarget = null;

    const followOp = this.targetOp === ServerTriggerType.APPLAYER3 || this.targetOp === ServerTriggerType.OPPLAYER3;

    let interacted = false;

    if (this.target && this.canAccess()) {
        // ...
    }
    // ...
}
```

TS has no early-return on `target == null`. The function executes its
top assignments, then the target-gated `if (this.target && ...)` block
falls through silently when there's no target.

### 4.3 Follow-op consumer (Player.ts:1035-1042)

Already mirrored at `interaction.go:802-809`. NAI-174 does not change
this site.

## 5. Design

### 5.1 Bug 2 fix — `(*Player).processInteraction` reorder

In `modules/world/interaction.go:189-228`, move the three top writes
(followX, followZ, nextTarget) ABOVE the `p.target == nil` early-return.
Final structure:

```go
func (p *Player) processInteraction() {
    // TS L1201-1203 — unconditional top writes.
    p.followX = p.lastStepX
    p.followZ = p.lastStepZ
    p.nextTarget = nil

    if p.target == nil {
        return
    }
    if p.client == nil || p.client.server == nil {
        return
    }
    s := p.client.server
    // ... entry-guard short-circuit (goscape-only delay window
    //     optimization at L204-206) ...

    // NAI-79 Stage 1 — pre-step state capture for Frame B emit at tail.
    // ... unchanged ...
}
```

The Frame B branch-counter resets at lines 222-224 (`p.lastInteractBranchPre = 0`,
`p.lastInteractBranchPost = 0`, `p.interactCallSlot = 0`) STAY where they
are — gated behind the `p.target == nil` check per the existing
`TestProcessInteraction_CanAccessGate_NilTarget_PostStepSkipped`
invariant at `interaction_canaccess_gate_test.go:138-154` (which asserts
"branch counters not mutated with nil target at entry").

Doc-comment update at `interaction.go:170-188`: drop the "No target /
no client / delayed: no-op" framing from the branch summary — the top
writes now fire unconditionally per TS. Replace with: "No client /
delayed: no-op AFTER the top followX/Z/nextTarget writes (TS L1200-1203
fires unconditionally). No-target: top writes fire, then early-return."

### 5.2 Bug 1 fix — `processLogins` lastStep init

In `modules/world/tick.go:99-167`, insert the lastStep init immediately
after the LOGIN-trigger run at lines 155-158 and before the InputTracking
allocation at line 163 (concretely: at the blank-line boundary at line
159, replacing it with the two-line write + a trailing blank line).
Matches TS `Player.ts:506-513` ordering — LOGIN trigger first, then
lastStep init. Audit at design time confirmed nothing between the LOGIN
trigger and the end of the `processLogins` loop body mutates `p.x/p.z`
(`NewInputTracking` reads only, doesn't mutate).

```go
// TS Player.ts:511-512 — establish the "imaginary previous step from
// the west" so followX/Z reads a valid coord before the player takes
// their first step. Mirrors the teleport-time init at
// player_script.go:565-566.
p.lastStepX = p.x - 1
p.lastStepZ = p.z
```

NPC equivalent: `Npc.onLogin` doesn't exist in TS (NPCs don't log in).
NAI-19 / Despawn-lifecycle covers NPC spawn-time field defaults. No NPC
change needed for NAI-174.

### 5.3 Doc-comment retirement

`interaction.go:171-188` — see §5.1.

No other doc-comment carries `NAI-173-FU-FOLLOW-MODE-INVESTIGATION` —
it lives only in the memory tracker `nai_followups.md` "## From NAI-173"
section. T3 retires the carry-forward there with a strikethrough +
`RETIRED 2026-05-11 by NAI-174` annotation, mirroring how T3 of NAI-173
retired the NAI-91-D entry.

## 6. Affected call sites (audit complete)

Production callers of `(*Player).processInteraction`:
- `modules/world/tick.go:626` — per-tick processInteractions loop.
  Now writes followX/Z + nextTarget for every player every tick.

Producers of `p.lastStepX/Z` (writes):
- `modules/world/player.go:502-503` — NewPlayer init (-1, -1). Unchanged.
- `modules/world/tick.go:99-167` — processLogins. NEW write per §5.2.
- `modules/world/player_script.go:565-566` — Teleport. Unchanged.
- `modules/world/movement.go:140-141` — per-step transition. Unchanged.

Consumers of `p.followX/Z` (reads):
- `modules/world/interaction.go:807` — follow-op `queueWaypoint`.
  Unchanged (NAI-174 makes its input valid; the call shape stays).

Consumers of `p.lastStepX/Z` (reads):
- `modules/world/interaction.go:227-228` — `processInteraction` top
  writes (the read side of "followX = lastStepX"). Moved per §5.1.

## 7. Test plan

### 7.1 Bug 2 pin — `TestProcessInteraction_TopWritesFireUnconditionally`

`modules/world/interaction_canaccess_gate_test.go` (or sibling). Fixture:
`newTestPlayer`; manually set `p.lastStepX = 3200, p.lastStepZ = 3210`,
`p.followX = -1, p.followZ = -1`, `p.nextTarget = somePlayer`,
`p.target = nil`.

Call `p.processInteraction()`. Assert:
- `p.followX == 3200`
- `p.followZ == 3210`
- `p.nextTarget == nil`
- `p.lastInteractBranchPre == 0` and `p.lastInteractBranchPost == 0`
  (existing invariant from NilTarget_PostStepSkipped — preserved).

Pre-NAI-174: FAILS — followX/Z stays at -1/-1 because the early-return
fires before the writes.

### 7.2 Bug 2 pin — update the existing NilTarget test

`TestProcessInteraction_CanAccessGate_NilTarget_PostStepSkipped` at
`interaction_canaccess_gate_test.go:138-154`. Doc-comment update:
clarify that the top three writes (followX/Z, nextTarget) DO fire even
with nil target, but the branch counters do not. Body unchanged (the
existing branch-counter assertion remains correct post-NAI-174).

### 7.3 Bug 1 pin — `TestProcessLogins_LastStepXZ_InitialisedFromOnLogin`

`modules/world/tick_test.go` (or sibling that already exercises
processLogins; verify at plan-time). Fixture: construct a *Server, queue
a *Player into `s.newPlayers` with `p.x = 3200, p.z = 3210, p.level = 0`,
fresh from `newTestPlayer` (lastStepX = -1, lastStepZ = -1).

Call `s.processLogins()`. Assert:
- `p.lastStepX == 3199` (= p.x - 1)
- `p.lastStepZ == 3210` (= p.z)

Pre-NAI-174: FAILS — processLogins doesn't set these.

### 7.4 Integration pin — `TestPlayerFollow_PathToPathingTarget_QueuesValidLeaderCoord`

`modules/world/interaction_test.go` (next to the existing follow-op
tests around `TestFollowOpAnchoredChase` at line 962). Fixture:

- Leader `*Player` at (3220, 3220, 0). Set `leader.lastStepX = 3219;
  leader.lastStepZ = 3220` directly (simulating post-login state — the
  test bypasses processLogins).
- Follower `*Player` at (3225, 3225, 0). Set `follower.lastStepX = 3224;
  follower.lastStepZ = 3225`. Set `follower.target = leader`,
  `follower.targetOp = 3` (raw op-slot 3, per goscape convention at
  `interaction.go:56-163`; `isFollowOp` matches `targetOp == 3 &&
  target.(*Player)`).
- Drain client output via the standard helper.

Tick once: call `leader.processInteraction()` first (no-op for leader
since `leader.target == nil`, but the §5.1 top writes fire →
`leader.followX == 3219`), then `follower.processInteraction()`.

Assert: follower's queued waypoint is `(3219, 3220)` —
`pathToPathingTarget`'s follow-op arm at interaction.go:807 queued to
leader's followX/Z. Pre-NAI-174 the assertion fails because
`leader.followX = -1` from NewPlayer init.

Second sub-test: simulate leader walking one step.
`leader.lastStepX = leader.x; leader.lastStepZ = leader.z;
leader.x = 3221; leader.z = 3220` (mirroring movement.go:140-143's
stepOnce shape). Tick leader (top writes fire → `leader.followX = 3220`).
Tick follower; assert follower's new waypoint is `(3220, 3220)` —
leader's pre-step tile.

### 7.5 Regression — full suite

`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` post each
TDD task. The NilTarget invariant at §7.2 protects the branch-counter
contract; verify no other tests assert "followX/Z unchanged on
no-target processInteraction call" — `grep p.followX modules/world/*_test.go`
returned 0 hits at design time.

### 7.6 Smoke

User-driven Java client (per `smoke_test_server_handoff.md`):

1. Two clients connect. Leader does NOT move after login. Follower
   `/follow`s leader. Expected: follower walks to orthogonal-adjacent
   within 2-3 ticks and stops there. Pre-NAI-174 smoke (NAI-173 close):
   follower stalls ~5 tiles SW.
2. Same two clients, leader walks N. Expected: follower trails 1 tile
   behind leader's current tile (NAI-173 diagonal-reject is the upper
   bound — follower may end orthogonally adjacent, not diagonally).
3. Same two clients, leader stops. Expected: follower stops at
   orthogonal-adjacent without overshoot.

If smoke regresses Target 1: investigate `processLogins` ordering —
maybe LOGIN-trigger script teleports the player so the lastStep init
needs to fire AFTER teleport-by-script. Place the new write per §5.2.

## 8. Risks + premise audit

| Risk | Mitigation |
|---|---|
| Reordering top writes above `p.target == nil` early-return breaks an existing test that asserts "no field mutation on no-target processInteraction" | Pre-flight grep at brainstorm time: only `TestProcessInteraction_CanAccessGate_NilTarget_PostStepSkipped` asserts no-mutation, and it asserts on branch counters (lines 222-224) — NOT followX/Z/nextTarget. Per `risk_register_premise_grep`. |
| New per-tick writes to followX/Z/nextTarget for every targetless player are hot-path | Negligible — three field writes on a struct. The existing per-tick processPathing/processInteractions already touches every player. |
| Bug 1 fix lands at the wrong point in processLogins (LOGIN trigger could teleport before init) | §5.2 places the write AFTER `runScript(LOGIN-trigger)` per TS Player.ts:506-513 ordering. Plan-author verifies via Read of full `processLogins` body — flag if any other `p.x/p.z`-mutating call sits between the LOGIN trigger and where we place the lastStep write. |
| Test fixtures bypass `processLogins` so `p.lastStepX = -1` post-newTestPlayer; new processInteraction top writes now propagate `(-1, -1)` into followX/Z in every fixture | TS-faithful behavior — test fixtures that want a valid followX/Z must set lastStepX/Z themselves (mirroring the production login init). No fixture currently reads followX/Z so this is invisible. |
| NPC-on-player follow path (NPC `targetOp` of equivalent OPPLAYER3 equivalent) hits the *Player type assertion at `interaction.go:802-809` and silently skips | Already TS-faithful per NAI-98-D analysis and §3. NAI-174 does not change this. |
| pathfinder behavior on `queueWaypoint(-1, -1)` is observable in some non-follow path | grep `queueWaypoint(.*-1` and `queueWaypoint.*followX` in modules/world — the only follow-op consumer is `interaction.go:807`. Other queueWaypoint callers pass real coords from MoveClick / scripts. |
| NAI-173 diagonal-reject interacts with follow chase — follower may now stop one tile farther than pre-NAI-173 | TS-faithful contract; smoke Target 2 of NAI-174 confirms. If follower ends diagonally-adjacent (because no orthogonal tile is reachable), the path-flow loops back through pathToPathingTarget which queues to leader.followX again — converges naturally. No new gate needed. |

## 9. Cadence

`compressed_cadence` per `compressed_cadence.md` + `bundle0_short_circuits_stage1_audit.md`:
- Static-confirmable at brainstorm time (both bugs grep+read evidenced).
- ~6 production LOC + ~80 test LOC.
- Single spec+plan combined doc — NO separate plan file. T1+T2+T3 land
  via subagent-driven-development against this doc.

## 10. Deviations

None. This is a TS-fidelity port, not a divergence.

- The NPC-side `followX/Z` field absence stays as the documented
  goscape divergence under the NAI-98-D family (out of scope per §3).
  Already labeled per `defensive_gate_doc_comment_label.md` at the
  `*Player` type-assertion in `interaction.go:802-809`.

## 11. Tech stack

Go 1.26+ per `go_version.md`. No new dependencies. No new file
creation — all edits are in-file at existing sites.

## 12. Task breakdown

### T1 — Bug 2 reorder (interaction.go)

- TDD RED: write `TestProcessInteraction_TopWritesFireUnconditionally`
  per §7.1; verify FAIL on the followX/followZ rows.
- TDD GREEN: move the three top writes above the early-return per §5.1.
- TDD: re-verify the existing NilTarget test still passes; update its
  doc-comment per §7.2.
- Doc-comment: update the processInteraction header per §5.1 final
  paragraph.
- Commit: `feat(world): NAI-174 T1 — port unconditional top writes`

### T2 — Bug 1 login init (tick.go)

- TDD RED: write `TestProcessLogins_LastStepXZ_InitialisedFromOnLogin`
  per §7.3; verify FAIL.
- TDD GREEN: add the two-line `p.lastStepX = p.x - 1; p.lastStepZ = p.z`
  in `processLogins` per §5.2.
- Commit: `feat(world): NAI-174 T2 — port onLogin lastStep init`

### T3 — Integration pin + memory retirement

- Add `TestPlayerFollow_PathToPathingTarget_QueuesValidLeaderCoord`
  per §7.4 — TWO sub-tests (stationary leader, leader-moved-one-step).
- Retire the `NAI-173-FU-FOLLOW-MODE-INVESTIGATION` entry in
  `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
  under "## From NAI-173" — strikethrough + `RETIRED 2026-05-11 by
  NAI-174` annotation. Mirrors NAI-173 T3.4.
- Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` and
  `go vet ./...` — both green.
- Commit: `feat(world): NAI-174 T3 — pin player-follow chase, retire FU`
- Close commit (empty): `chore(close): NAI-174 — follow-mode init,
  retire NAI-173-FU-FOLLOW-MODE-INVESTIGATION` with `Closes memory:
  NAI-173-FU-FOLLOW-MODE-INVESTIGATION` trailer.
