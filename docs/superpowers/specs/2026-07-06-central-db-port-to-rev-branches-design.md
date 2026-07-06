# Central-DB Consolidation Port to rev-254 / rev-245.2 / rev-244 / rev-225 Design

**Date:** 2026-07-06
**Branches:** rev-254, rev-245.2, rev-244, rev-225 (ported in that order, newest first)
**Tech:** Go 1.26+, modernc.org/sqlite, jackc/pgx/v5, golang-migrate (all as landed on rev-274)
**Scope:** Propagate the rev-274 central-database consolidation + PostgreSQL backend (merge `55bd489b`) and its two follow-ups (nil-logger guard `bc44f258`, friends.proto federation-comment retirement `311e0502`) to the four other revision branches, adapting each branch's schema and friends behaviors to **its own** pinned TS revision.

## Context

The rev-274 effort (spec `2026-07-05-central-db-consolidation-postgres-design.md`, plan `2026-07-05-central-db-consolidation-postgres.md`) replaced the DB-2 federated split (separate `login.db`/`friends.db`) with one central database that login and friends access as independent clients (`pkg/gamedb`, invisible `database` migration-anchor module), re-keyed the friends tables to the TS account-id schema with FK+CASCADE extensions, restored the TS behaviors federation had blocked, and added a config-selectable PostgreSQL backend. The user has now directed propagation to all rev branches — an explicit override of the original rev-274-only scope decision, in the same way the arch-review follow-ups were force-propagated.

Current branch tips at spec time: rev-254 `21f76eda`, rev-245.2 `1c8698d1`, rev-244 `fa6b068f`, rev-225 `fcd4e9c2` (all post-tech-debt-cleanup, none pushed). Worktrees exist at `../goscape-rev254`, `../goscape-rev245.2`, `../goscape-rev244`, `../goscape-rev225`.

### Why this is an adaptation, not a copy

- **Friends schema/behavior varies by TS revision.** Each branch's friends tables and persistence semantics must mirror its own Engine-TS pin (user decision: per-branch TS fidelity). Known/expected deltas, to be confirmed by per-branch audit:
  - The members-aware 200 friend cap is believed 274-only.
  - TS 244 keys `public_chat` by **resolved account_id** (+ profile + world) — the old federated design could not express this (goscape 244/245.2 store raw username instead); the central DB makes the true TS-244 shape implementable for the first time.
  - TS 254 keys `public_chat` by session_uuid (like 274); goscape rev-254 currently carries extra profile/world columns that the consolidation removes (recovered via `session` join).
  - TS 225 predates several of these shapes; its actual query/schema forms must be read, not assumed.
- **Login schema varies by branch.** rev-225 has no B5 surface: no `login`, `message_thread`, `message`, `message_status`, `account_session`, `wealth_event` tables; no `account_login.logged_out`/`logout_time` (its `account` still carries `logout_time`); no hop timer. rev-244/245.2/254 match 274's login-table set.
- **Every branch has drifted** (per-branch public_chat re-keys, tech-debt adaptations), so cherry-picking the 21 rev-274 commits was rejected — conflicts would smuggle 274 behaviors past the fidelity audit.

## Decisions (from brainstorm Q&A)

1. **Fidelity:** per-branch TS fidelity. Each branch's unified schema + friends behaviors mirror its own Engine-TS pin, audited before implementation.
2. **Manual smoke tests: none.** Verification is fully automated per branch (suites, gates, config-verify, helm render, gated live-postgres), leaning on the rev-274 boot-smoke precedent for the shared architecture.
3. **Approach:** per-branch adapt-with-audit, newest-first (Approach A). COPYABLE-vs-ADAPT per the established cross-rev methodology; commits land directly on each rev branch in its worktree.

## Goals

