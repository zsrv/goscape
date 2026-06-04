# Multi-Revision Repo Conversion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert goscape from a single-`main` repo into the goscape-client multi-revision branch model: `main` = cross-revision docs hub (no code), `rev-225` = the full existing history.

**Architecture:** Pure git branch surgery plus one docs-hub commit — **no history rewrite**, no remote. Tidy commits land first (so `rev-225` owns them), then `rev-225` is branched at the tidied tip, `main` is re-pointed at the initial commit `6579371c`, and one "Cross-revision docs hub" commit gives `main` exactly 4 files: `README.md`, `REFERENCES.md`, `PORTING-LESSONS.md`, `.gitignore`.

**Tech Stack:** git, Go toolchain (verification only — no code changes).

**Spec:** `docs/superpowers/specs/2026-06-03-multi-revision-repo-design.md`

**Conventions for every commit in this plan:** run from the repo root `/home/owner/Code/github.com/zsrv/goscape`; always `git commit --no-gpg-sign`; end every commit message with the `Co-Authored-By: Claude …` trailer shown in each step.

**Read this whole plan before starting.** Mid-execution (Task 4) the worktree sits on the docs-hub `main`, where this plan file does not exist on disk (it is tracked on the `rev-225` lineage). Every file content needed is inline below; if you need the plan itself while on `main`, read it via `git show rev-225:docs/superpowers/plans/2026-06-03-multi-revision-repo-conversion.md`.

---

### Task 1: Pre-flight check + tidy commit 1 (durable docs)

**Files:**
- Add (already exist on disk, untracked): `RUNESCRIPT.md`, `docs/superpowers/handoffs/2026-05-23-ts-parity-full-audit-resume.md`, `docs/superpowers/plans/2026-05-23-pack-determinism.md`
- Add: `docs/superpowers/plans/2026-06-03-multi-revision-repo-conversion.md` (this plan, if not yet committed)

- [ ] **Step 1: Verify the working tree has no modified tracked files**

Run: `git status --porcelain`

Expected: only `??` (untracked) lines — `.claude/`, `RUNESCRIPT.md`, `docs/superpowers/handoffs/2026-05-23-ts-parity-full-audit-resume.md`, `docs/superpowers/plans/2026-05-23-pack-determinism.md`, `public/`, and possibly this plan file. **If any line starts with `M`/`A`/`D`/`R` (modified tracked files — `deploy/bundled/goscape.yaml` and `pkg/util/build/build.go` are known offenders per PORTING.md "#274 flip-prediction"), STOP and ask the user before proceeding.**

- [ ] **Step 2: Verify we are on `main` and record the starting tip**

Run: `git branch --show-current && git rev-parse main`

Expected: `main` and a 40-char SHA (the pre-tidy tip; the post-tidy tip is recorded in Task 3).

- [ ] **Step 3: Commit the durable docs**

```bash
git add RUNESCRIPT.md \
  docs/superpowers/handoffs/2026-05-23-ts-parity-full-audit-resume.md \
  docs/superpowers/plans/2026-05-23-pack-determinism.md \
  docs/superpowers/plans/2026-06-03-multi-revision-repo-conversion.md
git commit --no-gpg-sign -m "docs: commit untracked durable docs before multi-revision conversion

RUNESCRIPT.md (RuneScript language reference) + two superpowers work docs
+ the conversion plan, so the rev-225 lineage owns them.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 4: Verify the commit**

Run: `git status --porcelain`

Expected: only `?? .claude/` and `?? public/` remain.

### Task 2: Tidy commit 2 (.gitignore)

**Files:**
- Modify: `.gitignore` (repo root)

- [ ] **Step 1: Replace `.gitignore` with the following exact content**

```gitignore
# IDEs
.idea/
.vscode/

# Binaries
*.exe
cmd/goscape/goscape
cmd/goscape/goscape-debug
cmd/goscape-cli/goscape-cli
dist/


data/
dataplayers/

# Runtime assets (soundfonts etc.) — not source
public/

.cache/
.serena/

# Git worktrees
.worktrees/
.claude/worktrees/

