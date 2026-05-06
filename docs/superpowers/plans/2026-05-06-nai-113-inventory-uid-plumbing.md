# NAI-113 — Inventory side-panel uid plumbing fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compose `Player.uid` at login per TS World.ts:937 and switch `updateInvs` to look up listener Source via `LookupPlayerByUID`, restoring per-player inventory wire emission for the Tutorial Island bronze-axe + tinderbox display (and unblocking the latent FINDUID/PFINDUID/UID opcodes as a cascade fix).

**Architecture:** Two production-line edits + one new helper file (`composeUID`) + one test-helper one-liner. Existing test fixtures continue to work as-is because `composeUID(0, slot) == slot` (tests have empty username → username37=0). Stretch-scope adds unit tests for FINDUID/PFINDUID/UID opcodes that were dead under the prior `uid==-1` defaults.

**Tech Stack:** Go 1.26+. No new dependencies. Builds via `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.

---

## File Structure

| File | Status | Responsibility |
|------|--------|----------------|
| `modules/world/player_uid.go` | NEW | `composeUID(username37 uint64, slot int) int` — single source of truth for the formula |
| `modules/world/player_uid_test.go` | NEW | Unit tests for `composeUID` against TS-derived input/output pairs |
| `modules/world/server.go` | MODIFY | `Server.addPlayer` calls `composeUID` after slot assignment |
| `modules/world/server_test.go` | MODIFY | Integration test for `addPlayer` setting non-(-1) uid |
| `modules/world/player.go` | MODIFY | `updateInvs` switches `players[Source]` → `LookupPlayerByUID(Source)` |
| `modules/world/inv_update_test.go` | MODIFY | `newInvListenerTestPlayer` composes uid; new cross-player integration test |
| `pkg/script/handlers_player_test.go` | MODIFY | Add FINDUID + P_FINDUID composed-uid tests |
| `pkg/script/handlers_dialog_test.go` | MODIFY | Add UID opcode push test |
| `pkg/script/handlers_inv_test.go` | MODIFY | Augment INV_TRANSMIT test asserting composed-uid Source |

---

## Task 1: `composeUID` helper + unit test

**Files:**
- Create: `modules/world/player_uid.go`
- Create: `modules/world/player_uid_test.go`

- [ ] **Step 1.1: Write the failing test**

Create `modules/world/player_uid_test.go`:

```go
package world

import "testing"

