# NAI-7 — NPC Hunt Core

Add the NPC-side hunt skeleton: `huntMode`/`huntRange`/`huntClock`/
`huntTarget` fields on `*Npc`, `processNpcHunt` tick-pass helper,
`huntAll` dispatcher with variant stubs, `OpNpcSetHunt` /
`OpNpcSetHuntMode` opcode wiring, and extension of NAI-5's
`revertType` to reset the new hunt fields.

Part of the NPC AI tick decomposition roadmap. Blockers: NAI-1
(HuntType cache loader), NAI-2 (ActiveNpc interface + turn()
prefix). Roadmap fidelity risk: **Medium**.

## Goal

After NAI-7 ships:

1. NPCs track a hunt state tuple (`huntMode`, `huntRange`, `huntClock`,
   `huntTarget`) seeded from `NpcType.HuntMode` + `NpcType.HuntRange`
   at construction.
2. `processNpcHunt` runs each tick on alive, non-delayed NPCs per
   TS `Npc.ts:158-171` ordering (between isValid gate and regen).
3. `huntAll` dispatcher matches TS `Npc.ts:249-277` — throttled by
   `hunt.Rate`, gated by mode validity, dispatches to four stubbed
   variant functions (`huntPlayers` / `huntNpcs` / `huntObjs` /
   `huntLocs`). Picks `huntTarget` randomly from any hunted slice.
4. Scripts can configure hunt state via `npc_sethunt <range>` and
   `npc_sethuntmode <typeId>` opcodes. `npc_sethuntmode -1` clears.
5. `revertType()` (from NAI-5) now also resets hunt fields, matching
   TS `resetEntity` at lines 309-312.

## Scope

Implements the NAI-7 row of the NAI roadmap. Variants are stubs;
NAI-8 fills `huntPlayers`; NAI-9 fills `huntNpcs`/`huntObjs`/`huntLocs`.

### Non-goals (deferred)

1. **Variant function bodies.** `huntPlayers` / `huntNpcs` /
   `huntObjs` / `huntLocs` return `nil` in NAI-7. NAI-8 adds
   `huntPlayers` (highest fidelity-risk per roadmap due to PAUSEHUNT
   player-exemption + HuntCheck script dispatch); NAI-9 bundles
   the other three.
2. **Real observer counting.** TS checks
   `rsbuf.getNpcObservers(this.nid) > 0` to decide whether to pause
   PAUSEHUNT-mode hunts. Go has no equivalent rsbuf API. NAI-7
   inlines `observers := 1` (stub: always observed) and tracks the
   follow-up. PAUSEHUNT behavior is observationally equivalent to
   KEEPHUNTING until real observer tracking lands.
3. **`consumeHuntTarget`** — transferring `huntTarget` to `target`
   for interaction. NAI-10 scope.
4. **HuntCheck script dispatch** — used by variant functions.
   Bundled with NAI-8.
5. **`NumberNotNull` on popped range** — TS `NPC_SETHUNT` wraps
   `popInt()` in `check(NumberNotNull)`. Tracked audit item.
6. **`npc_changetype` duration wiring** — deferred per roadmap
   note; keeping NAI-7 focused on hunt.

## TS reference

- `Engine-TS/src/engine/entity/Npc.ts:60-67` (field declarations:
  `huntMode: -1`, `huntClock: 0`, `huntrange: 0`, `huntTarget: null`)
- `Engine-TS/src/engine/entity/Npc.ts:101-102` (constructor seeds
  `huntMode` and `huntrange` from NpcType)
- `Engine-TS/src/engine/entity/Npc.ts:158-171` (turn() hunt block)
- `Engine-TS/src/engine/entity/Npc.ts:249-277` (huntAll dispatcher)
- `Engine-TS/src/engine/entity/Npc.ts:309-312` (resetEntity resets
  hunt fields)
