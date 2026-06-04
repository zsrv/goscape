# NAI-126 — NPC_DEL handler + paramtype DefaultInt + modernization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the `[proc,npc_death]` cascade-tail surfaced by NAI-125 close smoke (2026-05-08) by porting `NPC_DEL` (opcode 2510) and converging two queued NAI-124 carryovers (paramtype `DefaultInt` sign-extension + modernization sweep).

**Architecture:** Three independent bundles dispatched as separate task sequences. Bundle 1 (PRIMARY) routes the script-side `NPC_DEL` opcode through `script.WorldVars.RemoveNpc` to `Server.removeNpc`, mirroring the OBJ_DEL pattern (NAI-115-D2). Bundle 2 fixes `paramtype.DefaultInt` storage to `int32` (matches `enumtype`/`dbtabletype` siblings) so `int(pt.DefaultInt)` sign-extends correctly. Bundle 3 mechanically rewrites flagged S1001/minmax/rangeint warnings.

**Tech Stack:** Go 1.26+.

**Spec:** `docs/superpowers/specs/2026-05-08-nai-126-npc-del-handler-design.md` (`6dcb29d`).

**Commit ordering:** Bundle 1 first (smoke-binding), then Bundle 2, then Bundle 3 — keeps cascade attribution clean per `cascade_theory_smoke_binding`.

**Pre-dispatch invariants** (controller responsibility, not implementer):
- Re-grep+Read every line number cited in this plan against HEAD before each task dispatch (`controller_preflight`).
- Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && go vet ./... && go build ./...` after each commit; ignore stale IDE diagnostics (`verify_implementer_claims`).
- `git status` after each commit to catch worktree-stray writes (`feedback_subagent_wt_path`).

---

## Bundle 1 — NPC_DEL handler (PRIMARY, smoke-bound)

### Task 1.1: Extend `ActiveNpc` interface with `Respawnrate() int` and add impls

**Files:**
- Modify: `pkg/script/active.go` (interface declaration; insert after `LastMovement()` group)
- Modify: `modules/world/npc_script.go` (add `*Npc.Respawnrate()` impl after existing `LastMovement` at line 40)
- Modify: `pkg/script/handlers_npc_test.go` (extend `mockNpc` struct + getter)
- Modify: `pkg/script/handlers_player_test.go` (add `Respawnrate()` to `mockActiveNpc`)

The 3 ActiveNpc implementers at HEAD `6d04cf8` are: `*Npc` (compile-time asserted at `npc_script.go:11`), `mockNpc` (`handlers_npc_test.go:199`), and `mockActiveNpc` (in `handlers_player_test.go`). All three must implement `Respawnrate()` or the interface compile-time assertion at `npc_script.go:11` and the package builds will fail.

- [ ] **Step 1: Add `Respawnrate() int` to the `ActiveNpc` interface**

In `pkg/script/active.go`, locate the `LastMovement()` declaration (around line 631 — re-grep at HEAD with `grep -n "LastMovement() int" pkg/script/active.go`). Add the new method directly after the `LastMovement` block (before the next interface method):

```go
	// Respawnrate returns the active NPC type's respawnrate config field
	// (objtype.NpcType.RespawnRate, uint16 widened to int). Read by NPC_DEL
	// — passed as the duration arg to script.WorldVars.RemoveNpc. Mirrors
	// TS check(state.activeNpc.type, NpcTypeValid).respawnrate at
	// NpcOps.ts:79.
	Respawnrate() int
```

- [ ] **Step 2: Add `*Npc.Respawnrate()` impl**

In `modules/world/npc_script.go`, append after the existing `LastMovement` method (line 40):

```go
// Respawnrate returns the NPC type's respawnrate config field
// (uint16 widened to int). Read by NPC_DEL — passed as the duration
// arg to script.WorldVars.RemoveNpc. Mirrors TS
// check(state.activeNpc.type, NpcTypeValid).respawnrate at NpcOps.ts:79.
func (n *Npc) Respawnrate() int { return int(n.typ.RespawnRate) }
```

- [ ] **Step 3: Extend `mockNpc` with respawnrate field + getter**

In `pkg/script/handlers_npc_test.go`, locate the `mockNpc` struct (line 199). Add a `respawnrate int` field — group it with the other simple-int fields near the top of the struct (e.g., next to `nid int`):

```go
	nid                                int
	respawnrate                        int
