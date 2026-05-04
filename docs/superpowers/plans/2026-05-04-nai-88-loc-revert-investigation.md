# NAI-88 — Stage 1 lifecycle-revert probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install 6 NodeDebug-gated `slog.Debug` probes (P1–P6) on the goscape loc lifecycle revert path so a single user-driven re-smoke discriminates surviving hypotheses (a) tracker enrolment / spurious Unregister and (d) `LifecycleTick != now` silent early-return. No behavior changes.

**Architecture:** Investigation sub-spec, Stage 1 only. T1 plumbs a `*slog.Logger` + `nodeDebug bool` into `locObjTracker` (one constructor signature change rippled to 6 call sites — 1 production + 5 test fixtures). T2–T5 add probes at 6 sites: `Register`/`Unregister` (P5/P6), `Server.ChangeLoc` decision both arms (P4), `Server.processZones` lifecycle iter (P1), `Server.turnLoc` entry (P2), `Server.RevertLoc` entry (P3). T6 runs final `go build` + `go test ./...` and emits the user-handoff smoke prompt. Every probe carries the literal comment `// NAI-88 probe; remove at Stage 2 close` so retire is one grep + Edit per site at NAI-89 close. Stage 2 fix is out of scope (NAI-89, separate brainstorm).

**Tech Stack:** Go 1.26+; `log/slog`; existing `cfg.NodeDebug` gating convention (`modules/world/handler_oploc.go:104-118`, `modules/world/interaction_debug.go:48-69`).

---

## Pre-flight controller checklist (run before T1 dispatch)

Per `controller_preflight.md`. Re-grep these line numbers at HEAD to confirm staleness; if any drift, fix the plan inline before dispatching:

```bash
# Production sites
grep -n "newLocObjTracker\|func.*Register\|func.*Unregister" modules/world/loc_tracker.go
# Expected: 4 occurrences at lines 23 (newLocObjTracker), 34 (Register), 43 (Unregister), 1 internal `t.list.AddTail` on line 39

grep -n "locObjTracker: newLocObjTracker" modules/world/server.go
# Expected: line 167

grep -n "loc.IsChanged() || loc.Lifecycle == entitypkg.LifecycleDespawn" modules/world/world_zone.go
# Expected: line 60

grep -n "func (s \*Server) processZones" modules/world/tick.go
# Expected: line 470

grep -n "func (s \*Server) turnLoc\|func (s \*Server) RevertLoc" modules/world/loc_turn.go
# Expected: line 15 (turnLoc), 39 (RevertLoc)

# Test fixture sites
grep -n "newLocObjTracker()" modules/world/loc_tracker_test.go modules/world/server_test.go
# Expected: 4 lines in loc_tracker_test.go (10, 23, 37, 51) + 1 line in server_test.go (318)
```

If any line drifts, update the matching task's "Files" header and step references.

---

## File Structure

| File | Role | Touch |
|---|---|---|
| `modules/world/loc_tracker.go` | Houses `locObjTracker` struct + `Register`/`Unregister`/`All`. T1 adds `log`/`nodeDebug` fields; T2 adds P5/P6 probes inside Register/Unregister. | T1, T2 |
| `modules/world/server.go` | Server constructor. T1 updates one call to `newLocObjTracker(...)` at line 167. | T1 |
| `modules/world/loc_tracker_test.go` | Tracker unit tests. T1 updates 4 fixture sites to pass `(nil, false)`. | T1 |
| `modules/world/server_test.go` | Test-server fixture. T1 updates 1 fixture site at line 318 to pass `(nil, false)`. | T1 |
| `modules/world/world_zone.go` | Houses `Server.ChangeLoc` (line 40). T3 adds P4 probe at the post-decision both-arm fork (line 60-64). | T3 |
| `modules/world/tick.go` | Houses `Server.processZones` (line 470). T4 adds P1 probe inside the lifecycle iter loop (line 482). | T4 |
| `modules/world/loc_turn.go` | Houses `Server.turnLoc` (line 15) and `Server.RevertLoc` (line 39). T5 adds P2 (turnLoc entry) and P3 (RevertLoc entry) probes. | T5 |

