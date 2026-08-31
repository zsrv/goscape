# rev-274 Pin Update (Engine-TS dee467c8→4c95f87e, Content 7f97b0a5→37607266) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Advance the rev-274 branch to Engine-TS `4c95f87efe00b068cadbd229d94736626907bd1a` and Content `376072662e78a314bf35bb18815be39521491a6b` — port the 4 upstream engine commits, refresh the byte-parity reference cache, and re-pin REFERENCES.md.

**Architecture:** The Engine-TS delta is a fast-forward of exactly 4 commits touching 8 files (+55/−48). Each upstream commit becomes one goscape commit, ported true-to-TS in upstream chronological order. The Content delta (31 commits, 5637 files — dominated by the `_8`-suffix model rename that pairs with upstream `3b653372`) is consumed by advancing the Server274-ref reference worktrees and re-running the upstream TS pack toolchain, then re-running goscape's byte-parity gates against the fresh reference cache.

**Tech Stack:** Go 1.26+ (`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix on every `go` invocation), Node 24 (`npm run build` in Server274-ref/engine for the reference pack).

## Global Constraints

- Every commit: `git commit --no-gpg-sign`, message trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- True-to-TS fidelity gate: every behavioral divergence from the TS at the NEW pin needs an explicit PORTING.md row or inline PORTING-EXCEPTION; none are expected in this plan.
- Canonical TS source: `~/Code/github.com/LostCityRS/Engine-TS` ONLY (at `4c95f87e` — `git -C … show 4c95f87e:<file>` when the working tree is elsewhere). Never read `Engine-TS_274` or `Server/engine`.
- TS citation convention: code comments cite `TS <File>.ts:<line> @4c95f87e` for regions changed by this update; leave `@dee467c8`/older citations alone in untouched regions (they remain accurate — the delta touched only 8 files).
- The upstream work list is `git -C ~/Code/github.com/LostCityRS/Engine-TS diff dee467c8..4c95f87e` (fast-forward; commits in order: `3da10133`, `e31a8719`, `3b653372`, `4c95f87e`).
- Server274-ref (`~/Code/github.com/LostCityRS/Server274-ref`) is OUTSIDE the sandbox write allowlist — commands touching it need the sandbox override (expected, user-gated).
- Reference-cache-dependent tests resolve via `GOSCAPE_REF274_DIR=~/Code/github.com/LostCityRS/Server274-ref/engine`.

---

### Task 1: Port upstream `3da10133` — MoveClick: don't clear the interaction on op-click moves

Upstream rationale: a `MOVE_OPCLICK` is always paired with a following op packet that clears+sets the interaction itself. Clearing in the move handler drops the target in the gap when the per-tick user packet limit splits the pair across ticks. This REVERTS the f0ccbe8a "unconditional clear" posture goscape currently documents.

**Files:**
- Modify: `modules/world/handlers_game.go` (`moveClickInner` — the `p.ClearPendingAction()` site ~line 304, plus the stale doc comments at the `handleMoveOpClick` header ~line 221 and flow-note item 3 ~line 251)
- Test: `modules/world/move_opclick_pending_action_test.go` (new; follow the fixture style of `modules/world/move_minimap_click_test.go`)

**Interfaces:**
- Consumes: existing `moveClickInner(p *Player, payload []byte, opClick bool, trailingBytes int) error`, `(*Player).ClearPendingAction()`, `(*Player).SetInteraction(...)`.
- Produces: no new symbols — behavior change only (later tasks don't depend on this one).

**TS reference (MoveClickHandler.ts @4c95f87e, lines 26-32):**

```ts
// Clear previous interaction — but not for op-click moves.
// A MOVE_OPCLICK is always paired with a following op packet that clears+sets
// the interaction itself. Clearing here would drop the target in the gap when
// the per-tick user packet limit splits the pair across ticks.
if (!message.opClick) {
    player.clearPendingAction();
}
```

- [ ] **Step 1: Write the failing test**

Two cases in the new test file: (a) `MOVE_OPCLICK` payload leaves a previously-set target/pending action intact; (b) `MOVE_GAMECLICK` still clears it. Build the player + target via the same fixture helpers the existing move-click tests use (see `move_minimap_click_test.go` for server/player construction and payload framing: byte 0 ctrlHeld, G2 startX, G2 startZ, then dx/dz pairs). Assert via the observable the fixture exposes (e.g. `p.target != nil` / queued op state) — mirror how existing interaction tests assert `ClearPendingAction` effects.

```go
func TestMoveOpClickPreservesPendingAction(t *testing.T) {
	// fixture: player with an interaction target set (SetInteraction on an Npc)
	// payload: valid single-waypoint move click
	// call: handleMoveOpClick(p, payload)
	// assert: p.target still non-nil (interaction NOT cleared)
}

func TestMoveGameClickClearsPendingAction(t *testing.T) {
	// same fixture
	// call: handleMoveGameClick(p, payload)
	// assert: p.target == nil (interaction cleared)
}
```

- [ ] **Step 2: Run test to verify the opclick case fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestMove(OpClick|GameClick).*PendingAction' -v`
Expected: `TestMoveOpClickPreservesPendingAction` FAILS (interaction currently cleared unconditionally); the gameclick case passes.

- [ ] **Step 3: Implement the guard**

In `moveClickInner`, replace:

```go
	// Clear previous interaction (TS L27-28) — unconditional since f0ccbe8a
	// (fires for MOVE_OPCLICK too).
	p.ClearPendingAction()
```

with:

```go
	// Clear previous interaction — but not for op-click moves (TS
	// MoveClickHandler.ts:26-32 @4c95f87e, upstream #103): a MOVE_OPCLICK is
	// always paired with a following op packet that clears+sets the
	// interaction itself; clearing here would drop the target in the gap when
	// the per-tick user packet limit splits the pair across ticks.
	if !opClick {
		p.ClearPendingAction()
	}
```

Update the `handleMoveOpClick` doc comment (drop the "handler body no longer branches on opClick" f0ccbe8a note — it branches again as of 3da10133) and flow-note item 3 in the `moveClickInner` header ("ClearPendingAction — for ALL move opcodes" → gated on `!opClick` per @4c95f87e). The "opClick is retained for the debug log only" line is now stale too — remove it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestMove' -v`
Expected: PASS (including all pre-existing move-click tests; if one of them pins the unconditional-clear posture for MOVE_OPCLICK, update that test to the new TS-pinned behavior and cite @4c95f87e).

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/move_opclick_pending_action_test.go
git commit --no-gpg-sign -m "fix(world): don't clear interaction on op-click moves (TS 3da10133)"
```

---

### Task 2: Port upstream `e31a8719` — player/npc facing logic matched to OSRS (reorient split)

Upstream restructures facing into two halves and moves both from the post-movement `processInfo` sweep into each entity's own turn:

- `reorientEntity()` (NEW name) — serverside faceAngle toward a **pathing** (player/npc) target, `client=false`, runs BEFORE movement/interaction, paired with `setFaceEntity()`.
- `reorient()` (REWRITTEN) — faces a **loc/obj** target once stopped (`stepsTaken == 0`), now `client=true` (ships the face-coord mask), runs AFTER movement. Early-returns for a pathing target.
- `setInteraction()` no longer calls `focus()` at all (it only records `targetX/Z` for loc/obj).
- `World.processInfo` no longer reorients players or npcs — an npc that took no turn this tick keeps its spawn orientation.

**Files:**
- Modify: `modules/world/movement.go` (`(*Player).reorient` ~line 317 → split into `reorientEntity` + `reorient`)
- Modify: `modules/world/npc_interaction.go` (`(*Npc).reorient` ~line 1178 → same split; `(*Npc).SetInteraction` focus site ~line 1093-1101; `(*Npc).focus` doc-comment driver list ~line 1136)
- Modify: `modules/world/interaction.go` (`(*Player).SetInteraction` focus site ~line 133-147)
- Modify: `modules/world/tick.go` (`processPlayerFacing` ~line 1219 gains `p.reorientEntity()`; new `processPlayerReorient` pass wired between `processInteractions` and `processEnergy` ~line 238-241; `processInfo` drops both reorient loops ~lines 1035-1047 & 1069-1072)
- Modify: `modules/world/npc_ai.go` (`(*Npc).turn` gains `n.reorientEntity(); n.reorient()` between `processMovementInteraction` and `setFaceEntity` ~line 51-56)
- Test: extend/update `modules/world/player_reorient_test.go`, `modules/world/npc_reorient_test.go` (both currently pin the OLD single-reorient-in-processInfo behavior); check `modules/world/rsbuf_per_tick_test.go`, `npc_masks_test.go`, `tick_zero_players_test.go` for order-sensitive pins.

**Interfaces:**
- Produces: `(*Player).reorientEntity()`, `(*Player).reorient()`, `(*Npc).reorientEntity()`, `(*Npc).reorient()`, `(*Server).processPlayerReorient()` — all unexported, consumed only inside `modules/world`.
- Consumes: existing `focus(fx, fz int, instant bool)` on both types (instant=true writes faceSquare + ORs the face-coord mask).

**TS reference (PathingEntity.ts @4c95f87e):**

```ts
reorientEntity(): void {
    const target: Entity | null = this.target;
    if (target instanceof PathingEntity) {
        this.focus(CoordGrid.fine(target.x, target.width), CoordGrid.fine(target.z, target.length), false);
    }
}

reorient(): void {
    if (this.target instanceof PathingEntity) {
        return;
    }
    if (this.targetX !== -1 && this.stepsTaken === 0) {
        this.focus(this.targetX, this.targetZ, true);   // client=true — ships the face-coord
        this.targetX = -1;
        this.targetZ = -1;
    }
}
```

TS World.ts per-player order @4c95f87e: `processEngineQueue → setFaceEntity → reorientEntity → processInteraction (movement inside) → reorient → updateEnergy`. TS Npc.turn order: `processMovementInteraction → reorientEntity → reorient → setFaceEntity`. goscape's split-pass tick (Arc-29 documented deviation) maps this to: `processPlayerFacing` (setFaceEntity + reorientEntity per player) → …movement/interactions… → NEW `processPlayerReorient` pass → `processEnergy`.

- [ ] **Step 1: Write/adjust the failing tests**

In `player_reorient_test.go` + `npc_reorient_test.go` (rename/extend existing cases; keep their fixture style):

```go
// (a) pathing-target: reorientEntity refocuses faceAngle on the target's current
//     position with NO face-coord mask (instant=false) — same as old behavior,
//     new name/site.
// (b) loc/obj-target stopped: reorient() now fires focus(instant=true) — assert
//     faceSquareX/Z written AND the face-coord mask bit set, targetX cleared.
//     (Old behavior: instant=false, no mask — this is the key wire-visible change.)
// (c) loc/obj-target still moving (stepsTaken>0): reorient() no-ops, targetX kept.
// (d) pathing-target: reorient() early-returns even when targetX != -1.
// (e) SetInteraction no longer focuses: after SetInteraction on a loc with
//     kind=InteractionEngine, faceSquareX/Z are UNCHANGED and no face-coord mask
//     is set (old behavior: instant=true immediate focus); faceAngle also
//     unchanged; targetX/Z ARE recorded.
// (f) npc that takes no turn this tick keeps spawn orientation: run processInfo
//     alone (no npc.turn) with an npc holding a target — faceAngle unchanged
//     (old behavior: processInfo reoriented it).
```

Use the mask constants the codebase already uses (`rsbuf.NpcMaskFaceCoord` npc-side; the player face-coord mask constant used by `(*Player).focus` — read `movement.go`/`player_masks.go` for its name).

- [ ] **Step 2: Run to verify the new expectations fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'Reorient' -v`
Expected: cases (b), (e), (f) FAIL against current code; (a), (c), (d) may pass.

- [ ] **Step 3: Implement**

`modules/world/movement.go` — replace `(*Player).reorient` with (keep the doc-comment style, cite `TS PathingEntity.ts reorientEntity/reorient @4c95f87e`):

```go
func (p *Player) reorientEntity() {
	switch t := p.target.(type) {
	case *Player:
		p.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		p.focus(coordgrid.Fine(t.x, t.size), coordgrid.Fine(t.z, t.size), false)
	}
}

func (p *Player) reorient() {
	switch p.target.(type) {
	case *Player, *Npc:
		return
	}
	if p.targetX != -1 && p.stepsTaken == 0 {
		p.focus(p.targetX, p.targetZ, true)
		p.targetX = -1
		p.targetZ = -1
	}
}
```

`modules/world/npc_interaction.go` — same split for `(*Npc)` (mirror shape). In `(*Npc).SetInteraction`, DELETE the `n.focus(fx, fz, isNonPathing && kind == InteractionEngine)` call; keep the `fx/fz` computation only inside the `if isNonPathing { n.targetX = fx; n.targetZ = fz }` block (hoist as needed). Replace the focus-site comment with the upstream rationale: facing only changes during the entity's own turn (`reorientEntity` for pathing, `reorient` for loc/obj) per @4c95f87e. Update the `(*Npc).focus` doc comment's driver list (setInteraction is no longer a driver; `reorient` is now the only `instant=true` caller).

`modules/world/interaction.go` — same deletion in `(*Player).SetInteraction`: drop `p.focus(fx, fz, isNonPathing && kind == InteractionEngine)`, keep targetX/Z caching, update the comment block.

`modules/world/tick.go`:

```go
func (s *Server) processPlayerFacing() {
	players := s.snapshotPlayers()
	for _, p := range players {
		// TS World.ts @4c95f87e: both halves of "face the interaction target"
		// run here, the same tick the op set the target and before
		// processInteraction can clear it: the FACE_ENTITY mask, plus the
		// serverside faceAngle toward a pathing target (for new observers).
		p.setFaceEntity()
		p.reorientEntity()
	}
}

// processPlayerReorient runs after movement: face a loc/obj target if we
// walked over and held still (needs this tick's stepsTaken). TS World.ts
// @4c95f87e — player.reorient() between processInteraction and updateEnergy.
func (s *Server) processPlayerReorient() {
	players := s.snapshotPlayers()
	for _, p := range players {
		p.reorient()
	}
}
```

Wire `s.processPlayerReorient()` (with `statPlayer` timing, matching sibling passes) in the tick loop directly after `s.processInteractions()` and before `s.processEnergy()`. In `processInfo`, delete the `p.reorient()` line from the rebuild loop (keep `p.buildArea.rebuildNormal()` and its NAI-93 comment) and delete the whole npc `n.reorient()` loop; leave one-line comments mirroring TS's `// facing (reorientEntity/reorient) runs in the player's turn (processPlayers), not here.` / `…in Npc.turn(), not here.`

`modules/world/npc_ai.go` — in `turn`, after `n.processMovementInteraction(s)`:

```go
	// Reorient during the npc's own turn (not the post-movement processInfo
	// sweep), so an npc that didn't take a turn this tick keeps its spawn
	// orientation. TS Npc.ts @4c95f87e: reorientEntity (faceAngle toward a
	// player/npc) then reorient (face a loc/obj once stopped), then the
	// FACE_ENTITY mask.
	n.reorientEntity()
	n.reorient()

	n.setFaceEntity()
```

- [ ] **Step 4: Run the world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache GOSCAPE_REF274_DIR=~/Code/github.com/LostCityRS/Server274-ref/engine go test ./modules/world/ 2>&1 | tail -20`
Expected: PASS. Any failure in order-sensitive tests (`rsbuf_per_tick_test.go`, `npc_masks_test.go`, `tick_zero_players_test.go`) means a pinned old-order expectation — update the pin to the @4c95f87e order with a citation, not the implementation.

- [ ] **Step 5: Commit**

```bash
git add modules/world/
git commit --no-gpg-sign -m "fix(world): match player/npc facing to OSRS — reorient in own turn (TS e31a8719)"
```

---

### Task 3: Port upstream `3b653372` — simplified model shape packing

Content's model files dropped the `_8` suffix for centrepiece_straight models (`door_8.ob2` → `door.ob2`); the pack tool now probes the RAW name for the centrepiece shape and the `directReference` special case is deleted. The unpack tool inverts this: shape `_8` (value 10) produces no suffix.

**Files:**
- Modify: `pkg/pack/loc.go` (`resolveLocModels` ~line 247-292 + its doc comment ~line 232-245)
- Modify: `pkg/unpack/config/rename.go` (`renameModelLoc` ~line 261-274)
- Modify: `pkg/unpack/config/driver.go` (`unpackModelNames` loc-arm suffix construction ~line 463-473)
- Test: `pkg/pack/loc_test.go` (existing resolveLocModels cases pin the directReference behavior), plus the unpack golden tests under `pkg/unpack/` (manifest-driven — will be re-validated in Task 5 after the reference regen; this task only keeps unit tests green)

**Interfaces:**
- Consumes: `modelPack.GetByName(name string) int` (−1 = absent), `LocShapeSuffix` map/array, `locShapeCentrepieceStraight` (= 10) in `pkg/pack`; `LocShapeSuffix [23]string` in `pkg/unpack/config`.
- Produces: no signature changes — `resolveLocModels(srcModels, modelPack, modelFlags, debugname)` and `renameModelLoc(modelID, shape, modelPack)` keep their shapes.

**TS reference (@4c95f87e):** `tools/pack/config/LocConfig.ts:326-336` (directReference block deleted; `ModelPack.getByName(srcModels[i])` probed for centrepiece), `tools/unpack/config/LocConfig.ts:12-20` (`shape !== LocShapeSuffix._8 && model.endsWith(…)`), `tools/unpack/config/Unpack.ts:256-266` (`const suffix = shape === LocShapeSuffix._8 ? '' : LocShapeSuffix[shape]`). The two other hunks in the upstream diff (`{ model: number; shape: number }` semicolon, `(code - 30) + 1` parens) are TS formatting only — no Go counterpart.

- [ ] **Step 1: Update/write the failing tests**

In `pkg/pack/loc_test.go`: retire the directReference-pinning cases; new pins:

```go
// (a) raw-name hit: modelPack has "door" → resolves to {model: id("door"), shape: 10}
//     and modelFlags[id] |= 0x4. (Old code required "door_8" for this.)
// (b) raw + shaped variants: modelPack has "door" AND "door_1" → BOTH emitted:
//     {door, shape 10} then {door_1, shape 1}. (Old directReference logic forced
//     raw-only to shape _8 and skipped when variants existed.)
// (c) no matches at all → error "failed to find suitable loc models" (unchanged).
```

In the unpack unit tests (find the existing `renameModelLoc`/`unpackModelNames` cases via `grep -rn renameModelLoc pkg/unpack --include='*_test.go'`): shape 10 no longer strips a suffix in `renameModelLoc`, and `unpackModelNames` emits `debugname` (no `_8`) for shape 10, `debugnamei2` (not `debugnamei2_8`) on collision.

- [ ] **Step 2: Run to verify failures**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ ./pkg/unpack/... -run 'Loc|Rename|ModelName' -v 2>&1 | tail -30`
Expected: new pins FAIL.

- [ ] **Step 3: Implement**

`pkg/pack/loc.go` — in `resolveLocModels`, delete the whole `directReference` probe block AND the forced-shape block; replace the centrepiece probe:

```go
	for _, raw := range srcModels {
		// centrepiece_straight comes first in their data, so we check it
		// first — the raw (unsuffixed) name IS the centrepiece model since
		// TS LocConfig.ts:328-336 @4c95f87e (upstream 3b653372; Content
		// dropped the _8 filename suffix in the paired rename).
		if id := modelPack.GetByName(raw); id != -1 {
			if modelFlags != nil {
				modelFlags[id] |= 0x4
			}
			models = append(models, locModelShape{model: id, shape: locShapeCentrepieceStraight})
		}
		for shape := 0; shape <= 22; shape++ {
			// … existing suffixed-shape loop unchanged …
		}
	}
```

Update the `resolveLocModels` doc comment (drop the directReference bullet, cite @4c95f87e).

`pkg/unpack/config/rename.go` — `renameModelLoc`:

```go
	suffix := LocShapeSuffix[shape]
	// shape 10 (centrepiece_straight, "_8") carries no filename suffix since
	// TS LocConfig.ts:15 @4c95f87e — only strip for non-centrepiece shapes.
	if shape != 10 && strings.HasSuffix(name, suffix) {
		name = name[:len(name)-2]
	}
```

`pkg/unpack/config/driver.go` — `unpackModelNames` loc arm:

```go
	// TS Unpack.ts:259-264 @4c95f87e: centrepiece_straight (shape 10) emits
	// no suffix; all other shapes keep theirs.
	suffix := LocShapeSuffix[shape]
	if shape == 10 {
		suffix = ""
	}
	name := debugname + suffix
	i := 2
	for modelPack.GetByName(name) != -1 {
		name = fmt.Sprintf("%si%d%s", debugname, i, suffix)
		i++
	}
```

- [ ] **Step 4: Run the pack/unpack unit suites**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... ./pkg/unpack/... 2>&1 | tail -15`
Expected: unit tests PASS. Manifest-driven golden tests that read `GOSCAPE_REF274_DIR` may fail if run with the env set against the OLD reference cache — that's expected until Task 5; run WITHOUT the env here (they skip).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/ pkg/unpack/
git commit --no-gpg-sign -m "feat(pack): simplified model shape packing — raw name is centrepiece (TS 3b653372)"
```

---

### Task 4: Port upstream `4c95f87e` — STAT_RANDOM: clamp level to 99

**Files:**
- Modify: `pkg/script/handlers_player.go` (`statRandomThreshold` ~line 628-648 + doc comment; `handleStatRandom` doc comment ~line 649-670)
- Test: `pkg/script/handlers_player_test.go` (existing statRandom threshold pins)

**Interfaces:**
- `statRandomThreshold(low, high, level int) int` — signature unchanged; clamp applied inside so every caller/test exercises it (TS clamps at the call site; same observable).

**TS reference (PlayerOps.ts:583-588 @4c95f87e):**

```ts
const level = state.activePlayer.levels[stat];
const clampedLevel = Math.min(level, 99);
const value = Math.floor((low * (99 - clampedLevel)) / 98) + Math.floor((high * (clampedLevel - 1)) / 98) + 1;
```

- [ ] **Step 1: Write the failing test**

```go
func TestStatRandomThresholdClampsLevelTo99(t *testing.T) {
	// boosted stat: level 120 must behave exactly like level 99
	if got, want := statRandomThreshold(10, 10, 120), statRandomThreshold(10, 10, 99); got != want {
		t.Fatalf("clamped threshold = %d, want %d", got, want)
	}
	// level 99: low*(0)/98 + high*98/98 + 1 = high + 1
	if got := statRandomThreshold(10, 40, 99); got != 41 {
		t.Fatalf("threshold(10,40,99) = %d, want 41", got)
	}
	// sub-99 path unchanged: level 1 → low + 1
	if got := statRandomThreshold(10, 40, 1); got != 11 {
		t.Fatalf("threshold(10,40,1) = %d, want 11", got)
	}
}
```

If an existing test pins the UNclamped >99 float64/floor divergence (the level=120 case in the current doc comment), update it — that regime is now clamped away upstream.

- [ ] **Step 2: Run to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestStatRandom -v`
Expected: FAIL on the level-120 case (current code computes the negative-numerator floor path).

- [ ] **Step 3: Implement**

```go
func statRandomThreshold(low, high, level int) int {
	// TS PlayerOps.ts:584 @4c95f87e (upstream #110): clamp the live level to
	// 99 so boosted stats can't push the low term's (99-level) factor
	// negative. math.Floor retained for the OOB-stat level=0 regime (the
	// (level-1) numerator is still negative there; JS Math.floor rounds
	// toward -∞ where Go int division truncates toward zero).
	clamped := min(level, 99)
	return int(math.Floor(float64(low)*float64(99-clamped)/98)) +
		int(math.Floor(float64(high)*float64(clamped-1)/98)) + 1
}
```

Trim the now-stale level=120 float-divergence explanation in the old doc comment down to the level=0 rationale, and refresh the formula quote in the `handleStatRandom` doc comment (`clampedLevel` per @4c95f87e).

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ 2>&1 | tail -5`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "fix(script): STAT_RANDOM clamps level to 99 (TS 4c95f87e)"
```

---

### Task 5: Advance Server274-ref to the new pins and regenerate the byte-parity baseline

The reference checkouts already contain both target commits locally (verified in pre-flight; no network needed). The upstream pack run takes ~12s (`build.log` precedent). Everything under Server274-ref needs the sandbox override.

**Files:**
- Modify (outside repo): `~/Code/github.com/LostCityRS/Server274-ref/engine` (checkout `4c95f87e`, rebuild `data/pack`), `…/content` (checkout `376072662`)
- Modify: `pkg/packall/testdata/ref274_manifest.txt` (regenerate sha256s + header pins)
- Modify: `pkg/unpack/testdata/ref274/*.manifest.txt` (regenerate if the unpack goldens shifted)
- Modify: `pkg/unpack/unpacktest/harness.go` + `pkg/pack/compiler/runescript/jag_file_writer.go:22` + `modules/world/testdata_path_test.go` header comments IF their pinned-commit citations are now wrong (they cite @dee467c8/7f97b0a5 as "the pinned Content" — bump to the new SHAs)

**Interfaces:**
- Produces: a Server274-ref reference cache at the NEW pins that every `GOSCAPE_REF274_DIR` consumer (world tests, packall parity, unpack goldens) reads from Task 6 onward.

- [ ] **Step 1: Preserve the old reference cache, advance the pins**

```bash
cd ~/Code/github.com/LostCityRS/Server274-ref
mv engine/data/pack engine/data/pack.dee467c8.bak   # keep the old baseline until parity passes
git -C engine checkout 4c95f87efe00b068cadbd229d94736626907bd1a
git -C content checkout 376072662e78a314bf35bb18815be39521491a6b
git -C engine log --oneline -1 && git -C content log --oneline -1   # verify
```

(If `engine/.cache` exists — the incremental maps-server layout — move it aside too so the build is clean.)

- [ ] **Step 2: Rebuild the reference cache with the upstream toolchain**

```bash
cd ~/Code/github.com/LostCityRS/Server274-ref/engine
npm run build 2>&1 | tail -20
```

Expected: `pack: ~12s`, exit 0 (a few known `missing model` WARNs are normal — compare against `../build.log`). Verify `data/pack/server/` + `data/pack/client/` repopulated.

- [ ] **Step 3: Regenerate `pkg/packall/testdata/ref274_manifest.txt`**

Manifest format is `<sha256> <path-relative-to-engine>` with `#` comments. Regenerate each existing entry's hash from the NEW reference cache (the file list should be unchanged — same 56-file scope; if a file appeared/disappeared, investigate before proceeding):

```bash
cd ~/Code/github.com/zsrv/goscape
awk '!/^#/ && NF==2 {print $2}' pkg/packall/testdata/ref274_manifest.txt | while read -r f; do
  sha=$(sha256sum "~/Code/github.com/LostCityRS/Server274-ref/engine/$f" | cut -d' ' -f1)
  echo "$sha $f"
done
```

Splice the new hashes back in, update the header comment pins (`Engine-TS 4c95f87e + Content 37607266`, packed 2026-07-16).

- [ ] **Step 4: Run the packall byte-parity gate**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
GOSCAPE_REF274_DIR=~/Code/github.com/LostCityRS/Server274-ref/engine \
go test ./pkg/packall/ -run Parity -v 2>&1 | tail -30
```

Expected: PASS — goscape's packer (with Task 3's change) reproduces the new TS-packed cache byte-for-byte for every manifest file. A mismatch here means Task 3 diverged from TS (or `.cache` staleness — see the cache-staleness trap: rebuild clean before debugging the encoder).

- [ ] **Step 5: Regenerate the unpack goldens if shifted, run the unpack suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
GOSCAPE_REF274_DIR=~/Code/github.com/LostCityRS/Server274-ref/engine \
go test ./pkg/unpack/... 2>&1 | tail -15
```

If `AssertManifest` failures appear, regenerate `pkg/unpack/testdata/ref274/<family>.manifest.txt` from the new unpack output (check `git log --follow` on one manifest for the established regen method before hand-rolling one) and re-run to green.

- [ ] **Step 6: Run the world suite against the new cache, then drop the backup**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
GOSCAPE_REF274_DIR=~/Code/github.com/LostCityRS/Server274-ref/engine \
go test ./modules/world/ 2>&1 | tail -10
rm -rf ~/Code/github.com/LostCityRS/Server274-ref/engine/data/pack.dee467c8.bak
```

Expected: PASS (script.dat recompiled from 31 Content commits — script-dependent world tests exercise the new bytecode).

- [ ] **Step 7: Commit the manifest/citation updates**

```bash
git add pkg/packall/testdata/ pkg/unpack/testdata/ pkg/unpack/unpacktest/ pkg/pack/compiler/runescript/jag_file_writer.go modules/world/testdata_path_test.go
git commit --no-gpg-sign -m "test: re-pin ref274 byte-parity baseline at Engine-TS 4c95f87e + Content 37607266"
```

(Only include the comment-citation files if actually touched.)

---

### Task 6: Re-pin REFERENCES.md on main + final gate

**Files:**
- Modify (on `main` branch): `REFERENCES.md` §rev-274 (Engine-TS pin `dee467c8…` → `4c95f87efe00b068cadbd229d94736626907bd1a`, Content pin `7f97b0a5…` → `376072662e78a314bf35bb18815be39521491a6b`, note 2's work-list diff range, capture date)
- Verify: no PORTING.md changes expected (all 4 ports are full-fidelity; add a row ONLY if a task declared a deviation)

- [ ] **Step 1: Full test gate on rev-274**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
GOSCAPE_REF274_DIR=~/Code/github.com/LostCityRS/Server274-ref/engine \
go test ./... 2>&1 | grep -v '^ok\|no test files' | tail -20
```

Expected: clean. Then the race gate on touched packages (`-short` per the alloc-gate trap):

```bash
CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
GOSCAPE_REF274_DIR=~/Code/github.com/LostCityRS/Server274-ref/engine \
go test -race -short ./modules/world/ ./pkg/script/ ./pkg/pack/... ./pkg/unpack/... ./pkg/packall/ 2>&1 | tail -10
```

- [ ] **Step 2: Update REFERENCES.md on main via a temp worktree**

```bash
git worktree add "$TMPDIR/goscape-main-pin" main
# edit $TMPDIR/goscape-main-pin/REFERENCES.md §rev-274:
#   Engine-TS pinned commit → 4c95f87efe00b068cadbd229d94736626907bd1a
#   Content pinned commit   → 376072662e78a314bf35bb18815be39521491a6b
#   note 2 work list        → git -C Engine-TS diff dee467c8..4c95f87e superseded;
#                             record "updated 2026-07-16 (4 engine commits + 31 content commits)"
git -C "$TMPDIR/goscape-main-pin" add REFERENCES.md
git -C "$TMPDIR/goscape-main-pin" commit --no-gpg-sign -m "docs(references): rev-274 re-pinned to Engine-TS 4c95f87e + Content 37607266"
git worktree remove "$TMPDIR/goscape-main-pin"
```

- [ ] **Step 3: Verify branch state**

```bash
git log --oneline main -1 && git log --oneline rev-274 -6
```

Expected: main tip = the re-pin docs commit; rev-274 tip = Task-5 manifest commit atop the 4 port commits.

---

## Self-Review Notes

- Spec coverage: 4 upstream engine commits → Tasks 1-4 (one commit each, upstream order); Content pin → Task 5 (reference regen + parity); pin bookkeeping → Task 6. Complete.
- The upstream `tools/` formatting-only hunks (semicolon, parens) have no Go counterpart — documented in Task 3 so the implementer doesn't hunt for them.
- Type consistency: `reorientEntity`/`reorient` names used identically in Tasks 2's tick.go/npc_ai.go wiring and movement.go/npc_interaction.go definitions; `statRandomThreshold` signature unchanged across Task 4 steps.
- Deliberate sequencing: Task 3 (pack tool) MUST land before Task 5's parity run — the new reference cache is packed from renamed Content models and only matches a goscape packer that probes raw names.
