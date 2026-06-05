# RESUME: rev-244 port — Bundle 4 (script runtime)

Self-contained resume prompt. Written 2026-06-04 after Bundle 3 shipped.

## Where you are

Multi-revision Go port of the LostCityRS Engine-TS server: `main` = codeless
docs hub; **`rev-244` = the active 225→244 porting branch**. Work on
`rev-244` only. The work list is the cross-pin diff
`git -C /home/owner/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4`
(local checkout sits AT the 244 pin `9aadcec4`). Pins:
`git show main:REFERENCES.md` §rev-244.

## Read these first (in order)

1. `git show main:PORTING-LESSONS.md` — porting philosophy, §3 gotchas,
   §4 citations, §5 gates.
2. `docs/superpowers/specs/2026-06-03-rev244-port-design.md` — umbrella:
   7 bundles, definition of done.
3. `PORTING.md` §"rev-244 Bundle audit trail" — B1 + B2 + B3 correspondence
   tables and decision rows; the B5-early worker-eval subsection.
4. `docs/superpowers/specs/2026-06-04-rev244-b3-engine-core-design.md` —
   B3's user decisions (deferred smoke, pid adoption) bind later bundles.

## State: B1 + B2 + B3 SHIPPED

B1 (12 commits): io/cache primitives. B2 (28 commits): wire protocol +
rsbuf damage2. B3 (48 commits `4384f3e0..b1408f4d`): engine core —
PlayerList/pid foundation (gap-db-datastruct-4 CLOSED), entity behavior
deltas (setAnim>=, regen countdown, modal re-shape, overlay plumbing,
energy /6, AFK literals, huntAll(), nid hoist), account_id +
InputTrackingBlob threading, world-side rate-limit REMOVED, in-engine
OnDemand (parse+cycle+lifecycle), 244 login handshake (op 14/15, supermod
19), CrcTable-from-FileStream, PreloadedPacks deleted, ondemand HTTP
routes re-based on FileStream, rs2.cgi token, MidiPack registry (midi
window closed code-side). Final whole-bundle integration review: READY.
Suite + build + vet + `-race` all green 2026-06-04.

**Open windows — ALL consolidated at B6 (user decision, B3 spec):**
map-delivery, midi live-verification, B1 format window, and the
end-to-end 244-client login smoke. The 244 reference-cache generation
(missed B1 de-risk) is a **B6 prerequisite**. Until B6: empty FileStream
cache → every login rejected out-of-date (rev244-b3-crc-compare row) and
OnDemand serves rejection frames — expected.

**PORTING-EXCEPTION ids added by B3:** `rev244-b3-crc-compare` (per-slot
login CRC compare vs TS CrcBuffer32 — strictly stronger, wire-identical),
`rev244-b3-ws-origin` (origin check kept vs upstream TODO-comment-out),
`rev244-b3-ws-ondemand-gate` (config recorded, not enforced — needs a
WS-origin marker on the world client).

## Next: Bundle 4 — script runtime (`pkg/script`)

Surface (umbrella): `ScriptOpcode.ts` (226/206), `ServerOps.ts` +175,
`DebugOps.ts` +55, `PlayerOps.ts` (40/72), `InvOps.ts` (35/31),
`ScriptOpcodePointers.ts` (27/12), `ScriptIterators.ts` DELETED (−58),
`ScriptRunner.ts` (4/6), `ScriptState.ts` (1/10), `DbOps.ts` (9/21),
`NumberOps.ts` (4/4), `StringOps.ts` (1/51), `StructOps.ts` (0/22),
`ScriptFile.ts` (6/1), `NpcOps.ts` (1/52).

**B4 brief flags (collected during B3):**

1. **IF_OPENOVERLAY script op** calls the now-existing
   `Player.OpenOverlay(com)` (modules/world/player_script.go; B3 T10).
   The wire row + encoder shipped in B2 (`0ef495fb`); B3 shipped entity
   state + per-tick flush. B4 wires only the op dispatch.
2. **IF_SETRECOL script-op removal** — B2 decision row: the encoder/table
   row stayed wired pending B4's removal of the script op (gone from 244
   ScriptOpcode.ts).
3. **getqueue/clearqueue**: B3 T12 pinned Go's slice re-entry semantics
   (`player_queue_reentry_244_test.go`) — clear-during-execute and
   append-during-execute both match TS; the TS 244 queue-cursor
   save/restore guard is structurally unnecessary in Go. Route the ops
   through `UnlinkQueuedScript` / `QueueCount`.
4. **`World.addPlayer` is NOT-PORTED dead-at-pin** — if B4's DebugOps
   (+55) or ServerOps (+175) turn out to call it, revisit the B3 row
   (verify callers in YOUR slice before assuming).
5. **staffModLevel ≥ 2 tier exists** (supermod login byte 19, B3 T18) —
   244 cheat/debug ops may gate on it.
6. **`Player.members` is populated** (B2 `010ee146`) and
   **`Player.accountID`** (B3 T13) — available to any 244 op that reads
   them.

**No client gate after B4** — the umbrella orders B6 after B4 (script-op
changes affect compiler output); the smoke rides B6.

## Process (B1+B2+B3-proven; repeat it)

Brainstorm → spec (commit) → plan (writing-plans; bite-sized TDD; exact TS
extraction commands as contracts) → subagent-driven execution: implementer
(sonnet) → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks; full-suite gate + PORTING.md
correspondence-audit at bundle end; final whole-bundle integration review.

**Bake into every implementer prompt (recurring defects, B2+B3-proven):**
- Every `// TS <File>.ts:<lines>` citation verified against a `| cat -n`
  numbered listing BEFORE writing (B3 implementers caught plan-brief
  line-number misreads TWICE this way — T11 D2, T24 #4; keep the mandate).
- Every reject-path test must seed earlier-gate prerequisites so the gate
  under test is the discriminating condition.
- Final-review "missing X" findings can be false positives — verify
  directly before fixing.

## Mechanics & gotchas

- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix.
  Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: CGO_ENABLED=1.
  Every commit: `--no-gpg-sign`.
- modules/world full suite ~2.5 min — not hung.
- Sandbox `git status` shows phantom `??` dotfiles (device-node masks) —
  never stage; never `git add -A`. Warn every subagent.
- Post-TDD stale LSP diagnostics are normal AND routinely false-alarm
  whole files — trust real build/vet/test runs only.
- A subagent cut off by a session limit may leave COMPLETE-but-uncommitted
  work — assess `git status` + vet + tests before redispatching (B3's T23
  was finished this way at near-zero cost).
- TS citations cite the 244 pin; adopt 244 names on renames; deviations get
  PORTING.md rows; accepted divergences get `PORTING-EXCEPTION (<id>, …)`.
- Pre-existing gofmt-dirty files (NOT B3's): heropoints_test.go, world.go,
  pkg/pathfinder/routefinder/pathfinder1_test.go,
  pkg/script/handlers_number_test.go, pkg/telemetry/noop.go — candidates
  for a standalone sweep, out of bundle scope.
- Known pre-existing nits (recorded, not B4 work): world-full pid
  exhaustion sends OpLogout without TS's "world full" session log
  (World.ts:920-936 — goscape splits cap-check from pid-exhaustion).
