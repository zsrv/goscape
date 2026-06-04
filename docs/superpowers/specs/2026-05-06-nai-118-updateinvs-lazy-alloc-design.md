# NAI-118 — `Player.updateInvs` lazy-allocate listener inv (smithing-interface empty fix)

**Cadence:** Compressed (per `compressed_cadence`). Combined spec + plan inline; subagent-driven execution; no formal review loop.

**Investigation:** `investigation_subspec_cadence` Bundle 0 short-circuit per `bundle0_short_circuits_stage1_audit` — Bundle 0 produced a binding line-level TS-source diff; Bundle 1 audit subagent SKIPPED.

**Tech stack:** Go 1.26+ (pkg `modules/world`).

---

## §1. Symptom

Surfaced by NAI-117 smoke (2026-05-06): on Tutorial Island, after the Mining Instructor steps gate the player past `^newbie_mining_instructor_smith_a_dagger`, using a bronze bar on an anvil opens the smithing interface but **no items are shown to smith**. The interface frame renders; the `column1..column5` slots are empty.

---

## §2. Bundle 0 — controller pre-flight (verified at HEAD `044f1bb`)

### §2.1 Dispatch trace

1. **Client** sends OPLOCU (item-on-loc) for bronze_bar onto anvil.
2. **`handleOpLocU`** (`modules/world/handler_oploc.go:230-325`) gates the click, snapshots `p.lastUseItem = useObj`, sets `p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)`. ✅ verified.
3. **`tryFireOpTrigger` → `fireOpTriggerLoc`** (`modules/world/interaction_trigger.go:126-194`) on contact-range tick fires `[oplocu,anvil]` via `apLocTriggerForOp(targetOpLocU) + 7 → TriggerOpLocU (71)`. ✅ verified.
4. **Content script** `LostCityRS/Content/scripts/skill_smithing/scripts/smithing/smithing.rs2:33` runs:
   - Tutorial-progress gates at lines 34-44.
   - Past gates: `oc_category(last_useitem) = category_151` (bronze_bar matches) → `~smithing_anvil_interface(last_useitem)`.
   - `[label,smithing_anvil_interface]` (line 59) issues per-bar `inv_transmit(smithing_bronze1..5, smithing:column1..5)` (lines 121-125), then `if_openmain(smithing)` (line 171). ✅ smoke confirms interface opens, so the script reaches line 171.
5. **`handleInvTransmit`** (`pkg/script/handlers_inv.go:431-442`) calls `s.Self.InvListenOnCom(invType, com, s.Self.UID())`. ✅ verified.
6. **`Player.invListenOnCom`** (`modules/world/player.go:981-1004`) registers listener; for `SCOPE_SHARED` rewrites source to -1, otherwise stores caller-supplied source. ✅ verified.

### §2.2 The bug

`Player.updateInvs` (`modules/world/player.go:766-805`) reads the listener's inventory via direct map access:

```go
for com, l := range p.invListeners {
    var inv *inventory.Inventory
    if l.Source == -1 {
        inv = p.client.server.invs[l.Type]               // ← direct map read
    } else {
        otherActive := p.client.server.LookupPlayerByUID(l.Source)
        if otherActive == nil { continue }
        other, ok := otherActive.(*Player)
        if !ok || other == nil { continue }
        inv = other.invs[l.Type]                         // ← direct map read
    }
    if inv == nil { continue }                           // ← silently skips wire emit
    ...
}
```

When a listener is registered for an InvType that no prior code path has accessed, both branches return `nil` and the wire emission is silently dropped. No fallback, no allocation.

### §2.3 TS counterpart (`Engine-TS/src/engine/entity/Player.ts:1400-1438`)

```typescript
getInventoryFromListener(listener) {
    if (listener.source === -1) {
        return World.getInventory(listener.type);     // lazy-allocs SHARED
    } else {
        const player = World.getPlayerByUid(listener.source);
        if (!player) return null;
        return player.getInventory(listener.type);    // lazy-allocs per-player
    }
}

getInventory(inv) {
    ...
    if (invType.scope === InvType.SCOPE_SHARED) {
        container = World.getInventory(inv);
    } else {
        container = this.invs.get(inv);
        if (!container) {
            container = Inventory.fromType(inv);       // ← seeds StockObj
            this.invs.set(inv, container);
        }
    }
    return container;
}
```

