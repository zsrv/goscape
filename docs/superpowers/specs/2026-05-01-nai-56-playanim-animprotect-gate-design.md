# NAI-56: PlayAnim animProtect gate — wire S7b-D1 reader

> **Cadence:** Compressed (`compressed_cadence.md`). Single combined
> spec+plan doc, ≤~15 source LOC, no formal whole-impl review. Inline
> TDD.

## Goal

Close `S7b-D1` by wiring the existing-but-unread `Player.animProtect`
flag into `(*Player).PlayAnim` as an early-return gate. Mirrors TS
`Player.playAnimation` (`Player.ts:1842`) `if (... || this.animProtect) return`.

## Context: animProtect is plumbed but dead

Goscape already has the full plumbing chain for `P_ANIMPROTECT`:

| Layer | Location | Status |
|---|---|---|
| Field | `(*Player).animProtect int` at `player.go:169` | ✅ declared |
| Setter (production) | `(*Player).SetAnimProtect(v int)` at `player_script.go:806` | ✅ wired |
| Setter (interface) | `script.ActivePlayer.SetAnimProtect(v int)` at `pkg/script/active.go:402` | ✅ |
| Script-opcode writer | `P_ANIMPROTECT` (`pkg/script/handlers_player.go:117`) calls `s.Self.SetAnimProtect(v)` | ✅ wired |
| **Reader** | TS `Player.ts:1842` checks `this.animProtect` in `playAnimation` | ❌ **unported — S7b-D1** |

The doc-comment on `SetAnimProtect` at `player_script.go:802-806` already
acknowledges the gap:

> "anim-protect flag; when nonzero, in-engine animation requests should be
> suppressed (reader path unported — S7b-D1; paid down when anim playback
> is ported)"

NAI-56 ports the reader.

## TS `Player.playAnimation` (Player.ts:1841-1851)

```ts
playAnimation(anim: number, delay: number) {
    if (anim >= SeqType.count || this.animProtect) {
        return;
    }

    if (anim == -1 || this.animId == -1 || SeqType.get(anim).priority > SeqType.get(this.animId).priority || SeqType.get(this.animId).priority === 0) {
        this.animId = anim;
        this.animDelay = delay;
        this.masks |= PlayerInfoProt.ANIM;
    }
}
```

Three TS gates, only one of which has full plumbing in goscape today:

1. **`anim >= SeqType.count`** — bounds-reject out-of-range seq id. Requires
   the `SeqType` config registry, which goscape has not ported (`grep
   SeqType pkg/ modules/` is empty). **Out of scope** — tracked as
   NAI-56-D1 below.
2. **`this.animProtect`** — the flag this sub-spec wires. **In scope.**
3. **Priority comparison** — only overwrite the in-flight anim if (a) clearing
   (anim==-1), (b) no current anim (animId==-1), (c) new priority >
   current priority, or (d) current priority == 0. Requires `SeqType.get(id).priority`,
   so depends on the SeqType config-port. **Out of scope** — tracked as
   NAI-56-D1 below.

## Goscape's current `(*Player).PlayAnim` (player_script.go:541-547)

```go
// PlayAnim schedules sequence seqID with the given client-side delay on
// the player's primary animation slot. seqID=-1 clears.
func (p *Player) PlayAnim(seqID, delay int) {
    p.animID = seqID
    p.animDelay = delay
    p.masks |= rsbuf.MaskAnim
}
```

Naive write — no gate of any kind.

`PlayAnim` is the **only production caller** of the player anim mask.
`(*Player).Animate` (`player_masks.go:8`) is a sibling test-only helper
with the same body shape; production grep confirms zero non-test callers.
NAI-56 leaves `Animate` alone (test-only convenience; gating it would
require a full caller audit and adds zero behaviour-correctness value).

## NPC scope

TS `Npc.playAnimation` (`Npc.ts:453-462`) has the SeqType-bounds and
priority gates **but no `animProtect`** — NPCs don't carry an animProtect
flag. Goscape's NPC production setter is `(*Npc).Animate`
(`npc_masks.go:8`, called by `handleNpcAnim` via the `script.ActiveNpc`
interface). NPC anim has nothing for this sub-spec to wire — its
SeqType-bounds/priority gaps fold under NAI-56-D1 alongside Player.

## The fix

