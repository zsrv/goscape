CREATE TABLE private_chat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile TEXT NOT NULL,
    from_username37 INTEGER NOT NULL,
    to_username37 INTEGER NOT NULL,
    coord INTEGER NOT NULL,
    message TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_private_chat_to
    ON private_chat (profile, to_username37, created);

CREATE INDEX idx_private_chat_from
    ON private_chat (profile, from_username37, created);
