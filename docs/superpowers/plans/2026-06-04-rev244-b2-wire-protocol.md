# rev-244 Bundle 2: wire protocol + rsbuf — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the B2 slice of the Engine-TS 225→244 delta (`e1dea19f..9aadcec4 -- src/network`, 115 files) plus the `@2004scape/rsbuf` crate delta (`225` branch → `origin/244` tip `1defefb`, the damage2 commit), with the damage2 entity feed pulled forward from B3.

**Architecture:** Faithful TS→Go translation per `PORTING-LESSONS.md` (read first: `git show main:PORTING-LESSONS.md` — §3 gotchas, §4 citations, §5 gates). Three slices in dependency order: opcode tables → handler family → rsbuf damage2. Each task slices the cross-pin diff to one file group; the TS diff is the contract. All work lands on branch `rev-244`.

**Tech Stack:** Go 1.26. Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix. Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: `-race` on touched packages (CGO_ENABLED=1). Every commit: `--no-gpg-sign`.

**Spec:** `docs/superpowers/specs/2026-06-04-rev244-b2-wire-protocol-design.md`

**References:**
- Engine-TS at the 244 pin: `/home/owner/Code/github.com/LostCityRS/Engine-TS` (checkout IS at `9aadcec4`)
- rsbuf Rust source: `/home/owner/Code/github.com/2004scape/rsbuf` — 225 baseline = branch `225`; 244 target = `origin/244` (tip `1defefb`, verified identical to the published npm `244.1.0` dist)

**Scope decisions already made (do not relitigate):**
- damage2 entity hunks (PathingEntity.ts:92-96,606-610 / Player.ts:1870-1890 / Npc.ts:475-494) are pulled FORWARD from B3 (user-approved). B3 must NOT double-apply — Task 12 records the decision rows.
- rsbuf depth = delta-port + targeted spot-check of surrounding logic in touched files (user-approved). Not a full line re-audit.
- goscape's `Ops [256]` stays opcode-keyed; the TS `index` ctor field has zero readers at the pin → NO-OP decision row.
- `handlers_game.go` registration switches to named opcode constants (approved structural improvement).
- Known windows: map-delivery (DATA_* removed here, OnDemand lands B3) and midi-id (MIDI_* sends need MidiPack, lands B3). Both get PORTING.md rows; end-to-end client smoke is gated "after B2+B3" by the umbrella spec.
- Sandbox gotcha: `git status` shows phantom `??` dotfiles (`.bashrc`, `.gitconfig`, …) — device-node masks, NOT real files. Never stage them; never `git add -A`. Warn every subagent.

---

## Slice 1 — opcode tables

### Task 1: Client prot table — 244 renumber + named constants + registration

**Files:**
- Modify: `pkg/io/protocol/game/client/prot.go`
- Modify: `pkg/io/protocol/game/client/prot_test.go`
- Modify: `modules/world/handlers_game.go` (registration block, `init()` at :35-115)

TS contract: `git -C /home/owner/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:src/network/game/client/ClientGameProt.ts` — read it in full. The 244 ctor is `(index, opcode, size)`; the `index` (NXT packet index) is only written into `ClientGameProt.all`, never read → goscape does NOT model it (decision row in Task 12).

Removed at 244: `REBUILD_GETMAPS` (was 150/-1), `EVENT_CAMERA_POSITION` (was 189/6). Renamed: `IDK_SAVEDESIGN`→`IF_PLAYERDESIGN`, `TUT_CLICKSIDE`→`TUTORIAL_CLICKSIDE`. Size change: `INV_BUTTOND` 6→7 (new trailing `mode` byte — handler reads it in Task 6).

The complete 244 table (name, opcode, size — categories carry over from the current goscape table by name; they live on TS model classes, unchanged in the delta):

| name | op | size | cat | | name | op | size | cat |
|---|---|---|---|---|---|---|---|---|
| NO_TIMEOUT | 107 | 0 | c | | OPHELD1 | 228 | 6 | u |
| IDLE_TIMER | 146 | 0 | c | | OPHELD2 | 166 | 6 | u |
| EVENT_TRACKING | 217 | -2 | r | | OPHELD3 | 221 | 6 | u |
| ANTICHEAT_OPLOGIC1 | 47 | 4 | c | | OPHELD4 | 6 | 6 | u |
| ANTICHEAT_OPLOGIC2 | 218 | 4 | c | | OPHELD5 | 133 | 6 | u |
| ANTICHEAT_OPLOGIC3 | 37 | 3 | c | | OPHELDT | 143 | 8 | u |
| ANTICHEAT_OPLOGIC4 | 34 | 2 | c | | OPHELDU | 58 | 12 | u |
| ANTICHEAT_OPLOGIC5 | 7 | 0 | c | | INV_BUTTON1 | 153 | 6 | u |
| ANTICHEAT_OPLOGIC6 | 177 | 4 | c | | INV_BUTTON2 | 193 | 6 | u |
| ANTICHEAT_OPLOGIC7 | 50 | 4 | c | | INV_BUTTON3 | 158 | 6 | u |
| ANTICHEAT_OPLOGIC8 | 100 | 2 | c | | INV_BUTTON4 | 204 | 6 | u |
| ANTICHEAT_OPLOGIC9 | 169 | 1 | c | | INV_BUTTON5 | 212 | 6 | u |
| ANTICHEAT_CYCLELOGIC1 | 46 | 1 | c | | IF_BUTTON | 39 | 2 | u |
| ANTICHEAT_CYCLELOGIC2 | 148 | -1 | c | | RESUME_PAUSEBUTTON | 11 | 2 | u |
| ANTICHEAT_CYCLELOGIC3 | 144 | 3 | c | | CLOSE_MODAL | 187 | 0 | u |
| ANTICHEAT_CYCLELOGIC4 | 41 | 4 | c | | RESUME_P_COUNTDIALOG | 190 | 4 | u |
| ANTICHEAT_CYCLELOGIC5 | 232 | 0 | c | | TUTORIAL_CLICKSIDE | 233 | 1 | u |
| ANTICHEAT_CYCLELOGIC6 | 215 | -1 | c | | MOVE_OPCLICK | 167 | -1 | u |
| OPOBJ1 | 231 | 6 | u | | REPORT_ABUSE | 251 | 10 | u |
| OPOBJ2 | 110 | 6 | u | | MOVE_MINIMAPCLICK | 56 | -1 | u |
| OPOBJ3 | 27 | 6 | u | | INV_BUTTOND | 81 | 7 | u |
| OPOBJ4 | 17 | 6 | u | | IGNORELIST_DEL | 207 | 8 | u |
| OPOBJ5 | 225 | 6 | u | | IGNORELIST_ADD | 203 | 8 | u |
| OPOBJT | 25 | 8 | u | | IF_PLAYERDESIGN | 8 | 13 | u |
| OPOBJU | 111 | 12 | u | | CHAT_SETMODE | 98 | 3 | u |
| OPNPC1 | 222 | 2 | u | | MESSAGE_PRIVATE | 170 | -1 | u |
| OPNPC2 | 84 | 2 | u | | FRIENDLIST_DEL | 69 | 8 | u |
| OPNPC3 | 132 | 2 | u | | FRIENDLIST_ADD | 9 | 8 | u |
| OPNPC4 | 229 | 2 | u | | CLIENT_CHEAT | 76 | -1 | u |
| OPNPC5 | 102 | 2 | u | | MESSAGE_PUBLIC | 171 | -1 | u |
| OPNPCT | 101 | 4 | u | | MOVE_GAMECLICK | 63 | -1 | u |
| OPNPCU | 52 | 8 | u | | OPLOC1 | 238 | 6 | u |
| OPPLAYER1 | 211 | 2 | u | | OPLOC2 | 38 | 6 | u |
| OPPLAYER2 | 219 | 2 | u | | OPLOC3 | 19 | 6 | u |
| OPPLAYER3 | 64 | 2 | u | | OPLOC4 | 55 | 6 | u |
| OPPLAYER4 | 43 | 2 | u | | OPLOC5 | 243 | 6 | u |
| OPPLAYERT | 73 | 4 | u | | OPLOCT | 182 | 8 | u |
| OPPLAYERU | 48 | 8 | u | | OPLOCU | 106 | 12 | u |