No new files. No new packages. No `pkg/entity` changes (that package stays dependency-free; logger lives in `modules/world` only).

---

## Task 1: Plumb logger into `locObjTracker`

Pure refactor. No probes yet. After T1, build + tests must remain green at unchanged shape; the constructor signature change ripples to 6 call sites (1 production + 5 test fixtures). Test fixtures pass `(nil, false)` so the tracker's future probe sites become no-ops in unit tests.

**Files:**
- Modify: `modules/world/loc_tracker.go:1-56`
- Modify: `modules/world/server.go:167`
- Modify: `modules/world/loc_tracker_test.go:10`, `:23`, `:37`, `:51`
- Modify: `modules/world/server_test.go:318`

- [ ] **Step 1: Read current `loc_tracker.go`**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go env GOMOD
cat modules/world/loc_tracker.go
```

Expected: file is 55 lines, no `log/slog` import, no `*slog.Logger` field.

- [ ] **Step 2: Modify `modules/world/loc_tracker.go` — add logger plumbing**

Replace the entire file with:

```go
package world

import (
	"iter"
	"log/slog"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// locObjTracker is the per-Server registry of NonPathing entities with
// pending lifecycle transitions. Iterated each tick by Server.processZones.
// Mirrors TS World.locObjTracker (Engine-TS/.../World.ts:154,964-973).
//
// Backed by pkg/zone.DoublyLinkList for O(1) Add/Unlink and an auxiliary
// map *NonPathing → *Element for O(1) Unregister-by-pointer.
type locObjTracker struct {
	list  *zone.DoublyLinkList[*entitypkg.NonPathing]
	nodes map[*entitypkg.NonPathing]*zone.Element[*entitypkg.NonPathing]

	// log + nodeDebug back the NAI-88 Stage 1 probes (P5/P6) at
	// Register/Unregister. Both are nil-safe; production passes
	// (s.log, s.cfg.NodeDebug) at Server.New, test fixtures pass
	// (nil, false) and the probe emit-helper short-circuits.
	// NAI-88 probe; remove at Stage 2 close.
	log       *slog.Logger
	nodeDebug bool
}

// newLocObjTracker constructs an empty tracker. Server.New calls this
// once at server startup. log+nodeDebug back NAI-88 Stage 1 probes;
// pass (nil, false) in tests to no-op them.
func newLocObjTracker(log *slog.Logger, nodeDebug bool) *locObjTracker {
	return &locObjTracker{
		list:      &zone.DoublyLinkList[*entitypkg.NonPathing]{},
		nodes:     map[*entitypkg.NonPathing]*zone.Element[*entitypkg.NonPathing]{},
		log:       log,
		nodeDebug: nodeDebug,
	}
}

// Register adds np to the tracker. Idempotent — re-registering an
// already-tracked np unlinks the old node first to keep the list
// duplicate-free, matching TS behavior where setLifeCycle always
// unlinks the previous eventTracker before re-adding.
func (t *locObjTracker) Register(np *entitypkg.NonPathing) {
	if existing, ok := t.nodes[np]; ok {
		existing.Unlink()
		delete(t.nodes, np)
	}
	t.nodes[np] = t.list.AddTail(np)
}

// Unregister removes np from the tracker. No-op if np is not tracked.
func (t *locObjTracker) Unregister(np *entitypkg.NonPathing) {
	if e, ok := t.nodes[np]; ok {
		e.Unlink()
		delete(t.nodes, np)
	}
}

// All returns an iterator over the tracked entries in insertion order.
// Callers that mutate the tracker mid-iteration MUST snapshot first
// (Server.processZones does this).
func (t *locObjTracker) All() iter.Seq[*entitypkg.NonPathing] {
	return t.list.All(false)
}
```

(T2 will add P5/P6 emit calls inside `Register`/`Unregister`. T1 leaves the bodies unchanged so this step is a pure plumbing change.)

- [ ] **Step 3: Modify `modules/world/server.go:167` — production call site**

Old:

```go
		locObjTracker: newLocObjTracker(),
```

New:

```go
		locObjTracker: newLocObjTracker(logger, cfg.NodeDebug),
```

Verify with: `grep -n "newLocObjTracker(logger, cfg.NodeDebug)" modules/world/server.go` → exactly 1 hit at line 167.

- [ ] **Step 4: Modify `modules/world/loc_tracker_test.go` — 4 fixture sites**

For each of lines 10, 23, 37, 51 in `modules/world/loc_tracker_test.go`:

Old:

```go
	tr := newLocObjTracker()
```

New:

```go
	tr := newLocObjTracker(nil, false)
```

Use `Edit` with `replace_all: true` on `modules/world/loc_tracker_test.go` since the old string is identical at all 4 sites. Verify with: `grep -c "newLocObjTracker(nil, false)" modules/world/loc_tracker_test.go` → 4. And: `grep -c "newLocObjTracker()" modules/world/loc_tracker_test.go` → 0.

- [ ] **Step 5: Modify `modules/world/server_test.go:318` — test-server fixture**

Old:

```go
		locObjTracker:  newLocObjTracker(),
```

New:

```go
		locObjTracker:  newLocObjTracker(nil, false),
```

(Two-space alignment with surrounding fields preserved.)

- [ ] **Step 6: Verify build + tests**

Run, in this order:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected:
- `go build ./...` → no output, exit 0.
- `go test ./modules/world/...` → all tests pass (tracker tests pass with `(nil, false)` constructor args; nil-log probes will be no-ops once added in T2).
- `go test ./...` → all tests pass.

If any test fails, the constructor signature ripple was missed somewhere. Re-grep for `newLocObjTracker()` (zero-arg form) — must be 0 occurrences across the whole repo.

```bash
grep -rn "newLocObjTracker()" --include="*.go"
```

Expected: empty output (no zero-arg calls remain).

- [ ] **Step 7: Commit**

```bash
git add modules/world/loc_tracker.go modules/world/server.go modules/world/loc_tracker_test.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-88 T1 — locObjTracker logger plumbing

Adds *slog.Logger + nodeDebug bool fields to locObjTracker; the
constructor newLocObjTracker now takes both args. Production call
site at server.go:167 passes (logger, cfg.NodeDebug); 5 test
fixtures pass (nil, false) so existing tracker unit tests stay
green unchanged.

Pure refactor — no probe emissions yet (T2 adds P5/P6 inside
Register/Unregister method bodies). Step toward NAI-88 Stage 1
discrimination of hypotheses (a) tracker enrolment and (d)
silent early-return.
EOF
)"
```

---

## Task 2: P5 + P6 probes in `Register`/`Unregister`

Add the tracker-internal probes that observe every entry/exit of the lifecycle list. P6 captures `runtime.Caller(1)` to surface the unexpected-caller variant of hypothesis (a). T1 already plumbed the logger; T2 only adds emit calls.

**Files:**
- Modify: `modules/world/loc_tracker.go:34-46` (Register and Unregister bodies)

- [ ] **Step 1: Modify `modules/world/loc_tracker.go` — add `runtime` import and probes**

Apply two edits:

(2.1) At the top of `loc_tracker.go`, change the import block from:

```go
import (
	"iter"
	"log/slog"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)