func TestComposeUID(t *testing.T) {
	tests := []struct {
		name       string
		username37 uint64
		slot       int
		want       int
	}{
		{
			name:       "zero username37 + slot returns slot only",
			username37: 0,
			slot:       2,
			want:       2,
		},
		{
			name:       "zero username37 + slot 1",
			username37: 0,
			slot:       1,
			want:       1,
		},
		{
			name:       "zero username37 + max-11-bit slot",
			username37: 0,
			slot:       2047, // 0x7FF
			want:       2047,
		},
		{
			name:       "username37=1 + slot=0 shifts up 11 bits",
			username37: 1,
			slot:       0,
			want:       1 << 11, // 2048
		},
		{
			name:       "username37=1 + slot=2 ORs slot in",
			username37: 1,
			slot:       2,
			want:       (1 << 11) | 2, // 2050
		},
		{
			name:       "max-21-bit username37 + max-11-bit slot",
			username37: 0x1FFFFF,
			slot:       0x7FF,
			want:       (0x1FFFFF << 11) | 0x7FF,
		},
		{
			name:       "username37 above 21 bits is masked",
			username37: 0x1FFFFF | (1 << 21), // bit 21 should be discarded
			slot:       5,
			want:       (0x1FFFFF << 11) | 5,
		},
		{
			name:       "slot above 11 bits is masked",
			username37: 1,
			slot:       0x7FF | (1 << 11), // bit 11 should be discarded
			want:       (1 << 11) | 0x7FF,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := composeUID(tc.username37, tc.slot)
			if got != tc.want {
				t.Errorf("composeUID(%#x, %d) = %d, want %d", tc.username37, tc.slot, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 1.2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestComposeUID ./modules/world/`

Expected: build failure with `undefined: composeUID`.

- [ ] **Step 1.3: Write minimal implementation**

Create `modules/world/player_uid.go`:

```go
package world

// composeUID derives a Player.uid from username37 + slot.
//
// Mirrors TS Engine-TS World.ts:937:
//
//	player.uid = ((Number(player.username37 & 0x1fffffn) << 11) | player.slot) >>> 0;
//
// The lower 21 bits of username37 are shifted up 11 bits; the 11-bit
// slot occupies the low bits. Stable per (account, slot) for the
// session. Goscape masks slot to 11 bits defensively (TS slot is ≤2047
// by construction); username37 is masked to 21 bits matching TS.
//
// Single source of truth for the formula. Production callers:
// Server.addPlayer. Test callers: newInvListenerTestPlayer (and any
// future test fixture that needs a deterministic per-player uid).
func composeUID(username37 uint64, slot int) int {
	return int(((username37 & 0x1FFFFF) << 11) | uint64(slot&0x7FF))
}
```

- [ ] **Step 1.4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestComposeUID ./modules/world/`

Expected: PASS, all 8 sub-cases green.

- [ ] **Step 1.5: Commit**

```bash
git add modules/world/player_uid.go modules/world/player_uid_test.go
git commit --no-gpg-sign -m "feat(uid): NAI-113 T1 — composeUID helper per TS World.ts:937"
```

---

## Task 2: Wire `composeUID` into `Server.addPlayer`

**Files:**
- Modify: `modules/world/server.go:683-704`
- Modify: `modules/world/server_test.go` (add a test; if file doesn't exist or test name collides, place the test in `modules/world/player_uid_test.go` next to T1's tests)

- [ ] **Step 2.1: Write the failing test**

Append to `modules/world/player_uid_test.go`:

```go
import (
	"testing"
	// Existing imports above remain; this comment is a hint for the implementer
	// — actually keep imports merged at top of file. Add what's needed:
	"github.com/zsrv/goscape/pkg/util"
)

// TestAddPlayerComposesUID pins NAI-113 Bug A fix: Server.addPlayer
// must compose Player.uid per TS World.ts:937 after slot assignment.
// Pre-fix uid stayed at the constructor default of -1 (newPlayer at
// player.go:430).
func TestAddPlayerComposesUID(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.username = "alice"
	p.username37 = util.ToBase37("alice")

	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	if p.uid == -1 {
		t.Fatal("p.uid: still -1 after addPlayer; composeUID wiring missing")
	}
	want := composeUID(p.username37, p.slot)
	if p.uid != want {
		t.Errorf("p.uid: got %d, want %d (composeUID(%#x, %d))", p.uid, want, p.username37, p.slot)
	}
}

// TestAddPlayerEmptyUsernameComposesSlotOnlyUID pins the test-fixture
// migration premise: with username37=0 (empty username, the default in
// newTestPlayer), composeUID returns slot only. Existing test literals
// `Source: <slot>` continue to match composed uids unchanged.
func TestAddPlayerEmptyUsernameComposesSlotOnlyUID(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	// p.username37 left as default (0).

	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	if p.uid != p.slot {
		t.Errorf("p.uid: got %d, want %d (slot only when username37=0)", p.uid, p.slot)
	}
}
```

If `newTestServer` is not in scope, search for it: `rg -n "func newTestServer" modules/world/`. The constructor lives in `modules/world/server_test.go`.

- [ ] **Step 2.2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAddPlayerComposesUID ./modules/world/`

Expected: FAIL with `p.uid: still -1 after addPlayer; composeUID wiring missing`.

- [ ] **Step 2.3: Wire composeUID into addPlayer**

Open `modules/world/server.go:683-704`. The current `addPlayer` body is:

```go
func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	for i := 1; i < len(s.players); i++ {
		if s.players[i] == nil {
			p.slot = i
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			p.active = true
			if s.zoneMap != nil {
				z := s.zoneMap.Get(p.level, p.x, p.z)
				p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
			}
			if s.rsbuf != nil {
				s.rsbuf.AddPlayer(int32(p.slot))
			}
			return nil
		}
	}
	return errWorldFull
}
```

Insert one line after `p.slot = i`:

```go
			p.slot = i
			p.uid = composeUID(p.username37, p.slot) // NAI-113: TS World.ts:937
			s.players[i] = p
```

- [ ] **Step 2.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestAddPlayer" ./modules/world/`

Expected: PASS for both `TestAddPlayerComposesUID` and `TestAddPlayerEmptyUsernameComposesSlotOnlyUID`. Run the wider package to catch unrelated regressions:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS — no other tests should regress; the addPlayer paths that didn't care about uid still don't care.

- [ ] **Step 2.5: Commit**

```bash
git add modules/world/server.go modules/world/player_uid_test.go
git commit --no-gpg-sign -m "feat(uid): NAI-113 T2 — Server.addPlayer composes Player.uid"
```

---

## Task 3: Switch `updateInvs` to `LookupPlayerByUID` + migrate test helper

**Files:**
- Modify: `modules/world/player.go:766-800` (specifically line 777)
- Modify: `modules/world/inv_update_test.go:11-20` (helper) + add new cross-player test
- Read for reference: `pkg/script/handlers_player.go:857-862` (type-assert convention)

- [ ] **Step 3.1: Write the failing integration test**

Append to `modules/world/inv_update_test.go`:

```go
// TestUpdateInvsSelfListenerEmitsViaComposedUID is the NAI-113 binding
// test: a player listening on their own inv (Source = self uid, the
// production INV_TRANSMIT shape) must produce an UpdateInvFull packet
// when inv.Update fires. Pre-fix this path was silently broken because
// p.uid stayed at -1, INV_TRANSMIT registered Source=-1, and
// updateInvs routed to the world-shared inv table (which had no entry
// for per-player invtypes).
//
// Asserts the full chain: composed uid + Source=composed uid +
// LookupPlayerByUID-based emit.
func TestUpdateInvsSelfListenerEmitsViaComposedUID(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	p, cc := newInvListenerTestPlayer(t, s, 5)
	if p.uid == -1 {
		t.Fatalf("precondition: helper must compose uid; got -1 (T1/T2 wiring missing)")
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = true
	p.invs[93] = inv

	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: p.uid, FirstSeen: false},
	}

	received := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("self-listener with composed uid Source should emit UpdateInvFull; got 0 bytes")
	}
}

// TestUpdateInvsCrossPlayerListenerEmitsViaComposedUID exercises the
// INVOTHER_TRANSMIT shape: viewer's listener at Source=owner.uid must
// resolve the owner via LookupPlayerByUID and emit owner.invs[Type].
func TestUpdateInvsCrossPlayerListenerEmitsViaComposedUID(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = false // FirstSeen should fire emit regardless of Update.
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	if owner.uid == -1 || viewer.uid == -1 {
		t.Fatalf("precondition: helper must compose uids; owner.uid=%d, viewer.uid=%d", owner.uid, viewer.uid)
	}

	viewer.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: owner.uid, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("cross-player FirstSeen listener should emit; got 0 bytes")
	}
	if viewer.invListeners[149].FirstSeen {
		t.Error("FirstSeen should flip to false post-emit")
	}
}
```

- [ ] **Step 3.2: Run new tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestUpdateInvsSelfListenerEmitsViaComposedUID|TestUpdateInvsCrossPlayerListenerEmitsViaComposedUID" ./modules/world/`

Expected: BOTH FAIL with "precondition: helper must compose uid; got -1" — `newInvListenerTestPlayer` does not yet set p.uid.

- [ ] **Step 3.3: Update `newInvListenerTestPlayer` to compose uid**

Edit `modules/world/inv_update_test.go:11-20`. Current body:

```go
func newInvListenerTestPlayer(t *testing.T, s *Server, slot int) (*Player, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.slot = slot
	p.invs = map[int]*inventory.Inventory{}
	s.players[slot] = p
	return p, cc
}
```

Add one line after `p.slot = slot`:

```go
func newInvListenerTestPlayer(t *testing.T, s *Server, slot int) (*Player, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.slot = slot
	p.uid = composeUID(p.username37, p.slot) // NAI-113: matches Server.addPlayer
	p.invs = map[int]*inventory.Inventory{}
	s.players[slot] = p
	return p, cc
}
```

- [ ] **Step 3.4: Run new tests to verify only the lookup-shape failure remains**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestUpdateInvsSelfListenerEmitsViaComposedUID|TestUpdateInvsCrossPlayerListenerEmitsViaComposedUID" ./modules/world/`

Expected:
- `TestUpdateInvsSelfListenerEmitsViaComposedUID` may now PASS (Source=p.uid; old code does `players[p.uid]`. With username37=0 + slot=5, p.uid=5, and `s.players[5]=p` was set by helper, so old code accidentally finds the right player. This is the "tests pass for wrong reason" pattern from `test_passes_for_wrong_reason.md`.) **Important:** This test alone does NOT prove the lookup is via uid; it proves the production path emits a packet. We rely on the LookupPlayerByUID switch in 3.5 to make the test exercise the correct code path.
- `TestUpdateInvsCrossPlayerListenerEmitsViaComposedUID` may PASS for the same coincidental reason (owner.uid=2, owner placed at s.players[2]).

To force a real failure that PROVES the lookup-shape switch is needed, add a third test that breaks the slot-indexed coincidence. Append to `inv_update_test.go`:

```go
// TestUpdateInvsCrossPlayerNonSlotUID forces the LookupPlayerByUID
// path to be exercised for real: the owner is placed at slot 2 but
// has uid manually set to a value far above any valid slot index, so
// players[uid] would index out of bounds (or a wrong slot) under the
// pre-fix slot-indexed lookup. Under the LookupPlayerByUID fix the
// emit succeeds.
func TestUpdateInvsCrossPlayerNonSlotUID(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	owner.uid = 0xABCDEF // far above len(s.players); pre-fix `players[0xABCDEF]` panics
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = true
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: owner.uid, FirstSeen: false},
	}

	received := drainConn(t, vcc)
	// Under pre-fix code this panics with "index out of range".
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("updateInvs panicked under slot-indexed lookup: %v", r)
		}
	}()
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("cross-player listener with non-slot uid should emit via LookupPlayerByUID; got 0 bytes")
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestUpdateInvsCrossPlayerNonSlotUID" ./modules/world/`

Expected: FAIL with panic recovered (`index out of range`) under pre-fix slot-indexed lookup. This is the test that drives the implementation in 3.5.

- [ ] **Step 3.5: Switch `updateInvs` to `LookupPlayerByUID`**

Edit `modules/world/player.go:773-783`. Current body:

```go
		if l.Source == -1 {
			inv = p.client.server.invs[l.Type]
		} else {
			other := p.client.server.players[l.Source]
			if other == nil {
				continue
			}
			inv = other.invs[l.Type]
		}
		if inv == nil {
			continue
		}
```

Replace the `else` branch:

```go
		if l.Source == -1 {
			inv = p.client.server.invs[l.Type]
		} else {
			otherActive := p.client.server.LookupPlayerByUID(l.Source)
			if otherActive == nil {
				continue
			}
			other, ok := otherActive.(*Player)
			if !ok || other == nil {
				continue
			}
			inv = other.invs[l.Type]
		}
		if inv == nil {
			continue
		}
```

The type-assert mirrors `pkg/script/handlers_player.go:857-862`. `LookupPlayerByUID` returns `script.ActivePlayer` (interface); concrete type within `modules/world` is `*Player`.

- [ ] **Step 3.6: Run all updateInvs-related tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestUpdateInvs" ./modules/world/`

Expected: ALL PASS, including the three new tests + four pre-existing (`TestUpdateInvsFirstSeenFires`, `TestUpdateInvsRespectsDirty`, `TestUpdateInvsWorldSource`, `TestUpdateInvsSkipsMissingSource`). The pre-existing tests pass because composed uid == slot when username37=0, so their `Source: 2` literals continue to match.

Run the wider package:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS, no regressions.

- [ ] **Step 3.7: Commit**

```bash
git add modules/world/player.go modules/world/inv_update_test.go
git commit --no-gpg-sign -m "fix(inv): NAI-113 T3 — updateInvs lookup via LookupPlayerByUID"
```

---

## Task 4: FINDUID two-player composed-uid test (stretch coverage)

**Files:**
- Read for reference: `pkg/script/handlers_player.go:851-866` (handleFindUID)
- Read for reference: `pkg/script/runner_test.go` (mockPlayer + lookup harness)
- Modify: `pkg/script/handlers_player_test.go`

This task adds a unit test only; no production change.

- [ ] **Step 4.1: Locate existing FINDUID test scaffolding**

Run: `rg -n "FindUID|FINDUID|handleFindUID" pkg/script/handlers_player_test.go`

If a `TestFindUID` test already exists, augment it. If none, write fresh. The mockPlayer harness lives in `pkg/script/runner_test.go`.

- [ ] **Step 4.2: Write the test**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestFindUIDComposedUIDLookup pins NAI-113 stretch coverage:
// FINDUID with composed uids (post-fix Server.addPlayer wiring)
// resolves a registered other-player and rebinds Self. Pre-fix this
// path was always dead because every Player.uid was -1 →
// LookupPlayerByUID(any_uid) returned nil → FINDUID always pushed 0.
func TestFindUIDComposedUIDLookup(t *testing.T) {
	self := &mockPlayer{uid: 1234}
	other := &mockPlayer{uid: 5678}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{
		1234: self,
		5678: other,
	}}

	sf := &ScriptFile{
		Name: "find_uid_other",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpFindUID,
			OpReturn,
		},
		IntOperands: []int{5678, 0, 0},
	}
	state := &ScriptState{
		ScriptFile:     sf,
		Self:           self,
		Pointers:       PtrActivePlayer,
		PlayerLookup:   lookup,
		StackCapacity:  4,
	}
	if err := state.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := state.PopInt()
	if got != 1 {
		t.Errorf("FINDUID push: got %d, want 1 (lookup hit)", got)
	}
	if state.Self != other {
		t.Errorf("Self: not rebound to other; got %v", state.Self)
	}
}

// TestFindUIDLookupMiss pins the negative case: FINDUID with an
// unregistered uid pushes 0 and leaves Self untouched.
func TestFindUIDLookupMiss(t *testing.T) {
	self := &mockPlayer{uid: 1234}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{
		1234: self,
	}}

	sf := &ScriptFile{
		Name: "find_uid_miss",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpFindUID,
			OpReturn,
		},
		IntOperands: []int{9999, 0, 0},
	}
	state := &ScriptState{
		ScriptFile:    sf,
		Self:          self,
		Pointers:      PtrActivePlayer,
		PlayerLookup:  lookup,
		StackCapacity: 4,
	}
	if err := state.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := state.PopInt()
	if got != 0 {
		t.Errorf("FINDUID push: got %d, want 0 (lookup miss)", got)
	}
	if state.Self != self {
		t.Errorf("Self: should be untouched on miss; got %v", state.Self)
	}
}
```

**Implementer note:** `mockPlayerLookup` and `mockPlayer.uid` may need to be added to `pkg/script/runner_test.go`. Re-grep before assuming: `rg -n "mockPlayerLookup|byUID" pkg/script/runner_test.go`. If `mockPlayerLookup` does not exist, add a minimal one:

```go
// mockPlayerLookup implements PlayerLookup for handler tests.
type mockPlayerLookup struct {
	byUID map[int]ActivePlayer
}

