# RESUME: rev-244 port — Bundle 5 (server/login/db)

Self-contained resume prompt. Written 2026-06-05 after Bundle 4 shipped.

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
3. `PORTING.md` §"rev-244 Bundle audit trail" — B1..B4 correspondence
   tables and decision rows; the B5-early worker-eval subsection.
4. `docs/superpowers/specs/2026-06-04-rev244-worker-multiworld-eval.md` —
   the B5-early deliverable; its §2-4, §5, §7 itemize B5's wire work.

## State: B1 + B2 + B3 + B4 SHIPPED

B1 (12 commits): io/cache primitives. B2 (28): wire protocol + rsbuf
damage2. B3 (48): engine core. **B4 (25 commits `b663bf63..16147b05`):
script runtime** — full 244 opcode renumber (413-entry pin test;
compiler renumbers with it via `script.ScriptOpcodeMap`), IF_SETRECOL
removed end-to-end (closes the B2 deferral; TS keeps 103/6
defined-but-unbound, goscape's fused Op row removed), unified
`huntIterator` + NPC_HUNTNEXT (drive-next-before-instanceof, NPC_FINDNEXT
split pinned), HINT_NPC/HINT_PLAYER 244 contracts (+ activePlayer2
deletion), DB_GETFIELD tuple-index removal, InvOps untradeable-stop +
wealth recipient re-keys (`ActivePlayer.RecipientSession` seam),
NPC_STATHEAL/MAP_BLOCKED deltas, BUFFER_FULL + IF_OPENOVERLAY (closes
the B2→B3 overlay chain), runner error-shape deltas (frame-0 skip on
BOTH paths), NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT, **full cycleStats/
lastCycleStats port (user decision)** + MAP_PRODUCTION + 12 MAP_LAST*
ops. Final whole-bundle integration review: READY (all 7 checks).
Gates 2026-06-05: build 0 / vet pre-existing-only / full test 0 /
`-race` 0 (world+script+pack).

**New PORTING-EXCEPTION:** `rev244-b4-bwout-reset` (BANDWIDTH_OUT reset
at tick start — goscape writes throughout the tick vs TS's single flush
pass; intent over line). Marker count 21.

**Open windows — ALL still consolidated at B6 (user decision, B3 spec):**
map-delivery, midi live-verification, B1 format window, **B4 script.dat
opcode-numbering window**, and the end-to-end 244-client login smoke.
244 reference-cache generation = B6 prerequisite. B6 must NOT
double-apply the B1 clientinterface writer pull-forward hunks.

## Next: Bundle 5 — server/login/db

The worker/multiworld **evaluation already shipped** (B5-early, before
B2): verdict = 244 worker architecture is transport-only, no game-client
wire impact; worker files NOT-PORTED (architecture-mapped to dskit).
B5 implements the itemized remainder (eval §2-4, §5, §7 + B3/B4 tracker
rows):

1. **Login-server rate-limit replacement** — 3-in-5s same-account+IP +
   45s hop timer (LoginServer.ts:234-269, 366-379). World-side limiting
   was REMOVED in B3 (`f4e7571e`); protection gap is explicit and open.
2. **`world_heartbeat` producer** (World.ts:1252-1275) — login gRPC
   proto surface.
3. **`messageCount` real query** (Messages.ts; proto field exists,
   wired to a stub).
4. **Friends `public_message` re-key + logger `report`/`input_track`
   message shapes** — seams compiling, adapters in place; private-sibling
   coordination per the telemetry split (dormant seams stay public).
5. **Prisma singleworld/multiworld schema deltas** → SQLite schemas
   (modules/login, modules/friends).
6. Formal NOT-PORTED rows for the worker files (deferred from the eval).

**B5 brief flags (collected during B3/B4):**
- `Player.members`, `Player.accountID`, `RecipientSession` all exist on
  the entity — available to any login/friends surface that reads them.
- The TRADE wealth event's recipient_items/value live on the telemetry
  `TradeCompletedEvent`, not the in-memory WealthEvent (B4 known-residual
  row) — if B5's logger work touches wealth shapes, that's the landing
  site.
- staffModLevel ≥ 2 tier exists (supermod login byte 19, B3 T18).

## Process (B1-B4-proven; repeat it)

Brainstorm → spec (commit) → plan (writing-plans; bite-sized TDD; exact TS
extraction commands as contracts) → subagent-driven execution: implementer
(sonnet) → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks; full-suite gate + PORTING.md
correspondence-audit at bundle end; final whole-bundle integration review.

**Bake into every implementer prompt (recurring defects, B2-B4-proven):**
- Every `// TS <File>.ts:<lines>` citation verified against a `| cat -n`
  numbered listing BEFORE writing (B4's plan itself carried two wrong
  GETQUEUE line numbers that the cat-n mandate caught — keep it).
- Every reject-path test must seed earlier-gate prerequisites.
- Final-review "missing X" findings can be false positives — verify first.
- Interface-method additions cascade into test fakes — PORTING.md
  §NEW-INTERFACE-METHOD-COMPILE-CASCADE.
- **NEW (B4 lesson): pkg/script-only gates MISS modules/world E2E
  fallout** — T4's HINT_PLAYER contract change broke a world E2E fixture
  discovered two tasks later (bisect `631737b7`); two sibling tests had
  been passing via silent uid-miss. Run the world suite in any task that
  changes a script-op contract the world tests exercise.

## Mechanics & gotchas

- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix.
  Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: CGO_ENABLED=1.
  Every commit: `--no-gpg-sign` + the Claude trailer.
- modules/world full suite ~2.5 min — not hung.
- Sandbox `git status` shows phantom `??` dotfiles (device-node masks) —
  never stage; never `git add -A`. Warn every subagent.
- Post-TDD stale LSP diagnostics are normal AND routinely false-alarm
  whole files (B4 saw this after EVERY interface change) — trust real
  build/vet/test runs only.
- TS citations cite the 244 pin; adopt 244 names on renames; deviations
  get PORTING.md rows; accepted divergences get
  `PORTING-EXCEPTION (<id>, …)` markers.
- Pre-existing gofmt-dirty files (NOT B4's): heropoints_test.go,
  world.go, pkg/pathfinder/routefinder/pathfinder1_test.go,
  pkg/script/handlers_number_test.go, pkg/telemetry/noop.go — standalone
  sweep candidates, out of bundle scope.
- Pre-existing dead code noted in B4 reviews (not B4's, cleanup-pass
  candidates): `activePlayerPointer`/`setActivePlayer` in
  pkg/script/active_player.go (never-wired NAI-era scaffolding;
  setActivePlayer maps to a live TS surface, keep-vs-delete needs a
  decision); `Npc.HeroPointsClear` seam (zero callers since B4 T7,
  retained forward-compat).
