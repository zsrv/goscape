# PORTING.md

Active tracker of open deviations between goscape (Go) and the upstream
LostCityRS Engine-TS TypeScript reference (checked out alongside this repo
as `../Engine-TS`).

This is the **active backlog**. Closed/shipped rows + audit-history +
goscape-extensions inventory live in [`docs/PORTING-CLOSED.md`](docs/PORTING-CLOSED.md).

Post-Arc-22 (May 2026) the doc was split: ~25 closed rows + 8 PARITY
subsystem tables + extensions tables moved out, leaving only items that
need attention. **Update PORTING.md as a side effect of any work that
touches a tracked region; move rows to PORTING-CLOSED.md once shipped.**

## Legend

- 🚧 **DEVIATION** — known divergence; may or may not need a fix
- ❌ **GAP** — TS has the feature, Go doesn't (or stub-only)
- 🔒 **SECURITY** — exploit surface; fix priority
- ⚡ **PERF** — performance hotspot
- 📋 **BACKLOG** — open work with shippable spec
- ➕ **EXTENSION** — goscape adds capability beyond TS

Sev: 🔥 HIGH (real-world incident risk) / ⚠ MED (correctness or future-fragility) / ℹ LOW (docs / minor drift / deferred-by-design).

---

## 🚧 Open deviations

| Sev | Go file:line | TS source | Status | Size | Note |
|---|---|---|---|---|---|
| _(none — ARCH-1 CLOSED 2026-06-13: tick error-recovery now TS-faithful (panic-only retry), backported from rev-274; see backlog note below)_ |

---

## ⚡ Open performance hotspots

| Sev | Location | Issue | Size | Note |
|---|---|---|---|---|
| _(none — both LOW rows closed 2026-06-03: PERF-1 tick player-snapshot scratch + PERF-2 hunt zone-iteration scratch/iterator; benchmarks + closure rows in [`docs/PORTING-CLOSED.md`](docs/PORTING-CLOSED.md) §Performance hotspots)_ |

---

## 📋 Open backlog

| ID | Area | Size | Notes |
|---|---|---|---|
| _(none — ARCH-1 CLOSED 2026-06-13: tick error-recovery now TS-faithful (panic-only retry at NPC lifecycle + world-script queue), backported from rev-274. Verified vs rev-244 pin `9aadcec4`: `Npc.ts:122-150` + `World.ts` world-queue try/catch + `ScriptRunner.execute` internal catch all identical to rev-274's `dee467c8`. Origin commits `c43ab876` (A) + `dc9d16b6` (B); spec `docs/superpowers/specs/2026-06-13-rev274-arch1-tick-recovery-design.md`)_ |

NAI-201 closed as TS-parity exception at `cf95634a` (Arc 23). Closure rationale in
the new `NpcModeMap` doc block (`pkg/objtype/npcmode.go`). TS itself has QUEUE1..20
commented out — Go matches that posture; the constants + dispatch + pack-parser
exist for forward-compat. Do NOT re-investigate unless TS uncomments.

NAI-30 / NAI-31 closed as Case-B TS-parity exception (Arc 25). Closure rationale
in `pkg/rsbuf/doc.go`. TS PlayerInfo.ts + NpcInfo.ts + their encoders total 50 LOC
of stub containers; TS delegates the actual rendering to the external `@2004scape/rsbuf`
Rust crate. Go reimplements that crate's logic natively across `pkg/rsbuf/` (NAI-30
Bundles 1-4 + NAI-31 Bundles 1-3 + NAI-32 Tasks 1-3 + NAI-116) and integrates it
into the tick loop. Do NOT re-investigate as an outstanding port.

---

## 📋 2026-05-28 fresh-audit MEDIUM backlog

Full evidence (quotes + LOW/refuted/confirmed-exception + 136 deferred markers) in
[`docs/superpowers/audits/2026-05-28-ts-parity-audit-fresh.md`](docs/superpowers/audits/2026-05-28-ts-parity-audit-fresh.md)
and its coverage addendum [`…-coverage.md`](docs/superpowers/audits/2026-05-28-ts-parity-audit-coverage.md).
Reference pinned at Engine-TS `e1dea19f`. CRITICAL/HIGH are in the Open-deviations table above.
Dupes across ledgers merged (aliases noted). Severities post-adversarial-verdict.

| id | Go file:line | TS source | Note |
|---|---|---|---|
| _(none — table emptied by script-core-1 closure on 2026-05-31; the 2026-05-28 fresh-audit MED 4-pack-bundle arc is complete)_ |

---

## Tracking conventions

### Row lifecycle

