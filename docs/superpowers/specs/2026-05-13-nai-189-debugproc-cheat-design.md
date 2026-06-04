# NAI-189 — Dev-block DEBUGPROC cheat (`::~name <args…>`)

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:59-148`
**Predecessors:** NAI-183 (cheat infra), NAI-186 (super-mod cohort + `setvar/getvar` ByName), NAI-187 (admin spawn cohort + Loc/Npc/Component ByName cluster), NAI-188 (::speed + carryforward rewrite).
**HEAD at spec-write:** `1f277ac`

## §1 Goal

Port the dev-tier DEBUGPROC dispatch path (TS `ClientCheatHandler.ts:59-148`) into goscape's existing dev block at `modules/world/handlers_game.go:402`. Mutates nothing in the world by itself — its sole responsibility is to resolve a `[debugproc,X]` runescript by name, parse positional arguments per the script's declared `ParamTypes`, and dispatch it via the existing `Server.runScript` entry.

This is the last unported branch of the dev block that is **not** infra-blocked. After NAI-189 closes, `ClientCheatHandler.ts` is 100% ported except for the two-line `reload` / `rebuild` stubs at TS L149-153, which remain correctly framed as a downstream cache/script hot-reload subsystem (the eventual NAI-190+ thread).

The carryforward comment at `modules/world/handlers_game.go:370-389` is incomplete at HEAD — it lists only `reload` and `rebuild` as remaining cheats and silently omits DEBUGPROC. NAI-189 rewrites this comment on close (per memory `tracker_entry_framing_can_be_incomplete` and `tracker_carryforward_listings_compound`).

## §2 Out of scope

| Concern | TS line | Why deferred |
|---|---|---|
| `reload` cheat | `ClientCheatHandler.ts:149-150` | `World.reload()` — full cache hot-reload pipeline. NAI-190+. |
| `rebuild` cheat | `ClientCheatHandler.ts:151-153` | `World.rebuild()` — script-provider hot-reload. NAI-190+. |
| Non-debugproc dev-block cheats (`fly`/`naive`/`random`/`speed`) | TS L168-186, L154-167 | Already ported. NAI-189 must not regress their dispatch. |
| `[command,X]` and `[clientscript,X]` script flavors | not in TS file | DEBUGPROC dispatches only `[debugproc,X]` per TS L61. |
| TS `v8.writeHeapSnapshot` semantics for `::snapshot` | TS L477-480 | Already ported at `handlers_game.go:667` with a `pprof`-equivalent functional analog (NAI-184). Out of scope here. |

## §3 Pre-flight audit

Per memory `controller_preflight` and `risk_register_premise_grep`, every premise below was re-verified against HEAD `1f277ac`.

### §3.1 TS dispatch shape (ClientCheatHandler.ts:59-148)

```text
if (cmd[0] === Environment.NODE_DEBUGPROC_CHAR) {
    const script = ScriptProvider.getByName(`[debugproc,${cmd.slice(1)}]`);
    if (!script) return false;
    const params = new Array(script.info.parameterTypes.length).fill(-1);
    for (let i = 0; i < script.info.parameterTypes.length; i++) {
        const type = script.info.parameterTypes[i];
        try {
            switch (type) {
                case ScriptVarType.STRING: { const v = args.shift(); params[i] = v ?? ''; break; }
                case ScriptVarType.INT:    { const v = args.shift(); params[i] = parseInt(v ?? '0', 10) | 0; break; }
                case ScriptVarType.OBJ:
                case ScriptVarType.NAMEDOBJ: { const n = args.shift(); params[i] = ObjType.getId(n ?? ''); break; }
                case ScriptVarType.NPC:   { const n = args.shift(); params[i] = NpcType.getId(n ?? ''); break; }
                case ScriptVarType.LOC:   { const n = args.shift(); params[i] = LocType.getId(n ?? ''); break; }
                case ScriptVarType.SEQ:   { const n = args.shift(); params[i] = SeqType.getId(n ?? ''); break; }
                case ScriptVarType.STAT:  { const n = args.shift() ?? ''; params[i] = PlayerStatMap.get(n.toUpperCase()); break; }
                case ScriptVarType.INV:   { const n = args.shift(); params[i] = InvType.getId(n ?? ''); break; }
                case ScriptVarType.COORD: {
                    const args2 = cheat.split('_');
                    const level = parseInt(args2[0].slice(6));
                    const mx = parseInt(args2[1]);
                    const mz = parseInt(args2[2]);
                    const lx = parseInt(args2[3]);
                    const lz = parseInt(args2[4]);
                    params[i] = CoordGrid.packCoord(level, (mx << 6) + lx, (mz << 6) + lz);
                    break;
                }
                case ScriptVarType.INTERFACE: { const n = args.shift(); params[i] = Component.getId(n ?? ''); break; }
                case ScriptVarType.SPOTANIM: { const n = args.shift(); params[i] = SpotanimType.getId(n ?? ''); break; }
                case ScriptVarType.IDKIT:    { const n = args.shift(); params[i] = IdkType.getId(n ?? ''); break; }
            }
        } catch (_) { return false; }
    }
    player.executeScript(ScriptRunner.init(script, player, null, params), false);
}
```

Twelve TS arms; the `OBJ` / `NAMEDOBJ` pair share a body. TS lookups use `getId(name)` which returns `-1` on miss; the loop does **not** abort on `-1` — it places `-1` into the params slot and continues (TS L74-139 swallow misses silently). The try/catch is reached only on programmer errors like `null.slice()`, not on missing-name lookups.

### §3.2 goscape primitives at HEAD `1f277ac`

| Concern | Location | Status |
|---|---|---|
| `ScriptProvider.GetByName` | `pkg/script/provider.go:156` | ✓ Returns `nil` on miss. |
| `Server.runScript(sf, self, target, protect, intArgs, stringArgs)` | `modules/world/script.go:92` | ✓ Existing entry. Builds state via `buildPlayerScriptState` + executes via `resumeOrFinish`. |
| `script.Init(sf, self, protect, intArgs, stringArgs)` | `pkg/script/runner.go:12` | ✓ Parallel-slice arg convention. |
| `ObjType.ByName` | `pkg/objtype/objtype.go` | ✓ Returns `*ObjType` or `nil`. |
| `NpcType.ByName` | `pkg/objtype/npctype.go` | ✓ NAI-187. |
| `LocType.ByName` | `pkg/objtype/loctype.go` | ✓ NAI-187. |
| `ComponentType.ByName` | `pkg/objtype/componenttype.go` | ✓ NAI-187. |
| `VarpType.ByName` | `pkg/objtype/varptype.go` | ✓ NAI-186. |
| `objtype.PlayerStatMap` | `pkg/objtype/playerstat.go` | ✓ Already used by `setstat`/`advancestat`. |
| `CoordGrid.PackCoord` | `pkg/coordgrid/coordgrid.go` | ✓ Already used across the codebase. |
| `SeqType.ByName` | `pkg/objtype/seqtype.go` | **MISSING** — `ConfigNames` map populated at `seqtype.go:164-165` but no `ByName` method. |
| `SpotanimType.ByName` | `pkg/objtype/spotanimtype.go` | **MISSING** — `ConfigNames` map populated at `spotanimtype.go:130-131` but no `ByName` method. |
| `IdkType.ByName` | `pkg/objtype/idktype.go` | **MISSING** — `ConfigNames` map populated at `idktype.go:127-128` but no `ByName` method. |
| `InvType.ByName` | `pkg/objtype/invtype.go` | **MISSING** — `ConfigNames` map populated at `invtype.go:127-128` but no `ByName` method. |
| `Server.seqTypes` / `.spotanimTypes` / `.idkTypes` / `.invTypes` | `modules/world/server.go:95, 116, 119, 120` | ✓ All plumbed and loaded at startup. |
| `Server.scriptProvider` | `modules/world/server.go` (used in `script.go:49`) | ✓ Plumbed. |
| `Server.cfg.NodeDebugprocChar` | `modules/world/config.go:15` (default `"~"`) | ✓ Plumbed but currently unread. NAI-189 activates it. |
| `script.ScriptFile.ParamTypes []byte` | `pkg/script/file.go:20` | ✓ Decoded at `file.go:91-93`. Each byte is a `ScriptVarType` value cast to `uint8`. |
| `parseIntOr(s string, def int) int` | `modules/world/handlers_game.go:1011` | ✓ Already used by other cheats; mirrors TS `tryParseInt`. |
| `parts` and `args` split in `handleClientCheat` | `handlers_game.go:362-368` | ✓ `parts[0]` is the lowered cmd; `args` is the post-first-space tail. |

### §3.3 ParamTypes byte → ScriptVarType mapping

`ScriptFile.ParamTypes` stores `byte` values; the `ScriptVarType` enum has values up to 255 (`ScriptVarTypeNpcStat = 254`, `ScriptVarTypeAutoInt = 255`). The compare in DEBUGPROC is `objtype.ScriptVarType(b) == objtype.ScriptVarTypeString` etc. — already the convention in `pkg/script/handlers_db.go:115`, no abstraction needed.

The 12 TS-handled arms map to:

| TS name | Goscape constant | Decimal value |
|---|---|---|
| STRING | `ScriptVarTypeString` | 115 |
| INT | `ScriptVarTypeInt` | 105 |
| OBJ | `ScriptVarTypeObj` | 111 |
| NAMEDOBJ | `ScriptVarTypeNamedObj` | 79 |
| NPC | `ScriptVarTypeNPC` | 110 |
| LOC | `ScriptVarTypeLoc` | 108 |
| SEQ | `ScriptVarTypeSeq` | 65 |
| STAT | `ScriptVarTypeStat` | 83 |
| INV | `ScriptVarTypeInv` | 118 |
| COORD | `ScriptVarTypeCoord` | 99 |
| INTERFACE | `ScriptVarTypeInterface` | 97 |
| SPOTANIM | `ScriptVarTypeSpotanim` | 116 |
| IDKIT | `ScriptVarTypeIdkit` | 75 |

All exist in `pkg/objtype/paramtype.go:29-55`.

### §3.4 Existing cheat-test fixture pattern

`modules/world/handlers_game_test.go` and `modules/world/handler_cheats_supermod_test.go` use `newTestPlayer` (or similar) and feed cheat input via the production `handleClientCheat` entry. Per memory `test_fixture_view_parity`, the test server needs `worldVars`/`configsView`/`invLookup`/`npcLookup` initialised for any script adapter to fire — DEBUGPROC will exercise the same path, so the fixture must initialise those.

Per memory `scriptstate_test_fixture_idioms`, `&ScriptState{}` directly is **not** the right shape here — DEBUGPROC tests should go through `runScript`, which builds state via `buildPlayerScriptState` and ensures `StackCapacity`, `Pointers`, `Provider`, `World`, `Configs`, etc. are wired.

## §4 Architecture

Three layers, in port order:

### §4.1 Layer 1: 4 new `ByName` helpers in `pkg/objtype/`

Mirrors NAI-186/187 cluster shape. Each helper consumes the existing `ConfigNames map[string]int` already populated at parse time. Each:

```go
// ByName returns the SeqType with the given debug-name, or nil if no
// SeqType has that name. Mirrors TS SeqType.getByName
// (cache/config/SeqType.ts), returning nil for the TS null sentinel.
//
// Used by debugproc dispatch (modules/world/handlers_game.go) to
// resolve SEQ-typed positional args. NAI-189.
func (c *SeqTypeConfigs) ByName(name string) *SeqType {
    if c == nil || c.ConfigNames == nil {
        return nil
    }
    id, ok := c.ConfigNames[name]
    if !ok {
        return nil
    }
    if id < 0 || id >= len(c.Configs) {
        return nil
    }
    return c.Configs[id]
}
```

Repeated for `SpotanimTypeConfigs`, `IdkTypeConfigs`, `InvTypeConfigs`. Each gets one `_test.go` test (positive lookup, missing-name returns nil, nil-receiver returns nil).

The defensive `c == nil` / `ConfigNames == nil` guards match the NAI-187 helpers (`loctype.go ByName` / `npctype.go ByName`) for consistency.

### §4.2 Layer 2: `dispatchDebugproc` helper in `modules/world/handlers_game.go`

Single new helper, ~120 LOC including the per-type switch. Returns nothing — TS's `false` return on parse failure is communicated by "not running the script" (early `return nil` from the helper before calling `s.runScript`). The caller in `handleClientCheat` does not propagate this — TS's `false` return is invisible to the client after the dev-block branch (the function falls through to `return true` at L619 either way).

```go
// dispatchDebugproc resolves a [debugproc,X] script by name and dispatches
// it via s.runScript with arguments parsed per the script's declared
// ParamTypes. Mirrors TS ClientCheatHandler.ts:59-148.
//
// cmd is the lowered first token of the cheat string, already verified
// to start with s.cfg.NodeDebugprocChar (default "~"). args is the
// post-first-space tail (may be empty). rawCheat is the full lowered
// cheat string, needed for the COORD branch's underscore-split parsing.
//
// TS-fidelity notes per spec §4.2:
//   - Unknown script name → silent return (TS L62-64).
//   - Unknown ByName lookup for any arg → -1 placed in slot (TS L74-139).
//   - Missing arg for any arg → empty/zero default; ByName lookups land at -1.
//   - COORD arg re-parses rawCheat (TS L113-124).
//   - Run via s.runScript with self=p, target=nil, protect=false (TS L148).
//
// NAI-189.
func (s *Server) dispatchDebugproc(p *Player, cmd string, args string, rawCheat string) {
    prefix := s.cfg.NodeDebugprocChar
    if !strings.HasPrefix(cmd, prefix) || len(cmd) <= len(prefix) {
        return
    }
    name := cmd[len(prefix):]
    sf := s.scriptProvider.GetByName(fmt.Sprintf("[debugproc,%s]", name))
    if sf == nil {
        return
    }

    // Tokenise the rest-args once. TS uses args.shift() over a pre-split
    // array; goscape's args is a single string. strings.Fields collapses
    // whitespace runs, matching TS's lowercased-then-split-on-' ' behaviour
    // (handlers_game.go:362 already lowercased the entire cheat).
    tokens := strings.Fields(args)
    take := func() string {
        if len(tokens) == 0 {
            return ""
        }
        t := tokens[0]
        tokens = tokens[1:]
        return t
    }

    paramCount := len(sf.ParamTypes)
    intArgs := make([]int, 0, paramCount)
    stringArgs := make([]string, 0, paramCount)

    for i := 0; i < paramCount; i++ {
        switch objtype.ScriptVarType(sf.ParamTypes[i]) {
        case objtype.ScriptVarTypeString:
            stringArgs = append(stringArgs, take())
        case objtype.ScriptVarTypeInt:
            // TS: parseInt(v ?? '0', 10) | 0  — bit-or-zero coerces NaN→0.
            // parseIntOr returns the default on parse failure, matching.
            intArgs = append(intArgs, parseIntOr(take(), 0))
        case objtype.ScriptVarTypeObj, objtype.ScriptVarTypeNamedObj:
            intArgs = append(intArgs, lookupID(s.objTypes.ByName(take())))
        case objtype.ScriptVarTypeNPC:
            intArgs = append(intArgs, lookupID(s.npcTypes.ByName(take())))
        case objtype.ScriptVarTypeLoc:
            intArgs = append(intArgs, lookupID(s.locTypes.ByName(take())))
        case objtype.ScriptVarTypeSeq:
            intArgs = append(intArgs, lookupID(s.seqTypes.ByName(take())))
        case objtype.ScriptVarTypeStat:
            tok := strings.ToUpper(take())
            // TS: PlayerStatMap.get(name.toUpperCase()) — returns undefined
            // on miss, which JS happily passes through (number-or-undefined
            // in the int slot). goscape: ok=false → -1, matching the TS-
            // miss-equivalent for downstream stat-id reads.
            stat, ok := objtype.PlayerStatMap[tok]
            if !ok {
                intArgs = append(intArgs, -1)
            } else {
                intArgs = append(intArgs, stat)
            }
        case objtype.ScriptVarTypeInv:
            intArgs = append(intArgs, lookupID(s.invTypes.ByName(take())))
        case objtype.ScriptVarTypeCoord:
            // TS L113-124: parse the WHOLE lowered cheat by underscore.
            //   "~setpos coord_0_50_50_32_32"
            //   args2 = ["~setpos coord", "0", "50", "50", "32", "32"]
            //   level = parseInt("~setpos coord".slice(6)) = parseInt(" coord") = NaN... wait.
            //   Actually slice(6) on a 13-char string drops "~setpo" → "s coord" — see §4.2.1.
            // The TS arithmetic is fragile but we mirror it verbatim and
            // pin one canonical input via test (§7).
            intArgs = append(intArgs, parseCoordFromRawCheat(rawCheat))
        case objtype.ScriptVarTypeInterface:
            intArgs = append(intArgs, lookupID(s.componentTypes.ByName(take())))
        case objtype.ScriptVarTypeSpotanim:
            intArgs = append(intArgs, lookupID(s.spotanimTypes.ByName(take())))
        case objtype.ScriptVarTypeIdkit:
            intArgs = append(intArgs, lookupID(s.idkTypes.ByName(take())))
        default:
            // TS has no default; any unrecognised type leaves the slot at -1.
            intArgs = append(intArgs, -1)
        }
    }

    s.runScript(sf, p, nil, false, intArgs, stringArgs)
}