(78 entries: 17 c, 1 r, 60 u.)

- [ ] **Step 1: Write/regenerate the failing pin test.** `prot_test.go` already pins the 225 table — regenerate it to assert the full 244 contract (the old contract is the wrong contract on this branch). Test shape:

```go
func TestOps244Table(t *testing.T) {
	want := []struct {
		opcode   uint8
		name     string
		size     int
		category int
	}{
		// TS ClientGameProt.ts (244 pin) — full table.
		{107, "NO_TIMEOUT", 0, CategoryClientEvent},
		{146, "IDLE_TIMER", 0, CategoryClientEvent},
		{217, "EVENT_TRACKING", -2, CategoryRestrictedEvent},
		// ... every row from the table above, all 78 ...
		{63, "MOVE_GAMECLICK", -1, CategoryUserEvent},
	}
	for _, w := range want {
		got := Ops[w.opcode]
		if got.Name != w.name || got.PayloadSize != w.size || got.Category != w.category {
			t.Errorf("Ops[%d] = %+v, want %s/%d/%d", w.opcode, got, w.name, w.size, w.category)
		}
	}
	// Removed packets must be unknown opcodes now.
	var known int
	for _, op := range Ops {
		if op.Name != "" {
			known++
		}
	}
	if known != len(want) {
		t.Errorf("known opcodes = %d, want %d (stale 225 entries or removed packets still present)", known, len(want))
	}
	for _, op := range Ops {
		if op.Name == "REBUILD_GETMAPS" || op.Name == "EVENT_CAMERA_POSITION" ||
			op.Name == "IDK_SAVEDESIGN" || op.Name == "TUT_CLICKSIDE" {
			t.Errorf("removed/renamed 225 packet %q still in table", op.Name)
		}
	}
}
```

- [ ] **Step 2: Run to verify FAIL** — `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/client/` → FAIL (table still 225).

- [ ] **Step 3: Implement.** In `prot.go`, add an exported opcode-constant block and rewrite `init()` to use it (single source of truth for registration):

```go
// 244 wire opcodes. TS ClientGameProt.ts (244 pin) — the TS ctor's first
// arg (NXT packet index) has zero readers at the pin and is not modeled.
const (
	OpcNoTimeout            uint8 = 107
	OpcIdleTimer            uint8 = 146
	OpcEventTracking        uint8 = 217
	OpcAnticheatOplogic1    uint8 = 47
	// ... one constant per row of the table above ...
	OpcOpNpc1               uint8 = 222
	OpcInvButtonD           uint8 = 81
	OpcIfPlayerDesign       uint8 = 8
	OpcTutorialClickSide    uint8 = 233
	OpcMoveGameClick        uint8 = 63
)
```

then `set(OpcOpNpc1, "OPNPC1", 2, u)` etc. for every row; delete the `REBUILD_GETMAPS` and `EVENT_CAMERA_POSITION` lines; rename `IDK_SAVEDESIGN`→`IF_PLAYERDESIGN` and `TUT_CLICKSIDE`→`TUTORIAL_CLICKSIDE`; `INV_BUTTOND` size 6→7.

In `handlers_game.go`, rewrite every registration to the constants (e.g. `gameHandlers[gameclient.OpcOpNpc1] = handleOpNpc1`), drop the `gameHandlers[150] = handleRebuildGetMaps` line (function deletion is Task 4), and rename the comment-names to the 244 names (`handleTutClickSide` stays wired under `OpcTutorialClickSide`; `handleIdkSaveDesignGame` under `OpcIfPlayerDesign` — function renames ride with their handler tasks).

- [ ] **Step 4: PASS + whole-tree build** — `go test ./pkg/io/protocol/game/client/` PASS; `CGO_ENABLED=0 go build -trimpath ./...` (handlers_game.go must compile; `handleRebuildGetMaps` becomes unused but still compiles — deleted in Task 4). Run `go test ./modules/world/ -run 'TestGamePacket|TestReadPacket|TestHandler' -count=1` to catch tests that pin raw opcodes; update any that do to the constants (verify each against the 244 table first).
- [ ] **Step 5: Commit** — `git commit --no-gpg-sign -m "feat(protocol): 244 client opcode renumber + named constants [rev-244 B2]"` (+ Claude trailer).

### Task 2: Server + zone prot — 244 renumber (incl. the rsbuf zone duplicate)

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `pkg/io/protocol/game/server/prot_test.go`
- Modify: `pkg/rsbuf/zone_encoders.go` (`ZoneOp*` consts at :11-22)
- Test: add cross-package consistency pin (see Step 1)

TS contracts:
`git -C /home/owner/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:src/network/game/server/ServerGameProt.ts` and `…:src/network/game/server/ServerGameZoneProt.ts`.

The complete 244 server table (name → opcode/size). **Defer the five size-changed rows to Task 3** (UPDATE_PID, LAST_LOGIN_INFO, REBUILD_NORMAL, MIDI_SONG, MIDI_JINGLE — their emitters must change in the same commit); everything else lands here:

