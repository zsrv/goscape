# NAI-114 Stage 3 — OPHELDU instrumentation probe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a single revertable instrumentation pass to bind among Path A1 (script not registered), Path A2 (trigger-lookup ID mismatch), Path B (script ran but exited on a gate), and Path D (cascade rebound) for the NAI-114 firemaking smoke residual.

**Architecture:** Three instrumentation sites — boot-time `[opheldu,*]` enumeration (one new helper file), pre-dispatch context log inside `handleOpHeldU`, and per-arm hit-trace logs additively inserted in the existing 4-arm fallback chain. One new exported accessor on `script.Provider` (`Names() []string`). All output goes to `slog` at INFO/DEBUG levels — no test assertions on log shape (probe code is throwaway).

**Tech Stack:** Go 1.26+. Mirrors LostCityRS/Engine-TS canonical TS source for invariants only (no TS-faithful semantics added or changed).

---

## File-Structure Map

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/provider.go` | Modify (append after `Count()` at L210-218) | Add `Names() []string` accessor returning all registered script names |
| `pkg/script/provider_test.go` | Create (new file) OR modify existing | Single test pinning `Names()` returns expected names for a small Register'd fixture |
| `modules/world/debug_nai114.go` | Create (new file) | `logOpHeldUScriptInventory(*script.Provider, *slog.Logger)` helper — iterates `Provider.Names()`, filters by `[opheldu,` prefix, emits one INFO line |
| `modules/world/server.go` | Modify (single 3-line block after L295, inside the `s.scriptProvider != nil` guard) | Call `logOpHeldUScriptInventory(s.scriptProvider, s.log)` once Load succeeds |
| `modules/world/handler_opheld.go` | Modify (additive only — pre-dispatch context log + 4× per-arm hit-trace + dispatch-or-miss log) | Inline DEBUG log calls at the 6 insertion points described in spec §4.2-4.3 |

No file deletions. No restructuring of existing code blocks. No changes to TS-faithful semantics.

---

## Reference Material

### `handler_opheld.go` current state (verified at HEAD `1e178cb`)

The 4-arm fallback at lines 370-396 (excerpted; verbatim):

```go
// 4-arm trigger fallback (TS OpHeldUHandler.ts:96-117); first hit wins.
sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)

if sf == nil {
    sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, useObjType.ConfigType.ID, -1)
    // Arm (b): UNCONDITIONAL swap whenever (a) misses, regardless of
    // whether (b)'s lookup succeeded (TS OpHeldUHandler.ts:101-102).
    p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
    p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
}

if sf == nil && objType.Category != -1 {
    sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
}

if sf == nil && useObjType.Category != -1 {
    sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
    // Arm (d): UNCONDITIONAL swap whenever (c) misses or is skipped,
    // regardless of whether (d)'s lookup succeeded (TS OpHeldUHandler.ts:115-116).
    p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
    p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
}

if sf == nil {
    p.MessageGame("Nothing interesting happens.")
    return nil
}

s.runScript(sf, p, nil, true, nil, nil)
return nil
```

### `script.Provider` (verified at HEAD `1e178cb`)

`pkg/script/provider.go` exposes `byName map[string]*ScriptFile` (unexported, L19), `Register(*ScriptFile)` (L182), `Count() int` (L210). The new accessor goes after `Count()`.

### `Server.log` (verified at HEAD `1e178cb`)

`modules/world/server.go:49` declares `log *slog.Logger`. Reachable from `handleOpHeldU` via `s := p.client.server` then `s.log` (already done at line 275).

### `objType.ConfigType` (verified at HEAD `1e178cb`)

`pkg/objtype/configtype.go:11-15` — `ConfigType{ID int, DebugName string}`. `objType.ConfigType.ID` and `objType.ConfigType.DebugName` are the loggable identifiers. `objType.Category` is `int` at `pkg/objtype/objtype.go:132`.

---

## Self-Review Checklist (already applied during plan-write)

1. **Spec coverage:** §4.1 → Task 1 + Task 2; §4.2 → Task 3 Step 3.2; §4.3 → Task 3 Steps 3.3-3.7; §4.4 → Task 4. §5 smoke routing → Task 5 (handoff text). §3 out-of-scope items not in any task (correctly excluded).
2. **Placeholder scan:** none.
3. **Type consistency:** `Names()` returning `[]string` used identically in Task 1 (signature), Task 2 (caller), and `pkg/script/provider_test.go` test (assertion). `logOpHeldUScriptInventory` signature identical between Task 2 (definition) and Task 2 (call site).

---

## Task 1: Add `Provider.Names()` accessor

**Files:**
- Modify: `pkg/script/provider.go` (append after `Count()` at L210-218)
- Create: `pkg/script/provider_test.go` (new file)

**Why:** `script.Provider.byName` is unexported; the boot-time enumeration in Task 2 needs a way to iterate registered names without leaking the map. A 3-line accessor is cheaper than the alternative (looping `GetByID` 0..Count()-1 reading `f.Name`).

- [ ] **Step 1.1: Write failing test for `Names()`**

Create `pkg/script/provider_test.go` with content:

```go
package script

