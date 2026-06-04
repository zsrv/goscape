# Multi-revision repo conversion — design

**Date:** 2026-06-03
**Status:** Approved (Approach A — exact goscape-client replay)

## Goal

Convert goscape from a single-`main` repo into the multi-revision branch model
already established in goscape-client (2026-05-23):

- `main` — slim **cross-revision docs hub** (no code)
- `rev-225` — the full existing code history (what `main` is today)
- future `rev-N` — cut from the nearest prior revision branch when porting starts

Scope is **restructure only**: no `rev-244` branch, no 244 porting, no
remote/publishing setup.

## Why this shape

goscape-client proved the model: `merge-base main rev-225` = the repo's initial
commit (connected history, not orphan), `REFERENCES.md` pins upstream reference
commits per revision like a lockfile, and each revision branch is fully
self-contained (no shared code packages across revisions). The "Future
revisions" recipe — branch `rev-N` from the nearest prior revision, diff the
upstream reference across the gap, make the Go branch diff mirror the upstream
diff — transfers verbatim to the server side.

**No history rewrite.** All 3385 existing commits and their SHAs stay exactly
as-is. This matters post-scrub (2026-06-03): nothing gets re-pointed, and every
SHA recorded in PORTING docs/audit ledgers stays valid.

## Steps

### 1. Pre-conversion tidy (lands on the rev-225 lineage)

On current `main`, before any branch surgery:

1. **Docs commit** — add the untracked durable docs:
   - `RUNESCRIPT.md`
   - `docs/superpowers/handoffs/2026-05-23-ts-parity-full-audit-resume.md`
   - `docs/superpowers/plans/2026-05-23-pack-determinism.md`
2. **.gitignore commit** — add `public/` (soundfont binary), `.claude/`,
   `.mcp.json`, `.serena/` (matching goscape-client's ignore conventions).

All commits use `--no-gpg-sign`.

### 2. Branch surgery (no history rewrite)

```bash
git branch rev-225 main          # rev-225 = full history, tip = tidied main
git checkout rev-225
git branch -f main 6579371c      # re-point main at Initial commit
git checkout main                # build hub commit here
# ... hub commit (step 3) ...
git checkout rev-225             # end on the active work branch
```

No remote exists; nothing to push.

### 3. Hub commit — "Cross-revision docs hub" on `main`

One commit that deletes the 5 initial-commit code files (`build.sh`,
`cmd/goscape/main.go`, `config.yaml`, `modules/asset/config.go`,
`pkg/util/log/log.go`) and leaves **exactly 4 files** (same tree shape as
goscape-client `main`):

| File | Content |
|---|---|
| `README.md` | Proper hub README (improving on the client's leftover stub): what goscape is, the branch model, where to find what |
| `REFERENCES.md` | Same "commit hash is the real pin / lockfile" framing as the client's. `## rev-225` table pinning the upstream references the port was built against: **Engine-TS `e1dea19f` (primary)**, plus Client-Java, Content, Engine (Java) / Server pins — exact hashes captured at implementation time from local reference checkouts, cross-checked against goscape-client's REFERENCES.md where shared. Ends with the "Future revisions" recipe adapted for the server |
| `PORTING-LESSONS.md` | Distilled **server-side** TS→Go cross-revision lessons, structured like the client's (§1 Philosophy: faithful 1:1 TS→Go; §2 Porting workflow for a new revision; §3 Gotchas). Sources: PORTING.md conventions, `docs/PORTING-CLOSED.md`, audit ledgers under `docs/superpowers/audits/`, and the project's durable lessons — identity UID-vs-slot, TS-unified-handler/Go-fork drift, tests-that-pin-bugs, "by-design comment is the prime suspect", tick-ordering parity, fix-the-path-that-serializes, the `PORTING-EXCEPTION` marker convention, byte-diff workflow, "what does TS do?" before estimating any gap |
| `.gitignore` | Full ignore file modeled on the client hub's |

### 4. What stays on `rev-225`

Everything. No deletions: `PORTING.md`, `docs/PORTING-CLOSED.md`,
`docs/superpowers/`, `CLAUDE.md`, `README.md`, all code. Mirroring the client,
`rev-225`'s CLAUDE.md gets **no** branch-model note.

## Verification

- `git merge-base main rev-225` == `6579371c`
- `git ls-tree main` lists exactly the 4 hub files
- `rev-225` tip == pre-surgery `main` tip SHA (recorded before starting)
- `git status` clean on both branches
- `go build ./...` still green on `rev-225` (sanity only — no code is touched)

## Out of scope

- `rev-244` branch creation and any 244 porting work
- Remote/publishing setup
- Changes to `rev-225` code or revision-specific docs beyond the tidy commits

## Addendum (2026-06-03, post-ship): empty shared root

After the conversion shipped, the user requested that `main`'s *history*
contain no code at all — while staying **connected** to `rev-225`, exactly
like goscape-client (whose root commit `cabab8f` is empty; that is the only
reason its hub history is code-free *and* connected). goscape's original root
`6579371c` carried 5 code files, so the client shape required a history
rewrite, which the user explicitly authorized with the constraint that
commit timestamps be preserved.

What was done (superseding this spec's "no history rewrite" property and an
intermediate orphan-`main` attempt):

1. A new **empty root commit** `2f24acf0` ("Initial commit", author/committer
   dates copied from the original `6579371c`) was created.
2. All of `rev-225` was re-parented onto it (`git replace --graft` +
   `git filter-branch`): every commit's tree, author/committer identity,
   author/committer timestamp, and message verified byte-identical
   pre/post-rewrite; only parent links (and therefore SHAs) changed.
   New `rev-225` tip: `78059d83` (was `0669177e`).
3. `main` was rebuilt as `2f24acf0` → hub commit `75888720` (byte-identical
   4-file tree and message of the original hub commit `aef80ce6`, original
   timestamps).

Resulting invariants:

- `git merge-base main rev-225` == `2f24acf0` (connected, mirrors the client)
- Every file ever touched in `main`'s history is one of the 4 hub docs
- **All pre-rewrite SHAs are stale** (including those cited in PORTING docs
  and audit ledgers); match commits by author-date + subject instead.
  Pre-rewrite tips are kept at `refs/backup/pre-empty-root-rev-225`
  (`0669177e`) and `refs/backup/pre-empty-root-main` (`bd227722`); delete
  those refs + `git gc` to drop the old history entirely.

Post-addendum follow-ups (same day): the backup refs above were deleted and
the old history purged (`reflog expire` + `gc --prune=now`); in-repo doc
SHA citations were remapped to current history (resolved via the pre-split
monorepo / prescrub bundles in `/home/owner`, matched by author-date +
subject); and the second commit — the rewritten original "Initial commit"
(5-file skeleton) — was reworded to **"Bootstrap server skeleton"** so only
the empty shared root bears the name "Initial commit" (one more re-parent
of all descendants, timestamps preserved, `main` and the root unchanged;
citations positionally re-remapped).
