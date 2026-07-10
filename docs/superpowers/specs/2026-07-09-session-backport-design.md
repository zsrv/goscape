# Session backport to rev-254 / rev-245.2 / rev-244 / rev-225 — Design

**Date:** 2026-07-09
**Status:** Approved
**Scope decisions (user):** include PERF-3 on all branches (explicit override of
the standing no-forward-port policy and of the same-day rev-274-only scoping —
precedent: the arch-review follow-up forward-port); build all four
goscape-singleplayer revision branches (not just the enabling seams).

## What is being backported

Everything landed on rev-274 during the 2026-07-09 session:

1. goscape `world.wordenc_path` knob (`eee67f039`+`fdef314f1`)
2. goscape PERF-3 ScriptState buffer pool (`0c55ea7ca`+`4840bdea1`) incl. the
   identity-hardened stale-leak pin (`7754eb66f`)
3. goscape PORTING doc fixes: per-branch PERF-3 closure row (as in
   `861b6fb57`+`92c8f0cae`) and the stale lineroutefinder row flip
   (`af2cd97e4`)
4. goscape-client `clientextras.ExitFunc` hook (`6e4d708`) and the
   `pkg/jagex2/launch` extraction (`71dcce3`)
5. goscape-singleplayer: a new revision branch per target rev, replicating the
   rev-274 internals (`internal/server`, `cmd/goscape-singleplayer`, README)

## Execution shape

Branch-major waves in order **rev-254 → rev-245.2 → rev-244 → rev-225**; per
branch three sequential tasks (goscape port → goscape-client port →
singleplayer branch), each with its own review gate. Work happens in the
existing per-rev worktrees (`../goscape-revNNN`, `../goscape-client-revNNN` —
all verified present, clean, on-branch). Squashed-per-repo commits per branch,
following the chat-kafka port convention.

## Per-branch matrix (from the four investigation surveys, 2026-07-09)

### goscape

| Item | rev-254 | rev-245.2 | rev-244 | rev-225 |
|---|---|---|---|---|
| wordenc knob | ADAPT-mechanical | ADAPT-mechanical | ADAPT-mechanical | **SKIP — N/A** |
| PERF-3 pool (pkg/script) | COPY verbatim | COPY | COPY | COPY |
| PERF-3 call sites | COPY (locate arms by `case Finished, Aborted:`) | COPY | COPY | COPY (resumeOrFinishWorld at ~script.go:262) |
| PERF-3 docs row | add, per-branch benchmark number | same | same | same |
| lineroutefinder row flip | verify branch's file path + pre-alloc first | same | same | same |
| ondemand.cache_path knob | — | — | — | **ADD (new, Go-original)** |

Key survey facts the plan relies on:
- `pkg/script` `Init`, `StackCapacity=1024`, `FrameCapacity=50`, the Execution
  enum, all three dispatchers, the exactly-three cross-tick storage points
  (Player/Npc `activeScript`, `worldScriptQueue`), the identity guards, and the
  SP-guarded pop discipline are **byte-identical (or safety-equivalent) on all
  four branches**. No fourth executor, no history/debug retention, no above-SP
  reads anywhere. PERF-3 is safe everywhere; no branch excluded.
- wordenc: rev-254/245.2/244 have rev-274's pre-change `encfilter.go`
  byte-identical (`Load()` hardcoding `data/raw/wordenc`; single prod caller
  `world.NewServer`; `data/raw/wordenc` committed, 13,310 B) and the same
  `rsa_private_key_path` anchors in `modules/world/config.go` +
  `examples/full-config-reference.yaml`. **rev-225 differs by design**: its
  `Load(cachePath)` reads `<cachePath>/client/wordenc` (pre-244 TS,
  `WordEnc.ts:37-44`), no `data/raw/wordenc` in tree — already
  cwd-independent given an absolute `world.cache_path`; the knob would be a
  redesign, not a port. Skip and document.
- rev-225 ondemand: no `cache_path` field; `modules/ondemand/handler.go`
  serves hardcoded cwd-relative `data/pack/client/*` paths. The new
  `ondemand.cache_path` knob (default `./data/pack`, stock behavior
  preserved) parameterizes those paths — same pattern and justification as
  the wordenc knob on 274. **Plan-time verification required:** where
  rev-225's CRC snapshot (`cache.MakeCRCs`) sources its path, and whether the
  world module has any other cwd-relative read at 225.
