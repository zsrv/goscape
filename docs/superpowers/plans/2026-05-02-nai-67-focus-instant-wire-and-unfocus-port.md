# NAI-67 focus instant-wire port + Player.SetInteraction driver + Npc.unfocus port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the `instant=true` wire branch into `(*Player).focus` + `(*Npc).focus` per TS PathingEntity.ts:321-333, port the missing `(*Player).SetInteraction → p.focus(...)` call per TS:528, and port `(*Npc).unfocus` + wire it into `resetEntityForRespawn`. `(*Player).unfocus` is deferred (no consumer at HEAD).

**Architecture:** Two bundles. **B1 — focus-family wire + driver** has three TDD cycles (Player.focus, Npc.focus, Player.SetInteraction driver) + close commit; closes `NAI-65-D-FOCUS-INSTANT-WIRE`. **B2 — Npc.unfocus + respawn wire** has two TDD cycles (Npc.unfocus, resetEntityForRespawn wire) + close commit; opens `NAI-67-D-PLAYER-UNFOCUS-DEFERRED`.

**Tech Stack:** Go 1.26+. TS source canonical path: `$HOME/Code/github.com/LostCityRS/Engine-TS/`.

**Spec:** `docs/superpowers/specs/2026-05-02-nai-67-focus-instant-wire-and-unfocus-port-design.md`.

**Net deviation tally:** 13 (post-NAI-66) → -1 close +1 open → **13** at NAI-67 close.

---

## Pre-flight Verification (Controller)

Per `controller_preflight.md`: re-grep the following at HEAD before each implementer dispatch. **Stale assertions ⇒ pause and re-author the affected task before dispatch.**

- [ ] `(*Player).focus` is at `modules/world/player_script.go:415-419` with `_ = instant`. Run: `grep -n "func (p \*Player) focus" modules/world/player_script.go` → expect line ~415.
- [ ] `(*Npc).focus` is at `modules/world/npc_interaction.go:706-710` with `_ = instant`. Run: `grep -n "func (n \*Npc) focus" modules/world/npc_interaction.go` → expect line ~706.
- [ ] `(*Player).SetInteraction` is at `modules/world/interaction.go:61-101` and does NOT call `p.focus(...)`. Run: `grep -n "p.focus(\|p\.focus *(" modules/world/interaction.go` → expect zero hits.
- [ ] DEVIATION block at `modules/world/player_script.go:409-414` cites `NAI-65-D-FOCUS-INSTANT-WIRE`. Run: `grep -n "NAI-65-D-FOCUS-INSTANT-WIRE" modules/world/player_script.go modules/world/npc_interaction.go` → expect ≥1 hit each.
- [ ] Existing NAI-65 dual-pin tests are at `modules/world/player_script_test.go:1147-1181` (TestPlayerFocus_HelperWritesFaceAngleOnly) and `modules/world/npc_interaction_test.go:902-920` (TestNpcFocusSetsFaceAngleCoords). Run: `grep -n "TestPlayerFocus_HelperWritesFaceAngleOnly\|TestNpcFocusSetsFaceAngleCoords" modules/world/`.
- [ ] `(*Npc).SetInteraction` already passes `isNonPathing && kind == InteractionEngine` to `n.focus` at `modules/world/npc_interaction.go:665`. Run: `grep -n "n.focus(fx, fz, isNonPathing" modules/world/npc_interaction.go` → expect a single hit on line ~665.
- [ ] `targetWidthLength` is at `modules/world/npc_interaction.go:690-695`. Run: `grep -n "func targetWidthLength" modules/world/npc_interaction.go`.
- [ ] `resetEntityForRespawn` is at `modules/world/npc_registry.go:115`. Run: `grep -n "func (s \*Server) resetEntityForRespawn" modules/world/npc_registry.go`.
- [ ] `coordgrid.Fine` exists at `pkg/coordgrid/coordgrid.go:127`. Run: `grep -n "^func Fine" pkg/coordgrid/coordgrid.go`.
- [ ] Mask constants: `rsbuf.MaskFaceCoord = 0x20`, `rsbuf.NpcMaskFaceCoord = 0x80`. Run: `grep -nE "MaskFaceCoord\s*=" pkg/rsbuf/visibility.go pkg/rsbuf/npc_source.go`.
- [ ] Interaction enum has only `InteractionEngine` and `InteractionScript`. Run: `grep -n "InteractionEngine\|InteractionScript" modules/world/interaction.go` → expect both within `const ( … )` block at L16-19.
- [ ] No existing `unfocus` method on Npc or Player. Run: `grep -n "func (n \*Npc) unfocus\|func (p \*Player) unfocus" modules/world/*.go` → expect empty.
- [ ] Tests run clean against HEAD. Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...` → expect PASS.

If any expectation fails, halt and re-author affected tasks. Do not dispatch with stale premises.

---

## Bundle 1 — focus-family wire + driver

**Goal:** close `NAI-65-D-FOCUS-INSTANT-WIRE`. Surface and close (in the same bundle) the previously-untagged `(*Player).SetInteraction` missing focus() call.

### Task 1.1 — `(*Player).focus` instant=true wire branch

**Files:**
- Modify: `modules/world/player_script.go` (replace body of `(*Player).focus` at L415-419 + drop DEVIATION block at L409-414)
- Modify: `modules/world/player_script_test.go` (flip `TestPlayerFocus_HelperWritesFaceAngleOnly` polarity at L1143-1181)

**Test commands:**
- Targeted: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerFocus -v`
- Module: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

