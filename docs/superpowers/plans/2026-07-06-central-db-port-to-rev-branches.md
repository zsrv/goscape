# Central-DB Consolidation Port to rev Branches Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the rev-274 central-database consolidation + PostgreSQL backend (and both follow-up fixes) to rev-254, rev-245.2, rev-244, and rev-225, adapting each branch's schema and friends behaviors to its own pinned TS revision.

**Architecture:** Four sequential per-branch phases, newest first. Each phase: TS audit first (produces the branch's normative behavior contract), then a compressed replay of the rev-274 arc — gamedb package + per-branch DDL, app wiring, login re-plumb + time.Time, friends re-key per audit, deployment/docs — with rev-274's committed files as the reference implementation and the audit as the source of per-branch deltas.

**Tech Stack:** Go 1.26, modernc.org/sqlite, jackc/pgx/v5, golang-migrate v4, dskit modules/services, buf (protos), helm.

**Spec:** `docs/superpowers/specs/2026-07-06-central-db-port-to-rev-branches-design.md`
**Reference implementation:** rev-274 at `311e0502` (consolidation merge `55bd489b` + follow-ups `bc44f258`, `311e0502`). The rev-274 spec/plan documents live at `docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md` and `docs/superpowers/plans/2026-07-05-central-db-consolidation-postgres.md` on rev-274.

## Global Constraints

- Branch order is HARD: rev-254 → rev-245.2 → rev-244 → rev-225. A branch's phase completes (all tasks + reviews + verification green) before the next branch starts.
- Work in each branch's existing worktree: `~/Code/github.com/zsrv/goscape-rev254`, `…-rev245.2`, `…-rev244`, `…-rev225`. Commits land directly on the rev branch (no feature branches). Every shell command in a phase runs from that phase's worktree root unless stated otherwise.
- After EVERY implementer task: controller checks `git status --short` + `git log --oneline -3` in the MAIN checkout (`~/Code/github.com/zsrv/goscape`) AND the other worktrees' tips for stray writes/commits (known failure mode).
- Go commands: prefix `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`. Git commits: `git commit --no-gpg-sign`; `git status --short` before staging; stage only named paths. Report SHAs only from observed `git log` output.
- TS reference reads: `git -C ~/Code/github.com/LostCityRS/Engine-TS show <PIN>:<path>` — never move that checkout's branch. Pins (verified against main:REFERENCES.md and present locally): **rev-254 → `2e3bcf43`, rev-245.2 → `3c16994c`, rev-244 → `9aadcec4`, rev-225 → `e1dea19f`**. No other LostCityRS paths may be read.
- Per-branch TS fidelity is BINDING: where the branch's audit contract disagrees with rev-274 behavior OR with this plan's expectations, the audit wins.
- FK posture: FK + `ON DELETE CASCADE` on account-id columns goscape reads/writes; NO FK on free-string ignore targets or session-uuid soft references; dormant tables bare. An account-id-keyed `public_chat` (if the audit confirms it, e.g. rev-244) DOES get the FK.
- Timestamps: timestamptz (postgres) ⇔ DATETIME declared type (sqlite) on the same column set per branch; Go reads/writes via time.Time / sql.NullTime; the load-bearing modernc-decltype comment appears in every sqlite DDL.
- Every production SQL string wrapped in `db.Rebind(...)`. jstring trap: multiples of 37 map to "invalid_name" — use the `distinctUsername37s` helper pattern in tests that need many usernames.
- Trust fresh `go test`/`go build` output only; IDE diagnostics during ports are routinely stale.
- No manual smoke tests (user decision). Gated live-postgres suites run per branch against the user's podman server (`GOSCAPE_TEST_POSTGRES_DSN='postgres://postgres:goscape@localhost:5432/goscape_test?sslmode=disable'`); those runs need the sandbox network override — if the server is down, report it and mark that verification pending rather than skipping silently.
- `go mod tidy` is broken repo-wide (pre-existing, unrelated: coder/websocket internal test package). Use targeted `go get` only.

## Pre-verified facts the tasks rely on

- Login migration files are **blob-identical across all five branches** (rev-225 lacks only `000005_rev244_b5.up.sql`). Therefore the login-tables section of rev-274's unified DDL is verbatim-correct for rev-254/245.2/244; rev-225's differs by a deterministic delta (below).
- Current friends `public_chat` era per branch: rev-254 session_uuid-keyed (+profile/world extras); rev-245.2/244 username-keyed (+profile/world); rev-225 session_uuid-keyed (+profile).
- `proto/friends/friends.proto` `PublicMessageRequest` per branch: rev-254 has `session_uuid`+`profile`; rev-245.2/244 have `username`+`profile`; rev-225 has `session_uuid` and NO profile field.
- rev-225 login-schema delta vs 274 DDL: NO `login`, `message_thread`, `message`, `message_status`, `account_session`, `wealth_event` tables (nor `idx_login_account_ip_time`, `idx_wealth_event_recipient`); `account_login` has NO `logged_out`/`logout_time` columns (its shape is `account_id, profile, node_id, logged_in` PK(account_id, profile), FK CASCADE); `account` KEEPS `logout_time` (nullable, time-semantic → DATETIME/timestamptz).

---

# Phase template

Phases P1–P4 instantiate the same seven tasks. Task numbering: `P<n>.T<m>`. Branch parameters:

| Phase | Branch | Worktree | TS pin | Report/audit prefix |
|---|---|---|---|---|
| P1 | rev-254 | `…/goscape-rev254` | `2e3bcf43` | `port254` |
| P2 | rev-245.2 | `…/goscape-rev245.2` | `3c16994c` | `port2452` |
| P3 | rev-244 | `…/goscape-rev244` | `9aadcec4` | `port244` |
| P4 | rev-225 | `…/goscape-rev225` | `e1dea19f` | `port225` |

All audit/report files live in the MAIN checkout's `.superpowers/sdd/` (shared across worktrees is NOT true for untracked files — `.superpowers/` exists only in the main working tree; subagents write reports there by absolute path).

---

### Task T1: TS behavior audit for <branch>

**Files:**
- Create: `~/Code/github.com/zsrv/goscape/.superpowers/sdd/audit-<prefix>.md`
- No repo files change. Read-only against the TS checkout at <PIN> and the branch's current goscape sources.

**Interfaces:**
- Produces: the branch's normative behavior-contract file consumed by T4/T5 implementers and every reviewer in this phase.

- [ ] **Step 1: Locate the TS sources at the pin**

```bash
TS=~/Code/github.com/LostCityRS/Engine-TS
git -C $TS show <PIN> --stat --oneline | head -3          # confirm the pin resolves
git -C $TS ls-tree -r --name-only <PIN> | grep -iE "friend(server|serverrepository)?\.ts|schema\.prisma" 
```

Expected paths (VERIFY per pin; older pins may differ): `src/server/friend/FriendServer.ts`, `src/server/friend/FriendServerRepository.ts` (may not exist at older pins — friend logic may be inline in FriendServer.ts), `prisma/schema.prisma` or `prisma/singleworld/schema.prisma`.

- [ ] **Step 2: Extract and read**

```bash
git -C $TS show <PIN>:<friend-server-path>            # full read
git -C $TS show <PIN>:<repository-path>               # if it exists at this pin
git -C $TS show <PIN>:<prisma-path> | sed -n '/model friendlist/,/^}/p'   # + ignorelist, private_chat, public_chat, account
```

- [ ] **Step 3: Write the contract file**

`audit-<prefix>.md` MUST contain, with TS file:line citations for every row:

| Item | Record |
|---|---|
| `addFriend` | cap value; members-aware? owner/target resolution (which columns selected); dup handling; insert row shape |
| `addIgnore` | cap; owner resolution; target storage (string vs id); conflict handling |
| `loadFriends` / `loadIgnores` | join shape; ORDER BY presence/direction |
| `deleteFriend` / `deleteIgnore` | delete predicate shape |
| Private message path | existence check (throw/drop vs absent); insert row shape incl. timestamp handling |
| `public_chat` | full row shape; identity keying (session_uuid / username→account_id / other) |
| `private_chat` | full row shape |
| prisma models | verbatim field lists for friendlist, ignorelist, private_chat, public_chat, account |
| Deltas vs rev-274 contract | explicit list — every place this branch differs from the rev-274 implementation (the 274 contract is in `modules/friends/repository.go` doc comments on rev-274) |
| goscape-side notes | which of these behaviors the branch's CURRENT modules/friends code implements vs deviates on (read the branch's repository.go/handler.go) |

