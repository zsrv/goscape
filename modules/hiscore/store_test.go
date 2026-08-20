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

	insertAccount(t, db, "zezima", 0, nil)
	insertAccount(t, db, "modash", 2, nil)      // staff — hidden
	insertAccount(t, db, "cheater", 0, &future) // banned — hidden
	insertAccount(t, db, "reformed", 0, &past)  // ban expired — visible

	tests := []struct {
		name    string
		lookup  string
		wantErr error
	}{
		{"plain account", "zezima", nil},
		{"staff hidden", "modash", ErrNotFound},
		{"active ban hidden", "cheater", ErrNotFound},
		{"expired ban visible", "reformed", nil},
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
