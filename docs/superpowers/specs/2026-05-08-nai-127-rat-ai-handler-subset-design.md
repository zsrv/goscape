# NAI-127 — Rat-AI handler subset (FINDHERO family + DAMAGE / GENDER / P_PREVENTLOGOUT)

**Status:** spec — draft 1
**Date:** 2026-05-08
**Predecessor:** NAI-126 close (`e3bd6cc`); cascade-bound to the Tutorial-Island giant-rat smoke surfaced at NAI-126 close (2026-05-08).
**Cadence:** subagent-driven-development — two bundles dispatched as separate task sequences with controller pre-flight + Sonnet code-reviewer between; user-launched smoke binds Bundle 2 PRIMARY only.
**Tech stack:** Go 1.26+.

## §0 — One-line summary

Close the rat-AI cascade-tail by porting six unhandled script opcodes — NPC_FINDHERO (2519), FINDHERO (2018), BOTH_HEROPOINTS (2003), DAMAGE (2015), GENDER (2020), P_PREVENTLOGOUT (2084) — and retiring the long-parked NAI-40-SB2 carry-forward. Bundle 1 (FINDHERO family, 3 opcodes) is plumbing-heavy; Bundle 2 (rat-attacks-player primitives, 3 opcodes) is the smoke-binding closer.

## §1 — Symptom and binding evidence

**Smoke (NAI-126 close, `e3bd6cc`, 2026-05-08):**
Fresh char + bronze dagger vs Tutorial Island giant rat. Rat dies (NPC_DEL works, NAI-126 PRIMARY met). Server log emits:

```
WARN: script "[ai_queue3,newbiegiantrat]": no handler for NPC_FINDHERO (opcode 2519) at pc=2
```

`[ai_queue3,newbiegiantrat]` aborts; the chained `obj_add(npc_coord, npc_param(death_drop), 1, ^lootdrop_duration)` and `obj_add(npc_coord, raw_rat_meat, 1, ^lootdrop_duration)` never run, no loot drops.

**Audit at HEAD (`e3bd6cc`)** per `missing_handler_audit`:

```
$ awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/handled.txt
$ awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/declared.txt
$ comm -23 /tmp/declared.txt /tmp/handled.txt | wc -l
62
```

**Static-trace closure** of `[ai_queue1/3,newbiegiantrat]` + `[ai_opplayer2,newbiegiantrat]` + `[opnpc2/apnpc2,newbiegiantrat]` + every transitively-gosub'd proc/queue/label (`npc_combat.rs2`, `npc_death.rs2`, `auto_retaliate.rs2`, `player_combat.rs2`, `damage.rs2`, `combat.rs2`, `sound.rs2`, `logout.rs2`, `ring_of_life.rs2`, `ring_of_recoil.rs2`) intersected with the 62 unhandled opcodes:

| Opcode | # | RS2 keyword | Site | TS file:line |
|---|---|---|---|---|
| NPC_FINDHERO | 2519 | `npc_findhero` | `[ai_queue3,newbiegiantrat]`; `[proc,npc_default_death]` | NpcOps.ts:114-130 |
| DAMAGE | 2015 | `damage(uid, hitmark, amt)` | `[proc,damage_self]` (via `[queue,combat_damage_player]`) | PlayerOps.ts:768-779 |
| GENDER | 2020 | `gender()` | `[proc,human_hit_sound]` (via `~damage_self`) | PlayerOps.ts:968-970 |
| P_PREVENTLOGOUT | 2084 | `p_preventlogout(msg, ticks)` | `[proc,combat_preventlogout]` (via `[queue,playerhit_n_retaliate]`) | PlayerOps.ts:626-630 |

**Stretch to FINDHERO family (per scope decision 2026-05-08)** — NPC_FINDHERO shares HeroPoints/pointer-flip plumbing with two further player-side opcodes already in NAI-40-SB2 carry-forward (8 grep hits in `nai_followups.md`):

| Opcode | # | RS2 keyword | TS file:line |
|---|---|---|---|
| FINDHERO | 2018 | `findhero` | PlayerOps.ts:1138-1154 |
| BOTH_HEROPOINTS | 2003 | `both_heropoints(damage)` | PlayerOps.ts:1156-1167 |

**Total: 6 opcodes.** Post-close audit expectation: `62 - 6 = 56` declared-but-unhandled opcodes.

