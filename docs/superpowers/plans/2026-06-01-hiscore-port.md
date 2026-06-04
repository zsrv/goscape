# Hiscore Write-Path Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `LoginServer.updateHiscores` — export a logged-out player's enabled-stat XP/levels into two new SQLite leaderboard tables — closing audit IDs `login-server-9` / `gap-db-datastruct-9`.

**Architecture:** A new migration adds `hiscore` (per-stat) and `hiscore_large` (total) tables. The login module re-parses the logout save blob for the 21 stat XP values (TS re-loads the save the same way), then upserts the rows in one transaction using SQLite conditional upsert. `PlayerLogout` calls the export best-effort after `setLoggedOut`, exactly where TS does.

**Tech Stack:** Go, `database/sql` + `modernc.org/sqlite`, golang-migrate (embedded), `pkg/objtype` (PlayerStat metadata + XP/level math).

**Spec:** `docs/superpowers/specs/2026-06-01-hiscore-port-design.md`

---

## File Structure

- **Create** `modules/login/migrations/000004_hiscore.up.sql` — the two tables (auto-embedded via `//go:embed migrations/*.sql`).
- **Modify** `modules/login/save.go` — add `saveStats` (21 stat XP extractor).
- **Create** `modules/login/hiscore.go` — `updateHiscores` + `upsertHiscore`.
- **Modify** `modules/login/handler.go:288-328` — replace the PORTING-EXCEPTION comment with the real call.
- **Create** `modules/login/hiscore_test.go` — gates, happy path, level-15 gate, skip-when-equal.
- **Modify** `modules/login/save_test.go` — add `makeSaveWithStats` helper + `TestSaveStats`.
- **Modify** `docs/PORTING-CLOSED.md` + `PORTING.md` — flip the row to FIXED.

Test commands are prefixed with the project's Go env per CLAUDE.md:
`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/...`

---

## Task 1: Migration — hiscore tables

**Files:**
- Create: `modules/login/migrations/000004_hiscore.up.sql`
- Test: `modules/login/hiscore_test.go` (new — first test goes here)

- [ ] **Step 1: Write the failing test**

Create `modules/login/hiscore_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestMigrationCreatesHiscoreTables -v`
Expected: FAIL — `table "hiscore" not created by migrations`.

- [ ] **Step 3: Create the migration**

Create `modules/login/migrations/000004_hiscore.up.sql`:

```sql
-- Hiscore leaderboard tables. Mirrors TS prisma models `hiscore` (value Int)
-- and `hiscore_large` (value BigInt) at Engine-TS/prisma/singleworld/schema.prisma:47-69,
-- written on graceful logout by LoginServer.updateHiscores (login-server-9 /
-- gap-db-datastruct-9). SQLite INTEGER is 64-bit, so one column shape serves
-- both the Int per-stat table and the BigInt total table. PK ordering
-- (profile, type, account_id) matches the prisma @@id.
CREATE TABLE hiscore (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);

CREATE TABLE hiscore_large (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestMigrationCreatesHiscoreTables -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/login/migrations/000004_hiscore.up.sql modules/login/hiscore_test.go
git commit --no-gpg-sign -m "feat(login): add hiscore + hiscore_large tables [gap-db-datastruct-9]"
```

---

## Task 2: `saveStats` — extract 21 stat XP values from the save blob

**Files:**
- Modify: `modules/login/save.go`
- Modify: `modules/login/save_test.go`

- [ ] **Step 1: Write the failing test + blob helper**

Add to `modules/login/save_test.go`. First add `"github.com/zsrv/goscape/pkg/objtype"` to its import block, then append:

