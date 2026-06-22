# NAI-8 huntPlayers Variant Fill — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace NAI-7's `huntPlayers` stub with a filter pipeline that iterates the player grid in `huntRange`, applies range + level + `checkAfk` filters, and returns the candidate `[]entity`. Close out the misread NAI-2 follow-up and rescope the NAI-7 observer-counting follow-up in memory.

**Architecture:** Single task. Replace stub body with filter-pipeline implementation. Add 4 unit tests exercising range, level, AFK, and empty-grid paths. Update `nai_followups.md` with 3 edits: close misread NAI-2 item, rescope NAI-7 observer item to NAI-9, add NAI-8 deferred-filters section.

**Tech Stack:** Go 1.26+, existing `pkg/grid.NearbyPlayers`, existing `*Player.IsZonesAfk` (`modules/world/afkzone.go:57`), existing `entity` interface (`modules/world/movement_consts.go:45`).

**Spec:** `docs/superpowers/specs/2026-04-22-nai-8-hunt-players-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

---

## File Structure

**Modified:**
- `modules/world/npc_hunt.go` — replace `huntPlayers` stub body (~1 line → ~45 lines)
- `modules/world/npc_event_queue_test.go` — add 1 test-helper + 4 unit tests (~130 lines)
- `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — 3 edits

Single task closes NAI-8.

---

## Task 1: huntPlayers filter pipeline + 4 tests + memory updates — closes NAI-8

**Files:**
- Modify: `modules/world/npc_hunt.go` (replace `huntPlayers` stub body)
- Modify: `modules/world/npc_event_queue_test.go` (test-helper + 4 tests)
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (3 edits)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_event_queue_test.go`:

```go
// addPlayerToServer seeds s.players[slot] + s.grid with a minimal
// *Player at the given coords. Used by NAI-8 huntPlayers tests.
// Slot 0 is reserved per existing convention.
func addPlayerToServer(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	if s.grid == nil {
		s.grid = grid.New()
	}
	p := &Player{
		slot:  slot,
		x:     x,
		z:     z,
		level: level,
	}
	s.players[slot] = p
	s.grid.Add(slot, x, z, level)
	return p
}

func TestHuntPlayersInRange(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	pInRange := addPlayerToServer(t, s, 1, n.x+3, n.z+3, n.level)
	_ = addPlayerToServer(t, s, 2, n.x+20, n.z+20, n.level) // out of range

	hunt := &objtype.HuntType{}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d players, want 1 (in-range only)", len(hunted))
	}
	if hunted[0].Slot() != pInRange.slot {
		t.Errorf("hunted[0]: got slot %d, want slot %d", hunted[0].Slot(), pInRange.slot)
	}
}

func TestHuntPlayersFiltersByLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	pSameLevel := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
	_ = addPlayerToServer(t, s, 2, n.x+2, n.z+2, n.level+1) // wrong level

	hunt := &objtype.HuntType{}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (same-level only)", len(hunted))
	}
	if hunted[0].Slot() != pSameLevel.slot {
		t.Errorf("hunted[0]: got slot %d, want slot %d", hunted[0].Slot(), pSameLevel.slot)
	}
}

func TestHuntPlayersSkipsAfkZonedPlayers(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	pActive := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
	pAfk := addPlayerToServer(t, s, 2, n.x+3, n.z+3, n.level)
	pAfk.lastAfkZone = 1000 // IsZonesAfk saturates at 1000

	// With CheckAfk=true, AFK player is filtered.
	huntWithAfk := &objtype.HuntType{CheckAfk: true}
	hunted := n.huntPlayers(s, huntWithAfk)
	if len(hunted) != 1 {
		t.Fatalf("CheckAfk=true: got %d, want 1 (AFK filtered)", len(hunted))
	}
	if hunted[0].Slot() != pActive.slot {
		t.Errorf("CheckAfk=true: got slot %d, want slot %d (active)", hunted[0].Slot(), pActive.slot)
	}

	// With CheckAfk=false, both players returned.
	huntNoAfk := &objtype.HuntType{CheckAfk: false}
	hunted = n.huntPlayers(s, huntNoAfk)
	if len(hunted) != 2 {
		t.Errorf("CheckAfk=false: got %d, want 2 (filter inactive, both returned)", len(hunted))
	}
}