- [ ] **Step 1: Rewrite the test to the new contract.** Replace the existing `TestPlayerFocus_HelperWritesFaceAngleOnly` with the polarity-flipped version. Open `modules/world/player_script_test.go` and replace L1143-1181 with:

```go
// TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant pins TS
// PathingEntity.focus (PathingEntity.ts:321-333). instant=false sets
// faceAngleX/Z only — does NOT touch faceSquareX/Z or masks.
// instant=true ALSO writes faceSquareX = fineX, faceSquareZ = fineZ,
// and ORs MaskFaceCoord into masks.
//
// Per ts_asymmetry_dual_pin.md: dual-pin both branches. The
// instant=false absence-pin escalates if upstream changes the focus()
// shape; the instant=true presence-pin escalates if the wire writes
// regress.
func TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant(t *testing.T) {
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.faceAngleX = -1
	p.faceAngleZ = -1
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	// instant=false — faceAngle written; faceSquare/mask untouched.
	p.focus(123, 456, false)
	if p.faceAngleX != 123 || p.faceAngleZ != 456 {
		t.Errorf("instant=false faceAngle: got (%d, %d), want (123, 456)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("instant=false faceSquare: got (%d, %d), want (-1, -1) unchanged", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks != 0 {
		t.Errorf("instant=false masks: got %d, want 0 unchanged", p.masks)
	}

	// instant=true — faceAngle written; faceSquare = (fx, fz);
	// MaskFaceCoord ORed in.
	p.focus(789, 1011, true)
	if p.faceAngleX != 789 || p.faceAngleZ != 1011 {
		t.Errorf("instant=true faceAngle: got (%d, %d), want (789, 1011)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != 789 || p.faceSquareZ != 1011 {
		t.Errorf("instant=true faceSquare: got (%d, %d), want (789, 1011)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&rsbuf.MaskFaceCoord == 0 {
		t.Errorf("instant=true masks: MaskFaceCoord bit not set (masks=%d)", p.masks)
	}
}
```

Confirm `rsbuf` is already imported in this file (it is — used elsewhere in the test). If not, add `"github.com/zsrv/goscape/pkg/rsbuf"`.

- [ ] **Step 2: Run the test to verify it fails.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant -v`
Expected: FAIL on the instant=true block (faceSquare still at -1; mask still 0).

- [ ] **Step 3: Implement the wire branch.** In `modules/world/player_script.go`, replace L409-419 (DEVIATION block + body) with:

```go
// focus records the fine-grained face-angle coord. Mirrors TS
// PathingEntity.focus (Engine-TS/src/engine/entity/PathingEntity.ts:321-333).
// instant=true ALSO writes faceSquareX/Z to (fx, fz) and ORs
// MaskFaceCoord into masks.
//
// Coord-frame note: focus() takes RAW fine coords (already
// CoordGrid.fine'd). Distinct from (*Player).FaceSquare in
// modules/world/player_masks.go which takes absolute coords and
// applies *2+1.
//
// Drivers per TS: Teleport (PathingEntity.ts:289), takeStep
// (PathingEntity.ts:220), reorient (PathingEntity.ts:353,358),
// setInteraction (PathingEntity.ts:528). The setInteraction site is
// the only one that ever passes instant=true — gated on
// (target instanceof NonPathingEntity && interaction === Interaction.ENGINE).
func (p *Player) focus(fx, fz int, instant bool) {
	p.faceAngleX = fx
	p.faceAngleZ = fz
	if instant {
		p.faceSquareX = fx
		p.faceSquareZ = fz
		p.masks |= rsbuf.MaskFaceCoord
	}
}
```

- [ ] **Step 4: Run the test to verify it passes.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant -v`
Expected: PASS.

- [ ] **Step 5: Run the full module to catch regressions.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS. (This catches any callsite that depended on the old write-only `instant` semantics.)

- [ ] **Step 6: Commit.**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-67 T1.1 — (*Player).focus instant=true wire branch

Ports TS PathingEntity.focus (PathingEntity.ts:321-333) instant=true
branch: writes faceSquareX/Z = (fx, fz) and ORs MaskFaceCoord into
masks. Drops the NAI-65-D-FOCUS-INSTANT-WIRE deviation block.

Existing dual-pin test polarity-flipped: presence-pin on the
instant=true wire writes; absence-pin retained on the instant=false
path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1.2 — `(*Npc).focus` instant=true wire branch

**Files:**
- Modify: `modules/world/npc_interaction.go` (replace body of `(*Npc).focus` at L706-710 + drop DEVIATION block at L701-705)
- Modify: `modules/world/npc_interaction_test.go` (flip `TestNpcFocusSetsFaceAngleCoords` polarity at L902-920)

