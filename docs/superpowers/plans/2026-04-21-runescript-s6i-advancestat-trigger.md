# RuneScript S6i: AdvanceStat Trigger Fire Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Player.advanceStat` dispatch (Player.ts:1804-1807). New `Provider.GetByTriggerSpecific` (single-tier lookup, no fallback) + new unexported `Player.advanceStat(stat int)` helper. Wired into `Player.AddXP`'s level-up block as a sibling call after `changeStat`. Closes the S6h final-review follow-up #2.

**Architecture:** Two tasks. Task 1 ships the new Provider method + 6 tier-isolation tests — pure infrastructure with no consumer changes; build stays green. Task 2 ships the `advanceStat` helper, wires it into AddXP, refreshes the AddXP doc comment, and adds 4 fire-semantic tests including an integration test that verifies S6h's changeStat and S6i's advanceStat both fire in TS order.

**Tech Stack:** Go; `pkg/script` Provider lookup; `modules/world` Player level-up dispatch.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6i-advancestat-trigger-design.md`](../specs/2026-04-21-runescript-s6i-advancestat-trigger-design.md) (commit `3b3d5ac`)

---

## Task 1: `Provider.GetByTriggerSpecific` + 6 tier-isolation tests

**Files:**
- Modify: `pkg/script/provider.go` (append `GetByTriggerSpecific` after `GetByTrigger`)
- Modify: `pkg/script/provider_test.go` (append 6 tier-isolation tests)

This task is pure infrastructure. No consumer changes — `Player.advanceStat` (Task 2) is the first caller.

- [ ] **Step 1: Write the failing tests FIRST.** Open `pkg/script/provider_test.go`. Append at the end:

```go
func TestGetByTriggerSpecificTypeOnly(t *testing.T) {
	p := NewProvider()
	typeKey := uint32(TriggerAdvanceStat) | (0x2 << 8) | (uint32(0) << 10) // stat 0 = Attack
	sf := &ScriptFile{Name: "[advancestat,attack]", LookupKey: typeKey}
	p.Register(sf)

	got := p.GetByTriggerSpecific(TriggerAdvanceStat, 0, -1)
	if got != sf {
		t.Errorf("type-specific lookup: got %v, want %v", got, sf)
	}
}

func TestGetByTriggerSpecificCategoryOnly(t *testing.T) {
	p := NewProvider()
	catKey := uint32(TriggerChangeStat) | (0x1 << 8) | (uint32(7) << 10) // category 7
	sf := &ScriptFile{Name: "[changestat,_cat7]", LookupKey: catKey}
	p.Register(sf)

	got := p.GetByTriggerSpecific(TriggerChangeStat, -1, 7)
	if got != sf {
		t.Errorf("category-only lookup: got %v, want %v", got, sf)
	}
}

func TestGetByTriggerSpecificGlobalOnly(t *testing.T) {
	p := NewProvider()
	globalKey := uint32(TriggerChangeStat)
	sf := &ScriptFile{Name: "[changestat,_]", LookupKey: globalKey}
	p.Register(sf)

	got := p.GetByTriggerSpecific(TriggerChangeStat, -1, -1)
	if got != sf {
		t.Errorf("global-only lookup: got %v, want %v", got, sf)
	}
}

func TestGetByTriggerSpecificNoFallback(t *testing.T) {
	// Register ONLY the global tier; specific lookup must NOT fall through.
	p := NewProvider()
	globalKey := uint32(TriggerAdvanceStat)
	p.Register(&ScriptFile{Name: "[advancestat,_]", LookupKey: globalKey})

	got := p.GetByTriggerSpecific(TriggerAdvanceStat, 0, -1)
	if got != nil {
		t.Errorf("type-specific lookup with only-global registered: got %v, want nil (no fallback)", got)
	}
}

func TestGetByTriggerSpecificTypeShortCircuitsCategory(t *testing.T) {
	// type=5, cat=3 — only category script registered. Specific must return nil
	// because typeID != -1 picks the type tier and ignores the cat tier.
	p := NewProvider()
	catKey := uint32(TriggerChangeStat) | (0x1 << 8) | (uint32(3) << 10)
	p.Register(&ScriptFile{Name: "[changestat,_cat3]", LookupKey: catKey})

	got := p.GetByTriggerSpecific(TriggerChangeStat, 5, 3)
	if got != nil {
		t.Errorf("type-tier short-circuit: got %v, want nil (cat ignored when type set)", got)
	}
}