Single 3-line addition to `(*Player).PlayAnim`:

```go
func (p *Player) PlayAnim(seqID, delay int) {
    if p.animProtect != 0 {
        return // TS Player.ts:1842 — animProtect gate (closes S7b-D1)
    }
    p.animID = seqID
    p.animDelay = delay
    p.masks |= rsbuf.MaskAnim
}
```

The `!= 0` test mirrors the TS truthy check on a numeric field. The
existing setter `SetAnimProtect(v int)` accepts arbitrary ints (matches TS
`number`), so any nonzero value blocks; the doc-comment already documents
this contract.

Doc-comment updated to point at the gate's TS source line.

## Test plan

Two new tests, paralleling the `TestAnimateSetsMask` shape in
`modules/world/player_masks_test.go`. New file
`modules/world/player_anim_test.go` to keep the gate tests grouped under
the production setter (`PlayAnim`) rather than the test-only sibling
(`Animate`).

1. **`TestPlayAnim_AnimProtectBlocksWrite`** — set `p.animProtect = 1`,
   record initial `(animID, animDelay, masks)`, call `p.PlayAnim(123, 5)`,
   assert all three unchanged. Pins the gate.

2. **`TestPlayAnim_AnimProtectZeroAllowsWrite`** — `p.animProtect = 0`
   (the default), call `p.PlayAnim(123, 5)`, assert `animID==123`,
   `animDelay==5`, `masks & MaskAnim != 0`. Pins the baseline (regression
   guard for the existing path).

`TestAnimateSetsMask` (`player_masks_test.go:9`) remains green —
`Animate` is untouched and still bypasses the gate (test-only helper).

## Tracked deviations

**Closed:**
- `S7b-D1` — `Player.animProtect` reader now wired in `(*Player).PlayAnim`.

**Introduced:**
- **NAI-56-D1** — `(*Player).PlayAnim` (`player_script.go:543`) and
  `(*Npc).Animate` (`npc_masks.go:8`) skip TS `playAnimation`'s remaining
  two gates: (a) `anim >= SeqType.count` bounds-reject and (b)
  priority-comparison overwrite gate. Both depend on the unported
  `SeqType` config registry (`Engine-TS/src/cache/config/SeqType.ts`,
  133 LOC; ports analogous to NAI-46 IdkType cadence). **Closure:** future
  SeqType config-port sub-spec.

## Net deviation tally

21 → 21 (one closed, one introduced).

## Out of scope

- SeqType config-port (its own sub-spec; tracked as NAI-56-D1).
- Renaming `PlayAnim` → `PlayAnimation` (downstream interface +
  mock-recorder churn for zero behavioural return).
- Gating `(*Player).Animate` (test-only sibling; production grep confirms
  zero non-test callers).
- NPC-side animProtect (TS `Npc.playAnimation` has no animProtect gate).

## Observable wire-behavior delta

A player whose script ran `P_ANIMPROTECT(1)` now correctly suppresses
subsequent `ANIM` script-opcode writes via the script-handler →
`s.Self.PlayAnim` → gate path. Previously the flag was set and forgotten;
the next `ANIM` opcode in the same tick or until-cleared window
overwrote `animID`/`animDelay` and re-flagged `MaskAnim` regardless,
clobbering whatever protected animation was in flight.

---

# Plan

Subagent-driven-development per `execution_mode_default.md`. Single
bundle, two tasks. Each task is its own commit. Close-trailer in T2's
commit per `close_commit_memory_trailer.md`.

## Task 1: Add the animProtect gate to `(*Player).PlayAnim` (TDD)

**Files:**
- Create: `modules/world/player_anim_test.go`
- Modify: `modules/world/player_script.go` (`PlayAnim` body at lines
  541-547 + `SetAnimProtect` doc-comment at lines 802-806)

- [ ] **Step 1: Write the failing tests**

