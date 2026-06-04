# NAI-72 — Social subsystem foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 6 silent-discard game opcodes (CHAT_SETMODE, FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL, REPORT_ABUSE) into `modules/world/`, mirroring TS handlers line-by-line. Adds 2 Player fields (socialProtect + reportAbuseProtect), a per-tick reset hook, a 3-bridge interface trio (no-op default + recording test capture), and a ReportAbuseReason enum. Folds in one true-to-TS bug fix in `pkg/util/jstring.FromBase37`. Net deviation tally 14 → 18 (post-erratum).

**Erratum (2026-05-02, post-`ba3fa3f`):** Pre-flight grep before T1 dispatch surfaced that `Player.staffModLevel int32` already exists at `player.go:73` with a producer at `server.go:590` (login proto). The `NAI-72-D-STAFFMODLEVEL-DEAD-WRITER` deviation was retracted, T1 Step 8 dropped its staffModLevel field-add, T4 Step 3 dropped its "no producer at HEAD" doc-comment, Stage 1 review focus #6 dropped, commit-msg templates corrected. ReportAbuse handler reads `p.staffModLevel` (existing `int32` field) directly — comparison `p.staffModLevel > 0` works unchanged. ReportAbuse tests still set `p.staffModLevel = 1` to drive the moderator-mute branch deterministically. See spec erratum block.

