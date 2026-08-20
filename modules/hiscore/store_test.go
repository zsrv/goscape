package hiscore

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

func TestLookupAccountByName(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	past := testClock.Add(-24 * time.Hour)
	future := testClock.Add(24 * time.Hour)
	boundary := testClock

	insertAccount(t, db, "zezima", 0, nil)
	insertAccount(t, db, "modash", 2, nil)         // staff — hidden
	insertAccount(t, db, "cheater", 0, &future)    // banned — hidden
	insertAccount(t, db, "reformed", 0, &past)     // ban expired — visible
	insertAccount(t, db, "onthedot", 0, &boundary) // banned_until == now — hidden

	tests := []struct {
		name    string
		lookup  string
		wantErr error
	}{
		{"plain account", "zezima", nil},
		{"staff hidden", "modash", ErrNotFound},
		{"active ban hidden", "cheater", ErrNotFound},
		{"expired ban visible", "reformed", nil},
		{"banned_until exactly now hidden", "onthedot", ErrNotFound},
		{"unknown", "nobody", ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.LookupAccountByName(t.Context(), tc.lookup, testClock)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("LookupAccountByName(%q): err = %v, want %v", tc.lookup, err, tc.wantErr)
			}
			if tc.wantErr == nil && got.Username != tc.lookup {
				t.Errorf("username = %q, want %q", got.Username, tc.lookup)
			}
			if tc.wantErr == nil && got.ID == 0 {
				t.Error("ID = 0, want a real account id")
			}
		})
	}
}

// TestLookupAccountByName_UTCNormalization proves the store normalizes a
// caller-supplied now to UTC before it reaches SQL, so a caller passing
// local time (e.g. a later HTTP-layer task using plain time.Now) cannot
// skew the ban filter by its offset.
//
// The DATETIME columns are compared as text, so the bug this guards
// against isn't just "wrong instant" in the abstract — a positive UTC
// offset can push now's formatted wall-clock date past midnight while
// the stored (UTC) banned_until is still on the prior day. That flips
// the lexicographic string comparison SQL performs, so the boundary
// must straddle midnight UTC to actually exercise it: a same-day
// ±24h fixture (as in TestLookupAccountByName) would pass whether or
// not the store normalizes now, because the date component never
// crosses a day.
func TestLookupAccountByName_UTCNormalization(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	// trueNow is 30 minutes before midnight UTC.
	trueNow := time.Date(2026, 8, 19, 23, 30, 0, 0, time.UTC)
	// stillBanned is 15 minutes after trueNow (same UTC day): the ban
	// has not expired yet, so the account must stay hidden.
	stillBanned := trueNow.Add(15 * time.Minute)
	// expired is 15 minutes before trueNow: the ban has already
	// lapsed, so the account must be visible.
	expired := trueNow.Add(-15 * time.Minute)

	insertAccount(t, db, "zezima", 0, nil)
	insertAccount(t, db, "cheater", 0, &stillBanned)
	insertAccount(t, db, "reformed", 0, &expired)

	// offsetNow represents the exact same instant as trueNow, but its
	// wall clock (and therefore its formatted date) has rolled over to
	// the next day.
	offsetNow := trueNow.In(time.FixedZone("test+05", 5*60*60))
	if offsetNow.Day() == trueNow.Day() {
		t.Fatalf("test fixture is broken: offsetNow (%v) must land on a different calendar day than trueNow (%v)", offsetNow, trueNow)
	}

	for _, tc := range []struct {
		name    string
		lookup  string
		wantErr error
	}{
		{"unaffected by ban still hidden across zones", "cheater", ErrNotFound},
		{"expired ban still visible across zones", "reformed", nil},
		{"plain account still visible across zones", "zezima", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantAcct, wantErr := store.LookupAccountByName(t.Context(), tc.lookup, trueNow)
			if !errors.Is(wantErr, tc.wantErr) {
				t.Fatalf("trueNow (UTC): err = %v, want %v", wantErr, tc.wantErr)
			}

			gotAcct, gotErr := store.LookupAccountByName(t.Context(), tc.lookup, offsetNow)
			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("offsetNow (non-UTC, same instant): err = %v, want %v (must agree with trueNow)", gotErr, tc.wantErr)
			}
			if gotAcct != wantAcct {
				t.Errorf("offsetNow: account = %+v, want %+v (must agree with trueNow)", gotAcct, wantAcct)
			}
		})
	}
}

