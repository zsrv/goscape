# NAI-65 — pathing-entity focus & step-tracking (partial closure)

## Goal

Close the three NAI-34 deviations whose target field already exists on the
entity at HEAD:

- **NAI-34-D3-Player** — `(*Player).Teleport` is missing the
  `focus(fine(moveX, width), fine(moveZ, length), false)` call from TS
  `PathingEntity.teleport` (`Engine-TS/src/engine/entity/PathingEntity.ts:286-289`).
- **NAI-34-D3-NPC** — `(*Npc).Teleport` is missing the same `focus(...)` call.
- **NAI-34-D4-Player** — `(*Player).Teleport` is missing
  `lastStepX = x - 1; lastStepZ = z` (TS `PathingEntity.ts:291-292`).

Re-frame the residual NAI-34 deviations with sharper "blocked-on-X" wording
in `nai_followups.md` (see § Deviation tags / Reframed).

## Non-goals

- D4-NPC — Npc has no `lastStepX/Z` field; adding them is dead-API per
  `dead_api_polish.md` until an NPC stride-tracking consumer ports.
- D5-NPC — Npc has no `jump` field; `pkg/rsbuf/npc.go:15-33` Npc struct has
  no `Jump` field either, mirroring upstream Rust `npc.rs:3-29`. Adding
  diverges from rsbuf upstream parity.
- NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ — Player.targetX/Z fields exist
  (`modules/world/player.go:98`, init -1) but the only TS reader is
  `reorient()` (`PathingEntity.ts:349-362`), which is unported in goscape.
  Closure bundles with the future `pathing-entity-reorient-and-stride-tracking`
  sub-spec.
- TS L274-275 `messageGame('Invalid teleport!')` on the unallocated-zone
  reject branch — orthogonal polish item, out of scope.

## TS reference

`Engine-TS/src/engine/entity/PathingEntity.ts:267-298`:

```ts
teleport(x, z, level) {
    if (isNaN(level)) level = 0;                          // (defensive — n/a in Go)
    level = Math.max(0, Math.min(level, 3));              // D1 — closed NAI-36-T7
    if (!isZoneAllocated(level, x, z) &&
        (!(this instanceof Player) || this.staffModLevel < 3)) { // D2 — closed NAI-36-T7
        if (this instanceof Player) this.messageGame('Invalid teleport!'); // out of scope
        return;
    }
    const previousX = this.x, previousZ = this.z, previousLevel = this.level;
    this.x = x; this.z = z; this.level = level;
    const dir = CoordGrid.face(previousX, previousZ, x, z);
    const moveX = CoordGrid.moveX(this.x, dir);
    const moveZ = CoordGrid.moveZ(this.z, dir);
    this.focus(CoordGrid.fine(moveX, this.width),         // D3 — IN SCOPE both entities
               CoordGrid.fine(moveZ, this.length), false);
    this.refreshZonePresence(previousX, previousZ, previousLevel);
    this.lastStepX = this.x - 1;                          // D4 — IN SCOPE Player only
    this.lastStepZ = this.z;
    this.tele = true;
    if (previousLevel != level) {                         // D5 — Player closed NAI-36-T7;
        this.moveSpeed = MoveSpeed.INSTANT;               //      NPC deferred
        this.jump = true;
    }
}
```

`focus()` from `PathingEntity.ts:321-333`:

```ts
focus(fineX, fineZ, client) {
    this.faceAngleX = fineX;
    this.faceAngleZ = fineZ;
    if (client) {
        this.faceSquareX = fineX;
        this.faceSquareZ = fineZ;
        this.masks |= this.coordmask;
    }
}
```

Goscape's `Npc.focus` (`modules/world/npc_interaction.go:706-710`) stores the
`instant` flag write-only — the `if (client)` branch is unported and tracked
under the existing free-text "face-instant wire protocol" follow-up. NAI-65
formalizes this as `NAI-65-D-FOCUS-INSTANT-WIRE` (see § Deviation tags).

## Edge cases

- **In-place teleport (prev == new)**: `coordgrid.Face` returns `-1` (the
  `Direction` underlying int sentinel). `coordgrid.DeltaX/DeltaZ` default-case
  yields `0`, so `MoveX(x, -1) = x` and `MoveZ(z, -1) = z`. Focus pins
  self-center coords; `lastStepX/Z` writes still apply (`x-1, z`). Pinned by
  test.