## §2 — Adjacency at HEAD

All six opcode constants are declared at `pkg/script/opcode.go`:

```
OpBothHeroPoints      Opcode = 2003   (line 103)
OpDamage              Opcode = 2015   (line 115)
OpFindHero            Opcode = 2018   (line 118)
OpGender              Opcode = 2020   (line 120)
OpPPreventLogout      Opcode = 2084   (line 184)
OpNpcFindHero         Opcode = 2519   (line 256)
```

None has a dispatch entry in `pkg/script/handlers.go`.

**Existing infra (NO porting required):**
- `modules/world/heropoints.go` — `HeroPoints` struct + `AddHero(uid, amount)` + `TopContributor() int` (NAI-120 Bundle 2D).
- `modules/world/npc.go:149,215` — `Npc.heroPoints HeroPoints` field initialized at `NewHeroPoints(16)`.
- `pkg/script/active.go:820-824` — `ActiveNpc.AddHeroPoints(playerUID, amount int)` interface method (NAI-120 Bundle 2D).
- `pkg/script/handlers_npc.go:1061-1090` — `handleNpcHeroPoints` (NPC_HEROPOINTS, opcode 2524).
- `modules/world/server.go:782-791` — `Server.LookupPlayerByUID(uid int) script.ActivePlayer`.
- `modules/world/server.go:85,238` — `worldVarsView` adapter pattern.
- `pkg/script/state.go:50-109` — `WorldVars` interface + `RemoveObj/AddObj/RemoveNpc` adapter precedent.
- `pkg/script/pointer.go:8-15` — `PtrActivePlayer` (`1<<0`) + `PtrActivePlayer2` (`1<<1`) flags.
- `pkg/script/state.go:236-241` — `Self` (primary) and `Self2` (secondary) `ActivePlayer` slots.
- `pkg/script/handlers_player.go:42-51` — `requireActivePlayer2` validator helper.
- `pkg/script/active.go:579-582` — `ActivePlayer.Gender() int` getter (NAI series).
- `modules/world/player_masks.go:126-139` — `Player.Damage(amount, dmgType int)` impl (S6e/post).
- `modules/world/player.go:172` — `Player.gender int` field.
- `modules/world/player.go:258-260` — `Player.preventLogoutMessage`/`preventLogoutUntil` fields.
- `pkg/script/handlers_dialog.go:10` etc. — `requireProtectedActivePlayer` helper for P_PREVENTLOGOUT gate.

**Goscape gap (porting required):**
- `Player.heroPoints HeroPoints` — does NOT exist. TS Player has `player.heroPoints` distinct from `npc.heroPoints` for the player→player BOTH_HEROPOINTS path. (PlayerOps.ts:514, 553, 610 do `player.heroPoints.clear()` on stat reset; out-of-scope here — clear hooks deferred.)
- `ActivePlayer.AddHeroPoints / TopContributor` — do NOT exist. Parallel to `ActiveNpc.AddHeroPoints / TopContributor`.
- `ActivePlayer.SetPreventLogout / ApplyDamage` — do NOT exist as interface methods (concrete fields/impls do).
- `ActiveNpc.TopContributor` — does NOT exist as an interface method (`HeroPoints.TopContributor` impl exists at `heropoints.go:60`).
- `WorldVars.LookupPlayerByUID` — does NOT exist as an interface method (`Server.LookupPlayerByUID` exists; not exposed via `worldVarsView`).

## §3 — Bundle 1: FINDHERO family

**Opcodes:** NPC_FINDHERO (2519), FINDHERO (2018), BOTH_HEROPOINTS (2003).

### §3.1 — TS reference (verbatim)

`Engine-TS/src/engine/script/handlers/NpcOps.ts:114-130`:

```typescript
[ScriptOpcode.NPC_FINDHERO]: checkedHandler(ActiveNpc, state => {
    const hash64 = state.activeNpc.heroPoints.findHero();
    if (hash64 === -1n) {
        state.pushInt(0);
        return;
    }

    const player = World.getPlayerByHash64(hash64);
    if (!player) {
        state.pushInt(0);
        return;
    }

    state.activePlayer = player;
    state.pointerAdd(ActivePlayer[state.intOperand]);
    state.pushInt(1);
}),
```