**Architecture:** Six free-function handlers (`p.client.server` access pattern, matching NAI-71's `handler_opheld.go`) registered directly in `handlers_game.go init()`. Foundation task (T1) lays bridge interfaces, default no-op impl, recording capture for tests, Player flag fields, processCleanup hook, ReportAbuseReason enum, Server fields, and NewServer init. T2-T4 each port one handler family with its tests. Two-stage Sonnet review (after T1, after T4).

**Tech Stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`. Test idioms from `handler_opheld_test.go`, `handler_inv_button_test.go`. Test fixtures: `newTestPlayer(t)` + `newTestServer(t)` from `server_test.go:311`, wired via `p.client.server = s`.

**Spec:** `docs/superpowers/specs/2026-05-02-nai-72-social-subsystem-foundation-design.md`.

**Predecessor commit:** `1617869` (NAI-72 spec). HEAD entering: `1617869`.

---

## Pre-flight (controller, before each task dispatch)

Per `controller_preflight.md` and `spec_followup_tracker_freshness.md`, controller re-verifies before each task that spec premises still hold at HEAD. Run from repo root.

**Before T1:**
```bash
# Confirm absence of all NAI-72 surfaces
# (Note: staffModLevel is EXPECTED to have hits — it already exists at
# player.go:73 with login-proto producer. Erratum dropped its field-add.)
rg -n "socialProtect|reportAbuseProtect" pkg/ modules/
rg -n "FriendsBridge|LoginBridgeMod|LoggerBridge|noopBridges|recordingBridges" modules/
rg -n "ReportAbuseReason" pkg/ modules/
rg -n "gameHandlers\[(11|79|118|171|190|244)\]" modules/

# Confirm dependencies exist
rg -n "func FromBase37" pkg/util/jstring/
rg -n "NodeProduction\b" modules/world/config.go modules/world/server_varp.go
rg -n "func \(p \*Player\) ResetMasks\(\)" modules/world/player_masks.go
rg -n "func \(p \*Player\) MessageGame\(" modules/world/message_game.go
rg -n "func newTestPlayer\(|func newTestServer\(" modules/world/

# Confirm processCleanup hook site is unchanged
rg -n "func \(s \*Server\) processCleanup\(\)" modules/world/tick.go
```

**Before T2/T3/T4:** re-grep `gameHandlers\[N\]` for the specific opcode being added (T1 may shift line numbers in `handlers_game.go`); re-grep `socialProtect`/`reportAbuseProtect` to confirm T1 landed; re-grep `FriendsBridge`/`LoginBridgeMod`/`LoggerBridge` to confirm interfaces exist.

**Before T4 specifically:** re-read TS `ReportAbuseHandler.ts:9-26` AND `ReportAbuse.ts:4-17` (enum) to confirm no upstream drift, and verify `cfg.NodeProduction` is still readable from a Player handler via `p.client.server.cfg`.

---

## Task 1 — Foundation (jstring fix + Player fields + processCleanup + bridges + enum + Server fields)

**Files:**
- Modify: `pkg/util/jstring/jstring.go` (add 3-line `% 37 == 0` check after line 38)
- Create: `pkg/util/jstring/jstring_test.go` (or extend if exists)
- Modify: `modules/world/player.go` (add 3 fields after line 176)
- Modify: `modules/world/tick.go` (extend `processCleanup` player loop at line 475)
- Modify: `modules/world/server.go` (add 3 Server fields after line 119; init in NewServer)
- Create: `modules/world/bridges.go`
- Create: `modules/world/bridges_test.go`
- Create: `modules/world/social.go`
- Create: `modules/world/social_test.go`
- Create: `modules/world/tick_social_reset_test.go` (or extend existing tick test file)

### T1 Step 1: Write failing test for `FromBase37` `% 37 == 0` invalid_name branch

- [ ] Check if `pkg/util/jstring/jstring_test.go` exists.

```bash
ls pkg/util/jstring/
```

If absent, create it with:

```go
package util

import "testing"

func TestFromBase37InvalidNameUpperBound(t *testing.T) {
    // 6582952005840035281 = 37**12 — sentinel TS line 38.
    if got := FromBase37(6582952005840035281); got != "invalid_name" {
        t.Errorf("upper-bound FromBase37: got %q, want %q", got, "invalid_name")
    }
}

func TestFromBase37InvalidNameMod37(t *testing.T) {
    // Any nonzero multiple of 37 must return "invalid_name" per TS
    // JString.ts:42-44. Pre-NAI-72 goscape returns the decoded string.
    cases := []uint64{37, 74, 1369, 37 * 12345}
    for _, v := range cases {
        if got := FromBase37(v); got != "invalid_name" {
            t.Errorf("FromBase37(%d): got %q, want %q", v, got, "invalid_name")
        }
    }
}

func TestFromBase37ValidNameDecodes(t *testing.T) {
    // Sanity check: a valid encoded name round-trips through ToBase37.
    name := "alice"
    encoded := ToBase37(name)
    if got := FromBase37(encoded); got != name {
        t.Errorf("FromBase37(ToBase37(%q)): got %q, want %q", name, got, name)
    }
}
```

If the file exists, append the two `InvalidName*` tests + the round-trip sanity test.

- [ ] **Step 2: Run the test, verify the `% 37 == 0` test fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/util/jstring/ -run "TestFromBase37InvalidName" -v
```

Expected: `TestFromBase37InvalidNameUpperBound` PASS; `TestFromBase37InvalidNameMod37` FAIL (returns decoded string instead of `"invalid_name"`); `TestFromBase37ValidNameDecodes` PASS.

- [ ] **Step 3: Apply the fix in `pkg/util/jstring/jstring.go`**

Edit lines 36-38. The current block is:

```go
func FromBase37(v uint64) string {
    // >= 37 to the 12th power
    if v < 0 || v >= 6582952005840035281 {
        return "invalid_name"
    }
```

Replace with:

```go
func FromBase37(v uint64) string {
    // >= 37 to the 12th power
    if v >= 6582952005840035281 {
        return "invalid_name"
    }

    // Mirrors TS JString.ts:42-44 — values divisible by 37 are invalid
    // (NAI-72: surfaced by social handler invalid_name gate).
    if v != 0 && v%37 == 0 {
        return "invalid_name"
    }
```

Note: also drops the impossible `v < 0` guard (`v` is `uint64`); TS uses bigint where the guard makes sense. The `v != 0` guard preserves the existing zero-input behavior (the upper-bound branch handles all other multiples of 37 via the loop).

- [ ] **Step 4: Re-run jstring tests, verify all pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/util/jstring/ -v
```

Expected: all tests PASS, including the two `InvalidName*` cases.

- [ ] **Step 5: Run wider tests to confirm no regressions in any consumer**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS. If a player-display test fails because it relied on a `% 37 == 0` username decoding, fix that test (it was wrong per TS).

### T1 Step 6-10: Add Player fields + per-tick reset

- [ ] **Step 6: Write failing test for per-tick reset of socialProtect / reportAbuseProtect**

Create `modules/world/tick_social_reset_test.go`:

```go
package world

import "testing"

// TestProcessCleanupResetsSocialFlags pins NAI-72: processCleanup must
// reset socialProtect and reportAbuseProtect to false on every player
// each tick. Mirrors TS Player.resetEntity(false) at Player.ts:466-467,
// called from World.ts:1138.
func TestProcessCleanupResetsSocialFlags(t *testing.T) {
    s := newTestServer(t)
    p, _ := newTestPlayer(t)
    p.client.server = s
    s.playersMu.Lock()
    s.playerLoop = append(s.playerLoop, p)
    s.playersMu.Unlock()

    p.socialProtect = true
    p.reportAbuseProtect = true

    s.processCleanup()

    if p.socialProtect {
        t.Error("socialProtect: not reset by processCleanup")
    }
    if p.reportAbuseProtect {
        t.Error("reportAbuseProtect: not reset by processCleanup")
    }
}

// TestProcessCleanupPreservesStaffModLevel pins that staffModLevel
// is NOT reset per-tick (it's set once at login per TS World.ts:1895).
func TestProcessCleanupPreservesStaffModLevel(t *testing.T) {
    s := newTestServer(t)
    p, _ := newTestPlayer(t)
    p.client.server = s
    s.playersMu.Lock()
    s.playerLoop = append(s.playerLoop, p)
    s.playersMu.Unlock()

    p.staffModLevel = 2

    s.processCleanup()

    if p.staffModLevel != 2 {
        t.Errorf("staffModLevel: got %d after cleanup, want 2 (not reset)", p.staffModLevel)
    }
}
```

- [ ] **Step 7: Run test, verify compile-fail (fields don't exist yet)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessCleanup -v
```

Expected: compile error `p.socialProtect undefined` and `p.reportAbuseProtect undefined`. (`p.staffModLevel = 2` already compiles — existing field.)

- [ ] **Step 8: Add the 2 Player fields in `modules/world/player.go`**

Insert after line 176 (the end of the `=== chat state ===` block):

```go

	// === social spam protection (NAI-72) ===
	// socialProtect gates FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL, and
	// (future) MESSAGE_PRIVATE — at most one such packet per tick per
	// player. Reset to false in processCleanup. Set to true at handler-
	// success bottom. Mirrors TS Player.socialProtect (Player.ts:386,
	// reset Player.ts:466).
	socialProtect bool

	// reportAbuseProtect gates REPORT_ABUSE — at most one per tick per
	// player. Reset/set semantics identical to socialProtect. Mirrors
	// TS Player.reportAbuseProtect (Player.ts:387, reset Player.ts:467).
	reportAbuseProtect bool
```

(Erratum: `staffModLevel int32` already exists at `player.go:73` with a producer at `server.go:590` from login proto; ReportAbuse handler reads it directly without re-declaring.)

- [ ] **Step 9: Re-run cleanup test, verify it now compiles but the reset assertion fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessCleanup -v
```

Expected: `TestProcessCleanupResetsSocialFlags` FAIL (`socialProtect: not reset by processCleanup`); `TestProcessCleanupPreservesStaffModLevel` PASS (`processCleanup` does not touch `staffModLevel` — the field has unrelated producers/consumers at HEAD but the per-tick reset path is independent).

- [ ] **Step 10: Add the per-tick reset in `modules/world/tick.go:processCleanup`**

Edit the existing player loop at line 475. Currently:

```go
	for _, p := range players {
		p.ResetMasks()
	}
```

Replace with:

```go
	for _, p := range players {
		p.ResetMasks()
		// NAI-72 — TS Player.resetEntity(false) at Player.ts:466-467.
		// Reset social/report spam-protect flags so the next tick admits
		// at most one social/report packet per type per player.
		// (Other resetEntity fields — protect, chatColour/Effect/Rights,
		// chatMessage, logMessage — belong to other sub-specs; tracked
		// as NAI-72-N-RESETENTITY-PARTIAL.)
		p.socialProtect = false
		p.reportAbuseProtect = false
	}
```

- [ ] **Step 11: Re-run, verify both cleanup tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessCleanup -v
```

Expected: both tests PASS.

### T1 Step 12-17: Bridge interfaces + no-op default + recording capture

- [ ] **Step 12: Create `modules/world/bridges.go`**

```go
package world

import "time"

// FriendsBridge mirrors TS World.friendThread.postMessage(...) for
// social-list mutations and chat-mode propagation. Real impl is a
// future friends-server module (see NAI-72-D-FRIENDS-SERVER-BRIDGE).
type FriendsBridge interface {
	AddFriend(playerUsername string, target uint64)
	RemoveFriend(playerUsername string, target uint64)
	AddIgnore(playerUsername string, target uint64)
	RemoveIgnore(playerUsername string, target uint64)
	SetChatMode(playerUsername string, privateChat int)
}

// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). The existing LoginClient is auth-only; this is a
// separate moderation channel. Real impl deferred via
// NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.
type LoginBridgeMod interface {
	NotifyPlayerBan(staff, username string, until time.Time)
	NotifyPlayerMute(staff, username string, until time.Time)
}

// LoggerBridge mirrors TS World.loggerThread.postMessage('report', ...).
// Real impl deferred via NAI-72-D-LOGGER-BRIDGE. The same closure path
// will activate the EventTracking handler.
type LoggerBridge interface {
	// NotifyPlayerReport posts an abuse report. reason is the string label
	// of the ReportAbuseReason enum value (e.g. "MACROING").
	NotifyPlayerReport(player *Player, offender, reason string)
}

// noopBridges is the default impl wired into NewServer. Records nothing,
// performs no I/O.
type noopBridges struct{}

func (noopBridges) AddFriend(string, uint64)                  {}
func (noopBridges) RemoveFriend(string, uint64)               {}
func (noopBridges) AddIgnore(string, uint64)                  {}
func (noopBridges) RemoveIgnore(string, uint64)               {}
func (noopBridges) SetChatMode(string, int)                   {}
func (noopBridges) NotifyPlayerBan(string, string, time.Time) {}
func (noopBridges) NotifyPlayerMute(string, string, time.Time) {}
func (noopBridges) NotifyPlayerReport(*Player, string, string) {}
```

- [ ] **Step 13: Create `modules/world/bridges_test.go` (recordingBridges + interface compliance + no-op smoke)**

```go
package world

import (
	"testing"
	"time"
)

// recordingBridges captures every bridge call into typed slices for
// per-handler assertion. Used by handler_chatsetmode_test.go,
// handler_social_list_test.go, handler_reportabuse_test.go.
type recordedFriendsCall struct {
	method           string // "AddFriend" | "RemoveFriend" | "AddIgnore" | "RemoveIgnore" | "SetChatMode"
	playerUsername   string
	targetUsername37 uint64
	privateChatMode  int // SetChatMode only
}
type recordedLoginModCall struct {
	method   string // "NotifyPlayerBan" | "NotifyPlayerMute"
	staff    string
	username string
	until    time.Time
}
type recordedLoggerCall struct {
	method   string // "NotifyPlayerReport"
	player   *Player
	offender string
	reason   string
}

type recordingBridges struct {
	friends  []recordedFriendsCall
	loginMod []recordedLoginModCall
	logger   []recordedLoggerCall
}

func (r *recordingBridges) AddFriend(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "AddFriend", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) RemoveFriend(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "RemoveFriend", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) AddIgnore(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "AddIgnore", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) RemoveIgnore(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "RemoveIgnore", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) SetChatMode(p string, privateChat int) {
	r.friends = append(r.friends, recordedFriendsCall{method: "SetChatMode", playerUsername: p, privateChatMode: privateChat})
}
func (r *recordingBridges) NotifyPlayerBan(staff, username string, until time.Time) {
	r.loginMod = append(r.loginMod, recordedLoginModCall{method: "NotifyPlayerBan", staff: staff, username: username, until: until})
}
func (r *recordingBridges) NotifyPlayerMute(staff, username string, until time.Time) {
	r.loginMod = append(r.loginMod, recordedLoginModCall{method: "NotifyPlayerMute", staff: staff, username: username, until: until})
}
func (r *recordingBridges) NotifyPlayerReport(player *Player, offender, reason string) {
	r.logger = append(r.logger, recordedLoggerCall{method: "NotifyPlayerReport", player: player, offender: offender, reason: reason})
}

// installRecordingBridges wires a recordingBridges into all 3 Server
// bridge fields and returns the recorder. Used by per-handler tests.
func installRecordingBridges(s *Server) *recordingBridges {
	rec := &recordingBridges{}
	s.friendsBridge = rec
	s.loginBridgeMod = rec
	s.loggerBridge = rec
	return rec
}

// Compile-time: recordingBridges and noopBridges both satisfy all 3
// interfaces. Breaks the build if any signature drifts.
var (
	_ FriendsBridge   = (*recordingBridges)(nil)
	_ LoginBridgeMod  = (*recordingBridges)(nil)
	_ LoggerBridge    = (*recordingBridges)(nil)
	_ FriendsBridge   = noopBridges{}
	_ LoginBridgeMod  = noopBridges{}
	_ LoggerBridge    = noopBridges{}
)

// TestNoopBridgesAllMethods exercises every noopBridges method to keep
// 100% coverage on the no-op impl (catches accidental panics in any
// future signature change).
func TestNoopBridgesAllMethods(t *testing.T) {
	var b noopBridges
	b.AddFriend("u", 1)
	b.RemoveFriend("u", 1)
	b.AddIgnore("u", 1)
	b.RemoveIgnore("u", 1)
	b.SetChatMode("u", 0)
	now := time.Now()
	b.NotifyPlayerBan("s", "u", now)
	b.NotifyPlayerMute("s", "u", now)
	b.NotifyPlayerReport(nil, "off", "REASON")
}

// TestRecordingBridgesCapturesAllCalls exercises every recordingBridges
// method and verifies the slices grow as expected.
func TestRecordingBridgesCapturesAllCalls(t *testing.T) {
	rec := &recordingBridges{}
	rec.AddFriend("alice", 100)
	rec.RemoveFriend("alice", 101)
	rec.AddIgnore("alice", 102)
	rec.RemoveIgnore("alice", 103)
	rec.SetChatMode("alice", 1)
	if len(rec.friends) != 5 {
		t.Errorf("friends: got %d records, want 5", len(rec.friends))
	}
	if rec.friends[0].method != "AddFriend" || rec.friends[0].targetUsername37 != 100 {
		t.Errorf("AddFriend record: %+v", rec.friends[0])
	}
	if rec.friends[4].method != "SetChatMode" || rec.friends[4].privateChatMode != 1 {
		t.Errorf("SetChatMode record: %+v", rec.friends[4])
	}

	now := time.Now()
	rec.NotifyPlayerBan("auto", "evilbob", now)
	rec.NotifyPlayerMute("alice", "evilbob", now)
	if len(rec.loginMod) != 2 {
		t.Errorf("loginMod: got %d, want 2", len(rec.loginMod))
	}

	rec.NotifyPlayerReport(nil, "evilbob", "MACROING")
	if len(rec.logger) != 1 || rec.logger[0].reason != "MACROING" {
		t.Errorf("NotifyPlayerReport record: %+v", rec.logger)
	}
}
```

- [ ] **Step 14: Run bridge tests, expect compile-fail (Server fields don't exist)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNoopBridges|TestRecordingBridges" -v
```

Expected: compile error `s.friendsBridge undefined` from `installRecordingBridges`.

- [ ] **Step 15: Add Server bridge fields and NewServer init**

Edit `modules/world/server.go`. After line 119 (`scriptProvider *script.Provider`), insert:

```go

	// Social/moderation bridges (NAI-72). Default to noopBridges{} in
	// NewServer; tests inject recordingBridges via installRecordingBridges.
	// Real impls deferred per NAI-72-D-{FRIENDS-SERVER-BRIDGE,
	// LOGIN-SERVER-BRIDGE-MOD, LOGGER-BRIDGE}.
	friendsBridge  FriendsBridge
	loginBridgeMod LoginBridgeMod
	loggerBridge   LoggerBridge
```

In `NewServer` at `server.go:128`, after the `s := &Server{...}` literal block (around line 153), add:

```go
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
```

(Place before `s.tcpWg.Add(1)` for clarity.)

Also add the same default-init to `newTestServer(t *testing.T) *Server` in `modules/world/server_test.go:311` so the foundation tests don't need to set them manually:

```go
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		quit:           make(chan interface{}),
		log:            discardLogger(),
		scriptProvider: defaultTestProvider(),
		zoneMap:        zone.NewZoneMap(),
		rsbuf:          rsbuf.New(),
	}
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
	return s
}
```

- [ ] **Step 16: Re-run bridge tests, verify all pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNoopBridges|TestRecordingBridges" -v
```