```go
// makeSaveWithStats builds a version-6 SAV blob carrying the 21 stat XP values
// (each i32 XP + u8 level, level byte left 0). Extends makeValidSave's header
// (offset 0..27) with the 105-byte stat block at offset 28, then a trailing CRC.
func makeSaveWithStats(playtime int32, xps [objtype.PlayerStatCount]int32) []byte {
	const stride = 5
	body := make([]byte, 28+objtype.PlayerStatCount*stride)
	body[0], body[1] = 0x20, 0x04 // magic 0x2004
	body[2], body[3] = 0x00, 0x06 // version 6
	body[24] = byte(playtime >> 24)
	body[25] = byte(playtime >> 16)
	body[26] = byte(playtime >> 8)
	body[27] = byte(playtime)
	for i := range objtype.PlayerStatCount {
		o := 28 + i*stride
		x := uint32(xps[i])
		body[o] = byte(x >> 24)
		body[o+1] = byte(x >> 16)
		body[o+2] = byte(x >> 8)
		body[o+3] = byte(x)
		// body[o+4] (current-level byte) intentionally left 0 — saveStats ignores it.
	}
	crc := packet.GetCRC(body, 0, len(body))
	return append(body, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

func TestSaveStats(t *testing.T) {
	var xps [objtype.PlayerStatCount]int32
	for i := range xps {
		xps[i] = int32((i + 1) * 1000)
	}
	got, ok := saveStats(makeSaveWithStats(42, xps))
	if !ok {
		t.Fatal("saveStats: ok=false for a valid stat-carrying blob")
	}
	if got != xps {
		t.Errorf("saveStats: got %v, want %v", got, xps)
	}

	// A header-only blob (no stat block) is too short → ok=false.
	if _, ok := saveStats(makeValidSave(0)); ok {
		t.Error("saveStats: ok=true for a blob with no stat block")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestSaveStats -v`
Expected: FAIL to compile — `undefined: saveStats`.

- [ ] **Step 3: Implement `saveStats`**

Add `"github.com/zsrv/goscape/pkg/objtype"` to `save.go`'s import block, then append to `save.go`:

```go
// saveStats extracts the 21 per-stat XP values from a SAV blob. Mirrors the
// stat block PlayerLoading reads (modules/world/player_load.go:151-156): right
// after playtime, 21 entries of (i32 XP + u8 current level). Only the XP is
// returned — base levels derive from it via objtype.GetLevelByExp, exactly as
// TS PlayerLoading.ts:94 and player_load.go:154 do (the stored level byte is
// ignored). Version-aware: playtime is i32 for v2+ and u16 for v1
// (player_load.go:144-149). Returns (zero, false) when the blob is too short.
func saveStats(save []byte) ([objtype.PlayerStatCount]int32, bool) {
	var stats [objtype.PlayerStatCount]int32
	if len(save) < 4 {
		return stats, false
	}
	version := uint16(save[2])<<8 | uint16(save[3])
	statsOff := savePlaytimeOffset + 4 // v2+ playtime is i32
	if version < 2 {
		statsOff = savePlaytimeOffset + 2 // v1 playtime is u16
	}
	const stride = 5 // i32 XP + u8 current level
	if len(save) < statsOff+objtype.PlayerStatCount*stride {
		return stats, false
	}
	for i := range objtype.PlayerStatCount {
		o := statsOff + i*stride
		stats[i] = int32(uint32(save[o])<<24 | uint32(save[o+1])<<16 | uint32(save[o+2])<<8 | uint32(save[o+3]))
	}
	return stats, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestSaveStats -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/login/save.go modules/login/save_test.go
git commit --no-gpg-sign -m "feat(login): extract per-stat XP from save blob (saveStats)"
```

---

## Task 3: `updateHiscores` — the export logic

