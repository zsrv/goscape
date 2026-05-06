# NAI-114 — Stage 1.1 Bundle 0 findings

**Date:** 2026-05-06
**Spec:** docs/superpowers/specs/2026-05-06-nai-114-opheldu-tinderbox-firemaking-investigation-design.md
**Stage:** 1.1 — controller disasm extension; static-only; no production change.

## 1. SWITCH table for `[opheldu,tinderbox]` PC 18

Probe output (script-level switch tables; goscape `ScriptFile.SwitchTables` is a slice indexed by the SWITCH operand):

```
switch[0] (2 cases):
  case   150 → PC offset 4
  case   212 → PC offset 1
```

Resolved against the bytecode at PCs 18-26 of `[opheldu,tinderbox]`:

| Case key | PC offset | Routes to (PC of next instruction)¹ | Meaning |
|---|---|---|---|
| 150 | 4 | PC 23 = `LAST_USEITEM 0` then PC 24 = `JUMP_WITH_PARAMS 7360` | Light-source category (lantern/torch ignition). |
| 212 | 1 | PC 20 = `LAST_USESLOT 0` then PC 21 = `JUMP_WITH_PARAMS 7356` | **Logs category → `[label,light_logs_inv]`.** |
| (default) | — | PC 19 = `BRANCH 6` → PC 26 → `PUSH_CONSTANT_INT 0; GOSUB 2130` | "Nothing interesting happens." |

¹ Per goscape's branch convention (`pkg/script/runner.go:50-53`), `s.PC += offset` then loop `s.PC++`. SWITCH is at PC 18; handler sets PC = 18 + offset; loop bumps to 19 + offset. Case 212: `19 + 1 = 20`. Case 150: `19 + 4 = 23`. Default fallthrough: `18 + 1 = 19` (BRANCH instruction).

**Verdict:** case 212 is **PRESENT**. Routes to PC 20 → `JUMP_WITH_PARAMS 7356` ([label,light_logs_inv]). The cache-side switch table is correct.

## 2. Disasm: `[label,light_logs_inv]` (id 7356)

Source: `LostCityRS/Server/content/scripts/skill_firemaking/scripts/firemaking.rs2`.
Locals: 3 int locals, 1 int arg.