**Test commands:**
- Targeted: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcFocus -v`
- Module: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

- [ ] **Step 1: Rewrite the test.** In `modules/world/npc_interaction_test.go`, replace L902-920 with:

```go
// TestNpcFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant pins TS
// PathingEntity.focus (PathingEntity.ts:321-333) for the Npc
// override. Symmetric with TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant.
// instant=true ORs NpcMaskFaceCoord (= 0x80, distinct from
// MaskFaceCoord = 0x20 used by Player).
func TestNpcFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.faceSquareX = -1
	n.faceSquareZ = -1
	n.masks = 0

	// instant=false — faceAngle written; faceSquare/mask untouched.
	n.focus(6431, 6431, false)
	if n.faceAngleX != 6431 || n.faceAngleZ != 6431 {
		t.Errorf("instant=false faceAngle: got (%d, %d), want (6431, 6431)", n.faceAngleX, n.faceAngleZ)
	}
	if n.faceSquareX != -1 || n.faceSquareZ != -1 {
		t.Errorf("instant=false faceSquare: got (%d, %d), want (-1, -1) unchanged", n.faceSquareX, n.faceSquareZ)
	}
	if n.masks != 0 {
		t.Errorf("instant=false masks: got %d, want 0 unchanged", n.masks)
	}

	// instant=true — faceAngle written; faceSquare = (fx, fz);
	// NpcMaskFaceCoord ORed in.
	n.focus(1000, 2000, true)
	if n.faceAngleX != 1000 || n.faceAngleZ != 2000 {
		t.Errorf("instant=true faceAngle: got (%d, %d), want (1000, 2000)", n.faceAngleX, n.faceAngleZ)
	}
	if n.faceSquareX != 1000 || n.faceSquareZ != 2000 {
		t.Errorf("instant=true faceSquare: got (%d, %d), want (1000, 2000)", n.faceSquareX, n.faceSquareZ)
	}
	if n.masks&rsbuf.NpcMaskFaceCoord == 0 {
		t.Errorf("instant=true masks: NpcMaskFaceCoord bit not set (masks=%d)", n.masks)
	}
}
```

Confirm `rsbuf` is imported in this file (verify with `grep "pkg/rsbuf" modules/world/npc_interaction_test.go`; if absent, add `"github.com/zsrv/goscape/pkg/rsbuf"` to the import block).

- [ ] **Step 2: Run the test to verify it fails.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant -v`
Expected: FAIL on the instant=true block.

- [ ] **Step 3: Implement the wire branch.** In `modules/world/npc_interaction.go`, replace L697-710 (DEVIATION block + body) with:

```go
// focus records the fine-grained face-angle coord. Mirrors TS
// PathingEntity.focus (Engine-TS/src/engine/entity/PathingEntity.ts:321-333).
// instant=true ALSO writes faceSquareX/Z to (fx, fz) and ORs
// NpcMaskFaceCoord into masks.
//
// Coord-frame note: focus() takes RAW fine coords. Distinct from
// (*Npc).FaceSquare in modules/world/npc_masks.go which takes absolute
// coords and applies *2+1.
//
// Drivers per TS: takeStep (PathingEntity.ts:220), Teleport
// (PathingEntity.ts:289), reorient (PathingEntity.ts:353,358),
// setInteraction (PathingEntity.ts:528). The setInteraction site
// (modules/world/npc_interaction.go:665) is the only one that ever
// passes instant=true.
func (n *Npc) focus(fx, fz int, instant bool) {
	n.faceAngleX = fx
	n.faceAngleZ = fz
	if instant {
		n.faceSquareX = fx
		n.faceSquareZ = fz
		n.masks |= rsbuf.NpcMaskFaceCoord
	}
}
```

- [ ] **Step 4: Run the test to verify it passes.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant -v`
Expected: PASS.

- [ ] **Step 5: Run the full module to catch regressions.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS. (This catches the existing `(*Npc).SetInteraction` driver site at `npc_interaction.go:665` which now actively writes wire bits when an engine-clicked Loc/Obj target is set.)

- [ ] **Step 6: Commit.**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-67 T1.2 — (*Npc).focus instant=true wire branch

Symmetric with T1.1. instant=true writes faceSquareX/Z = (fx, fz) and
ORs NpcMaskFaceCoord (=0x80) into masks. Drops the
NAI-65-D-FOCUS-INSTANT-WIRE deviation block.

Activates the existing (*Npc).SetInteraction driver at
npc_interaction.go:665 which already passes
isNonPathing && kind == InteractionEngine to focus().

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1.3 — `(*Player).SetInteraction` TS:528 driver port

**Files:**
- Modify: `modules/world/interaction.go` (insert focus() call into `Player.SetInteraction` at L78-100; add `entitypkg` import to L1-6)
- Modify: `modules/world/interaction_test.go` (append 5 new tests)

**Test commands:**
- Targeted: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSetInteraction -v`
- Module: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

- [ ] **Step 1: Write the 5 failing tests.** Append to `modules/world/interaction_test.go`:

