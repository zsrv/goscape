# RESUME: rev-244 port — Bundle 6 (pack pipeline re-baseline)

Self-contained resume prompt. Written 2026-06-05 after Bundle 5 shipped.

## Where you are

Multi-revision Go port of the LostCityRS Engine-TS server: `main` = codeless
docs hub; **`rev-244` = the active 225→244 porting branch**. Work on
`rev-244` only. The work list is the cross-pin diff
`git -C $HOME/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4`
(local checkout sits AT the 244 pin `9aadcec4`). Pins:
`git show main:REFERENCES.md` §rev-244.

## Read these first (in order)

1. `git show main:PORTING-LESSONS.md` — porting philosophy, §3 gotchas
   (especially "Pack pipeline / byte parity": fix-determinism-FIRST,
   jagFileVersion=26), §4 citations, §5 gates.
2. `docs/superpowers/specs/2026-06-03-rev244-port-design.md` — umbrella:
   7 bundles, definition of done; §Risks ranks the B6 compiler swap as
   risk #1.
3. `PORTING.md` §"rev-244 Bundle audit trail" — B1..B5 correspondence
   tables and decision rows.

## State: B1 + B2 + B3 + B4 + B5 ALL SHIPPED

B1 (12 commits): io/cache primitives. B2 (28): wire protocol + rsbuf
damage2. B3 (48): engine core. B4 (25): script runtime (244 opcode
renumber). **B5 (16 commits `8fddfb4d..4e4f8192`): server/login/db** —
login schema 000005 (per-attempt `login` table, per-profile
`account_login.logged_out`/`logout_time`, message-centre +
dormant account_session/wealth_event tables; `account.logout_time`
DROPPED — login-server-7 CLOSED), 3-in-5s rate limit + 45s hop timer
(LOGIN_RESULT_RATE_LIMITED→byte 16, HOP_TIMER→byte 9), real
getUnreadMessageCount on both reply paths, friends multi-profile
(per-message `profile` on every RPC, repositories[profile],
(profile,*)-keyed registries, WorldConnect mismatch reject deleted,
`friends.node-profile` config retired), public_chat re-keyed
session_uuid→username+world (legacy rows preserved as
`public_chat_legacy_225`), logger report/input_track seam re-keys.
Gates 2026-06-05: build 0 / vet pre-existing-only / full suite 0 /
`-race` 0 (login+friends+world).

**New PORTING-EXCEPTION:** `rev244-b5-startup-profile` (TS dropped
profile from world_startup while its server still filters by it —
upstream reset broken at pin; goscape keeps the field). Marker count 22.
**B5 #177 catch:** `world_heartbeat`'s 244 consumer is `case
'world_heartbeat': break;` — doc-closed NO-OP, no RPC modeled.

## Next: Bundle 6 — pack pipeline re-baseline

**B6 is where ALL deferred windows close (user decision, B3 spec):**

1. **PREREQUISITE: generate the 244 reference cache** from the local
   Engine-TS 244 checkout (Engine-TS 244 + Content 244 + RuneScriptKt-26
   jar — pins in main:REFERENCES.md). This was the missed B1 de-risk;
   nothing else in B6 can be verified without it.
2. `tools/pack` delta (~1.3k/1.2k) — the 244 compiler swap
   (`@lostcityrs/runescript` npm → RuneScriptKt-26 jar) is the umbrella's
   RISK #1; apply the fix-determinism-first byte-diff loop (Arc-26
   lessons #188-194).
3. `src/cache/DevThread.ts` packAll signature (B1-deferred) +
   `src/app.ts` BUILD_STARTUP_UPDATE/packAll(modelFlags) (B3-deferred).
4. **Windows that close here:** map-delivery (B2), midi
   live-verification (B3), B1 format window
   (`rev244-b1-format-window` marker at pkg/objtype/seqtype.go:98 + the
   two skip-gated tests), **B4 script.dat opcode-numbering window**
   (compiler+runtime renumbered together; byte-parity vs the 244
   reference cache verifies it).
5. **End-to-end 244-client login smoke** (Client-Java `01f16088` /
   goscape-client `rev-244`) — the umbrella's (b) definition-of-done
   item, amended from post-B2+B3 to B6.

**B6 must NOT double-apply:**
- The B1 clientinterface writer pull-forward hunks
  (`pkg/pack/clientinterface/pack.go` — Component trans P1 +
  layer-childCount g1→g2, TS PackShared.ts:267-274,428-431, landed in
  `e4e881d8`).
- jagFileVersion stays 26 unless the upstream meta-repo moves its
  runescript pin past `750291c` (PORTING-LESSONS §3).

**B6 brief flags:**
- `TestDecodeRealCacheBlob` "bad trailer position" is a pre-existing
  DECODER bug in pkg/script/file.go (Arc-26 residual), NOT a packer bug —
  don't chase it through the packer.
- The B1 format-window skips: `TestLoadSeqTypes_FromPack`
  (pkg/objtype/seqtype_test.go) + `TestNewServer_LoadsWordencFilter`
  (modules/world/server_wordenc_test.go) — both un-skip once the 244
  cache exists.
- Empty/absent cache → empty CrcTable → every login rejected
  out-of-date (rev244-b3-crc-compare row) — the login smoke needs the
  real cache in place first.

## Process (B1-B5-proven; repeat it)

Brainstorm → spec (commit) → plan (writing-plans; bite-sized TDD; exact TS
extraction commands as contracts) → subagent-driven execution: implementer
(sonnet) → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks; full-suite gate + PORTING.md
correspondence-audit at bundle end; final whole-bundle integration review.

**Bake into every implementer prompt (recurring defects, B2-B5-proven):**
- Every `// TS <File>.ts:<lines>` citation verified against a `| cat -n`
  numbered listing BEFORE writing.
