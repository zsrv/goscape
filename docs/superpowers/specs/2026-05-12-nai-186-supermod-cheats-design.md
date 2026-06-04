# NAI-186 — Super-mod cheats (setvis/ban/mute/kick)

**Date:** 2026-05-12
**Author:** Claude (Opus 4.7, 1M context)
**Status:** Draft for review
**Predecessors:** NAI-183 (super-mod outer guard + ::tele/::getcoord), NAI-184 (mod cheat cohort), NAI-185 (admin block non-spawn cheats; close commit b2631d4)
**Tech stack:** Go 1.26+, modules/world

## 1. Goal

Port the four `staffModLevel >= 2 && NODE_PRODUCTION` super-mod cheats from
`Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:549-616` into
the existing goscape super-mod switch at
`modules/world/handlers_game.go:765` (introduced by NAI-183).

- `::setvis <level>` — toggle player visibility (DEFAULT / SOFT / HARD)
- `::ban <username> <minutes>` — schedule moderation ban via loginBridgeMod
- `::mute <username> <minutes>` — schedule moderation mute via loginBridgeMod
- `::kick <username>` — flag a logged-in player for logout

Closes the **super-mod cluster** of the NAI-185-D4 carryforward block at
`modules/world/handlers_game.go:367-381`. Two clusters remain after this
sub-spec: **Dev block** (reload/rebuild/speed) and **Admin block**
(locadd/npcadd/openmain).

## 2. Scope decision

Single cohort, one sub-spec. Reasoning:

- All 4 cheats share the same gate (`NodeProduction && staffModLevel >= 2`).
- All 4 sit in the same TS switch block (L549-616) and route to the same
  goscape switch (handlers_game.go:765+).
- All structural building blocks are already in goscape: `rsbuf.Visibility*`
  constants, `BlockWalk*` constants, `s.gamemap.ChangeNPCCollision` /
  `ChangePlayerCollision`, `s.LookupPlayerByUsername`, `loginBridgeMod.Notify*`,
  `recordingBridges` fixture, `parseIntOr` helper.
- The NAI-185 carryforward block claimed "setvis trivial; ban/mute/kick non-trivial."
  Pre-flight analysis inverted this: setvis is the moderate one (blockWalk +
  collision flag transitions, an SOFT message-only stub), while ban/mute/kick
  are 5-10 LOC each routed through existing infrastructure.

(Carryforward "trivial/non-trivial" framing treated as hypothesis per memory
`tracker_entry_framing_can_be_incomplete` and `risk_register_premise_grep`.)

## 3. TS source (canonical reference)

Path: `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts`

### 3.1 setvis (L549-568)

```ts
} else if (cmd === 'setvis' && Environment.NODE_PRODUCTION) {
    // authentic
    if (args.length < 1) {
        // ::setvis <level>
        return false;
    }

    switch (args[0]) {
        case '0':
            player.setVisibility(Visibility.DEFAULT);
            break;
        case '1':
            player.setVisibility(Visibility.SOFT);
            break;
        case '2':
            player.setVisibility(Visibility.HARD);
            break;
        default:
            return false;
    }
}
```

Calls `Player.setVisibility` (Player.ts:1875-1891):

```ts
setVisibility(visibility: Visibility) {
    if (visibility === Visibility.SOFT) {
        this.messageGame(`vis: ${visibility} (not implemented - you are still on vis: ${this.visibility})`);
        return;
    }
    // This doesn't actually cancel interactions, source: ...
    this.visibility = visibility;
    if (visibility === Visibility.DEFAULT) {
        this.blockWalk = BlockWalk.NPC;
        changeNpcCollision(this.width, this.x, this.z, this.level, true);
    } else {
        this.blockWalk = BlockWalk.NONE;
        changeNpcCollision(this.width, this.x, this.z, this.level, false);
        changePlayerCollision(this.width, this.x, this.z, this.level, false);
    }
    this.messageGame(`vis: ${visibility}`);
}
```

### 3.2 ban (L569-581)

```ts
} else if (cmd === 'ban' && Environment.NODE_PRODUCTION) {
    if (args.length < 2) {
        player.messageGame('Usage: ::ban <username> <minutes>');
        return false;
    }
    const username = args[0];
    const minutes = Math.max(0, tryParseInt(args[1], 60));
    World.notifyPlayerBan(player.username, username, Date.now() + minutes * 60 * 1000);
    player.messageGame(`Player '${args[0]}' has been banned for ${minutes} minutes.`);
}
```

