-- DB-1 (PORTING.md Arc 18): add ON DELETE CASCADE to account_login and
-- session foreign keys, plus an index on session(account_id, profile)
-- to back the LEFT JOIN scan during auth.
--
-- SQLite cannot ALTER TABLE ADD CONSTRAINT, so we re-create each table
-- using the same pattern already established by 000002_session_uuid_check.
-- foreign_keys is OFF inside the migration transaction (golang-migrate
-- runs migrations in a tx and does not toggle the pragma), so the
-- INSERT…SELECT copies are not gated on the parent table; we still
-- preserve all rows since we copy from the live old table before drop.

-- account_login: add ON DELETE CASCADE on account_id FK.
CREATE TABLE account_login_new (
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile TEXT NOT NULL,
    node_id INTEGER NOT NULL DEFAULT 0,
    logged_in INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, profile)
);

INSERT INTO account_login_new (account_id, profile, node_id, logged_in)
SELECT account_id, profile, node_id, logged_in FROM account_login;

DROP TABLE account_login;

ALTER TABLE account_login_new RENAME TO account_login;

-- session: add ON DELETE CASCADE on account_id FK, preserving the UUID
-- CHECK constraint introduced in 000002.
CREATE TABLE session_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL CHECK (
        session_uuid = ''
        OR session_uuid GLOB '????????-????-????-????-????????????'
    ),
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time TEXT NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

INSERT INTO session_new (id, session_uuid, account_id, profile, world, uid, login_time, remote_address)
SELECT id, session_uuid, account_id, profile, world, uid, login_time, remote_address
FROM session;

DROP TABLE session;

ALTER TABLE session_new RENAME TO session;

-- Backs the LEFT JOIN in accountByUsername (db.go) which previously
-- scanned session-wide rows when looking up the per-profile login row.
-- account_login is the actual join target (idx exists via PK), but the
-- session table is also keyed by (account_id, profile) for shutdown /
-- audit lookups.
CREATE INDEX idx_session_account_profile
    ON session (account_id, profile);
