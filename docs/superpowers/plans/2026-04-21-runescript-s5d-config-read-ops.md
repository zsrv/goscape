# RuneScript S5d: Config-Read Opcodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register 33 config-read opcode handlers (EnumOps×2 + StructOps×1 + LocConfigOps×7 + NpcConfigOps×8 + ObjConfigOps×15) backed by three new cache loaders (EnumType, StructType, LocType) and a shared `paramLookup` helper for the four `*_PARAM` variants.

**Architecture:** Three loaders in `pkg/objtype/` follow the existing pattern. One handler file (`pkg/script/handlers_config.go`) hosts all 33 handlers plus a `Configs` interface on `ScriptState` (added at `pkg/script/configs.go`). Server implements `Configs` via a thin `serverConfigsView`. Zero wire traffic — reads only.

**Tech Stack:** Go 1.22+, existing objtype loader pattern, existing ConfigType machinery.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5d-config-read-ops-design.md`](../specs/2026-04-21-runescript-s5d-config-read-ops-design.md)

---

## Task 1: Config loaders (EnumType, StructType, LocType)

**Files:**
- Create: `pkg/objtype/enumtype.go`, `enumtype_test.go`
- Create: `pkg/objtype/structtype.go`, `structtype_test.go`
- Create: `pkg/objtype/loctype.go`, `loctype_test.go`

- [ ] **Step 1: Investigate TS source to get exact decode codes**

Read these three TS files to find the `decode(code, dat)` switch for each type:
- `$HOME/Code/github.com/LostCityRS/Engine-TS/src/cache/config/EnumType.ts`
- `$HOME/Code/github.com/LostCityRS/Engine-TS/src/cache/config/StructType.ts`
- `$HOME/Code/github.com/LostCityRS/Engine-TS/src/cache/config/LocType.ts` (if it exists in the cache/config dir) — LocType might split server/client like ObjType. **Load only server-side fields.** Skip any codes that read client-side data (model, icon, etc.).

Extract the exact code numbers + wire format per field. The spec's field lists are targets but the real source is the TS decoder.

- [ ] **Step 2: Write `pkg/objtype/enumtype.go`**

Follow the exact pattern of `pkg/objtype/varptype.go` (which you can read for reference). Structure:
- `type EnumType struct { ConfigType; InputType ScriptVarType; OutputType ScriptVarType; DefaultInt int32; DefaultString string; Values map[int32]any }`
- `Decode(code, dat)` switch handling all codes TS handles, including the `values` map entries.
- `NewEnumType(id int) *EnumType` with sensible zero values.
- `type EnumTypeConfigs struct { ConfigNames map[string]int; Configs []*EnumType }`
- `LoadEnumTypes(dir string) (*EnumTypeConfigs, error)` reads `data/pack/server/enum.dat`.
- `parseEnumTypes(dat *packet.Packet) (*EnumTypeConfigs, error)` follows count loop pattern.

**IMPORTANT for Values map**: entries are encoded as (inputKey, output) pairs. The output type depends on `OutputType`: STRING values via `dat.GJStrLF()`, INT values via `dat.G4()`. Dispatch at decode time.

- [ ] **Step 3: Write `pkg/objtype/structtype.go`**

Much simpler — StructType has only `Params ParamMap` and `DebugName`. Code 249 decodes Params (`ot.Params = DecodeParams(dat)`), code 250 decodes DebugName. Other codes return `fmt.Errorf("unrecognized struct config code %d", code)`.

- [ ] **Step 4: Write `pkg/objtype/loctype.go`**

Server-only fields per spec: Category, Desc, Width, Length, Params, DebugName. Match the TS code numbers for those fields ONLY — unknown codes silently consume their bytes (NOT error), because loc.dat has many codes from mixed server+client and we skip client.

**Exact rule**: if a code you don't recognize comes up, return nil — do NOT error. Add a comment noting this departure from the norm. Alternative: error on unknown codes and fail — check what loc.dat actually contains by loading the real file during test to see which codes appear.

- [ ] **Step 5: Write round-trip tests for each loader**

Follow the exact pattern of `pkg/objtype/varptype_test.go`:
- Build a synthetic `.dat` blob byte-by-byte using `packet.NewPacket`.
- Parse it via the internal `parseXxxTypes`.
- Assert field values, names, config count.

Three test files, one per type. Tests should NOT read the real cache — use synthetic blobs so they're hermetic.

- [ ] **Step 6: Build + test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/objtype/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add pkg/objtype/enumtype.go pkg/objtype/enumtype_test.go \
        pkg/objtype/structtype.go pkg/objtype/structtype_test.go \
        pkg/objtype/loctype.go pkg/objtype/loctype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): EnumType + StructType + LocType cache loaders

Decodes data/pack/server/{enum,struct,loc}.dat following the existing
ConfigType / DecodeType pattern. EnumType carries input/output type,
defaults, and a values map with int/string dispatch. StructType wraps
just params + debugname. LocType is server-side subset only (category,
desc, width, length, params, debugname) — unknown codes are skipped
silently to tolerate mixed server+client fields.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `Configs` interface + `ScriptState.Configs` hook

**Files:**
- Create: `pkg/script/configs.go`
- Modify: `pkg/script/state.go`

- [ ] **Step 1: Create `pkg/script/configs.go`**

```go
package script

