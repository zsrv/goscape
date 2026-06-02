-- Hiscore leaderboard tables. Mirrors TS prisma models `hiscore` (value Int)
-- and `hiscore_large` (value BigInt) at Engine-TS/prisma/singleworld/schema.prisma:47-69,
-- written on graceful logout by LoginServer.updateHiscores (login-server-9 /
-- gap-db-datastruct-9). SQLite INTEGER is 64-bit, so one column shape serves
-- both the Int per-stat table and the BigInt total table. PK ordering
-- (profile, type, account_id) matches the prisma @@id.
CREATE TABLE hiscore (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);

CREATE TABLE hiscore_large (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);
