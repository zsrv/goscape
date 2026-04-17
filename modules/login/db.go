package login

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const dbTimeFormat = "2006-01-02 15:04:05"

type accountRow struct {
	ID            int
	Username      string
	Password      string
	StaffModLevel int
	Members       int
	LoggedIn      int
	NodeID        int
	BannedUntil   sql.NullString
	MutedUntil    sql.NullString
	LogoutTime    sql.NullString
	HasLoginRow   bool
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma journal_mode: %w", err)
	}
	if _, err = db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma foreign_keys: %w", err)
	}
	return db, nil
}

func accountByUsername(ctx context.Context, db *sql.DB, username, profile string) (*accountRow, error) {
	const query = `
SELECT a.id, a.username, a.password, a.staff_mod_level, a.members,
       a.banned_until, a.muted_until, a.logout_time,
       COALESCE(al.logged_in, 0),
       COALESCE(al.node_id, 0),
       CASE WHEN al.account_id IS NOT NULL THEN 1 ELSE 0 END as has_login_row
FROM account a
LEFT JOIN account_login al ON al.account_id = a.id AND al.profile = ?
WHERE a.username = ?`

	row := &accountRow{}
	var hasLoginRow int
	err := db.QueryRowContext(ctx, query, profile, username).Scan(
		&row.ID,
		&row.Username,
		&row.Password,
		&row.StaffModLevel,
		&row.Members,
		&row.BannedUntil,
		&row.MutedUntil,
		&row.LogoutTime,
		&row.LoggedIn,
		&row.NodeID,
		&hasLoginRow,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("accountByUsername: %w", err)
	}
	row.HasLoginRow = hasLoginRow == 1
	return row, nil
}

func ipBanned(ctx context.Context, db *sql.DB, ip string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ipban WHERE ip = ?`, ip).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("ipBanned: %w", err)
	}
	return count > 0, nil
}

func insertAccount(ctx context.Context, db *sql.DB, username, hashedPassword, registrationIP string) (int64, error) {
	result, err := db.ExecContext(ctx,
		`INSERT INTO account (username, password, registration_ip) VALUES (?, ?, ?)`,
		username, hashedPassword, registrationIP,
	)
	if err != nil {
		return 0, fmt.Errorf("insertAccount: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insertAccount: last insert id: %w", err)
	}
	return id, nil
}

func setAccountMembers(ctx context.Context, db *sql.DB, accountID int) error {
	_, err := db.ExecContext(ctx, `UPDATE account SET members = 1 WHERE id = ?`, accountID)
	if err != nil {
		return fmt.Errorf("setAccountMembers: %w", err)
	}
	return nil
}

func upsertAccountLogin(ctx context.Context, db *sql.DB, accountID int, profile string, hasLoginRow bool, nodeID int) error {
	var err error
	if hasLoginRow {
		_, err = db.ExecContext(ctx,
			`UPDATE account_login SET logged_in = 1, node_id = ? WHERE account_id = ? AND profile = ?`,
			nodeID, accountID, profile,
		)
	} else {
		_, err = db.ExecContext(ctx,
			`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, 1)`,
			accountID, profile, nodeID,
		)
	}
	if err != nil {
		return fmt.Errorf("upsertAccountLogin: %w", err)
	}
	return nil
}

func insertSession(ctx context.Context, db *sql.DB, sessionUUID string, accountID int, profile string, world, uid int, remoteAddr string) error {
	loginTime := time.Now().UTC().Format(dbTimeFormat)
	_, err := db.ExecContext(ctx,
		`INSERT INTO session (session_uuid, account_id, profile, world, uid, login_time, remote_address) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionUUID, accountID, profile, world, uid, loginTime, remoteAddr,
	)
	if err != nil {
		return fmt.Errorf("insertSession: %w", err)
	}
	return nil
}

func clearWorldSessions(ctx context.Context, db *sql.DB, nodeID int, profile string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE account_login SET logged_in = 0 WHERE node_id = ? AND profile = ?`,
		nodeID, profile,
	)
	if err != nil {
		return fmt.Errorf("clearWorldSessions: %w", err)
	}
	return nil
}

func setLoggedOut(ctx context.Context, db *sql.DB, accountID int, profile string, nodeID int) error {
	_, err := db.ExecContext(ctx,
		`UPDATE account_login SET logged_in = 0 WHERE account_id = ? AND profile = ? AND node_id = ?`,
		accountID, profile, nodeID,
	)
	if err != nil {
		return fmt.Errorf("setLoggedOut: %w", err)
	}
	return nil
}

func setAccountBanned(ctx context.Context, db *sql.DB, username string, until time.Time) error {
	_, err := db.ExecContext(ctx,
		`UPDATE account SET banned_until = ? WHERE username = ?`,
		until.Format(dbTimeFormat), username,
	)
	if err != nil {
		return fmt.Errorf("setAccountBanned: %w", err)
	}
	return nil
}

func setAccountMuted(ctx context.Context, db *sql.DB, username string, until time.Time) error {
	_, err := db.ExecContext(ctx,
		`UPDATE account SET muted_until = ? WHERE username = ?`,
		until.Format(dbTimeFormat), username,
	)
	if err != nil {
		return fmt.Errorf("setAccountMuted: %w", err)
	}
	return nil
}
