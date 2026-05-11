# NAI-162 — Trivial-handler sweep #4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 15 script-opcode handlers across 4 bundles, closing the missing-handler cascade-tail from 18 to 3 and retiring deviation chain NAI-115-D1.

**Architecture:** Bundle-ordered TDD ports. B0 lands 6 TS-unimplemented stubs (P_OPHELD pattern). B1 lands 4 small handlers + 4 prerequisite methods/helpers (HeroPoints.Clear, IsIndoors, LastLoginInfo, InvTotalParamStack). B2 lands the WealthEvent subsystem (struct + (*Player).AddWealthEvent + ObjTypes.ByName lookup) plus 3 handlers (WealthEvent, PLocMerge, POpPlayerT) AND retires NAI-115-D1 (6 sites). B3 lands inv-drop handlers (BothDropSlot, InvDropAll) which leverage B2's AddWealthEvent for SCOPE_PERM.

**Tech Stack:** Go 1.26+ (`go_version.md`). Test framework: standard `testing`. Build commands prefix `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` per global CLAUDE.md.

**Spec reference:** `docs/superpowers/specs/2026-05-11-nai-162-trivial-handler-sweep-4-design.md`

**Cohort discipline (every task):**
- Pop order is **LIFO** matching `handleInvDropSlot` precedent. TS `popInts(N) → [a, b, c, d]` translates to goscape `d := s.PopInt(); c := s.PopInt(); b := s.PopInt(); a := s.PopInt()`.
- `IntOperand` access is `s.Script.IntOperands[s.PC]` per `setActiveLocSlot` at handlers_loc.go:30.
- All commits use `git commit --no-gpg-sign` (global CLAUDE.md).
- Test fixtures push args in TS push order (so popping yields TS popInts order).
- Per memory `mock_recorder_field_naming_check.md`: grep the actual `mockPlayer` struct and match the dominant recorder convention (e.g., `lastFooCalls []FooCall`) before codifying any mock field name.
- Per memory `defensive_gate_doc_comment_label.md`: label nil-client/server guards "(goscape defensive; TS skips this check)".

---

## File Structure

### Files Created

| Path | Responsibility |
|---|---|
| `modules/world/wealth.go` | `WealthEvent` struct, `WealthItem` struct, `WealthEventType*` constants |
| `pkg/pathfinder/collision/indoors.go` | `IsIndoors(flag uint32) bool` predicate (operates on already-read flag) |
| `pkg/pathfinder/collision/indoors_test.go` | Unit tests for IsIndoors |
| `pkg/script/handlers_b0_stubs.go` | B0 stub handlers (6 functions) |
| `pkg/script/handlers_b0_stubs_test.go` | B0 table-driven dispatch test |
| `pkg/script/handlers_server.go` (if not present) | `handleMapIndoors` (and future ServerOps handlers) |
| `pkg/script/handlers_server_test.go` | MAP_INDOORS tests |

### Files Modified

| Path | Reason |
|---|---|
| `pkg/script/handlers.go` | Add 15 dispatch-map entries (6 B0 + 4 B1 + 3 B2 + 2 B3) |
| `pkg/script/handlers_player.go` | `handleLastLoginInfo`, `handleWealthEvent`, `handlePLocMerge`, `handlePOpPlayerT` |
| `pkg/script/handlers_player_test.go` | Tests for the four player handlers above + mockPlayer additions |
| `pkg/script/handlers_inv.go` | `handleInvTotalParamStack`, `handleBothDropSlot`, `handleInvDropAll` + NAI-115-D1 retirement (6 sites) |
| `pkg/script/handlers_inv_test.go` | Tests for the three inv handlers above + retirement test updates |
| `pkg/script/handlers_npc.go` | `handleNpcStatHeal` |
| `pkg/script/handlers_npc_test.go` | NPC_STATHEAL tests + mockNpc additions |
| `pkg/script/handlers_obj.go` | NAI-115-D1 retirement (1 site) |
| `pkg/script/state.go` | `ActivePlayer` interface widening (LastLoginInfo, InvTotalParamStack, AddWealthEvent); `ActiveNpc` interface widening (HeroPointsClear); `WorldSurface` widening (IsIndoors, MergeLoc) |
| `pkg/script/runner_test.go` | `mockPlayer` / `mockActiveNpc` recorder fields + methods |
| `pkg/script/handlers_npc_test.go` | `mockNpc` recorder fields + methods |
| `pkg/objtype/objtype.go` | Add `(*ObjTypes).ByName(name string) *ObjType` lookup (B2 only) |
| `pkg/objtype/objtype_test.go` | `ByName` lookup test |
| `modules/world/heropoints.go` | `(*HeroPoints).Clear()` method |
| `modules/world/heropoints_test.go` | `Clear()` test |
| `modules/world/player.go` | Add `wealthLog []WealthEvent` field; `Session() string` getter if absent |
| `modules/world/player_script.go` | `(*Player).LastLoginInfo()`, `(*Player).InvTotalParamStack(inv, param int) int`, `(*Player).AddWealthEvent(evt WealthEvent)` |
| `modules/world/player_script_test.go` | Unit tests for the three Player methods |
| `modules/world/server.go` (or `world_zone.go`) | `(*Server).IsIndoors(x, z, level int) bool` adapter calling `pkg/pathfinder/collision.IsIndoors` against the global flagmap |
| `modules/world/server_invs.go` (or analogue) | `(*Server)` adapter expose AddObj-with-NoReceiver and IsIndoors if not already wired through `WorldSurface` |

---

## Bundle B0 — Stub sweep (6 handlers)

**Goal:** Land 6 TS-unimplemented opcode stubs (P_OPHELD pattern). Audit recount 18 → 12.

### Task B0.1: Re-verify TS-unimplemented status

**Files:**
- Verify only (no edits)

- [ ] **Step 1: Re-run the per-op MISSING-IN-TS check at HEAD**

Run:
```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  for op in PUSH_VARBIT POP_VARBIT SET_GENDER LC_OP OC_IOP OC_OP; do
    hit=$(rg -l "ScriptOpcode\.${op}\]" --type ts src/engine/script/handlers/ 2>/dev/null | head -1)
    if [ -z "$hit" ]; then echo "MISSING-IN-TS: $op"; else echo "FOUND-IN-TS:   $op ($hit)"; fi
  done
```

Expected: All 6 lines report `MISSING-IN-TS`. If any report `FOUND-IN-TS`, STOP and revisit the spec — that op needs a real port, not a stub.

- [ ] **Step 2: Re-run the missing-handler audit at HEAD**

Run:
```bash
mkdir -p /tmp/claude && \
  awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/claude/handled.txt && \
  awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/claude/declared.txt && \
  comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l && \
  comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt
```

Expected: count == 18; list includes the 6 B0 ops.

### Task B0.2: Write B0 stub handlers (TDD: failing test first)

**Files:**
- Create: `pkg/script/handlers_b0_stubs_test.go`
- Create: `pkg/script/handlers_b0_stubs.go`
- Modify: `pkg/script/handlers.go` (add 6 dispatch entries)

- [ ] **Step 1: Write the failing table-driven test**

Create `pkg/script/handlers_b0_stubs_test.go`:

```go
package script

import (
	"strings"
	"testing"
)

// TestNAI162B0StubsReturnUnimplemented pins the 6 TS-unimplemented
// stubs (PUSH_VARBIT, POP_VARBIT, SET_GENDER, LC_OP, OC_IOP, OC_OP).
// Each returns an error containing "unimplemented" without mutating
// any pointer state. Mirrors NAI-161 P_OPHELD stub-with-pin shape.
func TestNAI162B0StubsReturnUnimplemented(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
		want string
	}{
		{"PUSH_VARBIT", OpPushVarbit, "PUSH_VARBIT: unimplemented"},
		{"POP_VARBIT", OpPopVarbit, "POP_VARBIT: unimplemented"},
		{"SET_GENDER", OpSetGender, "SET_GENDER: unimplemented"},
		{"LC_OP", OpLcOp, "LC_OP: unimplemented"},
		{"OC_IOP", OpOcIop, "OC_IOP: unimplemented"},
		{"OC_OP", OpOcOp, "OC_OP: unimplemented"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, ok := handlers[tc.op]
			if !ok {
				t.Fatalf("opcode %d (%s) has no dispatch entry", tc.op, tc.name)
			}
			s := &ScriptState{}
			err := handler(s)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q: want substring %q", err.Error(), tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestNAI162B0StubsReturnUnimplemented -v
```

Expected: FAIL — opcodes have no dispatch entry (`handlers[OpPushVarbit]` missing).

- [ ] **Step 3: Implement the 6 stubs**

Create `pkg/script/handlers_b0_stubs.go`:

```go
package script

import "fmt"

// handlePushVarbit (PUSH_VARBIT, opcode 25) is a TS-unimplemented stub.
// TS declares the opcode at ScriptOpcode.ts:22 (// official, see cs2)
// but has no handlers/* case-label entry. Per NAI-162 §3 deviation
// NAI-162-D-STUB-PUSHVARBIT, this stub returns an 'unimplemented'
// error rather than no-op so future TS sync re-ports the handler
// explicitly. Mirrors NAI-161 handlePOpHeld shape.
func handlePushVarbit(s *ScriptState) error {
	return fmt.Errorf("PUSH_VARBIT: unimplemented")
}

// handlePopVarbit (POP_VARBIT, opcode 27) — TS-unimplemented stub.
// NAI-162-D-STUB-POPVARBIT.
func handlePopVarbit(s *ScriptState) error {
	return fmt.Errorf("POP_VARBIT: unimplemented")
}

// handleSetGender (SET_GENDER, opcode 2099) — TS-unimplemented stub.
// NAI-162-D-STUB-SETGENDER.
func handleSetGender(s *ScriptState) error {
	return fmt.Errorf("SET_GENDER: unimplemented")
}

// handleLcOp (LC_OP, opcode 4105) — TS-unimplemented stub. Pairs with
// the future OPHELD trigger-plumbing cohort (NAI-161 forward-route).
// NAI-162-D-STUB-LCOP.
func handleLcOp(s *ScriptState) error {
	return fmt.Errorf("LC_OP: unimplemented")
}

// handleOcIop (OC_IOP, opcode 4205) — TS-unimplemented stub.
// NAI-162-D-STUB-OCIOP.
func handleOcIop(s *ScriptState) error {
	return fmt.Errorf("OC_IOP: unimplemented")
}

// handleOcOp (OC_OP, opcode 4208) — TS-unimplemented stub.
// NAI-162-D-STUB-OCOP.
func handleOcOp(s *ScriptState) error {
	return fmt.Errorf("OC_OP: unimplemented")
}
```

- [ ] **Step 4: Add dispatch entries to `pkg/script/handlers.go`**

Locate the `var handlers = map[Opcode]Handler{` map. Add (in opcode-numeric order to match existing convention):

```go
	OpPushVarbit:    handlePushVarbit,
	OpPopVarbit:     handlePopVarbit,
	// ... existing entries ...
	OpSetGender:     handleSetGender,
	// ... existing entries ...
	OpLcOp:          handleLcOp,
	// ... existing entries ...
	OpOcIop:         handleOcIop,
	OpOcOp:          handleOcOp,
```

Plan executor: insert each entry near the other opcodes in its numeric range; do not just append at the end.

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestNAI162B0StubsReturnUnimplemented -v
```

Expected: PASS — 6 subtests pass.

- [ ] **Step 6: Run full pkg/script tests for regression check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -count=1
```

Expected: PASS.

- [ ] **Step 7: Re-run missing-handler audit (recount)**

Run:
```bash
awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/claude/handled.txt && \
  awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/claude/declared.txt && \
  comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l
```

Expected output: `12`.

- [ ] **Step 8: Commit B0**

```bash
git add pkg/script/handlers_b0_stubs.go pkg/script/handlers_b0_stubs_test.go pkg/script/handlers.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B0 — 6 TS-unimplemented opcode stubs

PUSH_VARBIT (25), POP_VARBIT (27), SET_GENDER (2099), LC_OP (4105),
OC_IOP (4205), OC_OP (4208) — all declared in TS ScriptOpcode.ts
but lacking handlers/* case-labels. Stub-with-pin pattern per
NAI-161 P_OPHELD (deviations NAI-162-D-STUB-*). Missing-handler
audit: 18 → 12.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle B1 — Trivial + small (4 handlers + 4 prerequisites)

**Goal:** Land 4 prerequisite methods/helpers and 4 handlers. Audit recount 12 → 8.

### Task B1.1: `(*HeroPoints).Clear()` (prerequisite for NPC_STATHEAL)

**Files:**
- Modify: `modules/world/heropoints.go`
- Test: `modules/world/heropoints_test.go`

- [ ] **Step 1: Write the failing test**

Add to `modules/world/heropoints_test.go`:

```go
// TestHeroPoints_Clear pins (*HeroPoints).Clear() — resets the
// contributor ledger to zero entries. Mirrors TS HeroPoints.clear()
// called from NPC_STATHEAL HP-full branch (NpcOps.ts:255).
func TestHeroPoints_Clear(t *testing.T) {
	hp := NewHeroPoints(10)
	hp.AddHero(101, 50)
	hp.AddHero(202, 30)
	if got := len(hp.entries); got != 2 {
		t.Fatalf("setup: want 2 entries, got %d", got)
	}

	hp.Clear()

	if got := len(hp.entries); got != 0 {
		t.Errorf("after Clear: want 0 entries, got %d", got)
	}
}