# Claude Code: per-user local settings + personal session state. To SHARE
# team config, un-ignore specific paths, e.g.:
#   !.claude/settings.json
#   !.claude/commands/
.claude/
# Project MCP config can carry server URLs / tokens — ignore by default.
.mcp.json
# Private per its own header (CLAUDE.md, by contrast, stays committed).
CLAUDE.local.md
```

(This is the current file with four additions: `public/`, `.claude/`, `.mcp.json`, `CLAUDE.local.md` — matching goscape-client's ignore conventions. The pre-existing `.claude/worktrees/` line is kept; it is now redundant but harmless.)

- [ ] **Step 2: Verify the ignores take effect**

Run: `git status --porcelain`

Expected: exactly one line — `M .gitignore` (the `?? .claude/` and `?? public/` lines disappear).

- [ ] **Step 3: Commit**

```bash
git add .gitignore
git commit --no-gpg-sign -m "chore: gitignore public/ and Claude Code local state

public/ holds runtime assets (soundfont); .claude/ + .mcp.json +
CLAUDE.local.md are per-user local config, mirroring goscape-client's
ignore conventions.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 4: Verify clean tree**

Run: `git status --porcelain`

Expected: no output.

### Task 3: Branch surgery

**Files:** none (branch pointers only).

- [ ] **Step 1: Record the post-tidy tip SHA (this becomes the `rev-225` tip)**

```bash
PRE_SHA=$(git rev-parse main) && echo "rev-225 will be: $PRE_SHA"
```

Write the printed SHA down (it is re-checked in Step 4 and Task 5).

- [ ] **Step 2: Create `rev-225` at the tidied `main` tip and switch to it**

```bash
git branch rev-225 main
git checkout rev-225
```

Expected: `Switched to branch 'rev-225'`.

- [ ] **Step 3: Re-point `main` at the initial commit**

```bash
git branch -f main 6579371cec7baa180461d30dd939422e24ce544c
```

(`6579371c` = "Initial commit", 2025-08-21 — 5 files: `build.sh`, `cmd/goscape/main.go`, `config.yaml`, `modules/asset/config.go`, `pkg/util/log/log.go`.)

- [ ] **Step 4: Verify both pointers**

```bash
git rev-parse rev-225 && git rev-parse main
```

Expected: first line = `$PRE_SHA` from Step 1; second line = `6579371cec7baa180461d30dd939422e24ce544c`.

### Task 4: Docs-hub commit on `main`

**Files (all relative to repo root, written while on `main`):**
- Delete: `build.sh`, `cmd/goscape/main.go`, `config.yaml`, `modules/asset/config.go`, `pkg/util/log/log.go`
- Create: `README.md`, `REFERENCES.md`, `PORTING-LESSONS.md`, `.gitignore`

- [ ] **Step 1: Switch to `main`**

```bash
git checkout main
```

Expected: `Switched to branch 'main'`. The worktree now shows only the 5 initial-commit files plus ignored/untracked leftovers (`.claude/`, `public/`). **Note:** this plan file and CLAUDE.md are no longer on disk — they are tracked on `rev-225`.

- [ ] **Step 2: Remove the 5 initial-commit code files**

```bash
git rm build.sh cmd/goscape/main.go config.yaml modules/asset/config.go pkg/util/log/log.go
```

Expected: 5 `rm '…'` lines.

- [ ] **Step 3: Create `README.md` with this exact content**

```markdown
# goscape

A Go rewrite of the [Lost City](https://github.com/LostCityRS) TypeScript
RuneScape server ([Engine-TS](https://github.com/LostCityRS/Engine-TS)),
compatible with the Lost City Java client.

This repository hosts **one Go port per game revision, one branch per
revision**. `main` carries no code — only the cross-revision documentation
below.

## Branch model

| Branch | Contents |
|---|---|
| `main` | This docs hub: cross-revision references and porting lessons |
| `rev-225` | The complete revision-225 server port (full history) |

Future revisions: branch `rev-N` from the nearest prior revision branch and
apply the upstream delta — see the "Future revisions" recipe in
[`REFERENCES.md`](REFERENCES.md) and the porting workflow in
[`PORTING-LESSONS.md`](PORTING-LESSONS.md).

## Files on this branch

- [`REFERENCES.md`](REFERENCES.md) — the upstream reference repos + commits
  each revision was ported from. Treat it like a lockfile.
- [`PORTING-LESSONS.md`](PORTING-LESSONS.md) — durable TS→Go porting
  knowledge that applies across revisions. Read it before starting a new
  revision.

Revision-specific docs (architecture, build/run, the deviation tracker) live
on each revision branch: see `README.md`, `CLAUDE.md`, `PORTING.md`, and
`docs/PORTING-CLOSED.md` on `rev-225`.
```

