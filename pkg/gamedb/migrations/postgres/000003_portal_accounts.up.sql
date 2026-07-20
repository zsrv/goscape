-- Account-management system (portal). Mirror of the sqlite file; see
-- that file's header for design notes.
-- Spec: docs/superpowers/specs/2026-07-19-account-management-design.md

CREATE TABLE portal_account (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    email_verified INTEGER NOT NULL DEFAULT 0,
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE portal_identity (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    provider_username TEXT NOT NULL DEFAULT '',
    linked_at timestamptz NOT NULL,
    revoked_at timestamptz,
    UNIQUE (provider, provider_user_id),
    UNIQUE (account_id, provider)
);

CREATE TABLE portal_character (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    username TEXT NOT NULL UNIQUE,
    game_account_id BIGINT NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_portal_character_account ON portal_character (account_id);

CREATE TABLE portal_group (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT ''
);
INSERT INTO portal_group (name, description) VALUES
    ('manually_approved', 'Bypasses the linked-identity character-creation gate.'),
    ('admin', 'Grants access to portal /admin pages and admin actions.');

CREATE TABLE portal_group_member (
    group_id BIGINT NOT NULL REFERENCES portal_group(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    added_by BIGINT REFERENCES portal_account(id) ON DELETE SET NULL,
    added_at timestamptz NOT NULL,
    PRIMARY KEY (group_id, account_id)
);

CREATE TABLE portal_session (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_portal_session_account ON portal_session (account_id);

CREATE TABLE portal_token (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_portal_token_account ON portal_token (account_id, purpose);

CREATE TABLE portal_audit_log (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_account_id BIGINT REFERENCES portal_account(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_portal_audit_created ON portal_audit_log (created_at);