```

to:

```go
import (
	"fmt"
	"iter"
	"log/slog"
	"runtime"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)
```

(2.2) Replace the `Register` body. Old:

```go
func (t *locObjTracker) Register(np *entitypkg.NonPathing) {
	if existing, ok := t.nodes[np]; ok {
		existing.Unlink()
		delete(t.nodes, np)
	}
	t.nodes[np] = t.list.AddTail(np)
}
```

New (the new logic adds 1 probe emit at the end; existing branches unchanged):

```go
func (t *locObjTracker) Register(np *entitypkg.NonPathing) {
	if existing, ok := t.nodes[np]; ok {
		existing.Unlink()
		delete(t.nodes, np)
	}
	t.nodes[np] = t.list.AddTail(np)
	// NAI-88 probe; remove at Stage 2 close.
	if t.nodeDebug && t.log != nil {
		t.log.Debug("nai88 tracker register",
			"event_id", "P5",
			"np_addr", fmt.Sprintf("%p", np),
			"tracker_size_after", t.list.Size(),
		)
	}
}
```

(2.3) Replace the `Unregister` body. Old:

```go
func (t *locObjTracker) Unregister(np *entitypkg.NonPathing) {
	if e, ok := t.nodes[np]; ok {
		e.Unlink()
		delete(t.nodes, np)
	}
}
```

New (probe emits unconditionally on call so we observe both no-op and real Unregister; `caller` field uses `runtime.Caller(1)` to identify the call source — critical for hypothesis (a) "spurious mid-window Unregister"):

```go
func (t *locObjTracker) Unregister(np *entitypkg.NonPathing) {
	hit := false
	if e, ok := t.nodes[np]; ok {
		e.Unlink()
		delete(t.nodes, np)
		hit = true
	}
	// NAI-88 probe; remove at Stage 2 close.
	if t.nodeDebug && t.log != nil {
		caller := "unknown"
		if _, file, line, ok := runtime.Caller(1); ok {
			caller = fmt.Sprintf("%s:%d", file, line)
		}
		t.log.Debug("nai88 tracker unregister",
			"event_id", "P6",
			"np_addr", fmt.Sprintf("%p", np),
			"hit", hit,
			"tracker_size_after", t.list.Size(),
			"caller", caller,
		)
	}
}
```

- [ ] **Step 2: Verify build + tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: both green. Test fixtures pass `(nil, false)` so the new probe blocks are no-ops in tests.

Sanity:

```bash
grep -c "NAI-88 probe" modules/world/loc_tracker.go
```

Expected: 3 (1 in struct comment from T1 + 2 in Register/Unregister probe blocks).

- [ ] **Step 3: Commit**

```bash
git add modules/world/loc_tracker.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-88 T2 — P5/P6 tracker probes

