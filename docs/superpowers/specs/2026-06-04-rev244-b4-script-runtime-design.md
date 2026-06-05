# rev-244 B4 — script runtime — design

**Date:** 2026-06-04
**Status:** Approved
**Branch:** rev-244 (umbrella: `2026-06-03-rev244-port-design.md` §B4; resume
context: `docs/superpowers/handoffs/2026-06-04-RESUME-rev244-port-b4.md`)

## Goal

Port the script-runtime slice of the 225→244 Engine-TS delta: the full
`ScriptOpcode.ts` renumber, the handler-surface deltas (ServerOps +175,
DebugOps +55, PlayerOps 40/72, InvOps 35/31, NpcOps 1/52, StringOps 1/51,
StructOps deleted, DbOps 9/21, NumberOps 4/4), the
`ScriptOpcodePointers.ts`/`ScriptRunner.ts`/`ScriptState.ts`/`ScriptFile.ts`
core deltas, and the `PlayerHuntAllCommandIterator` removal. All TS citations
refer to the 244 pin `9aadcec4`.

**Handoff correction:** `ScriptIterators.ts` is NOT deleted upstream — only
`PlayerHuntAllCommandIterator` (−58 lines) is removed; `HuntIterator`,
`NpcHuntAllCommandIterator`, `NpcIterator`, `LocIterator`, `ObjIterator`
survive at the pin.

## Scope slice (extraction command)

```
git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/engine/script
```

(15 files, +584/−556.) Plus three B4-assigned externals:

1. The **B2-deferred IF_SETRECOL wire-row removal** (B2 decision row: "the
   encoder/table row stayed wired pending B4's removal of the script op").
2. The **B3-deferred IF_OPENOVERLAY op dispatch** (B3 shipped
   `Player.OpenOverlay` + per-tick flush; B4 wires only the script op).
3. The **world-side `cycleStats`/`lastCycleStats` instrumentation** the new
   debug ops read — a pre-existing 225-era gap (TS had it at BOTH pins;
   goscape never ported it), closed here by user decision.

## User decisions (recorded 2026-06-04)

1. **Cycle stats ported fully.** The 12 new `MAP_LAST*` debug ops are backed
   by real WorldStat instrumentation: tick-section stopwatches plus
   bandwidth in/out counters, not stubs.
2. **Renumber-first structure.** The opcode renumber lands as one mechanical
   foundation commit-pair (the B3 pid-rename pattern) before any behavioral
   slice; the compiler name→value map is all-or-nothing.

## Design

### 1. Foundation — opcode renumber (lands first, own commits)

Re-derive every constant in `pkg/script/opcode.go` from the 244 enum
(ScriptOpcode.ts:1-457). Blocks are reshuffled wholesale; notable anchors:
INV base becomes `INV_ALLSTOCK = 4300`, debug base becomes `ERROR = 10000`,
the struct block (4700) is deleted, and `STRUCT_PARAM`, `SPLIT_*` (×5),
`HUNTALL`/`HUNTNEXT`, `PROJANIM_NPC`/`PROJANIM_PL`, and `MAP_MULTIWAY` move
into the core block. The same commit updates `opcode_map.go` (name table),
`opcode_pointers.go` (renamed keys + new rows per ScriptOpcodePointers.ts
diff), `String()`, handler-table keys, and `disasm.go` fallout.

- **Deletions:** `OpPushVarbit`/`OpPopVarbit` + their NAI-162 stubs in
  `handlers_b0_stubs.go` (244 deletes the enum entries —
  ScriptOpcode.ts:18-19 comments them out; closes
  NAI-162-D-STUB-PUSHVARBIT/POPVARBIT with a TS-deleted-upstream note);
  `OpMapLive` (replaced by MAP_PRODUCTION, §7); `OpStatTotal`
  (PlayerOps.ts handler deleted); `OpIfSetRecol` (§6).
- **Renames (adopt 244 names):** `OpHintPl→OpHintPlayer`,
  `OpLowMem→OpLowMemory`, `OpReadyAnim→OpBasReadyAnim`,
  `OpRunAnim→OpBasRunning`, `OpTurnAnim→OpBasTurnOnSpot`,
  `OpWalkAnim→OpBasWalkF`, `OpWalkAnimB/L/R→OpBasWalkB/L/R` (BAS_* family,
  PlayerOps.ts:926-960).
- **Pin test:** full name→value table extracted from a `cat -n` numbered
  listing of 244 `ScriptOpcode.ts`; every surviving name asserted, every
  removed name (`PUSH_VARBIT`, `POP_VARBIT`, `MAP_LIVE`, `STAT_TOTAL`,
  `IF_SETRECOL`, `HINT_PL`, `LOWMEM`, `READYANIM`, `RUNANIM`, `TURNANIM`,
  `WALKANIM`, `WALKANIM_B/L/R`) asserted absent.
- **Compiler coupling:** `pkg/pack/compiler/symbols.go` consumes
  `opcode_map.go`, so compiler and runtime renumber together; compiler
  tests/goldens pinning 225 numeric values update mechanically in the same
  change.
- **Format-window row:** packed `script.dat` opcode numbering shifts with
  the compiler. Byte-parity verification rides B6 against the 244 reference
  cache (consistent with `rev244-b1-format-window` and the B3 user decision
  that ALL windows close at B6).

### 2. Hunt-iterator unification

`ScriptState.playerIterator` is replaced by a unified `huntIterator` (TS
ScriptState.ts:124 — single field typed `IterableIterator<Entity>`). Go: one
field declared `huntIterator any`, holding either `*PlayerIterator` or
`*NpcIterator`; each consumer type-switches it, reproducing TS's
`instanceof` runtime checks with TS-matching error strings.

- **HUNTALL** (ServerOps.ts:53-61): keeps goscape's `PlayerIterator` —
  verified: TS `HuntIterator`'s PLAYER branch (ScriptIterators.ts:77-97) is
  line-identical to the deleted `PlayerHuntAllCommandIterator` (same
  descending zone-scan order, same `getAllPlayersSafe(true)`, same
  player-as-src LOS argument order) — but stores it in `huntIterator`.
  Citations move to ServerOps.ts + the HuntIterator lines; the equivalence
  is recorded in a comment.
- **HUNTNEXT** (ServerOps.ts:63-77): consumes `huntIterator`; non-player
  iterator → error mirroring TS `'[ServerOps] huntnext command must result
  instance of Player.'`.
- **NPC_HUNT** (ServerOps.ts:79-107): logic unchanged — moved file upstream;
  citation update only.
- **NPC_HUNTALL** (ServerOps.ts:109-119): now feeds `huntIterator`, not
  `npcIterator` → **NPC_FINDNEXT no longer consumes npc_huntall results**
  (behavioral; pinned).
- **NPC_HUNTNEXT** (ServerOps.ts:121-135, NEW op): consumes `huntIterator`,
  requires npc-yielding iterator (TS error string analog); pointer row
  `{require: find_npc, require2: find_npc, set: active_npc,
  set2: active_npc2, conditional: true}` (ScriptOpcodePointers.ts:160-166).
- `playerIterator` field deleted; `PlayerIterator` type survives as the
  HUNTALL engine.

### 3. Per-op behavioral deltas (each its own TDD slice)

- **HINT_NPC** (PlayerOps.ts:963-965): pops nid from the stack
  (`check NumberNotNull`) instead of reading `state.activeNpc.nid`. Pointer
  row unchanged (still requires active_npc — faithful to TS even though the
  handler no longer reads it).
- **HINT_PLAYER** (PlayerOps.ts:967-974): pops uid; `LookupPlayerByUID`
  (exists, server.go:1634); nil → silent return; `HintPlayer(p.Pid())`.
  Then delete the `activePlayer2()` helper in `active_player.go` — its sole
  caller is this handler (verified), exactly mirroring TS's deletion of the
  `activePlayer2` getter (ScriptState.ts 225:222-230). The `Self2` field,
  operand machinery, and setters stay (TS keeps `_activePlayer2`).
- **DB_GETFIELD** (DbOps.ts:97-119): tuple-index sub-selection removed —
  packed key now carries table+column only (`tableColumnPacked` rename);
  pushes ALL values of the column.
- **BOTH_DROPSLOT** (InvOps.ts:705-730): wealth event gains
  `recipient_id: toPlayer.account_id` + session via `isClientConnected ?
  client.uuid : 'disconnected'`; untradeables now **stop after delete**
  (`return` — no longer drop to the primary player).
- **INV_DROPALL** (InvOps.ts:764-780): untradeables **stop after delete**
  (`continue` — no longer drop to `state.activePlayer.hash64`).
- **Trade/stake wealth re-keys** (InvOps.ts:446-499): STAKE/TRADE events
  gain `recipient_id` + `toSession` fallback. B3 (`07e44a61`) threaded
  `RecipientID` into `WealthEventParams`; B4 updates these handler call
  sites where not already passing it.
- **NPC_STATHEAL** (NpcOps.ts:243-252): the full-heal
  `heroPoints.clear()` branch is deleted upstream — remove the Go analog.
- **MAP_BLOCKED** (ServerOps.ts:281-285): F2P-world gate removed
  (`handlers_map.go` `IsFreeToPlay` branch goes).
- **P_OPOBJ** (PlayerOps.ts:986-1001): guard becomes
  `!objType.op || !objType.op[type]` (catches undefined/null/empty) —
  align Go's nil/empty check; local renames adopted.
- **BUFFER_FULL** (PlayerOps.ts:198-203, NEW): pushes 0 (TS todo posture,
  Ash-tweet citation preserved) + pointer row (require active_player).
- **IF_OPENOVERLAY** (PlayerOps.ts:709-712, NEW): dispatch to B3's
  `Player.OpenOverlay(com)` via the ActivePlayer seam — raw `popInt`, **no**
  NumberNotNull check (−1 must pass through to clear); pointer row
  (require active_player/active_player2). Closes the B2 (`0ef495fb` table
  row) → B3 (`ebce9706` entity state + flush) → B4 chain.
- **Runner deltas** (ScriptRunner.ts:148-260): unknown-opcode error becomes
  `Unknown opcode <num>` (was name-mapped); player script-error line gains
  pid (`pid:<pid> name:<username>`); the debug-trace loops change
  `i >= 0 → i > 0` (frame 0 skipped in BOTH the player-facing and console
  traces).
- **ScriptFile** (ScriptFile.ts:136-143): `STANDALONE_BUNDLE` fileName
  branch — NOT-PORTED, platform-inapplicable (B3 taxonomy).

### 4. Verified NO-OPs / already-applied (decision rows, no code)

- **RANDOM / RANDOMINC** (NumberOps.ts:32-41): 244 switches
  `nextDouble()*n` → `JavaRandom.nextInt(max(0,n))`. Verified at the pin:
  `checkIsPositiveInt` rejects only `n<0` (JavaRandom.ts:58-62), and
  `nextInt(0)` hits the power-of-two branch returning 0 — so 244's clamp
  semantics ≡ goscape's existing `n≤0→0` / `n<0→0` handlers. The Go
  `math/rand/v2` vs Java-stream divergence is pre-existing posture
  (distribution-equivalent), recorded on the row.
- **STAT_RANDOM** (PlayerOps.ts:577): `floor(nextDouble()*256)` →
  `nextInt(256)` — distribution-identical; goscape already draws via
  `rand.IntN`-family (verify exact line during implementation; expected
  NO-OP).
- **GETQUEUE** (PlayerOps.ts:894-906): `.all()` → `head()/next()` iteration
  style only — Go slice iteration is cursor-free (B3 T12 pinned re-entry
  semantics); already routed through `QueueCount`. CLEARQUEUE likewise
  already routes through `UnlinkQueuedScript`. NO-OP row citing T12.
- **IF_OPENCHAT / IF_OPENMAIN_SIDE** (PlayerOps.ts:635-645): TS call-site
  renames `openChatModal→openChat`, `openMainSideModal→openMainModalSide` —
  **B3-shipped** (`d5a70fb1`; `active.go` already exposes
  `OpenChat`/`OpenMainModalSide`). Audit-listed, not double-applied.
- **PROJANIM_PL** `-player.slot-1 → -player.pid-1` (ServerOps.ts:331):
  B3-shipped (`fcc7e212` pid rename). Audit-listed.
- **Handler file moves** (NPC_HUNT/NPC_HUNTALL → ServerOps, SPLIT_* →
  ServerOps, STRUCT_PARAM → ServerOps, StructOps.ts deleted, AFK_EVENT /
  GETTIMER / STAT_ADVANCE / TUT_* / WALKTRIGGER / STAT / STAT_HEAL /
  STAT_SUB enum-position moves): goscape's handler-file organization is its
  own (handlers_*.go by domain); only citations and opcode values change.
- **InvOps whitespace-only hunks** (trailing spaces, InvOps.ts:231-234 et
  al.): NO-OP.
- **DebugOps/ScriptRunner import churn**: NO-OP.

### 5. Enum-only ops (no handler at the pin)

`IF_MULTIZONE` ("moved to engine, remove this"), `IF_OPENMAINOVERLAY`,
`PLAYER_FINDALLZONE` ("todo: replace with huntall"), `PLAYER_FINDNEXT`,
`LAST_COORD` — verified: zero `handlers/*` entries at `9aadcec4`. Go gets
constants + name-map entries + NAI-162-style typed-error stubs (the
established `handlers_b0_stubs.go` posture: error, not no-op, so a future
TS sync re-ports explicitly). `LAST_COORD` additionally gets its pointer
row (ScriptOpcodePointers.ts:528-531 — TS ships the row despite no
handler).

### 6. IF_SETRECOL full removal (closes the B2 deferral row)

Remove: script op constant + name/pointer rows + `handleIfSetRecol` +
`ActivePlayer.IfSetRecol` seam method (`active.go:400-403`) + the world
implementation + the wire table row `OpIfSetRecol` (`pkg/io/protocol/game/
server/prot.go:39`) + its name row (`prot.go:245`). TS deleted
`IfSetRecolEncoder.ts` + model at 244 (B2 row); the B2 decision row gets
its closure note.

### 7. Cycle stats (modules/world; backs the 13 new debug ops)

Per user decision §1 — full port of the WorldStat surface (pre-existing
gap; TS shape identical at both pins):

- `cycleStats`/`lastCycleStats [12]uint16` on the world server struct, index
  order per the TS enum (WorldStat.ts:1-14: CYCLE, WORLD, CLIENT_IN, NPC,
  PLAYER, LOGOUT, LOGIN, ZONE, CLIENT_OUT, CLEANUP, BANDWIDTH_IN,
  BANDWIDTH_OUT). **uint16 wrap faithful** (TS Uint16Array truncation).
- Ten section stopwatches mapped onto goscape's tick pipeline: WORLD
  (World.ts:619), CLIENT_IN (:690), NPC (:721), PLAYER (:775), LOGOUT
  (:845), LOGIN (:975), ZONE (:1004), CLIENT_OUT (:1144), CLEANUP (:1218),
  CYCLE measured at cycle end before telemetry (:487); `lastCycleStats`
  snapshot copy at :489-500.
- BANDWIDTH_IN: reset at client-in start (:629); incremented where the tick
  goroutine consumes input bytes (TS `NetworkPlayer.ts:83` `+= bytesRead` —
  verify TS context against a `cat -n` listing during planning; the Go
  increment point is the on-tick packet drain, keeping the counter
  tick-goroutine-owned and race-free).
- BANDWIDTH_OUT: reset pre-write in client-out (:1111); incremented per
  write (`NetworkPlayer.ts:241` `+= buf.pos`) on goscape's tick-side write
  path.
- Script seam: `WorldVars` (state.go:57) gains `LastCycleStat(stat int)
  int`.
- Handlers in `handlers_debug.go`: `MAP_PRODUCTION` (DebugOps.ts:16-18 —
  the old MAP_LIVE body relocated+renamed; reads the NODE_PRODUCTION
  config equivalent goscape's `handleMapLive` already reads) + 12
  `MAP_LAST*` (DebugOps.ts:20-68) reading `LastCycleStat`.
- goscape's pkg/telemetry posture is untouched — TS's prometheus `track*`
  lines exist at both pins (B3 NO-OP territory); only the script-visible
  `lastCycleStats` surface is new.

### 8. Count ops

`NPCCOUNT`/`ZONECOUNT`/`LOCCOUNT`/`OBJCOUNT` (ServerOps.ts:402-417, NEW) →
`WorldVars` gains `TotalNpcs`/`TotalZones`/`TotalLocs`/`TotalObjs`,
implemented per TS `World.getTotalNpcs` (World.ts:1734-1736) and
`GameMap.getTotalZones/getTotalLocs/getTotalObjs` (GameMap.ts:102-112).
The TS methods exist at BOTH pins; goscape lacked them — accessor-sized B4
prerequisite, not a gap row.

### 9. Handoff-flag dispositions

- **`World.addPlayer`** (flag #4): verified — no hunk in the B4 slice calls
  it; the B3 dead-at-pin row stands unchanged.
- **`staffModLevel >= 2`** (flag #5): verified — no new op in this slice
  gates on it (cheat gating was B2's ClientCheatHandler NO-OP).
- **getqueue/clearqueue** (flag #3): NO-OP per §4.
- **`Player.members` / `accountID`** (flag #6): consumed by the InvOps
  wealth re-keys (§3) as anticipated.

## Decision-row taxonomy (PORTING.md §B4 audit trail, established up front)

- **NOT-PORTED, platform-inapplicable:** ScriptFile STANDALONE_BUNDLE
  branch.
- **NO-OP:** RANDOM/RANDOMINC clamp; STAT_RANDOM draw; GETQUEUE/CLEARQUEUE
  iteration; handler file moves + enum-position moves; whitespace hunks;
  import churn.
- **B3-shipped, NOT double-applied (audit-listed):** IF_OPENCHAT /
  IF_OPENMAIN_SIDE renames (`d5a70fb1`); PROJANIM_PL pid (`fcc7e212`);
  `Player.OpenOverlay` + flush (`ebce9706`); RecipientID field threading
  (`07e44a61`).
- **Closed by B4:** B2 IF_SETRECOL deferral row; B2→B3 IF_OPENOVERLAY
  chain; NAI-162-D-STUB-PUSHVARBIT/POPVARBIT (TS deleted the enum entries).
- **New stubs (NAI-162 posture):** IF_MULTIZONE, IF_OPENMAINOVERLAY,
  PLAYER_FINDALLZONE, PLAYER_FINDNEXT, LAST_COORD.

## Tracker rows to create

1. **script.dat opcode-numbering window** — compiler + runtime renumbered
   together; byte-parity verification rides B6 (extends
   rev244-b1-format-window posture; B3 user decision).
2. **Cycle-stats gap closure** — pre-existing 225-era divergence closed;
   uint16-wrap fidelity + Go bandwidth-increment-point note recorded.
3. **NPC_FINDNEXT/npc_huntall split** — behavior pin recorded (TS-faithful,
   not a deviation; row documents the 225→244 semantic change for content
   authors).

## Testing

TDD pins per behavioral delta: full opcode name→value table (244-extracted)
+ removed-names-absent; hunt split (npc_huntall no longer feeds
NPC_FINDNEXT; HUNTNEXT/NPC_HUNTNEXT type-mismatch errors; NPC_HUNTNEXT
happy path); HINT_NPC pops nid; HINT_PLAYER uid lookup + missing-player
silent return; DB_GETFIELD pushes all column values (tuple-index gone);
untradeable-stop for BOTH_DROPSLOT + INV_DROPALL; wealth re-key call
shapes; statheal-no-heroPoints-clear; MAP_BLOCKED no-F2P-gate; P_OPOBJ
guard; BUFFER_FULL pushes 0; IF_OPENOVERLAY dispatches to OpenOverlay
(−1 passes); runner messages (Unknown opcode, pid line, frame-0 skip);
cycle-stats (section values recorded, lastCycle snapshot timing, bandwidth
accumulate/reset, uint16 wrap); count ops; enum-only stub errors; pointer
rows (NPC_HUNTNEXT conditional, LAST_COORD, BUFFER_FULL, IF_OPENOVERLAY;
IF_SETRECOL absent).

Reject-path tests seed earlier-gate prerequisites (B2/B3 mandate). Tests
pinning 225 numeric opcode values update mechanically with the renumber
commit.

## Gates & process

B1/B2/B3-proven cycle: plan via writing-plans (bite-sized TDD, exact TS
extraction commands as contracts) → subagent-driven execution (implementer
sonnet → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks) → full-suite gate + PORTING.md §B4
correspondence audit (every scope-diff file → commit/decision row) → final
whole-bundle integration review.

Implementer-prompt mandates (recurring defects): citations verified against
`cat -n` listings BEFORE writing; reject-path tests seed earlier-gate
prerequisites; final-review "missing X" findings verified before fixing.

Gates: `CGO_ENABLED=0 go build -trimpath ./...`; `go vet ./...`; full
`go test ./... -count=1` (real exit codes); `-race` (CGO_ENABLED=1) on
modules/world + pkg/script + pkg/pack/compiler. Commits on `rev-244` only,
`--no-gpg-sign`; subagents warned about phantom `??` dotfiles (never
`git add -A`).

## Risks

1. **Renumber blast radius** — every handler-table key, String() case, and
   numeric test pin moves at once; mitigated by the 244-extracted table pin
   landing in the same commit + the full-suite gate.
2. **Tick-loop instrumentation on the hot path** — first per-section
   timing surface in the tick pipeline; single-writer (tick-goroutine-owned
   counters) + `-race` gate.
3. **Compiler golden churn** — expected with the renumber; B6 byte-parity
   against the 244 reference cache is the real verdict.
4. **Hunt-split content fallout** — any content script pairing npc_huntall
   with npc_findnext changes behavior (TS-faithful); the pin test documents
   the new contract.
