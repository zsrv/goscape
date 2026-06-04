# NAI-183 — Port TS ClientCheatHandler outer guards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure `handleClientCheat` into three TS-faithful gate blocks (L52 addSessionLog, L56 dev block w/ NodeProduction guard, L483 super-mod block) and retire `DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE` (4 references).

**Architecture:** Spec at `docs/superpowers/specs/2026-05-12-nai-183-cheat-outer-guard-design.md`. Three sequential top-level guard blocks replace the per-arm `staffModLevel < 2` gates. `::tele`/`::getcoord` move under the `>=2` super-mod block (TS L483); `::reboot`/`::slowreboot`/`::serverdrop` move under the `!NodeProduction && >=4` dev block (TS L56). The `::reboot`/`::slowreboot` inner `&& NodeProduction` clauses are preserved as TS-faithful dead code (TS quirk; see comments). `::say` stays ungated (not in TS ClientCheatHandler). The L52 `addSessionLog` tier is ported using existing `Player.AddSessionLog` infra.

**Tech Stack:** Go 1.26+

---

## File map

- **Modify:** `modules/world/handlers_game.go` — restructure `handleClientCheat` body (lines 367-466 of HEAD `bc97189`).
- **Modify:** `modules/world/handlers_game_test.go` — modify 6 tests (lines 509-641 region), add 3 tests, retire D2 doc-comment reference.

No new files. No changes outside these two files.

---

## Task 1: Update tests to reflect TS-faithful semantics (TDD red)

This task modifies 6 existing tests and adds 3 new tests in `modules/world/handlers_game_test.go`. After this task, `go test ./modules/world/ -run TestHandleClientCheat` will fail in expected ways (the new + modified tests pin TS-faithful behavior; the unchanged code still uses per-arm `<2` gates).

**Files:**
- Modify: `modules/world/handlers_game_test.go:509-641`

### Step 1: Replace the 4 reboot/slowreboot tests with their inverted, dead-code-pinning forms

- [ ] Replace the entire block from line 509 ("`// --- NAI-182 B6: ::reboot / ::slowreboot / ::serverdrop staff cheats ---`") through `TestHandleClientCheat_SlowReboot_NonIntegerArgFallsBackToDefault` (line 605) with:

```go
// --- NAI-183: ::reboot / ::slowreboot dev-block dead-code pins ---

// TS ClientCheatHandler.ts:360-373 places ::reboot and ::slowreboot
// under `if (!Environment.NODE_PRODUCTION && staffModLevel >= 4)` with
// inner `&& Environment.NODE_PRODUCTION` clauses. Inside that outer
// block NODE_PRODUCTION is always false, so those inner clauses are
// dead. goscape preserves the TS-faithful structure verbatim, so
// ::reboot / ::slowreboot do NOT fire under default config
// (cfg.NodeProduction=false). NAI-183.

// TestHandleClientCheat_Reboot_DeadUnderDefaultConfig pins the TS-faithful
// dead-code semantics for ::reboot at staffModLevel=4 with the default
// cfg.NodeProduction=false: the inner `&& NodeProduction` clause blocks
// the rebootTimer call, so shutdownTick stays at its newTestServer
// initial value (-1). Mirrors TS ClientCheatHandler.ts:360-364.
func TestHandleClientCheat_Reboot_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	go io.Copy(io.Discard, cc) // keep pipe unblocked

	dispatchTeleCheat(t, p, "reboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::reboot under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_NoArgs_DeadUnderDefaultConfig — same
// dead-code pin for ::slowreboot with no args. Mirrors TS L365-373.
func TestHandleClientCheat_SlowReboot_NoArgs_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::slowreboot under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_WithArg_DeadUnderDefaultConfig — same
// dead-code pin for ::slowreboot with a seconds arg. NAI-183.
func TestHandleClientCheat_SlowReboot_WithArg_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot 60")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::slowreboot 60 under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_NonInteger_DeadUnderDefaultConfig —
// same dead-code pin for the tryParseInt-fallback arg path. NAI-183.
func TestHandleClientCheat_SlowReboot_NonInteger_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot abc")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::slowreboot abc under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}
```