### 3.3 mute (L582-594) — same shape as ban with NotifyPlayerMute

### 3.4 kick (L595-616)

```ts
} else if (cmd === 'kick' && Environment.NODE_PRODUCTION) {
    if (args.length < 1) {
        player.messageGame('Usage: ::kick <username>');
        return false;
    }
    const username = args[0];
    const other = World.getPlayerByUsername(username);
    if (other) {
        other.loggingOut = true;
        if (isClientConnected(other)) {
            other.logout();
            other.client.close();
        }
        player.messageGame(`Player '${args[0]}' has been kicked from the game.`);
    } else {
        player.messageGame(`Player '${args[0]}' does not exist or is not logged in.`);
    }
}
```

## 4. Architecture

### 4.1 New `Player.SetVisibility` method

File: `modules/world/player.go`

```go
// SetVisibility mirrors TS Player.setVisibility (Engine-TS/src/engine/entity/Player.ts:1875-1891).
// SOFT is a TS stub: emits a "not implemented" message and returns without
// state change. DEFAULT and HARD update visibility + blockWalk and toggle
// per-tile collision flags. Player.width is always 1 in TS (PathingEntity
// init); hardcode size=1 here.
func (p *Player) SetVisibility(v rsbuf.Visibility) {
    if v == rsbuf.VisibilitySoft {
        p.MessageGame(fmt.Sprintf("vis: %d (not implemented - you are still on vis: %d)", int(v), int(p.visibility)))
        return
    }
    p.visibility = v
    s := p.client.server
    if v == rsbuf.VisibilityDefault {
        p.blockWalk = BlockWalkNpc
        s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, true)
    } else {
        p.blockWalk = BlockWalkNone
        s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, false)
        s.gamemap.ChangePlayerCollision(1, p.x, p.z, p.level, false)
    }
    p.MessageGame(fmt.Sprintf("vis: %d", int(v)))
}
```

Imports needed in player.go: confirm `fmt` and `rsbuf` are already imported
(they are — `rsbuf.Visibility` field at player.go:389 and existing
`p.MessageGame(fmt.Sprintf(...))` patterns elsewhere).

### 4.2 New cheat case-arms in handler

File: `modules/world/handlers_game.go`, inside `if p.staffModLevel >= 2` switch
at L765-context (after the existing `getcoord` / `tele` arms).

```go
case "setvis":
    // TS L549-568 — ::setvis <level>. NodeProduction-gated.
    if !p.client.server.cfg.NodeProduction {
        return nil
    }
    if args == "" {
        return nil
    }
    sub := strings.SplitN(args, " ", 2)
    switch sub[0] {
    case "0":
        p.SetVisibility(rsbuf.VisibilityDefault)
    case "1":
        p.SetVisibility(rsbuf.VisibilitySoft)
    case "2":
        p.SetVisibility(rsbuf.VisibilityHard)
    default:
        return nil
    }
case "ban":
    // TS L569-581 — ::ban <username> <minutes>. NodeProduction-gated.
    if !p.client.server.cfg.NodeProduction {
        return nil
    }
    sub := strings.SplitN(args, " ", 2)
    if len(sub) < 2 || sub[0] == "" {
        p.MessageGame("Usage: ::ban <username> <minutes>")
        return nil
    }
    username := sub[0]
    minutes := parseIntOr(sub[1], 60)
    if minutes < 0 {
        minutes = 0
    }
    p.client.server.loginBridgeMod.NotifyPlayerBan(p.username, username, time.Now().Add(time.Duration(minutes)*time.Minute))
    p.MessageGame(fmt.Sprintf("Player '%s' has been banned for %d minutes.", username, minutes))
case "mute":
    // TS L582-594 — ::mute <username> <minutes>. NodeProduction-gated.
    if !p.client.server.cfg.NodeProduction {
        return nil
    }
    sub := strings.SplitN(args, " ", 2)
    if len(sub) < 2 || sub[0] == "" {
        p.MessageGame("Usage: ::mute <username> <minutes>")
        return nil
    }
    username := sub[0]
    minutes := parseIntOr(sub[1], 60)
    if minutes < 0 {
        minutes = 0
    }
    p.client.server.loginBridgeMod.NotifyPlayerMute(p.username, username, time.Now().Add(time.Duration(minutes)*time.Minute))
    p.MessageGame(fmt.Sprintf("Player '%s' has been muted for %d minutes.", username, minutes))
case "kick":
    // TS L595-616 — ::kick <username>. NodeProduction-gated.
    if !p.client.server.cfg.NodeProduction {
        return nil
    }
    if args == "" {
        p.MessageGame("Usage: ::kick <username>")
        return nil
    }
    sub := strings.SplitN(args, " ", 2)
    username := sub[0]
    if other := p.client.server.LookupPlayerByUsername(username); other != nil {
        // DEVIATION-NAI-186-D1 — TS does inline `other.logout(); client.close()`.
        // Goscape sets loggingOut=true and lets processLogouts (tick.go:277)
        // handle the teardown chain (writeOut OpLogout, flushWrite, conn.Close,
        // removePlayer). Same end-state, ≤1 tick defer.
        other.loggingOut = true
        p.MessageGame(fmt.Sprintf("Player '%s' has been kicked from the game.", username))
    } else {
        p.MessageGame(fmt.Sprintf("Player '%s' does not exist or is not logged in.", username))
    }
```

