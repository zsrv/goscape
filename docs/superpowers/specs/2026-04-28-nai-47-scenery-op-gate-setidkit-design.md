# NAI-47 — Scenery-OP Gate + SETIDKIT Opcode

## Motivation

Two deferred items from the NAI-45/NAI-46 cluster are now unblocked:

1. **NAI-46-F1** (`Player.tryInteract allowOpScenery gate`): TS `Player.ts:1113`
   takes `allowOpScenery: boolean` and gates OP fires for Loc/Obj targets behind
   `(target instanceof PathingEntity || allowOpScenery)`. Goscape's `tryInteract()`
   has no such gate, so scenery items can be op'd on the same tick as movement
   (diverging from TS). The NPC side already implements this correctly at
   `npc_interaction.go:247` — the Player side is the only gap.

2. **SETIDKIT** (`OpSetIdKit = 2100`): declared in `pkg/script/opcode.go:200` but
   has no handler. NAI-46 deferred it pending IdkType; IdkType now exists
   post-NAI-46. Handler pops `(idkit, color)`, validates idkit via
   `Configs.IdkType`, adjusts slot for gender, and writes `body[slot]` and
   `colors[colorSlot]` on the active player.

**TS reference:**
- `Engine-TS/src/engine/entity/Player.ts:1113-1188` (`tryInteract`)
- `Engine-TS/src/engine/entity/Player.ts:1200-1264` (`processInteraction` — call sites)
- `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1066-1106` (SETIDKIT)

## Tech Stack

**Go 1.26+** (modern syntax). All `go` commands: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`. All commits: `--no-gpg-sign`.

## Deviations

No new deviations opened. No existing deviations retired.

## Scope

**In scope:**

- `modules/world/interaction.go` — add `allowOpScenery bool` to `tryInteract`,
  add isPathing gate, update 2 call sites
- `modules/world/interaction_test.go` — 4 new gate tests
- `pkg/script/configs.go` — add `IdkType(id int) *objtype.IdkType`
- `modules/world/server_configs.go` — implement `IdkType`
- `pkg/script/handlers_config_test.go` — add `idks` field + method to `mockConfigs`
- `pkg/script/handlers_loc_test.go` — add `IdkType` stub to `fakeConfigs`
- `pkg/script/active.go` — add `Gender() int`, `SetBodyPart(slot, idkit int)`,
  `SetColorPart(slot, color int)` to `ActivePlayer`
- `modules/world/player_script.go` — implement 3 new methods on `*Player`
- `pkg/script/runner_test.go` — add `genderValue`, `bodyParts`, `colorParts` fields
  + 3 new methods to `mockPlayer`
- `pkg/script/handlers_player.go` — add `handleSetIdKit` + `idkColorSlot`
- `pkg/script/handlers.go` — register `OpSetIdKit: handleSetIdKit`
- `pkg/script/handlers_player_test.go` — 7 new SETIDKIT tests

**Out of scope:**

- SETGENDER opcode (2099) — separate future sub-spec
- `MaskAppearance` raise from SETIDKIT — TS does not set it; script must call
  BUILDAPPEARANCE separately
- `IdkType` exposure for other consumers (none exist today)

---

## Pre-flight (HEAD `2aea3df`)

| Claim | Result |
|---|---|
| `tryInteract()` at `interaction.go:244` — no params | ✓ |
| Pre-step call at `interaction.go:169`: `p.tryInteract()` | ✓ |
| Post-step call at `interaction.go:192`: `p.tryInteract()` | ✓ |
| `OpSetIdKit Opcode = 2100` at `opcode.go:200` | ✓ |
| No `handleSetIdKit` in `handlers.go` or `handlers_player.go` | ✓ absent |
| `Configs` interface (`configs.go:10`) — no `IdkType` method | ✓ absent |
| `server_configs.go` — no `IdkType` method | ✓ absent |
| `mockConfigs` at `handlers_config_test.go:11` — no `idks` field | ✓ absent |
| `fakeConfigs` at `handlers_loc_test.go:16` — no `IdkType` method | ✓ absent |
| `ActivePlayer` at `active.go:6` — no `Gender`/`SetBodyPart`/`SetColorPart` | ✓ absent |
| `mockPlayer` at `runner_test.go` — no `genderValue`/`bodyParts`/`colorParts` | ✓ absent |
| `s.idkTypes *objtype.IdkTypeConfigs` on `Server` | ✓ present (NAI-46) |
| `interaction_test.go` imports `entitypkg` | ✓ present |
| NPC-side `tryInteract(s, allowOpScenery bool)` at `npc_interaction.go:247` | ✓ reference template |

---

## File Map

| Action | Path | What changes |
|--------|------|-------------|
| MODIFY | `modules/world/interaction.go:244` | `tryInteract()` → `tryInteract(allowOpScenery bool)` + isPathing gate |
| MODIFY | `modules/world/interaction.go:169,192` | Update 2 call sites |
| MODIFY | `modules/world/interaction_test.go` | Add 4 gate tests |
| MODIFY | `pkg/script/configs.go` | Add `IdkType` method |
| MODIFY | `modules/world/server_configs.go` | Implement `IdkType` (after `InvType`) |
| MODIFY | `pkg/script/handlers_config_test.go` | Add `idks` + `IdkType` to `mockConfigs` |
| MODIFY | `pkg/script/handlers_loc_test.go` | Add `IdkType` stub to `fakeConfigs` |
| MODIFY | `pkg/script/active.go` | Add 3 methods to `ActivePlayer` (after `PlayJingle`) |
| MODIFY | `modules/world/player_script.go` | Implement 3 methods on `*Player` (near `LowMemory`) |
| MODIFY | `pkg/script/runner_test.go` | Add fields + 3 methods to `mockPlayer` |
| MODIFY | `pkg/script/handlers_player.go` | Add `handleSetIdKit` + `idkColorSlot` |
| MODIFY | `pkg/script/handlers.go` | Register `OpSetIdKit: handleSetIdKit` |
| MODIFY | `pkg/script/handlers_player_test.go` | Add 7 SETIDKIT tests |

---

## Task 1 — `tryInteract` allowOpScenery gate (NAI-46-F1)

**Files:** `modules/world/interaction.go`, `modules/world/interaction_test.go`

**TS reference:** `Player.ts:1113-1188` (`tryInteract`), `Player.ts:1223,1245` (call sites).

**Template:** `(*Npc).tryInteract(s, allowOpScenery bool)` at `npc_interaction.go:247-271`.

### 1a. Write failing tests

Add to `modules/world/interaction_test.go` (after existing `effectiveApRange` tests):

```go
// --- NAI-47: tryInteract allowOpScenery gate ---