func (m *mockPlayerLookup) LookupPlayerByUID(uid int) ActivePlayer {
	if m == nil {
		return nil
	}
	return m.byUID[uid]
}
```

If `mockPlayer` does not have a `uid` field or `UID()` method, follow the existing pattern in `runner_test.go` for adding fields (the file is the canonical mock). Re-grep: `rg -n "type mockPlayer\b|func \(m \*mockPlayer\) UID" pkg/script/runner_test.go`.

- [ ] **Step 4.3: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestFindUID" ./pkg/script/`

Expected: PASS (both new tests).

- [ ] **Step 4.4: Commit**

```bash
git add pkg/script/handlers_player_test.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "test(script): NAI-113 T4 — FINDUID composed-uid coverage"
```

---

## Task 5: P_FINDUID self-reacquire fast-path test

**Files:**
- Read: `pkg/script/handlers_player.go:868-899` (handlePFindUID)
- Modify: `pkg/script/handlers_player_test.go`

- [ ] **Step 5.1: Write the test**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestPFindUIDSelfReacquireFastPath pins the post-NAI-113 self-
// reacquire short-circuit: when the script is already protected on a
// player whose UID matches the popped uid, P_FINDUID pushes 1 without
// consulting PlayerLookup. Pre-fix this branch was dead because
// s.Self.UID() was always -1 — `-1 == any_uid` was always false.
func TestPFindUIDSelfReacquireFastPath(t *testing.T) {
	self := &mockPlayer{uid: 1234}
	// Lookup that panics if called — proves the fast-path bypasses it.
	lookup := &panicOnLookup{}

	sf := &ScriptFile{
		Name: "pfind_uid_self",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPFindUID,
			OpReturn,
		},
		IntOperands: []int{1234, 0, 0},
	}
	state := &ScriptState{
		ScriptFile:    sf,
		Self:          self,
		Pointers:      PtrActivePlayer,
		Protect:       true,
		PlayerLookup:  lookup,
		StackCapacity: 4,
	}
	if err := state.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := state.PopInt()
	if got != 1 {
		t.Errorf("P_FINDUID self-reacquire push: got %d, want 1", got)
	}
	if state.Self != self {
		t.Errorf("Self: must remain unchanged on fast-path; got %v", state.Self)
	}
}

