package login

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/objtype"
)

// hiscoreTableExists reports whether a table is present in the migrated DB.
func hiscoreTableExists(t *testing.T, db *gamedb.DB, name string) bool {
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

func queryHiscoreRow(t *testing.T, db *gamedb.DB, table string, accountID, typ int) (level int, value int64, date time.Time, found bool) {
	t.Helper()
	err := db.QueryRow(
		`SELECT level, value, date FROM `+table+` WHERE account_id = ? AND type = ? AND profile = 'main'`,
		accountID, typ,
	).Scan(&level, &value, &date)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, time.Time{}, false
	}
	if err != nil {
		t.Fatalf("queryHiscoreRow(%s, type=%d): %v", table, typ, err)
	}
	return level, value, date, true
}

// statsForLevels builds an xps array where stat i has exactly the XP threshold
// for the given level (so GetLevelByExp returns that level).
func statsForLevels(levels [objtype.PlayerStatCount]int) [objtype.PlayerStatCount]int32 {
	var xps [objtype.PlayerStatCount]int32
	for i := range levels {
		xps[i] = int32(objtype.GetExpByLevel(levels[i]))
	}
	return xps
}

func TestUpdateHiscores_HappyPath(t *testing.T) {
	db := createTestDB(t)
	id := int(insertTestAccount(t, db, "hsplayer", "pw"))
	acct, err := accountByUsername(t.Context(), db, "hsplayer", "main")
	if err != nil || acct == nil {
		t.Fatalf("accountByUsername: %v acct=%v", err, acct)
	}

	// Attack(0)=level 20 (>=15 → its own hiscore row), Defence(1)=level 14
	// (<15 → excluded from hiscore but counted in the total). All others level 1.
	var levels [objtype.PlayerStatCount]int
	for i := range levels {
		levels[i] = 1
	}
	levels[objtype.PlayerStatAttack] = 20
	levels[objtype.PlayerStatDefence] = 14
	xps := statsForLevels(levels)

	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, xps), "main"); err != nil {
		t.Fatalf("updateHiscores: %v", err)
	}

	// hiscore_large type 0 = total over enabled stats.
	var wantXp int64
	var wantLevel int
	for i := range objtype.PlayerStatCount {
		if !objtype.PlayerStatEnabled[i] {
			continue
		}
		wantXp += int64(xps[i])
		wantLevel += objtype.GetLevelByExp(int(xps[i]))
	}
	lvl, val, _, found := queryHiscoreRow(t, db, "hiscore_large", id, 0)
	if !found {
		t.Fatal("hiscore_large type 0 row missing")
	}
	if val != wantXp || lvl != wantLevel {
		t.Errorf("hiscore_large: got (level=%d,value=%d), want (level=%d,value=%d)", lvl, val, wantLevel, wantXp)
	}

	// Attack (type 1) present at level 20.
	if lvl, val, _, found := queryHiscoreRow(t, db, "hiscore", id, objtype.PlayerStatAttack+1); !found || lvl != 20 || val != int64(xps[objtype.PlayerStatAttack]) {
		t.Errorf("hiscore attack: found=%v level=%d value=%d, want level=20", found, lvl, val)
	}
	// Defence (type 2) absent — level 14 < 15.
	if _, _, _, found := queryHiscoreRow(t, db, "hiscore", id, objtype.PlayerStatDefence+1); found {
		t.Error("hiscore defence: row present, want absent (level 14 < 15 gate)")
	}
}

func TestUpdateHiscores_StaffSkip(t *testing.T) {
	db := createTestDB(t)
	id := int(insertTestAccount(t, db, "staffer", "pw"))
	if _, err := db.Exec(`UPDATE account SET staff_mod_level = 2 WHERE id = ?`, id); err != nil {
		t.Fatalf("set staff level: %v", err)
	}
	acct, _ := accountByUsername(t.Context(), db, "staffer", "main")

	var levels [objtype.PlayerStatCount]int
	for i := range levels {
		levels[i] = 50
	}
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, statsForLevels(levels)), "main"); err != nil {
		t.Fatalf("updateHiscores: %v", err)
	}
	if _, _, _, found := queryHiscoreRow(t, db, "hiscore_large", id, 0); found {
		t.Error("staff (mod level 2) must be excluded from hiscores")
	}
}