Convention notes:

- Uses `p.client.server.X` inline (mirrors NAI-184 L445-451 reboot/serverdrop
  pattern; no local `s := p.client.server` alias).
- Uses `parseIntOr(arg, default)` (already-used at handlers_game.go:449, 474, etc.).
- Uses `time.Now().Add(time.Duration(minutes) * time.Minute)` — equivalent to
  TS `Date.now() + minutes * 60 * 1000`.
- Argument splitting via `strings.SplitN(args, " ", 2)` per cheat (the outer
  handler already split `cheat → cmd + args` once; each cheat re-splits if it
  takes multiple args).

### 4.3 Carryforward block update

File: `modules/world/handlers_game.go:367-381`.

Drop the super-mod cluster from the carryforward listing; supersede the tag
`DEVIATION-NAI-185-D4-CARRYFORWARD` → `DEVIATION-NAI-186-D2-CARRYFORWARD`.
Remaining clusters: Dev block (reload/rebuild/speed) and Admin block
(locadd/npcadd/openmain) — 6 cheats total.

Deviation-tag numbering for NAI-186: D1 = kick teardown defer (§5.5);
D2 = carryforward continuation block.

## 5. Data flow & invariants

### 5.1 Argument parsing semantics (TS-faithful)

The outer handler (handlers_game.go:358) lowercases the entire `cheat` string
via `strings.ToLower(raw)`. So all 4 cheats see lowercase `args`, including
usernames. TS does the same (`input.toLowerCase()` at L42-43). Match.

### 5.2 `parseIntOr` + `< 0 → 0` clamp

TS: `Math.max(0, tryParseInt(args[1], 60))`.
Two-step expression: `tryParseInt` falls back to `60` on unparseable;
`Math.max(0, …)` clamps the parsed result.

Go reproduction:

```go
minutes := parseIntOr(sub[1], 60)  // default-on-parse-failure
if minutes < 0 {                   // clamp negative
    minutes = 0
}
```

Behavior table:

| input          | TS result | Go result |
|----------------|-----------|-----------|
| `"30"`         | 30        | 30        |
| `"abc"`        | 60 (default; max(0, 60)) | 60 (default; 60 ≥ 0 untouched) |
| `"-5"`         | 0 (max clamp) | 0 (`< 0` clamp) |
| `"0"`          | 0         | 0         |

Match across all cases.

### 5.3 until-time formula

TS: `Date.now() + minutes * 60 * 1000` (milliseconds).
Go: `time.Now().Add(time.Duration(minutes) * time.Minute)`.

The two bridge call sites (`handler_reportabuse.go:50`,
`handler_message_private.go:42`) already use `time.Now().Add(48 * time.Hour)`
style — NAI-186 follows the same convention.

### 5.4 staff argument: `p.username`, NOT `"automated"`

