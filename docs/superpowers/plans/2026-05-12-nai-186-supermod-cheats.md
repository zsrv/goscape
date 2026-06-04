# NAI-186 Super-Mod Cheats Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the four `staffModLevel >= 2 && NODE_PRODUCTION` super-mod cheats (`::setvis`, `::ban`, `::mute`, `::kick`) from `Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:549-616` into `modules/world/handlers_game.go`.

**Architecture:** Adds one new method on `Player` (`SetVisibility`) and four inline case-arms in the existing super-mod switch at `modules/world/handlers_game.go:765`. ban/mute route through the existing `loginBridgeMod.NotifyPlayerBan` / `NotifyPlayerMute`. kick sets `loggingOut=true` (TS-deviation D1: teardown defers to processLogouts instead of inline `logout+close`).

**Tech Stack:** Go 1.26+, modules/world. Uses existing `parseIntOr`, `recordingBridges`, `newTestServer`/`newTestPlayer`, `reportAbuseSetupWithOnlineOffender`-style player-loop wiring.

**Spec:** `docs/superpowers/specs/2026-05-12-nai-186-supermod-cheats-design.md` (commit 6a1aab8).

**Memory anchors:**
- `drainconn_iocopy_race` — do not combine `io.Copy(io.Discard, cc)` with `drainAfterTele` on the same conn.
- `mock_recorder_field_naming_check` — recordingBridges field names: `method/staff/username/until` (verified at plan-write).
- `controller_preflight` — controller runs 30-s grep+Read before each implementer dispatch.
- `verify_implementer_claims` — every test claim verified via fresh independent run.
- `ts_asymmetry_dual_pin` — pin SOFT stub state AND message (absence-pin).
- `enumerate_all_sites` — re-grep `DEVIATION-NAI-185-D4-CARRYFORWARD` at plan-write (1 occurrence in modules/world/handlers_game.go:367; no stale references elsewhere).

---

## Pre-flight verification (verified at plan-write, no action required)

These were re-greped at plan-write against HEAD (commit 6a1aab8). The plan-author/controller may re-confirm before each dispatch, but the answers are baked in below.

| ID | Question | Answer |
|----|----------|--------|
| R1 | `parseIntOr` signature | `func parseIntOr(s string, def int) int` at `modules/world/handlers_game.go:853` |
| R2 | recordingBridges fields | `loginMod []recordedLoginModCall{method, staff, username, until}` at `modules/world/bridges_test.go:18,45-47,76,79` |
| R3 | `MessageGame` shape | `p.MessageGame(string)` — used at `handlers_game.go:555` etc. with `fmt.Sprintf` |
| R4 | gamemap helpers | `s.gamemap.ChangeNPCCollision(size, x, z, level, add bool)` and `ChangePlayerCollision(...)` at `pkg/gamemap/gamemap.go:120-128` |
| R5 | test factories | `newTestServer(t)` at `server_test.go:311`; `newTestPlayer(t)` at `player_test.go:17`; wire-up pattern: `reportAbuseSetup` at `handler_reportabuse_test.go:34-43`, `reportAbuseSetupWithOnlineOffender` at L315-325 |
| R6 | NodeProduction | `cfg.NodeProduction bool` default `false` at `modules/world/config.go:43` |
| R7 | flag readback | `s.gamemap.Pathfinder.Flags.IsFlagged(x, z, level, flagMask)`; for unmapped tiles call `AllocateIfAbsent(x, z, level)` first (precedent: `npc_hunt_entities_test.go:631`) |
| R8 | imports already present | `player.go` has `fmt`, `strings`, `time`, `github.com/zsrv/goscape/pkg/rsbuf`. `handlers_game.go` has `fmt`, `strings`, `time` (lines 3-13). No new imports needed. |
| Bonus | super-mod switch close | Closes at `handlers_game.go:836`. New cheat arms splice in between `teleto` (ends L835) and the closing `}` at L836. |

---

## Task 1: Add `Player.SetVisibility`

**Files:**
- Modify: `modules/world/player.go` (add method near other Player methods, e.g. near `MessageGame`)
- Modify: `modules/world/player_test.go` (add 3 tests)

**Context:** Mirrors TS `Player.setVisibility` at `Engine-TS/src/engine/entity/Player.ts:1875-1891`. Three arms: SOFT (message-only stub, no state change), DEFAULT (visibility+blockWalk+collision), HARD (visibility+blockWalk+collision×2). Player width is always 1 in TS PathingEntity init — hardcode `size=1`.

- [ ] **Step 1: Write the failing test for SetVisibility(Default)**

In `modules/world/player_test.go`, append at the end of the file:

```go
// TestPlayerSetVisibilityDefault pins TS Player.setVisibility(DEFAULT) at
// Engine-TS/src/engine/entity/Player.ts:1875-1891. DEFAULT arm sets
// visibility=Default, blockWalk=Npc, calls ChangeNPCCollision(...,true)
// at player coords, and emits MessageGame "vis: 0".
func TestPlayerSetVisibilityDefault(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 3200, 3200, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)

	// Start from Hard so we can observe the transition into Default.
	p.visibility = rsbuf.VisibilityHard
	p.blockWalk = BlockWalkNone

	received := drainConn(t, cc)

	p.SetVisibility(rsbuf.VisibilityDefault)

	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want VisibilityDefault", p.visibility)
	}
	if p.blockWalk != BlockWalkNpc {
		t.Errorf("blockWalk: got %v, want BlockWalkNpc", p.blockWalk)
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockNPCs) {
		t.Error("FlagBlockNPCs: must be set at player tile after SetVisibility(Default)")
	}
	if !bytes.Contains(out, []byte("vis: 0")) {
		t.Errorf("MessageGame: out missing 'vis: 0'; got %q", out)
	}
}
```

