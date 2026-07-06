package login

import (
	"context"
	"fmt"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// getUnreadMessageCount is the SQL port of TS Messages.ts:3-37
// (getUnreadMessageCount, Kysely): count threads the account
// participates in whose newest non-deleted message postdates the
// account's read/deleted status stamps, excluding threads where the
// account itself sent the last message. Returns 0 on empty tables —
// the same observable as the pre-B5 stub until message rows exist
// (goscape has no website writer; the tables are schema parity).
func getUnreadMessageCount(ctx context.Context, db *gamedb.DB, accountID int) (int, error) {
	const query = `
SELECT COUNT(*)
FROM message_thread thd
LEFT JOIN message_status s
       ON s.thread_id = thd.id AND s.account_id = ?
INNER JOIN (
    SELECT thread_id, MAX(created) AS last_message_date
    FROM message
    WHERE deleted IS NULL
    GROUP BY thread_id
) last_message ON last_message.thread_id = thd.id
WHERE (thd.from_account_id = ? OR thd.to_account_id = ?)
  AND (s.deleted IS NULL OR s.deleted < last_message.last_message_date)
  AND (s."read" IS NULL OR s."read" < last_message.last_message_date)
  AND thd.last_message_from != ?`
	var n int
	// accountID binds 4×: status-JOIN scope, from/to participant filter,
	// own-last-message exclusion — keep the arg count in lockstep with
	// the query's `?` placeholders when editing.
	if err := db.QueryRowContext(ctx, db.Rebind(query), accountID, accountID, accountID, accountID).Scan(&n); err != nil {
		return 0, fmt.Errorf("getUnreadMessageCount: %w", err)
	}
	return n, nil
}
