# NAI-139 Stage 1 — Bundle B2: `~stat_reset_all` proc subtree audit

**Date:** 2026-05-09  
**Scope:** `[proc,stat_reset_all]` + `[proc,stat_reset]` and their constituent opcodes.  
**TS source:** `LostCityRS/Content/scripts/player/scripts/stat.rs2:62-92`

---

## TS proc bodies (confirmed by Read)

```runescript
[proc,stat_reset](stat $stat)           ← line 62
def_int $d = sub(stat($stat), stat_base($stat));
if ($d > 0) {
    stat_sub($stat, abs($d), 0);
} else if ($d < 0) {
    stat_add($stat, abs($d), 0);
}

[proc,stat_reset_all]                   ← line 71
def_int $i = 1;
while ($i <= enum_getoutputcount(stats)) {
    ~stat_reset(enum(int, stat, stats, $i));
    $i = calc($i + 1);
}

[proc,.stat_reset](stat $stat)          ← line 78 (NPC context, dot-prefix)
def_int $d = sub(.stat($stat), .stat_base($stat));
if ($d > 0) {
    .stat_sub($stat, abs($d), 0);
} else if ($d < 0) {
    .stat_add($stat, abs($d), 0);
}

[proc,.stat_reset_all]                  ← line 87 (NPC context)
```

---

## Script cache loading — proc resolution

`Provider.Load` (pkg/script/provider.go:42-106) reads `script.dat` + `script.idx`, decodes each blob via `Decode`, and populates `p.byName[f.Name]` (provider.go:99). `GetByName` (provider.go:156) performs a direct map lookup. The `Name` field is the first NUL-terminated string in the blob (file.go:85: `f.Name = pkt.GJStrNUL()`). The RuneScript compiler stores the proc name as that string (e.g. `"proc,stat_reset_all"`). `GOSUB_WITH_PARAMS` (opcode 40) uses the script id baked into the operand at compile time (handlers.go:682-683: `targetID := uint32(s.Script.IntOperands[s.PC]); target := s.Provider.GetByID(targetID)`). Resolution is by numeric id, not name lookup at runtime; the name table is for trigger-based dispatch only. **Conclusion:** `~stat_reset_all` and `~stat_reset` will load correctly from cache as long as the packed ids are valid.

---

## Compiler construct note

`def_int`, `calc`, and `while` are **RuneScript compiler sugar**, not runtime opcodes. They compile to: `PUSH_CONSTANT_INT` / `PUSH_INT_LOCAL` / `POP_INT_LOCAL` + arithmetic + `BRANCH*` opcodes. Confirmed: no `CALC`, `DEF_INT`, or `WHILE` appear in opcode.go — only the underlying primitives exist. All underlying primitives are registered in the handlers map (see rows below).

---

## Findings table

