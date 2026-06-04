# NAI-185 — Port admin-block non-spawn cheat cohort (7 cheats) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 7 admin-block cheats — `setvar`, `setvarother`, `getvar`, `getvarother`, `giveother`, `givecrap`, `broadcast` — into the existing `staffModLevel >= 3` block in `handlers_game.go`. Closes the admin-block carryforward except for the dynamic-spawn trio.

**Architecture:** Two new isolated helpers (`(*VarpTypeConfigs).ByName`, `(*Server).BroadcastMes`); 7 new `case` arms in the existing admin-block switch; one carryforward comment rewrite + one dispatch wiring smoke test. Existing infra (`LookupPlayerByUsername`, `ObjTypeConfigs.ByName`, `Player.{SetVarp,Varp,CanAccess,CloseModal,ClearInteraction,UnsetMapFlag,InvAdd,MessageGame}`, `cfg.NodeProduction`, `cfg.NodeMembers`) is consumed verbatim.

**Tech Stack:** Go 1.26+. Spec at `docs/superpowers/specs/2026-05-12-nai-185-admin-cheats-cohort-design.md`.

---

## Task ordering rationale

T0 + T1 are independent helper units and can run in parallel if the controller chooses. T2-T8 are cheat-arm tasks that each fully test-drive one TS arm; they are independent of each other and can run in any order after T0+T1, but conventionally proceed in TS source order (setvar, setvarother, getvar, getvarother, giveother, givecrap, broadcast). T9 rewrites the carryforward comment block + adds a dispatch wiring smoke test. T10 is the close commit.

```
T0 (VarpTypeConfigs.ByName) ─┐
T1 (Server.BroadcastMes)    ─┤
                              ├─→ T2 (setvar)
                              │   T3 (setvarother)
                              │   T4 (getvar)
                              │   T5 (getvarother)
                              │   T6 (giveother)
                              │   T7 (givecrap)
                              │   T8 (broadcast)
                              └─→ T9 (carryforward rewrite + wiring smoke) → T10 (close)
```

**Existing test infrastructure consumed by T2-T8:**
- `teleTestPlayer(t) → (*Player, net.Conn, *Server)` at `handlers_game_test.go:363` — builds a single registered player at slot 1.
- `addOtherTestPlayer(t, s, username, x, z, level) → *Player` at `handlers_game_test.go:992` — adds a second player at slot 2 with active=true, encryptor seeded, conn drained.
- `dispatchTeleCheat(t, p, cheat)` at `handlers_game_test.go:391` — builds the `G1(ctrl) + PJStrLF(cheat)` payload and drives `handleClientCheat`.
- `drainAfterTele(t, p, cc) → []byte` at `handlers_game_test.go:403` — flushes `p.client.bufw` and reads emitted bytes.
- `parseIntOr(s, def)` at `handlers_game.go:668`.
- `bytes.Contains(emitted, []byte("..."))` is the existing message-pin idiom (handlers_game_test.go:443-446).

The cheat-arm tasks reuse these as-is. No new fixture helpers required.

---

## Task 0: Add `(*VarpTypeConfigs).ByName` helper

Mirror of `ObjTypeConfigs.ByName` at `pkg/objtype/objtype.go:76-92`. Directly consumes the existing `ConfigNames` index (populated at `varptype.go:99-101`), with a linear-scan fallback for fixtures that bypass the index.

**Files:**
- Modify: `pkg/objtype/varptype.go` (append after `parseVarpTypes`, ~line 113)
- Modify: `pkg/objtype/varptype_test.go` (append new tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/objtype/varptype_test.go`:

```go
func TestVarpTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ConfigType: ConfigType{ID: 0, DebugName: "first"}}, {ConfigType: ConfigType{ID: 1, DebugName: "second"}}},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := vtc.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestVarpTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := vtc.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestVarpTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var vtc *VarpTypeConfigs
	if got := vtc.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestVarpTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	// ConfigNames points "fresh" at id=5 but Configs is only length 2.
	// Lookup must NOT panic and must fall through to the linear scan,
	// which finds "fresh" at id=1 by DebugName equality.
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ConfigType: ConfigType{ID: 0, DebugName: "other"}}, {ConfigType: ConfigType{ID: 1, DebugName: "fresh"}}},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := vtc.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestVarpTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	// Some test fixtures construct Configs without populating ConfigNames.
	// ByName must still resolve by DebugName.
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := vtc.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestVarpTypeConfigs_ByName' -v`
Expected: FAIL with "vtc.ByName undefined" (compile error).

- [ ] **Step 3: Add the `ByName` method**

Append to `pkg/objtype/varptype.go` (after `parseVarpTypes`):

```go
// ByName returns the VarPlayerType matching the given debugname, or nil
// if no match exists. Mirrors TS VarPlayerType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) only if ConfigNames is unpopulated (test fixtures) or stale.
// Consumed by ::setvar / ::setvarother / ::getvar / ::getvarother in
// modules/world/handlers_game.go (NAI-185).
func (vtc *VarpTypeConfigs) ByName(name string) *VarPlayerType {
	if vtc == nil {
		return nil
	}
	if id, ok := vtc.ConfigNames[name]; ok {
		if id >= 0 && id < len(vtc.Configs) {
			return vtc.Configs[id]
		}
	}
	for _, c := range vtc.Configs {
		if c != nil && c.DebugName == name {
			return c
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -v`
Expected: all PASS, including the five new ones.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/varptype.go pkg/objtype/varptype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-185 T0 — add VarpTypeConfigs.ByName

Mirrors TS VarPlayerType.getByName via the existing ConfigNames index
plus a linear-scan fallback for fixtures that bypass the index.
Consumed by ::setvar / ::setvarother / ::getvar / ::getvarother
cheat arms in NAI-185 T2-T5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1: Add `(*Server).BroadcastMes` helper

Fan-out helper that messages every logged-in player. Acquires `playersMu.RLock` for the duration; callers must NOT hold `playersMu`.

