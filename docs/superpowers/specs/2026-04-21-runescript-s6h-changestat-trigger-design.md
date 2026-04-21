# Sub-spec RuneScript S6h: ChangeStat Trigger Fire — Design

**Status:** Draft → ready for plan
**Scope:** Port TS `Player.changeStat(stat)` (Player.ts:1816-1821). Wire `Player.AddXP` (S6g) to fire the `[changestat,<skill>]` trigger asynchronously via the ENGINE queue when a level-up occurs. Refactor `playerQueueRequest` to hold `*ScriptFile` directly; add sibling `Player.EnqueueScriptFile` method. Shifts ID→ScriptFile resolution from fire-time to enqueue-time. Closes the deferred follow-up flagged at the end of S6g.
**Out of scope:** Any cache script using `[changestat,*]`. Combat-level recomputation on stat-change (separate sub-spec). `QueueEngine` variant of `PlayerQueueType` (TS has ENGINE; goscape uses `QueueNormal` as the closest match until a consumer actually needs the distinction). De-duplication of multi-level-up changeStat fires. NPC-side `AI_CHANGESTAT`-equivalent.

---

## Rationale

S6g's final review explicitly flagged ChangeStat trigger fire as the natural follow-up (recommendation #1). `Player.AddXP` correctly computes level-up math but doesn't dispatch the cache-script event that TS fires at `Player.ts:1772`. Without this, cache scripts that subscribe to `[changestat,<skill>]` — commonly used for level-up messages, stat-milestone unlocks, quest-progress advancement — never run.

The work also resolves a subtle cleanup flagged implicitly by S6g's scope: `Player.EnqueueScriptTyped(scriptID uint32, ...)` takes a script-ID but `Provider.GetByTrigger` returns `*ScriptFile`. Without a sibling API, the changeStat fire would need to reverse-lookup the ScriptFile's index, which is either O(n) (scan `Provider.scripts`) or requires a new ID-bearing field on ScriptFile. The cleanest answer is to route the queue through `*ScriptFile` directly — which the existing `processPlayerQueue` already needs via `GetByID` at fire time. Moving the resolution from fire-time to enqueue-time is a net-simpler code path with the same observable behavior.

## Architecture

```
modules/world/
├── player_script.go           (modify) — playerQueueRequest struct
│     field shape change; EnqueueScriptTyped refactored to resolve
│     ID→ScriptFile via scriptProvider.GetByID then delegate to new
│     EnqueueScriptFile; new Player.changeStat(stat int) helper;
│     Player.AddXP fires changeStat inside its afterBase > beforeBase
│     block
│
├── tick.go                    (modify) — processPlayerQueue uses
│     req.Script directly instead of GetByID(req.ScriptID)
│
└── player_script_test.go      (modify) — 5 new tests for EnqueueScriptFile
      direct path + ChangeStat fire semantics
```

Total **~230 LOC** (production + tests).

## Components

### 1. `playerQueueRequest` struct shape change

`modules/world/player_script.go:16-21` current:

```go
type playerQueueRequest struct {
	ScriptID uint32
	Delay    int
	IntArg   int
	Type     script.PlayerQueueType
}
```

After:

```go
type playerQueueRequest struct {
	Script *script.ScriptFile
	Delay  int
	IntArg int
	Type   script.PlayerQueueType
}
```

The `*script.ScriptFile` pointer becomes the single source of truth. ID-to-file resolution moves from `processPlayerQueue` (fire-time) to `EnqueueScriptTyped` (enqueue-time).

### 2. New `Player.EnqueueScriptFile`

Sibling to `EnqueueScriptTyped`. Stores the ScriptFile directly:

```go
// EnqueueScriptFile appends a queued fresh-run request for a specific
// ScriptFile. Delay=0 fires on the next processActiveScripts pass (subject
// to the STRONG/NORMAL gate in processPlayerQueue). Nil sf is a silent
// no-op — engine dispatchers (e.g. changeStat) call GetByTrigger and may
// legitimately pass nil when no cache script is registered for the event.
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay, intArg int, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	p.queue = append(p.queue, playerQueueRequest{
		Script: sf,
		Delay:  delay,
		IntArg: intArg,
		Type:   qtype,
	})
}
```

### 3. `Player.EnqueueScriptTyped` refactor

Same public signature (QUEUE opcode handler unchanged). Internal delegation:

```go
// EnqueueScriptTyped implements script.ActivePlayer.EnqueueScriptTyped by
// resolving scriptID → *ScriptFile via scriptProvider.GetByID and
// delegating to EnqueueScriptFile. Silent no-op on missing script or
// unwired server — same observable contract as the pre-S6h impl, where
// processPlayerQueue's GetByID check served the same role.
//
// Resolution shifts from fire-time (pre-S6h) to enqueue-time (S6h).
// Same tick boundary in practice; simpler codepath.
func (p *Player) EnqueueScriptTyped(scriptID uint32, delay, intArg int, qtype script.PlayerQueueType) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	p.EnqueueScriptFile(p.client.server.scriptProvider.GetByID(scriptID), delay, intArg, qtype)
}
```