Expected: both PASS.

- [ ] **Step 17: Run wider tests, confirm no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: PASS. (Existing handler tests should still pass since they rely on `newTestServer` which now also installs noopBridges.)

### T1 Step 18-21: ReportAbuseReason enum + reasonLabel

- [ ] **Step 18: Create `modules/world/social.go`**

```go
package world

// ReportAbuseReason mirrors TS ReportAbuse.ts:4-17. Values 0-11 are
// sent over the wire by REPORT_ABUSE (opcode 190); out-of-range values
// trigger an automated ban (per ReportAbuseHandler.ts:14).
type ReportAbuseReason uint8

const (
	ReportAbuseOffensiveLanguage     ReportAbuseReason = 0
	ReportAbuseItemScamming          ReportAbuseReason = 1
	ReportAbusePasswordScamming      ReportAbuseReason = 2
	ReportAbuseBugAbuse              ReportAbuseReason = 3
	ReportAbuseStaffImpersonation    ReportAbuseReason = 4
	ReportAbuseAccountSharing        ReportAbuseReason = 5
	ReportAbuseMacroing              ReportAbuseReason = 6
	ReportAbuseMultiLogging          ReportAbuseReason = 7
	ReportAbuseEncouragingBreakRules ReportAbuseReason = 8
	ReportAbuseMisuseCustomerSupport ReportAbuseReason = 9
	ReportAbuseAdvertisingWebsite    ReportAbuseReason = 10
	ReportAbuseRealWorldTrading      ReportAbuseReason = 11
)

// reasonLabel returns the canonical string label for a ReportAbuseReason
// value, used as the LoggerBridge.NotifyPlayerReport `reason` argument.
// Out-of-range values return "" (caller is responsible for range-checking
// before calling, per the ReportAbuse handler's gate-then-call order).
func reasonLabel(r ReportAbuseReason) string {
	switch r {
	case ReportAbuseOffensiveLanguage:
		return "OFFENSIVE_LANGUAGE"
	case ReportAbuseItemScamming:
		return "ITEM_SCAMMING"
	case ReportAbusePasswordScamming:
		return "PASSWORD_SCAMMING"
	case ReportAbuseBugAbuse:
		return "BUG_ABUSE"
	case ReportAbuseStaffImpersonation:
		return "STAFF_IMPERSONATION"
	case ReportAbuseAccountSharing:
		return "ACCOUNT_SHARING"
	case ReportAbuseMacroing:
		return "MACROING"
	case ReportAbuseMultiLogging:
		return "MULTI_LOGGING"
	case ReportAbuseEncouragingBreakRules:
		return "ENCOURAGING_BREAK_RULES"
	case ReportAbuseMisuseCustomerSupport:
		return "MISUSE_CUSTOMER_SUPPORT"
	case ReportAbuseAdvertisingWebsite:
		return "ADVERTISING_WEBSITE"
	case ReportAbuseRealWorldTrading:
		return "REAL_WORLD_TRADING"
	}
	return ""
}
```

- [ ] **Step 19: Create `modules/world/social_test.go`**

```go
package world

import "testing"

// TestReasonLabelAllValid pins every in-range ReportAbuseReason value
// to its canonical string label. Mirrors TS ReportAbuse.ts:4-17.
func TestReasonLabelAllValid(t *testing.T) {
	cases := []struct {
		reason ReportAbuseReason
		want   string
	}{
		{ReportAbuseOffensiveLanguage, "OFFENSIVE_LANGUAGE"},
		{ReportAbuseItemScamming, "ITEM_SCAMMING"},
		{ReportAbusePasswordScamming, "PASSWORD_SCAMMING"},
		{ReportAbuseBugAbuse, "BUG_ABUSE"},
		{ReportAbuseStaffImpersonation, "STAFF_IMPERSONATION"},
		{ReportAbuseAccountSharing, "ACCOUNT_SHARING"},
		{ReportAbuseMacroing, "MACROING"},
		{ReportAbuseMultiLogging, "MULTI_LOGGING"},
		{ReportAbuseEncouragingBreakRules, "ENCOURAGING_BREAK_RULES"},
		{ReportAbuseMisuseCustomerSupport, "MISUSE_CUSTOMER_SUPPORT"},
		{ReportAbuseAdvertisingWebsite, "ADVERTISING_WEBSITE"},
		{ReportAbuseRealWorldTrading, "REAL_WORLD_TRADING"},
	}
	for _, tc := range cases {
		if got := reasonLabel(tc.reason); got != tc.want {
			t.Errorf("reasonLabel(%d): got %q, want %q", tc.reason, got, tc.want)
		}
	}
}

// TestReasonLabelOutOfRangeReturnsEmpty pins out-of-range behavior.
func TestReasonLabelOutOfRangeReturnsEmpty(t *testing.T) {
	for _, r := range []ReportAbuseReason{12, 13, 100, 255} {
		if got := reasonLabel(r); got != "" {
			t.Errorf("reasonLabel(%d): got %q, want \"\"", r, got)
		}
	}
}

// TestReportAbuseReasonRangeBoundary pins the constants used by the
// handler's range gate at handler_reportabuse.go.
func TestReportAbuseReasonRangeBoundary(t *testing.T) {
	if ReportAbuseOffensiveLanguage != 0 {
		t.Errorf("ReportAbuseOffensiveLanguage: got %d, want 0", ReportAbuseOffensiveLanguage)
	}
	if ReportAbuseRealWorldTrading != 11 {
		t.Errorf("ReportAbuseRealWorldTrading: got %d, want 11", ReportAbuseRealWorldTrading)
	}
}
```