```
  0:  PUSH_CONSTANT_INT         93         ; inv = inv_inv
  1:  PUSH_INT_LOCAL            0          ; arg slot
  2:  INV_GETOBJ                0          ; → obj id at slot
  3:  POP_INT_LOCAL             1          ; local1 = obj.id (logs)
  4:  PUSH_INT_LOCAL            1
  5:  PUSH_CONSTANT_INT         86         ; param 86 = firemaking_level
  6:  OC_PARAM                  0          ; param[86] of logs = 1
  7:  POP_INT_LOCAL             2          ; local2 = required level
  8:  PUSH_CONSTANT_INT         11         ; stat 11 = firemaking
  9:  STAT                      0
 10:  PUSH_INT_LOCAL            2
 11:  BRANCH_LESS_THAN          1          ; if stat < required → mes-fail
 12:  BRANCH                    11
 13:  PUSH_CONSTANT_STRING      "You need a Firemaking level of "
 14:  PUSH_INT_LOCAL            2
 15:  TOSTRING                  0
 16:  PUSH_CONSTANT_STRING      " to burn "
 17:  PUSH_INT_LOCAL            1
 18:  OC_NAME                   0
 19:  PUSH_CONSTANT_STRING      "."
 20:  JOIN_STRING               5
 21:  MES                       0
 22:  RETURN                    0
 23:  BRANCH                    0
 24:  COORD                     0          ; player coord
 25:  GOSUB_WITH_PARAMS         7358       ; [proc,area_allow_loc_add] → 0/1
 26:  PUSH_CONSTANT_INT         0
 27:  BRANCH_EQUALS             1          ; if result == 0 → "can't light a fire here"
 28:  BRANCH                    4
 29:  PUSH_CONSTANT_STRING      "You can't light a fire here."
 30:  MES                       0
 31:  RETURN                    0
 32:  BRANCH                    0
 33:  PUSH_CONSTANT_INT         93         ; inv_inv
 34:  PUSH_CONSTANT_INT         590        ; tinderbox.id
 35:  INV_TOTAL                 0          ; total tinderbox
 36:  PUSH_CONSTANT_INT         1
 37:  BRANCH_LESS_THAN          1          ; if < 1 → "need a tinderbox"
 38:  BRANCH                    4
 39:  PUSH_CONSTANT_STRING      "You need a tinderbox to light a fire."
 40:  MES                       0
 41:  RETURN                    0
 42:  BRANCH                    0
 43:  PUSH_CONSTANT_INT         93         ; inv_inv
 44:  COORD                     0          ; player coord
 45:  PUSH_INT_LOCAL            0          ; logs slot
 46:  PUSH_CONSTANT_INT         200
 47:  INV_DROPSLOT              0          ; drop logs to ground at coord, ttl=200
 48:  PUSH_VARP                 58         ; varp 58 = firemaking_attempt clock
 49:  MAP_CLOCK                 0          ; world tick clock
 50:  BRANCH_LESS_THAN          1          ; if attempt-clock < world-clock → first attempt
 51:  BRANCH                    15         ; → PC 67 (success/fail roll)
 52:  P_STOPACTION              0          ; (first attempt path)
 53:  PUSH_CONSTANT_INT         733        ; firemaking anim seq
 54:  PUSH_CONSTANT_INT         0
 55:  ANIM                      0
 56:  PUSH_CONSTANT_INT         195        ; firemaking sound
 57:  PUSH_CONSTANT_INT         0
 58:  PUSH_CONSTANT_INT         0
 59:  SOUND_SYNTH               0
 60:  PUSH_CONSTANT_STRING      "You attempt to light the logs."
 61:  MES                       0
 62:  MAP_CLOCK                 0
 63:  PUSH_CONSTANT_INT         3
 64:  ADD                       0
 65:  POP_VARP                  58         ; varp 58 = world_clock + 3
 66:  BRANCH                    36         ; → PC 103 (P_OPOBJ 4 — tail-call self next tick)
 67:  PUSH_VARP                 58
 68:  MAP_CLOCK                 0
 69:  BRANCH_EQUALS             1          ; if attempt-clock == world-clock → success roll
 70:  BRANCH                    31         ; else (attempt-clock > world-clock + retry?) → PC 89
 71:  PUSH_CONSTANT_INT         11         ; firemaking stat
 72:  PUSH_CONSTANT_INT         64
 73:  PUSH_CONSTANT_INT         512
 74:  STAT_RANDOM               0          ; random success roll
 75:  PUSH_CONSTANT_INT         1
 76:  BRANCH_EQUALS             1          ; if success
 77:  BRANCH                    11         ; → PC 89 (failure path)
 78:  MAP_CLOCK                 0
 79:  PUSH_CONSTANT_INT         4
 80:  ADD                       0
 81:  POP_VARP                  58         ; varp 58 = world_clock + 4 (cooldown)
 82:  OBJ_COORD                 0          ; coord of dropped logs obj
 83:  GOSUB_WITH_PARAMS         7359       ; [proc,push_player] (animate-and-step-off)
 84:  OBJ_COORD                 0
 85:  PUSH_INT_LOCAL            1          ; logs.id
 86:  GOSUB_WITH_PARAMS         7357       ; [proc,firemaking_success] (loc_add fire + stat_advance + obj_addall ash)
 87:  RETURN                    0
 88:  BRANCH                    12
 89:  PUSH_CONSTANT_INT         733        ; (failure / retry path)
 90:  PUSH_CONSTANT_INT         0
 91:  ANIM                      0
 92:  PUSH_CONSTANT_INT         195
 93:  PUSH_CONSTANT_INT         0
 94:  PUSH_CONSTANT_INT         0
 95:  SOUND_SYNTH               0
 96:  MAP_CLOCK                 0
 97:  PUSH_CONSTANT_INT         4
 98:  ADD                       0
 99:  POP_VARP                  58         ; varp 58 = world_clock + 4
100:  BRANCH                    0
101:  BRANCH                    0
102:  BRANCH                    0
103:  PUSH_CONSTANT_INT         4          ; op slot 4 (tail-call self via P_OPOBJ)
104:  P_OPOBJ                   0
105:  RETURN                    0
```

