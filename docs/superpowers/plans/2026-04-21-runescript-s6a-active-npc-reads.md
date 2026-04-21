# RuneScript S6a: Active NPC Reads + VARN Real Impl Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Promote `ActiveNpc` to a real 9-method interface, add `ScriptState.ActiveNpc`, register 8 NPC read handlers, replace VARN stubs with real impls, add per-Npc `uid` + `varns` storage with `*Npc` impls.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6a-active-npc-reads-design.md`](../specs/2026-04-21-runescript-s6a-active-npc-reads-design.md)

---

## Task 1: ActiveNpc interface + ScriptState.ActiveNpc field + mockNpc fixture

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `pkg/script/state.go`
- Modify or Create test fixture: `pkg/script/runner_test.go` (mockNpc helper, OR create one in handlers_npc_test.go in Task 2)

- [ ] **Step 1: Replace `ActiveNpc` stub** in `pkg/script/active.go`. Find:

```go
type ActiveNpc interface{}
```

Replace with:

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

Leave `ActiveLoc` and `ActiveObj` as stubs.

- [ ] **Step 2: Add `ActiveNpc` field to ScriptState** in `pkg/script/state.go`. Find the `ScriptState` struct and add (next to `Self ActivePlayer`):

```go
// ActiveNpc is the NPC that NPC_* and VARN ops target. Nil if no
// NPC is bound to this script's execution. Set by callers (test
// fixtures, OPNPC trigger routing in a future sub-spec).
ActiveNpc ActiveNpc
```

- [ ] **Step 3: Build pkg/script + run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Both should pass. The interface change doesn't break existing callers (no concrete `ActiveNpc` implementer existed yet — *Npc gets one in Task 3).

- [ ] **Step 4: Commit**

```bash
git add pkg/script/active.go pkg/script/state.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S6a ActiveNpc interface + ScriptState.ActiveNpc

Promotes ActiveNpc from a stub interface to a real 9-method surface
(NpcType/X/Z/Level/Stat/BaseStat/Category/UID + NpcVarN/SetNpcVarN).
Adds ScriptState.ActiveNpc field set by callers before Execute.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report**: commit SHA + green build/test status.

---

## Task 2: 8 NPC handlers + VARN replacement + tests

**Files:**
- Create: `pkg/script/handlers_npc.go`
- Create: `pkg/script/handlers_npc_test.go` (with mockNpc + 10+ tests)
- Modify: `pkg/script/handlers_vars.go` (replace stubs)
- Modify: `pkg/script/handlers_vars_test.go` (extend tests)
- Modify: `pkg/script/handlers.go` (register 8)

- [ ] **Step 1: Verify opcode constants** exist in `pkg/script/opcode.go`:
- OpNpcType, OpNpcCoord, OpNpcStat, OpNpcBaseStat, OpNpcName, OpNpcHasOp, OpNpcUID, OpNpcCategory.

- [ ] **Step 2: Verify the `Op` field on `*objtype.NpcType`** — grep `pkg/objtype/npctype.go`. Should be `Op []string` per S5d. Adapt if different.

- [ ] **Step 3: Read TS NpcOps.ts pop orders** for NPC_HASOP / NPC_STAT / NPC_BASESTAT. Each pops a single int (op id or stat id).

- [ ] **Step 4: Create `pkg/script/handlers_npc.go`** with the 8 handlers + a small `requireActiveNpc` helper. Spec §3 has the exact code.

- [ ] **Step 5: Create `pkg/script/handlers_npc_test.go`** including a `mockNpc` fixture struct (spec §6) and unit tests:

  - `TestNpcType` — preset mockNpc.typeID=42, run NPC_TYPE script, assert PopInt == 42.
  - `TestNpcCoord` — preset mockNpc.x=3222, z=3222, level=0, assert PopInt == packed value.
  - `TestNpcStatHP` — preset curHP=99, run NPC_STAT with stat id 0, assert 99.
  - `TestNpcStatOtherReturnsZero` — same but stat id 5, assert 0.
  - `TestNpcBaseStat` — analog with baseHP.
  - `TestNpcUID` — assert preset UID is pushed.
  - `TestNpcCategory` — extend mockConfigs (already in `handlers_config_test.go`'s setup) with a NpcType at id 7, category 99; preset mockNpc.typeID=7; assert NPC_CATEGORY pushes 99.
  - `TestNpcName` — extend mockConfigs with NpcType.Name="Hans"; assert NPC_NAME pushes "Hans".
  - `TestNpcHasOpExisting` — NpcType.Op=[“Talk-to”, "", ""]; NPC_HASOP(1) pushes 1.
  - `TestNpcHasOpMissing` — same NpcType, NPC_HASOP(2) pushes 0.
  - `TestNpcOpsRequireActiveNpc` — table-driven over all 8 opcodes, asserts each returns error when ActiveNpc is nil.

  Use Edit/Write tool, NOT bash heredoc, to avoid `!=` corruption.

  Mockconfigs hookup: handlers_config_test.go has the existing `mockConfigs` struct with `npcs map[int]*objtype.NpcType`. Use that.

- [ ] **Step 6: Replace VARN stubs in `pkg/script/handlers_vars.go`**:

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

- [ ] **Step 7: Update VARN tests in `pkg/script/handlers_vars_test.go`** — add or update:
  - `TestPushVarnReadsActiveNpc` — preset mockNpc.varns[5] = 42, run PUSH_VARN with operand 5, assert 42.
  - `TestPopVarnWritesActiveNpc` — run POP_VARN with operand 7, value 99, assert mockNpc.varns[7] == 99.
  - `TestVarnRequireActiveNpc` — both PUSH_VARN and POP_VARN return error when ActiveNpc is nil. (This may CHANGE existing test behavior — old TestVarnStubs likely expected silent success. Update it to expect errors instead.)

- [ ] **Step 8: Register 8 handlers in `pkg/script/handlers.go`** at end of map with `// S6a: NPC reads.` block.

