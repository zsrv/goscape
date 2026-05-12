# NAI-184 — Port tractable D3 cheat cohort + correct NAI-183 reboot-trio placement

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 11 of the 28 unported `ClientCheatHandler` cheats and correct the NAI-183 misclassification that placed `reboot`/`slowreboot`/`serverdrop` in the wrong outer block.

**Architecture:** Add the missing TS L189 admin `>=3` outer-guard block to `handleClientCheat`. Each cheat arm becomes a `case` in the appropriate switch. Two new methods on `*Player` (`SetStat`, `InvAdd`) mirror TS `Player.setLevel` and `Player.invAdd` bare-entity semantics.

**Tech Stack:** Go 1.26+. Spec at `docs/superpowers/specs/2026-05-12-nai-184-cheat-cohort-port-design.md`.

---

## Task ordering rationale

T0 + T1 + T5 are foundation pieces that subsequent cheat-arm tasks depend on. T2 is the structural restructure that everything else slots into. T3-T8 add cheat arms (each task fully test-drives one cohort). T9 is the close commit.

```
T0 (PlayerStatMap) ─┐
T1 (Player.SetStat) ┤
T2 (admin block +   ├─→ T3 (fly/naive/random)
    reboot relocate)│   T4 (setstat/advancestat/minme) ← needs T0+T1
                    │   T5 (Player.InvAdd) → T6 (give/givemany)
                    │   T7 (snapshot)
                    │   T8 (teleother/teleto)
                    └─→ T9 (close: DEVIATION rewrite + memory trailer)
```

T0, T1, T5 are independent and can run in parallel if the controller chooses. T2 must complete before T3-T8.

---

## Task 0: Add `PlayerStatMap` + `PlayerStatEnabled` to pkg/objtype

**Files:**
- Modify: `pkg/objtype/playerstat.go`
- Test: `pkg/objtype/playerstat_test.go`

- [ ] **Step 1: Write the failing test**

Add at the bottom of `pkg/objtype/playerstat_test.go`:

```go
func TestPlayerStatMap_AllNamesResolveToValidIndices(t *testing.T) {
	// Mirrors TS PlayerStat.ts:25-47. Every name in PlayerStatMap must
	// map to its corresponding PlayerStat* constant.
	cases := map[string]int{
		"ATTACK":      PlayerStatAttack,
		"DEFENCE":     PlayerStatDefence,
		"STRENGTH":    PlayerStatStrength,
		"HITPOINTS":   PlayerStatHitpoints,
		"RANGED":      PlayerStatRanged,
		"PRAYER":      PlayerStatPrayer,
		"MAGIC":       PlayerStatMagic,
		"COOKING":     PlayerStatCooking,
		"WOODCUTTING": PlayerStatWoodcutting,
		"FLETCHING":   PlayerStatFletching,
		"FISHING":     PlayerStatFishing,
		"FIREMAKING":  PlayerStatFiremaking,
		"CRAFTING":    PlayerStatCrafting,
		"SMITHING":    PlayerStatSmithing,
		"MINING":      PlayerStatMining,
		"HERBLORE":    PlayerStatHerblore,
		"AGILITY":     PlayerStatAgility,
		"THIEVING":    PlayerStatThieving,
		"STAT18":      PlayerStat18,
		"STAT19":      PlayerStat19,
		"RUNECRAFT":   PlayerStatRunecraft,
	}
	if len(PlayerStatMap) != len(cases) {
		t.Fatalf("PlayerStatMap len = %d, want %d", len(PlayerStatMap), len(cases))
	}
	for name, want := range cases {
		got, ok := PlayerStatMap[name]
		if !ok {
			t.Errorf("PlayerStatMap[%q] missing", name)
			continue
		}
		if got != want {
			t.Errorf("PlayerStatMap[%q] = %d, want %d", name, got, want)
		}
	}
}

func TestPlayerStatEnabled_MatchesTSPattern(t *testing.T) {
	// TS PlayerStat.ts:53. False only at STAT18, STAT19.
	want := [PlayerStatCount]bool{
		true, true, true, true, true, true, true, true, true, true,
		true, true, true, true, true, true, true, true, false, false, true,
	}
	if PlayerStatEnabled != want {
		t.Errorf("PlayerStatEnabled = %v, want %v", PlayerStatEnabled, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestPlayerStatMap_AllNamesResolveToValidIndices|TestPlayerStatEnabled_MatchesTSPattern' -v`
Expected: FAIL — `PlayerStatMap` and `PlayerStatEnabled` undefined.

- [ ] **Step 3: Add the new exports**

In `pkg/objtype/playerstat.go`, after the existing `const ( PlayerStatAttack = 0 ... )` block, add:

```go
// PlayerStatMap maps uppercase stat name → stat index. Mirrors TS
// PlayerStatMap (PlayerStat.ts:25-47). Used by ::setstat / ::advancestat
// cheat parsing in modules/world/handlers_game.go (NAI-184).
var PlayerStatMap = map[string]int{
	"ATTACK":      PlayerStatAttack,
	"DEFENCE":     PlayerStatDefence,
	"STRENGTH":    PlayerStatStrength,
	"HITPOINTS":   PlayerStatHitpoints,
	"RANGED":      PlayerStatRanged,
	"PRAYER":      PlayerStatPrayer,
	"MAGIC":       PlayerStatMagic,
	"COOKING":     PlayerStatCooking,
	"WOODCUTTING": PlayerStatWoodcutting,
	"FLETCHING":   PlayerStatFletching,
	"FISHING":     PlayerStatFishing,
	"FIREMAKING":  PlayerStatFiremaking,
	"CRAFTING":    PlayerStatCrafting,
	"SMITHING":    PlayerStatSmithing,
	"MINING":      PlayerStatMining,
	"HERBLORE":    PlayerStatHerblore,
	"AGILITY":     PlayerStatAgility,
	"THIEVING":    PlayerStatThieving,
	"STAT18":      PlayerStat18,
	"STAT19":      PlayerStat19,
	"RUNECRAFT":   PlayerStatRunecraft,
}

// PlayerStatEnabled mirrors TS PlayerStat.ts:53. False entries (STAT18,
// STAT19) are unused 2004-era reserved slots; ::minme skips them.
var PlayerStatEnabled = [PlayerStatCount]bool{
	true, true, true, true, true, true, true, true, true, true,
	true, true, true, true, true, true, true, true, false, false, true,
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -v`
Expected: all PASS, including the two new ones.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/playerstat.go pkg/objtype/playerstat_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-184 T0 — add PlayerStatMap + PlayerStatEnabled

Ports TS PlayerStat.ts:25-47 (PlayerStatMap) and :53 (PlayerStatEnabled).
Consumed by ::setstat / ::advancestat / ::minme cheat handlers in NAI-184 T4.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1: Add `Player.SetStat` method

