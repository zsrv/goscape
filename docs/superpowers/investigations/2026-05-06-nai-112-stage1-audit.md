# NAI-112 Stage 1 — Tutorial-tab-click chatbox-advance audit

**Audit subagent:** Sonnet (Explore, read-only); dispatched per plan §2.2 at HEAD `ab52a7d`.
**Controller HEAD-verification:** complete; all goscape file:line citations and TS file:line citations re-read at HEAD; binary `script.dat` independently re-extracted (Python) confirming the audit's `[tutorial,_]` LookupKey=159 claim.

---

## TS reference chain summary

### TutClickSideHandler.ts shape

`Engine-TS/src/network/game/client/handler/TutClickSideHandler.ts:8-23` — `handle()` reads `tab` from the message, gates `tab < 0 || tab > 13` (returns false), then calls `ScriptProvider.getByTriggerSpecific(ServerTriggerType.TUTORIAL, -1, -1)` (line 16). If the script is found, calls `player.executeScript(ScriptRunner.init(script, player), true)` (line 18) — **no tab argument** passed to `ScriptRunner.init`, which defaults `args` to `[]` (empty). No additional pre-dispatch gates beyond the 0-13 range check.

### LookupKey derivation for `[tutorial,_]`

The compiler's `generateLookupKey` (in `@lostcityrs/runescript/dist/runescript.js`, bundled): `trigger.subjectMode === L.Name → key = -1`; else `key = trigger.id`. `TUTORIAL` is `{id: 159, name: "TUTORIAL", subjectMode: L.None, ...}`. `L.None` is not `L.Name`, has no `"type"` property, so the `if ("type" in R && U != null)` branch is skipped. Result: `LookupKey = 159 = uint32(TriggerTutorial)`.

**Independently verified by binary inspection** of `data/pack/server/script.dat`: script `[tutorial,_]` at id=8023, size=1062, LookupKey=`0x0000009f`=159. Header bytes: `5b7475746f7269616c2c5f5d00...` (`[tutorial,_]\0...`), then SourceFile NUL-terminated, then `00 00 00 9f` (LookupKey).

### `[tutorial,_]` body opcode list

From `Server/content/scripts/tutorial/scripts/tutorial.rs2:143-176` cross-verified against compiled binary at id=8023. 11 distinct opcodes:

- `PUSH_CONSTANT_INT` (0)
- `PUSH_VARP` (1) — read `%tutorial`
- `POP_VARP` (2) — write `%tutorial`
- `BRANCH` (6)
- `BRANCH_EQUALS` (8)
- `RETURN` (21)
- `GOSUB_WITH_PARAMS` (40) — `~tutorial_step_player_controls_left_click` (id=7935 at rs2:146); `~set_tutorial_progress` (id=8024 at rs2:176)
- `JUMP_WITH_PARAMS` (41) — `@newbie_magic_instructor_opened_tab` (id=7816, only in `^tutorial_open_magic_tab` branch)
- `HINT_NPC` (2028)
- `NPC_FIND` (2513)
- `INV_ADD` (4302) — `inv_add(inv, bronze_axe, 1)` and `inv_add(inv, tinderbox, 1)` (only in `^newbie_survival_instructor_open_inventory` branch)

**No `getarg` / `PUSH_INT_ARG` reads** anywhere in the body (`int_arg_count=0`, `str_arg_count=0` in binary trailer). H4 refuted from binary.

### Client-Java sidebar dispatcher

`Client-Java/src/main/java/deob/client.java`. Opcode 175 is sent at line 4749: `this.out.p1isaac(175)` followed by `this.out.p1(this.selectedTab)` — **opcode 175 IS sent**. Mechanism: server sends packet 126 → `flashingTab = in.g1()` (line 9855). On subsequent render-tick, `if (flashingTab != -1) redrawSideicons = true` (line 4744). Inside `redrawSideicons` block (line 4746-4751): `if (flashingTab != -1 && flashingTab == selectedTab)` → sends opcode 175 + selectedTab byte, sets `flashingTab = -1`. User clicking inventory tab via `handleTabInput()` (line 8243-8249) sets `selectedTab = 3` (^tab_inventory=3) and `redrawSideicons = true`, triggering the condition. No tutorial-mode gate on `handleTabInput()`. H2 refuted.

### Goscape pack-server / compile path

Goscape consumes a **pre-built `script.dat`** at `data/pack/server/script.dat` (4329268 bytes, 8032 entries, version 26). There is no in-process RuneScript compiler in goscape. The LookupKey for `[tutorial,_]` is written by the external `@lostcityrs/runescript` npm package, which produced LookupKey=159 (binary-verified). Goscape's `Decode` (`pkg/script/file.go:88`) reads this as `f.LookupKey = pkt.G4()` (uint32, BE) = 159. `Provider.Load` (`pkg/script/provider.go:100-101`) registers `p.byKey[159] = f` since `159 ≠ 0xFFFFFFFF`.

