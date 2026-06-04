# RuneScript S6c: NPC Mutating Ops Batch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose four NPC-mutating script opcodes — NPC_ANIM, NPC_FACESQUARE, NPC_CHANGETYPE, NPC_DAMAGE — so scripts can make NPCs animate, turn, morph, and take damage. All four ride on existing mask encoder entries.

**Architecture:** Three vertical slices. Task 1 adds three safe-wrap methods to `ActiveNpc` (Animate, FaceCoord, ChangeType) + their handlers + unit tests. Task 2 adds the fourth interface method (Damage), a new `*Npc.Damage` method that manages HP, the handler, and HP integration tests. Task 3 adds one E2E compound-masks test proving NPC_ANIM and NPC_SAY can land in the same script tick.

**Tech Stack:** Go; `pkg/script` runtime; `modules/world/npc_masks.go` for concrete NPC output methods; pre-existing `rsbuf` NPC-info mask encoder.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6c-npc-mutating-ops-design.md`](../specs/2026-04-21-runescript-s6c-npc-mutating-ops-design.md) (commits `cb16948` + `dc29a8a`)

---

## Task 1: NPC_ANIM + NPC_FACESQUARE + NPC_CHANGETYPE handlers + interface

**Files:**
- Modify: `pkg/script/active.go` (extend `ActiveNpc` with three methods)
- Modify: `pkg/script/handlers_npc.go` (three new handlers)
- Modify: `pkg/script/handlers.go` (three registrations)
- Modify: `pkg/script/handlers_npc_test.go` (mockNpc fields/methods, four tests)

- [ ] **Step 1: Extend the `ActiveNpc` interface.** Open `pkg/script/active.go`. Find the interface. Immediately after `Say(text []byte)`, insert:

```go
	// Animate schedules sequence `id` with client-side `delay` on the NPC's
	// primary animation slot this tick. id = -1 clears.
	Animate(id, delay int)

	// FaceCoord rotates the NPC to face absolute square (x, z). Wire coords
	// are doubled + 1 (face-center convention).
	FaceCoord(x, z int)

	// ChangeType morphs the NPC to `newType`. The client swaps the model on
	// the next NPC-info flush; server-side fields beyond typeId are not
	// re-initialized (stats, category, etc. still reference the old config).
	// The script op NPC_CHANGETYPE also carries a `duration` parameter for
	// timed revert, but S6c discards it (method takes type only); future
	// AI sub-spec wires a revert timer.
	ChangeType(newType int)
```

- [ ] **Step 2: Write failing tests.** Open `pkg/script/handlers_npc_test.go`. First extend `mockNpc`. Find the struct, add these fields just before the closing `}`:

```go
	animCalls       []struct{ id, delay int }
	faceCoordCalls  []struct{ x, z int }
	changeTypeCalls []int