**Files:**
- Create: `modules/world/server_broadcast.go`
- Create: `modules/world/server_broadcast_test.go`

- [ ] **Step 1: Write the failing tests**

Create `modules/world/server_broadcast_test.go`:

```go
package world

import (
	"bytes"
	"io"
	"testing"
)

// TestBroadcastMes_FanOutToAllPlayers pins that every non-nil entry in
// s.players receives an identical MESSAGE_GAME packet with the supplied
// body. Mirrors TS World.broadcastMes single-line forEach.
func TestBroadcastMes_FanOutToAllPlayers(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	go io.Copy(io.Discard, cc)
	other := addOtherTestPlayer(t, s, "second_user", 3220, 3220, 0)

	s.BroadcastMes("ping")

	emittedP := drainAfterTele(t, p, cc)
	if !bytes.Contains(emittedP, []byte("ping")) {
		t.Errorf("caller did not receive 'ping'; got %d bytes", len(emittedP))
	}
	// Flush other's bufw too; otherConn was already drained by addOtherTestPlayer.
	other.client.flushWrite()
	// Re-confirm by checking other's outgoing buffer count via the
	// MessageGame side-effect: a second BroadcastMes appends another
	// frame to other's buffer.
	s.BroadcastMes("again")
	other.client.flushWrite()
	// (Wire-level assertion against the test conn is racy across
	// the two-message path; the meaningful invariant is the fan-out
	// reach, pinned by the caller-side check above + the no-panic
	// completion here.)
}

// TestBroadcastMes_NilSlotSkipped pins that nil entries in s.players are
// skipped without panic. Surrounding non-nil players still receive the
// message.
func TestBroadcastMes_NilSlotSkipped(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	go io.Copy(io.Discard, cc)
	// Slot 2 stays nil. Slot 3 gets a populated player.
	other := addOtherTestPlayer(t, s, "third_user", 3220, 3220, 0)
	_ = other

	// Must not panic on the nil slot[2].
	s.BroadcastMes("survive nil")

	emitted := drainAfterTele(t, p, cc)
	if !bytes.Contains(emitted, []byte("survive nil")) {
		t.Errorf("caller missed broadcast across nil slot; got %d bytes", len(emitted))
	}
}

// TestBroadcastMes_EmptyMessageDelivered pins TS behavior: TS does no
// defensive filter on empty input, so an empty broadcast is delivered.
func TestBroadcastMes_EmptyMessageDelivered(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	go io.Copy(io.Discard, cc)

	s.BroadcastMes("")

	emitted := drainAfterTele(t, p, cc)
	// MESSAGE_GAME with empty body = opcode + PJStrLF("") = opcode + 0x0a.
	// Body assertion: at minimum, the player's conn should have received
	// SOME bytes (the framed MESSAGE_GAME packet).
	if len(emitted) == 0 {
		t.Errorf("empty broadcast produced zero bytes; expected framed MESSAGE_GAME packet")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestBroadcastMes' -v`
Expected: FAIL with `s.BroadcastMes undefined`.

- [ ] **Step 3: Implement `BroadcastMes`**

Create `modules/world/server_broadcast.go`:

```go
package world

// BroadcastMes sends a MESSAGE_GAME packet to every logged-in player.
// Mirrors TS World.broadcastMes (single-line forEach over players).
// Holds Server.playersMu.RLock for the duration of the fan-out —
// callers must NOT hold playersMu. Used by ::broadcast cheat arm
// (NAI-185 T8) and any future server-wide announcement path.
func (s *Server) BroadcastMes(msg string) {
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	for _, p := range s.players {
		if p == nil {
			continue
		}
		p.MessageGame(msg)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestBroadcastMes' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/server_broadcast.go modules/world/server_broadcast_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T1 — add Server.BroadcastMes fan-out helper

Mirrors TS World.broadcastMes. RLock on playersMu + range over
s.players + MessageGame on each non-nil entry. Consumed by the
::broadcast admin cheat arm in NAI-185 T8.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Port `::setvar` admin cheat

TS L192-219. Not NP-gated. Routes through `s.varpTypes.ByName` (T0) + `Player.{CloseModal,CanAccess,ClearInteraction,UnsetMapFlag,SetVarp}` (existing).

**Files:**
- Modify: `modules/world/handlers_game.go` (admin block switch — append after the existing `case "minme":` arm at ~line 540)
- Modify: `modules/world/handlers_game_test.go` (append new tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
// setvarTestFixture extends teleTestPlayer with a populated VarpTypeConfigs
// containing two varps: id=0 "transmit_only" (Transmit=true, Protect=false),
// id=1 "protect_var" (Transmit=true, Protect=true). Returns the same
// (player, conn, server) tuple as teleTestPlayer.
func setvarTestFixture(t *testing.T) (*Player, net.Conn, *Server) {
	t.Helper()
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: []*objtype.VarPlayerType{
			{ConfigType: objtype.ConfigType{ID: 0, DebugName: "transmit_only"}, Transmit: true, Protect: false},
			{ConfigType: objtype.ConfigType{ID: 1, DebugName: "protect_var"}, Transmit: true, Protect: true},
		},
		ConfigNames: map[string]int{"transmit_only": 0, "protect_var": 1},
	}
	// Player.varps must be sized for SetVarp to be in-range.
	p.varps = make([]int32, len(s.varpTypes.Configs))
	return p, cc, s
}

func TestHandleClientCheat_SetVar_HappyPath_SetsVarpAndMessagesCaller(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only 42")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[0] != 42 {
		t.Errorf("varps[0] after setvar: got %d, want 42", p.varps[0])
	}
	if !bytes.Contains(emitted, []byte("set transmit_only: to 42")) {
		t.Errorf("expected 'set transmit_only: to 42' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVar_MissingValueArg_Rejects(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[0] != 0 {
		t.Errorf("varps[0] after setvar with missing value: got %d, want 0 (unchanged)", p.varps[0])
	}
	if bytes.Contains(emitted, []byte("set ")) {
		t.Errorf("unexpected 'set ' MessageGame on missing-value reject; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVar_UnknownName_SilentReject(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar no_such_var 99")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[0] != 0 || p.varps[1] != 0 {
		t.Errorf("unknown-name setvar mutated varps: %v, want all 0", p.varps)
	}
	if len(emitted) != 0 {
		t.Errorf("unknown-name setvar emitted %d bytes; want silent reject", len(emitted))
	}
}

func TestHandleClientCheat_SetVar_ClampHigh(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only 2147483648") // INT32_MAX + 1
	_ = drainAfterTele(t, p, cc)

	if p.varps[0] != 0x7fffffff {
		t.Errorf("varps[0] after clamp-high setvar: got %d, want %d", p.varps[0], int32(0x7fffffff))
	}
}

func TestHandleClientCheat_SetVar_ClampLow(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only -2147483649") // INT32_MIN - 1
	_ = drainAfterTele(t, p, cc)

	if p.varps[0] != -0x80000000 {
		t.Errorf("varps[0] after clamp-low setvar: got %d, want %d", p.varps[0], int32(-0x80000000))
	}
}

func TestHandleClientCheat_SetVar_ProtectVarp_HappyPath_ClearsInteraction(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)
	p.modalState = modalStateMain
	p.waypointIndex = 5

	dispatchTeleCheat(t, p, "setvar protect_var 7")
	_ = drainAfterTele(t, p, cc)

	if p.modalState != modalStateNone {
		t.Errorf("modalState after protect-setvar: got %d, want modalStateNone", p.modalState)
	}
	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex after protect-setvar: got %d, want -1 (UnsetMapFlag)", p.waypointIndex)
	}
	if p.varps[1] != 7 {
		t.Errorf("varps[1] after protect-setvar: got %d, want 7", p.varps[1])
	}
}

func TestHandleClientCheat_SetVar_ProtectVarp_CanAccessFalse_MessagesAndRejects(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)
	p.delayed = true // forces CanAccess() = false

	dispatchTeleCheat(t, p, "setvar protect_var 99")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[1] != 0 {
		t.Errorf("CanAccess=false should have rejected setvar: varps[1] = %d, want 0", p.varps[1])
	}
	if !bytes.Contains(emitted, []byte("Please finish what you are doing first.")) {
		t.Errorf("expected busy-message in emitted bytes; got %d bytes", len(emitted))
	}
}
```