- [ ] **Step 4: Create `REFERENCES.md` with this exact content**

```markdown
# Reference Sources

The upstream sources each Go revision was ported **from**. Branch names move,
so the **commit hash is the real pin** — treat this file like a lockfile for
the port. To port a new revision, diff the new reference commit against the
commit recorded here for the revision you branch from (see the "Porting
workflow" section of `PORTING-LESSONS.md`).

Local working-copy paths are machine-specific and do not belong here; only
the portable URL / branch / commit do.

## rev-225 — Go branch `rev-225`

| Repo | Role | URL | Branch | Pinned commit |
|---|---|---|---|---|
| Engine-TS | **primary** — authoritative translation source; every ported Go region maps to a TS function | https://github.com/LostCityRS/Engine-TS | `225` | `e1dea19f256c7ff1a89d47024c811c755ad2184d` |
| Content | game content (`.rs2` scripts, configs, maps) packed and served by the server | https://github.com/LostCityRS/Content | `225` | `9901aa27b60198afac49012f45f32e4eb4d5c012` |
| Client-Java | the client this server speaks to; wire-protocol cross-check | https://github.com/LostCityRS/Client-Java | `225-clean` | `cc3781de9e45265c52711dca850cd154f03c3a2c` |
| RuneScriptTS | RuneScript compiler reference for `pkg/script` + the pack pipeline (`@lostcityrs/runescript`; Engine-TS at the pin above depends on `^0.9.4`) | https://github.com/LostCityRS/RuneScriptTS | `main` | `750291cf59f55f64d8a9565d2607110b532dad94` |
| Engine | engine reference (Java) | https://github.com/LostCityRS/Engine | `main` | `5b5584280d910511ac5635e1025b9fd2912a8264` |
| Server | runnable meta-repo whose `engine/` checkout is Engine-TS at the pinned commit; the TS-packed-cache **byte-parity baseline** for the pack pipeline | https://github.com/LostCityRS/Server | `main` | `326bb4a3b24fbf7a1bf503ec598a4c2cab118ee1` |

(Commits captured 2026-06-03 from the goscape-client `REFERENCES.md` 225 pins
plus the local reference checkouts. The local working copies have since moved
to 244 branches — the pins above are what the rev-225 port corresponds to,
regardless of where those branches point now.)

Notes:

- The packer writes `jagFileVersion=26`; do **not** bump it to 27 unless the
  upstream Server meta-repo pins `@lostcityrs/runescript` past `750291c`
  (see `PORTING-LESSONS.md` §3, "Pack pipeline / byte parity").

## Future revisions

When porting revision *N*:

1. Add a `## rev-N` section below recording the reference commits used.
2. Branch the Go code `rev-N` from `rev-225` (or the nearest prior revision).
3. Diff the primary reference across the gap —
   `git -C Engine-TS diff e1dea19f..<rev-N commit>` — and apply the
   corresponding Go deltas on the `rev-N` branch, so the Go branch diff
   mirrors the TS revision diff.
4. Bump the **Content** and **RuneScriptTS** pins in the same section — the
   pack pipeline is byte-parity-checked against the cache the upstream
   meta-repo packs, so engine, content, and compiler move together.
```

- [ ] **Step 5: Create `PORTING-LESSONS.md` with this exact content**

```markdown
# Porting Lessons (TypeScript → Go, RuneScape server)

Cross-revision knowledge for porting a LostCityRS Engine-TS server revision
to Go. This is the durable, repo-owned distillation of what makes these ports
correct and what bites if you translate naively. Read it before starting a
new revision.

Companion files: `REFERENCES.md` (pinned upstream commits per revision), and
each revision branch's own `README.md` / `CLAUDE.md` / `PORTING.md` /
`docs/PORTING-CLOSED.md` (revision-specific conventions and the deviation
tracker).

