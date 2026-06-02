package login

import (
	"database/sql"
	"errors"
	"testing"
)

// hiscoreTableExists reports whether a table is present in the migrated DB.
func hiscoreTableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatalf("hiscoreTableExists(%s): %v", name, err)
	}
	return got == name
}

func TestMigrationCreatesHiscoreTables(t *testing.T) {
	db := createTestDB(t)
	for _, name := range []string{"hiscore", "hiscore_large"} {
		if !hiscoreTableExists(t, db, name) {
			t.Errorf("table %q not created by migrations", name)
		}
	}
}