Adds slog.Debug emissions at Register (P5) and Unregister (P6) of
the locObjTracker. Both gated by nodeDebug + nil-log; tests stay
no-op via (nil, false) fixtures from T1.

P5 fields: tick (caller's currentTick available via cross-probe
correlation; P5 itself reports np_addr + tracker_size_after).
P6 fields: np_addr + hit + tracker_size_after + caller (one-frame
runtime.Caller(1) "file:line"). caller is the discrimination key
for hypothesis (a) "spurious mid-window Unregister between tick
43 ChangeLoc and tick 46 processZones".
EOF
)"
```

---

## Task 3: P4 probe in `Server.ChangeLoc` post-decision

Probe the both-arm fork at `world_zone.go:60-64` so we observe whether tick-43 `ChangeLoc(loc, 83, ..., 3)` enrolled the loc (`arm="register"`, `duration=3`) or untracked it (`arm="untrack"`). This discriminates hypothesis (a) at its origin: if the user-driven `loc_change(inviswall, 3)` flowed through the wrong arm, the tracker never sees the np at all.

**Files:**
- Modify: `modules/world/world_zone.go:60-64`

- [ ] **Step 1: Read the current `ChangeLoc` decision block**

```bash
sed -n '55,66p' modules/world/world_zone.go
```

Expected: shows the if/else at lines 60-64 with calls to `loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)` (register arm) and `loc.SetLifeCycle(-1, s.currentTick, nil)` (untrack arm).

- [ ] **Step 2: Modify `modules/world/world_zone.go` — add P4 probe**

Old (lines 60-64):

```go
	if loc.IsChanged() || loc.Lifecycle == entitypkg.LifecycleDespawn {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}
```

New (single probe-emit helper precedes the dispatch and records which arm fires, with all decision inputs):

```go
	armRegister := loc.IsChanged() || loc.Lifecycle == entitypkg.LifecycleDespawn
	// NAI-88 probe; remove at Stage 2 close.
	if s.cfg.NodeDebug && s.log != nil {
		arm := "untrack"
		if armRegister {
			arm = "register"
		}
		s.log.Debug("nai88 change_loc setlifecycle",
			"event_id", "P4",
			"tick", s.currentTick,
			"loc_x", loc.X,
			"loc_z", loc.Z,
			"loc_level", loc.Level,
			"loc_type", loc.Type(),
			"is_changed", loc.IsChanged(),
			"lifecycle", int(loc.Lifecycle),
			"duration", duration,
			"arm", arm,
		)
	}
	if armRegister {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}