- [ ] **Step 4: Sanity gates**

The audit must explicitly answer: (a) does this TS have the members-aware cap? (b) what does `public_chat` key by, exactly? (c) does the PM path have the account-existence throw? (d) do friendlist/ignorelist/private_chat carry profile columns at this pin? If any answer cannot be determined from the pinned source, say so and STOP (NEEDS_CONTEXT) rather than guessing.

No commit (no repo changes).

---

### Task T2: gamedb package + per-branch unified schema

**Files (in the worktree):**
- Create (copy): `pkg/gamedb/` — everything except the two migration DDL files, verbatim from rev-274
- Create (adapt): `pkg/gamedb/migrations/sqlite/000001_init.up.sql`, `pkg/gamedb/migrations/postgres/000001_init.up.sql`
- Modify: `go.mod`, `go.sum` (pgx/v5)

**Interfaces:**
- Consumes: `audit-<prefix>.md` (friends-table DDL shapes).
- Produces: `gamedb.Config/Open/DB/Rebind/Migrate/IsForeignKeyViolation/NewMigratorService` + `gamedbtest.OpenTestSchema` — identical signatures to rev-274 (later tasks and copied code depend on them verbatim).

- [ ] **Step 1: Copy the branch-independent package**

```bash
git checkout rev-274 -- pkg/gamedb
git rm -r --cached pkg/gamedb/migrations 2>/dev/null; rm -rf pkg/gamedb/migrations   # DDL is per-branch; rebuilt next
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go get github.com/jackc/pgx/v5@v5.10.0
```