- [ ] **Step 9: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Must pass.

- [ ] **Step 10: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go \
        pkg/script/handlers_vars.go pkg/script/handlers_vars_test.go \
        pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S6a NPC read handlers + VARN real impls

8 instance-read opcodes (NPC_TYPE, NPC_COORD, NPC_STAT, NPC_BASESTAT,
NPC_NAME, NPC_HASOP, NPC_UID, NPC_CATEGORY). All gate on
ActiveNpc != nil. NPC_NAME / NPC_CATEGORY / NPC_HASOP look up the
NpcType via Configs (S5d's hook).

VARN stubs replaced with real handlers that route through
ActiveNpc.NpcVarN / SetNpcVarN — no longer silent no-ops.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Do NOT touch modules/world** — Task 3 handles that.

---

## Task 3: Npc impls + uid/varns + E2E

**Files:**
- Modify: `modules/world/npc.go`
- Create: `modules/world/npc_script.go`
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Add `uid` and `varns` fields to `Npc` struct** in `modules/world/npc.go`:

```go
type Npc struct {
    // ... existing fields ...
    uid   int       // (typeId << 16) | nid; computed in NewNpc
    varns []int32   // per-NPC vars; nil until first SetNpcVarN
}
```

- [ ] **Step 2: Initialize `uid` in `NewNpc`**. Find the constructor; add at the end:

```go
n.uid = (typeId << 16) | nid
```

If the constructor signature differs from `NewNpc(nid, typeID, x, z, level, typ)` — adapt. Read the existing function to find param names.

- [ ] **Step 3: Create `modules/world/npc_script.go`** with the 9 ActiveNpc method impls (spec §5).

Verify the `*Npc` field names match what you reference (`typeId`, `x`, `z`, `level`, `curHP`, `baseHP`, `typ`, `nid`).

- [ ] **Step 4: Full build + tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Must pass.

- [ ] **Step 5: Add E2E test** in `modules/world/script_test.go` via Edit tool (no heredoc):

```go
func TestNpcNameViaScript(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}

    // Seed an NpcType at id 7 named "Hans".
    s.npcTypes = &objtype.NPCTypeConfigs{
        Configs: make([]*objtype.NpcType, 8),
    }
    s.npcTypes.Configs[7] = &objtype.NpcType{
        ConfigType: objtype.ConfigType{ID: 7, DebugName: "hans"},
        Name:       "Hans",
    }

    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

    // Build an Npc instance of type 7. Adapt to actual NewNpc signature.
    npc := NewNpc(0 /* nid */, 7 /* typeID */, 3222, 3222, 0, s.npcTypes.Configs[7])

    // Script: npc_name → mes → return.
    sf := &script.ScriptFile{
        Name: "[npcname,test]",
        Opcodes: []script.Opcode{
            script.OpNpcName,
            script.OpMes,
            script.OpReturn,
        },
        IntOperands:      []int32{0, 0, 0},
        StringOperands:   []string{"", "", ""},
        InstructionCount: 3,
    }

    received := drainConn(t, cc)

    // Inline runScript steps so we can set ActiveNpc.
    state := script.Init(sf, p, false, nil, nil)
    state.Provider = s.scriptProvider
    state.World = s.worldVars
    state.Configs = s.configsView
    state.Inv = s.invLookup
    state.ActiveNpc = npc
    if err := script.Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    p.client.flushWrite()
    got := <-received

    // Wire = opcode(1) + len(1) + PJStrLF("Hans") = 7 bytes
    if len(got) != 7 {
        t.Fatalf("wire: got %d bytes, want 7", len(got))
    }
    if string(got[2:6]) != "Hans" || got[6] != 0x0a {
        t.Errorf("payload: got %q, want 'Hans\\n'", got[2:])
    }
}
```

Adapt the `NewNpc` call to match the actual constructor signature (read `modules/world/npc.go`).

- [ ] **Step 6: Run + race + vet + handler count**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcName -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go
```

Handler count: **193** (185 + 8).

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc.go modules/world/npc_script.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Npc impls for ActiveNpc + uid + varns

Adds uid (computed as (typeId<<16)|nid in NewNpc) and varns []int32
fields. *Npc satisfies script.ActiveNpc with 9 methods: type/x/z/level
reads from existing fields, NpcStat(0)/NpcBaseStat(0) maps to curHP/
baseHP (other stat ids return 0 with TODO), NpcVarN/SetNpcVarN backs
to varns slice (lazy-grown, 1024-cap).

E2E TestNpcNameViaScript: build a real Npc, set state.ActiveNpc, run
NPC_NAME → MES — assert "Hans" reaches the wire.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

- [ ] `go build ./...` clean
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` clean
- [ ] `go vet ./...` clean
- [ ] Handler count = 193