Setter `state.activePlayer` checks `intOperand` to route to `_activePlayer` or `_activePlayer2` per `ScriptState.ts:235-241` (verified in §2 brainstorm).

`Engine-TS/src/engine/script/handlers/PlayerOps.ts:1138-1154`:

```typescript
[ScriptOpcode.FINDHERO]: checkedHandler(ActivePlayer, state => {
    const hash64 = state.activePlayer.heroPoints.findHero();
    if (hash64 === -1n) {
        state.pushInt(0);
        return;
    }

    const player = World.getPlayerByHash64(hash64);
    if (!player) {
        state.pushInt(0);
        return;
    }
    state._activePlayer2 = player;
    state.pointerAdd(ScriptPointer.ActivePlayer2);
    state.pushInt(1);
}),
```

FINDHERO is asymmetric vs NPC_FINDHERO — always sets `_activePlayer2` regardless of intOperand. Direct field write, not setter.

`Engine-TS/src/engine/script/handlers/PlayerOps.ts:1156-1167`:

```typescript
[ScriptOpcode.BOTH_HEROPOINTS]: checkedHandler(ActivePlayer, state => {
    const damage: number = check(state.popInt(), NumberNotNull);
    const secondary: boolean = state.intOperand === 1;

    const fromPlayer: Player | null = secondary ? state._activePlayer2 : state._activePlayer;
    const toPlayer: Player | null = secondary ? state._activePlayer : state._activePlayer2;

    if (!fromPlayer || !toPlayer) {
        throw new Error('player is null');
    }

    toPlayer.heroPoints.addHero(fromPlayer.hash64, damage);
}),
```

### §3.2 — Surface

| Layer | File | Change |
|---|---|---|
| Interface | `pkg/script/active.go` | Add `TopContributor() int` to `ActiveNpc`. Add `AddHeroPoints(playerUID, amount int)`, `TopContributor() int` to `ActivePlayer`. |
| Interface | `pkg/script/state.go` | Add `LookupPlayerByUID(uid int) ActivePlayer` to `WorldVars`. |
| Handler | `pkg/script/handlers_npc.go` | New `handleNpcFindHero` (alphabetic between `handleNpcFindAllZone` and `handleNpcHasOp` — exact placement re-verified at plan-write per `controller_preflight`). |
| Handler | `pkg/script/handlers_player.go` | New `handleFindHero` and `handleBothHeroPoints` (alphabetic placement). |
| Dispatch | `pkg/script/handlers.go` | Three new entries: `OpNpcFindHero`, `OpFindHero`, `OpBothHeroPoints`. |
| Impl | `modules/world/npc_script.go` | New `(n *Npc) TopContributor() int { return n.heroPoints.TopContributor() }`. |
| Impl | `modules/world/player.go` | New field `heroPoints HeroPoints` initialized `NewHeroPoints(16)` at `newPlayer()` (modules/world/player.go:426; mirrors `Npc.heroPoints` init at `npc.go:215`). |
| Impl | `modules/world/player_script.go` (or wherever ActivePlayer methods live) | New `(p *Player) AddHeroPoints(uid, amount int) { p.heroPoints.AddHero(uid, amount) }`. New `(p *Player) TopContributor() int { return p.heroPoints.TopContributor() }`. |
| Adapter | `modules/world/server.go` | New `(w worldVarsView) LookupPlayerByUID(uid int) script.ActivePlayer { return w.s.LookupPlayerByUID(uid) }`. |
| Test fixture | `pkg/script/handlers_npc_test.go` | Extend `mockNpc`/`mockActiveNpc` with `topContributor int` field + getter. |
| Test fixture | `pkg/script/handlers_player_test.go` | Extend `mockPlayer` with `topContributor int`, `addHeroPointsCalls []mockAddHero`. Verify `mockPlayer.UID()` exists; add if absent. |
| Test fixture | `pkg/script/handlers_vars_test.go` | Extend `mockWorld` with `players map[int]ActivePlayer` + `LookupPlayerByUID(uid int) ActivePlayer { return m.players[uid] }`. |

### §3.3 — Handler bodies