import (
	"slices"
	"testing"
)

// TestProviderNames pins the new Names() accessor returning every
// registered script's Name. NAI-114 Stage 3 instrumentation.
func TestProviderNames(t *testing.T) {
	p := NewProvider()
	p.Register(&ScriptFile{Name: "[opheldu,tinderbox]", LookupKey: 0xFFFFFFFF})
	p.Register(&ScriptFile{Name: "[opheldu,logs]", LookupKey: 0xFFFFFFFF})
	p.Register(&ScriptFile{Name: "[proc,unrelated]", LookupKey: 0xFFFFFFFF})

	got := p.Names()
	slices.Sort(got)
	want := []string{"[opheldu,logs]", "[opheldu,tinderbox]", "[proc,unrelated]"}
	if !slices.Equal(got, want) {
		t.Errorf("Names(): got %v, want %v", got, want)
	}
}
```

(Note: `LookupKey: 0xFFFFFFFF` makes `Register` skip the byKey insertion — see provider.go:187-189. We only care about byName for Names().)

- [ ] **Step 1.2: Run the test to confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestProviderNames ./pkg/script/`

Expected: FAIL — compile error `p.Names undefined`.

- [ ] **Step 1.3: Add `Names()` accessor**

Open `pkg/script/provider.go`. Append after `Count()` (after L218):

```go

// Names returns the slice of all registered script names. Order is map
// iteration order (unstable). Used by NAI-114 Stage 3 instrumentation to
// filter for [opheldu,*] entries at boot. Tests should sort the result
// before comparing.
func (p *Provider) Names() []string {
	out := make([]string, 0, len(p.byName))
	for name := range p.byName {
		out = append(out, name)
	}
	return out
}
```

- [ ] **Step 1.4: Re-run test → expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestProviderNames ./pkg/script/`

- [ ] **Step 1.5: Run full pkg/script suite → expect all PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`

- [ ] **Step 1.6: Commit**

```bash
git add pkg/script/provider.go pkg/script/provider_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-114 Stage 3 — add Provider.Names() accessor

Exposes a sorted-iterable list of all registered script names. Required
by the upcoming NAI-114 Stage 3 instrumentation pass which filters for
[opheldu,*] entries at boot to bind whether the firemaking script is
even registered.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Create `debug_nai114.go` + wire boot-time enumeration

**Files:**
- Create: `modules/world/debug_nai114.go` (new file)
- Modify: `modules/world/server.go` (single 3-line block after L295, inside the existing `s.scriptProvider != nil` path)

**Why:** Single-file containment of the probe makes Stage 4 close revert as `git revert <SHA>`. Boot-time enumeration emits once at startup; if `[opheldu,tinderbox]` is absent, Path A1 binds without needing a smoke run.

- [ ] **Step 2.1: Create `modules/world/debug_nai114.go`**

Write the file:

```go
package world

import (
	"log/slog"
	"sort"
	"strings"

	"github.com/zsrv/goscape/pkg/script"
)