(Match the pgx version rev-274 pinned — check `git show rev-274:go.mod | grep pgx`. If `go get` needs more transitive sums, add them the same way; NEVER `go mod tidy`.)

- [ ] **Step 2: Compose the sqlite DDL**

Start from rev-274's file: `git show rev-274:pkg/gamedb/migrations/sqlite/000001_init.up.sql > pkg/gamedb/migrations/sqlite/000001_init.up.sql`. Then:

- **P1/P2/P3 (rev-254/245.2/244):** the login-tables section (account … wealth_event + their indexes) is verbatim-correct (blob-identical migration chains — pre-verified). Replace ONLY the friends-tables section (friendlist, ignorelist, private_chat, public_chat + their indexes) with the audit-contract shapes. Where the audit matches 274 (expected for most of friendlist/ignorelist/private_chat), the 274 DDL stands; where it differs (e.g. P3 public_chat keyed by account_id + profile + world per TS 244), write that shape with FK-follows-column-type (account-id ⇒ `REFERENCES account(id) ON DELETE CASCADE`; session-uuid/string ⇒ no FK). Keep the FK-posture header comment and the DATETIME-decltype comment; update the header's TS citation to this branch's pin.
- **P4 (rev-225):** additionally apply the login-schema delta: delete the `login`, `message_thread`, `message`, `message_status`, `account_session`, `wealth_event` tables and `idx_login_account_ip_time`, `idx_wealth_event_recipient`; change `account_login` to its pre-B5 shape (`account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE, profile TEXT NOT NULL, node_id INTEGER NOT NULL DEFAULT 0, logged_in INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (account_id, profile)`); add `logout_time DATETIME` back to `account` (after `muted_until`).
- Time-semantic columns get DATETIME decltype exactly where postgres gets timestamptz (Step 3 keeps the sets in lockstep).