- [ ] **Step 20: Run social tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestReasonLabel|TestReportAbuseReasonRange" -v
```

Expected: all PASS.

- [ ] **Step 21: Full regression sweep + race**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1
```

Expected: PASS.

### T1 Step 22: Commit foundation

- [ ] **Step 22: Commit T1**

```bash
git add pkg/util/jstring/jstring.go pkg/util/jstring/jstring_test.go \
  modules/world/player.go modules/world/tick.go \
  modules/world/server.go modules/world/server_test.go \
  modules/world/bridges.go modules/world/bridges_test.go \
  modules/world/social.go modules/world/social_test.go \
  modules/world/tick_social_reset_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-72 T1 — social subsystem foundation
             (Player flags, processCleanup hook, bridges, enum)

- pkg/util/jstring.FromBase37: add TS % 37 == 0 invalid-name check
  (true-to-TS bug fix per JString.ts:42-44, surfaced by NAI-72 social
  handler invalid_name gate)
- Player: add socialProtect, reportAbuseProtect fields
  (mirror Player.ts:386-387). staffModLevel field already exists at
  player.go:73 with login-proto producer — see erratum block.
- processCleanup: per-tick reset of social/report protect flags
  (mirror Player.resetEntity(false) at Player.ts:466-467)
- Server: add friendsBridge, loginBridgeMod, loggerBridge fields with
  noopBridges{} default in NewServer + newTestServer
- bridges.go: FriendsBridge, LoginBridgeMod, LoggerBridge interfaces
  + noopBridges no-op impl
- bridges_test.go: recordingBridges capture impl + interface compliance
  pins + TestNoopBridgesAllMethods + TestRecordingBridgesCapturesAllCalls
- social.go: ReportAbuseReason enum (12 values, 0..11) + reasonLabel
- tick_social_reset_test.go: per-tick reset coverage + staffModLevel
  preservation pin (pins that processCleanup does NOT reset
  staffModLevel, defending against accidental future loop-body changes)

Plan: docs/superpowers/plans/2026-05-02-nai-72-social-subsystem-foundation.md
Spec: docs/superpowers/specs/2026-05-02-nai-72-social-subsystem-foundation-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Stage 1 review (controller, after T1)

Per `runescript_cadence.md` and `superpowers_code_reviewer_model.md` (Sonnet only):

Dispatch `superpowers:code-reviewer` agent on Sonnet with these focus areas:

1. **Bridge interface shape** — are the three interfaces minimal? Does `recordingBridges` capture every field needed by the per-handler tests in T2-T4?
2. **`recordingBridges` test idiom** — is `installRecordingBridges` clear, single-use, no hidden coupling?
3. **`processCleanup` hook** — minimal change, no scope creep into other reset fields, doc-comment cites NAI-72-N-RESETENTITY-PARTIAL?
4. **`FromBase37` fix** — does the new branch match TS exactly? Is the `v != 0` guard correct (preserves zero behavior)?
5. **Deviation tag completeness** — is every TS behavior gap that ships in T1 covered by either a deviation tag or a tracker note? (Specifically: NAI-72-N-RESETENTITY-PARTIAL for the partial reset. Note: `NAI-72-D-STAFFMODLEVEL-DEAD-WRITER` was retracted via erratum; the field already has a producer.)
6. **(Retracted via erratum.)** Original focus area asked about `consume_reserved_constant` pattern for staffModLevel; pre-flight grep showed the field already has a producer at `server.go:590`, so the pattern does not apply. Reviewer should instead spot-check that `T1` did NOT add a duplicate `staffModLevel` field and that ReportAbuse-driving tests use `p.staffModLevel = 1` direct assignment to keep the moderator-mute branch deterministic.

If reviewer surfaces any high-confidence issue, controller reads source + verifies before re-dispatch (per `audit_subagent_fabrication.md`). Fix-ups commit independently.

---

## Task 2 — CHAT_SETMODE handler (opcode 244)

**Files:**
- Create: `modules/world/handler_chatsetmode.go`
- Create: `modules/world/handler_chatsetmode_test.go`
- Modify: `modules/world/handlers_game.go` (add 1 init() line)

### T2 Step 1: Write failing test for CHAT_SETMODE handler

- [ ] **Step 1: Create `modules/world/handler_chatsetmode_test.go`**

```go
package world

import "testing"

// TestHandleChatSetModeAssignsAllThreeFields pins ChatSetModeHandler.ts:7-13:
// the 3 wire bytes are written into Player.publicChat / .privateChat /
// .tradeDuel.
func TestHandleChatSetModeAssignsAllThreeFields(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	installRecordingBridges(s)

	// publicChat=2, privateChat=1, tradeDuel=0
	payload := []byte{2, 1, 0}
	if err := handleChatSetMode(p, payload); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if p.publicChat != 2 {
		t.Errorf("publicChat: got %d, want 2", p.publicChat)
	}
	if p.privateChat != 1 {
		t.Errorf("privateChat: got %d, want 1", p.privateChat)
	}
	if p.tradeDuel != 0 {
		t.Errorf("tradeDuel: got %d, want 0", p.tradeDuel)
	}
}

// TestHandleChatSetModeFiresFriendsBridge pins ChatSetModeHandler.ts:11
// — sendPrivateChatModeToFriendsServer is called with player.username and
// the new privateChat value.
func TestHandleChatSetModeFiresFriendsBridge(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)

	if err := handleChatSetMode(p, []byte{0, 1, 2}); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Fatalf("friends bridge: got %d calls, want 1", len(rec.friends))
	}
	got := rec.friends[0]
	if got.method != "SetChatMode" {
		t.Errorf("method: got %q, want SetChatMode", got.method)
	}
	if got.playerUsername != "alice" {
		t.Errorf("playerUsername: got %q, want alice", got.playerUsername)
	}
	if got.privateChatMode != 1 {
		t.Errorf("privateChatMode: got %d, want 1", got.privateChatMode)
	}
}

// TestHandleChatSetModeIgnoresSocialProtect pins that ChatSetMode is NOT
// gated by socialProtect (TS ChatSetModeHandler.ts has no such gate,
// unlike Friend/Ignore/MessagePrivate).
func TestHandleChatSetModeIgnoresSocialProtect(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	p.socialProtect = true

	if err := handleChatSetMode(p, []byte{1, 0, 0}); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if p.publicChat != 1 {
		t.Errorf("publicChat: got %d, want 1 (no socialProtect gate)", p.publicChat)
	}
	if len(rec.friends) != 1 {
		t.Errorf("bridge: got %d calls, want 1 (no socialProtect gate)", len(rec.friends))
	}
}

// TestHandleChatSetModeDoesNotSetSocialProtect pins that ChatSetMode does
// NOT set socialProtect = true (TS handler does not).
func TestHandleChatSetModeDoesNotSetSocialProtect(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	installRecordingBridges(s)

	if err := handleChatSetMode(p, []byte{0, 0, 0}); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if p.socialProtect {
		t.Error("socialProtect: must NOT be set by ChatSetMode")
	}
}

// TestHandleChatSetModeNilServerNoOp pins the goscape defensive guard:
// a Player with no server reference returns nil without panic.
func TestHandleChatSetModeNilServerNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = nil

	if err := handleChatSetMode(p, []byte{1, 1, 1}); err != nil {
		t.Errorf("handleChatSetMode with nil server: got err %v, want nil", err)
	}
	if p.publicChat != 0 {
		t.Errorf("publicChat: got %d, want 0 (no-op on nil server)", p.publicChat)
	}
}
```

- [ ] **Step 2: Run test, verify compile-fail (`handleChatSetMode` undefined)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleChatSetMode -v
```

Expected: compile error `undefined: handleChatSetMode`.

- [ ] **Step 3: Create `modules/world/handler_chatsetmode.go`**