**Files:**
- Modify: `modules/world/player_script.go` (insert near existing `SetCurLevel` at line 644)
- Test: `modules/world/player_script_test.go`

- [ ] **Step 1: Write the failing test**

Add to `modules/world/player_script_test.go`:

```go
func TestSetStat_WritesBaseCurAndXPClamped(t *testing.T) {
	cases := []struct {
		name     string
		level    int
		wantLvl  uint8
		wantXP   int32
	}{
		{"normal mid", 50, 50, int32(objtype.GetExpByLevel(50))},
		{"clamps to 1 from 0", 0, 1, int32(objtype.GetExpByLevel(1))},
		{"clamps to 1 from -5", -5, 1, int32(objtype.GetExpByLevel(1))},
		{"clamps to 99 from 100", 100, 99, int32(objtype.GetExpByLevel(99))},
		{"clamps to 99 from 150", 150, 99, int32(objtype.GetExpByLevel(99))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{}
			p.SetStat(objtype.PlayerStatAttack, tc.level)
			if p.baseLevels[objtype.PlayerStatAttack] != tc.wantLvl {
				t.Errorf("baseLevels = %d, want %d", p.baseLevels[objtype.PlayerStatAttack], tc.wantLvl)
			}
			if p.levels[objtype.PlayerStatAttack] != tc.wantLvl {
				t.Errorf("levels = %d, want %d", p.levels[objtype.PlayerStatAttack], tc.wantLvl)
			}
			if p.stats[objtype.PlayerStatAttack] != tc.wantXP {
				t.Errorf("stats = %d, want %d", p.stats[objtype.PlayerStatAttack], tc.wantXP)
			}
		})
	}
}

func TestSetStat_OOBStatDropsSilently(t *testing.T) {
	p := &Player{}
	p.SetStat(-1, 50)
	p.SetStat(21, 50)
	// No state mutation expected, no panic.
	for i := 0; i < objtype.PlayerStatCount; i++ {
		if p.baseLevels[i] != 0 || p.levels[i] != 0 || p.stats[i] != 0 {
			t.Errorf("stat %d mutated after OOB SetStat", i)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSetStat_' -v`
Expected: FAIL — `SetStat` undefined on `*Player`.

- [ ] **Step 3: Implement the method**

In `modules/world/player_script.go`, immediately after the existing `SetCurLevel` (currently at line 644), add:

```go
// SetStat clamps level to [1, 99] and writes baseLevels, levels, and
// stats (XP) for the given stat slot. Mirrors TS Player.setLevel
// (Player.ts:1823-1834). Used by ::setstat and ::minme cheats (NAI-184).
//
// DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD: TS additionally
// recomputes combatLevel and calls buildAppearance(appearanceInv) on
// change (TS L1830-1833). Combat-level recompute + appearance rebuild
// are deferred to a future combat sub-spec.
func (p *Player) SetStat(stat, level int) {
	if !statBounds(stat) {
		return
	}
	if level < 1 {
		level = 1
	}
	if level > 99 {
		level = 99
	}
	p.baseLevels[stat] = uint8(level)
	p.levels[stat] = uint8(level)
	p.stats[stat] = int32(objtype.GetExpByLevel(level))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSetStat_' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-184 T1 — Player.SetStat full-triple write

Mirrors TS Player.setLevel (Player.ts:1823-1834): writes baseLevels,
levels, and stats[XP] for the given slot, clamped to [1, 99]. Combat-level
recompute deferred (DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD).

Consumed by NAI-184 T4 (::setstat, ::minme).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Restructure outer-guard hierarchy + relocate reboot trio

**Files:**
- Modify: `modules/world/handlers_game.go:336-467` (`handleClientCheat` body)
- Modify: `modules/world/handlers_game_test.go` (6 tests modified, 1 deleted)

This is the structural change that everything else depends on. The body of `handleClientCheat` becomes four sequential `if` blocks (sessionLog, dev, admin, super-mod). The reboot trio moves from dev to admin.

- [ ] **Step 1: Pre-flight grep**

Run: `rg 'DEVIATION-NAI-182-D3' modules/ pkg/ cmd/`
Note the hit count. Should be exactly 1 (the comment in handlers_game.go).

Run: `rg 'TS-faithful dead code' modules/world/handlers_game.go`
Expected: 2 hits (reboot comment + slowreboot comment). These will be retired in this task.

Run: `grep -nE 'TestHandleClientCheat_(Reboot|SlowReboot|ServerDrop|RebootCheats|NodeProductionTrue)' modules/world/handlers_game_test.go`
Note the 7 test names + line numbers — these are the tests modified or deleted below.

- [ ] **Step 2: Write the failing test (inverted reboot-trio gate)**

Modify the 6 existing tests at their current locations in `modules/world/handlers_game_test.go`. Pattern: change `p.staffModLevel = 4` → `3`. For `TestHandleClientCheat_RebootCheats_StaffGate` change the loop boundary (or single-shot assertion) from staffModLevel `1`/`2`/`3` to `0`/`1`/`2` and rewrite the doc-comment to "admin-block staff gate per TS L189".

Concretely, for each of these 6 tests, find the setup line `p.staffModLevel = 4` (or similar) and change to `3`:
1. `TestHandleClientCheat_Reboot_DeadUnderDefaultConfig`
2. `TestHandleClientCheat_SlowReboot_NoArgs_DeadUnderDefaultConfig`
3. `TestHandleClientCheat_SlowReboot_WithArg_DeadUnderDefaultConfig`
4. `TestHandleClientCheat_SlowReboot_NonInteger_DeadUnderDefaultConfig`
5. `TestHandleClientCheat_ServerDrop_ClosesConn`
6. `TestHandleClientCheat_RebootCheats_StaffGate` — boundary moves from `<4` to `<3`; rewrite doc-comment line.

DELETE the test `TestHandleClientCheat_NodeProductionTrue_DevBlockShortCircuits` entirely (its premise no longer holds — serverdrop now in admin block with no NP outer gate, so NP=true does NOT collapse its dispatch).

- [ ] **Step 3: Run tests to verify the modified set FAILS against current handler**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(Reboot|SlowReboot|ServerDrop|RebootCheats)' -v`
Expected: At least one FAIL — because the current handler still places these cases in the dev block (gate `<4` not `<3`).

- [ ] **Step 4: Restructure `handleClientCheat` body**

In `modules/world/handlers_game.go`, replace the body of `handleClientCheat` (currently lines 336-467) with the four-block structure. The pre-cheat setup (lines 337-358) is unchanged. The new body keeps the L52 sessionLog tier unchanged, keeps the dev block but EMPTIES its switch (the reboot trio moves out; no new arms in T2), adds the NEW L189 admin block with the relocated reboot/slowreboot/serverdrop, keeps the super-mod block unchanged.