// panicOnLookup proves the fast-path bypasses PlayerLookup entirely.
type panicOnLookup struct{}

func (p *panicOnLookup) LookupPlayerByUID(uid int) ActivePlayer {
	panic("PlayerLookup must not be consulted on P_FINDUID self-reacquire fast-path")
}

// TestPFindUIDSelfReacquireSkippedWhenUnprotected pins the inverse:
// even when uid matches Self, P_FINDUID consults PlayerLookup if the
// script is not currently Protect=true.
func TestPFindUIDSelfReacquireSkippedWhenUnprotected(t *testing.T) {
	self := &mockPlayer{uid: 1234, canAccess: true}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{
		1234: self,
	}}

	sf := &ScriptFile{
		Name: "pfind_uid_self_unprotected",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPFindUID,
			OpReturn,
		},
		IntOperands: []int{1234, 0, 0},
	}
	state := &ScriptState{
		ScriptFile:    sf,
		Self:          self,
		Pointers:      PtrActivePlayer,
		Protect:       false,
		PlayerLookup:  lookup,
		StackCapacity: 4,
	}
	if err := state.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := state.PopInt()
	if got != 1 {
		t.Errorf("P_FINDUID via lookup push: got %d, want 1", got)
	}
	if !state.Protect {
		t.Error("Protect: must be set to true on successful lookup")
	}
}
```

**Implementer note:** `mockPlayer.canAccess` field — re-grep `pkg/script/runner_test.go`; if absent, add it and a `CanAccess()` method that returns `m.canAccess`. The handler at `handlers_player.go:890` calls `target.CanAccess()`; without it, `TestPFindUIDSelfReacquireSkippedWhenUnprotected` would push 0 even with a valid lookup.

- [ ] **Step 5.2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestPFindUID" ./pkg/script/`

