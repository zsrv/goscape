CREATE TABLE friendlist (
    profile TEXT NOT NULL,
    owner_username37 INTEGER NOT NULL,
    target_username37 INTEGER NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, owner_username37, target_username37)
);

CREATE INDEX idx_friendlist_target
    ON friendlist (profile, target_username37);

CREATE TABLE ignorelist (
    profile TEXT NOT NULL,
    owner_username37 INTEGER NOT NULL,
    target_username37 INTEGER NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, owner_username37, target_username37)
);