```

The pre-computed `armRegister` boolean preserves single-evaluation of the original condition (matters because `IsChanged()` reads `CurrentInfo` which could in principle race; production already serialises on the tick goroutine, but keeping single-eval avoids drift between the probe and the actual dispatch).

- [ ] **Step 3: Verify build + tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: both green.

- [ ] **Step 4: Commit**

```bash
git add modules/world/world_zone.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-88 T3 — P4 ChangeLoc decision probe

Adds slog.Debug at Server.ChangeLoc post-decision fork
(world_zone.go:60-64). Records which SetLifeCycle arm fires
("register" vs "untrack") plus tick/coords/type/is_changed/
lifecycle/duration. Probe is the discriminator for hypothesis
(a) origin — confirms whether tick-43 loc_change(inviswall, 3)
correctly took the register arm with duration=3, or fell into
untrack (would explain non-revert without ever reaching the
tracker).

Pre-computes armRegister to keep single-evaluation of the
IsChanged()-or-DESPAWN condition; preserves observable behaviour.
EOF
)"
```

---

## Task 4: P1 probe in `Server.processZones` lifecycle iter

Probe the lifecycle scan loop in `processZones` so we observe per-tick whether the tracker is empty or populated, and the cursor index of each iteration. This is the discriminator for hypothesis (a) at the consumer side: if `tracker_size=0` from tick 43 onward, the np never registered (regardless of what P5 says — they cross-foot). It also bounds hypothesis (d): the loop must fire on tick 46 with `tracker_size>=1` for P2/P3 to even be reached.

**Files:**
- Modify: `modules/world/tick.go:470-495`

- [ ] **Step 1: Read current `processZones`**

```bash
sed -n '470,496p' modules/world/tick.go
```

Expected: shows the snapshot/iter/ComputeShared structure exactly as quoted in the spec.

- [ ] **Step 2: Modify `modules/world/tick.go` — add P1 probe**

Old (lines 470-495):

```go
func (s *Server) processZones() {
	if s.locObjTracker != nil {
		// Snapshot to a slice — the tracker uses a linked list whose
		// iteration is invalidated by mid-iteration Unlink. The bare
		// type-assert (no comma-ok) panics if the field ever holds
		// something other than *locObjTracker, surfacing the bug
		// loudly rather than silently dropping all per-tick processing.
		t := s.locObjTracker.(*locObjTracker)
		snap := make([]*entitypkg.NonPathing, 0, t.list.Size())
		for np := range t.All() {
			snap = append(snap, np)
		}
		for _, np := range snap {
			switch p := np.Parent().(type) {
			case *entitypkg.Loc:
				s.turnLoc(p, s.currentTick)
			case *entitypkg.Obj:
				// TODO(NAI-86 D-N86-3): Obj.Turn ports later.
				_ = p
			}
		}
	}
	for z := range s.zonesTracking {
		z.ComputeShared()
	}
}
```

New (add an outer-loop tracker_size emit before iteration, plus a per-iter cursor emit inside the dispatch loop):

```go
func (s *Server) processZones() {
	if s.locObjTracker != nil {
		// Snapshot to a slice — the tracker uses a linked list whose
		// iteration is invalidated by mid-iteration Unlink. The bare
		// type-assert (no comma-ok) panics if the field ever holds
		// something other than *locObjTracker, surfacing the bug
		// loudly rather than silently dropping all per-tick processing.
		t := s.locObjTracker.(*locObjTracker)
		snap := make([]*entitypkg.NonPathing, 0, t.list.Size())
		for np := range t.All() {
			snap = append(snap, np)
		}
		// NAI-88 probe; remove at Stage 2 close.
		if s.cfg.NodeDebug && s.log != nil {
			s.log.Debug("nai88 process_zones iter",
				"event_id", "P1",
				"tick", s.currentTick,
				"tracker_size", len(snap),
				"cursor", -1,
			)
		}
		for i, np := range snap {
			// NAI-88 probe; remove at Stage 2 close.
			if s.cfg.NodeDebug && s.log != nil {
				s.log.Debug("nai88 process_zones iter",
					"event_id", "P1",
					"tick", s.currentTick,
					"tracker_size", len(snap),
					"cursor", i,
					"np_addr", fmt.Sprintf("%p", np),
				)
			}
			switch p := np.Parent().(type) {
			case *entitypkg.Loc:
				s.turnLoc(p, s.currentTick)
			case *entitypkg.Obj:
				// TODO(NAI-86 D-N86-3): Obj.Turn ports later.
				_ = p
			}
		}
	}
	for z := range s.zonesTracking {
		z.ComputeShared()
	}
}
```

The `cursor=-1` outer emit lets us see "tick fired, tracker empty" cases (no per-iter emit follows). The inner emits include `np_addr` so they correlate with P5/P6 by the same key.

(2.2) Add `"fmt"` to the import block of `modules/world/tick.go` if not already present:

```bash
head -15 modules/world/tick.go
```

If `"fmt"` is absent, edit the import block. Old:

```go
import (
	"sort"
	"time"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)