```go
// pkg/script/handlers_npc.go

// handleNpcFindHero (NPC_FINDHERO, opcode 2519) returns the player who
// has accumulated the largest HeroPoints credit on this NPC's ledger
// (typically the highest-damage attacker), and binds them to the
// primary or secondary active-player slot per IntOperand. Pushes 1 on
// success, 0 if the ledger is empty or the resolved player has logged
// out. Mirrors TS NpcOps.ts:114-130 (state.activePlayer setter
// behavior at ScriptState.ts:235-241 — IntOperand routes to Self
// (primary) or Self2 (secondary)).
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard (goscape defensive;
// TS skips this check). Mirrors handleNpcDel from NAI-126. Retire when
// an upstream invariant proves s.World non-nil for any executing
// script.
func handleNpcFindHero(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_FINDHERO"); err != nil {
        return err
    }
    if s.World == nil {
        s.PushInt(0)
        return nil
    }
    uid := s.ActiveNpc.TopContributor()
    if uid == 0 {
        s.PushInt(0)
        return nil
    }
    player := s.World.LookupPlayerByUID(uid)
    if player == nil {
        s.PushInt(0)
        return nil
    }
    if s.IntOperand == 0 {
        s.Self = player
        s.Pointers |= PtrActivePlayer
    } else {
        s.Self2 = player
        s.Pointers |= PtrActivePlayer2
    }
    s.PushInt(1)
    return nil
}
```

```go
// pkg/script/handlers_player.go

// handleFindHero (FINDHERO, opcode 2018) returns the player who has
// accumulated the largest HeroPoints credit on the active player's
// ledger, binding them to the SECONDARY active-player slot regardless
// of IntOperand. Mirrors TS PlayerOps.ts:1138-1154.
func handleFindHero(s *ScriptState) error {
    if err := requireActivePlayer(s, "FINDHERO"); err != nil {
        return err
    }
    if s.World == nil {
        s.PushInt(0)
        return nil
    }
    uid := s.Self.TopContributor()
    if uid == 0 {
        s.PushInt(0)
        return nil
    }
    player := s.World.LookupPlayerByUID(uid)
    if player == nil {
        s.PushInt(0)
        return nil
    }
    s.Self2 = player
    s.Pointers |= PtrActivePlayer2
    s.PushInt(1)
    return nil
}

// handleBothHeroPoints (BOTH_HEROPOINTS, opcode 2003) credits `damage`
// to the receiving player's HeroPoints ledger, attributed to the
// sending player's UID. IntOperand selects the swap direction:
//
//   IntOperand=0 → from=Self (primary), to=Self2 (secondary)
//   IntOperand=1 → from=Self2 (secondary), to=Self (primary)
//
// Mirrors TS PlayerOps.ts:1156-1167. Returns an error if either slot
// is nil (TS throws).
func handleBothHeroPoints(s *ScriptState) error {
    if err := requireActivePlayer(s, "BOTH_HEROPOINTS"); err != nil {
        return err
    }
    damage := s.PopInt()
    secondary := s.IntOperand == 1
    var from, to ActivePlayer
    if secondary {
        from, to = s.Self2, s.Self
    } else {
        from, to = s.Self, s.Self2
    }
    if from == nil || to == nil {
        return fmt.Errorf("BOTH_HEROPOINTS: player is null")
    }
    to.AddHeroPoints(from.UID(), damage)
    return nil
}
```

### §3.4 — Test cases (RED → GREEN; 13 tests)

Per `superpowers:test-driven-development` and `scriptstate_test_fixture_idioms` — explicit `StackCapacity` init, push-order matched to pop-order, `Pointers` flags set per case.

**NPC_FINDHERO (5):**
1. `TestNpcFindHero_EmptyLedger` — `topContributor=0` → push 0, no pointer flip, no Self/Self2 mutation.
2. `TestNpcFindHero_PrimarySlot` — `topContributor=42`, mockWorld players[42]=p, IntOperand=0 → push 1, `Self==p`, `Pointers&PtrActivePlayer != 0`.
3. `TestNpcFindHero_SecondarySlot` — `topContributor=42`, IntOperand=1 → push 1, `Self2==p`, `Pointers&PtrActivePlayer2 != 0`.
4. `TestNpcFindHero_LookupReturnsNil` — `topContributor=99`, players map empty → push 0.
5. `TestNpcFindHero_RequiresActiveNpc` — Pointers=0 → returns error.

