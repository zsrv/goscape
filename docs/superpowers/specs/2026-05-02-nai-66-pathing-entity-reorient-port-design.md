# NAI-66 — pathing-entity reorient port + permanent NPC stride/jump dead-API closure

**Status:** Spec, awaiting plan.
**Date:** 2026-05-02
**Closes:** `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`.
**Reframes (doc-only):** `NAI-34-D4-NPC`, `NAI-34-D5-NPC`.
**Net deviation tally:** 14 (post-NAI-65) → **13** (post-NAI-66).
**Carry-forward source:** NAI-65 close note, "pathing-entity-reorient-and-stride-tracking" sub-spec.

## Tech Stack

Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS` (per `ts_source_canonical_path.md`). No Rust rsbuf code touched in this sub-spec.

## 1. Background

NAI-65 closed `NAI-34-D3` (focus on Teleport) for both entities and `NAI-34-D4-Player` (lastStepX/Z on Player.Teleport). The natural follow-on is `(*Player).reorient()` and `(*Npc).reorient()` — the per-tick refocus method invoked from TS `World.processInfo` (`World.ts:995, 1046`). Porting `reorient()` materializes a consumer for `Player.targetX/Z`, which closes `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ` by activating the default arm of `Player.SetInteraction` (Loc/Obj fine-coord cache).

The two remaining NAI-34 NPC items were originally framed as "blocked on consumer." Re-audit at NAI-66 spec-write reveals they are **dead-API in TS itself**:

- `lastStepX/Z`: TS PathingEntity sets these on every step + teleport, but the only reader (`Player.ts:1201-1202` `followX/Z` setup) is Player-scoped. No NPC reader.
- `jump`: TS PathingEntity sets `npc.jump = true` on level-change, but no NPC encoder consumes it. Rust upstream `rsbuf::Npc` (`2004scape/rsbuf` branch 225, `src/npc.rs:3-29`) has no `jump` field. PlayerInfoEncoder reads `player.jump` but NpcInfoEncoder has no symmetric branch.

Per `dead_api_polish.md`, goscape skips both writes. Reframing to "permanent dead-API skip" is the honest characterization; tags remain to preserve grep-discoverability for future audits.

## 2. Scope

### 2.1 Active port

1. Add `(*Player).reorient()` mirroring `PathingEntity.ts:349-361`.
2. Add `(*Npc).reorient()` mirroring same.
3. Activate `Player.SetInteraction` default arm (`modules/world/interaction.go:92-99`) to write `targetX = fine(t.x, width)`, `targetZ = fine(t.z, length)` for `*Loc`/`*Obj` targets via the existing `targetWidthLength` helper at `modules/world/npc_interaction.go:687`.
4. Wire `reorient()` invocation into `Server.processInfo` (`modules/world/tick.go:329`):
   - Player reorient loop before `ComputePlayers`.
   - Npc reorient loop before `ComputeNpcs`.

### 2.2 Doc-only reframe (close commit)

Update `nai_followups.md` carry-forward entries for `NAI-34-D4-NPC` and `NAI-34-D5-NPC` to "**permanent dead-API skip; closure requires upstream-TS NPC consumer materializing.**" Update DEVIATION block doc-comments at any tag sites that still claim "blocked on consumer materializing" framing.

### 2.3 Out of scope

- `(*Npc).Teleport` writes for `lastStepX/Z` and `jump` — fields would remain unconsumed; per `dead_api_polish.md` not ported.
- Adding `lastStepX/Z` or `jump` fields to Npc struct — same.
- `rsbuf::Npc.jump` field + `npcinfo` encoder branch — Rust upstream does not have it; would require divergence from `2004scape/rsbuf`.
- Any other PathingEntity method ports (`validateDistanceWalked`, etc.).

## 3. TS Source Mapping

```
LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts:349-361
  → modules/world/movement.go (*Player).reorient
  → modules/world/npc_interaction.go (*Npc).reorient

LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts:542-545
  (already mirrored at npc_interaction.go:680-683 for Npc)
  → modules/world/interaction.go default arm (Player target switch)

LostCityRS/Engine-TS/src/engine/World.ts:995
  → modules/world/tick.go processInfo, Player loop, before ComputePlayers

LostCityRS/Engine-TS/src/engine/World.ts:1046
  → modules/world/tick.go processInfo, Npc loop, before ComputeNpcs