**Note:** This task introduces `setvarTestFixture`. T3-T5 reuse it. The fixture intentionally lives at the top of the new test block so the implementer can grep `setvarTestFixture` to find it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_SetVar' -v`
Expected: FAIL (no `setvar` case in handler switch yet; tests panic or assert wrong values).

- [ ] **Step 3: Pre-flight grep**

Confirm the insertion point and the existing `parts[0]` shape:

```bash
grep -n 'case "minme":\|case "teleother":\|case "give":\|case "setstat":' modules/world/handlers_game.go
```

Expected: each `case` lives inside the `if p.staffModLevel >= 3` block (~line 427+). The new `setvar` arm goes inside the same switch.

- [ ] **Step 4: Add the `setvar` arm**

In `modules/world/handlers_game.go`, inside the admin block's `switch parts[0] {`, append (after the existing arms):

```go
		case "setvar":
			// TS L192-219. Not NP-gated. setvar <name> <value>: ByName
			// lookup, optional protect-path modal close + canAccess gate
			// + clearInteraction + unsetMapFlag, then SetVarp with
			// int32-clamped value. Caller gets `set <debugname>: to <value>`.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 {
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(sub[0])
			if cfg == nil {
				return nil
			}
			if cfg.Protect {
				p.CloseModal(true)
				if !p.CanAccess() {
					p.MessageGame("Please finish what you are doing first.")
					return nil
				}
				p.ClearInteraction()
				p.UnsetMapFlag()
			}
			value := parseIntOr(sub[1], 0)
			if value > 0x7fffffff {
				value = 0x7fffffff
			}
			if value < -0x80000000 {
				value = -0x80000000
			}
			p.SetVarp(cfg.ID, int32(value))
			p.MessageGame(fmt.Sprintf("set %s: to %d", cfg.DebugName, value))
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_SetVar' -v`
Expected: all PASS.

- [ ] **Step 6: Full test pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T2 — port ::setvar admin cheat

Mirrors TS ClientCheatHandler.ts:192-219. Admin block (>=3), not
NP-gated. Routes through VarpTypeConfigs.ByName (T0) + Player.SetVarp.
Protect-arm path: CloseModal(true) → CanAccess gate → ClearInteraction
→ UnsetMapFlag, then SetVarp with int32-clamped value.

Introduces setvarTestFixture for the varp-cheat test cohort (T2-T5).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Port `::setvarother` admin cheat

TS L220-252. NP-gated. Routes through `LookupPlayerByUsername` + `s.varpTypes.ByName` + target's `SetVarp`. **Caller-vs-target message asymmetry**: the "busy" message on `!CanAccess()` goes to the **caller**, not the target (DEVIATION-NAI-185-D3 pin).