```go
package world

import "github.com/zsrv/goscape/pkg/io/packet"

// handleChatSetMode handles client opcode 244 (CHAT_SETMODE), payload
// 3 bytes: g1 publicChat, g1 privateChat, g1 tradeDuel.
//
// Mirrors TS ChatSetModeHandler.ts:7-13. No socialProtect gate (TS
// does not gate this opcode). Activates Player.publicChat / .privateChat
// / .tradeDuel which are declared at player.go:172 but were unwritten
// prior to NAI-72.
//
// Friends-server propagation deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE.
func handleChatSetMode(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	pk := packet.NewPacket(payload)
	p.publicChat = int(pk.G1())
	p.privateChat = int(pk.G1())
	p.tradeDuel = int(pk.G1())
	p.client.server.friendsBridge.SetChatMode(p.username, p.privateChat)
	return nil
}
```

- [ ] **Step 4: Run handler tests, verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleChatSetMode -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Wire opcode 244 into `gameHandlers[]`**

Edit `modules/world/handlers_game.go`. After line 67 (the end of the OPHELD block), insert before the `MESSAGE_PUBLIC` line:

```go
	gameHandlers[244] = handleChatSetMode // CHAT_SETMODE
```

- [ ] **Step 6: Full regression sweep + race**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit T2**

```bash
git add modules/world/handler_chatsetmode.go modules/world/handler_chatsetmode_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-72 T2 — CHAT_SETMODE handler port (opcode 244)

- handler_chatsetmode.go: 3-byte payload → Player.publicChat /
  .privateChat / .tradeDuel + friendsBridge.SetChatMode propagation
- gameHandlers[244] wired in handlers_game.go init()
- 5 tests: 3-field assignment, friends-bridge call shape,
  socialProtect non-gate, no protect-set, nil-server defensive

Mirrors TS ChatSetModeHandler.ts:7-13. No deviation opened (matches TS
exactly modulo the bridge stub already tagged in T1).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Friend/Ignore family (opcodes 11, 79, 118, 171)

**Files:**
- Create: `modules/world/handler_social_list.go`
- Create: `modules/world/handler_social_list_test.go`
- Modify: `modules/world/handlers_game.go` (add 4 init() lines)

### T3 Step 1: Write failing tests

- [ ] **Step 1: Create `modules/world/handler_social_list_test.go`**

```go
package world

import (
	"testing"

	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// commonSocialListSetup wires a player against a server with recording
// bridges and a known username. Returns p, recorder.
func commonSocialListSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	return p, rec
}

// payloadG8 returns an 8-byte big-endian payload encoding the username37
// value, matching what packet.G8 reads.
func payloadG8(v uint64) []byte {
	return []byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	}
}

func TestHandleFriendListAddCallsBridgeAndSetsProtect(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListAdd: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Fatalf("friends: got %d calls, want 1", len(rec.friends))
	}
	got := rec.friends[0]
	if got.method != "AddFriend" || got.playerUsername != "alice" || got.targetUsername37 != target {
		t.Errorf("AddFriend record: %+v (want method=AddFriend, user=alice, target=%d)", got, target)
	}
	if !p.socialProtect {
		t.Error("socialProtect: must be true after successful Friend/Ignore call")
	}
}

func TestHandleFriendListDelCallsRemoveFriend(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")
	if err := handleFriendListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListDel: %v", err)
	}
	if len(rec.friends) != 1 || rec.friends[0].method != "RemoveFriend" {
		t.Errorf("expected one RemoveFriend call, got %+v", rec.friends)
	}
	if !p.socialProtect {
		t.Error("socialProtect not set")
	}
}

func TestHandleIgnoreListAddCallsAddIgnore(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")
	if err := handleIgnoreListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleIgnoreListAdd: %v", err)
	}
	if len(rec.friends) != 1 || rec.friends[0].method != "AddIgnore" {
		t.Errorf("expected one AddIgnore call, got %+v", rec.friends)
	}
	if !p.socialProtect {
		t.Error("socialProtect not set")
	}
}

func TestHandleIgnoreListDelCallsRemoveIgnore(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")
	if err := handleIgnoreListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("handleIgnoreListDel: %v", err)
	}
	if len(rec.friends) != 1 || rec.friends[0].method != "RemoveIgnore" {
		t.Errorf("expected one RemoveIgnore call, got %+v", rec.friends)
	}
	if !p.socialProtect {
		t.Error("socialProtect not set")
	}
}

// TestHandleSocialListSocialProtectGate pins {Friend,Ignore}List handlers'
// early-return when socialProtect is already set: no bridge call, no
// re-set (no-op).
func TestHandleSocialListSocialProtectGate(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	p.socialProtect = true
	target := util.ToBase37("bob")

	for _, fn := range []func(*Player, []byte) error{
		handleFriendListAdd, handleFriendListDel,
		handleIgnoreListAdd, handleIgnoreListDel,
	} {
		if err := fn(p, payloadG8(target)); err != nil {
			t.Fatalf("handler error: %v", err)
		}
	}
	if len(rec.friends) != 0 {
		t.Errorf("bridge: got %d calls, want 0 (gated by socialProtect)", len(rec.friends))
	}
}

// TestHandleSocialListInvalidNameGate pins the FromBase37 == "invalid_name"
// gate. Use 37 — multiple of 37 → invalid_name per the T1 jstring fix.
func TestHandleSocialListInvalidNameGate(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := uint64(37) // % 37 == 0 → "invalid_name"

	if got := util.FromBase37(target); got != "invalid_name" {
		t.Fatalf("test premise broken: FromBase37(%d) = %q, want invalid_name", target, got)
	}

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListAdd: %v", err)
	}
	if len(rec.friends) != 0 {
		t.Errorf("bridge: got %d calls, want 0 (gated by invalid_name)", len(rec.friends))
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on invalid_name early-return")
	}
}

// TestHandleSocialListInvalidNameUpperBoundGate pins the upper-bound
// "invalid_name" branch (>= 37**12).
func TestHandleSocialListInvalidNameUpperBoundGate(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := uint64(6582952005840035281) // == 37**12

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListAdd: %v", err)
	}
	if len(rec.friends) != 0 {
		t.Errorf("bridge: got %d calls, want 0 (gated by upper-bound invalid_name)", len(rec.friends))
	}
}

// TestHandleSocialListNilServerNoOp pins the defensive guard.
func TestHandleSocialListNilServerNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = nil
	target := util.ToBase37("bob")

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Errorf("handleFriendListAdd nil-server: got err %v", err)
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on nil-server early-return")
	}
}
```

- [ ] **Step 2: Run tests, verify compile-fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleFriendList -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleIgnoreList -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleSocialList -v
```

Expected: compile errors (`handleFriendListAdd` etc. undefined).

- [ ] **Step 3: Create `modules/world/handler_social_list.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// socialListAction enumerates the four bridge methods invoked by the
// Friend/Ignore handler family.
type socialListAction int

const (
	socialAddFriend    socialListAction = iota // op 118
	socialRemoveFriend                         // op 11
	socialAddIgnore                            // op 79
	socialRemoveIgnore                         // op 171
)

// handleSocialList is the shared body of FRIENDLIST_ADD/DEL and
// IGNORELIST_ADD/DEL. All four:
//  1. Decode g8 username (uint64 base37).
//  2. Early-return if socialProtect is set OR the username decodes to
//     the "invalid_name" sentinel.
//  3. Dispatch to the appropriate FriendsBridge method.
//  4. Set socialProtect = true.
//
// Mirrors TS {Friend,Ignore}List{Add,Del}Handler.ts:8-15 (all four share
// an identical body shape modulo the World call).
//
// Friends-server propagation deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE.
func handleSocialList(p *Player, payload []byte, action socialListAction) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	pk := packet.NewPacket(payload)
	username := pk.G8()

	if p.socialProtect || util.FromBase37(username) == "invalid_name" {
		return nil
	}

	fb := p.client.server.friendsBridge
	switch action {
	case socialAddFriend:
		fb.AddFriend(p.username, username)
	case socialRemoveFriend:
		fb.RemoveFriend(p.username, username)
	case socialAddIgnore:
		fb.AddIgnore(p.username, username)
	case socialRemoveIgnore:
		fb.RemoveIgnore(p.username, username)
	}
	p.socialProtect = true
	return nil
}

func handleFriendListAdd(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialAddFriend)
}
func handleFriendListDel(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialRemoveFriend)
}
func handleIgnoreListAdd(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialAddIgnore)
}
func handleIgnoreListDel(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialRemoveIgnore)
}
```

- [ ] **Step 4: Run tests, verify all pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleFriendList|TestHandleIgnoreList|TestHandleSocialList" -v
```

Expected: all 8 tests PASS.

- [ ] **Step 5: Wire 4 opcodes into `gameHandlers[]`**

Edit `modules/world/handlers_game.go`. After the `gameHandlers[244]` line added in T2, insert:

```go
	gameHandlers[118] = handleFriendListAdd // FRIENDLIST_ADD
	gameHandlers[11] = handleFriendListDel  // FRIENDLIST_DEL
	gameHandlers[79] = handleIgnoreListAdd  // IGNORELIST_ADD
	gameHandlers[171] = handleIgnoreListDel // IGNORELIST_DEL
