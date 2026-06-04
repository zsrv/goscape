# NAI-23 Follow-up Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Activate the two remaining huntPlayers deferred filters (checkNotBusy, checkNotTooStrong) — completing the NAI-8 deferred filter list — audit NumberNotNull wrapping across three opcode-handler files, and mark three stale tracker entries Resolved.

**Architecture:** Four bundles. Bundle 1 is pure memory-file housekeeping (no git commit; absorbed into the NAI-23 close commit's `Closes memory:` trailer). Bundles 2 and 3 add Player query methods (`Busy()`, `IsInWilderness()`) and wire them into `(*Npc).huntPlayers` as new filter guards. Bundle 4 dispatches one implementer subagent per handler file (Tasks 4a/4b/4c) for parallel TDD audit-passes; each per-file commit lands as `feat`. Two-stage review (code quality + TS fidelity) runs after Bundle 3 completes for Bundles 2 and 3, and after each of 4a/4b/4c for Bundle 4.

**Tech Stack:** Go 1.26+. Existing test scaffolding (`Init`/`Execute`/`mockNpc` in `pkg/script/`; `newTestPlayer(t)` at `modules/world/player_test.go:14`; existing huntPlayers integration patterns at `npc_hunt_test.go`). No new dependencies.

**Reference**: `docs/superpowers/specs/2026-04-25-nai-23-followup-bundle-design.md` (committed at `ce3fd23`).

---

## Task 1 — Bundle 1: Tracker hygiene (memory-only)

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (3 entries marked Resolved)
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/spec_followup_tracker_freshness.md` (append addendum)

**Cadence:** Compressed — no separate test step, no per-task commit. Edits stage operationally before Bundle 2 begins; the close commit's `Closes memory:` trailer cites all three entries.

- [ ] **Step 1: Read entry context for `nai_followups.md:786`**

Run: `Read /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` with offset=786, limit=10.
Expected: First line is `### Stale `*Npc.typ` snapshot after changetype (newly observable post-NAI-18)`. Confirm the body starts with ``(*Npc).changeTypeImpl` at `modules/world/npc_masks.go:53-74` updates`.

- [ ] **Step 2: Mark entry at `nai_followups.md:786` Resolved**

Edit:

`old_string` (the heading + first line of body):
```
### Stale `*Npc.typ` snapshot after changetype (newly observable post-NAI-18)

`(*Npc).changeTypeImpl` at `modules/world/npc_masks.go:53-74` updates
```

`new_string`:
```
### Stale `*Npc.typ` snapshot after changetype (newly observable post-NAI-18)

**Resolved 2026-04-24 (NAI-19 Task 3, commit `8e94b29`).** `n.lookupType(newType)` is
now lifted outside the `if reset` block in `(*Npc).changeTypeImpl` and unconditionally
assigned to `n.typ` (`modules/world/npc_masks.go:68-69`). Both CHANGETYPE and KEEPALL
paths refresh the snapshot; `n.typ.X` reads after a changetype are no longer stale.
This closes the staleness bug NAI-18 made newly observable via `inApproachDistance`'s
`n.typ.Size` read.

---

_Original deferral body (preserved for historical context):_

`(*Npc).changeTypeImpl` at `modules/world/npc_masks.go:53-74` updates
```

- [ ] **Step 3: Read entry context for `nai_followups.md:1272`**

Run: `Read /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` with offset=1272, limit=10.
Expected: First line is `### Promote `n.size` snapshot to LoS-path reads (`inApproachDistance`, `approachEntitySize`)`.

- [ ] **Step 4: Mark entry at `nai_followups.md:1272` Resolved**

Edit:

`old_string`:
```
### Promote `n.size` snapshot to LoS-path reads (`inApproachDistance`, `approachEntitySize`)

NAI-20 Task 2 introduced `*Npc.size` and `*Npc.blockWalk` snapshot fields
```

`new_string`:
```
### Promote `n.size` snapshot to LoS-path reads (`inApproachDistance`, `approachEntitySize`)

**Resolved 2026-04-25 (NAI-21 Bundle 1, commit `ed2f432`).** Both call sites now read
the snapshot: `(*Npc).inApproachDistance` reads `n.size` for `selfSize`
(`modules/world/npc_interaction.go:581`); `approachEntitySize` reads `t.size` for the
`*Npc` branch (`modules/world/npc_interaction.go:532`). Two regression tests landed:
`TestInApproachDistanceUsesSelfSizeSnapshotNotTyp` and
`TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp` (dual-pin per `ts_asymmetry_dual_pin`
memory).

---

_Original deferral body (preserved for historical context):_

NAI-20 Task 2 introduced `*Npc.size` and `*Npc.blockWalk` snapshot fields
```

- [ ] **Step 5: Read entry context for `nai_followups.md:244`**

Run: `Read /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` with offset=244, limit=10.
Expected: First line is `### Scope note: `npc_changetype` duration wiring status`.

- [ ] **Step 6: Mark cross-ref at `nai_followups.md:244` Resolved**

Edit:

`old_string`:
```
### Scope note: `npc_changetype` duration wiring status

NAI-5 originally suggested folding this into NAI-7 (since NAI-7 touches
the ActiveNpc interface). NAI-7 explicitly punted (spec non-goal #6).
The ChangeType(newType, duration) signature change is now unassigned.
See "From NAI-5" entry above for the full remediation plan.
```

`new_string`:
```
### Scope note: `npc_changetype` duration wiring status

**Resolved 2026-04-23 (NAI-16 Task 2)** — see "From NAI-5" entry above for the
full closure attribution. Original scope note preserved below for historical
context.

---

_Original scope note (preserved for historical context):_

NAI-5 originally suggested folding this into NAI-7 (since NAI-7 touches
the ActiveNpc interface). NAI-7 explicitly punted (spec non-goal #6).
The ChangeType(newType, duration) signature change is now unassigned.
See "From NAI-5" entry above for the full remediation plan.
```

- [ ] **Step 7: Append addendum to `spec_followup_tracker_freshness.md`**

Read the current file end to find the append point.
Run: `Read /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/spec_followup_tracker_freshness.md`.

Edit by appending after the existing "Spec-write checklist addition:" block. New text to append (after the existing final list item):

```

**Triggered also by:** NAI-23 spec-write (2026-04-25) caught two additional stale
primary entries — `nai_followups.md:786` (`Stale *Npc.typ snapshot after
changetype`, resolved by NAI-19 Task 3 commit `8e94b29`) and `nai_followups.md:1272`
(`Promote n.size snapshot to LoS-path reads`, resolved by NAI-21 Bundle 1 commit
`ed2f432`) — plus one stale cross-ref at `:244`. Three stale entries discovered
in a single spec-write pre-flight pass confirms the pattern: NAI-N close commits
routinely resolve the work without updating the corresponding tracker entry.
Suggests NAI-N close commits should explicitly include tracker-entry updates as
a side-action, complementing the `close_commit_memory_trailer` git-trailer
discipline.
```

- [ ] **Step 8: Verify edits**

Re-read each edited region to confirm:
- `nai_followups.md` near :786 — Resolved marker visible with NAI-19 attribution.
- `nai_followups.md` near :1272 — Resolved marker visible with NAI-21 attribution.
- `nai_followups.md` near :244 — Resolved marker visible with NAI-16 cross-ref.
- `spec_followup_tracker_freshness.md` end — addendum visible.

- [ ] **Step 9: NO commit (memory-only). Move to Task 2.**

The close commit at NAI-23 close will include `Closes memory:` trailer citing the three entries.

---

## Task 2 — Bundle 2: `Player.Busy()` aggregator + `checkNotBusy` huntPlayers filter

**Files:**
- Modify: `modules/world/player.go` (add `Busy()` method)
- Modify: `modules/world/npc_hunt.go` (insert filter; update doc-comment)
- Test: `modules/world/player_test.go` (add `Busy()` unit tests)
- Test: `modules/world/npc_hunt_test.go` (add filter integration tests)

**TDD plan:** Write all unit tests for `Busy()`, run to verify they fail with "undefined", implement `Busy()`, run to verify pass; then write integration tests for the filter, run to verify fail (filter not yet wired), wire the filter, run to verify pass.

- [ ] **Step 1: Read context**

Read `modules/world/player.go:25-180` (around modal-state constants and the `delayed`/`modalState` fields) to find good `Busy()` placement.
Read `modules/world/npc_hunt.go:85-145` to confirm filter-coverage doc-comment block and the line-138 insertion point.
Read `modules/world/npc_hunt_test.go:777-900` to study the existing CheckInv test fixture pattern (which we'll mirror for the new filter integration tests).

- [ ] **Step 2: Write failing `Busy()` unit tests in `modules/world/player_test.go`**

Append to the file (after existing tests; placement near other simple-state-predicate tests if any):

```go
func TestPlayerBusyNotDelayedNoModal(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.Busy() {
		t.Error("Busy: got true, want false (fresh player has neither delayed nor modal)")
	}
}

func TestPlayerBusyDelayedOnly(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true
	if !p.Busy() {
		t.Error("Busy: got false, want true (delayed=true)")
	}
}

func TestPlayerBusyModalMainOnly(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	if !p.Busy() {
		t.Error("Busy: got false, want true (modalStateMain set)")
	}
}

func TestPlayerBusyModalChatOnly(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	if !p.Busy() {
		t.Error("Busy: got false, want true (modalStateChat set)")
	}
}

func TestPlayerBusyModalSideOnlyNotBusy(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateSide
	if p.Busy() {
		t.Error("Busy: got true, want false (modalStateSide alone — TS containsModalInterface excludes side per Player.ts:796-799)")
	}
}

func TestPlayerBusyDelayedAndModalCombined(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true
	p.modalState = modalStateMain
	if !p.Busy() {
		t.Error("Busy: got false, want true (both delayed and modal)")
	}
}
```

- [ ] **Step 3: Run tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "^TestPlayerBusy" ./modules/world/`
Expected: COMPILE ERROR — `p.Busy undefined (type *Player has no field or method Busy)`.

- [ ] **Step 4: Implement `Busy()` in `modules/world/player.go`**

Pick placement near other simple state predicates (e.g., near `IsZonesAfk()` or after the modal-state-related methods if grouped). If no obvious group exists, add near the end of the file's method block.

```go
// Busy returns true when the player cannot accept new interactions —
// either delayed (suspended by script delay) or has a main/chat modal
// open. Mirrors TS Player.busy() at Engine-TS/.../Player.ts:801-803
// (which composes containsModalInterface at Player.ts:796-799 — the
// SIDE bit is intentionally excluded).
func (p *Player) Busy() bool {
	return p.delayed || p.modalState&(modalStateMain|modalStateChat) != 0
}
```

- [ ] **Step 5: Run unit tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "^TestPlayerBusy" ./modules/world/`
Expected: PASS — 6 tests pass.

- [ ] **Step 6: Write failing filter integration tests in `modules/world/npc_hunt_test.go`**

Mirror the fixture pattern of `TestHuntPlayersCheckInvDisabled` (line 780) and `TestHuntPlayersCheckInvObjPasses` (line 802). Append after the existing CheckInv block.

```go
// TestHuntPlayersCheckNotBusyFiltersBusyPlayer pins NAI-23 Bundle 2: when
// hunt.CheckNotBusy is true and the candidate player is busy (delayed or
// main/chat modal open), the player is filtered out. Mirrors TS
// Npc.ts:931-933.
func TestHuntPlayersCheckNotBusyFiltersBusyPlayer(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	// Busy player at (3203, 3200).
	pBusy := addPlayerToServer(t, s, 1, 3203, 3200, 0)
	pBusy.delayed = true
	// Fresh player at (3197, 3200).
	pFresh := addPlayerToServer(t, s, 2, 3197, 3200, 0)

	hunt := &objtype.HuntType{
		CheckNotBusy: true,
		CheckAfk:     false, // disabled — keep test isolated to checkNotBusy
		CheckVis:     objtype.HuntVisOff,
		CheckInv:     -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (busy player must be filtered out)", len(hunted))
	}
	if hunted[0] != pFresh {
		t.Errorf("hunted[0]: got busy player, want fresh player")
	}
}

// TestHuntPlayersCheckNotBusyDisabled pins NAI-23 Bundle 2: when
// hunt.CheckNotBusy is false, busy players are NOT filtered (the filter
// is gated on the bool flag).
func TestHuntPlayersCheckNotBusyDisabled(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	// Busy player at (3203, 3200).
	p := addPlayerToServer(t, s, 1, 3203, 3200, 0)
	p.delayed = true

	hunt := &objtype.HuntType{
		CheckNotBusy: false, // explicit — filter disabled
		CheckAfk:     false,
		CheckVis:     objtype.HuntVisOff,
		CheckInv:     -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (filter disabled — busy must pass)", len(hunted))
	}
}
```

If `addPlayerToServer` and `newTestServer` test helpers are not present at HEAD, mirror the existing CheckInv test's fixture-construction pattern verbatim. If the player-construction helper has a different name, use the one already used by the existing tests in the same file.

- [ ] **Step 7: Run integration tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "^TestHuntPlayersCheckNotBusy" ./modules/world/`
Expected: FAIL — `TestHuntPlayersCheckNotBusyFiltersBusyPlayer` fails with `hunted: got 2, want 1` (filter not yet wired; busy player not filtered).

- [ ] **Step 8: Wire the filter in `modules/world/npc_hunt.go`**

Insert immediately before the existing line `// checkAfk (TS:935-937): filter players who've gone AFK` (current line ~138):

```go
		// checkNotBusy (TS:931-933): skip players whose state cannot
		// accept a hunt interaction (delayed or main/chat modal open).
		if hunt.CheckNotBusy && p.Busy() {
			continue
		}
```

Update the filter-coverage doc-comment block (`npc_hunt.go:88-103`):
- Move the `checkNotBusy` line out of "Filters DEFERRED" into "Filter coverage" with the `(NAI-23, TS:931-933)` attribution.

The new "Filter coverage" block should read (only `checkNotBusy` line moved):

```go
// Filter coverage:
//   - Range + level match:     always
//   - checkNotBusy             (NAI-23, TS:931-933)
//   - checkAfk                 (NAI-8,  TS:935-937)
//   - CheckVis LoS/LoW         (NAI-12, TS per ScriptIterators.ts:88-94)
//   - Outer combat guard       (NAI-15, TS:942)
//   - checkNotCombat           (NAI-15, TS:943-945)
//   - checkNotCombatSelf       (NAI-16, TS:946-948)
//   - checkVars                (NAI-15, TS:950-957)
//   - checkInv                 (NAI-22, TS:959-969)
```

The "Filters DEFERRED" block should now contain only `checkNotTooStrong`:

```go
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
```

(Bundle 3 will retire the last DEFERRED entry; the "DEFERRED" comment block will be removed entirely there.)

- [ ] **Step 9: Run all tests in `modules/world/` to verify PASS + no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Expected: PASS — all tests pass including the 6 new `TestPlayerBusy*` and 2 new `TestHuntPlayersCheckNotBusy*`.

- [ ] **Step 10: Commit Bundle 2**

```bash
git add modules/world/player.go modules/world/player_test.go modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-23 Bundle 2 — Player.Busy() + checkNotBusy huntPlayers filter

Add Player.Busy() aggregating the two predicates TS player.busy() checks
(delayed || (modalState & (MAIN|CHAT)) != 0). Wire into huntPlayers'
checkNotBusy filter at the TS-faithful position (between range/level and
checkAfk per Npc.ts:931-933).

Both Busy() predicates already existed in goscape — the tracker entry's
"needs Player.Busy() equivalent" framing dramatized what is a 1-line
aggregator over already-ported state. HuntType.CheckNotBusy bool is
already decoded (hunttype.go:55,116) so live config can opt-in.

8 new tests: 6 Busy() unit tests pinning each predicate combination
(including TestPlayerBusyModalSideOnlyNotBusy which pins TS-fidelity
of the MAIN|CHAT mask vs SIDE), 2 huntPlayers integration tests.

Closes one of two remaining NAI-8 deferred filters (Bundle 3 closes
the last). No deviation tags retired or introduced.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Bundle 3: `Player.IsInWilderness()` + `checkNotTooStrong` huntPlayers filter

**Files:**
- Modify: `modules/world/player.go` (add `IsInWilderness()` method)
- Modify: `modules/world/npc_hunt.go` (insert filter; remove `Filters DEFERRED` block; update filter-coverage doc-comment)
- Test: `modules/world/player_test.go` (add `IsInWilderness()` unit + boundary tests)
- Test: `modules/world/npc_hunt_test.go` (add filter integration tests)

**TDD plan:** Write `IsInWilderness()` unit + boundary tests, verify FAIL, implement, verify PASS; write filter integration tests, verify FAIL, wire filter, verify PASS.

- [ ] **Step 1: Read context**

Read `modules/world/npc_hunt.go:85-200` to find:
- The exact post-NAI-23-Bundle-2 filter-coverage block (Bundle 2 already moved checkNotBusy out).
- Line ~159 ("// Outer combat guard — TS:942") for the new checkNotTooStrong insertion.
- The "Filters DEFERRED" block (currently containing only checkNotTooStrong post-Bundle-2).

- [ ] **Step 2: Write failing `IsInWilderness()` unit tests in `modules/world/player_test.go`**

Append after Bundle 2's `TestPlayerBusy*` tests:

```go
func TestPlayerIsInWildernessSouthRectInside(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z = 3000, 5000 // inside south rect: x in [2944,3392), z in [3520,6400)
	if !p.IsInWilderness() {
		t.Error("IsInWilderness: got false, want true (3000,5000 inside south rect)")
	}
}

func TestPlayerIsInWildernessNorthRectInside(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z = 3000, 11000 // inside north rect: x in [2944,3392), z in [9920,12800)
	if !p.IsInWilderness() {
		t.Error("IsInWilderness: got false, want true (3000,11000 inside north rect)")
	}
}

func TestPlayerIsInWildernessOutsideAllRects(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z = 3000, 3500 // z=3500 below south rect's lower bound 3520
	if p.IsInWilderness() {
		t.Error("IsInWilderness: got true, want false (3000,3500 outside south rect)")
	}
}

// TestPlayerIsInWildernessBoundaries pins TS Player.ts:2082-2090 boundary
// asymmetry: lower-edge inclusive (>=), upper-edge exclusive (<). A
// future "fix" to <= would flip these red.
func TestPlayerIsInWildernessBoundaries(t *testing.T) {
	cases := []struct {
		name string
		x, z int
		want bool
	}{
		{"south_lower_corner_inclusive", 2944, 3520, true},
		{"south_just_below_x_lower", 2943, 5000, false},
		{"south_upper_x_exclusive", 3392, 5000, false},
		{"south_upper_z_exclusive", 3000, 6400, false},
		{"north_lower_z_inclusive", 3000, 9920, true},
		{"north_upper_z_exclusive", 3000, 12800, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newTestPlayer(t)
			p.x, p.z = tc.x, tc.z
			got := p.IsInWilderness()
			if got != tc.want {
				t.Errorf("IsInWilderness(%d,%d): got %v, want %v", tc.x, tc.z, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "^TestPlayerIsInWilderness" ./modules/world/`
Expected: COMPILE ERROR — `p.IsInWilderness undefined`.

- [ ] **Step 4: Implement `IsInWilderness()` in `modules/world/player.go`**

Place adjacent to the `Busy()` method added in Bundle 2:

```go
// IsInWilderness returns true when the player is inside one of the two
// hardcoded wilderness rectangles. Mirrors TS Player.isInWilderness()
// at Engine-TS/.../Player.ts:2082-2090.
//
// South wilderness: x in [2944, 3392), z in [3520, 6400).
// North wilderness: x in [2944, 3392), z in [9920, 12800).
//
// Bounds are inclusive on the lower edge and exclusive on the upper —
// preserve verbatim: `<=` would shift the boundary by one tile vs TS.
func (p *Player) IsInWilderness() bool {
	if p.x >= 2944 && p.x < 3392 && p.z >= 3520 && p.z < 6400 {
		return true
	}
	if p.x >= 2944 && p.x < 3392 && p.z >= 9920 && p.z < 12800 {
		return true
	}
	return false
}
```

- [ ] **Step 5: Run tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "^TestPlayerIsInWilderness" ./modules/world/`
Expected: PASS — 3 standalone tests + 6 boundary sub-tests = 9 PASS.

- [ ] **Step 6: Write failing filter integration tests in `modules/world/npc_hunt_test.go`**

Append after Bundle 2's `TestHuntPlayersCheckNotBusy*` tests. NPC `npcType.VisLevel = 30` and player `combatLevel` values are chosen so the filter triggers exactly when intended.

```go
// TestHuntPlayersCheckNotTooStrongFiltersStrongPlayerOutsideWilderness pins
// NAI-23 Bundle 3: when CheckNotTooStrong is OutsideWilderness AND the
// player is outside wilderness AND combatLevel > vislevel*2, the player
// is filtered. Mirrors TS Npc.ts:939-941.
func TestHuntPlayersCheckNotTooStrongFiltersStrongPlayerOutsideWilderness(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	// Strong player OUTSIDE wilderness (south of south-rect, z=3500 < 3520).
	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 100 // > vislevel*2 = 60

	hunt := &objtype.HuntType{
		CheckNotTooStrong: objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:          false,
		CheckVis:          objtype.HuntVisOff,
		CheckInv:          -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (strong player outside wilderness must be filtered)", len(hunted))
	}
}

// TestHuntPlayersCheckNotTooStrongIgnoresStrongPlayerInWilderness pins
// the wilderness escape clause: the protection is disabled INSIDE the
// wilderness (TS Npc.ts:939: `!player.isInWilderness() && ...`).
func TestHuntPlayersCheckNotTooStrongIgnoresStrongPlayerInWilderness(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3000, 5000, 0, npcType) // NPC inside south wilderness
	n.server = s
	n.huntRange = 5

	// Strong player INSIDE wilderness.
	p := addPlayerToServer(t, s, 1, 3003, 5000, 0)
	p.combatLevel = 100

	hunt := &objtype.HuntType{
		CheckNotTooStrong: objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:          false,
		CheckVis:          objtype.HuntVisOff,
		CheckInv:          -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (filter disabled inside wilderness)", len(hunted))
	}
}

// TestHuntPlayersCheckNotTooStrongAllowsWeakPlayer pins the
// combatLevel > vislevel*2 lower bound: a player at or below 2x vislevel
// passes regardless of wilderness state.
func TestHuntPlayersCheckNotTooStrongAllowsWeakPlayer(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	// Weak player outside wilderness.
	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 50 // NOT > 60

	hunt := &objtype.HuntType{
		CheckNotTooStrong: objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:          false,
		CheckVis:          objtype.HuntVisOff,
		CheckInv:          -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (combatLevel <= vislevel*2 must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckNotTooStrongDisabled pins the off switch: when
// CheckNotTooStrong is Off, the filter is skipped entirely.
func TestHuntPlayersCheckNotTooStrongDisabled(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	// Strong player outside wilderness.
	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 100

	hunt := &objtype.HuntType{
		CheckNotTooStrong: objtype.HuntCheckNotTooStrongOff,
		CheckAfk:          false,
		CheckVis:          objtype.HuntVisOff,
		CheckInv:          -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (filter disabled — strong player must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckNotTooStrongBoundaryComparison pins the strict-`>`
// comparison: combatLevel exactly equal to 2*vislevel passes (TS uses
// `>`, not `>=`). A future `>=` regression would flip this red.
func TestHuntPlayersCheckNotTooStrongBoundaryComparison(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	// combatLevel exactly equal to 2*vislevel = 60.
	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 60

	hunt := &objtype.HuntType{
		CheckNotTooStrong: objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:          false,
		CheckVis:          objtype.HuntVisOff,
		CheckInv:          -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (combatLevel == 2*vislevel must pass; > not >=)", len(hunted))
	}
}
```

- [ ] **Step 7: Run integration tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "^TestHuntPlayersCheckNotTooStrong" ./modules/world/`
Expected: FAIL — `TestHuntPlayersCheckNotTooStrongFiltersStrongPlayerOutsideWilderness` fails (filter not yet wired; strong player not filtered).

- [ ] **Step 8: Wire the filter in `modules/world/npc_hunt.go`**

Insert immediately before the existing line `// Outer combat guard — TS:942.` (current line ~159 post-Bundle-2):

```go
		// checkNotTooStrong (TS:939-941): skip players whose combatLevel
		// is more than 2x the NPC's vislevel when they are OUTSIDE the
		// wilderness (the wilderness disables this protection). Filter
		// only applies when CheckNotTooStrong is OutsideWilderness;
		// Off → filter skipped.
		if hunt.CheckNotTooStrong == objtype.HuntCheckNotTooStrongOutsideWilderness &&
			!p.IsInWilderness() &&
			p.combatLevel > n.typ.VisLevel*2 {
			continue
		}
```

Update the filter-coverage doc-comment block (`npc_hunt.go:88-103`):
- Move the `checkNotTooStrong` line out of "Filters DEFERRED" into "Filter coverage" with the `(NAI-23, TS:939-941)` attribution.
- Remove the "Filters DEFERRED" block entirely (it is now empty — Bundle 3 closes the last deferred filter).
- Add a one-line note `// All NAI-8 deferred filters now ported (NAI-23 closes checkNotBusy + checkNotTooStrong).`

The final filter-coverage block should read:

```go
// Filter coverage:
//   - Range + level match:     always
//   - checkNotBusy             (NAI-23, TS:931-933)
//   - checkAfk                 (NAI-8,  TS:935-937)
//   - CheckVis LoS/LoW         (NAI-12, TS per ScriptIterators.ts:88-94)
//   - checkNotTooStrong        (NAI-23, TS:939-941)
//   - Outer combat guard       (NAI-15, TS:942)
//   - checkNotCombat           (NAI-15, TS:943-945)
//   - checkNotCombatSelf       (NAI-16, TS:946-948)
//   - checkVars                (NAI-15, TS:950-957)
//   - checkInv                 (NAI-22, TS:959-969)
//
// All NAI-8 deferred filters now ported (NAI-23 closes checkNotBusy +
// checkNotTooStrong).
//
// CheckVis (NAI-12) preserves the TS player-as-source / NPC-as-dest
// argument swap quirk — see FIDELITY note at the gate below.
//
// NAI-8 dispatches NO scripts. TS huntPlayers is a config-driven
// filter pipeline, not a script runner.
```

If pre-flight at this step finds `n.typ` could be nil at the call site (test surfaces a nil-deref test path), STOP and escalate to the controller per the spec's "Bundle 3 nil-guard ambiguity" risk; do NOT inline a guard.

- [ ] **Step 9: Run all tests in `modules/world/` to verify PASS + no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Expected: PASS — all tests including 9 new `TestPlayerIsInWilderness*` (with sub-tests) and 5 new `TestHuntPlayersCheckNotTooStrong*`.

- [ ] **Step 10: Commit Bundle 3**

```bash
git add modules/world/player.go modules/world/player_test.go modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-23 Bundle 3 — Player.IsInWilderness() + checkNotTooStrong filter

Add Player.IsInWilderness() with TS-verbatim coord bounds (Player.ts:2082-2090):
south rect x∈[2944,3392) × z∈[3520,6400); north rect x∈[2944,3392) × z∈[9920,12800).
Wire into huntPlayers' checkNotTooStrong filter at the TS-faithful position
(between CheckVis-inlined and outer combat guard per Npc.ts:939-941).

The "needs wilderness detection" framing in the tracker dramatized what
is a 2-rect coord comparison — TS isInWilderness has no map-zone lookup
or metadata dependency. HuntCheckNotTooStrongOutsideWilderness enum and
NpcType.VisLevel both already exist.

11 new tests: 3 IsInWilderness() unit + 6 boundary sub-tests pinning the
TS >=/< asymmetry, 5 huntPlayers integration tests covering the strong-
outside-wilderness, weak-outside-wilderness, strong-inside-wilderness,
filter-disabled, and == 2*vislevel boundary cases.

Closes the last NAI-8 deferred filter — every TS huntPlayers filter is
now ported. "Filters DEFERRED" block retired in npc_hunt.go. No
deviation tags retired or introduced.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundles 4a/4b/4c parallelization note

Tasks 4a, 4b, 4c are **fully independent** — each touches one production handler file + its corresponding test file with no cross-file imports or shared state. The controller dispatches all three implementer subagents in parallel per the `dispatching-parallel-agents` discipline. Each subagent runs the same audit-rubric pattern against its single assigned file.

Each task uses the same per-handler audit rubric defined in spec § Bundle 4. The implementer subagent must:
1. List every `state.PopInt()` site in the assigned file (`grep -n "state\.PopInt()"`).
2. For each site, locate the TS counterpart handler in `Engine-TS/src/engine/script/handlers/` and check whether TS wraps the popped value with `check(state.popInt(), NumberNotNull)`.
3. Apply the rubric:
   - TS wraps + value is semantically a non-negative count → **WRAP** (add `checkNotNull`).
   - TS does not wrap → **SKIP** (preserve TS tolerance; record rationale).
   - Value is semantically signed (coord delta, search-relative offset, arithmetic operand) → **SKIP** regardless of TS (record rationale).
   - Unclear → **ESCALATE** to controller; do NOT guess.
4. For each WRAPPED handler, add a null-pin test in the `_test.go` file mirroring `TestHandleNpcDelayNullRejected` at `pkg/script/handlers_npc_test.go:1407-1434`.
5. Report the per-handler audit table to the controller as part of the implementer's return summary.

**Wrap pattern:**

```go
v := state.PopInt()
if err := checkNotNull(v, "OP_NAME"); err != nil {
    return err
}
// ... existing handler body ...
```

`OP_NAME` matches the existing convention (e.g., `"NPC_DELAY"`, `"P_ANIMPROTECT"`) — uppercase op-name with underscores. Read existing wrapped handlers in the same file (or `handlers_npc.go`/`handlers_player.go` for cross-file convention reference) for casing.

**Test pattern (single-int handler):**

Mirror `TestHandleNpcDelayNullRejected` at `pkg/script/handlers_npc_test.go:1407-1434`. Pop -1, execute, assert error, assert handler did not perform its side-effect.

```go
func TestHandle<HandlerName>NullRejected(t *testing.T) {
	// Set up state with -1 pushed for the int that should reject null.
	sf := &ScriptFile{
		Name: "<op_lower>_null",
		Opcodes: []Opcode{
			OpPushConstantInt, // push <int_being_tested> = -1
			Op<HandlerOp>,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	// Set up any pointers/active-x state the handler requires.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for input=-1, got nil")
	}
	want := "<OP_NAME>: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	// Optionally: assert no side-effect on the active object.
}
```

For multi-int handlers, table-drive: pop -1 in one slot at a time (other slots stay valid). Group as sub-tests so each null position is independently observable.

---

## Task 4a — Bundle 4: `handlers_npc.go` NumberNotNull audit

**Files:**
- Modify: `pkg/script/handlers_npc.go` (per-handler `checkNotNull` wraps)
- Test: `pkg/script/handlers_npc_test.go` (per-handler null-pin tests)

**Pre-flight context:** Six handlers are already wrapped (`NPC_DELAY`, `NPC_QUEUE`, `NPC_SETTIMER`, `NPC_SETHUNT`, `NPC_FIND` distance, `NPC_FINDCAT` distance). Audit pass targets the remaining ~17-23 unwrapped popInt sites. Existing wraps live in the same file, so the implementer reuses casing and shape.

- [ ] **Step 1: Enumerate popInt sites in `handlers_npc.go`**

Run: `grep -n "state\.PopInt()" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_npc.go`
Record the line numbers and surrounding handler names.

- [ ] **Step 2: For each site, apply the audit rubric**

For each line found in Step 1:
- Identify the enclosing handler (`func handle<X>(s *ScriptState) error`).
- Read the TS counterpart at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts` (same handler name, lowercase).
- Decide: WRAP / SKIP / ESCALATE per the spec rubric.
- If the handler already has a `checkNotNull` wrap (one of the 6 known), record the line and skip.

Build the per-handler audit table:

```
| Handler | Line | TS NumberNotNull-wrapped? | Action | Rationale |
|---------|------|---------------------------|--------|-----------|
| handleNpcDelay | 287 | yes | already wrapped | n/a |
| handleX | NNN | yes | WRAP | n/a |
| handleY | NNN | no  | SKIP | TS does not wrap |
| handleZ | NNN | n/a | SKIP | signed value (coord delta) |
```

Report this table when ESCALATING any unclear case OR at task close.

- [ ] **Step 3: Write failing null-pin tests for every WRAP candidate**

For each handler decided WRAP, append a null-pin test to `pkg/script/handlers_npc_test.go` per the test pattern in the parallelization note above. Pick the existing `TestHandleNpcDelayNullRejected` (line 1407) as the template.

- [ ] **Step 4: Run the new tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "NullRejected" ./pkg/script/`
Expected: FAIL — every newly added null-pin test fails (the wraps aren't in production yet).

- [ ] **Step 5: Add `checkNotNull` wraps to every WRAP candidate in `handlers_npc.go`**

For each WRAP candidate, insert the wrap immediately after the `state.PopInt()` line (same shape as existing wraps in the file).

- [ ] **Step 6: Run all `pkg/script/` tests to verify PASS + no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`
Expected: PASS — all new null-pin tests + existing tests pass.

- [ ] **Step 7: Commit Bundle 4a**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-23 Bundle 4a — handlers_npc.go NumberNotNull audit

Audit pass per NAI-23 spec § Bundle 4: every state.PopInt() in
handlers_npc.go is checked against its TS counterpart in NpcOps.ts; sites
where TS wraps with check(state.popInt(), NumberNotNull) gain a goscape
checkNotNull wrap; signed-value sites and TS-unwrapped sites stay raw with
recorded rationale.

N handlers wrapped; M handlers skipped (rationale per audit table). N
new null-pin tests mirror TestHandleNpcDelayNullRejected. Existing 6
wraps preserved.

Per-handler audit table:
[paste the audit table from Step 2]

Continues the NumberNotNull fidelity sweep started by NAI-20 Task 4. No
deviation tags retired or introduced.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Replace N/M and `[paste the audit table from Step 2]` with the actual values.)

---

## Task 4b — Bundle 4: `handlers_inv.go` NumberNotNull audit

**Files:**
- Modify: `pkg/script/handlers_inv.go` (per-handler `checkNotNull` wraps)
- Test: `pkg/script/handlers_inv_test.go` (per-handler null-pin tests)

**Pre-flight context:** No existing `checkNotNull` wraps in this file (the file is virgin for this fidelity audit). Audit pass targets ~15-18 candidate handlers from the ~55 popInt sites. The implementer establishes the wrap convention in this file by reading `handlers_npc.go` for the existing pattern.

- [ ] **Step 1: Enumerate popInt sites in `handlers_inv.go`**

Run: `grep -n "state\.PopInt()" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_inv.go`
Record line numbers and surrounding handler names. Audit reports ~55 popInts; expect ~15-18 to be WRAP candidates after rubric application.

- [ ] **Step 2: For each site, apply the audit rubric**

For each popInt line:
- Identify the enclosing handler.
- Read the TS counterpart at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts` (same handler name, lowercase).
- Decide: WRAP / SKIP / ESCALATE per rubric.

Build the per-handler audit table per the spec format. ESCALATE any unclear case.

- [ ] **Step 3: Write failing null-pin tests for every WRAP candidate**

Append to `pkg/script/handlers_inv_test.go`. Reuse the file's existing test fixture patterns; if no existing wraps in this file, use `pkg/script/handlers_npc_test.go`'s `TestHandleNpcDelayNullRejected` (line 1407) as cross-file template.

- [ ] **Step 4: Run the new tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "NullRejected" ./pkg/script/`
Expected: FAIL — every newly added null-pin test fails.

- [ ] **Step 5: Add `checkNotNull` wraps to every WRAP candidate in `handlers_inv.go`**

For each WRAP candidate, insert immediately after the `state.PopInt()` line. Reuse the wrap shape established in `handlers_npc.go`.

- [ ] **Step 6: Run all `pkg/script/` tests to verify PASS + no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`
Expected: PASS.

- [ ] **Step 7: Commit Bundle 4b**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-23 Bundle 4b — handlers_inv.go NumberNotNull audit

Audit pass per NAI-23 spec § Bundle 4: every state.PopInt() in
handlers_inv.go is checked against its TS counterpart in InvOps.ts; sites
where TS wraps with check(state.popInt(), NumberNotNull) gain a goscape
checkNotNull wrap; signed-value sites and TS-unwrapped sites stay raw with
recorded rationale.

First fidelity-audit pass on handlers_inv.go (no prior wraps in this
file). N handlers wrapped; M handlers skipped (rationale per audit
table). N new null-pin tests follow the handlers_npc.go shape.

Per-handler audit table:
[paste the audit table from Step 2]

Continues NAI-23 Bundle 4's parallel-dispatched audit pass. No
deviation tags retired or introduced.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4c — Bundle 4: `handlers_interface.go` NumberNotNull audit

**Files:**
- Modify: `pkg/script/handlers_interface.go` (per-handler `checkNotNull` wraps)
- Test: `pkg/script/handlers_interface_test.go` (per-handler null-pin tests)

**Pre-flight context:** No existing `checkNotNull` wraps in this file. Audit pass targets ~12-15 candidate handlers from the ~48 popInt sites.

- [ ] **Step 1: Enumerate popInt sites in `handlers_interface.go`**

Run: `grep -n "state\.PopInt()" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_interface.go`
Record line numbers and surrounding handler names.

- [ ] **Step 2: For each site, apply the audit rubric**

For each popInt line:
- Identify the enclosing handler.
- Read the TS counterpart at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/InterfaceOps.ts` (or `IfOps.ts` — locate via grep if the filename differs).
- Decide: WRAP / SKIP / ESCALATE per rubric.

Build the per-handler audit table. ESCALATE any unclear case.

- [ ] **Step 3: Write failing null-pin tests for every WRAP candidate**

Append to `pkg/script/handlers_interface_test.go`. Use the same template as Task 4b (cross-file reference to `handlers_npc_test.go`'s null-pin shape).

- [ ] **Step 4: Run the new tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "NullRejected" ./pkg/script/`
Expected: FAIL.

- [ ] **Step 5: Add `checkNotNull` wraps to every WRAP candidate in `handlers_interface.go`**

For each WRAP candidate, insert the wrap immediately after the `state.PopInt()` line.

- [ ] **Step 6: Run all `pkg/script/` tests to verify PASS + no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`
Expected: PASS.

- [ ] **Step 7: Commit Bundle 4c**

```bash
git add pkg/script/handlers_interface.go pkg/script/handlers_interface_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-23 Bundle 4c — handlers_interface.go NumberNotNull audit

Audit pass per NAI-23 spec § Bundle 4: every state.PopInt() in
handlers_interface.go is checked against its TS counterpart in
InterfaceOps.ts; sites where TS wraps with check(state.popInt(),
NumberNotNull) gain a goscape checkNotNull wrap; signed-value sites and
TS-unwrapped sites stay raw with recorded rationale.

First fidelity-audit pass on handlers_interface.go (no prior wraps in
this file). N handlers wrapped; M handlers skipped (rationale per audit
table). N new null-pin tests.

Per-handler audit table:
[paste the audit table from Step 2]

Closes NAI-23 Bundle 4's three-file scope. handlers_config.go,
handlers_number.go, and remaining handler files explicitly out-of-scope
for this NAI cycle (tracked as future audit-pass sweeps). No deviation
tags retired or introduced.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Two-stage review checkpoint (post-Bundle-4c)

After all four implementer commits land, the controller dispatches two-stage review per `runescript_cadence`:

**Stage 1 — Code-quality review (per-bundle):**
- Bundle 2: review `Player.Busy()` body, filter-coverage doc-comment update, test names + assertions.
- Bundle 3: review `Player.IsInWilderness()` body, filter ordering, "Filters DEFERRED" block retirement, boundary tests.
- Bundle 4 (a/b/c): review per-file audit tables for completeness, test patterns, wrap consistency.

**Stage 2 — TS-fidelity review (per-bundle):**
- Bundle 2: re-read TS `Player.ts:801-803` + `:796-799` and `Npc.ts:931-933` against the goscape implementation.
- Bundle 3: re-read TS `Player.ts:2082-2090` and `Npc.ts:939-941` against goscape; verify `<` vs `<=` boundaries verbatim.
- Bundle 4 (a/b/c): cross-file ordering sweep — verify per-file audit tables match TS source-of-truth; spot-check 3-5 wrapped handlers per file against TS counterpart.

If review surfaces concerns, file remediation commits per the standard `polish(...)` shape before proceeding to close.

---

## Close commit

Once all reviews pass, land the NAI-23 close commit. The close commit:
- May contain remediation polish from review feedback (consolidate review fixes into a single `polish` commit BEFORE the close).
- The close commit itself is `chore(world): NAI-23 closed — four-bundle follow-up` with empty content (or a minimal commit-message-only commit). It carries the `Closes memory:` trailer per `close_commit_memory_trailer` memory.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world): NAI-23 closed — four-bundle follow-up

Closes the remaining NAI-8 deferred huntPlayers filters and a NumberNotNull
fidelity sweep across three opcode-handler files.

Bundle 1 (memory-only): marked 3 stale tracker entries Resolved
(nai_followups.md:786, :1272, :244-cross-ref).
Bundle 2 (feat): Player.Busy() + checkNotBusy huntPlayers filter.
Bundle 3 (feat): Player.IsInWilderness() + checkNotTooStrong huntPlayers
filter — completes the NAI-8 deferred filter list.
Bundle 4a/b/c (feat): NumberNotNull wraps + null-pin tests across
handlers_npc.go, handlers_inv.go, handlers_interface.go.

Net deviation count: 14 → 14.

Closes memory: nai_followups.md:786 (Resolved by NAI-19 Task 3, commit 8e94b29)
Closes memory: nai_followups.md:1272 (Resolved by NAI-21 Bundle 1, commit ed2f432)
Closes memory: nai_followups.md:244 (Resolved by NAI-16 Task 2 cross-ref)
Closes memory: spec_followup_tracker_freshness.md (NAI-23 corroborating addendum)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Plan self-review

**Spec coverage:** Every spec section maps to a task or step:
- Spec § Bundle 1 → Task 1 (memory edits + addendum).
- Spec § Bundle 2 → Task 2 (Busy + filter).
- Spec § Bundle 3 → Task 3 (IsInWilderness + filter + DEFERRED block retirement).
- Spec § Bundle 4 → Tasks 4a/4b/4c (per-file audit-pass).
- Spec § "Filter ordering invariant" → Bundle 2 doc-comment update + Bundle 3 final block.
- Spec § "Test strategy" → per-task test enumerations.
- Spec § "Risks" → Bundle 3 nil-guard escalation directive in Step 8.
- Spec § "Implementation cadence" → tasks structured as TDD; Bundles 4a/b/c flagged for parallel dispatch; review checkpoint after Bundle 4c.

**Placeholder scan:** None of the forbidden patterns ("TBD", "TODO", vague handling) appear in the plan. Bundle 4 audit tables intentionally use placeholders (`N`, `M`, `[paste the audit table]`) because the audit IS the work; the implementer fills them with discovered counts at task time.

**Type consistency:** `Busy()` and `IsInWilderness()` referenced consistently across Tasks 2 and 3. `HuntCheckNotTooStrongOutsideWilderness` enum value used in Task 3 matches the verified field at `pkg/objtype/hunttype.go:38`. `modalStateMain`/`modalStateChat` constants used in Task 2 match the verified constants at `player.go:31-32`.

No issues found.