**Files:**
- Create: `modules/login/hiscore.go`
- Modify: `modules/login/hiscore_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `modules/login/hiscore_test.go`. Add imports `"github.com/zsrv/goscape/pkg/objtype"` and `"time"` to its import block, then add this query helper and the tests:

```go
func queryHiscoreRow(t *testing.T, db *sql.DB, table string, accountID, typ int) (level int, value int64, date string, found bool) {
	t.Helper()
	err := db.QueryRow(
		`SELECT level, value, date FROM `+table+` WHERE account_id = ? AND type = ? AND profile = 'main'`,
		accountID, typ,
	).Scan(&level, &value, &date)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, "", false
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

func TestUpdateHiscores_BannedSkip(t *testing.T) {
	db := createTestDB(t)
	id := int(insertTestAccount(t, db, "banned", "pw"))
	if err := setAccountBanned(t.Context(), db, "banned", time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatalf("setAccountBanned: %v", err)
	}
	acct, _ := accountByUsername(t.Context(), db, "banned", "main")

	var levels [objtype.PlayerStatCount]int
	for i := range levels {
		levels[i] = 50
	}
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, statsForLevels(levels)), "main"); err != nil {
		t.Fatalf("updateHiscores: %v", err)
	}
	if _, _, _, found := queryHiscoreRow(t, db, "hiscore_large", id, 0); found {
		t.Error("actively-banned account must be excluded from hiscores")
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
	const sentinel = "2000-01-01 00:00:00"
	if _, err := db.Exec(`UPDATE hiscore SET date = ? WHERE account_id = ? AND type = ?`, sentinel, id, objtype.PlayerStatAttack+1); err != nil {
		t.Fatalf("stamp sentinel: %v", err)
	}

	// Re-export with the SAME stats → value unchanged → WHERE clause suppresses
	// the update → sentinel date preserved.
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, xps), "main"); err != nil {
		t.Fatalf("updateHiscores #2 (unchanged): %v", err)
	}
	if _, _, date, _ := queryHiscoreRow(t, db, "hiscore", id, objtype.PlayerStatAttack+1); date != sentinel {
		t.Errorf("unchanged re-export bumped date: got %q, want sentinel %q", date, sentinel)
	}

	// Re-export with a CHANGED attack XP → value differs → row updated.
	levels[objtype.PlayerStatAttack] = 40
	if err := updateHiscores(t.Context(), db, acct, makeSaveWithStats(0, statsForLevels(levels)), "main"); err != nil {
		t.Fatalf("updateHiscores #3 (changed): %v", err)
	}
	if lvl, _, date, _ := queryHiscoreRow(t, db, "hiscore", id, objtype.PlayerStatAttack+1); lvl != 40 || date == sentinel {
		t.Errorf("changed re-export did not update: level=%d date=%q", lvl, date)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestUpdateHiscores -v`
Expected: FAIL to compile — `undefined: updateHiscores`.

- [ ] **Step 3: Implement `updateHiscores`**

Create `modules/login/hiscore.go`:

```go
package login

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/objtype"
)

// updateHiscores exports a logged-out player's enabled-stat XP and levels into
// the hiscore (per-stat) and hiscore_large (total) tables. Mirrors TS
// LoginServer.updateHiscores (Engine-TS/src/server/login/LoginServer.ts:19-109):
// staff above mod-level-1 and actively-banned accounts are skipped; the total
// (type 0) goes to hiscore_large; per-stat rows (type = stat+1) go to hiscore
// only when the base level is >= 15. XP is the ×10 fixed-point value read
// straight from the save (same scaling as TS). Best-effort: PlayerLogout logs
// any error and still reports success (TS sends the logout response before
// awaiting this), so a hiscore failure never blocks logout.
func updateHiscores(ctx context.Context, db *sql.DB, account *accountRow, save []byte, profile string) error {
	if account == nil {
		return nil
	}
	if account.StaffModLevel > 1 {
		return nil
	}
	now := time.Now().UTC()
	if account.BannedUntil.Valid {
		// Active ban = ban end >= now (TS: banned_until >= new Date()).
		if t, err := time.Parse(dbTimeFormat, account.BannedUntil.String); err == nil && !now.After(t) {
			return nil
		}
	}

	stats, ok := saveStats(save)
	if !ok {
		return fmt.Errorf("updateHiscores: save blob too short to contain stats")
	}

	var totalXp int64
	var totalLevel int
	for i := range objtype.PlayerStatCount {
		if !objtype.PlayerStatEnabled[i] {
			continue
		}
		totalXp += int64(stats[i])
		totalLevel += objtype.GetLevelByExp(int(stats[i]))
	}

	date := now.Format(dbTimeFormat)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("updateHiscores: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := upsertHiscore(ctx, tx, "hiscore_large", account.ID, profile, 0, totalLevel, totalXp, date); err != nil {
		return err
	}

	for stat := range objtype.PlayerStatCount {
		if !objtype.PlayerStatEnabled[stat] {
			continue
		}
		baseLevel := objtype.GetLevelByExp(int(stats[stat]))
		if baseLevel < 15 {
			continue
		}
		if err := upsertHiscore(ctx, tx, "hiscore", account.ID, profile, stat+1, baseLevel, int64(stats[stat]), date); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("updateHiscores: commit: %w", err)
	}
	committed = true
	return nil
}

