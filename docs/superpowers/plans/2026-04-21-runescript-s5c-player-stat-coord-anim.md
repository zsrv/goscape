# RuneScript S5c: Player Stat/Coord/Facing/Anim Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register 24 PlayerOps handlers (23 active + 1 P_WALK stub) covering stat read/write, coord/facing query, teleport, and the animation-setter family. Zero new wire code — mutations flow through existing mask + dirty-compare plumbing.

**Architecture:** One handler file (`handlers_player.go`), ~15 new methods on `script.ActivePlayer`, thin impls on `*Player` that write existing fields. End-to-end test validates TeleJump via script.

**Tech Stack:** Go 1.22+, existing pkg/script, existing Player mask system.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5c-player-stat-coord-anim-design.md`](../specs/2026-04-21-runescript-s5c-player-stat-coord-anim-design.md)

---

## Task 1: Extend `ActivePlayer` + `mockPlayer`

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `pkg/script/runner_test.go`

- [ ] **Step 1: Add 15 methods to `ActivePlayer`** per the spec's "Interface extensions" section. Group with a `// S5c:` comment block. Exact method list:

```go
// S5c: position / facing / teleport.
CoordPacked() int
TeleJump(x, z, level int)
Teleport(x, z, level int)
FaceSquare(x, z int)

// S5c: stats.
Stat(id int) int
StatBase(id int) int
StatXP(id int) int
SetCurLevel(id int, level int)
AddXP(id int, xp int)

// S5c: animation.
PlayAnim(seqID, delay int)
PlaySpotAnim(id, height, delay int)
SetReadyAnim(seqID int)
SetTurnAnim(seqID int)
SetWalkAnim(seqID int)
SetWalkAnimB(seqID int)
SetWalkAnimL(seqID int)
SetWalkAnimR(seqID int)
SetRunAnim(seqID int)
```

- [ ] **Step 2: Extend `mockPlayer` with matching fields + method impls**

In `pkg/script/runner_test.go`, add capture fields (a struct with the last-call args is fine, e.g. `lastTeleJump struct{ x, z, level int }`) and impls that record the calls. Use a `levels [21]int`, `baseLevels [21]int`, `statXP [21]int` trio for stat returns so tests can pre-seed.

- [ ] **Step 3: Build the package to confirm the interface compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
```

Expected: package builds. `modules/world` will fail until Task 3 — that's OK.

- [ ] **Step 4: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5c ActivePlayer extensions for stat/coord/anim

Adds 15 methods covering position queries, teleport, facing, stat
read/write, XP, and the animation-setter family. mockPlayer fixture
gains matching capture fields so handler tests can verify each call.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Handlers + tests + map registration

**Files:**
- Create: `pkg/script/handlers_player.go`
- Create: `pkg/script/handlers_player_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Implement the 24 handlers per spec**

Key rules the implementer MUST verify against `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts`:

- **Pop order** for STAT_ADD / STAT_SUB / STAT_BOOST / STAT_DRAIN / STAT_HEAL: TS uses `popInts(3) = [stat, constant, percent]`. Stack top is `percent` (pop first), then `constant`, then `stat` at bottom.
- **Exact formula** for each stat-modifier handler: read TS lines 501–616 and mirror exactly. Spec's `STAT_ADD` formula is a guess — verify.
- **XP scaling in STAT_ADVANCE**: TS pops `(stat, xp)` where xp is in the wire-scaled unit. Player.AddXP is responsible for any scaling; the handler just passes through.
- **Coord pack/unpack**: `(level << 28) | (x << 14) | z`. Level mask is `0x3` (2-bit, since rev 225 has 4 levels), x/z mask is `0x3fff` (14-bit).
- **`P_WALK`**: stub only — pop one int, `slog.Debug`, return nil.
- **All stat ops validate `id < 0 || id >= 21`** and return an error — don't silently clamp.

Follow the same pattern as existing handlers (errors.New for fail-fast, `s.Pointers&PtrActivePlayer` check, etc.).

- [ ] **Step 2: Register 24 handlers in `pkg/script/handlers.go`**

Add an `// S5c: player stat/coord/facing/anim.` block at the end of the handlers map literal. Keep entries in the order the handler file defines them for readability.

- [ ] **Step 3: Write table-driven tests in `pkg/script/handlers_player_test.go`**

Pattern per spec's "Testing" section:
- Stat-read tests seed `mockPlayer.levels[i]` and assert the handler pushes the right value.
- Mutator tests pre-seed state, run the handler via a 1-instruction script, then assert `mockPlayer.setCurLevelCalls` / `mockPlayer.addXPCalls` have the expected args.
- Coord round-trip: `TestPTeleJumpUnpacksCoord` — build packed coord `(level<<28)|(x<<14)|z` for a known triple, run handler, assert `mp.lastTeleJump == {x, z, level}`.
- At minimum 20 sub-tests covering each of the 24 handlers + one "requires active player" case for the subset that dereferences Self.