## 3. Disasm: chained scripts

### id 7358 (`[proc,area_allow_loc_add]` — fire-tile probe)

Source: `firemaking.rs2`. Returns 1 if logs may become a fire here, 0 otherwise. Three gates:

```
  0:  PUSH_INT_LOCAL  0; MAP_LOCADDUNSAFE 0    ; tile.unsafe-loc-add allowed?
  ...                       BRANCH_EQUALS 1; BRANCH 3 → return 0 if not allowed
  8:  PUSH_INT_LOCAL  0; MAP_BLOCKED      0    ; tile blocked?
  ...                       BRANCH_EQUALS 1; BRANCH 3 → return 0 if blocked
 16:  PUSH_CONSTANT_INT 780; COORD; GOSUB 2120 ; [proc,inzone_coord_pair_table] db lookup
                                                ; (db 4096 = forbidden-fire-zones; 780 row?)
 19-22: BRANCH on result == 1  → return 0 if in forbidden zone
 25:  PUSH_CONSTANT_INT 1; RETURN              ; allowed
```

### id 7360 (`[label,ignite_light_source]` — case-150 path; not Tutorial-relevant)

Source: `light_source.rs2`. Lantern/torch ignition; off-path for our investigation but kept for completeness. INV_DEL the source, INV_ADD the lit form, MES "You light the X." Includes a Karamja-island INZONE check (45223111/314379519 coord pair).

### id 7359 (`[proc,push_player]` — step-off-and-face)

Source: `firemaking.rs2`. PUSH_INT_LOCAL 0 = direction-arg.
Logic: ANIM(-1, 0) (clear current anim) → GOSUB 3005 ([proc,in_duel_arena]) → if in duel arena, just P_DELAY 0 and RETURN. Else: probe four cardinal-step coords via MOVECOORD/LINEOFWALK; first walkable direction → P_TELEPORT to that coord and P_DELAY 0.

### id 7357 (`[proc,firemaking_success]` — fire-loc + xp + ash)

Source: `firemaking.rs2`. Locals: 3 int, 2 int args (arg0=coord, arg1=logs.id).

```
  0:  OBJ_DEL                                  ; remove dropped logs
  1-5: STAT_ADVANCE 11 by param[132](logs)=400 ; firemaking xp
  6-9: SOUND_SYNTH 194                         ; ignite sfx
 10-11: FACESQUARE arg0                        ; face fire
 12-13: MES "The fire catches and the logs begin to burn."
 14-18: random ttl = 100 + RANDOM(100)         ; local2
 19-24: LOC_ADD coord, 2732 (fire loc), shape=1, angle=10, ttl=local2
 25-28: WORLD_DELAY local2 - 2                 ; sleep this script for fire ttl
 29-33: OBJ_ADDALL arg0, 592 (ash), 1, 100     ; spawn ash for everyone after fire dies
 34: RETURN
```

### id 7905 (`[proc,tut_firemaking_success]` — tutorial sibling, not on opheldu-tinderbox path)