```go
// TestSetInteractionNpcTargetWritesFaceAngleNoFaceSquare pins TS
// PathingEntity.ts:528: Npc target (PathingEntity, not NonPathingEntity)
// passes instant=false ⇒ faceAngle written, faceSquare/mask untouched.
func TestSetInteractionNpcTargetWritesFaceAngleNoFaceSquare(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 50, 60, 0)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	wantFX := coordgrid.Fine(50, 1)
	wantFZ := coordgrid.Fine(60, 1)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("Npc target must NOT write faceSquare (got %d, %d)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&MaskFaceCoord != 0 {
		t.Errorf("Npc target must NOT set MaskFaceCoord (masks=%d)", p.masks)
	}
}

// TestSetInteractionPlayerTargetWritesFaceAngleNoFaceSquare pins TS
// PathingEntity.ts:528: Player target (PathingEntity) passes
// instant=false ⇒ faceAngle written, faceSquare/mask untouched.
func TestSetInteractionPlayerTargetWritesFaceAngleNoFaceSquare(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 50, 60, 0)
	defer otherWait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	p.SetInteraction(InteractionEngine, other, 1, -1)

	wantFX := coordgrid.Fine(50, 1)
	wantFZ := coordgrid.Fine(60, 1)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("Player target must NOT write faceSquare (got %d, %d)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&MaskFaceCoord != 0 {
		t.Errorf("Player target must NOT set MaskFaceCoord (masks=%d)", p.masks)
	}
}

// TestSetInteractionLocEngineWritesFaceSquareAndMask pins TS
// PathingEntity.ts:528: Loc target + InteractionEngine ⇒ instant=true
// path; faceSquare = (fx, fz); MaskFaceCoord ORed in.
func TestSetInteractionLocEngineWritesFaceSquareAndMask(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0
	// 3x2 Loc at (50, 60) — non-trivial sizing exercises width/length use.
	loc := entitypkg.NewLoc(0, 50, 60, 3, 2, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	wantFX := coordgrid.Fine(50, 3)
	wantFZ := coordgrid.Fine(60, 2)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != wantFX || p.faceSquareZ != wantFZ {
		t.Errorf("faceSquare: got (%d, %d), want (%d, %d)", p.faceSquareX, p.faceSquareZ, wantFX, wantFZ)
	}
	if p.masks&MaskFaceCoord == 0 {
		t.Errorf("MaskFaceCoord bit not set (masks=%d)", p.masks)
	}
}

// TestSetInteractionLocScriptDoesNotWriteFaceSquare pins TS:528 — Loc
// target + InteractionScript ⇒ instant=false (scripts don't trigger the
// engine-face wire write).
func TestSetInteractionLocScriptDoesNotWriteFaceSquare(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0
	loc := entitypkg.NewLoc(0, 50, 60, 3, 2, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionScript, loc, 1, -1)

	wantFX := coordgrid.Fine(50, 3)
	wantFZ := coordgrid.Fine(60, 2)
	// faceAngle still written on every SetInteraction (TS:528 unconditional).
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	// instant=false ⇒ faceSquare/mask untouched.
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("InteractionScript must NOT write faceSquare (got %d, %d)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&MaskFaceCoord != 0 {
		t.Errorf("InteractionScript must NOT set MaskFaceCoord (masks=%d)", p.masks)
	}
}

// TestSetInteractionObjEngineWritesFaceSquareAndMask pins TS:528 — Obj
// target + InteractionEngine ⇒ instant=true. Obj is always 1x1, so
// fine(_, 1).
func TestSetInteractionObjEngineWritesFaceSquareAndMask(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0
	obj := entitypkg.NewObj(0, 50, 60, entitypkg.LifecycleForever, 42, 1)

	p.SetInteraction(InteractionEngine, obj, 1, -1)

	wantFX := coordgrid.Fine(50, 1)
	wantFZ := coordgrid.Fine(60, 1)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != wantFX || p.faceSquareZ != wantFZ {
		t.Errorf("faceSquare: got (%d, %d), want (%d, %d)", p.faceSquareX, p.faceSquareZ, wantFX, wantFZ)
	}
	if p.masks&MaskFaceCoord == 0 {
		t.Errorf("MaskFaceCoord bit not set (masks=%d)", p.masks)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSetInteractionNpcTargetWritesFaceAngleNoFaceSquare|TestSetInteractionPlayerTargetWritesFaceAngleNoFaceSquare|TestSetInteractionLocEngineWritesFaceSquareAndMask|TestSetInteractionLocScriptDoesNotWriteFaceSquare|TestSetInteractionObjEngineWritesFaceSquareAndMask" -v`
Expected: ALL FAIL with faceAngle/faceSquare assertions failing because the helper isn't called yet.

- [ ] **Step 3: Implement the driver.** In `modules/world/interaction.go`:

(a) Add `entitypkg "github.com/zsrv/goscape/pkg/entity"` to the import block (currently L3-6: `coordgrid` + `gameserver`). New imports block:

```go
import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)
```

(b) Replace L77-100 (the body of `Player.SetInteraction` after `p.interactionFired = false`) with:

```go
	p.interactionFired = false

	// TS PathingEntity.ts:528 — focus on the target's fine coord.
	// instant=true ⇔ NonPathingEntity (Loc/Obj) clicked via the engine
	// (kind == InteractionEngine). Any other combination passes
	// instant=false: faceAngle still written, but faceSquare/mask are
	// not. Mirrors (*Npc).SetInteraction at modules/world/npc_interaction.go:660-665.
	tx, tz, _ := target.Coords()
	tw, tl := targetWidthLength(target)
	fx := coordgrid.Fine(tx, tw)
	fz := coordgrid.Fine(tz, tl)
	isNonPathing := false
	switch target.(type) {
	case *entitypkg.Loc, *entitypkg.Obj:
		isNonPathing = true
	}
	p.focus(fx, fz, isNonPathing && kind == InteractionEngine)

	switch t := target.(type) {
	case *Player:
		slot := t.slot + 32768
		if p.faceEntity != slot {
			p.faceEntity = slot
			p.masks |= p.entitymask
		}
	case *Npc:
		if p.faceEntity != t.nid {
			p.faceEntity = t.nid
			p.masks |= p.entitymask
		}
	default:
		// Loc/Obj target — cache fine-coord for reorient consumption.
		// TS PathingEntity.ts:542-545. Closes
		// NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ in NAI-66 (consumer is
		// (*Player).reorient at modules/world/movement.go).
		p.targetX = fx
		p.targetZ = fz
	}
}
```

Note the default arm now reuses `fx, fz` from the new top block — drops the `t.Coords()` / `targetWidthLength(t)` recomputation that was at the original L96-99.

- [ ] **Step 4: Run the new tests to verify they pass.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSetInteractionNpcTargetWritesFaceAngleNoFaceSquare|TestSetInteractionPlayerTargetWritesFaceAngleNoFaceSquare|TestSetInteractionLocEngineWritesFaceSquareAndMask|TestSetInteractionLocScriptDoesNotWriteFaceSquare|TestSetInteractionObjEngineWritesFaceSquareAndMask" -v`
Expected: ALL PASS.

- [ ] **Step 5: Run NAI-66 sizing tests to verify they still pass.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSetInteractionLocTargetWritesTargetXZ|TestSetInteractionObjTargetWritesTargetXZ" -v`
Expected: PASS — fx/fz reuse must preserve NAI-66 sizing semantics.

- [ ] **Step 6: Run the full module to catch regressions.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS.

- [ ] **Step 7: Race-detector run.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS.

- [ ] **Step 8: Commit.**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-67 T1.3 — Player.SetInteraction TS:528 focus() driver

Ports the missing TS PathingEntity.setInteraction:528 focus() call
into goscape's (*Player).SetInteraction. faceAngle is now written on
every call; faceSquare/MaskFaceCoord are written only when target is
NonPathingEntity (Loc/Obj) AND kind == InteractionEngine. Mirrors the
existing Npc-side pattern at modules/world/npc_interaction.go:660-665.

5 new tests pin the four (kind × target-shape) wire-write cases plus
the Player-target instant=false case.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1.4 — Bundle 1 close

**Files:** none (housekeeping commit only).

- [ ] **Step 1: Verify no residual `NAI-65-D-FOCUS-INSTANT-WIRE` references remain.**

Run: `grep -rn "NAI-65-D-FOCUS-INSTANT-WIRE" modules/ pkg/`
Expected: empty (the deviation is closed; doc-comment block was dropped in T1.1 and T1.2; no test still cites the tag).

If grep returns hits, edit the offending sites to remove the references before proceeding.

- [ ] **Step 2: Skip the close commit if grep was clean.** Bundle 1's close happens implicitly via the three feat commits. The full close + tracker update is in Task 2.4 (final NAI-67 close).

If grep returned hits and required edits, commit those:

```bash
git add -u
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world): NAI-67 B1 — drop residual NAI-65-D-FOCUS-INSTANT-WIRE references

Per retire_deviation_grep_all_comments.md: enumerate all tag mentions
at bundle close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 2 — Npc.unfocus port + respawn wire

**Goal:** Port `(*Npc).unfocus` per TS PathingEntity.ts:338-341, wire into `resetEntityForRespawn` per TS Npc.ts:280-296. Open `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` for the absent Player consumer.

### Task 2.1 — `(*Npc).unfocus` method

**Files:**
- Modify: `modules/world/npc_interaction.go` (append `(*Npc).unfocus` next to `(*Npc).focus` near L711)
- Modify: `modules/world/npc_interaction_test.go` (append `TestNpcUnfocusWritesDefaultSouthFaceAngle`)

**Test commands:**
- Targeted: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcUnfocus -v`

- [ ] **Step 1: Write the failing test.** Append to `modules/world/npc_interaction_test.go`:

```go
// TestNpcUnfocusWritesDefaultSouthFaceAngle pins TS
// PathingEntity.unfocus (PathingEntity.ts:338-341): faceAngle restored
// to fine(x, size), fine(z-1, size). Sub-pinned at size=1 and size=2.
//
// Per ts_asymmetry_dual_pin.md: explicitly assert NpcMaskFaceCoord is
// NOT ORed (TS unfocus leaves coordmask alone). Escalates if upstream
// changes that.
func TestNpcUnfocusWritesDefaultSouthFaceAngle(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"size1", 1},
		{"size2", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typ := &objtype.NpcType{Size: tc.size}
			n := NewNpc(1, 42, 100, 100, 0, typ)
			// Pre-state: distinguishable sentinels.
			n.faceAngleX = 999_999
			n.faceAngleZ = 999_999
			n.faceSquareX = -1
			n.faceSquareZ = -1
			n.masks = 0

			n.unfocus()

			wantFX := coordgrid.Fine(100, tc.size)
			wantFZ := coordgrid.Fine(100-1, tc.size)
			if n.faceAngleX != wantFX {
				t.Errorf("faceAngleX: got %d, want %d (Fine(x=100, size=%d))", n.faceAngleX, wantFX, tc.size)
			}
			if n.faceAngleZ != wantFZ {
				t.Errorf("faceAngleZ: got %d, want %d (Fine(z-1=99, size=%d))", n.faceAngleZ, wantFZ, tc.size)
			}
			// Conspicuous-absence pin: TS unfocus does NOT touch
			// faceSquare or coordmask. Per ts_asymmetry_dual_pin.md.
			if n.faceSquareX != -1 || n.faceSquareZ != -1 {
				t.Errorf("unfocus must NOT write faceSquare (got %d, %d)", n.faceSquareX, n.faceSquareZ)
			}
			if n.masks&rsbuf.NpcMaskFaceCoord != 0 {
				t.Errorf("unfocus must NOT OR NpcMaskFaceCoord (masks=%d)", n.masks)
			}
		})
	}
}
```

Verify `coordgrid` is imported in this file (`grep "coordgrid" modules/world/npc_interaction_test.go`); if not, add `"github.com/zsrv/goscape/pkg/coordgrid"`.

- [ ] **Step 2: Run the test to verify it fails.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcUnfocusWritesDefaultSouthFaceAngle -v`
Expected: FAIL with compile error "n.unfocus undefined".

- [ ] **Step 3: Implement the method.** In `modules/world/npc_interaction.go`, append immediately after `(*Npc).focus` (which ends at the new `}` from T1.2):

```go

// unfocus restores the default-south face-angle. Mirrors TS
// PathingEntity.unfocus (Engine-TS/src/engine/entity/PathingEntity.ts:338-341).
// No mask emit — TS unfocus leaves coordmask alone (faceAngle is the
// "where am I oriented" channel; mask is the wire signal, only fired
// from focus(instant=true) or FaceSquare).
//
// Caller: resetEntityForRespawn (modules/world/npc_registry.go), the
// goscape-shape equivalent of TS Npc.resetEntity(true) at Npc.ts:284.
func (n *Npc) unfocus() {
	n.faceAngleX = coordgrid.Fine(n.x, n.size)
	n.faceAngleZ = coordgrid.Fine(n.z-1, n.size)
}
```

Verify `coordgrid` is already imported in `modules/world/npc_interaction.go` (`grep "coordgrid" modules/world/npc_interaction.go`). It is.

- [ ] **Step 4: Run the test to verify it passes.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcUnfocusWritesDefaultSouthFaceAngle -v`
Expected: PASS (both `size1` and `size2` subtests).

- [ ] **Step 5: Commit.**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-67 T2.1 — (*Npc).unfocus port

Ports TS PathingEntity.unfocus (PathingEntity.ts:338-341): restores
faceAngleX/Z to Fine(x, size) / Fine(z-1, size). No mask emit.

(*Player).unfocus deferred — no consumer at HEAD (Player respawn /
death flow not ported). Tracked as NAI-67-D-PLAYER-UNFOCUS-DEFERRED
in T2.3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.2 — Wire `unfocus` into `resetEntityForRespawn`

**Files:**
- Modify: `modules/world/npc_registry.go` (insert `n.unfocus()` at the top of `resetEntityForRespawn` body, L116)
- Modify: `modules/world/npc_registry_test.go` (append `TestResetEntityForRespawnInvokesUnfocus`)

**Test commands:**
- Targeted: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResetEntityForRespawnInvokesUnfocus -v`

- [ ] **Step 1: Write the failing test.** Append to `modules/world/npc_registry_test.go`:

```go
// TestResetEntityForRespawnInvokesUnfocus pins TS Npc.resetEntity(true)
// at Npc.ts:284 — calls super.unfocus() to restore default-south
// face-angle. Goscape's resetEntityForRespawn is the goscape-shape
// equivalent of that branch.
func TestResetEntityForRespawnInvokesUnfocus(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1}
	n := newRegisteredNpc(t, s, typ, false)
	// Pre-state: simulate post-interaction face-angle drift.
	n.faceAngleX = 999_999
	n.faceAngleZ = 999_999

	s.resetEntityForRespawn(n)

	wantFX := coordgrid.Fine(n.x, n.size)
	wantFZ := coordgrid.Fine(n.z-1, n.size)
	if n.faceAngleX != wantFX {
		t.Errorf("faceAngleX: got %d, want %d (Fine(n.x=%d, size=%d))", n.faceAngleX, wantFX, n.x, n.size)
	}
	if n.faceAngleZ != wantFZ {
		t.Errorf("faceAngleZ: got %d, want %d (Fine(n.z-1=%d, size=%d))", n.faceAngleZ, wantFZ, n.z-1, n.size)
	}
}
```

Verify `coordgrid` is imported in `modules/world/npc_registry_test.go` (`grep "coordgrid" modules/world/npc_registry_test.go`); if not, add `"github.com/zsrv/goscape/pkg/coordgrid"`.

- [ ] **Step 2: Run the test to verify it fails.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResetEntityForRespawnInvokesUnfocus -v`
Expected: FAIL — `faceAngleX` still equals 999999 because nothing calls `unfocus`.