TS lazy-allocates and seeds `StockObj` from the InvType template at the wire-emission path. Goscape's direct map lookup does neither.

### §2.4 Why it surfaces here specifically

The `smithing_bronze1..5` invs in `LostCityRS/Content/scripts/skill_smithing/configs/smithing/smithing.inv` declare no `scope=` line, so they default to `SCOPE_TEMP=0` (per TS `InvType.ts:72` default — verified). Their stock items are configured as `stock1=bronze_dagger,1` etc., compiled into `StockObj`/`StockCount` and consumed by `inventory.FromType` (`pkg/inventory/inventory.go:40-53`).

Because no script reads or writes these inventories before `inv_transmit` registers the listener, they exist only as InvType templates — not as live `Inventory` objects in `player.invs[type]`. The wire emission's direct map lookup returns `nil`, the wire packet is dropped, and the client renders the interface with empty columns.

`invLookupView.Get` (`modules/world/server_invs.go:15-47`) already implements the correct lazy-alloc-plus-stock-seeding logic and is used by the script-op path. The wire-emission path (`updateInvs`) just doesn't go through it.

---

## §3. Fix shape

Route `updateInvs`'s per-listener inv lookup through `Server.invLookup` (`modules/world/server.go:78`, `invLookupView`), eliminating the direct map access. Single source of truth between the script-op path and the wire-emission path; matches the TS structure where `getInventoryFromListener` delegates to the same `getInventory` used by ops.

For `l.Source == -1`: `invListenOnCom` only sets source to -1 for `SCOPE_SHARED` types. `invLookupView.Get` for `SCOPE_SHARED` ignores the `self` argument and returns `srv.invs[typeID]`, lazy-allocating from `inventory.FromType` (which seeds `StockObj`). Pass any `script.ActivePlayer` (e.g., `p`); irrelevant.

For `l.Source != -1`: resolve `other = srv.LookupPlayerByUID(l.Source)` (typed as `script.ActivePlayer`), pass to `invLookupView.Get`. The view's per-player branch lazy-allocates into `other.invs[typeID]`.

The post-lookup `if inv == nil { continue }` guard stays — guards against the legitimate `InvType.Configs[type] == nil` case (out-of-range or unregistered type).

---

## §4. Diff (production)

`modules/world/player.go` — `updateInvs`, function body only (signature, doc-comment, surrounding methods unchanged):

```go
func (p *Player) updateInvs() {
    if p.client == nil || p.client.server == nil {
        return
    }
    srv := p.client.server
    // Collect all observed invs so we can clear Update after all listeners fire.
    observed := make([]*inventory.Inventory, 0, len(p.invListeners))
    for com, l := range p.invListeners {
        var self script.ActivePlayer
        if l.Source == -1 {
            // SCOPE_SHARED listener — invLookupView.Get ignores self for shared.
            self = p
        } else {
            self = srv.LookupPlayerByUID(l.Source)
            if self == nil {
                continue
            }
        }
        inv := srv.invLookup.Get(self, l.Type)
        if inv == nil {
            continue
        }

        if inv.Update || l.FirstSeen {
            sendUpdateInvFullCom(p, l.Com, inv)
            if l.FirstSeen {
                // Flip via read-modify-write — map values are not addressable.
                l.FirstSeen = false
                p.invListeners[com] = l
            }
        }
        observed = append(observed, inv)
    }
    // Clear inv.Update AFTER all listeners (multiple listeners can share an inv).
    for _, inv := range observed {
        inv.Update = false
    }
}
```

Net change: replace the direct map-access block (~12 lines) with a 4-line lookup through `srv.invLookup.Get`. No imports added (`script` already imported at `modules/world/player.go:17`).

The pre-existing typed-nil cleanup `other, ok := otherActive.(*Player); if !ok || other == nil { continue }` is dropped; `srv.invLookup.Get` accepts `script.ActivePlayer` and downcasts internally with a nil-typed-pointer-safe check (`server_invs.go:33-36`).

---

## §5. Tests (TDD red-then-green)

Add to `modules/world/player_inv_test.go`. Both tests fail at HEAD (red) and pass after the §4 fix (green).