---

## 1. Philosophy

**Faithful 1:1 translation is the default.** Every ported game-logic region
maps to an identifiable Engine-TS function, cited inline as
`// TS <File>.ts:<lines>`. goscape adapts the *infrastructure* (dskit service
lifecycle, module system, config layering) to Go idiom, but inside ported
game logic do **not** refactor opportunistically — behaviour bugs are found
by diff-checking the Go region against the cited TS source.

**Byte-faithful at the boundaries.** Wire protocol (ISAAC, RSA, opcodes),
codecs (bzip2, CRC, wordenc), collision, and the pack pipeline's cache output
are byte-for-byte against the reference. The pack pipeline is verified by
byte-diffing `script.dat` (and friends) against the cache the upstream
TS meta-repo packs.

**Deviations are tracked, never silent.** Each revision branch carries
`PORTING.md` (active backlog) + `docs/PORTING-CLOSED.md` (closed rows, parity
tables, audit history). Accepted in-code divergences carry a
`PORTING-EXCEPTION (<row-id>, <short>)` marker — `grep -rn "PORTING-EXCEPTION"
modules pkg cmd internal` lists every accepted exception (10 at the time of
the multi-revision conversion).

---

## 2. Porting workflow for a new revision

The Go port of revision N is (Go port of revision N-1) + (the translated TS
delta). That makes it a branch operation:

1. **Branch** `rev-N` from the nearest prior Go revision branch (e.g.
   `rev-225`).
2. **Diff the primary TS reference across the gap.** Look up the prior
   revision's pinned commit in `REFERENCES.md`, then
   `git -C Engine-TS diff <prev-commit>..<new-commit>`. That diff is your
   work list.
3. **Translate each TS change** into the corresponding Go region, applying
   the gotcha rules in §3. The Go branch diff (`git diff rev-(N-1) rev-N`)
   should correspond change-for-change to the TS diff — this is your audit.
4. **Record** the new reference commits in `REFERENCES.md` under `## rev-N` —
   including the Content and RuneScriptTS pins, which move together with the
   engine (the pack byte-parity baseline shifts with all three).
5. Each revision branch is self-contained (its own code, tooling, CI). Do
   **not** share code packages across revisions — independent faithful
   translations.
6. **Carry the tracker forward.** `PORTING.md` rows that still apply travel
   with the branch; close rows using the closure shapes defined in
   `PORTING.md` §Tracking conventions (FIXED / EXCEPTION-DOCUMENTED /
   NO-DIVERGENCE / NOT-A-GAP).

---

## 3. TS → Go translation gotchas

Each is a real bug class hit during the rev-225 port. Verify against the TS
source before "fixing" anything that merely looks wrong — and before
*preserving* anything that merely looks intentional.

### Identity & operand encoding

- **Pick ONE identity per subsystem.** Player identity exists as both `uid`
  and protocol `slot`; mixing them inside one subsystem caused repeated bugs
  (e.g. an obj-ownership check keyed by `uid` everywhere except one network
  handler that used `slot`). Convention: the network layer speaks `slot`,
  world/script logic speaks `uid`.
- **Script int-operand encoding is bit-packed.** The VARP/VARN secondary-
  player flag is **bit 16** of the int operand (`(intOperand>>16)&1`), not a
  0/1 selector. `.`-prefixed (secondary-player) script commands must use
  operand-aware accessors (`activePlayer()`), not `Self`, or writes silently
  hit the wrong player.

### Shared-handler / fork drift

- **When N opcodes share one TS handler, Go shares one implementation.**
  Forking per-opcode drifts (three move opcodes → one TS `MoveClickHandler`;
  the Go fork forgot modal-close in one copy). Parameterize the wire delta,
  not the behaviour.
- **When TS keeps logic in a shared base class (`PathingEntity`), a fix must
  land in BOTH Go forks** (Player and Npc) — Go has no inheritance, so the
  shared TS line exists twice.
- **Two parallel compute paths can feed the same wire field.** Fix the path
  that actually serializes (the renderer read accessors, not the bridge-fed
  struct field) — the spawn-orientation bug was "fixed" twice on the wrong
  path first.

