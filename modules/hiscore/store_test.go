package hiscore

import (
	"errors"
	"testing"
	"time"
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