```
IF_OPENCHAT 189/2     IF_OPENMAIN_SIDE 207/4   IF_CLOSE 214/0
IF_SETTAB 200/3       IF_SETTAB_ACTIVE 56/1    IF_OPENMAIN 10/2
IF_OPENSIDE 176/2     IF_OPENOVERLAY 158/2(NEW) IF_SETCOLOUR 78/4
IF_SETHIDE 123/3      IF_SETOBJECT 164/6       IF_SETMODEL 245/4
IF_SETRECOL 103/6     IF_SETANIM 219/4         IF_SETPLAYERHEAD 108/2
IF_SETTEXT 154/-2     IF_SETNPCHEAD 129/4      IF_SETPOSITION 241/6
TUT_FLASH 168/1       TUT_OPEN 174/2           UPDATE_INV_STOP_TRANSMIT 162/2
UPDATE_INV_FULL 72/-2 UPDATE_INV_PARTIAL 132/-2 CAM_LOOKAT 222/6
CAM_SHAKE 50/4        CAM_MOVETO 12/6          CAM_RESET 53/0
NPC_INFO 244/-2       PLAYER_INFO 86/-2        FINISH_TRACKING 60/0
ENABLE_TRACKING 22/0  MESSAGE_GAME 95/-1       UPDATE_IGNORELIST 7/-2
CHAT_FILTER_SETTINGS 9/3  MESSAGE_PRIVATE 30/-1  UPDATE_FRIENDLIST 70/9
UNSET_MAP_FLAG 62/0   UPDATE_RUNWEIGHT 160/2   HINT_ARROW 49/6
UPDATE_REBOOT_TIMER 85/2  UPDATE_STAT 24/6     UPDATE_RUNENERGY 177/1
RESET_ANIMS 242/0     LOGOUT 17/0              P_COUNTDIALOG 152/0
SET_MULTIWAY 97/1     VARP_SMALL 236/3         VARP_LARGE 226/6
RESET_CLIENT_VARCACHE 87/0  SYNTH_SOUND 151/5
UPDATE_ZONE_PARTIAL_FOLLOWS 94/2  UPDATE_ZONE_FULL_FOLLOWS 131/2
UPDATE_ZONE_PARTIAL_ENCLOSED 233/-2
--- zone (ServerGameZoneProt) ---
LOC_MERGE 29/14   LOC_ANIM 155/4   OBJ_DEL 39/3    OBJ_REVEAL 69/7
LOC_ADD_CHANGE 232/4   MAP_PROJANIM 137/15   LOC_DEL 125/2
OBJ_COUNT 209/7   MAP_ANIM 198/6   OBJ_ADD 234/5
--- removed (Task 4 deletes the vars + senders) ---
DATA_LAND, DATA_LAND_DONE, DATA_LOC, DATA_LOC_DONE
```

