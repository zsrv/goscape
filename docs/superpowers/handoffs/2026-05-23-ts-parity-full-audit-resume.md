# Resume — Full TS-Parity Codebase Audit

## Mission

Perform an **exhaustive, line-by-line, side-by-side function-level parity audit of the
entire goscape codebase against the TypeScript reference server (`Engine-TS`)**. Goal:
surface *every* deviation, missing translation, partial port, silently-dropped branch,
off-by-one, reordered operation, and stubbed/deferred item — anything where the Go code
does not do what the TS code does.

This is a **semantic** audit, not a structural one. Do **not** "verify the number of
instructions" or that a function "exists." Read the actual TS function and the actual Go
function and confirm they compute the **same result via the same logic, in the same order,
with the same edge-case handling**.

## Hard rules (non-negotiable)

1. **Trust no code comment.** Comments saying "TS does the same", "handled by X", "done
   elsewhere", "by design", "persistent by design", "matches TS World.ts:NNN", etc. are
   **claims to be verified, not facts**. Open the cited TS line and confirm. This codebase
   has a documented history of comments that lied (see Lessons below — multiple shipped bugs
   were guarded by a confidently-wrong "by design" / "TS does the same" comment).
2. **Verify the citation too.** When Go cites `File.ts:NNN-MMM`, open that exact range and
   confirm it says what the comment claims *and* that the surrounding context wasn't
   selectively quoted (a fix this arc came from a Go path that mirrored the *wrong* TS line).
3. **Surface every deferred/exception marker and re-verify it.** Treat `NAI-`,
   `PORTING-EXCEPTION`, `DEFERRED`, `DEVIATION`, `TODO`, `FIXME`, "stub", "not ported",
   "skipped" as audit targets. For each: is it *still* deferred? Is the deferral *real*
   (TS genuinely doesn't do it either), or is it a porting gap dressed up as a decision?
   ~444 Go files contain at least one such marker — none get a free pass.
4. **Two-parallel-paths trap.** When a value reaches the wire / DB / client via more than one
   code path (e.g. a stateful bridge **and** a source-accessor renderer), verify *which path
   actually serializes* and audit that one. Last arc, three commits fixed the wrong path
   because the real wire bytes came from a different function than the one being edited.
5. **A test passing does not mean the contract is right.** Tests in this repo have pinned
   *buggy* contracts. When a test asserts a behavior, cross-check that behavior against TS
   before treating the test as ground truth.
6. **Report, don't fix (by default).** This is an audit. Produce a deviation ledger. Only
   fix when the user explicitly says so, or batch trivially-safe fixes for approval. Keep the
   ledger authoritative.

## Reference paths & baselines

- **Go (subject):** `$HOME/Code/github.com/zsrv/goscape` — HEAD `6589f9d1` (TS-parity
  target; orientation arc closed at `1c7752da`).
- **TS (reference of record):** `$HOME/Code/github.com/LostCityRS/Engine-TS` —
  baseline commit `e1dea19f` (2026-02-23 "Synced with latest engine work"). Pin this SHA;
  if it has advanced, note the diff range so audit findings aren't confused with upstream drift.
- **Java client (wire-decode oracle only):** `$HOME/Code/github.com/LostCityRS/Client-Java`
  — use to adjudicate *wire-format* questions the TS side can't answer (opcode sizes, bit
  layouts, orientation/dstYaw math). Not the parity reference for logic.
- **Content (scripts/configs):** `$HOME/Code/github.com/LostCityRS/Content`.
- **Existing trackers:** `PORTING.md` (slim active), `docs/PORTING-CLOSED.md` (historical
  closed axes + shipped fixes). The audit should reconcile against these but **independently
  re-derive** — a row marked CLOSED is a claim to verify, not a skip.

## Audit surface — Go package ↔ TS source pairing

Walk every package. Suggested order: highest-risk game logic first, then engine/protocol,
then leaf utilities. Approximate pairings (confirm by reading, paths may not be 1:1):

