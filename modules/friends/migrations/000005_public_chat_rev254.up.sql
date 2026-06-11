-- rev-254 A3: public_chat re-key username -> session_uuid.
-- TS FriendServer.ts:286-297 @2e3bcf43 inserts the per-login session
-- UUID directly ({ session_uuid, timestamp, coord, message }) — the
-- 244-era username key (and its account_id resolution) is gone.
-- goscape keeps the profile + world columns: TS recovers them by
-- joining session_uuid against the login DB session table, which this
-- federated friends DB does not have (DB-2, db.go:21-35).
-- Pre-254 rows are keyed by username with no session mapping in this
-- DB; the legacy table is preserved for audit rather than dropped
-- (same posture as migration 000004's public_chat_legacy_225).
ALTER TABLE public_chat RENAME TO public_chat_legacy_244;

-- The old indexes follow the renamed table in SQLite, but their names
-- collide with the new indexes below. Drop them from the legacy copy
-- before creating the new table.
DROP INDEX idx_public_chat_recent;
DROP INDEX idx_public_chat_username;

CREATE TABLE public_chat (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    profile      TEXT    NOT NULL,
    world        INTEGER NOT NULL DEFAULT 0,
    session_uuid TEXT    NOT NULL,
    coord        INTEGER NOT NULL,
    message      TEXT    NOT NULL,
    created      TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_public_chat_session ON public_chat (profile, session_uuid, created);

CREATE INDEX idx_public_chat_recent ON public_chat (profile, created);