```

After the existing `mockNpc` methods, add:

```go
func (m *mockNpc) Animate(id, delay int) {
	m.animCalls = append(m.animCalls, struct{ id, delay int }{id, delay})
}
func (m *mockNpc) FaceCoord(x, z int) {
	m.faceCoordCalls = append(m.faceCoordCalls, struct{ x, z int }{x, z})
}
func (m *mockNpc) ChangeType(newType int) {
	m.changeTypeCalls = append(m.changeTypeCalls, newType)
}
```

Then append these four tests at the end of the file:

```go
func TestNpcAnim(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Push seq=42, push delay=3, NPC_ANIM. delay is on top per TS.
	sf := &ScriptFile{
		Name:             "[npcanim,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcAnim, OpReturn},
		IntOperands:      []int32{42, 3, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []struct{ id, delay int }{{42, 3}}
	if !reflect.DeepEqual(npc.animCalls, want) {
		t.Errorf("animCalls: got %v, want %v", npc.animCalls, want)
	}
}

func TestNpcFaceSquare(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Packed coord: level=0, x=3222, z=3218 → (0<<28)|(3222<<14)|3218
	coord := int32((3222 << 14) | 3218)
	sf := &ScriptFile{
		Name:             "[npcfacesquare,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpNpcFaceSquare, OpReturn},
		IntOperands:      []int32{coord, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []struct{ x, z int }{{3222, 3218}}
	if !reflect.DeepEqual(npc.faceCoordCalls, want) {
		t.Errorf("faceCoordCalls: got %v, want %v", npc.faceCoordCalls, want)
	}
}

func TestNpcChangeTypeDiscardsDuration(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Push newType=9, push duration=50, NPC_CHANGETYPE. duration on top.
	sf := &ScriptFile{
		Name:             "[npcchangetype,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcChangeType, OpReturn},
		IntOperands:      []int32{9, 50, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []int{9}
	if !reflect.DeepEqual(npc.changeTypeCalls, want) {
		t.Errorf("changeTypeCalls: got %v, want %v", npc.changeTypeCalls, want)
	}
}

func TestNpcMutatingOpsRequireActiveNpc_S6c(t *testing.T) {
	cases := []struct {
		op   Opcode
		name string
	}{
		{OpNpcAnim, "NPC_ANIM"},
		{OpNpcFaceSquare, "NPC_FACESQUARE"},
		{OpNpcChangeType, "NPC_CHANGETYPE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name: "[noactive," + tc.name + "]",
				// Push enough ints so the handler's pops don't underflow:
				// NPC_FACESQUARE needs 1, NPC_ANIM/NPC_CHANGETYPE need 2.
				Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, tc.op, OpReturn},
				IntOperands:      []int32{0, 0, 0, 0},
				StringOperands:   []string{"", "", "", ""},
				InstructionCount: 4,
			}
			state := Init(sf, nil, false, nil, nil)
			// ActiveNpc intentionally nil.
			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: got nil err, want %q: no active npc", tc.name)
			}
			if !strings.Contains(err.Error(), tc.name+": no active npc") {
				t.Errorf("err: got %q, want substring %q", err, tc.name+": no active npc")
			}
		})
	}
}
```

Note: if `reflect` or `strings` aren't already imported in `handlers_npc_test.go`, add them to the import block.

- [ ] **Step 3: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestNpcAnim|TestNpcFaceSquare|TestNpcChangeType|TestNpcMutatingOpsRequireActiveNpc_S6c" -v
```

Expected: FAIL — either at build (interface + mock compile errors) or at runtime (`no handler for NPC_ANIM` / `no handler for NPC_FACESQUARE` / `no handler for NPC_CHANGETYPE`).

- [ ] **Step 4: Add the three handlers.** Open `pkg/script/handlers_npc.go`. Append at the end of the file:

```go
// handleNpcAnim pops (seq, delay) in TS order (delay on top) and schedules
// the animation on the active NPC this tick.
func handleNpcAnim(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ANIM"); err != nil {
		return err
	}
	delay := s.PopInt()
	id := s.PopInt()
	s.ActiveNpc.Animate(id, delay)
	return nil
}

// handleNpcFaceSquare pops a single packed coord (level<<28 | x<<14 | z)
// and rotates the NPC to face that absolute square. Level bits are unused
// here (the NPC's own level always matches its face target in practice).
func handleNpcFaceSquare(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_FACESQUARE"); err != nil {
		return err
	}
	coord := s.PopInt()
	x := (coord >> 14) & 0x3fff
	z := coord & 0x3fff
	s.ActiveNpc.FaceCoord(x, z)
	return nil
}

// handleNpcChangeType pops (newType, duration) in TS order (duration on
// top) and morphs the NPC. S6c discards duration — timed revert is
// deferred to a future AI sub-spec.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	_ = s.PopInt() // duration; see spec S6c Gotchas
	newType := s.PopInt()
	s.ActiveNpc.ChangeType(newType)
	return nil
}
```

- [ ] **Step 5: Register the handlers.** Open `pkg/script/handlers.go`. Find the `OpNpcSay: handleNpcSay` entry (from S6b). Immediately after it, add:

```go
	// S6c: NPC mutating ops batch.
	OpNpcAnim:       handleNpcAnim,
	OpNpcFaceSquare: handleNpcFaceSquare,
	OpNpcChangeType: handleNpcChangeType,
```

- [ ] **Step 6: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestNpcAnim|TestNpcFaceSquare|TestNpcChangeType|TestNpcMutatingOpsRequireActiveNpc_S6c" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

All four new tests PASS. Full package green.

- [ ] **Step 7: Verify gofmt and build-wide consistency.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/script/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

All clean. gofmt output empty.

**Important check:** `*Npc` must continue to satisfy `script.ActiveNpc` at compile time. Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

This must succeed — the existing `Npc.Animate`, `Npc.FaceCoord`, `Npc.ChangeType` methods in `modules/world/npc_masks.go` already match the new interface members (the compile-time check `var _ script.ActiveNpc = (*Npc)(nil)` in `modules/world/npc_script.go:6` enforces this). If the build fails, the signatures diverged — read `modules/world/npc_masks.go:5-14,24-27,36-40` to confirm the world-side signatures are `Animate(id, delay int)`, `FaceCoord(x, z int)`, `ChangeType(newType int)`.

- [ ] **Step 8: Commit.**

```bash
git add pkg/script/active.go pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S6c NPC_ANIM + NPC_FACESQUARE + NPC_CHANGETYPE handlers

Extends ActiveNpc with Animate / FaceCoord / ChangeType (all three
satisfied by existing *Npc methods in modules/world/npc_masks.go).
Registers three handlers with verified TS pop orders:
- NPC_ANIM: (seq, delay), delay on top
- NPC_FACESQUARE: single packed coord
- NPC_CHANGETYPE: (newType, duration), duration on top. S6c discards
  duration; future AI sub-spec wires timed revert.

Four unit tests drive each handler through the full VM pipeline +
a table-driven require-active-npc guard test.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA, green test status, gofmt clean.

---

## Task 2: NPC_DAMAGE — `*Npc.Damage` method + handler + HP integration tests

**Files:**
- Modify: `pkg/script/active.go` (one more interface method)
- Modify: `pkg/script/handlers_npc.go` (new `handleNpcDamage`)
- Modify: `pkg/script/handlers.go` (register `OpNpcDamage`)
- Modify: `pkg/script/handlers_npc_test.go` (mockNpc.Damage + `TestNpcDamage`)
- Modify: `modules/world/npc_masks.go` (new `*Npc.Damage` method)
- Create or modify: `modules/world/npc_masks_test.go` (three HP tests)

- [ ] **Step 1: Extend the interface.** Open `pkg/script/active.go`. After the three S6c methods added in Task 1, add:

```go
	// Damage applies `amount` damage of `dmgType` to the NPC this tick,
	// flagging NpcMaskDamage. Decrements curHP (clamped at 0). Does NOT
	// trigger death handling or auto-retaliate — those belong in a future
	// NPC AI sub-spec.
	Damage(amount, dmgType int)
```

- [ ] **Step 2: Extend mockNpc.** In `pkg/script/handlers_npc_test.go`, add to the `mockNpc` struct fields:

```go
	damageCalls []struct{ amount, dmgType int }
```

And after the other mockNpc methods:

```go
func (m *mockNpc) Damage(amount, dmgType int) {
	m.damageCalls = append(m.damageCalls, struct{ amount, dmgType int }{amount, dmgType})
}
```

- [ ] **Step 3: Write the failing handler test.** Append to `pkg/script/handlers_npc_test.go`:

```go
func TestNpcDamage(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Push type=1, push amount=3, NPC_DAMAGE. amount is on top per TS.
	sf := &ScriptFile{
		Name:             "[npcdamage,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcDamage, OpReturn},
		IntOperands:      []int32{1, 3, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []struct{ amount, dmgType int }{{3, 1}}
	if !reflect.DeepEqual(npc.damageCalls, want) {
		t.Errorf("damageCalls: got %v, want %v", npc.damageCalls, want)
	}
}

func TestNpcDamageRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npcdamage_noactive,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcDamage, OpReturn},
		IntOperands:      []int32{0, 0, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_DAMAGE: no active npc")
	}
	if !strings.Contains(err.Error(), "NPC_DAMAGE: no active npc") {
		t.Errorf("err: got %q, want substring 'NPC_DAMAGE: no active npc'", err)
	}
}
```

- [ ] **Step 4: Write the failing HP integration tests.** Create or extend `modules/world/npc_masks_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func npcWithHP(t *testing.T, maxHP, curHP int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
	}
	// NpcType.Stats[3] is the Hitpoints slot (npcStatHitpoints=3 in
	// pkg/objtype/npctype.go). Stats is [6]uint8 per that file.
	typ.Stats[3] = uint8(maxHP)
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	npc.curHP = curHP
	return npc
}

func TestNpcDamageDecrementsHPAndSetsMask(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	if npc.curHP != 7 {
		t.Errorf("curHP: got %d, want 7", npc.curHP)
	}
	if npc.baseHP != 10 {
		t.Errorf("baseHP: got %d, want 10", npc.baseHP)
	}
	if npc.damageAmt != 3 {
		t.Errorf("damageAmt: got %d, want 3", npc.damageAmt)
	}
	if npc.damageType != 1 {
		t.Errorf("damageType: got %d, want 1", npc.damageType)
	}
	if npc.masks&rsbuf.NpcMaskDamage == 0 {
		t.Error("NpcMaskDamage bit not set on npc.masks")
	}
}

func TestNpcDamageClampsAtZero(t *testing.T) {
	npc := npcWithHP(t, 10, 2)
	npc.Damage(5, 1)
	if npc.curHP != 0 {
		t.Errorf("curHP: got %d, want 0 (clamped)", npc.curHP)
	}
	if npc.damageAmt != 5 {
		t.Errorf("damageAmt: got %d, want 5 (actual requested amount, not floored)", npc.damageAmt)
	}
}

func TestNpcDamageNegativeAmountClampsToZero(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(-3, 1)
	if npc.curHP != 10 {
		t.Errorf("curHP: got %d, want 10 (negative amount must not heal)", npc.curHP)
	}
	if npc.damageAmt != 0 {
		t.Errorf("damageAmt: got %d, want 0 (negative amount clamped)", npc.damageAmt)
	}
	if npc.masks&rsbuf.NpcMaskDamage == 0 {
		t.Error("NpcMaskDamage bit not set (mask should still flip even at zero damage)")
	}
}
```

Note on the test helper: if `NpcType.Stats` is a different type (e.g., `[6]int` rather than `[6]uint8`), adjust the cast. Read `pkg/objtype/npctype.go:11-19,150-160` to confirm the exact type before implementing. The spec notes `Stats[3]` as a `uint8` (from `dat.G2()` — actually that returns uint16, so `Stats` may be `[6]uint16` or cast-narrowed). Use whatever type matches the existing field.

- [ ] **Step 5: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestNpcDamage" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcDamage" -v
```

Expected: FAIL — script tests at handler registration (`no handler for NPC_DAMAGE`), world tests at build (`npc.Damage undefined`).

- [ ] **Step 6: Add the handler.** Append to `pkg/script/handlers_npc.go`:

```go
// handleNpcDamage pops (type, amount) in TS order (amount on top) and
// applies damage. The concrete Npc impl manages HP; this handler stays thin.
func handleNpcDamage(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DAMAGE"); err != nil {
		return err
	}
	amount := s.PopInt()
	dmgType := s.PopInt()
	s.ActiveNpc.Damage(amount, dmgType)
	return nil
}
```

- [ ] **Step 7: Register the handler.** In `pkg/script/handlers.go`, in the `// S6c: NPC mutating ops batch.` block added in Task 1, add:

```go
	OpNpcDamage: handleNpcDamage,
```

- [ ] **Step 8: Add `*Npc.Damage` in `modules/world/npc_masks.go`.** Append:

```go
// Damage applies `amount` damage of `dmgType` to the NPC this tick, flagging
// NpcMaskDamage so the NPC-info encoder emits the hitsplat. curHP decrements
// by amount (clamped at 0); baseHP is set from NpcType.Stats[npcStatHitpoints]
// (index 3) if available, otherwise left at its current value. Negative
// amount is coerced to 0 defensively so a script bug can't heal the NPC.
//
// This method is a pure output op — no death / auto-retaliate / aggro logic.
// Scripts that need death handling should check NPC_STAT(0) and fire their
// own despawn flow. The AI sub-spec will later ship a real combat loop.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	n.damageAmt = amount
	n.damageType = dmgType
	n.curHP -= amount
	if n.curHP < 0 {
		n.curHP = 0
	}
	if n.typ != nil {
		if hp := int(n.typ.Stats[3]); hp > 0 {
			n.baseHP = hp
		}
	}
	n.masks |= rsbuf.NpcMaskDamage
}
```

**Note on the `Stats[3]` literal:** if `pkg/objtype` exports a `NpcStatHitpoints` constant, prefer it over the magic `3`. Run `grep -n 'npcStatHitpoints\|NpcStatHitpoints' pkg/objtype/npctype.go` to confirm. The constant at `pkg/objtype/npctype.go:16` is unexported (`npcStatHitpoints`), so the magic number is currently the only option from outside the package unless you also add an exported alias — **do not** add that alias as part of this task; leave the magic number inline with an explanatory comment, since exporting is a separate hygiene change.

- [ ] **Step 9: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestNpcDamage" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcDamage" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

All tests green.

- [ ] **Step 10: Verify gofmt / vet / build.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/script/ modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ ./pkg/script/
```

All clean. If `gofmt -l` reports a file *you touched*, run `gofmt -w` on just that file. Do NOT sweep pre-existing drift.

- [ ] **Step 11: Commit.**

```bash
git add pkg/script/active.go pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go modules/world/npc_masks.go modules/world/npc_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): S6c NPC_DAMAGE + *Npc.Damage HP management