Expected: PASS (both new tests).

- [ ] **Step 5.3: Commit**

```bash
git add pkg/script/handlers_player_test.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "test(script): NAI-113 T5 — P_FINDUID self-reacquire fast-path"
```

---

## Task 6: UID opcode push test

**Files:**
- Read: `pkg/script/handlers_dialog.go:115-121` (handleUID)
- Modify: `pkg/script/handlers_dialog_test.go`

- [ ] **Step 6.1: Locate existing scaffolding**

Run: `rg -n "OpUID|handleUID|TestUID" pkg/script/handlers_dialog_test.go pkg/script/opcode.go`

Note the opcode constant name (likely `OpUID`).

- [ ] **Step 6.2: Write the test**

Append to `pkg/script/handlers_dialog_test.go`:

```go
// TestUIDOpcodePushesComposedUID pins NAI-113 stretch coverage:
// the UID opcode pushes the active player's composed uid, not -1.
// Pre-fix Player.uid was always -1 because Server.addPlayer never
// composed it; runescript callers branching on this value would
// short-circuit on -1.
func TestUIDOpcodePushesComposedUID(t *testing.T) {
	self := &mockPlayer{uid: 0xABCDEF}

	sf := &ScriptFile{
		Name: "uid_push",
		Opcodes: []Opcode{
			OpUID,
			OpReturn,
		},
	}
	state := &ScriptState{
		ScriptFile:    sf,
		Self:          self,
		Pointers:      PtrActivePlayer,
		StackCapacity: 4,
	}
	if err := state.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := state.PopInt()
	if got == -1 {
		t.Fatal("UID opcode pushed -1; composed-uid wiring not exercised")
	}
	if got != 0xABCDEF {
		t.Errorf("UID opcode push: got %#x, want %#x", got, 0xABCDEF)
	}
}
```