- [ ] **Step 3: Wire `unfocus()` into `resetEntityForRespawn`.** In `modules/world/npc_registry.go`, modify `resetEntityForRespawn` at L115-140. Insert `n.unfocus()` as the first statement:

```go
func (s *Server) resetEntityForRespawn(n *Npc) {
	// TS Npc.resetEntity(true) at Npc.ts:284 — restore default-south
	// face-angle. Reads n.x, n.z, n.size; none are mutated by the
	// rest of this function so the call order is safe at the top.
	n.unfocus()

	if n.typeId != n.baseType {
		n.typeId = n.baseType
		n.uid = (n.typeId << 16) | n.nid
		if newTyp := n.lookupType(n.baseType); newTyp != nil {
			n.typ = newTyp
		}
		n.masks |= rsbuf.NpcMaskChangeType
	}
	// (rest of function unchanged — typ stat reseed, queue/waypoint
	// clear, hunt-field reseed.)
```

Note: only the call insertion is shown; do not modify the rest of the function body.

- [ ] **Step 4: Run the test to verify it passes.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResetEntityForRespawnInvokesUnfocus -v`
Expected: PASS.

- [ ] **Step 5: Run the full module to catch regressions.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS. (NAI-20 / NAI-66 existing tests of `resetEntityForRespawn` should not break — `unfocus()` only writes faceAngle; existing tests pin `masks`, `typeId`, stats.)

- [ ] **Step 6: Race-detector run.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-67 T2.2 — wire unfocus into resetEntityForRespawn

Mirrors TS Npc.resetEntity(true) at Npc.ts:284 — restores default-south
face-angle on respawn. Inserted at top of function (reads n.x/n.z/n.size,
none mutated by the function body).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.3 — NAI-67 close + tracker entry

**Files:**
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append NAI-67 close section)
- Memory entry name reused: `nai_followups.md`

**No production code changes — close commit only.**

- [ ] **Step 1: Run full test suite.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (all packages).

- [ ] **Step 2: Verify zero residual NAI-65-D-FOCUS-INSTANT-WIRE references repo-wide.**

Run: `grep -rn "NAI-65-D-FOCUS-INSTANT-WIRE" .`
Expected: only references inside `docs/superpowers/specs/2026-05-01-nai-65-pathing-entity-focus-stride-partial-design.md` (historical) and `docs/superpowers/specs/2026-05-02-nai-67-focus-instant-wire-and-unfocus-port-design.md` (this spec). No production / test / memory hits.

- [ ] **Step 3: Append NAI-67 entry to nai_followups.md memory.**

Open `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` and append (after the existing NAI-66 close section):

```markdown

---

## NAI-67 — CLOSED 2026-05-02

**Scope:** focus instant=true wire branch for both `(*Player).focus` and `(*Npc).focus` (TS PathingEntity.ts:321-333), `(*Player).SetInteraction` TS:528 driver port, and `(*Npc).unfocus` port wired into `resetEntityForRespawn` (TS Npc.ts:280-296).

**Cadence:** Full sub-spec, two bundles. B1 (focus-family wire + driver) closed `NAI-65-D-FOCUS-INSTANT-WIRE` and surfaced+closed the previously-untagged `(*Player).SetInteraction` missing focus() divergence in the same bundle. B2 (Npc.unfocus + respawn wire) opened `NAI-67-D-PLAYER-UNFOCUS-DEFERRED`.

**Spec:** `docs/superpowers/specs/2026-05-02-nai-67-focus-instant-wire-and-unfocus-port-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-02-nai-67-focus-instant-wire-and-unfocus-port.md`.

**Close commit:** (this commit). T1.1: <SHA>. T1.2: <SHA>. T1.3: <SHA>. T2.1: <SHA>. T2.2: <SHA>.

**Follow-ups closed:**
- `NAI-65-D-FOCUS-INSTANT-WIRE` — both focus helpers now do the wire write on instant=true; (*Npc).SetInteraction's existing site at npc_interaction.go:665 instantly produces correct wire bits; (*Player).SetInteraction now also drives instant-true via its newly-ported focus() call per TS:528.

**Deviations opened:** `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` — `(*Player).unfocus` not ported; no consumer at HEAD (Player death/respawn flow not ported per `modules/world/player_masks.go:90` "future combat sub-spec" boundary). TS caller is Player.resetEntity(true) at Player.ts:454-457. Closure: future Player respawn / death sub-spec wires `p.unfocus()` into the to-be-written respawn path.

