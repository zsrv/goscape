package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

type Identity struct {
	ID               int64
	AccountID        int64
	Provider         string
	ProviderUserID   string
	ProviderUsername string
	LinkedAt         time.Time
	RevokedAt        sql.NullTime
}

type Character struct {
	ID            int64
	AccountID     int64
	GameAccountID int64
	Username      string
	CreatedAt     time.Time
}

// NormalizeCharacterName lowercases, maps spaces to underscores, and
// enforces the RS2 name rules (1-12 chars of [a-z0-9_], round-trips
// through jstring.ToSafeName). The returned name is what gets stored
// and what the game client must type at login.
func NormalizeCharacterName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, " ", "_")
	if name == "" || len(name) > 12 {
		return "", fmt.Errorf("character name must be 1-12 characters")
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return "", fmt.Errorf("character name may only contain letters, numbers, spaces and underscores")
		}
	}
	if safe := jstring.ToSafeName(name); safe != name {
		return "", fmt.Errorf("character name %q is not a valid game name", raw)
	}
	return name, nil
}

// LinkIdentity attaches a third-party identity. Two invariants, both
// enforced by migration-000003 UNIQUE constraints and mapped to
// sentinel errors here:
//   - (provider, provider_user_id) is globally unique — one Discord
//     identity vouches for at most one portal account, ever (a revoked
//     row still occupies the slot: the anti-bot "burn").
//   - (account_id, provider) is unique — one link per provider.
func (s *Store) LinkIdentity(ctx context.Context, accountID int64, provider, providerUserID, providerUsername string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_identity (account_id, provider, provider_user_id, provider_username, linked_at)
		 VALUES (?, ?, ?, ?, ?)`), accountID, provider, providerUserID, providerUsername, time.Now().UTC())
	if isUniqueViolation(err) {
		// Disambiguate which constraint fired.
		if existing, lookErr := s.IdentityByProviderUser(ctx, provider, providerUserID); lookErr == nil && existing.AccountID != accountID {
			return ErrIdentityTaken
		}
		return ErrAlreadyLinked
	}
	if err != nil {
		return fmt.Errorf("account: link identity: %w", err)
	}
	return nil
}

func (s *Store) IdentityByProviderUser(ctx context.Context, provider, providerUserID string) (*Identity, error) {
	var id Identity
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, account_id, provider, provider_user_id, provider_username, linked_at, revoked_at
		 FROM portal_identity WHERE provider = ? AND provider_user_id = ?`), provider, providerUserID).
		Scan(&id.ID, &id.AccountID, &id.Provider, &id.ProviderUserID, &id.ProviderUsername, &id.LinkedAt, &id.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("account: identity lookup: %w", err)
	}
	return &id, nil
}