// TestTryInteractNpcAllowsOpWhenSceneryGated pins that *Npc targets (PathingEntity)
// are always eligible for the OP branch regardless of allowOpScenery.
// Mirrors TS: (target instanceof PathingEntity || allowOpScenery).
func TestTryInteractNpcAllowsOpWhenSceneryGated(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	npc := makeInteractionNpc(t, s, 1, 101, 100, 0) // adjacent — in OP range
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	// allowOpScenery=false: NPC is PathingEntity so OP fires anyway.
	result := p.tryInteract(false)

	if !result {
		t.Error("tryInteract(false): got false, want true — NPC is PathingEntity, OP must fire")
	}
}

// TestTryInteractLocBlocksOpWhenSceneryFalse pins that *Loc targets cannot
// fire the OP branch when allowOpScenery=false. AP branch fires instead
// if in approach range.
func TestTryInteractLocBlocksOpWhenSceneryFalse(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.apRange = 10 // wide AP range so AP branch fires

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	// allowOpScenery=false + adjacent Loc → OP gated; AP fires instead (returns true).
	result := p.tryInteract(false)

	// AP branch fires (returns true) because the OP gate falls through to AP.
	if !result {
		t.Error("tryInteract(false) on adjacent Loc: got false, want true (AP fires)")
	}
	// interactionFired was set by tryFireApTrigger (no real script → marked fired anyway).
}

// TestTryInteractLocAllowsOpWhenSceneryTrue pins that *Loc targets CAN fire
// the OP branch when allowOpScenery=true.
func TestTryInteractLocAllowsOpWhenSceneryTrue(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	// allowOpScenery=true + adjacent Loc → OP fires.
	result := p.tryInteract(true)

	if !result {
		t.Error("tryInteract(true) on adjacent Loc: got false, want true (OP allowed)")
	}
}