Replace the comment block at lines 360-366 with:

```go
// DEVIATION-NAI-184-D2-D3-CARRYFORWARD — supersedes
// DEVIATION-NAI-182-D3-OTHER-CHEATS. 17 TS ClientCheatHandler cheats
// remain unported:
//   Dev block (!NP && >=4): reload, rebuild, speed.
//   Admin block (>=3):      setvar, setvarother, getvar, getvarother,
//                           giveother, givecrap, broadcast, locadd,
//                           npcadd, openmain.
//   Super-mod (>=2):        setvis, ban, mute, kick.
// Each is blocked on a missing subsystem (VarPlayerType.GetByName,
// World.broadcastMes, runtime tick-rate mutation, login moderation
// callbacks, dynamic Loc/NPC spawn, Visibility plumbing). Deferred
// to follow-up sub-specs.
```

Replace lines 379-412 (the entire current dev block) with:

```go
	// TS ClientCheatHandler.ts:56 — developer block. Gated on
	// `!Environment.NODE_PRODUCTION && staffModLevel >= 4`. Goscape
	// reads s.cfg.NodeProduction (modules/world/config.go:43, default
	// false). NAI-183.
	if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
		switch parts[0] {
		// (NAI-184 T3 will add fly/naive/random here.)
		}
	}

	// TS ClientCheatHandler.ts:189 — admin block. Gated on
	// staffModLevel >= 3. NAI-184 T2 added this outer guard and
	// relocated reboot/slowreboot/serverdrop here from the dev block
	// (NAI-183 misclassified them — see spec §2.1).
	if p.staffModLevel >= 3 {
		switch parts[0] {
		case "reboot":
			// TS L360-364. Production-only via inner && NodeProduction;
			// under default NodeProduction=false, this arm is dead.
			if p.client.server.cfg.NodeProduction {
				p.client.server.rebootTimer(0)
			}
		case "slowreboot":
			// TS L365-373. Production-only via inner && NodeProduction;
			// default 30s when args missing. Formula: ticks = ceil(s * 1000/600).
			if p.client.server.cfg.NodeProduction {
				seconds := parseIntOr(args, 30)
				ticks := int(math.Ceil(float64(seconds) * 1000.0 / 600.0))
				p.client.server.rebootTimer(ticks)
			}
		case "serverdrop":
			// TS L374-376 player.terminate(). No NP gate — fires at >=3
			// regardless of NodeProduction. Closes the TCP conn without
			// removing the player from s.players; the next reconnect hits
			// this player's slot and runs the onReconnect path.
			if p.client != nil && p.client.conn != nil {
				_ = p.client.conn.Close()
			}
		// (NAI-184 T4–T8 will add the remaining admin-block arms here.)
		}
	}
```

The L483 super-mod block (currently at lines 414-466) stays byte-identical for T2. T8 will add the `teleto` case to it.

- [ ] **Step 5: Run all handler-cheat tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_' -v`
Expected: all PASS, including the 6 modified tests. Number of test cases run should be `current_count - 1` (the deleted NodeProductionTrue test).

- [ ] **Step 6: Run full modules/world test suite to catch regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): NAI-184 T2 — add TS L189 admin block; relocate reboot trio

Adds the missing L189 admin (>=3) outer-guard block between the existing
L56 dev block and L483 super-mod block. Relocates reboot/slowreboot/
serverdrop from the dev block to the admin block per TS placement
(NAI-183 had misclassified these — see spec §2.1).

Test changes:
- 6 reboot-trio tests inverted: staffModLevel 4 → 3 (admin tier).
- TestHandleClientCheat_NodeProductionTrue_DevBlockShortCircuits deleted —
  serverdrop now in admin block with no NP outer guard, so NP=true does
  NOT collapse its dispatch.
- Retires NAI-183 "TS-faithful dead code" comments on reboot/slowreboot —
  in correct placement these arms fire under NP=true, not dead.

DEVIATION-NAI-182-D3-OTHER-CHEATS comment rewritten as
DEVIATION-NAI-184-D2-D3-CARRYFORWARD with corrected 17-cheat enumeration
(adds setvar, which NAI-182 D3 list omitted).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Port dev-block cheats (fly / naive / random)

**Files:**
- Modify: `modules/world/handlers_game.go` (dev block switch)
- Test: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_Fly_TogglesStrategy(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 4
	p.moveStrategy = MoveStrategySmart
	drainConn(t, s, p)

	dispatchTeleCheat(t, p, "fly")
	if p.moveStrategy != MoveStrategyFly {
		t.Errorf("after ::fly: moveStrategy = %v, want Fly", p.moveStrategy)
	}
	// MessageGame contents validated via the per-player out-stream buffer
	// (existing pattern — see TestHandleClientCheat_GetCoord_*).
	msgs := drainMessageGame(t, s, p)
	if !containsMessage(msgs, "Changed move strategy: fly") {
		t.Errorf("missing 'Changed move strategy: fly' in %v", msgs)
	}

	dispatchTeleCheat(t, p, "fly")
	if p.moveStrategy != MoveStrategySmart {
		t.Errorf("after second ::fly: moveStrategy = %v, want Smart", p.moveStrategy)
	}
	msgs = drainMessageGame(t, s, p)
	if !containsMessage(msgs, "Changed move strategy: smart") {
		t.Errorf("missing 'Changed move strategy: smart' in %v", msgs)
	}
}

func TestHandleClientCheat_Naive_TogglesStrategy(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 4
	p.moveStrategy = MoveStrategySmart
	drainConn(t, s, p)

	dispatchTeleCheat(t, p, "naive")
	if p.moveStrategy != MoveStrategyNaive {
		t.Errorf("after ::naive: moveStrategy = %v, want Naive", p.moveStrategy)
	}
	msgs := drainMessageGame(t, s, p)
	if !containsMessage(msgs, "Naive move strategy: naive") {
		t.Errorf("missing 'Naive move strategy: naive' in %v", msgs)
	}

	dispatchTeleCheat(t, p, "naive")
	if p.moveStrategy != MoveStrategySmart {
		t.Errorf("after second ::naive: moveStrategy = %v, want Smart", p.moveStrategy)
	}
	msgs = drainMessageGame(t, s, p)
	if !containsMessage(msgs, "Naive move strategy: smart") {
		t.Errorf("missing 'Naive move strategy: smart' in %v", msgs)
	}
}