Create `modules/world/player_anim_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/rsbuf"
)

// TestPlayAnim_AnimProtectBlocksWrite — NAI-56. With animProtect set,
// PlayAnim must early-return and leave animID, animDelay, and the
// MaskAnim bit untouched. Mirrors TS Player.ts:1842 where the animProtect
// truthy check short-circuits playAnimation before any write.
func TestPlayAnim_AnimProtectBlocksWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	initialAnimID := p.animID
	initialAnimDelay := p.animDelay
	initialMasks := p.masks

	p.animProtect = 1
	p.PlayAnim(123, 5)

	if p.animID != initialAnimID {
		t.Errorf("animID: got %d, want %d (unchanged)", p.animID, initialAnimID)
	}
	if p.animDelay != initialAnimDelay {
		t.Errorf("animDelay: got %d, want %d (unchanged)", p.animDelay, initialAnimDelay)
	}
	if p.masks != initialMasks {
		t.Errorf("masks: got %d, want %d (unchanged)", p.masks, initialMasks)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim bit should not be set when animProtect blocks the write")
	}
}

// TestPlayAnim_AnimProtectZeroAllowsWrite — NAI-56. Baseline regression
// guard: with animProtect=0 (default), PlayAnim writes through and sets
// MaskAnim. Pins that the new gate has no effect on the unprotected path.
func TestPlayAnim_AnimProtectZeroAllowsWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.animProtect != 0 {
		t.Fatalf("animProtect: precondition got %d, want 0", p.animProtect)
	}

	p.PlayAnim(123, 5)

	if p.animID != 123 {
		t.Errorf("animID: got %d, want 123", p.animID)
	}
	if p.animDelay != 5 {
		t.Errorf("animDelay: got %d, want 5", p.animDelay)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim bit should be set when animProtect=0")
	}
}
```

- [ ] **Step 2: Run tests to verify the gate test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayAnim_AnimProtect" -v`

Expected: `TestPlayAnim_AnimProtectBlocksWrite` FAILS (animID becomes 123,
animDelay becomes 5, masks gains the MaskAnim bit — none of those should
happen). `TestPlayAnim_AnimProtectZeroAllowsWrite` PASSES (current
behavior already matches the post-fix expectation for the baseline path).

- [ ] **Step 3: Add the gate**

In `modules/world/player_script.go`, replace the `PlayAnim` block at
lines 541-547 with:

```go
// PlayAnim schedules sequence seqID with the given client-side delay on
// the player's primary animation slot. seqID=-1 clears. Mirrors TS
// Player.playAnimation (Player.ts:1841-1851); the animProtect early-return
// is the TS L1842 gate (NAI-56, closes S7b-D1). The remaining TS gates
// (anim >= SeqType.count bounds and priority comparison at L1846) depend
// on the unported SeqType config registry — tracked as NAI-56-D1.
func (p *Player) PlayAnim(seqID, delay int) {
	if p.animProtect != 0 {
		return // TS Player.ts:1842 — animProtect gate (NAI-56)
	}
	p.animID = seqID
	p.animDelay = delay
	p.masks |= rsbuf.MaskAnim
}
```

- [ ] **Step 4: Re-run the gate tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayAnim_AnimProtect" -v`

Expected: both PASS.

- [ ] **Step 4b: Refresh the `SetAnimProtect` doc-comment**

In `modules/world/player_script.go`, replace the `SetAnimProtect` block at
lines 802-806 from:

```go
// SetAnimProtect implements script.ActivePlayer.SetAnimProtect. Stores the
// anim-protect flag; when nonzero, in-engine animation requests should be
// suppressed (reader path unported — S7b-D1; paid down when anim playback
// is ported). Matches TS Player.ts:321 (field) + PlayerOps.ts:1171-1172.
func (p *Player) SetAnimProtect(v int) { p.animProtect = v }
```

to:

```go
// SetAnimProtect implements script.ActivePlayer.SetAnimProtect. Stores the
// anim-protect flag; when nonzero, PlayAnim suppresses in-engine animation
// requests (NAI-56 wired the reader at PlayAnim's L1842 gate). Matches TS
// Player.ts:321 (field) + PlayerOps.ts:1171-1172.
func (p *Player) SetAnimProtect(v int) { p.animProtect = v }
```

This removes the `S7b-D1` reference now that the deviation is closed.