- **Width/length**: Player is 1×1 — pass literal `1` to `coordgrid.Fine` for
  both axes. Npc is `n.size`×`n.size` (square; `int(typ.Size)` at NewNpc).
- **Order vs. existing closure**: NAI-36-T7 placed `refreshPlayerZone` /
  `refreshNpcZone` BEFORE `tele = true` (TS L290-293). NAI-65 inserts the
  focus call BEFORE refresh (TS L286-290), and `lastStepX/Z` writes AFTER
  refresh and BEFORE `tele = true` (TS L291-293). Full ordered shape becomes:
  capture prev → write x/z/level → focus → refresh → lastStep adjust → tele.
- **Direction sentinel `-1`**: `coordgrid.Direction` is a typed alias
  (`pkg/coordgrid/coordgrid.go`) whose underlying int admits `-1` from
  `Face`'s `return -1` line. `DeltaX(-1)` and `DeltaZ(-1)` both fall through
  the switch's default case returning 0.

## Per-deviation closure plan

### T1 — `(*Player).focus` helper + Player.Teleport closure (D3-Player + D4-Player)

**New helper** in `modules/world/player_script.go` (paired with FaceSquare for
ergonomic locality; both deal with face-orientation state) or in a fresh
`modules/world/player_focus.go` — plan-author chooses at plan-write time:

```go
// focus records the fine-grained face-angle target. Mirrors TS
// PathingEntity.focus.
//
// DEVIATION NAI-65-D-FOCUS-INSTANT-WIRE: TS focus(_, _, client=true) ALSO
// writes faceSquareX/Z + masks |= coordmask. Goscape's wire protocol
// doesn't currently branch on it, so the flag is accepted for signature
// parity but stored write-only. Mirror site: (*Npc).focus
// (npc_interaction.go:706). Closure: future "face-instant wire protocol"
// sub-spec.
func (p *Player) focus(fx, fz int, instant bool) {
    p.faceAngleX = fx
    p.faceAngleZ = fz
    _ = instant
}
```

**Player.Teleport edits** (`modules/world/player_script.go:351-385`). New
ordered body:

```go
prevX, prevZ, prevLevel := p.x, p.z, p.level
p.x, p.z, p.level = x, z, level

// NAI-65 D3-Player: focus call from TS PathingEntity.ts:286-289.
dir := coordgrid.Face(prevX, prevZ, x, z)
moveX := coordgrid.MoveX(p.x, dir)
moveZ := coordgrid.MoveZ(p.z, dir)
p.focus(coordgrid.Fine(moveX, 1), coordgrid.Fine(moveZ, 1), false)

refreshPlayerZone(p, prevX, prevZ, prevLevel)

// NAI-65 D4-Player: lastStep adjust from TS PathingEntity.ts:291-292.
p.lastStepX = p.x - 1
p.lastStepZ = p.z

p.tele = true

// D5-Player (closed NAI-36-T7).
if prevLevel != level {
    p.moveSpeed = MoveSpeedInstant
    p.jump = true
}
```

### T2 — Npc.Teleport closure (D3-NPC)

**Npc.Teleport edits** (`modules/world/npc_script.go:128-144`):

```go
prevX, prevZ, prevLevel := n.x, n.z, n.level
n.x, n.z, n.level = x, z, level

// NAI-65 D3-NPC: focus call from TS PathingEntity.ts:286-289. Width=length=size.
dir := coordgrid.Face(prevX, prevZ, x, z)
moveX := coordgrid.MoveX(n.x, dir)
moveZ := coordgrid.MoveZ(n.z, dir)
n.focus(coordgrid.Fine(moveX, n.size), coordgrid.Fine(moveZ, n.size), false)

refreshNpcZone(n.server, n, prevX, prevZ, prevLevel)
n.tele = true
```

T2 does NOT add `lastStepX/Z` writes (D4-NPC deferred — no fields).

### T3 — DEVIATION-block trim across all reference sites

Per-instance Edit (no `replace_all` per `plan_doc_replaceall_timeline.md`)
on each commenting site:

1. `modules/world/npc_script.go:96-127` — Teleport doc-comment.
   Switch D3-Player + D3-NPC + D4-Player from "RESIDUAL" to "CLOSED in
   NAI-65". Keep D4-NPC + D5-NPC residual; reframe the "blocked on X" line
   for each.