// upsertHiscore inserts or updates one leaderboard row. The conditional
// ON CONFLICT ... WHERE value <> excluded.value reproduces TS's skip-when-
// unchanged semantic in a single statement: `date` is bumped only when the
// value actually changed. table is a trusted compile-time constant
// ("hiscore" / "hiscore_large"), never user input.
func upsertHiscore(ctx context.Context, tx *sql.Tx, table string, accountID int, profile string, typ, level int, value int64, date string) error {
	q := fmt.Sprintf(`INSERT INTO %s (account_id, profile, type, level, value, date)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(profile, type, account_id)
DO UPDATE SET level = excluded.level, value = excluded.value, date = excluded.date
WHERE %s.value <> excluded.value`, table, table)
	if _, err := tx.ExecContext(ctx, q, accountID, profile, typ, level, value, date); err != nil {
		return fmt.Errorf("updateHiscores: upsert %s type %d: %w", table, typ, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestUpdateHiscores -v`
Expected: PASS (all four `TestUpdateHiscores_*`).

- [ ] **Step 5: Commit**

```bash
git add modules/login/hiscore.go modules/login/hiscore_test.go
git commit --no-gpg-sign -m "feat(login): port updateHiscores export logic [login-server-9]"
```

---

## Task 4: Wire `updateHiscores` into `PlayerLogout`

**Files:**
- Modify: `modules/login/handler.go:288-328`
- Modify: `modules/login/handler_test.go` (augment the happy-path logout test)

- [ ] **Step 1: Write the failing test**

Add to `modules/login/handler_test.go` (it already imports what it needs for handler tests; add `"github.com/zsrv/goscape/pkg/objtype"` if not present):

```go
// TestPlayerLogout_WritesHiscores pins login-server-9: a graceful logout exports
// the player's enabled-stat XP into hiscore_large (type 0) end-to-end through the
// handler, mirroring TS LoginServer.ts:450.
func TestPlayerLogout_WritesHiscores(t *testing.T) {
	h, _ := newTestHandler(t)
	id := int(insertTestAccount(t, h.db, "logouths", "pw"))

	var levels [objtype.PlayerStatCount]int
	for i := range levels {
		levels[i] = 1
	}
	levels[objtype.PlayerStatAttack] = 25
	save := makeSaveWithStats(0, statsForLevels(levels))

	if _, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId: 1, Profile: "main", Username: "logouths", Save: save,
	}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}

	if _, _, _, found := queryHiscoreRow(t, h.db, "hiscore_large", id, 0); !found {
		t.Error("PlayerLogout did not export hiscores (hiscore_large type 0 missing)")
	}
	if _, _, _, found := queryHiscoreRow(t, h.db, "hiscore", id, objtype.PlayerStatAttack+1); !found {
		t.Error("PlayerLogout did not export the attack hiscore row (level 25 >= 15)")
	}
}
```

Note: `newTestHandler` returns `(h, savePath)`; `h.db` is the migrated DB (same `createTestDB` instance used elsewhere — confirm `handler` has a `db` field, which `setLoggedOut(ctx, h.db, ...)` already uses). `queryHiscoreRow`, `makeSaveWithStats`, and `statsForLevels` are defined in Task 2/3 test files in the same package.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestPlayerLogout_WritesHiscores -v`
Expected: FAIL — `hiscore_large type 0 missing` (the handler still has the no-op comment).

- [ ] **Step 3: Replace the PORTING-EXCEPTION comment with the real call**

In `modules/login/handler.go`, replace the entire comment block at lines 304-325 (everything between the `setLoggedOut` block and the `return` statement) with:

```go
	// Export the logged-out player's enabled-stat XP/levels to the hiscore
	// leaderboard tables. Mirrors TS LoginServer.ts:450 (the call site, right
	// after setLoggedOut). Best-effort: TS sends the logout response before
	// awaiting updateHiscores, so a hiscore failure must not fail the logout —
	// log it and still report success (login-server-9 / gap-db-datastruct-9).
	if err := updateHiscores(ctx, h.db, account, req.Save, req.Profile); err != nil {
		h.log.Warn("updateHiscores failed; logout unaffected",
			slog.String("username", req.Username),
			slog.String("profile", req.Profile),
			slog.Any("err", err))
	}

	return &loginpb.PlayerLogoutResponse{Success: true}, nil
```

(Confirm the handler's logger field name — grep shows `h.log` is used elsewhere in the file. If it is `h.logger`, use that. `slog` is already imported in handler.go.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -v`
Expected: PASS — the new test plus all existing login tests (the happy-path/save-failure/logout-time tests are unaffected; a `makeValidSave(N)` blob with no stat block makes `saveStats` return false → `updateHiscores` returns an error that is logged, not fatal, so those tests still see `Success: true`).

- [ ] **Step 5: Commit**

```bash
git add modules/login/handler.go modules/login/handler_test.go
git commit --no-gpg-sign -m "feat(login): call updateHiscores on PlayerLogout [login-server-9]"
```

---

## Task 5: Close the audit IDs in the porting docs

**Files:**
- Modify: `docs/PORTING-CLOSED.md` (the `login-server-9 / gap-db-datastruct-9` row)
- Modify: `PORTING.md` (Arc closure line)

- [ ] **Step 1: Flip the PORTING-CLOSED.md row**

Find the row at `docs/PORTING-CLOSED.md:68` (`⚠️ MED ... login-server-9 / gap-db-datastruct-9 ... EXCEPTION-DOCUMENTED fix/med-bundle-19 (b15c84f9)`). Change its status cell from `✅ **EXCEPTION-DOCUMENTED** fix/med-bundle-19 (b15c84f9)` to `✅ **FIXED** fix/hiscore-port` and append to the row's notes:

```
PROMOTED EXCEPTION → FIXED 2026-06-01 (fix/hiscore-port): the blocking dependency
(absent hiscore subsystem) shipped. Ported the write path to TS parity — migration
000004 adds `hiscore` + `hiscore_large`, and PlayerLogout now calls updateHiscores
(LoginServer.ts:450) to export enabled-stat XP/levels. Scope decision: write-path
only; TS has NO hiscore serving endpoint (`hiscore` appears in TS source only in
db/types.ts + LoginServer.ts), so no read/serve surface was built (YAGNI, no goscape
consumer). Both audit IDs now read FIXED. See docs/superpowers/specs/2026-06-01-hiscore-port-design.md.
```

- [ ] **Step 2: Extend the PORTING.md Arc line**

In `PORTING.md`, locate the most recent Arc 27/28 closure line (the CategoryType cluster closure at the top of the open-items / tracking section) and add a sibling line:

```
- login-server-9 / gap-db-datastruct-9 (hiscore write-path port) — FIXED fix/hiscore-port 2026-06-01; promoted from EXCEPTION-DOCUMENTED (med-bundle-19). Migration 000004 + updateHiscores on PlayerLogout. Write-path parity only (TS has no serving endpoint).
```

(Match the exact bullet/format of the surrounding lines — open `PORTING.md` and mirror the adjacent entries' style; the precise wording is less important than consistency with neighbors.)

- [ ] **Step 3: Verify build + full module test once more**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/login/
```
Expected: build clean; tests PASS; `gofmt -l` prints nothing.

- [ ] **Step 4: Commit**

```bash
git add docs/PORTING-CLOSED.md PORTING.md
git commit --no-gpg-sign -m "docs(porting): close login-server-9 / gap-db-datastruct-9 (hiscore write-path)"
```

---

## Final verification (before FF to main)

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean.
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/...` green.
- [ ] `gofmt -l modules/login/` empty.
- [ ] **#274 no-op flip:** `git diff --name-only main..HEAD` includes neither `deploy/bundled/goscape.yaml` nor `pkg/util/build/build.go`; md5 of both byte-identical pre/post-FF.
- [ ] **#289 parallel-main:** `git rev-parse main` unchanged since branch point (`1ca2bffc`); if advanced, rebase `git -c commit.gpgsign=false rebase main` then re-verify before `merge --ff-only`.
- [ ] Straight-FF onto main (sandbox-disabled retry for the primary-worktree merge, per every prior arc FF).
- [ ] Update `FIX-ARC-RESUME.md` with the landed SHA and counts.
```