- The stale `⚠ MED` lineroutefinder row and the `PORTING.md` "(none — both
  LOW rows closed…)" pointer line exist identically on all four branches. The
  fix (`✅ FIXED`, NEW-F `ef7838743`) must be re-verified per branch: confirm
  the branch's actual lineroutefinder path (the `routefinder` package move
  may be 274-only) and the pre-alloc's presence before flipping; if a branch
  lacks the NEW-F fix, leave the row open there and note it instead.

### goscape-client

| Item | rev-254 | rev-245.2 | rev-244 | rev-225 |
|---|---|---|---|---|
| ExitFunc hook | COPY | COPY | COPY | COPY |
| launch extraction | ADAPT: window title stays `"RS2 user client - release #"+ver` | same as 254 | ADAPT: banner literal `244`; signlink import is `pkg/jagex2/client/sign/signlink` | ADAPT: **no StoreID** (field+flag+wiring omitted), banner literal `225`, `platform.Main(789, 532, "Jagex", …)` |

All branches: 7 flags (225: 6 — no `-store-id`), same `clientextras` globals
incl. `WSPath`/`TransportKind`, same `parseWorldServer`/`parseOndemandServer`,
`pkg/profiling` + audio present, no `pkg/jagex2/launch` collision, gameshell
`Shutdown` tail byte-identical. The hook's three-file shape is identical
everywhere; the loop-closure `os.Exit(0)`→`clientextras.ExitFunc(0)` lands in
the new `launch.go` as on 274.

### goscape-singleplayer (new branch per rev, from `main`)

- `go.mod` replaces point at the **per-rev worktrees**
  (`../goscape-revNNN`, `../goscape-client-revNNN`) — an improvement over
  rev-274's `../goscape` model; no sibling branch-shuffling. Boot test cache
  path repoints likewise (`../../../goscape-revNNN/data/pack`), overridable
  via `GOSCAPE_SP_TEST_CACHE`.
- rev-254/245.2/244: internals near-verbatim from rev-274 (the wordenc knob
  exists there after the goscape port; `launch.Options` identical).
- rev-225: config builder drops `Options.WordEncPath`/`CheckWordEnc` (wordenc
  ships inside the cache) and sets the new `OnDemand.CachePath`; `CheckCache`
  probes the split-layout pack (rev-225 has no `main_file_cache.dat` — probe
  `<cache>/client/config` or equivalent, fixed at plan time from the actual
  layout); `main` omits `--wordenc-path`; `launch.Options` omits `StoreID`.
- README per branch documents the worktree pairing and any rev-specific flags.

## Verification

- Per goscape branch: `go build ./...`, full `go test ./pkg/script/...
  ./modules/world/...` (+ wordenc packages where ported), `-race` (CGO=1) on
  pkg/script + modules/world, gofmt; PERF-3 pool tests + dispatcher tests
  green; `TestInitReleaseAllocBytes` gate; benchmark B/op recorded per branch
  for that branch's docs row.
- Per client branch: module build + full tests, headless `-version` and
  `-mem bogus` runs matching stock outputs (with the branch's banner/title).
- Per singleplayer branch: `CGO_ENABLED=1 go build ./...`, `go vet`, config
  tests green; boot smoke **will SKIP** — no packed cache exists for any
  target rev (`data/pack` absent from all rev worktrees; `data/src` inputs
  are rev-specific and absent). Branches land code-complete/compile-proven;
  boot- and windowed-smokes are deferred until a per-rev cache is provided
  (wire via `GOSCAPE_SP_TEST_CACHE` or `--cache-dir`). This limitation is
  explicit and accepted.

## Out of scope

- Producing per-rev packed caches (rev-specific `data/src`/reference-cache
  inputs; user-owned).
- The singleplayer `--memory-limit` flag (still an open optional item).
- Any behavior change beyond the documented Go-original knobs; every port
  preserves stock defaults on its branch.
- Pushing any branch.