```

New:

```go
import (
	"fmt"
	"sort"
	"time"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)
```

If `"fmt"` is already there, skip 2.2.

- [ ] **Step 3: Verify build + tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: both green.

- [ ] **Step 4: Commit**

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-88 T4 — P1 processZones lifecycle iter probe

Adds two slog.Debug emit sites inside Server.processZones: an
outer-loop "tracker_size=N, cursor=-1" frame fires every tick
even when the tracker is empty, and a per-iteration cursor frame
captures np_addr for cross-probe correlation with P5/P6.

P1 lets us cross-foot the tracker_size at tick 46 against P5's
tick-43 register and any P6 unregister between. Empty tracker
at tick 46 falsifies the "registered but stuck" reading of
hypothesis (d) and confirms hypothesis (a) np-not-registered.
EOF
)"
```

---

## Task 5: P2 + P3 probes in `turnLoc` / `RevertLoc` entry

Probe the consumer-side gates: P2 fires at `turnLoc` entry **before** the `LifecycleTick != now` early-return guard, so we observe every tick that the np reaches the dispatcher. P3 fires at `RevertLoc` entry, confirming the case-2 dispatch landed. Together they discriminate hypothesis (d): if P2 fires at tick 46 with `lifecycle_tick=46, now=46` but P3 doesn't fire, the bug is in the switch case-match; if P2 fires but reports `lifecycle_tick != now`, the early-return is hit and `LifecycleTick` is wrong upstream.

**Files:**
- Modify: `modules/world/loc_turn.go:15-33` (turnLoc)
- Modify: `modules/world/loc_turn.go:39-62` (RevertLoc)

- [ ] **Step 1: Modify `modules/world/loc_turn.go` — add `turnLoc` (P2) probe**

Old (lines 15-18, the entry of `turnLoc` and the guard):

```go
func (s *Server) turnLoc(l *entitypkg.Loc, now int) {
	if l.LifecycleTick != now {
		return
	}
```

New (probe fires *before* the guard so the early-return is observable):

```go
func (s *Server) turnLoc(l *entitypkg.Loc, now int) {
	// NAI-88 probe; remove at Stage 2 close.
	if s.cfg.NodeDebug && s.log != nil {
		s.log.Debug("nai88 turn_loc entry",
			"event_id", "P2",
			"tick", s.currentTick,
			"now", now,
			"loc_x", l.X,
			"loc_z", l.Z,
			"loc_level", l.Level,
			"loc_type", l.Type(),
			"lifecycle", int(l.Lifecycle),
			"is_active", l.IsActive,
			"is_changed", l.IsChanged(),
			"lifecycle_tick", l.LifecycleTick,
		)
	}
	if l.LifecycleTick != now {
		return
	}
```

- [ ] **Step 2: Modify `modules/world/loc_turn.go` — add `RevertLoc` (P3) probe**

Old (line 39 entry + the existing top of the function body):

```go
func (s *Server) RevertLoc(l *entitypkg.Loc) {
	if s.gamemap != nil && s.locTypes != nil {
```

New:

```go
func (s *Server) RevertLoc(l *entitypkg.Loc) {
	// NAI-88 probe; remove at Stage 2 close.
	if s.cfg.NodeDebug && s.log != nil {
		s.log.Debug("nai88 revert_loc entry",
			"event_id", "P3",
			"tick", s.currentTick,
			"loc_x", l.X,
			"loc_z", l.Z,
			"loc_level", l.Level,
			"loc_type", l.Type(),
		)
	}
	if s.gamemap != nil && s.locTypes != nil {
```