func TestHuntPlayersReturnsEmptyWhenNoCandidates(t *testing.T) {
	s := newServerForScriptTest(t)
	s.grid = grid.New()
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	hunt := &objtype.HuntType{}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (empty grid)", len(hunted))
	}
}
```

**Test file imports:** `modules/world/npc_event_queue_test.go` already imports `objtype` and `script`. Add `"github.com/zsrv/goscape/pkg/grid"` to the import block if not already present (check before adding — might be there already).

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayers' -v
```

Expected: tests PASS (the stub returns `nil`, so empty-grid + no-candidates tests pass trivially; in-range, level-filter, and AFK tests FAIL on `len(hunted) != 1`). Or in the AFK test's second assertion they FAIL on `!= 2`. At minimum `TestHuntPlayersInRange` and `TestHuntPlayersFiltersByLevel` must FAIL.

- [ ] **Step 3: Replace `huntPlayers` stub body**

In `modules/world/npc_hunt.go`, locate the existing stub:

```go
// huntPlayers is stubbed at NAI-7; NAI-8 fills the body.
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

Replace with the full body:

```go
// huntPlayers iterates the player grid in huntRange and returns
// players passing the filter chain. Matches TS Npc.huntPlayers at
// Engine-TS/.../Npc.ts:921-973.
//
// Filter coverage (NAI-8):
//   - Range + level match: always
//   - checkAfk: via p.IsZonesAfk (TS:935-937)
//
// Filters DEFERRED to future audit pass (Go infrastructure
// missing; each TS line cited):
//   - checkNotBusy (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong (TS:939-941)  — wilderness + combat
//   - checkNotCombat (TS:943-945)     — varp+8-tick window
//   - checkNotCombatSelf (TS:946-948) — varp+8-tick window
//   - checkVars (TS:950-957)          — varp condition chain
//   - checkInv (TS:959-969)           — inventory queries
//
// NAI-8 dispatches NO scripts. TS huntPlayers is a config-driven
// filter pipeline, not a script runner.
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil {
		return nil
	}
	// TS HuntIterator zone-radius formula at ScriptIterators.ts:57:
	// radius = (1 + distance/8) | 0.
	zoneRadius := 1 + n.huntRange/8
	slots := s.grid.NearbyPlayers(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, slot := range slots {
		if slot < 0 || slot >= len(s.players) {
			continue
		}
		p := s.players[slot]
		if p == nil {
			continue
		}
		if p.level != n.level {
			continue
		}
		dx := p.x - n.x
		if dx < 0 {
			dx = -dx
		}
		dz := p.z - n.z
		if dz < 0 {
			dz = -dz
		}
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// checkAfk (TS:935-937): filter players who've gone AFK
		// (1000-tick same-zone threshold).
		if hunt.CheckAfk && p.IsZonesAfk() {
			continue
		}
		hunted = append(hunted, p)
	}
	return hunted
}
```

No new imports needed in `npc_hunt.go` — `objtype` is already imported.

- [ ] **Step 4: Run tests to verify they pass + full suite + race**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayers' -v -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all 4 new tests PASS; full world suite PASS (no prior test should regress — huntPlayers had no consumer in NAI-7 other than the stub); race suite clean.

- [ ] **Step 5: Update `nai_followups.md` memory with 3 edits**

Open `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`.

**Edit 1 — Close the misread NAI-2 follow-up.** Find the section titled `### NAI-8 prerequisite: protected-pointer cleanup in resumeOrFinishNpc`. Replace the entire body (everything between that header and the next `###` section) with:

```
**Resolved 2026-04-22 (closed as misread):** The premise of this
follow-up was that NAI-8's huntPlayers dispatches player-anchored
scripts that could suspend, requiring protected-pointer cleanup on
resume. Re-reading TS Npc.huntPlayers at Engine-TS/.../Npc.ts:921-973
shows it's a config-driven filter pipeline (checkNotBusy, checkAfk,
checkNotTooStrong, checkNotCombat, checkVars, checkInv) with zero
script dispatch. No protected-pointer concern. If some future
sub-spec does route player-anchored scripts through NPC suspension,
file a fresh follow-up then with accurate scope.
```

**Edit 2 — Rescope the NAI-7 observer-counting follow-up.** Find the section titled `### NAI-8 blocker: real observer counting for PAUSEHUNT gate`. Update the section title and body to reflect that NAI-8 is unaffected (PLAYER-type hunts exempt from PAUSEHUNT per TS Npc.ts:162), and the real blocker is NAI-9.

Change the heading from:
```
### NAI-8 blocker: real observer counting for PAUSEHUNT gate
```

To:
```
### NAI-9 blocker: real observer counting for PAUSEHUNT gate
```

Replace the "Impact today" and "NAI-8 owns the fix" paragraphs. Find:

```
**NAI-8 owns the fix.** Before huntPlayers grid-iterates, NAI-8 must
```

And change that line to:

```
**NAI-9 owns the fix.** Before huntNpcs/huntObjs/huntLocs grid-
iterate for PAUSEHUNT-configured HuntTypes, NAI-9 must
```

(The rest of the fix-remediation body remains the same — two options: rsbuf API addition or grid iteration.)

Also update the reasoning paragraph. Find:

```
**Impact when NAI-8/9 fill variants:** PAUSEHUNT NPC/Obj/Loc hunts
```

Change to:

```
**Impact when NAI-9 fills variants:** PAUSEHUNT NPC/Obj/Loc hunts
```