NAI-186 ban/mute pass `p.username` (the invoking moderator's username) — this
is a manual-staff invocation. Distinguishes from:

- `handler_reportabuse.go:50` — passes `"automated"` (rule-triggered).
- `handler_message_private.go:42` — passes `"automated"` (auto-ban on
  unrecognised PM target).

Real semantic difference; not just plumbing.

### 5.5 kick teardown (DEVIATION-NAI-186-D1)

TS sequence (L606-611):

```
other.loggingOut = true;
if isClientConnected(other):
    other.logout();
    other.client.close();
```

Goscape sequence:

```
other.loggingOut = true;
// (defer to processLogouts at tick.go:277)
```

`processLogouts` runs every tick on every player with `loggingOut=true` and:

1. clears `activeScript`,
2. `writeOut(OpLogout, nil)`,
3. `flushWrite()`,
4. `conn.Close()`,
5. `s.removePlayer(p)`.

Same end-state. Order-of-operations within the kicking tick differs (TS: kick
mid-tick on the kicker's PM-decode path triggers victim teardown immediately;
goscape: victim teardown happens when the tick reaches `processLogouts`).
Tag as DEVIATION-NAI-186-D1; retire if/when goscape grows a synchronous
force-logout helper.

## 6. Error handling

| Case                                           | TS behavior                            | Go behavior                             |
|------------------------------------------------|----------------------------------------|-----------------------------------------|
| `NodeProduction=false` (any of 4 cheats)       | arm-selector false → no dispatch       | inner `if NodeProduction` false → `return nil` |
| `staffModLevel < 2`                            | outer block not entered                | outer block not entered                 |
| setvis: no arg                                 | `args.length < 1` → `return false`     | `args == ""` → `return nil`             |
| setvis: bad arg (e.g. `setvis 5`)              | `default: return false`                | `default: return nil`                   |
| ban/mute: < 2 args                             | usage message + `return false`         | usage message + `return nil`            |
| ban/mute: target user not logged in            | no check; bridge call still made       | same; bridge call still made            |
| ban/mute: minutes unparseable                  | tryParseInt → 60                       | parseIntOr → 60                         |
| ban/mute: minutes negative                     | `max(0, ...)` → 0                      | `if minutes < 0 { minutes = 0 }` → 0    |
| kick: no arg                                   | usage message + `return false`         | usage message + `return nil`            |
| kick: target not in playerLoop                 | "does not exist or is not logged in"   | same                                    |

No new error types; no new bridge methods. The cheat handler returns `error`
but every existing arm returns `nil` regardless of validation outcome (cheats
are best-effort; bad input is silently ignored or messaged).

## 7. Testing strategy

### 7.1 New test file: `modules/world/handler_cheats_supermod_test.go`

(Naming mirrors any precedent in the package — fall back to alongside
existing `handlers_game_test.go` if no per-cluster test file exists. Plan
must grep at write-time.)

### 7.2 Test fixtures

- Existing `recordingBridges` (modules/world/bridges_test.go:75-79) with
  `loginMod []recordedLoginModCall{method, staff, username, until}` slice —
  use as-is. Plan-author must grep the struct definition for exact field
  names per memory `mock_recorder_field_naming_check`.
- Existing test-player factory (used by reportabuse / message_private tests)
  — verify it sets up `p.client.server.cfg`, `p.client.server.loginBridgeMod`
  (recordingBridges), and inserts into `s.playerLoop` with `active=true`.
- Dispatch via `handleClientCheat(p, payload)` directly; build payload using
  the `::<cmd> <args>` encoder pattern at handlers_game_test.go:389
  (`dispatchTeleCheat`-style helper).
- **Fixture risk per memory `drainconn_iocopy_race`**: if any test combines
  `go io.Copy(io.Discard, cc)` with a `drainAfterTele`-style call on the same
  conn, the discard goroutine races the drain reader. Pick ONE pattern per
  conn; prefer the existing reportabuse-test convention if it has one.

### 7.3 Test cases

**setvis (6 cases):**

| Case | Cheat                    | Expected post-state                                          | Expected message                                              |
|------|--------------------------|--------------------------------------------------------------|---------------------------------------------------------------|
| 1    | `::setvis 0` (NP=true)   | `visibility=Default, blockWalk=Npc`; one ChangeNPCCollision(1,x,z,level,true) | "vis: 0"                                          |
| 2    | `::setvis 1` (NP=true)   | unchanged (SOFT stub: visibility, blockWalk, collision); test fixture starts at `visibility=Default (0)` per player.go:556 | "vis: 1 (not implemented - you are still on vis: 0)" |
| 3    | `::setvis 2` (NP=true)   | `visibility=Hard, blockWalk=None`; ChangeNPCCollision(...,false) + ChangePlayerCollision(...,false) | "vis: 2"                  |
| 4    | `::setvis 5` (NP=true)   | unchanged                                                    | none                                                          |
| 5    | `::setvis ` (NP=true)    | unchanged                                                    | none                                                          |
| 6    | `::setvis 0` (NP=false)  | unchanged                                                    | none                                                          |

Case 2 is TS-faithful absence-pin per memory `ts_asymmetry_dual_pin` —
pin both the message AND the unchanged state.

For collision-call assertions, either (a) read back the
`s.gamemap.Pathfinder.Flags` at the player's tile to verify FlagBlockNPCs /
FlagBlockPlayers presence/absence (matches existing
`npc_registry_test.go:206` precedent with mask `FlagBlockNPCs | FlagBlockPlayers`),
or (b) use a fake/spy on `gamemap` if one exists. Prefer (a) — real-flag
read-back is the production-faithful path. Plan-author confirms which.

**ban (5 cases):**

| Case | Cheat                       | Expected loginMod[0]                                                   | Expected message                                  |
|------|-----------------------------|------------------------------------------------------------------------|---------------------------------------------------|
| 1    | `::ban bob 30` (NP=true)    | `method=NotifyPlayerBan, staff=p.username, username=bob, until≈now+30m` | "Player 'bob' has been banned for 30 minutes."   |
| 2    | `::ban bob` (NP=true)       | empty                                                                  | "Usage: ::ban <username> <minutes>"               |
| 3    | `::ban bob abc` (NP=true)   | `method=NotifyPlayerBan, ..., until≈now+60m`                           | "Player 'bob' has been banned for 60 minutes."   |
| 4    | `::ban bob -5` (NP=true)    | `until≈now` (0 minutes)                                                | "Player 'bob' has been banned for 0 minutes."    |
| 5    | `::ban bob 30` (NP=false)   | empty                                                                  | none                                              |

`until` assertion: tolerance of ±100ms vs `time.Now().Add(minutes * time.Minute)` (same pattern as existing reportabuse/message_private tests).

**mute (parallel 5 cases):** identical structure, asserts `NotifyPlayerMute`,
"muted" in message.

**kick (4 cases):**

| Case | Setup                                       | Cheat                | Expected on target            | Expected message                                |
|------|---------------------------------------------|----------------------|-------------------------------|-------------------------------------------------|
| 1    | bob in playerLoop, active=true              | `::kick bob`         | `loggingOut=true`             | "Player 'bob' has been kicked from the game."   |
| 2    | bob not in playerLoop                       | `::kick bob`         | n/a                           | "Player 'bob' does not exist or is not logged in." |
| 3    | bob in playerLoop                           | `::kick ` (NP=true)  | unchanged                     | "Usage: ::kick <username>"                      |
| 4    | bob in playerLoop                           | `::kick bob` (NP=false) | unchanged                  | none                                            |

**Aggregate gate (1 case)** — only add if NAI-183 didn't already cover:
parametrized over all 4 cheats, asserts inert at `staffModLevel=1`.

### 7.4 Coverage rationale

Covers: TS-faithful happy path × 4, NodeProduction gate × 4, arg-validation
× 4, SOFT stub absence-pin (setvis), negative-clamp (ban), parse-fail default
(ban), lookup-miss (kick), `loggingOut=true` post-condition (kick).

Excluded from spec (rationale): kick teardown completion (tick-level — covered
by existing reboot_test.go:142 `processShutdown` and tick.go:277 reachable
state); username case-sensitivity (out of scope — pre-existing divergence at
::teleto).

## 8. Files to touch

| File                                                       | Change                                                  | Approx. LOC |
|------------------------------------------------------------|---------------------------------------------------------|-------------|
| `modules/world/player.go`                                  | + `SetVisibility(v Visibility)` method                  | ~25         |
| `modules/world/handlers_game.go`                           | + 4 case-arms in super-mod switch                       | ~80         |
| `modules/world/handlers_game.go:367-381`                   | retire super-mod from carryforward block; supersede tag | ~10 (net -5) |
| `modules/world/handler_cheats_supermod_test.go` (new)      | new test file, ~20 test cases                           | ~400        |

No new packages, no new bridge methods, no new types.

## 9. Risks & verification (pre-dispatch grep checklist for plan-author)

Per memory `risk_register_premise_grep` and `plan_sibling_site_guard_audit`,
the plan-author MUST grep+Read these against HEAD before codifying:

- **R1**: `parseIntOr(s string, def int) int` signature — `rg -n 'func parseIntOr' modules/world/`.
- **R2**: `recordingBridges.loginMod` field names (`method`, `staff`, `username`, `until`) —
  `rg -n 'recordedLoginModCall' modules/world/bridges_test.go`.
- **R3**: `MessageGame` method receiver and signature — `rg -n 'func.*MessageGame'`.
- **R4**: `s.gamemap.ChangeNPCCollision` exact name (caps) — `rg -n 'ChangeNPCCollision' pkg/gamemap/`.
- **R5**: existing test-player factory used by reportabuse/message_private —
  identify `newTestPlayer` / similar and confirm it inserts into `playerLoop`.
- **R6**: `cfg.NodeProduction` field — `rg -n 'NodeProduction' modules/world/config.go`.
- **R7**: collision flag read-back — confirm `s.gamemap.Pathfinder.Flags` is
  the accessor name (precedent: npc_registry_test.go:206).
- **R8**: pre-existing fmt/rsbuf/time/strings imports in `modules/world/player.go`
  and `modules/world/handlers_game.go` — add any missing.

Verification gates per memory `verify_implementer_claims`:

- Plan-author runs each test fixture mentally against the new SUT before
  committing (memory `plan_runnable_test_fixtures`).
- Controller pre-flight before each implementer dispatch (memory
  `controller_preflight`): 30-second grep+Read against HEAD.
- Per memory `enumerate_all_sites`: re-grep `DEVIATION-NAI-185-D4-CARRYFORWARD`
  at plan-write to confirm no stale references in adjacent files.

## 10. TS-faithfulness ledger (deviations)

| Tag                           | Cheat | Description                                                                                    | Retire when                                  |
|-------------------------------|-------|------------------------------------------------------------------------------------------------|----------------------------------------------|
| DEVIATION-NAI-186-D1          | kick  | Teardown defers to processLogouts within same tick rather than synchronous logout+conn.close   | goscape grows synchronous force-logout helper |
| DEVIATION-NAI-186-D2-CARRYFORWARD | (handler) | Dev + Admin clusters remain unported (6 cheats)                                            | Both clusters get their own sub-specs        |

No other deviations.

## 11. Closure protocol

At close (after all tests pass + smoke):

1. Verify cohort: `rg -n 'DEVIATION-NAI-186' modules/ pkg/` — confirm exactly
   one D1 anchor + one carryforward continuation block.
2. Run `go test -race -count=3 ./modules/world/...` for the supermod tests
   (per memory `drainconn_iocopy_race`).
3. Update `MEMORY.md` only if non-derivable lessons surface (e.g. carryforward
   "trivial" framing was wrong). Per memory `nai_followups`, do NOT add memory
   entries for code-derivable info.
4. Close commit message: `chore(close): NAI-186 — super-mod cheats cohort
   complete` with `Closes memory:` trailer if any memory file was added
   (memory `close_commit_memory_trailer`).
5. Resume prompt for next sub-spec candidate (memory `post_task_handoff`).

## 12. Out-of-scope

- Dev block (reload/rebuild/speed) — defer; subsystem work required.
- Admin block (locadd/npcadd/openmain) — defer; dynamic spawn + interface routing.
- Username case-sensitivity normalization at lookup — pre-existing divergence
  at ::teleto; cross-cohort follow-up.
- Per-cheat handler-file extraction (Approach B from brainstorm) — premature;
  revisit if handlers_game.go grows beyond a refactor threshold.
- Synchronous force-logout helper — defer; current `loggingOut=true → next
  processLogouts` chain has same end-state.
