package login

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

type accountRow struct {
	ID            int
	Username      string
	Password      string
	StaffModLevel int
	Members       int
	LoggedIn      int
	NodeID        int
	BannedUntil   sql.NullTime
	MutedUntil    sql.NullTime
	LogoutTime    sql.NullTime
	HasLoginRow   bool
}

func accountByUsername(ctx context.Context, db *gamedb.DB, username, profile string) (*accountRow, error) {
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
	err := db.QueryRowContext(ctx, db.Rebind(query), profile, username).Scan(
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

func ipBanned(ctx context.Context, db *gamedb.DB, ip string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, db.Rebind(`SELECT COUNT(*) FROM ipban WHERE ip = ?`), ip).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("ipBanned: %w", err)
	}
	return count > 0, nil
}

func insertAccount(ctx context.Context, db *gamedb.DB, username, hashedPassword, registrationIP string) (int64, error) {
	// INSERT ... RETURNING is the dialect-uniform id-retrieval form
	// (SQLite >= 3.35 and Postgres both support it; LastInsertId is
	// sqlite-only).
	var id int64
	err := db.QueryRowContext(ctx,
		db.Rebind(`INSERT INTO account (username, password, registration_ip) VALUES (?, ?, ?) RETURNING id`),
		username, hashedPassword, registrationIP,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insertAccount: %w", err)
	}
	return id, nil
}

func setAccountMembers(ctx context.Context, db *gamedb.DB, accountID int) error {
	_, err := db.ExecContext(ctx, db.Rebind(`UPDATE account SET members = 1 WHERE id = ?`), accountID)
	if err != nil {
		return fmt.Errorf("setAccountMembers: %w", err)
	}
	return nil
}

// execer is the common subset of *sql.DB and *sql.Tx used by the *Tx
// wrappers so PlayerLogin can group inserts in one transaction (DB-1).
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func upsertAccountLogin(ctx context.Context, db *gamedb.DB, accountID int, profile string, nodeID int) error {
	return upsertAccountLoginTx(ctx, db, db, accountID, profile, nodeID)
}

func upsertAccountLoginTx(ctx context.Context, db *gamedb.DB, ex execer, accountID int, profile string, nodeID int) error {
	_, err := ex.ExecContext(ctx, db.Rebind(`
		INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, 1)
		ON CONFLICT(account_id, profile) DO UPDATE SET logged_in = 1, node_id = excluded.node_id`),
		accountID, profile, nodeID,
	)
	if err != nil {
		return fmt.Errorf("upsertAccountLogin: %w", err)
	}
	return nil
}

func insertSession(ctx context.Context, db *gamedb.DB, sessionUUID string, accountID int, profile string, world, uid int, remoteAddr string) error {
	return insertSessionTx(ctx, db, db, sessionUUID, accountID, profile, world, uid, remoteAddr)
}

