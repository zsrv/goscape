-- rev-244: public_chat re-key session_uuid -> username (+ world).
-- TS FriendServer.ts:287-305 / prisma model public_chat
-- (account_id + profile + world); goscape stores the username directly —
-- the friends DB has no account table (DB-2 federation), so TS's
-- username->account_id resolution has no landing site (PORTING.md §B5).
-- Pre-244 rows are keyed by session_uuid with no username mapping in
-- this DB; the legacy table is preserved for audit rather than dropped.
ALTER TABLE public_chat RENAME TO public_chat_legacy_225;

-- The old indexes follow the renamed table in SQLite, but their names
-- collide with the new indexes below. Drop them from the legacy copy
-- before creating the new table.
DROP INDEX idx_public_chat_recent;
DROP INDEX idx_public_chat_session;

CREATE TABLE public_chat (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    profile  TEXT    NOT NULL,
    world    INTEGER NOT NULL DEFAULT 0,
    username TEXT    NOT NULL,
    coord    INTEGER NOT NULL,
    message  TEXT    NOT NULL,
    created  TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_public_chat_username ON public_chat (profile, username, created);

CREATE INDEX idx_public_chat_recent ON public_chat (profile, created);