```

## 4. Method Shape

### 4.1 `(*Player).reorient`

Place in `modules/world/movement.go` adjacent to existing Player-movement helpers.

```go
// reorient is the per-tick refocus invoked from Server.processInfo before
// rsbuf compute. Mirrors TS PathingEntity.reorient at
// Engine-TS/src/engine/entity/PathingEntity.ts:349-361.
//
// PathingEntity targets (Player/Npc) are refocused on the target's current
// position (target may have moved this tick). Non-pathing targets (Loc/Obj)
// trigger one-shot focus + clear of the cached fine-coord (targetX/Z) iff
// the player took zero steps this tick — semantically "the entity moved off
// while we were trying to reach it."
func (p *Player) reorient() {
	switch t := p.target.(type) {
	case *Player:
		p.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		p.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	default:
		if p.targetX != -1 && p.stepsTaken == 0 {
			p.focus(p.targetX, p.targetZ, false)
			p.targetX = -1
			p.targetZ = -1
		}
	}
}
```

### 4.2 `(*Npc).reorient`

Place in `modules/world/npc_interaction.go` near `(*Npc).focus`. Structurally identical:

```go
// reorient — Npc-side per-tick refocus. Mirrors TS PathingEntity.reorient.
func (n *Npc) reorient() {
	switch t := n.target.(type) {
	case *Player:
		n.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		n.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	default:
		if n.targetX != -1 && n.stepsTaken == 0 {
			n.focus(n.targetX, n.targetZ, false)
			n.targetX = -1
			n.targetZ = -1
		}
	}
}
```

### 4.3 `Player.SetInteraction` default arm

Replace the DEVIATION block at `modules/world/interaction.go:92-99`. Unlike `Npc.SetInteraction`, the Player version does not pre-compute target coords above the switch — `tx, tz` must be retrieved inline in the default arm via the `entity` interface's `Coords()` method (verified in use at `npc_interaction.go:660` `tx, tz, _ := target.Coords()`).

```go
	default:
		// Loc/Obj target — cache fine-coord for reorient consumption.
		// TS PathingEntity.ts:542-545.
		tx, tz, _ := t.Coords()
		tw, tl := targetWidthLength(t)
		p.targetX = coordgrid.Fine(tx, tw)
		p.targetZ = coordgrid.Fine(tz, tl)
```

`targetWidthLength` is already in the same package (`modules/world/npc_interaction.go:687`). `coordgrid.Fine` is the canonical fine-coord helper. Plan-author should confirm at preflight that `entity.Coords()` returns `(int, int, int)` matching the destructure pattern.

### 4.4 `processInfo` wire point

Insert in `modules/world/tick.go:processInfo` (~line 329, top of body after the players-snapshot copy):

```go
	// NAI-66: TS World.ts:995 — per-tick refocus before rsbuf compute.
	for _, p := range players {
		p.reorient()
	}
```

Symmetric for npcs, before `s.renderer.ComputeNpcs(npcSources)`:

```go
	// NAI-66: TS World.ts:1046 — npc-side per-tick refocus.
	for _, n := range s.npcLoop {
		n.reorient()
	}