- [ ] **Step 5: Run the full world-package suite (regression gate)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: PASS. Specifically `TestAnimateSetsMask` and
`TestResetMasksClearsEphemerals` (both call `p.Animate(123, 5)`, not
`PlayAnim`) remain green — the test-only `Animate` setter is untouched.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_script.go modules/world/player_anim_test.go
git commit --no-gpg-sign -m "feat(world): NAI-56 T1 — gate PlayAnim on animProtect (closes S7b-D1)"
```

---

## Task 2: Verification + close-trailer commit

**Files:** none (verification only).

- [ ] **Step 1: Verify no stale `S7b-D1` references in source code**

Run: `rg "S7b-D1" pkg/ modules/ cmd/`

Expected: zero matches in `pkg/`, `modules/`, `cmd/` source paths.
Both the `PlayAnim` doc-comment (T1 Step 3) and the `SetAnimProtect`
doc-comment (T1 Step 4b) were refreshed; the tag should be fully retired
from the source tree.

If any stale match surfaces, edit it out and add a fixup commit before
proceeding.

- [ ] **Step 2: Verify `S7b-D1` references in `nai_followups.md` memory
       are historical-only**

Run: `rg "S7b-D1" /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/`

Expected: matches in `nai_followups.md` (the historical entry from
NAI-52 spec context); these are historical and acceptable. The memory
update is part of the post-close handoff, not this commit.

- [ ] **Step 3: Run the full repo suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 4: Verify the gate has the expected single new branch**

Run: `rg "p\.animProtect" modules/world/`

Expected: matches at `player.go` (field declaration), `player_script.go`
(`SetAnimProtect` setter + `PlayAnim` gate), and `player_anim_test.go`
(test fixtures). No other writes or reads.

- [ ] **Step 5: Close commit (memory trailer)**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-56 — PlayAnim animProtect gate; closes S7b-D1

Closes deviation: S7b-D1 (Player.animProtect reader now wired in
(*Player).PlayAnim per TS Player.ts:1842).

New deviation: NAI-56-D1 — (*Player).PlayAnim and (*Npc).Animate skip
TS playAnimation's SeqType.count bounds-reject (Player.ts:1842 /
Npc.ts:454) and priority-comparison overwrite gate (Player.ts:1846 /
Npc.ts:458). Both depend on the unported SeqType config registry.
Closure: future SeqType config-port sub-spec.

Net deviation tally: 21 → 21 (one closed, one introduced).

Closes memory: nai_followups.md
EOF
)"
```

---

## Self-review

**Spec coverage:**
- Single behavior change: `(*Player).PlayAnim` early-returns when
  `animProtect != 0`. T1 ✓
- Test plan items 1-2 (gate + baseline): T1 ✓
- Doc-comment refresh on `SetAnimProtect` (S7b-D1 retirement): T1 Step 3
  / T2 Step 1 ✓
- Verification of grep-clean source tree + retired tag: T2 ✓

**Type/signature consistency:**
- `(*Player).PlayAnim(seqID, delay int)` signature unchanged.
- `(*Player).animProtect` is `int` (`player.go:169`); `!= 0` truthy test
  is the canonical Go translation of TS `if (this.animProtect)` on a
  numeric field.
- No interface change required (`script.ActivePlayer.PlayAnim` signature
  is `PlayAnim(seqID, delay int)` per `pkg/script/active.go:113`).

**Placeholder scan:** No TBD / TODO / "implement later" / "similar to"
language. Every step shows full code; tests show full bodies.

**Deviation-tag consistency:** `S7b-D1` retired in T1 (gate added) + T2
Step 1 (doc-comment refresh) + T2 Step 2 (verification). New tag
`NAI-56-D1` introduced in T2 close-commit narrative; tracking will be
applied to `nai_followups.md` as part of the post-close handoff (memory
entry, not source code).

**Compressed-cadence justification:** Source-LOC budget — gate (3 LOC) +
doc-comment refresh (~5 LOC) ≈ 8 source LOC. Test additions (~50 LOC) are
test-only and don't count against the source-LOC threshold per
`compressed_cadence.md`. ≤15 threshold easily met. No formal whole-impl
review required.

**Test-fixture sanity check** (per `plan_runnable_test_fixtures.md`):
- `newTestPlayer(t)` is the established fixture in `player_masks_test.go`
  and elsewhere; returns a `(*Player, *clientConn)` pair with default
  field values (animID=-1, animDelay=-1, animProtect=0, masks=0).
- `rsbuf.MaskAnim` is the existing exported constant used by every
  player-mask test.
- No mock-recorder fields touched (the gate test reads `Player`'s real
  state, not via interface).
