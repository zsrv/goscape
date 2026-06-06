# RESUME: rev-244 port — Bundle 7 (goscape-cli unpack)

Self-contained resume prompt. Written 2026-06-06 after Bundle 6 shipped.

## Where you are

Multi-revision Go port of the LostCityRS Engine-TS server: `main` = codeless
docs hub; **`rev-244` = the active 225→244 porting branch**. Work on
`rev-244` only. The work list is the cross-pin diff
`git -C /home/owner/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4`.
Pins: `git show main:REFERENCES.md` §rev-244 (now also pins the
RuneScriptKt-26 jar sha and cloudflare/zlib `886098f3` — the gzip
byte-parity reference).

## Read these first (in order)

1. `git show main:PORTING-LESSONS.md` — porting philosophy, §3 gotchas,
   §5 gates.
2. `docs/superpowers/specs/2026-06-03-rev244-port-design.md` — umbrella;
   B7 = the last bundle.
3. `PORTING.md` §"rev-244 Bundle audit trail" §B6 — the full
   correspondence table + live-smoke record.

## State: B1..B6 ALL SHIPPED — umbrella items (a)-(d) at B6 ALL MET

**B6 (37 commits `30461f78..8104be1a`): pack pipeline re-baseline.**
**FULL-TREE BYTE PARITY achieved** vs the upstream 244 reference cache
(Engine-TS `9aadcec4` + Content `e5d0282e` + RuneScriptKt-26 jar):
2,671/2,671 files byte-identical (incl. script.dat/idx), ondemand.zip
4,764/4,764 content-identical. **Live 244-client smoke PASSED** (login,
walk, shop/bank, music, map crossing, npc kill/despawn/respawn). Final
integration review: READY.

Key B6 infrastructure you can lean on:

- **Pinned reference worktrees** at
  `/home/owner/Code/github.com/LostCityRS/Server244-ref/{engine,content,javaclient}`
  (Engine-TS @ pin, Content @ pin, Client-Java @ pin) — the OTHER local
  reference checkouts have ALL moved to later-revision branches
  (`254-GOSCAPE` etc.); NEVER use their working trees, always these
  worktrees. The reference cache lives at
  `Server244-ref/engine/data/pack` (+ `data/symbols`).
- **Parity gates** (env `GOSCAPE_REF244_DIR=<...>/Server244-ref/engine`):
  `pkg/packall/parity_test.go` (full tree) and
  `pkg/io/gziputil` `RefCorpus` (gzip corpus 5,626 files).
- **Bit-exact gzip**: `gziputil.CompressGz` routes through a pure-Go port
  of cloudflare-zlib deflate L6 (`cfdeflate.go`/`cftrees.go`) — bun's
  `node:zlib.gzipSync` backend. Do NOT swap back to stdlib flate.
- **`.sym` exporter** (`pkg/pack/compiler/symbols_export.go`) — byte-parity
  vs `data/symbols/`; the compiler-input diff anchor.
- Live-smoke fixes worth knowing: wire revision constant
  `pkg/io/protocol/revision` now 244 (`4606660a`); `world.content_path`
  feeds the midi name registry (`b26d8dd5`); dead NPCs must flow through
  `ComputeNpc(active=false)` (`973e221b`).

## Next: Bundle 7 — `goscape-cli unpack`

Scope (umbrella §B7): the NEW `tools/unpack` (+3,793 lines at the pin)
ports as a `goscape-cli unpack` command family.

```
git -C /home/owner/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4 --stat -- tools/unpack
```

(All-new files at 244 — read them whole, not as diffs.) Natural
verification: unpack the byte-parity cache and compare against the
Content worktree's source tree (roundtrip: pack→unpack→pack).

## Non-blocking follow-ups inherited from the B6 final review

1. `config.yaml` hardcodes the absolute `Server244-ref/content` path in
   `world.content_path` (live-smoke fix) — relativize or document as a
   local override if the repo gets wider distribution.
2. `rev244-b6-ondemand-zip`: ondemand.zip parity is content-level; B7's
   unpack work must consume ENTRY CONTENT, never assume raw-zip-byte
   parity.
3. `TestDecodeRealCacheBlob` "bad trailer position" — Arc-26 residual
   DECODER bug in `pkg/script/file.go`, still open; B7-adjacent (unpack
   reads script blobs) and now easily reproducible against the
   byte-parity cache.
4. `rev244-b6-build-stamp` int32 wrap ~2038 — documented, no action.
5. T4-era minor: `ValidateConfigPackNames` multi-orphan error is
   map-iteration-ordered (single-orphan fixtures only in tests).

## Process (B1-B6-proven; repeat it)

Brainstorm → spec (commit) → plan (writing-plans; bite-sized TDD; exact TS
extraction commands as contracts) → subagent-driven execution: implementer
(sonnet) → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks; full-suite gate + PORTING.md
correspondence-audit at bundle end; final whole-bundle integration review.

**Bake into every implementer prompt (recurring defects, B2-B6-proven):**
- Every `// TS <File>.ts:<lines>` citation verified against a `| cat -n`
  numbered listing BEFORE writing — and cite FINAL-FILE numbers, not
  diff-hunk numbers (two B6 fixes were off-by-N from this).
- Reject-path tests must seed earlier-gate prerequisites.
- Final-review "missing X" findings can be false positives — verify first.
- Interface-method additions cascade into test fakes.
- Run the modules/world suite when a world-exercised contract changes.
- Sandbox bash mangles `!=` in heredocs AND inline `python3 -c` — write
  code with Write/Edit tools.
- Ask "what does TS's CONSUMER do?" before modeling a producer.
- **NEW (B6): "wire a function" means CALLED FROM THE LIVE PATH** — two
  B6 tasks landed correct-but-dead code (uncalled validator, unwired CRC
  constants); spec reviewers must grep for call sites, not just
  existence.
- **NEW (B6): a goscape-specific constant with no TS counterpart file is
  invisible to TS-diff slicing** — the 225→244 revision constant survived
  five bundles because its file never appeared in any scope slice and its
  own test pinned the stale value. When a TS hunk changes a VALUE that
  goscape keeps in an infra-side constant, grep goscape for the OLD value
  repo-wide.
- **NEW (B6): live-smoke findings cluster in silent-degradation paths**
  (empty-path skip with no log; dead-entity skip in a render loop). When
  porting a gate, ask what happens on the EMPTY side of it.

## Mechanics & gotchas

- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix.
  Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: CGO_ENABLED=1.
  Every commit: `--no-gpg-sign` + the Claude trailer.
- modules/world full suite ~2.5 min — not hung.
- Sandbox `git status` shows phantom `??` dotfiles — never stage; never
  `git add -A`. Warn every subagent.
- Stale LSP diagnostics false-alarm whole files routinely — trust real
  build/vet/test runs only.
- Sandbox blocks localhost network — `curl 127.0.0.1` needs the bypass.
- TS citations cite the 244 pin; deviations get PORTING.md rows; accepted
  divergences get `PORTING-EXCEPTION (<id>, …)` markers (22 at B6 close).
- The smoke server runs via
  `go run ./cmd/goscape --config.file config.yaml` (cache at data/pack,
  packed from Server244-ref/content — re-pack with goscape-cli pack if
  data/ is cleaned).
