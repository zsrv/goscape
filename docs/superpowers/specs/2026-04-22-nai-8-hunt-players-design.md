# NAI-8 — `huntPlayers` Variant

Fill the `huntPlayers` variant stub (from NAI-7) with a filter
pipeline that iterates the player grid in `huntRange` of the NPC,
applies available config-driven filters, and returns the candidate
`[]entity` for `huntAll` to random-pick from.

Part of the NPC AI tick decomposition roadmap. Blocker: NAI-7
(hunt skeleton + variant stubs). Roadmap fidelity risk: **High**
(rating based on assumption of HuntCheck script dispatch —
corrected below).

## Critical memory correction

Before NAI-8: `nai_followups.md` had a "NAI-8 prerequisite:
protected-pointer cleanup in resumeOrFinishNpc" item that said NAI-8
"will route player-anchored scripts through NPC suspension" and
therefore MUST extend `runNpcScript`'s signature to accept an active
player + add protected-pointer cleanup to `resumeOrFinishNpc`.

**That was based on misreading.** TS `Npc.huntPlayers` at
`Engine-TS/.../Npc.ts:921-973` does NOT dispatch any scripts. It's a
config-driven filter pipeline (checkNotBusy, checkAfk,
checkNotTooStrong, checkNotCombat, checkVars, checkInv) where every
filter reads field/varp values and evaluates synchronously — no
`ScriptRunner.init`/`execute` call, no suspension possibility.

The roadmap's shorthand "HuntCheck script fire" was imprecise. NAI-8
ships zero script dispatch.

NAI-8 updates `nai_followups.md` to close the misread item and flag
the real scope concerns.

## Goal

After NAI-8 ships:

1. `huntPlayers(s, hunt)` returns `[]entity` of `*Player`s in
   `huntRange` of the NPC, passing all implemented filters.
2. Integration works via `huntAll`: a PLAYER-type `HuntType`
   invocation (currently unreachable from `Npc.turn()` — turn() skips
   huntAll for PLAYER types; future `OpNpcHuntAll` will reach it)
   selects a random `huntTarget` from the filtered slice.
3. Filter pipeline faithfully matches the ordering of TS
   `Npc.ts:921-973`, with unimplemented filters marked as TODOs
   with line references.

### Reachability note

`huntPlayers` is NOT called from the `Npc.turn()` path. TS and Go
both have `if hunt.Type != HuntModePlayer { huntAll(...) }` at the
turn level (TS `Npc.ts:164`, Go `modules/world/npc_hunt.go:47`).
`huntPlayers` reaches only via:

- `huntAll` with a PLAYER-type HuntType (dispatched from `huntAll`'s
  switch), invoked by explicit script call (e.g.,
  `OpNpcHuntAll` opcode 2526 when wired — not yet)
- Direct test calls

NAI-8 ships the infrastructure; real wiring lands with whatever
sub-spec wires `OpNpcHuntAll`.

## Scope — what's IN

1. Player-grid iteration via `s.grid.NearbyPlayers(x, z, level,
   zoneRadius)` with TS's formula `zoneRadius := 1 + huntRange/8`
2. Range check: `|dx| <= huntRange && |dz| <= huntRange` +
   `p.level == n.level`
3. `checkAfk` filter: `if hunt.CheckAfk && p.IsZonesAfk() { continue }`
   (uses existing `modules/world/afkzone.go:57`)
4. Update `nai_followups.md`: close the misread NAI-2 follow-up,
   rescope the observer-counting item to NAI-9, add a NAI-8
   follow-up listing deferred filters.

## Scope — explicit non-goals (deferred to future audit)

The following TS `huntPlayers` filters are NOT ported; each carries
a TODO comment in the Go source pointing at the TS line:

1. `checkNotBusy` (TS:931-933) — requires `Player.Busy()` method
   equivalent (TS checks active script, modal, interaction, etc.)
2. `checkNotTooStrong` (TS:939-941) — requires wilderness detection
   + NPC `type.vislevel` comparison
3. `checkNotCombat` / `checkNotCombatSelf` (TS:943-948) — varp reads
   with 8-tick combat window; multi-zone exemption
