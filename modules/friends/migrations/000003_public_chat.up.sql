CREATE TABLE public_chat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile TEXT NOT NULL,
    session_uuid TEXT NOT NULL,
    coord INTEGER NOT NULL,
    message TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_public_chat_session
    ON public_chat (profile, session_uuid, created);

CREATE INDEX idx_public_chat_recent
    ON public_chat (profile, created);