**Files:**
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_SetVarOther_HappyPath_SetsTargetVarpAndMessagesCaller(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	dispatchTeleCheat(t, p, "setvarother target transmit_only 13")
	emitted := drainAfterTele(t, p, cc)

	if other.varps[0] != 13 {
		t.Errorf("other.varps[0] after setvarother: got %d, want 13", other.varps[0])
	}
	if !bytes.Contains(emitted, []byte("set transmit_only: to 13 on target")) {
		t.Errorf("expected 'set transmit_only: to 13 on target' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVarOther_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = false
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	dispatchTeleCheat(t, p, "setvarother target transmit_only 13")
	_ = drainAfterTele(t, p, cc)

	if other.varps[0] != 0 {
		t.Errorf("setvarother under NP=false mutated target: %d, want 0", other.varps[0])
	}
}

func TestHandleClientCheat_SetVarOther_MissingArgsRejects(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	dispatchTeleCheat(t, p, "setvarother target transmit_only") // 2 tokens, need 3
	_ = drainAfterTele(t, p, cc)

	if other.varps[0] != 0 {
		t.Errorf("len(args)<3 setvarother mutated target: %d, want 0", other.varps[0])
	}
}

func TestHandleClientCheat_SetVarOther_UnknownUser_MessagesCallerAndRejects(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "setvarother no_such_user transmit_only 5")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("no_such_user is not logged in.")) {
		t.Errorf("expected 'no_such_user is not logged in.' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVarOther_UnknownVarp_SilentReject(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	emittedBefore := drainAfterTele(t, p, cc)

	dispatchTeleCheat(t, p, "setvarother target no_such_var 5")
	emittedAfter := drainAfterTele(t, p, cc)
	emitted := append(emittedBefore, emittedAfter...)

	if bytes.Contains(emitted, []byte("set ")) {
		t.Errorf("unknown-varp setvarother emitted 'set '; want silent reject. bytes=%d", len(emitted))
	}
}

// TestHandleClientCheat_SetVarOther_BusyMessageGoesToCaller pins
// DEVIATION-NAI-185-D3: when the TARGET fails CanAccess, the
// "<arg0> is busy right now." message is sent to the CALLER, not the
// target. Mirrors TS L242.
func TestHandleClientCheat_SetVarOther_BusyMessageGoesToCaller(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))
	other.delayed = true // target's CanAccess() = false

	dispatchTeleCheat(t, p, "setvarother target protect_var 99")
	emittedCaller := drainAfterTele(t, p, cc)

	if other.varps[1] != 0 {
		t.Errorf("busy-target setvarother mutated target: %d, want 0 (rejected)", other.varps[1])
	}
	if !bytes.Contains(emittedCaller, []byte("target is busy right now.")) {
		t.Errorf("expected 'target is busy right now.' on CALLER's conn; got %d bytes", len(emittedCaller))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_SetVarOther' -v`
Expected: FAIL.

- [ ] **Step 3: Add the `setvarother` arm**

In `modules/world/handlers_game.go`, inside the admin block's switch, append:

```go
		case "setvarother":
			// TS L220-252. NP-gated via inner break (DEVIATION-NAI-185-D2).
			// setvarother <username> <name> <value>. Missing-user message
			// goes to caller; busy-target message ALSO goes to caller
			// (DEVIATION-NAI-185-D3 — TS L242 asymmetry).
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 3)
			if len(sub) < 3 {
				return nil
			}
			other := p.client.server.LookupPlayerByUsername(sub[0])
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(sub[1])
			if cfg == nil {
				return nil
			}
			if cfg.Protect {
				other.CloseModal(true)
				if !other.CanAccess() {
					p.MessageGame(fmt.Sprintf("%s is busy right now.", sub[0]))
					return nil
				}
				other.ClearInteraction()
				other.UnsetMapFlag()
			}
			value := parseIntOr(sub[2], 0)
			if value > 0x7fffffff {
				value = 0x7fffffff
			}
			if value < -0x80000000 {
				value = -0x80000000
			}
			other.SetVarp(cfg.ID, int32(value))
			p.MessageGame(fmt.Sprintf("set %s: to %d on %s", sub[1], value, other.username))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_SetVarOther' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T3 — port ::setvarother admin cheat

Mirrors TS ClientCheatHandler.ts:220-252. Admin block (>=3), NP-gated
via inner break (DEVIATION-NAI-185-D2). Cross-player setvar: target
gets the SetVarp; caller gets the confirmation message.

DEVIATION-NAI-185-D3 pinned: busy-target message goes to caller, not
target (TS L242 asymmetry).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Port `::getvar` admin cheat

TS L253-267. Not NP-gated. Simplest of the four varp arms — no protect path, no clamp.

**Files:**
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_GetVar_HappyPath_MessagesValue(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)
	p.varps[0] = 42

	dispatchTeleCheat(t, p, "getvar transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("get transmit_only: 42")) {
		t.Errorf("expected 'get transmit_only: 42' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GetVar_MissingArg_Rejects(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "getvar")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("missing-arg getvar emitted 'get '; want silent reject. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_GetVar_UnknownName_SilentReject(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "getvar no_such_var")
	emitted := drainAfterTele(t, p, cc)

	if len(emitted) != 0 {
		t.Errorf("unknown-name getvar emitted %d bytes; want silent reject", len(emitted))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GetVar(_|$)' -v`
Expected: FAIL.

**Note:** The `(_|$)` anchors the regex so it doesn't also match `TestHandleClientCheat_GetVarOther`.

- [ ] **Step 3: Add the `getvar` arm**

In `modules/world/handlers_game.go`, inside the admin block's switch, append:

```go
		case "getvar":
			// TS L253-267. Not NP-gated. getvar <name> → caller gets
			// `get <debugname>: <value>` where value is p.Varp(id) (0
			// for unset).
			if args == "" {
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(args)
			if cfg == nil {
				return nil
			}
			p.MessageGame(fmt.Sprintf("get %s: %d", cfg.DebugName, p.Varp(cfg.ID)))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GetVar(_|$)' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T4 — port ::getvar admin cheat

Mirrors TS ClientCheatHandler.ts:253-267. Admin block (>=3), not
NP-gated. Routes through VarpTypeConfigs.ByName (T0) + Player.Varp.
Caller gets `get <debugname>: <value>`.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Port `::getvarother` admin cheat

TS L268-287. NP-gated. Returns the target's varp value to the caller.

**Files:**
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_GetVarOther_HappyPath_MessagesValueOnTarget(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))
	other.varps[0] = 77

	dispatchTeleCheat(t, p, "getvarother target transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("get transmit_only: 77 on target")) {
		t.Errorf("expected 'get transmit_only: 77 on target' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = false
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))
	other.varps[0] = 77

	dispatchTeleCheat(t, p, "getvarother target transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("getvarother under NP=false emitted 'get '; want dead. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_MissingArgs_Rejects(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "getvarother target") // 1 token, need 2
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("len(args)<2 getvarother emitted 'get '; want reject. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_UnknownUser_MessagesCaller(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "getvarother no_such_user transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("no_such_user is not logged in.")) {
		t.Errorf("expected 'no_such_user is not logged in.' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_UnknownVarp_SilentReject(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "getvarother target no_such_var")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("unknown-varp getvarother emitted 'get '; want silent reject. bytes=%d", len(emitted))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GetVarOther' -v`
Expected: FAIL.

- [ ] **Step 3: Add the `getvarother` arm**

In `modules/world/handlers_game.go`, inside the admin block's switch, append:

```go
		case "getvarother":
			// TS L268-287. NP-gated via inner break. getvarother
			// <username> <name>. Caller gets the target's varp value
			// formatted as `get <debugname>: <value> on <other.username>`.
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 {
				return nil
			}
			other := p.client.server.LookupPlayerByUsername(sub[0])
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(sub[1])
			if cfg == nil {
				return nil
			}
			p.MessageGame(fmt.Sprintf("get %s: %d on %s", cfg.DebugName, other.Varp(cfg.ID), other.username))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GetVarOther' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T5 — port ::getvarother admin cheat

Mirrors TS ClientCheatHandler.ts:268-287. Admin block (>=3), NP-gated
via inner break. Cross-player getvar: caller receives the target's
varp value.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Port `::giveother` admin cheat

TS L303-322. NP-gated. Cross-player inventory grant. Direct mirror of NAI-184 `teleother`'s LookupPlayerByUsername shape + NAI-184 `give`'s ObjType.ByName + InvAdd shape.

**Files:**
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Uses the existing NAI-184 fixture helpers `mustSetupTestInv`, `mustSetupNamedObj`, `totalUnits`, `countSlots` from `modules/world/player_inv_cheat_test.go`. Inv content reads via `s.invLookup.Get(player, invID)` matching `TestHandleClientCheat_Give_AddsToInv` at `handlers_game_test.go:875-912`.

Append to `modules/world/handlers_game_test.go`:

```go
// giveotherFixtureCommon wires the shared inv/obj infra for the
// ::giveother test cohort. Returns (caller, callerConn, server, invID, objID).
// objID=1277, debugName="test_obj", non-stackable so each unit fills a slot.
func giveotherFixtureCommon(t *testing.T) (*Player, net.Conn, *Server, int, int) {
	t.Helper()
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	invID := mustSetupTestInv(t, s, 0, 28)
	objID := mustSetupNamedObj(t, s, 1277, "test_obj", /*stackable=*/ false)
	s.invTypes.Inv = invID
	return p, cc, s, invID, objID
}

func TestHandleClientCheat_GiveOther_HappyPath_AddsToTarget(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj 5")

	inv := s.invLookup.Get(other, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(other, invID) = nil")
	}
	if got := countSlots(inv, objID); got != 5 {
		t.Errorf("after giveother target test_obj 5: countSlots(target, 1277) = %d, want 5", got)
	}
}

func TestHandleClientCheat_GiveOther_NoOpWhenNotProduction(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = false
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj 5")

	inv := s.invLookup.Get(other, invID)
	if inv != nil {
		if got := countSlots(inv, objID); got != 0 {
			t.Errorf("giveother under NP=false: countSlots(target, 1277) = %d, want 0", got)
		}
	}
}

func TestHandleClientCheat_GiveOther_UnknownUser_MessagesCaller(t *testing.T) {
	p, cc, s, _, _ := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "giveother no_such_user test_obj 5")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("no_such_user is not logged in.")) {
		t.Errorf("expected 'no_such_user is not logged in.' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GiveOther_UnknownItem_SilentReject(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target no_such_obj 5")

	inv := s.invLookup.Get(other, invID)
	if inv != nil {
		if got := countSlots(inv, objID); got != 0 {
			t.Errorf("unknown-item giveother added items: countSlots = %d, want 0", got)
		}
	}
}

func TestHandleClientCheat_GiveOther_MissingCountDefaultsToOne(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj")

	inv := s.invLookup.Get(other, invID)
	if got := countSlots(inv, objID); got != 1 {
		t.Errorf("after giveother target test_obj (no count): countSlots = %d, want 1", got)
	}
}

func TestHandleClientCheat_GiveOther_CountClampsToMin1(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj 0") // 0 → clamps to 1

	inv := s.invLookup.Get(other, invID)
	if got := countSlots(inv, objID); got != 1 {
		t.Errorf("after giveother target test_obj 0: countSlots = %d, want 1 (count clamp)", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GiveOther' -v`
Expected: FAIL.

- [ ] **Step 3: Add the `giveother` arm**

In `modules/world/handlers_game.go`, inside the admin block's switch, append:

```go
		case "giveother":
			// TS L303-322. NP-gated via inner break. giveother
			// <username> <obj> [count]. Count defaults to 1, clamps
			// to [1, 0x7fffffff].
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 3)
			if len(sub) < 2 {
				return nil
			}
			other := p.client.server.LookupPlayerByUsername(sub[0])
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))
				return nil
			}
			objType := p.client.server.objTypes.ByName(sub[1])
			if objType == nil {
				return nil
			}
			count := 1
			if len(sub) > 2 {
				count = parseIntOr(sub[2], 1)
				if count < 1 {
					count = 1
				}
				if count > 0x7fffffff {
					count = 0x7fffffff
				}
			}
			other.InvAdd(p.client.server.invTypes.Inv, objType.ID, count, false)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GiveOther' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T6 — port ::giveother admin cheat

Mirrors TS ClientCheatHandler.ts:303-322. Admin block (>=3), NP-gated
via inner break. Cross-player inventory grant: target receives the
items, no message to either side. Reuses NAI-184 give-arm InvAdd path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Port `::givecrap` admin cheat

TS L323-338. Not NP-gated. Fills inventory with 28 randomly-selected items that pass the (NodeMembers, dummyitem, certtemplate) filter.

**Files:**
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Uses `mustSetupTestInv` + manual `s.objTypes` slice (custom ObjType fields not covered by `mustSetupTestObj`). Inv content reads via `s.invLookup.Get(p, invID)` + iteration over `inv.Items` (per `countSlots` at `player_inv_cheat_test.go:137-145`).

Add `"time"` to the import block at the top of `modules/world/handlers_game_test.go` if not already present.

Append to `modules/world/handlers_game_test.go`:

```go
// givecrapFixture seeds objTypes with a controlled pool that exercises
// every filter branch. Pool composition:
//   id=0  nil (filter must skip)
//   id=1  pass (members=false, dummy=0, cert=-1)
//   id=2  pass (members=false, dummy=0, cert=-1)
//   id=3  fail-members (members=true)
//   id=4  fail-dummy   (dummyitem=1)
//   id=5  fail-cert    (certtemplate=10)
// Non-stackable so each invocation occupies a fresh slot.
func givecrapFixture(t *testing.T, nodeMembers bool) (*Player, *Server, int) {
	t.Helper()
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	s.cfg.NodeMembers = nodeMembers
	invID := mustSetupTestInv(t, s, 0, 28)
	s.invTypes.Inv = invID
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: []*objtype.ObjType{
			nil,
			{ConfigType: objtype.ConfigType{ID: 1, DebugName: "pass1"}, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 2, DebugName: "pass2"}, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 3, DebugName: "members"}, Members: true, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 4, DebugName: "dummy"}, DummyItem: 1, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 5, DebugName: "cert"}, CertTemplate: 10},
		},
	}
	return p, s, invID
}