2. `modules/world/player_script.go:341-350` — Teleport doc-comment.
   Switch D3-Player + D4-Player to closed; cite NAI-65.
3. `pkg/script/active.go:598-616` — Teleport adapter doc-comment.
   Same trim shape.
4. `modules/world/npc_script_test.go:732` — single-line test doc.
   Switch from "D3 (focus), D4 (lastStepX/Z), D5-NPC" to "D4-NPC (no
   field), D5-NPC (no field)".
5. `modules/world/interaction.go:92-98` — NAI-41 deviation comment.
   Update referenced sub-spec name from "focus/step-tracking" to
   "reorient-and-stride-tracking".

Verification: `rg "D3-Player|D3-NPC|D4-Player" pkg/ modules/ cmd/` post-T3
must show ZERO RESIDUAL hits (only CLOSED-tagged or test-doc references
allowed).

### CLOSE — close commit + memory updates

- Append `## NAI-65 — CLOSED <date>` block to
  `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`.
- Re-frame the carry-forward list at the bottom of the file: rename
  "pathing-entity-focus-and-step-tracking" → "pathing-entity-reorient-and-stride-tracking";
  list its bundled deviations as D4-NPC + D5-NPC + NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ
  with per-item blocked-on framing.
- Close commit carries the `Closes memory:` trailer per
  `close_commit_memory_trailer.md`.

## Test strategy

### T1 tests (`modules/world/player_script_test.go`)

| # | Name | Asserts |
|---|---|---|
| 1 | `TestPlayerFocusHelper` | Direct unit test. `instant=false` → `faceAngleX/Z` set, `faceSquareX/Z` and `masks` UNCHANGED. `instant=true` → SAME (write-only flag pinned per `ts_asymmetry_dual_pin.md`). |
| 2 | `TestPlayerTeleportFocus` | (3200,3200,0) → (3300,3300,0). Dir = NE; MoveX/MoveZ both +1 → moveX=3301, moveZ=3301. `faceAngleX = Fine(3301, 1) = 3301*64 + 31 = 211295`; `faceAngleZ = 211295`. |
| 3 | `TestPlayerTeleportLastStep` | Same call. `p.lastStepX = 3299`; `p.lastStepZ = 3300`. |
| 4 | `TestPlayerTeleportInPlace` | Teleport(p.x, p.z, p.level). `Face` returns -1; `MoveX/MoveZ` no-op. `faceAngleX = Fine(p.x, 1)`; `faceAngleZ = Fine(p.z, 1)`. `lastStepX = p.x - 1`; `lastStepZ = p.z`. `p.tele = true`. |
| 5 | `TestPlayerTeleportLevelChange` | Already exists for D5; cross-check focus + lastStep additions don't perturb the moveSpeed/jump assertions. |

### T2 tests (`modules/world/npc_script_test.go`)

| # | Name | Asserts |
|---|---|---|
| 1 | `TestNpcTeleportFocus` | size=1 NPC, (3200,3200) → (3300,3300). Pre-state: `n.faceAngleX = -1`. Post-state: `n.faceAngleX = 211295`; `n.faceAngleZ = 211295`. |
| 2 | `TestNpcTeleportFocusSize2` | size=2 NPC. `Fine(3301, 2) = 3301*64 + 63 = 211327`. Pin so a refactor that drops `n.size` to literal `1` fails. |
| 3 | `TestNpcTeleportInPlace` | Same shape as Player T1 test #4 but no lastStep assertion. |

### T3

No new tests; mechanical comment trim. Verified by `rg` listed above.

### Spec-test-coverage crosscheck

Per `plan_test_coverage_crosscheck.md`: at plan-write time the plan-author
diffs each task's code block against the test list above and confirms every
prod write-site has at least one pinning test.

## Deviation tags

### Opened by NAI-65

- `NAI-65-D-FOCUS-INSTANT-WIRE` — Player.focus + Npc.focus store the
  `instant` parameter write-only; TS `focus(_, _, client=true)` would also
  write `faceSquareX/Z` and OR `coordmask` into masks. Two sites:
  `modules/world/player_script.go` (or `player_focus.go`),
  `modules/world/npc_interaction.go:706`. Closure: future "face-instant
  wire protocol" sub-spec when a non-Teleport caller (e.g. SetInteraction's
  Engine-clicked Loc/Obj) passes `instant=true`.

