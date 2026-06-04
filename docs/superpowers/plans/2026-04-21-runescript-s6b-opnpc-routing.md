# RuneScript S6b: OPNPC Trigger Routing + NPC_SAY Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fire `[opnpc<N>,<npcType>]` scripts with `Self=player, ActiveNpc=npc` when a player reaches interaction range of an anchored NPC target. Ship `NPC_SAY` alongside so dispatch is demonstrable on the wire.

**Architecture:** New `modules/world/interaction_trigger.go` owns `tryFireOpTrigger(p)`, called from the reach-success branch of `Player.processInteraction`. A one-shot `Player.interactionFired` gate prevents per-tick re-dispatch while the player stands adjacent. `NPC_SAY` rides on the pre-existing `NpcMaskSay` encoder (no new world-module production code).

**Tech Stack:** Go; `pkg/script` runtime; `modules/world` tick loop; existing `rsbuf` NPC-info mask encoder.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6b-opnpc-routing-design.md`](../specs/2026-04-21-runescript-s6b-opnpc-routing-design.md) (commit `1eb769c`)

---

## Task 1: NPC_SAY — ActiveNpc.Say + handler + tests

**Files:**
- Modify: `pkg/script/active.go` (extend `ActiveNpc` interface)
- Modify: `pkg/script/handlers_npc.go` (new `handleNpcSay`)
- Modify: `pkg/script/handlers.go` (register `OpNpcSay`)
- Modify: `pkg/script/handlers_npc_test.go` (extend `mockNpc`, two tests)

- [ ] **Step 1: Extend the `ActiveNpc` interface.** Open `pkg/script/active.go`. The interface currently ends after `SetNpcVarN(id int, val int32)` (around line 273). Insert immediately before the closing `}`:

```go
	// Say buffers text as the NPC's speech bubble for the current tick,
	// flagging NpcMaskSay so the NPC-info encoder emits it. Empty text is
	// allowed (produces an empty bubble that clears itself next tick via
	// ResetMasks).
	Say(text []byte)
```

- [ ] **Step 2: Write the failing handler test.** Open `pkg/script/handlers_npc_test.go`. Find the `mockNpc` struct. Add a field just before the closing `}`:

```go
	sayCalls []string
```

And after the existing mockNpc methods, add:

```go
func (m *mockNpc) Say(text []byte) {
	m.sayCalls = append(m.sayCalls, string(text))
}
```

Then append these two tests at the end of the file:

```go
func TestNpcSay(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	sf := &ScriptFile{
		Name:             "[npcsay,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpNpcSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hello", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := npc.sayCalls, []string{"hello"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("sayCalls: got %v, want %v", got, want)
	}
}

func TestNpcSayRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npcsay_noactive,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpNpcSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hello", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally left nil.
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_SAY: no active npc")
	}
	if got := err.Error(); !contains(got, "NPC_SAY: no active npc") {
		t.Errorf("err: got %q, want substring 'NPC_SAY: no active npc'", got)
	}
}

// contains is a tiny strings.Contains alias local to this file; if the
// package already has a helper of the same name delete this and reuse.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

Note: if `pkg/script/handlers_npc_test.go` already imports `strings`, replace `contains(got, "NPC_SAY: no active npc")` with `strings.Contains(...)` and omit the local helper.

- [ ] **Step 3: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcSay -v
```

Expected: both tests FAIL. `TestNpcSay` fails at build (`Say` not on interface, or no handler for `OpNpcSay`). `TestNpcSayRequiresActiveNpc` same.

- [ ] **Step 4: Add `handleNpcSay`.** Open `pkg/script/handlers_npc.go`. Append at the end of the file:

```go
// handleNpcSay pops a string and sets it as the active NPC's speech
// bubble for this tick. Empty strings are legal (clears the bubble).
func handleNpcSay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SAY"); err != nil {
		return err
	}
	text := s.PopString()
	s.ActiveNpc.Say([]byte(text))
	return nil
}
```

- [ ] **Step 5: Register the handler.** Open `pkg/script/handlers.go`. Find the S6a NPC-reads block (registration of `OpNpcType`, `OpNpcCoord`, etc.). Add immediately after the last S6a registration:

```go
	// S6b: NPC mutating ops.
	OpNpcSay: handleNpcSay,
```

- [ ] **Step 6: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcSay -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Both `TestNpcSay` and `TestNpcSayRequiresActiveNpc` PASS. Full package green.

- [ ] **Step 7: Commit.**

```bash
git add pkg/script/active.go pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S6b NPC_SAY handler + ActiveNpc.Say

