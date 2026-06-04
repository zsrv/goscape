# NAI-114 Stage 5 — `resolveListenerInv` UID fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Patch `resolveListenerInv` to interpret `InventoryListener.Source >= 0` as a UID (matching TS), add 4 unit tests pinning all branches, ship as one fix commit. Stage 5 close (after smoke ✅) reverts the Stage 4 probe and ships an NAI-114-close memory commit.

**Architecture:** Single-function rewrite mirroring sister consumer `Player.updateInvs`'s UID-lookup pattern. New test file `modules/world/resolve_listener_inv_test.go` with 4 unit tests covering world-source / player-source-match / player-source-offline / player-source-null-inv. Existing handler tests untouched.

**Tech Stack:** Go 1.26+. `pkg/inventory` for the `*Inventory` type. `Server.LookupPlayerByUID` (already exposed) for the UID-lookup primitive.

---

## File Structure

| File | Status | Responsibility |
|------|--------|----------------|
| `modules/world/handler_opnpc.go` | Modify (L9-26) | Rewrite `resolveListenerInv` body + doc comment. |
| `modules/world/player.go` | Modify (L26) | Update `InventoryListener.Source` field comment from "owning player's slot" to "owning player's UID". |
| `modules/world/resolve_listener_inv_test.go` | Create | 4 unit tests pinning all branches of `resolveListenerInv`. |

Stage 5 close work (Tasks 4-5, executed only after user smoke ✅):

| File | Status | Responsibility |
|------|--------|----------------|
| `modules/world/handler_opheld.go` | Modify (via revert) | Revert of `550840b` removes 18 inline DEBUG lines + helper + slog/sort imports. |
| `.claude/projects/.../memory/*.md` | Create + Modify | New entry on UID-vs-slot semantic-name collision; updates to `cascade_theory_smoke_binding`, `investigation_subspec_cadence`, `MEMORY.md` index. |

---

## Pre-flight (controller, not implementer)

- [ ] **Verify HEAD is `27e1ee8`** (Stage 5 spec commit) and working tree clean.

```bash
git rev-parse HEAD
# Expected: 27e1ee8...
git status --short
# Expected: only ?? .claude/, ?? test_typed_nil.go, and untracked dotfiles (no modified files in tracked tree)
```

- [ ] **Verify the buggy function shape at HEAD** (sanity-check that the spec premises are still accurate):

```bash
sed -n '9,26p' modules/world/handler_opnpc.go
```

Expected output ends with the buggy `s.players[listener.Source]` block:

```
// resolveListenerInv returns the inventory the given listener observes,
// or nil if it can't be resolved. Source = -1 → world-shared inventory
// (Server.invs[Type]); otherwise the source is another player's slot,
// and the inventory is that player's local invs[Type]. Mirrors TS
// getInventoryFromListener in Player.ts.
func resolveListenerInv(s *Server, listener InventoryListener) *inventory.Inventory {
	if listener.Source == -1 {
		return s.invs[listener.Type]
	}
	if listener.Source < 0 || listener.Source >= len(s.players) {
		return nil
	}
	other := s.players[listener.Source]
	if other == nil {
		return nil
	}
	return other.invs[listener.Type]
}
```

If the shape differs, STOP — the plan was authored against this exact source.

---

## Task 1: Write the failing tests (TDD red)

**Files:**
- Create: `modules/world/resolve_listener_inv_test.go`

**Why TDD red first:** Test 2 (the player-source-match case) is the bug-pin and MUST fail on current code. Tests 1, 3, 4 will pass on current code (some "for the wrong reason" per `test_passes_for_wrong_reason` memory — Test 4 in particular returns nil pre-fix because the `Source >= len(s.players)` bounds check trips, not because of the post-fix's null-inv branch). That's acceptable: the suite as a whole becomes a regression pin once the fix lands and they all pass for the *right* reason.

- [ ] **Step 1.1: Create `modules/world/resolve_listener_inv_test.go`**

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
)

// TestResolveListenerInvWorldSource pins Source=-1 → returns the world-
// shared inventory at s.invs[Type]. Branch existed pre-fix and stays
// behaviorally identical post-fix.
func TestResolveListenerInvWorldSource(t *testing.T) {
	s := newTestServer(t)
	s.invs = map[int]*inventory.Inventory{
		42: inventory.New(42, 28, inventory.StackNormal),
	}
	want := s.invs[42]

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: -1})

	if got != want {
		t.Errorf("resolveListenerInv(world): got %p, want %p", got, want)
	}
}