// TestTryInteractProcessInteractionCallSites pins the two call-site semantics
// via processInteraction: pre-step always passes false, post-step passes
// stepsTaken==0 (true only when no movement this tick).
func TestTryInteractProcessInteractionCallSites(t *testing.T) {
	s := newTestServer(t)

	// Scenario: Loc target, player already adjacent (no movement needed),
	// so post-step call gets allowOpScenery=true (stepsTaken==0).
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.stepsTaken = 0 // no movement this tick

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	p.processInteraction()

	// OP or AP fired (interacted=true), and interaction was auto-cleared.
	if p.target != nil {
		t.Error("target should be nil after interaction auto-clear")
	}
}
```

**Run failing:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... \
  -run 'TestTryInteract' -v 2>&1 | head -20
```

Expected: compile failure (`tryInteract` call with bool arg doesn't match `tryInteract()` signature).

### 1b. Implement the gate

In `modules/world/interaction.go`, replace the full `tryInteract` function (lines 241-261):

**Old:**
```go
// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined.
// Returns true when an OP or AP trigger fired this tick.
func (p *Player) tryInteract() bool {
	tx, tz, _ := p.target.Coords()
	if inOperableDistance(p.x, p.z, tx, tz) {
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return true
	}
	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		return true
	}
	return false
}
```

**New:**
```go
// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined.
// Returns true when an OP or AP trigger fired this tick.
//
// allowOpScenery gates the OP branch for non-PathingEntity targets
// (Loc, Obj). Mirrors TS Player.tryInteract(allowOpScenery: boolean)
// at Player.ts:1113. Callers:
//   - pre-step (always false): scenery OP blocked before movement
//   - post-step (stepsTaken==0): scenery OP allowed only if no walk
//
// NPC side equivalent: (*Npc).tryInteract(s, allowOpScenery bool)
// at npc_interaction.go:247.
func (p *Player) tryInteract(allowOpScenery bool) bool {
	tx, tz, _ := p.target.Coords()
	if inOperableDistance(p.x, p.z, tx, tz) {
		_, isNpc := p.target.(*Npc)
		_, isPlayer := p.target.(*Player)
		if isNpc || isPlayer || allowOpScenery {
			p.interacted = true
			if !p.interactionFired {
				tryFireOpTrigger(p)
			}
			return true
		}
		// Loc/Obj + !allowOpScenery: fall through to AP check.
	}
	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		return true
	}
	return false
}
```

### 1c. Update call sites

In `processInteraction()`:

Line 169: `interacted = p.tryInteract()` → `interacted = p.tryInteract(false)`

Line 192: `interacted = p.tryInteract()` → `interacted = p.tryInteract(p.stepsTaken == 0)`

### 1d. Verify + commit

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... \
  -run 'TestTryInteract|TestProcessInteraction' -v 2>&1 | tail -20
```

Expected: all new tests PASS, existing processInteraction tests still PASS.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: PASS.

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-47 T1 — tryInteract allowOpScenery gate (NAI-46-F1)

Adds allowOpScenery bool to (*Player).tryInteract, gating OP fires for
non-PathingEntity targets (Loc/Obj). Pre-step passes false; post-step
passes stepsTaken==0. Mirrors TS Player.ts:1113-1188 and the already-
correct (*Npc).tryInteract(s, allowOpScenery) at npc_interaction.go:247.
4 new tests cover NPC-overrides, Loc-blocked, Loc-allowed, and call-site
semantics.

Closes memory: NAI-46-F1

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — `Configs.IdkType` interface + server implementation + mock stubs

**Files:** `pkg/script/configs.go`, `modules/world/server_configs.go`,
`pkg/script/handlers_config_test.go`, `pkg/script/handlers_loc_test.go`

This task is a compile prerequisite for Task 4 (handler). No new tests; the
mocks must compile.

### 2a. Add method to `pkg/script/configs.go`

After `InvType(id int) *objtype.InvType`, add:

```go
IdkType(id int) *objtype.IdkType
```

Full Configs interface now has 11 methods (was 10 + the 2 FindDbRows variants).

### 2b. Add implementation to `modules/world/server_configs.go`

After `InvType` (line 79), add:

```go
func (c serverConfigsView) IdkType(id int) *objtype.IdkType {
	if c.s == nil || c.s.idkTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.idkTypes.Configs) {
		return nil
	}
	return c.s.idkTypes.Configs[id]
}
```

### 2c. Update `mockConfigs` in `pkg/script/handlers_config_test.go`

Add field to struct (after `invs` on line 18):
```go
idks    map[int]*objtype.IdkType
```

Add method (after `InvType` line 27):
```go
func (m *mockConfigs) IdkType(id int) *objtype.IdkType { return m.idks[id] }
```

### 2d. Add `IdkType` stub to `fakeConfigs` in `pkg/script/handlers_loc_test.go`

After the `InvType` stub (line 26):
```go
func (f *fakeConfigs) IdkType(id int) *objtype.IdkType       { return nil }
```

### 2e. Compile check

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1
```