```

- [ ] **Step 6: Full regression sweep + race**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit T3**

```bash
git add modules/world/handler_social_list.go modules/world/handler_social_list_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-72 T3 — Friend/Ignore family handler port
             (opcodes 118, 11, 79, 171)

- handler_social_list.go: shared handleSocialList core + 4 wrappers
  (handleFriendListAdd/Del, handleIgnoreListAdd/Del). All four:
  decode g8 username, early-return on socialProtect | invalid_name
  gate, dispatch friendsBridge method, set socialProtect = true.
- gameHandlers[118|11|79|171] wired in handlers_game.go init()
- 8 tests: per-handler bridge-call shape + socialProtect-set;
  socialProtect early-return gate (all 4); invalid_name gate via
  % 37 == 0 (mod-37) AND upper-bound (37**12); nil-server defensive

Mirrors TS {Friend,Ignore}List{Add,Del}Handler.ts:8-15 line-by-line.
No deviation opened (matches TS exactly modulo the friends-server
bridge stub already tagged in T1).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — REPORT_ABUSE handler (opcode 190)

**Files:**
- Create: `modules/world/handler_reportabuse.go`
- Create: `modules/world/handler_reportabuse_test.go`
- Modify: `modules/world/handlers_game.go` (add 1 init() line)

### T4 Step 1: Write failing tests

- [ ] **Step 1: Create `modules/world/handler_reportabuse_test.go`**

```go
package world

import (
	"io"
	"testing"
	"time"

	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// reportAbusePayload returns a 10-byte REPORT_ABUSE payload:
// g8 offender, g1 reason, g1 moderatorMute(bool).
func reportAbusePayload(offender uint64, reason ReportAbuseReason, moderatorMute bool) []byte {
	mute := byte(0)
	if moderatorMute {
		mute = 1
	}
	return []byte{
		byte(offender >> 56), byte(offender >> 48), byte(offender >> 40), byte(offender >> 32),
		byte(offender >> 24), byte(offender >> 16), byte(offender >> 8), byte(offender),
		byte(reason), mute,
	}
}

// reportAbuseSetup returns p, conn, recorder. Caller must drain conn
// for handlers that call MessageGame.
func reportAbuseSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, conn := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	t.Cleanup(func() {
		// Drain any buffered MessageGame writes (writeOut is buffered;
		// if the test reads via clientConn it should explicitly check
		// the bytes — most tests just need draining to avoid blocking).
		_, _ = io.Copy(io.Discard, &nopReader{conn: conn, deadline: time.Millisecond})
	})
	return p, rec
}

// nopReader is a bounded reader that returns EOF after a short deadline.
// Avoids hanging in t.Cleanup when handlers don't write any bytes.
type nopReader struct {
	conn     interface{ SetReadDeadline(time.Time) error }
	deadline time.Duration
}

func (r *nopReader) Read([]byte) (int, error) {
	r.conn.SetReadDeadline(time.Now().Add(r.deadline))
	return 0, io.EOF
}

func TestHandleReportAbuseInRangeReasonFiresLoggerAndMessageAndProtect(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	offender := util.ToBase37("bob")

	payload := reportAbusePayload(offender, ReportAbuseMacroing, false)
	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}

	if len(rec.logger) != 1 {
		t.Fatalf("logger: got %d calls, want 1", len(rec.logger))
	}
	got := rec.logger[0]
	if got.method != "NotifyPlayerReport" {
		t.Errorf("method: got %q, want NotifyPlayerReport", got.method)
	}
	if got.player != p {
		t.Errorf("player: got %p, want %p", got.player, p)
	}
	if got.offender != "bob" {
		t.Errorf("offender: got %q, want bob", got.offender)
	}
	if got.reason != "MACROING" {
		t.Errorf("reason: got %q, want MACROING", got.reason)
	}
	if !p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must be true after successful in-range report")
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0 (no ban/mute on in-range)", len(rec.loginMod))
	}
}

func TestHandleReportAbuseOutOfRangeReasonFiresBan(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	offender := util.ToBase37("bob")

	// reason 12 (just past RealWorldTrading=11) → automated 48h ban
	payload := reportAbusePayload(offender, ReportAbuseReason(12), false)
	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d calls, want 1", len(rec.loginMod))
	}
	got := rec.loginMod[0]
	if got.method != "NotifyPlayerBan" {
		t.Errorf("method: got %q, want NotifyPlayerBan", got.method)
	}
	if got.staff != "automated" {
		t.Errorf("staff: got %q, want automated", got.staff)
	}
	if got.username != "alice" {
		t.Errorf("username: got %q, want alice", got.username)
	}
	// 48h ban; allow some slack on the wall-clock comparison
	want := time.Now().Add(48 * time.Hour)
	if got.until.Before(want.Add(-time.Minute)) || got.until.After(want.Add(time.Minute)) {
		t.Errorf("until: got %v, want ~%v (±1min)", got.until, want)
	}

	if len(rec.logger) != 0 {
		t.Errorf("logger: got %d calls, want 0 (out-of-range short-circuits)", len(rec.logger))
	}
	if p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must NOT be set on out-of-range branch (TS does not set)")
	}
}

func TestHandleReportAbuseProtectGate(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.reportAbuseProtect = true
	offender := util.ToBase37("bob")

	if err := handleReportAbuse(p, reportAbusePayload(offender, ReportAbuseOffensiveLanguage, false)); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.logger) != 0 || len(rec.loginMod) != 0 {
		t.Errorf("bridges: got logger=%d loginMod=%d, want 0/0 (gated)", len(rec.logger), len(rec.loginMod))
	}
}

// Moderator-mute fires only when ALL three conditions are true:
// moderatorMute && p.staffModLevel > 0 && cfg.NodeProduction.
func TestHandleReportAbuseModeratorMuteAllConditionsTrue(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 1
	p.client.server.cfg.NodeProduction = true
	offender := util.ToBase37("bob")

	if err := handleReportAbuse(p, reportAbusePayload(offender, ReportAbuseMacroing, true)); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.loginMod) != 1 || rec.loginMod[0].method != "NotifyPlayerMute" {
		t.Fatalf("loginMod: got %+v, want one NotifyPlayerMute call", rec.loginMod)
	}
	got := rec.loginMod[0]
	if got.staff != "alice" || got.username != "bob" {
		t.Errorf("Mute record: got staff=%q username=%q, want alice/bob", got.staff, got.username)
	}
	// Logger still fires — moderator-mute is additive to NotifyPlayerReport.
	if len(rec.logger) != 1 {
		t.Errorf("logger: got %d, want 1 (mute is additive, not exclusive)", len(rec.logger))
	}
	if !p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must be set on in-range mute branch")
	}
}

func TestHandleReportAbuseModeratorMuteFlagFalseDoesNotFire(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 1
	p.client.server.cfg.NodeProduction = true
	offender := util.ToBase37("bob")

	// moderatorMute=false → mute branch must not fire
	if err := handleReportAbuse(p, reportAbusePayload(offender, ReportAbuseMacroing, false)); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	for _, c := range rec.loginMod {
		if c.method == "NotifyPlayerMute" {
			t.Errorf("unexpected mute call: %+v", c)
		}
	}
}

func TestHandleReportAbuseModeratorMuteStaffZeroDoesNotFire(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 0 // not staff
	p.client.server.cfg.NodeProduction = true
	offender := util.ToBase37("bob")

	if err := handleReportAbuse(p, reportAbusePayload(offender, ReportAbuseMacroing, true)); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	for _, c := range rec.loginMod {
		if c.method == "NotifyPlayerMute" {
			t.Errorf("unexpected mute call (staffModLevel=0): %+v", c)
		}
	}
}

func TestHandleReportAbuseModeratorMuteNonProductionDoesNotFire(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 1
	p.client.server.cfg.NodeProduction = false // dev mode
	offender := util.ToBase37("bob")

	if err := handleReportAbuse(p, reportAbusePayload(offender, ReportAbuseMacroing, true)); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	for _, c := range rec.loginMod {
		if c.method == "NotifyPlayerMute" {
			t.Errorf("unexpected mute call (NodeProduction=false): %+v", c)
		}
	}
}

// TestHandleReportAbuseMessageGameAck pins the exact ack text sent to
// the client on successful in-range report.
func TestHandleReportAbuseMessageGameAck(t *testing.T) {
	s := newTestServer(t)
	p, conn := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	installRecordingBridges(s)

	offender := util.ToBase37("bob")
	if err := handleReportAbuse(p, reportAbusePayload(offender, ReportAbuseOffensiveLanguage, false)); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}

	// Drain the wire: writeOut sends OpMessageGame + len + JagString payload.
	// We just confirm the Thank-you string appears in the bytes the client
	// would receive.
	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	if n == 0 {
		t.Fatal("no bytes written to client (MessageGame did not fire)")
	}
	want := []byte("Thank-you, your abuse report has been received")
	if !contains(buf[:n], want) {
		t.Errorf("MessageGame payload missing expected text\ngot bytes: %q\nwant substring: %q", buf[:n], want)
	}
}

func contains(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestHandleReportAbuseNilServerNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = nil
	if err := handleReportAbuse(p, reportAbusePayload(0, 0, false)); err != nil {
		t.Errorf("nil-server: got err %v", err)
	}
	if p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must remain false on nil-server early-return")
	}
}

// TestHandleReportAbuseRangeBoundaryEdges pins the EXACT range boundaries:
// reason 0 (OFFENSIVE_LANGUAGE) and 11 (REAL_WORLD_TRADING) must fire the
// in-range path, not the ban path.
func TestHandleReportAbuseRangeBoundaryEdges(t *testing.T) {
	for _, reason := range []ReportAbuseReason{ReportAbuseOffensiveLanguage, ReportAbuseRealWorldTrading} {
		p, rec := reportAbuseSetup(t)
		offender := util.ToBase37("bob")
		if err := handleReportAbuse(p, reportAbusePayload(offender, reason, false)); err != nil {
			t.Fatalf("reason=%d: %v", reason, err)
		}
		if len(rec.loginMod) != 0 {
			t.Errorf("reason=%d: unexpected ban (boundary should be in-range)", reason)
		}
		if len(rec.logger) != 1 {
			t.Errorf("reason=%d: logger calls = %d, want 1", reason, len(rec.logger))
		}
	}
}
```