// TestPlayerCard_RanksAndTiebreaks pins the total ordering:
// value DESC, then date ASC (first to reach it wins), then account_id
// ASC. Ranks must be 1-based, unique and gapless across visible rows.
func TestPlayerCard_RanksAndTiebreaks(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	early := testClock.Add(-48 * time.Hour)
	late := testClock.Add(-1 * time.Hour)

	// Three players tied on XP; `early` beats `late`, and the pair tied
	// on both value and date falls back to account_id.
	top := insertAccount(t, db, "topper", 0, nil)     // higher XP  -> rank 1
	first := insertAccount(t, db, "firstly", 0, nil)  // tie, early  -> rank 2
	lowID := insertAccount(t, db, "aardvark", 0, nil) // tie, late   -> rank 3
	highID := insertAccount(t, db, "zulu", 0, nil)    // tie, late   -> rank 4

	const attack = 1
	insertHiscore(t, db, "hiscore", top, "main", attack, 99, 20_000_000, late)
	insertHiscore(t, db, "hiscore", first, "main", attack, 90, 10_000_000, early)
	insertHiscore(t, db, "hiscore", lowID, "main", attack, 90, 10_000_000, late)
	insertHiscore(t, db, "hiscore", highID, "main", attack, 90, 10_000_000, late)

	want := map[int64]int64{top: 1, first: 2, lowID: 3, highID: 4}
	for acctID, wantRank := range want {
		card, err := store.PlayerCard(t.Context(), "main", acctID, testClock)
		if err != nil {
			t.Fatalf("PlayerCard(%d): %v", acctID, err)
		}
		if len(card.Skills) != 1 {
			t.Fatalf("PlayerCard(%d): got %d skill entries, want 1", acctID, len(card.Skills))
		}
		if got := card.Skills[0].Rank; got != wantRank {
			t.Errorf("account %d: rank = %d, want %d", acctID, got, wantRank)
		}
	}
}

