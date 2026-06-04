# NAI-127 — Rat-AI handler subset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port six unhandled script opcodes (NPC_FINDHERO, FINDHERO, BOTH_HEROPOINTS, DAMAGE, GENDER, P_PREVENTLOGOUT) to silence the NAI-126-close cascade-tail and unblock the rat-AI loop's death-loot + player-hit-render + combat-prevent-logout primitives.

**Architecture:** Two bundles. Bundle 1 ports the FINDHERO family (3 opcodes sharing HeroPoints/pointer-flip plumbing); Bundle 2 ports rat-attacks-player primitives (3 standalone handlers reusing Bundle 1's `LookupPlayerByUID` adapter). Each handler is added to `pkg/script/handlers_*.go` with its dispatch entry in `pkg/script/handlers.go`; underlying state lives on `*Npc`/`*Player` in `modules/world/`; tests live in `pkg/script/handlers_*_test.go`.

**Tech stack:** Go 1.26+. TDD per `superpowers:test-driven-development`. Sonnet code-reviewer per bundle (never Opus per `superpowers_code_reviewer_model`). Controller pre-flight before each implementer dispatch per `controller_preflight`. User-launched smoke at end of Bundle 2 per `smoke_test_server_handoff`.

**Spec:** `docs/superpowers/specs/2026-05-08-nai-127-rat-ai-handler-subset-design.md` (`5a53aed`).

---

## Bundle 1 — FINDHERO family

Three opcodes (NPC_FINDHERO 2519, FINDHERO 2018, BOTH_HEROPOINTS 2003) sharing HeroPoints/pointer-flip plumbing. Five implementer tasks (T1.1–T1.5) then Sonnet code-reviewer.

### Task 1.1: Add `ActiveNpc.TopContributor` interface method + impl + mocks

**Files:**
- Modify: `pkg/script/active.go` (add interface method to `ActiveNpc`)
- Modify: `modules/world/npc_script.go` (add `*Npc.TopContributor` impl)
- Modify: `pkg/script/handlers_npc_test.go` (extend `mockNpc` with `topContributor` field + getter)
- Modify: `pkg/script/handlers_player_test.go` (extend `mockActiveNpc` with `TopContributor() int` no-op stub)

- [ ] **Step 1: Add `TopContributor()` to `ActiveNpc` interface**

In `pkg/script/active.go`, immediately after the `AddHeroPoints` declaration (existing at line ~824), add:

```go
	// TopContributor returns the playerUID with the largest HeroPoints
	// credit on this NPC's ledger, or 0 if the ledger is empty. Used by
	// NPC_FINDHERO (NpcOps.ts:114-130) — TS uses hash64; goscape uses
	// int playerUID. The 0-empty-sentinel mirrors HeroPoints.TopContributor.
	TopContributor() int
```

- [ ] **Step 2: Add `*Npc.TopContributor` impl**

In `modules/world/npc_script.go`, immediately after the existing `AddHeroPoints` method (line ~74), add:

```go
// TopContributor implements script.ActiveNpc. Returns the playerUID
// with the largest HeroPoints credit, or 0 if the ledger is empty.
// Mirrors TS state.activeNpc.heroPoints.findHero() at NpcOps.ts:115
// (TS returns hash64 -1n on empty; goscape uses 0).
func (n *Npc) TopContributor() int {
	return n.heroPoints.TopContributor()
}
```

- [ ] **Step 3: Extend `mockNpc` recorder**

In `pkg/script/handlers_npc_test.go`, in the `mockNpc` struct (line ~199), add `topContributor int` near the `addHeroPointsCalls` field:

```go
	// NAI-127 Bundle 1: NPC_FINDHERO ledger-top getter.
	topContributor int
```

After the existing `Respawnrate()` getter (line ~256), add:

```go
func (m *mockNpc) TopContributor() int { return m.topContributor }
```

- [ ] **Step 4: Extend `mockActiveNpc` no-op**

In `pkg/script/handlers_player_test.go`, immediately after the existing `func (m *mockActiveNpc) AddHeroPoints(_, _ int) {}` (line ~80), add:

```go
func (m *mockActiveNpc) TopContributor() int                                  { return 0 }
```

- [ ] **Step 5: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean (no errors).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(nai-127): T1.1 — ActiveNpc.TopContributor interface + Npc impl + mock stubs

Adds the ledger-top getter consumed by NPC_FINDHERO (Bundle 1, T1.5).
*Npc delegates to existing HeroPoints.TopContributor (NAI-120 Bundle 2D).
mockNpc records via topContributor int field; mockActiveNpc returns 0."
```

---

### Task 1.2: Add `Player.heroPoints` field + `ActivePlayer.AddHeroPoints/TopContributor` interface methods + `*Player` impls + `mockPlayer` stubs

**Files:**
- Modify: `pkg/script/active.go` (add interface methods to `ActivePlayer`)
- Modify: `modules/world/player.go` (add `heroPoints HeroPoints` field + init at `newPlayer()`)
- Modify: `modules/world/player_script.go` (add `*Player.AddHeroPoints` + `*Player.TopContributor` impls)
- Modify: `pkg/script/runner_test.go` (extend `mockPlayer` with `topContributor` field + `addHeroPointsCalls` + getter + recorder method)

- [ ] **Step 1: Add interface methods to `ActivePlayer`**

In `pkg/script/active.go`, find the existing `Gender()` declaration (line ~582) and insert after it:

```go
	// AddHeroPoints credits `amount` to `playerUID` on the player's
	// hero-point ledger. Mirrors TS Player.heroPoints.addHero(...) at
	// PlayerOps.ts:1167 (BOTH_HEROPOINTS recipient). Parallel to
	// ActiveNpc.AddHeroPoints. NAI-127 Bundle 1.
	AddHeroPoints(playerUID, amount int)

	// TopContributor returns the playerUID with the largest HeroPoints
	// credit on this player's ledger, or 0 if the ledger is empty. Used
	// by FINDHERO (PlayerOps.ts:1138-1154). NAI-127 Bundle 1.
	TopContributor() int
```

- [ ] **Step 2: Add `Player.heroPoints` field**

In `modules/world/player.go`, find the `Npc.heroPoints HeroPoints` field anchor pattern at `npc.go:149` for reference. Then in the `Player` struct (containing `requestLogout, requestIdleLogout, loggingOut bool` near line ~258), add a `heroPoints HeroPoints` field. Recommend placing it near the existing `gender` field area (~line 172) for cohesion. Exact placement:

Add this line where it does not collide with sibling-aligned fields (find a free anchor in the struct definition):

```go
	// NAI-127 Bundle 1: per-player HeroPoints ledger (parallel to
	// Npc.heroPoints from NAI-120). Read by FINDHERO; written by
	// BOTH_HEROPOINTS. TS Player.heroPoints = new HeroPoints(16) at
	// Engine-TS/.../Player.ts:76.
	heroPoints HeroPoints
```

In `newPlayer()` (modules/world/player.go:426), find a clean spot in the constructor body and add:

```go
	p.heroPoints = NewHeroPoints(16)
```

(If `newPlayer()` returns a `&Player{...}` literal directly, add `heroPoints: NewHeroPoints(16),` to the literal instead.)

- [ ] **Step 3: Add `*Player` impls**

In `modules/world/player_script.go`, near other `ActivePlayer` interface impls, add:

```go
// AddHeroPoints implements script.ActivePlayer. Credits amount to
// playerUID on the player's hero-point ledger. Used by BOTH_HEROPOINTS.
// Mirrors TS Player.heroPoints.addHero at PlayerOps.ts:1167.
func (p *Player) AddHeroPoints(playerUID, amount int) {
	p.heroPoints.AddHero(playerUID, amount)
}

// TopContributor implements script.ActivePlayer. Returns the playerUID
// with the largest HeroPoints credit, or 0 if the ledger is empty.
// Used by FINDHERO. Mirrors TS state.activePlayer.heroPoints.findHero()
// at PlayerOps.ts:1139.
func (p *Player) TopContributor() int {
	return p.heroPoints.TopContributor()
}
```

- [ ] **Step 4: Extend `mockPlayer` recorder**

In `pkg/script/runner_test.go`, in the `mockPlayer` struct (~line 99), add fields near `genderValue` (line ~325):

```go
	// NAI-127 Bundle 1: FINDHERO ledger-top getter.
	topContributor int

	// NAI-127 Bundle 1: BOTH_HEROPOINTS recipient recorder. Mirrors
	// mockNpc.addHeroPointsCalls.
	addHeroPointsCalls []struct{ playerUID, amount int }
```

Then add the methods near the existing `Gender()` method (line ~637) — group with other ActivePlayer-interface implementations:

```go
func (m *mockPlayer) TopContributor() int { return m.topContributor }

func (m *mockPlayer) AddHeroPoints(playerUID, amount int) {
	m.addHeroPointsCalls = append(m.addHeroPointsCalls, struct{ playerUID, amount int }{playerUID, amount})
}
```

- [ ] **Step 5: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: all green (no new tests yet; existing pass).

- [ ] **Step 6: Commit**

```bash
git add pkg/script/active.go modules/world/player.go modules/world/player_script.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "feat(nai-127): T1.2 — Player.heroPoints + ActivePlayer.AddHeroPoints/TopContributor

Adds the per-player HeroPoints ledger (parallel to Npc.heroPoints from
NAI-120). Initialized NewHeroPoints(16) at newPlayer() per TS
Player.ts:76. ActivePlayer interface gains AddHeroPoints (consumer
BOTH_HEROPOINTS) and TopContributor (consumer FINDHERO). mockPlayer
records via addHeroPointsCalls slice + topContributor int."
```

---

### Task 1.3: Add `WorldVars.LookupPlayerByUID` interface method + adapter + `mockWorld`

**Files:**
- Modify: `pkg/script/state.go` (add interface method to `WorldVars`)
- Modify: `modules/world/server.go` (add `worldVarsView.LookupPlayerByUID` adapter)
- Modify: `pkg/script/handlers_vars_test.go` (extend `mockWorld` with `playersByUID` map + `LookupPlayerByUID` method)

- [ ] **Step 1: Add interface method**

In `pkg/script/state.go`, find the `RemoveNpc` declaration (line ~100) and insert after the closing comment block of `AddObj` (after line ~108):

```go

	// LookupPlayerByUID resolves a packed Player UID to the matching
	// ActivePlayer, or nil if no logged-in player has that UID. Used by
	// NPC_FINDHERO, FINDHERO, and DAMAGE. Mirrors TS World.getPlayerByUid
	// (PlayerOps.ts:773) / World.getPlayerByHash64 (PlayerOps.ts:1144,
	// NpcOps.ts:120).
	LookupPlayerByUID(uid int) ActivePlayer
```

- [ ] **Step 2: Add `worldVarsView` adapter**

In `modules/world/server.go`, near the existing `worldVarsView` method definitions (other view methods at ~lines 130-260), add:

```go
// LookupPlayerByUID implements script.WorldVars. Delegates to
// Server.LookupPlayerByUID (server.go:782-791). NAI-127 Bundle 1.
func (w worldVarsView) LookupPlayerByUID(uid int) script.ActivePlayer {
	return w.s.LookupPlayerByUID(uid)
}
```

- [ ] **Step 3: Extend `mockWorld`**

In `pkg/script/handlers_vars_test.go`, in the `mockWorld` struct (line ~11), add `playersByUID` field near the existing `players int` field (line ~15) — note: do NOT collide with the existing `players int` (which is PlayerCount, not a lookup table):

```go
	// NAI-127 Bundle 1: LookupPlayerByUID lookup table. Distinct from
	// the existing `players int` field (which backs PlayerCount).
	playersByUID map[int]ActivePlayer
```

Then near other no-op stubs (after `RemoveNpc` at line ~56), add:

```go
func (m *mockWorld) LookupPlayerByUID(uid int) ActivePlayer {
	if m.playersByUID == nil {
		return nil
	}
	return m.playersByUID[uid]
}
```

- [ ] **Step 4: Verify build + existing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/state.go modules/world/server.go pkg/script/handlers_vars_test.go
git commit --no-gpg-sign -m "feat(nai-127): T1.3 — WorldVars.LookupPlayerByUID interface + adapter + mock

Exposes Server.LookupPlayerByUID through WorldVars for the FINDHERO
family handlers (NPC_FINDHERO, FINDHERO, DAMAGE). worldVarsView
delegates; mockWorld backs via playersByUID map (distinct from the
existing players int field that backs PlayerCount)."
```

---

### Task 1.4: RED — write 13 failing handler tests

**Files:**
- Modify: `pkg/script/handlers_npc_test.go` (5 NPC_FINDHERO tests)
- Modify: `pkg/script/handlers_player_test.go` (4 FINDHERO + 4 BOTH_HEROPOINTS tests)

- [ ] **Step 1: Add 5 NPC_FINDHERO tests**

Append to `pkg/script/handlers_npc_test.go` (after the last existing test):

```go
// --- NAI-127 Bundle 1: NPC_FINDHERO (opcode 2519) ---

// newFindHeroState builds a ScriptState with PtrActiveNpc set,
// ActiveNpc=npc, and World=mw, IntOperand-driven by intOperand.
func newNpcFindHeroState(npc ActiveNpc, mw WorldVars, intOperand int) *ScriptState {
	s := &ScriptState{
		StackCapacity: 16,
		World:         mw,
		ActiveNpc:     npc,
		Pointers:      PtrActiveNpc,
		IntOperand:    int32(intOperand),
	}
	return s
}

func TestNpcFindHero_EmptyLedger(t *testing.T) {
	npc := &mockNpc{topContributor: 0}
	mw := &mockWorld{}
	s := newNpcFindHeroState(npc, mw, 0)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO empty: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("NPC_FINDHERO empty: pushed %d, want 0", got)
	}
	if s.Self != nil || s.Self2 != nil {
		t.Errorf("NPC_FINDHERO empty: Self/Self2 should remain nil")
	}
	if s.Pointers&(PtrActivePlayer|PtrActivePlayer2) != 0 {
		t.Errorf("NPC_FINDHERO empty: ActivePlayer pointer flags must not be set")
	}
}

func TestNpcFindHero_PrimarySlot(t *testing.T) {
	p := &mockPlayer{uidValue: 42}
	npc := &mockNpc{topContributor: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: p}}
	s := newNpcFindHeroState(npc, mw, 0)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO primary: err=%v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_FINDHERO primary: pushed %d, want 1", got)
	}
	if s.Self != p {
		t.Errorf("NPC_FINDHERO primary: Self=%v, want %v", s.Self, p)
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Errorf("NPC_FINDHERO primary: PtrActivePlayer flag must be set")
	}
}

func TestNpcFindHero_SecondarySlot(t *testing.T) {
	p := &mockPlayer{uidValue: 42}
	npc := &mockNpc{topContributor: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: p}}
	s := newNpcFindHeroState(npc, mw, 1)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO secondary: err=%v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_FINDHERO secondary: pushed %d, want 1", got)
	}
	if s.Self2 != p {
		t.Errorf("NPC_FINDHERO secondary: Self2=%v, want %v", s.Self2, p)
	}
	if s.Pointers&PtrActivePlayer2 == 0 {
		t.Errorf("NPC_FINDHERO secondary: PtrActivePlayer2 flag must be set")
	}
}

func TestNpcFindHero_LookupReturnsNil(t *testing.T) {
	npc := &mockNpc{topContributor: 99}
	mw := &mockWorld{} // empty playersByUID
	s := newNpcFindHeroState(npc, mw, 0)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO loggedout: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("NPC_FINDHERO loggedout: pushed %d, want 0", got)
	}
}

func TestNpcFindHero_RequiresActiveNpc(t *testing.T) {
	mw := &mockWorld{}
	s := &ScriptState{StackCapacity: 16, World: mw} // no PtrActiveNpc
	if err := handleNpcFindHero(s); err == nil {
		t.Fatalf("NPC_FINDHERO no-active-npc: want error, got nil")
	}
}
```

- [ ] **Step 2: Add 4 FINDHERO tests**

Append to `pkg/script/handlers_player_test.go`:

```go
// --- NAI-127 Bundle 1: FINDHERO (opcode 2018) ---

func newFindHeroState(self *mockPlayer, mw WorldVars, intOperand int) *ScriptState {
	return &ScriptState{
		StackCapacity: 16,
		World:         mw,
		Self:          self,
		Pointers:      PtrActivePlayer,
		IntOperand:    int32(intOperand),
	}
}

func TestFindHero_EmptyLedger(t *testing.T) {
	self := &mockPlayer{topContributor: 0}
	s := newFindHeroState(self, &mockWorld{}, 0)
	if err := handleFindHero(s); err != nil {
		t.Fatalf("FINDHERO empty: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("FINDHERO empty: pushed %d, want 0", got)
	}
	if s.Self2 != nil {
		t.Errorf("FINDHERO empty: Self2 must remain nil")
	}
}

// FINDHERO ALWAYS sets Self2 (secondary) regardless of IntOperand —
// pin TS asymmetry vs NPC_FINDHERO per ts_asymmetry_dual_pin.
func TestFindHero_PopulatedAlwaysSetsSelf2(t *testing.T) {
	other := &mockPlayer{uidValue: 7}
	self := &mockPlayer{topContributor: 7}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{7: other}}
	for _, op := range []int{0, 1} {
		s := newFindHeroState(self, mw, op)
		if err := handleFindHero(s); err != nil {
			t.Fatalf("FINDHERO op=%d: err=%v", op, err)
		}
		if got := s.PopInt(); got != 1 {
			t.Errorf("FINDHERO op=%d: pushed %d, want 1", op, got)
		}
		if s.Self2 != other {
			t.Errorf("FINDHERO op=%d: Self2=%v, want %v", op, s.Self2, other)
		}
		if s.Pointers&PtrActivePlayer2 == 0 {
			t.Errorf("FINDHERO op=%d: PtrActivePlayer2 must be set", op)
		}
	}
}

func TestFindHero_LookupReturnsNil(t *testing.T) {
	self := &mockPlayer{topContributor: 99}
	mw := &mockWorld{} // empty
	s := newFindHeroState(self, mw, 0)
	if err := handleFindHero(s); err != nil {
		t.Fatalf("FINDHERO loggedout: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("FINDHERO loggedout: pushed %d, want 0", got)
	}
}

func TestFindHero_RequiresActivePlayer(t *testing.T) {
	mw := &mockWorld{}
	s := &ScriptState{StackCapacity: 16, World: mw} // no PtrActivePlayer
	if err := handleFindHero(s); err == nil {
		t.Fatalf("FINDHERO no-active-player: want error, got nil")
	}
}
```

- [ ] **Step 3: Add 4 BOTH_HEROPOINTS tests**

Append to `pkg/script/handlers_player_test.go`:

```go
// --- NAI-127 Bundle 1: BOTH_HEROPOINTS (opcode 2003) ---

// newBothHeroPointsState builds a state with both Self and Self2 set,
// IntOperand and damage configured, and PtrActivePlayer set. Optional
// nilSelf2 lets tests pin the nil-slot error path.
func newBothHeroPointsState(self, other *mockPlayer, intOperand, damage int, nilSelf2 bool) *ScriptState {
	s := &ScriptState{
		StackCapacity: 16,
		Self:          self,
		Pointers:      PtrActivePlayer,
		IntOperand:    int32(intOperand),
	}
	if !nilSelf2 {
		s.Self2 = other
	}
	s.PushInt(damage)
	return s
}

func TestBothHeroPoints_PrimaryToSecondary(t *testing.T) {
	from := &mockPlayer{uidValue: 11}
	to := &mockPlayer{uidValue: 22}
	s := newBothHeroPointsState(from, to, 0, 5, false)
	if err := handleBothHeroPoints(s); err != nil {
		t.Fatalf("BOTH_HEROPOINTS primary: err=%v", err)
	}
	if got := len(to.addHeroPointsCalls); got != 1 {
		t.Fatalf("BOTH_HEROPOINTS primary: to.addHeroPointsCalls=%d, want 1", got)
	}
	if call := to.addHeroPointsCalls[0]; call.playerUID != 11 || call.amount != 5 {
		t.Errorf("BOTH_HEROPOINTS primary: call=%+v, want {11,5}", call)
	}
	if got := len(from.addHeroPointsCalls); got != 0 {
		t.Errorf("BOTH_HEROPOINTS primary: from.addHeroPointsCalls=%d, want 0", got)
	}
}

func TestBothHeroPoints_SecondaryToPrimary(t *testing.T) {
	primary := &mockPlayer{uidValue: 11}
	secondary := &mockPlayer{uidValue: 22}
	s := newBothHeroPointsState(primary, secondary, 1, 9, false)
	if err := handleBothHeroPoints(s); err != nil {
		t.Fatalf("BOTH_HEROPOINTS secondary: err=%v", err)
	}
	if got := len(primary.addHeroPointsCalls); got != 1 {
		t.Fatalf("BOTH_HEROPOINTS secondary: primary.addHeroPointsCalls=%d, want 1", got)
	}
	if call := primary.addHeroPointsCalls[0]; call.playerUID != 22 || call.amount != 9 {
		t.Errorf("BOTH_HEROPOINTS secondary: call=%+v, want {22,9}", call)
	}
}

func TestBothHeroPoints_NilSlot(t *testing.T) {
	from := &mockPlayer{uidValue: 11}
	s := newBothHeroPointsState(from, nil, 0, 5, true) // Self2 nil
	if err := handleBothHeroPoints(s); err == nil {
		t.Fatalf("BOTH_HEROPOINTS nilSelf2: want error, got nil")
	}
}

// Pin that handler still calls AddHeroPoints with amount=0; ledger
// no-ops downstream per HeroPoints.AddHero `if amount < 1 return`.
func TestBothHeroPoints_AmountZero(t *testing.T) {
	from := &mockPlayer{uidValue: 11}
	to := &mockPlayer{uidValue: 22}
	s := newBothHeroPointsState(from, to, 0, 0, false)
	if err := handleBothHeroPoints(s); err != nil {
		t.Fatalf("BOTH_HEROPOINTS zero: err=%v", err)
	}
	if got := len(to.addHeroPointsCalls); got != 1 {
		t.Errorf("BOTH_HEROPOINTS zero: to.addHeroPointsCalls=%d, want 1 (mock records before ledger no-ops)", got)
	}
	if call := to.addHeroPointsCalls[0]; call.amount != 0 {
		t.Errorf("BOTH_HEROPOINTS zero: call.amount=%d, want 0", call.amount)
	}
}
```

- [ ] **Step 4: Run RED — verify all 13 fail with "undefined" handler symbols**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestNpcFindHero|TestFindHero|TestBothHeroPoints' -v`
Expected: 13 failing tests (compile error: `undefined: handleNpcFindHero`, `undefined: handleFindHero`, `undefined: handleBothHeroPoints`).

- [ ] **Step 5: Commit RED**

```bash
git add pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "test(nai-127): T1.4 — RED tests for FINDHERO family (13 cases)

NPC_FINDHERO (5): empty ledger / primary slot / secondary slot /
loggedout / no-active-npc.
FINDHERO (4): empty / populated-always-Self2 (TS asymmetry per
ts_asymmetry_dual_pin) / loggedout / no-active-player.
BOTH_HEROPOINTS (4): primary→secondary / secondary→primary / nil-slot
error / amount=0 still records call."
```

---

### Task 1.5: GREEN — implement 3 handlers + 3 dispatch entries

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcFindHero`)
- Modify: `pkg/script/handlers_player.go` (add `handleFindHero`, `handleBothHeroPoints`)
- Modify: `pkg/script/handlers.go` (add 3 dispatch entries)

- [ ] **Step 1: Add `handleNpcFindHero`**

In `pkg/script/handlers_npc.go`, locate alphabetic position between `handleNpcFindAllZone` (~line 793) and `handleNpcHasOp` (~line 175 — actually NpcFind* sit later; pre-flight re-grep at HEAD before this step). Append the handler in a logical position near other NPC find-family handlers:

```go
// handleNpcFindHero (NPC_FINDHERO, opcode 2519) returns the player
// with the largest HeroPoints credit on this NPC's ledger and binds
// them to the primary or secondary active-player slot per IntOperand.
// Pushes 1 on success, 0 if the ledger is empty, the resolved player
// has logged out, or s.World is nil. Mirrors TS NpcOps.ts:114-130 —
// state.activePlayer setter behavior at ScriptState.ts:235-241
// routes to Self (primary) or Self2 (secondary) based on intOperand.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard (goscape defensive;
// TS skips this check). Mirrors handleNpcDel from NAI-126. Retire when
// an upstream invariant proves s.World non-nil for any executing
// script.
func handleNpcFindHero(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_FINDHERO"); err != nil {
		return err
	}
	if s.World == nil {
		s.PushInt(0)
		return nil
	}
	uid := s.ActiveNpc.TopContributor()
	if uid == 0 {
		s.PushInt(0)
		return nil
	}
	player := s.World.LookupPlayerByUID(uid)
	if player == nil {
		s.PushInt(0)
		return nil
	}
	if s.IntOperand == 0 {
		s.Self = player
		s.Pointers |= PtrActivePlayer
	} else {
		s.Self2 = player
		s.Pointers |= PtrActivePlayer2
	}
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 2: Add `handleFindHero` and `handleBothHeroPoints`**

In `pkg/script/handlers_player.go`, append (or place at a sensible alphabetic position — pre-flight re-grep):

```go
// handleFindHero (FINDHERO, opcode 2018) returns the player with the
// largest HeroPoints credit on the active player's ledger, binding
// them to the SECONDARY active-player slot regardless of IntOperand.
// Pushes 1 on success, 0 if the ledger is empty, the resolved player
// has logged out, or s.World is nil. Mirrors TS PlayerOps.ts:1138-1154.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard (goscape defensive;
// TS skips this check). Retire per the same condition as NPC_FINDHERO.
func handleFindHero(s *ScriptState) error {
	if err := requireActivePlayer(s, "FINDHERO"); err != nil {
		return err
	}
	if s.World == nil {
		s.PushInt(0)
		return nil
	}
	uid := s.Self.TopContributor()
	if uid == 0 {
		s.PushInt(0)
		return nil
	}
	player := s.World.LookupPlayerByUID(uid)
	if player == nil {
		s.PushInt(0)
		return nil
	}
	s.Self2 = player
	s.Pointers |= PtrActivePlayer2
	s.PushInt(1)
	return nil
}

// handleBothHeroPoints (BOTH_HEROPOINTS, opcode 2003) credits `damage`
// to the receiving player's HeroPoints ledger, attributed to the
// sending player's UID. IntOperand selects the swap direction:
//
//	IntOperand=0 → from=Self (primary),    to=Self2 (secondary)
//	IntOperand=1 → from=Self2 (secondary), to=Self (primary)
//
// Mirrors TS PlayerOps.ts:1156-1167. Returns an error if either slot
// is nil (TS throws).
func handleBothHeroPoints(s *ScriptState) error {
	if err := requireActivePlayer(s, "BOTH_HEROPOINTS"); err != nil {
		return err
	}
	damage := s.PopInt()
	secondary := s.IntOperand == 1
	var from, to ActivePlayer
	if secondary {
		from, to = s.Self2, s.Self
	} else {
		from, to = s.Self, s.Self2
	}
	if from == nil || to == nil {
		return fmt.Errorf("BOTH_HEROPOINTS: player is null")
	}
	to.AddHeroPoints(from.UID(), damage)
	return nil
}
```

(`fmt` is already imported at handlers_player.go:5; verify before commit.)

- [ ] **Step 3: Add 3 dispatch entries**

In `pkg/script/handlers.go`, insert each entry in alphabetic position within the existing dispatch map. Pre-flight re-grep `OpBoth`, `OpFind`, `OpNpcFind` at HEAD to find current neighbors. Suggested anchors:

- `OpBothHeroPoints: handleBothHeroPoints,` — near other `OpBoth*` entries (or insert into the existing player-misc cluster).
- `OpFindHero: handleFindHero,` — near `OpFindUID: handleFindUID,` (line ~453 per HEAD).
- `OpNpcFindHero: handleNpcFindHero,` — near the existing NPC find cluster (`OpNpcFindAllZone` at line ~441).

Implementer: re-grep at HEAD before placement to confirm exact line numbers; place each in the most natural alphabetic position. Avoid introducing new comment headers — match surrounding style.

- [ ] **Step 4: Run GREEN — all 13 pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestNpcFindHero|TestFindHero|TestBothHeroPoints' -v`
Expected: 13 PASS.

- [ ] **Step 5: Run full repo gates**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 6: Commit GREEN**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_player.go pkg/script/handlers.go
git commit --no-gpg-sign -m "feat(nai-127): T1.5 — handleNpcFindHero/handleFindHero/handleBothHeroPoints (GREEN)

3 handlers + 3 dispatch entries in pkg/script/handlers.go. NPC_FINDHERO
mirrors NpcOps.ts:114-130 (IntOperand-driven primary/secondary slot
flip). FINDHERO mirrors PlayerOps.ts:1138-1154 (always Self2 — TS
asymmetry pinned per ts_asymmetry_dual_pin). BOTH_HEROPOINTS mirrors
PlayerOps.ts:1156-1167.

DEVIATION-NAI-127-D1 declared in NPC_FINDHERO + FINDHERO doc-comments
(defensive nil-s.World guard mirroring NAI-126 handleNpcDel)."
```

---

### Bundle 1 — Sonnet code-reviewer

**Required between Bundle 1 close and Bundle 2 dispatch.** Per `superpowers_code_reviewer_model` — Sonnet, never Opus.

Dispatch a `feature-dev:code-reviewer` agent (or `superpowers:requesting-code-review`) on the 5-commit Bundle 1 diff. Reviewer checklist:

1. **TS fidelity:** Each handler line-by-line vs the verbatim TS in `docs/superpowers/specs/2026-05-08-nai-127-rat-ai-handler-subset-design.md` §3.1.
2. **Pointer flow:** NPC_FINDHERO IntOperand-driven Self/Self2 + flag; FINDHERO always Self2; BOTH_HEROPOINTS swap direction.
3. **Mock satisfaction:** All three implementers (`*Npc`, `mockNpc`, `mockActiveNpc`) satisfy `ActiveNpc.TopContributor`. All four (`*Player`, `mockPlayer`, plus any other `ActivePlayer` impls) satisfy `ActivePlayer.AddHeroPoints/TopContributor`. All three (`worldVarsView`, `mockWorld`, plus any sibling) satisfy `WorldVars.LookupPlayerByUID`.
4. **Deviation tagging:** DEVIATION-NAI-127-D1 doc-comment present + retire-condition stated.
5. **No YAGNI:** Did anything unrelated land?
6. **TS-asymmetry pin:** `TestFindHero_PopulatedAlwaysSetsSelf2` exercises both IntOperand=0 and 1.

Address any reviewer findings as a sub-commit before Bundle 2.

---

## Bundle 2 — rat-attacks-player primitives

Three opcodes (DAMAGE 2015, GENDER 2020, P_PREVENTLOGOUT 2084). Three implementer tasks (T2.1–T2.3) then Sonnet code-reviewer.

### Task 2.1: Add `ActivePlayer.SetPreventLogout`/`ApplyDamage` interface methods + `*Player` impls + mock recorders

**Files:**
- Modify: `pkg/script/active.go` (add 2 interface methods to `ActivePlayer`)
- Modify: `modules/world/player_script.go` (add `*Player.SetPreventLogout` + `*Player.ApplyDamage` impls)
- Modify: `pkg/script/runner_test.go` (extend `mockPlayer` with `applyDamageCalls`, `preventLogoutMessage`, `preventLogoutUntil` recorders + impl methods)

- [ ] **Step 1: Add interface methods**

In `pkg/script/active.go`, near the methods added in Bundle 1 (`AddHeroPoints`, `TopContributor`), append:

```go
	// SetPreventLogout records an anti-log message and absolute-tick
	// deadline. Used by P_PREVENTLOGOUT (PlayerOps.ts:626-630). The
	// caller computes `untilTick = currentTick + popped-ticks` —
	// matches TS `World.currentTick + check(popInt(), NumberNotNull)`.
	// NAI-127 Bundle 2.
	SetPreventLogout(message string, untilTick int)

	// ApplyDamage applies `amount` damage of `dmgType` to this player.
	// Used by DAMAGE (PlayerOps.ts:768-779). NAI-127 Bundle 2.
	ApplyDamage(amount, dmgType int)
```

- [ ] **Step 2: Add `*Player` impls**

In `modules/world/player_script.go`, append:

```go
// SetPreventLogout implements script.ActivePlayer. Mirrors TS
// PlayerOps.ts:628-629 (state.activePlayer.preventLogoutMessage =
// msg; state.activePlayer.preventLogoutUntil = currentTick + ticks).
// NAI-127 Bundle 2.
func (p *Player) SetPreventLogout(message string, untilTick int) {
	p.preventLogoutMessage = message
	p.preventLogoutUntil = untilTick
}

// ApplyDamage implements script.ActivePlayer. Delegates to
// Player.Damage (player_masks.go:126), which is the existing
// damage-mask producer. Mirrors TS player.applyDamage(amount, type)
// at PlayerOps.ts:778. NAI-127 Bundle 2.
func (p *Player) ApplyDamage(amount, dmgType int) {
	p.Damage(amount, dmgType)
}
```

- [ ] **Step 3: Extend `mockPlayer`**

In `pkg/script/runner_test.go`, in the `mockPlayer` struct, near the Bundle 1 additions (`topContributor`, `addHeroPointsCalls`), add:

```go
	// NAI-127 Bundle 2: DAMAGE recorder.
	applyDamageCalls []struct{ amount, dmgType int }

	// NAI-127 Bundle 2: P_PREVENTLOGOUT recorders.
	preventLogoutMessage string
	preventLogoutUntil   int
```

Then near the Bundle 1 method additions, add:

```go
func (m *mockPlayer) SetPreventLogout(message string, untilTick int) {
	m.preventLogoutMessage = message
	m.preventLogoutUntil = untilTick
}

func (m *mockPlayer) ApplyDamage(amount, dmgType int) {
	m.applyDamageCalls = append(m.applyDamageCalls, struct{ amount, dmgType int }{amount, dmgType})
}
```

- [ ] **Step 4: Verify build + existing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/active.go modules/world/player_script.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "feat(nai-127): T2.1 — ActivePlayer.SetPreventLogout + ApplyDamage + mock recorders

Adds the two interface setters consumed by P_PREVENTLOGOUT and DAMAGE
(Bundle 2). *Player.SetPreventLogout writes the existing
preventLogoutMessage/preventLogoutUntil fields (player.go:259-260).
*Player.ApplyDamage delegates to existing Player.Damage
(player_masks.go:126). mockPlayer records via applyDamageCalls slice +
preventLogoutMessage/Until fields."
```

---

### Task 2.2: RED — write 8 failing handler tests

**Files:**
- Modify: `pkg/script/handlers_player_test.go` (DAMAGE 3 + GENDER 2 + P_PREVENTLOGOUT 3)

- [ ] **Step 1: Add 3 DAMAGE tests**

Append to `pkg/script/handlers_player_test.go`:

```go
// --- NAI-127 Bundle 2: DAMAGE (opcode 2015) ---

// newDamageState builds a state with World set + push order matching
// the handler's pop order: amount, hitType, uid (the handler pops
// amount first, then hitType, then uid).
func newDamageState(mw WorldVars, uid, hitType, amount int) *ScriptState {
	s := &ScriptState{StackCapacity: 16, World: mw}
	s.PushInt(uid)
	s.PushInt(hitType)
	s.PushInt(amount)
	return s
}

func TestDamage_HappyPath(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	s := newDamageState(mw, 42, 1, 7)
	if err := handleDamage(s); err != nil {
		t.Fatalf("DAMAGE happy: err=%v", err)
	}
	if got := len(target.applyDamageCalls); got != 1 {
		t.Fatalf("DAMAGE happy: applyDamageCalls=%d, want 1", got)
	}
	if call := target.applyDamageCalls[0]; call.amount != 7 || call.dmgType != 1 {
		t.Errorf("DAMAGE happy: call=%+v, want {amount:7,dmgType:1}", call)
	}
}

func TestDamage_UnknownUID(t *testing.T) {
	mw := &mockWorld{} // empty playersByUID
	s := newDamageState(mw, 99, 1, 7)
	if err := handleDamage(s); err != nil {
		t.Fatalf("DAMAGE unknown: err=%v", err)
	}
	// no panic, no call recorded — silent no-op
}

// Pin TS quirk: DAMAGE uses raw `state =>` with no checkedHandler;
// goscape's handler must NOT call requireActivePlayer.
func TestDamage_NoPointerGate(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	s := newDamageState(mw, 42, 1, 5)
	// Pointers=0 — no PtrActivePlayer set.
	if err := handleDamage(s); err != nil {
		t.Fatalf("DAMAGE no-pointer: err=%v (pointer-gate must be absent)", err)
	}
	if got := len(target.applyDamageCalls); got != 1 {
		t.Errorf("DAMAGE no-pointer: applyDamageCalls=%d, want 1", got)
	}
}
```

- [ ] **Step 2: Add 2 GENDER tests**

Append:

```go
// --- NAI-127 Bundle 2: GENDER (opcode 2020) ---

// newGenderState builds a state with Self set; deliberately does NOT
// set PtrActivePlayer to pin TS quirk (no checkedHandler at
// PlayerOps.ts:968-970) per ts_asymmetry_dual_pin.
func newGenderState(self *mockPlayer) *ScriptState {
	return &ScriptState{StackCapacity: 16, Self: self}
}

func TestGender_Male(t *testing.T) {
	self := &mockPlayer{genderValue: 0}
	s := newGenderState(self)
	if err := handleGender(s); err != nil {
		t.Fatalf("GENDER male: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("GENDER male: pushed %d, want 0", got)
	}
}

func TestGender_Female(t *testing.T) {
	self := &mockPlayer{genderValue: 1}
	s := newGenderState(self)
	if err := handleGender(s); err != nil {
		t.Fatalf("GENDER female: err=%v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("GENDER female: pushed %d, want 1", got)
	}
}
```

- [ ] **Step 3: Add 3 P_PREVENTLOGOUT tests**

Append:

```go
// --- NAI-127 Bundle 2: P_PREVENTLOGOUT (opcode 2084) ---

// newPreventLogoutState builds a state with Self + protected flag +
// ticks/msg pre-pushed. Push order matches handler pop order: msg
// pushed FIRST (popped last) and ticks pushed LAST (popped first), so
// PopInt returns ticks and PopString returns msg.
func newPreventLogoutState(self *mockPlayer, mw WorldVars, msg string, ticks int, protect bool) *ScriptState {
	s := &ScriptState{
		StackCapacity: 16,
		World:         mw,
		Self:          self,
		Pointers:      PtrActivePlayer,
		Protect:       protect,
	}
	s.PushString(msg)
	s.PushInt(ticks)
	return s
}

func TestPPreventLogout_HappyPath(t *testing.T) {
	self := &mockPlayer{}
	mw := &mockWorld{tick: 100}
	s := newPreventLogoutState(self, mw, "Combat", 16, true)
	if err := handlePPreventLogout(s); err != nil {
		t.Fatalf("P_PREVENTLOGOUT happy: err=%v", err)
	}
	if self.preventLogoutMessage != "Combat" {
		t.Errorf("P_PREVENTLOGOUT happy: msg=%q, want %q", self.preventLogoutMessage, "Combat")
	}
	if self.preventLogoutUntil != 116 {
		t.Errorf("P_PREVENTLOGOUT happy: until=%d, want 116", self.preventLogoutUntil)
	}
}

func TestPPreventLogout_RequiresProtected(t *testing.T) {
	self := &mockPlayer{}
	mw := &mockWorld{tick: 100}
	s := newPreventLogoutState(self, mw, "Combat", 16, false) // not protected
	if err := handlePPreventLogout(s); err == nil {
		t.Fatalf("P_PREVENTLOGOUT not-protected: want error, got nil")
	}
}

func TestPPreventLogout_NoActivePlayer(t *testing.T) {
	mw := &mockWorld{tick: 100}
	s := &ScriptState{StackCapacity: 16, World: mw} // no PtrActivePlayer
	s.PushString("Combat")
	s.PushInt(16)
	if err := handlePPreventLogout(s); err == nil {
		t.Fatalf("P_PREVENTLOGOUT no-active-player: want error, got nil")
	}
}
```

- [ ] **Step 4: Run RED — verify all 8 fail with "undefined" symbols**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestDamage|TestGender|TestPPreventLogout' -v`
Expected: 8 failing (compile error: `undefined: handleDamage`, `handleGender`, `handlePPreventLogout`).

- [ ] **Step 5: Commit RED**

```bash
git add pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "test(nai-127): T2.2 — RED tests for DAMAGE/GENDER/P_PREVENTLOGOUT (8 cases)

DAMAGE (3): happy path / unknown UID silent no-op / no-pointer-gate
(pin TS quirk per ts_asymmetry_dual_pin — handler MUST NOT call
requireActivePlayer).
GENDER (2): male / female (also pin no-pointer-gate by omitting
PtrActivePlayer in fixture).
P_PREVENTLOGOUT (3): happy / not-protected error / no-active-player
error."
```

---

### Task 2.3: GREEN — implement 3 handlers + 3 dispatch entries

**Files:**
- Modify: `pkg/script/handlers_player.go` (add 3 handlers)
- Modify: `pkg/script/handlers.go` (add 3 dispatch entries)

- [ ] **Step 1: Add 3 handlers**

In `pkg/script/handlers_player.go`, append (or place at sensible alphabetic positions; pre-flight re-grep before placing):

```go
// handleDamage (DAMAGE, opcode 2015) applies damage to the player
// resolved from a UID popped from the stack. Pop order (TS): amount,
// hitType, uid (LIFO via popInt). Silent no-op if the UID does not
// resolve to a logged-in player. Mirrors TS PlayerOps.ts:768-779.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard. Without s.World
// there is no way to resolve the UID.
//
// DEVIATION-NAI-127-D2: no PtrActivePlayer gate — TS uses raw
// `state =>`, not checkedHandler. Pinned by TestDamage_NoPointerGate
// per ts_asymmetry_dual_pin.
func handleDamage(s *ScriptState) error {
	amount := s.PopInt()
	hitType := s.PopInt()
	uid := s.PopInt()
	if s.World == nil {
		return nil
	}
	player := s.World.LookupPlayerByUID(uid)
	if player == nil {
		return nil
	}
	player.ApplyDamage(amount, hitType)
	return nil
}

// handleGender (GENDER, opcode 2020) pushes the active player's
// gender (0=male, 1=female). Mirrors TS PlayerOps.ts:968-970.
//
// DEVIATION-NAI-127-D2: TS uses raw `state =>` — there is no pointer
// gate (no requireActivePlayer). state.activePlayer access is
// nil-unsafe. Goscape preserves this quirk per ts_asymmetry_dual_pin.
// Pinned by TestGender_Male/Female (no PtrActivePlayer in fixture).
// Retire only if upstream TS adds a checkedHandler wrapping.
func handleGender(s *ScriptState) error {
	s.PushInt(s.Self.Gender())
	return nil
}

// handlePPreventLogout (P_PREVENTLOGOUT, opcode 2084) sets the
// player's anti-log message and absolute tick deadline. Pop order
// (TS): popString first (message), then popInt (additional ticks
// from current tick). Mirrors TS PlayerOps.ts:626-630.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard (currentTick read
// requires World).
func handlePPreventLogout(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_PREVENTLOGOUT"); err != nil {
		return err
	}
	if s.World == nil {
		return nil
	}
	ticks := s.PopInt()
	msg := s.PopString()
	s.Self.SetPreventLogout(msg, s.World.CurrentTick()+ticks)
	return nil
}
```

- [ ] **Step 2: Add 3 dispatch entries**

In `pkg/script/handlers.go`, insert each entry at its alphabetic position. Pre-flight re-grep for current neighbors:

- `OpDamage: handleDamage,` — near `OpFindHero` placement from Bundle 1.
- `OpGender: handleGender,` — near `OpHeadIconsGet` etc. (re-grep).
- `OpPPreventLogout: handlePPreventLogout,` — near other `OpP*` protected ops (e.g., `OpPDelay`).

Match surrounding placement style; do not add new section headers.

- [ ] **Step 3: Run GREEN — all 8 Bundle 2 + 13 Bundle 1 pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestNpcFindHero|TestFindHero|TestBothHeroPoints|TestDamage|TestGender|TestPPreventLogout' -v`
Expected: 21 PASS.

- [ ] **Step 4: Run full repo gates**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 5: Verify unhandled-opcode count drops 62→56**

Run:

```bash
mkdir -p /tmp/claude
awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/claude/handled.txt
awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/claude/declared.txt
comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l
```

Expected: `56` (was 62; 6 newly handled).

Also verify the 6 specifically:

```bash
comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | grep -E 'OpBothHeroPoints|OpDamage|OpFindHero|OpGender|OpNpcFindHero|OpPPreventLogout'
```

Expected: empty (no output).

- [ ] **Step 6: Commit GREEN**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go
git commit --no-gpg-sign -m "feat(nai-127): T2.3 — handleDamage/handleGender/handlePPreventLogout (GREEN)

3 handlers + 3 dispatch entries. DAMAGE mirrors PlayerOps.ts:768-779
(no pointer gate — TS quirk pinned per ts_asymmetry_dual_pin). GENDER
mirrors PlayerOps.ts:968-970 (likewise nil-unsafe per TS). P_PREVENT
LOGOUT mirrors PlayerOps.ts:626-630 (ProtectedActivePlayer gate).

Unhandled-opcode audit drops 62 → 56 (6 newly dispatched).

DEVIATION-NAI-127-D1 + D2 declared in doc-comments."
```

---

### Bundle 2 — Sonnet code-reviewer

Sonnet, never Opus. Dispatch on the 3-commit Bundle 2 diff. Reviewer checklist:

1. **TS fidelity:** Each handler line-by-line vs verbatim TS in spec §4.1.
2. **Pop order:** DAMAGE — amount → hitType → uid; P_PREVENTLOGOUT — ticks (int) → msg (string).
3. **Pointer-gate absence pin:** GENDER and DAMAGE doc-comments label DEVIATION-NAI-127-D2 explicitly; tests pin the absence.
4. **Mock satisfaction:** `*Player`, `mockPlayer`, plus any other `ActivePlayer` impl satisfy `SetPreventLogout` and `ApplyDamage`.
5. **No YAGNI:** No drive-by edits unrelated to the 3 opcodes.
6. **Unhandled-count drop:** Step 5 in T2.3 produced `56` and the 6 in-scope opcodes are absent from the diff.

Address findings as a sub-commit before close commit.

---

## Smoke handoff (post-T2.3 reviewer-fix)

User-launched Java-client smoke per `smoke_test_server_handoff` and `java_client_coord_chat_suppression`. Controller emits the resume prompt at the end of the implementer flow; user runs the server + client.

**Smoke setup:**
- Fresh char (no skills trained, default inventory empty + bronze dagger equipped on right hand).
- Tutorial Island giant rat, attack from coord box outside chat-suppression zone.
- Server log tail running.

**Smoke matrix:** see spec §5.

**Close conditions:** see spec §5 decision tree. Items 1+2 PASS = PRIMARY met → controller emits close commit (template below).

**Close commit (after smoke confirms PRIMARY):**

```
chore(close): NAI-127 — FINDHERO family + rat-AI primitives ported; smoke confirmed PRIMARY met

PRIMARY (smoke-bound, Bundle 1 + Bundle 2): script-side NPC_FINDHERO
(2519), FINDHERO (2018), BOTH_HEROPOINTS (2003), DAMAGE (2015),
GENDER (2020), P_PREVENTLOGOUT (2084) ported. Tutorial-Island fresh
char + bronze dagger vs giant rat smoke (2026-05-08): rat dies, the
NAI-126 cascade-tail "no handler for opcode 2519" WARN is silenced,
death-loot drops on the ground.

Bundle 1 (FINDHERO family): 5 implementer tasks. Adds Player.heroPoints
ledger (parallel to Npc.heroPoints from NAI-120). Three new ActivePlayer
interface methods (AddHeroPoints, TopContributor) + ActiveNpc method
(TopContributor) + WorldVars.LookupPlayerByUID adapter.

Bundle 2 (rat-attacks-player primitives): 3 implementer tasks. Two new
ActivePlayer methods (SetPreventLogout, ApplyDamage); 3 handlers piggy
back on Bundle 1 plumbing.

DEVIATION-NAI-127-D1: defensive nil-World guards in NPC_FINDHERO,
FINDHERO, DAMAGE, P_PREVENTLOGOUT (goscape-defensive; TS skips). Mirrors
NAI-126 handleNpcDel.

DEVIATION-NAI-127-D2: GENDER + DAMAGE handlers intentionally omit
requireActivePlayer to preserve TS quirk (raw `state =>`). Pinned per
ts_asymmetry_dual_pin.

Smoke also confirmed [items 3-5 outcomes per smoke matrix].

Unhandled-opcode count drops 62 → 56.

Retires NAI-40-SB2 carry-forward (FINDHERO + BOTH_HEROPOINTS + hash64
infra); 8 grep hits in nai_followups.md to update.

Closes memory:
- (any new entries added during this sub-spec)
```

---

## Self-Review

**Spec coverage:**

- §1 / §2 (audit + adjacency) — Plan tasks T1.1–T1.3, T2.1 add the missing surfaces. ✓
- §3.2 / §4.2 (surface tables) — Each row covered by at least one task step. ✓
- §3.3 / §4.3 (handler bodies) — T1.5 + T2.3 contain verbatim handler bodies. ✓
- §3.4 / §4.4 (test cases) — T1.4 + T2.2 contain all 21 tests verbatim. ✓
- §3.5 / §4.5 (task plans) — Mapped 1:1 to plan T1.1–T1.5 and T2.1–T2.3. ✓
- §5 (smoke matrix) — Smoke handoff section references spec §5 directly. ✓
- §6 (deviations) — D1 + D2 doc-comments embedded in handler bodies. ✓
- §7 (out of scope / carry-forward) — Plan does not introduce any out-of-scope work. ✓
- §8 (pattern memories) — Applied throughout (controller_preflight, ts_asymmetry_dual_pin, defensive_gate_doc_comment_label, mock_recorder_field_naming_check, scriptstate_test_fixture_idioms, plan_runnable_test_fixtures, etc.).

**Placeholder scan:** clean. Every step has the actual content.

**Type consistency:**
- `ActiveNpc.TopContributor() int` — declared T1.1, consumed T1.5 (handleNpcFindHero), mocked T1.1.
- `ActivePlayer.AddHeroPoints(playerUID, amount int)` — declared T1.2, consumed T1.5 (handleBothHeroPoints), mocked T1.2.
- `ActivePlayer.TopContributor() int` — declared T1.2, consumed T1.5 (handleFindHero), mocked T1.2.
- `WorldVars.LookupPlayerByUID(uid int) ActivePlayer` — declared T1.3, consumed T1.5 (3 handlers) + T2.3 (handleDamage), mocked T1.3.
- `ActivePlayer.SetPreventLogout(message string, untilTick int)` — declared T2.1, consumed T2.3 (handlePPreventLogout), mocked T2.1.
- `ActivePlayer.ApplyDamage(amount, dmgType int)` — declared T2.1, consumed T2.3 (handleDamage), mocked T2.1.
- Mock recorder field shapes match between declaration (T1.2/T2.1) and assertion (T1.4/T2.2): `addHeroPointsCalls []struct{ playerUID, amount int }`, `applyDamageCalls []struct{ amount, dmgType int }`, `preventLogoutMessage string`, `preventLogoutUntil int`, `topContributor int`.

All consistent.

**Plan-runnable test fixtures (per `plan_runnable_test_fixtures` mental-execute):**
- `newNpcFindHeroState` — sets `World`, `ActiveNpc`, `Pointers=PtrActiveNpc`, `IntOperand`. ✓
- `newFindHeroState` — sets `World`, `Self`, `Pointers=PtrActivePlayer`, `IntOperand`. ✓
- `newBothHeroPointsState` — sets `Self`, `Pointers=PtrActivePlayer`, optionally `Self2`, `IntOperand`, push-pre `damage`. PopInt in handler returns last-pushed value. ✓
- `newDamageState` — push order (uid, hitType, amount); PopInt LIFO returns amount, hitType, uid in that order. ✓
- `newGenderState` — sets `Self` only, Pointers=0 to pin no-gate quirk. ✓
- `newPreventLogoutState` — push order (msg, ticks); PopInt returns ticks; PopString returns msg. ✓