```

Then add the getter alongside other one-liners (e.g., next to `Nid()` around line 254):

```go
func (m *mockNpc) Respawnrate() int { return m.respawnrate }
```

Re-grep before this edit:
```bash
grep -n "func (m \*mockNpc) Nid\|func (m \*mockNpc) LastMovement" pkg/script/handlers_npc_test.go
```
to confirm the insertion site against HEAD.

- [ ] **Step 4: Extend `mockActiveNpc` with `Respawnrate()`**

In `pkg/script/handlers_player_test.go`, locate `mockActiveNpc` and add a `Respawnrate() int` method. Re-grep first to find the exact insertion site:

```bash
grep -n "func (m \*mockActiveNpc) NpcType\|func (m \*mockActiveNpc) LastMovement" pkg/script/handlers_player_test.go
```

Add (alongside the other ActiveNpc methods on `mockActiveNpc`):

```go
func (m *mockActiveNpc) Respawnrate() int { return 0 }
```

(Default 0 is sufficient — `mockActiveNpc` is used by handler_player tests that do not exercise NPC_DEL.)

- [ ] **Step 5: Build to verify all implementers satisfy the new interface**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && go vet ./...
```
Expected: clean exit. If `*Npc` is missing the method, the assertion at `modules/world/npc_script.go:11` fails. If `mockNpc` or `mockActiveNpc` is missing it, package test builds fail.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(nai-126): T1.1 — Respawnrate() on ActiveNpc + 3 impls"
```

---

### Task 1.2: Extend `WorldVars` interface with `RemoveNpc(npc, duration int)` + adapter + mock

**Files:**
- Modify: `pkg/script/state.go` (interface declaration, add directly after `RemoveObj` at line 92)
- Modify: `modules/world/server_varp.go` (add adapter after `RemoveObj` at line ~138)
- Modify: `pkg/script/handlers_vars_test.go` (add no-op stub on `mockWorld` after `RemoveObj` at line 51)

- [ ] **Step 1: Add `RemoveNpc` to `WorldVars` interface**

In `pkg/script/state.go`, locate the `RemoveObj(obj ActiveObj)` declaration at line 92. Add immediately after:

```go
	// RemoveNpc removes the given NPC from the world. duration is passed
	// through to Server.removeNpc, which scales it by player count and
	// writes lifecycleTick (RESPAWN-lifecycle) or schedules registry
	// cleanup (DESPAWN-lifecycle). Mirrors TS World.removeNpc at
	// World.ts:1296-1319. Used by NPC_DEL.
	RemoveNpc(npc ActiveNpc, duration int)
