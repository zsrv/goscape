# Account Management System & Player Portal — Design

**Date:** 2026-07-19
**Status:** Approved (brainstorm complete)
**Branch:** rev-274

## Problem

goscape has no notion of a player account distinct from a game character. Identity
today is a flat `account` table (username + bcrypt password, auto-created on first
RS2 login via the login gRPC service). There is no way to:

- require players to register before creating characters,
- group multiple characters under one owning account,
- gate character creation on a linked third-party identity (e.g. Discord) to
  raise the cost of bot-account creation,
- let admins manually approve accounts that bypass the linking requirement,
- give players any web-facing self-service surface.

## Decisions (settled during brainstorm)

1. **Placement:** built as new module(s) inside the goscape repo (not a separate
   repo/service). Full reuse of dskit modules, layered config, `pkg/gamedb`.
2. **Game login:** character name + **account** password. The RS2 login packet's
   username field carries the character name; the password is the owning
   account's portal password.
3. **Identity:** roll our own (no Keycloak/Kanidm). Rationale: the game-login
   path needs raw server-side password verification (ROPC is the IdP's worst
   feature); gating/link data lives in our DB regardless; there is exactly one
   relying party. Includes email flows (verification + self-service reset) in v1.
4. **Integration:** the login module delegates game-login auth to the account
   module via gRPC (`VerifyGameLogin`), gated behind a new `login.auth_mode`
   knob. Default `local` preserves today's behavior exactly.
5. **Gate rule (v1):** character creation allowed iff account is active AND
   email verified AND (member of `manually_approved` OR has a non-revoked linked
   identity whose provider is in the configured gate list, default `["discord"]`).
6. **Character creation:** in the portal only, with a configurable per-account
   limit (default 5).
7. **Admin surfaces:** role-gated portal admin pages AND a CLI verb group.
8. **Migration:** greenfield. No claim flow for legacy auto-created rows.
9. **Build shape:** one module, server-rendered `html/template` portal (no SPA,
   no frontend toolchain), embedded assets.
10. **Unlinking:** players can never self-unlink (anti-bot: a linked identity is
    a commitment). Admin-only unlink (soft, "burns" the identity) plus a
    separate deliberate release action (hard delete). Both audited.

## Architecture

One new module `modules/account/`, deps `common, database`, included in `all`,
runnable alone via `--target account`. Two listeners:

- **HTTP portal** (dskit server, like ondemand): SSR pages — registration,
  login, email verification, password reset, Discord linking, character
  creation, settings, and `/admin/*`.
- **gRPC `AccountService`**: `VerifyGameLogin` for the login module + admin RPCs
  for the CLI.

Game-login call chain: world → login → account, all lazy-dialed gRPC. Any
co-location or multi-host split works (same deployment story as login/friends
over a Postgres central DB).

The login module never touches `portal_*` tables; the account module never
touches game-state columns (`account_login`, `session`, bans/mutes, members,
staff level). Shared coupling is only the migration lineage and one FK.

## Data model

New central-DB migration `000003` (SQLite + Postgres variants). The existing
`account` table is untouched and remains per-character game state (username =
character name). New tables:

| Table | Columns (key) |
|---|---|
| `portal_account` | `id` PK, `email` UNIQUE (stored lowercased), `email_verified` bool, `password_hash` (argon2id PHC string), `status` (`active`/`disabled`), `created_at`, `updated_at` |
| `portal_identity` | `id` PK, `account_id` FK, `provider` (`'discord'`), `provider_user_id`, `provider_username`, `linked_at`, `revoked_at` NULL. UNIQUE(`provider`,`provider_user_id`) — one third-party identity vouches for at most one account, ever; UNIQUE(`account_id`,`provider`) |
| `portal_character` | `id` PK, `account_id` FK, `username` (game name), `game_account_id` FK → `account.id`, `created_at` |
| `portal_group` | `id` PK, `name` UNIQUE, `description`. Seeded: `manually_approved`, `admin` |
| `portal_group_member` | PK(`group_id`,`account_id`), `added_by` (nullable account id), `added_at` |
| `portal_session` | `id` PK, `token_hash` (SHA-256 of cookie token), `account_id`, `created_at`, `expires_at`, `ip`, `user_agent` |
| `portal_token` | `id` PK, `account_id`, `purpose` (`verify_email`/`reset_password`), `token_hash`, `expires_at`, `used_at` NULL, `created_at` |
| `portal_audit_log` | `id` PK, `actor_account_id` (nullable — CLI/system), `action`, `target`, `details`, `created_at` |

**Character creation is one transaction** inserting `portal_character` AND the
game `account` row (sentinel unusable password, see below). The game-side
`account.username` UNIQUE constraint is the single source of name reservation —
no race window, and legacy rows are automatically respected.

