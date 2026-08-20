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
//   - the rank subquery in cardQuery, below
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

// Entry is one board row belonging to a specific player.
type Entry struct {
	Type      int
	Rank      int64
	Level     int
	ValueX10  int64 // raw fixed-point tenths, exactly as stored
	UpdatedAt time.Time
}

// Card is a player's full hiscore standing. Skills is sparse: it holds
// only the stats that actually have rows (the write path exports a stat
// only at base level >= 15), in ascending type order. Overall is nil
// when the player has never been exported.
type Card struct {
	Account Account
	Overall *Entry
	Skills  []Entry
}

// cardQuery ranks every row a player holds in one table. The correlated
// subquery counts visible rows strictly ahead under the total ordering
// (value DESC, date ASC, account_id ASC), so rank is 1-based and unique.
//
// The subquery repeats the visibility filter deliberately: without it,
// hidden accounts would consume ranks here but not on the leaderboard,
// and the two endpoints would disagree.
//
// %[1]s is the table name, always from TableForType (a compile-time
// constant), never from request input.
//
// Placeholder order: (1) visibility cutoff inside the subquery,
// (2) profile, (3) account_id.
const cardQuery = `
SELECT h.type, h.level, h.value, h.date,
       1 + (SELECT COUNT(*)
              FROM %[1]s r
              JOIN account ra ON ra.id = r.account_id
             WHERE r.profile = h.profile
               AND r.type = h.type
               AND ra.staff_mod_level <= 1
               AND (ra.banned_until IS NULL OR ra.banned_until < ?)
               AND (r.value > h.value
                 OR (r.value = h.value
                     AND (r.date < h.date
                       OR (r.date = h.date AND r.account_id < h.account_id)))))
  FROM %[1]s h
 WHERE h.profile = ? AND h.account_id = ?
 ORDER BY h.type ASC`

// PlayerCard returns the player's overall standing plus every per-stat
// row they hold, each with its rank. Two queries — one per table —
// rather than one per stat.
//
// now is normalized to UTC once here, before either query runs; see
// LookupAccountByName for why an un-normalized now is unsafe against
// TEXT-stored DATETIME columns.
func (s *Store) PlayerCard(ctx context.Context, profile string, accountID int64, now time.Time) (Card, error) {
	now = now.UTC()

	card := Card{}

	overall, err := s.entriesFor(ctx, "hiscore_large", profile, accountID, now)
	if err != nil {
		return Card{}, err
	}
	if len(overall) > 0 {
		card.Overall = &overall[0]
	}

	skills, err := s.entriesFor(ctx, "hiscore", profile, accountID, now)
	if err != nil {
		return Card{}, err
	}
	card.Skills = skills
	return card, nil
}

// entriesFor runs cardQuery against one table. table must be a
// compile-time constant. now must already be normalized to UTC — the
// only caller, PlayerCard, does that once before calling entriesFor
// twice, rather than normalizing it again per table here.
func (s *Store) entriesFor(ctx context.Context, table, profile string, accountID int64, now time.Time) ([]Entry, error) {
	q := fmt.Sprintf(cardQuery, table)
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(q), now, profile, accountID)
	if err != nil {
		return nil, fmt.Errorf("hiscore: card query %s: %w", table, err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Type, &e.Level, &e.ValueX10, &e.UpdatedAt, &e.Rank); err != nil {
			return nil, fmt.Errorf("hiscore: card scan %s: %w", table, err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hiscore: card rows %s: %w", table, err)
	}
	return out, nil
}
