-- Tighten session.session_uuid: enforce UUID-shape-or-empty at the
-- schema level. Pre-slice-7 rows hold RemoteAddr().String() (e.g.
-- "127.0.0.1:42193") in this column; that same value lives in the
-- separate remote_address column, so coercing session_uuid to '' on
-- legacy rows loses no audit data. Going forward, insertSession
-- (slice 7) only writes uuid.NewString() values so the CHECK is
-- defensive against future regressions.

CREATE TABLE session_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL CHECK (
        session_uuid = ''
        OR session_uuid GLOB '????????-????-????-????-????????????'
    ),
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time TEXT NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

INSERT INTO session_new (id, session_uuid, account_id, profile, world, uid, login_time, remote_address)
SELECT
    id,
    CASE
        WHEN session_uuid GLOB '????????-????-????-????-????????????' THEN session_uuid
        ELSE ''
    END,
    account_id, profile, world, uid, login_time, remote_address
FROM session;

DROP TABLE session;

ALTER TABLE session_new RENAME TO session;