// logOpHeldUScriptInventory emits a single INFO line listing every
// registered script whose name starts with "[opheldu,". Called once at
// server boot after script provider Load succeeds. NAI-114 Stage 3
// instrumentation; revert at Stage 4 close.
func logOpHeldUScriptInventory(p *script.Provider, log *slog.Logger) {
	if p == nil || log == nil {
		return
	}
	all := p.Names()
	matches := make([]string, 0, 8)
	for _, name := range all {
		if strings.HasPrefix(name, "[opheldu,") {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	log.Info("opheldu script registry",
		"count", len(matches),
		"names", strings.Join(matches, ","))
}
```

- [ ] **Step 2.2: Wire the call from `server.go`**

Open `modules/world/server.go`. Currently L291-295:

```go
	s.scriptProvider = script.NewProvider()
	if err := s.scriptProvider.Load(filepath.Join(cfg.CachePath, "server")); err != nil {
		s.log.Warn("script provider load failed; scripts will not run", "err", err)
		s.scriptProvider = nil
	}
```

Replace with (adds 3 lines after the closing brace at L295):

```go
	s.scriptProvider = script.NewProvider()
	if err := s.scriptProvider.Load(filepath.Join(cfg.CachePath, "server")); err != nil {
		s.log.Warn("script provider load failed; scripts will not run", "err", err)
		s.scriptProvider = nil
	}
	if s.scriptProvider != nil {
		logOpHeldUScriptInventory(s.scriptProvider, s.log)
	}
```

- [ ] **Step 2.3: Build → expect clean**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

- [ ] **Step 2.4: Run full test suite → expect all PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

(Tests should be unaffected — boot-time logging is benign and only emits when `s.scriptProvider != nil` post-Load, which is the production path.)

- [ ] **Step 2.5: Commit**

```bash
git add modules/world/debug_nai114.go modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(debug): NAI-114 Stage 3 — boot-time opheldu script-registry log

Adds a one-shot INFO log line at server boot enumerating every
registered script whose name starts with "[opheldu,". Binds NAI-114
Stage 3 hypothesis Path A1 (script not registered) without requiring a
smoke run.

All instrumentation lives in the new file modules/world/debug_nai114.go
plus a 3-line hook in server.go. Reverted at NAI-114 Stage 4 close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Inline instrumentation in `handler_opheld.go`

**Files:**
- Modify: `modules/world/handler_opheld.go` (6 additive log-call insertions in `handleOpHeldU` body)

**Why:** Each OPHELDU event needs server-stdout visibility into (a) the obj/useObj resolution that feeds the trigger lookup, (b) which arms hit/missed, (c) whether dispatch occurred or fell through to "Nothing interesting happens." `s.log` is reachable via `s := p.client.server` (already in scope at the function's existing line 275).

**Constraint:** ADDITIVE ONLY. Do not restructure the 4-arm chain. Existing tests in `handler_opheld_test.go` must remain unmodified and green.

- [ ] **Step 3.1: HEAD-verify line numbers before editing**

Open `modules/world/handler_opheld.go` at HEAD. Confirm:
- Line 275: `s := p.client.server`
- Line 369 (or thereabouts): blank line just before `// 4-arm trigger fallback (TS OpHeldUHandler.ts:96-117); first hit wins.`
- Line 371: `sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)`
- Lines 373-379: arm (b) block (`if sf == nil { sf = ...; p.lastItem, p.lastUseItem = ...; p.lastSlot, p.lastUseSlot = ... }`)
- Lines 381-383: arm (c) block (`if sf == nil && objType.Category != -1 { sf = ... }`)
- Lines 385-391: arm (d) block (`if sf == nil && useObjType.Category != -1 { sf = ...; ... }`)
- Lines 393-396: fallback-miss block (`if sf == nil { p.MessageGame("Nothing interesting happens."); return nil }`)
- Line 398: `s.runScript(sf, p, nil, true, nil, nil)`

If any of the line numbers have drifted, adjust insertion points to match the same anchors (find by string-search for the comment "4-arm trigger fallback", then by the function calls themselves).

- [ ] **Step 3.2: Insert pre-dispatch context log (Site 2)**

Insert the following IMMEDIATELY BEFORE the `// 4-arm trigger fallback (TS OpHeldUHandler.ts:96-117); first hit wins.` comment line (i.e., directly after the members-only-gate block ending around line 367):

```go
	s.log.Debug("opheldu trigger probe context",
		"tick", s.currentTick,
		"obj", obj,
		"obj_name", objType.ConfigType.DebugName,
		"obj_config_id", objType.ConfigType.ID,
		"obj_category", objType.Category,
		"useObj", useObj,
		"useObj_name", useObjType.ConfigType.DebugName,
		"useObj_config_id", useObjType.ConfigType.ID,
		"useObj_category", useObjType.Category)
```

- [ ] **Step 3.3: Insert arm (a) hit-trace (Site 3a)**

Insert IMMEDIATELY AFTER the line `sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)`:

```go
	s.log.Debug("opheldu arm probe", "arm", "a", "key", "type", "type_id", objType.ConfigType.ID, "hit", sf != nil)
```

- [ ] **Step 3.4: Insert arm (b) hit-trace (Site 3b)**

Inside the `if sf == nil { ... }` block for arm (b), insert IMMEDIATELY AFTER the existing `sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, useObjType.ConfigType.ID, -1)` line, BEFORE the `// Arm (b): UNCONDITIONAL swap...` comment and the swap statements:

```go
		s.log.Debug("opheldu arm probe", "arm", "b", "key", "type", "type_id", useObjType.ConfigType.ID, "hit", sf != nil)
```

- [ ] **Step 3.5: Insert arm (c) hit-trace (Site 3c)**

The arm (c) block is currently:

```go
	if sf == nil && objType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
	}
```

Replace with (adds 1 line after the lookup, plus an else-branch logging the skip case):

```go
	if sf == nil && objType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
		s.log.Debug("opheldu arm probe", "arm", "c", "key", "category", "category_id", objType.Category, "hit", sf != nil)
	} else if sf == nil {
		s.log.Debug("opheldu arm probe", "arm", "c", "skipped", true, "reason", "objType.Category == -1")
	}
```

(The `else if sf == nil` guard means: log "skipped" only when arm (c) was bypassed because the category was -1 — not when we're skipping because an earlier arm hit. When `sf != nil`, neither branch fires; we already logged the earlier arm's hit.)

- [ ] **Step 3.6: Insert arm (d) hit-trace (Site 3d)**

The arm (d) block is currently:

```go
	if sf == nil && useObjType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
		// Arm (d): UNCONDITIONAL swap whenever (c) misses or is skipped,
		// regardless of whether (d)'s lookup succeeded (TS OpHeldUHandler.ts:115-116).
		p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
		p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
	}
```

Replace with (adds the hit log after the lookup, plus an else-branch for the skip case):

```go
	if sf == nil && useObjType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
		s.log.Debug("opheldu arm probe", "arm", "d", "key", "category", "category_id", useObjType.Category, "hit", sf != nil)
		// Arm (d): UNCONDITIONAL swap whenever (c) misses or is skipped,
		// regardless of whether (d)'s lookup succeeded (TS OpHeldUHandler.ts:115-116).
		p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
		p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
	} else if sf == nil {
		s.log.Debug("opheldu arm probe", "arm", "d", "skipped", true, "reason", "useObjType.Category == -1")
	}
```

- [ ] **Step 3.7: Insert fallback-miss + dispatch logs (Site 3-final)**

The fallback-miss block is currently:

```go
	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}

	s.runScript(sf, p, nil, true, nil, nil)
	return nil
```

Replace with (adds one DEBUG log inside the miss block before the existing `p.MessageGame`, and one DEBUG log before the existing `s.runScript`):

```go
	if sf == nil {
		s.log.Debug("opheldu fallback miss — sending Nothing interesting happens", "tick", s.currentTick)
		p.MessageGame("Nothing interesting happens.")
		return nil
	}

	s.log.Debug("opheldu dispatch", "tick", s.currentTick, "script", sf.Name)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
```

- [ ] **Step 3.8: Build → expect clean**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

- [ ] **Step 3.9: Run full test suite → expect all PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Existing handler_opheld_test.go tests must pass unmodified — the additive-only constraint preserves all observable behavior.

- [ ] **Step 3.10: Commit**

```bash
git add modules/world/handler_opheld.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(debug): NAI-114 Stage 3 — opheldu trigger-lookup hit-trace

Adds 6 inline DEBUG log calls to handleOpHeldU:
  - one pre-dispatch context log (obj/useObj IDs, names, categories)
  - one per-arm hit-trace (4 total: a, b, c, d) with skip-cause when
    category arms are bypassed
  - one dispatch log when a script is found, OR one fallback-miss log
    when all 4 arms miss and the handler sends "Nothing interesting
    happens."

Additive only — no semantic change to the 4-arm chain. All existing
handler_opheld_test.go tests pass unmodified. Reverted at NAI-114
Stage 4 close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Smoke handoff

**Files:** none (out-of-tree verification).

**Why:** Per `smoke_test_server_handoff` — instrumentation runs against a live goscape server in the user's environment.

- [ ] **Step 4.1: Confirm post-instrumentation `go test ./...` PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages PASS (cached or fresh). No regressions.

- [ ] **Step 4.2: Hand off to user for smoke launch**

Reply to user with this paste-ready handoff:

> NAI-114 Stage 3 instrumentation complete. Please launch the goscape server for smoke verification:
>
> `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml`
>
> Then connect via the Java client rev-225 and walk Tutorial Island to the fire-making step (use tinderbox on logs in inventory at least once).
>
> **Capture from server stdout:**
>
> 1. The single INFO line at startup matching `msg="opheldu script registry"` (with `count=` and `names=`).
> 2. Per OPHELDU event: the DEBUG lines matching `msg="opheldu trigger probe context"`, `msg="opheldu arm probe"` (4 of these per event), and either `msg="opheldu dispatch"` OR `msg="opheldu fallback miss — sending Nothing interesting happens"`.
> 3. Whether any subsequent `level=WARN msg="script execute error"` line appears.
>
> Paste those lines back. Decision matrix in spec §5.1 will route to Stage 4.

- [ ] **Step 4.3: Wait for user smoke result before designing Stage 4**

Do not write any further code or commits. Stage 4 brainstorm starts only after user provides the smoke output.

---

## Stage-3-internal close (after smoke binding, before Stage 4)

This sub-spec is closed at the moment the smoke output binds exactly one row in the spec §5.1 decision matrix. The Stage-3 close commit is **deferred** and folded into the eventual NAI-114 Stage 4 close commit (which also reverts the instrumentation in Tasks 2-3). No standalone Stage-3 close commit; instead:

- The decision-matrix row's diagnosis is recorded as the opening line of the Stage 4 brainstorm.
- The instrumentation revert (`git revert` of Task 2 and Task 3 commits) lands as the first commit of Stage 4 implementation, before any fix code.
- Memory updates (per spec §7) deferred to Stage 4 close.