```

Both loops run before rsbuf compute so that any cleared `targetX/Z` and any new `faceAngleX/Z` from focus() are observed by the rsbuf state push.

## 5. Lifecycle of `targetX/Z`

- **Init:** `-1` (already at `player.go:389-390` and `npc.go:180-181`).
- **Write:** `SetInteraction` default arm (Loc/Obj branch) overwrites unconditionally.
- **Read + clear:** `reorient()` is the only consumer; clears to `-1` after focus.
- **NOT cleared in:** `ClearInteraction` (TS does not clear them there; preserved as-is — no divergence introduced).

## 6. Testing Strategy (per `plan_test_coverage_crosscheck.md`)

### 6.1 Player.reorient — 5 cases

Add to `modules/world/player_interaction_test.go` (or a dedicated `player_reorient_test.go` if file size grows large).

1. **PathingEntity target = *Player** — pre: target is a *Player at (X, Z); post: `faceAngleX == fine(t.x, 1)`, `faceAngleZ == fine(t.z, 1)`; `targetX/Z` unchanged (still -1 from init).
2. **PathingEntity target = *Npc** — symmetric.
3. **Default arm: Loc target with `stepsTaken==0`** — pre: target is a *Loc, `targetX = 999, targetZ = 1001, stepsTaken = 0`; post: `faceAngleX == 999, faceAngleZ == 1001, targetX == -1, targetZ == -1`.
4. **Default arm: Loc target with `stepsTaken > 0`** — pre: same Loc state, `stepsTaken = 3`; post: state unchanged (no focus, no clear).
5. **`target == nil` no-op** — pre: target = nil, targetX/Z = -1, stepsTaken = 0; post: state unchanged; no panic.

### 6.2 Npc.reorient — 5 cases

Symmetric in `modules/world/npc_interaction_test.go`. Same five branches against `(*Npc).reorient`.

### 6.3 Player.SetInteraction default arm — 2 cases

Add to `modules/world/player_interaction_test.go` near existing SetInteraction tests.

1. **Loc target with non-1x1 dimensions** — `Loc{X:50, Z:60, Width:3, Length:2}`; assert `p.targetX == fine(50, 3)`, `p.targetZ == fine(60, 2)`.
2. **Obj target (always 1x1)** — `Obj{X:50, Z:60, Type:42}`; assert `p.targetX == fine(50, 1)`, `p.targetZ == fine(60, 1)`.

### 6.4 processInfo wire-test — 1 case

Add to `modules/world/rsbuf_per_tick_test.go`. Pre-tick: a *Player with a *Loc target, `targetX = 999, targetZ = 1001, stepsTaken = 0`. Run `s.processInfo()`. Post-tick: `p.targetX == -1, p.targetZ == -1, p.faceAngleX == 999, p.faceAngleZ == 1001`. Pins both ordering (reorient runs before ComputePlayers) and that the player loop does invoke reorient.

### 6.5 Test counts

Player: 5 reorient + 2 SetInteraction = 7 cases.
Npc: 5 reorient = 5 cases.
processInfo: 1 wire test.
**Total: 13 new test cases.**

## 7. Deviations Closed / Reframed

### 7.1 Closed

- **`NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`** — Player.SetInteraction default arm now writes `targetX/Z`; consumer (`reorient`) materialized in this sub-spec. Tag site at `modules/world/interaction.go:92-99` removed (DEVIATION block deleted; replaced by active write).

### 7.2 Reframed (no code change)

- **`NAI-34-D4-NPC`** — From "blocked on consumer" → "**permanent dead-API skip.** TS PathingEntity sets `lastStepX/Z` on Npc, but no TS reader consumes them on NPC instances (only Player.ts:1201-1202 reads `lastStepX/Z` for `followX/Z` setup). Closure: requires upstream-TS NPC consumer to materialize."
- **`NAI-34-D5-NPC`** — From "blocked on `rsbuf.Npc.Jump` field + npcinfo encoder branch" → "**permanent dead-API skip.** TS PathingEntity sets `npc.jump = true` on level-change, but no TS NPC encoder reads it. Rust upstream `rsbuf::Npc` (branch 225, `src/npc.rs:3-29`) has no `jump` field. Closure: requires upstream-TS NPC encoder consumer to materialize."

Tags remain on existing DEVIATION-block doc-comments at the tag sites (`modules/world/npc_script.go:107-121`, `pkg/script/active.go:613-621`, etc.) for grep-discoverability per `retire_deviation_grep_all_comments.md`. The tag-site doc-comment narrative is updated in the same close commit to use the new "permanent dead-API skip" framing.

## 8. Deviations Introduced

None. Reorient port is a direct shape-for-shape mirror of TS. Player.SetInteraction default arm mirrors `PathingEntity.ts:542-545` byte-for-byte. The duck-typed type-switch (`case *Player`, `case *Npc`) instead of `instanceof PathingEntity` is the same pattern used in `Npc.SetInteraction:651-666` and `Player.SetInteraction:79-91` — established convention, not a new divergence.

## 9. Implementation Approach

**Single TDD task** per Q2-A. Per `runescript_cadence.md` and `execution_mode_default.md`:

1. Brainstorm (this doc).
2. Plan via `superpowers:writing-plans`.
3. Subagent-driven TDD execution (one task; one subagent dispatch).
4. Two-stage review (whole-impl + code-quality reviewer per `superpowers:requesting-code-review`).
5. Close commit (with `Closes memory:` trailer per `close_commit_memory_trailer.md`).

## 10. Verification Checklist (pre-dispatch)

Per `controller_preflight.md`, verify against HEAD before dispatching the implementer:

- [ ] `modules/world/interaction.go:92-99` still hosts the `default:` DEVIATION block (line numbers may have drifted).
- [ ] `targetWidthLength` is still at `modules/world/npc_interaction.go:687` (or grep-locate it).
- [ ] `Player.targetX`, `Player.targetZ`, `Player.stepsTaken`, `Player.target`, `Player.focus` all exist on `*Player`.
- [ ] `Npc.targetX`, `Npc.targetZ`, `Npc.stepsTaken`, `Npc.target`, `Npc.focus` all exist on `*Npc`.
- [ ] `Server.processInfo` is at `modules/world/tick.go:329` (or grep-locate).
- [ ] No `(*Player).reorient` or `(*Npc).reorient` already declared.
- [ ] `coordgrid.Fine` is the canonical fine-coord helper (not `coordgrid.fine` lowercase or `pkg/coord/CoordGrid`).

## 11. Memory Pattern Compliance

- `dead_api_polish.md` — drove the NAI-34-D4-NPC + D5-NPC reframe (no field add, no write).
- `plan_grep_helper_patterns.md` — grepped for `targetWidthLength`; reuses existing helper instead of inlining.
- `ts_source_canonical_path.md` — only `LostCityRS/Engine-TS` cited; no sibling repo references.
- `flat_arg_signature_for_cross_lang_parity.md` — duplicated method on Player/Npc preferred over interface abstraction (precedent: NAI-41 SetInteraction).
- `plan_test_coverage_crosscheck.md` — § 6 enumerates one case per code branch.
- `controller_preflight.md` — § 10 checklist for pre-dispatch verification.
- `close_commit_memory_trailer.md` — close commit will carry `Closes memory:` trailer.
- `retire_deviation_grep_all_comments.md` — § 7.2 reframes tag-site doc-comments at every NAI-34-D4/D5-NPC mention, not only production touch points.

## 12. Carry-forwards (post-NAI-66)

- `NAI-65-D-FOCUS-INSTANT-WIRE` — face-instant wire protocol; no driver yet.
- `NAI-34-D4-NPC` + `NAI-34-D5-NPC` — permanent dead-API skip (re-framed by this spec; no further closure path until upstream-TS adds NPC consumer).
- All other NAI-65 carry-forwards remain unchanged.