Extends ActiveNpc with Say([]byte), registers handleNpcSay at
OpNpcSay (2532). Handler pops a string and delegates to the concrete
NPC's Say method; empty strings legal. Requires ActiveNpc != nil.

Modules/world impl ships in a later task — *Npc.Say already exists in
npc_masks.go:11-14 and satisfies the interface at compile time via
the existing S6a compile-time check.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + green test status.

---

## Task 2: `Player.interactionFired` gate field + reset sites

**Files:**
- Modify: `modules/world/player.go` (add field)
- Modify: `modules/world/interaction.go` (reset in Set/Clear)
- Modify: `modules/world/interaction_test.go` (new test for gate reset)

- [ ] **Step 1: Write the failing test.** Open `modules/world/interaction_test.go`. Append:

```go
func TestSetInteractionResetsInteractionFired(t *testing.T) {
	p := &Player{}
	p.interactionFired = true
	npc := &Npc{nid: 0, typeId: 7}
	p.SetInteraction(InteractionEngine, npc, 1)
	if p.interactionFired {
		t.Error("SetInteraction: interactionFired should be reset to false")
	}
}

func TestClearInteractionResetsInteractionFired(t *testing.T) {
	p := &Player{}
	p.interactionFired = true
	p.ClearInteraction()
	if p.interactionFired {
		t.Error("ClearInteraction: interactionFired should be reset to false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSetInteractionResetsInteractionFired -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestClearInteractionResetsInteractionFired -v
```

Expected: FAIL at build (`p.interactionFired undefined`).

- [ ] **Step 3: Add the field.** Open `modules/world/player.go`. Find the `// === interaction target ===` block (around line 83). Immediately after the existing `repathed bool` line, add:

```go
	interactionFired bool
```

The block should read:

```go
	// === interaction target ===
	target          entity
	targetOp        int
	targetSubject   struct{ typ, com int }
	interactionKind InteractionKind
	apRange         int
	apRangeCalled   bool
	interacted      bool
	repathed        bool
	interactionFired bool
	delayed         bool
	delayedUntil    int
```

- [ ] **Step 4: Reset the gate in `SetInteraction`.** Open `modules/world/interaction.go`. Find `SetInteraction`. Add `p.interactionFired = false` at the end:

```go
// SetInteraction anchors the interaction state machine on a target entity.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op int) {
	p.target = target
	p.targetOp = op
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false
}
```

- [ ] **Step 5: Reset the gate in `ClearInteraction`.** Same file:

```go
// ClearInteraction resets interaction state to idle.
func (p *Player) ClearInteraction() {
	p.target = nil
	p.targetOp = -1
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false
}
```

- [ ] **Step 6: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSetInteractionResetsInteractionFired|TestClearInteractionResetsInteractionFired" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Both PASS. Full package green.

- [ ] **Step 7: Commit.**

```bash
git add modules/world/player.go modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player.interactionFired one-shot dispatch gate

Adds interactionFired bool alongside the existing interaction fields.
Reset by both SetInteraction (new anchor = fresh dispatch) and
ClearInteraction (idle = next anchor starts fresh). No consumer yet;
S6b's tryFireOpTrigger will read and write this field.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA.

---

## Task 3: `tryFireOpTrigger` helper + processInteraction hook + unit tests

**Files:**
- Create: `modules/world/interaction_trigger.go`
- Create: `modules/world/interaction_trigger_test.go`
- Modify: `modules/world/interaction.go` (hook the call)

- [ ] **Step 1: Write the failing happy-path test.** Create `modules/world/interaction_trigger_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// buildNpcSayScript produces a tiny [push "hello", NPC_SAY, RETURN] script
// keyed at the trigger+typeID-specific lookup key.
func buildNpcSayScript(trigger script.ServerTriggerType, typeID int, text string) *script.ScriptFile {
	key := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
	return &script.ScriptFile{
		Name:             "[opnpc1,test]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{text, "", ""},
		InstructionCount: 3,
	}
}

// newTriggerFixture builds a Server + Player + live Npc of typeID=7 with a
// seeded NPC_SAY script registered at (TriggerOpNpc1, 7, 0).
func newTriggerFixture(t *testing.T) (*Server, *Player, *Npc) {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpc1, 7, "hello"))
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
		Op:         []string{"Talk-to", "", "", "", ""},
		Category:   0,
	}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1)
	p.interacted = true // simulate reach
	return s, p, npc
}