---

## Per-hypothesis verdict

### H1 — `[tutorial,_]` not under byKey[159]

**Verdict: REFUTED**

Evidence:
- `data/pack/server/script.dat` direct extraction (Python big-endian read): script name `[tutorial,_]` at id=8023, LookupKey=`0x0000009f`=159. Independently re-verified by controller.
- TS compiler `generateLookupKey` (runescript.js): `TUTORIAL.subjectMode = L.None` ≠ `L.Name` → `key = id = 159`; `"type" not in R` → no type/category bits added → `key = 159`.
- Goscape `Decode` (pkg/script/file.go:88): `f.LookupKey = pkt.G4()` reads 159. `Provider.Load` (provider.go:100): `p.byKey[159] = f`.
- `GetByTriggerSpecific(TriggerTutorial=159, -1, -1)` (provider.go:152): returns `p.byKey[uint32(159)]` = the `[tutorial,_]` script.
- Arithmetic: `LookupKeyForGlobal(TriggerTutorial)` = `uint32(159)` (lookup_key.go:18). Binary-LookupKey == lookup-key-for-global-trigger == 159. Exact match.

Fix-shape size: N/A.

### H2 — Java client doesn't send opcode 175

**Verdict: REFUTED**

Evidence:
- `Client-Java/.../deob/client.java:4749`: `this.out.p1isaac(175)`; line 4750: `this.out.p1(this.selectedTab)`. Sent unconditionally when `flashingTab != -1 && flashingTab == selectedTab` in the `redrawSideicons` block.
- `client.java:8243-8249`: `handleTabInput()` sets `selectedTab = 3` (inventory) and `redrawSideicons = true`. No tutorial-mode gate.
- `client.java:9855`: server pkt 126 (`TUT_FLASH`) sets `flashingTab`. Goscape sends this via `FlashTutorial(tab)` → opcode 126.
- `pkg/io/protocol/game/client/prot.go:101`: `set(175, "TUT_CLICKSIDE", 1, u)` — 1-byte payload matches `p1(selectedTab)`.

Fix-shape size: N/A.

### H3 — downstream opcode aborts in `[tutorial,_]` body

**Verdict: REFUTED (for all branches in the body)**

Evidence — opcode coverage cross-check against `pkg/script/handlers.go`:

| Opcode | Name | Registered |
|---|---|---|
| 0 | PushConstantInt | handlers.go:14 |
| 1 | PushVarp | handlers.go:184 |
| 2 | PopVarp | handlers.go:185 |
| 6 | Branch | handlers.go:21 |
| 8 | BranchEquals | handlers.go:23 |
| 21 | Return | handlers.go:16 |
| 40 | GosubWithParams | handlers.go:30 |
| 41 | JumpWithParams | handlers.go:324 |
| 2028 | HintNpc | handlers.go:434 |
| 2513 | NpcFind | handlers.go:390 |
| 4302 | InvAdd | handlers.go:278 |

All 11 body-level opcodes are wired. Audit walked downstream proc opcodes (Switch, SplitInit, SplitPageCount, SplitLineCount, SplitGet, IfSetText, TutOpen, InvTransmit, IfSetTab, TutFlash, HintStop, HintCoord) and found all registered. No NAI-110/NAI-109-shape "constant declared with no handler" gap exists for this body.

Fix-shape size: N/A.

### H4 — runScript args mismatch

**Verdict: REFUTED**

Evidence:
- `TutClickSideHandler.ts:18`: `ScriptRunner.init(script, player)` — third arg `args` defaults to `[]` (`ScriptRunner.ts:66`).
- Goscape `handler_interface.go:147`: `runScript(sf, p, nil, true, nil, nil)` — `intArgs=nil`, `stringArgs=nil` (empty).
- Binary-confirmed: `[tutorial,_]` has `int_arg_count=0, str_arg_count=0`. No `getarg`/`PUSH_INT_ARG` opcodes in body.

Fix-shape size: N/A.

### H5 — GetByTriggerSpecific too narrow vs TS dispatch

**Verdict: REFUTED**

Evidence:
- `TutClickSideHandler.ts:16`: `getByTriggerSpecific(ServerTriggerType.TUTORIAL, -1, -1)` — single-tier global lookup.
- `ScriptProvider.ts:147-153`: with `(type=-1, category=-1)` returns `scriptLookup.get(trigger)` = global key=159.
- Goscape `provider.go:145-152`: `GetByTriggerSpecific(TriggerTutorial, -1, -1)` returns `p.byKey[uint32(trigger)]` = `p.byKey[159]`.
- Identical single-tier global dispatch on both sides.