- `Engine-TS/src/engine/script/handlers/NpcOps.ts:174-176`
  (NPC_SETHUNT — sets range)
- `Engine-TS/src/engine/script/handlers/NpcOps.ts:178-185`
  (NPC_SETHUNTMODE — sets mode, -1 clears)

## Architecture

### File layout

**Created:**
- `modules/world/npc_hunt.go` — `processNpcHunt` method on `*Server`,
  `huntAll` method on `*Npc`, four variant-stub methods on `*Npc`

**Modified:**
- `modules/world/npc.go` — `+huntMode`, `+huntRange`, `+huntClock`,
  `+huntTarget` fields in a new `// === hunt ===` block; NewNpc
  seeds `huntMode`/`huntRange`; revertType extended with hunt resets;
  `+SetHuntRange` + `+SetHuntMode` methods
- `modules/world/npc_ai.go` — `+s.processNpcHunt(n)` call in
  `Npc.turn()` between isValid gate and `processNpcRegen`
- `pkg/script/active.go` — `ActiveNpc` interface extended with
  `SetHuntRange(int)` and `SetHuntMode(int)`
- `pkg/script/handlers_npc.go` — `+handleNpcSetHunt`,
  `+handleNpcSetHuntMode`
- `pkg/script/handlers.go` — register `OpNpcSetHunt`, `OpNpcSetHuntMode`
- `pkg/script/handlers_npc_test.go` — `mockNpc` adds
  `setHuntRangeCalls`, `setHuntModeCalls` + recording stubs; 4 new
  handler tests
- `pkg/script/handlers_player_test.go` — `mockActiveNpc` adds
  `SetHuntRange`, `SetHuntMode` no-op stubs
- `modules/world/npc_event_queue_test.go` — 6 tests (3 unit + 3
  integration)

### Field additions (`modules/world/npc.go`)

New `// === hunt ===` block, added after the existing `// === interaction ===`
block (or inline with the existing `target entity` / `faceEntity int`
grouping — implementer chooses the most gofmt-friendly location):

```go
	// === hunt ===
	huntMode   int    // -1 = no hunt; else HuntType id
	huntRange  int
	huntClock  int
	huntTarget entity
```

**NewNpc seeds:**

```go
		huntMode:  typ.HuntMode,
		huntRange: int(typ.HuntRange),
```

### `revertType` extension

In NAI-5's `revertType` method on `*Npc`, add at the end (preserving
all existing behavior):

```go
	// NAI-7: hunt-field resets. Matches TS resetEntity at
	// Engine-TS/.../Npc.ts:309-312.
	n.huntClock = 0
	n.huntTarget = nil
	if n.typ != nil {
		n.huntRange = int(n.typ.HuntRange)
		n.huntMode = n.typ.HuntMode
	}
```

### `SetHuntRange` + `SetHuntMode` methods (`modules/world/npc.go`)

```go
// SetHuntRange sets the NPC's hunt search radius. Called by the
// NPC_SETHUNT opcode. Implements script.ActiveNpc.SetHuntRange.
func (n *Npc) SetHuntRange(r int) {
	n.huntRange = r
}

// SetHuntMode sets the NPC's HuntType id. -1 clears the hunt mode.
// Called by the NPC_SETHUNTMODE opcode. Implements
// script.ActiveNpc.SetHuntMode. Mirrors TS NpcOps.ts:178-185 —
// accepts any int; the consumer (processNpcHunt) validates bounds.
func (n *Npc) SetHuntMode(m int) {
	n.huntMode = m
}
```

### `ActiveNpc` interface extension (`pkg/script/active.go`)

Append after existing NAI-2/3/4/6 methods:

```go
	// SetHuntRange sets the NPC's hunt search radius. Called by
	// the NPC_SETHUNT opcode.
	SetHuntRange(r int)

	// SetHuntMode sets the NPC's HuntType id. -1 clears; caller
	// does no bounds validation (the hunt processor validates when
	// looking up the HuntType). Mirrors TS NpcOps.ts:178-185.
	SetHuntMode(m int)
```