- All four branches gain: `pkg/gamedb` (with nil-logger guard), the `database` module, one central DB (default `data/goscape.db`), independent-client wiring for login+friends, clean-break config (`database:` section; per-module `sqlite_dsn` keys removed), PostgreSQL backend (pgx/v5, per-dialect migrations, Helm postgres values with `sslmode: prefer` default), time.Time end-to-end login time handling with the DATETIME/timestamptz decltype principle, per-branch-truthful friends.proto comments, and docs.
- Each branch's friends persistence matches its own TS pin, with the DB-2 exceptions retired in that branch's terms.
- FK posture everywhere: FK+CASCADE on account-id columns goscape reads/writes; no FK on free-string ignore targets or session-uuid soft references; dormant tables bare. Where a branch's TS shape differs (e.g. 244's account-id `public_chat`), the FK follows the column type — an account-id-keyed `public_chat` gets the FK.

## Non-Goals

- Changing anything further on rev-274 (it is the source, not a target).
- Cross-branch behavior uniformity where TS revisions differ (explicitly rejected).
- Data migration tooling (clean break per branch, same as 274; old per-branch `login.db`/`friends.db` files are simply retired).
- Pushing any branch.
- Manual/boot smoke testing (user decision).

## Design

### 1. Port order and workspace mechanics

rev-254 → rev-245.2 → rev-244 → rev-225, each branch completing (implementation + reviews + verification) before the next starts. Work happens in each branch's existing worktree; commits land directly on the rev branch (no feature branches — matches the tech-debt-cleanup port pattern). After each subagent task, the controller verifies no stray writes landed in other worktrees or the main checkout (`git status` + `git log` on main tree — the known haiku-class failure mode).

The TS reference for every branch is the single local canonical checkout `/home/owner/Code/github.com/LostCityRS/Engine-TS`, read at each branch's pin via `git -C <checkout> show <pin>:<path>` — the checkout's own branch (274-GOSCAPE) never moves. Pins from `main:REFERENCES.md`: rev-254 → `2e3bcf43`, rev-245.2 → `3c16994c`, rev-244 → `9aadcec4`, rev-225 → `e1dea19f`. The prisma schema path at older pins may differ (e.g. `prisma/schema.prisma` vs `prisma/singleworld/schema.prisma`); the audit locates it per pin.

### 2. COPYABLE vs ADAPT inventory

**COPYABLE** (verbatim `git checkout rev-274 -- <path>` from within the target worktree; content is branch-independent):
- `pkg/gamedb/*.go` + `pkg/gamedb/gamedbtest/` (includes the nil-logger guard + `TestOpen_NilLoggerSafe`, Rebind, `IsForeignKeyViolation` with pgx path, MigratorService, the gated-postgres harness)
- `go.mod`/`go.sum` deltas (pgx/v5 and transitive sums) — apply via `go get`/targeted edits per branch, since each branch's module graph may differ slightly; verify with the compile gate (note: `go mod tidy` is broken repo-wide for an unrelated pre-existing reason; use targeted `go get`)