- [ ] **Step 4: Run pkg/script tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5c player opcodes (23 handlers + P_WALK stub)

STAT family (10): STAT, STAT_BASE, STAT_TOTAL, STAT_ADD, STAT_SUB,
STAT_BOOST, STAT_DRAIN, STAT_HEAL, STAT_ADVANCE, STAT_RANDOM.
Coord/facing (4): COORD, FACESQUARE, P_TELEPORT, P_TELEJUMP.
Animation (9): ANIM, SPOTANIM_PL, READYANIM/TURNANIM/RUNANIM and the
four WALKANIM variants.

P_WALK registered as a Debug-log stub until pathfinder integration.
Formulas for STAT_ADD/SUB/BOOST/DRAIN/HEAL verified against TS
PlayerOps.ts. Coord packing matches CoordGrid.packCoord's
(level<<28)|(x<<14)|z layout with 2-bit level, 14-bit x/z masks.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `*Player` method impls

**Files:**
- Modify: `modules/world/player_script.go`
- Possibly modify: `modules/world/player.go` (if spotanim fields missing)
- Possibly modify: `modules/world/player_masks.go` (if mask constants missing)

- [ ] **Step 1: Implement the 15 `*Player` methods** per spec's "Player method impls" section.

Before implementing:
1. Open `modules/world/player.go` and `modules/world/player_masks.go`. Confirm the exact names of: stat-mutation mask (probably `MaskAppearance` or a dedicated flag), face-coord mask (likely `MaskFaceCoord`), anim mask (likely `MaskAnim`), spotanim mask + fields.
2. If spotanim fields are missing from `Player`, add them: `spotanim int`, `spotanimHeight int`, `spotanimDelay int`, plus a `MaskSpotAnim` constant if needed. Follow the existing mask-bit pattern and wire the clear in `ResetMasks`.
3. `SetCurLevel` writes to `p.levels[id]` directly — no explicit dirty flag (the existing `lastLevels` diff in `updateStats()` handles it).
4. `AddXP` only adds to `p.stats[id]` for S5c. Base-level re-derivation is a TODO comment pointing at the getLevelByExp table.

- [ ] **Step 2: Full build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean build. `var _ script.ActivePlayer = (*Player)(nil)` assertion passes.

- [ ] **Step 3: Run all existing tests to ensure no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/player_script.go modules/world/player.go modules/world/player_masks.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player impls for S5c stat/coord/anim methods

CoordPacked returns (level<<28)|(x<<14)|z. Teleport/TeleJump set the
existing tele/jump one-shot flags — ResetMasks clears after emission.
FaceSquare writes faceSquareX/Z and flips MaskFaceCoord. Stat read
methods index levels/baseLevels/stats; SetCurLevel writes levels
directly and the existing lastLevels dirty-compare handles wire sync.
AddXP adds to stats; base-level re-derivation TBD. BAS animation
setters write existing readyanim/turnanim/walkanim*/runanim fields.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: End-to-end wire tests

**Files:**
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Add `TestTelejumpViaScript`**

```go
func TestTelejumpViaScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script: push_constant_int <packed>, p_telejump, return
	// Pack: level=0, x=3222, z=3222 (Lumbridge center).
	packed := (0 << 28) | (3222 << 14) | 3222
	sf := &script.ScriptFile{
		Name: "[telejump,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPTeleJump,
			script.OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.runScript(sf, p, true, nil, nil)

	if p.x != 3222 || p.z != 3222 || p.level != 0 {
		t.Errorf("coord: got (%d,%d,%d), want (3222,3222,0)", p.x, p.z, p.level)
	}
	if !p.tele {
		t.Error("tele flag: got false, want true")
	}
	if !p.jump {
		t.Error("jump flag: got false, want true")
	}
}
```

- [ ] **Step 2: Add `TestStatAdvanceViaScript`**

```go
func TestStatAdvanceViaScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.stats[3] = 100 // skill 3 starts at 100 XP

	// Script: push 3, push 50, stat_advance, return
	sf := &script.ScriptFile{
		Name: "[statadvance,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt, // push stat id
			script.OpPushConstantInt, // push xp delta
			script.OpStatAdvance,
			script.OpReturn,
		},
		IntOperands:      []int32{3, 50, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	s.runScript(sf, p, true, nil, nil)

	if got := int(p.stats[3]); got != 150 {
		t.Errorf("p.stats[3] after AddXP(3, 50): got %d, want 150", got)
	}
}
```

- [ ] **Step 3: Run tests + race + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestTelejump|TestStatAdvance' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): end-to-end S5c script → player-state tests

TestTelejumpViaScript: push_constant_int packed_coord + p_telejump
warps the player to (3222, 3222, 0) and flips tele/jump one-shot flags.
TestStatAdvanceViaScript: stat_advance adds XP to p.stats[id].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Checklist

After completing all tasks:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` — clean build
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — all tests pass
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` — no race warnings
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — no vet issues
- [ ] Handler count in `handlers.go` now reads **102** (78 after S5b + 24 S5c, including P_WALK stub).
