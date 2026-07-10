# Session Backport Implementation Plan (4 branches × 3 repos)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the 2026-07-09 session's rev-274 work (wordenc knob, PERF-3 buffer pool, doc fixes, client ExitFunc+launch seams, singleplayer branches) on rev-254, rev-245.2, rev-244, and rev-225.

**Architecture:** Branch-major waves (254 → 245.2 → 244 → 225), three tasks per wave: goscape port → goscape-client port → singleplayer branch. Ports follow the cross-rev methodology: COPYABLE files come via `git checkout rev-274 -- <file>` after a byte-identity guard; everything else is same-anchor adaptation with the rev-274 commits (visible in every worktree via `git show`) as the source of truth. Spec: `docs/superpowers/specs/2026-07-09-session-backport-design.md`.

**Tech Stack:** Go 1.26, existing per-rev worktrees (verified present + clean): goscape at `../goscape-rev{254,245.2,244,225}`, client at `../goscape-client-rev{254,245.2,244,225}`, singleplayer at `/home/owner/Code/github.com/zsrv/goscape-singleplayer`.

## Global Constraints

- Source-of-truth rev-274 commits: wordenc `eee67f039`+`fdef314f1`; PERF-3 `0c55ea7ca` (pkg/script) + `4840bdea1` (call sites) + `7754eb66f` (hardened pin); docs `861b6fb57`+`92c8f0cae`+`af2cd97e4`; client `6e4d708` (ExitFunc) + `71dcce3` (launch). Read them with `git show <sha>` inside any worktree of the same repo — port their content faithfully, adapting ONLY what the task's parameter table names.
- `go` commands: prefix env `GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache"`. goscape builds CGO=0; `-race` runs and all goscape-client / singleplayer builds need `CGO_ENABLED=1`.
- Shell writes to any worktree outside the main goscape checkout fail under the sandbox ("Read-only file system") — retry the exact command with the sandbox disabled.
- Commits: `git commit --no-gpg-sign`; `git status --short` first; stage only files the task names. Never stage `.superpowers/`, stray binaries, or phantom `/dev/null` dotfiles (sandbox mask-mounts).
- Every port preserves the branch's stock default behavior. Adaptation lists are exhaustive — anything not listed ports verbatim. If a byte-identity guard or anchor is missing (file differs where the plan says it matches), STOP and report NEEDS_CONTEXT with the actual diff; do not improvise.
- Locate PERF-3 Release insertion points by the `case script.Finished, script.Aborted:` arms, never by line number.
- Do not touch rev-274 (`../goscape`, `../goscape-client`, singleplayer `rev-274`) or `main` branches anywhere.

---

## Procedure P-GOSCAPE (goscape port; used by Tasks 1, 4, 7; Task 10 is the bespoke rev-225 variant)

Parameters per task: ⟨WT⟩ = worktree dir, ⟨BR⟩ = branch.

- [ ] **Step 1: Preconditions** — `cd ⟨WT⟩`; `git branch --show-current` = ⟨BR⟩; `git status --short` clean (ignore `.superpowers/`, phantom dotfiles).

- [ ] **Step 2: wordenc knob — copy the byte-identical core**

Guard, then copy (post-change files from rev-274):
```bash
git diff eee67f039^ ⟨BR⟩ -- pkg/wordenc/encfilter/encfilter.go pkg/wordenc/encfilter/encfilter_test.go
# Expected: EMPTY (survey-verified byte-identical). If not empty → STOP, NEEDS_CONTEXT.
git checkout rev-274 -- pkg/wordenc/encfilter/encfilter.go pkg/wordenc/encfilter/encfilter_test.go
```

- [ ] **Step 3: wordenc knob — same-anchor insertions**

Read `git show eee67f039` and apply its `modules/world/config.go`, `modules/world/server.go`, `modules/world/server_wordenc_test.go`, `modules/world/server_lifecycle_test.go`, and `examples/full-config-reference.yaml` hunks to this branch at the SAME anchors (the `RSAPrivateKeyPath` field / `world.rsa-private-key-path` flag / `rsa_private_key_path` yaml key / the `encfilter.Load()` call + its comment). Insertion content is verbatim from the commit; surrounding branch content stays untouched. Branch-specific test note: this branch's `server_wordenc_test.go` uses its own `refNNNCacheDir`-style fixtures — put the added `WordEncPath: filepath.Join("data", "raw", "wordenc")` lines into the branch's existing cfg literals, mirroring what `eee67f039` did on 274. Also apply `fdef314f1` (comment-accuracy follow-up) where its hunks match.