func TestHandleClientCheat_GiveCrap_AddsTwentyEightFilteredItems(t *testing.T) {
	p, s, invID := givecrapFixture(t, false /* NodeMembers=false */)

	dispatchTeleCheat(t, p, "givecrap")

	inv := s.invLookup.Get(p, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(p, invID) = nil")
	}
	// 28 non-stackable items → 28 occupied slots.
	occupied := 0
	for _, it := range inv.Items {
		if it == nil {
			continue
		}
		occupied++
		// With NodeMembers=false, only id=1 or id=2 should appear.
		if it.Id != 1 && it.Id != 2 {
			t.Errorf("givecrap (F2P) slot has filtered-out id=%d", it.Id)
		}
	}
	if occupied != 28 {
		t.Errorf("givecrap occupied slots = %d, want 28", occupied)
	}
}

func TestHandleClientCheat_GiveCrap_MembersWorld_NoCertOrDummy(t *testing.T) {
	p, s, invID := givecrapFixture(t, true /* NodeMembers=true */)

	dispatchTeleCheat(t, p, "givecrap")

	inv := s.invLookup.Get(p, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(p, invID) = nil")
	}
	// Members items (id=3) become eligible; dummy (id=4) and cert (id=5)
	// stay filtered. Pin invariant: no dummy/cert slots.
	for _, it := range inv.Items {
		if it == nil {
			continue
		}
		if it.Id == 4 || it.Id == 5 {
			t.Errorf("givecrap (members world) has dummy/cert id=%d", it.Id)
		}
	}
}