### Step 2: Update `TestHandleClientCheat_ServerDrop_ClosesConn` to use staffModLevel=4

- [ ] Locate the existing test at `TestHandleClientCheat_ServerDrop_ClosesConn` (line 611). Replace `p.staffModLevel = 2` with `p.staffModLevel = 4`. Update the leading doc-comment to reference NAI-183:

```go
// TestHandleClientCheat_ServerDrop_ClosesConn pins that ::serverdrop
// closes the TCP connection but leaves the player in s.players so that
// the next reconnect hits the same slot (onReconnect path).
// Mirrors TS ClientCheatHandler.ts:374-376 player.terminate(). Gated
// under TS L56 dev block (!NodeProduction && staffModLevel >= 4); fires
// because TS ::serverdrop has no inner `&& NodeProduction` clause.
// NAI-183.
func TestHandleClientCheat_ServerDrop_ClosesConn(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	slotBefore := p.slot
	_ = cc

	dispatchTeleCheat(t, p, "serverdrop")

	if s.players[slotBefore] != p {
		t.Errorf("player removed from slot %d after ::serverdrop; should remain for reconnect", slotBefore)
	}
	if _, err := p.client.conn.Write([]byte{0}); err == nil {
		t.Error("p.client.conn.Write succeeded after ::serverdrop; expected closed-conn error")
	}
}
```

### Step 3: Update `TestHandleClientCheat_RebootCheats_StaffGate` to gate at <4 and retire D2 reference

- [ ] Locate the existing test at line 631. Replace its body with (note the rename is optional — keeping the name keeps git-blame continuity):

```go
// TestHandleClientCheat_RebootCheats_StaffGate pins that ::reboot is
// silently rejected (shutdownTick unchanged) when p.staffModLevel < 4
// (the TS L56 dev-block gate). NAI-183.
func TestHandleClientCheat_RebootCheats_StaffGate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3 // below the dev-block gate (>=4)
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "reboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::reboot with staffModLevel=3: got %d, want -1 (gate blocked)", s.shutdownTick)
	}
}
```

### Step 4: Add the three new tests after `TestHandleClientCheat_RebootCheats_StaffGate`

- [ ] Append at the end of the file (after the existing closing brace of the previous test):