// lookupID extracts the ID from a ByName result, returning -1 on nil.
// Mirrors TS getId() semantics: getId(name) returns -1 for unknown names.
// The type parameter accepts any *T with an ID int field.
func lookupID[T interface{ GetID() int }](v T) int {
    var zero T
    if any(v) == any(zero) { // nil-pointer check via interface-equality
        return -1
    }
    return v.GetID()
}
```

**Open implementation decision for the plan:** `lookupID` as written assumes a `GetID() int` method on each `*Type`. Alternative shapes (in increasing order of preference):

1. Per-type inline expansion: `nt := s.npcTypes.ByName(take()); id := -1; if nt != nil { id = nt.ID }; intArgs = append(intArgs, id)`. Verbose but no generics ceremony and no new method.
2. Per-`*Type` `ID()` getter via Go generics: requires every config to satisfy an interface.
3. Lambdas per arm: `intArgs = append(intArgs, idOf(s.objTypes.ByName(take())))` with one `idOf` per concrete type (variadic by name).

**Recommended for plan**: option 1 (inline expansion). Each arm is 3 lines instead of 1; total handler grows to ~150 LOC; zero new helpers; trivially greppable. Per memory `interface_at_cyclic_import_boundary`, lean toward concrete types unless an interface is required by a boundary.

### §4.2.1 COORD parsing — TS arithmetic detail

TS L113-124:
```ts
const args2 = cheat.split('_');
const level = parseInt(args2[0].slice(6));
const mx = parseInt(args2[1]);
const mz = parseInt(args2[2]);
const lx = parseInt(args2[3]);
const lz = parseInt(args2[4]);
params[i] = CoordGrid.packCoord(level, (mx << 6) + lx, (mz << 6) + lz);
```

`cheat` at this point is the **full lowered cheat string** (TS L44 `const { input: cheat } = message`, L46 `args = cheat.toLowerCase().split(' ')`). The `slice(6)` is a hardcoded offset; it produces:

| Input cheat | `args2[0]` | `slice(6)` | `parseInt` |
|---|---|---|---|
| `~coord_0_50_50_32_32` | `"~coord"` (6 chars) | `""` | `NaN` |
| `~setpos coord_0_50_50_32_32` | `"~setpos coord"` (13 chars) | `"s coord"` | `NaN` |
| `~mycoord_0_50_50_32_32` | `"~mycoord"` (8 chars) | `"rd"` | `NaN` |
| `~thecoord_0_50_50_32_32` | `"~thecoord"` (9 chars) | `"ord"` | `NaN` |

In all reasonable invocations, `slice(6)` lands on non-digit chars and `parseInt` returns `NaN`. `(mx << 6) + lx` and friends parse correctly (the integer slots are non-empty). `packCoord(NaN, ...)` then produces a level-bits-corrupted value but a valid x/z pair. **The level component is effectively always 0 (NaN coerces to 0 in `<<` operations downstream of `packCoord`)** — but the int reaches the script as a NaN-tainted slot.

Note: JS `parseInt('') === NaN` and `parseInt('rd') === NaN`. Goscape's `parseIntOr("rd", 0) === 0` — so a verbatim port using `parseIntOr` actually **diverges from TS** (TS NaN vs goscape 0). The closest faithful port is `strconv.Atoi` with an explicit NaN-equivalent sentinel.

Goscape options:
- **(a) Mirror TS arithmetic literally** — `slice(6)` + `Atoi` + sentinel-on-failure (e.g. propagate `-1` for the level slot). Pin one canonical-form test that asserts whatever this produces; flag as `DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE`.
- **(b) Derive level offset from `cmd` length** — `args2[0].slice(len(cmd)+1)` produces what TS *meant* to write (the level digit after `coord_`). This treats the TS bug as a port-time correction; flag as `DEVIATION-NAI-189-D1-FIX-TS-COORD-OFFSET`.

**Recommended**: (a). Per memory `true_to_ts_gate`, the default is verbatim TS even when TS is broken — fidelity-on-bugs preserves the upstream's invariant set. Per memory `defensive_gate_doc_comment_label`, the deviation tag makes the choice greppable for future revisits (especially if a smoke surfaces a debugproc that actually consumes a COORD arg).

**Plan-author worksheet**: confirm the slice-and-Atoi sentinel value (`-1`? `0`? `math.MinInt32`?) at plan-write by running the TS `parseInt` semantics through a JS REPL on the actual cheat shape and recording the resulting `packCoord` int.

### §4.3 Layer 3: prefix branch in `handleClientCheat`

Insert before the existing dev-block switch at `handlers_game.go:402-460`:

```go
if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
    // TS ClientCheatHandler.ts:59 — debugproc prefix dispatch BEFORE
    // the fixed-cmd switch. Cmd-form is `~scriptname` (or whatever
    // s.cfg.NodeDebugprocChar is set to, default "~"). NAI-189.
    if prefix := p.client.server.cfg.NodeDebugprocChar; prefix != "" && strings.HasPrefix(parts[0], prefix) {
        p.client.server.dispatchDebugproc(p, parts[0], args, cheat)
        return nil
    }
    switch parts[0] {
    case "fly":
        // ... unchanged
```

Notes:
- Pre-switch placement mirrors TS dispatch order (L59-148 before the else-if ladder for `reload`/`rebuild`/`speed`/`fly`/...).
- Empty-prefix config (`s.cfg.NodeDebugprocChar == ""`) short-circuits — the prefix branch is skipped and all cheats fall through to the switch. Matches the TS behaviour where `Environment.NODE_DEBUGPROC_CHAR` being unset would make `cmd[0] === undefined` always false.
- `cheat` is the third arg (the full lowered cheat string from `handlers_game.go:361`); needed for the COORD case in `dispatchDebugproc`.

### §4.4 Layer 4: carryforward rewrite (close-time)

Edit `handlers_game.go:370-389` to add the DEBUGPROC retirement note and demote the now-final cluster to just `reload` and `rebuild`. New text shape:

```text
DEVIATION-NAI-189-D1-CARRYFORWARD — supersedes
DEVIATION-NAI-188-D1-CARRYFORWARD. 2 TS ClientCheatHandler cheats
remain unported, both in the dev block (!NP && >=4) and both blocked
on the same infra gap (cache / script hot-reload):
  reload:  TS L149-150. Calls World.reload() — full cache
           hot-reload pipeline. No goscape equivalent.
  rebuild: TS L151-153. Calls World.rebuild() — script-provider
           hot-reload. Same infra gap as reload.
NAI-189 retired the DEBUGPROC dispatch path (TS L59-148):
[debugproc,X] scripts now resolve via s.scriptProvider.GetByName
and dispatch via s.runScript, with positional arguments parsed
per ScriptFile.ParamTypes. See dispatchDebugproc + 4 new ByName
helpers (Seq/Spotanim/Idk/Inv).
NAI-188 retired ::speed (TS L154-167) ...
[remainder unchanged]
```

The previous NAI-187/188 paragraphs stay; only the tally + the new DEBUGPROC paragraph are added.

## §5 Data flow

```
client cheat packet  →  handleClientCheat (handlers_game.go)
                        ├─ parse "~mydebug 42 banana"  →
                        │   parts[0]="~mydebug", args="42 banana", cheat="~mydebug 42 banana"
                        ├─ dev-block guard: !NodeProduction && staffModLevel >= 4
                        └─ prefix branch (HasPrefix(parts[0], "~")):
                            └─ dispatchDebugproc(p, "~mydebug", "42 banana", "~mydebug 42 banana")
                                ├─ scriptProvider.GetByName("[debugproc,mydebug]")
                                ├─ for i := range sf.ParamTypes:
                                │   ├─ switch sf.ParamTypes[i]:
                                │   │   ├─ ScriptVarTypeInt   → intArgs += parseIntOr(token, 0)
                                │   │   ├─ ScriptVarTypeString → stringArgs += token
                                │   │   ├─ ScriptVarTypeObj   → intArgs += s.objTypes.ByName(token).ID|-1
                                │   │   └─ ... (12 arms)
                                └─ s.runScript(sf, p, nil, false, intArgs, stringArgs)
                                    └─ existing buildPlayerScriptState → script.Execute → resumeOrFinish
```

## §6 Concurrency

DEBUGPROC dispatch is single-goroutine — same path as every other cheat (`processIn` → tick goroutine). The dispatched script runs synchronously within the tick (TS uses synchronous `executeScript`; goscape `runScript` also runs synchronously, suspending into the active-script slot if the script yields). No new concurrency surface.

## §7 Testing

### §7.1 Per-arg-type unit tests on `dispatchDebugproc`

One test per arg-type arm (12 total), driven by staged 1-param `[debugproc,test_X]` `ScriptFile` fixtures. Pattern (uses `newTestPlayer` / equivalent test server setup):

```go
func TestDispatchDebugproc_Int(t *testing.T) {
    s := newTestServer(t)
    p := newTestPlayer(t, s)
    p.staffModLevel = 4
    s.cfg.NodeProduction = false

    sf := stageDebugprocScript(t, s, "myint", []byte{
        byte(objtype.ScriptVarTypeInt),
    })
    // ... arrange so the script writes IntLocals[0] to a recordable sink

    handleClientCheat(p, []byte("~myint 42"))

    if got := stagedSink.LastInt; got != 42 {
        t.Errorf("IntLocals[0] = %d; want 42", got)
    }
    _ = sf // reference for clarity
}
```

| Test | Input | Stub `ParamTypes` | Assertion |
|---|---|---|---|
| `TestDispatchDebugproc_String` | `"~mystr hello"` | `[String]` | `stringArgs[0] == "hello"` |
| `TestDispatchDebugproc_String_Missing` | `"~mystr"` | `[String]` | `stringArgs[0] == ""` (TS `?? ''`) |
| `TestDispatchDebugproc_Int` | `"~myint 42"` | `[Int]` | `intArgs[0] == 42` |
| `TestDispatchDebugproc_Int_NonNumeric` | `"~myint banana"` | `[Int]` | `intArgs[0] == 0` (TS `parseInt(..) \| 0`) |
| `TestDispatchDebugproc_Int_Missing` | `"~myint"` | `[Int]` | `intArgs[0] == 0` (TS `?? '0'`) |
| `TestDispatchDebugproc_Obj_Hit` | `"~myobj knife"` (`objTypes.ConfigNames["knife"]=946`) | `[Obj]` | `intArgs[0] == 946` |
| `TestDispatchDebugproc_Obj_Miss` | `"~myobj unknown"` | `[Obj]` | `intArgs[0] == -1` (TS `getId('unknown')`) |
| `TestDispatchDebugproc_Namedobj_Hit` | `"~mynamedobj knife"` | `[Namedobj]` | `intArgs[0] == 946` |
| `TestDispatchDebugproc_Npc_Hit` | `"~mynpc man"` | `[NPC]` | matches `npcTypes.ConfigNames["man"]` |
| `TestDispatchDebugproc_Loc_Hit` | `"~myloc table_basic"` | `[Loc]` | matches `locTypes.ConfigNames["table_basic"]` |
| `TestDispatchDebugproc_Seq_Hit` | `"~myseq human_walk"` | `[Seq]` | matches `seqTypes.ConfigNames["human_walk"]` |
| `TestDispatchDebugproc_Stat_Hit` | `"~mystat attack"` | `[Stat]` | `intArgs[0] == objtype.PlayerStatAttack` (TS `PlayerStatMap.get("ATTACK")`) |
| `TestDispatchDebugproc_Stat_Miss` | `"~mystat unknown"` | `[Stat]` | `intArgs[0] == -1` |
| `TestDispatchDebugproc_Inv_Hit` | `"~myinv inv"` | `[Inv]` | matches `invTypes.ConfigNames["inv"]` |
| `TestDispatchDebugproc_Coord_OneToken` | `"~coord_0_50_50_32_32"` | `[Coord]` | `intArgs[0]` is the actual output of the TS slice(6)+parseInt arithmetic (`slice(6)` returns `""` for `"~coord"` so level coerces to the chosen sentinel — pin verbatim per §4.2.1). Per memory `skip_pin_full_struct_capture`, the expected int must come from a verbatim record of the TS-equivalent computation, not inferred. |
| `TestDispatchDebugproc_Coord_TwoToken` | `"~setpos coord_0_50_50_32_32"` | `[Coord]` | Same arithmetic, different `args2[0]` (`"~setpos coord"` → `slice(6) = "s coord"`). Pin verbatim. Documents the D1 deviation. |
| `TestDispatchDebugproc_Interface_Hit` | `"~myiface welcome_screen"` | `[Interface]` | matches `componentTypes.ConfigNames["welcome_screen"]` |
| `TestDispatchDebugproc_Spotanim_Hit` | `"~myspot air_strike"` | `[Spotanim]` | matches `spotanimTypes.ConfigNames["air_strike"]` |
| `TestDispatchDebugproc_Idkit_Hit` | `"~myidk arms"` | `[Idkit]` | matches `idkTypes.ConfigNames["arms"]` |

### §7.2 Negative path / no-dispatch tests

| Test | Input | Assertion |
|---|---|---|
| `TestDispatchDebugproc_UnknownScript` | `"~nonexistent foo"` | `s.runScript` not called (no active script on p; no side effects). |
| `TestDispatchDebugproc_BareDelimiter` | `"~"` | `len(cmd) <= len(prefix)` short-circuit; no lookup. |
| `TestDispatchDebugproc_EmptyDebugprocChar` | `s.cfg.NodeDebugprocChar=""`, input `"~foo"` | Prefix branch skipped; falls through to fixed-cmd switch (which has no case → no-op). |

### §7.3 Gate tests

| Test | `staffModLevel` | `NodeProduction` | Input | Assertion |
|---|---|---|---|---|
| `TestDispatchDebugproc_Gate_Mod3` | 3 | false | `"~mydebug"` | Dev-block skipped (TS `>= 4`); script not dispatched. |
| `TestDispatchDebugproc_Gate_Prod` | 4 | true | `"~mydebug"` | Dev-block skipped (TS `!NODE_PRODUCTION`); script not dispatched. |
| `TestDispatchDebugproc_Gate_Both` | 4 | false | `"~mydebug"` | Dispatched (positive control for the gate). |

### §7.4 Cohort-compatibility tests

`fly` / `naive` / `random` / `speed` must still dispatch correctly through the post-prefix-branch switch. The existing tests for these (search at plan-write — `grep -l "TestCheatFly\|TestCheatSpeed" modules/world/*_test.go`) must continue to pass unmodified.

If `parts[0]` does not start with `s.cfg.NodeDebugprocChar`, the prefix branch is a single boolean check + a fall-through. Concrete pin:

| Test | Input | Assertion |
|---|---|---|
| `TestDispatchDebugproc_Fallthrough_Fly` | `"fly"` (no prefix) | `p.moveStrategy` toggles per existing `case "fly":` semantics. |

### §7.5 ByName helper tests

One test per new helper (4 total), mirroring NAI-187 cluster shape:

| Test | File | Assertion |
|---|---|---|
| `TestSeqTypeConfigs_ByName` | `pkg/objtype/seqtype_test.go` | Stage `ConfigNames["human_walk"]=42` + `Configs[42]={ID:42}`; `ByName("human_walk").ID == 42`; `ByName("nope") == nil`; `(*SeqTypeConfigs)(nil).ByName("x") == nil`. |
| `TestSpotanimTypeConfigs_ByName` | `pkg/objtype/spotanimtype_test.go` | parallel |
| `TestIdkTypeConfigs_ByName` | `pkg/objtype/idktype_test.go` | parallel |
| `TestInvTypeConfigs_ByName` | `pkg/objtype/invtype_test.go` | parallel |

### §7.6 Smoke

End-to-end smoke requires:
1. A `[debugproc,X]` runescript present in the loaded cache, OR
2. A test fixture script staged via `stageDebugprocScript`.

Per memory `smoke_test_server_handoff`, Java-client-driven smokes need the user to launch the server. **Plan-author worksheet**: at plan-write, grep `runescript-rev-225/src/scripts/` (or whichever content path goscape's `2004scape/Server` snapshot lives at) for `[debugproc,…]` definitions to identify a smoke target — likely a `[debugproc,getpos]` or `[debugproc,setpos]`-style entry. If none exists, NAI-189 closes on unit tests alone and routes a smoke-handoff prompt to the user explaining how to add a one-liner debugproc to validate end-to-end.

## §8 Risk register

| Risk | Evidence | Mitigation |
|---|---|---|
| `script.Init` array sizing when args exceed declared ParamTypes | `runner.go:21-22` uses `max(declared, len(args))` | Spec dispatches exactly `paramCount` args; `len(intArgs)+len(stringArgs) == paramCount` by construction. No over-fill possible. Under-fill (script declares N but TS-style `take()` runs out) is handled per type — int slots get 0, string slots get "", lookup slots get -1. |
| `take()` mutates a closed-over `tokens` slice (shared state) | Single-goroutine dispatch; closure is local to `dispatchDebugproc` | No race; documented inline. |
| COORD parsing fragility | §4.2.1 trace shows TS produces `NaN` for two-token form | Mirror verbatim with `DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE`; pin both single-token and two-token forms in tests. |
| New ByName helpers vs production loaders | Loaders populate `ConfigNames` only when `DebugName != ""` (verified at `seqtype.go:164-165`, `spotanimtype.go:130-131`, `idktype.go:127-128`, `invtype.go:127-128`) | ByName helpers return nil for empty/missing keys; entries with empty `DebugName` are simply unreachable by ByName (TS-consistent). |
| Test fixture must stage `ScriptFile.ParamTypes` | `pkg/script/file.go:91-93` decodes from packet | Tests build `*ScriptFile` literals directly with `ParamTypes: []byte{byte(objtype.ScriptVarTypeInt)}` and register via `scriptProvider.Register(...)` or equivalent — verify the registration API exists at plan-write (`grep -n "Register\|AddScript\|provider_test" pkg/script/`). |
| Per memory `controller_preflight`: helper-API audit | §3.2 confirms `s.runScript`, `s.scriptProvider`, all `s.*Types` plumbed | No new plumbing required. |
| Per memory `plan_grep_helper_patterns`: existing token-shift helper | `handlers_game.go` has no pre-existing tokeniser helper | `take()` defined as a local closure; not extracted to a package-level helper. |
| Per memory `plan_sibling_site_guard_audit`: `s.scriptProvider != nil` guard | `script.go:49` uses `s.scriptProvider` directly without nil check | Mirror by direct use; `newTestServer` MUST init `s.scriptProvider` per memory `test_fixture_view_parity`. |
| Per memory `int32_hex_literal_overflow`: TS `parseInt(...) \| 0` | `parseIntOr(token, 0)` returns Go `int` (64-bit on 64-bit platforms) | Out-of-range ints from `parseIntOr` are not clamped; script handlers downstream may or may not honour the int32 contract. NAI-189 does not introduce a clamp — TS doesn't either after the `\| 0`, since modern JS coerces to int32 via the bitor. Risk-flag for future audit; not blocking. |
| Per memory `mock_recorder_field_naming_check`: stub-script test fixture | Plan-author must grep `pkg/script/provider*` to confirm Register API | Done at plan-write. |

## §9 Deviations from TS

- `DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE` — TS COORD parsing arithmetic (`args2[0].slice(6)`) produces `NaN` for two-token cheats like `~setpos coord_…`. Goscape mirrors this verbatim; the one-token form (`~coord_…`) works correctly in both. Flag for future fix coordinated with upstream LostCity content.

No other deviations.

## §10 Close criteria

- All §7 tests pass with `-race`.
- 4 new `ByName` helpers exported on `pkg/objtype/{seqtype,spotanimtype,idktype,invtype}.go`.
- `dispatchDebugproc` helper in `modules/world/handlers_game.go` with all 12 arg-type arms.
- Prefix branch in `handleClientCheat` (§4.3) dispatches `~*` to `dispatchDebugproc` BEFORE the fixed-cmd switch.
- Existing `fly` / `naive` / `random` / `speed` cohort tests pass unmodified.
- `DEVIATION-NAI-188-D1-CARRYFORWARD` doc-comment rewritten to `DEVIATION-NAI-189-D1-CARRYFORWARD`; tally **unchanged at 2** (DEBUGPROC was never counted — it was missing from the prior tally per §1); a new paragraph notes DEBUGPROC retirement and lists the 4 new ByName helpers + `dispatchDebugproc`.
- `git grep "NodeDebugprocChar" modules/` shows the field is now read (previously unread).
- Closing commit body includes `Closes memory:` trailer per memory `close_commit_memory_trailer`.

## §11 Plan-author worksheet

Plan should split into tasks roughly:

- **T1** (helper) — `SeqType.ByName` + test.
- **T2** (helper) — `SpotanimType.ByName` + test.
- **T3** (helper) — `IdkType.ByName` + test.
- **T4** (helper) — `InvType.ByName` + test.
- **T5** (RED) — write all §7.1/§7.2/§7.3 tests against the unported handler. All must fail (no prefix branch; no `dispatchDebugproc`). Run `-race`.
- **T6** (GREEN) — add `dispatchDebugproc` helper + prefix branch wiring in `handleClientCheat`. All §7 tests pass. Existing dev-block-cohort tests still pass.
- **T7** (DOCS) — rewrite the `DEVIATION-NAI-188-D1-CARRYFORWARD` block per §4.4.
- **CLOSE** — `chore(close): NAI-189 …` with `Closes memory:` trailer for any new tracker entries (DEVIATION-NAI-189-D1 likely worth a memory entry per memory `tracker_carryforward_listings_compound`).

Plan-author pre-flight:
1. Grep `pkg/script/provider*` at plan-write to confirm the script-registration API for test fixtures (`Register`, `Add`, or equivalent).
2. Grep `pkg/objtype` `ConfigNames` populator branches to confirm parallel pattern across Seq/Spotanim/Idk/Inv before bulk-cloning the NAI-187 ByName shape.
3. Re-Read `modules/world/handlers_game.go:402-460` at plan-write to confirm the dev-block scope hasn't shifted (per memory `spec_test_runtime_behavior_verify`).
4. Identify the cheat-test file holding `TestCheatFly` / `TestCheatSpeed_*` at plan-write — DEBUGPROC tests live alongside or in a new sibling file.
5. Grep `runescript-rev-225/src/scripts/**/*.rs2` (or the goscape cache content snapshot) for `^\[debugproc,` definitions to identify a smoke target for §7.6.