4. `checkVars` (TS:950-957) — varp condition chain using
   `hunt.CheckVars` + `HuntType.CheckHuntCondition` evaluator
5. `checkInv` (TS:959-969) — inventory quantity queries via
   `invTotal` / `invTotalParam`

Each is a separate Go-infrastructure addition. None are NAI-8
blockers — the spec calls them out as deferred, and the regression
tests exercise only the filters NAI-8 implements.

## TS reference

- `Engine-TS/src/engine/entity/Npc.ts:921-973` (huntPlayers body)
- `Engine-TS/src/engine/script/ScriptIterators.ts:36-66` (HuntIterator
  zone-radius formula)
- `Engine-TS/src/engine/entity/Npc.ts:263-264` (huntAll PLAYER case)
- `Engine-TS/src/engine/entity/Npc.ts:162,164` (turn()-level skip of
  PLAYER-type huntAll)

## Architecture

### Files modified

- `modules/world/npc_hunt.go` — replace `huntPlayers` stub body
- `modules/world/npc_event_queue_test.go` — 4 tests
- `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — 3 updates

### `huntPlayers` body

Replace the existing stub:

```go
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

With:

```go
// huntPlayers iterates the player grid in huntRange and returns
// players passing the filter chain. Matches TS Npc.huntPlayers at
// Engine-TS/.../Npc.ts:921-973.
//
// Filter coverage (NAI-8):
//   - Range + level match: always
//   - checkAfk: via p.IsZonesAfk (TS:935-937)
//
// Filters DEFERRED to future audit pass (Go infrastructure
// missing; each TS line cited):
//   - checkNotBusy (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong (TS:939-941)  — wilderness + combat
//   - checkNotCombat (TS:943-945)     — varp+8-tick window
//   - checkNotCombatSelf (TS:946-948) — varp+8-tick window
//   - checkVars (TS:950-957)          — varp condition chain
//   - checkInv (TS:959-969)           — inventory queries
//
// NAI-8 dispatches NO scripts. TS huntPlayers is a config-driven
// filter pipeline, not a script runner (despite the NAI-2 memory
// follow-up's misreading — closed in nai_followups).
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil {
		return nil
	}
	// TS HuntIterator zone-radius formula at ScriptIterators.ts:57:
	// radius = (1 + distance/8) | 0.
	zoneRadius := 1 + n.huntRange/8
	slots := s.grid.NearbyPlayers(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, slot := range slots {
		if slot < 0 || slot >= len(s.players) {
			continue
		}
		p := s.players[slot]
		if p == nil {
			continue
		}
		if p.level != n.level {
			continue
		}
		dx := p.x - n.x
		if dx < 0 {
			dx = -dx
		}
		dz := p.z - n.z
		if dz < 0 {
			dz = -dz
		}
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// checkAfk (TS:935-937): filter players who've gone AFK
		// (1000-tick same-zone threshold).
		if hunt.CheckAfk && p.IsZonesAfk() {
			continue
		}
		hunted = append(hunted, p)
	}
	return hunted
}
```

### Memory updates (`nai_followups.md`)

Three changes:

1. **Close the misread NAI-2 follow-up.** The "NAI-8 prerequisite:
   protected-pointer cleanup in resumeOrFinishNpc" item (lines
   ~17-33) is based on misreading of TS huntPlayers. Replace the
   body with a "Resolved 2026-04-22 (closed as misread)" note
   citing this spec.
2. **Rescope the NAI-7 observer-counting item.** Currently says
   "NAI-8 blocker" — incorrect. PAUSEHUNT observer gate exempts
   PLAYER-type hunts (TS Npc.ts:162), so NAI-8 doesn't need
   observer counting. Rescope to "NAI-9 blocker" (NPC/Obj/Loc
   variants where PAUSEHUNT genuinely gates).
3. **Add NAI-8 follow-up section.** List the 5 deferred filters
   (checkNotBusy, checkNotTooStrong, checkNotCombat/Self, checkVars,
   checkInv) with TS line references and rough remediation notes.

## Test strategy

4 tests in `modules/world/npc_event_queue_test.go`:

