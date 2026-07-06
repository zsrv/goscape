-- Unified central-database schema (fresh lineage; clean break from the
-- retired modules/login/migrations + modules/friends/migrations chains).
-- Spec: docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md
--
-- FK posture: account-referencing columns that goscape itself
-- reads/writes carry REFERENCES account(id) ON DELETE CASCADE — a
-- goscape extension over TS (the 9aadcec4 prisma schemas declare zero
-- @relation fields). Deliberate exceptions:
--   * ignorelist.value — raw username string, NO FK: TS addIgnore
--     (FriendServerRepository.ts:249-294) never checks the target, so
--     you can ignore usernames that don't exist.
--   * message_* / account_session / wealth_event — dormant landing
--     tables, NO FKs, mirroring their prisma-generated DDL.
--
-- DATETIME decltype is load-bearing: modernc.org/sqlite only maps
-- driver values to time.Time when sqlite3_column_decltype is
-- DATE/DATETIME/TIMESTAMP; TEXT columns scan as strings (pinned by
-- TestSQLite_TimeRoundTrip*). Principle: timestamptz in the postgres
-- file ⇔ DATETIME here.

CREATE TABLE account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    registration_ip TEXT NOT NULL DEFAULT '',
    staff_mod_level INTEGER NOT NULL DEFAULT 0,
    members INTEGER NOT NULL DEFAULT 0,
    banned_until DATETIME,
    muted_until DATETIME
);

CREATE TABLE account_login (
    account_id  INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile     TEXT    NOT NULL,
    node_id     INTEGER NOT NULL DEFAULT 0,
    logged_in   INTEGER NOT NULL DEFAULT 0,
    logged_out  INTEGER NOT NULL DEFAULT 0,
    logout_time DATETIME,
    PRIMARY KEY (account_id, profile)
);

CREATE TABLE session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL CHECK (
        session_uuid GLOB '????????-????-????-????-????????????'
    ),
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time DATETIME NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_session_account_profile ON session (account_id, profile);

CREATE TABLE ipban (
    ip TEXT NOT NULL PRIMARY KEY,
    added_by TEXT NOT NULL DEFAULT '',
    added_on TEXT NOT NULL DEFAULT ''
);

CREATE TABLE hiscore (
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       DATETIME NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);

CREATE TABLE hiscore_large (
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       DATETIME NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);

CREATE TABLE login (
    uuid       TEXT    NOT NULL PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    world      INTEGER NOT NULL,
    timestamp  DATETIME NOT NULL,
    uid        INTEGER NOT NULL DEFAULT 0,
    ip         TEXT
);

CREATE INDEX idx_login_account_ip_time ON login (account_id, ip, timestamp);

CREATE TABLE message_thread (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    to_account_id     INTEGER,
    from_account_id   INTEGER NOT NULL,
    last_message_from INTEGER NOT NULL,
    subject           TEXT    NOT NULL,
    created           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    messages          INTEGER NOT NULL DEFAULT 1,
    closed            DATETIME,
    closed_by         INTEGER,
    marked_spam       DATETIME,
    marked_spam_by    INTEGER
);

CREATE TABLE message (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL,
    sender_id  INTEGER NOT NULL,
    sender_ip  TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    created    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    edited     DATETIME,
    edited_by  INTEGER,
    deleted    DATETIME,
    deleted_by INTEGER
);

CREATE TABLE message_status (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL,
    account_id INTEGER NOT NULL,
    "read"     DATETIME,
    deleted    DATETIME
);

CREATE TABLE account_session (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id   INTEGER NOT NULL,
    world        INTEGER NOT NULL DEFAULT 0,
    profile      TEXT    NOT NULL DEFAULT 'main',
    session_uuid TEXT    NOT NULL,
    timestamp    DATETIME NOT NULL,
    coord        INTEGER NOT NULL,
    event        TEXT    NOT NULL,
    event_type   INTEGER NOT NULL DEFAULT -1
);

CREATE TABLE wealth_event (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp         DATETIME NOT NULL,
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

-- ==== friends tables: TS 9aadcec4 shape + goscape FK extensions ====

CREATE TABLE friendlist (
    profile           TEXT    NOT NULL,
    account_id        INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    friend_account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    created           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, account_id, friend_account_id)
);

-- Backs GetFollowers / IsVisibleTo reverse lookups (friend-side scan).
CREATE INDEX idx_friendlist_friend ON friendlist (profile, friend_account_id);

CREATE TABLE ignorelist (
    profile    TEXT    NOT NULL,
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    value      TEXT    NOT NULL,
    created    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, account_id, value)
);

CREATE TABLE private_chat (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile       TEXT    NOT NULL,
    timestamp     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    coord         INTEGER NOT NULL,
    to_account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    message       TEXT    NOT NULL
);

CREATE INDEX idx_private_chat_to   ON private_chat (profile, to_account_id, timestamp);
CREATE INDEX idx_private_chat_from ON private_chat (profile, account_id, timestamp);

CREATE TABLE public_chat (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile    TEXT    NOT NULL,
    world      INTEGER NOT NULL,
    timestamp  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    coord      INTEGER NOT NULL,
    message    TEXT    NOT NULL
);

CREATE INDEX idx_public_chat_account ON public_chat (profile, account_id, timestamp);