The prior "**Impact today**" paragraph is still accurate (stub's zero-impact). Leave it.

**Edit 3 — Add NAI-8 follow-up section.** Append at the end of the file:

```
## From NAI-8 (2026-04-22)

### Deferred filters in huntPlayers (future audit)

NAI-8 shipped huntPlayers with 3 of 8 TS filters: range, level match,
and checkAfk. The remaining 5 filters are deferred because each
requires Go-side infrastructure that doesn't exist yet. Each has an
inline TODO in `modules/world/npc_hunt.go` pointing at the TS source
line.

Deferred filters (TS Npc.ts line refs):

1. **checkNotBusy (TS:931-933)** — needs a `Player.Busy()` method
   equivalent. TS `player.busy()` checks active script, open modal,
   pending interaction, etc. Port when those Player-state fields
   land.

2. **checkNotTooStrong (TS:939-941)** — needs wilderness detection
   (map-data query: "is this coord in the wilderness zone?") AND
   access to `NpcType.VisLevel` at filter-evaluation time. VisLevel
   is already loaded; wilderness detection needs a map-metadata
   lookup.

3. **checkNotCombat (TS:943-945)** — needs a Varp read with an
   8-tick combat-window comparison against `World.currentTick`.
   Varp read infrastructure exists (S5b); the combat-window gate
   needs tracking when the active player last entered combat.

4. **checkNotCombatSelf (TS:946-948)** — same as checkNotCombat but
   on the NPC side (`this.getVar(hunt.checkNotCombatSelf)`). Needs
   per-NPC varn read (varns infrastructure exists from S6-era).

5. **checkVars (TS:950-957)** — loops hunt's CheckVars slice
   (`[]HuntCheckVar{VarID, Condition, Val}`), evaluating each via
   `hunt.checkHuntCondition(playerVarp, op, val)`. The condition
   evaluator is a simple switch on `>`, `<`, `=`, `!` — implementable
   today but defers pending a real script-content need.

6. **checkInv (TS:959-969)** — needs `Player.InvTotal(inv, obj)` and
   `Player.InvTotalParam(inv, param)` methods. Inventory infrastructure
   exists from S5e; these specific aggregation queries may need
   new methods.

Approach when filling: add each filter incrementally in dedicated
follow-up sub-specs OR a single audit pass. Each filter is
independently testable (mock Player methods, add HuntType config,
assert pass/fail). Keep the filter-ordering per TS (checkNotBusy →
checkAfk → checkNotTooStrong → target+multi check → checkVars →
checkInv).
```

- [ ] **Step 6: Grep sanity check**

Run:
```
rg -n '\bhuntPlayers\b|\bNAI-8\b' modules/ pkg/ docs/superpowers/
```

Expected:
- `modules/world/npc_hunt.go` contains the `huntPlayers` body (no longer a stub)
- `modules/world/npc_event_queue_test.go` contains 4 test functions + `addPlayerToServer` helper
- `modules/world/npc_ai.go` has no direct mention of huntPlayers (turn() reaches it only via huntAll for PLAYER-type, which turn() already skips)
- NAI-8 references in spec/plan docs

Report match count. No stray references expected.

- [ ] **Step 7: Commit, closing NAI-8**

```bash
git add modules/world/npc_hunt.go modules/world/npc_event_queue_test.go $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-8 huntPlayers filter pipeline — closes NAI-8

Replace NAI-7's huntPlayers stub with a filter pipeline porting TS
Npc.huntPlayers (Npc.ts:921-973). Implements the 3 filters whose
Go infrastructure exists:
  - Range check (chebyshev distance via dx/dz ≤ huntRange)
  - Level match (p.level == n.level)
  - checkAfk via p.IsZonesAfk (TS:935-937)

Defers 5 other filters (checkNotBusy, checkNotTooStrong,
checkNotCombat/Self, checkVars, checkInv) with inline TODO + TS
line refs. Each needs Go-side infrastructure (Player.Busy,
wilderness detection, varp+8-tick window, inventory queries) that
doesn't exist yet.

Zone-radius formula `1 + huntRange/8` ports TS HuntIterator
(ScriptIterators.ts:57). Chebyshev distance + level equality match
TS HuntIterator's entity-level filter.

Memory updates in nai_followups:
  1. Close the misread NAI-2 follow-up (no protected-pointer
     cleanup needed — huntPlayers dispatches no scripts).
  2. Rescope NAI-7 observer-counting item from NAI-8 to NAI-9
     blocker (PLAYER-type hunts exempt from PAUSEHUNT gate per
     TS Npc.ts:162).
  3. Add NAI-8 follow-up section listing the 5 deferred filters
     with remediation notes.

Four-test strategy: in-range filter, level mismatch filter, AFK
filter (both CheckAfk=true and =false assertions), empty-grid.

huntPlayers is still unreachable from Npc.turn() (turn() skips
huntAll for PLAYER types per TS Npc.ts:164 + Go npc_hunt.go:47);
observable behavior lands when OpNpcHuntAll (opcode 2526) is
wired by a future sub-spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist results

**1. Spec coverage:**

| Spec section | Task |
|---|---|
| Replace `huntPlayers` stub body with filter pipeline | Task 1 Step 3 |
| 4 unit tests (range, level, AFK, empty) | Task 1 Step 1 |
| Memory: close misread NAI-2 follow-up | Task 1 Step 5 (Edit 1) |
| Memory: rescope NAI-7 observer item to NAI-9 | Task 1 Step 5 (Edit 2) |
| Memory: add NAI-8 follow-up section | Task 1 Step 5 (Edit 3) |

All spec items covered.

**2. Placeholder scan:** No TBDs/TODOs in plan steps. The code includes inline TODO comments for 5 deferred filters — that's legitimate code-level documentation pointing at TS line refs, not a plan placeholder.

**3. Type consistency:** `huntPlayers(s *Server, hunt *objtype.HuntType) []entity` matches NAI-7's stub signature + `huntAll` caller expectation. `addPlayerToServer(t, s, slot, x, z, level) *Player` test helper signature is self-contained. `entity` interface (Slot + Coords) consistent with NAI-7 usage.

---

## Commit trail (for reference)

One commit closes NAI-8:

1. `feat(world): NAI-8 huntPlayers filter pipeline — closes NAI-8`

Task leaves the tree green; the commit closes NAI-8 and updates memory to reflect scope corrections.