Expected: no output (clean build).

### 2f. Commit

```bash
git add pkg/script/configs.go modules/world/server_configs.go \
        pkg/script/handlers_config_test.go pkg/script/handlers_loc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-47 T2 — Configs.IdkType + server implementation + mock stubs

Adds IdkType(id int) *objtype.IdkType to the script.Configs interface,
implements it on serverConfigsView (follows ObjType/InvType pattern),
and updates mockConfigs + fakeConfigs stubs so the codebase compiles.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — `ActivePlayer` interface additions + `mockPlayer` + `*Player` implementations

**Files:** `pkg/script/active.go`, `modules/world/player_script.go`,
`pkg/script/runner_test.go`

### 3a. Add 3 methods to `ActivePlayer` in `pkg/script/active.go`

After `PlayJingle(delay int, name string)` (line 432), before the closing `}`:

```go
	// NAI-47: SETIDKIT appearance mutation.

	// Gender returns the player's gender (0=male, 1=female). Used by SETIDKIT
	// to determine the body-part slot offset (female slots = type − 7).
	// Mirrors TS state.activePlayer.gender at PlayerOps.ts:1073.
	Gender() int

	// SetBodyPart writes body[slot] = idkit. Called by SETIDKIT after slot
	// computation. Does NOT flip MaskAppearance — the script must call
	// BUILDAPPEARANCE separately (TS pattern: SETIDKIT then BUILDAPPEARANCE).
	// Mirrors TS state.activePlayer.body[slot] = idkType.id at PlayerOps.ts:1079.
	SetBodyPart(slot, idkit int)

	// SetColorPart writes colors[slot] = color. Called by SETIDKIT for the
	// color slot that corresponds to the body-part type (0/1→0, 2/3→1, 5→2,
	// 6→3; type 4 has no color write). Mirrors TS state.activePlayer.colors
	// at PlayerOps.ts:1102.
	SetColorPart(slot, color int)
```

### 3b. Implement on `*Player` in `modules/world/player_script.go`

Add near `LowMemory()` (line 700):

```go
// NAI-47: SETIDKIT appearance mutation.
func (p *Player) Gender() int                      { return p.gender }
func (p *Player) SetBodyPart(slot, idkit int)      { p.body[slot] = idkit }
func (p *Player) SetColorPart(slot, color int)     { p.colors[slot] = color }
```

### 3c. Add fields and methods to `mockPlayer` in `pkg/script/runner_test.go`

Add fields to the `mockPlayer` struct after `lowMemoryValue bool` (around line 266):

```go
	// NAI-47: SETIDKIT gender + appearance captures.
	genderValue int
	bodyParts   [7]int
	colorParts  [5]int
```

Add methods (after `LowMemory()` around line 513):

```go
// NAI-47: SETIDKIT appearance-mutation captures.
func (m *mockPlayer) Gender() int                      { return m.genderValue }
func (m *mockPlayer) SetBodyPart(slot, idkit int)      { m.bodyParts[slot] = idkit }
func (m *mockPlayer) SetColorPart(slot, color int)     { m.colorParts[slot] = color }
```

### 3d. Compile + full test run

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1
```

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
```

Expected: clean build and all PASS.

### 3e. Commit

```bash
git add pkg/script/active.go modules/world/player_script.go \
        pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-47 T3 — ActivePlayer Gender/SetBodyPart/SetColorPart + mockPlayer