- [ ] **Step 2: Run tests, verify compile-fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleReportAbuse -v
```

Expected: compile error `handleReportAbuse undefined`.

- [ ] **Step 3: Create `modules/world/handler_reportabuse.go`**

```go
package world

import (
	"time"

	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// handleReportAbuse handles client opcode 190 (REPORT_ABUSE), payload
// 10 bytes: g8 offender, g1 reason, g1 moderatorMute(bool).
//
// Mirrors TS ReportAbuseHandler.ts:9-26. Branch order:
//  1. reportAbuseProtect early-return (no protect-set on this branch).
//  2. Out-of-range reason → automated 48h ban + early-return
//     (no protect-set; matches TS line 14).
//  3. Optional moderator-mute branch (gated 3-way: moderatorMute &&
//     p.staffModLevel > 0 && cfg.NodeProduction).
//  4. Logger bridge call.
//  5. MessageGame ack.
//  6. reportAbuseProtect = true.
//
// The MACROING/BUG_ABUSE submitInput=true branch (TS World.ts:2298-2304)
// is intentionally omitted — input-recording subsystem not ported
// (NAI-72-D-INPUT-RECORDING-NOT-PORTED).
//
// Friends/login/logger bridges all stubbed; see NAI-72-D-FRIENDS-SERVER-
// BRIDGE / NAI-72-D-LOGIN-SERVER-BRIDGE-MOD / NAI-72-D-LOGGER-BRIDGE.
func handleReportAbuse(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	if p.reportAbuseProtect {
		return nil
	}
	pk := packet.NewPacket(payload)
	offender := pk.G8()
	reason := ReportAbuseReason(pk.G1())
	moderatorMute := pk.G1() != 0

	s := p.client.server

	// Range gate: TS sets a 48h automated ban for out-of-range values
	// (anti-tamper guard against modified clients). No protect-set on
	// this branch — TS ReportAbuseHandler.ts:14-17.
	if reason < ReportAbuseOffensiveLanguage || reason > ReportAbuseRealWorldTrading {
		s.loginBridgeMod.NotifyPlayerBan("automated", p.username, time.Now().Add(48*time.Hour))
		return nil
	}

	// Moderator-mute branch: only fires if the reporting player has staff
	// level > 0 AND the bool flag is set AND we're in production. Mutes
	// the offender for 48h. (TS ReportAbuseHandler.ts:19-22.) Reads the
	// existing Player.staffModLevel int32 (set from login proto at
	// server.go:590).
	if moderatorMute && p.staffModLevel > 0 && s.cfg.NodeProduction {
		s.loginBridgeMod.NotifyPlayerMute(p.username, util.FromBase37(offender), time.Now().Add(48*time.Hour))
	}

	s.loggerBridge.NotifyPlayerReport(p, util.FromBase37(offender), reasonLabel(reason))
	p.MessageGame("Thank-you, your abuse report has been received")
	p.reportAbuseProtect = true
	return nil
}
```

- [ ] **Step 4: Run tests, verify all pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleReportAbuse -v
```

Expected: all tests PASS. If `TestHandleReportAbuseMessageGameAck` is flaky on the `conn.Read` due to the buffered nature of `writeOut`, increase the read deadline to 200ms.

- [ ] **Step 5: Wire opcode 190 into `gameHandlers[]`**

Edit `modules/world/handlers_game.go`. After the `gameHandlers[171]` line added in T3, insert:

```go
	gameHandlers[190] = handleReportAbuse // REPORT_ABUSE
```

- [ ] **Step 6: Full regression sweep + race**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit T4**

```bash
git add modules/world/handler_reportabuse.go modules/world/handler_reportabuse_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-72 T4 — REPORT_ABUSE handler port (opcode 190)

- handler_reportabuse.go: 10-byte payload (g8 offender, g1 reason,
  g1 moderatorMute) with 6-branch logic:
   1. reportAbuseProtect early-return
   2. Out-of-range reason → automated 48h ban (no protect-set)
   3. Optional moderator-mute (3-way gate: flag && staffLvl>0 &&
      NodeProduction)
   4. Logger bridge NotifyPlayerReport
   5. MessageGame "Thank-you, your abuse report has been received"
   6. reportAbuseProtect = true
- gameHandlers[190] wired in handlers_game.go init()
- 10 tests: in-range fires logger+ack+protect; out-of-range fires
  ban without protect-set; reportAbuseProtect gate; moderator-mute
  3-way conditions (all-true, flag-false, staff-zero, dev-mode);
  MessageGame ack text pin; nil-server defensive; range-boundary
  edges (0, 11)

Mirrors TS ReportAbuseHandler.ts:9-26 line-by-line. The MACROING/
BUG_ABUSE submitInput=true branch (TS World.ts:2298-2304) is
intentionally omitted — input-recording subsystem not ported
(NAI-72-D-INPUT-RECORDING-NOT-PORTED).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Stage 2 review (controller, after T4, before close)

Per `runescript_cadence.md` and `superpowers_code_reviewer_model.md` (Sonnet only):

Dispatch `superpowers:code-reviewer` agent on Sonnet with these focus areas:

1. **TS-fidelity gate, every handler line-by-line.** Read TS `ChatSetModeHandler.ts`, `FriendListAddHandler.ts`, `FriendListDelHandler.ts`, `IgnoreListAddHandler.ts`, `IgnoreListDelHandler.ts`, `ReportAbuseHandler.ts` and verify each goscape handler matches branch-for-branch.
2. **`spec_unconditional_swap_in_arm_block` re-confirmation.** Spec R7 claims the lesson is not applicable. Reviewer reconfirms by checking that no "set protect = true" assignment is nested inside a conditional `if (!script)` arm block — they should all be at the BOTTOM of the function, OUTSIDE the gate-check.
3. **`submitInput=true` omission is documented.** ReportAbuse handler doc-comment must explicitly call out that the MACROING/BUG_ABUSE submitInput branch is intentionally omitted with the deviation tag.
4. **ReportAbuse range-check direction.** Verify the comparison is `reason < ReportAbuseOffensiveLanguage || reason > ReportAbuseRealWorldTrading` — NOT `>=` or `<=`, NOT inverted. TS uses `<` and `>` per ReportAbuseHandler.ts:14.
5. **Moderator-mute 3-way gate order.** Verify all three conditions are checked AND-style (short-circuit): `moderatorMute && p.staffModLevel > 0 && s.cfg.NodeProduction`. TS uses the same order at ReportAbuseHandler.ts:19.
6. **Bridge call shape per handler.** Verify each bridge method receives the exact arguments TS posts in the `postMessage(...)` calls (especially `notifyPlayerBan` staff="automated" string literal, and the `48*time.Hour` durations).
7. **Defensive-gate doc-comment labels.** Per `defensive_gate_doc_comment_label.md`, every `nil` check on `p.client`/`p.client.server` is labeled "(goscape defensive; TS reaches via static accessor)".

If reviewer surfaces any high-confidence issue, controller verifies before re-dispatch (per `audit_subagent_fabrication.md`). Fix-up commits land independently before close.

---

## Close — integration smoke + deviation tally + memory + close commit

**Files:**
- Create: `modules/world/handlers_game_social_smoke_test.go`
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append NAI-72 entry)
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (no new lines unless a new lesson surfaces)

### Close Step 1: Integration smoke

- [ ] **Step 1: Create `modules/world/handlers_game_social_smoke_test.go`**

```go
package world

import (
	"testing"

	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// TestNAI72AllSixOpcodesDispatch drives all 6 NAI-72 opcodes through one
// recordingBridges fixture and asserts the expected bridge effect for
// each, plus the per-tick reset between bursts. Mirrors a smoke pass of
// the entire bundle.
func TestNAI72AllSixOpcodesDispatch(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	s.playersMu.Lock()
	s.playerLoop = append(s.playerLoop, p)
	s.playersMu.Unlock()

	target := util.ToBase37("bob")

	// Burst 1: all 4 social-list opcodes — second through fourth must be
	// gated by socialProtect (set by the first).
	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("FriendListAdd: %v", err)
	}
	if err := handleFriendListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("FriendListDel: %v", err)
	}
	if err := handleIgnoreListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("IgnoreListAdd: %v", err)
	}
	if err := handleIgnoreListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("IgnoreListDel: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Errorf("burst 1: friends bridge calls = %d, want 1 (last 3 gated by socialProtect)", len(rec.friends))
	}

	// CHAT_SETMODE always fires (no socialProtect gate).
	if err := handleChatSetMode(p, []byte{0, 1, 0}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if len(rec.friends) != 2 {
		t.Errorf("after ChatSetMode: friends bridge = %d, want 2", len(rec.friends))
	}

	// REPORT_ABUSE in-range fires logger and sets reportAbuseProtect.
	if err := handleReportAbuse(p, reportAbusePayload(target, ReportAbuseOffensiveLanguage, false)); err != nil {
		t.Fatalf("ReportAbuse: %v", err)
	}
	if len(rec.logger) != 1 {
		t.Errorf("logger: got %d, want 1", len(rec.logger))
	}
	if !p.reportAbuseProtect {
		t.Error("reportAbuseProtect not set")
	}
	// Second ReportAbuse this tick is gated.
	rec.logger = nil
	if err := handleReportAbuse(p, reportAbusePayload(target, ReportAbuseOffensiveLanguage, false)); err != nil {
		t.Fatalf("ReportAbuse #2: %v", err)
	}
	if len(rec.logger) != 0 {
		t.Errorf("second ReportAbuse same tick: logger = %d, want 0 (gated)", len(rec.logger))
	}

	// Per-tick reset clears both protect flags.
	s.processCleanup()
	if p.socialProtect {
		t.Error("socialProtect not reset by processCleanup")
	}
	if p.reportAbuseProtect {
		t.Error("reportAbuseProtect not reset by processCleanup")
	}

	// Burst 2: post-reset, social handlers fire again.
	rec.friends = nil
	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("FriendListAdd post-reset: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Errorf("post-reset FriendListAdd: friends = %d, want 1", len(rec.friends))
	}
}
```

- [ ] **Step 2: Run smoke test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNAI72AllSixOpcodesDispatch -v
```