### §5.1 `TestUpdateInvsLazyAllocSeedsStockTemp`

Pin: SCOPE_TEMP listener for an unallocated InvType with `StockObj` populated emits a `sendUpdateInvFullCom` carrying the stock items.

```go
func TestUpdateInvsLazyAllocSeedsStockTemp(t *testing.T) {
    // InvType: SCOPE_TEMP, capacity 5, stock=[bronze_dagger=100, bronze_sword=101].
    cfg := &objtype.InvType{
        ConfigType: objtype.ConfigType{ID: testInvTypeID},
        Scope:      objtype.InvTypeScopeTemp,
        Size:       5,
        StockObj:   []uint16{100, 101, 0, 0, 0},
        StockCount: []uint16{1, 1, 0, 0, 0},
    }
    configs := make([]*objtype.InvType, testInvTypesLen)
    configs[testInvTypeID] = cfg

    p, cc := newTestPlayer(t)
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    s := &Server{
        log:       discardLogger(),
        invTypes:  &objtype.InvTypeConfigs{Configs: configs},
        invs:      make(map[int]*inventory.Inventory),
        playerLoop: []*Player{p},  // for LookupPlayerByUID
    }
    s.invLookup = invLookupView{s: s}
    p.client.server = s
    p.uid = 12345
    p.active = true

    // Register listener at com=149 — SCOPE_TEMP, source=p.UID.
    p.invListenOnCom(testInvTypeID, 149, p.uid)
    if p.invListeners[149].Source != p.uid {
        t.Fatalf("setup: Source got %d, want %d", p.invListeners[149].Source, p.uid)
    }
    if _, ok := p.invs[testInvTypeID]; ok {
        t.Fatal("setup: per-player inv slot must be empty before updateInvs")
    }

    received := drainConn(t, cc)
    p.updateInvs()
    p.client.flushWrite()

    got := <-received
    if len(got) == 0 {
        t.Fatal("updateInvs should emit a wire packet for a SCOPE_TEMP listener with stock items")
    }
    // Wire shape: 1 opcode + 1 size byte (UpdateInvFull is var-byte; size in payload header) +
    // payload: P2 com + P1 size + per-slot (P2 id+1, P1 count) for size slots.
    // Decode just enough to assert the first two slots are bronze_dagger=100 and bronze_sword=101.
    // (Wire format per modules/world/inv_update.go:15-41.)
    // ... assert decoded payload contains com=149, size=5, slot0=(100+1, 1), slot1=(101+1, 1).

    // Lazy-alloc must have populated p.invs[testInvTypeID].
    if got, ok := p.invs[testInvTypeID]; !ok || got == nil {
        t.Error("updateInvs should lazy-allocate per-player inv from InvType")
    } else if got.GetItemCount(100) != 1 || got.GetItemCount(101) != 1 {
        t.Errorf("lazy-allocated inv missing stock items: got items %v", got.Items)
    }
}
```

### §5.2 `TestUpdateInvsLazyAllocSeedsStockShared`

Pin: SCOPE_SHARED listener (source rewritten to -1 by `invListenOnCom`) emits a `sendUpdateInvFullCom` from a freshly lazy-allocated `srv.invs[type]`.

Same structure as §5.1 with:
- `Scope: objtype.InvTypeScopeShared`
- `p.invListenOnCom(testInvTypeID, 149, 99)` — caller source=99 rewritten to -1 by SHARED scope branch.
- Pre-condition: `s.invs[testInvTypeID]` must be absent.
- Post-condition: `s.invs[testInvTypeID]` populated with stock items; wire packet decoded matches.

### §5.3 Test scaffolding notes

- `inventory` and `script` packages already imported by `player_inv_test.go` neighbors; add as needed.
- The wire-decode assertions can use the `drainConn`/`<-received` pattern already established by `TestInvStopListenOnComRemovesListener` (`player_inv_test.go:89-115`). Decoding the payload requires reading past the opcode byte and the var-byte size header. If the existing helpers don't cover this, add a local helper in the test file (do not pollute `inv_update.go`).
- The bronze_dagger=100, bronze_sword=101 IDs are arbitrary positive ints — they don't need to match real ObjType IDs because `sendUpdateInvFullCom` doesn't validate against `ObjType`.

---