```go
// --- NAI-183: outer-guard restructure tests ---

// TestHandleClientCheat_AddsSessionLogAtModLevel2 pins the TS L52-54
// `if (staffModLevel >= 2) addSessionLog(MODERATOR, 'Ran cheat', cheat)`
// tier. Dispatches an unrecognized cheat ("foo") so no arm body fires
// and the test isolates the L52 tier. Below modLevel 2, no entry is
// pushed. Join semantics: `message + " " + strings.Join(args, " ")` →
// "Ran cheat foo" (cheat is the lowercased input WITHOUT the stripped
// "::" prefix per handlers_game.go:345-347). NAI-183.
func TestHandleClientCheat_AddsSessionLogAtModLevel2(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	go io.Copy(io.Discard, cc)

	// At staffModLevel=2 the L52 tier fires.
	p.staffModLevel = 2
	dispatchTeleCheat(t, p, "foo")

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs after L52-tier dispatch at staffModLevel=2: got %d, want 1", got)
	}
	if got := s.sessionLogs[0].EventType; got != LoggerEventTypeModerator {
		t.Errorf("EventType: got %d, want %d (LoggerEventTypeModerator)", got, LoggerEventTypeModerator)
	}
	if got := s.sessionLogs[0].Event; got != "Ran cheat foo" {
		t.Errorf("Event: got %q, want %q", got, "Ran cheat foo")
	}

	// Below the gate: no entry pushed.
	s.sessionLogs = s.sessionLogs[:0]
	p.staffModLevel = 1
	dispatchTeleCheat(t, p, "foo")

	if got := len(s.sessionLogs); got != 0 {
		t.Errorf("sessionLogs at staffModLevel=1: got %d, want 0 (below L52 gate)", got)
	}
}

// TestHandleClientCheat_ServerDrop_StaffGate pins that ::serverdrop is
// silently rejected when p.staffModLevel < 4 (TS L56 dev-block gate).
// Sibling of TestHandleClientCheat_RebootCheats_StaffGate. NAI-183.
func TestHandleClientCheat_ServerDrop_StaffGate(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 3 // below the dev-block gate (>=4)
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "serverdrop")

	if _, err := p.client.conn.Write([]byte{0}); err != nil {
		t.Errorf("p.client.conn.Write failed after ::serverdrop at staffModLevel=3: %v; want success (gate blocked, conn still open)", err)
	}
}

// TestHandleClientCheat_NodeProductionTrue_DevBlockShortCircuits pins
// that flipping cfg.NodeProduction=true collapses the entire TS L56
// `!NodeProduction && >=4` dev block. ::serverdrop (which fires under
// default config at modLevel=4) does NOT fire. The L52 addSessionLog
// tier still fires (gated independently on >=2 only). NAI-183.
func TestHandleClientCheat_NodeProductionTrue_DevBlockShortCircuits(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	s.cfg.NodeProduction = true
	p.staffModLevel = 4
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "serverdrop")

	if _, err := p.client.conn.Write([]byte{0}); err != nil {
		t.Errorf("p.client.conn.Write failed after ::serverdrop with NodeProduction=true: %v; want success (dev block collapsed)", err)
	}
	if got := len(s.sessionLogs); got != 1 {
		t.Errorf("sessionLogs: got %d, want 1 (L52 tier should fire independently of NodeProduction)", got)
	}
}
```

### Step 5: Run the test suite to verify the expected red state

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleClientCheat -v`
- [ ] Expected: compile succeeds. The 4 renamed dead-code tests FAIL (current code's per-arm `<2` gate plus modLevel=4 setup means the arm bodies still fire and mutate `shutdownTick`). `TestHandleClientCheat_ServerDrop_ClosesConn` PASSES (still works at modLevel=4 since current per-arm gate is `<2`). `TestHandleClientCheat_RebootCheats_StaffGate` at modLevel=3 PASSES (current `<2` gate also rejects). The 3 new tests FAIL (no addSessionLog wiring; ::serverdrop fires under current per-arm-only gate even with NodeProduction=true).
- [ ] Expected failure summary: at minimum the 4 dead-code-pin tests + `TestHandleClientCheat_AddsSessionLogAtModLevel2` + `TestHandleClientCheat_NodeProductionTrue_DevBlockShortCircuits`. Do NOT proceed to Task 2 unless the failure pattern matches.

### Step 6: Commit the test-only red state

- [ ] Run:

```bash
git add modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-183 — pin TS-faithful cheat outer-guard semantics (red)

Modify 6 reboot-cohort tests + add 3 outer-guard tests (addSessionLog
tier, ServerDrop staff gate, NodeProduction=true short-circuit). Tests
fail against current per-arm <2 gates; Task 2 restructures the handler
to make them pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Restructure `handleClientCheat` into three outer guards (TDD green) + retire D2

This task replaces the per-arm `staffModLevel < 2` gate pattern with three sequential top-level guard blocks mirroring TS L52, L56, L483. After this task, the test suite passes and grep for `DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE` returns zero hits.

**Files:**
- Modify: `modules/world/handlers_game.go:367-466`

### Step 1: Replace the body of `handleClientCheat` from `parts[0]` switch onward

- [ ] Locate the existing block at handlers_game.go:367 (the `switch parts[0] {` line). Replace everything from that line through the closing `}` of the function body at line 466 with:

```go
	// TS ClientCheatHandler.ts:52-54 — addSessionLog tier. Logs every
	// cheat invocation from staffModLevel >= 2 to the MODERATOR session
	// log channel. Ported via Player.AddSessionLog (modules/world/player.go).
	// Join semantics: "Ran cheat" + " " + cheat. NAI-183.
	if p.staffModLevel >= 2 {
		p.AddSessionLog(LoggerEventTypeModerator, "Ran cheat", cheat)
	}

	// TS ClientCheatHandler.ts:56 — developer block. Gated on
	// `!Environment.NODE_PRODUCTION && staffModLevel >= 4`. Goscape
	// reads s.cfg.NodeProduction (modules/world/config.go:43, default
	// false). NAI-183.
	if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
		switch parts[0] {
		case "reboot":
			// Mirrors TS L360-364. TS-faithful dead code: the inner
			// `&& NodeProduction` clause never fires because the outer
			// block runs only when NodeProduction=false. Preserved
			// verbatim to mirror the TS quirk (likely refactor
			// artifact). NAI-183.
			if p.client.server.cfg.NodeProduction {
				s := p.client.server
				s.rebootTimer(0)
			}
		case "slowreboot":
			// Mirrors TS L365-373. Same TS-faithful dead-code pattern
			// as ::reboot above. Default 30 seconds when args is
			// missing/unparseable (TS tryParseInt semantics); formula
			// ticks = ceil(seconds * 1000 / 600). NAI-183.
			if p.client.server.cfg.NodeProduction {
				seconds := parseIntOr(args, 30)
				ticks := int(math.Ceil(float64(seconds) * 1000.0 / 600.0))
				s := p.client.server
				s.rebootTimer(ticks)
			}
		case "serverdrop":
			// Mirrors TS L374-376 player.terminate(). No inner clause
			// in TS — actually fires under the outer !NodeProduction
			// guard. Closes the TCP conn without removing the player
			// from s.players; the next reconnect hits this player's
			// slot and runs the onReconnect path. NAI-183.
			if p.client != nil && p.client.conn != nil {
				_ = p.client.conn.Close()
			}
		}
	}

	// TS ClientCheatHandler.ts:483 — super-mod block. Gated on
	// staffModLevel >= 2. NAI-183.
	if p.staffModLevel >= 2 {
		switch parts[0] {
		case "getcoord":
			// Mirrors TS L489 — `::getcoord` displays the player's
			// current coord as level,mapX,mapZ,localX,localZ.
			p.MessageGame(coordgrid.FormatString(p.level, p.x, p.z, ","))
		case "tele":
			// Mirrors TS `::tele level,mapX,mapZ[,localX,localZ]` at
			// ClientCheatHandler.ts:491-524. Single-arg form:
			// "::tele 0,50,50,32,32".
			//
			// NAI-93 closed the prior DEVIATION block here: closeModal,
			// canAccess gate, and the unsetMapFlag bundle (sendUnsetMapFlag
			// + waypointIndex reset, per TS Player.unsetMapFlag at
			// Player.ts:2169-2172) are now wired. ClearInteraction
			// preserved.
			if args == "" {
				return nil
			}
			coord := strings.Split(args, ",")
			if len(coord) < 3 {
				return nil
			}

			// Pre-tele cleanup chain — order per TS lines 504-512.
			p.CloseModal(true) // TS closeModal() default-arg.
			if !p.CanAccess() {
				p.MessageGame("Please finish what you are doing first.")
				return nil
			}
			p.ClearInteraction()
			sendUnsetMapFlag(p)
			p.waypointIndex = -1 // TS Player.unsetMapFlag → clearWaypoints.

			level := parseIntOr(coord[0], 0)
			mx := parseIntOr(coord[1], 50)
			mz := parseIntOr(coord[2], 50)
			lx := 32
			if len(coord) > 3 {
				lx = parseIntOr(coord[3], 32)
			}
			lz := 32
			if len(coord) > 4 {
				lz = parseIntOr(coord[4], 32)
			}
			if level < 0 || level > 3 || mx < 0 || mx > 255 || mz < 0 || mz > 255 || lx < 0 || lx > 63 || lz < 0 || lz > 63 {
				return nil
			}
			p.TeleJump((mx<<6)+lx, (mz<<6)+lz, level)
		}
	}

	// Ungated arms. ::say has no TS counterpart in ClientCheatHandler
	// (TS routes it through ChatHandler instead); kept ungated. NAI-183.
	switch parts[0] {
	case "say":
		if args != "" {
			p.Say([]byte(args))
		}
	}

	return nil
}
```