| Go package (file count) | TS reference |
|---|---|
| `modules/world/` (109) | `src/engine/World.ts`, `src/engine/entity/*` (Player.ts, Npc.ts, PathingEntity.ts, Interaction.ts, hunt/, tracking/, BuildArea.ts, Inventory.ts, …), `src/engine/zone/`, `src/network/` |
| `pkg/script/` (46) | `src/engine/script/` (ScriptRunner, ScriptState, handlers/command tables, opcode impls) |
| `pkg/pack/` (34) + `pkg/pack/compiler/*` | RuneScript compiler + cache packers — pair against `@lostcityrs/runescript` and TS `src/cache`/pack tooling (note: pinned-commit-sensitive; see Arc 26 jagFileVersion=26) |
| `pkg/objtype/` (27) | `src/cache/config/*Type.ts` (NpcType, ObjType, LocType, SeqType, VarpType, …) |
| `pkg/rsbuf/` (16) | **Special:** TS `PlayerInfo.ts`/`NpcInfo.ts` are ~stubs delegating to the Rust crate `@2004scape/rsbuf`. Go reimplements the *Rust crate* natively. Parity reference here is the **Rust crate** / Java-client decode, NOT TS. Confirmed PARITY in Arc 15 — re-spot-check, don't deep-walk against TS stubs. |
| `pkg/pathfinder/*` (routefinder/collision/loc/reach/flag) | `src/engine/entity/` movement + `src/util` pathing (LineValidator, RouteFinder, CollisionFlagMap) |
| `pkg/io/packet/` (5), `pkg/io/protocol/` (+login) | `src/io/` (Packet, ISAAC), `src/network/` login/RSA. Adjudicate wire format vs Java client. |
| `pkg/io/isaac/` | TS ISAAC + client ISAAC (must match both). |
| `pkg/io/jagfile/`, `pkg/io/bzip2/`, `pkg/cache/`, `pkg/pixpack/` | `src/cache/`, `src/io/`. |
| `pkg/zone/`, `pkg/gamemap/`, `pkg/entity/`, `pkg/inventory/` | `src/engine/zone/`, `GameMap.ts`, entity/`Inventory.ts`. |
| `pkg/wordenc/`, `pkg/wordenc/encfilter/` | `src/wordenc/`. |
| `modules/friends/`, `modules/login/`, `pkg/friendspb`, `pkg/loginpb` | `src/friend.ts`, `src/login.ts`, friend/login servers. |
| `modules/asset/` | `src/web.ts` / asset serving. |
| `internal/dskit/*` | **Not TS** — port of Grafana dskit. Out of TS-parity scope (note as N/A). |
| `pkg/detection/`, `modules/detection/`, `modules/telemetry/`, `pkg/telemetry/`, `pkg/eventspb/`, `cmd/goscape-cli/` | **goscape extensions, no TS counterpart** — explicitly OUT of scope (these are the user's telemetry/detection work). Mark N/A; do not touch. |
| `cmd/goscape/`, `cmd/goscape/app/` | `src/app.ts` / bootstrap (loose pairing). |

## Method per function

For each TS source file paired to a Go file:
1. List the TS file's functions/methods top to bottom.
2. For each, locate the Go counterpart (grep by name/behavior, not just file).
3. Read both fully. Compare: control flow, branch conditions, operation order, integer
   widths/overflow, off-by-one (`<` vs `<=`, `-1` sentinels, fine-coord `*2+1`), default
   values, error/early-return paths, mask/flag emission, side effects on shared state.
4. Classify each finding:
   - **MISSING** — TS branch/function not ported.
   - **DEVIATION** — ported but computes differently.
   - **WRONG-PATH / DEAD-CODE** — ported into a path that doesn't reach the wire/DB.
   - **STALE-DEFER** — marked deferred but TS actually does it (real gap).
   - **CONFIRMED-EXCEPTION** — Go differs but TS genuinely also stubs/TODOs it (cite TS SHA+line; this is fine).
   - **COMMENT-LIE** — comment's claim contradicted by the cited TS.
   - **N/A** — no TS counterpart (dskit, detection, extensions).
5. Record with: Go `file:line`, TS `file:line`, classification, one-line evidence, severity.

## Output: deviation ledger

Create `docs/superpowers/audits/2026-05-23-ts-parity-audit.md` (new dir). One table per
package, columns: `Go loc | TS loc | Class | Severity | Finding | Evidence`. Maintain a
running summary at top: packages audited / functions compared / open deviations by severity.
Commit the ledger incrementally per package so progress survives session boundaries.

## Execution strategy

- **Subagent fan-out, one package (or one TS file cluster) per agent**, isolated read-only
  walks returning a findings table — proven at scale this project (Arc 27 detection engine,
  Arc 22/23 parallel worktree agents). Aggregate into the ledger.
- Audit is large: `modules/world` alone is 109 files vs `src/engine/entity/*` + `World.ts`.
  Plan multi-session. Keep the ledger as the durable state; each session resumes from the
  first not-yet-audited package row.
- Start with the riskiest game-logic surface where prior bugs clustered: `modules/world`
  (tick ordering, interactions, hunt/AI, masks, save), then `pkg/script` (handlers/operands),
  then `pkg/objtype`/`pkg/pack`, then protocol/io, then leaves.

## Lessons that shaped this audit (from `MEMORY.md` — read before starting)

- **#196** A test can pin a *bug* — verify the contract against TS, don't trust the test.
- **#197** Fork-N-ways: when TS shares one handler across N opcodes, Go must too; a bug fixed
  in one fork often lives in its siblings.
- **#198 / #201** A "by design" / "persistent by design" comment is a **prime suspect**, not
  reassurance. Multiple shipped bugs hid behind one.
- **#202** Two parallel compute paths can feed the same wire field; audit the one that
  actually serializes (this arc: renderer-accessor vs stateful bridge).
- **Comment-lie theme** (Arc 26 #189, Arc 30 #6): "handled by Y" / "TS does the same" claims
  have been wrong and load-bearing. Verify every one.
- **TS-state-first** (Arc 24 #177, Arc 25 #179): before logging a "missing port," confirm
  what TS actually does — several "gaps" turned out to be TS TODOs/stubs (CONFIRMED-EXCEPTION,
  not MISSING). Conversely, several "deferred" items were real gaps TS *did* implement.

## First steps next session

1. Read `MEMORY.md` index + the arc memos for context (esp. lessons above).
2. Confirm `Engine-TS` HEAD still `e1dea19f`; if advanced, scope the upstream diff.
3. Create `docs/superpowers/audits/2026-05-23-ts-parity-audit.md` skeleton (package rows +
   summary header).
4. Begin with `modules/world` vs `src/engine/World.ts` + `src/engine/entity/PathingEntity.ts`
   (the per-tick pass ordering and movement/interaction interleave — highest blast radius).