- [ ] **Step 4: wordenc gate**

```bash
GOPATH=... go test ./pkg/wordenc/... ./modules/world/ -run 'Wordenc|WordEnc|TestLoad' -v && GOPATH=... go build ./...
```
Expected: PASS (the `TestNewServer*` wordenc tests may SKIP without `GOSCAPE_REF*_DIR` — pre-existing gate, fine). Commit 1: `git add` the seven wordenc files → `feat(world): configurable wordenc_path for embedders (default TS-faithful) [port of eee67f039+fdef314f1]`.

- [ ] **Step 5: PERF-3 — copy pkg/script**

```bash
git diff 0c55ea7ca^ ⟨BR⟩ -- pkg/script/runner.go pkg/script/state.go
```
If EMPTY for a file → `git checkout rev-274 -- <that file>`. If runner.go/state.go differ from 274's pre-change state (survey says Init is byte-identical but whole files were not certified): apply `0c55ea7ca`'s hunks manually — the Init buffer-source swap and the `buf *scriptBuffers` field appended as the last `ScriptState` field with its doc comment, both verbatim from `git show 0c55ea7ca`. Then always:
```bash
git checkout rev-274 -- pkg/script/pool.go pkg/script/pool_test.go
```
(brings the pool and the `7754eb66f`-hardened tests wholesale — both files are new/self-contained).

- [ ] **Step 6: PERF-3 — the three Release call sites**

