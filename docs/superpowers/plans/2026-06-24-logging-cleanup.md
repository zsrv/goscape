# Logging Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a named `trace` level below `debug`, per-module level overrides, a `component=` tag on every server log line, and sweep all call sites against a written level contract — without touching the CLI tooling loggers.

**Architecture:** A config-only `log.Level` type teaches YAML/flags the word `trace`; it is converted to `slog.Level` at the logger-construction boundary, so `NewLogger` and every logger field stay `slog.Level`. A `ReplaceAttr` hook inside `NewLogger` renders `TRACE` and trims `source` to `file.go:line`. Component attribution is delivered by deriving single-`component=` child loggers from the raw module logger at each subsystem seam. Then every call site is re-leveled against the contract.

**Tech Stack:** Go 1.26+, `log/slog`, `go.yaml.in/yaml/v2` (honors `encoding.TextUnmarshaler` — verified), standard `flag` `TextVar`.

**Spec:** `docs/superpowers/specs/2026-06-24-logging-cleanup-design.md`

## Global Constraints

- Go commands MUST be prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- Commits MUST use `git commit --no-gpg-sign` and end with the trailer `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Build flags mirror the project: `CGO_ENABLED=0 go build -trimpath ...`; race tests need `CGO_ENABLED=1`.
- Follow the use-modern-go skill for syntax.
- **Do not modify `cmd/goscape-cli/*` loggers** — `NewLogger` keeps its `slog.Level` signature precisely so they remain untouched.
- `log.Level` is a **config-only** type. Loggers, handlers, and `NewLogger` use `slog.Level`. Convert with `slog.Level(x)` / `log.Level(x)` only at the config boundary (`config.go`, `main.go`, `modules.go`).
- One `component=` attr per line, lowercase dotted `<module>.<subsystem>`. **Never stamp `component` twice** — derive children from the raw (un-stamped) logger.
- The level contract (every call site is measured against it):

  | Level | Meaning | Frequency |
  |---|---|---|
  | `error` | operation failed, needs operator attention, unrecoverable here | rare |
  | `warn` | unexpected but handled — degraded mode, client misbehavior | uncommon |
  | `info` | significant lifecycle / operational milestones | **never per-conn/packet/tick** |
  | `debug` | per-connection lifecycle, handshake steps, diagnostics | per-conn OK |
  | `trace` | raw byte dumps, per-packet, per-tick firehose | unbounded OK |

- Component values (declared in `modules/world/logging.go`, Task 5): `world`, `world.server`, `world.net`, `world.tick`, `world.script`, `world.friends`, `world.login`, `world.content`; single-component modules: `login`, `friends`, `ondemand`.

---

## Task 1: `log.Level` type with `trace`

**Files:**
- Create: `pkg/util/log/level.go`
- Test: `pkg/util/log/level_test.go`

**Interfaces:**
- Produces: `const log.LevelTrace = slog.LevelDebug - 4` (an `slog.Level`, value -8); `type log.Level slog.Level` implementing `encoding.TextMarshaler`/`encoding.TextUnmarshaler` + `fmt.Stringer`; `func log.Trace(l *slog.Logger, msg string, args ...any)`.

- [ ] **Step 1: Write the failing test**

Create `pkg/util/log/level_test.go`:

```go
package log_test

import (
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/util/log"
)

func TestLevelUnmarshalText(t *testing.T) {
	cases := map[string]slog.Level{
		"trace": log.LevelTrace,
		"TRACE": log.LevelTrace,
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for in, want := range cases {
		var l log.Level
		if err := l.UnmarshalText([]byte(in)); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", in, err)
		}
		if slog.Level(l) != want {
			t.Errorf("UnmarshalText(%q) = %v, want %v", in, slog.Level(l), want)
		}
	}
}

func TestLevelUnmarshalInvalid(t *testing.T) {
	var l log.Level
	if err := l.UnmarshalText([]byte("loud")); err == nil {
		t.Fatal("expected error for invalid level, got nil")
	}
}

func TestLevelMarshalRoundTrip(t *testing.T) {
	for _, lv := range []log.Level{log.Level(log.LevelTrace), log.Level(slog.LevelInfo), log.Level(slog.LevelError)} {
		b, err := lv.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%v): %v", slog.Level(lv), err)
		}
		var got log.Level
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", b, err)
		}
		if got != lv {
			t.Errorf("round trip %v -> %q -> %v", slog.Level(lv), b, slog.Level(got))
		}
	}
}

func TestLevelTraceMarshalsAsTRACE(t *testing.T) {
	b, err := log.Level(log.LevelTrace).MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "TRACE" {
		t.Errorf("MarshalText = %q, want TRACE", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/util/log/ -run TestLevel -v`
Expected: FAIL — `undefined: log.Level` / `undefined: log.LevelTrace`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/util/log/level.go`:

```go
package log

import (
	"context"
	"log/slog"
	"strings"
)

// LevelTrace is one step below slog.LevelDebug — the firehose level for
// per-packet / per-tick output. slog has no built-in trace level, so we
// define one. It is an ordinary slog.Level value (-8); handlers and the
// Trace helper use it directly.
const LevelTrace = slog.LevelDebug - 4

// Level is a config-only wrapper around slog.Level that additionally
// understands the name "trace". Config structs use Level so YAML and
// flags accept "trace"; it is converted back to slog.Level at the
// logger-construction boundary (NewLogger keeps an slog.Level signature).
type Level slog.Level

// UnmarshalText parses "trace" (case-insensitive) as LevelTrace and
// delegates everything else to slog.Level (debug/info/warn/error plus
// their +/-N offset forms).
func (l *Level) UnmarshalText(b []byte) error {
	if strings.EqualFold(strings.TrimSpace(string(b)), "trace") {
		*l = Level(LevelTrace)
		return nil
	}
	var sl slog.Level
	if err := sl.UnmarshalText(b); err != nil {
		return err
	}
	*l = Level(sl)
	return nil
}

// MarshalText renders LevelTrace as "TRACE" and delegates otherwise.
func (l Level) MarshalText() ([]byte, error) {
	if slog.Level(l) == LevelTrace {
		return []byte("TRACE"), nil
	}
	return slog.Level(l).MarshalText()
}

// String renders LevelTrace as "TRACE" and delegates otherwise.
func (l Level) String() string {
	if slog.Level(l) == LevelTrace {
		return "TRACE"
	}
	return slog.Level(l).String()
}

// Trace logs at LevelTrace. slog.Logger has no Trace method; this helper
// fills the gap for the handful of firehose call sites.
func Trace(l *slog.Logger, msg string, args ...any) {
	l.Log(context.Background(), LevelTrace, msg, args...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/util/log/ -run TestLevel -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/util/log/level.go pkg/util/log/level_test.go
git commit --no-gpg-sign -m "feat(logging): add trace level + config-only Level type

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: `NewLogger` renders `TRACE` + trims source

**Files:**
- Modify: `pkg/util/log/log.go`
- Test: `pkg/util/log/log_test.go` (create)

**Interfaces:**
- Consumes: `log.LevelTrace`, `log.Trace` (Task 1).
- Produces: unchanged `NewLogger(level slog.Level, format string, w io.Writer) (*slog.Logger, error)` signature; handlers now render `TRACE` and `source=file.go:line`.

- [ ] **Step 1: Write the failing test**

Create `pkg/util/log/log_test.go`:

```go
package log_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/util/log"
)

func TestNewLoggerRendersTrace(t *testing.T) {
	var buf bytes.Buffer
	logger, err := log.NewLogger(log.LevelTrace, "text", &buf)
	if err != nil {
		t.Fatal(err)
	}
	log.Trace(logger, "firehose")
	out := buf.String()
	if !strings.Contains(out, "level=TRACE") {
		t.Errorf("want level=TRACE in %q", out)
	}
	if !strings.Contains(out, "msg=firehose") {
		t.Errorf("want msg=firehose in %q", out)
	}
}

func TestTraceHiddenAtDebug(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := log.NewLogger(slog.LevelDebug, "text", &buf)
	log.Trace(logger, "should not appear")
	if buf.Len() != 0 {
		t.Errorf("trace record emitted at debug level: %q", buf.String())
	}
}

func TestSourceTrimmedToBase(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := log.NewLogger(slog.LevelInfo, "text", &buf)
	logger.Info("hi")
	out := buf.String()
	if !strings.Contains(out, "source=log_test.go:") {
		t.Errorf("want source=log_test.go:<line> in %q", out)
	}
	if strings.Contains(out, "source=/") {
		t.Errorf("source should be trimmed, not absolute: %q", out)
	}
}

func TestInvalidFormatErrors(t *testing.T) {
	if _, err := log.NewLogger(slog.LevelInfo, "xml", &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for invalid format")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/util/log/ -run 'TestNewLogger|TestTrace|TestSource' -v`
Expected: FAIL — `TestNewLoggerRendersTrace` finds `level=DEBUG-4` not `level=TRACE`; `TestSourceTrimmedToBase` finds an absolute path.

- [ ] **Step 3: Write minimal implementation**

Replace the whole body of `pkg/util/log/log.go` with:

```go
package log

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
)

// NewLogger creates a slog.Logger writing to w in the given format.
// format is "text" (key-value) or "json".
func NewLogger(level slog.Level, format string, w io.Writer) (*slog.Logger, error) {
	switch strings.ToLower(format) {
	case "text":
		return NewStdLogger(level, w), nil
	case "json":
		return NewStructuredLogger(level, w), nil
	default:
		return nil, fmt.Errorf("invalid log format: %s", format)
	}
}

// NewStdLogger creates a logger that logs messages in key-value format.
func NewStdLogger(level slog.Level, w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, handlerOptions(level)))
}

// NewStructuredLogger creates a logger that logs messages in JSON format.
func NewStructuredLogger(level slog.Level, w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, handlerOptions(level)))
}

// handlerOptions builds the shared slog.HandlerOptions: AddSource on, the
// given minimum level, and a ReplaceAttr that (1) renders LevelTrace as
// "TRACE" (slog would otherwise print "DEBUG-4") and (2) trims the source
// to "file.go:line" (the default prints the absolute compile path).
func handlerOptions(level slog.Level) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == LevelTrace {
					a.Value = slog.StringValue("TRACE")
				}
			case slog.SourceKey:
				if src, ok := a.Value.Any().(*slog.Source); ok && src != nil {
					a.Value = slog.StringValue(filepath.Base(src.File) + ":" + strconv.Itoa(src.Line))
				}
			}
			return a
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/util/log/ -v`
Expected: PASS (Task 1 + Task 2 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/util/log/log.go pkg/util/log/log_test.go
git commit --no-gpg-sign -m "feat(logging): render TRACE level + trim source to file:line

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Global config accepts `trace`; remove the boot config dump

**Files:**
- Modify: `cmd/goscape/app/config.go` (import block; field at line 16; flag at line 37)
- Modify: `cmd/goscape/main.go` (line 25 conversion; remove line 42)
- Test: `cmd/goscape/app/config_logging_test.go` (create)

**Interfaces:**
- Consumes: `log.Level`, `log.LevelTrace` (Task 1).
- Produces: `Config.LogLevel` is now `log.Level`.

- [ ] **Step 1: Write the failing test**

Create `cmd/goscape/app/config_logging_test.go`:

```go
package app_test

import (
	"log/slog"
	"testing"

	yaml "go.yaml.in/yaml/v2"

	"github.com/zsrv/goscape/cmd/goscape/app"
	"github.com/zsrv/goscape/pkg/util/log"
)

func TestGlobalLogLevelParsesTrace(t *testing.T) {
	var c app.Config
	if err := yaml.Unmarshal([]byte("log_level: trace\n"), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if slog.Level(c.LogLevel) != log.LevelTrace {
		t.Errorf("LogLevel = %v, want trace(-8)", slog.Level(c.LogLevel))
	}
}

func TestDefaultLogLevelIsInfo(t *testing.T) {
	c := app.NewDefaultConfig()
	if slog.Level(c.LogLevel) != slog.LevelInfo {
		t.Errorf("default LogLevel = %v, want INFO", slog.Level(c.LogLevel))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/app/ -run TestGlobalLogLevel -v`
Expected: FAIL to compile — `slog.Level(c.LogLevel)` where `c.LogLevel` is already `slog.Level` is a no-op conversion, but `log.LevelTrace` comparison and the trace parse will fail (current `slog.Level` rejects `"trace"` → unmarshal error).

- [ ] **Step 3: Write minimal implementation**

In `cmd/goscape/app/config.go`, add the import (keep `log/slog`):

```go
import (
	"flag"
	"log/slog"

	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/ondemand"
	"github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/util/log"
)
```

Change the field (line 16):

```go
	LogLevel  log.Level  `yaml:"log_level,omitempty"` // global log level, default for modules too
```

Change the flag registration (line 37):

```go
	f.TextVar(&c.LogLevel, "log.level", log.Level(slog.LevelInfo), "Only log messages with the given severity or above. Valid levels: [trace, debug, info, warn, error]")
```

In `cmd/goscape/main.go`, change line 25:

```go
	logger, err := log.NewLogger(slog.Level(config.LogLevel), config.LogFormat, os.Stdout)
```

And **delete** line 42 entirely:

```go
	fmt.Printf("%+v\n", config) // DEBUG
```

(`fmt` is still used by the `Fprintf` calls in `main.go`, so leave the import.)

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/... -run 'TestGlobalLogLevel|TestDefaultLogLevel' -v`
Expected: PASS.
Then build: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./cmd/goscape`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape/app/config.go cmd/goscape/app/config_logging_test.go cmd/goscape/main.go
git commit --no-gpg-sign -m "feat(logging): global log_level accepts trace; drop boot config dump

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Per-module overrides + module `component=` stamping

**Files:**
- Modify: `modules/world/config.go` (import swap; field at line 15)
- Modify: `modules/login/config.go` (import; new field)
- Modify: `modules/friends/config.go` (import; new field)
- Modify: `cmd/goscape/app/modules.go` (4 init blocks)
- Test: `cmd/goscape/app/config_logging_test.go` (extend)

**Interfaces:**
- Consumes: `log.Level` (Task 1).
- Produces: `world.Config.LogLevel`, `login.Config.LogLevel`, `friends.Config.LogLevel` are all `*log.Level`. Each module's runtime logger carries `component=<module>` (world: raw, stamped internally in Task 5).

- [ ] **Step 1: Write the failing test**

Append to `cmd/goscape/app/config_logging_test.go`:

```go
func TestModuleLogLevelOverridesParse(t *testing.T) {
	var c app.Config
	doc := "world:\n  log_level: trace\nlogin:\n  log_level: warn\nfriends:\n  log_level: error\n"
	if err := yaml.Unmarshal([]byte(doc), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.World.LogLevel == nil || slog.Level(*c.World.LogLevel) != log.LevelTrace {
		t.Errorf("world override = %v, want trace", c.World.LogLevel)
	}
	if c.Login.LogLevel == nil || slog.Level(*c.Login.LogLevel) != slog.LevelWarn {
		t.Errorf("login override = %v, want warn", c.Login.LogLevel)
	}
	if c.Friends.LogLevel == nil || slog.Level(*c.Friends.LogLevel) != slog.LevelError {
		t.Errorf("friends override = %v, want error", c.Friends.LogLevel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/app/ -run TestModuleLogLevelOverridesParse -v`
Expected: FAIL to compile — `c.Login.LogLevel` / `c.Friends.LogLevel` undefined; `c.World.LogLevel` is `*slog.Level` so `log.LevelTrace` parse fails.

- [ ] **Step 3: Write minimal implementation**

In `modules/world/config.go`, swap the unused `log/slog` import for `pkg/util/log`:

```go
import (
	"flag"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/io/protocol"
	"github.com/zsrv/goscape/pkg/util/log"
)
```

Change the field (line 15):

```go
	LogLevel          *log.Level    `yaml:"log_level"`
```

In `modules/login/config.go`, add the import and a field. New import block:

```go
import (
	"flag"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/zsrv/goscape/pkg/util/log"
)
```

Add as the first field of the `Config` struct (after `type Config struct {`):

```go
	LogLevel *log.Level `yaml:"log_level"` // optional per-module override; nil = inherit global
```

In `modules/friends/config.go`, add the import:

```go
import (
	"flag"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/util/log"
)
```

Add as the first field of the `Config` struct:

```go
	LogLevel *log.Level `yaml:"log_level"` // optional per-module override; nil = inherit global
```

In `cmd/goscape/app/modules.go`, update the **ondemand** block (lines 41-52) — convert global to `slog.Level` and stamp the component:

```go
	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.OnDemand.Server.LogLevel != nil {
		logLevel = *g.cfg.OnDemand.Server.LogLevel
	}

	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout)
	if err != nil {
		g.logger.Error("failed to create logger", "module", "ondemand", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "ondemand")

	g.cfg.OnDemand.Server.Log = logger
```

Update the **login** block (lines 99-106):

```go
	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Login.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.Login.LogLevel)
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout)
	if err != nil {
		g.logger.Error("failed to create logger", "module", "login", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "login")

	l, err := login.New(g.cfg.Login, logger)
```

Update the **friends** block (lines 121-128):

```go
	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Friends.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.Friends.LogLevel)
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout)
	if err != nil {
		g.logger.Error("failed to create logger", "module", "friends", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "friends")

	f, err := friends.New(g.cfg.Friends, logger)
```

Update the **world** block (lines 143-148) — convert types; **do NOT stamp component** (world stamps its own finer components in Task 5):

```go
	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.World.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.World.LogLevel)
	}

	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout)
	if err != nil {
		g.logger.Error("failed to create logger", "module", "world", "err", err)
		os.Exit(1)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/app/ -run TestModuleLogLevelOverridesParse -v`
Expected: PASS.
Then: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Expected: builds clean (catches any remaining `slog.Level` vs `log.Level` mismatch in `modules.go`).

- [ ] **Step 5: Commit**

```bash
git add modules/world/config.go modules/login/config.go modules/friends/config.go cmd/goscape/app/modules.go cmd/goscape/app/config_logging_test.go
git commit --no-gpg-sign -m "feat(logging): per-module level overrides + component= per module

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: World component child loggers

**Files:**
- Create: `modules/world/logging.go` (component constants)
- Modify: `modules/world/world.go` (`New`: lines 28-78)
- Modify: `modules/world/server.go` (`Server` struct fields; `NewServer` logger derivation ~line 433; bridge construction lines 459-474; `newClient` call line 950)
- Modify: `modules/world/logger_bridge.go` (line 27 component value)

**Interfaces:**
- Consumes: the raw `*slog.Logger` passed into `world.New` (Task 4 passes it un-stamped).
- Produces: `Server.log` = `world.server`; `Server.logNet` = `world.net`; `Server.logTick` = `world.tick`; `Server.logScript` = `world.script`; `Server.logContent` = `world.content`. `World.log` = `world`. Clients receive `world.net`. Bridges receive `world.friends`/`world.login`.

- [ ] **Step 1: Write the failing test**

Create `modules/world/logging_test.go`:

```go
package world

import "testing"

func TestComponentConstantsAreDistinct(t *testing.T) {
	all := []string{compWorld, compServer, compNet, compTick, compScript, compFriends, compLogin, compContent}
	seen := map[string]bool{}
	for _, c := range all {
		if c == "" {
			t.Error("empty component constant")
		}
		if seen[c] {
			t.Errorf("duplicate component %q", c)
		}
		seen[c] = true
	}
	if compWorld != "world" || compNet != "world.net" {
		t.Errorf("unexpected component naming: %q %q", compWorld, compNet)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentConstants -v`
Expected: FAIL — `undefined: compWorld` etc.

- [ ] **Step 3: Write minimal implementation**

Create `modules/world/logging.go`:

```go
package world

// Component attribute values for this module's loggers. Every world log
// line carries exactly one component= attr; the dotted prefix encodes the
// module. Children are derived from the RAW (un-stamped) module logger so
// a line never carries two component= attrs.
const (
	compWorld   = "world"         // module-level / fallback
	compServer  = "world.server"  // server lifecycle (listen/load/start/stop)
	compNet     = "world.net"     // per-connection packet I/O
	compTick    = "world.tick"    // per-tick processing
	compScript  = "world.script"  // RuneScript engine
	compFriends = "world.friends" // friends-server RPC
	compLogin   = "world.login"   // login-server RPC
	compContent = "world.content" // content watcher / hot reload
	compReport  = "world.report"  // player report / session log / input track
)
```

In `modules/world/world.go`, stamp the base component and derive client children. Change the `New` body so:
- `w.log` becomes the base `world` logger;
- the login/friends clients get their own components;
- the failure warnings use `w.log`;
- the raw `logger` is passed to `NewServer`.

```go
	w := &World{
		cfg: cfg,
		log: logger.With("component", compWorld),
	}
```

```go
	var loginClient LoginClient
	if cfg.LoginServerEnabled {
		lc, err := NewLoginClient(cfg.LoginServerAddress, logger.With("component", compLogin))
		if err != nil {
			// Log the error but don't fail startup — the world should run even if login is unreachable.
			w.log.Warn("failed to create login client", slog.Any("err", err))
		} else {
			loginClient = lc
		}
	}
	w.loginClient = loginClient

	var friendsClient FriendsClient
	if cfg.FriendsServerEnabled {
		fc, err := NewFriendsClient(cfg.FriendsServerAddress, logger.With("component", compFriends))
		if err != nil {
			w.log.Warn("failed to create friends client", slog.Any("err", err))
		} else {
			friendsClient = fc
		}
	}
	w.friendsClient = friendsClient

	server, err := NewServer(cfg, loginClient, friendsClient, logger, tap)
```

(The `signals.NewHandler(logger)` call at line 51 keeps the raw logger — it is a generic signal handler, not a world subsystem.)

In `modules/world/server.go`, add the child-logger fields to the `Server` struct, next to the existing `log` field (line 63):

```go
	log        *slog.Logger // component=world.server (server lifecycle)
	logNet     *slog.Logger // component=world.net (per-connection I/O)
	logTick    *slog.Logger // component=world.tick
	logScript  *slog.Logger // component=world.script
	logContent *slog.Logger // component=world.content
```

In `NewServer`, change the struct-literal `log:` field (line 433) and derive the children immediately after the literal is assigned to `s`. The literal becomes:

```go
		log:              logger.With("component", compServer),
```

Immediately after `s := &Server{...}` (after the closing `}` of the literal), add:

```go
	s.logNet = logger.With("component", compNet)
	s.logTick = logger.With("component", compTick)
	s.logScript = logger.With("component", compScript)
	s.logContent = logger.With("component", compContent)
```

Change the bridge constructions (lines 459-474) to pass component children derived from the raw `logger` (not `s.log`):

```go
	s.friendsBridge = defaultFriendsBridge(friendsClient, int32(cfg.NodeID), cfg.NodeProfile, s.bridgesCtx, logger.With("component", compFriends))
	s.friendsDispatcher = newEmitFriendsDispatcher(s, logger.With("component", compFriends))
	s.friendsAdminBridge = defaultFriendsAdminBridge(friendsClient, cfg.NodeProfile, logger.With("component", compFriends))

	innerSlog := newSlogWorldEventsDispatcher(logger.With("component", compFriends))
```

```go
		sub := newWorldEventsSubscriber(friendsClient, int32(cfg.NodeID), cfg.NodeProfile, s.worldEventsDispatcher, logger.With("component", compFriends))
```

```go
	s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.bridgesCtx, logger.With("component", compLogin))
	s.loggerBridge = NewSlogLoggerBridge(logger, s.cfg.NodeID, s.cfg.NodeProfile)
```

Change the `newClient` call (line 950) to pass the net logger:

```go
		c := newClient(conn, s.cfg.TCPServerWriteTimeout, s.logNet)
```

In `modules/world/logger_bridge.go`, change the component value (line 27) from `logger_bridge` to the world scheme:

```go
		log:     parent.With("component", compReport),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentConstants -v`
Expected: PASS.
Then build + full world suite: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: builds clean; existing world tests still pass (logger field rename did not break behavior).

- [ ] **Step 5: Commit**

```bash
git add modules/world/logging.go modules/world/logging_test.go modules/world/world.go modules/world/server.go modules/world/logger_bridge.go
git commit --no-gpg-sign -m "feat(logging): derive component= child loggers at world seams

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Call-site level-contract sweep

**Files:**
- Modify: `modules/world/server.go` (known fixes + net/server reclassification)
- Modify: `modules/world/client.go` (`world.net` firehose lines)
- Modify: `modules/world/tick.go`, `modules/world/script.go`, `modules/world/npc_script.go` (`s.log` → `s.logTick` / `s.logScript`)
- Modify: `modules/world/content_watcher.go`, `modules/world/reload.go` (`world.content`)
- Modify: any other `modules/world/*.go` flagged by the audit grep below

**Interfaces:**
- Consumes: `log.Trace` (Task 1); `s.logNet`/`s.logTick`/`s.logScript`/`s.logContent`, `c.log` = `world.net` (Task 5).
- Produces: every call site conforms to the level contract; no remaining per-packet/per-tick lines above `trace`.

This task is **contract-driven**, not line-by-line prescriptive: each `modules/world` call site is read and re-leveled per the contract table in Global Constraints. The concrete known fixes below are mandatory; the remaining sweep applies the same rules.

- [ ] **Step 1: Add the `log` import to the firehose files**

`server.go` and `client.go` will call `log.Trace`. They are `package world`; import `pkg/util/log` aliased to avoid confusion with the many `.log` fields:

```go
	applog "github.com/zsrv/goscape/pkg/util/log"
```

(Use `applog.Trace(...)` at call sites. Run goimports/build to confirm placement.)

- [ ] **Step 2: Apply the mandatory known fixes**

`modules/world/server.go`:

```go
// line ~1033 — per-packet, was Info
applog.Trace(c.log, "received data", "num_bytes", len(msg))
```

```go
// line ~1034 — raw bytes per packet, was Debug
applog.Trace(c.log, "received data payload", "data", fmt.Sprintf("%v", msg))
```

```go
// line ~1095 — unexpected condition, was Info
c.log.Warn("unhandled client state", "state", c.state)
```

```go
// line ~1182 — per partial read, was Info
applog.Trace(c.log, "partial packet data received, waiting for more", "opcode", loginreq.OpReqInitGameConnection, "length", pLen)
```

```go
// line ~990 — per-connection, was s.log.Info; net component + debug
s.logNet.Debug("connection closed", "remote_addr", conn.RemoteAddr())
```

`modules/world/client.go`:

```go
// line ~157 — raw bytes per outgoing packet, was Debug
applog.Trace(c.log, "sent data", "opcode", c.opcode, "num_bytes", len(data), "data", fmt.Sprintf("%v", data))
```

- [ ] **Step 3: Sweep the remaining sites by file**

Run the audit to enumerate every world call site with its current level:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache true # (no go needed)
grep -rEn '\b(s|c|b)\.log(Net|Tick|Script|Content)?\.(Trace|Debug|Info|Warn|Error)\(' modules/world/*.go | grep -v _test.go
```

For each line, apply the contract:
1. **Pick the component logger** for the file's subsystem: `tick.go` → `s.logTick`; `script.go`/`npc_script.go` → `s.logScript`; `content_watcher.go`/`reload.go` → `s.logContent`; per-connection paths → `c.log`/`s.logNet`; everything else stays `s.log` (`world.server`). Replace the receiver accordingly (e.g. `s.log.Warn(` → `s.logTick.Warn(` inside `tick.go`).
2. **Re-level** against the table: anything firing per-tick or per-packet → `applog.Trace`; unexpected-but-handled → `Warn`; per-connection chatter → `Debug`; true milestones stay `Info`.
3. **Coverage gaps:** scan for error returns that log nothing —

```bash
grep -rEn 'return .*err' modules/world/*.go | grep -v _test.go
```

   Where a terminal error is swallowed with no log, add `s.log<Comp>.Warn("…", "err", err)` (or `Error` if unrecoverable) per the contract. If a re-level is genuinely ambiguous, leave it and list it in the commit body for review rather than guessing.

- [ ] **Step 4: Verify build + suite + no stray firehose at info**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...
```

Expected: build clean; world suite passes; race clean.
Then confirm no per-packet/per-tick line remains above trace:

```bash
grep -rEn '\.(Info|Debug)\(' modules/world/server.go modules/world/client.go modules/world/tick.go | grep -iE 'received|sent data|payload|per.tick|partial packet'
```

Expected: no matches (all such lines are now `applog.Trace`).

- [ ] **Step 5: Commit**

```bash
git add modules/world/
git commit --no-gpg-sign -m "refactor(logging): sweep world call sites to the level contract

- per-packet/per-tick firehose -> trace
- unhandled client state -> warn; per-conn close -> debug (world.net)
- route subsystem lines to component child loggers

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Document config + full verification

**Files:**
- Modify: `examples/full-config-reference.yaml` (top-level `log_level` valid values; per-module `log_level` comments at lines 156/324; add login/friends override comments)
- Modify: `examples/bundled/goscape.yaml` (only if it sets `log_level`)

**Interfaces:** none (docs + verification).

- [ ] **Step 1: Update the full reference**

In `examples/full-config-reference.yaml`, change the top-level level comment/value to advertise `trace` (line ~54):

```yaml
# Valid levels: trace, debug, info, warn, error
log_level: info
```

Ensure the per-module override comments (lines ~156 and ~324, currently `#log_level: info`) document that login and friends also support `log_level` (YAML-only). Add a commented `log_level` under the `login:` and `friends:` sections mirroring the existing world/ondemand examples.

- [ ] **Step 2: Verify the example configs parse and validate**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o "$TMPDIR/goscape" ./cmd/goscape
"$TMPDIR/goscape" --config.file examples/full-config-reference.yaml --config.verify=true
```

Expected: exits 0 (config valid; strict-unmarshal accepts the new keys).

- [ ] **Step 3: Full test + race on touched packages**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/util/log/... ./cmd/goscape/... ./modules/world/...
```

Expected: all pass.

- [ ] **Step 4: Manual smoke (user-launched)**

Hand off to the user (per the `smoke_test_server_handoff` convention) to run:

```bash
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/bundled/goscape.yaml
```

Confirm: at default `info` no `received data` / per-tick lines appear; set `world:\n  log_level: trace` and confirm the firehose appears **only** under `component=world.*`, while other modules stay at the global level. Lines render `level=TRACE` and `source=file.go:line`.

- [ ] **Step 5: Commit**

```bash
git add examples/
git commit --no-gpg-sign -m "docs(config): document trace level + per-module log_level overrides

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Roll out to the other 4 rev branches

**Files:** the same set, on `rev-254`, `rev-245.2`, `rev-244`, `rev-225`.

**Interfaces:** none (port).

The shared infra is byte-identical across branches (verified: `pkg/util/log/log.go` hash matches on all 5; config level/format lines identical). The per-branch difference is only the **call-site sweep** (each branch has a subset of `modules/world` sites).

- [ ] **Step 1: For each branch, cherry-pick the shared-infra commits**

For `<branch>` in `rev-254 rev-245.2 rev-244 rev-225`:

```bash
git switch <branch>
git cherry-pick <Task1-sha> <Task2-sha> <Task3-sha> <Task4-sha>
```

(These four commits — `level.go`, `NewLogger`, global config, per-module config + `modules.go` — should apply cleanly since the infra files match. Resolve any trivial context drift.)

- [ ] **Step 2: Re-create the world component loggers (Task 5) per branch**

Apply Task 5 manually on each branch — the `Server` struct field set and bridge list may differ slightly by rev. Verify the `component=` seams exist; add `modules/world/logging.go` + `logging_test.go`.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentConstants`

- [ ] **Step 3: Re-run the call-site sweep (Task 6) per branch**

Run the Task 6 audit grep on the branch and apply the contract to that branch's call sites. The mandatory known fixes apply wherever those lines exist on the branch.

- [ ] **Step 4: Per-branch verification**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Update that branch's example configs (Task 7) and verify with `--config.verify=true`. Per memory `helm_chart_per_branch_config`, ensure example keys match the branch's struct (they will — the struct gains the same fields).

- [ ] **Step 5: Commit per branch** (squash or keep the cherry-picks + a sweep commit, matching the branch's history style).

---

## Self-Review Notes

- **Spec coverage:** trace level → T1/T2; per-module overrides → T4; component tags → T4 (modules) + T5 (world); level contract sweep + known smells → T6; config docs → T7; rollout → T8; testing → T2/T3/T4/T5/T6/T7. All spec sections map to a task.
- **Type consistency:** `log.Level` (config) vs `slog.Level` (runtime) boundary is explicit in every task; `LevelTrace` is `slog.Level`-typed throughout; component constants (`compWorld`…`compReport`) defined once in T5 and consumed in T5/T6.
- **CLI untouched:** `NewLogger` keeps its `slog.Level` signature (Global Constraints + T2), so the 5 `cmd/goscape-cli` callers compile unchanged.