- [ ] **Step 6.3: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestUIDOpcode" ./pkg/script/`

Expected: PASS.

- [ ] **Step 6.4: Commit**

```bash
git add pkg/script/handlers_dialog_test.go
git commit --no-gpg-sign -m "test(script): NAI-113 T6 — UID opcode pushes composed uid"
```

---

## Task 7: INV_TRANSMIT registers Source = composed uid

**Files:**
- Read: `pkg/script/handlers_inv.go:422-433` (handleInvTransmit)
- Read: `pkg/script/handlers_inv_test.go:383-415` (existing TestInvTransmitRegistersListener)
- Modify: `pkg/script/handlers_inv_test.go`

The existing INV_TRANSMIT test sets `mockPlayer.uid = 42` (a literal). It already passes because `mockPlayer.UID()` returns whatever uid field is set. The change here is to add a SECOND test that proves the Source comes from the active player's UID specifically (not from any other source), and that mockPlayer.UID() returns the composed value when uid is composed.

- [ ] **Step 7.1: Read the existing test to confirm shape**

Run: `rg -n "TestInvTransmitRegistersListener|mockPlayer{uid" pkg/script/handlers_inv_test.go`

Open the file and read lines 383-415. Verify the assertion is `Source: 42` literal.

- [ ] **Step 7.2: Add a complementary test**

Append to `pkg/script/handlers_inv_test.go`:

```go
// TestInvTransmitSourceTracksActivePlayerUID pins NAI-113 the wire:
// INV_TRANSMIT propagates the active player's UID() to the listener
// Source field — independent of any literal. If a future refactor
// changes mockPlayer.UID() return shape (e.g., composed via formula),
// the listener Source must follow.
func TestInvTransmitSourceTracksActivePlayerUID(t *testing.T) {
	const wantUID = 0xDEADBEE // arbitrary non-(-1), non-42 sentinel
	mp := &mockPlayer{uid: wantUID}

	sf := &ScriptFile{
		Name: "inv_transmit_uid_track",
		Opcodes: []Opcode{
			OpPushConstantInt, // com (bottom of pop order: popInt() pops it first)
			OpPushConstantInt, // invType
			OpInvTransmit,
			OpReturn,
		},
		// Note pop order in handleInvTransmit: invType = PopInt() (top); com = PopInt() (bottom).
		// Push order is com first, then invType.
		IntOperands: []int{149, 93, 0, 0},
	}
	state := &ScriptState{
		ScriptFile:    sf,
		Self:          mp,
		Pointers:      PtrActivePlayer,
		StackCapacity: 4,
	}
	if err := state.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(mp.lastInvListenOnCom) != 1 {
		t.Fatalf("expected 1 InvListenOnCom call, got %d", len(mp.lastInvListenOnCom))
	}
	got := mp.lastInvListenOnCom[0]
	if got.Source != wantUID {
		t.Errorf("Source: got %d, want %d (must propagate active player UID)", got.Source, wantUID)
	}
}
```

