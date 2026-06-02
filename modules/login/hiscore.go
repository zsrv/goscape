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