## §6. Verification

Per `verification_before_completion`:

1. Both tests in §5 fail at HEAD (`go test -run 'TestUpdateInvsLazyAlloc' ./modules/world/...`).
2. Apply §4 diff.
3. Both tests pass.
4. Full `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
5. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.

---

## §7. Smoke (user-launched, post-merge)

Per `smoke_test_server_handoff`: ask user to relaunch the server with the new binary and verify the smithing case binds.

**Smoke target:** Mining Instructor smithing-interface empty (NAI-117 SECONDARY residual #2).

**Procedure:**
1. Java client login.
2. Tutorial Island Mining Instructor area.
3. Tutorial state past `^newbie_mining_instructor_smith_a_dagger` (have hammer + bronze bar in inventory).
4. Use bronze bar on anvil.

**PRIMARY bind:** Smithing interface opens AND shows the bronze item set:
- column1: bronze_dagger, bronze_sword, bronze_scimitar, bronze_longsword, bronze_2h_sword
- column2: bronze_axe, bronze_mace, bronze_warhammer, bronze_battleaxe
- column3: bronze_chainbody, bronze_platelegs, bronze_plateskirt, bronze_platebody
- column4: bronze_med_helm, bronze_full_helm, bronze_sq_shield, bronze_kiteshield
- column5: bronze_dart_tip, bronze_arrowheads, bronze_knife, bronzecraftwire

(Per `LostCityRS/Content/scripts/skill_smithing/configs/smithing/smithing.inv:1-35`.)

**Secondary check:** No console errors on the click. No regressions in any other inventory-driven feature (player main inventory `inv` was already pre-allocated by `processLogins`, so its emission path was already exercising live-`Inventory` lookups; the fix only changes behavior for listeners pointing at unallocated InvTypes).

---

## §8. Out of scope

- **Boot-time SCOPE_SHARED pre-allocation** (TS `World.ts:222-235` pre-allocates all SHARED invs at world boot): unnecessary because the lazy-alloc path covers correctness. Could be added as a startup optimization if the lazy-alloc cost on the first `updateInvs` ever materializes; not measured.
- **`smithing_anvil` content-script semantics** (XP curves, bar consumption, dialog choices): not in scope. Engine-side wire-emission is the only fault.
- **NAI-117 SECONDARY residual #1** (run-mode visible-effect wiring, `defaultMoveSpeed`/`tempRun` port): separate sub-spec candidate per `nai_followups`.

---

## §9. Pattern memories applied

- `bundle0_short_circuits_stage1_audit` — Bundle 0 produced a binding line-level diff; Bundle 1 audit SKIPPED.
- `compressed_cadence` — combined spec+plan, single `docs(spec): ...` commit, no formal review loop, subagent execution.
- `controller_preflight` — Bundle 0 verified all anchor lines/symbols at HEAD `044f1bb`.
- `investigation_subspec_cadence` — Stage 1 collapsed into Bundle 0; Stage 2 = §4 fix; smoke is binding feature gate.
- `dispatch_correct_reach_blocked` — frames the NAI-117 close: PRIMARY (run-mode opcode-error silence) closed; SECONDARY content outcomes (smithing items rendered) bind here.
- `smoke_test_server_handoff` — user-launched smoke binds.
- `close_commit_memory_trailer` — apply on close commit.

---

## §10. Plan cadence

Compressed: single subagent dispatch.

**Task 1 — TDD red→green→commit:**
1. Write red tests per §5.1 + §5.2 in `modules/world/player_inv_test.go`. Run; verify both fail at HEAD.
2. Apply §4 diff to `Player.updateInvs` in `modules/world/player.go`.
3. Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — must be clean.
4. Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — must be clean.
5. Commit as `fix(world): NAI-118 — updateInvs lazy-allocate listener inv (smithing-interface empty fix)` with hand-off content listing files touched, line counts, and HEAD verification.

After commit: controller drives a fresh `go test ./...` + `git show HEAD --stat` post-flight verification (`verify_implementer_claims`), then hands off to user for smoke.

If smoke binds: close commit `chore(close): NAI-118 — final close after smoke binding` with `Closes memory:` trailer and full residual report (anticipated: none expected; possible adjacent surprises will route per `smoke_surfaces_adjacent_divergences`).
