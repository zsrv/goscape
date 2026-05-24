# Resume — Fix ALL TS-parity audit findings (most→least critical)

## Mission

Fix **every** deviation in the TS-parity audit ledger, working **most-critical → least-critical**,
checking each item off in a tracker as it lands. This is the implementation follow-up to the
report-only audit completed 2026-05-23.

- **Audit ledger (evidence of record):** `docs/superpowers/audits/2026-05-23-ts-parity-audit.md`
  — one cluster section (A–S) per package, with `Go loc | TS loc | Class | Severity | Finding | Evidence`.
- **Tracker (your live checklist):** `docs/superpowers/audits/2026-05-23-ts-parity-fix-tracker.md`
  — every finding as a `- [ ]` item, grouped by severity in fix order. **Check items off (`- [x]`),
  append the fixing commit SHA, and update the per-severity counts as you go.** The tracker is the
  durable progress state across sessions — resume from the first unchecked item.

## Reference paths & baselines (pinned — same as the audit)

- **Go (subject):** `/home/owner/Code/github.com/zsrv/goscape` — was at `6589f9d1` when audited
  (audit-doc commits `57731847`→`8d0db475` sit on top; engine code unchanged).
- **TS (reference of record):** `/home/owner/Code/github.com/LostCityRS/Engine-TS` @ `e1dea19f`.
- **Java client (wire oracle):** `/home/owner/Code/github.com/LostCityRS/Client-Java`.
- **Pathfinding reference:** the `@2004scape/rsmod-pathfinder` v5.0.4 crate source (TS delegates to it),
  at `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/` if present, else the WASM `.d.ts` enum dump.
- **Content (scripts/configs):** `/home/owner/Code/github.com/LostCityRS/Content`.

## Hard rules (carry over from the audit — do NOT skip)

1. **Re-verify each finding against TS before fixing.** The ledger is high-confidence but the rule that
   produced it still applies: open the cited TS line, confirm the intended behavior, then port it. A fix
   that mirrors the *wrong* TS line is worse than no fix (prior arc lost 3 commits to this).
2. **Tests in this repo have pinned BUGGY contracts.** Several findings are guarded by green tests that
   assert the wrong behavior. When you fix the code you MUST update the test to the correct (TS-matching)
   contract — do not leave the old assertion. Bug-pinning tests are flagged in the tracker with `⚠TEST`.
3. **Comment-lies:** several "by design"/"matches TS" comments were confirmed false. When you fix the
   underlying code, correct or delete the lying comment in the same change.
4. **Two-parallel-paths:** when a value reaches the wire/DB via more than one path, fix the one that
   actually serializes (this is exactly the unresolved `faceSquare` dispute — see below).
5. **RNG / concurrency changes:** if a fix changes RNG-dependent or tick-timing behavior, run the suite
   under `-race` AND check the real exit code (a single lucky pass is not green — Arc 30 lesson).

## Verification per fix (evidence before "done")

For each item, before checking it off:
1. Build: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
2. Test: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` (add `-race` for tick/queue/save/concurrency fixes).
3. If a CRITICAL touches the wire (CLIENT_CHEAT) or world-load (gamemap CSV, regenrate), prefer a
   **live repro** (run the server / exercise the path) over headless tests alone — the audit flagged
   CLIENT_CHEAT as "verify live first" precisely because headless tests fabricated the input.
4. Invoke the `use-modern-go` skill for Go style (per global CLAUDE.md).
5. Commit with `git commit --no-gpg-sign`, one fix (or one coherent batch) per commit, ending the message
   with the `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer. Commit to
   `main` (established project workflow). Then check the tracker item and record the SHA.

## Fix ordering & batching (the tracker encodes this; notes on non-obvious cases)

Work top-down through the tracker. A few items are coupled or need care:

- **C1 regenrate-214** — pure 1-line decoder add (`case 214: t.RegenRate = int(dat.G2())` in
  `pkg/objtype/npctype.go`); fix the lying comment at `npctype.go:331`. Do this FIRST (cheapest, verified).
- **C2 gamemap CSV** — the correct expander already exists at `pkg/pack/worldmap/csv.go:processCsv`; wire
  it into `gamemap.Init`/`loadCsvMap`. **Fix the `⚠TEST` `TestInitLoadsCsvMaps` in the same change** (it
  pins the fabricated comma format). **H10 (tile-vs-zone key, `multimap.go:12`) MUST be fixed together** —
  a correct CSV with the wrong key granularity still won't match zones.
- **C3 CLIENT_CHEAT** — **live-repro FIRST** ("all `::` cheats broken" is a big claim for a working
  server). If confirmed: remove the leading `G1()` at `handlers_game.go:594` AND the fabricated `P1(0)` in
  `handler_cheats_supermod_test.go:45`.
- **H3 shop restock** depends on **M10 (assureFullRemoval)** and **M11 (stockObj count-0 retention)** in
  `pkg/inventory` — port those inventory primitives first, then the restock tick pass.
- **H11/H12/H13 F2P-gates** are one coherent batch in `pkg/gamemap/load.go` (note: `bordersFreeToPlay`
  doesn't exist in goscape yet — port it from TS GameMap).
- **H1 NPC operand gap** — mirror the shipped player fix `c58cac51`: add an operand-aware
  `s.activeNpcResolved()` accessor + operand-aware `requireActiveNpc`, then swap ~35 call sites
  (full opcode list in ledger cluster J). Expect a few reactive test rounds.
- **Arithmetic group M15–M18** (DIVIDE/MODULO/SCALE truncate-toward-zero, SIN/COS truncate-not-round) —
  one batch; they share the `toInt32`/truncation semantics.
- **D1 faceSquare (DISPUTED) — DO NOT FIX until traced.** Two agents disagreed: HIGH stale-square leak vs
  intentional Arc-30 #202 force-emit architecture. It may share a root cause with **M2 (missing per-step
  `focus()`)**. Trace the live render path (rsbuf renderer + Java client decode) for a walked-away-from
  face target FIRST; only then decide fix-vs-document. Resolve this LAST among HIGHs, or split to its own
  investigation. Whatever you conclude, update both the tracker and ledger.

## DO NOT FIX (CONFIRMED-EXCEPTIONS — genuine TS-parity, listed at the bottom of the tracker)

These differ from TS but TS itself stubs/TODOs/delegates, or they're deliberate goscape architecture.
Touching them would *break* parity or waste effort. They're marked `EXCEPT` in the tracker with the
reason. Examples: NpcMode QUEUE1..20 (TS has them commented out), `Inventory.transfer` (no TS callers),
wealth-flush (NAI-162-D telemetry), DB find_db pointer-asymmetry, CategoryType resolve-at-pack,
DEFINE_ARRAY (Go extension; TS throws), pixpack quantize (NAI-213-D), gRPC/SQLite transport for
friend/login. Confirm against the ledger before deciding any "EXCEPT" item is actually a gap.

## First steps next session

1. Read `MEMORY.md` index + `arc_31_full_ts_parity_audit.md` (this audit's memo) for context.
2. Open the tracker; find the first unchecked item (should be C1).
3. Confirm baselines still pinned (Engine-TS `e1dea19f`); if Go HEAD advanced, rebase your mental model.
4. Fix C1 → verify → commit → check off → record SHA. Repeat down the tracker.
