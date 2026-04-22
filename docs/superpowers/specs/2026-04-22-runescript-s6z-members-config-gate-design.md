# S6z — Members-Config Runtime Gate Design

> **Sub-spec context:** Twenty-fifth runescript sub-spec. Closes S6m-D4 + S6o-D4. Adds the "members-only item on a free-to-play world" validation gate to `handleOpLocU` and `handleOpNpcU`, matching TS `OpLocUHandler.ts:70-73` and `OpNpcUHandler.ts:72-75`.

> **TS-faithfulness gate:** Matches TS exactly. **Zero new deviations.**

> **Scope:** Tiny single-task sub-spec. ~20 LOC impl + ~40 LOC tests.

## 1. Goal

When the server runs with `cfg.NodeMembers = false` (free-to-play world), reject OpLocU / OpNpcU wire clicks that attempt to use a members-only item (`ObjType.Members == true`). Send the player a specific message and drop the interaction.

## 2. TS reference

```typescript
// OpLocUHandler.ts:70-73
if (ObjType.get(useObj).members && !Environment.NODE_MEMBERS) {
    player.messageGame("To use this item please login to a members' server.");
    return;
}
```

Identical pattern at `OpNpcUHandler.ts:72-75`.

Placement: **after** the useCom validation (Component / inv listener checks) and **before** the state mutation (`lastUseItem = useObj`). Per TS source.

## 3. Goscape infrastructure

All pieces exist:
- `ObjType.Members bool` at `pkg/objtype/objtype.go:107`
- `Config.NodeMembers bool` at `modules/world/config.go:36` (CLI flag + YAML field, default `true`)
- `Server.objTypes *objtype.ObjTypeConfigs` at `modules/world/server.go:67`
- `Player.MessageGame(msg string)` at `modules/world/message_game.go:15`

No new infrastructure needed.

## 4. Architecture

One gate per handler, inserted between the S6p listener-validation block and the `lastUseItem = useObj` assignment:

```go
// S6m-D4 closed in S6z: reject members-only items on free-to-play
// worlds. Matches TS OpLocUHandler.ts:70-73.
if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
    if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
        p.MessageGame("To use this item please login to a members' server.")
        sendUnsetMapFlag(p)
        return nil
    }
}
```

Bounds/nil guards are defensive — matches the pattern already established in `handleOpLocU`'s LocType check (see line 255 of current code).

Note on behavior when `useObj` is out of bounds or ObjType is missing: silently skip the gate (falls through to the happy path). Consistent with TS, which `ObjType.get(useObj)` would throw if the ID is invalid — but goscape's earlier S6p gates already validated `HasAt(useSlot, useObj)` against a real inventory, so by the time this gate runs, `useObj` is guaranteed to be a real item ID. The defensive guard is belt-and-suspenders.

## 5. File map

| File | Action |
|---|---|
| `modules/world/handler_oploc.go` | Insert gate in `handleOpLocU`; flip S6m-D4 comment to closure form |
| `modules/world/handler_opnpc.go` | Insert gate in `handleOpNpcU`; flip S6o-D4 comment to closure form |
| `modules/world/handler_oploc_test.go` | Add 2 new tests (members-on-f2p rejected; members-on-members allowed) |
| `modules/world/handler_opnpc_test.go` | Add 2 new tests (same shape for NPC) |

## 6. Handler edits

### 6.1 `handleOpLocU` (handler_oploc.go)

Current validation order (post-S6p):
1. delayed
2. payload-len
3. viewport
4. GetLoc
5. LocType
6. listener lookup (S6p gate)
7. resolveListenerInv
8. HasAt
9. **NEW: members-config gate**
10. state mutation + SetInteraction

Also: update the S6m-D4 comment block in the docstring from "DEVIATION" to "S6m-D4 closed in S6z." Validation gates list grows from 7 to 8.

### 6.2 `handleOpNpcU` (handler_opnpc.go)

Same structure, same placement.

## 7. Tests

Per handler (4 new tests total):

```go
// TestHandleOpLocUMembersOnFreeWorldRejected — S6z closes S6m-D4.
// When s.cfg.NodeMembers is false and the useObj ObjType has Members=true,
// the handler rejects with UnsetMapFlag + a MessageGame packet.
func TestHandleOpLocUMembersOnFreeWorldRejected(t *testing.T) {
    s, p, _, cc := makeOpLocFixture(t)
    s.cfg.NodeMembers = false
    // Seed a members ObjType for useObj=1511.
    if s.objTypes == nil {
        s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
    }
    s.objTypes.Configs[1511] = &objtype.ObjType{
        ConfigType: objtype.ConfigType{ID: 1511, DebugName: "members_item"},
        Members:    true,
    }
    // Satisfy the S6p listener+inv gate: register a listener and put the item in slot 3.
    if s.invs == nil {
        s.invs = make(map[int]*inventory.Inventory)
    }
    inv := inventory.New(93, 28, inventory.StackNormal)
    inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
    s.invs[93] = inv
    p.invListenOnCom(93, 149, -1)

    received := drainConn(t, cc)
    _ = handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149))
    p.client.flushWrite()
    got := <-received

    if len(got) == 0 {
        t.Fatal("expected UnsetMapFlag/MessageGame packet; got nothing")
    }
    if p.target != nil {
        t.Error("target should remain nil for members-on-free rejection")
    }
}

// TestHandleOpLocUMembersOnMembersWorldAllowed — confirms the gate only
// fires when NodeMembers is false.
func TestHandleOpLocUMembersOnMembersWorldAllowed(t *testing.T) {
    s, p, loc, _ := makeOpLocFixture(t)
    s.cfg.NodeMembers = true // default, but explicit for clarity
    if s.objTypes == nil {
        s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
    }
    s.objTypes.Configs[1511] = &objtype.ObjType{
        ConfigType: objtype.ConfigType{ID: 1511, DebugName: "members_item"},
        Members:    true,
    }
    if s.invs == nil {
        s.invs = make(map[int]*inventory.Inventory)
    }
    inv := inventory.New(93, 28, inventory.StackNormal)
    inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
    s.invs[93] = inv
    p.invListenOnCom(93, 149, -1)

    if err := handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149)); err != nil {
        t.Fatalf("handleOpLocU: %v", err)
    }
    if p.target != loc {
        t.Errorf("target: got %v, want loc (members world should allow)", p.target)
    }
}
```

Same 2 tests for OpNpcU (with `makeOpNpcFixture` + `p2x4NpcUPayload` instead).

## 8. Deviations

| ID | Status |
|---|---|
| **S6m-D4** | **✅ CLOSED in S6z** |
| **S6o-D4** | **✅ CLOSED in S6z** |

No new deviations.

## 9. Scope

- Impl: ~20 LOC (2 gate blocks + 2 comment updates)
- Tests: ~100 LOC (4 new tests)
- 1 commit, ~120 LOC total