func (s *Store) IdentitiesByAccount(ctx context.Context, accountID int64) ([]Identity, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(
		`SELECT id, account_id, provider, provider_user_id, provider_username, linked_at, revoked_at
		 FROM portal_identity WHERE account_id = ? ORDER BY provider`), accountID)
	if err != nil {
		return nil, fmt.Errorf("account: identities: %w", err)
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var id Identity
		if err := rows.Scan(&id.ID, &id.AccountID, &id.Provider, &id.ProviderUserID, &id.ProviderUsername, &id.LinkedAt, &id.RevokedAt); err != nil {
			return nil, fmt.Errorf("account: scan identity: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RevokeIdentity soft-deletes ("burns") a link: the row keeps occupying
// the (provider, provider_user_id) UNIQUE slot but no longer satisfies
// the gate. Admin-only operation; the caller audits it.
func (s *Store) RevokeIdentity(ctx context.Context, accountID int64, provider string) error {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_identity SET revoked_at = ? WHERE account_id = ? AND provider = ? AND revoked_at IS NULL`),
		time.Now().UTC(), accountID, provider)
	if err != nil {
		return fmt.Errorf("account: revoke identity: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReleaseIdentity hard-deletes a link, freeing the third-party identity
// for use on another account (support flow: player lost their old
// portal account). Admin-only; the caller audits it.
func (s *Store) ReleaseIdentity(ctx context.Context, provider, providerUserID string) error {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_identity WHERE provider = ? AND provider_user_id = ?`), provider, providerUserID)
	if err != nil {
		return fmt.Errorf("account: release identity: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GateEligible implements the spec's character-creation gate clause:
// member of manually_approved OR a non-revoked identity whose provider
// is in providers. It does NOT check status/verified/limit — the
// creation path (portal handler / admin RPC) composes those.
func (s *Store) GateEligible(ctx context.Context, accountID int64, providers []string) (bool, error) {
	approved, err := s.IsGroupMember(ctx, GroupManuallyApproved, accountID)
	if err != nil {
		return false, err
	}
	if approved {
		return true, nil
	}
	if len(providers) == 0 {
		return false, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(providers)), ",")
	args := []any{accountID}
	for _, p := range providers {
		args = append(args, p)
	}
	var n int
	err = s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT COUNT(*) FROM portal_identity
		 WHERE account_id = ? AND revoked_at IS NULL AND provider IN (`+placeholders+`)`), args...).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("account: gate query: %w", err)
	}
	return n > 0, nil
}

// CreateCharacter reserves the name and creates BOTH rows in one
// transaction: the game `account` row (with the unusable sentinel
// password — auth is delegated in account mode) and the
// portal_character row pointing at it. The game account.username
// UNIQUE constraint is the single source of name reservation, so
// legacy rows are automatically respected. name must already be
// normalized via NormalizeCharacterName.
func (s *Store) CreateCharacter(ctx context.Context, accountID int64, name string, limit int) (Character, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Character{}, fmt.Errorf("account: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var n int
	if err := tx.QueryRowContext(ctx, s.db.Rebind(
		`SELECT COUNT(*) FROM portal_character WHERE account_id = ?`), accountID).Scan(&n); err != nil {
		return Character{}, fmt.Errorf("account: count characters: %w", err)
	}
	if n >= limit {
		return Character{}, ErrCharacterLimit
	}

	if _, err := tx.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO account (username, password, registration_ip) VALUES (?, ?, 'portal')`),
		name, SentinelGamePassword); err != nil {
		if isUniqueViolation(err) {
			return Character{}, ErrNameTaken
		}
		return Character{}, fmt.Errorf("account: insert game account: %w", err)
	}
	var gameID int64
	if err := tx.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id FROM account WHERE username = ?`), name).Scan(&gameID); err != nil {
		return Character{}, fmt.Errorf("account: game account id: %w", err)
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_character (account_id, username, game_account_id, created_at)
		 VALUES (?, ?, ?, ?)`), accountID, name, gameID, now); err != nil {
		if isUniqueViolation(err) {
			return Character{}, ErrNameTaken
		}
		return Character{}, fmt.Errorf("account: insert character: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Character{}, fmt.Errorf("account: commit: %w", err)
	}
	committed = true

	var ch Character
	err = s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, account_id, game_account_id, username, created_at
		 FROM portal_character WHERE username = ?`), name).
		Scan(&ch.ID, &ch.AccountID, &ch.GameAccountID, &ch.Username, &ch.CreatedAt)
	if err != nil {
		return Character{}, fmt.Errorf("account: reload character: %w", err)
	}
	return ch, nil
}

func (s *Store) CharactersByAccount(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(
		`SELECT id, account_id, game_account_id, username, created_at
		 FROM portal_character WHERE account_id = ? ORDER BY id`), accountID)
	if err != nil {
		return nil, fmt.Errorf("account: characters: %w", err)
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		var ch Character
		if err := rows.Scan(&ch.ID, &ch.AccountID, &ch.GameAccountID, &ch.Username, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("account: scan character: %w", err)
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// CharacterWithAccount resolves a game login: character name → owning
// portal account. Used by VerifyGameLogin.
func (s *Store) CharacterWithAccount(ctx context.Context, username string) (*Character, *PortalAccount, error) {
	var ch Character
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, account_id, game_account_id, username, created_at
		 FROM portal_character WHERE username = ?`), username).
		Scan(&ch.ID, &ch.AccountID, &ch.GameAccountID, &ch.Username, &ch.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("account: character lookup: %w", err)
	}
	acct, err := s.AccountByID(ctx, ch.AccountID)
	if err != nil {
		return nil, nil, err
	}
	return &ch, acct, nil
}