Note: the `bytes`, `math`, `strconv`, `strings` imports and the `coordgrid`, `packet` imports remain unchanged. The DEVIATION-NAI-182-D3-OTHER-CHEATS comment block at lines 360-366 stays in place (above the new code).

### Step 2: Run the test suite to verify all tests pass

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleClientCheat -v`
- [ ] Expected: ALL `TestHandleClientCheat_*` tests PASS, including the 4 dead-code-pin tests, the 3 new tests, and the unchanged `Tele`/`GetCoord` tests.

### Step 3: Run the full module suite + race detector

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
- [ ] Expected: PASS with no regressions.
- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/`
- [ ] Expected: PASS with no race detections.

### Step 4: Verify D2 retirement — grep returns zero

- [ ] Run: `rg "DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE" modules/ pkg/ cmd/`
- [ ] Expected: zero output (no remaining references). If any hits remain, locate and remove them before commit.

### Step 5: Verify whole-repo build

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- [ ] Expected: clean build, no errors.
- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
- [ ] Expected: full repo passes.

### Step 6: Commit the close

- [ ] Run:

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-183 — port TS ClientCheatHandler outer guards; retire D2

Restructure handleClientCheat into three sequential top-level blocks
mirroring TS L52,L56,L483: addSessionLog tier (>=2), dev block
(!NodeProduction && >=4) wrapping ::reboot/::slowreboot/::serverdrop,
super-mod block (>=2) wrapping ::tele/::getcoord. Preserve TS-faithful
dead code on ::reboot/::slowreboot (inner `&& NodeProduction` under
outer `!NodeProduction`; never fires; mirrors likely TS refactor
artifact). ::say stays ungated (not in TS ClientCheatHandler). Port
the L52 addSessionLog("Ran cheat", cheat) call via existing
Player.AddSessionLog infra. Retires DEVIATION-NAI-182-D2-CHEAT-NODE-
PRODUCTION-GATE (4 references).

Closes memory: docs/superpowers/specs/2026-05-12-nai-183-cheat-outer-guard-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review

- **Spec coverage:** §3.1 target shape → Task 2 Step 1. §3.2 behavioral matrix → Task 1 Steps 1-4 (test pins). §4.1 retirement → Task 2 Step 4 (grep verification). §5.1 modified tests → Task 1 Steps 1-3. §5.2 added tests → Task 1 Step 4. §5.3 unchanged tests → not touched (no task needed). §7 controller pre-flight → embedded in Task 1 Step 5 (red verification) + Task 2 Steps 2-5 (green + grep). All sections covered.
- **Placeholder scan:** No TBD/TODO/"add appropriate handling" anywhere; full code blocks for every modify/insert; exact bash commands for every verification.
- **Type consistency:** `LoggerEventTypeModerator` (used in Task 1 Step 4 + Task 2 Step 1) — confirmed as exported alias at modules/world/session_log.go. `p.AddSessionLog` signature `(eventType LoggerEventType, message string, args ...string)` — verified at modules/world/player.go:1321. `s.cfg.NodeProduction` access path — verified against sibling guards (handler_reportabuse.go, server_varp.go) + idiom (player_varp_probe_test.go uses `s.cfg.NodeDebug = true`). `s.shutdownTick` initial value `-1` — verified at modules/world/server_test.go:321.