1. **`TestHuntPlayersInRange`** — build `*Server` with `s.grid`
   populated; add two players: `pInRange` at `n.x+3, n.z+3,
   n.level` (within `n.huntRange=10`) and `pOutOfRange` at
   `n.x+20, n.z+20, n.level` (outside). Call
   `n.huntPlayers(s, &HuntType{})` directly. Assert `len(hunted) ==
   1` and `hunted[0].Slot() == pInRange.slot`.

2. **`TestHuntPlayersFiltersByLevel`** — two players same (x,z)
   as NPC, one matching `n.level`, one on a different level.
   Assert only the matching-level player is returned.

3. **`TestHuntPlayersSkipsAfkZonedPlayers`** — two players in
   range + same level. One has `IsZonesAfk() == true` (force via
   `p.lastAfkZone = 1000`). `hunt := &HuntType{CheckAfk: true}`.
   Assert only the non-AFK player returned. Separately, with
   `hunt.CheckAfk = false`, assert BOTH players returned (filter
   inactive).

4. **`TestHuntPlayersReturnsEmptyWhenNoCandidates`** — empty grid
   → nil or empty slice returned.

### Test fixture notes

- Building `*Server` with populated `s.grid` + `s.players`
  requires setup beyond existing `newServerForScriptTest`. The
  test adds a helper `addPlayerToServer(s, slot, x, z, level)`
  that sets `s.players[slot]` and calls `s.grid.Add(slot, x, z,
  level)`. Slot 0 is reserved (existing pattern); use slots 1+.
- Players need a minimal `*Player` with `x`, `z`, `level`, `slot`,
  `lastAfkZone` set. No need for client/network state.

## Fidelity notes

1. **Zone radius formula** matches TS `ScriptIterators.ts:57` —
   `(1 + distance/8) | 0` → Go `1 + huntRange/8` (integer
   division rounds toward zero, matching TS `| 0`).
2. **Tile-distance check** via `|dx| <= huntRange && |dz| <=
   huntRange`. TS `HuntIterator` does the same at the entity-level
   distance check (after zone filtering). Chebyshev distance per
   RS2 convention.
3. **Level equality** — TS filters at HuntIterator level (different
   levels skipped). Go's `p.level != n.level` check mirrors.
4. **`checkAfk` semantic** — TS `player.zonesAfk()` at OSRS
   equivalent of 10 minutes idle-in-zone. Go's `IsZonesAfk()` at
   afkzone.go:57 saturates at 1000 ticks (matches "10 min at 600ms/tick").
5. **Filter order** — Go processes range+level first, then checkAfk.
   TS does HuntIterator range first, then checkNotBusy→checkAfk→
   rest. Since NAI-8 skips checkNotBusy, the effective first filter
   is checkAfk — consistent with TS ordering minus the deferred
   filters.

## Rough LOC

- `modules/world/npc_hunt.go`: ~45 (replace 1-line stub with ~45-line
  body)
- `modules/world/npc_event_queue_test.go`: ~130 (4 tests + helper)
- `nai_followups.md`: edits (no new Go code)

Total ≈ 175 LOC. Over the roadmap's ~80 estimate because tests need
grid+server fixture setup that didn't exist in prior NAI sub-specs.

## Dependencies

- **Blocks:** NAI-9 (huntNpcs/Objs/Locs) — will use the same
  zone-radius formula and grid-iteration pattern. NAI-10
  (consumeHuntTarget) — needs `n.huntTarget` to be populated;
  NAI-8 enables that via the `huntAll` → `huntPlayers` path.
- **Blocked by:** NAI-7 (variant stubs + HuntType infrastructure).

## Verifications to resolve during plan-write

1. Does `*Player` satisfy the `entity` interface? (Likely yes —
   NAI-2 had `target entity` on *Npc and *Player was the original
   interaction target. Check `Slot()` and `Coords()` methods
   exist on *Player.)
2. Is `s.players[slot]` safe to read without `s.playersMu` in the
   tick-serial `processNpcs` context? (Yes per existing
   `removeNpc` pattern; confirm during plan.)
3. Does `newServerForScriptTest` set up `s.grid`? (Probably not;
   test helper `addPlayerToServer` may need to `s.grid = grid.New()`
   as well.)