func TestHandleClientCheat_Random_SetsAfkEventReady(t *testing.T) {
	_, p := teleTestPlayer(t)
	p.staffModLevel = 4
	p.afkEventReady = false
	dispatchTeleCheat(t, p, "random")
	if !p.afkEventReady {
		t.Errorf("after ::random: afkEventReady = false, want true")
	}
}
```

**Helper note**: `drainMessageGame` and `containsMessage` are NOT existing helpers — they need to be added (or use whatever pattern existing `MessageGame` tests use). Pre-flight grep for the canonical pattern:

Run: `grep -n 'MessageGame' modules/world/handlers_game_test.go | head -5`

If existing tests inspect MessageGame via direct field access (e.g. `p.outQueue` or a packet drain), use that pattern instead. If no helper exists yet, add one at the bottom of `handlers_game_test.go` mirroring the existing drain pattern.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(Fly|Naive|Random)' -v`
Expected: FAIL — the dev block's switch is empty (only structural placeholders).

- [ ] **Step 3: Add the three arms to the dev block**

In `modules/world/handlers_game.go`, inside the dev block's `switch parts[0] {` (added by T2), add:

```go
		case "fly":
			// TS L168-175 — toggles between Fly and Smart strategies and
			// emits a MessageGame describing the current state.
			if p.moveStrategy == MoveStrategyFly {
				p.moveStrategy = MoveStrategySmart
			} else {
				p.moveStrategy = MoveStrategyFly
			}
			if p.moveStrategy == MoveStrategyFly {
				p.MessageGame("Changed move strategy: fly")
			} else {
				p.MessageGame("Changed move strategy: smart")
			}
		case "naive":
			// TS L176-183 — toggles between Naive and Smart.
			if p.moveStrategy == MoveStrategyNaive {
				p.moveStrategy = MoveStrategySmart
			} else {
				p.moveStrategy = MoveStrategyNaive
			}
			if p.moveStrategy == MoveStrategyNaive {
				p.MessageGame("Naive move strategy: naive")
			} else {
				p.MessageGame("Naive move strategy: smart")
			}
		case "random":
			// TS L184-186 — primes the AFK event for the next tick.
			p.afkEventReady = true
```

Remove the placeholder comment `// (NAI-184 T3 will add fly/naive/random here.)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(Fly|Naive|Random)' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-184 T3 — port ::fly / ::naive / ::random dev cheats

Mirrors TS ClientCheatHandler.ts:168-186. All three are dev-block arms
(!NodeProduction && staffModLevel >= 4). Fly/naive toggle moveStrategy
between Smart and Fly/Naive respectively, emitting a MessageGame each
time. Random primes afkEventReady for the next tick.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Port admin-block stat cheats (setstat / advancestat / minme)

Depends on T0 (PlayerStatMap, PlayerStatEnabled) and T1 (Player.SetStat).

**Files:**
- Modify: `modules/world/handlers_game.go` (admin block switch)
- Test: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_SetStat_SetsBaseCurAndXP(t *testing.T) {
	_, p := teleTestPlayer(t)
	p.staffModLevel = 3

	dispatchTeleCheat(t, p, "setstat attack 50")
	if p.baseLevels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("baseLevels[ATTACK] = %d, want 50", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("levels[ATTACK] = %d, want 50", p.levels[objtype.PlayerStatAttack])
	}
	wantXP := int32(objtype.GetExpByLevel(50))
	if p.stats[objtype.PlayerStatAttack] != wantXP {
		t.Errorf("stats[ATTACK] = %d, want %d", p.stats[objtype.PlayerStatAttack], wantXP)
	}

	// Unknown stat name: no mutation.
	p.baseLevels[objtype.PlayerStatDefence] = 11
	dispatchTeleCheat(t, p, "setstat fake_stat 99")
	if p.baseLevels[objtype.PlayerStatDefence] != 11 {
		t.Errorf("unknown stat mutated DEFENCE: got %d, want 11", p.baseLevels[objtype.PlayerStatDefence])
	}
}

func TestHandleClientCheat_AdvanceStat_ZerosThenAddsXP(t *testing.T) {
	_, p := teleTestPlayer(t)
	p.staffModLevel = 3
	// Pre-populate to verify the L428-431 zero-reset before AddXP.
	p.stats[objtype.PlayerStatAttack] = 999999
	p.baseLevels[objtype.PlayerStatAttack] = 30
	p.levels[objtype.PlayerStatAttack] = 30

	dispatchTeleCheat(t, p, "advancestat attack 50")

	wantXP := int32(objtype.GetExpByLevel(50))
	if p.stats[objtype.PlayerStatAttack] != wantXP {
		t.Errorf("stats[ATTACK] after advancestat = %d, want %d (= GetExpByLevel(50))",
			p.stats[objtype.PlayerStatAttack], wantXP)
	}
	// AddXP recomputes baseLevels from XP; at GetExpByLevel(50) XP the
	// derived baseLevel is 50.
	if p.baseLevels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("baseLevels[ATTACK] after advancestat = %d, want 50", p.baseLevels[objtype.PlayerStatAttack])
	}
	// Un-buffed branch of AddXP advances levels alongside baseLevels.
	if p.levels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("levels[ATTACK] after advancestat = %d, want 50", p.levels[objtype.PlayerStatAttack])
	}
}

func TestHandleClientCheat_MinMe_AllEnabledStatsSetTo1ExceptHitpoints(t *testing.T) {
	_, p := teleTestPlayer(t)
	p.staffModLevel = 3
	// Pre-populate all stats to a high value.
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 99
		p.levels[i] = 99
		p.stats[i] = int32(objtype.GetExpByLevel(99))
	}

	dispatchTeleCheat(t, p, "minme")

	for i := 0; i < objtype.PlayerStatCount; i++ {
		if !objtype.PlayerStatEnabled[i] {
			// Reserved STAT18/STAT19 unchanged.
			if p.baseLevels[i] != 99 {
				t.Errorf("disabled stat %d mutated: baseLevels = %d, want 99 (unchanged)", i, p.baseLevels[i])
			}
			continue
		}
		want := uint8(1)
		if i == objtype.PlayerStatHitpoints {
			want = 10
		}
		if p.baseLevels[i] != want {
			t.Errorf("stat %d after minme: baseLevels = %d, want %d", i, p.baseLevels[i], want)
		}
		if p.levels[i] != want {
			t.Errorf("stat %d after minme: levels = %d, want %d", i, p.levels[i], want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(SetStat|AdvanceStat|MinMe)' -v`
Expected: FAIL — admin block doesn't have setstat/advancestat/minme arms yet.

- [ ] **Step 3: Add the three arms**

In `modules/world/handlers_game.go`, inside the admin block's `switch parts[0] {` (added by T2), insert before the `case "reboot":` line (alphabetical-ish order, doesn't matter functionally since cases are exclusive):