- Every reject-path test must seed earlier-gate prerequisites.
- Final-review "missing X" findings can be false positives — verify first.
- Interface-method additions cascade into test fakes — PORTING.md
  §NEW-INTERFACE-METHOD-COMPILE-CASCADE.
- pkg/script-only gates MISS modules/world E2E fallout — run the world
  suite when a contract the world exercises changes (B4 lesson).
- **NEW (B5 lesson): sandbox bash mangles `!=` inside heredocs** (history
  expansion writes `\!=` even in quoted heredocs) — write Go test code
  with the Write/Edit tools, never `cat >> file << 'EOF'`.
- **NEW (B5 lesson): ask "what does TS's CONSUMER do?" before modeling a
  producer** — world_heartbeat's consumer was `break;`; the handoff's
  "gRPC proto surface" framing would have shipped a dead RPC (#177 at
  the message level, not just the feature level).

## Mechanics & gotchas

- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix.
  Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: CGO_ENABLED=1.
  Every commit: `--no-gpg-sign` + the Claude trailer.
- modules/world full suite ~2.5 min — not hung.
- Sandbox `git status` shows phantom `??` dotfiles (device-node masks) —
  never stage; never `git add -A`. Warn every subagent.
- Post-TDD stale LSP diagnostics are normal AND routinely false-alarm
  whole files (every B4/B5 interface change) — trust real build/vet/test
  runs only.
- Proto changes: edit proto/*.proto then `make protos` (buf generate,
  local plugins; it deletes ALL *.pb.go first — stage only
  genuinely-changed generated files).
- TS citations cite the 244 pin; adopt 244 names on renames; deviations
  get PORTING.md rows; accepted divergences get
  `PORTING-EXCEPTION (<id>, …)` markers (22 mentions at B5 close).
- Pre-existing gofmt-dirty files (NOT B5's): heropoints_test.go,
  world.go, pkg/pathfinder/routefinder/pathfinder1_test.go,
  pkg/script/handlers_number_test.go, pkg/telemetry/noop.go — standalone
  sweep candidates, out of bundle scope.
- Pre-existing cleanup-pass candidates noted in B5 reviews (no TS
  counterpart changed at 244; do NOT fix mid-bundle): friends
  subscriptions/worldSubscriptions `send` unlock-then-write drop window;
  admin_bridge `context.Background()` (bypasses bridgesCtx shutdown
  cancellation); silent parse-failure bypass on banned/muted/logout_time
  timestamp reads; B4-era `activePlayerPointer`/`setActivePlayer`
  scaffolding + `Npc.HeroPointsClear` zero-caller seam.
