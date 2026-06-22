# NAI-73 — EventTracking + InputTracking + LoggerBridge realisation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the EVENT_TRACKING client opcode (81) and its supporting `InputTracking` per-player state machine, ship a real default `LoggerBridge` impl (`slogLoggerBridge`), and retroactively wire MACROING/BUG_ABUSE → `submitInput=true` in REPORT_ABUSE — closing `NAI-72-D-LOGGER-BRIDGE` and `NAI-72-D-INPUT-RECORDING-NOT-PORTED`, opening `NAI-73-D-INPUT-NO-SESSION-LOG-KICK`.

**Architecture:** Extend the existing `LoggerBridge` interface (NAI-72) with a second method `SubmitInputTracking`. Add a real default impl `slogLoggerBridge` writing structured slog records; bind it in `NewServer` to replace the `noopBridges{}` placeholder for the `loggerBridge` field. Add `Player.input *InputTracking`, `Player.submitInput bool`, `Player.session string` fields. Port `engine/entity/tracking/InputTracking.ts` as `modules/world/input_tracking.go` — a per-player state machine called from the last line of `Player.processIn`. The state machine schedules tracking windows on `(TRACKING_RATE=200, TRACKING_TIME=150, REMAINING_DATA_UPLOAD_LEEWAY=16)` ticks with `[-15,+15]` per-player jitter, sends `EnableTracking` (op 226) / `FinishTracking` (op 133) server packets at boundaries, accumulates blobs from the EVENT_TRACKING handler, and submits them via `LoggerBridge.SubmitInputTracking` when the window closes. The no-report kick branch sets `requestIdleLogout = true`; the TS `addSessionLog` call is deferred to a future session-log NAI.

**Tech Stack:** Go 1.26+, `math/rand/v2`, `log/slog`, `encoding/base64`. TS source canonical path: `LostCityRS/Engine-TS`.

**Predecessor:** NAI-72 (HEAD `74925f7`). Spec: `docs/superpowers/specs/2026-05-02-nai-73-event-tracking-input-recording-design.md` (commit `2eef62e`).

**Constants** (defined once in `input_tracking.go`):
- `inputTrackingRate = 200` — ticks between tracking sessions
- `inputTrackingTime = 150` — duration of a tracking window
- `inputTrackingRemainingDataUploadLeeway = 16` — grace ticks for trailing client data
- `inputTrackingJitterRange = 15` — `±15` ticks of jitter on first-scheduled start

**Premises verified at HEAD `74925f7`** (per `controller_preflight.md`):

```
$ rg -n "InputTracking|p\.input\b" modules/         (no hits — subsystem absent)
$ rg -n "submitInput\b" modules/                    (only cfg knob — Player field absent)
$ rg -n "p\.session\b" modules/                     (no hits — Player.session absent)
$ rg -n "OpEnableTracking|OpFinishTracking" pkg/    (no hits — opcodes absent)
$ rg -n "gameHandlers\[81\]" modules/               (no hits — handler unbound)
$ rg -n "LookupPlayerByUsername" modules/           (no hits — helper absent)
$ rg -n "loggerBridge\b" modules/world/server.go    (line 126,164 — field exists, default noopBridges{})
```

`pkg/coordgrid.PackCoord(level, x, z int) int` exists at `pkg/coordgrid/coordgrid.go:158` (use it for slogLoggerBridge coord serialisation).

`math/rand/v2` is goscape's RNG (per `npc_hunt.go:4`, `player.go:4`). Use package-level `rand.IntN(31) - 15` for jitter (matches `npc_interaction.go:86`).

`installRecordingBridges` is the test-pattern (`bridges_test.go:63`). All existing handler tests bind `recordingBridges` for the `loggerBridge` field, so swapping production from `noopBridges{}` to `slogLoggerBridge{}` won't affect existing tests.

---

## Task 1: Foundation — Player fields, config knob, server-prot opcodes

**Files:**
- Modify: `modules/world/player.go` — add 3 fields to `Player` struct
- Modify: `modules/world/config.go` — add `NodeLimitBytesPerTrackingSession` field + flag binding
- Modify: `pkg/io/protocol/game/server/prot.go` — add `OpEnableTracking`, `OpFinishTracking`
- Test: `pkg/io/protocol/game/server/prot_test.go` (extend existing test if any) OR add a new `prot_test.go` if absent

### Step 1.1: Add Player fields

- [ ] **Step 1.1.1: Read current Player struct around line 213**

Run: `sed -n '210,220p' modules/world/player.go` to confirm `requestIdleLogout` placement.

- [ ] **Step 1.1.2: Add fields after the `// === session flags ===` block**

Modify `modules/world/player.go` to add three new fields. Find the existing section (around line 210-215) and add directly after `requestLogout, requestIdleLogout, loggingOut bool`:

```go
	// === input tracking (NAI-73) ===
	// input is the per-player anti-cheat input-recording state machine.
	// Mirrors TS Player.input (Player.ts:305). Allocated in processLogins;
	// nil before login transitions to ClientStateGame.
	input *InputTracking
	// submitInput is the per-player gate for detailed tracking-event
	// submission. Set true by REPORT_ABUSE when reason ∈ {MACROING,
	// BUG_ABUSE} (TS World.notifyPlayerReport, World.ts:2298-2304).
	// Read by InputTracking.shouldSubmitTrackingDetails together with
	// cfg.NodeSubmitInput. Mirrors TS Player.submitInput (Player.ts:306).
	submitInput bool
	// session is the per-player session correlation key for the logger
	// bridge. Defaults to "headless" (TS Player.session = 'headless',
	// Player.ts:304). Real UUID assignment is owned by login-server-bridge
	// integration — tracked as NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.
	session string
```

The `*InputTracking` type does not yet exist — Go allows the struct field to reference an unresolved package-local type as long as the type is defined in the same package by build time. Task 3 defines `InputTracking`; the package will not build between Task 1 and Task 3 if these tasks are run sequentially.

**Mitigation for sequential builds:** Add a stub type at the end of Task 1 to keep the package compiling:

In `modules/world/player.go`, append at the very bottom of the file (or place in a new file `modules/world/input_tracking.go` as a forward declaration that Task 3 will replace):

```go
// InputTracking is the per-player input-recording state machine. Stub
// declaration only; full impl lands in NAI-73 Task 3.
//
// TODO(NAI-73-T3): Replace this stub with the real type.
type InputTracking struct{}
```

Place this stub in a **new file** `modules/world/input_tracking.go` so Task 3 simply replaces the file contents (no merge of player.go needed):

```go
// Package world: NAI-73 InputTracking per-player anti-cheat input-recording
// state machine. Mirrors TS engine/entity/tracking/InputTracking.ts.
package world

// InputTracking is the per-player input-recording state machine. Stub
// declaration only; full impl lands in NAI-73 Task 3.
//
// TODO(NAI-73-T3): Replace this stub with the real type and methods.
type InputTracking struct{}
```

- [ ] **Step 1.1.3: Run package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`

Expected: PASS.

- [ ] **Step 1.1.4: Commit**

```bash
git add modules/world/player.go modules/world/input_tracking.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T1.1 — Player.input/submitInput/session fields

Plumbing for the InputTracking subsystem ported in T3. Stub *InputTracking
type keeps the package compiling between T1 and T3."
```

### Step 1.2: Add config knob

- [ ] **Step 1.2.1: Add struct field**

Modify `modules/world/config.go`. After the existing `NodeSubmitInput` field at line 42:

Find:
```go
	NodeSubmitInput                  bool          `yaml:"node_submit_input"`
```

Add after it:
```go
	NodeLimitBytesPerTrackingSession int           `yaml:"node_limit_bytes_per_tracking_session"`
```

- [ ] **Step 1.2.2: Add flag binding**

Find the line registering `NodeSubmitInput` (around line 73):
```go
	f.BoolVar(&c.NodeSubmitInput, "world.node-submit-input", false, "Whether clients should be instructed to submit detailed tracking events to the server")
```

Add after it:
```go
	f.IntVar(&c.NodeLimitBytesPerTrackingSession, "world.node-limit-bytes-per-tracking-session", 50000, "Per-session upper limit on accumulated EVENT_TRACKING blob bytes before further blobs are dropped (TS Environment.NODE_LIMIT_BYTES_PER_TRACKING_SESSION default 50000)")
```

- [ ] **Step 1.2.3: Build and run config tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConfig -count=1`

Expected: PASS (existing tests still pass; new field default applies when not set).

- [ ] **Step 1.2.4: Add a default-value test**

Read `modules/world/config_test.go` to find the convention for default-value tests. If a `TestConfig*Defaults` test exists, extend it; otherwise add:

```go
func TestConfigNodeLimitBytesPerTrackingSessionDefault(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.NodeLimitBytesPerTrackingSession != 50000 {
		t.Errorf("NodeLimitBytesPerTrackingSession default: got %d, want 50000", c.NodeLimitBytesPerTrackingSession)
	}
}
```

If `flag` is not yet imported in the test file, add it.

- [ ] **Step 1.2.5: Run the new test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConfigNodeLimitBytesPerTrackingSessionDefault -count=1 -v`

Expected: PASS.

- [ ] **Step 1.2.6: Commit**

```bash
git add modules/world/config.go modules/world/config_test.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T1.2 — NodeLimitBytesPerTrackingSession config knob

Per-session upper limit on accumulated EVENT_TRACKING blob bytes. Mirrors
TS Environment.NODE_LIMIT_BYTES_PER_TRACKING_SESSION (default 50000)."
```

### Step 1.3: Add server-prot opcodes

- [ ] **Step 1.3.1: Read current server prot.go around line 50-60 to find an insertion point**

Run: `sed -n '46,65p' pkg/io/protocol/game/server/prot.go`

You should see opcode definitions like `OpRebuildNormal`, `OpUpdateInvFull` etc. Pick a logical place to insert — after the existing block of "info/state" opcodes (around line 60-65) is fine.

- [ ] **Step 1.3.2: Add opcode constants**

Modify `pkg/io/protocol/game/server/prot.go`. Add at the end of the existing var block (or in a logically grouped spot):

```go
	// Input-tracking signals — server tells client to start/stop sending
	// EVENT_TRACKING blobs (op 81). NAI-73; mirrors TS ServerGameProt.ts:43-44.
	OpEnableTracking = Op{Opcode: 226, PayloadSize: 0}
	OpFinishTracking = Op{Opcode: 133, PayloadSize: 0}