**FINDHERO (4):**
6. `TestFindHero_EmptyLedger` — Self.TopContributor=0 → push 0.
7. `TestFindHero_PopulatedLedger` — push 1, Self2 set, PtrActivePlayer2 set, regardless of IntOperand=0 (TS asymmetry pinned).
8. `TestFindHero_LookupReturnsNil` — uid resolves nil → push 0.
9. `TestFindHero_RequiresActivePlayer` — Pointers=0 → error.

**BOTH_HEROPOINTS (4):**
10. `TestBothHeroPoints_PrimaryToSecondary` — IntOperand=0, both slots set → `Self2.addHeroPointsCalls = [{from=Self.UID(), amount=damage}]`.
11. `TestBothHeroPoints_SecondaryToPrimary` — IntOperand=1, both slots set → `Self.addHeroPointsCalls = [{from=Self2.UID(), amount=damage}]`.
12. `TestBothHeroPoints_NilSlot` — IntOperand=0, Self2=nil → returns error.
13. `TestBothHeroPoints_AmountZero` — pin that handler runs end-to-end and AddHeroPoints is called with 0; ledger no-ops downstream per existing semantics.

### §3.5 — Bundle 1 task plan (5 implementer tasks, then reviewer)

- T1.1 (Haiku) — `ActiveNpc.TopContributor` interface + `*Npc.TopContributor` impl + `mockNpc`/`mockActiveNpc` stubs.
- T1.2 (Haiku) — `ActivePlayer.AddHeroPoints` + `ActivePlayer.TopContributor` interface methods + `Player.heroPoints` field + `*Player` impls + `mockPlayer` stubs (recording fields).
- T1.3 (Haiku) — `WorldVars.LookupPlayerByUID` interface + `worldVarsView.LookupPlayerByUID` adapter + `mockWorld.players` + `LookupPlayerByUID` impl.
- T1.4 (Haiku) — RED: 13 tests (NPC_FINDHERO 5 / FINDHERO 4 / BOTH_HEROPOINTS 4).
- T1.5 (Haiku) — GREEN: 3 handlers + 3 dispatch entries (alphabetic).
- Reviewer (Sonnet per `superpowers_code_reviewer_model`): TS fidelity line-by-line; deviation tagging; no YAGNI; mock satisfaction across all three implementers (`*Npc`, `mockNpc`, `mockActiveNpc`; `*Player`, `mockPlayer`).

## §4 — Bundle 2: Rat-attacks-player primitives

**Opcodes:** DAMAGE (2015), GENDER (2020), P_PREVENTLOGOUT (2084).

### §4.1 — TS reference (verbatim)

`Engine-TS/src/engine/script/handlers/PlayerOps.ts:768-779`:

```typescript
[ScriptOpcode.DAMAGE]: state => {
    const amount = check(state.popInt(), NumberNotNull);
    const type = check(state.popInt(), HitTypeValid);
    const uid = check(state.popInt(), NumberNotNull);

    const player = World.getPlayerByUid(uid);
    if (!player) {
        return;
    }

    player.applyDamage(amount, type);
},
```

`Engine-TS/src/engine/script/handlers/PlayerOps.ts:968-970`:

```typescript
[ScriptOpcode.GENDER]: state => {
    state.pushInt(state.activePlayer.gender);
},
```

`Engine-TS/src/engine/script/handlers/PlayerOps.ts:626-630`:

```typescript
[ScriptOpcode.P_PREVENTLOGOUT]: checkedHandler(ProtectedActivePlayer, state => {
    state.activePlayer.preventLogoutMessage = check(state.popString(), StringNotNull);
    state.activePlayer.preventLogoutUntil = World.currentTick + check(state.popInt(), NumberNotNull);
}),
```

DAMAGE and GENDER use raw `state =>` (no `checkedHandler`). DAMAGE doesn't gate on any pointer (it looks up the target player by UID). GENDER reads `state.activePlayer.gender` and would NPE if Self is nil — TS preserves this quirk.

### §4.2 — Surface