**Deviations closed:** `NAI-65-D-FOCUS-INSTANT-WIRE`.

**Net deviation tally:** -1 closure, +1 open = 13 → 13 (unchanged).

**Wire-behaviour delta at HEAD:**
- B1 activates real wire writes on (*Npc).SetInteraction's existing engine-clicked Loc/Obj branch (npc_interaction.go:665) AND on the newly-ported (*Player).SetInteraction equivalent. Previously both stored the instant flag write-only.
- B2 wires `n.unfocus()` into respawn — small face-angle correction at NPC respawn; observable via the wire only via subsequent reorient/SetInteraction overwrites, but TS-fidelity correct.

**Memory entries reinforced (no edits needed):**
- `runescript_cadence.md` — full two-bundle cadence.
- `true_to_ts_gate.md` — every behavioural change cited against TS PathingEntity.ts and Npc.ts source lines.
- `dead_api_polish.md` — drove the `(*Player).unfocus` deferral (no consumer ⇒ no helper).
- `ts_asymmetry_dual_pin.md` — both focus helpers' instant=false/true cases dual-pinned; Npc.unfocus's "no mask emit" absence-pinned.
- `controller_preflight.md` — pre-flight grep gates ran clean.
- `enumerate_all_sites.md` — pre-flight enumeration of all `focus(` callers (11 sites at HEAD).
- `plan_grep_helper_patterns.md` — `targetWidthLength` reused in T1.3, not inlined.
- `plan_var_name_collision.md` — `tx, tz, fx, fz` checked against scope at plan-write time.
- `audit_full_method_against_ts.md` — Player.SetInteraction full-method audit caught the missing focus() call.
- `ts_base_class_read_for_inherited_behavior.md` — TS Npc respawn unfocus call traced to the leaf (Npc.ts:284), goscape's matching site is `resetEntityForRespawn`.
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:` trailer.

**Carry-forwards (still open after NAI-67):**
- `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` (new) — Player.unfocus port, blocked on Player respawn / death sub-spec.
- `NAI-34-D4-NPC` + `NAI-34-D5-NPC` — permanent dead-API skip (unchanged).
- `NAI-35-T3-D1` op[1] operability gate audit (unchanged).
- All other deferred carry-forwards from NAI-65 / NAI-66 (NAI-37 / NAI-44 walktrigger, NAI-40-SB1/SB2/SB4, NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET, NAI-44-D-CANACCESS-NO-STUN-CHECK, NAI-59-D-MODALTUTORIAL-NO-PRODUCER) unchanged.
```

Replace `<SHA>` placeholders by running `git log --oneline -10` after the close commit and back-filling. (The close commit can carry the placeholders if backfilling is too costly; they're informational.)

- [ ] **Step 4: Final close commit.**

Replace the `<SHAs>` in the body with the actual short commit SHAs from `git log --oneline -10` (T1.1..T1.3 in B1, T2.1..T2.2 in B2). Then commit:

```bash
git -C $HOME/Code/github.com/zsrv/goscape commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-67 — focus instant-wire port + Player.SetInteraction driver + Npc.unfocus port

B1 (focus-family wire + driver) closes NAI-65-D-FOCUS-INSTANT-WIRE.
Both (*Player).focus and (*Npc).focus instant=true branch now writes
faceSquareX/Z and ORs the coord mask per TS PathingEntity.ts:321-333.
(*Player).SetInteraction now calls p.focus(...) per TS:528 with
isNonPathing && kind == InteractionEngine — the previously-untagged
divergence.

B2 (Npc.unfocus + respawn wire) ports (*Npc).unfocus per TS
PathingEntity.ts:338-341 and wires it into resetEntityForRespawn
mirroring TS Npc.ts:284. (*Player).unfocus deferred — opens
NAI-67-D-PLAYER-UNFOCUS-DEFERRED (no consumer at HEAD).

Net deviation tally: 13 → 13 (one closed, one opened).

Closes memory: nai_followups.md NAI-67

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Note: `--allow-empty` because no production-code change is in this commit — the close is informational. Memory file edit is outside the repo and not part of this commit.)

- [ ] **Step 5: Save the memory file.** Memory edits to `$HOME/.claude/projects/.../nai_followups.md` are outside the goscape repo and persist independently — no commit needed.

---

## Post-flight verification

After T2.3:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — all PASS.
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...` — PASS.
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — clean.
- [ ] `grep -rn "NAI-65-D-FOCUS-INSTANT-WIRE" modules/ pkg/ cmd/` — zero hits (only docs/specs reference it).
- [ ] `grep -rn "NAI-67-D-PLAYER-UNFOCUS-DEFERRED" .` — at least one hit in `nai_followups.md` (and possibly the spec/plan docs).
- [ ] `git log --oneline main..HEAD` shows: 5 feat commits (T1.1, T1.2, T1.3, T2.1, T2.2) + 1 chore close (T2.3). T1.4 is conditional and may or may not be present.