### Closed by NAI-65

- `NAI-34-D3-Player`
- `NAI-34-D3-NPC`
- `NAI-34-D4-Player`

### Reframed (still residual after NAI-65)

- `NAI-34-D4-NPC` — blocked on NPC stride-tracking consumer.
- `NAI-34-D5-NPC` — blocked on `rsbuf.Npc.Jump` field + npcinfo encoder
  branch (upstream Rust `npc.rs:3-29` parity).
- `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ` — blocked on `(*Player).reorient`
  port (only TS reader of Player.targetX/Z).

Carry-forward sub-spec rename: `pathing-entity-focus-and-step-tracking` →
`pathing-entity-reorient-and-stride-tracking`.

## Wire-behaviour delta

**None at HEAD.** Both `(*Player).focus` and `(*Npc).focus` write to fields
(`faceAngleX/Z`) that have NO HEAD-reachable wire-encoder reader (verified
via `rg "faceAngleX|FaceAngleX"` returning only entity-internal sites and
test-only sites). Player.lastStepX/Z is read from `p.followX = p.lastStepX`
in `processInteraction` (`modules/world/interaction.go:160-161`), but
`p.followX/followZ` themselves have NO production reader (verified
`rg "followX|followZ"` returns only the write-site and NewPlayer init).

The closure is purely TS-shape correctness work. Future sub-specs that port
`reorient()`, the rsbuf Npc.Jump field, or wire-side faceSquare-from-focus
will read the now-correct state without a migration step.

This is exactly the trade the user accepted as "option C" during brainstorm
— see `defensive_gate_doc_comment_label.md` for the comment-labeling
convention applied to each new dead-write site.

## Risk register

- **Dead-write surface growth** — accepted per option C. Each new write
  site labels itself as TS-faithful-but-currently-dead via doc-comment
  per `defensive_gate_doc_comment_label.md`.
- **Per-instance Edit on T3** — must Edit each comment-block separately;
  `replace_all` forbidden per `plan_doc_replaceall_timeline.md`.
- **Direction sentinel handling** — `coordgrid.Face` returns `-1` on
  src==dst; `DeltaX/DeltaZ` default-case → 0. Pinned by in-place test.
- **Pre-flight grep targets** (per `enumerate_all_sites.md`):
  `D3-Player`, `D3-NPC`, `D4-Player`, `D4-NPC`, `D5-NPC`,
  `NAI-34-D3`, `NAI-34-D4`, `NAI-34-D5`, `face-instant`.
  Plan-author re-greps each at plan-write time and pre-dispatch.
- **Plan-author Go variable-name collisions** (per
  `plan_var_name_collision.md`): T1 + T2 introduce `dir`, `moveX`, `moveZ`
  locals inside `Teleport`. Verify no enclosing-scope shadow at plan-write.

## Cadence

Single bundle, 3 implementation tasks + 1 close. ~21 production LOC,
~75 test LOC, ~30 LOC of comment churn across 6 files (5 modified, 0-1
new). Above the compressed-cadence threshold (`compressed_cadence.md`) —
use full cadence with subagent-driven TDD per `runescript_cadence.md`.

## Memory entries reinforced

- `runescript_cadence.md` — full cadence, 3-task TDD bundle.
- `true_to_ts_gate.md` — every behavioural change cited against TS source.
- `dead_api_polish.md` — D4-NPC + D5-NPC deferred (no consumer); D3 + D4-Player
  closed because target fields exist on the entity at HEAD.
- `defensive_gate_doc_comment_label.md` — new dead-write sites
  doc-labeled.
- `enumerate_all_sites.md` — pre-flight grep targets enumerated above.
- `retire_deviation_grep_all_comments.md` — T3 ends with the verifying grep.
- `plan_doc_replaceall_timeline.md` — T3 uses per-instance Edit, not
  `replace_all`.
- `plan_var_name_collision.md` — `dir`/`moveX`/`moveZ` locals checked.
- `plan_test_coverage_crosscheck.md` — applied at plan-write time.
- `ts_asymmetry_dual_pin.md` — TestPlayerFocusHelper pins both `instant=false`
  and `instant=true` to assert the write-only flag.
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:`
  trailer.