| Layer | File | Change |
|---|---|---|
| Interface | `pkg/script/active.go` | Add `SetPreventLogout(message string, untilTick int)` and `ApplyDamage(amount, dmgType int)` to `ActivePlayer`. |
| Handler | `pkg/script/handlers_player.go` | New `handleDamage`, `handleGender`, `handlePPreventLogout`. |
| Dispatch | `pkg/script/handlers.go` | Three new entries: `OpDamage`, `OpGender`, `OpPPreventLogout`. |
| Impl | `modules/world/player_script.go` (or sibling, re-grepped) | New `(p *Player) SetPreventLogout(msg string, until int) { p.preventLogoutMessage = msg; p.preventLogoutUntil = until }`. New `(p *Player) ApplyDamage(amount, dmgType int) { p.Damage(amount, dmgType) }` (delegates to existing `Player.Damage` at `player_masks.go:126`). |
| Test fixture | `pkg/script/handlers_player_test.go` | Extend `mockPlayer` with `preventLogoutMessage string`, `preventLogoutUntil int`, `applyDamageCalls []mockApplyDamage` recording fields + getter `Gender() int`. |

### §4.3 — Handler bodies

```go
// pkg/script/handlers_player.go

// handleDamage (DAMAGE, opcode 2015) applies damage to the player
// resolved from a UID popped from the stack. Pop order (TS): amount,
// hitType, uid (LIFO via popInt). Silent no-op if the UID does not
// resolve to a logged-in player. Mirrors TS PlayerOps.ts:768-779.
//
// DEVIATION-NAI-127-D1 (mirrors NPC_FINDHERO): defensive nil-s.World
// guard. Without s.World there is no way to resolve the UID.
//
// Note: no PtrActivePlayer gate — TS uses raw `state =>`, not
// checkedHandler. This is intentional; the handler's input is fully
// stack-driven and does not read state.activePlayer. Any caller that
// invokes DAMAGE without first establishing the target via UID is
// correct script-side; the handler does not read `s.Self`.
func handleDamage(s *ScriptState) error {
    amount := s.PopInt()
    hitType := s.PopInt()
    uid := s.PopInt()
    if s.World == nil {
        return nil
    }
    player := s.World.LookupPlayerByUID(uid)
    if player == nil {
        return nil
    }
    player.ApplyDamage(amount, hitType)
    return nil
}

// handleGender (GENDER, opcode 2020) pushes the active player's
// gender (0=male, 1=female). Mirrors TS PlayerOps.ts:968-970.
//
// DEVIATION-NAI-127-D2: TS uses raw `state =>` — there is no pointer
// gate (no requireActivePlayer). state.activePlayer access is
// nil-unsafe. Goscape preserves this quirk per ts_asymmetry_dual_pin.
// Tests pin both presence (Self set → correct push) and the absence
// (no error path is exercised — PtrActivePlayer is not required).
// Retire only if upstream TS adds a checkedHandler wrapping.
func handleGender(s *ScriptState) error {
    s.PushInt(s.Self.Gender())
    return nil
}

// handlePPreventLogout (P_PREVENTLOGOUT, opcode 2084) sets the
// player's anti-log message and absolute tick deadline. Pop order
// (TS): popString first (message), then popInt (additional ticks
// from current tick). Mirrors TS PlayerOps.ts:626-630.
func handlePPreventLogout(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "P_PREVENTLOGOUT"); err != nil {
        return err
    }
    if s.World == nil {
        return nil
    }
    ticks := s.PopInt()
    msg := s.PopString()
    s.Self.SetPreventLogout(msg, s.World.CurrentTick()+ticks)
    return nil
}
```

### §4.4 — Test cases (RED → GREEN; 8 tests)

**DAMAGE (3):**
1. `TestDamage_HappyPath` — uid=42, amount=7, hitType=1, mockWorld.players[42]=p → `p.applyDamageCalls = [{amount=7, dmgType=1}]`.
2. `TestDamage_UnknownUID` — uid=99, players empty → no calls recorded, no error.
3. `TestDamage_NoPointerGate` — Pointers=0, valid lookup → still applies damage. (Pins absence of `requireActivePlayer` per TS quirk.)

**GENDER (2):**
4. `TestGender_Male` — Self.gender=0 → push 0.
5. `TestGender_Female` — Self.gender=1 → push 1.

**P_PREVENTLOGOUT (3):**
6. `TestPPreventLogout_HappyPath` — Self set + protected pointer + msg="Combat" + ticks=16 + currentTick=100 → `Self.preventLogoutMessage="Combat"`, `Self.preventLogoutUntil=116`.
7. `TestPPreventLogout_RequiresProtected` — Pointers=PtrActivePlayer (not protected) → error.
8. `TestPPreventLogout_NoActivePlayer` — Pointers=0 → error.