```go
		case "setstat":
			// TS L401-414 — setstat <skill> <level> via PlayerStatMap.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 {
				return nil
			}
			stat, ok := objtype.PlayerStatMap[strings.ToUpper(sub[0])]
			if !ok {
				return nil
			}
			level := parseIntOr(sub[1], 0)
			p.SetStat(stat, level)
		case "advancestat":
			// TS L415-431 — zero stats/baseLevels/levels then AddXP to
			// reach `level`. AddXP fires [changestat,X] and [advancestat,X]
			// triggers on level-up.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			stat, ok := objtype.PlayerStatMap[strings.ToUpper(sub[0])]
			if !ok {
				return nil
			}
			levelStr := ""
			if len(sub) > 1 {
				levelStr = sub[1]
			}
			level := parseIntOr(levelStr, 0)
			p.stats[stat] = 0
			p.baseLevels[stat] = 1
			p.levels[stat] = 1
			p.AddXP(stat, objtype.GetExpByLevel(level))
		case "minme":
			// TS L432-440 — set every enabled stat to 1 except HITPOINTS
			// which goes to 10.
			for i := 0; i < objtype.PlayerStatCount; i++ {
				if !objtype.PlayerStatEnabled[i] {
					continue
				}
				if i == objtype.PlayerStatHitpoints {
					p.SetStat(i, 10)
				} else {
					p.SetStat(i, 1)
				}
			}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(SetStat|AdvanceStat|MinMe)' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-184 T4 — port ::setstat / ::advancestat / ::minme

Mirrors TS ClientCheatHandler.ts:401-440. Admin-block (>=3) arms:
- ::setstat <skill> <level> — full-triple write via Player.SetStat
- ::advancestat <skill> <level> — zero then AddXP to reach level;
  fires [changestat,X] and [advancestat,X] triggers via existing AddXP
- ::minme — every enabled stat to 1 except HITPOINTS=10

Routes through objtype.PlayerStatMap (T0) and Player.SetStat (T1).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add `Player.InvAdd` bare-entity method

Mirrors TS `Player.invAdd` (Player.ts:1496-1504): bare entity wrapper that bypasses the script-VM gates which live in the INV_ADD opcode body.

**Files:**
- Create: `modules/world/player_inv_cheat.go` (new file — keeps cheat helpers grouped; mirrors pattern of `player_inv.go` if it exists, else creates a new logical home)
- Test: `modules/world/player_inv_cheat_test.go`

- [ ] **Step 1: Pre-flight: confirm file placement**

Run: `ls modules/world/player_inv*.go 2>/dev/null`

If `player_inv.go` exists and is small enough to host the new method, append to it instead of creating `player_inv_cheat.go`. Use the conventional pattern in the existing codebase.

Run: `grep -nE 'func \(p \*Player\) ' modules/world/player_inv*.go 2>/dev/null | head -10`

Note the file containing existing `*Player` inv methods. Add to that file.

- [ ] **Step 2: Write the failing test**

Add to the appropriate test file (matching the production file location):

```go
func TestPlayerInvAdd_StackableAddsToOneSlot(t *testing.T) {
	s, p := teleTestPlayer(t)
	// Use a stackable obj (e.g. coins, ObjID 995). Verify at runtime via
	// the test fixture's objtype loader; this assumes coins are loaded.
	const coinsID = 995
	const inv = 0 // The "inv" inventory type id — resolve from s.invTypes.Inv
	if s.invTypes != nil {
		// Override if invTypes loaded.
	}

	p.InvAdd(s.invTypes.Inv, coinsID, 100)

	gotInv := s.invLookup.Get(p, s.invTypes.Inv)
	if gotInv == nil {
		t.Fatalf("inv lookup returned nil")
	}
	total := 0
	for i := 0; i < gotInv.Capacity(); i++ {
		obj, count := gotInv.SlotObj(i)
		if obj == coinsID {
			total += count
		}
	}
	if total != 100 {
		t.Errorf("after InvAdd: total coins = %d, want 100", total)
	}
}

func TestPlayerInvAdd_NonStackableOverflowDropsToFloor(t *testing.T) {
	s, p := teleTestPlayer(t)
	// Pre-fill the inv to capacity with one non-stackable obj.
	const swordID = 1277 // bronze sword, non-stackable (verify via objtype if available)
	const fillCount = 28 // standard inv slot count

	for i := 0; i < fillCount; i++ {
		p.InvAdd(s.invTypes.Inv, swordID, 1)
	}

	// Now adding 3 more should overflow → drop on floor at p.x, p.z.
	p.InvAdd(s.invTypes.Inv, swordID, 3)

	// Verify 28 still in inv.
	gotInv := s.invLookup.Get(p, s.invTypes.Inv)
	count := 0
	for i := 0; i < gotInv.Capacity(); i++ {
		obj, _ := gotInv.SlotObj(i)
		if obj == swordID {
			count++
		}
	}
	if count != fillCount {
		t.Errorf("inv slot count = %d, want %d", count, fillCount)
	}

	// Verify ground at player's tile has 3 swords (queryable via zone tracker).
	// Implementation: walk the player's zone for ground objs at (p.x, p.z).
	// Exact API depends on existing zone test helpers; pre-flight grep for
	// `zoneObjsAt` / `ZoneObjs` / similar.
}
```

**Pre-flight grep**: Run `grep -nE 'func.*\bInventory\) (Capacity|SlotObj|Slot\b)' pkg/inventory/inventory.go` to confirm the inventory accessor names match. Adjust the test to use the actual API names.

If `s.invTypes` is nil in `teleTestPlayer` (no real cache loaded), this test path won't work. Pre-flight check: `grep 'invTypes' modules/world/handlers_game_test.go modules/world/test_helpers*.go` — does the existing test fixture load real invTypes? If not, use a smaller-scoped unit test against a hand-rolled `*Inventory` and validate the wrapper's routing instead of full e2e.

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayerInvAdd_' -v`
Expected: FAIL — `InvAdd` undefined on `*Player`.

- [ ] **Step 4: Implement `Player.InvAdd`**

Add to the chosen file (e.g. append to `modules/world/player_inv.go` or create `player_inv_cheat.go`):

