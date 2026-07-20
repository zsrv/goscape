package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// Store is the repository over the portal_* tables. All SQL is written
// with ? placeholders and passed through db.Rebind for dialect safety.
type Store struct {
	db *gamedb.DB
}

func NewStore(db *gamedb.DB) *Store { return &Store{db: db} }

// Sentinel errors. Handlers map these to friendly page messages; the
// gRPC layer maps them to response statuses.
var (
	ErrNotFound       = errors.New("account: not found")
	ErrEmailTaken     = errors.New("account: email already registered")
	ErrIdentityTaken  = errors.New("account: identity already linked to another account")
	ErrAlreadyLinked  = errors.New("account: a link for this provider already exists on this account")
	ErrNameTaken      = errors.New("account: character name already taken")
	ErrCharacterLimit = errors.New("account: character limit reached")
)

type PortalAccount struct {
	ID            int64
	Email         string
	EmailVerified bool
	PasswordHash  string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AuditEntry struct {
	ID        int64
	Actor     sql.NullInt64
	Action    string
	Target    string
	Details   string
	CreatedAt time.Time
}

// isUniqueViolation reports whether err is a UNIQUE-constraint failure
// on either backend (modernc sqlite: "UNIQUE constraint failed";
// pgx: SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "23505")
}

// NormalizeEmail is the single place email case/space normalization
// happens; every read and write path goes through it.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Store) CreateAccount(ctx context.Context, email, passwordHash string) (int64, error) {
	email = NormalizeEmail(email)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_account (email, email_verified, password_hash, status, created_at, updated_at)
		 VALUES (?, 0, ?, ?, ?, ?)`), email, passwordHash, StatusActive, now, now)
	if isUniqueViolation(err) {
		return 0, ErrEmailTaken
	}
	if err != nil {
		return 0, fmt.Errorf("account: create account: %w", err)
	}
	acct, err := s.AccountByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	return acct.ID, nil
}

func (s *Store) accountBy(ctx context.Context, where string, arg any) (*PortalAccount, error) {
	var (
		a        PortalAccount
		verified int
	)
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, email, email_verified, password_hash, status, created_at, updated_at
		 FROM portal_account WHERE `+where), arg).
		Scan(&a.ID, &a.Email, &verified, &a.PasswordHash, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("account: fetch account: %w", err)
	}
	a.EmailVerified = verified == 1
	return &a, nil
}

func (s *Store) AccountByEmail(ctx context.Context, email string) (*PortalAccount, error) {
	return s.accountBy(ctx, "email = ?", NormalizeEmail(email))
}

func (s *Store) AccountByID(ctx context.Context, id int64) (*PortalAccount, error) {
	return s.accountBy(ctx, "id = ?", id)
}

func (s *Store) updateAccount(ctx context.Context, id int64, set string, args ...any) error {
	args = append(args, time.Now().UTC(), id)
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_account SET `+set+`, updated_at = ? WHERE id = ?`), args...)
	if err != nil {
		return fmt.Errorf("account: update account: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetEmailVerified(ctx context.Context, id int64) error {
	return s.updateAccount(ctx, id, "email_verified = 1")
}

func (s *Store) SetPasswordHash(ctx context.Context, id int64, phc string) error {
	return s.updateAccount(ctx, id, "password_hash = ?", phc)
}

func (s *Store) SetAccountStatus(ctx context.Context, id int64, status string) error {
	if status != StatusActive && status != StatusDisabled {
		return fmt.Errorf("account: invalid status %q", status)
	}
	return s.updateAccount(ctx, id, "status = ?", status)
}

func (s *Store) groupID(ctx context.Context, group string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id FROM portal_group WHERE name = ?`), group).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("account: unknown group %q", group)
	}
	if err != nil {
		return 0, fmt.Errorf("account: group lookup: %w", err)
	}
	return id, nil
}

// AddGroupMember is idempotent: re-adding an existing member is a no-op.
// addedBy 0 records a NULL actor (CLI/system).
func (s *Store) AddGroupMember(ctx context.Context, group string, accountID, addedBy int64) error {
	gid, err := s.groupID(ctx, group)
	if err != nil {
		return err
	}
	var actor sql.NullInt64
	if addedBy != 0 {
		actor = sql.NullInt64{Int64: addedBy, Valid: true}
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_group_member (group_id, account_id, added_by, added_at)
		 VALUES (?, ?, ?, ?)`), gid, accountID, actor, time.Now().UTC())
	if isUniqueViolation(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("account: add group member: %w", err)
	}
	return nil
}

func (s *Store) RemoveGroupMember(ctx context.Context, group string, accountID int64) error {
	gid, err := s.groupID(ctx, group)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_group_member WHERE group_id = ? AND account_id = ?`), gid, accountID); err != nil {
		return fmt.Errorf("account: remove group member: %w", err)
	}
	return nil
}

func (s *Store) IsGroupMember(ctx context.Context, group string, accountID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT COUNT(*) FROM portal_group_member gm
		 JOIN portal_group g ON g.id = gm.group_id
		 WHERE g.name = ? AND gm.account_id = ?`), group, accountID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("account: group membership: %w", err)
	}
	return n > 0, nil
}

// AppendAudit records an admin/security event. actor 0 ⇒ NULL (system
// or CLI). Audit failures should not break the calling action — most
// call sites log-and-continue; only admin actions treat this as fatal.
func (s *Store) AppendAudit(ctx context.Context, actor int64, action, target, details string) error {
	var a sql.NullInt64
	if actor != 0 {
		a = sql.NullInt64{Int64: actor, Valid: true}
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_audit_log (actor_account_id, action, target, details, created_at)
		 VALUES (?, ?, ?, ?, ?)`), a, action, target, details, time.Now().UTC()); err != nil {
		return fmt.Errorf("account: append audit: %w", err)
	}
	return nil
}

func (s *Store) RecentAudit(ctx context.Context, limit int, target string) ([]AuditEntry, error) {
	q := `SELECT id, actor_account_id, action, target, details, created_at
	      FROM portal_audit_log`
	args := []any{}
	if target != "" {
		q += ` WHERE target = ?`
		args = append(args, target)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(q), args...)
	if err != nil {
		return nil, fmt.Errorf("account: recent audit: %w", err)
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &e.Details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("account: scan audit: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