```

- [ ] **Step 1.3.3: Read existing prot_test.go pattern**

Run: `cat pkg/io/protocol/game/server/prot_test.go`

If the file exists with a table-driven test, extend the table. If absent, create it with the pattern from the **client** prot_test.go (which exists per the spec's §2.1 verification: `pkg/io/protocol/game/client/prot_test.go:18`).

- [ ] **Step 1.3.4: Add server-prot test entries**

If a test table exists, append entries:
```go
	{OpEnableTracking, 226, 0},
	{OpFinishTracking, 133, 0},
```

If `prot_test.go` does NOT exist for the server package, create `pkg/io/protocol/game/server/prot_test.go`:
```go
package server

import "testing"

func TestServerProtOpcodes(t *testing.T) {
	cases := []struct {
		op   Op
		code int
		size int
	}{
		{OpEnableTracking, 226, 0},
		{OpFinishTracking, 133, 0},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.code {
			t.Errorf("opcode: got %d, want %d", tc.op.Opcode, tc.code)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("payload size: got %d, want %d", tc.op.PayloadSize, tc.size)
		}
	}
}
```

- [ ] **Step 1.3.5: Run server-prot tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/server/ -count=1 -v`

Expected: PASS (new entries verified; existing tests still pass).

- [ ] **Step 1.3.6: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go pkg/io/protocol/game/server/prot_test.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T1.3 — server-prot opcodes 226/133 for input tracking

OpEnableTracking (226) / OpFinishTracking (133), both 0-payload. Mirrors
TS ServerGameProt.ENABLE_TRACKING / FINISH_TRACKING."
```

---

## Task 2: LoggerBridge extension + slogLoggerBridge default

**Files:**
- Modify: `modules/world/bridges.go` — extend `LoggerBridge` interface; add `noopBridges.SubmitInputTracking`
- Modify: `modules/world/bridges_test.go` — extend `recordingBridges` with `SubmitInputTracking`; add a `recordedInputTrackingCall` struct
- Create: `modules/world/logger_bridge.go` — `slogLoggerBridge` impl
- Create: `modules/world/logger_bridge_test.go` — slog-buffer tests for both bridge methods
- Modify: `modules/world/server.go` — `NewServer` swaps `loggerBridge` from `noopBridges{}` to `NewSlogLoggerBridge(s.log)`

### Step 2.1: Extend LoggerBridge interface

- [ ] **Step 2.1.1: Modify bridges.go**

Open `modules/world/bridges.go`. Find the `LoggerBridge` interface (currently lines 25-32):

```go
// LoggerBridge mirrors TS World.loggerThread.postMessage('report', ...).
// Real impl deferred via NAI-72-D-LOGGER-BRIDGE. The same closure path
// will activate the EventTracking handler.
type LoggerBridge interface {
	// NotifyPlayerReport posts an abuse report. reason is the string label
	// of the ReportAbuseReason enum value (e.g. "MACROING").
	NotifyPlayerReport(player *Player, offender, reason string)
}
```

Replace it with:

```go
// LoggerBridge is the structured-log sink for engine analytics events.
// Mirrors TS World.loggerThread.postMessage(...) for the 'report' and
// 'input_track' channels. Default impl is slogLoggerBridge (see
// logger_bridge.go); tests bind a recordingBridges capture impl.
type LoggerBridge interface {
	// NotifyPlayerReport posts an abuse report (TS World.notifyPlayerReport
	// at World.ts:2297-2313, channel 'report'). reason is the string label
	// of the ReportAbuseReason enum value (e.g. "MACROING").
	NotifyPlayerReport(player *Player, offender, reason string)

	// SubmitInputTracking posts a per-player input-recording blob from the
	// anti-cheat tracking subsystem (TS World.submitInputTracking at
	// World.ts:2314-2321, channel 'input_track'). blob is the raw bytes
	// from the EVENT_TRACKING client packet.
	SubmitInputTracking(player *Player, blob []byte)
}
```

Find the `noopBridges` impl block (lines 36-45) and add the new method after `NotifyPlayerReport`:

```go
func (noopBridges) SubmitInputTracking(*Player, []byte)        {}
```

- [ ] **Step 2.1.2: Build to confirm interface compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`

Expected: FAIL — `recordingBridges` no longer satisfies `LoggerBridge` (missing `SubmitInputTracking`). This validates that the compile-time interface assertions in `bridges_test.go:74-76` are catching the gap.

### Step 2.2: Extend recordingBridges (test impl)

- [ ] **Step 2.2.1: Modify bridges_test.go**

Open `modules/world/bridges_test.go`. After the `recordedLoggerCall` struct (around line 23-28), add a sibling struct:

```go
type recordedInputTrackingCall struct {
	method string // "SubmitInputTracking"
	player *Player
	blob   []byte
}
```

In the `recordingBridges` struct (around line 30-34), add a new field:

```go
type recordingBridges struct {
	friends      []recordedFriendsCall
	loginMod     []recordedLoginModCall
	logger       []recordedLoggerCall
	inputTracks  []recordedInputTrackingCall  // NAI-73
}
```

After the existing `NotifyPlayerReport` method (around line 57-59), add:

```go
func (r *recordingBridges) SubmitInputTracking(player *Player, blob []byte) {
	// Copy blob to defend against caller mutation.
	cp := make([]byte, len(blob))
	copy(cp, blob)
	r.inputTracks = append(r.inputTracks, recordedInputTrackingCall{method: "SubmitInputTracking", player: player, blob: cp})
}
```

- [ ] **Step 2.2.2: Run bridges_test.go to verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNoopBridgesAllMethods -count=1 -v`

Expected: FAIL — the test exercises every noop method but doesn't yet call `SubmitInputTracking`.

- [ ] **Step 2.2.3: Extend TestNoopBridgesAllMethods**

In `modules/world/bridges_test.go`, find `TestNoopBridgesAllMethods` (around line 85). Add a line at the end before the closing brace:

```go
	b.SubmitInputTracking(nil, []byte{1, 2, 3})
```

- [ ] **Step 2.2.4: Run again to confirm pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNoopBridgesAllMethods -count=1 -v`

Expected: PASS.

- [ ] **Step 2.2.5: Add a recordingBridges capture test**

Add a new test in `bridges_test.go` (or extend `TestRecordingBridgesCapturesAllCalls`):

```go
func TestRecordingBridgesCapturesSubmitInputTracking(t *testing.T) {
	rec := &recordingBridges{}
	rec.SubmitInputTracking(nil, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	if len(rec.inputTracks) != 1 {
		t.Fatalf("inputTracks: got %d, want 1", len(rec.inputTracks))
	}
	got := rec.inputTracks[0]
	if got.method != "SubmitInputTracking" {
		t.Errorf("method: got %q, want SubmitInputTracking", got.method)
	}
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !bytes.Equal(got.blob, want) {
		t.Errorf("blob: got %x, want %x", got.blob, want)
	}
	// Mutation defense
	got.blob[0] = 0x00
	if rec.inputTracks[0].blob[0] != 0xDE {
		t.Error("blob copy must be defensive (not aliasing caller bytes)")
	}
}
```

If `bytes` is not yet imported, add it.

- [ ] **Step 2.2.6: Run new test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRecordingBridgesCapturesSubmitInputTracking -count=1 -v`

Expected: PASS.

### Step 2.3: Add slogLoggerBridge default impl

- [ ] **Step 2.3.1: Create logger_bridge_test.go (failing test for NotifyPlayerReport)**

Create new file `modules/world/logger_bridge_test.go`:

```go
package world

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestSlogLoggerBridgeNotifyPlayerReport pins that NotifyPlayerReport
// emits a structured slog record with the expected keys: type=report,
// session, offender, reason, coord.
func TestSlogLoggerBridgeNotifyPlayerReport(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger)

	p := &Player{session: "test-session"}
	p.x = 3200
	p.z = 3200
	// p.level defaults to 0

	bridge.NotifyPlayerReport(p, "evilbob", "MACROING")

	out := buf.String()
	for _, want := range []string{
		"type=report",
		"session=test-session",
		"offender=evilbob",
		"reason=MACROING",
		"coord=", // packed value; exact value asserted separately
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}

// TestSlogLoggerBridgeSubmitInputTracking pins that SubmitInputTracking
// emits a record with type=input_track, session, blob_len, blob_b64.
func TestSlogLoggerBridgeSubmitInputTracking(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger)

	p := &Player{session: "test-session"}
	bridge.SubmitInputTracking(p, []byte{0x00, 0x01, 0x02})

	out := buf.String()
	for _, want := range []string{
		"type=input_track",
		"session=test-session",
		"blob_len=3",
		"blob_b64=AAEC", // base64 of 0x000102
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}
```

- [ ] **Step 2.3.2: Run failing test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSlogLoggerBridge -count=1`

Expected: FAIL — `NewSlogLoggerBridge` undefined.

- [ ] **Step 2.3.3: Create logger_bridge.go**

Create new file `modules/world/logger_bridge.go`:

```go
package world

import (
	"encoding/base64"
	"log/slog"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// slogLoggerBridge is the production default LoggerBridge impl. Emits
// one structured slog record per call under a child logger keyed
// component=logger_bridge. NAI-73 closes NAI-72-D-LOGGER-BRIDGE by
// shipping this default; tests still bind recordingBridges via
// installRecordingBridges(s).
type slogLoggerBridge struct {
	log *slog.Logger
}

// NewSlogLoggerBridge wraps parent in a child logger keyed
// component=logger_bridge.
func NewSlogLoggerBridge(parent *slog.Logger) *slogLoggerBridge {
	return &slogLoggerBridge{log: parent.With("component", "logger_bridge")}
}

// NotifyPlayerReport emits a 'report' record. Mirrors TS
// World.notifyPlayerReport's loggerThread.postMessage call (World.ts:2305).
func (b *slogLoggerBridge) NotifyPlayerReport(p *Player, offender, reason string) {
	b.log.Info("player_report",
		"type", "report",
		"session", p.session,
		"coord", coordgrid.PackCoord(int(p.level), int(p.x), int(p.z)),
		"offender", offender,
		"reason", reason,
	)
}

// SubmitInputTracking emits an 'input_track' record. Mirrors TS
// World.submitInputTracking's loggerThread.postMessage call (World.ts:2315).
// blob is base64-encoded for log readability and to match TS
// Buffer.from(buf).toString('base64') (World.ts:2319).
func (b *slogLoggerBridge) SubmitInputTracking(p *Player, blob []byte) {
	b.log.Info("input_track",
		"type", "input_track",
		"session", p.session,
		"blob_len", len(blob),
		"blob_b64", base64.StdEncoding.EncodeToString(blob),
	)
}

// Compile-time interface satisfaction.
var _ LoggerBridge = (*slogLoggerBridge)(nil)
```