| token | kind | ts_ref | goscape_dispatch | status | evidence |
|-------|------|--------|------------------|--------|----------|
| `~stat_reset_all` | proc | stat.rs2:71 | pkg/script/provider.go:98-99 + handlers.go:682-684 | WIRED | Loaded by name into `p.byName`; called via `GOSUB_WITH_PARAMS` using numeric script id baked at compile time. provider.go:99: `p.byName[f.Name] = f`; handlers.go:682: `targetID := uint32(s.Script.IntOperands[s.PC])` |
| `~stat_reset` | proc | stat.rs2:62 | pkg/script/provider.go:98-99 + handlers.go:682-684 | WIRED | Same mechanism as `~stat_reset_all`. Called from `stat_reset_all` body via `GOSUB_WITH_PARAMS` with operand = script id. |
| `enum_getoutputcount` | opcode | stat.rs2:73 | handlers.go:257 → handlers_config.go:116 | WIRED | `OpEnumGetOutputCount: handleEnumGetOutputCount` (handlers.go:257). handlers_config.go:116: `func handleEnumGetOutputCount(s *ScriptState) error {` — pops enumID, calls `s.Configs.EnumType(enumID)`, pushes `len(et.Values)`. |
| `enum` | opcode | stat.rs2:74 | handlers.go:256 → handlers_config.go:70 | WIRED | `OpEnum: handleEnum` (handlers.go:256). handlers_config.go:70: `func handleEnum(s *ScriptState) error {` — pops `[inputType, outputType, enumID, key]`, validates types, pushes value from `et.Values` or default. |
| `stat` (player) | opcode | stat.rs2:63 | handlers.go:215 → handlers_player.go:255 | WIRED | `OpStat: handleStat` (handlers.go:215). handlers_player.go:253-264: `func handleStat(s *ScriptState) error` — pops stat id, pushes `s.Self.Stat(id)`. |
| `stat_base` (player) | opcode | stat.rs2:63 | handlers.go:216 → handlers_player.go:268 | WIRED | `OpStatBase: handleStatBase` (handlers.go:216). handlers_player.go:268: `func handleStatBase(s *ScriptState) error` — pops stat id, pushes `s.Self.StatBase(id)`. |
| `stat_sub` (player, 3-arg) | opcode | stat.rs2:66 | handlers.go:219 → handlers_player.go:336 | WIRED | `OpStatSub: handleStatSub` (handlers.go:219). Pop order: percent (top), constant, id (bottom) — matches TS `popInts(3)`. Formula: `subbed = cur - (constant + (base*percent)/100)`, clamped `≥ 0`, stored via `SetCurLevel`. handlers_player.go:336-359. |
| `stat_add` (player, 3-arg) | opcode | stat.rs2:68 | handlers.go:218 → handlers_player.go:305 | WIRED | `OpStatAdd: handleStatAdd` (handlers.go:218). Pop order: percent (top), constant, id (bottom). Formula: `added = cur + (constant + (base*percent)/100)`, clamped `≤ 255`, stored via `SetCurLevel`. handlers_player.go:300-329. |
| `.stat` (NPC context) | opcode | stat.rs2:79 | handlers.go:403 | WIRED | `OpNpcStat: handleNpcStat` (handlers.go:403). Confirmed opcode 2516 registered. |
| `.stat_base` (NPC context) | opcode | stat.rs2:79 | — | UNKNOWN | `OpNpcStatBase` not found in opcode.go or handlers.go: `grep -n "NpcStatBase\|NPC_STATBASE\|OpNpcStatBase" pkg/script/opcode.go pkg/script/handlers.go` returned no results. The `.stat_base` opcode is needed by `[proc,.stat_reset]` to compute `$d`. Needs further investigation (may be encoded differently or absent). |
| `.stat_sub` (NPC context) | opcode | stat.rs2:82 | handlers.go:438 → handlers_npc.go:1007 | WIRED | `OpNpcStatSub: handleNpcStatSub` (handlers.go:438). handlers_npc.go:999: `func handleNpcStatSub(s *ScriptState) error` — pops percent, constant, stat; formula `subbed = cur - (constant + (base*percent)/100)` clamped ≥ 0, stored via `SetNpcStat`. |
| `.stat_add` (NPC context) | opcode | stat.rs2:84 | handlers.go:437 → handlers_npc.go:975 | WIRED | `OpNpcStatAdd: handleNpcStatAdd` (handlers.go:437). handlers_npc.go:965: `func handleNpcStatAdd(s *ScriptState) error` — pops percent, constant, stat; formula `added = cur + (constant + (base*percent)/100)` clamped ≤ 255, stored via `SetNpcStat`. |
| `abs` | opcode | stat.rs2:66,68 | handlers.go:47 → handlers_number.go:112 | WIRED | `OpAbs: handleAbs` (handlers.go:47). handlers_number.go:112: `func handleAbs(s *ScriptState) error { x := s.PopInt(); if x < 0 { x = -x }; s.PushInt(x) }`. |
| `sub` | opcode | stat.rs2:63 | handlers.go:28 → handlers.go:638 | WIRED | `OpSub: handleSub` (handlers.go:28). handlers.go:638: `func handleSub(s *ScriptState) error { b := s.PopInt(); a := s.PopInt(); s.PushInt(a - b) }`. |
| `calc` | compiler-sugar | stat.rs2:75 | handlers.go:27,630 (OpAdd) | WIRED | `calc($i + 1)` compiles to `PUSH_INT_LOCAL[$i]` + `PUSH_CONSTANT_INT[1]` + `OpAdd`. `OpAdd: handleAdd` registered handlers.go:27. handlers.go:630: `func handleAdd`. No runtime `CALC` opcode. |
| `def_int` | compiler-sugar | stat.rs2:63,72 | handlers.go:17-18 (OpPushIntLocal/OpPopIntLocal) | WIRED | `def_int $d = …` compiles to compute expression + `POP_INT_LOCAL`. `OpPushIntLocal: handlePushIntLocal` (handlers.go:17); `OpPopIntLocal: handlePopIntLocal` (handlers.go:18). No runtime `DEF_INT` opcode. |
| `while` | compiler-sugar | stat.rs2:73 | handlers.go:21,38-41 (OpBranch + comparison branches) | WIRED | `while` compiles to branch opcodes. `OpBranch: handleBranch` (handlers.go:21); `OpBranchLessThanOrEquals: handleBranchLessThanOrEquals` (handlers.go:40) implements `$i <= …`. No runtime `WHILE` opcode. |
| `if` (>0 / <0 conditions) | compiler-sugar | stat.rs2:65,67 | handlers.go:39,38 (OpBranchNot + OpBranchGreaterThan etc.) | WIRED | `if ($d > 0)` → `PUSH_INT_LOCAL[$d]` + `PUSH_CONSTANT_INT[0]` + `BRANCH_GREATER_THAN`. All comparison-branch opcodes registered handlers.go:38-41. |

---

## Semantic verification: stat_sub/stat_add do NOT clear XP

`handleStatAdd` calls `s.Self.SetCurLevel(id, added)` (handlers_player.go:327). `handleStatSub` calls `s.Self.SetCurLevel(id, subbed)` (handlers_player.go:358). `SetCurLevel` is defined in the `ActivePlayer` interface (active.go:130-132) as "overrides the player's current level for skill id, clamped to [0, 255]." XP is managed only by `AddXP` (active.go:134-137), which is called exclusively by `handleStatXPAdd` (handlers_player.go:474-496). **Conclusion: stat_sub/stat_add modify only the current (boosted/drained) level. XP is untouched. This is correct TS-fidelity behavior.**

---

## Summary of status counts

| status | count | tokens |
|--------|-------|--------|
| WIRED | 16 | `~stat_reset_all`, `~stat_reset`, `enum_getoutputcount`, `enum`, `stat`, `stat_base`, `stat_sub`, `stat_add`, `.stat`, `.stat_sub`, `.stat_add`, `abs`, `sub`, `calc`, `def_int`, `while`/`if` |
| UNKNOWN | 1 | `.stat_base` (NPC context) — opcode not found in goscape; needed by `[proc,.stat_reset]` |
| STUB | 0 | — |
| MISSING | 0 | — |

## Risk

`.stat_base` (NPC context variant) has no corresponding opcode or handler in goscape. This will cause a runtime abort if `[proc,.stat_reset]` (and thus `[proc,.stat_reset_all]`) is ever executed in NPC-context. Player-context `~stat_reset_all` / `~stat_reset` are fully wired and safe.