// TestHeroPoints_Clear_Empty pins that Clear() on an empty ledger
// is a safe no-op (no panic).
func TestHeroPoints_Clear_Empty(t *testing.T) {
	hp := NewHeroPoints(10)
	hp.Clear()
	if got := len(hp.entries); got != 0 {
		t.Errorf("want 0 entries, got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestHeroPoints_Clear -v
```

Expected: FAIL — `hp.Clear undefined`.

- [ ] **Step 3: Implement Clear**

Add to `modules/world/heropoints.go` (after `AddHero` / wherever the file's method group ends):

```go
// Clear zeroes the contributor ledger. Mirrors TS HeroPoints.clear()
// invoked from NPC_STATHEAL HP-full branch at NpcOps.ts:255.
func (h *HeroPoints) Clear() {
	h.entries = h.entries[:0]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestHeroPoints_Clear -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/heropoints.go modules/world/heropoints_test.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-162 B1.1 — (*HeroPoints).Clear()

Resets contributor ledger to zero entries. Prerequisite for B1.4
NPC_STATHEAL HP-full branch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.2: `collision.IsIndoors` helper (prerequisite for MAP_INDOORS)

**Files:**
- Create: `pkg/pathfinder/collision/indoors.go`
- Create: `pkg/pathfinder/collision/indoors_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/pathfinder/collision/indoors_test.go`:

```go
package collision

import "testing"

// TestIsIndoors pins the predicate against the FlagRoof bit. Mirrors
// TS isIndoors (GameMap.ts:417-419) which calls isFlagged(...,
// CollisionFlag.ROOF).
func TestIsIndoors(t *testing.T) {
	cases := []struct {
		name string
		flag uint32
		want bool
	}{
		{"open-tile-no-roof", FlagOpen, false},
		{"roof-only", FlagRoof, true},
		{"roof-plus-blockwalk", FlagRoof | FlagBlockWalk, true},
		{"blockwalk-only", FlagBlockWalk, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsIndoors(tc.flag); got != tc.want {
				t.Errorf("IsIndoors(%#x) = %v, want %v", tc.flag, got, tc.want)
			}
		})
	}
}
```

Plan executor: verify `FlagOpen` and `FlagBlockWalk` constant names against `pkg/pathfinder/collision/flag.go` before running. If the constants are named differently (e.g., `FlagNone`, `FlagBlocked`), update the test fixture to match. Per memory `plan_grep_helper_patterns.md`, grep the actual file.

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/collision -run TestIsIndoors -v
```

Expected: FAIL — `IsIndoors undefined`.

- [ ] **Step 3: Implement IsIndoors**

Create `pkg/pathfinder/collision/indoors.go`:

```go
package collision

// IsIndoors reports whether the given tile flag carries the FlagRoof
// bit. Mirrors TS isIndoors (GameMap.ts:417-419), which calls
// isFlagged(x, z, level, CollisionFlag.ROOF). Caller is responsible
// for resolving (x, z, level) to a flag via the FlagMap.
func IsIndoors(flag uint32) bool {
	return flag&FlagRoof != 0
}
```

Plan executor: confirm the underlying flag type (uint32 vs int). If `FlagRoof` is typed as `int` per the package, change the signature accordingly. Grep `type Flag` in `pkg/pathfinder/collision/flag.go`.

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/collision -run TestIsIndoors -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/pathfinder/collision/indoors.go pkg/pathfinder/collision/indoors_test.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(collision): NAI-162 B1.2 — IsIndoors flag predicate

Mirrors TS isIndoors (GameMap.ts:417-419) — flag&FlagRoof != 0.
Prerequisite for B1.4 MAP_INDOORS handler.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.3: `(*Player).LastLoginInfo()` stub method

Per spec §2.2.2 R6 risk: if `LAST_LOGIN_INFO` ServerProt is absent, this method lands as a silent no-op + tracked deviation `NAI-162-D-LASTLOGIN-NO-PACKET`. Pre-flight grep at brainstorm confirmed absence; plan executor MUST re-verify at task-start time.

**Files:**
- Modify: `modules/world/player_script.go`
- Test: `modules/world/player_script_test.go`

- [ ] **Step 1: Re-verify ServerProt absence**

Run:
```bash
rg -n "LastLoginInfo|LAST_LOGIN_INFO" pkg/io/protocol/ 2>/dev/null
```

Expected: empty (no matches). If matches appear: STOP, this task gets an upgrade — port the prot first.

- [ ] **Step 2: Write the failing test**

Add to `modules/world/player_script_test.go`:

```go
// TestPlayer_LastLoginInfo_StubNoOp pins (*Player).LastLoginInfo() as
// a silent no-op until LAST_LOGIN_INFO ServerProt is ported. Tracked
// as NAI-162-D-LASTLOGIN-NO-PACKET. Test pins the method exists and
// doesn't panic on a fresh Player.
func TestPlayer_LastLoginInfo_StubNoOp(t *testing.T) {
	p := &Player{}
	p.LastLoginInfo() // must not panic; current behaviour is no-op
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestPlayer_LastLoginInfo -v
```

Expected: FAIL — `p.LastLoginInfo undefined`.

- [ ] **Step 4: Implement the stub method**

Add to `modules/world/player_script.go` (after an existing method that fits the file's grouping):

```go
// LastLoginInfo emits a LAST_LOGIN_INFO server packet with the
// previous-login timestamp and IP. Mirrors TS Player.lastLoginInfo
// (PlayerOps.ts:932 caller).
//
// NAI-162-D-LASTLOGIN-NO-PACKET: ServerProt absent at NAI-162 cut.
// Method is a no-op until the prot is ported. Once the prot lands,
// implementation queues an outgoing packet via the standard
// (*Player) client.write pattern.
func (p *Player) LastLoginInfo() {
	// Intentional no-op pending ServerProt port.
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestPlayer_LastLoginInfo -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-162 B1.3 — (*Player).LastLoginInfo stub

NAI-162-D-LASTLOGIN-NO-PACKET: LAST_LOGIN_INFO ServerProt absent;
method is a silent no-op until the prot is ported. Prerequisite for
B1 handler dispatch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.4: `(*Player).InvTotalParamStack(invID, paramID int) int`

Plan executor: per memory `audit_full_method_against_ts.md`, read the FULL TS `Player.invTotalParamStack` body before codifying (search `invTotalParamStack` in `Engine-TS/src/engine/entity/Player.ts`). The brainstorm assumed the formula `Σ slot.count * objType.param(paramID)` but TS may apply additional filters (stackable / scope) that need to be mirrored.

**Files:**
- Modify: `modules/world/player_script.go`
- Test: `modules/world/player_script_test.go`

- [ ] **Step 1: Read TS source**

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  rg -nB2 -A20 "invTotalParamStack" src/engine/entity/Player.ts
```

Record the full body. Note any filter beyond "non-empty slots".

- [ ] **Step 2: Write the failing test**

Add to `modules/world/player_script_test.go`:

```go
// TestPlayer_InvTotalParamStack pins the sum formula. Build a player
// with an inv containing two slots: {id=10, count=3} + {id=20, count=5}.
// objType.param(7) returns 100 for id=10, 50 for id=20. Expected:
// 3*100 + 5*50 = 550.
//
// Plan executor: adjust the fixture if TS Player.invTotalParamStack
// includes filters not assumed at NAI-162 brainstorm.
func TestPlayer_InvTotalParamStack(t *testing.T) {
	// Fixture: plan executor wires a mockable inventory + configs.
	// The shape below is the public contract pinned by this test.
	p := newTestPlayerWithInv(t, 5 /*invID*/, []invSlotFixture{
		{id: 10, count: 3},
		{id: 20, count: 5},
	})
	p.attachConfigs(map[int]map[int]int{
		// objID → paramID → value
		10: {7: 100},
		20: {7: 50},
	})

	got := p.InvTotalParamStack(5, 7)
	if got != 550 {
		t.Errorf("InvTotalParamStack: got %d, want 550", got)
	}
}

// TestPlayer_InvTotalParamStack_EmptyInv pins zero return on empty inv.
func TestPlayer_InvTotalParamStack_EmptyInv(t *testing.T) {
	p := newTestPlayerWithInv(t, 5, nil)
	if got := p.InvTotalParamStack(5, 7); got != 0 {
		t.Errorf("empty inv: got %d, want 0", got)
	}
}

// TestPlayer_InvTotalParamStack_NilInv pins nil-inv-returns-zero.
func TestPlayer_InvTotalParamStack_NilInv(t *testing.T) {
	p := &Player{}
	if got := p.InvTotalParamStack(999, 7); got != 0 {
		t.Errorf("missing inv: got %d, want 0", got)
	}
}
```

Plan executor: confirm `newTestPlayerWithInv` and `invSlotFixture` helper names against existing test infra at `modules/world/player_inv_test.go`. If helpers don't exist with these exact names, either create matching helpers OR adapt the test to the existing helper API. Per memory `plan_grep_helper_patterns.md`, grep first.

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestPlayer_InvTotalParamStack -v
```

Expected: FAIL — method undefined.

- [ ] **Step 4: Implement InvTotalParamStack**

Add to `modules/world/player_script.go`:

```go
// InvTotalParamStack sums (slot.count * objType.Param(paramID))
// across every non-empty slot of the inventory at invID. Param values
// resolve via the ObjType accessor; missing params contribute zero.
// Returns zero on nil-client, missing inventory, or missing InvType.
//
// Mirrors TS Player.invTotalParamStack (Player.ts caller of
// InvOps.ts:795). Plan executor: cross-check TS body for filters
// beyond "non-empty slot" before merging.
//
// (goscape defensive; TS skips this check) Nil-client guard mirrors
// other player methods that operate via server resolution.
func (p *Player) InvTotalParamStack(invID, paramID int) int {
	if p.client == nil || p.client.server == nil {
		return 0
	}
	inv := p.client.server.invs.Get(p, invID)
	if inv == nil {
		return 0
	}
	total := 0
	for slot := 0; slot < inv.Capacity(); slot++ {
		obj := inv.Get(slot)
		if obj == nil {
			continue
		}
		objType := p.client.server.objTypes.Get(obj.ID)
		if objType == nil {
			continue
		}
		paramVal := objType.IntParam(paramID)
		total += obj.Count * paramVal
	}
	return total
}
```

Plan executor: verify the exact accessor names — `p.client.server.invs.Get`, `p.client.server.objTypes.Get`, `objType.IntParam(paramID)`, `inv.Capacity()`, `inv.Get(slot)`, `obj.ID`, `obj.Count` — by grep before locking in. Per memory `mock_recorder_field_naming_check.md`. If names differ, adapt to the actual API.

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestPlayer_InvTotalParamStack -v
```

Expected: PASS (all 3 subtests).

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-162 B1.4 — (*Player).InvTotalParamStack

Sums slot.count × objType.IntParam(paramID) across non-empty inv
slots. Mirrors TS Player.invTotalParamStack. Prerequisite for B1
handler dispatch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.5: Widen `ActivePlayer` / `ActiveNpc` / `WorldSurface` interfaces

**Files:**
- Modify: `pkg/script/state.go`
- Modify: `pkg/script/runner_test.go` (mockPlayer additions)
- Modify: `pkg/script/handlers_npc_test.go` (mockNpc additions if present)
- Modify: `pkg/script/handlers_player_test.go` (if it defines a separate mock)

- [ ] **Step 1: Identify the interface declarations**

Run:
```bash
rg -nB1 -A30 "^type ActivePlayer interface|^type ActiveNpc interface|^type WorldSurface interface" pkg/script/state.go
```

Record the current method sets.

- [ ] **Step 2: Plan the additions**

The interface must gain these methods so handlers can call them:

- `ActivePlayer`:
  - `LastLoginInfo()`
  - `InvTotalParamStack(invID, paramID int) int`
  - `AddWealthEvent(evt WealthEvent)` — landed by B2 but signature defined here

- `ActiveNpc`:
  - `HeroPointsClear()` — wrapper for `(*Npc).heroPoints.Clear()` since the mock doesn't have a HeroPoints field

- `WorldSurface`:
  - `IsIndoors(x, z, level int) bool`

Note: `WealthEvent` is a struct in `modules/world` (B2). To avoid import cycle, declare `WealthEvent` in a package the script layer can see — either:
- Option A: Define `WealthEvent` in `pkg/script` and have `modules/world` re-export / consume it.
- Option B: Use an anonymous interface signature in pkg/script (`AddWealthEvent(eventType int, items []WealthItemArg, value int, recipientSession string)`).

Plan executor: pick Option A (Define `WealthEvent` + `WealthItem` in `pkg/script` package as plain data types; `modules/world` consumes them). This is cleaner and matches the existing pattern for cross-package data shapes. See memory `interface_at_cyclic_import_boundary`.

- [ ] **Step 3: Add to `pkg/script/state.go`** (above the ActivePlayer interface)

```go
// WealthEvent captures a single wealth-affecting event for analytics.
// Mirrors TS WealthEvent payload at PlayerOps.ts:1197-1201 plus
// InvOps.ts:695-700, 781-789. Goscape AddWealthEvent appends to an
// in-memory log only per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY;
// analytics RPC integration deferred.
type WealthEvent struct {
	EventType        int
	AccountItems     []WealthItem
	AccountValue     int
	RecipientSession string // optional; empty for non-PVP events
}

// WealthItem is a single line-item inside a WealthEvent.
type WealthItem struct {
	ID    int
	Name  string
	Count int
}

// WealthEventType enum mirrors TS WealthEventType.
const (
	WealthEventTypeDrop    = 1 // (TS-aligned ordinal — plan executor
	WealthEventTypePVP     = 2 //  confirms against TS WealthEventType
	WealthEventTypeDeath   = 3 //  enum at definition site)
)
```

Plan executor: read TS `WealthEventType` enum at brainstorm-time grep (search `enum WealthEventType` or `WealthEventType\.` in Engine-TS); update the constant values to match TS ordinals.

- [ ] **Step 4: Widen `ActivePlayer` interface**

Locate `type ActivePlayer interface {` and append methods:

```go
	// NAI-162 B1: trivial-handler sweep #4 widenings.
	LastLoginInfo()
	InvTotalParamStack(invID, paramID int) int
	AddWealthEvent(evt WealthEvent)
```

- [ ] **Step 5: Widen `ActiveNpc` interface**

Locate `type ActiveNpc interface {` and append:

```go
	// NAI-162 B1: NPC_STATHEAL HP-full branch.
	HeroPointsClear()
```

- [ ] **Step 6: Widen `WorldSurface` interface (or equivalent)**

Locate the world-facing interface (run `rg -n "type WorldSurface interface|type Configs interface" pkg/script/state.go`). Add:

```go
	// NAI-162 B1: MAP_INDOORS handler.
	IsIndoors(x, z, level int) bool
```

If the interface doesn't exist or world surface is provided differently, route IsIndoors via `s.World` field of `ScriptState`. Plan executor inspects existing patterns (e.g., how MERGE_LOC reaches `World.MergeLoc`) and matches.

- [ ] **Step 7: Add mock methods to `mockPlayer` (`pkg/script/runner_test.go`)**

Per memory `mock_recorder_field_naming_check.md`, grep the existing mockPlayer field naming convention first:

```bash
rg -nA3 "type mockPlayer struct" pkg/script/runner_test.go
```

Then add (matching the convention — `lastFooCalls` or `fooCalls` per existing style):

```go
// inside mockPlayer struct:
	lastLoginInfoCalls           int
	invTotalParamStackReturn     int // configurable for tests
	invTotalParamStackArgs       []invTotalParamStackArg
	addWealthEventCalls          []WealthEvent
```

Add helper struct (top-level in `runner_test.go`):

```go
type invTotalParamStackArg struct {
	InvID, ParamID int
}
```

Add method implementations on mockPlayer:

```go
func (m *mockPlayer) LastLoginInfo()                  { m.lastLoginInfoCalls++ }
func (m *mockPlayer) InvTotalParamStack(inv, p int) int {
	m.invTotalParamStackArgs = append(m.invTotalParamStackArgs, invTotalParamStackArg{InvID: inv, ParamID: p})
	return m.invTotalParamStackReturn
}
func (m *mockPlayer) AddWealthEvent(evt WealthEvent) {
	m.addWealthEventCalls = append(m.addWealthEventCalls, evt)
}
```

- [ ] **Step 8: Add `HeroPointsClear` to `mockNpc` and `mockActiveNpc`**

Run:
```bash
rg -nA10 "type mockNpc struct|type mockActiveNpc struct" pkg/script/
```

Add fields + methods matching existing convention:

```go
// mockNpc additions:
	heroPointsClearCalls int
func (m *mockNpc) HeroPointsClear() { m.heroPointsClearCalls++ }

// mockActiveNpc additions (handlers_player_test.go):
func (m *mockActiveNpc) HeroPointsClear() {}
```

- [ ] **Step 9: Add `IsIndoors` to world surface mock**

Locate the mock that implements the world surface (likely in `runner_test.go` or `handlers_test.go`). Add:

```go
// inside mockWorld (or whatever the mock is named):
	isIndoorsReturn bool  // configurable; default false
	isIndoorsArgs   []indoorArg

func (m *mockWorld) IsIndoors(x, z, level int) bool {
	m.isIndoorsArgs = append(m.isIndoorsArgs, indoorArg{X: x, Z: z, Level: level})
	return m.isIndoorsReturn
}

type indoorArg struct{ X, Z, Level int }
```

Plan executor: substitute the actual mock type name.

- [ ] **Step 10: Verify compile (no tests yet)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/... ./modules/world/...
```

Expected: build succeeds. If `modules/world` fails because `(*Player)` lacks `AddWealthEvent` or `(*Npc)` lacks `HeroPointsClear`: those are landed in upcoming tasks (B1.6 wires HeroPointsClear; B2.2 wires AddWealthEvent). Skip the build check until after B2.2, OR temporarily add no-op stubs on `*Player` / `*Npc` to satisfy the interface and remove the stubs later.

Per memory `interface_at_cyclic_import_boundary`: cleanest approach is to land the interface methods + concrete-type stubs in this same task to satisfy the type checker. Concrete bodies will be filled in B1.6 / B2.2.

Add to `modules/world/player_script.go`:

```go
// AddWealthEvent stub — concrete body lands in B2.2 (NAI-162).
func (p *Player) AddWealthEvent(evt script.WealthEvent) {
	// placeholder; B2.2 wires p.wealthLog.
}
```

Add to `modules/world/npc.go` (or wherever `*Npc` methods live):

```go
// HeroPointsClear wraps n.heroPoints.Clear for the ActiveNpc
// interface. NAI-162 B1.
func (n *Npc) HeroPointsClear() {
	n.heroPoints.Clear()
}
```

- [ ] **Step 11: Re-run build**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: success.

- [ ] **Step 12: Commit**

```bash
git add pkg/script/state.go pkg/script/runner_test.go pkg/script/handlers_player_test.go pkg/script/handlers_npc_test.go modules/world/player_script.go modules/world/npc.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B1.5 — widen ActivePlayer/ActiveNpc/WorldSurface

ActivePlayer gains LastLoginInfo, InvTotalParamStack, AddWealthEvent.
ActiveNpc gains HeroPointsClear. WorldSurface gains IsIndoors.
WealthEvent + WealthItem + WealthEventType* defined in pkg/script
to avoid import cycle (per interface_at_cyclic_import_boundary).
mockPlayer/mockNpc/mockWorld gain matching recorders.
AddWealthEvent on (*Player) is a placeholder; B2.2 wires
p.wealthLog.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.6: `handleLastLoginInfo` (LAST_LOGIN_INFO, opcode 2054)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers.go` (dispatch entry)
- Test: `pkg/script/handlers_player_test.go`

- [ ] **Step 1: Write the failing test**

Add to `pkg/script/handlers_player_test.go`:

```go
// TestHandleLastLoginInfo pins the single-delegation pattern. No pop,
// no push — handler calls Self.LastLoginInfo and returns. Mirrors TS
// PlayerOps.ts:931-933.
func TestHandleLastLoginInfo(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:     mp,
		Pointers: PtrActivePlayer,
	}
	if err := handleLastLoginInfo(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.lastLoginInfoCalls != 1 {
		t.Errorf("LastLoginInfo: got %d calls, want 1", mp.lastLoginInfoCalls)
	}
}

// TestHandleLastLoginInfo_NoActivePlayer is folded into the existing
// TestHandlersRequireActivePlayer table — add OpLastLoginInfo as a
// new row.
```

Then locate `TestHandlersRequireActivePlayer` (run `rg -nB1 -A5 "TestHandlersRequireActivePlayer" pkg/script/handlers_player_test.go`) and add to the table:

```go
{op: OpLastLoginInfo, name: "LAST_LOGIN_INFO", handler: handleLastLoginInfo},
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleLastLoginInfo -v
```

Expected: FAIL — `handleLastLoginInfo undefined`.

- [ ] **Step 3: Implement the handler**

Add to `pkg/script/handlers_player.go` (locate the player-handlers grouping):

```go
// handleLastLoginInfo (LAST_LOGIN_INFO, opcode 2054). Single delegation
// to (*Player).LastLoginInfo. Mirrors TS PlayerOps.ts:931-933.
func handleLastLoginInfo(s *ScriptState) error {
	if err := requireActivePlayer(s, "LAST_LOGIN_INFO"); err != nil {
		return err
	}
	s.Self.LastLoginInfo()
	return nil
}
```

- [ ] **Step 4: Add dispatch entry to `pkg/script/handlers.go`**

```go
	OpLastLoginInfo: handleLastLoginInfo,
```

- [ ] **Step 5: Run tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleLastLoginInfo -v && \
  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandlersRequireActivePlayer -v
```

Expected: both PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B1.6 — LAST_LOGIN_INFO handler (opcode 2054)

Single delegation to Self.LastLoginInfo. Mirrors TS PlayerOps.ts:931-933.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.7: `handleInvTotalParamStack` (INV_TOTALPARAM_STACK, opcode 4329)

**Files:**
- Modify: `pkg/script/handlers_inv.go`
- Modify: `pkg/script/handlers.go`
- Test: `pkg/script/handlers_inv_test.go`

- [ ] **Step 1: Write the failing test**

Add to `pkg/script/handlers_inv_test.go`:

```go
// TestHandleInvTotalParamStack pins the pop-order [param, inv] and
// the pushInt of the delegated sum. Mirrors TS InvOps.ts:792-796.
func TestHandleInvTotalParamStack(t *testing.T) {
	mp := &mockPlayer{invTotalParamStackReturn: 42}
	s := &ScriptState{
		Self:          mp,
		Pointers:      PtrActivePlayer,
		StackCapacity: 4,
	}
	// TS popInts(2) → [inv, param], so push order is inv first then param.
	s.PushInt(5)  // inv
	s.PushInt(7)  // param

	if err := handleInvTotalParamStack(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mp.invTotalParamStackArgs) != 1 {
		t.Fatalf("delegate: got %d calls, want 1", len(mp.invTotalParamStackArgs))
	}
	got := mp.invTotalParamStackArgs[0]
	if got.InvID != 5 || got.ParamID != 7 {
		t.Errorf("delegate args: got %+v, want {InvID:5, ParamID:7}", got)
	}
	if v := s.PopInt(); v != 42 {
		t.Errorf("pushed: got %d, want 42", v)
	}
}
```

Plan executor: confirm `StackCapacity` initialization style matches existing tests (run `rg -n "StackCapacity" pkg/script/handlers_inv_test.go`). The fixture above assumes the existing pattern.

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleInvTotalParamStack -v
```

Expected: FAIL — `handleInvTotalParamStack undefined`.

- [ ] **Step 3: Implement the handler**

Add to `pkg/script/handlers_inv.go`:

```go
// handleInvTotalParamStack (INV_TOTALPARAM_STACK, opcode 4329). Pops
// param then inv (LIFO; TS popInts(2) → [inv, param]). Delegates to
// Self.InvTotalParamStack and pushes the result. Mirrors TS
// InvOps.ts:792-796.
func handleInvTotalParamStack(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_TOTALPARAM_STACK"); err != nil {
		return err
	}
	param := s.PopInt()
	inv := s.PopInt()
	s.PushInt(s.Self.InvTotalParamStack(inv, param))
	return nil
}
```

- [ ] **Step 4: Add dispatch entry to `pkg/script/handlers.go`**

```go
	OpInvTotalParamStack: handleInvTotalParamStack,
```

- [ ] **Step 5: Run test**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleInvTotalParamStack -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go pkg/script/handlers.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B1.7 — INV_TOTALPARAM_STACK handler (opcode 4329)

Pops [param, inv] (LIFO) and delegates to Self.InvTotalParamStack.
Mirrors TS InvOps.ts:792-796.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.8: `handleMapIndoors` (MAP_INDOORS, opcode 1010)

**Files:**
- Create or modify: `pkg/script/handlers_server.go`
- Modify: `pkg/script/handlers.go`
- Create or modify: `pkg/script/handlers_server_test.go`

- [ ] **Step 1: Verify file existence**

Run:
```bash
ls pkg/script/handlers_server.go 2>/dev/null && echo "exists" || echo "create"
```

If absent: this task creates it. Use the same package + import style as `pkg/script/handlers_player.go`.

- [ ] **Step 2: Write the failing test**

Create or add to `pkg/script/handlers_server_test.go`:

```go
package script

import "testing"

// TestHandleMapIndoors_True pins the pushInt(1) path: IsIndoors
// returns true for the popped coord.
func TestHandleMapIndoors_True(t *testing.T) {
	mw := &mockWorld{isIndoorsReturn: true}
	s := &ScriptState{
		World:         mw,
		StackCapacity: 4,
	}
	// CoordPack: assume coord 12345 encodes a valid (x,z,level) per
	// existing checkCoord helper. Plan executor confirms test fixture.
	s.PushInt(packTestCoord(0, 3200, 3200))
	if err := handleMapIndoors(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v := s.PopInt(); v != 1 {
		t.Errorf("pushed: got %d, want 1", v)
	}
	if len(mw.isIndoorsArgs) != 1 {
		t.Errorf("IsIndoors call count: got %d, want 1", len(mw.isIndoorsArgs))
	}
}

// TestHandleMapIndoors_False pins pushInt(0).
func TestHandleMapIndoors_False(t *testing.T) {
	mw := &mockWorld{isIndoorsReturn: false}
	s := &ScriptState{World: mw, StackCapacity: 4}
	s.PushInt(packTestCoord(0, 3200, 3200))
	if err := handleMapIndoors(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v := s.PopInt(); v != 0 {
		t.Errorf("pushed: got %d, want 0", v)
	}
}

// TestHandleMapIndoors_InvalidCoord pins coord-validation error path.
func TestHandleMapIndoors_InvalidCoord(t *testing.T) {
	mw := &mockWorld{}
	s := &ScriptState{World: mw, StackCapacity: 4}
	s.PushInt(-1) // invalid coord
	err := handleMapIndoors(s)
	if err == nil {
		t.Fatal("expected error on invalid coord")
	}
}
```

Plan executor: substitute `packTestCoord` with the helper used by existing coord-test fixtures (likely defined in `handlers_test.go` or similar). Per memory `plan_grep_helper_patterns.md`.

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleMapIndoors -v
```

Expected: FAIL.

- [ ] **Step 4: Implement the handler**

Create or add to `pkg/script/handlers_server.go`:

```go
package script

import "fmt"

// handleMapIndoors (MAP_INDOORS, opcode 1010). Pops a coord, validates,
// and pushes 1 if the tile carries the roof flag else 0. Mirrors TS
// ServerOps.ts:139-143.
func handleMapIndoors(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_INDOORS: no world surface")
	}
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "MAP_INDOORS")
	if err != nil {
		return err
	}
	if s.World.IsIndoors(x, z, level) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 5: Add dispatch entry to `pkg/script/handlers.go`**

```go
	OpMapIndoors: handleMapIndoors,
```

- [ ] **Step 6: Add `(*Server).IsIndoors` adapter in `modules/world`**

The `WorldSurface` interface gained `IsIndoors`; the concrete `*Server` must implement it. Add to `modules/world/server.go` (or `world_zone.go`):

```go
// IsIndoors reports whether (x, z, level) carries the roof flag in
// the global FlagMap. Implements pkg/script.WorldSurface.IsIndoors.
// Mirrors TS isIndoors (GameMap.ts:417-419).
func (s *Server) IsIndoors(x, z, level int) bool {
	flag := s.flagMap.Get(x, z, level) // plan executor confirms accessor name
	return collision.IsIndoors(flag)
}
```

Plan executor: confirm the flagmap accessor — grep `func.*FlagMap.*Get\b|flagMap\.` in `modules/world/server.go`. Type of `flag` must match `collision.IsIndoors` signature.

- [ ] **Step 7: Run tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleMapIndoors -v && \
  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: PASS + build OK.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_server.go pkg/script/handlers_server_test.go pkg/script/handlers.go modules/world/server.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B1.8 — MAP_INDOORS handler (opcode 1010)

Pops coord, validates, delegates to World.IsIndoors → pushes 1/0.
Mirrors TS ServerOps.ts:139-143.
(*Server).IsIndoors wraps collision.IsIndoors against the global
FlagMap.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.9: `handleNpcStatHeal` (NPC_STATHEAL, opcode 2539)

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers.go`
- Test: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Read TS body**

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  sed -n '241,257p' src/engine/script/handlers/NpcOps.ts
```

Confirm the arithmetic:
```
const [stat, constant, percent] = state.popInts(3);
const base = npc.baseLevels[stat];
const current = npc.levels[stat];
const healed = current + ((constant + (base * percent) / 100) | 0);
npc.levels[stat] = Math.min(healed, base);
if (stat === NpcStat.HITPOINTS && npc.levels[stat] >= npc.baseLevels[stat]) {
    npc.heroPoints.clear();
}
```

- [ ] **Step 2: Write the failing tests**

Add to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcStatHeal_PartialHeal pins the heal formula and the
// cap at baseLevels[stat]. Stat=Attack (non-HP), base=10, current=2,
// constant=3, percent=50 → healed = 2 + (3 + 10*50/100) = 10.
func TestHandleNpcStatHeal_PartialHeal(t *testing.T) {
	mn := newTestMockNpc(t)
	mn.baseLevels[objtype.NpcStatAttack] = 10
	mn.levels[objtype.NpcStatAttack] = 2

	s := &ScriptState{
		ActiveNpc:     mn,
		Pointers:      PtrActiveNpc,
		StackCapacity: 8,
	}
	// LIFO push order; TS popInts(3) → [stat, constant, percent]
	s.PushInt(objtype.NpcStatAttack) // stat
	s.PushInt(3)                     // constant
	s.PushInt(50)                    // percent

	if err := handleNpcStatHeal(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mn.levels[objtype.NpcStatAttack]; got != 10 {
		t.Errorf("levels[Attack]: got %d, want 10", got)
	}
	if mn.heroPointsClearCalls != 0 {
		t.Errorf("heroPointsClear: got %d calls, want 0 (Attack stat)", mn.heroPointsClearCalls)
	}
}

// TestHandleNpcStatHeal_HpFullClearsHeroPoints pins the HP-full
// branch. base=20, current=18, constant=5, percent=50 → healed=33;
// min(33, 20) = 20 (full HP). Clears HeroPoints.
func TestHandleNpcStatHeal_HpFullClearsHeroPoints(t *testing.T) {
	mn := newTestMockNpc(t)
	mn.baseLevels[objtype.NpcStatHitpoints] = 20
	mn.levels[objtype.NpcStatHitpoints] = 18

	s := &ScriptState{
		ActiveNpc:     mn,
		Pointers:      PtrActiveNpc,
		StackCapacity: 8,
	}
	s.PushInt(objtype.NpcStatHitpoints)
	s.PushInt(5)
	s.PushInt(50)

	if err := handleNpcStatHeal(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mn.levels[objtype.NpcStatHitpoints]; got != 20 {
		t.Errorf("levels[Hitpoints]: got %d, want 20", got)
	}
	if mn.heroPointsClearCalls != 1 {
		t.Errorf("heroPointsClear: got %d calls, want 1", mn.heroPointsClearCalls)
	}
}

// TestHandleNpcStatHeal_HpHealsButNotFull pins that HeroPoints stays
// untouched when HP doesn't reach base. base=20, current=10, constant=2,
// percent=10 → healed = 10 + (2 + 2) = 14 (below base).
func TestHandleNpcStatHeal_HpHealsButNotFull(t *testing.T) {
	mn := newTestMockNpc(t)
	mn.baseLevels[objtype.NpcStatHitpoints] = 20
	mn.levels[objtype.NpcStatHitpoints] = 10

	s := &ScriptState{
		ActiveNpc:     mn,
		Pointers:      PtrActiveNpc,
		StackCapacity: 8,
	}
	s.PushInt(objtype.NpcStatHitpoints)
	s.PushInt(2)
	s.PushInt(10)

	if err := handleNpcStatHeal(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mn.levels[objtype.NpcStatHitpoints]; got != 14 {
		t.Errorf("levels[Hitpoints]: got %d, want 14", got)
	}
	if mn.heroPointsClearCalls != 0 {
		t.Errorf("heroPointsClear: got %d calls, want 0 (HP not full)", mn.heroPointsClearCalls)
	}
}

// TestHandleNpcStatHeal_NonHpStatNeverClears: even when stat reaches
// base, only HITPOINTS gates HeroPoints.Clear().
func TestHandleNpcStatHeal_NonHpStatNeverClears(t *testing.T) {
	mn := newTestMockNpc(t)
	mn.baseLevels[objtype.NpcStatStrength] = 10
	mn.levels[objtype.NpcStatStrength] = 5

	s := &ScriptState{
		ActiveNpc:     mn,
		Pointers:      PtrActiveNpc,
		StackCapacity: 8,
	}
	s.PushInt(objtype.NpcStatStrength)
	s.PushInt(10) // constant=10 fully heals
	s.PushInt(0)

	if err := handleNpcStatHeal(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mn.levels[objtype.NpcStatStrength]; got != 10 {
		t.Errorf("levels[Strength]: got %d, want 10", got)
	}
	if mn.heroPointsClearCalls != 0 {
		t.Errorf("heroPointsClear: got %d calls, want 0 (non-HP)", mn.heroPointsClearCalls)
	}
}
```

Plan executor: confirm `newTestMockNpc` helper name and `mn.baseLevels` / `mn.levels` field names against existing test infra. If they differ, adjust the test fixture accordingly. Confirm `objtype.NpcStatAttack`, `objtype.NpcStatHitpoints`, `objtype.NpcStatStrength` constant names in pkg/objtype.

- [ ] **Step 3: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleNpcStatHeal -v
```

Expected: FAIL — `handleNpcStatHeal undefined`.

- [ ] **Step 4: Implement the handler**

Add to `pkg/script/handlers_npc.go`:

```go
// handleNpcStatHeal (NPC_STATHEAL, opcode 2539). Heals the active NPC's
// `stat` by `constant + (base*percent/100)`, capped at base. When HP
// reaches base, clears the NPC's HeroPoints ledger. Mirrors TS
// NpcOps.ts:241-257.
//
// Pop order (LIFO): percent, constant, stat ← TS popInts(3) returns
// [stat, constant, percent].
func handleNpcStatHeal(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STATHEAL"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	stat := s.PopInt()

	if err := checkNpcStat(stat, "NPC_STATHEAL"); err != nil {
		return err
	}
	if err := checkNumberNotNull(constant, "NPC_STATHEAL"); err != nil {
		return err
	}
	if err := checkNumberNotNull(percent, "NPC_STATHEAL"); err != nil {
		return err
	}

	base := s.ActiveNpc.BaseLevel(stat)
	current := s.ActiveNpc.Level(stat)
	healed := current + (constant + (base*percent)/100) // TS `| 0` ≡ Go int truncation
	if healed > base {
		healed = base
	}
	s.ActiveNpc.SetLevel(stat, healed)

	if stat == objtype.NpcStatHitpoints && healed >= base {
		s.ActiveNpc.HeroPointsClear()
	}
	return nil
}
```

Plan executor: verify these helper functions and method names against existing patterns:
- `requireActiveNpc(s, "OP")` — grep `^func requireActiveNpc` in `pkg/script/handlers_npc.go`.
- `checkNpcStat(stat, "OP")` — grep `func checkNpcStat`.
- `checkNumberNotNull(n, "OP")` — grep `func checkNumberNotNull` or `func checkNotNull`.
- `s.ActiveNpc.BaseLevel(stat)`, `.Level(stat)`, `.SetLevel(stat, v)` — grep the existing `ActiveNpc` interface. If the interface exposes the levels arrays differently (e.g., `BaseLevels()` returning a slice), adjust accordingly. Per memory `plan_grep_helper_patterns.md` and `audit_full_method_against_ts.md`.

- [ ] **Step 5: Add dispatch entry to `pkg/script/handlers.go`**

```go
	OpNpcStatHeal: handleNpcStatHeal,
```

- [ ] **Step 6: Run tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleNpcStatHeal -v
```

Expected: 4 PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go pkg/script/handlers.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B1.9 — NPC_STATHEAL handler (opcode 2539)

Heals NPC stat by constant + (base*percent/100), caps at base. HP-full
branch clears HeroPoints. Mirrors TS NpcOps.ts:241-257.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.10: B1 close — audit recount + bundle close commit

- [ ] **Step 1: Run full test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 2: Re-run missing-handler audit**

```bash
awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/claude/handled.txt && \
  awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/claude/declared.txt && \
  comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l
```

Expected: `8`.

- [ ] **Step 3: Bundle close commit (empty if all per-task commits cover the work)**

If there are no untracked files / staged changes, skip the empty commit and reference recount in B2's first commit. Otherwise:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-162 B1 — trivial+small bundle landed (12→8)

4 prerequisite methods/helpers + 4 handlers ported:
  - (*HeroPoints).Clear() [B1.1]
  - collision.IsIndoors [B1.2]
  - (*Player).LastLoginInfo [B1.3, stub per NAI-162-D-LASTLOGIN-NO-PACKET]
  - (*Player).InvTotalParamStack [B1.4]
  - interface wideners + mocks [B1.5]
  - handleLastLoginInfo (2054) [B1.6]
  - handleInvTotalParamStack (4329) [B1.7]
  - handleMapIndoors (1010) [B1.8]
  - handleNpcStatHeal (2539) [B1.9]

Missing-handler audit: 12 → 8.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle B2 — WealthEvent + player-interaction + NAI-115-D1 retirement

**Goal:** Land WealthEvent subsystem (struct + AddWealthEvent + ObjTypes.ByName), 3 handlers (WealthEvent, PLocMerge, POpPlayerT), and retire NAI-115-D1 (6 sites). Audit recount 8 → 5.

### Task B2.1: `(*ObjTypes).ByName(name string) *ObjType` lookup

**Files:**
- Modify: `pkg/objtype/objtype.go` (or wherever ObjTypes is defined)
- Test: `pkg/objtype/objtype_test.go`

- [ ] **Step 1: Locate the ObjTypes container**

Run:
```bash
rg -nB1 -A5 "type ObjTypes struct|^type ObjType struct" pkg/objtype/
```

Record the container's field names. ObjTypes likely has `Configs []*ObjType` and a `byName map[string]*ObjType` index OR builds a temporary map.

- [ ] **Step 2: Write the failing test**

Add to `pkg/objtype/objtype_test.go`:

```go
// TestObjTypes_ByName pins (*ObjTypes).ByName lookup by debugname.
// Mirrors TS ObjType.getByName.
func TestObjTypes_ByName(t *testing.T) {
	cfg := newTestObjTypes(t, map[int]string{
		558:  "mind_rune",
		4151: "abyssal_whip",
	})
	got := cfg.ByName("abyssal_whip")
	if got == nil {
		t.Fatal("ByName(\"abyssal_whip\"): got nil, want id=4151")
	}
	if got.ID != 4151 {
		t.Errorf("got ID=%d, want 4151", got.ID)
	}
}

// TestObjTypes_ByName_Unknown pins nil return.
func TestObjTypes_ByName_Unknown(t *testing.T) {
	cfg := newTestObjTypes(t, map[int]string{558: "mind_rune"})
	if got := cfg.ByName("unknown_obj"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
```

Plan executor: confirm or write `newTestObjTypes(t, map[int]string)` helper if absent.

- [ ] **Step 3: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype -run TestObjTypes_ByName -v
```

Expected: FAIL.

- [ ] **Step 4: Implement ByName**

Add to `pkg/objtype/objtype.go`:

```go
// ByName returns the ObjType matching the given debugname, or nil if
// no match exists. Mirrors TS ObjType.getByName. O(N) scan; if call
// volume warrants, switch to a name→ID index built at load time.
func (cfg *ObjTypes) ByName(name string) *ObjType {
	for _, c := range cfg.Configs {
		if c != nil && c.DebugName == name {
			return c
		}
	}
	return nil
}
```

Plan executor: replace `cfg.Configs` with the actual field name from Step 1.

- [ ] **Step 5: Run test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype -run TestObjTypes_ByName -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/objtype.go pkg/objtype/objtype_test.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-162 B2.1 — (*ObjTypes).ByName lookup

Returns the ObjType matching debugname, or nil. Mirrors TS
ObjType.getByName. Prerequisite for B2.5 WEALTH_EVENT handler.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B2.2: `(*Player).AddWealthEvent` real body + `wealthLog` field

Replaces the placeholder stub from B1.5.

**Files:**
- Modify: `modules/world/player.go` (add field)
- Modify: `modules/world/player_script.go` (real body)
- Test: `modules/world/player_script_test.go`

- [ ] **Step 1: Add `wealthLog` field to Player**

Edit `modules/world/player.go` — locate the Player struct and add:

```go
	// wealthLog is the in-memory append-only log of wealth events.
	// NAI-162 B2; NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY (analytics
	// RPC deferred).
	wealthLog []script.WealthEvent
```

- [ ] **Step 2: Write the failing test**

Add to `modules/world/player_script_test.go`:

```go
// TestPlayer_AddWealthEvent pins append-to-log behaviour. Two events
// append to the log in order.
func TestPlayer_AddWealthEvent(t *testing.T) {
	p := &Player{}
	e1 := script.WealthEvent{EventType: script.WealthEventTypeDrop, AccountValue: 1000}
	e2 := script.WealthEvent{EventType: script.WealthEventTypePVP, AccountValue: 5000}
	p.AddWealthEvent(e1)
	p.AddWealthEvent(e2)
	if got := len(p.wealthLog); got != 2 {
		t.Fatalf("len(wealthLog): got %d, want 2", got)
	}
	if p.wealthLog[0].AccountValue != 1000 || p.wealthLog[1].AccountValue != 5000 {
		t.Errorf("wealthLog values: got %v", p.wealthLog)
	}
}
```

- [ ] **Step 3: Run test (B1.5 stub returns success but produces no append)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestPlayer_AddWealthEvent -v
```

Expected: FAIL — `len(wealthLog) == 0`.

- [ ] **Step 4: Replace stub with real body**

Edit `modules/world/player_script.go` — replace the B1.5 placeholder with:

```go
// AddWealthEvent appends `evt` to this player's in-memory wealth log.
// Mirrors TS Player.addWealthEvent. Per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY,
// goscape does not emit an analytics RPC; the log is a queryable
// in-memory record only.
func (p *Player) AddWealthEvent(evt script.WealthEvent) {
	p.wealthLog = append(p.wealthLog, evt)
}
```

- [ ] **Step 5: Run test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestPlayer_AddWealthEvent -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player.go modules/world/player_script.go modules/world/player_script_test.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-162 B2.2 — (*Player).AddWealthEvent real body

Replaces B1.5 stub with append-to-wealthLog. NAI-162-D-WEALTHEVENT-
IN-MEMORY-ONLY: analytics RPC deferred.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B2.3: `handleWealthEvent` (WEALTH_EVENT, opcode 2131)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers.go`
- Test: `pkg/script/handlers_player_test.go`

- [ ] **Step 1: Re-read TS body**

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  sed -n '1191,1202p' src/engine/script/handlers/PlayerOps.ts
```

Note pop order: `popString(name)` first, then `popInts(3) → [eventType, count, value]`. Combined LIFO push order: `name → eventType → count → value` (we pop `value, count, eventType, name`).

- [ ] **Step 2: Write the failing tests**

Add to `pkg/script/handlers_player_test.go`:

```go
// TestHandleWealthEvent_KnownObj pins the happy path: ObjTypes.ByName
// resolves; AddWealthEvent called with assembled struct.
func TestHandleWealthEvent_KnownObj(t *testing.T) {
	mp := &mockPlayer{}
	mc := newTestConfigs(t, withObjByName("abyssal_whip", &objtype.ObjType{ID: 4151, DebugName: "abyssal_whip"}))
	s := &ScriptState{
		Self:          mp,
		Configs:       mc,
		Pointers:      PtrActivePlayer,
		StackCapacity: 8,
	}
	// Push order matching TS popString(name) + popInts(3) →
	// [eventType, count, value]. Goscape LIFO: pop value, count,
	// eventType, name. Push: name, eventType, count, value.
	s.PushString("abyssal_whip")
	s.PushInt(WealthEventTypeDrop) // eventType
	s.PushInt(1)                   // count
	s.PushInt(120000)              // value

	if err := handleWealthEvent(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1", len(mp.addWealthEventCalls))
	}
	got := mp.addWealthEventCalls[0]
	if got.EventType != WealthEventTypeDrop {
		t.Errorf("EventType: got %d, want %d", got.EventType, WealthEventTypeDrop)
	}
	if got.AccountValue != 120000 {
		t.Errorf("AccountValue: got %d, want 120000", got.AccountValue)
	}
	if len(got.AccountItems) != 1 ||
		got.AccountItems[0].ID != 4151 ||
		got.AccountItems[0].Name != "abyssal_whip" ||
		got.AccountItems[0].Count != 1 {
		t.Errorf("AccountItems: got %+v, want [{ID:4151 Name:abyssal_whip Count:1}]", got.AccountItems)
	}
}

// TestHandleWealthEvent_UnknownObj pins ObjTypes.ByName→nil path:
// AccountItems[0].ID == -1 (TS `objType?.id` undefined ≡ goscape -1).
func TestHandleWealthEvent_UnknownObj(t *testing.T) {
	mp := &mockPlayer{}
	mc := newTestConfigs(t /* no obj-by-name fixture */)
	s := &ScriptState{
		Self:          mp,
		Configs:       mc,
		Pointers:      PtrActivePlayer,
		StackCapacity: 8,
	}
	s.PushString("unknown_obj")
	s.PushInt(WealthEventTypeDrop)
	s.PushInt(1)
	s.PushInt(0)

	if err := handleWealthEvent(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1", len(mp.addWealthEventCalls))
	}
	got := mp.addWealthEventCalls[0]
	if len(got.AccountItems) != 1 || got.AccountItems[0].ID != -1 {
		t.Errorf("AccountItems[0].ID: got %v, want -1", got.AccountItems)
	}
}
```

Plan executor: confirm `newTestConfigs` helper API and `withObjByName(...)` option-builder pattern against existing test infra (run `rg -nB1 -A3 "func newTestConfigs" pkg/script/`). If a different helper exists, adapt.

- [ ] **Step 3: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleWealthEvent -v
```

Expected: FAIL — `handleWealthEvent undefined`.

- [ ] **Step 4: Implement the handler**

Add to `pkg/script/handlers_player.go`:

```go
// handleWealthEvent (WEALTH_EVENT, opcode 2131). Pops a string name,
// then 3 ints (LIFO: value, count, eventType matching TS popInts(3) →
// [eventType, count, value]). Resolves the obj via Configs.ObjByName;
// missing name → id=-1 (matches TS `objType?.id` undefined). Calls
// Self.AddWealthEvent. Mirrors TS PlayerOps.ts:1191-1202.
func handleWealthEvent(s *ScriptState) error {
	if err := requireActivePlayer(s, "WEALTH_EVENT"); err != nil {
		return err
	}
	value := s.PopInt()
	count := s.PopInt()
	eventType := s.PopInt()
	name := s.PopString()

	objID := -1
	if s.Configs != nil {
		if t := s.Configs.ObjByName(name); t != nil {
			objID = t.ID
		}
	}
	s.Self.AddWealthEvent(WealthEvent{
		EventType:    int(eventType),
		AccountItems: []WealthItem{{ID: objID, Name: name, Count: int(count)}},
		AccountValue: int(value),
	})
	return nil
}
```

Plan executor: the `Configs` interface needs an `ObjByName(name string) *objtype.ObjType` method to surface the lookup. If it doesn't, widen the interface — search `type Configs interface` in `pkg/script/state.go` and add:

```go
	// ObjByName resolves an obj by debugname for WEALTH_EVENT handler.
	// NAI-162 B2.
	ObjByName(name string) *objtype.ObjType
```

Then implement on the production Configs adapter in `modules/world/` (search `func.*Configs.*InvType\b` for the existing patterns) to delegate to `(*ObjTypes).ByName`.

- [ ] **Step 5: Add dispatch entry to `pkg/script/handlers.go`**

```go
	OpWealthEvent: handleWealthEvent,
```

- [ ] **Step 6: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleWealthEvent -v
```

Expected: 2 PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go pkg/script/state.go modules/world/ && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B2.3 — WEALTH_EVENT handler (opcode 2131)

Pops [value, count, eventType, name] (LIFO) → resolves obj via
Configs.ObjByName → calls Self.AddWealthEvent. Missing name → id=-1.
Configs interface widened with ObjByName. Mirrors TS
PlayerOps.ts:1191-1202.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B2.4: `handlePLocMerge` (P_LOCMERGE, opcode 2074)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers.go`
- Modify: `pkg/script/state.go` (widen WorldSurface with MergeLoc if absent)
- Test: `pkg/script/handlers_player_test.go`

- [ ] **Step 1: Verify MergeLoc accessor**

```bash
rg -n "MergeLoc\b" pkg/script/state.go modules/world/server.go modules/world/world_zone.go 2>/dev/null
```

If `World.MergeLoc` isn't on the WorldSurface interface, widen it:

```go
// WorldSurface (or equivalent) addition:
	MergeLoc(loc ActiveLoc, startCycle, endCycle, seZ, seX, nwZ, nwX int)
```

And ensure `(*Server).MergeLoc` matches this signature. Per memory `plan_sibling_site_guard_audit.md`.

- [ ] **Step 2: Read TS body**

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  sed -n '922,929p' src/engine/script/handlers/PlayerOps.ts
```

Pop order: TS `popInts(4) → [startCycle, endCycle, southEast, northWest]`. Goscape LIFO: northWest, southEast, endCycle, startCycle.

- [ ] **Step 3: Write the failing tests**

Add to `pkg/script/handlers_player_test.go`:

```go
// TestHandlePLocMerge_Happy pins the World.MergeLoc dispatch with
// argument unpacking from TS popInts(4) → [startCycle, endCycle, se, nw].
func TestHandlePLocMerge_Happy(t *testing.T) {
	mp := &mockPlayer{}
	mw := &mockWorld{}
	loc := mockActiveLoc(t /* setup helper */)
	s := &ScriptState{
		Self:          mp,
		World:         mw,
		ActiveLoc:     loc,
		Pointers:      PtrActivePlayer | PtrProtectedActivePlayer | PtrActiveLoc,
		StackCapacity: 8,
	}
	// LIFO push order: startCycle, endCycle, southEast, northWest
	s.PushInt(10)                      // startCycle
	s.PushInt(50)                      // endCycle
	s.PushInt(packTestCoord(0, 3200, 3200)) // southEast
	s.PushInt(packTestCoord(0, 3210, 3210)) // northWest

	if err := handlePLocMerge(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mw.mergeLocCalls) != 1 {
		t.Fatalf("MergeLoc: got %d calls, want 1", len(mw.mergeLocCalls))
	}
	got := mw.mergeLocCalls[0]
	if got.StartCycle != 10 || got.EndCycle != 50 {
		t.Errorf("cycles: got {%d,%d}, want {10,50}", got.StartCycle, got.EndCycle)
	}
	if got.SeX != 3200 || got.SeZ != 3200 || got.NwX != 3210 || got.NwZ != 3210 {
		t.Errorf("rect: got se(%d,%d) nw(%d,%d), want (3200,3200) (3210,3210)",
			got.SeX, got.SeZ, got.NwX, got.NwZ)
	}
}

// TestHandlePLocMerge_InvalidCoord
func TestHandlePLocMerge_InvalidCoord(t *testing.T) {
	mp := &mockPlayer{}
	mw := &mockWorld{}
	s := &ScriptState{
		Self:          mp,
		World:         mw,
		Pointers:      PtrActivePlayer | PtrProtectedActivePlayer | PtrActiveLoc,
		StackCapacity: 8,
	}
	s.PushInt(10)
	s.PushInt(50)
	s.PushInt(-1) // bad southEast
	s.PushInt(packTestCoord(0, 3210, 3210))

	if err := handlePLocMerge(s); err == nil {
		t.Fatal("expected error on invalid southEast coord")
	}
}

// TestHandlePLocMerge_NotProtected — folded into
// TestHandlersRequireProtectedActivePlayer table.
```

Plan executor: substitute `mockActiveLoc`, `mockWorld.mergeLocCalls`, and the `MergeLoc` record struct (needs fields StartCycle, EndCycle, SeX, SeZ, NwX, NwZ) per the existing mock patterns. Add the recorder fields to `mockWorld` as part of this task.

Then locate `TestHandlersRequireProtectedActivePlayer` table and add:

```go
{op: OpPLocMerge, name: "P_LOCMERGE", handler: handlePLocMerge},
```

- [ ] **Step 4: Run tests to verify failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandlePLocMerge -v
```

Expected: FAIL.

- [ ] **Step 5: Implement the handler**

Add to `pkg/script/handlers_player.go`:

```go
// handlePLocMerge (P_LOCMERGE, opcode 2074). Pops [northWest, southEast,
// endCycle, startCycle] (LIFO; TS popInts(4) → [startCycle, endCycle,
// southEast, northWest]). Validates both coords; delegates to
// World.MergeLoc with the active loc. Mirrors TS PlayerOps.ts:922-929.
func handlePLocMerge(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_LOCMERGE"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "P_LOCMERGE"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("P_LOCMERGE: no world surface")
	}
	northWest := s.PopInt()
	southEast := s.PopInt()
	endCycle := s.PopInt()
	startCycle := s.PopInt()

	seLevel, seX, seZ, err := checkCoord(southEast, "P_LOCMERGE")
	if err != nil {
		return err
	}
	_ = seLevel // matches TS use of se.z/se.x only
	nwLevel, nwX, nwZ, err := checkCoord(northWest, "P_LOCMERGE")
	if err != nil {
		return err
	}
	_ = nwLevel

	s.World.MergeLoc(s.ActiveLoc, startCycle, endCycle, seZ, seX, nwZ, nwX)
	return nil
}
```

Plan executor: confirm `requireActiveLoc` and `s.ActiveLoc` field names. Confirm `WorldSurface.MergeLoc` signature against `(*Server).MergeLoc` at `modules/world/world_zone.go:113`. If the concrete signature has additional args (e.g., the player as the wave-attribution argument), thread them.

- [ ] **Step 6: Add dispatch entry**

```go
	OpPLocMerge: handlePLocMerge,
```

- [ ] **Step 7: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandlePLocMerge -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go pkg/script/state.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B2.4 — P_LOCMERGE handler (opcode 2074)

Protected gate; pops [nwX, seX, endCycle, startCycle] (LIFO);
delegates to World.MergeLoc. Mirrors TS PlayerOps.ts:922-929.
WorldSurface widened with MergeLoc.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B2.5: `handlePOpPlayerT` (P_OPPLAYERT, opcode 2082)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers.go`
- Test: `pkg/script/handlers_player_test.go`

- [ ] **Step 1: Re-read TS**

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  sed -n '1127,1135p' src/engine/script/handlers/PlayerOps.ts
```

Note silent return on `!target` (TS lines 1130-1132).

- [ ] **Step 2: Write the failing tests**

Add to `pkg/script/handlers_player_test.go`:

```go
// TestHandlePOpPlayerT_Happy: protected gate set, Self2 present, spell
// id pops, StopAction + SetInteractionScriptPlayer fire.
func TestHandlePOpPlayerT_Happy(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:          mp,
		Self2:         mp2,
		Pointers:      PtrActivePlayer | PtrProtectedActivePlayer | PtrActivePlayer2,
		StackCapacity: 4,
	}
	s.PushInt(1234) // spellId

	if err := handlePOpPlayerT(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("StopAction: got %d calls, want 1", mp.stopActionCalls)
	}
	if len(mp.setInteractionScriptPlayerCalls) != 1 {
		t.Fatalf("SetInteractionScriptPlayer: got %d, want 1", len(mp.setInteractionScriptPlayerCalls))
	}
	got := mp.setInteractionScriptPlayerCalls[0]
	if got.Target != mp2 || got.Op != 1234 {
		t.Errorf("call args: got {%v %d}, want {%v 1234}", got.Target, got.Op, mp2)
	}
}

// TestHandlePOpPlayerT_NilSelf2 pins TS lines 1130-1132 silent return:
// no error, no StopAction, no SetInteraction.
func TestHandlePOpPlayerT_NilSelf2(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:          mp,
		Self2:         nil,
		Pointers:      PtrActivePlayer | PtrProtectedActivePlayer, // no PtrActivePlayer2
		StackCapacity: 4,
	}
	s.PushInt(1234)

	if err := handlePOpPlayerT(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.stopActionCalls != 0 {
		t.Errorf("StopAction: got %d, want 0 (silent return)", mp.stopActionCalls)
	}
	if len(mp.setInteractionScriptPlayerCalls) != 0 {
		t.Errorf("SetInteraction calls: got %d, want 0", len(mp.setInteractionScriptPlayerCalls))
	}
}

// TestHandlePOpPlayerT_NotProtected — folded into
// TestHandlersRequireProtectedActivePlayer table.
```

Plan executor: ensure `mp.stopActionCalls` and `mp.setInteractionScriptPlayerCalls` fields exist on `mockPlayer`; if not, add them in this task (per memory `mock_recorder_field_naming_check.md`).

Add to the `TestHandlersRequireProtectedActivePlayer` table:

```go
{op: OpPOpPlayerT, name: "P_OPPLAYERT", handler: handlePOpPlayerT},
```

- [ ] **Step 3: Run tests to verify failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandlePOpPlayerT -v
```

Expected: FAIL.

- [ ] **Step 4: Implement the handler**

Add to `pkg/script/handlers_player.go`:

```go
// handlePOpPlayerT (P_OPPLAYERT, opcode 2082). Protected gate. Pops
// spellId. If Self2 absent, silently returns (TS lines 1130-1132).
// Otherwise StopAction + SetInteractionScriptPlayer(Self2, spellId).
// Mirrors TS PlayerOps.ts:1127-1135.
func handlePOpPlayerT(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPPLAYERT"); err != nil {
		return err
	}
	spellID := s.PopInt()
	if err := checkNumberNotNull(spellID, "P_OPPLAYERT"); err != nil {
		return err
	}
	if s.Self2 == nil || s.Pointers&PtrActivePlayer2 == 0 {
		return nil // silent — TS PlayerOps.ts:1130-1132
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptPlayer(s.Self2, int(spellID))
	return nil
}
```

Plan executor: confirm `StopAction` and `SetInteractionScriptPlayer` are on `ActivePlayer` interface. SetInteractionScriptPlayer at handlers_player.go:1409 was an existing call site — confirm the method exists on the interface side too. If absent: widen the interface in this task (add to B1.5 retroactively).

- [ ] **Step 5: Add dispatch entry**

```go
	OpPOpPlayerT: handlePOpPlayerT,
```

- [ ] **Step 6: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandlePOpPlayerT -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B2.5 — P_OPPLAYERT handler (opcode 2082)

Protected gate. Pops spellId. Silent return when Self2 absent (TS
PlayerOps.ts:1130-1132). Otherwise StopAction + SetInteractionScript
Player(Self2, spellId). Mirrors TS PlayerOps.ts:1127-1135.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B2.6: Retire NAI-115-D1 deviation (6 sites)

**Files:**
- Modify: `pkg/script/handlers_inv.go` (5 sites)
- Modify: `pkg/script/handlers_obj.go` (1 site)
- Modify: `pkg/script/handlers_inv_test.go` (test-expectation updates)

- [ ] **Step 1: Enumerate all sites**

Run:
```bash
rg -n "NAI-115-D1" pkg/ modules/ cmd/
```

Record every hit. Brainstorm baseline (HEAD `f38fc3e`):
- `pkg/script/handlers_inv.go:776, 784-786, 852-853, 1245-1248, 1356`
- `pkg/script/handlers_obj.go:217`

If new sites appear (e.g., in test files), include them.

- [ ] **Step 2: For each handlers_inv.go site, replace skip-block with emit-call**

For each SCOPE_PERM branch currently guarded with `// NAI-115-D1: TS calls addWealthEvent here for SCOPE_PERM. Skipped.`, replace with the inline call. Example transformation at handlers_inv.go:852:

Before:
```go
	// NAI-115-D1: TS calls addWealthEvent here for SCOPE_PERM. Skipped.
	// (goscape: content can emit via OpWealthEvent 2131.)
	// (no code here)
```

After:
```go
	// TS-faithful per InvOps.ts:445-494: emit addWealthEvent for
	// SCOPE_PERM drop. NAI-115-D1 retired at NAI-162 B2.
	if invType.Scope == objtype.InvTypeScopePerm {
		objType := s.Configs.ObjType(obj.ID)
		debugName := ""
		if objType != nil {
			debugName = objType.DebugName
		}
		cost := 0
		if objType != nil {
			cost = objType.Cost
		}
		s.Self.AddWealthEvent(WealthEvent{
			EventType:    WealthEventTypeDrop,
			AccountItems: []WealthItem{{ID: obj.ID, Name: debugName, Count: obj.Count}},
			AccountValue: obj.Count * cost,
		})
	}
```

Plan executor: at each site, READ the TS counterpart for the exact `eventType` value (Drop vs Death vs PVP per TS WealthEventType enum), the `AccountValue` formula, and any `RecipientSession` field. Each of the 5 inv-go sites and 1 obj-go site may map to a different TS event_type per the inv-flow semantics — don't paste the same body to all 6.

For the 4 deviation-comment-only block sites (handlers_inv.go:1245-1248 etc.), strip the comment block entirely.

- [ ] **Step 3: Update existing inv-test expectations**

Run:
```bash
rg -n "addWealthEventCalls|wealthLog\b" pkg/script/handlers_inv_test.go pkg/script/handlers_obj_test.go
```

For each test that currently asserts `addWealthEventCalls == 0` on a SCOPE_PERM path, flip the expectation to `len(addWealthEventCalls) == 1` AND assert the event shape (EventType, AccountItems, AccountValue).

Per memory `audit_full_method_against_ts.md`: per test, re-read the TS source to confirm the expected event shape.

- [ ] **Step 4: Write the binding retirement test**

Add to `pkg/script/handlers_inv_test.go`:

```go
// TestNAI115D1Retirement_InvDropSlotScopePermEmitsWealthEvent pins
// the behaviour flip: pre-NAI-162, INV_DROPSLOT on a SCOPE_PERM
// inv skipped AddWealthEvent (NAI-115-D1 deviation). Post-NAI-162
// B2.6, the path emits. Mirrors TS InvOps.ts:445-494.
func TestNAI115D1Retirement_InvDropSlotScopePermEmitsWealthEvent(t *testing.T) {
	mp := &mockPlayer{}
	mc := newTestConfigs(t,
		withInvType(/* protect=true, scope=ScopePerm */),
		withObjType(/* objID, debugname, cost */),
	)
	mw := &mockWorld{}
	s := &ScriptState{
		Self:          mp,
		Configs:       mc,
		World:         mw,
		Pointers:      PtrActivePlayer | PtrProtectedActivePlayer,
		StackCapacity: 8,
	}
	// setup: place obj in slot 0 of fixture inv
	// pushed args: duration, slot, coord, invID (LIFO matches TS popInts(4))
	s.PushInt(5)                            // invID
	s.PushInt(packTestCoord(0, 3200, 3200)) // coord
	s.PushInt(0)                            // slot
	s.PushInt(50)                           // duration

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d, want 1 (retirement)", len(mp.addWealthEventCalls))
	}
}
```

Plan executor: fill in the `withInvType` / `withObjType` fixture details and the slot setup using the existing test-helper API.

- [ ] **Step 5: Run all inv + obj tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run "TestHandleInv|TestNAI115D1" -v
```

Expected: all PASS, including the new retirement test.

- [ ] **Step 6: Final retirement audit**

```bash
rg -n "NAI-115-D1" pkg/ modules/ cmd/
```

Expected: only mentions are in close-commit-message text in this plan + spec; no live deviation comments remain in production code. (Test comments that *describe* the retirement may stay.)

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_obj.go pkg/script/handlers_inv_test.go pkg/script/handlers_obj_test.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): NAI-162 B2.6 — retire NAI-115-D1 deviation chain

SCOPE_PERM drop paths in handlers_inv.go (5 sites) and handlers_obj.go
(1 site) now emit Self.AddWealthEvent inline, matching TS. Deviation
comments removed. Existing tests updated to assert event emission;
new TestNAI115D1Retirement_InvDropSlotScopePermEmitsWealthEvent pins
the behaviour flip.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B2.7: B2 close

- [ ] **Step 1: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 2: Audit recount**

```bash
awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/claude/handled.txt && \
  awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/claude/declared.txt && \
  comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l
```

Expected: `5`.

- [ ] **Step 3: Bundle close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-162 B2 — WealthEvent + interaction + retirement (8→5)

3 handlers + WealthEvent subsystem + NAI-115-D1 retirement:
  - (*ObjTypes).ByName [B2.1]
  - (*Player).AddWealthEvent + wealthLog [B2.2]
  - handleWealthEvent (2131) [B2.3]
  - handlePLocMerge (2074) [B2.4]
  - handlePOpPlayerT (2082) [B2.5]
  - NAI-115-D1 retired at 6 sites [B2.6]

Missing-handler audit: 8 → 5. SCOPE_PERM drop paths now emit
wealth events (was skipped).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle B3 — Inv drops (2 handlers)

**Goal:** Land `handleBothDropSlot` and `handleInvDropAll` using B2's `AddWealthEvent`. Audit recount 5 → 3.

### Task B3.1: `handleBothDropSlot` (BOTH_DROPSLOT, opcode 4300)

**Files:**
- Modify: `pkg/script/handlers_inv.go`
- Modify: `pkg/script/handlers.go`
- Test: `pkg/script/handlers_inv_test.go`

- [ ] **Step 1: Re-read full TS body**

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  sed -n '672,723p' src/engine/script/handlers/InvOps.ts
```

Confirm sequence:
1. popInts(4) → [inv, coord, slot, duration]
2. Validate (InvTypeValid, DurationValid, CoordValid)
3. `secondary := state.intOperand == 1`
4. From/to swap based on secondary
5. Nil-check both → error "player is null"
6. Protect gate: `ProtectedActivePlayer[secondary ? 1 : 0]` — slot-1 protect if secondary, slot-0 if primary. Only enforced when `invType.protect && scope != SHARED`
7. fromPlayer.invGetSlot(invID, slot) → obj; nil → error "$slot is empty"
8. If `invType.scope === SCOPE_PERM`: emit `addWealthEvent({event_type: PVP, items: [{...}], value: count*cost, recipient_session: toPlayer.session})`
9. fromPlayer.invDel(invID, objID, count, slot) → completed; if 0, return
10. Construct dropObj
11. Untradeable: `World.addObj(dropObj, fromPlayer.hash64, duration)`. Tradeable: `World.addObj(dropObj, toPlayer.hash64, duration)`

- [ ] **Step 2: Write the failing tests** (11 cases per spec §5.4)

Add to `pkg/script/handlers_inv_test.go`. The full suite below covers every branch from §5.4:

```go
// TestHandleBothDropSlot_PrimaryFromSelf_NonProtected:
// secondary=0, inv.protect=false → skips protect gate.
func TestHandleBothDropSlot_PrimaryFromSelf_NonProtected(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	mc := newTestConfigs(t,
		withInvType( /*id=5, protect=false, scope=ScopeNormal*/ ),
		withObjType( /*id=10, debugname="rune", cost=100, tradeable=false*/ ),
	)
	mw := &mockWorld{}
	mp.setInvSlot(5, 0, obj{ID: 10, Count: 3})

	s := &ScriptState{
		Self:          mp,
		Self2:         mp2,
		Configs:       mc,
		World:         mw,
		Pointers:      PtrActivePlayer | PtrActivePlayer2,
		StackCapacity: 8,
	}
	// intOperand=0 (primary)
	s.Script = &ScriptFile{IntOperands: []int32{0, 0}}
	s.PC = 0

	// Push order: inv, coord, slot, duration
	s.PushInt(5)
	s.PushInt(packTestCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mw.addObjCalls) != 1 {
		t.Fatalf("addObj: got %d, want 1", len(mw.addObjCalls))
	}
	got := mw.addObjCalls[0]
	if got.ReceiverHash != mp.Hash64() {
		t.Errorf("untradeable receiver: got %d, want fromPlayer.hash64", got.ReceiverHash)
	}
}

// TestHandleBothDropSlot_PrimaryFromSelf_Protected_HasProtect:
// secondary=0, inv.protect=true, has PtrProtectedActivePlayer.
func TestHandleBothDropSlot_PrimaryFromSelf_Protected_HasProtect(t *testing.T) {
	// ... identical setup with invType.Protect=true, Pointers |= PtrProtectedActivePlayer ...
}

// TestHandleBothDropSlot_PrimaryFromSelf_Protected_NoProtect:
// Same as above but Pointers lacks PtrProtectedActivePlayer.
func TestHandleBothDropSlot_PrimaryFromSelf_Protected_NoProtect(t *testing.T) {
	// ... expect error containing "requires protected access" ...
}

// TestHandleBothDropSlot_SecondaryFromSelf2: secondary=1 →
// fromPlayer = Self2; addObj receiver = Self2.hash64 (untradeable).
func TestHandleBothDropSlot_SecondaryFromSelf2(t *testing.T) {
	// ... setup with s.Script.IntOperands[s.PC] = 1; mp2 holds the inv ...
}

// TestHandleBothDropSlot_SecondaryProtectViaSlot1: secondary=1
// requires PtrProtectedActivePlayer2 (slot-1 protect), NOT slot-0.
// Pins R3 (protect-gate indexing).
func TestHandleBothDropSlot_SecondaryProtectViaSlot1(t *testing.T) {
	// ... Pointers = ActivePlayer|ActivePlayer2|ProtectedActivePlayer
	// (slot-0 only). Expect error (slot-1 protect missing). ...
}

// TestHandleBothDropSlot_ScopePerm_EmitsPVPWealthEvent
func TestHandleBothDropSlot_ScopePerm_EmitsPVPWealthEvent(t *testing.T) {
	// ... withInvType(scope=ScopePerm), withObjType(cost=1000) ...
	// ... obj{ID:10, Count:5} → expected AccountValue=5000, EventType=PVP ...
	if got.EventType != WealthEventTypePVP {
		t.Errorf("...")
	}
	if got.RecipientSession != mp2Session {
		t.Errorf("...")
	}
}

// TestHandleBothDropSlot_TradeableGoesToReceiver: tradeable obj →
// addObj.receiver = toPlayer.hash64.
func TestHandleBothDropSlot_TradeableGoesToReceiver(t *testing.T) {
	// ... withObjType(tradeable=true) ...
	// ... assert addObj.ReceiverHash == mp2.Hash64() ...
}

// TestHandleBothDropSlot_NullPlayer: Self2 absent → error.
func TestHandleBothDropSlot_NullPlayer(t *testing.T) {
	// ... s.Self2 = nil, s.Pointers without PtrActivePlayer2 ...
	// ... expect error "BOTH_DROPSLOT: player is null" ...
}

// TestHandleBothDropSlot_EmptySlot
func TestHandleBothDropSlot_EmptySlot(t *testing.T) {
	// ... slot 0 has no obj ... expect error "$slot is empty" ...
}

// TestHandleBothDropSlot_InvDelZero: InvDel returns 0 → return,
// no addObj.
func TestHandleBothDropSlot_InvDelZero(t *testing.T) {
	// ... mock InvDel to return 0; assert len(addObjCalls) == 0 ...
}
```

Plan executor: each test case above is sketched with `... //setup ...` in the interest of plan length. Fill out each test in TDD order:

For each subtask (B3.1.A through B3.1.J), write the full test, run, watch fail, then implement just enough to make THAT subtest pass. Bundle each batch of 2-3 closely-related assertions into one commit. The pattern from the first test is the template.

- [ ] **Step 3: Run tests to verify failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleBothDropSlot -v
```

Expected: FAIL across the board.

- [ ] **Step 4: Implement `handleBothDropSlot`**

Add to `pkg/script/handlers_inv.go`:

```go
// handleBothDropSlot (BOTH_DROPSLOT, opcode 4300). Drops a slot's
// contents at a coord, with from/to player swap based on IntOperand.
// secondary == 1 ⇒ fromPlayer = Self2, toPlayer = Self.
// SCOPE_PERM drops emit a PVP wealth event with the toPlayer's
// session as recipient.
// Mirrors TS InvOps.ts:672-723.
//
// Pop order (LIFO): duration, slot, coord, invID (TS popInts(4) →
// [inv, coord, slot, duration]).
func handleBothDropSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "BOTH_DROPSLOT"); err != nil {
		return err
	}
	if s.Configs == nil {
		return fmt.Errorf("BOTH_DROPSLOT: no configs")
	}
	if s.World == nil {
		return fmt.Errorf("BOTH_DROPSLOT: no world surface")
	}

	duration := s.PopInt()
	slot := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	invType := s.Configs.InvType(int(invID))
	if invType == nil {
		return fmt.Errorf("BOTH_DROPSLOT: invalid inv id (%d)", invID)
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("BOTH_DROPSLOT: %w", err)
	}
	level, x, z, err := checkCoord(coord, "BOTH_DROPSLOT")
	if err != nil {
		return err
	}

	secondary := s.Script.IntOperands[s.PC] == 1

	var fromPlayer, toPlayer ActivePlayer
	if secondary {
		fromPlayer = s.Self2
		toPlayer = s.Self
	} else {
		fromPlayer = s.Self
		toPlayer = s.Self2
	}
	if fromPlayer == nil || toPlayer == nil {
		return fmt.Errorf("BOTH_DROPSLOT: player is null")
	}

	// Protect gate: enforced only when invType.protect && scope != SHARED.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		if secondary {
			if err := requireProtectedActivePlayer2(s, "BOTH_DROPSLOT"); err != nil {
				return err
			}
		} else {
			if err := requireProtectedActivePlayer(s, "BOTH_DROPSLOT"); err != nil {
				return err
			}
		}
	}

	// Slot lookup on fromPlayer.
	inv := s.World.InvFor(fromPlayer, int(invID)) // plan executor: confirm accessor name
	if inv == nil {
		return fmt.Errorf("BOTH_DROPSLOT: fromPlayer inv missing")
	}
	obj := inv.Get(int(slot))
	if obj == nil {
		return fmt.Errorf("BOTH_DROPSLOT: $slot is empty")
	}
	objType := s.Configs.ObjType(obj.ID)
	debugName := ""
	cost := 0
	tradeable := false
	if objType != nil {
		debugName = objType.DebugName
		cost = objType.Cost
		tradeable = objType.Tradeable
	}

	// SCOPE_PERM PVP wealth event.
	if invType.Scope == objtype.InvTypeScopePerm {
		s.Self.AddWealthEvent(WealthEvent{
			EventType:        WealthEventTypePVP,
			AccountItems:     []WealthItem{{ID: obj.ID, Name: debugName, Count: obj.Count}},
			AccountValue:     obj.Count * cost,
			RecipientSession: toPlayer.Session(),
		})
	}

	completed := fromPlayer.InvDel(int(invID), obj.ID, obj.Count, int(slot))
	if completed == 0 {
		return nil
	}

	// Determine receiver: untradeable stays with fromPlayer.
	var receiver int
	if tradeable {
		receiver = toPlayer.Hash64()
	} else {
		receiver = fromPlayer.Hash64()
	}

	s.World.AddObj(level, x, z, obj.ID, completed, int(duration), receiver)
	return nil
}
```

Plan executor: many accessor names above are PLAN-LEVEL placeholders. Confirm each against goscape current API before writing code:
- `s.World.InvFor(player, id)` — likely doesn't exist; the existing pattern uses `s.invs.Get(self, typeID)` or similar. Re-read `handleInvDropSlot` at handlers_inv.go:791 and mirror its inv-lookup pattern, but for an arbitrary player (not just `s.Self`).
- `fromPlayer.InvDel(...)`, `fromPlayer.Hash64()`, `toPlayer.Session()` — verify on `ActivePlayer` interface; if absent, widen interface in B1.5 (retroactively, OR in this task).
- `s.World.AddObj` — verify signature.

Per memory `audit_full_method_against_ts.md`: each handler accessor must be grep-verified before locking the implementation.

- [ ] **Step 5: Add dispatch entry**

```go
	OpBothDropSlot: handleBothDropSlot,
```

- [ ] **Step 6: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleBothDropSlot -v
```

Expected: 10 PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go pkg/script/handlers.go pkg/script/state.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B3.1 — BOTH_DROPSLOT handler (opcode 4300)

From/to player swap based on IntOperand. SCOPE_PERM emits PVP wealth
event with toPlayer.session as recipient. Untradeable obj routes to
fromPlayer; tradeable routes to toPlayer. Mirrors TS InvOps.ts:672-723.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B3.2: `handleInvDropAll` (INV_DROPALL, opcode 4309)

**Files:**
- Modify: `pkg/script/handlers_inv.go`
- Modify: `pkg/script/handlers.go`
- Test: `pkg/script/handlers_inv_test.go`

- [ ] **Step 1: Re-read full TS body**

```bash
cd /home/owner/Code/github.com/LostCityRS/Engine-TS && \
  sed -n '726,790p' src/engine/script/handlers/InvOps.ts
```

Confirm:
- popInts(3) → [inv, coord, duration]
- Protect via `ProtectedActivePlayer[state.intOperand]`
- Walk every slot; SCOPE_PERM accumulates wealth-log Map keyed by objID
- Per slot: delete from inv; addObj receiver = activePlayer.hash64 if untradeable else `Obj.NO_RECEIVER`
- Post-loop: if wealth-log non-empty, emit `AddWealthEvent(eventType=Death, items: <log values>, value: totalValue)`

- [ ] **Step 2: Write the failing tests** (4 cases per spec §5.4)

Add to `pkg/script/handlers_inv_test.go`:

```go
// TestHandleInvDropAll_EmptyInv: no non-empty slots → no addObj, no
// wealth event.
func TestHandleInvDropAll_EmptyInv(t *testing.T) {
	mp := &mockPlayer{}
	// inv with 28 capacity, all empty
	// ... assert addObjCalls == 0 && addWealthEventCalls == 0 ...
}

// TestHandleInvDropAll_MixedSlots: 3 non-empty slots, SCOPE_NORMAL →
// 3 addObj calls, no wealth event.
func TestHandleInvDropAll_MixedSlots(t *testing.T) {
	// ... withInvType(scope=ScopeNormal), 3 non-empty slots (ids 10, 20, 10) ...
	// ... assert len(addObjCalls) == 3, addWealthEventCalls == 0 ...
}

// TestHandleInvDropAll_ScopePerm_AccumulatesWealthLog: SCOPE_PERM
// with 3 non-empty slots (ids 10, 20, 10) → 3 addObj + 1 wealth event
// with 2 line items (id=10 has count=count_slot0+count_slot2) and
// AccountValue summed.
func TestHandleInvDropAll_ScopePerm_AccumulatesWealthLog(t *testing.T) {
	// ... obj fixtures: {ID:10, Count:3}, {ID:20, Count:2}, {ID:10, Count:5} ...
	// ... withObjType(10, cost:100), withObjType(20, cost:50) ...
	// ... after: addWealthEventCalls[0].AccountItems has 2 entries:
	//     {ID:10, Count:8}, {ID:20, Count:2}
	// AccountValue = (3+5)*100 + 2*50 = 900
	// EventType = WealthEventTypeDeath
}

// TestHandleInvDropAll_TradeableSplit: tradeable obj → addObj.receiver
// == Obj.NoReceiver. Untradeable → addObj.receiver == self.Hash64().
func TestHandleInvDropAll_TradeableSplit(t *testing.T) {
	// ... two slots: id=10 (tradeable=true), id=20 (tradeable=false) ...
	// ... addObjCalls[0].ReceiverHash == NoReceiverConst ...
	// ... addObjCalls[1].ReceiverHash == mp.Hash64() ...
}
```

Plan executor: confirm `NoReceiverConst` constant — likely `entity.NoReceiver = -1` or `zone.NoReceiverHash`. Grep `NoReceiver|NO_RECEIVER` to find.

- [ ] **Step 3: Run tests to verify failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleInvDropAll -v
```

Expected: FAIL.

- [ ] **Step 4: Implement the handler**

Add to `pkg/script/handlers_inv.go`:

```go
// handleInvDropAll (INV_DROPALL, opcode 4309). Walks every slot of the
// named inv, dropping each obj to the world. SCOPE_PERM accumulates
// a per-objID wealth log; after the loop, emits a single Death-type
// wealth event with aggregated items + total value.
// Mirrors TS InvOps.ts:726-790.
//
// Pop order (LIFO): duration, coord, invID.
func handleInvDropAll(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DROPALL"); err != nil {
		return err
	}
	if s.Configs == nil {
		return fmt.Errorf("INV_DROPALL: no configs")
	}
	if s.World == nil {
		return fmt.Errorf("INV_DROPALL: no world surface")
	}
	duration := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	invType := s.Configs.InvType(int(invID))
	if invType == nil {
		return fmt.Errorf("INV_DROPALL: invalid inv id (%d)", invID)
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPALL: %w", err)
	}
	level, x, z, err := checkCoord(coord, "INV_DROPALL")
	if err != nil {
		return err
	}

	// Protect gate: ProtectedActivePlayer[intOperand]. intOperand 0 →
	// slot-0; 1 → slot-1.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		secondary := s.Script.IntOperands[s.PC] == 1
		if secondary {
			if err := requireProtectedActivePlayer2(s, "INV_DROPALL"); err != nil {
				return err
			}
		} else {
			if err := requireProtectedActivePlayer(s, "INV_DROPALL"); err != nil {
				return err
			}
		}
	}

	inv := s.World.InvFor(s.Self, int(invID))
	if inv == nil {
		return nil
	}

	type wealthLogEntry struct {
		id    int
		name  string
		count int
		cost  int
	}
	wealthLog := map[int]*wealthLogEntry{}
	totalValue := 0

	for slot := 0; slot < inv.Capacity(); slot++ {
		obj := inv.Get(slot)
		if obj == nil {
			continue
		}
		objType := s.Configs.ObjType(obj.ID)
		debugName := ""
		cost := 0
		tradeable := false
		if objType != nil {
			debugName = objType.DebugName
			cost = objType.Cost
			tradeable = objType.Tradeable
		}

		if invType.Scope == objtype.InvTypeScopePerm {
			if e := wealthLog[obj.ID]; e != nil {
				e.count += obj.Count
			} else {
				wealthLog[obj.ID] = &wealthLogEntry{id: obj.ID, name: debugName, count: obj.Count, cost: cost}
			}
			totalValue += obj.Count * cost
		}

		inv.Delete(slot)

		var receiver int
		if tradeable {
			receiver = NoReceiverConst // plan executor confirms constant
		} else {
			receiver = s.Self.Hash64()
		}
		s.World.AddObj(level, x, z, obj.ID, obj.Count, int(duration), receiver)
	}

	if len(wealthLog) > 0 {
		items := make([]WealthItem, 0, len(wealthLog))
		for _, e := range wealthLog {
			items = append(items, WealthItem{ID: e.id, Name: e.name, Count: e.count})
		}
		s.Self.AddWealthEvent(WealthEvent{
			EventType:    WealthEventTypeDeath,
			AccountItems: items,
			AccountValue: totalValue,
		})
	}
	return nil
}
```

Plan executor: confirm:
- `inv.Capacity()`, `inv.Get(slot)`, `inv.Delete(slot)` against `*inventory.Inventory` API.
- `NoReceiverConst` actual name + import.
- `objType.Tradeable` field name.
- Map-keying gotcha (R8): the `*wealthLogEntry` pointer pattern above sidesteps the gotcha. Per memory `audit_full_method_against_ts.md`.

- [ ] **Step 5: Add dispatch entry**

```go
	OpInvDropAll: handleInvDropAll,
```

- [ ] **Step 6: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestHandleInvDropAll -v
```

Expected: 4 PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go pkg/script/handlers.go && \
  git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-162 B3.2 — INV_DROPALL handler (opcode 4309)

Walks every slot, drops each obj. SCOPE_PERM accumulates per-objID
wealth log → single Death-type AddWealthEvent post-loop. Tradeable
objs go to NoReceiver; untradeable stay with the player. Mirrors TS
InvOps.ts:726-790.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B3.3: B3 close + final NAI-162 roll-up

- [ ] **Step 1: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 2: Run race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script ./modules/world
```

Expected: PASS (no race detected).

- [ ] **Step 3: Final audit recount**

```bash
awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/claude/handled.txt && \
  awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/claude/declared.txt && \
  echo "=== count ===" && \
  comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l && \
  echo "=== remaining ===" && \
  comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt
```

Expected: count == 3; remaining == `{OpLineOfSight, OpNpcAdd, OpNpcHunt}`.

- [ ] **Step 4: B3 + NAI-162 final close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-162 — trivial-handler sweep #4 closed (15 ops, 18→3)

15 handlers landed across 4 bundles:
  B0 (stubs):  PUSH_VARBIT 25, POP_VARBIT 27, SET_GENDER 2099,
               LC_OP 4105, OC_IOP 4205, OC_OP 4208
  B1 (small):  LAST_LOGIN_INFO 2054, INV_TOTALPARAM_STACK 4329,
               MAP_INDOORS 1010, NPC_STATHEAL 2539
  B2 (mod.):   WEALTH_EVENT 2131, P_LOCMERGE 2074, P_OPPLAYERT 2082
               + NAI-115-D1 deviation chain retired (6 sites)
  B3 (drops):  BOTH_DROPSLOT 4300, INV_DROPALL 4309

Supporting infra:
  - (*HeroPoints).Clear()
  - collision.IsIndoors / (*Server).IsIndoors adapter
  - (*Player).LastLoginInfo (stub, NAI-162-D-LASTLOGIN-NO-PACKET)
  - (*Player).InvTotalParamStack
  - (*Player).AddWealthEvent + p.wealthLog field
  - (*Npc).HeroPointsClear (interface wrapper)
  - (*ObjTypes).ByName lookup
  - pkg/script.WealthEvent + WealthItem + WealthEventType* + interface widenings

Missing-handler audit: 18 → 3 (OpLineOfSight 1005, OpNpcAdd 2500,
OpNpcHunt 2525 — forward-routed to NAI-163).

Deviations opened:
  - NAI-162-D-STUB-{PUSHVARBIT,POPVARBIT,SETGENDER,LCOP,OCIOP,OCOP}
  - NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY
  - NAI-162-D-LASTLOGIN-NO-PACKET

Deviation retired:
  - NAI-115-D1 (SCOPE_PERM drop addWealthEvent skip)

Smoke confirmed:
  - B2: NAI-115-D1 retirement via SCOPE_PERM drop content path
        (Self.AddWealthEvent fires; wealthLog appended)
  - B3: inv-drops no-WARN under content-pin smoke

Closes memory:
  - runescript_cadence.md
  - controller_preflight.md
  - missing_handler_audit.md
  - execution_mode_default.md
  - superpowers_clear_between_spec_and_impl.md
  - superpowers_code_reviewer_model.md
  - spec_iteration_scope_audit.md
  - audit_full_method_against_ts.md
  - spec_ts_source_read.md
  - vararg_opcode_shapes_dont_share_with_fixed_arg_siblings.md
  - retire_deviation_grep_all_comments.md
  - true_to_ts_gate.md
  - defensive_gate_doc_comment_label.md
  - mock_recorder_field_naming_check.md
  - plan_grep_helper_patterns.md
  - plan_sibling_site_guard_audit.md
  - scriptstate_test_fixture_idioms.md
  - cascade_theory_smoke_binding.md
  - smoke_test_server_handoff.md
  - nodedebug_gateway_probe_pattern.md
  - enumerate_all_sites.md
  - close_commit_memory_trailer.md
  - interface_at_cyclic_import_boundary.md

NAI-163 handoff: 3 deferred ops (LineOfSight raycast, NpcAdd entity-
create, NpcHunt closest-NPC scan). Each warrants its own brainstorm.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Smoke handoffs

### B2 smoke — NAI-115-D1 retirement (REQUIRED)

After B2 commits land, hand off to user for smoke:

> "B2 landed. Please launch the server (per smoke_test_server_handoff). Smoke target: drop a permanent-scope item (e.g., from a SCOPE_PERM inv slot in a death-drop or trade-drop content script). Expected post-NAI-162: server logs show the wealth event was emitted (grep `wealth_event` or the in-memory log via a NodeDebug-style probe per nodedebug_gateway_probe_pattern). Pre-NAI-162 behaviour was silent skip."

### B3 smoke — inv-drops (REQUIRED)

After B3 commits land:

> "B3 landed. Please launch the server. Smoke target: exercise either BOTH_DROPSLOT (PVP drop content path) or INV_DROPALL (full-inv-clear, e.g., death drop). Expected: no `no handler for BOTH_DROPSLOT (4300)` or `no handler for INV_DROPALL (4309)` WARN in logs. If smoke surfaces an adjacent divergence per smoke_surfaces_adjacent_divergences, route to NAI-163 brainstorm."

---

## Self-review (writer's pass before handoff)

Spec coverage:
- [x] §1 cohort table — all 18 ops accounted for (15 in plan + 3 deferred).
- [x] §2.1 B0 stubs — Task B0.2 lands all 6.
- [x] §2.2 B1 — Tasks B1.1–B1.10 cover (*HeroPoints).Clear, IsIndoors, LastLoginInfo, InvTotalParamStack, interface wideners, 4 handlers, close.
- [x] §2.3 B2 — Tasks B2.1–B2.7 cover ObjTypes.ByName, AddWealthEvent real body, 3 handlers, NAI-115-D1 retirement, close.
- [x] §2.4 B3 — Tasks B3.1–B3.3 cover BothDropSlot, InvDropAll, final close.
- [x] §3 Deviations — Tasks B0.2, B1.3, B2.2 open the documented deviations. B2.6 retires NAI-115-D1.
- [x] §4 Risk register — R1 addressed by B2.6 Step 3; R3 by B3.1 §5.4 #5; R4 by B1.2 + B1.8; R5 by B0.1; R6 by B1.3 Step 1; R7 by B1.5; R8 by B3.2 pointer-map pattern; R10 by B2.6 Step 1.
- [x] §5 Test strategy — every named test in the spec has a corresponding task step.
- [x] §6 Smoke binding — B2 and B3 smoke handoffs at end of plan.
- [x] §7 Cadence routing — per-bundle close commits + final roll-up.
- [x] §10 No-deviations audit — all 15 ops bodies cited at TS line.

Placeholder scan: no "TBD" / "TODO" left. "Plan executor confirms X" is intentional and bounded (specific grep + accessor mapping).

Type consistency:
- WealthEvent fields: EventType, AccountItems, AccountValue, RecipientSession — consistent across §2.3.1, B1.5, B2.2, B2.3, B3.1, B3.2.
- AccountItems[i] fields: ID, Name, Count — consistent.
- Pop order documented as LIFO + TS-popInts-translated everywhere it matters.
- Interface widenings (B1.5) declare WealthEvent + AddWealthEvent; B2.2 fills the body; B3.1/B3.2 call it.

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-11-nai-162-trivial-handler-sweep-4.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — fresh subagent per task, two-stage review between tasks, ~30 tasks total. Best for a multi-bundle plan with infra dependencies; reviewer catches accessor-name drift per memory `controller_preflight`.

**2. Inline Execution** — execute tasks in this session using executing-plans, with batch checkpoints at each bundle close (B0, B1, B2, B3). Faster but heavier on context.

**Per memory `superpowers_clear_between_spec_and_impl`: regardless of choice, the user should `/clear` between this session (which wrote the spec + plan) and the impl session.**
