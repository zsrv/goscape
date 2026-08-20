package hiscore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// ErrNotFound means the requested account or board row is absent, or is
// hidden by the visibility rules. The two are deliberately
// indistinguishable to callers: a hidden account must not be
// detectable through the API.
var ErrNotFound = errors.New("hiscore: not found")

// Store is the module's only SQL surface.
type Store struct{ db *gamedb.DB }

func NewStore(db *gamedb.DB) *Store { return &Store{db: db} }

// Account is a visible account resolved from its safe name.
type Account struct {
	ID       int64
	Username string
}

// Visibility rule — stated once here, restated inline by every query in
// this file that must honour it:
//
//	an account's rows are visible iff staff_mod_level <= 1
//	AND (banned_until IS NULL OR banned_until < now)
//
// modules/login's updateHiscores applies the same rule, but only at
// logout. Applying it again at read time is what makes a ban take effect
// on the boards immediately, rather than at the offender's next logout.
//
// Query sites that MUST carry this predicate — if one drifts, ranks
// disagree between endpoints:
//   - LookupAccountByName, below
//   - the rank subquery in the player-card query (a later change)
//   - the shared leaderboard SELECT (a later change)
//
// Drift is caught by TestPlayerCard_HiddenRowsDoNotConsumeRanks,
// TestLeaderboardByOffset_ExcludesHidden and
// TestRankAgreement_CardVsLeaderboard, all added by later changes.

// LookupAccountByName resolves a base37 safe name to a visible account.
// The caller normalizes the name (jstring.ToSafeName) before calling.
//
// now is normalized to UTC before it reaches SQL: DATETIME columns are
// stored in UTC (the repo-wide convention — see modules/account and
// modules/login), and this store does not trust callers to have done
// the normalization themselves. A local-offset now compared against a
// UTC banned_until can misjudge the ban filter by the offset.
func (s *Store) LookupAccountByName(ctx context.Context, safeName string, now time.Time) (Account, error) {
	now = now.UTC()

	const q = `SELECT a.id, a.username
	  FROM account a
	 WHERE a.username = ?
	   AND a.staff_mod_level <= 1
	   AND (a.banned_until IS NULL OR a.banned_until < ?)`

	var acct Account
	err := s.db.QueryRowContext(ctx, s.db.Rebind(q), safeName, now).Scan(&acct.ID, &acct.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("hiscore: lookup account: %w", err)
	}
	return acct, nil
}