- [ ] **Step 3: Verify build + tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: both green.

Cross-foot probe count:

```bash
grep -c "NAI-88 probe" modules/world/loc_turn.go modules/world/world_zone.go modules/world/tick.go modules/world/loc_tracker.go
```

Expected per-file counts:
- `modules/world/loc_turn.go`: 2 (P2, P3)
- `modules/world/world_zone.go`: 1 (P4)
- `modules/world/tick.go`: 2 (P1 outer + P1 inner)
- `modules/world/loc_tracker.go`: 3 (struct comment + P5 + P6)

Total: 8 occurrences of `NAI-88 probe` across the touched files. (Probe-site count is 6: P1×2 sites + P2 + P3 + P4 + P5 + P6 = 7 emit blocks; the 8th occurrence is the struct-field doc comment in loc_tracker.go from T1.)

- [ ] **Step 4: Commit**

```bash
git add modules/world/loc_turn.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-88 T5 — P2/P3 turnLoc + RevertLoc entry probes

Adds slog.Debug at Server.turnLoc entry (P2) BEFORE the
LifecycleTick != now early-return guard, and at Server.RevertLoc
entry (P3). P2 fields include both s.currentTick and the now arg
to surface any clock-threading divergence between
processZones (which passes s.currentTick) and turnLoc's local
view; lifecycle_tick is reported so a "stuck" entry shows up
as lifecycle_tick != now on every iteration after tick 43.

Together with P1, this discriminates hypothesis (d): np reaches
turnLoc but lifecycle_tick is wrong vs np reaches turnLoc and
matches but case-arm dispatches don't fire.
EOF
)"
```

---

## Task 6: Final verification + Stage 1 close commit + smoke handoff

No code changes. Run the full test+build sweep, generate the user-handoff smoke prompt, and write the close commit with the `Closes memory:` trailer.

**Files:** none modified. Close commit is empty-of-code (or piggybacks on T5 if `git status` is clean).

- [ ] **Step 1: Full repo verification**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected: both green.

- [ ] **Step 2: Probe inventory check**

```bash
grep -rn "nai88 " modules/world --include="*.go" | grep -v _test.go
```

Expected: 7 emit-site lines (P1 outer + P1 inner + P2 + P3 + P4 + P5 + P6) with event_id values P1..P6 (P1 at 2 sites). Confirm one line per `event_id`:

```bash
grep -rn "event_id.*\"P[1-6]\"" modules/world --include="*.go" | grep -v _test.go | wc -l
```

Expected: `7` (P1 appears at 2 sites; P2..P6 each at 1).

- [ ] **Step 3: Probe-comment retire-marker check**

```bash
grep -rn "NAI-88 probe; remove at Stage 2 close" modules/world --include="*.go" | grep -v _test.go | wc -l
```

Expected: `8` (one per emit block + the struct-field doc comment).

- [ ] **Step 4: Sanity check — no zero-arg tracker calls remain anywhere**

```bash
grep -rn "newLocObjTracker()" --include="*.go"
```

Expected: empty output.

- [ ] **Step 5: Re-grep that no `pkg/entity` files were touched**

`pkg/entity` must remain dependency-free of `log/slog`. Verify:

```bash
git diff --stat HEAD~5..HEAD -- pkg/entity
```

Expected: empty output (the 5-task stretch from T1..T5 should not have modified anything under `pkg/entity`).

- [ ] **Step 6: Emit the smoke handoff prompt**

Per `smoke_test_server_handoff.md`, the user launches the world server. Report the following exactly to the user (post-task summary; do not run the server in this session):

