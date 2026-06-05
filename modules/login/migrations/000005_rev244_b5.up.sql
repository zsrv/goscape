-- rev-244 B5: login-server rate limit + hop timer + messageCount +
-- dormant logger landing tables. Mirrors the 244 prisma singleworld
-- schema delta (Engine-TS prisma/singleworld/schema.prisma at 9aadcec4).
-- Spec: docs/superpowers/specs/2026-06-05-rev244-b5-server-login-db-design.md.

-- 1. Per-attempt login log (TS prisma model `login`, "attempts
-- (monitoring abuse)"). uuid is goscape's per-attempt sessionUUID — the
-- stand-in for TS's per-socket uuid (LoginServer.ts:260 `uuid: socket`),
-- same one-row-per-attempt cardinality. The composite index backs the
-- 3-in-5s window scan (LoginServer.ts:235-242).
CREATE TABLE login (
    uuid       TEXT    NOT NULL PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    world      INTEGER NOT NULL,
    timestamp  TEXT    NOT NULL,
    uid        INTEGER NOT NULL DEFAULT 0,
    ip         TEXT
);

CREATE INDEX idx_login_account_ip_time ON login (account_id, ip, timestamp);

-- 2. login-server-7 closure (steps i-iii): per-profile logged_out node
-- id + logout_time on account_login (TS prisma account_login
-- logged_out/logout_time; the 45s hop timer reads both,
-- LoginServer.ts:366-371). Re-create per the 000002/000003 precedent.
-- logged_out backfills 0 (origin node never recorded pre-244; the hop
-- timer's `!= 0` gate treats it as no-block). logout_time backfills
-- from the per-account column it replaces.
CREATE TABLE account_login_new (
    account_id  INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile     TEXT    NOT NULL,
    node_id     INTEGER NOT NULL DEFAULT 0,
    logged_in   INTEGER NOT NULL DEFAULT 0,
    logged_out  INTEGER NOT NULL DEFAULT 0,
    logout_time TEXT,
    PRIMARY KEY (account_id, profile)
);

INSERT INTO account_login_new (account_id, profile, node_id, logged_in, logged_out, logout_time)
SELECT al.account_id, al.profile, al.node_id, al.logged_in, 0, a.logout_time
FROM account_login al
JOIN account a ON a.id = al.account_id;

DROP TABLE account_login;

ALTER TABLE account_login_new RENAME TO account_login;

-- 3. login-server-7 closure (step v): drop the per-account column.
-- SQLite >= 3.35 supports DROP COLUMN for plain unindexed columns.
ALTER TABLE account DROP COLUMN logout_time;

-- 4. Message-centre tables backing getUnreadMessageCount (TS
-- Messages.ts; prisma models message_thread / message / message_status).
-- message_tag and tag are website-only (no goscape consumer) — see the
-- B5 NOT-PORTED rows in PORTING.md.
--
-- These tables deliberately carry NO foreign-key constraints: the 244
-- prisma models declare no @relation fields, so the upstream
-- prisma-generated SQL creates bare integer columns (see prisma
-- migration 20250303210826_message_centre). Same posture for the
-- dormant account_session/wealth_event tables below. The `login` table
-- above differs because this module's own convention (000003) adds
-- cascade FKs where goscape itself reads/writes.
CREATE TABLE message_thread (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    to_account_id     INTEGER,
    from_account_id   INTEGER NOT NULL,
    last_message_from INTEGER NOT NULL,
    subject           TEXT    NOT NULL,
    created           TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated           TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    messages          INTEGER NOT NULL DEFAULT 1,
    closed            TEXT,
    closed_by         INTEGER,
    marked_spam       TEXT,
    marked_spam_by    INTEGER
);

CREATE TABLE message (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL,
    sender_id  INTEGER NOT NULL,
    sender_ip  TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    created    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    edited     TEXT,
    edited_by  INTEGER,
    deleted    TEXT,
    deleted_by INTEGER
);

CREATE TABLE message_status (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL,
    account_id INTEGER NOT NULL,
    "read"     TEXT,
    deleted    TEXT
);

-- 5. Dormant logger landing tables (user decision, spec §User decisions
-- 1): account_session replaces TS session_log, wealth_event replaces
-- session_wealth (prisma models at 9aadcec4). NO Go reader or writer in
-- this public repo — the logger sink is slog-only
-- (modules/world/logger_bridge.go); the private sibling owns consumers.
CREATE TABLE account_session (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id   INTEGER NOT NULL,
    world        INTEGER NOT NULL DEFAULT 0,
    profile      TEXT    NOT NULL DEFAULT 'main',
    session_uuid TEXT    NOT NULL,
    timestamp    TEXT    NOT NULL,
    coord        INTEGER NOT NULL,
    event        TEXT    NOT NULL,
    event_type   INTEGER NOT NULL DEFAULT -1
);

CREATE TABLE wealth_event (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp         TEXT    NOT NULL,
    coord             INTEGER NOT NULL,
    world             INTEGER NOT NULL DEFAULT 0,
    profile           TEXT    NOT NULL DEFAULT 'main',
    event_type        INTEGER NOT NULL DEFAULT -1,
    account_id        INTEGER NOT NULL,
    account_session   TEXT    NOT NULL,
    account_items     TEXT    NOT NULL,
    account_value     INTEGER NOT NULL,
    recipient_id      INTEGER,
    recipient_session TEXT,
    recipient_items   TEXT,
    recipient_value   INTEGER
);

CREATE INDEX idx_wealth_event_recipient ON wealth_event (recipient_id);