func TestTryFireOpTrigger_HappyPath(t *testing.T) {
	_, p, npc := newTriggerFixture(t)
	tryFireOpTrigger(p)
	if string(npc.sayText) != "hello" {
		t.Errorf("sayText: got %q, want %q", npc.sayText, "hello")
	}
	if p.target != nil {
		t.Error("target: expected cleared after Finished")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true after dispatch")
	}
}
```

Note: `newTestServer` and `newTestPlayer` already exist in `modules/world/server_test.go` and `modules/world/player_test.go`. `NewNpc` is in `modules/world/npc.go` — verify its signature (`nid, typeID, x, z, level, typ`) before using; adapt if different.

- [ ] **Step 2: Run the test to verify it fails.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTryFireOpTrigger_HappyPath -v
```

Expected: FAIL at build (`tryFireOpTrigger undefined`).

- [ ] **Step 3: Create the helper.** Create `modules/world/interaction_trigger.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// tryFireOpTrigger fires the [opnpc<op>,<npcType>] trigger for the player's
// anchored NPC target when the player has just reached interaction range.
// Matches TS Player.tryInteract() for the NPC branch.
//
// Preconditions (guaranteed by caller — Player.processInteraction):
//   - p.interacted == true
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
//
// Behaviour:
//   - Target not *Npc: no-op; set interactionFired so we don't retry.
//     (OPLOC/OPOBJ branches will extend this switch in a later sub-spec.)
//   - Player became delayed between reach and dispatch: defer; leave
//     interactionFired false so we retry next tick.
//   - NPC dead: clear interaction silently.
//   - targetOp out of [1,5]: clear interaction silently.
//   - No script found (type/category/global): clear interaction silently.
//   - Script suspends (P_DELAY/P_PAUSEBUTTON/P_COUNTDIALOG): keep
//     interaction anchored; resumeOrFinish already stored the state.
//   - Script finishes / aborts: clear interaction.
func tryFireOpTrigger(p *Player) {
	srv := p.client.server

	npc, ok := p.target.(*Npc)
	if !ok {
		p.interactionFired = true
		return
	}

	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if npc.dead {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpNpc1 + script.ServerTriggerType(op-1)
	category := 0
	if npc.typ != nil {
		category = npc.typ.Category
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, npc.typeId, category)
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup

	srv.resumeOrFinish(state, p)

	p.interactionFired = true
	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
}
```

- [ ] **Step 4: Run the test to verify it passes.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTryFireOpTrigger_HappyPath -v
```

Expected: PASS.

- [ ] **Step 5: Add the remaining 9 edge-case tests.** Append to `modules/world/interaction_trigger_test.go`:

```go
func TestTryFireOpTrigger_NoScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1)
	p.interacted = true
	tryFireOpTrigger(p)
	if p.target != nil {
		t.Error("target: expected cleared when no script found")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
	}
}

func TestTryFireOpTrigger_DeadNpc(t *testing.T) {
	_, p, npc := newTriggerFixture(t)
	npc.dead = true
	tryFireOpTrigger(p)
	if p.target != nil {
		t.Error("target: expected cleared for dead npc")
	}
	if len(npc.sayText) != 0 {
		t.Error("sayText: expected empty (script did not run)")
	}
}

// nonNpcEntity satisfies the entity interface for the wrong-target-type test.
type nonNpcEntity struct{}

func (nonNpcEntity) Slot() int                    { return 0 }
func (nonNpcEntity) Coords() (x, z, level int)    { return 0, 0, 0 }

func TestTryFireOpTrigger_WrongTargetType(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.SetInteraction(InteractionEngine, nonNpcEntity{}, 1)
	p.interacted = true
	tryFireOpTrigger(p)
	if p.target == nil {
		t.Error("target: expected preserved for non-npc (future OPLOC branch)")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
	}
}

func TestTryFireOpTrigger_BadOp(t *testing.T) {
	_, p, _ := newTriggerFixture(t)
	p.targetOp = 99
	tryFireOpTrigger(p)
	if p.target != nil {
		t.Error("target: expected cleared for bad op")
	}
}

