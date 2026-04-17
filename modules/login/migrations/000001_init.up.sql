CREATE TABLE account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    registration_ip TEXT NOT NULL DEFAULT '',
    staff_mod_level INTEGER NOT NULL DEFAULT 0,
    members INTEGER NOT NULL DEFAULT 0,
    banned_until TEXT,
    muted_until TEXT,
    logout_time TEXT
);

CREATE TABLE account_login (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    node_id INTEGER NOT NULL DEFAULT 0,
    logged_in INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, profile)
);

CREATE TABLE session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL,
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time TEXT NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

CREATE TABLE ipban (
    ip TEXT NOT NULL PRIMARY KEY,
    added_by TEXT NOT NULL DEFAULT '',
    added_on TEXT NOT NULL DEFAULT ''
);