**ADAPT** (per branch):
- `pkg/gamedb/migrations/sqlite/000001_init.up.sql` and `migrations/postgres/000001_init.up.sql` — composed from (a) that branch's own cumulative login-table DDL (derived from its `modules/login/migrations/` chain before deletion) and (b) the friends tables per that branch's TS audit. Both files carry the DATETIME-decltype load-bearing comment; the timestamptz(pg) ⇔ DATETIME(sqlite) column-set correspondence is exact per branch.
- `modules/login/` — the gamedb-client re-plumbing + Rebind sweep + time.Time sweep against that branch's actual function inventory (rev-225 lacks the hop-timer/B5 code paths; enumerate by grep per branch, not by the 274 site list).
- `modules/friends/` — repository/handler re-key to the branch's TS behavior contract (§3); module wiring mirrors 274's shape; per-branch test fallout (seeding, retired-federation-test deletion, `distinctUsername37s` where sequential 37s appear).
- `cmd/goscape/app/` — `database` module registration + deps + config section, resolved against per-branch drift in `modules.go`/`config.go`.
- `examples/bundled/goscape.yaml`, `examples/full-config-reference.yaml`, `examples/bundled/README.md` — per-branch trims (each branch's reference file documents only its own options).
- Helm chart (`production/helm/goscape`) — the Task-8 + Task-13 changes (unified `database:` block, postgres values with `sslmode: prefer`, secret-based DSN via expand-env) applied to each branch's chart state.
- `proto/friends/friends.proto` + `make protos` regen — each branch's `PublicMessageRequest` comment updated to its own post-consolidation truth (244/245.2's current comments describe the username-keyed era; what replaces them depends on that branch's audited public_chat shape — on 244, if `public_chat` becomes account-id-keyed, the comment explains the server-side resolution rather than a session join).
- `CLAUDE.md`, `docs/PORTING.md` — per-branch module-graph/config notes and a dated DB-2-retirement entry naming the branch's TS pin and audited deltas.

### 3. Per-branch TS audit (first task of every branch)

Before any implementation on a branch, an audit task reads the branch's pinned TS sources — `src/server/friend/FriendServerRepository.ts`, `src/server/friend/FriendServer.ts` (path may differ at older pins; locate), the prisma schema, and the login-server DB surface if needed — and produces a **behavior-contract table** with, at minimum:

| Item | What the audit records |
|---|---|
| `addFriend` | cap value + members-awareness; owner/target resolution; dup handling; insert row shape |
| `addIgnore` | cap; owner resolution; target storage form (string vs id); conflict handling |
| `loadFriends` / `loadIgnores` | join shape; ORDER BY |
| `deleteFriend` / `deleteIgnore` | delete predicate shape |
| Private messages | existence-check semantics (throw/drop vs none); insert row shape |
| `public_chat` | full row shape + how identity is keyed (session_uuid / username→account_id / other) |
| `private_chat` | full row shape |
| prisma friends tables | exact model fields for friendlist/ignorelist/private_chat/public_chat |

The audit is the branch's normative appendix: the implementation plan's friends tasks cite it, implementers code against it, reviewers verify against it. Where the audit contradicts this spec's "known deltas," **the audit wins** (it read the source).

### 4. Behavior restorations per branch

Same principle as 274: consolidation removes the federation rationale, so each branch's DB-2 exception blocks (`NAI-S4A-D-FED-*` and equivalents present on that branch) are retired and the TS behaviors restored **in that branch's TS terms** — e.g. a branch whose TS PM path has the executeTakeFirstOrThrow existence check gets the silent-drop restoration; a branch whose TS predates that check does not gain one. Dual-pin tests per restored behavior, per branch.

### 5. Verification per branch (all automated)

- Full `go test ./...` + compile-all gate (`go test -run '^$' ./...`) + `go vet ./...`
- `-race` on `pkg/gamedb`, `modules/login`, `modules/friends` (CGO_ENABLED=1)
- `CGO_ENABLED=0 go build -trimpath ./...`
- Config-verify on both example files + the clean-break probe (a scratch config with a stale `sqlite_dsn` key must fail naming the key)
- `helm lint` + render checks (sqlite default unchanged shape; postgres render with DSN/env/arg; World-mode exclusion; `required` guards)
- Gated live-postgres suites (`GOSCAPE_TEST_POSTGRES_DSN` against the user's podman server; requires the sandbox network override) — validates each branch's postgres DDL for real
- Stale-reference sweep (no `sqlite_dsn`/`SQLiteDSN`/old-DB-path references outside docs/history)
- Final per-branch review before the next branch starts

### 6. Risks & mitigations

- **Audit misses a TS delta** → reviewers re-verify implementations against the TS pin directly (the 274 effort's reviewers did this and caught real details); PORTING.md records each branch's deltas for later audit trails.
- **Per-branch drift breaks a "copyable" file** → the compile-all gate after every copy step catches it; the file moves to the ADAPT column for that branch.
- **Worktree/stray-write hazards** (known failure modes) → controller verifies main-tree cleanliness after every implementer task; sequential single-implementer dispatch only.
- **`jstring` multiples-of-37 collisions** in ported tests → `distinctUsername37s` helper pattern documented in each branch's friends test task.
- **Stale-LSP diagnostics during ports** (known gotcha) → trust fresh `go test`/`go build` output, never IDE diagnostics.