// TestUpdateHiscores_BannedGate pins the 254-pin contract (TS
// LoginServer.ts:27-29 @2e3bcf43 — the banned gate 245.2 reverted is
// RE-ADDED):
//
//	if (account.banned_until !== null && new Date(account.banned_until) >= new Date()) {
//	    return;
//	}
//
// Currently-banned accounts are skipped; an EXPIRED ban exports normally.
func TestUpdateHiscores_BannedGate(t *testing.T) {
	db := createTestDB(t)

	var levels [objtype.PlayerStatCount]int
	for i := range levels {
		levels[i] = 50
	}
	xps := statsForLevels(levels)

	// Active ban (24h in the future) → skipped.
	id := int(insertTestAccount(t, db, "banned", "pw"))
	if err := setAccountBanned(t.Context(), db, "banned", time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatalf("setAccountBanned: %v", err)
	}
	acct, _ := accountByUsername(t.Context(), db, "banned", "main")
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, xps), "main"); err != nil {
		t.Fatalf("updateHiscores: %v", err)
	}
	if _, _, _, found := queryHiscoreRow(t, db, "hiscore_large", id, 0); found {
		t.Error("hiscore_large row present for actively-banned account — 2e3bcf43 skips banned accounts")
	}

	// Expired ban (24h in the past) → exported normally.
	id2 := int(insertTestAccount(t, db, "exbanned", "pw"))
	if err := setAccountBanned(t.Context(), db, "exbanned", time.Now().UTC().Add(-24*time.Hour)); err != nil {
		t.Fatalf("setAccountBanned: %v", err)
	}
	acct2, _ := accountByUsername(t.Context(), db, "exbanned", "main")
	if err := updateHiscores(t.Context(), db, acct2, makeSaveWithStats(0, xps), "main"); err != nil {
		t.Fatalf("updateHiscores: %v", err)
	}
	if _, _, _, found := queryHiscoreRow(t, db, "hiscore_large", id2, 0); !found {
		t.Error("hiscore_large row missing for expired-ban account — only ACTIVE bans skip (banned_until >= now)")
	}
}

// TestUpdateHiscores_SkipWhenEqualPreservesDate pins the conditional-upsert
// semantic: re-exporting an UNCHANGED value must not bump `date` (TS skips the
// UPDATE when value is unchanged). Independent of wall-clock: we pre-seed the
// row with a sentinel date and the exact value updateHiscores will compute.
func TestUpdateHiscores_SkipWhenEqualPreservesDate(t *testing.T) {
	db := createTestDB(t)
	id := int(insertTestAccount(t, db, "stable", "pw"))
	acct, _ := accountByUsername(t.Context(), db, "stable", "main")

	var levels [objtype.PlayerStatCount]int
	for i := range levels {
		levels[i] = 1
	}
	levels[objtype.PlayerStatAttack] = 30
	xps := statsForLevels(levels)

	// First export creates the rows.
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, xps), "main"); err != nil {
		t.Fatalf("updateHiscores #1: %v", err)
	}
	// Stamp a sentinel date onto the attack row.
	sentinel := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`UPDATE hiscore SET date = ? WHERE account_id = ? AND type = ?`, sentinel, id, objtype.PlayerStatAttack+1); err != nil {
		t.Fatalf("stamp sentinel: %v", err)
	}

	// Re-export with the SAME stats → value unchanged → WHERE clause suppresses
	// the update → sentinel date preserved.
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, xps), "main"); err != nil {
		t.Fatalf("updateHiscores #2 (unchanged): %v", err)
	}
	if _, _, date, _ := queryHiscoreRow(t, db, "hiscore", id, objtype.PlayerStatAttack+1); !date.UTC().Equal(sentinel) {
		t.Errorf("unchanged re-export bumped date: got %v, want sentinel %v", date, sentinel)
	}

	// Re-export with a CHANGED attack XP → value differs → row updated.
	levels[objtype.PlayerStatAttack] = 40
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, statsForLevels(levels)), "main"); err != nil {
		t.Fatalf("updateHiscores #3 (changed): %v", err)
	}
	if lvl, _, date, _ := queryHiscoreRow(t, db, "hiscore", id, objtype.PlayerStatAttack+1); lvl != 40 || date.UTC().Equal(sentinel) {
		t.Errorf("changed re-export did not update: level=%d date=%v", lvl, date)
	}
}