- **Adding a finding**: file:line on Go side + TS source citation + status + size estimate.
- **Closing a row**: flip status to ✅ FIXED with commit SHA, **then move to `docs/PORTING-CLOSED.md`** at next opportunity (don't let closed rows pile up here).
- **Audit-arc references**: `[[arc-N-name]]` links session memos in `~/.claude/projects/.../memory/`.

### Closure shapes

A row can close as any of the following — pick the shape that matches reality:

- **✅ FIXED** — literal port of the cited TS behavior into the Go code. The most
  common shape; required when the divergence has any observable behavior delta.
  Commit pattern: `fix(<scope>): port <thing> [<row-id>]` + a docs commit that
  removes the open row from PORTING.md and adds a closure row to
  `docs/PORTING-CLOSED.md` citing the fix SHA.
- **✅ EXCEPTION-DOCUMENTED** — divergence promoted to a formal `PORTING-EXCEPTION
  (<row-id>)` marker in code, with no behavior change. Use ONLY when the
  divergence is structural-only AND either (a) explicitly chosen by a written
  spec (cite the spec file) or (b) a real-divergence-deferral where the fix
  scope is too large for a single commit (cite the concrete failure mode + full
  closure scope so a future arc can pick it up). The marker comment MUST name
  the row id + cite TS source + state the rationale. Examples:
  bundle-17 staff-only rebuild broadcast (spec-cited), bundle-18 flat playerLoop
  (architectural-promotion), bundle-19 login-server-7 per-profile logout_time
  (real-divergence-deferral).
- **✅ NO-DIVERGENCE** — closure for false-deviation rows where the original
  audit marker was wrong. The cited divergence does NOT exist when traced
  carefully against current TS + goscape code. Comment-only commit removes the
  misleading marker + the PORTING.md row; PORTING-CLOSED.md row documents the
  re-verification. Example: NAI-91 (the "mask-reset-1-tick-lag" was a false
  claim — TS resetPathingEntity actually fires at tick end, same as goscape).
- **✅ NOT-A-GAP** — closure for rows mis-classified as gaps when goscape
  already implements the equivalent behavior, just structurally differently.
  Example: `logMessage` is implemented inline in `handleMessagePublic`, not a
  deferred field+tick-consume.

### Before classifying a row as EXCEPTION-DOCUMENTED

The 2026-06-01 LOW-row sweep retrofit found that several markers framed as
"functionally equivalent" or "structural-only divergence" actually had hidden
behavior deltas inside the methods they cited. Before accepting an EXCEPTION
classification:

- **Trace every method body the row's scope covers against the cited TS source.**
  A struct-level "fields lifted onto Player" marker can mask method-level
  behavior divergences (NAI-30 Bundle 4: `rebuildScenery` cleared activeZones
  but TS `BuildArea.rebuildNormal` doesn't — the structural-only marker missed
  this).
- **Trace every read site for the equivalent gate** when the row covers a
  Player-level state. A derived gate (`activeScript.Pointers&PAP`) can match TS
  in 99% of code paths but diverge in the edge case where TS clears the gate
  while keeping the underlying signal (NAI-111-D1: TS `closeModal` clears
  `protect=false` even mid-flight; goscape's derivation couldn't model this
  without re-triggering NAI-53 T3).
- **Re-derive the divergence claim from first principles** instead of accepting
  the marker's framing. The audit author may have seen the structural shape but
  not traced the consequences. If the claim is "TS X = goscape Y, functionally
  equivalent", verify ALL the operations on X have matching operations on Y at
  the call-site level, not just the data layout.

### When an EXCEPTION marker cites a historical regression

The PORTING-EXCEPTION block at `player_script.go:1066` (DEVIATION-NAI-111-D1)
cited the NAI-53 T3 incident as a reason to keep goscape's `activeScript`-
derived protect gate: clearing PAP in `CloseModal` had broken in-flight resumed
scripts (`tut_close` inside `[label,tutorial_complete]` caused `P_TELEJUMP` to
abort). The straightforward read ("can't make this TS-faithful without re-
triggering the regression") would have closed the row as CONFIRMED-EXCEPTION.

The actual fix shape was visible only after asking *what NAI-53 T3 actually
broke*: the historical bug cleared PAP-on-state, which doubled as the handler
pointerCheck source AND the player-protect gate — conflating two concerns. The
TS architecture has them SEPARATE (Player.protect gate vs script-state PAP
pointer). A literal TS-faithful port introduces a separate `Player.protect`
bool while keeping PAP-on-state intact. The regression CANNOT re-trigger
because the new code path doesn't touch PAP-on-state at all.

**Rule:** when a PORTING-EXCEPTION marker cites a historical regression, ask
whether the regression was about the SHAPE of the gate or about the CONFLATION
of concerns. If conflation, the TS-faithful split may sidestep the regression
entirely — re-derive the fix from TS architecture before accepting the
EXCEPTION classification.

### Bundle template (the 4-pack arc)

The MED bundle arc shipped 19 four-pack bundles plus singletons / clusters /
merged-aliases / coupled IDs. The template that emerged:

- **4 picks per bundle** is the default cadence; 5-pick bundles work when one
  pick is a tiny merged-alias closure (2 IDs in a single commit row).
- **File-disjoint hunks** are the core constraint. Two picks may touch the
  same file at line-range-disjoint hunks (function-level disjointness OR
  hunk-level disjointness within the same function — both shipped cleanly).
  Same-file picks at overlapping hunks are NOT bundle-feasible.
- **Fix-vs-docs ratio variants** observed:
  - 4-fix (bundles 1–14)
  - 2-fix + 2-docs (bundles 15–17, 19) — most common late-arc shape
  - 2-fix + 1-PORTING-pin + 1-CONFIRMED-EXCEPTION (bundle-16)
  - 1-fix + 3-docs (bundle-18) — most architectural-deferrals shape
  Picked per the architectural-EXCEPTION density of the remaining backlog.
- **Dedicated commits** (single fix, no bundle slot) are correct when an audit
  row is flagged "Big scope. Architectural; dedicated commit. Not bundle-
  feasible" — see script-core-1 + script-core-5 (the LAST MEDIUM-table row).

### Subagent-driven cadence for dedicated-commit ports

The CategoryType subsystem port (`46d43c9d`, 2026-06-01) ran the full subagent-
driven loop for a dedicated-commit port: **per implementation task, 1 implementer
→ 1 spec reviewer → 1 quality reviewer.** All 6 tasks passed both reviews on
first dispatch (minor / below-threshold nits only). When to reach for this
cadence vs. controller-direct verification:

- **Subagent-driven** fits dedicated-commit architectural ports (a new loader
  subsystem, interface wiring, a multi-file compile cascade) where each task is
  independently reviewable and a written spec exists up front. Write the spec
  commit FIRST so both reviewers have a contract to check against.
- **Controller-direct** (implement inline, single holistic Opus review at arc
  end) fits tiny mechanical bundle picks where a per-task review is overkill.
- Per task the three roles are: implementer makes the change + RED→GREEN test;
  spec reviewer checks TS-parity against the cited source; quality reviewer
  checks Go idiom + test rigor. Keep tasks file-disjoint where possible so the
  three roles never race on shared state.

### #274 flip-prediction (the no-op flip prediction)

`examples/bundled/goscape.yaml` and `pkg/util/build/build.go` are operator-flip
files routinely modified in the primary worktree (uncommitted local edits).
Branch work that doesn't touch these files MUST leave them byte-identical post-
FF. Pre-FF protocol:

- `md5sum examples/bundled/goscape.yaml pkg/util/build/build.go` to snapshot the
  current operator-flip state.
- Verify the branch's `git diff --name-only $(git merge-base main HEAD) HEAD`
  does NOT list either file.
- After FF, re-`md5sum` and confirm empty delta. Both files should still report
  as `M`/` M` in primary worktree status (uncommitted operator flips intact).

If a parallel-main advance touches `goscape.yaml` LEGITIMATELY (e.g., a new
config stanza for a different module), the rebase passes through cleanly as
long as the merge-base diff confirms zero overlap with my branch's files —
the parallel-advance edits land in the FF without interfering with the
operator's uncommitted local changes.

### Working agreement

- **Top severity first, one item at a time**, shipped end-to-end before moving
  on. Per item: branch `git checkout -b fix/<id> main`, systematic-debugging
  (confirm root cause + cite TS) then TDD (RED→GREEN), commit fix, then docs
  commit.
- Closing a finding = THREE edits: (a) remove the row from PORTING.md "Open
  deviations"; (b) extend the existing `Arc 27` line in PORTING.md "Recent
  audit history" with the closure + update "Next:"; (c) add a closure row to
  `docs/PORTING-CLOSED.md` (Security-findings table for DoS/security; "Open
  deviations / divergences" table for correctness/behaviour).
- `--no-gpg-sign` on every commit; `go` prefixed `GOPATH=$TMPDIR/go
  GOCACHE=$TMPDIR/go-cache`.

### Env quirks

- `go test -race` UNAVAILABLE: no C compiler (`runtime/cgo: gcc not found`).
  Use plain `go test ./...`; note the race gap in commit messages.
- Cannot `git checkout main` in this ts-audit worktree (main checked out in the
  primary worktree) — branch via `git checkout -b fix/<id> main`.
- Vanilla `git rebase` ignores `--no-gpg-sign` and global `commit.gpgsign=true`
  makes it die. Workaround: `git -c commit.gpgsign=false rebase main`.
- `.git/config "Device or resource busy"` on `branch -f`/`-D`/`-m` is BENIGN
  (the ref op still succeeds; only the config-file rewrite fails).
- Sandbox blocks primary-worktree write ops (FF-merge, `checkout --`) with
  "Read-only file system". Retry with `dangerouslyDisableSandbox: true`. Read-
  only git ops (`rev-parse`, `status`, `log`) work in-sandbox.
- Stale-gopls regularly fires phantom "undefined symbol" errors at lines NOT
  touched by the current edit — `go build ./...` is the ground truth. Don't
  chase the phantoms; verify via build.

### Sed-batched multi-file field renames

The NAI-30 Bundle 4 unflatten and NAI-111-D1 protect-bool refactor both used
sed for mechanical field renames across many test files. Pattern:

```
sed -i -E 's/\b([a-zA-Z_][a-zA-Z0-9_]*)\.(<field>|<field>|...)\b/\1.<wrapper>.\2/g' <files>
```

The alpha-prefix capture group `\b([a-zA-Z_][a-zA-Z0-9_]*)\.` handles `p.X`,
`p0.X`, `p1.X`, `pa.X`, etc. receivers correctly — zero false positives
observed across ~80 sed-batched touches. Verify post-sed with a complementary
grep for the unmigrated pattern returning empty:

```
grep -nE "\.(<field>|<field>)\b" <files> | grep -v "<wrapper>\\."
```

### NEW-INTERFACE-METHOD-COMPILE-CASCADE

Adding a required method to a shared interface (`Configs`, etc.) breaks every
implementor in the same compile unit at once — all test mocks (`fakeConfigs`,
`fakeDbConfigs`, `mockConfigs`, …) AND the production `serverConfigsView` impl.
First seen in med-bundle h-core-3 / h-config-5 (`591039eb`, `Configs.VarsType`),
re-exercised by the CategoryType port (`46d43c9d`, Task 2 atomic 8-file commit).

- **Cost is bounded** when the new method is conceptually parallel to an existing
  one — clone the closest sibling's impl (`VarsType` paralleled `VarnType`,
  silent zero-value default for nil-configs / out-of-range). It gets more
  expensive only when the method introduces a genuinely new shape.
- **Batch the whole cascade into one atomic commit**: interface decl + every
  impl (mocks + production) + the new helper + the dispatch update. Never leave
  the tree building half-broken between commits.
- `go build ./...` is the authoritative gate. In-editor diagnostics fire on the
  half-edited tree mid-cascade; trust the build, not the squiggles (see Env
  quirks → stale-gopls).

---

## rev-244 Bundle audit trail

Per-bundle record of the Engine-TS 225→244 port (`e1dea19f..9aadcec4`). Each
subsection maps every TS file in the bundle's scope to a goscape commit on the
`rev-244` branch or a deferral/no-op decision row, so the port can be audited
hunk-for-hunk against the upstream cross-pin diff.

### B1 — io/cache/util primitives (2026-06-04)

Scope diff = `git -C ../Engine-TS diff --numstat e1dea19f..9aadcec4 -- src/io src/cache src/util/DoublyLinkList.ts`.
Plan: [`docs/superpowers/plans/2026-06-03-rev244-b1-io-cache-primitives.md`](docs/superpowers/plans/2026-06-03-rev244-b1-io-cache-primitives.md).

**Decision rows**

- 🚧 `src/util/DoublyLinkList.ts` (new, +32) — **NOT-PORTED, dead-at-pin.** Zero
  consumers at `9aadcec4` (`git -C ../Engine-TS grep -l DoublyLinkList 9aadcec4 -- src tools`
  returns only the file itself). Revisit if a later bundle's TS begins importing it.
- 🚧 `src/io/Packet.ts` (±3/3) — **NO-OP.** Delta is import-path moves
  (`#/datastruct/{DoublyLinkable,LinkList}` → `#/util/…`, no Go analog) plus
  `static bitmask` made `private`. goscape's `pkg/io/packet/packetbit.go` bitmask
  is already unexported. No Go change.
- 🚧 `src/cache/CrcTable.ts` (±18/20), `src/cache/PreloadedPacks.ts` (−41, deleted
  upstream), `src/cache/DevThread.ts` (±3/2) — **DEFERRED.** CrcTable +
  PreloadedPacks rewiring is coupled to the new OnDemand engine → **B3**;
  DevThread `packAll` signature change → **B6**. Do not touch
  `pkg/cache/crctable.go` / `pkg/cache/preloaded.go` in B1.
- ✅ **Format-inconsistency window CLOSED at B6.** The config decoders now read
  the 244 cache format and `pkg/pack` writes 244 format (B6 parity test
  `pkg/packall/parity_test.go`, commit `a69634e7`). Closed artifacts:
  `PORTING-EXCEPTION (rev244-b1-format-window)` marker removed from
  `pkg/objtype/seqtype.go` (replaced with closure note, B6 cleanup commit);
  `TestLoadSeqTypes_FromPack` now runs against the Server244-ref reference cache
  (no longer skipped); `TestNewServer_LoadsWordencFilter` now uses the Server244-ref
  244-format pack + `t.Chdir` to project root for `data/raw/wordenc` resolution
  (no longer skipped).
- ℹ `pkg/io/gziputil` divergence note — TS `Uint8Array.subarray(off, off+len)`
  **clamps** out-of-range offsets/lengths; Go's `src[off:off+length]` slicing
  **panics**. Every upstream `compressGz`/`decompressGz` caller passes exact
  bounds, so no clamping shim is coded around it (recorded, not coded).
- ℹ `pkg/pack/clientinterface/pack.go` — the Component `trans` (P1) +
  layer-childCount g1→**g2** writer hunks (TS `PackShared.ts:267-274,428-431`)
  were pulled **FORWARD** from B6 into `e4e881d8` to keep the Component
  round-trip test coherent. **B6 must not double-apply** these two hunks.

**Correspondence audit** — every B1-scope file in the cross-pin diff → Go commit / decision:

| TS file (e1dea19f..9aadcec4) | numstat | goscape commit / decision |
|---|---|---|
| `src/io/FileStream.ts` | +225/0 | `8fcb734e` feat(io): port 244 FileStream dat/idx cache store |
| `src/io/GZip.ts` | +33/0 | `56f2698e` feat(io): port 244 GZip helpers with OS-byte zeroing |
| `src/io/PemUtil.ts` | +29/0 | `8ed60e04` feat(util): port 244 per-deployment PEM token |
| `src/io/Packet.ts` | +3/−3 | **NO-OP** (import moves + `bitmask` already-private in Go) |
| `src/cache/config/SeqFrame.ts` | 0/−43 | `7aa88cb0` SeqType/AnimFrame restructure (SeqFrame deleted → AnimFrame.instances) |
| `src/cache/config/SeqType.ts` | +52/−7 | `7aa88cb0` (frameCount, move anims, postDecode duration) |
| `src/cache/graphics/AnimBase.ts` | +1/−83 | `7aa88cb0` (slimmed alongside AnimFrame) |
| `src/cache/graphics/AnimFrame.ts` | +8/−212 | `7aa88cb0` (+ `b472b436` empty-Instances guard, `2d092b31` transform-decode test/marker) |
| `src/cache/config/Component.ts` | +13/−11 | `e4e881d8` Component decode — trans byte, g2 children, 244 field names (+clientinterface writer pull-forward) |
| `src/cache/config/NpcType.ts` | +12/0 | `d00a4b05` NpcType codes 99-102 |
| `src/cache/config/ObjType.ts` | +25/−8 | `d00a4b05` ObjType members gating |
| `src/cache/wordenc/WordEnc.ts` | +2/−8 | `e4eaec54` wordenc 244 load path — raw jag, unconditional |
| `src/cache/CrcTable.ts` | +18/−20 | **DEFERRED → B3** (OnDemand-coupled) |
| `src/cache/PreloadedPacks.ts` | 0/−41 | **DEFERRED → B3** (deleted upstream; OnDemand-coupled) |
| `src/cache/DevThread.ts` | +3/−2 | ✅ **CLOSED in B6** — `rev244-b6-packall-modelflags` NO-OP (packAll out-param read by no caller at pin; Go keeps `PackAll` with the slice internally; B6 decision row) |
| `src/util/DoublyLinkList.ts` | +32/0 | **NOT-PORTED** (dead-at-pin, zero consumers at `9aadcec4`) |

Every line of the cross-pin diff maps to a commit or decision above — no unmapped hunks.
(`a647f395` gofmt whitespace in `modules/world/server.go` is a follow-up cleanup with no TS counterpart.)

**Gates (2026-06-04):** full `go test ./... -count=1` exit 0; `CGO_ENABLED=0 go build -trimpath ./...` clean; `go vet ./...` clean except the pre-existing `pkg/util/build` self-assignment placeholders.

### B5 early deliverable — worker/multiworld evaluation (2026-06-04)

The umbrella spec's "written worker/multiworld evaluation" (a B5 deliverable,
executed before B2 per the ordering rationale) is complete:
[`docs/superpowers/specs/2026-06-04-rev244-worker-multiworld-eval.md`](docs/superpowers/specs/2026-06-04-rev244-worker-multiworld-eval.md).
**Verdict: the 244 worker architecture is transport-only (browser-bundle mode);
no game-client wire impact — B2 may freeze handler shapes.** Internal
login/friends/logger wire deltas map to goscape's gRPC protos as **B5** work
(itemized in the eval §2-4, §7); the worker files themselves are NOT-PORTED
(architecture-mapped to dskit modules — formal rows land with B5). Flags:
world-side `NODE_RATELIMIT_*` login limiting was removed upstream (B3 removes
`modules/world/login_ratelimit*.go`, B5 lands the login-server replacement —
B3's plan must carry the tracker row); the login handshake re-shape
(seed moved into the opcode-14 reply) stays in B3.

### B2 — wire protocol + rsbuf (2026-06-04)

Scope diff = `git -C ../Engine-TS diff --numstat e1dea19f..9aadcec4 -- src/network`
(115 files, +620/−946) plus the rsbuf crate delta
`git -C ../../2004scape/rsbuf diff 225 origin/244 -- src` (6 files, +64/−8 —
tip `1defefb`, verified identical to the published npm `244.1.0`).
Spec: [`docs/superpowers/specs/2026-06-04-rev244-b2-wire-protocol-design.md`](docs/superpowers/specs/2026-06-04-rev244-b2-wire-protocol-design.md).
Plan: [`docs/superpowers/plans/2026-06-04-rev244-b2-wire-protocol.md`](docs/superpowers/plans/2026-06-04-rev244-b2-wire-protocol.md).
27 commits, `b1cb81d4..a1d8ec70`.

**Decision rows**

- 🚧 TS `ClientGameProt.index` ctor field (NXT packet index) — **NOT-MODELED.**
  Zero readers at the pin (only written into `ClientGameProt.all`). goscape's
  `Ops [256]` stays opcode-keyed. Revisit only if later TS reads `.all`.
- ⚠ **Map-delivery window.** `REBUILD_GETMAPS` + `DATA_LAND/_DONE/DATA_LOC/_DONE`
  removed (`a6fa1e8f` — handler, senders, table rows; TS deletes the files and
  unbinds both repositories). 244 map delivery = engine OnDemand, which lands
  in **B3**. No client map delivery in between; the post-B2+B3 client smoke is
  the closing gate. No staff-rebuild `PORTING-EXCEPTION` marker existed in code
  (bundle-17's row was spec-cited only) — nothing to close.
- ⚠ **Midi-id window.** 244 MIDI_SONG/MIDI_JINGLE carry pack ids; the MidiPack
  name→id registry lands in **B3**. `midiIDByName` returns −1 (mirrors TS's
  `id !== -1` guard) → PlaySong/PlayJingle are silent no-ops.
  `PORTING-EXCEPTION (rev244-b2-midi-window)` at `modules/world/midi_encoders.go`.
- ⚠ **damage2 entity pull-forward (user-approved).** PathingEntity.ts:92-96,
  606-610 / Player.ts:1870-1890 / Npc.ts:475-494 are PORTED HERE (`2afa543c`):
  `damage2Amt`/`damage2Type`/`hitmarkSlot` + alternation in BOTH forks + per-tick
  resets. **B3 must NOT double-apply these hunks.** B3 also owns any wholesale
  `damageAmt`→`hitmarkDamage` rename and the World.ts:1041-1042/1086-1087
  compute-feed hunks (already wired here at tick.go).
- 🚧 rsbuf `renderer.rs` cache-index/count changes (8→9, 7→8, to_index shifts) —
  **NO-OP for Go.** goscape's Renderer composes per-slot payload slices, not
  per-prot cache arrays; the write order lives in `writeMaskPayloads` /
  `writeNpcMaskPayloads`. T12 spot-check verdicts: all 9 touched pkg/rsbuf files
  clean except the two write-order header comments (fixed in `1684f8f4`).
  Buf-struct `DamageTaken2/DamageType2` are lib.rs-parity bookkeeping — the wire
  reads the Source accessors (Arc-30 #202 insurance, documented in-code).
- 🚧 `IfSetRecolEncoder.ts` + model deleted upstream; repository unbinding —
  **DEFERRED → B4.** The IF_SETRECOL table row is value-identical across pins
  (103/6, unchanged); goscape's emitter stays wired until B4 removes the script
  op (`IF_SETRECOL` is gone from 244 ScriptOpcode.ts).
  **✅ CLOSED in B4 (`b7c9d08f`)** — script op + wire row + name row removed;
  opcode 103 unassigned in both (fused-Op nuance, see §B4 closure row).
- 🚧 `IF_OPENOVERLAY` — table row + `OpIfOpenOverlay` added (`0ef495fb`); the
  encoder is inline-at-call-site per goscape convention and the call site lands
  with **B4**'s IF_OPENOVERLAY script op.
  **✅ CLOSED in B4 (`0d9f0ad4`)** — script op dispatches to B3's
  `Player.OpenOverlay` (raw popInt, −1 passes through); B2→B3→B4 chain complete.
- 🚧 `InvButtonD.mode` byte — decoded and discarded, **matching TS's own
  posture** (`// todo: pass message.mode to script` at the pin). NO-DIVERGENCE;
  no PORTING-EXCEPTION marker (Arc-24 taxonomy: TS-matching ≠ exception).
  Revisit when TS consumes it.
- 🚧 TS `UpdatePid`→`UpdateUid192` model rename — **NOT-ADOPTED** (import alias
  for the same model/encoder; goscape sends inline).
- 🚧 'hidden' op-string gate drops (OpNpc/OpObj/OpLoc) — TS removed the
  `=== 'hidden'` comparisons; "hidden" strings now pass the handler gates in
  both TS and Go. NPC_HASOP-style script semantics are independent and
  unchanged.
- ℹ `LastLoginInfo.warnMembersInNonMembers` ported with its real derivation
  (`!cfg.NodeMembers && p.members`, TS Player.ts:2197); follow-up `010ee146`
  populated `Player.members` from the login response (TS World.ts:1937) — the
  field became wire-load-bearing here (UPDATE_PID byte 3 + the warn flag).

**Correspondence audit** — every file in the B2 scope diff → commit / decision:

| TS surface (e1dea19f..9aadcec4) | goscape commit / decision |
|---|---|
| `ClientGameProt.ts` (81/78) | `b1cb81d4` client renumber + `Opc*` constants + registration (+`803c43ce`, `3b64bae2` doc fixes) |
| `ServerGameProt.ts` (58/61) + `ServerGameZoneProt.ts` (10/10) | `0ef495fb` (+`77d3917c`); zone dup consts in `pkg/rsbuf/zone_encoders.go` renumbered same commit; five size-changed rows in `1e3bd50e` |
| `RebuildNormalEncoder/UpdatePidEncoder/LastLoginInfoEncoder/MidiSongEncoder/MidiJingleEncoder` | `1e3bd50e` table-row+emitter pairs (+`010ee146` members wiring) |
| `RebuildGetMaps*` (handler/decoder/model, −99) + `DataLand*/DataLoc*` (−89) + repository unbindings | `a6fa1e8f` removal |
| `OpHeld*` handlers/decoders/models | `1a4e71fe` (+`6285bd20`) |
| `InvButton*` handlers/decoders/models | `07ce53ca` (+`24f7d54a`) |
| `OpNpc*` | `1989f7ac` (+`65493b7d`) |
| `OpObj*` | `7fa35ba2` (+`10ceb1f6`) |
| `OpLoc*` | `b4cf5b25` (+`e45e92f9`) |
| `OpPlayer*` | `cca7155c` (+`f520bc00`) |
| `IfButtonHandler/Decoder`, `MessagePublicHandler/Decoder`, `MessagePrivateDecoder`, `ChatSetModeHandler`, `IfPlayerDesign*` (renamed), `TutorialClickSide*` (renamed), `NoTimeoutDecoder/model` | `f117b094` (+`9a952e56`) |
| `ClientCheatHandler` (STANDALONE_BUNDLE gating), `IdleTimerHandler` (import-only), `EventTracking`/`ResumePCountDialog` models (formatting) | **NO-OP** (verified in T11 review) |
| Model `com`→`component` renames (×14) + decoder import-only deltas (×8) | **NO-OP for Go** — goscape decodes inline; 244 names adopted in handler locals/comments |
| `ClientGameProtRepository.ts` / `ServerGameProtRepository.ts` | **NO-OP** — TS DI infra; the Go analog is the table + `handlers_game.go` registration (covered above) |
| `HintArrowEncoder/model` (playerSlot→pid rename) | **NO-OP** — wire unchanged; goscape keeps slot terminology (network-layer identity convention) |
| rsbuf `prot.rs`/`renderer.rs`/`info.rs`/`lib.rs`/`player.rs`/`npc.rs` (damage2 commit `1defefb`) | `1684f8f4` (+`c7207bff`, `b34675f7`); entity feed `2afa543c`; end-to-end wire pin `a1d8ec70` |

Every hunk of the scope diff maps to a commit or decision above — no unmapped hunks.

**Gates (2026-06-04):** full `go test ./... -count=1` exit 0;
`CGO_ENABLED=0 go build -trimpath ./...` exit 0; `go vet ./...` exit 0 (only
the pre-existing `pkg/util/build` self-assignment placeholders);
`-race` on `pkg/rsbuf` + `pkg/io/protocol/...` + `modules/world` green.
The B1 format-window skips remain (expire B6).

### B3 — engine core (2026-06-04)

Scope diff = `git -C ../Engine-TS diff --numstat e1dea19f..9aadcec4 -- src/engine
src/server/tcp/TcpServer.ts src/web.ts src/app.ts src/cache/CrcTable.ts
src/cache/PreloadedPacks.ts ':!src/engine/script'` (engine-script files are B4),
plus the `MidiPack` slice of `tools/pack/PackFile.ts` (engine-imported at the
pin) and the world-side login rate-limit removal.
Spec: [`docs/superpowers/specs/2026-06-04-rev244-b3-engine-core-design.md`](docs/superpowers/specs/2026-06-04-rev244-b3-engine-core-design.md).
Plan: [`docs/superpowers/plans/2026-06-04-rev244-b3-engine-core.md`](docs/superpowers/plans/2026-06-04-rev244-b3-engine-core.md).
48 commits, `4384f3e0..` (spec/plan + 24 implementation tasks + per-task review fixes).

**User decisions (recorded in the spec):** 244 runtime cache deferred to B6 —
all FileStream-backed serving built against synthetic fixtures; **the umbrella's
"post-B2+B3 client smoke" gate is AMENDED to B6**, where the map-delivery, midi,
and B1 format windows now ALL close. `pid` adopted wholesale (supersedes B2's
HintArrow keep-slot row — see that row's strike-note below).

**Decision rows**

- 🚧 `World.addPlayer()` (World.ts:1607-1610, new at 244) — **NOT-PORTED,
  dead-at-pin.** Zero callers at `9aadcec4` across src/tools (B1 DoublyLinkList
  precedent). Revisit if a later bundle's TS calls it.
- 🚧 `Npc.spawnTriggerPending` (Npc.ts:67, new at 244) — **NOT-PORTED,
  dead-at-pin.** Declaration only; zero consumers.
- 🚧 `STANDALONE_BUNDLE` branches, `WorkerFactory`/`createWorker`,
  `savePlayers` `typeof self` guard, the `createDevThread` wrapper churn —
  **NOT-PORTED, platform-inapplicable** (browser-bundle mode; worker-eval
  verdict — the formal per-file rows land with B5 as planned).
- ⚠ **`PORTING-EXCEPTION (rev244-b3-crc-compare)`** (markers:
  `pkg/cache/crctable.go`, `modules/world/server.go` handleLogin) — TS 244
  leaves `CrcTable` EMPTY (makeCrcs resets it, CrcTable.ts:13, never pushes)
  and the login check compares the single 32-bit `CrcBuffer32` hash
  (World.ts:2170). goscape — since rev-225 — validates per-slot
  (`slices.Equal(Table, client CRCs)`). Wire bytes identical; the per-slot
  predicate is strictly stronger (a crc32-colliding forged blob passes TS,
  never goscape). Surfaced by the T19 spec review; formerly an UNTRACKED
  divergence — now marked at both sites. Empty/absent cache → empty Table →
  every login rejected out-of-date until B6 produces a real cache.
- ⚠ **`PORTING-EXCEPTION (rev244-b3-ws-origin)`**
  (`modules/ondemand/websocket.go`) — TS 244 commented out the ENTIRE WS
  open() validation block (web.ts:125-154 TODO); goscape KEEPS its
  AllowedOrigins check (user-approved security posture — matching a
  TODO-commented upstream regression would weaken validation for zero
  fidelity gain).
- ⚠ **`PORTING-EXCEPTION (rev244-b3-ws-ondemand-gate)`**
  (`modules/ondemand/config.go`) — TS gates WS state-2 OnDemand routing under
  `NODE_WS_ONDEMAND` at the WS layer (web.ts:165-176). goscape's WS proxy
  cannot see the client state (the world conn handler owns the state machine);
  the `ondemand.websocket-ws-ondemand` config field (default false, matching
  TS) is recorded but **not enforced** until a WS-origin marker exists on the
  world client. Honest limitation, documented at the field.
- ✅ **`PORTING-EXCEPTION (rev244-b2-midi-window)` CLOSED** (`0f1ea964`) —
  MidiPack name→id registry loaded from `<ContentPath>/pack/midi.pack`
  (PackFile.ts:206, PackFileBase.ts:50-84 verbatim-name semantics);
  PlaySong/PlayJingle producers per Player.ts:1919-1933 (songs
  lowercase+underscore+strip, jingles lowercase ONLY — the B2-era jingle
  underscore→space normalization was corrected to the 244 contract).
  TS-faithful quirk reproduced as-is: the real midi.pack keys multi-word
  songs WITH SPACES while playSong normalizes to underscores → multi-word
  songs silently no-op in TS 244 too (upstream's own todo at Player.ts:1918).
  Live verification rides B6 — **CLOSED** (B6 live smoke PASSED; in-game music
  verified; midi registry silence fixed `b26d8dd5`).
- ✅ **`PORTING-EXCEPTION (gap-db-datastruct-4)` CLOSED** (`94f40331`) — the
  225-era playerLoop flat-slice exception is superseded by the faithful 244
  `PlayerList` port: pid-keyed registry, pid-order tick iteration,
  IP-windowed `getNextPid` (`(low octet % 20) * 100`, 100-wide priority scan,
  round-robin fallback floor pid 1). Closure notes remain in-code at the
  former marker sites.
- 🚧 **B2 HintArrow keep-slot row SUPERSEDED** — 244 renames the player
  identity field wholesale; `Player.slot`→`pid` adopted across 62 files
  (`fcc7e212`). Inventory/component/hitmark slots keep their names (genuine
  RS2 slots). The shared entity-interface `Slot()` method survives on BOTH
  types (Player→pid, Npc→nid; TS never unified these) with `Pid()` as the
  player-specific accessor.
- 🚧 **Reconnect socket-handover NOT-PORTED (pre-existing architectural
  divergence, T11-verified).** TS World.ts:863-892 finds the resident
  same-username player, swaps `other.client`, sends byte 15, calls
  `cleanupPlayerBuildArea(other.pid)` and `other.onReconnect()`. goscape
  reconnects via a FRESH Player + `addPlayer` (new pid, fresh BuildArea;
  NAI-182 design) — the 244 handover hunks (`other.session` deletion,
  pid-arg cleanup) have no landing site. The duplicate-login case is
  delegated to the login service (`LOGIN_RESULT_ALREADY_LOGGED_IN`).
- 🚧 **Wealth dedup-group NOT-PRESENT (pre-existing 225-era gap,
  T13-verified).** TS `wealthTransactionGroup` (present at BOTH pins,
  World.ts:2276-2284 re-keys it to account_id/recipient_id at 244) was never
  ported; the 244 re-key has no landing site. goscape's wealth events are
  in-memory append only (NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY).
- ℹ account_id threading (`07e44a61`): `Player.accountID` sourced from the
  existing `PlayerLoginResponse.account_id` proto field (no B5 dependency);
  goscape's zero-value default is 0 vs TS −1 on login-bypass paths (no
  consumer distinguishes; documented at client.go); the TS NetworkPlayer
  `'disconnected'` session state is not modeled (single-Player-type
  simplification, documented); logger/friends proto shapes UNCHANGED
  (dormant seam — adapters convert; B5/private-sibling owns message shapes).
- ℹ T24 residual NO-OP batch (verdicts + evidence in `7797c9f7`'s body):
  writeInner placeholders (Go one-pass length-then-payload, wire-identical),
  writeInner null-client guard, getInventoryFromListener tightening,
  takeStep null→−1 (the plan mis-attributed it to validateDistanceWalked —
  T24 corrected), processHuntFollow guard drop, Zone DoublyLinkList→LinkList
  swap, GameMap CSV split, app.ts lifecycle deltas, LinkList iteration style
  (T12 pinned the queue re-entry semantics: Go slice iteration is
  cursor-free — the 244 queue-cursor save/restore guard is structurally
  unnecessary; clear-during-execute and append-during-execute both match TS),
  shop-restock optional chaining.

**Correspondence audit** — every file in the B3 scope diff → commit / decision:

| TS surface (e1dea19f..9aadcec4) | goscape commit / decision |
|---|---|
| `src/engine/entity/EntityList.ts` (+23, PlayerList) | `60221c4c` playerList port (+`3856f3a2`) |
| `src/engine/World.ts` — playerLoop/players→PlayerList, getNextPid, getTotalPlayers, removePlayer | `94f40331` (+`1f26651b`); closes gap-db-datastruct-4 |
| `src/engine/World.ts` + entity family — slot→pid rename | `fcc7e212` (+`80f18b4d`) 62 files |
| `Player.ts:1857` + `Npc.ts:461` setAnim `>=` | `2f10deb6` (+`c66c5d71`) both forks |
| `Player.ts:692` energy /6; `:1823,1843` WORN rebuild; `:453` cleanup appearanceInv | `2442f390` (+`0886b6a8`) |
| `Npc.ts:514-532` regen countdown rework (regenInterval deleted) | `dc33a57b` (+`34614c66`) |
| `GameMap.ts:127-133` nid-allocation hoist (obj hoist NO-OP — no allocation side effect) | `80248224` (+`d67ca047` spawnBootNpc error-shape) |
| `World.ts:638` AFK literals 0.0833/0.1666; `Npc.ts:249-252` + `World.ts:613` huntAll() | `4891db4f` (+`7d4fd910`) |
| `Player.ts:1928-2021` modal opens (4× suspended-script clear deleted; openMainModalSide rename) | `d5a70fb1` (+`35c24519`) |
| `Player.ts:358-359,1955-1965` + `NetworkPlayer.ts:192-195` overlay plumbing (B4 wires the script op) | `ebce9706` (+`4f70df52`) |
| `Player.ts` 225:554-555 onReconnect masks-resync deletion | `e853dc47` (+`99b00c8f`); D2/D3 NO-OP rows verified |
| `Player.ts:892-919` queue-cursor save/restore | **NO-OP** — Go slice iteration cursor-free; pinned `ab8c3433` (+`9747a9e6`) |
| `Player.ts:306,633-645` + `NetworkPlayer.ts:252-263` + `SessionLog.ts` + `WealthEvent.ts` account_id | `07e44a61` (+`b6a91444`) |
| `InputTrackingBlob.ts` (+11) + `InputTracking.ts` re-shape + `World.ts:2343-2352` | `2f67fed2` (+`c81e56bc`) |
| `World.ts` rate-limit deletion (TTLCaches + NODE_RATELIMIT_*) | `f4e7571e` (+`d3aada38`); replacement → B5 (tracker) |
| `src/engine/OnDemand.ts` (+123) | `b2e7adac` parse/queues (+`e39cfc4a`), `02ce3929` cycle/send/lifecycle (+`59037a70`) |
| `World.ts:2143-2245` handshake (op 14/15, supermod 19) + `TcpServer.ts` seed removal + routing | `1f69f708` (+`0c1554a2`, `bf92e57b`); op 16/18 verified NO-CHANGE |
| `src/cache/CrcTable.ts` (B1-deferred) | `23cbbc02` (+`60066477` crc-compare exception markers) |
| `src/cache/PreloadedPacks.ts` (−41, deleted upstream; B1-deferred) | `59240b70` deletion + consumer dispositions |
| `src/web.ts:63-84` asset routes (versionlist new, models/.mid gone, ondemand.zip+build new) | `1de71136` (+`9e2a20fd`); missing-file 404 decision row (TS 500s) |
| `src/web.ts:101-104` token + `:125-176` WS gates + 225 `:134-137` WS seed removal | `130f6583` (+`8fc18ac2` os.Hostname fix); ws-origin + ws-ondemand-gate exceptions |
| `tools/pack/PackFile.ts:206` MidiPack + `Player.ts:1919-1933` producers | `0f1ea964`; closes rev244-b2-midi-window |
| `Player.ts:452-453` buildArea.clear in cleanup (both-pins gap, handoff flag) | `7797c9f7` |
| `World.ts:1252-1275` savePlayers `world_heartbeat` | **DEFERRED → B5** (gRPC proto surface; per-player autosave unchanged) |
| `World.ts:863-892` reconnect handover hunks | **NO-LANDING-SITE** (architectural divergence row above) |
| `World.ts:2276-2284` wealth dedup re-key | **NO-LANDING-SITE** (dedup never ported — row above) |
| `World.ts:1607-1610` addPlayer; `Npc.ts:67` spawnTriggerPending | **NOT-PORTED** (dead-at-pin rows above) |
| `src/app.ts` (16/19) | ✅ **CLOSED in B6** — NO-OP (worker/exception lifecycle → dskit); `BUILD_STARTUP_UPDATE` NOT-PORTED; `packAll(modelFlags)` → `rev244-b6-packall-modelflags` NO-OP; `createWorker` ×3 NOT-PORTED; `printError` catch NOT-PORTED (B6 correspondence audit) |
| Small entity files (CameraInfo/Entity/LocObjEvent/ModalState/NpcEventRequest/NpcQueueRequest/ObjDelayedRequest/PlayerQueueRequest, 1-2 lines each) + `Zone.ts` list swap + `GameMap.ts` CSV split | **NO-OP** (import moves / Linkable swap / T24 verdicts 6-7) |
| `entity/tracking/SessionLog.ts` (+1) / `WealthEvent.ts` (4/2) | covered by `07e44a61` |

Every hunk of the scope diff maps to a commit or decision above — no unmapped hunks.

**Tracker rows (open work this bundle creates/carries):**

1. **Login rate limiting absent everywhere** until B5 lands the login-server
   replacement (3-in-5s same-account+IP + 45s hop timer, LoginServer.ts:234-269/
   366-379). Accepted dev-branch window (worker-eval §5).
   **✅ CLOSED in B5** (`53715e4d` + `d5240e66` — see §B5 tracker rows).
2. **`world_heartbeat`** producer (World.ts:1252-1275) → B5 (login gRPC proto).
   **✅ CLOSED in B5** as NO-OP — dead-at-pin consumer (§B5 decision row).
3. **Map-delivery + midi + B1 format windows ALL close at B6** — the umbrella's
   post-B2+B3 client smoke is amended to B6 (user decision, spec §User
   decisions); the 244 reference-cache generation (missed B1 de-risk) is now a
   **B6 prerequisite**.
   **✅ CLOSED in B6** — B6 parity test (`pkg/packall/parity_test.go`, `a69634e7`)
   verified byte-identical output vs Server244-ref. Map delivery feeds live
   OnDemand (T19 client smoke gate). Midi window closed by `0f1ea964` (B3,
   already annotated above). B1 format window closed: seqtype marker removed,
   `TestLoadSeqTypes_FromPack` + `TestNewServer_LoadsWordencFilter` un-skipped
   (B6 cleanup commit).
4. **Logger/friends message shapes** → B5/private-sibling (seams compiling,
   adapters in place).
   **✅ CLOSED in B5 for the public repo** (`4e4f8192`/`704dad98`/`1d173abc`;
   proto/events deltas remain private-sibling-owned — §B5 tracker rows).
5. **`messageCount` real query** (Messages.ts) → B5 (proto field exists, wired).
   **✅ CLOSED in B5** (`83a8e6d6`).
6. **ws-ondemand-gate enforcement** needs a WS-origin marker on the world
   client (exception row above) — revisit if/when WS clients matter.

**Gates (2026-06-04):** full `go test ./... -count=1` exit 0;
`CGO_ENABLED=0 go build -trimpath ./...` exit 0; `go vet ./...` clean except
the pre-existing `pkg/util/build` placeholders; `-race` green on
modules/world + modules/ondemand + pkg/cache + pkg/script + pkg/rsbuf +
pkg/zone + pkg/io/protocol/... . Marker audit: 18 `PORTING-EXCEPTION`
mentions (3 new B3 ids: crc-compare ×2, ws-origin ×1, ws-ondemand-gate ×2 +
1 test cross-ref; rev244-b2-midi-window removed; gap-db-datastruct-4 now
closure-notes only).

### B4 — script runtime (2026-06-05)

Scope diff = `git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/engine/script`
(15 files, +584/−556) plus three B4-assigned externals: the B2-deferred
IF_SETRECOL wire-row removal, the B3-deferred IF_OPENOVERLAY script-op
dispatch, and the world-side `cycleStats`/`lastCycleStats` instrumentation the
new debug ops read (a pre-existing 225-era gap, closed here by user decision).
Spec: [`docs/superpowers/specs/2026-06-04-rev244-b4-script-runtime-design.md`](docs/superpowers/specs/2026-06-04-rev244-b4-script-runtime-design.md).
Plan: [`docs/superpowers/plans/2026-06-04-rev244-b4-script-runtime.md`](docs/superpowers/plans/2026-06-04-rev244-b4-script-runtime.md).
24 commits (spec/plan + 22 implementation/doc commits), `b663bf63..f093d4e6`.

**User decisions (recorded in the spec):** (1) **cycle stats ported fully** —
the 12 new `MAP_LAST*` debug ops are backed by real WorldStat instrumentation
(tick-section stopwatches + bandwidth in/out counters), not stubs, closing a
pre-existing 225-era gap; (2) **renumber-first** — the opcode renumber lands as
one mechanical foundation commit-pair (the B3 pid-rename pattern) before any
behavioral slice, because the compiler name→value map is all-or-nothing.

**Decision rows**

- ⚠ **`PORTING-EXCEPTION (rev244-b4-bwout-reset)` NEW** (markers:
  `modules/world/tick.go:99`, cross-refs `tick.go:272` +
  `world_stats.go:37`) — TS resets `BANDWIDTH_OUT` at World.ts:1111, the head
  of `processClientsOut`, its ONLY write pass, so the stat covers every byte
  written that cycle. goscape emits packets incrementally THROUGHOUT the tick
  (login resync, script-driven sends, relay-action friends/PM packets), so
  resetting at the TS line would silently drop everything written before
  client-out. Reset moved to tick start (`tick.go:109`) to preserve the stat's
  TS-INTENT ("bytes out this cycle") at the cost of the literal reset-line
  position. Intent-over-line.
- ✅ **B2 IF_SETRECOL deferral row CLOSED** (`b7c9d08f`) — script op constant,
  name/pointer rows, `handleIfSetRecol`, the `ActivePlayer.IfSetRecol` seam +
  world impl, AND the wire table row (`OpIfSetRecol`) + name row all removed.
  **Fused-Op nuance:** TS 244 KEEPS the ServerGameProt 103/6 constant
  defined-but-unbound (the encoder/model/repository-binding are deleted);
  goscape's `Op` type fuses constant+encoder, so removing the row is the
  consistent translation — opcode 103 is unassigned in BOTH (no other 244 op
  took it; `prot.go:30` records the gap). `IF_SETRECOL` is gone from 244
  ScriptOpcode.ts.
- ✅ **B2→B3→B4 overlay chain CLOSED** (`0d9f0ad4`) — IF_OPENOVERLAY dispatches
  to B3's `Player.OpenOverlay(com)` via the ActivePlayer seam: raw `popInt`,
  NO `NumberNotNull` check (−1 must pass through to clear), per
  PlayerOps.ts:709-712. Closes the B2 (`0ef495fb` table row + `OpIfOpenOverlay`)
  → B3 (`ebce9706` entity state + per-tick flush) → B4 (script op) chain.
- ✅ **NAI-162-D-STUB-PUSHVARBIT / NAI-162-D-STUB-POPVARBIT CLOSED**
  (`b663bf63`) — 244 comments out the enum entries (ScriptOpcode.ts:18-19);
  `OpPushVarbit`/`OpPopVarbit` + their NAI-162 stubs deleted with a
  TS-deleted-upstream note. (`OpStatTotal`, `OpMapLive`, `OpIfSetRecol` also
  deleted in the same renumber.)
- ℹ **OBJCOUNT naming** (`4268ba95`) — TS `OBJCOUNT` (ServerOps.ts, 1033)
  binds to `OpZoneObjCount` in Go to avoid a collision with the pre-existing
  `OBJ_COUNT`/3503 → `OpObjCount`. Distinct ops, distinct Go names.
- ℹ **Hunt guard-ordering** (`491822b8`, spec-review-caught) — HUNTNEXT /
  NPC_HUNTNEXT drive the iterator's `next()` BEFORE the instanceof/type check
  (TS ServerOps.ts:64-73): an exhausted wrong-type iterator pushes 0, it does
  NOT raise the type-mismatch error. The naïve check-then-drive ordering would
  diverge on exhaustion.
- ℹ **HINT fallout + Self2 observable** (`631737b7` + `1f2a0681`) — HINT_NPC
  pops nid; HINT_PLAYER pops uid via `LookupPlayerByUID`. The `activePlayer2()`
  getter + `requireActivePlayer2` were deleted (sole-caller mirror of TS's
  `activePlayer2` getter deletion). World E2E fixtures had pinned the 225
  contract (5 callers re-pointed; 2 had been silently passing via uid-miss);
  the `Self2`-binding lost its wire observable — coverage note recorded with
  the fixture re-point.
- ℹ **RecipientSession unreachable-`disconnected` adaptation** (`43a0b4dc`) —
  TS computes the recipient session via `isClientConnected ? client.uuid :
  'disconnected'`. goscape's `ActivePlayer.RecipientSession` honesty note
  records that the `'disconnected'` branch is production-unreachable (the
  client is never nilled in goscape's single-Player-type model); the seam
  carries the branch for fidelity but it cannot fire.
- 🚧 **TRADE `recipient_items`/`recipient_value` known-residual** — TS
  populates `recipient_items: toItems` + `recipient_value: toTotal` on the
  TRADE `addWealthEvent` call (InvOps.ts:494-495). goscape's in-memory
  `WealthEvent` (the B4 re-key target — `de628f37`) carries only
  `RecipientID`/`RecipientSession`; the recipient items/value land on the
  SEPARATE telemetry `TradeCompletedEvent` struct as `ItemsReceived`/
  `ValueReceived` (`handlers_inv.go:1793,1795`). This is a pre-existing shape,
  outside the B4 re-key scope — the `WealthEvent.RecipientItems` field exists
  (active.go:43) but is unpopulated on the TRADE path. Recorded, not papered
  over.
- ℹ **Cycle-stats attribution deviations** (`c321e11d` + `aeb70ba7`) —
  (a) TS leaves `processInfo` UNMEASURED (its timing is folded into no stat);
  goscape attributes it to `CLIENT_OUT`, the adjacent phase that consumes its
  rsbuf output, so goscape's CLIENT_OUT reads slightly higher than TS's
  (in-code note at `tick.go:256-260`). (b) `CYCLE` excludes the
  pre-existing-hoisted `processShutdown`/`autosavePlayers` passes (L3/L4 tick
  deviations: TS runs both at cycle END and measures them inside CYCLE,
  goscape hoists both to top-of-body BEFORE the CYCLE stopwatch starts).
  (c) uint16-wrap faithful (TS `Uint16Array` truncation). (d) BANDWIDTH_IN
  counted at the on-tick decode drain — the same delta the lastResponse logic
  reads (TS NetworkPlayer.ts:78-83), keeping the counter tick-goroutine-owned.
- ℹ **TotalNpcs O(n)** (`4268ba95`) — TS `World.getTotalNpcs` returns
  `npcs.count` (O(1), World.ts:1734-1736); goscape has no count field on the
  fixed `[8192]*Npc` array and scans for non-nil slots (O(n)). Acceptable on a
  debug-only count op (`server_varp.go:371`).
- ℹ **NO-OP / already-applied verdict batch** (verified this task; evidence in
  the controller report): RANDOM/RANDOMINC clamp (244 `nextInt(max(0,n))` ≡
  goscape's `n≤0→0`/`n<0→0` — `nextInt(0)` hits JavaRandom's power-of-two
  branch returning 0, `checkIsPositiveInt` rejects only `n<0`); STAT_RANDOM
  `nextInt(256)` ≡ `rand.IntN(256)` (distribution-identical, comment already
  faithful); GETQUEUE/CLEARQUEUE iteration-style only (Go routes through
  QueueCount/UnlinkQueuedScript; B3 T12 pinned re-entry semantics); ScriptFile
  `STANDALONE_BUNDLE` branch NOT-PORTED, platform-inapplicable (B3 taxonomy);
  handler file moves (SPLIT_* ×5 + STRUCT_PARAM → ServerOps, StructOps.ts
  deleted, NPC_HUNT/NPC_HUNTALL → ServerOps) citation-only — re-pointed in
  `f093d4e6`; enum-position moves (AFK_EVENT/GETTIMER/STAT_ADVANCE/TUT_*/
  WALKTRIGGER/STAT/STAT_HEAL/STAT_SUB) covered by the renumber, no citation
  change; InvOps whitespace hunks + DbOps/ScriptRunner/PlayerOps import churn
  one-line NO-OP. World.addPlayer (handoff flag #4) + staffModLevel≥2 (#5)
  verified with no B4-slice consumer — the B3 dead-at-pin rows stand.
- ℹ **B3-shipped, audit-listed (NOT double-applied):** IF_OPENCHAT /
  IF_OPENMAIN_SIDE call-site renames (`d5a70fb1`); PROJANIM_PL `pid` arg
  (`fcc7e212`); `Player.OpenOverlay` + flush (`ebce9706`); `RecipientID` field
  threading into `WealthEventParams` (`07e44a61`).

**Correspondence audit** — every file in the B4 scope diff (+ 3 externals) → commit / decision:

| TS surface (e1dea19f..9aadcec4) | numstat | goscape commit / decision |
|---|---|---|
| `src/engine/script/ScriptOpcode.ts` | +226/−206 | `b663bf63` full renumber (+`e4001b92`, `82020a6a` doc/gofmt sweep); closes NAI-162-D varbit stubs |
| `src/engine/script/ScriptOpcodePointers.ts` | +27/−12 | `b663bf63` (renamed keys + new rows: NPC_HUNTNEXT conditional, LAST_COORD, BUFFER_FULL, IF_OPENOVERLAY) |
| `src/engine/script/ScriptIterators.ts` | 0/−58 | `84b8ea2a` huntIterator unification (only `PlayerHuntAllCommandIterator` deleted; `HuntIterator` etc. survive — handoff correction) |
| `src/engine/script/ScriptState.ts` | +1/−10 | `84b8ea2a` `playerIterator`→`huntIterator` (`PlayerIterator` type survives as the HUNTALL engine) |
| `src/engine/script/ScriptRunner.ts` | +4/−6 | `294f5c24` (unknown-opcode `Unknown opcode <num>`, pid/name in player error log, backtrace `i>0` frame-0 skip — Go shares one impl for TS's two loops) + `c6005b60` (pid attrs on the world-queue player path too; nil-guard) |
| `src/engine/script/ScriptFile.ts` | +6/−1 | **NOT-PORTED** — `STANDALONE_BUNDLE` fileName branch, platform-inapplicable (decision row) |
| `handlers/ServerOps.ts` | +175/−10 | `84b8ea2a`/`491822b8` HUNTALL/HUNTNEXT/NPC_HUNTALL/NPC_HUNTNEXT; `4268ba95` NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT; SPLIT_* + STRUCT_PARAM + NPC_HUNT file-move citations (`f093d4e6`) |
| `handlers/DebugOps.ts` | +55/0 | `5cebb3e9` MAP_PRODUCTION + 12 MAP_LAST* (backed by `c321e11d` cycle stats) |
| `handlers/PlayerOps.ts` | +40/−72 | `631737b7` HINT_NPC/HINT_PLAYER; `0d9f0ad4` BUFFER_FULL + IF_OPENOVERLAY; `cb4fab32` MAP_BLOCKED/P_OPOBJ; STAT_RANDOM/GETQUEUE/CLEARQUEUE **NO-OP** (verdict batch); STAT_TOTAL handler deleted (`b663bf63`) |
| `handlers/InvOps.ts` | +35/−31 | `de628f37` untradeable-stop-after-delete ×2 + PVP/STAKE/TRADE wealth re-keys (+`43a0b4dc`, `eae5de71`); whitespace hunks **NO-OP**; TRADE recipient_items residual (row above) |
| `handlers/NpcOps.ts` | +1/−52 | `cb4fab32` NPC_STATHEAL full-heal heroPoints branch removed (`Npc.HeroPointsClear` seam now zero-caller, retained); NPC_HUNT/NPC_HUNTALL moved → ServerOps (covered above) |
| `handlers/DbOps.ts` | +9/−21 | `da896c1a` DB_GETFIELD tuple-index sub-selection removed → pushes full column; import churn **NO-OP** |
| `handlers/NumberOps.ts` | +4/−4 | **NO-OP** — RANDOM/RANDOMINC `nextInt(max(0,n))` ≡ goscape's clamp (verdict batch) |
| `handlers/StringOps.ts` | +1/−51 | SPLIT_* family moved → ServerOps (citation-only, `f093d4e6`); residual StringOps ops unchanged |
| `handlers/StructOps.ts` | 0/−22 | **DELETED upstream** — STRUCT_PARAM moved → ServerOps; Go `handleStructParam` citation re-pointed (`f093d4e6`) |
| **External:** IF_SETRECOL wire-row removal (B2-deferred) | — | `b7c9d08f` (closes B2 deferral row; fused-Op nuance above) |
| **External:** IF_OPENOVERLAY script-op dispatch (B3-deferred) | — | `0d9f0ad4` (closes B2→B3→B4 overlay chain) |
| **External:** `World` cycleStats/lastCycleStats instrumentation (225-era gap) | — | `c321e11d` (+`9a4d9b96` bwout-reset exception, `aeb70ba7` attribution honesty) |

Every hunk of the scope diff (+ the 3 externals) maps to a commit or decision above — no unmapped hunks.

**Tracker rows (open work this bundle creates/carries):**

1. **script.dat opcode-numbering window** — the compiler (`pkg/pack/compiler/
   symbols.go` via `opcode_map.go`) + runtime renumber together, so packed
   `script.dat` opcode numbering shifts. Byte-parity verification rides **B6**
   against the 244 reference cache (extends `rev244-b1-format-window`; B3 user
   decision that ALL windows close at B6).
   **✅ CLOSED in B6** — B6 parity test verified `script.dat` byte-identical vs
   Server244-ref reference (`a69634e7`; convergence commits `fee9d9f1`,
   `effb79f2`, `7bdd56e7`).
2. **Cycle-stats pre-existing gap CLOSED** (user decision) — the 225-era
   WorldStat divergence is closed with real instrumentation; uint16-wrap
   fidelity + the bwout-reset / processInfo-attribution / CYCLE-exclusion
   deviations are recorded (rows above). No further work; tracked as closed.
3. **NPC_FINDNEXT / npc_huntall split** — TS-faithful 225→244 semantic change
   (`84b8ea2a`): NPC_HUNTALL now feeds the unified `huntIterator`, so
   **NPC_FINDNEXT no longer consumes npc_huntall results**. Not a deviation;
   documented here for content authors — any content script pairing
   `npc_huntall` with `npc_findnext` changes behavior at 244. Pinned by the
   hunt-split tests.

**Gates (2026-06-05):** `CGO_ENABLED=0 go build -trimpath ./...` exit 0;
`go vet ./...` exit 1 — ONLY the pre-existing `pkg/util/build` self-assignment
placeholders (B1/B3 precedent); full `go test ./... -count=1 -timeout 20m`
exit 0 (modules/world 145s); `-race` (CGO_ENABLED=1) on
`modules/world` + `pkg/script` + `pkg/pack/...` exit 0 (modules/world 149s).
Marker audit: **21** `PORTING-EXCEPTION` mentions (was 18 at B3); 1 new B4 id
**`rev244-b4-bwout-reset`** with 3 mentions (1 code site `tick.go:99`, 2
cross-refs `tick.go:272` + `world_stats.go:37`). No B4 ids retired.

### B5 — server/login/db (2026-06-05)

Scope diff = `git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/server/login
src/server/friend src/server/logger src/server/worker prisma` (14 src files +
2 schema.prisma + migrations churn) plus the B3 tracker rows assigned here
(rate-limit replacement, world_heartbeat, logger/friends shapes, messageCount).
Spec: [`docs/superpowers/specs/2026-06-05-rev244-b5-server-login-db-design.md`](docs/superpowers/specs/2026-06-05-rev244-b5-server-login-db-design.md).
Plan: [`docs/superpowers/plans/2026-06-05-rev244-b5-server-login-db.md`](docs/superpowers/plans/2026-06-05-rev244-b5-server-login-db.md).
16 commits, `8fddfb4d..4e4f8192`.

**User decisions (recorded in the spec):** (1) schema delta ports
consumer-backed tables PLUS `account_session`/`wealth_event` as dormant
logger landing sites; website-only models get NOT-PORTED rows; (2)
login-server-7 closes fully with the legacy `account.logout_time` column
dropped; (3) friends goes multi-profile with per-message `profile` fields
(TS-shaped); (4) `world_heartbeat` + `WorldStartupRequest.profile` are
doc-closures.

**Decision rows**

- ✅ **`PORTING-EXCEPTION (login-server-7)` CLOSED** (`8fddfb4d`) — migration
  000005 adds per-profile `account_login.logged_out`/`logout_time`
  (backfilled from `account.logout_time`, which is dropped — closure step v);
  `setLoggedOut` stamps both per (account, profile) (TS LoginServer.ts:484-496);
  the M25 safety reject reads the per-profile column, eliminating the
  multi-profile spurious-reject failure mode. Closure notes in-code at
  db.go's setLoggedOut. Re-login preserves `logged_out`/`logout_time`
  (TS :438-457 writes only logged_in/login_time) — pinned (`d08963c0`).
- ⚠ **`PORTING-EXCEPTION (rev244-b5-startup-profile)` NEW** (marker:
  `modules/login/handler.go` WorldStartup) — TS 244 dropped `profile` from
  the world_startup message (LoginClient.ts:13-27) while LoginServer.ts:160-171
  still filters the account_login reset by the now-undefined value — the
  upstream reset matches nothing at the pin. goscape KEEPS the field and the
  per-profile reset (correct behavior over broken-line fidelity;
  rev244-b3-ws-origin precedent).
- 🚧 **`world_heartbeat` NO-OP, dead-at-pin consumer** — World.ts:1251-1273
  savePlayers posts it; LoginThread.ts:183-185 is `case 'world_heartbeat':
  break;` — the message never reaches the login server (no LoginClient
  method exists). Producer not modeled (B1 DoublyLinkList dead-at-pin
  precedent). Closes B3 tracker row 2. The handoff's "login gRPC proto
  surface" framing was a #177 over-estimate.
- 🚧 **Worker files NOT-PORTED, platform-inapplicable** (formal rows closing
  the worker-eval deferral): `src/util/WorkerFactory.ts` (+11),
  `src/appWorker.ts` (+8), `src/server/worker/WorkerServer.ts` (+50),
  `src/server/worker/WorkerClientSocket.ts` (+24), and the
  `STANDALONE_BUNDLE` branches in LoginThread.ts/FriendThread.ts/
  LoggerThread.ts — browser-bundle mode, architecture-mapped to dskit
  (worker-eval verdict; B3 NOT-PORTED taxonomy).
- 🚧 **Website-only schema models NOT-PORTED** (user decision 1): `newspost`,
  `tag`/`account_tag`, `message_tag`, `mod_action`, `input_report`/
  `input_report_event_raw` re-shape, the `account`
  2FA/email/oauth/notes/password_updated columns, the prisma `session`
  re-shape (goscape's session table already diverged with its own audit
  columns), and the hiscore PK reorder `(profile, account_id, type)`
  (index-order only, no behavior; goscape keeps `(profile, type,
  account_id)`). No goscape consumer for any of these — goscape has no
  website.
- ℹ **`account_session`/`wealth_event` dormant landing tables** (`8fddfb4d`)
  — schema-only (user decision 1), NO Go reader or writer in this public
  repo (the logger sink is slog-only); the private sibling owns consumers.
  The message/dormant tables carry NO FK constraints, mirroring the
  prisma-generated SQL (no `@relation` at the pin — verified against
  migration `20250303210826_message_centre`). The B4 TRADE
  recipient_items known-residual row is unchanged.
- 🚧 **friends `public_chat` account_id resolution NO-LANDING-SITE**
  (`062a3293`) — TS FriendServer.ts:287-305 resolves username→account_id
  against the shared account table; goscape's friends DB is
  username-keyed by design (DB-2 federation, no account table), so the
  username is stored directly. Pre-244 session_uuid rows preserved as
  `public_chat_legacy_225` (two-phase preservation pin `550bade5`).
  **RETIRED (2026-07-06):** DB-2 federation is retired — see the "DB-2
  federation RETIRED" entry in Recent audit history. `public_chat` now
  resolves `username` to `account_id` against the central database's
  shared `account` table (`LogPublicMessage`'s `accountIDByUsername`
  helper) and writes TS's exact 6-column row shape at this pin
  (`account_id`, `profile`, `world`, `timestamp`, `coord`, `message`); a
  missing account drops the row silently (`errAccountMissing`), matching
  TS's `executeTakeFirstOrThrow` throw + outer try/catch. There is no more
  account_id resolution to land.
- ℹ **FriendServerRepository internals NO-OP/N-A** — the orderBy
  `'f.created', 'asc'` → `'f.created asc'` Kysely-API form change, the
  addFriend select slimming (`account.members` no longer selected), and
  the inline 100-cap all target TS's account-id-keyed repository; goscape's
  username37-keyed repository has no corresponding lines (cap already
  inline as `friendListLimit`). Verified hunk-by-hunk.
  **RETIRED (2026-07-06):** DB-2 federation is retired — see the "DB-2
  federation RETIRED" entry in Recent audit history. The repository is
  now account-id-keyed like TS (owner and target both resolved via the
  shared `account` table), so these lines DO have a corresponding Go
  form — but this branch's own TS pin (`9aadcec4`), like rev-245.2's, has
  no members-aware cap split: `friendListLimit` stays a flat `100` for
  both `AddFriend`/`AddIgnore`, matching FriendServerRepository.ts:233/268
  exactly (no `account.members` read anywhere in the TS repository at
  this pin).
- ℹ **`LoginClient.remaining` drop already-aligned** — goscape never carried
  the field (worker-eval §2); the rest of the LoginClient.ts delta is
  field-order churn (NO-OP).
- ℹ **`TestHandler_WorldConnect_ProfileMismatch` deleted** (`a7234653`) —
  pinned the 225 WORLD_CONNECT profile reject that TS deleted at 244
  (verified: FriendServer.ts:92-103 has no profile comparison at the pin);
  replaced by an any-profile-accepted pin + multi-profile isolation pins
  (hard-rule #2: a test can pin removed behavior).
- ℹ **`friends.node-profile` config field retired** (`a7234653`) — TS 244
  deleted the server-side `profile = Environment.NODE_PROFILE` field; the
  friends server no longer validates a configured profile (the world's
  `world.node-profile` is what it SENDS, unchanged).
- ℹ **Logger seam adaptation scope** (`4e4f8192`) — `report` re-keyed
  session→username + world/profile/timestamp (LoggerClient.ts:48-67);
  `input_track` gains the world/profile envelope (LoggerClient.ts:76-86) —
  its TS `timestamp` param is NOT modeled (goscape's seam has no
  caller-supplied timestamp; the slog record carries its own time).
  `proto/events/v1` message shapes deliberately untouched — the
  telemetry-split posture (dormant seams stay public; the private
  sibling owns message shapes). `session_log`→`account_session` and the
  wealth-event reshape land only as the dormant tables above.
- ℹ **Rate-limit adaptations** (`53715e4d`) — TS's per-socket `uuid`
  becomes goscape's per-attempt sessionUUID (same one-row-per-attempt
  cardinality); TS's `timestamp: toDbDate(nodeTime)` becomes the server
  clock (no world clock on the RPC; both sides of the 5s window use the
  same clock — observable unchanged); TS's `if (account)` guard collapses
  (goscape returns earlier when no account and no auto-register).
- ℹ Pre-existing patterns NOT touched (quality-review verdicts, recorded
  as cleanup-pass candidates, no TS counterpart changed at 244): the
  subscriptions/worldSubscriptions `send` unlock-then-write drop window
  (pre-dates B5, identical shape at the base SHA); admin_bridge
  `context.Background()` posture; parse-failure silent-bypass on
  banned/muted/logout_time timestamp columns (all writers in-package).

**Correspondence audit** — every file in the B5 scope diff → commit / decision:

| TS surface (e1dea19f..9aadcec4) | numstat | goscape commit / decision |
|---|---|---|
| `src/server/login/LoginServer.ts` | +59/−4 | rate limit `53715e4d` (+`6804c746`), hop timer `d5240e66`, messageCount wiring `83a8e6d6` (+`5c05394a`), logged_out stamp `8fddfb4d` (+`d08963c0`) |
| `src/server/login/Messages.ts` | +37 | `83a8e6d6` SQL port (fixture matrix pins the unread semantics) |
| `src/server/login/LoginClient.ts` | 10/9 | world_startup profile drop → rev244-b5-startup-profile exception; `remaining` drop already-aligned; rest field-order churn **NO-OP** |
| `src/server/login/LoginThread.ts` | 27/13 | STANDALONE_BUNDLE **NOT-PORTED**; `world_heartbeat: break` → **NO-OP** row |
| `src/server/login/index.d.ts` | 0/−1 | covered by the `remaining`-drop row |
| `src/server/friend/FriendServer.ts` | 136/101 | multi-profile `a7234653` (+`30d65a1e`), public_chat re-key `062a3293` (+`550bade5`), profile carriage `704dad98`/`1d173abc` |
| `src/server/friend/FriendServerRepository.ts` | 13/13 | **NO-OP/N-A** (username37-keyed repo; verdicts row above) |
| `src/server/friend/FriendThread.ts` | 28/14 | STANDALONE_BUNDLE **NOT-PORTED**; `public_message` username re-key `704dad98`/`1d173abc` |
| `src/server/logger/LoggerClient.ts` | 13/5 | report + input_track seam re-key `4e4f8192` |
| `src/server/logger/LoggerServer.ts` (48/16) + `LoggerThread.ts` (31/17) + `WealthEventType.ts` (1/1) | — | dormant-seam decision row: `account_session`/`wealth_event` tables `8fddfb4d`; consumers private-sibling-owned |
| `src/server/worker/WorkerServer.ts` (+50) + `WorkerClientSocket.ts` (+24) | — | **NOT-PORTED** rows above (with WorkerFactory.ts/appWorker.ts from the B3 surface) |
| `prisma/singleworld/schema.prisma` (214/71) + `prisma/multiworld/schema.prisma` (213/71) + migrations churn | — | consumer-backed + dormant tables `8fddfb4d` (login) / `062a3293` (friends); website-only models **NOT-PORTED** row; multiworld delta observably identical for the ported models (covered-by-singleworld) |
| **External:** login wire mapping (World.ts:1871-1911 reply dispatch) | — | `7eb38361` RATE_LIMITED→16 / HOP_TIMER→9 enum + `loginResultToRS2` |
| **External:** `World.ts:1620-1628` logPublicChat username key + `:677-679` gate | — | `1d173abc` (+`96e5fa60` gofmt) — 225 session gate removed with the re-key |

Every hunk of the scope diff (+ the 2 externals) maps to a commit or decision above — no unmapped hunks.

**Tracker rows (B3 rows closed by this bundle):**

1. **B3 row 1 (login rate limiting absent everywhere) CLOSED** — the 244
   replacement pair shipped: 3-in-5s same-account+IP (`53715e4d`) + 45s hop
   timer (`d5240e66`) over the new `login` attempts table + per-profile
   logged_out columns.
2. **B3 row 2 (world_heartbeat) CLOSED** — NO-OP, dead-at-pin consumer
   (decision row above).
3. **B3 row 4 (logger/friends message shapes) CLOSED for the public repo** —
   report/input_track seam re-keys + public_message re-key shipped;
   `proto/events/v1` deltas remain private-sibling-owned (dormant seams
   stay public and compiling).
4. **B3 row 5 (messageCount real query) CLOSED** — `83a8e6d6`.

**Gates (2026-06-05):** `CGO_ENABLED=0 go build -trimpath ./...` exit 0;
`go vet ./...` — ONLY the pre-existing `pkg/util/build` self-assignment
placeholders (B1/B3/B4 precedent); full `go test ./... -count=1
-timeout 20m` REAL exit 0 (67 packages ok, zero FAIL); `-race`
(CGO_ENABLED=1) on modules/login (9.6s) +
modules/friends (12.7s) + modules/world (148.7s) exit 0. Final
whole-bundle integration review: **READY** (all 7 checks — spec
coverage, SHA-table accuracy, no double-application of B3 surfaces,
fresh-install migrations, no phantom staging, config consistency,
scope-diff coverage). Marker audit: **22** `PORTING-EXCEPTION`
mentions (was 21 at B4); +1 new id `rev244-b5-startup-profile` (1 mention);
`login-server-7` retired (0 mentions; closure notes remain in-code).

### B6 — pack pipeline re-baseline (2026-06-05)

Scope diff = `git -C ../Engine-TS diff e1dea19f..9aadcec4 -- tools/pack src/app.ts`
(all packer tool changes + the top-level orchestration entry point).
Plan: [`docs/superpowers/plans/2026-06-05-rev244-b6-pack-pipeline.md`](docs/superpowers/plans/2026-06-05-rev244-b6-pack-pipeline.md).

**Decision rows**

- 🚧 **`rev244-b6-packall-modelflags` NO-OP boundary** — TS `packAll(modelFlags)`
  out-param is read by NO caller at the pin (`app.ts:28-29`, `DevThread.ts:24-25`,
  `Build.ts:163-166` all discard the returned slice). Go keeps
  `PackAll(srcDir,outDir,dataPackDir,rawDir)` and owns the `[]int` slice
  internally. Closes the B1-deferred DevThread row + B3-deferred `app.ts
  packAll` row. Doc comment in `pkg/packall/packall.go`.
- 🚧 **`updateCompiler()`/BUILD_STARTUP_UPDATE NOT-PORTED** — TS
  `src/app.ts:14-16` downloads the RuneScriptKt JAR via `updateCompiler()`
  when `BUILD_STARTUP_UPDATE` env is set. goscape uses its own native Go
  compiler (`pkg/pack/compiler/`); no JAR download, no env gate needed.
  Architecture-mapped: dskit `--config.file` drives startup behaviour.
- 🚧 **`createWorker()` hunks in app.ts NOT-PORTED** — TS `app.ts:35-37`
  spawns login/friend/logger workers via `WorkerFactory`. goscape uses
  dskit module graph with one binary; these rows close the remaining B3
  app.ts worker-launch hunks (already doc-closed at B5 for WorkerServer /
  WorkerClientSocket).
- 🚧 **`printError` catch blocks NOT-PORTED** — TS `app.ts:27` catches
  `packAll` errors with `printError(err.message)`. Go propagates errors
  via `fmt.Errorf` wraps and the `cmd/goscape-cli` `runPack` function logs
  them via slog + returns exit 1. Architecture-mapped: dskit error
  propagation.
- ⚠ **`PORTING-EXCEPTION (rev244-b6-build-stamp)` NEW** (marker:
  `pkg/packall/packall.go`) — TS writes 4 bytes via `Packet.p4(Date.now()/1000)`
  (signed int32). Go writes `uint32` big-endian. Observable divergence only
  after Unix second overflow of int32 (~2038). Parity-exempt artifact.
- ⚠ **`PORTING-EXCEPTION (rev244-b6-ondemand-zip)` NEW** (markers:
  `pkg/packall/packall.go` x2) — TS uses `fflate.zipSync({level:0})` which
  embeds tool-specific zip headers and timestamps. Go uses `archive/zip`
  with `zip.Store` method and fixed `ModTime=time.Unix(0,0).UTC()` for
  determinism. Entry content is byte-identical; zip container bytes differ.
  Content-level parity.
- ✅ **`rev244-b1-format-window` exception CLOSED** — the seqtype.go
  `PORTING-EXCEPTION (rev244-b1-format-window)` comment replaced with a
  closure note (B6 cleanup commit); `TestLoadSeqTypes_FromPack` and
  `TestNewServer_LoadsWordencFilter` un-skipped and passing green against
  the Server244-ref 244-format reference cache.

**Correspondence audit** — `src/app.ts` B6 scope → commit / decision:

| TS surface (e1dea19f..9aadcec4) | numstat | goscape commit / decision |
|---|---|---|
| `src/app.ts:14-16` BUILD_STARTUP_UPDATE | +3 | **NOT-PORTED** (updateCompiler row above) |
| `src/app.ts:24` `packAll(modelFlags)` | port | `rev244-b6-packall-modelflags` NO-OP doc row |
| `src/app.ts:27` printError catch | +1 | **NOT-PORTED** (printError row above) |
| `src/app.ts:35-37` createWorker x3 | +3 | **NOT-PORTED** (createWorker row above) |
| `tools/pack/PackAll.ts` orchestration | +90 | this commit (`pkg/packall/packall.go`) |
| `tools/pack/PackAll.ts:73-75` build stamp | +3 | `rev244-b6-build-stamp` EXCEPTION |
| `tools/pack/PackAll.ts:77-90` ondemand.zip | +14 | `rev244-b6-ondemand-zip` EXCEPTION |

Marker audit: **22** `PORTING-EXCEPTION` mentions (was 22 at B5); +2 new ids
`rev244-b6-build-stamp` (1 mention) and `rev244-b6-ondemand-zip` (2 mentions);
`rev244-b1-format-window` retired (−1, seqtype.go closure-note replacement,
B6 cleanup commit); net 0 vs B5 actual. Prior B6 audit entry claimed 20 —
corrected here: the actual `grep -rn "PORTING-EXCEPTION (" modules pkg cmd
internal | wc -l` count post-cleanup is **22**.

**Correspondence audit** — `tools/pack/*` B6 scope → commit / decision:

| TS surface (e1dea19f..9aadcec4) | goscape commit / decision |
|---|---|
| `tools/pack/Build.ts` orchestration deltas | `c812b781` (PackAll orchestration); BUILD_STARTUP_UPDATE **NOT-PORTED** row above |
| `tools/pack/Compiler.ts` (deleted at pin) | superseded by native compiler; semantic deltas: `fee9d9f1` (constant case), `8ac35749` (bridge deltas 1-3), `effb79f2` (db_find withCount), `7bdd56e7` (local slot recycling per RuneScriptKt LocalTable) |
| `tools/pack/CompilerSymbols.ts` (new at pin) | `8ac35749` (WriteCompilerSymbols, 32/32 ref-parity) + `0c8a9e0c` (smoke-pack stage) |
| `tools/pack/NameMap.ts` + `tools/pack/Parse.ts` | `481ea70b` (CR normalisation) + `8e0beec0` (pal.png) + `d3cf8ec0` (4-field sprite meta) + `ccf1133a` (citation fixes) |
| `tools/pack/PixPack.ts` | `d3cf8ec0` (4-field sprite meta tolerance) + `ccf1133a` (citation fix) |
| `tools/pack/PackAll.ts` | `c812b781` (orchestration, 15 stages) + `a69634e7` (byte-parity gate) |
| `tools/pack/PackAll.ts:73-75` build stamp | `rev244-b6-build-stamp` EXCEPTION (see decision row) |
| `tools/pack/PackAll.ts:77-90` ondemand.zip | `rev244-b6-ondemand-zip` EXCEPTION (see decision row) |
| `tools/pack/PackFile.ts` registries + name verify | `2cfec7ea` (animset/map/midi registries, universal config-name verification) + `58619dbc` (wire name-verify into live pack path) |
| `tools/pack/chat/pack.ts` | `8e0beec0` (raw wordenc blob replaces four-txt Jagfile) |
| `tools/pack/config/IdkConfig.ts` | `9e3f0d5b` (model/head modelFlags bits) |
| `tools/pack/config/LocConfig.ts` | `9e3f0d5b` (model/origin/offset/scale modelFlags bits) |
| `tools/pack/config/SpotAnimConfig.ts` | `9e3f0d5b` (modelFlags bits) |
| `tools/pack/config/PackShared.ts` (modelFlags plumbing) | `b692e78b` (thread modelFlags through all packXxxConfigs) |
| `tools/pack/config/InvConfig.ts` | `b1d1ce01` (dense stock order, remove sparse-stock 225 pass) |
| `tools/pack/config/NpcConfig.ts` | `b1cb4832` (ambient/contrast/headicon/alwaysontop + model flags) + `658f2f3f` (alwaysontop unconditional emit fix) |
| `tools/pack/config/ObjConfig.ts` | `88a01023` (resize/ambient/contrast + model_index flags) |
| `tools/pack/config/SeqConfig.ts` | `619bd681` (preanim_move/postanim_move/duplicatebehavior) |
| `tools/pack/config/PackShared.ts` (8 CRC verifications) | `c812b781` (eight BuildVerify callbacks wired in pack_configs.go) |
| `tools/pack/graphics/pack.ts` | `27cfb146` (per-file gzip model/animset archives, drop 225 jag aggregation) |
| `tools/pack/midi/pack.ts` | `27cfb146` (per-file gzip midi archives, drop 225 jag aggregation) |
| `tools/pack/interface/PackClient.ts` + `PackShared.ts` | `d3cf8ec0` (CRC 316858560, `!layerId`, modelFlags; B1 `e4e881d8` hunks NOT double-applied — pre-244 hunks verified by-SHA) |
| `tools/pack/map/Pack.js` | `a824e218` (maps to gzip + archive 4, level/x/z npc-obj emission, npc-type validation) |
| `tools/pack/map/Worldmap.ts` | `3abb07a9` (packWater retired, underground-pass exception removed, floorcol additions, font members dropped, interleaved jag order) |
| `tools/pack/sound/pack.ts` | `8e0beec0` (+ `ccf1133a` citation fixes; sound CONFIRMED-EXCEPTION documented) |
| `tools/pack/sprites/media.ts` | `8e0beec0` (+ `ccf1133a` citation fix) |
| `tools/pack/sprites/textures.ts` | `8e0beec0` (+ `ccf1133a` citation fix) |
| `tools/pack/sprites/title.ts` | `8e0beec0` (+ `ccf1133a` citation fix) |
| `tools/pack/versionlist/pack.ts` (new at pin) | `0f348913` (new pkg/pack/versionlist, closes B4 script.dat numbering window) |
| **External:** `src/cache/DevThread.ts` | `c812b781` decision row — B1/B3 deferrals CLOSED via `rev244-b6-packall-modelflags` NO-OP boundary; `a977dd5a` flips B1 format window |
| **External:** `src/app.ts` | decision rows above (BUILD_STARTUP_UPDATE / createWorker / printError NOT-PORTED; packAll NO-OP row) |
| **External:** `src/util/RuneScriptCompiler.ts` (new at pin) | **NOT-PORTED** — TS wrapper for RuneScriptKt JAR invocation; goscape uses native Go compiler `pkg/pack/compiler/`; no JAR invocation needed |

Every hunk of the tools/pack scope diff (+ 3 externals) maps to a commit or decision above — no unmapped hunks.

**Gzip parity discovery (2026-06-05):** The B6 reference run revealed that TS's
`node:zlib.gzipSync` (bun 1.2.20) produces byte-identical output to Cloudflare's
zlib fork at commit `886098f3`, not to standard zlib. Go's `compress/gzip` at
default level produces divergent deflate streams. User decision: ship a bit-exact
port of `cf-zlib` deflate level 6 (`cfdeflate.go` / `cftrees.go`) as
`pkg/io/gziputil/`. CompressGz re-routed through `CompressCFGz`. Corpus
verification: 5,626/5,626 files byte-identical. `pkg/io/gziputil/` quality-review
cleanup `79b7bdd8`. REFERENCES.md pin `f43bfe85` on `main`.

**Live smoke record (2026-06-05/06; Client-Java @ pin `01f1608` via
`Server244-ref/javaclient` worktree):** Login, walking, shop/bank interaction,
in-game music, map crossing, NPC kill + despawn + respawn all PASSED. Three live
findings discovered and fixed during the smoke session:

| Finding | Fix SHA | Root cause |
|---|---|---|
| Client login rejected with reply 6 ("RuneScape has been updated!") | `4606660a` | Wire revision constant `225` not updated to `244`; goscape-specific constant with no TS counterpart file — its test pinned the stale value (PORTING-LESSONS §3: a test can pin a bug) |
| No in-game music (every midi_song/midi_jingle no-opped) | `b26d8dd5` | Empty `world.content_path` silently disabled the midi registry load with no log line; warn added + config pointed at pinned 244 content |
| NPC corpses never visually despawned from client view | `973e221b` | `processInfo` NPC compute loop had `if n.dead { continue }` guard preventing `ComputeNpc(active=false)` for dead RESPAWN-lifecycle NPCs; TS `World.ts:1066-1096` iterates dead NPCs deliberately |

**Reference cache:** `ebc46c05` — sha256 manifest (Engine-TS `9aadcec4` + Content
`e5d0282e` + RuneScriptKt-26 jar `38e16e2c`). 2,641 pack files + 32 `.sym` files.

**Byte-parity verdict:** FULL TREE — 2,671/2,671 reference files byte-identical
(acceptance test `pkg/packall/parity_test.go`, env `GOSCAPE_REF244_DIR`; `a69634e7`).
`ondemand.zip` 4,764/4,764 content-identical (container bytes differ per
`rev244-b6-ondemand-zip` EXCEPTION). goscape-extra server/maps csvs documented as
runtime-consumer copies.

**Windows closed by B6:**

1. **B1 format window (`rev244-b1-format-window`)** — `a977dd5a`; seqtype.go
   closure-note replaces the exception comment; `TestLoadSeqTypes_FromPack` and
   `TestNewServer_LoadsWordencFilter` un-skipped, passing green against 244 cache.
2. **B2 map window** — `a977dd5a`; 244 reference cache supplied.
3. **B3 midi window** — `a977dd5a`; 244 reference cache supplied.
4. **B4 script.dat numbering window** — `0f348913` (versionlist closes it) +
   `a977dd5a` confirmation.
5. **B1 DevThread deferral** — `c812b781` via `rev244-b6-packall-modelflags` NO-OP.
6. **B3 `app.ts` packAll row** — same NO-OP boundary.
7. **B3 client-smoke deferral** — live smoke PASSED (see above).
8. **All format windows** — full 244 cache shipped by PackAll.

**Definition-of-done checklist:**

- [x] (a) Correspondence audit — every scope file mapped above (no unmapped hunks)
- [x] (b) Live client smoke — PASSED (Client-Java `01f1608`; login/walk/shop/bank/music/map/npc all green; 3 live findings fixed)
- [x] (c) Byte-parity — FULL TREE 2,671/2,671 reference files identical; ondemand.zip content-identical
- [x] (d) Final gates (2026-06-06): `CGO_ENABLED=0 go build -trimpath ./...` exit 0; `go vet ./...` pre-existing-only (`pkg/util/build` self-assign, B1/B3/B4/B5 precedent); full `go test ./... -count=1 -timeout 20m` exit 0; `-race` (CGO_ENABLED=1) on pack/packall/gziputil/pixpack/script/protocol/world exit 0

### B7 — goscape-cli unpack (2026-06-06)

Scope diff = `git -C ../Engine-TS diff --numstat e1dea19f..9aadcec4 -- tools/unpack`
(31 files, +3,793 — all new at the pin) plus two out-of-slice src/ dependencies
ported on first Go consumption (`src/cache/graphics/Model.ts`, `Pix.ts` —
unchanged across the pin gap, invisible to diff slicing).
Spec: [`docs/superpowers/specs/2026-06-06-rev244-b7-unpack-design.md`](docs/superpowers/specs/2026-06-06-rev244-b7-unpack-design.md).
Plan: [`docs/superpowers/plans/2026-06-06-rev244-b7-unpack.md`](docs/superpowers/plans/2026-06-06-rev244-b7-unpack.md).
46 commits, `bcbeb72a..677da8e6` (+ this audit).

**Verification method — TS-output byte parity.** Each in-scope TS entrypoint
was run AT THE PIN (bun, Server244-ref/engine) against a scratch copy of the
pinned Content tree + the B6 byte-parity client cache copied to `data/unpack`;
the resulting changed-sets (sha256), positive write-sets (mtime-marker), and
normalized stdout were committed as per-family manifests
(`pkg/unpack/testdata/ref244/`, `bcbeb72a`). Every Go family carries an
env-gated parity test (`GOSCAPE_REF244_DIR`) replaying identical inputs through
the shared `pkg/unpack/unpacktest` harness — ALL 16 GREEN. Notable inputs
finding: `maps/ignore.csv` is MISSING at the Content pin, so the `worldmap.jag`
unpack input was generated at the pin via `tools/pack/map/Worldmap.ts` with an
empty `ignore.csv` shim (provenance immaterial to unpack parity — both sides
consume the same jag). TS-Logger stdout timestamps are normalized away
(`STDOUT-NORM`); stderr (console.time timings, console.error diagnostics) is
not a parity channel.

**Gates (2026-06-06):** `CGO_ENABLED=0 go build -trimpath ./...` exit 0;
`go vet ./...` pre-existing-only (`pkg/util/build` self-assign); full
`go test ./... -count=1` exit 0; `-race` (CGO_ENABLED=1) on
pkg/unpack/cmd/goscape-cli/filestream/objtype exit 0; env-gated parity:
all 16 unpack families + the B6 `pkg/packall` full-tree pack gate + the
gziputil corpus exit 0 (B7 did not disturb B6 parity).

#### B7 decision rows

| Decision ID | Description | Commit |
|---|---|---|
| `rev244-b7-png-bytes` | Jimp-vs-image/png encoder byte divergence — parity is decoded-pixel-level. The `unpacktest` harness pixel-compares `.png` files (manifest-side AND result-side extras) against the `<family>.post` reference snapshot instead of sha equality. Marker at the `pkg/unpack/internal/pix` PNG write site. | `f5f6667c` |
| `rev244-b7-synth-curation` | **NOT-PORTED ×6:** `sound/{Generate,Match,Reorganize,RenameFile,PrintDirectory,PrintOrderDirectory}.ts` (~267 lines) — one-off dev-machine curation utilities depending on artifacts goscape does not have (`Generate` shells out to `java -cp data/pack/rs2client.jar jagex2.client.SoundSynth` over a `data/pack/377-synth` dump; the rest read/curate a hand-made `data/pack/synths.json`). Same closure shape as B1's `DoublyLinkList` row. Revisit only if those artifacts enter the tree. | spec §Scope (user-approved) |
| `rev244-b7-pix-encode-half` | Pix.ts `packHeader`/`pack` (sprite ENCODE) intentionally unported — `pkg/pack/sprites` owns encoding (YAGNI; documented in `pkg/unpack/internal/pix/doc.go`, with the dropped `unpackJagToPng` explicit-dims / `preferHorizontal=false` dead branches). | `7ce5d54c` |
| `rev244-b7-sound-keepnames` | sound `Wave.unpack` keepNames=false branch (CRC pre-scan + reuse path, TS ~27-38/60-78) unported — unreachable from the entrypoint (default `keepNames=true`); documented in `pkg/unpack/sound/sound.go`. | `abd2a247` |

**Correspondence audit** — every B7-scope file → Go commit / decision:

| TS file (e1dea19f..9aadcec4) | lines | goscape commit / decision |
|---|---|---|
| `tools/unpack/checksum.ts` | +18 | `2a2aa92f` pkg/unpack/checksum |
| `tools/unpack/config/Common.ts` | +3 | `5031dd67` ConfigIdx |
| `tools/unpack/config/Compare.ts` | +69 | `2a2aa92f` config.Compare |
| `tools/unpack/config/FloConfig.ts` | +47 | `5031dd67` |
| `tools/unpack/config/IdkConfig.ts` | +149 | `3106b0b2` |
| `tools/unpack/config/LocConfig.ts` | +330 | `3106b0b2` (+ `726bd71c` active gbool fix, `2792322b` review fixes) |
| `tools/unpack/config/NpcConfig.ts` | +188 | `3106b0b2` |
| `tools/unpack/config/ObjConfig.ts` | +258 | `3106b0b2` |
| `tools/unpack/config/SeqConfig.ts` | +138 | `5031dd67` (+ `66b3c782` notes) |
| `tools/unpack/config/SpotAnimConfig.ts` | +142 | `3106b0b2` |
| `tools/unpack/config/Unpack.ts` | +368 | `5031dd67` (readConfigIdx) + `6c5ebdb5` (driver/names/reorder/merge/locmodels; + `acb15df9` review fixes) |
| `tools/unpack/config/VarpConfig.ts` | +33 | `5031dd67` |
| `tools/unpack/graphics/UnpackAnims.ts` | +117 | `4ea12286` (+ `9989046e` review fixes) |
| `tools/unpack/graphics/UnpackModels.ts` | +57 | `4ea12286` |
| `tools/unpack/interface/Unpack.ts` | +875 | `13e53d60` (binary decode) + `cc1426d8` (naming/.if export; + `006a6c10`/`be6e36ad` review fixes) |
| `tools/unpack/map/Unpack.ts` | +264 | `07c5e205` (+ `478b06e4` shared-harness parity test) |
| `tools/unpack/midi/Unpack.ts` | +43 | `dd618b46` |
| `tools/unpack/sound/Generate.ts` | +24 | **NOT-PORTED** (`rev244-b7-synth-curation`) |
| `tools/unpack/sound/Match.ts` | +99 | **NOT-PORTED** (`rev244-b7-synth-curation`) |
| `tools/unpack/sound/PrintDirectory.ts` | +30 | **NOT-PORTED** (`rev244-b7-synth-curation`) |
| `tools/unpack/sound/PrintOrderDirectory.ts` | +44 | **NOT-PORTED** (`rev244-b7-synth-curation`) |
| `tools/unpack/sound/RenameFile.ts` | +26 | **NOT-PORTED** (`rev244-b7-synth-curation`) |
| `tools/unpack/sound/Reorganize.ts` | +44 | **NOT-PORTED** (`rev244-b7-synth-curation`) |
| `tools/unpack/sound/Unpack.ts` | +230 | `abd2a247` (+ `c2581295` first-wins lookup) |
| `tools/unpack/sprite/media.ts` | +17 | `f5f6667c` |
| `tools/unpack/sprite/textures.ts` | +19 | `f5f6667c` |
| `tools/unpack/sprite/title.ts` | +39 | `f5f6667c` |
| `tools/unpack/versionlist/anim_index.ts` | +13 | `dd618b46` |
| `tools/unpack/versionlist/midi_index.ts` | +14 | `dd618b46` |
| `tools/unpack/versionlist/model_index.ts` | +62 | `dd618b46` (+ `f26df389` close-error fix) |
| `tools/unpack/worldmap/Unpack.ts` | +33 | `f955613d` (+ `da5d70ca` test cleanup) |
| `src/cache/graphics/Model.ts` | (dep) | `9020486e` pkg/unpack/internal/model (+ `e762a947` review fixes) — first Go consumer |
| `src/cache/graphics/Pix.ts` | (dep) | `7ce5d54c` pkg/unpack/internal/pix (+ `e67b9ea9`/`8d711f52`/`cee67343` — sheet-failure path, errorf threading, degenerate-data TS-equivalence) — first Go consumer; encode half `rev244-b7-pix-encode-half` |

Supporting infrastructure (no TS counterpart): parity manifests `bcbeb72a`;
`pkg/unpack/unpacktest` harness `9d87daa5`+`bf5ec7eb`+`6d87932e`+`d14a82c8`
(+spaced-path fix in `dd618b46`, png changed-set exemption in `f5f6667c`);
`cmd_unpack.go` CLI verb `f21fb0e0`+`2ff21fff` (every family wired —
call-site rule satisfied). Two shared-file changes rode along: `pkg/io/filestream`
decompress now decodes exactly one gzip member (`20b61dc5`, supersedes
`6c5ebdb5`'s broad tolerance — versioned cache entries are a complete member +
2-byte version trailer; node `gunzipSync` ignores trailing garbage =
`Multistream(false)`); `pkg/objtype/flotype.go` gained RGB/Texture/Overlay/
Occlude decode (additive, TS FloType.ts-faithful defaults, `f955613d`).

Every file of the B7 scope maps to a commit or decision row above — no
unmapped hunks.

#### rev-244 umbrella close-out (2026-06-06)

B7 was the LAST bundle. With it, every umbrella definition-of-done item
(spec `docs/superpowers/specs/2026-06-03-rev244-port-design.md`) is met:

- [x] (a) Change-for-change correspondence — per-bundle audit trails B1..B7
  above cover the full cross-pin diff `e1dea19f..9aadcec4`; B7 closes the
  final surface (`tools/unpack`).
- [x] (b) Live 244-client smoke — PASSED at B6.
- [x] (c) Pack byte-parity — FULL TREE at B6; re-verified green after B7.
- [x] (d) Suite green incl. `-race` — B7 final gates above.
- [x] Scope decisions: `tools/unpack` → `goscape-cli unpack` (B7); worker
  evaluation delivered (B5 early deliverable); B7 final integration review:
  READY.

**The rev-244 port is COMPLETE.** Non-blocking residuals carried forward
(not bundle work): `config.yaml` hardcodes the absolute Server244-ref
content path (B6 live-smoke local override); ~~`TestDecodeRealCacheBlob`
Arc-26 residual decoder bug in `pkg/script/file.go`~~ — RETIRED: the
"residual" was a phantom; the failure was the TEST's idx-walk bug (8-byte
idx header assumed; idx has 4), fixed at `0a068e40` (2026-05-22). The
decoder was never wrong. Retired at rev-245.2 (`0c5ce79a`, decoder verified
against 32,826 blobs across four era-caches) together with the all-blobs
test hardening, backported here; ~~`ValidateConfigPackNames` multi-orphan
error is map-iteration-ordered (T4-era minor)~~ — FIXED: names are now
checked in NameToID-ascending order, matching TS's id-ascending Set
iteration (PackFile.ts:117-121 @9aadcec4); fixed at rev-245.2 (`9c02d555`),
backported here with the MultiOrphanDeterministic pin test.

---

## Recent audit history (full log in `docs/PORTING-CLOSED.md`)

- Arc 21 — Phase 2 closure (NAI-162 13th-flag CLOSED).
- Arc 22 — ERR-1 + COV-1 N=2 parallel ship; R5 catalogued.
- Arc 23 — R5 mutex fix (`e804649e`), PORTING.md OPEN/CLOSED split (`d8e784e7`), pkg/pack ERR-1 sentinels (`9214f970`), emission stubs cleanup (`ee349b9b`).
- Arc 24 — NAI-201 closed as Case-B TS-parity exception (`cf95634a`); QoL sweep added 6 `PORTING-EXCEPTION` grep-markers (`8dd5f2ef` + `a63661fd`).
- Arc 25 — NAI-30 / NAI-31 closed as Case-B TS-parity exception. Scoping pass confirmed TS PlayerInfo.ts + NpcInfo.ts + encoders are 50 LOC of stub containers delegating to external `@2004scape/rsbuf` Rust crate; Go reimplements that crate's logic natively in `pkg/rsbuf/` (shipped across NAI-30 Bundles 1-4 + NAI-31 Bundles 1-3 + NAI-32 + NAI-116). Backlog row was stale. Closure rationale in `pkg/rsbuf/doc.go`.
- Arc 26 — **Fresh whole-codebase TS-parity audit (2026-05-28, report-only)**. ~67 units (57 main + 10 coverage) walked line-by-line vs Engine-TS `e1dea19f` with adversarial verification; distrusted all prior "fixed"/"by design" claims and re-verified. Ledgers: `docs/superpowers/audits/2026-05-28-ts-parity-audit-fresh.md` (+`-coverage.md`). Surfaced **2 CRITICAL + 5 HIGH + ~58 MEDIUM** (now in the tables above), ~150 LOW, 4 refuted, ~116 confirmed-exceptions, 136 deferred markers adjudicated. **The Arc-30 "100/100 fixed, byte-clean" posture was incomplete** — the worst finding (`gap-login-wire-1`, unauth login-RSA panic → whole-server crash) and `npc-ai-1` (AI trigger keyed by target not the acting NPC) both live in areas Arc-30 punted (rsbuf/pathfinder/AI-trigger). `npc-ai-1`, `pathing-1`, `gap-login-wire-1` independently re-verified against both codebases. No fixes applied. Session memo: `[[ts_parity_fresh_audit_2026_05_28]]`.
- rev-244 B1 — io/cache/util primitives ported (Engine-TS `e1dea19f..9aadcec4`): FileStream `8fcb734e`, GZip `56f2698e`, PemUtil `8ed60e04`, SeqType/AnimFrame `7aa88cb0`, Component `e4e881d8`, NpcType/ObjType `d00a4b05`, WordEnc `e4eaec54`. DoublyLinkList NOT-PORTED (dead-at-pin); Packet NO-OP; CrcTable/PreloadedPacks→B3, DevThread→B6. Full hunk-for-hunk correspondence audit in the §rev-244 Bundle audit trail above.
- Arc 27 — **Fresh-audit fix arc** (working CRIT→HIGH→… one at a time off `main`). `gap-login-wire-1` (CRIT, unauth login-RSA panic → whole-server crash) **FIXED** `6ac72985` on branch `fix/gap-login-wire-1`: per-connection `recover()` in new `Server.serveConn` (TS `TcpServer.ts:29-41` parity) + root-cause/fix-contract tests (RED→GREEN, `-race` clean). `npc-ai-1` (CRIT, NPC AI `ai_op*`/`ai_ap*` triggers keyed by the TARGET's identity instead of the acting NPC's `this.type`+`type.category`) **FIXED** `9677cc71` on branch `fix/npc-ai-1`: all 8 `fireAi*` op/ap helpers now key `GetByTrigger` on the acting npc's `typeId` + new `aiTriggerCategory()` helper for every target kind (TS `Npc.ts:992`); player branch's hardcoded `(0,0)` fixed, dead `objCategory`/`locCategory` removed; new `npc_ai_trigger_subject_test.go` pins the contract + npc-ai-2 wrong-subject tests updated (`-race` unavailable here, no C compiler — plain `go test ./...` green). Both rows moved to `docs/PORTING-CLOSED.md`; both branches merged to `main` (`7175ec5e`). `pathing-1` (HIGH, runtime `MoveRestrict` enum dropped `BLOCKED_NORMAL` so every npc with `moverestrict ≥ 2` was misread via the raw-byte cast in `npc.go:187`) **FIXED** on branch `fix/pathing-1`: inserted `MoveRestrictBlockedNormal` into `movement_consts.go` to restore the canonical 0..6 numbering (matches `MoveRestrict.ts` + `pkg/objtype/npctype.go`) + added the `BLOCKED_NORMAL` case (→`CollisionFlag.NPC` / `CollisionType.LINE_OF_SIGHT`) to the 3 consumer switches (`Npc.blockWalkFlag`, `{Npc,Player}.getCollisionStrategy`); the existing named cases were already TS-correct, only mis-fed. New `npc_moverestrict_test.go` pins enum↔canonical alignment + per-raw-value `blockWalkFlag`/`getCollisionStrategy` (RED→GREEN; `-race` unavailable, no C compiler — `go test ./modules/world/` green). Row moved to `docs/PORTING-CLOSED.md`. `interaction-1` (HIGH, player `processInteraction` ported only validateTarget's level gate — the changetype and `isValid(hash64)` gates were missing, so a player kept interacting with a changed-type/removed/private-now-invalid target) **FIXED** on branch `fix/interaction-1` (`b337b6a0`): added `(*Player).validateTarget` (level / changetype for Npc+Loc / polymorphic `isValid(hash64)` — Npc `!dead && !delayed`, Obj `IsValidFor(UID)` reveal+count, else `Entity.IsValid`) wired into the pre-step interact arm inside the CanAccess gate (TS `Player.ts:1186-1198,1209-1224`), removing the standalone level check. Closes its root cause **interaction-2** (`SetInteraction` now snapshots `targetSubject.typ` for Npc/Loc/Obj else -1, mirroring `(*Npc).SetInteraction` + TS `PathingEntity.ts:521-526`) and consequentially **interaction-3** (the unified validateTarget-fail path uses bundled `unsetMapFlag()` not wire-only `sendUnsetMapFlag`). New `interaction_validate_target_test.go` pins each gate + the changetype clear through `processInteraction` (RED→GREEN); 4 followOp fixtures gained `active=true` on the follow target (gate 3 requires a valid in-world player). `go test ./modules/world/` green (`-race` unavailable). Rows moved to `docs/PORTING-CLOSED.md`. `login-server-1` (HIGH, a login for an account already logged in on the **same** node with `Reconnecting=false` fell through the already-logged-in gate into the full-login tx → OK/NEW_PLAYER, admitting a second concurrent session where TS rejects) **FIXED** on branch `fix/login-server-1` (`ad501486`): restructured `PlayerLogin` step 7 to mirror TS `LoginServer.ts:271,318`'s if/else-if — within the logged-in gate, `same-node && Reconnecting` → reconnect, **else** → ALREADY_LOGGED_IN (different-node behaviour unchanged). `handler_test.go` pins the previously-admitted same-node non-reconnect case (RED→GREEN); `go test ./modules/login/` green (`-race` unavailable). Row moved to `docs/PORTING-CLOSED.md`. `rsbuf-player-1` (HIGH, `writePlayers` set `extend=1` for every tracked other with a non-empty high-def block with no byte-budget check, so a crowded view could overflow the 4997-byte PlayerInfo packet the Java client decodes) **FIXED** on branch `fix/rsbuf-player-1` (`1913914b`): ported Rust `write_players`'s per-other budget gate (rsbuf `info.rs:118-127`) — the movement leaf is always emitted but the high-def block is appended only when `hdLen>0 && fits(...)`, falling to `idle()` once the budget is exhausted, using goscape's existing live-measurement `fits()` (the same convention `writeNewPlayers` already uses); reintroduced the `BITS_RUN/WALK/EXTEND` leaf-size constants retired at NAI-30 B4 T4.6. New `TestPlayerInfo_TrackedOthers_RespectByteBudget` crowds the view with 40 tracked others × 250-byte high-def (~2× over budget) and pins the packet stays ≤ `maxPlayerInfoBytes` while still packing as many blocks as fit (RED: 10000 bytes → GREEN: ≤ 4997). `go test ./pkg/rsbuf/` green (`-race` unavailable). Row moved to `docs/PORTING-CLOSED.md`. `gap-world-reload-events-1` (HIGH, the last open HIGH — `RelayReload()` called `dispatchRebuildRequest()`, triggering a full content PackAll + a `reloadFn(true)` that clobbered every player/shared inventory on each relay; TS `World.ts:2036` handles RELAY_RELOAD as a plain `reload(false)`: config-only reload of the already-packed types, no repack, clearInvs=false) **FIXED** on branch `fix/gap-world-reload-events-1` (`6a7f95b2`): `RelayReload()` now enqueues `s.reloadFn(false)` on the tick goroutine (where Reload runs safely against tick-owned state) and logs any reload error at Error — no repack (the `::rebuild`/fsnotify pipeline owns packing; per-world repacking was both wasteful and incorrect), no inventory clobber. `reload.go`'s own doc comment already anticipated this wiring. Tests: `TestWorldStateOps_Reload_CallsReloadConfigOnly` pins `reload(false)` with no `rebuildReq` post (RED→GREEN); the friends smoke test's RelayReload arm + the prior `rebuildReq`-pinning test re-pointed to the config-only contract. `go test ./modules/world/` green (`-race` unavailable). Row moved to `docs/PORTING-CLOSED.md`. `rsbuf-npc-1` (MED, the NpcInfo sibling of `rsbuf-player-1` — `writeNpcs` set `extend=1` for every tracked NPC with a non-empty high-def block with no byte-budget check, so a crowded NPC view could overflow the 4997-byte NpcInfo packet) **FIXED** on branch `fix/rsbuf-npc-1` (`86840f96`): mirrored the landed `writePlayers` gate — reintroduced the `npcBitsRun/Walk/Extend` leaf-size constants and gated each `writeNpcs` movement arm on `hdLen>0 && fits(...)`, falling to idle once the budget is exhausted (Rust `write_npcs` info.rs:483-491); the movement leaf is always emitted, only the high-def block conditional. New `TestNpcInfo_TrackedNpcs_RespectByteBudget` crowds the view with 40 tracked NPCs × 250-byte high-def (RED: 10000 bytes → GREEN: ≤ 4997). `go test ./pkg/rsbuf/` green (`-race` unavailable). Row moved to `docs/PORTING-CLOSED.md`. `interaction-4` (MED, ClearInteraction omitted the targetSubject identity reset that TS PathingEntity.clearInteraction:550-555 performs, so a stale subject snapshot — typ + goscape's x/z/level loc/obj extension + com — survived an interaction clear) **FIXED** on branch `fix/interaction-4` (`e3146713`): ClearInteraction now resets all five targetSubject fields to -1 (mirrors TS {type:-1,com:-1} plus goscape's x/z/level extension written by handler_oploc/handler_opobj for locStillValid/objStillValid). The non-TS interacted/repathed resets were deliberately retained — removing `interacted` is coupled to interaction-6 (it is reset only on Set/ClearInteraction, never per-tick as TS does in resetPathingEntity:88, so dropping the reset here would let it stay stuck-true), and that cleanup was folded into the interaction-6 row. `TestClearInteraction_ResetsTargetSubject` pins the cleared snapshot (RED {typ:7 x:51 z:50 level:0 com:42} → GREEN all -1); `go test ./modules/world/` green (148s, `-race` unavailable, no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `world-tick-1` (MED, downgraded from HIGH — `processShutdown`'s 1024-tick deadline set a per-player `forceRemove` flag that `processLogouts` honored, but `processLogouts`' inner `if !p.CanAccess() || len(p.engineQueue) > 0 || !queueDiscardable { continue }` gate runs *before* the force branch, so a stuck player — delayed / open-modal / active-protected-script, pending engineQueue, or non-discardable queue — was skipped forever, hanging shutdown past the very deadline meant to evict it) **FIXED** on branch `fix/world-tick-1` (`9b28c5bd`): `processShutdown` now snapshots `playerLoop` and calls `removePlayerOnTick` directly on every remaining player once `duration >= 1024`, bypassing the logout drain (mirrors TS `World.ts:1207-1213`'s inline `removePlayer` loop); the now-unused `forceRemove` field (`player.go`) and its `processLogouts` branch (`tick.go`) were removed. `TestProcessShutdown_ForceRemovesStuckPlayerAfter1024` pins the stuck-player (delayed / !CanAccess) eviction and the two existing 1024/1023 boundary tests were re-pointed from the flag to the direct-removal contract (RED→GREEN; full `go test ./modules/world/` green, 148s, `-race` unavailable). Row moved to `docs/PORTING-CLOSED.md`. `interaction-6` (MED, the per-tick reset half of the interaction-4 coupling — `apRangeCalled` was reset only on Set/ClearInteraction and at the start of an AP-script exec, never per-tick like TS `resetPathingEntity` does at PathingEntity.ts:588, so a stale-true value carried from a prior tick's `p_aprange` suppressed the auto-clear (`else if interacted && !p.apRangeCalled`) on the next tick that fired via the OP path — which, unlike the AP path, never resets `apRangeCalled` — leaving the player stuck on the target an extra tick; `interacted` is a TS-parity cosmetic reset with no goscape control-flow reader) **FIXED** on branch `fix/interaction-6` (`4535a846`): `Player.ResetMasks` now resets both fields each tick, mirroring TS `PathingEntity.ts:587-588` and the NPC sibling at `npc_masks.go:277`; the non-TS `interacted`/`repathed` resets are dropped from `SetInteraction` (TS PathingEntity.ts:517-518 resets only apRange+apRangeCalled) and `ClearInteraction` (TS PathingEntity.ts:554-555 same), closing the deferred half of `interaction-4`. `repathed` is vestigial — NAI-98 retired its reader and no production code ever writes it true. The two SKIP-PINNED NAI-108-D tests (`TestPlayerInteractedDoesNotLeakAcrossIdleTick` / `TestPlayerApRangeCalledDoesNotLeakAcrossIdleTick`) are unskipped (RED→GREEN; toggle-off proof confirms the fix is load-bearing); `TestClearInteractionResetsAll` drops its now-orphaned post-Clear `interacted`/`repathed` assertions. `go test ./modules/world/` green (148.7s, `-race` unavailable, no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `interaction-7` (MED, a goscape-only `delayed && currentTick<delayedUntil` short-circuit at the top of `processInteractionPreMove` pre-empted the post-step HEAD (TS `Player.ts:1227-1239`) for a delayed player, skipping `pathToPathingTarget`'s followOp chase recompute (TS L1039-1042) and the L1237 exhaustion clear — so a delayed follower stopped re-pathing to the leader's `followX/Z` and stalled at their previous tile) **FIXED** on branch `fix/interaction-7` (`bfb7a852`): dropped the short-circuit; TS only gates the *interact* arms on `canAccess` (L1210, L1244) and the post-step HEAD runs unconditionally on `!interacted`. The existing `CanAccess()` call-site gates on the pre-step arm (`interaction.go:355`) and post-step arm (`interaction.go:417`) already handle the actually-delayed case TS-faithfully. New `TestProcessInteraction_CanAccessGate_Delayed_FollowOp_PathRecomputed` pins a delayed follower re-queueing a chase waypoint to the leader's `followX/Z` via TS L1039-1042 (RED `waypointIndex=-1` → GREEN `waypointIndex=0, wp=(3219, 3220)`); the pre-existing delayed-preservation test was renamed (`...EarlyReturnsBeforePathing`→`...PreservesInteraction`) and its comments updated (its load-bearing assertion — target preserved — still holds). `go test ./modules/world/` green (150s, `-race` unavailable, no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `player-script-1` (MED, downgraded from HIGH — `StopAction` (`player_script.go:1261`) called `ClearInteraction()` + `ClearPendingAction()` but omitted the `unsetMapFlag()` call entirely; TS `Player.stopAction` (Player.ts:944-947) is `clearPendingAction() + unsetMapFlag()`, and TS `unsetMapFlag` (Player.ts:2169-2172) is `clearWaypoints() + write(UnsetMapFlag)` — goscape therefore preserved the walk queue AND never sent `OpUnsetMapFlag`, so a content script's `p_stopaction` left the player walking past the stop point and the client's map-click indicator stayed lit; the StopAction doc comment ("Walk queue is preserved") was the symptom-as-spec lie called out by the audit row) **FIXED** on branch `fix/player-script-1` (`4484fbe0`): appended the existing `(*Player).unsetMapFlag()` helper (`interaction.go:61` — the TS-bundled `waypointIndex=-1 + sendUnsetMapFlag` bundle) to `StopAction` and rewrote the doc comment; the leading `ClearInteraction` call stays — it carries the targetSubject reset that goscape's partial `ClearPendingAction` (`player-script-7`, still open) does not. Coupled fixture wiring: TS-faithful `StopAction` now emits a wire packet, so `newApTriggerNpcFixture` (which exercises `script.handleP_OpNpc → StopAction` via `fireOpTriggerNpc`) wires an ISAAC encryptor; tests that assert specific encrypted bytes overwrite with their own key, tests that don't read bytes are unaffected — the other op-fire fixtures (Obj/Player) did not surface failures under the suite. New `TestStopAction_ClearsWaypointAndEmitsUnsetMapFlag` (sibling-shaped to `TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket`) pins both arms — `waypointIndex=5 → -1` AND the encrypted `OpUnsetMapFlag` byte on the wire (RED before fix, GREEN after); `go test ./modules/world/` green (150.5s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `npc-hunt-1/2/3` (MED cluster — TS `Zone.getAll{Npcs,Objs,Locs}Safe` (Zone.ts:399-405, 411-417, 459-465) yields only entities passing `isValid()`; goscape's hunt entry points were missing the corresponding gates: huntNpcs didn't exclude delayed candidate NPCs (TS `Npc.isValid` override at Npc.ts:370-375 never propagated through goscape's `pkg/zone.NpcsSafe` because `(*Npc).IsValid` returned only `!n.dead`), huntObjs omitted the `count≥1 && isActive` gate (TS `Obj.isValid` at Obj.ts:52-62), huntLocs omitted the `isActive` gate (TS `Entity.isValid` base at Entity.ts:32-34)) **FIXED** on branch `fix/npc-hunt-cluster` (`e3a48a7e`): npc-hunt-1 — `(*Npc).IsValid` now returns `!n.dead && !n.delayed`, mirroring TS `Npc.isValid` exactly; the delayed gate now propagates through `NpcsSafe` to huntNpcs and `NpcIterator` transparently. Compensating change at `processNpcHuntPlayers` (`npc_hunt.go:82`) which TS `World.ts:579-580` explicitly allows delayed NPCs ("Hunts will process even if the npc is delayed during this portion") — that site now gates on the base `n.dead` directly rather than the now-stricter IsValid. The previous "DEVIATION: pkg/entity knows nothing about scheduling state" doc comment was a documented mistake: `(*Npc).IsValid` is on `modules/world.Npc`, which already owns the `delayed` field, so the layering excuse never applied. Two stale "Npc.IsValid is only !dead" doc-comments (gate 3 of `(*Player).validateTarget` at `interaction.go:208-214`, gate 4 of `(*Npc).validateTarget` at `npc_interaction.go:644-647`) updated to reflect the new contract; the spelled-out `!t.dead && !t.delayed` bodies kept as redundant-but-safe defense in depth. npc-hunt-2 — added inline `IsActive && Count>=1` filter at huntObjs (`npc_hunt_entities.go:108-110`), mirroring TS `Obj.isValid` (Obj.ts:52-62 → Entity.ts:32-34); the reveal/hash64 arm not exercised — huntObjs passes no hash64; goscape's `Zone.Objs` is a raw slice (no `ObjsSafe` iterator helper), so the gate is inline. npc-hunt-3 — added inline `IsActive` filter at huntLocs (`npc_hunt_entities.go:182-185`), mirroring TS `Entity.isValid` base predicate. Tests: three RED→GREEN pinning tests added — `TestHuntNpcsExcludesDelayedNpc`, `TestHuntObjsExcludesInactiveOrDepletedObj` (inactive + `count=0` both excluded; healthy seed returned), `TestHuntLocsExcludesInactiveLoc`. Four raw-append test helpers (`addObjToZone`/`addObjToZoneAt`/`addLocToZoneAt` + one inline raw-seed in `TestHuntObjsMissingTypeConfigSkipsOnCategoryFilter`) updated to set `IsActive=true` post-append, mirroring `pkg/zone.AddStaticObj`/`AddStaticLoc` — without this, pre-existing huntObjs/huntLocs LoS-pass tests would have spuriously asserted zero results under the new gate. `go test ./modules/world/` green (149.6s, `-race` unavailable — no C compiler). Three rows moved to `docs/PORTING-CLOSED.md`. `pathing-2` (MED — `(*Player).resolveMovement` (`movement.go:41-118`) lacked the TS `PathingEntity.processMovement` (`PathingEntity.ts:134-137`) early-return on `moveSpeed===INSTANT/STATIONARY`; the existing bridge above (NAI-135) preserves Instant when set by `P_TELEJUMP` (`player_script.go:600`) / `RebuildNormal` (`login_resync.go:98`), but the next line just fell through to `validateAndAdvanceStep`, so any queued waypoint from a prior `pathToMoveClick` got stepped on the teleport tick — producing an animated walk-step inside the same tick as the jump) **FIXED** on branch `fix/pathing-2` (`455e008a`): added the early-return (`walkDir=-1`, `runDir=-1`, `tempRun=0`, return) after the lastTickX/Z/Level capture and before the no-waypoints check, mirroring TS `PathingEntity.ts:134-137`. Gate placement is deliberate — capturing lastTickX/Z FIRST (= post-teleport position) makes the next tick's `validateDistanceWalked` baseline correct; if the gate ran before the capture, the pre-teleport coords from the prior tick would carry forward and re-flag `jump=true` on the following tick. The `tempRun=0` reset mirrors TS `Player.updateMovement`'s `!super.processMovement() → tempRun=0` branch (`Player.ts:670-673`). STATIONARY is structurally-parity only for Player (the existing bridge overwrites it to WALK/RUN unless `moveSpeed` entered as INSTANT) but kept to match TS L135 verbatim. Tests: new `TestResolveMovement_InstantMoveSpeed_SuppressesStep` (sibling-shaped to `TestResolveMovementNoPathClearsDirections`) pins `waypointIndex`/position unchanged + `walkDir`/`runDir`/`tempRun`/`stepsTaken` reset (RED→GREEN); `TestResolveMovement_TempRunPreservedDuringSteps` re-pointed to set `moveSpeed=Walk` explicitly so the bridge fires (it relied on the bug — `newTestPlayer`'s default `moveSpeed=Instant` from `player.go:558` previously survived the bridge skip AND step). `go test ./modules/world/` green (150s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `player-net-2` (MED — `onReconnect` (`login_resync.go:57-60`) called `CloseModal(false)`, so any pre-disconnect `QueueWeak` entries survived the resync; TS `Player.onReconnect` (Player.ts:543) calls `this.closeModal()` with no args and the default `clearWeakQueue=true` (Player.ts:741) drops the weak queue. The existing doc-comment ("preserves main modal, drops chat/side") was a comment-lie — `false`/`true` only gates the weakQueue arm; per-slot IF_CLOSE dispatch fires identically in both arms once `modalState != None`) **FIXED** on branch `fix/player-net-2` (`40cf18d7`): flipped the call to `p.CloseModal(true)` and rewrote the doc-comment to cite TS Player.ts:543/741. New `TestOnReconnect_ClearsWeakQueue` seeds one `QueueStrong` + one `QueueWeak` entry, runs `onReconnect`, asserts queue len 1 with the strong entry preserved (RED `got 2, want 1` → GREEN). Existing `TestOnReconnect_EmitsResyncSequence` byte-stream unchanged — `newTestPlayer` has `modalState=None`, so `CloseModal(true)` clears the (empty) weak queue then early-returns at the `modalStateNone` check without dispatching any wire packet (the test's `(d) closeModal` comment updated for accuracy). Toggle-off proof confirms the fix is load-bearing. `go test ./modules/world/` green (150s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `player-net-3` (MED — `onReconnect` (`login_resync.go:92-94`, block (k)) ORed only `p.entitymask` into `p.masks`, omitting the second OR that TS Player.onReconnect performs at Player.ts:555 (`this.masks |= PlayerInfoProt.APPEARANCE; // resync appearance`); without the appearance dirty bit set, the next mask-block emit after resync skipped the appearance payload, so any appearance change that occurred just before disconnect or between login and reconnect did not propagate to newly-visible observers — the face_entity half (Player.ts:554) was already wired correctly via `p.masks |= p.entitymask`, the appearance half was the gap) **FIXED** on branch `fix/player-net-3` (`9baa0f47`): appended `p.masks |= rsbuf.MaskAppearance` immediately after the existing entitymask OR in block (k), mirroring TS L555 verbatim; `rsbuf.MaskAppearance` is 0x1 (`pkg/rsbuf/visibility.go:16`), independent of `entitymask`'s default `MaskFaceEntity=0x4` (`player.go:614`); doc-comment for block (k) rewritten to cite Player.ts:554-555 (both halves) and document why the appearance OR is load-bearing. New `TestOnReconnect_OrsAppearanceMaskIntoMasks` (sibling to `TestOnReconnect_OrsEntityMaskIntoMasks`) presets `p.entitymask = rsbuf.MaskFaceEntity` so the assertion on `p.masks & rsbuf.MaskAppearance != 0` cannot be coincidentally satisfied by the entitymask OR (RED `MaskAppearance bit (0x1) not set after onReconnect; got 0x4` → GREEN). `go test ./modules/world/` green (149.8s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `player-net-1` (MED — `processIn` reset `opcalled` AFTER the `c.state != ClientStateGame` early-return and never reset `userPath`; TS `NetworkPlayer.decodeIn` (NetworkPlayer.ts:55-57) clears both at the very top of the method BEFORE the `isClientConnected` early-return, so a stale path from a prior tick's MoveClick handler cannot leak into the next tick's `processPostDecode` (which gates `moveClickRequest=true` on `len(userPath)>0 || opcalled` at TS Player.ts:613 / `player_post_decode.go:23`)) **FIXED** on branch `fix/med-bundle-1` (`d5cdffb0`): added `p.userPath = nil` + moved `p.opcalled = false` to before the `c.state` check; `decodedThisTick` (NAI-146 T1) stays where it is — it semantically pairs with the readAny loop below, not with TS L55-57. `TestProcessIn_ClearsUserPathAtDecodeStart` pins the steady-state reset (RED `userPath: got [57005 48879]` → GREEN empty) and `TestProcessIn_ClearsUserPathBeforeDisconnectCheck` pins the placement (RED `got [42]` on a disconnected-state player → GREEN) so a future fix that puts the reset inside the connected branch reproduces RED. `go test ./modules/world/` green (149.9s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `npc-ai-3` (MED — TS `Npc.ts:69` declares `nextPatrolTick: number = -1` as the field default; the patrol-tele gate at TS `Npc.ts:728` / `npc_interaction.go:123` is structured as `(x != dest.X || z != dest.Z) && nextPatrolTick > -1 && currentTick >= nextPatrolTick`, so a fresh patrol NPC's first tick has the gate dormant and the NPC walks to its first waypoint organically; goscape's `NewNpc` omitted the field from the struct literal, leaving Go's zero value (0), and at `currentTick=0` both halves of the gate trivially held, force-teleporting any patrol NPC to its first waypoint on the first tick after spawn) **FIXED** on branch `fix/med-bundle-1` (`38ff8de0`): added `nextPatrolTick: -1` to the `NewNpc` struct literal. `TestNewNpc_InitsNextPatrolTickToMinusOne` pins the contract on the constructor (RED `got 0, want -1` → GREEN) and `TestPatrolMode_FreshNpcDoesNotForceTeleportOnFirstTick` pins the behavioural consequence (RED NPC at (3210, 3310) → GREEN at start (3200, 3300)). The existing `TestPatrolMode_PreservesDestLevel` explicitly sets `nextPatrolTick=0` to force the patrol-tele branch and is unaffected by the default change. `go test ./modules/world/` green (149.8s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `player-core-2` (MED — TS `Player.updateEnergy` (Player.ts:690-693) keeps `weightKg` as float (`this.runweight / 1000`), clamps the float to [0, 64], then truncates only the final `loss` expression via `| 0`; goscape's `weightKg := p.runweight / 1000` int-divided BEFORE the `67*weightKg/64` math, dropping the kg fraction and systematically under-draining for any partial-kg encumbrance — e.g. runweight=32500 (32.5 kg) gave loss=100 vs TS's 101) **FIXED** on branch `fix/med-bundle-1` (`98fec06c`): ported the drain expression to `float64(p.runweight) / 1000` with the final `int(67 + 67*clampWeight/64)` truncate, matching TS L693 placement. `TestUpdateEnergy_DrainFractionalKgUsesFloatMath` pins runweight=32500 → 9899 (RED 9900 → GREEN 9899); the five existing drain tests use integer-kg values (0, 64000, 200000, -1000) where the bug silently agreed with TS, so they remain green under the float port (negative clamps to 0 float, ≥64kg clamps to 64.0 float — both yield identical loss). Companion gofmt-realignment of the `npc.go` struct literal (carried in the same commit). `go test ./modules/world/` green (150.3s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `npc-ai-5 / pathing-5 / interaction-5` cluster (MED — TS `PathingEntity.inApproachDistance` at PathingEntity.ts:392-406 gates the footprint-overlap bail on `target instanceof PathingEntity`, so the "you are not within ap distance ... if you are underneath it" bail fires ONLY for Player/Npc targets; Loc and Obj targets skip it. A player standing on a 3x3 Loc footprint (banker counter, shop stall) or sharing a tile with an Obj is still in approach distance to fire its AP script. goscape applied the bail unconditionally at TWO call sites: `(*Npc).inApproachDistance` at `npc_interaction.go:900` — every NPC AP trigger → npc-ai-5 — and the free `inApproachDistance` used by the player's `tryInteract` pre-check at `interaction.go:869` → pathing-5 + interaction-5, suppressing valid AP fires whenever the source overlapped a non-pathing target's footprint) **FIXED** on branch `fix/npc-ai-5-cluster` (`db990a22`): `(*Npc).inApproachDistance` wraps the `coordgrid.Intersects` bail in a `switch target.(type) { case *Player, *Npc: ... }` (Loc/Obj fall through to the distance + LoS gates unchanged); free `inApproachDistance` gains an `isPathingTarget bool` last arg and gates the bail on it; `(*Player).tryInteract` (`interaction.go:540-558`) hoists the existing `isPathing` switch ABOVE the `inApproachDistance` call (was below) and threads the bool through. The bail still fires for Player/Npc targets, so existing pin tests (`TestInApproachDistanceSameTile`, `TestInApproachDistance_EdgeAware_MultiTileTarget` case 4 "source under 3x3 footprint", `TestNpcInApproachDistanceSizeAwareDistance` "player under size-3 footprint", `TestNpcInApproachDistance` "same tile — false") all stay green. New tests: `TestInApproachDistance_NonPathingTarget_SkipsFootprintBail` (Loc-overlap + same-tile Obj + out-of-range Loc control for the free function) and `TestNpcInApproachDistance_NonPathingTarget_SkipsFootprintBail` (size-3 NPC vs overlapping 3x3 Loc + 1x1 Obj cases + out-of-range Loc control on the Npc method) — both RED before fix with `got false, want true` for the overlap cases, GREEN after; toggle-off proof on both halves. All 14 existing free-function test callers updated to pass `true` as the new last arg. `go test ./modules/world/` green (149.6s, `-race` unavailable — no C compiler). Row moved to `docs/PORTING-CLOSED.md`. `h-loc` cluster (MED — TS `Zone.getLoc` → `getLocsSafe` (Zone.ts:259-266, 471-477) and `getAllLocsSafe(reverse)` (Zone.ts:459-465) yield only `loc.isValid()`-passing locs; `Entity.isValid()` === `isActive` (Entity.ts:32-34), and `Loc` does not override that predicate. goscape ported these surfaces without the `isActive` gate at three places, so stale/removed locs (still linked in `zn.Locs` when lifecycle==RESPAWN) re-fired `OpLoc`/`OpLocT`/`OpLocU` clicks (3 callers of `Server.GetLoc` in `handler_oploc.go`), leaked through `LOC_FIND` (via `serverLocOps.GetLoc`), and appeared in `LocIterator`/`LOC_FINDALLZONE` (also yielded forward instead of TS's reversed order from `getAllLocsSafe(true)`)) **FIXED** on branch `fix/h-loc-cluster` (`cd0c6468`): `Server.GetLoc` (`loc_lookup.go`) now gates on `l.IsActive` — closes `entity-base-3` / `h-loc-1` / `zone-sub-5` (LOC_FIND inherits the filter via `serverLocOps.GetLoc`) and makes the existing `serverLocOps.GetLoc`/`LocOps.GetLoc` doc-comments truthful (closes `h-loc-2`). New `LocOps.AllLocsSafe(level, x, z, reverse bool)` method on the `pkg/script` interface, implemented in `serverLocOps` (mirrors TS `getAllLocsSafe(reverse)` — filters `IsActive`, reverses if requested); `LocIterator` (`pkg/script/loc_iterator.go`) routes through it with `reverse=true` per TS `ScriptIterators.ts:378` — closes `h-loc-4`. `AllLocsInZone` stays unfiltered (TS `MAP_LOCADDUNSAFE` at `ServerOps.ts:212-252` uses `getAllLocsUnsafe()` and does its own per-loc `{isActive, type.active, WALL-only-isActive}` checks; filtering `AllLocsInZone` would break the WALL-only branch). Test pin: `TestServerGetLocReturnsLocWhenPresent` / `*FiltersByTypeID` had silently relied on the bug (raw-append leaves `IsActive=false`, GetLoc with no filter still returned them), so the existing seeds now set `IsActive=true` post-append to preserve named intent; new `TestServerGetLoc_FiltersInactiveLoc` pins the negative side — closes `h-loc-6`. Tests: `TestServerGetLoc_FiltersInactiveLoc` + `TestServerLocOps_AllLocsSafe_FiltersInactiveAndReverses` (two subtests: reverse=true and reverse=false, each pins filter + order) + `TestLocIteratorRoutesThroughAllLocsSafeWithReverseTrue` (precise routing-contract pin — fixture populates BOTH mock methods so a regression to `AllLocsInZone` surfaces as "AllLocsSafe call count: got 0, want 1" rather than an upstream empty-iterator path). Three toggle-off proofs verified independently (revert the `IsActive` filter, the `AllLocsSafe` filter, or the LocIterator routing — each goes RED only on its matching test). Coupled test-helper edits: 7 raw `zn.Locs = append` seed sites set `IsActive=true` post-append so component-gate, OpLoc-success-path, viewport-boundary, missing-LocType (3 OpLoc variants), and loctype_nil debug-gate scenarios continue to reach their intended gates instead of bailing at the new `getloc_nil` short-circuit — `seedLocAt` (handler_component_gate_test.go), 5 sites in handler_oploc_test.go, 1 in interaction_debug_test.go. Also added a no-op `AllLocsSafe` to two existing fakes (`fakeLocOps` in `pkg/script/loc_ops_test.go`, `mapLocAddUnsafeOps` in `pkg/script/handlers_map_test.go`) so they still satisfy the (now-larger) `LocOps` interface. `h-loc-3` (LOC_ADD `CoordValid` range check) is a different root cause and remains open; the audit-noted `entity.Loc.IsValid()` always-true behaviour is a deliberate, documented layering divergence (pkg/entity cannot depend on modules/world; the in-world-right-now check lives on `IsActive`, gated by world-module helpers) and not in any cluster row. `go test ./modules/world/` green (150.1s) + `./pkg/script/` green (0.07s) (`-race` unavailable — no C compiler). Six rows moved to `docs/PORTING-CLOSED.md`. `med-bundle-2` (4 MED picks across disjoint files, single ff-merge per the med-bundle-1 template) **FIXED** on branch `fix/med-bundle-2`: `player-net-5` (`503e093f`) — TS `NetworkPlayer.updateStats` (NetworkPlayer.ts:330) gates the per-tick run-energy emit on `Math.floor(re)/100 !== Math.floor(lre)/100`; for integer re/lre that's `re !== lre`, so TS emits on ANY internal change. goscape's int-division gate `re/100 != lre/100` suppressed same-wire-byte changes (e.g. 10000→10050: both bytes 100 → no packet). Fix: gate on `p.runenergy != p.lastRunEnergy` (the wire encoder is unchanged; only emission frequency now matches TS). The pre-existing `TestUpdateStatsRunEnergyCoarseGrain` silently pinned the bug behavior; replaced with `TestUpdateStats_RunEnergy_EmitsOnAnyChange` covering all 4 cases (first-tick / same-byte-bump / cross-byte-bump / no-change). `npc-core-1` (`5df2d406`) — TS `Npc.resetEntity(true)` at Npc.ts:307 calls `resetDefaults()` (Npc.ts:411-425) which clears target/targetOp/apRange/apRangeCalled/targetSubject/faceEntity/timerInterval and sets `masks |= entitymask`. goscape's `resetEntityForRespawn` omitted that call entirely; a respawned NPC kept stale interaction state from its previous life. Fix: invoke `n.resetDefaults()` after the varsString fill, then re-apply the apRange/apRangeCalled/targetSubject/timerInterval resets inline — goscape's `resetDefaults()` is the NAI-11-stripped subset (target/targetOp/faceEntity/masks only), so the stripped fields are handled at the respawn site. `TestResetEntityForRespawn_AppliesResetDefaultsTSFidelity` pins all 8 reset axes. `npc-ai-4` (`b9a31844`) — TS `wanderMode` (Npc.ts:697-715) gates ONLY on `moverestrict !== NOMOVE && Math.random() < 0.125`, then calls `randomWalk(wanderrange)` UNCONDITIONALLY. goscape's pre-fix predicate added `&& n.typ.WanderRange > 0`, suppressing the wander roll entirely for 0-range NPCs (a drifted 0-range NPC could only return home via the 500-tick teleport). Fix: delete the `&& WanderRange > 0` clause; the inner block is already TS-faithful when WanderRange=0 (rand.IntN(1)=0 → dx=dz=0 → queues (startX, startZ) only when off-spawn, matching TS randomWalk(0)). `TestWanderMode_ZeroRange_DisplacedNpc_QueuesSpawnReturn` pins the 1/8 cadence empirically (~100/800 hits post-fix, 0/800 pre-fix). `h-loc-3` (`8dcb08ce`) — TS `LOC_ADD` (LocOps.ts:23) calls `check(coord, CoordValid)` FIRST, before the type/angle/shape/duration validators; CoordValid (ScriptValidators.ts:109) rejects packed coords outside [0, 2^31-1]. goscape's pre-fix `handleLocAdd` skipped CoordValid; with a negative coord, `UnpackCoord` returned garbage and the handler fell through to `LocOps.AddLoc` on a bogus position. Fix: invoke `checkCoord` (handlers_npc.go:55 — the existing TS CoordValid port, reused by every other COORD-typed handler) as the FIRST validator; reuse its (level, x, z) return for the downstream LocsAtCoord/AddLoc calls. `TestLocAddRejectsBadCoord` pins coord=-1 returning `LOC_ADD: coord out of range (-1)` and short-circuiting before any LocOps call. All 4 with toggle-off proofs; `go test ./modules/world/` + `./pkg/script/` green; build/vet/gofmt clean. Four rows moved to `docs/PORTING-CLOSED.md`. `med-bundle-3` (4 MED picks across 2 fresh files: `pkg/script/handlers_player.go` + `pkg/script/handlers_inv.go`; single ff-merge per the med-bundle-1/-2 template) **FIXED** on branch `fix/med-bundle-3`: `h-player-2` (`dbad8450`) — TS `PlayerOps.ts:769,771` wraps DAMAGE amount and uid in `check(state.popInt(), NumberNotNull)`; goscape's `handleDamage` validated only hitType, so `amount=-1` (script null sentinel) reached `player.ApplyDamage(-1, ...)` where TS would have aborted at the popInt site, and `uid=-1` produced a silent lookup miss. Fix: insert `checkNotNull` on amount (after the first PopInt) and on uid (after the third PopInt) in the same order TS pops + validates them; doc-comment updated to enumerate the three validators per slot. `TestDamage_NullAmountRejected` and `TestDamage_NullUIDRejected` pin both abort paths (RED→GREEN); `ApplyDamage` verified not to fire on either reject. `h-player-3` (`f916e5de`) — TS `PlayerOps.ts:240` (FACESQUARE), 440 (P_TELEJUMP), 448 (P_TELEPORT) all wrap `state.popInt()` in `check(..., CoordValid)`; `CoordValid` (ScriptValidators.ts:109) rejects packed coords outside [0, 2147483647] including `-1` (the universal null sentinel for COORD-typed script ints). goscape's three handlers skipped the check and called `unpackCoord` directly on the raw popped int, so a negative packed value was silently bit-masked into a garbage tile and forwarded to `FaceSquare`/`Teleport`/`TeleJump`; sibling `P_WALK` already calls `checkCoord` (internal inconsistency, now closed). Fix: route each handler's popped coord through `checkCoord` (the same helper LOC_ADD now uses post `h-loc-3`) and short-circuit on error; for P_TELEPORT the existing argCoord+(level,x,z) names are preserved so the NodeDebug log block downstream is untouched. Three RED→GREEN tests: `TestPTeleportRejectsBadCoord`, `TestPTeleJumpRejectsBadCoord`, `TestFaceSquareRejectsBadCoord` each push coord=-1, assert the `"<OP>: coord out of range"` error substring, and verify the corresponding `mockPlayer` call counter stays at 0. `h-inv-1` (`a4180f1d`) — TS `InvOps.ts:592-596` calls `player.invAdd(toInvType.id, ..., completed)` with NO 4th arg, so `assureFullInsertion` defaults to true (Player.ts:1496) — destination Add is all-or-nothing. goscape's `handleInvMoveItemUncert` passed `AddOpts{Stackable: ...}` without setting `AssureFullInsertion`, leaving the zero-value false, which produced a silent partial-fill divergence: when `toInv` has insufficient room (`count > free` for non-stackables, or stack overflow for stackables), TS rolls back the Add entirely while goscape added as much as fit and dropped the overflow. Because the source items are already Removed by the time the Add runs, the divergence is observable as: post-fix the destination is unchanged on overflow (TS-faithful); pre-fix it was partial-filled and the overflow disappeared. Fix: set `AssureFullInsertion: true` on the destination `AddOpts` to match the TS default. `TestInvMoveItemUncert_NearFullDest_AssuresFullInsertion` is a RED→GREEN regression pin (destination main inv cap=28 pre-filled with 26 non-stackable swords → free=2; moving 5 swords from bank with count(5)>free(2); post-fix asserts main retains 26 swords; pre-fix observed 28 from partial-fill). The cert-direction sibling `INV_MOVEITEM_CERT` is out-of-scope (separate handler, separate audit pass). `h-inv-2` (`0c780135`) — TS `InvOps.ts:634-640` chains `check(inv, InvTypeValid)` THEN `check(category, CategoryTypeValid)`. goscape's `handleInvTotalCat` ran only the inv check and walked the slot loop with `category=-1` treated as a literal Category value — no obj matched, so the handler silently pushed `total=0` where TS would have aborted at the popInt site. Fix: call `checkCategoryType` (`handlers_npc.go:159` — the existing partial-validator port, S7f-D3, which rejects `-1` only; the count-bound check is gated on the missing CategoryType config loader) after `checkInvType`, mirroring TS's two-stage validator chain. `TestInvTotalCat_RejectsNullCategory` is a RED→GREEN regression pin (`category=-1` must produce `"INV_TOTALCAT: category null(-1)"` error instead of silent `total=0`); existing `TestInvTotalCat` happy-path remains green. All 4 with toggle-off proofs; `go test ./modules/world/` + `./pkg/script/` green (150.1s + 0.07s, `-race` unavailable — no C compiler); build/vet/gofmt clean. Four rows moved to `docs/PORTING-CLOSED.md`. `med-bundle-4` (4 MED picks across disjoint files — `pkg/script/handlers_config.go` (paramLookup), `pkg/script/handlers_inv.go` + `pkg/script/handlers_obj.go` (performInvAdd signature thread + OBJ_TAKEITEM caller + NAI-153-D3 doc rewrite), `modules/world/player_script.go` (AddXP), `pkg/zone/zone.go` (AddLoc reorder + Revert); single ff-merge per the med-bundle-1/-2/-3 template) **FIXED** on branch `fix/med-bundle-4`: `h-config-4 / h-obj-3` (`08fa3fd6`) — TS `ParamHelper.getStringParam` (ParamHelper.ts:10-16) returns `defaultValue ?? 'null'`; when a ParamType's `defaultString` is unset (TS field default `null`, ParamType.ts:63), every TS caller (OC_PARAM, OBJ_PARAM, LC_PARAM, NC_PARAM, NPC_PARAM, STRUCT_PARAM) sees the literal string "null". goscape stores DefaultString as a Go `string` (zero-value ""), so an unset defaultString surfaced as "" — the string-fallthrough in paramLookup unconditionally pushed pt.DefaultString → a script reading a missing-string param off such a ParamType saw "" where TS would have shown "null". Fix: in paramLookup's string-default branch, push "null" when pt.DefaultString == "" and the configured value otherwise; the goscape loader only writes DefaultString when opcode 5 fires, so "" reliably means "unset" in practice. `TestOcParamMissingKeyStringFallback_NullLiteral` pins OC_PARAM(995, 2): Obj 995 (Coins) has no param 2; ParamType 2 has DefaultString="" (RED `got "", want "null"` → GREEN); sibling `TestOcParamMissingKeyStringFallback` (ParamType 3 with DefaultString="fallback") still passes — non-empty defaults are preserved verbatim. `h-obj-2` (`4b45a416`) — TS OBJ_TAKEITEM (ObjOps.ts:147) calls `Player.invAdd(invType.id, obj.type, obj.count)` WITHOUT the 4th arg, so the bare entity method's `assureFullInsertion` default of `true` (Player.ts:1496) governs — the destination Add is all-or-nothing. TS INV_ADD (InvOps.ts:73) is the foil, passing `false` explicitly. goscape's shared performInvAdd hard-coded AssureFullInsertion=false for both opcodes, producing a partial-fill divergence on tight OBJ_TAKEITEM destinations. The original NAI-153-D3 doc-block acknowledged the gate divergence (no-op for realistic call shapes) but understated the assureFullInsertion divergence — the "comment-lie" called out by the fresh audit. Fix: thread `assureFull bool` through performInvAdd; OBJ_TAKEITEM passes `true`, INV_ADD passes `false`; shared helper's Inventory.Add now carries the caller's bit. Rewrote the NAI-153-D3 doc-block to spell out the actual divergence shape; updated performInvAdd's docstring to match. `TestHandleObjTakeItem_NearFullInv_AssuresFullInsertion`: inv 93 cap 28, slots 0..25 pre-filled with 26 non-stackable swords (free=2), active obj is 5 swords. count(5) > free(2) triggers the gate. Post-fix inv slots 26+27 stay nil (RED→GREEN; pre-fix slot 26 held {Id:558 Count:1}); RemoveObj from world still fires once. Existing TAKEITEM happy-path / lifecycle / invalid-obj / depleted-obj / bad-invType tests stay green. `TestPerformInvAdd_DirectCall` re-pointed to pass `false` explicitly (moral INV_ADD shape, preserves its named intent). `player-core-3` (`28e9dd90`) — TS Player.addXp (Player.ts:1742-1744) opens with `if (xp < 0) throw new Error(...)` — a hard pre-mutation throw that aborts the calling script via the runner-level catch. goscape's (*Player).AddXP omitted the guard and fell through to the `next := min(stats+xp, MaxXP)` math, which silently REDUCED stored XP (then clamped to ≥0) on any negative input. Fix: early-return on `xp < 0` after the existing statBounds(id) gate and before the multi/min/clamp math. The entity-layer guard plugs the silent-reduction surface; the full TS-faithful "script error on negative" path lives at handleStatAdvance (pkg/script/handlers_player.go:582) where TS does NumberNotNull + throws via addXp — that script-error surface is a deferred deviation called out in the new doc-comment (NumberNotNull already rejects -1; the remaining gap is xp ∈ {-2, -3, ...} reaching AddXP and silently no-op'ing instead of script-erroring). `TestAddXP_NegativeInputIsNoop` (renamed from `TestAddXPNegativeClampsAtZero` which pinned the BUG asserting stats=0): seed stats=50/baseLevels=1/levels=1, AddXP(stat, -100, false). Pre-fix RED: stats=0 (silent reduce). Post-fix GREEN: all three fields untouched. Existing `TestAddXPOOBIsNoop` / level-up / changestat / advancestat / allowMulti / NodeXPRate tests stay green — non-negative-input shapes are unaffected. `zone-sub-1` (`a5b46400`) — TS Zone.addLoc (Zone.ts:219-228) sequences: pack coord → (Despawn-only) addTail+locsCount++ → loc.revert() → loc.isActive=true → queueEvent. TS Loc.revert (Loc.ts:50-52) restores `currentInfo = baseInfo`, so the LocAddChange built into the queued event reads the BASE type/shape/angle (the Loc.ts getters surface currentInfo). goscape's Zone.AddLoc skipped Revert() entirely AND captured the LocAddChange bytes BEFORE any state mutation — a Loc whose CurrentInfo had been rewritten by a prior Change() retained that state through AddLoc and emitted the CHANGED shape/angle/type, and ended AddLoc still IsChanged(). Fix: reorder to mirror TS — Despawn-append → loc.Revert() → IsActive=true → encode LocAddChange (now reading the reverted CurrentInfo) → queueEvent. (*entity.Loc).Revert() already exists at pkg/entity/loc.go:72 with the TS-equivalent body; this fix just calls it. `TestAddLocRevertsCurrentInfoBeforeEmittingEvent`: NewLoc with BaseInfo (type=100, shape=5, angle=2), Change(101, 6, 1) to mutate CurrentInfo, AddLoc. Pre-fix RED on all four assertions — (1) CurrentInfo stayed 0x298065 (changed) want 0x314064 (base); (2) IsChanged()=true want false; (3) emitted shapeAngle byte=0x19 ((6<<2)|1) want 0x16 ((5<<2)|2); (4) emitted locID=101 want 100. All four GREEN post-fix. Sibling AddLoc/ChangeLoc/RemoveLoc tests stay green — they use unmutated CurrentInfo == BaseInfo locs, so explicit Revert() is a no-op for them. All 4 with clean RED→GREEN proofs; `go test ./pkg/script/ ./pkg/zone/ ./modules/world/` green (0.06s + 0.005s + 150.1s, `-race` unavailable — no C compiler); build/vet/gofmt clean. Four rows moved to `docs/PORTING-CLOSED.md`. `med-bundle-5` (4 MED picks across 3 disjoint code files — `modules/world/obj_lookup.go` (zone-sub-4 — audit-row path `obj_lookup.go:46-54` is at `modules/world/`, not `pkg/zone/`), `pkg/objtype/objtype.go` (cfg-onl-2), `modules/world/player_script.go` (player-core-1/pathing-4 + player-script-7 at far-apart hunks L654 vs L1274 + StopAction at L1294); single ff-merge per the med-bundle-1/-2/-3/-4 template) **FIXED** on branch `fix/med-bundle-5`: `zone-sub-4` (`d7f1a28b`) — TS `Zone.getObjOfReceiver` (Zone.ts:362-369) iterates `getObjsSafe` (Zone.ts:423-429) which gates each yielded obj on `obj.isValid()` — the hash-less form reduces to `count >= 1 && Entity.isActive` (Obj.ts:52-62). goscape's pre-fix `(*Server).getObjOfReceiver` loop matched on (x,z,type,receiver) alone, so a depleted (count<1) or removed-but-still-linked (!isActive) obj with a matching ReceiverID was returned to `World.AddObj`'s merge-decision path (world_zone.go:146), silently merging a fresh drop into a stale pile. Sibling `(*Server).GetObj` (obj_lookup.go:18) already carries the same isValid filter from the h-loc cluster work; this commit closes the gap on the exact-receiver variant. New `TestGetObjOfReceiver_SkipsInvalidObjs` pins both halves (depleted + inactive) with explicit assertion messages and a healthy-private-merge-target control to prove the filter is not over-broad (RED→GREEN; toggle-off reproduces RED on both sub-assertions). `cfg-onl-2` (`0e8d5426`) — TS `ObjType.ts:69-73` (F2P param-autodisable fixup) uses `ParamType.get(key)?.autodisable` — the optional-chain silently no-ops when the ParamType lookup misses, leaving the param in place. goscape's pre-fix `applyPostDecodeFixups` (objtype.go:112) did a raw `ptc.Configs[k]` slice index. Two panic surfaces: (a) out-of-range key — when a config carries a `Params` key `>= len(ptc.Configs)` the slice index panics ("index out of range"); (b) nil slot — a sparse cache leaves `ptc.Configs[k] = nil` and the chained `.AutoDisable` read nil-derefs. TS path is benign on both because optional-chain short-circuits to undefined (falsy). Fix mirrors the chain: bounds-check the index, nil-check the slot, then read AutoDisable. Three new tests: `TestApplyPostDecodeFixupsF2P_OutOfRangeParam_NoPanic` (pre-fix RED panic `"index out of range [99] with length 2"`, post-fix completes and leaves param in place); `TestApplyPostDecodeFixupsF2P_NilParamTypeSlot_NoPanic` (in-range nil slot); `TestApplyPostDecodeFixupsF2P_AutoDisableTrue_DeletesParam` control test proving the AutoDisable=true delete path still fires so the new guards are not over-broad. `player-core-1 / pathing-4` (`77e22412`) — TS `Player.faceSquare` (Player.ts:1898-1900) calls `focus(CoordGrid.fine(x,1), CoordGrid.fine(z,1), true)`. `PathingEntity.focus` (PathingEntity.ts:321-333) unconditionally writes `faceAngleX/Z` BEFORE the client gate (L325-326), then writes `faceSquareX/Z` inside it (L329-330). goscape's pre-fix `(*Player).FaceSquare` only wrote `faceSquareX/Z` plus the mask. The missed `faceAngleX/Z` writes matter because faceAngle is the persistent resting orientation that survives the per-tick faceSquare reset (`effectiveFaceCoord` at player_script.go:645 falls back to it when faceSquare resets to -1 each tick — see `TestResetMasksResetsFaceSquare`); a P_FACESQUARE issued without a follow-up walk step left faceAngle stuck at its prior value (typically south from `unfocus()`), so the next tick's forced FACE_COORD low-def emit silently re-oriented the entity to that stale direction. New `TestFaceSquare_WritesFaceAngle` pins both halves at the focus fine coord (Fine(x,1) = x*2+1) with explicit pre-call capture of the southern faceAngle to prove the write actually changes it (RED `got (6401,6399), want (6421,6421)` → GREEN). `TestResetMasksResetsFaceSquare`'s narrative comment realigned (the "(south)" annotation was the pre-fix accident — TS-faithful faceAngle is now the focus coord). `player-script-7` (`0c41ac98`) — TS `Player.clearPendingAction` (Player.ts:950-953) is just `clearInteraction() + closeModal()`. `clearInteraction` (PathingEntity.ts:550-555) resets target/targetOp/targetSubject/apRange/apRangeCalled. goscape's pre-fix `(*Player).ClearPendingAction` did a partial inline reset (target + targetOp only), leaving apRange / apRangeCalled / targetSubject stuck at their last interaction's values. Many handlers funnel through `ClearPendingAction` (handler_opheld.go, handler_op_player.go, minimap-walk modal-close — 8 callsites), so any of them could carry a stale apRange / apRangeCalled / targetSubject into the next SetInteraction. Post-interaction-6 `(*Player).ClearInteraction` is itself TS-faithful, so delegating gives the full TS-shape with no duplicated knowledge; interactionKind is goscape-specific (outside TS's clearInteraction) — preserved as a defensive reset to InteractionEngine. Coupled cleanup: player-script-1's LANDED `StopAction` added an explicit leading `ClearInteraction` call as a defensive cover for player-script-7's gap; with the gap closed, the explicit leading call is now redundant and dropped to match TS `Player.stopAction` (Player.ts:944-947) = `clearPendingAction() + unsetMapFlag()` exactly (existing `TestStopAction_ClearsWaypointAndEmitsUnsetMapFlag` stays green). New `TestClearPendingAction_FullyClearsInteractionState` pins all three post-fix invariants (apRange=10, apRangeCalled=false, targetSubject fully -1) with explicit pre-call setup at the bug-leaking values (RED on all three sub-assertions → GREEN). All 4 with clean RED→GREEN proofs + toggle-off proofs; `go test ./modules/world/ ./pkg/objtype/` green (150.7s + 0.01s, `-race` unavailable — no C compiler); build/vet/gofmt clean. Four rows moved to `docs/PORTING-CLOSED.md`. `med-bundle-6` (4 MED picks across 4 disjoint code files — `modules/world/player_script.go` + new helper in `modules/world/reboot.go` (world-tick-2 — `CanAccess` shutdown short-circuit and a new `(*Server).shutdown()` getter), `modules/world/npc_interaction.go` (npc-core-2 — wanderMode + updateMovement live `n.typ.MoveRestrict` reads), `pkg/script/handlers_server.go` (h-server-1 — `MOVECOORD` routes through `coordgrid.PackCoord`), `pkg/zone/list.go` + doc-comment update on `modules/world/loc_tracker.go` (datastruct-db-1 — `DoublyLinkList.All` captures next/prev before yield); single ff-merge per the med-bundle-1/-2/-3/-4/-5 template) **FIXED** on branch `fix/med-bundle-6`: `world-tick-2` (`792cd247`) — TS `Player.canAccess` (Player.ts:805-812) short-circuits to `true` whenever `World.shutdown` is true (World.ts:197-199 — `shutdownTick != -1 && currentTick >= shutdownTick`), with the TS comment spelling it out: "once the world has gone past shutting down, no protection rules apply". This relaxation is what lets the shutdown drain complete — handler resolutions, teleport-to-spawn, and other forced operations must not be blocked by a stuck modal, a delayed flag, or a stored protected script. goscape's `(*Player).CanAccess` (modules/world/player_script.go:400-411) omitted that gate entirely; the doc-comment said "goscape has no global shutdown flag to consult" — outdated since NAI-182 wired up `s.shutdownTick` and the just-landed `world-tick-1` fix added direct force-removal at the 1024-tick exhaustion. Fix: added a `(*Server).shutdown()` helper (reboot.go) matching TS `World.shutdown` getter, and inserted a nil-safe early-return at the top of CanAccess: when `p.client.server.shutdown()` is true, return true unconditionally. The two nil guards on `p.client` and `p.client.server` tolerate the bare-Player test fixtures (`newPlayer` / `newTestClient`) that intentionally don't wire a server — those preserve the existing `TestPlayerCanAccess` four-case truth table by falling through to the default path with `shutdown()=false` (zero-value `shutdownTick=-1`). Doc-comment rewritten to cite TS L806-808 with the verbatim "no protection rules apply" rationale. New `TestCanAccess_ShutdownRelaxation` stacks every block predicate (delayed + modalMain + activeScript with PtrProtectedActivePlayer) on a player wired to a server, then walks four shutdown states: shutdownTick=-1 (precondition: blocks fire); pending (currentTick<shutdownTick: blocks fire); past-deadline (currentTick>shutdownTick: relaxation lifts blocks); at-deadline (currentTick==shutdownTick: TS uses >= so also lifts). Toggle-off proof: removing the early-return reproduces RED on both past-deadline and at-deadline. New `TestCanAccess_NilServerSafe`: bare Player from `newPlayer/newTestClient` (no server wired) must reach the default true branch via the nil guard, not nil-deref. `npc-core-2` (`1fa39bf0`) — TS `Npc.updateMovement` (Npc.ts:337-341) and `Npc.wanderMode` (Npc.ts:697-703) both open with `const type = NpcType.get(this.type)` and read `type.moverestrict` on every tick, so a `ChangeType` swap to a NoMove (or out-of-NoMove) type takes effect immediately on the next movement/wander tick. goscape's `n.moveRestrict` is a frozen snapshot captured at NewNpc (npc.go:187 — `moveRestrict: MoveRestrict(typ.MoveRestrict)`); `changeTypeImpl` (npc_masks.go:87) refreshes `n.typ` to point at the new NpcType, but never refreshes `n.moveRestrict` — so an NPC ChangeType'd into a NoMove type kept walking, and one ChangeType'd out of NoMove stayed frozen. The bug is symmetric. Fix: both sites now read `MoveRestrict(n.typ.MoveRestrict)` — the live value from the typ pointer that ChangeType already keeps fresh. `wanderMode` already had `if n.typ == nil { return }` at the top, so the live read is unconditionally safe; `updateMovement` has no such pre-flight, so the new code falls back to the frozen `n.moveRestrict` only when `n.typ == nil` (preserves the existing nil-typ test path at npc_interaction_test.go:653 which asserts that step still proceeds; production NPCs always have typ wired). New `TestUpdateMovement_LiveMoveRestrict`: NPC constructed with MoveRestrict=Normal (frozen `n.moveRestrict=Normal`), then `n.typ.MoveRestrict` mutated to NoMove simulating a ChangeType refresh — updateMovement must return false + reset directions (live NoMove short-circuit). New `TestWanderMode_LiveMoveRestrict`: same setup; over 400 wander ticks ZERO waypoints must be queued (pre-fix queues ~50). Toggle-off proofs on both halves confirmed. Existing wander frequency / 0-range / nil-typ tests stay green. `h-server-1` (`5f3fe9b5`) — TS `ServerOps.ts:106` packs the result via `CoordGrid.packCoord` (CoordGrid.ts:136-138): `(z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)`. goscape pre-fix packed raw at `handlers_server.go:96`: `(level << 28) | (cx << 14) | cz` — any cx (or cz) delta that pushed the post-offset value above 0x3fff (16383) bled into the level field, and any level above 3 bled past bit 31 into the sign bit. Concretely: start cx=0x3fff with delta +1 → cx=0x4000; pre-fix OR-shift lands the 0x4000 bit at bit 28 (lowest level bit) → unpacked level reads as 1 instead of 0. Fix: route the push through `pkg/coordgrid.PackCoord` (coordgrid.go:167 — already the byte-equivalent port of TS CoordGrid.packCoord, used across pack/* + rsbuf for the same wire shape). New `TestMoveCoord_AppliesPackingMasks` starts at (level=0, cx=0x3fff, cz=0) with x=+1 delta and asserts unpacked level=0, cx=0x0000, cz=0x0000 (TS-faithful mask). Pre-fix RED: `level bits: got 1, want 0`. Toggle-off proof: reverting the masked `PackCoord` call to the raw OR-shift reproduces RED with the exact "cx=0x4000 overflow must NOT bleed into level" message. Existing `TestMoveCoord` (small in-bounds values) stays green — masks have no observable effect when cx/cz/level are within their respective ranges. `datastruct-db-1` (`327ca754`) — TS `DoublyLinkList.all` (datastruct/DoublyLinkList.ts:73-87) captures `this.cursor` (which is the NEXT pointer) BEFORE each yield and restores it after, exactly so a yield body that Unlinks the just-yielded node — Unlink clears the node's `next2/prev2` pointers — does not strand the iterator. goscape pre-fix at `pkg/zone/list.go:68-84` advanced via `n = n.next` (or `n = n.prev`) AFTER yield, so if yield called `Unlink` on n the loop terminated at n (n.next was cleared to nil by Unlink). The `loc_tracker.go` doc-comment had a workaround sentence telling callers to snapshot before iterating. Fix: capture next (or prev) BEFORE yield, mirroring TS L77-80 1:1 (goscape doesn't need a cursor field — the local `save` is the same idea). Removing OTHER nodes during a yield body (specifically the saved next pointer) is still unsafe — TS shares the same limitation; snapshot-first remains the right pattern for callers that touch more than the current node, and `loc_tracker.go`'s doc-comment is updated to spell that out. New `TestDoublyLinkList_AllForward_SurvivesMidIterationUnlink`: list `[1,2,3,4,5]`; yield body Unlinks the element with Value=2; full iteration must yield `[1,2,3,4,5]` (pre-fix stops at 2 because Unlink clears `n.next`; post-fix continues). New `TestDoublyLinkList_AllReverse_SurvivesMidIterationUnlink` mirrors the above for reverse=true. Toggle-off proofs verified on both directions. All 4 with clean RED→GREEN proofs + toggle-off proofs; `go test ./modules/world/ ./pkg/zone/ ./pkg/script/` green (`-race` unavailable — no C compiler); build/vet/gofmt clean. Four rows moved to `docs/PORTING-CLOSED.md`. `med-bundle-7` (4 MED picks across 4 fully-disjoint fresh-file packages — `pkg/objtype/seqtype.go` (cfg-media-1 — SeqType opcode-1 bounds-guard removal), `pkg/io/packet/packet.go` (io-packet-4 — GJStr drops final byte on no-terminator), `pkg/pack/compiler/symbols.go` (pack-media-compiler-1 — 7 missing ScriptVarType cases), `modules/world/config.go` (logger-transport-3 — `world.tcp-read-timeout` default 5s→30s); single ff-merge per the med-bundle-1..-6 template) **FIXED** on branch `fix/med-bundle-7`: `cfg-media-1` (`5208b685`) — TS `SeqType.decode` opcode-1 at `SeqType.ts:95-102` unconditionally derefs `SeqFrame.instances[this.frames[i]].delay` when per-frame delay reads 0; if `frames[i]` is OOR, TS throws TypeError and aborts the parse. goscape pre-fix wrapped the deref in `idx >= 0 && idx < len(Instances)`, silently falling through to the L101 default of `d = 1` and masking the bad-data condition. Fix: drop the bounds-check conditions; nil-frames guard preserved as CONFIRMED-EXCEPTION (additive robustness; production callers wire frames before Decode). `TestSeqTypeDecode_DelayZeroOutOfRangeFramesIndex_Panics` pins the new behaviour with deferred recover; existing in-range + nil-frames tests stay green. Coupled `pkg/pack/seq_roundtrip_test.go` fixture re-point: `TestPackSeqRoundTrip` used `&SeqFrameConfigs{}` with empty Instances and packed a frame at index 0; pre-fix the bounds guard caught the OOR and emitted `delay=1`, post-fix it would nil-deref. Added explicit `delay1=1` to the test script to sidestep the L97 fallback entirely (test scope is Loops/Priority/Frames round-trip, not delay-fallback). `io-packet-4` (`17094fdb`) — TS `Packet.gjstr` at `Packet.ts:267-276` reads each byte BEFORE the `(b !== terminator && pos < length)` check; on a buffer with no terminator the final iteration reads the last byte (pos advances past it) and the AND short-circuit exits without appending — so TS drops the final byte from the returned string while still consuming it. goscape pre-fix's no-terminator branch sliced `Data[start : start+length]` with `length = Pos-start`, keeping the final byte TS would have dropped (trailing-garbage character on any unterminated JagString read). Fix: `length := p.Pos - start - 1` in the no-terminator branch only; terminator-found branch unchanged. New `TestPacket_GJStr_NoTerminator_DropsFinalByte` pins both shape ("hello" → "hell") and consumption (Pos advances to len); existing terminator-bearing tests stay green. `pack-media-compiler-1` (`7ee78c9b`) — TS `ScriptVarType.getType` at `ScriptVarType.ts:28-83` has 25 case arms; goscape's `scriptVarTypeName` (pkg/pack/compiler/symbols.go:175-216) covered 18 and fell through to "unknown" for the remaining 7 (`AutoInt` 255, `Varp` 86, `PlayerUid` 112, `NpcUid` 78, `NpcStat` 254, `Idkit` 75, `Dbrow` 208). All seven constants exist in `pkg/objtype/scriptvartype.go:11-35`; only the name-rendering switch was incomplete, so every compiler symbol whose declared type was one of those 7 had its type name rendered as the literal string "unknown" in symbol-table output. Fix: insert the 7 case arms in objtype declaration order, immediately before the default arm. `TestScriptVarTypeName_KnownCodes` table extended with 7 new entries; pre-fix all 7 fail in one run with `scriptVarTypeName(N) = "unknown", want "X"`. `logger-transport-3` (`e6a7f4f9`) — TS `TcpServer.ts:19` sets per-connection idle-socket timeout to 30000 ms via `s.setTimeout(30000)`. goscape's `TCPServerReadTimeout` flag (modules/world/config.go:77) drove the equivalent behaviour via `SetReadDeadline` but defaulted to 5s — six times faster than TS, so idle keep-alive clients could be disconnected aggressively where TS would have waited. Fix: default `5*time.Second` → `30*time.Second`; debug-socket mode (NodeDebugSocket=true at server.go:830) already bypasses the deadline and is untouched. New `TestConfigTCPServerReadTimeoutDefault` pins the new default; no operator config in `deploy/` overrides this flag so production deployments inherit it. All 4 with clean RED→GREEN proofs + toggle-off proofs; `go test ./pkg/objtype/ ./pkg/io/packet/ ./pkg/pack/... ./modules/world/` green (~150s incl. modules/world; `-race` unavailable — no C compiler); build/vet/gofmt clean. Four rows moved to `docs/PORTING-CLOSED.md`. Next: all CRIT+HIGH closed plus sixty MED (`rsbuf-npc-1`, `interaction-4`, `world-tick-1`, `interaction-6`, `interaction-7`, `player-script-1`, `npc-hunt-1`, `npc-hunt-2`, `npc-hunt-3`, `pathing-2`, `player-net-2`, `player-net-3`, `player-net-1`, `npc-ai-3`, `player-core-2`, `npc-ai-5`, `pathing-5`, `interaction-5`, `entity-base-3`, `h-loc-1`, `zone-sub-5`, `h-loc-2`, `h-loc-4`, `h-loc-6`, `player-net-5`, `npc-core-1`, `npc-ai-4`, `h-loc-3`, `h-player-2`, `h-player-3`, `h-inv-1`, `h-inv-2`, `h-config-4 / h-obj-3`, `h-obj-2`, `player-core-3`, `zone-sub-1`, `zone-sub-4`, `cfg-onl-2`, `player-core-1 / pathing-4`, `player-script-7`, `world-tick-2`, `npc-core-2`, `h-server-1`, `datastruct-db-1`, `cfg-media-1`, `io-packet-4`, `pack-media-compiler-1`, `logger-transport-3`, `world-tick-5`, `login-server-4`, `pathfinder-3`, `util-1`, `friend-server-1`, `pathfinder-1`, `inventory-2 / gap-server-codec-models-1`, `world-ops-3`, `world-ops-2`, `world-tick-4`, `h-config-3`, `net-client-h-social-5`); med-bundle-8 also closes the COMMENT-LIE row `npc-ai-8` (the tick.go:912-916 despawn comment's "tick_recovery covers the panic case" claim becomes truthful once recoverNpc is wired) and the explicit dup row `player-core-9` (same lines as util-1) — both ledger-only (never listed in PORTING.md Open). med-bundle-9 closes 4 small MED picks across 4 fully-disjoint packages (`modules/friends/repository.go` for `friend-server-1` — FIRST touch of modules/friends/ by any bundle; `pkg/pathfinder/routefinder/routefinder.go` for `pathfinder-1` — FIRST touch of pkg/pathfinder/routefinder/ in this arc; `modules/world/inv_update.go` for `inventory-2 / gap-server-codec-models-1` — merged-alias finding at the encoder boundary, counted as one slot; `modules/world/npc_registry.go` for `world-ops-3` — at line-range disjoint from med-bundle-2's `npcResetTypeSubsystemTeleportOnRespawn`). All 4 with clean RED→GREEN proofs + toggle-revert proofs; `go test ./modules/friends/ ./pkg/pathfinder/... ./modules/world/` green (~149s + 11s + 1s incl. modules/world; `-race` unavailable — no C compiler); build/vet/gofmt clean on touched files. Four rows moved to `docs/PORTING-CLOSED.md`. med-bundle-10 closes 4 more small MED picks across 4 disjoint files (`modules/world/server.go` for `world-ops-2` — clear NPC-collision footprint on removePlayer matching `World.ts:1601`; `modules/world/reboot.go` for `world-tick-4` — `tickRate=0` acceleration after `duration>2` matching `World.ts:1222-1225`, line-range disjoint from world-tick-1/2's processShutdown hunks; `pkg/script/handlers_config.go` for `h-config-3` — paramLookup falls through to default on type mismatch matching `ParamHelper.ts:10-24`, hunks disjoint from med-bundle-4's h-config-4 default-branch fix at the same function; `modules/world/handler_interface.go` for `net-client-h-social-5` — IdkSaveDesign validates even when `s.idkTypes` is nil matching `IdkSaveDesignHandler.ts:18-35`, with a coupled re-point of `TestHandleIdkSaveDesignColorOutOfBounds` per the cfg-media-1 precedent so the color guard remains the test's discriminator now that the idk gate rejects nil-registry input). One pick-swap mid-bundle: `friend-server-5` was scoped out after the schema-decoupling note in `modules/friends/db.go` clarified the federated-DB design ("no account FK; orphan rows accepted as federation trade-off") — closing it cleanly would require either a cross-RPC existence check into the login store or formalising the federation trade-off as a CONFIRMED-EXCEPTION, neither a bundle-slot decision. `net-client-h-social-5` replaced it. All 4 with clean RED→GREEN proofs + toggle-revert proofs; `go test ./pkg/script/ ./modules/world/` green (~150s incl. modules/world; `-race` unavailable — no C compiler); build/vet clean. Four rows moved to `docs/PORTING-CLOSED.md`. med-bundle-11 closes 4 more small MED picks across 4 disjoint packages (`modules/asset/rs2cgi.go` for `entry-1` — `tryParseIntDefault` ports JS `parseInt(value)` leading-digit semantics matching TS `TryParse.ts:11-26` (whitespace skip, optional sign, `0x`/`0X` hex prefix, then leading decimal digits — `1x`→1 / `10abc`→10 / `3.5`→3 / `0x10`→16 / `  42`→42 vs strict `Atoi` returning 0); `pkg/pathfinder/routefinder/stepvalidator.go` for `pathfinder-2` — `isBlockedSouthEast` default arm gains the leading `(x+size, z-1, FlagBlockSouthEast)` corner check matching rsmod canonical `step_validator.rs`, mirroring the three sibling diagonals' existing leading-corner gates and closing the SE-only omission for size≥3 actors; `pkg/rsbuf/renderer.go` for `rsbuf-player-2` — both low-def builds (`lowDefFull` + `lowDefNoApp`) pass `suppressChat=false` matching Rust `lowdefinition` (`info.rs:296-346`, which never strips CHAT — only the self-echo `highdefinition` arm does at `info.rs:282-293`), preserving CHAT in the add block so a player chatting the same tick they become visible is heard by new observers; `pkg/script/handlers_core.go` + `pkg/script/handlers.go` for `script-core-2` (dup `h-core-5` closes same commit) — both GOSUB handlers gate `FrameSP >= FrameCapacity` at the TOP and return an error matching TS `CoreOps.ts:194-214` throw→Aborted, so a pathological/miscompiled script that nests GOSUB past 50 frames aborts gracefully where the pre-fix `GosubCall` panic crashed the host goroutine). All 4 picks across 4 fully-disjoint pristine-in-this-arc packages (modules/asset and pkg/rsbuf had no prior fix-arc touches; pkg/pathfinder/routefinder had med-bundle-9's pathfinder-1 at a fully different function; pkg/script had several prior touches but handlers_core.go's handleGosub + handlers.go's handleGosubWithParams were both un-touched in this arc) — confirms the "pristine packages still available" thesis from med-bundle-10's NEXT note. A small `style:` followup commit re-aligns the entry-1 table-comments per gofmt (the rs2cgi_test.go controller-side fixup pattern from med-bundle-5). All 4 with clean RED→GREEN proofs + toggle-revert proofs (entry-1: 6 sub-RED cases for the audit-cited divergences; pathfinder-2: cited assertion message reproduces; rsbuf-player-2: both lowDefFull + lowDefNoApp header bits go RED simultaneously; script-core-2: both handlers panic with the GosubCall message pre-fix, gracefully return error post-fix). `go test -count=1 ./modules/asset/ ./pkg/pathfinder/... ./pkg/rsbuf/ ./pkg/script/` green (~15s total; `-race` unavailable — no C compiler); build/vet clean apart from the documented 5 pre-existing `pkg/util/build/build.go` self-assignment warnings (the #274 operator-flip lines). Four IDs closed across 4 fixes (entry-1 / pathfinder-2 / rsbuf-player-2 / script-core-2 + h-core-5 dup); entry-1 was not in PORTING.md's open-table (audit-ledger-only row), so the PORTING.md row removal is 3 lines for the other 3 picks. med-bundle-12 closes 4 more small MED picks across 4 fully-disjoint packages (`modules/login/db.go` + `handler.go` for `login-server-3` — drop `AND node_id = ?` from the setLoggedOut UPDATE and the nodeID param from the signature so a force-logout from a sibling world can still clear a stale `account_login` row, matching TS LoginServer.ts:438-439,484-485; `pkg/io/packet/packet.go` for `net-server-enc-3` — reimplement PJStr to emit `(rune & 0xff)` per UTF-16 code unit (with surrogate-pair handling via `unicode/utf16.EncodeRune` for supplementary planes), matching TS Packet.ts:330-337 `pjstr` and fixing wire-byte divergence on any non-ASCII player chat / username / debug string; `pkg/script/handlers_player.go` for `h-player-4` — extract a pure `statRandomThreshold` helper using `math.Floor` on float64 to match TS Math.floor-toward-`-∞` for the boosted-stat regime where `(99-level)` flips sign, and remove the `checkStatID` abort that TS PlayerOps.ts:578-586 lacks (relies on `(*Player).Stat` returning 0 for OOB ids); `modules/world/player.go` for `player-net-7` (also gap-client-models-1 / gap-configs-snapshot-netbase-1 coupled per the audit row's merged-alias triple) — add a fourth `handled bool` return to `(*Player).readPacket` and gate the per-tick userLimit/clientLimit/restrictedLimit switch on `handled==true`, matching TS NetworkPlayer.ts:143-152's `handler.handle()===true` gate so unhandled opcodes (e.g. EVENT_CAMERA_POSITION at 189 which has no Go handler) no longer burn per-tick budget slots). All 4 picks across 4 fully-disjoint packages (modules/login burned by med-bundle-8's login-server-4 at handler.go:100/117 but db.go was pristine in this arc and login-server-3 is the first touch of setLoggedOut; pkg/io/packet burned by med-bundle-7's io-packet-4 at packet.go:236-252 but the PJStr write path at L407-412 is line-disjoint; pkg/script/handlers_player.go touched by several prior fixes but the STAT_RANDOM handler at L609-628 was un-touched in this arc; modules/world/player.go burned by player-net-1 + player-net-5 at L1163-1169 and L994 but the L1187-1202 hunk plus the readPacket signature change at L1235-1309 are all disjoint). All 4 with clean RED→GREEN proofs + toggle-revert RED proofs (login-server-3: cited assertion message reproduces; net-server-enc-3: 3 non-ASCII subtests go RED while the ASCII regression-guard subtest stays GREEN, confirming the toggle isolated the bug; h-player-4: both the formula-on-boosted-stats and the OOB-abort gates toggled together, the no-divergence formula subtests stay GREEN; player-net-7: clientLimit=5-vs-0 RED on EVENT_CAMERA_POSITION). gofmt -l clean on all 9 touched code files; no style-fixup commit needed. `go test -count=1 ./modules/login/ ./pkg/io/packet/ ./pkg/script/ ./modules/world/` green (login 0.23s, packet 0.004s incl. TestPJStrNoAlloc, script 0.07s, world 149.9s; `-race` unavailable — no C compiler); build/vet clean apart from the documented 5 pre-existing `pkg/util/build/build.go` self-assignment warnings (#274 operator-flip lines). Four IDs closed across 4 fixes (login-server-3 / net-server-enc-3 / h-player-4 / player-net-7), plus 2 coupled aliases via player-net-7 (gap-client-models-1 / gap-configs-snapshot-netbase-1) — PORTING.md row removal count = 4. The MED queue remains — the other MED/LOW rows in the fresh-audit ledger. **2026-06-01 LOW sweep — NAI-91 closed as NO-DIVERGENCE** `8e5441e3` (`fix/nai-91`): the lone `🚧 TIMING` row in Open deviations was a false-deviation. Comments in `modules/world/player_masks.go:99-107` + `modules/world/npc_masks.go:212-218` claimed "TS resetPathingEntity fires at tick start, goscape fires at tick end → 1-tick lag" — but TS actually calls `resetEntity` → `resetPathingEntity` from `World.processCleanup` (W.ts:1138) which runs at tick end AFTER processClientsOut (CLIENT_OUT at W.ts:1122), identical to goscape's tick.go:166-169 cycle order. Both engines arm the entitymask in tick N's cleanup; both consume it in tick N+1's info-pass — byte-equivalent timing. Comment-only fix; removed `PORTING-EXCEPTION (NAI-91, mask-reset-1-tick-lag)` marker from `(*Player).ResetMasks`. Existing trailing-clear tests at `player_masks_test.go:244+` + `npc_masks_test.go:230+` already pin the contract. PORTING.md "Open deviations" lost its only `🚧 TIMING` row; the 2 remaining `🚧 DOC` rows (NAI-30 BuildArea, NAI-111-D1 protect-bool) stay open pending dedicated investigation of their TS-faithful ports. **2026-06-01 LOW sweep continuation — NAI-30 Bundle 4 closed as FIXED** `5eb1856b` (`fix/nai-30`): the `🚧 DOC` row for BuildArea-flattened was a REAL structural-only divergence (TS Player.ts:320 holds a per-player `BuildArea` struct; goscape had flattened the 4 fields + 3 methods onto Player directly). Per the user direction "be true to TS" (option B refused in favor of option A literal port), unflattened into new `modules/world/build_area.go` with `type buildArea struct { player *Player; loadedZones map[int]bool; activeZones map[int]bool; mapsquares map[uint16]bool; lastBuild int }` mirroring TS one-to-one + 5 methods on `*buildArea` (clear, rebuildZones, shouldRebuild, rebuildScenery, rebuildNormal). Player gains `buildArea *buildArea` constructed via `newBuildArea(p)` after the struct literal so the backref mirrors TS `new BuildArea(this)`. Goscape-only `rebuiltOnce bool` stays on Player as the lone NAI-93-divergence-bookkeeping field (forced because tick.go's processLogins sets originX to a real coord BEFORE rebuildNormal runs in processInfo, so TS's originX=-1 first-build sentinel pattern cannot work). TS-fidelity behavior change inside rebuildScenery: stopped clearing activeZones — TS BuildArea.rebuildNormal at BuildArea.ts:71,90 only clears mapsquares + loadedZones, never activeZones (the activeZones clear happens at the top of rebuildZones at BuildArea.ts:33). 28 production + 56 test refs migrated via sed-batched `p.X` → `p.buildArea.X` across 7 files. Existing R-D2 test `TestRebuildScenery_DoesNotPrePopulateActiveZones` → renamed `TestRebuildScenery_DoesNotTouchActiveZones` with strengthened assertion (pre-set sentinel must survive rebuildScenery and be cleared by the subsequent rebuildZones call). Full `go test -count=1 ./modules/world/` green (149.191s); `go test -count=1 ./...` green except the pre-existing `pkg/pack` `TestBuildVerify_BUILD_VERIFY_NotPresent` failure. Sole remaining LOW deviation row: NAI-111-D1 protect-bool. **2026-06-01 LOW sweep continuation — NAI-111-D1 closed as FIXED** `83f1cc7c` (`fix/nai-111-d1`): TS Player.protect (Player.ts:359) is a Player-level "is a protected script executing or suspended" gate, distinct from the script-state PtrProtectedActivePlayer pointer (handler pointerCheck source). Goscape's `protectedScriptActive()` derived the gate from `activeScript.Pointers&PAP`, conflating the two. The conflation blocked TS-faithful CloseModal semantics: TS at Player.ts:746 clears `this.protect=false` unconditionally even mid-flight, but goscape applying that clear by stripping PAP-on-state would re-trigger the NAI-53 T3 regression (tut_close inside [label,tutorial_complete] aborted P_TELEJUMP because the handler pointerCheck failed). Per the user direction "be true to TS", split the concerns: new `protect bool` field on Player mirrors TS Player.ts:359 exactly; protectedScriptActive() rewritten as 1-line wrapper `return p.protect`; set/clear lifecycle now mirrors TS one-to-one — runScript entry sets true (TS L2103), resumeOrFinish unconditionally clears post-Execute (TS L2110), Suspended/PauseButton/CountDialog dispatch arm re-sets true if state.Pointers&PAP still set (TS L2141 preserve-when-delayed — derivation matches because Init at runner.go:38 sets PAP iff protect=true), CloseModal clears unconditionally (TS L746 — does NOT touch activeScript.Pointers, preserving NAI-53 T3 regression-guard), ResetMasks defensive tick-end clear (TS L460). 6 fixture sites updated (5 mechanical adjacency-line `p.protect = true` additions alongside existing `activeScript = {Pointers: PAP}` plants; player_test.go truth-table replaced with p.protect-direct table; modal_close_test.go's TestCloseModal_NotDelayed_ProtectedScriptActiveStaysTrue flipped + renamed `...ProtectClearedTSFaithful` with inverted assertion). Sibling test TestCloseModalPreservesInFlightProtectOnResumedScript stays green unchanged (still pins activeScript.Pointers preservation, the NAI-53 T3 regression-guard at the script-state layer). Third LOW-row sweep closure — completes the 3-of-3 LOW deviation row clearance. Full `go test -count=1 ./modules/world/` green (149.517s); `go test -count=1 ./...` green except the pre-existing pkg/pack failure. **PORTING.md "Open deviations" table now has only the indefinitely-deferred MED ARCH-1 tick_recovery / npc_ai try/catch row remaining; all 3 LOW deviation rows closed.** **2026-06-01 ARC RETROSPECTIVE — Arc 27 fix-arc COMPLETE.** After 106 audit IDs closed across the arc (2 CRIT + 5 HIGH from the 2026-05-28 fresh-audit + ~58 MED via 19 four-pack bundles + 3 MED clusters + 7 MED singletons + the script-core-1/-5 coupled-alias dedicated commit + 3 LOW post-bundle-arc sweep closures), the only OPEN row in PORTING.md is the indefinitely-deferred MED ARCH-1 (tick_recovery / npc_ai try/catch), plus 2 LOW perf hotspots that are not TS-divergences and stay on the watchlist. The "Tracking conventions" section above is expanded with the arc's bundle template + file-disjointness rule + #274 flip-prediction protocol + four-way closure-shape taxonomy (✅ FIXED / ✅ EXCEPTION-DOCUMENTED / ✅ NO-DIVERGENCE / ✅ NOT-A-GAP) + EXCEPTION-pre-classification triage rules + historical-regression-vs-conflation rule + env quirks + sed-batched-rename pattern. Future fix-arcs against this codebase should consult that section before treating any row as "structural-only" or "functionally equivalent" — the 2026-06-01 LOW sweep retrofit found that the NAI-91 / NAI-30 / NAI-111-D1 markers each had hidden behavior deltas inside ostensibly equivalent shapes. **STALE-DEFER comment-cleanup sweep** (`293232bf`, comment-only): purged 7 vestigial "stub" / "skeleton" / "deferred" framings whose underlying behavior had long since shipped — `pkg/script/handlers.go` Camera / SPLIT_* / PushVarn/PopVarn / P_WALK headers + `pkg/rsbuf/npcinfo.go` Encode T3.2-SKELETON narrative + `modules/world/npc_player_modes.go` SMART-deferred claim + `modules/world/npc_masks.go` moveSpeed-deferred reframed to CONFIRMED-EXCEPTION net-equivalent (pathing-10 verdict NONE). Re-verified each site against current TS + Go code per the "Tracking conventions" trace-method-bodies rule before purging. The audit ledger at `2026-05-28-ts-parity-audit-fresh.md:974` explicitly tagged these as "STALE-DEFER markers needing comment cleanup". Most other audit-line-974 targets (heropoints.go Stub headers, loc_iterator.go "equivalent" framing) were already addressed by their underlying closures (util-1/-2 in med-bundle-8, h-loc-4/-5 in the h-loc cluster) — only the 7 entries in this sweep remained.
- Arc 28 (2026-06-01): CategoryType subsystem ported — closes gap-world-reload-events-8 / cfg-var-9 / h-npc-3 cluster (3 audit IDs, 6 commits across TDD per-task). Full TS-faithful CategoryTypeValid bound check at checkCategoryType replaces the partial -1-only stub. See docs/PORTING-CLOSED.md.
- Arc 29 (2026-06-01): login-server-9 / gap-db-datastruct-9 (hiscore write-path port) — FIXED fix/hiscore-port; promoted from EXCEPTION-DOCUMENTED (med-bundle-19). Migration 000004 adds hiscore + hiscore_large tables; PlayerLogout now calls updateHiscores (LoginServer.ts:450). Write-path parity only (TS has no serving endpoint). See docs/PORTING-CLOSED.md.
- rev-244 B3 — engine core ported (Engine-TS `e1dea19f..9aadcec4`): PlayerList+pid `94f40331`/`fcc7e212`, entity deltas (setAnim>= `2f10deb6`, regen `dc33a57b`, modals `d5a70fb1`, overlay `ebce9706`), account_id `07e44a61`, InputTrackingBlob `2f67fed2`, rate-limit removal `f4e7571e`, OnDemand `b2e7adac`+`02ce3929`, handshake `1f69f708`, CrcTable `23cbbc02`, PreloadedPacks deleted `59240b70`, HTTP routes `1de71136`, token/WS `130f6583`, MidiPack `0f1ea964` (closes rev244-b2-midi-window), buildArea.clear `7797c9f7`. gap-db-datastruct-4 CLOSED; 3 new PORTING-EXCEPTION ids (crc-compare, ws-origin, ws-ondemand-gate); client smoke + all windows → B6 (user decision). Full correspondence audit in §rev-244 Bundle audit trail above.
- rev-244 B4 — script runtime ported (Engine-TS `e1dea19f..9aadcec4`, 15 script files + 3 externals): opcode renumber `b663bf63`, huntIterator unify `84b8ea2a`/`491822b8` (NPC_FINDNEXT/npc_huntall split), HINT_NPC/PLAYER `631737b7`, DB_GETFIELD full-column `da896c1a`, InvOps untradeable-stop + wealth re-key `de628f37`, NPC_STATHEAL/MAP_BLOCKED/P_OPOBJ `cb4fab32`, BUFFER_FULL + IF_OPENOVERLAY `0d9f0ad4`, runner deltas `294f5c24`/`c6005b60`, count ops `4268ba95`, cycle stats `c321e11d`+`9a4d9b96`+`aeb70ba7`, MAP_PRODUCTION + MAP_LAST* `5cebb3e9`, IF_SETRECOL wire removal `b7c9d08f`, moved-handler citations `f093d4e6`. CLOSED: B2 IF_SETRECOL deferral, B2→B3→B4 overlay chain, NAI-162-D varbit stubs. 1 new PORTING-EXCEPTION id (`rev244-b4-bwout-reset`); cycle-stats 225-era gap closed (user decision); script.dat numbering window → B6. Full correspondence audit in §rev-244 Bundle audit trail above.
- rev-244 B5 — server/login/db ported (Engine-TS `e1dea19f..9aadcec4`, 14 server files + 2 prisma schemas + 2 externals): login schema 000005 `8fddfb4d` (+`d08963c0` — attempts table, per-profile logged_out/logout_time backfill, message + dormant account_session/wealth_event tables, account.logout_time dropped), RATE_LIMITED/HOP_TIMER enum + bytes 16/9 `7eb38361`, 3-in-5s rate limit `53715e4d` (+`6804c746`), 45s hop timer `d5240e66`, getUnreadMessageCount `83a8e6d6` (+`5c05394a`), friends profile proto `704dad98`, multi-profile server `a7234653` (+`30d65a1e`), public_chat username re-key `062a3293` (+`550bade5`), world-side profile carriage + username public-chat log `1d173abc` (+`96e5fa60`), logger report/input_track seam re-key `4e4f8192`. CLOSED: PORTING-EXCEPTION login-server-7, B3 tracker rows 1/2/4/5 (rate limit, world_heartbeat dead-at-pin NO-OP, logger/friends shapes public-half, messageCount). 1 new PORTING-EXCEPTION id (`rev244-b5-startup-profile`); worker files + website-only schema models formally NOT-PORTED. Full correspondence audit in §rev-244 Bundle audit trail above.
- rev-244 B6 — pack pipeline re-baseline (Engine-TS `e1dea19f..9aadcec4`, 31 tools/pack files + 3 externals): modelFlags plumbing `b692e78b`, PackFile registries `2cfec7ea`+`58619dbc`, CR normalisation `481ea70b`, SeqConfig `619bd681`, NpcConfig `b1cb4832`+`658f2f3f`, ObjConfig `88a01023`, InvConfig `b1d1ce01`, IdkConfig/LocConfig/SpotAnimConfig+CRC pins `9e3f0d5b`, wordenc blob+pal.png+cache-arch-0 `8e0beec0`, gzip model/animset/midi archives `27cfb146`, maps gzip+archive-4 `a824e218`, versionlist `0f348913`, PackAll orchestration `c812b781`, clientinterface CRC/!layerId/modelFlags `d3cf8ec0`, Worldmap 244 `3abb07a9`, CompilerSymbols `8ac35749`, compiler deltas `fee9d9f1`+`effb79f2`+`7bdd56e7`, cf-zlib bit-exact port `5e3cbef4`+`79b7bdd8`, byte-parity gate `a69634e7`, windows closed `a977dd5a`, review cleanup `ccf1133a`. Live smoke PASSED (Client-Java `01f1608`): 3 live-finds fixed `4606660a`/`b26d8dd5`/`973e221b`. Byte-parity: 2,671/2,671 reference files identical. CLOSED: B1 DevThread/format-window, B2 map, B3 midi/app.ts/client-smoke, B4 script.dat — all format windows. NOT-PORTED: `updateCompiler()`/BUILD_STARTUP_UPDATE, `createWorker()`, `printError`, `RuneScriptCompiler.ts`. 2 new PORTING-EXCEPTION ids (`rev244-b6-build-stamp`, `rev244-b6-ondemand-zip`); marker count **22** (confirmed). Full correspondence audit in §rev-244 Bundle audit trail above.
- Strange Plant (triffid) `opnpc`/`apnpc` `npc_delay` continuation fix (2026-06-14): the player-side `resumeOrFinish` switch was missing a `case script.NpcSuspended` arm, so a player-anchored script that suspends via `npc_delay`/`npc_arrivedelay` (every `opnpc`/`apnpc` carries an ActiveNpc) had its continuation DROPPED at the `default` "unsupported execution state" arm. `macro_event_triffid.rs2`'s `[opnpc1]` pick handler ends `npc_delay(22); npc_del;` — the dropped `npc_del` left the picked plant alive, so its hostile `ai_timer` resumed and attacked the player after they had picked the fruit. Fix stores the continuation on the active NPC (`state.ActiveNpc.StoreActiveScript`), mirroring TS `Player.executeScript` (`script.activeNpc.activeScript = script`); pin test `TestOpNpcScriptNpcDelayStoresContinuationOnNpc`. Verified faithful vs rev-244 pin `9aadcec4` (`Player.ts:2136-2137`; NPC_SUSPENDED arm unchanged since `9d771f26`), so this restores fidelity rather than forward-porting. Backported from rev-274 `59d007f1`; local commit `d8c74211`.
- NPC-dispatcher player-suspend continuation fix (2026-06-14): sibling of the Strange Plant fix on the NPC side, found by auditing all three post-`Execute` dispatch sites for this bug class. An NPC-anchored script that carries an active player (`buildNpcScriptState` binds an ActivePlayer target → `state.Self`, e.g. `ai_opplayer`/`ai_applayer`) and suspends ON THAT PLAYER had its continuation DROPPED in `resumeOrFinishNpc`'s `default` arm (clear+warn). TS `Npc.executeScript` stores it on the active player (`Npc.ts:223-224 @9aadcec4`: `} else { script.activePlayer.activeScript = script; }`); fix mirrors that (store on `state.Self`, else clear defensively). LATENT — unreachable in pinned content (the player-suspend opcodes require ProtectedActivePlayer, which NPC-anchored scripts don't grant; combat `ai_opplayer` uses `npc_delay`→NpcSuspended), mirrored for fidelity. Pin test `TestResumeOrFinishNpcStoresPlayerSuspendOnActivePlayer`. Origin rev-274 `5d3dcf4d`; local commit `4847913d`.
- NPC-dispatcher protect-cleanup tail (2026-06-14): completes the `resumeOrFinishNpc` TS-fidelity port (follow-on to the player-suspend fix above). Ported the tail of TS `Npc.executeScript` (`Npc.ts:230-237 @9aadcec4`) that releases protected access held on the active player(s) — `script._activePlayer[2].protect = false` + `pointerRemove(ProtectedActivePlayer[2])` — so a stale protect flag (protectedScriptActive) can't block the player's later interactions. The player dispatcher needs no equivalent (it clears the protagonist's own protect, which IS the active player there; TS `Player.executeScript` has no such tail — verified, no parallel gap). LATENT — `buildNpcScriptState` never sets the ProtectedActivePlayer pointer in npc context; mirrored for fidelity. Pin test `TestResumeOrFinishNpcReleasesProtectedActivePlayer`. Origin rev-274 `54ecab8a`; local commit `2f12813a`.
- Arc-28 fidelity backport from rev-274 (2026-07-02): ported 5 goscape-vs-TS fidelity fixes (arch-28.1 through arch-28.5) as 8 local commits. `9c7c20b6` sqlite pool serialization — `SetMaxOpenConns(1)` + per-connection `busy_timeout`/`foreign_keys` pragmas riding the DSN (a `db.Exec PRAGMA` only reaches one pooled connection), matching TS better-sqlite3's single synchronous connection; fixes SQLITE_BUSY on concurrent writes at mass-logout shutdown. `816b111d` non-zero exit on module failure — restores upstream dskit's post-stop `ServicesByState[Failed]` check that `App.Run` had dropped, so orchestrator restart policies fire again. `244875bd` WS-bridge connections now route through the existing `serveConn` panic-recover + a `wsGateMu`-guarded quit/tcpWg admission gate (`HandleConn` previously called `handleTCPConn` directly, bypassing both — a malformed WS-framed login could crash the whole process). The 28.4 connection-teardown cluster landed as four commits: `9ba6c8fb` adds a guaranteed non-lossy `removalQueue` for disconnect removals (previously rode the lossy 64-slot `relayActionQueue`, which could ghost a disconnected player for the 100-tick no-response timeout); `dc07a4f0` refcounts conn/tick ownership of pooled connection buffers (`client.teardownRefs` + `dropConnRef`/`dropTickRef`), closing a buffer-reuse race where the conn goroutine's teardown could recycle `bufw` into a new connection while the tick was still flushing the old player's frames into it; `f0732a24` adds a `liveConns` registry gated by `admissionGateMu` (renamed from `wsGateMu`, now guarding both admission sites) so `Shutdown` can force-close a chatty connection instead of hanging on `tcpWg.Wait` forever, plus a `worldServiceFns` extraction fixing a `BasicService` stop-without-run deadlock reachable when the service context is cancelled between Starting and Running; `cf60358a` drops the tick-goroutine write budget from 30s to 2s and adds `flushWriteOrClose` so one stalled client can no longer freeze every player's tick for up to 30s. Closing the arc, `42d980b0` adds a bounded 3-attempt retry (`sendPlayerLogoutWithRetry`) for logout-save RPCs, closing a window where a login-service restart during the single prior attempt silently lost up to ~15 minutes of a departing player's progress. Origin commits on rev-274: `2320a18c` (28.1), `73ba3977` (28.2), `ec60a488` (28.3), `78d9b0d9`+`2ea5e1de`+`e4141eb7`+`418bdc8a` (28.4 cluster — rev-274's cluster-review fix-wave commits `8733646d` and `668e273a` were folded into the corresponding commit above rather than kept separate), `c2836731` (28.5); plus three targeted polish items from rev-274's `9f644730`: two folded into their owning commits (sendLoginOK takes the tick's buffer ref before `appendNewPlayer` publishes the player, in the 28.4b commit; the retry loop's slog key is the singular `attempt` on both branches, in the 28.5 commit), and one initially missed and landed in a follow-up review-fixup commit (serveTCP's admission-gate refusal log reads "refusing connection accepted during shutdown" instead of reusing the Accept-error message). NOT ported: rev-274's arch-28.6 CI workflow (repo tooling, not a fidelity fix) and its final gofmt-stable doc-comment commit (rev-274-specific whitespace state). Residual, not fixed, matching rev-274's own posture: the OnDemand pump goroutine (`onDemand.run` → `clientODAdapter.send`) is a bufw writer outside the conn/tick refcount model on a pure-OnDemand connection (`c.player == nil`, refcount pinned at 1) — pre-existing race, documented on `client.teardownRefs`'s field comment. Gates all green on this branch: `go build ./...`; `go test -run '^$' ./...`; the arc's targeted tests (`TestOpenDB*`/`TestDSN*`, `TestFailedServicesError*`, `TestHandleConn*`/`TestRemoval*`/`TestDrainRemovals*`/`TestClientTeardown*`/`TestWorldService*`/`TestShutdown*`/`TestLogoutSave*`/`TestFlushWrite*`/`TestWriteTimeout*`); `-race` on `modules/login`, `modules/friends`, and the world subset; and the full `modules/world` suite (`go test -count=1`, 2322 passed / 0 failed / 19 pre-existing cache-dependent skips) — the rev-274-report drainConn hang did not reproduce on this branch's tip, so its 85876fa7 cherry-pick was not needed. Update (2026-07-02): arch-28.6's CI workflow, noted "NOT ported" above, has now landed — `.github/workflows/go.yml` copied verbatim from rev-274's final state (`25f554eb`) behind an independent branch-local gofmt sweep (build/test/vet/race gates all green).
- Arc-29 fidelity backport from rev-274 (2026-07-02): ported the 4 arch-29 goscape-vs-TS fidelity fixes as 4 local commits, closing the arch-28 residual this branch's own PORTING.md entry (above) had documented. `b865832a` (arch-29.1) closes that residual directly: `client.tryRef()` lets the OnDemand pump (`onDemand.send` → `clientODAdapter.send`, this branch's naming for the pump call chain) take a transient buffer ref (CAS acquire-if-live) around each write+flush instead of running outside the conn/tick refcount model entirely; a refused send returns `net.ErrClosed`, which `onDemand.send` already treats like any other send error (stops sending further chunks); `handleTCPConn`'s teardown defer also skips the pre-login flush for `ClientStateOndemand` conns, since the pump now co-owns `bufw`. `5d6ad85f` (arch-29.2) adds gRPC keepalive: a new `modules/world/grpc_keepalive.go` (`clientKeepaliveParams`/`worldClientKeepalive`, 30s probe / 10s timeout / `PermitWithoutStream=true`) wired into both `NewLoginClient` and `NewFriendsClient` dial options, mirrored by a `KeepaliveEnforcementPolicy` (15s `MinTime`, `PermitWithoutStream=true`) on both `modules/login/server.go` and `modules/friends/server.go` `newGRPCServer` — without this a NAT/firewall silently dropping connection state left subscriber streams blocked in `Recv()` forever, since the reconnect supervisors only fire on stream errors. `872662ac` (arch-29.3, folding the origin's two-commit fix wave) gives `LoginClient.WorldStartup`/`FriendsClient.WorldConnect` error returns (ripples through both fake test impls plus `flakyLoginClient` in `logout_save_retry_test.go` — the compile-all gate is the proof) and adds `Server.retryBridgeRegistration(name, call)`: an idempotent registration retried every `bridgeRetryDelay` (5s default) in a goroutine parented to `bridgesCtx`, tracked in the new `Server.bridgeWg` and joined in `Shutdown` right after `bridgesCancel`; `world.go`'s `startingBody` calls it instead of firing WorldStartup/WorldConnect once inline, so boot no longer stalls on the ~20s gRPC min-connect timeout and a transient login/friends outage at boot self-heals instead of stranding crashed-out players at `ALREADY_LOGGED_IN` forever (WorldStartup's blanket `logged_in=0` clear is the only thing that ever resets that flag). Folded in the same arc's reviewer-caught login-integrity fix: `Server.worldStartupDone` (atomic.Bool) gates `callPlayerLoginRPC` with the identical offline reject (opcode 8) used for an unreachable login server, and only `worldStartupCall` (wrapping the retry) opens it — strictly after the same RPC's `logged_in` wipe — so no login can be admitted before that wipe and then have its live session's flag erased by an eventually-successful retry; `initLoginGate` opens the gate immediately for a standalone world (nil login client). `95f9a1bd` (arch-29.4, folding the origin's two-commit fix wave incl. its bound-tightening follow-up) fixes a `--target friends` SIGTERM hang: `Friends.running`'s `ctx.Done` path now calls the new `(*subscriptions).closeAll`/`(*worldSubscriptions).closeAll` (close every subscriber's `done` channel + delete its entry, one lock acquisition) before `f.srv.shutdown()`, since `SubscribeUpdates`/`SubscribeWorldEvents` handler loops previously had no server-side way to exit while a client stream stayed open (a normal steady state with any world still attached, not a client bug); `grpcServer.shutdown` also now races `GracefulStop` against a `grace` window (`defaultGracefulStopBound` = 5s in production, injectable in tests) and forces `Stop()` if it elapses, backstopping the narrow race where a `Subscribe` lands between `closeAll` running and `GracefulStop` actually closing the listener. `Config.GracefulShutdownTimeout` (already present, pre-existing, on this branch) stays unwired — `grace` is the hardcoded `defaultGracefulStopBound` constant, not read from cfg; wiring it is a later, non-fidelity-tagged arc item and explicitly out of scope for this backport. Origin commits on rev-274: `47d72a4b` (29.1), `9a4a55cf` (29.2), `b7ff6a5f`+`750ded55` (29.3 wave), `f1955711`+`d061d9b9` (29.4 wave). N/A verdicts: none on this branch — all four fixes applied (this branch has the OnDemand pump + `clientODAdapter`, unlike rev-225 where 29.1 is N/A). Gates all green: `go build ./...`; `go test -run '^$' ./...` (compile-all, the proof for the 29.3 interface-signature ripple); the arc's targeted tests (`TestODAdapter*`/`TestTryRef*`/`TestClientTeardown*`/`TestClientKeepalive*`/`TestRetryBridge*`/`TestLoginGate*`/`TestShutdown*`/`TestHandleConn*` in `modules/world`; `TestSubscriptions*`/`TestWorldSubscriptions*`/`TestGRPCServer*`/`TestFriends_Running*` in `modules/friends`); `-race` on the world targeted set, the full `modules/friends` suite, and `modules/login`; and the full `go test -count=1 ./modules/world/... ./modules/friends/...` (both green, world ~36s). Operator note (arch-29.3): a persistently-failing WorldStartup registration now blocks all logins on this world by design (the same wire behavior as an unreachable login server, opcode 8) rather than silently proceeding with a stale `logged_in` flag — if `retryBridgeRegistration`'s "bridge registration failed; retrying" warning repeats, the login server is unreachable and players cannot log in until it recovers. `37028765` (arch-29.13, backported separately on 2026-07-02): goscape fired one goroutine per friends-server mutation RPC (`grpcFriendsBridge`'s Add/RemoveFriend, Add/RemoveIgnore, `SetChatMode`, Private/PublicMessage; the inline `PlayerLogin` call in `tick.go`'s `processLogins`; the inline `PlayerLogout` call in `server.go`'s `removePlayerOnTick`), so gRPC gave no cross-RPC ordering guarantee unlike TS's single `World.friendThread.postMessage` channel, which is strictly FIFO across every player — a login/logout pair for one player could apply to the friends server as logout-then-login, and an add-then-delete pair could apply as delete-then-add. Adds `friendsMutationDispatcher` (new `modules/world/friends_dispatcher.go`): a single global unbounded FIFO queue served by exactly one worker goroutine (deliberately NOT per-player, matching origin's rationale that TS has no per-player partitioning). Every friends-mutation call site above now enqueues a closure instead of firing its own goroutine; `defaultFriendsBridge`/`grpcFriendsBridge` take the dispatcher instead of `parentCtx` (this branch's heaviest ADAPT point — `bridges.go`'s `PublicMessage` keeps its rev-244-specific `username`-keyed signature, converted to enqueue in place, not replaced with rev-274's `sessionUUID` shape). The worker is started from `NewWorldService`'s `startingBody` and folded into the pre-existing `Server.bridgeWg` (this branch's arch-29.3 backport already established the `bridgeWg`/`bridgeWg.Go` convention; this branch predates arch-29.8's world-events-subscriber acquisition-in-starting convention, so the worker spawn has no sibling call to sit next to and is placed standalone in `startingBody`). Two ordering tests (`TestFriendsDispatchPreservesLoginLogoutOrder`, `TestFriendsDispatchAddThenDeleteOrder`), ported verbatim from origin, were first proven RED against a throwaway stub `enqueue` that reproduced the old per-call goroutine fan-out (observed `[logout login]` / `[del add]`), then GREEN against the real single-worker FIFO implementation. Origin commit on rev-274: `adc2fecf`. Gates all green: `go build ./...`; `go test -run '^$' ./...` (compile-all); the two ordering tests plus the branch's full friends-bridge test suites (`TestFriendsDispatch*`/`TestFriendsMutationDispatcher*`/`TestGRPCFriendsBridge*`/`TestDefaultFriendsBridge*`/`TestProcessLogins*`/`TestRemovePlayerOnTick*` in `modules/world`); `-race` on the `modules/world` friends-targeted set and the full `modules/friends` suite; the full `go test -count=1 ./modules/world/... ./modules/friends/...` (both green, world ~39s); `gofmt -l .` empty outside `.claude/`. `[BACKPORT-FIDELITY]`.
- USER-DIRECTED follow-up forward-port (2026-07-03): four rev-274-only operational/lifecycle/config/doc improvements ported onto rev-244 at the user's explicit request ("port it too"), replicating the rev-254 pilot. **This is a deliberate DEPARTURE from goscape's normal no-forward-port policy** — unlike the arch-28/arch-29 backport entries above, NONE of these restore TS fidelity; they replicate later-rev engine behavior onto an older branch on request. (FFP-5) `docs(world)`: the `grpcLoginClient.PlayerForceLogout` wrapper + its `LoginClient` interface entry are documented as retained-but-uncalled (the disconnect-without-save caller was removed for TS parity; the stale "used on disconnect" comment is corrected to say it is kept to mirror the login RPC surface); comment-only, origin `707f5b12`, local `de106a83`. (FFP-2) `fix(ondemand)`: `initOnDemand` binds the HTTP listener via `server.New` then constructs the OnDemand module; when `ondemand.New` errored the bound socket leaked until process exit (no service ever runs to `Shutdown` it). Added `server.Server.Close()` (releases the bound listener for the construct-then-abort path) and call it on the `ondemand.New` error branch; red-green pinned by `TestServerClose_ReleasesListener`; origin `086df9c2`, local `2bef3dac`. (FFP-3) `feat(friends)`: `friends.graceful_shutdown_timeout` was declared but UNWIRED — `newGRPCServer` hardcoded `defaultGracefulStopBound` (5s) and the flag defaulted to 30s. Wired `newGRPCServer` to consume `Config.GracefulShutdownTimeout` (coercing a non-positive value to `defaultGracefulStopBound`), changed the flag default to `defaultGracefulStopBound` so an unset key stays 5s (**behavior-preserving: effective default shutdown timing is unchanged**), and added a `Validate` rule rejecting a non-positive `graceful_shutdown_timeout`; the one enabled-config test that built a zero-grace `Config` (`TestFriends_Running_StopsWithOpenSubscriberStream`) now sets it explicitly to `defaultGracefulStopBound`. New `config_test.go` red-green pins the rule (`TestConfigValidate_RejectsNonPositiveGrace`). Like rev-254, this branch's `newGRPCServer` already took `cfg Config`, so no signature/call-site threading was needed; unlike rev-254, this branch's friends `Config` has no `Profile` field, so `validConfig()` omits it. Origin `fc19df09` (grace-wiring slice) + `c0b80a98` (Validate rule), local `9bbcac62`. (FFP-1) `feat(observability)`: the full arch-29.6 `/healthz` (tick-liveness) + `/debug/status` feature with the boot deadline. `world.Server` gains three atomics (`lastTickNano`/`currentTickAtomic`/`lastCycleMillis`) stamped once per tick by `stampTick()` right after `s.currentTick++`, plus `HealthSnapshot()`; `modules/ondemand/health.go` defines a mirrored `ondemand.HealthSnapshot` + `RegisterHealthRoutes(mux, snap)` (import-free of `modules/world`), with the app wiring a field-by-field adapter in `initOnDemand` (coexisting with the FFP-2 `serv.Close()` change in the same function). The boot deadline treats a non-positive `LastTick` as "starting" within `healthzBootGrace` (30s), only flipping to 503 once a real timestamp goes stale (>10s) or the grace elapses with no first tick. **Branch adaptation:** rev-244's `playerList.count` was a plain `int` (unlike rev-254, where it was already an `atomic.Int32`); it was PROMOTED to `atomic.Int32` here so both `HealthSnapshot`'s cross-goroutine read AND the pre-existing lock-free `getTotalPlayers` read on the connection-admission path (checked against `NodeMaxConnected` while the tick goroutine writes `count`) are race-safe, matching the rev-274 origin and the other rev branches. `set`/`remove` now use `count.Add(±1)`; `getTotalPlayers` and `worldVarsView.PlayerCount` read `count.Load()` lock-free (`PlayerCount` drops its former `RLock`); `HealthSnapshot` reads `count.Load()` directly (no lock). The three tick atomics are added identically to the template. Helm `_helpers.tpl` swaps the readiness probe from `tcpSocket` to `httpGet /healthz` on `ondemand-http` for SingleBinary/World; Management keeps `tcpSocket` on `login-grpc` (no HTTP port) — verified by `helm template` for all three modes. Origin `8fe1a457` + `eb43c055` (boot deadline) + `384701c2` (review fixup), local `ad4998a5`. Gates: `go build ./...`, compile-all (`go test -run '^$' ./...`), `go test ./modules/{ondemand,world,friends}/ ./pkg/dskit/server/ ./cmd/goscape/app/`, `-race` on the touched non-world packages, and `gofmt -l .` (outside `.claude/`) empty — all green.
- Tech-debt cleanup batch (2026-07-03), ported from rev-274's three-agent debt audit, fanned out alongside rev-245.2/rev-225 from the rev-254 pilot. **Behavior-preserving** — no wire/gameplay/cache-output change; A7 (the `fire{Op,Ap}Trigger{Npc,Loc,Obj}` consolidation) was reviewed and **deliberately skipped** at the rev-274 go/no-go (~180 LOC not worth the hot-path fidelity risk). Items A1-A6 and A8-A10 landed per the rev-274 template. **rev-244 divergences**: A4 — same shape as rev-245.2 (18 `packAndSave*` funcs, 3-arg `readAndValidate`, no `varbit` in the transmitted set). A5 was a full re-derivation of the god-file split (2084 LOC / 38 funcs extracted from `server.go`), proven a pure move via package/import-stripped line-diff. Merge tip `f92061cd`. NOT pushed. Cross-ref rev-274 `1953b743` for the full 5-branch record.
- ✅ **DB-2 federation RETIRED (2026-07-06)** — login+friends now share one central database (`pkg/gamedb`, spec `2026-07-05-central-db-consolidation-postgres`), ported to rev-244 per the phase audit (`.superpowers/sdd/audit-port244.md`, `9aadcec4` pin — rev-244's OWN TS pin, NOT rev-245.2's `3c16994c`, rev-254's `2e3bcf43`, or rev-274's `dee467c8`; `merge-base --is-ancestor` confirms `9aadcec4`/`3c16994c` are not ancestors of each other despite being committed six minutes apart same-day). **Audit finding: the entire friend-persistence surface at `9aadcec4` is byte-identical to rev-245.2's own pin (`3c16994c`)** — confirmed by `git diff 3c16994c 9aadcec4 -- src/server/friend/FriendServer.ts src/server/friend/FriendServerRepository.ts prisma/singleworld/schema.prisma prisma/multiworld/schema.prisma`, exactly one hunk, and by an empty `git diff 3c16994c 9aadcec4 -- prisma/*/migrations`. **The one delta is non-persistence**: `register()`'s in-memory staff gate (FriendServerRepository.ts:83) flips from `!this.playerStaff.has(username37) && staffLvl > 0` (3c16994c) to `staffLvl > 1 && !this.playerStaff.has(username37)` (9aadcec4) — operand-order + threshold bump, governing only the in-process `playerStaff` set consulted by `isVisibleTo`'s staff-bypass; no SQL, no prisma model, no insert/select shape differs. goscape's own `isStaffLocked` (`modules/friends/repository.go:580-586`) already gated on `staffLvl > 1` with a doc comment correctly citing `9aadcec4` — **this port needed no change on that point**, it was already TS-244-correct going in. Every persistence-surface finding from the rev-245.2 audit therefore transfers verbatim: **(1) no members-aware cap** — flat `friendListLimit = 100`/`ignoreListLimit = 100` for both `AddFriend`/`AddIgnore`, matching FriendServerRepository.ts:233/268 exactly (no `account.members` read at this pin); **(2) `public_chat` is the OLDER 6-column account_id+profile+world-keyed shape**, not the 4-column `session_uuid` shape rev-254/274 target — `LogPublicMessage` resolves the wire `username` to `account_id` via `accountIDByUsername`, mirroring TS's `executeTakeFirstOrThrow` (FriendServer.ts:294); a missing account surfaces `errAccountMissing`, mapped by the `PublicMessage` handler to a silent success, matching TS's throw + outer per-connection try/catch drop. `pkg/gamedb` (sqlite + postgres backends, pgx/v5) copied from rev-245.2's reviewed port (source tip `f6c3a596`; landed here as `354d4eaf`), unchanged from that copy — same 6-column `public_chat` schema (FK + ON DELETE CASCADE, `idx_public_chat_account`), same `friendlist`/`ignorelist`/`private_chat`/`login`/`account` tables. Restored TS behaviors previously excepted: `addFriend`/`addIgnore` owner+target account resolution (FriendServerRepository.ts:210-247/249-294 @9aadcec4) with the flat-100 cap noted above (no profile filter on the COUNT, matching TS exactly); PM account-existence check (FriendServer.ts:275-276 @9aadcec4 — the `NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK` exception blocks in `repository.go`/`handler.go` are deleted, `LogPrivateMessage` now returns `errAccountMissing` and `PrivateMessage` maps it to a silent success matching TS's throw-inside-catch drop); `public_chat` account_id resolution described above. Deliberate non-FKs (matching TS): `ignorelist.value` (TS's `addIgnore` never resolves its target `value37` against `account` at any pin — the raw username string is stored as-is). Deletes the federation-era `modules/friends/{db.go,db_test.go,migrations/*.sql}` (superseded by `pkg/gamedb/migrations/`); `friends.go`/`config.go` retire `SQLiteDSN` in favor of `gamedb.Config` (independent-clients model, same posture as login); `cmd/goscape/app/modules.go`'s `initFriends` now passes `g.cfg.Database`. Supersedes the "friends `public_chat` account_id resolution NO-LANDING-SITE" and "FriendServerRepository internals NO-OP/N-A" rows above, both marked retired with a pointer back to this entry. Full audit/citation/test record: `.superpowers/sdd/audit-port244.md`, `.superpowers/sdd/port244-t2-report.md`..`t6-report.md`. Phase commits `354d4eaf`..`003dded2` (direct on rev-244, no merge). NOT pushed.