Fix-shape size: N/A.

### H6 — TS protect gate absent in goscape (asymmetric behavior)

**Verdict: INCONCLUSIVE but not credible cause**

Evidence: TS `Player.runScript` gates `if (!force && protect && (this.protect || this.delayed)) { return -1; }`. Goscape `runScript` (modules/world/script.go:99-103) has no equivalent gate. Asymmetry direction: goscape RUNS scripts when TS would SKIP — i.e., over-fires, not under-fires. The observed symptom is under-firing. H6 cannot explain the symptom; flagged for future tracking but not bound here.

---

## Recommended binding

**Two+ plausible — Bundle 1b instrumentation needed.**

All five catalogued hypotheses H1-H5 are statically refuted by binary inspection and TS code reading. No credible H6 surfaced. The symptom (silent non-advance + no warn log) is consistent with the `[tutorial,_]` script not firing OR firing without observable effect, but every static check confirms the pipeline is correctly wired end-to-end.

**Bundle 1b instrumentation plan** (per plan Task 5):
1. `modules/world/handler_interface.go:138-149`: log entry payload (`tab`) + lookup result (`sf == nil`) at `handleTutClickSide`.
2. `pkg/script/provider.go:103-105`: enumerate global-tier `byKey` registrations (key < 256) at `Provider.Load` end, including key 159.

User-launched smoke disambiguates:
- **No "TUT_CLICKSIDE entry" line** → packet routing issue (handler not called); promotes a new H7 (handler-table mis-registration or session-state gate upstream).
- **`scriptFound=false`** → contradicts binary; points at `Provider.Load` not consuming the cache or `byKey` being cleared between Load and lookup.
- **`scriptFound=true` + chatbox unchanged** → script does fire but downstream effect (TutOpen / IfSetTab / chatbox progression) is silently broken; promotes H8 (downstream-effect divergence — possibly in the `set_tutorial_progress` proc chain).

---

## "Verified at HEAD" claims for controller spot-check

Goscape (verified by controller at HEAD `ab52a7d`):
- `modules/world/handler_interface.go:138-149` — `handleTutClickSide`: `GetByTriggerSpecific(TriggerTutorial, -1, -1)`; `runScript(sf, p, nil, true, nil, nil)`; gate `tab < 0 || tab > 13`. ✅
- `pkg/script/provider.go:145-153` — `GetByTriggerSpecific(t, -1, -1)` returns `byKey[uint32(trigger)]` (global tier; no fallback). ✅
- `pkg/script/provider.go:100-102` — `if f.LookupKey != 0xFFFFFFFF { p.byKey[f.LookupKey] = f }`. ✅
- `pkg/script/lookup_key.go:18-20` — `LookupKeyForGlobal(t) = uint32(t)`. ✅
- `pkg/script/trigger.go:164` — `TriggerTutorial = 159`. ✅
- `pkg/script/file.go:88` — `f.LookupKey = pkt.G4()` (BE u32). ✅
- `modules/world/handlers_game.go:84` — `gameHandlers[175] = handleTutClickSide`. ✅
- `modules/world/handlers_game.go:123-128` — wrapper `handleTutClickSide(p, payload) → p.client.server.handleTutClickSide(p, payload)`. ✅
- `pkg/io/protocol/game/client/prot.go:101` — `set(175, "TUT_CLICKSIDE", 1, u)`. ✅

Binary (controller-reproduced via Python):
- `data/pack/server/script.dat`: entry_count=8032, version=26; script `[tutorial,_]` at id=8023, size=1062, LookupKey=`0x0000009f`=159. ✅

External (controller-reread):
- `Engine-TS/src/network/game/client/handler/TutClickSideHandler.ts:8-23` — handle() body as quoted; line 16 `getByTriggerSpecific(TUTORIAL, -1, -1)`; line 18 `executeScript(ScriptRunner.init(script, player), true)`. ✅
- `Engine-TS/src/engine/script/ScriptProvider.ts:147-153` — `getByTriggerSpecific` with `(-1,-1)` returns `scriptLookup.get(trigger)`. ✅

LookupKey arithmetic (controller-recomputed):
- `trigger=TUTORIAL=159`, `subjectMode=L.None` (not `L.Name`), no `"type"` key → `key = id = 159`. Bit-equivalent to goscape's `LookupKeyForGlobal(159) = 159`. Binary header confirms `00 00 00 9f`. ✅

---

## Controller verdict

All audit-claimed evidence verified at HEAD. The static refutation of H1-H5 stands. **Proceeding to Bundle 1b instrumentation** per plan Task 5.
