# rev-254 Pin-Advance Addendum Plan (Engine-TS 43e02957 → 2e3bcf43)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port the post-pin Engine-TS delta `43e02957..2e3bcf43` (200 files, +4506/−3655) on top of the completed Phases 1-2, then run the original plan's Phases 3-4 (pack/unpack) against the new pin.

**Context:** The original rev-254 plan (`2026-06-10-rev254-port.md`) executed Tasks 1-16 at engine pin `43e02957`. That pin cannot pack the pinned Content (midi dbtable type); the user advanced the engine pin to `2e3bcf43` (spec amendment `530ba805`, main pins `c2100894`). Verified across the advance: client/server/zone wire tables byte-identical; `ENGINE_REVISION` 254 unchanged — T2-T16 stand. The reference cache at the new pin builds clean (T17 DONE: bun 1.2.20, `@lostcityrs/runescript@0.9.6` in-process compiler, script.dat 5.3MB version-27 header, `server/varbit.dat` present, **0 .sym files**, log `/tmp/claude-1000/rev254-ref-pack.log`, 2 benign missing-model warnings).

**All TS citations** refer to `2e3bcf43` unless tagged. Same conventions as the original plan (git show only; --no-gpg-sign; `[rev-254]` suffix; TDD; foreground tests; tests pinning superseded contracts get updated).

**Verified-unchanged at the new pin (do NOT re-derive):** the 9 config CRCs incl. varbit `-1387031023`; interface CRC `1728499832`; sound CRC `831919863`; interface script ops 14-20; varbit pack shapes (client code 1 / server code 250); loc model2-4/raiseobject/code-5 fork; npc turnspeed; VarbitPack registration; per-script binary format in script.dat (only the header version moves 26→27).

---

## Phase 1b — post-pin engine delta

### Task A1: Script-op table regen at 2e3bcf43 + handler renames/removals + pointer re-sync