### §4.5 — Bundle 2 task plan (3 implementer tasks, then reviewer)

- T2.1 (Haiku) — `ActivePlayer.SetPreventLogout` + `ActivePlayer.ApplyDamage` interface methods + `*Player` impls + `mockPlayer` recording fields.
- T2.2 (Haiku) — RED: 8 tests.
- T2.3 (Haiku) — GREEN: 3 handlers + 3 dispatch entries.
- Reviewer (Sonnet): TS fidelity per opcode; verify GENDER deliberately omits `requireActivePlayer`; P_PREVENTLOGOUT protected-active-player gate via `requireProtectedActivePlayer`.

## §5 — Smoke matrix & close decision tree

User-launched (Java client) per `smoke_test_server_handoff` after Bundle 2 close. Fresh char + bronze dagger vs Tutorial Island giant rat at coord box outside chat-suppression zone (per `java_client_coord_chat_suppression`).

| # | Item | Pre-NAI-127 (`e3bd6cc`) | Post-NAI-127 expected | Binding? |
|---|---|---|---|---|
| 1 | `[ai_queue3,newbiegiantrat]` no longer aborts at pc=2 | `no handler for NPC_FINDHERO (opcode 2519)` WARN | WARN silenced | **PRIMARY** (close-binding) |
| 2 | Rat death-loot drop visible on ground | rat dies, no loot | `npc_param(death_drop)` + `raw_rat_meat` ground-objs render | **PRIMARY** (close-binding) |
| 3 | Rat melee hit renders red splat on player | hit-splat rendering may be 0/suppressed | non-zero red splats, HP decrements | secondary cascade |
| 4 | Player hit-sound plays on rat-hits-player | silent | wav plays | secondary cascade |
| 5 | Combat-prevent-logout message on logout-attempt during combat | crash/silent | "You can't log out until 10 seconds…" | secondary cascade |
| 6 | Server-log unhandled-opcode count | 62 | 56 | hygiene |

**Close decision tree (per `cascade_theory_smoke_binding` + `smoke_surfaces_adjacent_divergences`):**
- **Items 1+2 PASS, items 3-5 PASS:** close PRIMARY met, no carry-forward.
- **Items 1+2 PASS, items 3-5 FAIL with adjacent-handler WARN:** close per row 4 of `smoke_surfaces_adjacent_divergences`; route surfaced opcode(s) to NAI-128 with WARN evidence.
- **Items 1+2 PASS, items 3-5 FAIL without WARN:** port has a defect; block close, investigate.
- **Item 1 or 2 FAIL:** port has a defect; block close.

## §6 — Deviations & risks

**DEVIATION-NAI-127-D1 (Bundle 1 + Bundle 2 DAMAGE):** Defensive nil-`s.World` guard tagged "(goscape defensive; TS skips this check)" per `defensive_gate_doc_comment_label`. Mirrors `handleNpcDel` from NAI-126. Retire when an upstream invariant proves `s.World` non-nil for any executing script.

**DEVIATION-NAI-127-D2 (Bundle 2 GENDER):** GENDER handler INTENTIONALLY omits `requireActivePlayer` to preserve TS quirk (`PlayerOps.ts:968-970` uses raw `state =>`). Pin both presence (Self set → push correct gender) AND absence (no error path) per `ts_asymmetry_dual_pin`. Retire only on upstream TS fix.

**Risk: BOTH_HEROPOINTS test fixture symmetry.** Test 10 sets both `Self` and `Self2` non-nil; test 11 reverses; test 12 sets only one. Per `plan_runnable_test_fixtures` mental-execute the fixtures at plan-write.

**Risk: Bundle 2 dispatched without independent smoke between bundles.** Bundle 1 alone doesn't reach a smoke-binding state (rat death-loot needs both NPC_FINDHERO returning true AND obj_add succeeding — both wired). If Bundle 1 has defects, Bundle 2's smoke surfaces them as item 1 failures.