func TestGetByTriggerSpecificMissingReturnsNil(t *testing.T) {
	p := NewProvider() // empty
	if got := p.GetByTriggerSpecific(TriggerChangeStat, 0, -1); got != nil {
		t.Errorf("empty provider: got %v, want nil", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestGetByTriggerSpecific" -v
```

Expected: FAIL at build with `p.GetByTriggerSpecific undefined` for all six call sites.

- [ ] **Step 3: Add `Provider.GetByTriggerSpecific`.** Open `pkg/script/provider.go`. Find the existing `GetByTrigger` method (around lines 108-127). Append immediately after it:

```go
// GetByTriggerSpecific returns the script for a single tier without the
// 3-level fallback that GetByTrigger does. Caller picks which tier by
// passing -1 for the others:
//   - typeID != -1: returns only the type-specific lookup (no fallback)
//   - else categoryID != -1: returns only the category lookup
//   - else: returns the global lookup
//
// Returns nil if the chosen tier has no registered script. Matches TS
// ScriptProvider.getByTriggerSpecific (ScriptProvider.ts:147-154).
//
// Used by Player.advanceStat to enforce the contract that
// [advancestat,<skill>] scripts must skill-key — a global [advancestat,_]
// script would fire on every stat advance regardless of which skill,
// which is almost certainly a bug not a feature. ChangeStat keeps the
// 3-tier GetByTrigger fallback because "any stat changed" handlers
// (combat-level recompute, regen) are meaningful.
func (p *Provider) GetByTriggerSpecific(trigger ServerTriggerType, typeID, categoryID int) *ScriptFile {
	if typeID != -1 {
		return p.byKey[uint32(trigger)|(0x2<<8)|(uint32(typeID)<<10)]
	}
	if categoryID != -1 {
		return p.byKey[uint32(trigger)|(0x1<<8)|(uint32(categoryID)<<10)]
	}
	return p.byKey[uint32(trigger)]
}
```

Map indexing returns nil for missing keys — same observable behavior as TS's optional-chained `Map.get(...)` returning undefined.

- [ ] **Step 4: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestGetByTriggerSpecific" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

All 6 new tests PASS. Full package green.

- [ ] **Step 5: Build full repo + quality checks.** No consumer changes; build stays green.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/script/
```

All clean. `gofmt -l` empty for `pkg/script/provider.go` and `pkg/script/provider_test.go`.

- [ ] **Step 6: Commit.**

```bash
git add pkg/script/provider.go pkg/script/provider_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): Provider.GetByTriggerSpecific (single-tier lookup)

Ports TS ScriptProvider.getByTriggerSpecific (ScriptProvider.ts:
147-154). Returns the script from a single tier (type / category /
global, picked by caller via -1 sentinels) with NO fallback —
distinct from GetByTrigger which cascades through all three.

The "no fallback" semantic is the design point: callers like
Player.advanceStat enforce the contract that [advancestat,<skill>]
scripts must skill-key. A global [advancestat,_] would fire on every
stat advance regardless of which skill — almost certainly a bug, not
a feature.

Six tier-isolation tests pin the semantic, including the load-bearing
TestGetByTriggerSpecificNoFallback that differentiates from GetByTrigger.

Pure infrastructure for S6i Task 2, which uses this method to dispatch
the AdvanceStat trigger from Player.AddXP's level-up branch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + green build/tests + gofmt clean.

---

## Task 2: `Player.advanceStat` + AddXP wiring + doc refresh + 4 tests

**Files:**
- Modify: `modules/world/player_script.go` (new `Player.advanceStat(stat int)` helper + AddXP one-line addition + AddXP doc refresh)
- Modify: `modules/world/player_script_test.go` (4 new tests)

- [ ] **Step 1: Write the failing tests FIRST.** Open `modules/world/player_script_test.go`. Append at the end:

```go
func TestAddXPFiresAdvanceStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [advancestat,attack=0] at the type-specific lookup key.
	key := uint32(script.TriggerAdvanceStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	sf := &script.ScriptFile{
		Name:      "[advancestat,attack]",
		LookupKey: key,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3

	if len(p.queue) != before+1 {
		t.Fatalf("queue len: got %d, want %d (+1 advancestat)", len(p.queue), before+1)
	}
	req := p.queue[before]
	if req.Script != sf {
		t.Errorf("queue[%d].Script: got %v, want [advancestat,attack] (%v)", before, req.Script, sf)
	}
}

func TestAddXPDoesNotFireAdvanceStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := uint32(script.TriggerAdvanceStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[advancestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 100) // → 200, still level 1

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (no level-up = no advancestat fire)",
			len(p.queue), before)
	}
}

func TestAddXPAdvanceStatNoFallbackToGlobal(t *testing.T) {
	// Register a GLOBAL [advancestat,_] script. AdvanceStat uses
	// GetByTriggerSpecific which does NOT fall back, so the global script
	// should NOT fire on a per-skill level-up.
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalKey := uint32(script.TriggerAdvanceStat)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[advancestat,_]", LookupKey: globalKey})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (global script must NOT fire — advancestat is type-specific only)",
			len(p.queue), before)
	}
	// Verify level-up math still happened.
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3 (level-up math independent of advancestat fire)",
			p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPFiresBothChangeAndAdvanceStatOnLevelUp(t *testing.T) {
	// Both triggers should enqueue when both scripts are registered.
	// Validates that S6h's changeStat and S6i's advanceStat coexist
	// AND that they fire in TS order (changeStat first, advanceStat second).
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	changeKey := uint32(script.TriggerChangeStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	advKey := uint32(script.TriggerAdvanceStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	changeSF := &script.ScriptFile{Name: "[changestat,attack]", LookupKey: changeKey}
	advSF := &script.ScriptFile{Name: "[advancestat,attack]", LookupKey: advKey}
	s.scriptProvider.Register(changeSF)
	s.scriptProvider.Register(advSF)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up

	if len(p.queue) != before+2 {
		t.Fatalf("queue len: got %d, want %d (+2 — both changestat and advancestat)",
			len(p.queue), before+2)
	}
	// Order: changeStat before advanceStat (matches TS Player.ts:1772, 1804).
	if p.queue[before].Script != changeSF {
		t.Errorf("queue[%d].Script: got %v, want changestat first", before, p.queue[before].Script)
	}
	if p.queue[before+1].Script != advSF {
		t.Errorf("queue[%d].Script: got %v, want advancestat second", before+1, p.queue[before+1].Script)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP(FiresAdvanceStat|DoesNotFireAdvanceStat|AdvanceStatNoFallback|FiresBothChangeAndAdvanceStat)" -v
```

Expected: FAIL — the tests assert on queue entries that don't exist yet (AddXP doesn't call advanceStat). The "doesn't fire" and "no fallback" tests may coincidentally pass since no enqueue happens without the helper being called; the other two will fail.

- [ ] **Step 3: Add `Player.advanceStat` helper.** Open `modules/world/player_script.go`. Find the existing `Player.changeStat` helper (added in S6h, search for `func (p *Player) changeStat`). Insert IMMEDIATELY AFTER `changeStat`:

```go
// advanceStat fires the [advancestat,<skill>] trigger for the given stat
// slot when a cache script is registered for that exact stat. Unlike
// changeStat (which uses the 3-level fallback via GetByTrigger), this
// uses GetByTriggerSpecific — type-specific only, no category or global
// fallback. A global [advancestat,_] script would be wrong here: cache
// scripts that say "Congratulations, you just advanced an Attack level!"
// must be skill-keyed.
//
// Enqueued as QueueNormal so it runs asynchronously through
// processPlayerQueue. Matches TS Player.ts:1804-1807 exactly.
//
// Silent no-op if no specific script is registered (GetByTriggerSpecific
// returns nil → EnqueueScriptFile's nil-check short-circuits). Called
// from AddXP's level-up branch after changeStat.
func (p *Player) advanceStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTriggerSpecific(script.TriggerAdvanceStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
}
```

- [ ] **Step 4: Wire `advanceStat` into `Player.AddXP`.** In the same file, find the existing level-up block in `AddXP`. Currently:

```go
	if afterBase > beforeBase {
		// Level-up: fire the [changestat,<skill>] trigger if registered.
		// Matches TS Player.ts:1772.
		p.changeStat(id)
	}
}
```

Replace with:

```go
	if afterBase > beforeBase {
		// Level-up: fire [changestat,<skill>] then [advancestat,<skill>]
		// triggers if registered. Matches TS Player.ts:1772, 1804-1807.
		p.changeStat(id)
		p.advanceStat(id)
	}
}
```

- [ ] **Step 5: Refresh the AddXP doc comment.** In the same file, find the doc block above `Player.AddXP`. The S6h-era line currently reads:

```
// On level-up (baseLevels increases), fires the [changestat,<skill>] trigger
// via changeStat — matches TS Player.ts:1772. Does NOT recompute combat
// level (future combat sub-spec).
```

Replace with:

```
// On level-up (baseLevels increases), fires the [changestat,<skill>] trigger
// via changeStat (TS Player.ts:1772) then the [advancestat,<skill>] trigger
// via advanceStat (TS Player.ts:1804-1807). Does NOT recompute combat
// level (future combat sub-spec).
```

- [ ] **Step 6: Run new advanceStat tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP(FiresAdvanceStat|DoesNotFireAdvanceStat|AdvanceStatNoFallback|FiresBothChangeAndAdvanceStat)" -v
```

All 4 PASS.

- [ ] **Step 7: Verify the full AddXP test suite still passes (S6g + S6h + S6i together).**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestStatAdvanceViaScript -v
```

All pass. `TestStatAdvanceViaScript` adds 50 XP to stats=100; total 150 < 830 (level-2 threshold), so `afterBase > beforeBase` is false and neither helper fires. No interference with the assertion `stats[3] == 150`.

- [ ] **Step 8: Run full repo + quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All PASS / clean. `gofmt -l` empty for files you touched.

- [ ] **Step 9: Commit.**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S6i Player.AddXP fires [advancestat,<skill>] on level-up

Ports TS Player.advanceStat dispatch (Player.ts:1804-1807). New
unexported Player.advanceStat(stat int) helper does
GetByTriggerSpecific(TriggerAdvanceStat, stat, -1) and enqueues via
EnqueueScriptFile as QueueNormal. Called from Player.AddXP inside
the afterBase > beforeBase block AFTER changeStat (TS order).

Key TS-faithful design: AdvanceStat uses GetByTriggerSpecific (no
fallback) while ChangeStat uses GetByTrigger (3-tier fallback). A
global [advancestat,_] script would fire on every stat advance
regardless of which skill — almost certainly a bug, not a feature.
TestAddXPAdvanceStatNoFallbackToGlobal pins this design intent.

Four new tests:
- TestAddXPFiresAdvanceStatOnLevelUp
- TestAddXPDoesNotFireAdvanceStatWithoutLevelUp
- TestAddXPAdvanceStatNoFallbackToGlobal
- TestAddXPFiresBothChangeAndAdvanceStatOnLevelUp (integration; pins
  TS fire order — changeStat before advanceStat)

AddXP doc-comment refreshed to mention both triggers fire.

Closes the S6h final-review follow-up #2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + all 4 new tests pass + full repo green + race + vet + gofmt clean.

---

## Self-Review Checklist

After both tasks complete:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/script/ modules/world/` empty (or only pre-existing drift)
- [ ] Two commits on main: T1 `feat(script): Provider.GetByTriggerSpecific`, T2 `feat(world): S6i Player.AddXP fires [advancestat,<skill>]`
- [ ] Spec coverage:
  - [ ] `Provider.GetByTriggerSpecific` with first-non-(-1)-tier-wins semantic → T1
  - [ ] 6 tier-isolation tests pinning type-only / category-only / global-only / no-fallback / type-shorts-cat / missing → T1
  - [ ] `Player.advanceStat(stat int)` helper using GetByTriggerSpecific → T2
  - [ ] `Player.AddXP` calls `advanceStat(id)` after `changeStat(id)` in level-up block → T2
  - [ ] AddXP doc comment refreshed to mention both triggers fire → T2
  - [ ] 3 advanceStat-only behavior tests + 1 integration test verifying TS fire order → T2
  - [ ] Existing regression tests (TestStatAdvanceViaScript, S6g/S6h test suites) still pass → both tasks