The enum restructures 418→396: 231 values move; renames (BAS_READYANIM→READYANIM, BAS_RUNNING→RUNANIM, BAS_TURNONSPOT→TURNANIM, BAS_WALK_F→WALKANIM, BAS_WALK_B/L/R→WALKANIM_B/L/R, HINT_PLAYER→HINT_PL, LOWMEMORY→LOWMEM, IF_SETRESUMEBUTTONS→IF_ADDRESUMEBUTTON — verify each is rename vs behavior change by diffing the handlers); additions (MAP_LIVE 1011, MIDI_LENGTH 1022, HINT_PL 2029, IF_ADDRESUMEBUTTON 2047, NPC_HASOP — read each handler at the pin); removals (NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT, BUFFER_FULL, IF_MULTIZONE, PLAYER_FINDALLZONE/FINDNEXT, IF_OPENMAINOVERLAY, LAST_COORD, NPC_HUNTNEXT, MAP_PRODUCTION, the 12 MAP_LAST* — for each, check whether goscape has a handler to delete and whether any content script references it). ScriptOpcodePointers diff: FINDUID gains corrupt rows; **NPC_FINDHERO/OBJ_ADD/OBJ_TAKEITEM rows REVERT to their pre-254 shapes** (undoing original-plan Task 9's changes — transcribe the diff exactly); NPC_HUNTNEXT row deleted; pointer count changes. ScriptRunner: error message uses opcode NAME; player error logging username-not-pid; debug-frame loop `i >= 0`. Regenerate `opcode_map_254_pin_test.go` from the new enum (396 entries) and the pointers pin. The compiler (pkg/pack/compiler) reads ScriptOpcodeMap dynamically — pack tests gate fallout. SET_PLAYER_OP/STAT_TOTAL/PUSH_VARBIT/POP_VARBIT survive — verify their values at the new enum.

### Task A2: pid → slot + player-loop restructure

TS replaces PlayerList with `players: Player[2048]` + `playerLoop: HashTable<Player>` bucketed by IP (IPv4 full addr; IPv6 site-prefix mod 256); `player.pid` → `player.slot` (uid calc `((username37 & 0x1fffff) << 11) | slot`); `getNextPlayerSlot()` linear 1..2046; rsbuf playerInfo/npcInfo calls take slot. Map onto goscape's pid machinery (modules/world players registry, rsbuf calls, UpdatePid packet, hintPlayer slot+32768). Read TS World.ts:170-210/870-920 + Player.ts:305-320 first; distinguish true behavior (slot assignment order, iteration order effects on tick processing!) from refactor. Iteration-order changes are behavioral for goscape parity — pin the new assignment behavior.

### Task A3: Session-UUID identity + friends-relay/logging changes

`account_id` field → `session` (client UUID, default 'headless'; set at NetworkPlayer ctor); addSessionLog/addWealthEvent signatures drop account_id; logPublicChat sends session_uuid; the friends `world_heartbeat` relay message removed (only player_autosave). Map onto goscape's logger bridge + session plumbing (p.session exists from the InputTracking work — extend).

### Task A4: Login rate-limiting + Environment deltas

TTL caches: address attempts (NODE_RATELIMIT_ADDRESS_LOGIN default 30, 60s window) and device attempts (NODE_RATELIMIT_DEVICE_LOGIN default 5, 15s window, uid+ip key); exceeded → reply byte 16 + close; NODE_HOP_TIME (45000). Environment removals: STANDALONE_BUNDLE, WEB_SOCKET_TOKEN_PROTECTION, BUILD_JAVA_PATH, BUILD_STARTUP_UPDATE (goscape analogs: grep config for any mirrors — likely none). Read TS World.ts onLoginCycle at the pin for the exact reply codes and window semantics; goscape's login flow is in modules/world server.go handleLogin.

### Task A5: InputTracking v2

At 2e3bcf43: `softLimit = 1500` (was max=500); `seq` REMOVED; overflow checks against actual buffer capacity (`buf.pos + N >= buf.length`) not the soft limit — read the file fully, the semantics differ per event; flush calls `World.submitInputTracking(player, rawBytes)` (InputTrackingBlob.ts DELETED — blob assembly moves to the logger receiver). Reworks T5's modules/world/input_tracking.go + the logger-bridge submit signature. Update the T5 byte-layout pins (layouts unchanged; thresholds/signature change).

### Task A6: Sparse varp save encoding (SAVE-FORMAT CHANGE — migration landmine class)

TS Player save now writes count of non-zero SCOPE_PERM varps then (p2 id + pVarInt value) pairs, replacing the dense p4-per-varp block. CHECK THE SAVE VERSION at the pin (PlayerLoading/Player save header) — mirror exactly how TS versions the format and loads OLD saves; goscape must load both dense (existing local saves) and sparse per the same version gate. Read TS PlayerLoading.ts at the pin IN FULL before touching the Go load path (lesson f4334477: registry-sized allocation stays; only the overlay loop changes). Also port pVarInt if goscape's packet lacks it.

### Task A7: Movement/pathing delta (upstream f0ccbe8a + 2787f1fb + d39e707d)

MoveRestrict leaves PathingEntity (NPCs read NpcType.moverestrict on demand; players always NORMAL); `moveStrategy` field (NAIVE for NPCs; player NAIVE iff NODE_CLIENT_ROUTEFINDER else SMART); `naivePathToTarget()`; `AllowRepath` enum + repath-at-last-waypoint gating; walktrigger condition now `NODE_CLIENT_ROUTEFINDER && !followOp`; `setFaceEntity()` runs AFTER processMovementInteraction for both Player and Npc; `isLastOrNoWaypoint` → `isLastWaypoint` semantics; Npc overlap → `randomWalk()`; retreat logic (d39e707d). This is the highest-risk task — goscape's pathing was ported line-carefully across many arcs. Read the three upstream commits individually (`git show f0ccbe8a`, `2787f1fb`, `d39e707d`) plus the net diff; port commit-by-commit with pins per behavior.

### Task A8: Npc delta — regenInterval, huntAll(hunt), wander rename

regen clock counts UP against a cached regenInterval refreshed from NpcType each expiry (type-change mid-life respected); `huntAll(hunt)` takes the HuntType (caller fetches; undefined-guard); `randomWalk(range)` → `wander(range)` naming; interaction-clear drops the manual faceEntity mask writes (setFaceEntity owns it). Map onto modules/world npc.go/npc_hunt.go/npc_regen sites.

### Task A9: Modal/resume-buttons + interface method renames

`resumeButtons` array on Player; cleared in cleanup + on every modal open when the active script is in COUNTDIALOG/PAUSEBUTTON; IF_ADDRESUMEBUTTON op (from A1) populates it; RESUME_PAUSEBUTTON handling validates against it (read the handler at the pin). TS method renames openOverlay→openMainOverlay etc. — follow goscape's naming only where it reduces TS-mapping friction (Go names are Go-side; note the mapping in comments).

### Task A10: MIDI runtime — Midi cache, id-based audio, MIDI_LENGTH

New `src/cache/midi/Midi.ts` (loads midi lengths from the cache; `getLength(id)` centiseconds) loaded at World.start; `playSong(id)`/`playJingle(id)` take ids (name→id moves to the script layer — check which script ops/cheats do the lookup now and what goscape's midi name registry should do); MIDI_LENGTH script op; ScriptVarType MIDI=77 runtime side. Read Midi.ts fully (what cache file does it read — the packed midi index?).

### Task A11: Misc engine sweep

Queue iteration cursor-removal semantics (verify goscape's queue processing matches the `.all()` iteration behavior — RE-READ the TS diff hunks; if goscape never had the cursor quirk, record no-op); `buildAppearance(this.appearanceInv)`; AFK_CHANCE1/2 constants + afkEventReady timing move; NetworkPlayer pad-byte `pos += N` (check Go equivalence); World helper renames; varbit explicit-parens hunk (no-op — Go already groups correctly, verify); anything in the net diff not covered by A1-A10 — walk `git diff 43e02957..2e3bcf43 --stat -- src` and assign every file a disposition (ported-in-task-X / no-op / NOT-PORTED with reason).

### Phase 1b exit gate

Build/vet/full suite/`-race` per the original plan's Phase-1 gate.

---

## Phase 3 (amended) — pack pipeline at the new pin

- **A12 (was T18):** Varbit pack family — unchanged spec (facts re-verified at 2e3bcf43).
- **A13 (was T19/T20/T21):** Loc/npc packers, interface ops 14-20 + CRC, config+sound CRCs, maps comment-skip — unchanged specs; ALSO the maps Pack.js tokenization restructure (verify Go parser equivalence).
- **A14 (NEW): dbtable/dbrow midi type** — ScriptVarType MIDI=77 ('M') in the Go pack type-char map; dbrow value lookup → midi pack id; getTypeChar-null → pack error (TS DbTableConfig.ts hunks at 2dc4a811); REQUIRED to pack Content 254 (the original blocker). Plus the DbRow/DbTable improved validation errors.
- **A15 (NEW): jagFileVersion 26 → 27** in pkg/pack/compiler/runescript/jag_file_writer.go (+tests). The Arc-26 REFERENCES condition is met: `@lostcityrs/runescript@0.9.6`; the reference script.dat header reads 27 (verified at offset 4-7). pkg/script/file.go needs no change (per-script format identical); check any Go-side version gate on script.dat reads.
- **A16 (NEW): symbols-gate re-scope** — upstream produces NO .sym files at the new pin (CompilerSymbols.ts deleted; symbols are in-memory CompilerTypeInfo). Decision (controller-approved): KEEP goscape's .sym export as a documented Go-only feature; RETIRE `symbols_export_ref_parity_test.go`'s upstream-reference gate (no baseline exists) — replace with a self-consistency/golden test or delete with a PORTING-EXCEPTION note; packall full-tree parity excludes data/symbols.
- **A17 (was T22/T23):** env rename → GOSCAPE_REF254_DIR + full-tree parity vs the new reference cache (script.dat now compared against the npm-compiler output — the Go compiler's byte-parity target changes from RuneScriptKt-26-jar output to @lostcityrs/runescript 0.9.6 output; expect the version byte plus whatever 0.9.6 changed beyond 750291c — byte-diff loop as usual).
- **T24/T25:** unchanged (data refresh, live smoke).

## Phase 4 — unchanged

T26-T29 as originally planned (modelRenameOffset facts unchanged at the new pin; bmfont-parser/bmfont are authoring tools outside the 16 families — confirm during T28). T30 close-out audits `3c16994c..2e3bcf43` + the rsbuf crate diff.

---

## Execution notes

- Original-plan task numbering continues (harness tasks); A-tasks get inserted before the remaining T18+ work.
- The two delta-map exploration reports (this session) are starting points, NOT verified specs — every implementer verifies against `git show 2e3bcf43:<path>` per the standing convention; both reports' line numbers are approximate.
- Reviewer hallucination precedent this session: one quality reviewer fabricated a TS guard (LocType postDecode) — spec reviewers MUST quote TS verbatim for any disputed contract.