Add any missing imports at the top of `player_test.go`:
- `"bytes"` (if not already present)
- `"github.com/zsrv/goscape/pkg/collision"` (if not already present)
- `"github.com/zsrv/goscape/pkg/gamemap"`
- `io2 "github.com/zsrv/goscape/pkg/io/isaac"` (already aliased in other test files; check the existing alias in this file)

To check existing imports run: `head -25 modules/world/player_test.go`.

- [ ] **Step 2: Run the test to verify it fails (SUT undefined)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPlayerSetVisibilityDefault -count=1
```

Expected: FAIL with `p.SetVisibility undefined (type *Player has no field or method SetVisibility)`.

- [ ] **Step 3: Add the SetVisibility method**

In `modules/world/player.go`, append a new method (location: after the existing `MessageGame` method, or near other public methods — find a similar method like `MessageGame` and add `SetVisibility` adjacent):

```go
// SetVisibility mirrors TS Player.setVisibility (Engine-TS/src/engine/entity/Player.ts:1875-1891).
// SOFT is a TS stub: emits a "not implemented" message and returns without
// state change (TS L1876-1879). DEFAULT/HARD update visibility + blockWalk
// and toggle per-tile collision flags. Player.width is always 1 in TS
// (PathingEntity init); hardcoded size=1 here.
//
// DEVIATION-NAI-186-D1 cohort: no deviation in this method itself;
// see kick teardown for NAI-186 D1.
func (p *Player) SetVisibility(v rsbuf.Visibility) {
	if v == rsbuf.VisibilitySoft {
		p.MessageGame(fmt.Sprintf("vis: %d (not implemented - you are still on vis: %d)", int(v), int(p.visibility)))
		return
	}
	p.visibility = v
	s := p.client.server
	if v == rsbuf.VisibilityDefault {
		p.blockWalk = BlockWalkNpc
		s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, true)
	} else {
		p.blockWalk = BlockWalkNone
		s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, false)
		s.gamemap.ChangePlayerCollision(1, p.x, p.z, p.level, false)
	}
	p.MessageGame(fmt.Sprintf("vis: %d", int(v)))
}
```

Verify `fmt` and `rsbuf` are imported in `player.go` (per R8 pre-flight: yes, both present at lines 3-16). No new imports.

- [ ] **Step 4: Run the test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPlayerSetVisibilityDefault -count=1
```

Expected: PASS.

- [ ] **Step 5: Add the SetVisibility(Soft) absence-pin test**

In `modules/world/player_test.go`, append:

```go
// TestPlayerSetVisibilitySoftStub pins TS Player.setVisibility(SOFT) early
// return at Engine-TS/src/engine/entity/Player.ts:1876-1879. SOFT is a
// message-only stub: no state change to visibility, blockWalk, or
// collision flags. Pinning both presence-of-message AND absence-of-state-
// change per memory ts_asymmetry_dual_pin.
func TestPlayerSetVisibilitySoftStub(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 3200, 3201, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)

	// Initial state: defaults (visibility=Default, blockWalk=BlockWalkNpc per
	// modules/world/player.go:556+523).
	if p.visibility != rsbuf.VisibilityDefault {
		t.Fatalf("preflight: visibility should default to Default")
	}
	if p.blockWalk != BlockWalkNpc {
		t.Fatalf("preflight: blockWalk should default to BlockWalkNpc")
	}

	received := drainConn(t, cc)
	p.SetVisibility(rsbuf.VisibilitySoft)
	p.client.flushWrite()
	out := <-received

	// State pins: unchanged.
	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (Default)", p.visibility)
	}
	if p.blockWalk != BlockWalkNpc {
		t.Errorf("blockWalk: got %v, want unchanged (BlockWalkNpc)", p.blockWalk)
	}

	// Message pin: includes "vis: 1 (not implemented - you are still on vis: 0)".
	if !bytes.Contains(out, []byte("vis: 1 (not implemented - you are still on vis: 0)")) {
		t.Errorf("MessageGame: missing TS-faithful SOFT stub string; got %q", out)
	}
}
```

- [ ] **Step 6: Run to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPlayerSetVisibilitySoftStub -count=1
```

Expected: PASS.

- [ ] **Step 7: Add the SetVisibility(Hard) test**

In `modules/world/player_test.go`, append:

```go
// TestPlayerSetVisibilityHard pins TS Player.setVisibility(HARD) at
// Engine-TS/src/engine/entity/Player.ts:1885-1890. HARD arm sets
// visibility=Hard, blockWalk=None, calls ChangeNPCCollision(...,false)
// AND ChangePlayerCollision(...,false), and emits "vis: 2".
func TestPlayerSetVisibilityHard(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 3200, 3202, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)

	// Seed FlagBlockNPCs at the tile so we can observe the clear.
	s.gamemap.Pathfinder.Flags.Add(p.x, p.z, p.level, collision.FlagBlockNPCs|collision.FlagBlockPlayers)

	received := drainConn(t, cc)
	p.SetVisibility(rsbuf.VisibilityHard)
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityHard {
		t.Errorf("visibility: got %d, want VisibilityHard", p.visibility)
	}
	if p.blockWalk != BlockWalkNone {
		t.Errorf("blockWalk: got %v, want BlockWalkNone", p.blockWalk)
	}
	// After HARD: both FlagBlockNPCs and FlagBlockPlayers must be cleared.
	if s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockNPCs) {
		t.Error("FlagBlockNPCs: must be cleared at player tile after SetVisibility(Hard)")
	}
	if s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockPlayers) {
		t.Error("FlagBlockPlayers: must be cleared at player tile after SetVisibility(Hard)")
	}
	if !bytes.Contains(out, []byte("vis: 2")) {
		t.Errorf("MessageGame: missing 'vis: 2'; got %q", out)
	}
}
```

- [ ] **Step 8: Run all three tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestPlayerSetVisibility' -count=1 -race
```