```go
// InvAdd mirrors TS Player.invAdd (Player.ts:1496-1504): a bare entity-
// level helper that bypasses the protect/scope/dummyitem gates which
// live in the INV_ADD opcode body. Used by admin cheats (::give /
// ::givemany / ::givecrap / ::giveother) where gates are not desired.
//
// Adds count units of obj to the inventory identified by invTypeID at the
// player's slot. Overflow drops at the player's tile via Server.AddObj
// with duration 200 ticks and ReceiverID = the player's UID (private
// drop). Matches pkg/script.performInvAdd overflow behavior but omits
// its validator chain.
//
// Silent no-op on missing invType, missing objType, or nil inv lookup.
func (p *Player) InvAdd(invTypeID, obj, count int) {
	srv := p.client.server
	if srv == nil || srv.invTypes == nil || srv.objTypes == nil {
		return
	}
	if invTypeID < 0 || invTypeID >= len(srv.invTypes.Configs) {
		return
	}
	invType := srv.invTypes.Configs[invTypeID]
	if invType == nil {
		return
	}
	if obj < 0 || obj >= len(srv.objTypes.Configs) {
		return
	}
	objType := srv.objTypes.Configs[obj]
	if objType == nil {
		return
	}

	inv := srv.invLookup.Get(p, invTypeID)
	if inv == nil {
		return
	}

	stackable := objType.Stackable
	stockObj := false
	for _, id := range invType.StockObj {
		if int(id) == obj {
			stockObj = true
			break
		}
	}

	tx := inv.Add(obj, count, inventory.AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: false,
		Stackable:           stackable,
		StockObj:            stockObj,
	})

	overflow := count - tx.Completed
	if overflow > 0 {
		level := (p.CoordPacked() >> 28) & 0x3
		x := p.X()
		z := p.Z()
		receiverID := p.UID()
		if !stackable || overflow == 1 {
			for range overflow {
				dropObj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, obj, 1)
				dropObj.ReceiverID = receiverID
				dropObj.Reveal = entitypkg.ObjReveal
				srv.AddObj(dropObj, receiverID, 200)
			}
		} else {
			dropObj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, obj, overflow)
			dropObj.ReceiverID = receiverID
			dropObj.Reveal = entitypkg.ObjReveal
			srv.AddObj(dropObj, receiverID, 200)
		}
	}
}
```

**Imports** to ensure are present at top of the file: `"github.com/zsrv/goscape/pkg/inventory"`, `entitypkg "github.com/zsrv/goscape/pkg/entity"` (match the existing alias in modules/world — pre-flight grep `grep -n 'entitypkg' modules/world/player.go` to confirm).

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayerInvAdd_' -v`
Expected: all PASS.

- [ ] **Step 6: Verify byte-parity with performInvAdd for the simple in-inv case**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS, including all existing inv-related tests.

- [ ] **Step 7: Commit**

```bash
git add modules/world/player_inv*.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-184 T5 — Player.InvAdd bare-entity helper

Mirrors TS Player.invAdd (Player.ts:1496-1504): bypasses the protect/
scope/dummyitem gates that live in the INV_ADD opcode body. Bypassing
is correct for admin cheats (::give / ::givemany) where gates aren't
desired. Routes through the same inventory.Add + overflow→Server.AddObj
path as pkg/script.performInvAdd, without the validator chain.

Consumed by NAI-184 T6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Port admin-block inventory cheats (give / givemany)

Depends on T5.

**Files:**
- Modify: `modules/world/handlers_game.go` (admin block)
- Test: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Pre-flight grep**

Run: `grep -n 'ByName' pkg/objtype/objtype.go`
Expected output includes `func (otc *ObjTypeConfigs) ByName(name string) *ObjType`.

Confirm the returned `*ObjType` exposes an `Id` (or `ID`) field. If unsure, run `grep -nE 'Id |ID ' pkg/objtype/objtype.go | head -5`.

- [ ] **Step 2: Write the failing tests**

Add to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_Give_AddsToInv(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 3
	// Use any known obj name from the loaded objtype config. Pre-flight
	// grep: rg -n '"coins"' pkg/objtype/objtype_test.go to find a name
	// guaranteed to load.
	const objName = "coins" // canonical stackable test obj
	const wantCount = 5

	dispatchTeleCheat(t, p, "give "+objName+" "+itoa(wantCount))

	objType := s.objTypes.ByName(objName)
	if objType == nil {
		t.Fatalf("test setup: obj %q not loaded in objtype configs", objName)
	}
	gotInv := s.invLookup.Get(p, s.invTypes.Inv)
	total := 0
	for i := 0; i < gotInv.Capacity(); i++ {
		obj, count := gotInv.SlotObj(i)
		if obj == objType.Id {
			total += count
		}
	}
	if total != wantCount {
		t.Errorf("after give: total = %d, want %d", total, wantCount)
	}

	// Unknown obj name: no mutation.
	pre := total
	dispatchTeleCheat(t, p, "give fake_obj 5")
	total = 0
	for i := 0; i < gotInv.Capacity(); i++ {
		obj, count := gotInv.SlotObj(i)
		if obj == objType.Id {
			total += count
		}
	}
	if total != pre {
		t.Errorf("unknown obj mutated inv: %d → %d", pre, total)
	}

	// Missing count: defaults to 1.
	preTotal := total
	dispatchTeleCheat(t, p, "give "+objName)
	total = 0
	for i := 0; i < gotInv.Capacity(); i++ {
		obj, count := gotInv.SlotObj(i)
		if obj == objType.Id {
			total += count
		}
	}
	if total != preTotal+1 {
		t.Errorf("give without count: total = %d, want %d (pre+1)", total, preTotal+1)
	}
}

func TestHandleClientCheat_GiveMany_Adds1000(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 3
	const objName = "coins"

	dispatchTeleCheat(t, p, "givemany "+objName)

	objType := s.objTypes.ByName(objName)
	if objType == nil {
		t.Fatalf("test setup: obj %q not loaded", objName)
	}
	gotInv := s.invLookup.Get(p, s.invTypes.Inv)
	total := 0
	for i := 0; i < gotInv.Capacity(); i++ {
		obj, count := gotInv.SlotObj(i)
		if obj == objType.Id {
			total += count
		}
	}
	if total != 1000 {
		t.Errorf("after givemany: total = %d, want 1000", total)
	}
}

// itoa is a tiny helper for assembling cheat strings.
func itoa(n int) string { return strconv.Itoa(n) }
```

If `s.invTypes` / `s.objTypes` are not real-cache-loaded in `teleTestPlayer`, switch to a fixture that DOES load them — pre-flight grep `grep -n 'LoadObjTypes\|LoadInvTypes' modules/world/*test*.go` to find the canonical real-cache test helper.

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(Give|GiveMany)' -v`
Expected: FAIL — give/givemany arms don't exist.

- [ ] **Step 4: Add give / givemany to admin block**

In `modules/world/handlers_game.go`, inside the admin block's switch (added by T2, populated by T4), add:

```go
		case "give":
			// TS L288-302 — give <obj> [count]. Count clamps to [1, MAXINT].
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			objType := p.client.server.objTypes.ByName(sub[0])
			if objType == nil {
				return nil
			}
			count := 1
			if len(sub) > 1 {
				count = parseIntOr(sub[1], 1)
				if count < 1 {
					count = 1
				}
				if count > 0x7fffffff {
					count = 0x7fffffff
				}
			}
			p.InvAdd(p.client.server.invTypes.Inv, objType.Id, count)
		case "givemany":
			// TS L339-352 — givemany <obj>. Fixed 1000 count.
			if args == "" {
				return nil
			}
			objType := p.client.server.objTypes.ByName(strings.SplitN(args, " ", 2)[0])
			if objType == nil {
				return nil
			}
			p.InvAdd(p.client.server.invTypes.Inv, objType.Id, 1000)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(Give|GiveMany)' -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-184 T6 — port ::give / ::givemany admin cheats

Mirrors TS ClientCheatHandler.ts:288-302 (give) and :339-352 (givemany).
Both route through Player.InvAdd (T5). Give accepts [count] with [1,
0x7fffffff] clamp; givemany fixes count at 1000.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Port `::snapshot` admin cheat

**Files:**
- Modify: `modules/world/handlers_game.go` (admin block + imports)
- Test: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing test**

Add to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_Snapshot_WritesHeapFile(t *testing.T) {
	_, p := teleTestPlayer(t)
	p.staffModLevel = 3

	preFiles, _ := filepath.Glob(filepath.Join(os.TempDir(), "heap-*.pprof"))

	dispatchTeleCheat(t, p, "snapshot")

	postFiles, _ := filepath.Glob(filepath.Join(os.TempDir(), "heap-*.pprof"))
	if len(postFiles) <= len(preFiles) {
		t.Errorf("snapshot did not write a heap-*.pprof file (pre=%d, post=%d)", len(preFiles), len(postFiles))
	}

	// Cleanup newly-created files.
	for _, f := range postFiles {
		found := false
		for _, pf := range preFiles {
			if pf == f {
				found = true
				break
			}
		}
		if !found {
			os.Remove(f)
		}
	}
}
```

Required imports at the top of the test file (add if missing): `"os"`, `"path/filepath"`.

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Snapshot_' -v`
Expected: FAIL — no snapshot case.

- [ ] **Step 3: Add snapshot to admin block**

In `modules/world/handlers_game.go`, inside the admin block's switch, add:

```go
		case "snapshot":
			// TS L477-480 — writes a heap snapshot. TS uses v8's JSON-ish
			// format; goscape uses runtime/pprof.WriteHeapProfile (Go's
			// native heap-profile format). Functional analog — TS-fidelity
			// here is dispatch behavior, not output bytes.
			path := filepath.Join(os.TempDir(), fmt.Sprintf("heap-%d.pprof", time.Now().UnixNano()))
			if f, err := os.Create(path); err == nil {
				if err := pprof.WriteHeapProfile(f); err == nil && p.client.server.log != nil {
					p.client.server.log.Info("heap snapshot written", "path", path)
				}
				f.Close()
			}
```

**Imports** to add to `handlers_game.go` if not already present: `"os"`, `"path/filepath"`, `"runtime/pprof"`, `"time"`, `"fmt"`. Pre-flight grep `head -30 modules/world/handlers_game.go` to see current imports and add only the missing ones.

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Snapshot_' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-184 T7 — port ::snapshot admin cheat

Mirrors TS ClientCheatHandler.ts:477-480 (v8.writeHeapSnapshot). Goscape
uses runtime/pprof.WriteHeapProfile instead — different format but
functional analog (admin debug helper). Writes to $TMPDIR/heap-<nanos>.pprof
and logs the path via s.log.Info (mirrors TS printDebug).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Port cross-player tele cheats (teleother / teleto)

**Files:**
- Modify: `modules/world/handlers_game.go` (admin block + super-mod block)
- Test: `modules/world/handlers_game_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/handlers_game_test.go`:

```go
func TestHandleClientCheat_TeleOther_MovesTargetToSource(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 3
	p.x, p.z, p.level = 3200, 3200, 0
	s.cfg.NodeProduction = true

	// Build a second player at a different coord.
	other := addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleother other_user")

	if other.x != 3200 || other.z != 3200 || other.level != 0 {
		t.Errorf("after teleother: other at (%d, %d, %d), want (3200, 3200, 0)", other.x, other.z, other.level)
	}
}

func TestHandleClientCheat_TeleOther_NoOpWhenNotProduction(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = false
	other := addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleother other_user")

	if other.x != 3220 || other.z != 3220 {
		t.Errorf("teleother under NP=false moved other: at (%d, %d), want (3220, 3220)", other.x, other.z)
	}
}

func TestHandleClientCheat_TeleOther_UnknownUserMessagesCaller(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = true
	drainConn(t, s, p)

	dispatchTeleCheat(t, p, "teleother no_such_user")

	msgs := drainMessageGame(t, s, p) // helper from T3
	if !containsMessage(msgs, "no_such_user is not logged in.") {
		t.Errorf("expected 'no_such_user is not logged in.' in %v", msgs)
	}
}

func TestHandleClientCheat_TeleOther_AdminGate(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 2 // below admin tier
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleother other_user")

	if other.x != 3220 {
		t.Errorf("teleother at staffModLevel=2 moved other: at x=%d, want 3220", other.x)
	}
}

func TestHandleClientCheat_TeleTo_MovesSourceToTarget(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 2 // super-mod
	p.x, p.z, p.level = 3200, 3200, 0
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)
	_ = other

	dispatchTeleCheat(t, p, "teleto other_user")

	if p.x != 3220 || p.z != 3220 {
		t.Errorf("after teleto: caller at (%d, %d), want (3220, 3220)", p.x, p.z)
	}
}

func TestHandleClientCheat_TeleTo_NoOpWhenNotProduction(t *testing.T) {
	s, p := teleTestPlayer(t)
	p.staffModLevel = 2
	p.x, p.z = 3200, 3200
	s.cfg.NodeProduction = false
	addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleto other_user")

	if p.x != 3200 || p.z != 3200 {
		t.Errorf("teleto under NP=false moved caller: at (%d, %d), want (3200, 3200)", p.x, p.z)
	}
}
```

**Pre-flight**: `addOtherTestPlayer` is NOT an existing helper. Two routes:

1. **Add helper** at the bottom of `handlers_game_test.go`:

```go
// addOtherTestPlayer builds a minimal *Player with the given username
// and coord, registers it in the server, and returns it. Used by
// teleother / teleto tests.
func addOtherTestPlayer(t *testing.T, s *Server, username string, x, z, level int) *Player {
	t.Helper()
	other := &Player{username: username, x: x, z: z, level: level}
	// ... register via s.addPlayer or similar (pre-flight grep
	//     for the canonical test helper that creates a second player).
	// ... ensure LookupPlayerByUsername(username) returns it.
	return other
}
```

