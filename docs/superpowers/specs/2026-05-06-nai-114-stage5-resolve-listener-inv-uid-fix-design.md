# NAI-114 Stage 5 — `resolveListenerInv` UID-vs-slot fix design

**Date:** 2026-05-06
**Predecessor:** [NAI-114 Stage 4 spec](2026-05-06-nai-114-stage4-reject-gate-probe-design.md) (closed; smoke bound `inv_unresolved` at site #10).
**Status:** Stage 5 fix sub-spec — patches `resolveListenerInv` to interpret `InventoryListener.Source >= 0` as a UID (matching TS) instead of a slot index. Closes NAI-114 on user-smoke confirmation.

---

## 1. Symptom + Stage 4 binding

NAI-114 Stage 4 instrumentation shipped at commit `1f657ab` and was smoked on tutorial firemaking. 5/5 OPHELDU events (ticks 63, 74, 77, 80, 85) bound:

```
opheldu entry  obj=2511 slot=2 comId=3214 useObj=590 useSlot=1 useComId=3214 delayed=false delayedUntil=60
opheldu reject gate=inv_unresolved listener_type=93 listener_source=2232170497
```

`listener_source=2232170497` matches the player's UID (cross-confirmed via `interaction_debug` logs `player_uid=2232170497` from earlier ticks). Sites #1-#9 all passed silently — the listener IS registered for `comId=3214`, with `Type=93`, `Source=PlayerUID`. Site #10 (`resolveListenerInv == nil`) fires every event.

## 2. Root cause

`resolveListenerInv` (`modules/world/handler_opnpc.go:14-26`) treats `listener.Source >= 0` as a slot index into `s.players[]`:

```go
if listener.Source < 0 || listener.Source >= len(s.players) {
    return nil
}
other := s.players[listener.Source]
```

UIDs (~2.2 billion at the smoke binding) are orders of magnitude larger than `len(s.players)` (max 2047 per `NodeMaxPlayers`). The bounds check trips, returns nil — every player-source listener silently fails.

**TS reference** (`LostCityRS/Engine-TS/src/engine/entity/Player.ts:getInventoryFromListener`):

```ts
} else if (listener.source === -1) {
    return World.getInventory(listener.type);
} else {
    const player = World.getPlayerByUid(listener.source);
    if (!player) {
        return null;
    }
    return player.getInventory(listener.type);
}
```

`source` is unambiguously a UID. `World.getPlayerByUid` is the correct lookup primitive.

**goscape registration sites** (`pkg/script/handlers_inv.go`):

- L440: `s.Self.InvListenOnCom(invType, com, s.Self.UID())` (INV_TRANSMIT)
- L486: `s.Self.InvListenOnCom(invType, com, uid)` (INVOTHER_TRANSMIT — `uid` popped from script stack, also a UID)

Both pass UIDs. Registration is correct.

**Sister consumer** (`modules/world/player.go:766-805` `updateInvs`) already does the right thing:

```go
otherActive := p.client.server.LookupPlayerByUID(l.Source)
if otherActive == nil { continue }
other, ok := otherActive.(*Player)
if !ok || other == nil { continue }
inv = other.invs[l.Type]
```

Only `resolveListenerInv` indexes-by-slot. Singular site — verified by grepping `s.players[X]` non-test occurrences (3 hits: `handler_opnpc.go:21` is the buggy one; `server.go:740` and `server.go:756` use `p.slot` correctly).

This is the latent half of a partial NAI-24 fix. Per `pkg/script/handlers_inv.go:421-422`, the registration site originally hard-coded `Source=-1` and was corrected at NAI-24 Bundle 2 to pass UID. The corresponding fix on the resolution side never landed because no smoke exercised the player-source branch end-to-end (sidebar inv display works because `updateInvs` was always correct; OPHELDU and friends route through the buggy resolver).

## 3. Scope

### In scope
- Rewrite `resolveListenerInv` body to mirror `updateInvs`'s UID-lookup pattern.
- Update its doc comment to say "UID" instead of "slot".
- Add 4 unit tests in a new file `modules/world/resolve_listener_inv_test.go` pinning all four branches.
- After user-smoke confirms tutorial firemaking works: revert the Stage 4 probe (`1f657ab`) in this same sub-spec and ship a NAI-114 close commit with `Closes memory:` trailer.

### Out of scope
- Any change to registration sites (`pkg/script/handlers_inv.go`) — already correct.
- Any change to other handlers (OPNPC, OPLOC, OPOBJ, OP_PLAYER, INV_BUTTON) that consume `resolveListenerInv` — they all benefit from the fix without modification.
- Audit of other UID-vs-slot consumers — already grepped (no other site).
- Test-helper extraction. `newTestServer` + `newTestPlayer` + the existing `s.players[i] = &Player{slot: i}` + `s.playerLoop = append(...)` pattern at `server_test.go:475-479` is sufficient for the multi-player fixture.

## 4. Stage 1 — fix implementation

### 4.1 The function rewrite

Replace `modules/world/handler_opnpc.go:9-26` with:

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

Type-assert pattern is identical to `player.go:781-784` — `LookupPlayerByUID` returns `script.ActivePlayer` (an interface), and the consumer needs `*Player` to read the unexported `invs` field.

### 4.2 InventoryListener struct field comment

`modules/world/player.go:26` currently says:

```go
Source    int  // -1 = world-shared inventory, else owning player's slot
```

Update to:

```go
Source    int  // -1 = world-shared inventory, else owning player's UID
```

### 4.3 Tests — new file `modules/world/resolve_listener_inv_test.go`

Four test cases pinning the function contract independently of any handler:

**Test 1: `TestResolveListenerInvWorldSource`**
- Setup: `s := newTestServer(t)`; populate `s.invs[42] = inventory.New(...)`.
- Call: `inv := resolveListenerInv(s, InventoryListener{Type: 42, Source: -1})`.
- Assert: `inv == s.invs[42]` (pointer equality).

**Test 2: `TestResolveListenerInvPlayerSourceMatch`** — *the regression pin for this bug*
- Setup: `s := newTestServer(t)`; create a target player at `s.players[5]` with `slot=5`, `uid=98765`, `active=true`; append to `s.playerLoop`; populate `target.invs[42]` with a known inventory.
- Call: `inv := resolveListenerInv(s, InventoryListener{Type: 42, Source: 98765})`.
- Assert: `inv == target.invs[42]` (pointer equality). Confirms UID lookup, not slot indexing.

**Test 3: `TestResolveListenerInvPlayerSourceOffline`**
- Setup: `s := newTestServer(t)`; no player with UID 999999 active.
- Call: `inv := resolveListenerInv(s, InventoryListener{Type: 42, Source: 999999})`.
- Assert: `inv == nil` (no panic, no slot-OOB).

**Test 4: `TestResolveListenerInvPlayerSourceNullInv`**
- Setup: same as Test 2 but `target.invs[42]` left nil.
- Call: `inv := resolveListenerInv(s, InventoryListener{Type: 42, Source: 98765})`.
- Assert: `inv == nil`.

Existing `handler_inv_button_test.go:105` (`delete(s.invs, 93)` exercising the world-source nil path at the handler level) stays unchanged.

### 4.4 Build + test verification

After fix lands:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: build clean; all PASS including 4 new tests. Existing handler tests unaffected since they use `Source=-1` (now-untouched branch) or fixtures with no listener at all.

### 4.5 Commit sequence

1. `docs(spec): NAI-114 Stage 5 — resolve-listener-inv UID fix design` ← this commit.
2. `docs(plan): NAI-114 Stage 5 — resolve-listener-inv UID fix plan` ← from `writing-plans`.
3. `fix(world): NAI-114 Stage 5 — resolveListenerInv interpret Source as UID` ← single fix commit (function rewrite + struct comment + 4 tests).

## 5. Stage 2 — user-smoke handoff + close routing

User re-launches goscape after commit 3 lands, connects via Java client rev-225, walks tutorial fire-making (same character, same tinderbox-on-logs sequence as Stages 3 and 4).

### 5.1 Decision matrix

| Smoke outcome | Routing |
|---|---|
| Tutorial firemaking succeeds (fire produced) | Smoke ✅ — proceed to §6 close. |
| OPHELDU still fails, gate is now `objType_unregistered` (#15 / #16) for obj 2511 or 590 | New downstream gate; open Stage 6 in same NAI to investigate objType cache. |
| OPHELDU reaches trigger lookup (no `opheldu reject` line) but no fire produced | Dispatch is unblocked but content fails downstream — route to NAI-115 brainstorm; close NAI-114 anyway since the dispatch fix landed. |
| OPHELDU still binds `inv_unresolved` | Fix didn't take effect — investigate at-rest test pass + smoke fail mismatch (likely `LookupPlayerByUID` doesn't see this listener's source player; rare). Stage 6. |

### 5.2 Stage 5 close (after smoke ✅)

Three commits land in this order:

4. `Revert "chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation"` ← `git revert 1f657ab`. Removes the 18 inline DEBUG lines + `snapshotInvListenerKeys` helper + `slog`/`sort` imports. Tree returns to pre-Stage-4 shape.
5. `chore(memory): NAI-114 close — UID-vs-slot semantic-name collision` ← memory file additions only (no production code). Adds new memory entry; updates `cascade_theory_smoke_binding` and `investigation_subspec_cadence` with NAI-114 example.

(NAI close commits also carry `Closes memory:` trailers per `close_commit_memory_trailer`.)

## 6. Risk register

| # | Risk | Probability | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Multi-player test fixture wiring (target player needs `active=true`, `uid` set, slot wired into both `s.players[]` and `s.playerLoop`) is finicky | LOW | Test-author churn | Pattern at `server_test.go:475-479` is reusable: `s.players[i] = &Player{slot: i}; s.playerLoop = append(s.playerLoop, s.players[i])`. Test 2 just adds `uid` and `active=true` and an `invs` slice. |
| R2 | Smoke surfaces a *different* downstream gate (likely `objType_unregistered` for obj 2511 or 590) | LOW | Stage 6 needed | Probe is still in tree at smoke time; new binding logged automatically without code change. |
| R3 | Other player-source-listener handlers still have hidden defects past `resolveListenerInv` | LOW | Latent | All 6 handlers funnel through this function; the smoke covers OPHELDU but every other handler resolves identically post-fix. |
| R4 | `LookupPlayerByUID` linear-scans `s.playerLoop`; a future caller in a tight loop could be perf-sensitive | NIL | None for this fix | Out of scope; mirrors TS's `World.getPlayerByUid`. |
| R5 | Test 2 fixture needs `target.client.server` if any test path enters code that dereferences it | LOW | Test panic | `resolveListenerInv` only reads `target.invs`; no client/server deref. Verified at spec-write time. |
| R6 | New tests fail when run with `-race` due to playerLoop concurrent access | LOW | CI flake | All tests use a freshly-built server; no goroutines started. Same shape as existing `LookupPlayerByUID` test fixtures (per `server_test.go` patterns). |

## 7. Tech stack & deliverables

- **Go 1.26+** per `go_version`.
- **TS source:** `LostCityRS/Engine-TS/src/engine/entity/Player.ts:getInventoryFromListener` per `ts_source_canonical_path`.

**Memory updates on NAI-114 close (Stage 5 close commit 5):**

- New entry on UID-vs-slot semantic-name collision (suggested name `inventory_listener_source_uid_not_slot.md`). Same pattern as `loctype_blockwalk_vs_flag_blockwalk` — a field whose name doesn't disambiguate two competing semantics, with one consumer following one convention and another consumer following the opposite.
- Update `cascade_theory_smoke_binding.md` with NAI-114 as a 4-stage probe → fix example.
- Update `investigation_subspec_cadence.md` with NAI-114 as a 4-stage chain (Stage 1 = brainstorm, Stage 3 = first probe, Stage 4 = second probe after first under-anticipated, Stage 5 = fix).
- Closes memory trailer on the NAI-114 close commit per `close_commit_memory_trailer`.

---

## 8. Self-review

1. **Placeholder scan:** none. Every test fixture detail is explicit; the function rewrite is full source.
2. **Internal consistency:** §1 binding ↔ §2 root cause ↔ §4.1 fix shape are all aligned on UID-vs-slot. §4.2 doc-comment fix matches §4.1's struct semantics.
3. **Scope check:** focused single-function fix; the close work is straightforward (1 revert + 1 memory commit).
4. **Ambiguity check:** §4.3 specifies pointer-equality assertions (not deep-compare) so test failures will be unambiguous. §5.1 splits four smoke outcomes with explicit routing.
5. **Test/library capability:** unit tests use plain `if got != want { t.Errorf(...) }` patterns matching the rest of `modules/world/*_test.go`; no library-capability mismatch (per `spec_library_capability_match`).
6. **Dropped-tests crosscheck:** no existing tests are dropped or modified; 4 new tests are added (per `plan_test_coverage_crosscheck`).
