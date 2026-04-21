# Sub-spec RuneScript S6a: Active NPC Reads + VARN Real Impl — Design

**Status:** Draft → ready for plan
**Scope:** Promote `ActiveNpc` from stub interface to real surface with 9 methods. Add `ScriptState.ActiveNpc` field. 8 new NPC instance-read handlers (NPC_TYPE, NPC_COORD, NPC_STAT, NPC_BASESTAT, NPC_NAME, NPC_HASOP, NPC_UID, NPC_CATEGORY). Replace VARN stub handlers with real impls that route through ActiveNpc. Add per-Npc `varns` storage + `uid` field. Add stub mockNpc fixture for tests.
**Out of scope:** OPNPC trigger routing (S6b — player-clicks-NPC chain). NPC_FIND* lookups (need world-wide NPC search; defer until OPNPC routing is in place). Mutating ops (NPC_SAY, NPC_ANIM, NPC_DAMAGE, NPC_SETTARGET, etc.). Per-NPC stat array beyond HP — for S6a `NpcStat(0)` returns `curHP` and other stat ids return 0 with a TODO comment. VarNpcType cache loader — flat fixed-size `varns [1024]int32` (or dynamic) stand-in for now.

---

## Goal

After S6a:

- Scripts that have `state.ActiveNpc` set (manually in tests; eventually by S6b's OPNPC routing) can read NPC instance state: type, coord, current/base stats (HP only for now), name, op-list membership, UID, category.
- VARN reads/writes are no longer stubs — they route to the active NPC's `varns` slice.
- Demo: a synthetic test that constructs a real Npc, sets `state.ActiveNpc = npc`, runs a script `[NPC_NAME, MES, RETURN]`, and observes the NPC's name on the wire.

## Architecture

```
pkg/script/
├── active.go             ActiveNpc interface gains 9 methods
├── state.go              + ActiveNpc ActiveNpc field
├── handlers_vars.go      replace VARN stubs with real impls
├── handlers_npc.go       (new) 8 read handlers
├── handlers_npc_test.go  (new) unit tests + mockNpc fixture
└── handlers.go           + 8 map entries

modules/world/
├── npc.go                + uid int + varns []int32 fields
├── npc_script.go         (new) ActiveNpc impls on *Npc
└── script_test.go        + E2E NPC_NAME read with manually-set ActiveNpc
```

## Components

### 1. `ActiveNpc` interface — `pkg/script/active.go`

Replace the existing stub:
```go
type ActiveNpc interface{}
```

With:
```go
// ActiveNpc is the per-NPC surface that NPC_* opcodes and VARN
// handlers read/write. Set on ScriptState before Execute by callers
// that target a specific NPC (test fixtures, OPNPC routing, etc.).
type ActiveNpc interface {
    NpcType() int                 // returns NpcType.id
    NpcX() int
    NpcZ() int
    NpcLevel() int
    NpcStat(stat int) int         // current (boosted) level — S6a: only HP (id 0) is real
    NpcBaseStat(stat int) int     // base level — S6a: only HP (id 0) is real
    NpcCategory() int
    NpcUID() int                  // (typeId << 16) | nid
    NpcVarN(id int) int32
    SetNpcVarN(id int, val int32)
}
```

Also add an accessor for NPC name. Two options:
- **(a)** `NpcName() string` on the interface — clean, but couples interface to type config.
- **(b)** Handler resolves via `s.Configs.NpcType(s.Self.NpcType()).Name`.

Pick **(b)** — keeps ActiveNpc lean, reuses S5d's Configs surface. NPC_NAME handler does the lookup.

### 2. `ScriptState.ActiveNpc` — `pkg/script/state.go`

Add to the struct (next to `Self ActivePlayer`):
```go
// ActiveNpc is the NPC that NPC_* and VARN ops target. Nil if no NPC
// is bound to this script's execution. Set by callers (e.g. test
// fixtures, OPNPC trigger routing in a future sub-spec).
ActiveNpc ActiveNpc
```

### 3. NPC handlers — `pkg/script/handlers_npc.go`

```go
package script

import (
    "errors"
    "fmt"
)

func requireActiveNpc(s *ScriptState, op string) error {
    if s.ActiveNpc == nil {
        return fmt.Errorf("%s: no active npc", op)
    }
    return nil
}

func handleNpcType(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_TYPE"); err != nil {
        return err
    }
    s.PushInt(s.ActiveNpc.NpcType())
    return nil
}

func handleNpcCoord(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_COORD"); err != nil {
        return err
    }
    n := s.ActiveNpc
    s.PushInt((n.NpcLevel() << 28) | (n.NpcX() << 14) | n.NpcZ())
    return nil
}

func handleNpcStat(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_STAT"); err != nil {
        return err
    }
    stat := s.PopInt()
    s.PushInt(s.ActiveNpc.NpcStat(stat))
    return nil
}

func handleNpcBaseStat(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_BASESTAT"); err != nil {
        return err
    }
    stat := s.PopInt()
    s.PushInt(s.ActiveNpc.NpcBaseStat(stat))
    return nil
}

func handleNpcName(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_NAME"); err != nil {
        return err
    }
    if s.Configs == nil {
        return errors.New("NPC_NAME: no configs")
    }
    cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
    if cfg == nil {
        s.PushString("null")
        return nil
    }
    name := cfg.Name
    if name == "" {
        name = cfg.DebugName
    }
    if name == "" {
        name = "null"
    }
    s.PushString(name)
    return nil
}

func handleNpcHasOp(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_HASOP"); err != nil {
        return err
    }
    op := s.PopInt()
    if s.Configs == nil {
        s.PushInt(0)
        return nil
    }
    cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
    if cfg == nil {
        s.PushInt(0)
        return nil
    }
    // op is 1-indexed per TS.
    idx := op - 1
    if idx < 0 || idx >= len(cfg.Op) || cfg.Op[idx] == "" {
        s.PushInt(0)
    } else {
        s.PushInt(1)
    }
    return nil
}

func handleNpcUID(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_UID"); err != nil {
        return err
    }
    s.PushInt(s.ActiveNpc.NpcUID())
    return nil
}

func handleNpcCategory(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_CATEGORY"); err != nil {
        return err
    }
    if s.Configs == nil {
        s.PushInt(-1)
        return nil
    }
    cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
    if cfg == nil {
        s.PushInt(-1)
        return nil
    }
    s.PushInt(cfg.Category)
    return nil
}
```

Verify the actual `Op` field name on `*objtype.NpcType` by reading `pkg/objtype/npctype.go`. The S5d work used `Op` per the report; confirm.

### 4. VARN replacement — `pkg/script/handlers_vars.go`

Replace existing stubs:
```go
func handlePushVarn(s *ScriptState) error {
    if s.ActiveNpc == nil {
        return errors.New("PUSH_VARN: no active npc")
    }
    s.PushInt(int(s.ActiveNpc.NpcVarN(varOperandID(s))))
    return nil
}

func handlePopVarn(s *ScriptState) error {
    if s.ActiveNpc == nil {
        return errors.New("POP_VARN: no active npc")
    }
    val := int32(s.PopInt())
    s.ActiveNpc.SetNpcVarN(varOperandID(s), val)
    return nil
}
```

The `varOperandID` helper exists from S5b.

### 5. Npc impl — `modules/world/npc.go` field additions + `modules/world/npc_script.go`

Field additions:
```go
type Npc struct {
    // ... existing ...
    uid   int       // (typeId << 16) | nid; computed on construction
    varns []int32   // per-NPC variables; nil until first SetNpcVarN
}
```

Set `uid` in `NewNpc`:
```go
n.uid = (typeId << 16) | nid
```

ActiveNpc impls in new file `modules/world/npc_script.go`:

```go
package world

func (n *Npc) NpcType() int    { return n.typeId }
func (n *Npc) NpcX() int       { return n.x }
func (n *Npc) NpcZ() int       { return n.z }
func (n *Npc) NpcLevel() int   { return n.level }
func (n *Npc) NpcUID() int     { return n.uid }
func (n *Npc) NpcCategory() int {
    if n.typ == nil {
        return -1
    }
    return n.typ.Category
}

// NpcStat returns the current level for the given stat id. S6a maps
// stat id 0 (HITPOINTS) to curHP; other stat ids return 0. Real
// per-stat storage lands with NPC combat (S6c).
func (n *Npc) NpcStat(stat int) int {
    if stat == 0 {
        return n.curHP
    }
    return 0
}

// NpcBaseStat is the base-level analogue. Stat 0 → baseHP.
func (n *Npc) NpcBaseStat(stat int) int {
    if stat == 0 {
        return n.baseHP
    }
    return 0
}

// NpcVarN returns the per-NPC int variable at id. Returns 0 on OOB.
func (n *Npc) NpcVarN(id int) int32 {
    if id < 0 || id >= len(n.varns) {
        return 0
    }
    return n.varns[id]
}

// SetNpcVarN writes the per-NPC int variable at id, growing the slice
// if needed (capped at 1024 to bound runaway scripts).
func (n *Npc) SetNpcVarN(id int, val int32) {
    if id < 0 {
        return
    }
    const npcVarnCap = 1024
    if id >= npcVarnCap {
        return
    }
    if id >= len(n.varns) {
        next := make([]int32, id+1)
        copy(next, n.varns)
        n.varns = next
    }
    n.varns[id] = val
}
```

### 6. mockNpc fixture for unit tests — `pkg/script/handlers_npc_test.go`

```go
type mockNpc struct {
    typeID, x, z, level, uid, category int
    curHP, baseHP                       int
    name                                string // ignored — handler uses Configs
    varns                               map[int]int32
}

func (m *mockNpc) NpcType() int    { return m.typeID }
func (m *mockNpc) NpcX() int       { return m.x }
func (m *mockNpc) NpcZ() int       { return m.z }
func (m *mockNpc) NpcLevel() int   { return m.level }
func (m *mockNpc) NpcUID() int     { return m.uid }
func (m *mockNpc) NpcCategory() int { return m.category }
func (m *mockNpc) NpcStat(stat int) int {
    if stat == 0 { return m.curHP }
    return 0
}
func (m *mockNpc) NpcBaseStat(stat int) int {
    if stat == 0 { return m.baseHP }
    return 0
}
func (m *mockNpc) NpcVarN(id int) int32 {
    if m.varns == nil { return 0 }
    return m.varns[id]
}
func (m *mockNpc) SetNpcVarN(id int, val int32) {
    if m.varns == nil { m.varns = make(map[int]int32) }
    m.varns[id] = val
}
```

### 7. Tests

**Unit tests** (`pkg/script/handlers_npc_test.go`):
- TestNpcType, TestNpcCoord, TestNpcStatHP (id 0), TestNpcStatOtherReturnsZero (id 5), TestNpcBaseStat, TestNpcUID, TestNpcCategory.
- TestNpcName: extends `mockConfigs` with an `NpcType` at id 7 named "Hans". Run `[push 7?, npc_type, npc_name, return]` — but wait, NPC_TYPE pushes from ActiveNpc, doesn't pop. So just preset mockNpc.typeID=7 and mockConfigs.npcs[7] = NpcType with Name="Hans".
- TestNpcHasOpExisting / TestNpcHasOpMissing.
- TestPushVarnReadsActiveNpc / TestPopVarnWritesActiveNpc.
- TestNpcOpsRequireActiveNpc (table-driven negatives).

**E2E** (`modules/world/script_test.go`):
- `TestNpcNameViaScript`: build a real `*Npc` with typeID pointing to a seeded NpcType; run a script that calls NPC_NAME + MES; assert wire bytes contain the name.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/script/active.go` (diff) | +20 |
| `pkg/script/state.go` (diff) | +5 |
| `pkg/script/handlers_npc.go` | ~150 |
| `pkg/script/handlers_npc_test.go` | ~300 (incl. mockNpc) |
| `pkg/script/handlers_vars.go` (diff) | +14 |
| `pkg/script/handlers_vars_test.go` (diff) | +50 |
| `pkg/script/handlers.go` (diff) | +12 |
| `modules/world/npc.go` (diff) | +5 |
| `modules/world/npc_script.go` | ~70 |
| `modules/world/script_test.go` (diff) | +60 |
| **Total** | **~690** |

## Key design calls

- **NpcName via Configs lookup, not interface method.** Keeps ActiveNpc lean and reuses S5d's loaded NpcType configs.
- **NpcStat(0) == curHP, others == 0** — pragmatic minimal mapping. NPCs only track HP per-instance for now; combat stat boosts come with S6c.
- **`varns []int32` grows lazily up to a 1024 cap.** Most NPCs will never use varns, so default-nil keeps memory cheap. Cap prevents runaway scripts.
- **`uid = (typeId << 16) | nid`** matches TS exactly. nid is the slot index; typeId is the NPC type. Two NPCs of the same type at different slots get distinct UIDs.
- **VARN secondary bit ignored** (low 16 bits only) — same as VARP in S5b.
- **No npc_find / OPNPC routing.** Tests manually set state.ActiveNpc; real cache scripts will need S6b for triggering. Document.

## Gotchas

- **Pop order for NPC_HASOP / NPC_STAT / NPC_BASESTAT**: each pops one int (op id or stat id). Verify against TS NpcOps.ts during impl.
- **NpcType.Op vs Ops field name**: confirmed as `Op` in S5d work; verify by greppping `pkg/objtype/npctype.go`.
- **Existing Npc ctor**: read `func NewNpc(...)` in `modules/world/npc.go` to confirm signature; add the `uid` assignment alongside other init.
- **The `Configs` interface hook on ScriptState exists from S5d**. NPC_NAME and NPC_CATEGORY rely on it being set (which `runScript` already does).
- **Heredoc `!=` bug**: use Edit/Write for test code.