// TestResolveListenerInvPlayerSourceMatch is the regression pin for
// NAI-114 Stage 5. Pre-fix this test FAILS — the function indexes
// s.players[Source] and Source=98765 trips the >= len(s.players)
// bounds check, returning nil. Post-fix it passes via
// LookupPlayerByUID → target.invs[Type].
func TestResolveListenerInvPlayerSourceMatch(t *testing.T) {
	s := newTestServer(t)

	target := &Player{
		slot:   5,
		uid:    98765,
		active: true,
		invs: map[int]*inventory.Inventory{
			42: inventory.New(42, 28, inventory.StackNormal),
		},
	}
	s.players[target.slot] = target
	s.playerLoop = append(s.playerLoop, target)
	want := target.invs[42]

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: 98765})

	if got != want {
		t.Errorf("resolveListenerInv(player UID): got %p, want %p", got, want)
	}
}

// TestResolveListenerInvPlayerSourceOffline pins Source=<UID with no
// active player> → returns nil cleanly (no panic, no slot OOB).
func TestResolveListenerInvPlayerSourceOffline(t *testing.T) {
	s := newTestServer(t)
	// No player with uid=999999 wired into s.playerLoop.

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: 999999})

	if got != nil {
		t.Errorf("resolveListenerInv(no such uid): got %p, want nil", got)
	}
}

// TestResolveListenerInvPlayerSourceNullInv pins target online but
// target.invs[Type] is nil (or unset) → returns nil. Pre-fix this
// passes for the wrong reason (bounds-check trips before the player
// is even consulted); post-fix it exercises the actual null-inv
// branch. Documenting that here so a future reader doesn't take the
// pre-fix pass as evidence the function works.
func TestResolveListenerInvPlayerSourceNullInv(t *testing.T) {
	s := newTestServer(t)

	target := &Player{
		slot:   5,
		uid:    98765,
		active: true,
		// invs left nil — Go map reads on a nil map return the zero
		// value, so target.invs[42] is nil.
	}
	s.players[target.slot] = target
	s.playerLoop = append(s.playerLoop, target)

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: 98765})

	if got != nil {
		t.Errorf("resolveListenerInv(null inv slot): got %p, want nil", got)
	}
}
```

- [ ] **Step 1.2: Run the new tests — expect Test 2 to FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveListenerInv -v
```

Expected:
- `TestResolveListenerInvWorldSource` PASS
- `TestResolveListenerInvPlayerSourceMatch` **FAIL** with a message of the form `resolveListenerInv(player UID): got 0x0, want 0x...`
- `TestResolveListenerInvPlayerSourceOffline` PASS
- `TestResolveListenerInvPlayerSourceNullInv` PASS (for the wrong reason — see test docstring)

**If all 4 pass:** the spec premise is wrong; STOP and re-read `modules/world/handler_opnpc.go`. The bug should be reproducible at HEAD `27e1ee8`.

**If Test 2 fails with a different shape (panic, compile error):** STOP and report the error — likely a fixture wiring issue, not the bug under test.

- [ ] **Step 1.3: Do NOT commit yet.** Keep the red state for Task 2's TDD-green step.

---

## Task 2: Apply the fix (TDD green)

**Files:**
- Modify: `modules/world/handler_opnpc.go` (function body + doc comment, L9-26)
- Modify: `modules/world/player.go` (struct field comment, L26)

- [ ] **Step 2.1: Replace `resolveListenerInv` body + doc comment**

Find (in `modules/world/handler_opnpc.go`):

```go
// resolveListenerInv returns the inventory the given listener observes,
// or nil if it can't be resolved. Source = -1 → world-shared inventory
// (Server.invs[Type]); otherwise the source is another player's slot,
// and the inventory is that player's local invs[Type]. Mirrors TS
// getInventoryFromListener in Player.ts.
func resolveListenerInv(s *Server, listener InventoryListener) *inventory.Inventory {
	if listener.Source == -1 {
		return s.invs[listener.Type]
	}
	if listener.Source < 0 || listener.Source >= len(s.players) {
		return nil
	}
	other := s.players[listener.Source]
	if other == nil {
		return nil
	}
	return other.invs[listener.Type]
}
```

Replace with:

```go
// resolveListenerInv returns the inventory the given listener observes,
// or nil if it can't be resolved. Source = -1 → world-shared inventory
// (Server.invs[Type]); otherwise Source is another player's UID, and
// the inventory is that player's local invs[Type]. Mirrors TS
// Player.getInventoryFromListener (Player.ts:getInventoryFromListener).
//
// NAI-114 Stage 5: prior to this fix Source was indexed directly into
// s.players[], which silently failed for any UID >= len(s.players)
// (always, in practice). Sister consumer Player.updateInvs already used
// LookupPlayerByUID; this function now matches.
func resolveListenerInv(s *Server, listener InventoryListener) *inventory.Inventory {
	if listener.Source == -1 {
		return s.invs[listener.Type]
	}
	otherActive := s.LookupPlayerByUID(listener.Source)
	if otherActive == nil {
		return nil
	}
	other, ok := otherActive.(*Player)
	if !ok || other == nil {
		return nil
	}
	return other.invs[listener.Type]
}
```

- [ ] **Step 2.2: Update `InventoryListener.Source` struct field comment**

Find (in `modules/world/player.go` near L26):

```go
	Source    int  // -1 = world-shared inventory, else owning player's slot
```

Replace with:

```go
	Source    int  // -1 = world-shared inventory, else owning player's UID
```