Expected: PASS for all 3 (Default, SoftStub, Hard).

- [ ] **Step 9: Commit**

```bash
git add modules/world/player.go modules/world/player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-186 T1 — port Player.setVisibility

Mirrors TS Player.setVisibility (Engine-TS/src/engine/entity/Player.ts:1875-1891).
Three arms: SOFT message-only stub (no state change), DEFAULT/HARD
update visibility + blockWalk + collision flags via gamemap.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `::setvis` cheat dispatch arm

**Files:**
- Modify: `modules/world/handlers_game.go` (insert case-arm in super-mod switch, between `teleto` and switch close)
- Create: `modules/world/handler_cheats_supermod_test.go`

**Context:** TS `ClientCheatHandler.ts:549-568` — arg "0"/"1"/"2" dispatches to `player.setVisibility(DEFAULT/SOFT/HARD)`. Bad arg → silent return. NodeProduction-gated.

**Insertion point:** super-mod switch in `handleClientCheat` ends at `modules/world/handlers_game.go:836` (closing `}` of the switch). The new `case "setvis":` block goes immediately before that `}`, after the existing `case "teleto":` body (which ends at L835 with `p.TeleJump(other.x, other.z, other.level)`).

- [ ] **Step 1: Create the test file with the happy-path test**

Create `modules/world/handler_cheats_supermod_test.go`:

```go
package world

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/collision"
	"github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// supermodSetup wires a Player into a Server at staffModLevel=2 (super-mod
// gate), with cfg.NodeProduction=true, recordingBridges installed, and a
// fixed-seed ISAAC encryptor so cheat-dispatched MessageGame writes go
// through writeOut without panic. Mirrors handler_reportabuse_test.go's
// reportAbuseSetup style. Returns the player, the test conn, the server,
// and the recorder.
func supermodSetup(t *testing.T) (*Player, net.Conn, *Server, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.cfg.NodeProduction = true
	rec := installRecordingBridges(s)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.username = "alice"
	p.staffModLevel = 2
	p.x, p.z, p.level = 3200, 3200, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)
	return p, cc, s, rec
}

// dispatchCheat sends `::<cmd> <args>` (without the `::` prefix per TS L42)
// through handleClientCheat. Mirrors handlers_game_test.go:392 dispatchTeleCheat.
func dispatchCheat(t *testing.T, p *Player, cheat string) {
	t.Helper()
	pkt := packet.NewPacket(nil)
	pkt.P1(0) // ctrlHeld byte (unused)
	pkt.PJStrLF(cheat)
	if err := handleClientCheat(p, pkt.Data); err != nil {
		t.Fatalf("handleClientCheat: %v", err)
	}
}

// drainOut flushes p.client.bufw and returns bytes received on cc.
// Pick ONE drain pattern per conn per memory drainconn_iocopy_race.
func drainOut(t *testing.T, p *Player, cc net.Conn) []byte {
	t.Helper()
	received := drainConn(t, cc)
	p.client.flushWrite()
	return <-received
}

// TestSetvisDispatchDefault pins TS ClientCheatHandler.ts:557-558 case '0'
// → Player.setVisibility(DEFAULT). Full state assertion lives in
// TestPlayerSetVisibilityDefault; here we pin only that dispatch reaches
// SetVisibility with the right enum (proxy: post-call visibility==Default).
func TestSetvisDispatchDefault(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	// Start from Hard so we can observe the transition.
	p.visibility = rsbuf.VisibilityHard

	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 0")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility after ::setvis 0: got %d, want Default", p.visibility)
	}
	if !bytes.Contains(out, []byte("vis: 0")) {
		t.Errorf("MessageGame: missing 'vis: 0'; got %q", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails (dispatch arm not yet wired)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestSetvisDispatchDefault -count=1
```

Expected: FAIL — `visibility after ::setvis 0: got 2, want 0` (Hard preserved because no dispatch arm).

- [ ] **Step 3: Add the setvis case-arm to handlers_game.go**

In `modules/world/handlers_game.go`, locate the super-mod switch closing brace at L836. Add the new `case "setvis":` just before it (after the existing `case "teleto":` body ends at L835).

Search for the exact insertion line `p.TeleJump(other.x, other.z, other.level)` and the trailing `}` `}` of the case+switch. Insert:

```go
		case "setvis":
			// TS ClientCheatHandler.ts:549-568 — ::setvis <level>.
			// NodeProduction-gated. NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			switch sub[0] {
			case "0":
				p.SetVisibility(rsbuf.VisibilityDefault)
			case "1":
				p.SetVisibility(rsbuf.VisibilitySoft)
			case "2":
				p.SetVisibility(rsbuf.VisibilityHard)
			default:
				return nil
			}