### Tick order & scheduling

- **Tick ORDER is behaviour.** TS interleaves movement between pre-step and
  post-step interaction passes; running "movement, then interactions" broke
  walk-up combat. Port the pipeline order, not just the pieces.
- **`Player.Save()` is tick-goroutine-only.** Off-tick paths (disconnect,
  shutdown) must route through the relay action queue so saves run on-tick;
  three separate save-loss paths came from violating this.
- **The protected-access invariant is load-bearing for content.** All ~1159
  `.rs2` content scripts consume items *after* a dialogue yield without
  checking `inv_del`'s return. That is safe ONLY because `runScript` refuses
  to start a protected script while another holds protected access (TS
  `Player.ts:2094`). Immediate-run handlers (the opheld family, if_button /
  inv_button) bypass `CanAccess()` and rely solely on that guard; script
  resumes bypass `runScript` entirely (the `force=true` path). Removing or
  weakening the guard re-opens live item-dupe exploits.

### Trust nothing that isn't the TS source

- **A test can pin a bug.** A "deviation" test that contradicts TS may be
  enshrining the bug it was written around — verify against TS before
  preserving the pinned contract, and update the test when TS disagrees.
- **"By design" / "handled elsewhere" comments lie.** A comment claiming a
  divergence is intentional (or covered by another path) is the prime
  suspect, not evidence. Re-verify against TS before trusting it.
- **Ask "what does TS do?" FIRST.** Before estimating any gap as
  implementation work, check the TS state: if TS itself has the feature
  stubbed or commented out (NpcMode QUEUE1..20), the correct Go closure is
  documentation, not implementation.
- **Mis-described audit rows are themselves misdirection.** Severity or
  category in a finding can be wrong; re-derive the divergence claim from
  first principles against current TS + Go before acting on it.

### Pack pipeline / byte parity

- **Fix determinism FIRST, then byte-diff.** A "non-deterministic output"
  complaint against an external baseline can hide a multi-bug stack: one
  map-iteration-order bug masked four separate parity bugs that only became
  visible once output was stable. Go map iteration is randomized — any
  ordered output derived from a map needs explicit ordering.
- **Mirror TS's data-driven dispatch.** Discriminating types via Go pointer
  equality (instead of the data-driven check TS does) is fragile and broke
  default-value emission.
- **`jagFileVersion=26`.** Do not bump to 27 unless the upstream meta-repo
  pins `@lostcityrs/runescript` past `750291c` (see `REFERENCES.md`).

### Process

- **Capture `go test`'s real exit code** — and stress-run any fix that
  changes RNG-dependent behaviour. A flaky test passing once by RNG luck
  masked a real regression.
- **Get a live client debug log EARLY** when headless repros keep passing —
  one tick-ordering bug was invisible to every synthetic repro and obvious in
  the first live log.

---

## 4. Comment & reference conventions

- **Cite TS by file and line:** `// TS World.ts:128-129` next to the ported
  region. The file/symbol is the durable anchor; line numbers drift — fix
  them when touching the code, don't invest in keeping them precise.
- **No per-comment revision tags.** The branch *is* the revision context, and
  `REFERENCES.md` pins the exact TS commit — together they make every bare
  `// TS:` comment unambiguous.
- **`PORTING-EXCEPTION (<row-id>, <short>)`** one-line markers (with a `See
  PORTING.md` cross-reference) index every accepted divergence in code. Keep
  them grep-discoverable; each keeps its row-id reference.
- **`PORTING.md` is updated as a side effect** of any work that touches a
  tracked region; closed rows move to `docs/PORTING-CLOSED.md`.

---

## 5. Verification

- **Gates:** `CGO_ENABLED=0 go build -trimpath ./...`, `go vet ./...`,
  `go test ./...`, and `go test -race` on touched packages (the race detector
  needs `CGO_ENABLED=1`, the default). The world tick loop is goroutine-heavy;
  race coverage matters.
- **Byte parity:** ISAAC, RSA, opcodes, bzip2, CRC, wordenc, and collision are
  byte-faithful and pinned by tests; pack output is byte-diffed against the
  upstream-packed cache when the pack pipeline changes.