```

- [ ] **Step 2: Add `worldVarsView.RemoveNpc` adapter**

Re-grep before edit:
```bash
grep -n "func (w worldVarsView) RemoveObj" modules/world/server_varp.go
```
to confirm `RemoveObj` line at HEAD. Add the new method directly after `RemoveObj`:

```go
// RemoveNpc implements script.WorldVars.RemoveNpc. Type-asserts the
// script-side ActiveNpc to *Npc and calls the existing
// Server.removeNpc. Mirrors RemoveObj. Type-assert miss is a silent
// no-op (matches RemoveObj behavior); production NPC pointers are
// always *Npc.
func (w worldVarsView) RemoveNpc(npc script.ActiveNpc, duration int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	w.s.removeNpc(realNpc, duration)
}
```

- [ ] **Step 3: Add `mockWorld.RemoveNpc` no-op stub**

In `pkg/script/handlers_vars_test.go`, locate the `RemoveObj` stub at line 51. Add immediately after:

```go
// NAI-126 Bundle 1: default no-op stub for NPC_DEL test fixture. Tests
// exercising RemoveNpc override via fakeWorldRemoveNpc.
func (m *mockWorld) RemoveNpc(npc ActiveNpc, duration int) {}
```

- [ ] **Step 4: Build + vet to verify**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && go vet ./...
```
Expected: clean exit.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/state.go modules/world/server_varp.go pkg/script/handlers_vars_test.go
git commit --no-gpg-sign -m "feat(nai-126): T1.2 — WorldVars.RemoveNpc + adapter + mock stub"
```

---

### Task 1.3: Write 5 RED tests for `handleNpcDel`

**Files:**
- Modify: `pkg/script/handlers_npc_test.go` (append at end of file or alongside existing NPC handler tests)

Existing reference: `fakeWorldRemoveObj` at `pkg/script/handlers_obj_test.go:33-42`. Mirror its shape exactly.

- [ ] **Step 1: Write `fakeWorldRemoveNpc` recorder + 5 tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// fakeWorldRemoveNpc records RemoveNpc calls. Embeds *mockWorld so the
// rest of the WorldVars surface stays no-op. Mirrors fakeWorldRemoveObj
// at handlers_obj_test.go:33.
type fakeWorldRemoveNpc struct {
	*mockWorld
	calls []struct {
		npc      ActiveNpc
		duration int
	}
}

func (f *fakeWorldRemoveNpc) RemoveNpc(npc ActiveNpc, duration int) {
	f.calls = append(f.calls, struct {
		npc      ActiveNpc
		duration int
	}{npc, duration})
}

func newNpcDelState(t *testing.T, npc ActiveNpc, world WorldVars) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "npc_del",
		Opcodes:          []Opcode{OpNpcDel, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.World = world
	state.Pointers |= PtrActiveNpc
	return state
}

func TestHandleNpcDel_CallsRemoveNpc(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	npc := &mockNpc{typeID: 5, respawnrate: 50}
	state := newNpcDelState(t, npc, w)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("RemoveNpc calls: got %d, want 1", len(w.calls))
	}
	if got, want := w.calls[0].duration, 50; got != want {
		t.Errorf("duration: got %d, want %d", got, want)
	}
}

func TestHandleNpcDel_PassesActiveNpcInstance(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	npc := &mockNpc{typeID: 5, respawnrate: 50}
	state := newNpcDelState(t, npc, w)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("RemoveNpc calls: got %d, want 1", len(w.calls))
	}
	if w.calls[0].npc != ActiveNpc(npc) {
		t.Errorf("npc identity mismatch: got %v, want %v", w.calls[0].npc, npc)
	}
}

func TestHandleNpcDel_NoActiveNpcErrors(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	sf := &ScriptFile{
		Name:             "npc_del_no_active",
		Opcodes:          []Opcode{OpNpcDel, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	// Pointers flag NOT set → requireActiveNpc gate fires.
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NPC_DEL") {
		t.Errorf("error: %v, want containing \"NPC_DEL\"", err)
	}
	if len(w.calls) != 0 {
		t.Errorf("RemoveNpc calls: got %d, want 0", len(w.calls))
	}
}

func TestHandleNpcDel_NilWorldErrors(t *testing.T) {
	npc := &mockNpc{typeID: 5, respawnrate: 50}
	sf := &ScriptFile{
		Name:             "npc_del_nil_world",
		Opcodes:          []Opcode{OpNpcDel, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.World = nil
	state.Pointers |= PtrActiveNpc
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no world surface") {
		t.Errorf("error: %v, want containing \"no world surface\"", err)
	}
}

func TestHandleNpcDel_ZeroRespawnrate(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	npc := &mockNpc{typeID: 5, respawnrate: 0}
	state := newNpcDelState(t, npc, w)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("RemoveNpc calls: got %d, want 1", len(w.calls))
	}
	if got, want := w.calls[0].duration, 0; got != want {
		t.Errorf("duration: got %d, want %d", got, want)
	}
}
```