```

The indentation must match the surrounding cases (one level inside the `switch parts[0] {` inside the `if p.staffModLevel >= 2 {`).

Verify `rsbuf` is imported in handlers_game.go: `grep -n 'rsbuf' modules/world/handlers_game.go | head -3`. If not, add `"github.com/zsrv/goscape/pkg/rsbuf"` to the import block.

- [ ] **Step 4: Run to verify the test passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestSetvisDispatchDefault -count=1
```

Expected: PASS.

- [ ] **Step 5: Add the remaining 5 setvis tests**

Append to `modules/world/handler_cheats_supermod_test.go`:

```go
// TestSetvisDispatchSoftStub pins TS L560-562 case '1' → SOFT stub.
// SOFT path: message emitted, state unchanged.
func TestSetvisDispatchSoftStub(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	// Initial visibility=Default per newPlayer.
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 1")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (Default) — SOFT is a stub", p.visibility)
	}
	if !bytes.Contains(out, []byte("vis: 1 (not implemented - you are still on vis: 0)")) {
		t.Errorf("MessageGame: missing TS-faithful SOFT stub; got %q", out)
	}
}

// TestSetvisDispatchHard pins TS L563-565 case '2' → HARD.
func TestSetvisDispatchHard(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 2")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityHard {
		t.Errorf("visibility after ::setvis 2: got %d, want Hard", p.visibility)
	}
	if !bytes.Contains(out, []byte("vis: 2")) {
		t.Errorf("MessageGame: missing 'vis: 2'; got %q", out)
	}
}

// TestSetvisDispatchBadArg pins TS L566-567 default: return false — no
// state change, no message.
func TestSetvisDispatchBadArg(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 5")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (Default)", p.visibility)
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected on bad arg; got %q", out)
	}
}

// TestSetvisDispatchNoArg pins TS L551-554 args.length < 1 → return false.
func TestSetvisDispatchNoArg(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged", p.visibility)
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}

// TestSetvisDispatchNodeProductionGate pins the && Environment.NODE_PRODUCTION
// arm selector at TS L549. NodeProduction=false → arm inert.
func TestSetvisDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	s.cfg.NodeProduction = false
	p.visibility = rsbuf.VisibilityDefault

	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 2") // Hard request
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (NodeProduction=false → inert)", p.visibility)
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected when NodeProduction=false; got %q", out)
	}
}
```

- [ ] **Step 6: Run all setvis dispatch tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestSetvisDispatch' -count=1 -race
```

Expected: PASS for all 5 (Default, SoftStub, Hard, BadArg, NoArg, NodeProductionGate).

Wait — that's 5 tests but the spec said 6 cases. Cross-check: spec §7.3 setvis has 6 cases (1-6). Cases 1-3 → Default/SoftStub/Hard; case 4 → BadArg; case 5 → NoArg; case 6 → NodeProductionGate. Tests above cover all 6 (5 functions; the happy-path Default is in Step 1, plus 5 added here = 6 total). Confirmed.

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handler_cheats_supermod_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-186 T2 — port ::setvis cheat

Mirrors TS ClientCheatHandler.ts:549-568. NodeProduction-gated;
dispatches to Player.SetVisibility for level 0/1/2; silent on bad
or missing arg.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `::ban` cheat dispatch arm

**Files:**
- Modify: `modules/world/handlers_game.go` (insert case-arm after `case "setvis":` in super-mod switch)
- Modify: `modules/world/handler_cheats_supermod_test.go` (append 5 tests)

**Context:** TS `ClientCheatHandler.ts:569-581` — `::ban <username> <minutes>`. NodeProduction-gated. Calls `World.notifyPlayerBan(player.username, target, Date.now() + minutes*60*1000)`. Goscape routes via `s.loginBridgeMod.NotifyPlayerBan(staff, target, until)` (`bridges.go:28`).

- [ ] **Step 1: Write the failing happy-path ban test**

Append to `modules/world/handler_cheats_supermod_test.go`:

```go
// TestBanDispatchHappy pins TS L569-581 ::ban <username> <minutes>.
// Asserts recordingBridges.loginMod[0] = {method:NotifyPlayerBan,
// staff:p.username, username:<arg>, until:≈now+minutes}. Note: TS
// lowercases args (L42), so "bob" stays "bob".
func TestBanDispatchHappy(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d calls, want 1", len(rec.loginMod))
	}
	got := rec.loginMod[0]
	if got.method != "NotifyPlayerBan" {
		t.Errorf("method: got %q, want NotifyPlayerBan", got.method)
	}
	if got.staff != "alice" {
		t.Errorf("staff: got %q, want alice (the calling moderator)", got.staff)
	}
	if got.username != "bob" {
		t.Errorf("username: got %q, want bob", got.username)
	}
	wantUntil := before.Add(30 * time.Minute)
	if diff := got.until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+30m; want within 5s", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been banned for 30 minutes.")) {
		t.Errorf("MessageGame: missing ack; got %q", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestBanDispatchHappy -count=1
```

Expected: FAIL — `loginMod: got 0 calls, want 1` (no dispatch arm).

- [ ] **Step 3: Add the ban case-arm**

In `modules/world/handlers_game.go`, immediately after the `case "setvis":` block added in Task 2, insert:

```go
		case "ban":
			// TS ClientCheatHandler.ts:569-581 — ::ban <username> <minutes>.
			// NodeProduction-gated. Calls World.notifyPlayerBan with
			// staff=p.username (manual-staff invocation; distinct from the
			// "automated" callers at handler_reportabuse.go:50 and
			// handler_message_private.go:42). NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 || sub[0] == "" {
				p.MessageGame("Usage: ::ban <username> <minutes>")
				return nil
			}
			username := sub[0]
			minutes := parseIntOr(sub[1], 60)
			if minutes < 0 {
				minutes = 0
			}
			p.client.server.loginBridgeMod.NotifyPlayerBan(p.username, username, time.Now().Add(time.Duration(minutes)*time.Minute))
			p.MessageGame(fmt.Sprintf("Player '%s' has been banned for %d minutes.", username, minutes))
```

- [ ] **Step 4: Run to verify happy path passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestBanDispatchHappy -count=1
```

Expected: PASS.

- [ ] **Step 5: Add the remaining ban tests**

Append to `modules/world/handler_cheats_supermod_test.go`:

```go
// TestBanDispatchUsage pins TS L571-574: args.length < 2 → usage message,
// no bridge call.
func TestBanDispatchUsage(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0 on missing minutes arg", len(rec.loginMod))
	}
	if !bytes.Contains(out, []byte("Usage: ::ban <username> <minutes>")) {
		t.Errorf("MessageGame: missing usage; got %q", out)
	}
}

// TestBanDispatchUnparseableMinutes pins TS L578: tryParseInt default 60
// applied when minutes arg fails to parse.
func TestBanDispatchUnparseableMinutes(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob abc")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	wantUntil := before.Add(60 * time.Minute)
	if diff := rec.loginMod[0].until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+60m default", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been banned for 60 minutes.")) {
		t.Errorf("MessageGame: missing 60-minute ack; got %q", out)
	}
}

// TestBanDispatchNegativeClamp pins TS L578 Math.max(0, ...) — negative
// minutes clamps to 0.
func TestBanDispatchNegativeClamp(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob -5")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	// 0 minutes → until ≈ now.
	if diff := rec.loginMod[0].until.Sub(before); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now (0-min clamp)", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been banned for 0 minutes.")) {
		t.Errorf("MessageGame: missing 0-minute ack; got %q", out)
	}
}

// TestBanDispatchNodeProductionGate pins TS L569 && NODE_PRODUCTION.
// NodeProduction=false → arm inert (no bridge call, no message).
func TestBanDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, rec := supermodSetup(t)
	s.cfg.NodeProduction = false
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0 (NodeProduction=false)", len(rec.loginMod))
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}
```

- [ ] **Step 6: Run all ban tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestBanDispatch' -count=1 -race
```

Expected: PASS for all 5 (Happy, Usage, UnparseableMinutes, NegativeClamp, NodeProductionGate).

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handler_cheats_supermod_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-186 T3 — port ::ban cheat

Mirrors TS ClientCheatHandler.ts:569-581. NodeProduction-gated;
calls loginBridgeMod.NotifyPlayerBan with staff=p.username (manual-
staff invocation, distinct from automated callers). Default 60
minutes; clamped to ≥ 0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add `::mute` cheat dispatch arm

**Files:**
- Modify: `modules/world/handlers_game.go` (insert case-arm after `case "ban":` in super-mod switch)
- Modify: `modules/world/handler_cheats_supermod_test.go` (append 5 tests)

**Context:** TS `ClientCheatHandler.ts:582-594` — identical shape to ban; calls `notifyPlayerMute`, ack says "muted".

- [ ] **Step 1: Write the failing happy-path mute test**

Append to `modules/world/handler_cheats_supermod_test.go`:

```go
// TestMuteDispatchHappy pins TS L582-594 ::mute <username> <minutes>.
func TestMuteDispatchHappy(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d calls, want 1", len(rec.loginMod))
	}
	got := rec.loginMod[0]
	if got.method != "NotifyPlayerMute" {
		t.Errorf("method: got %q, want NotifyPlayerMute", got.method)
	}
	if got.staff != "alice" {
		t.Errorf("staff: got %q, want alice", got.staff)
	}
	if got.username != "bob" {
		t.Errorf("username: got %q, want bob", got.username)
	}
	wantUntil := before.Add(30 * time.Minute)
	if diff := got.until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+30m", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been muted for 30 minutes.")) {
		t.Errorf("MessageGame: missing ack; got %q", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestMuteDispatchHappy -count=1
```

Expected: FAIL — no dispatch arm.

- [ ] **Step 3: Add the mute case-arm**

In `modules/world/handlers_game.go`, immediately after the `case "ban":` block added in Task 3, insert:

```go
		case "mute":
			// TS ClientCheatHandler.ts:582-594 — ::mute <username> <minutes>.
			// NodeProduction-gated. Calls World.notifyPlayerMute with
			// staff=p.username. NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 || sub[0] == "" {
				p.MessageGame("Usage: ::mute <username> <minutes>")
				return nil
			}
			username := sub[0]
			minutes := parseIntOr(sub[1], 60)
			if minutes < 0 {
				minutes = 0
			}
			p.client.server.loginBridgeMod.NotifyPlayerMute(p.username, username, time.Now().Add(time.Duration(minutes)*time.Minute))
			p.MessageGame(fmt.Sprintf("Player '%s' has been muted for %d minutes.", username, minutes))
```

- [ ] **Step 4: Run to verify happy path passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestMuteDispatchHappy -count=1
```

Expected: PASS.

- [ ] **Step 5: Add the remaining mute tests**

Append to `modules/world/handler_cheats_supermod_test.go`:

```go
func TestMuteDispatchUsage(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0", len(rec.loginMod))
	}
	if !bytes.Contains(out, []byte("Usage: ::mute <username> <minutes>")) {
		t.Errorf("MessageGame: missing usage; got %q", out)
	}
}

func TestMuteDispatchUnparseableMinutes(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob abc")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	wantUntil := before.Add(60 * time.Minute)
	if diff := rec.loginMod[0].until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+60m default", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been muted for 60 minutes.")) {
		t.Errorf("MessageGame: missing 60-minute ack; got %q", out)
	}
}

func TestMuteDispatchNegativeClamp(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob -5")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	if diff := rec.loginMod[0].until.Sub(before); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now (0-min clamp)", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been muted for 0 minutes.")) {
		t.Errorf("MessageGame: missing 0-minute ack; got %q", out)
	}
}

func TestMuteDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, rec := supermodSetup(t)
	s.cfg.NodeProduction = false
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0", len(rec.loginMod))
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}
```

- [ ] **Step 6: Run all mute tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestMuteDispatch' -count=1 -race
```

Expected: PASS for all 5.

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handler_cheats_supermod_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-186 T4 — port ::mute cheat

Mirrors TS ClientCheatHandler.ts:582-594. NodeProduction-gated;
calls loginBridgeMod.NotifyPlayerMute with staff=p.username.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add `::kick` cheat dispatch arm

**Files:**
- Modify: `modules/world/handlers_game.go` (insert case-arm after `case "mute":` in super-mod switch)
- Modify: `modules/world/handler_cheats_supermod_test.go` (append 4 tests + a 1-line helper)

**Context:** TS `ClientCheatHandler.ts:595-616` — `::kick <username>`. NodeProduction-gated. Looks up target via `World.getPlayerByUsername`; on hit, sets `loggingOut=true` then inline `logout()+client.close()`. Goscape sets `loggingOut=true` and lets `processLogouts` (tick.go:277) handle teardown — DEVIATION-NAI-186-D1.

Helper pattern for kick fixture mirrors `reportAbuseSetupWithOnlineOffender` at `handler_reportabuse_test.go:315-325`: a second `newTestPlayer` wired to the same Server, `active=true`, appended to `s.playerLoop`, so `LookupPlayerByUsername` finds it.

- [ ] **Step 1: Add the kick helper to the test file**

Append to `modules/world/handler_cheats_supermod_test.go`:

```go
// kickAttachTarget wires a second Player as `targetName` into s.playerLoop
// (active=true) so s.LookupPlayerByUsername(targetName) returns it.
// Mirrors handler_reportabuse_test.go:315 reportAbuseSetupWithOnlineOffender.
func kickAttachTarget(t *testing.T, s *Server, targetName string) *Player {
	t.Helper()
	target, _ := newTestPlayer(t)
	target.client.server = s
	target.username = targetName
	target.active = true
	s.playerLoop = append(s.playerLoop, target)
	return target
}
```

- [ ] **Step 2: Write the failing happy-path kick test**

Append:

```go
// TestKickDispatchHappy pins TS L605-612: lookup hit → loggingOut=true +
// ack message. DEVIATION-NAI-186-D1: TS calls inline logout()+close();
// goscape defers teardown to processLogouts. Test pins loggingOut=true
// (the precondition for processLogouts to fire next tick).
func TestKickDispatchHappy(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	target := kickAttachTarget(t, s, "bob")

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick bob")
	p.client.flushWrite()
	out := <-received

	if !target.loggingOut {
		t.Error("target.loggingOut: must be true after ::kick (DEVIATION-NAI-186-D1: defers to processLogouts)")
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been kicked from the game.")) {
		t.Errorf("MessageGame: missing ack; got %q", out)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestKickDispatchHappy -count=1
```

Expected: FAIL — `target.loggingOut: must be true after ::kick` (no dispatch arm).

- [ ] **Step 4: Add the kick case-arm**

In `modules/world/handlers_game.go`, immediately after the `case "mute":` block added in Task 4, insert:

```go
		case "kick":
			// TS ClientCheatHandler.ts:595-616 — ::kick <username>.
			// NodeProduction-gated. Lookup via LookupPlayerByUsername; on
			// hit, set loggingOut=true and ack.
			//
			// DEVIATION-NAI-186-D1 — TS does inline `other.logout(); other.client.close()`
			// at L608-611. Goscape sets loggingOut=true and lets processLogouts
			// (tick.go:277) handle teardown (writeOut OpLogout + flushWrite +
			// conn.Close + s.removePlayer). Same end-state, ≤1 tick defer.
			// Retire if/when goscape grows a synchronous force-logout helper.
			//
			// NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			if args == "" {
				p.MessageGame("Usage: ::kick <username>")
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			username := sub[0]
			if other := p.client.server.LookupPlayerByUsername(username); other != nil {
				other.loggingOut = true
				p.MessageGame(fmt.Sprintf("Player '%s' has been kicked from the game.", username))
			} else {
				p.MessageGame(fmt.Sprintf("Player '%s' does not exist or is not logged in.", username))
			}
```

- [ ] **Step 5: Run to verify happy path passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestKickDispatchHappy -count=1
```

Expected: PASS.

- [ ] **Step 6: Add the remaining kick tests**

Append:

```go
// TestKickDispatchLookupMiss pins TS L613-615: target not online → "does
// not exist or is not logged in" message; no state mutation elsewhere.
func TestKickDispatchLookupMiss(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	// Do NOT call kickAttachTarget — playerLoop has only the caller.

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick ghost")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Player 'ghost' does not exist or is not logged in.")) {
		t.Errorf("MessageGame: missing lookup-miss ack; got %q", out)
	}
}

// TestKickDispatchUsage pins TS L597-600 args.length < 1 → usage message.
func TestKickDispatchUsage(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	target := kickAttachTarget(t, s, "bob")

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick")
	p.client.flushWrite()
	out := <-received

	if target.loggingOut {
		t.Error("target.loggingOut: must remain false on empty arg")
	}
	if !bytes.Contains(out, []byte("Usage: ::kick <username>")) {
		t.Errorf("MessageGame: missing usage; got %q", out)
	}
}

// TestKickDispatchNodeProductionGate pins TS L595 && NODE_PRODUCTION.
func TestKickDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	s.cfg.NodeProduction = false
	target := kickAttachTarget(t, s, "bob")

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick bob")
	p.client.flushWrite()
	out := <-received

	if target.loggingOut {
		t.Error("target.loggingOut: must remain false when NodeProduction=false")
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}
```

- [ ] **Step 7: Run all kick tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestKickDispatch' -count=1 -race
```

Expected: PASS for all 4.

- [ ] **Step 8: Run the full supermod suite to confirm cohesion**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestSetvis|TestBan|TestMute|TestKick|TestPlayerSetVisibility' -count=3 -race
```

Expected: PASS for all (-count=3 catches races flagged by memory `drainconn_iocopy_race`).

- [ ] **Step 9: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handler_cheats_supermod_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-186 T5 — port ::kick cheat

Mirrors TS ClientCheatHandler.ts:595-616. NodeProduction-gated;
sets target.loggingOut=true and lets processLogouts handle teardown
(DEVIATION-NAI-186-D1: same end-state as TS inline logout+close).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Retire super-mod cluster from carryforward block

**Files:**
- Modify: `modules/world/handlers_game.go:367-381` — replace the NAI-185-D4 carryforward block with the NAI-186-D2 version.

**Context:** The NAI-185 close commit (b2631d4) left a 15-line comment block at L367-381 listing 10 unported cheats in 3 clusters. NAI-186 completes the super-mod cluster — drop those 4 cheats from the listing; supersede the tag.

- [ ] **Step 1: Read the current carryforward block**

Read `modules/world/handlers_game.go:367-381` (or as appropriate based on line drift from Tasks 2-5; locate via grep `DEVIATION-NAI-185-D4-CARRYFORWARD`):

```bash
grep -n 'DEVIATION-NAI-185-D4-CARRYFORWARD\|Dev block\|Admin block\|Super-mod' modules/world/handlers_game.go
```

The current block (verbatim from spec pre-flight) reads:

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

- [ ] **Step 2: Replace the block**

Use the Edit tool to replace the entire 15-line block (from `// DEVIATION-NAI-185-D4-CARRYFORWARD — supersedes` through `// Each cluster warrants its own follow-up sub-spec.`) with:

```go
	// DEVIATION-NAI-186-D2-CARRYFORWARD — supersedes
	// DEVIATION-NAI-185-D4-CARRYFORWARD. 6 TS ClientCheatHandler
	// cheats remain unported:
	//   Dev block (!NP && >=4): reload, rebuild, speed.
	//     Blocked on cache/script reload subsystem + runtime
	//     tick-rate mutation (tick.go interval is currently fixed).
	//   Admin block (>=3):      locadd, npcadd, openmain.
	//     Blocked on dynamic Loc/Npc spawn + interface routing.
	// NAI-186 retired the super-mod cluster (setvis/ban/mute/kick).
	// Each cluster warrants its own follow-up sub-spec.
```

- [ ] **Step 3: Verify the build is still green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -race
```

Expected: build OK; all tests PASS (test count up by ~20 from NAI-186 cohort).

- [ ] **Step 4: Audit deviation-tag references**

Per memory `retire_deviation_grep_all_comments`, enumerate any stale references to the old tag:

```bash
rg -n 'DEVIATION-NAI-185-D4' modules/ pkg/ docs/
```

Expected: 0 hits in `modules/` and `pkg/`. `docs/` may retain references in historical specs — leave those untouched.

```bash
rg -n 'DEVIATION-NAI-186-D1\|DEVIATION-NAI-186-D2' modules/ pkg/
```

Expected: 1+ hits each (D1 in kick code-comment + kick test; D2 in carryforward block).

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-186 — rewrite carryforward block, retire super-mod

Retires super-mod cluster (setvis/ban/mute/kick) from the NAI-185-D4
carryforward block at handlers_game.go:367. Supersedes the tag to
DEVIATION-NAI-186-D2-CARRYFORWARD. Remaining clusters: Dev block
(reload/rebuild/speed) and Admin block (locadd/npcadd/openmain).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Cohort close + final verification

**Files:** none (verification-only)

**Context:** Final cross-verification of the cohort. Per memory `verify_implementer_claims`, run the full test suite with race detection and fresh cache so no stale results leak in.

- [ ] **Step 1: Run the full modules/world test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -race
```

Expected: PASS. Note any new failures — pre-existing failures (if any) should be confirmed against HEAD~7 (the pre-NAI-186 commit) per memory `verify_implementer_claims`.

- [ ] **Step 2: Verify deviation tags**

```bash
rg -n 'DEVIATION-NAI-186' modules/ pkg/ docs/superpowers/specs/
```

Expected layout:
- `modules/world/handlers_game.go` — D1 in kick case-arm code-comment; D2 in carryforward block (2 hits in handlers_game.go).
- `modules/world/handler_cheats_supermod_test.go` — D1 referenced in TestKickDispatchHappy doc-comment (1 hit).
- `modules/world/player.go` — D1 cohort mention in SetVisibility doc-comment (1 hit, optional).
- `docs/superpowers/specs/2026-05-12-nai-186-supermod-cheats-design.md` — multiple hits in the ledger (expected, frozen historical doc).
- `docs/superpowers/plans/2026-05-12-nai-186-supermod-cheats.md` — multiple hits in this plan doc.

No hits expected for `DEVIATION-NAI-185-D4-CARRYFORWARD` outside `docs/`.

- [ ] **Step 3: Cohort smoke (manual server invocation) — optional**

If the user runs a local server + Java client smoke session (per memory `smoke_test_server_handoff`, server launches must be user-driven), the workflow is:

```bash
# user runs:
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
# In config.yaml, set world.node_production: true (or pass --world.node-production)
# AND seat a test account with staffmodlevel=2.
# In Java client: ::setvis 1   →  message "vis: 1 (not implemented...)"
#                 ::setvis 0   →  "vis: 0"
#                 ::ban junk 1 →  ack
#                 ::kick junk  →  ack + kick observed in second client session
```

If no smoke session is run, leave as plan-time-only verification.

- [ ] **Step 4: Final close commit (if Step 1 and Step 2 are green)**

```bash
git log --oneline -8
```

Verify the last 6-7 commits read: T1, T2, T3, T4, T5, T6, plus the original `spec(nai-186)`. Then:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-186 — super-mod cheats cohort complete

Ports the 4 staffModLevel>=2 && NodeProduction super-mod cheats
(setvis/ban/mute/kick) from TS ClientCheatHandler.ts:549-616.
Retires the super-mod cluster from the NAI-185-D4 carryforward
block; 6 cheats remain (Dev block + Admin block) and are tracked
under DEVIATION-NAI-186-D2-CARRYFORWARD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Empty close commit is OPTIONAL — per recent close-commit precedent at b2631d4 / 5cbd949, a close-commit is conventional but not required if Task 6's commit already captures the cluster-retirement narrative. Use `--allow-empty` only if there's no content to commit; otherwise let Task 6's commit BE the close commit and skip this step.)

- [ ] **Step 5: Update memory if any non-derivable lessons surfaced**

Per memory `nai_followups` and the global "save when surprising / non-obvious" rule:

- If the SetVisibility port surfaced a TS-only quirk worth recording (e.g. the SOFT message-only stub is not documented elsewhere), add a memory entry.
- If implementer found that the `parseIntOr` + `< 0` clamp ordering matters in some subtle way, record it.
- Otherwise — and likely — no new memory entries; the spec's pre-flight `tracker_entry_framing_can_be_incomplete` callout is already-captured wisdom.

If adding a memory entry, also add `Closes memory: <slug>.md` trailer to the close commit per memory `close_commit_memory_trailer`.

---

## Self-review (plan-author internal pass)

### Spec coverage

| Spec section | Coverage |
|--------------|----------|
| §1 Goal — 4 cheats from TS L549-616 | T1 (SetVisibility) + T2 (setvis) + T3 (ban) + T4 (mute) + T5 (kick) |
| §3.1 setvis TS source                | T1 method body matches L1875-1891 verbatim; T2 dispatch matches L549-568 |
| §3.2 ban TS source                   | T3 dispatch matches L569-581 |
| §3.3 mute TS source                  | T4 dispatch matches L582-594 |
| §3.4 kick TS source                  | T5 dispatch matches L595-616 (with D1 deviation) |
| §4.1 SetVisibility method            | T1 |
| §4.2 4 dispatch arms                 | T2-T5 |
| §4.3 carryforward update             | T6 |
| §5.2 parseIntOr + clamp              | T3/T4 happy + unparseable + negative tests cover all rows of the table |
| §5.3 until-time formula              | T3/T4 tests pin via ±5s tolerance |
| §5.4 staff arg                       | T3/T4 happy pin staff="alice" (manual-staff, not "automated") |
| §5.5 D1 kick deviation               | T5 documented in code-comment + test doc-comment |
| §6 Error-handling table              | All 11 rows covered across T2-T5 |
| §7.3 test cases                      | setvis 6 cases (T2 step 1+5×5); ban 5 (T3); mute 5 (T4); kick 4 (T5) — 20 total |
| §8 files                             | player.go (T1), handlers_game.go (T2-T6), new test file (T2-T5) |
| §9 R1-R8 risks                       | All resolved at plan-write (top of plan) |
| §10 D1+D2 deviations                 | Both tagged in T5 + T6 |
| §11 closure                          | T6 + T7 |

No gaps.

### Placeholder scan

- No "TBD", "TODO", "implement later" outside the deferred-cohort discussion (which IS the topic).
- All code blocks are complete; no "..." in implementation snippets.
- Test code is fully written; no "add appropriate assertions" hand-waving.
- Commit messages are concrete with cohort tags.

### Type consistency

- `rsbuf.Visibility` type used consistently (T1 method param; T2 case-arm enum lookups).
- `BlockWalkNpc` / `BlockWalkNone` used (NOT `BlockWalkNPC` / `BlockWalkNONE`) — matches `modules/world/player.go:523` precedent.
- `s.gamemap.ChangeNPCCollision` (caps "NPC") — matches `pkg/gamemap/gamemap.go:120`.
- `collision.FlagBlockNPCs` / `FlagBlockPlayers` — matches `npc_registry_test.go:206`.
- `recordingBridges.loginMod[i].method/staff/username/until` — matches `bridges_test.go:18,76`.
- `LookupPlayerByUsername` — matches `server.go:891`.
- `loginBridgeMod.NotifyPlayerBan/Mute` — matches `bridges.go:28-29`, `handler_reportabuse.go:50`.
- `parseIntOr(s, def)` — matches `handlers_game.go:853`.
- `dispatchCheat` (NOT `dispatchTeleCheat`) is a NEW helper added in T2 to avoid colliding with the existing `dispatchTeleCheat` in `handlers_game_test.go:392`; intentional naming distinction.

No inconsistencies.