### 4. New `Player.changeStat` helper

TS port of Player.ts:1816-1821:

```go
// changeStat fires the [changestat,<skill>] trigger for the given stat
// slot when a cache script is registered. Enqueued as QueueNormal so it
// runs asynchronously through processPlayerQueue, not inline with the
// triggering action. Matches TS Player.changeStat (Player.ts:1816-1821)
// which uses PlayerQueueType.ENGINE — goscape's closest match is
// QueueNormal (same tick-later semantics, same delayed-player gating).
//
// Silent no-op if no script is registered (GetByTrigger returns nil →
// EnqueueScriptFile's nil-check short-circuits). Called from AddXP's
// level-up branch.
func (p *Player) changeStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTrigger(script.TriggerChangeStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
}
```

### 5. `Player.AddXP` — fire on level-up

Inside the existing `afterBase > beforeBase` block, add the `changeStat(id)` call. The drain-replenish branch is a sibling condition — both gate on `afterBase > beforeBase`:

```go
func (p *Player) AddXP(id int, xp int) {
	// ... existing body ...

	if afterBase > beforeBase && int(p.levels[id]) < beforeBase {
		// Drained + level-up: replenish levels by the level delta.
		p.levels[id] = uint8(min(int(p.levels[id])+(afterBase-beforeBase), 255))
	}
	if afterBase > beforeBase {
		// Level-up: fire the [changestat,<skill>] trigger if registered.
		// Matches TS Player.ts:1772.
		p.changeStat(id)
	}
}
```

Note the two branches share the `afterBase > beforeBase` guard but are independent — a non-drained level-up still fires changeStat. TS has them similarly separate inside its outer `if` block (Player.ts:1766-1772).

### 6. `processPlayerQueue` simplification

`modules/world/tick.go:219-245` current body uses `GetByID(req.ScriptID)` at fire time. Simplify to use the pre-resolved `req.Script`:

```go
func (s *Server) processPlayerQueue(p *Player) {
	i := 0
	for i < len(p.queue) {
		req := &p.queue[i]
		req.Delay--
		if req.Delay > 0 {
			i++
			continue
		}
		// STRONG queue fires even when delayed; others wait for idle.
		if p.delayed && req.Type != script.QueueStrong {
			i++
			continue
		}
		sf := req.Script
		intArg := req.IntArg
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		if sf != nil {
			s.runScript(sf, p, false, []int{intArg}, nil)
		}
		// Don't advance i — we just removed the current element.
	}
}
```

Two lookups at enqueue-time become zero lookups at fire-time. Net speedup and removed redundant provider-nil-check.

## Data flow (happy-path level-up with changeStat script registered)

1. Cache script `[changestat,attack]` loaded at startup; `ScriptFile.LookupKey = (165 | (0x2<<8) | (0<<10)) = 0xA5` for Attack stat.
2. Player trains Attack; AddXP(PlayerStatAttack, 1000) called on tick N. Crosses level-2 threshold → `afterBase=3 > beforeBase=2`.
3. `p.changeStat(0)` called. `GetByTrigger(TriggerChangeStat, 0, -1)` returns the registered ScriptFile.
4. `EnqueueScriptFile(sf, 0, 0, QueueNormal)` appends a `playerQueueRequest{Script: sf, Delay: 0, ...}` to `p.queue`.
5. On tick N+1, `processPlayerQueue(p)` decrements Delay (0 → -1), pops the entry, runs the script via `s.runScript(sf, p, false, []int{0}, nil)`.
6. The `[changestat,attack]` script runs with `state.Self = p`; can use `STAT(3)` to read the new HP, `STAT_BASE(0)` to read the new Attack level, `MES` to announce "Congratulations, you just advanced an Attack level!" etc.

## Edge cases

| # | Case | Handling |
|---|---|---|
| 1 | AddXP crosses a level threshold, script registered | changeStat fires → EnqueueScriptFile appends queue entry |
| 2 | AddXP without level-up | outer `afterBase > beforeBase` guard fails → changeStat not called |
| 3 | Level-up + no registered script | GetByTrigger returns nil → EnqueueScriptFile no-ops on nil |
| 4 | Player unwired (nil `client.server`) | changeStat early-returns on nil guard |
| 5 | Multi-level-up in one call (AddXP 100000 → 5+ levels) | Single changeStat call — fires once per AddXP invocation regardless of level delta. Matches TS; de-dup is the script's responsibility if desired. |
| 6 | changeStat fires from within a running script (e.g., script → AddXP → changeStat) | Queue grows mid-tick; processPlayerQueue picks up the new entry next tick. Async boundary preserved. |
| 7 | EnqueueScriptTyped with invalid ID | GetByID returns nil → EnqueueScriptFile no-ops. Same observable behavior as pre-refactor. |
| 8 | p.delayed == true when changeStat fires | Queue entry enqueued as QueueNormal; processor gates on `p.delayed` until clear. TS ENGINE has same gating. |
| 9 | TriggerChangeStat lookup with stat out of range | GetByTrigger doesn't validate stat values — encodes into lookup key. If someone happened to register at that key, it'd fire; otherwise no-op. Matches TS. |