**Implementer note:** Verify the push order against `handleInvTransmit` body. `pkg/script/handlers_inv.go:422-432`:
```go
invType := s.PopInt()
com := s.PopInt()
```
Top of stack is invType; below is com. Push order in fixture: com first (pushed earlier → bottom), then invType. The `IntOperands` slice indexes match `OpPushConstantInt` order in the Opcodes slice. If existing fixture in `TestInvTransmitRegistersListener` has a different convention, mirror it instead.

- [ ] **Step 7.3: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvTransmit" ./pkg/script/`

Expected: PASS.

- [ ] **Step 7.4: Commit**

```bash
git add pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "test(script): NAI-113 T7 — INV_TRANSMIT Source tracks active UID"
```

---

## Task 8: Full-suite verification + smoke handoff + close commit

**Files:**
- Read: existing memory entries on close commits + smoke handoff

- [ ] **Step 8.1: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: ALL packages PASS. Any failure is a regression introduced by NAI-113 work; do NOT proceed to smoke handoff until clean.

- [ ] **Step 8.2: Run race detector on world + script packages**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ ./pkg/script/`

Expected: PASS, no races.

- [ ] **Step 8.3: Build the binary**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o /tmp/goscape-nai-113 ./cmd/goscape`

Expected: clean build.

- [ ] **Step 8.4: Smoke handoff to user**

Per `smoke_test_server_handoff.md`: the smoke must be user-launched (Java client cannot reach Claude's sandboxed server process).

Output to user:

> NAI-113 implementation complete. Smoke handoff:
>
> Please run the server (`./goscape --config.file config.yaml`) and from a fresh-account Java client:
> 1. Walk through Tutorial Island to the Survival Expert dialogue.
> 2. Trigger `^newbie_survival_instructor_open_inventory` (chatbox prompts inventory tab click).
> 3. Click the inventory tab.
>
> Pin: bronze axe (1351) shows in slot 0 AND tinderbox (590) shows in slot 1 of the inventory side panel.
>
> Report back PASS/FAIL with any adjacent symptoms observed.

WAIT for user smoke result before proceeding.

- [ ] **Step 8.5: On smoke PASS — write close commit**

If smoke passes, write the close commit body. Per `close_commit_memory_trailer.md`, include `Closes memory:` trailer with paths to memory entries that change as a result.

```bash
git log --oneline -10  # capture the bundle commit shas (T1-T7)
```

Compose the close commit message:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-113 — inventory side-panel uid plumbing fix

Closes NAI-112 SECONDARY residual. Bug A (Player.uid never composed in
production) + Bug B (updateInvs slot-indexed lookup) both fixed via
composeUID helper + Server.addPlayer wiring + Player.updateInvs switch
to LookupPlayerByUID. Cascade fix unblocks FINDUID/PFINDUID/UID
opcodes (previously dead under uid==-1 default).

Smoke (2026-05-06): Tutorial Island fresh account → Survival Expert
dialogue → inventory tab click → bronze_axe in slot 0 + tinderbox in
slot 1 ✓

Bundles:
  T1 — composeUID helper                           [SHA]
  T2 — Server.addPlayer composes Player.uid       [SHA]
  T3 — updateInvs uses LookupPlayerByUID          [SHA]
  T4 — FINDUID composed-uid tests                 [SHA]
  T5 — P_FINDUID self-reacquire fast-path test    [SHA]
  T6 — UID opcode push test                       [SHA]
  T7 — INV_TRANSMIT Source-tracks-UID test        [SHA]

Closes memory: nai_followups.md (NAI-113 entry); add new memory entry
on uid-broken-consumer cascade pattern.

Spec: docs/superpowers/specs/2026-05-06-nai-113-inventory-uid-plumbing-design.md
Plan: docs/superpowers/plans/2026-05-06-nai-113-inventory-uid-plumbing.md
EOF
)"
```

Replace `[SHA]` placeholders with actual short shas from `git log`.

- [ ] **Step 8.6: Update memory entries**

Per `post_task_handoff.md`, save non-derivable info. Two updates:

1. **Update `nai_followups.md`** — append a `## NAI-113 — CLOSED <DATE>` section with bound + fix + smoke + cascade notes (mirror NAI-112 section shape).