### Opcode handlers (`pkg/script/handlers_npc.go`)

```go
// handleNpcSetHunt (NPC_SETHUNT, opcode 2533) sets the NPC's hunt
// range. Despite the opcode name, this sets RANGE only — hunt-mode
// uses the separate NPC_SETHUNTMODE opcode. Mirrors TS
// NpcOps.ts:174-176.
func handleNpcSetHunt(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNT"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntRange(s.PopInt())
	return nil
}

// handleNpcSetHuntMode (NPC_SETHUNTMODE, opcode 2534) sets the
// NPC's HuntType id. -1 clears. Mirrors TS NpcOps.ts:178-185.
func handleNpcSetHuntMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNTMODE"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntMode(s.PopInt())
	return nil
}
```

Registrations in `pkg/script/handlers.go` — alphabetical positions
within the NPC block (after `OpNpcSetTimer`).

### `processNpcHunt` + `huntAll` + variant stubs (`modules/world/npc_hunt.go`)

```go
package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/objtype"
)

// processNpcHunt runs the per-tick hunt pass. Matches TS
// Npc.ts:158-171.
//
// Observer gate: TS checks rsbuf.getNpcObservers(this.nid); Go
// has no equivalent observer-count API yet, so we inline
// `observers := 1` (always observed). PAUSEHUNT semantics are
// currently unobservable — tracked as follow-up in nai_followups
// memory.
//
// Note on mode bounds: SetHuntMode accepts any int (including
// out-of-range values) to match TS. processNpcHunt validates
// bounds against s.huntTypes.Configs and silently no-ops on
// invalid.
func (s *Server) processNpcHunt(n *Npc) {
	if n.huntMode == -1 {
		return
	}
	if s.huntTypes == nil ||
		n.huntMode < 0 ||
		n.huntMode >= len(s.huntTypes.Configs) {
		return
	}
	hunt := s.huntTypes.Configs[n.huntMode]
	if hunt == nil {
		return
	}
	observers := 1 // TODO: rsbuf.GetNpcObservers when available
	if hunt.NobodyNear == objtype.HuntNobodyNearPauseHunt &&
		observers <= 0 &&
		hunt.Type != objtype.HuntModePlayer {
		return
	}
	if hunt.Type != objtype.HuntModePlayer {
		n.huntAll(s, hunt)
	}
	n.huntClock++
}

// huntAll dispatches to a hunted-type variant and sets huntTarget.
// Matches TS Npc.ts:249-277. Variants are stubs at NAI-7; NAI-8
// fills huntPlayers; NAI-9 fills huntNpcs/huntObjs/huntLocs.
func (n *Npc) huntAll(s *Server, hunt *objtype.HuntType) {
	n.huntTarget = nil
	if n.huntClock < hunt.Rate-1 {
		return
	}
	if hunt.Type == objtype.HuntModeOff || n.huntRange < 1 {
		return
	}
	var hunted []entity
	switch hunt.Type {
	case objtype.HuntModePlayer:
		hunted = n.huntPlayers(s, hunt)
	case objtype.HuntModeNpc:
		hunted = n.huntNpcs(s, hunt)
	case objtype.HuntModeObj:
		hunted = n.huntObjs(s, hunt)
	case objtype.HuntModeScenery:
		hunted = n.huntLocs(s, hunt)
	}
	if len(hunted) > 0 {
		n.huntTarget = hunted[rand.IntN(len(hunted))]
	}
}

// huntPlayers is stubbed at NAI-7; NAI-8 fills the body.
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntNpcs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntObjs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntLocs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

### `Npc.turn()` wire (`modules/world/npc_ai.go`)

Insert the hunt call inside the post-isValid section, BEFORE
`processNpcRegen`:

```go
	if n.dead || n.delayed {
		return
	}
	s.processNpcHunt(n)   // NAI-7 — matches TS Npc.ts:158-171
	s.processNpcRegen(n)  // NAI-6
	s.processNpcTimer(n)  // NAI-4
	s.processNpcQueue(n)  // NAI-3