- **Pin tests:** when a fix lands, pin the TS-correct contract with a test.
  When an existing test pinned the buggy contract, update the test — after
  verifying against TS (§3, "A test can pin a bug").
```

- [ ] **Step 6: Create `.gitignore` with this exact content**

```gitignore
# .gitignore for the goscape docs-hub main branch. Revision branches carry
# their own (larger) ignore files.

# IDEs / editors
.idea/
.vscode/
*.swp
*~

# Stray build/runtime output from revision-branch work
*.exe
dist/
data/
dataplayers/
public/
.cache/
.serena/

# Git worktrees
.worktrees/

# Claude Code: per-user local settings + personal session state. To SHARE
# team config, un-ignore specific paths, e.g.:
#   !.claude/settings.json
#   !.claude/commands/
.claude/
# Project MCP config can carry server URLs / tokens — ignore by default.
.mcp.json
# Private per its own header.
CLAUDE.local.md

# OS cruft
.DS_Store
Thumbs.db
```

- [ ] **Step 7: Stage and verify exactly what the hub commit contains**

```bash
git add README.md REFERENCES.md PORTING-LESSONS.md .gitignore
git status --porcelain
```

Expected output — exactly these 9 lines (order may vary):

```
A  .gitignore
A  PORTING-LESSONS.md
A  README.md
A  REFERENCES.md
D  build.sh
D  cmd/goscape/main.go
D  config.yaml
D  modules/asset/config.go
D  pkg/util/log/log.go
```

(`config.yaml` is `D` here because the *initial-commit* version is being
deleted from `main`; the current config lives on `rev-225` untouched.)

- [ ] **Step 8: Commit the hub**

```bash
git commit --no-gpg-sign -m "Cross-revision docs hub

main now carries no code — one branch per game revision (rev-225 holds
the full history). README explains the branch model; REFERENCES.md pins
the upstream commits each revision was ported from; PORTING-LESSONS.md
distills cross-revision TS->Go porting knowledge.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 9: Verify the hub tree is exactly 4 files**

```bash
git ls-tree -r --name-only main
```

Expected output — exactly:

```
.gitignore
PORTING-LESSONS.md
README.md
REFERENCES.md
```

### Task 5: End state + full verification

**Files:** none.

- [ ] **Step 1: Return to the active work branch**

```bash
git checkout rev-225
```

Expected: `Switched to branch 'rev-225'`; the full worktree (code, docs, this plan) is back.

- [ ] **Step 2: Verify branch topology**

```bash
git merge-base main rev-225
```

Expected: `6579371cec7baa180461d30dd939422e24ce544c` (the initial commit — connected history, mirroring goscape-client where `merge-base main rev-225` = its initial commit).

- [ ] **Step 3: Verify rev-225 tip and clean status**

```bash
git rev-parse rev-225 && git status --porcelain
```

Expected: the `$PRE_SHA` recorded in Task 3 Step 1, then no status output.

- [ ] **Step 4: Build sanity (no code changed — this is a tripwire, not a test)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./...
```

Expected: exit 0, no output.

- [ ] **Step 5: Final survey**

```bash
git branch -v && git log --oneline -3 main && git log --oneline -3 rev-225
```

Expected: two branches — `main` at the "Cross-revision docs hub" commit (its parent = `6579371c Initial commit`), `rev-225` at the tidied tip (recent commits: the gitignore chore, the docs commit, the spec commit `ce5e59f1`).

---

## Self-review notes (kept for the executor)

- The spec's "exact hashes captured at implementation time" clause is resolved: every pin in Task 4 Step 4 was verified on 2026-06-03 against the goscape-client `REFERENCES.md` 225 section and the local checkouts (`Server225_2/engine` HEAD == `e1dea19f…` on branch `225`; RuneScriptTS `750291c` == full `750291cf59f55f64d8a9565d2607110b532dad94`; Engine-TS dep `@lostcityrs/runescript ^0.9.4`).
- No remote exists (`git remote -v` is empty) — there is deliberately no push step.
- Out of scope per spec: no `rev-244` branch, no CLAUDE.md branch-model note on `rev-225`, no changes to revision-specific docs beyond the two tidy commits.