// TestPlayerCard_HiddenRowsDoNotConsumeRanks is the bug this design is
// most exposed to: if the rank subquery omits the visibility filter,
// hidden accounts silently occupy ranks and the card disagrees with the
// leaderboard.
func TestPlayerCard_HiddenRowsDoNotConsumeRanks(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	future := testClock.Add(24 * time.Hour)
	staff := insertAccount(t, db, "modash", 2, nil)
	banned := insertAccount(t, db, "cheater", 0, &future)
	player := insertAccount(t, db, "zezima", 0, nil)

	const attack = 1
	insertHiscore(t, db, "hiscore", staff, "main", attack, 99, 30_000_000, testClock)
	insertHiscore(t, db, "hiscore", banned, "main", attack, 99, 25_000_000, testClock)
	insertHiscore(t, db, "hiscore", player, "main", attack, 90, 10_000_000, testClock)

	card, err := store.PlayerCard(t.Context(), "main", player, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if got := card.Skills[0].Rank; got != 1 {
		t.Errorf("rank = %d, want 1 — two hidden rows above must not consume ranks", got)
	}
}

// TestPlayerCard_SparseSkillsAndOverall pins that per-stat rows are
// sparse (written only at base level >= 15) while overall always exists,
// and that raw x10 values survive the store untouched.
func TestPlayerCard_SparseSkillsAndOverall(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	acct := insertAccount(t, db, "zezima", 0, nil)
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 150, 5_000_000, testClock)
	insertHiscore(t, db, "hiscore", acct, "main", 1, 60, 3_000_000, testClock)
	insertHiscore(t, db, "hiscore", acct, "main", 4, 55, 2_000_000, testClock)

	card, err := store.PlayerCard(t.Context(), "main", acct, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if card.Overall == nil {
		t.Fatal("Overall = nil, want the hiscore_large row")
	}
	if card.Overall.ValueX10 != 5_000_000 {
		t.Errorf("Overall.ValueX10 = %d, want 5000000 (raw x10, undivided)", card.Overall.ValueX10)
	}
	if card.Overall.Rank != 1 {
		t.Errorf("Overall.Rank = %d, want 1", card.Overall.Rank)
	}
	if len(card.Skills) != 2 {
		t.Fatalf("got %d skill entries, want 2 — absent rows must not be synthesized", len(card.Skills))
	}
	if card.Skills[0].Type != 1 || card.Skills[1].Type != 4 {
		t.Errorf("skill types = %d,%d, want 1,4 in ascending order", card.Skills[0].Type, card.Skills[1].Type)
	}
	if card.Skills[0].ValueX10 != 3_000_000 {
		t.Errorf("Skills[0].ValueX10 = %d, want 3000000 (raw x10)", card.Skills[0].ValueX10)
	}
}

// A visible account that has never been exported has no rows at all.
func TestPlayerCard_NeverExported(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	acct := insertAccount(t, db, "freshman", 0, nil)
	card, err := store.PlayerCard(t.Context(), "main", acct, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if card.Overall != nil || len(card.Skills) != 0 {
		t.Fatalf("got overall=%v skills=%d, want an empty card", card.Overall, len(card.Skills))
	}
}

// Boards are per-profile; a row under another profile is invisible here.
func TestPlayerCard_ProfileIsolation(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	acct := insertAccount(t, db, "zezima", 0, nil)
	insertHiscore(t, db, "hiscore", acct, "beta", 1, 99, 13_000_000, testClock)

	card, err := store.PlayerCard(t.Context(), "main", acct, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if len(card.Skills) != 0 {
		t.Fatalf("got %d entries under profile main, want 0 — beta rows must not leak", len(card.Skills))
	}
}

// seedBoard inserts n visible players on one board, descending in XP so
// that account "p0" is rank 1. Returns usernames in rank order.
func seedBoard(t *testing.T, db *gamedb.DB, profile string, typ, n int) []string {
	t.Helper()
	names := make([]string, 0, n)
	table := TableForType(typ)
	for i := range n {
		name := fmt.Sprintf("p%d", i)
		id := insertAccount(t, db, name, 0, nil)
		insertHiscore(t, db, table, id, profile, typ, 99-i, int64(1_000_000-i*1000), testClock)
		names = append(names, name)
	}
	return names
}

func TestLeaderboardByOffset(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)
	names := seedBoard(t, db, "main", 1, 10)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 3, 4, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	for i, r := range rows {
		wantRank := int64(3 + i + 1)
		if r.Rank != wantRank {
			t.Errorf("row %d: rank = %d, want %d", i, r.Rank, wantRank)
		}
		if r.Username != names[3+i] {
			t.Errorf("row %d: username = %q, want %q", i, r.Username, names[3+i])
		}
	}
}

func TestLeaderboardByOffset_ExcludesHidden(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	future := testClock.Add(24 * time.Hour)
	staff := insertAccount(t, db, "modash", 2, nil)
	banned := insertAccount(t, db, "cheater", 0, &future)
	player := insertAccount(t, db, "zezima", 0, nil)

	insertHiscore(t, db, "hiscore", staff, "main", 1, 99, 30_000_000, testClock)
	insertHiscore(t, db, "hiscore", banned, "main", 1, 99, 25_000_000, testClock)
	insertHiscore(t, db, "hiscore", player, "main", 1, 90, 10_000_000, testClock)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 0, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 visible", len(rows))
	}
	if rows[0].Username != "zezima" || rows[0].Rank != 1 {
		t.Errorf("got %q at rank %d, want zezima at rank 1", rows[0].Username, rows[0].Rank)
	}
}

// The card and the leaderboard must agree on rank for the same player,
// including when hidden accounts sit above them.
func TestRankAgreement_CardVsLeaderboard(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	future := testClock.Add(24 * time.Hour)
	hidden := insertAccount(t, db, "cheater", 0, &future)
	insertHiscore(t, db, "hiscore", hidden, "main", 1, 99, 99_000_000, testClock)
	names := seedBoard(t, db, "main", 1, 5)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 0, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	for _, r := range rows {
		card, err := store.PlayerCard(t.Context(), "main", r.AccountID, testClock)
		if err != nil {
			t.Fatalf("PlayerCard(%d): %v", r.AccountID, err)
		}
		if len(card.Skills) != 1 {
			t.Fatalf("player %s: got %d entries, want 1", r.Username, len(card.Skills))
		}
		if card.Skills[0].Rank != r.Rank {
			t.Errorf("player %s: card rank %d != leaderboard rank %d",
				r.Username, card.Skills[0].Rank, r.Rank)
		}
	}
	if len(rows) != len(names) {
		t.Errorf("got %d rows, want %d", len(rows), len(names))
	}
}

func TestLeaderboardByOffset_Overall(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)
	seedBoard(t, db, "main", 0, 3)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 0, 0, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset(overall): %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows from hiscore_large, want 3", len(rows))
	}
}

func TestLeaderboardByOffset_PastEnd(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)
	seedBoard(t, db, "main", 1, 3)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 100, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows past the end, want 0 and no error", len(rows))
	}
}