## Portal auth

- **Login identifier:** email + password. Game login is character name +
  account password (resolved `portal_character` → `portal_account`).
- **Hashing:** argon2id (`x/crypto/argon2`), PHC-encoded, params in config.
- **Password policy:** 8–20 chars, client-typable printable ASCII. The 20-char
  cap and charset exist because the account password is also typed into the
  Java client's login screen. The registration form explains this.
- **Case sensitivity:** portal passwords are case-sensitive. In `account` auth
  mode the login module forwards the client password verbatim. The TS-parity
  lowercasing quirk applies only in `local` mode (config-gated divergence —
  satisfies the fidelity gate).
- **Sessions:** 32 random bytes → base64url cookie (`HttpOnly`, `Secure`,
  `SameSite=Lax`); DB stores SHA-256. Idle TTL 7d, absolute TTL 30d (config).
  Logout deletes the row; password reset invalidates all sessions.
- **CSRF:** per-session token embedded in every state-changing form.
- **Rate limiting:** in-memory token buckets, per-IP and per-identifier, on
  login attempts, registration, and email sends.

## Email flows

Plain SMTP (host/port/from in config; STARTTLS auto-negotiated), text templates.

- **Verification:** registration creates the account immediately and sends a
  link (`portal_token`, purpose `verify_email`, 24h, single-use). SMTP failure
  never blocks registration; dashboard offers resend. Verified email is
  required before character creation.
- **Reset:** request by email → uniform "if that account exists, we sent a
  link" (no enumeration) → token (1h, single-use) → new password → all
  sessions invalidated.

## Discord linking

- OAuth2 authorization-code flow via `golang.org/x/oauth2`, scope `identify`
  only. `state` random and bound to the portal session.
- Callback inserts `portal_identity`; if the Discord identity is already linked
  (or burned) elsewhere, the UNIQUE constraint renders a friendly error.
- Provider config is table-driven (`account.providers.<name>`) so additional
  providers are additive.

### Unlinking rules (anti-bot)

- **No self-unlink**, ever, in v1 — no config knob. Settings shows the link
  with no remove control.
- **Admin unlink is soft:** sets `revoked_at`. The identity stays burned — it
  still occupies UNIQUE(`provider`,`provider_user_id`) and cannot vouch for
  another account. A revoked link no longer satisfies the gate; existing
  characters are untouched.
- **Admin release is a separate deliberate action:** hard-deletes the row,
  freeing the identity (legitimate case: player lost their old portal account).
- Both actions audited with actor, target account, provider user id.

## Gate evaluation

Single choke point in the account service, checked at character creation:

```
allowed = account.status == active
       && account.email_verified
       && characters(account) < account.character_limit
       && ( member(account, "manually_approved")
         || exists portal_identity(account, provider IN gate.providers,
                                   revoked_at IS NULL) )
```

Empty `gate.providers` ⇒ linking optional, only `manually_approved` gates
(closed-beta posture with zero code change). The gate runs only at creation
time; later unlink/revoke never disables existing characters.

## Game-login integration

New proto `proto/account/account.proto`:

```proto
service AccountService {
  rpc VerifyGameLogin(VerifyGameLoginRequest) returns (VerifyGameLoginResponse);
  // Admin RPCs (bearer-token gated): SearchAccounts, GetAccount,
  // SetGroupMembership, SetAccountStatus, UnlinkIdentity, ReleaseIdentity,
  // AdminResetPassword, BootstrapAdmin
}

message VerifyGameLoginRequest {
  string character_name = 1;  // safe-name normalized by login module
  string password = 2;        // verbatim from client (post-RSA decode)
  string remote_address = 3;  // audit / rate-limit context
}
message VerifyGameLoginResponse {
  Status status = 1;          // OK | INVALID_CREDENTIALS | ACCOUNT_DISABLED
                              // | EMAIL_UNVERIFIED | ERROR
  int64 game_account_id = 2;
  int64 portal_account_id = 3;
}
```

`login.auth_mode`:

- **`local` (default):** exactly today's behavior — bcrypt, lowercase quirk,
  `login.auto_register` honored. Zero change for existing configs and parity
  baselines.
- **`account`:** `PlayerLogin` replaces lookup-compare-autocreate with one
  `VerifyGameLogin` call. IP-ban check stays first (before the RPC). On OK it
  loads the game `account` row by `game_account_id` and continues through the
  unchanged remainder (members upgrade, ban/mute, already-logged-in/reconnect,
  hop timer, save read, `account_login` upsert). `auto_register=true` +
  `auth_mode=account` is a boot-time `Validate` error.

### Error mapping (account mode)

