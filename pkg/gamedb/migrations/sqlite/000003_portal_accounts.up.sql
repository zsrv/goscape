-- Account-management system (portal): accounts as containers for game
-- characters, third-party identity links, admin groups, web sessions.
-- Spec: docs/superpowers/specs/2026-07-19-account-management-design.md
--
-- The existing `account` table remains per-character game state; these
-- tables are the identity layer above it. Boolean-ish columns are
-- INTEGER 0/1 (matches account.members) so Go scanning is uniform
-- across dialects. DATETIME decltype is load-bearing (see 000001).

CREATE TABLE portal_account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    email_verified INTEGER NOT NULL DEFAULT 0,
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE portal_identity (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    provider_username TEXT NOT NULL DEFAULT '',
    linked_at DATETIME NOT NULL,
    -- Soft "burn": a revoked identity still occupies the UNIQUE below,
    -- so one Discord identity can never vouch for a second account.
    revoked_at DATETIME,
    UNIQUE (provider, provider_user_id),
    UNIQUE (account_id, provider)
);

CREATE TABLE portal_character (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    username TEXT NOT NULL UNIQUE,
    game_account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_portal_character_account ON portal_character (account_id);

CREATE TABLE portal_group (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT ''
);
INSERT INTO portal_group (name, description) VALUES
    ('manually_approved', 'Bypasses the linked-identity character-creation gate.'),
    ('admin', 'Grants access to portal /admin pages and admin actions.');

CREATE TABLE portal_group_member (
    group_id INTEGER NOT NULL REFERENCES portal_group(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    added_by INTEGER REFERENCES portal_account(id) ON DELETE SET NULL,
    added_at DATETIME NOT NULL,
    PRIMARY KEY (group_id, account_id)
);

CREATE TABLE portal_session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT NOT NULL UNIQUE,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_portal_session_account ON portal_session (account_id);

CREATE TABLE portal_token (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_portal_token_account ON portal_token (account_id, purpose);

CREATE TABLE portal_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_account_id INTEGER REFERENCES portal_account(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_portal_audit_created ON portal_audit_log (created_at);