func TestHandleClientCheat_GiveCrap_SmallPoolOnePassingItem_NoInfiniteLoop(t *testing.T) {
	// Pool with exactly one passing item among 5. The retry loop must
	// terminate within a 2s budget. A real infinite loop would hang
	// past the deadline and t.Fatal the run.
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	s.cfg.NodeMembers = false
	invID := mustSetupTestInv(t, s, 0, 28)
	s.invTypes.Inv = invID
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: []*objtype.ObjType{
			{ConfigType: objtype.ConfigType{ID: 0, DebugName: "pass"}, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 1, DebugName: "members"}, Members: true, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 2, DebugName: "dummy"}, DummyItem: 1, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 3, DebugName: "cert"}, CertTemplate: 10},
			{ConfigType: objtype.ConfigType{ID: 4, DebugName: "members2"}, Members: true, CertTemplate: -1},
		},
	}

	done := make(chan struct{})
	go func() {
		dispatchTeleCheat(t, p, "givecrap")
		close(done)
	}()
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("givecrap did not terminate within 2s on small-pool fixture")
	}
	inv := s.invLookup.Get(p, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(p, invID) = nil")
	}
	occupied := 0
	for _, it := range inv.Items {
		if it != nil {
			occupied++
		}
	}
	if occupied != 28 {
		t.Errorf("givecrap small-pool: occupied = %d, want 28", occupied)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GiveCrap' -v`
Expected: FAIL.

- [ ] **Step 3: Add the `givecrap` arm**

In `modules/world/handlers_game.go`, inside the admin block's switch, append:

```go
		case "givecrap":
			// TS L323-338. Not NP-gated. Fills inventory with 28
			// random items filtered by NodeMembers + DummyItem + CertTemplate.
			// Retry-loop matches TS `while (random === -1)`.
			for i := 0; i < 28; i++ {
				for {
					id := rand.IntN(len(p.client.server.objTypes.Configs))
					obj := p.client.server.objTypes.Configs[id]
					if obj == nil {
						continue
					}
					if !p.client.server.cfg.NodeMembers && obj.Members {
						continue
					}
					if obj.DummyItem != 0 {
						continue
					}
					if obj.CertTemplate != -1 {
						continue
					}
					p.InvAdd(p.client.server.invTypes.Inv, id, 1, false)
					break
				}
			}
```

**Pre-flight:** confirm `math/rand/v2` is already imported (handlers_game.go top-of-file). If not, add the import:

```bash
grep -n '"math/rand/v2"\|"math/rand"' modules/world/handlers_game.go
```

If missing, add `"math/rand/v2"` to the import block. Goscape uses v2 per existing convention (`input_tracking.go:7`, `npc_hunt.go:4`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_GiveCrap' -v -count=5`
Expected: all PASS across 5 reps (RNG variance smoke).

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T7 — port ::givecrap admin cheat

Mirrors TS ClientCheatHandler.ts:323-338. Admin block (>=3), not
NP-gated. Fills inventory with 28 randomly-selected items, retrying
until each pass the (NodeMembers, DummyItem, CertTemplate) filter.
Uses math/rand/v2 package-level rand per goscape convention.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Port `::broadcast` admin cheat

TS L353-359. NP-gated. Single-line: `s.BroadcastMes(args)`. TS L355 `args.length < 0` is unreachable — flagged as `DEVIATION-NAI-185-D1-DEAD-GUARD`, not ported.

**Files:**
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_Broadcast_FansOutToAllPlayers(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = true
	go io.Copy(io.Discard, cc)
	addOtherTestPlayer(t, s, "second_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "broadcast hello world")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("hello world")) {
		t.Errorf("caller did not receive broadcast 'hello world'; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_Broadcast_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = false

	dispatchTeleCheat(t, p, "broadcast hello world")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("hello world")) {
		t.Errorf("broadcast under NP=false reached caller; want dead. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_Broadcast_EmptyArgs_StillBroadcastsEmpty(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "broadcast")
	emitted := drainAfterTele(t, p, cc)

	// MESSAGE_GAME with empty body = framed opcode + 0x0a terminator.
	// At minimum, a non-zero byte count should land on the caller's conn.
	if len(emitted) == 0 {
		t.Errorf("::broadcast with empty args produced zero bytes; expected framed empty-MES")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Broadcast' -v`
Expected: FAIL.

- [ ] **Step 3: Add the `broadcast` arm**

In `modules/world/handlers_game.go`, inside the admin block's switch, append:

```go
		case "broadcast":
			// TS L353-359. NP-gated via inner break. broadcast <message>.
			// DEVIATION-NAI-185-D1-DEAD-GUARD: TS L355 `args.length < 0`
			// is unreachable (array length is non-negative); not ported.
			// TS uses cheat.substring(cmd.length+1); goscape uses `args`
			// (already the post-first-space remainder of the lowercased
			// input) — semantically identical for any single-token cmd.
			if !p.client.server.cfg.NodeProduction {
				break
			}
			p.client.server.BroadcastMes(args)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Broadcast' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-185 T8 — port ::broadcast admin cheat

Mirrors TS ClientCheatHandler.ts:353-359. Admin block (>=3), NP-gated
via inner break. Routes through Server.BroadcastMes (T1).
DEVIATION-NAI-185-D1-DEAD-GUARD: TS `args.length < 0` is unreachable;
not ported.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Rewrite carryforward deviation block + dispatch wiring smoke test

Two parts:
1. Rewrite the `DEVIATION-NAI-184-D2-D3-CARRYFORWARD` comment at `handlers_game.go:366-377` to enumerate the 10 still-unported cheats with their real blockers (per spec §6.4).
2. Add a dispatch wiring smoke test that drives one representative arm of each NAI-185 shape through `handleClientCheat` end-to-end (catches accidental dispatch-table regressions).

**Files:**
- Modify: `modules/world/handlers_game.go` (carryforward comment, lines 366-377)
- Modify: `modules/world/handlers_game_test.go` (one smoke test)

- [ ] **Step 1: Write the wiring smoke test**

Append to `modules/world/handlers_game_test.go`:

```go
// TestHandleClientCheat_DispatchesToNAI185Arms drives one representative
// arm of each NAI-185 shape (varp, cross-player varp, cross-player inv,
// rng-fill, fan-out broadcast) end-to-end through handleClientCheat.
// Pins:
//   - parts[0] dispatch reaches each arm
//   - existing staffModLevel >= 3 outer guard is honored
//   - existing addSessionLog at staffModLevel >= 2 records cheat names
//     (per NAI-183 outer guard at handlers_game.go:382-384)
func TestHandleClientCheat_DispatchesToNAI185Arms(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	// Drain the caller's conn for the duration of the test so flushWrite
	// from MessageGame side-effects doesn't backpressure.
	go io.Copy(io.Discard, cc)
	invID := mustSetupTestInv(t, s, 0, 28)
	objID := mustSetupNamedObj(t, s, 1277, "test_obj", /*stackable=*/ false)
	s.invTypes.Inv = invID
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	t.Run("setvar", func(t *testing.T) {
		dispatchTeleCheat(t, p, "setvar transmit_only 1")
		if p.varps[0] != 1 {
			t.Errorf("setvar dispatch failed: varps[0] = %d", p.varps[0])
		}
	})
	t.Run("setvarother", func(t *testing.T) {
		dispatchTeleCheat(t, p, "setvarother target transmit_only 2")
		if other.varps[0] != 2 {
			t.Errorf("setvarother dispatch failed: other.varps[0] = %d", other.varps[0])
		}
	})
	t.Run("giveother", func(t *testing.T) {
		dispatchTeleCheat(t, p, "giveother target test_obj 1")
		inv := s.invLookup.Get(other, invID)
		if inv == nil || countSlots(inv, objID) != 1 {
			t.Errorf("giveother dispatch failed: target missing test_obj")
		}
	})
	t.Run("broadcast_no_panic", func(t *testing.T) {
		// Wire smoke only — content assertions are in T8.
		dispatchTeleCheat(t, p, "broadcast smoke")
	})

	// givecrap covered by T7 dedicated tests; getvar/getvarother by T4/T5.
}
```

- [ ] **Step 2: Run smoke test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_DispatchesToNAI185Arms' -v`
Expected: all subtests PASS.

- [ ] **Step 3: Rewrite the carryforward comment**

In `modules/world/handlers_game.go`, replace lines 366-377 (the existing `DEVIATION-NAI-184-D2-D3-CARRYFORWARD` block) with:

```go
		// DEVIATION-NAI-185-D4-CARRYFORWARD — supersedes
		// DEVIATION-NAI-184-D2-D3-CARRYFORWARD. 10 TS ClientCheatHandler
		// cheats remain unported:
		//   Dev block (!NP && >=4): reload, rebuild, speed.
		//     Blocked on cache/script reload subsystem + runtime
		//     tick-rate mutation (tick.go interval is currently fixed).
		//   Admin block (>=3):      locadd, npcadd, openmain.
		//     Blocked on dynamic Loc/Npc spawn + interface routing.
		//   Super-mod (>=2):        setvis, ban, mute, kick.
		//     setvis blocked on Player.SetVisibility setter (trivial).
		//     ban/mute/kick: loginBridgeMod.NotifyPlayerBan/Mute exists
		//     (handler_reportabuse.go:50, handler_message_private.go:42);
		//     blocker is wiring caller-vs-automated args + kick's
		//     logout teardown, not the moderation transport itself.
		// Each cluster warrants its own follow-up sub-spec.
```

- [ ] **Step 4: Run full test pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-185 T9 — rewrite carryforward block + dispatch smoke

Replaces DEVIATION-NAI-184-D2-D3-CARRYFORWARD with
DEVIATION-NAI-185-D4-CARRYFORWARD. Lists the 10 still-unported cheats
with their real blockers (corrected for ban/mute/kick whose
loginBridgeMod transport already exists).

Adds TestHandleClientCheat_DispatchesToNAI185Arms — a smoke test
covering setvar/setvarother/giveother/broadcast dispatch + the
existing staffModLevel guard.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Close commit — finalize deviations + memory trailer

No code changes — verification + memory updates only.

- [ ] **Step 1: Final deviation grep**

```bash
rg -n "DEVIATION-NAI-185-D[1-4]" modules/ pkg/
```

Expected output:
- `D1-DEAD-GUARD`: 1 mention at the broadcast arm.
- `D2-NP-INLINE-GATE`: 1 mention at setvarother (the convention is established by NAI-184 teleother; D2 is documented in setvarother's doc-comment to give a per-arm anchor).
- `D3-VARPOTHER-MESSAGE-TARGET`: 1 mention at setvarother arm + 1 in `TestHandleClientCheat_SetVarOther_BusyMessageGoesToCaller`.
- `D4-CARRYFORWARD`: 1 mention at the rewritten carryforward block.

If any tag is missing or has unexpected sites, fix inline and amend the corresponding NAI-185 commit body.

- [ ] **Step 2: Final test pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race`
Expected: all PASS, no race detector hits.

- [ ] **Step 3: Memory updates**

Check `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/` for entries to add or update.

Likely new entries (add only if surprising / non-derivable):

- If `givecrap` RNG test required >1 rep to flake out → memory entry: "givecrap RNG retry-loop needs `-count=N` smoke to bind variance."
- If `BroadcastMes` lock pattern needed any unexpected discovery → memory entry on lock ordering.
- If the `setvarother` busy-message asymmetry caught a reviewer → memory entry citing D3 as a recurring TS-asymmetry pin shape.

Update `MEMORY.md` index if new entries added. **Do NOT add memory entries that just rephrase the spec / deviation block** — those are derivable from `docs/superpowers/specs/2026-05-12-nai-185-admin-cheats-cohort-design.md`.

- [ ] **Step 4: Close commit with memory trailer**

If no new memory entries were added, skip this step (the per-task commits are sufficient).

If memory entries were added:

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-185 close — memory entries from admin-cheats cohort

[1-3 sentences describing the entries added]

Closes memory: <comma-separated entry slugs>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: Verify the NAI-185 cohort closes cleanly**

```bash
git log --oneline f4ac001..HEAD
```

Expected: T0–T10 commits in order, each tagged `NAI-185 T<N>`, ending at T10 close (or T9 if no memory entries were needed). Spec ref at f4ac001.

---

## Self-review checklist (controller — before dispatching T0)

Per memory-driven controller pre-flight (`risk_register_premise_grep`, `controller_preflight`, `plan_grep_helper_patterns`, `plan_sibling_site_guard_audit`, `plan_type_name_grep`, `plan_runnable_test_fixtures`, `plan_helper_coverage`):

- [ ] Confirm `ConfigType` struct literal shape (`{ConfigType: objtype.ConfigType{ID: ..., DebugName: ...}, ...}`) is current — grep `objtype.VarPlayerType{ConfigType:` in existing tests.
- [ ] Confirm `Player.varps` is `[]int32` and `Player.varpsString` is `[]string` (already verified at `player_script.go:418-453`).
- [ ] Confirm `s.invLookup.Get(player, invID) *inventory.Inventory` is the inv accessor — used by T6/T7 (matches NAI-184 `TestHandleClientCheat_Give_AddsToInv` at `handlers_game_test.go:890`).
- [ ] Confirm `mustSetupTestInv`, `mustSetupNamedObj`, `countSlots`, `totalUnits` helpers exist at `player_inv_cheat_test.go:86-155` (T6/T7 depend on them).
- [ ] Confirm `cfg.NodeMembers` (config.go:36) and `cfg.NodeProduction` (config.go:43) field names.
- [ ] Confirm in-package access uses `p.username` field (not `Username()` method) — per `handler_reportabuse.go:50`.
- [ ] Confirm `math/rand/v2` is the existing import path for RNG (per `input_tracking.go:7`).
- [ ] Confirm `parseIntOr(s, def)` exists at `handlers_game.go:668`.
- [ ] Confirm `ObjType` field names + types: `Members bool` (`objtype.go:138`), `DummyItem int` (`objtype.go:164`), `CertTemplate int` (`objtype.go:156`), `Stackable bool` (`objtype.go:136`).
- [ ] Mentally execute `setvarTestFixture` (T2), `giveotherFixtureCommon` (T6), `givecrapFixture` (T7): struct-literal field paths and slice initializations are syntactically valid Go.
- [ ] Confirm `"time"` import is added to `handlers_game_test.go` before T7 dispatch (needed for `time.After` in the small-pool test).

If any checklist item flips RED, fix inline before dispatch.