| VerifyGameLogin status | PlayerLogin result | Client sees |
|---|---|---|
| `INVALID_CREDENTIALS` | `LOGIN_RESULT_INVALID_CREDENTIALS` | Invalid username or password |
| `ACCOUNT_DISABLED` | `LOGIN_RESULT_ACCOUNT_DISABLED` | Your account has been disabled |
| `EMAIL_UNVERIFIED` | `LOGIN_RESULT_ACCOUNT_DISABLED` | Your account has been disabled (portal explains why) |
| `ERROR`, RPC failure, or 5s deadline | existing login-server-offline path | Login server offline |

### Sentinel password

Portal-created game `account` rows get `password = '!portal-managed!'` (not a
valid bcrypt hash). If a deployment is ever flipped back to `local` mode,
bcrypt compare against the sentinel always fails — portal characters cannot be
hijacked through the legacy path.

## Admin surfaces

**Portal `/admin/*`** (requires `admin` group; non-admins get 404; every action
audited):

- Search by email, character name, or Discord id → account detail (status,
  identities incl. revoked, characters, groups, recent audit entries).
- Actions: add/remove `manually_approved`, disable/enable account, unlink
  (burn) / release identity, resend verification, trigger reset email.
- `/admin/audit`: filterable audit log.

**CLI:** `goscape-cli account` verb group over the admin gRPC surface:
`search`, `show`, `approve`, `unapprove`, `disable`, `enable`, `unlink`,
`release-identity`, `reset-password`, `bootstrap-admin`. Admin RPCs require a
bearer token in gRPC metadata (`account.admin_token`; RPCs disabled unless
set; constant-time compare). `bootstrap-admin` promotes an existing registered
account into `admin` — solves the first-admin bootstrap; thereafter the portal
UI suffices.

## Configuration

New `account:` section (layered precedence, strict decoding as everywhere):

```yaml
account:
  enabled: true
  http_listen_address: ...   # portal
  http_listen_port: ...
  grpc_listen_address: ...   # AccountService
  grpc_listen_port: ...
  public_url: ""             # base URL for email links + OAuth redirect
  character_limit: 5
  gate:
    providers: ["discord"]
  argon2:
    memory_kib: 65536
    time: 2
    parallelism: 1
  session:
    idle_ttl: 168h
    absolute_ttl: 720h
  smtp:
    host: ...
    port: 587
    from: ...
    username: ...
    password: ...
    # STARTTLS is negotiated automatically by net/smtp whenever the
    # relay advertises it; there is deliberately no knob for it.
  providers:
    discord:
      client_id: ...
      client_secret: ...
  admin_token: ""            # empty = admin RPCs disabled

login:
  auth_mode: local           # local | account
  account_grpc_address: ""   # required when auth_mode=account (Validate)
```

`examples/full-config-reference.yaml` gains the full section at defaults;
`examples/bundled/goscape.yaml` stays `local` mode.

## Error handling

- SMTP down: registration succeeds unverified; resend from dashboard.
- Discord/OAuth failure: retry page; no state written.
- Name-taken race: unique constraint inside the creation tx → friendly error.
- Account service down: blocks game logins only in `account` mode (client sees
  login-server-offline) and portal always; nothing corrupts.
- Duplicate Discord link attempt: friendly error naming no other account.

## Testing

- **Unit:** gate policy matrix (approved / linked / revoked / unverified /
  at-limit), argon2id round-trip, password policy, character-name validation
  (reuse `util.ToSafeName` + base-37 rules), token single-use/expiry.
- **DB:** migration + query tests via the `gamedbtest` pattern, SQLite and
  Postgres both.
- **HTTP:** `httptest` — registration→verify→login→link→create-character happy
  path, CSRF rejection, session expiry, admin authz, enumeration-safe reset.
- **gRPC:** bufconn — `VerifyGameLogin` all statuses, admin-token enforcement.
- **Integration:** login module in `account` mode against in-process account
  service — full `PlayerLogin` matrix incl. error-mapping table; plus a
  `local`-mode regression run proving zero behavior change.
- **Smoke:** user-launched `--target all` + portal walk-through.

## Fidelity notes

All changes are additive and config-gated. `login.auth_mode=local` (default)
preserves TS-faithful behavior bit-for-bit, including the password-lowercasing
quirk and auto-register. Ref-parity baselines are untouched. The account
module is goscape-specific infrastructure (like healthz/Helm/telemetry), not a
game-behavior divergence.

## Out of scope (v1)

- TOTP/2FA, email change flow hardening beyond re-verification, account
  deletion/GDPR tooling.
- Discord guild-membership or role checks (gate is link-existence only; the
  portal-driven OAuth design leaves the door open).
- Player self-unlink (deliberately excluded, not deferred).
- Migration/claim flow for legacy auto-created rows.
- SPA/JSON public API; htmx progressive enhancement can be added later.