2. **Reuse existing helper** if one exists. Pre-flight grep: `grep -nE 'func.*[Tt]est[A-Z].*Player.*Server\)|second.*Player|otherPlayer' modules/world/*test*.go | head -10`. If an existing helper provides the second-player fixture, use it.

- [ ] **Step 2: Run tests to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(TeleOther|TeleTo)' -v`
Expected: FAIL.

- [ ] **Step 3: Add `teleother` to admin block**

In `modules/world/handlers_game.go`, inside the admin block's switch:

```go
		case "teleother":
			// TS L377-400 — teleother <username> (production-only).
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			other := p.client.server.LookupPlayerByUsername(args)
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", args))
				return nil
			}
			other.CloseModal(true)
			if !other.CanAccess() {
				p.MessageGame(fmt.Sprintf("%s is busy right now.", args))
				return nil
			}
			other.ClearInteraction()
			sendUnsetMapFlag(other)
			other.waypointIndex = -1
			other.TeleJump(p.x, p.z, p.level)
```

- [ ] **Step 4: Add `teleto` to super-mod block**

In `modules/world/handlers_game.go`, inside the super-mod block's switch (currently containing `getcoord` and `tele`), append:

```go
		case "teleto":
			// TS L525-548 — teleto <username> (production-only).
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			other := p.client.server.LookupPlayerByUsername(args)
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", args))
				return nil
			}
			p.CloseModal(true)
			if !p.CanAccess() {
				p.MessageGame("Please finish what you are doing first.")
				return nil
			}
			p.ClearInteraction()
			sendUnsetMapFlag(p)
			p.waypointIndex = -1
			p.TeleJump(other.x, other.z, other.level)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_(TeleOther|TeleTo)' -v`
Expected: all PASS.

- [ ] **Step 6: Full test pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-184 T8 — port ::teleother / ::teleto cross-player tele

Mirrors TS ClientCheatHandler.ts:377-400 (teleother, admin >=3) and
:525-548 (teleto, super-mod >=2). Both NodeProduction-gated via inner
case-body break. Use existing Server.LookupPlayerByUsername +
Player.TeleJump infra.

teleother moves the target to the caller's coord; teleto moves the
caller to the target's coord. Both apply the standard pre-tele cleanup
(CloseModal + CanAccess gate + ClearInteraction + unsetMapFlag).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Close commit — finalize deviations + memory trailer

**Files:**
- No code changes (verify only).

- [ ] **Step 1: Final deviation grep**

Run: `rg "DEVIATION-NAI-182-D3-OTHER-CHEATS" modules/ pkg/ cmd/`
Expected: ZERO hits. The comment was rewritten in T2 to `DEVIATION-NAI-184-D2-D3-CARRYFORWARD`.

Run: `rg "DEVIATION-NAI-184-D2-D3-CARRYFORWARD" modules/ pkg/ cmd/`
Expected: exactly 1 hit (the rewritten comment in handlers_game.go).

Run: `rg "DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD" modules/ pkg/ cmd/`
Expected: at least 1 hit (the SetStat doc-comment in player_script.go).

Run: `rg "TS-faithful dead code" modules/world/handlers_game.go`
Expected: ZERO hits. The NAI-183 dead-code preservation comments were retired in T2.

- [ ] **Step 2: Full test pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race`
Expected: PASS.

- [ ] **Step 3: Build**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o /tmp/goscape ./cmd/goscape`
Expected: PASS, no warnings.

- [ ] **Step 4: Close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-184 — D3 cheat cohort port (11 cheats) + NAI-183 reboot-trio relocation

Closes NAI-184. 11 cheats ported from DEVIATION-NAI-182-D3-OTHER-CHEATS:
  Dev block (!NP && >=4):  fly, naive, random
  Admin block (>=3):       setstat, advancestat, minme, give, givemany,
                           snapshot, teleother (&& NP)
  Super-mod block (>=2):   teleto (&& NP)

Structural fix: NAI-183 had placed reboot/slowreboot/serverdrop in the
dev !NP >=4 block based on a TS-source misread (the trio belongs in the
admin >=3 block). NAI-184 T2 relocates them and retires the
"TS-faithful dead code" preservation comments.

Carry-forward: DEVIATION-NAI-184-D2-D3-CARRYFORWARD enumerates the 17
remaining unported cheats (reload, rebuild, speed, setvar, setvarother,
getvar, getvarother, giveother, givecrap, broadcast, locadd, npcadd,
openmain, setvis, ban, mute, kick). Each is blocked on a missing
subsystem (VarPlayerType.GetByName, World.broadcastMes, runtime
tick-rate mutation, login moderation callbacks, dynamic spawn,
Visibility plumbing).

New deviations:
- DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD — combat-level recompute
  + appearance rebuild deferred to future combat sub-spec.

Closes memory:
- audit_full_method_when_restructuring.md (NAI-183 misclassification was
  exactly this failure mode — re-derived TS hierarchy line-by-line)
- audit_arithmetic_correction_in_rollup.md (re-counted the D3 enumeration
  in the spec body before commit)
- consume_reserved_constant.md (PlayerStat18/19 stay reserved; not
  enumerated by PlayerStatEnabled)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist (controller — before dispatching T0)

Per memory `controller_preflight.md`:

- [ ] `rg 'DEVIATION-NAI-182-D3' modules/ pkg/ cmd/` baseline count recorded.
- [ ] `Server.LookupPlayerByUsername` signature confirmed at `modules/world/server.go:891`.
- [ ] `inventory.Inventory.Add` signature + `inventory.AddOpts` fields confirmed at `pkg/inventory/inventory.go:180`.
- [ ] `srv.invTypes.Configs` / `srv.objTypes.Configs` slice access confirmed in `modules/world/server.go:88-89`.
- [ ] `entitypkg.NewObj` constructor signature confirmed (used by `worldVarsView.AddObj` at `modules/world/server_varp.go:191`).
- [ ] `Player.UID()`, `Player.X()`, `Player.Z()`, `Player.CoordPacked()` confirmed accessible.
- [ ] `Player.MessageGame`, `CloseModal`, `CanAccess`, `ClearInteraction`, `TeleJump` confirmed (used by existing `::tele`).
- [ ] `sendUnsetMapFlag(p)` + `p.waypointIndex = -1` pattern confirmed at handlers_game.go:447-448.
- [ ] `parseIntOr(s, default)` helper confirmed in handlers_game.go.
- [ ] Test fixture status: `teleTestPlayer` loads real objtype/invtype configs OR a stub. T5/T6 plans assume real-cache; if test fixture is stub-only, restructure T5/T6 tests to mock `*ObjType` instead.