- [ ] **Step 2.3.4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSlogLoggerBridge -count=1 -v`

Expected: PASS for both tests.

### Step 2.4: Wire slogLoggerBridge as the production default

- [ ] **Step 2.4.1: Read NewServer around lines 160-170**

Run: `sed -n '155,170p' modules/world/server.go`

You should see `s.loggerBridge = noopBridges{}` at line 164.

- [ ] **Step 2.4.2: Replace the noopBridges binding for loggerBridge**

Modify `modules/world/server.go`. Find:
```go
	s.loggerBridge = noopBridges{}
```

Replace with:
```go
	s.loggerBridge = NewSlogLoggerBridge(s.log)
```

The other two bindings (`s.friendsBridge = noopBridges{}` and `s.loginBridgeMod = noopBridges{}`) are unchanged — their deviations (FRIENDS-SERVER-BRIDGE, LOGIN-SERVER-BRIDGE-MOD) remain open.

- [ ] **Step 2.4.3: Run full world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: PASS (all existing tests use `installRecordingBridges` which overrides the default, so the prod-default swap should not affect them).

If any test fails: it likely asserts that *no* logger output happened during a server-init path. Inspect; either bind `noopBridges{}` explicitly in that test, or update assertions.

- [ ] **Step 2.4.4: Commit**

```bash
git add modules/world/bridges.go modules/world/bridges_test.go modules/world/logger_bridge.go modules/world/logger_bridge_test.go modules/world/server.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T2 — LoggerBridge.SubmitInputTracking + slogLoggerBridge default

Extends LoggerBridge with the second TS channel ('input_track'). Real
default impl ships (slogLoggerBridge wrapping *slog.Logger) and replaces
the noopBridges binding for the loggerBridge field in NewServer. Closes
NAI-72-D-LOGGER-BRIDGE in interface terms; T7 retires the deviation tags
in doc-comments after T3-T6 land.

friendsBridge and loginBridgeMod retain their noopBridges bindings —
FRIENDS-SERVER-BRIDGE and LOGIN-SERVER-BRIDGE-MOD deviations unchanged."
```

---

## Task 3: InputTracking entity

**Files:**
- Modify (replace): `modules/world/input_tracking.go` — full state machine (replaces the T1.1 stub)
- Create: `modules/world/input_tracking_test.go` — table-driven state-machine tests
- Modify: `modules/world/player.go` — add `WriteEnableTracking` / `WriteFinishTracking` methods

### Step 3.1: Add WriteEnableTracking / WriteFinishTracking helpers

- [ ] **Step 3.1.1: Modify player.go**

Open `modules/world/player.go`. After an existing `writeOut`-based helper (e.g. find the location of `func (p *Player) MessageGame` or similar), add:

```go
// WriteEnableTracking sends the EnableTracking server packet (op 226,
// 0 payload). Mirrors TS InputTracking.enable() at InputTracking.ts:102.
// Called only from InputTracking.enable().
func (p *Player) WriteEnableTracking() {
	p.writeOut(gameserver.OpEnableTracking, nil)
}

// WriteFinishTracking sends the FinishTracking server packet (op 133,
// 0 payload). Mirrors TS InputTracking.disable() at InputTracking.ts:114.
// Called only from InputTracking.disable().
func (p *Player) WriteFinishTracking() {
	p.writeOut(gameserver.OpFinishTracking, nil)
}
```

The `gameserver` import alias is the same one used by other writeOut callers; verify by grepping for `gameserver.OpMessageGame` in `message_game.go`.

- [ ] **Step 3.1.2: Build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`

Expected: PASS.

### Step 3.2: Replace input_tracking.go stub with full impl

- [ ] **Step 3.2.1: Read TS InputTracking.ts:1-163 line-by-line as the source of truth**

Reference: `$HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/tracking/InputTracking.ts`

Key sections:
- Lines 10-14: timing constants
- Lines 16-39: fields + constructor
- Lines 41-67: `nextScheduledTrackingStart` / `nextScheduledTrackingEnd` / `shouldStartTracking` / `shouldEndTracking`
- Lines 73-92: `onCycle()`
- Lines 94-115: `enable()` / `disable()`
- Lines 117-128: `isActive()` / `shouldSubmitTrackingDetails()`
- Lines 130-133: `record()`
- Lines 140-158: `submitEvents()`
- Lines 160-162: `offset()` (uniform random in `[-n, +n]`)

- [ ] **Step 3.2.2: Replace the input_tracking.go stub**

Replace the entire contents of `modules/world/input_tracking.go` with:

```go
// Package world: NAI-73 InputTracking per-player anti-cheat input-recording
// state machine. Line-by-line port of TS engine/entity/tracking/
// InputTracking.ts. Closes NAI-72-D-INPUT-RECORDING-NOT-PORTED.
package world

import (
	"math/rand/v2"
)

// Timing constants — mirror TS InputTracking.ts:10-14.
const (
	// inputTrackingRate is the number of ticks between scheduled tracking
	// sessions (~120s at 600ms/tick). TS InputTracking.TRACKING_RATE.
	inputTrackingRate = 200
	// inputTrackingTime is the duration of each tracking window
	// (~90s). TS InputTracking.TRACKING_TIME.
	inputTrackingTime = 150
	// inputTrackingRemainingDataUploadLeeway is the grace period after a
	// tracking window closes during which the client may still flush
	// trailing EVENT_TRACKING blobs (~10s). TS
	// InputTracking.REMAINING_DATA_UPLOAD_LEEWAY.
	inputTrackingRemainingDataUploadLeeway = 16
	// inputTrackingJitterRange is the absolute bound on the
	// per-player random offset added to the first scheduled
	// tracking-start tick. Yields uniform [-15, +15]. TS InputTracking.offset(15).
	inputTrackingJitterRange = 15
	// inputTrackingMaxBlobBytes is the per-packet upper bound on the
	// EVENT_TRACKING client payload (the handler-side gate). TS
	// EventTrackingHandler.ts:9 (`bytes.length > 500`).
	inputTrackingMaxBlobBytes = 500
)

// InputTracking is the per-player input-recording state machine. Mirrors
// TS InputTracking class. One instance per logged-in Player, allocated
// in processLogins. Owns scheduling (start/end window ticks), recorded
// blob accumulation, and end-of-window submission to the LoggerBridge.
type InputTracking struct {
	// player is the back-pointer used for:
	//  - reading submitInput (shouldSubmitTrackingDetails)
	//  - reading client.server.cfg (NodeSubmitInput, NodeDebug,
	//    NodeLimitBytesPerTrackingSession)
	//  - reading client.server.loggerBridge (submitEvents)
	//  - writing requestIdleLogout (submitEvents kick branch)
	//  - calling WriteEnableTracking / WriteFinishTracking (enable/disable)
	player *Player

	// hasSeenReport: at least one EVENT_TRACKING report received this
	// session window. Pinned by EventTrackingHandler.handle.
	// TS InputTracking.ts:19.
	hasSeenReport bool
	// waitingForRemainingData: tracking window has closed but the
	// REMAINING_DATA_UPLOAD_LEEWAY grace has not yet expired. TS
	// InputTracking.ts:21.
	waitingForRemainingData bool
	// enabled: tracking is currently active (between startTrackingAt and
	// endTrackingAt, inclusive). TS InputTracking.ts:24.
	enabled bool

	// startTrackingAt: tick at which the next/current tracking window opens.
	// TS InputTracking.ts:27.
	startTrackingAt int
	// endTrackingAt: tick at which the next/current tracking window closes.
	// TS InputTracking.ts:30.
	endTrackingAt int

	// recordedBlobs: accumulated EVENT_TRACKING payloads for this window.
	// Submitted (as recordedBlobs[0] only — TS quirk) at submitEvents.
	// TS InputTracking.ts:33.
	recordedBlobs [][]byte
	// recordedBlobsSizeTotal: byte total across all recordedBlobs. Compared
	// against cfg.NodeLimitBytesPerTrackingSession by the handler. TS
	// InputTracking.ts:35.
	recordedBlobsSizeTotal int
}

// NewInputTracking allocates a fresh InputTracking for player. Initial
// startTrackingAt is set to currentTick + inputTrackingRate + jitter
// (uniform [-15, +15]); endTrackingAt is startTrackingAt +
// inputTrackingTime. Mirrors TS InputTracking constructor (line 37-39
// + initial-value expressions on lines 27, 30).
func NewInputTracking(player *Player, currentTick int) *InputTracking {
	t := &InputTracking{player: player}
	t.startTrackingAt = t.nextScheduledTrackingStart(currentTick)
	t.endTrackingAt = t.startTrackingAt + inputTrackingTime
	return t
}

// nextScheduledTrackingStart returns the tick at which the next
// tracking session should start. Mirrors TS
// InputTracking.nextScheduledTrackingStart (lines 44-46).
func (t *InputTracking) nextScheduledTrackingStart(currentTick int) int {
	return currentTick + inputTrackingRate + offset(inputTrackingJitterRange)
}

// shouldStartTracking returns true when the current tick has reached or
// passed startTrackingAt. Mirrors TS line 58-60.
func (t *InputTracking) shouldStartTracking(currentTick int) bool {
	return currentTick >= t.startTrackingAt
}

// shouldEndTracking returns true when the current tick has reached or
// passed endTrackingAt. Mirrors TS line 65-67.
func (t *InputTracking) shouldEndTracking(currentTick int) bool {
	return currentTick >= t.endTrackingAt
}

// IsActive reports whether the tracking window is currently open or in
// the post-close grace period. Mirrors TS isActive (lines 117-120).
// Consumed by the EVENT_TRACKING handler as its second gate.
func (t *InputTracking) IsActive(currentTick int) bool {
	withinTicks := currentTick >= t.startTrackingAt && currentTick <= t.endTrackingAt
	return withinTicks || t.waitingForRemainingData
}

// ShouldSubmitTrackingDetails reports whether the player should
// actually submit blob data (vs just acknowledging IsActive). Mirrors
// TS shouldSubmitTrackingDetails (lines 126-128).
func (t *InputTracking) ShouldSubmitTrackingDetails() bool {
	if t.player == nil || t.player.client == nil || t.player.client.server == nil {
		return false
	}
	return t.player.submitInput || t.player.client.server.cfg.NodeSubmitInput
}

// Record appends rawData to recordedBlobs and grows the size total.
// Mirrors TS record (lines 130-133). Caller is responsible for any
// gating (the handler checks IsActive, ShouldSubmitTrackingDetails,
// and recordedBlobsSizeTotal cap before calling Record).
func (t *InputTracking) Record(rawData []byte) {
	t.recordedBlobsSizeTotal += len(rawData)
	t.recordedBlobs = append(t.recordedBlobs, rawData)
}

// enable transitions tracking to active. Mirrors TS enable (lines 94-103).
// Called from OnCycle only.
func (t *InputTracking) enable(currentTick int) {
	if t.enabled {
		return
	}
	t.enabled = true
	t.startTrackingAt = currentTick                   // enabled immediately
	t.endTrackingAt = t.startTrackingAt + inputTrackingTime
	t.player.WriteEnableTracking()
}

// disable transitions tracking to inactive and starts the
// REMAINING_DATA_UPLOAD_LEEWAY grace. Mirrors TS disable (lines 105-115).
// Called from OnCycle only.
func (t *InputTracking) disable(currentTick int) {
	if !t.enabled {
		return
	}
	t.enabled = false
	t.startTrackingAt = t.nextScheduledTrackingStart(currentTick) // at the next interval
	t.endTrackingAt = currentTick                                  // disabled immediately
	t.waitingForRemainingData = true
	t.player.WriteFinishTracking()
}

// submitEvents finalises the window. Mirrors TS submitEvents (lines 140-158).
// Branches:
//   - hasSeenReport && shouldSubmit → loggerBridge.SubmitInputTracking(player, recordedBlobs[0])
//     (TS submits only blob index 0, even when multiple blobs were recorded — quirk preserved).
//   - !hasSeenReport && !cfg.NodeDebug → requestIdleLogout = true
//     (TS additionally calls addSessionLog(ENGINE, "Client did not submit
//     an input tracking report") which is deferred via
//     NAI-73-D-INPUT-NO-SESSION-LOG-KICK; structured-log entry is
//     missing in goscape until the session-log NAI lands).
//
// All branches reset waitingForRemainingData / recordedBlobs /
// recordedBlobsSizeTotal / hasSeenReport (TS lines 154-157).
func (t *InputTracking) submitEvents() {
	s := t.player.client.server
	if t.hasSeenReport {
		if t.ShouldSubmitTrackingDetails() {
			s.loggerBridge.SubmitInputTracking(t.player, t.recordedBlobs[0])
		}
	} else if !s.cfg.NodeDebug {
		// NAI-73-D-INPUT-NO-SESSION-LOG-KICK: TS also calls
		// player.addSessionLog(LoggerEventType.ENGINE, ...) on this
		// branch (InputTracking.ts:150). Goscape has no session-log
		// subsystem yet; deferred to a future session-log NAI.
		t.player.requestIdleLogout = true
	}
	t.waitingForRemainingData = false
	t.recordedBlobs = nil
	t.recordedBlobsSizeTotal = 0
	t.hasSeenReport = false
}

// OnCycle is the per-tick state-machine dispatch. Mirrors TS onCycle
// (lines 73-92). Called from Player.processInputTracking, which is
// called from the last line of Player.processIn (mirrors TS World.ts:646
// — same per-player iteration of the client-input phase).
func (t *InputTracking) OnCycle(currentTick int) {
	if t.waitingForRemainingData {
		if t.endTrackingAt+inputTrackingRemainingDataUploadLeeway < currentTick {
			t.submitEvents()
		}
		return
	}
	if t.shouldStartTracking(currentTick) && !t.enabled {
		t.enable(currentTick)
		return
	}
	if t.shouldEndTracking(currentTick) && t.enabled {
		t.disable(currentTick)
		return
	}
}

// offset returns a uniform random integer in [-n, +n]. Mirrors TS
// offset (lines 160-162) which uses Math.random; goscape uses
// math/rand/v2 package-level rand.IntN per the existing convention
// (npc_interaction.go:86, npc_hunt.go:82).
func offset(n int) int {
	return rand.IntN(n*2+1) - n
}
```