If `strings` is not yet imported in `handlers_npc_test.go`, add it to the import block. Re-grep first:
```bash
grep -n "^import\|\"strings\"" pkg/script/handlers_npc_test.go | head
```

- [ ] **Step 2: Run RED tests, expect failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcDel -v
```
Expected: all 5 tests FAIL. Most failure mode: `Execute` returns `"no handler for OpNpcDel (opcode 2510)"` because there is no dispatch entry. Two tests (`NoActiveNpcErrors`, `NilWorldErrors`) might appear to pass if their error-shape happens to match `"NPC_DEL"` — confirm test bodies still genuinely fail by reading the exact error string.

If Step 2 shows any test passing on RED, stop and check: the implementation may already exist or the test logic is wrong.

- [ ] **Step 3: Commit RED tests**

```bash
git add pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "test(nai-126): T1.3 — NPC_DEL handler tests (RED)"
```

---

### Task 1.4: Implement `handleNpcDel` + dispatch entry (GREEN)

**Files:**
- Modify: `pkg/script/handlers_npc.go` (insert handler between `handleNpcDamage` at line 300 and `handleNpcDelay` at line 313)
- Modify: `pkg/script/handlers.go` (insert dispatch entry between `OpNpcDamage` line 407 and `OpNpcDelay` line 408)

- [ ] **Step 1: Add `handleNpcDel` to `handlers_npc.go`**

Re-grep before edit:
```bash
grep -n "^func handleNpcDamage\|^func handleNpcDelay\|^// handleNpcDelay" pkg/script/handlers_npc.go
```
to confirm insertion site. Insert directly after `handleNpcDamage`'s closing brace and before the `// handleNpcDelay` comment block:

```go
// handleNpcDel (NPC_DEL, opcode 2510) removes the active NPC. The
// duration passed to World.RemoveNpc is the active NPC type's
// respawnrate; Server.removeNpc scales it by player count and writes
// it to lifecycleTick (RESPAWN-lifecycle) or schedules registry
// cleanup (DESPAWN-lifecycle, currently dead-bool model — see
// modules/world/npc_registry.go:181 and TODO(NAI-19)).
//
// Mirrors TS NpcOps.ts:78-80:
//
//	[ScriptOpcode.NPC_DEL]: checkedHandler(ActiveNpc, state => {
//	    World.removeNpc(state.activeNpc, check(state.activeNpc.type, NpcTypeValid).respawnrate);
//	}),
//
// DEVIATION-NAI-126-D1: nil-World defensive guard (goscape defensive;
// TS skips this check — World is always present in a running engine).
// Mirrors handleObjDel at handlers_obj.go:122-124. Retire when an
// upstream invariant proves s.World is non-nil for any executing
// script.
func handleNpcDel(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DEL"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("NPC_DEL: no world surface")
	}
	s.World.RemoveNpc(s.ActiveNpc, s.ActiveNpc.Respawnrate())
	return nil
}
```

`fmt` is already imported in `handlers_npc.go` (other handlers use it); re-grep to confirm:
```bash
grep -n "\"fmt\"" pkg/script/handlers_npc.go
```

- [ ] **Step 2: Add dispatch entry to `handlers.go`**

In `pkg/script/handlers.go`, line 407 currently reads `OpNpcDamage:            handleNpcDamage,`. Insert one line after it (alphabetic between `Damage` and `Delay`):

```go
	OpNpcDel:               handleNpcDel,
```

The trailing-column whitespace alignment matches the surrounding entries (use the same column count as `OpNpcDamage`). Re-read lines 405-410 first to confirm.