## Testing strategy

### New tests in `modules/world/player_script_test.go`

| Test | Asserts |
|---|---|
| `TestEnqueueScriptFileDirectPath` | `EnqueueScriptFile(sf, 3, 42, QueueNormal)` appends exactly one entry with matching Script / Delay / IntArg / Type fields |
| `TestEnqueueScriptFileNilIsNoop` | `EnqueueScriptFile(nil, 0, 0, QueueNormal)` doesn't append |
| `TestAddXPFiresChangeStatOnLevelUp` | Register `[changestat,attack]` at trigger-specific key; AddXP crosses level; assert queue has exactly one additional entry, and `queue[N].Script` is the registered ScriptFile, `Type == QueueNormal` |
| `TestAddXPDoesNotFireChangeStatWithoutLevelUp` | Register the script; AddXP with stats below the level-2 threshold; assert queue length unchanged |
| `TestAddXPChangeStatNoScriptIsNoop` | NO script registered; AddXP with level-up; assert queue length unchanged AND `baseLevels` still advanced correctly (level-up math independent of changeStat fire) |

The `AddXP*ChangeStat*` tests use the `before := len(p.queue)` pattern to account for any pre-existing queue entries the fixture might seed (today it doesn't, but the pattern is idiom-stable across future fixture changes).

### Regression tests that remain green

- `TestEnqueueScriptTyped*` paths in `script_test.go:239, 269, 336, 337` — the refactored `EnqueueScriptTyped` resolves ID via `GetByID` and delegates to `EnqueueScriptFile`. Tests register scripts via `RegisterAt(id, ...)` before enqueueing, so `GetByID(id)` returns the expected ScriptFile.
- `TestStatAdvanceViaScript` (script_test.go:511) — AddXP(3, 50) adds to `stats[3] = 100 → 150`; 150 XP is below the level-2 threshold, so the outer guard fails and `changeStat` doesn't fire. Assertion `stats[3] == 150` still holds.
- `processPlayerQueue` tests — observable behavior identical; the simplification to `req.Script` doesn't change what fires when.

## LOC estimate

| File | LOC |
|---|---|
| `modules/world/player_script.go` (diff) | +35 / -10 |
| `modules/world/tick.go` (diff) | +3 / -6 |
| `modules/world/player_script_test.go` (diff) | +100 |
| **Total** | **~230** |

## Key design calls

- **Resolve-at-enqueue-time.** Simpler than the "both fields" approach. Same observable contract because queue entries are fired within a single tick boundary of enqueue; a script disappearing mid-tick would be a real bug, not a timing nuance to preserve.
- **Two public enqueue methods (sibling, not replacement).** `EnqueueScriptFile` for engine dispatch paths that already have a ScriptFile; `EnqueueScriptTyped` for QUEUE-opcode callers that have an ID. Matches the TS layering (low-level `enqueueScript(script, type)` vs opcode-handler indirection).
- **`QueueNormal` for changeStat, not `QueueStrong`.** TS uses ENGINE which is async-but-respects-delay-gating. QueueNormal has the matching gating semantics (STRONG bypasses delay; Normal waits). Adding a dedicated `QueueEngine` variant would be over-engineering until a consumer behavioral delta appears.
- **No `QueueEngine` vs `QueueNormal` renaming.** Preserves existing PlayerQueueType enum values used by handler code from S5h.
- **One changeStat fire per AddXP, not per level crossed.** Matches TS exactly. If a cache script wants per-level processing, it can check `before` vs `after` in its own body via `STAT_BASE(stat)`.
- **`changeStat` is unexported (lowercase).** Script-handler layer doesn't call it directly; only `AddXP` does. Keeping it unexported signals "internal engine dispatch" and lets us rename freely in future.

## Gotchas

- **`EnqueueScriptTyped`'s new nil guard.** Pre-S6h, the nil-server case was handled downstream in `processPlayerQueue` (`if s.scriptProvider != nil`). Post-S6h, `EnqueueScriptTyped` must short-circuit when `p.client.server.scriptProvider` is nil — otherwise it panics. The guard is three-deep (`p.client`, `.server`, `.scriptProvider`) to match the pattern in `SetDelayed` and other engine-dispatch helpers.
- **Queue processing test fixtures.** Tests that construct `playerQueueRequest{ScriptID: 0xAAAA}` directly (not via `EnqueueScriptTyped`) break with the struct change. Grep confirms none exist today — all test enqueues go through the public method.
- **`ActivePlayer.EnqueueScriptTyped` interface.** The `script.ActivePlayer` interface signature remains unchanged (still takes `scriptID uint32`). The interface exists for script handlers to abstract over the player implementation; it doesn't need to expose the new file-direct API. If a future handler wants direct file enqueue, extend the interface then.
- **The TS `changeStat` is actually called from multiple places** beyond advanceStat. In the TS source, `changeStat` is also called from boost/drain ops when the skill level changes. S6h ports only the AddXP call site because that's what S6g unblocked. Future boost/drain sub-spec can re-use the same `Player.changeStat` helper — the API is designed for exactly that reuse.
