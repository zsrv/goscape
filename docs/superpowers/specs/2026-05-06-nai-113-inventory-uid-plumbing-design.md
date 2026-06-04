# NAI-113 — Inventory side-panel uid plumbing fix

**Date:** 2026-05-06
**Status:** spec — direct fix sub-spec (Bundle 0 line-level diff short-circuit; no Stage 1 audit cadence)
**Predecessor:** NAI-112 SECONDARY residual (PRIMARY closed at `679122a`)
**Tech Stack:** Go 1.26+
**TS reference:** `LostCityRS/Engine-TS` (canonical port source)

## 1. Symptom

After NAI-112 PRIMARY fix (`679122a`), Tutorial Island chatbox advances to "Cut down a tree" on inventory-tab click as required. The SECONDARY half remains open: bronze axe (1351) + tinderbox (590) added by `[tutorial,_]` `^newbie_survival_instructor_open_inventory` branch
```
inv_add(inv, bronze_axe, 1);
inv_add(inv, tinderbox, 1);
```
do not display in the Java client's inventory side panel after the click. Inventory contents are added server-side (NAI-112 Bundle 1 INV_ADD logs verified) but never serialized to the client.

## 2. Root cause (static analysis at HEAD `679122a`)

**Bug A — `Player.uid` is never assigned in production code.**

`modules/world/player.go:430` initializes `uid: -1`. Grep over `modules/world/` confirms no production path writes to `p.uid` post-construction. The constructor's value persists for the player's lifetime.

TS `Engine-TS/src/engine/World.ts:937` composes uid at `addPlayer`:
```ts
player.uid = ((Number(player.username37 & 0x1fffffn) << 11) | player.slot) >>> 0;
```
Goscape's `Server.addPlayer` (`modules/world/server.go:683-704`) sets `p.slot` and pushes to `playerLoop` but skips this composition.

**Bug B — `updateInvs` indexes `Source` as a slot, not a uid.**

`modules/world/player.go:777`:
```go
other := p.client.server.players[l.Source]
```
`s.players` is a slot-indexed slice. TS `Player.ts:1406` does:
```ts
const player = World.getPlayerByUid(listener.source);
```
Goscape's `Server.LookupPlayerByUID` (`server.go:784`) is the analogue and is already used by `handleFindUID`/`handlePFindUID`. It is not used here.

**Combined effect.** `INV_TRANSMIT` handler (`pkg/script/handlers_inv.go:431`) calls `s.Self.InvListenOnCom(invType, com, s.Self.UID())`. `s.Self.UID()` returns `-1` (Bug A). `invListenOnCom` (`player.go:977-1001`) stores `Source: -1`. `updateInvs` enters the `Source == -1` branch (`player.go:774-776`), looks up `server.invs[l.Type]` (the world-shared inv table), which is `nil` for per-player invtypes (`inv` has `scope=perm` per `Content/scripts/player/configs/player.inv:1-5`). Result: emit silently skipped. No `UpdateInvFull` packet ever reaches the client for the active player's own inventory.

**Why existing tests pass.** `inv_update_test.go:32-33` and 4 sibling test files manually populate `viewer.invListeners = {... Source: 2 ...}` and `s.players[2] = owner`, bypassing both the production UID composition AND validating the slot-indexed lookup that's only correct under that test convention. Production paths never go through this fixture shape.

**Latent uid-broken consumers (audit surfaced; covered by stretch scope per Q4):**
- `handleFindUID` (`handlers_player.go:851-866`) — `LookupPlayerByUID(uid)` always returns nil; FINDUID always pushes 0.
- `handlePFindUID` (`handlers_player.go:868-899`) — same; self-reacquire fast-path `s.Self.UID() == uid` is always `-1 == uid` → always false.
- `handleUID` (`handlers_dialog.go:115-121`) — pushes `-1` to runescript stack instead of composed uid.

## 3. Design

### 3.1 Production changes

**3.1.1 New helper — `modules/world/player_uid.go` (new file, ~12 LOC).**
```go
package world

// composeUID derives a Player.uid from username37 + slot per TS
// Engine-TS World.ts:937:
//   uid = ((username37 & 0x1FFFFF) << 11) | slot   (>>> 0 truncation)
// The lower 21 bits of username37 are shifted up 11 bits; the 11-bit
// slot occupies the low bits. Stable per (account, slot) for the
// session. Same formula appears at the test helper newInvListenerTestPlayer.
func composeUID(username37 uint64, slot int) int {
    return int(((username37 & 0x1FFFFF) << 11) | uint64(slot&0x7FF))
}
```

