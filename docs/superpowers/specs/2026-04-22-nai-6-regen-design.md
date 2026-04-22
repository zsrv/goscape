# NAI-6 — NPC Stat Regen (`processRegen`)

Add `regenInterval`/`regenClock` fields on `*Npc` and a `processNpcRegen`
helper that ticks the regen clock, reloads the interval from
`NpcType.RegenRate` when it fires, and moves `curHP` one step toward
`baseHP`. Wire into `Npc.turn()` between the isValid gate and
`processNpcTimer`, matching TS order: regen → timer → queue.

Part of the NPC AI tick decomposition roadmap. Blocker: NAI-2.
Roadmap fidelity risk: **Medium**.

## Goal

After NAI-6 ships:

1. NPCs tick `regenClock` each turn (unconditionally while alive + not
   delayed — the isValid gate added in NAI-5 handles those conditions
   externally).
2. When `regenClock >= regenInterval`:
   - Interval reloads from `NpcType.RegenRate` (matches the TS
     Vorkath-changetype quirk — regenrate takes effect on fire, not
     on changetype).
   - `regenClock` resets to 0.
   - `curHP` moves one step toward `baseHP` (+1 if low, -1 if high,
     unchanged if equal).
3. `NewNpc` seeds `regenInterval` from `typ.RegenRate` (default 100
   ticks per `NewNpcType` — that's 60 seconds at 600ms/tick).

## Scope

Implements the NAI-6 row of the NAI roadmap.

### Non-goals

1. **Full 6-stat regen.** TS `processRegen` walks `baseLevels[]` /
   `levels[]` arrays with 6 entries per NPC (attack, defence,
   strength, hitpoints, ranged, magic). Go's `*Npc` currently uses
   only `curHP` / `baseHP` — a single-stat subset. Porting the full
   6-stat arrays requires adding the arrays themselves + updating
   `NewNpc`, `revertType`, `initialHP` (which would become
   `initialStats`), plus any existing NAI-2/3/5 code paths that
   touch stats. Out of scope; land with NAI-7 or a dedicated
   stats-subsystem sub-spec.
2. **Prayer regen.** The NAI roadmap row mentions "HP + prayer + stat
   regen" but prayer is a player-only stat in TS; NPCs have no
   prayer. Roadmap phrasing was imprecise. Out of scope.
3. **Mask-raising on HP change.** TS `processRegen` does NOT raise
   any mask when stats change — the rsbuf encoder reads `curHP`
   directly. Go stays faithful; no new masks.
4. **`regenClock` / `regenInterval` reset in `revertType`.** TS
   `resetEntity` (the revertType parallel) does NOT reset regen
   fields. The Vorkath quirk at TS `Npc.ts:507-511` intentionally
   reloads `regenInterval` on fire rather than on changetype.
   Go preserves this.
5. **Internal `!n.delayed` gate.** TS `processRegen` has no internal
   delayed gate; TS's `turn()` calls it only after the isValid
   early-return. Go's equivalent is the existing
   `if n.dead || n.delayed { return }` at `npc_ai.go` after the
   Events block. `processNpcRegen` needs no internal gate.

## TS reference

- `Engine-TS/src/engine/entity/Npc.ts:62-63` (field declarations —
  `regenInterval` starts at 0, `regenClock` starts at 0)
- `Engine-TS/src/engine/entity/Npc.ts:176` (turn() call site)
- `Engine-TS/src/engine/entity/Npc.ts:505-525` (processRegen body,
  including the Vorkath-comment explaining the reload-on-fire quirk)

## Architecture

### File layout

**Modified:**
- `modules/world/npc.go` — `+regenInterval`, `+regenClock` fields;
  `NewNpc` seeds `regenInterval = int(typ.RegenRate)`
- `modules/world/npc_script.go` — `+processNpcRegen` method on
  `*Server`
- `modules/world/npc_ai.go` — `+s.processNpcRegen(n)` call in `turn()`
  between isValid gate and `processNpcTimer`
- `modules/world/npc_event_queue_test.go` — 5 tests (append; matches
  the "NPC-tick tests live here" pattern NAI-5 established)

No new files.

### Field additions

In `modules/world/npc.go`, within the `// === lifecycle ===` block
(or a new `// === regen ===` block — bikeshed; the NAI-5 `baseType`
field lives in the lifecycle block so `regenInterval` / `regenClock`
fitting alongside makes sense), add after `baseType`:

```go
	// === lifecycle ===
	lifecycle                  int
	lifecycleTick              int
	respawnRate                int
	dead                       bool
	startX, startZ, startLevel int
	baseType                   int
	regenInterval              int
	regenClock                 int
```

**NewNpc seed:** add one line to the struct literal:

```go
		respawnRate:     int(typ.RespawnRate),
		timerInterval:   int(typ.Timer),
		regenInterval:   int(typ.RegenRate),
```

### `processNpcRegen` method (`modules/world/npc_script.go`)

Appended (after `processNpcTimer` from NAI-4):

```go
// processNpcRegen ticks the regen clock and, on interval elapse,
// reloads the interval from NpcType.RegenRate and moves curHP one
// step toward baseHP. Matches TS Npc.processRegen at
// Engine-TS/.../Npc.ts:505-525.
//
// Behaviour:
//   - regenClock increments unconditionally when called (TS has no
//     internal delayed gate — the caller's isValid check handles it;
//     in Go, Npc.turn()'s "if n.dead || n.delayed { return }" gate
//     before this call provides the same guarantee).
//   - When regenClock hits regenInterval: reload regenInterval from
//     n.typ.RegenRate (matches TS's Vorkath-changetype quirk where
//     the new rate takes effect on fire, not on changetype), reset
//     clock to 0, and move curHP one step toward baseHP.
//   - Single-stat (HP-only) for now; full 6-stat array regen
//     deferred until stat arrays land on *Npc.
func (s *Server) processNpcRegen(n *Npc) {
	n.regenClock++
	if n.regenClock < n.regenInterval {
		return
	}
	if n.typ != nil {
		n.regenInterval = int(n.typ.RegenRate)
	}
	n.regenClock = 0
	switch {
	case n.curHP < n.baseHP:
		n.curHP++
	case n.curHP > n.baseHP:
		n.curHP--
	}
}
```

### `Npc.turn()` wire (`modules/world/npc_ai.go`)

Insert the regen-pass call immediately AFTER the `isValid` gate
(`if n.dead || n.delayed { return }`) and BEFORE the existing
`s.processNpcTimer(n)` call. Final post-gate section:

```go
	// === isValid gate (NAI-5) ===
	if n.dead || n.delayed {
		return
	}

	// === Regen + timer + queue (NAI-6, NAI-4, NAI-3) ===
	s.processNpcRegen(n) // NAI-6 — matches TS Npc.ts:176
	s.processNpcTimer(n)
	s.processNpcQueue(n)
```

## Test strategy

All tests in `modules/world/npc_event_queue_test.go` (appending).

1. **`TestNewNpcSeedsRegenInterval`** — unit: build NpcType with
   `RegenRate: 7`, call `NewNpc`, assert `n.regenInterval == 7`.

2. **`TestProcessNpcRegenIncrementsClock`** — unit: build `*Server` +
   `*Npc` with `regenInterval=100`, `regenClock=0`, call
   `s.processNpcRegen(n)` once, assert `n.regenClock == 1`, curHP
   unchanged.

3. **`TestProcessNpcRegenFiresAtInterval`** — unit: `regenInterval=3`,
   `regenClock=0`, `curHP=5`, `baseHP=10`, call 3 times. After call
   3: `regenClock == 0` (reset), `curHP == 6` (incremented by 1).
   Also seed `n.typ.RegenRate = 99` before the 3rd call to confirm
   the reload: `n.regenInterval == 99` after.

4. **`TestProcessNpcRegenClampsAtBaseHP`** — `regenInterval=3`,
   `curHP=10`, `baseHP=10`, call 3 times, assert `curHP == 10`
   (no change — neither `<` nor `>` branch fires).

5. **`TestProcessNpcRegenDecrementsWhenAboveBase`** — `regenInterval=3`,
   `curHP=12`, `baseHP=10`, call 3 times, assert `curHP == 11`
   (decremented toward base).

## Fidelity notes

1. **Interval reload on fire** (TS `Npc.ts:508-511` Vorkath comment):
   matches the TS behaviour that changetype doesn't immediately
   change regen rate; the new rate takes effect on the next regen
   fire. Go preserves exactly.
2. **`curHP` move by 1 per fire** (TS `Npc.ts:518-522`): matches.
3. **No internal delayed gate**: matches TS. The `Npc.turn()` isValid
   gate handles it externally.
4. **No revertType reset**: TS `resetEntity` doesn't touch regen
   fields; Go matches.

## Rough LOC

- `modules/world/npc.go`: +3 (2 fields + 1 NewNpc line)
- `modules/world/npc_script.go`: +25 (processNpcRegen)
- `modules/world/npc_ai.go`: +1 (call line + comment inline)
- `modules/world/npc_event_queue_test.go`: +80 (5 tests)

Total ≈ 110 LOC. Slightly over the roadmap's ~60 estimate because
the tests are thorough (5 cases cover increment-only, fire-at-interval
with type-reload, clamp-at-equal, and decrement-from-above).

## Dependencies

- **Blocks:** NAI-7 (hunt core) — not strictly (NAI-7 doesn't touch
  regen), but NAI-7's eventual stat-array port will consume
  `processNpcRegen` as the extension point for the 6-stat walk.
- **Blocked by:** NAI-2 (runNpcScript infra not used here, but
  processNpc* method-on-*Server pattern established there). NAI-5's
  isValid gate is the external `!n.dead && !n.delayed` guard that
  lets processNpcRegen skip the internal check.