func insertSessionTx(ctx context.Context, db *gamedb.DB, ex execer, sessionUUID string, accountID int, profile string, world, uid int, remoteAddr string) error {
	loginTime := time.Now().UTC()
	_, err := ex.ExecContext(ctx,
		db.Rebind(`INSERT INTO session (session_uuid, account_id, profile, world, uid, login_time, remote_address) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		sessionUUID, accountID, profile, world, uid, loginTime, remoteAddr,
	)
	if err != nil {
		return fmt.Errorf("insertSession: %w", err)
	}
	return nil
}

func clearWorldSessions(ctx context.Context, db *gamedb.DB, nodeID int, profile string) error {
	_, err := db.ExecContext(ctx,
		db.Rebind(`UPDATE account_login SET logged_in = 0 WHERE node_id = ? AND profile = ?`),
		nodeID, profile,
	)
	if err != nil {
		return fmt.Errorf("clearWorldSessions: %w", err)
	}
	return nil
}

// setLoggedOut clears the account_login flag and stamps account.logout_time.
// Mirrors TS LoginServer.ts:429-440 (setLoggedOut), which sets logged_in=0 plus
// logout_time. goscape stores logout_time on the `account` table (not
// account_login as in TS) and has no `logged_out` node-id column (TS keeps it
// for bookkeeping; nothing reads it). The logout_time stamp is what arms the M25
// "save missing but logout_time set" safety reject on the next login, so the
// two writes are wrapped in a transaction to stay consistent.
//
// The UPDATE matches by (account_id, profile) only — node_id is intentionally
// excluded so a force-logout originating from a different node clears the
// row a previous world wrote (TS LoginServer.ts:438-439,484-485 likewise
// keys the clear by account/profile, not by which node currently holds it).
//
// PORTING-EXCEPTION (login-server-7): the logout_time stamp is per-ACCOUNT
// here (goscape's `account.logout_time` column is keyed by account_id
// alone) but per-PROFILE in TS (`account_login.logout_time`, keyed by
// (account_id, profile) — see the UPDATE shape at LoginServer.ts:430-440).
// For an account that logs in across more than one profile (different
// worlds / save slots), TS stamps an independent logout_time per profile;
// goscape stamps one logout_time shared by all profiles. The latent
// failure mode: profile A graceful-logs-out (stamps account.logout_time),
// profile B (same account, never logged in before so its save file does
// not exist) then attempts a first login — the M25 safety reject at
// handler.go reads the shared account.logout_time != NULL and force-
// rejects profile B with a spurious "save missing but logout_time set"
// even though profile B has a legitimate first-login posture. Closing
// this requires (i) a new migration adding account_login.logout_time
// (NULL allowed), (ii) backfilling from account.logout_time for every
// existing (account_id, profile) row, (iii) updating setLoggedOut to
// stamp account_login.logout_time instead of account.logout_time, (iv)
// rewriting the M25 safety-reject site in handler.go to read the
// per-profile column via the existing accountByUsername LEFT-JOIN, and
// (v) deciding whether to drop account.logout_time (SQLite drop-column
// is a CREATE-TABLE-COPY-DROP-RENAME dance pre-3.35) or leave it as
// legacy. Broader than a setLoggedOut-site fix and broader than any
// reasonable bundle slot. The deviation is real but only triggers on
// multi-profile accounts; single-profile deployments are unaffected.
// See docs/PORTING.md.
func setLoggedOut(ctx context.Context, db *gamedb.DB, accountID int, profile string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("setLoggedOut: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx,
		db.Rebind(`UPDATE account_login SET logged_in = 0 WHERE account_id = ? AND profile = ?`),
		accountID, profile,
	); err != nil {
		return fmt.Errorf("setLoggedOut: clear logged_in: %w", err)
	}

	logoutTime := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		db.Rebind(`UPDATE account SET logout_time = ? WHERE id = ?`),
		logoutTime, accountID,
	); err != nil {
		return fmt.Errorf("setLoggedOut: stamp logout_time: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("setLoggedOut: commit: %w", err)
	}
	committed = true
	return nil
}

// clearLoggedInFlag clears the account_login.logged_in flag WITHOUT
// stamping account.logout_time. Mirrors the TS force-logout path at
// LoginServer.ts:477-487, which writes only `logged_in:0, login_time:null`
// — distinct from the graceful logout path (LoginServer.ts:425-440) which
// also writes `logged_out:nodeId, logout_time:...`.
//
// Used by PlayerForceLogout. Stamping logout_time here would arm the M25
// "save missing but logout_time set" safety reject (handler.go:233) on
// the next login attempt — wrong for a force-logout, which is supposed
// to release the logged-in lock so the player can reconnect cleanly.
// [login-server-2]
func clearLoggedInFlag(ctx context.Context, db *gamedb.DB, accountID int, profile string) error {
	if _, err := db.ExecContext(ctx,
		db.Rebind(`UPDATE account_login SET logged_in = 0 WHERE account_id = ? AND profile = ?`),
		accountID, profile,
	); err != nil {
		return fmt.Errorf("clearLoggedInFlag: %w", err)
	}
	return nil
}

func setAccountBanned(ctx context.Context, db *gamedb.DB, username string, until time.Time) error {
	_, err := db.ExecContext(ctx,
		db.Rebind(`UPDATE account SET banned_until = ? WHERE username = ?`),
		until, username,
	)
	if err != nil {
		return fmt.Errorf("setAccountBanned: %w", err)
	}
	return nil
}

func setAccountMuted(ctx context.Context, db *gamedb.DB, username string, until time.Time) error {
	_, err := db.ExecContext(ctx,
		db.Rebind(`UPDATE account SET muted_until = ? WHERE username = ?`),
		until, username,
	)
	if err != nil {
		return fmt.Errorf("setAccountMuted: %w", err)
	}
	return nil
}