**3.1.2 `Server.addPlayer` (`server.go:683-704`).** Insert after `p.slot = i`:
```go
p.uid = composeUID(p.username37, p.slot)
```
Single line; no other addPlayer changes.

**3.1.3 `Player.updateInvs` (`player.go:766-800`).** Replace line 777:
```go
// before
other := p.client.server.players[l.Source]
// after
otherActive := p.client.server.LookupPlayerByUID(l.Source)
other, ok := otherActive.(*Player)
if !ok || other == nil {
    continue
}
```
Type-assert mirrors `pkg/script/handlers_player.go:857-862` convention. `LookupPlayerByUID` returns `script.ActivePlayer`; goscape uses concrete `*Player` from there.

### 3.2 Test fixture migration

All five test files use the pattern `viewer.invListeners = {Source: <slot>}` + `s.players[<slot>] = owner`. After Bug B fix, `LookupPlayerByUID(<slot>)` returns nil because no test player has `uid==<slot>`. Migration:

1. `newInvListenerTestPlayer(t, s, slot)` (and any sibling helpers — `addPlayerToServer`, etc.) sets `p.uid = composeUID(p.username37, p.slot)` after the slot assignment. If `username37` is unset in tests (zero), composeUID falls back to slot-only — still unique per (test) player.
2. Each test's `Source: <slot>` literal becomes `Source: owner.uid` (or `Source: owner.UID()`).

**Files affected (5):**
- `modules/world/inv_update_test.go` (4 fixtures + helper)
- `modules/world/player_inv_test.go` (~6 fixtures)
- `modules/world/modal_close_test.go` (~3 fixtures)
- `modules/world/handler_opobj_test.go` (~3 fixtures)
- `modules/world/player_clearcomlisteners_test.go` (~3 fixtures)

Pre-flight enumeration is mandatory (per `enumerate_all_sites.md`); plan-author re-greps `Source: \d+` and `\.invListeners = ` at plan-write time.

### 3.3 Stretch coverage (Q4-B)

Add unit tests under `pkg/script/`:

**3.3.1 FINDUID two-player lookup.** Two `mockPlayer` fixtures with composed uids; FINDUID pushes 1 and rebinds `s.Self` when `popInt() == other.uid`. (Existing `handlers_player_test.go` may already have a FINDUID skeleton; extend if so.)

**3.3.2 P_FINDUID self-reacquire fast-path.** `s.Self.UID() == popped uid && s.Protect == true` short-circuits push 1 without lookup. Pre-fix: `-1 == any_uid` was always false, so this path was dead. Test asserts the fast-path triggers and that `PlayerLookup` is NOT consulted (e.g., via a panicking-on-call lookup stub).

**3.3.3 UID opcode (`handlers_dialog.go:115-121`) push.** Asserts the opcode pushes a non-(-1) value matching the active player's composed uid.