(The exact whitespace between `Source`, `int`, and the comment may vary; use Edit's surrounding-context disambiguation. The line must match the InventoryListener struct field, NOT any unrelated `Source int` field.)

- [ ] **Step 2.3: Re-run the 4 tests — expect all PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveListenerInv -v
```

Expected:
```
=== RUN   TestResolveListenerInvWorldSource
--- PASS: TestResolveListenerInvWorldSource
=== RUN   TestResolveListenerInvPlayerSourceMatch
--- PASS: TestResolveListenerInvPlayerSourceMatch
=== RUN   TestResolveListenerInvPlayerSourceOffline
--- PASS: TestResolveListenerInvPlayerSourceOffline
=== RUN   TestResolveListenerInvPlayerSourceNullInv
--- PASS: TestResolveListenerInvPlayerSourceNullInv
PASS
```

If Test 2 still fails: re-check Step 2.1 — the literal-byte replacement may have introduced a typo. Diff against the spec §4.1 fix code.

- [ ] **Step 2.4: Run full repo build + tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: build clean, all PASS. The fix is purely a code-path change inside `resolveListenerInv`; no caller sees a different signature, so existing handler tests for OPHELDU / OPNPC / OPLOC / OPOBJ / OP_PLAYER / INV_BUTTON are unaffected.

If any handler test fails: STOP and report. Likely cause would be a fixture in another test file that relied on the buggy `s.players[Source]` semantics — but per spec §3 grep, none exists.

- [ ] **Step 2.5: Commit (single fix commit covering tests + function + struct comment)**

```bash
git add modules/world/handler_opnpc.go modules/world/player.go modules/world/resolve_listener_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-114 Stage 5 — resolveListenerInv interpret Source as UID

InventoryListener.Source is a UID (assigned by registration sites in
pkg/script/handlers_inv.go via Self.UID() / popped uid). resolveListenerInv
was indexing s.players[Source] directly, which silently failed for any
UID >= len(s.players) — always, in practice (UIDs are ~2.2 billion;
len(s.players) is at most 2047 per NodeMaxPlayers).

Sister consumer Player.updateInvs (player.go:766-805) already uses the
correct LookupPlayerByUID + type-assert pattern; this function now
matches. TS Player.ts:getInventoryFromListener is the canonical reference
and uses World.getPlayerByUid.

Singular site — verified at spec-write time by grepping s.players[X] non-
test occurrences (3 hits: handler_opnpc.go:21 was the bug, server.go:740
and :756 use p.slot correctly).

Adds 4 unit tests in resolve_listener_inv_test.go pinning all four
branches (world / player-match / player-offline / player-null-inv) —
the function previously had zero direct test coverage; existing handler-
level tests for INV_BUTTON only exercised the world-source nil path.

Bound by NAI-114 Stage 4 smoke (gate=inv_unresolved on 5/5 OPHELDU events
during tutorial firemaking with listener_type=93, listener_source=PlayerUID).
Stage 4 probe revert + NAI-114 close commits land separately after user
smoke confirms tutorial firemaking now produces a fire.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2.6: Verify commit content matches stated scope**

```bash
git show HEAD --stat
```

Expected (line counts approximate):
```
 modules/world/handler_opnpc.go             |  10 +++++-----
 modules/world/player.go                    |   2 +-
 modules/world/resolve_listener_inv_test.go |  90 ++++++++++++++++++++++++++
 3 files changed, 96 insertions(+), 6 deletions(-)
```

```bash
git status --short
```

Expected: only `?? .claude/`, `?? test_typed_nil.go`, untracked dotfiles. Working tree clean.

Per `implementer_commit_content_verify` memory: confirm what shipped is what we said would ship.

---

## Task 3: User-smoke handoff

**This task is controller-only — no code change.**

- [ ] **Step 3.1: Verify commit shape post-fix**

```bash
git log --oneline -4
```

Expected (top to bottom):
```
<sha> fix(world): NAI-114 Stage 5 — resolveListenerInv interpret Source as UID
27e1ee8 docs(spec): NAI-114 Stage 5 — resolve-listener-inv UID fix design
550840b chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation
1750014 Revert "chore(debug): NAI-114 Stage 3 — boot-time opheldu script-registry log"
```

- [ ] **Step 3.2: Emit paste-ready smoke handoff prompt**

Surface this verbatim to the user:

> Stage 5 fix landed. Stage 4 probe still active (will be reverted at Stage 5 close after smoke ✅). To smoke:
>
> 1. `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`
> 2. Connect via Java client rev-225, log in as the same tutorial-firemaking-step character used in Stages 3 and 4.
> 3. At the firemaking step, use tinderbox on logs (3-5 attempts).
> 4. Stop the server and capture stdout for any line containing `"opheldu entry"`, `"opheldu reject"`, or any `script_name=[opheldu,*]` log.
>
> **Expected outcomes:**
>
> - **Smoke ✅** — `opheldu entry` lines appear with NO accompanying `opheldu reject` lines. The `[opheldu,tinderbox]`/`[opheldu,logs]` script fires and a fire is produced (or whatever subsequent tutorial step the script triggers). Spec §5.2 close routing.
> - **`opheldu reject gate=objType_unregistered`** — the obj 2511 / 590 cache lookup fails. New downstream gate, route to NAI-114 Stage 6.
> - **`opheldu reject gate=inv_unresolved` still firing** — fix didn't take effect (rare; investigate why `LookupPlayerByUID` doesn't see this listener's source). NAI-114 Stage 6.
> - **`opheldu entry` with no reject, but no fire produced** — dispatch unblocked but content fails downstream. Close NAI-114 (the dispatch fix landed) and route to NAI-115 brainstorm.
>
> Paste the captured logs back to start NAI-114 close (or Stage 6 / NAI-115 if needed).

---

## Task 4: Stage 5 close — revert Stage 4 probe

**Conditional on user-smoke ✅ at Task 3.** Skip if smoke results route to Stage 6 / NAI-115.

**Files:**
- Modify (via revert): `modules/world/handler_opheld.go`

- [ ] **Step 4.1: Revert the Stage 4 probe**

```bash
git revert --no-gpg-sign --no-edit 550840b
```

Expected: clean revert (no conflicts — the only changes between `550840b` and HEAD touch different files: `27e1ee8` is a doc, the fix commit touches `handler_opnpc.go` / `player.go` / `resolve_listener_inv_test.go`).

- [ ] **Step 4.2: Verify the revert removed only Stage 4 instrumentation**

```bash
git show HEAD --stat
```

Expected:
```
 modules/world/handler_opheld.go | 61 -----------------------------------------
 1 file changed, 61 deletions(-)
```

```bash
grep -c 'opheldu reject\|opheldu entry\|snapshotInvListenerKeys\|slog.Default()' modules/world/handler_opheld.go
```

Expected: `0` (all four search strings removed).

- [ ] **Step 4.3: Verify build + tests still pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: build clean, all PASS — including the 4 new `TestResolveListenerInv*` tests (the revert doesn't touch the fix or its tests).

---

## Task 5: NAI-114 close memory commit

**Conditional on Task 4 completing.** Memory updates only — no production code.

**Files:**
- Create: `.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/inventory_listener_source_uid_not_slot.md`
- Modify: `.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/cascade_theory_smoke_binding.md`
- Modify: `.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/investigation_subspec_cadence.md`
- Modify: `.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`

- [ ] **Step 5.1: Write new memory entry on the UID-vs-slot collision**

Create `inventory_listener_source_uid_not_slot.md` with frontmatter type=feedback. Content sketch (controller authors at close time, may refine wording):

```markdown
---
name: InventoryListener.Source is a UID, not a slot
description: NAI-114 caught one consumer indexing s.players[Source] directly while sister consumer correctly used LookupPlayerByUID; semantic-name collision survived 4 stages of investigation
type: feedback
---

When a struct field has two competing semantic interpretations (slot index vs UID) AND the field name doesn't disambiguate, audit ALL consumers when one consumer is touched.

**Concrete instance (NAI-114):** `InventoryListener.Source` is set to `Self.UID()` at registration (per TS Player.invListenOnCom). Two consumers existed:
- `Player.updateInvs` (correct): `s.LookupPlayerByUID(l.Source)` then type-assert to `*Player`
- `resolveListenerInv` (buggy): `s.players[listener.Source]` direct index — UIDs (~2.2B) are far larger than `len(s.players)` (2047), so the bounds check tripped and every player-source listener silently returned nil

The bug survived NAI-24 Bundle 2 (which fixed registration to pass UID instead of -1) because no smoke exercised the player-source branch end-to-end at that time. Surfaced 5 sub-specs later when tutorial firemaking smoke bound `gate=inv_unresolved`.

**Why:** Field-name collisions where one type is bounded (slot, max ~2047) and the other is unbounded (UID, ~2 billion) make for silent-failure bugs that compile and pass world-source tests.

**How to apply:** When fixing a registration site that sets a field whose name doesn't disambiguate two semantic interpretations, grep ALL non-test consumers and verify each consumer's interpretation matches the new convention. Pattern: `rg 'fieldname' modules/ pkg/ | grep -v _test.go`.
```

- [ ] **Step 5.2: Update `cascade_theory_smoke_binding.md`** to add NAI-114 as a 4-stage probe → fix example.

(Existing memory file content varies; controller appends an "Examples" or "Instances" section bullet referencing NAI-114 as a successful chain: Stage 3 first probe under-anticipated the gate position, Stage 4 second probe bound it, Stage 5 fixed it, all via cascade-binding smokes.)

- [ ] **Step 5.3: Update `investigation_subspec_cadence.md`** to add the 4-stage chain pattern.

(Existing memory file mentions chained probe sub-specs; append NAI-114 as a 4-stage instance — Stage 3 = first probe, Stage 4 = second probe after first probe under-anticipated, Stage 5 = fix sub-spec — confirming probe sub-specs CAN chain when the first probe binds an unanticipated upstream branch.)

- [ ] **Step 5.4: Add new memory entry to `MEMORY.md` index**

Append one line to `MEMORY.md`:

```
- [InventoryListener.Source is a UID not a slot](inventory_listener_source_uid_not_slot.md) — semantic-name collision; one consumer indexed s.players[Source] while sister used LookupPlayerByUID; field-name didn't disambiguate (NAI-114 closed)
```

(Use Edit's append-to-end pattern; preserve existing index ordering.)

- [ ] **Step 5.5: Commit**

```bash
git add .claude/projects/
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(memory): NAI-114 close — InventoryListener.Source UID-vs-slot collision

Closes NAI-114 (4-stage investigation: brainstorm → Stage 3 first probe
→ Stage 4 second probe → Stage 5 fix). Smoke confirmed tutorial
firemaking works post-fix.

Adds inventory_listener_source_uid_not_slot.md memory entry; updates
cascade_theory_smoke_binding and investigation_subspec_cadence with
NAI-114 as a reusable 4-stage probe chain example.

Closes memory: inventory_listener_source_uid_not_slot,
cascade_theory_smoke_binding, investigation_subspec_cadence

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.6: Verify final commit shape**

```bash
git log --oneline -6
```

Expected (top to bottom):
```
<sha> chore(memory): NAI-114 close — InventoryListener.Source UID-vs-slot collision
<sha> Revert "chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation"
<sha> fix(world): NAI-114 Stage 5 — resolveListenerInv interpret Source as UID
27e1ee8 docs(spec): NAI-114 Stage 5 — resolve-listener-inv UID fix design
550840b chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation
1750014 Revert "chore(debug): NAI-114 Stage 3 — boot-time opheldu script-registry log"
```

NAI-114 closed.

---

## Self-Review

**1. Spec coverage:**

| Spec section | Plan task |
|---|---|
| §3 In-scope: rewrite resolveListenerInv body + doc comment | Task 2 (Steps 2.1) |
| §3 In-scope: update Source field comment | Task 2 (Step 2.2) |
| §3 In-scope: 4 unit tests pinning all branches | Task 1 (Step 1.1) |
| §3 In-scope: revert Stage 4 probe at Stage 5 close | Task 4 |
| §3 In-scope: NAI-114 close commit with memory updates | Task 5 |
| §4.1 The fix function rewrite | Step 2.1 (verbatim from spec) |
| §4.2 Struct field comment update | Step 2.2 |
| §4.3 4 test cases | Step 1.1 (each as a named test) |
| §4.4 Build + test verification | Step 2.4 |
| §4.5 Commit sequence (3 + 4 + 5) | Steps 2.5, 4.x, 5.5 |
| §5 User smoke + decision matrix | Task 3 (handoff prompt enumerates §5.1's four routings) |
| §6 Risk register R1 (multi-player fixture) | Step 1.1 — fixture mirrors `server_test.go:475-479` pattern |
| §6 R2 (other downstream gate at smoke) | Task 3 prompt enumerates `objType_unregistered` route |
| §6 R3 (latent defects in other handlers) | Spec §3 already says all 6 handlers benefit identically |
| §7 Memory updates list | Task 5 (3 files + index) |

All §3 in-scope items have tasks; all §4 deliverables are step-anchored.

**2. Placeholder scan:** none. Every code block is full source. Every command has expected output. Test 4's "passes for the wrong reason pre-fix" is documented in the test docstring AND in Task 1's preamble, so it's not a placeholder.

**3. Type/identifier consistency:**
- `resolveListenerInv` signature unchanged across spec and plan — `(s *Server, listener InventoryListener) *inventory.Inventory`.
- `InventoryListener` field names (`Type`, `Source`) match spec §4.1.
- `inventory.New(typeId, capacity, stackType int)` and `inventory.StackNormal` constant verified at spec-write time against `pkg/inventory/inventory.go:5-36`.
- `Player` field names used in test fixtures (`slot`, `uid`, `active`, `invs`) verified at spec-write time against `modules/world/player.go:64,66,74,282,316`.
- `Server.players` type is `[2048]*Player` (fixed-size array) — `s.players[target.slot] = target` is valid without map allocation.
- `Server.invs` type is `map[int]*inventory.Inventory` — Test 1 assigns the map literal directly (`s.invs = map[int]*inventory.Inventory{...}`) since `newTestServer` doesn't init it.
- `s.playerLoop` is the slice scanned by `LookupPlayerByUID` — fixture appends to it via `s.playerLoop = append(s.playerLoop, target)` matching `server_test.go:479`.
- `LookupPlayerByUID` filters via `if p == nil || !p.active { continue }; if p.uid == uid { return p }` — fixture sets `active: true` and `uid: 98765`.

**4. Cross-check test-helper flag/lifecycle:**
- `newTestServer(t)` does NOT init `s.invs` (verified at server_test.go:311-326). Test 1 assigns the map literal directly.
- `newTestServer(t)` does NOT init `s.players` either, but `s.players` is `[2048]*Player` (fixed array, zero-initialized). Direct slot indexing works.
- `target.invs` left nil in Test 4 — Go map reads on nil maps return zero value, so `target.invs[42]` is `(*inventory.Inventory)(nil)` without panic. Verified.

**5. Commit-ordering:**
- Pre-flight: HEAD = `27e1ee8` (spec).
- After Task 2: HEAD = fix commit on top of spec.
- After Task 4: HEAD = revert of `550840b` on top of fix.
- After Task 5: HEAD = memory commit on top of revert.

Total: 4 commits land in this plan's execution (spec is already committed pre-flight).

**6. TDD discipline:**
- Step 1.1 writes failing tests; Step 1.2 observes failure (Test 2); Step 1.3 explicitly says no commit yet.
- Step 2.1-2.2 implement minimal fix; Step 2.3 observes pass; Step 2.4 verifies no regressions; Step 2.5 commits.
- Test + fix bundle as one logical commit; spec author and impl author are the same agent so the discipline is preserved without separate red/green commits.