Extends ActiveNpc with Damage(amount, dmgType). The concrete
*Npc.Damage method decrements curHP (clamped at 0), refreshes
baseHP from NpcType.Stats[3], and flags NpcMaskDamage. Negative
amounts are clamped to 0 defensively.

Handler pops (type, amount) with amount on top per TS NpcOps.ts.

Pure output op — no death / aggro / retaliate. AI loop ships later.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA, green script + world suites, gofmt clean.

---

## Task 3: E2E compound-masks test

**Files:**
- Modify: `modules/world/script_test.go` (append one new E2E test)

- [ ] **Step 1: Write the failing E2E test.** Open `modules/world/script_test.go`. Append at the end:

```go
func TestOpNpc1FiresScriptAndEmitsAnimPlusSay(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	// [opnpc1, type=7]: push seq=42 + push delay=3 + NPC_ANIM +
	//                   push "cluck" + NPC_SAY + RETURN.
	key := uint32(script.TriggerOpNpc1) | (0x2 << 8) | (uint32(7) << 10)
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[opnpc1,chicken]",
		LookupKey: key,
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,    // seq
			script.OpPushConstantInt,    // delay
			script.OpNpcAnim,            // consume (seq, delay)
			script.OpPushConstantString, // "cluck"
			script.OpNpcSay,             // consume string
			script.OpReturn,
		},
		IntOperands:      []int32{42, 3, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "cluck", "", ""},
		InstructionCount: 6,
	})

	p, _ := newTestPlayer(t)
	p.client.server = s

	npcType := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "chicken"},
		Op:         []string{"Talk-to", "", "", "", ""},
	}
	npc := NewNpc(0, 7, p.x+1, p.z, p.level, npcType)
	s.npcs[0] = npc

	// Fire OPNPC1 click.
	payload := []byte{0x00, 0x00}
	if err := handleOpNpc1(p, payload); err != nil {
		t.Fatalf("handleOpNpc1: %v", err)
	}

	// Drive one tick — player is already adjacent, reach succeeds,
	// tryFireOpTrigger dispatches the compound script.
	p.processInteraction()

	if npc.animID != 42 {
		t.Errorf("animID: got %d, want 42", npc.animID)
	}
	if npc.animDelay != 3 {
		t.Errorf("animDelay: got %d, want 3", npc.animDelay)
	}
	if string(npc.sayText) != "cluck" {
		t.Errorf("sayText: got %q, want 'cluck'", npc.sayText)
	}
	if npc.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim bit: not set — compound mask writes may be broken")
	}
	if npc.masks&rsbuf.NpcMaskSay == 0 {
		t.Error("NpcMaskSay bit: not set — compound mask writes may be broken")
	}
	if p.target != nil {
		t.Error("target: expected cleared after script Finished")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true after dispatch")
	}
}
```