import "github.com/zsrv/goscape/pkg/objtype"

// Configs is the config-type lookup surface for config-read opcodes
// (OC_*, NC_*, LC_*, ENUM, STRUCT_PARAM). Implementations return nil
// when the type isn't loaded or the id is out of range.
type Configs interface {
    ObjType(id int) *objtype.ObjType
    NpcType(id int) *objtype.NPCType
    LocType(id int) *objtype.LocType
    EnumType(id int) *objtype.EnumType
    StructType(id int) *objtype.StructType
    ParamType(id int) *objtype.ParamType
}
```

- [ ] **Step 2: Add the field to ScriptState**

In `pkg/script/state.go`, add next to the `World WorldVars` field:

```go
// Configs is the config lookup surface. Callers set this after Init
// if the script uses config-read opcodes (OC_*, NC_*, LC_*, ENUM,
// STRUCT_PARAM).
Configs Configs
```

- [ ] **Step 3: Build to confirm**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
```

- [ ] **Step 4: Commit**

```bash
git add pkg/script/configs.go pkg/script/state.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): Configs interface + ScriptState.Configs hook for S5d

Six-method interface (ObjType/NpcType/LocType/EnumType/StructType/
ParamType) exposes the hosting server's cache lookups to config-read
handlers without coupling pkg/script to modules/world.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: 33 handlers + paramLookup helper + tests

**Files:**
- Create: `pkg/script/handlers_config.go`
- Create: `pkg/script/handlers_config_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Study the 5 TS handler files to get exact pops + formulas**

- `/Engine-TS/src/engine/script/handlers/EnumOps.ts`
- `/Engine-TS/src/engine/script/handlers/StructOps.ts`
- `/Engine-TS/src/engine/script/handlers/LocConfigOps.ts`
- `/Engine-TS/src/engine/script/handlers/NpcConfigOps.ts`
- `/Engine-TS/src/engine/script/handlers/ObjConfigOps.ts`

For each handler in the 33-opcode list (see spec §1), note: exact pop order, exact field name accessed, any special logic (NC_OP's 1-based index; OC_CERT's cert-chain traversal).

- [ ] **Step 2: Write the 33 handlers in `pkg/script/handlers_config.go`**

Layout: one handler per function, grouped by category with comment blocks. Use helpers:

```go
// paramLookup is the shared path for *_PARAM opcodes. Reads the param
// value from the config's params map, falling back to ParamType defaults.
// Dispatches int vs string push based on ParamType.Type.
func paramLookup(s *ScriptState, params objtype.ParamMap, paramID int) error { ... }
```

Every handler validates its config pointer is non-nil; returns `fmt.Errorf` with the opcode name on failure.

For OC_CERT / OC_UNCERT: if `ObjType.Certlink` / `ObjType.Certtemplate` fields don't exist in Go's ObjType, you have two choices:
(a) Add them to ObjType + decode them in its Decode switch, OR
(b) Stub the handlers to push the input id unchanged with a `slog.Debug` flag.

Pick (b) if adding ObjType fields would expand scope significantly. Document either way.

For NC_OP: pop `(npcID, op)` where op is 1-based. Push `npc.Op[op-1]` if in range, else empty string. Check the Go NpcType for the `Op` field name (might be `op` / `Op` / an array literal — adapt).

- [ ] **Step 3: Register all 33 handlers in `pkg/script/handlers.go`**

Add a block `// S5d: config-read ops (enum/struct/loc/npc/obj).` at the end of the map. Organize entries by category with sub-comments for skimability.

- [ ] **Step 4: Write `pkg/script/handlers_config_test.go`**

Create a `mockConfigs` struct implementing the `Configs` interface. Seed it with 2-3 ObjType / NpcType / LocType / EnumType / StructType entries covering:
- A named ObjType at id 995 with `Name: "Coins"`, `Cost: 1`, `Stackable: true`, a params map entry, `Members: false`.
- An NpcType at id 0 with `Name: "man"`, size 1, vislevel 2, an Op list `[“Talk-to”, “”, ""]`.
- A LocType at id 0 with `Name: "Door"`, width 1, length 2, category 1.
- An EnumType at id 0 with InputType=INT, OutputType=STRING, Values `{1: "one", 2: "two"}`, defaults.
- A StructType at id 0 with a param map entry.
- A ParamType at paramid 1 (INT type, default 0) and at paramid 2 (STRING type, default "").