```
NAI-88 Stage 1 probes installed at HEAD <SHA-of-T5-or-T6-close>. Re-smoke instructions:

1. From this repo: launch the world server with --world.node-debug=true
   (already default per modules/world/config.go:76):

   CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml

2. Connect with the Java client and log in to a Tutorial Island
   character positioned to click newbie_door1.

3. Click newbie_door1 once. Wait 5+ ticks past tick 43 (≈3 seconds
   real time after the click resolves and player walks through).

4. Capture the world server log. Filter to the probe channel:

   grep "nai88 " <world.log> > nai88-smoke.log

5. Hand nai88-smoke.log back for analysis. Expected line count for a
   correct revert: ~12 lines (P4 once at tick 43, P5 once at tick 43,
   P1 at every tick from 43 onward with cursor=-1 plus per-iter cursors,
   P2 at every tick the loc reaches turnLoc, P3 once at tick 46, P6
   once at tick 46 from RevertLoc's SetLifeCycle(-1) tail).

   The discrimination table in the spec maps observed line patterns
   to root-cause routing for NAI-89.
```

- [ ] **Step 7: Stage 1 close commit (empty if T5 already committed everything)**

If `git status` is clean from T5 (everything already committed), make an empty Stage-1-close marker commit:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
close: NAI-88 Stage 1 — lifecycle-revert probe set installed

6 probe sites (P1..P6) live across processZones/turnLoc/RevertLoc/
ChangeLoc/Register/Unregister. NodeDebug-gated, nil-log-safe; all
paths fall back to no-op in test fixtures via (nil, false) tracker
construction. go build ./... + go test -count=1 ./... green at
this HEAD.

User-driven re-smoke pending: click newbie_door1 on Tutorial
Island, capture nai88-smoke.log, route results through the
discrimination table in
docs/superpowers/specs/2026-05-04-nai-88-loc-revert-investigation-design.md
to drive Stage 2 (NAI-89) brainstorm.

Closes memory: nai_followups.md NAI-87 carry-forward (NAI-88
candidate lifecycle revert).
EOF
)"
```

If `git status` shows uncommitted changes from T5, those should be staged + included; the close commit instead amends the message of T5's commit by replacing the task-N body with the close-commit body **only if T5 was the final code commit and contained all of T5's work** — but per project convention (`close_commit_memory_trailer.md`), prefer a separate close commit. Do NOT use `git commit --amend` here.

---

## Self-review (run before user handoff)

**1. Spec coverage:**

| Spec section | Plan task |
|---|---|
| Probe site P1 (processZones lifecycle iter) | T4 |
| Probe site P2 (turnLoc entry) | T5 |
| Probe site P3 (RevertLoc entry) | T5 |
| Probe site P4 (ChangeLoc decision both arms) | T3 |
| Probe site P5 (locObjTracker.Register) | T2 |
| Probe site P6 (locObjTracker.Unregister with caller) | T2 |
| Logger plumbing into locObjTracker | T1 |
| 5 test fixture updates `(nil, false)` | T1 step 4–5 |
| Build + tests green at every commit | every task step 6/2/3 |
| Per-probe field schemas (tick + event_id + site-specific) | T2/T3/T4/T5 step bodies |
| `// NAI-88 probe; remove at Stage 2 close` retire markers | every probe block |
| Smoke handoff (NodeDebug=true) + grep nai88 | T6 step 6 |
| `Closes memory:` trailer | T6 step 7 |

No gaps.

**2. Placeholder scan:** All steps contain real code blocks. No "TBD", no "similar to Task N", no "fill in". The fixture updates at T1.4 are repeated across 4 sites with one explicit `replace_all: true` instruction.

**3. Type/identifier consistency:**
- `locObjTracker` constructor: `newLocObjTracker(log *slog.Logger, nodeDebug bool)` — same signature in T1 and at every call site.
- Field names: `t.log`, `t.nodeDebug` (T1 + T2). Match struct definition.
- Event IDs: `"P1"` (T4 outer + inner), `"P2"` (T5), `"P3"` (T5), `"P4"` (T3), `"P5"` (T2), `"P6"` (T2). Match spec.
- Probe-comment marker string: `"NAI-88 probe; remove at Stage 2 close"` literal at every site (struct comment + 7 emit blocks). Used as the grep target in T6 step 3.
- `s.cfg.NodeDebug` + `s.log != nil` gating identical at every Server-method probe (P1–P4); `t.nodeDebug` + `t.log != nil` for tracker-internal probes (P5–P6).
- `np_addr` field uses `fmt.Sprintf("%p", np)` consistently at P1-inner, P5, P6 — same correlation key shape.

No drift.