2. **Add new memory entry** at `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/uid_broken_consumer_cascade.md` (or merge into `protocol_stub_not_completed.md` if substantively similar):

```markdown
---
name: uid-broken consumer cascade fix
description: when a per-player struct field defaults to a sentinel and consumers fall through to dead branches, fixing assignment closes multiple stub-not-completed sites at once
type: feedback
---

When a struct field defaults to a sentinel (-1, 0, "") and multiple
consumers fall through to dead-code paths because of it, the fix to
the field's assignment site can simultaneously close multiple
"stub-not-completed" surfaces. Audit them at fix-author time rather
than discovering them later.

**Why:** NAI-113 found Player.uid stayed at -1 in production; INV_TRANSMIT,
FINDUID, P_FINDUID, and UID opcode all silently degraded (lookup
miss / push -1 / always-false self-reacquire). Adding the one-line
composeUID call to addPlayer made all four correct simultaneously.

**How to apply:** When fixing a field-assignment bug, grep all `*.UID()`,
`LookupBy*`, and field-read sites in `pkg/` and `modules/` to surface
the cascade. Add stretch tests for sites whose behavior was
silently broken; document the cascade in the close commit body so
future audits don't re-flag them as untracked deviations.
```

Add a one-line entry to `MEMORY.md`:

```
- [uid-broken consumer cascade fix](uid_broken_consumer_cascade.md) — sentinel-default field that breaks multiple consumers; audit at fix-author time
```

- [ ] **Step 8.7: Final commit (memory updates)**

```bash
git add ... # (memory files live outside the goscape repo; the close commit references them but does not stage them)
```

The memory files live at `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/` — outside the goscape git repo. They persist via the harness's auto-memory system; no commit required.

---

## Self-Review

**Spec coverage:**
- Spec §2 Bug A → T1 + T2 ✓
- Spec §2 Bug B → T3 ✓
- Spec §3.1.1 composeUID → T1 ✓
- Spec §3.1.2 addPlayer wiring → T2 ✓
- Spec §3.1.3 updateInvs LookupPlayerByUID → T3 ✓
- Spec §3.2 test fixture migration → T3 (helper one-liner; existing literals continue to work via `composeUID(0, slot)==slot` invariant) ✓
- Spec §3.3.1 FINDUID → T4 ✓
- Spec §3.3.2 P_FINDUID → T5 ✓
- Spec §3.3.3 UID opcode → T6 ✓
- Spec §3.3.4 INV_TRANSMIT composed-uid Source → T7 ✓
- Spec §3.4 smoke → T8.4 ✓
- Spec §4 test strategy table → all rows mapped to T1-T8 ✓
- Spec §9 close-commit memory entries → T8.5 + T8.6 ✓

**Placeholder scan:** No TBDs. Steps 4.2 / 5.1 / 7.2 contain `Implementer note:` blocks that direct re-grepping of mock infrastructure rather than codifying possibly-stale assumptions — this is the `controller_preflight.md` pattern, not a placeholder.

**Type consistency:**
- `composeUID(uint64, int) int` — same signature in T1 def, T2 caller in addPlayer, T3 caller in test helper.
- `LookupPlayerByUID` returns `script.ActivePlayer` — type-assert to `*Player` at T3.5 (mirrors handler convention at handlers_player.go:857-862).
- `mockPlayer.uid int` + `UID() int` method — referenced consistently across T4/T5/T6/T7. Implementer notes flag if these need adding to runner_test.go.
- `mockPlayerLookup.LookupPlayerByUID(int) ActivePlayer` — interface-conformant.

**Bundle ordering:** T1 (helper) → T2 (production wire-in) → T3 (lookup switch) — production fix complete after T3. T4-T7 are test-only stretch coverage; T8 is verify+smoke+close. No ordering hazards.

**Test sequencing:** Each task writes its failing test first, then implementation, then verifies pass. T3 includes the explicit "test passes for wrong reason" mitigation via `TestUpdateInvsCrossPlayerNonSlotUID` to force the LookupPlayerByUID switch to be exercised by a test that fails under slot-indexed lookup.

**Memory of common pitfalls applied:**
- `enumerate_all_sites.md` — pre-flight enumerated 5 test files; only inv_update_test.go's helper requires modification.
- `controller_preflight.md` — premise verifications baked into Step 4.1, 5.1, 6.1, 7.1 directing re-grep before assuming.
- `test_passes_for_wrong_reason.md` — explicitly addressed at T3.4-3.5 via the non-slot-uid forcing test.
- `close_commit_memory_trailer.md` — applied at T8.5.
- `smoke_test_server_handoff.md` — applied at T8.4.
- `bundle0_short_circuits_stage1_audit.md` — spec §8 documents the rationale for skipping Stage 1 audit cadence.