Adds three appearance-mutation methods to the ActivePlayer interface for
SETIDKIT: Gender() int, SetBodyPart(slot, idkit int), SetColorPart(slot,
color int). Implements on *Player and extends mockPlayer with capture
fields. No MaskAppearance side-effect — callers must call BUILDAPPEARANCE.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — `handleSetIdKit` handler + registration

**Files:** `pkg/script/handlers_player.go`, `pkg/script/handlers.go`

**TS reference:** `PlayerOps.ts:1066-1106`.

### 4a. Write failing tests first (in `handlers_player_test.go`)

Add to `pkg/script/handlers_player_test.go`:

```go
// --- NAI-47: handleSetIdKit ---

func buildIdkTypeConfig(id, typ int) *objtype.IdkType {
	c := objtype.NewIdkType(id)
	c.Type = typ
	return c
}

func TestHandleSetIdKitRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{}
	s.PushInt(0)
	s.PushInt(0)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for no active player, got nil")
	}
}

func TestHandleSetIdKitNilConfigs(t *testing.T) {
	s := &ScriptState{Pointers: PtrActivePlayer, Self: &mockPlayer{}}
	s.PushInt(0) // idkit (pushed first = below)
	s.PushInt(0) // color (pushed last = top)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for nil Configs, got nil")
	}
}

func TestHandleSetIdKitInvalidIdkit(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{}}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: &mockPlayer{}, Configs: mc}
	s.PushInt(5) // idkit=5 — not in registry (pushed first = below)
	s.PushInt(0) // color (pushed last = top)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for invalid idkit, got nil")
	}
}

// TestHandleSetIdKitMaleHair: gender=0, idkType.Type=0 (hair) → body[0]=idkit,
// colors[0]=color (hair colorSlot).
func TestHandleSetIdKitMaleHair(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{3: buildIdkTypeConfig(3, 0)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc}
	s.PushInt(3) // idkit=3 (Type=0, male hair) — pushed first = below
	s.PushInt(7) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[0] != 3 {
		t.Errorf("bodyParts[0]: got %d, want 3 (idkit id)", mp.bodyParts[0])
	}
	if mp.colorParts[0] != 7 {
		t.Errorf("colorParts[0]: got %d, want 7 (hair color)", mp.colorParts[0])
	}
}

// TestHandleSetIdKitFemaleSlotAdjust: gender=1, idkType.Type=7 (female hair).
// slot = 7 − 7 = 0, adjustedType = 0 → colorSlot=0.
func TestHandleSetIdKitFemaleSlotAdjust(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{9: buildIdkTypeConfig(9, 7)}}
	mp := &mockPlayer{genderValue: 1}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc}
	s.PushInt(9) // idkit=9 (Type=7 → female hair, slot=0) — pushed first = below
	s.PushInt(2) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[0] != 9 {
		t.Errorf("bodyParts[0]: got %d, want 9", mp.bodyParts[0])
	}
	if mp.colorParts[0] != 2 {
		t.Errorf("colorParts[0]: got %d, want 2", mp.colorParts[0])
	}
}

// TestHandleSetIdKitSkinNoColorWrite: Type=4 (hands) has no color slot.
// colorParts must stay at zero defaults.
func TestHandleSetIdKitSkinNoColorWrite(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{4: buildIdkTypeConfig(4, 4)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc}
	s.PushInt(4)  // idkit=4 (Type=4, hands/skin) — pushed first = below
	s.PushInt(99) // color (should not be written) — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[4] != 4 {
		t.Errorf("bodyParts[4]: got %d, want 4", mp.bodyParts[4])
	}
	for i, v := range mp.colorParts {
		if v != 0 {
			t.Errorf("colorParts[%d]: got %d, want 0 (no color write for Type=4)", i, v)
		}
	}
}

// TestHandleSetIdKitLegs: Type=5 → colorSlot=2.
func TestHandleSetIdKitLegs(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{5: buildIdkTypeConfig(5, 5)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc}
	s.PushInt(5)  // idkit=5 (Type=5, legs) — pushed first = below
	s.PushInt(11) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.colorParts[2] != 11 {
		t.Errorf("colorParts[2]: got %d, want 11 (legs colorSlot=2)", mp.colorParts[2])
	}
}
```

