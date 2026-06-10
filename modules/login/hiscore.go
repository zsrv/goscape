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
// LoginServer.updateHiscores (Engine-TS/src/server/login/LoginServer.ts:20-26
// @3c16994c): only staff above mod-level-1 are skipped; banned accounts are
// exported normally (245.2 reverts 244's ccc263c7 banned_until gate — the
// 245.2 updateHiscores signature is { id, staffmodlevel } with no
// banned_until). The total (type 0) goes to hiscore_large; per-stat rows
// (type = stat+1) go to hiscore only when the base level is >= 15. XP is the
// ×10 fixed-point value read straight from the save (same scaling as TS).
// Best-effort: PlayerLogout logs any error and still reports success (TS sends
// the logout response before awaiting this), so a hiscore failure never blocks
// logout.
func updateHiscores(ctx context.Context, db *sql.DB, account *accountRow, save []byte, profile string) error {
	if account == nil {
		return nil
	}
	if account.StaffModLevel > 1 {
		return nil
	}
	now := time.Now().UTC()

	stats, ok := saveStats(save)
	if !ok {
		// Defensive no-op: a valid logout save always contains the stat block,
		// so a too-short blob means there is nothing to export. Treat it like
		// the staff/ban gates above — skip silently, do not fail the logout
		// (spec 2026-06-01-hiscore-port-design.md gates).
		return nil
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