- [ ] **Step 2: Run the test to verify it passes.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestOpNpc1FiresScriptAndEmitsAnimPlusSay -v
```

Expected: **PASS on first run.** This is a capstone E2E — Tasks 1–2 shipped the behavior, and this test just verifies the compound chain works end-to-end. If it fails:
- Check `rsbuf`, `objtype`, `script` imports are present in the file header.
- Check `npc.animID` and `npc.animDelay` are the actual field names (read `modules/world/npc.go` around the NPC struct).
- Check `s.npcs` is a fixed-size array (`[8192]*Npc`) per the S6b E2E fixture pattern.

If the test fails for a non-fixture reason (e.g. mask bit not actually set after running the compound script), that's a real bug — investigate before proceeding.

- [ ] **Step 3: Run full repo suite.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All PASS / empty. If `gofmt -l` flags `script_test.go` only, `gofmt -w` it. Don't sweep other files.

- [ ] **Step 4: Commit.**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): S6c E2E — compound NPC_ANIM + NPC_SAY in one script tick

Drives the full OPNPC1 chain with a two-op script body — NPC_ANIM
then NPC_SAY. Asserts both mask bits (NpcMaskAnim | NpcMaskSay) end
up set on the same NPC in the same tick, proving the encoder handles
compound mask writes and resumeOrFinish doesn't clear masks between
opcodes.

Hermetic, same pattern as the S6b E2E.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA, full-repo green, vet + race + gofmt clean.

---

## Self-Review Checklist

After Tasks 1–3 complete:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/script/ modules/world/` empty
- [ ] Three commits on main: S6c safe-wrap handlers / NPC_DAMAGE / E2E
- [ ] Spec requirements covered:
  - [ ] ActiveNpc gains Animate / FaceCoord / ChangeType / Damage → Tasks 1+2
  - [ ] 4 handlers registered → Tasks 1+2
  - [ ] New `*Npc.Damage` method with HP clamp → Task 2
  - [ ] HP integration tests (decrement / clamp-zero / negative) → Task 2
  - [ ] E2E compound-masks test → Task 3
  - [ ] All pop orders tested (NPC_ANIM, NPC_FACESQUARE, NPC_CHANGETYPE duration-ignore, NPC_DAMAGE) → Tasks 1+2
  - [ ] Require-active-npc guard tests → Task 1 + Task 2