In `modules/world/script.go` (`resumeOrFinish`, `resumeOrFinishWorld`) and `modules/world/npc_script.go` (`resumeOrFinishNpc`): insert `script.Release(state)` as the LAST statement of each `case script.Finished, script.Aborted:` arm, with the exact comments from `git show 4840bdea1` (after `OnScriptFinishedOrAborted(state)` in the player/NPC dispatchers; directly in the comment-only arm in resumeOrFinishWorld). NO suspend arm, NO default arm, NO pre-switch error path. Then:
```bash
git checkout rev-274 -- modules/world/script_pool_release_test.go
```
and verify its fixture dependencies exist on this branch (`newTestServer`, `newReturnImmediatelyScript`, the player/NPC helpers it uses — survey says the queue fixtures are identical; if a helper is missing/renamed, adapt the test to this branch's neighboring-test idiom and note it in your report).

- [ ] **Step 7: PERF-3 gate + benchmark**

```bash
GOPATH=... go test ./pkg/script/ -v -count=1 && \
GOPATH=... go test ./pkg/script/... ./modules/world/... && \
GOPATH=... CGO_ENABLED=1 go test -race ./pkg/script/ ./modules/world/ && \
GOPATH=... go test -run '^$' ./... && gofmt -l pkg/script modules/world
GOPATH=... go test ./pkg/script/ -run '^$' -bench BenchmarkInitRelease -benchtime 10000x
```
Expected: all PASS / clean compile-all / no gofmt output; record the B/op (⟨BENCH⟩, expect ≤~1000). A FAILURE in any existing world test is real information — STOP and report; never weaken a test or move a Release. Commit 2: pkg/script files + the two modules/world files + the test → `perf(script,world): ScriptState buffer pool + terminal-arm Release (PERF-3) [port of 0c55ea7ca+4840bdea1+7754eb66f]`.

- [ ] **Step 8: docs**

In `docs/PORTING-CLOSED.md` §Performance hotspots: (a) append the PERF-3 row — take the FINAL rev-274 row text (`git show af2cd97e4:docs/PORTING-CLOSED.md | grep -A1 'PERF-3'` or read the file on rev-274), then adapt exactly two things: replace the commit SHAs with THIS branch's two port commits, and replace `833 B/op` with ⟨BENCH⟩; append a trailing sentence: `Ported from rev-274 (soak churn numbers measured there; same engine).` (b) replace the stale `⚠ MED` lineroutefinder row with rev-274's final ✅ FIXED version (from `af2cd97e4`) verbatim — survey verified all four branches have `pkg/pathfinder/routefinder/lineroutefinder.go` with `lineRouteCoordsCap = 64` at line 21, so the text is accurate as-is. In `docs/PORTING.md`: replace the "(none — both LOW rows closed…)" pointer line with rev-274's final version (mentions PERF-1/2/3; from `git show 92c8f0cae^:docs/PORTING.md` — the line as updated by `861b6fb57`). Commit 3: both docs → `docs(porting): PERF-3 closure row + lineroutefinder row flip [port]`.

- [ ] **Step 9: Report** — branch tip SHA, ⟨BENCH⟩, gate outputs, any fixture adaptations.

## Procedure P-CLIENT (goscape-client port; Tasks 2, 5, 8, 11)

Parameters: ⟨WT⟩, ⟨BR⟩, plus the per-task ADAPT table.

- [ ] **Step 1: Preconditions** — `cd ⟨WT⟩`; branch = ⟨BR⟩; clean tree.

- [ ] **Step 2: ExitFunc hook (port of `6e4d708`)**

Apply to this branch (same three-file shape everywhere; do NOT wholesale-checkout `clientextras.go` — its data tables are rev-specific):
1. `pkg/jagex2/client/clientextras/clientextras.go`: add `import "os"` (file currently has no import block) + append the `ExitFunc` var with its full doc comment, verbatim from `git show 6e4d708`.
2. `pkg/jagex2/client/gameshell.go`: swap the `Shutdown` tail `os.Exit(0)` → `clientextras.ExitFunc(0)` with the comment update from `6e4d708`; imports: drop `"os"` (its only use), add the clientextras import.
3. Copy the test: `git checkout rev-274 -- pkg/jagex2/client/clientextras/clientextras_test.go`.

Gate: `GOPATH=... CGO_ENABLED=1 go test ./pkg/jagex2/client/clientextras/ -v && go build ./...` → PASS. Commit 1: three files → `feat(clientextras): ExitFunc hook so embedders can intercept process exit [port of 6e4d708]`.

- [ ] **Step 3: launch extraction (port of `71dcce3`, applied to THIS branch's wiring)**

This is an extraction of the BRANCH'S OWN `cmd/client/main.go`, not a copy of 274's `launch.go`: create `pkg/jagex2/launch/launch.go` with the same `Options`/`configure`/`Run` structure as `git show 71dcce3:pkg/jagex2/launch/launch.go`, but every moved statement and comment comes verbatim from this branch's current `main.go` (banner expression, title/dimensions, subsystem wiring, comment blocks with their Java refs). Apply the task's ADAPT table (below) — it is the complete list of structural differences. Rewrite `cmd/client/main.go` as flags → Options → `launch.Run(opts)` following `71dcce3`'s main.go shape, keeping this branch's flag set and validation messages; `parseWorldServer`/`parseOndemandServer` stay untouched. Copy the test then adapt if the ADAPT table says so: `git checkout rev-274 -- pkg/jagex2/launch/launch_test.go`.

- [ ] **Step 4: Gate**

```bash
GOPATH=... CGO_ENABLED=1 go build ./... && GOPATH=... CGO_ENABLED=1 go test ./... && gofmt -l pkg/jagex2/launch cmd/client
GOPATH=... CGO_ENABLED=1 go run ./cmd/client -version
GOPATH=... CGO_ENABLED=1 go run ./cmd/client -mem bogus; echo "exit=$?"
```
Expected: build/tests/gofmt clean; `-version` prints build info exit 0; `-mem bogus` prints this branch's error text, exit 1 (banner no longer precedes it — same accepted deviation as 274). Commit 2: launch files + main.go → `refactor(launch): extract embeddable client startup from cmd/client [port of 71dcce3]`.

- [ ] **Step 5: Report** — tip SHA, gate outputs, confirmation each ADAPT item landed.

## Procedure P-SP (singleplayer branch; Tasks 3, 6, 9, 12)

Parameters: ⟨BR⟩ (new branch name = rev name), ⟨GWT⟩/⟨CWT⟩ (sibling worktree names), plus per-task ADAPT table.

- [ ] **Step 1: Create the branch from rev-274**

```bash
cd /home/owner/Code/github.com/zsrv/goscape-singleplayer
git status --short   # must be clean apart from untracked profiles/
git checkout -b ⟨BR⟩ rev-274
```

- [ ] **Step 2: Repoint the wiring**

`go.mod` replace block → `../⟨GWT⟩` and `../⟨CWT⟩`. `internal/server/server_test.go` cacheDir default → `"../../../⟨GWT⟩/data/pack"`. README "Usage" section: retitle to this rev, and state the pairing (`replace` targets are the ⟨GWT⟩/⟨CWT⟩ worktrees; no branch-switching needed). Apply the task's ADAPT table (complete list of code differences).

- [ ] **Step 3: Tidy + gates**

```bash
GOPATH=... go mod tidy && GOPATH=... CGO_ENABLED=1 go build ./... && GOPATH=... go vet ./... && \
GOPATH=... go test ./... -v -count=1 && gofmt -l cmd internal
GOPATH=... CGO_ENABLED=1 go build -o goscape-singleplayer ./cmd/goscape-singleplayer && ./goscape-singleplayer --help; ./goscape-singleplayer --cache-dir /nonexistent; echo "exit=$?"; rm goscape-singleplayer
```
Expected: tidy rewrites requires against the worktree paths; build/vet/gofmt clean; config tests PASS; **boot test SKIPS** (no `data/pack` in ⟨GWT⟩ — report the skip honestly, never claim boot-proven); `--help` lists this branch's flags; bad cache-dir fails fast exit 1, no ports bound.

- [ ] **Step 4: Commit + report**

One commit: `feat: ⟨BR⟩ singleplayer branch — paired to ⟨GWT⟩/⟨CWT⟩`. Report: tip SHA, gate outputs, explicit "boot test SKIPPED (no per-rev cache)".

---

### Task 1: goscape rev-254 — P-GOSCAPE with ⟨WT⟩=`/home/owner/Code/github.com/zsrv/goscape-rev254`, ⟨BR⟩=`rev-254`
No extra adaptations (closest branch to 274).

### Task 2: goscape-client rev-254 — P-CLIENT with ⟨WT⟩=`/home/owner/Code/github.com/zsrv/goscape-client-rev254`, ⟨BR⟩=`rev-254`
**ADAPT table:** (1) `platform.Main` title argument stays this branch's `"RS2 user client - release #"+strconv.Itoa(signlink.ClientVersion)` (274 uses `"Jagex"` — do NOT copy that); dimensions 765,503 unchanged. Nothing else differs; Options has all 8 fields; signlink import is `pkg/sign/signlink`.

### Task 3: singleplayer rev-254 — P-SP with ⟨BR⟩=`rev-254`, ⟨GWT⟩=`goscape-rev254`, ⟨CWT⟩=`goscape-client-rev254`
**ADAPT table:** none beyond P-SP Step 2 (internals identical — the wordenc knob and full launch.Options exist on the siblings after Tasks 1-2).

### Task 4: goscape rev-245.2 — P-GOSCAPE with ⟨WT⟩=`…/goscape-rev245.2`, ⟨BR⟩=`rev-245.2`
No extra adaptations.

### Task 5: goscape-client rev-245.2 — P-CLIENT with ⟨WT⟩=`…/goscape-client-rev245.2`, ⟨BR⟩=`rev-245.2`
**ADAPT table:** identical to Task 2's (versioned title; everything else standard).

### Task 6: singleplayer rev-245.2 — P-SP with ⟨BR⟩=`rev-245.2`, ⟨GWT⟩=`goscape-rev245.2`, ⟨CWT⟩=`goscape-client-rev245.2`
**ADAPT table:** none.

### Task 7: goscape rev-244 — P-GOSCAPE with ⟨WT⟩=`…/goscape-rev244`, ⟨BR⟩=`rev-244`
No extra adaptations.

### Task 8: goscape-client rev-244 — P-CLIENT with ⟨WT⟩=`…/goscape-client-rev244`, ⟨BR⟩=`rev-244`
**ADAPT table:** (1) banner keeps this branch's literal `strconv.Itoa(244)` (not `signlink.ClientVersion`); (2) ALL signlink imports in the new launch.go and main.go use this branch's nested path `github.com/zsrv/goscape-client/pkg/jagex2/client/sign/signlink`; (3) title is `"Jagex"`, 765,503 (matches 274).

### Task 9: singleplayer rev-244 — P-SP with ⟨BR⟩=`rev-244`, ⟨GWT⟩=`goscape-rev244`, ⟨CWT⟩=`goscape-client-rev244`
**ADAPT table:** none.

### Task 10: goscape rev-225 — bespoke variant of P-GOSCAPE, ⟨WT⟩=`…/goscape-rev225`, ⟨BR⟩=`rev-225`

Differences from P-GOSCAPE:
- **SKIP Steps 2-4 (wordenc knob) entirely** — rev-225's `encfilter.Load(cachePath)` already reads `<cachePath>/client/wordenc`, cwd-independent (spec §matrix). Do not touch encfilter or the world config.
- Steps 5-7 (PERF-3) as written; resumeOrFinishWorld's terminal arm sits near `modules/world/script.go:262` on this branch.
- **NEW Step 6b: `ondemand.cache_path` knob (Go-original, default-preserving).** In `modules/ondemand/config.go`: add field `CachePath string \`yaml:"cache_path"\`` (place before `PublicDir`) with doc comment: "CachePath is the directory containing the packed client cache tree (`client/…` jags + `client/songs/*.mid` + `client/maps/*`). Rev-225 serves these as static files; the default preserves the historical hardcoded `data/pack` relative path (resolved against the process working directory). Go-original embedding knob — same pattern as `world.rsa_private_key_path`."; register flag `f.StringVar(&c.CachePath, "ondemand.cache-path", "./data/pack", "Cache root; archive and song/map files are served from <path>/client/.")` next to the PublicDir flag. In `modules/ondemand/handler.go`: replace every one of the ten `path.Join("data/pack/client…", …)` / `"data/pack/client"` literals (lines 82,96,102,108,114,120,126,132,138,153) with `path.Join(cfg-reachable CachePath, "client", …)` preserving each route's leaf exactly — thread the config the same way the handler already receives `PublicDir` (follow the existing wiring; if handlers are methods on the OnDemand struct, use its cfg field). Add/adjust the branch's ondemand handler tests if any pin the literal paths (adapt them to a cfg with CachePath `"data/pack"` so expectations are unchanged). Add `cache_path` to this branch's `examples/full-config-reference.yaml` ondemand section at its default. Gate: `go test ./modules/ondemand/... && go build ./...`. Separate commit: `feat(ondemand): configurable cache_path for embedders (default preserves data/pack) [rev-225 analogue of the wordenc knob]`.
- Step 8 (docs) as written, plus in the PERF-3 row's trailing sentence also note `rev-225: wordenc knob N/A (cache-path era)`.

### Task 11: goscape-client rev-225 — P-CLIENT with ⟨WT⟩=`…/goscape-client-rev225`, ⟨BR⟩=`rev-225`

**ADAPT table:** (1) **Omit `StoreID` from `Options`** and omit the `signlink.StoreID` assignment from `configure()` — this branch has no `-store-id` flag and never sets StoreID (state this in a launch.go comment: "rev-225 predates the -store-id flag; StoreID is not part of this branch's Options"); (2) banner keeps the literal `strconv.Itoa(225)`; (3) `platform.Main(789, 532, "Jagex", …)` — this branch's dimensions; (4) `launch_test.go`: drop the StoreID field from the Options literal and the `signlink.StoreID` save/restore + assertion.

### Task 12: singleplayer rev-225 — P-SP with ⟨BR⟩=`rev-225`, ⟨GWT⟩=`goscape-rev225`, ⟨CWT⟩=`goscape-client-rev225`

**ADAPT table (complete):**
1. `internal/server/config.go`: delete `Options.WordEncPath`, the wordenc derivation/absolutization in `NewConfig`, the `cfg.World.WordEncPath` assignment, and `CheckWordEnc` (rev-225 loads wordenc from inside the cache). Keep `cfg.OnDemand.CachePath = cacheDir` (the Task 10 knob provides the field). Change `CheckCache` to probe the split layout: `os.Stat(filepath.Join(cacheDir, "client", "config"))`, message: "no packed rev-225 game cache at %s (client/config missing): point --cache-dir at a rev-225 split-layout pack (data/pack with client/ + server/)".
2. `internal/server/config_test.go`: remove the three wordenc tests; adapt `TestCheckCache` to create `client/config` inside the temp dir; drop wordenc assertions from `TestNewConfigWiresLoopbackStack`.
3. `internal/server/server_test.go`: cacheDir → `"../../../goscape-rev225/data/pack"`; the boot test's `CheckCache` now probes the split layout — **it will RUN if the rev-225 worktree's split pack (`data/pack/client/config`) is present** (survey says it is). Report RAN/SKIPPED honestly; a boot failure is real information — STOP and report, don't patch around.
4. `cmd/goscape-singleplayer/main.go`: remove the `--wordenc-path` flag, its Options passthrough, and the `CheckWordEnc` call; remove `StoreID: 32` from the `launch.Options` literal (the rev-225 launch package has no such field); update the cache-dir flag help to mention the split layout.
5. `README.md`: rev-225 usage — no wordenc file needed (baked into the cache), split-layout cache note, pairing with the rev-225 worktrees.

### Task 13: Final whole-project review

One reviewer over all twelve branch tips (review packages per repo per branch), checking: adaptation tables fully applied, no rev-274-isms leaked into branch content (wrong banner/title/paths), release-arm placement per branch, docs accuracy per branch, and the per-task Minor lists. Then the ledger/memory close-out.
