# Resume prompt — goscape public core porting (next session)

Point the next session at this file. Current as of 2026-06-03, `main` HEAD `5d878af9`.

## What this repo is now

On 2026-06-03 the monorepo was split: **this repo is the PUBLIC, core-only game server**
(filtered + content-scrubbed history, ~3378 commits). Everything analytics-related lives in
the private sibling module `../goscape-telemetry-platform` (depends one-way on this repo
via `replace`; do NOT add imports in that direction from here). The dormant no-op seams
(`pkg/telemetry` Emitter registry, `pkg/tapper` Tapper, `pkg/eventspb`, the inline
emit taps in world/login/script) are intentional public API — leave them.

Working notes for the split live in the project memory (`telemetry_split_pointer`);
`CLAUDE.md` reflects the current architecture (dskit at `pkg/dskit`, core-only app).

## Green baseline (verify before starting work)

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: **all 65 packages `ok`, zero FAIL** (verified 2026-06-03). Notes:
- The `modules/world` Reload tests skip automatically when the generated `data/pack`
  cache is absent (`realCacheDir` in `modules/world/testdata_path_test.go`). Build the
  cache with `goscape-cli pack` to exercise them for real.
- The old `pkg/pack` `TestBuildVerify_BUILD_VERIFY_NotPresent` CONFIRMED-EXCEPTION is
  RETIRED (the offending comment was reworded 2026-06-03) — a FAIL there is a real
  regression now.
- `go vet ./pkg/util/build/` reports pre-existing self-assignment lines (residue of the
  retired operator-flip mechanism) — known, see "repo hygiene" below.
- Stale-gopls fires phantom diagnostics constantly in this codebase; a fresh
  `go build ./...` is the only truth (long-standing project convention).

## Where the porting effort stands

Reference: Lost City Engine-TS at `/home/owner/Code/github.com/LostCityRS/Engine-TS`
(audits pinned at Engine-TS `e1dea19f`); Java client at
`/home/owner/Code/github.com/LostCityRS/Client-Java`. Wire revision 225
(`pkg/io/protocol/revision.Expected`).

The parity effort is, as tracked, **complete**:
- `docs/superpowers/audits/2026-05-23-ts-parity-fix-tracker.md` — **101/101 closed**.
- `PORTING.md` (the active backlog; closed log in `docs/PORTING-CLOSED.md`):
  - **Open deviations: 1** — ARCH-1 (tick recovery swallows world-script panic vs TS
    try/catch retry; `tick_recovery.go:54` + `npc_ai.go` despawn site). **Deferred
    indefinitely by decision** (Arcs 15/18/20/22) — do not pick up without a new reason.
  - **Open perf hotspots: 2 × LOW** (per-tick player-slice allocs in `tick.go`;
    O(N²) zone iteration in `npc_hunt_entities.go`) — optional, not urgent.
  - **2026-05-28 fresh-audit MEDIUM backlog: emptied** (script-core-1 closure,
    2026-05-31).
- Historical/superseded docs (do not resume from these): the 2026-05-23
  `ts-parity-full-audit-resume.md` handoff (superseded by the completed 2026-05-28 fresh
  audit), `plans/2026-05-23-pack-determinism.md` (shipped —
  `cmd/goscape-cli/cmd_pack_determinism_test.go` exists).

**Read `PORTING.md` §"Tracking conventions" before doing any parity work** — it encodes
the row lifecycle, closure shapes, EXCEPTION rules, the 4-pack bundle arc template, and
the subagent cadence this project runs on.

## Candidate work queues (pick with the user)

1. **Publish the repo** (it has NO git remote yet):
   - Create the GitHub repo, `git remote add origin …`, push `main`.
   - Never push from any backup artifact (`~/goscape-*.bundle`, `~/goscape-presplit-*`)
     — they contain the unscrubbed pre-split history.
2. **Repo hygiene for public credibility** (small, mechanical):
   - Trim Loki-boilerplate Makefile targets that reference nonexistent dirs (helm, mixin,
     dist/gox, snyk, dev-k3d, `validate-example-configs: loki` broken dep,
     `update-goscape-release-sha`) and the `.github/workflows` jsonnet that points at
     `zsrv/goscape-release`. Also resolves the THIRD-PARTY-NOTICES "Loki scaffolding"
     question by removing the derivation.
   - Clean `pkg/util/build/build.go` self-assignments (vet noise; flip-mechanism residue).
3. **Beyond-parity core features** (TS-parity is done; new work is product work):
   - Hiscores serving endpoint (write path shipped 2026-06-01 `fix/hiscore-port`;
     serving was deliberately YAGNI'd — see PORTING-CLOSED bundle-19 row).
   - The 2 LOW perf hotspots above, with benchmarks.
4. **Run the real thing**: pack content (`goscape-cli pack`), boot `--target all`, smoke
   with the Java client (login via `deploy/bundled/scripts/fake-login.sh` first).

## Conventions that survive the split (from PORTING.md / project memory)

- Go commands prefixed `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`; commits with
  `--no-gpg-sign`; commit with explicit pathspec (untracked env files live in the tree).
- The operator-flip files / `#274` flip-prediction conventions are RETIRED with the split
  (the flip mechanism is gone; `deploy/bundled/goscape.yaml` is now a plain core example).
- True-to-TS gate: deviations need a tracker row + `PORTING-EXCEPTION` marker; TS source
  reads happen at plan-time, not from memory.