- [ ] **Step 1: Regenerate the pin test.** `prot_test.go` carries a name→Op table (`prot_test.go` around :253-301) — update every expected opcode/size to the 244 values above; add `IF_OPENOVERLAY`; remove the four `DATA_*` rows (the vars are deleted in Task 4 — for THIS task keep the vars but drop them from the test's known-set count if the test counts; if the test references `OpDataLand` it stays compiling until Task 4). Add a zone-duplicate consistency pin in `pkg/rsbuf`:

```go
// pkg/rsbuf/zone_encoders_test.go
func TestZoneOpcodesMatchServerProt(t *testing.T) {
	pairs := []struct {
		nested int
		op     gameserver.Op
	}{
		{ZoneOpLocMerge, gameserver.OpLocMerge},
		{ZoneOpLocAnim, gameserver.OpLocAnim},
		{ZoneOpObjDel, gameserver.OpObjDel},
		{ZoneOpObjReveal, gameserver.OpObjReveal},
		{ZoneOpLocAddChange, gameserver.OpLocAddChange},
		{ZoneOpMapProjAnim, gameserver.OpMapProjAnim},
		{ZoneOpLocDel, gameserver.OpLocDel},
		{ZoneOpObjCount, gameserver.OpObjCount},
		{ZoneOpMapAnim, gameserver.OpMapAnim},
		{ZoneOpObjAdd, gameserver.OpObjAdd},
	}
	for _, p := range pairs {
		if p.nested != int(p.op.Opcode) {
			t.Errorf("nested zone opcode %d != server prot %d", p.nested, p.op.Opcode)
		}
	}
}
```

(Check the actual `gameserver` var names in `pkg/io/protocol/game/server/prot.go` :88-101 — `OpLocMerge`, `OpObjAdd`, etc. — and the import alias used by existing rsbuf tests. If `pkg/rsbuf` importing `gameserver` creates an unwanted dependency, assert the literal 244 values in both packages' tests instead — two tests pinning the same constants.)

- [ ] **Step 2: FAIL run** — `go test ./pkg/io/protocol/game/server/ ./pkg/rsbuf/`.
- [ ] **Step 3: Implement** — update every `Op{Opcode: …}` value in server `prot.go` per the table (the five deferred rows keep their 225 values for now); add `OpIfOpenOverlay = Op{Opcode: 158, PayloadSize: 2}` next to the other IF_OPEN* vars with a `// TS ServerGameProt.ts (244): IF_OPENOVERLAY. Call site lands with B4's IF_OPENOVERLAY script op.` comment; update the ten `ZoneOp*` constants in `pkg/rsbuf/zone_encoders.go` AND the matching `Op*` zone vars in server prot.go. Also update the hardcoded opcode references in `pkg/rsbuf/zone_encoders.go` comments (:163-175 mention opcodes 135/7/162 — now 131/94/233).
- [ ] **Step 4: PASS + build** — `go test ./pkg/io/protocol/game/server/ ./pkg/rsbuf/ ./modules/world/ -count=1`; whole-tree build. Zone/encoder tests in `modules/world` or `pkg/rsbuf` that pin old opcodes get updated to the 244 contract (verify each against the table first).
- [ ] **Step 5: Commit** — `feat(protocol): 244 server + zone opcode renumber, IF_OPENOVERLAY row [rev-244 B2]`.

### Task 3: Size-changed server packets — table row + emitter together

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go` (5 rows) + `prot_test.go`
- Modify: `modules/world/rebuildmap.go` (`sendRebuildNormal` :18-35)
- Modify: `modules/world/login_resync.go` (`sendUpdatePid` :14-19)
- Modify: `modules/world/player_script.go` (LastLoginInfo emitter ~:1735-1751; `PlaySong` ~:1597-1612; `PlayJingle` ~:1634-1648)
- Modify: `modules/world/midi_encoders.go` (`encodeMidiSong`/`encodeMidiJingle`)
- Tests: the emitters' existing test files

TS contracts (read each in full):
`git -C …Engine-TS show 9aadcec4:src/network/game/server/codec/RebuildNormalEncoder.ts` (p2 zoneX, p2 zoneZ — the 225 per-mapsquare CRC loop is GONE; the 244 client fetches maps via OnDemand),
`…/UpdatePidEncoder.ts` (p2 uid, pbool members),
`…/LastLoginInfoEncoder.ts` (p4 ip, p2 days, p1 recovery, p2 unread, pbool warnMembersInNonMembers),
`…/MidiSongEncoder.ts` (p2 id), `…/MidiJingleEncoder.ts` (p2 id, p2 delay).
Producer context: TS `Player.ts:1919-1933` (`MidiPack.getByName` → `if (id !== -1) write`), `Player.ts:2198` (`new LastLoginInfo(lastIp, daysSinceLogin, daysSinceRecoveriesChanged, this.messageCount, warnMembersInNonMembers)`).

- [ ] **Step 1: Update emitter tests to the 244 wire (failing first).** Per emitter: REBUILD_NORMAL payload = exactly 4 bytes (zoneX/zoneZ, no CRC list, no `cache.Preload()` dependency); UPDATE_PID = 3 bytes (slot + members bool from server config); LAST_LOGIN_INFO = 10 bytes (trailing warn bool); MIDI_SONG = 2 bytes (id); MIDI_JINGLE = 4 bytes (id, delay). For midi, pin the no-op path: unresolvable name → nothing written (mirrors TS `id !== -1` guard).
- [ ] **Step 2: FAIL run** — `go test ./modules/world/ -run 'Rebuild|UpdatePid|LastLogin|PlaySong|PlayJingle' -count=1`.
- [ ] **Step 3: Implement.**
  - `sendRebuildNormal`: drop the mapsquare loop + `cache.Preload()` usage; keep `p2(zoneX) p2(zoneZ)`. Update its doc comment citation to `// TS RebuildNormalEncoder.ts (244): p2 zoneX, p2 zoneZ`. Callers passing `mapsquares` lose the arg — chase the compile cascade.
  - `sendUpdatePid`: append `pbool(members)`; thread the world-members flag from the caller (`s.cfg.NodeMembers` — check the actual config field name in `modules/world/config.go`).
  - LastLoginInfo emitter: append the warn bool. TS computes `warnMembersInNonMembers` in B3-surface code — until B3 lands, compute the same expression the 244 producer uses; read `git -C …Engine-TS show 9aadcec4:src/engine/entity/Player.ts | sed -n '2180,2200p'` and port the exact derivation. If the derivation depends on B3-only state, emit `false` with a `// rev244-b2: placeholder until B3 ports Player.ts:2198 producer` comment + tracker row.
  - Midi: rewrite `encodeMidiSong(buf, id)` → `p2(id)`; `encodeMidiJingle(buf, id, delay)` → `p2(id) p2(delay)`. `PlaySong`/`PlayJingle` switch from `cache.Preload()` blob lookups to a name→id lookup behind a new helper `midiIDByName(name string) int` that returns `-1` until B3 wires the MidiPack registry (faithful to TS's `id !== -1` guard → silent no-op). Mark with `PORTING-EXCEPTION (rev244-b2-midi-window, silent until B3 MidiPack)`.
  - Update the five `Op*` table rows + prot_test.go expectations (UPDATE_PID 210/3, LAST_LOGIN_INFO 44/10, REBUILD_NORMAL 165/4, MIDI_SONG 240/2, MIDI_JINGLE 173/4).
- [ ] **Step 4: PASS + build + `go vet ./...`.**
- [ ] **Step 5: Commit** — `feat(protocol): 244 size-changed server packets — rebuild/updatepid/lastlogin/midi [rev-244 B2]`.

### Task 4: Dead-packet removal — REBUILD_GETMAPS + DATA_* (+ EVENT_CAMERA_POSITION remnants)

**Files:**
- Delete code in: `modules/world/data_map.go` (whole file if nothing else lives there), `modules/world/handlers_game.go` (`handleRebuildGetMaps` if defined there — locate with `grep -rn "handleRebuildGetMaps" modules/world`)
- Modify: `pkg/io/protocol/game/server/prot.go` (delete `OpDataLand`/`OpDataLoc`/`OpDataLandDone`/`OpDataLocDone` :107-110 + their prot_test rows)
- Delete/modify: their tests (`grep -rln "RebuildGetMaps\|DataLand\|DataLoc" modules/world pkg`)

TS contract: at `9aadcec4` the files `RebuildGetMapsHandler.ts`, `RebuildGetMapsDecoder.ts`, `RebuildGetMaps.ts`, `DataLand*.ts`, `DataLoc*.ts` are DELETED (−70/−17/−12/−56 lines) and `ServerGameProtRepository.ts` unbinds them. 244 map delivery = engine OnDemand (B3).

- [ ] **Step 1:** `grep -rn "EVENT_CAMERA_POSITION" modules pkg cmd internal` — confirm the only remnant was the Ops row (already gone in Task 1); delete any handler/plumbing found.
- [ ] **Step 2:** Check the bundle-17 staff-rebuild `PORTING-EXCEPTION` marker (`grep -rn "PORTING-EXCEPTION" modules pkg | grep -i rebuild`): if the marked code is being deleted here, the exception row closes (record in Task 12); if the marker covers REBUILD_NORMAL (kept), it stays.
- [ ] **Step 3:** Delete `handleRebuildGetMaps`, the `data_map.go` senders, the four `OpData*` vars + test rows + the senders' tests. Keep `rebuildmap.go` (REBUILD_NORMAL stays).
- [ ] **Step 4:** Build + `go test ./modules/world/ ./pkg/io/... -count=1` green; `go vet ./...`.
- [ ] **Step 5: Commit** — `feat(world): remove REBUILD_GETMAPS/DATA_* map streaming (244 moves maps to OnDemand) [rev-244 B2]`.

---

## Slice 2 — handler family

Shared rules for Tasks 5-10 (from the verified diffs; each task re-verifies against its own TS files — do NOT pattern-apply):
- goscape decodes payloads inline in `modules/world/handler_*.go`; TS decoder renames (`com`→`component`, `npcSlot`→`nid`) are comment/naming-level — adopt the 244 names in Go locals where they appear.
- The recurring 244 behavioral reshapes seen in `OpHeldHandler.ts` and `OpNpcHandler.ts`: validation order changes; `player.clearPendingAction()` added to reject paths; the `delayed` check MOVED (OpHeld: after validation; OpNpc: stays first but reject paths gain clearPendingAction); explicit per-op `if/else` trigger dispatch replaces `TRIGGER1 + (op-1)` arithmetic; sessionlog emission gated per-op.
- The protected-access invariant (`runScript` guard, `modules/world/script.go`) is load-bearing — never weakened by handler edits.
- TDD per task: update the handler's existing tests to pin each 244 behavioral delta FIRST (red), then port, then green. When an existing test pins 225 behavior, update it (verify vs TS first).
- Citations: `// TS <File>.ts:<lines>` at the 244 pin.

### Task 5: OpHeld family (OPHELD1-5, OPHELDT, OPHELDU)

**Files:** Modify `modules/world/handler_opheld.go` + `handler_opheld_test.go`

TS contract: `git -C …Engine-TS diff e1dea19f..9aadcec4 -- src/network/game/client/handler/OpHeldHandler.ts src/network/game/client/handler/OpHeldTHandler.ts src/network/game/client/handler/OpHeldUHandler.ts src/network/game/client/codec/OpHeldDecoder.ts src/network/game/client/codec/OpHeldTDecoder.ts src/network/game/client/codec/OpHeldUDecoder.ts` — read in full.

Verified OpHeld deltas (244): validation runs FIRST (component visible+interactable → iop valid (op≠5: `(type.iop && !type.iop[op-1]) || !type.iop` rejects) → listener exists → inv valid/hasAt), each reject calls `clearPendingAction()` and returns; `delayed` check moved AFTER all validation (no clearPendingAction on that path); `lastItem`/`lastSlot` set after; `clearPendingAction` iff `com.rootLayer != player.modalMain` (unchanged); explicit per-op trigger dispatch; sessionlog for ops 1-4 only (op 5 wealth-logged in content). OpHeldU (31/48) and OpHeldT (14/24) have their own reshapes — translate from their diffs.

- [ ] **Step 1:** Update `handler_opheld_test.go` to pin: (a) reject-path order — invalid component rejected even when `p.delayed` (new: validation precedes delayed); (b) every reject path calls ClearPendingAction (observable: pending action cleared); (c) delayed-only rejection does NOT clear pending action; (d) op5 skips sessionlog, ops 1-4 emit. RED.
- [ ] **Step 2:** Port all three handlers per their diffs; update the gate-order doc comment in `handler_opheld.go` (it documents the 225 order).
- [ ] **Step 3:** `go test ./modules/world/ -run 'OpHeld' -count=1` PASS, then the full world suite (`-count=1`, ~2.5 min — not hung).
- [ ] **Step 4: Commit** — `feat(world): 244 OpHeld family — validation order, clearPendingAction rejects [rev-244 B2]`.

### Task 6: InvButton + InvButtonD (new mode byte)

**Files:** Modify `modules/world/handler_inv_button.go` + `handler_inv_button_test.go`

TS contract: `git -C …Engine-TS diff e1dea19f..9aadcec4 -- src/network/game/client/handler/InvButtonHandler.ts src/network/game/client/handler/InvButtonDHandler.ts src/network/game/client/codec/InvButtonDecoder.ts src/network/game/client/codec/InvButtonDDecoder.ts src/network/game/client/model/InvButtonD.ts`.

Verified: `InvButtonD` wire grows a trailing `mode` g1 (size 6→7, table updated in Task 1); decoder returns `(component, slot, targetSlot, mode)`. InvButtonHandler 24/21 + InvButtonDHandler 11/19 reshapes per diff (what 244 does with `mode` — read the handler diff and port exactly).

- [ ] **Step 1:** Tests pin the 7-byte read incl. mode + the handler's mode-dependent behavior per the TS diff. RED.
- [ ] **Step 2:** Port. **Step 3:** Targeted + full-package tests PASS. **Step 4: Commit** — `feat(world): 244 InvButton/InvButtonD — mode byte + handler reshape [rev-244 B2]`.

### Task 7: OpNpc family (OPNPC1-5, OPNPCT, OPNPCU)

**Files:** Modify `modules/world/handler_opnpc.go` + `handler_opnpc_test.go`

TS contract: `git -C …Engine-TS diff e1dea19f..9aadcec4 -- src/network/game/client/handler/OpNpcHandler.ts src/network/game/client/handler/OpNpcTHandler.ts src/network/game/client/handler/OpNpcUHandler.ts src/network/game/client/codec/OpNpcDecoder.ts src/network/game/client/codec/OpNpcTDecoder.ts src/network/game/client/codec/OpNpcUDecoder.ts`.

Verified OpNpc deltas: delayed → UnsetMapFlag only (no clearPendingAction); merged `!npc || npc.delayed` reject (UnsetMapFlag + clearPendingAction); `rsbuf.hasNpc` reject + clearPendingAction; npcType.op check simplified to falsy (`!npcType.op || !npcType.op[op-1]` — the explicit `'hidden'` comparison is gone; verify what goscape's op-string representation needs); explicit APNPC1-5 dispatch. T/U variants per their diffs.

- [ ] Steps as Task 5 (tests pin reject-path clearPendingAction matrix → RED → port → green → commit `feat(world): 244 OpNpc family [rev-244 B2]`).

### Task 8: OpObj family (OPOBJ1-5, OPOBJT, OPOBJU)

**Files:** Modify `modules/world/handler_opobj.go` + `handler_opobj_test.go`

TS contract: `git -C …Engine-TS diff e1dea19f..9aadcec4 -- src/network/game/client/handler/OpObjHandler.ts src/network/game/client/handler/OpObjTHandler.ts src/network/game/client/handler/OpObjUHandler.ts src/network/game/client/codec/OpObjDecoder.ts src/network/game/client/codec/OpObjTDecoder.ts src/network/game/client/codec/OpObjUDecoder.ts` — read in full (OpObj 21/9, OpObjT 5/10, OpObjU 21/31).

- [ ] **Step 1:** Update `handler_opobj_test.go` to pin each 244 behavioral delta found in the diff (expect the family pattern: reject-path `clearPendingAction`, validation reshapes, explicit trigger dispatch — but pin only what THIS diff shows). RED.
- [ ] **Step 2:** Port all three handlers per their diffs with `// TS OpObj*Handler.ts:<lines>` citations.
- [ ] **Step 3:** `go test ./modules/world/ -run 'OpObj' -count=1` PASS, then the full package.
- [ ] **Step 4: Commit** — `feat(world): 244 OpObj family [rev-244 B2]`.

### Task 9: OpLoc family (OPLOC1-5, OPLOCT, OPLOCU)

**Files:** Modify `modules/world/handler_oploc.go` + `handler_oploc_test.go`

TS contract: `git -C …Engine-TS diff e1dea19f..9aadcec4 -- src/network/game/client/handler/OpLocHandler.ts src/network/game/client/handler/OpLocTHandler.ts src/network/game/client/handler/OpLocUHandler.ts src/network/game/client/codec/OpLocDecoder.ts src/network/game/client/codec/OpLocTDecoder.ts src/network/game/client/codec/OpLocUDecoder.ts` — read in full (OpLoc 19/8, OpLocT 6/11, OpLocU 21/31).

- [ ] **Step 1:** Update `handler_oploc_test.go` to pin each 244 behavioral delta found in the diff. RED.
- [ ] **Step 2:** Port all three handlers per their diffs with citations.
- [ ] **Step 3:** `go test ./modules/world/ -run 'OpLoc' -count=1` PASS, then the full package.
- [ ] **Step 4: Commit** — `feat(world): 244 OpLoc family [rev-244 B2]`.

### Task 10: OpPlayer family (OPPLAYER1-4, OPPLAYERT, OPPLAYERU)

**Files:** Modify `modules/world/handler_op_player.go` + `handler_op_player_test.go`

TS contract: `git -C …Engine-TS diff e1dea19f..9aadcec4 -- src/network/game/client/handler/OpPlayerHandler.ts src/network/game/client/handler/OpPlayerTHandler.ts src/network/game/client/handler/OpPlayerUHandler.ts src/network/game/client/codec/OpPlayerDecoder.ts src/network/game/client/codec/OpPlayerTDecoder.ts src/network/game/client/codec/OpPlayerUDecoder.ts` — read in full (OpPlayer 16/9, OpPlayerT 7/12, OpPlayerU 18/28).

- [ ] **Step 1:** Update `handler_op_player_test.go` to pin each 244 behavioral delta found in the diff. RED.
- [ ] **Step 2:** Port all three handlers per their diffs with citations.
- [ ] **Step 3:** `go test ./modules/world/ -run 'OpPlayer' -count=1` PASS, then the full package.
- [ ] **Step 4: Commit** — `feat(world): 244 OpPlayer family [rev-244 B2]`.

### Task 11: Small-handler sweep

**Files (locate each with grep; all in `modules/world/`):**
- `handler_interface.go` (IfButton), `handlers_game.go`/`handler_message_*.go` (MessagePublic/Private inline decodes), `handler_chatsetmode.go`, the IfPlayerDesign handler (`handleIdkSaveDesignGame` — rename to `handleIfPlayerDesign`), the TutClickSide handler (rename to `handleTutorialClickSide`), NoTimeout, EventTracking, ClientCheat, IdleTimer, ResumePCountDialog sites + tests.

TS contract: `git -C …Engine-TS diff e1dea19f..9aadcec4 -- src/network/game/client/handler/IfButtonHandler.ts src/network/game/client/handler/MessagePublicHandler.ts src/network/game/client/handler/ChatSetModeHandler.ts src/network/game/client/handler/IdkSaveDesignHandler.ts src/network/game/client/handler/TutClickSideHandler.ts src/network/game/client/handler/ClientCheatHandler.ts src/network/game/client/handler/IdleTimerHandler.ts src/network/game/client/codec/MessagePublicDecoder.ts src/network/game/client/codec/MessagePrivateDecoder.ts src/network/game/client/codec/IfButtonDecoder.ts src/network/game/client/codec/NoTimeoutDecoder.ts src/network/game/client/codec/TutorialClickSideDecoder.ts src/network/game/client/model/EventTracking.ts src/network/game/client/model/ResumePCountDialog.ts`

Verified deltas: IfButton 4/7 behavioral trim; MessagePublic/Private decoders change their `buf.pos` consumption arithmetic (byte-level — port exactly); ChatSetMode +1 line; IfPlayerDesign 5/4 + rename; TutorialClickSide rename (decoder reads g1 tab, same as 225); NoTimeout gains an explicit empty decoder upstream (goscape `handleNoTimeout` already no-ops — citation update only); ClientCheat 2/2 is `STANDALONE_BUNDLE` gating of reload/rebuild cheats (goscape doesn't port STANDALONE_BUNDLE → NO-OP, record row); IdleTimer 0/1 import removal → NO-OP; EventTracking/ResumePCountDialog model 1/3 formatting → NO-OP.

- [ ] **Step 1:** Tests for the real deltas (IfButton, MessagePublic/Private consumption, IfPlayerDesign, ChatSetMode) — RED.
- [ ] **Step 2:** Port + renames (`handleIdkSaveDesignGame`→`handleIfPlayerDesign`, `handleTutClickSide`→`handleTutorialClickSide`, chase references incl. handlers_game.go comments).
- [ ] **Step 3:** Full `modules/world` suite green.
- [ ] **Step 4: Commit** — `feat(world): 244 small-handler sweep — ifbutton/message/design/tutorial renames [rev-244 B2]`.

---

## Slice 3 — rsbuf damage2 + entity feed

### Task 12: pkg/rsbuf damage2 (the crate delta) + spot-check

**Files:**
- Modify: `pkg/rsbuf/visibility.go` (player mask consts :16-25), `pkg/rsbuf/npc_source.go` (npc consts :4-12 + NpcSource), `pkg/rsbuf/source.go` (PlayerSource), `pkg/rsbuf/player.go`, `pkg/rsbuf/npc.go`, `pkg/rsbuf/buf.go` (ComputePlayer :158, ComputeNpc :302), `pkg/rsbuf/mask_payload.go`, `pkg/rsbuf/npc_mask_payload.go`, `pkg/rsbuf/renderer.go`
- Tests: the packages' existing `*_test.go` files

Rust contract (THE work list): `git -C /home/owner/Code/github.com/2004scape/rsbuf diff 225 origin/244 -- src` (+64/−8, 6 files). Key facts, all verified:

- `prot.rs`: player `DAMAGE2 = 0x400` APPENDED (no existing bit changes); npc `DAMAGE2 = 0x1` fills the previously-unused 0x1 slot (**no existing npc bit changes** — 225 already started at ANIM=0x2). Internal cache indices: player DAMAGE2→5 (FACE_COORD 5→6, CHAT 6→7, SPOT_ANIM 7→8); npc DAMAGE2→4 (CHANGE_TYPE 4→5, SPOT_ANIM 5→6, FACE_COORD 6→7).
- `renderer.rs`: cache arrays 8→9 (player) / 7→8 (npc); DAMAGE2 cached via the SAME Damage payload shape `(damage_taken2, damage_type2, current_hitpoints, base_hitpoints)`.
- `info.rs` — **wire order** (the byte contract):
  - Player write_blocks (info.rs:349-405): mask header `>0xff → ip2(masks|BIG)` else p1; then APPEARANCE → ANIM → FACE_ENTITY → SAY → DAMAGE → FACE_COORD → CHAT → SPOT_ANIM → EXACT_MOVE → **DAMAGE2 LAST**.
  - Npc write_blocks (info.rs:673-707): `p1(masks & 0xff)`; then **DAMAGE2 FIRST** → ANIM → FACE_ENTITY → SAY → DAMAGE → CHANGE_TYPE → SPOT_ANIM → FACE_COORD.
- `lib.rs`: `compute_player`/`compute_npc` gain `damageTaken2: i32, damageType2: i32` immediately after `damageType`.
- `player.rs`/`npc.rs`: `damage_taken2`/`damage_type2` fields, init −1, reset −1 in the per-tick reset.

goscape mapping notes:
- goscape's `Renderer` composes per-slot HighDef/LowDef byte slices (NOT per-prot caches like Rust) — the Rust cache-count/index changes have NO direct Go analog. Record as a NO-OP decision row after verifying `renderer.go`'s composition order matches the info.rs wire order above (that's where the order lives in Go — `writeMaskPayloads`).
- `PlayerSource`/`NpcSource` interfaces gain `Damage2Amt() int` / `Damage2Type() int`; `go build ./...` reveals every implementer (expect: rsbuf-internal structs + the modules/world adapters — implement all, Task 13 wires real values).

- [ ] **Step 1: Failing tests.**

```go
// pkg/rsbuf/mask_payload_test.go (extend existing patterns)
// Rust prot.rs (244): player DAMAGE2 bit + write order — DAMAGE2 payload
// is written LAST, after EXACT_MOVE (info.rs:402-404).
func TestPlayerMaskDamage2Bit(t *testing.T) {
	if MaskDamage2 != 0x400 {
		t.Fatalf("MaskDamage2 = %#x, want 0x400", MaskDamage2)
	}
}

func TestWriteMaskPayloadsDamage2Last(t *testing.T) {
	p := &fakePlayerSource{ // use the file's existing fake/stub source type
		damageAmt: 3, damageType: 1, damage2Amt: 7, damage2Type: 2,
		curHP: 9, baseHP: 10,
	}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskDamage|MaskDamage2)
	// DAMAGE block (amt,type,cur,base) then DAMAGE2 block (amt2,type2,cur,base).
	want := []byte{3, 1, 9, 10, 7, 2, 9, 10}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("payload = %v, want %v", buf.Bytes(), want)
	}
}

// pkg/rsbuf/npc_mask_payload_test.go
// Rust prot.rs (244): npc DAMAGE2 = 0x1, written FIRST (info.rs:683-685).
func TestNpcMaskDamage2Bit(t *testing.T) {
	if NpcMaskDamage2 != 0x1 {
		t.Fatalf("NpcMaskDamage2 = %#x, want 0x1", NpcMaskDamage2)
	}
}

func TestNpcWriteMaskPayloadsDamage2First(t *testing.T) {
	n := &fakeNpcSource{
		damageAmt: 3, damageType: 1, damage2Amt: 7, damage2Type: 2,
		curHP: 9, baseHP: 10,
	}
	buf := packet.NewPacket(nil)
	writeNpcMaskPayloads(buf, n, NpcMaskDamage|NpcMaskDamage2) // use the file's actual writer name
	want := []byte{7, 2, 9, 10, 3, 1, 9, 10} // DAMAGE2 first, then DAMAGE
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("payload = %v, want %v", buf.Bytes(), want)
	}
}
```

(Adapt fake-source type names and writer function names to the existing test files' conventions — read them first. Add a ComputePlayer/ComputeNpc round-trip test asserting the new args land in the rsbuf Player/Npc structs and reset to −1 on the per-tick reset.)

- [ ] **Step 2: FAIL run** — `go test ./pkg/rsbuf/ -run 'Damage2'`.
- [ ] **Step 3: Implement** per the Rust diff, file by file: constants (`MaskDamage2 = 0x400` in visibility.go; `NpcMaskDamage2 = 0x1` in npc_source.go); interface methods; struct fields + ctor/reset −1 (player.go, npc.go — mirror the existing DamageTaken/DamageType lines); `ComputePlayer`/`ComputeNpc` +2 params positioned after damageType (buf.go — callers break: fix Task 13's tick.go sites with `-1, -1` placeholders IN THIS TASK so the tree compiles; Task 13 replaces them with real fields); `writeDamage2` appended LAST in `writeMaskPayloads` (mask_payload.go — update the order doc comment :15-19); npc DAMAGE2 FIRST in npc_mask_payload.go (update :12 comment); renderer.go composition order per info.rs.
- [ ] **Step 4: Spot-check (user-approved depth):** while in each touched Go file, diff its surrounding mask/write-order/payload logic against the 244 Rust source (`git -C …rsbuf show origin/244:src/<file>.rs`). Record per-file verdicts (clean / divergence found+fixed / divergence found+row) in the task report for Task 14's audit rows.
- [ ] **Step 5: PASS + `-race`** — `go test ./pkg/rsbuf/ -count=1` then `CGO_ENABLED=1 go test -race ./pkg/rsbuf/`.
- [ ] **Step 6: Commit** — `feat(rsbuf): 244 damage2 — player 0x400 last, npc 0x1 first, compute params [rev-244 B2]`.

### Task 13: modules/world damage2 feed (the pulled-forward entity hunks)

**Files:**
- Modify: `modules/world/masks.go` (both const blocks)
- Modify: `modules/world/player.go` (fields near :482), `modules/world/npc.go` (fields near :140)
- Modify: `modules/world/player_masks.go` (`Damage` :196 region; reset :130 region), `modules/world/npc_masks.go` (:175 region; reset :230 region)
- Modify: `modules/world/tick.go` (bridge args :925-ish player, :965-ish npc — replace Task 12's `-1, -1` placeholders)
- Tests: `modules/world/player_masks_test.go` / `npc_masks_test.go` (or the files' existing test homes)

TS contract (the pulled-forward hunks — cite them, and Task 14 records the B3 must-not-double-apply rows):
- `PathingEntity.ts:92-96` — `hitmarkSlot = 0`, `hitmark2Damage = -1`, `hitmark2Type = -1` fields.
- `PathingEntity.ts:606-610` — per-tick reset: hitmark fields → −1 AND `hitmarkSlot = 0`.
- `Player.ts:1870-1890` / `Npc.ts:475-494` — `applyDamage`: HP clamp (unchanged), then `hitmarkSlot % 2 === 1` → second slot (`hitmark2*` + `masks |= DAMAGE2`) else first slot (`hitmark*` + `masks |= DAMAGE`); `hitmarkSlot++` always. Parity quirk to pin: slot 0 → DAMAGE, slot 1 → DAMAGE2, slot 2 → DAMAGE again (overwrites), reset to 0 each tick.
- TS feeds the renderer from `hitmark2Damage/hitmark2Type` (World.ts:1041-1042 player, :1086-1087 npc).

goscape naming: existing fields are `damageAmt`/`damageType` — add `damage2Amt`/`damage2Type` + `hitmarkSlot` alongside (B3's wholesale `hitmark*` rename pass, if adopted, happens there; decision row notes it).

- [ ] **Step 1: Failing tests.**

```go
// modules/world — alternation pin, BOTH forks (player shown; mirror for npc).
// TS Player.ts:1879-1889 / PathingEntity.ts:610.
func TestPlayerDamageAlternatesHitmarkSlots(t *testing.T) {
	p := newTestPlayer(t) // use the file's existing helper
	p.Damage(3, 0)        // slot 0 → first hitmark
	if p.damageAmt != 3 || p.masks&MaskDamage == 0 {
		t.Fatal("first hit must fill hitmark 1 + MaskDamage")
	}
	p.Damage(5, 1) // slot 1 → second hitmark
	if p.damage2Amt != 5 || p.damage2Type != 1 || p.masks&MaskDamage2 == 0 {
		t.Fatal("second hit must fill hitmark 2 + MaskDamage2")
	}
	p.Damage(7, 0) // slot 2 → first hitmark again (overwrite)
	if p.damageAmt != 7 {
		t.Fatal("third hit must overwrite hitmark 1")
	}
}

func TestPlayerResetClearsHitmark2AndSlot(t *testing.T) {
	p := newTestPlayer(t)
	p.Damage(3, 0)
	p.Damage(5, 1)
	p.resetMasks() // use the actual per-tick reset name at player_masks.go:126-140
	if p.damage2Amt != -1 || p.damage2Type != -1 || p.hitmarkSlot != 0 {
		t.Fatal("per-tick reset must clear hitmark2 fields to -1 and hitmarkSlot to 0")
	}
}
```

- [ ] **Step 2: FAIL run** — `go test ./modules/world/ -run 'Hitmark|Damage' -count=1`.
- [ ] **Step 3: Implement:** `MaskDamage2 = 1024` + `NpcMaskDamage2 = 1` in masks.go (cite TS PlayerInfoProt/NpcInfoProt via rsbuf 244); fields; alternation in BOTH `Damage` producers (player_masks.go:196 region, npc_masks.go:175 region — the TS shared-base-class line exists twice in Go, port BOTH); resets in both reset sites (+ `hitmarkSlot = 0`); tick.go bridge args `int32(p.damage2Amt), int32(p.damage2Type)` (and npc equivalents) replacing the `-1, -1` placeholders.
- [ ] **Step 4: Consistency pin** — masks.go constants vs pkg/rsbuf constants: extend whichever existing test pins that correspondence (`grep -rn "MaskDamage" modules/world/*_test.go`) or add one.
- [ ] **Step 5: PASS + `-race`** — `go test ./modules/world/ -count=1` (~2.5 min) then `-race` on it.
- [ ] **Step 6: Commit** — `feat(world): 244 damage2 feed — hitmarkSlot alternation, both forks (pulled forward from B3) [rev-244 B2]`.

### Task 14: Gates + PORTING.md B2 audit trail

**Files:** Modify `PORTING.md` (§rev-244 Bundle audit trail — add a `### B2` subsection in B1's format)

- [ ] **Step 1: Full gates, real exit codes:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1; echo "EXIT: $?"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./...; echo "EXIT: $?"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...; echo "EXIT: $?"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=1 go test -race ./pkg/rsbuf/ ./modules/world/ ./pkg/io/protocol/... ; echo "EXIT: $?"
```
(`pkg/util/build` self-assignment vet warnings are pre-existing; the two B1 format-window skips remain.)
- [ ] **Step 2: Correspondence audit.** For every file in `git -C …Engine-TS diff --numstat e1dea19f..9aadcec4 -- src/network` (115 files) + the 6 rsbuf crate files: map to a Go commit or decision row. Bulk NO-OP groups are fine (e.g. "model `com`→`component` renames ×14 — Go decodes inline, naming adopted in handler locals"; "decoder import-only deltas ×8 — NO-OP"; "`ClientGameProtRepository`/`ServerGameProtRepository` — TS DI infra, Go analog is the table+registration, covered by Tasks 1-2").
- [ ] **Step 3: Decision rows** (each with TS citation):
  - TS `ClientGameProt.index` — NOT-MODELED (zero readers at pin).
  - Map-delivery window — DATA_*/REBUILD_GETMAPS removed; OnDemand lands B3; expires at B3's positive boot/login smoke.
  - Midi-id window — `PORTING-EXCEPTION (rev244-b2-midi-window)`; MidiPack registry lands B3.
  - damage2 entity pull-forward — **B3 must NOT double-apply** PathingEntity.ts:92-96,606-610 / Player.ts:1870-1890 / Npc.ts:475-494; B3 owns any `damageAmt`→`hitmarkDamage` wholesale rename.
  - rsbuf Rust cache-index/count changes — NO-OP for Go (composition differs); spot-check verdicts from Task 12 recorded per file.
  - `IfSetRecolEncoder` deletion — DEFERRED → B4 (script-op removal); wire row unchanged (103/6); goscape emitter stays wired until B4.
  - `IF_OPENOVERLAY` call site — DEFERRED → B4 (script op).
  - `UpdatePid`→`UpdateUid192` TS model rename — NOT-ADOPTED (goscape sends inline; note only).
  - bundle-17 staff-rebuild marker disposition (from Task 4).
  - LastLoginInfo `warnMembersInNonMembers` derivation — if stubbed false in Task 3, row pointing at B3.
- [ ] **Step 4: Commit** — `docs(porting): rev-244 B2 audit trail — protocol/handlers/rsbuf correspondence [rev-244 B2]`.

---

## Execution notes (B1 process, repeat it)

- Subagent-driven: implementer (sonnet) → TS-parity spec reviewer → quality reviewer per substantive task (Tasks 1-3, 5-13); controller-direct verification for Task 4; Task 14 controller-run. The B1 spec reviewer caught a real panic bug — do not skip.
- Warn every subagent: phantom `??` dotfiles in git status (never stage; no `git add -A`); modules/world suite ~2.5 min; post-TDD stale LSP diagnostics are normal — trust real build/test runs.
- Tests that pin 225 contracts are updated to 244 after TS verification (PORTING-LESSONS §3: the old contract is the wrong contract on this branch).
- Task order is dependency order; Tasks 5-10 are mutually independent (parallelizable across subagents IF file-disjoint — they are, one handler file each; handlers_game.go is only touched by Task 1/4/11).

## Self-review notes

- Spec coverage: slice 1 = Tasks 1-4 (tables, named constants, size-changed emitters, dead packets); slice 2 = Tasks 5-11 (whole handler family incl. decoder deltas); slice 3 = Tasks 12-13 (crate delta + pulled-forward feed); gates/audit = Task 14. The spec's "Testing" bullets map: table pins (T1/T2/T3), handler pins (T5-T11), rsbuf bit/order/alternation pins (T12/T13), gates (T14).
- The five size-changed server packets ride with their emitters (T3) so every task ends green.
- Wire-order facts in Task 12 were re-derived from info.rs at `origin/244` (player DAMAGE2 LAST, npc DAMAGE2 FIRST) — they supersede the spec's coarser "after DAMAGE" wording, which described the Rust cache-iteration lists.
- NPC mask bits do NOT shift (225 had 0x1 unused) — supersedes the spec's risk-#1 wording; the grep-audit for raw mask literals still runs in Task 13 Step 4 via the consistency pin.