- [ ] **Step 3: Run tests, expect GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcDel -v
```
Expected: all 5 tests PASS.

- [ ] **Step 4: Run full test + vet + build sweep**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && go vet ./... && go build ./...
```
Expected: all packages PASS. Pre-existing modernization warnings on `state.go`/`runner.go`/`handlers_npc.go`/test files (catalogued at NAI-124 close, retired in this plan's Bundle 3) are NOT introduced by Bundle 1 — confirm any new warnings are not on lines this task touched. If unsure, `git stash && go vet ./... && git stash pop` to compare.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go
git commit --no-gpg-sign -m "fix(nai-126): T1.4 — handleNpcDel handler + dispatch entry (GREEN)"
```

---

## Bundle 2 — paramtype DefaultInt sign-extension

### Task 2.1: Write RED tests in new `paramtype_test.go`

**Files:**
- Create: `pkg/objtype/paramtype_test.go`

Verified absent at HEAD `6d04cf8`:
```bash
ls pkg/objtype/paramtype_test.go 2>&1 | head
```
should report no such file.

- [ ] **Step 1: Find the helper that constructs a one-field packet for Decode**

Decode takes `(code uint8, dat *packet2.Packet)`. The packet helper is at `pkg/io/packet/`. Re-grep for an existing decoder test pattern:

```bash
grep -rn "P4\|.PutUint32\|G4()" pkg/objtype/*_test.go pkg/io/packet/*.go 2>/dev/null | head
```

The convention used by sibling tests (e.g., `enumtype_test.go` if present, or any `_test.go` in `pkg/objtype/`) is to construct a `*packet.Packet` with the wire bytes via `.P4(...)`, then pass `&pkt` (or `pkt`) into `Decode`.

Re-grep to confirm:
```bash
grep -rn "packet.NewPacket\|packet2.NewPacket\|&packet.Packet" pkg/objtype/ 2>/dev/null | head
```

Use the exact construction form already in the package. If no existing test uses `Packet.P4`, the import is `github.com/zsrv/goscape/pkg/io/packet` (re-grep to confirm; `paramtype.go` itself uses `packet2 "github.com/zsrv/goscape/pkg/io/packet"`).

- [ ] **Step 2: Write the test file**

Write `pkg/objtype/paramtype_test.go`:

```go
package objtype

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func newParamPacket(b0, b1, b2, b3 uint8) *packet.Packet {
	pkt := packet.NewPacket(nil)
	pkt.P1(b0)
	pkt.P1(b1)
	pkt.P1(b2)
	pkt.P1(b3)
	return pkt
}

func TestParamType_DecodeNegativeDefault(t *testing.T) {
	pt := NewParamType(0)
	pkt := newParamPacket(0xFF, 0xFF, 0xFF, 0xFF)
	if err := pt.Decode(2, pkt); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := pt.DefaultInt, int32(-1); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
	if got, want := int(pt.DefaultInt), -1; got != want {
		t.Errorf("int(DefaultInt): got %d, want %d", got, want)
	}
}

func TestParamType_DecodePositiveDefault(t *testing.T) {
	pt := NewParamType(0)
	pkt := newParamPacket(0x00, 0x00, 0x00, 0x64)
	if err := pt.Decode(2, pkt); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := pt.DefaultInt, int32(100); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
}

func TestParamType_DecodeMaxInt32(t *testing.T) {
	pt := NewParamType(0)
	pkt := newParamPacket(0x7F, 0xFF, 0xFF, 0xFF)
	if err := pt.Decode(2, pkt); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := pt.DefaultInt, int32(2147483647); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
}
```

If `packet.NewPacket(nil)` is the wrong constructor or `P1` is not the right write method, re-grep `pkg/io/packet/packet.go` for the actual API:
```bash
grep -n "^func NewPacket\|^func New\|^func (p \*Packet) P[0-9]" pkg/io/packet/packet.go | head
```
and adjust the helper accordingly. The intent is: produce a packet whose internal buffer has 4 bytes `[b0, b1, b2, b3]` so that `dat.G4()` reads them as a big-endian uint32.

- [ ] **Step 3: Run tests, expect RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestParamType -v
```
Expected: `TestParamType_DecodeNegativeDefault` FAILS with `DefaultInt: got 4294967295, want -1` (because the field is currently `uint32`). The two non-negative tests may PASS at this stage (their values fit in both `uint32` and `int32`); that is fine — they pin the GREEN-state correct behavior.

If the negative test does not fail (e.g., compiles but the assertion `int32(-1)` triggers a Go type error because `pt.DefaultInt` is currently `uint32`), the test code's assertion line itself will be a build error — which is also RED. Move to Step 4 either way.

- [ ] **Step 4: Commit RED tests**

```bash
git add pkg/objtype/paramtype_test.go
git commit --no-gpg-sign -m "test(nai-126): T2.1 — paramtype DefaultInt sign-extension tests (RED)"
```

---

### Task 2.2: Convert `DefaultInt` to `int32` (GREEN)

**Files:**
- Modify: `pkg/objtype/paramtype.go` (lines 111, 121, 183 per HEAD `6dcb29d`)

- [ ] **Step 1: Re-grep to confirm line numbers at HEAD**

```bash
grep -n "DefaultInt\b" pkg/objtype/paramtype.go
```
Expected output: `111:	DefaultInt    uint32`, `121:		pt.DefaultInt = dat.G4()`, `183:		//DefaultInt: -1, // this is -1 in js, default 0 here`. If line numbers differ, use the current HEAD lines for the edits below.

- [ ] **Step 2: Change the field type (line 111)**

```go
	DefaultInt    int32
```

- [ ] **Step 3: Cast at decode (line 121)**

```go
		pt.DefaultInt = int32(dat.G4())
```

- [ ] **Step 4: Drop the obsolete comment (line 183)**

Remove the entire line `		//DefaultInt: -1, // this is -1 in js, default 0 here` from the `NewParamType` constructor body. The post-bundle field type (`int32`) and consumer sign-extension via `int(pt.DefaultInt)` together make the original concern obsolete.

- [ ] **Step 5: Run paramtype tests, expect GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestParamType -v
```
Expected: all 3 tests PASS.

- [ ] **Step 6: Run full test + vet + build to confirm consumer sites still compile**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && go vet ./... && go build ./...
```
Expected: all packages PASS. The 3 consumer sites (`pkg/script/handlers_config.go:51`, `pkg/script/handlers_inv.go:256`, `modules/world/npc_hunt.go:297`) all use `int(pt.DefaultInt)` which type-promotes correctly from `int32`. If a NEW consumer added since 2026-05-08 reads `pt.DefaultInt` directly as `uint32`, it will fail to compile — re-grep at HEAD to surface:
```bash
grep -rn "pt\.DefaultInt\b\|\.DefaultInt\b" pkg/ modules/ 2>/dev/null | grep -v _test.go | grep -v memory/ | head -20
```
Confirm only the 3 known sites + the producer (paramtype.go) appear.

- [ ] **Step 7: Commit**

```bash
git add pkg/objtype/paramtype.go
git commit --no-gpg-sign -m "fix(nai-126): T2.2 — paramtype DefaultInt uint32→int32 (GREEN)"
```

---

## Bundle 3 — Modernization sweep

### Task 3.1: S1001 copy-loop fixes in `state.go` and `runner.go`

**Files:**
- Modify: `pkg/script/state.go` (lines 380, 385, 403, 407)
- Modify: `pkg/script/runner.go` (lines 30, 33)

All 6 sites at HEAD have the same shape: `for i, v := range src { dst[i] = v }`. Rewrite as `copy(dst, src)`. `copy` correctly copies `min(len(dst), len(src))` — same semantics as the bounded for-loop because `dst` is always pre-allocated to a length ≥ `len(src)` at every site here (verified inline below).

- [ ] **Step 1: Re-grep to confirm line numbers at HEAD**

```bash
grep -n "for i, v := range" pkg/script/state.go pkg/script/runner.go
```
Expected:
- `pkg/script/state.go:380:	for i, v := range intArgs {`
- `pkg/script/state.go:385:	for i, v := range stringArgs {`
- `pkg/script/state.go:403:	for i, v := range intArgs {`
- `pkg/script/state.go:407:	for i, v := range stringArgs {`
- `pkg/script/runner.go:30:	for i, v := range intArgs {`
- `pkg/script/runner.go:33:	for i, v := range stringArgs {`

If line numbers shifted, use the current ones.

- [ ] **Step 2: Read each site to confirm it is a clean copy**

For each of the 6 sites, Read the surrounding ~5 lines and confirm:
- The body is exactly `dst[i] = v` (where `dst` is `s.IntLocals` or `s.StringLocals` or `intLocals`/`stringLocals`).
- The dst slice is allocated with `make([]T, max(...))` such that `len(dst) >= len(src)` is invariant (e.g., `make([]int, max(int(target.IntLocalCount), len(intArgs)))`).

Both invariants hold at every site (verified in spec §3 review). If any site deviates (e.g., does an element-wise transform), leave it alone.

- [ ] **Step 3: Edit `pkg/script/state.go` lines 380-381**

Original:
```go
	for i, v := range intArgs {
		s.IntLocals[i] = v
	}
```

Replace with:
```go
	copy(s.IntLocals, intArgs)
```

- [ ] **Step 4: Edit `pkg/script/state.go` lines 385-387 (string variant)**

Original:
```go
	for i, v := range stringArgs {
		s.StringLocals[i] = v
	}
```

Replace with:
```go
	copy(s.StringLocals, stringArgs)
```

- [ ] **Step 5: Edit `pkg/script/state.go` lines 403-405**

Same `intArgs` → `s.IntLocals` shape. Replace with `copy(s.IntLocals, intArgs)`.

- [ ] **Step 6: Edit `pkg/script/state.go` lines 407-409**

Same `stringArgs` → `s.StringLocals` shape. Replace with `copy(s.StringLocals, stringArgs)`.

- [ ] **Step 7: Edit `pkg/script/runner.go` lines 30-32 and 33-35**

Both have shape `for i, v := range intArgs { intLocals[i] = v }` and `for i, v := range stringArgs { stringLocals[i] = v }`. Replace with `copy(intLocals, intArgs)` and `copy(stringLocals, stringArgs)` respectively.

Note: the destination variable in `runner.go` is the local var `intLocals`/`stringLocals`, NOT `s.IntLocals`. Read lines 22-40 first to confirm the variable names in scope.

- [ ] **Step 8: Run tests + vet to confirm no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && go vet ./...
```
Expected: all PASS. The 6 S1001 warnings should disappear from `go vet` output. Other modernization warnings (minmax in handlers_npc.go, rangeint in test files) remain — they retire in tasks 3.2 / 3.3.

- [ ] **Step 9: Commit**

```bash
git add pkg/script/state.go pkg/script/runner.go
git commit --no-gpg-sign -m "refactor(nai-126): T3.1 — S1001 copy-loops → copy() (state.go, runner.go)"
```

---

### Task 3.2: minmax fixes in `handlers_npc.go`

**Files:**
- Modify: `pkg/script/handlers_npc.go` (lines 932, 969-971, 1003-1005)

Three minmax sites at HEAD `6dcb29d`:
- Line 932: `if dx > dz { s.PushInt(dx) } else { s.PushInt(dz) }` (NPC_RANGE — max)
- Lines 969-971: `if added > 255 { added = 255 }` (NPC_STATADD — min clamp)
- Lines 1003-1005: `if subbed < 0 { subbed = 0 }` (NPC_STATSUB — max clamp)

Note: spec §3 listed only 2 sites (`923/957`), but plan-author re-grep at HEAD found a 3rd (`1003-1005`). All 3 ship in this task.

- [ ] **Step 1: Re-grep to confirm sites at HEAD**

```bash
grep -n "if dx > dz\|if added > 255\|if subbed < 0\|added = 255\|subbed = 0" pkg/script/handlers_npc.go
```
Expected output matches the lines above. If shifted, use HEAD lines.

- [ ] **Step 2: Edit NPC_RANGE max site (~line 932)**

Original:
```go
	if dx > dz {
		s.PushInt(dx)
	} else {
		s.PushInt(dz)
	}
```

Replace with:
```go
	s.PushInt(max(dx, dz))
```

- [ ] **Step 3: Edit NPC_STATADD min-clamp site (~lines 969-971)**

Original:
```go
	if added > 255 {
		added = 255
	}
```

Replace with:
```go
	added = min(added, 255)
```

- [ ] **Step 4: Edit NPC_STATSUB max-clamp site (~lines 1003-1005)**

Original:
```go
	if subbed < 0 {
		subbed = 0
	}
```

Replace with:
```go
	subbed = max(subbed, 0)
```

- [ ] **Step 5: Run tests + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ && go vet ./pkg/script/
```
Expected: all PASS. NPC_RANGE / NPC_STATADD / NPC_STATSUB tests should still GREEN — semantics are identical for the integer types used.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_npc.go
git commit --no-gpg-sign -m "refactor(nai-126): T3.2 — minmax in NPC_RANGE/STATADD/STATSUB"
```

---

### Task 3.3: rangeint fixes in test files

**Files:**
- Modify: `pkg/script/handlers_npc_test.go:2113`
- Modify: `pkg/script/handlers_player_test.go:146`

- [ ] **Step 1: Re-grep to confirm sites at HEAD**

```bash
grep -n "for i := 0; i < " pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go
```
Expected:
- `pkg/script/handlers_npc_test.go:2113:	for i := 0; i < 5; i++ { // bounded loop — guards against infinite-loop bugs`
- `pkg/script/handlers_player_test.go:146:	for i := 0; i < NumStats; i++ {`

If the lines shifted, use HEAD lines.

- [ ] **Step 2: Edit `handlers_npc_test.go:2113`**

Original:
```go
	for i := 0; i < 5; i++ { // bounded loop — guards against infinite-loop bugs
```

Replace with:
```go
	for i := range 5 { // bounded loop — guards against infinite-loop bugs
```

- [ ] **Step 3: Edit `handlers_player_test.go:146`**

Original:
```go
	for i := 0; i < NumStats; i++ {
```

Replace with:
```go
	for i := range NumStats {
```

- [ ] **Step 4: Run tests + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ && go vet ./pkg/script/
```
Expected: all PASS.

- [ ] **Step 5: Final full sweep — confirm catalogued warnings retired**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && go vet ./... && go build ./...
```
Expected: all packages PASS. The S1001/minmax/rangeint warnings catalogued at NAI-124 close (`nai_followups.md:6313`) should no longer appear for the lines this Bundle touched. Other modernization warnings, if any, are pre-existing and out-of-scope.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "refactor(nai-126): T3.3 — rangeint in handlers_npc_test, handlers_player_test"
```

---

## Post-implementation: smoke handoff

After all 3 bundles commit, the controller invokes `superpowers:requesting-code-review` with a Sonnet code-reviewer (`superpowers_code_reviewer_model`) over the Bundle-1 diff (the only PRIMARY-binding change). On reviewer approval:

1. **Smoke handoff** — controller asks the user to launch the server (`smoke_test_server_handoff`); user reproduces the cascade scenario (fresh char + bronze dagger vs Tutorial Island giant rat); controller confirms the `no handler for NPC_DEL` WARN is gone and rat-respawn cycles work.

2. **Close commit** — `chore(close): NAI-126 — ...` with `Closes memory:` trailer (`close_commit_memory_trailer`) enumerating any new memory entries surfaced during this sub-spec.

If the smoke surfaces an adjacent residual:
- ≤30 LOC and shape clearly bounded → in-scope-stretch into NAI-126.
- Otherwise → route forward as NAI-127 candidate per `smoke_surfaces_adjacent_divergences`.