- [ ] **Step 3: Compose the postgres DDL**

Same procedure from `git show rev-274:pkg/gamedb/migrations/postgres/000001_init.up.sql`, applying the identical structural edits with the rev-274 type mapping (BIGINT IDENTITY PKs; BIGINT for account-id-ish; INTEGER small ints; timestamptz + `DEFAULT now()`; `ipban.added_on` TEXT; session CHECK regex). The timestamptz column set must equal the sqlite DATETIME set exactly.

- [ ] **Step 4: Verify**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/... -count=1
```
Expected: PASS; the 3 `TestPostgres_*` SKIP. Note: `TestMigrate_CreatesAllTables` lists 16 tables — on P4 edit its table list to the branch's actual set (drop the six B5 tables). Cascade/CHECK/independent-clients tests must pass unmodified (their tables exist on every branch). On P3, if `public_chat` is account-id-keyed per audit, extend `TestMigrate_AccountDeleteCascades` in this task with a public_chat row assertion (insert via seeded account, delete account, expect 0 rows) — complete test edit, not a stub:

```go
	// P3 only — public_chat is account-id-keyed at TS 244 (audit-<prefix>.md):
	mustExec(`INSERT INTO public_chat (account_id, profile, world, coord, message) VALUES (?, 'main', 1, 0, 'hi')`, owner)
	// … and add {"public_chat", "account_id", owner, 0} to the cascade assertion table.
```
(Adjust column list to the audited shape.)

- [ ] **Step 5: Commit**

```bash
git add pkg/gamedb/ go.mod go.sum
git commit --no-gpg-sign -m "feat(gamedb): central-db client + <branch> unified schema (TS <PIN> shapes)"
```

---

### Task T3: app wiring — database module + config section

**Files:** `cmd/goscape/app/config.go`, `cmd/goscape/app/modules.go`, `cmd/goscape/app/config_test.go`, `cmd/goscape/app/modules_test.go`

**Interfaces:**
- Consumes: `gamedb.Config`, `gamedb.NewMigratorService` (T2).
- Produces: `Config.Database gamedb.Config` (yaml `database,omitempty`); module const `Database = "database"`; deps `Database:{Common}`, `Friends:{Common,Database}`, `Login:{Common,Database}`.

- [ ] **Step 1: Extract the rev-274 reference patch and apply**

```bash
git -C ~/Code/github.com/zsrv/goscape diff 8b9a889b..3c428bbf -- cmd/goscape/app/ > /tmp/t3-ref.patch
git apply --3way /tmp/t3-ref.patch || true   # expect fuzz/conflicts from branch drift — resolve by hand to the same end state
```

End state must match the Produces block exactly; `initDatabase` returns `nil, nil` + "module disabled" log when neither Login nor Friends is enabled; `Validate()` calls `c.Database.Validate()` before the module fan-out, unwrapped. If the patch conflicts badly, implement directly from the rev-274 files (`git show rev-274:cmd/goscape/app/modules.go` etc.) — same end state.

- [ ] **Step 2: Test fallout**

The branch's `TestConfigValidate_ZeroValue`-equivalent and DAG-topology pinning tests need the same re-pin rev-274 made (zero-value Config → `NewDefaultConfig()`; topology map gains the Database edges). Check whether this branch even has those tests (tech-debt-era drift); update what exists, add nothing new.

- [ ] **Step 3: Verify + commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/... ./pkg/gamedb/ -count=1
git add cmd/goscape/app/ && git commit --no-gpg-sign -m "feat(app): database module — central-db migration anchor + database: config section"
```

(login/friends still on their private DSNs at this point — expected, same as the 274 arc.)

---

### Task T4: login → gamedb client + time.Time (274 Tasks 4+11 combined)