func TestTryFireOpTrigger_ScriptSuspends(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Script: P_DELAY(5) + RETURN. Operands layout per S4 reference.
	suspendScript := &script.ScriptFile{
		Name:             "[opnpc1,suspend]",
		LookupKey:        uint32(script.TriggerOpNpc1) | (0x2 << 8) | (uint32(7) << 10),
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpPDelay, script.OpReturn},
		IntOperands:      []int32{5, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(suspendScript)
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1)
	p.interacted = true
	tryFireOpTrigger(p)
	if p.target == nil {
		t.Error("target: expected preserved across suspension")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
	}
	if p.activeScript == nil {
		t.Error("activeScript: expected stored for resume")
	}
}

func TestTryFireOpTrigger_PlayerDelayed(t *testing.T) {
	s, p, _ := newTriggerFixture(t)
	p.delayed = true
	p.delayedUntil = s.currentTick + 3
	tryFireOpTrigger(p)
	if p.target == nil {
		t.Error("target: expected preserved while delayed")
	}
	if p.interactionFired {
		t.Error("interactionFired: expected false so next tick retries")
	}
}

func TestTryFireOpTrigger_ReClickResetsFired(t *testing.T) {
	_, p, _ := newTriggerFixture(t)
	tryFireOpTrigger(p)
	if !p.interactionFired {
		t.Fatalf("pre: expected interactionFired true")
	}
	npc2Type := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 8}, Op: []string{"Talk-to"}}
	npc2 := NewNpc(1, 8, p.x, p.z, p.level, npc2Type)
	p.SetInteraction(InteractionEngine, npc2, 1)
	if p.interactionFired {
		t.Error("interactionFired: expected false after SetInteraction")
	}
}

func TestTryFireOpTrigger_CategoryFallback(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Script registered at category-level, not type-level.
	categoryKey := uint32(script.TriggerOpNpc1) | (0x1 << 8) | (uint32(3) << 10)
	catScript := &script.ScriptFile{
		Name:             "[opnpc1,_cat3]",
		LookupKey:        categoryKey,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"category", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(catScript)
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}, Category: 3}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1)
	p.interacted = true
	tryFireOpTrigger(p)
	if string(npc.sayText) != "category" {
		t.Errorf("sayText: got %q, want %q", npc.sayText, "category")
	}
}

func TestTryFireOpTrigger_GlobalFallback(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalScript := &script.ScriptFile{
		Name:             "[opnpc1,_]",
		LookupKey:        uint32(script.TriggerOpNpc1),
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"global", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(globalScript)
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1)
	p.interacted = true
	tryFireOpTrigger(p)
	if string(npc.sayText) != "global" {
		t.Errorf("sayText: got %q, want %q", npc.sayText, "global")
	}
}
```

- [ ] **Step 6: Run the full trigger test suite.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTryFireOpTrigger -v
```

Expected: all 10 PASS.

- [ ] **Step 7: Hook `tryFireOpTrigger` into `Player.processInteraction`.** Open `modules/world/interaction.go`. Find the `inOperableDistance` branch inside `processInteraction` (around line 68). Replace:

```go
	if inOperableDistance(p.x, p.z, tx, tz) {
		if npc, ok := p.target.(*Npc); ok {
			p.SetFaceEntity(npc.nid)
		}
		p.interacted = true
		return
	}
```

with:

```go
	if inOperableDistance(p.x, p.z, tx, tz) {
		if npc, ok := p.target.(*Npc); ok {
			p.SetFaceEntity(npc.nid)
		}
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return
	}
```

- [ ] **Step 8: Run the full world suite to confirm no regression.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

All PASS/clean.

- [ ] **Step 9: Commit.**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go modules/world/interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S6b tryFireOpTrigger dispatch + processInteraction hook

On reach-success Player.processInteraction now calls tryFireOpTrigger
(gated by interactionFired). The helper resolves the target NPC,
looks up [opnpc<op>,<type>] via scriptProvider.GetByTrigger (3-level
type/category/global fallback), builds a ScriptState with Self +
ActiveNpc bound, and runs via resumeOrFinish. Suspension preserves
the interaction anchor; Finished/Aborted clears it.

10 unit tests: happy path, no-script, dead npc, wrong target type,
bad op, suspension, delayed player, re-click reset, category + global
fallbacks.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + full-package green status.

---

## Task 4: E2E — click OPNPC1, walk, arrive, NPC_SAY on wire

**Files:**
- Modify: `modules/world/script_test.go` (append E2E test)

- [ ] **Step 1: Write the failing E2E test.** Open `modules/world/script_test.go`. Append:

```go
func TestOpNpc1FiresScriptAndEmitsSay(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	// Register [opnpc1, type=7] = push "cluck cluck" + NPC_SAY + RETURN.
	key := uint32(script.TriggerOpNpc1) | (0x2 << 8) | (uint32(7) << 10)
	s.scriptProvider.Register(&script.ScriptFile{
		Name:             "[opnpc1,chicken]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"cluck cluck", "", ""},
		InstructionCount: 3,
	})

	p, _ := newTestPlayer(t)
	p.client.server = s

	// Place an NPC of type 7 adjacent to the player so reach is immediate.
	npcType := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "chicken"},
		Op:         []string{"Talk-to", "", "", "", ""},
	}
	npc := NewNpc(0, 7, p.x+1, p.z, p.level, npcType)
	// Ensure the NPC slice has slot 0 populated so handleOpNpc1 can find it.
	if len(s.npcs) == 0 {
		s.npcs = append(s.npcs, npc)
	} else {
		s.npcs[0] = npc
	}

	// Build the OPNPC1 payload (p2(slot=0)) and fire it through the handler.
	payload := []byte{0x00, 0x00}
	if err := handleOpNpc1(p, payload); err != nil {
		t.Fatalf("handleOpNpc1: %v", err)
	}
	if p.target != npc {
		t.Fatalf("post-click: target=%v, want npc", p.target)
	}

	// Drive one processInteraction tick — player is already adjacent, so
	// reach succeeds immediately and tryFireOpTrigger runs.
	p.processInteraction()

	if string(npc.sayText) != "cluck cluck" {
		t.Errorf("sayText: got %q, want 'cluck cluck'", npc.sayText)
	}
	if npc.masks&rsbuf.NpcMaskSay == 0 {
		t.Error("NpcMaskSay bit: not set on npc.masks")
	}
	if p.target != nil {
		t.Error("target: expected cleared after script Finished")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true after dispatch")
	}
}
```

Add `"github.com/zsrv/goscape/pkg/rsbuf"` to the imports block at the top of the file if not already present.

- [ ] **Step 2: Run the test to verify it fails.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestOpNpc1FiresScriptAndEmitsSay -v
```

Expected: FAIL if the test imports / NPC slot setup mismatches, OR PASS if Task 3's hook is already in place. If it fails for a reason **other than** import/setup (e.g., target not cleared, sayText empty), there's a bug — investigate before moving on.

- [ ] **Step 3: Fix any fixture issues** (import paths, `s.npcs` slice type, etc.) until the test passes. No production changes should be needed — Tasks 1–3 already ship the behaviour.

- [ ] **Step 4: Run the full repo suite.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

All PASS/clean.

- [ ] **Step 5: Commit.**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): S6b E2E — OPNPC1 click fires script and emits NPC_SAY

Drives the full chain: handleOpNpc1 validates + SetInteraction,
processInteraction reach-check succeeds, tryFireOpTrigger dispatches
the [opnpc1,chicken] script, NPC_SAY buffers "cluck cluck" with
NpcMaskSay set, interaction clears on script Finished.

Hermetic — drives packet parser + tick phases directly, no TCP.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + green full-repo status.

---

## Self-Review Checklist

After completing all four tasks, verify:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean
- [ ] `grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go` shows handler count **+1** vs pre-S6b baseline (NPC_SAY is the only new handler; other S6b changes are in modules/world).
- [ ] Four commits on main: NPC_SAY / interactionFired / dispatch / E2E.
- [ ] Spec requirements covered:
  - [ ] Section 1 (Goal): OPNPC1..5 click → walk → arrive → script fires with Self+ActiveNpc → ✅ Tasks 3+4
  - [ ] NPC_SAY observable on wire → ✅ Task 4
  - [ ] Suspension preserves interaction → ✅ Task 3 test
  - [ ] No-script = silent clear → ✅ Task 3 test
  - [ ] Component 1 (interactionFired) → ✅ Task 2
  - [ ] Component 2 (tryFireOpTrigger) → ✅ Task 3
  - [ ] Component 3 (processInteraction hook) → ✅ Task 3
  - [ ] Component 4 (ActiveNpc.Say) → ✅ Task 1
  - [ ] Component 5 (handleNpcSay) → ✅ Task 1
  - [ ] Component 6 (mockNpc.Say) → ✅ Task 1
  - [ ] 10 edge cases → ✅ Task 3 test matrix
  - [ ] NPC_SAY unit + interface-guard tests → ✅ Task 1
  - [ ] E2E wire test → ✅ Task 4