- [ ] **Step 3.2.3: Build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`

Expected: PASS.

### Step 3.3: TDD — IsActive matrix

- [ ] **Step 3.3.1: Create input_tracking_test.go with IsActive table**

Create new file `modules/world/input_tracking_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// inputTrackingTestSetup wires a Player against a Server with
// recordingBridges and a configured cfg. Returns the tracking entity,
// player, and recorder for assertions. The currentTick parameter
// initialises t.startTrackingAt so callers can deterministically place
// the window relative to test ticks.
func inputTrackingTestSetup(t *testing.T, currentTick int) (*InputTracking, *Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	rec := installRecordingBridges(s)
	// Deterministic placement: caller sets startTrackingAt directly
	// after this returns, so jitter from NewInputTracking does not
	// affect the test.
	tt := &InputTracking{player: p}
	return tt, p, rec
}

// TestInputTrackingIsActiveMatrix pins the 4 corners of IsActive.
func TestInputTrackingIsActiveMatrix(t *testing.T) {
	cases := []struct {
		name        string
		currentTick int
		startAt     int
		endAt       int
		waiting     bool
		want        bool
	}{
		{"pre-window", 99, 100, 200, false, false},
		{"on-start", 100, 100, 200, false, true},
		{"mid-window", 150, 100, 200, false, true},
		{"on-end", 200, 100, 200, false, true},
		{"post-window", 201, 100, 200, false, false},
		{"post-window-but-waiting", 201, 100, 200, true, true},
		{"pre-window-but-waiting", 99, 100, 200, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, _, _ := inputTrackingTestSetup(t, 0)
			tt.startTrackingAt = tc.startAt
			tt.endTrackingAt = tc.endAt
			tt.waitingForRemainingData = tc.waiting
			got := tt.IsActive(tc.currentTick)
			if got != tc.want {
				t.Errorf("IsActive(%d) startAt=%d endAt=%d waiting=%v: got %v, want %v",
					tc.currentTick, tc.startAt, tc.endAt, tc.waiting, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3.3.2: Run test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTrackingIsActiveMatrix -count=1 -v`

Expected: PASS for all 7 cases.

### Step 3.4: TDD — ShouldSubmitTrackingDetails

- [ ] **Step 3.4.1: Add test**

Append to `input_tracking_test.go`:

```go
// TestInputTrackingShouldSubmitTrackingDetailsMatrix pins the 2x2 OR
// of (player.submitInput, cfg.NodeSubmitInput).
func TestInputTrackingShouldSubmitTrackingDetailsMatrix(t *testing.T) {
	cases := []struct {
		name            string
		playerSubmit    bool
		cfgSubmit       bool
		want            bool
	}{
		{"both-false", false, false, false},
		{"player-only", true, false, true},
		{"cfg-only", false, true, true},
		{"both-true", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _ := inputTrackingTestSetup(t, 0)
			p.submitInput = tc.playerSubmit
			p.client.server.cfg.NodeSubmitInput = tc.cfgSubmit
			got := tt.ShouldSubmitTrackingDetails()
			if got != tc.want {
				t.Errorf("ShouldSubmitTrackingDetails: playerSubmit=%v cfgSubmit=%v: got %v, want %v",
					tc.playerSubmit, tc.cfgSubmit, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3.4.2: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTrackingShouldSubmitTrackingDetailsMatrix -count=1 -v`

Expected: PASS.

### Step 3.5: TDD — Record

- [ ] **Step 3.5.1: Add test**

```go
// TestInputTrackingRecord pins blob accumulation and size totalisation.
func TestInputTrackingRecord(t *testing.T) {
	tt, _, _ := inputTrackingTestSetup(t, 0)
	tt.Record([]byte{1, 2, 3})
	tt.Record([]byte{4, 5})
	if got, want := len(tt.recordedBlobs), 2; got != want {
		t.Errorf("recordedBlobs len: got %d, want %d", got, want)
	}
	if got, want := tt.recordedBlobsSizeTotal, 5; got != want {
		t.Errorf("recordedBlobsSizeTotal: got %d, want %d", got, want)
	}
	if !bytes.Equal(tt.recordedBlobs[0], []byte{1, 2, 3}) {
		t.Errorf("recordedBlobs[0]: got %x, want 010203", tt.recordedBlobs[0])
	}
}
```

- [ ] **Step 3.5.2: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTrackingRecord -count=1 -v`

Expected: PASS.

### Step 3.6: TDD — enable() writes EnableTracking and sets state

- [ ] **Step 3.6.1: Add test**

```go
// TestInputTrackingEnable pins enable's state transitions and the
// EnableTracking server-packet write.
func TestInputTrackingEnable(t *testing.T) {
	tt, p, _ := inputTrackingTestSetup(t, 0)
	tt.enabled = false
	tt.startTrackingAt = 1000 // will be overwritten to currentTick

	tt.enable(500)

	if !tt.enabled {
		t.Error("enabled: must be true after enable()")
	}
	if got, want := tt.startTrackingAt, 500; got != want {
		t.Errorf("startTrackingAt: got %d, want %d (currentTick at enable)", got, want)
	}
	if got, want := tt.endTrackingAt, 500+inputTrackingTime; got != want {
		t.Errorf("endTrackingAt: got %d, want %d", got, want)
	}

	// Verify EnableTracking packet was written. The byte layout is the
	// ISAAC-encrypted opcode (single byte; OpEnableTracking has 0
	// payload). Drain the client out-stream and decode by checking length.
	out := drainClientOut(t, p)
	if len(out) != 1 {
		t.Fatalf("client out: got %d bytes, want 1 (EnableTracking opcode)", len(out))
	}
	// Decode the ISAAC-encrypted byte against a parallel encryptor seeded
	// the same way to verify the opcode.
	parallel := io2.New([4]uint32{1, 2, 3, 4})
	wantOpcode := byte(226 + parallel.NextInt())
	if out[0] != wantOpcode {
		t.Errorf("EnableTracking opcode (encrypted): got %d, want %d", out[0], wantOpcode)
	}
}

// TestInputTrackingEnableIdempotent pins that calling enable() when
// already enabled is a no-op.
func TestInputTrackingEnableIdempotent(t *testing.T) {
	tt, p, _ := inputTrackingTestSetup(t, 0)
	tt.enabled = true
	tt.startTrackingAt = 100
	tt.endTrackingAt = 250

	tt.enable(500)

	if got, want := tt.startTrackingAt, 100; got != want {
		t.Errorf("startTrackingAt should not change: got %d, want %d", got, want)
	}
	out := drainClientOut(t, p)
	if len(out) != 0 {
		t.Errorf("idempotent enable should not write: got %d bytes", len(out))
	}
}
```

**Refactor needed before this step:** the existing `inputTrackingTestSetup` from Step 3.3.1 returns 3 values (`*InputTracking`, `*Player`, `*recordingBridges`). To verify server-packet writes, callers also need the client-side end of the test pipe (`cc`, returned as `newTestPlayer`'s second value). Update `inputTrackingTestSetup` to return 4 values and update prior test callers (IsActive, ShouldSubmit, Record) accordingly:

```go
func inputTrackingTestSetup(t *testing.T, currentTick int) (*InputTracking, *Player, net.Conn, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	rec := installRecordingBridges(s)
	tt := &InputTracking{player: p}
	return tt, p, cc, rec
}
```

Then add the `drainClientOut` helper:

```go
// drainClientOut reads everything currently buffered on the client-side
// end of the test pipe and returns it. Verifies server-packet writes
// from InputTracking.enable / disable.
func drainClientOut(t *testing.T, p *Player, cc net.Conn) []byte {
	t.Helper()
	if err := p.client.flushWrite(); err != nil {
		t.Fatalf("flushWrite: %v", err)
	}
	if err := cc.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 4096)
	n, _ := cc.Read(buf)
	// EOF / timeout is expected when no bytes are queued (returns n=0).
	return buf[:n]
}
```

Add `import "net"` and `import "time"` to `input_tracking_test.go`.

Update prior test signatures (Step 3.3, 3.4, 3.5) to use the 4-value return: `tt, _, _, _ := inputTrackingTestSetup(t, 0)` (or capture as needed).

If existing tests in the codebase already use a different drain helper (e.g. `data_map_test.go:18-22`'s pattern), prefer that over rolling a new one.

- [ ] **Step 3.6.2: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTrackingEnable -count=1 -v`

Expected: PASS for both `TestInputTrackingEnable` and `TestInputTrackingEnableIdempotent`.

### Step 3.7: TDD — disable()

- [ ] **Step 3.7.1: Add test**

```go
// TestInputTrackingDisable pins disable's state transitions and the
// FinishTracking server-packet write.
func TestInputTrackingDisable(t *testing.T) {
	tt, p, cc, _ := inputTrackingTestSetup(t, 0)
	tt.enabled = true
	tt.startTrackingAt = 100
	tt.endTrackingAt = 250

	tt.disable(300)

	if tt.enabled {
		t.Error("enabled: must be false after disable()")
	}
	if !tt.waitingForRemainingData {
		t.Error("waitingForRemainingData: must be true after disable()")
	}
	if got, want := tt.endTrackingAt, 300; got != want {
		t.Errorf("endTrackingAt: got %d, want %d (currentTick at disable)", got, want)
	}
	// startTrackingAt rescheduled to a future tick — exact value depends on
	// jitter, but it must be in [300+inputTrackingRate-15, 300+inputTrackingRate+15].
	wantMin := 300 + inputTrackingRate - inputTrackingJitterRange
	wantMax := 300 + inputTrackingRate + inputTrackingJitterRange
	if tt.startTrackingAt < wantMin || tt.startTrackingAt > wantMax {
		t.Errorf("startTrackingAt: got %d, want in [%d, %d]", tt.startTrackingAt, wantMin, wantMax)
	}

	// FinishTracking packet was written.
	out := drainClientOut(t, p, cc)
	if len(out) != 1 {
		t.Fatalf("client out: got %d bytes, want 1 (FinishTracking opcode)", len(out))
	}
}

// TestInputTrackingDisableIdempotent pins that disable() when already
// disabled is a no-op.
func TestInputTrackingDisableIdempotent(t *testing.T) {
	tt, p, cc, _ := inputTrackingTestSetup(t, 0)
	tt.enabled = false

	tt.disable(300)

	if tt.waitingForRemainingData {
		t.Error("waitingForRemainingData should not be set on no-op disable")
	}
	out := drainClientOut(t, p, cc)
	if len(out) != 0 {
		t.Errorf("idempotent disable should not write: got %d bytes", len(out))
	}
}
```

- [ ] **Step 3.7.2: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTrackingDisable -count=1 -v`

Expected: PASS for both.

### Step 3.8: TDD — submitEvents 4-branch matrix

- [ ] **Step 3.8.1: Add test**

```go
// TestInputTrackingSubmitEventsMatrix pins all 4 branches of
// submitEvents (TS InputTracking.submitEvents at lines 140-158).
func TestInputTrackingSubmitEventsMatrix(t *testing.T) {
	cases := []struct {
		name            string
		hasSeenReport   bool
		shouldSubmit    bool
		nodeDebug       bool
		blobsBefore     [][]byte
		wantBridgeCalls int
		wantKick        bool
		wantSubmittedBlob []byte
	}{
		{
			name:            "report+submit→bridge",
			hasSeenReport:   true,
			shouldSubmit:    true,
			nodeDebug:       false,
			blobsBefore:     [][]byte{{0xAA}, {0xBB}, {0xCC}},
			wantBridgeCalls: 1,
			wantKick:        false,
			wantSubmittedBlob: []byte{0xAA}, // TS quirk: only blob[0]
		},
		{
			name:            "report+!submit→nothing",
			hasSeenReport:   true,
			shouldSubmit:    false,
			nodeDebug:       false,
			blobsBefore:     [][]byte{{0xAA}},
			wantBridgeCalls: 0,
			wantKick:        false,
		},
		{
			name:            "!report+!debug→kick",
			hasSeenReport:   false,
			shouldSubmit:    false,
			nodeDebug:       false,
			blobsBefore:     nil,
			wantBridgeCalls: 0,
			wantKick:        true,
		},
		{
			name:            "!report+debug→nothing",
			hasSeenReport:   false,
			shouldSubmit:    false,
			nodeDebug:       true,
			blobsBefore:     nil,
			wantBridgeCalls: 0,
			wantKick:        false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _, rec := inputTrackingTestSetup(t, 0)
			tt.hasSeenReport = tc.hasSeenReport
			tt.recordedBlobs = tc.blobsBefore
			tt.recordedBlobsSizeTotal = 0
			for _, b := range tc.blobsBefore {
				tt.recordedBlobsSizeTotal += len(b)
			}
			tt.waitingForRemainingData = true
			p.submitInput = tc.shouldSubmit
			p.client.server.cfg.NodeSubmitInput = false
			p.client.server.cfg.NodeDebug = tc.nodeDebug
			p.requestIdleLogout = false

			tt.submitEvents()

			if got := len(rec.inputTracks); got != tc.wantBridgeCalls {
				t.Errorf("bridge calls: got %d, want %d", got, tc.wantBridgeCalls)
			}
			if tc.wantBridgeCalls > 0 && tc.wantSubmittedBlob != nil {
				if !bytes.Equal(rec.inputTracks[0].blob, tc.wantSubmittedBlob) {
					t.Errorf("submitted blob: got %x, want %x", rec.inputTracks[0].blob, tc.wantSubmittedBlob)
				}
			}
			if got := p.requestIdleLogout; got != tc.wantKick {
				t.Errorf("requestIdleLogout: got %v, want %v", got, tc.wantKick)
			}

			// Reset invariants — every branch must clear state.
			if tt.waitingForRemainingData {
				t.Error("waitingForRemainingData: must be false after submitEvents")
			}
			if tt.recordedBlobs != nil {
				t.Errorf("recordedBlobs: must be nil after submitEvents, got %v", tt.recordedBlobs)
			}
			if tt.recordedBlobsSizeTotal != 0 {
				t.Errorf("recordedBlobsSizeTotal: must be 0 after submitEvents, got %d", tt.recordedBlobsSizeTotal)
			}
			if tt.hasSeenReport {
				t.Error("hasSeenReport: must be false after submitEvents")
			}
		})
	}
}
```

- [ ] **Step 3.8.2: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTrackingSubmitEventsMatrix -count=1 -v`

Expected: PASS for all 4 branches.

### Step 3.9: TDD — OnCycle dispatch

- [ ] **Step 3.9.1: Add test**

```go
// TestInputTrackingOnCycleDispatch pins OnCycle's branch dispatch.
// Each case pins one of: enable, disable, grace-expired-submit, or no-op.
func TestInputTrackingOnCycleDispatch(t *testing.T) {
	cases := []struct {
		name             string
		startAt          int
		endAt            int
		enabled          bool
		waiting          bool
		currentTick      int
		wantEnabled      bool
		wantWaiting      bool
		wantClearedBlobs bool
	}{
		{"pre-window-noop", 100, 250, false, false, 50, false, false, false},
		{"on-start-enable", 100, 250, false, false, 100, true, false, false},
		{"mid-window-noop", 100, 250, true, false, 175, true, false, false},
		{"on-end-disable", 100, 250, true, false, 250, false, true, false},
		{"waiting-grace-not-expired", 100, 250, false, true, 250 + inputTrackingRemainingDataUploadLeeway, false, true, false},
		{"waiting-grace-expired-submit", 100, 250, false, true, 250 + inputTrackingRemainingDataUploadLeeway + 1, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _, _ := inputTrackingTestSetup(t, 0)
			tt.startTrackingAt = tc.startAt
			tt.endTrackingAt = tc.endAt
			tt.enabled = tc.enabled
			tt.waitingForRemainingData = tc.waiting
			tt.recordedBlobs = [][]byte{{0xAA}} // populate so we can detect submitEvents reset
			tt.recordedBlobsSizeTotal = 1
			p.client.server.cfg.NodeDebug = true // suppress kick

			tt.OnCycle(tc.currentTick)

			if got := tt.enabled; got != tc.wantEnabled {
				t.Errorf("enabled: got %v, want %v", got, tc.wantEnabled)
			}
			if got := tt.waitingForRemainingData; got != tc.wantWaiting {
				t.Errorf("waitingForRemainingData: got %v, want %v", got, tc.wantWaiting)
			}
			cleared := tt.recordedBlobs == nil
			if cleared != tc.wantClearedBlobs {
				t.Errorf("recordedBlobs cleared: got %v, want %v", cleared, tc.wantClearedBlobs)
			}
		})
	}
}
```

- [ ] **Step 3.9.2: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTrackingOnCycleDispatch -count=1 -v`

Expected: PASS for all 6 cases.

### Step 3.10: Run full input-tracking test suite

- [ ] **Step 3.10.1**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInputTracking -count=1 -v`

Expected: PASS — all `TestInputTracking*` tests.

- [ ] **Step 3.10.2: Commit**

```bash
git add modules/world/input_tracking.go modules/world/input_tracking_test.go modules/world/player.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T3 — InputTracking entity port

Line-by-line port of TS engine/entity/tracking/InputTracking.ts:1-163.
State-machine OnCycle handles enable/disable/grace-expired-submit
transitions; submitEvents has 4 branches pinned by the test matrix.

Helpers WriteEnableTracking / WriteFinishTracking on Player. submitEvents
preserves the TS quirk of submitting only recordedBlobs[0] even when
multiple blobs were recorded. Kick branch flags requestIdleLogout=true;
addSessionLog deferred via NAI-73-D-INPUT-NO-SESSION-LOG-KICK."
```

---

## Task 4: EVENT_TRACKING handler

**Files:**
- Create: `modules/world/handler_event_tracking.go`
- Create: `modules/world/handler_event_tracking_test.go`
- Modify: `modules/world/handlers_game.go` — register `gameHandlers[81] = handleEventTracking` in `init()`

### Step 4.1: TDD — handler 5-gate matrix (failing test first)

- [ ] **Step 4.1.1: Read existing handler test pattern**

Run: `head -60 modules/world/handler_chatsetmode_test.go` — note the structure.

- [ ] **Step 4.1.2: Create handler_event_tracking_test.go**

```go
package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func eventTrackingTestSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	rec := installRecordingBridges(s)
	p.input = &InputTracking{player: p}
	return p, rec
}

// TestHandleEventTrackingLenZeroReturnsFalse: empty payloads are dropped
// without state mutation. TS EventTrackingHandler.ts:9-11.
func TestHandleEventTrackingLenZeroReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	if err := handleEventTracking(p, []byte{}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if p.input.hasSeenReport {
		t.Error("hasSeenReport: must remain false on empty payload")
	}
	if len(p.input.recordedBlobs) != 0 {
		t.Errorf("recordedBlobs: got %d, want 0", len(p.input.recordedBlobs))
	}
}

// TestHandleEventTrackingLenOver500ReturnsFalse: payloads >500 bytes
// are dropped. TS EventTrackingHandler.ts:9-11.
func TestHandleEventTrackingLenOver500ReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	payload := make([]byte, 501)
	if err := handleEventTracking(p, payload); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if p.input.hasSeenReport {
		t.Error("hasSeenReport: must remain false on oversize payload")
	}
}

// TestHandleEventTrackingNotActiveReturnsFalse: blobs received outside
// the active window are dropped. TS EventTrackingHandler.ts:12-14.
func TestHandleEventTrackingNotActiveReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	// Window starts in the future: not active at processIn currentTick.
	p.input.startTrackingAt = 1000
	p.input.endTrackingAt = 2000
	// IsActive() reads s.currentTick — which here is 0 (newTestServer default).
	if err := handleEventTracking(p, []byte{0xAA}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if p.input.hasSeenReport {
		t.Error("hasSeenReport: must remain false when !IsActive")
	}
}

// TestHandleEventTrackingActiveButNotShouldSubmitShortCircuit:
// active but submitInput+cfg.NodeSubmitInput both false → returns true
// after setting hasSeenReport=true, but DOES NOT call Record. TS
// EventTrackingHandler.ts:18-20.
func TestHandleEventTrackingActiveButNotShouldSubmitShortCircuit(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.submitInput = false
	p.client.server.cfg.NodeSubmitInput = false

	if err := handleEventTracking(p, []byte{0xAA}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if !p.input.hasSeenReport {
		t.Error("hasSeenReport: must be true after first valid blob")
	}
	if len(p.input.recordedBlobs) != 0 {
		t.Errorf("recordedBlobs: must remain empty (short-circuit), got %d", len(p.input.recordedBlobs))
	}
}

// TestHandleEventTrackingCapExceededReturnsFalse: when
// recordedBlobsSizeTotal > cfg.NodeLimitBytesPerTrackingSession, the
// handler returns false without recording. TS
// EventTrackingHandler.ts:21-25.
func TestHandleEventTrackingCapExceededReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.submitInput = true
	p.client.server.cfg.NodeLimitBytesPerTrackingSession = 100
	p.input.recordedBlobsSizeTotal = 101 // already over

	if err := handleEventTracking(p, []byte{0xAA}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	// hasSeenReport set first (TS line 16), THEN cap check (line 21);
	// both true cases here — hasSeenReport=true but Record skipped.
	if !p.input.hasSeenReport {
		t.Error("hasSeenReport: must be true (set before cap check)")
	}
	if len(p.input.recordedBlobs) != 0 {
		t.Errorf("recordedBlobs: must NOT grow on cap-exceeded, got %d", len(p.input.recordedBlobs))
	}
}

// TestHandleEventTrackingHappyPathRecords: active + shouldSubmit +
// under cap → Record called, hasSeenReport=true, returns true.
func TestHandleEventTrackingHappyPathRecords(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.submitInput = true
	p.client.server.cfg.NodeLimitBytesPerTrackingSession = 50000

	if err := handleEventTracking(p, []byte{0xAA, 0xBB, 0xCC}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if !p.input.hasSeenReport {
		t.Error("hasSeenReport: must be true on happy path")
	}
	if got := len(p.input.recordedBlobs); got != 1 {
		t.Errorf("recordedBlobs: got %d, want 1", got)
	}
	if got := p.input.recordedBlobsSizeTotal; got != 3 {
		t.Errorf("recordedBlobsSizeTotal: got %d, want 3", got)
	}
}
```

- [ ] **Step 4.1.3: Run failing test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleEventTracking -count=1`

Expected: FAIL — `handleEventTracking` undefined.

### Step 4.2: Implement handler

- [ ] **Step 4.2.1: Read TS handler one more time**

Reference: `network/game/client/handler/EventTrackingHandler.ts:7-28`. Branch order:
1. len ∈ (0, 500] gate (else return false)
2. IsActive gate (else return false)
3. hasSeenReport = true
4. !ShouldSubmitTrackingDetails → return true (no record)
5. cap gate (else return false)
6. Record(bytes); return true

- [ ] **Step 4.2.2: Create handler_event_tracking.go**

```go
package world

// handleEventTracking handles client opcode 81 (EVENT_TRACKING),
// payload size -2 (2-byte length prefix), category RESTRICTED_EVENT.
//
// Mirrors TS EventTrackingHandler.handle (EventTrackingHandler.ts:7-28).
// Branch order:
//  1. len ∈ (0, 500] gate
//  2. p.input.IsActive(currentTick) gate
//  3. p.input.hasSeenReport = true
//  4. !p.input.ShouldSubmitTrackingDetails() → return early (no record)
//  5. p.input.recordedBlobsSizeTotal > cap gate
//  6. p.input.Record(payload)
//
// All gates that "fail" return nil (TS returns false from the handler;
// goscape signature is `error` — nil means "handled, drop").
func handleEventTracking(p *Player, payload []byte) error {
	if p.input == nil {
		return nil
	}
	n := len(payload)
	if n == 0 || n > inputTrackingMaxBlobBytes {
		return nil
	}
	if p.client == nil || p.client.server == nil {
		return nil
	}
	currentTick := p.client.server.currentTick
	if !p.input.IsActive(currentTick) {
		return nil
	}
	p.input.hasSeenReport = true
	if !p.input.ShouldSubmitTrackingDetails() {
		return nil
	}
	if p.input.recordedBlobsSizeTotal > p.client.server.cfg.NodeLimitBytesPerTrackingSession {
		return nil
	}
	// Defensive copy: payload may alias the read buffer.
	cp := make([]byte, n)
	copy(cp, payload)
	p.input.Record(cp)
	return nil
}
```

- [ ] **Step 4.2.3: Run handler tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleEventTracking -count=1 -v`

Expected: PASS for all 6 cases.

### Step 4.3: Wire handler into gameHandlers[81]

- [ ] **Step 4.3.1: Read handlers_game.go init() block**

Run: `sed -n '15,40p' modules/world/handlers_game.go` — note where existing handlers register.

- [ ] **Step 4.3.2: Add registration**

Modify `modules/world/handlers_game.go`. In the `init()` function, add (in numeric-opcode order if the file is sorted; otherwise alongside similar RESTRICTED_EVENT handlers):

```go
	gameHandlers[81] = handleEventTracking // EVENT_TRACKING (NAI-73)
```

- [ ] **Step 4.3.3: Verify dispatch via existing wire test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestReadPacketEventTrackingTwoByteLenPrefix -count=1 -v`

Expected: PASS — the existing wire-level test now exercises the bound handler. (The test asserts the read path; the handler returns nil so the test outcome is unchanged.)

- [ ] **Step 4.3.4: Run full world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: PASS — no regressions.

- [ ] **Step 4.3.5: Commit**

```bash
git add modules/world/handler_event_tracking.go modules/world/handler_event_tracking_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T4 — EVENT_TRACKING handler port (opcode 81)

Line-by-line port of TS EventTrackingHandler.ts:7-28. Five-gate matrix:
len bounds, IsActive, hasSeenReport set, ShouldSubmitTrackingDetails
short-circuit, recordedBlobsSizeTotal cap. Wired into gameHandlers[81]."
```

---

## Task 5: Per-tick wiring

**Files:**
- Modify: `modules/world/player.go` — add `processInputTracking` method; call it from end of `processIn`

### Step 5.1: TDD — processIn calls input.OnCycle

- [ ] **Step 5.1.1: Add test to player_test.go (extension)**

Append to `modules/world/player_test.go`:

```go
// TestProcessInCallsInputTrackingOnCycle pins that the last line of
// Player.processIn dispatches to InputTracking.OnCycle (TS World.ts:646
// placement parity).
func TestProcessInCallsInputTrackingOnCycle(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.state = ClientStateGame
	p.input = &InputTracking{player: p}
	// Position the window so OnCycle should fire enable() this tick.
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.input.enabled = false

	p.processIn(0)

	if !p.input.enabled {
		t.Error("input.enabled: must be true after processIn → OnCycle → enable()")
	}
}
```

- [ ] **Step 5.1.2: Run failing test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessInCallsInputTrackingOnCycle -count=1`

Expected: FAIL — `processIn` doesn't yet call OnCycle.

### Step 5.2: Implement processInputTracking

- [ ] **Step 5.2.1: Add processInputTracking method**

Modify `modules/world/player.go`. After `processIn` (after line 730), add:

```go
// processInputTracking dispatches per-tick input-recording state-machine
// work. Mirrors TS Player.processInputTracking (Player.ts:1271-1273) →
// this.input.onCycle(). Called from the end of processIn, mirroring TS
// World.ts:646 placement (last step of the per-player iteration in the
// client-input phase).
//
// Nil-guards p.input because newly-logged-in players may transition to
// ClientStateGame before processLogins allocates their InputTracking.
func (p *Player) processInputTracking(currentTick int) {
	if p.input == nil {
		return
	}
	p.input.OnCycle(currentTick)
}
```

- [ ] **Step 5.2.2: Wire into processIn**

In `modules/world/player.go`, find the end of `processIn` (around line 729-730):

```go
	if readAny {
		p.lastResponse = currentTick // mirrors TS decodeIn() line 80
	}
}
```

Replace with:

```go
	if readAny {
		p.lastResponse = currentTick // mirrors TS decodeIn() line 80
	}

	// NAI-73: per-tick input-tracking dispatch. Mirrors TS World.ts:646
	// placement (last step of per-player client-input phase iteration).
	p.processInputTracking(currentTick)
}
```

- [ ] **Step 5.2.3: Run test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessInCallsInputTrackingOnCycle -count=1 -v`

Expected: PASS.

- [ ] **Step 5.2.4: Run full world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: PASS.

- [ ] **Step 5.2.5: Commit**

```bash
git add modules/world/player.go modules/world/player_test.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T5 — per-tick InputTracking.OnCycle dispatch

processIn calls processInputTracking → input.OnCycle as its last step.
Mirrors TS World.ts:646 placement (per-player client-input phase, after
packet read loop). Nil-guarded for the brief login → ClientStateGame
window before processLogins allocates the entity."
```

---

## Task 6: Init at login

**Files:**
- Modify: `modules/world/tick.go` — `processLogins` allocates `p.input = NewInputTracking(p, s.currentTick)` and defaults `p.session = "headless"`

### Step 6.1: TDD — processLogins allocates p.input

- [ ] **Step 6.1.1: Read processLogins to find the per-player init block**

Run: `sed -n '78,135p' modules/world/tick.go` — locate the `for _, p := range batch { ... }` body where existing per-player init runs (after `addPlayer`).

- [ ] **Step 6.1.2: Add test**

Append to `modules/world/tick_test.go` (or create if absent):

```go
// TestProcessLoginsAllocatesInputTracking pins that newly logged-in
// players have a non-nil InputTracking with a future-scheduled window.
func TestProcessLoginsAllocatesInputTracking(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	s.newPlayers = []*Player{p}
	s.currentTick = 1000

	s.processLogins()

	if p.input == nil {
		t.Fatal("p.input: must be non-nil after processLogins")
	}
	if p.input.player != p {
		t.Error("p.input.player back-pointer must equal p")
	}
	wantMin := 1000 + inputTrackingRate - inputTrackingJitterRange
	wantMax := 1000 + inputTrackingRate + inputTrackingJitterRange
	if p.input.startTrackingAt < wantMin || p.input.startTrackingAt > wantMax {
		t.Errorf("startTrackingAt: got %d, want in [%d, %d]", p.input.startTrackingAt, wantMin, wantMax)
	}
	if got, want := p.input.endTrackingAt, p.input.startTrackingAt+inputTrackingTime; got != want {
		t.Errorf("endTrackingAt: got %d, want %d (startTrackingAt + inputTrackingTime)", got, want)
	}
	if got, want := p.session, "headless"; got != want {
		t.Errorf("p.session default: got %q, want %q", got, want)
	}
}
```

- [ ] **Step 6.1.3: Run failing test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLoginsAllocatesInputTracking -count=1`

Expected: FAIL — `p.input` is nil.

### Step 6.2: Implement init

- [ ] **Step 6.2.1: Add init in processLogins**

Modify `modules/world/tick.go` `processLogins` body. After existing per-player init lines (e.g. after `p.varps = make([]int32, ...)` around line 102 or wherever the per-player block consolidates), add:

```go
		// NAI-73: allocate the InputTracking state machine. Defaults
		// session to "headless" until LOGIN-SERVER-BRIDGE-MOD ships a
		// real UUID assignment.
		p.input = NewInputTracking(p, s.currentTick)
		if p.session == "" {
			p.session = "headless"
		}
```

The `if p.session == ""` guard makes the assignment idempotent (login-server bridge can pre-set it; default fills in otherwise).

- [ ] **Step 6.2.2: Run test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLoginsAllocatesInputTracking -count=1 -v`

Expected: PASS.

- [ ] **Step 6.2.3: Run full world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: PASS.

- [ ] **Step 6.2.4: Commit**

```bash
git add modules/world/tick.go modules/world/tick_test.go
git commit --no-gpg-sign -m "feat(world): NAI-73 T6 — InputTracking init at login

processLogins allocates p.input via NewInputTracking and defaults
p.session to \"headless\" (mirrors TS Player.session default; real UUID
assignment owned by LOGIN-SERVER-BRIDGE-MOD)."
```

---

## Task 7: REPORT_ABUSE retroactive close + deviation tag retirement

**Files:**
- Modify: `modules/world/server.go` — add `LookupPlayerByUsername` if absent
- Modify: `modules/world/handler_reportabuse.go` — wire MACROING/BUG_ABUSE → submitInput=true; rewrite doc-comment
- Modify: `modules/world/handler_reportabuse_test.go` — add 3 cases (MACROING, BUG_ABUSE, OffensiveLanguage)
- Modify: `modules/world/bridges.go` — retire deviation comment from LoggerBridge interface
- Modify: `modules/world/bridges_test.go` — no change beyond T2

### Step 7.1: Add LookupPlayerByUsername helper

- [ ] **Step 7.1.1: Verify it doesn't exist**

Run: `grep -n "LookupPlayerByUsername\|LookupPlayerByName" modules/world/server.go`

Expected: no output.

- [ ] **Step 7.1.2: Add the helper**

Modify `modules/world/server.go`. After `LookupPlayerByUID` (around line 757-770), add:

```go
// LookupPlayerByUsername returns the logged-in player whose username
// field matches the argument exactly, or nil if none is active.
// Mirrors TS World.getPlayerByUsername (World.ts:1675-1689). Intended
// to be called from the tick goroutine (playerLoop is unguarded there).
//
// Match is case-sensitive on the goscape username field (which is set
// at login from the client-supplied display name). TS keys on
// username37 (base37-encoded) but the inputs to this lookup are
// already strings in goscape's call sites.
func (s *Server) LookupPlayerByUsername(name string) *Player {
	for _, p := range s.playerLoop {
		if p == nil || !p.active {
			continue
		}
		if p.username == name {
			return p
		}
	}
	return nil
}
```

### Step 7.2: TDD — MACROING flips offender's submitInput

- [ ] **Step 7.2.1: Read existing handler_reportabuse_test.go fixture**

Run: `head -80 modules/world/handler_reportabuse_test.go` to confirm the `reportAbuseSetup` and `reportAbusePayload` helpers from T0 are usable.

- [ ] **Step 7.2.2: Add test**

Append to `modules/world/handler_reportabuse_test.go`:

```go
// reportAbuseSetupWithOnlineOffender extends reportAbuseSetup by also
// adding an offender Player to the server's playerLoop with the given
// username. Returns reporter, offender, and recorder.
func reportAbuseSetupWithOnlineOffender(t *testing.T, offenderName string) (*Player, *Player, *recordingBridges) {
	t.Helper()
	reporter, rec := reportAbuseSetup(t)
	s := reporter.client.server
	offender, _ := newTestPlayer(t)
	offender.client.server = s
	offender.username = offenderName
	offender.active = true
	s.playerLoop = append(s.playerLoop, offender)
	return reporter, offender, rec
}

// TestHandleReportAbuseMacroingFlipsSubmitInput pins that reason=
// MACROING(6) on an online offender flips offender.submitInput=true.
// Mirrors TS World.notifyPlayerReport (World.ts:2298-2304).
func TestHandleReportAbuseMacroingFlipsSubmitInput(t *testing.T) {
	reporter, offender, _ := reportAbuseSetupWithOnlineOffender(t, "evilbob")
	if offender.submitInput {
		t.Fatal("preflight: offender.submitInput should start false")
	}
	payload := reportAbusePayload(util.ToBase37("evilbob"), ReportAbuseMacroing, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if !offender.submitInput {
		t.Error("offender.submitInput: must be true after MACROING report")
	}
}

// TestHandleReportAbuseBugAbuseFlipsSubmitInput pins the same for BUG_ABUSE.
func TestHandleReportAbuseBugAbuseFlipsSubmitInput(t *testing.T) {
	reporter, offender, _ := reportAbuseSetupWithOnlineOffender(t, "evilbob")
	payload := reportAbusePayload(util.ToBase37("evilbob"), ReportAbuseBugAbuse, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if !offender.submitInput {
		t.Error("offender.submitInput: must be true after BUG_ABUSE report")
	}
}

// TestHandleReportAbuseNonMacroingDoesNotFlipSubmitInput pins that
// other reasons (e.g. OffensiveLanguage=0) do NOT flip submitInput.
func TestHandleReportAbuseNonMacroingDoesNotFlipSubmitInput(t *testing.T) {
	reporter, offender, _ := reportAbuseSetupWithOnlineOffender(t, "evilbob")
	payload := reportAbusePayload(util.ToBase37("evilbob"), ReportAbuseOffensiveLanguage, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if offender.submitInput {
		t.Error("offender.submitInput: must remain false for non-MACROING/BUG_ABUSE reasons")
	}
}

// TestHandleReportAbuseMacroingOfflineOffenderNoOp pins that MACROING
// against an offline offender does not panic and does not affect any
// other state. (TS getPlayerByUsername returns undefined; the handler
// silently skips the submitInput flip.)
func TestHandleReportAbuseMacroingOfflineOffenderNoOp(t *testing.T) {
	reporter, _ := reportAbuseSetup(t) // no offender added
	payload := reportAbusePayload(util.ToBase37("ghost"), ReportAbuseMacroing, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	// No panic — done.
}
```

- [ ] **Step 7.2.3: Run failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleReportAbuse.*SubmitInput -count=1`

Expected: FAIL — handler doesn't yet flip submitInput.

### Step 7.3: Implement MACROING/BUG_ABUSE flip

- [ ] **Step 7.3.1: Modify handleReportAbuse**

Open `modules/world/handler_reportabuse.go`. Find the line just before `s.loggerBridge.NotifyPlayerReport(...)` (around line 61). Insert a new branch:

```go
	// NAI-73: MACROING/BUG_ABUSE → flip the offender's submitInput so
	// the next InputTracking window submits detailed events for offline
	// review. Mirrors TS World.notifyPlayerReport (World.ts:2298-2304).
	// Closes the NAI-72-D-INPUT-RECORDING-NOT-PORTED retroactive doc-
	// comment block (lines 23-25 below, retired in this commit).
	if reason == ReportAbuseMacroing || reason == ReportAbuseBugAbuse {
		if offenderPlayer := s.LookupPlayerByUsername(util.FromBase37(offender)); offenderPlayer != nil {
			offenderPlayer.submitInput = true
		}
	}

	s.loggerBridge.NotifyPlayerReport(p, util.FromBase37(offender), reasonLabel(reason))
```

- [ ] **Step 7.3.2: Rewrite the doc-comment block**

In the same file, replace lines 22-28 (the entire deviation block):

Find:
```go
// The MACROING/BUG_ABUSE submitInput=true branch (TS World.ts:2298-2304)
// is intentionally omitted — input-recording subsystem not ported
// (NAI-72-D-INPUT-RECORDING-NOT-PORTED).
//
// Friends/login/logger bridges all stubbed; see NAI-72-D-FRIENDS-SERVER-
// BRIDGE / NAI-72-D-LOGIN-SERVER-BRIDGE-MOD / NAI-72-D-LOGGER-BRIDGE.
```

Replace with:
```go
// MACROING/BUG_ABUSE flips offenderPlayer.submitInput = true so the
// next InputTracking window submits detailed events to the logger
// bridge. Mirrors TS World.notifyPlayerReport at World.ts:2298-2304.
// Closes NAI-73's retroactive REPORT_ABUSE polish.
//
// Friends and login bridges are stubbed (noopBridges); see
// NAI-72-D-FRIENDS-SERVER-BRIDGE and NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.
// Logger bridge ships the slogLoggerBridge default impl as of NAI-73.
```

- [ ] **Step 7.3.3: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleReportAbuse -count=1 -v`

Expected: PASS — the 4 new submitInput tests plus all existing tests.

### Step 7.4: Retire deviation tags from bridges.go interface comment

- [ ] **Step 7.4.1: Modify bridges.go LoggerBridge interface comment**

Verify the T2-edited comment block on `LoggerBridge` no longer mentions `NAI-72-D-LOGGER-BRIDGE` (it shouldn't — T2 already replaced it). Grep to confirm:

Run: `grep -n "NAI-72-D-LOGGER-BRIDGE" modules/world/bridges.go`

Expected: no output.

If any stale references survive, delete them.

- [ ] **Step 7.4.2: Search for any other lingering deviation tag references**

Run: `grep -rn "NAI-72-D-LOGGER-BRIDGE\|NAI-72-D-INPUT-RECORDING-NOT-PORTED" modules/ pkg/ docs/superpowers/specs/ 2>/dev/null`

Expected: only references in:
- `docs/superpowers/specs/2026-05-02-nai-72-social-subsystem-foundation-design.md` (historical record — leave)
- `docs/superpowers/specs/2026-05-02-nai-73-event-tracking-input-recording-design.md` (closure declaration — leave)
- `docs/superpowers/plans/2026-05-02-nai-72-social-subsystem-foundation.md` (historical record — leave)
- `docs/superpowers/plans/2026-05-02-nai-73-event-tracking-input-recording.md` (this file — leave)

Any reference outside `docs/` is a stale doc-comment that must be retired. For each, delete or rewrite per the local context.

### Step 7.5: Final full-suite run

- [ ] **Step 7.5.1: Run all world tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: PASS.

- [ ] **Step 7.5.2: Run all goscape tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`

Expected: PASS.

- [ ] **Step 7.5.3: Run with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1`

Expected: PASS.

### Step 7.6: Close commit

- [ ] **Step 7.6.1: Update memory entries**

After commit, update `MEMORY.md` and the appropriate per-deviation memory files:
- Retire `NAI-72-D-LOGGER-BRIDGE` and `NAI-72-D-INPUT-RECORDING-NOT-PORTED` per the close commit's `Closes memory:` trailer.
- Open `NAI-73-D-INPUT-NO-SESSION-LOG-KICK` per the `Opens memory:` trailer.
- Update `nai_followups.md` with the NAI-73 closure section.

(Memory updates are a post-merge step done by the controller, not the implementer; the close commit's trailers make them grep-discoverable.)

- [ ] **Step 7.6.2: Close commit**

```bash
git add modules/world/server.go modules/world/handler_reportabuse.go modules/world/handler_reportabuse_test.go modules/world/bridges.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-73 — EventTracking + InputTracking + LoggerBridge realisation

Closes NAI-72-D-LOGGER-BRIDGE: extended LoggerBridge interface with
SubmitInputTracking; shipped slogLoggerBridge real default impl bound
in NewServer (replacing noopBridges{} for the loggerBridge field).

Closes NAI-72-D-INPUT-RECORDING-NOT-PORTED: ported the full TS
InputTracking entity (200/150/16-tick scheduler with [-15,+15] jitter),
EVENT_TRACKING handler (op 81) wired into gameHandlers, per-tick
OnCycle dispatch from end of processIn, init at login.

Retroactive close: REPORT_ABUSE MACROING/BUG_ABUSE → offenderPlayer.
submitInput=true wired (TS World.ts:2298-2304); doc-comment retired.

Opens NAI-73-D-INPUT-NO-SESSION-LOG-KICK: kick branch ships
requestIdleLogout=true only; addSessionLog deferred to future
session-log NAI.

Net deviation tally 18 → 17 (−2 +1).

Closes memory: NAI-72-D-LOGGER-BRIDGE
Closes memory: NAI-72-D-INPUT-RECORDING-NOT-PORTED
Opens memory: NAI-73-D-INPUT-NO-SESSION-LOG-KICK

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Spec coverage checklist

Cross-foot against `docs/superpowers/specs/2026-05-02-nai-73-event-tracking-input-recording-design.md`:

| Spec § | Requirement | Task |
|:-:|---|:-:|
| §3.2 #1 | Extend `LoggerBridge` interface with `SubmitInputTracking` | T2 |
| §3.2 #2 | Add `slogLoggerBridge` real default; bind in `NewServer` | T2 |
| §3.2 #3 | Update `noopBridges.SubmitInputTracking` for interface satisfaction | T2 |
| §3.2 #4 | Add `Player.input *InputTracking` field + init in `processLogins` | T1.1 + T6 |
| §3.2 #5 | Add `Player.submitInput bool` field | T1.1 |
| §3.2 #6 | Add `Player.session string` field default `"headless"` | T1.1 + T6 |
| §3.2 #7 | Add `cfg.NodeLimitBytesPerTrackingSession` | T1.2 |
| §3.2 #8 | Add server-prot opcodes 226/133 + tests | T1.3 |
| §3.2 #9 | `WriteEnableTracking` / `WriteFinishTracking` helpers | T3.1 |
| §3.2 #10 | `InputTracking` entity full state-machine port + TDD | T3.2-T3.10 |
| §3.2 #11 | EVENT_TRACKING handler + register in `gameHandlers[81]` | T4 |
| §3.2 #12 | `processInputTracking` + call from end of `processIn` | T5 |
| §3.2 #13 | `loggerBridge.SubmitInputTracking` from `submitEvents` | T3.2 (impl in submitEvents) |
| §3.2 #14 | REPORT_ABUSE retroactive: MACROING/BUG_ABUSE → submitInput=true | T7.3 |
| §3.2 #15 | Retire deviation tags `NAI-72-D-LOGGER-BRIDGE` + `NAI-72-D-INPUT-RECORDING-NOT-PORTED` | T2 + T7.4 |
| §5.1 | InputTracking state-machine tests | T3.3-T3.9 |
| §5.2 | EVENT_TRACKING handler 5-gate matrix | T4.1 |
| §5.3 | LoggerBridge slog-buffer tests | T2.3 |
| §5.4 | REPORT_ABUSE retroactive tests (MACROING/BUG_ABUSE/non-) | T7.2 |
| §5.5 | `NodeLimitBytesPerTrackingSession` default test | T1.2 |
| §5.6 | Server-prot opcode wiring tests | T1.3 |
| §5.7 | Per-tick wiring test | T5.1 |

All spec requirements covered.

---

## Risk register (re-stated from spec §8 with mitigations)

| Risk | Mitigation in plan |
|---|---|
| `LookupPlayerByUsername` helper missing | T7.1 adds it (modeled on `LookupPlayerByUID`). |
| `packCoord(p)` helper status | Verified at HEAD: `pkg/coordgrid.PackCoord(level, x, z int) int`. T2.3 uses it. |
| `submitEvents` TS quirk: only blob[0] submitted | T3.2 `submitEvents` impl preserves; T3.8 test pins explicitly with 3-blob fixture. |
| Per-player RNG init pattern | Verified: package-level `math/rand/v2.IntN`. T3.2 `offset` uses it directly; no per-player RNG plumbing needed. |
| `processInputTracking` placement | T5 spec explicitly: end of `processIn`. |
| `slogLoggerBridge` swap regressions | T2.4.3 runs full test suite; if regressions surface, rebind via `installRecordingBridges` in affected tests. |
| `cfg.NodeDebug` default `true` masks kick branch | T3.8 test cases set `cfg.NodeDebug = false` explicitly. |

---

## Self-review notes

- Type consistency: `*InputTracking` referenced in T1.1 (stub) → T3.2 (real). Method signatures stable across tasks.
- `WriteEnableTracking` / `WriteFinishTracking`: defined T3.1, called T3.2 (`enable`/`disable`). Names consistent.
- `LookupPlayerByUsername`: defined T7.1, called T7.3. Consistent.
- All test helpers (`inputTrackingTestSetup`, `eventTrackingTestSetup`, `reportAbuseSetupWithOnlineOffender`, `drainClientOut`) are defined in the test file where first used; downstream tests reuse them by import.
