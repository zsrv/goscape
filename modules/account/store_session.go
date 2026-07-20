package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Email-token purposes (portal_token.purpose).
const (
	TokenPurposeVerifyEmail   = "verify_email"
	TokenPurposeResetPassword = "reset_password"
)

// NewRawToken returns 32 random bytes base64url-encoded — the value
// that travels in a cookie or email link. Only its hash is stored.
func NewRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("account: token entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken is the storage form of a raw token: hex(sha256(raw)). A DB
// leak therefore does not leak usable session/reset tokens.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateSession(ctx context.Context, accountID int64, tokenHash, ip, userAgent string, cfg SessionConfig) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_session (token_hash, account_id, created_at, expires_at, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`),
		tokenHash, accountID, now, now.Add(cfg.IdleTTL), ip, userAgent); err != nil {
		return fmt.Errorf("account: create session: %w", err)
	}
	return nil
}

// SessionAccount validates a session token hash and returns the owning
// account. A hit slides the idle expiry forward, clamped to the
// absolute expiry (created_at + AbsoluteTTL).
func (s *Store) SessionAccount(ctx context.Context, tokenHash string, cfg SessionConfig) (*PortalAccount, error) {
	var (
		accountID int64
		createdAt time.Time
		expiresAt time.Time
	)
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT account_id, created_at, expires_at FROM portal_session WHERE token_hash = ?`), tokenHash).
		Scan(&accountID, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("account: session lookup: %w", err)
	}
	now := time.Now().UTC()
	absolute := createdAt.Add(cfg.AbsoluteTTL)
	if now.After(expiresAt) || now.After(absolute) {
		// Expired: clean up eagerly and report missing.
		_, _ = s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM portal_session WHERE token_hash = ?`), tokenHash)
		return nil, ErrNotFound
	}
	newExpiry := now.Add(cfg.IdleTTL)
	if newExpiry.After(absolute) {
		newExpiry = absolute
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_session SET expires_at = ? WHERE token_hash = ?`), newExpiry, tokenHash); err != nil {
		return nil, fmt.Errorf("account: session touch: %w", err)
	}
	return s.AccountByID(ctx, accountID)
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_session WHERE token_hash = ?`), tokenHash); err != nil {
		return fmt.Errorf("account: delete session: %w", err)
	}
	return nil
}

// DeleteAccountSessions implements "log out everywhere" (used by
// password reset and admin disable).
func (s *Store) DeleteAccountSessions(ctx context.Context, accountID int64) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_session WHERE account_id = ?`), accountID); err != nil {
		return fmt.Errorf("account: delete account sessions: %w", err)
	}
	return nil
}

func (s *Store) CreateToken(ctx context.Context, accountID int64, purpose, tokenHash string, ttl time.Duration) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_token (account_id, purpose, token_hash, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`), accountID, purpose, tokenHash, now.Add(ttl), now); err != nil {
		return fmt.Errorf("account: create token: %w", err)
	}
	return nil
}

// ConsumeToken atomically marks a live token used and returns its
// account. The UPDATE's WHERE clause is the single-use guarantee — a
// second consume matches zero rows.
func (s *Store) ConsumeToken(ctx context.Context, purpose, tokenHash string) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_token SET used_at = ?
		 WHERE purpose = ? AND token_hash = ? AND used_at IS NULL AND expires_at > ?`),
		now, purpose, tokenHash, now)
	if err != nil {
		return 0, fmt.Errorf("account: consume token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	var accountID int64
	if err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT account_id FROM portal_token WHERE purpose = ? AND token_hash = ?`),
		purpose, tokenHash).Scan(&accountID); err != nil {
		return 0, fmt.Errorf("account: consumed token lookup: %w", err)
	}
	return accountID, nil
}