```

### Mock updates

**`mockNpc` (`pkg/script/handlers_npc_test.go`):** add fields +
recording methods:

```go
	setHuntRangeCalls []int
	setHuntModeCalls  []int
```

```go
func (m *mockNpc) SetHuntRange(r int) { m.setHuntRangeCalls = append(m.setHuntRangeCalls, r) }
func (m *mockNpc) SetHuntMode(m2 int) { m.setHuntModeCalls = append(m.setHuntModeCalls, m2) }
```

**`mockActiveNpc` (`pkg/script/handlers_player_test.go`):** add no-op
stubs:

```go
func (m *mockActiveNpc) SetHuntRange(_ int) {}
func (m *mockActiveNpc) SetHuntMode(_ int)  {}
```

## Test strategy

### Handler tests (`pkg/script/handlers_npc_test.go`)

4 tests:

1. **`TestHandleNpcSetHunt`** — happy path: push `range=15`, execute
   `OpNpcSetHunt`, assert `mockNpc.setHuntRangeCalls[0] == 15`.
2. **`TestHandleNpcSetHuntWithoutActiveNpcErrors`** — defensive:
   nil ActiveNpc, expect error `"NPC_SETHUNT: no active npc"`.
3. **`TestHandleNpcSetHuntMode`** — happy path + `-1` clear path:
   set mode to 3, then to -1, assert both recorded.
4. **`TestHandleNpcSetHuntModeWithoutActiveNpcErrors`** — defensive.

### Unit tests (`modules/world/npc_event_queue_test.go`)

3 tests:

5. **`TestNewNpcSeedsHuntFromType`** — `NewNpc` with `HuntMode=3`,
   `HuntRange=5` → `n.huntMode == 3`, `n.huntRange == 5`.
6. **`TestNpcRevertTypeResetsHuntFields`** — modify all 4 hunt
   fields (`huntRange=99, huntMode=0, huntClock=42, huntTarget=nil`
   — actually huntTarget needs an entity stub or just confirm nil);
   call `revertType()`; assert `huntRange` and `huntMode` are reset
   to `typ.HuntRange` / `typ.HuntMode` respectively, `huntClock == 0`,
   `huntTarget == nil`.
7. **`TestNpcSetHuntRangeAndMode`** — direct method tests:
   `n.SetHuntRange(7)` → `n.huntRange == 7`; `n.SetHuntMode(-1)` →
   `n.huntMode == -1`.

### Integration tests (`modules/world/npc_event_queue_test.go`)

3 tests:

8. **`TestProcessNpcHuntSkipsWhenHuntModeNegative`** — `n.huntMode=-1`,
   tick once, assert `n.huntClock == 0` (no-op; no clock increment).
9. **`TestProcessNpcHuntIncrementsClockWhenHuntModeValid`** — build
   `*Server` with `huntTypes` seeded with a HuntType at index 0 having
   `Type=HuntModeOff` (huntAll short-circuits) and
   `NobodyNear=HuntNobodyNearKeepHunting` (gate passes). Set
   `n.huntMode=0`, tick once, assert `n.huntClock == 1`.
10. **`TestProcessNpcHuntPauseHuntRunsWithObserverStub`** — confirms
    the observer stub's effect. Set `NobodyNear=PauseHunt`,
    `Type=HuntModeNpc`, tick once, assert `huntClock == 1` (stub's
    `observers := 1` means PAUSEHUNT gate passes, hunt still runs).
    Serves as a regression guard: when real observer tracking lands
    (replacing the stub), this test's expected value changes to 0
    and documents the stub removal.

## Fidelity notes

1. **`NPC_SETHUNT` sets range only** — despite the misleading opcode
   name, TS `NpcOps.ts:174-176` sets `huntrange`. Go matches.
2. **Observer stub** — `observers := 1` is a documented divergence.
   PAUSEHUNT behavior is currently equivalent to KEEPHUNTING.
3. **Hunt-clock increments AFTER hunt-pass** — matches TS
   `Npc.ts:168-169` (outside the huntAll call but inside the gated
   block).
4. **Player-type hunt skips huntAll call** — matches TS `Npc.ts:164`:
   `if (hunt && hunt.type !== HuntModeType.PLAYER)`. Player hunts in
   TS go through a different dispatch (the `hunt` block at
   `Npc.ts:162` already gates player hunts, and `huntAll` is only
   called for non-player types). Wait — re-reading TS, player hunt
   IS called via huntAll in the switch. Corrected: the outer
   `if (hunt && hunt.type !== HuntModeType.PLAYER)` at line 164 is
   a NON-PLAYER-ONLY guard around huntAll. For player hunts, huntAll
   is NOT called at the turn() level — but player-hunt dispatch
   happens elsewhere? Actually re-reading: line 162 has the gate,
   line 164 has the non-player guard on huntAll, but the huntAll
   dispatcher itself (line 249+) still has a `HuntModePlayer` case
   calling `huntPlayers`. The reconciliation: `Npc.turn()` calls
   `huntAll` only for non-player types; player-type hunts never
   reach `huntAll` via the turn() path. The `HuntModePlayer` case
   in huntAll is reachable only via explicit scripted calls (which
   NAI-7 doesn't wire). Go faithfully replicates both: the
   `if hunt.Type != HuntModePlayer { n.huntAll(s, hunt) }` guard
   at processNpcHunt, AND the `HuntModePlayer` case in huntAll
   (unreachable from NAI-7's turn() path but matching TS structure).
5. **`huntClock` reset in revertType** — TS `Npc.ts:311` sets
   `this.huntClock = 0`. Go matches.

## Rough LOC

- `modules/world/npc.go`: +~25 (4 fields + 2 NewNpc seeds + 2
  methods + revertType extension)
- `modules/world/npc_hunt.go` (new): ~95 (processNpcHunt + huntAll
  + 4 stubs + imports)
- `modules/world/npc_ai.go`: +1 (call line)
- `pkg/script/active.go`: +10 (2 interface methods)
- `pkg/script/handlers_npc.go`: +20 (2 handlers)
- `pkg/script/handlers.go`: +2 (2 registrations)
- `pkg/script/handlers_npc_test.go`: +50 (mock + 4 tests)
- `pkg/script/handlers_player_test.go`: +2 (2 stubs)
- `modules/world/npc_event_queue_test.go`: +120 (6 tests)

Total ≈ 325 LOC. Over the roadmap's ~100 estimate because the
test suite is comprehensive and the mock cascade (2 interface
methods × 2 mocks) is unavoidable.

## Dependencies

- **Blocks:** NAI-8 (huntPlayers), NAI-9 (huntNpcs/Objs/Locs),
  NAI-10 (consumeHuntTarget).
- **Blocked by:** NAI-1 (HuntType loader — the `s.huntTypes`
  registry populated at startup). NAI-2 (ActiveNpc interface +
  runNpcScript infrastructure). NAI-5's `revertType` which NAI-7
  extends.

## Verifications to resolve during plan-write

1. Does Go's `entity` interface (used by `*Npc.target`) have a
   nil-check guard consumers can assume? (`huntTarget = nil` is
   semantically "no target"; downstream needs to handle nil.)
2. Does `mockNpc.SetHuntMode` parameter name collide with the
   `m` receiver? (`m2 int` if yes; alternatively use `mode int`.)
3. Does `*script.Provider` expose HuntType configs, or is
   `s.huntTypes` a world-level field (from NAI-1)? Confirm the
   access path in processNpcHunt.