**Files:** `modules/login/**` (delete `migrations/`; slim `db.go`; thread `*gamedb.DB`; time sweep), `cmd/goscape/app/modules.go` (initLogin passes `g.cfg.Database`), any `login.New` callers found by the compile gate (e.g. `modules/world/friends_smoke_test.go` on branches that have it)

**Interfaces:**
- Consumes: `gamedb.Open/DB/Rebind/Migrate` (T2), app `g.cfg.Database` (T3).
- Produces: `login.New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger)`; login reads/writes times as time.Time/sql.NullTime.

Reference implementation: `git -C <main> diff 3c428bbf..3596d06e -- modules/login/` (re-plumb) and `git -C <main> diff e4917f9e..fc949fa8 -- modules/login/` (time sweep). These are guides, not patches — this branch's function inventory differs (P4/rev-225 has NO hop-timer/B5 code; P1–P3 should closely match 274's inventory).

- [ ] **Step 1: Re-plumb** — delete `modules/login/migrations/` + db.go's openDB/dsnWithPragmas/ensureDBParentDir/migrateDB/embed; `New` gains `dbCfg`; `starting` uses `gamedb.Open(l.dbCfg, l.log)` with error text `"open central database: %w"`; thread `*gamedb.DB` through every `*sql.DB` param/field; wrap EVERY SQL string in `db.Rebind(...)`. Enumerate first: `grep -n "sql.DB\|QueryRowContext\|QueryContext\|ExecContext" modules/login/*.go | grep -v _test`. Config loses `SQLiteDSN` (field+flag+Validate). App initLogin passes `g.cfg.Database`.
- [ ] **Step 2: Time sweep** — delete `dbTimeFormat`; NullString time fields → sql.NullTime; formatted writes → time.Time (UTC preserved); parse sites → .Valid/.Time; `insertAccount` LastInsertId → `INSERT … RETURNING id`. Enumerate: `grep -rn "dbTimeFormat\|time.Parse\|LastInsertId" modules/login/`. After: `grep -rn dbTimeFormat modules/login/` MUST be empty.
- [ ] **Step 3: Test fallout** — `createTestDB` becomes the gamedb flavor with env-gated postgres mode: copy the body from `git show rev-274:modules/login/db_test.go` (the helper + its imports), reusing `gamedbtest.OpenTestSchema`. Delete pragma-helper tests (pinned in pkg/gamedb now); fix `''` session_uuid inserts to well-formed UUIDs; formatted-time seeds → time.Time; guard sqlite-only probes (`PRAGMA`/`sqlite_master`) with the env-var skip. Fix every `login.New` caller the compile gate exposes.
- [ ] **Step 4: Verify + commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ ./cmd/... -count=1 && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
git add -A modules/login/ cmd/goscape/app/modules.go <other callers>
git commit --no-gpg-sign -m "refactor(login): independent gamedb client; time.Time end-to-end; retire private lineage"
```

---

### Task T5: friends re-key per audit + restorations (274 Tasks 5+6 combined)

**Files:** `modules/friends/**` (delete `db.go`, `db_test.go`, `migrations/`; re-key `repository.go`; restore in `handler.go`; wiring in `friends.go`, `config.go`), `cmd/goscape/app/modules.go` (initFriends), callers found by compile gate (`modules/world/friends_smoke_test.go` where present)

**Interfaces:**
- Consumes: `audit-<prefix>.md` (THE contract), T2 schema, `jstring.FromBase37/ToBase37`.
- Produces: repository API in the rev-274 shape (username37-based method signatures; `errAccountMissing` sentinel if-and-only-if the audit shows the TS PM existence check) with per-branch internals.

Reference implementation: rev-274's `modules/friends/repository.go` + `handler.go` (read directly with `git show rev-274:<path>`). Implementation rule: **for each method, take the rev-274 implementation, then apply the audit contract's deltas** — e.g. if the audit says the cap is flat 100 without members, the owner-resolution query selects only `id` and the constant set shrinks; if `public_chat` is account-id-keyed (expected P3), `LogPublicMessage` takes the branch's wire identity (username37 at 244/245.2), resolves it per the TS shape, and inserts the audited row; if the TS PM path has no existence throw (possible at older pins — audit decides), `LogPrivateMessage` stays unconditional and NO `errAccountMissing`/handler-drop is introduced (and the branch's `NAI-S4A-D-FED-*` blocks are retired with a comment stating the TS-at-pin truth instead).

- [ ] **Step 1: New-behavior tests first (RED)** — port rev-274's dual-pin tests (`git show rev-274:modules/friends/repository_test.go`, the `TestAddFriend_*`/`TestAddIgnore_*`/`TestLogPrivateMessage_*`/`TestLogPublicMessage_*` block + `seedAccount`/`seedMemberAccount`/`distinctUsername37s` helpers), then EDIT them to the audit contract (drop the members-cap tests if the audit says flat cap; re-shape the public_chat row-shape test to the audited shape; drop/keep PM-drop tests per audit). Every kept test cites the branch pin's TS file:line.
- [ ] **Step 2: Re-key repository.go + handler.go per the rule above.** Wiring: `friends.New(cfg, dbCfg, logger)`; `starting` opens via gamedb; config loses `SQLiteDSN`; app initFriends passes `g.cfg.Database`. Delete db.go/db_test.go/migrations. `createTestDB` gamedb-flavor with env-gated pg mode (copy from rev-274's `modules/friends/repository_test.go`).
- [ ] **Step 3: Suite fallout** — seed accounts in every list/PM test per the audit's resolution rules; delete tests pinning retired federation behavior (name them in the commit message); `distinctUsername37s` wherever sequential 37s appear; fix `friends.New` callers the compile gate exposes.
- [ ] **Step 4: Verify + commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ ./cmd/... -count=1 && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
git add -A modules/friends/ cmd/goscape/app/modules.go <other callers>
git commit --no-gpg-sign -m "feat(friends): re-key persistence to TS <PIN> account-id schema via central database

Audit contract: .superpowers/sdd/audit-<prefix>.md. Deleted federation-era tests: <names>"
```

---

### Task T6: examples, Helm, proto, docs (274 Tasks 7+8+13+14 compressed)

**Files:** `examples/bundled/goscape.yaml`, `examples/full-config-reference.yaml`, `examples/bundled/README.md`, `production/helm/goscape/values.yaml`, `production/helm/goscape/templates/_helpers.tpl`, `proto/friends/friends.proto` + regenerated `pkg/friendspb/friends.pb.go`, `CLAUDE.md`, `docs/PORTING.md`

- [ ] **Step 1: Examples** — mirror rev-274's end state (`git show rev-274:examples/…`), adapted to the branch's option set: add the `database:` section (backend sqlite default `data/goscape.db`; postgres dsn ""/max_open_conns 8 documented at ACTUAL defaults), remove `sqlite_dsn` from login/friends sections, fix README data paths, fix the env-expansion example line if the branch has it.
- [ ] **Step 2: Helm** — apply rev-274's two chart deltas (`git -C <main> diff 31cec693..65dea836 -- production/helm/goscape/` and `git -C <main> diff 8a4c75de..2237b192 -- production/helm/goscape/` plus the sslmode fix `1679e9e2`): unified `database:` block in baseConfig (SingleBinary|Management gate), per-module dsn lines removed, `goscape.database.*` values with **`sslmode: prefer`** default, secret-based DSN via `${GOSCAPE_DB_PASSWORD}` + `--config.expand-env=true` + secretKeyRef env when postgres active, `required` guards. Resolve per-branch chart drift by hand to the same end state.
- [ ] **Step 3: Proto comment** — update `PublicMessageRequest`'s stale-federation comments to the branch's post-consolidation truth, then `make protos` and verify the pb.go diff is comment-only:
  - P1 (rev-254, session_uuid+profile fields): same statement as rev-274's (`git show rev-274:proto/friends/friends.proto`), citing TS `2e3bcf43`.
  - P2/P3 (username+profile fields): state that TS <PIN> resolves `username`→account_id against the shared account table at insert (cite the audited FriendServer lines); with the central DB the server now performs that resolution; `profile`/`world_id` consumption per the audited row shape (P3 keeps them if the TS row carries profile/world; the comment must match what T5 actually implemented).
  - P4 (session_uuid, no profile field): if the current comment block has no federation claim, verify and touch nothing (record that in the report); otherwise state the session-join truth citing TS `e1dea19f`.
- [ ] **Step 4: Docs** — `CLAUDE.md`: module graph gains the database row; config note sentence (copy rev-274's wording). `docs/PORTING.md`: dated entry per the branch, modeled on rev-274's "DB-2 federation RETIRED" entry but naming THIS branch's pin and the audit's delta list; mark the branch's superseded federation rows retired per file convention.
- [ ] **Step 5: Verify + commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/bundled/goscape.yaml --config.verify=true
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/full-config-reference.yaml --config.verify=true
helm lint production/helm/goscape && helm template test production/helm/goscape | grep -c sqlite_dsn   # expect 0
git add examples/ production/helm/goscape/ proto/ pkg/friendspb/ CLAUDE.md docs/PORTING.md
git commit --no-gpg-sign -m "docs+helm+proto: central-db config surface for <branch>"
```

---

### Task T7: Phase verification (controller-run)

- [ ] Full sweep in the worktree: `go test ./... -count=1`; compile gate `go test -run '^$' ./...`; `go vet ./...`; `CGO_ENABLED=1 go test -race ./pkg/gamedb/... ./modules/login/ ./modules/friends/ -count=1`; `CGO_ENABLED=0 go build -trimpath ./...`.
- [ ] Clean-break probe: scratch copy of the bundled yaml + `sqlite_dsn: x` under `login:` → `--config.verify=true` fails naming `sqlite_dsn`.
- [ ] Helm renders: sqlite default (no `sqlite_dsn` anywhere, `database:` in SingleBinary/Management only); postgres Management render (dsn with `${GOSCAPE_DB_PASSWORD}`, sslmode `prefer`, env + expand-env arg); World exclusion; `required` guards fire.
- [ ] Gated live-postgres (sandbox override): `GOSCAPE_TEST_POSTGRES_DSN='postgres://postgres:goscape@localhost:5432/goscape_test?sslmode=disable' go test ./pkg/gamedb/... ./modules/login/ ./modules/friends/ -count=1` — validates this branch's pg DDL live. If the server is unreachable, record "pending live-pg" in the ledger; do not fail the phase silently.
- [ ] Stale-reference sweep: `grep -rn "login.db\|friends.db\|sqlite_dsn\|SQLiteDSN" --include='*.go' --include='*.yaml' --include='*.tpl' .` → hits only in docs/history/test-temp-file basenames.
- [ ] Main-checkout + other-worktree cleanliness check.
- [ ] Per-branch final review (whole-phase diff `scripts/review-package <phase-base> HEAD`, reviewer verifies against `audit-<prefix>.md` + this plan's constraints; sonnet cap).
- [ ] Ledger entry with the branch tip SHA; then the next phase starts.

---

# Phase instantiations

### Phase P1 — rev-254 (worktree `…/goscape-rev254`, pin `2e3bcf43`)

Tasks P1.T1 … P1.T7 per the template. Branch-specific expectations (audit confirms/overrides):
- TS 254 `public_chat` is session_uuid-keyed `{session_uuid, timestamp, coord, message}` (goscape's current 000005 comment cites FriendServer.ts:286-297 @2e3bcf43 exactly like 274) — expect the friends section of the DDL and `LogPublicMessage` to match rev-274 verbatim.
- Open question for the audit: members-aware cap and `addFriend` resolution shape at `2e3bcf43` (rev-274's members-200 arrived at some point ≤274; determine whether 254 has it).
- This branch's state most closely matches 274-pre; expect T4/T5 reference diffs to apply with minimal drift.

### Phase P2 — rev-245.2 (worktree `…/goscape-rev245.2`, pin `3c16994c`)

- Current goscape `public_chat` is username-keyed (rev-244 era). The audit determines TS 245.2's true shape (likely the TS-244 username→account_id resolution model — confirm); T5 implements the audited shape, which the central DB now makes expressible.
- `PublicMessageRequest` has `username` + `profile` fields; proto comment per T6 Step 3 P2 rule.
- `isVisibleTo`/staff semantics at `3c16994c` cited in the branch's existing code comments (FriendServerRepository.ts:83 @3c16994c) — the audit re-checks visibility internals only if T5's port touches them (IsVisibleTo mechanics are goscape-side SQL; semantics unchanged unless the audit flags a delta).

### Phase P3 — rev-244 (worktree `…/goscape-rev244`, pin `9aadcec4`)

- Expected headline delta: TS 244 `public_chat` keyed by resolved account_id (+ profile + world; prisma `public_chat` model at `9aadcec4` — audit records the exact fields). T5 implements true TS-244 persistence for the first time: resolve the wire username → account id at insert (missing account → per-TS behavior, audit says whether drop or throw), FK on the account-id column, `world`/`profile` columns per the audited model.
- T2 Step 4 adds the public_chat cascade assertion (code given in the template).
- Login B5 surface exists on this branch (hop timer etc.) — T4 site list ≈ 274's.

### Phase P4 — rev-225 (worktree `…/goscape-rev225`, pin `e1dea19f`)

- Apply the rev-225 login-schema delta (Pre-verified facts) in T2; edit `TestMigrate_CreatesAllTables`'s list accordingly.
- TS at `e1dea19f` predates later friend-server refactors: the audit must locate the friend-server file(s) (path may differ) and derive all shapes from scratch. Expect: no members cap, simpler PM path (existence check presence UNKNOWN — audit decides), `public_chat` session_uuid-keyed without profile (goscape's current 000003 carries profile — the audit + T5 decide whether profile stays [goscape multi-profile extension documented on this branch] or drops [TS-faithful]; default per fidelity policy: TS-faithful, drop it, profile recoverable via session join).
- `PublicMessageRequest` has no profile field; T6 Step 3 P4 rule.
- No B5 login code: T4's time sweep has fewer sites (no hop-timer parse); `accountByUsername` shape differs — enumerate by grep, not by 274's list.

---

## Execution notes

- Model selection per the SDD skill: audits and T5 need sonnet; T2/T3/T6 default sonnet (haiku only where the work is verbatim-copy + verify); reviewers sonnet (user cap).
- Each phase's reviewer receives: the task brief, the audit file, the review package, and the branch's TS pin for spot-checks.
- Ledger: `.superpowers/sdd/progress.md` in the MAIN checkout gains a "Port to rev branches" section; one line per task, one per phase completion with tip SHA.
- The rev-274 memory file `central_db_consolidation_2026_07_05.md` gets updated at the very end (all four branches recorded); PORTING.md updates happen per branch in T6.

## Self-review notes (resolved during writing)

- Login migration chains verified blob-identical across branches (the fact making T2's "copy the login section verbatim" safe); rev-225's delta enumerated exactly.
- Proto field surfaces verified per branch (254: session_uuid+profile; 245.2/244: username+profile; 225: session_uuid only).
- The plan deliberately does NOT pre-write per-branch friends code: the spec's audit-first mandate makes the audit file the code's source of truth; every T5 step names the exact rev-274 file to start from and the transformation rule. This is the no-placeholder form appropriate to a fidelity-audited port.
- `.superpowers/` exists only in the main checkout — all audit/report/ledger paths are absolute into it.