For each handler, a test that pushes the expected args via a one-instruction script, runs Execute, asserts the popped result. Cover int vs string param dispatch.

Aim for 40+ sub-tests total.

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestConfig|TestOc|TestNc|TestLc|TestEnum|TestStruct' -v
```

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_config.go pkg/script/handlers_config_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5d config-read opcodes (33 handlers)

EnumOps (2): ENUM, ENUM_GETOUTPUTCOUNT — int/string dispatch via
EnumType.OutputType.
StructOps (1): STRUCT_PARAM — delegates to paramLookup helper.
LocConfigOps (7): LC_NAME, LC_PARAM, LC_CATEGORY, LC_DESC,
LC_DEBUGNAME, LC_WIDTH, LC_LENGTH.
NpcConfigOps (8): NC_NAME, NC_PARAM, NC_CATEGORY, NC_DESC,
NC_DEBUGNAME, NC_OP (1-based), NC_SIZE, NC_VISLEVEL.
ObjConfigOps (15): OC_NAME, OC_PARAM, OC_CATEGORY, OC_DESC, OC_MEMBERS,
OC_WEIGHT, OC_WEARPOS{,2,3}, OC_COST, OC_TRADEABLE, OC_DEBUGNAME,
OC_CERT, OC_UNCERT, OC_STACKABLE.

Shared paramLookup helper absorbs OC_PARAM/NC_PARAM/LC_PARAM/
STRUCT_PARAM duplication; int/string push dispatched by ParamType.Type.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Server integration + E2E test

**Files:**
- Modify: `modules/world/server.go`
- Create: `modules/world/server_configs.go`
- Modify: `modules/world/script.go`
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Add fields to `Server` struct**

```go
enumTypes   *objtype.EnumTypeConfigs
structTypes *objtype.StructTypeConfigs
locTypes    *objtype.LocTypeConfigs
configsView serverConfigsView
```

- [ ] **Step 2: Load the three new config types in `NewServer`**

After the existing `varsTypes` load:

```go
enumTypes, err := objtype.LoadEnumTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load enum types: %w", err)
}
structTypes, err := objtype.LoadStructTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load struct types: %w", err)
}
locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load loc types: %w", err)
}
s.enumTypes = enumTypes
s.structTypes = structTypes
s.locTypes = locTypes
s.configsView = serverConfigsView{s: s}
```

- [ ] **Step 3: Create `modules/world/server_configs.go`**

```go
package world

import "github.com/zsrv/goscape/pkg/objtype"

type serverConfigsView struct{ s *Server }

func (c serverConfigsView) ObjType(id int) *objtype.ObjType {
    if c.s == nil || c.s.objTypes == nil { return nil }
    if id < 0 || id >= len(c.s.objTypes.Configs) { return nil }
    return c.s.objTypes.Configs[id]
}
// ... repeat for NpcType, LocType, EnumType, StructType, ParamType
```

Six nearly-identical methods. Each guards nil config store + OOB id.

- [ ] **Step 4: Wire `state.Configs` in `runScript`**

In `modules/world/script.go`, add next to existing `state.World = s.worldVars`:

```go
state.Configs = s.configsView
```

- [ ] **Step 5: Add E2E test in `modules/world/script_test.go`**

`TestOcNameViaScript`: seed `s.objTypes` with a minimal `ObjType` at id 995 named "Coins". Run a script `push_constant_int 995, oc_name, return`. Assert `state.PopString() == "Coins"`. Use Edit tool (avoid `!=` heredoc bug).

**Note**: if the real cache loads 11,000+ ObjTypes at server init, don't replace them — just verify the lookup works against the real cache. Actually — simpler: make the test replace `s.objTypes` wholesale with a hand-built `objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{...}}`.

- [ ] **Step 6: Full build + test + race + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Handler count check: `grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go` should read **135** (102 after S5c + 33 S5d).

- [ ] **Step 7: Commit**

```bash
git add modules/world/server.go modules/world/server_configs.go modules/world/script.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Server loads enum/struct/loc types; wires Configs into script

NewServer now loads enum.dat, struct.dat, loc.dat from the cache path.
serverConfigsView exposes all six config lookups (obj/npc/loc/enum/
struct/param) via script.Configs. runScript sets state.Configs so
config-read handlers resolve.

TestOcNameViaScript: one-instruction script oc_name(995) returns "Coins"
against a seeded ObjType.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

After completing all tasks:

- [ ] Full build clean
- [ ] Full tests pass
- [ ] Race-free
- [ ] vet-clean
- [ ] Handler count = 135
- [ ] No new behavior: every handler is a pure read against already-loaded configs