Expected: PASS.

- [ ] **Step 3: Final full regression sweep + race**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: PASS / no diagnostics.

### Close Step 4: Memory updates

- [ ] **Step 4: Append NAI-72 entry to `nai_followups.md`**

Append a `## NAI-72 — CLOSED 2026-05-02` block at the end of `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`, mirroring the NAI-71 close entry shape:

- Scope summary
- Cadence (4 tasks + close, Sonnet two-stage review)
- Spec + plan paths
- Close commit SHA + per-task SHAs (filled in at commit time)
- **Follow-ups closed:** none
- **Deviations opened:** all 4 with closure paths (post-erratum: STAFFMODLEVEL-DEAD-WRITER retracted)
- **Deviations closed:** none
- **Net deviation tally:** -0, +4 = 14 → 18 (post-erratum)
- **Wire-behaviour delta:** all 6 opcodes
- **Lessons confirmed:** runescript_cadence, true_to_ts_gate, controller_preflight (caught the staffModLevel pre-flight blocker), risk_register_premise_grep (R6 was the missed grep), defensive_gate_doc_comment_label, plan_grep_helper_patterns, enumerate_all_sites, superpowers_code_reviewer_model, execution_mode_default, close_commit_memory_trailer, spec_unconditional_swap_in_arm_block (verified not-applicable)
- **Lessons surfaced (saved as memory entries):** consider whether `risk_register_premise_grep.md` needs a 2nd-instance reinforcement note (NAI-72 R6 is a 2nd occurrence of the same analogy-reasoning failure mode after NAI-69)
- **Carry-forwards (still open after NAI-72):** all 4 new deviations + carry-forward unchanged from NAI-71

### Close Step 5: Close commit

- [ ] **Step 5: Final commit**

```bash
git add modules/world/handlers_game_social_smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-72 — social subsystem foundation
              (CHAT_SETMODE + Friend/Ignore + REPORT_ABUSE — 6 opcodes)

Closes 6 silent-discard slots in gameHandlers[]:
  244 CHAT_SETMODE
  118 FRIENDLIST_ADD
   11 FRIENDLIST_DEL
   79 IGNORELIST_ADD
  171 IGNORELIST_DEL
  190 REPORT_ABUSE

T1 Foundation (jstring fix + 2 Player fields + processCleanup +
   bridges + ReportAbuseReason)             (TBD-SHA)
T2 ChatSetMode handler                      (TBD-SHA)
T3 Friend/Ignore family (4 handlers)        (TBD-SHA)
T4 ReportAbuse handler                      (TBD-SHA)

Plus integration smoke pinning all 6 dispatch paths through one
recordingBridges fixture, including socialProtect / reportAbuseProtect
per-tick reset between bursts.

Wire-behaviour delta:
- Chat-set-mode requests now write Player.publicChat/privateChat/
  tradeDuel and propagate to friends-server (stub).
- Friend/Ignore add/remove now invoke the friends-server bridge
  (stub) and gate by socialProtect (one per tick per player).
- Report-abuse requests now invoke the logger bridge (stub),
  acknowledge the player, and gate by reportAbuseProtect; out-of-
  range reasons trigger an automated ban via the login-mod bridge.
- pkg/util/jstring.FromBase37 now matches TS — % 37 == 0 returns
  "invalid_name" (true-to-TS bug fix surfaced by social handler gate).

Net deviation tally: 14 → 18 (-0 / +4, post-erratum).

Opens memory: NAI-72-D-FRIENDS-SERVER-BRIDGE
Opens memory: NAI-72-D-LOGIN-SERVER-BRIDGE-MOD
Opens memory: NAI-72-D-LOGGER-BRIDGE
Opens memory: NAI-72-D-INPUT-RECORDING-NOT-PORTED

Spec: docs/superpowers/specs/2026-05-02-nai-72-social-subsystem-foundation-design.md
Plan: docs/superpowers/plans/2026-05-02-nai-72-social-subsystem-foundation.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Plan self-review checklist

| Spec section / requirement | Plan task |
|---|---|
| §3.2.1 Player fields | T1 Step 8 |
| §3.2.2 processCleanup hook | T1 Step 10 + Steps 6-7 (failing test) |
| §3.2.3 bridges.go (interfaces + noop) | T1 Step 12 |
| §3.2.3 recordingBridges (test) | T1 Step 13 |
| §3.2.4 Server fields + NewServer init | T1 Step 15 |
| §3.2.5 social.go (enum + reasonLabel) | T1 Steps 18-19 |
| §3.2.6 handler_chatsetmode.go | T2 Step 3 |
| §3.2.6 handler_social_list.go | T3 Step 3 |
| §3.2.6 handler_reportabuse.go | T4 Step 3 |
| §3.2.7 handlers_game.go init() entries | T2 Step 5, T3 Step 5, T4 Step 5 |
| §3.2.8 jstring.FromBase37 fix | T1 Step 3 |
| Stage 1 review focus areas | After T1, see "Stage 1 review" block |
| Stage 2 review focus areas | After T4, see "Stage 2 review" block |
| Integration smoke | Close Step 1 |
| 4 deviation tags opened (post-erratum) | Close Step 4 (memory) + Close Step 5 (commit trailer) |

No placeholders, no TBD/TODO except the close commit's per-task SHA blanks (filled at commit time). All file paths are absolute-from-repo-root. Every test step shows full test code; every implementation step shows full code. Method signatures match across tasks (`AddFriend(playerUsername string, target uint64)` is identical in interface, noop, recording, and 4 handler call sites).