**3.3.4 INV_TRANSMIT registers Source=composed-uid.** Existing `TestInvTransmitRegistersListener` (`handlers_inv_test.go:388`) currently passes against `mockPlayer.uid = 42` (literal). Update mockPlayer fixture to populate uid via composition (or explicit literal that's consistent with what production would compose) and assert the listener's Source matches.

### 3.4 Smoke test (binding closure per Q3-A)

Manual smoke under user-launched server (per `smoke_test_server_handoff.md`):
1. Fresh account → spawn at tutorial start coord.
2. Walk through to Survival Expert dialogue.
3. Trigger `^newbie_survival_instructor_open_inventory` (chatbox prompts inventory tab click).
4. Click inventory tab.
5. **Pin:** inventory side panel shows bronze axe in slot 0 AND tinderbox in slot 1.

Smoke binds NAI-113 closure per cascade-theory smoke rule. Adjacent untracked divergences route per `smoke_surfaces_adjacent_divergences.md` (in-scope-stretch if ≤30 LOC, else NAI-114).

## 4. Test strategy

| Layer | Test | Asserts |
|-------|------|---------|
| Unit | `composeUID` direct | Formula matches TS World.ts:937 against (username37, slot) inputs including username37=0 + max-21-bit cases |
| Unit | INV_TRANSMIT registers composed-uid Source | Mock player with composed uid; listener.Source == that uid (not -1) |
| Unit | FINDUID two-player rebind | popInt(other.uid) → push 1, s.Self rebinds, Pointers |= PtrActivePlayer |
| Unit | P_FINDUID self-reacquire fast-path | popInt(self.uid) under Protect=true → push 1 without consulting PlayerLookup |
| Unit | UID opcode push | Pushes composed uid; asserts != -1 |
| Integration | `updateInvs` self-listener emits UpdateInvFull | Player with own inv as listener-Source uid; FirstSeen + Update both fire emit |
| Integration | `updateInvs` cross-player listener (INVOTHER_TRANSMIT shape) | Two test players with composed uids; viewer's listener at Source=owner.uid produces packet for owner.invs[Type] |
| Integration | All 5 migrated test files green | No `Source: <int_literal>` regressions; LookupPlayerByUID lookup path exercised |
| Smoke | Tutorial Island bronze axe + tinderbox display | User-launched server; Java client renders both items in inventory side panel post-tab-click |

## 5. Risk register

- **R1 — `username37=0` test players collide on lookup.** All test players with empty username37 produce uid=slot. Two test players in different slots get distinct uids; same slot shouldn't coexist. **Mitigation:** the migration produces unique uids per slot already; no risk.
- **R2 — Reconnection path breaks uid stability mid-session.** Goscape's reconnection flow (if any) might re-call `addPlayer` and recompose uid. **Mitigation:** TS recomposes too (same formula on each addPlayer); behavior matches. Document in spec §3.1.2 that recomposition is intentional.
- **R3 — Concurrent `Source==-1` listener registration paths legitimate.** A future feature might register Source=-1 for a world-shared inv listener (bank shared-scope). **Mitigation:** preserve the `Source == -1` branch in `updateInvs`; spec §3 §edge-cases flags it as currently-unused-in-production but supported.
- **R4 — `LookupPlayerByUID` linear-scan cost regresses tick budget.** O(active players) per listener per tick. **Mitigation:** TS does the same scan. Listener counts per player are bounded (1-3 typical). Not a measurable regression.
- **R5 — Plan-time miscount of `Source: \d+` sites.** Five files enumerated above; plan-author re-greps at plan-write. **Mitigation:** controller pre-flight per `controller_preflight.md` re-runs `rg "Source:\s*\d+" modules/world/` against HEAD before each implementer dispatch.

## 6. Out of scope

- Cross-player INVOTHER_TRANSMIT smoke (Q3-A defers to NAI-114+ if a real content flow surfaces residuals; unit test from §4 covers the lookup-path correctness).
- Audit of `Player.UID()` consumers beyond the three latent sites (FINDUID/PFINDUID/UID). Other sites (`interaction_debug.go`, `handler_oploc.go`) only log uid as an opaque identifier; pre-fix `-1` log noise is harmless.
- `Player.uid` persistence to/from disk. Goscape, like TS, recomposes per session.

## 7. Closes

- NAI-112 SECONDARY residual (inventory side panel post-tab-click).
- Latent uid-broken consumers FINDUID/PFINDUID/UID (audit-and-test as part of stretch coverage; no behavior-change to production handler bodies).

## 8. Cadence

Direct fix (no Stage 1 audit). Per `bundle0_short_circuits_stage1_audit.md`: TS source diff at World.ts:937 is binding evidence; grep evidence for Bug A (zero `p.uid =` writes in production) and Bug B (`players[l.Source]` literal at `player.go:777`) is greppable in <60s by controller pre-flight. Stage 1 audit subagent would only re-verify these grep findings.

Bundles:
- **B1: production fix** — composeUID helper + addPlayer call + updateInvs LookupPlayerByUID switch.
- **B2: test fixture migration** — five test files migrated via composed-uid helper.
- **B3: stretch coverage** — FINDUID/PFINDUID/UID/INV_TRANSMIT unit tests.
- **B4: smoke + close commit with `Closes memory:` trailer per `close_commit_memory_trailer.md`.**

## 9. Memory entries to add at close

- Verified-at-HEAD note on `Player.uid` composition formula (TS World.ts:937 cite + composeUID location).
- "uid-broken consumers cascade fix" pattern entry: when a struct field defaults to a sentinel (-1) and consumers fall through to dead branches, fixing the field's assignment can simultaneously close multiple "stub-not-completed" sites; audit them before merging the assignment fix.