// TestLeaderboardByOffset_UTCNormalization mirrors
// TestLookupAccountByName_UTCNormalization's shape, adapted to the
// leaderboard query: a still-banned account's ban must not silently
// lift just because the caller passed a non-UTC now.
//
// The banned account's ban expires shortly before UTC midnight. At the
// true UTC instant the ban is still active, so the account must stay
// off the page. A same-day offset would be vacuous here — modernc.org/
// sqlite's conn.formatTime serializes a non-UTC time.Time with its own
// offset attached, and DATETIME columns are then compared as TEXT, so
// only a scenario where the offset pushes now's formatted wall-clock
// date past midnight (while the stored, UTC banned_until is still on
// the prior day) can flip the lexicographic comparison and let a
// still-banned account leak onto the leaderboard.
func TestLeaderboardByOffset_UTCNormalization(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	// trueNow is 30 minutes before midnight UTC.
	trueNow := time.Date(2026, 8, 19, 23, 30, 0, 0, time.UTC)
	// stillBanned expires 20 minutes after trueNow (23:50 UTC, still
	// shortly before midnight): the ban is still active at trueNow, so
	// the account must stay off the page.
	stillBanned := trueNow.Add(20 * time.Minute)

	banned := insertAccount(t, db, "cheater", 0, &stillBanned)
	player := insertAccount(t, db, "zezima", 0, nil)

	const attack = 1
	insertHiscore(t, db, "hiscore", banned, "main", attack, 99, 50_000_000, trueNow)
	insertHiscore(t, db, "hiscore", player, "main", attack, 90, 10_000_000, trueNow)

	// offsetNow represents the exact same instant as trueNow, but its
	// wall clock (and therefore its formatted date) has rolled over to
	// the next day.
	offsetNow := trueNow.In(time.FixedZone("test+05", 5*60*60))
	if offsetNow.Day() == trueNow.Day() {
		t.Fatalf("test fixture is broken: offsetNow (%v) must land on a different calendar day than trueNow (%v)", offsetNow, trueNow)
	}

	wantRows, err := store.LeaderboardByOffset(t.Context(), "main", attack, 0, 10, trueNow)
	if err != nil {
		t.Fatalf("LeaderboardByOffset(trueNow): %v", err)
	}
	if len(wantRows) != 1 || wantRows[0].Username != "zezima" {
		t.Fatalf("LeaderboardByOffset(trueNow): got %v, want only zezima — still-banned cheater must be absent", wantRows)
	}

	gotRows, err := store.LeaderboardByOffset(t.Context(), "main", attack, 0, 10, offsetNow)
	if err != nil {
		t.Fatalf("LeaderboardByOffset(offsetNow): %v", err)
	}
	if len(gotRows) != 1 || gotRows[0].Username != "zezima" {
		t.Fatalf("LeaderboardByOffset(offsetNow): got %v, want only zezima (must agree with trueNow) — still-banned cheater must not leak onto the page", gotRows)
	}
}

// TestPlayerCard_UTCNormalization mirrors
// TestLookupAccountByName_UTCNormalization's shape, adapted to the
// player-card rank subquery: a competitor's ban must not silently lift
// just because the caller passed a non-UTC now.
//
// The competitor's ban expires shortly before UTC midnight. At the true
// UTC instant, the ban is still active, so the competitor's (higher-XP)
// row must stay invisible and must not consume the target's rank 1. If
// PlayerCard failed to normalize now to UTC, a positive-offset now whose
// wall-clock date has already rolled to the next day would format as
// TEXT that lexicographically sorts past the competitor's banned_until
// — even though the true instant is still before it — making the ban
// spuriously appear expired and letting the competitor's row count,
// demoting the target to rank 2.
func TestPlayerCard_UTCNormalization(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	// trueNow is 30 minutes before midnight UTC.
	trueNow := time.Date(2026, 8, 19, 23, 30, 0, 0, time.UTC)
	// competitorBan expires 20 minutes after trueNow (23:50 UTC, still
	// shortly before midnight): the ban is still active at trueNow, so
	// the competitor must stay hidden.
	competitorBan := trueNow.Add(20 * time.Minute)

	target := insertAccount(t, db, "zezima", 0, nil)
	competitor := insertAccount(t, db, "cheater", 0, &competitorBan)

	const attack = 1
	insertHiscore(t, db, "hiscore", target, "main", attack, 90, 10_000_000, trueNow)
	insertHiscore(t, db, "hiscore", competitor, "main", attack, 99, 50_000_000, trueNow)

	// offsetNow represents the exact same instant as trueNow, but its
	// wall clock (and therefore its formatted date) has rolled over to
	// the next day.
	offsetNow := trueNow.In(time.FixedZone("test+05", 5*60*60))
	if offsetNow.Day() == trueNow.Day() {
		t.Fatalf("test fixture is broken: offsetNow (%v) must land on a different calendar day than trueNow (%v)", offsetNow, trueNow)
	}

	wantCard, err := store.PlayerCard(t.Context(), "main", target, trueNow)
	if err != nil {
		t.Fatalf("PlayerCard(trueNow): %v", err)
	}
	if wantCard.Skills[0].Rank != 1 {
		t.Fatalf("PlayerCard(trueNow): rank = %d, want 1 — still-banned competitor must not count", wantCard.Skills[0].Rank)
	}

	gotCard, err := store.PlayerCard(t.Context(), "main", target, offsetNow)
	if err != nil {
		t.Fatalf("PlayerCard(offsetNow): %v", err)
	}
	if gotCard.Skills[0].Rank != wantCard.Skills[0].Rank {
		t.Errorf("PlayerCard(offsetNow): rank = %d, want %d (must agree with trueNow)", gotCard.Skills[0].Rank, wantCard.Skills[0].Rank)
	}
}
