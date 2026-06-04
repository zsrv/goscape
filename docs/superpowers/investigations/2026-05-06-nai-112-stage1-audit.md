# NAI-112 Stage 1 — Tutorial-tab-click chatbox-advance audit

**Audit subagent:** Sonnet (Explore, read-only); dispatched per plan §2.2 at HEAD `fbfa0c8`.
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

Goscape (verified by controller at HEAD `fbfa0c8`):
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

---

## Bundle 1b — runtime evidence (smoke 2026-05-06)

User-launched goscape + Java client rev-225 against instrumented HEAD `f76c2da`. User logged in fresh (LOGIN_RESULT_NEW_PLAYER), walked through Tutorial Island chatbox steps (Survival Expert dialog, opcode 235 RESUME_PAUSEBUTTON repeated), then clicked the inventory tab (^tab_inventory=3) when prompted by "Click on the flashing backpack icon to the …".

### Server-boot byKey enumeration (excerpt — 21 global-tier registrations total)

```
INFO NAI-112 instr: byKey global-tier registration key=159 scriptName=[tutorial,_]
INFO NAI-112 instr: byKey global-tier registration key=157 scriptName=[login,_]
INFO NAI-112 instr: byKey global-tier registration key=158 scriptName=[logout,_]
INFO NAI-112 instr: byKey global-tier registration key=165 scriptName=[changestat,_]
... (17 more global registrations omitted)
```

**Verdict: H1 runtime-refuted.** `[tutorial,_]` IS registered at `byKey[159]` post-Provider.Load — exactly as the binary extraction predicted. The script.dat → Decode → Provider.Load pipeline produces the expected registration.

### Tab-click trace (single click, tab=3)

```
DEBUG msg="game packet" opcode=175 name=TUT_CLICKSIDE len=1
INFO  NAI-112 instr: TUT_CLICKSIDE entry tab=3 payloadLen=1
INFO  NAI-112 instr: TUT_CLICKSIDE lookup tab=3 scriptFound=true
```

**Verdict: H2 runtime-refuted.** Java client at rev-225 DOES send opcode 175 with `tab=3` (`^tab_inventory`) on Tutorial-Island inventory-tab click. No tutorial-mode gate on the client side suppresses the dispatch.

**Verdict: H1 + H5 doubly runtime-refuted.** `GetByTriggerSpecific(TriggerTutorial, -1, -1)` returns a non-nil ScriptFile (`scriptFound=true`); the global-tier dispatch finds `[tutorial,_]`.

### Symptom after click

User reports: chatbox message "Click on the flashing backpack icon to the …" remained visible (did NOT advance), and inventory side panel did NOT display.

**No warn or error log fires** in the `[tutorial,_]` execution window (08:13:22.162 to 08:13:23.4). No `script error` or `interpret abort` log appears.

### Bound hypothesis: H6 — `[tutorial,_]` body runs but downstream effect is silently broken

The full pipeline is wired and fires correctly: 175 → handleTutClickSide → GetByTriggerSpecific → script found → runScript invoked. The audit's static H1-H5 refutation is corroborated by runtime evidence on every step. Yet the user-visible effect (advance chatbox + open inventory) does NOT happen.

This narrows the divergence to **inside the `[tutorial,_]` script execution itself or its downstream proc chain**:

- **H6.a** — wrong branch fires: `%tutorial` varp at click time does not match the branch the content expects (varp persistence / save-load divergence between goscape and TS, OR earlier tutorial step left the wrong %tutorial value). User just completed a fresh-account login flow; varp could be uninitialized differently than TS.
- **H6.b** — correct branch fires but the proc it gosubs (`~set_tutorial_progress`, `~tutorial_step_player_controls_left_click`, etc.) has a per-opcode TS-divergence that nullifies the effect (e.g., `tut_open` produces wrong-component-id and silently no-ops, or `if_settab` mis-targets, or a varp-write to %tutorial doesn't propagate to the outbound varp packet).
- **H6.c** — the script runs to completion but the visible "advance chatbox" effect is mediated by `if_close_sub` / `tut_close` / similar that has a goscape-divergence in modal-stack handling.

**Note on H6 directional asymmetry from §H6 above:** the original H6 (TS protect gate absent) was ruled out as wrong-direction. This is a NEW H6 distinct from the protect-gate one — the hypothesis number is reused for the runtime-binding context.

### Stage 2 fix-shape sizing (preliminary)

Stage 2 needs first an observation pass — instrument or statically determine which `%tutorial` value the player has at click time and which branch fires. Then the fix shape depends on which sub-hypothesis (a/b/c) lands:

- H6.a: a single varp-init or save-load fix; ≤20 LOC. Likely involves auditing the post-LOGIN_RESULT_NEW_PLAYER varp seeding path against TS.
- H6.b: per-opcode TS-divergence fix in one of the procs; size depends on which opcode. Range: ≤20 LOC (single opcode) to ~80 LOC (proc chain audit).
- H6.c: modal-stack / outbound-varp dispatch fix; ≤30 LOC.

Stage 2 plan should be its own audit-then-fix sub-spec, not a single-shot fix; the static audit refuted all surface hypotheses, so the next layer is also non-trivial. Per spec §5 LOC guardrail (~80 LOC), Stage 2 should pause for user confirmation if scope creeps past that bound.

---

## Final binding

**H6 — `[tutorial,_]` body runs but downstream effect is silently broken.** Runtime-bound by Bundle 1b smoke 2026-05-06. Stage 2 must first triangulate which sub-hypothesis (H6.a/b/c) by observing `%tutorial` at click time and walking the matched branch's effects against TS.

---

## Stage 2.1 — runtime evidence (smoke 2026-05-06 at HEAD `3b37ffc`)

User-launched smoke against the Bundle 1 instrumentation (`3b37ffc`). Java client rev-225, fresh account (`LOGIN_RESULT_NEW_PLAYER`), walk through Survival Expert dialog to "Click on the flashing backpack icon to the …", click inventory tab.

**Visible client behavior:** chatbox does NOT advance to "Cut down a tree"; inventory side panel does NOT display the bronze axe + tinderbox. Symptom matches NAI-110 close-smoke residual.

**Click event trace (verbatim from server stdout, click at `08:52:00.270`):**

```
08:52:00 INFO TUT_CLICKSIDE entry tab=3 tutorialVarpID=281 tutorialVal=20
08:52:00 INFO TUT_CLICKSIDE lookup tab=3 scriptFound=true
08:52:00 INFO INV_ADD typeID=93 obj=1351 count=1 invResolved=true hasActivePlayer=true
08:52:00 INFO INV_ADD typeID=93 obj=590 count=1 invResolved=true hasActivePlayer=true
08:52:00 INFO SetVarp id=281 prev=20 val=30
08:52:00 INFO IfSetText com=6180 text="Cut down a tree"
08:52:00 INFO IfSetText com=6181 text="You can click on the backpack icon at any time"
08:52:00 INFO IfSetText com=6182 text="to view the items that you currently have in your inventory."
08:52:00 INFO IfSetText com=6183 text="You will see that you now have an axe in your inventory."
08:52:00 INFO IfSetText com=6184 text="Use this to get some logs by clicking on the indicated tree."
08:52:00 INFO OpenTutorial com=6179 prevModalTutorial=6179 lastModalTutorial=6179 willEmitOnEncodeOut=false
08:52:00 INFO TUT_CLICKSIDE postScript tab=3 tutorialValAfter=30
08:52:00 INFO encodeOut TutOpen diff-suppressed modalTutorial=6179 lastModalTutorial=6179
```

**Reading:**

Every script-side effect of the `^newbie_survival_instructor_open_inventory` branch fires correctly:

- `%tutorial == 20` at click time (matches `^newbie_survival_instructor_open_inventory`); H6.a refuted.
- `tab=3` resolved to scriptFound=true; lookup correct.
- `inv_add(inv, bronze_axe=1351, 1)` and `inv_add(inv, tinderbox=590, 1)` both fire on a resolved inv with active player; refutes H6.b for `INV_ADD`.
- `%tutorial = 30` (`^newbie_survival_instructor_cut_tree`); refutes H6.b for `POP_VARP`.
- All five `IfSetText(com=6180..6184)` calls fire with the expected "Cut down a tree" + body strings; refutes H6.b for `IF_SETTEXT` payload encoding.
- `OpenTutorial(6179)` is called (the second call this session at this com — first was at `08:51:31` login emit, with intermediate calls at `08:51:44` / `08:51:47` / `08:51:58` for view_inventory / talk_to_survival).

The chain dies at `encodeOut`'s diff-check at `modules/world/player.go:387-391`: `prevModalTutorial=6179 == lastModalTutorial=6179` → `willEmitOnEncodeOut=false` → `encodeOut TutOpen diff-suppressed`. **No `OpTutOpen` wire packet is emitted to the client.** The Java client receives the IF_SETTEXT updates but no TUT_OPEN re-trigger to flush the overlay redraw, so the chatbox visibly retains the previous "view your inventory" content.

The same diff-suppress shape is observable on every prior `OpenTutorial(6179)` call in this session (08:51:44, 08:51:47, 08:51:58). Only the first emit at login (`OpenTutorial com=6179 prevModalTutorial=-1 lastModalTutorial=-1 willEmitOnEncodeOut=true` at 08:51:31) actually wires; every subsequent call is suppressed.

This matches the H6.c discrimination row exactly: `SetVarp tutorial=30` fires AND `INV_ADD` for both items AND `IfSetText` for "Cut down a tree" + body lines AND `OpenTutorial` second-call `willEmitOnEncodeOut=false` AND `encodeOut TutOpen diff-suppressed` fires.

---

## Stage 2.1 binding (2026-05-06)

**Bound: H6.c** — `Player.OpenTutorial` defers wire emit to `encodeOut`'s `modalTutorial != lastModalTutorial` diff at `modules/world/player.go:387-391`, suppressing every same-com re-open. TS `Player.openTutorial` at `Engine-TS/src/engine/entity/Player.ts:1999-2003` writes `new TutOpen(com)` UNCONDITIONALLY on every call. The Java client requires the TUT_OPEN re-emit to flush the overlay redraw after IF_SETTEXT updates; goscape's diff-suppress strands the UI on the previous chatbox content.

Discriminating signals from smoke trace:

- TUT_CLICKSIDE entry: `tutorialVal=20` (matches `^newbie_survival_instructor_open_inventory`).
- Branch fired: `^newbie_survival_instructor_open_inventory` (identified by `SetVarp id=281 prev=20 val=30` log; `30` = `^newbie_survival_instructor_cut_tree`).
- Effects observed: `INV_ADD bronze_axe(1351)` + `INV_ADD tinderbox(590)`; 5× `IfSetText(com=6180..6184)` with "Cut down a tree" content; `OpenTutorial(com=6179, willEmitOnEncodeOut=false)`.
- TutOpen wire emits: 1 (login `OpenTutorial(6179)` at `08:51:31` with `prevLast=-1`); diff-suppresses: 4 in-session against `lastModalTutorial=6179` (08:51:44, 08:51:47, 08:51:58, 08:52:00) plus per-tick suppress noise.

Stage 2.2 fix routes to: **Task 4c** of plan `2026-05-06-nai-112-stage2-tutorial-tab-click-fix.md` — move the wire emit out of `encodeOut` and into `Player.OpenTutorial` / `Player.CloseTutorial` directly, mirroring TS `Player.ts:1999-2003` / `:716-726`.

---

## Stage 2.2 fix shipped + first final smoke (2026-05-06 at HEAD `b362d9b`)

Stage 2.2 fix landed at `241511d` (TS-fidelity reorder fixup at `aa4fe03`) and Bundle 3 instrumentation revert at `b362d9b`.

**First final smoke against `b362d9b`:** symptom unchanged. Both chatbox-advance and inventory-side-panel still broken to the user. With instrumentation reverted, the trace was uninformative — TUT_CLICKSIDE arrived but no logs disambiguated whether the script ran, whether the OpenTutorial fix path was hit, or whether the inventory-side issue was downstream of TUT_OPEN re-emit.

Per `smoke_unchanged_means_multiple_blockers`: re-open Stage 2.1 with smoke evidence. New disambiguation: H6.c-α (client ignores duplicate `TUT_OPEN(com)`), H6.c-β (separate inventory divergence per the original spec note), H6.c-γ (fix didn't land at runtime).

---

## Bundle 1.5 instrumentation + second final smoke (2026-05-06 at HEAD `b5165d6`)

Re-instrumented four sites for one disambiguation cycle: `handleTutClickSide` entry/lookup/postScript; `Player.OpenTutorial` post-writeOut; `Player.CloseTutorial` post-writeOut; `Player.IfSetTab` entry. Reverted at `8eb6989` after the smoke trace bound the residual.

**Second final smoke (click event at `09:46:56.611`):**

```
TUT_CLICKSIDE entry tab=3
TUT_CLICKSIDE lookup tab=3 scriptFound=true
OpenTutorial wired com=6179 prevModalTutorial=6179
TUT_CLICKSIDE postScript tab=3
```

**Visible client behavior:** chatbox advanced to "Cut down a tree" ✓; inventory side panel did NOT display bronze axe + tinderbox.

**Three-way binding:**

- **H6.c-γ refuted** — `OpenTutorial wired` log fires post-click with `prevModalTutorial=6179` (the exact duplicate-com case the pre-fix `encodeOut` diff would have suppressed). The Stage 2.2 fix is engaged at runtime.
- **H6.c-α refuted** — chatbox visibly advanced. The Java client DOES redraw the tutorial overlay on a duplicate `TUT_OPEN(com)`. The H6.c diagnosis was correct; the H6.c fix works for the chatbox-advance symptom.
- **H6.c-β confirmed** — bronze axe + tinderbox don't display, but `OpenTutorial wired` + `TUT_CLICKSIDE postScript` both fired. The trace shows `IfSetTab com=3213 tab=3` at `09:46:55` (during pre-click `tutorial_step_view_inventory` setup) had already bound tab=3 to the inventory com. The `[tutorial,_]` `cut_tree` branch correctly does NOT re-bind tab=3 (matches TS). So the inventory-tab BINDING is fine; the inventory CONTENTS aren't reaching the client. This is a separate engine-layer divergence in goscape's inventory wire-sync path (likely the post-`Inventory.add()` UPDATE_INV* emit; or a missing `inv_listener`/`inv_open` style binding setup; or a tutorial-mode-specific render gate).

---

## Final binding (revised)

**NAI-112 PRIMARY (chatbox-advance / `[tutorial,_]` branch dispatch correct, TUT_OPEN wire suppressed): H6.c — TUT_OPEN unconditional re-emit divergence.** Smoke-confirmed at HEAD `b362d9b` second smoke 2026-05-06 → chatbox advances to "Cut down a tree" after inventory-tab click.

**NAI-112 SECONDARY (inventory side panel doesn't display): H6.c-β — separate inventory wire-sync divergence.** Surface unknown at this stage; needs its own brainstorm + instrumentation pass on the `inv_add` → UPDATE_INV* path. Routes to **NAI-113** as a fresh sub-spec — not downstream of the H6.c TUT_OPEN fix; not a cascade residual; an independent gap surfaced by the same user-visible symptom.

Per `dispatch_correct_reach_blocked` shape: PRIMARY (TS-faithful port) closes here; SECONDARY (content outcome) routes to NAI-113.