**Run failing:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... \
  -run 'TestHandleSetIdKit' -v 2>&1 | head -15
```

Expected: compile failure (`handleSetIdKit` undefined).

### 4b. Implement `handleSetIdKit` in `pkg/script/handlers_player.go`

Add after `handleBuildAppearance` (near line 155):

```go
// handleSetIdKit (SETIDKIT, opcode 2100) sets one body-part slot on the
// active player's appearance. Pops (idkit int, color int) from the stack.
// Validates idkit via Configs.IdkType; writes body[slot] and
// colors[colorSlot] (slot adjusted for gender). Script must call
// BUILDAPPEARANCE separately to trigger the appearance rebuild.
// Mirrors TS PlayerOps.ts:1066-1106.
func handleSetIdKit(s *ScriptState) error {
	if err := requireActivePlayer(s, "SETIDKIT"); err != nil {
		return err
	}
	color := s.PopInt()
	idkit := s.PopInt()
	if s.Configs == nil || s.Configs.IdkType(idkit) == nil {
		return fmt.Errorf("SETIDKIT: invalid idkit %d", idkit)
	}
	idk := s.Configs.IdkType(idkit)
	gender := s.Self.Gender()
	slot := idk.Type
	if gender == 1 {
		slot -= 7
	}
	s.Self.SetBodyPart(slot, idkit)
	adjustedType := idk.Type
	if gender == 1 {
		adjustedType -= 7
	}
	if cs := idkColorSlot(adjustedType); cs >= 0 {
		s.Self.SetColorPart(cs, color)
	}
	return nil
}

// idkColorSlot maps the gender-adjusted body-part type (0-6) to the
// colors array index. Returns -1 when no color write is needed (type=4,
// hands/skin — set via SETSKINCOLOUR instead).
// Mirrors TS PlayerOps.ts:1082-1103 color-slot mapping.
func idkColorSlot(t int) int {
	switch t {
	case 0, 1:
		return 0 // hair / jaw
	case 2, 3:
		return 1 // torso / arms
	case 5:
		return 2 // legs
	case 6:
		return 3 // feet
	default:
		return -1 // type=4 (hands/skin): no color write
	}
}
```

Ensure `"fmt"` is in the import block (it already is — the file uses `fmt.Errorf` elsewhere).

### 4c. Register in `pkg/script/handlers.go`

In the handler dispatch table, after `OpSay` or alphabetically near `OpSetIdKit`, add:

```go
OpSetIdKit: handleSetIdKit,
```

### 4d. Verify + commit

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... \
  -run 'TestHandleSetIdKit' -v 2>&1 | tail -20
```

Expected: all 7 SETIDKIT tests PASS.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
```

Expected: all packages PASS.

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go \
        pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-47 T4 — handleSetIdKit opcode handler (SETIDKIT 2100)

Ports TS PlayerOps.ts:1066-1106. Pops (idkit, color), validates via
Configs.IdkType, adjusts slot for gender (female: type−7), writes
body[slot]=idkit and colors[idkColorSlot(adjustedType)]=color. Type=4
(hands) has no color write. Script must call BUILDAPPEARANCE separately.
Registers OpSetIdKit in the handler dispatch table.
7 tests cover: no-active-player, nil-Configs, invalid-idkit, male-hair,
female-slot-adjust, skin-no-color, and legs-colorSlot-2.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Close commit

After all tasks pass:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | grep -E 'ok|FAIL'
```

Expected: all packages `ok`, no `FAIL`.

```bash
git tag -a "nai-47-close" -m "NAI-47 close: scenery-OP gate + SETIDKIT"
```

(Optional tag — omit if not using tags.)

---

## Deviation tally

- Retired: none
- Opened: none
- Net: ±0

---

## Close commit trailer (for NAI-47 merge commit)

```
Closes memory: NAI-46-F1
```