For reference. Not invoked from `[opheldu,tinderbox]` chain (it's invoked from `[label,tut_light_logs_inv]` id 7904, the newbielogs branch at PC 5 of the parent).

### id 7942 (`[proc,tutorial_please_wait_firemaking]` — tutorial chatbox)

Off path. Newbielogs branch only.

### id 8030 (`[proc,tutorialstep]`) and id 3005 (`[proc,in_duel_arena]`) and id 2120 (`[proc,inzone_coord_pair_table]`)

Helper procs. 2120 walks `db 4096` (a coord-pair table) and returns 1 if `(arg1, db_field_value)` is INZONE matching arg-coord. Used by 7358 and 3005.

### id 2130 (`[proc,displaymessage]`)

Default fallthrough target from `[opheldu,tinderbox]` PC 27 (case-212 ABSENT default) — but we now know case 212 is PRESENT, so this default is unreachable for logs.

```
0: PUSH_CONSTANT_INT 105; PUSH_CONSTANT_INT 115; PUSH_CONSTANT_INT 11; PUSH_INT_LOCAL 0;
   ENUM 0; MES 0; RETURN
```

ENUM(105, 115, 11, key=arg0): chatbox displaymessage table — index 0 = "Nothing interesting happens." per Engine-TS convention.

## 4. Java client wire ordering

From `/home/owner/Code/github.com/LostCityRS/Client-Java/src/main/java/deob/client.java:5072-5078` (the `var5 == 881` branch — "use X with Y" menu option):

```java
this.out.p1isaac(130);                    // OPHELDU opcode
this.out.p2(var6);                        // wire field 0 = obj    = target.id (clicked-on)
this.out.p2(var3);                        // wire field 1 = slot   = target.slot
this.out.p2(var4);                        // wire field 2 = com    = target.com
this.out.p2(this.objInterface);           // wire field 3 = useObj = use-item.id (held; saved at var5 == 188)
this.out.p2(this.objSelectedSlot);        // wire field 4 = useSlot
this.out.p2(this.objSelectedInterface);   // wire field 5 = useCom
```

`this.objInterface` is set at `client.java:5118` inside `var5 == 188` (the "Use" menu option) where `var6 = source-item.id`. Despite the deobfuscated name, the value held is the use-item's obj id. ObjType comes from `ObjType.get(var6)` immediately after (line 5119).

TS decoder (`Engine-TS/src/network/game/client/codec/OpHeldUDecoder.ts:9-18`) reads in matching order: obj, slot, com, useObj, useSlot, useCom.

When player drags **tinderbox onto logs**:
- Wire `obj`    = **logs**       (target — clicked-on)
- Wire `useObj` = **tinderbox**   (use-item — held in selected state)

**Therefore:** in `handleOpHeldU` (`Engine-TS/src/network/game/client/handler/OpHeldUHandler.ts:96-117`):
- Arm `[opheldu, obj=logs]` lookup at line 96 → **MISS** (no `[opheldu,logs]` in cache).
- Arm `[opheldu, useObj=tinderbox]` lookup at line 100 → **HIT**, swap fires (lines 101-102).
- Post-dispatch: `lastItem = tinderbox`, `lastUseItem = logs`.

Inside `[opheldu,tinderbox]` body, `LAST_USEITEM = logs` (id 1511, Category=212). The PC 17 `OC_CATEGORY(LAST_USEITEM)` push is 212; SWITCH at PC 18 case 212 → PC 20 → `JUMP_WITH_PARAMS 7356` ([label,light_logs_inv]).

## 5. SWITCH opcode-handler walk (goscape vs TS)

**Goscape** — `pkg/script/handlers_array.go:52-63`:

```go
func handleSwitch(s *ScriptState) error {
    key := int32(s.PopInt())
    tableIdx := int(s.Script.IntOperands[s.PC])
    if tableIdx < 0 || tableIdx >= len(s.Script.SwitchTables) {
        return nil
    }
    table := s.Script.SwitchTables[tableIdx]
    if offset, ok := table[key]; ok {
        s.PC += int(offset)
    }
    return nil
}
```

PC convention: `pkg/script/runner.go:50-53` — handler sets PC such that the dispatch loop's `s.PC++` lands on the next instruction. SWITCH at PC=18, hit with offset=1: handler sets PC=19; loop bumps to 20.

**TS** — `Engine-TS/src/engine/script/handlers/CoreOps.ts:244-255`:

```ts
[ScriptOpcode.SWITCH]: state => {
    const key = state.popInt();
    const table = state.script.switchTables[state.intOperand];
    if (table === undefined) {
        return;
    }
    const result = table[key];
    if (result) {
        state.pc += result;
    }
},
```

TS ScriptRunner has the same +1-after-handler convention (the goscape comment cites it directly).

**Diff verdict:** semantic match for case 212 → offset 1.

| Aspect | TS | Goscape |
|---|---|---|
| Pop key | popInt | PopInt |
| Operand source | `state.intOperand` | `s.Script.IntOperands[s.PC]` |
| Missing table | early return | early return on bounds |
| Hit advance | `state.pc += result` | `s.PC += int(offset)` |
| Miss advance | none (fallthrough) | none (fallthrough) |

One minor edge: TS's `if (result)` treats `result === 0` as miss; goscape's `if offset, ok` treats it as a hit with `+= 0`. Net PC effect is identical (0 vs no-op). Not relevant for offset=1.

**H3.a (SWITCH-decode-layer divergence) → REFUTED.**

## 6. Opcode inventory (chain-wide)

64 distinct opcodes. **No `*** UNKNOWN ***` entries** — every opcode in the chain has a name registered in `pkg/script/opcode.go:String()`.

```
  ADD
  ANIM
  BRANCH
  BRANCH_EQUALS
  BRANCH_GREATER_THAN
  BRANCH_LESS_THAN
  BRANCH_NOT
  COORD
  DB_GETFIELD
  DB_GETFIELDCOUNT
  DIVIDE
  ENUM
  FACESQUARE
  GOSUB_WITH_PARAMS
  IF_SETTEXT
  INV_ADD
  INV_DEL
  INV_DROPSLOT
  INV_GETOBJ
  INV_ITEMSPACE
  INV_SIZE
  INV_TOTAL
  INZONE
  JOIN_STRING
  JUMP_WITH_PARAMS
  LAST_USEITEM
  LAST_USESLOT
  LINEOFWALK
  LOC_ADD
  LOWERCASE
  MAP_BLOCKED
  MAP_CLOCK
  MAP_LOCADDUNSAFE
  MES
  MOVECOORD
  OBJ_ADD
  OBJ_ADDALL
  OBJ_COORD
  OBJ_DEL
  OC_CATEGORY
  OC_NAME
  OC_PARAM
  POP_INT_LOCAL
  POP_VARP
  PUSH_CONSTANT_INT
  PUSH_CONSTANT_STRING
  PUSH_INT_LOCAL
  PUSH_STRING_LOCAL
  PUSH_VARP
  P_DELAY
  P_OPOBJ
  P_STOPACTION
  P_TELEPORT
  RANDOM
  RETURN
  SOUND_SYNTH
  SPLIT_INIT
  SPLIT_PAGECOUNT
  STAT
  STAT_ADVANCE
  STAT_RANDOM
  SWITCH
  TOSTRING
  WORLD_DELAY
```

**Unknown opcodes:** none.

Cache state confirmed:

```
logs id=1511 Category=212 Members=false
  param[86]  = 1     (firemaking level)
  param[132] = 400   (firemaking xp ×10)
newbielogs id=2511 Category=-1 Members=false
tinderbox  id=590  Category=-1 Members=false
```

## 7. Bundle 0 hypothesis-status update

| H | Status | Evidence |
|---|---|---|
| H3.a (SWITCH case-212 mismatch — data) | REFUTED | §1: case 212 PRESENT, offset=1, routes to PC 20 → JUMP 7356. |
| H3.a' (SWITCH case-212 mismatch — handler) | REFUTED | §5: goscape `handleSwitch` matches TS line-by-line. |
| H3.b (opcode in 7356 silent-abort) | LIVE — pending Stage 1.2 audit | Full opcode walk needed for handlers in §6 against TS. |
| H3.c (chain opcode silent-abort) | LIVE — pending Stage 1.2 audit | Same; chain includes 7358, 7359, 7357. |
| H3.d (ENUM(105,115,11,...) loader gap) | OFF-PATH | Default-path proc 2130 unreachable for logs since §1 case 212 hit. |

## 8. Stage 1.2 dispatch readiness

Subagent inputs ready: spec, this note, full opcode inventory (§6), full disasm of `[label,light_logs_inv]` and chained helpers (§2-3), wire-order pin (§4). H3.b/H3.c remain LIVE — Stage 1.2 must walk every opcode handler in §6 against `Engine-TS/src/engine/script/handlers/` and bind a single divergent opcode (or class) responsible for the abort.