**Risk: NPC_FINDHERO returns 0 because the player→NPC ledger isn't being credited.** Per NAI-120 Bundle 2D, NPC_HEROPOINTS is wired. The credit site is `[proc,player_melee_attack]` line 31 in `player_melee.rs2`. If `player_melee_attack` is itself blocked by another unhandled opcode upstream of `npc_heropoints($damage_capped)`, item 2 will FAIL with "rat dies, no loot" — symptom indistinguishable from NPC_FINDHERO returning 0 in goscape's empty-ledger case. Mitigation: smoke item 1 (no WARN) is the close-binding, not item 2; item 2 failure with no WARN routes to NAI-128 investigation.

**Risk: `Player.heroPoints` clear hooks not wired.** TS clears `player.heroPoints` on stat-resets at `PlayerOps.ts:514, 553, 610`. Goscape will not wire these hooks in NAI-127. The rat-AI smoke does not exercise stat-reset paths, so this is acceptable for NAI-127 PRIMARY. Tracked as carry-forward.

## §7 — Out of scope / carry-forward

**Out of scope for NAI-127:**
- Other 56 unhandled opcodes from `/tmp/claude/unhandled.txt`.
- Player death/respawn handling (`[queue,player_death]`); reaches it only on player HP=0.
- `ring_of_life_check` / `ring_of_recoil_lose_charge` content procs.
- `~combat_preventlogout` cleanup-on-tick path. Tick-end check that consumes `preventLogoutUntil` may not yet exist; verify at plan-write that the field is read at logout-attempt time. If not, that's a separate NAI-N+M.
- `Player.heroPoints` clear hooks (PlayerOps.ts:514, 553, 610).

**Carry-forward (NAI-128+ candidates):**
- Smoke-surfaced adjacent unhandled opcodes (route per item 6 with WARN evidence).
- `Player.heroPoints` clear-on-stat-reset hooks (parked).
- Prior-NAI carryovers (NAI-115 P1/P2, NAI-117, NAI-119, NAI-121 residuals #2/#3, NAI-111, parked deviations) unchanged.

**Eligible to retire on NAI-127 Bundle 1 close:** the 8 grep hits referencing `NAI-40-SB2 FINDHERO + BOTH_HEROPOINTS` carry-forward in `nai_followups.md` (lines 2638, 2660, 2709, 2763, 2817, 2881, 3497, 3549).

## §8 — Pattern memories that apply

- `missing_handler_audit` — used at §1 to size 62 declared-but-unhandled and constrain to the 6-opcode rat-AI subset.
- `cascade_theory_smoke_binding` — close on item 1+2 (root-cause silencing), route items 3-5 residuals.
- `smoke_surfaces_adjacent_divergences` — adjacent-handler WARN routing decision tree.
- `controller_preflight` — re-grep all plan premises (line numbers, helper signatures, struct field names, alphabetic placement) at HEAD before each implementer dispatch.
- `audit_subagent_fabrication` — controller did the TS-source reads directly (verbatim quoted in §3.1 / §4.1); no audit subagent dispatched.
- `verify_implementer_claims` — fresh independent `go build ./... && go test ./... && go vet ./...` after each commit.
- `superpowers_code_reviewer_model` — Sonnet reviewer per bundle, never Opus.
- `execution_mode_default` — dispatched via subagent-driven-development without offering a mode menu.
- `defensive_gate_doc_comment_label` — DEVIATION-NAI-127-D1 + D2 doc-comment labels.
- `ts_asymmetry_dual_pin` — GENDER absence-pin tests for D2.
- `mock_recorder_field_naming_check` — implementer greps existing `mockPlayer` / `mockNpc` field names at HEAD before adding new recorders.
- `scriptstate_test_fixture_idioms` — explicit `StackCapacity` init, push-order matched to pop-order, `Pointers` flags per case.
- `plan_runnable_test_fixtures` — mental-execute test fixtures at plan-write.
- `plan_grep_helper_patterns` — grep `requireProtectedActivePlayer` etc. before inlining a guard in P_PREVENTLOGOUT.
- `feedback_subagent_wt_path` — `git status` post-each-commit confirms clean main tree.
- `close_commit_memory_trailer` — applied on close commit.
- `superpowers_clear_between_spec_and_impl` — after spec is approved and plan is written, controller emits resume prompt and stops; user `/clear`s before implementing.
- `smoke_test_server_handoff` — user-launched smoke for Bundle 2 close.
- `java_client_coord_chat_suppression` — smoke setup must be outside Tutorial Island chat-suppression coord boxes.
